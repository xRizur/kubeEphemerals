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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnvironmentTemplateSpec defines the desired state of EnvironmentTemplate.
// Templates provide reusable configurations for ephemeral environments,
// allowing users to create environments from pre-configured setups.
type EnvironmentTemplateSpec struct {
	// DisplayName is the human-readable name shown in the UI
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=100
	DisplayName string `json:"displayName"`

	// Description provides details about what this template deploys
	// +kubebuilder:validation:MaxLength=500
	// +optional
	Description string `json:"description,omitempty"`

	// Icon is an emoji or icon identifier for the UI
	// +kubebuilder:validation:MaxLength=10
	// +optional
	Icon string `json:"icon,omitempty"`

	// Tags are labels for filtering/categorization
	// +optional
	Tags []string `json:"tags,omitempty"`

	// Components lists the Helm charts that will be deployed.
	// At least one component is required.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Components []ComponentSpec `json:"components"`

	// DefaultTTL is the default time-to-live for environments created from this template
	// +kubebuilder:validation:Pattern=`^([0-9]+h)?([0-9]+m)?([0-9]+s)?$`
	// +kubebuilder:default="1h"
	// +optional
	DefaultTTL string `json:"defaultTTL,omitempty"`

	// DefaultGateway provides default gateway configuration
	// +optional
	DefaultGateway *GatewayDefaults `json:"defaultGateway,omitempty"`
}

// ComponentSpec defines a single component (Helm chart) within a template.
// Each component represents one Helm chart to be deployed.
type ComponentSpec struct {
	// Name is a human-readable name for this component
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Repository is the Helm chart repository URL
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https?://.*`
	Repository string `json:"repository"`

	// Chart is the name of the Helm chart
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Chart string `json:"chart"`

	// Version is the chart version to deploy
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[0-9]+\.[0-9]+\.[0-9]+.*$`
	Version string `json:"version"`

	// ServiceName is the name of the Kubernetes service created by this chart
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ServiceName string `json:"serviceName"`

	// ServicePort is the port exposed by the service
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	ServicePort int32 `json:"servicePort"`

	// Values contains default Helm values for this component
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Values *apiextensionsv1.JSON `json:"values,omitempty"`

	// Primary indicates if this is the main component (used for routing).
	// Exactly one component must be marked as primary.
	// +kubebuilder:default=false
	// +optional
	Primary bool `json:"primary,omitempty"`
}

// GatewayDefaults provides default gateway configuration for templates
type GatewayDefaults struct {
	// Name is the default gateway name
	// +kubebuilder:validation:MinLength=1
	// +optional
	Name string `json:"name,omitempty"`

	// Namespace is the default gateway namespace
	// +kubebuilder:validation:MinLength=1
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// EnvironmentTemplateStatus defines the observed state of EnvironmentTemplate
type EnvironmentTemplateStatus struct {
	// UsageCount tracks how many environments have been created from this template
	// +optional
	UsageCount int32 `json:"usageCount,omitempty"`

	// LastUsedTime is when this template was last used to create an environment
	// +optional
	LastUsedTime *metav1.Time `json:"lastUsedTime,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=etpl
// +kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
// +kubebuilder:printcolumn:name="Components",type=integer,JSONPath=`.spec.components | length`
// +kubebuilder:printcolumn:name="Default TTL",type=string,JSONPath=`.spec.defaultTTL`
// +kubebuilder:printcolumn:name="Usage",type=integer,JSONPath=`.status.usageCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// EnvironmentTemplate is the Schema for the environmenttemplates API.
// It defines reusable configurations for creating ephemeral environments.
type EnvironmentTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnvironmentTemplateSpec   `json:"spec,omitempty"`
	Status EnvironmentTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EnvironmentTemplateList contains a list of EnvironmentTemplate
type EnvironmentTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EnvironmentTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EnvironmentTemplate{}, &EnvironmentTemplateList{})
}
