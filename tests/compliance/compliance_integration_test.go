package compliance

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jscharber/audimodal/pkg/dlp/compliance"
	"github.com/jscharber/audimodal/pkg/dlp/patterns"
	"github.com/jscharber/audimodal/pkg/dlp/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTestDataPath returns the path to test data files
func getTestDataPath() string {
	// Check environment variable first
	if path := os.Getenv("TEST_DATA_PATH"); path != "" {
		return path
	}
	// Default relative path from audimodal directory
	return "../../tas-test-data/txt"
}

// getDefaultRiskLevel returns the default risk level for a PII type
// Note: GDPR-001 skips low-risk findings, so emails need at least Medium risk
// to trigger GDPR personal data violations (emails ARE personal data under GDPR)
func getDefaultRiskLevel(piiType types.PIIType) types.RiskLevel {
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

// TestIntegration_PatternMatching_SSN tests SSN pattern matching integration
func TestIntegration_PatternMatching_SSN(t *testing.T) {
	registry := patterns.NewPatternRegistry()
	matcher := registry.GetMatcher(types.PIITypeSSN)
	require.NotNil(t, matcher)

	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"dashed format", "Employee SSN: 123-45-6789", 1},
		{"space format", "SSN: 123 45 6789", 1},
		{"no separator", "SSN: 123456789", 1},
		{"multiple SSNs", "SSN1: 123-45-6789, SSN2: 234-56-7890", 2},
		{"invalid starts with 000", "SSN: 000-12-3456", 0},
		{"invalid starts with 666", "SSN: 666-12-3456", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := matcher.Match(tt.input)
			assert.Equal(t, tt.expected, len(matches))
		})
	}
}

// TestIntegration_PatternMatching_CreditCard tests credit card pattern matching integration
func TestIntegration_PatternMatching_CreditCard(t *testing.T) {
	registry := patterns.NewPatternRegistry()
	matcher := registry.GetMatcher(types.PIITypeCreditCard)
	require.NotNil(t, matcher)

	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"visa", "Card: 4532015112830366", 1},
		{"mastercard", "Card: 5425233430109903", 1},
		{"amex", "Card: 378282246310005", 1},
		{"discover", "Card: 6011111111111117", 1},
		{"invalid luhn", "Card: 1234567890123456", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := matcher.Match(tt.input)
			assert.Equal(t, tt.expected, len(matches))
		})
	}
}

// TestIntegration_Compliance_GDPR tests GDPR compliance checking integration
func TestIntegration_Compliance_GDPR(t *testing.T) {
	ctx := context.Background()
	registry := patterns.NewPatternRegistry()
	checker := compliance.NewBasicComplianceChecker()

	// Sample content with GDPR-relevant PII
	content := `Customer Record
Name: John Smith
Email: john.smith@example.com
Phone: (555) 123-4567
SSN: 123-45-6789`

	// Run pattern matchers and collect findings
	var findings []types.Finding
	findingID := 0
	for piiType, matcher := range registry.GetAllMatchers() {
		matches := matcher.Match(content)
		for _, match := range matches {
			findingID++
			findings = append(findings, types.Finding{
				ID:         string(rune(findingID)),
				Type:       piiType,
				Value:      match.Value,
				StartPos:   match.StartPos,
				EndPos:     match.EndPos,
				Confidence: match.Confidence,
				Context:    match.Context,
				RiskLevel:  getDefaultRiskLevel(piiType),
			})
		}
	}

	// Check GDPR compliance
	scanResult := &types.ScanResult{Findings: findings}
	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "GDPR", Rule: "GDPR-002"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)
	require.NoError(t, err)

	// Should have violations for personal data
	assert.False(t, result.IsCompliant)
	assert.NotEmpty(t, result.Violations)

	// Should have required actions
	assert.NotEmpty(t, result.RequiredActions)
}

// TestIntegration_Compliance_HIPAA tests HIPAA compliance checking integration
func TestIntegration_Compliance_HIPAA(t *testing.T) {
	ctx := context.Background()
	registry := patterns.NewPatternRegistry()
	checker := compliance.NewBasicComplianceChecker()

	// Sample content with HIPAA-relevant PHI
	content := `Medical Record
Patient: Jane Doe
SSN: 234-56-7890
DOB: 05/15/1985
Email: jane.doe@hospital.com`

	// Run pattern matchers and collect findings
	var findings []types.Finding
	findingID := 0
	for piiType, matcher := range registry.GetAllMatchers() {
		matches := matcher.Match(content)
		for _, match := range matches {
			findingID++
			findings = append(findings, types.Finding{
				ID:         string(rune(findingID)),
				Type:       piiType,
				Value:      match.Value,
				StartPos:   match.StartPos,
				EndPos:     match.EndPos,
				Confidence: match.Confidence,
				Context:    match.Context,
				RiskLevel:  getDefaultRiskLevel(piiType),
			})
		}
	}

	// Check HIPAA compliance
	scanResult := &types.ScanResult{Findings: findings}
	rules := []types.ComplianceRule{
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)
	require.NoError(t, err)

	// Should have violations for PHI
	assert.False(t, result.IsCompliant)
	assert.NotEmpty(t, result.Violations)

	// Verify critical severity for HIPAA
	for _, v := range result.Violations {
		assert.Equal(t, "critical", v.Severity)
	}
}

// TestIntegration_Compliance_PCI tests PCI-DSS compliance checking integration
func TestIntegration_Compliance_PCI(t *testing.T) {
	ctx := context.Background()
	registry := patterns.NewPatternRegistry()
	checker := compliance.NewBasicComplianceChecker()

	// Sample content with credit card data
	// Note: Credit card numbers must be unformatted (no dashes) for pattern matching
	content := `Payment Record
Visa: 4532015112830366
Mastercard: 5425233430109903`

	// Run pattern matchers and collect findings
	var findings []types.Finding
	findingID := 0
	for piiType, matcher := range registry.GetAllMatchers() {
		matches := matcher.Match(content)
		for _, match := range matches {
			findingID++
			findings = append(findings, types.Finding{
				ID:         string(rune(findingID)),
				Type:       piiType,
				Value:      match.Value,
				StartPos:   match.StartPos,
				EndPos:     match.EndPos,
				Confidence: match.Confidence,
				Context:    match.Context,
				RiskLevel:  getDefaultRiskLevel(piiType),
			})
		}
	}

	// Check PCI-DSS compliance
	scanResult := &types.ScanResult{Findings: findings}
	rules := []types.ComplianceRule{
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)
	require.NoError(t, err)

	// Should have violations for cardholder data
	assert.False(t, result.IsCompliant)
	assert.Len(t, result.Violations, 2) // 2 credit cards

	// Verify all violations are for credit cards
	for _, v := range result.Violations {
		assert.Equal(t, types.PIITypeCreditCard, v.PIIType)
		assert.Equal(t, "critical", v.Severity)
	}
}

// TestIntegration_Compliance_CCPA tests CCPA compliance checking integration
func TestIntegration_Compliance_CCPA(t *testing.T) {
	ctx := context.Background()
	registry := patterns.NewPatternRegistry()
	checker := compliance.NewBasicComplianceChecker()

	// Sample content with CCPA-relevant personal info
	content := `Consumer Profile
Email: consumer@california.com
Name: John Californian
IP Address: 192.168.1.100
SSN: 345-67-8901`

	// Run pattern matchers and collect findings
	var findings []types.Finding
	findingID := 0
	for piiType, matcher := range registry.GetAllMatchers() {
		matches := matcher.Match(content)
		for _, match := range matches {
			findingID++
			findings = append(findings, types.Finding{
				ID:         string(rune(findingID)),
				Type:       piiType,
				Value:      match.Value,
				StartPos:   match.StartPos,
				EndPos:     match.EndPos,
				Confidence: match.Confidence,
				Context:    match.Context,
				RiskLevel:  getDefaultRiskLevel(piiType),
			})
		}
	}

	// Check CCPA compliance
	scanResult := &types.ScanResult{Findings: findings}
	rules := []types.ComplianceRule{
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)
	require.NoError(t, err)

	// Should have violations for personal information
	assert.False(t, result.IsCompliant)
	assert.NotEmpty(t, result.Violations)

	// CCPA uniquely covers IP addresses
	foundIPViolation := false
	for _, v := range result.Violations {
		if v.PIIType == types.PIITypeIPAddress {
			foundIPViolation = true
			break
		}
	}
	assert.True(t, foundIPViolation, "CCPA should flag IP addresses")
}

// TestIntegration_MultiRegulation tests checking multiple regulations at once
func TestIntegration_MultiRegulation(t *testing.T) {
	ctx := context.Background()
	registry := patterns.NewPatternRegistry()
	checker := compliance.NewBasicComplianceChecker()

	// Sample content with multiple PII types
	// Note: Credit card numbers must be unformatted (no dashes) for pattern matching
	content := `Comprehensive Record
Name: Test User
Email: test@example.com
Phone: (555) 987-6543
SSN: 456-78-9012
Credit Card: 4111111111111111
IP: 10.0.0.1`

	// Run pattern matchers and collect findings
	var findings []types.Finding
	findingID := 0
	for piiType, matcher := range registry.GetAllMatchers() {
		matches := matcher.Match(content)
		for _, match := range matches {
			findingID++
			findings = append(findings, types.Finding{
				ID:         string(rune(findingID)),
				Type:       piiType,
				Value:      match.Value,
				StartPos:   match.StartPos,
				EndPos:     match.EndPos,
				Confidence: match.Confidence,
				Context:    match.Context,
				RiskLevel:  getDefaultRiskLevel(piiType),
			})
		}
	}

	// Check all regulations
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

	// Should have violations from multiple regulations
	assert.False(t, result.IsCompliant)
	assert.NotEmpty(t, result.Violations)

	// Count violations by regulation
	regulationCounts := make(map[string]int)
	for _, v := range result.Violations {
		regulationCounts[v.Rule[:4]]++ // Extract first 4 chars (GDPR, HIPA, PCI-, CCPA)
	}

	// Should have violations from each regulation
	assert.Greater(t, regulationCounts["GDPR"], 0, "Should have GDPR violations")
	assert.Greater(t, regulationCounts["PCI-"], 0, "Should have PCI-DSS violations")
	assert.Greater(t, regulationCounts["CCPA"], 0, "Should have CCPA violations")
}

// TestIntegration_CleanDocument tests compliance checking with no PII
func TestIntegration_CleanDocument(t *testing.T) {
	ctx := context.Background()
	registry := patterns.NewPatternRegistry()
	checker := compliance.NewBasicComplianceChecker()

	// Clean content with no PII
	content := `Project Status Report
The quarterly results show strong performance across all metrics.
Team morale remains high and we are on track for the yearly goals.
No sensitive information included in this summary.`

	// Run pattern matchers and collect findings
	var findings []types.Finding
	for _, matcher := range registry.GetAllMatchers() {
		matches := matcher.Match(content)
		assert.Empty(t, matches, "Clean document should have no PII matches")
	}

	// Check all regulations with no findings
	scanResult := &types.ScanResult{Findings: findings}
	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)
	require.NoError(t, err)

	// Should be compliant with no violations
	assert.True(t, result.IsCompliant)
	assert.Empty(t, result.Violations)
}

// TestIntegration_ConfidenceScores tests that confidence scores are properly calculated
func TestIntegration_ConfidenceScores(t *testing.T) {
	registry := patterns.NewPatternRegistry()

	tests := []struct {
		name       string
		piiType    types.PIIType
		input      string
		minConf    float64
	}{
		{"SSN dashed high confidence", types.PIITypeSSN, "123-45-6789", 0.85},
		{"SSN plain lower confidence", types.PIITypeSSN, "123456789", 0.6},
		{"Credit card formatted high", types.PIITypeCreditCard, "4532-0151-1283-0366", 0.85},
		{"Phone parentheses high", types.PIITypePhoneNumber, "(555) 123-4567", 0.85},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := registry.GetMatcher(tt.piiType)
			require.NotNil(t, matcher)

			conf := matcher.GetConfidenceScore(tt.input)
			assert.GreaterOrEqual(t, conf, tt.minConf)
		})
	}
}

// TestIntegration_FalsePositiveReduction tests that false positives are properly rejected
func TestIntegration_FalsePositiveReduction(t *testing.T) {
	registry := patterns.NewPatternRegistry()

	// Content with things that look like PII but aren't
	falsePositives := []struct {
		name    string
		piiType types.PIIType
		input   string
	}{
		{"SSN invalid prefix 000", types.PIITypeSSN, "000-12-3456"},
		{"SSN invalid prefix 666", types.PIITypeSSN, "666-12-3456"},
		{"SSN invalid prefix 9xx", types.PIITypeSSN, "912-34-5678"},
		{"Credit card fails Luhn", types.PIITypeCreditCard, "1234567890123456"},
		{"IP invalid octet > 255", types.PIITypeIPAddress, "256.1.1.1"},
	}

	for _, tt := range falsePositives {
		t.Run(tt.name, func(t *testing.T) {
			matcher := registry.GetMatcher(tt.piiType)
			require.NotNil(t, matcher)

			matches := matcher.Match(tt.input)
			assert.Empty(t, matches, "Should not match false positive: %s", tt.input)
		})
	}
}

// TestIntegration_LargeDocument tests processing a large document
func TestIntegration_LargeDocument(t *testing.T) {
	registry := patterns.NewPatternRegistry()

	// Generate a large document with known PII
	var content string
	expectedSSN := 0
	expectedEmail := 0

	for i := 0; i < 100; i++ {
		if i%10 == 0 {
			content += "Employee SSN: 123-45-6789\n"
			expectedSSN++
		}
		if i%15 == 0 {
			content += "Contact: user@example.com\n"
			expectedEmail++
		}
		content += "Lorem ipsum dolor sit amet, consectetur adipiscing elit.\n"
	}

	// Count SSN matches
	ssnMatcher := registry.GetMatcher(types.PIITypeSSN)
	ssnMatches := ssnMatcher.Match(content)
	assert.Equal(t, expectedSSN, len(ssnMatches))

	// Count Email matches
	emailMatcher := registry.GetMatcher(types.PIITypeEmail)
	emailMatches := emailMatcher.Match(content)
	assert.Equal(t, expectedEmail, len(emailMatches))
}

// TestIntegration_TestDataFiles tests reading from actual test data files if available
func TestIntegration_TestDataFiles(t *testing.T) {
	testDataPath := getTestDataPath()

	// Skip if test data not available
	if _, err := os.Stat(testDataPath); os.IsNotExist(err) {
		t.Skipf("Test data not available at %s, skipping file-based tests", testDataPath)
	}

	registry := patterns.NewPatternRegistry()

	tests := []struct {
		name     string
		file     string
		piiType  types.PIIType
		minCount int
	}{
		{"SSN samples", "pii-types/ssn_samples.txt", types.PIITypeSSN, 3},
		{"Credit card samples", "pii-types/credit_card_samples.txt", types.PIITypeCreditCard, 3},
		{"Email samples", "pii-types/email_samples.txt", types.PIITypeEmail, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := filepath.Join(testDataPath, tt.file)

			// Skip if specific file doesn't exist
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				t.Skipf("Test file not available: %s", filePath)
			}

			content, err := os.ReadFile(filePath)
			require.NoError(t, err)

			matcher := registry.GetMatcher(tt.piiType)
			require.NotNil(t, matcher)

			matches := matcher.Match(string(content))
			assert.GreaterOrEqual(t, len(matches), tt.minCount,
				"Expected at least %d matches in %s", tt.minCount, tt.file)
		})
	}
}
