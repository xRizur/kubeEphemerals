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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("EphemeralEnv Types", func() {

	Context("EphemeralEnvSpec", func() {

		Describe("IsIsolationEnabled", func() {
			It("should return true when Isolation is nil (default)", func() {
				spec := EphemeralEnvSpec{
					Isolation: nil,
				}
				Expect(spec.IsIsolationEnabled()).To(BeTrue())
			})

			It("should return true when Isolation is explicitly true", func() {
				spec := EphemeralEnvSpec{
					Isolation: BoolPtr(true),
				}
				Expect(spec.IsIsolationEnabled()).To(BeTrue())
			})

			It("should return false when Isolation is explicitly false", func() {
				spec := EphemeralEnvSpec{
					Isolation: BoolPtr(false),
				}
				Expect(spec.IsIsolationEnabled()).To(BeFalse())
			})
		})

		Describe("GetTTL", func() {
			It("should return default TTL (24h) when TTL is empty", func() {
				spec := EphemeralEnvSpec{
					TTL: "",
				}
				expected, _ := time.ParseDuration("24h")
				Expect(spec.GetTTL()).To(Equal(expected))
			})

			It("should parse valid TTL string", func() {
				spec := EphemeralEnvSpec{
					TTL: "2h",
				}
				expected, _ := time.ParseDuration("2h")
				Expect(spec.GetTTL()).To(Equal(expected))
			})

			It("should parse TTL with minutes", func() {
				spec := EphemeralEnvSpec{
					TTL: "30m",
				}
				expected, _ := time.ParseDuration("30m")
				Expect(spec.GetTTL()).To(Equal(expected))
			})

			It("should parse complex TTL format", func() {
				spec := EphemeralEnvSpec{
					TTL: "1h30m",
				}
				expected, _ := time.ParseDuration("1h30m")
				Expect(spec.GetTTL()).To(Equal(expected))
			})

			It("should return default TTL on invalid format", func() {
				spec := EphemeralEnvSpec{
					TTL: "invalid",
				}
				expected, _ := time.ParseDuration("24h")
				Expect(spec.GetTTL()).To(Equal(expected))
			})
		})
	})

	Context("GatewaySpec", func() {

		Describe("GetTargetPort", func() {
			It("should return default port (80) when TargetPort is 0", func() {
				gateway := GatewaySpec{
					TargetPort: 0,
				}
				Expect(gateway.GetTargetPort()).To(Equal(int32(80)))
			})

			It("should return specified port when set", func() {
				gateway := GatewaySpec{
					TargetPort: 8080,
				}
				Expect(gateway.GetTargetPort()).To(Equal(int32(8080)))
			})
		})
	})

	Context("EphemeralEnv", func() {

		Describe("GetNamespaceName", func() {
			It("should return namespace with env- prefix", func() {
				env := EphemeralEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pr-123",
					},
				}
				Expect(env.GetNamespaceName()).To(Equal("env-pr-123"))
			})

			It("should handle complex names", func() {
				env := EphemeralEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name: "feature-branch-abc-123",
					},
				}
				Expect(env.GetNamespaceName()).To(Equal("env-feature-branch-abc-123"))
			})
		})

		Describe("GetHTTPRouteName", func() {
			It("should return route name with route- prefix", func() {
				env := EphemeralEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pr-456",
					},
				}
				Expect(env.GetHTTPRouteName()).To(Equal("route-pr-456"))
			})
		})

		Describe("GetReferenceGrantName", func() {
			It("should return grant name with grant- prefix", func() {
				env := EphemeralEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pr-789",
					},
				}
				Expect(env.GetReferenceGrantName()).To(Equal("grant-pr-789"))
			})
		})

		Describe("CalculateExpirationTime", func() {
			It("should calculate expiration based on creation time and TTL", func() {
				creationTime := time.Now().Add(-1 * time.Hour)
				env := EphemeralEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-env",
						CreationTimestamp: metav1.NewTime(creationTime),
					},
					Spec: EphemeralEnvSpec{
						TTL: "2h",
					},
				}

				expiration := env.CalculateExpirationTime()
				expected := creationTime.Add(2 * time.Hour)

				// Allow 1 second tolerance
				Expect(expiration.Time).To(BeTemporally("~", expected, time.Second))
			})

			It("should use default TTL when not specified", func() {
				creationTime := time.Now()
				env := EphemeralEnv{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-env",
						CreationTimestamp: metav1.NewTime(creationTime),
					},
					Spec: EphemeralEnvSpec{
						TTL: "", // Empty, should default to 24h
					},
				}

				expiration := env.CalculateExpirationTime()
				expected := creationTime.Add(24 * time.Hour)

				Expect(expiration.Time).To(BeTemporally("~", expected, time.Second))
			})
		})

		Describe("IsExpired", func() {
			It("should return false when ExpirationTime is nil", func() {
				env := EphemeralEnv{
					Status: EphemeralEnvStatus{
						ExpirationTime: nil,
					},
				}
				Expect(env.IsExpired()).To(BeFalse())
			})

			It("should return false when not yet expired", func() {
				futureTime := metav1.NewTime(time.Now().Add(1 * time.Hour))
				env := EphemeralEnv{
					Status: EphemeralEnvStatus{
						ExpirationTime: &futureTime,
					},
				}
				Expect(env.IsExpired()).To(BeFalse())
			})

			It("should return true when expired", func() {
				pastTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
				env := EphemeralEnv{
					Status: EphemeralEnvStatus{
						ExpirationTime: &pastTime,
					},
				}
				Expect(env.IsExpired()).To(BeTrue())
			})
		})
	})

	Context("EphemeralEnvStatus", func() {

		Describe("SetPhase", func() {
			It("should set phase to Pending", func() {
				status := EphemeralEnvStatus{}
				status.SetPhase(PhasePending)
				Expect(status.Phase).To(Equal(PhasePending))
			})

			It("should set phase to Active", func() {
				status := EphemeralEnvStatus{}
				status.SetPhase(PhaseActive)
				Expect(status.Phase).To(Equal(PhaseActive))
			})

			It("should set phase to Failed", func() {
				status := EphemeralEnvStatus{}
				status.SetPhase(PhaseFailed)
				Expect(status.Phase).To(Equal(PhaseFailed))
			})

			It("should set phase to Expired", func() {
				status := EphemeralEnvStatus{}
				status.SetPhase(PhaseExpired)
				Expect(status.Phase).To(Equal(PhaseExpired))
			})
		})
	})

	Context("Helper Functions", func() {

		Describe("BoolPtr", func() {
			It("should return pointer to true", func() {
				ptr := BoolPtr(true)
				Expect(ptr).NotTo(BeNil())
				Expect(*ptr).To(BeTrue())
			})

			It("should return pointer to false", func() {
				ptr := BoolPtr(false)
				Expect(ptr).NotTo(BeNil())
				Expect(*ptr).To(BeFalse())
			})
		})

		Describe("StringPtr", func() {
			It("should return pointer to string", func() {
				ptr := StringPtr("test")
				Expect(ptr).NotTo(BeNil())
				Expect(*ptr).To(Equal("test"))
			})

			It("should handle empty string", func() {
				ptr := StringPtr("")
				Expect(ptr).NotTo(BeNil())
				Expect(*ptr).To(Equal(""))
			})
		})
	})

	Context("Constants", func() {
		It("should have correct default values", func() {
			Expect(DefaultTTL).To(Equal("24h"))
			Expect(DefaultTargetPort).To(Equal(int32(80)))
			Expect(NamespacePrefix).To(Equal("env-"))
		})

		It("should have correct phase values", func() {
			Expect(string(PhasePending)).To(Equal("Pending"))
			Expect(string(PhaseActive)).To(Equal("Active"))
			Expect(string(PhaseFailed)).To(Equal("Failed"))
			Expect(string(PhaseExpired)).To(Equal("Expired"))
		})

		It("should have correct condition type values", func() {
			Expect(ConditionTypeNamespaceReady).To(Equal("NamespaceReady"))
			Expect(ConditionTypeNetworkPolicyApplied).To(Equal("NetworkPolicyApplied"))
			Expect(ConditionTypeHelmDeployed).To(Equal("HelmDeployed"))
			Expect(ConditionTypeHTTPRouteReady).To(Equal("HTTPRouteReady"))
		})
	})
})
