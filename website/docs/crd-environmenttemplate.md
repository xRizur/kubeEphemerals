# EnvironmentTemplate CRD

`EnvironmentTemplate` defines a reusable “service catalog” entry: display name, description, default TTL, and a list of Helm components. Users create EphemeralEnvs by referencing a template (`templateRef`) so they don’t have to repeat repository/chart/version/service details.

## API

- **Group:** `ephemeral.ephemeralenv.io`
- **Version:** `v1alpha1`
- **Kind:** `EnvironmentTemplate`
- **Short name:** `etpl`

## Spec (all parameters)

### Top-level

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `displayName` | string | Yes | Human-readable name (e.g. for UI). Max 100 chars. |
| `description` | string | No | What this template deploys. Max 500 chars. |
| `icon` | string | No | Emoji or icon id for UI. Max 10 chars. |
| `tags` | []string | No | Labels for filtering (e.g. `web`, `database`). |
| `components` | [ComponentSpec](#componentspec)[] | **Yes** (min 1) | Helm charts to deploy. Exactly one must have `primary: true`. |
| `defaultTTL` | string | No (default `1h`) | Default TTL for envs created from this template. Pattern: Go duration. |
| `defaultGateway` | [GatewayDefaults](#gatewaydefaults) | No | Default Gateway name/namespace for templates. |

### ComponentSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Component name (e.g. `frontend`). |
| `repository` | string | Yes | Helm repository URL. |
| `chart` | string | Yes | Chart name. |
| `version` | string | Yes | Chart version. |
| `serviceName` | string | Yes | Service name created by this chart. |
| `servicePort` | int32 | Yes | Service port. 1–65535. |
| `primary` | bool | No (default false) | Exactly one component must be `true` (used for routing). |
| `values` | object (JSON) | No | Default Helm values for this component. |

### GatewayDefaults

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Default Gateway name. |
| `namespace` | string | Default Gateway namespace. |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `usageCount` | int32 | How many envs were created from this template. |
| `lastUsedTime` | time | Last time an env was created from this template. |
| `conditions` | [] | Status conditions. |

## YAML example

```yaml
apiVersion: ephemeral.ephemeralenv.io/v1alpha1
kind: EnvironmentTemplate
metadata:
  name: fullstack-webapp
  namespace: ephemeral-system
  labels:
    tier: fullstack
spec:
  displayName: Full Stack Web Application
  description: |
    Frontend (Nginx), backend placeholder, and PostgreSQL.
  icon: "🚀"
  tags:
    - fullstack
    - web
    - database
  defaultTTL: "4h"
  defaultGateway:
    name: main-gateway
    namespace: default
  components:
    - name: frontend
      repository: https://charts.bitnami.com/bitnami
      chart: nginx
      version: "15.0.0"
      serviceName: frontend-nginx
      servicePort: 80
      primary: true
      values:
        service:
          type: ClusterIP
        resources:
          limits:
            memory: "128Mi"
            cpu: "100m"
    - name: database
      repository: https://charts.bitnami.com/bitnami
      chart: postgresql
      version: "12.0.0"
      serviceName: postgresql
      servicePort: 5432
      primary: false
      values:
        auth:
          username: appuser
          database: appdb
        primary:
          persistence:
            enabled: false
```

Creating an EphemeralEnv from this template only requires `templateRef` plus `gateway` (and optional `ttl` override). See [EphemeralEnv – From template](/docs/crd-ephemeralenv#from-template-templateref).
