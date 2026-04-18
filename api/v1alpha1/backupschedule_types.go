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

type BackupFrequency string

const (
	BackupFrequencyNow          BackupFrequency = "Now"
	BackupFrequencyEvery4Hours  BackupFrequency = "Every4Hours"
	BackupFrequencyEvery8Hours  BackupFrequency = "Every8Hours"
	BackupFrequencyEvery12Hours BackupFrequency = "Every12Hours"
	BackupFrequencyDaily        BackupFrequency = "Daily"
	BackupFrequencyWeekly       BackupFrequency = "Weekly"
	BackupFrequencyMonthly      BackupFrequency = "Monthly"
	BackupFrequencyCustom       BackupFrequency = "Custom"
)

type CustomInterval struct {
	// Every is the interval value.
	// +kubebuilder:validation:Minimum=1
	Every int32 `json:"every"`

	// Unit is the interval unit.
	// +kubebuilder:validation:Enum=Minutes;Hours;Days
	Unit string `json:"unit"`
}

// BackupScheduleSpec defines the desired state of BackupSchedule.
type BackupScheduleSpec struct {
	// Frequency is the backup frequency preset.
	// Use "Now" to trigger an immediate backup, then continue on the next preset.
	// Use "Custom" with customInterval for non-standard intervals.
	// +kubebuilder:validation:Enum=Now;Every4Hours;Every8Hours;Every12Hours;Daily;Weekly;Monthly;Custom
	// +kubebuilder:default=Daily
	// +optional
	Frequency BackupFrequency `json:"frequency,omitempty"`

	// Time is the time of day in HH:MM format (UTC).
	// Used with Daily, Weekly, and Monthly frequencies.
	// +kubebuilder:validation:Pattern=`^([01]\d|2[0-3]):[0-5]\d$`
	// +kubebuilder:default="02:00"
	// +optional
	Time string `json:"time,omitempty"`

	// DayOfWeek is the day for Weekly frequency.
	// +kubebuilder:validation:Enum=Sunday;Monday;Tuesday;Wednesday;Thursday;Friday;Saturday
	// +optional
	DayOfWeek string `json:"dayOfWeek,omitempty"`

	// DayOfMonth is the day for Monthly frequency (1-28).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=28
	// +optional
	DayOfMonth *int32 `json:"dayOfMonth,omitempty"`

	// CustomInterval defines a custom repeat interval.
	// Used when frequency is "Custom".
	// +optional
	CustomInterval *CustomInterval `json:"customInterval,omitempty"`

	// Schedule is a raw cron expression for advanced users.
	// If set, this overrides the frequency/time/day fields.
	// +optional
	Schedule string `json:"schedule,omitempty"`

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

	// ResolvedSchedule is the cron expression computed from the frequency fields.
	ResolvedSchedule string `json:"resolvedSchedule,omitempty"`

	// HumanReadableSchedule is a user-friendly description of the schedule.
	HumanReadableSchedule string `json:"humanReadableSchedule,omitempty"`

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
// +kubebuilder:printcolumn:name="Frequency",type=string,JSONPath=`.spec.frequency`
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.status.humanReadableSchedule`
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
