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

type BootSequencePhase string

const (
	BootSequencePhasePending    BootSequencePhase = "Pending"
	BootSequencePhaseInProgress BootSequencePhase = "InProgress"
	BootSequencePhaseComplete   BootSequencePhase = "Complete"
	BootSequencePhaseTimedOut   BootSequencePhase = "TimedOut"
	BootSequencePhaseFailed     BootSequencePhase = "Failed"
)

// BootStageSelector selects the workloads a stage gates on. Currently namespace
// scoped: a stage is Ready when the health gate holds across all pods in these
// namespaces.
type BootStageSelector struct {
	// Namespaces are the namespaces whose pods this stage gates on.
	// +kubebuilder:validation:MinItems=1
	Namespaces []string `json:"namespaces"`
}

// BootStage is one ordered stage of the intra-cluster workload black-start.
type BootStage struct {
	// Name is a unique identifier for the stage.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Selector picks the workloads this stage gates on.
	// +kubebuilder:validation:Required
	Selector BootStageSelector `json:"selector"`

	// HealthGate is the readiness condition that must hold before dependent
	// stages are released. AllPodsReady: every non-completed pod in the selected
	// namespaces is Ready (namespaces with no pods are trivially satisfied).
	// +kubebuilder:validation:Enum=AllPodsReady
	// +kubebuilder:default=AllPodsReady
	// +optional
	HealthGate string `json:"healthGate,omitempty"`

	// DependsOn lists stage names that must be Ready before this stage is
	// evaluated. Ordering is by this DAG, not list order.
	// +optional
	DependsOn []string `json:"dependsOn,omitempty"`

	// TimeoutSeconds bounds how long to wait for this stage to become Ready
	// before the sequence is marked TimedOut.
	// +kubebuilder:default=600
	// +kubebuilder:validation:Minimum=30
	// +optional
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`

	// OnStuck selects behavior when the stage does not become Ready within its
	// timeout. force: proceed to dependents anyway; haltAndAlert: stop the
	// sequence and mark TimedOut (default).
	// +kubebuilder:validation:Enum=force;haltAndAlert
	// +kubebuilder:default=haltAndAlert
	// +optional
	OnStuck string `json:"onStuck,omitempty"`
}

// BootSequenceSpec defines the desired state of BootSequence.
type BootSequenceSpec struct {
	// Stages are the ordered workload stages. Ordering is by DependsOn (a DAG),
	// not list order.
	// +kubebuilder:validation:MinItems=1
	Stages []BootStage `json:"stages"`
}

// BootStageStatus is the observed readiness of a stage.
type BootStageStatus struct {
	Name       string `json:"name"`
	Ready      bool   `json:"ready"`
	Forced     bool   `json:"forced,omitempty"`
	PodsTotal  int32  `json:"podsTotal"`
	PodsReady  int32  `json:"podsReady"`
	Message    string `json:"message,omitempty"`
}

// BootSequenceStatus defines the observed state of BootSequence.
type BootSequenceStatus struct {
	// +kubebuilder:validation:Enum=Pending;InProgress;Complete;TimedOut;Failed
	Phase BootSequencePhase `json:"phase,omitempty"`

	Stages []BootStageStatus `json:"stages,omitempty"`

	StartTime *metav1.Time `json:"startTime,omitempty"`

	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	Message string `json:"message,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BootSequence declares an ordered, health-gated intra-cluster workload
// black-start: the spoke operator releases each stage only once its dependencies
// satisfy their health gate (e.g. storage Ready before apps). It is the
// workload-layer complement to the hub's cross-cluster FleetBootSequence.
type BootSequence struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BootSequenceSpec   `json:"spec,omitempty"`
	Status BootSequenceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BootSequenceList contains a list of BootSequence.
type BootSequenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BootSequence `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BootSequence{}, &BootSequenceList{})
}
