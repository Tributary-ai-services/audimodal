package performance

import (
	"context"
	"strings"
	"testing"

	"github.com/jscharber/audimodal/pkg/dlp/compliance"
	"github.com/jscharber/audimodal/pkg/dlp/patterns"
	"github.com/jscharber/audimodal/pkg/dlp/types"
	"github.com/stretchr/testify/assert"
)

// Document generators for performance testing

// generateDocument creates a document of approximately the specified size in bytes
func generateDocument(sizeBytes int, withPII bool) string {
	// Base text with optional PII
	var baseText string
	if withPII {
		baseText = `
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Customer SSN: 123-45-6789
Email: test@example.com
Phone: (555) 123-4567
Credit Card: 4111111111111111
IP: 192.168.1.100
Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.
`
	} else {
		baseText = `
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.
Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris.
Duis aute irure dolor in reprehenderit in voluptate velit esse cillum.
Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia.
`
	}

	// Calculate repetitions needed
	repetitions := sizeBytes / len(baseText)
	if repetitions < 1 {
		repetitions = 1
	}

	return strings.Repeat(baseText, repetitions)
}

// TestPerformance_SmallDocument tests processing time for small documents (<1KB)
func TestPerformance_SmallDocument(t *testing.T) {
	registry := patterns.NewPatternRegistry()
	doc := generateDocument(1024, true) // ~1KB

	// Run pattern matching
	var totalMatches int
	for _, matcher := range registry.GetAllMatchers() {
		matches := matcher.Match(doc)
		totalMatches += len(matches)
	}

	// Should find PII
	assert.Greater(t, totalMatches, 0, "Should find PII in small document")
}

// TestPerformance_MediumDocument tests processing time for medium documents (~100KB)
func TestPerformance_MediumDocument(t *testing.T) {
	registry := patterns.NewPatternRegistry()
	doc := generateDocument(100*1024, true) // ~100KB

	// Run pattern matching
	var totalMatches int
	for _, matcher := range registry.GetAllMatchers() {
		matches := matcher.Match(doc)
		totalMatches += len(matches)
	}

	// Should find multiple PII instances
	assert.Greater(t, totalMatches, 10, "Should find multiple PII in medium document")
}

// TestPerformance_LargeDocument tests processing time for large documents (~1MB)
func TestPerformance_LargeDocument(t *testing.T) {
	registry := patterns.NewPatternRegistry()
	doc := generateDocument(1024*1024, true) // ~1MB

	// Run pattern matching
	var totalMatches int
	for _, matcher := range registry.GetAllMatchers() {
		matches := matcher.Match(doc)
		totalMatches += len(matches)
	}

	// Should find many PII instances
	assert.Greater(t, totalMatches, 100, "Should find many PII in large document")
}

// TestPerformance_VeryLargeDocument tests processing time for very large documents (~10MB)
func TestPerformance_VeryLargeDocument(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping very large document test in short mode")
	}

	registry := patterns.NewPatternRegistry()
	doc := generateDocument(10*1024*1024, true) // ~10MB

	// Run pattern matching
	var totalMatches int
	for _, matcher := range registry.GetAllMatchers() {
		matches := matcher.Match(doc)
		totalMatches += len(matches)
	}

	// Should find many PII instances
	assert.Greater(t, totalMatches, 1000, "Should find many PII in very large document")
}

// TestPerformance_CleanDocument tests processing clean documents (no PII)
func TestPerformance_CleanDocument(t *testing.T) {
	registry := patterns.NewPatternRegistry()
	doc := generateDocument(100*1024, false) // ~100KB clean

	// Run pattern matching
	var totalMatches int
	for _, matcher := range registry.GetAllMatchers() {
		matches := matcher.Match(doc)
		totalMatches += len(matches)
	}

	// Should find no PII
	assert.Equal(t, 0, totalMatches, "Should find no PII in clean document")
}

// TestPerformance_HighDensityPII tests documents with very high PII density
func TestPerformance_HighDensityPII(t *testing.T) {
	// Create high density PII document
	highDensity := strings.Repeat(`
SSN: 123-45-6789 SSN: 234-56-7890 SSN: 345-67-8901
Email: a@b.com Email: c@d.com Email: e@f.com
Card: 4111111111111111 Card: 5500000000000004
IP: 192.168.1.1 IP: 10.0.0.1 IP: 8.8.8.8
Phone: (555) 111-1111 Phone: (555) 222-2222
`, 100)

	registry := patterns.NewPatternRegistry()

	// Count matches
	var totalMatches int
	for _, matcher := range registry.GetAllMatchers() {
		matches := matcher.Match(highDensity)
		totalMatches += len(matches)
	}

	// Should find many PII instances
	assert.Greater(t, totalMatches, 500, "Should find many PII in high density document")
}

// TestPerformance_EndToEnd tests full pipeline: scanning + compliance checking
func TestPerformance_EndToEnd(t *testing.T) {
	ctx := context.Background()
	registry := patterns.NewPatternRegistry()
	checker := compliance.NewBasicComplianceChecker()
	doc := generateDocument(100*1024, true) // ~100KB

	// Step 1: Pattern matching
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
	scanResult := &types.ScanResult{Findings: findings}
	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "GDPR", Rule: "GDPR-002"},
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	result, err := checker.CheckCompliance(ctx, scanResult, rules)

	// Verify results
	assert.NoError(t, err)
	assert.False(t, result.IsCompliant)
	assert.NotEmpty(t, result.Violations)
}

// BenchmarkPerformance_SmallDocument benchmarks small document processing
func BenchmarkPerformance_SmallDocument(b *testing.B) {
	registry := patterns.NewPatternRegistry()
	doc := generateDocument(1024, true)
	matchers := registry.GetAllMatchers()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, matcher := range matchers {
			matcher.Match(doc)
		}
	}
}

// BenchmarkPerformance_MediumDocument benchmarks medium document processing
func BenchmarkPerformance_MediumDocument(b *testing.B) {
	registry := patterns.NewPatternRegistry()
	doc := generateDocument(100*1024, true)
	matchers := registry.GetAllMatchers()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, matcher := range matchers {
			matcher.Match(doc)
		}
	}
}

// BenchmarkPerformance_LargeDocument benchmarks large document processing
func BenchmarkPerformance_LargeDocument(b *testing.B) {
	registry := patterns.NewPatternRegistry()
	doc := generateDocument(1024*1024, true)
	matchers := registry.GetAllMatchers()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, matcher := range matchers {
			matcher.Match(doc)
		}
	}
}

// BenchmarkPerformance_EndToEnd benchmarks full pipeline
func BenchmarkPerformance_EndToEnd(b *testing.B) {
	ctx := context.Background()
	registry := patterns.NewPatternRegistry()
	checker := compliance.NewBasicComplianceChecker()
	doc := generateDocument(100*1024, true)
	matchers := registry.GetAllMatchers()
	rules := []types.ComplianceRule{
		{Regulation: "GDPR", Rule: "GDPR-001"},
		{Regulation: "GDPR", Rule: "GDPR-002"},
		{Regulation: "HIPAA", Rule: "HIPAA-001"},
		{Regulation: "PCI-DSS", Rule: "PCI-001"},
		{Regulation: "CCPA", Rule: "CCPA-001"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Scan
		var findings []types.Finding
		findingID := 0
		for piiType, matcher := range matchers {
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
					RiskLevel:  types.RiskLevelMedium,
				})
			}
		}

		// Check compliance
		scanResult := &types.ScanResult{Findings: findings}
		checker.CheckCompliance(ctx, scanResult, rules)
	}
}
