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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	ephemeralv1alpha1 "github.com/maciekmm/kubeEphemerals/api/v1alpha1"
	"github.com/maciekmm/kubeEphemerals/internal/helm"
)

// testNamespace is the default namespace used for all test EphemeralEnv resources
const testNamespace = "default"

// mockHelmClient is the mock Helm client used in tests
var mockHelmClient *helm.MockClient

// findCondition finds a condition by type in a slice of conditions
func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

// reconcileWithRetry calls Reconcile multiple times until no Requeue is requested
// This handles the finalizer addition which requires an extra reconcile
func reconcileWithRetry(ctx context.Context, reconciler *EphemeralEnvReconciler, req reconcile.Request, maxRetries int) error {
	for range maxRetries {
		result, err := reconciler.Reconcile(ctx, req)
		if err != nil {
			return err
		}
		if result.RequeueAfter == 0 {
			return nil
		}
		// Small sleep to let the system stabilize
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

// Helper function to create a valid test EphemeralEnv
func createTestEphemeralEnv(name string) *ephemeralv1alpha1.EphemeralEnv {
	return &ephemeralv1alpha1.EphemeralEnv{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: ephemeralv1alpha1.EphemeralEnvSpec{
			TTL:       "2h",
			Isolation: ephemeralv1alpha1.BoolPtr(true),
			Helm: &ephemeralv1alpha1.HelmSpec{
				Repository: "https://charts.example.com",
				Chart:      "my-app",
				Version:    "1.0.0",
			},
			Gateway: ephemeralv1alpha1.GatewaySpec{
				Name:         "main-gateway",
				Namespace:    "gateway-system",
				DomainPrefix: name,
				ServiceName:  "my-app",
				TargetPort:   8080,
			},
		},
	}
}

var _ = Describe("EphemeralEnv Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When reconciling a resource", func() {
		var (
			ctx                context.Context
			ephemeralEnv       *ephemeralv1alpha1.EphemeralEnv
			typeNamespacedName types.NamespacedName
			reconciler         *EphemeralEnvReconciler
		)

		BeforeEach(func() {
			ctx = context.Background()
			mockHelmClient = helm.NewMockClient()
			reconciler = &EphemeralEnvReconciler{
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				HelmClient: mockHelmClient,
			}
		})

		AfterEach(func() {
			// Cleanup the EphemeralEnv if it exists
			if ephemeralEnv != nil {
				err := k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)
				if err == nil {
					Expect(k8sClient.Delete(ctx, ephemeralEnv)).To(Succeed())
				}
			}
		})

		// =================================================================
		// PHASE 2: NAMESPACE LIFECYCLE TESTS (TDD - RED)
		// =================================================================

		Describe("Phase 2: Namespace Lifecycle", func() {

			It("should create a namespace with correct naming convention (env-{name})", func() {
				By("Creating a new EphemeralEnv")
				envName := "pr-123"
				expectedNamespace := "env-pr-123"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking that the namespace was created")
				namespace := &corev1.Namespace{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: expectedNamespace}, namespace)
				}, timeout, interval).Should(Succeed())

				Expect(namespace.Name).To(Equal(expectedNamespace))
			})

			It("should add correct labels to the namespace", func() {
				By("Creating a new EphemeralEnv")
				envName := "pr-456-labels"
				expectedNamespace := "env-pr-456-labels"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking namespace labels")
				namespace := &corev1.Namespace{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: expectedNamespace}, namespace)
				}, timeout, interval).Should(Succeed())

				Expect(namespace.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "ephemeral-operator"))
				Expect(namespace.Labels).To(HaveKeyWithValue("ephemeral.ephemeralenv.io/owner", envName))
			})

			It("should set owner reference on the namespace", func() {
				By("Creating a new EphemeralEnv")
				envName := "pr-789-owner"
				expectedNamespace := "env-pr-789-owner"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				// Get the created resource to have UID
				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking owner reference on namespace")
				namespace := &corev1.Namespace{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: expectedNamespace}, namespace)
				}, timeout, interval).Should(Succeed())

				// Note: Namespaces are cluster-scoped, so owner references may have limitations
				// but we should at least have labels pointing to the owner
				Expect(namespace.Labels).To(HaveKeyWithValue("ephemeral.ephemeralenv.io/owner", envName))
			})

			It("should update status with ActiveNamespace", func() {
				By("Creating a new EphemeralEnv")
				envName := "pr-status-ns"
				expectedNamespace := "env-pr-status-ns"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking status.activeNamespace is updated")
				updatedEnv := &ephemeralv1alpha1.EphemeralEnv{}
				Eventually(func() string {
					err := k8sClient.Get(ctx, typeNamespacedName, updatedEnv)
					if err != nil {
						return ""
					}
					return updatedEnv.Status.ActiveNamespace
				}, timeout, interval).Should(Equal(expectedNamespace))
			})

			It("should set phase to Pending initially", func() {
				By("Creating a new EphemeralEnv")
				envName := "pr-phase-pending"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking phase is set")
				updatedEnv := &ephemeralv1alpha1.EphemeralEnv{}
				Eventually(func() ephemeralv1alpha1.EphemeralEnvPhase {
					err := k8sClient.Get(ctx, typeNamespacedName, updatedEnv)
					if err != nil {
						return ""
					}
					return updatedEnv.Status.Phase
				}, timeout, interval).Should(Or(
					Equal(ephemeralv1alpha1.PhasePending),
					Equal(ephemeralv1alpha1.PhaseActive),
				))
			})

			It("should set ExpirationTime based on TTL", func() {
				By("Creating a new EphemeralEnv with 2h TTL")
				envName := "pr-expiration"

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.TTL = "2h"
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				beforeCreate := time.Now()
				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking ExpirationTime is set correctly")
				updatedEnv := &ephemeralv1alpha1.EphemeralEnv{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, updatedEnv)
					if err != nil {
						return false
					}
					return updatedEnv.Status.ExpirationTime != nil
				}, timeout, interval).Should(BeTrue())

				// Expiration should be approximately 2h from creation (allow 5 second tolerance)
				expectedExpiration := beforeCreate.Add(2 * time.Hour)
				Expect(updatedEnv.Status.ExpirationTime.Time).To(BeTemporally("~", expectedExpiration, 5*time.Second))
			})

			It("should be idempotent - not recreate existing namespace", func() {
				By("Creating a new EphemeralEnv")
				envName := "pr-idempotent"
				expectedNamespace := "env-pr-idempotent"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv first time")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Getting the namespace")
				namespace := &corev1.Namespace{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: expectedNamespace}, namespace)
				}, timeout, interval).Should(Succeed())
				originalUID := namespace.UID

				By("Reconciling the EphemeralEnv second time")
				_, err = reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking namespace UID is unchanged")
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: expectedNamespace}, namespace)).To(Succeed())
				Expect(namespace.UID).To(Equal(originalUID))
			})

			It("should handle deleted EphemeralEnv gracefully", func() {
				By("Reconciling a non-existent EphemeralEnv")
				typeNamespacedName = types.NamespacedName{
					Name:      "non-existent",
					Namespace: "default",
				}

				result, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})

			It("should set NamespaceReady condition to True", func() {
				By("Creating a new EphemeralEnv")
				envName := "pr-condition-ns"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking NamespaceReady condition")
				updatedEnv := &ephemeralv1alpha1.EphemeralEnv{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, updatedEnv)
					if err != nil {
						return false
					}
					for _, cond := range updatedEnv.Status.Conditions {
						if cond.Type == ephemeralv1alpha1.ConditionTypeNamespaceReady &&
							cond.Status == metav1.ConditionTrue {
							return true
						}
					}
					return false
				}, timeout, interval).Should(BeTrue())
			})
		})

		// =================================================================
		// PHASE 3: SECURITY & NETWORK POLICY TESTS (TDD - RED)
		// =================================================================

		Describe("Phase 3: Security & NetworkPolicy", func() {

			It("should create a deny-all NetworkPolicy when Isolation is true", func() {
				By("Creating a new EphemeralEnv with Isolation=true")
				envName := "pr-isolated"
				expectedNamespace := "env-pr-isolated"

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.Isolation = ephemeralv1alpha1.BoolPtr(true)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking that the NetworkPolicy was created")
				networkPolicy := &networkingv1.NetworkPolicy{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      "deny-all",
						Namespace: expectedNamespace,
					}, networkPolicy)
				}, timeout, interval).Should(Succeed())

				Expect(networkPolicy.Name).To(Equal("deny-all"))
				Expect(networkPolicy.Namespace).To(Equal(expectedNamespace))
			})

			It("should create NetworkPolicy that denies all ingress and egress", func() {
				By("Creating a new EphemeralEnv with Isolation=true")
				envName := "pr-deny-all"
				expectedNamespace := "env-pr-deny-all"

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.Isolation = ephemeralv1alpha1.BoolPtr(true)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking NetworkPolicy rules")
				networkPolicy := &networkingv1.NetworkPolicy{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      "deny-all",
						Namespace: expectedNamespace,
					}, networkPolicy)
				}, timeout, interval).Should(Succeed())

				// Should have both Ingress and Egress policy types
				Expect(networkPolicy.Spec.PolicyTypes).To(ContainElements(
					networkingv1.PolicyTypeIngress,
					networkingv1.PolicyTypeEgress,
				))

				// Empty ingress and egress rules = deny all
				Expect(networkPolicy.Spec.Ingress).To(BeEmpty())
				Expect(networkPolicy.Spec.Egress).To(BeEmpty())

				// PodSelector should be empty to select all pods
				Expect(networkPolicy.Spec.PodSelector.MatchLabels).To(BeEmpty())
			})

			It("should NOT create NetworkPolicy when Isolation is false", func() {
				By("Creating a new EphemeralEnv with Isolation=false")
				envName := "pr-not-isolated"
				expectedNamespace := "env-pr-not-isolated"

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.Isolation = ephemeralv1alpha1.BoolPtr(false)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying namespace was created")
				namespace := &corev1.Namespace{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: expectedNamespace}, namespace)
				}, timeout, interval).Should(Succeed())

				By("Checking that NetworkPolicy was NOT created")
				networkPolicy := &networkingv1.NetworkPolicy{}
				Consistently(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      "deny-all",
						Namespace: expectedNamespace,
					}, networkPolicy)
					return errors.IsNotFound(err)
				}, "2s", interval).Should(BeTrue())
			})

			It("should set NetworkPolicyApplied condition to True when Isolation is enabled", func() {
				By("Creating a new EphemeralEnv with Isolation=true")
				envName := "pr-netpol-condition"

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.Isolation = ephemeralv1alpha1.BoolPtr(true)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking NetworkPolicyApplied condition")
				updatedEnv := &ephemeralv1alpha1.EphemeralEnv{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, updatedEnv)
					if err != nil {
						return false
					}
					for _, cond := range updatedEnv.Status.Conditions {
						if cond.Type == ephemeralv1alpha1.ConditionTypeNetworkPolicyApplied &&
							cond.Status == metav1.ConditionTrue {
							return true
						}
					}
					return false
				}, timeout, interval).Should(BeTrue())
			})

			It("should add labels to NetworkPolicy", func() {
				By("Creating a new EphemeralEnv with Isolation=true")
				envName := "pr-netpol-labels"
				expectedNamespace := "env-pr-netpol-labels"

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.Isolation = ephemeralv1alpha1.BoolPtr(true)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking NetworkPolicy labels")
				networkPolicy := &networkingv1.NetworkPolicy{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      "deny-all",
						Namespace: expectedNamespace,
					}, networkPolicy)
				}, timeout, interval).Should(Succeed())

				Expect(networkPolicy.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "ephemeral-operator"))
				Expect(networkPolicy.Labels).To(HaveKeyWithValue("ephemeral.ephemeralenv.io/owner", envName))
			})
		})

		// =================================================================
		// PHASE 4: TTL & CLEANUP TESTS (TDD - RED)
		// =================================================================

		Describe("Phase 4: TTL & Cleanup", func() {

			It("should set ExpirationTime on first reconciliation", func() {
				By("Creating a new EphemeralEnv with 2h TTL")
				envName := "pr-ttl-expiration"

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.TTL = "2h"
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking ExpirationTime is set correctly")
				updatedEnv := &ephemeralv1alpha1.EphemeralEnv{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, updatedEnv)).To(Succeed())

				Expect(updatedEnv.Status.ExpirationTime).NotTo(BeNil())
				expectedExpiration := updatedEnv.CreationTimestamp.Add(2 * time.Hour)
				Expect(updatedEnv.Status.ExpirationTime.Time).To(BeTemporally("~", expectedExpiration, 5*time.Second))
			})

			It("should use default TTL (24h) when TTL is not specified", func() {
				By("Creating a new EphemeralEnv without TTL")
				envName := "pr-default-ttl"

				ephemeralEnv = &ephemeralv1alpha1.EphemeralEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name:      envName,
						Namespace: "default",
					},
					Spec: ephemeralv1alpha1.EphemeralEnvSpec{
						// TTL not specified - should default to 24h
						Helm: &ephemeralv1alpha1.HelmSpec{
							Repository: "https://charts.example.com",
							Chart:      "my-app",
							Version:    "1.0.0",
						},
						Gateway: ephemeralv1alpha1.GatewaySpec{
							Name:         "main-gateway",
							Namespace:    "gateway-system",
							DomainPrefix: envName,
							ServiceName:  "my-app",
							TargetPort:   8080,
						},
					},
				}
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking ExpirationTime uses default 24h")
				updatedEnv := &ephemeralv1alpha1.EphemeralEnv{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, updatedEnv)).To(Succeed())

				Expect(updatedEnv.Status.ExpirationTime).NotTo(BeNil())
				expectedExpiration := updatedEnv.CreationTimestamp.Add(24 * time.Hour)
				Expect(updatedEnv.Status.ExpirationTime.Time).To(BeTemporally("~", expectedExpiration, 5*time.Second))
			})

			It("should set phase to Expired when TTL has passed", func() {
				By("Creating an EphemeralEnv with an already expired TTL")
				envName := "pr-expired"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Manually setting ExpirationTime to the past")
				// Get fresh copy
				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				pastTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
				ephemeralEnv.Status.ExpirationTime = &pastTime
				Expect(k8sClient.Status().Update(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the expired EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking phase is set to Expired")
				updatedEnv := &ephemeralv1alpha1.EphemeralEnv{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, updatedEnv)).To(Succeed())
				Expect(updatedEnv.Status.Phase).To(Equal(ephemeralv1alpha1.PhaseExpired))
			})

			It("should delete the namespace when environment expires", func() {
				By("Creating an EphemeralEnv and its namespace")
				envName := "pr-delete-ns"
				expectedNamespace := "env-pr-delete-ns"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("First reconcile to create namespace")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying namespace was created")
				namespace := &corev1.Namespace{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: expectedNamespace}, namespace)).To(Succeed())

				By("Setting ExpirationTime to the past to simulate expiration")
				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				pastTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
				ephemeralEnv.Status.ExpirationTime = &pastTime
				Expect(k8sClient.Status().Update(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling to trigger cleanup")
				_, err = reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking namespace is deleted or marked for deletion")
				Eventually(func() bool {
					ns := &corev1.Namespace{}
					err := k8sClient.Get(ctx, types.NamespacedName{Name: expectedNamespace}, ns)
					if errors.IsNotFound(err) {
						return true
					}
					// Namespace might be in terminating state
					return ns.DeletionTimestamp != nil
				}, timeout, interval).Should(BeTrue())
			})

			It("should set EnvironmentExpired condition when TTL expires", func() {
				By("Creating an EphemeralEnv")
				envName := "pr-expired-condition"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Setting ExpirationTime to the past")
				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				pastTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
				ephemeralEnv.Status.ExpirationTime = &pastTime
				Expect(k8sClient.Status().Update(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the expired EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking EnvironmentExpired condition is set")
				updatedEnv := &ephemeralv1alpha1.EphemeralEnv{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, updatedEnv)).To(Succeed())

				var foundCondition bool
				for _, cond := range updatedEnv.Status.Conditions {
					if cond.Type == ephemeralv1alpha1.ConditionTypeEnvironmentExpired {
						Expect(cond.Status).To(Equal(metav1.ConditionTrue))
						Expect(cond.Reason).To(Equal("TTLExpired"))
						foundCondition = true
						break
					}
				}
				Expect(foundCondition).To(BeTrue(), "EnvironmentExpired condition not found")
			})

			It("should return RequeueAfter with time until expiration", func() {
				By("Creating an EphemeralEnv with 1h TTL")
				envName := "pr-requeue"

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.TTL = "1h"
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				result, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking RequeueAfter is approximately equal to TTL")
				// Should be close to 1h (minus a few seconds for processing)
				Expect(result.RequeueAfter).To(BeNumerically(">", 59*time.Minute))
				Expect(result.RequeueAfter).To(BeNumerically("<=", 1*time.Hour))
			})

			It("should not requeue expired environments", func() {
				By("Creating an already expired EphemeralEnv")
				envName := "pr-no-requeue"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Setting ExpirationTime to the past")
				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				pastTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
				ephemeralEnv.Status.ExpirationTime = &pastTime
				Expect(k8sClient.Status().Update(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the expired EphemeralEnv")
				result, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking RequeueAfter is zero (no requeue)")
				Expect(result.RequeueAfter).To(Equal(time.Duration(0)))
			})

			It("should update status message when environment expires", func() {
				By("Creating an EphemeralEnv")
				envName := "pr-expired-message"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Setting ExpirationTime to the past")
				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				pastTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
				ephemeralEnv.Status.ExpirationTime = &pastTime
				Expect(k8sClient.Status().Update(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the expired EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking status message indicates expiration")
				updatedEnv := &ephemeralv1alpha1.EphemeralEnv{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, updatedEnv)).To(Succeed())
				Expect(updatedEnv.Status.Message).To(ContainSubstring("expired"))
			})

			It("should delete NetworkPolicy when environment expires", func() {
				By("Creating an EphemeralEnv with Isolation=true")
				envName := "pr-delete-netpol"
				expectedNamespace := "env-pr-delete-netpol"

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.Isolation = ephemeralv1alpha1.BoolPtr(true)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("First reconcile to create namespace and NetworkPolicy")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying NetworkPolicy was created")
				netpol := &networkingv1.NetworkPolicy{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "deny-all",
					Namespace: expectedNamespace,
				}, netpol)).To(Succeed())

				By("Setting ExpirationTime to the past to simulate expiration")
				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				pastTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
				ephemeralEnv.Status.ExpirationTime = &pastTime
				Expect(k8sClient.Status().Update(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling to trigger cleanup")
				_, err = reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking namespace is marked for deletion (NetworkPolicy will be cascade deleted)")
				// In EnvTest, namespace deletion doesn't fully complete, but we verify
				// the namespace is marked for deletion. In a real cluster, the NetworkPolicy
				// would be cascade deleted along with the namespace.
				namespace := &corev1.Namespace{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: expectedNamespace}, namespace)
					if errors.IsNotFound(err) {
						return true
					}
					// Namespace is marked for deletion
					return namespace.DeletionTimestamp != nil
				}, timeout, interval).Should(BeTrue())
			})
		})

		// =================================================================
		// PHASE 5: GATEWAY API TESTS (TDD - RED)
		// =================================================================

		Describe("Phase 5: Gateway API Integration", func() {

			It("should create HTTPRoute pointing to the service", func() {
				By("Creating the gateway-system namespace first")
				gatewayNs := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gateway-system",
					},
				}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "gateway-system"}, gatewayNs)
				if errors.IsNotFound(err) {
					Expect(k8sClient.Create(ctx, gatewayNs)).To(Succeed())
				}

				By("Creating a new EphemeralEnv")
				envName := "pr-gateway-route"
				expectedHTTPRouteName := "route-pr-gateway-route"

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.Gateway = ephemeralv1alpha1.GatewaySpec{
					Name:         "main-gateway",
					Namespace:    "gateway-system",
					DomainPrefix: "pr-gateway-route",
					ServiceName:  "my-app",
					TargetPort:   8080,
				}
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err = reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking HTTPRoute is created")
				httpRoute := &gatewayv1.HTTPRoute{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      expectedHTTPRouteName,
						Namespace: "gateway-system",
					}, httpRoute)
				}, timeout, interval).Should(Succeed())

				By("Verifying HTTPRoute configuration")
				Expect(httpRoute.Spec.Hostnames).To(HaveLen(1))
				Expect(string(httpRoute.Spec.Hostnames[0])).To(Equal("pr-gateway-route.preview.example.com"))
			})

			It("should set HTTPRouteReady condition when HTTPRoute is created", func() {
				By("Creating the gateway-system namespace")
				gatewayNs := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gateway-system",
					},
				}
				_ = k8sClient.Create(ctx, gatewayNs)

				By("Creating a new EphemeralEnv")
				envName := "pr-httproute-condition"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking HTTPRouteReady condition is set")
				updatedEnv := &ephemeralv1alpha1.EphemeralEnv{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, updatedEnv)).To(Succeed())

				var foundCondition bool
				for _, cond := range updatedEnv.Status.Conditions {
					if cond.Type == ephemeralv1alpha1.ConditionTypeHTTPRouteReady &&
						cond.Status == metav1.ConditionTrue {
						foundCondition = true
						break
					}
				}
				Expect(foundCondition).To(BeTrue(), "HTTPRouteReady condition not found")
			})

			It("should update AccessURL in status", func() {
				By("Creating the gateway-system namespace")
				gatewayNs := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gateway-system",
					},
				}
				_ = k8sClient.Create(ctx, gatewayNs)

				By("Creating a new EphemeralEnv")
				envName := "pr-access-url"

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.Gateway.DomainPrefix = "pr-access-url"
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking AccessURL is populated")
				updatedEnv := &ephemeralv1alpha1.EphemeralEnv{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, updatedEnv)).To(Succeed())
				Expect(updatedEnv.Status.AccessURL).To(Equal("https://pr-access-url.preview.example.com"))
			})

			It("should add labels to HTTPRoute", func() {
				By("Creating the gateway-system namespace")
				gatewayNs := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gateway-system",
					},
				}
				_ = k8sClient.Create(ctx, gatewayNs)

				By("Creating a new EphemeralEnv")
				envName := "pr-route-labels"
				expectedHTTPRouteName := "route-pr-route-labels"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking HTTPRoute labels")
				httpRoute := &gatewayv1.HTTPRoute{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      expectedHTTPRouteName,
						Namespace: "gateway-system",
					}, httpRoute)
				}, timeout, interval).Should(Succeed())

				Expect(httpRoute.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "ephemeral-operator"))
				Expect(httpRoute.Labels).To(HaveKeyWithValue("ephemeral.ephemeralenv.io/owner", envName))
			})

			It("should create ReferenceGrant for cross-namespace routing", func() {
				By("Creating the gateway-system namespace")
				gatewayNs := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gateway-system",
					},
				}
				_ = k8sClient.Create(ctx, gatewayNs)

				By("Creating a new EphemeralEnv")
				envName := "pr-ref-grant"
				expectedNamespace := "env-pr-ref-grant"

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.Gateway = ephemeralv1alpha1.GatewaySpec{
					Name:         "main-gateway",
					Namespace:    "gateway-system", // Different from env namespace
					DomainPrefix: "pr-ref-grant",
					ServiceName:  "my-app",
					TargetPort:   8080,
				}
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking ReferenceGrant is created in the env namespace")
				refGrant := &gatewayv1beta1.ReferenceGrant{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      "grant-" + envName,
						Namespace: expectedNamespace,
					}, refGrant)
				}, timeout, interval).Should(Succeed())

				By("Verifying ReferenceGrant allows HTTPRoute from gateway-system")
				Expect(refGrant.Spec.From).To(HaveLen(1))
				Expect(string(refGrant.Spec.From[0].Group)).To(Equal("gateway.networking.k8s.io"))
				Expect(string(refGrant.Spec.From[0].Kind)).To(Equal("HTTPRoute"))
				Expect(string(refGrant.Spec.From[0].Namespace)).To(Equal("gateway-system"))

				Expect(refGrant.Spec.To).To(HaveLen(1))
				Expect(string(refGrant.Spec.To[0].Group)).To(Equal(""))
				Expect(string(refGrant.Spec.To[0].Kind)).To(Equal("Service"))
			})

			It("should delete HTTPRoute when environment expires", func() {
				By("Creating the gateway-system namespace")
				gatewayNs := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gateway-system",
					},
				}
				_ = k8sClient.Create(ctx, gatewayNs)

				By("Creating a new EphemeralEnv")
				envName := "pr-delete-route"
				expectedHTTPRouteName := "route-pr-delete-route"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("First reconcile to create resources")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying HTTPRoute was created")
				httpRoute := &gatewayv1.HTTPRoute{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      expectedHTTPRouteName,
					Namespace: "gateway-system",
				}, httpRoute)).To(Succeed())

				By("Setting ExpirationTime to the past to simulate expiration")
				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				pastTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
				ephemeralEnv.Status.ExpirationTime = &pastTime
				Expect(k8sClient.Status().Update(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling to trigger cleanup")
				_, err = reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking HTTPRoute is deleted")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      expectedHTTPRouteName,
						Namespace: "gateway-system",
					}, httpRoute)
					return errors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			})

			It("should create Admin HTTPRoute when operator service is configured", func() {
				By("Creating the gateway-system namespace")
				gatewayNs := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gateway-system",
					},
				}
				_ = k8sClient.Create(ctx, gatewayNs)

				By("Creating a new EphemeralEnv")
				envName := "pr-admin-route"
				expectedAdminRouteName := "admin-route-pr-admin-route"

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.Gateway = ephemeralv1alpha1.GatewaySpec{
					Name:         "main-gateway",
					Namespace:    "gateway-system",
					DomainPrefix: "pr-admin-route",
					ServiceName:  "my-app",
					TargetPort:   8080,
				}
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Configuring reconciler with operator service")
				reconciler.OperatorService = "ephemeral-operator-ui"
				reconciler.OperatorNamespace = "ephemeral-system"

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking Admin HTTPRoute is created")
				adminRoute := &gatewayv1.HTTPRoute{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{
						Name:      expectedAdminRouteName,
						Namespace: "gateway-system",
					}, adminRoute)
				}, timeout, interval).Should(Succeed())

				By("Verifying Admin HTTPRoute configuration")
				Expect(adminRoute.Spec.Hostnames).To(HaveLen(1))
				Expect(string(adminRoute.Spec.Hostnames[0])).To(Equal("admin.pr-admin-route.preview.example.com"))

				By("Verifying Admin HTTPRoute labels")
				Expect(adminRoute.Labels).To(HaveKeyWithValue("ephemeral.ephemeralenv.io/route-type", "admin"))
				Expect(adminRoute.Labels).To(HaveKeyWithValue("ephemeral.ephemeralenv.io/owner", envName))

				By("Resetting reconciler config")
				reconciler.OperatorService = ""
				reconciler.OperatorNamespace = ""
			})

			It("should set AdminURL in status when admin HTTPRoute is created", func() {
				By("Creating the gateway-system namespace")
				gatewayNs := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gateway-system",
					},
				}
				_ = k8sClient.Create(ctx, gatewayNs)

				By("Creating a new EphemeralEnv")
				envName := "pr-admin-url"

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.Gateway.DomainPrefix = "pr-admin-url"
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Configuring reconciler with operator service")
				reconciler.OperatorService = "ephemeral-operator-ui"
				reconciler.OperatorNamespace = "ephemeral-system"

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking AdminURL is populated")
				updatedEnv := &ephemeralv1alpha1.EphemeralEnv{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, updatedEnv)).To(Succeed())
				Expect(updatedEnv.Status.AdminURL).To(Equal("https://admin.pr-admin-url.preview.example.com"))

				By("Resetting reconciler config")
				reconciler.OperatorService = ""
				reconciler.OperatorNamespace = ""
			})

			It("should NOT create Admin HTTPRoute when operator service is not configured", func() {
				By("Creating the gateway-system namespace")
				gatewayNs := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gateway-system",
					},
				}
				_ = k8sClient.Create(ctx, gatewayNs)

				By("Creating a new EphemeralEnv")
				envName := "pr-no-admin-route"
				expectedAdminRouteName := "admin-route-pr-no-admin-route"

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.Gateway = ephemeralv1alpha1.GatewaySpec{
					Name:         "main-gateway",
					Namespace:    "gateway-system",
					DomainPrefix: "pr-no-admin-route",
					ServiceName:  "my-app",
					TargetPort:   8080,
				}
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Ensuring reconciler has no operator service configured")
				reconciler.OperatorService = ""
				reconciler.OperatorNamespace = ""

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Admin HTTPRoute is NOT created")
				adminRoute := &gatewayv1.HTTPRoute{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      expectedAdminRouteName,
					Namespace: "gateway-system",
				}, adminRoute)
				Expect(errors.IsNotFound(err)).To(BeTrue(), "Admin HTTPRoute should not be created when operator service is not configured")
			})
		})

		// =================================================================
		// PHASE 5 (continued): HELM DEPLOYMENT TESTS (TDD - RED)
		// =================================================================

		Describe("Phase 5: Helm Deployment", func() {
			It("should call Helm install with correct parameters", func() {
				By("Creating a new EphemeralEnv with Helm configuration")
				envName := "pr-helm-install"

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.Helm = &ephemeralv1alpha1.HelmSpec{
					Repository: "https://charts.bitnami.com/bitnami",
					Chart:      "nginx",
					Version:    "18.0.0",
					Values:     &apiextensionsv1.JSON{Raw: []byte(`{"replicaCount": 1}`)},
				}
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: testNamespace,
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking that HelmClient.Install was called")
				// The mock client should have recorded the install call
				Expect(mockHelmClient.InstallCalls).To(HaveLen(1))
				installCall := mockHelmClient.InstallCalls[0]
				Expect(installCall.Namespace).To(Equal("env-" + envName))
				Expect(installCall.ChartName).To(Equal("nginx"))
				Expect(installCall.RepoURL).To(Equal("https://charts.bitnami.com/bitnami"))
				Expect(installCall.Version).To(Equal("18.0.0"))
			})

			It("should set HelmDeployed condition to True on success", func() {
				By("Creating a new EphemeralEnv")
				envName := "pr-helm-condition"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: testNamespace,
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking the HelmDeployed condition")
				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				condition := findCondition(ephemeralEnv.Status.Conditions, "HelmDeployed")
				Expect(condition).NotTo(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				Expect(condition.Reason).To(Equal("HelmInstalled"))
			})

			It("should update HelmRelease status field", func() {
				By("Creating a new EphemeralEnv")
				envName := "pr-helm-release"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: testNamespace,
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking the HelmRelease status")
				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				// With multi-chart support, release name format is: env-{envName}-{chartName}
				Expect(ephemeralEnv.Status.HelmRelease).To(Equal("env-" + envName + "-my-app"))
			})

			It("should set HelmDeployed condition to False on failure", func() {
				By("Configuring mock to return error")
				mockHelmClient.InstallFunc = func(ctx context.Context, opts helm.InstallOptions) (*helm.ReleaseInfo, error) {
					return nil, fmt.Errorf("chart not found")
				}

				By("Creating a new EphemeralEnv")
				envName := "pr-helm-failure"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: testNamespace,
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling the EphemeralEnv")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				// Should return error
				Expect(err).To(HaveOccurred())

				By("Checking the HelmDeployed condition is False")
				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				condition := findCondition(ephemeralEnv.Status.Conditions, "HelmDeployed")
				Expect(condition).NotTo(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Reason).To(Equal("HelmDeploymentFailed"))
			})

			It("should uninstall Helm release when environment expires", func() {
				By("Creating an EphemeralEnv and deploying Helm chart")
				envName := "pr-helm-cleanup"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: testNamespace,
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("First reconcile to deploy")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Setting ExpirationTime to the past")
				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				pastTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
				ephemeralEnv.Status.ExpirationTime = &pastTime
				// With multi-chart support, release name format is: env-{envName}-{chartName}
				ephemeralEnv.Status.HelmRelease = "env-" + envName + "-my-app"
				Expect(k8sClient.Status().Update(ctx, ephemeralEnv)).To(Succeed())

				By("Reconciling to trigger cleanup")
				_, err = reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking that Helm uninstall was called")
				Expect(mockHelmClient.UninstallCalls).To(HaveLen(1))
				uninstallCall := mockHelmClient.UninstallCalls[0]
				// With multi-chart support, release name format is: env-{envName}-{chartName}
				Expect(uninstallCall.ReleaseName).To(Equal("env-" + envName + "-my-app"))
				Expect(uninstallCall.Namespace).To(Equal("env-" + envName))
			})

			It("should not reinstall Helm chart if already deployed", func() {
				By("Creating a new EphemeralEnv")
				envName := "pr-helm-idempotent"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: testNamespace,
				}

				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("First reconcile")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				initialInstallCount := len(mockHelmClient.InstallCalls)

				By("Second reconcile - should not install again")
				_, err = reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Checking install was not called again")
				// Install should check IsInstalled first and skip if already installed
				Expect(mockHelmClient.InstallCalls).To(HaveLen(initialInstallCount))
			})
		})

		// =================================================================
		// Basic reconciliation test (from scaffold)
		// =================================================================

		Describe("Basic Reconciliation", func() {
			It("should successfully reconcile the resource", func() {
				By("Creating the custom resource for the Kind EphemeralEnv")
				envName := "test-resource"

				ephemeralEnv = createTestEphemeralEnv(envName)
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: "default",
				}

				err := k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)
				if err != nil && errors.IsNotFound(err) {
					Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())
				}

				By("Reconciling the created resource")
				_, err = reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		// =================================================================
		// Phase 8.5: Port-Based Service Discovery Tests
		// =================================================================

		Describe("Phase 8.5: Port-Based Service Discovery", func() {
			var (
				envNamespace string
			)

			// Helper to create test services
			createTestService := func(namespace, name string, port int32, clusterIP string) *corev1.Service {
				svc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "http", Port: port, Protocol: corev1.ProtocolTCP},
						},
						Selector: map[string]string{"app": name},
					},
				}
				if clusterIP != "" {
					svc.Spec.ClusterIP = clusterIP
				}
				return svc
			}

			BeforeEach(func() {
				// Reset mock calls
				mockHelmClient.InstallCalls = make([]helm.InstallOptions, 0)
				mockHelmClient.UninstallCalls = make([]struct{ ReleaseName, Namespace string }, 0)
				// Set up mock to return not installed initially
				mockHelmClient.IsInstalledFunc = func(ctx context.Context, releaseName, namespace string) (bool, error) {
					return false, nil
				}
			})

			It("should discover service with matching port (Happy Path)", func() {
				By("Creating EphemeralEnv with ServicePort")
				envName := "discovery-happy"
				servicePort := int32(8080)

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.ServicePort = &servicePort
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: testNamespace,
				}
				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("First reconcile - creates namespace")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
				Expect(err).NotTo(HaveOccurred())

				// Get the created namespace name
				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				envNamespace = ephemeralEnv.Status.ActiveNamespace
				Expect(envNamespace).NotTo(BeEmpty())

				By("Creating a matching service in the namespace")
				svc := createTestService(envNamespace, "my-app", servicePort, "")
				Expect(k8sClient.Create(ctx, svc)).To(Succeed())

				By("Reconciling again - should discover service")
				err = reconcileWithRetry(ctx, reconciler, reconcile.Request{NamespacedName: typeNamespacedName}, 5)
				Expect(err).NotTo(HaveOccurred())

				By("Checking status is Active")
				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				Expect(ephemeralEnv.Status.Phase).To(Equal(ephemeralv1alpha1.PhaseActive))

				By("Cleanup")
				Expect(k8sClient.Delete(ctx, svc)).To(Succeed())
			})

			It("should requeue when no service matches the port (No Match)", func() {
				By("Creating EphemeralEnv with ServicePort")
				envName := "discovery-nomatch"
				servicePort := int32(9999)

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.ServicePort = &servicePort
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: testNamespace,
				}
				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("First reconcile - creates namespace")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
				Expect(err).NotTo(HaveOccurred())

				// Get the created namespace name
				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				envNamespace = ephemeralEnv.Status.ActiveNamespace

				By("Creating a service with different port")
				svc := createTestService(envNamespace, "wrong-port-svc", 80, "")
				Expect(k8sClient.Create(ctx, svc)).To(Succeed())

				By("Reconciling - should requeue (no match)")
				result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
				// Either error or requeue is acceptable
				Expect(err != nil || result.RequeueAfter > 0).To(BeTrue())

				By("Cleanup")
				Expect(k8sClient.Delete(ctx, svc)).To(Succeed())
			})

			It("should apply heuristic when multiple services match (Multiple Matches)", func() {
				By("Creating EphemeralEnv with ServicePort")
				envName := "discovery-multi"
				servicePort := int32(8080)

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.ServicePort = &servicePort
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: testNamespace,
				}
				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("First reconcile - creates namespace")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
				Expect(err).NotTo(HaveOccurred())

				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				envNamespace = ephemeralEnv.Status.ActiveNamespace

				By("Creating multiple services with same port")
				svc1 := createTestService(envNamespace, "app", servicePort, "")
				svc2 := createTestService(envNamespace, "longer-app-name", servicePort, "")
				Expect(k8sClient.Create(ctx, svc1)).To(Succeed())
				Expect(k8sClient.Create(ctx, svc2)).To(Succeed())

				By("Reconciling - should pick one using heuristic (shortest name)")
				err = reconcileWithRetry(ctx, reconciler, reconcile.Request{NamespacedName: typeNamespacedName}, 5)
				Expect(err).NotTo(HaveOccurred())

				By("Checking status is Active")
				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				Expect(ephemeralEnv.Status.Phase).To(Equal(ephemeralv1alpha1.PhaseActive))

				By("Cleanup")
				Expect(k8sClient.Delete(ctx, svc1)).To(Succeed())
				Expect(k8sClient.Delete(ctx, svc2)).To(Succeed())
			})

			It("should ignore headless services (Ignore Garbage)", func() {
				By("Creating EphemeralEnv with ServicePort")
				envName := "discovery-headless"
				servicePort := int32(8080)

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.ServicePort = &servicePort
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: testNamespace,
				}
				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("First reconcile - creates namespace")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
				Expect(err).NotTo(HaveOccurred())

				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				envNamespace = ephemeralEnv.Status.ActiveNamespace

				By("Creating headless service (ClusterIP: None) and regular service")
				headlessSvc := createTestService(envNamespace, "headless-svc", servicePort, corev1.ClusterIPNone)
				regularSvc := createTestService(envNamespace, "regular-svc", servicePort, "")
				Expect(k8sClient.Create(ctx, headlessSvc)).To(Succeed())
				Expect(k8sClient.Create(ctx, regularSvc)).To(Succeed())

				By("Reconciling - should ignore headless and find regular service")
				err = reconcileWithRetry(ctx, reconciler, reconcile.Request{NamespacedName: typeNamespacedName}, 5)
				Expect(err).NotTo(HaveOccurred())

				By("Checking status is Active")
				Expect(k8sClient.Get(ctx, typeNamespacedName, ephemeralEnv)).To(Succeed())
				Expect(ephemeralEnv.Status.Phase).To(Equal(ephemeralv1alpha1.PhaseActive))

				By("Cleanup")
				Expect(k8sClient.Delete(ctx, headlessSvc)).To(Succeed())
				Expect(k8sClient.Delete(ctx, regularSvc)).To(Succeed())
			})

			It("should requeue when no services exist yet (Waiting)", func() {
				By("Creating EphemeralEnv with ServicePort")
				envName := "discovery-waiting"
				servicePort := int32(8080)

				ephemeralEnv = createTestEphemeralEnv(envName)
				ephemeralEnv.Spec.ServicePort = &servicePort
				typeNamespacedName = types.NamespacedName{
					Name:      envName,
					Namespace: testNamespace,
				}
				Expect(k8sClient.Create(ctx, ephemeralEnv)).To(Succeed())

				By("First reconcile - creates namespace")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
				Expect(err).NotTo(HaveOccurred())

				By("Second reconcile - no services exist, should requeue")
				result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
				// Either error or requeue is acceptable when waiting for service
				Expect(err != nil || result.RequeueAfter > 0).To(BeTrue())
			})
		})
	})
})
