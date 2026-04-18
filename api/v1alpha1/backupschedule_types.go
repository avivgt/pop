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

type BackupSchedulePhase string

const (
	BackupSchedulePhaseActive    BackupSchedulePhase = "Active"
	BackupSchedulePhaseSuspended BackupSchedulePhase = "Suspended"
)

// BackupScheduleSpec defines the desired state of BackupSchedule.
type BackupScheduleSpec struct {
	// Schedule is a cron expression (e.g. "0 2 * * *" for daily at 2 AM).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Schedule string `json:"schedule"`

	// PVCName is the PVC for etcd backup storage.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	PVCName string `json:"pvcName"`

	// RetentionCount is the maximum number of completed backups to keep.
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	// +optional
	RetentionCount int32 `json:"retentionCount,omitempty"`

	// RetentionDays is the maximum age (in days) of backups to keep.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=1
	// +optional
	RetentionDays int32 `json:"retentionDays,omitempty"`

	// Suspend pauses scheduling when true.
	// +kubebuilder:default=false
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// BackupTimeoutMinutes passed through to each created EtcdBackup.
	// +kubebuilder:default=15
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=60
	// +optional
	BackupTimeoutMinutes int32 `json:"backupTimeoutMinutes,omitempty"`

	// ExportTarget optionally configures S3 export for scheduled backups.
	// +optional
	ExportTarget *ExportTarget `json:"exportTarget,omitempty"`
}

// BackupScheduleStatus defines the observed state of BackupSchedule.
type BackupScheduleStatus struct {
	// Phase is Active or Suspended.
	Phase BackupSchedulePhase `json:"phase,omitempty"`

	// LastBackupTime is when the most recent backup was triggered.
	LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`

	// NextBackupTime is the computed next trigger time.
	NextBackupTime *metav1.Time `json:"nextBackupTime,omitempty"`

	// LastSuccessfulBackup is the name of the most recent successful EtcdBackup.
	LastSuccessfulBackup string `json:"lastSuccessfulBackup,omitempty"`

	// FailedBackupsCount is the running count of failed scheduled backups.
	FailedBackupsCount int32 `json:"failedBackupsCount,omitempty"`

	// ActiveBackup is the name of the in-progress EtcdBackup, if any.
	ActiveBackup string `json:"activeBackup,omitempty"`

	// Conditions represent the latest available observations.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`
// +kubebuilder:printcolumn:name="Suspend",type=boolean,JSONPath=`.spec.suspend`
// +kubebuilder:printcolumn:name="Last Backup",type=date,JSONPath=`.status.lastBackupTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BackupSchedule creates EtcdBackup CRs on a cron schedule with retention.
type BackupSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupScheduleSpec   `json:"spec,omitempty"`
	Status BackupScheduleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BackupScheduleList contains a list of BackupSchedule.
type BackupScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupSchedule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupSchedule{}, &BackupScheduleList{})
}
