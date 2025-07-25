# AudiModal.ai

AudiModal.ai is a multi-tenant, cloud-native SaaS platform for secure, AI-driven document processing at enterprise scale. Built for regulated industries, it unifies multimodal data ingestion, compliance automation, and real-time streaming into a single, Kubernetes-powered architecture.

With built-in support for GDPR, HIPAA, SOX, and PCI DSS, AudiModal.ai delivers auditable document workflows, zero-trust security, and full data sovereignty—while supporting massive scale across thousands of users and millions of documents.


## 🚀 Quick Start

### Prerequisites

- Go 1.21 or later
- Docker and Docker Compose
- Kubernetes cluster (for production deployment)
- Make

### Environment Setup

#### 1. Install Go (if not already installed)

```bash
# Run the Go installation script
chmod +x scripts/install-go.sh
./scripts/install-go.sh
```

#### 2. Setup Development Environment

```bash
# Install all development tools and dependencies
chmod +x scripts/setup-dev.sh
./scripts/setup-dev.sh
```

This script will install:
- Go development tools (controller-gen, mockgen, ginkgo, etc.)
- Kubernetes tools (kubectl, kustomize, kind, helm)
- Linting and formatting tools (golangci-lint)
- Additional utilities (jq, yq, docker-compose)

#### 3. Initialize Project

```bash
# Download Go dependencies
go mod tidy

# Generate code and manifests
make generate
make manifests

# Run tests
make test
```

## 🏗️ Project Structure

```
├── api/v1/                     # API definitions and CRDs
├── cmd/                        # Application entry points
│   ├── cli/                    # CLI interface
│   ├── controller/             # Kubernetes controller
│   ├── file-discovery/         # File discovery service
│   ├── processor/              # Document processor
│   └── server/                 # Main server
├── config/                     # Kubernetes configuration
├── controllers/                # Kubernetes controllers
├── deployments/                # Deployment configurations
│   ├── docker-compose/         # Docker Compose setup
│   ├── helm/                   # Helm charts
│   ├── kubernetes/             # Kubernetes manifests
│   └── terraform/              # Infrastructure as Code
├── docs/                       # Documentation
├── examples/                   # Usage examples
├── internal/                   # Private application code
├── pkg/                        # Public packages
├── scripts/                    # Build and setup scripts
└── test/                       # Test files and fixtures
```

## 🛠️ Development

### Build

```bash
# Build all binaries
make build

# Build Docker images
make docker-build
```

### Testing

```bash
# Run unit tests
make test

# Run integration tests
make test-integration

# Run all tests with coverage
./scripts/run-tests.sh
```

### Code Quality

```bash
# Format code
make fmt

# Run linter
make lint

# Vet code
make vet
```

### Development Workflow

1. **Make changes** to the code
2. **Generate manifests** if you modified CRDs:
   ```bash
   make generate
   make manifests
   ```
3. **Run tests**:
   ```bash
   make test
   ```
4. **Check code quality**:
   ```bash
   make lint
   make vet
   ```

## 🚢 Deployment

### Local Development

```bash
# Setup local Kubernetes cluster with Kind
./scripts/deploy-local.sh
```

### Docker Compose

```bash
# Start all services
cd deployments/docker-compose
docker-compose up -d
```

### Kubernetes

```bash
# Install CRDs
make install

# Deploy controller
make deploy
```

### Production Deployment

See the deployment guides in `docs/deployment/` for:
- AWS deployment with Terraform
- Azure deployment with Terraform
- GCP deployment with Terraform
- Helm chart installation

## 📖 Available Make Targets

| Target | Description |
|--------|-------------|
| `help` | Display help information |
| `build` | Build manager binary |
| `test` | Run tests |
| `test-integration` | Run integration tests |
| `fmt` | Run go fmt against code |
| `vet` | Run go vet against code |
| `lint` | Run golangci-lint |
| `generate` | Generate code containing DeepCopy methods |
| `manifests` | Generate CRD manifests |
| `docker-build` | Build docker images |
| `docker-push` | Push docker images |
| `install` | Install CRDs into K8s cluster |
| `deploy` | Deploy controller to K8s cluster |

## 🧰 Development Tools

The setup script installs these essential tools:

- **controller-gen**: Generate Kubernetes manifests
- **kustomize**: Kubernetes configuration management
- **kubectl**: Kubernetes CLI
- **kind**: Local Kubernetes clusters
- **helm**: Kubernetes package manager
- **golangci-lint**: Go linter
- **mockgen**: Generate mocks for testing
- **ginkgo**: BDD testing framework
- **docker-compose**: Multi-container Docker applications

## 🔧 Environment Variables

Key environment variables for development:

```bash
# Go environment
export GOPATH=$HOME/go
export GOBIN=$GOPATH/bin
export PATH=$GOBIN:$PATH

# Docker images
export IMG=document-processing-platform:latest
export CONTROLLER_IMG=document-processing-controller:latest

# Registry for pushing images
export REGISTRY=localhost:5000
export TAG=latest
```

## 📦 Dependencies

This project uses Go modules. Key dependencies include:

- **Kubernetes**: API machinery and client libraries
- **Controller Runtime**: Kubernetes controller framework
- **Confluent Kafka**: Event streaming
- **OpenTelemetry**: Observability and tracing
- **Prometheus**: Metrics collection
- **Vault**: Secrets management
- **AWS/Azure/GCP SDKs**: Cloud storage integration

## 🧪 Testing

### Unit Tests

```bash
# Run all unit tests
go test ./pkg/... ./internal/... -race -coverprofile=coverage.out

# Generate coverage report
go tool cover -html=coverage.out -o coverage.html
```

### Integration Tests

```bash
# Run integration tests
go test ./test/integration/... -tags=integration
```

### Test Scripts

```bash
# Run comprehensive test suite
./scripts/run-tests.sh
```

## 📋 Development Scripts

Located in the `scripts/` directory:

- `install-go.sh`: Install latest Go version
- `setup-dev.sh`: Setup complete development environment
- `run-tests.sh`: Run comprehensive test suite
- `generate-crds.sh`: Generate and validate CRDs
- `build-images.sh`: Build and optionally push Docker images
- `deploy-local.sh`: Setup local development cluster

## 🔍 Troubleshooting

### Common Issues

1. **Go not found**: Restart your terminal after running setup scripts
2. **Permission denied**: Ensure scripts are executable (`chmod +x scripts/*.sh`)
3. **Docker issues**: Make sure Docker is running and you have permissions
4. **Kubernetes issues**: Verify kubectl is configured and cluster is accessible

### Getting Help

- Check the documentation in `docs/`
- Review examples in `examples/`
- Run `make help` for available targets
- Check the troubleshooting guide in `docs/deployment/`

## 📚 Documentation

Comprehensive documentation is available in the `docs/` directory:

- Architecture and design decisions
- API documentation
- Deployment guides
- Development workflows
- Plugin development guides

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests and linting
5. Submit a pull request

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

---

*This README was generated to help you get started quickly with the AudiModal platform. For detailed documentation, please refer to the `docs/` directory.*
