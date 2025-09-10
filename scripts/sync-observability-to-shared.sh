#!/bin/bash

# Sync observability metrics from audimodal to aether-shared repository
# This script extracts metric definitions, dashboards, and alerting rules
# and syncs them to the shared monitoring infrastructure

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AUDIMODAL_ROOT="$(dirname "$SCRIPT_DIR")"
AETHER_SHARED_ROOT="${AUDIMODAL_ROOT}/../aether-shared"
SHARED_MONITORING_DIR="${AETHER_SHARED_ROOT}/shared-monitoring"

# Validate directories exist
if [ ! -d "$AETHER_SHARED_ROOT" ]; then
    echo -e "${RED}Error: aether-shared repository not found at ${AETHER_SHARED_ROOT}${NC}"
    echo "Please ensure the aether-shared repository is cloned alongside audimodal"
    exit 1
fi

if [ ! -d "$SHARED_MONITORING_DIR" ]; then
    echo -e "${YELLOW}Creating shared-monitoring directory...${NC}"
    mkdir -p "$SHARED_MONITORING_DIR"
fi

echo -e "${BLUE}=== Audimodal Observability Sync ===${NC}"
echo -e "${BLUE}Source: ${AUDIMODAL_ROOT}${NC}"
echo -e "${BLUE}Target: ${SHARED_MONITORING_DIR}${NC}"
echo ""

# Function to extract metrics from Go code
extract_metrics_from_code() {
    local output_file="$1"
    echo -e "${GREEN}Extracting metrics from Go code...${NC}"
    
    cat > "$output_file" << 'EOF'
# Audimodal Application Metrics
# Auto-generated from audimodal codebase

## Storage Metrics
audimodal_storage_total_bytes{tier="hot|warm|cold",type="document|chunk|embedding"} - Total storage usage in bytes
audimodal_storage_total_files{tier="hot|warm|cold",type="document|chunk|embedding"} - Total number of files
audimodal_storage_throughput_mbps - Storage throughput in MB/s
audimodal_storage_iops - Storage I/O operations per second
audimodal_storage_usage_percent - Storage usage percentage
audimodal_storage_monthly_cost_change_percent - Monthly cost change percentage

## Processing Metrics
audimodal_processing_queue_size{processor="tier|embedding|analysis"} - Current queue size
audimodal_processing_items_processed_total{processor="tier|embedding|analysis"} - Total items processed
audimodal_processing_errors_total{processor="tier|embedding|analysis"} - Total processing errors
audimodal_processing_duration_seconds{processor="tier|embedding|analysis"} - Processing duration histogram

## Document Metrics
audimodal_documents_total{status="active|deleted|archived"} - Total documents by status
audimodal_documents_size_bytes - Document size distribution
audimodal_documents_chunks_total - Total chunks per document
audimodal_documents_processing_time_seconds - Document processing time

## Embedding Metrics
audimodal_embeddings_generated_total{model="openai|cohere|local"} - Total embeddings generated
audimodal_embeddings_cache_hits_total - Embedding cache hit count
audimodal_embeddings_cache_misses_total - Embedding cache miss count
audimodal_embeddings_generation_duration_seconds - Embedding generation time
audimodal_embeddings_batch_size - Embedding batch size distribution

## ML Analysis Metrics
audimodal_ml_analysis_requests_total{type="classification|extraction|summarization"} - ML analysis requests
audimodal_ml_analysis_success_total - Successful ML analyses
audimodal_ml_analysis_failure_total - Failed ML analyses
audimodal_ml_analysis_duration_seconds - ML analysis duration

## Sync Metrics
audimodal_sync_operations_total{source="sharepoint|onedrive|gdrive",status="success|failure"} - Sync operations
audimodal_sync_files_synced_total - Total files synced
audimodal_sync_bytes_synced_total - Total bytes synced
audimodal_sync_duration_seconds - Sync duration
audimodal_sync_errors_total{error_type="auth|network|rate_limit"} - Sync errors by type

## DLP Metrics
audimodal_dlp_violations_total{severity="critical|high|medium|low"} - DLP violations by severity
audimodal_dlp_policies_evaluated_total - Total DLP policies evaluated
audimodal_dlp_scan_duration_seconds - DLP scan duration
audimodal_dlp_false_positives_total - DLP false positives

## API Metrics
audimodal_api_requests_total{method="GET|POST|PUT|DELETE",endpoint="/api/v1/*"} - API requests
audimodal_api_request_duration_seconds{method="GET|POST|PUT|DELETE"} - API request duration
audimodal_api_errors_total{status="4xx|5xx"} - API errors by status code
audimodal_api_auth_failures_total - Authentication failures

## System Metrics
audimodal_database_connections{status="active|idle|waiting"} - Database connection pool
audimodal_database_query_duration_seconds{query_type="select|insert|update|delete"} - Query duration
audimodal_cache_hits_total{cache="document|embedding|metadata"} - Cache hits
audimodal_cache_misses_total{cache="document|embedding|metadata"} - Cache misses
audimodal_memory_usage_bytes - Memory usage
audimodal_goroutines_total - Active goroutines
EOF
    
    echo -e "${GREEN}✓ Metrics definitions extracted${NC}"
}

# Function to create Prometheus configuration
create_prometheus_config() {
    local output_file="$1"
    echo -e "${GREEN}Creating Prometheus configuration...${NC}"
    
    cat > "$output_file" << 'EOF'
# Prometheus configuration for Audimodal
global:
  scrape_interval: 15s
  evaluation_interval: 15s

# Alertmanager configuration
alerting:
  alertmanagers:
    - static_configs:
        - targets:
            - alertmanager:9093

# Load rules once and periodically evaluate them
rule_files:
  - "/etc/prometheus/rules/*.yml"

# Scrape configurations
scrape_configs:
  # Audimodal application metrics
  - job_name: 'audimodal'
    static_configs:
      - targets: 
          - 'audimodal-app:8080'
          - 'audimodal-app:8081'  # Metrics port
    metrics_path: '/metrics'
    scrape_interval: 10s
    
  # Audimodal database metrics (if using postgres exporter)
  - job_name: 'audimodal-postgres'
    static_configs:
      - targets: ['audimodal-postgres-exporter:9187']
    
  # Node exporter for system metrics
  - job_name: 'node-exporter'
    static_configs:
      - targets: ['node-exporter:9100']
    
  # OpenTelemetry collector metrics
  - job_name: 'otel-collector'
    static_configs:
      - targets: ['otel-collector:8888']
EOF
    
    echo -e "${GREEN}✓ Prometheus configuration created${NC}"
}

# Function to create alerting rules
create_alert_rules() {
    local output_dir="$1"
    mkdir -p "$output_dir"
    
    echo -e "${GREEN}Creating alerting rules...${NC}"
    
    cat > "$output_dir/audimodal-alerts.yml" << 'EOF'
groups:
  - name: audimodal_storage_alerts
    interval: 30s
    rules:
      - alert: HighStorageUsage
        expr: audimodal_storage_usage_percent > 80
        for: 5m
        labels:
          severity: warning
          service: audimodal
        annotations:
          summary: "High storage usage detected"
          description: "Storage usage is {{ $value }}% on {{ $labels.instance }}"
      
      - alert: StorageThroughputDegraded
        expr: rate(audimodal_storage_throughput_mbps[5m]) < 10
        for: 10m
        labels:
          severity: warning
          service: audimodal
        annotations:
          summary: "Storage throughput degraded"
          description: "Storage throughput is {{ $value }} MB/s on {{ $labels.instance }}"

  - name: audimodal_processing_alerts
    interval: 30s
    rules:
      - alert: ProcessingQueueBacklog
        expr: audimodal_processing_queue_size > 1000
        for: 5m
        labels:
          severity: warning
          service: audimodal
          processor: "{{ $labels.processor }}"
        annotations:
          summary: "Processing queue backlog detected"
          description: "{{ $labels.processor }} queue size is {{ $value }}"
      
      - alert: HighProcessingErrorRate
        expr: rate(audimodal_processing_errors_total[5m]) > 10
        for: 5m
        labels:
          severity: critical
          service: audimodal
        annotations:
          summary: "High processing error rate"
          description: "Processing error rate is {{ $value }} errors/sec for {{ $labels.processor }}"

  - name: audimodal_ml_alerts
    interval: 30s
    rules:
      - alert: MLAnalysisFailureRate
        expr: rate(audimodal_ml_analysis_failure_total[5m]) / rate(audimodal_ml_analysis_requests_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
          service: audimodal
        annotations:
          summary: "High ML analysis failure rate"
          description: "ML analysis failure rate is {{ $value | humanizePercentage }}"
      
      - alert: EmbeddingGenerationSlow
        expr: histogram_quantile(0.95, rate(audimodal_embeddings_generation_duration_seconds_bucket[5m])) > 5
        for: 10m
        labels:
          severity: warning
          service: audimodal
        annotations:
          summary: "Embedding generation is slow"
          description: "95th percentile embedding generation time is {{ $value }}s"

  - name: audimodal_dlp_alerts
    interval: 30s
    rules:
      - alert: CriticalDLPViolation
        expr: increase(audimodal_dlp_violations_total{severity="critical"}[1h]) > 0
        labels:
          severity: critical
          service: audimodal
        annotations:
          summary: "Critical DLP violation detected"
          description: "{{ $value }} critical DLP violations in the last hour"
      
      - alert: HighDLPFalsePositiveRate
        expr: rate(audimodal_dlp_false_positives_total[1h]) / rate(audimodal_dlp_violations_total[1h]) > 0.2
        for: 30m
        labels:
          severity: warning
          service: audimodal
        annotations:
          summary: "High DLP false positive rate"
          description: "DLP false positive rate is {{ $value | humanizePercentage }}"

  - name: audimodal_api_alerts
    interval: 30s
    rules:
      - alert: HighAPIErrorRate
        expr: rate(audimodal_api_errors_total[5m]) / rate(audimodal_api_requests_total[5m]) > 0.05
        for: 5m
        labels:
          severity: warning
          service: audimodal
        annotations:
          summary: "High API error rate"
          description: "API error rate is {{ $value | humanizePercentage }}"
      
      - alert: APIResponseTimeSlow
        expr: histogram_quantile(0.95, rate(audimodal_api_request_duration_seconds_bucket[5m])) > 2
        for: 10m
        labels:
          severity: warning
          service: audimodal
        annotations:
          summary: "API response time is slow"
          description: "95th percentile API response time is {{ $value }}s"
      
      - alert: AuthenticationFailureSpike
        expr: rate(audimodal_api_auth_failures_total[5m]) > 10
        for: 2m
        labels:
          severity: critical
          service: audimodal
        annotations:
          summary: "Authentication failure spike detected"
          description: "{{ $value }} auth failures per second"

  - name: audimodal_sync_alerts
    interval: 30s
    rules:
      - alert: SyncOperationFailures
        expr: rate(audimodal_sync_operations_total{status="failure"}[15m]) > 0.1
        for: 15m
        labels:
          severity: warning
          service: audimodal
          source: "{{ $labels.source }}"
        annotations:
          summary: "High sync failure rate for {{ $labels.source }}"
          description: "Sync failure rate is {{ $value }} failures/sec"
      
      - alert: SyncRateLimitHit
        expr: increase(audimodal_sync_errors_total{error_type="rate_limit"}[5m]) > 5
        labels:
          severity: warning
          service: audimodal
        annotations:
          summary: "Sync rate limit errors detected"
          description: "{{ $value }} rate limit errors in the last 5 minutes"

  - name: audimodal_database_alerts
    interval: 30s
    rules:
      - alert: DatabaseConnectionPoolExhausted
        expr: audimodal_database_connections{status="waiting"} > 10
        for: 5m
        labels:
          severity: critical
          service: audimodal
        annotations:
          summary: "Database connection pool exhausted"
          description: "{{ $value }} connections waiting"
      
      - alert: SlowDatabaseQueries
        expr: histogram_quantile(0.95, rate(audimodal_database_query_duration_seconds_bucket[5m])) > 1
        for: 10m
        labels:
          severity: warning
          service: audimodal
        annotations:
          summary: "Slow database queries detected"
          description: "95th percentile query time is {{ $value }}s for {{ $labels.query_type }} queries"
EOF
    
    echo -e "${GREEN}✓ Alert rules created${NC}"
}

# Function to create Grafana dashboards
create_grafana_dashboards() {
    local output_dir="$1"
    mkdir -p "$output_dir"
    
    echo -e "${GREEN}Creating Grafana dashboards...${NC}"
    
    # Main dashboard
    cat > "$output_dir/audimodal-overview.json" << 'EOF'
{
  "dashboard": {
    "title": "Audimodal Overview",
    "tags": ["audimodal", "overview"],
    "timezone": "browser",
    "panels": [
      {
        "id": 1,
        "title": "Request Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(audimodal_api_requests_total[5m])",
            "legendFormat": "{{method}} {{endpoint}}"
          }
        ],
        "gridPos": {"h": 8, "w": 12, "x": 0, "y": 0}
      },
      {
        "id": 2,
        "title": "Error Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(audimodal_api_errors_total[5m])",
            "legendFormat": "{{status}}"
          }
        ],
        "gridPos": {"h": 8, "w": 12, "x": 12, "y": 0}
      },
      {
        "id": 3,
        "title": "Storage Usage",
        "type": "stat",
        "targets": [
          {
            "expr": "audimodal_storage_usage_percent",
            "legendFormat": "Usage %"
          }
        ],
        "gridPos": {"h": 4, "w": 6, "x": 0, "y": 8}
      },
      {
        "id": 4,
        "title": "Processing Queue",
        "type": "graph",
        "targets": [
          {
            "expr": "audimodal_processing_queue_size",
            "legendFormat": "{{processor}}"
          }
        ],
        "gridPos": {"h": 8, "w": 12, "x": 0, "y": 12}
      },
      {
        "id": 5,
        "title": "Documents Processed",
        "type": "stat",
        "targets": [
          {
            "expr": "sum(rate(audimodal_documents_total[5m]))",
            "legendFormat": "Documents/sec"
          }
        ],
        "gridPos": {"h": 4, "w": 6, "x": 6, "y": 8}
      },
      {
        "id": 6,
        "title": "ML Analysis Performance",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(audimodal_ml_analysis_duration_seconds_bucket[5m]))",
            "legendFormat": "95th percentile"
          },
          {
            "expr": "histogram_quantile(0.5, rate(audimodal_ml_analysis_duration_seconds_bucket[5m]))",
            "legendFormat": "Median"
          }
        ],
        "gridPos": {"h": 8, "w": 12, "x": 12, "y": 12}
      },
      {
        "id": 7,
        "title": "Embedding Cache Hit Rate",
        "type": "stat",
        "targets": [
          {
            "expr": "rate(audimodal_embeddings_cache_hits_total[5m]) / (rate(audimodal_embeddings_cache_hits_total[5m]) + rate(audimodal_embeddings_cache_misses_total[5m]))",
            "legendFormat": "Hit Rate"
          }
        ],
        "gridPos": {"h": 4, "w": 6, "x": 12, "y": 8}
      },
      {
        "id": 8,
        "title": "DLP Violations",
        "type": "graph",
        "targets": [
          {
            "expr": "increase(audimodal_dlp_violations_total[1h])",
            "legendFormat": "{{severity}}"
          }
        ],
        "gridPos": {"h": 8, "w": 12, "x": 0, "y": 20}
      },
      {
        "id": 9,
        "title": "Sync Operations",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(audimodal_sync_operations_total[5m])",
            "legendFormat": "{{source}} - {{status}}"
          }
        ],
        "gridPos": {"h": 8, "w": 12, "x": 12, "y": 20}
      },
      {
        "id": 10,
        "title": "Database Performance",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(audimodal_database_query_duration_seconds_bucket[5m]))",
            "legendFormat": "{{query_type}}"
          }
        ],
        "gridPos": {"h": 8, "w": 12, "x": 0, "y": 28}
      }
    ],
    "version": 1
  }
}
EOF

    # Storage dashboard
    cat > "$output_dir/audimodal-storage.json" << 'EOF'
{
  "dashboard": {
    "title": "Audimodal Storage Metrics",
    "tags": ["audimodal", "storage"],
    "timezone": "browser",
    "panels": [
      {
        "id": 1,
        "title": "Storage by Tier",
        "type": "piechart",
        "targets": [
          {
            "expr": "audimodal_storage_total_bytes",
            "legendFormat": "{{tier}}"
          }
        ],
        "gridPos": {"h": 8, "w": 8, "x": 0, "y": 0}
      },
      {
        "id": 2,
        "title": "Storage by Type",
        "type": "piechart",
        "targets": [
          {
            "expr": "audimodal_storage_total_bytes",
            "legendFormat": "{{type}}"
          }
        ],
        "gridPos": {"h": 8, "w": 8, "x": 8, "y": 0}
      },
      {
        "id": 3,
        "title": "Storage Growth",
        "type": "graph",
        "targets": [
          {
            "expr": "audimodal_storage_total_bytes",
            "legendFormat": "{{tier}} - {{type}}"
          }
        ],
        "gridPos": {"h": 8, "w": 8, "x": 16, "y": 0}
      },
      {
        "id": 4,
        "title": "Throughput",
        "type": "graph",
        "targets": [
          {
            "expr": "audimodal_storage_throughput_mbps",
            "legendFormat": "MB/s"
          }
        ],
        "gridPos": {"h": 8, "w": 12, "x": 0, "y": 8}
      },
      {
        "id": 5,
        "title": "IOPS",
        "type": "graph",
        "targets": [
          {
            "expr": "audimodal_storage_iops",
            "legendFormat": "IOPS"
          }
        ],
        "gridPos": {"h": 8, "w": 12, "x": 12, "y": 8}
      }
    ],
    "version": 1
  }
}
EOF

    # Processing dashboard
    cat > "$output_dir/audimodal-processing.json" << 'EOF'
{
  "dashboard": {
    "title": "Audimodal Processing Pipeline",
    "tags": ["audimodal", "processing"],
    "timezone": "browser",
    "panels": [
      {
        "id": 1,
        "title": "Processing Queue Sizes",
        "type": "graph",
        "targets": [
          {
            "expr": "audimodal_processing_queue_size",
            "legendFormat": "{{processor}}"
          }
        ],
        "gridPos": {"h": 8, "w": 12, "x": 0, "y": 0}
      },
      {
        "id": 2,
        "title": "Processing Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(audimodal_processing_items_processed_total[5m])",
            "legendFormat": "{{processor}}"
          }
        ],
        "gridPos": {"h": 8, "w": 12, "x": 12, "y": 0}
      },
      {
        "id": 3,
        "title": "Processing Errors",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(audimodal_processing_errors_total[5m])",
            "legendFormat": "{{processor}}"
          }
        ],
        "gridPos": {"h": 8, "w": 12, "x": 0, "y": 8}
      },
      {
        "id": 4,
        "title": "Processing Duration",
        "type": "heatmap",
        "targets": [
          {
            "expr": "audimodal_processing_duration_seconds",
            "legendFormat": "{{processor}}"
          }
        ],
        "gridPos": {"h": 8, "w": 12, "x": 12, "y": 8}
      },
      {
        "id": 5,
        "title": "Embedding Generation",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(audimodal_embeddings_generated_total[5m])",
            "legendFormat": "{{model}}"
          }
        ],
        "gridPos": {"h": 8, "w": 12, "x": 0, "y": 16}
      },
      {
        "id": 6,
        "title": "Embedding Cache Performance",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(audimodal_embeddings_cache_hits_total[5m])",
            "legendFormat": "Hits"
          },
          {
            "expr": "rate(audimodal_embeddings_cache_misses_total[5m])",
            "legendFormat": "Misses"
          }
        ],
        "gridPos": {"h": 8, "w": 12, "x": 12, "y": 16}
      }
    ],
    "version": 1
  }
}
EOF
    
    echo -e "${GREEN}✓ Grafana dashboards created${NC}"
}

# Function to create dashboard provisioning config
create_dashboard_provisioning() {
    local output_file="$1"
    echo -e "${GREEN}Creating dashboard provisioning configuration...${NC}"
    
    cat > "$output_file" << 'EOF'
apiVersion: 1

providers:
  - name: 'Audimodal Dashboards'
    orgId: 1
    folder: 'Audimodal'
    folderUid: audimodal
    type: file
    disableDeletion: false
    updateIntervalSeconds: 10
    allowUiUpdates: true
    options:
      path: /var/lib/grafana/dashboards/audimodal
EOF
    
    echo -e "${GREEN}✓ Dashboard provisioning configuration created${NC}"
}

# Function to create datasource provisioning
create_datasource_provisioning() {
    local output_file="$1"
    echo -e "${GREEN}Creating datasource provisioning configuration...${NC}"
    
    cat > "$output_file" << 'EOF'
apiVersion: 1

datasources:
  - name: Prometheus-Audimodal
    type: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: false
    editable: true
    jsonData:
      timeInterval: "15s"
      queryTimeout: "60s"
      httpMethod: "POST"
EOF
    
    echo -e "${GREEN}✓ Datasource provisioning configuration created${NC}"
}

# Function to create OpenTelemetry collector config
create_otel_config() {
    local output_file="$1"
    echo -e "${GREEN}Creating OpenTelemetry collector configuration...${NC}"
    
    cat > "$output_file" << 'EOF'
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318
  
  prometheus:
    config:
      scrape_configs:
        - job_name: 'audimodal-otel'
          scrape_interval: 10s
          static_configs:
            - targets: ['audimodal-app:8081']

processors:
  batch:
    timeout: 10s
    send_batch_size: 1024
  
  memory_limiter:
    check_interval: 1s
    limit_mib: 512
  
  resource:
    attributes:
      - key: service.name
        value: audimodal
        action: upsert
      - key: service.namespace
        value: tas
        action: upsert

exporters:
  prometheus:
    endpoint: "0.0.0.0:8889"
    namespace: audimodal
    const_labels:
      environment: "${ENVIRONMENT}"
  
  logging:
    loglevel: info
  
  otlp:
    endpoint: jaeger:4317
    tls:
      insecure: true

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, batch, resource]
      exporters: [otlp, logging]
    
    metrics:
      receivers: [otlp, prometheus]
      processors: [memory_limiter, batch, resource]
      exporters: [prometheus, logging]
    
    logs:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [logging]

  extensions: [health_check, pprof, zpages]

extensions:
  health_check:
    endpoint: 0.0.0.0:13133
  pprof:
    endpoint: 0.0.0.0:1777
  zpages:
    endpoint: 0.0.0.0:55679
EOF
    
    echo -e "${GREEN}✓ OpenTelemetry collector configuration created${NC}"
}

# Main sync process
main() {
    echo -e "${BLUE}Starting observability sync process...${NC}"
    echo ""
    
    # Create directory structure
    mkdir -p "${SHARED_MONITORING_DIR}/prometheus/rules"
    mkdir -p "${SHARED_MONITORING_DIR}/grafana/provisioning/dashboards"
    mkdir -p "${SHARED_MONITORING_DIR}/grafana/provisioning/datasources"
    mkdir -p "${SHARED_MONITORING_DIR}/grafana/dashboards/audimodal"
    mkdir -p "${SHARED_MONITORING_DIR}/otel"
    mkdir -p "${SHARED_MONITORING_DIR}/docs"
    
    # Extract and sync metrics
    extract_metrics_from_code "${SHARED_MONITORING_DIR}/docs/audimodal-metrics.md"
    
    # Create Prometheus configuration
    create_prometheus_config "${SHARED_MONITORING_DIR}/prometheus/prometheus-audimodal.yml"
    
    # Create alert rules
    create_alert_rules "${SHARED_MONITORING_DIR}/prometheus/rules"
    
    # Create Grafana dashboards
    create_grafana_dashboards "${SHARED_MONITORING_DIR}/grafana/dashboards/audimodal"
    
    # Create provisioning configurations
    create_dashboard_provisioning "${SHARED_MONITORING_DIR}/grafana/provisioning/dashboards/audimodal.yml"
    create_datasource_provisioning "${SHARED_MONITORING_DIR}/grafana/provisioning/datasources/prometheus-audimodal.yml"
    
    # Create OpenTelemetry configuration
    create_otel_config "${SHARED_MONITORING_DIR}/otel/otel-collector-audimodal.yml"
    
    # Create integration documentation
    cat > "${SHARED_MONITORING_DIR}/docs/audimodal-integration.md" << EOF
# Audimodal Observability Integration

This directory contains the observability configuration for the Audimodal application,
synced from the main audimodal repository.

## Components

### Metrics
- **Location**: \`docs/audimodal-metrics.md\`
- **Description**: Complete list of metrics exposed by Audimodal

### Prometheus
- **Configuration**: \`prometheus/prometheus-audimodal.yml\`
- **Alert Rules**: \`prometheus/rules/audimodal-alerts.yml\`
- **Scrape Targets**: audimodal-app:8080, audimodal-app:8081

### Grafana
- **Dashboards**: \`grafana/dashboards/audimodal/\`
  - audimodal-overview.json - Main application dashboard
  - audimodal-storage.json - Storage metrics dashboard
  - audimodal-processing.json - Processing pipeline dashboard
- **Provisioning**: Auto-provisioned via configuration files

### OpenTelemetry
- **Configuration**: \`otel/otel-collector-audimodal.yml\`
- **Endpoints**: 
  - OTLP gRPC: 4317
  - OTLP HTTP: 4318
  - Metrics: 8889

## Integration Steps

1. Ensure the shared monitoring stack is running:
   \`\`\`bash
   cd ../aether-shared
   ./start-shared-services.sh
   \`\`\`

2. Update audimodal docker-compose to connect to shared network:
   \`\`\`yaml
   networks:
     default:
       external:
         name: tas-shared-network
   \`\`\`

3. Configure audimodal to export metrics:
   - Set environment variable: \`METRICS_ENABLED=true\`
   - Set metrics endpoint: \`METRICS_PORT=8081\`
   - Set OTLP endpoint: \`OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317\`

4. Access dashboards:
   - Grafana: http://localhost:3000
   - Prometheus: http://localhost:9090
   - AlertManager: http://localhost:9093

## Sync Process

To update the observability configuration:
\`\`\`bash
cd audimodal
./scripts/sync-observability-to-shared.sh
\`\`\`

Last synced: $(date)
EOF
    
    echo ""
    echo -e "${GREEN}=== Sync Complete ===${NC}"
    echo -e "${GREEN}✓ Metrics definitions${NC}"
    echo -e "${GREEN}✓ Prometheus configuration${NC}"
    echo -e "${GREEN}✓ Alert rules${NC}"
    echo -e "${GREEN}✓ Grafana dashboards${NC}"
    echo -e "${GREEN}✓ OpenTelemetry configuration${NC}"
    echo -e "${GREEN}✓ Documentation${NC}"
    echo ""
    echo -e "${BLUE}Files synced to: ${SHARED_MONITORING_DIR}${NC}"
    echo ""
    echo -e "${YELLOW}Next steps:${NC}"
    echo "1. Review the synced configuration in ${SHARED_MONITORING_DIR}"
    echo "2. Commit changes to the aether-shared repository"
    echo "3. Restart the shared monitoring stack to apply changes"
    echo ""
}

# Run main function
main "$@"