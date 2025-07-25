# Authentication

AudiModal API uses API key-based authentication with JWT tokens for session management.

## API Key Authentication

### Getting an API Key

1. **Contact Sales**: Reach out to sales@audimodal.ai
2. **Account Setup**: Complete tenant onboarding
3. **Key Generation**: Receive your API key via secure channel

### Using API Keys

Include your API key in the `Authorization` header:

```bash
curl -H "Authorization: Bearer sk_live_1234567890abcdef" \
     https://api.audimodal.ai/v1/tenants
```

### Key Formats

- **Live Keys**: `sk_live_` prefix for production
- **Test Keys**: `sk_test_` prefix for development
- **Tenant Keys**: `tk_` prefix for tenant-specific access

## JWT Token Authentication

For interactive applications, use JWT tokens:

### Login Endpoint

```http
POST /v1/auth/login
Content-Type: application/json

{
  "email": "user@company.com",
  "password": "secure_password",
  "tenant_id": "tenant_123"
}
```

### Response

```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "user": {
    "id": "user_123",
    "email": "user@company.com",
    "role": "admin",
    "tenant_id": "tenant_123"
  }
}
```

### Using JWT Tokens

```bash
curl -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
     https://api.audimodal.ai/v1/files
```

## Refresh Tokens

When access tokens expire, use refresh tokens:

### Refresh Endpoint

```http
POST /v1/auth/refresh
Content-Type: application/json

{
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

## Multi-Tenant Access

### Tenant-Scoped Keys

API keys are automatically scoped to your tenant:

```bash
# This key only accesses tenant_123 resources
curl -H "Authorization: Bearer tk_tenant_123_abc123" \
     https://api.audimodal.ai/v1/files
```

### Cross-Tenant Access (Admin Only)

Admin users can access multiple tenants:

```bash
curl -H "Authorization: Bearer sk_live_admin_key" \
     -H "X-Tenant-ID: tenant_456" \
     https://api.audimodal.ai/v1/files
```

## Rate Limiting

Rate limits are enforced per API key:

### Rate Limit Headers

```http
HTTP/1.1 200 OK
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 999
X-RateLimit-Reset: 1640995200
```

### Rate Limit Exceeded

```http
HTTP/1.1 429 Too Many Requests
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1640995200

{
  "error": {
    "code": "RATE_LIMIT_EXCEEDED",
    "message": "API rate limit exceeded",
    "retry_after": 60
  }
}
```

## Security Best Practices

### API Key Security

- **Environment Variables**: Store keys in environment variables
- **Key Rotation**: Rotate keys regularly
- **Scope Limitation**: Use tenant-specific keys when possible
- **Secure Storage**: Never commit keys to version control

```bash
# Good: Environment variable
export AUDIMODAL_API_KEY="sk_live_1234567890abcdef"

# Bad: Hardcoded in source
const apiKey = "sk_live_1234567890abcdef"
```

### HTTPS Only

All API requests must use HTTPS:

```bash
# ✅ Secure
curl https://api.audimodal.ai/v1/health

# ❌ Insecure (will be rejected)
curl http://api.audimodal.ai/v1/health
```

### IP Allowlisting

Configure IP allowlists for production keys:

```http
POST /v1/auth/ip-allowlist
Authorization: Bearer sk_live_admin_key
Content-Type: application/json

{
  "allowed_ips": [
    "192.168.1.0/24",
    "10.0.0.100"
  ]
}
```

## Error Responses

### Invalid API Key

```http
HTTP/1.1 401 Unauthorized

{
  "error": {
    "code": "INVALID_API_KEY",
    "message": "Invalid or expired API key"
  }
}
```

### Insufficient Permissions

```http
HTTP/1.1 403 Forbidden

{
  "error": {
    "code": "INSUFFICIENT_PERMISSIONS",
    "message": "Insufficient permissions for this operation",
    "required_role": "admin"
  }
}
```

### Expired Token

```http
HTTP/1.1 401 Unauthorized

{
  "error": {
    "code": "TOKEN_EXPIRED",
    "message": "Access token has expired",
    "expires_at": "2024-01-01T12:00:00Z"
  }
}
```

## Testing Authentication

### Health Check

Test your authentication with the health endpoint:

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" \
     https://api.audimodal.ai/v1/health
```

### Expected Response

```json
{
  "status": "healthy",
  "service": "audimodal-api",
  "version": "1.0.0",
  "tenant_id": "tenant_123",
  "timestamp": "2024-01-01T12:00:00Z"
}
```

## SDK Authentication

### Python

```python
from audimodal import AudiModalClient

client = AudiModalClient(
    api_key="sk_live_1234567890abcdef",
    base_url="https://api.audimodal.ai/v1"
)

# Test authentication
health = client.health()
print(f"Authenticated as tenant: {health.tenant_id}")
```

### JavaScript

```javascript
import { AudiModalClient } from '@audimodal/sdk';

const client = new AudiModalClient({
  apiKey: 'sk_live_1234567890abcdef',
  baseUrl: 'https://api.audimodal.ai/v1'
});

// Test authentication
const health = await client.health();
console.log(`Authenticated as tenant: ${health.tenant_id}`);
```

### Go

```go
package main

import (
    "github.com/audimodal/go-sdk"
)

func main() {
    client := audimodal.NewClient("sk_live_1234567890abcdef")
    
    health, err := client.Health()
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Authenticated as tenant: %s\n", health.TenantID)
}
```