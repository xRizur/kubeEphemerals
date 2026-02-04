# Installation

Install the Ephemeral Operator with Helm. The chart installs the controller, RBAC, and optional CRDs.

## Helm (local chart)

From the repository root:

```bash
helm upgrade --install ephemeral-operator ./charts/ephemeral-operator \
  --namespace ephemeral-operator-system \
  --create-namespace
```

## Helm (OCI)

When the chart is published to a registry:

```bash
helm install my-op oci://ghcr.io/your-org/kubeEphemerals/charts/ephemeral-operator \
  --namespace ephemeral-operator-system \
  --create-namespace
```

Replace `your-org/kubeEphemerals` with your registry path.

## Verify

```bash
kubectl get pods -n ephemeral-operator-system
```

The operator runs:

- **Controller** – reconciles EphemeralEnv and EnvironmentTemplate (port 8081 for probes).
- **UI server** – dashboard on port **8082** (configurable via [Helm values](/docs/helm-values)).

## Next steps

- Configure [Helm values](/docs/helm-values) (e.g. `config.uiBaseDomain`, `config.externalApiUrl` for kubeconfig).
- Set up a [Gateway and HTTPRoute](/docs/gateway-httproute) so environments get URLs.
- Create an [EphemeralEnv](/docs/crd-ephemeralenv) or [EnvironmentTemplate](/docs/crd-environmenttemplate).
