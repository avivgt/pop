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
	"sort"
	"time"

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
	"github.com/avivgt/pop/internal/eventsource"
	popmetrics "github.com/avivgt/pop/internal/metrics"
)

type AutomatedProceduresConfigReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	SourceManager *eventsource.Manager
}

// +kubebuilder:rbac:groups=pop.openshift.io,resources=automatedproceduresconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pop.openshift.io,resources=automatedproceduresconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pop.openshift.io,resources=automatedproceduresconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups=pop.openshift.io,resources=powerevents,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch

func (r *AutomatedProceduresConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	config := &popv1alpha1.AutomatedProceduresConfig{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	config.Status.ObservedGeneration = config.Generation

	if !config.Spec.Enabled {
		config.Status.AlertEndpointReady = false
		config.Status.PowerStatus = popv1alpha1.PowerStatusUnknown
		meta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "Disabled",
			Message: "Automated procedures disabled",
		})
		if err := r.Status().Update(ctx, config); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, types.NamespacedName{Name: config.Spec.BackupConfig.PVCName, Namespace: config.Namespace}, pvc); err != nil {
		if apierrors.IsNotFound(err) {
			meta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionFalse,
				Reason:  "PVCNotFound",
				Message: fmt.Sprintf("Backup PVC %q not found", config.Spec.BackupConfig.PVCName),
			})
			if err := r.Status().Update(ctx, config); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	if config.Spec.AlertEndpoint != nil && config.Spec.AlertEndpoint.AuthTokenSecret != "" {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: config.Spec.AlertEndpoint.AuthTokenSecret, Namespace: config.Namespace}, secret); err != nil {
			if apierrors.IsNotFound(err) {
				meta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
					Type:    "Ready",
					Status:  metav1.ConditionFalse,
					Reason:  "AuthSecretNotFound",
					Message: fmt.Sprintf("Auth token Secret %q not found", config.Spec.AlertEndpoint.AuthTokenSecret),
				})
				if err := r.Status().Update(ctx, config); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			return ctrl.Result{}, err
		}
	}

	powerStatus := popv1alpha1.PowerStatusNormal
	var activeEventRef string
	var lastEventTime *metav1.Time

	eventList := &popv1alpha1.PowerEventList{}
	if err := r.List(ctx, eventList, client.InNamespace(config.Namespace)); err != nil {
		logger.Error(err, "failed to list PowerEvents")
	} else if len(eventList.Items) > 0 {
		sort.Slice(eventList.Items, func(i, j int) bool {
			return eventList.Items[i].CreationTimestamp.After(eventList.Items[j].CreationTimestamp.Time)
		})
		latest := &eventList.Items[0]
		t := latest.CreationTimestamp
		lastEventTime = &t

		if latest.Spec.EventType == popv1alpha1.PowerEventTypeFailure {
			switch latest.Status.Phase {
			case popv1alpha1.PowerEventPhaseDetected,
				popv1alpha1.PowerEventPhaseBackingUp,
				popv1alpha1.PowerEventPhaseAwaitingConfirmation,
				popv1alpha1.PowerEventPhaseConfirmed,
				popv1alpha1.PowerEventPhaseShuttingDown:
				powerStatus = popv1alpha1.PowerStatusOnBattery
				activeEventRef = latest.Name
			}
		}
	}

	config.Status.PowerStatus = powerStatus
	config.Status.ActiveEventRef = activeEventRef
	config.Status.LastPowerEvent = lastEventTime
	config.Status.AlertEndpointReady = true

	if powerStatus == popv1alpha1.PowerStatusOnBattery {
		popmetrics.PowerStatus.Set(1)
	} else {
		popmetrics.PowerStatus.Set(0)
	}

	if r.SourceManager != nil {
		r.SourceManager.Reconcile(config.Spec.EventSources)
		config.Status.EventSources = r.SourceManager.Statuses()
	}

	meta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "Validated",
		Message: fmt.Sprintf("Automated procedures active, priority: %s, power: %s", config.Spec.ClusterPriority, powerStatus),
	})

	if err := r.Status().Update(ctx, config); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *AutomatedProceduresConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&popv1alpha1.AutomatedProceduresConfig{}).
		Complete(r)
}
