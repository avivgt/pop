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
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	popv1alpha1 "github.com/avivgt/pop/api/v1alpha1"
	popmetrics "github.com/avivgt/pop/internal/metrics"
)

type EtcdBackupReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Clientset kubernetes.Interface
}

// +kubebuilder:rbac:groups=pop.openshift.io,resources=etcdbackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pop.openshift.io,resources=etcdbackups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pop.openshift.io,resources=etcdbackups/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *EtcdBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	backup := &popv1alpha1.EtcdBackup{}
	if err := r.Get(ctx, req.NamespacedName, backup); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if backup.Status.Phase == popv1alpha1.EtcdBackupPhaseCompleted ||
		backup.Status.Phase == popv1alpha1.EtcdBackupPhaseFailed {
		return ctrl.Result{}, nil
	}

	if backup.Status.Phase == "" {
		backup.Status.Phase = popv1alpha1.EtcdBackupPhasePending
		now := metav1.Now()
		backup.Status.StartTime = &now
		if err := r.Status().Update(ctx, backup); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	switch backup.Status.Phase {
	case popv1alpha1.EtcdBackupPhasePending:
		return r.reconcilePending(ctx, backup)
	case popv1alpha1.EtcdBackupPhaseValidating:
		return r.reconcileValidating(ctx, backup)
	case popv1alpha1.EtcdBackupPhaseRunning:
		return r.reconcileRunning(ctx, backup)
	default:
		logger.Info("unknown phase", "phase", backup.Status.Phase)
		return ctrl.Result{}, nil
	}
}

func (r *EtcdBackupReconciler) reconcilePending(ctx context.Context, backup *popv1alpha1.EtcdBackup) (ctrl.Result, error) {
	backup.Status.Phase = popv1alpha1.EtcdBackupPhaseValidating
	backup.Status.Message = "Validating prerequisites"
	if err := r.Status().Update(ctx, backup); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

func (r *EtcdBackupReconciler) reconcileValidating(ctx context.Context, backup *popv1alpha1.EtcdBackup) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, types.NamespacedName{Name: backup.Spec.PVCName, Namespace: backup.Namespace}, pvc); err != nil {
		if apierrors.IsNotFound(err) {
			return r.setFailed(ctx, backup, fmt.Sprintf("PVC %q not found in namespace %q", backup.Spec.PVCName, backup.Namespace))
		}
		return ctrl.Result{}, err
	}

	readyNodes, totalNodes, err := r.listControlPlaneNodes(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(readyNodes) == 0 {
		return r.setFailed(ctx, backup, fmt.Sprintf("no ready control plane nodes (total: %d)", totalNodes))
	}

	if len(readyNodes) < totalNodes {
		logger.Info("degraded control plane", "ready", len(readyNodes), "total", totalNodes)
		meta.SetStatusCondition(&backup.Status.Conditions, metav1.Condition{
			Type:    "ControlPlaneHealth",
			Status:  metav1.ConditionFalse,
			Reason:  "DegradedControlPlane",
			Message: fmt.Sprintf("%d of %d control plane nodes ready", len(readyNodes), totalNodes),
		})
	} else {
		meta.SetStatusCondition(&backup.Status.Conditions, metav1.Condition{
			Type:    "ControlPlaneHealth",
			Status:  metav1.ConditionTrue,
			Reason:  "AllNodesReady",
			Message: fmt.Sprintf("%d of %d control plane nodes ready", len(readyNodes), totalNodes),
		})
	}

	selectedNode := backup.Spec.ControlPlaneNode
	if selectedNode == "" {
		selectedNode = readyNodes[0].Name
		logger.Info("auto-selected control plane node", "node", selectedNode)
	} else {
		found := false
		for _, n := range readyNodes {
			if n.Name == selectedNode {
				found = true
				break
			}
		}
		if !found {
			return r.setFailed(ctx, backup, fmt.Sprintf("specified node %q is not a ready control plane node", selectedNode))
		}
	}

	backup.Status.BackupNode = selectedNode
	backup.Status.Phase = popv1alpha1.EtcdBackupPhaseRunning
	backup.Status.Message = fmt.Sprintf("Creating backup job on node %s", selectedNode)
	meta.SetStatusCondition(&backup.Status.Conditions, metav1.Condition{
		Type:    "Validated",
		Status:  metav1.ConditionTrue,
		Reason:  "PrerequisitesMet",
		Message: fmt.Sprintf("PVC %q exists, backup node %s selected", backup.Spec.PVCName, selectedNode),
	})
	if err := r.Status().Update(ctx, backup); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

func (r *EtcdBackupReconciler) reconcileRunning(ctx context.Context, backup *popv1alpha1.EtcdBackup) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	jobName := fmt.Sprintf("etcd-backup-%s", backup.Name)

	existingJob := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: backup.Namespace}, existingJob)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		job := r.buildBackupJob(backup, jobName)
		if err := ctrl.SetControllerReference(backup, job, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, job); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
			return ctrl.Result{}, err
		}

		logger.Info("created backup job", "job", jobName)
		backup.Status.JobName = jobName
		backup.Status.Message = "Backup job running"
		if err := r.Status().Update(ctx, backup); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if existingJob.Status.Succeeded > 0 {
		logger.Info("backup job completed successfully")
		now := metav1.Now()
		backup.Status.Phase = popv1alpha1.EtcdBackupPhaseCompleted
		backup.Status.CompletionTime = &now
		backup.Status.Message = "Backup completed successfully"
		backup.Status.BackupPath = fmt.Sprintf("etcd-backup-%s", backup.Name)
		popmetrics.EtcdBackupTotal.WithLabelValues("success").Inc()
		popmetrics.EtcdBackupLastSuccess.SetToCurrentTime()
		if backup.Status.StartTime != nil {
			popmetrics.EtcdBackupDuration.Observe(time.Since(backup.Status.StartTime.Time).Seconds())
		}
		r.parseBackupMetadata(ctx, backup)
		meta.SetStatusCondition(&backup.Status.Conditions, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "BackupCompleted",
			Message: "etcd backup completed and exported to PVC",
		})
		if err := r.Status().Update(ctx, backup); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	for _, condition := range existingJob.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			return r.setFailed(ctx, backup, fmt.Sprintf("backup job failed: %s", condition.Message))
		}
	}

	if existingJob.Status.Active > 0 {
		backup.Status.Message = "Backup job in progress"
		if err := r.Status().Update(ctx, backup); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *EtcdBackupReconciler) buildBackupJob(backup *popv1alpha1.EtcdBackup, jobName string) *batchv1.Job {
	privileged := true
	runAsRoot := int64(0)
	hostPathType := corev1.HostPathDirectory
	backoffLimit := int32(2)
	timeout := int64(backup.Spec.BackupTimeoutMinutes) * 60

	retentionCount := backup.Spec.RetentionCount
	if retentionCount <= 0 {
		retentionCount = 5
	}

	backupScript := `set -euo pipefail
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
BACKUP_DIR="/backup/${TIMESTAMP}"

echo "Enforcing PVC retention before backup (keeping last ${MAX_BACKUPS} backups)..."
BACKUP_COUNT=$(ls -1d /backup/2* 2>/dev/null | wc -l)
if [ "${BACKUP_COUNT}" -ge "${MAX_BACKUPS}" ]; then
  REMOVE_COUNT=$((BACKUP_COUNT - MAX_BACKUPS + 1))
  ls -1d /backup/2* | sort | head -n "${REMOVE_COUNT}" | while read OLD_DIR; do
    echo "Removing old backup: ${OLD_DIR}"
    rm -rf "${OLD_DIR}"
  done
fi

PVC_USE_PCT=$(df /backup | tail -1 | awk '{gsub(/%/,""); print $5}')
while [ "${PVC_USE_PCT}" -ge 80 ]; do
  OLDEST=$(ls -1d /backup/2* 2>/dev/null | sort | head -n1)
  if [ -z "${OLDEST}" ]; then
    echo "WARNING: PVC at ${PVC_USE_PCT}% but no old backups to remove"
    break
  fi
  echo "PVC at ${PVC_USE_PCT}%, removing ${OLDEST} to free space"
  rm -rf "${OLDEST}"
  PVC_USE_PCT=$(df /backup | tail -1 | awk '{gsub(/%/,""); print $5}')
done

echo "PVC usage before backup:"
df -h /backup

mkdir -p "${BACKUP_DIR}"

echo "Starting etcd backup on $(hostname) at ${TIMESTAMP}"
chroot /host /usr/local/bin/cluster-backup.sh /home/core/assets/backup

echo "Copying backup artifacts to PVC..."
cp /host/home/core/assets/backup/snapshot_*.db "${BACKUP_DIR}/" 2>/dev/null || true
cp /host/home/core/assets/backup/static_kuberesources_*.tar.gz "${BACKUP_DIR}/" 2>/dev/null || true

echo "Cleaning up node-local backup files..."
rm -rf /host/home/core/assets/backup/snapshot_*.db
rm -rf /host/home/core/assets/backup/static_kuberesources_*.tar.gz

echo "Backup contents:"
ls -la "${BACKUP_DIR}/"

FILE_COUNT=$(ls -1 "${BACKUP_DIR}/" | wc -l)
if [ "${FILE_COUNT}" -lt 2 ]; then
  echo "ERROR: Expected at least 2 backup files, found ${FILE_COUNT}"
  exit 1
fi

BACKUP_SIZE=$(du -sb "${BACKUP_DIR}" | cut -f1)
PVC_USED=$(df -B1 /backup | tail -1 | awk '{print $3}')
PVC_CAPACITY=$(df -B1 /backup | tail -1 | awk '{print $2}')
echo "BACKUP_META:${BACKUP_SIZE}:${PVC_USED}:${PVC_CAPACITY}"

echo "Backup completed successfully to ${BACKUP_DIR}"
`

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: backup.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "pop-etcd-backup",
				"app.kubernetes.io/managed-by": "pop-operator",
				"pop.openshift.io/backup":      backup.Name,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			ActiveDeadlineSeconds:   &timeout,
			TTLSecondsAfterFinished: ptr(int32(3600)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":       "pop-etcd-backup",
						"app.kubernetes.io/managed-by": "pop-operator",
						"pop.openshift.io/backup":      backup.Name,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "pop-controller-manager",
					RestartPolicy:      corev1.RestartPolicyNever,
					HostNetwork:        true,
					HostPID:            true,
					NodeSelector: map[string]string{
						"kubernetes.io/hostname": backup.Status.BackupNode,
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
							Name:    "backup",
							Image:   "registry.access.redhat.com/ubi9/ubi-minimal:latest",
							Command: []string{"/bin/bash", "-c"},
							Args:    []string{backupScript},
							Env: []corev1.EnvVar{
								{
									Name:  "MAX_BACKUPS",
									Value: fmt.Sprintf("%d", retentionCount),
								},
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
								{
									Name:      "backup-storage",
									MountPath: "/backup",
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
						{
							Name: "backup-storage",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: backup.Spec.PVCName,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *EtcdBackupReconciler) listControlPlaneNodes(ctx context.Context) (ready []corev1.Node, total int, err error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return nil, 0, err
	}

	for _, node := range nodeList.Items {
		isCP := false
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			isCP = true
		}
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			isCP = true
		}
		if !isCP {
			continue
		}
		total++
		if isNodeReady(&node) {
			ready = append(ready, node)
		}
	}
	return ready, total, nil
}


func (r *EtcdBackupReconciler) parseBackupMetadata(ctx context.Context, backup *popv1alpha1.EtcdBackup) {
	logger := log.FromContext(ctx)
	if r.Clientset == nil {
		return
	}

	podList := &corev1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(backup.Namespace),
		client.MatchingLabels{"pop.openshift.io/backup": backup.Name},
	); err != nil {
		logger.Error(err, "failed to list backup pods for metadata")
		return
	}

	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase != corev1.PodSucceeded {
			continue
		}
		logStream, err := r.Clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{}).Stream(ctx)
		if err != nil {
			logger.Error(err, "failed to read backup pod logs")
			return
		}
		defer logStream.Close()

		scanner := bufio.NewScanner(logStream)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "BACKUP_META:") {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(line, "BACKUP_META:"), ":")
			if len(parts) != 3 {
				continue
			}
			backupSize, _ := strconv.ParseInt(parts[0], 10, 64)
			pvcUsed, _ := strconv.ParseInt(parts[1], 10, 64)
			pvcCapacity, _ := strconv.ParseInt(parts[2], 10, 64)

			backup.Status.BackupSizeBytes = backupSize
			backup.Status.BackupSize = formatBytes(backupSize)
			backup.Status.PVCUsedBytes = pvcUsed
			backup.Status.PVCCapacityBytes = pvcCapacity
			if pvcCapacity > 0 {
				backup.Status.PVCUsagePercent = int32(pvcUsed * 100 / pvcCapacity)
			}
			logger.Info("parsed backup metadata",
				"backupSize", backup.Status.BackupSize,
				"pvcUsage", fmt.Sprintf("%d%%", backup.Status.PVCUsagePercent))
			return
		}
	}
}

func formatBytes(bytes int64) string {
	const (
		gi = 1024 * 1024 * 1024
		mi = 1024 * 1024
	)
	switch {
	case bytes >= gi:
		return fmt.Sprintf("%.1fGi", float64(bytes)/float64(gi))
	case bytes >= mi:
		return fmt.Sprintf("%.0fMi", float64(bytes)/float64(mi))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func (r *EtcdBackupReconciler) setFailed(ctx context.Context, backup *popv1alpha1.EtcdBackup, message string) (ctrl.Result, error) {
	log.FromContext(ctx).Error(fmt.Errorf(message), "backup failed")
	popmetrics.EtcdBackupTotal.WithLabelValues("failure").Inc()
	now := metav1.Now()
	backup.Status.Phase = popv1alpha1.EtcdBackupPhaseFailed
	backup.Status.Message = message
	backup.Status.CompletionTime = &now
	meta.SetStatusCondition(&backup.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  "BackupFailed",
		Message: message,
	})
	if err := r.Status().Update(ctx, backup); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func ptr[T any](v T) *T { return &v }

func (r *EtcdBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&popv1alpha1.EtcdBackup{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
