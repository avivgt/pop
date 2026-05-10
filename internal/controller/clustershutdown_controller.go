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

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
	popmetrics "github.com/avivgt/pop/internal/metrics"
)

type ClusterShutdownReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=pop.openshift.io,resources=clustershutdowns,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pop.openshift.io,resources=clustershutdowns/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pop.openshift.io,resources=clustershutdowns/finalizers,verbs=update
// +kubebuilder:rbac:groups=pop.openshift.io,resources=etcdbackups,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=pop.openshift.io,resources=workloadevacuations,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create

func (r *ClusterShutdownReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	shutdown := &popv1alpha1.ClusterShutdown{}
	if err := r.Get(ctx, req.NamespacedName, shutdown); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if shutdown.Status.Phase == popv1alpha1.ClusterShutdownPhaseComplete ||
		shutdown.Status.Phase == popv1alpha1.ClusterShutdownPhaseFailed {
		return ctrl.Result{}, nil
	}

	if shutdown.Status.Phase == "" {
		shutdown.Status.Phase = popv1alpha1.ClusterShutdownPhasePending
		now := metav1.Now()
		shutdown.Status.StartTime = &now
		if err := r.Status().Update(ctx, shutdown); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	switch shutdown.Status.Phase {
	case popv1alpha1.ClusterShutdownPhasePending:
		return r.reconcilePending(ctx, shutdown)
	case popv1alpha1.ClusterShutdownPhaseScheduled:
		return r.reconcileScheduled(ctx, shutdown)
	case popv1alpha1.ClusterShutdownPhaseValidating:
		return r.reconcileValidating(ctx, shutdown)
	case popv1alpha1.ClusterShutdownPhaseCordoning:
		return r.reconcileCordoning(ctx, shutdown)
	case popv1alpha1.ClusterShutdownPhaseRunningHooks:
		return r.reconcileRunningHooks(ctx, shutdown)
	case popv1alpha1.ClusterShutdownPhaseNotifyingApps:
		return r.reconcileNotifyingApps(ctx, shutdown)
	case popv1alpha1.ClusterShutdownPhaseBackingUp:
		return r.reconcileBackingUp(ctx, shutdown)
	case popv1alpha1.ClusterShutdownPhaseShuttingDown:
		return r.reconcileShuttingDown(ctx, shutdown)
	default:
		logger.Info("unknown phase", "phase", shutdown.Status.Phase)
		return ctrl.Result{}, nil
	}
}

func (r *ClusterShutdownReconciler) reconcilePending(ctx context.Context, shutdown *popv1alpha1.ClusterShutdown) (ctrl.Result, error) {
	// Compute estimated completion time.
	var hookTime int32
	for _, hook := range shutdown.Spec.PreShutdownHooks {
		hookTime += hook.TimeoutSeconds
	}
	backupTime := int32(300)
	shutdownTime := shutdown.Spec.ShutdownDelayMinutes * 2 * 60

	base := time.Now()
	if shutdown.Spec.ScheduledTime != nil {
		base = shutdown.Spec.ScheduledTime.Time
	}
	estimated := metav1.NewTime(base.Add(time.Duration(hookTime+backupTime+shutdownTime) * time.Second))
	shutdown.Status.EstimatedCompletionTime = &estimated

	if shutdown.Spec.ScheduledTime != nil && shutdown.Spec.ScheduledTime.Time.After(time.Now()) {
		shutdown.Status.Phase = popv1alpha1.ClusterShutdownPhaseScheduled
		shutdown.Status.Message = fmt.Sprintf("Shutdown scheduled for %s", shutdown.Spec.ScheduledTime.Time.Format(time.RFC3339))
		if err := r.Status().Update(ctx, shutdown); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Until(shutdown.Spec.ScheduledTime.Time)}, nil
	}

	shutdown.Status.Phase = popv1alpha1.ClusterShutdownPhaseValidating
	shutdown.Status.Message = "Validating prerequisites"
	if err := r.Status().Update(ctx, shutdown); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

func (r *ClusterShutdownReconciler) reconcileScheduled(ctx context.Context, shutdown *popv1alpha1.ClusterShutdown) (ctrl.Result, error) {
	if shutdown.Spec.ScheduledTime != nil && shutdown.Spec.ScheduledTime.Time.After(time.Now()) {
		shutdown.Status.Message = fmt.Sprintf("Shutdown scheduled for %s", shutdown.Spec.ScheduledTime.Time.Format(time.RFC3339))
		if err := r.Status().Update(ctx, shutdown); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Until(shutdown.Spec.ScheduledTime.Time)}, nil
	}

	shutdown.Status.Phase = popv1alpha1.ClusterShutdownPhaseValidating
	shutdown.Status.Message = "Scheduled time reached, validating prerequisites"
	if err := r.Status().Update(ctx, shutdown); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

func (r *ClusterShutdownReconciler) reconcileValidating(ctx context.Context, shutdown *popv1alpha1.ClusterShutdown) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !shutdown.Spec.SkipCertCheck {
		expiry, err := r.checkCertExpiry(ctx)
		if err != nil {
			logger.Error(err, "failed to check certificate expiry")
			return r.setFailed(ctx, shutdown, fmt.Sprintf("failed to check certificate expiry: %v", err))
		}

		if expiry != nil {
			shutdown.Status.CertificateExpiry = expiry
			popmetrics.CertificateExpiryTimestamp.Set(float64(expiry.Time.Unix()))
			threshold := time.Duration(shutdown.Spec.CertExpiryThresholdDays) * 24 * time.Hour
			remaining := time.Until(expiry.Time)
			if remaining < threshold {
				return r.setFailed(ctx, shutdown, fmt.Sprintf(
					"certificate expires on %s (in %d days), within threshold of %d days — renew certificates before shutdown",
					expiry.Time.Format(time.RFC3339),
					int(remaining.Hours()/24),
					shutdown.Spec.CertExpiryThresholdDays,
				))
			}
			logger.Info("certificate expiry check passed",
				"expiry", expiry.Time.Format(time.RFC3339),
				"remainingDays", int(remaining.Hours()/24))
		}
	}

	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return ctrl.Result{}, err
	}

	var nodeStatuses []popv1alpha1.NodeShutdownState
	for _, node := range nodeList.Items {
		if !isNodeReady(&node) {
			logger.Info("skipping NotReady node", "node", node.Name)
			continue
		}
		role := "worker"
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			role = "control-plane"
		} else if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			role = "control-plane"
		}
		nodeStatuses = append(nodeStatuses, popv1alpha1.NodeShutdownState{
			Name:   node.Name,
			Role:   role,
			Status: "Pending",
		})
	}

	if len(nodeStatuses) == 0 {
		return r.setFailed(ctx, shutdown, "no ready nodes found in the cluster")
	}

	shutdown.Status.NodeStatuses = nodeStatuses
	shutdown.Status.TotalNodes = int32(len(nodeStatuses))

	// Check WorkloadEvacuation reference if set.
	if shutdown.Spec.WorkloadEvacuationRef != "" {
		evacuation := &popv1alpha1.WorkloadEvacuation{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      shutdown.Spec.WorkloadEvacuationRef,
			Namespace: shutdown.Namespace,
		}, evacuation)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("WorkloadEvacuation not found, proceeding", "ref", shutdown.Spec.WorkloadEvacuationRef)
			} else {
				return ctrl.Result{}, err
			}
		} else {
			switch evacuation.Status.Phase {
			case popv1alpha1.EvacuationPhaseComplete:
				logger.Info("WorkloadEvacuation completed", "ref", shutdown.Spec.WorkloadEvacuationRef)
			case popv1alpha1.EvacuationPhaseFailed:
				logger.Info("WorkloadEvacuation failed, proceeding anyway", "ref", shutdown.Spec.WorkloadEvacuationRef)
			default:
				shutdown.Status.Message = fmt.Sprintf("Waiting for WorkloadEvacuation %s (phase: %s)", shutdown.Spec.WorkloadEvacuationRef, evacuation.Status.Phase)
				if err := r.Status().Update(ctx, shutdown); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		}
	}

	shutdown.Status.Phase = popv1alpha1.ClusterShutdownPhaseCordoning
	shutdown.Status.Message = fmt.Sprintf("Validation passed, cordoning nodes (%d nodes enumerated)", len(nodeStatuses))
	meta.SetStatusCondition(&shutdown.Status.Conditions, metav1.Condition{
		Type:    "Validated",
		Status:  metav1.ConditionTrue,
		Reason:  "PrerequisitesMet",
		Message: fmt.Sprintf("Certificate check passed, %d nodes enumerated", len(nodeStatuses)),
	})
	if err := r.Status().Update(ctx, shutdown); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

func (r *ClusterShutdownReconciler) reconcileCordoning(ctx context.Context, shutdown *popv1alpha1.ClusterShutdown) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !shutdown.Spec.CordonNodes || shutdown.Spec.DryRun {
		logger.Info("node cordoning skipped", "cordonNodes", shutdown.Spec.CordonNodes, "dryRun", shutdown.Spec.DryRun)
		shutdown.Status.Phase = popv1alpha1.ClusterShutdownPhaseRunningHooks
		shutdown.Status.Message = "Cordoning skipped, running pre-shutdown hooks"
		if err := r.Status().Update(ctx, shutdown); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	var cordonedCount int32
	for _, ns := range shutdown.Status.NodeStatuses {
		node := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: ns.Name}, node); err != nil {
			logger.Error(err, "failed to get node for cordoning", "node", ns.Name)
			continue
		}

		if node.Spec.Unschedulable {
			cordonedCount++
			continue
		}

		original := node.DeepCopy()
		node.Spec.Unschedulable = true
		if err := r.Patch(ctx, node, client.MergeFrom(original)); err != nil {
			logger.Error(err, "failed to cordon node", "node", ns.Name)
			continue
		}
		logger.Info("cordoned node", "node", ns.Name)
		cordonedCount++
	}

	shutdown.Status.CordonedNodes = cordonedCount

	nextPhase := popv1alpha1.ClusterShutdownPhaseRunningHooks
	nextMessage := "All nodes cordoned, running pre-shutdown hooks"
	if len(shutdown.Spec.PreShutdownHooks) == 0 {
		nextPhase = popv1alpha1.ClusterShutdownPhaseNotifyingApps
		nextMessage = "All nodes cordoned, notifying applications"
	}

	shutdown.Status.Phase = nextPhase
	shutdown.Status.Message = nextMessage
	meta.SetStatusCondition(&shutdown.Status.Conditions, metav1.Condition{
		Type:    "NodesCordoned",
		Status:  metav1.ConditionTrue,
		Reason:  "AllNodesCordoned",
		Message: fmt.Sprintf("Cordoned %d nodes", cordonedCount),
	})
	if err := r.Status().Update(ctx, shutdown); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

func (r *ClusterShutdownReconciler) reconcileRunningHooks(ctx context.Context, shutdown *popv1alpha1.ClusterShutdown) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if len(shutdown.Spec.PreShutdownHooks) == 0 {
		logger.Info("no pre-shutdown hooks configured, skipping")
		shutdown.Status.Phase = popv1alpha1.ClusterShutdownPhaseNotifyingApps
		shutdown.Status.Message = "No hooks to run, notifying applications"
		if err := r.Status().Update(ctx, shutdown); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Initialize hook statuses if empty.
	if len(shutdown.Status.HookStatuses) == 0 {
		for _, hook := range shutdown.Spec.PreShutdownHooks {
			shutdown.Status.HookStatuses = append(shutdown.Status.HookStatuses, popv1alpha1.HookStatus{
				Name:  hook.Name,
				Phase: "Pending",
			})
		}
	}

	allCompleted := true
	for i, hook := range shutdown.Spec.PreShutdownHooks {
		hs := &shutdown.Status.HookStatuses[i]

		switch hs.Phase {
		case "Pending":
			allCompleted = false
			jobName := fmt.Sprintf("shutdown-hook-%s-%s", shutdown.Name, hook.Name)
			if len(jobName) > 63 {
				jobName = jobName[:63]
			}

			backoffLimit := int32(0)
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobName,
					Namespace: shutdown.Namespace,
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "pop-operator",
						"pop.openshift.io/shutdown":    shutdown.Name,
						"pop.openshift.io/hook":        hook.Name,
					},
				},
				Spec: batchv1.JobSpec{
					ActiveDeadlineSeconds: &[]int64{int64(hook.TimeoutSeconds)}[0],
					BackoffLimit:          &backoffLimit,
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{
									Name:    "hook",
									Image:   hook.Image,
									Command: hook.Command,
								},
							},
						},
					},
				},
			}

			if err := ctrl.SetControllerReference(shutdown, job, r.Scheme); err != nil {
				logger.Error(err, "failed to set owner reference on hook job", "hook", hook.Name)
				continue
			}
			if err := r.Create(ctx, job); err != nil {
				if apierrors.IsAlreadyExists(err) {
					hs.Phase = "Running"
					hs.JobName = jobName
					continue
				}
				logger.Error(err, "failed to create hook job", "hook", hook.Name)
				hs.Phase = "Failed"
				hs.Message = fmt.Sprintf("failed to create job: %v", err)
				continue
			}

			logger.Info("created hook job", "hook", hook.Name, "job", jobName)
			hs.Phase = "Running"
			hs.JobName = jobName

		case "Running":
			allCompleted = false
			job := &batchv1.Job{}
			if err := r.Get(ctx, types.NamespacedName{Name: hs.JobName, Namespace: shutdown.Namespace}, job); err != nil {
				if apierrors.IsNotFound(err) {
					hs.Phase = "Failed"
					hs.Message = "hook job not found"
				} else {
					logger.Error(err, "failed to get hook job", "hook", hook.Name)
				}
				continue
			}

			if job.Status.Succeeded > 0 {
				hs.Phase = "Completed"
				hs.Message = "hook completed successfully"
			} else if job.Status.Failed > 0 {
				hs.Phase = "Failed"
				hs.Message = "hook job failed"
			}

		case "Completed":
			// Already done.

		case "Failed":
			// Will be caught below.
		}
	}

	// Check for any failures.
	for _, hs := range shutdown.Status.HookStatuses {
		if hs.Phase == "Failed" {
			if err := r.Status().Update(ctx, shutdown); err != nil {
				return ctrl.Result{}, err
			}
			return r.setFailed(ctx, shutdown, fmt.Sprintf("pre-shutdown hook %q failed: %s", hs.Name, hs.Message))
		}
	}

	if allCompleted {
		shutdown.Status.Phase = popv1alpha1.ClusterShutdownPhaseNotifyingApps
		shutdown.Status.Message = "All hooks completed, notifying applications"
		meta.SetStatusCondition(&shutdown.Status.Conditions, metav1.Condition{
			Type:    "HooksCompleted",
			Status:  metav1.ConditionTrue,
			Reason:  "AllHooksSucceeded",
			Message: fmt.Sprintf("All %d pre-shutdown hooks completed successfully", len(shutdown.Spec.PreShutdownHooks)),
		})
		if err := r.Status().Update(ctx, shutdown); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	shutdown.Status.Message = "Running pre-shutdown hooks"
	if err := r.Status().Update(ctx, shutdown); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *ClusterShutdownReconciler) reconcileNotifyingApps(ctx context.Context, shutdown *popv1alpha1.ClusterShutdown) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !shutdown.Spec.NotifyApplications {
		logger.Info("application notification disabled, skipping")
		shutdown.Status.Phase = popv1alpha1.ClusterShutdownPhaseBackingUp
		shutdown.Status.Message = "Notification skipped, starting backup"
		if err := r.Status().Update(ctx, shutdown); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	cmName := "pop-shutdown-signal"
	estimatedShutdownTime := ""
	if shutdown.Status.EstimatedCompletionTime != nil {
		estimatedShutdownTime = shutdown.Status.EstimatedCompletionTime.Time.Format(time.RFC3339)
	}

	data := map[string]string{
		"status":                "imminent",
		"shutdownName":          shutdown.Name,
		"reason":                shutdown.Spec.Reason,
		"estimatedShutdownTime": estimatedShutdownTime,
	}

	existingCM := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: shutdown.Namespace}, existingCM)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cmName,
				Namespace: shutdown.Namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "pop-operator",
					"pop.openshift.io/shutdown":    shutdown.Name,
				},
			},
			Data: data,
		}
		if err := r.Create(ctx, cm); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("created shutdown signal ConfigMap", "name", cmName)
	} else {
		existingCM.Data = data
		if err := r.Update(ctx, existingCM); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("updated shutdown signal ConfigMap", "name", cmName)
	}

	meta.SetStatusCondition(&shutdown.Status.Conditions, metav1.Condition{
		Type:    "ApplicationsNotified",
		Status:  metav1.ConditionTrue,
		Reason:  "SignalConfigMapCreated",
		Message: fmt.Sprintf("Created shutdown signal ConfigMap %s", cmName),
	})

	shutdown.Status.Phase = popv1alpha1.ClusterShutdownPhaseBackingUp
	shutdown.Status.Message = "Applications notified, starting backup"
	if err := r.Status().Update(ctx, shutdown); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

func (r *ClusterShutdownReconciler) reconcileBackingUp(ctx context.Context, shutdown *popv1alpha1.ClusterShutdown) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	backupName := fmt.Sprintf("shutdown-%s", shutdown.Name)

	existingBackup := &popv1alpha1.EtcdBackup{}
	err := r.Get(ctx, types.NamespacedName{Name: backupName, Namespace: shutdown.Namespace}, existingBackup)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		etcdBackup := &popv1alpha1.EtcdBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      backupName,
				Namespace: shutdown.Namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "pop-operator",
					"pop.openshift.io/shutdown":    shutdown.Name,
				},
			},
			Spec: popv1alpha1.EtcdBackupSpec{
				PVCName:              shutdown.Spec.PVCName,
				BackupTimeoutMinutes: 15,
			},
		}
		if err := ctrl.SetControllerReference(shutdown, etcdBackup, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, etcdBackup); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
			return ctrl.Result{}, err
		}

		logger.Info("created EtcdBackup", "name", backupName)
		shutdown.Status.EtcdBackupRef = backupName
		shutdown.Status.Message = "etcd backup in progress"
		if err := r.Status().Update(ctx, shutdown); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	switch existingBackup.Status.Phase {
	case popv1alpha1.EtcdBackupPhaseCompleted:
		logger.Info("etcd backup completed")
		meta.SetStatusCondition(&shutdown.Status.Conditions, metav1.Condition{
			Type:    "BackupCompleted",
			Status:  metav1.ConditionTrue,
			Reason:  "EtcdBackupSucceeded",
			Message: fmt.Sprintf("etcd backup %s completed successfully", backupName),
		})

		if shutdown.Spec.DryRun {
			now := metav1.Now()
			shutdown.Status.Phase = popv1alpha1.ClusterShutdownPhaseComplete
			shutdown.Status.CompletionTime = &now
			shutdown.Status.Message = "Dry run complete — backup successful, no nodes were shut down"
			meta.SetStatusCondition(&shutdown.Status.Conditions, metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Reason:  "DryRunComplete",
				Message: "Dry run completed successfully",
			})
			if err := r.Status().Update(ctx, shutdown); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		shutdown.Status.Phase = popv1alpha1.ClusterShutdownPhaseShuttingDown
		shutdown.Status.Message = "Starting graceful shutdown"
		if err := r.Status().Update(ctx, shutdown); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil

	case popv1alpha1.EtcdBackupPhaseFailed:
		return r.setFailed(ctx, shutdown, fmt.Sprintf("etcd backup failed: %s", existingBackup.Status.Message))

	default:
		shutdown.Status.Message = fmt.Sprintf("Waiting for etcd backup (phase: %s)", existingBackup.Status.Phase)
		if err := r.Status().Update(ctx, shutdown); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
}

func (r *ClusterShutdownReconciler) reconcileShuttingDown(ctx context.Context, shutdown *popv1alpha1.ClusterShutdown) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	workerDelay := shutdown.Spec.ShutdownDelayMinutes
	controlPlaneDelay := shutdown.Spec.ShutdownDelayMinutes * 2

	allDone := true
	anyFailed := false
	shutdownCount := int32(0)

	for i := range shutdown.Status.NodeStatuses {
		ns := &shutdown.Status.NodeStatuses[i]

		if ns.Status == "Shutdown" {
			shutdownCount++
			continue
		}
		if ns.Status == "Failed" {
			anyFailed = true
			shutdownCount++
			continue
		}

		if ns.Status == "ShuttingDown" {
			allDone = false
			pod := &corev1.Pod{}
			err := r.Get(ctx, types.NamespacedName{Name: ns.ShutdownPodName, Namespace: shutdown.Namespace}, pod)
			if err != nil {
				if apierrors.IsNotFound(err) {
					ns.Status = "Failed"
					ns.Message = "shutdown pod not found"
					anyFailed = true
					shutdownCount++
				}
				continue
			}

			switch pod.Status.Phase {
			case corev1.PodSucceeded:
				ns.Status = "Shutdown"
				ns.Message = "shutdown command completed"
				shutdownCount++
			case corev1.PodFailed:
				ns.Status = "Failed"
				ns.Message = "shutdown pod failed"
				anyFailed = true
				shutdownCount++
			}
			continue
		}

		// Status == "Pending" — create the shutdown pod
		allDone = false
		delay := workerDelay
		if ns.Role == "control-plane" {
			delay = controlPlaneDelay
		}

		pod := r.buildShutdownPod(shutdown, ns, delay)
		if err := ctrl.SetControllerReference(shutdown, pod, r.Scheme); err != nil {
			logger.Error(err, "failed to set owner reference on shutdown pod", "node", ns.Name)
			continue
		}
		if err := r.Create(ctx, pod); err != nil {
			if apierrors.IsAlreadyExists(err) {
				ns.Status = "ShuttingDown"
				ns.ShutdownPodName = pod.Name
				continue
			}
			logger.Error(err, "failed to create shutdown pod", "node", ns.Name)
			ns.Status = "Failed"
			ns.Message = fmt.Sprintf("failed to create shutdown pod: %v", err)
			anyFailed = true
			shutdownCount++
			continue
		}

		logger.Info("created shutdown pod", "node", ns.Name, "pod", pod.Name, "delay", delay)
		ns.Status = "ShuttingDown"
		ns.ShutdownPodName = pod.Name
	}

	shutdown.Status.ShutdownNodes = shutdownCount

	if allDone {
		now := metav1.Now()
		shutdown.Status.CompletionTime = &now
		if anyFailed {
			shutdown.Status.Phase = popv1alpha1.ClusterShutdownPhaseFailed
			shutdown.Status.Message = "Shutdown completed with failures on some nodes"
			meta.SetStatusCondition(&shutdown.Status.Conditions, metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionFalse,
				Reason:  "ShutdownPartialFailure",
				Message: "Some nodes failed to shut down",
			})
		} else {
			shutdown.Status.Phase = popv1alpha1.ClusterShutdownPhaseComplete
			shutdown.Status.Message = fmt.Sprintf("All %d nodes shutting down", shutdown.Status.TotalNodes)
			dryrun := "false"
			if shutdown.Spec.DryRun {
				dryrun = "true"
			}
			popmetrics.ClusterShutdownTotal.WithLabelValues("success", dryrun).Inc()
			meta.SetStatusCondition(&shutdown.Status.Conditions, metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Reason:  "ShutdownComplete",
				Message: fmt.Sprintf("Shutdown initiated on all %d nodes", shutdown.Status.TotalNodes),
			})
		}
		if err := r.Status().Update(ctx, shutdown); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	shutdown.Status.Message = fmt.Sprintf("Shutting down nodes (%d/%d)", shutdownCount, shutdown.Status.TotalNodes)
	if err := r.Status().Update(ctx, shutdown); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *ClusterShutdownReconciler) buildShutdownPod(shutdown *popv1alpha1.ClusterShutdown, ns *popv1alpha1.NodeShutdownState, delayMinutes int32) *corev1.Pod {
	privileged := true
	runAsRoot := int64(0)
	hostPathType := corev1.HostPathDirectory
	podName := fmt.Sprintf("shutdown-%s-%s", shutdown.Name, ns.Name)
	if len(podName) > 63 {
		podName = podName[:63]
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: shutdown.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "pop-node-shutdown",
				"app.kubernetes.io/managed-by": "pop-operator",
				"pop.openshift.io/shutdown":    shutdown.Name,
				"pop.openshift.io/target-node": ns.Name,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			HostNetwork:   true,
			HostPID:       true,
			NodeSelector: map[string]string{
				"kubernetes.io/hostname": ns.Name,
			},
			Tolerations: []corev1.Toleration{
				{
					Key:      "node-role.kubernetes.io/master",
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoSchedule,
				},
				{
					Key:      "node-role.kubernetes.io/control-plane",
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
			Containers: []corev1.Container{
				{
					Name:    "shutdown",
					Image:   "registry.access.redhat.com/ubi9/ubi-minimal:latest",
					Command: []string{"/bin/bash", "-c"},
					Args: []string{
						fmt.Sprintf(`echo "Initiating shutdown of node $(hostname) in %d minute(s)..."
chroot /host shutdown -h %d`, delayMinutes, delayMinutes),
					},
					SecurityContext: &corev1.SecurityContext{
						Privileged: &privileged,
						RunAsUser:  &runAsRoot,
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "host",
							MountPath: "/host",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "host",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/",
							Type: &hostPathType,
						},
					},
				},
			},
		},
	}
}

func (r *ClusterShutdownReconciler) checkCertExpiry(ctx context.Context) (*metav1.Time, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      "kube-apiserver-to-kubelet-signer",
		Namespace: "openshift-kube-apiserver-operator",
	}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	expiryStr, ok := secret.Annotations["auth.openshift.io/certificate-not-after"]
	if !ok {
		return nil, nil
	}

	expiry, err := time.Parse(time.RFC3339, expiryStr)
	if err != nil {
		expiry, err = time.Parse("2006-01-02T15:04:05Z", expiryStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse certificate expiry %q: %w", expiryStr, err)
		}
	}

	t := metav1.NewTime(expiry)
	return &t, nil
}

func (r *ClusterShutdownReconciler) setFailed(ctx context.Context, shutdown *popv1alpha1.ClusterShutdown, message string) (ctrl.Result, error) {
	log.FromContext(ctx).Error(fmt.Errorf(message), "shutdown failed")
	popmetrics.ClusterShutdownTotal.WithLabelValues("failure", "false").Inc()
	now := metav1.Now()
	shutdown.Status.Phase = popv1alpha1.ClusterShutdownPhaseFailed
	shutdown.Status.Message = message
	shutdown.Status.CompletionTime = &now
	meta.SetStatusCondition(&shutdown.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  "ShutdownFailed",
		Message: message,
	})
	if err := r.Status().Update(ctx, shutdown); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ClusterShutdownReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&popv1alpha1.ClusterShutdown{}).
		Owns(&popv1alpha1.EtcdBackup{}).
		Owns(&corev1.Pod{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
