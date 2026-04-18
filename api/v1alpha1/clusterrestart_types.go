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

type ClusterRestartPhase string

const (
	ClusterRestartPhasePending                ClusterRestartPhase = "Pending"
	ClusterRestartPhaseWaitingForControlPlane ClusterRestartPhase = "WaitingForControlPlane"
	ClusterRestartPhaseApprovingCSRs          ClusterRestartPhase = "ApprovingCSRs"
	ClusterRestartPhaseWaitingForWorkers      ClusterRestartPhase = "WaitingForWorkers"
	ClusterRestartPhaseVerifyingOperators     ClusterRestartPhase = "VerifyingOperators"
	ClusterRestartPhaseComplete               ClusterRestartPhase = "Complete"
	ClusterRestartPhaseFailed                 ClusterRestartPhase = "Failed"
)

type NodeRestartStatus struct {
	// Name of the node.
	Name string `json:"name"`

	// Role is "control-plane" or "worker".
	Role string `json:"role"`

	// Ready indicates whether the node is Ready.
	Ready bool `json:"ready"`

	// Message provides additional detail.
	Message string `json:"message,omitempty"`
}

// ClusterRestartSpec defines the desired state of ClusterRestart.
type ClusterRestartSpec struct {
	// AutoApproveCSRs automatically approves pending kubelet CSRs after restart.
	// +kubebuilder:default=true
	// +optional
	AutoApproveCSRs bool `json:"autoApproveCSRs,omitempty"`

	// WaitTimeoutMinutes is how long to wait for nodes to become Ready.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=120
	// +optional
	WaitTimeoutMinutes int32 `json:"waitTimeoutMinutes,omitempty"`

	// ExpectedNodes is the expected number of nodes. If set, the controller
	// verifies this many nodes exist and are Ready before completing.
	// +optional
	ExpectedNodes *int32 `json:"expectedNodes,omitempty"`
}

// ClusterRestartStatus defines the observed state of ClusterRestart.
type ClusterRestartStatus struct {
	// Phase is the current phase.
	// +kubebuilder:validation:Enum=Pending;WaitingForControlPlane;ApprovingCSRs;WaitingForWorkers;VerifyingOperators;Complete;Failed
	Phase ClusterRestartPhase `json:"phase,omitempty"`

	// NodeStatuses tracks readiness of each node.
	NodeStatuses []NodeRestartStatus `json:"nodeStatuses,omitempty"`

	// ApprovedCSRs is the count of CSRs approved by this controller.
	ApprovedCSRs int32 `json:"approvedCSRs,omitempty"`

	// DegradedOperators lists ClusterOperators that are Degraded or not Available.
	DegradedOperators []string `json:"degradedOperators,omitempty"`

	// TotalNodes is the total number of nodes discovered.
	TotalNodes int32 `json:"totalNodes,omitempty"`

	// ReadyNodes is the count of nodes that are Ready.
	ReadyNodes int32 `json:"readyNodes,omitempty"`

	// StartTime is when the restart process started.
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the process completed.
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Message provides detail about the current phase.
	Message string `json:"message,omitempty"`

	// Conditions represent the latest available observations.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyNodes`
// +kubebuilder:printcolumn:name="Total",type=integer,JSONPath=`.status.totalNodes`
// +kubebuilder:printcolumn:name="CSRs",type=integer,JSONPath=`.status.approvedCSRs`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterRestart orchestrates bringing an OpenShift cluster back up
// after a graceful shutdown.
type ClusterRestart struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterRestartSpec   `json:"spec,omitempty"`
	Status ClusterRestartStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterRestartList contains a list of ClusterRestart.
type ClusterRestartList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterRestart `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterRestart{}, &ClusterRestartList{})
}
