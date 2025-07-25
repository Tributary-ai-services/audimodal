# DeepLake API Migration Guide

This document describes the migration from the mock DeepLake implementation to the actual DeepLake API service.

## Overview

The AudiModal application has been updated to use the actual DeepLake API service instead of the previous mock implementation. This provides:

- **Real vector database functionality** using DeepLake's optimized storage and search capabilities
- **Production-ready performance** with proper indexing and caching
- **Advanced search features** including hybrid search and multi-dataset queries
- **Comprehensive API** with import/export, backup/restore, and monitoring capabilities
- **Multi-tenant support** for enterprise deployments
- **Clean architecture** with no legacy mock code

## Architecture Changes

### Before (Mock Implementation)
```
AudiModal Server
├── HTTP Handlers
├── Embedding Service
└── Mock DeepLake Store (in-memory maps)
```

### After (API Integration)
```
AudiModal Server                    DeepLake API Service
├── HTTP Handlers                   ├── FastAPI REST API
├── Embedding Service               ├── Real DeepLake Integration
└── DeepLake API Client ────────────┤ ├── Vector Search & Storage
                                    ├── Import/Export Features
                                    ├── Backup/Restore
                                    └── Multi-tenant Support
```

## Key Changes

### 1. New API Client
- **File**: `pkg/embeddings/client/deeplake_client.go`
- **Purpose**: HTTP client implementing the VectorStore interface
- **Features**: Full API coverage, retry logic, error handling, type safety

### 2. Updated Handlers
- **File**: `internal/server/handlers/embeddings.go`
- **Changes**: Completely replaced mock implementation with real API client
- **Features**: Direct API access with comprehensive endpoint coverage

### 3. Consolidated API Handlers
- **File**: `internal/server/handlers/embeddings.go`
- **Purpose**: Single handler file with all DeepLake API functionality
- **Endpoints**: Full CRUD operations, search variants, vector management

### 4. Configuration Updates
- **File**: `internal/server/config.go`
- **Addition**: `DeepLakeAPIConfig` struct with comprehensive settings
- **Features**: Connection pooling, TLS, health checks, caching, rate limiting

### 5. Updated Configuration File
- **File**: `config/server.yaml`
- **Addition**: Complete DeepLake API configuration section
- **Environment**: Support for environment variable overrides

### 6. Removed Files
- **Removed**: `pkg/embeddings/vectorstore/deeplake.go` (mock implementation)
- **Removed**: `internal/server/handlers/deeplake_api.go` (consolidated into embeddings.go)
- **Removed**: `pkg/embeddings/vectorstore/` directory

## API Endpoint Mapping

| Current Endpoint | DeepLake API Endpoint | Status |
|---|---|---|
| `POST /api/v1/embeddings/documents` | Uses batch vector insert | ✅ Working |
| `POST /api/v1/embeddings/search` | `POST /api/v1/datasets/{id}/search` | ✅ Working |
| `GET /api/v1/embeddings/datasets` | `GET /api/v1/datasets/` | ✅ Working |
| `POST /api/v1/embeddings/datasets` | `POST /api/v1/datasets/` | ✅ Working |
| `GET /api/v1/embeddings/datasets/{name}` | `GET /api/v1/datasets/{id}` | ✅ Working |
| `GET /api/v1/embeddings/datasets/{name}/stats` | `GET /api/v1/datasets/{id}/stats` | ✅ Working |

### New Available Endpoints

The DeepLake API provides many additional endpoints not previously available:

| New Endpoint | Description |
|---|---|
| `POST /api/v1/deeplake/datasets/{id}/search/text` | Text-based semantic search |
| `POST /api/v1/deeplake/datasets/{id}/search/hybrid` | Hybrid vector + text search |
| `POST /api/v1/deeplake/search/multi-dataset` | Cross-dataset search |
| `POST /api/v1/deeplake/datasets/{id}/vectors/batch` | Batch vector operations |
| `POST /api/v1/deeplake/datasets/{id}/index` | Index management |
| `POST /api/v1/deeplake/datasets/{id}/import` | Data import from files |
| `POST /api/v1/deeplake/datasets/{id}/export` | Data export to files |
| `GET /api/v1/deeplake/health` | Service health checks |
| `GET /api/v1/deeplake/metrics` | Service metrics |

## Configuration

### Environment Variables

Set these environment variables to configure the DeepLake API connection:

```bash
# Required
DEEPLAKE_API_URL=http://localhost:8000
DEEPLAKE_API_KEY=your-api-key-here

# Optional
DEEPLAKE_TENANT_ID=your-tenant-id
DEEPLAKE_API_TIMEOUT=30s
DEEPLAKE_API_RETRIES=3
DEEPLAKE_API_USER_AGENT=AudiModal-Go/1.0

# Connection pooling
DEEPLAKE_API_MAX_IDLE_CONNS=10
DEEPLAKE_API_MAX_CONNS_PER_HOST=100
DEEPLAKE_API_IDLE_CONN_TIMEOUT=90s

# TLS (for production)
DEEPLAKE_API_TLS_INSECURE_SKIP_VERIFY=false
DEEPLAKE_API_TLS_CERT_FILE=/path/to/client.crt
DEEPLAKE_API_TLS_KEY_FILE=/path/to/client.key
DEEPLAKE_API_TLS_CA_FILE=/path/to/ca.crt

# Health checks
DEEPLAKE_API_HEALTH_CHECK_ENABLED=true
DEEPLAKE_API_HEALTH_CHECK_INTERVAL=30s
DEEPLAKE_API_HEALTH_CHECK_TIMEOUT=10s

# Caching
DEEPLAKE_API_CACHE_ENABLED=true
DEEPLAKE_API_CACHE_TTL=5m
DEEPLAKE_API_CACHE_MAX_SIZE=1000

# Rate limiting
DEEPLAKE_API_RATE_LIMIT_ENABLED=false
DEEPLAKE_API_RATE_LIMIT_RPS=100
DEEPLAKE_API_RATE_LIMIT_BURST=200
```

### YAML Configuration

Add this section to your `config/server.yaml`:

```yaml
deeplake_api:
  base_url: "http://localhost:8000"
  api_key: "your-api-key"
  tenant_id: ""
  timeout: "30s"
  retries: 3
  user_agent: "AudiModal-Go/1.0"
  
  max_idle_conns: 10
  max_conns_per_host: 100
  idle_conn_timeout: "90s"
  
  tls_insecure_skip_verify: false
  tls_cert_file: ""
  tls_key_file: ""
  tls_ca_file: ""
  
  health_check_enabled: true
  health_check_interval: "30s"
  health_check_timeout: "10s"
  
  cache_enabled: true
  cache_ttl: "5m"
  cache_max_size: 1000
  
  rate_limit_enabled: false
  rate_limit_rps: 100.0
  rate_limit_burst: 200
```

## Deployment

### Prerequisites

1. **DeepLake API Service**: Ensure the DeepLake API service is running and accessible
2. **API Key**: Obtain a valid API key from the DeepLake service
3. **Network Access**: Ensure network connectivity between AudiModal and DeepLake API
4. **TLS Certificates** (for production): Set up proper TLS certificates for secure communication

### Development Setup

1. Start the DeepLake API service:
   ```bash
   cd ../deeplake-api
   python -m uvicorn app.main:app --host 0.0.0.0 --port 8000
   ```

2. Configure AudiModal:
   ```bash
   export DEEPLAKE_API_URL=http://localhost:8000
   export DEEPLAKE_API_KEY=dev-key-12345
   ```

3. Start AudiModal:
   ```bash
   go run cmd/server/main.go
   ```

### Production Setup

1. **Use HTTPS**: Always use HTTPS for the DeepLake API in production
   ```bash
   export DEEPLAKE_API_URL=https://deeplake-api.your-domain.com
   ```

2. **Secure API Key**: Use a strong API key and store it securely
   ```bash
   export DEEPLAKE_API_KEY=$(cat /path/to/secure/api-key)
   ```

3. **Enable TLS**: Configure client certificates if required
   ```bash
   export DEEPLAKE_API_TLS_CERT_FILE=/path/to/client.crt
   export DEEPLAKE_API_TLS_KEY_FILE=/path/to/client.key
   ```

4. **Health Checks**: Enable health checks for monitoring
   ```bash
   export DEEPLAKE_API_HEALTH_CHECK_ENABLED=true
   ```

## Testing

### Integration Test

Run the integration test to verify the API connection:

```bash
cd examples
go run deeplake_api_test.go
```

This test will:
1. Connect to the DeepLake API
2. List existing datasets
3. Create a test dataset
4. Insert test vectors
5. Perform similarity searches
6. Test text-based search
7. Update and delete vectors
8. Retrieve dataset statistics

### HTTP API Testing

Test the HTTP endpoints using curl:

```bash
# List datasets
curl -X GET http://localhost:8080/api/v1/embeddings/datasets

# Create dataset
curl -X POST http://localhost:8080/api/v1/embeddings/datasets \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test_dataset",
    "description": "Test dataset",
    "dimensions": 1536,
    "metric_type": "cosine"
  }'

# Get dataset info
curl -X GET http://localhost:8080/api/v1/embeddings/datasets/test_dataset

# Perform search
curl -X POST http://localhost:8080/api/v1/embeddings/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "machine learning concepts",
    "dataset": "test_dataset",
    "options": {
      "top_k": 5,
      "include_content": true
    }
  }'
```

## Migration Steps

### For Existing Applications

1. **Update Configuration**: Add DeepLake API configuration to your config files
2. **Set Environment Variables**: Configure the API connection settings
3. **Start DeepLake API**: Ensure the DeepLake API service is running
4. **Test Connection**: Run the integration test to verify connectivity
5. **Update Deployment**: Deploy the updated AudiModal service
6. **Monitor**: Check logs and metrics to ensure everything is working correctly

### Data Migration

If you have existing data in the mock implementation, you'll need to:

1. **Export Data**: Use the existing API to export your data
2. **Format Data**: Convert to DeepLake API format
3. **Import Data**: Use the new import endpoints to load data into DeepLake
4. **Verify**: Test searches and operations to ensure data integrity

## Troubleshooting

### Common Issues

1. **Connection Refused**
   - Ensure DeepLake API service is running
   - Check the base URL configuration
   - Verify network connectivity

2. **Authentication Failed**
   - Check API key configuration
   - Ensure API key is valid and not expired
   - Verify tenant ID if using multi-tenancy

3. **TLS/SSL Errors**
   - Check certificate configuration
   - Verify certificate validity
   - Consider setting `tls_insecure_skip_verify: true` for testing (not production)

4. **Timeout Errors**
   - Increase timeout settings
   - Check network latency
   - Monitor DeepLake API service performance

5. **Rate Limiting**
   - Check rate limit configuration
   - Monitor API usage
   - Consider increasing limits or implementing backoff

### Debug Mode

Enable debug logging to troubleshoot issues:

```bash
export LOG_LEVEL=debug
go run cmd/server/main.go
```

### Health Checks

Monitor the health check endpoints:

```bash
# AudiModal health
curl http://localhost:8080/health

# DeepLake API health
curl http://localhost:8000/api/v1/health
```

## Performance Considerations

### Optimization Tips

1. **Connection Pooling**: Configure appropriate connection pool sizes
2. **Caching**: Enable caching for frequently accessed data
3. **Batch Operations**: Use batch endpoints for bulk operations
4. **Indexing**: Configure appropriate indexes for your use case
5. **Monitoring**: Set up monitoring and alerting for performance metrics

### Scaling

1. **Horizontal Scaling**: DeepLake API supports horizontal scaling
2. **Load Balancing**: Use load balancers for high availability
3. **Caching Layers**: Implement additional caching layers if needed
4. **Regional Deployment**: Deploy in multiple regions for global access

## Support

For issues related to:
- **AudiModal Integration**: Check GitHub issues or create a new issue
- **DeepLake API**: Refer to the DeepLake API documentation
- **Configuration**: Review this guide and the example configuration files
- **Performance**: Monitor metrics and adjust configuration as needed

## Changelog

### v1.0.0 - DeepLake API Integration
- ✅ Added DeepLake API client implementation
- ✅ Updated handlers to use real API instead of mock
- ✅ Added comprehensive configuration options
- ✅ Implemented new API endpoints
- ✅ Added integration tests
- ✅ Updated documentation

### Upcoming Features
- 🔄 Enhanced caching with Redis support
- 🔄 Improved error handling and retry logic
- 🔄 Metrics and monitoring integration
- 🔄 Advanced search features (hybrid, multi-dataset)
- 🔄 Data import/export functionality