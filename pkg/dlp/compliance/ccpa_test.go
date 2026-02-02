package compliance

import (
	"context"
	"testing"

	"github.com/jscharber/audimodal/pkg/dlp/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCCPAChecker_PersonalInfo_WithFindings(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	tests := []struct {
		name         string
		findings     []types.Finding
		expectViols  bool
		minViolCount int
	}{
		{
			name: "email triggers CCPA-001",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeEmail, RiskLevel: types.RiskLevelMedium},
			},
			expectViols:  true,
			minViolCount: 1,
		},
		{
			name: "name triggers CCPA-001",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeName, RiskLevel: types.RiskLevelMedium},
			},
			expectViols:  true,
			minViolCount: 1,
		},
		{
			name: "address triggers CCPA-001",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeAddress, RiskLevel: types.RiskLevelMedium},
			},
			expectViols:  true,
			minViolCount: 1,
		},
		{
			name: "SSN triggers CCPA-001",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeSSN, RiskLevel: types.RiskLevelCritical},
			},
			expectViols:  true,
			minViolCount: 1,
		},
		{
			name: "IP address triggers CCPA-001",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeIPAddress, RiskLevel: types.RiskLevelLow},
			},
			expectViols:  true,
			minViolCount: 1,
		},
		{
			name: "all CCPA types together",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeEmail, RiskLevel: types.RiskLevelMedium},
				{ID: "2", Type: types.PIITypeName, RiskLevel: types.RiskLevelMedium},
				{ID: "3", Type: types.PIITypeAddress, RiskLevel: types.RiskLevelMedium},
				{ID: "4", Type: types.PIITypeSSN, RiskLevel: types.RiskLevelCritical},
				{ID: "5", Type: types.PIITypeIPAddress, RiskLevel: types.RiskLevelLow},
			},
			expectViols:  true,
			minViolCount: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanResult := &types.ScanResult{Findings: tt.findings}
			rules := []types.ComplianceRule{
				{Regulation: "CCPA", Rule: "CCPA-001"},
			}

			result, err := checker.CheckCompliance(ctx, scanResult, rules)

			require.NoError(t, err)
			if tt.expectViols {
				assert.False(t, result.IsCompliant)
				assert.GreaterOrEqual(t, len(result.Violations), tt.minViolCount)

				// Verify all violations are for CCPA-001 with high severity
				for _, v := range result.Violations {
					assert.Equal(t, "CCPA-001", v.Rule)
					assert.Equal(t, "high", v.Severity)
				}
			} else {
				assert.True(t, result.IsCompliant)
			}
		})
	}
}

func TestCCPAChecker_NoRelevantFindings(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	// Credit card and phone number are not CCPA personal info types in current implementation
	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "1", Type: types.PIITypeCreditCard, RiskLevel: types.RiskLevelCritical},
			{ID: "2", Type: types.PIITypePhoneNumber, RiskLevel: types.RiskLevelMedium},
		},
	}

	rules := []types.ComplianceRule{
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	require.NoError(t, err)
	assert.True(t, result.IsCompliant)
	assert.Empty(t, result.Violations)
}

func TestCCPAChecker_RequiredActions(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "1", Type: types.PIITypeEmail, RiskLevel: types.RiskLevelMedium},
		},
	}

	rules := []types.ComplianceRule{
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	require.NoError(t, err)
	assert.NotEmpty(t, result.RequiredActions)

	// Check for expected CCPA actions
	actionStr := ""
	for _, action := range result.RequiredActions {
		actionStr += action + " "
	}
	assert.Contains(t, actionStr, "consumer rights")
}

func TestCCPAChecker_ViolationDescription(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "1", Type: types.PIITypeEmail, RiskLevel: types.RiskLevelMedium},
		},
	}

	rules := []types.ComplianceRule{
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	require.NoError(t, err)
	require.NotEmpty(t, result.Violations)

	// Check violation description mentions CCPA
	assert.Contains(t, result.Violations[0].Description, "CCPA")
}

func TestCCPAChecker_IPAddressIncluded(t *testing.T) {
	// CCPA uniquely includes IP addresses as personal information
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "1", Type: types.PIITypeIPAddress, RiskLevel: types.RiskLevelLow},
		},
	}

	rules := []types.ComplianceRule{
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	require.NoError(t, err)
	assert.False(t, result.IsCompliant)
	assert.Len(t, result.Violations, 1)
	assert.Equal(t, types.PIITypeIPAddress, result.Violations[0].PIIType)
}

func TestCCPAChecker_VersusGDPR(t *testing.T) {
	// Compare CCPA vs GDPR coverage
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	// IP address should trigger CCPA but not GDPR-001
	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "1", Type: types.PIITypeIPAddress, RiskLevel: types.RiskLevelLow},
		},
	}

	// Check CCPA
	ccpaRules := []types.ComplianceRule{
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}
	ccpaResult, err := checker.CheckCompliance(ctx, scanResult, ccpaRules)
	require.NoError(t, err)
	assert.False(t, ccpaResult.IsCompliant, "CCPA should flag IP address")

	// Check GDPR
	gdprRules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
	}
	gdprResult, err := checker.CheckCompliance(ctx, scanResult, gdprRules)
	require.NoError(t, err)
	assert.True(t, gdprResult.IsCompliant, "GDPR-001 should not flag IP address")
}
