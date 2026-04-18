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

type EvacuationStrategy string

const (
	EvacuationStrategyAllAtOnce EvacuationStrategy = "AllAtOnce"
	EvacuationStrategyCanary    EvacuationStrategy = "Canary"
)

type EvacuationPhase string

const (
	EvacuationPhasePending    EvacuationPhase = "Pending"
	EvacuationPhaseEvacuating EvacuationPhase = "Evacuating"
	EvacuationPhaseVerifying  EvacuationPhase = "Verifying"
	EvacuationPhaseComplete   EvacuationPhase = "Complete"
	EvacuationPhaseFailed     EvacuationPhase = "Failed"
)

type EvacuationWave struct {
	// Percent of workloads to migrate in this wave.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	Percent int32 `json:"percent"`

	// VerifyDurationSeconds is how long to verify health after migration
	// before proceeding to the next wave.
	// +kubebuilder:default=300
	// +optional
	VerifyDurationSeconds int32 `json:"verifyDurationSeconds,omitempty"`
}

type WaveStatus struct {
	// Wave is the wave index (1-based).
	Wave int32 `json:"wave"`

	// Percent is the target percentage for this wave.
	Percent int32 `json:"percent"`

	// Phase is Pending, Migrating, Verifying, Complete, or Failed.
	Phase string `json:"phase"`

	// Migrated is the number of workloads migrated in this wave.
	Migrated int32 `json:"migrated"`

	// Total is the target number of workloads for this wave.
	Total int32 `json:"total"`

	// Message provides detail.
	Message string `json:"message,omitempty"`
}

// WorkloadEvacuationSpec defines the desired state of WorkloadEvacuation.
type WorkloadEvacuationSpec struct {
	// Strategy is AllAtOnce or Canary (A/B testing style gradual migration).
	// +kubebuilder:validation:Enum=AllAtOnce;Canary
	// +kubebuilder:default=Canary
	Strategy EvacuationStrategy `json:"strategy"`

	// Waves defines the canary waves for gradual migration.
	// Only used when strategy is Canary.
	// +optional
	Waves []EvacuationWave `json:"waves,omitempty"`

	// TargetClusters lists preferred destination clusters for workloads.
	// If empty, ACM chooses based on available capacity.
	// +optional
	TargetClusters []string `json:"targetClusters,omitempty"`

	// Namespaces lists the namespaces to evacuate.
	// If empty, all user namespaces are evacuated.
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

	// DryRun simulates the evacuation without moving workloads.
	// +kubebuilder:default=false
	// +optional
	DryRun bool `json:"dryRun,omitempty"`

	// Reason documents why the evacuation is happening.
	// +optional
	Reason string `json:"reason,omitempty"`
}

// WorkloadEvacuationStatus defines the observed state of WorkloadEvacuation.
type WorkloadEvacuationStatus struct {
	// Phase is the current phase of the evacuation.
	// +kubebuilder:validation:Enum=Pending;Evacuating;Verifying;Complete;Failed
	Phase EvacuationPhase `json:"phase,omitempty"`

	// CurrentWave is the active wave index (1-based).
	CurrentWave int32 `json:"currentWave,omitempty"`

	// WaveStatuses tracks each wave's progress.
	WaveStatuses []WaveStatus `json:"waveStatuses,omitempty"`

	// TotalWorkloads is the total number of workloads in scope.
	TotalWorkloads int32 `json:"totalWorkloads,omitempty"`

	// MigratedWorkloads is the total successfully migrated so far.
	MigratedWorkloads int32 `json:"migratedWorkloads,omitempty"`

	// StartTime is when the evacuation started.
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when it completed.
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Message provides detail about the current phase.
	Message string `json:"message,omitempty"`

	// Conditions represent the latest available observations.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Strategy",type=string,JSONPath=`.spec.strategy`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Migrated",type=integer,JSONPath=`.status.migratedWorkloads`
// +kubebuilder:printcolumn:name="Total",type=integer,JSONPath=`.status.totalWorkloads`
// +kubebuilder:printcolumn:name="DryRun",type=boolean,JSONPath=`.spec.dryRun`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// WorkloadEvacuation requests ACM to migrate workloads away from this cluster,
// supporting canary/A-B testing style gradual migration.
type WorkloadEvacuation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkloadEvacuationSpec   `json:"spec,omitempty"`
	Status WorkloadEvacuationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkloadEvacuationList contains a list of WorkloadEvacuation.
type WorkloadEvacuationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkloadEvacuation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkloadEvacuation{}, &WorkloadEvacuationList{})
}
