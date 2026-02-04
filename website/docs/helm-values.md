# Helm Chart Values (values.yaml)

The operator is installed via the Helm chart in `charts/ephemeral-operator/`. Below are the configurable values and how they map to the operator behavior.

## Top-level keys

### image

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `image.repository` | string | `ghcr.io/maciekmm/kubeephemerals/ephemeral-operator` | Image repository. |
| `image.tag` | string | `latest` | Image tag. |
| `image.pullPolicy` | string | `IfNotPresent` | Image pull policy. |

### replicaCount

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `replicaCount` | int | `1` | Number of controller manager replicas. |

### resources

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `resources.limits.cpu` | string | `500m` | CPU limit. |
| `resources.limits.memory` | string | `128Mi` | Memory limit. |
| `resources.requests.cpu` | string | `10m` | CPU request. |
| `resources.requests.memory` | string | `64Mi` | Memory request. |

### config (operator/UI configuration)

These values are passed as environment variables or CLI flags to the operator.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `config.defaultTTL` | string | `24h` | Default TTL hint (e.g. for UI). |
| `config.externalApiUrl` | string | `""` | Overrides API server URL in generated kubeconfigs. Maps to **KUBECONFIG_SERVER_URL**. |
| `config.adminGroup` | string | `""` | Admin group for RBAC/dashboard. Maps to **ADMIN_GROUP**. |
| `config.leaderElect` | bool | `true` | Enable leader election for HA. |
| `config.metricsBindAddress` | string | `:8443` | Metrics endpoint (`:8443` HTTPS, `:8080` HTTP, `0` to disable). |
| `config.uiBindAddress` | string | `:8082` | UI server bind address. |
| `config.uiPlatformDomain` | string | `platform.local` | Platform domain for global dashboard. |
| `config.uiBaseDomain` | string | `preview.example.com` | Base domain for environment URLs (e.g. `pr-123.preview.example.com`). |
| `config.operatorService` | string | `""` | Operator Service name for admin HTTPRoute. Empty = disable. |
| `config.operatorNamespace` | string | `""` | Operator namespace (admin HTTPRoute, kubeconfig ConfigMap). Defaults to release namespace. |
| `config.kubeconfigConfigMapName` | string | `ephemeral-operator-ui-config` | ConfigMap name (in operator namespace) for kubeconfig server URL override. Key: `kubeconfig-server-url`. |

### namespaceOverride

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `namespaceOverride` | string | `""` | Override namespace for the operator deployment. |

### serviceAccount

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `serviceAccount.create` | bool | `true` | Create a dedicated ServiceAccount. |
| `serviceAccount.name` | string | `""` | ServiceAccount name. Empty = default from chart. |

### installCRDs

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `installCRDs` | bool | `true` | If `true`, the chart installs CRDs from `crds/`. Set to `false` if you manage CRDs separately. |

## Example overrides

```yaml
# values override
config:
  defaultTTL: "12h"
  uiBaseDomain: "preview.mycompany.com"
  externalApiUrl: "https://k8s.mycompany.com"
  metricsBindAddress: ":8443"

image:
  repository: myreg.io/myorg/ephemeral-operator
  tag: "1.0.0"
  pullPolicy: IfNotPresent

replicaCount: 1
installCRDs: true
```

## Kubeconfig server URL

Generated kubeconfigs need an API server URL reachable by the user. Priority:

1. **ConfigMap** – In operator namespace, key `kubeconfig-server-url` in the ConfigMap named `config.kubeconfigConfigMapName`.
2. **Environment** – `config.externalApiUrl` (KUBECONFIG_SERVER_URL).
3. **Cluster** – `kube-public/cluster-info` or the operator’s rest config.

See [ARCHITECTURE Appendix D](https://github.com/maciekmm/kubeEphemerals/blob/main/ARCHITECTURE.md#appendix-d-kubeconfig-api-server-url) for details.
