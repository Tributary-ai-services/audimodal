package compliance

import (
	"context"
	"testing"

	"github.com/jscharber/audimodal/pkg/dlp/types"
)

// Sample findings for benchmarking
var (
	singleFinding = []types.Finding{
		{ID: "1", Type: types.PIITypeSSN, Value: "123-45-6789", RiskLevel: types.RiskLevelCritical},
	}

	smallFindings = []types.Finding{
		{ID: "1", Type: types.PIITypeSSN, Value: "123-45-6789", RiskLevel: types.RiskLevelCritical},
		{ID: "2", Type: types.PIITypeEmail, Value: "test@example.com", RiskLevel: types.RiskLevelLow},
		{ID: "3", Type: types.PIITypeCreditCard, Value: "4111111111111111", RiskLevel: types.RiskLevelCritical},
	}

	mediumFindings = generateFindings(50)
	largeFindings  = generateFindings(500)
)

// generateFindings creates a slice of test findings
func generateFindings(count int) []types.Finding {
	findings := make([]types.Finding, count)
	piiTypes := []types.PIIType{
		types.PIITypeSSN,
		types.PIITypeCreditCard,
		types.PIITypeEmail,
		types.PIITypePhoneNumber,
		types.PIITypeIPAddress,
	}
	riskLevels := []types.RiskLevel{
		types.RiskLevelCritical,
		types.RiskLevelHigh,
		types.RiskLevelMedium,
		types.RiskLevelLow,
	}

	for i := 0; i < count; i++ {
		findings[i] = types.Finding{
			ID:        string(rune('A' + (i % 26))),
			Type:      piiTypes[i%len(piiTypes)],
			Value:     "test-value",
			RiskLevel: riskLevels[i%len(riskLevels)],
		}
	}
	return findings
}

// BenchmarkNewBasicComplianceChecker benchmarks checker creation
func BenchmarkNewBasicComplianceChecker(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewBasicComplianceChecker()
	}
}

// BenchmarkComplianceChecker_GetSupportedRegulations benchmarks getting regulations
func BenchmarkComplianceChecker_GetSupportedRegulations(b *testing.B) {
	checker := NewBasicComplianceChecker()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.GetSupportedRegulations()
	}
}

// BenchmarkComplianceChecker_GDPR_SingleFinding benchmarks GDPR checking with single finding
func BenchmarkComplianceChecker_GDPR_SingleFinding(b *testing.B) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()
	scanResult := &types.ScanResult{Findings: singleFinding}
	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "GDPR", Rule: "GDPR-002"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.CheckCompliance(ctx, scanResult, rules)
	}
}

// BenchmarkComplianceChecker_GDPR_SmallFindings benchmarks GDPR checking with small findings
func BenchmarkComplianceChecker_GDPR_SmallFindings(b *testing.B) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()
	scanResult := &types.ScanResult{Findings: smallFindings}
	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "GDPR", Rule: "GDPR-002"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.CheckCompliance(ctx, scanResult, rules)
	}
}

// BenchmarkComplianceChecker_GDPR_MediumFindings benchmarks GDPR checking with medium findings
func BenchmarkComplianceChecker_GDPR_MediumFindings(b *testing.B) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()
	scanResult := &types.ScanResult{Findings: mediumFindings}
	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "GDPR", Rule: "GDPR-002"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.CheckCompliance(ctx, scanResult, rules)
	}
}

// BenchmarkComplianceChecker_GDPR_LargeFindings benchmarks GDPR checking with large findings
func BenchmarkComplianceChecker_GDPR_LargeFindings(b *testing.B) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()
	scanResult := &types.ScanResult{Findings: largeFindings}
	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "GDPR", Rule: "GDPR-002"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.CheckCompliance(ctx, scanResult, rules)
	}
}

// BenchmarkComplianceChecker_HIPAA benchmarks HIPAA checking
func BenchmarkComplianceChecker_HIPAA(b *testing.B) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()
	scanResult := &types.ScanResult{Findings: smallFindings}
	rules := []types.ComplianceRule{
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.CheckCompliance(ctx, scanResult, rules)
	}
}

// BenchmarkComplianceChecker_PCI benchmarks PCI-DSS checking
func BenchmarkComplianceChecker_PCI(b *testing.B) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()
	scanResult := &types.ScanResult{Findings: smallFindings}
	rules := []types.ComplianceRule{
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.CheckCompliance(ctx, scanResult, rules)
	}
}

// BenchmarkComplianceChecker_CCPA benchmarks CCPA checking
func BenchmarkComplianceChecker_CCPA(b *testing.B) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()
	scanResult := &types.ScanResult{Findings: smallFindings}
	rules := []types.ComplianceRule{
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.CheckCompliance(ctx, scanResult, rules)
	}
}

// BenchmarkComplianceChecker_AllRegulations benchmarks checking all regulations
func BenchmarkComplianceChecker_AllRegulations(b *testing.B) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()
	scanResult := &types.ScanResult{Findings: smallFindings}
	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "GDPR", Rule: "GDPR-002"},
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.CheckCompliance(ctx, scanResult, rules)
	}
}

// BenchmarkComplianceChecker_AllRegulations_LargeFindings benchmarks all regulations with large findings
func BenchmarkComplianceChecker_AllRegulations_LargeFindings(b *testing.B) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()
	scanResult := &types.ScanResult{Findings: largeFindings}
	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "GDPR", Rule: "GDPR-002"},
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.CheckCompliance(ctx, scanResult, rules)
	}
}

// BenchmarkComplianceChecker_NoRules benchmarks checking with no rules
func BenchmarkComplianceChecker_NoRules(b *testing.B) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()
	scanResult := &types.ScanResult{Findings: smallFindings}
	rules := []types.ComplianceRule{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.CheckCompliance(ctx, scanResult, rules)
	}
}

// BenchmarkComplianceChecker_NoFindings benchmarks checking with no findings
func BenchmarkComplianceChecker_NoFindings(b *testing.B) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()
	scanResult := &types.ScanResult{Findings: []types.Finding{}}
	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.CheckCompliance(ctx, scanResult, rules)
	}
}

// BenchmarkComplianceChecker_FindingSizes benchmarks with different finding counts
func BenchmarkComplianceChecker_FindingSizes(b *testing.B) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()
	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "GDPR", Rule: "GDPR-002"},
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	sizes := map[string]int{
		"1":    1,
		"10":   10,
		"50":   50,
		"100":  100,
		"500":  500,
		"1000": 1000,
	}

	for name, size := range sizes {
		findings := generateFindings(size)
		scanResult := &types.ScanResult{Findings: findings}

		b.Run(name+"_findings", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				checker.CheckCompliance(ctx, scanResult, rules)
			}
		})
	}
}

// BenchmarkComplianceChecker_RegulationCount benchmarks with different rule counts
func BenchmarkComplianceChecker_RegulationCount(b *testing.B) {
	checker := NewBasicComplianceChecker()
	ctx := context.Background()
	scanResult := &types.ScanResult{Findings: smallFindings}

	ruleSets := map[string][]types.ComplianceRule{
		"1_rule": {
			{Regulation: "GDPR", Rule: "GDPR-001"},
		},
		"2_rules": {
			{Regulation: "GDPR", Rule: "GDPR-001"},
			{Regulation: "HIPAA", Rule: "HIPAA-001"},
		},
		"3_rules": {
			{Regulation: "GDPR", Rule: "GDPR-001"},
			{Regulation: "HIPAA", Rule: "HIPAA-001"},
			{Regulation: "PCI-DSS", Rule: "PCI-001"},
		},
		"5_rules": {
			{Regulation: "GDPR", Rule: "GDPR-001"},
			{Regulation: "GDPR", Rule: "GDPR-002"},
			{Regulation: "HIPAA", Rule: "HIPAA-001"},
			{Regulation: "PCI-DSS", Rule: "PCI-001"},
			{Regulation: "CCPA", Rule: "CCPA-001"},
		},
	}

	for name, rules := range ruleSets {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				checker.CheckCompliance(ctx, scanResult, rules)
			}
		})
	}
}
