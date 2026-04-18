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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

// PowerEventReconciler reconciles a PowerEvent object
type PowerEventReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=pop.openshift.io,resources=powerevents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pop.openshift.io,resources=powerevents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pop.openshift.io,resources=powerevents/finalizers,verbs=update
// +kubebuilder:rbac:groups=pop.openshift.io,resources=powerprotectionconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=pop.openshift.io,resources=etcdbackups,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=pop.openshift.io,resources=clustershutdowns,verbs=get;list;watch;create
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *PowerEventReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	event := &popv1alpha1.PowerEvent{}
	if err := r.Get(ctx, req.NamespacedName, event); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Terminal states — nothing to do.
	if event.Status.Phase == popv1alpha1.PowerEventPhaseComplete ||
		event.Status.Phase == popv1alpha1.PowerEventPhaseFailed ||
		event.Status.Phase == popv1alpha1.PowerEventPhaseCancelled {
		return ctrl.Result{}, nil
	}

	// Initial: set phase to Detected.
	if event.Status.Phase == "" {
		now := metav1.Now()
		event.Status.Phase = popv1alpha1.PowerEventPhaseDetected
		event.Status.DetectedAt = &now
		event.Status.Message = "Power event detected"
		meta.SetStatusCondition(&event.Status.Conditions, metav1.Condition{
			Type:    "Detected",
			Status:  metav1.ConditionTrue,
			Reason:  "EventDetected",
			Message: fmt.Sprintf("Power event of type %s detected", event.Spec.EventType),
		})
		if err := r.Status().Update(ctx, event); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	switch event.Status.Phase {
	case popv1alpha1.PowerEventPhaseDetected:
		return r.reconcileDetected(ctx, event)
	case popv1alpha1.PowerEventPhaseBackingUp:
		return r.reconcileBackingUp(ctx, event)
	case popv1alpha1.PowerEventPhaseAwaitingConfirmation:
		return r.reconcileAwaitingConfirmation(ctx, event)
	case popv1alpha1.PowerEventPhaseConfirmed:
		return r.reconcileConfirmed(ctx, event)
	case popv1alpha1.PowerEventPhaseShuttingDown:
		return r.reconcileShuttingDown(ctx, event)
	default:
		logger.Info("unknown phase", "phase", event.Status.Phase)
		return ctrl.Result{}, nil
	}
}

func (r *PowerEventReconciler) reconcileDetected(ctx context.Context, event *popv1alpha1.PowerEvent) (ctrl.Result, error) {
	switch event.Spec.EventType {
	case popv1alpha1.PowerEventTypeRestored:
		return r.handlePowerRestored(ctx, event)
	case popv1alpha1.PowerEventTypeFailure:
		return r.handlePowerFailure(ctx, event)
	case popv1alpha1.PowerEventTypeTest:
		return r.handleTestEvent(ctx, event)
	default:
		return r.setFailed(ctx, event, fmt.Sprintf("Unknown event type: %s", event.Spec.EventType))
	}
}

func (r *PowerEventReconciler) handlePowerRestored(ctx context.Context, event *popv1alpha1.PowerEvent) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Find any active PowerEvent with type PowerFailure.
	eventList := &popv1alpha1.PowerEventList{}
	if err := r.List(ctx, eventList, client.InNamespace(event.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

	for i := range eventList.Items {
		pe := &eventList.Items[i]
		if pe.Name == event.Name {
			continue
		}
		if pe.Spec.EventType != popv1alpha1.PowerEventTypeFailure {
			continue
		}
		if isTerminalPhase(pe.Status.Phase) {
			continue
		}

		// Cancel the active PowerFailure event.
		now := metav1.Now()
		pe.Status.Phase = popv1alpha1.PowerEventPhaseCancelled
		pe.Status.CancelledAt = &now
		pe.Status.Message = "Power restored. Shutdown cancelled. Backup retained."
		meta.SetStatusCondition(&pe.Status.Conditions, metav1.Condition{
			Type:    "Cancelled",
			Status:  metav1.ConditionTrue,
			Reason:  "PowerRestored",
			Message: "Power restored before shutdown completed",
		})
		if err := r.Status().Update(ctx, pe); err != nil {
			logger.Error(err, "failed to cancel active PowerFailure event", "name", pe.Name)
			return ctrl.Result{}, err
		}
		logger.Info("cancelled active PowerFailure event", "name", pe.Name)
	}

	// Set this PowerRestored event to Complete.
	event.Status.Phase = popv1alpha1.PowerEventPhaseComplete
	event.Status.Message = "Power restored event processed"
	meta.SetStatusCondition(&event.Status.Conditions, metav1.Condition{
		Type:    "Complete",
		Status:  metav1.ConditionTrue,
		Reason:  "PowerRestored",
		Message: "Power restored event processed successfully",
	})
	if err := r.Status().Update(ctx, event); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *PowerEventReconciler) handlePowerFailure(ctx context.Context, event *popv1alpha1.PowerEvent) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Look up the PowerProtectionConfig singleton named "default".
	config := &popv1alpha1.PowerProtectionConfig{}
	err := r.Get(ctx, types.NamespacedName{Name: "default", Namespace: event.Namespace}, config)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.setFailed(ctx, event, "PowerProtectionConfig 'default' not found")
		}
		return ctrl.Result{}, err
	}

	if !config.Spec.Enabled {
		return r.setFailed(ctx, event, "Power protection is not enabled")
	}

	// Create an EtcdBackup CR immediately.
	backupName := fmt.Sprintf("power-backup-%s", event.Name)
	etcdBackup := &popv1alpha1.EtcdBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupName,
			Namespace: event.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "pop-operator",
				"pop.openshift.io/powerevent":  event.Name,
			},
		},
		Spec: popv1alpha1.EtcdBackupSpec{
			PVCName:              config.Spec.BackupConfig.PVCName,
			BackupTimeoutMinutes: 15,
			ExportTarget:         config.Spec.BackupConfig.ExportTarget,
		},
	}
	if err := ctrl.SetControllerReference(event, etcdBackup, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Create(ctx, etcdBackup); err != nil {
		if apierrors.IsAlreadyExists(err) {
			logger.Info("EtcdBackup already exists", "name", backupName)
		} else {
			return ctrl.Result{}, err
		}
	} else {
		logger.Info("created EtcdBackup", "name", backupName)
	}

	// Compute confirmation deadline.
	now := metav1.Now()
	confirmationSeconds := config.Spec.PowerConfirmationSeconds
	if confirmationSeconds == 0 {
		confirmationSeconds = 120
	}
	deadline := metav1.NewTime(now.Add(time.Duration(confirmationSeconds) * time.Second))

	event.Status.EtcdBackupRef = backupName
	event.Status.BackupStartedAt = &now
	event.Status.ConfirmationDeadline = &deadline
	event.Status.Phase = popv1alpha1.PowerEventPhaseBackingUp
	event.Status.Message = "Emergency etcd backup in progress"
	meta.SetStatusCondition(&event.Status.Conditions, metav1.Condition{
		Type:    "BackupStarted",
		Status:  metav1.ConditionTrue,
		Reason:  "EtcdBackupCreated",
		Message: fmt.Sprintf("Emergency backup %s created", backupName),
	})
	if err := r.Status().Update(ctx, event); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *PowerEventReconciler) handleTestEvent(ctx context.Context, event *popv1alpha1.PowerEvent) (ctrl.Result, error) {
	event.Status.Phase = popv1alpha1.PowerEventPhaseComplete
	event.Status.Message = "Test event processed"
	meta.SetStatusCondition(&event.Status.Conditions, metav1.Condition{
		Type:    "Complete",
		Status:  metav1.ConditionTrue,
		Reason:  "TestProcessed",
		Message: "Test event processed",
	})
	if err := r.Status().Update(ctx, event); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *PowerEventReconciler) reconcileBackingUp(ctx context.Context, event *popv1alpha1.PowerEvent) (ctrl.Result, error) {
	// Check for cancellation: look for a PowerRestored event newer than this one.
	if cancelled, err := r.checkForCancellation(ctx, event); err != nil {
		return ctrl.Result{}, err
	} else if cancelled {
		return r.setCancelled(ctx, event)
	}

	now := time.Now()
	deadlinePassed := event.Status.ConfirmationDeadline != nil && !now.Before(event.Status.ConfirmationDeadline.Time)

	// Fetch the EtcdBackup CR.
	if event.Status.EtcdBackupRef != "" {
		backup := &popv1alpha1.EtcdBackup{}
		err := r.Get(ctx, types.NamespacedName{Name: event.Status.EtcdBackupRef, Namespace: event.Namespace}, backup)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Backup was deleted — if deadline passed, move on; otherwise requeue.
				if deadlinePassed {
					return r.transitionToAwaitingConfirmation(ctx, event)
				}
				return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
			}
			return ctrl.Result{}, err
		}

		if backup.Status.Phase == popv1alpha1.EtcdBackupPhaseCompleted || deadlinePassed {
			return r.transitionToAwaitingConfirmation(ctx, event)
		}

		if backup.Status.Phase == popv1alpha1.EtcdBackupPhaseFailed && deadlinePassed {
			return r.transitionToAwaitingConfirmation(ctx, event)
		}
	}

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *PowerEventReconciler) transitionToAwaitingConfirmation(ctx context.Context, event *popv1alpha1.PowerEvent) (ctrl.Result, error) {
	event.Status.Phase = popv1alpha1.PowerEventPhaseAwaitingConfirmation
	event.Status.Message = "Awaiting power outage confirmation"
	meta.SetStatusCondition(&event.Status.Conditions, metav1.Condition{
		Type:    "AwaitingConfirmation",
		Status:  metav1.ConditionTrue,
		Reason:  "BackupPhaseComplete",
		Message: "Backup phase complete, awaiting confirmation deadline",
	})
	if err := r.Status().Update(ctx, event); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *PowerEventReconciler) reconcileAwaitingConfirmation(ctx context.Context, event *popv1alpha1.PowerEvent) (ctrl.Result, error) {
	// Check for cancellation.
	if cancelled, err := r.checkForCancellation(ctx, event); err != nil {
		return ctrl.Result{}, err
	} else if cancelled {
		return r.setCancelled(ctx, event)
	}

	now := time.Now()
	if event.Status.ConfirmationDeadline != nil && !now.Before(event.Status.ConfirmationDeadline.Time) {
		// Deadline passed — confirm the outage.
		confirmedAt := metav1.Now()
		event.Status.Phase = popv1alpha1.PowerEventPhaseConfirmed
		event.Status.ConfirmedAt = &confirmedAt
		event.Status.Message = "Power outage confirmed"
		meta.SetStatusCondition(&event.Status.Conditions, metav1.Condition{
			Type:    "Confirmed",
			Status:  metav1.ConditionTrue,
			Reason:  "ConfirmationDeadlinePassed",
			Message: "Power outage confirmed after confirmation period",
		})
		if err := r.Status().Update(ctx, event); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *PowerEventReconciler) reconcileConfirmed(ctx context.Context, event *popv1alpha1.PowerEvent) (ctrl.Result, error) {
	// Check for cancellation.
	if cancelled, err := r.checkForCancellation(ctx, event); err != nil {
		return ctrl.Result{}, err
	} else if cancelled {
		return r.setCancelled(ctx, event)
	}

	// Look up PowerProtectionConfig.
	config := &popv1alpha1.PowerProtectionConfig{}
	err := r.Get(ctx, types.NamespacedName{Name: "default", Namespace: event.Namespace}, config)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.setFailed(ctx, event, "PowerProtectionConfig 'default' not found during confirmed phase")
		}
		return ctrl.Result{}, err
	}

	if !config.Spec.AutonomousShutdown {
		event.Status.Phase = popv1alpha1.PowerEventPhaseComplete
		event.Status.Message = "Power outage confirmed. Backup complete. Manual shutdown required."
		meta.SetStatusCondition(&event.Status.Conditions, metav1.Condition{
			Type:    "Complete",
			Status:  metav1.ConditionTrue,
			Reason:  "ManualShutdownRequired",
			Message: "Autonomous shutdown not enabled; manual intervention required",
		})
		if err := r.Status().Update(ctx, event); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Compute priority delay.
	var delaySeconds int32
	if config.Spec.ShutdownDelayOverrideSeconds != nil {
		delaySeconds = *config.Spec.ShutdownDelayOverrideSeconds
	} else {
		delaySeconds = priorityDelaySeconds(config.Spec.ClusterPriority)
	}

	now := metav1.Now()
	shutdownDeadline := metav1.NewTime(now.Add(time.Duration(delaySeconds) * time.Second))
	event.Status.ShutdownDeadline = &shutdownDeadline

	if delaySeconds == 0 {
		// Immediately go to ShuttingDown.
		event.Status.Phase = popv1alpha1.PowerEventPhaseShuttingDown
		event.Status.Message = "Autonomous shutdown initiated immediately"
		meta.SetStatusCondition(&event.Status.Conditions, metav1.Condition{
			Type:    "ShuttingDown",
			Status:  metav1.ConditionTrue,
			Reason:  "ImmediateShutdown",
			Message: "BestEffort priority — shutting down immediately",
		})
		if err := r.Status().Update(ctx, event); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	event.Status.Phase = popv1alpha1.PowerEventPhaseShuttingDown
	event.Status.Message = fmt.Sprintf("Autonomous shutdown scheduled for %s", shutdownDeadline.Time.Format(time.RFC3339))
	meta.SetStatusCondition(&event.Status.Conditions, metav1.Condition{
		Type:    "ShuttingDown",
		Status:  metav1.ConditionTrue,
		Reason:  "ShutdownScheduled",
		Message: fmt.Sprintf("Shutdown scheduled with %ds delay for %s priority", delaySeconds, config.Spec.ClusterPriority),
	})
	if err := r.Status().Update(ctx, event); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: time.Duration(delaySeconds) * time.Second}, nil
}

func (r *PowerEventReconciler) reconcileShuttingDown(ctx context.Context, event *popv1alpha1.PowerEvent) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Wait for shutdown deadline if set and not yet reached.
	if event.Status.ShutdownDeadline != nil && time.Now().Before(event.Status.ShutdownDeadline.Time) {
		remaining := time.Until(event.Status.ShutdownDeadline.Time)
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	if event.Status.ClusterShutdownRef == "" {
		// Create a ClusterShutdown CR.
		config := &popv1alpha1.PowerProtectionConfig{}
		err := r.Get(ctx, types.NamespacedName{Name: "default", Namespace: event.Namespace}, config)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return r.setFailed(ctx, event, "PowerProtectionConfig 'default' not found during shutdown phase")
			}
			return ctrl.Result{}, err
		}

		shutdownName := fmt.Sprintf("power-shutdown-%s", event.Name)
		clusterShutdown := &popv1alpha1.ClusterShutdown{
			ObjectMeta: metav1.ObjectMeta{
				Name:      shutdownName,
				Namespace: event.Namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "pop-operator",
					"pop.openshift.io/powerevent":  event.Name,
				},
			},
			Spec: popv1alpha1.ClusterShutdownSpec{
				PVCName:              config.Spec.BackupConfig.PVCName,
				DryRun:               false,
				SkipCertCheck:        true,
				SkipPreflightChecks:  true,
				ShutdownDelayMinutes: config.Spec.BackupConfig.ShutdownDelayMinutes,
			},
		}
		if err := ctrl.SetControllerReference(event, clusterShutdown, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, clusterShutdown); err != nil {
			if apierrors.IsAlreadyExists(err) {
				logger.Info("ClusterShutdown already exists", "name", shutdownName)
			} else {
				return ctrl.Result{}, err
			}
		} else {
			logger.Info("created ClusterShutdown", "name", shutdownName)
		}

		now := metav1.Now()
		event.Status.ClusterShutdownRef = shutdownName
		event.Status.ShutdownStartedAt = &now
		event.Status.Message = fmt.Sprintf("Cluster shutdown %s initiated", shutdownName)
		if err := r.Status().Update(ctx, event); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Check the ClusterShutdown status.
	shutdown := &popv1alpha1.ClusterShutdown{}
	err := r.Get(ctx, types.NamespacedName{Name: event.Status.ClusterShutdownRef, Namespace: event.Namespace}, shutdown)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.setFailed(ctx, event, fmt.Sprintf("ClusterShutdown %s not found", event.Status.ClusterShutdownRef))
		}
		return ctrl.Result{}, err
	}

	switch shutdown.Status.Phase {
	case popv1alpha1.ClusterShutdownPhaseComplete:
		event.Status.Phase = popv1alpha1.PowerEventPhaseComplete
		event.Status.Message = "Cluster shutdown completed successfully"
		meta.SetStatusCondition(&event.Status.Conditions, metav1.Condition{
			Type:    "Complete",
			Status:  metav1.ConditionTrue,
			Reason:  "ShutdownComplete",
			Message: "Autonomous cluster shutdown completed",
		})
		if err := r.Status().Update(ctx, event); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil

	case popv1alpha1.ClusterShutdownPhaseFailed:
		return r.setFailed(ctx, event, fmt.Sprintf("Cluster shutdown failed: %s", shutdown.Status.Message))

	default:
		event.Status.Message = fmt.Sprintf("Waiting for cluster shutdown (phase: %s)", shutdown.Status.Phase)
		if err := r.Status().Update(ctx, event); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
}

// checkForCancellation lists PowerEvents with type PowerRestored that are newer
// than the given event. If any are found, it means power was restored.
func (r *PowerEventReconciler) checkForCancellation(ctx context.Context, event *popv1alpha1.PowerEvent) (bool, error) {
	eventList := &popv1alpha1.PowerEventList{}
	if err := r.List(ctx, eventList, client.InNamespace(event.Namespace)); err != nil {
		return false, err
	}

	for i := range eventList.Items {
		pe := &eventList.Items[i]
		if pe.Name == event.Name {
			continue
		}
		if pe.Spec.EventType != popv1alpha1.PowerEventTypeRestored {
			continue
		}
		if pe.CreationTimestamp.After(event.CreationTimestamp.Time) {
			return true, nil
		}
	}
	return false, nil
}

func (r *PowerEventReconciler) setCancelled(ctx context.Context, event *popv1alpha1.PowerEvent) (ctrl.Result, error) {
	now := metav1.Now()
	event.Status.Phase = popv1alpha1.PowerEventPhaseCancelled
	event.Status.CancelledAt = &now
	event.Status.Message = "Power restored. Shutdown cancelled. Backup retained."
	meta.SetStatusCondition(&event.Status.Conditions, metav1.Condition{
		Type:    "Cancelled",
		Status:  metav1.ConditionTrue,
		Reason:  "PowerRestored",
		Message: "Power restored. Shutdown cancelled. Backup retained.",
	})
	if err := r.Status().Update(ctx, event); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *PowerEventReconciler) setFailed(ctx context.Context, event *popv1alpha1.PowerEvent, message string) (ctrl.Result, error) {
	log.FromContext(ctx).Error(fmt.Errorf(message), "power event failed")
	event.Status.Phase = popv1alpha1.PowerEventPhaseFailed
	event.Status.Message = message
	meta.SetStatusCondition(&event.Status.Conditions, metav1.Condition{
		Type:    "Failed",
		Status:  metav1.ConditionTrue,
		Reason:  "PowerEventFailed",
		Message: message,
	})
	if err := r.Status().Update(ctx, event); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func isTerminalPhase(phase popv1alpha1.PowerEventPhase) bool {
	return phase == popv1alpha1.PowerEventPhaseComplete ||
		phase == popv1alpha1.PowerEventPhaseFailed ||
		phase == popv1alpha1.PowerEventPhaseCancelled
}

func priorityDelaySeconds(priority popv1alpha1.ClusterPriority) int32 {
	switch priority {
	case popv1alpha1.ClusterPriorityCritical:
		return 300 // 5 minutes
	case popv1alpha1.ClusterPriorityStandard:
		return 120 // 2 minutes
	case popv1alpha1.ClusterPriorityBestEffort:
		return 0
	default:
		return 120
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *PowerEventReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&popv1alpha1.PowerEvent{}).
		Owns(&popv1alpha1.EtcdBackup{}).
		Owns(&popv1alpha1.ClusterShutdown{}).
		Complete(r)
}
