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

type ClusterShutdownPhase string

const (
	ClusterShutdownPhasePending       ClusterShutdownPhase = "Pending"
	ClusterShutdownPhaseScheduled     ClusterShutdownPhase = "Scheduled"
	ClusterShutdownPhaseValidating    ClusterShutdownPhase = "Validating"
	ClusterShutdownPhaseCordoning     ClusterShutdownPhase = "Cordoning"
	ClusterShutdownPhaseDraining      ClusterShutdownPhase = "Draining"
	ClusterShutdownPhaseRunningHooks  ClusterShutdownPhase = "RunningHooks"
	ClusterShutdownPhaseNotifyingApps ClusterShutdownPhase = "NotifyingApps"
	ClusterShutdownPhaseBackingUp     ClusterShutdownPhase = "BackingUp"
	ClusterShutdownPhaseShuttingDown  ClusterShutdownPhase = "ShuttingDown"
	ClusterShutdownPhaseComplete      ClusterShutdownPhase = "Complete"
	ClusterShutdownPhaseFailed        ClusterShutdownPhase = "Failed"
)

// ClusterType is the detected cluster topology.
type ClusterType string

const (
	// ClusterTypeSingleNode is a single-node OpenShift cluster (SNO): one node
	// acting as both control-plane and worker. No peer nodes to drain to.
	ClusterTypeSingleNode ClusterType = "SingleNode"
	// ClusterTypeMultiNode is any cluster with more than one node.
	ClusterTypeMultiNode ClusterType = "MultiNode"
)

type ShutdownHook struct {
	// Name is a human-readable identifier for this hook.
	Name string `json:"name"`

	// Image is the container image to run.
	Image string `json:"image"`

	// Command is the command to execute.
	Command []string `json:"command"`

	// TimeoutSeconds is the maximum time for this hook to complete.
	// +kubebuilder:default=300
	TimeoutSeconds int32 `json:"timeoutSeconds"`
}

type HookStatus struct {
	// Name of the hook.
	Name string `json:"name"`

	// Phase is Pending, Running, Completed, or Failed.
	Phase string `json:"phase"`

	// JobName is the name of the Job created for this hook.
	JobName string `json:"jobName,omitempty"`

	// Message provides detail.
	Message string `json:"message,omitempty"`
}

type NodeShutdownState struct {
	// Name of the node.
	Name string `json:"name"`

	// Role is "control-plane" or "worker".
	Role string `json:"role"`

	// Status is "Pending", "ShuttingDown", "Shutdown", or "Failed".
	Status string `json:"status"`

	// ShutdownPodName is the name of the pod that initiated shutdown on this node.
	ShutdownPodName string `json:"shutdownPodName,omitempty"`

	// Message provides detail about the node shutdown status.
	Message string `json:"message,omitempty"`
}

// ClusterShutdownSpec defines the desired state of ClusterShutdown.
type ClusterShutdownSpec struct {
	// PVCName is the name of the PVC for the etcd backup.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	PVCName string `json:"pvcName"`

	// DryRun when true validates prerequisites and takes an etcd backup
	// but does NOT shut down nodes.
	// +kubebuilder:default=false
	// +optional
	DryRun bool `json:"dryRun,omitempty"`

	// ShutdownDelayMinutes is the delay passed to `shutdown -h` on worker nodes.
	// Control plane nodes receive double this value so workers shut down first.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=60
	// +optional
	ShutdownDelayMinutes int32 `json:"shutdownDelayMinutes,omitempty"`

	// SkipCertCheck when true bypasses the kube-apiserver certificate expiry check.
	// +kubebuilder:default=false
	// +optional
	SkipCertCheck bool `json:"skipCertCheck,omitempty"`

	// CertExpiryThresholdDays is the minimum remaining certificate lifetime
	// (in days) required before proceeding with shutdown. If the certificate
	// expires sooner, the shutdown is rejected.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=1
	// +optional
	CertExpiryThresholdDays int32 `json:"certExpiryThresholdDays,omitempty"`

	// SkipPreflightChecks when true bypasses enhanced pre-flight checks
	// (degraded operators, in-progress updates).
	// +kubebuilder:default=false
	// +optional
	SkipPreflightChecks bool `json:"skipPreflightChecks,omitempty"`

	// Reason documents why the shutdown is happening (audit trail).
	// +optional
	Reason string `json:"reason,omitempty"`

	// ScheduledTime is a future time to begin the shutdown.
	// If set, the controller waits until this time before proceeding.
	// +optional
	ScheduledTime *metav1.Time `json:"scheduledTime,omitempty"`

	// CordonNodes when true cordons all nodes before shutdown.
	// +kubebuilder:default=true
	// +optional
	CordonNodes bool `json:"cordonNodes,omitempty"`

	// PreShutdownHooks is a list of Jobs to run before shutdown.
	// All hooks must complete successfully before proceeding.
	// +optional
	PreShutdownHooks []ShutdownHook `json:"preShutdownHooks,omitempty"`

	// NotifyApplications when true creates a ConfigMap signal that
	// applications can watch to prepare for shutdown.
	// +kubebuilder:default=true
	// +optional
	NotifyApplications bool `json:"notifyApplications,omitempty"`

	// WorkloadEvacuationRef optionally references a WorkloadEvacuation CR.
	// If set, the controller waits for evacuation to complete before shutting down.
	// +optional
	WorkloadEvacuationRef string `json:"workloadEvacuationRef,omitempty"`

	// DrainTimeoutSeconds is the maximum time to wait for pod evictions per node.
	// +kubebuilder:default=300
	// +kubebuilder:validation:Minimum=30
	// +kubebuilder:validation:Maximum=1800
	// +optional
	DrainTimeoutSeconds int32 `json:"drainTimeoutSeconds,omitempty"`
}

// ClusterShutdownStatus defines the observed state of ClusterShutdown.
type ClusterShutdownStatus struct {
	// Phase is the current phase.
	// +kubebuilder:validation:Enum=Pending;Scheduled;Validating;Cordoning;Draining;RunningHooks;NotifyingApps;BackingUp;ShuttingDown;Complete;Failed
	Phase ClusterShutdownPhase `json:"phase,omitempty"`

	// EtcdBackupRef is the name of the EtcdBackup CR created for this shutdown.
	EtcdBackupRef string `json:"etcdBackupRef,omitempty"`

	// CertificateExpiry is the detected certificate expiry time.
	CertificateExpiry *metav1.Time `json:"certificateExpiry,omitempty"`

	// NodeStatuses tracks the shutdown state per node.
	NodeStatuses []NodeShutdownState `json:"nodeStatuses,omitempty"`

	// TotalNodes is the total number of nodes targeted.
	TotalNodes int32 `json:"totalNodes,omitempty"`

	// ClusterType is the detected topology: SingleNode (SNO) or MultiNode.
	// Determines whether the drain phase performs pod eviction.
	// +kubebuilder:validation:Enum=SingleNode;MultiNode
	// +optional
	ClusterType ClusterType `json:"clusterType,omitempty"`

	// ShutdownNodes is the count of nodes that have been issued a shutdown command.
	ShutdownNodes int32 `json:"shutdownNodes,omitempty"`

	// StartTime is when the shutdown process started.
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the process completed.
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// EstimatedCompletionTime is the estimated time the shutdown will complete.
	EstimatedCompletionTime *metav1.Time `json:"estimatedCompletionTime,omitempty"`

	// HookStatuses tracks pre-shutdown hook execution.
	HookStatuses []HookStatus `json:"hookStatuses,omitempty"`

	// CordonedNodes is the count of nodes that have been cordoned.
	CordonedNodes int32 `json:"cordonedNodes,omitempty"`

	// PreflightStatus summarizes pre-flight check results.
	PreflightStatus *PreflightStatus `json:"preflightStatus,omitempty"`

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
// +kubebuilder:printcolumn:name="DryRun",type=boolean,JSONPath=`.spec.dryRun`
// +kubebuilder:printcolumn:name="Nodes",type=integer,JSONPath=`.status.totalNodes`
// +kubebuilder:printcolumn:name="Shutdown",type=integer,JSONPath=`.status.shutdownNodes`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterShutdown orchestrates a full OpenShift graceful shutdown:
// validate prerequisites, back up etcd, and shut down all nodes.
type ClusterShutdown struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterShutdownSpec   `json:"spec,omitempty"`
	Status ClusterShutdownStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterShutdownList contains a list of ClusterShutdown.
type ClusterShutdownList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterShutdown `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterShutdown{}, &ClusterShutdownList{})
}
