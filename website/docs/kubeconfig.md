# Self-service kubeconfig

The operator can generate a **restricted kubeconfig** for each ephemeral environment. Developers use it to run `kubectl` against only that environment’s namespace, with a short-lived token and RBAC managed by the operator.

## How it works

1. For each environment, the controller creates a **ServiceAccount** and **Role** in the environment namespace (e.g. `env-pr-123`). The Role grants list/get/watch on typical resources (pods, services, etc.) in that namespace.
2. When a user requests a kubeconfig (via the UI or API), the UI server creates a **short-lived token** for that ServiceAccount (TokenRequest API).
3. The server builds a kubeconfig YAML with:
   - **Cluster:** API server URL and CA (from cluster-info or overrides).
   - **User:** token for the env’s ServiceAccount.
   - **Context:** namespace set to the environment namespace (e.g. `env-pr-123`).
4. The response is sent with `Content-Disposition: attachment; filename="kubeconfig-<env-name>.yaml"` so the browser offers a download.

## API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/envs/{name}/kubeconfig` | Returns a YAML kubeconfig file for the environment `name`. |

- **Requires:** The EphemeralEnv `name` must exist and have `status.activeNamespace` set (environment must be provisioned).
- **Response:** `200` with body = kubeconfig YAML; `Content-Type: application/x-yaml` (or similar); `Content-Disposition` with filename `kubeconfig-{name}.yaml`.

## API server URL

The kubeconfig must contain an **API server URL** that is reachable from where the user runs `kubectl` (e.g. your laptop). That URL is not always the same as the one the operator uses internally (e.g. in-cluster or minikube internal IP). The operator resolves it in this order:

1. **ConfigMap** – In the operator namespace, a ConfigMap (default name: `ephemeral-operator-ui-config`) with key `kubeconfig-server-url`. Used for per-cluster override (e.g. public endpoint).
2. **Environment** – `KUBECONFIG_SERVER_URL` (or Helm `config.externalApiUrl`). Same idea: set the reachable API server URL.
3. **Cluster** – `kube-public/cluster-info` ConfigMap (bootstrap kubeconfig) or the operator’s rest config.

See [Helm values](/docs/helm-values#config-operator-ui-configuration) and [ARCHITECTURE Appendix D](https://github.com/maciekmm/kubeEphemerals/blob/main/ARCHITECTURE.md#appendix-d-kubeconfig-api-server-url) for details.

## UI

In the **environment detail** view of the built-in dashboard, a **“Download kubeconfig”** button calls the same API and triggers a file download. The demo on the landing page mocks this; the real operator UI serves the actual kubeconfig.

## RBAC and token TTL

- The token is created with an expiration (bounded by the environment TTL or a default, e.g. 12h).
- The Role in the environment namespace is tailored for developer access (e.g. pods, services, logs). Admin or write actions are not granted by this kubeconfig.

## Not included: admin UI / OAuth proxy

**Admin API/UI behind OAuth2 proxy** (admin vs user roles, sub-helm chart for the dashboard) is developed on a **separate branch**. The ephemeral envs themselves do not include that layer. This doc covers only the **kubeconfig self-service** feature that is in the main line: per-env kubeconfig download via the UI or `GET /api/envs/{name}/kubeconfig`.
