# AudiModal.ai Developer Documentation

## Version: 1.8.0 - ML/AI Classification Platform
**Last Updated**: January 29, 2025  
**Target Audience**: Developers, DevOps Engineers, System Integrators

---

## 📚 **Table of Contents**

1. [Platform Overview](#platform-overview)
2. [Architecture & Design](#architecture--design)
3. [Getting Started](#getting-started)
4. [Core Components](#core-components)
5. [ML/AI Classification Platform](#mlai-classification-platform)
6. [API Reference](#api-reference)
7. [Data Models & Schemas](#data-models--schemas)
8. [Configuration Management](#configuration-management)
9. [Development Workflow](#development-workflow)
10. [Testing & Quality Assurance](#testing--quality-assurance)
11. [Deployment & Operations](#deployment--operations)
12. [Performance Optimization](#performance-optimization)
13. [Security & Compliance](#security--compliance)
14. [Troubleshooting](#troubleshooting)
15. [Contributing Guidelines](#contributing-guidelines)

---

## 📖 **Platform Overview**

### **What is AudiModal.ai?**

AudiModal.ai is a comprehensive AI-powered document processing and analytics platform that transforms unstructured document data into actionable business intelligence. The platform combines advanced machine learning, semantic search, knowledge graphs, and predictive analytics to provide enterprise-grade document intelligence.

### **Core Capabilities**

```mermaid
graph TB
    A[Document Ingestion] --> B[Processing Pipeline]
    B --> C[ML/AI Classification]
    C --> D[Knowledge Graph]
    D --> E[Semantic Search]
    E --> F[Insights & Analytics]
    F --> G[Predictive Models]
    G --> H[Enterprise APIs]
```

### **Technology Stack**

| Component | Technology | Version | Purpose |
|-----------|------------|---------|---------|
| **Runtime** | Go | 1.23.0+ | Core platform language |
| **Database** | PostgreSQL | 13+ | Primary data storage |
| **Cache** | Redis | 6+ | Caching and session storage |
| **Message Queue** | Apache Kafka | 2.8+ | Streaming and event processing |
| **Vector Store** | DeepLake | Latest | Vector embeddings storage |
| **Monitoring** | OpenTelemetry | Latest | Distributed tracing |
| **Search** | Custom Engine | N/A | Semantic search capabilities |
| **Container** | Docker | 20.10+ | Containerization |
| **Orchestration** | Kubernetes | 1.24+ | Container orchestration |

---

## 🏗️ **Architecture & Design**

### **High-Level Architecture**

```mermaid
graph TB
    subgraph "Client Layer"
        WEB[Web Dashboard]
        API[REST APIs]
        SDK[Client SDKs]
    end
    
    subgraph "API Gateway"
        GW[API Gateway]
        AUTH[Authentication]
        RATE[Rate Limiting]
    end
    
    subgraph "Core Services"
        DOC[Document Service]
        PROC[Processing Service]
        SEARCH[Search Service]
        ML[ML/AI Service]
    end
    
    subgraph "ML/AI Platform"
        TRAIN[Model Training]
        PRED[Predictions]
        KG[Knowledge Graph]
        INS[Insights Engine]
    end
    
    subgraph "Data Layer"
        PG[(PostgreSQL)]
        REDIS[(Redis)]
        KAFKA[(Kafka)]
        DL[(DeepLake)]
    end
    
    WEB --> GW
    API --> GW
    SDK --> GW
    GW --> DOC
    GW --> PROC
    GW --> SEARCH
    GW --> ML
    ML --> TRAIN
    ML --> PRED
    ML --> KG
    ML --> INS
    DOC --> PG
    PROC --> KAFKA
    SEARCH --> DL
    ML --> PG
```

### **Service Architecture Patterns**

#### **1. Microservices Architecture**
- **Domain-Driven Design**: Services organized by business capability
- **Event-Driven Communication**: Asynchronous messaging via Kafka
- **API-First Design**: RESTful APIs with OpenAPI specifications
- **Polyglot Persistence**: Different storage solutions for different needs

#### **2. Layered Architecture**
```
┌─────────────────────────────────────┐
│           Presentation Layer        │  ← REST APIs, Web UI, Client SDKs
├─────────────────────────────────────┤
│            Business Layer           │  ← Core business logic, ML/AI services
├─────────────────────────────────────┤
│           Integration Layer         │  ← External connectors, message queues
├─────────────────────────────────────┤
│           Persistence Layer         │  ← Databases, file storage, caching
└─────────────────────────────────────┘
```

#### **3. Event-Driven Architecture**
- **Event Sourcing**: Document processing events stored for audit
- **CQRS Pattern**: Separate read/write models for optimization
- **Message Streaming**: Real-time event processing with Kafka
- **Webhook Integration**: External system notifications

---

## 🚀 **Getting Started**

### **Prerequisites**

#### **Development Environment**
```bash
# Required tools
go version      # Go 1.23.0+
docker --version    # Docker 20.10+
kubectl version     # Kubernetes CLI
git --version       # Git version control

# Optional tools
make --version      # Build automation
golangci-lint --version  # Code linting
```

#### **Runtime Dependencies**
```bash
# Database
postgresql://user:pass@localhost:5432/audimodal

# Cache
redis://localhost:6379

# Message Queue
kafka://localhost:9092

# Vector Store
http://localhost:8000  # DeepLake API
```

### **Quick Start**

#### **1. Clone Repository**
```bash
git clone https://github.com/audimodal/audimodal.git
cd audimodal
```

#### **2. Environment Setup**
```bash
# Copy environment template
cp .env.example .env

# Configure environment variables
export AUDIMODAL_ENV=development
export AUDIMODAL_DB_URL=postgresql://user:pass@localhost:5432/audimodal
export DEEPLAKE_API_URL=http://localhost:8000
export DEEPLAKE_API_KEY=your-api-key
```

#### **3. Build and Run**
```bash
# Install dependencies
go mod download

# Build the application
make build

# Run database migrations
make migrate

# Start the server
make run

# Or use Docker Compose
docker-compose up -d
```

#### **4. Verify Installation**
```bash
# Health check
curl http://localhost:8080/health

# API documentation
curl http://localhost:8080/api/v1/docs

# Web dashboard
open http://localhost:8080/dashboard
```

### **Development Setup**

#### **IDE Configuration**

**VS Code Settings (`.vscode/settings.json`)**:
```json
{
    "go.lintTool": "golangci-lint",
    "go.lintFlags": ["--fast"],
    "go.testFlags": ["-v", "-race"],
    "go.buildTags": "integration",
    "editor.formatOnSave": true,
    "go.useLanguageServer": true
}
```

**GoLand Configuration**:
- Enable Go modules integration
- Configure golangci-lint as external tool
- Set up run configurations for different environments

---

## 🔧 **Core Components**

### **1. Document Processing Pipeline**

#### **Processing Tiers**
```go
type ProcessingTier int

const (
    Tier1 ProcessingTier = 1  // Files < 10MB
    Tier2 ProcessingTier = 2  // Files 10MB - 1GB
    Tier3 ProcessingTier = 3  // Files > 1GB
)
```

#### **File Readers**
```
pkg/readers/
├── pdf/           # PDF processing with OCR
├── office/        # Microsoft Office formats
├── email/         # Email and PST archives
├── image/         # Image processing with OCR
├── archive/       # ZIP and archive extraction
└── text/          # Plain text formats
```

#### **Processing Flow**
```mermaid
graph LR
    A[File Upload] --> B[Format Detection]
    B --> C[Tier Assignment]
    C --> D[Content Extraction]
    D --> E[Chunking Strategy]
    E --> F[Embedding Generation]
    F --> G[Vector Storage]
```

### **2. Enterprise Connectors**

#### **Supported Platforms**
| Connector | Protocol | Authentication | Real-time Sync |
|-----------|----------|----------------|----------------|
| **SharePoint** | Microsoft Graph API | OAuth 2.0 | ✅ Webhooks |
| **Google Drive** | Google Drive API | OAuth 2.0 | ✅ Webhooks |
| **Box** | Box API | OAuth 2.0/JWT | ✅ Webhooks |
| **Dropbox** | Dropbox API | OAuth 2.0 | ✅ Long Polling |
| **OneDrive** | Microsoft Graph API | OAuth 2.0 | ✅ Delta API |
| **Confluence** | Atlassian REST API | OAuth 1.0/2.0 | ✅ Webhooks |
| **Slack** | Slack API | Bot Token | ✅ Events API |
| **Notion** | Notion API | OAuth 2.0 | ✅ Webhooks |

#### **Connector Architecture**
```go
type Connector interface {
    Connect(ctx context.Context, config ConnectorConfig) error
    Sync(ctx context.Context, options SyncOptions) (*SyncResult, error)
    ListFiles(ctx context.Context, filter FileFilter) ([]RemoteFile, error)
    DownloadFile(ctx context.Context, fileID string) (*FileContent, error)
    HandleWebhook(ctx context.Context, payload []byte) error
}
```

### **3. Unified Sync Framework**

#### **Sync Orchestration**
```go
type SyncOrchestrator struct {
    config         *SyncConfig
    jobManager     *JobManager
    conflictResolver *ConflictResolver
    scheduler      *Scheduler
    metricsCollector *MetricsCollector
}
```

#### **Conflict Resolution Strategies**
```go
type ResolutionStrategy string

const (
    LastWriteWins  ResolutionStrategy = "last_write_wins"
    PreserveBoth   ResolutionStrategy = "preserve_both"
    SourcePriority ResolutionStrategy = "source_priority"
    MergeStrategy  ResolutionStrategy = "merge"
    ManualResolve  ResolutionStrategy = "manual"
)
```

### **4. Anomaly Detection System**

#### **Detection Types**
```go
type AnomalyType string

const (
    StatisticalAnomaly AnomalyType = "statistical"
    ContentAnomaly     AnomalyType = "content"
    BehavioralAnomaly  AnomalyType = "behavioral"
    SecurityAnomaly    AnomalyType = "security"
)
```

#### **Detection Algorithms**
- **Statistical**: Z-score, IQR, Modified Z-score
- **Content**: Sentiment analysis, quality scoring, duplicate detection
- **Behavioral**: User profiling, access pattern analysis
- **Security**: DLP rules, malware indicators, threat intelligence

---

## 🤖 **ML/AI Classification Platform**

### **1. Model Training Framework**

#### **Training Architecture**
```go
type ModelTrainer struct {
    config         *TrainingConfig
    trainingJobs   map[uuid.UUID]*TrainingJob
    models         map[string]*TrainedModel
    datasetManager *DatasetManager
    jobQueue       chan *TrainingJob
    workers        []*TrainingWorker
}
```

#### **Supported Model Types**
```go
type ModelType string

const (
    ClassificationModel  ModelType = "classification"
    RegressionModel      ModelType = "regression"
    ClusteringModel      ModelType = "clustering"
    EmbeddingModel       ModelType = "embedding"
    AnomalyDetectionModel ModelType = "anomaly_detection"
)
```

#### **Training Job Lifecycle**
```mermaid
graph LR
    A[Job Created] --> B[Queued]
    B --> C[Dataset Prepared]
    C --> D[Training Started]
    D --> E[Model Training]
    E --> F[Validation]
    F --> G[Model Registered]
    G --> H[Deployment Ready]
```

#### **Dataset Management**
```go
type Dataset struct {
    ID              uuid.UUID        `json:"id"`
    Name            string           `json:"name"`
    DatasetType     DatasetType      `json:"dataset_type"`
    Format          DatasetFormat    `json:"format"`
    RecordCount     int              `json:"record_count"`
    QualityScore    float64          `json:"quality_score"`
    Statistics      *DatasetStatistics `json:"statistics"`
}
```

### **2. Model Registry & Versioning**

#### **Model Registry Architecture**
```go
type ModelRegistry struct {
    models    map[string]*ModelEntry
    versions  map[string]map[string]*ModelVersion
    aliases   map[string]*ModelAlias
}
```

#### **A/B Testing Framework**
```go
type ABTestExperiment struct {
    ID                 uuid.UUID           `json:"id"`
    ControlVersion     string              `json:"control_version"`
    TreatmentVersions  []TreatmentVersion  `json:"treatment_versions"`
    TrafficSplit      TrafficSplitConfig  `json:"traffic_split"`
    PrimaryMetric     ExperimentMetric    `json:"primary_metric"`
    Results           *ExperimentResults  `json:"results"`
}
```

#### **Model Deployment Pipeline**
```mermaid
graph TB
    A[Model Training] --> B[Model Registration]
    B --> C[Version Creation]
    C --> D[Approval Process]
    D --> E[A/B Test Setup]
    E --> F[Traffic Splitting]
    F --> G[Performance Monitoring]
    G --> H[Production Deployment]
```

### **3. Predictive Analytics Engine**

#### **Prediction Types**
```go
type PredictionType string

const (
    DocumentLifecycle PredictionType = "document_lifecycle"
    AccessPatterns    PredictionType = "access_patterns"
    StorageOptimization PredictionType = "storage_optimization"
    UserBehavior      PredictionType = "user_behavior"
)
```

#### **Time Series Forecasting**
```go
type ForecastInput struct {
    SeriesType     TimeSeriesType  `json:"series_type"`
    HistoricalData []DataPoint     `json:"historical_data"`
    Horizon        time.Duration   `json:"horizon"`
    Frequency      string          `json:"frequency"`
}
```

### **4. Knowledge Graph & Content Intelligence**

#### **Knowledge Graph Structure**
```go
type KnowledgeGraph struct {
    nodes      map[uuid.UUID]*Node
    edges      map[uuid.UUID]*Edge
    indices    *GraphIndices
    processors map[string]EntityProcessor
    extractors map[string]RelationExtractor
}
```

#### **Entity Types**
```go
type NodeType string

const (
    DocumentNode     NodeType = "document"
    PersonNode       NodeType = "person"
    OrganizationNode NodeType = "organization"
    LocationNode     NodeType = "location"
    ConceptNode      NodeType = "concept"
    TopicNode        NodeType = "topic"
    KeywordNode      NodeType = "keyword"
)
```

#### **Relationship Mapping**
```go
type RelationshipMapper struct {
    knowledgeGraph   *KnowledgeGraph
    similarityEngine *SimilarityEngine
    clusteringEngine *ClusteringEngine
    relationshipCache map[string]*CachedRelationship
}
```

### **5. Semantic Search Engine**

#### **Search Architecture**
```go
type SemanticSearchEngine struct {
    embeddingEngine  *EmbeddingEngine
    indexEngine      *IndexEngine
    queryProcessor   *QueryProcessor
    rankingEngine    *RankingEngine
    documentIndex    map[uuid.UUID]*SearchableDocument
}
```

#### **Search Modes**
```go
type SearchMode string

const (
    ExactSearch      SearchMode = "exact"
    FuzzySearch      SearchMode = "fuzzy"
    SemanticSearch   SearchMode = "semantic"
    HybridSearch     SearchMode = "hybrid"
    ConceptualSearch SearchMode = "conceptual"
)
```

#### **Relevance Scoring**
```go
func (sse *SemanticSearchEngine) calculateRelevanceScore(
    doc *SearchableDocument, 
    query *SearchQuery, 
    queryEmbedding []float64,
) float64 {
    // Semantic similarity (60%)
    semanticScore := sse.calculateSemanticScore(doc, queryEmbedding)
    score += semanticScore * 0.6
    
    // Keyword matching (20%)
    keywordScore := sse.calculateKeywordScore(doc, query)
    score += keywordScore * 0.2
    
    // Popularity (10%)
    score += doc.PopularityScore * 0.1
    
    // Recency (10%)
    recencyScore := sse.calculateRecencyScore(doc)
    score += recencyScore * 0.1
    
    return score * doc.BoostFactor
}
```

### **6. Insights & Analytics Engine**

#### **Insight Types**
```go
type InsightType string

const (
    ContentInsight      InsightType = "content"
    UsageInsight        InsightType = "usage"
    TrendInsight        InsightType = "trend"
    AnomalyInsight      InsightType = "anomaly"
    RecommendationInsight InsightType = "recommendation"
    PredictionInsight   InsightType = "prediction"
)
```

#### **Report Generation**
```go
type DocumentReport struct {
    ID              uuid.UUID            `json:"id"`
    ReportType      ReportType           `json:"report_type"`
    Summary         *ReportSummary       `json:"summary"`
    DocumentMetrics *DocumentMetricsSection `json:"document_metrics"`
    UserMetrics     *UserMetricsSection  `json:"user_metrics"`
    ContentAnalysis *ContentAnalysisSection `json:"content_analysis"`
    TrendAnalysis   *TrendAnalysisSection `json:"trend_analysis"`
    Insights        []DocumentInsight    `json:"insights"`
}
```

---

## 🌐 **API Reference**

### **Authentication**

#### **JWT Bearer Token**
```bash
curl -H "Authorization: Bearer ${JWT_TOKEN}" \
     https://api.audimodal.ai/v1/files
```

#### **API Key**
```bash
curl -H "X-API-Key: ${API_KEY}" \
     https://api.audimodal.ai/v1/files
```

### **Core API Endpoints**

#### **Document Management**
```http
GET    /api/v1/files                    # List files
POST   /api/v1/files                    # Upload file
GET    /api/v1/files/{id}               # Get file details
DELETE /api/v1/files/{id}               # Delete file
POST   /api/v1/files/{id}/reprocess     # Reprocess file
```

#### **Search & Discovery**
```http
POST   /api/v1/search                   # Document search
GET    /api/v1/search/suggestions       # Search suggestions
POST   /api/v1/ml/search/semantic       # Semantic search
POST   /api/v1/ml/search/index          # Index document
```

#### **ML/AI Training**
```http
GET    /api/v1/ml/training/jobs         # List training jobs
POST   /api/v1/ml/training/jobs         # Create training job
GET    /api/v1/ml/training/jobs/{id}    # Get job details
POST   /api/v1/ml/training/jobs/{id}/cancel # Cancel job
```

#### **Model Registry**
```http
GET    /api/v1/ml/registry/models       # List models
POST   /api/v1/ml/registry/models       # Register model
GET    /api/v1/ml/registry/models/{name}/versions # List versions
```

#### **Predictions**
```http
POST   /api/v1/ml/predictions/predict   # Make prediction
POST   /api/v1/ml/predictions/forecast  # Time series forecast
POST   /api/v1/ml/predictions/lifecycle # Document lifecycle
```

#### **Knowledge Graph**
```http
GET    /api/v1/ml/knowledge-graph/entities     # List entities
POST   /api/v1/ml/knowledge-graph/entities     # Create entity
POST   /api/v1/ml/knowledge-graph/query        # Graph query
```

#### **Insights & Analytics**
```http
POST   /api/v1/ml/insights/generate     # Generate insights
GET    /api/v1/ml/insights              # List insights
POST   /api/v1/ml/insights/reports      # Generate report
GET    /api/v1/ml/insights/reports      # List reports
```

### **Request/Response Examples**

#### **Upload File**
```bash
curl -X POST \
  -H "Authorization: Bearer ${JWT_TOKEN}" \
  -F "file=@document.pdf" \
  -F "metadata={\"title\":\"Sample Document\",\"tags\":[\"important\"]}" \
  https://api.audimodal.ai/v1/files
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "document.pdf",
  "size": 1024000,
  "processing_status": "pending",
  "created_at": "2025-01-29T10:00:00Z"
}
```

#### **Semantic Search**
```bash
curl -X POST \
  -H "Authorization: Bearer ${JWT_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "machine learning algorithms",
    "search_mode": "hybrid",
    "limit": 20
  }' \
  https://api.audimodal.ai/v1/ml/search/semantic
```

**Response:**
```json
{
  "query_id": "123e4567-e89b-12d3-a456-426614174000",
  "total_results": 45,
  "results": [
    {
      "document": {
        "id": "550e8400-e29b-41d4-a716-446655440001",
        "title": "Introduction to Machine Learning",
        "content": "Machine learning algorithms are...",
        "document_type": "pdf"
      },
      "score": 0.95,
      "semantic_score": 0.92,
      "keyword_score": 0.87
    }
  ],
  "execution_time": "45ms"
}
```

---

## 📊 **Data Models & Schemas**

### **Core Entities**

#### **Tenant**
```go
type Tenant struct {
    ID        uuid.UUID `gorm:"primaryKey" json:"id"`
    Name      string    `gorm:"not null" json:"name"`
    Domain    string    `gorm:"unique;not null" json:"domain"`
    Status    TenantStatus `json:"status"`
    Settings  JSON      `json:"settings"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

#### **DataSource**
```go
type DataSource struct {
    ID           uuid.UUID    `gorm:"primaryKey" json:"id"`
    TenantID     uuid.UUID    `gorm:"not null;index" json:"tenant_id"`
    Name         string       `gorm:"not null" json:"name"`
    Type         SourceType   `gorm:"not null" json:"type"`
    Status       SourceStatus `json:"status"`
    Config       JSON         `json:"config"`
    SyncSchedule JSON         `json:"sync_schedule"`
    LastSync     *time.Time   `json:"last_sync"`
    CreatedAt    time.Time    `json:"created_at"`
    UpdatedAt    time.Time    `json:"updated_at"`
}
```

#### **File**
```go
type File struct {
    ID                uuid.UUID      `gorm:"primaryKey" json:"id"`
    TenantID          uuid.UUID      `gorm:"not null;index" json:"tenant_id"`
    DataSourceID      uuid.UUID      `gorm:"index" json:"datasource_id"`
    Name              string         `gorm:"not null" json:"name"`
    Path              string         `json:"path"`
    Size              int64          `json:"size"`
    MimeType          string         `json:"mime_type"`
    FileType          FileType       `json:"file_type"`
    Checksum          string         `gorm:"index" json:"checksum"`
    ProcessingStatus  ProcessingStatus `json:"processing_status"`
    ProcessingTier    int            `json:"processing_tier"`
    ChunkCount        int            `json:"chunk_count"`
    EmbeddingStatus   EmbeddingStatus `json:"embedding_status"`
    Metadata          JSON           `json:"metadata"`
    Tags              pq.StringArray `gorm:"type:text[]" json:"tags"`
    CreatedAt         time.Time      `json:"created_at"`
    UpdatedAt         time.Time      `json:"updated_at"`
    ProcessedAt       *time.Time     `json:"processed_at"`
}
```

### **ML/AI Entities**

#### **Training Job**
```go
type TrainingJob struct {
    ID              uuid.UUID       `gorm:"primaryKey" json:"id"`
    TenantID        uuid.UUID       `gorm:"not null;index" json:"tenant_id"`
    Name            string          `gorm:"not null" json:"name"`
    Description     string          `json:"description"`
    ModelType       ModelType       `gorm:"not null" json:"model_type"`
    DatasetID       uuid.UUID       `json:"dataset_id"`
    Status          JobStatus       `json:"status"`
    Progress        float64         `json:"progress"`
    Configuration   JSON            `json:"configuration"`
    Parameters      JSON            `json:"parameters"`
    Metrics         JSON            `json:"metrics"`
    ErrorMessage    string          `json:"error_message"`
    CreatedAt       time.Time       `json:"created_at"`
    StartedAt       *time.Time      `json:"started_at"`
    CompletedAt     *time.Time      `json:"completed_at"`
}
```

#### **Model Version**
```go
type ModelVersion struct {
    ID                 uuid.UUID        `gorm:"primaryKey" json:"id"`
    ModelName          string           `gorm:"not null;index" json:"model_name"`
    Version            string           `gorm:"not null" json:"version"`
    Status             VersionStatus    `json:"status"`
    TrainingJobID      uuid.UUID        `json:"training_job_id"`
    Artifacts          JSON             `json:"artifacts"`
    ValidationMetrics  JSON             `json:"validation_metrics"`
    DeploymentInfo     JSON             `json:"deployment_info"`
    ApprovalStatus     ApprovalStatus   `json:"approval_status"`
    CreatedAt          time.Time        `json:"created_at"`
    CreatedBy          uuid.UUID        `json:"created_by"`
}
```

### **Database Schema**

#### **Core Tables**
```sql
-- Tenants table
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    domain VARCHAR(255) UNIQUE NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    settings JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Data sources table
CREATE TABLE datasources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'inactive',
    config JSONB,
    sync_schedule JSONB,
    last_sync TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Files table
CREATE TABLE files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    datasource_id UUID REFERENCES datasources(id),
    name VARCHAR(1000) NOT NULL,
    path TEXT,
    size BIGINT,
    mime_type VARCHAR(255),
    file_type VARCHAR(50),
    checksum VARCHAR(64),
    processing_status VARCHAR(50) DEFAULT 'pending',
    processing_tier INTEGER,
    chunk_count INTEGER DEFAULT 0,
    embedding_status VARCHAR(50) DEFAULT 'pending',
    metadata JSONB,
    tags TEXT[],
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    processed_at TIMESTAMP
);
```

#### **ML/AI Tables**
```sql
-- Training jobs table
CREATE TABLE training_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    model_type VARCHAR(50) NOT NULL,
    dataset_id UUID,
    status VARCHAR(50) DEFAULT 'pending',
    progress FLOAT DEFAULT 0.0,
    configuration JSONB,
    parameters JSONB,
    metrics JSONB,
    error_message TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    started_at TIMESTAMP,
    completed_at TIMESTAMP
);

-- Model versions table
CREATE TABLE model_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    model_name VARCHAR(255) NOT NULL,
    version VARCHAR(100) NOT NULL,
    status VARCHAR(50) DEFAULT 'draft',
    training_job_id UUID REFERENCES training_jobs(id),
    artifacts JSONB,
    validation_metrics JSONB,
    deployment_info JSONB,
    approval_status VARCHAR(50) DEFAULT 'pending',
    created_at TIMESTAMP DEFAULT NOW(),
    created_by UUID
);
```

#### **Indexes**
```sql
-- Performance indexes
CREATE INDEX idx_files_tenant_id ON files(tenant_id);
CREATE INDEX idx_files_datasource_id ON files(datasource_id);
CREATE INDEX idx_files_processing_status ON files(processing_status);
CREATE INDEX idx_files_checksum ON files(checksum);
CREATE INDEX idx_training_jobs_tenant_id ON training_jobs(tenant_id);
CREATE INDEX idx_training_jobs_status ON training_jobs(status);
CREATE INDEX idx_model_versions_model_name ON model_versions(model_name);

-- Composite indexes
CREATE INDEX idx_files_tenant_status ON files(tenant_id, processing_status);
CREATE INDEX idx_training_jobs_tenant_status ON training_jobs(tenant_id, status);
```

---

## ⚙️ **Configuration Management**

### **Configuration Structure**

#### **Server Configuration (`config/server.yaml`)**
```yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: "30s"
  write_timeout: "30s"
  idle_timeout: "120s"

database:
  host: "localhost"
  port: 5432
  name: "audimodal"
  user: "audimodal"
  password: "${DATABASE_PASSWORD}"
  ssl_mode: "prefer"
  max_open_conns: 100
  max_idle_conns: 10

redis:
  host: "localhost"
  port: 6379
  password: "${REDIS_PASSWORD}"
  db: 0
  pool_size: 100

kafka:
  brokers:
    - "localhost:9092"
  consumer_group: "audimodal-consumers"
  topics:
    file_processing: "file-processing"
    document_events: "document-events"

deeplake_api:
  base_url: "${DEEPLAKE_API_URL}"
  api_key: "${DEEPLAKE_API_KEY}"
  timeout: "30s"
  max_retries: 3
  retry_delay: "1s"

ml_ai:
  enabled: true
  model_training:
    max_concurrent_jobs: 5
    worker_pool_size: 10
    default_timeout: "1h"
    resource_limits:
      memory: "8Gi"
      cpu: "4"
  
  semantic_search:
    embedding_model: "sentence-transformers/all-MiniLM-L6-v2"
    vector_dimensions: 384
    similarity_threshold: 0.5
    max_results: 100
    cache_size: 1000
    cache_ttl: "1h"
  
  knowledge_graph:
    max_nodes: 100000
    max_edges: 500000
    indexing_enabled: true
    semantic_search_enabled: true
  
  insights:
    analysis_interval: "1h"
    retention_period: "30d"
    confidence_threshold: 0.7
    max_insights_per_category: 20

monitoring:
  enabled: true
  metrics_port: 9090
  health_check_interval: "30s"
  tracing:
    enabled: true
    endpoint: "http://jaeger:14268/api/traces"
    service_name: "audimodal-api"

logging:
  level: "info"
  format: "json"
  output: "stdout"
  structured: true
```

### **Environment Variables**

#### **Core Configuration**
```bash
# Server
AUDIMODAL_ENV=production
AUDIMODAL_HOST=0.0.0.0
AUDIMODAL_PORT=8080

# Database
DATABASE_URL=postgresql://user:pass@localhost:5432/audimodal
DATABASE_PASSWORD=secure_password
DATABASE_MAX_CONNECTIONS=100

# Cache
REDIS_URL=redis://localhost:6379
REDIS_PASSWORD=redis_password

# Message Queue
KAFKA_BROKERS=localhost:9092
KAFKA_CONSUMER_GROUP=audimodal-consumers

# DeepLake API
DEEPLAKE_API_URL=http://localhost:8000
DEEPLAKE_API_KEY=dl_api_key_123

# Authentication
JWT_SECRET=super_secret_jwt_key
JWT_EXPIRATION=24h
API_KEY_HEADER=X-API-Key

# ML/AI Configuration
ML_AI_ENABLED=true
ML_TRAINING_MAX_JOBS=5
ML_TRAINING_TIMEOUT=3600
SEMANTIC_SEARCH_MODEL=sentence-transformers/all-MiniLM-L6-v2
KNOWLEDGE_GRAPH_MAX_NODES=100000

# Monitoring
METRICS_ENABLED=true
METRICS_PORT=9090
TRACING_ENABLED=true
TRACING_ENDPOINT=http://jaeger:14268/api/traces

# Security
TLS_ENABLED=false
TLS_CERT_FILE=/etc/ssl/certs/audimodal.crt
TLS_KEY_FILE=/etc/ssl/private/audimodal.key
```

### **Configuration Loading**
```go
type Config struct {
    Server     ServerConfig     `yaml:"server"`
    Database   DatabaseConfig   `yaml:"database"`
    Redis      RedisConfig      `yaml:"redis"`
    Kafka      KafkaConfig      `yaml:"kafka"`
    DeepLakeAPI DeepLakeAPIConfig `yaml:"deeplake_api"`
    MLAI       MLAIConfig       `yaml:"ml_ai"`
    Monitoring MonitoringConfig `yaml:"monitoring"`
    Logging    LoggingConfig    `yaml:"logging"`
}

func LoadConfig(configPath string) (*Config, error) {
    config := &Config{}
    
    // Load from file
    if configPath != "" {
        data, err := ioutil.ReadFile(configPath)
        if err != nil {
            return nil, err
        }
        
        if err := yaml.Unmarshal(data, config); err != nil {
            return nil, err
        }
    }
    
    // Override with environment variables
    if err := env.Parse(config); err != nil {
        return nil, err
    }
    
    // Validate configuration
    if err := config.Validate(); err != nil {
        return nil, err
    }
    
    return config, nil
}
```

---

## 🔄 **Development Workflow**

### **Git Workflow**

#### **Branch Strategy**
```
main                    # Production-ready code
├── develop            # Integration branch
├── feature/*          # Feature development
├── bugfix/*           # Bug fixes
├── release/*          # Release preparation
└── hotfix/*           # Production hotfixes
```

#### **Commit Message Format**
```
type(scope): description

feat(ml): add semantic search engine
fix(api): resolve authentication issue
docs(readme): update installation guide
test(unit): add model training tests
refactor(search): optimize query processing
```

### **Development Process**

#### **1. Feature Development**
```bash
# Create feature branch
git checkout -b feature/new-ml-capability

# Development cycle
make test           # Run tests
make lint           # Code linting
make build          # Build application

# Commit changes
git add .
git commit -m "feat(ml): implement new capability"

# Push and create PR
git push origin feature/new-ml-capability
gh pr create --title "Add new ML capability"
```

#### **2. Code Review Process**
- **Automated Checks**: CI/CD pipeline runs tests, linting, security scans
- **Peer Review**: At least one approval required from team member
- **Documentation**: Update documentation for API changes
- **Testing**: Ensure adequate test coverage (>80%)

#### **3. Integration Testing**
```bash
# Run full test suite
make test-integration

# Run specific ML/AI tests
make test-ml

# Performance testing
make bench

# Security scanning
make security-scan
```

### **Build System**

#### **Makefile Targets**
```makefile
# Development
.PHONY: dev
dev: build run

.PHONY: build
build:
	go build -o bin/audimodal ./cmd/server

.PHONY: run
run:
	./bin/audimodal

# Testing
.PHONY: test
test:
	go test -v -race ./...

.PHONY: test-integration
test-integration:
	go test -v -tags=integration ./...

.PHONY: test-ml
test-ml:
	go test -v ./pkg/analysis/...

# Code Quality
.PHONY: lint
lint:
	golangci-lint run

.PHONY: fmt
fmt:
	go fmt ./...

# Docker
.PHONY: docker-build
docker-build:
	docker build -t audimodal:latest .

.PHONY: docker-run
docker-run:
	docker-compose up -d
```

### **Continuous Integration**

#### **GitHub Actions Workflow (`.github/workflows/ci.yml`)**
```yaml
name: CI/CD Pipeline

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main, develop]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:13
        env:
          POSTGRES_PASSWORD: postgres
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5

    steps:
    - uses: actions/checkout@v4
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.23'
    
    - name: Cache dependencies
      uses: actions/cache@v3
      with:
        path: ~/go/pkg/mod
        key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
    
    - name: Install dependencies
      run: go mod download
    
    - name: Run tests
      run: make test
    
    - name: Run linting
      uses: golangci/golangci-lint-action@v3
    
    - name: Run security scan
      uses: securecodewarrior/github-action-add-sarif@v1
      with:
        sarif-file: 'security-scan-results.sarif'
    
    - name: Build application
      run: make build
    
    - name: Run integration tests
      run: make test-integration
      env:
        DATABASE_URL: postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable

  build-and-push:
    needs: test
    runs-on: ubuntu-latest
    if: github.ref == 'refs/heads/main'
    
    steps:
    - uses: actions/checkout@v4
    
    - name: Build and push Docker image
      uses: docker/build-push-action@v5
      with:
        context: .
        push: true
        tags: |
          audimodal/api:latest
          audimodal/api:${{ github.sha }}
```

---

## 🧪 **Testing & Quality Assurance**

### **Testing Strategy**

#### **Test Pyramid**
```
    ┌─────────────┐
    │  E2E Tests  │  ← Integration tests, API tests
    ├─────────────┤
    │ Integration │  ← Service tests, database tests
    ├─────────────┤
    │ Unit Tests  │  ← Function tests, logic tests
    └─────────────┘
```

### **Unit Testing**

#### **Test Structure**
```go
func TestModelTrainer_StartTraining(t *testing.T) {
    tests := []struct {
        name    string
        job     *TrainingJob
        want    uuid.UUID
        wantErr bool
    }{
        {
            name: "successful training job creation",
            job: &TrainingJob{
                Name:      "test-model",
                ModelType: ClassificationModel,
                DatasetID: uuid.New(),
            },
            wantErr: false,
        },
        {
            name: "invalid model type",
            job: &TrainingJob{
                Name:      "test-model",
                ModelType: "invalid",
            },
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            trainer := NewModelTrainer(defaultConfig())
            
            jobID, err := trainer.StartTraining(context.Background(), tt.job)
            
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            
            assert.NoError(t, err)
            assert.NotEqual(t, uuid.Nil, jobID)
        })
    }
}
```

#### **Mock Implementations**
```go
type MockEmbeddingEngine struct {
    embeddings map[string][]float64
}

func (m *MockEmbeddingEngine) GenerateEmbedding(ctx context.Context, text string) ([]float64, error) {
    if embedding, exists := m.embeddings[text]; exists {
        return embedding, nil
    }
    
    // Generate mock embedding
    embedding := make([]float64, 384)
    for i := range embedding {
        embedding[i] = rand.Float64()
    }
    
    m.embeddings[text] = embedding
    return embedding, nil
}
```

### **Integration Testing**

#### **Database Testing**
```go
func TestTrainingJobRepository_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }
    
    db := setupTestDB(t)
    defer teardownTestDB(t, db)
    
    repo := NewTrainingJobRepository(db)
    
    job := &TrainingJob{
        Name:      "integration-test",
        ModelType: ClassificationModel,
        Status:    JobStatusPending,
    }
    
    // Test create
    err := repo.Create(context.Background(), job)
    assert.NoError(t, err)
    assert.NotEqual(t, uuid.Nil, job.ID)
    
    // Test get
    retrieved, err := repo.GetByID(context.Background(), job.ID)
    assert.NoError(t, err)
    assert.Equal(t, job.Name, retrieved.Name)
}
```

#### **API Testing**
```go
func TestMLHandlers_CreateTrainingJob_Integration(t *testing.T) {
    app := setupTestApp(t)
    defer teardownTestApp(t, app)
    
    requestBody := `{
        "name": "test-job",
        "model_type": "classification",
        "dataset_id": "550e8400-e29b-41d4-a716-446655440001"
    }`
    
    req := httptest.NewRequest("POST", "/ml/training/jobs", strings.NewReader(requestBody))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+testJWT)
    
    resp, err := app.Test(req)
    require.NoError(t, err)
    
    assert.Equal(t, http.StatusCreated, resp.StatusCode)
    
    var response map[string]interface{}
    err = json.NewDecoder(resp.Body).Decode(&response)
    require.NoError(t, err)
    
    assert.Contains(t, response, "job_id")
    assert.Equal(t, "started", response["status"])
}
```

### **Performance Testing**

#### **Benchmark Tests**
```go
func BenchmarkSemanticSearch_Search(b *testing.B) {
    engine := setupSearchEngine(b)
    
    query := &SearchQuery{
        QueryText:  "machine learning algorithms",
        SearchMode: HybridSearch,
        Limit:      20,
    }
    
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        _, err := engine.Search(context.Background(), query)
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkModelTrainer_ParallelTraining(b *testing.B) {
    trainer := setupModelTrainer(b)
    
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            job := &TrainingJob{
                Name:      fmt.Sprintf("bench-job-%d", rand.Int()),
                ModelType: ClassificationModel,
                DatasetID: uuid.New(),
            }
            
            _, err := trainer.StartTraining(context.Background(), job)
            if err != nil {
                b.Error(err)
            }
        }
    })
}
```

### **Load Testing**

#### **K6 Load Test Script**
```javascript
import http from 'k6/http';
import { check, sleep } from 'k6';

export let options = {
  stages: [
    { duration: '30s', target: 10 },
    { duration: '1m', target: 50 },
    { duration: '30s', target: 100 },
    { duration: '1m', target: 100 },
    { duration: '30s', target: 0 },
  ],
};

export default function () {
  // Test semantic search endpoint
  let searchPayload = JSON.stringify({
    query: 'machine learning algorithms',
    search_mode: 'hybrid',
    limit: 20,
  });

  let searchParams = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${__ENV.JWT_TOKEN}`,
    },
  };

  let searchResponse = http.post(
    'http://localhost:8080/api/v1/ml/search/semantic',
    searchPayload,
    searchParams
  );

  check(searchResponse, {
    'search status is 200': (r) => r.status === 200,
    'search response time < 500ms': (r) => r.timings.duration < 500,
  });

  sleep(1);
}
```

### **Test Coverage**

#### **Coverage Report**
```bash
# Generate coverage report
go test -coverprofile=coverage.out ./...

# View coverage in browser
go tool cover -html=coverage.out

# Coverage threshold check
go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//' | awk '{if ($1 < 80) exit 1}'
```

#### **Coverage Targets**
- **Overall Coverage**: >80%
- **Critical Paths**: >90%
- **ML/AI Components**: >85%
- **API Handlers**: >75%

---

## 🚀 **Deployment & Operations**

### **Container Deployment**

#### **Dockerfile**
```dockerfile
# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/server

# Production stage
FROM alpine:3.18

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/

COPY --from=builder /app/main .
COPY --from=builder /app/config ./config
COPY --from=builder /app/web ./web

EXPOSE 8080 9090

CMD ["./main"]
```

#### **Docker Compose**
```yaml
version: '3.8'

services:
  audimodal-api:
    build: .
    ports:
      - "8080:8080"
      - "9090:9090"
    environment:
      - AUDIMODAL_ENV=production
      - DATABASE_URL=postgresql://postgres:password@postgres:5432/audimodal
      - REDIS_URL=redis://redis:6379
      - KAFKA_BROKERS=kafka:9092
      - DEEPLAKE_API_URL=http://deeplake:8000
    depends_on:
      - postgres
      - redis
      - kafka
      - deeplake
    volumes:
      - ./config:/app/config
    restart: unless-stopped

  postgres:
    image: postgres:13
    environment:
      - POSTGRES_DB=audimodal
      - POSTGRES_PASSWORD=password
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"

  redis:
    image: redis:6-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data

  kafka:
    image: confluentinc/cp-kafka:7.4.0
    environment:
      KAFKA_ZOOKEEPER_CONNECT: zookeeper:2181
      KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://kafka:9092
      KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1
    depends_on:
      - zookeeper
    ports:
      - "9092:9092"

  zookeeper:
    image: confluentinc/cp-zookeeper:7.4.0
    environment:
      ZOOKEEPER_CLIENT_PORT: 2181

volumes:
  postgres_data:
  redis_data:
```

### **Kubernetes Deployment**

#### **Deployment Manifest**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: audimodal-api
  labels:
    app: audimodal-api
spec:
  replicas: 3
  selector:
    matchLabels:
      app: audimodal-api
  template:
    metadata:
      labels:
        app: audimodal-api
    spec:
      containers:
      - name: audimodal-api
        image: audimodal/api:1.8.0
        ports:
        - containerPort: 8080
        - containerPort: 9090
        env:
        - name: AUDIMODAL_ENV
          value: "production"
        - name: DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: audimodal-secrets
              key: database-url
        - name: DEEPLAKE_API_KEY
          valueFrom:
            secretKeyRef:
              name: audimodal-secrets
              key: deeplake-api-key
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "2Gi"
            cpu: "1000m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        volumeMounts:
        - name: config-volume
          mountPath: /app/config
      volumes:
      - name: config-volume
        configMap:
          name: audimodal-config
---
apiVersion: v1
kind: Service
metadata:
  name: audimodal-api-service
spec:
  selector:
    app: audimodal-api
  ports:
  - name: http
    port: 80
    targetPort: 8080
  - name: metrics
    port: 9090
    targetPort: 9090
  type: ClusterIP
```

#### **ConfigMap**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: audimodal-config
data:
  server.yaml: |
    server:
      host: "0.0.0.0"
      port: 8080
    database:
      host: "postgres-service"
      port: 5432
      name: "audimodal"
    redis:
      host: "redis-service"
      port: 6379
    kafka:
      brokers:
        - "kafka-service:9092"
    ml_ai:
      enabled: true
      model_training:
        max_concurrent_jobs: 5
      semantic_search:
        embedding_model: "sentence-transformers/all-MiniLM-L6-v2"
```

### **Monitoring & Observability**

#### **Prometheus Configuration**
```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'audimodal-api'
    static_configs:
      - targets: ['audimodal-api-service:9090']
    metrics_path: /metrics
    scrape_interval: 30s

  - job_name: 'audimodal-ml'
    static_configs:
      - targets: ['audimodal-api-service:9090']
    metrics_path: /ml/metrics
    scrape_interval: 60s
```

#### **Grafana Dashboard**
```json
{
  "dashboard": {
    "title": "AudiModal.ai Monitoring",
    "panels": [
      {
        "title": "Request Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(http_requests_total[5m])",
            "legendFormat": "{{method}} {{endpoint}}"
          }
        ]
      },
      {
        "title": "ML Training Jobs",
        "type": "stat",
        "targets": [
          {
            "expr": "ml_training_jobs_total",
            "legendFormat": "Total Jobs"
          }
        ]
      },
      {
        "title": "Search Performance",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(search_duration_seconds_bucket[5m]))",
            "legendFormat": "95th percentile"
          }
        ]
      }
    ]
  }
}
```

### **Health Checks**

#### **Health Check Implementation**
```go
type HealthChecker struct {
    db          *gorm.DB
    redis       *redis.Client
    deepLakeAPI *client.DeepLakeClient
}

func (h *HealthChecker) CheckHealth(ctx context.Context) *HealthStatus {
    status := &HealthStatus{
        Status:    "healthy",
        Timestamp: time.Now(),
        Services:  make(map[string]string),
    }

    // Check database
    if err := h.db.Ping(); err != nil {
        status.Services["database"] = "unhealthy"
        status.Status = "unhealthy"
    } else {
        status.Services["database"] = "healthy"
    }

    // Check Redis
    if err := h.redis.Ping(ctx).Err(); err != nil {
        status.Services["redis"] = "unhealthy"
        status.Status = "unhealthy"
    } else {
        status.Services["redis"] = "healthy"
    }

    // Check DeepLake API
    if err := h.deepLakeAPI.HealthCheck(ctx); err != nil {
        status.Services["deeplake"] = "unhealthy"
        status.Status = "unhealthy"
    } else {
        status.Services["deeplake"] = "healthy"
    }

    return status
}
```

---

## ⚡ **Performance Optimization**

### **Database Optimization**

#### **Connection Pooling**
```go
func configureDatabasePool(db *gorm.DB) {
    sqlDB, _ := db.DB()
    
    // Maximum number of open connections
    sqlDB.SetMaxOpenConns(100)
    
    // Maximum number of idle connections
    sqlDB.SetMaxIdleConns(10)
    
    // Maximum lifetime of connections
    sqlDB.SetConnMaxLifetime(time.Hour)
    
    // Maximum idle time for connections
    sqlDB.SetConnMaxIdleTime(time.Minute * 30)
}
```

#### **Query Optimization**
```go
// Use indexes for frequent queries
func (r *FileRepository) FindByTenantAndStatus(ctx context.Context, tenantID uuid.UUID, status ProcessingStatus) ([]*File, error) {
    var files []*File
    
    // This query uses the composite index: idx_files_tenant_status
    err := r.db.WithContext(ctx).
        Where("tenant_id = ? AND processing_status = ?", tenantID, status).
        Order("created_at DESC").
        Find(&files).Error
    
    return files, err
}

// Use preloading to avoid N+1 queries
func (r *FileRepository) FindWithDataSource(ctx context.Context, tenantID uuid.UUID) ([]*File, error) {
    var files []*File
    
    err := r.db.WithContext(ctx).
        Preload("DataSource").
        Where("tenant_id = ?", tenantID).
        Find(&files).Error
    
    return files, err
}
```

### **Caching Strategy**

#### **Multi-Level Caching**
```go
type CacheManager struct {
    l1Cache *sync.Map           // In-memory cache
    l2Cache *redis.Client       // Redis cache
    l3Cache storage.Backend     // Persistent storage
}

func (cm *CacheManager) Get(ctx context.Context, key string) (interface{}, error) {
    // Level 1: In-memory cache
    if value, ok := cm.l1Cache.Load(key); ok {
        return value, nil
    }
    
    // Level 2: Redis cache
    value, err := cm.l2Cache.Get(ctx, key).Result()
    if err == nil {
        // Store in L1 cache
        cm.l1Cache.Store(key, value)
        return value, nil
    }
    
    // Level 3: Persistent storage (miss)
    return nil, ErrCacheMiss
}
```

#### **Search Result Caching**
```go
func (sse *SemanticSearchEngine) Search(ctx context.Context, query *SearchQuery) (*SearchResult, error) {
    // Generate cache key
    cacheKey := sse.generateCacheKey(query)
    
    // Check cache
    if cached := sse.getCachedResult(cacheKey); cached != nil {
        return cached.Result, nil
    }
    
    // Perform search
    result, err := sse.performSearch(ctx, query)
    if err != nil {
        return nil, err
    }
    
    // Cache result
    sse.cacheResult(cacheKey, result)
    
    return result, nil
}
```

### **Concurrent Processing**

#### **Worker Pool Pattern**
```go
type WorkerPool struct {
    workers   int
    taskQueue chan Task
    wg        sync.WaitGroup
    ctx       context.Context
    cancel    context.CancelFunc
}

func NewWorkerPool(workers int) *WorkerPool {
    ctx, cancel := context.WithCancel(context.Background())
    
    wp := &WorkerPool{
        workers:   workers,
        taskQueue: make(chan Task, workers*2),
        ctx:       ctx,
        cancel:    cancel,
    }
    
    // Start workers
    for i := 0; i < workers; i++ {
        wp.wg.Add(1)
        go wp.worker(i)
    }
    
    return wp
}

func (wp *WorkerPool) worker(id int) {
    defer wp.wg.Done()
    
    for {
        select {
        case task := <-wp.taskQueue:
            task.Execute(wp.ctx)
        case <-wp.ctx.Done():
            return
        }
    }
}
```

#### **Batch Processing**
```go
func (dm *DatasetManager) ProcessDocumentsBatch(ctx context.Context, documents []*Document) error {
    const batchSize = 100
    
    for i := 0; i < len(documents); i += batchSize {
        end := i + batchSize
        if end > len(documents) {
            end = len(documents)
        }
        
        batch := documents[i:end]
        
        if err := dm.processBatch(ctx, batch); err != nil {
            return fmt.Errorf("failed to process batch %d-%d: %w", i, end, err)
        }
    }
    
    return nil
}
```

### **Memory Optimization**

#### **Object Pooling**
```go
var embeddingPool = sync.Pool{
    New: func() interface{} {
        return make([]float64, 384) // Standard embedding size
    },
}

func (ee *EmbeddingEngine) GenerateEmbedding(ctx context.Context, text string) ([]float64, error) {
    // Get embedding slice from pool
    embedding := embeddingPool.Get().([]float64)
    defer embeddingPool.Put(embedding)
    
    // Generate embedding
    err := ee.generateEmbeddingInto(ctx, text, embedding)
    if err != nil {
        return nil, err
    }
    
    // Return copy to avoid pool interference
    result := make([]float64, len(embedding))
    copy(result, embedding)
    
    return result, nil
}
```

#### **Streaming Processing**
```go
func (pe *ProcessingEngine) ProcessLargeFile(ctx context.Context, fileID uuid.UUID) error {
    file, err := pe.getFile(ctx, fileID)
    if err != nil {
        return err
    }
    
    // Open file for streaming
    reader, err := pe.storage.OpenReader(ctx, file.Path)
    if err != nil {
        return err
    }
    defer reader.Close()
    
    // Process in chunks to avoid loading entire file into memory
    buffer := make([]byte, 64*1024) // 64KB chunks
    
    for {
        n, err := reader.Read(buffer)
        if err == io.EOF {
            break
        }
        if err != nil {
            return err
        }
        
        chunk := buffer[:n]
        if err := pe.processChunk(ctx, fileID, chunk); err != nil {
            return err
        }
    }
    
    return nil
}
```

---

## 🔒 **Security & Compliance**

### **Authentication & Authorization**

#### **JWT Authentication**
```go
type AuthMiddleware struct {
    jwtSecret []byte
}

func (am *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := extractToken(r)
        if token == "" {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        
        claims, err := am.validateToken(token)
        if err != nil {
            http.Error(w, "Invalid token", http.StatusUnauthorized)
            return
        }
        
        // Add claims to context
        ctx := context.WithValue(r.Context(), "claims", claims)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func (am *AuthMiddleware) validateToken(tokenString string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
        return am.jwtSecret, nil
    })
    
    if err != nil {
        return nil, err
    }
    
    if claims, ok := token.Claims.(*Claims); ok && token.Valid {
        return claims, nil
    }
    
    return nil, fmt.Errorf("invalid token")
}
```

#### **Multi-Tenant Authorization**
```go
func (am *AuthMiddleware) AuthorizeResource(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        claims := r.Context().Value("claims").(*Claims)
        resourceTenantID := mux.Vars(r)["tenantId"]
        
        // Check if user has access to tenant
        if claims.TenantID != resourceTenantID {
            http.Error(w, "Forbidden", http.StatusForbidden)
            return
        }
        
        next.ServeHTTP(w, r)
    })
}
```

### **Data Encryption**

#### **Encryption at Rest**
```go
type EncryptionService struct {
    key []byte
}

func (es *EncryptionService) EncryptSensitiveData(data []byte) ([]byte, error) {
    block, err := aes.NewCipher(es.key)
    if err != nil {
        return nil, err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }
    
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return nil, err
    }
    
    ciphertext := gcm.Seal(nonce, nonce, data, nil)
    return ciphertext, nil
}

func (es *EncryptionService) DecryptSensitiveData(ciphertext []byte) ([]byte, error) {
    block, err := aes.NewCipher(es.key)
    if err != nil {
        return nil, err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }
    
    nonceSize := gcm.NonceSize()
    if len(ciphertext) < nonceSize {
        return nil, fmt.Errorf("ciphertext too short")
    }
    
    nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
    plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return nil, err
    }
    
    return plaintext, nil
}
```

#### **Encryption in Transit**
```go
func configureTLS(server *http.Server) {
    // TLS configuration
    tlsConfig := &tls.Config{
        MinVersion:               tls.VersionTLS12,
        CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
        PreferServerCipherSuites: true,
        CipherSuites: []uint16{
            tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
            tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
            tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
        },
    }
    
    server.TLSConfig = tlsConfig
}
```

### **Input Validation & Sanitization**

#### **Request Validation**
```go
type Validator struct {
    validate *validator.Validate
}

func NewValidator() *Validator {
    v := validator.New()
    
    // Register custom validators
    v.RegisterValidation("uuid", validateUUID)
    v.RegisterValidation("filename", validateFilename)
    
    return &Validator{validate: v}
}

func (v *Validator) ValidateStruct(s interface{}) error {
    return v.validate.Struct(s)
}

// Custom validators
func validateUUID(fl validator.FieldLevel) bool {
    _, err := uuid.Parse(fl.Field().String())
    return err == nil
}

func validateFilename(fl validator.FieldLevel) bool {
    filename := fl.Field().String()
    
    // Check for path traversal attempts
    if strings.Contains(filename, "..") {
        return false
    }
    
    // Check for null bytes
    if strings.Contains(filename, "\x00") {
        return false
    }
    
    return true
}
```

#### **SQL Injection Prevention**
```go
// Always use parameterized queries
func (r *FileRepository) FindByTenantAndName(ctx context.Context, tenantID uuid.UUID, name string) (*File, error) {
    var file File
    
    // GOOD: Parameterized query
    err := r.db.WithContext(ctx).
        Where("tenant_id = ? AND name = ?", tenantID, name).
        First(&file).Error
    
    return &file, err
}

// BAD: String concatenation (vulnerable to SQL injection)
// err := r.db.Raw("SELECT * FROM files WHERE tenant_id = '" + tenantID.String() + "' AND name = '" + name + "'").Scan(&file).Error
```

### **Audit Logging**

#### **Audit Trail Implementation**
```go
type AuditLogger struct {
    logger *zap.Logger
    db     *gorm.DB
}

func (al *AuditLogger) LogEvent(ctx context.Context, event *AuditEvent) error {
    // Extract user information from context
    claims := ctx.Value("claims").(*Claims)
    
    auditLog := &AuditLog{
        EventType:   event.Type,
        UserID:      claims.UserID,
        TenantID:    claims.TenantID,
        ResourceID:  event.ResourceID,
        Action:      event.Action,
        Details:     event.Details,
        IPAddress:   extractIPAddress(ctx),
        UserAgent:   extractUserAgent(ctx),
        Timestamp:   time.Now(),
    }
    
    // Log to database
    if err := al.db.Create(auditLog).Error; err != nil {
        al.logger.Error("Failed to create audit log", zap.Error(err))
        return err
    }
    
    // Log to structured logger
    al.logger.Info("Audit event",
        zap.String("event_type", string(event.Type)),
        zap.String("user_id", claims.UserID.String()),
        zap.String("action", event.Action),
        zap.Any("details", event.Details),
    )
    
    return nil
}
```

### **Data Loss Prevention (DLP)**

#### **Content Scanning**
```go
type DLPScanner struct {
    rules []DLPRule
}

func (scanner *DLPScanner) ScanContent(ctx context.Context, content string) (*DLPResult, error) {
    result := &DLPResult{
        Safe:       true,
        Violations: []DLPViolation{},
    }
    
    for _, rule := range scanner.rules {
        if violation := rule.Check(content); violation != nil {
            result.Safe = false
            result.Violations = append(result.Violations, *violation)
        }
    }
    
    return result, nil
}

// Example DLP rule for credit card numbers
type CreditCardRule struct{}

func (r *CreditCardRule) Check(content string) *DLPViolation {
    // Luhn algorithm check for credit card numbers
    ccRegex := regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`)
    matches := ccRegex.FindAllString(content, -1)
    
    for _, match := range matches {
        if r.isValidCreditCard(match) {
            return &DLPViolation{
                RuleType:    "credit_card",
                Match:       match,
                Severity:    "high",
                Description: "Potential credit card number detected",
            }
        }
    }
    
    return nil
}
```

---

## 🐛 **Troubleshooting**

### **Common Issues**

#### **Database Connection Issues**
```bash
# Check database connectivity
psql "postgresql://user:pass@localhost:5432/audimodal" -c "SELECT version();"

# Check connection pool
curl http://localhost:8080/metrics | grep db_connections

# View database logs
docker logs audimodal-postgres

# Reset connection pool
curl -X POST http://localhost:8080/admin/db/reset-pool
```

#### **Memory Issues**
```bash
# Check memory usage
curl http://localhost:8080/metrics | grep go_memstats

# Generate memory profile
go tool pprof http://localhost:8080/debug/pprof/heap

# Garbage collection stats
curl http://localhost:8080/debug/vars | jq '.memstats'
```

#### **Performance Issues**
```bash
# Check CPU profile
go tool pprof http://localhost:8080/debug/pprof/profile

# Check goroutine count
curl http://localhost:8080/metrics | grep go_goroutines

# Check slow queries
tail -f /var/log/postgresql/slow-queries.log
```

### **Debugging ML/AI Components**

#### **Model Training Issues**
```go
// Enable debug logging for training
func (mt *ModelTrainer) StartTraining(ctx context.Context, job *TrainingJob) (uuid.UUID, error) {
    logger := log.WithFields(log.Fields{
        "job_id": job.ID,
        "model_type": job.ModelType,
        "dataset_id": job.DatasetID,
    })
    
    logger.Info("Starting training job")
    
    // Validate dataset
    dataset, err := mt.datasetManager.GetDataset(ctx, job.DatasetID)
    if err != nil {
        logger.WithError(err).Error("Failed to get dataset")
        return uuid.Nil, err
    }
    
    if dataset.Status != DatasetStatusReady {
        logger.WithField("dataset_status", dataset.Status).Error("Dataset not ready")
        return uuid.Nil, fmt.Errorf("dataset not ready: %s", dataset.Status)
    }
    
    // Continue with training...
}
```

#### **Search Performance Issues**
```go
// Enable search debugging
func (sse *SemanticSearchEngine) Search(ctx context.Context, query *SearchQuery) (*SearchResult, error) {
    start := time.Now()
    
    // Log query details
    log.WithFields(log.Fields{
        "query_text": query.QueryText,
        "search_mode": query.SearchMode,
        "limit": query.Limit,
    }).Info("Starting search")
    
    // Generate query embedding
    embStart := time.Now()
    queryEmbedding, err := sse.embeddingEngine.GenerateEmbedding(ctx, query.QueryText)
    embDuration := time.Since(embStart)
    
    log.WithField("embedding_time", embDuration).Debug("Generated query embedding")
    
    if err != nil {
        log.WithError(err).Error("Failed to generate query embedding")
        return nil, err
    }
    
    // Continue with search...
    
    totalDuration := time.Since(start)
    log.WithFields(log.Fields{
        "total_duration": totalDuration,
        "results_count": len(result.Results),
    }).Info("Search completed")
    
    return result, nil
}
```

### **Log Analysis**

#### **Structured Logging**
```go
type StructuredLogger struct {
    logger *zap.Logger
}

func (sl *StructuredLogger) LogError(ctx context.Context, err error, fields map[string]interface{}) {
    zapFields := make([]zap.Field, 0, len(fields)+2)
    
    // Add context fields
    if claims := ctx.Value("claims"); claims != nil {
        if c, ok := claims.(*Claims); ok {
            zapFields = append(zapFields,
                zap.String("user_id", c.UserID.String()),
                zap.String("tenant_id", c.TenantID.String()),
            )
        }
    }
    
    // Add request ID
    if requestID := ctx.Value("request_id"); requestID != nil {
        zapFields = append(zapFields, zap.String("request_id", requestID.(string)))
    }
    
    // Add custom fields
    for key, value := range fields {
        zapFields = append(zapFields, zap.Any(key, value))
    }
    
    sl.logger.Error(err.Error(), zapFields...)
}
```

#### **Log Aggregation with ELK Stack**
```yaml
# Filebeat configuration
filebeat.inputs:
- type: log
  paths:
    - /var/log/audimodal/*.log
  fields:
    service: audimodal-api
  fields_under_root: true

output.elasticsearch:
  hosts: ["elasticsearch:9200"]

# Logstash pipeline
input {
  beats {
    port => 5044
  }
}

filter {
  if [service] == "audimodal-api" {
    json {
      source => "message"
    }
    
    date {
      match => [ "timestamp", "ISO8601" ]
    }
  }
}

output {
  elasticsearch {
    hosts => ["elasticsearch:9200"]
    index => "audimodal-logs-%{+YYYY.MM.dd}"
  }
}
```

---

## 🤝 **Contributing Guidelines**

### **Development Standards**

#### **Code Style**
```go
// Good: Clear naming and documentation
// ProcessDocumentBatch processes a batch of documents for ML training
func (trainer *ModelTrainer) ProcessDocumentBatch(ctx context.Context, documents []*Document) error {
    if len(documents) == 0 {
        return nil
    }
    
    logger := log.WithFields(log.Fields{
        "batch_size": len(documents),
        "trainer_id": trainer.ID,
    })
    
    logger.Info("Processing document batch")
    
    for _, doc := range documents {
        if err := trainer.processDocument(ctx, doc); err != nil {
            logger.WithError(err).WithField("document_id", doc.ID).Error("Failed to process document")
            return fmt.Errorf("failed to process document %s: %w", doc.ID, err)
        }
    }
    
    logger.Info("Document batch processed successfully")
    return nil
}
```

#### **Error Handling**
```go
// Good: Comprehensive error handling with context
func (api *MLHandlers) CreateTrainingJob(w http.ResponseWriter, r *http.Request) {
    var req CreateTrainingJobRequest
    
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        api.respondWithError(w, http.StatusBadRequest, "Invalid request body", err)
        return
    }
    
    if err := api.validator.ValidateStruct(&req); err != nil {
        api.respondWithError(w, http.StatusBadRequest, "Validation failed", err)
        return
    }
    
    tenantID := getTenantID(r)
    userID := getUserID(r)
    
    trainingJob := &TrainingJob{
        Name:        req.Name,
        ModelType:   req.ModelType,
        DatasetID:   req.DatasetID,
        TenantID:    tenantID,
        CreatedBy:   userID,
    }
    
    jobID, err := api.modelTrainer.StartTraining(r.Context(), trainingJob)
    if err != nil {
        // Log error with context
        api.logger.WithError(err).WithFields(log.Fields{
            "tenant_id": tenantID,
            "user_id": userID,
            "model_type": req.ModelType,
        }).Error("Failed to start training job")
        
        api.respondWithError(w, http.StatusInternalServerError, "Failed to start training job", err)
        return
    }
    
    response := map[string]interface{}{
        "job_id": jobID,
        "status": "started",
    }
    
    api.respondWithJSON(w, http.StatusCreated, response)
}
```

### **Testing Requirements**

#### **Unit Test Coverage**
```go
func TestModelTrainer_StartTraining(t *testing.T) {
    tests := []struct {
        name     string
        job      *TrainingJob
        setup    func(*MockDatasetManager)
        wantErr  bool
        errType  error
    }{
        {
            name: "successful training job",
            job: &TrainingJob{
                Name:      "test-job",
                ModelType: ClassificationModel,
                DatasetID: uuid.New(),
            },
            setup: func(mock *MockDatasetManager) {
                mock.On("GetDataset", mock.Anything, mock.Anything).Return(&Dataset{
                    Status: DatasetStatusReady,
                }, nil)
            },
            wantErr: false,
        },
        {
            name: "dataset not ready",
            job: &TrainingJob{
                Name:      "test-job",
                ModelType: ClassificationModel,
                DatasetID: uuid.New(),
            },
            setup: func(mock *MockDatasetManager) {
                mock.On("GetDataset", mock.Anything, mock.Anything).Return(&Dataset{
                    Status: DatasetStatusProcessing,
                }, nil)
            },
            wantErr: true,
            errType: ErrDatasetNotReady,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockDatasetManager := &MockDatasetManager{}
            if tt.setup != nil {
                tt.setup(mockDatasetManager)
            }
            
            trainer := &ModelTrainer{
                datasetManager: mockDatasetManager,
                trainingJobs:   make(map[uuid.UUID]*TrainingJob),
                jobQueue:       make(chan *TrainingJob, 10),
            }
            
            jobID, err := trainer.StartTraining(context.Background(), tt.job)
            
            if tt.wantErr {
                assert.Error(t, err)
                if tt.errType != nil {
                    assert.ErrorIs(t, err, tt.errType)
                }
                return
            }
            
            assert.NoError(t, err)
            assert.NotEqual(t, uuid.Nil, jobID)
            
            mockDatasetManager.AssertExpectations(t)
        })
    }
}
```

### **Documentation Requirements**

#### **API Documentation**
```go
// CreateTrainingJob creates a new ML model training job
//
// @Summary Create training job
// @Description Start a new model training job with specified parameters
// @Tags ML/AI Training
// @Accept json
// @Produce json
// @Param request body CreateTrainingJobRequest true "Training job parameters"
// @Success 201 {object} TrainingJobResponse "Training job created"
// @Failure 400 {object} ErrorResponse "Invalid request"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /ml/training/jobs [post]
func (api *MLHandlers) CreateTrainingJob(w http.ResponseWriter, r *http.Request) {
    // Implementation...
}
```

#### **Package Documentation**
```go
// Package training provides ML model training capabilities for the AudiModal platform.
//
// This package includes:
//   - Model training job management
//   - Dataset management and validation
//   - Training progress tracking
//   - Resource management and optimization
//
// Example usage:
//
//     trainer := training.NewModelTrainer(config)
//     
//     job := &training.TrainingJob{
//         Name:      "document-classifier",
//         ModelType: training.ClassificationModel,
//         DatasetID: datasetID,
//     }
//     
//     jobID, err := trainer.StartTraining(ctx, job)
//     if err != nil {
//         log.Fatalf("Failed to start training: %v", err)
//     }
//
package training
```

### **Pull Request Process**

#### **PR Template**
```markdown
## Description
Brief description of changes made in this PR.

## Type of Change
- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update

## ML/AI Components
- [ ] Model Training
- [ ] Predictions
- [ ] Knowledge Graph
- [ ] Semantic Search
- [ ] Insights Engine

## Testing
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] ML/AI specific tests added/updated
- [ ] Performance tests (if applicable)

## Documentation
- [ ] Code comments updated
- [ ] API documentation updated
- [ ] README updated (if applicable)
- [ ] OpenAPI spec updated (if applicable)

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Breaking changes documented
- [ ] Tests added for new functionality
- [ ] All CI checks pass
```

### **Release Process**

#### **Version Management**
```bash
# Create release branch
git checkout -b release/v1.9.0

# Update version files
echo "1.9.0" > VERSION
git add VERSION
git commit -m "chore: bump version to 1.9.0"

# Create tag
git tag -a v1.9.0 -m "Release version 1.9.0"

# Merge to main
git checkout main
git merge release/v1.9.0

# Push changes
git push origin main
git push origin v1.9.0
```

---

**📚 This completes the comprehensive Developer Documentation for AudiModal.ai v1.8.0. For the latest updates and additional resources, visit the project repository and API documentation.**