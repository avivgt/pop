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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

const (
	bootStageOnStuckForce = "force"
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
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

func (r *BootSequenceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	bs := &popv1alpha1.BootSequence{}
	if err := r.Get(ctx, req.NamespacedName, bs); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	bs.Status.ObservedGeneration = bs.Generation

	if bs.Status.Phase == popv1alpha1.BootSequencePhaseComplete ||
		bs.Status.Phase == popv1alpha1.BootSequencePhaseTimedOut ||
		bs.Status.Phase == popv1alpha1.BootSequencePhaseFailed {
		return ctrl.Result{}, nil
	}

	if bs.Status.StartTime == nil {
		now := metav1.Now()
		bs.Status.StartTime = &now
		bs.Status.Phase = popv1alpha1.BootSequencePhaseInProgress
	}

	order, err := topoSortStages(bs.Spec.Stages)
	if err != nil {
		bs.Status.Phase = popv1alpha1.BootSequencePhaseFailed
		bs.Status.Message = fmt.Sprintf("invalid stages: %v", err)
		meta.SetStatusCondition(&bs.Status.Conditions, metav1.Condition{
			Type: "Valid", Status: metav1.ConditionFalse, Reason: "InvalidStages", Message: err.Error(),
		})
		if uerr := r.Status().Update(ctx, bs); uerr != nil {
			return ctrl.Result{}, uerr
		}
		return ctrl.Result{}, nil // spec error: wait for the user to fix it
	}

	byName := make(map[string]popv1alpha1.BootStage, len(bs.Spec.Stages))
	for _, s := range bs.Spec.Stages {
		byName[s.Name] = s
	}

	readyByName := make(map[string]bool, len(order))
	statuses := make([]popv1alpha1.BootStageStatus, 0, len(order))
	elapsed := time.Since(bs.Status.StartTime.Time)
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
		if !depsReady {
			msg = "waiting on dependencies"
		} else {
			t, rdy, gerr := r.evaluateHealthGate(ctx, stage)
			if gerr != nil {
				return ctrl.Result{}, gerr
			}
			total, ready = t, rdy
			gatePassed = ready == total // AllPodsReady (0 pods => trivially satisfied)
			if gatePassed {
				msg = fmt.Sprintf("%d/%d pods ready", ready, total)
			} else {
				msg = fmt.Sprintf("%d/%d pods ready, waiting", ready, total)
			}
		}

		forced := false
		stageReady := depsReady && gatePassed
		if !stageReady && depsReady && int32(elapsed.Seconds()) > stage.TimeoutSeconds {
			if stage.OnStuck == bootStageOnStuckForce {
				stageReady = true
				forced = true
				msg = fmt.Sprintf("forced ready after %ds timeout (%d/%d pods ready)",
					stage.TimeoutSeconds, ready, total)
			} else {
				timedOut = true
				msg = fmt.Sprintf("timed out after %ds (%d/%d pods ready)",
					stage.TimeoutSeconds, ready, total)
			}
		}

		readyByName[name] = stageReady
		if !stageReady {
			allReady = false
		}
		statuses = append(statuses, popv1alpha1.BootStageStatus{
			Name: name, Ready: stageReady, Forced: forced,
			PodsTotal: int32(total), PodsReady: int32(ready), Message: msg,
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

// evaluateHealthGate returns (totalPods, readyPods) across the stage's selected
// namespaces, counting only pods that are not already terminal (Succeeded/Failed).
// A pod is ready when its PodReady condition is True.
func (r *BootSequenceReconciler) evaluateHealthGate(ctx context.Context, stage popv1alpha1.BootStage) (total, ready int, err error) {
	for _, ns := range stage.Selector.Namespaces {
		podList := &corev1.PodList{}
		if lerr := r.List(ctx, podList, client.InNamespace(ns)); lerr != nil {
			return 0, 0, lerr
		}
		for i := range podList.Items {
			pod := &podList.Items[i]
			if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
				continue // completed pods (jobs, one-shots) are not gating
			}
			total++
			if podReady(pod) {
				ready++
			}
		}
	}
	return total, ready, nil
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
		Complete(r)
}
