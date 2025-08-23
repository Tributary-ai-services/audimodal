# Test Case: Create Large File Record

## Overview
This test validates the creation of a file record for a large file (1MB), demonstrating how the system handles size limits and large file metadata tracking.

## Call Flow

```mermaid
sequenceDiagram
    participant Test as Test Case
    participant Client as HTTP Client
    participant API as AudiModal API
    participant Handler as FileHandler
    participant DB as Database
    participant Response as Response Writer

    Test->>Test: Prepare large file data
    Note over Test: {<br/>  "filename": "large_doc.txt",<br/>  "extension": "txt",<br/>  "content_type": "text/plain",<br/>  "size": 1048576, // 1MB<br/>  "checksum": "large-file-checksum",<br/>  "metadata": {"size_test": "large"}<br/>}
    
    Test->>Client: Create POST request
    Note over Client: URL: /api/v1/tenants/{tenantId}/files<br/>Headers: Content-Type: application/json<br/>X-Tenant-ID: 9855e094...
    
    Client->>API: POST /api/v1/tenants/{tenantId}/files
    
    API->>API: Extract tenant context
    API->>API: Route to FileHandler
    
    API->>Handler: ServeHTTP(w, r)
    
    Handler->>Handler: Parse URL path
    Handler->>Handler: Validate tenant context
    Handler->>Handler: Route to CreateFile()
    
    Handler->>Handler: Decode JSON body
    Note over Handler: size: 1048576 bytes (1MB)
    Handler->>DB: Get tenant repository
    
    DB-->>Handler: Tenant repository
    
    Handler->>Handler: Create File model
    Note over Handler: Set status: "discovered"<br/>Set encryption: "none"<br/>Generate UUID<br/>size: 1048576
    
    Handler->>DB: ValidateAndCreate(file)
    
    DB->>DB: Validate required fields
    Note over DB: No size limit validation<br/>at database level
    DB->>DB: Insert into files table
    DB-->>Handler: Created file record
    
    Handler->>Response: WriteCreated(201, file)
    Response->>Client: HTTP 201 Created
    Note over Client: Response body:<br/>{<br/>  "success": true,<br/>  "data": {<br/>    "id": "3beb7c42...",<br/>    "filename": "large_doc.txt",<br/>    "size": 1048576,<br/>    ...<br/>  }<br/>}
    
    Client->>Test: Return response
    Test->>Test: Validate response
    Note over Test: - Check status code = 201<br/>- Verify file ID exists<br/>- Check size = 1048576<br/>- Verify large file handling<br/>- Confirm status = "discovered"
```

## Request Details

### Endpoint
```
POST /api/v1/tenants/9855e094-36a6-4d3a-a4f5-d77da4614439/files
```

### Headers
```
Content-Type: application/json
X-Tenant-ID: 9855e094-36a6-4d3a-a4f5-d77da4614439
```

### Request Body
```json
{
  "filename": "large_doc.txt",
  "extension": "txt",
  "content_type": "text/plain",
  "size": 1048576,
  "checksum": "large-file-checksum",
  "checksum_type": "sha256",
  "metadata": {
    "size_test": "large"
  }
}
```

### File Size Details
- **Size**: 1,048,576 bytes (1 MB)
- **Purpose**: Test large file handling
- **Category**: Large document processing

## Response Details

### Success Response (201 Created)
```json
{
  "success": true,
  "data": {
    "id": "3beb7c42-1f71-456a-827d-4f2f9f50aab3",
    "tenant_id": "9855e094-36a6-4d3a-a4f5-d77da4614439",
    "filename": "large_doc.txt",
    "extension": "txt",
    "content_type": "text/plain",
    "size": 1048576,
    "checksum": "large-file-checksum",
    "checksum_type": "sha256",
    "status": "discovered",
    "encryption_status": "none",
    "metadata": {
      "size_test": "large"
    },
    "created_at": "2025-08-14T01:12:00Z",
    "updated_at": "2025-08-14T01:12:00Z"
  },
  "timestamp": "2025-08-14T01:12:00Z",
  "request_id": "req_123456792"
}
```

## Key Implementation Details

1. **Large File Support**: System accepts 1MB file without size restrictions
2. **Size Tracking**: File size is stored as integer (1048576 bytes)
3. **No Size Limits**: API doesn't enforce file size limits at record level
4. **Metadata Tagging**: Large files can be tagged for special handling
5. **Performance**: Large file metadata creation performs normally

## Test Validations

1. **Status Code**: Verify HTTP 201 Created
2. **File ID**: Check that a UUID is generated
3. **Size Preservation**: Ensure exact size (1048576) is stored
4. **Large File Metadata**: Verify "size_test": "large" metadata
5. **No Size Errors**: Confirm no validation errors for large size
6. **Response Time**: Ensure reasonable performance for large file metadata

## Size Comparison with Other Tests

| Test Case | File Size | Notes |
|-----------|-----------|--------|
| Text File | 85 bytes | Small text content |
| PDF File | 1,024 bytes | Medium document |
| JSON File | 58 bytes | Small structured data |
| No Metadata | 19 bytes | Minimal content |
| **Large File** | **1,048,576 bytes** | **1MB large document** |

## Large File Considerations

### Storage Impact
- **Database**: Only metadata stored (negligible impact)
- **Actual Storage**: File content stored separately
- **Indexing**: May require special handling for large content
- **Processing**: Could require chunking strategies

### Processing Implications
1. **Memory Usage**: Large files may need streaming processing
2. **Embedding Generation**: Might require text chunking
3. **Search Indexing**: May need segmented indexing
4. **Network Transfer**: Could require resumable uploads

## Use Cases for Large Files

1. **Documents**: Large PDFs, presentations, reports
2. **Data Files**: CSV exports, database dumps
3. **Media Files**: Images, audio (if supported)
4. **Archive Files**: ZIP, compressed documents
5. **Log Files**: System logs, application logs

## Notes

- The API handles large file metadata without size restrictions
- Actual file content is not uploaded through this endpoint
- Large files might require special processing considerations
- Metadata tagging helps identify files needing special handling
- Size is stored as integer - supports files up to several GB
- Production systems should consider storage and processing limits
- Large file processing may benefit from chunking strategies