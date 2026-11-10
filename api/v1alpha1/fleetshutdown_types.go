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

type FleetShutdownPhase string

const (
	FleetShutdownPhasePending         FleetShutdownPhase = "Pending"
	FleetShutdownPhaseInProgress      FleetShutdownPhase = "InProgress"
	FleetShutdownPhaseCompleted       FleetShutdownPhase = "Completed"
	FleetShutdownPhasePartiallyFailed FleetShutdownPhase = "PartiallyFailed"
)

// FleetShutdownPolicy is inherited by each per-spoke ClusterShutdown.
type FleetShutdownPolicy struct {
	// OnStuck selects the per-phase stuck behavior on the spoke.
	// +kubebuilder:validation:Enum=force;skip;haltAndAlert;escalateToHub
	// +kubebuilder:default=force
	// +optional
	OnStuck string `json:"onStuck,omitempty"`

	// HardDeadlineSeconds bounds the whole spoke shutdown before force-progress.
	// +optional
	HardDeadlineSeconds int32 `json:"hardDeadlineSeconds,omitempty"`
}

// FleetShutdownSpec defines the desired state of FleetShutdown.
type FleetShutdownSpec struct {
	// PlacementRef names an ACM Placement (same namespace) selecting target clusters.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	PlacementRef string `json:"placementRef"`

	// PVCName is passed through to each spoke ClusterShutdown's etcd backup.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	PVCName string `json:"pvcName"`

	// Reason documents why the fleet shutdown is happening (audit trail).
	// +optional
	Reason string `json:"reason,omitempty"`

	// DryRun passes through to each spoke ClusterShutdown.
	// +kubebuilder:default=false
	// +optional
	DryRun bool `json:"dryRun,omitempty"`

	// ShutdownPolicy is inherited by each spoke ClusterShutdown.
	// +optional
	ShutdownPolicy *FleetShutdownPolicy `json:"shutdownPolicy,omitempty"`
}

// FleetClusterStatus is the per-cluster shutdown progress, aggregated from the
// ManifestWork status feedback of each spoke's ClusterShutdown.
type FleetClusterStatus struct {
	Name    string `json:"name"`
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`

	// BackupRef is the EtcdBackup CR name taken on this spoke before shutdown,
	// surfaced from the spoke ClusterShutdown's status.etcdBackupRef via
	// ManifestWork feedback. Empty until the spoke reports it.
	BackupRef string `json:"backupRef,omitempty"`
}

// FleetShutdownStatus defines the observed state of FleetShutdown.
type FleetShutdownStatus struct {
	// +kubebuilder:validation:Enum=Pending;InProgress;Completed;PartiallyFailed
	Phase FleetShutdownPhase `json:"phase,omitempty"`

	// Clusters is the per-cluster shutdown progress.
	Clusters []FleetClusterStatus `json:"clusters,omitempty"`

	// SelectedClusters is the number of clusters selected by the Placement.
	SelectedClusters int32 `json:"selectedClusters,omitempty"`

	// CompletedClusters is the number of clusters whose shutdown has completed.
	CompletedClusters int32 `json:"completedClusters,omitempty"`

	// ObservedGeneration is the most recent generation observed.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Message provides detail about the current phase.
	Message string `json:"message,omitempty"`

	// Conditions represent the latest available observations.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Selected",type=integer,JSONPath=`.status.selectedClusters`
// +kubebuilder:printcolumn:name="Completed",type=integer,JSONPath=`.status.completedClusters`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// FleetShutdown is a hub-authored intent to gracefully shut down a selected set
// of managed clusters. The hub controller fans out a per-spoke ClusterShutdown
// via ManifestWork.
type FleetShutdown struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FleetShutdownSpec   `json:"spec,omitempty"`
	Status FleetShutdownStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FleetShutdownList contains a list of FleetShutdown.
type FleetShutdownList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FleetShutdown `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FleetShutdown{}, &FleetShutdownList{})
}
