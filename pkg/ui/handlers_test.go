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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ephemeralv1alpha1 "github.com/maciekmm/kubeEphemerals/api/v1alpha1"
)

func TestUIHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/ui Handlers Suite")
}

func minimalGatewaySpec() ephemeralv1alpha1.GatewaySpec {
	return ephemeralv1alpha1.GatewaySpec{
		Name:         "gw",
		Namespace:    "gateway-system",
		DomainPrefix: "test",
		ServiceName:  "svc",
		TargetPort:   80,
	}
}

var _ = Describe("ListEnvs isolation", func() {
	var (
		scheme   *runtime.Scheme
		fakeClient client.Client
		handler  *EnvHandler
		ctx      context.Context
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(ephemeralv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&ephemeralv1alpha1.EphemeralEnv{}).
			Build()
		handler = &EnvHandler{
			Client:           fakeClient,
			AdminUser:        "admin",
			DefaultNamespace: "default",
		}
		ctx = context.Background()
	})

	Context("TestListEnvs_Isolation", func() {
		BeforeEach(func() {
			envA := &ephemeralv1alpha1.EphemeralEnv{
				ObjectMeta: metav1.ObjectMeta{Name: "env-a", Namespace: "default"},
				Spec: ephemeralv1alpha1.EphemeralEnvSpec{
					Owner:   "alice",
					TTL:     "1h",
					Gateway: minimalGatewaySpec(),
				},
			}
			envB := &ephemeralv1alpha1.EphemeralEnv{
				ObjectMeta: metav1.ObjectMeta{Name: "env-b", Namespace: "default"},
				Spec: ephemeralv1alpha1.EphemeralEnvSpec{
					Owner:   "bob",
					TTL:     "1h",
					Gateway: minimalGatewaySpec(),
				},
			}
			Expect(fakeClient.Create(ctx, envA)).To(Succeed())
			Expect(fakeClient.Create(ctx, envB)).To(Succeed())
		})

		It("Scenario A: context user alice -> list returns only EnvA", func() {
			ctxAlice := ContextWithUser(ctx, "alice")
			list, err := handler.ListEnvs(ctxAlice)
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(HaveLen(1))
			Expect(list[0].Name).To(Equal("env-a"))
			Expect(list[0].Spec.Owner).To(Equal("alice"))
		})

		It("Scenario B: context user bob -> delete EnvA returns 403 Forbidden", func() {
			ctxBob := ContextWithUser(ctx, "bob")
			err := handler.DeleteEnv(ctxBob, "env-a")
			Expect(err).To(Equal(ErrForbidden))
			// EnvA should still exist
			env := &ephemeralv1alpha1.EphemeralEnv{}
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "env-a", Namespace: "default"}, env)).To(Succeed())
		})

		It("Scenario C: context user admin -> list returns EnvA and EnvB", func() {
			ctxAdmin := ContextWithUser(ctx, "admin")
			list, err := handler.ListEnvs(ctxAdmin)
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(HaveLen(2))
			names := []string{list[0].Name, list[1].Name}
			Expect(names).To(ContainElement("env-a"))
			Expect(names).To(ContainElement("env-b"))
		})
	})
})

var _ = Describe("DeleteEnv and CreateEnv", func() {
	var (
		scheme      *runtime.Scheme
		fakeClient  client.Client
		handler     *EnvHandler
		ctx         context.Context
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(ephemeralv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&ephemeralv1alpha1.EphemeralEnv{}).
			Build()
		handler = &EnvHandler{
			Client:           fakeClient,
			AdminUser:        "admin",
			DefaultNamespace: "default",
		}
		ctx = context.Background()
	})

	It("CreateEnv sets spec.owner to current user", func() {
		ctxAlice := ContextWithUser(ctx, "alice")
		env := &ephemeralv1alpha1.EphemeralEnv{
			ObjectMeta: metav1.ObjectMeta{Name: "new-env", Namespace: "default"},
			Spec: ephemeralv1alpha1.EphemeralEnvSpec{
				TTL:     "1h",
				Gateway: minimalGatewaySpec(),
			},
		}
		Expect(handler.CreateEnv(ctxAlice, env)).To(Succeed())
		Expect(env.Spec.Owner).To(Equal("alice"))
		var got ephemeralv1alpha1.EphemeralEnv
		Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "new-env", Namespace: "default"}, &got)).To(Succeed())
		Expect(got.Spec.Owner).To(Equal("alice"))
	})

	It("DeleteEnv allows owner to delete own env", func() {
		env := &ephemeralv1alpha1.EphemeralEnv{
			ObjectMeta: metav1.ObjectMeta{Name: "own-env", Namespace: "default"},
			Spec: ephemeralv1alpha1.EphemeralEnvSpec{
				Owner:   "bob",
				TTL:     "1h",
				Gateway: minimalGatewaySpec(),
			},
		}
		Expect(fakeClient.Create(ctx, env)).To(Succeed())
		ctxBob := ContextWithUser(ctx, "bob")
		Expect(handler.DeleteEnv(ctxBob, "own-env")).To(Succeed())
		err := fakeClient.Get(ctx, client.ObjectKey{Name: "own-env", Namespace: "default"}, &ephemeralv1alpha1.EphemeralEnv{})
		Expect(err).To(HaveOccurred()) // env should be gone (not found)
	})
})
