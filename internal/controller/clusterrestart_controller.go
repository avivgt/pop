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

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

// ClusterRestartReconciler reconciles a ClusterRestart object
type ClusterRestartReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// degradedCheckCount tracks consecutive checks with degraded operators
	degradedCheckCount int
}

// +kubebuilder:rbac:groups=pop.openshift.io,resources=clusterrestarts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pop.openshift.io,resources=clusterrestarts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pop.openshift.io,resources=clusterrestarts/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests/approval,verbs=update
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=signers,resourceNames=kubernetes.io/kubelet-serving;kubernetes.io/kube-apiserver-client-kubelet,verbs=approve
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusteroperators,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *ClusterRestartReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	restart := &popv1alpha1.ClusterRestart{}
	if err := r.Get(ctx, req.NamespacedName, restart); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if restart.Status.Phase == popv1alpha1.ClusterRestartPhaseComplete ||
		restart.Status.Phase == popv1alpha1.ClusterRestartPhaseFailed {
		return ctrl.Result{}, nil
	}

	if restart.Status.Phase == "" {
		restart.Status.Phase = popv1alpha1.ClusterRestartPhasePending
		now := metav1.Now()
		restart.Status.StartTime = &now
		if err := r.Status().Update(ctx, restart); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	switch restart.Status.Phase {
	case popv1alpha1.ClusterRestartPhasePending:
		return r.reconcilePending(ctx, restart)
	case popv1alpha1.ClusterRestartPhaseWaitingForControlPlane:
		return r.reconcileWaitingForControlPlane(ctx, restart)
	case popv1alpha1.ClusterRestartPhaseApprovingCSRs:
		return r.reconcileApprovingCSRs(ctx, restart)
	case popv1alpha1.ClusterRestartPhaseWaitingForWorkers:
		return r.reconcileWaitingForWorkers(ctx, restart)
	case popv1alpha1.ClusterRestartPhaseVerifyingOperators:
		return r.reconcileVerifyingOperators(ctx, restart)
	default:
		logger.Info("unknown phase", "phase", restart.Status.Phase)
		return ctrl.Result{}, nil
	}
}

func (r *ClusterRestartReconciler) reconcilePending(ctx context.Context, restart *popv1alpha1.ClusterRestart) (ctrl.Result, error) {
	restart.Status.Phase = popv1alpha1.ClusterRestartPhaseWaitingForControlPlane
	restart.Status.Message = "Waiting for control plane nodes to become Ready"
	if err := r.Status().Update(ctx, restart); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

func (r *ClusterRestartReconciler) reconcileWaitingForControlPlane(ctx context.Context, restart *popv1alpha1.ClusterRestart) (ctrl.Result, error) {
	if r.isTimedOut(restart) {
		return r.setFailed(ctx, restart, "timed out waiting for control plane nodes to become Ready")
	}

	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return ctrl.Result{}, err
	}

	var cpNodeStatuses []popv1alpha1.NodeRestartStatus
	allReady := true

	for _, node := range nodeList.Items {
		_, isMaster := node.Labels["node-role.kubernetes.io/master"]
		_, isCP := node.Labels["node-role.kubernetes.io/control-plane"]
		if !isMaster && !isCP {
			continue
		}

		ready := isNodeReady(&node)
		msg := "NotReady"
		if ready {
			msg = "Ready"
		} else {
			allReady = false
		}

		cpNodeStatuses = append(cpNodeStatuses, popv1alpha1.NodeRestartStatus{
			Name:    node.Name,
			Role:    "control-plane",
			Ready:   ready,
			Message: msg,
		})
	}

	restart.Status.NodeStatuses = cpNodeStatuses

	if allReady && len(cpNodeStatuses) > 0 {
		restart.Status.Phase = popv1alpha1.ClusterRestartPhaseApprovingCSRs
		restart.Status.Message = fmt.Sprintf("All %d control plane nodes are Ready, approving CSRs", len(cpNodeStatuses))
		meta.SetStatusCondition(&restart.Status.Conditions, metav1.Condition{
			Type:    "ControlPlaneReady",
			Status:  metav1.ConditionTrue,
			Reason:  "AllControlPlaneNodesReady",
			Message: fmt.Sprintf("All %d control plane nodes are Ready", len(cpNodeStatuses)),
		})
		if err := r.Status().Update(ctx, restart); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	readyCount := 0
	for _, ns := range cpNodeStatuses {
		if ns.Ready {
			readyCount++
		}
	}
	restart.Status.Message = fmt.Sprintf("Waiting for control plane nodes (%d/%d ready)", readyCount, len(cpNodeStatuses))
	if err := r.Status().Update(ctx, restart); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *ClusterRestartReconciler) reconcileApprovingCSRs(ctx context.Context, restart *popv1alpha1.ClusterRestart) (ctrl.Result, error) {
	csrList := &certificatesv1.CertificateSigningRequestList{}
	if err := r.List(ctx, csrList); err != nil {
		return ctrl.Result{}, err
	}

	logger := log.FromContext(ctx)

	for i := range csrList.Items {
		csr := csrList.Items[i]

		// Skip already approved or denied CSRs
		if isCSRApproved(&csr) {
			continue
		}

		// Only approve kubelet-related CSRs
		if csr.Spec.SignerName != "kubernetes.io/kubelet-serving" &&
			csr.Spec.SignerName != "kubernetes.io/kube-apiserver-client-kubelet" {
			continue
		}

		if restart.Spec.AutoApproveCSRs {
			csr.Status.Conditions = append(csr.Status.Conditions, certificatesv1.CertificateSigningRequestCondition{
				Type:           certificatesv1.CertificateApproved,
				Status:         corev1.ConditionTrue,
				Reason:         "POPAutoApproved",
				Message:        "Approved by POP operator during cluster restart",
				LastUpdateTime: metav1.Now(),
			})

			if err := r.SubResource("approval").Update(ctx, &csr); err != nil {
				logger.Error(err, "failed to approve CSR", "csr", csr.Name)
				continue
			}

			logger.Info("approved CSR", "csr", csr.Name, "signerName", csr.Spec.SignerName)
			restart.Status.ApprovedCSRs++
		}
	}

	restart.Status.Phase = popv1alpha1.ClusterRestartPhaseWaitingForWorkers
	restart.Status.Message = fmt.Sprintf("CSR approval complete (%d approved), waiting for worker nodes", restart.Status.ApprovedCSRs)
	if err := r.Status().Update(ctx, restart); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *ClusterRestartReconciler) reconcileWaitingForWorkers(ctx context.Context, restart *popv1alpha1.ClusterRestart) (ctrl.Result, error) {
	if r.isTimedOut(restart) {
		return r.setFailed(ctx, restart, fmt.Sprintf("timed out waiting for worker nodes to become Ready (%d/%d ready)", restart.Status.ReadyNodes, restart.Status.TotalNodes))
	}

	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return ctrl.Result{}, err
	}

	var nodeStatuses []popv1alpha1.NodeRestartStatus
	var readyCount int32
	totalCount := int32(len(nodeList.Items))

	for _, node := range nodeList.Items {
		role := "worker"
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			role = "control-plane"
		} else if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			role = "control-plane"
		}

		ready := isNodeReady(&node)
		msg := "NotReady"
		if ready {
			msg = "Ready"
			readyCount++
		}

		nodeStatuses = append(nodeStatuses, popv1alpha1.NodeRestartStatus{
			Name:    node.Name,
			Role:    role,
			Ready:   ready,
			Message: msg,
		})
	}

	restart.Status.NodeStatuses = nodeStatuses
	restart.Status.TotalNodes = totalCount
	restart.Status.ReadyNodes = readyCount

	if restart.Spec.ExpectedNodes != nil && totalCount < *restart.Spec.ExpectedNodes {
		restart.Status.Message = fmt.Sprintf("Waiting for nodes to join (%d/%d present, %d expected)", totalCount, *restart.Spec.ExpectedNodes, *restart.Spec.ExpectedNodes)
		if err := r.Status().Update(ctx, restart); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	if readyCount == totalCount && totalCount > 0 {
		restart.Status.Phase = popv1alpha1.ClusterRestartPhaseVerifyingOperators
		restart.Status.Message = fmt.Sprintf("All %d nodes are Ready, verifying ClusterOperators", totalCount)
		meta.SetStatusCondition(&restart.Status.Conditions, metav1.Condition{
			Type:    "AllNodesReady",
			Status:  metav1.ConditionTrue,
			Reason:  "AllNodesReady",
			Message: fmt.Sprintf("All %d nodes are Ready", totalCount),
		})
		r.degradedCheckCount = 0
		if err := r.Status().Update(ctx, restart); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	restart.Status.Message = fmt.Sprintf("Waiting for worker nodes (%d/%d ready)", readyCount, totalCount)
	if err := r.Status().Update(ctx, restart); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *ClusterRestartReconciler) reconcileVerifyingOperators(ctx context.Context, restart *popv1alpha1.ClusterRestart) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	coList := &unstructured.UnstructuredList{}
	coList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "config.openshift.io",
		Version: "v1",
		Kind:    "ClusterOperatorList",
	})

	if err := r.List(ctx, coList); err != nil {
		logger.Info("failed to list ClusterOperators (may not be an OpenShift cluster)", "error", err)
		// If we can't list ClusterOperators, just complete
		return r.setComplete(ctx, restart, "Cluster restart complete (ClusterOperator verification skipped)")
	}

	var degradedOperators []string

	for _, co := range coList.Items {
		name := co.GetName()
		conditions, found, err := unstructured.NestedSlice(co.Object, "status", "conditions")
		if err != nil || !found {
			degradedOperators = append(degradedOperators, name)
			continue
		}

		available := false
		degraded := false

		for _, c := range conditions {
			cond, ok := c.(map[string]interface{})
			if !ok {
				continue
			}

			condType, _, _ := unstructured.NestedString(cond, "type")
			condStatus, _, _ := unstructured.NestedString(cond, "status")

			if condType == "Available" && condStatus == "True" {
				available = true
			}
			if condType == "Degraded" && condStatus == "True" {
				degraded = true
			}
		}

		if !available || degraded {
			degradedOperators = append(degradedOperators, name)
		}
	}

	restart.Status.DegradedOperators = degradedOperators

	if len(degradedOperators) == 0 {
		return r.setComplete(ctx, restart, fmt.Sprintf("Cluster restart complete, all %d ClusterOperators healthy", len(coList.Items)))
	}

	r.degradedCheckCount++
	logger.Info("degraded operators detected", "operators", degradedOperators, "consecutiveChecks", r.degradedCheckCount)

	if r.degradedCheckCount >= 3 {
		// After 3 consecutive checks with degraded operators, complete with warning
		now := metav1.Now()
		restart.Status.Phase = popv1alpha1.ClusterRestartPhaseComplete
		restart.Status.CompletionTime = &now
		restart.Status.Message = fmt.Sprintf("Cluster restart complete with %d degraded operators: %v", len(degradedOperators), degradedOperators)
		meta.SetStatusCondition(&restart.Status.Conditions, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "CompleteWithWarning",
			Message: fmt.Sprintf("Cluster restart complete but %d operators are degraded", len(degradedOperators)),
		})
		meta.SetStatusCondition(&restart.Status.Conditions, metav1.Condition{
			Type:    "DegradedOperators",
			Status:  metav1.ConditionTrue,
			Reason:  "OperatorsDegraded",
			Message: fmt.Sprintf("Degraded operators: %v", degradedOperators),
		})
		if err := r.Status().Update(ctx, restart); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	restart.Status.Message = fmt.Sprintf("Waiting for %d degraded operators to stabilize (check %d/3): %v", len(degradedOperators), r.degradedCheckCount, degradedOperators)
	if err := r.Status().Update(ctx, restart); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *ClusterRestartReconciler) setComplete(ctx context.Context, restart *popv1alpha1.ClusterRestart, message string) (ctrl.Result, error) {
	now := metav1.Now()
	restart.Status.Phase = popv1alpha1.ClusterRestartPhaseComplete
	restart.Status.CompletionTime = &now
	restart.Status.Message = message
	meta.SetStatusCondition(&restart.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "RestartComplete",
		Message: message,
	})
	if err := r.Status().Update(ctx, restart); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ClusterRestartReconciler) setFailed(ctx context.Context, restart *popv1alpha1.ClusterRestart, message string) (ctrl.Result, error) {
	log.FromContext(ctx).Error(fmt.Errorf(message), "restart failed")
	now := metav1.Now()
	restart.Status.Phase = popv1alpha1.ClusterRestartPhaseFailed
	restart.Status.Message = message
	restart.Status.CompletionTime = &now
	meta.SetStatusCondition(&restart.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  "RestartFailed",
		Message: message,
	})
	if err := r.Status().Update(ctx, restart); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ClusterRestartReconciler) isTimedOut(restart *popv1alpha1.ClusterRestart) bool {
	if restart.Status.StartTime == nil {
		return false
	}
	timeout := time.Duration(restart.Spec.WaitTimeoutMinutes) * time.Minute
	return time.Since(restart.Status.StartTime.Time) > timeout
}

func isNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func isCSRApproved(csr *certificatesv1.CertificateSigningRequest) bool {
	for _, condition := range csr.Status.Conditions {
		if condition.Type == certificatesv1.CertificateApproved ||
			condition.Type == certificatesv1.CertificateDenied {
			return true
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterRestartReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&popv1alpha1.ClusterRestart{}).
		Complete(r)
}
