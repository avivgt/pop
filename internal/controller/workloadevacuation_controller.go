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
	"errors"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

// WorkloadEvacuationReconciler reconciles a WorkloadEvacuation object
type WorkloadEvacuationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=pop.openshift.io,resources=workloadevacuations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pop.openshift.io,resources=workloadevacuations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pop.openshift.io,resources=workloadevacuations/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *WorkloadEvacuationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	evacuation := &popv1alpha1.WorkloadEvacuation{}
	if err := r.Get(ctx, req.NamespacedName, evacuation); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	evacuation.Status.ObservedGeneration = evacuation.Generation

	// Terminal phases — nothing to do.
	if evacuation.Status.Phase == popv1alpha1.EvacuationPhaseComplete ||
		evacuation.Status.Phase == popv1alpha1.EvacuationPhaseFailed {
		return ctrl.Result{}, nil
	}

	// Initialize: empty phase → Pending.
	if evacuation.Status.Phase == "" {
		evacuation.Status.Phase = popv1alpha1.EvacuationPhasePending
		now := metav1.Now()
		evacuation.Status.StartTime = &now
		if err := r.Status().Update(ctx, evacuation); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	switch evacuation.Status.Phase {
	case popv1alpha1.EvacuationPhasePending:
		return r.reconcilePending(ctx, evacuation)
	case popv1alpha1.EvacuationPhaseEvacuating:
		return r.reconcileEvacuating(ctx, evacuation)
	default:
		logger.Info("unknown phase", "phase", evacuation.Status.Phase)
		return ctrl.Result{}, nil
	}
}

func (r *WorkloadEvacuationReconciler) reconcilePending(ctx context.Context, evacuation *popv1alpha1.WorkloadEvacuation) (ctrl.Result, error) {
	// 1. Determine which namespaces are in scope.
	namespaces, err := r.namespacesInScope(ctx, evacuation.Spec.Namespaces)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 2. Count workloads (Deployments + StatefulSets) across those namespaces.
	totalWorkloads := int32(0)
	for _, ns := range namespaces {
		deployments := &appsv1.DeploymentList{}
		if err := r.List(ctx, deployments, client.InNamespace(ns)); err != nil {
			return ctrl.Result{}, err
		}
		totalWorkloads += int32(len(deployments.Items))

		statefulSets := &appsv1.StatefulSetList{}
		if err := r.List(ctx, statefulSets, client.InNamespace(ns)); err != nil {
			return ctrl.Result{}, err
		}
		totalWorkloads += int32(len(statefulSets.Items))
	}

	evacuation.Status.TotalWorkloads = totalWorkloads

	// 3. Handle dry-run mode.
	if evacuation.Spec.DryRun {
		evacuation.Status.Phase = popv1alpha1.EvacuationPhaseComplete
		evacuation.Status.Message = fmt.Sprintf("Dry run: %d workloads in %d namespaces would be evacuated", totalWorkloads, len(namespaces))
		now := metav1.Now()
		evacuation.Status.CompletionTime = &now

		if evacuation.Spec.Strategy == popv1alpha1.EvacuationStrategyCanary && len(evacuation.Spec.Waves) > 0 {
			evacuation.Status.WaveStatuses = r.buildDryRunWaveStatuses(evacuation.Spec.Waves, totalWorkloads)
		}

		if err := r.Status().Update(ctx, evacuation); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// 4. Initialize wave statuses for real evacuation.
	evacuation.Status.WaveStatuses = r.buildWaveStatuses(evacuation)

	// 5. Transition to Evacuating.
	evacuation.Status.Phase = popv1alpha1.EvacuationPhaseEvacuating
	evacuation.Status.CurrentWave = 1
	evacuation.Status.Message = "Starting evacuation"
	if err := r.Status().Update(ctx, evacuation); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

func (r *WorkloadEvacuationReconciler) reconcileEvacuating(ctx context.Context, evacuation *popv1alpha1.WorkloadEvacuation) (ctrl.Result, error) {
	waveStatuses := evacuation.Status.WaveStatuses
	totalWaves := int32(len(waveStatuses))
	currentWave := evacuation.Status.CurrentWave

	// Safety check.
	if currentWave < 1 || currentWave > totalWaves {
		return r.setFailed(ctx, evacuation, fmt.Sprintf("invalid currentWave %d (total waves: %d)", currentWave, totalWaves))
	}

	// Check for any failed wave.
	for _, ws := range waveStatuses {
		if ws.Phase == "Failed" {
			return r.setFailed(ctx, evacuation, fmt.Sprintf("Wave %d failed: %s", ws.Wave, ws.Message))
		}
	}

	// Current wave index (0-based).
	idx := currentWave - 1
	currentWaveStatus := waveStatuses[idx]

	if currentWaveStatus.Phase == "Complete" {
		// Check if there are more waves.
		if currentWave < totalWaves {
			evacuation.Status.CurrentWave = currentWave + 1
			evacuation.Status.Message = fmt.Sprintf("Wave %d/%d: %d/%d workloads", currentWave+1, totalWaves, waveStatuses[currentWave].Migrated, waveStatuses[currentWave].Total)
			if err := r.Status().Update(ctx, evacuation); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}

		// All waves complete.
		now := metav1.Now()
		evacuation.Status.Phase = popv1alpha1.EvacuationPhaseComplete
		evacuation.Status.CompletionTime = &now
		migratedTotal := int32(0)
		for _, ws := range waveStatuses {
			migratedTotal += ws.Migrated
		}
		evacuation.Status.MigratedWorkloads = migratedTotal
		evacuation.Status.Message = fmt.Sprintf("All %d waves complete, %d workloads migrated", totalWaves, migratedTotal)
		if err := r.Status().Update(ctx, evacuation); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Still in progress — update message and requeue.
	evacuation.Status.Message = fmt.Sprintf("Wave %d/%d: %d/%d workloads", currentWave, totalWaves, currentWaveStatus.Migrated, currentWaveStatus.Total)
	if err := r.Status().Update(ctx, evacuation); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// namespacesInScope returns the list of namespace names to evacuate.
// If explicit namespaces are specified, use those; otherwise list all
// non-system namespaces.
func (r *WorkloadEvacuationReconciler) namespacesInScope(ctx context.Context, explicit []string) ([]string, error) {
	if len(explicit) > 0 {
		return explicit, nil
	}

	nsList := &corev1.NamespaceList{}
	if err := r.List(ctx, nsList); err != nil {
		return nil, err
	}

	namespaces := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		name := ns.Name
		if name == "default" ||
			strings.HasPrefix(name, "openshift-") ||
			strings.HasPrefix(name, "kube-") {
			continue
		}
		namespaces = append(namespaces, name)
	}
	return namespaces, nil
}

// buildDryRunWaveStatuses creates wave statuses for dry-run canary mode,
// showing what each wave would migrate.
func (r *WorkloadEvacuationReconciler) buildDryRunWaveStatuses(waves []popv1alpha1.EvacuationWave, totalWorkloads int32) []popv1alpha1.WaveStatus {
	statuses := make([]popv1alpha1.WaveStatus, 0, len(waves))
	for i, wave := range waves {
		waveTotal := totalWorkloads * wave.Percent / 100
		statuses = append(statuses, popv1alpha1.WaveStatus{
			Wave:     int32(i + 1),
			Percent:  wave.Percent,
			Phase:    "Pending",
			Migrated: 0,
			Total:    waveTotal,
			Message:  fmt.Sprintf("Would migrate %d workloads (%d%%)", waveTotal, wave.Percent),
		})
	}
	return statuses
}

// buildWaveStatuses initializes wave statuses for a real evacuation.
func (r *WorkloadEvacuationReconciler) buildWaveStatuses(evacuation *popv1alpha1.WorkloadEvacuation) []popv1alpha1.WaveStatus {
	totalWorkloads := evacuation.Status.TotalWorkloads

	if evacuation.Spec.Strategy == popv1alpha1.EvacuationStrategyAllAtOnce || len(evacuation.Spec.Waves) == 0 {
		return []popv1alpha1.WaveStatus{
			{
				Wave:    1,
				Percent: 100,
				Phase:   "Pending",
				Total:   totalWorkloads,
			},
		}
	}

	statuses := make([]popv1alpha1.WaveStatus, 0, len(evacuation.Spec.Waves))
	for i, wave := range evacuation.Spec.Waves {
		waveTotal := totalWorkloads * wave.Percent / 100
		statuses = append(statuses, popv1alpha1.WaveStatus{
			Wave:    int32(i + 1),
			Percent: wave.Percent,
			Phase:   "Pending",
			Total:   waveTotal,
		})
	}
	return statuses
}

func (r *WorkloadEvacuationReconciler) setFailed(ctx context.Context, evacuation *popv1alpha1.WorkloadEvacuation, message string) (ctrl.Result, error) {
	log.FromContext(ctx).Error(errors.New(message), "evacuation failed")
	now := metav1.Now()
	evacuation.Status.Phase = popv1alpha1.EvacuationPhaseFailed
	evacuation.Status.Message = message
	evacuation.Status.CompletionTime = &now
	if err := r.Status().Update(ctx, evacuation); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkloadEvacuationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&popv1alpha1.WorkloadEvacuation{}).
		Complete(r)
}
