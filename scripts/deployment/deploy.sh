#!/bin/bash

# eAIIngest Deployment Script
# This script handles deployment to different environments

set -euo pipefail

# Default values
ENVIRONMENT=""
NAMESPACE="eaiingest"
DOCKER_REGISTRY="docker.io"
IMAGE_TAG="latest"
CONFIG_FILE=""
DRY_RUN=false
VERBOSE=false
HELM_CHART_PATH="./deployments/helm/eaiingest"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

show_usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Deploy eAIIngest to Kubernetes cluster

OPTIONS:
    -e, --environment ENVIRONMENT    Target environment (dev, staging, prod)
    -n, --namespace NAMESPACE        Kubernetes namespace (default: eaiingest)
    -r, --registry REGISTRY          Docker registry (default: docker.io)
    -t, --tag TAG                    Image tag (default: latest)
    -c, --config CONFIG_FILE         Additional config file
    -d, --dry-run                    Perform a dry run
    -v, --verbose                    Verbose output
    -h, --help                       Show this help message

EXAMPLES:
    $0 -e dev -t v1.0.0
    $0 -e prod -n production -r registry.company.com -t v1.2.3
    $0 -e staging --dry-run --verbose

EOF
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -e|--environment)
            ENVIRONMENT="$2"
            shift 2
            ;;
        -n|--namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        -r|--registry)
            DOCKER_REGISTRY="$2"
            shift 2
            ;;
        -t|--tag)
            IMAGE_TAG="$2"
            shift 2
            ;;
        -c|--config)
            CONFIG_FILE="$2"
            shift 2
            ;;
        -d|--dry-run)
            DRY_RUN=true
            shift
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -h|--help)
            show_usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            show_usage
            exit 1
            ;;
    esac
done

# Validate required parameters
if [[ -z "$ENVIRONMENT" ]]; then
    log_error "Environment is required. Use -e/--environment"
    show_usage
    exit 1
fi

# Validate environment
case $ENVIRONMENT in
    dev|development)
        ENVIRONMENT="development"
        ;;
    staging|stage)
        ENVIRONMENT="staging"
        ;;
    prod|production)
        ENVIRONMENT="production"
        ;;
    *)
        log_error "Invalid environment: $ENVIRONMENT. Must be one of: dev, staging, prod"
        exit 1
        ;;
esac

log_info "Starting deployment to $ENVIRONMENT environment"

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check if kubectl is installed and configured
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed or not in PATH"
        exit 1
    fi
    
    # Check if helm is installed
    if ! command -v helm &> /dev/null; then
        log_error "helm is not installed or not in PATH"
        exit 1
    fi
    
    # Check if docker is installed
    if ! command -v docker &> /dev/null; then
        log_error "docker is not installed or not in PATH"
        exit 1
    fi
    
    # Test kubectl connectivity
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster"
        exit 1
    fi
    
    log_success "Prerequisites check passed"
}

# Create namespace if it doesn't exist
create_namespace() {
    log_info "Ensuring namespace $NAMESPACE exists..."
    
    if kubectl get namespace "$NAMESPACE" &> /dev/null; then
        log_info "Namespace $NAMESPACE already exists"
    else
        if [[ "$DRY_RUN" == "true" ]]; then
            log_info "[DRY RUN] Would create namespace: $NAMESPACE"
        else
            kubectl create namespace "$NAMESPACE"
            log_success "Created namespace: $NAMESPACE"
        fi
    fi
}

# Build and push Docker image
build_and_push_image() {
    log_info "Building and pushing Docker image..."
    
    local image_name="${DOCKER_REGISTRY}/eaiingest/eaiingest:${IMAGE_TAG}"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY RUN] Would build and push image: $image_name"
        return
    fi
    
    # Build the image
    log_info "Building Docker image: $image_name"
    docker build -t "$image_name" .
    
    # Push the image
    log_info "Pushing Docker image: $image_name"
    docker push "$image_name"
    
    log_success "Image built and pushed: $image_name"
}

# Deploy with Helm
deploy_with_helm() {
    log_info "Deploying with Helm..."
    
    local release_name="eaiingest-$ENVIRONMENT"
    local values_file="$HELM_CHART_PATH/values-$ENVIRONMENT.yaml"
    
    # Check if environment-specific values file exists
    if [[ ! -f "$values_file" ]]; then
        values_file="$HELM_CHART_PATH/values.yaml"
        log_warn "Environment-specific values file not found, using default values.yaml"
    fi
    
    # Prepare helm command
    local helm_cmd="helm upgrade --install $release_name $HELM_CHART_PATH"
    helm_cmd="$helm_cmd --namespace $NAMESPACE"
    helm_cmd="$helm_cmd --values $values_file"
    helm_cmd="$helm_cmd --set app.image.tag=$IMAGE_TAG"
    helm_cmd="$helm_cmd --set app.image.registry=$DOCKER_REGISTRY"
    
    # Add additional config file if specified
    if [[ -n "$CONFIG_FILE" ]]; then
        helm_cmd="$helm_cmd --values $CONFIG_FILE"
    fi
    
    # Add environment-specific settings
    case $ENVIRONMENT in
        development)
            helm_cmd="$helm_cmd --set app.env.EAI_ENV=development"
            helm_cmd="$helm_cmd --set app.env.EAI_LOG_LEVEL=debug"
            helm_cmd="$helm_cmd --set app.replicaCount=1"
            ;;
        staging)
            helm_cmd="$helm_cmd --set app.env.EAI_ENV=staging"
            helm_cmd="$helm_cmd --set app.env.EAI_LOG_LEVEL=info"
            helm_cmd="$helm_cmd --set app.replicaCount=2"
            ;;
        production)
            helm_cmd="$helm_cmd --set app.env.EAI_ENV=production"
            helm_cmd="$helm_cmd --set app.env.EAI_LOG_LEVEL=warn"
            helm_cmd="$helm_cmd --set app.replicaCount=3"
            ;;
    esac
    
    if [[ "$DRY_RUN" == "true" ]]; then
        helm_cmd="$helm_cmd --dry-run"
        log_info "[DRY RUN] Helm command: $helm_cmd"
    fi
    
    if [[ "$VERBOSE" == "true" ]]; then
        helm_cmd="$helm_cmd --debug"
    fi
    
    # Execute helm command
    log_info "Executing: $helm_cmd"
    eval "$helm_cmd"
    
    if [[ "$DRY_RUN" != "true" ]]; then
        log_success "Deployment completed successfully"
    fi
}

# Wait for deployment to be ready
wait_for_deployment() {
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY RUN] Would wait for deployment to be ready"
        return
    fi
    
    log_info "Waiting for deployment to be ready..."
    
    local deployment_name="eaiingest-app"
    local timeout=300  # 5 minutes
    
    if kubectl wait --for=condition=available --timeout=${timeout}s deployment/$deployment_name -n "$NAMESPACE"; then
        log_success "Deployment is ready"
    else
        log_error "Deployment failed to become ready within $timeout seconds"
        exit 1
    fi
}

# Run post-deployment checks
post_deployment_checks() {
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY RUN] Would run post-deployment checks"
        return
    fi
    
    log_info "Running post-deployment checks..."
    
    # Check pod status
    log_info "Checking pod status..."
    kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/name=eaiingest
    
    # Check service status
    log_info "Checking service status..."
    kubectl get services -n "$NAMESPACE"
    
    # Test health endpoint
    log_info "Testing health endpoint..."
    local service_name="eaiingest-service"
    if kubectl port-forward service/$service_name 8080:8080 -n "$NAMESPACE" &
    then
        local port_forward_pid=$!
        sleep 5
        
        if curl -f http://localhost:8080/health &> /dev/null; then
            log_success "Health endpoint is responding"
        else
            log_error "Health endpoint is not responding"
        fi
        
        kill $port_forward_pid 2>/dev/null || true
    fi
    
    log_success "Post-deployment checks completed"
}

# Cleanup function
cleanup() {
    log_info "Cleaning up..."
    # Kill any background processes
    jobs -p | xargs -r kill 2>/dev/null || true
}

# Main execution
main() {
    # Set trap for cleanup
    trap cleanup EXIT
    
    log_info "===== eAIIngest Deployment ====="
    log_info "Environment: $ENVIRONMENT"
    log_info "Namespace: $NAMESPACE"
    log_info "Registry: $DOCKER_REGISTRY"
    log_info "Tag: $IMAGE_TAG"
    log_info "Dry Run: $DRY_RUN"
    log_info "Verbose: $VERBOSE"
    log_info "=================================="
    
    check_prerequisites
    create_namespace
    build_and_push_image
    deploy_with_helm
    wait_for_deployment
    post_deployment_checks
    
    log_success "Deployment completed successfully!"
    
    if [[ "$DRY_RUN" != "true" ]]; then
        log_info "You can access the application at:"
        kubectl get ingress -n "$NAMESPACE" -o wide
    fi
}

# Execute main function
main "$@"