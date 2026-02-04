# Ephemeral Environment Operator - Architecture Document

> **Source of Truth** for the entire development process.  
> Last Updated: February 2, 2026

---

## Table of Contents

1. [Project Goal](#1-project-goal)
2. [Tech Stack & Standards](#2-tech-stack--standards)
3. [Custom Resource Definition (CRD) Design](#3-custom-resource-definition-crd-design)
4. [Reconciliation Logic (The Loop)](#4-reconciliation-logic-the-loop)
5. [Development Roadmap (TDD Focused)](#5-development-roadmap-tdd-focused)
6. [Development Environment](#6-development-environment)
7. [Current Project Status](#7-current-project-status)
8. [Appendix D: Kubeconfig API Server URL](#appendix-d-kubeconfig-api-server-url)

---

## 1. Project Goal

### Vision

Building a **Kubernetes Operator** that manages short-lived (ephemeral) environments for Pull Requests. It bridges the gap between CI/CD pipelines and the Kubernetes cluster, enabling developers to:

- Automatically spin up isolated preview environments for every PR
- Share a unique URL with stakeholders for testing and review
- Automatically tear down environments after a configurable TTL (Time-To-Live)
- Ensure security through network isolation between environments

### Problem Statement

Modern development workflows require fast feedback loops. When a developer opens a Pull Request, they need:

1. **Isolated Environment** - A dedicated namespace with all dependencies deployed
2. **Accessible URL** - A unique endpoint to access the application
3. **Automatic Cleanup** - No manual intervention to delete stale environments
4. **Security** - Environments should not interfere with each other

### Solution

The `EphemeralEnv` Custom Resource abstracts all this complexity. A CI/CD pipeline simply creates an `EphemeralEnv` CR, and the operator handles:

```
PR Opened → CI creates EphemeralEnv CR → Operator provisions everything → Developer gets URL
PR Closed/TTL Expired → Operator cleans up everything automatically
```

---

## 2. Tech Stack & Standards

### Core Technologies

| Technology | Version | Purpose |
|------------|---------|---------|
| **Go (Golang)** | 1.22+ | Primary language - focus on idiomatic, clean code |
| **Kubebuilder** | v4.x | Operator scaffolding & controller-runtime integration |
| **Controller-Runtime** | v0.18+ | Core reconciliation framework |
| **Gateway API** | v1.0+ (Standard) | Modern networking via `HTTPRoute` (NOT Ingress) |
| **Helm SDK** | v3.14+ (`helm.sh/helm/v3`) | Chart deployment - **NO `os/exec` calls** |
| **Ginkgo** | v2.x | BDD-style testing framework |
| **Gomega** | v1.x | Assertion library |
| **EnvTest** | (controller-runtime) | Local Kubernetes control plane for testing |

### Coding Standards

#### Go Idioms to Follow

```go
// ✅ DO: Use explicit error handling
if err != nil {
    return ctrl.Result{}, fmt.Errorf("failed to create namespace: %w", err)
}

// ✅ DO: Use context propagation
func (r *EphemeralEnvReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)
    // ...
}

// ✅ DO: Use table-driven tests
var testCases = []struct {
    name     string
    input    string
    expected string
}{
    {"valid TTL", "2h", "2h0m0s"},
    {"default TTL", "", "24h0m0s"},
}

// ❌ DON'T: Use os/exec for Helm
// ❌ DON'T: Ignore errors
// ❌ DON'T: Use global state
```

#### Helm SDK Usage (NOT os/exec)

```go
// ✅ CORRECT: Use Helm SDK programmatically
import (
    "helm.sh/helm/v3/pkg/action"
    "helm.sh/helm/v3/pkg/chart/loader"
    "helm.sh/helm/v3/pkg/cli"
)

func (r *EphemeralEnvReconciler) installChart(ctx context.Context, namespace string, spec EphemeralEnvSpec) error {
    settings := cli.New()
    actionConfig := new(action.Configuration)
    
    if err := actionConfig.Init(settings.RESTClientGetter(), namespace, "secret", log.Printf); err != nil {
        return err
    }
    
    client := action.NewInstall(actionConfig)
    client.Namespace = namespace
    client.ReleaseName = "ephemeral-release"
    
    chart, err := loader.Load(spec.Helm.Chart)
    if err != nil {
        return err
    }
    
    _, err = client.Run(chart, spec.Helm.Values)
    return err
}

// ❌ WRONG: Never do this
// exec.Command("helm", "install", ...)
```

### Testing Methodology: TDD

We follow **strict Test-Driven Development**:

```
┌─────────────────────────────────────────────────────────────┐
│                      TDD Cycle                              │
│                                                             │
│    ┌─────────┐     ┌─────────┐     ┌─────────────┐         │
│    │  RED    │────▶│  GREEN  │────▶│  REFACTOR   │         │
│    │ (Test)  │     │ (Code)  │     │  (Clean)    │         │
│    └─────────┘     └─────────┘     └─────────────┘         │
│         │                                   │               │
│         └───────────────────────────────────┘               │
│                      Repeat                                 │
└─────────────────────────────────────────────────────────────┘

1. RED:    Write a failing test in `*_test.go`
2. GREEN:  Write minimal code to make the test pass
3. REFACTOR: Clean up while keeping tests green
```

---

## 3. Custom Resource Definition (CRD) Design

### Full Type Definition

```go
// api/v1alpha1/ephemeralenv_types.go

package v1alpha1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EphemeralEnvSpec defines the desired state of EphemeralEnv
type EphemeralEnvSpec struct {
    // Helm contains the Helm chart deployment configuration
    // +kubebuilder:validation:Required
    Helm HelmSpec `json:"helm"`

    // TTL is the time-to-live duration for the ephemeral environment
    // After this duration, the environment will be automatically deleted
    // +kubebuilder:validation:Pattern=`^([0-9]+h)?([0-9]+m)?([0-9]+s)?$`
    // +kubebuilder:default="24h"
    // +optional
    TTL string `json:"ttl,omitempty"`

    // Isolation enables NetworkPolicy to isolate the environment
    // When true, a deny-all NetworkPolicy is created in the namespace
    // +kubebuilder:default=true
    // +optional
    Isolation bool `json:"isolation,omitempty"`

    // Gateway configures the Gateway API HTTPRoute for external access
    // +kubebuilder:validation:Required
    Gateway GatewaySpec `json:"gateway"`
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

    // Version is the specific version of the Helm chart
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:Pattern=`^[0-9]+\.[0-9]+\.[0-9]+.*$`
    Version string `json:"version"`

    // Values contains key-value pairs to override chart values
    // +optional
    Values map[string]string `json:"values,omitempty"`
}

// GatewaySpec defines the Gateway API configuration
type GatewaySpec struct {
    // Name is the name of the parent Gateway resource
    // +kubebuilder:validation:Required
    Name string `json:"name"`

    // Namespace is the namespace where the Gateway resides
    // +kubebuilder:validation:Required
    Namespace string `json:"namespace"`

    // DomainPrefix is the subdomain prefix for the HTTPRoute
    // The full URL will be: {domainPrefix}.{gateway-domain}
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
    DomainPrefix string `json:"domainPrefix"`

    // TargetPort is the port of the backend service (default: 80)
    // +kubebuilder:default=80
    // +optional
    TargetPort int32 `json:"targetPort,omitempty"`

    // ServiceName is the name of the service to route to
    // +kubebuilder:validation:Required
    ServiceName string `json:"serviceName"`
}

// EphemeralEnvStatus defines the observed state of EphemeralEnv
type EphemeralEnvStatus struct {
    // Phase represents the current lifecycle phase of the EphemeralEnv
    // +kubebuilder:validation:Enum=Pending;Active;Failed;Expired
    Phase EphemeralEnvPhase `json:"phase,omitempty"`

    // ActiveNamespace is the name of the created namespace
    // Format: env-{cr-name}
    ActiveNamespace string `json:"activeNamespace,omitempty"`

    // AccessURL is the full URL to access the deployed application
    AccessURL string `json:"accessURL,omitempty"`

    // ExpirationTime is the timestamp when the environment will be deleted
    ExpirationTime *metav1.Time `json:"expirationTime,omitempty"`

    // HelmRelease contains information about the Helm release
    HelmRelease string `json:"helmRelease,omitempty"`

    // Conditions represent the latest available observations of the resource's state
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // LastReconcileTime is the timestamp of the last reconciliation
    LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

    // Message provides human-readable status information
    Message string `json:"message,omitempty"`
}

// EphemeralEnvPhase represents the lifecycle phase
// +kubebuilder:validation:Enum=Pending;Active;Failed;Expired
type EphemeralEnvPhase string

const (
    // PhasePending indicates the environment is being provisioned
    PhasePending EphemeralEnvPhase = "Pending"
    
    // PhaseActive indicates the environment is running and accessible
    PhaseActive EphemeralEnvPhase = "Active"
    
    // PhaseFailed indicates the environment failed to provision
    PhaseFailed EphemeralEnvPhase = "Failed"
    
    // PhaseExpired indicates the TTL has been exceeded
    PhaseExpired EphemeralEnvPhase = "Expired"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=eenv
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.accessURL`
//+kubebuilder:printcolumn:name="Expires",type=date,JSONPath=`.status.expirationTime`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// EphemeralEnv is the Schema for the ephemeralenvs API
type EphemeralEnv struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   EphemeralEnvSpec   `json:"spec,omitempty"`
    Status EphemeralEnvStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EphemeralEnvList contains a list of EphemeralEnv
type EphemeralEnvList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []EphemeralEnv `json:"items"`
}

func init() {
    SchemeBuilder.Register(&EphemeralEnv{}, &EphemeralEnvList{})
}
```

### Example Custom Resource

```yaml
apiVersion: ephemeral.example.com/v1alpha1
kind: EphemeralEnv
metadata:
  name: pr-123-frontend
  namespace: ephemeral-operator-system
spec:
  helm:
    repository: "https://charts.example.com"
    chart: "frontend-app"
    version: "1.2.3"
    values:
      image.tag: "pr-123-sha256abc"
      replicas: "1"
      resources.limits.memory: "256Mi"
  ttl: "2h"
  isolation: true
  gateway:
    name: "main-gateway"
    namespace: "gateway-system"
    domainPrefix: "pr-123"
    serviceName: "frontend-app"
    targetPort: 8080
```

### Status Example (After Reconciliation)

```yaml
status:
  phase: Active
  activeNamespace: "env-pr-123-frontend"
  accessURL: "https://pr-123.preview.example.com"
  expirationTime: "2026-01-29T16:30:00Z"
  helmRelease: "ephemeral-release"
  lastReconcileTime: "2026-01-29T14:30:00Z"
  message: "Environment is active and accessible"
  conditions:
    - type: NamespaceReady
      status: "True"
      lastTransitionTime: "2026-01-29T14:30:00Z"
    - type: HelmDeployed
      status: "True"
      lastTransitionTime: "2026-01-29T14:30:05Z"
    - type: NetworkPolicyApplied
      status: "True"
      lastTransitionTime: "2026-01-29T14:30:02Z"
    - type: HTTPRouteReady
      status: "True"
      lastTransitionTime: "2026-01-29T14:30:10Z"
```

---

## 4. Reconciliation Logic (The Loop)

### Flow Diagram

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                         RECONCILE LOOP                                        │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │  1. FETCH EphemeralEnv CR     │
                    │     Get(ctx, req.NamespacedName)│
                    └───────────────────────────────┘
                                    │
                    ┌───────────────┴───────────────┐
                    │ Not Found?                    │
                    │ (Deleted externally)          │
                    └───────────────┬───────────────┘
                           YES      │      NO
                            │       │       │
                            ▼       │       ▼
                    ┌───────────┐   │   ┌───────────────────────────────┐
                    │  RETURN   │   │   │  2. CHECK TTL EXPIRATION      │
                    │  (nil)    │   │   │  if time.Now() > ExpirationTime│
                    └───────────┘   │   └───────────────────────────────┘
                                    │               │
                                    │   ┌───────────┴───────────┐
                                    │   │ Expired?              │
                                    │   └───────────┬───────────┘
                                    │      YES      │      NO
                                    │       │       │       │
                                    │       ▼       │       ▼
                                    │   ┌───────────────┐   │
                                    │   │ DELETE ALL    │   │
                                    │   │ - Namespace   │   │
                                    │   │ - HTTPRoute   │   │
                                    │   │ - Set Expired │   │
                                    │   └───────────────┘   │
                                    │           │           │
                                    │           ▼           │
                                    │   ┌───────────────┐   │
                                    │   │ RETURN (nil)  │   │
                                    │   └───────────────┘   │
                                    │                       │
                                    │                       ▼
                                    │   ┌───────────────────────────────┐
                                    │   │  3. ENSURE NAMESPACE          │
                                    │   │  CreateOrUpdate("env-{name}") │
                                    │   └───────────────────────────────┘
                                    │                       │
                                    │                       ▼
                                    │   ┌───────────────────────────────┐
                                    │   │  4. ENSURE NETWORK POLICY     │
                                    │   │  if spec.Isolation == true    │
                                    │   │  Create deny-all NetworkPolicy│
                                    │   └───────────────────────────────┘
                                    │                       │
                                    │                       ▼
                                    │   ┌───────────────────────────────┐
                                    │   │  5. HELM INSTALL/UPGRADE      │
                                    │   │  Using Helm SDK (NOT exec)    │
                                    │   │  Deploy chart to namespace    │
                                    │   └───────────────────────────────┘
                                    │                       │
                                    │                       ▼
                                    │   ┌───────────────────────────────┐
                                    │   │  6. ENSURE HTTPRoute          │
                                    │   │  Create Gateway API route     │
                                    │   │  + ReferenceGrant if needed   │
                                    │   └───────────────────────────────┘
                                    │                       │
                                    │                       ▼
                                    │   ┌───────────────────────────────┐
                                    │   │  7. UPDATE STATUS             │
                                    │   │  - Phase: Active              │
                                    │   │  - AccessURL                  │
                                    │   │  - Conditions                 │
                                    │   └───────────────────────────────┘
                                    │                       │
                                    │                       ▼
                                    │   ┌───────────────────────────────┐
                                    │   │  8. REQUEUE                   │
                                    │   │  RequeueAfter: timeUntilTTL   │
                                    │   └───────────────────────────────┘
```

### Step-by-Step Implementation Details

#### Step 1: Fetch the EphemeralEnv CR

```go
func (r *EphemeralEnvReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)
    
    // Fetch the EphemeralEnv instance
    ephemeralEnv := &ephemeralv1alpha1.EphemeralEnv{}
    if err := r.Get(ctx, req.NamespacedName, ephemeralEnv); err != nil {
        if apierrors.IsNotFound(err) {
            // CR was deleted, nothing to do
            logger.Info("EphemeralEnv resource not found, likely deleted")
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, fmt.Errorf("failed to fetch EphemeralEnv: %w", err)
    }
    // ... continue
}
```

#### Step 2: TTL Check & Expiration Handling

```go
func (r *EphemeralEnvReconciler) handleTTL(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) (bool, ctrl.Result, error) {
    logger := log.FromContext(ctx)
    
    // Calculate expiration if not set
    if env.Status.ExpirationTime == nil {
        ttl, err := time.ParseDuration(env.Spec.TTL)
        if err != nil {
            ttl = 24 * time.Hour // Default
        }
        expirationTime := metav1.NewTime(env.CreationTimestamp.Add(ttl))
        env.Status.ExpirationTime = &expirationTime
    }
    
    // Check if expired
    if time.Now().After(env.Status.ExpirationTime.Time) {
        logger.Info("EphemeralEnv has expired, initiating cleanup")
        env.Status.Phase = ephemeralv1alpha1.PhaseExpired
        
        // Trigger cleanup
        if err := r.cleanupResources(ctx, env); err != nil {
            return true, ctrl.Result{RequeueAfter: 30 * time.Second}, err
        }
        
        return true, ctrl.Result{}, nil // Stop reconciliation
    }
    
    // Calculate requeue time
    requeueAfter := time.Until(env.Status.ExpirationTime.Time)
    return false, ctrl.Result{RequeueAfter: requeueAfter}, nil
}
```

#### Step 3: Namespace Lifecycle

```go
func (r *EphemeralEnvReconciler) ensureNamespace(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) error {
    logger := log.FromContext(ctx)
    namespaceName := fmt.Sprintf("env-%s", env.Name)
    
    ns := &corev1.Namespace{
        ObjectMeta: metav1.ObjectMeta{
            Name: namespaceName,
            Labels: map[string]string{
                "app.kubernetes.io/managed-by": "ephemeral-operator",
                "ephemeral.example.com/owner":  env.Name,
            },
        },
    }
    
    // Set owner reference for garbage collection
    if err := controllerutil.SetControllerReference(env, ns, r.Scheme); err != nil {
        return fmt.Errorf("failed to set owner reference: %w", err)
    }
    
    // CreateOrUpdate pattern
    result, err := controllerutil.CreateOrUpdate(ctx, r.Client, ns, func() error {
        // Mutation function - update labels if needed
        ns.Labels["ephemeral.example.com/owner"] = env.Name
        return nil
    })
    
    if err != nil {
        return fmt.Errorf("failed to ensure namespace: %w", err)
    }
    
    logger.Info("Namespace reconciled", "namespace", namespaceName, "result", result)
    env.Status.ActiveNamespace = namespaceName
    return nil
}
```

#### Step 4: Security - NetworkPolicy

```go
func (r *EphemeralEnvReconciler) ensureNetworkPolicy(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) error {
    if !env.Spec.Isolation {
        return nil // Isolation disabled
    }
    
    logger := log.FromContext(ctx)
    namespaceName := env.Status.ActiveNamespace
    
    // Deny-all ingress and egress policy
    policy := &networkingv1.NetworkPolicy{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "deny-all",
            Namespace: namespaceName,
        },
        Spec: networkingv1.NetworkPolicySpec{
            PodSelector: metav1.LabelSelector{}, // Selects all pods
            PolicyTypes: []networkingv1.PolicyType{
                networkingv1.PolicyTypeIngress,
                networkingv1.PolicyTypeEgress,
            },
            // Empty Ingress and Egress = deny all
            Ingress: []networkingv1.NetworkPolicyIngressRule{},
            Egress:  []networkingv1.NetworkPolicyEgressRule{},
        },
    }
    
    result, err := controllerutil.CreateOrUpdate(ctx, r.Client, policy, func() error {
        return nil
    })
    
    if err != nil {
        return fmt.Errorf("failed to ensure NetworkPolicy: %w", err)
    }
    
    logger.Info("NetworkPolicy reconciled", "namespace", namespaceName, "result", result)
    return nil
}
```

#### Step 5: Helm SDK Deployment

```go
func (r *EphemeralEnvReconciler) deployHelmChart(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) error {
    logger := log.FromContext(ctx)
    namespace := env.Status.ActiveNamespace
    
    // Initialize Helm configuration
    settings := cli.New()
    settings.SetNamespace(namespace)
    
    actionConfig := new(action.Configuration)
    if err := actionConfig.Init(
        r.RESTClientGetter,
        namespace,
        "secret", // Store releases as secrets
        func(format string, v ...interface{}) {
            logger.Info(fmt.Sprintf(format, v...))
        },
    ); err != nil {
        return fmt.Errorf("failed to init Helm config: %w", err)
    }
    
    // Check if release exists
    histClient := action.NewHistory(actionConfig)
    histClient.Max = 1
    _, err := histClient.Run(env.Name)
    
    if err == driver.ErrReleaseNotFound {
        // Install new release
        return r.installChart(ctx, actionConfig, env)
    } else if err != nil {
        return fmt.Errorf("failed to check release history: %w", err)
    }
    
    // Upgrade existing release
    return r.upgradeChart(ctx, actionConfig, env)
}

func (r *EphemeralEnvReconciler) installChart(ctx context.Context, cfg *action.Configuration, env *ephemeralv1alpha1.EphemeralEnv) error {
    client := action.NewInstall(cfg)
    client.Namespace = env.Status.ActiveNamespace
    client.ReleaseName = env.Name
    client.Wait = true
    client.Timeout = 5 * time.Minute
    
    // Locate chart
    chartPath, err := client.ChartPathOptions.LocateChart(
        env.Spec.Helm.Chart,
        cli.New(),
    )
    if err != nil {
        return fmt.Errorf("failed to locate chart: %w", err)
    }
    
    chart, err := loader.Load(chartPath)
    if err != nil {
        return fmt.Errorf("failed to load chart: %w", err)
    }
    
    // Convert values
    vals := make(map[string]interface{})
    for k, v := range env.Spec.Helm.Values {
        vals[k] = v
    }
    
    _, err = client.Run(chart, vals)
    if err != nil {
        return fmt.Errorf("failed to install chart: %w", err)
    }
    
    env.Status.HelmRelease = env.Name
    return nil
}
```

#### Step 6: Gateway API HTTPRoute

```go
func (r *EphemeralEnvReconciler) ensureHTTPRoute(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) error {
    logger := log.FromContext(ctx)
    
    // Build the hostname
    hostname := gatewayv1.Hostname(fmt.Sprintf("%s.preview.example.com", env.Spec.Gateway.DomainPrefix))
    
    // Parent Gateway reference
    gatewayNamespace := gatewayv1.Namespace(env.Spec.Gateway.Namespace)
    parentRef := gatewayv1.ParentReference{
        Name:      gatewayv1.ObjectName(env.Spec.Gateway.Name),
        Namespace: &gatewayNamespace,
    }
    
    // Backend service reference
    serviceNamespace := gatewayv1.Namespace(env.Status.ActiveNamespace)
    port := gatewayv1.PortNumber(env.Spec.Gateway.TargetPort)
    backendRef := gatewayv1.HTTPBackendRef{
        BackendRef: gatewayv1.BackendRef{
            BackendObjectReference: gatewayv1.BackendObjectReference{
                Name:      gatewayv1.ObjectName(env.Spec.Gateway.ServiceName),
                Namespace: &serviceNamespace,
                Port:      &port,
            },
        },
    }
    
    // Create HTTPRoute
    httpRoute := &gatewayv1.HTTPRoute{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("route-%s", env.Name),
            Namespace: env.Spec.Gateway.Namespace,
            Labels: map[string]string{
                "ephemeral.example.com/owner": env.Name,
            },
        },
        Spec: gatewayv1.HTTPRouteSpec{
            CommonRouteSpec: gatewayv1.CommonRouteSpec{
                ParentRefs: []gatewayv1.ParentReference{parentRef},
            },
            Hostnames: []gatewayv1.Hostname{hostname},
            Rules: []gatewayv1.HTTPRouteRule{
                {
                    BackendRefs: []gatewayv1.HTTPBackendRef{backendRef},
                },
            },
        },
    }
    
    result, err := controllerutil.CreateOrUpdate(ctx, r.Client, httpRoute, func() error {
        return nil
    })
    
    if err != nil {
        return fmt.Errorf("failed to ensure HTTPRoute: %w", err)
    }
    
    logger.Info("HTTPRoute reconciled", "name", httpRoute.Name, "result", result)
    env.Status.AccessURL = fmt.Sprintf("https://%s", hostname)
    return nil
}

// ReferenceGrant for cross-namespace routing
func (r *EphemeralEnvReconciler) ensureReferenceGrant(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) error {
    // Only needed if HTTPRoute and Service are in different namespaces
    if env.Spec.Gateway.Namespace == env.Status.ActiveNamespace {
        return nil
    }
    
    grant := &gatewayv1beta1.ReferenceGrant{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("grant-%s", env.Name),
            Namespace: env.Status.ActiveNamespace,
        },
        Spec: gatewayv1beta1.ReferenceGrantSpec{
            From: []gatewayv1beta1.ReferenceGrantFrom{
                {
                    Group:     "gateway.networking.k8s.io",
                    Kind:      "HTTPRoute",
                    Namespace: gatewayv1beta1.Namespace(env.Spec.Gateway.Namespace),
                },
            },
            To: []gatewayv1beta1.ReferenceGrantTo{
                {
                    Group: "",
                    Kind:  "Service",
                },
            },
        },
    }
    
    _, err := controllerutil.CreateOrUpdate(ctx, r.Client, grant, func() error {
        return nil
    })
    
    return err
}
```

#### Step 7: Status Update

```go
func (r *EphemeralEnvReconciler) updateStatus(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv, phase ephemeralv1alpha1.EphemeralEnvPhase, message string) error {
    env.Status.Phase = phase
    env.Status.Message = message
    env.Status.LastReconcileTime = &metav1.Time{Time: time.Now()}
    
    // Update conditions
    meta.SetStatusCondition(&env.Status.Conditions, metav1.Condition{
        Type:               "Ready",
        Status:             metav1.ConditionTrue,
        Reason:             string(phase),
        Message:            message,
        LastTransitionTime: metav1.Now(),
    })
    
    return r.Status().Update(ctx, env)
}
```

---

## 5. Development Roadmap (TDD Focused)

### Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         DEVELOPMENT PHASES                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│  Phase 1    │  Phase 2    │  Phase 3     │  Phase 4    │  Phase 5          │
│  SCAFFOLD   │  NAMESPACE  │  SECURITY    │  TTL        │  HELM+GATEWAY     │
│             │             │              │             │                    │
│  API Types  │  Create NS  │  NetPolicy   │  Expiration │  Chart Deploy     │
│  CRD Gen    │  Owner Ref  │  Isolation   │  Cleanup    │  HTTPRoute        │
│  Basic Test │  Labels     │  Conditions  │  Requeue    │  Full E2E         │
└─────────────────────────────────────────────────────────────────────────────┘
     Week 1       Week 2        Week 3        Week 4         Week 5-6
```

---

### Phase 1: Scaffolding & API Definition

**Goal:** Set up Kubebuilder project and define CRD types.

#### RED (Write Failing Tests First)

```go
// internal/controller/ephemeralenv_controller_test.go

var _ = Describe("EphemeralEnv Controller", func() {
    Context("When creating an EphemeralEnv", func() {
        It("Should accept valid spec fields", func() {
            env := &ephemeralv1alpha1.EphemeralEnv{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-env",
                    Namespace: "default",
                },
                Spec: ephemeralv1alpha1.EphemeralEnvSpec{
                    TTL:       "2h",
                    Isolation: true,
                    Helm: ephemeralv1alpha1.HelmSpec{
                        Repository: "https://charts.example.com",
                        Chart:      "my-app",
                        Version:    "1.0.0",
                    },
                    Gateway: ephemeralv1alpha1.GatewaySpec{
                        Name:         "main-gateway",
                        Namespace:    "gateway-system",
                        DomainPrefix: "test",
                        ServiceName:  "my-app",
                    },
                },
            }
            
            Expect(k8sClient.Create(ctx, env)).Should(Succeed())
            
            // Verify defaults are applied
            fetched := &ephemeralv1alpha1.EphemeralEnv{}
            Expect(k8sClient.Get(ctx, types.NamespacedName{
                Name:      "test-env",
                Namespace: "default",
            }, fetched)).Should(Succeed())
            
            Expect(fetched.Spec.TTL).Should(Equal("2h"))
            Expect(fetched.Spec.Isolation).Should(BeTrue())
        })
        
        It("Should apply default TTL of 24h when not specified", func() {
            env := &ephemeralv1alpha1.EphemeralEnv{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-env-defaults",
                    Namespace: "default",
                },
                Spec: ephemeralv1alpha1.EphemeralEnvSpec{
                    Helm: ephemeralv1alpha1.HelmSpec{
                        Repository: "https://charts.example.com",
                        Chart:      "my-app",
                        Version:    "1.0.0",
                    },
                    Gateway: ephemeralv1alpha1.GatewaySpec{
                        Name:         "main-gateway",
                        Namespace:    "gateway-system",
                        DomainPrefix: "test-defaults",
                        ServiceName:  "my-app",
                    },
                },
            }
            
            Expect(k8sClient.Create(ctx, env)).Should(Succeed())
            
            fetched := &ephemeralv1alpha1.EphemeralEnv{}
            Expect(k8sClient.Get(ctx, types.NamespacedName{
                Name:      "test-env-defaults",
                Namespace: "default",
            }, fetched)).Should(Succeed())
            
            Expect(fetched.Spec.TTL).Should(Equal("24h"))
        })
    })
})
```

#### GREEN (Implementation)

```bash
# Scaffold the project
kubebuilder init --domain example.com --repo github.com/yourorg/ephemeral-operator

# Create the API
kubebuilder create api --group ephemeral --version v1alpha1 --kind EphemeralEnv

# Edit api/v1alpha1/ephemeralenv_types.go with the structs from Section 3

# Generate CRD manifests
make generate
make manifests
```

#### Tasks

- [ ] Initialize Kubebuilder project
- [ ] Define `EphemeralEnvSpec` struct with all fields
- [ ] Define `EphemeralEnvStatus` struct
- [ ] Add Kubebuilder markers for defaults and validation
- [ ] Generate CRD YAML
- [ ] Write and pass basic type tests

---

### Phase 2: Namespace Lifecycle

**Goal:** Controller creates a namespace when an EphemeralEnv CR is created.

#### RED (Write Failing Tests First)

```go
// internal/controller/ephemeralenv_controller_test.go

var _ = Describe("EphemeralEnv Controller - Namespace", func() {
    Context("When an EphemeralEnv is created", func() {
        It("Should create a namespace with correct naming convention", func() {
            envName := "pr-456"
            expectedNamespace := "env-pr-456"
            
            env := createTestEphemeralEnv(envName)
            Expect(k8sClient.Create(ctx, env)).Should(Succeed())
            
            // Wait for reconciliation
            Eventually(func() bool {
                ns := &corev1.Namespace{}
                err := k8sClient.Get(ctx, types.NamespacedName{Name: expectedNamespace}, ns)
                return err == nil
            }, timeout, interval).Should(BeTrue())
            
            // Verify labels
            ns := &corev1.Namespace{}
            Expect(k8sClient.Get(ctx, types.NamespacedName{Name: expectedNamespace}, ns)).Should(Succeed())
            Expect(ns.Labels["app.kubernetes.io/managed-by"]).Should(Equal("ephemeral-operator"))
            Expect(ns.Labels["ephemeral.example.com/owner"]).Should(Equal(envName))
        })
        
        It("Should set owner reference on the namespace", func() {
            envName := "pr-789"
            expectedNamespace := "env-pr-789"
            
            env := createTestEphemeralEnv(envName)
            Expect(k8sClient.Create(ctx, env)).Should(Succeed())
            
            Eventually(func() bool {
                ns := &corev1.Namespace{}
                err := k8sClient.Get(ctx, types.NamespacedName{Name: expectedNamespace}, ns)
                if err != nil {
                    return false
                }
                // Check owner reference
                for _, ref := range ns.OwnerReferences {
                    if ref.Kind == "EphemeralEnv" && ref.Name == envName {
                        return true
                    }
                }
                return false
            }, timeout, interval).Should(BeTrue())
        })
        
        It("Should update status with ActiveNamespace", func() {
            envName := "pr-status-test"
            
            env := createTestEphemeralEnv(envName)
            Expect(k8sClient.Create(ctx, env)).Should(Succeed())
            
            Eventually(func() string {
                fetched := &ephemeralv1alpha1.EphemeralEnv{}
                k8sClient.Get(ctx, types.NamespacedName{
                    Name:      envName,
                    Namespace: "default",
                }, fetched)
                return fetched.Status.ActiveNamespace
            }, timeout, interval).Should(Equal("env-pr-status-test"))
        })
    })
})
```

#### GREEN (Implementation)

```go
// internal/controller/ephemeralenv_controller.go

func (r *EphemeralEnvReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)
    
    // Fetch CR
    env := &ephemeralv1alpha1.EphemeralEnv{}
    if err := r.Get(ctx, req.NamespacedName, env); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // Ensure namespace
    if err := r.ensureNamespace(ctx, env); err != nil {
        logger.Error(err, "Failed to ensure namespace")
        return ctrl.Result{}, err
    }
    
    // Update status
    if err := r.Status().Update(ctx, env); err != nil {
        return ctrl.Result{}, err
    }
    
    return ctrl.Result{}, nil
}
```

#### Tasks

- [ ] Write test: CR creation triggers namespace creation
- [ ] Write test: Namespace has correct name format `env-{name}`
- [ ] Write test: Namespace has owner reference
- [ ] Write test: Status.ActiveNamespace is updated
- [ ] Implement `ensureNamespace()` function
- [ ] Run tests and verify all pass

---

### Phase 3: Security & NetworkPolicy

**Goal:** When `Isolation=true`, create a deny-all NetworkPolicy.

#### RED (Write Failing Tests First)

```go
var _ = Describe("EphemeralEnv Controller - Security", func() {
    Context("When Isolation is enabled", func() {
        It("Should create a deny-all NetworkPolicy", func() {
            envName := "pr-isolated"
            
            env := createTestEphemeralEnv(envName)
            env.Spec.Isolation = true
            Expect(k8sClient.Create(ctx, env)).Should(Succeed())
            
            // Wait for namespace
            Eventually(func() error {
                ns := &corev1.Namespace{}
                return k8sClient.Get(ctx, types.NamespacedName{Name: "env-" + envName}, ns)
            }, timeout, interval).Should(Succeed())
            
            // Check NetworkPolicy exists
            Eventually(func() bool {
                np := &networkingv1.NetworkPolicy{}
                err := k8sClient.Get(ctx, types.NamespacedName{
                    Name:      "deny-all",
                    Namespace: "env-" + envName,
                }, np)
                return err == nil
            }, timeout, interval).Should(BeTrue())
            
            // Verify policy denies all
            np := &networkingv1.NetworkPolicy{}
            Expect(k8sClient.Get(ctx, types.NamespacedName{
                Name:      "deny-all",
                Namespace: "env-" + envName,
            }, np)).Should(Succeed())
            
            Expect(np.Spec.PolicyTypes).Should(ContainElements(
                networkingv1.PolicyTypeIngress,
                networkingv1.PolicyTypeEgress,
            ))
            Expect(np.Spec.Ingress).Should(BeEmpty())
            Expect(np.Spec.Egress).Should(BeEmpty())
        })
        
        It("Should update condition NetworkPolicyApplied", func() {
            envName := "pr-isolated-condition"
            
            env := createTestEphemeralEnv(envName)
            env.Spec.Isolation = true
            Expect(k8sClient.Create(ctx, env)).Should(Succeed())
            
            Eventually(func() bool {
                fetched := &ephemeralv1alpha1.EphemeralEnv{}
                k8sClient.Get(ctx, types.NamespacedName{
                    Name:      envName,
                    Namespace: "default",
                }, fetched)
                
                for _, cond := range fetched.Status.Conditions {
                    if cond.Type == "NetworkPolicyApplied" && cond.Status == metav1.ConditionTrue {
                        return true
                    }
                }
                return false
            }, timeout, interval).Should(BeTrue())
        })
    })
    
    Context("When Isolation is disabled", func() {
        It("Should NOT create a NetworkPolicy", func() {
            envName := "pr-not-isolated"
            
            env := createTestEphemeralEnv(envName)
            env.Spec.Isolation = false
            Expect(k8sClient.Create(ctx, env)).Should(Succeed())
            
            // Wait for namespace
            Eventually(func() error {
                ns := &corev1.Namespace{}
                return k8sClient.Get(ctx, types.NamespacedName{Name: "env-" + envName}, ns)
            }, timeout, interval).Should(Succeed())
            
            // NetworkPolicy should NOT exist
            Consistently(func() bool {
                np := &networkingv1.NetworkPolicy{}
                err := k8sClient.Get(ctx, types.NamespacedName{
                    Name:      "deny-all",
                    Namespace: "env-" + envName,
                }, np)
                return apierrors.IsNotFound(err)
            }, "3s", interval).Should(BeTrue())
        })
    })
})
```

#### GREEN (Implementation)

```go
func (r *EphemeralEnvReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... previous code ...
    
    // Ensure NetworkPolicy if isolation is enabled
    if err := r.ensureNetworkPolicy(ctx, env); err != nil {
        return ctrl.Result{}, err
    }
    
    // ... rest of code ...
}
```

#### Tasks

- [ ] Write test: Isolation=true creates deny-all NetworkPolicy
- [ ] Write test: Isolation=false does NOT create NetworkPolicy
- [ ] Write test: Condition `NetworkPolicyApplied` is updated
- [ ] Implement `ensureNetworkPolicy()` function
- [ ] Run tests and verify all pass

---

### Phase 4: TTL & Automatic Cleanup

**Goal:** Environments expire and get cleaned up automatically.

#### RED (Write Failing Tests First)

```go
var _ = Describe("EphemeralEnv Controller - TTL", func() {
    Context("When TTL expires", func() {
        It("Should delete the namespace after TTL", func() {
            envName := "pr-short-lived"
            
            env := createTestEphemeralEnv(envName)
            env.Spec.TTL = "2s" // Very short for testing
            Expect(k8sClient.Create(ctx, env)).Should(Succeed())
            
            // Verify namespace is created
            Eventually(func() error {
                ns := &corev1.Namespace{}
                return k8sClient.Get(ctx, types.NamespacedName{Name: "env-" + envName}, ns)
            }, timeout, interval).Should(Succeed())
            
            // Wait for TTL + buffer
            time.Sleep(5 * time.Second)
            
            // Trigger reconciliation
            // (In real envtest, this happens automatically)
            
            // Namespace should be deleted
            Eventually(func() bool {
                ns := &corev1.Namespace{}
                err := k8sClient.Get(ctx, types.NamespacedName{Name: "env-" + envName}, ns)
                return apierrors.IsNotFound(err)
            }, "30s", interval).Should(BeTrue())
        })
        
        It("Should set Phase to Expired", func() {
            envName := "pr-expire-phase"
            
            env := createTestEphemeralEnv(envName)
            env.Spec.TTL = "1s"
            Expect(k8sClient.Create(ctx, env)).Should(Succeed())
            
            Eventually(func() ephemeralv1alpha1.EphemeralEnvPhase {
                fetched := &ephemeralv1alpha1.EphemeralEnv{}
                k8sClient.Get(ctx, types.NamespacedName{
                    Name:      envName,
                    Namespace: "default",
                }, fetched)
                return fetched.Status.Phase
            }, "30s", interval).Should(Equal(ephemeralv1alpha1.PhaseExpired))
        })
        
        It("Should calculate correct ExpirationTime", func() {
            envName := "pr-expiration-calc"
            
            env := createTestEphemeralEnv(envName)
            env.Spec.TTL = "1h"
            
            beforeCreate := time.Now()
            Expect(k8sClient.Create(ctx, env)).Should(Succeed())
            
            Eventually(func() bool {
                fetched := &ephemeralv1alpha1.EphemeralEnv{}
                k8sClient.Get(ctx, types.NamespacedName{
                    Name:      envName,
                    Namespace: "default",
                }, fetched)
                
                if fetched.Status.ExpirationTime == nil {
                    return false
                }
                
                // Expiration should be ~1h from creation
                expectedMin := beforeCreate.Add(1 * time.Hour)
                expectedMax := beforeCreate.Add(1*time.Hour + 30*time.Second)
                
                return fetched.Status.ExpirationTime.Time.After(expectedMin) &&
                       fetched.Status.ExpirationTime.Time.Before(expectedMax)
            }, timeout, interval).Should(BeTrue())
        })
    })
    
    Context("When TTL has not expired", func() {
        It("Should requeue for later reconciliation", func() {
            envName := "pr-requeue"
            
            env := createTestEphemeralEnv(envName)
            env.Spec.TTL = "24h"
            Expect(k8sClient.Create(ctx, env)).Should(Succeed())
            
            // Phase should be Active, not Expired
            Eventually(func() ephemeralv1alpha1.EphemeralEnvPhase {
                fetched := &ephemeralv1alpha1.EphemeralEnv{}
                k8sClient.Get(ctx, types.NamespacedName{
                    Name:      envName,
                    Namespace: "default",
                }, fetched)
                return fetched.Status.Phase
            }, timeout, interval).Should(Equal(ephemeralv1alpha1.PhaseActive))
            
            // Should remain active
            Consistently(func() ephemeralv1alpha1.EphemeralEnvPhase {
                fetched := &ephemeralv1alpha1.EphemeralEnv{}
                k8sClient.Get(ctx, types.NamespacedName{
                    Name:      envName,
                    Namespace: "default",
                }, fetched)
                return fetched.Status.Phase
            }, "5s", interval).Should(Equal(ephemeralv1alpha1.PhaseActive))
        })
    })
})
```

#### GREEN (Implementation)

```go
func (r *EphemeralEnvReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... fetch CR ...
    
    // Handle TTL
    expired, result, err := r.handleTTL(ctx, env)
    if err != nil {
        return result, err
    }
    if expired {
        return result, nil
    }
    
    // ... rest of reconciliation ...
    
    // Return with RequeueAfter for TTL
    return ctrl.Result{RequeueAfter: time.Until(env.Status.ExpirationTime.Time)}, nil
}

func (r *EphemeralEnvReconciler) cleanupResources(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) error {
    // Delete namespace (cascading delete will clean up resources)
    ns := &corev1.Namespace{}
    if err := r.Get(ctx, types.NamespacedName{Name: env.Status.ActiveNamespace}, ns); err == nil {
        if err := r.Delete(ctx, ns); err != nil {
            return err
        }
    }
    
    // Delete HTTPRoute
    // ... implementation ...
    
    return nil
}
```

#### Tasks

- [ ] Write test: ExpirationTime is calculated correctly
- [ ] Write test: Phase becomes Expired after TTL
- [ ] Write test: Namespace is deleted on expiration
- [ ] Write test: Non-expired envs stay Active
- [ ] Implement `handleTTL()` function
- [ ] Implement `cleanupResources()` function
- [ ] Run tests and verify all pass

---

### Phase 5: Helm & Gateway API Integration

**Goal:** Full deployment with Helm SDK and Gateway API routing.

#### RED (Write Failing Tests First)

```go
var _ = Describe("EphemeralEnv Controller - Helm & Gateway", func() {
    Context("Helm Chart Deployment", func() {
        It("Should deploy a Helm chart to the environment namespace", func() {
            envName := "pr-helm-deploy"
            
            env := createTestEphemeralEnv(envName)
            env.Spec.Helm = ephemeralv1alpha1.HelmSpec{
                Repository: "https://charts.bitnami.com/bitnami",
                Chart:      "nginx",
                Version:    "15.0.0",
                Values: map[string]string{
                    "replicaCount": "1",
                },
            }
            Expect(k8sClient.Create(ctx, env)).Should(Succeed())
            
            // Check Helm release status is updated
            Eventually(func() string {
                fetched := &ephemeralv1alpha1.EphemeralEnv{}
                k8sClient.Get(ctx, types.NamespacedName{
                    Name:      envName,
                    Namespace: "default",
                }, fetched)
                return fetched.Status.HelmRelease
            }, "60s", interval).Should(Equal(envName))
            
            // Verify HelmDeployed condition
            Eventually(func() bool {
                fetched := &ephemeralv1alpha1.EphemeralEnv{}
                k8sClient.Get(ctx, types.NamespacedName{
                    Name:      envName,
                    Namespace: "default",
                }, fetched)
                
                for _, cond := range fetched.Status.Conditions {
                    if cond.Type == "HelmDeployed" && cond.Status == metav1.ConditionTrue {
                        return true
                    }
                }
                return false
            }, "60s", interval).Should(BeTrue())
        })
    })
    
    Context("Gateway API HTTPRoute", func() {
        BeforeEach(func() {
            // Create test Gateway
            gateway := &gatewayv1.Gateway{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-gateway",
                    Namespace: "gateway-system",
                },
                Spec: gatewayv1.GatewaySpec{
                    GatewayClassName: "test-class",
                    Listeners: []gatewayv1.Listener{
                        {
                            Name:     "http",
                            Port:     80,
                            Protocol: gatewayv1.HTTPProtocolType,
                        },
                    },
                },
            }
            Expect(k8sClient.Create(ctx, gateway)).Should(Succeed())
        })
        
        It("Should create HTTPRoute pointing to the service", func() {
            envName := "pr-gateway"
            
            env := createTestEphemeralEnv(envName)
            env.Spec.Gateway = ephemeralv1alpha1.GatewaySpec{
                Name:         "test-gateway",
                Namespace:    "gateway-system",
                DomainPrefix: "pr-gateway",
                ServiceName:  "my-app",
                TargetPort:   8080,
            }
            Expect(k8sClient.Create(ctx, env)).Should(Succeed())
            
            // Check HTTPRoute is created
            Eventually(func() bool {
                route := &gatewayv1.HTTPRoute{}
                err := k8sClient.Get(ctx, types.NamespacedName{
                    Name:      "route-" + envName,
                    Namespace: "gateway-system",
                }, route)
                return err == nil
            }, timeout, interval).Should(BeTrue())
            
            // Verify route configuration
            route := &gatewayv1.HTTPRoute{}
            Expect(k8sClient.Get(ctx, types.NamespacedName{
                Name:      "route-" + envName,
                Namespace: "gateway-system",
            }, route)).Should(Succeed())
            
            Expect(route.Spec.Hostnames).Should(ContainElement(
                gatewayv1.Hostname("pr-gateway.preview.example.com"),
            ))
        })
        
        It("Should create ReferenceGrant for cross-namespace routing", func() {
            envName := "pr-ref-grant"
            
            env := createTestEphemeralEnv(envName)
            env.Spec.Gateway = ephemeralv1alpha1.GatewaySpec{
                Name:         "test-gateway",
                Namespace:    "gateway-system", // Different from env namespace
                DomainPrefix: "pr-ref-grant",
                ServiceName:  "my-app",
            }
            Expect(k8sClient.Create(ctx, env)).Should(Succeed())
            
            // Check ReferenceGrant is created in the service namespace
            Eventually(func() bool {
                grant := &gatewayv1beta1.ReferenceGrant{}
                err := k8sClient.Get(ctx, types.NamespacedName{
                    Name:      "grant-" + envName,
                    Namespace: "env-" + envName,
                }, grant)
                return err == nil
            }, timeout, interval).Should(BeTrue())
        })
        
        It("Should update AccessURL in status", func() {
            envName := "pr-url"
            
            env := createTestEphemeralEnv(envName)
            env.Spec.Gateway.DomainPrefix = "pr-url"
            Expect(k8sClient.Create(ctx, env)).Should(Succeed())
            
            Eventually(func() string {
                fetched := &ephemeralv1alpha1.EphemeralEnv{}
                k8sClient.Get(ctx, types.NamespacedName{
                    Name:      envName,
                    Namespace: "default",
                }, fetched)
                return fetched.Status.AccessURL
            }, timeout, interval).Should(Equal("https://pr-url.preview.example.com"))
        })
    })
    
    Context("Full E2E Flow", func() {
        It("Should complete full lifecycle: Create -> Active -> Expired -> Cleaned", func() {
            envName := "pr-e2e"
            
            env := &ephemeralv1alpha1.EphemeralEnv{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      envName,
                    Namespace: "default",
                },
                Spec: ephemeralv1alpha1.EphemeralEnvSpec{
                    TTL:       "10s",
                    Isolation: true,
                    Helm: ephemeralv1alpha1.HelmSpec{
                        Repository: "https://charts.example.com",
                        Chart:      "nginx",
                        Version:    "1.0.0",
                    },
                    Gateway: ephemeralv1alpha1.GatewaySpec{
                        Name:         "test-gateway",
                        Namespace:    "gateway-system",
                        DomainPrefix: "pr-e2e",
                        ServiceName:  "nginx",
                        TargetPort:   80,
                    },
                },
            }
            
            // CREATE
            Expect(k8sClient.Create(ctx, env)).Should(Succeed())
            
            // PENDING -> ACTIVE
            Eventually(func() ephemeralv1alpha1.EphemeralEnvPhase {
                fetched := &ephemeralv1alpha1.EphemeralEnv{}
                k8sClient.Get(ctx, types.NamespacedName{
                    Name:      envName,
                    Namespace: "default",
                }, fetched)
                return fetched.Status.Phase
            }, "60s", interval).Should(Equal(ephemeralv1alpha1.PhaseActive))
            
            // Verify all resources exist
            Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "env-" + envName}, &corev1.Namespace{})).Should(Succeed())
            Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "deny-all", Namespace: "env-" + envName}, &networkingv1.NetworkPolicy{})).Should(Succeed())
            Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "route-" + envName, Namespace: "gateway-system"}, &gatewayv1.HTTPRoute{})).Should(Succeed())
            
            // ACTIVE -> EXPIRED
            Eventually(func() ephemeralv1alpha1.EphemeralEnvPhase {
                fetched := &ephemeralv1alpha1.EphemeralEnv{}
                k8sClient.Get(ctx, types.NamespacedName{
                    Name:      envName,
                    Namespace: "default",
                }, fetched)
                return fetched.Status.Phase
            }, "30s", interval).Should(Equal(ephemeralv1alpha1.PhaseExpired))
            
            // CLEANUP - Namespace should be deleted
            Eventually(func() bool {
                err := k8sClient.Get(ctx, types.NamespacedName{Name: "env-" + envName}, &corev1.Namespace{})
                return apierrors.IsNotFound(err)
            }, "30s", interval).Should(BeTrue())
        })
    })
})
```



#### GREEN (Implementation)

Full implementation of:
- `deployHelmChart()` using Helm SDK
- `ensureHTTPRoute()` with Gateway API
- `ensureReferenceGrant()` for cross-namespace routing
- Complete status updates with all conditions

#### Tasks

- [ ] Write test: Helm chart deploys to namespace
- [ ] Write test: HelmDeployed condition is set
- [ ] Write test: HTTPRoute is created with correct hostname
- [ ] Write test: ReferenceGrant is created for cross-namespace
- [ ] Write test: AccessURL is populated in status
- [ ] Write test: Full E2E lifecycle
- [ ] Implement Helm SDK integration
- [ ] Implement Gateway API HTTPRoute creation
- [ ] Implement ReferenceGrant creation
- [ ] Run all tests and verify full integration

---

### Phase 6: Local Development Setup

**Goal:** Automate local development environment setup with Minikube, Gateway API, and Envoy Gateway for efficient operator testing.

#### Overview

Before we can properly test the Gateway API integration in a real cluster, we need a reproducible local development environment. This phase creates automation scripts to set up:

1. **Minikube** - Local Kubernetes cluster with adequate resources
2. **Metrics Server** - Required for Phase 7 (resource monitoring)
3. **Gateway API CRDs** - Standard v1.0+ CRDs for HTTPRoute, Gateway, GatewayClass
4. **Envoy Gateway** - Production-ready Gateway API implementation

#### Files Created

| File | Purpose |
|------|---------|
| `scripts/setup-local-lab.sh` | Idempotent Bash script to bootstrap the environment |
| `config/samples/gateway_class_setup.yaml` | GatewayClass and Gateway manifests |

#### Script: `scripts/setup-local-lab.sh`

```bash
#!/bin/bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
MINIKUBE_CPUS=2
MINIKUBE_MEMORY=4096
GATEWAY_API_VERSION="v1.2.1"
ENVOY_GATEWAY_VERSION="v1.2.4"

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Check if Minikube is running
check_minikube() {
    if minikube status --format='{{.Host}}' 2>/dev/null | grep -q "Running"; then
        return 0
    fi
    return 1
}

# Start Minikube if not running
setup_minikube() {
    if check_minikube; then
        log_info "Minikube is already running"
    else
        log_info "Starting Minikube with ${MINIKUBE_CPUS} CPUs and ${MINIKUBE_MEMORY}MB RAM..."
        minikube start --cpus=${MINIKUBE_CPUS} --memory=${MINIKUBE_MEMORY}
    fi
    
    # Enable metrics-server addon
    log_info "Enabling metrics-server addon..."
    minikube addons enable metrics-server
}

# Install Gateway API CRDs
install_gateway_api() {
    log_info "Installing Gateway API CRDs (${GATEWAY_API_VERSION})..."
    kubectl apply -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml"
}

# Install Envoy Gateway
install_envoy_gateway() {
    log_info "Installing Envoy Gateway (${ENVOY_GATEWAY_VERSION})..."
    
    # Add Envoy Gateway Helm repo if not exists
    if ! helm repo list | grep -q "envoy-gateway"; then
        helm repo add envoy-gateway https://envoy-gateway.github.io/helm-charts
    fi
    helm repo update
    
    # Install or upgrade Envoy Gateway
    helm upgrade --install envoy-gateway envoy-gateway/gateway-helm \
        --version "${ENVOY_GATEWAY_VERSION}" \
        --namespace envoy-gateway-system \
        --create-namespace \
        --wait
    
    # Wait for controller to be ready
    log_info "Waiting for Envoy Gateway controller to be ready..."
    kubectl wait --namespace envoy-gateway-system \
        --for=condition=available \
        --timeout=120s \
        deployment/envoy-gateway
}

# Main
main() {
    log_info "=== Ephemeral Operator Local Lab Setup ==="
    
    setup_minikube
    install_gateway_api
    install_envoy_gateway
    
    log_info "=== Setup Complete ==="
    log_info "Apply GatewayClass and Gateway:"
    log_info "  kubectl apply -f config/samples/gateway_class_setup.yaml"
}

main "$@"
```

#### Manifest: `config/samples/gateway_class_setup.yaml`

```yaml
---
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: eg
spec:
  controllerName: gateway.envoyproxy.io/gatewayclass-controller
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: main-gateway
  namespace: default
spec:
  gatewayClassName: eg
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
```

#### Usage

```bash
# 1. Run the setup script
./scripts/setup-local-lab.sh

# 2. Apply GatewayClass and Gateway
kubectl apply -f config/samples/gateway_class_setup.yaml

# 3. Verify setup
kubectl get gatewayclass
kubectl get gateway -A
kubectl get pods -n envoy-gateway-system

# 4. Run the operator locally
make run

# 5. Create a test EphemeralEnv
kubectl apply -f config/samples/ephemeral_v1alpha1_ephemeralenv.yaml
```

#### Verification Checklist

- [ ] Minikube running with 4GB RAM, 2 CPUs
- [ ] `metrics-server` addon enabled
- [ ] Gateway API CRDs installed (`kubectl get crd gateways.gateway.networking.k8s.io`)
- [ ] Envoy Gateway controller running (`kubectl get pods -n envoy-gateway-system`)
- [ ] GatewayClass `eg` exists and accepted
- [ ] Gateway `main-gateway` exists in `default` namespace

---

### Phase 7: Automatic Cleanup with Finalizers

**Goal:** Implement automatic cleanup of all resources when an EphemeralEnv is deleted, using the Kubernetes finalizer pattern.

#### Overview

When a user deletes an EphemeralEnv CR (either manually or via TTL expiration), we need to ensure all associated resources are properly cleaned up:
- HTTPRoute
- Helm release
- Namespace (which cascades to NetworkPolicy, ReferenceGrant, etc.)

The finalizer pattern ensures the controller has a chance to perform cleanup before Kubernetes removes the CR.

#### Implementation

1. **Add Finalizer on Creation** - When reconciling a new EphemeralEnv, add our finalizer
2. **Check for Deletion** - At the start of reconciliation, check if the CR is being deleted
3. **Perform Cleanup** - If being deleted, clean up all resources in order
4. **Remove Finalizer** - After cleanup, remove the finalizer to allow deletion to complete

#### Finalizer Name
```go
const ephemeralEnvFinalizer = "ephemeral.ephemeralenv.io/finalizer"
```

#### Cleanup Order
1. Delete admin HTTPRoute (if exists)
2. Delete main HTTPRoute
3. Uninstall Helm release
4. Delete namespace (cascades to NetworkPolicy, ReferenceGrant)
5. Remove finalizer

#### Tasks

- [x] Add finalizer constant
- [x] Add finalizer when CR is created
- [x] Check for deletion at start of reconciliation
- [x] Implement cleanup function with proper ordering
- [x] Remove finalizer after successful cleanup
- [x] Test deletion workflow

---

### Phase 8: Service Catalog & Templates API

**Goal:** Provide a templates system so users don't have to manually enter Helm repository URLs and chart details when creating environments. Also support multi-chart deployments.

#### Phase 8.1: UI Form Fix

Fixed the UI form to include all required Gateway fields:
- Gateway Name
- Gateway Namespace  
- Domain Prefix (auto-populated from environment name)

#### Phase 8.2: EnvironmentTemplate CRD

**Overview:**
Created a Kubernetes Custom Resource Definition (CRD) for templates. Templates are now first-class Kubernetes resources that can be managed via kubectl.

**New CRD: EnvironmentTemplate**

```yaml
apiVersion: ephemeral.ephemeralenv.io/v1alpha1
kind: EnvironmentTemplate
metadata:
  name: fullstack-webapp
  namespace: ephemeral-system
spec:
  displayName: "Full Stack Web Application"
  description: "Complete app with frontend and database"
  icon: "🚀"
  tags:
    - fullstack
    - production-like
  defaultTTL: "4h"
  components:
    - name: frontend
      repository: https://charts.bitnami.com/bitnami
      chart: nginx
      version: "15.0.0"
      serviceName: frontend-nginx
      servicePort: 80
      primary: true
    - name: database
      repository: https://charts.bitnami.com/bitnami
      chart: postgresql
      version: "12.0.0"
      serviceName: postgresql
      servicePort: 5432
      primary: false
```

**Files Created/Modified:**

| File | Purpose |
|------|---------|
| `api/v1alpha1/environmenttemplate_types.go` | CRD type definitions with kubebuilder markers |
| `internal/ui/templates.go` | API handlers that read from CRD (not mock data) |
| `config/crd/bases/ephemeral.ephemeralenv.io_environmenttemplates.yaml` | Generated CRD manifest |
| `config/samples/ephemeral_v1alpha1_environmenttemplate.yaml` | Sample templates |
| `internal/controller/environmenttemplate_controller_test.go` | CRD unit tests |

#### Phase 8.3: Multi-Chart Deployment Support

**Overview:**
Updated EphemeralEnvSpec to support deploying multiple Helm charts (components) in a single environment. This enables deploying full-stack applications with frontend, backend, and database.

**Updated EphemeralEnvSpec:**

```go
type EphemeralEnvSpec struct {
    // Legacy single-chart deployment (deprecated, kept for backward compatibility)
    Helm *HelmSpec `json:"helm,omitempty"`
    
    // NEW: Multi-chart deployment
    Components []DeployedComponentSpec `json:"components,omitempty"`
    
    // NEW: Reference to an EnvironmentTemplate
    TemplateRef *TemplateReference `json:"templateRef,omitempty"`
    
    TTL       string      `json:"ttl,omitempty"`
    Isolation *bool       `json:"isolation,omitempty"`
    Gateway   GatewaySpec `json:"gateway"`
}

// DeployedComponentSpec defines a component to be deployed
type DeployedComponentSpec struct {
    Name        string `json:"name"`
    Repository  string `json:"repository"`
    Chart       string `json:"chart"`
    Version     string `json:"version"`
    ServiceName string `json:"serviceName"`
    ServicePort int32  `json:"servicePort"`
    Values      *apiextensionsv1.JSON `json:"values,omitempty"`
    Primary     bool   `json:"primary,omitempty"`
}

// TemplateReference references an EnvironmentTemplate
type TemplateReference struct {
    Name      string `json:"name"`
    Namespace string `json:"namespace,omitempty"`
}
```

**Component Resolution Priority:**
1. `Components[]` - If specified, use these directly
2. `TemplateRef` - If specified, fetch template and use its components
3. `Helm` (legacy) - Fall back to single-chart deployment

**ComponentStatus Tracking:**
```go
type ComponentStatus struct {
    Name        string       `json:"name"`
    ReleaseName string       `json:"releaseName,omitempty"`
    Status      string       `json:"status,omitempty"`
    LastDeployed *metav1.Time `json:"lastDeployed,omitempty"`
    Message     string       `json:"message,omitempty"`
}
```

#### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/templates` | List all EnvironmentTemplate CRDs |
| GET | `/api/templates/{id}` | Get a specific template by name |
| GET | `/api/templates?namespace=X` | Filter templates by namespace |

#### Example: Multi-Chart Environment

```yaml
apiVersion: ephemeral.ephemeralenv.io/v1alpha1
kind: EphemeralEnv
metadata:
  name: fullstack-demo
spec:
  ttl: "4h"
  components:
    - name: frontend
      repository: "https://charts.bitnami.com/bitnami"
      chart: "nginx"
      version: "15.0.0"
      serviceName: "frontend-nginx"
      servicePort: 80
      primary: true
    - name: database
      repository: "https://charts.bitnami.com/bitnami"
      chart: "postgresql"
      version: "12.0.0"
      serviceName: "postgresql"
      servicePort: 5432
  gateway:
    name: "main-gateway"
    namespace: "default"
    domainPrefix: "fullstack-demo"
    serviceName: "frontend-nginx"
    targetPort: 80
```

#### Example: Using Template Reference

```yaml
apiVersion: ephemeral.ephemeralenv.io/v1alpha1
kind: EphemeralEnv
metadata:
  name: from-template
spec:
  templateRef:
    name: fullstack-webapp
    namespace: ephemeral-system
  ttl: "6h"  # Override template TTL
  gateway:
    name: "main-gateway"
    namespace: "default"
    domainPrefix: "from-template"
    serviceName: "frontend-nginx"
    targetPort: 80
```

#### Tasks

- [x] Create `EnvironmentTemplate` CRD with kubebuilder markers
- [x] Generate CRD manifests (`make manifests`)
- [x] Create sample templates YAML
- [x] Update UI templates.go to read from CRD
- [x] Write CRD unit tests (TDD)
- [x] Add `Components[]` to EphemeralEnvSpec
- [x] Add `TemplateRef` to EphemeralEnvSpec
- [x] Add `ComponentStatus[]` to EphemeralEnvStatus
- [x] Update controller to deploy multiple charts
- [x] Update uninstall to clean up all components
- [x] Add RBAC for EnvironmentTemplate resources
- [ ] Update UI to use templates dropdown
- [ ] Add template selection to create environment form

| Template ID | Display Name | Components | Default TTL |
|------------|--------------|------------|-------------|
| `fullstack` | Full Stack | Nginx + PostgreSQL | 2h |
| `frontend` | Frontend Only | Nginx | 1h |
| `podinfo` | Podinfo Demo | Podinfo | 30m |
| `redis` | Redis Cache | Redis | 1h |

#### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/templates` | List all available templates |
| GET | `/api/templates/{id}` | Get a specific template by ID |

#### Example Response

```json
GET /api/templates

[
  {
    "id": "fullstack",
    "displayName": "Full Stack",
    "description": "Complete web application stack with Nginx frontend and PostgreSQL database.",
    "icon": "🚀",
    "defaultTTL": "2h",
    "tags": ["web", "database", "production-like"],
    "components": [
      {
        "name": "Web Server",
        "repository": "https://charts.bitnami.com/bitnami",
        "chart": "nginx",
        "version": "18.2.5",
        "serviceName": "nginx",
        "servicePort": 80,
        "primary": true
      },
      {
        "name": "Database",
        "repository": "https://charts.bitnami.com/bitnami",
        "chart": "postgresql",
        "version": "16.2.5",
        "serviceName": "postgresql",
        "servicePort": 5432
      }
    ]
  }
]
```

#### Tasks

- [x] Define `EnvironmentTemplate` struct
- [x] Define `ComponentSpec` struct
- [x] Create `TemplateRegistry` with mock data
- [x] Implement `GET /api/templates` handler
- [x] Implement `GET /api/templates/{id}` handler
- [x] Register routes in server setup
- [ ] Update UI to use templates dropdown
- [ ] Add template selection to create environment form

---

## Appendix A: Project Structure

```
ephemeral-operator/
├── api/
│   └── v1alpha1/
│       ├── ephemeralenv_types.go      # CRD type definitions
│       ├── groupversion_info.go       # API group registration
│       └── zz_generated.deepcopy.go   # Generated deep copy
├── cmd/
│   └── main.go                        # Entry point
├── config/
│   ├── crd/
│   │   └── bases/                     # Generated CRD YAML
│   ├── manager/                       # Controller manager config
│   ├── rbac/                          # RBAC rules
│   └── samples/                       # Example CRs
├── internal/
│   ├── controller/
│   │   ├── ephemeralenv_controller.go      # Main reconciler
│   │   ├── ephemeralenv_controller_test.go # Ginkgo tests
│   │   ├── namespace.go                    # Namespace logic
│   │   ├── networkpolicy.go                # NetworkPolicy logic
│   │   ├── helm.go                         # Helm SDK integration
│   │   └── gateway.go                      # Gateway API logic
│   └── helm/
│       └── client.go                  # Helm client wrapper
├── test/
│   ├── e2e/                           # End-to-end tests (Kind cluster)
│   │   ├── e2e_suite_test.go          # Suite setup (build image, load Kind)
│   │   ├── e2e_test.go                # Manager + Helm + corner cases
│   │   └── fixtures/                  # EphemeralEnv YAML for e2e
│   └── utils/                         # Kind, CertManager, Run helpers
├── Dockerfile
├── Makefile
├── go.mod
├── go.sum
└── ARCHITECTURE.md                    # This file
```

---

## Appendix B: Useful Commands

```bash
# Generate CRD manifests
make generate
make manifests

# Run tests
make test

# Run with EnvTest (local control plane)
make test KUBEBUILDER_ASSETS="$(setup-envtest use -p path)"

# Install CRDs to cluster
make install

# Run controller locally
make run

# Build and push Docker image
make docker-build docker-push IMG=<registry>/ephemeral-operator:tag

# Deploy to cluster
make deploy IMG=<registry>/ephemeral-operator:tag
```

---

## Appendix C: References

- [Kubebuilder Book](https://book.kubebuilder.io/)
- [Controller Runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime)
- [Gateway API](https://gateway-api.sigs.k8s.io/)
- [Helm SDK Documentation](https://pkg.go.dev/helm.sh/helm/v3)
- [Ginkgo Testing Framework](https://onsi.github.io/ginkgo/)
- [Gomega Matchers](https://onsi.github.io/gomega/)

---

## 6. Development Environment

### Windows with WSL

**This project is developed on Windows with WSL (Windows Subsystem for Linux).**

All Go commands, tests, and the operator MUST be run inside WSL:

```powershell
# From PowerShell - run commands in WSL
wsl bash -c "cd /mnt/c/Users/<user>/github/kubeEphemerals && <command>"

# Or use interactive shell
wsl bash -ic "cd /mnt/c/Users/<user>/github/kubeEphemerals && go test ./..."
```

**Important:** The Kubernetes cluster (minikube) runs inside WSL. Use `wsl bash -c` for all kubectl, helm, and go commands.

### Compiling

```bash
# Compile all packages (from WSL)
wsl bash -ic "cd /mnt/c/Users/<user>/github/kubeEphemerals && go build ./..."

# Generate CRDs and DeepCopy (after editing *_types.go)
wsl bash -ic "cd /mnt/c/Users/<user>/github/kubeEphemerals && make manifests generate"

# Install CRDs to cluster
wsl bash -ic "cd /mnt/c/Users/<user>/github/kubeEphemerals && make install"
```

### Running the Operator (Background Process)

**IMPORTANT:** Always kill the old operator before starting a new one!

```bash
# 1. Kill existing operator
wsl bash -ic "pkill -9 main 2>/dev/null; pkill -9 'go run' 2>/dev/null"

# 2. Start operator in background
wsl bash -ic "cd /mnt/c/Users/<user>/github/kubeEphemerals && nohup go run ./cmd/main.go > /tmp/operator.log 2>&1 &"

# 3. Check logs
wsl bash -ic "tail -f /tmp/operator.log"
```

The operator runs:
- **Controller Manager** on port 8081 (health probes)
- **UI Server** on port **8082** (dashboard)

### Accessing the UI

After starting the operator, open: **http://localhost:8082**

---

## 7. Current Project Status

### Completed Phases ✅

| Phase | Description | Status |
|-------|-------------|--------|
| 1 | Project Scaffolding & CRD Types | ✅ Complete |
| 2 | Namespace Lifecycle | ✅ Complete |
| 3 | NetworkPolicy & Security | ✅ Complete |
| 4 | TTL & Automatic Cleanup | ✅ Complete |
| 5 | Helm SDK Integration | ✅ Complete |
| 6 | Gateway API & HTTPRoute | ✅ Complete |
| 7 | Web UI Dashboard | ✅ Complete |
| 8 | Service Catalog (Templates) | ✅ Complete |
| 12 | Authentication & Multi-tenancy | ✅ Complete |

### In Progress 🚧

| Phase | Description | Status |
|-------|-------------|--------|
| 9 | CI/CD Integration | 🚧 Planned |
| 10 | Production Hardening | 🚧 Planned |

### Test Coverage

Tests are **ESSENTIAL and MANDATORY**. We follow strict TDD (Test-Driven Development).

#### Running Tests

```bash
# Run ALL tests (from WSL)
wsl bash -c "cd /mnt/c/Users/<user>/github/kubeEphemerals && make test"

# Run controller tests only
wsl bash -ic "cd /mnt/c/Users/<user>/github/kubeEphemerals && go test -v ./internal/controller/..."

# Run UI tests only
wsl bash -ic "cd /mnt/c/Users/<user>/github/kubeEphemerals && go test -v ./internal/ui/..."

# Run with coverage report
wsl bash -ic "cd /mnt/c/Users/<user>/github/kubeEphemerals && go test -coverprofile=cover.out ./... && go tool cover -html=cover.out"
```

#### Test Files Structure

| Test File | Description | Test Count |
|-----------|-------------|------------|
| `internal/controller/ephemeralenv_controller_test.go` | Controller reconciliation tests with envtest | 50+ tests |
| `test/e2e/e2e_test.go` | E2E on Kind: Manager, Helm, EphemeralEnv lifecycle and corner cases | Manager + Helm + corner |
| `internal/controller/environmenttemplate_controller_test.go` | Template controller tests | ~10 tests |
| `internal/ui/server_test.go` | UI API handlers tests | 47+ tests |
| `api/v1alpha1/ephemeralenv_types_test.go` | CRD validation and defaults | ~10 tests |

#### Coverage Requirements

- **Target coverage:** >80% for business logic
- **Controller logic:** Must test all reconciliation paths
- **API handlers:** Must test all HTTP endpoints
- **CRD types:** Must test validation and defaults

### Current Project Structure

```
ephemeral-operator/
├── api/v1alpha1/
│   ├── ephemeralenv_types.go          # EphemeralEnv CRD schema
│   ├── environmenttemplate_types.go   # EnvironmentTemplate CRD schema
│   ├── ephemeralenv_helpers.go        # Helper methods for CRD
│   ├── ephemeralenv_types_test.go     # Type validation tests
│   ├── groupversion_info.go           # API group registration
│   ├── suite_test.go                  # Test suite setup
│   └── zz_generated.deepcopy.go       # Generated (DO NOT EDIT)
│
├── cmd/
│   └── main.go                        # Entry point (manager + UI server)
│
├── config/
│   ├── crd/bases/                     # Generated CRDs (DO NOT EDIT)
│   ├── rbac/role.yaml                 # Generated RBAC (DO NOT EDIT)
│   ├── samples/
│   │   ├── ephemeral_v1alpha1_ephemeralenv.yaml
│   │   ├── ephemeral_v1alpha1_environmenttemplate.yaml
│   │   └── gateway_class_setup.yaml
│   └── ...
│
├── internal/
│   ├── controller/
│   │   ├── ephemeralenv_controller.go           # Main reconciler (800+ lines)
│   │   ├── ephemeralenv_controller_test.go      # Controller tests (50+ tests)
│   │   ├── environmenttemplate_controller_test.go
│   │   └── suite_test.go                        # EnvTest setup
│   │
│   ├── helm/
│   │   └── client.go                  # Helm SDK client wrapper
│   │
│   └── ui/
│       ├── server.go                  # HTTP server & routes
│       ├── server_test.go             # UI API tests (47+ tests)
│       ├── templates.go               # Templates API handlers (CRUD)
│       ├── static/                    # Static assets (CSS, JS)
│       └── templates/
│           ├── base.html              # Base layout
│           ├── global_dashboard.html  # Main dashboard (Environments + Templates tabs)
│           └── env_dashboard.html     # Environment details page (Overview, Pods, Logs, Config)
│
├── Makefile                           # Build commands
├── ARCHITECTURE.md                    # This file
├── AGENTS.md                          # AI Agent guide
└── PROJECT                            # Kubebuilder metadata (DO NOT EDIT)
```

### Key Features Implemented

1. **EphemeralEnv CRD** - Custom resource for ephemeral environments
2. **EnvironmentTemplate CRD** - Service catalog for reusable templates
3. **Namespace Lifecycle** - Auto-creation with owner references
4. **NetworkPolicy Isolation** - Deny-all policy for security
5. **TTL & Cleanup** - Automatic expiration and resource cleanup
6. **Helm SDK Integration** - Programmatic chart deployment (NO os/exec)
7. **Gateway API** - HTTPRoute + ReferenceGrant for external access
8. **Web UI Dashboard** - Full CRUD for environments and templates
9. **Multi-chart Support** - Deploy multiple Helm charts per environment
10. **Template References** - Create environments from predefined templates
11. **Kubeconfig self-service** - Download kubeconfig per environment (token + RBAC)

---

## Appendix D: Kubeconfig API Server URL

When the operator generates a kubeconfig for an environment, it must embed an **API server URL** that is reachable from where the user runs `kubectl`. That URL is not always the same as the one the operator uses internally (e.g. in-cluster `https://kubernetes.default.svc` or minikube’s internal IP). This section summarizes how other platforms solve this and how we align.

### How others do it

| Platform | Approach | Source of “external” URL |
|----------|----------|---------------------------|
| **Rancher** | Kubeconfig includes cluster server URL. For ACE-enabled clusters: if **FQDN is set** on the cluster, that FQDN is used as the single entry; otherwise entries for control plane nodes. | Cluster-level config (FQDN / control plane addresses). [Kubeconfigs API](https://ranchermanager.docs.rancher.com/api/workflows/kubeconfigs), [ACE](https://ranchermanager.docs.rancher.com/how-to-guides/new-user-guides/manage-clusters/access-clusters/authorized-cluster-endpoint). |
| **EKS** | Cluster has an explicit **endpoint** (public/private). Kubeconfig is generated with that endpoint. | Platform-managed endpoint (e.g. `cluster.region.eks.amazonaws.com`). [Cluster endpoint](https://docs.aws.amazon.com/eks/latest/userguide/cluster-endpoint.html). |
| **RKE2** | Default kubeconfig uses `127.0.0.1`; docs say to replace with the RKE2 server IP/hostname when using from outside. | Explicit replacement or configured address. [Cluster access](https://docs.rke2.io/cluster_access). |
| **kubeadm** | `kube-public/cluster-info` ConfigMap holds bootstrap kubeconfig with **controlPlaneEndpoint** (or advertise address). Used for join and discovery. | Set at init via `controlPlaneEndpoint`; cluster-info is the standard place. |

Common idea: the “kubeconfig server URL” is a **cluster-level, configured** value (FQDN, endpoint, or control plane address), not “whatever the operator sees.” For minikube/kind, that often differs from the in-cluster or node IP (e.g. `127.0.0.1:port` from the host).

### Our approach

1. **Prefer `kube-public/cluster-info`**  
   Standard bootstrap ConfigMap (kubeadm and many installers). If the cluster was set up with a proper control plane endpoint, cluster-info usually has a usable URL.

2. **Fallback to `rest.Config`**  
   If cluster-info is missing or unreadable, use the operator’s `rest.Config` (Host + CA). Works when the operator runs locally with a kubeconfig that already has the right server.

3. **Override via env: `KUBECONFIG_SERVER_URL`**  
   When cluster-info (or rest.Config) gives an internal/unreachable URL (e.g. minikube internal IP), the deployer sets the reachable URL (e.g. `https://127.0.0.1:32771` for minikube). Same idea as RKE2 “replace 127.0.0.1 with server IP.”

4. **Override via ConfigMap (operator namespace)**  
   Optional: read `kubeconfig-server-url` from a ConfigMap in the operator’s namespace (e.g. `ephemeral-operator-system`). This gives a **per-cluster** configuration (like Rancher’s FQDN or EKS endpoint) without relying on env vars in the deployment. Preferred in production: set once per cluster, works for minikube, EKS, RKE, and any installer.

Priority order: **ConfigMap key** → **env `KUBECONFIG_SERVER_URL`** → cluster-info → rest.Config. CA always comes from cluster-info or rest.Config; only the server URL is overridden.

This keeps the feature **cluster-agnostic**: the same code path works for minikube, EKS, Rancher-managed clusters, and kubeadm; the operator or platform just sets the appropriate URL (env or ConfigMap).

---

*This document is the Source of Truth. All implementation decisions should reference this architecture.*
