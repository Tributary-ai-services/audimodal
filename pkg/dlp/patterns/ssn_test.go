package patterns

import (
	"testing"

	"github.com/jscharber/audimodal/pkg/dlp/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSNMatcher_ValidFormats(t *testing.T) {
	matcher := NewSSNMatcher()

	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"dashed format", "SSN: 123-45-6789", 1},
		{"space format", "SSN: 123 45 6789", 1},
		{"no separator", "SSN: 123456789", 1},
		{"multiple SSNs", "SSN1: 123-45-6789, SSN2: 234-56-7890", 2},
		{"mixed formats", "First: 123-45-6789, Second: 234 56 7890, Third: 345678901", 3},
		{"in sentence", "The applicant's SSN is 123-45-6789 and is valid.", 1},
		{"with context", "Employee SSN: 456-78-9012 (verified)", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := matcher.Match(tt.input)
			assert.Equal(t, tt.expected, len(matches), "Expected %d matches for input: %s", tt.expected, tt.input)
		})
	}
}

func TestSSNMatcher_InvalidFormats(t *testing.T) {
	matcher := NewSSNMatcher()

	tests := []struct {
		name  string
		input string
	}{
		{"starts with 000", "SSN: 000-12-3456"},
		{"starts with 666", "SSN: 666-12-3456"},
		{"starts with 9xx", "SSN: 912-34-5678"},
		{"starts with 900", "SSN: 900-12-3456"},
		{"middle 00", "SSN: 123-00-6789"},
		{"last 0000", "SSN: 123-45-0000"},
		{"too short 8 digits", "SSN: 12-34-5678"},
		{"too long 10 digits", "SSN: 1234-56-7890"},
		{"all zeros", "SSN: 000-00-0000"},
		{"all 9s", "SSN: 999-99-9999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := matcher.Match(tt.input)
			assert.Equal(t, 0, len(matches), "Should not match invalid SSN: %s", tt.input)
		})
	}
}

func TestSSNMatcher_ConfidenceScore(t *testing.T) {
	matcher := NewSSNMatcher()

	tests := []struct {
		name        string
		input       string
		minConf     float64
		maxConf     float64
	}{
		{"dashed format high confidence", "123-45-6789", 0.85, 1.0},
		{"space format high confidence", "123 45 6789", 0.85, 1.0},
		{"no separator lower confidence", "123456789", 0.6, 0.8},
		{"invalid format zero confidence", "000-12-3456", 0.0, 0.35},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := matcher.GetConfidenceScore(tt.input)
			assert.GreaterOrEqual(t, conf, tt.minConf, "Confidence should be >= %f", tt.minConf)
			assert.LessOrEqual(t, conf, tt.maxConf, "Confidence should be <= %f", tt.maxConf)
		})
	}
}

func TestSSNMatcher_MatchDetails(t *testing.T) {
	matcher := NewSSNMatcher()

	input := "Employee SSN: 123-45-6789 verified"
	matches := matcher.Match(input)

	require.Len(t, matches, 1)

	match := matches[0]
	assert.Equal(t, "123-45-6789", match.Value)
	assert.Greater(t, match.StartPos, 0)
	assert.Greater(t, match.EndPos, match.StartPos)
	assert.Greater(t, match.Confidence, 0.0)
	assert.NotEmpty(t, match.Context)
}

func TestSSNMatcher_GetName(t *testing.T) {
	matcher := NewSSNMatcher()
	assert.Equal(t, "ssn", matcher.GetName())
}

func TestSSNMatcher_GetType(t *testing.T) {
	matcher := NewSSNMatcher()
	assert.Equal(t, types.PIITypeSSN, matcher.GetType())
}

func TestSSNMatcher_IsEnabled(t *testing.T) {
	matcher := NewSSNMatcher()

	tests := []struct {
		name     string
		config   *types.ScanConfig
		expected bool
	}{
		{
			name: "enabled when no patterns specified",
			config: &types.ScanConfig{
				EnabledPatterns:  []types.PIIType{},
				DisabledPatterns: []types.PIIType{},
			},
			expected: true,
		},
		{
			name: "enabled when explicitly enabled",
			config: &types.ScanConfig{
				EnabledPatterns:  []types.PIIType{types.PIITypeSSN},
				DisabledPatterns: []types.PIIType{},
			},
			expected: true,
		},
		{
			name: "disabled when explicitly disabled",
			config: &types.ScanConfig{
				EnabledPatterns:  []types.PIIType{},
				DisabledPatterns: []types.PIIType{types.PIITypeSSN},
			},
			expected: false,
		},
		{
			name: "disabled when not in enabled list",
			config: &types.ScanConfig{
				EnabledPatterns:  []types.PIIType{types.PIITypeEmail},
				DisabledPatterns: []types.PIIType{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.IsEnabled(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSSNMatcher_IsValidSSN(t *testing.T) {
	matcher := NewSSNMatcher()

	validSSNs := []string{
		"123456789",
		"234567890",
		"345678901",
		"456789012",
		"567890123",
	}

	for _, ssn := range validSSNs {
		t.Run("valid_"+ssn, func(t *testing.T) {
			assert.True(t, matcher.isValidSSN(ssn), "SSN %s should be valid", ssn)
		})
	}

	invalidSSNs := []string{
		"000123456", // starts with 000
		"666123456", // starts with 666
		"900123456", // starts with 9
		"123004567", // middle 00
		"123450000", // last 0000
		"12345678",  // too short
		"1234567890", // too long
	}

	for _, ssn := range invalidSSNs {
		t.Run("invalid_"+ssn, func(t *testing.T) {
			assert.False(t, matcher.isValidSSN(ssn), "SSN %s should be invalid", ssn)
		})
	}
}

func TestSSNMatcher_EdgeCases(t *testing.T) {
	matcher := NewSSNMatcher()

	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty string", "", 0},
		{"only whitespace", "   ", 0},
		{"SSN at start", "123-45-6789 is the number", 1},
		{"SSN at end", "The number is 123-45-6789", 1},
		{"SSN with parentheses", "(123-45-6789)", 1},
		{"SSN with quotes", "\"123-45-6789\"", 1},
		{"phone number not SSN", "Call 555-123-4567", 0}, // 3-3-4 not 3-2-4
		{"date not SSN", "Date: 01-15-2024", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := matcher.Match(tt.input)
			assert.Equal(t, tt.expected, len(matches))
		})
	}
}
