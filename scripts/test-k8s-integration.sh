#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE="${NAMESPACE:-aether-be}"
TEST_POD_NAME="audimodal-integration-test"
GO_IMAGE="golang:1.24"

# Functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓ PASS]${NC} $1"
}

log_error() {
    echo -e "${RED}[✗ FAIL]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

cleanup() {
    log_info "Cleaning up test pod..."
    kubectl delete pod "$TEST_POD_NAME" -n "$NAMESPACE" --ignore-not-found=true 2>/dev/null || true
}

# Trap cleanup on exit
trap cleanup EXIT

echo "═══════════════════════════════════════════════════════════════"
echo " AudiModal Kubernetes Integration Tests"
echo "═══════════════════════════════════════════════════════════════"
echo ""

# Step 1: Verify kubectl connectivity
log_info "Verifying kubectl connectivity..."
if ! kubectl cluster-info &>/dev/null; then
    log_error "Cannot connect to Kubernetes cluster"
    exit 1
fi
log_success "Connected to Kubernetes cluster"

# Step 2: Verify namespace exists
log_info "Checking namespace: $NAMESPACE"
if ! kubectl get namespace "$NAMESPACE" &>/dev/null; then
    log_error "Namespace $NAMESPACE does not exist"
    exit 1
fi
log_success "Namespace $NAMESPACE exists"

# Step 3: Verify required services are running
log_info "Verifying required services..."

if ! kubectl get pods -n "$NAMESPACE" -l app=audimodal --field-selector=status.phase=Running | grep -q Running; then
    log_warning "AudiModal pod is not running"
fi

if ! kubectl get pods -n "$NAMESPACE" -l app=deeplake-api --field-selector=status.phase=Running | grep -q Running; then
    log_warning "DeepLake API pod is not running"
fi

# Step 4: Clean up any existing test pod
log_info "Cleaning up any existing test pod..."
cleanup

# Step 5: Get source code directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AUDIMODAL_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

if [ ! -d "$AUDIMODAL_DIR/tests" ]; then
    log_error "Tests directory not found at $AUDIMODAL_DIR/tests"
    exit 1
fi

log_info "Using audimodal directory: $AUDIMODAL_DIR"

# Step 6: Get environment variables from existing audimodal deployment
log_info "Extracting environment variables from audimodal deployment..."
OPENAI_API_KEY=$(kubectl get deployment audimodal -n "$NAMESPACE" -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="OPENAI_API_KEY")].value}' 2>/dev/null || echo "")
DEEPLAKE_API_KEY=$(kubectl get deployment audimodal -n "$NAMESPACE" -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="DEEPLAKE_API_KEY")].value}' 2>/dev/null || echo "")

# Step 7: Create test pod
log_info "Creating test pod..."
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: $TEST_POD_NAME
  namespace: $NAMESPACE
spec:
  restartPolicy: Never
  containers:
  - name: test-runner
    image: $GO_IMAGE
    command: ["/bin/bash", "-c"]
    args:
    - |
      set -e
      echo "Installing system dependencies..."
      apt-get update -qq
      apt-get install -y -qq librdkafka-dev pkg-config

      echo "Waiting for test code to be copied..."
      while [ ! -f /workspace/tests-ready ]; do
        sleep 1
      done

      echo "Setting up test environment..."
      cd /workspace

      echo "Installing Go dependencies..."
      go mod download

      echo "Running integration tests..."
      go test ./tests -v -timeout=30m

      TEST_EXIT_CODE=\$?
      echo "Tests completed with exit code: \$TEST_EXIT_CODE"
      exit \$TEST_EXIT_CODE
    env:
    - name: DEEPLAKE_API_URL
      value: "http://deeplake-api:8000"
    - name: AUDIMODAL_URL
      value: "http://audimodal:8080"
    - name: OPENAI_API_KEY
      value: "$OPENAI_API_KEY"
    - name: DEEPLAKE_API_KEY
      value: "$DEEPLAKE_API_KEY"
EOF

if [ $? -ne 0 ]; then
    log_error "Failed to create test pod"
    exit 1
fi

log_success "Test pod created"

# Step 8: Wait for pod to be running
log_info "Waiting for test pod to be running..."
kubectl wait --for=condition=Ready pod/"$TEST_POD_NAME" -n "$NAMESPACE" --timeout=120s

if [ $? -ne 0 ]; then
    log_error "Test pod failed to become ready"
    kubectl describe pod "$TEST_POD_NAME" -n "$NAMESPACE"
    exit 1
fi

log_success "Test pod is ready"

# Step 9: Create workspace directory and copy full audimodal codebase
log_info "Creating workspace directory..."
kubectl exec "$TEST_POD_NAME" -n "$NAMESPACE" -- mkdir -p /workspace

log_info "Copying audimodal codebase to pod (this may take a moment)..."
# Copy all required directories
kubectl cp "$AUDIMODAL_DIR/tests" "$NAMESPACE/$TEST_POD_NAME:/workspace/tests"
kubectl cp "$AUDIMODAL_DIR/pkg" "$NAMESPACE/$TEST_POD_NAME:/workspace/pkg"
kubectl cp "$AUDIMODAL_DIR/internal" "$NAMESPACE/$TEST_POD_NAME:/workspace/internal" 2>/dev/null || true
kubectl cp "$AUDIMODAL_DIR/go.mod" "$NAMESPACE/$TEST_POD_NAME:/workspace/go.mod"
kubectl cp "$AUDIMODAL_DIR/go.sum" "$NAMESPACE/$TEST_POD_NAME:/workspace/go.sum"

if [ $? -ne 0 ]; then
    log_error "Failed to copy audimodal codebase"
    exit 1
fi

# Signal that files are ready
kubectl exec "$TEST_POD_NAME" -n "$NAMESPACE" -- touch /workspace/tests-ready

log_success "Audimodal codebase copied successfully"

# Step 10: Stream logs
echo ""
echo "═══════════════════════════════════════════════════════════════"
echo " Test Output"
echo "═══════════════════════════════════════════════════════════════"
echo ""

kubectl logs -f "$TEST_POD_NAME" -n "$NAMESPACE"

# Step 11: Get exit code
echo ""
echo "═══════════════════════════════════════════════════════════════"
echo " Test Results"
echo "═══════════════════════════════════════════════════════════════"
echo ""

POD_STATUS=$(kubectl get pod "$TEST_POD_NAME" -n "$NAMESPACE" -o jsonpath='{.status.phase}')
EXIT_CODE=$(kubectl get pod "$TEST_POD_NAME" -n "$NAMESPACE" -o jsonpath='{.status.containerStatuses[0].state.terminated.exitCode}')

log_info "Pod Status: $POD_STATUS"
log_info "Exit Code: $EXIT_CODE"

if [ "$EXIT_CODE" = "0" ]; then
    log_success "All tests passed!"
    exit 0
else
    log_error "Tests failed with exit code: $EXIT_CODE"
    exit "$EXIT_CODE"
fi
