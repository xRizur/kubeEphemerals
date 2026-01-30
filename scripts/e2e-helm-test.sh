#!/bin/bash
# =============================================================================
# E2E Test Script for Ephemeral Operator with Helm Deployment
# =============================================================================
# This script tests the full lifecycle:
# 1. Deploy the operator
# 2. Create an EphemeralEnv with Helm chart
# 3. Verify all resources are created
# 4. Test Gateway routing
# 5. Test cleanup on deletion
# =============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE="default"
ENV_NAME="pr-helm-test"
EXPECTED_NAMESPACE="env-${ENV_NAME}"
TIMEOUT=300  # 5 minutes timeout for Helm deployment

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

wait_for_condition() {
    local resource=$1
    local condition=$2
    local timeout=$3
    local namespace=$4
    
    log_info "Waiting for $resource to be $condition (timeout: ${timeout}s)..."
    
    local ns_flag=""
    if [ -n "$namespace" ]; then
        ns_flag="-n $namespace"
    fi
    
    if kubectl wait $ns_flag --for=condition=$condition $resource --timeout=${timeout}s 2>/dev/null; then
        log_success "$resource is $condition"
        return 0
    else
        log_error "$resource did not become $condition within ${timeout}s"
        return 1
    fi
}

wait_for_pods() {
    local namespace=$1
    local timeout=$2
    
    log_info "Waiting for pods in namespace $namespace to be ready..."
    
    local end_time=$((SECONDS + timeout))
    while [ $SECONDS -lt $end_time ]; do
        local ready=$(kubectl get pods -n "$namespace" --no-headers 2>/dev/null | grep -c "Running" || echo "0")
        local total=$(kubectl get pods -n "$namespace" --no-headers 2>/dev/null | wc -l || echo "0")
        
        if [ "$total" -gt 0 ] && [ "$ready" -eq "$total" ]; then
            log_success "All $total pods are running in namespace $namespace"
            return 0
        fi
        
        log_info "Waiting... ($ready/$total pods ready)"
        sleep 5
    done
    
    log_error "Pods did not become ready within ${timeout}s"
    kubectl get pods -n "$namespace"
    return 1
}

# =============================================================================
# Test Functions
# =============================================================================

test_prerequisites() {
    log_info "=== Checking Prerequisites ==="
    
    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl not found"
        exit 1
    fi
    log_success "kubectl found"
    
    # Check cluster connection
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster"
        exit 1
    fi
    log_success "Connected to Kubernetes cluster"
    
    # Check if CRD exists
    if ! kubectl get crd ephemeralenvs.ephemeral.ephemeralenv.io &> /dev/null; then
        log_error "EphemeralEnv CRD not found. Run 'make install' first."
        exit 1
    fi
    log_success "EphemeralEnv CRD exists"
    
    # Check if Gateway exists
    if ! kubectl get gateway main-gateway -n default &> /dev/null; then
        log_warning "Gateway 'main-gateway' not found. Creating it..."
        kubectl apply -f config/samples/gateway_class_setup.yaml
        sleep 5
    fi
    log_success "Gateway 'main-gateway' exists"
    
    echo ""
}

test_create_ephemeralenv() {
    log_info "=== Test: Create EphemeralEnv with Helm ==="
    
    # Clean up any existing test resources
    log_info "Cleaning up any existing test resources..."
    kubectl delete ephemeralenv ${ENV_NAME} --ignore-not-found=true
    kubectl delete namespace ${EXPECTED_NAMESPACE} --ignore-not-found=true
    sleep 5
    
    # Create the EphemeralEnv
    log_info "Creating EphemeralEnv '${ENV_NAME}'..."
    kubectl apply -f config/samples/ephemeral_v1alpha1_ephemeralenv_helm.yaml
    
    # Wait for namespace to be created
    log_info "Waiting for namespace '${EXPECTED_NAMESPACE}' to be created..."
    local end_time=$((SECONDS + 30))
    while [ $SECONDS -lt $end_time ]; do
        if kubectl get namespace ${EXPECTED_NAMESPACE} &> /dev/null; then
            log_success "Namespace '${EXPECTED_NAMESPACE}' created"
            break
        fi
        sleep 2
    done
    
    if ! kubectl get namespace ${EXPECTED_NAMESPACE} &> /dev/null; then
        log_error "Namespace was not created"
        kubectl describe ephemeralenv ${ENV_NAME}
        return 1
    fi
    
    echo ""
}

test_helm_deployment() {
    log_info "=== Test: Helm Chart Deployment ==="
    
    # Wait for Helm release to be deployed
    log_info "Waiting for Helm deployment (this may take a few minutes)..."
    
    if ! wait_for_pods ${EXPECTED_NAMESPACE} ${TIMEOUT}; then
        log_error "Helm deployment failed"
        log_info "Checking EphemeralEnv status..."
        kubectl get ephemeralenv ${ENV_NAME} -o yaml
        log_info "Checking events in namespace..."
        kubectl get events -n ${EXPECTED_NAMESPACE} --sort-by='.lastTimestamp'
        return 1
    fi
    
    # Check HelmDeployed condition
    log_info "Checking HelmDeployed condition..."
    local helm_status=$(kubectl get ephemeralenv ${ENV_NAME} -o jsonpath='{.status.conditions[?(@.type=="HelmDeployed")].status}')
    
    if [ "$helm_status" == "True" ]; then
        log_success "HelmDeployed condition is True"
    else
        log_error "HelmDeployed condition is not True (got: $helm_status)"
        kubectl get ephemeralenv ${ENV_NAME} -o yaml
        return 1
    fi
    
    # Check HelmRelease status field
    local helm_release=$(kubectl get ephemeralenv ${ENV_NAME} -o jsonpath='{.status.helmRelease}')
    log_info "Helm release name: ${helm_release}"
    
    if [ -n "$helm_release" ]; then
        log_success "HelmRelease status field is set"
    else
        log_warning "HelmRelease status field is empty"
    fi
    
    echo ""
}

test_gateway_routing() {
    log_info "=== Test: Gateway API Routing ==="
    
    # Check HTTPRoute
    log_info "Checking HTTPRoute..."
    local route_name="route-${ENV_NAME}"
    
    if kubectl get httproute ${route_name} -n default &> /dev/null; then
        log_success "HTTPRoute '${route_name}' exists"
    else
        log_error "HTTPRoute '${route_name}' not found"
        return 1
    fi
    
    # Check HTTPRouteReady condition
    local route_status=$(kubectl get ephemeralenv ${ENV_NAME} -o jsonpath='{.status.conditions[?(@.type=="HTTPRouteReady")].status}')
    
    if [ "$route_status" == "True" ]; then
        log_success "HTTPRouteReady condition is True"
    else
        log_warning "HTTPRouteReady condition is not True (got: $route_status)"
    fi
    
    # Check AccessURL
    local access_url=$(kubectl get ephemeralenv ${ENV_NAME} -o jsonpath='{.status.accessURL}')
    log_info "Access URL: ${access_url}"
    
    if [ -n "$access_url" ]; then
        log_success "AccessURL is set"
    else
        log_warning "AccessURL is not set"
    fi
    
    # Check ReferenceGrant
    log_info "Checking ReferenceGrant..."
    local grant_name="grant-${ENV_NAME}"
    
    if kubectl get referencegrant ${grant_name} -n ${EXPECTED_NAMESPACE} &> /dev/null; then
        log_success "ReferenceGrant '${grant_name}' exists"
    else
        log_warning "ReferenceGrant '${grant_name}' not found (may not be needed if same namespace)"
    fi
    
    echo ""
}

test_network_policy() {
    log_info "=== Test: NetworkPolicy ==="
    
    # Check if NetworkPolicy exists (isolation was disabled for this test)
    if kubectl get networkpolicy deny-all -n ${EXPECTED_NAMESPACE} &> /dev/null; then
        log_info "NetworkPolicy 'deny-all' exists (isolation enabled)"
    else
        log_info "NetworkPolicy 'deny-all' not found (isolation disabled - expected)"
    fi
    
    echo ""
}

test_ephemeralenv_status() {
    log_info "=== Test: EphemeralEnv Status ==="
    
    # Get full status
    log_info "EphemeralEnv Status:"
    kubectl get ephemeralenv ${ENV_NAME} -o wide
    
    echo ""
    log_info "Conditions:"
    kubectl get ephemeralenv ${ENV_NAME} -o jsonpath='{range .status.conditions[*]}{.type}: {.status} ({.reason}){"\n"}{end}'
    
    # Check phase
    local phase=$(kubectl get ephemeralenv ${ENV_NAME} -o jsonpath='{.status.phase}')
    log_info "Phase: ${phase}"
    
    if [ "$phase" == "Active" ]; then
        log_success "Environment is Active"
    else
        log_warning "Environment is not Active (got: $phase)"
    fi
    
    # Check expiration time
    local expiration=$(kubectl get ephemeralenv ${ENV_NAME} -o jsonpath='{.status.expirationTime}')
    log_info "Expiration Time: ${expiration}"
    
    echo ""
}

test_cleanup() {
    log_info "=== Test: Cleanup on Deletion ==="
    
    # Delete the EphemeralEnv
    log_info "Deleting EphemeralEnv '${ENV_NAME}'..."
    kubectl delete ephemeralenv ${ENV_NAME}
    
    # Wait for namespace to be deleted
    log_info "Waiting for namespace '${EXPECTED_NAMESPACE}' to be deleted..."
    local end_time=$((SECONDS + 60))
    while [ $SECONDS -lt $end_time ]; do
        if ! kubectl get namespace ${EXPECTED_NAMESPACE} &> /dev/null; then
            log_success "Namespace '${EXPECTED_NAMESPACE}' deleted"
            break
        fi
        sleep 2
    done
    
    if kubectl get namespace ${EXPECTED_NAMESPACE} &> /dev/null; then
        log_warning "Namespace still exists (may take time to terminate)"
    fi
    
    # Check HTTPRoute is deleted
    local route_name="route-${ENV_NAME}"
    if ! kubectl get httproute ${route_name} -n default &> /dev/null; then
        log_success "HTTPRoute '${route_name}' deleted"
    else
        log_warning "HTTPRoute '${route_name}' still exists"
    fi
    
    echo ""
}

print_summary() {
    log_info "=== Resources Created ==="
    echo ""
    
    log_info "Namespace:"
    kubectl get namespace ${EXPECTED_NAMESPACE} --ignore-not-found
    
    echo ""
    log_info "Pods in namespace:"
    kubectl get pods -n ${EXPECTED_NAMESPACE} --ignore-not-found
    
    echo ""
    log_info "Services in namespace:"
    kubectl get svc -n ${EXPECTED_NAMESPACE} --ignore-not-found
    
    echo ""
    log_info "HTTPRoute:"
    kubectl get httproute -n default --ignore-not-found
    
    echo ""
    log_info "EphemeralEnv:"
    kubectl get ephemeralenv --ignore-not-found
    
    echo ""
}

# =============================================================================
# Main
# =============================================================================

main() {
    echo "=============================================="
    echo "  Ephemeral Operator E2E Test with Helm"
    echo "=============================================="
    echo ""
    
    cd "$(dirname "$0")/.."
    
    test_prerequisites
    test_create_ephemeralenv
    test_helm_deployment
    test_gateway_routing
    test_network_policy
    test_ephemeralenv_status
    
    print_summary
    
    echo ""
    log_info "Press Enter to run cleanup test, or Ctrl+C to keep resources..."
    read -r
    
    test_cleanup
    
    echo ""
    echo "=============================================="
    log_success "E2E Tests Complete!"
    echo "=============================================="
}

# Run main if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
