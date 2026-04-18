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

type ExportTargetType string

const (
	ExportTargetPVC ExportTargetType = "PVC"
	ExportTargetS3  ExportTargetType = "S3"
)

// S3Config defines S3-compatible storage configuration.
type S3Config struct {
	// Bucket is the S3 bucket name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Bucket string `json:"bucket"`

	// Region is the AWS region.
	// +kubebuilder:default="us-east-1"
	// +optional
	Region string `json:"region,omitempty"`

	// Endpoint is the S3-compatible endpoint URL (for MinIO, ODF, etc.).
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// CredentialsSecret is the name of a Secret containing "access-key" and "secret-key".
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	CredentialsSecret string `json:"credentialsSecret"`

	// Prefix is a path prefix within the bucket.
	// +optional
	Prefix string `json:"prefix,omitempty"`
}

// ExportTarget defines where backup artifacts are stored.
type ExportTarget struct {
	// Type is PVC or S3.
	// +kubebuilder:validation:Enum=PVC;S3
	// +kubebuilder:default=PVC
	Type ExportTargetType `json:"type"`

	// S3 is required when Type is S3.
	// +optional
	S3 *S3Config `json:"s3,omitempty"`
}

// PreflightStatus summarizes pre-flight check results.
type PreflightStatus struct {
	// DegradedOperators lists ClusterOperators that are Degraded.
	DegradedOperators []string `json:"degradedOperators,omitempty"`

	// UpdateInProgress is true if a cluster version update is in progress.
	UpdateInProgress bool `json:"updateInProgress"`

	// Passed indicates whether all checks passed.
	Passed bool `json:"passed"`

	// Message provides detail about failures.
	Message string `json:"message,omitempty"`
}
