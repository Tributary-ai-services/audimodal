# Files API

The Files API manages document ingestion, processing, and metadata operations in AudiModal.

## Base URL

```
/v1/tenants/{tenant_id}/files
```

## Upload File

Upload a document for processing.

### Request

```http
POST /v1/tenants/{tenant_id}/files
Authorization: Bearer tk_tenant_123_key
Content-Type: multipart/form-data

Content-Disposition: form-data; name="file"; filename="contract.pdf"
Content-Type: application/pdf

[Binary file content]

Content-Disposition: form-data; name="metadata"
Content-Type: application/json

{
  "document_type": "contract",
  "department": "legal",
  "classification": "confidential",
  "source": "sharepoint",
  "tags": ["contract", "legal", "2024"],
  "custom_fields": {
    "contract_value": 150000,
    "client_name": "Acme Inc",
    "effective_date": "2024-01-01"
  }
}
```

### Form Parameters

- `file` (required): The document file
- `metadata` (optional): JSON metadata object
- `processing_config` (optional): Processing configuration
- `priority` (optional): Processing priority (`low`, `normal`, `high`)

### Response

```http
HTTP/1.1 201 Created
Content-Type: application/json

{
  "id": "file_abc123",
  "tenant_id": "tenant_123abc",
  "filename": "contract.pdf",
  "original_filename": "contract.pdf",
  "file_type": "pdf",
  "file_size": 2048576,
  "checksum": "sha256:abc123def456...",
  "status": "uploaded",
  "processing_status": "queued",
  "created_at": "2024-01-15T14:30:00Z",
  "updated_at": "2024-01-15T14:30:00Z",
  "metadata": {
    "document_type": "contract",
    "department": "legal",
    "classification": "confidential",
    "source": "sharepoint",
    "tags": ["contract", "legal", "2024"],
    "custom_fields": {
      "contract_value": 150000,
      "client_name": "Acme Inc",
      "effective_date": "2024-01-01"
    }
  },
  "storage": {
    "provider": "s3",
    "bucket": "audimodal-tenant-123",
    "key": "files/2024/01/15/file_abc123.pdf",
    "url": "https://api.audimodal.ai/v1/files/file_abc123/download"
  },
  "processing": {
    "tier": null,
    "session_id": null,
    "priority": "normal",
    "estimated_completion": null
  },
  "compliance": {
    "frameworks": ["GDPR", "SOX"],
    "classification": "confidential",
    "contains_pii": null,
    "retention_date": "2031-01-15T14:30:00Z"
  }
}
```

## List Files

List files with filtering and pagination.

### Request

```http
GET /v1/tenants/{tenant_id}/files?page=1&page_size=50&status=processed&file_type=pdf
Authorization: Bearer tk_tenant_123_key
```

### Query Parameters

- `page` (integer): Page number (default: 1)
- `page_size` (integer): Items per page (default: 50, max: 200)
- `status` (string): Filter by status
- `processing_status` (string): Filter by processing status
- `file_type` (string): Filter by file type
- `document_type` (string): Filter by document type
- `classification` (string): Filter by classification level
- `created_after` (datetime): Filter by creation date
- `created_before` (datetime): Filter by creation date
- `search` (string): Full-text search in filename and content
- `tags` (array): Filter by tags
- `sort` (string): Sort field (`created_at`, `filename`, `file_size`)
- `order` (string): Sort order (`asc`, `desc`)

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "files": [
    {
      "id": "file_abc123",
      "filename": "contract.pdf",
      "file_type": "pdf",
      "file_size": 2048576,
      "status": "processed",
      "processing_status": "completed",
      "created_at": "2024-01-15T14:30:00Z",
      "metadata": {
        "document_type": "contract",
        "classification": "confidential",
        "tags": ["contract", "legal", "2024"]
      },
      "processing": {
        "chunks": 15,
        "processing_time_ms": 5400,
        "completed_at": "2024-01-15T14:35:00Z"
      },
      "compliance": {
        "contains_pii": true,
        "dlp_violations": 0,
        "compliance_score": 98.5
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
    "by_type": {
      "pdf": 45,
      "docx": 23,
      "xlsx": 12
    },
    "by_status": {
      "processed": 67,
      "processing": 8,
      "failed": 5
    }
  }
}
```

## Get File

Get detailed file information.

### Request

```http
GET /v1/files/{file_id}
Authorization: Bearer tk_tenant_123_key
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "id": "file_abc123",
  "tenant_id": "tenant_123abc",
  "filename": "contract.pdf",
  "original_filename": "contract.pdf",
  "file_type": "pdf",
  "file_size": 2048576,
  "checksum": "sha256:abc123def456...",
  "status": "processed",
  "processing_status": "completed",
  "created_at": "2024-01-15T14:30:00Z",
  "updated_at": "2024-01-15T14:35:00Z",
  "metadata": {
    "document_type": "contract",
    "department": "legal",
    "classification": "confidential",
    "source": "sharepoint",
    "tags": ["contract", "legal", "2024"],
    "custom_fields": {
      "contract_value": 150000,
      "client_name": "Acme Inc",
      "effective_date": "2024-01-01"
    }
  },
  "content": {
    "extracted_text": "CONTRACT AGREEMENT\n\nThis agreement is entered into...",
    "page_count": 12,
    "word_count": 3456,
    "character_count": 21453,
    "language": "en",
    "format_version": "PDF-1.7"
  },
  "processing": {
    "tier": "tier_1",
    "session_id": "session_def456",
    "priority": "normal",
    "started_at": "2024-01-15T14:30:30Z",
    "completed_at": "2024-01-15T14:35:00Z",
    "processing_time_ms": 5400,
    "chunks": 15,
    "embeddings_generated": true,
    "analysis_completed": true
  },
  "storage": {
    "provider": "s3",
    "bucket": "audimodal-tenant-123",
    "key": "files/2024/01/15/file_abc123.pdf",
    "url": "https://api.audimodal.ai/v1/files/file_abc123/download",
    "encrypted": true,
    "compression": "gzip"
  },
  "compliance": {
    "frameworks": ["GDPR", "SOX"],
    "classification": "confidential",
    "contains_pii": true,
    "pii_types": ["email", "phone", "ssn"],
    "dlp_violations": 0,
    "compliance_score": 98.5,
    "retention_date": "2031-01-15T14:30:00Z",
    "data_subject_rights": ["access", "rectification", "erasure"]
  },
  "analysis": {
    "sentiment": "neutral",
    "sentiment_score": 0.05,
    "entities": [
      {
        "text": "Acme Inc",
        "type": "ORGANIZATION",
        "confidence": 0.98
      },
      {
        "text": "January 1, 2024",
        "type": "DATE",
        "confidence": 0.95
      }
    ],
    "keywords": [
      {
        "term": "contract",
        "relevance": 0.92
      },
      {
        "term": "agreement",
        "relevance": 0.87
      }
    ],
    "topics": [
      {
        "topic": "legal_contract",
        "confidence": 0.94
      }
    ],
    "summary": "A contract agreement between two parties establishing terms and conditions for services.",
    "quality_score": 95.5
  }
}
```

## Update File

Update file metadata and configuration.

### Request

```http
PUT /v1/files/{file_id}
Authorization: Bearer tk_tenant_123_key
Content-Type: application/json

{
  "metadata": {
    "document_type": "contract",
    "classification": "restricted",
    "tags": ["contract", "legal", "2024", "high-value"],
    "custom_fields": {
      "contract_value": 175000,
      "client_name": "Acme Inc",
      "effective_date": "2024-01-01",
      "reviewed_by": "Jane Doe"
    }
  },
  "compliance": {
    "classification": "restricted",
    "retention_date": "2032-01-15T14:30:00Z"
  }
}
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "id": "file_abc123",
  "updated_at": "2024-01-15T16:00:00Z",
  "metadata": {
    "document_type": "contract",
    "classification": "restricted",
    "tags": ["contract", "legal", "2024", "high-value"],
    "custom_fields": {
      "contract_value": 175000,
      "client_name": "Acme Inc",
      "effective_date": "2024-01-01",
      "reviewed_by": "Jane Doe"
    }
  },
  "compliance": {
    "classification": "restricted",
    "retention_date": "2032-01-15T14:30:00Z"
  }
}
```

## Delete File

Delete a file and all associated data.

### Request

```http
DELETE /v1/files/{file_id}
Authorization: Bearer tk_tenant_123_key
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "message": "File deleted successfully",
  "file_id": "file_abc123",
  "deleted_at": "2024-01-15T16:00:00Z",
  "cleanup": {
    "chunks_deleted": 15,
    "embeddings_deleted": 15,
    "storage_freed_bytes": 2048576
  }
}
```

## Download File

Download the original file.

### Request

```http
GET /v1/files/{file_id}/download
Authorization: Bearer tk_tenant_123_key
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/pdf
Content-Length: 2048576
Content-Disposition: attachment; filename="contract.pdf"
X-File-ID: file_abc123
X-Checksum: sha256:abc123def456...

[Binary file content]
```

## Process File

Trigger manual file processing.

### Request

```http
POST /v1/files/{file_id}/process
Authorization: Bearer tk_tenant_123_key
Content-Type: application/json

{
  "force_reprocess": false,
  "priority": "high",
  "processing_options": {
    "extract_text": true,
    "generate_embeddings": true,
    "run_analysis": true,
    "dlp_scan": true,
    "chunking_strategy": "semantic",
    "chunk_size": 1000,
    "chunk_overlap": 100
  }
}
```

### Response

```http
HTTP/1.1 202 Accepted
Content-Type: application/json

{
  "message": "Processing initiated",
  "file_id": "file_abc123",
  "session_id": "session_ghi789",
  "priority": "high",
  "estimated_completion": "2024-01-15T16:10:00Z"
}
```

## Get File Content

Get extracted text content.

### Request

```http
GET /v1/files/{file_id}/content?format=text&include_metadata=true
Authorization: Bearer tk_tenant_123_key
```

### Query Parameters

- `format` (string): Content format (`text`, `html`, `markdown`)
- `include_metadata` (boolean): Include content metadata
- `page` (integer): Specific page number (for multi-page documents)

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "file_id": "file_abc123",
  "content": "CONTRACT AGREEMENT\n\nThis agreement is entered into between...",
  "format": "text",
  "metadata": {
    "extraction_method": "ocr_plus_text",
    "confidence": 0.98,
    "page_count": 12,
    "word_count": 3456,
    "character_count": 21453,
    "language": "en",
    "quality_score": 95.5
  },
  "pages": [
    {
      "page_number": 1,
      "content": "CONTRACT AGREEMENT\n\nThis agreement...",
      "word_count": 287,
      "confidence": 0.99
    }
  ]
}
```

## List File Chunks

Get document chunks for vector operations.

### Request

```http
GET /v1/files/{file_id}/chunks?page=1&page_size=20
Authorization: Bearer tk_tenant_123_key
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "file_id": "file_abc123",
  "chunks": [
    {
      "id": "chunk_123",
      "chunk_index": 0,
      "content": "CONTRACT AGREEMENT\n\nThis agreement is entered into...",
      "content_hash": "sha256:chunk123...",
      "word_count": 245,
      "character_count": 1456,
      "embedding_id": "embedding_456",
      "quality_score": 96.2,
      "metadata": {
        "page_numbers": [1],
        "section": "header",
        "chunk_type": "text"
      }
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total_count": 15,
    "total_pages": 1
  }
}
```

## Get File Violations

Get DLP violations for a file.

### Request

```http
GET /v1/files/{file_id}/violations?severity=high&status=open
Authorization: Bearer tk_tenant_123_key
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "file_id": "file_abc123",
  "violations": [
    {
      "id": "violation_789",
      "policy_id": "policy_pii",
      "severity": "medium",
      "status": "open",
      "violation_type": "pii_detection",
      "detected_at": "2024-01-15T14:35:00Z",
      "content_snippet": "SSN: 123-45-****",
      "location": {
        "page": 3,
        "line": 45,
        "char_start": 1234,
        "char_end": 1245
      },
      "details": {
        "pii_type": "ssn",
        "confidence": 0.98,
        "policy_rule": "detect_ssn_patterns"
      },
      "remediation": {
        "action": "redact",
        "status": "pending",
        "redacted_content": "SSN: ***-**-****"
      }
    }
  ],
  "summary": {
    "total_violations": 1,
    "by_severity": {
      "high": 0,
      "medium": 1,
      "low": 0
    },
    "by_status": {
      "open": 1,
      "resolved": 0,
      "ignored": 0
    }
  }
}
```

## File Status Values

### Status

- `uploaded` - File uploaded successfully
- `processing` - File being processed
- `processed` - Processing completed successfully
- `failed` - Processing failed
- `deleted` - File marked for deletion

### Processing Status

- `queued` - Waiting for processing
- `extracting` - Extracting text content
- `chunking` - Creating document chunks
- `embedding` - Generating embeddings
- `analyzing` - Running ML analysis
- `scanning` - DLP scanning
- `completed` - All processing complete
- `failed` - Processing failed
- `cancelled` - Processing cancelled

### Classification Levels

- `public` - Public information
- `internal` - Internal use only
- `confidential` - Confidential information
- `restricted` - Highly restricted access
- `top_secret` - Top secret classification

## Error Responses

### File Not Found

```http
HTTP/1.1 404 Not Found

{
  "error": {
    "code": "FILE_NOT_FOUND",
    "message": "File not found",
    "file_id": "file_invalid"
  }
}
```

### Unsupported File Type

```http
HTTP/1.1 400 Bad Request

{
  "error": {
    "code": "UNSUPPORTED_FILE_TYPE",
    "message": "Unsupported file type",
    "file_type": "xyz",
    "supported_types": ["pdf", "docx", "xlsx", "txt", "csv", "json"]
  }
}
```

### File Too Large

```http
HTTP/1.1 413 Payload Too Large

{
  "error": {
    "code": "FILE_TOO_LARGE",
    "message": "File exceeds maximum size limit",
    "file_size": 536870912,
    "max_size": 268435456
  }
}
```

### Processing Failed

```http
HTTP/1.1 422 Unprocessable Entity

{
  "error": {
    "code": "PROCESSING_FAILED",
    "message": "File processing failed",
    "file_id": "file_abc123",
    "details": {
      "stage": "text_extraction",
      "error": "Corrupted PDF file",
      "retry_possible": false
    }
  }
}
```