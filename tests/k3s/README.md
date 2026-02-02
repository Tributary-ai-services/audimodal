# K3s Integration Tests

This directory contains end-to-end integration tests that run against the AudiModal service deployed in K3s (Kubernetes).

## Overview

These tests upload documents to the K3s-deployed AudiModal service, trigger processing, and verify that compliance violations are correctly detected.

## Test Flow

1. **Upload** - Upload test document (txt or pdf) via `POST /api/v1/tenants/{tenant_id}/files`
2. **Process** - Trigger processing via `POST /api/v1/tenants/{tenant_id}/files/{file_id}/process`
3. **Wait** - Poll status via `GET /api/v1/tenants/{tenant_id}/files/{file_id}` until completed
4. **Verify** - Retrieve violations via `GET /api/v1/tenants/{tenant_id}/files/{file_id}/violations`
5. **Assert** - Verify expected compliance violations match actual

## Prerequisites

1. **AudiModal Service**: Running and accessible via Ingress URL
2. **API Key**: Valid test API key with appropriate permissions
3. **Test Data**: Test data files in `tas-test-data/` directory

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `AUDIMODAL_API_KEY` | **Yes** | - | API key for authentication |
| `AUDIMODAL_URL` | No | `https://audimodal.tas.scharber.com` | AudiModal API base URL |
| `AUDIMODAL_TENANT_ID` | No | `test-tenant` | Tenant ID for tests |
| `AUDIMODAL_TIMEOUT` | No | `30s` | HTTP client timeout |
| `AUDIMODAL_POLL_INTERVAL` | No | `2s` | Polling interval for status checks |
| `AUDIMODAL_MAX_POLL_ATTEMPTS` | No | `30` | Maximum poll attempts |
| `TEST_DATA_PATH` | No | `../../../tas-test-data` | Path to test data directory |

### Setting Up API Keys

API keys are stored in the `aether-secrets` repository:

```bash
# Create test API key file (gitignored)
cat > ../aether-secrets/apps/audimodal/api-test.env << 'EOF'
AUDIMODAL_API_KEY=your-actual-test-api-key
AUDIMODAL_URL=https://audimodal.tas.scharber.com
AUDIMODAL_TENANT_ID=test-tenant
EOF
```

## Running Tests

### Option 1: Source Secrets Manually

```bash
# Source the test API key
source /path/to/aether-secrets/apps/audimodal/api-test.env

# Run tests
cd audimodal
make test-k3s
```

### Option 2: Use Convenience Makefile Target

```bash
cd audimodal
make test-k3s-with-secrets
```

### Option 3: Export Variables Directly

```bash
export AUDIMODAL_API_KEY=your-test-api-key
export AUDIMODAL_URL=https://audimodal.tas.scharber.com

cd audimodal
go test -v -timeout 10m ./tests/k3s/...
```

## Available Test Commands

```bash
# Full integration test suite
make test-k3s

# With automatic secret loading (from aether-secrets)
make test-k3s-with-secrets

# Quick health check only
make test-k3s-short

# GDPR-specific tests
make test-k3s-gdpr

# All regulation tests
make test-k3s-all-regulations
```

## Test Cases

### Health Check
- `TestHealthCheck` - Verifies the service is reachable

### GDPR Tests
- `TestGDPR_PersonalData` - Tests GDPR-001 (email, phone detection)
- `TestGDPR_SpecialCategory` - Tests GDPR-002 (SSN detection)

### HIPAA Tests
- `TestHIPAA_PHI` - Tests HIPAA-001 (PHI detection)

### PCI-DSS Tests
- `TestPCI_CardholderData` - Tests PCI-001 (credit card detection)

### CCPA Tests
- `TestCCPA_PersonalInfo` - Tests CCPA-001 (personal info + IP address detection)

### Multi-Regulation Tests
- `TestMultiRegulation` - Tests documents triggering multiple regulations
- `TestMedicalRecords` - Tests comprehensive medical record scanning
- `TestFinancialReport` - Tests financial report scanning

### PII Type Tests
- `TestSSNDetection` - Tests SSN pattern detection
- `TestCreditCardDetection` - Tests credit card pattern detection
- `TestEmailDetection` - Tests email pattern detection

### Clean Document Tests
- `TestCleanDocument` - Verifies no false positives on clean documents
- `TestFalsePositiveRejection` - Verifies false positives are rejected

## Test Data

Tests use data from the `tas-test-data` repository:

```
tas-test-data/
├── txt/
│   ├── compliance/
│   │   ├── gdpr/
│   │   │   ├── gdpr_personal_data.txt
│   │   │   └── gdpr_special_category.txt
│   │   ├── hipaa/
│   │   │   └── hipaa_phi.txt
│   │   ├── pci-dss/
│   │   │   └── pci_cardholder_data.txt
│   │   └── ccpa/
│   │       └── ccpa_personal_info.txt
│   ├── pii-types/
│   │   ├── ssn_samples.txt
│   │   ├── credit_card_samples.txt
│   │   └── email_samples.txt
│   ├── multi-pii/
│   │   ├── employee_records.txt
│   │   ├── medical_records.txt
│   │   └── financial_report.txt
│   ├── edge-cases/
│   │   └── false_positive_check.txt
│   └── negative/
│       └── clean_document.txt
└── pdf/
    └── [mirrors txt/ structure]
```

## Troubleshooting

### "AUDIMODAL_API_KEY not set"
Ensure you've exported the API key:
```bash
export AUDIMODAL_API_KEY=your-api-key
```

### "Connection refused" / "No such host"
- Verify the AudiModal service is running: `kubectl get pods -n eaiingest`
- Check the Ingress URL is correct
- Verify DNS resolution: `nslookup audimodal.tas.scharber.com`

### "Unauthorized"
- Verify your API key is valid
- Check the API key has appropriate permissions for the tenant

### "Timeout"
- Increase `AUDIMODAL_TIMEOUT` and `AUDIMODAL_MAX_POLL_ATTEMPTS`
- Check service logs: `kubectl logs -n eaiingest -l app=audimodal`

### "File not found"
- Verify `TEST_DATA_PATH` points to the correct `tas-test-data` directory
- Check the test data files exist

## Adding New Tests

1. Add test data file to `tas-test-data/txt/` (and convert to PDF)
2. Add test function in `k3s_test.go` following the existing pattern
3. Use `s.uploadTestFile()` to upload and track files for cleanup
4. Use `s.processAndWait()` to trigger processing
5. Assert expected violations using helper functions from `helpers.go`

Example:
```go
func (s *K3sComplianceSuite) TestNewRegulation() {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()

    fileResp, _ := s.uploadTestFile(ctx, "txt/compliance/new/test_file.txt")
    s.processAndWait(ctx, fileResp.FileID)

    violations, err := s.client.GetViolations(ctx, s.config.TenantID, fileResp.FileID)
    s.Require().NoError(err)

    // Assert expected violations
    s.Assert().True(ContainsRule(violations.Violations, "NEW-001"))
}
```

## CI/CD Integration

For CI/CD pipelines, use Kubernetes Secrets:

```yaml
# In your CI workflow
- name: Run K3s Integration Tests
  env:
    AUDIMODAL_API_KEY: ${{ secrets.AUDIMODAL_TEST_API_KEY }}
    AUDIMODAL_URL: https://audimodal.tas.scharber.com
  run: |
    cd audimodal
    make test-k3s
```
