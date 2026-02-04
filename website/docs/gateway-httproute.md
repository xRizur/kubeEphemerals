# Gateway API & HTTPRoute

The operator exposes each ephemeral environment via the **Gateway API**: it creates an **HTTPRoute** that binds to your Gateway and routes traffic to the environment’s Service. Optionally it creates a **ReferenceGrant** when the Gateway and the Service are in different namespaces.

## Prerequisites

- **Gateway API CRDs** installed (e.g. from [gateway-api releases](https://github.com/kubernetes-sigs/gateway-api/releases)).
- A **Gateway** (e.g. Envoy Gateway, Istio) that listens on a hostname and serves TLS/HTTP.
- The operator needs RBAC to create HTTPRoute and ReferenceGrant in the appropriate namespaces.

## What the operator creates

1. **HTTPRoute** – In the **Gateway’s namespace** (from `spec.gateway.namespace`). It references:
   - **ParentRef:** your Gateway (`spec.gateway.name` + `namespace`).
   - **Hostnames:** derived from `spec.gateway.domainPrefix` and the Gateway’s configured domain (e.g. `pr-123.preview.example.com`).
   - **BackendRef:** the Service in the **environment namespace** (`spec.gateway.serviceName` + `spec.gateway.targetPort`).

2. **ReferenceGrant** (if needed) – When the HTTPRoute lives in the Gateway namespace and the Service is in the environment namespace, the operator creates a ReferenceGrant in the **environment namespace** so the Gateway implementation can reference that cross-namespace Service.

## EphemeralEnv gateway fields

| Field | Description |
|-------|-------------|
| `gateway.name` | Name of the Gateway resource. |
| `gateway.namespace` | Namespace where the Gateway (and HTTPRoute) live. |
| `gateway.domainPrefix` | Subdomain for this env. Full hostname is typically `<domainPrefix>.<base-domain>`. |
| `gateway.serviceName` | Name of the Kubernetes Service in the env namespace to route to. |
| `gateway.targetPort` | Port on that Service (default 80). |

If you use **service discovery** (`spec.servicePort`), the operator finds the Service in the env namespace that exposes that port and uses it for the route instead of `gateway.serviceName`.

## Base domain and URL

The actual hostname (and thus `status.accessURL`) depends on how your Gateway is configured. The operator typically builds the URL from a base domain (e.g. from UI config or a fixed pattern like `preview.example.com`). So for `domainPrefix: pr-123` you might get `https://pr-123.preview.example.com`.

## Example: Gateway + GatewayClass

Minimal setup (e.g. for Envoy Gateway):

```yaml
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

After that, creating an EphemeralEnv with `gateway.name: main-gateway`, `gateway.namespace: default`, and a `domainPrefix` will result in an HTTPRoute and (if needed) a ReferenceGrant. Your Gateway implementation then serves traffic to the environment’s Service.

## Cleanup

When the EphemeralEnv is deleted or expires, the operator deletes the HTTPRoute and the ReferenceGrant before deleting the namespace.
