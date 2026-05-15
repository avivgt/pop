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

type ClusterPriority string

const (
	ClusterPriorityCritical   ClusterPriority = "Critical"
	ClusterPriorityStandard   ClusterPriority = "Standard"
	ClusterPriorityBestEffort ClusterPriority = "BestEffort"
)

type PowerStatus string

const (
	PowerStatusNormal    PowerStatus = "Normal"
	PowerStatusOnBattery PowerStatus = "OnBattery"
	PowerStatusUnknown   PowerStatus = "Unknown"
)

type BackupConfig struct {
	// PVCName is the PVC for emergency backup storage.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	PVCName string `json:"pvcName"`

	// ExportTarget optionally configures S3 export for emergency backups.
	// +optional
	ExportTarget *ExportTarget `json:"exportTarget,omitempty"`

	// ShutdownDelayMinutes is passed to ClusterShutdown during autonomous shutdown.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	ShutdownDelayMinutes int32 `json:"shutdownDelayMinutes,omitempty"`
}

type AlertEndpointConfig struct {
	// Port is the HTTP port for the PSU alert webhook receiver.
	// +kubebuilder:default=8082
	// +kubebuilder:validation:Minimum=1024
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// AuthTokenSecret is the name of a Secret containing a "token" key
	// used for bearer token authentication on the alert endpoint.
	// +optional
	AuthTokenSecret string `json:"authTokenSecret,omitempty"`
}

type WebhookNotificationConfig struct {
	// Enabled activates webhook notifications.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// URL is the target for POST JSON notifications.
	// +optional
	URL string `json:"url,omitempty"`

	// AuthSecret is the name of a Secret containing an "authorization" key
	// sent as the Authorization header.
	// +optional
	AuthSecret string `json:"authSecret,omitempty"`
}

type SMTPNotificationConfig struct {
	// Enabled activates SMTP email notifications.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// Host is the SMTP server hostname.
	// +optional
	Host string `json:"host,omitempty"`

	// Port is the SMTP server port.
	// +kubebuilder:default=587
	// +optional
	Port int32 `json:"port,omitempty"`

	// From is the sender email address.
	// +optional
	From string `json:"from,omitempty"`

	// To is the list of recipient email addresses.
	// +optional
	To []string `json:"to,omitempty"`

	// CredentialsSecret is the name of a Secret with "username" and "password".
	// +optional
	CredentialsSecret string `json:"credentialsSecret,omitempty"`

	// UseTLS enables STARTTLS.
	// +kubebuilder:default=true
	// +optional
	UseTLS bool `json:"useTLS,omitempty"`
}

type AlertmanagerNotificationConfig struct {
	// Enabled activates Alertmanager notifications.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// URL is the Alertmanager API endpoint.
	// Defaults to the cluster's built-in Alertmanager if empty.
	// +optional
	URL string `json:"url,omitempty"`
}

type KubernetesEventsConfig struct {
	// Enabled activates Kubernetes Event emission.
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`
}

type NotificationConfig struct {
	// Webhook configures generic HTTP webhook notifications.
	// +optional
	Webhook *WebhookNotificationConfig `json:"webhook,omitempty"`

	// SMTP configures email notifications.
	// +optional
	SMTP *SMTPNotificationConfig `json:"smtp,omitempty"`

	// Alertmanager configures Alertmanager push notifications.
	// +optional
	Alertmanager *AlertmanagerNotificationConfig `json:"alertmanager,omitempty"`

	// KubernetesEvents configures K8s Event emission.
	// +optional
	KubernetesEvents *KubernetesEventsConfig `json:"kubernetesEvents,omitempty"`
}

// --- Event Sources ---

// EventSourcesConfig configures how power and infrastructure events are detected.
type EventSourcesConfig struct {
	// Webhook is the existing PSU alert webhook receiver.
	// +optional
	Webhook *AlertEndpointConfig `json:"webhook,omitempty"`

	// SNMP configures SNMP polling of UPS devices.
	// +optional
	SNMP *SNMPSourceConfig `json:"snmp,omitempty"`

	// IPMI configures IPMI/Redfish BMC power monitoring.
	// +optional
	IPMI *IPMISourceConfig `json:"ipmi,omitempty"`

	// NUT configures Network UPS Tools client monitoring.
	// +optional
	NUT *NUTSourceConfig `json:"nut,omitempty"`
}

type SNMPSourceConfig struct {
	// Enabled activates SNMP polling.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// Host is the SNMP agent address (UPS/PDU).
	// +optional
	Host string `json:"host,omitempty"`

	// Port is the SNMP port.
	// +kubebuilder:default=161
	// +optional
	Port int32 `json:"port,omitempty"`

	// Community is the SNMP v2c community string.
	// +kubebuilder:default="public"
	// +optional
	Community string `json:"community,omitempty"`

	// Version is the SNMP version: "v2c" or "v3".
	// +kubebuilder:validation:Enum=v2c;v3
	// +kubebuilder:default="v2c"
	// +optional
	Version string `json:"version,omitempty"`

	// PollingIntervalSeconds is how often to poll the device.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=5
	// +optional
	PollingIntervalSeconds int32 `json:"pollingIntervalSeconds,omitempty"`

	// BatteryStatusOID is the OID for UPS battery status (default: UPS-MIB 1.3.6.1.2.1.33.1.2.1.0).
	// +optional
	BatteryStatusOID string `json:"batteryStatusOID,omitempty"`

	// OutputSourceOID is the OID for UPS output source (default: UPS-MIB 1.3.6.1.2.1.33.1.4.1.0).
	// +optional
	OutputSourceOID string `json:"outputSourceOID,omitempty"`

	// BatteryRemainingOID is the OID for estimated battery minutes remaining.
	// +optional
	BatteryRemainingOID string `json:"batteryRemainingOID,omitempty"`

	// AuthSecret is the name of a Secret with SNMPv3 credentials (username, authPass, privPass).
	// +optional
	AuthSecret string `json:"authSecret,omitempty"`
}

type IPMISourceConfig struct {
	// Enabled activates IPMI/Redfish monitoring.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// Host is the BMC address.
	// +optional
	Host string `json:"host,omitempty"`

	// Port is the BMC port.
	// +kubebuilder:default=443
	// +optional
	Port int32 `json:"port,omitempty"`

	// Protocol is "redfish" or "ipmi".
	// +kubebuilder:validation:Enum=redfish;ipmi
	// +kubebuilder:default="redfish"
	// +optional
	Protocol string `json:"protocol,omitempty"`

	// PollingIntervalSeconds is how often to poll the BMC.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=5
	// +optional
	PollingIntervalSeconds int32 `json:"pollingIntervalSeconds,omitempty"`

	// AuthSecret is the name of a Secret with BMC credentials (username, password).
	// +kubebuilder:validation:Required
	AuthSecret string `json:"authSecret"`

	// TLSInsecure skips TLS certificate verification.
	// +kubebuilder:default=false
	// +optional
	TLSInsecure bool `json:"tlsInsecure,omitempty"`
}

type NUTSourceConfig struct {
	// Enabled activates NUT client monitoring.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// Host is the NUT server address.
	// +optional
	Host string `json:"host,omitempty"`

	// Port is the NUT server port.
	// +kubebuilder:default=3493
	// +optional
	Port int32 `json:"port,omitempty"`

	// UPSName is the UPS identifier on the NUT server.
	// +kubebuilder:validation:Required
	// +optional
	UPSName string `json:"upsName,omitempty"`

	// PollingIntervalSeconds is how often to poll the NUT server.
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=5
	// +optional
	PollingIntervalSeconds int32 `json:"pollingIntervalSeconds,omitempty"`

	// AuthSecret is the name of a Secret with NUT credentials (username, password).
	// +optional
	AuthSecret string `json:"authSecret,omitempty"`
}

// EventSourceStatus reports the status of a single event source.
type EventSourceStatus struct {
	// Type is the source type: "webhook", "snmp", "ipmi", "nut".
	Type string `json:"type"`

	// Connected indicates whether the source is reachable.
	Connected bool `json:"connected"`

	// LastCheck is the last time connectivity was verified.
	// +optional
	LastCheck *metav1.Time `json:"lastCheck,omitempty"`

	// Message provides additional detail.
	// +optional
	Message string `json:"message,omitempty"`
}

// AutomatedProceduresConfigSpec defines the desired state of AutomatedProceduresConfig.
type AutomatedProceduresConfigSpec struct {
	// Enabled is the master switch for automated procedures.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// ClusterPriority determines the shutdown delay tier for this cluster.
	// +kubebuilder:validation:Enum=Critical;Standard;BestEffort
	// +kubebuilder:default=Standard
	// +optional
	ClusterPriority ClusterPriority `json:"clusterPriority,omitempty"`

	// BatteryCapacityMinutes is the expected UPS battery duration.
	// +kubebuilder:default=15
	// +kubebuilder:validation:Minimum=1
	// +optional
	BatteryCapacityMinutes int32 `json:"batteryCapacityMinutes,omitempty"`

	// PowerConfirmationSeconds is how long to wait before confirming a power outage.
	// +kubebuilder:default=120
	// +kubebuilder:validation:Minimum=10
	// +optional
	PowerConfirmationSeconds int32 `json:"powerConfirmationSeconds,omitempty"`

	// AutonomousShutdown when true automatically shuts down the cluster
	// after the confirmation period + priority delay without human intervention.
	// +kubebuilder:default=false
	// +optional
	AutonomousShutdown bool `json:"autonomousShutdown,omitempty"`

	// ShutdownDelayOverrideSeconds overrides the tier-based shutdown delay.
	// +optional
	ShutdownDelayOverrideSeconds *int32 `json:"shutdownDelayOverrideSeconds,omitempty"`

	// BackupConfig defines backup storage for emergency backups.
	// +kubebuilder:validation:Required
	BackupConfig BackupConfig `json:"backupConfig"`

	// AlertEndpoint configures the PSU alert webhook receiver.
	// +optional
	AlertEndpoint *AlertEndpointConfig `json:"alertEndpoint,omitempty"`

	// EventSources configures how power and infrastructure events are detected.
	// +optional
	EventSources *EventSourcesConfig `json:"eventSources,omitempty"`

	// ManagementURL is the URL of the UPS/PDU management interface.
	// +optional
	ManagementURL string `json:"managementURL,omitempty"`

	// Notification configures notification channels.
	// +optional
	Notification *NotificationConfig `json:"notification,omitempty"`
}

// AutomatedProceduresConfigStatus defines the observed state of AutomatedProceduresConfig.
type AutomatedProceduresConfigStatus struct {
	// AlertEndpointReady indicates the alert webhook server is listening.
	AlertEndpointReady bool `json:"alertEndpointReady,omitempty"`

	// PowerStatus is the current power state.
	// +kubebuilder:validation:Enum=Normal;OnBattery;Unknown
	PowerStatus PowerStatus `json:"powerStatus,omitempty"`

	// LastPowerEvent is the timestamp of the last power event.
	LastPowerEvent *metav1.Time `json:"lastPowerEvent,omitempty"`

	// ActiveEventRef is the name of the current active PowerEvent.
	ActiveEventRef string `json:"activeEventRef,omitempty"`

	// EventSources reports the status of each configured event source.
	// +optional
	EventSources []EventSourceStatus `json:"eventSources,omitempty"`

	// Conditions represent the latest available observations.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Priority",type=string,JSONPath=`.spec.clusterPriority`
// +kubebuilder:printcolumn:name="Power",type=string,JSONPath=`.status.powerStatus`
// +kubebuilder:printcolumn:name="Autonomous",type=boolean,JSONPath=`.spec.autonomousShutdown`
// +kubebuilder:printcolumn:name="Enabled",type=boolean,JSONPath=`.spec.enabled`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AutomatedProceduresConfig defines automated event response for the cluster.
type AutomatedProceduresConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutomatedProceduresConfigSpec   `json:"spec,omitempty"`
	Status AutomatedProceduresConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AutomatedProceduresConfigList contains a list of AutomatedProceduresConfig.
type AutomatedProceduresConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AutomatedProceduresConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AutomatedProceduresConfig{}, &AutomatedProceduresConfigList{})
}
