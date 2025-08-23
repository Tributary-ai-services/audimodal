# AudiModal File Upload Test Suite

This directory contains comprehensive tests for the file upload functionality, including both unit tests and integration tests.

## Test Overview

### ✅ Unit Tests (No Database Dependencies)

**Location**: `internal/server/handlers/file_upload_unit_test.go`

- **Request Validation Tests**: Validates file upload request structure and required fields
- **File Size Threshold Logic**: Tests the 10MB threshold enforcement for multipart vs S3 routing
- **Multipart Form Parsing**: Tests multipart form field extraction and validation
- **JSON Request Parsing**: Tests JSON request parsing and validation logic

**Run Unit Tests**:
```bash
go test -v ./internal/server/handlers -run "TestValidateFileUploadRequest|TestFileSizeThresholdLogic|TestMultipartFormParsing|TestJSONRequestParsing"
```

### 🧪 Integration Tests (Database + Storage Required)

**Location**: `tests/file_upload_integration_test.go`

- **End-to-End File Upload**: Tests complete multipart upload flow with database
- **S3 URL Upload**: Tests JSON-based file registration with S3 URLs
- **File Size Threshold Enforcement**: Tests actual HTTP request size limits
- **Security Tests**: Tests tenant isolation and data source validation
- **Error Handling**: Tests various error scenarios with actual responses

**Location**: `tests/s3_upload_integration_test.go`

- **Presigned URL Generation**: Tests S3 presigned URL creation with MinIO
- **Actual S3 Upload**: Tests uploading files to MinIO using presigned URLs
- **Storage Service Configuration**: Tests storage service configuration and validation

## Running Tests

### Prerequisites

1. **Database**: PostgreSQL running on `localhost:5433`
   ```bash
   docker-compose up -d audimodal-postgres
   ```

2. **MinIO (Optional)**: For S3 integration tests
   ```bash
   # Start shared infrastructure with MinIO
   cd ../aether-shared
   docker-compose up -d minio-shared
   ```

### Test Commands

**Unit Tests Only** (Fast, no dependencies):
```bash
go test -v ./internal/server/handlers -run "Test.*Unit.*|TestValidate.*|TestFileSizeThreshold.*"
```

**Integration Tests** (Requires database):
```bash
# Set environment variables
export TEST_DB_HOST=localhost
export TEST_DB_PORT=5433
export TEST_DB_NAME=audimodal
export TEST_DB_USER=audimodal-admin
export TEST_DB_PASSWORD=eaipassword

# Run integration tests
go test -v ./tests -run TestFileUpload
```

**S3 Integration Tests** (Requires MinIO):
```bash
# Set MinIO environment variables
export AWS_ENDPOINT_URL=http://localhost:9000
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin123
export AWS_REGION=us-east-1
export AWS_S3_BUCKET=audimodal-uploads
export AWS_S3_FORCE_PATH_STYLE=true

# Run S3 tests
go test -v ./tests -run TestS3Upload
```

**All Tests with Script**:
```bash
./tests/run_integration_tests.sh
```

## Test Coverage

### ✅ File Upload Scenarios Tested

1. **Multipart Uploads (≤10MB)**:
   - Valid file uploads with metadata
   - Missing required fields (file, datasource_id)
   - Empty files
   - Invalid JSON metadata
   - File size validation

2. **JSON Uploads (>10MB or S3 URLs)**:
   - Valid S3 URLs
   - Missing required fields
   - Invalid URL formats
   - Unsupported URL schemes
   - URL validation with access checks

3. **Size-Based Routing**:
   - Files exactly 10MB (multipart allowed)
   - Files >10MB (multipart rejected, S3 required)
   - Large files via JSON (50MB+ allowed)

4. **Security & Validation**:
   - Tenant isolation (can't use other tenant's data sources)
   - Data source validation (must exist and belong to tenant)
   - Content type validation
   - Request size limits

5. **S3 Integration**:
   - Presigned URL generation
   - Actual file uploads to MinIO
   - Storage service configuration
   - Environment variable validation

### 🧪 Test Types

| Test Type | Files | Dependencies | Purpose |
|-----------|-------|--------------|---------|
| **Unit** | `*_unit_test.go` | None | Fast validation logic testing |
| **Handler** | `file_handler_test.go` | None | HTTP handler routing and parsing |
| **Integration** | `*_integration_test.go` | Database | End-to-end functionality |
| **S3** | `s3_*_test.go` | Database + MinIO | Storage integration |

## Environment Variables

### Database Configuration
```bash
TEST_DB_HOST=localhost
TEST_DB_PORT=5433
TEST_DB_NAME=audimodal
TEST_DB_USER=audimodal-admin
TEST_DB_PASSWORD=eaipassword
```

### S3/MinIO Configuration
```bash
AWS_ENDPOINT_URL=http://localhost:9000
AWS_ACCESS_KEY_ID=minioadmin
AWS_SECRET_ACCESS_KEY=minioadmin123
AWS_REGION=us-east-1
AWS_S3_BUCKET=audimodal-uploads
AWS_S3_FORCE_PATH_STYLE=true
EAI_ENCRYPTION_KEY=test-encryption-key-32-bytes-xxx
```

## Test Results

All tests are currently passing:

- ✅ **12/12** Unit tests passing
- ✅ **4/4** File size threshold tests passing  
- ✅ **5/5** Multipart form parsing tests passing
- ✅ **4/4** JSON request parsing tests passing

**Integration tests** require proper database and MinIO setup to run.

## Next Steps

1. Add tests for search endpoint error handling
2. Add tests for file metadata and status endpoints  
3. Add file processing queue tests
4. Expand S3 integration tests with error scenarios
5. Add performance tests for large file uploads

## Notes

- Unit tests are designed to be fast and run without external dependencies
- Integration tests provide end-to-end validation but require infrastructure
- S3 tests can be skipped if MinIO is not available (test will show as skipped)
- All tests use the testify framework for assertions and test organization