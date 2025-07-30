# AudiModal Kubernetes Custom Resource Definitions (CRDs)

This directory contains the Kubernetes Custom Resource Definitions (CRDs) and supporting resources for the AudiModal platform, enabling cloud-native deployment and management of multi-tenant document processing and AI workloads.

## 📋 Overview

The AudiModal Kubernetes operator provides cloud-native management for:
- **Multi-tenant isolation** with namespace-based separation
- **Data source lifecycle management** with automated synchronization
- **Policy-as-code** for Data Loss Prevention (DLP) and compliance
- **Processing workflow orchestration** with configurable pipelines
- **RBAC and security policies** for enterprise deployments

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                   AudiModal Platform                        │
├─────────────────────────────────────────────────────────────┤
│  Tenant CRD    │  DataSource CRD  │  DLP Policy CRD         │
│  Multi-tenant  │  Source Mgmt     │  Compliance Rules       │
│  Isolation     │  Sync Workflows  │  Security Scanning      │
├─────────────────────────────────────────────────────────────┤
│              Processing Session CRD                         │
│         Workflow Orchestration & Pipeline Mgmt             │
├─────────────────────────────────────────────────────────────┤
│                AudiModal Operator                          │
│           Controllers & Admission Webhooks                 │
├─────────────────────────────────────────────────────────────┤
│                  Kubernetes Platform                       │
│            Namespaces │ RBAC │ Storage │ Network           │
└─────────────────────────────────────────────────────────────┘
```

## 📦 Components

### Custom Resource Definitions

| CRD | Purpose | Scope |
|-----|---------|-------|
| **Tenant** | Multi-tenant resource management and isolation | Namespaced |
| **DataSource** | Data source lifecycle and synchronization | Namespaced |
| **DLPPolicy** | Data Loss Prevention and compliance policies | Namespaced |
| **ProcessingSession** | Document processing workflow management | Namespaced |

### Supporting Resources

- **RBAC**: Cluster roles and bindings for operator and tenant management
- **Security Policies**: Network policies, admission webhooks, and security configurations
- **Installation Scripts**: Automated deployment and management tools

## 🚀 Quick Start

### Prerequisites

- Kubernetes cluster (v1.20+)
- `kubectl` configured with cluster-admin access
- Optional: Helm 3.x for advanced deployments

### Installation

1. **Install CRDs and base resources:**
   ```bash
   ./install.sh install
   ```

2. **Verify installation:**
   ```bash
   ./install.sh verify
   ```

3. **Create sample resources:**
   ```bash
   ./install.sh samples
   ```

### Deploy the Operator

```bash
# Build and deploy the operator
docker build -t audimodal/operator:latest -f deployments/Dockerfile .
kubectl apply -f deployments/kubernetes/operator/
```

## 📖 Usage Examples

### Creating a Tenant

```yaml
apiVersion: audimodal.ai/v1
kind: Tenant
metadata:
  name: acme-corp
  namespace: audimodal-system
spec:
  name: "ACME Corporation"
  slug: "acme-corp"
  plan: "enterprise"
  settings:
    maxStorageGB: 1000
    maxUsers: 100
    maxDataSources: 50
    enableDLP: true
    enableAdvancedAnalytics: true
  resources:
    cpu: "8"
    memory: "16Gi"
    storage: "500Gi"
    gpuCount: 2
  security:
    isolationLevel: "namespace"
    encryptionAtRest: true
    encryptionInTransit: true
    networkPolicies: true
    allowedIPs:
    - "10.0.0.0/8"
    - "192.168.1.0/24"
```

### Configuring a Data Source

```yaml
apiVersion: audimodal.ai/v1
kind: DataSource
metadata:
  name: sharepoint-documents
  namespace: audimodal-acme-corp
spec:
  name: "SharePoint Document Library"
  type: "sharepoint"
  enabled: true
  config:
    siteUrl: "/sites/documents"
    tenantId: "12345678-1234-1234-1234-123456789abc"
    libraryNames:
    - "Documents"
    - "Shared Documents"
    credentials:
      secretRef:
        name: sharepoint-creds
        namespace: audimodal-acme-corp
    maxFileSize: "100MB"
    supportedExtensions:
    - ".pdf"
    - ".docx"
    - ".pptx"
    - ".xlsx"
  sync:
    mode: "scheduled"
    schedule: "0 */4 * * *"  # Every 4 hours
    batchSize: 100
    parallelWorkers: 10
    retryPolicy:
      maxRetries: 3
      backoffStrategy: "exponential"
  processing:
    chunkingStrategy: "semantic"
    chunkSize: 1000
    chunkOverlap: 100
    enableOCR: true
    enableAnalysis: true
    enableDLP: true
```

### Defining a DLP Policy

```yaml
apiVersion: audimodal.ai/v1
kind: DLPPolicy
metadata:
  name: pii-detection
  namespace: audimodal-acme-corp
spec:
  name: "PII Detection Policy"
  description: "Detect and protect personally identifiable information"
  enabled: true
  priority: 100
  scope:
    fileTypes: ["*"]
  rules:
  - name: "SSN Detection"
    type: "data_type_detector"
    severity: "critical"
    action: "redact"
    config:
      dataTypes: ["ssn"]
      confidenceThreshold: 0.9
      redactionStyle: "asterisk"
  - name: "Credit Card Detection"
    type: "regex_pattern"
    severity: "high"
    action: "mask"
    config:
      pattern: "\\b(?:\\d{4}[\\s-]?){3}\\d{4}\\b"
      confidenceThreshold: 0.85
  actions:
    notifications:
      channels: ["email", "slack"]
    audit:
      enabled: true
      logLevel: "detailed"
  compliance:
    frameworks: ["gdpr", "pci_dss"]
    riskLevel: "high"
```

### Creating a Processing Session

```yaml
apiVersion: audimodal.ai/v1
kind: ProcessingSession
metadata:
  name: quarterly-report-analysis
  namespace: audimodal-acme-corp
spec:
  name: "Q4 2024 Report Analysis"
  dataSource:
    name: "sharepoint-documents"
    filters:
      paths: ["/Reports/2024/Q4"]
      fileTypes: [".pdf", ".docx"]
      maxFiles: 1000
  pipeline:
    strategy: "parallel"
    maxParallelism: 20
    stages:
    - name: "content_extraction"
      type: "content_extraction"
      enabled: true
      config:
        enableOCR: true
        ocrEngine: "textract"
    - name: "chunking"
      type: "chunking"
      enabled: true
      config:
        strategy: "semantic"
        chunkSize: 1500
        overlap: 150
    - name: "embedding_generation"
      type: "embedding_generation"
      enabled: true
      config:
        provider: "openai"
        model: "text-embedding-ada-002"
        batchSize: 50
    - name: "vector_storage"
      type: "vector_storage"
      enabled: true
      config:
        vectorStore: "deeplake"
        indexName: "quarterly-reports"
    - name: "dlp_scanning"
      type: "dlp_scanning"
      enabled: true
      config:
        policies: ["pii-detection"]
        scanMode: "non_blocking"
  schedule:
    mode: "immediate"
    priority: "high"
  resources:
    cpu: "4"
    memory: "8Gi"
    storage: "50Gi"
  monitoring:
    alerting:
      enabled: true
      onFailure: true
      onCompletion: true
```

## 🔧 Configuration

### Operator Configuration

The operator can be configured using environment variables or ConfigMaps:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: audimodal-operator-config
  namespace: audimodal-system
data:
  # Logging
  log-level: "info"
  enable-debug: "false"
  
  # Tracing
  enable-tracing: "true"
  tracing-endpoint: "http://jaeger:14268/api/traces"
  tracing-sample-rate: "1.0"
  
  # Resource limits
  default-cpu-limit: "2"
  default-memory-limit: "4Gi"
  default-storage-limit: "10Gi"
  
  # Tenant isolation
  enable-network-policies: "true"
  default-isolation-level: "namespace"
  
  # Processing
  max-parallel-sessions: "10"
  default-chunk-size: "1000"
  default-batch-size: "50"
```

### Security Configuration

Security policies can be customized:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: audimodal-security-config
  namespace: audimodal-system
data:
  # Enforcement levels: strict, moderate, permissive
  enforcement-level: "strict"
  
  # Container security
  allow-privileged: "false"
  require-non-root: "true"
  
  # Network security
  enable-network-policies: "true"
  require-tls: "true"
  
  # RBAC
  enable-rbac-enforcement: "true"
  require-service-accounts: "true"
```

## 🔒 Security Features

### Multi-Tenant Isolation

- **Namespace isolation**: Each tenant gets a dedicated namespace
- **RBAC enforcement**: Fine-grained permissions per tenant
- **Network policies**: Traffic isolation between tenants
- **Resource quotas**: CPU, memory, and storage limits per tenant

### Data Protection

- **Encryption at rest**: Configurable per tenant
- **Encryption in transit**: TLS enforcement for all communications
- **DLP integration**: Policy-based data loss prevention
- **Audit logging**: Comprehensive audit trails

### Access Control

- **Service accounts**: Dedicated service accounts per tenant
- **Role-based access**: Multiple permission levels (admin, user, viewer)
- **Admission webhooks**: Validation and mutation of resources
- **Security policies**: Pod security standards enforcement

## 📊 Monitoring and Observability

### Metrics

The operator exposes Prometheus metrics for monitoring:

- `audimodal_tenants_total`: Total number of tenants
- `audimodal_datasources_total`: Total number of data sources
- `audimodal_processing_sessions_active`: Active processing sessions
- `audimodal_dlp_violations_total`: DLP violations detected
- `audimodal_operator_reconcile_duration`: Controller reconciliation time

### Alerts

Pre-configured Prometheus alerts:

- Tenant quota violations
- DLP policy violations
- Processing session failures
- Operator health issues

### Tracing

OpenTelemetry integration for distributed tracing across:

- Controller operations
- Processing workflows
- External API calls
- Database operations

## 🛠️ Development

### Building the Operator

```bash
# Build locally
make build

# Build Docker image
make docker-build

# Deploy to cluster
make deploy
```

### Running Tests

```bash
# Unit tests
make test

# Integration tests
make test-integration

# End-to-end tests
make test-e2e
```

### Code Generation

```bash
# Generate CRD manifests
make manifests

# Generate Go code
make generate

# Generate documentation
make docs
```

## 📚 API Reference

### Tenant API

- **Spec**: Tenant configuration and requirements
- **Status**: Current state, resource usage, and health
- **Subresources**: Scale operations for tenant resources

### DataSource API

- **Spec**: Data source configuration and sync settings
- **Status**: Sync status, statistics, and health monitoring
- **Operations**: Manual sync triggers, credential rotation

### DLPPolicy API

- **Spec**: Policy rules, actions, and compliance mapping
- **Status**: Deployment status, violation statistics
- **Validation**: Rule syntax validation, policy conflicts

### ProcessingSession API

- **Spec**: Workflow definition and resource requirements
- **Status**: Execution progress, stage completion, metrics
- **Subresources**: Scale operations for parallel processing

## 🔧 Troubleshooting

### Common Issues

1. **CRD Installation Fails**
   ```bash
   # Check cluster permissions
   kubectl auth can-i create customresourcedefinitions
   
   # Verify cluster version
   kubectl version
   ```

2. **Tenant Provisioning Stuck**
   ```bash
   # Check operator logs
   kubectl logs -n audimodal-system deployment/audimodal-operator
   
   # Check tenant status
   kubectl get tenant <name> -o yaml
   ```

3. **DataSource Sync Issues**
   ```bash
   # Check credentials
   kubectl get secret <credentials-secret> -n <namespace>
   
   # Check datasource status
   kubectl describe datasource <name> -n <namespace>
   ```

### Debug Mode

Enable debug logging:

```bash
kubectl patch deployment audimodal-operator -n audimodal-system -p '{"spec":{"template":{"spec":{"containers":[{"name":"manager","env":[{"name":"LOG_LEVEL","value":"debug"}]}]}}}}'
```

### Health Checks

```bash
# Check operator health
kubectl get deployment audimodal-operator -n audimodal-system

# Check CRD status
kubectl get crds | grep audimodal

# Verify RBAC
kubectl auth can-i --list --as=system:serviceaccount:audimodal-system:audimodal-operator
```

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](../../../LICENSE) file for details.

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/audimodal/platform/issues)
- **Documentation**: [docs.audimodal.ai](https://docs.audimodal.ai)
- **Community**: [Discord](https://discord.gg/audimodal)
- **Email**: support@audimodal.ai