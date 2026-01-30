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
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DefaultTTL is the default time-to-live for ephemeral environments
	DefaultTTL = "24h"

	// DefaultTargetPort is the default service port
	DefaultTargetPort int32 = 80

	// NamespacePrefix is the prefix used for environment namespaces
	NamespacePrefix = "env-"
)

// IsIsolationEnabled returns whether network isolation is enabled.
// Defaults to true if not explicitly set.
func (e *EphemeralEnvSpec) IsIsolationEnabled() bool {
	if e.Isolation == nil {
		return true // Default to true
	}
	return *e.Isolation
}

// GetTTL returns the TTL duration. Returns default if parsing fails.
func (e *EphemeralEnvSpec) GetTTL() time.Duration {
	ttl := e.TTL
	if ttl == "" {
		ttl = DefaultTTL
	}

	duration, err := time.ParseDuration(ttl)
	if err != nil {
		// Return default on parse error
		duration, _ = time.ParseDuration(DefaultTTL)
	}
	return duration
}

// GetTargetPort returns the target port, using default if not set
func (g *GatewaySpec) GetTargetPort() int32 {
	if g.TargetPort == 0 {
		return DefaultTargetPort
	}
	return g.TargetPort
}

// GetNamespaceName returns the namespace name for this environment
func (e *EphemeralEnv) GetNamespaceName() string {
	return fmt.Sprintf("%s%s", NamespacePrefix, e.Name)
}

// GetHTTPRouteName returns the HTTPRoute name for this environment
func (e *EphemeralEnv) GetHTTPRouteName() string {
	return fmt.Sprintf("route-%s", e.Name)
}

// GetReferenceGrantName returns the ReferenceGrant name for this environment
func (e *EphemeralEnv) GetReferenceGrantName() string {
	return fmt.Sprintf("grant-%s", e.Name)
}

// CalculateExpirationTime calculates the expiration time based on creation time and TTL
func (e *EphemeralEnv) CalculateExpirationTime() metav1.Time {
	ttl := e.Spec.GetTTL()
	return metav1.NewTime(e.CreationTimestamp.Add(ttl))
}

// IsExpired checks if the environment has exceeded its TTL
func (e *EphemeralEnv) IsExpired() bool {
	if e.Status.ExpirationTime == nil {
		return false
	}
	return time.Now().After(e.Status.ExpirationTime.Time)
}

// SetPhase is a helper to update the phase
func (s *EphemeralEnvStatus) SetPhase(phase EphemeralEnvPhase) {
	s.Phase = phase
}

// Helper function to create a bool pointer (useful for tests and specs)
func BoolPtr(b bool) *bool {
	return &b
}

// Helper function to create a string pointer
func StringPtr(s string) *string {
	return &s
}
