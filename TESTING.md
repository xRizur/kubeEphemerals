# Phase 12: Authentication & Multi-tenancy – Testing Guide

This document describes how to install, log in, and verify the authentication and multi-tenant isolation added in Phase 12.

## Architecture (Chained Proxy with Gateway API)

1. **Gateway API** receives external traffic.
2. **HTTPRoute** forwards traffic to **OAuth2 Proxy**.
3. **OAuth2 Proxy** performs authentication (htpasswd by default; OIDC optional via values).
4. **Go backend** trusts the `X-Forwarded-User` header and enforces owner-based isolation.

---

## 1. Install with Helm

### Prerequisites

- Kubernetes cluster with Gateway API CRDs installed.
- Helm 3.

### Add the OAuth2 Proxy Helm repo (when using auth)

```bash
helm repo add oauth2-proxy https://oauth2-proxy.github.io/oauth2-proxy
helm repo update
```

### Install the operator (no auth – dev/local)

```bash
helm install ephemeral-operator ./charts/ephemeral-operator \
  --namespace ephemeral-operator-system \
  --create-namespace
```

### Install with auth (htpasswd – admin / password out of the box)

```bash
helm install ephemeral-operator ./charts/ephemeral-operator \
  --namespace ephemeral-operator-system \
  --create-namespace \
  --set auth.enabled=true \
  --set oauth2-proxy.config.existingConfig=ephemeral-operator-system-ephemeral-operator-oauth2-config
```

**Note:** Replace `ephemeral-operator-system-ephemeral-operator-oauth2-config` with `<release-namespace>-ephemeral-operator-oauth2-config` if you use a different release or namespace. The chart creates a ConfigMap with that name containing the correct upstream to the UI service.

### Optional: Platform HTTPRoute (Gateway API)

If you use a Gateway for the platform dashboard, set:

```bash
--set auth.gateway.name=main-gateway \
--set auth.gateway.namespace=gateway-system \
--set auth.gateway.hostname=platform.local
```

Then the chart creates an HTTPRoute that sends traffic to **oauth2-proxy** (not directly to the app), enforcing the auth chain.

---

## 2. Log in with admin / password

When auth is enabled with the default htpasswd secret:

- **Username:** `admin`
- **Password:** `password`

Use these credentials at the platform dashboard URL (e.g. `https://platform.local` or your Gateway host). OAuth2 Proxy will set `X-Forwarded-User` to `admin`, and the backend will treat that user as admin (can list and delete all environments).

**Change the default password in production:** override the auth secret, e.g.:

```bash
# Generate htpasswd (bcrypt)
htpasswd -nbB admin 'your-secure-password'

# Use in Helm
--set auth.htpasswd='admin:$2y$10$...'
```

---

## 3. Switch to GitHub (or other) OAuth

To use GitHub OIDC instead of htpasswd, adjust `values.yaml` (or `--set`) so that OAuth2 Proxy uses an OIDC provider instead of htpasswd:

1. **Disable htpasswd and set OIDC config** (example for GitHub):

   ```yaml
   auth:
     enabled: true
     htpasswd: ""   # leave empty when using OIDC

   oauth2-proxy:
     config:
       configFile: |
         provider = "github"
         client_id = "your-github-client-id"
         client_secret_file = "/etc/oauth2-proxy/client-secret"
         redirect_url = "https://platform.local/oauth2/callback"
         upstreams = [ "http://ephemeral-operator:80" ]
       # Or keep using existingConfig and ensure the ConfigMap upstream matches your UI service
     extraArgs:
       set-xauthrequest: "true"
   ```

2. Create a Secret with your GitHub OAuth client secret and mount it as needed (see [oauth2-proxy docs](https://oauth2-proxy.github.io/oauth2-proxy/docs/configuration/overview)).

3. Reinstall or upgrade the release. Users will then log in via GitHub; `X-Forwarded-User` will be set from the OIDC identity.

---

## 4. Run Go tests for security logic

From the project root (preferably in WSL as per AGENTS.md):

```bash
# All UI-related tests (middleware + isolation)
go test ./pkg/ui/... ./internal/ui/... -v -count=1
```

### What is covered

- **`pkg/ui/middleware_test.go` – TestAuthMiddleware**
  - **Scenario A (Prod):** Request with `X-Forwarded-User: alice` → context user is `alice`.
  - **Scenario B (Dev):** No header, `UnsafeDevMode=true` → context user is `dev@local`.
  - **Scenario C (Unauthorized):** No header, `UnsafeDevMode=false` → HTTP 401.

- **`pkg/ui/handlers_test.go` – TestListEnvs_Isolation**
  - **Scenario A:** User `alice` → list returns only envs with `spec.owner == alice`.
  - **Scenario B:** User `bob` → delete of `alice`’s env returns 403 Forbidden.
  - **Scenario C:** User `admin` → list returns all envs (EnvA and EnvB).

These tests confirm that the backend correctly uses the forwarded user and enforces owner-based isolation and admin override.

---

## 5. E2E tests (Kind cluster)

E2E tests run on a **real Kind cluster** and cover corner cases that unit/envtest cannot.

**Prerequisites:** Kind installed, Docker (for image build). Default cluster: `ephemeral-operator-test-e2e` (override with `KIND_CLUSTER`).

**Run (from WSL or Linux):**

```bash
make test-e2e
```

This will: build the manager image, load it into Kind, run Manager suite (deploy, patch `UNSAFE_DEV_MODE=true`, pod/metrics/kubeconfig, **EphemeralEnv lifecycle and corner cases**), then Helm suite.

**Corner cases covered:**

- EphemeralEnv with `spec.owner`: create → namespace/status; list with `X-Forwarded-User: alice` returns only alice's envs.
- Delete as non-owner (bob deletes alice's env) → **403 Forbidden**.
- Delete as owner (alice deletes own env) → **204 No Content**.
- GET `/api/templates` → 200.
- Invalid EphemeralEnv (e.g. missing gateway namespace) → stays Pending or validation error.

See `test/e2e/e2e_test.go` and `test/e2e/fixtures/` for fixtures and test flow.

---

## 6. After changing CRD types (e.g. Owner)

If you modify `api/v1alpha1/ephemeralenv_types.go` (or other types), regenerate manifests and deepcopy from **WSL**:

```bash
wsl bash -c "cd /mnt/c/Users/<user>/github/kubeEphemerals && make manifests generate"
```

Then reinstall or upgrade the chart so CRDs include the new fields.
