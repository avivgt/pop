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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type EtcdBackupPhase string

const (
	EtcdBackupPhasePending    EtcdBackupPhase = "Pending"
	EtcdBackupPhaseValidating EtcdBackupPhase = "Validating"
	EtcdBackupPhaseRunning    EtcdBackupPhase = "Running"
	EtcdBackupPhaseCompleted  EtcdBackupPhase = "Completed"
	EtcdBackupPhaseFailed     EtcdBackupPhase = "Failed"
)

// EtcdBackupSpec defines the desired state of EtcdBackup.
type EtcdBackupSpec struct {
	// PVCName is the name of the PersistentVolumeClaim where backups are stored.
	// The PVC must exist in the same namespace as the EtcdBackup resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	PVCName string `json:"pvcName"`

	// ControlPlaneNode optionally specifies which control plane node to run the
	// backup on. If empty, the controller selects one automatically.
	// +optional
	ControlPlaneNode string `json:"controlPlaneNode,omitempty"`

	// BackupTimeoutMinutes is the maximum time allowed for the backup Job to complete.
	// +kubebuilder:default=15
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=60
	// +optional
	BackupTimeoutMinutes int32 `json:"backupTimeoutMinutes,omitempty"`

	// ExportTarget defines where backup artifacts are exported.
	// Defaults to PVC export using PVCName.
	// +optional
	ExportTarget *ExportTarget `json:"exportTarget,omitempty"`

	// RetentionCount is the max number of backups to keep on the PVC.
	// Older backups are deleted before the new backup runs.
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	// +optional
	RetentionCount int32 `json:"retentionCount,omitempty"`
}

// EtcdBackupStatus defines the observed state of EtcdBackup.
type EtcdBackupStatus struct {
	// Phase is the current phase of the backup.
	// +kubebuilder:validation:Enum=Pending;Validating;Running;Completed;Failed
	Phase EtcdBackupPhase `json:"phase,omitempty"`

	// BackupNode is the control plane node where the backup was taken.
	BackupNode string `json:"backupNode,omitempty"`

	// BackupPath is the path within the PVC where backup files are stored.
	BackupPath string `json:"backupPath,omitempty"`

	// SnapshotFile is the name of the etcd snapshot file.
	SnapshotFile string `json:"snapshotFile,omitempty"`

	// StaticResourcesFile is the name of the static resources archive.
	StaticResourcesFile string `json:"staticResourcesFile,omitempty"`

	// JobName is the name of the backup Job created by this resource.
	JobName string `json:"jobName,omitempty"`

	// StartTime is when the backup started.
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the backup completed.
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// S3URI is the S3 object URI if exported to S3.
	S3URI string `json:"s3URI,omitempty"`

	// Message provides detail about the current phase.
	Message string `json:"message,omitempty"`

	// BackupSizeBytes is the size of this backup's artifacts in bytes.
	BackupSizeBytes int64 `json:"backupSizeBytes,omitempty"`

	// BackupSize is the human-readable backup size (e.g., "1.5Gi").
	BackupSize string `json:"backupSize,omitempty"`

	// PVCUsedBytes is the PVC usage in bytes after this backup completed.
	PVCUsedBytes int64 `json:"pvcUsedBytes,omitempty"`

	// PVCCapacityBytes is the total PVC capacity in bytes.
	PVCCapacityBytes int64 `json:"pvcCapacityBytes,omitempty"`

	// PVCUsagePercent is the PVC usage percentage after this backup.
	PVCUsagePercent int32 `json:"pvcUsagePercent,omitempty"`

	// EstimatedCapacity is how many backups of this size the PVC can hold at 80% utilization.
	EstimatedCapacity int32 `json:"estimatedCapacity,omitempty"`

	// RecommendedPVCSize is the PVC size needed to hold retentionCount backups of this size.
	RecommendedPVCSize string `json:"recommendedPVCSize,omitempty"`

	// Conditions represent the latest available observations.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.backupNode`
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.status.backupSize`
// +kubebuilder:printcolumn:name="PVC%",type=integer,JSONPath=`.status.pvcUsagePercent`
// +kubebuilder:printcolumn:name="Fits",type=integer,JSONPath=`.status.estimatedCapacity`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// EtcdBackup triggers a one-shot etcd backup on a control plane node
// and exports the artifacts to a PVC.
type EtcdBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EtcdBackupSpec   `json:"spec,omitempty"`
	Status EtcdBackupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EtcdBackupList contains a list of EtcdBackup.
type EtcdBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EtcdBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EtcdBackup{}, &EtcdBackupList{})
}
