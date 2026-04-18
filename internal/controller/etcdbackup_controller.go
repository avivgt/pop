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

type EtcdBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=pop.openshift.io,resources=etcdbackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pop.openshift.io,resources=etcdbackups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pop.openshift.io,resources=etcdbackups/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;delete
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

	controlPlaneNodes, err := r.listControlPlaneNodes(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(controlPlaneNodes) == 0 {
		return r.setFailed(ctx, backup, "no control plane nodes found")
	}

	selectedNode := backup.Spec.ControlPlaneNode
	if selectedNode == "" {
		selectedNode = controlPlaneNodes[0].Name
		logger.Info("auto-selected control plane node", "node", selectedNode)
	} else {
		found := false
		for _, n := range controlPlaneNodes {
			if n.Name == selectedNode {
				found = true
				break
			}
		}
		if !found {
			return r.setFailed(ctx, backup, fmt.Sprintf("specified node %q is not a control plane node", selectedNode))
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

	if existingJob.Status.Failed > 0 {
		backoffLimit := int32(2)
		if existingJob.Spec.BackoffLimit != nil {
			backoffLimit = *existingJob.Spec.BackoffLimit
		}
		if existingJob.Status.Failed >= backoffLimit {
			return r.setFailed(ctx, backup, fmt.Sprintf("backup job failed after %d attempts", existingJob.Status.Failed))
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

	backupScript := `set -euo pipefail
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
BACKUP_DIR="/backup/${TIMESTAMP}"
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
			BackoffLimit:          &backoffLimit,
			ActiveDeadlineSeconds: &timeout,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":       "pop-etcd-backup",
						"app.kubernetes.io/managed-by": "pop-operator",
						"pop.openshift.io/backup":      backup.Name,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					HostNetwork:   true,
					HostPID:       true,
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

func (r *EtcdBackupReconciler) listControlPlaneNodes(ctx context.Context) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return nil, err
	}

	var controlPlane []corev1.Node
	for _, node := range nodeList.Items {
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			controlPlane = append(controlPlane, node)
			continue
		}
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			controlPlane = append(controlPlane, node)
		}
	}
	return controlPlane, nil
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

func (r *EtcdBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&popv1alpha1.EtcdBackup{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
