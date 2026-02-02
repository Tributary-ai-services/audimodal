package performance

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/jscharber/audimodal/pkg/dlp/compliance"
	"github.com/jscharber/audimodal/pkg/dlp/patterns"
	"github.com/jscharber/audimodal/pkg/dlp/types"
	"github.com/stretchr/testify/assert"
)

// getMemoryUsage returns current memory usage in bytes
func getMemoryUsage() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}

// TestMemoryUsage_PatternRegistry tests memory usage of pattern registry
func TestMemoryUsage_PatternRegistry(t *testing.T) {
	runtime.GC() // Clean up before measurement
	before := getMemoryUsage()

	registry := patterns.NewPatternRegistry()
	_ = registry.GetAllMatchers()

	after := getMemoryUsage()
	memUsed := after - before

	// Registry should use less than 1MB
	assert.Less(t, memUsed, uint64(1*1024*1024),
		"Pattern registry should use less than 1MB, used: %d bytes", memUsed)
}

// TestMemoryUsage_SmallDocument tests memory usage for small document processing
func TestMemoryUsage_SmallDocument(t *testing.T) {
	registry := patterns.NewPatternRegistry()
	doc := generateDocument(1024, true)

	runtime.GC()
	before := getMemoryUsage()

	// Process document
	for _, matcher := range registry.GetAllMatchers() {
		matcher.Match(doc)
	}

	after := getMemoryUsage()
	memUsed := after - before

	// Processing small doc should use less than 5MB
	assert.Less(t, memUsed, uint64(5*1024*1024),
		"Small document processing should use less than 5MB, used: %d bytes", memUsed)
}

// TestMemoryUsage_LargeDocument tests memory usage for large document processing
func TestMemoryUsage_LargeDocument(t *testing.T) {
	registry := patterns.NewPatternRegistry()
	doc := generateDocument(1024*1024, true) // 1MB document

	runtime.GC()
	before := getMemoryUsage()

	// Process document
	for _, matcher := range registry.GetAllMatchers() {
		matcher.Match(doc)
	}

	after := getMemoryUsage()
	memUsed := after - before

	// Processing 1MB doc should use less than 50MB
	assert.Less(t, memUsed, uint64(50*1024*1024),
		"1MB document processing should use less than 50MB, used: %d bytes", memUsed)
}

// TestMemoryUsage_ManyFindings tests memory usage with many findings
func TestMemoryUsage_ManyFindings(t *testing.T) {
	// Generate document with many PII items
	doc := strings.Repeat(`
SSN: 123-45-6789 SSN: 234-56-7890 SSN: 345-67-8901
Email: a@b.com Email: c@d.com Email: e@f.com
Card: 4111111111111111 Card: 5500000000000004
`, 1000)

	registry := patterns.NewPatternRegistry()

	runtime.GC()
	before := getMemoryUsage()

	// Process and collect findings
	var findings []types.Finding
	findingID := 0
	for piiType, matcher := range registry.GetAllMatchers() {
		matches := matcher.Match(doc)
		for _, match := range matches {
			findingID++
			findings = append(findings, types.Finding{
				ID:         string(rune('0' + (findingID % 10))),
				Type:       piiType,
				Value:      match.Value,
				StartPos:   match.StartPos,
				EndPos:     match.EndPos,
				Confidence: match.Confidence,
				Context:    match.Context,
				RiskLevel:  types.RiskLevelMedium,
			})
		}
	}

	after := getMemoryUsage()
	memUsed := after - before

	// Should find many findings
	assert.Greater(t, len(findings), 1000, "Should find many PII instances")

	// Memory should be reasonable
	assert.Less(t, memUsed, uint64(100*1024*1024),
		"Many findings should use less than 100MB, used: %d bytes", memUsed)
}

// TestMemoryUsage_ComplianceChecker tests memory usage of compliance checking
func TestMemoryUsage_ComplianceChecker(t *testing.T) {
	ctx := context.Background()

	// Create large set of findings
	findings := make([]types.Finding, 1000)
	for i := 0; i < 1000; i++ {
		findings[i] = types.Finding{
			ID:        string(rune('A' + (i % 26))),
			Type:      types.PIITypeSSN,
			Value:     "123-45-6789",
			RiskLevel: types.RiskLevelCritical,
		}
	}

	scanResult := &types.ScanResult{Findings: findings}
	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "GDPR", Rule: "GDPR-002"},
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	runtime.GC()
	before := getMemoryUsage()

	checker := compliance.NewBasicComplianceChecker()
	result, _ := checker.CheckCompliance(ctx, scanResult, rules)

	after := getMemoryUsage()
	memUsed := after - before

	// Should have violations
	assert.NotEmpty(t, result.Violations)

	// Compliance checking should use less than 10MB
	assert.Less(t, memUsed, uint64(10*1024*1024),
		"Compliance checking should use less than 10MB, used: %d bytes", memUsed)
}

// TestMemoryUsage_EndToEnd tests memory usage of full pipeline
func TestMemoryUsage_EndToEnd(t *testing.T) {
	ctx := context.Background()
	doc := generateDocument(1024*1024, true) // 1MB document

	runtime.GC()
	before := getMemoryUsage()

	// Step 1: Pattern matching
	registry := patterns.NewPatternRegistry()
	var findings []types.Finding
	findingID := 0
	for piiType, matcher := range registry.GetAllMatchers() {
		matches := matcher.Match(doc)
		for _, match := range matches {
			findingID++
			findings = append(findings, types.Finding{
				ID:         string(rune('0' + (findingID % 10))),
				Type:       piiType,
				Value:      match.Value,
				StartPos:   match.StartPos,
				EndPos:     match.EndPos,
				Confidence: match.Confidence,
				Context:    match.Context,
				RiskLevel:  types.RiskLevelMedium,
			})
		}
	}

	// Step 2: Compliance checking
	checker := compliance.NewBasicComplianceChecker()
	scanResult := &types.ScanResult{Findings: findings}
	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "GDPR", Rule: "GDPR-002"},
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}
	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	after := getMemoryUsage()
	memUsed := after - before

	// Verify processing completed
	assert.NoError(t, err)
	assert.NotEmpty(t, findings)
	assert.NotEmpty(t, result.Violations)

	// Full pipeline for 1MB doc should use less than 100MB
	assert.Less(t, memUsed, uint64(100*1024*1024),
		"Full pipeline for 1MB doc should use less than 100MB, used: %d bytes", memUsed)
}

// TestMemoryUsage_BatchProcessing tests memory usage with batch processing
func TestMemoryUsage_BatchProcessing(t *testing.T) {
	registry := patterns.NewPatternRegistry()
	doc := generateDocument(10*1024, true) // 10KB document

	runtime.GC()
	before := getMemoryUsage()

	// Process 100 documents
	for i := 0; i < 100; i++ {
		for _, matcher := range registry.GetAllMatchers() {
			matcher.Match(doc)
		}
	}

	after := getMemoryUsage()
	memUsed := after - before

	// Batch processing should not accumulate excessive memory
	assert.Less(t, memUsed, uint64(50*1024*1024),
		"Batch processing 100 docs should use less than 50MB, used: %d bytes", memUsed)
}

// BenchmarkMemory_PatternMatching benchmarks memory allocations in pattern matching
func BenchmarkMemory_PatternMatching(b *testing.B) {
	registry := patterns.NewPatternRegistry()
	doc := generateDocument(10*1024, true)
	matchers := registry.GetAllMatchers()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, matcher := range matchers {
			matcher.Match(doc)
		}
	}
}

// BenchmarkMemory_ComplianceChecking benchmarks memory allocations in compliance checking
func BenchmarkMemory_ComplianceChecking(b *testing.B) {
	ctx := context.Background()
	checker := compliance.NewBasicComplianceChecker()

	findings := make([]types.Finding, 100)
	for i := 0; i < 100; i++ {
		findings[i] = types.Finding{
			ID:        string(rune('A' + (i % 26))),
			Type:      types.PIITypeSSN,
			Value:     "123-45-6789",
			RiskLevel: types.RiskLevelCritical,
		}
	}

	scanResult := &types.ScanResult{Findings: findings}
	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.CheckCompliance(ctx, scanResult, rules)
	}
}

// BenchmarkMemory_EndToEnd benchmarks memory allocations in full pipeline
func BenchmarkMemory_EndToEnd(b *testing.B) {
	ctx := context.Background()
	registry := patterns.NewPatternRegistry()
	checker := compliance.NewBasicComplianceChecker()
	doc := generateDocument(10*1024, true)
	matchers := registry.GetAllMatchers()
	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Scan
		var findings []types.Finding
		for piiType, matcher := range matchers {
			matches := matcher.Match(doc)
			for _, match := range matches {
				findings = append(findings, types.Finding{
					Type:       piiType,
					Value:      match.Value,
					Confidence: match.Confidence,
					RiskLevel:  types.RiskLevelMedium,
				})
			}
		}

		// Check compliance
		scanResult := &types.ScanResult{Findings: findings}
		checker.CheckCompliance(ctx, scanResult, rules)
	}
}
