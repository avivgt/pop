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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
)

// BackupScheduleReconciler reconciles a BackupSchedule object
type BackupScheduleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=pop.openshift.io,resources=backupschedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pop.openshift.io,resources=backupschedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pop.openshift.io,resources=backupschedules/finalizers,verbs=update
// +kubebuilder:rbac:groups=pop.openshift.io,resources=etcdbackups,verbs=get;list;watch;create;delete

func (r *BackupScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. Fetch the BackupSchedule CR.
	schedule := &popv1alpha1.BackupSchedule{}
	if err := r.Get(ctx, req.NamespacedName, schedule); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	now := time.Now()

	// 2. If suspended, set phase and requeue.
	if schedule.Spec.Suspend {
		schedule.Status.Phase = popv1alpha1.BackupSchedulePhaseSuspended

		// Still compute nextBackupTime for informational purposes.
		if err := r.computeAndSetNextBackupTime(schedule, now); err != nil {
			logger.Error(err, "failed to parse cron schedule")
		}

		if err := r.Status().Update(ctx, schedule); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// 3. Set phase to Active.
	schedule.Status.Phase = popv1alpha1.BackupSchedulePhaseActive

	// 4. Check active backup status.
	if schedule.Status.ActiveBackup != "" {
		activeBackup := &popv1alpha1.EtcdBackup{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      schedule.Status.ActiveBackup,
			Namespace: schedule.Namespace,
		}, activeBackup)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Active backup was deleted externally; clear it.
				logger.Info("active backup not found, clearing", "backup", schedule.Status.ActiveBackup)
				schedule.Status.ActiveBackup = ""
			} else {
				return ctrl.Result{}, err
			}
		} else {
			switch activeBackup.Status.Phase {
			case popv1alpha1.EtcdBackupPhaseCompleted:
				logger.Info("active backup completed", "backup", activeBackup.Name)
				schedule.Status.LastSuccessfulBackup = activeBackup.Name
				nowTime := metav1.NewTime(now)
				schedule.Status.LastBackupTime = &nowTime
				schedule.Status.ActiveBackup = ""
			case popv1alpha1.EtcdBackupPhaseFailed:
				logger.Info("active backup failed", "backup", activeBackup.Name)
				schedule.Status.FailedBackupsCount++
				nowTime := metav1.NewTime(now)
				schedule.Status.LastBackupTime = &nowTime
				schedule.Status.ActiveBackup = ""
			default:
				// Backup still in progress, requeue.
				if err := r.Status().Update(ctx, schedule); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
			}
		}
	}

	// 5. Resolve and parse the schedule.
	cronExpr, humanDesc := resolveSchedule(schedule)
	schedule.Status.ResolvedSchedule = cronExpr
	schedule.Status.HumanReadableSchedule = humanDesc

	if schedule.Spec.Frequency == popv1alpha1.BackupFrequencyNow && schedule.Status.LastBackupTime == nil {
		timestamp := now.Format("20060102-150405")
		backupName := fmt.Sprintf("%s-%s", schedule.Name, timestamp)
		etcdBackup := &popv1alpha1.EtcdBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      backupName,
				Namespace: schedule.Namespace,
				Labels:    map[string]string{"pop.openshift.io/schedule": schedule.Name},
			},
			Spec: popv1alpha1.EtcdBackupSpec{
				PVCName:              schedule.Spec.PVCName,
				BackupTimeoutMinutes: schedule.Spec.BackupTimeoutMinutes,
				ExportTarget:         schedule.Spec.ExportTarget,
			},
		}
		if err := ctrl.SetControllerReference(schedule, etcdBackup, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, etcdBackup); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, err
		}
		logger.Info("triggered immediate backup", "backup", backupName)
		schedule.Status.ActiveBackup = backupName
		nowTime := metav1.NewTime(now)
		schedule.Status.LastBackupTime = &nowTime
		if err := r.Status().Update(ctx, schedule); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	cronSchedule, err := cron.ParseStandard(cronExpr)
	if err != nil {
		logger.Error(err, "failed to parse resolved schedule", "schedule", cronExpr)
		if updateErr := r.Status().Update(ctx, schedule); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// 6. Compute nextBackupTime from lastBackupTime (or creation time if never run).
	referenceTime := schedule.CreationTimestamp.Time
	if schedule.Status.LastBackupTime != nil {
		referenceTime = schedule.Status.LastBackupTime.Time
	}
	nextBackupTime := cronSchedule.Next(referenceTime)

	// 7. If current time >= nextBackupTime, create a new EtcdBackup.
	if !now.Before(nextBackupTime) {
		timestamp := now.Format("20060102-150405")
		backupName := fmt.Sprintf("%s-%s", schedule.Name, timestamp)

		etcdBackup := &popv1alpha1.EtcdBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      backupName,
				Namespace: schedule.Namespace,
				Labels: map[string]string{
					"pop.openshift.io/schedule": schedule.Name,
				},
			},
			Spec: popv1alpha1.EtcdBackupSpec{
				PVCName:              schedule.Spec.PVCName,
				BackupTimeoutMinutes: schedule.Spec.BackupTimeoutMinutes,
				ExportTarget:         schedule.Spec.ExportTarget,
			},
		}

		if err := ctrl.SetControllerReference(schedule, etcdBackup, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, etcdBackup); err != nil {
			if apierrors.IsAlreadyExists(err) {
				logger.Info("backup already exists, requeueing", "backup", backupName)
				return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
			}
			return ctrl.Result{}, err
		}

		logger.Info("created scheduled backup", "backup", backupName)
		schedule.Status.ActiveBackup = backupName

		// Update nextBackupTime to the next run after now.
		nextRun := cronSchedule.Next(now)
		nextRunTime := metav1.NewTime(nextRun)
		schedule.Status.NextBackupTime = &nextRunTime
	} else {
		nextTime := metav1.NewTime(nextBackupTime)
		schedule.Status.NextBackupTime = &nextTime
	}

	// 8. Enforce retention.
	if err := r.enforceRetention(ctx, schedule, now); err != nil {
		logger.Error(err, "failed to enforce retention")
	}

	// Update status.
	if err := r.Status().Update(ctx, schedule); err != nil {
		return ctrl.Result{}, err
	}

	// 9. RequeueAfter: time until nextBackupTime.
	if schedule.Status.NextBackupTime != nil {
		delay := time.Until(schedule.Status.NextBackupTime.Time)
		if delay < 0 {
			delay = 1 * time.Second
		}
		return ctrl.Result{RequeueAfter: delay}, nil
	}

	return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
}

func (r *BackupScheduleReconciler) computeAndSetNextBackupTime(schedule *popv1alpha1.BackupSchedule, now time.Time) error {
	cronExpr, humanDesc := resolveSchedule(schedule)
	schedule.Status.ResolvedSchedule = cronExpr
	schedule.Status.HumanReadableSchedule = humanDesc

	cronSchedule, err := cron.ParseStandard(cronExpr)
	if err != nil {
		return err
	}

	referenceTime := schedule.CreationTimestamp.Time
	if schedule.Status.LastBackupTime != nil {
		referenceTime = schedule.Status.LastBackupTime.Time
	}

	nextBackupTime := cronSchedule.Next(referenceTime)
	if nextBackupTime.Before(now) {
		nextBackupTime = cronSchedule.Next(now)
	}

	nextTime := metav1.NewTime(nextBackupTime)
	schedule.Status.NextBackupTime = &nextTime
	return nil
}

var dayToCron = map[string]string{
	"Sunday": "0", "Monday": "1", "Tuesday": "2", "Wednesday": "3",
	"Thursday": "4", "Friday": "5", "Saturday": "6",
}

func resolveSchedule(schedule *popv1alpha1.BackupSchedule) (cronExpr string, humanReadable string) {
	if schedule.Spec.Schedule != "" {
		return schedule.Spec.Schedule, fmt.Sprintf("Custom cron: %s", schedule.Spec.Schedule)
	}

	t := schedule.Spec.Time
	if t == "" {
		t = "02:00"
	}
	hour := t[:2]
	minute := t[3:5]

	switch schedule.Spec.Frequency {
	case popv1alpha1.BackupFrequencyNow:
		return fmt.Sprintf("%s %s * * *", minute, hour), "Immediate (one-time)"
	case popv1alpha1.BackupFrequencyEvery4Hours:
		return "0 */4 * * *", "Every 4 hours"
	case popv1alpha1.BackupFrequencyEvery8Hours:
		return "0 */8 * * *", "Every 8 hours"
	case popv1alpha1.BackupFrequencyEvery12Hours:
		return "0 */12 * * *", "Every 12 hours"
	case popv1alpha1.BackupFrequencyDaily:
		return fmt.Sprintf("%s %s * * *", minute, hour), fmt.Sprintf("Daily at %s UTC", t)
	case popv1alpha1.BackupFrequencyWeekly:
		day := schedule.Spec.DayOfWeek
		if day == "" {
			day = "Sunday"
		}
		cronDay := dayToCron[day]
		return fmt.Sprintf("%s %s * * %s", minute, hour, cronDay), fmt.Sprintf("Weekly on %s at %s UTC", day, t)
	case popv1alpha1.BackupFrequencyMonthly:
		dom := int32(1)
		if schedule.Spec.DayOfMonth != nil {
			dom = *schedule.Spec.DayOfMonth
		}
		return fmt.Sprintf("%s %s %d * *", minute, hour, dom), fmt.Sprintf("Monthly on day %d at %s UTC", dom, t)
	case popv1alpha1.BackupFrequencyCustom:
		if schedule.Spec.CustomInterval != nil {
			ci := schedule.Spec.CustomInterval
			switch ci.Unit {
			case "Minutes":
				return fmt.Sprintf("*/%d * * * *", ci.Every), fmt.Sprintf("Every %d minutes", ci.Every)
			case "Hours":
				return fmt.Sprintf("0 */%d * * *", ci.Every), fmt.Sprintf("Every %d hours", ci.Every)
			case "Days":
				return fmt.Sprintf("%s %s */%d * *", minute, hour, ci.Every), fmt.Sprintf("Every %d days at %s UTC", ci.Every, t)
			}
		}
		return "0 2 * * *", "Custom (invalid, defaulting to daily at 02:00)"
	default:
		return fmt.Sprintf("%s %s * * *", minute, hour), fmt.Sprintf("Daily at %s UTC", t)
	}
}

func (r *BackupScheduleReconciler) enforceRetention(ctx context.Context, schedule *popv1alpha1.BackupSchedule, now time.Time) error {
	logger := log.FromContext(ctx)

	// List all EtcdBackup CRs owned by this BackupSchedule using label selector.
	backupList := &popv1alpha1.EtcdBackupList{}
	if err := r.List(ctx, backupList,
		client.InNamespace(schedule.Namespace),
		client.MatchingLabels{"pop.openshift.io/schedule": schedule.Name},
	); err != nil {
		return err
	}

	// Filter to only Completed backups.
	var completed []popv1alpha1.EtcdBackup
	for _, b := range backupList.Items {
		if b.Status.Phase == popv1alpha1.EtcdBackupPhaseCompleted {
			completed = append(completed, b)
		}
	}

	// Sort by creation time (oldest first).
	sort.Slice(completed, func(i, j int) bool {
		return completed[i].CreationTimestamp.Time.Before(completed[j].CreationTimestamp.Time)
	})

	retentionCount := schedule.Spec.RetentionCount
	if retentionCount == 0 {
		retentionCount = 10
	}
	retentionDays := schedule.Spec.RetentionDays
	if retentionDays == 0 {
		retentionDays = 30
	}
	cutoffTime := now.AddDate(0, 0, -int(retentionDays))

	// Delete any beyond retentionCount (oldest are at the front).
	if int32(len(completed)) > retentionCount {
		excessCount := int32(len(completed)) - retentionCount
		for i := int32(0); i < excessCount; i++ {
			backup := &completed[i]
			logger.Info("deleting backup exceeding retention count", "backup", backup.Name)
			if err := r.Delete(ctx, backup); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	// Delete any older than retentionDays.
	for i := range completed {
		backup := &completed[i]
		if backup.CreationTimestamp.Time.Before(cutoffTime) {
			logger.Info("deleting backup exceeding retention days", "backup", backup.Name)
			if err := r.Delete(ctx, backup); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&popv1alpha1.BackupSchedule{}).
		Owns(&popv1alpha1.EtcdBackup{}).
		Complete(r)
}
