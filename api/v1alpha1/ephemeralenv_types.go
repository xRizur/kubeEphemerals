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

// EphemeralEnvSpec defines the desired state of EphemeralEnv
type EphemeralEnvSpec struct {
	// Owner is the user identity that owns this environment (from X-Forwarded-User / auth).
	// Used for multi-tenant isolation: list/delete are filtered by owner unless user is admin.
	// +optional
	Owner string `json:"owner,omitempty"`

	// Helm contains the Helm chart deployment configuration for single-chart deployments.
	// DEPRECATED: Use Components for new deployments. This field is kept for backward compatibility.
	// When both Helm and Components are specified, Components takes precedence.
	// +optional
	Helm *HelmSpec `json:"helm,omitempty"`

	// Components lists multiple Helm charts to deploy in this environment.
	// Each component is deployed as a separate Helm release.
	// At least one component must be marked as Primary for routing.
	// +optional
	Components []DeployedComponentSpec `json:"components,omitempty"`

	// TemplateRef references an EnvironmentTemplate to use as the base configuration.
	// When specified, the template's components will be used.
	// +optional
	TemplateRef *TemplateReference `json:"templateRef,omitempty"`

	// TTL is the time-to-live duration for the ephemeral environment.
	// After this duration, the environment will be automatically deleted.
	// Format: Go duration string (e.g., "2h", "30m", "24h")
	// +kubebuilder:validation:Pattern=`^([0-9]+h)?([0-9]+m)?([0-9]+s)?$`
	// +kubebuilder:default="24h"
	// +optional
	TTL string `json:"ttl,omitempty"`

	// Isolation enables NetworkPolicy to isolate the environment.
	// When true, a deny-all NetworkPolicy is created in the namespace.
	// +kubebuilder:default=true
	// +optional
	Isolation *bool `json:"isolation,omitempty"`

	// ServicePort is the application port used for port-based service discovery.
	// When set, the operator discovers the backend Service in the environment namespace
	// by finding a Service that exposes this port (ignoring Headless, ExternalName, and metrics services).
	// The discovered service name is used for HTTPRoute instead of Gateway.ServiceName.
	// When not set, Gateway.ServiceName is used directly.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	ServicePort *int32 `json:"servicePort,omitempty"`

	// Gateway configures the Gateway API HTTPRoute for external access
	// +kubebuilder:validation:Required
	Gateway GatewaySpec `json:"gateway"`
}

// DeployedComponentSpec defines a component to be deployed as part of the environment.
// This is similar to ComponentSpec from EnvironmentTemplate but with additional runtime fields.
type DeployedComponentSpec struct {
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

	// Values contains Helm values to override chart defaults.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Values *apiextensionsv1.JSON `json:"values,omitempty"`

	// Primary indicates if this is the main component (used for routing).
	// Exactly one component must be marked as primary.
	// +kubebuilder:default=false
	// +optional
	Primary bool `json:"primary,omitempty"`
}

// TemplateReference references an EnvironmentTemplate
type TemplateReference struct {
	// Name is the name of the EnvironmentTemplate
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace is the namespace where the template resides.
	// If empty, uses the same namespace as the EphemeralEnv.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// HelmSpec defines the Helm chart configuration
type HelmSpec struct {
	// Repository is the Helm chart repository URL
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https?://.*`
	Repository string `json:"repository"`

	// Chart is the name of the Helm chart
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Chart string `json:"chart"`

	// Version is the specific version of the Helm chart to deploy
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[0-9]+\.[0-9]+\.[0-9]+.*$`
	Version string `json:"version"`

	// Values contains Helm values to override chart defaults.
	// Supports any YAML structure including nested objects, arrays, integers, etc.
	// These are passed to Helm during install/upgrade.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Values *apiextensionsv1.JSON `json:"values,omitempty"`
}

// GatewaySpec defines the Gateway API configuration for routing
type GatewaySpec struct {
	// Name is the name of the parent Gateway resource
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace is the namespace where the Gateway resides
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`

	// DomainPrefix is the subdomain prefix for the HTTPRoute.
	// The full URL will be: https://{domainPrefix}.{gateway-domain}
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
	DomainPrefix string `json:"domainPrefix"`

	// ServiceName is the name of the backend service to route traffic to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ServiceName string `json:"serviceName"`

	// TargetPort is the port of the backend service
	// +kubebuilder:default=80
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	TargetPort int32 `json:"targetPort,omitempty"`
}

// EphemeralEnvPhase represents the lifecycle phase of the EphemeralEnv
// +kubebuilder:validation:Enum=Pending;Active;Failed;Expired
type EphemeralEnvPhase string

const (
	// PhasePending indicates the environment is being provisioned
	PhasePending EphemeralEnvPhase = "Pending"

	// PhaseActive indicates the environment is running and accessible
	PhaseActive EphemeralEnvPhase = "Active"

	// PhaseFailed indicates the environment failed to provision
	PhaseFailed EphemeralEnvPhase = "Failed"

	// PhaseExpired indicates the TTL has been exceeded and cleanup is in progress
	PhaseExpired EphemeralEnvPhase = "Expired"
)

// EphemeralEnvStatus defines the observed state of EphemeralEnv
type EphemeralEnvStatus struct {
	// Phase represents the current lifecycle phase of the EphemeralEnv
	// +optional
	Phase EphemeralEnvPhase `json:"phase,omitempty"`

	// ActiveNamespace is the name of the created namespace for this environment.
	// Format: env-{cr-name}
	// +optional
	ActiveNamespace string `json:"activeNamespace,omitempty"`

	// AccessURL is the full URL to access the deployed application
	// +optional
	AccessURL string `json:"accessURL,omitempty"`

	// AdminURL is the URL to access the admin dashboard for this environment
	// +optional
	AdminURL string `json:"adminURL,omitempty"`

	// ExpirationTime is the timestamp when the environment will be deleted
	// +optional
	ExpirationTime *metav1.Time `json:"expirationTime,omitempty"`

	// HelmRelease contains the name of the Helm release (for single-chart deployments)
	// DEPRECATED: Use ComponentStatuses for multi-chart deployments
	// +optional
	HelmRelease string `json:"helmRelease,omitempty"`

	// ComponentStatuses tracks the status of each deployed component
	// +optional
	ComponentStatuses []ComponentStatus `json:"componentStatuses,omitempty"`

	// LastReconcileTime is the timestamp of the last successful reconciliation
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// Message provides human-readable status information
	// +optional
	Message string `json:"message,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	// Standard condition types:
	// - NamespaceReady: The namespace has been created
	// - NetworkPolicyApplied: The NetworkPolicy has been applied (if isolation=true)
	// - HelmDeployed: The Helm chart has been deployed successfully
	// - HTTPRouteReady: The HTTPRoute has been created and accepted
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ComponentStatus tracks the deployment status of a single component
type ComponentStatus struct {
	// Name is the component name (matching DeployedComponentSpec.Name)
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// ReleaseName is the Helm release name for this component
	// +optional
	ReleaseName string `json:"releaseName,omitempty"`

	// Status is the deployment status of this component
	// +optional
	Status string `json:"status,omitempty"`

	// LastDeployed is when the component was last deployed
	// +optional
	LastDeployed *metav1.Time `json:"lastDeployed,omitempty"`

	// Message contains any status message for this component
	// +optional
	Message string `json:"message,omitempty"`
}

// Condition types for EphemeralEnv
const (
	// ConditionTypeNamespaceReady indicates the namespace is created and ready
	ConditionTypeNamespaceReady = "NamespaceReady"

	// ConditionTypeNetworkPolicyApplied indicates the NetworkPolicy is applied
	ConditionTypeNetworkPolicyApplied = "NetworkPolicyApplied"

	// ConditionTypeHelmDeployed indicates the Helm release is deployed
	ConditionTypeHelmDeployed = "HelmDeployed"

	// ConditionTypeHTTPRouteReady indicates the HTTPRoute is created and accepted
	ConditionTypeHTTPRouteReady = "HTTPRouteReady"

	// ConditionTypeEnvironmentExpired indicates the TTL has expired
	ConditionTypeEnvironmentExpired = "EnvironmentExpired"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=eenv
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.accessURL`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.status.activeNamespace`
// +kubebuilder:printcolumn:name="Expires",type=date,JSONPath=`.status.expirationTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// EphemeralEnv is the Schema for the ephemeralenvs API.
// It manages short-lived environments for Pull Requests, providing
// automatic namespace creation, Helm chart deployment, network isolation,
// and Gateway API routing.
type EphemeralEnv struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EphemeralEnvSpec   `json:"spec,omitempty"`
	Status EphemeralEnvStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EphemeralEnvList contains a list of EphemeralEnv
type EphemeralEnvList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []EphemeralEnv `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EphemeralEnv{}, &EphemeralEnvList{})
}
