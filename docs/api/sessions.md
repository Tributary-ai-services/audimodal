# Processing Sessions API

Processing Sessions orchestrate document workflows, managing the end-to-end processing pipeline from ingestion to analysis.

## Base URL

```
/v1/tenants/{tenant_id}/sessions
```

## Create Processing Session

Create a new processing session for document workflows.

### Request

```http
POST /v1/tenants/{tenant_id}/sessions
Authorization: Bearer tk_tenant_123_key
Content-Type: application/json

{
  "name": "Q1 Legal Document Review",
  "description": "Process all legal documents for Q1 compliance review",
  "type": "batch_processing",
  "priority": "high",
  "configuration": {
    "processing_options": {
      "extract_text": true,
      "generate_embeddings": true,
      "run_analysis": true,
      "dlp_scan": true,
      "chunking_strategy": "semantic",
      "chunk_size": 1000,
      "chunk_overlap": 100
    },
    "filters": {
      "document_type": ["contract", "agreement"],
      "created_after": "2024-01-01T00:00:00Z",
      "classification": ["confidential", "restricted"]
    },
    "output_settings": {
      "generate_summary": true,
      "create_report": true,
      "notify_on_completion": true,
      "webhook_url": "https://api.company.com/webhooks/audimodal"
    }
  },
  "scheduled_start": "2024-01-16T09:00:00Z",
  "max_processing_time": 7200,
  "retry_policy": {
    "max_retries": 3,
    "retry_delay": 300,
    "backoff_multiplier": 2.0
  }
}
```

### Request Parameters

- `name` (string, required): Session name
- `description` (string, optional): Session description
- `type` (string, required): Session type (`batch_processing`, `incremental`, `manual`)
- `priority` (string, optional): Processing priority (`low`, `normal`, `high`)
- `configuration` (object, required): Processing configuration
- `scheduled_start` (datetime, optional): Scheduled start time
- `max_processing_time` (integer, optional): Maximum processing time in seconds
- `retry_policy` (object, optional): Retry configuration

### Response

```http
HTTP/1.1 201 Created
Content-Type: application/json

{
  "id": "session_abc123",
  "tenant_id": "tenant_123abc",
  "name": "Q1 Legal Document Review",
  "description": "Process all legal documents for Q1 compliance review",
  "type": "batch_processing",
  "status": "created",
  "priority": "high",
  "created_at": "2024-01-15T14:30:00Z",
  "updated_at": "2024-01-15T14:30:00Z",
  "scheduled_start": "2024-01-16T09:00:00Z",
  "started_at": null,
  "completed_at": null,
  "configuration": {
    "processing_options": {
      "extract_text": true,
      "generate_embeddings": true,
      "run_analysis": true,
      "dlp_scan": true,
      "chunking_strategy": "semantic",
      "chunk_size": 1000,
      "chunk_overlap": 100
    },
    "filters": {
      "document_type": ["contract", "agreement"],
      "created_after": "2024-01-01T00:00:00Z",
      "classification": ["confidential", "restricted"]
    },
    "output_settings": {
      "generate_summary": true,
      "create_report": true,
      "notify_on_completion": true,
      "webhook_url": "https://api.company.com/webhooks/audimodal"
    }
  },
  "progress": {
    "total_files": 0,
    "processed_files": 0,
    "failed_files": 0,
    "percentage": 0.0,
    "current_stage": "created",
    "estimated_completion": null
  },
  "stats": {
    "processing_time_ms": 0,
    "total_chunks": 0,
    "total_embeddings": 0,
    "dlp_violations": 0
  },
  "retry_policy": {
    "max_retries": 3,
    "retry_delay": 300,
    "backoff_multiplier": 2.0,
    "current_retries": 0
  }
}
```

## List Processing Sessions

List processing sessions with filtering and pagination.

### Request

```http
GET /v1/tenants/{tenant_id}/sessions?page=1&page_size=50&status=running&type=batch_processing
Authorization: Bearer tk_tenant_123_key
```

### Query Parameters

- `page` (integer): Page number (default: 1)
- `page_size` (integer): Items per page (default: 50, max: 100)
- `status` (string): Filter by status
- `type` (string): Filter by session type
- `priority` (string): Filter by priority
- `created_after` (datetime): Filter by creation date
- `created_before` (datetime): Filter by creation date
- `search` (string): Search in name and description
- `sort` (string): Sort field (`created_at`, `updated_at`, `name`)
- `order` (string): Sort order (`asc`, `desc`)

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "sessions": [
    {
      "id": "session_abc123",
      "name": "Q1 Legal Document Review",
      "type": "batch_processing",
      "status": "running",
      "priority": "high",
      "created_at": "2024-01-15T14:30:00Z",
      "started_at": "2024-01-16T09:00:00Z",
      "progress": {
        "total_files": 150,
        "processed_files": 75,
        "failed_files": 2,
        "percentage": 50.0,
        "current_stage": "embedding_generation",
        "estimated_completion": "2024-01-16T11:30:00Z"
      },
      "stats": {
        "processing_time_ms": 1800000,
        "total_chunks": 1245,
        "total_embeddings": 1245
      }
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 50,
    "total_count": 1,
    "total_pages": 1,
    "has_next": false,
    "has_previous": false
  },
  "aggregations": {
    "by_status": {
      "running": 1,
      "completed": 15,
      "failed": 2
    },
    "by_type": {
      "batch_processing": 12,
      "incremental": 6
    }
  }
}
```

## Get Processing Session

Get detailed session information.

### Request

```http
GET /v1/sessions/{session_id}
Authorization: Bearer tk_tenant_123_key
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "id": "session_abc123",
  "tenant_id": "tenant_123abc",
  "name": "Q1 Legal Document Review",
  "description": "Process all legal documents for Q1 compliance review",
  "type": "batch_processing",
  "status": "running",
  "priority": "high",
  "created_at": "2024-01-15T14:30:00Z",
  "updated_at": "2024-01-16T10:15:00Z",
  "scheduled_start": "2024-01-16T09:00:00Z",
  "started_at": "2024-01-16T09:00:00Z",
  "completed_at": null,
  "configuration": {
    "processing_options": {
      "extract_text": true,
      "generate_embeddings": true,
      "run_analysis": true,
      "dlp_scan": true,
      "chunking_strategy": "semantic",
      "chunk_size": 1000,
      "chunk_overlap": 100
    },
    "filters": {
      "document_type": ["contract", "agreement"],
      "created_after": "2024-01-01T00:00:00Z",
      "classification": ["confidential", "restricted"]
    },
    "output_settings": {
      "generate_summary": true,
      "create_report": true,
      "notify_on_completion": true,
      "webhook_url": "https://api.company.com/webhooks/audimodal"
    }
  },
  "progress": {
    "total_files": 150,
    "processed_files": 75,
    "failed_files": 2,
    "skipped_files": 0,
    "percentage": 50.0,
    "current_stage": "embedding_generation",
    "estimated_completion": "2024-01-16T11:30:00Z",
    "stages": [
      {
        "name": "file_discovery",
        "status": "completed",
        "started_at": "2024-01-16T09:00:00Z",
        "completed_at": "2024-01-16T09:05:00Z",
        "files_processed": 150
      },
      {
        "name": "text_extraction",
        "status": "completed",
        "started_at": "2024-01-16T09:05:00Z",
        "completed_at": "2024-01-16T09:45:00Z",
        "files_processed": 150
      },
      {
        "name": "embedding_generation",
        "status": "running",
        "started_at": "2024-01-16T09:45:00Z",
        "completed_at": null,
        "files_processed": 75
      }
    ]
  },
  "stats": {
    "processing_time_ms": 4500000,
    "total_chunks": 1245,
    "total_embeddings": 622,
    "dlp_violations": 5,
    "compliance_score": 97.2,
    "average_processing_time_per_file_ms": 30000
  },
  "files": {
    "total": 150,
    "by_status": {
      "processed": 73,
      "processing": 2,
      "failed": 2,
      "pending": 73
    },
    "by_type": {
      "pdf": 89,
      "docx": 42,
      "xlsx": 19
    }
  },
  "errors": [
    {
      "file_id": "file_error1",
      "filename": "corrupted_file.pdf",
      "stage": "text_extraction",
      "error_code": "CORRUPTED_FILE",
      "error_message": "Unable to extract text from corrupted PDF",
      "occurred_at": "2024-01-16T09:30:00Z",
      "retry_count": 3
    }
  ],
  "retry_policy": {
    "max_retries": 3,
    "retry_delay": 300,
    "backoff_multiplier": 2.0,
    "current_retries": 0
  }
}
```

## Start Processing Session

Start a created or scheduled session.

### Request

```http
POST /v1/sessions/{session_id}/start
Authorization: Bearer tk_tenant_123_key
Content-Type: application/json

{
  "force_start": false,
  "override_schedule": false
}
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "message": "Session started successfully",
  "session_id": "session_abc123",
  "status": "running",
  "started_at": "2024-01-16T09:00:00Z",
  "estimated_completion": "2024-01-16T11:30:00Z"
}
```

## Pause Processing Session

Pause a running session.

### Request

```http
POST /v1/sessions/{session_id}/pause
Authorization: Bearer tk_tenant_123_key
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "message": "Session paused successfully",
  "session_id": "session_abc123",
  "status": "paused",
  "paused_at": "2024-01-16T10:30:00Z",
  "progress_saved": {
    "processed_files": 75,
    "current_stage": "embedding_generation"
  }
}
```

## Resume Processing Session

Resume a paused session.

### Request

```http
POST /v1/sessions/{session_id}/resume
Authorization: Bearer tk_tenant_123_key
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "message": "Session resumed successfully",
  "session_id": "session_abc123",
  "status": "running",
  "resumed_at": "2024-01-16T10:35:00Z",
  "estimated_completion": "2024-01-16T12:00:00Z"
}
```

## Cancel Processing Session

Cancel a running or paused session.

### Request

```http
POST /v1/sessions/{session_id}/cancel
Authorization: Bearer tk_tenant_123_key
Content-Type: application/json

{
  "reason": "Resource constraints",
  "cleanup_partial_results": true
}
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "message": "Session cancelled successfully",
  "session_id": "session_abc123",
  "status": "cancelled",
  "cancelled_at": "2024-01-16T10:30:00Z",
  "cleanup": {
    "partial_results_cleaned": true,
    "processed_files_retained": 75
  }
}
```

## Retry Failed Files

Retry processing for failed files in a session.

### Request

```http
POST /v1/sessions/{session_id}/retry
Authorization: Bearer tk_tenant_123_key
Content-Type: application/json

{
  "file_ids": ["file_error1", "file_error2"],
  "retry_all_failed": false,
  "reset_retry_count": true
}
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "message": "Retry initiated for failed files",
  "session_id": "session_abc123",
  "retried_files": 2,
  "retry_job_id": "retry_def456",
  "estimated_completion": "2024-01-16T11:00:00Z"
}
```

## Get Session Files

List files in a processing session.

### Request

```http
GET /v1/sessions/{session_id}/files?page=1&page_size=50&status=failed&file_type=pdf
Authorization: Bearer tk_tenant_123_key
```

### Query Parameters

- `page` (integer): Page number (default: 1)
- `page_size` (integer): Items per page (default: 50, max: 200)
- `status` (string): Filter by processing status
- `file_type` (string): Filter by file type
- `stage` (string): Filter by processing stage
- `has_errors` (boolean): Filter files with errors

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "session_id": "session_abc123",
  "files": [
    {
      "file_id": "file_abc123",
      "filename": "contract.pdf",
      "file_type": "pdf",
      "file_size": 2048576,
      "status": "processed",
      "processing_status": "completed",
      "stage": "completed",
      "started_at": "2024-01-16T09:05:00Z",
      "completed_at": "2024-01-16T09:07:00Z",
      "processing_time_ms": 2000,
      "chunks_created": 15,
      "embeddings_generated": 15,
      "errors": []
    },
    {
      "file_id": "file_error1",
      "filename": "corrupted_file.pdf",
      "file_type": "pdf",
      "file_size": 1024000,
      "status": "failed",
      "processing_status": "failed",
      "stage": "text_extraction",
      "started_at": "2024-01-16T09:30:00Z",
      "failed_at": "2024-01-16T09:31:00Z",
      "retry_count": 3,
      "errors": [
        {
          "stage": "text_extraction",
          "error_code": "CORRUPTED_FILE",
          "error_message": "Unable to extract text from corrupted PDF",
          "occurred_at": "2024-01-16T09:31:00Z"
        }
      ]
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 50,
    "total_count": 2,
    "total_pages": 1
  }
}
```

## Get Session Summary

Get session processing summary and insights.

### Request

```http
GET /v1/sessions/{session_id}/summary
Authorization: Bearer tk_tenant_123_key
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "session_id": "session_abc123",
  "name": "Q1 Legal Document Review",
  "status": "completed",
  "summary": {
    "total_files": 150,
    "successfully_processed": 148,
    "failed_files": 2,
    "total_processing_time": "2h 30m",
    "average_processing_time_per_file": "1m 2s",
    "total_chunks_created": 2456,
    "total_embeddings_generated": 2456,
    "total_storage_used": "1.2 GB"
  },
  "insights": {
    "document_types": {
      "contract": 89,
      "agreement": 42,
      "policy": 19
    },
    "classification_distribution": {
      "confidential": 120,
      "restricted": 28,
      "internal": 2
    },
    "compliance_analysis": {
      "average_score": 97.2,
      "violations_found": 5,
      "frameworks_evaluated": ["GDPR", "SOX", "HIPAA"]
    },
    "content_analysis": {
      "languages_detected": ["en", "es"],
      "avg_sentiment_score": 0.12,
      "key_topics": [
        {"topic": "payment_terms", "documents": 67},
        {"topic": "liability", "documents": 45},
        {"topic": "termination", "documents": 38}
      ]
    }
  },
  "recommendations": [
    {
      "type": "optimization",
      "message": "Consider using Tier 2 processing for documents larger than 10MB",
      "impact": "20% faster processing"
    },
    {
      "type": "compliance",
      "message": "5 documents contain potential PII violations",
      "action": "Review DLP policy configuration"
    }
  ],
  "export_options": {
    "report_formats": ["pdf", "html", "json"],
    "data_exports": ["embeddings", "metadata", "analysis_results"]
  }
}
```

## Export Session Results

Export session results and analysis.

### Request

```http
POST /v1/sessions/{session_id}/export
Authorization: Bearer tk_tenant_123_key
Content-Type: application/json

{
  "format": "json",
  "include_embeddings": true,
  "include_analysis": true,
  "include_metadata": true,
  "generate_report": true,
  "compression": "zip"
}
```

### Response

```http
HTTP/1.1 202 Accepted
Content-Type: application/json

{
  "export_id": "export_ghi789",
  "session_id": "session_abc123",
  "status": "processing",
  "created_at": "2024-01-16T12:00:00Z",
  "estimated_completion": "2024-01-16T12:15:00Z",
  "download_url": null
}
```

## Session Status Values

### Status

- `created` - Session created but not started
- `scheduled` - Session scheduled for future execution
- `running` - Session currently processing
- `paused` - Session temporarily paused
- `completed` - Session completed successfully
- `cancelled` - Session cancelled by user
- `failed` - Session failed due to errors

### Processing Stages

- `file_discovery` - Discovering files to process
- `text_extraction` - Extracting text from documents
- `chunking` - Creating document chunks
- `embedding_generation` - Generating vector embeddings
- `analysis` - Running ML analysis
- `dlp_scanning` - Data loss prevention scanning
- `compliance_check` - Compliance validation
- `finalization` - Finalizing results

### Session Types

- `batch_processing` - Process multiple files in batch
- `incremental` - Process only new/changed files
- `manual` - Manual processing of specific files
- `scheduled` - Regularly scheduled processing
- `real_time` - Real-time stream processing

## Error Responses

### Session Not Found

```http
HTTP/1.1 404 Not Found

{
  "error": {
    "code": "SESSION_NOT_FOUND",
    "message": "Processing session not found",
    "session_id": "session_invalid"
  }
}
```

### Invalid Session State

```http
HTTP/1.1 400 Bad Request

{
  "error": {
    "code": "INVALID_SESSION_STATE",
    "message": "Cannot start session in current state",
    "current_status": "completed",
    "valid_statuses": ["created", "scheduled", "paused"]
  }
}
```

### Resource Limit Exceeded

```http
HTTP/1.1 429 Too Many Requests

{
  "error": {
    "code": "CONCURRENT_SESSION_LIMIT",
    "message": "Maximum concurrent sessions exceeded",
    "current_sessions": 5,
    "max_allowed": 5,
    "retry_after": 1800
  }
}
```