/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

const (
	bootStageOnStuckForce   = "force"
	bootStageDefaultTimeout = int32(600)
)

// BootSequenceReconciler walks a BootSequence's stages in dependency order,
// marking each Ready only when its health gate holds across the selected
// namespaces' pods. It is the intra-cluster, workload-layer complement to the
// hub's FleetBootSequence (which gates whole clusters).
type BootSequenceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=pop.openshift.io,resources=bootsequences,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pop.openshift.io,resources=bootsequences/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pop.openshift.io,resources=bootsequences/finalizers,verbs=update
// +kubebuilder:rbac:groups=pop.openshift.io,resources=clusterrestarts,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

func (r *BootSequenceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	bs := &popv1alpha1.BootSequence{}
	if err := r.Get(ctx, req.NamespacedName, bs); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	restartUID, err := r.latestClusterRestartUID(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	specChanged := bs.Status.ObservedGeneration != 0 && bs.Generation != bs.Status.ObservedGeneration
	newRestart := restartUID != "" && restartUID != bs.Status.LastClusterRestartUID
	if specChanged || (bootSequenceTerminal(bs.Status.Phase) && newRestart) {
		resetBootSequenceStatus(bs)
		bs.Status.LastClusterRestartUID = restartUID
	} else if bootSequenceTerminal(bs.Status.Phase) {
		return ctrl.Result{}, nil
	}

	if bs.Status.LastClusterRestartUID == "" && restartUID != "" {
		bs.Status.LastClusterRestartUID = restartUID
	}

	bs.Status.ObservedGeneration = bs.Generation

	if bs.Status.StartTime == nil {
		now := metav1.Now()
		bs.Status.StartTime = &now
		bs.Status.Phase = popv1alpha1.BootSequencePhaseInProgress
	}

	order, err := topoSortStages(bs.Spec.Stages)
	if err != nil {
		bs.Status.Message = fmt.Sprintf("invalid stages: %v", err)
		meta.SetStatusCondition(&bs.Status.Conditions, metav1.Condition{
			Type: "Valid", Status: metav1.ConditionFalse, Reason: "InvalidStages", Message: err.Error(),
		})
		if uerr := r.Status().Update(ctx, bs); uerr != nil {
			return ctrl.Result{}, uerr
		}
		return ctrl.Result{}, nil
	}
	meta.SetStatusCondition(&bs.Status.Conditions, metav1.Condition{
		Type: "Valid", Status: metav1.ConditionTrue, Reason: "ValidStages", Message: "stage DAG is valid",
	})

	byName := make(map[string]popv1alpha1.BootStage, len(bs.Spec.Stages))
	for _, s := range bs.Spec.Stages {
		byName[s.Name] = s
	}

	prevEligible := map[string]*metav1.Time{}
	for _, s := range bs.Status.Stages {
		if s.EligibleSince != nil {
			prevEligible[s.Name] = s.EligibleSince
		}
	}

	readyByName := make(map[string]bool, len(order))
	statuses := make([]popv1alpha1.BootStageStatus, 0, len(order))
	allReady, timedOut := true, false

	for _, name := range order {
		stage := byName[name]

		depsReady := true
		for _, dep := range stage.DependsOn {
			if !readyByName[dep] {
				depsReady = false
				break
			}
		}

		total, ready := 0, 0
		msg := ""
		gatePassed := false
		var eligibleSince *metav1.Time
		if !depsReady {
			msg = "waiting on dependencies"
		} else {
			if prev, ok := prevEligible[name]; ok {
				eligibleSince = prev
			} else {
				now := metav1.Now()
				eligibleSince = &now
			}
			t, rdy, missing, gerr := r.evaluateHealthGate(ctx, stage)
			if gerr != nil {
				return ctrl.Result{}, gerr
			}
			total, ready = t, rdy
			switch {
			case len(missing) > 0:
				msg = "namespace " + strings.Join(missing, ", ") + " not found"
			case total == 0:
				msg = "waiting for workloads (0 pods)"
			case ready == total:
				gatePassed = true
				msg = fmt.Sprintf("%d/%d pods ready", ready, total)
			default:
				msg = fmt.Sprintf("%d/%d pods ready, waiting", ready, total)
			}
		}

		forced := false
		stageReady := depsReady && gatePassed
		if !stageReady && depsReady && eligibleSince != nil {
			timeout := stage.TimeoutSeconds
			if timeout <= 0 {
				timeout = bootStageDefaultTimeout
			}
			if time.Since(eligibleSince.Time) > time.Duration(timeout)*time.Second {
				if stage.OnStuck == bootStageOnStuckForce {
					stageReady = true
					forced = true
					msg = fmt.Sprintf("forced ready after %ds timeout (%d/%d pods ready)",
						timeout, ready, total)
				} else {
					timedOut = true
					msg = fmt.Sprintf("timed out after %ds (%d/%d pods ready)",
						timeout, ready, total)
				}
			}
		}

		readyByName[name] = stageReady
		if !stageReady {
			allReady = false
		}
		statuses = append(statuses, popv1alpha1.BootStageStatus{
			Name: name, Ready: stageReady, Forced: forced,
			PodsTotal: int32(total), PodsReady: int32(ready), Message: msg,
			EligibleSince: eligibleSince,
		})
	}

	bs.Status.Stages = statuses

	switch {
	case timedOut:
		bs.Status.Phase = popv1alpha1.BootSequencePhaseTimedOut
		bs.Status.Message = "One or more stages did not become ready within their timeout"
		meta.SetStatusCondition(&bs.Status.Conditions, metav1.Condition{
			Type: "Complete", Status: metav1.ConditionFalse, Reason: "StageTimeout", Message: bs.Status.Message,
		})
		if err := r.Status().Update(ctx, bs); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	case allReady:
		bs.Status.Phase = popv1alpha1.BootSequencePhaseComplete
		bs.Status.Message = "All stages ready"
		meta.SetStatusCondition(&bs.Status.Conditions, metav1.Condition{
			Type: "Complete", Status: metav1.ConditionTrue, Reason: "AllStagesReady", Message: "All stages ready",
		})
		if err := r.Status().Update(ctx, bs); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	default:
		bs.Status.Phase = popv1alpha1.BootSequencePhaseInProgress
		bs.Status.Message = "Workload black-start in progress"
		if err := r.Status().Update(ctx, bs); err != nil {
			return ctrl.Result{}, err
		}
		logger.V(1).Info("boot sequence in progress", "stages", len(statuses))
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
}

func bootSequenceTerminal(phase popv1alpha1.BootSequencePhase) bool {
	return phase == popv1alpha1.BootSequencePhaseComplete ||
		phase == popv1alpha1.BootSequencePhaseTimedOut
}

func resetBootSequenceStatus(bs *popv1alpha1.BootSequence) {
	bs.Status = popv1alpha1.BootSequenceStatus{}
}

func (r *BootSequenceReconciler) latestClusterRestartUID(ctx context.Context) (string, error) {
	list := &popv1alpha1.ClusterRestartList{}
	if err := r.List(ctx, list); err != nil {
		return "", err
	}
	var latest *popv1alpha1.ClusterRestart
	for i := range list.Items {
		cr := &list.Items[i]
		if latest == nil || cr.CreationTimestamp.After(latest.CreationTimestamp.Time) {
			latest = cr
		}
	}
	if latest == nil {
		return "", nil
	}
	return string(latest.UID), nil
}

// evaluateHealthGate returns (totalPods, readyPods, missingNamespaces) across
// the stage's selected namespaces, counting only pods that are not already
// terminal (Succeeded/Failed). A pod is ready when its PodReady condition is True.
func (r *BootSequenceReconciler) evaluateHealthGate(ctx context.Context, stage popv1alpha1.BootStage) (total, ready int, missing []string, err error) {
	for _, ns := range stage.Selector.Namespaces {
		namespace := &corev1.Namespace{}
		if gerr := r.Get(ctx, types.NamespacedName{Name: ns}, namespace); gerr != nil {
			if apierrors.IsNotFound(gerr) {
				missing = append(missing, ns)
				continue
			}
			return 0, 0, nil, gerr
		}
		podList := &corev1.PodList{}
		if lerr := r.List(ctx, podList, client.InNamespace(ns)); lerr != nil {
			return 0, 0, nil, lerr
		}
		for i := range podList.Items {
			pod := &podList.Items[i]
			if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
				continue
			}
			total++
			if podReady(pod) {
				ready++
			}
		}
	}
	return total, ready, missing, nil
}

// podReady reports whether the pod's PodReady condition is True.
func podReady(pod *corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// topoSortStages returns the stage names in dependency order, or an error if a
// DependsOn references an unknown stage or the graph has a cycle.
func topoSortStages(stages []popv1alpha1.BootStage) ([]string, error) {
	names := make(map[string]bool, len(stages))
	for _, s := range stages {
		if names[s.Name] {
			return nil, fmt.Errorf("duplicate stage name %q", s.Name)
		}
		names[s.Name] = true
	}
	indeg := make(map[string]int, len(stages))
	deps := make(map[string][]string, len(stages))
	for _, s := range stages {
		if _, ok := indeg[s.Name]; !ok {
			indeg[s.Name] = 0
		}
		for _, d := range s.DependsOn {
			if !names[d] {
				return nil, fmt.Errorf("stage %q depends on unknown stage %q", s.Name, d)
			}
			deps[s.Name] = append(deps[s.Name], d)
			indeg[s.Name]++
		}
	}

	var queue, order []string
	for _, s := range stages {
		if indeg[s.Name] == 0 {
			queue = append(queue, s.Name)
		}
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		order = append(order, n)
		for _, s := range stages {
			for _, d := range deps[s.Name] {
				if d == n {
					indeg[s.Name]--
					if indeg[s.Name] == 0 {
						queue = append(queue, s.Name)
					}
				}
			}
		}
	}
	if len(order) != len(stages) {
		return nil, fmt.Errorf("dependency cycle detected among stages")
	}
	return order, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BootSequenceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&popv1alpha1.BootSequence{}).
		Watches(&popv1alpha1.ClusterRestart{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllBootSequences)).
		Complete(r)
}

func (r *BootSequenceReconciler) enqueueAllBootSequences(ctx context.Context, _ client.Object) []reconcile.Request {
	list := &popv1alpha1.BootSequenceList{}
	if err := r.List(ctx, list); err != nil {
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
			Name: list.Items[i].Name, Namespace: list.Items[i].Namespace,
		}})
	}
	return reqs
}
