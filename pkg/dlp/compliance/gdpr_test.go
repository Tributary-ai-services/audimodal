package compliance

import (
	"context"
	"testing"

	"github.com/jscharber/audimodal/pkg/dlp/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGDPRChecker_PersonalData_WithFindings(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	tests := []struct {
		name         string
		findings     []types.Finding
		expectViols  bool
		minViolCount int
	}{
		{
			name: "email triggers GDPR-001",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeEmail, RiskLevel: types.RiskLevelMedium},
			},
			expectViols:  true,
			minViolCount: 1,
		},
		{
			name: "name triggers GDPR-001",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeName, RiskLevel: types.RiskLevelMedium},
			},
			expectViols:  true,
			minViolCount: 1,
		},
		{
			name: "address triggers GDPR-001",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeAddress, RiskLevel: types.RiskLevelMedium},
			},
			expectViols:  true,
			minViolCount: 1,
		},
		{
			name: "phone triggers GDPR-001",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypePhoneNumber, RiskLevel: types.RiskLevelMedium},
			},
			expectViols:  true,
			minViolCount: 1,
		},
		{
			name: "multiple personal data types",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeEmail, RiskLevel: types.RiskLevelMedium},
				{ID: "2", Type: types.PIITypeName, RiskLevel: types.RiskLevelMedium},
				{ID: "3", Type: types.PIITypePhoneNumber, RiskLevel: types.RiskLevelMedium},
			},
			expectViols:  true,
			minViolCount: 3,
		},
		{
			name: "low risk email does not trigger",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeEmail, RiskLevel: types.RiskLevelLow},
			},
			expectViols:  false,
			minViolCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanResult := &types.ScanResult{Findings: tt.findings}
			rules := []types.ComplianceRule{
				{Regulation: "GDPR", Rule: "GDPR-001"},
			}

			result, err := checker.CheckCompliance(ctx, scanResult, rules)

			require.NoError(t, err)
			if tt.expectViols {
				assert.False(t, result.IsCompliant)
				assert.GreaterOrEqual(t, len(result.Violations), tt.minViolCount)

				// Verify all violations are for GDPR-001
				for _, v := range result.Violations {
					assert.Equal(t, "GDPR-001", v.Rule)
					assert.Equal(t, "high", v.Severity)
				}
			} else {
				assert.True(t, result.IsCompliant)
			}
		})
	}
}

func TestGDPRChecker_SpecialCategory_WithFindings(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	tests := []struct {
		name         string
		findings     []types.Finding
		expectViols  bool
		minViolCount int
	}{
		{
			name: "SSN triggers GDPR-002",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeSSN, RiskLevel: types.RiskLevelCritical},
			},
			expectViols:  true,
			minViolCount: 1,
		},
		{
			name: "DOB triggers GDPR-002",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeDateOfBirth, RiskLevel: types.RiskLevelHigh},
			},
			expectViols:  true,
			minViolCount: 1,
		},
		{
			name: "multiple special category data",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeSSN, RiskLevel: types.RiskLevelCritical},
				{ID: "2", Type: types.PIITypeDateOfBirth, RiskLevel: types.RiskLevelHigh},
			},
			expectViols:  true,
			minViolCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanResult := &types.ScanResult{Findings: tt.findings}
			rules := []types.ComplianceRule{
				{Regulation: "GDPR", Rule: "GDPR-002"},
			}

			result, err := checker.CheckCompliance(ctx, scanResult, rules)

			require.NoError(t, err)
			if tt.expectViols {
				assert.False(t, result.IsCompliant)
				assert.GreaterOrEqual(t, len(result.Violations), tt.minViolCount)

				// Verify all violations are for GDPR-002 with critical severity
				for _, v := range result.Violations {
					assert.Equal(t, "GDPR-002", v.Rule)
					assert.Equal(t, "critical", v.Severity)
				}
			} else {
				assert.True(t, result.IsCompliant)
			}
		})
	}
}

func TestGDPRChecker_BothRules(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	// Create findings that trigger both GDPR rules
	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "1", Type: types.PIITypeEmail, RiskLevel: types.RiskLevelMedium},   // GDPR-001
			{ID: "2", Type: types.PIITypeName, RiskLevel: types.RiskLevelMedium},    // GDPR-001
			{ID: "3", Type: types.PIITypeSSN, RiskLevel: types.RiskLevelCritical},   // GDPR-002
			{ID: "4", Type: types.PIITypeDateOfBirth, RiskLevel: types.RiskLevelHigh}, // GDPR-002
		},
	}

	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "GDPR", Rule: "GDPR-002"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	require.NoError(t, err)
	assert.False(t, result.IsCompliant)

	// Count violations by rule
	rule001Count := 0
	rule002Count := 0
	for _, v := range result.Violations {
		switch v.Rule {
		case "GDPR-001":
			rule001Count++
			assert.Equal(t, "high", v.Severity)
		case "GDPR-002":
			rule002Count++
			assert.Equal(t, "critical", v.Severity)
		}
	}

	assert.GreaterOrEqual(t, rule001Count, 2, "Expected at least 2 GDPR-001 violations")
	assert.GreaterOrEqual(t, rule002Count, 2, "Expected at least 2 GDPR-002 violations")
}

func TestGDPRChecker_NoRelevantFindings(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	// Credit card is not a GDPR personal data type
	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "1", Type: types.PIITypeCreditCard, RiskLevel: types.RiskLevelCritical},
		},
	}

	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	require.NoError(t, err)
	assert.True(t, result.IsCompliant)
	assert.Empty(t, result.Violations)
}

func TestGDPRChecker_RequiredActions(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "1", Type: types.PIITypeEmail, RiskLevel: types.RiskLevelMedium},
		},
	}

	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	require.NoError(t, err)
	assert.NotEmpty(t, result.RequiredActions)

	// Check for expected GDPR actions
	actionStr := ""
	for _, action := range result.RequiredActions {
		actionStr += action + " "
	}
	assert.Contains(t, actionStr, "data protection")
}
