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
	"errors"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"gopkg.in/yaml.v3"
)

const (
	kubeconfigAPIVersion = "v1"
	kubeconfigKind       = "Config"
	// DeveloperAccessSAName is the ServiceAccount name used for developer kubeconfig (must match controller).
	DeveloperAccessSAName = "developer-access"
	// DefaultKubeconfigTokenExpiration is the default token TTL when env TTL is not set or is very long.
	DefaultKubeconfigTokenExpiration = 12 * time.Hour

	// ClusterInfoNamespace is the namespace for the cluster-info ConfigMap (kubeadm / many installers).
	ClusterInfoNamespace = "kube-public"
	// ClusterInfoConfigMapName is the ConfigMap name with cluster server URL and CA (often external/bootstrap endpoint).
	ClusterInfoConfigMapName = "cluster-info"
	// ClusterInfoKubeconfigKey is the key in that ConfigMap containing kubeconfig YAML.
	ClusterInfoKubeconfigKey = "kubeconfig"
)

// GetClusterInfoServerAndCA tries to read the API server URL and CA from the cluster-info ConfigMap
// in kube-public (used by kubeadm and many installers for bootstrap). That endpoint is often the
// external or "join" address, so generated kubeconfigs work from outside the cluster.
// Returns (serverURL, caData, nil) on success; on any error returns ("", nil, err) and caller should fall back to rest.Config.
func GetClusterInfoServerAndCA(ctx context.Context, kubeClient kubernetes.Interface) (serverURL string, caData []byte, err error) {
	if kubeClient == nil {
		return "", nil, errors.New("no Kubernetes client")
	}
	cm, err := kubeClient.CoreV1().ConfigMaps(ClusterInfoNamespace).Get(ctx, ClusterInfoConfigMapName, metav1.GetOptions{})
	if err != nil {
		return "", nil, err
	}
	raw, ok := cm.Data[ClusterInfoKubeconfigKey]
	if !ok {
		return "", nil, errors.New("cluster-info ConfigMap has no kubeconfig key")
	}
	var parsed struct {
		Clusters []struct {
			Name    string `yaml:"name"`
			Cluster struct {
				Server                   string `yaml:"server"`
				CertificateAuthorityData string `yaml:"certificate-authority-data"`
			} `yaml:"cluster"`
		} `yaml:"clusters"`
	}
	if err := yaml.Unmarshal([]byte(raw), &parsed); err != nil {
		return "", nil, err
	}
	if len(parsed.Clusters) == 0 {
		return "", nil, errors.New("cluster-info kubeconfig has no clusters")
	}
	serverURL = parsed.Clusters[0].Cluster.Server
	if serverURL == "" {
		return "", nil, errors.New("cluster-info cluster has no server")
	}
	caB64 := parsed.Clusters[0].Cluster.CertificateAuthorityData
	if caB64 != "" {
		caData, err = base64.StdEncoding.DecodeString(caB64)
		if err != nil {
			return "", nil, fmt.Errorf("decode cluster-info CA: %w", err)
		}
	}
	return serverURL, caData, nil
}

// GenerateKubeconfigYAML builds a standard kubeconfig YAML with the given cluster and token.
// The context is set with namespace so kubectl doesn't require -n.
// serverURL: API server URL (e.g. https://kubernetes.default.svc)
// caData: CA certificate (optional; can be nil for insecure)
// token: bearer token for the user
// namespace: target namespace (e.g. env-{name})
// envName: used for context name and filename (e.g. pr-123)
func GenerateKubeconfigYAML(serverURL string, caData []byte, token string, namespace string, envName string) ([]byte, error) {
	if serverURL == "" {
		return nil, errors.New("server URL is required")
	}
	if token == "" {
		return nil, errors.New("token is required")
	}
	if namespace == "" {
		return nil, errors.New("namespace is required")
	}
	if envName == "" {
		return nil, errors.New("env name is required")
	}

	clusterName := "env-" + envName
	userName := "developer-access"
	contextName := namespace

	caB64 := ""
	if len(caData) > 0 {
		caB64 = base64.StdEncoding.EncodeToString(caData)
	}

	kc := kubeconfigOut{
		APIVersion:     kubeconfigAPIVersion,
		Kind:           kubeconfigKind,
		CurrentContext: contextName,
		Clusters: []clusterEntry{{
			Name: clusterName,
			Cluster: clusterSpec{
				Server:                   serverURL,
				CertificateAuthorityData: caB64,
			},
		}},
		Contexts: []contextEntry{{
			Name: contextName,
			Context: contextSpec{
				Cluster:   clusterName,
				User:      userName,
				Namespace: namespace,
			},
		}},
		Users: []userEntry{{
			Name: userName,
			User: userSpec{
				Token: token,
			},
		}},
	}

	out, err := yaml.Marshal(kc)
	if err != nil {
		return nil, fmt.Errorf("marshal kubeconfig: %w", err)
	}
	return out, nil
}

type kubeconfigOut struct {
	APIVersion     string         `yaml:"apiVersion"`
	Kind           string         `yaml:"kind"`
	CurrentContext string         `yaml:"current-context"`
	Clusters       []clusterEntry `yaml:"clusters"`
	Contexts       []contextEntry `yaml:"contexts"`
	Users          []userEntry    `yaml:"users"`
}

type clusterEntry struct {
	Name    string      `yaml:"name"`
	Cluster clusterSpec `yaml:"cluster"`
}

type clusterSpec struct {
	Server                   string `yaml:"server"`
	CertificateAuthorityData string `yaml:"certificate-authority-data,omitempty"`
}

type contextEntry struct {
	Name    string      `yaml:"name"`
	Context contextSpec `yaml:"context"`
}

type contextSpec struct {
	Cluster   string `yaml:"cluster"`
	User      string `yaml:"user"`
	Namespace string `yaml:"namespace"`
}

type userEntry struct {
	Name string   `yaml:"name"`
	User userSpec `yaml:"user"`
}

type userSpec struct {
	Token string `yaml:"token"`
}
