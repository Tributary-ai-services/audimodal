# Test Case: File List and Filtering

## Overview
This test validates the ability to list files for a tenant and filter them by content type, demonstrating the file listing and filtering functionality.

## Call Flow

```mermaid
sequenceDiagram
    participant Test as Test Case
    participant Client as HTTP Client
    participant API as AudiModal API
    participant FileHandler as FileHandler
    participant DB as Database
    participant Response as Response Writer

    Test->>Test: Create multiple test files
    Note over Test: Creates:<br/>- "list_test_1.txt" (text/plain)<br/>- "list_test_2.pdf" (application/pdf)
    
    loop For each test file
        Test->>Client: POST /api/v1/tenants/{tenantId}/files
        Client->>API: Create file request
        API->>FileHandler: CreateFile()
        FileHandler->>DB: Insert file record
        DB-->>FileHandler: File created
        FileHandler->>Response: WriteCreated(201, file)
        Response->>Client: HTTP 201 Created
        Client->>Test: File creation successful
    end

    Test->>Test: Test 1: List all files
    Test->>Client: GET /api/v1/tenants/{tenantId}/files
    Note over Client: URL: /api/v1/tenants/{tenantId}/files<br/>Headers: X-Tenant-ID: {tenantId}
    
    Client->>API: List files request
    
    API->>API: Extract tenant context
    API->>API: Route to FileHandler
    
    API->>FileHandler: ServeHTTP(w, r)
    
    FileHandler->>FileHandler: Parse URL path
    FileHandler->>FileHandler: Validate tenant context
    FileHandler->>FileHandler: Route to ListFiles()
    
    FileHandler->>FileHandler: Extract pagination parameters
    Note over FileHandler: page, pageSize, offset from context
    
    FileHandler->>DB: Get tenant repository
    DB-->>FileHandler: Tenant repository
    
    FileHandler->>DB: Query files with pagination
    Note over DB: SELECT * FROM files<br/>WHERE tenant_id = ?<br/>ORDER BY created_at DESC<br/>LIMIT ? OFFSET ?
    
    DB-->>FileHandler: List of files
    
    FileHandler->>FileHandler: Convert to response format
    FileHandler->>DB: Get total count
    Note over DB: SELECT COUNT(*) FROM files<br/>WHERE tenant_id = ?
    DB-->>FileHandler: Total count
    
    FileHandler->>Response: WritePaginated(200, files, pagination)
    Response->>Client: HTTP 200 OK
    Note over Client: Response:<br/>{<br/>  "success": true,<br/>  "data": [file1, file2, ...],<br/>  "total": N,<br/>  "page": 1,<br/>  "page_size": 50<br/>}
    
    Client->>Test: List of all files
    Test->>Test: Validate: at least 2 files returned

    Test->>Test: Test 2: Filter by content type
    Test->>Client: GET /api/v1/tenants/{tenantId}/files?content_type=text/plain
    Note over Client: URL with query parameter:<br/>?content_type=text/plain
    
    Client->>API: Filtered list request
    API->>FileHandler: ListFiles() with filter
    
    FileHandler->>FileHandler: Extract filter parameters
    Note over FileHandler: content_type = "text/plain"
    
    FileHandler->>DB: Query with content type filter
    Note over DB: SELECT * FROM files<br/>WHERE tenant_id = ?<br/>AND content_type = ?<br/>ORDER BY created_at DESC
    
    DB-->>FileHandler: Filtered files
    FileHandler->>Response: WritePaginated(200, filtered_files)
    Response->>Client: HTTP 200 OK
    
    Client->>Test: Filtered file list
    Test->>Test: Validate: only text/plain files returned
    Note over Test: Check each file has<br/>content_type = "text/plain"
```

## Request Details

### Step 1: Create Test Files (Setup)
```
POST /api/v1/tenants/9855e094-36a6-4d3a-a4f5-d77da4614439/files
```

**File 1 - Text File:**
```json
{
  "filename": "list_test_1.txt",
  "extension": "txt",
  "content_type": "text/plain",
  "size": 100,
  "checksum": "checksum1",
  "checksum_type": "sha256"
}
```

**File 2 - PDF File:**
```json
{
  "filename": "list_test_2.pdf",
  "extension": "pdf",
  "content_type": "application/pdf",
  "size": 200,
  "checksum": "checksum2",
  "checksum_type": "sha256"
}
```

### Step 2: List All Files
```
GET /api/v1/tenants/9855e094-36a6-4d3a-a4f5-d77da4614439/files
```

### Step 3: Filter by Content Type
```
GET /api/v1/tenants/9855e094-36a6-4d3a-a4f5-d77da4614439/files?content_type=text/plain
```

**Headers (for all requests):**
```
X-Tenant-ID: 9855e094-36a6-4d3a-a4f5-d77da4614439
```

## Response Details

### List All Files Response (200 OK)
```json
{
  "success": true,
  "data": [
    {
      "id": "file-uuid-1",
      "tenant_id": "9855e094-36a6-4d3a-a4f5-d77da4614439",
      "filename": "list_test_1.txt",
      "extension": "txt",
      "content_type": "text/plain",
      "size": 100,
      "created_at": "2025-08-14T01:31:00Z"
    },
    {
      "id": "file-uuid-2",
      "tenant_id": "9855e094-36a6-4d3a-a4f5-d77da4614439",
      "filename": "list_test_2.pdf",
      "extension": "pdf",
      "content_type": "application/pdf",
      "size": 200,
      "created_at": "2025-08-14T01:31:01Z"
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 50,
    "total": 19,
    "offset": 0
  },
  "timestamp": "2025-08-14T01:31:02Z",
  "request_id": "req_123456794"
}
```

### Filtered List Response (200 OK)
```json
{
  "success": true,
  "data": [
    {
      "id": "file-uuid-1",
      "tenant_id": "9855e094-36a6-4d3a-a4f5-d77da4614439",
      "filename": "list_test_1.txt",
      "extension": "txt",
      "content_type": "text/plain",
      "size": 100,
      "created_at": "2025-08-14T01:31:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 50,
    "total": 8,
    "offset": 0
  },
  "timestamp": "2025-08-14T01:31:03Z",
  "request_id": "req_123456795"
}
```

## Key Implementation Details

1. **Pagination**: Default page size is 50, results are paginated
2. **Sorting**: Files are ordered by `created_at DESC` (newest first)
3. **Filtering**: Supports multiple filter parameters in query string
4. **Tenant Scoping**: Only files for the specific tenant are returned
5. **Count Query**: Separate query to get total count for pagination

## Available Filters

The API supports these query parameters:

| Filter | Example | Description |
|--------|---------|-------------|
| `status` | `?status=discovered` | Filter by file status |
| `content_type` | `?content_type=text/plain` | Filter by MIME type |
| `extension` | `?extension=pdf` | Filter by file extension |
| `data_source_id` | `?data_source_id=uuid` | Filter by data source |
| `session_id` | `?session_id=uuid` | Filter by processing session |
| `pii_detected` | `?pii_detected=true` | Filter by PII detection status |

## Test Validations

### List All Files Test:
1. **Status Code**: Verify HTTP 200 OK
2. **Data Array**: Confirm response contains file array
3. **Minimum Count**: At least 2 files returned (the ones we created)
4. **Pagination**: Check pagination metadata is present
5. **File Fields**: Verify each file has required fields

### Filter by Content Type Test:
1. **Status Code**: Verify HTTP 200 OK
2. **Filter Accuracy**: All returned files have correct content_type
3. **Exclusion**: Files with different content types are excluded
4. **Data Integrity**: Filtered files contain complete data

## Database Query Patterns

### List All Files:
```sql
SELECT * FROM files 
WHERE tenant_id = ? 
ORDER BY created_at DESC 
LIMIT ? OFFSET ?
```

### With Content Type Filter:
```sql
SELECT * FROM files 
WHERE tenant_id = ? 
AND content_type = ? 
ORDER BY created_at DESC 
LIMIT ? OFFSET ?
```

### Count Query:
```sql
SELECT COUNT(*) FROM files 
WHERE tenant_id = ? 
[AND filter_conditions...]
```

## Use Cases

1. **File Management UI**: Display files in admin interfaces
2. **Content Type Analysis**: Group files by type for processing
3. **Audit and Reporting**: List files for compliance reporting
4. **Bulk Operations**: Find files matching criteria for batch processing
5. **Storage Analysis**: Understand file distribution and usage

## Performance Considerations

- **Indexing**: Queries use indexed fields (tenant_id, created_at)
- **Pagination**: Prevents large result sets from overwhelming clients
- **Efficient Filtering**: Database-level filtering reduces data transfer
- **Separate Count**: Count query runs separately for accurate pagination

## Notes

- The test creates files specifically for testing list functionality
- Filters can be combined (e.g., `?content_type=text/plain&status=discovered`)
- Total count includes all files matching filters, not just current page
- Files from previous tests may appear in the list (total > 2)
- Pagination defaults handle large datasets automatically
- Tenant isolation ensures security across multi-tenant usage