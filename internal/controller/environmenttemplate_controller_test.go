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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ephemeralv1alpha1 "github.com/xrizur/kubeEphemerals/api/v1alpha1"
)

var _ = Describe("EnvironmentTemplate CRD", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When creating an EnvironmentTemplate", func() {
		It("Should create a single-component template successfully", func() {
			ctx := context.Background()

			template := &ephemeralv1alpha1.EnvironmentTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx-template",
					Namespace: "default",
				},
				Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
					DisplayName: "Simple Nginx",
					Description: "A simple nginx deployment for testing",
					Icon:        "🌐",
					Tags:        []string{"web", "proxy"},
					DefaultTTL:  "2h",
					Components: []ephemeralv1alpha1.ComponentSpec{
						{
							Name:        "nginx",
							Repository:  "https://charts.bitnami.com/bitnami",
							Chart:       "nginx",
							Version:     "15.0.0",
							ServiceName: "nginx",
							ServicePort: 80,
							Primary:     true,
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, template)).Should(Succeed())

			// Verify the template was created
			createdTemplate := &ephemeralv1alpha1.EnvironmentTemplate{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "nginx-template",
					Namespace: "default",
				}, createdTemplate)
			}, timeout, interval).Should(Succeed())

			Expect(createdTemplate.Spec.DisplayName).To(Equal("Simple Nginx"))
			Expect(createdTemplate.Spec.Components).To(HaveLen(1))
			Expect(createdTemplate.Spec.Components[0].Primary).To(BeTrue())

			// Cleanup
			Expect(k8sClient.Delete(ctx, template)).Should(Succeed())
		})

		It("Should create a multi-component template successfully", func() {
			ctx := context.Background()

			postgresValues := `{"auth":{"username":"app","database":"mydb"}}`

			template := &ephemeralv1alpha1.EnvironmentTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fullstack-template",
					Namespace: "default",
				},
				Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
					DisplayName: "Full Stack App",
					Description: "Complete app with frontend and database",
					Icon:        "🚀",
					Tags:        []string{"fullstack", "production-like"},
					DefaultTTL:  "4h",
					Components: []ephemeralv1alpha1.ComponentSpec{
						{
							Name:        "frontend",
							Repository:  "https://charts.bitnami.com/bitnami",
							Chart:       "nginx",
							Version:     "15.0.0",
							ServiceName: "frontend",
							ServicePort: 80,
							Primary:     true,
						},
						{
							Name:        "database",
							Repository:  "https://charts.bitnami.com/bitnami",
							Chart:       "postgresql",
							Version:     "12.0.0",
							ServiceName: "postgresql",
							ServicePort: 5432,
							Primary:     false,
							Values:      &apiextensionsv1.JSON{Raw: []byte(postgresValues)},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, template)).Should(Succeed())

			// Verify the template was created
			createdTemplate := &ephemeralv1alpha1.EnvironmentTemplate{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "fullstack-template",
					Namespace: "default",
				}, createdTemplate)
			}, timeout, interval).Should(Succeed())

			Expect(createdTemplate.Spec.Components).To(HaveLen(2))

			// Find primary component
			var primaryFound bool
			for _, comp := range createdTemplate.Spec.Components {
				if comp.Primary {
					primaryFound = true
					Expect(comp.Name).To(Equal("frontend"))
				}
			}
			Expect(primaryFound).To(BeTrue())

			// Cleanup
			Expect(k8sClient.Delete(ctx, template)).Should(Succeed())
		})

		It("Should reject a template without components", func() {
			ctx := context.Background()

			template := &ephemeralv1alpha1.EnvironmentTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-template",
					Namespace: "default",
				},
				Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
					DisplayName: "Empty Template",
					Components:  []ephemeralv1alpha1.ComponentSpec{}, // Empty!
				},
			}

			err := k8sClient.Create(ctx, template)
			Expect(err).To(HaveOccurred())
			// Should fail validation due to MinItems=1
		})

		It("Should reject a component with invalid repository URL", func() {
			ctx := context.Background()

			template := &ephemeralv1alpha1.EnvironmentTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-repo-template",
					Namespace: "default",
				},
				Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
					DisplayName: "Invalid Repo",
					Components: []ephemeralv1alpha1.ComponentSpec{
						{
							Name:        "test",
							Repository:  "not-a-valid-url", // Invalid!
							Chart:       "nginx",
							Version:     "15.0.0",
							ServiceName: "nginx",
							ServicePort: 80,
							Primary:     true,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, template)
			Expect(err).To(HaveOccurred())
			// Should fail validation due to URL pattern
		})

		It("Should reject a component with invalid version format", func() {
			ctx := context.Background()

			template := &ephemeralv1alpha1.EnvironmentTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-version-template",
					Namespace: "default",
				},
				Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
					DisplayName: "Invalid Version",
					Components: []ephemeralv1alpha1.ComponentSpec{
						{
							Name:        "test",
							Repository:  "https://charts.bitnami.com/bitnami",
							Chart:       "nginx",
							Version:     "latest", // Invalid! Should be semver
							ServiceName: "nginx",
							ServicePort: 80,
							Primary:     true,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, template)
			Expect(err).To(HaveOccurred())
			// Should fail validation due to version pattern
		})
	})

	Context("When listing EnvironmentTemplates", func() {
		BeforeEach(func() {
			ctx := context.Background()

			// Create test templates
			templates := []*ephemeralv1alpha1.EnvironmentTemplate{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "list-test-nginx",
						Namespace: "default",
					},
					Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
						DisplayName: "Nginx Test",
						Tags:        []string{"web"},
						Components: []ephemeralv1alpha1.ComponentSpec{
							{
								Name:        "nginx",
								Repository:  "https://charts.bitnami.com/bitnami",
								Chart:       "nginx",
								Version:     "15.0.0",
								ServiceName: "nginx",
								ServicePort: 80,
								Primary:     true,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "list-test-redis",
						Namespace: "default",
					},
					Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
						DisplayName: "Redis Test",
						Tags:        []string{"database", "cache"},
						Components: []ephemeralv1alpha1.ComponentSpec{
							{
								Name:        "redis",
								Repository:  "https://charts.bitnami.com/bitnami",
								Chart:       "redis",
								Version:     "17.0.0",
								ServiceName: "redis-master",
								ServicePort: 6379,
								Primary:     true,
							},
						},
					},
				},
			}

			for _, t := range templates {
				Expect(k8sClient.Create(ctx, t)).Should(Succeed())
			}
		})

		AfterEach(func() {
			ctx := context.Background()

			// Cleanup
			templates := &ephemeralv1alpha1.EnvironmentTemplateList{}
			Expect(k8sClient.List(ctx, templates, client.InNamespace("default"))).Should(Succeed())

			for _, t := range templates.Items {
				if t.Name == "list-test-nginx" || t.Name == "list-test-redis" {
					Expect(k8sClient.Delete(ctx, &t)).Should(Succeed())
				}
			}
		})

		It("Should list all templates in a namespace", func() {
			ctx := context.Background()

			templates := &ephemeralv1alpha1.EnvironmentTemplateList{}
			Eventually(func() int {
				err := k8sClient.List(ctx, templates, client.InNamespace("default"))
				if err != nil {
					return 0
				}
				count := 0
				for _, t := range templates.Items {
					if t.Name == "list-test-nginx" || t.Name == "list-test-redis" {
						count++
					}
				}
				return count
			}, timeout, interval).Should(Equal(2))
		})

		It("Should filter templates by label selector", func() {
			ctx := context.Background()

			// Add a label to one template
			template := &ephemeralv1alpha1.EnvironmentTemplate{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "list-test-nginx",
				Namespace: "default",
			}, template)).Should(Succeed())

			template.Labels = map[string]string{"tier": "frontend"}
			Expect(k8sClient.Update(ctx, template)).Should(Succeed())

			// Filter by label
			templates := &ephemeralv1alpha1.EnvironmentTemplateList{}
			Eventually(func() int {
				err := k8sClient.List(ctx, templates,
					client.InNamespace("default"),
					client.MatchingLabels{"tier": "frontend"})
				if err != nil {
					return 0
				}
				return len(templates.Items)
			}, timeout, interval).Should(Equal(1))

			Expect(templates.Items[0].Name).To(Equal("list-test-nginx"))
		})
	})

	Context("When updating an EnvironmentTemplate", func() {
		var template *ephemeralv1alpha1.EnvironmentTemplate

		BeforeEach(func() {
			ctx := context.Background()

			template = &ephemeralv1alpha1.EnvironmentTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update-test-template",
					Namespace: "default",
				},
				Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
					DisplayName: "Original Name",
					Description: "Original description",
					Components: []ephemeralv1alpha1.ComponentSpec{
						{
							Name:        "nginx",
							Repository:  "https://charts.bitnami.com/bitnami",
							Chart:       "nginx",
							Version:     "15.0.0",
							ServiceName: "nginx",
							ServicePort: 80,
							Primary:     true,
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, template)).Should(Succeed())
		})

		AfterEach(func() {
			ctx := context.Background()
			// Cleanup - ignore if not found
			t := &ephemeralv1alpha1.EnvironmentTemplate{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "update-test-template",
				Namespace: "default",
			}, t); err == nil {
				Expect(k8sClient.Delete(ctx, t)).Should(Succeed())
			}
		})

		It("Should update the display name", func() {
			ctx := context.Background()

			// Fetch fresh copy
			current := &ephemeralv1alpha1.EnvironmentTemplate{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "update-test-template",
				Namespace: "default",
			}, current)).Should(Succeed())

			current.Spec.DisplayName = "Updated Name"
			Expect(k8sClient.Update(ctx, current)).Should(Succeed())

			// Verify update
			updated := &ephemeralv1alpha1.EnvironmentTemplate{}
			Eventually(func() string {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "update-test-template",
					Namespace: "default",
				}, updated); err != nil {
					return ""
				}
				return updated.Spec.DisplayName
			}, timeout, interval).Should(Equal("Updated Name"))
		})

		It("Should add a new component", func() {
			ctx := context.Background()

			// Fetch fresh copy
			current := &ephemeralv1alpha1.EnvironmentTemplate{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "update-test-template",
				Namespace: "default",
			}, current)).Should(Succeed())

			current.Spec.Components = append(current.Spec.Components, ephemeralv1alpha1.ComponentSpec{
				Name:        "redis",
				Repository:  "https://charts.bitnami.com/bitnami",
				Chart:       "redis",
				Version:     "17.0.0",
				ServiceName: "redis-master",
				ServicePort: 6379,
				Primary:     false,
			})
			Expect(k8sClient.Update(ctx, current)).Should(Succeed())

			// Verify update
			updated := &ephemeralv1alpha1.EnvironmentTemplate{}
			Eventually(func() int {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "update-test-template",
					Namespace: "default",
				}, updated); err != nil {
					return 0
				}
				return len(updated.Spec.Components)
			}, timeout, interval).Should(Equal(2))
		})
	})

	Context("When deleting an EnvironmentTemplate", func() {
		It("Should delete the template successfully", func() {
			ctx := context.Background()

			// Create template
			template := &ephemeralv1alpha1.EnvironmentTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delete-test-template",
					Namespace: "default",
				},
				Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
					DisplayName: "Delete Me",
					Components: []ephemeralv1alpha1.ComponentSpec{
						{
							Name:        "nginx",
							Repository:  "https://charts.bitnami.com/bitnami",
							Chart:       "nginx",
							Version:     "15.0.0",
							ServiceName: "nginx",
							ServicePort: 80,
							Primary:     true,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, template)).Should(Succeed())

			// Delete it
			Expect(k8sClient.Delete(ctx, template)).Should(Succeed())

			// Verify deletion
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "delete-test-template",
					Namespace: "default",
				}, &ephemeralv1alpha1.EnvironmentTemplate{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When updating EnvironmentTemplate status", func() {
		It("Should track usage count", func() {
			ctx := context.Background()

			// Create template
			template := &ephemeralv1alpha1.EnvironmentTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usage-test-template",
					Namespace: "default",
				},
				Spec: ephemeralv1alpha1.EnvironmentTemplateSpec{
					DisplayName: "Usage Test",
					Components: []ephemeralv1alpha1.ComponentSpec{
						{
							Name:        "nginx",
							Repository:  "https://charts.bitnami.com/bitnami",
							Chart:       "nginx",
							Version:     "15.0.0",
							ServiceName: "nginx",
							ServicePort: 80,
							Primary:     true,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, template)).Should(Succeed())

			// Update status
			current := &ephemeralv1alpha1.EnvironmentTemplate{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "usage-test-template",
				Namespace: "default",
			}, current)).Should(Succeed())

			now := metav1.Now()
			current.Status.UsageCount = 5
			current.Status.LastUsedTime = &now
			Expect(k8sClient.Status().Update(ctx, current)).Should(Succeed())

			// Verify status update
			updated := &ephemeralv1alpha1.EnvironmentTemplate{}
			Eventually(func() int32 {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "usage-test-template",
					Namespace: "default",
				}, updated); err != nil {
					return 0
				}
				return updated.Status.UsageCount
			}, timeout, interval).Should(Equal(int32(5)))

			// Cleanup
			Expect(k8sClient.Delete(ctx, template)).Should(Succeed())
		})
	})
})
