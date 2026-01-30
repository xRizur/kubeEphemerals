#!/bin/bash
#
# Ephemeral Operator - Local Development Lab Setup
#
# This script sets up a local Kubernetes development environment with:
# - Minikube (4GB RAM, 2 CPUs)
# - Metrics Server addon (for Phase 7)
# - Gateway API CRDs (Standard v1.0+)
# - Envoy Gateway (production-ready Gateway implementation)
#
# Usage: ./scripts/setup-local-lab.sh
#
# The script is idempotent - safe to run multiple times.

set -euo pipefail

# =============================================================================
# Configuration
# =============================================================================

MINIKUBE_CPUS="${MINIKUBE_CPUS:-2}"
MINIKUBE_MEMORY="${MINIKUBE_MEMORY:-4096}"
GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.2.1}"
ENVOY_GATEWAY_VERSION="${ENVOY_GATEWAY_VERSION:-v1.2.4}"

# =============================================================================
# Colors and Logging
# =============================================================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info()  { echo -e "${GREEN}[INFO]${NC}  $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_step()  { echo -e "${BLUE}[STEP]${NC}  $1"; }

# =============================================================================
# Prerequisite Checks
# =============================================================================

check_prerequisites() {
    log_step "Checking prerequisites..."
    
    local missing=()
    
    if ! command -v minikube &> /dev/null; then
        missing+=("minikube")
    fi
    
    if ! command -v kubectl &> /dev/null; then
        missing+=("kubectl")
    fi
    
    if ! command -v helm &> /dev/null; then
        missing+=("helm")
    fi
    
    if [ ${#missing[@]} -ne 0 ]; then
        log_error "Missing required tools: ${missing[*]}"
        log_error "Please install them before running this script."
        echo ""
        echo "Installation guides:"
        echo "  - minikube: https://minikube.sigs.k8s.io/docs/start/"
        echo "  - kubectl:  https://kubernetes.io/docs/tasks/tools/"
        echo "  - helm:     https://helm.sh/docs/intro/install/"
        exit 1
    fi
    
    log_info "All prerequisites installed ✓"
}

# =============================================================================
# Minikube Setup
# =============================================================================

check_minikube_running() {
    if minikube status --format='{{.Host}}' 2>/dev/null | grep -q "Running"; then
        return 0
    fi
    return 1
}

setup_minikube() {
    log_step "Setting up Minikube..."
    
    if check_minikube_running; then
        log_info "Minikube is already running"
        
        # Show current configuration
        local current_cpus
        local current_memory
        current_cpus=$(minikube config get cpus 2>/dev/null || echo "unknown")
        current_memory=$(minikube config get memory 2>/dev/null || echo "unknown")
        log_info "Current config: CPUs=${current_cpus}, Memory=${current_memory}MB"
    else
        log_info "Starting Minikube with ${MINIKUBE_CPUS} CPUs and ${MINIKUBE_MEMORY}MB RAM..."
        minikube start \
            --cpus="${MINIKUBE_CPUS}" \
            --memory="${MINIKUBE_MEMORY}" \
            --addons=default-storageclass,storage-provisioner
        log_info "Minikube started successfully ✓"
    fi
}

enable_metrics_server() {
    log_step "Enabling metrics-server addon..."
    
    if minikube addons list | grep -q "metrics-server.*enabled"; then
        log_info "metrics-server addon is already enabled ✓"
    else
        minikube addons enable metrics-server
        log_info "metrics-server addon enabled ✓"
    fi
}

# =============================================================================
# Gateway API CRDs
# =============================================================================

install_gateway_api_crds() {
    log_step "Installing Gateway API CRDs (${GATEWAY_API_VERSION})..."
    
    # Check if Gateway API CRDs are already installed
    if kubectl get crd gateways.gateway.networking.k8s.io &> /dev/null; then
        local installed_version
        installed_version=$(kubectl get crd gateways.gateway.networking.k8s.io -o jsonpath='{.metadata.labels.gateway\.networking\.k8s\.io/bundle-version}' 2>/dev/null || echo "unknown")
        log_info "Gateway API CRDs already installed (version: ${installed_version})"
        log_info "Updating to ${GATEWAY_API_VERSION}..."
    fi
    
    kubectl apply -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml"
    log_info "Gateway API CRDs installed ✓"
}

# =============================================================================
# Envoy Gateway
# =============================================================================

install_envoy_gateway() {
    log_step "Installing Envoy Gateway (${ENVOY_GATEWAY_VERSION})..."
    
    # Check if already installed
    if kubectl get deployment -n envoy-gateway-system envoy-gateway &> /dev/null; then
        log_info "Envoy Gateway is already installed, upgrading..."
    fi
    
    # Install using OCI registry (recommended method)
    # See: https://gateway.envoyproxy.io/docs/install/install-helm/
    log_info "Installing Envoy Gateway from OCI registry..."
    helm upgrade --install eg oci://docker.io/envoyproxy/gateway-helm \
        --version "${ENVOY_GATEWAY_VERSION}" \
        --namespace envoy-gateway-system \
        --create-namespace \
        --wait \
        --timeout 5m
    
    log_info "Envoy Gateway installed ✓"
}

wait_for_envoy_gateway() {
    log_step "Waiting for Envoy Gateway controller to be ready..."
    
    # The deployment name is 'envoy-gateway' when using the OCI chart with release name 'eg'
    kubectl wait --namespace envoy-gateway-system \
        --for=condition=available \
        --timeout=120s \
        deployment/eg-envoy-gateway
    
    log_info "Envoy Gateway controller is ready ✓"
}

# =============================================================================
# Verification
# =============================================================================

verify_setup() {
    log_step "Verifying setup..."
    
    echo ""
    echo "┌─────────────────────────────────────────────────────────────────┐"
    echo "│                    Setup Verification                           │"
    echo "├─────────────────────────────────────────────────────────────────┤"
    
    # Minikube status
    if check_minikube_running; then
        echo -e "│ ✅ Minikube:        Running                                    │"
    else
        echo -e "│ ❌ Minikube:        Not Running                                │"
    fi
    
    # Metrics server
    if minikube addons list | grep -q "metrics-server.*enabled"; then
        echo -e "│ ✅ Metrics Server:  Enabled                                    │"
    else
        echo -e "│ ❌ Metrics Server:  Disabled                                   │"
    fi
    
    # Gateway API CRDs
    if kubectl get crd gateways.gateway.networking.k8s.io &> /dev/null; then
        echo -e "│ ✅ Gateway API:     CRDs Installed                             │"
    else
        echo -e "│ ❌ Gateway API:     CRDs Missing                               │"
    fi
    
    # Envoy Gateway
    if kubectl get deployment -n envoy-gateway-system eg-envoy-gateway &> /dev/null; then
        local ready
        ready=$(kubectl get deployment -n envoy-gateway-system eg-envoy-gateway -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
        if [ "$ready" -ge 1 ]; then
            echo -e "│ ✅ Envoy Gateway:   Ready (${ready} replica(s))                        │"
        else
            echo -e "│ ⏳ Envoy Gateway:   Starting...                               │"
        fi
    else
        echo -e "│ ❌ Envoy Gateway:   Not Installed                              │"
    fi
    
    echo "└─────────────────────────────────────────────────────────────────┘"
    echo ""
}

# =============================================================================
# Print Next Steps
# =============================================================================

print_next_steps() {
    echo ""
    log_info "=== Setup Complete ==="
    echo ""
    echo "Next steps:"
    echo ""
    echo "  1. Apply GatewayClass and Gateway:"
    echo "     kubectl apply -f config/samples/gateway_class_setup.yaml"
    echo ""
    echo "  2. Verify Gateway is ready:"
    echo "     kubectl get gatewayclass"
    echo "     kubectl get gateway"
    echo ""
    echo "  3. Install the operator CRDs:"
    echo "     make install"
    echo ""
    echo "  4. Run the operator locally:"
    echo "     make run"
    echo ""
    echo "  5. Create a test EphemeralEnv:"
    echo "     kubectl apply -f config/samples/ephemeral_v1alpha1_ephemeralenv.yaml"
    echo ""
    echo "To access services via Minikube tunnel (in a separate terminal):"
    echo "     minikube tunnel"
    echo ""
}

# =============================================================================
# Main
# =============================================================================

main() {
    echo ""
    echo "╔═══════════════════════════════════════════════════════════════════╗"
    echo "║       Ephemeral Operator - Local Development Lab Setup            ║"
    echo "╚═══════════════════════════════════════════════════════════════════╝"
    echo ""
    
    check_prerequisites
    setup_minikube
    enable_metrics_server
    install_gateway_api_crds
    install_envoy_gateway
    wait_for_envoy_gateway
    verify_setup
    print_next_steps
}

# Run main function
main "$@"
