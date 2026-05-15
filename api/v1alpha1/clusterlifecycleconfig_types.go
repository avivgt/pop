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

// --- Backup configuration ---

type BackupStorageConfig struct {
	// PVCName is the PVC for etcd backup storage.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	PVCName string `json:"pvcName"`

	// ExportTarget optionally configures S3 export.
	// +optional
	ExportTarget *ExportTarget `json:"exportTarget,omitempty"`
}

type BackupScheduleConfig struct {
	// Enabled activates automatic scheduled backups.
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// Frequency is the backup frequency preset.
	// +kubebuilder:validation:Enum=Every4Hours;Every8Hours;Every12Hours;Daily;Weekly;Monthly;Custom
	// +kubebuilder:default=Daily
	// +optional
	Frequency BackupFrequency `json:"frequency,omitempty"`

	// Time is the time of day in HH:MM format (UTC).
	// +kubebuilder:validation:Pattern=`^([01]\d|2[0-3]):[0-5]\d$`
	// +kubebuilder:default="02:00"
	// +optional
	Time string `json:"time,omitempty"`

	// DayOfWeek for Weekly frequency.
	// +kubebuilder:validation:Enum=Sunday;Monday;Tuesday;Wednesday;Thursday;Friday;Saturday
	// +optional
	DayOfWeek string `json:"dayOfWeek,omitempty"`

	// DayOfMonth for Monthly frequency (1-28).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=28
	// +optional
	DayOfMonth *int32 `json:"dayOfMonth,omitempty"`

	// CustomInterval for Custom frequency.
	// +optional
	CustomInterval *CustomInterval `json:"customInterval,omitempty"`
}

type BackupRetentionConfig struct {
	// Count is the maximum number of completed backups to keep.
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	Count int32 `json:"count,omitempty"`

	// Days is the maximum age of backups in days.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=1
	Days int32 `json:"days,omitempty"`
}

type LifecycleBackupConfig struct {
	// Storage defines where backups are stored.
	Storage BackupStorageConfig `json:"storage"`

	// Schedule configures automatic backup timing.
	// +optional
	Schedule *BackupScheduleConfig `json:"schedule,omitempty"`

	// Retention configures backup cleanup policy.
	// +optional
	Retention *BackupRetentionConfig `json:"retention,omitempty"`

	// TimeoutMinutes is the maximum time for a backup job.
	// +kubebuilder:default=15
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=60
	// +optional
	TimeoutMinutes int32 `json:"timeoutMinutes,omitempty"`
}

// --- Shutdown configuration ---

type LifecycleShutdownConfig struct {
	// DefaultDelayMinutes is the shutdown delay for worker nodes.
	// Control plane nodes get double this value.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	DefaultDelayMinutes int32 `json:"defaultDelayMinutes,omitempty"`

	// CordonNodes when true cordons all nodes before shutdown.
	// +kubebuilder:default=true
	// +optional
	CordonNodes bool `json:"cordonNodes,omitempty"`

	// NotifyApplications creates a ConfigMap signal before shutdown.
	// +kubebuilder:default=true
	// +optional
	NotifyApplications bool `json:"notifyApplications,omitempty"`

	// PreShutdownHooks are Jobs to run before shutdown.
	// +optional
	PreShutdownHooks []ShutdownHook `json:"preShutdownHooks,omitempty"`
}

// --- Top-level spec ---

// ClusterLifecycleConfigSpec is the single configuration for all POP features.
type ClusterLifecycleConfigSpec struct {
	// Backup configures etcd backup storage, scheduling, and retention.
	Backup LifecycleBackupConfig `json:"backup"`

	// Shutdown configures cluster shutdown defaults.
	// +optional
	Shutdown *LifecycleShutdownConfig `json:"shutdown,omitempty"`

	// Notification configures alert channels.
	// +optional
	Notification *NotificationConfig `json:"notification,omitempty"`
}

// --- Status ---

type BackupStatusSummary struct {
	// LastBackupTime is when the most recent backup was triggered.
	LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`
	// LastSuccessfulBackup is the name of the last successful EtcdBackup.
	LastSuccessfulBackup string `json:"lastSuccessfulBackup,omitempty"`
	// NextBackupTime is when the next scheduled backup will run.
	NextBackupTime *metav1.Time `json:"nextBackupTime,omitempty"`
	// ActiveBackup is the name of the in-progress backup.
	ActiveBackup string `json:"activeBackup,omitempty"`
	// TotalBackups is the total number of backups created.
	TotalBackups int32 `json:"totalBackups,omitempty"`
	// FailedBackups is the count of failed backups.
	FailedBackups int32 `json:"failedBackups,omitempty"`
	// ResolvedSchedule is the human-readable schedule description.
	ResolvedSchedule string `json:"resolvedSchedule,omitempty"`
}

type ClusterStatusSummary struct {
	// CertificateExpiry is when the kube-apiserver cert expires.
	CertificateExpiry *metav1.Time `json:"certificateExpiry,omitempty"`
	// CertificateRemainingDays is days until cert expiry.
	CertificateRemainingDays int32 `json:"certificateRemainingDays,omitempty"`
	// NodeCount is the total node count.
	NodeCount int32 `json:"nodeCount,omitempty"`
	// ControlPlaneNodes is the control plane node count.
	ControlPlaneNodes int32 `json:"controlPlaneNodes,omitempty"`
	// WorkerNodes is the worker node count.
	WorkerNodes int32 `json:"workerNodes,omitempty"`
}

// ClusterLifecycleConfigStatus is the observed state of the cluster lifecycle.
type ClusterLifecycleConfigStatus struct {
	// Backup summarizes backup status.
	Backup BackupStatusSummary `json:"backup,omitempty"`
	// Cluster summarizes cluster health.
	Cluster ClusterStatusSummary `json:"cluster,omitempty"`
	// Conditions represent the latest observations.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.status.backup.resolvedSchedule`
// +kubebuilder:printcolumn:name="Last Backup",type=date,JSONPath=`.status.backup.lastBackupTime`
// +kubebuilder:printcolumn:name="Nodes",type=integer,JSONPath=`.status.cluster.nodeCount`
// +kubebuilder:printcolumn:name="Cert Days",type=integer,JSONPath=`.status.cluster.certificateRemainingDays`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterLifecycleConfig is the single configuration for POP cluster lifecycle management.
// Create one instance named "default" in the operator namespace.
type ClusterLifecycleConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterLifecycleConfigSpec   `json:"spec,omitempty"`
	Status ClusterLifecycleConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterLifecycleConfigList contains a list of ClusterLifecycleConfig.
type ClusterLifecycleConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterLifecycleConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterLifecycleConfig{}, &ClusterLifecycleConfigList{})
}
