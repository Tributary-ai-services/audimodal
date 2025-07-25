# Tenants API

Tenants represent organizations in the AudiModal platform. Each tenant has isolated data, users, and configuration.

## Base URL

```
/v1/tenants
```

## Create Tenant

Create a new tenant organization.

### Request

```http
POST /v1/tenants
Authorization: Bearer sk_live_admin_key
Content-Type: application/json

{
  "name": "Acme Corporation",
  "slug": "acme-corp",
  "contact_email": "admin@acmecorp.com",
  "contact_name": "John Smith",
  "contact_phone": "+1-555-123-4567",
  "industry": "financial_services",
  "size": "enterprise",
  "country": "US",
  "timezone": "America/New_York",
  "settings": {
    "compliance_frameworks": ["GDPR", "HIPAA", "SOX"],
    "data_retention_days": 2557,
    "encryption_at_rest": true,
    "audit_logging": true
  },
  "quotas": {
    "max_users": 500,
    "max_documents_per_month": 100000,
    "max_storage_gb": 1000,
    "max_api_requests_per_hour": 10000
  }
}
```

### Response

```http
HTTP/1.1 201 Created
Content-Type: application/json

{
  "id": "tenant_123abc",
  "name": "Acme Corporation",
  "slug": "acme-corp",
  "status": "active",
  "contact_email": "admin@acmecorp.com",
  "contact_name": "John Smith",
  "contact_phone": "+1-555-123-4567",
  "industry": "financial_services",
  "size": "enterprise",
  "country": "US",
  "timezone": "America/New_York",
  "created_at": "2024-01-01T12:00:00Z",
  "updated_at": "2024-01-01T12:00:00Z",
  "settings": {
    "compliance_frameworks": ["GDPR", "HIPAA", "SOX"],
    "data_retention_days": 2557,
    "encryption_at_rest": true,
    "audit_logging": true,
    "dlp_enabled": true,
    "analytics_enabled": true
  },
  "quotas": {
    "max_users": 500,
    "max_documents_per_month": 100000,
    "max_storage_gb": 1000,
    "max_api_requests_per_hour": 10000
  },
  "usage": {
    "users": 0,
    "documents_this_month": 0,
    "storage_used_gb": 0.0,
    "api_requests_this_hour": 0
  },
  "compliance_score": 100.0,
  "subscription": {
    "plan": "enterprise",
    "billing_email": "billing@acmecorp.com",
    "next_billing_date": "2024-02-01T00:00:00Z"
  }
}
```

## List Tenants

List all tenants (admin only).

### Request

```http
GET /v1/tenants?page=1&page_size=50&status=active
Authorization: Bearer sk_live_admin_key
```

### Query Parameters

- `page` (integer): Page number (default: 1)
- `page_size` (integer): Items per page (default: 50, max: 100)
- `status` (string): Filter by status (`active`, `suspended`, `pending`)
- `industry` (string): Filter by industry
- `size` (string): Filter by company size
- `search` (string): Search by name or slug

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "tenants": [
    {
      "id": "tenant_123abc",
      "name": "Acme Corporation",
      "slug": "acme-corp",
      "status": "active",
      "industry": "financial_services",
      "size": "enterprise",
      "created_at": "2024-01-01T12:00:00Z",
      "usage": {
        "users": 45,
        "documents_this_month": 12500,
        "storage_used_gb": 156.7,
        "api_requests_this_hour": 234
      },
      "compliance_score": 98.5
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 50,
    "total_count": 1,
    "total_pages": 1,
    "has_next": false,
    "has_previous": false
  }
}
```

## Get Tenant

Get tenant details.

### Request

```http
GET /v1/tenants/{tenant_id}
Authorization: Bearer tk_tenant_123_key
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "id": "tenant_123abc",
  "name": "Acme Corporation",
  "slug": "acme-corp",
  "status": "active",
  "contact_email": "admin@acmecorp.com",
  "contact_name": "John Smith",
  "contact_phone": "+1-555-123-4567",
  "industry": "financial_services",
  "size": "enterprise",
  "country": "US",
  "timezone": "America/New_York",
  "created_at": "2024-01-01T12:00:00Z",
  "updated_at": "2024-01-15T09:30:00Z",
  "settings": {
    "compliance_frameworks": ["GDPR", "HIPAA", "SOX"],
    "data_retention_days": 2557,
    "encryption_at_rest": true,
    "audit_logging": true,
    "dlp_enabled": true,
    "analytics_enabled": true
  },
  "quotas": {
    "max_users": 500,
    "max_documents_per_month": 100000,
    "max_storage_gb": 1000,
    "max_api_requests_per_hour": 10000
  },
  "usage": {
    "users": 45,
    "documents_this_month": 12500,
    "storage_used_gb": 156.7,
    "api_requests_this_hour": 234
  },
  "compliance_score": 98.5,
  "subscription": {
    "plan": "enterprise",
    "billing_email": "billing@acmecorp.com",
    "next_billing_date": "2024-02-01T00:00:00Z",
    "mrr": 2499.00,
    "currency": "USD"
  }
}
```

## Update Tenant

Update tenant configuration.

### Request

```http
PUT /v1/tenants/{tenant_id}
Authorization: Bearer tk_tenant_123_admin_key
Content-Type: application/json

{
  "name": "Acme Corporation Ltd",
  "contact_email": "admin@acmecorp.com",
  "contact_phone": "+1-555-123-4567",
  "timezone": "America/New_York",
  "settings": {
    "compliance_frameworks": ["GDPR", "HIPAA", "SOX", "PCI_DSS"],
    "data_retention_days": 2557,
    "encryption_at_rest": true,
    "audit_logging": true,
    "dlp_enabled": true,
    "analytics_enabled": true
  }
}
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "id": "tenant_123abc",
  "name": "Acme Corporation Ltd",
  "updated_at": "2024-01-15T14:30:00Z",
  "settings": {
    "compliance_frameworks": ["GDPR", "HIPAA", "SOX", "PCI_DSS"],
    "data_retention_days": 2557,
    "encryption_at_rest": true,
    "audit_logging": true,
    "dlp_enabled": true,
    "analytics_enabled": true
  }
}
```

## Delete Tenant

Delete a tenant (admin only). This is a destructive operation.

### Request

```http
DELETE /v1/tenants/{tenant_id}
Authorization: Bearer sk_live_admin_key
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "message": "Tenant scheduled for deletion",
  "tenant_id": "tenant_123abc",
  "deletion_scheduled_at": "2024-01-15T14:30:00Z",
  "data_retention_until": "2024-04-15T14:30:00Z"
}
```

## Get Tenant Statistics

Get detailed tenant usage and performance statistics.

### Request

```http
GET /v1/tenants/{tenant_id}/stats?period=30d&include_details=true
Authorization: Bearer tk_tenant_123_key
```

### Query Parameters

- `period` (string): Time period (`1d`, `7d`, `30d`, `90d`)
- `include_details` (boolean): Include detailed breakdowns

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "tenant_id": "tenant_123abc",
  "period": "30d",
  "usage": {
    "documents_processed": 12500,
    "api_requests": 156000,
    "storage_used_gb": 156.7,
    "users_active": 42,
    "processing_hours": 125.5
  },
  "performance": {
    "avg_processing_time_ms": 1250,
    "api_avg_response_time_ms": 185,
    "uptime_percentage": 99.97,
    "error_rate_percentage": 0.12
  },
  "compliance": {
    "score": 98.5,
    "violations": 2,
    "policies_evaluated": 45000,
    "dlp_scans": 12500
  },
  "costs": {
    "total_usd": 2499.00,
    "compute_usd": 1200.00,
    "storage_usd": 156.70,
    "api_usd": 234.50,
    "ml_analysis_usd": 907.80
  },
  "details": {
    "documents_by_type": {
      "pdf": 8500,
      "docx": 2300,
      "xlsx": 1200,
      "txt": 500
    },
    "processing_by_tier": {
      "tier_1": 9800,
      "tier_2": 2100,
      "tier_3": 600
    },
    "api_by_endpoint": {
      "/v1/files": 89000,
      "/v1/embeddings/search": 34000,
      "/v1/sessions": 12000,
      "/v1/analytics": 21000
    }
  }
}
```

## Export Tenant Data

Export all tenant data for backup or migration.

### Request

```http
POST /v1/tenants/{tenant_id}/export
Authorization: Bearer tk_tenant_123_admin_key
Content-Type: application/json

{
  "format": "json",
  "include_documents": true,
  "include_vectors": true,
  "include_analytics": true,
  "encryption_enabled": true
}
```

### Response

```http
HTTP/1.1 202 Accepted
Content-Type: application/json

{
  "export_id": "export_abc123",
  "status": "in_progress",
  "created_at": "2024-01-15T14:30:00Z",
  "estimated_completion": "2024-01-15T16:00:00Z",
  "download_url": null
}
```

### Check Export Status

```http
GET /v1/tenants/{tenant_id}/exports/{export_id}
Authorization: Bearer tk_tenant_123_admin_key
```

```json
{
  "export_id": "export_abc123",
  "status": "completed",
  "created_at": "2024-01-15T14:30:00Z",
  "completed_at": "2024-01-15T15:45:00Z",
  "download_url": "https://exports.audimodal.ai/tenant_123abc/export_abc123.zip",
  "expires_at": "2024-01-22T15:45:00Z",
  "file_size_bytes": 1073741824,
  "checksum": "sha256:abc123def456..."
}
```

## Tenant Settings

### Available Settings

| Setting | Type | Description | Default |
|---------|------|-------------|---------|
| `compliance_frameworks` | array | Enabled compliance frameworks | `["GDPR"]` |
| `data_retention_days` | integer | Data retention period | `365` |
| `encryption_at_rest` | boolean | Enable encryption at rest | `true` |
| `audit_logging` | boolean | Enable audit logging | `true` |
| `dlp_enabled` | boolean | Enable DLP scanning | `true` |
| `analytics_enabled` | boolean | Enable ML analytics | `true` |
| `auto_classification` | boolean | Enable auto document classification | `true` |
| `webhook_url` | string | Webhook endpoint URL | `null` |
| `webhook_events` | array | Subscribed webhook events | `[]` |

### Compliance Frameworks

- `GDPR` - General Data Protection Regulation
- `HIPAA` - Health Insurance Portability and Accountability Act
- `SOX` - Sarbanes-Oxley Act
- `PCI_DSS` - Payment Card Industry Data Security Standard
- `CCPA` - California Consumer Privacy Act
- `ISO_27001` - ISO/IEC 27001 Information Security Management

### Industry Types

- `financial_services`
- `healthcare`
- `legal`
- `insurance`
- `government`
- `education`
- `technology`
- `manufacturing`
- `retail`
- `other`

### Company Sizes

- `startup` (1-10 employees)
- `small` (11-50 employees)
- `medium` (51-200 employees)
- `large` (201-1000 employees)
- `enterprise` (1000+ employees)

## Error Responses

### Tenant Not Found

```http
HTTP/1.1 404 Not Found

{
  "error": {
    "code": "TENANT_NOT_FOUND",
    "message": "Tenant not found",
    "tenant_id": "tenant_invalid"
  }
}
```

### Quota Exceeded

```http
HTTP/1.1 403 Forbidden

{
  "error": {
    "code": "QUOTA_EXCEEDED",
    "message": "Monthly document quota exceeded",
    "quota_type": "documents_per_month",
    "limit": 100000,
    "current": 100001
  }
}
```

### Invalid Compliance Framework

```http
HTTP/1.1 400 Bad Request

{
  "error": {
    "code": "INVALID_COMPLIANCE_FRAMEWORK",
    "message": "Unsupported compliance framework",
    "framework": "INVALID_FRAMEWORK",
    "supported": ["GDPR", "HIPAA", "SOX", "PCI_DSS", "CCPA", "ISO_27001"]
  }
}
```