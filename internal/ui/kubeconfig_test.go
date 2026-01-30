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
	"encoding/base64"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"gopkg.in/yaml.v3"
)

func TestGenerateKubeconfigYAML(t *testing.T) {
	tests := []struct {
		name       string
		serverURL  string
		caData     []byte
		token      string
		namespace  string
		envName    string
		wantErr    bool
		assertYAML func(t *testing.T, yamlBytes []byte)
	}{
		{
			name:      "output contains token server and namespace",
			serverURL: "https://api.example.com:6443",
			caData:    []byte("fake-ca-cert-data"),
			token:     "secret-token-123",
			namespace: "env-pr-123",
			envName:   "pr-123",
			wantErr:   false,
			assertYAML: func(t *testing.T, yamlBytes []byte) {
				yamlStr := string(yamlBytes)
				if !strings.Contains(yamlStr, "secret-token-123") {
					t.Errorf("kubeconfig YAML must contain the token; got: %s", yamlStr)
				}
				if !strings.Contains(yamlStr, "https://api.example.com:6443") {
					t.Errorf("kubeconfig YAML must contain the server URL; got: %s", yamlStr)
				}
				if !strings.Contains(yamlStr, "env-pr-123") {
					t.Errorf("kubeconfig YAML must contain the namespace in context; got: %s", yamlStr)
				}
			},
		},
		{
			name:      "valid kubeconfig structure",
			serverURL: "https://kubernetes.default.svc",
			caData:    []byte("ca-bytes"),
			token:     "my-token",
			namespace: "env-myenv",
			envName:   "myenv",
			wantErr:   false,
			assertYAML: func(t *testing.T, yamlBytes []byte) {
				var kc kubeconfigStruct
				if err := yaml.Unmarshal(yamlBytes, &kc); err != nil {
					t.Fatalf("generated YAML must be valid kubeconfig: %v", err)
				}
				if len(kc.Clusters) != 1 {
					t.Errorf("expected 1 cluster, got %d", len(kc.Clusters))
				}
				if kc.Clusters[0].Cluster.Server != "https://kubernetes.default.svc" {
					t.Errorf("cluster server = %q", kc.Clusters[0].Cluster.Server)
				}
				if len(kc.Users) != 1 {
					t.Errorf("expected 1 user, got %d", len(kc.Users))
				}
				if kc.Users[0].User.Token != "my-token" {
					t.Errorf("user token = %q", kc.Users[0].User.Token)
				}
				if len(kc.Contexts) != 1 {
					t.Errorf("expected 1 context, got %d", len(kc.Contexts))
				}
				if kc.Contexts[0].Context.Namespace != "env-myenv" {
					t.Errorf("context namespace = %q", kc.Contexts[0].Context.Namespace)
				}
				if kc.CurrentContext != "env-myenv" {
					t.Errorf("current-context = %q", kc.CurrentContext)
				}
			},
		},
		{
			name:      "empty server URL returns error",
			serverURL: "",
			caData:    []byte("ca"),
			token:     "t",
			namespace: "ns",
			envName:   "e",
			wantErr:   true,
		},
		{
			name:      "empty token returns error",
			serverURL: "https://api",
			caData:    []byte("ca"),
			token:     "",
			namespace: "ns",
			envName:   "e",
			wantErr:   true,
		},
		{
			name:      "empty namespace returns error",
			serverURL: "https://api",
			caData:    []byte("ca"),
			token:     "t",
			namespace: "",
			envName:   "e",
			wantErr:   true,
		},
		{
			name:      "empty envName returns error",
			serverURL: "https://api",
			caData:    []byte("ca"),
			token:     "t",
			namespace: "ns",
			envName:   "",
			wantErr:   true,
		},
		{
			name:      "nil caData is allowed (insecure cluster)",
			serverURL: "https://api",
			caData:    nil,
			token:     "t",
			namespace: "ns",
			envName:   "env",
			wantErr:   false,
			assertYAML: func(t *testing.T, yamlBytes []byte) {
				// Should still produce valid YAML; cluster may have empty or no CA
				var kc kubeconfigStruct
				if err := yaml.Unmarshal(yamlBytes, &kc); err != nil {
					t.Fatalf("nil caData kubeconfig must be valid: %v", err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateKubeconfigYAML(tt.serverURL, tt.caData, tt.token, tt.namespace, tt.envName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateKubeconfigYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if tt.assertYAML != nil {
				tt.assertYAML(t, got)
			}
		})
	}
}

func TestGetClusterInfoServerAndCA(t *testing.T) {
	ctx := context.Background()

	t.Run("returns error when kubeClient is nil", func(t *testing.T) {
		_, _, err := GetClusterInfoServerAndCA(ctx, nil)
		if err == nil {
			t.Fatal("expected error when kubeClient is nil")
		}
	})

	t.Run("returns error when cluster-info ConfigMap does not exist", func(t *testing.T) {
		//nolint:staticcheck // SA1019 fake.NewSimpleClientset is the standard for unit tests without applyconfig
		client := fake.NewSimpleClientset()
		_, _, err := GetClusterInfoServerAndCA(ctx, client)
		if err == nil {
			t.Fatal("expected error when cluster-info ConfigMap is missing")
		}
	})

	t.Run("returns server and CA from cluster-info ConfigMap", func(t *testing.T) {
		caB64 := base64.StdEncoding.EncodeToString([]byte("fake-ca-pem"))
		kubeconfigYAML := `clusters:
- name: my-cluster
  cluster:
    server: https://api.example.com:6443
    certificate-authority-data: ` + caB64 + "\n"
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ClusterInfoConfigMapName,
				Namespace: ClusterInfoNamespace,
			},
			Data: map[string]string{
				ClusterInfoKubeconfigKey: kubeconfigYAML,
			},
		}
		//nolint:staticcheck // SA1019 fake.NewSimpleClientset is the standard for unit tests without applyconfig
		client := fake.NewSimpleClientset(cm)
		serverURL, caData, err := GetClusterInfoServerAndCA(ctx, client)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if serverURL != "https://api.example.com:6443" {
			t.Errorf("serverURL = %q, want https://api.example.com:6443", serverURL)
		}
		if string(caData) != "fake-ca-pem" {
			t.Errorf("caData = %q, want fake-ca-pem", string(caData))
		}
	})

	t.Run("returns error when cluster-info has no kubeconfig key", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ClusterInfoConfigMapName,
				Namespace: ClusterInfoNamespace,
			},
			Data: map[string]string{},
		}
		//nolint:staticcheck // SA1019 fake.NewSimpleClientset is the standard for unit tests without applyconfig
		client := fake.NewSimpleClientset(cm)
		_, _, err := GetClusterInfoServerAndCA(ctx, client)
		if err == nil {
			t.Fatal("expected error when kubeconfig key is missing")
		}
	})
}

// kubeconfigStruct mirrors the kubeconfig YAML for assertion
type kubeconfigStruct struct {
	APIVersion     string `yaml:"apiVersion"`
	Kind           string `yaml:"kind"`
	CurrentContext string `yaml:"current-context"`
	Clusters       []struct {
		Name    string `yaml:"name"`
		Cluster struct {
			Server                   string `yaml:"server"`
			CertificateAuthorityData string `yaml:"certificate-authority-data"`
		} `yaml:"cluster"`
	} `yaml:"clusters"`
	Contexts []struct {
		Name    string `yaml:"name"`
		Context struct {
			Cluster   string `yaml:"cluster"`
			User      string `yaml:"user"`
			Namespace string `yaml:"namespace"`
		} `yaml:"context"`
	} `yaml:"contexts"`
	Users []struct {
		Name string `yaml:"name"`
		User struct {
			Token string `yaml:"token"`
		} `yaml:"user"`
	} `yaml:"users"`
}
