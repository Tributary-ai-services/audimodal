# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**AudiModal.ai** is an enterprise-grade, cloud-native document processing platform that provides AI-powered document intelligence, multi-tenant architecture, and compliance automation for regulated industries. Built with Go, it offers comprehensive document processing capabilities with advanced ML/AI classification, security scanning, and enterprise integrations.

## Data Models & Schema Reference

### Service-Specific Data Models
This service's data models are comprehensively documented in the centralized data models repository:

**Location**: `../aether-shared/data-models/audimodal/`

#### Key PostgreSQL Entity Models:
- **File Entity** (`file.md`) - Document metadata, processing status, S3 storage locations, and compliance flags
- **Tenant Entity** (`tenant.md`) - Multi-tenant isolation with resource limits and compliance settings
- **ProcessingSession Entity** (`processing-session.md`) - Document processing workflow tracking and status management

#### Cross-Service Integration:
- **Document Upload Flow** (`../aether-shared/data-models/cross-service/flows/document-upload.md`) - Integration with Aether backend and DeepLake vector storage
- **Platform ERD** (`../aether-shared/data-models/cross-service/diagrams/platform-erd.md`) - Complete entity relationship diagram
- **ID Mapping Chain** (`../aether-shared/data-models/cross-service/mappings/id-mapping-chain.md`) - Cross-service identifier relationships

#### When to Reference Data Models:
1. Before making schema changes to PostgreSQL tables or adding new fields
2. When implementing new API endpoints that interact with file processing
3. When debugging data-related issues or processing pipeline failures
4. When onboarding new developers to understand the data architecture
5. Before modifying tenant isolation or compliance tracking features

**Main Documentation Hub**: `../aether-shared/data-models/README.md` - Complete navigation for all 38 data model files

## Technology Stack

- **Language**: Go 1.21+
- **Database**: PostgreSQL (shared TAS infrastructure)
- **Storage**: MinIO (S3-compatible object storage)
- **Message Queue**: Kafka for event streaming
- **Framework**: Gin HTTP framework
- **Monitoring**: Prometheus metrics + OpenTelemetry

## Key Features

### AI-Powered Document Intelligence
- Custom ML model training with job management
- Predictive analytics and knowledge graph construction
- Advanced PII detection (15+ types)
- Semantic search with vector embeddings

### Enterprise Security & Compliance
- Multi-regulatory support (GDPR, HIPAA, SOX, PCI DSS)
- Zero-trust architecture with continuous verification
- Data loss prevention with automated redaction
- Immutable audit trail with 7-year retention

### Multi-Tenant Architecture
- Secure tenant isolation supporting 1,000+ concurrent tenants
- Kubernetes CRDs for declarative resource management
- Enterprise connectors (SharePoint, Confluence, Jira, Slack)
- Performance: 10,000+ files per hour per tenant

## PDF Processing Configuration

### Image Detection for OCR

The PDF processor supports configurable image detection settings that control when OCR is triggered:

| Config Option | Default | Description |
|---------------|---------|-------------|
| `ocr_any_image` | `false` | If true, trigger OCR for ANY image on page regardless of size |
| `ocr_image_min_width` | `200` | Minimum image width in pixels to trigger OCR |
| `ocr_image_min_height` | `200` | Minimum image height in pixels to trigger OCR |

These settings are used by both the streaming and map-reduce PDF processing pipelines.

### Map-Reduce Processing Mode

For large PDFs (>50 pages by default), the system uses a map-reduce pipeline with subprocess isolation to prevent OOM issues:

| Config Option | Default | Description |
|---------------|---------|-------------|
| `processing_mode` | `auto` | Processing mode: `streaming`, `mapreduce`, or `auto` |
| `mapreduce_page_threshold` | `50` | Use map-reduce for PDFs with more than this many pages |
| `mapreduce_workers` | `4` | Number of parallel workers for map-reduce mode |
| `ocr_dpi` | `150` | OCR image resolution in DPI (lower = less memory) |
| `ocr_language` | `eng` | OCR language code (ISO 639-2) |

## Common Commands

See the comprehensive documentation in:
- `README.md` - Platform overview and features
- `DEVELOPER.md` - Development setup and workflows
- `SETUP.md` - Installation and configuration
- `ROADMAP.md` - Future development plans

## Integration Points

- **Aether Backend**: Document upload and metadata synchronization
- **DeepLake API**: Vector embedding storage and semantic search
- **TAS LLM Router**: AI-powered document classification and analysis
- **Keycloak**: Multi-tenant authentication and authorization
- **MinIO**: Secure S3-compatible document storage
- **Kafka**: Async processing events and workflow triggers

## Important Notes

- All file processing includes mandatory security scanning and PII detection
- Compliance requirements are enforced at the database level with immutable audit logs
- Multi-tenant isolation is critical - always verify tenant context in API requests
- Document processing uses multi-tier architecture (real-time, batch, distributed)
- Integration with shared TAS infrastructure via `tas-shared-network` Docker network
