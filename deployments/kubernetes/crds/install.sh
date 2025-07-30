#!/bin/bash

# AudiModal Kubernetes CRDs Installation Script
# This script installs all Custom Resource Definitions and supporting resources

set -euo pipefail

# Configuration
NAMESPACE="audimodal-system"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KUBECTL_CMD="${KUBECTL_CMD:-kubectl}"
DRY_RUN="${DRY_RUN:-false}"
FORCE="${FORCE:-false}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
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

# Check if kubectl is available
check_kubectl() {
    if ! command -v "${KUBECTL_CMD}" &> /dev/null; then
        log_error "kubectl is not installed or not in PATH"
        exit 1
    fi
    
    # Check cluster connectivity
    if ! "${KUBECTL_CMD}" cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster"
        exit 1
    fi
    
    log_success "kubectl is available and cluster is accessible"
}

# Check if running as cluster admin
check_permissions() {
    if ! "${KUBECTL_CMD}" auth can-i create customresourcedefinitions --all-namespaces &> /dev/null; then
        log_error "Insufficient permissions to install CRDs. Cluster admin access required."
        exit 1
    fi
    
    log_success "Sufficient permissions verified"
}

# Create namespace if it doesn't exist
create_namespace() {
    if "${KUBECTL_CMD}" get namespace "${NAMESPACE}" &> /dev/null; then
        log_info "Namespace ${NAMESPACE} already exists"
    else
        log_info "Creating namespace ${NAMESPACE}"
        if [[ "${DRY_RUN}" == "true" ]]; then
            echo "DRY RUN: Would create namespace ${NAMESPACE}"
        else
            "${KUBECTL_CMD}" create namespace "${NAMESPACE}"
            "${KUBECTL_CMD}" label namespace "${NAMESPACE}" app=audimodal component=system
            log_success "Namespace ${NAMESPACE} created"
        fi
    fi
}

# Install or update CRDs
install_crds() {
    local crd_files=(
        "tenant_crd.yaml"
        "datasource_crd.yaml"
        "dlp_policy_crd.yaml"
        "processing_session_crd.yaml"
    )
    
    log_info "Installing Custom Resource Definitions..."
    
    for crd_file in "${crd_files[@]}"; do
        local file_path="${SCRIPT_DIR}/${crd_file}"
        
        if [[ ! -f "${file_path}" ]]; then
            log_error "CRD file not found: ${file_path}"
            exit 1
        fi
        
        log_info "Installing CRD from ${crd_file}"
        
        if [[ "${DRY_RUN}" == "true" ]]; then
            echo "DRY RUN: Would apply ${crd_file}"
            "${KUBECTL_CMD}" apply --dry-run=client -f "${file_path}"
        else
            "${KUBECTL_CMD}" apply -f "${file_path}"
        fi
    done
    
    log_success "All CRDs installed successfully"
}

# Install RBAC resources
install_rbac() {
    local rbac_file="${SCRIPT_DIR}/rbac.yaml"
    
    if [[ ! -f "${rbac_file}" ]]; then
        log_error "RBAC file not found: ${rbac_file}"
        exit 1
    fi
    
    log_info "Installing RBAC resources..."
    
    if [[ "${DRY_RUN}" == "true" ]]; then
        echo "DRY RUN: Would apply RBAC resources"
        "${KUBECTL_CMD}" apply --dry-run=client -f "${rbac_file}"
    else
        "${KUBECTL_CMD}" apply -f "${rbac_file}"
    fi
    
    log_success "RBAC resources installed successfully"
}

# Install security policies
install_security_policies() {
    local security_file="${SCRIPT_DIR}/security_policies.yaml"
    
    if [[ ! -f "${security_file}" ]]; then
        log_warning "Security policies file not found: ${security_file}"
        return 0
    fi
    
    log_info "Installing security policies..."
    
    if [[ "${DRY_RUN}" == "true" ]]; then
        echo "DRY RUN: Would apply security policies"
        "${KUBECTL_CMD}" apply --dry-run=client -f "${security_file}"
    else
        # Apply security policies with warning suppression for optional resources
        "${KUBECTL_CMD}" apply -f "${security_file}" 2>/dev/null || {
            log_warning "Some security policies could not be installed (optional components may not be available)"
        }
    fi
    
    log_success "Security policies installation completed"
}

# Wait for CRDs to be established
wait_for_crds() {
    local crds=(
        "tenants.audimodal.ai"
        "datasources.audimodal.ai"
        "dlppolicies.audimodal.ai"
        "processingsessions.audimodal.ai"
    )
    
    log_info "Waiting for CRDs to be established..."
    
    for crd in "${crds[@]}"; do
        log_info "Waiting for CRD ${crd} to be ready..."
        
        if [[ "${DRY_RUN}" == "true" ]]; then
            echo "DRY RUN: Would wait for ${crd}"
            continue
        fi
        
        local timeout=60
        local elapsed=0
        
        while [[ ${elapsed} -lt ${timeout} ]]; do
            if "${KUBECTL_CMD}" get crd "${crd}" -o jsonpath='{.status.conditions[?(@.type=="Established")].status}' 2>/dev/null | grep -q "True"; then
                log_success "CRD ${crd} is established"
                break
            fi
            
            sleep 2
            elapsed=$((elapsed + 2))
        done
        
        if [[ ${elapsed} -ge ${timeout} ]]; then
            log_error "Timeout waiting for CRD ${crd} to be established"
            exit 1
        fi
    done
    
    log_success "All CRDs are ready"
}

# Verify installation
verify_installation() {
    log_info "Verifying installation..."
    
    # Check CRDs
    local expected_crds=(
        "tenants.audimodal.ai"
        "datasources.audimodal.ai"
        "dlppolicies.audimodal.ai"
        "processingsessions.audimodal.ai"
    )
    
    for crd in "${expected_crds[@]}"; do
        if "${KUBECTL_CMD}" get crd "${crd}" &> /dev/null; then
            log_success "✓ CRD ${crd} is installed"
        else
            log_error "✗ CRD ${crd} is missing"
            return 1
        fi
    done
    
    # Check namespace
    if "${KUBECTL_CMD}" get namespace "${NAMESPACE}" &> /dev/null; then
        log_success "✓ Namespace ${NAMESPACE} exists"
    else
        log_error "✗ Namespace ${NAMESPACE} is missing"
        return 1
    fi
    
    # Check RBAC
    if "${KUBECTL_CMD}" get clusterrole audimodal-operator &> /dev/null; then
        log_success "✓ Operator ClusterRole exists"
    else
        log_error "✗ Operator ClusterRole is missing"
        return 1
    fi
    
    log_success "Installation verification completed successfully"
}

# Create sample resources
create_samples() {
    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "DRY RUN: Would create sample resources"
        return 0
    fi
    
    log_info "Creating sample resources..."
    
    # Sample tenant
    cat <<EOF | "${KUBECTL_CMD}" apply -f -
apiVersion: audimodal.ai/v1
kind: Tenant
metadata:
  name: sample-tenant
  namespace: ${NAMESPACE}
spec:
  name: "Sample Tenant"
  slug: "sample-tenant"
  plan: "pro"
  settings:
    maxStorageGB: 100
    maxUsers: 10
    maxDataSources: 5
    enableDLP: true
  resources:
    cpu: "2"
    memory: "4Gi"
    storage: "50Gi"
  security:
    isolationLevel: "namespace"
    encryptionAtRest: true
    encryptionInTransit: true
    networkPolicies: true
EOF

    # Sample data source
    cat <<EOF | "${KUBECTL_CMD}" apply -f -
apiVersion: audimodal.ai/v1
kind: DataSource
metadata:
  name: sample-filesystem
  namespace: ${NAMESPACE}
spec:
  name: "Sample Filesystem Source"
  type: "filesystem"
  enabled: true
  config:
    path: "/data/documents"
    maxFileSize: "100MB"
    supportedExtensions: [".pdf", ".docx", ".txt"]
  sync:
    mode: "scheduled"
    schedule: "0 */6 * * *"
    batchSize: 50
  processing:
    chunkingStrategy: "semantic"
    chunkSize: 1000
    enableAnalysis: true
EOF

    log_success "Sample resources created"
}

# Uninstall function
uninstall() {
    log_warning "Uninstalling AudiModal CRDs and resources..."
    
    if [[ "${FORCE}" != "true" ]]; then
        read -p "Are you sure you want to uninstall? This will delete all AudiModal resources. (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "Uninstall cancelled"
            exit 0
        fi
    fi
    
    # Delete custom resources first
    log_info "Deleting custom resources..."
    "${KUBECTL_CMD}" delete tenants --all --all-namespaces 2>/dev/null || true
    "${KUBECTL_CMD}" delete datasources --all --all-namespaces 2>/dev/null || true
    "${KUBECTL_CMD}" delete dlppolicies --all --all-namespaces 2>/dev/null || true
    "${KUBECTL_CMD}" delete processingsessions --all --all-namespaces 2>/dev/null || true
    
    # Delete CRDs
    log_info "Deleting CRDs..."
    "${KUBECTL_CMD}" delete crd tenants.audimodal.ai 2>/dev/null || true
    "${KUBECTL_CMD}" delete crd datasources.audimodal.ai 2>/dev/null || true
    "${KUBECTL_CMD}" delete crd dlppolicies.audimodal.ai 2>/dev/null || true
    "${KUBECTL_CMD}" delete crd processingsessions.audimodal.ai 2>/dev/null || true
    
    # Delete RBAC
    log_info "Deleting RBAC resources..."
    "${KUBECTL_CMD}" delete clusterrole audimodal-operator 2>/dev/null || true
    "${KUBECTL_CMD}" delete clusterrolebinding audimodal-operator 2>/dev/null || true
    "${KUBECTL_CMD}" delete clusterrole audimodal-tenant-admin 2>/dev/null || true
    "${KUBECTL_CMD}" delete clusterrole audimodal-tenant-user 2>/dev/null || true
    "${KUBECTL_CMD}" delete clusterrole audimodal-viewer 2>/dev/null || true
    "${KUBECTL_CMD}" delete clusterrole audimodal-admin 2>/dev/null || true
    
    # Delete namespace
    log_info "Deleting namespace..."
    "${KUBECTL_CMD}" delete namespace "${NAMESPACE}" 2>/dev/null || true
    
    log_success "Uninstall completed"
}

# Display help
show_help() {
    cat <<EOF
AudiModal Kubernetes CRDs Installation Script

Usage: $0 [OPTIONS] [COMMAND]

Commands:
  install     Install CRDs and supporting resources (default)
  uninstall   Remove all AudiModal CRDs and resources
  verify      Verify the installation
  samples     Create sample resources
  help        Show this help message

Options:
  --namespace NAME    Kubernetes namespace (default: audimodal-system)
  --dry-run          Show what would be done without making changes
  --force            Skip confirmation prompts
  --kubectl PATH     Path to kubectl binary (default: kubectl)

Environment Variables:
  KUBECTL_CMD        Path to kubectl binary
  DRY_RUN           Set to 'true' for dry-run mode
  FORCE             Set to 'true' to skip confirmations

Examples:
  $0 install                    # Install CRDs and resources
  $0 install --dry-run          # Show what would be installed
  $0 uninstall --force          # Uninstall without confirmation
  $0 verify                     # Verify installation
  $0 samples                    # Create sample resources

EOF
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        --dry-run)
            DRY_RUN="true"
            shift
            ;;
        --force)
            FORCE="true"
            shift
            ;;
        --kubectl)
            KUBECTL_CMD="$2"
            shift 2
            ;;
        install|uninstall|verify|samples|help)
            COMMAND="$1"
            shift
            ;;
        -h|--help)
            COMMAND="help"
            shift
            ;;
        *)
            log_error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Set default command
COMMAND="${COMMAND:-install}"

# Main execution
main() {
    case "${COMMAND}" in
        install)
            log_info "Starting AudiModal CRDs installation..."
            check_kubectl
            check_permissions
            create_namespace
            install_crds
            wait_for_crds
            install_rbac
            install_security_policies
            verify_installation
            log_success "AudiModal CRDs installation completed successfully!"
            log_info "You can now deploy the AudiModal operator and create tenants."
            ;;
        uninstall)
            check_kubectl
            uninstall
            ;;
        verify)
            check_kubectl
            verify_installation
            ;;
        samples)
            check_kubectl
            create_samples
            ;;
        help)
            show_help
            ;;
        *)
            log_error "Unknown command: ${COMMAND}"
            show_help
            exit 1
            ;;
    esac
}

# Run main function
main