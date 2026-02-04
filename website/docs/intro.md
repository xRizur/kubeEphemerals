# Introduction

**Ephemeral Operator** is a Kubernetes operator that manages short-lived (ephemeral) environments for Pull Requests and demos. It creates isolated namespaces, deploys Helm charts, applies NetworkPolicy for security, and exposes environments via the Gateway API.

## What it does

- **EphemeralEnv** – A custom resource that represents one ephemeral environment. You create an `EphemeralEnv` (via YAML or the built-in UI), and the operator:
  1. Creates a namespace (`env-<name>`)
  2. Optionally applies a **deny-all NetworkPolicy** (when `isolation: true`)
  3. Deploys one or more **Helm charts** (single chart or multiple components)
  4. Creates a **Gateway API HTTPRoute** so the app is reachable at a unique URL
  5. After the **TTL** expires, deletes the namespace and all resources

- **EnvironmentTemplate** – Reusable templates (service catalog) that define Helm components, default TTL, and metadata. You can create an `EphemeralEnv` by referencing a template (`templateRef`) or by specifying Helm/Gateway manually.

## Key concepts

| Concept | Description |
|--------|-------------|
| **TTL** | Time-to-live (e.g. `1h`, `24h`). After this duration the environment is automatically deleted. |
| **Isolation** | When `true`, a deny-all NetworkPolicy is created in the namespace so pods cannot talk to other namespaces. |
| **Gateway** | Gateway API Gateway + HTTPRoute. The operator creates an HTTPRoute so `https://<domainPrefix>.<base-domain>` routes to your app's Service. |
| **Components** | Multiple Helm charts in one environment (e.g. frontend + database). Exactly one component is **primary** (used for the HTTPRoute backend). |
| **TemplateRef** | Reference to an `EnvironmentTemplate`; the operator uses the template's components and optional default TTL. |

## Documentation map

- [Installation](/docs/installation) – Helm install and chart options
- [EphemeralEnv CRD](/docs/crd-ephemeralenv) – Full spec, all parameters, YAML examples
- [EnvironmentTemplate CRD](/docs/crd-environmenttemplate) – Template spec and examples
- [Network Policy](/docs/network-policy) – Isolation and deny-all policy
- [Gateway & HTTPRoute](/docs/gateway-httproute) – Routing and cross-namespace access
- [Helm chart values](/docs/helm-values) – Operator Helm `values.yaml` reference
- [Kubeconfig self-service](/docs/kubeconfig) – Per-environment kubeconfig download (API + UI)
- [Securing previews (OAuth2)](/docs/securing-previews) – Optional OAuth2 Proxy for preview URLs

The dashboard shows **logs** per environment.

## Quick start

1. Install the operator: `helm install my-op ./charts/ephemeral-operator -n ephemeral-operator-system --create-namespace`
2. Install Gateway API and a Gateway (e.g. Envoy Gateway). See [Gateway & HTTPRoute](/docs/gateway-httproute).
3. Create an `EnvironmentTemplate` (optional) or an `EphemeralEnv` with Helm + Gateway spec.
4. Open the built-in UI at `http://<operator>:8082` to create environments and templates from the dashboard.
