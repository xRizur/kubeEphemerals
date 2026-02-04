# EphemeralEnv CRD

`EphemeralEnv` is the main custom resource. One resource = one ephemeral environment (namespace + Helm + optional NetworkPolicy + HTTPRoute). After the TTL, the operator deletes the namespace and cleans up.

## API

- **Group:** `ephemeral.ephemeralenv.io`
- **Version:** `v1alpha1`
- **Kind:** `EphemeralEnv`
- **Short name:** `eenv`

## Spec (all parameters)

### Top-level

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ttl` | string | No (default `24h`) | Time-to-live. Format: Go duration (`1h`, `30m`, `24h`). Pattern: `^([0-9]+h)?([0-9]+m)?([0-9]+s)?$`. |
| `isolation` | bool | No (default `true`) | When `true`, a deny-all NetworkPolicy is created in the environment namespace. See [Network Policy](/docs/network-policy). |
| `gateway` | [GatewaySpec](#gatewayspec) | **Yes** | Gateway API: Gateway name/namespace, domain prefix, service name, target port. |
| `helm` | [HelmSpec](#helmspec) | No* | Single Helm chart (legacy). Ignored if `components` or `templateRef` is set. |
| `components` | [DeployedComponentSpec](#deployedcomponentspec)[] | No* | Multiple Helm charts. At least one must have `primary: true`. |
| `templateRef` | [TemplateReference](#templatereference) | No* | Use an EnvironmentTemplate; its components are used. |
| `servicePort` | int32 (1–65535) | No | Port for **service discovery**. When set, the operator finds the Service in the env namespace that exposes this port and uses it for the HTTPRoute (instead of `gateway.serviceName`). |

\* One of `helm`, `components`, or `templateRef` must be provided.

### GatewaySpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Name of the Gateway resource (e.g. `main-gateway`). |
| `namespace` | string | Yes | Namespace of the Gateway. |
| `domainPrefix` | string | Yes | Subdomain for this environment. URL is `https://<domainPrefix>.<gateway-domain>`. Pattern: `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`. |
| `serviceName` | string | Yes | Name of the Kubernetes Service to route to (in the env namespace). Or use `servicePort` for discovery. |
| `targetPort` | int32 | No (default 80) | Port on the Service. 1–65535. |

### HelmSpec (single-chart)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `repository` | string | Yes | Chart repository URL. Pattern: `^https?://.*`. |
| `chart` | string | Yes | Chart name. |
| `version` | string | Yes | Chart version (semver). Pattern: `^[0-9]+\.[0-9]+\.[0-9]+.*$`. |
| `values` | object (JSON) | No | Helm values (any structure). Passed to `helm install/upgrade`. |

### DeployedComponentSpec (multi-chart)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Component name (e.g. `frontend`). |
| `repository` | string | Yes | Helm repository URL. |
| `chart` | string | Yes | Chart name. |
| `version` | string | Yes | Chart version. |
| `serviceName` | string | Yes | Service name created by this chart. |
| `servicePort` | int32 | Yes | Service port. 1–65535. |
| `primary` | bool | No (default false) | Exactly one component must be `true`; it is used for the HTTPRoute backend. |
| `values` | object (JSON) | No | Helm values for this chart. |

### TemplateReference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Name of the EnvironmentTemplate. |
| `namespace` | string | No | Namespace of the template. If empty, same as EphemeralEnv. |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | `Pending` \| `Active` \| `Failed` \| `Expired`. |
| `activeNamespace` | string | Created namespace (e.g. `env-pr-123`). |
| `accessURL` | string | Full URL to the app (from HTTPRoute). |
| `adminURL` | string | URL to admin dashboard for this env (if configured). |
| `expirationTime` | time | When the environment will be deleted. |
| `helmRelease` | string | Helm release name (single-chart). |
| `componentStatuses` | [] | Per-component status (multi-chart). |
| `conditions` | [] | e.g. NamespaceReady, NetworkPolicyApplied, HelmDeployed, HTTPRouteReady. |
| `message` | string | Human-readable status. |

## YAML examples

### Single Helm chart

```yaml
apiVersion: ephemeral.ephemeralenv.io/v1alpha1
kind: EphemeralEnv
metadata:
  name: pr-123-demo
  labels:
    pr-number: "123"
spec:
  ttl: "2h"
  isolation: true
  helm:
    repository: "https://stefanprodan.github.io/podinfo"
    chart: "podinfo"
    version: "6.9.4"
    values:
      replicaCount: 2
      extraEnvs:
        - name: PODINFO_UI_MESSAGE
          value: "PR #123"
  gateway:
    name: "main-gateway"
    namespace: "default"
    domainPrefix: "pr-123"
    serviceName: "env-pr-123-demo-podinfo"
    targetPort: 9898
```

### Multi-chart (components)

```yaml
apiVersion: ephemeral.ephemeralenv.io/v1alpha1
kind: EphemeralEnv
metadata:
  name: fullstack-demo
spec:
  ttl: "4h"
  isolation: true
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
      primary: false
  gateway:
    name: "main-gateway"
    namespace: "default"
    domainPrefix: "fullstack-demo"
    serviceName: "frontend-nginx"
    targetPort: 80
```

### From template (templateRef)

```yaml
apiVersion: ephemeral.ephemeralenv.io/v1alpha1
kind: EphemeralEnv
metadata:
  name: from-template-demo
spec:
  templateRef:
    name: fullstack-webapp
    namespace: ephemeral-system
  ttl: "6h"
  gateway:
    name: "main-gateway"
    namespace: "default"
    domainPrefix: "template-demo"
    serviceName: "frontend-nginx"
    targetPort: 80
```

### Port-based service discovery (servicePort)

When you set `servicePort`, the operator discovers the Service in the environment namespace that exposes that port (ignoring headless/external-name/metrics) and uses it for the HTTPRoute:

```yaml
spec:
  ttl: "1h"
  servicePort: 9898
  templateRef:
    name: podinfo
    namespace: ephemeral-system
  gateway:
    name: "main-gateway"
    namespace: "default"
    domainPrefix: "pr-456"
    serviceName: "discovered"
    targetPort: 9898
```
