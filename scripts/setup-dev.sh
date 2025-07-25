#!/bin/bash

# scripts/setup-dev.sh
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Tool versions
CONTROLLER_GEN_VERSION="v0.13.0"
KUSTOMIZE_VERSION="v5.2.1"
GOLANGCI_LINT_VERSION="v1.54.2"
MOCKGEN_VERSION="v0.3.0"
GINKGO_VERSION="v2.13.0"
HELM_VERSION="v3.13.1"
KIND_VERSION="v0.20.0"
KUBECTL_VERSION="v1.28.0"
DOCKER_COMPOSE_VERSION="v2.21.0"
PROTOC_GEN_GO_VERSION="v1.31.0"
PROTOC_GEN_GO_GRPC_VERSION="v1.3.0"
WIRE_VERSION="v0.5.0"
MIGRATE_VERSION="v4.16.2"
GORELEASER_VERSION="v1.21.2"

# Detect OS and Architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case $ARCH in
    x86_64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) echo -e "${RED}Unsupported architecture: $ARCH${NC}"; exit 1 ;;
esac

echo -e "${BLUE}Setting up development environment for $OS/$ARCH${NC}"

# Create bin directory if it doesn't exist
BIN_DIR="${HOME}/.local/bin"
mkdir -p "$BIN_DIR"

# Add to PATH if not already there
if [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
    echo "export PATH=\"$BIN_DIR:\$PATH\"" >> ~/.bashrc
    echo "export PATH=\"$BIN_DIR:\$PATH\"" >> ~/.zshrc 2>/dev/null || true
    export PATH="$BIN_DIR:$PATH"
fi

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to download and install binary
install_binary() {
    local name=$1
    local version=$2
    local url=$3
    local binary_name=${4:-$name}
    
    echo -e "${YELLOW}Installing $name $version...${NC}"
    
    if command_exists "$binary_name" && [[ "$($binary_name version 2>/dev/null | head -1)" == *"$version"* ]]; then
        echo -e "${GREEN}$name $version is already installed${NC}"
        return
    fi
    
    local temp_dir=$(mktemp -d)
    cd "$temp_dir"
    
    if [[ "$url" == *.tar.gz ]]; then
        curl -sL "$url" | tar xz
        find . -name "$binary_name" -type f -executable | head -1 | xargs -I {} mv {} "$BIN_DIR/"
    elif [[ "$url" == *.zip ]]; then
        curl -sL "$url" -o package.zip
        unzip -q package.zip
        find . -name "$binary_name" -type f -executable | head -1 | xargs -I {} mv {} "$BIN_DIR/"
    else
        curl -sL "$url" -o "$binary_name"
        chmod +x "$binary_name"
        mv "$binary_name" "$BIN_DIR/"
    fi
    
    cd - >/dev/null
    rm -rf "$temp_dir"
    
    echo -e "${GREEN}$name installed successfully${NC}"
}

# Function to install Go tools
install_go_tool() {
    local name=$1
    local package=$2
    local version=${3:-latest}
    
    echo -e "${YELLOW}Installing Go tool: $name...${NC}"
    
    if [[ "$version" == "latest" ]]; then
        go install "$package@latest"
    else
        go install "$package@$version"
    fi
    
    echo -e "${GREEN}$name installed successfully${NC}"
}

# Check if Go is installed
if ! command_exists go; then
    echo -e "${RED}Go is not installed. Please install Go first.${NC}"
    echo -e "${BLUE}Visit: https://golang.org/doc/install${NC}"
    exit 1
fi

echo -e "${GREEN}Go $(go version | cut -d' ' -f3) detected${NC}"

# Install Go development tools
echo -e "\n${BLUE}Installing Go development tools...${NC}"

install_go_tool "controller-gen" "sigs.k8s.io/controller-tools/cmd/controller-gen" "$CONTROLLER_GEN_VERSION"
install_go_tool "mockgen" "go.uber.org/mock/mockgen" "$MOCKGEN_VERSION"
install_go_tool "ginkgo" "github.com/onsi/ginkgo/v2/ginkgo" "$GINKGO_VERSION"
install_go_tool "wire" "github.com/google/wire/cmd/wire" "$WIRE_VERSION"
install_go_tool "migrate" "github.com/golang-migrate/migrate/v4/cmd/migrate" "$MIGRATE_VERSION"
install_go_tool "protoc-gen-go" "google.golang.org/protobuf/cmd/protoc-gen-go" "$PROTOC_GEN_GO_VERSION"
install_go_tool "protoc-gen-go-grpc" "google.golang.org/grpc/cmd/protoc-gen-go-grpc" "$PROTOC_GEN_GO_GRPC_VERSION"

# Install golangci-lint
if ! command_exists golangci-lint; then
    echo -e "${YELLOW}Installing golangci-lint...${NC}"
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b "$BIN_DIR" "$GOLANGCI_LINT_VERSION"
    echo -e "${GREEN}golangci-lint installed successfully${NC}"
else
    echo -e "${GREEN}golangci-lint is already installed${NC}"
fi

# Install Kubernetes tools
echo -e "\n${BLUE}Installing Kubernetes tools...${NC}"

# kubectl
if ! command_exists kubectl; then
    install_binary "kubectl" "$KUBECTL_VERSION" \
        "https://dl.k8s.io/release/$KUBECTL_VERSION/bin/$OS/$ARCH/kubectl"
else
    echo -e "${GREEN}kubectl is already installed${NC}"
fi

# kustomize
if ! command_exists kustomize; then
    install_binary "kustomize" "$KUSTOMIZE_VERSION" \
        "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2F$KUSTOMIZE_VERSION/kustomize_${KUSTOMIZE_VERSION}_${OS}_${ARCH}.tar.gz"
else
    echo -e "${GREEN}kustomize is already installed${NC}"
fi

# kind
if ! command_exists kind; then
    install_binary "kind" "$KIND_VERSION" \
        "https://kind.sigs.k8s.io/dl/$KIND_VERSION/kind-$OS-$ARCH"
else
    echo -e "${GREEN}kind is already installed${NC}"
fi

# helm
if ! command_exists helm; then
    install_binary "helm" "$HELM_VERSION" \
        "https://get.helm.sh/helm-$HELM_VERSION-$OS-$ARCH.tar.gz" \
        "helm"
else
    echo -e "${GREEN}helm is already installed${NC}"
fi

# Install Docker Compose (if Docker is available)
if command_exists docker && ! command_exists docker-compose; then
    echo -e "${YELLOW}Installing docker-compose...${NC}"
    install_binary "docker-compose" "$DOCKER_COMPOSE_VERSION" \
        "https://github.com/docker/compose/releases/download/$DOCKER_COMPOSE_VERSION/docker-compose-$OS-$ARCH"
else
    echo -e "${GREEN}docker-compose is already installed or Docker not available${NC}"
fi

# Install goreleaser
if ! command_exists goreleaser; then
    install_binary "goreleaser" "$GORELEASER_VERSION" \
        "https://github.com/goreleaser/goreleaser/releases/download/$GORELEASER_VERSION/goreleaser_${OS}_${ARCH}.tar.gz"
else
    echo -e "${GREEN}goreleaser is already installed${NC}"
fi

# Install additional useful tools
echo -e "\n${BLUE}Installing additional development tools...${NC}"

# jq for JSON processing
if ! command_exists jq; then
    case $OS in
        linux)
            install_binary "jq" "1.6" \
                "https://github.com/stedolan/jq/releases/download/jq-1.6/jq-linux64" \
                "jq"
            ;;
        darwin)
            install_binary "jq" "1.6" \
                "https://github.com/stedolan/jq/releases/download/jq-1.6/jq-osx-amd64" \
                "jq"
            ;;
    esac
else
    echo -e "${GREEN}jq is already installed${NC}"
fi

# yq for YAML processing
if ! command_exists yq; then
    install_binary "yq" "v4.35.2" \
        "https://github.com/mikefarah/yq/releases/download/v4.35.2/yq_${OS}_${ARCH}.tar.gz"
else
    echo -e "${GREEN}yq is already installed${NC}"
fi

# Install pre-commit hooks
if command_exists python3 && command_exists pip3; then
    echo -e "${YELLOW}Installing pre-commit...${NC}"
    pip3 install --user pre-commit
    echo -e "${GREEN}pre-commit installed successfully${NC}"
fi

# Create useful aliases
echo -e "\n${BLUE}Creating useful development aliases...${NC}"
cat >> ~/.bash_aliases << 'EOF'
# Document Processing Platform aliases
alias k='kubectl'
alias kgp='kubectl get pods'
alias kgs='kubectl get svc'
alias kgd='kubectl get deploy'
alias kdp='kubectl describe pod'
alias kl='kubectl logs'
alias kex='kubectl exec -it'
alias kcf='kubectl create -f'
alias kaf='kubectl apply -f'
alias kdf='kubectl delete -f'

# Make shortcuts
alias build='make build'
alias test='make test'
alias lint='make lint'
alias deploy='make deploy'
alias gen='make generate'
alias man='make manifests'

# Docker shortcuts
alias dc='docker-compose'
alias dcu='docker-compose up'
alias dcd='docker-compose down'
alias dcb='docker-compose build'
alias dcl='docker-compose logs'

# Git shortcuts for this project
alias gs='git status'
alias ga='git add'
alias gc='git commit'
alias gp='git push'
alias gl='git pull'
alias gco='git checkout'
alias gb='git branch'
EOF

# Create development configuration files
echo -e "\n${BLUE}Creating development configuration files...${NC}"

# .golangci.yml
cat > .golangci.yml << 'EOF'
run:
  timeout: 5m
  modules-download-mode: readonly

linters-settings:
  govet:
    check-shadowing: true
  golint:
    min-confidence: 0
  gocyclo:
    min-complexity: 15
  maligned:
    suggest-new: true
  dupl:
    threshold: 100
  goconst:
    min-len: 2
    min-occurrences: 2
  misspell:
    locale: US
  lll:
    line-length: 140
  goimports:
    local-prefixes: github.com/yourorg/document-processing-platform
  gocritic:
    enabled-tags:
      - diagnostic
      - experimental
      - opinionated
      - performance
      - style

linters:
  enable:
    - bodyclose
    - deadcode
    - depguard
    - dogsled
    - dupl
    - errcheck
    - exportloopref
    - exhaustive
    - gochecknoinits
    - goconst
    - gocritic
    - gocyclo
    - gofmt
    - goimports
    - golint
    - gomnd
    - goprintffuncname
    - gosec
    - gosimple
    - govet
    - ineffassign
    - lll
    - misspell
    - nakedret
    - noctx
    - nolintlint
    - rowserrcheck
    - staticcheck
    - structcheck
    - stylecheck
    - typecheck
    - unconvert
    - unparam
    - unused
    - varcheck
    - whitespace

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - gomnd
        - gocyclo
        - errcheck
        - dupl
        - gosec
    - path: zz_generated\.deepcopy\.go
      linters:
        - lll
        - gofmt
EOF

# .pre-commit-config.yaml
cat > .pre-commit-config.yaml << 'EOF'
repos:
  - repo: https://github.com/pre-commit/pre-commit-hooks
    rev: v4.4.0
    hooks:
      - id: trailing-whitespace
      - id: end-of-file-fixer
      - id: check-yaml
      - id: check-added-large-files
      - id: check-merge-conflict
  
  - repo: local
    hooks:
      - id: go-fmt
        name: go fmt
        entry: gofmt
        language: system
        args: [-w]
        files: \.go$
      
      - id: go-vet
        name: go vet
        entry: go vet
        language: system
        files: \.go$
        pass_filenames: false
      
      - id: golangci-lint
        name: golangci-lint
        entry: golangci-lint run
        language: system
        files: \.go$
        pass_filenames: false
      
      - id: go-test
        name: go test
        entry: go test
        language: system
        args: [./...]
        files: \.go$
        pass_filenames: false
EOF

# Create scripts directory files
cat > scripts/run-tests.sh << 'EOF'
#!/bin/bash
set -euo pipefail

echo "Running unit tests..."
go test ./pkg/... ./internal/... -race -coverprofile=coverage.out

echo "Running integration tests..."
go test ./test/integration/... -tags=integration

echo "Running controller tests..."
go test ./controllers/... -coverprofile=controller-coverage.out

echo "Generating coverage report..."
go tool cover -html=coverage.out -o coverage.html
echo "Coverage report generated: coverage.html"
EOF

cat > scripts/generate-crds.sh << 'EOF'
#!/bin/bash
set -euo pipefail

echo "Generating CRD manifests..."
controller-gen rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

echo "Generating deepcopy methods..."
controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./..."

echo "Validating generated manifests..."
kubectl apply --dry-run=client -f config/crd/bases/

echo "CRD generation complete!"
EOF

cat > scripts/build-images.sh << 'EOF'
#!/bin/bash
set -euo pipefail

REGISTRY=${REGISTRY:-"localhost:5000"}
TAG=${TAG:-"latest"}

echo "Building images with tag: $TAG"

# Build main application image
echo "Building main application image..."
docker build -t "${REGISTRY}/document-processing-platform:${TAG}" .

# Build controller image
echo "Building controller image..."
docker build -f Dockerfile.controller -t "${REGISTRY}/document-processing-controller:${TAG}" .

# Build processor image
echo "Building processor image..."
docker build -f Dockerfile.processor -t "${REGISTRY}/document-processing-processor:${TAG}" .

echo "All images built successfully!"

if [[ "${PUSH:-false}" == "true" ]]; then
    echo "Pushing images to registry..."
    docker push "${REGISTRY}/document-processing-platform:${TAG}"
    docker push "${REGISTRY}/document-processing-controller:${TAG}"
    docker push "${REGISTRY}/document-processing-processor:${TAG}"
    echo "Images pushed successfully!"
fi
EOF

cat > scripts/deploy-local.sh << 'EOF'
#!/bin/bash
set -euo pipefail

echo "Setting up local development environment..."

# Create kind cluster if it doesn't exist
if ! kind get clusters | grep -q "dp-dev"; then
    echo "Creating kind cluster..."
    cat <<EOF | kind create cluster --name dp-dev --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  image: kindest/node:v1.28.0
  extraPortMappings:
  - containerPort: 30080
    hostPort: 8080
  - containerPort: 30443
    hostPort: 8443
EOF
fi

# Install CRDs
echo "Installing CRDs..."
make install

# Deploy dependencies (Kafka, PostgreSQL, etc.)
echo "Deploying dependencies..."
kubectl apply -f deployments/kubernetes/

# Build and load images into kind
echo "Building and loading images..."
make docker-build
kind load docker-image document-processing-platform:latest --name dp-dev
kind load docker-image document-processing-controller:latest --name dp-dev

# Deploy the application
echo "Deploying application..."
make deploy

echo "Local deployment complete!"
echo "Application will be available at http://localhost:8080"
EOF

# Make scripts executable
chmod +x scripts/*.sh

# Install Go modules
echo -e "\n${BLUE}Initializing Go module...${NC}"
if [[ ! -f "go.mod" ]]; then
    go mod init github.com/yourorg/document-processing-platform
fi

# Create initial go.mod with dependencies
cat > go.mod << 'EOF'
module github.com/yourorg/document-processing-platform

go 1.21

require (
    // Kubernetes dependencies
    k8s.io/api v0.28.4
    k8s.io/apimachinery v0.28.4
    k8s.io/client-go v0.28.4
    sigs.k8s.io/controller-runtime v0.16.3
    sigs.k8s.io/controller-tools v0.13.0
    
    // Event streaming
    github.com/confluentinc/confluent-kafka-go/v2 v2.3.0
    
    // Observability
    go.opentelemetry.io/otel v1.21.0
    go.opentelemetry.io/otel/trace v1.21.0
    go.opentelemetry.io/otel/metric v1.21.0
    github.com/prometheus/client_golang v1.17.0
    
    // Security
    github.com/hashicorp/vault/api v1.10.0
    
    // Database
    github.com/lib/pq v1.10.9
    github.com/golang-migrate/migrate/v4 v4.16.2
    
    // Cloud SDKs
    github.com/aws/aws-sdk-go-v2 v1.21.0
    github.com/aws/aws-sdk-go-v2/service/s3 v1.40.0
    google.golang.org/api v0.150.0
    github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.2.0
    
    // Utilities
    github.com/google/uuid v1.3.0
    github.com/gin-gonic/gin v1.9.1
    github.com/spf13/cobra v1.8.0
    github.com/spf13/viper v1.17.0
    
    // Testing
    github.com/stretchr/testify v1.8.4
    github.com/onsi/ginkgo/v2 v2.13.0
    github.com/onsi/gomega v1.29.0
    go.uber.org/mock v0.3.0
)
EOF

# Download dependencies
echo -e "${YELLOW}Downloading Go dependencies...${NC}"
go mod tidy

# Setup pre-commit hooks
if command_exists pre-commit; then
    echo -e "${YELLOW}Installing pre-commit hooks...${NC}"
    pre-commit install
fi

echo -e "\n${GREEN}Development environment setup complete!${NC}"
echo -e "${BLUE}Tools installed in: $BIN_DIR${NC}"
echo -e "${BLUE}Make sure to add $BIN_DIR to your PATH${NC}"
echo -e "${YELLOW}Please restart your shell or run: source ~/.bashrc${NC}"

# Display installed versions
echo -e "\n${BLUE}Installed tool versions:${NC}"
echo "Go: $(go version | cut -d' ' -f3)"
command_exists kubectl && echo "kubectl: $(kubectl version --client --short 2>/dev/null | cut -d' ' -f3)"
command_exists kustomize && echo "kustomize: $(kustomize version --short 2>/dev/null)"
command_exists helm && echo "helm: $(helm version --short 2>/dev/null)"
command_exists kind && echo "kind: $(kind version 2>/dev/null)"
command_exists golangci-lint && echo "golangci-lint: $(golangci-lint version 2>/dev/null | head -1)"
command_exists controller-gen && echo "controller-gen: $(controller-gen --version 2>/dev/null)"

echo -e "\n${GREEN}Ready to start developing! 🚀${NC}"
echo -e "${BLUE}Next steps:${NC}"
echo -e "1. Run: ${YELLOW}make generate${NC} to generate code"
echo -e "2. Run: ${YELLOW}make manifests${NC} to generate CRDs"
echo -e "3. Run: ${YELLOW}make test${NC} to run tests"
echo -e "4. Run: ${YELLOW}scripts/deploy-local.sh${NC} to set up local environment"
