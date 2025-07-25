# AudiModal Deployment Guide

This directory contains deployment configurations and scripts for the AudiModal platform. The platform supports multiple deployment methods and environments.

## Table of Contents

- [Quick Start](#quick-start)
- [Deployment Methods](#deployment-methods)
- [Environment Configuration](#environment-configuration)
- [Prerequisites](#prerequisites)
- [Deployment Scripts](#deployment-scripts)
- [Monitoring](#monitoring)
- [Security](#security)
- [Troubleshooting](#troubleshooting)

## Quick Start

### Development Environment

1. **Docker Compose (Recommended for development):**
   ```bash
   # Start development environment with hot reload
   docker-compose -f docker-compose.dev.yml up -d
   
   # View logs
   docker-compose -f docker-compose.dev.yml logs -f audimodal-dev
   
   # Stop environment
   docker-compose -f docker-compose.dev.yml down
   ```

2. **Access development services:**
   - Application: http://localhost:8080
   - Grafana: http://localhost:3000 (admin/admin123)
   - Prometheus: http://localhost:9090
   - pgAdmin: http://localhost:5050 (admin@audimodal.dev/admin123)
   - Redis Commander: http://localhost:8081
   - MinIO Console: http://localhost:9001 (minioadmin/minioadmin123)
   - Jaeger: http://localhost:16686
   - MailHog: http://localhost:8025

### Production Environment

1. **Kubernetes with Helm:**
   ```bash
   # Deploy to production
   ./scripts/deployment/deploy.sh --environment production --tag v1.0.0
   ```

## Deployment Methods

### 1. Docker Compose

Three Docker Compose configurations are available:

- **`docker-compose.yml`**: Basic multi-service setup
- **`docker-compose.dev.yml`**: Development environment with debugging tools
- **`docker-compose.prod.yml`**: Production-ready configuration with scaling

### 2. Kubernetes

Raw Kubernetes manifests are provided in `kubernetes/`:

```bash
# Apply all Kubernetes resources
kubectl apply -f deployments/kubernetes/namespace.yaml
kubectl apply -f deployments/kubernetes/secrets.yaml
kubectl apply -f deployments/kubernetes/configmap.yaml
kubectl apply -f deployments/kubernetes/rbac.yaml
kubectl apply -f deployments/kubernetes/pvc.yaml
kubectl apply -f deployments/kubernetes/statefulsets.yaml
kubectl apply -f deployments/kubernetes/deployment.yaml
kubectl apply -f deployments/kubernetes/services.yaml
```

### 3. Helm Charts

The Helm chart provides the most flexible deployment option:

```bash
# Add required repositories
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update

# Install with default values
helm install audimodal ./deployments/helm/audimodal

# Install with environment-specific values
helm install audimodal ./deployments/helm/audimodal \
  -f ./deployments/helm/audimodal/values-production.yaml

# Upgrade deployment
helm upgrade audimodal ./deployments/helm/audimodal \
  --set app.image.tag=v1.2.0
```

## Environment Configuration

### Development
- Single replica
- Debug logging
- Hot reload enabled
- All monitoring tools included
- Insecure secrets (for convenience)

### Staging
- 2 replicas
- Info-level logging
- Full monitoring
- SSL with staging certificates
- Moderate resource allocation

### Production
- 3+ replicas with autoscaling
- Warn-level logging
- Production SSL certificates
- External secret management
- High resource allocation
- Network policies enabled

## Prerequisites

### For Docker Compose
- Docker 20.10+
- Docker Compose 2.0+
- 4GB+ available RAM
- 10GB+ available disk space

### For Kubernetes
- Kubernetes 1.24+
- kubectl configured for your cluster
- Helm 3.8+
- cert-manager (for TLS)
- nginx-ingress-controller
- 8GB+ available cluster RAM
- 50GB+ available cluster storage

### Required Secrets (Production)

Create these secrets before deploying to production:

```bash
# Application secrets
kubectl create secret generic audimodal-secrets \
  --from-literal=jwt-secret="your-jwt-secret" \
  --from-literal=encryption-key="your-32-byte-key" \
  --namespace audimodal

# Database secrets
kubectl create secret generic postgres-secrets \
  --from-literal=postgres-password="your-db-password" \
  --namespace audimodal

# Redis secrets
kubectl create secret generic redis-secrets \
  --from-literal=redis-password="your-redis-password" \
  --namespace audimodal
```

## Deployment Scripts

### Main Deployment Script

The `scripts/deployment/deploy.sh` script provides a unified interface for deployments:

```bash
# Usage examples
./scripts/deployment/deploy.sh -e dev -t latest
./scripts/deployment/deploy.sh -e staging -t v1.0.0 --dry-run
./scripts/deployment/deploy.sh -e prod -n production -r registry.company.com -t v1.2.3
```

**Options:**
- `-e, --environment`: Target environment (dev, staging, prod)
- `-n, --namespace`: Kubernetes namespace
- `-r, --registry`: Docker registry
- `-t, --tag`: Image tag
- `-c, --config`: Additional config file
- `-d, --dry-run`: Perform dry run
- `-v, --verbose`: Verbose output

### CI/CD Pipeline

The GitHub Actions workflow (`.github/workflows/ci-cd.yml`) provides:

- **Continuous Integration:**
  - Go lint and security scanning
  - Unit and integration tests
  - Code coverage reporting
  - Docker image building

- **Continuous Deployment:**
  - Automatic staging deployment on `develop` branch
  - Production deployment on version tags
  - Smoke tests after deployment
  - GitHub release creation

## Monitoring

### Metrics Collection

- **Prometheus**: Metrics collection and alerting
- **Grafana**: Visualization and dashboards
- **Jaeger**: Distributed tracing
- **Application metrics**: Custom business metrics

### Key Metrics

- Request rate and latency
- Error rates by endpoint
- Resource utilization (CPU, memory)
- Database connection pool status
- Queue depth and processing times
- Classification accuracy and performance

### Dashboards

Pre-configured Grafana dashboards include:

- Application Overview
- Performance Metrics
- Infrastructure Health
- Business KPIs
- Error Tracking

## Security

### Container Security

- Non-root user execution
- Read-only root filesystem
- Dropped capabilities
- Security scanning with Trivy

### Network Security

- Network policies for pod-to-pod communication
- TLS termination at ingress
- Internal service communication encryption

### Secret Management

- External secret management recommended
- Kubernetes secrets for development
- Integration with cloud provider secret stores

### RBAC

- Minimal required permissions
- Service account isolation
- Pod security policies

## Configuration Management

### Environment Variables

Key configuration options:

```bash
# Core application
AUDIMODAL_ENV=production
AUDIMODAL_LOG_LEVEL=info
AUDIMODAL_CONFIG_PATH=/app/config/app.yaml

# Database
AUDIMODAL_DB_HOST=postgres-service
AUDIMODAL_DB_PORT=5432
AUDIMODAL_DB_NAME=audimodal
AUDIMODAL_DB_USER=audiuser
AUDIMODAL_DB_PASSWORD=secret

# Redis
AUDIMODAL_REDIS_HOST=redis-service
AUDIMODAL_REDIS_PORT=6379
AUDIMODAL_REDIS_PASSWORD=secret

# Authentication
AUDIMODAL_JWT_SECRET=your-jwt-secret
AUDIMODAL_ENCRYPTION_KEY=your-32-byte-key

# Cloud providers
AUDIMODAL_AWS_REGION=us-west-2
AUDIMODAL_AWS_ACCESS_KEY_ID=key
AUDIMODAL_AWS_SECRET_ACCESS_KEY=secret

# Monitoring
AUDIMODAL_METRICS_ENABLED=true
AUDIMODAL_TRACING_ENABLED=true
AUDIMODAL_JAEGER_ENDPOINT=http://jaeger:14268/api/traces
```

### ConfigMaps

Application configuration is managed through Kubernetes ConfigMaps:

- `audimodal-config`: Main application configuration
- `nginx-config`: Nginx proxy configuration
- `prometheus-config`: Monitoring configuration

## Troubleshooting

### Common Issues

1. **Pods not starting:**
   ```bash
   kubectl describe pod <pod-name> -n audimodal
   kubectl logs <pod-name> -n audimodal
   ```

2. **Database connection issues:**
   ```bash
   kubectl exec -it <app-pod> -n audimodal -- sh
   pg_isready -h postgres-service -p 5432
   ```

3. **Redis connection issues:**
   ```bash
   kubectl exec -it <app-pod> -n audimodal -- sh
   redis-cli -h redis-service -p 6379 ping
   ```

4. **Storage issues:**
   ```bash
   kubectl get pvc -n audimodal
   kubectl describe pvc <pvc-name> -n audimodal
   ```

### Health Checks

Application provides several health endpoints:

- `/health`: Basic health check
- `/ready`: Readiness check (dependencies)
- `/metrics`: Prometheus metrics
- `/debug/pprof`: Go profiling (development only)

### Log Analysis

```bash
# Application logs
kubectl logs -f deployment/audimodal-app -n audimodal

# All pods logs
kubectl logs -f -l app.kubernetes.io/name=audimodal -n audimodal

# Specific container logs
kubectl logs -f <pod-name> -c <container-name> -n audimodal
```

### Performance Monitoring

Monitor key performance indicators:

1. **Response times**: < 200ms for 95th percentile
2. **Error rates**: < 0.1% for 4xx/5xx errors
3. **Throughput**: Target requests per second
4. **Resource usage**: CPU < 70%, Memory < 80%

## Backup and Recovery

### Database Backup

Automated backups are configured for production:

```bash
# Manual backup
kubectl exec -it postgres-0 -n audimodal -- \
  pg_dump -U audiuser audimodal > backup.sql

# Restore from backup
kubectl exec -i postgres-0 -n audimodal -- \
  psql -U audiuser audimodal < backup.sql
```

### Application Data Backup

Persistent volumes are backed up according to the backup schedule:

```bash
# List backup jobs
kubectl get cronjobs -n audimodal

# Check backup status
kubectl get jobs -n audimodal
```

## Scaling

### Horizontal Scaling

```bash
# Manual scaling
kubectl scale deployment audimodal-app --replicas=5 -n audimodal

# Autoscaling configuration
kubectl get hpa -n audimodal
```

### Vertical Scaling

Update resource requests/limits in Helm values:

```yaml
app:
  resources:
    limits:
      memory: "2Gi"
      cpu: "2000m"
    requests:
      memory: "1Gi"
      cpu: "1000m"
```

## Support

For deployment issues:

1. Check the logs and metrics
2. Review the troubleshooting section
3. Consult the application documentation
4. Contact the platform team

## Version History

- v1.0.0: Initial deployment configuration
- v1.1.0: Added Helm charts and improved monitoring
- v1.2.0: Enhanced security and production readiness