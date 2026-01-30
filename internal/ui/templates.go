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

package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"

	"github.com/gorilla/mux"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ephemeralv1alpha1 "github.com/maciekmm/kubeEphemerals/api/v1alpha1"
)

// TemplateAPIResponse represents a template as returned by the API
// This is a simplified view of EnvironmentTemplate CRD for the UI
type TemplateAPIResponse struct {
	// ID is the template's Kubernetes name
	ID string `json:"id"`

	// DisplayName is the human-readable name shown in the UI
	DisplayName string `json:"displayName"`

	// Description provides details about what this template deploys
	Description string `json:"description"`

	// Namespace is where the template CRD is stored
	Namespace string `json:"namespace"`

	// Components lists the Helm charts that will be deployed
	Components []ComponentAPIResponse `json:"components"`

	// DefaultTTL is the default time-to-live for environments created from this template
	DefaultTTL string `json:"defaultTTL"`

	// Icon is an optional emoji or icon identifier for the UI
	Icon string `json:"icon,omitempty"`

	// Tags are optional labels for filtering/categorization
	Tags []string `json:"tags,omitempty"`

	// UsageCount tracks how many environments have been created from this template
	UsageCount int32 `json:"usageCount,omitempty"`
}

// ComponentAPIResponse defines a single component for the API response
type ComponentAPIResponse struct {
	// Name is a human-readable name for this component
	Name string `json:"name"`

	// Repository is the Helm chart repository URL
	Repository string `json:"repository"`

	// Chart is the name of the Helm chart
	Chart string `json:"chart"`

	// Version is the chart version to deploy
	Version string `json:"version"`

	// ServiceName is the name of the Kubernetes service created by this chart
	ServiceName string `json:"serviceName"`

	// ServicePort is the port exposed by the service
	ServicePort int32 `json:"servicePort"`

	// Values contains default Helm values for this component (as raw JSON)
	Values any `json:"values,omitempty"`

	// Primary indicates if this is the main component (used for routing)
	Primary bool `json:"primary,omitempty"`
}

// convertTemplateToAPIResponse converts a CRD EnvironmentTemplate to API response
func convertTemplateToAPIResponse(t *ephemeralv1alpha1.EnvironmentTemplate) TemplateAPIResponse {
	components := make([]ComponentAPIResponse, 0, len(t.Spec.Components))
	for _, c := range t.Spec.Components {
		var values interface{}
		if c.Values != nil && len(c.Values.Raw) > 0 {
			values = c.Values.Raw
		}
		components = append(components, ComponentAPIResponse{
			Name:        c.Name,
			Repository:  c.Repository,
			Chart:       c.Chart,
			Version:     c.Version,
			ServiceName: c.ServiceName,
			ServicePort: c.ServicePort,
			Values:      values,
			Primary:     c.Primary,
		})
	}

	return TemplateAPIResponse{
		ID:          t.Name,
		DisplayName: t.Spec.DisplayName,
		Description: t.Spec.Description,
		Namespace:   t.Namespace,
		Components:  components,
		DefaultTTL:  t.Spec.DefaultTTL,
		Icon:        t.Spec.Icon,
		Tags:        t.Spec.Tags,
		UsageCount:  t.Status.UsageCount,
	}
}

// handleListTemplates is the HTTP handler for GET /api/templates
// It lists all EnvironmentTemplate CRDs across all namespaces
func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Query parameter for namespace filtering
	namespace := r.URL.Query().Get("namespace")

	var templateList ephemeralv1alpha1.EnvironmentTemplateList
	var err error

	if namespace != "" {
		err = s.client.List(ctx, &templateList, client.InNamespace(namespace))
	} else {
		err = s.client.List(ctx, &templateList)
	}

	if err != nil {
		log.Error(err, "Failed to list EnvironmentTemplates")
		s.jsonError(w, "Failed to list templates", http.StatusInternalServerError)
		return
	}

	// Convert to API response format
	templates := make([]TemplateAPIResponse, 0, len(templateList.Items))
	for _, t := range templateList.Items {
		templates = append(templates, convertTemplateToAPIResponse(&t))
	}

	// Sort by display name for consistent ordering
	sort.Slice(templates, func(i, j int) bool {
		return templates[i].DisplayName < templates[j].DisplayName
	})

	s.jsonResponse(w, templates)
}

// handleGetTemplate is the HTTP handler for GET /api/templates/{id}
// The id format can be either "name" (searches all namespaces) or "namespace/name"
func (s *Server) handleGetTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	// Check if namespace is provided in the id
	namespace := r.URL.Query().Get("namespace")

	var template ephemeralv1alpha1.EnvironmentTemplate

	if namespace != "" {
		// Direct lookup with namespace
		err := s.client.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      id,
		}, &template)
		if err != nil {
			s.jsonError(w, "Template not found", http.StatusNotFound)
			return
		}
	} else {
		// Search across all namespaces
		var templateList ephemeralv1alpha1.EnvironmentTemplateList
		err := s.client.List(ctx, &templateList)
		if err != nil {
			log.Error(err, "Failed to list EnvironmentTemplates")
			s.jsonError(w, "Failed to search templates", http.StatusInternalServerError)
			return
		}

		found := false
		for _, t := range templateList.Items {
			if t.Name == id {
				template = t
				found = true
				break
			}
		}

		if !found {
			s.jsonError(w, "Template not found", http.StatusNotFound)
			return
		}
	}

	s.jsonResponse(w, convertTemplateToAPIResponse(&template))
}

// CreateTemplateRequest is the request body for creating a new template
type CreateTemplateRequest struct {
	Name        string                   `json:"name"`
	Namespace   string                   `json:"namespace"`
	DisplayName string                   `json:"displayName"`
	Description string                   `json:"description"`
	Icon        string                   `json:"icon,omitempty"`
	Tags        []string                 `json:"tags,omitempty"`
	DefaultTTL  string                   `json:"defaultTTL"`
	Components  []CreateComponentRequest `json:"components"`
}

// CreateComponentRequest defines a component in the create request
type CreateComponentRequest struct {
	Name        string `json:"name"`
	Repository  string `json:"repository"`
	Chart       string `json:"chart"`
	Version     string `json:"version"`
	ServiceName string `json:"serviceName"`
	ServicePort int32  `json:"servicePort"`
	Primary     bool   `json:"primary,omitempty"`
}

// handleCreateTemplate is the HTTP handler for POST /api/templates
func (s *Server) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		s.jsonError(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Name
	}
	if req.Namespace == "" {
		req.Namespace = "ephemeral-system"
	}
	if req.DefaultTTL == "" {
		req.DefaultTTL = "1h"
	}
	if len(req.Components) == 0 {
		s.jsonError(w, "at least one component is required", http.StatusBadRequest)
		return
	}

	// Convert components
	components := make([]ephemeralv1alpha1.ComponentSpec, 0, len(req.Components))
	for _, c := range req.Components {
		if c.Name == "" || c.Repository == "" || c.Chart == "" {
			s.jsonError(w, "component name, repository, and chart are required", http.StatusBadRequest)
			return
		}
		components = append(components, ephemeralv1alpha1.ComponentSpec{
			Name:        c.Name,
			Repository:  c.Repository,
			Chart:       c.Chart,
			Version:     c.Version,
			ServiceName: c.ServiceName,
			ServicePort: c.ServicePort,
			Primary:     c.Primary,
		})
	}

	// Create the EnvironmentTemplate CRD
	template := &ephemeralv1alpha1.EnvironmentTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
			DisplayName: req.DisplayName,
			Description: req.Description,
			Icon:        req.Icon,
			Tags:        req.Tags,
			DefaultTTL:  req.DefaultTTL,
			Components:  components,
		},
	}

	if err := s.client.Create(ctx, template); err != nil {
		log.Error(err, "Failed to create EnvironmentTemplate", "name", req.Name)
		s.jsonError(w, "Failed to create template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Info("Created EnvironmentTemplate", "name", req.Name, "namespace", req.Namespace)
	w.WriteHeader(http.StatusCreated)
	s.jsonResponse(w, convertTemplateToAPIResponse(template))
}

// handleDeleteTemplate is the HTTP handler for DELETE /api/templates/{id}
func (s *Server) handleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]
	namespace := r.URL.Query().Get("namespace")

	if namespace == "" {
		namespace = "ephemeral-system"
	}

	template := &ephemeralv1alpha1.EnvironmentTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: namespace,
		},
	}

	if err := s.client.Delete(ctx, template); err != nil {
		log.Error(err, "Failed to delete EnvironmentTemplate", "name", id)
		s.jsonError(w, "Failed to delete template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Info("Deleted EnvironmentTemplate", "name", id, "namespace", namespace)
	w.WriteHeader(http.StatusNoContent)
}

// TemplateRegistry is kept for backward compatibility with existing code
// but now acts as a facade over the Kubernetes API
type TemplateRegistry struct {
	client    client.Client
	namespace string
}

// NewTemplateRegistry creates a new registry
// Note: In tests without a real client, this will return an empty registry
func NewTemplateRegistry() *TemplateRegistry {
	return &TemplateRegistry{}
}

// SetClient sets the Kubernetes client for the registry
func (r *TemplateRegistry) SetClient(c client.Client, namespace string) {
	r.client = c
	r.namespace = namespace
}

// List returns all available templates from CRD
func (r *TemplateRegistry) List() []TemplateAPIResponse {
	if r.client == nil {
		return []TemplateAPIResponse{}
	}

	var templateList ephemeralv1alpha1.EnvironmentTemplateList
	var err error

	if r.namespace != "" {
		err = r.client.List(context.Background(), &templateList, client.InNamespace(r.namespace))
	} else {
		err = r.client.List(context.Background(), &templateList)
	}

	if err != nil {
		log.Error(err, "Failed to list EnvironmentTemplates")
		return []TemplateAPIResponse{}
	}

	templates := make([]TemplateAPIResponse, 0, len(templateList.Items))
	for _, t := range templateList.Items {
		templates = append(templates, convertTemplateToAPIResponse(&t))
	}

	return templates
}

// Get returns a specific template by ID
func (r *TemplateRegistry) Get(id string) (TemplateAPIResponse, bool) {
	if r.client == nil {
		return TemplateAPIResponse{}, false
	}

	var templateList ephemeralv1alpha1.EnvironmentTemplateList
	err := r.client.List(context.Background(), &templateList)
	if err != nil {
		return TemplateAPIResponse{}, false
	}

	for _, t := range templateList.Items {
		if t.Name == id {
			return convertTemplateToAPIResponse(&t), true
		}
	}

	return TemplateAPIResponse{}, false
}
