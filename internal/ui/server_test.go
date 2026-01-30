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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ephemeralv1alpha1 "github.com/maciekmm/kubeEphemerals/api/v1alpha1"
)

func TestUI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UI Server Suite")
}

var _ = Describe("UI Server", func() {
	var (
		server *Server
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(ephemeralv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		// Create fake client with status subresource support
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&ephemeralv1alpha1.EphemeralEnv{}, &ephemeralv1alpha1.EnvironmentTemplate{}).
			Build()

		// Create server
		cfg := DefaultConfig()
		var err error
		server, err = NewServer(cfg, fakeClient, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("NewServer", func() {
		It("should create a server with default config", func() {
			Expect(server).NotTo(BeNil())
			Expect(server.config.PlatformDomain).To(Equal("platform.local"))
			Expect(server.config.AdminPrefix).To(Equal("admin"))
			Expect(server.config.BaseDomain).To(Equal("preview.example.com"))
		})

		It("should parse templates successfully", func() {
			Expect(server.templates).NotTo(BeNil())
		})

		It("should setup router", func() {
			Expect(server.router).NotTo(BeNil())
		})
	})

	Describe("Global Dashboard API", func() {
		Context("GET /api/envs", func() {
			It("should return empty list when no environments exist", func() {
				req := httptest.NewRequest("GET", "/api/envs", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))
				Expect(rec.Header().Get("Content-Type")).To(Equal("application/json"))

				var envs []ephemeralv1alpha1.EphemeralEnv
				Expect(json.NewDecoder(rec.Body).Decode(&envs)).To(Succeed())
				Expect(envs).To(BeEmpty())
			})

			It("should return environments when they exist", func() {
				// Create environment first
				env := &ephemeralv1alpha1.EphemeralEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-env",
						Namespace: "default",
					},
					Spec: ephemeralv1alpha1.EphemeralEnvSpec{
						TTL: "1h",
					},
				}
				Expect(server.client.Create(context.Background(), env)).To(Succeed())

				req := httptest.NewRequest("GET", "/api/envs", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var envs []ephemeralv1alpha1.EphemeralEnv
				Expect(json.NewDecoder(rec.Body).Decode(&envs)).To(Succeed())
				Expect(envs).To(HaveLen(1))
				Expect(envs[0].Name).To(Equal("test-env"))
			})
		})

		Context("POST /api/envs", func() {
			It("should create a new environment", func() {
				envJSON := `{
					"metadata": {
						"name": "pr-123",
						"namespace": "default"
					},
					"spec": {
						"ttl": "2h"
					}
				}`

				req := httptest.NewRequest("POST", "/api/envs", strings.NewReader(envJSON))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusCreated))

				var env ephemeralv1alpha1.EphemeralEnv
				Expect(json.NewDecoder(rec.Body).Decode(&env)).To(Succeed())
				Expect(env.Name).To(Equal("pr-123"))
				Expect(env.Spec.TTL).To(Equal("2h"))
			})

			It("should return error for invalid JSON", func() {
				req := httptest.NewRequest("POST", "/api/envs", strings.NewReader("invalid"))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusBadRequest))
			})
		})

		Context("DELETE /api/envs/{name}", func() {
			It("should delete an existing environment", func() {
				// Create environment first
				env := &ephemeralv1alpha1.EphemeralEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "to-delete",
						Namespace: "default",
					},
					Spec: ephemeralv1alpha1.EphemeralEnvSpec{
						TTL: "1h",
					},
				}
				Expect(server.client.Create(context.Background(), env)).To(Succeed())

				req := httptest.NewRequest("DELETE", "/api/envs/to-delete", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusNoContent))
			})
		})

		Context("GET /api/envs/{name}", func() {
			It("should return environment by name", func() {
				// Create environment first
				expTime := metav1.NewTime(time.Now().Add(1 * time.Hour))
				env := &ephemeralv1alpha1.EphemeralEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "get-test",
						Namespace: "default",
					},
					Spec: ephemeralv1alpha1.EphemeralEnvSpec{
						TTL: "1h",
					},
					Status: ephemeralv1alpha1.EphemeralEnvStatus{
						Phase:           ephemeralv1alpha1.PhaseActive,
						ActiveNamespace: "env-get-test",
						ExpirationTime:  &expTime,
					},
				}
				Expect(server.client.Create(context.Background(), env)).To(Succeed())

				req := httptest.NewRequest("GET", "/api/envs/get-test", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var result ephemeralv1alpha1.EphemeralEnv
				Expect(json.NewDecoder(rec.Body).Decode(&result)).To(Succeed())
				Expect(result.Name).To(Equal("get-test"))
				Expect(result.Status.Phase).To(Equal(ephemeralv1alpha1.PhaseActive))
			})

			It("should return 404 for non-existent environment", func() {
				req := httptest.NewRequest("GET", "/api/envs/nonexistent", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusNotFound))
			})
		})
	})

	Describe("Global Dashboard HTML", func() {
		Context("GET /", func() {
			It("should render global dashboard HTML", func() {
				req := httptest.NewRequest("GET", "/", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))
				Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("text/html"))

				body, err := io.ReadAll(rec.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(body)).To(ContainSubstring("Ephemeral Operator"))
				Expect(string(body)).To(ContainSubstring("Ephemeral Environments"))
			})

			It("should show environments in table", func() {
				// Create environment
				env := &ephemeralv1alpha1.EphemeralEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "html-test",
						Namespace: "default",
					},
					Spec: ephemeralv1alpha1.EphemeralEnvSpec{
						TTL: "1h",
					},
					Status: ephemeralv1alpha1.EphemeralEnvStatus{
						Phase: ephemeralv1alpha1.PhaseActive,
					},
				}
				Expect(server.client.Create(context.Background(), env)).To(Succeed())

				req := httptest.NewRequest("GET", "/", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				body, _ := io.ReadAll(rec.Body)
				Expect(string(body)).To(ContainSubstring("html-test"))
			})
		})
	})

	Describe("Environment Dashboard", func() {
		Context("GET /env/{name}", func() {
			It("should render environment dashboard", func() {
				// Create environment
				env := &ephemeralv1alpha1.EphemeralEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "env-detail",
						Namespace: "default",
					},
					Spec: ephemeralv1alpha1.EphemeralEnvSpec{
						TTL: "1h",
						Helm: &ephemeralv1alpha1.HelmSpec{
							Repository: "https://charts.example.com",
							Chart:      "myapp",
							Version:    "1.0.0",
						},
						Gateway: ephemeralv1alpha1.GatewaySpec{
							Name:         "test-gateway",
							Namespace:    "gateway-system",
							DomainPrefix: "env-detail",
							ServiceName:  "myapp",
							TargetPort:   80,
						},
					},
					Status: ephemeralv1alpha1.EphemeralEnvStatus{
						Phase:           ephemeralv1alpha1.PhaseActive,
						ActiveNamespace: "env-env-detail",
					},
				}
				Expect(server.client.Create(context.Background(), env)).To(Succeed())

				req := httptest.NewRequest("GET", "/env/env-detail", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				body, _ := io.ReadAll(rec.Body)
				Expect(string(body)).To(ContainSubstring("env-detail"))
				Expect(string(body)).To(ContainSubstring("tab-overview")) // Check for tab ID
				Expect(string(body)).To(ContainSubstring("tab-pods"))
				Expect(string(body)).To(ContainSubstring("tab-logs"))
				Expect(string(body)).To(ContainSubstring("tab-config"))
			})

			It("should return 404 for non-existent environment", func() {
				req := httptest.NewRequest("GET", "/env/nonexistent", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("POST /api/envs/{name}/extend-ttl", func() {
			It("should extend TTL for environment", func() {
				// Create environment with expiration
				expTime := metav1.NewTime(time.Now().Add(30 * time.Minute))
				env := &ephemeralv1alpha1.EphemeralEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "extend-test",
						Namespace: "default",
					},
					Spec: ephemeralv1alpha1.EphemeralEnvSpec{
						TTL: "1h",
					},
					Status: ephemeralv1alpha1.EphemeralEnvStatus{
						Phase:          ephemeralv1alpha1.PhaseActive,
						ExpirationTime: &expTime,
					},
				}
				Expect(server.client.Create(context.Background(), env)).To(Succeed())

				reqBody := `{"duration": "1h"}`
				req := httptest.NewRequest("POST", "/api/envs/extend-test/extend-ttl", strings.NewReader(reqBody))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var result map[string]any
				Expect(json.NewDecoder(rec.Body).Decode(&result)).To(Succeed())
				Expect(result["status"]).To(Equal("extended"))
			})

			It("should return error for invalid duration", func() {
				// Create environment
				env := &ephemeralv1alpha1.EphemeralEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-dur",
						Namespace: "default",
					},
				}
				Expect(server.client.Create(context.Background(), env)).To(Succeed())

				reqBody := `{"duration": "invalid"}`
				req := httptest.NewRequest("POST", "/api/envs/invalid-dur/extend-ttl", strings.NewReader(reqBody))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusBadRequest))
			})
		})

		Context("GET /api/envs/{name}/pods", func() {
			It("should return pods for environment", func() {
				// Create environment
				env := &ephemeralv1alpha1.EphemeralEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pods-test",
						Namespace: "default",
					},
					Status: ephemeralv1alpha1.EphemeralEnvStatus{
						ActiveNamespace: "env-pods-test",
					},
				}
				Expect(server.client.Create(context.Background(), env)).To(Succeed())

				req := httptest.NewRequest("GET", "/api/envs/pods-test/pods", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var pods []map[string]any
				Expect(json.NewDecoder(rec.Body).Decode(&pods)).To(Succeed())
				// Empty because no pods exist in fake client
				Expect(pods).To(BeEmpty())
			})
		})
	})

	Describe("Host-based Routing", func() {
		It("should route platform host to global dashboard", func() {
			req := httptest.NewRequest("GET", "/", nil)
			req.Host = "platform.local"
			rec := httptest.NewRecorder()

			server.router.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			body, _ := io.ReadAll(rec.Body)
			Expect(string(body)).To(ContainSubstring("Ephemeral Environments"))
		})

		It("should extract env name from admin host", func() {
			// Create environment
			env := &ephemeralv1alpha1.EphemeralEnv{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pr-456",
					Namespace: "default",
				},
				Status: ephemeralv1alpha1.EphemeralEnvStatus{
					Phase:           ephemeralv1alpha1.PhaseActive,
					ActiveNamespace: "env-pr-456",
				},
			}
			Expect(server.client.Create(context.Background(), env)).To(Succeed())

			req := httptest.NewRequest("GET", "/", nil)
			req.Host = "admin.pr-456.preview.example.com"
			rec := httptest.NewRecorder()

			server.router.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			body, _ := io.ReadAll(rec.Body)
			Expect(string(body)).To(ContainSubstring("pr-456"))
		})
	})

	Describe("Template Functions", func() {
		It("should format time until correctly", func() {
			funcs := server.templateFuncs()
			timeUntil := funcs["timeUntil"].(func(*metav1.Time) string)

			// Test nil
			Expect(timeUntil(nil)).To(Equal("N/A"))

			// Test future time
			future := metav1.NewTime(time.Now().Add(1 * time.Hour))
			result := timeUntil(&future)
			Expect(result).To(ContainSubstring("m"))

			// Test past time
			past := metav1.NewTime(time.Now().Add(-1 * time.Hour))
			Expect(timeUntil(&past)).To(Equal("Expired"))
		})

		It("should return correct status class", func() {
			funcs := server.templateFuncs()
			statusClass := funcs["statusClass"].(func(ephemeralv1alpha1.EphemeralEnvPhase) string)

			Expect(statusClass(ephemeralv1alpha1.PhaseActive)).To(Equal("status-active"))
			Expect(statusClass(ephemeralv1alpha1.PhasePending)).To(Equal("status-pending"))
			Expect(statusClass(ephemeralv1alpha1.PhaseFailed)).To(Equal("status-failed"))
			Expect(statusClass(ephemeralv1alpha1.PhaseExpired)).To(Equal("status-expired"))
			Expect(statusClass("Unknown")).To(Equal("status-unknown"))
		})
	})

	Describe("SanitizeEnvName", func() {
		It("should convert to lowercase", func() {
			Expect(SanitizeEnvName("PR-123")).To(Equal("pr-123"))
		})

		It("should replace underscores with dashes", func() {
			Expect(SanitizeEnvName("feature_branch")).To(Equal("feature-branch"))
		})
	})

	// =============================================================================
	// Phase 8.2: Templates API Tests (TDD) - Now testing CRD-based templates
	// =============================================================================
	Describe("Templates API", func() {
		// Helper function to create test templates
		createTestTemplates := func() {
			ctx := context.Background()

			// Create fullstack template
			fullstack := &ephemeralv1alpha1.EnvironmentTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fullstack",
					Namespace: "default",
				},
				Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
					DisplayName: "Full Stack",
					Description: "Complete web application stack",
					Icon:        "🚀",
					DefaultTTL:  "2h",
					Tags:        []string{"web", "database"},
					Components: []ephemeralv1alpha1.ComponentSpec{
						{
							Name:        "Web Server",
							Repository:  "https://charts.bitnami.com/bitnami",
							Chart:       "nginx",
							Version:     "18.2.5",
							ServiceName: "nginx",
							ServicePort: 80,
							Primary:     true,
						},
						{
							Name:        "Database",
							Repository:  "https://charts.bitnami.com/bitnami",
							Chart:       "postgresql",
							Version:     "16.2.5",
							ServiceName: "postgresql",
							ServicePort: 5432,
							Primary:     false,
						},
					},
				},
			}
			Expect(server.client.Create(ctx, fullstack)).To(Succeed())

			// Create frontend template
			frontend := &ephemeralv1alpha1.EnvironmentTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "frontend",
					Namespace: "default",
				},
				Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
					DisplayName: "Frontend Only",
					Description: "Lightweight web server",
					Icon:        "🌐",
					DefaultTTL:  "1h",
					Tags:        []string{"web", "static"},
					Components: []ephemeralv1alpha1.ComponentSpec{
						{
							Name:        "Web Server",
							Repository:  "https://charts.bitnami.com/bitnami",
							Chart:       "nginx",
							Version:     "18.2.5",
							ServiceName: "nginx",
							ServicePort: 80,
							Primary:     true,
						},
					},
				},
			}
			Expect(server.client.Create(ctx, frontend)).To(Succeed())

			// Create podinfo template
			podinfo := &ephemeralv1alpha1.EnvironmentTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "podinfo",
					Namespace: "default",
				},
				Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
					DisplayName: "Podinfo Demo",
					Description: "Lightweight Go microservice for testing",
					Icon:        "🧪",
					DefaultTTL:  "30m",
					Tags:        []string{"demo", "testing"},
					Components: []ephemeralv1alpha1.ComponentSpec{
						{
							Name:        "Podinfo",
							Repository:  "https://stefanprodan.github.io/podinfo",
							Chart:       "podinfo",
							Version:     "6.7.1",
							ServiceName: "podinfo",
							ServicePort: 9898,
							Primary:     true,
						},
					},
				},
			}
			Expect(server.client.Create(ctx, podinfo)).To(Succeed())

			// Create redis template
			redis := &ephemeralv1alpha1.EnvironmentTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "redis",
					Namespace: "default",
				},
				Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
					DisplayName: "Redis Cache",
					Description: "In-memory data store",
					Icon:        "⚡",
					DefaultTTL:  "1h",
					Tags:        []string{"cache", "database"},
					Components: []ephemeralv1alpha1.ComponentSpec{
						{
							Name:        "Redis",
							Repository:  "https://charts.bitnami.com/bitnami",
							Chart:       "redis",
							Version:     "20.3.1",
							ServiceName: "redis-master",
							ServicePort: 6379,
							Primary:     true,
						},
					},
				},
			}
			Expect(server.client.Create(ctx, redis)).To(Succeed())
		}

		Context("GET /api/templates", func() {
			BeforeEach(func() {
				createTestTemplates()
			})

			It("should return list of available templates from CRD", func() {
				req := httptest.NewRequest("GET", "/api/templates", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))
				Expect(rec.Header().Get("Content-Type")).To(Equal("application/json"))

				var templates []TemplateAPIResponse
				Expect(json.NewDecoder(rec.Body).Decode(&templates)).To(Succeed())
				Expect(templates).NotTo(BeEmpty())
			})

			It("should include fullstack template from CRD", func() {
				req := httptest.NewRequest("GET", "/api/templates", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var templates []TemplateAPIResponse
				Expect(json.NewDecoder(rec.Body).Decode(&templates)).To(Succeed())

				var found bool
				for _, t := range templates {
					if t.ID == "fullstack" {
						found = true
						Expect(t.DisplayName).To(Equal("Full Stack"))
						Expect(t.Components).NotTo(BeEmpty())
						Expect(t.DefaultTTL).NotTo(BeEmpty())
						break
					}
				}
				Expect(found).To(BeTrue(), "fullstack template should exist")
			})

			It("should include frontend template from CRD", func() {
				req := httptest.NewRequest("GET", "/api/templates", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var templates []TemplateAPIResponse
				Expect(json.NewDecoder(rec.Body).Decode(&templates)).To(Succeed())

				var found bool
				for _, t := range templates {
					if t.ID == "frontend" {
						found = true
						Expect(t.DisplayName).To(Equal("Frontend Only"))
						Expect(t.Components).To(HaveLen(1))
						break
					}
				}
				Expect(found).To(BeTrue(), "frontend template should exist")
			})

			It("should have at least 4 templates when created", func() {
				req := httptest.NewRequest("GET", "/api/templates", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var templates []TemplateAPIResponse
				Expect(json.NewDecoder(rec.Body).Decode(&templates)).To(Succeed())
				Expect(len(templates)).To(BeNumerically(">=", 4))
			})

			It("should return empty list when no templates exist", func() {
				// Create a fresh server without templates
				freshScheme := runtime.NewScheme()
				Expect(ephemeralv1alpha1.AddToScheme(freshScheme)).To(Succeed())
				Expect(corev1.AddToScheme(freshScheme)).To(Succeed())

				freshClient := fake.NewClientBuilder().
					WithScheme(freshScheme).
					WithStatusSubresource(&ephemeralv1alpha1.EphemeralEnv{}, &ephemeralv1alpha1.EnvironmentTemplate{}).
					Build()

				cfg := DefaultConfig()
				freshServer, err := NewServer(cfg, freshClient, nil)
				Expect(err).NotTo(HaveOccurred())

				req := httptest.NewRequest("GET", "/api/templates", nil)
				rec := httptest.NewRecorder()

				freshServer.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var templates []TemplateAPIResponse
				Expect(json.NewDecoder(rec.Body).Decode(&templates)).To(Succeed())
				Expect(templates).To(BeEmpty())
			})

			It("should filter templates by namespace", func() {
				// Create template in different namespace
				ctx := context.Background()
				otherTemplate := &ephemeralv1alpha1.EnvironmentTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-template",
						Namespace: "other-namespace",
					},
					Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
						DisplayName: "Other Template",
						Components: []ephemeralv1alpha1.ComponentSpec{
							{
								Name:        "Test",
								Repository:  "https://example.com/charts",
								Chart:       "test",
								Version:     "1.0.0",
								ServiceName: "test",
								ServicePort: 80,
								Primary:     true,
							},
						},
					},
				}
				Expect(server.client.Create(ctx, otherTemplate)).To(Succeed())

				// Filter by default namespace
				req := httptest.NewRequest("GET", "/api/templates?namespace=default", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var templates []TemplateAPIResponse
				Expect(json.NewDecoder(rec.Body).Decode(&templates)).To(Succeed())

				// Should only have templates from default namespace
				for _, t := range templates {
					Expect(t.Namespace).To(Equal("default"))
				}
			})
		})

		Context("GET /api/templates/{id}", func() {
			BeforeEach(func() {
				createTestTemplates()
			})

			It("should return specific template by ID", func() {
				req := httptest.NewRequest("GET", "/api/templates/fullstack", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))
				Expect(rec.Header().Get("Content-Type")).To(Equal("application/json"))

				var template TemplateAPIResponse
				Expect(json.NewDecoder(rec.Body).Decode(&template)).To(Succeed())
				Expect(template.ID).To(Equal("fullstack"))
				Expect(template.DisplayName).To(Equal("Full Stack"))
			})

			It("should return 404 for non-existent template", func() {
				req := httptest.NewRequest("GET", "/api/templates/nonexistent", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusNotFound))
			})

			It("should return podinfo template with correct structure", func() {
				req := httptest.NewRequest("GET", "/api/templates/podinfo", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var template TemplateAPIResponse
				Expect(json.NewDecoder(rec.Body).Decode(&template)).To(Succeed())
				Expect(template.ID).To(Equal("podinfo"))
				Expect(template.Components).To(HaveLen(1))
				Expect(template.Components[0].Chart).To(Equal("podinfo"))
				Expect(template.Components[0].Primary).To(BeTrue())
			})

			It("should return template with namespace filter", func() {
				req := httptest.NewRequest("GET", "/api/templates/fullstack?namespace=default", nil)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var template TemplateAPIResponse
				Expect(json.NewDecoder(rec.Body).Decode(&template)).To(Succeed())
				Expect(template.ID).To(Equal("fullstack"))
				Expect(template.Namespace).To(Equal("default"))
			})
		})
	})

	// =============================================================================
	// TemplateRegistry Unit Tests (CRD-based)
	// =============================================================================
	Describe("TemplateRegistry with CRD", func() {
		var registry *TemplateRegistry

		BeforeEach(func() {
			registry = NewTemplateRegistry()
			registry.SetClient(server.client, "")

			// Create test templates in the CRD
			ctx := context.Background()

			templates := []*ephemeralv1alpha1.EnvironmentTemplate{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "test-fullstack", Namespace: "default"},
					Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
						DisplayName: "Test Full Stack",
						DefaultTTL:  "2h",
						Components: []ephemeralv1alpha1.ComponentSpec{
							{Name: "nginx", Repository: "https://charts.bitnami.com/bitnami", Chart: "nginx", Version: "18.0.0", ServiceName: "nginx", ServicePort: 80, Primary: true},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "test-frontend", Namespace: "default"},
					Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
						DisplayName: "Test Frontend",
						DefaultTTL:  "1h",
						Components: []ephemeralv1alpha1.ComponentSpec{
							{Name: "nginx", Repository: "https://charts.bitnami.com/bitnami", Chart: "nginx", Version: "18.0.0", ServiceName: "nginx", ServicePort: 80, Primary: true},
						},
					},
				},
			}

			for _, t := range templates {
				Expect(server.client.Create(ctx, t)).To(Succeed())
			}
		})

		Context("List()", func() {
			It("should return all templates from CRD", func() {
				templates := registry.List()
				Expect(templates).NotTo(BeEmpty())
				Expect(len(templates)).To(BeNumerically(">=", 2))
			})

			It("should return empty list when client not set", func() {
				emptyRegistry := NewTemplateRegistry()
				templates := emptyRegistry.List()
				Expect(templates).To(BeEmpty())
			})
		})

		Context("Get()", func() {
			It("should return template when exists", func() {
				template, ok := registry.Get("test-fullstack")
				Expect(ok).To(BeTrue())
				Expect(template.ID).To(Equal("test-fullstack"))
				Expect(template.DisplayName).To(Equal("Test Full Stack"))
			})

			It("should return false when template not found", func() {
				_, ok := registry.Get("nonexistent")
				Expect(ok).To(BeFalse())
			})

			It("should return false when client not set", func() {
				emptyRegistry := NewTemplateRegistry()
				_, ok := emptyRegistry.Get("test-fullstack")
				Expect(ok).To(BeFalse())
			})
		})
	})

	// =============================================================================
	// CRD ComponentSpec Validation Tests
	// =============================================================================
	Describe("ComponentSpec Validation in CRD", func() {
		BeforeEach(func() {
			ctx := context.Background()

			// Create a template with components for validation tests
			template := &ephemeralv1alpha1.EnvironmentTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "validation-test-template",
					Namespace: "default",
				},
				Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
					DisplayName: "Validation Test",
					DefaultTTL:  "2h",
					Components: []ephemeralv1alpha1.ComponentSpec{
						{
							Name:        "Primary Component",
							Repository:  "https://charts.bitnami.com/bitnami",
							Chart:       "nginx",
							Version:     "18.0.0",
							ServiceName: "nginx",
							ServicePort: 80,
							Primary:     true,
						},
						{
							Name:        "Secondary Component",
							Repository:  "https://charts.bitnami.com/bitnami",
							Chart:       "postgresql",
							Version:     "16.0.0",
							ServiceName: "postgresql",
							ServicePort: 5432,
							Primary:     false,
						},
					},
				},
			}
			Expect(server.client.Create(ctx, template)).To(Succeed())
		})

		It("should have required fields for primary component", func() {
			ctx := context.Background()

			var template ephemeralv1alpha1.EnvironmentTemplate
			Expect(server.client.Get(ctx, client.ObjectKey{
				Name:      "validation-test-template",
				Namespace: "default",
			}, &template)).To(Succeed())

			var primaryComponent *ephemeralv1alpha1.ComponentSpec
			for i := range template.Spec.Components {
				if template.Spec.Components[i].Primary {
					primaryComponent = &template.Spec.Components[i]
					break
				}
			}

			Expect(primaryComponent).NotTo(BeNil(), "should have a primary component")
			Expect(primaryComponent.Repository).NotTo(BeEmpty())
			Expect(primaryComponent.Chart).NotTo(BeEmpty())
			Expect(primaryComponent.Version).NotTo(BeEmpty())
			Expect(primaryComponent.ServiceName).NotTo(BeEmpty())
			Expect(primaryComponent.ServicePort).To(BeNumerically(">", 0))
		})

		It("should have valid Helm repository URLs", func() {
			ctx := context.Background()

			var templateList ephemeralv1alpha1.EnvironmentTemplateList
			Expect(server.client.List(ctx, &templateList)).To(Succeed())

			for _, t := range templateList.Items {
				for _, c := range t.Spec.Components {
					Expect(c.Repository).To(HavePrefix("https://"), "Repository should use HTTPS: %s", c.Repository)
				}
			}
		})

		It("should have valid service ports", func() {
			ctx := context.Background()

			var template ephemeralv1alpha1.EnvironmentTemplate
			Expect(server.client.Get(ctx, client.ObjectKey{
				Name:      "validation-test-template",
				Namespace: "default",
			}, &template)).To(Succeed())

			for _, c := range template.Spec.Components {
				Expect(c.ServicePort).To(BeNumerically(">=", 1), "Port should be >= 1")
				Expect(c.ServicePort).To(BeNumerically("<=", 65535), "Port should be <= 65535")
			}
		})
	})

	// =============================================================================
	// EnvironmentTemplate CRD Validation Tests
	// =============================================================================
	Describe("EnvironmentTemplate CRD Validation", func() {
		BeforeEach(func() {
			ctx := context.Background()

			templates := []*ephemeralv1alpha1.EnvironmentTemplate{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "crd-test-fullstack", Namespace: "default"},
					Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
						DisplayName: "Full Stack Test",
						DefaultTTL:  "2h",
						Components: []ephemeralv1alpha1.ComponentSpec{
							{Name: "nginx", Repository: "https://charts.bitnami.com/bitnami", Chart: "nginx", Version: "18.0.0", ServiceName: "nginx", ServicePort: 80, Primary: true},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "crd-test-redis", Namespace: "default"},
					Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
						DisplayName: "Redis Test",
						DefaultTTL:  "1h30m",
						Components: []ephemeralv1alpha1.ComponentSpec{
							{Name: "redis", Repository: "https://charts.bitnami.com/bitnami", Chart: "redis", Version: "20.0.0", ServiceName: "redis", ServicePort: 6379, Primary: true},
						},
					},
				},
			}

			for _, t := range templates {
				Expect(server.client.Create(ctx, t)).To(Succeed())
			}
		})

		It("should have valid DefaultTTL format", func() {
			ctx := context.Background()

			var templateList ephemeralv1alpha1.EnvironmentTemplateList
			Expect(server.client.List(ctx, &templateList, client.InNamespace("default"))).To(Succeed())

			validTTLPattern := `^(\d+h)?(\d+m)?(\d+s)?$`
			for _, t := range templateList.Items {
				if t.Spec.DefaultTTL != "" {
					Expect(t.Spec.DefaultTTL).To(MatchRegexp(validTTLPattern), "Template %s has invalid TTL: %s", t.Name, t.Spec.DefaultTTL)
				}
			}
		})

		It("should have non-empty Name and DisplayName", func() {
			ctx := context.Background()

			var templateList ephemeralv1alpha1.EnvironmentTemplateList
			Expect(server.client.List(ctx, &templateList, client.InNamespace("default"))).To(Succeed())

			for _, t := range templateList.Items {
				Expect(t.Name).NotTo(BeEmpty(), "Template Name should not be empty")
				Expect(t.Spec.DisplayName).NotTo(BeEmpty(), "Template DisplayName should not be empty")
			}
		})

		It("should have at least one component", func() {
			ctx := context.Background()

			var templateList ephemeralv1alpha1.EnvironmentTemplateList
			Expect(server.client.List(ctx, &templateList, client.InNamespace("default"))).To(Succeed())

			for _, t := range templateList.Items {
				Expect(t.Spec.Components).NotTo(BeEmpty(), "Template %s should have at least one component", t.Name)
			}
		})

		It("should have exactly one primary component", func() {
			ctx := context.Background()

			var templateList ephemeralv1alpha1.EnvironmentTemplateList
			Expect(server.client.List(ctx, &templateList, client.InNamespace("default"))).To(Succeed())

			for _, t := range templateList.Items {
				primaryCount := 0
				for _, c := range t.Spec.Components {
					if c.Primary {
						primaryCount++
					}
				}
				Expect(primaryCount).To(Equal(1), "Template %s should have exactly one primary component, has %d", t.Name, primaryCount)
			}
		})
	})
})

var _ = Describe("Context Functions", func() {
	It("should return empty string when no env in context", func() {
		ctx := context.Background()
		Expect(getEnvFromContext(ctx)).To(Equal(""))
	})

	It("should return env name when in context", func() {
		ctx := context.WithValue(context.Background(), envContextKey, "test-env")
		Expect(getEnvFromContext(ctx)).To(Equal("test-env"))
	})
})
