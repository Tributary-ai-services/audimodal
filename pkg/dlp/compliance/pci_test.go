package compliance

import (
	"context"
	"testing"

	"github.com/jscharber/audimodal/pkg/dlp/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPCIChecker_CardholderData_WithFindings(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	tests := []struct {
		name         string
		findings     []types.Finding
		expectViols  bool
		minViolCount int
	}{
		{
			name: "single credit card triggers PCI-001",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeCreditCard, RiskLevel: types.RiskLevelCritical},
			},
			expectViols:  true,
			minViolCount: 1,
		},
		{
			name: "multiple credit cards",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeCreditCard, RiskLevel: types.RiskLevelCritical},
				{ID: "2", Type: types.PIITypeCreditCard, RiskLevel: types.RiskLevelCritical},
				{ID: "3", Type: types.PIITypeCreditCard, RiskLevel: types.RiskLevelCritical},
			},
			expectViols:  true,
			minViolCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanResult := &types.ScanResult{Findings: tt.findings}
			rules := []types.ComplianceRule{
				{Regulation: "PCI-DSS", Rule: "PCI-001"},
			}

			result, err := checker.CheckCompliance(ctx, scanResult, rules)

			require.NoError(t, err)
			if tt.expectViols {
				assert.False(t, result.IsCompliant)
				assert.GreaterOrEqual(t, len(result.Violations), tt.minViolCount)

				// Verify all violations are for PCI-001 with critical severity
				for _, v := range result.Violations {
					assert.Equal(t, "PCI-001", v.Rule)
					assert.Equal(t, "critical", v.Severity)
					assert.Equal(t, types.PIITypeCreditCard, v.PIIType)
				}
			} else {
				assert.True(t, result.IsCompliant)
			}
		})
	}
}

func TestPCIChecker_NoRelevantFindings(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	// SSN, email, phone are not PCI-DSS cardholder data
	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "1", Type: types.PIITypeSSN, RiskLevel: types.RiskLevelCritical},
			{ID: "2", Type: types.PIITypeEmail, RiskLevel: types.RiskLevelMedium},
			{ID: "3", Type: types.PIITypePhoneNumber, RiskLevel: types.RiskLevelMedium},
			{ID: "4", Type: types.PIITypeIPAddress, RiskLevel: types.RiskLevelLow},
		},
	}

	rules := []types.ComplianceRule{
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	require.NoError(t, err)
	assert.True(t, result.IsCompliant)
	assert.Empty(t, result.Violations)
}

func TestPCIChecker_RequiredActions(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "1", Type: types.PIITypeCreditCard, RiskLevel: types.RiskLevelCritical},
		},
	}

	rules := []types.ComplianceRule{
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	require.NoError(t, err)
	assert.NotEmpty(t, result.RequiredActions)

	// Check for expected PCI-DSS actions
	actionStr := ""
	for _, action := range result.RequiredActions {
		actionStr += action + " "
	}
	assert.Contains(t, actionStr, "PCI DSS")
}

func TestPCIChecker_ViolationDescription(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "1", Type: types.PIITypeCreditCard, RiskLevel: types.RiskLevelCritical},
		},
	}

	rules := []types.ComplianceRule{
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	require.NoError(t, err)
	require.NotEmpty(t, result.Violations)

	// Check violation description mentions cardholder data
	assert.Contains(t, result.Violations[0].Description, "Cardholder data")
}

func TestPCIChecker_MixedPIITypes(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	// Mix of credit cards and other PII
	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "1", Type: types.PIITypeCreditCard, RiskLevel: types.RiskLevelCritical},
			{ID: "2", Type: types.PIITypeEmail, RiskLevel: types.RiskLevelMedium},
			{ID: "3", Type: types.PIITypeCreditCard, RiskLevel: types.RiskLevelCritical},
			{ID: "4", Type: types.PIITypeSSN, RiskLevel: types.RiskLevelCritical},
		},
	}

	rules := []types.ComplianceRule{
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	require.NoError(t, err)
	assert.False(t, result.IsCompliant)

	// Should only have violations for credit cards (2 violations)
	assert.Equal(t, 2, len(result.Violations))

	for _, v := range result.Violations {
		assert.Equal(t, types.PIITypeCreditCard, v.PIIType)
	}
}
