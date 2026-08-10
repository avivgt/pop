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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type FleetBootSequencePhase string

const (
	FleetBootSequencePhasePending    FleetBootSequencePhase = "Pending"
	FleetBootSequencePhaseInProgress FleetBootSequencePhase = "InProgress"
	FleetBootSequencePhaseComplete   FleetBootSequencePhase = "Complete"
	FleetBootSequencePhaseTimedOut   FleetBootSequencePhase = "TimedOut"
)

// FleetBootTier is one ordered stage of the fleet black-start.
type FleetBootTier struct {
	// Name is a unique identifier for the tier.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// PlacementRef names an ACM Placement (same namespace) selecting this tier's clusters.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	PlacementRef string `json:"placementRef"`

	// DependsOn lists tier names that must be Ready before this tier is evaluated.
	// +optional
	DependsOn []string `json:"dependsOn,omitempty"`

	// TimeoutSeconds bounds how long to wait for this tier to become Ready before
	// the sequence is marked TimedOut.
	// +kubebuilder:default=1800
	// +kubebuilder:validation:Minimum=60
	// +optional
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`
}

// FleetBootSequenceSpec defines the desired state of FleetBootSequence.
type FleetBootSequenceSpec struct {
	// Tiers are the ordered stages. Ordering is by DependsOn (a DAG), not list order.
	// +kubebuilder:validation:MinItems=1
	Tiers []FleetBootTier `json:"tiers"`
}

// FleetBootTierStatus is the observed readiness of a tier.
type FleetBootTierStatus struct {
	Name          string `json:"name"`
	Ready         bool   `json:"ready"`
	ClustersTotal int32  `json:"clustersTotal"`
	ClustersReady int32  `json:"clustersReady"`
	Message       string `json:"message,omitempty"`
}

// FleetBootSequenceStatus defines the observed state of FleetBootSequence.
type FleetBootSequenceStatus struct {
	// +kubebuilder:validation:Enum=Pending;InProgress;Complete;TimedOut
	Phase FleetBootSequencePhase `json:"phase,omitempty"`

	Tiers []FleetBootTierStatus `json:"tiers,omitempty"`

	StartTime *metav1.Time `json:"startTime,omitempty"`

	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	Message string `json:"message,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// FleetBootSequence declares an ordered, health-gated fleet black-start: the hub
// observes each tier become Available (ManagedClusterConditionAvailable) in
// dependency order. It does not power hardware on (that is the optional Phase-3b
// add-on); it enforces the readiness barrier and reports progress.
type FleetBootSequence struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FleetBootSequenceSpec   `json:"spec,omitempty"`
	Status FleetBootSequenceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FleetBootSequenceList contains a list of FleetBootSequence.
type FleetBootSequenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetBootSequence `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FleetBootSequence{}, &FleetBootSequenceList{})
}
