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

type PowerEventType string

const (
	PowerEventTypeFailure  PowerEventType = "PowerFailure"
	PowerEventTypeRestored PowerEventType = "PowerRestored"
	PowerEventTypeTest     PowerEventType = "Test"
)

type PowerEventPhase string

const (
	PowerEventPhaseDetected             PowerEventPhase = "Detected"
	PowerEventPhaseBackingUp            PowerEventPhase = "BackingUp"
	PowerEventPhaseAwaitingConfirmation PowerEventPhase = "AwaitingConfirmation"
	PowerEventPhaseConfirmed            PowerEventPhase = "Confirmed"
	PowerEventPhaseShuttingDown         PowerEventPhase = "ShuttingDown"
	PowerEventPhaseComplete             PowerEventPhase = "Complete"
	PowerEventPhaseCancelled            PowerEventPhase = "Cancelled"
	PowerEventPhaseFailed               PowerEventPhase = "Failed"
)

type NotificationRecord struct {
	// Channel is the notification channel used.
	Channel string `json:"channel"`

	// SentAt is when the notification was sent.
	SentAt metav1.Time `json:"sentAt"`

	// Success indicates whether the notification was sent successfully.
	Success bool `json:"success"`

	// Message is a detail about the notification outcome.
	Message string `json:"message,omitempty"`
}

// PowerEventSpec defines the desired state of PowerEvent.
type PowerEventSpec struct {
	// EventType is the type of power event.
	// +kubebuilder:validation:Enum=PowerFailure;PowerRestored;Test
	// +kubebuilder:validation:Required
	EventType PowerEventType `json:"eventType"`

	// Source is the identifier of the UPS/PDU that reported the event.
	// +optional
	Source string `json:"source,omitempty"`

	// TriggeredBy indicates how the event was created.
	// +kubebuilder:validation:Enum=webhook;manual
	// +kubebuilder:default=webhook
	// +optional
	TriggeredBy string `json:"triggeredBy,omitempty"`

	// Message is an optional detail about the event.
	// +optional
	Message string `json:"message,omitempty"`
}

// PowerEventStatus defines the observed state of PowerEvent.
type PowerEventStatus struct {
	// Phase is the current phase of the power event response.
	// +kubebuilder:validation:Enum=Detected;BackingUp;AwaitingConfirmation;Confirmed;ShuttingDown;Complete;Cancelled;Failed
	Phase PowerEventPhase `json:"phase,omitempty"`

	// DetectedAt is when the power event was first detected.
	DetectedAt *metav1.Time `json:"detectedAt,omitempty"`

	// ConfirmedAt is when the outage was confirmed (after confirmation period).
	ConfirmedAt *metav1.Time `json:"confirmedAt,omitempty"`

	// CancelledAt is when the event was cancelled (power restored).
	CancelledAt *metav1.Time `json:"cancelledAt,omitempty"`

	// BackupStartedAt is when the emergency backup started.
	BackupStartedAt *metav1.Time `json:"backupStartedAt,omitempty"`

	// ShutdownStartedAt is when the autonomous shutdown started.
	ShutdownStartedAt *metav1.Time `json:"shutdownStartedAt,omitempty"`

	// EtcdBackupRef is the name of the EtcdBackup CR created for this event.
	EtcdBackupRef string `json:"etcdBackupRef,omitempty"`

	// ClusterShutdownRef is the name of the ClusterShutdown CR created.
	ClusterShutdownRef string `json:"clusterShutdownRef,omitempty"`

	// NotificationsSent tracks notifications sent for this event.
	NotificationsSent []NotificationRecord `json:"notificationsSent,omitempty"`

	// ConfirmationDeadline is when the confirmation period expires.
	ConfirmationDeadline *metav1.Time `json:"confirmationDeadline,omitempty"`

	// ShutdownDeadline is when the autonomous shutdown will trigger.
	ShutdownDeadline *metav1.Time `json:"shutdownDeadline,omitempty"`

	// ObservedGeneration is the most recent generation observed.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Message provides detail about the current phase.
	Message string `json:"message,omitempty"`

	// Conditions represent the latest available observations.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.eventType`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.source`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PowerEvent represents a power-related event detected by the PSU alert system.
type PowerEvent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PowerEventSpec   `json:"spec,omitempty"`
	Status PowerEventStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PowerEventList contains a list of PowerEvent.
type PowerEventList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PowerEvent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PowerEvent{}, &PowerEventList{})
}
