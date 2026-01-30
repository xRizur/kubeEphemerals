# ephemeral-operator

Kubernetes operator for **ephemeral environments**: short-lived namespaces with Helm-based deployments, NetworkPolicy isolation, and Gateway API HTTPRoute for access. Supports EnvironmentTemplate catalog and self-service kubeconfig via a built-in UI.

## Description

The ephemeral-operator manages two CRDs:

- **EphemeralEnv** – A single ephemeral environment (namespace, Helm releases, NetworkPolicy, HTTPRoute, TTL-based cleanup).
- **EnvironmentTemplate** – Reusable templates for creating EphemeralEnvs from the UI or CLI.

Use cases: PR preview environments, demo environments, temporary dev namespaces. The operator creates the namespace, deploys Helm charts, configures routing, and deletes the environment after the configured TTL.

## Getting Started

### Prerequisites

- Go 1.24+
- Docker 17.03+
- kubectl 1.11+
- Access to a Kubernetes 1.11+ cluster (e.g. [Kind](https://kind.sigs.k8s.io/) for local dev)

### Install with Helm (recommended)

1. Add the chart (when published) or use the local chart:

   ```sh
   # From repo root – install from local chart
   helm upgrade --install ephemeral-operator ./charts/ephemeral-operator \
     --namespace ephemeral-operator-system \
     --create-namespace
   ```

2. Override image and config as needed:

   ```sh
   helm upgrade --install ephemeral-operator ./charts/ephemeral-operator \
     --namespace ephemeral-operator-system \
     --create-namespace \
     --set image.repository=ghcr.io/<owner>/kubeephemerals/ephemeral-operator \
     --set image.tag=v0.1.0 \
     --set config.defaultTTL=24h \
     --set config.externalApiUrl=https://api.mycluster.example.com
   ```

3. Create sample resources:

   ```sh
   kubectl apply -k config/samples/
   ```

### Install with Kustomize (make deploy)

1. Build and push the image:

   ```sh
   make docker-build docker-push IMG=<your-registry>/ephemeral-operator:tag
   ```

2. Install CRDs and deploy the manager:

   ```sh
   make install
   make deploy IMG=<your-registry>/ephemeral-operator:tag
   ```

3. Create sample resources:

   ```sh
   kubectl apply -k config/samples/
   ```

**Note:** If you see RBAC errors, ensure you have sufficient privileges (e.g. cluster-admin) or are logged in as admin.

### Certificates and cert-manager

- **Default (no cert-manager):** The operator runs with **self-signed certificates** for the metrics endpoint. Controller-runtime generates them automatically. This is enough for development and many environments.
- **Production TLS with cert-manager:** This repo **does not install cert-manager**. To use cert-manager for metrics (or webhooks) you must:
  1. Install [cert-manager](https://cert-manager.io/docs/installation/) in the cluster yourself.
  2. For **Kustomize** deploy: uncomment the `[METRICS-WITH-CERTS]` and `[CERTMANAGER]` sections in `config/default/kustomization.yaml` and apply the cert-manager patches (see comments there). The patches reference a `metrics-server-cert` Secret and Certificate; you need to add the Certificate/Issuer resources in your overlay or a separate cert-manager config.
  3. For **Helm**: the chart does not yet include optional cert-manager integration (Certificate + volume mount). You can provide your own TLS secret and mount it via extra volumes/volumeMounts in values, or use an external process to populate the secret that the manager uses for metrics.

So: **cert-manager is optional**. Install it only if you want managed TLS for metrics (or webhooks). The operator works without it.

### Uninstall

**Helm:**

```sh
helm uninstall ephemeral-operator -n ephemeral-operator-system
kubectl delete ns ephemeral-operator-system
```

**Kustomize:**

```sh
kubectl delete -k config/samples/
make undeploy
make uninstall
```

## Project distribution

### YAML bundle

```sh
make build-installer IMG=<registry>/ephemeral-operator:tag
# Then: kubectl apply -f dist/install.yaml
```

### Helm chart

The chart lives under `charts/ephemeral-operator/`. To ship it:

- Package: `helm package charts/ephemeral-operator`
- Or push to a Helm repo and reference it in `helm install`.

**Note:** If you add webhooks or change CRDs, sync the chart manually (e.g. copy updated CRDs into `charts/ephemeral-operator/crds/` and adjust `values.yaml` / templates as needed).

## E2E tests

E2E tests require a **Kind** cluster and (optionally) **cert-manager**.

- **Kind:** Create a cluster and ensure `kind` is on `PATH`. Default cluster name: `ephemeral-operator-test-e2e` (override with `KIND_CLUSTER`).
- **Cert-manager:** Installed automatically by the suite unless `CERT_MANAGER_INSTALL_SKIP=true`.

Run:

```sh
make test-e2e
```

This will:

1. Build the manager image and load it into Kind.
2. Run **Manager (Kustomize)** tests: deploy with `make deploy`, verify pod, metrics, kubeconfig.
3. Run **Helm chart** tests: install the chart, verify pod and CRDs, create an EnvironmentTemplate and check it is reconciled.

So both deployment methods (Kustomize and Helm) are covered by e2e.

## Contributing

1. Follow TDD: write tests first, then implementation (see [AGENTS.md](AGENTS.md)).
2. Run `make test` and `make lint` before pushing.
3. Run `make test-e2e` in a Kind cluster for full validation.

Run `make help` for all available targets. For more context see the [Kubebuilder documentation](https://book.kubebuilder.io/introduction.html) and [ARCHITECTURE.md](ARCHITECTURE.md).

## License

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
