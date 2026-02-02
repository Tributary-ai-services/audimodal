# Compliance Testing Strategy

*AudiModal DLP Compliance Testing Framework*

---

## Table of Contents

1. [Overview](#overview)
2. [Test Categories](#test-categories)
3. [Test Data Requirements](#test-data-requirements)
4. [Test File Structure](#test-file-structure)
5. [Coverage Targets](#coverage-targets)
6. [Performance Benchmarks](#performance-benchmarks)
7. [Running Tests](#running-tests)
8. [CI/CD Integration](#cicd-integration)

---

## Overview

This document defines the testing strategy for AudiModal's DLP compliance scanning functionality. The goal is to ensure comprehensive coverage of all implemented compliance standards (GDPR, HIPAA, PCI-DSS, CCPA) and pattern matchers (SSN, Credit Card, Email, Phone, IP Address).

### Testing Principles

1. **Both Formats**: Test data exists as `.txt` and `.pdf` to verify both processing pipelines
2. **Known Values**: All test data contains known PII for deterministic validation
3. **Positive & Negative**: Each standard has both detection and false-positive tests
4. **Performance Aware**: Benchmark tests ensure acceptable processing speeds
5. **Reproducible**: Tests can run in any environment with consistent results

---

## Test Categories

### 1. Unit Tests

**Pattern Matcher Unit Tests** (`pkg/dlp/patterns/*_test.go`)

| Test Type | Purpose | Files |
|-----------|---------|-------|
| Valid Format Tests | Verify correct PII detection | `ssn_test.go`, `creditcard_test.go`, etc. |
| Invalid Format Tests | Verify rejection of malformed data | Same files |
| Confidence Score Tests | Verify confidence calculation | Same files |
| Edge Case Tests | Boundary conditions | Same files |

**Compliance Checker Unit Tests** (`pkg/dlp/compliance/*_test.go`)

| Test Type | Purpose | Files |
|-----------|---------|-------|
| GDPR Rules | Test GDPR-001, GDPR-002 | `gdpr_test.go` |
| HIPAA Rules | Test HIPAA-001 | `hipaa_test.go` |
| PCI-DSS Rules | Test PCI-001 | `pci_test.go` |
| CCPA Rules | Test CCPA-001 | `ccpa_test.go` |
| Multi-Regulation | Test overlapping rules | `checker_test.go` |

### 2. Integration Tests

**Scanner Integration Tests** (`tests/compliance/*_integration_test.go`)

| Test Type | Purpose | Files |
|-----------|---------|-------|
| TXT Processing | Full pipeline with text files | `pdf_compliance_test.go` |
| PDF Processing | Full pipeline with PDF files | `pdf_compliance_test.go` |
| Multi-Regulation | Documents triggering multiple standards | `multi_regulation_test.go` |

### 3. Performance Tests

**Benchmark Tests** (`pkg/dlp/*/*_bench_test.go`)

| Test Type | Purpose | Files |
|-----------|---------|-------|
| Matcher Benchmarks | Per-matcher performance | `matchers_bench_test.go` |
| Checker Benchmarks | Compliance checking speed | `checker_bench_test.go` |
| Scanner Benchmarks | Full scan performance | `scanner_bench_test.go` |
| Memory Profiling | Memory usage analysis | `tests/performance/` |

---

## Test Data Requirements

### Test Data Location

```
tas-test-data/
├── txt/                          # Source text files
│   ├── compliance/               # Compliance-specific tests
│   │   ├── gdpr/
│   │   ├── hipaa/
│   │   ├── pci-dss/
│   │   └── ccpa/
│   ├── pii-types/                # Single PII type tests
│   ├── multi-pii/                # Multi-PII combination tests
│   ├── edge-cases/               # Edge case tests
│   └── negative/                 # Clean documents
├── pdf/                          # Converted PDF versions
│   └── [mirrors txt/ structure]
├── performance/                  # Large files for benchmarks
└── scripts/
    └── convert_txt_to_pdf.py     # Conversion utility
```

### Test Data Files

#### Compliance-Specific Files

| File | Standard | PII Types | Purpose |
|------|----------|-----------|---------|
| `gdpr/gdpr_personal_data.txt` | GDPR | Email, Name, Address, Phone | Test GDPR-001 |
| `gdpr/gdpr_special_category.txt` | GDPR | SSN, DOB | Test GDPR-002 |
| `hipaa/hipaa_phi.txt` | HIPAA | SSN, DOB, Name, Email | Test HIPAA-001 |
| `pci-dss/pci_cardholder_data.txt` | PCI-DSS | Credit Card | Test PCI-001 |
| `ccpa/ccpa_personal_info.txt` | CCPA | Email, Name, Address, SSN, IP | Test CCPA-001 |

#### PII Type Files

| File | PII Type | Variations | Purpose |
|------|----------|------------|---------|
| `pii-types/ssn_samples.txt` | SSN | All valid formats + invalid | Test SSN matcher |
| `pii-types/credit_card_samples.txt` | Credit Card | Visa, MC, Amex, Discover + invalid | Test CC matcher |
| `pii-types/email_samples.txt` | Email | Valid formats + edge cases | Test email matcher |
| `pii-types/phone_samples.txt` | Phone | US formats + invalid | Test phone matcher |
| `pii-types/ip_address_samples.txt` | IP Address | Valid IPv4 + invalid | Test IP matcher |

#### Multi-PII Files

| File | PII Types | Purpose |
|------|-----------|---------|
| `multi-pii/employee_records.txt` | SSN, Name, Email, Phone, DOB | HR document simulation |
| `multi-pii/customer_database.txt` | Name, Email, Address, CC | Customer data simulation |
| `multi-pii/medical_records.txt` | SSN, DOB, Name, Address | Healthcare simulation |
| `multi-pii/financial_report.txt` | SSN, CC, Bank Account | Financial simulation |

#### Edge Case Files

| File | Purpose |
|------|---------|
| `edge-cases/partial_matches.txt` | Partial/incomplete PII patterns |
| `edge-cases/false_positive_check.txt` | Common false positive scenarios |
| `edge-cases/malformed_data.txt` | Malformed/corrupted data |

#### Negative Test Files

| File | Purpose |
|------|---------|
| `negative/clean_document.txt` | Document with no PII (should return 0 findings) |

#### Performance Test Files

| File | Size | Purpose |
|------|------|---------|
| `performance/small_1kb.txt` | ~1KB | Baseline benchmark |
| `performance/medium_100kb.txt` | ~100KB | Standard document |
| `performance/large_1mb.txt` | ~1MB | Large document |
| `performance/large_10mb.txt` | ~10MB | Stress test |
| `performance/high_density_pii.txt` | Variable | Many PII instances |

---

## Test File Structure

### Unit Test Structure

```go
// pkg/dlp/patterns/ssn_test.go
package patterns

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestSSNMatcher_ValidFormats(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected int  // number of matches
    }{
        {"dashed format", "SSN: 123-45-6789", 1},
        {"space format", "SSN: 123 45 6789", 1},
        {"no separator", "SSN: 123456789", 1},
        {"multiple SSNs", "SSN1: 123-45-6789, SSN2: 234-56-7890", 2},
    }

    matcher := NewSSNMatcher()
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            matches := matcher.Match(tt.input)
            assert.Equal(t, tt.expected, len(matches))
        })
    }
}

func TestSSNMatcher_InvalidFormats(t *testing.T) {
    tests := []struct {
        name  string
        input string
    }{
        {"starts with 000", "SSN: 000-12-3456"},
        {"starts with 666", "SSN: 666-12-3456"},
        {"starts with 9xx", "SSN: 912-34-5678"},
        {"middle 00", "SSN: 123-00-6789"},
        {"last 0000", "SSN: 123-45-0000"},
        {"too short", "SSN: 12-34-5678"},
        {"too long", "SSN: 1234-56-7890"},
    }

    matcher := NewSSNMatcher()
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            matches := matcher.Match(tt.input)
            assert.Equal(t, 0, len(matches), "Should not match invalid SSN")
        })
    }
}

func TestSSNMatcher_ConfidenceScore(t *testing.T) {
    matcher := NewSSNMatcher()

    // Formatted SSN should have higher confidence
    assert.Greater(t, matcher.GetConfidenceScore("123-45-6789"), 0.8)

    // Unformatted should have lower confidence
    assert.Less(t, matcher.GetConfidenceScore("123456789"), 0.8)
}
```

### Compliance Test Structure

```go
// pkg/dlp/compliance/gdpr_test.go
package compliance

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/jscharber/audimodal/pkg/dlp/types"
)

func TestGDPRChecker_PersonalData(t *testing.T) {
    checker := NewBasicComplianceChecker()

    // Create scan result with personal data findings
    scanResult := &types.ScanResult{
        Findings: []types.Finding{
            {ID: "1", Type: types.PIITypeEmail, RiskLevel: types.RiskLevelMedium},
            {ID: "2", Type: types.PIITypeName, RiskLevel: types.RiskLevelMedium},
        },
    }

    rules := []types.ComplianceRule{
        {Regulation: "GDPR", Rule: "GDPR-001"},
    }

    result, err := checker.CheckCompliance(context.Background(), scanResult, rules)

    assert.NoError(t, err)
    assert.False(t, result.IsCompliant)
    assert.GreaterOrEqual(t, len(result.Violations), 1)
}

func TestGDPRChecker_SpecialCategory(t *testing.T) {
    checker := NewBasicComplianceChecker()

    // Create scan result with special category data
    scanResult := &types.ScanResult{
        Findings: []types.Finding{
            {ID: "1", Type: types.PIITypeSSN, RiskLevel: types.RiskLevelCritical},
            {ID: "2", Type: types.PIITypeDateOfBirth, RiskLevel: types.RiskLevelHigh},
        },
    }

    rules := []types.ComplianceRule{
        {Regulation: "GDPR", Rule: "GDPR-002"},
    }

    result, err := checker.CheckCompliance(context.Background(), scanResult, rules)

    assert.NoError(t, err)
    assert.False(t, result.IsCompliant)
    assert.Equal(t, "critical", result.Violations[0].Severity)
}
```

### Integration Test Structure

```go
// tests/compliance/pdf_compliance_test.go
package compliance_test

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/jscharber/audimodal/pkg/dlp/scanner"
)

func getTestDataPath() string {
    if path := os.Getenv("TEST_DATA_PATH"); path != "" {
        return path
    }
    return "../../../tas-test-data"
}

func TestIntegration_PDFCompliance_GDPR(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }

    testDataPath := getTestDataPath()
    pdfPath := filepath.Join(testDataPath, "pdf", "compliance", "gdpr", "gdpr_personal_data.pdf")

    // Skip if test data not available
    if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
        t.Skipf("Test data not found at %s", pdfPath)
    }

    // Process PDF and check compliance
    s := scanner.NewDLPScanner(nil)
    content, err := os.ReadFile(pdfPath)
    assert.NoError(t, err)

    result, err := s.ScanContent(string(content))
    assert.NoError(t, err)

    // Should detect GDPR personal data
    assert.Greater(t, len(result.Findings), 0)
}

func TestIntegration_TXTCompliance_MultiRegulation(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }

    testDataPath := getTestDataPath()
    txtPath := filepath.Join(testDataPath, "txt", "multi-pii", "employee_records.txt")

    if _, err := os.Stat(txtPath); os.IsNotExist(err) {
        t.Skipf("Test data not found at %s", txtPath)
    }

    s := scanner.NewDLPScanner(nil)
    content, err := os.ReadFile(txtPath)
    assert.NoError(t, err)

    result, err := s.ScanContent(string(content))
    assert.NoError(t, err)

    // Should detect multiple PII types
    piiTypes := make(map[types.PIIType]bool)
    for _, finding := range result.Findings {
        piiTypes[finding.Type] = true
    }

    assert.True(t, piiTypes[types.PIITypeSSN], "Should detect SSN")
    assert.True(t, piiTypes[types.PIITypeEmail], "Should detect Email")
}
```

### Benchmark Test Structure

```go
// pkg/dlp/patterns/matchers_bench_test.go
package patterns

import (
    "strings"
    "testing"
)

func BenchmarkSSNMatcher_Match(b *testing.B) {
    matcher := NewSSNMatcher()
    content := "Employee SSN: 123-45-6789, Manager SSN: 234-56-7890"

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        matcher.Match(content)
    }
}

func BenchmarkSSNMatcher_Match_LargeDocument(b *testing.B) {
    matcher := NewSSNMatcher()
    // Create 100KB document with scattered SSNs
    content := generateLargeContent(100*1024, "SSN: 123-45-6789")

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        matcher.Match(content)
    }
}

func BenchmarkCreditCardMatcher_Match(b *testing.B) {
    matcher := NewCreditCardMatcher()
    content := "Visa: 4532-1234-5678-9123, MC: 5425-2334-3010-9903"

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        matcher.Match(content)
    }
}

func BenchmarkAllMatchers_Match(b *testing.B) {
    registry := NewPatternRegistry()
    content := `
        SSN: 123-45-6789
        Email: test@example.com
        Phone: (555) 123-4567
        CC: 4532-1234-5678-9123
        IP: 192.168.1.1
    `

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        for _, matcher := range registry.GetAllMatchers() {
            matcher.Match(content)
        }
    }
}

func generateLargeContent(size int, pii string) string {
    base := "Lorem ipsum dolor sit amet, consectetur adipiscing elit. "
    content := strings.Repeat(base, size/len(base))
    // Insert PII at random positions
    return content[:size/2] + " " + pii + " " + content[size/2:]
}
```

---

## Coverage Targets

### Component Coverage

| Component | Target | Measurement |
|-----------|--------|-------------|
| Pattern Matchers (`pkg/dlp/patterns/`) | 90% | Line coverage |
| Compliance Checker (`pkg/dlp/compliance/`) | 85% | Line coverage |
| Integration Tests | 80% | Integration scenarios |

### Test Case Coverage

| Category | Minimum Tests per Standard |
|----------|---------------------------|
| Positive Detection | 3-5 tests |
| Negative Detection | 3-5 tests |
| Edge Cases | 2-3 tests |
| Integration | 1-2 tests |

---

## Performance Benchmarks

### Performance Targets

| Metric | Target | Test Method |
|--------|--------|-------------|
| SSN detection speed | <1ms per match | `BenchmarkSSNMatcher_Match` |
| Credit card detection | <2ms per match (Luhn) | `BenchmarkCreditCardMatcher_Match` |
| Full scan (1KB doc) | <10ms | `BenchmarkScanner_SmallDocument` |
| Full scan (100KB doc) | <100ms | `BenchmarkScanner_MediumDocument` |
| Full scan (1MB doc) | <1s | `BenchmarkScanner_LargeDocument` |
| Memory per scan | <50MB for 1MB doc | `TestMemoryUsage_LargeDocument` |
| Concurrent scans | 100+ scans/sec | `BenchmarkScanner_ConcurrentScans` |

### Benchmark Commands

```bash
# Run all benchmarks
make test-compliance-bench

# Run with memory profiling
make test-compliance-memprofile

# Run with CPU profiling
make test-compliance-profile

# Run with execution tracing
make test-compliance-trace
```

---

## Running Tests

### Makefile Targets

```bash
# Run all compliance tests
make test-compliance

# Run unit tests only
make test-compliance-unit

# Run integration tests only
make test-compliance-integration

# Run with coverage report
make test-compliance-coverage

# Run benchmark tests
make test-compliance-bench

# Run with profiling
make test-compliance-profile
```

### Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `TEST_DATA_PATH` | Path to test data directory | `../../../tas-test-data` |
| `SKIP_INTEGRATION` | Skip integration tests | `false` |
| `BENCHMARK_ITERATIONS` | Benchmark iteration count | `1000` |

### Example Test Run

```bash
# Full test suite
cd audimodal
make test-compliance

# Quick unit tests
go test -v -short ./pkg/dlp/...

# Integration tests with specific test data
TEST_DATA_PATH=/path/to/tas-test-data go test -v ./tests/compliance/...

# Coverage report
go test -coverprofile=coverage.out ./pkg/dlp/...
go tool cover -html=coverage.out -o coverage.html
```

---

## CI/CD Integration

### GitHub Actions Workflow

```yaml
name: Compliance Tests

on:
  push:
    paths:
      - 'pkg/dlp/**'
      - 'tests/compliance/**'
  pull_request:
    paths:
      - 'pkg/dlp/**'
      - 'tests/compliance/**'

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Clone test data
        run: |
          git clone https://github.com/your-org/tas-test-data.git ../tas-test-data

      - name: Run unit tests
        run: make test-compliance-unit

      - name: Run integration tests
        run: make test-compliance-integration
        env:
          TEST_DATA_PATH: ../tas-test-data

      - name: Generate coverage report
        run: make test-compliance-coverage

      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          files: ./coverage.out
          flags: compliance
```

### Pre-commit Hooks

```bash
#!/bin/bash
# .git/hooks/pre-commit

# Run compliance unit tests before commit
make test-compliance-unit
if [ $? -ne 0 ]; then
    echo "Compliance unit tests failed. Please fix before committing."
    exit 1
fi
```

---

## Appendix: Test Data Content Examples

### SSN Test Data Format

```
# Valid SSN Formats (should match)
Employee SSN: 123-45-6789
Applicant SSN: 456 78 9012
Record ID: 234567890

# Invalid SSN Formats (should NOT match)
Invalid SSN: 000-12-3456 (starts with 000)
Invalid SSN: 666-12-3456 (starts with 666)
Invalid SSN: 912-34-5678 (starts with 9)
Invalid SSN: 123-00-6789 (middle 00)
Invalid SSN: 123-45-0000 (last 0000)
```

### Credit Card Test Data Format

```
# Valid Credit Cards (should match)
Visa: 4532-1234-5678-9123
Mastercard: 5425-2334-3010-9903
American Express: 3782-822463-10005
Discover: 6011-1234-5678-9012

# Invalid Credit Cards (should NOT match)
Invalid: 1234-5678-9012-3456 (fails Luhn check)
Invalid: 4532-1234-5678-9999 (bad checksum)
```

### GDPR Personal Data Test Format

```
Customer Record
===============
Name: John Smith
Email: john.smith@example.com
Phone: (555) 123-4567
Address: 123 Main Street, Anytown, ST 12345

This document contains personal data subject to GDPR Article 6.
Processing requires a valid legal basis under GDPR.
```
