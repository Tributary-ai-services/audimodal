package compliance

import (
	"context"
	"testing"
	"time"

	"github.com/jscharber/audimodal/pkg/dlp/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBasicComplianceChecker(t *testing.T) {
	checker := NewBasicComplianceChecker()

	assert.NotNil(t, checker)
	assert.Equal(t, "basic_compliance_checker", checker.name)
	assert.Equal(t, "1.0.0", checker.version)

	// Verify all four regulations are registered
	regulations := checker.GetSupportedRegulations()
	assert.Contains(t, regulations, "GDPR")
	assert.Contains(t, regulations, "HIPAA")
	assert.Contains(t, regulations, "PCI-DSS")
	assert.Contains(t, regulations, "CCPA")
}

func TestGetSupportedRegulations(t *testing.T) {
	checker := NewBasicComplianceChecker()
	regulations := checker.GetSupportedRegulations()

	assert.Len(t, regulations, 4)
	assert.Contains(t, regulations, "GDPR")
	assert.Contains(t, regulations, "HIPAA")
	assert.Contains(t, regulations, "PCI-DSS")
	assert.Contains(t, regulations, "CCPA")
}

func TestCheckCompliance_NoFindings(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	scanResult := &types.ScanResult{
		ChunkID:      "test-chunk-1",
		ScannedAt:    time.Now(),
		Scanner:      "test",
		TotalMatches: 0,
		Findings:     []types.Finding{},
	}

	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	require.NoError(t, err)
	assert.True(t, result.IsCompliant)
	assert.Empty(t, result.Violations)
}

func TestCheckCompliance_MultipleRegulations(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	// Create findings that trigger multiple regulations
	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "1", Type: types.PIITypeEmail, RiskLevel: types.RiskLevelMedium},
			{ID: "2", Type: types.PIITypeSSN, RiskLevel: types.RiskLevelCritical},
			{ID: "3", Type: types.PIITypeCreditCard, RiskLevel: types.RiskLevelCritical},
		},
	}

	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "GDPR", Rule: "GDPR-002"},
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	require.NoError(t, err)
	assert.False(t, result.IsCompliant)
	assert.NotEmpty(t, result.Violations)
	assert.NotEmpty(t, result.RequiredActions)
}

func TestCheckCompliance_UnsupportedRegulation(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "1", Type: types.PIITypeEmail, RiskLevel: types.RiskLevelMedium},
		},
	}

	rules := []types.ComplianceRule{
		{Regulation: "UNKNOWN", Rule: "UNKNOWN-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	// Should not error, just skip unknown regulations
	require.NoError(t, err)
	assert.True(t, result.IsCompliant)
}

func TestIsRuleEnabled(t *testing.T) {
	checker := NewBasicComplianceChecker()

	tests := []struct {
		name     string
		ruleID   string
		rules    []types.ComplianceRule
		expected bool
	}{
		{
			name:   "exact rule match",
			ruleID: "GDPR-001",
			rules: []types.ComplianceRule{
				{Regulation: "GDPR", Rule: "GDPR-001"},
			},
			expected: true,
		},
		{
			name:   "regulation match enables all rules",
			ruleID: "GDPR-001",
			rules: []types.ComplianceRule{
				{Regulation: "GDPR"},
			},
			expected: true,
		},
		{
			name:   "no matching rule",
			ruleID: "HIPAA-001",
			rules: []types.ComplianceRule{
				{Regulation: "GDPR", Rule: "GDPR-001"},
			},
			expected: false,
		},
		{
			name:     "empty rules",
			ruleID:   "GDPR-001",
			rules:    []types.ComplianceRule{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.isRuleEnabled(tt.ruleID, tt.rules)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateRequiredActions(t *testing.T) {
	checker := NewBasicComplianceChecker()

	tests := []struct {
		name       string
		ruleID     string
		minActions int
	}{
		{"GDPR-001", "GDPR-001", 2},
		{"GDPR-002", "GDPR-002", 2},
		{"HIPAA-001", "HIPAA-001", 2},
		{"PCI-001", "PCI-001", 2},
		{"CCPA-001", "CCPA-001", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ruleDef := ComplianceRuleDefinition{RuleID: tt.ruleID}
			violations := []types.ComplianceViolation{{Rule: tt.ruleID}}

			actions := checker.generateRequiredActions(ruleDef, violations)

			assert.GreaterOrEqual(t, len(actions), tt.minActions,
				"Expected at least %d actions for rule %s", tt.minActions, tt.ruleID)
		})
	}
}
