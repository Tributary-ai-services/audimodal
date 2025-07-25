# AudiModal API Documentation

## Overview

AudiModal provides a comprehensive REST API for enterprise document processing, AI-powered analysis, and compliance automation. This documentation covers all available endpoints, authentication, and integration patterns.

## Base URL

```
Production: https://api.audimodal.ai/v1
Staging: https://staging-api.audimodal.ai/v1
```

## Authentication

All API requests require authentication using API keys:

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" https://api.audimodal.ai/v1/health
```

## Quick Start

1. **Get API Key**: Contact sales@audimodal.ai for API access
2. **Create Tenant**: Set up your organization
3. **Add Data Source**: Connect your document repositories
4. **Process Documents**: Start AI-powered document analysis
5. **Query Results**: Search and analyze processed content

## API Reference

- [Authentication](./authentication.md)
- [Tenants](./tenants.md) - Multi-tenant organization management
- [Data Sources](./datasources.md) - Document repository connections
- [Files](./files.md) - Document management and processing
- [Processing Sessions](./sessions.md) - Workflow orchestration
- [Embeddings](./embeddings.md) - Vector operations and semantic search
- [DLP](./dlp.md) - Data Loss Prevention and compliance
- [Analytics](./analytics.md) - ML analysis and insights
- [Storage](./storage.md) - Multi-cloud storage operations

## SDKs and Libraries

- [Python SDK](./sdks/python.md)
- [JavaScript SDK](./sdks/javascript.md)
- [Go SDK](./sdks/go.md)
- [cURL Examples](./examples/curl.md)

## Rate Limits

- **Starter**: 1,000 requests/hour
- **Professional**: 10,000 requests/hour
- **Enterprise**: Custom limits

## Error Handling

All endpoints return standard HTTP status codes with detailed error messages:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid file format",
    "details": {
      "field": "file_type",
      "supported_formats": ["pdf", "docx", "txt"]
    }
  }
}
```

## Status Codes

- `200` - Success
- `201` - Created
- `400` - Bad Request
- `401` - Unauthorized
- `403` - Forbidden
- `404` - Not Found
- `429` - Rate Limited
- `500` - Internal Server Error

## Webhook Events

AudiModal can send webhooks for important events:

- `document.processed` - Document processing completed
- `session.completed` - Processing session finished
- `dlp.violation` - DLP policy violation detected
- `compliance.alert` - Compliance issue identified

## Support

- **Documentation**: [docs.audimodal.ai](https://docs.audimodal.ai)
- **API Support**: [api-support@audimodal.ai](mailto:api-support@audimodal.ai)
- **Status Page**: [status.audimodal.ai](https://status.audimodal.ai)