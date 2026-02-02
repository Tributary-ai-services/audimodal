# Feature Matrix: Databunker Integration for PII/HIPAA Tokenization

## Overview

This document outlines the feature requirements and evaluation for integrating Databunker as the tokenization backend for our PII/HIPAA compliance scanner. The scanner currently supports redaction; this integration adds reversible tokenization as an alternative output mode.

---

## Requirements Matrix

| Requirement | Priority | Databunker OSS | Databunker Pro | Notes |
|-------------|----------|----------------|----------------|-------|
| **Core Tokenization** |
| Create token from arbitrary value | P0 | ✅ | ✅ | REST API: `POST /v1/sharedrecord/{type}` |
| Retrieve original value from token | P0 | ✅ | ✅ | REST API: `GET /v1/sharedrecord/{type}/{token}` |
| Delete token | P1 | ✅ | ✅ | REST API: `DELETE /v1/sharedrecord/{type}/{token}` |
| Bulk tokenization (batch API) | P2 | ❌ | ✅ | Pro feature: secure bulk requests |
| **Token Characteristics** |
| UUID-based tokens | P0 | ✅ | ✅ | Returns UUID tokens by default |
| Format-preserving tokens | P3 | ❌ | ✅ | Pro feature |
| Deterministic/convergent tokens | P2 | ❌ | ❌ | Same input → same token (not supported) |
| Token expiration/TTL | P3 | ❌ | ✅ | Pro feature: record expiration |
| **Security** |
| Encryption at rest (AES-256) | P0 | ✅ | ✅ | All records encrypted |
| API authentication | P0 | ✅ | ✅ | X-Bunker-Token header |
| Audit logging | P1 | ✅ | ✅ | All access logged |
| SQL injection protection | P1 | ✅ | ✅ | By design (no direct DB queries) |
| Role-based access control | P2 | ⚠️ Basic | ✅ | Pro has advanced access control |
| Encryption key rotation | P2 | ❌ | ✅ | Pro feature |
| **Deployment** |
| Docker container | P0 | ✅ | ✅ | `securitybunker/databunker` |
| Kubernetes Helm chart | P1 | ✅ | ✅ | Available |
| High availability | P2 | ❌ | ✅ | Pro feature: database sharding |
| **Data Storage** |
| SQLite backend | P0 | ✅ | ✅ | Default, good for dev/test |
| PostgreSQL backend | P1 | ✅ | ✅ | Recommended for production |
| MySQL backend | P1 | ✅ | ✅ | Supported |
| **Compliance** |
| HIPAA compatible | P0 | ✅ | ✅ | Encryption + audit logging |
| GDPR features | P1 | ✅ | ✅ | Subject access/deletion workflows |
| PCI DSS compatible | P2 | ⚠️ | ✅ | Pro recommended for credit cards |
| SOC 2 compatible | P2 | Self-attest | ✅ | Pro has certification support |
| **Integration** |
| REST API | P0 | ✅ | ✅ | Full REST interface |
| Go SDK | P1 | ❌ | ❌ | Not available (HTTP client required) |
| Node.js SDK | P3 | ✅ | ✅ | Official SDK available |
| OpenAPI/Swagger spec | P1 | ✅ | ✅ | Available for client generation |

---

## API Integration Design

### Tokenization Flow

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  PII Scanner    │     │   Databunker    │     │   Database      │
│  (Go Service)   │     │   (Container)   │     │   (PostgreSQL)  │
└────────┬────────┘     └────────┬────────┘     └────────┬────────┘
         │                       │                       │
         │  POST /v1/sharedrecord/ssn                   │
         │  {"data":"123-45-6789"}                      │
         │──────────────────────►│                       │
         │                       │  INSERT encrypted     │
         │                       │──────────────────────►│
         │                       │                       │
         │  {"record":"rec_abc123"}                     │
         │◄──────────────────────│                       │
         │                       │                       │
```

### Detokenization Flow

```
         │  GET /v1/sharedrecord/ssn/rec_abc123        │
         │──────────────────────►│                       │
         │                       │  SELECT + decrypt     │
         │                       │──────────────────────►│
         │                       │                       │
         │  {"data":"123-45-6789"}                      │
         │◄──────────────────────│                       │
```

### Proposed Go Client Interface

```go
type TokenClient interface {
    // Tokenize stores a value and returns a token
    Tokenize(ctx context.Context, dataType string, value string) (token string, err error)
    
    // Detokenize retrieves the original value for a token
    Detokenize(ctx context.Context, dataType string, token string) (value string, err error)
    
    // Delete removes a token and its associated value
    Delete(ctx context.Context, dataType string, token string) error
}
```

### Data Types to Support

| Scanner Detection Type | Databunker Record Type | Example |
|------------------------|------------------------|---------|
| SSN | `ssn` | 123-45-6789 |
| Credit Card | `credit_card` | 4111111111111111 |
| Phone Number | `phone` | +1-555-123-4567 |
| Email Address | `email` | user@example.com |
| Medical Record Number | `mrn` | MRN-12345678 |
| Date of Birth | `dob` | 1990-01-15 |
| Driver's License | `drivers_license` | D1234567 |
| Passport Number | `passport` | AB1234567 |
| Bank Account | `bank_account` | 123456789012 |
| IP Address | `ip_address` | 192.168.1.1 |
| Generic PII | `pii` | (catchall) |

---

## Deployment Configuration

### Docker Compose (Development)

```yaml
version: '3.8'
services:
  databunker:
    image: securitybunker/databunker:latest
    ports:
      - "3000:3000"
    environment:
      - DATABUNKER_MASTERKEY=${DATABUNKER_MASTERKEY}
    volumes:
      - databunker_data:/databunker/data
    restart: unless-stopped

volumes:
  databunker_data:
```

### Kubernetes Helm Values (Production)

```yaml
# values.yaml
replicaCount: 1

image:
  repository: securitybunker/databunker
  tag: latest
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 3000

persistence:
  enabled: true
  storageClass: "standard"
  size: 10Gi

database:
  type: postgresql
  host: postgres-service
  port: 5432
  name: databunker
  existingSecret: databunker-db-credentials

resources:
  requests:
    memory: "256Mi"
    cpu: "100m"
  limits:
    memory: "512Mi"
    cpu: "500m"
```

---

## Implementation Tasks

- [ ] **Phase 1: Core Integration**
  - [ ] Create Go HTTP client wrapper for Databunker API
  - [ ] Add `--tokenize` flag to scanner CLI
  - [ ] Implement tokenize/detokenize for top 5 PII types (SSN, CC, phone, email, DOB)
  - [ ] Add Databunker connection configuration (URL, API token)
  - [ ] Unit tests with mock Databunker responses

- [ ] **Phase 2: Deployment**
  - [ ] Docker Compose config for local development
  - [ ] Kubernetes Helm chart integration
  - [ ] Health check endpoint monitoring
  - [ ] Documentation for Databunker setup

- [ ] **Phase 3: Production Readiness**
  - [ ] Connection pooling / retry logic
  - [ ] Circuit breaker for Databunker unavailability
  - [ ] Metrics (tokenization latency, error rates)
  - [ ] Audit log integration

- [ ] **Phase 4: Extended Features** (Optional)
  - [ ] Batch tokenization API
  - [ ] Token lookup by original value (if needed)
  - [ ] Evaluate Databunker Pro for format-preserving tokens

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Databunker service unavailable | Medium | High | Circuit breaker, fallback to redaction |
| Token storage exhaustion | Low | Medium | Monitoring, retention policies |
| API token compromise | Low | High | Secret management, rotation policy |
| Performance bottleneck | Medium | Medium | Connection pooling, async tokenization |
| Project abandonment | Low | High | OSS with MIT license, can fork; Pro tier indicates commercial viability |

---

## References

### Databunker Documentation

| Resource | URL |
|----------|-----|
| GitHub Repository | https://github.com/securitybunker/databunker |
| Official Website | https://databunker.org/ |
| Project Introduction | https://databunker.org/doc/introduction/ |
| API Documentation | https://databunker.org/doc/api/ |
| Databunker Pro Features | https://databunker.org/databunker-pro/ |
| Docker Hub | https://hub.docker.com/r/securitybunker/databunker |
| Getting Started Tutorial | https://marcusolsson.dev/data-privacy-vaults-using-databunker/ |

### Compliance & Security References

| Resource | URL |
|----------|-----|
| HIPAA Compliance Guide | https://databunker.org/use-case/hipaa-compliance/ |
| GDPR Compliance Guide | https://databunker.org/use-case/gdpr-compliance/ |
| ISO 27001 Compliance | https://databunker.org/use-case/iso27001-compliance/ |
| Security Architecture | https://databunker.org/doc/security/ |

### Alternative Solutions Evaluated

| Solution | URL | Reason Not Selected |
|----------|-----|---------------------|
| Open Privacy Vault (OPV) | https://github.com/open-privacy/opv | Limited maintenance activity since 2022 |
| Microsoft Presidio | https://github.com/microsoft/presidio | Detection-focused, not a token vault |
| HashiCorp Vault Transform | https://developer.hashicorp.com/vault/docs/secrets/transform | Requires Enterprise license ($$$) |
| Basis Theory | https://basistheory.com/ | SaaS-only, $0.10/token/month |
| ln80/pii | https://github.com/ln80/pii | Struct encryption, not token service |

### Technical References

| Resource | URL |
|----------|-----|
| Tokenization vs Encryption (NIST) | https://csrc.nist.gov/publications/detail/sp/800-188/final |
| PCI DSS Tokenization Guidelines | https://www.pcisecuritystandards.org/documents/Tokenization_Guidelines_Info_Supplement.pdf |
| HIPAA Security Rule | https://www.hhs.gov/hipaa/for-professionals/security/index.html |

### Community & Support

| Resource | URL |
|----------|-----|
| Databunker Discussions | https://github.com/securitybunker/databunker/discussions |
| HackerNoon Article | https://hackernoon.com/data-leak-prevention-with-databunker-xnn33u9 |
| Crunchbase Profile | https://www.crunchbase.com/organization/databunker-pro |
| Slashdot Reviews | https://slashdot.org/software/p/Databunker/ |

---

## UI Requirements

### Features to Surface in UI

| Feature | UI Element | Priority | Why Expose |
|---------|------------|----------|------------|
| **Tokenize vs Redact toggle** | Radio/switch per scan or global setting | P0 | Core decision users need to make |
| **Data types to tokenize** | Checklist (SSN, CC, email, etc.) | P0 | Users may want to tokenize SSN but redact emails |
| **Token output format** | Results display showing `[TOKEN:rec_abc123]` | P0 | Users need to see what replaced the original |
| **Detokenize action** | Button/API on scan results | P0 | Users need to retrieve original values |
| **Token lookup** | Search by token ID | P1 | "What was this token again?" |
| **Token deletion** | Delete button per token or bulk | P1 | GDPR "right to be forgotten" workflows |
| **Audit log viewer** | Table of who accessed what tokens, when | P1 | Compliance officers need this |
| **Connection status** | Health indicator (green/red) | P2 | "Is tokenization available right now?" |

### Features to Keep Hidden (Backend Only)

| Feature | Why Hide |
|---------|----------|
| Encryption algorithm (AES-256) | Implementation detail, no user decision |
| Database backend type | Ops concern, not user concern |
| API authentication tokens | Security - users shouldn't see/manage these |
| Container/Helm config | DevOps territory |
| Key rotation | Automated or admin-only |
| Connection pooling/retry logic | Invisible reliability |
| Bulk API batching | Optimization detail |

### UI Mockup (Minimal)

```
┌─────────────────────────────────────────────────────────┐
│ Scan Settings                                           │
├─────────────────────────────────────────────────────────┤
│ When PII is detected:                                   │
│   ○ Redact (replace with ****)                          │
│   ● Tokenize (reversible)                    [Connected]│
│                                                         │
│ Tokenize these types:                                   │
│   ☑ SSN        ☑ Credit Card    ☑ Phone               │
│   ☑ Email      ☑ DOB            ☐ IP Address          │
│   ☑ Medical Record Number                              │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│ Scan Results                                            │
├─────────────────────────────────────────────────────────┤
│ Found 3 PII items:                                      │
│                                                         │
│ Line 24: SSN                                            │
│   Original: 123-45-6789 → Token: rec_7hG4kL...         │
│   [Reveal] [Delete Token]                               │
│                                                         │
│ Line 45: Credit Card                                    │
│   Original: 4111****1111 → Token: rec_9xM2pQ...        │
│   [Reveal] [Delete Token]                               │
│                                                         │
│ Line 89: Email                                          │
│   Original: j***@example.com → Token: rec_3nK8wR...    │
│   [Reveal] [Delete Token]                               │
│                                                         │
│ [Export Tokenized] [Reveal All] [Delete All Tokens]    │
└─────────────────────────────────────────────────────────┘
```

### Permission Model

| Action | Access Level | Notes |
|--------|--------------|-------|
| Toggle tokenize/redact | All users | Basic scan configuration |
| Select data types | All users | Or admin-configurable defaults |
| View tokens | Scan owner | Users who ran the scan |
| Reveal (detokenize) | Restricted | Auditor, admin, compliance role |
| Delete tokens | Restricted | Compliance officer, admin |
| View audit logs | Restricted | Compliance officer, admin |
| Configure connection | Admin only | Backend settings |

### UI Implementation Tasks

- [ ] Add tokenization toggle to scan settings page
- [ ] Create PII type selector component (checkboxes)
- [ ] Display token IDs in scan results alongside detection type
- [ ] Implement "Reveal" button with permission check
- [ ] Implement "Delete Token" with confirmation dialog
- [ ] Add bulk actions (Reveal All, Delete All)
- [ ] Create audit log viewer page
- [ ] Add Databunker connection status indicator
- [ ] Implement token search/lookup interface

---

## Decision Record

**Decision**: Adopt Databunker OSS as the tokenization backend for the PII/HIPAA scanner.

**Rationale**:
1. Meets all P0 requirements (tokenize, detokenize, encryption, API auth)
2. Free and open source (MIT license)
3. Active maintenance and commercial backing (Pro tier)
4. Simple deployment (single container)
5. Go-based service aligns with our stack
6. Adequate documentation and community support

**Trade-offs Accepted**:
- No native Go SDK (HTTP client wrapper required)
- No format-preserving tokenization without Pro
- No deterministic/convergent token mode
- Self-attestation required for compliance certifications

**Future Considerations**:
- Evaluate Databunker Pro if batch processing or format-preserving tokens become requirements
- Monitor project health; maintain ability to migrate if needed
