#!/bin/bash
# Install operator + OAuth. Safe to run multiple times.
# Run from repo root: ./examples/oauth-demo/install-oauth.sh
# Or from this folder: ./install-oauth.sh (needs repo root 2 levels up)
# Login: admin / password

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Repo root: when script is in examples/oauth-demo, go up twice
REPO_ROOT="${REPO_ROOT:-$(cd "${SCRIPT_DIR}/../.." && pwd)}"
NS="${RELEASE_NAMESPACE:-ephemeral-operator-system}"

cd "$REPO_ROOT"

# 1. Envoy (skip if already there)
if helm list -n envoy-gateway-system -q 2>/dev/null | grep -q "^eg$"; then
  echo "[1/5] Envoy – skip"
else
  echo "[1/5] Envoy – install"
  SKIP_CRDS=()
  kubectl get crd gateways.gateway.networking.k8s.io &>/dev/null && SKIP_CRDS=(--skip-crds)
  helm upgrade --install eg oci://docker.io/envoyproxy/gateway-helm \
    --version "${ENVOY_GATEWAY_VERSION:-v1.2.4}" \
    -n envoy-gateway-system --create-namespace --wait --timeout 5m \
    "${SKIP_CRDS[@]}"
fi

# 2. Gateway
echo "[2/5] Gateway – apply"
kubectl apply -f "${SCRIPT_DIR}/gateway.yaml"

# 3. Repos + deps
echo "[3/5] Repos – deps"
helm repo add oauth2-proxy https://oauth2-proxy.github.io/manifests
helm repo update
helm dependency update "${REPO_ROOT}/charts/ephemeral-operator"

# 4. Operator + auth
echo "[4/5] Operator – install"
CONFIG_NAME="${NS}-ephemeral-operator-oauth2-config"
helm upgrade --install ephemeral-operator "${REPO_ROOT}/charts/ephemeral-operator" \
  -n "$NS" --create-namespace \
  --set auth.enabled=true \
  --set auth.gateway.name=main-gateway \
  --set auth.gateway.namespace=default \
  --set auth.gateway.hostname=platform.local \
  --set "oauth2-proxy.config.existingConfig=${CONFIG_NAME}"

# 5. Wait
echo "[5/5] Wait"
kubectl wait -n "$NS" --for=condition=available --timeout=120s deployment/ephemeral-operator 2>/dev/null || true
kubectl wait -n "$NS" --for=condition=available --timeout=120s deployment/ephemeral-operator-oauth2-proxy 2>/dev/null || true

echo ""
echo "Done. http://platform.local → Gateway IP. Login: admin / password"
echo "Sample: kubectl apply -f ${REPO_ROOT}/config/samples/ephemeral_v1alpha1_ephemeralenv.yaml"
