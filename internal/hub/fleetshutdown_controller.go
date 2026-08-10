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

package hub

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	workv1 "open-cluster-management.io/api/work/v1"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

// spokeNamespace is the namespace on each managed cluster where POP runs and
// where the fanned-out ClusterShutdown is applied.
const spokeNamespace = "pop-system"

// FleetShutdownReconciler reconciles a FleetShutdown by fanning out per-spoke
// ClusterShutdowns via ManifestWork.
type FleetShutdownReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// NOTE: RBAC for the hub controller is hand-maintained in config/hub/role.yaml
// (a distinct ClusterRole for the cmd/hub deployable), NOT via +kubebuilder:rbac
// markers — those aggregate into the spoke manager-role and would over-permission
// the spoke operator with fleet/ManifestWork/Placement access it must not have.

func (r *FleetShutdownReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	fs := &popv1alpha1.FleetShutdown{}
	if err := r.Get(ctx, req.NamespacedName, fs); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	fs.Status.ObservedGeneration = fs.Generation

	if fs.Status.Phase == popv1alpha1.FleetShutdownPhaseCompleted {
		return ctrl.Result{}, nil
	}

	// Resolve the target clusters from the referenced Placement.
	clusters, err := r.resolvePlacement(ctx, fs.Namespace, fs.Spec.PlacementRef)
	if err != nil {
		return ctrl.Result{}, err
	}
	fs.Status.SelectedClusters = int32(len(clusters))

	if len(clusters) == 0 {
		fs.Status.Phase = popv1alpha1.FleetShutdownPhasePending
		fs.Status.Message = "no clusters selected by placement " + fs.Spec.PlacementRef
		if err := r.Status().Update(ctx, fs); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Fan out: ensure a ManifestWork exists per selected cluster.
	for _, clusterName := range clusters {
		if err := r.ensureManifestWork(ctx, fs, clusterName); err != nil {
			logger.Error(err, "failed to ensure ManifestWork", "cluster", clusterName)
			return ctrl.Result{}, err
		}
	}

	// Aggregate per-cluster status from ManifestWork feedback.
	statuses := make([]popv1alpha1.FleetClusterStatus, 0, len(clusters))
	completed, failed := 0, 0
	for _, clusterName := range clusters {
		phase, msg := r.clusterPhase(ctx, fs.Name, clusterName)
		statuses = append(statuses, popv1alpha1.FleetClusterStatus{Name: clusterName, Phase: phase, Message: msg})
		switch phase {
		case string(popv1alpha1.ClusterShutdownPhaseComplete):
			completed++
		case string(popv1alpha1.ClusterShutdownPhaseFailed):
			failed++
		}
	}
	fs.Status.Clusters = statuses
	fs.Status.CompletedClusters = int32(completed)

	if completed+failed == len(clusters) {
		phase := popv1alpha1.FleetShutdownPhaseCompleted
		reason, message := "AllClustersShutDown", "All selected clusters shut down"
		if failed > 0 {
			phase = popv1alpha1.FleetShutdownPhasePartiallyFailed
			reason, message = "SomeClustersFailed", "Some clusters failed to shut down cleanly"
		}
		fs.Status.Phase = phase
		fs.Status.Message = message
		meta.SetStatusCondition(&fs.Status.Conditions, metav1.Condition{
			Type: "Complete", Status: metav1.ConditionTrue, Reason: reason, Message: message,
		})
		if err := r.Status().Update(ctx, fs); err != nil {
			return ctrl.Result{}, err
		}
		// Self-cleaning: remove the ManifestWorks now the operation is terminal.
		if err := r.cleanupManifestWorks(ctx, fs, clusters); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	fs.Status.Phase = popv1alpha1.FleetShutdownPhaseInProgress
	fs.Status.Message = "Fleet shutdown in progress"
	if err := r.Status().Update(ctx, fs); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

// resolvePlacement returns the managed-cluster names selected by the named
// Placement (see resolvePlacementDecisions in placement.go).
func (r *FleetShutdownReconciler) resolvePlacement(ctx context.Context, ns, name string) ([]string, error) {
	return resolvePlacementDecisions(ctx, r.Client, ns, name)
}

// ensureManifestWork creates the ManifestWork wrapping a ClusterShutdown for the
// given cluster if it does not already exist.
func (r *FleetShutdownReconciler) ensureManifestWork(ctx context.Context, fs *popv1alpha1.FleetShutdown, clusterName string) error {
	existing := &workv1.ManifestWork{}
	key := client.ObjectKey{Name: manifestWorkName(fs.Name), Namespace: clusterName}
	err := r.Get(ctx, key, existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	cs := &popv1alpha1.ClusterShutdown{
		ObjectMeta: metav1.ObjectMeta{Name: fs.Name, Namespace: spokeNamespace},
		Spec: popv1alpha1.ClusterShutdownSpec{
			PVCName: fs.Spec.PVCName,
			DryRun:  fs.Spec.DryRun,
			Reason:  fs.Spec.Reason,
		},
	}
	mw, err := buildManifestWork(clusterName, fs.Name, cs)
	if err != nil {
		return err
	}
	return r.Create(ctx, mw)
}

// clusterPhase returns the wrapped ClusterShutdown phase + message for a cluster.
func (r *FleetShutdownReconciler) clusterPhase(ctx context.Context, fsName, clusterName string) (string, string) {
	mw := &workv1.ManifestWork{}
	key := client.ObjectKey{Name: manifestWorkName(fsName), Namespace: clusterName}
	if err := r.Get(ctx, key, mw); err != nil {
		return "", "pending: ManifestWork not found"
	}
	return phaseFromFeedback(mw), feedbackString(mw, "message")
}

// cleanupManifestWorks deletes the fanned-out ManifestWorks once terminal.
func (r *FleetShutdownReconciler) cleanupManifestWorks(ctx context.Context, fs *popv1alpha1.FleetShutdown, clusters []string) error {
	for _, clusterName := range clusters {
		mw := &workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{Name: manifestWorkName(fs.Name), Namespace: clusterName},
		}
		if err := r.Delete(ctx, mw); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FleetShutdownReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// ManifestWorks live in per-cluster namespaces (cross-namespace from the
	// FleetShutdown), so owner references / Owns() don't apply; status is polled
	// via RequeueAfter while an operation is in progress.
	return ctrl.NewControllerManagedBy(mgr).
		For(&popv1alpha1.FleetShutdown{}).
		Complete(r)
}
