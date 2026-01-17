# File API Validation Rules

## Overview
This document describes the actual validation behavior of the File API based on comprehensive testing. The API is designed to be highly permissive, favoring data acceptance over strict validation.

## Core Principles

### 1. Permissive by Design
The API accepts minimal data and applies sensible defaults rather than rejecting requests. This approach:
- Reduces client-side complexity
- Allows for flexible data ingestion
- Simplifies integration with various data sources
- Provides graceful handling of incomplete data

### 2. Default Value Strategy
When fields are missing, the API applies defaults:
- **Status**: "discovered" (for new files)
- **Encryption Status**: "none"
- **Processing Tier**: "standard"
- **Timestamps**: Current time for created_at/updated_at
- **IDs**: Auto-generated UUIDs

## Validation Rules by Endpoint

### POST /api/v1/tenants/{tenantId}/files

#### Minimal Required Data
```json
{}  // Even empty JSON is accepted!
```

The API will create a file record with all default values.

#### Recommended Fields
While not strictly required, these fields should be provided for meaningful records:
```json
{
  "filename": "document.pdf",      // Identifies the file
  "extension": "pdf",              // File type
  "content_type": "application/pdf", // MIME type
  "size": 1024                     // File size in bytes
}
```

#### Optional Fields with Validation
```json
{
  "data_source_id": "uuid",        // Must reference existing data source
  "processing_session_id": "uuid", // Must be valid UUID if provided
  "checksum": "sha256:...",        // Any string accepted
  "checksum_type": "sha256",       // Any string accepted
  "metadata": {},                  // Any JSON object
  "custom_fields": {}              // Any JSON object
}
```

#### Validation Behaviors

| Field | Validation | Default | Notes |
|-------|------------|---------|-------|
| filename | None | Empty string allowed | Can be empty or null |
| extension | None | Empty string allowed | Not validated against filename |
| content_type | None | Empty string allowed | Not validated as valid MIME type |
| size | None | 0 if not provided | Negative values accepted |
| data_source_id | Foreign key check | null | Must exist if provided |
| processing_session_id | UUID format | null | Must be valid UUID if provided |
| url | None | Empty string allowed | No URL validation |
| path | None | Empty string allowed | No path validation |
| checksum | None | Empty string allowed | Format not validated |
| metadata | Must be JSON object | {} | Cannot be string or array |

### GET /api/v1/tenants/{tenantId}/files/{fileId}

#### Validation Rules
- **File ID**: Must be valid UUID format
- **Tenant Context**: Required via X-Tenant-ID header
- **File Existence**: Returns 404 if not found
- **Tenant Scoping**: File must belong to requesting tenant

### GET /api/v1/tenants/{tenantId}/files

#### Query Parameters
All query parameters are optional and permissive:

| Parameter | Validation | Example |
|-----------|------------|---------|
| status | Any string | ?status=processed |
| content_type | Any string | ?content_type=text/plain |
| extension | Any string | ?extension=pdf |
| data_source_id | Valid UUID | ?data_source_id=uuid |
| session_id | Valid UUID | ?session_id=uuid |
| pii_detected | "true" or "false" | ?pii_detected=true |

#### Pagination
- **Default page size**: 50
- **Maximum page size**: Not enforced
- **Offset calculation**: Automatic

### POST /api/v1/tenants/{tenantId}/files/{fileId}/process

#### Request Body
```json
{
  "chunking_strategy": "semantic",  // Optional, any string accepted
  "priority": "high",               // Optional, any string accepted
  "dlp_scan_enabled": true          // Optional, boolean
}
```

#### Validation Rules
- **File must exist**: Returns 404 if file not found
- **File must not be processing**: No check - can trigger multiple times
- **Strategy validation**: Any string accepted, defaults used if invalid

### POST /api/v1/tenants/{tenantId}/files/search

#### Request Body
```json
{
  "query": "search terms",     // Required, cannot be empty
  "top_k": 10,                // Optional, default 10
  "threshold": 0.7,            // Optional, default 0.7
  "filters": {}                // Optional, any object
}
```

#### Validation Rules
- **Query**: Required and must be non-empty string
- **Service Availability**: Returns 503 if embedding service unavailable
- **Authentication**: Returns 500 if OpenAI auth fails (not 401)

## Error Response Format

All validation errors follow this format:
```json
{
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable message",
    "details": "Technical details"
  },
  "request_id": "req_123456",
  "timestamp": "2025-08-14T12:00:00Z"
}
```

## Common Error Scenarios

### 1. Foreign Key Violations
**Trigger**: Invalid data_source_id
**Response**: 500 Internal Server Error
```json
{
  "error": {
    "code": "INTERNAL_SERVER_ERROR",
    "message": "Failed to create file",
    "details": "violates foreign key constraint \"files_data_source_id_fkey\""
  }
}
```

### 2. Invalid UUID Format
**Trigger**: Malformed UUID in path
**Response**: 400 Bad Request
```json
{
  "error": {
    "code": "BAD_REQUEST",
    "message": "Invalid file ID format"
  }
}
```

### 3. Missing Tenant Context
**Trigger**: Missing X-Tenant-ID header
**Response**: 400 Bad Request
```json
{
  "error": {
    "code": "BAD_REQUEST",
    "message": "Tenant context required"
  }
}
```

### 4. Service Dependencies
**Trigger**: Embedding service unavailable
**Response**: 503 Service Unavailable or 500 Internal Server Error
```json
{
  "error": {
    "code": "EMBEDDING_SERVICE_UNAVAILABLE",
    "message": "Vector search service is not available"
  }
}
```

## Best Practices

### 1. Client Implementation
- **Always provide recommended fields** even though they're optional
- **Handle 500 errors** for foreign key violations
- **Check service availability** before search operations
- **Use meaningful defaults** in your client code

### 2. Data Quality
- **Validate client-side** for better user experience
- **Provide complete metadata** for better searchability
- **Use consistent file naming** conventions
- **Include checksums** for data integrity

### 3. Error Handling
- **Expect permissive behavior** - API rarely rejects data
- **Handle service unavailability** gracefully
- **Log validation issues** client-side
- **Implement retry logic** for service errors

## Migration Considerations

If migrating from a stricter API:
1. **Remove unnecessary validation** from client code
2. **Rely on defaults** where appropriate
3. **Handle the permissive responses** appropriately
4. **Add client-side validation** if needed for business rules

## Security Notes

While the API is permissive with data validation, it maintains strict security:
- **Tenant isolation** is always enforced
- **Authentication** is required for all operations
- **Cross-tenant access** is prevented at database level
- **SQL injection** is prevented through parameterized queries

## Summary

The File API prioritizes:
1. **Ease of integration** over strict validation
2. **Sensible defaults** over required fields
3. **Graceful degradation** over hard failures
4. **Flexibility** over rigid schemas

This design allows for rapid development and integration while maintaining security and data integrity through other mechanisms.