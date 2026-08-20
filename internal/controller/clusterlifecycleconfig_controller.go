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

	"github.com/robfig/cron/v3"
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
	"github.com/avivgt/pop/internal/version"
)

type ClusterLifecycleConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=pop.openshift.io,resources=clusterlifecycleconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pop.openshift.io,resources=clusterlifecycleconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pop.openshift.io,resources=clusterlifecycleconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups=pop.openshift.io,resources=etcdbackups,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=pop.openshift.io,resources=powerevents,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=clusterclaims,verbs=get;list;watch;create;update;patch;delete

func (r *ClusterLifecycleConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	config := &popv1alpha1.ClusterLifecycleConfig{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	config.Status.ObservedGeneration = config.Generation

	now := time.Now()
	requeueAfter := 1 * time.Minute

	r.reconcileClusterStatus(ctx, config)

	if backupRequeue, err := r.reconcileBackupSchedule(ctx, config, now); err != nil {
		logger.Error(err, "backup scheduling error")
	} else if backupRequeue > 0 && backupRequeue < requeueAfter {
		requeueAfter = backupRequeue
	}

	if config.Spec.Backup.Schedule != nil && config.Spec.Backup.Schedule.Enabled {
		meta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
			Type:    "BackupScheduleReady",
			Status:  metav1.ConditionTrue,
			Reason:  "ScheduleActive",
			Message: config.Status.Backup.ResolvedSchedule,
		})
	}

	if config.Status.Cluster.CertificateRemainingDays > 30 {
		meta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
			Type:    "ClusterHealthy",
			Status:  metav1.ConditionTrue,
			Reason:  "CertificateValid",
			Message: fmt.Sprintf("Certificate expires in %d days", config.Status.Cluster.CertificateRemainingDays),
		})
	} else if config.Status.Cluster.CertificateRemainingDays > 0 {
		meta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
			Type:    "ClusterHealthy",
			Status:  metav1.ConditionFalse,
			Reason:  "CertificateExpiringSoon",
			Message: fmt.Sprintf("Certificate expires in %d days", config.Status.Cluster.CertificateRemainingDays),
		})
	}

	if err := r.Status().Update(ctx, config); err != nil {
		return ctrl.Result{}, err
	}

	lastBackup := ""
	if config.Status.Backup.LastBackupTime != nil {
		lastBackup = config.Status.Backup.LastBackupTime.Format(time.RFC3339)
	}
	if err := ensureClusterClaim(ctx, r.Client, ClaimLastBackupTime, lastBackup); err != nil {
		logger.V(1).Error(err, "failed to update last-backup-time ClusterClaim")
	}
	if err := ensureClusterClaim(ctx, r.Client, ClaimOperatorVersion, version.Version); err != nil {
		logger.V(1).Error(err, "failed to update operator-version ClusterClaim")
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *ClusterLifecycleConfigReconciler) reconcileClusterStatus(ctx context.Context, config *popv1alpha1.ClusterLifecycleConfig) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err == nil {
		cpCount := int32(0)
		for _, node := range nodeList.Items {
			if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
				cpCount++
			} else if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
				cpCount++
			}
		}
		config.Status.Cluster.NodeCount = int32(len(nodeList.Items))
		config.Status.Cluster.ControlPlaneNodes = cpCount
		config.Status.Cluster.WorkerNodes = int32(len(nodeList.Items)) - cpCount
	}

	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name: "kube-apiserver-to-kubelet-signer", Namespace: "openshift-kube-apiserver-operator",
	}, secret)
	if err == nil {
		if expiryStr, ok := secret.Annotations["auth.openshift.io/certificate-not-after"]; ok {
			if expiry, err := time.Parse(time.RFC3339, expiryStr); err == nil {
				t := metav1.NewTime(expiry)
				config.Status.Cluster.CertificateExpiry = &t
				config.Status.Cluster.CertificateRemainingDays = int32(time.Until(expiry).Hours() / 24)
				popmetrics.CertificateExpiryTimestamp.Set(float64(expiry.Unix()))
			}
		}
	}
}

func (r *ClusterLifecycleConfigReconciler) reconcileBackupSchedule(ctx context.Context, config *popv1alpha1.ClusterLifecycleConfig, now time.Time) (time.Duration, error) {
	logger := log.FromContext(ctx)

	if config.Spec.Backup.Schedule == nil || !config.Spec.Backup.Schedule.Enabled {
		config.Status.Backup.ResolvedSchedule = "Disabled"
		return 0, nil
	}

	cronExpr, humanDesc := resolveFrequencyConfig(config.Spec.Backup.Schedule)
	config.Status.Backup.ResolvedSchedule = humanDesc

	if config.Status.Backup.ActiveBackup != "" {
		activeBackup := &popv1alpha1.EtcdBackup{}
		err := r.Get(ctx, types.NamespacedName{Name: config.Status.Backup.ActiveBackup, Namespace: config.Namespace}, activeBackup)
		if err != nil {
			if apierrors.IsNotFound(err) {
				config.Status.Backup.ActiveBackup = ""
			} else {
				return 0, err
			}
		} else {
			switch activeBackup.Status.Phase {
			case popv1alpha1.EtcdBackupPhaseCompleted:
				config.Status.Backup.LastSuccessfulBackup = activeBackup.Name
				nowTime := metav1.NewTime(now)
				config.Status.Backup.LastBackupTime = &nowTime
				config.Status.Backup.ActiveBackup = ""
			case popv1alpha1.EtcdBackupPhaseFailed:
				config.Status.Backup.FailedBackups++
				nowTime := metav1.NewTime(now)
				config.Status.Backup.LastBackupTime = &nowTime
				config.Status.Backup.ActiveBackup = ""
			default:
				return 30 * time.Second, nil
			}
		}
	}

	cronSchedule, err := cron.ParseStandard(cronExpr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse schedule %q: %w", cronExpr, err)
	}

	refTime := config.CreationTimestamp.Time
	if config.Status.Backup.LastBackupTime != nil {
		refTime = config.Status.Backup.LastBackupTime.Time
	}
	nextBackupTime := cronSchedule.Next(refTime)

	if !now.Before(nextBackupTime) {
		timestamp := now.Format("20060102-150405")
		backupName := fmt.Sprintf("scheduled-%s", timestamp)
		etcdBackup := &popv1alpha1.EtcdBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      backupName,
				Namespace: config.Namespace,
				Labels:    map[string]string{"pop.openshift.io/managed-by": "clusterlifecycleconfig"},
			},
			Spec: popv1alpha1.EtcdBackupSpec{
				PVCName:              config.Spec.Backup.Storage.PVCName,
				BackupTimeoutMinutes: config.Spec.Backup.TimeoutMinutes,
				ExportTarget:         config.Spec.Backup.Storage.ExportTarget,
			},
		}
		if err := ctrl.SetControllerReference(config, etcdBackup, r.Scheme); err != nil {
			return 0, err
		}
		if err := r.Create(ctx, etcdBackup); err != nil && !apierrors.IsAlreadyExists(err) {
			return 0, err
		}
		logger.Info("created scheduled backup", "backup", backupName)
		config.Status.Backup.ActiveBackup = backupName
		nextBackupTime = cronSchedule.Next(now)
	}

	nextTime := metav1.NewTime(nextBackupTime)
	config.Status.Backup.NextBackupTime = &nextTime

	backupList := &popv1alpha1.EtcdBackupList{}
	if err := r.List(ctx, backupList, client.InNamespace(config.Namespace),
		client.MatchingLabels{"pop.openshift.io/managed-by": "clusterlifecycleconfig"}); err == nil {
		config.Status.Backup.TotalBackups = int32(len(backupList.Items))
		r.enforceBackupRetention(ctx, config, backupList, now)
	}

	delay := time.Until(nextBackupTime)
	if delay < 0 {
		delay = 1 * time.Second
	}
	return delay, nil
}

func (r *ClusterLifecycleConfigReconciler) enforceBackupRetention(ctx context.Context, config *popv1alpha1.ClusterLifecycleConfig, backupList *popv1alpha1.EtcdBackupList, now time.Time) {
	logger := log.FromContext(ctx)
	retCount := int32(10)
	retDays := int32(30)
	if config.Spec.Backup.Retention != nil {
		if config.Spec.Backup.Retention.Count > 0 {
			retCount = config.Spec.Backup.Retention.Count
		}
		if config.Spec.Backup.Retention.Days > 0 {
			retDays = config.Spec.Backup.Retention.Days
		}
	}

	var completed []popv1alpha1.EtcdBackup
	for _, b := range backupList.Items {
		if b.Status.Phase == popv1alpha1.EtcdBackupPhaseCompleted {
			completed = append(completed, b)
		}
	}
	sort.Slice(completed, func(i, j int) bool {
		return completed[i].CreationTimestamp.Before(&completed[j].CreationTimestamp)
	})

	cutoff := now.AddDate(0, 0, -int(retDays))
	if int32(len(completed)) > retCount {
		for i := int32(0); i < int32(len(completed))-retCount; i++ {
			logger.Info("deleting backup exceeding retention count", "backup", completed[i].Name)
			_ = r.Delete(ctx, &completed[i])
		}
	}
	for i := range completed {
		if completed[i].CreationTimestamp.Time.Before(cutoff) {
			logger.Info("deleting backup exceeding retention age", "backup", completed[i].Name)
			_ = r.Delete(ctx, &completed[i])
		}
	}
}

func resolveFrequencyConfig(sched *popv1alpha1.BackupScheduleConfig) (string, string) {
	t := sched.Time
	if t == "" {
		t = "02:00"
	}
	hour := t[:2]
	minute := t[3:5]

	switch sched.Frequency {
	case popv1alpha1.BackupFrequencyEvery4Hours:
		return "0 */4 * * *", "Every 4 hours"
	case popv1alpha1.BackupFrequencyEvery8Hours:
		return "0 */8 * * *", "Every 8 hours"
	case popv1alpha1.BackupFrequencyEvery12Hours:
		return "0 */12 * * *", "Every 12 hours"
	case popv1alpha1.BackupFrequencyDaily:
		return fmt.Sprintf("%s %s * * *", minute, hour), fmt.Sprintf("Daily at %s UTC", t)
	case popv1alpha1.BackupFrequencyWeekly:
		day := sched.DayOfWeek
		if day == "" {
			day = "Sunday"
		}
		cronDay := dayToCron[day]
		return fmt.Sprintf("%s %s * * %s", minute, hour, cronDay), fmt.Sprintf("Weekly on %s at %s UTC", day, t)
	case popv1alpha1.BackupFrequencyMonthly:
		dom := int32(1)
		if sched.DayOfMonth != nil {
			dom = *sched.DayOfMonth
		}
		return fmt.Sprintf("%s %s %d * *", minute, hour, dom), fmt.Sprintf("Monthly on day %d at %s UTC", dom, t)
	case popv1alpha1.BackupFrequencyCustom:
		if sched.CustomInterval != nil {
			ci := sched.CustomInterval
			switch ci.Unit {
			case "Minutes":
				return fmt.Sprintf("*/%d * * * *", ci.Every), fmt.Sprintf("Every %d minutes", ci.Every)
			case "Hours":
				return fmt.Sprintf("0 */%d * * *", ci.Every), fmt.Sprintf("Every %d hours", ci.Every)
			case "Days":
				return fmt.Sprintf("%s %s */%d * *", minute, hour, ci.Every), fmt.Sprintf("Every %d days at %s UTC", ci.Every, t)
			}
		}
		return "0 2 * * *", "Custom (invalid config, defaulting to daily)"
	default:
		return fmt.Sprintf("%s %s * * *", minute, hour), fmt.Sprintf("Daily at %s UTC", t)
	}
}

func (r *ClusterLifecycleConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&popv1alpha1.ClusterLifecycleConfig{}).
		Owns(&popv1alpha1.EtcdBackup{}).
		Complete(r)
}
