# Securing preview URLs (OAuth2 / Zero-Trust)

The Ephemeral Operator does **not** build in authentication for the preview URLs it exposes. The HTTPRoute only routes traffic to your app’s Service. To protect preview environments (e.g. only allow logged-in or authorized users), you can put an **OAuth2 Proxy** (or similar) in front of the apps.

## Pattern: OAuth2 Proxy in front of previews

A common approach is **zero-trust**: every request to `https://pr-123.preview.example.com` must be authenticated (e.g. OAuth2/OIDC). Options:

1. **OAuth2 Proxy per environment** – Run OAuth2 Proxy as a sidecar or separate Deployment in each ephemeral namespace. Ingress/HTTPRoute points to the proxy; the proxy validates the session and forwards to your app. You can template this in your Helm chart or use a wrapper chart that adds the proxy.
2. **Central OAuth2 Proxy** – One proxy (e.g. in a shared namespace) that handles auth for many hostnames; it forwards to the correct backend Service based on host/path. You’d configure the Gateway/HTTPRoute to send traffic to this proxy, and the proxy routes to `env-<name>/<service>`.
3. **Gateway-level auth** – Some Gateway implementations (e.g. Istio, Envoy with ext_authz) can call an OAuth2 or OIDC service before forwarding. Then you don’t need a separate proxy pod; the Gateway enforces auth for all preview hostnames.

## What the operator does and doesn’t do

- **Does:** Create HTTPRoute so that `https://<domainPrefix>.<base-domain>` reaches the **Service** you specified (or discovered) in the ephemeral namespace.
- **Does not:** Configure OAuth2, sessions, or auth headers. It does not deploy OAuth2 Proxy for you.

So “Zero-Trust Auth via OAuth2 Proxy” in the landing page means you **can** achieve zero-trust by adding OAuth2 Proxy (or Gateway-level auth) yourself; the operator just provides the routing layer.

## Example idea: OAuth2 Proxy in the same namespace

If your Helm chart deploys both the app and OAuth2 Proxy in the same namespace:

- Create a Service that points to the OAuth2 Proxy pod.
- In EphemeralEnv, set `gateway.serviceName` to that proxy Service and `gateway.targetPort` to the proxy’s port (e.g. 4180).
- The proxy is configured to authenticate (e.g. OIDC) and forward to your app’s Service inside the namespace.

That way every request to the preview URL hits the proxy first; only after auth does traffic reach your application.

## Summary

| Topic | Operator role | Your role |
|-------|----------------|-----------|
| Routing | Creates HTTPRoute to your Service | Deploy Gateway + (optional) auth |
| Auth | None | Add OAuth2 Proxy or Gateway auth |
| TLS | Handled by Gateway | Configure certs on Gateway/listener |

For full zero-trust, combine the operator’s HTTPRoute with your choice of OAuth2 Proxy or Gateway-level authentication.
