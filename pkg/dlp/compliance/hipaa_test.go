package compliance

import (
	"context"
	"testing"

	"github.com/jscharber/audimodal/pkg/dlp/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHIPAAChecker_PHI_WithFindings(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	tests := []struct {
		name         string
		findings     []types.Finding
		expectViols  bool
		minViolCount int
	}{
		{
			name: "SSN triggers HIPAA-001",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeSSN, RiskLevel: types.RiskLevelCritical},
			},
			expectViols:  true,
			minViolCount: 1,
		},
		{
			name: "DOB triggers HIPAA-001",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeDateOfBirth, RiskLevel: types.RiskLevelHigh},
			},
			expectViols:  true,
			minViolCount: 1,
		},
		{
			name: "Name triggers HIPAA-001",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeName, RiskLevel: types.RiskLevelMedium},
			},
			expectViols:  true,
			minViolCount: 1,
		},
		{
			name: "Email triggers HIPAA-001",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeEmail, RiskLevel: types.RiskLevelMedium},
			},
			expectViols:  true,
			minViolCount: 1,
		},
		{
			name: "multiple PHI types",
			findings: []types.Finding{
				{ID: "1", Type: types.PIITypeSSN, RiskLevel: types.RiskLevelCritical},
				{ID: "2", Type: types.PIITypeDateOfBirth, RiskLevel: types.RiskLevelHigh},
				{ID: "3", Type: types.PIITypeName, RiskLevel: types.RiskLevelMedium},
				{ID: "4", Type: types.PIITypeEmail, RiskLevel: types.RiskLevelMedium},
			},
			expectViols:  true,
			minViolCount: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanResult := &types.ScanResult{Findings: tt.findings}
			rules := []types.ComplianceRule{
				{Regulation: "HIPAA", Rule: "HIPAA-001"},
			}

			result, err := checker.CheckCompliance(ctx, scanResult, rules)

			require.NoError(t, err)
			if tt.expectViols {
				assert.False(t, result.IsCompliant)
				assert.GreaterOrEqual(t, len(result.Violations), tt.minViolCount)

				// Verify all violations are for HIPAA-001 with critical severity
				for _, v := range result.Violations {
					assert.Equal(t, "HIPAA-001", v.Rule)
					assert.Equal(t, "critical", v.Severity)
				}
			} else {
				assert.True(t, result.IsCompliant)
			}
		})
	}
}

func TestHIPAAChecker_NoRelevantFindings(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	// Credit card and IP address are not HIPAA PHI types
	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "1", Type: types.PIITypeCreditCard, RiskLevel: types.RiskLevelCritical},
			{ID: "2", Type: types.PIITypeIPAddress, RiskLevel: types.RiskLevelLow},
		},
	}

	rules := []types.ComplianceRule{
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	require.NoError(t, err)
	assert.True(t, result.IsCompliant)
	assert.Empty(t, result.Violations)
}

func TestHIPAAChecker_RequiredActions(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "1", Type: types.PIITypeSSN, RiskLevel: types.RiskLevelCritical},
		},
	}

	rules := []types.ComplianceRule{
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	require.NoError(t, err)
	assert.NotEmpty(t, result.RequiredActions)

	// Check for expected HIPAA actions
	actionStr := ""
	for _, action := range result.RequiredActions {
		actionStr += action + " "
	}
	assert.Contains(t, actionStr, "HIPAA")
}

func TestHIPAAChecker_ViolationDescription(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "1", Type: types.PIITypeSSN, RiskLevel: types.RiskLevelCritical},
		},
	}

	rules := []types.ComplianceRule{
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	require.NoError(t, err)
	require.NotEmpty(t, result.Violations)

	// Check violation description mentions PHI
	assert.Contains(t, result.Violations[0].Description, "Protected Health Information")
}

func TestHIPAAChecker_FindingIDsTracked(t *testing.T) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()

	scanResult := &types.ScanResult{
		Findings: []types.Finding{
			{ID: "finding-001", Type: types.PIITypeSSN, RiskLevel: types.RiskLevelCritical},
			{ID: "finding-002", Type: types.PIITypeName, RiskLevel: types.RiskLevelMedium},
		},
	}

	rules := []types.ComplianceRule{
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	require.NoError(t, err)
	require.NotEmpty(t, result.Violations)

	// Verify finding IDs are tracked in violations
	foundFinding001 := false
	foundFinding002 := false
	for _, v := range result.Violations {
		for _, fid := range v.FindingIDs {
			if fid == "finding-001" {
				foundFinding001 = true
			}
			if fid == "finding-002" {
				foundFinding002 = true
			}
		}
	}

	assert.True(t, foundFinding001, "Expected to find finding-001 in violations")
	assert.True(t, foundFinding002, "Expected to find finding-002 in violations")
}
