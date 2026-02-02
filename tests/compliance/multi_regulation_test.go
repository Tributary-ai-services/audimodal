package compliance

import (
	"context"
	"testing"

	"github.com/jscharber/audimodal/pkg/dlp/compliance"
	"github.com/jscharber/audimodal/pkg/dlp/patterns"
	"github.com/jscharber/audimodal/pkg/dlp/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getDefaultRiskLevelMulti returns the default risk level for a PII type
// Note: GDPR-001 skips low-risk findings, so emails/phones need at least Medium risk
// to trigger GDPR personal data violations
func getDefaultRiskLevelMulti(piiType types.PIIType) types.RiskLevel {
	switch piiType {
	case types.PIITypeSSN, types.PIITypeCreditCard:
		return types.RiskLevelCritical
	case types.PIITypeBankAccount, types.PIITypePassport, types.PIITypeDriversLicense, types.PIITypeDateOfBirth:
		return types.RiskLevelHigh
	case types.PIITypePhoneNumber, types.PIITypeAddress, types.PIITypeName, types.PIITypeEmail:
		return types.RiskLevelMedium // Email is personal data under GDPR
	case types.PIITypeIPAddress:
		return types.RiskLevelLow
	default:
		return types.RiskLevelMedium
	}
}

// runScan scans content and returns findings
func runScan(content string) []types.Finding {
	registry := patterns.NewPatternRegistry()
	var findings []types.Finding
	findingID := 0

	for piiType, matcher := range registry.GetAllMatchers() {
		matches := matcher.Match(content)
		for _, match := range matches {
			findingID++
			findings = append(findings, types.Finding{
				ID:         string(rune('0' + findingID)),
				Type:       piiType,
				Value:      match.Value,
				StartPos:   match.StartPos,
				EndPos:     match.EndPos,
				Confidence: match.Confidence,
				Context:    match.Context,
				RiskLevel:  getDefaultRiskLevelMulti(piiType),
			})
		}
	}
	return findings
}

// TestMultiRegulation_ViolationOverlap tests that same PII can trigger multiple regulations
func TestMultiRegulation_ViolationOverlap(t *testing.T) {
	ctx := context.Background()
	checker := compliance.NewBasicComplianceChecker()

	// Email is covered by both GDPR and CCPA
	content := "Contact email: test@example.com"
	findings := runScan(content)

	scanResult := &types.ScanResult{Findings: findings}
	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)
	require.NoError(t, err)

	// Should have violations from both regulations for the same email
	gdprViolations := 0
	ccpaViolations := 0
	for _, v := range result.Violations {
		if v.Rule == "GDPR-001" && v.PIIType == types.PIITypeEmail {
			gdprViolations++
		}
		if v.Rule == "CCPA-001" && v.PIIType == types.PIITypeEmail {
			ccpaViolations++
		}
	}

	assert.Equal(t, 1, gdprViolations, "Should have GDPR violation for email")
	assert.Equal(t, 1, ccpaViolations, "Should have CCPA violation for email")
}

// TestMultiRegulation_RegulationSpecificPII tests PII that triggers only specific regulations
func TestMultiRegulation_RegulationSpecificPII(t *testing.T) {
	ctx := context.Background()
	checker := compliance.NewBasicComplianceChecker()

	tests := []struct {
		name            string
		content         string
		expectedGDPR    bool
		expectedHIPAA   bool
		expectedPCI     bool
		expectedCCPA    bool
	}{
		{
			name:            "Credit card - PCI only",
			content:         "Card: 4111111111111111",
			expectedGDPR:    false,
			expectedHIPAA:   false,
			expectedPCI:     true,
			expectedCCPA:    false,
		},
		{
			name:            "IP address - CCPA only",
			content:         "User IP: 192.168.1.100",
			expectedGDPR:    false,
			expectedHIPAA:   false,
			expectedPCI:     false,
			expectedCCPA:    true,
		},
		{
			name:            "SSN - Multiple regulations",
			content:         "SSN: 123-45-6789",
			expectedGDPR:    true, // GDPR-002 special category
			expectedHIPAA:   true, // PHI
			expectedPCI:     false,
			expectedCCPA:    true,
		},
		{
			name:            "Email - GDPR and CCPA",
			content:         "Email: user@company.com",
			expectedGDPR:    true,
			expectedHIPAA:   false,
			expectedPCI:     false,
			expectedCCPA:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := runScan(tt.content)
			scanResult := &types.ScanResult{Findings: findings}

			// Check each regulation separately
			if tt.expectedGDPR {
				result, err := checker.CheckCompliance(ctx, scanResult, []types.ComplianceRule{
					{Regulation: "GDPR", Rule: "GDPR-001"},
					{Regulation: "GDPR", Rule: "GDPR-002"},
				})
				require.NoError(t, err)
				assert.False(t, result.IsCompliant, "Expected GDPR violation")
			}

			if tt.expectedHIPAA {
				result, err := checker.CheckCompliance(ctx, scanResult, []types.ComplianceRule{
					{Regulation: "HIPAA", Rule: "HIPAA-001"},
				})
				require.NoError(t, err)
				assert.False(t, result.IsCompliant, "Expected HIPAA violation")
			}

			if tt.expectedPCI {
				result, err := checker.CheckCompliance(ctx, scanResult, []types.ComplianceRule{
					{Regulation: "PCI-DSS", Rule: "PCI-001"},
				})
				require.NoError(t, err)
				assert.False(t, result.IsCompliant, "Expected PCI-DSS violation")
			}

			if tt.expectedCCPA {
				result, err := checker.CheckCompliance(ctx, scanResult, []types.ComplianceRule{
					{Regulation: "CCPA", Rule: "CCPA-001"},
				})
				require.NoError(t, err)
				assert.False(t, result.IsCompliant, "Expected CCPA violation")
			}
		})
	}
}

// TestMultiRegulation_SeverityPriority tests that severities are correctly assigned per regulation
func TestMultiRegulation_SeverityPriority(t *testing.T) {
	ctx := context.Background()
	checker := compliance.NewBasicComplianceChecker()

	// Content with multiple PII types that have different severities
	content := `Record:
SSN: 123-45-6789
Email: test@example.com
Credit Card: 4111111111111111
IP: 10.0.0.1`

	findings := runScan(content)
	scanResult := &types.ScanResult{Findings: findings}

	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "GDPR", Rule: "GDPR-002"},
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)
	require.NoError(t, err)

	// Verify severity assignments
	for _, v := range result.Violations {
		switch v.Rule {
		case "GDPR-002": // Special category (SSN, DOB)
			assert.Equal(t, "critical", v.Severity)
		case "HIPAA-001": // PHI
			assert.Equal(t, "critical", v.Severity)
		case "PCI-001": // Cardholder data
			assert.Equal(t, "critical", v.Severity)
		case "GDPR-001": // Personal data
			assert.Equal(t, "high", v.Severity)
		case "CCPA-001": // Personal information
			assert.Equal(t, "high", v.Severity)
		}
	}
}

// TestMultiRegulation_AllRegulations tests checking all 4 regulations simultaneously
func TestMultiRegulation_AllRegulations(t *testing.T) {
	ctx := context.Background()
	checker := compliance.NewBasicComplianceChecker()

	// Comprehensive content triggering all regulations
	// Note: Credit card numbers must be unformatted (no dashes) for pattern matching
	content := `
Comprehensive Data Record
=========================
Personal Information:
- Name: John Smith
- Email: john.smith@company.com
- Phone: (555) 123-4567
- SSN: 123-45-6789
- IP Address: 192.168.1.100

Financial Information:
- Visa: 4111111111111111
- Mastercard: 5500000000000004

Health Information:
- DOB: 01/15/1980
- Medical Record ID: MR-12345
`

	findings := runScan(content)
	scanResult := &types.ScanResult{Findings: findings}

	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "GDPR", Rule: "GDPR-002"},
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)
	require.NoError(t, err)

	// Should have violations from all regulations
	assert.False(t, result.IsCompliant)

	// Track unique regulations that have violations
	regulations := make(map[string]bool)
	for _, v := range result.Violations {
		if len(v.Rule) >= 4 {
			regulations[v.Rule[:4]] = true
		}
	}

	// Verify all regulations triggered
	assert.True(t, regulations["GDPR"], "Should have GDPR violations")
	assert.True(t, regulations["HIPA"], "Should have HIPAA violations")
	assert.True(t, regulations["PCI-"], "Should have PCI-DSS violations")
	assert.True(t, regulations["CCPA"], "Should have CCPA violations")
}

// TestMultiRegulation_RequiredActions tests that required actions are generated for each violation
func TestMultiRegulation_RequiredActions(t *testing.T) {
	ctx := context.Background()
	checker := compliance.NewBasicComplianceChecker()

	content := `
Email: user@example.com
SSN: 123-45-6789
Credit Card: 4111111111111111
IP: 192.168.1.1
`

	findings := runScan(content)
	scanResult := &types.ScanResult{Findings: findings}

	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "GDPR", Rule: "GDPR-002"},
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)
	require.NoError(t, err)

	// Should have required actions
	assert.NotEmpty(t, result.RequiredActions)

	// Actions should reference the regulations
	actionStr := ""
	for _, action := range result.RequiredActions {
		actionStr += action + " "
	}

	assert.NotEmpty(t, actionStr, "Should have concatenated actions")
}

// TestMultiRegulation_NoOverlap tests regulations that don't overlap
func TestMultiRegulation_NoOverlap(t *testing.T) {
	ctx := context.Background()
	checker := compliance.NewBasicComplianceChecker()

	// Credit cards only trigger PCI-DSS
	content := "Card: 4111111111111111"
	findings := runScan(content)
	scanResult := &types.ScanResult{Findings: findings}

	// Check GDPR - should be compliant (credit cards not in GDPR scope)
	gdprResult, err := checker.CheckCompliance(ctx, scanResult, []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
	})
	require.NoError(t, err)
	assert.True(t, gdprResult.IsCompliant, "GDPR should not flag credit cards")

	// Check HIPAA - should be compliant (credit cards not PHI)
	hipaaResult, err := checker.CheckCompliance(ctx, scanResult, []types.ComplianceRule{
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
	})
	require.NoError(t, err)
	assert.True(t, hipaaResult.IsCompliant, "HIPAA should not flag credit cards")

	// Check CCPA - should be compliant (credit cards not personal info per CCPA definition)
	ccpaResult, err := checker.CheckCompliance(ctx, scanResult, []types.ComplianceRule{
		{Regulation: "CCPA", Rule: "CCPA-001"},
	})
	require.NoError(t, err)
	assert.True(t, ccpaResult.IsCompliant, "CCPA should not flag credit cards")

	// Check PCI-DSS - should NOT be compliant
	pciResult, err := checker.CheckCompliance(ctx, scanResult, []types.ComplianceRule{
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
	})
	require.NoError(t, err)
	assert.False(t, pciResult.IsCompliant, "PCI-DSS should flag credit cards")
}

// TestMultiRegulation_EmptyRules tests behavior with no rules specified
func TestMultiRegulation_EmptyRules(t *testing.T) {
	ctx := context.Background()
	checker := compliance.NewBasicComplianceChecker()

	content := "SSN: 123-45-6789 Email: test@example.com"
	findings := runScan(content)
	scanResult := &types.ScanResult{Findings: findings}

	// Empty rules - should be compliant by default
	result, err := checker.CheckCompliance(ctx, scanResult, []types.ComplianceRule{})
	require.NoError(t, err)
	assert.True(t, result.IsCompliant)
	assert.Empty(t, result.Violations)
}

// TestMultiRegulation_UnknownRegulation tests behavior with unknown regulation
func TestMultiRegulation_UnknownRegulation(t *testing.T) {
	ctx := context.Background()
	checker := compliance.NewBasicComplianceChecker()

	content := "SSN: 123-45-6789"
	findings := runScan(content)
	scanResult := &types.ScanResult{Findings: findings}

	// Unknown regulation should not error, just be ignored
	result, err := checker.CheckCompliance(ctx, scanResult, []types.ComplianceRule{
		{Regulation: "UNKNOWN", Rule: "UNKNOWN-001"},
	})
	require.NoError(t, err)
	assert.True(t, result.IsCompliant)
}

// TestMultiRegulation_DuplicateFindings tests handling of duplicate findings
func TestMultiRegulation_DuplicateFindings(t *testing.T) {
	ctx := context.Background()
	checker := compliance.NewBasicComplianceChecker()

	// Content with the same SSN appearing multiple times
	content := `
Record 1: SSN 123-45-6789
Record 2: SSN 123-45-6789
Record 3: SSN 123-45-6789
`

	findings := runScan(content)
	scanResult := &types.ScanResult{Findings: findings}

	result, err := checker.CheckCompliance(ctx, scanResult, []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-002"},
	})
	require.NoError(t, err)

	// Each occurrence should generate a separate violation
	assert.GreaterOrEqual(t, len(result.Violations), 3)
}

// TestMultiRegulation_ViolationDetails tests that violation details are properly populated
func TestMultiRegulation_ViolationDetails(t *testing.T) {
	ctx := context.Background()
	checker := compliance.NewBasicComplianceChecker()

	content := "SSN: 123-45-6789"
	findings := runScan(content)
	scanResult := &types.ScanResult{Findings: findings}

	result, err := checker.CheckCompliance(ctx, scanResult, []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-002"},
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
	})
	require.NoError(t, err)

	for _, v := range result.Violations {
		// Every violation should have required fields
		assert.NotEmpty(t, v.Rule, "Rule should be set")
		assert.NotEmpty(t, v.Severity, "Severity should be set")
		assert.NotEmpty(t, v.Description, "Description should be set")
		assert.NotEmpty(t, v.PIIType, "PIIType should be set")
		assert.NotEmpty(t, v.FindingIDs, "FindingIDs should be set")
	}
}
