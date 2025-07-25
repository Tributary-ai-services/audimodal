# Deep Lake Vector Service - Complete Implementation Specification

## Executive Summary

This document provides a complete specification for creating a standalone Deep Lake Vector Service repository. The service will replace the mock Deep Lake implementation currently in the eAIIngest project with a production-ready Python service that provides both HTTP REST and gRPC APIs.

## Background & Context

### Current Problem
The AudiModal project currently contains a mock Deep Lake implementation in Go (`pkg/embeddings/vectorstore/deeplake.go`) that simulates Deep Lake functionality using in-memory maps. This won't work in production since Deep Lake is a Python-only library with no Go bindings.

### Solution
Create a standalone Python service that:
- Provides real Deep Lake integration
- Supports both HTTP REST and gRPC protocols
- Can be reused by any project or language
- Integrates seamlessly with the existing AudiModal architecture

## Repository Structure

```
deeplake-vector-service/
├── README.md
├── LICENSE
├── .gitignore
├── pyproject.toml
├── requirements.txt
├── requirements-dev.txt
├── docker-compose.yml
├── Dockerfile
├── .env.example
├── .github/
│   └── workflows/
│       ├── ci.yml
│       ├── cd.yml
│       └── docker.yml
├── app/
│   ├── __init__.py
│   ├── main.py                    # Main application entry point
│   ├── config/
│   │   ├── __init__.py
│   │   ├── settings.py            # Configuration management
│   │   └── logging.py             # Logging configuration
│   ├── api/
│   │   ├── __init__.py
│   │   ├── http/                  # HTTP REST API
│   │   │   ├── __init__.py
│   │   │   ├── v1/
│   │   │   │   ├── __init__.py
│   │   │   │   ├── datasets.py    # Dataset management endpoints
│   │   │   │   ├── vectors.py     # Vector operations endpoints
│   │   │   │   ├── search.py      # Search endpoints
│   │   │   │   └── health.py      # Health check endpoints
│   │   │   ├── middleware.py      # CORS, auth, rate limiting
│   │   │   └── dependencies.py    # FastAPI dependencies
│   │   └── grpc/                  # gRPC API
│   │       ├── __init__.py
│   │       ├── server.py          # gRPC server setup
│   │       └── handlers/
│   │           ├── __init__.py
│   │           ├── dataset_handler.py
│   │           ├── vector_handler.py
│   │           └── search_handler.py
│   ├── services/
│   │   ├── __init__.py
│   │   ├── deeplake_service.py    # Core Deep Lake operations
│   │   ├── auth_service.py        # Authentication & authorization
│   │   ├── cache_service.py       # Caching layer
│   │   └── metrics_service.py     # Monitoring & metrics
│   ├── models/
│   │   ├── __init__.py
│   │   ├── schemas.py             # Pydantic models for HTTP API
│   │   ├── proto/                 # Protocol buffer definitions
│   │   │   ├── __init__.py
│   │   │   ├── deeplake_service.proto
│   │   │   ├── deeplake_service_pb2.py
│   │   │   └── deeplake_service_pb2_grpc.py
│   │   └── exceptions.py          # Custom exceptions
│   ├── utils/
│   │   ├── __init__.py
│   │   ├── validators.py          # Input validation
│   │   ├── formatters.py          # Response formatting
│   │   └── helpers.py             # Utility functions
│   └── middleware/
│       ├── __init__.py
│       ├── auth.py                # Authentication middleware
│       ├── rate_limit.py          # Rate limiting
│       └── cors.py                # CORS configuration
├── tests/
│   ├── __init__.py
│   ├── conftest.py                # Test configuration
│   ├── unit/
│   │   ├── __init__.py
│   │   ├── test_deeplake_service.py
│   │   ├── test_http_api.py
│   │   └── test_grpc_api.py
│   ├── integration/
│   │   ├── __init__.py
│   │   ├── test_end_to_end.py
│   │   └── test_performance.py
│   └── fixtures/
│       ├── __init__.py
│       └── sample_data.py
├── docs/
│   ├── README.md
│   ├── api/
│   │   ├── http_api.md           # HTTP API documentation
│   │   ├── grpc_api.md           # gRPC API documentation
│   │   └── openapi.json          # OpenAPI specification
│   ├── deployment/
│   │   ├── docker.md
│   │   ├── kubernetes.md
│   │   └── docker-compose.md
│   └── examples/
│       ├── python_client.py
│       ├── go_client.go
│       ├── javascript_client.js
│       └── curl_examples.sh
├── deployment/
│   ├── kubernetes/
│   │   ├── namespace.yaml
│   │   ├── configmap.yaml
│   │   ├── secret.yaml
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   ├── ingress.yaml
│   │   └── hpa.yaml
│   └── helm/
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
├── scripts/
│   ├── build.sh
│   ├── test.sh
│   ├── deploy.sh
│   └── generate_proto.sh
└── proto/
    └── deeplake_service.proto     # Protocol buffer definitions
```

## API Design

### HTTP REST API

**Base URL**: `http://localhost:8000/api/v1`

#### Datasets
```
POST   /datasets                    # Create dataset
GET    /datasets                    # List datasets
GET    /datasets/{dataset_id}       # Get dataset info
PUT    /datasets/{dataset_id}       # Update dataset
DELETE /datasets/{dataset_id}       # Delete dataset
GET    /datasets/{dataset_id}/stats # Get dataset statistics
```

#### Vectors
```
POST   /datasets/{dataset_id}/vectors              # Insert vectors
GET    /datasets/{dataset_id}/vectors              # List vectors (paginated)
GET    /datasets/{dataset_id}/vectors/{vector_id}  # Get specific vector
PUT    /datasets/{dataset_id}/vectors/{vector_id}  # Update vector
DELETE /datasets/{dataset_id}/vectors/{vector_id}  # Delete vector
POST   /datasets/{dataset_id}/vectors/batch        # Batch insert vectors
DELETE /datasets/{dataset_id}/vectors/batch        # Batch delete vectors
```

#### Search
```
POST   /datasets/{dataset_id}/search               # Vector similarity search
POST   /datasets/{dataset_id}/search/text          # Text-based search
POST   /datasets/{dataset_id}/search/hybrid        # Hybrid search
POST   /search/multi-dataset                       # Multi-dataset search
```

#### Health & Monitoring
```
GET    /health                      # Health check
GET    /metrics                     # Prometheus metrics
GET    /stats                       # Service statistics
```

### gRPC API

**Protocol Buffer Definition**:
```protobuf
syntax = "proto3";

package deeplake.v1;

option go_package = "github.com/yourorg/deeplake-vector-service/proto/deeplake/v1;deeplakev1";

// Dataset Management Service
service DatasetService {
  rpc CreateDataset(CreateDatasetRequest) returns (CreateDatasetResponse);
  rpc GetDataset(GetDatasetRequest) returns (GetDatasetResponse);
  rpc ListDatasets(ListDatasetsRequest) returns (ListDatasetsResponse);
  rpc UpdateDataset(UpdateDatasetRequest) returns (UpdateDatasetResponse);
  rpc DeleteDataset(DeleteDatasetRequest) returns (DeleteDatasetResponse);
  rpc GetDatasetStats(GetDatasetStatsRequest) returns (GetDatasetStatsResponse);
}

// Vector Operations Service
service VectorService {
  rpc InsertVector(InsertVectorRequest) returns (InsertVectorResponse);
  rpc InsertVectors(InsertVectorsRequest) returns (InsertVectorsResponse);
  rpc GetVector(GetVectorRequest) returns (GetVectorResponse);
  rpc UpdateVector(UpdateVectorRequest) returns (UpdateVectorResponse);
  rpc DeleteVector(DeleteVectorRequest) returns (DeleteVectorResponse);
  rpc ListVectors(ListVectorsRequest) returns (ListVectorsResponse);
}

// Search Service
service SearchService {
  rpc SearchVectors(SearchVectorsRequest) returns (SearchVectorsResponse);
  rpc SearchByText(SearchByTextRequest) returns (SearchByTextResponse);
  rpc HybridSearch(HybridSearchRequest) returns (HybridSearchResponse);
}

// Health Service
service HealthService {
  rpc Check(HealthCheckRequest) returns (HealthCheckResponse);
  rpc GetMetrics(GetMetricsRequest) returns (GetMetricsResponse);
}

// Common message types
message Dataset {
  string id = 1;
  string name = 2;
  string description = 3;
  int32 dimensions = 4;
  string metric_type = 5;
  string index_type = 6;
  map<string, string> metadata = 7;
  string storage_location = 8;
  int64 vector_count = 9;
  int64 storage_size = 10;
  google.protobuf.Timestamp created_at = 11;
  google.protobuf.Timestamp updated_at = 12;
  string tenant_id = 13;
}

message Vector {
  string id = 1;
  string dataset_id = 2;
  string document_id = 3;
  string chunk_id = 4;
  repeated float values = 5;
  string content = 6;
  string content_hash = 7;
  map<string, string> metadata = 8;
  string content_type = 9;
  string language = 10;
  int32 chunk_index = 11;
  int32 chunk_count = 12;
  string model = 13;
  int32 dimensions = 14;
  google.protobuf.Timestamp created_at = 15;
  google.protobuf.Timestamp updated_at = 16;
  string tenant_id = 17;
}

message SearchResult {
  Vector vector = 1;
  float score = 2;
  float distance = 3;
  int32 rank = 4;
  map<string, string> explanation = 5;
}

message SearchOptions {
  int32 top_k = 1;
  float threshold = 2;
  string metric_type = 3;
  bool include_content = 4;
  bool include_metadata = 5;
  map<string, string> filters = 6;
  bool deduplicate = 7;
  bool group_by_document = 8;
  bool rerank = 9;
  int32 ef_search = 10;
  int32 nprobe = 11;
  float max_distance = 12;
  float min_score = 13;
}

// Request/Response messages
message CreateDatasetRequest {
  string name = 1;
  string description = 2;
  int32 dimensions = 3;
  string metric_type = 4;
  string index_type = 5;
  map<string, string> metadata = 6;
  string storage_location = 7;
  bool overwrite = 8;
  string tenant_id = 9;
}

message CreateDatasetResponse {
  Dataset dataset = 1;
  string message = 2;
}

message InsertVectorsRequest {
  string dataset_id = 1;
  repeated Vector vectors = 2;
  bool skip_existing = 3;
  bool overwrite = 4;
  int32 batch_size = 5;
  string tenant_id = 6;
}

message InsertVectorsResponse {
  int32 inserted_count = 1;
  int32 skipped_count = 2;
  int32 failed_count = 3;
  repeated string error_messages = 4;
  double processing_time_ms = 5;
}

message SearchVectorsRequest {
  string dataset_id = 1;
  repeated float query_vector = 2;
  SearchOptions options = 3;
  string tenant_id = 4;
}

message SearchVectorsResponse {
  repeated SearchResult results = 1;
  int32 total_found = 2;
  bool has_more = 3;
  double query_time_ms = 4;
  SearchStats stats = 5;
}

message SearchByTextRequest {
  string dataset_id = 1;
  string query_text = 2;
  SearchOptions options = 3;
  string tenant_id = 4;
}

message SearchByTextResponse {
  repeated SearchResult results = 1;
  int32 total_found = 2;
  bool has_more = 3;
  double query_time_ms = 4;
  double embedding_time_ms = 5;
  SearchStats stats = 6;
}

message SearchStats {
  int64 vectors_scanned = 1;
  int64 index_hits = 2;
  int64 filtered_results = 3;
  double reranking_time_ms = 4;
  double database_time_ms = 5;
  double post_processing_time_ms = 6;
}

message HealthCheckRequest {}

message HealthCheckResponse {
  string status = 1;
  string service = 2;
  string version = 3;
  google.protobuf.Timestamp timestamp = 4;
  map<string, string> dependencies = 5;
}
```

## Core Implementation Requirements

### 1. Deep Lake Service Core

```python
# app/services/deeplake_service.py
class DeepLakeService:
    def __init__(self, config: DeepLakeConfig):
        self.config = config
        self.datasets = {}
        self.cache = CacheService()
        self.metrics = MetricsService()
    
    async def create_dataset(self, request: CreateDatasetRequest) -> Dataset:
        """Create a new Deep Lake dataset"""
        # Implementation using actual Deep Lake API
        pass
    
    async def insert_vectors(self, request: InsertVectorsRequest) -> InsertVectorsResponse:
        """Insert vectors into Deep Lake dataset"""
        # Implementation using actual Deep Lake API
        pass
    
    async def search_vectors(self, request: SearchVectorsRequest) -> SearchVectorsResponse:
        """Perform vector similarity search"""
        # Implementation using actual Deep Lake API
        pass
    
    async def get_dataset_stats(self, dataset_id: str) -> DatasetStats:
        """Get comprehensive dataset statistics"""
        # Implementation using actual Deep Lake API
        pass
```

### 2. HTTP API Implementation

```python
# app/api/http/v1/datasets.py
from fastapi import APIRouter, Depends, HTTPException, Query
from app.services.deeplake_service import DeepLakeService
from app.models.schemas import DatasetCreate, DatasetResponse, DatasetStats
from app.api.http.dependencies import get_deeplake_service, get_current_tenant

router = APIRouter(prefix="/datasets", tags=["datasets"])

@router.post("/", response_model=DatasetResponse, status_code=201)
async def create_dataset(
    dataset: DatasetCreate,
    service: DeepLakeService = Depends(get_deeplake_service),
    tenant_id: str = Depends(get_current_tenant)
):
    """Create a new dataset"""
    try:
        result = await service.create_dataset(dataset, tenant_id)
        return result
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))

@router.get("/", response_model=list[DatasetResponse])
async def list_datasets(
    limit: int = Query(default=100, le=1000),
    offset: int = Query(default=0, ge=0),
    service: DeepLakeService = Depends(get_deeplake_service),
    tenant_id: str = Depends(get_current_tenant)
):
    """List all datasets for tenant"""
    return await service.list_datasets(tenant_id, limit, offset)
```

### 3. gRPC Implementation

```python
# app/api/grpc/handlers/dataset_handler.py
import grpc
from app.proto import deeplake_service_pb2_grpc, deeplake_service_pb2
from app.services.deeplake_service import DeepLakeService

class DatasetServicer(deeplake_service_pb2_grpc.DatasetServiceServicer):
    def __init__(self, deeplake_service: DeepLakeService):
        self.service = deeplake_service
    
    async def CreateDataset(self, request, context):
        """Create a new dataset"""
        try:
            result = await self.service.create_dataset(request)
            return deeplake_service_pb2.CreateDatasetResponse(
                dataset=result,
                message="Dataset created successfully"
            )
        except Exception as e:
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            raise
    
    async def SearchVectors(self, request, context):
        """Search for similar vectors"""
        try:
            result = await self.service.search_vectors(request)
            return result
        except Exception as e:
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            raise
```

### 4. Main Application

```python
# app/main.py
import asyncio
import uvicorn
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from contextlib import asynccontextmanager
import grpc
from concurrent.futures import ThreadPoolExecutor

from app.api.http.v1 import datasets, vectors, search, health
from app.api.grpc.server import create_grpc_server
from app.config.settings import settings
from app.services.deeplake_service import DeepLakeService
from app.middleware.auth import AuthMiddleware
from app.middleware.rate_limit import RateLimitMiddleware

@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan manager"""
    # Startup
    app.state.deeplake_service = DeepLakeService(settings.deeplake_config)
    app.state.grpc_server = create_grpc_server(app.state.deeplake_service)
    
    # Start gRPC server
    await app.state.grpc_server.start()
    
    yield
    
    # Shutdown
    await app.state.grpc_server.stop()
    await app.state.deeplake_service.close()

# FastAPI app
app = FastAPI(
    title="Deep Lake Vector Service",
    description="Universal Deep Lake vector database service with HTTP and gRPC APIs",
    version="1.0.0",
    lifespan=lifespan
)

# Add middleware
app.add_middleware(CORSMiddleware, allow_origins=["*"])
app.add_middleware(AuthMiddleware)
app.add_middleware(RateLimitMiddleware)

# Include routers
app.include_router(health.router, prefix="/api/v1")
app.include_router(datasets.router, prefix="/api/v1")
app.include_router(vectors.router, prefix="/api/v1")
app.include_router(search.router, prefix="/api/v1")

if __name__ == "__main__":
    uvicorn.run(
        "app.main:app",
        host=settings.http_host,
        port=settings.http_port,
        reload=settings.debug
    )
```

## Integration with AudiModal

### Current Integration Points

The AudiModal project uses the following data structures (from `pkg/embeddings/types.go`):

1. **Core Interfaces**:
   - `VectorStore` interface - needs gRPC client implementation
   - `EmbeddingService` interface - remains unchanged
   - `EmbeddingProvider` interface - remains unchanged

2. **Key Data Types**:
   - `DocumentVector` - maps to gRPC `Vector` message
   - `DatasetConfig` - maps to gRPC `CreateDatasetRequest`
   - `SearchOptions` - maps to gRPC `SearchOptions`
   - `SearchResult` - maps to gRPC `SearchVectorsResponse`
   - `ProcessDocumentRequest` - high-level wrapper
   - `ProcessDocumentResponse` - high-level wrapper

### Required Go Client Implementation

```go
// pkg/embeddings/vectorstore/deeplake_grpc.go
package vectorstore

import (
    "context"
    "google.golang.org/grpc"
    pb "github.com/yourorg/deeplake-vector-service/proto/deeplake/v1"
    "github.com/jscharber/eAIIngest/pkg/embeddings"
)

type DeepLakeGRPCStore struct {
    client pb.VectorServiceClient
    conn   *grpc.ClientConn
    config *embeddings.DeeplakeConfig
}

func NewDeepLakeGRPCStore(config *embeddings.DeeplakeConfig) (*DeepLakeGRPCStore, error) {
    conn, err := grpc.Dial(config.GRPCAddress, grpc.WithInsecure())
    if err != nil {
        return nil, err
    }
    
    client := pb.NewVectorServiceClient(conn)
    
    return &DeepLakeGRPCStore{
        client: client,
        conn:   conn,
        config: config,
    }, nil
}

func (d *DeepLakeGRPCStore) CreateDataset(ctx context.Context, config *embeddings.DatasetConfig) error {
    req := &pb.CreateDatasetRequest{
        Name:            config.Name,
        Description:     config.Description,
        Dimensions:      int32(config.Dimensions),
        MetricType:      config.MetricType,
        IndexType:       config.IndexType,
        StorageLocation: config.StorageLocation,
        Overwrite:       config.Overwrite,
    }
    
    _, err := d.client.CreateDataset(ctx, req)
    return err
}

func (d *DeepLakeGRPCStore) InsertVectors(ctx context.Context, datasetName string, vectors []*embeddings.DocumentVector) error {
    grpcVectors := make([]*pb.Vector, len(vectors))
    for i, v := range vectors {
        grpcVectors[i] = &pb.Vector{
            Id:          v.ID,
            DocumentId:  v.DocumentID,
            ChunkId:     v.ChunkID,
            Values:      v.Vector,
            Content:     v.Content,
            ContentHash: v.ContentHash,
            Metadata:    convertMetadata(v.Metadata),
            ContentType: v.ContentType,
            Language:    v.Language,
            ChunkIndex:  int32(v.ChunkIndex),
            ChunkCount:  int32(v.ChunkCount),
            Model:       v.Model,
            Dimensions:  int32(v.Dimensions),
            TenantId:    v.TenantID.String(),
        }
    }
    
    req := &pb.InsertVectorsRequest{
        DatasetId: datasetName,
        Vectors:   grpcVectors,
    }
    
    _, err := d.client.InsertVectors(ctx, req)
    return err
}

func (d *DeepLakeGRPCStore) SearchSimilar(ctx context.Context, datasetName string, queryVector []float32, options *embeddings.SearchOptions) (*embeddings.SearchResult, error) {
    grpcOptions := &pb.SearchOptions{
        TopK:            int32(options.TopK),
        Threshold:       options.Threshold,
        MetricType:      options.MetricType,
        IncludeContent:  options.IncludeContent,
        IncludeMetadata: options.IncludeMetadata,
        Filters:         convertFilters(options.Filters),
        Deduplicate:     options.Deduplicate,
        GroupByDocument: options.GroupByDoc,
        Rerank:          options.Rerank,
    }
    
    req := &pb.SearchVectorsRequest{
        DatasetId:   datasetName,
        QueryVector: queryVector,
        Options:     grpcOptions,
    }
    
    resp, err := d.client.SearchVectors(ctx, req)
    if err != nil {
        return nil, err
    }
    
    return convertSearchResponse(resp), nil
}
```

### Configuration Updates

Update `pkg/embeddings/types.go` to include gRPC configuration:

```go
// DeeplakeConfig contains configuration for Deeplake vector store
type DeeplakeConfig struct {
    // Existing fields...
    StorageLocation string        `yaml:"storage_location"`
    Token           string        `yaml:"token"`
    
    // New gRPC configuration
    GRPCAddress     string        `yaml:"grpc_address"`     // e.g., "localhost:50051"
    GRPCTLSEnabled  bool          `yaml:"grpc_tls_enabled"`
    GRPCTLSCert     string        `yaml:"grpc_tls_cert"`
    GRPCTimeout     time.Duration `yaml:"grpc_timeout"`
    
    // HTTP fallback configuration
    HTTPAddress     string        `yaml:"http_address"`     // e.g., "http://localhost:8000"
    HTTPTimeout     time.Duration `yaml:"http_timeout"`
    
    // Connection options
    MaxRetries      int           `yaml:"max_retries"`
    RetryDelay      time.Duration `yaml:"retry_delay"`
    MaxConnections  int           `yaml:"max_connections"`
}
```

## Deployment and Operations

### Docker Configuration

```dockerfile
# Dockerfile
FROM python:3.11-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    gcc \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements first for better caching
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application code
COPY . .

# Create non-root user
RUN useradd -m -u 1000 appuser && chown -R appuser:appuser /app
USER appuser

# Expose ports
EXPOSE 8000 50051

# Command to run the application
CMD ["python", "app/main.py"]
```

### Docker Compose

```yaml
# docker-compose.yml
version: '3.8'

services:
  deeplake-service:
    build: .
    ports:
      - "8000:8000"    # HTTP
      - "50051:50051"  # gRPC
    environment:
      - DEEPLAKE_STORAGE_LOCATION=/data/vectors
      - HTTP_HOST=0.0.0.0
      - HTTP_PORT=8000
      - GRPC_HOST=0.0.0.0
      - GRPC_PORT=50051
      - LOG_LEVEL=INFO
    volumes:
      - ./data:/data
      - ./logs:/app/logs
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8000/api/v1/health"]
      interval: 30s
      timeout: 10s
      retries: 3
    restart: unless-stopped

  # Optional: Redis for caching
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    command: redis-server --appendonly yes
    volumes:
      - redis_data:/data

  # Optional: Prometheus for metrics
  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus_data:/prometheus

volumes:
  redis_data:
  prometheus_data:
```

### Kubernetes Deployment

```yaml
# deployment/kubernetes/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: deeplake-service
  labels:
    app: deeplake-service
spec:
  replicas: 3
  selector:
    matchLabels:
      app: deeplake-service
  template:
    metadata:
      labels:
        app: deeplake-service
    spec:
      containers:
      - name: deeplake-service
        image: deeplake-service:latest
        ports:
        - containerPort: 8000
          name: http
        - containerPort: 50051
          name: grpc
        env:
        - name: DEEPLAKE_STORAGE_LOCATION
          value: "/data/vectors"
        - name: HTTP_HOST
          value: "0.0.0.0"
        - name: HTTP_PORT
          value: "8000"
        - name: GRPC_HOST
          value: "0.0.0.0"
        - name: GRPC_PORT
          value: "50051"
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "2Gi"
            cpu: "2000m"
        livenessProbe:
          httpGet:
            path: /api/v1/health
            port: 8000
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /api/v1/health
            port: 8000
          initialDelaySeconds: 5
          periodSeconds: 5
        volumeMounts:
        - name: data
          mountPath: /data
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: deeplake-data-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: deeplake-service
spec:
  selector:
    app: deeplake-service
  ports:
  - name: http
    port: 8000
    targetPort: 8000
  - name: grpc
    port: 50051
    targetPort: 50051
  type: ClusterIP
```

## Testing Strategy

### Unit Tests

```python
# tests/unit/test_deeplake_service.py
import pytest
from unittest.mock import Mock, AsyncMock
from app.services.deeplake_service import DeepLakeService
from app.models.schemas import DatasetCreate, VectorInsert

@pytest.fixture
def mock_deeplake_service():
    return DeepLakeService(mock_config)

@pytest.mark.asyncio
async def test_create_dataset(mock_deeplake_service):
    """Test dataset creation"""
    dataset = DatasetCreate(
        name="test-dataset",
        description="Test dataset",
        dimensions=1536,
        metric_type="cosine"
    )
    
    result = await mock_deeplake_service.create_dataset(dataset)
    assert result.name == "test-dataset"
    assert result.dimensions == 1536

@pytest.mark.asyncio
async def test_insert_vectors(mock_deeplake_service):
    """Test vector insertion"""
    vectors = [
        VectorInsert(
            id="vec1",
            document_id="doc1",
            values=[0.1, 0.2, 0.3],
            content="test content"
        )
    ]
    
    result = await mock_deeplake_service.insert_vectors("test-dataset", vectors)
    assert result.inserted_count == 1
```

### Integration Tests

```python
# tests/integration/test_end_to_end.py
import pytest
import httpx
import grpc
from app.proto import deeplake_service_pb2, deeplake_service_pb2_grpc

@pytest.mark.asyncio
async def test_http_api_flow():
    """Test complete HTTP API flow"""
    async with httpx.AsyncClient(base_url="http://localhost:8000") as client:
        # Create dataset
        dataset_response = await client.post("/api/v1/datasets", json={
            "name": "integration-test",
            "dimensions": 128,
            "metric_type": "cosine"
        })
        assert dataset_response.status_code == 201
        
        # Insert vectors
        vectors_response = await client.post(
            "/api/v1/datasets/integration-test/vectors",
            json={
                "vectors": [
                    {
                        "id": "vec1",
                        "document_id": "doc1",
                        "values": [0.1] * 128,
                        "content": "test content"
                    }
                ]
            }
        )
        assert vectors_response.status_code == 201
        
        # Search vectors
        search_response = await client.post(
            "/api/v1/datasets/integration-test/search",
            json={
                "query_vector": [0.1] * 128,
                "options": {"top_k": 5}
            }
        )
        assert search_response.status_code == 200
        results = search_response.json()
        assert len(results["results"]) == 1

@pytest.mark.asyncio
async def test_grpc_api_flow():
    """Test complete gRPC API flow"""
    async with grpc.aio.insecure_channel("localhost:50051") as channel:
        dataset_client = deeplake_service_pb2_grpc.DatasetServiceStub(channel)
        vector_client = deeplake_service_pb2_grpc.VectorServiceStub(channel)
        search_client = deeplake_service_pb2_grpc.SearchServiceStub(channel)
        
        # Create dataset
        dataset_response = await dataset_client.CreateDataset(
            deeplake_service_pb2.CreateDatasetRequest(
                name="grpc-test",
                dimensions=128,
                metric_type="cosine"
            )
        )
        assert dataset_response.dataset.name == "grpc-test"
        
        # Insert vectors
        vectors_response = await vector_client.InsertVectors(
            deeplake_service_pb2.InsertVectorsRequest(
                dataset_id="grpc-test",
                vectors=[
                    deeplake_service_pb2.Vector(
                        id="vec1",
                        document_id="doc1",
                        values=[0.1] * 128,
                        content="test content"
                    )
                ]
            )
        )
        assert vectors_response.inserted_count == 1
        
        # Search vectors
        search_response = await search_client.SearchVectors(
            deeplake_service_pb2.SearchVectorsRequest(
                dataset_id="grpc-test",
                query_vector=[0.1] * 128,
                options=deeplake_service_pb2.SearchOptions(top_k=5)
            )
        )
        assert len(search_response.results) == 1
```

## Performance and Monitoring

### Key Metrics to Track

1. **Request Metrics**:
   - Request rate (requests/second)
   - Request latency (P50, P95, P99)
   - Error rate
   - Request size distribution

2. **Vector Operation Metrics**:
   - Vector insertion rate
   - Search query latency
   - Index build time
   - Dataset size growth

3. **System Metrics**:
   - Memory usage
   - CPU utilization
   - Disk I/O
   - Network bandwidth

4. **Deep Lake Specific Metrics**:
   - Dataset count
   - Total vectors stored
   - Storage utilization
   - Cache hit rate

### Monitoring Implementation

```python
# app/services/metrics_service.py
from prometheus_client import Counter, Histogram, Gauge
import time

class MetricsService:
    def __init__(self):
        # Request metrics
        self.request_count = Counter(
            'deeplake_requests_total',
            'Total requests',
            ['method', 'endpoint', 'status']
        )
        
        self.request_duration = Histogram(
            'deeplake_request_duration_seconds',
            'Request duration',
            ['method', 'endpoint']
        )
        
        # Vector operation metrics
        self.vectors_inserted = Counter(
            'deeplake_vectors_inserted_total',
            'Total vectors inserted',
            ['dataset']
        )
        
        self.search_queries = Counter(
            'deeplake_search_queries_total',
            'Total search queries',
            ['dataset']
        )
        
        self.search_latency = Histogram(
            'deeplake_search_latency_seconds',
            'Search query latency',
            ['dataset']
        )
        
        # System metrics
        self.active_datasets = Gauge(
            'deeplake_active_datasets',
            'Number of active datasets'
        )
        
        self.total_vectors = Gauge(
            'deeplake_total_vectors',
            'Total vectors stored'
        )
    
    def record_request(self, method: str, endpoint: str, status: int, duration: float):
        self.request_count.labels(method=method, endpoint=endpoint, status=status).inc()
        self.request_duration.labels(method=method, endpoint=endpoint).observe(duration)
    
    def record_vector_insertion(self, dataset: str, count: int):
        self.vectors_inserted.labels(dataset=dataset).inc(count)
    
    def record_search_query(self, dataset: str, latency: float):
        self.search_queries.labels(dataset=dataset).inc()
        self.search_latency.labels(dataset=dataset).observe(latency)
```

## Security Considerations

### Authentication & Authorization

1. **API Key Authentication**:
   - Support for API key-based authentication
   - Rate limiting per API key
   - Scope-based permissions

2. **Tenant Isolation**:
   - Multi-tenant support with data isolation
   - Tenant-specific quotas and limits
   - Audit logging per tenant

3. **Data Protection**:
   - Encryption at rest for sensitive data
   - TLS encryption for all communications
   - Input validation and sanitization

### Implementation

```python
# app/middleware/auth.py
from fastapi import Request, HTTPException, Depends
from fastapi.security import HTTPBearer, HTTPAuthorizationCredentials
import jwt

security = HTTPBearer()

class AuthMiddleware:
    def __init__(self, secret_key: str):
        self.secret_key = secret_key
    
    async def __call__(self, request: Request, call_next):
        # Skip auth for health checks
        if request.url.path in ["/health", "/metrics"]:
            return await call_next(request)
        
        # Extract and validate token
        auth_header = request.headers.get("Authorization")
        if not auth_header:
            raise HTTPException(status_code=401, detail="Missing authorization header")
        
        try:
            token = auth_header.split(" ")[1]
            payload = jwt.decode(token, self.secret_key, algorithms=["HS256"])
            request.state.tenant_id = payload.get("tenant_id")
            request.state.user_id = payload.get("user_id")
        except jwt.InvalidTokenError:
            raise HTTPException(status_code=401, detail="Invalid token")
        
        return await call_next(request)
```

## Client Examples

### Python Client

```python
# docs/examples/python_client.py
import asyncio
import httpx
import grpc
from app.proto import deeplake_service_pb2, deeplake_service_pb2_grpc

class DeepLakeClient:
    def __init__(self, http_url: str = None, grpc_url: str = None):
        self.http_url = http_url
        self.grpc_url = grpc_url
        self.http_client = None
        self.grpc_channel = None
    
    async def __aenter__(self):
        if self.http_url:
            self.http_client = httpx.AsyncClient(base_url=self.http_url)
        if self.grpc_url:
            self.grpc_channel = grpc.aio.insecure_channel(self.grpc_url)
        return self
    
    async def __aexit__(self, exc_type, exc_val, exc_tb):
        if self.http_client:
            await self.http_client.aclose()
        if self.grpc_channel:
            await self.grpc_channel.close()
    
    async def create_dataset_http(self, name: str, dimensions: int, metric_type: str = "cosine"):
        """Create dataset via HTTP API"""
        response = await self.http_client.post("/api/v1/datasets", json={
            "name": name,
            "dimensions": dimensions,
            "metric_type": metric_type
        })
        return response.json()
    
    async def create_dataset_grpc(self, name: str, dimensions: int, metric_type: str = "cosine"):
        """Create dataset via gRPC API"""
        client = deeplake_service_pb2_grpc.DatasetServiceStub(self.grpc_channel)
        response = await client.CreateDataset(
            deeplake_service_pb2.CreateDatasetRequest(
                name=name,
                dimensions=dimensions,
                metric_type=metric_type
            )
        )
        return response
    
    async def insert_vectors_http(self, dataset_id: str, vectors: list):
        """Insert vectors via HTTP API"""
        response = await self.http_client.post(
            f"/api/v1/datasets/{dataset_id}/vectors",
            json={"vectors": vectors}
        )
        return response.json()
    
    async def search_vectors_http(self, dataset_id: str, query_vector: list, top_k: int = 10):
        """Search vectors via HTTP API"""
        response = await self.http_client.post(
            f"/api/v1/datasets/{dataset_id}/search",
            json={
                "query_vector": query_vector,
                "options": {"top_k": top_k}
            }
        )
        return response.json()

# Usage example
async def main():
    async with DeepLakeClient(http_url="http://localhost:8000") as client:
        # Create dataset
        dataset = await client.create_dataset_http("my-dataset", 1536)
        print(f"Created dataset: {dataset['name']}")
        
        # Insert vectors
        vectors = [
            {
                "id": "vec1",
                "document_id": "doc1",
                "values": [0.1] * 1536,
                "content": "Hello world"
            }
        ]
        result = await client.insert_vectors_http("my-dataset", vectors)
        print(f"Inserted {result['inserted_count']} vectors")
        
        # Search
        query_vector = [0.1] * 1536
        results = await client.search_vectors_http("my-dataset", query_vector)
        print(f"Found {len(results['results'])} similar vectors")

if __name__ == "__main__":
    asyncio.run(main())
```

### Go Client

```go
// docs/examples/go_client.go
package main

import (
    "context"
    "fmt"
    "log"
    "google.golang.org/grpc"
    pb "github.com/yourorg/deeplake-vector-service/proto/deeplake/v1"
)

func main() {
    // Connect to gRPC server
    conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer conn.Close()
    
    // Create clients
    datasetClient := pb.NewDatasetServiceClient(conn)
    vectorClient := pb.NewVectorServiceClient(conn)
    searchClient := pb.NewSearchServiceClient(conn)
    
    ctx := context.Background()
    
    // Create dataset
    datasetResp, err := datasetClient.CreateDataset(ctx, &pb.CreateDatasetRequest{
        Name:       "go-example",
        Dimensions: 128,
        MetricType: "cosine",
    })
    if err != nil {
        log.Fatalf("Failed to create dataset: %v", err)
    }
    fmt.Printf("Created dataset: %s\n", datasetResp.Dataset.Name)
    
    // Insert vectors
    vectors := []*pb.Vector{
        {
            Id:         "vec1",
            DocumentId: "doc1",
            Values:     make([]float32, 128),
            Content:    "Hello from Go!",
        },
    }
    
    insertResp, err := vectorClient.InsertVectors(ctx, &pb.InsertVectorsRequest{
        DatasetId: "go-example",
        Vectors:   vectors,
    })
    if err != nil {
        log.Fatalf("Failed to insert vectors: %v", err)
    }
    fmt.Printf("Inserted %d vectors\n", insertResp.InsertedCount)
    
    // Search vectors
    queryVector := make([]float32, 128)
    searchResp, err := searchClient.SearchVectors(ctx, &pb.SearchVectorsRequest{
        DatasetId:   "go-example",
        QueryVector: queryVector,
        Options: &pb.SearchOptions{
            TopK: 5,
        },
    })
    if err != nil {
        log.Fatalf("Failed to search vectors: %v", err)
    }
    fmt.Printf("Found %d similar vectors\n", len(searchResp.Results))
}
```

## Implementation Timeline

### Phase 1: Core Service (Weeks 1-2)
- [ ] Repository setup and structure
- [ ] Core Deep Lake service implementation
- [ ] Basic HTTP API (datasets, vectors, search)
- [ ] Docker configuration
- [ ] Unit tests

### Phase 2: gRPC Implementation (Weeks 3-4)
- [ ] Protocol buffer definitions
- [ ] gRPC server implementation
- [ ] gRPC client examples
- [ ] Integration tests
- [ ] Performance benchmarks

### Phase 3: Production Features (Weeks 5-6)
- [ ] Authentication and authorization
- [ ] Rate limiting and quotas
- [ ] Monitoring and metrics
- [ ] Kubernetes deployment
- [ ] Documentation and examples

### Phase 4: AudiModal Integration (Weeks 7-8)
- [ ] Go gRPC client implementation
- [ ] Replace mock Deep Lake implementation
- [ ] Integration testing
- [ ] Performance optimization
- [ ] Production deployment

## Success Criteria

1. **Functional Requirements**:
   - ✅ Complete Deep Lake API coverage
   - ✅ Both HTTP and gRPC protocols working
   - ✅ Multi-tenant support
   - ✅ Authentication and authorization
   - ✅ Comprehensive error handling

2. **Performance Requirements**:
   - ✅ Search latency < 100ms for datasets with 1M+ vectors
   - ✅ Support for 1000+ concurrent requests
   - ✅ Vector insertion rate > 10k vectors/second
   - ✅ Memory usage < 2GB for typical workloads

3. **Operational Requirements**:
   - ✅ Health checks and monitoring
   - ✅ Graceful shutdown
   - ✅ Docker and Kubernetes deployment
   - ✅ Comprehensive logging
   - ✅ Prometheus metrics

4. **Integration Requirements**:
   - ✅ Seamless replacement of mock implementation
   - ✅ Backward compatibility with existing APIs
   - ✅ No breaking changes to AudiModal
   - ✅ Production-ready deployment

## Conclusion

This specification provides a complete blueprint for implementing a production-ready Deep Lake Vector Service. The service will be:

- **Universal**: Works with any programming language
- **Scalable**: Handles production workloads
- **Maintainable**: Clean architecture and comprehensive tests
- **Secure**: Authentication, authorization, and data protection
- **Observable**: Full monitoring and metrics

The implementation will seamlessly replace the current mock implementation in AudiModal while providing a reusable service for the broader community.

---

**Next Steps**: Create the new repository and begin Phase 1 implementation using this specification as the guide.