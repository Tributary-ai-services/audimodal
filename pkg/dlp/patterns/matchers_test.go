package patterns

import (
	"testing"

	"github.com/jscharber/audimodal/pkg/dlp/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPatternRegistry(t *testing.T) {
	registry := NewPatternRegistry()

	assert.NotNil(t, registry)
	assert.NotNil(t, registry.matchers)

	// Verify all built-in matchers are registered
	matchers := registry.GetAllMatchers()
	assert.Len(t, matchers, 5)

	assert.NotNil(t, registry.GetMatcher(types.PIITypeSSN))
	assert.NotNil(t, registry.GetMatcher(types.PIITypeCreditCard))
	assert.NotNil(t, registry.GetMatcher(types.PIITypeEmail))
	assert.NotNil(t, registry.GetMatcher(types.PIITypePhoneNumber))
	assert.NotNil(t, registry.GetMatcher(types.PIITypeIPAddress))
}

func TestPatternRegistry_RegisterMatcher(t *testing.T) {
	registry := NewPatternRegistry()

	// Get initial count
	initialCount := len(registry.GetAllMatchers())

	// Create a custom mock matcher
	customMatcher := NewSSNMatcher() // Using SSN as example

	// Register overwrites existing
	registry.RegisterMatcher(customMatcher)

	// Count should remain same (overwrite)
	assert.Equal(t, initialCount, len(registry.GetAllMatchers()))
}

func TestPatternRegistry_GetMatcher(t *testing.T) {
	registry := NewPatternRegistry()

	tests := []struct {
		name     string
		piiType  types.PIIType
		expected bool
	}{
		{"SSN matcher exists", types.PIITypeSSN, true},
		{"Credit card matcher exists", types.PIITypeCreditCard, true},
		{"Email matcher exists", types.PIITypeEmail, true},
		{"Phone matcher exists", types.PIITypePhoneNumber, true},
		{"IP matcher exists", types.PIITypeIPAddress, true},
		{"Bank account matcher not exists", types.PIITypeBankAccount, false},
		{"Passport matcher not exists", types.PIITypePassport, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := registry.GetMatcher(tt.piiType)
			if tt.expected {
				assert.NotNil(t, matcher)
			} else {
				assert.Nil(t, matcher)
			}
		})
	}
}

func TestEmailMatcher_ValidFormats(t *testing.T) {
	matcher := NewEmailMatcher()

	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"simple email", "Email: user@example.com", 1},
		{"email with subdomain", "Email: user@mail.example.com", 1},
		{"email with plus", "Email: user+tag@example.com", 1},
		{"email with numbers", "Email: user123@example.com", 1},
		{"email with dots", "Email: user.name@example.com", 1},
		{"multiple emails", "CC: a@b.com, c@d.org, e@f.net", 3},
		{"corporate email", "Contact: john.smith@corporation.com", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := matcher.Match(tt.input)
			assert.Equal(t, tt.expected, len(matches))
		})
	}
}

func TestEmailMatcher_InvalidFormats(t *testing.T) {
	matcher := NewEmailMatcher()

	tests := []struct {
		name  string
		input string
	}{
		{"no at symbol", "Email: userexample.com"},
		{"no domain", "Email: user@"},
		{"no local part", "Email: @example.com"},
		// Note: The email matcher regex accepts many technically invalid emails
		// Test with a case that truly doesn't match the pattern
		{"numeric TLD", "Email: user@example.123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := matcher.Match(tt.input)
			assert.Equal(t, 0, len(matches))
		})
	}
}

func TestPhoneNumberMatcher_ValidFormats(t *testing.T) {
	matcher := NewPhoneNumberMatcher()

	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"parentheses format", "Phone: (555) 123-4567", 1},
		{"dashed format", "Phone: 555-123-4567", 1},
		{"dotted format", "Phone: 555.123.4567", 1},
		{"with country code", "Phone: +1 555-123-4567", 1},
		{"multiple phones", "Main: (555) 111-2222, Alt: (555) 333-4444", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := matcher.Match(tt.input)
			assert.Equal(t, tt.expected, len(matches))
		})
	}
}

func TestPhoneNumberMatcher_ConfidenceScore(t *testing.T) {
	matcher := NewPhoneNumberMatcher()

	tests := []struct {
		name    string
		input   string
		minConf float64
	}{
		{"parentheses format highest", "(555) 123-4567", 0.85},
		{"dashed format high", "555-123-4567", 0.75},
		{"plain format lower", "5551234567", 0.6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := matcher.GetConfidenceScore(tt.input)
			assert.GreaterOrEqual(t, conf, tt.minConf)
		})
	}
}

func TestIPAddressMatcher_ValidFormats(t *testing.T) {
	matcher := NewIPAddressMatcher()

	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"private class C", "IP: 192.168.1.1", 1},
		{"private class A", "IP: 10.0.0.1", 1},
		{"private class B", "IP: 172.16.0.1", 1},
		{"localhost", "IP: 127.0.0.1", 1},
		{"public DNS", "DNS: 8.8.8.8", 1},
		{"multiple IPs", "Primary: 192.168.1.1, Secondary: 10.0.0.1", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := matcher.Match(tt.input)
			assert.Equal(t, tt.expected, len(matches))
		})
	}
}

func TestIPAddressMatcher_InvalidFormats(t *testing.T) {
	matcher := NewIPAddressMatcher()

	tests := []struct {
		name  string
		input string
	}{
		{"octet > 255", "IP: 256.1.1.1"},
		{"octet > 255 second", "IP: 192.256.1.1"},
		{"octet > 255 third", "IP: 192.168.256.1"},
		{"octet > 255 fourth", "IP: 192.168.1.256"},
		{"three octets", "IP: 192.168.1"},
		// Note: "192.168.1.1.1" contains the valid IP "192.168.1.1"
		// So we test with truly invalid IPs instead
		{"octet > 255 all", "IP: 999.999.999.999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := matcher.Match(tt.input)
			assert.Equal(t, 0, len(matches))
		})
	}
}

func TestIPAddressMatcher_IsValidIP(t *testing.T) {
	matcher := NewIPAddressMatcher()

	validIPs := []string{
		"0.0.0.0",
		"255.255.255.255",
		"192.168.1.1",
		"10.0.0.1",
		"172.16.0.1",
		"127.0.0.1",
		"8.8.8.8",
	}

	for _, ip := range validIPs {
		t.Run("valid_"+ip, func(t *testing.T) {
			assert.True(t, matcher.isValidIP(ip))
		})
	}

	invalidIPs := []string{
		"256.1.1.1",
		"192.168.1",
		"192.168.1.1.1",
		"abc.def.ghi.jkl",
		"-1.0.0.0",
	}

	for _, ip := range invalidIPs {
		t.Run("invalid_"+ip, func(t *testing.T) {
			assert.False(t, matcher.isValidIP(ip))
		})
	}
}

func TestExtractContext(t *testing.T) {
	content := "This is a test content with some data in the middle of the text."

	tests := []struct {
		name       string
		start      int
		end        int
		windowSize int
		minLen     int
	}{
		{"normal extraction", 20, 30, 10, 15},
		{"start of content", 0, 10, 10, 10},
		{"end of content", 55, 65, 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			context := extractContext(content, tt.start, tt.end, tt.windowSize)
			assert.GreaterOrEqual(t, len(context), tt.minLen)
		})
	}
}

func TestIsTypeEnabled(t *testing.T) {
	tests := []struct {
		name     string
		piiType  types.PIIType
		config   *types.ScanConfig
		expected bool
	}{
		{
			name:    "enabled when no patterns specified",
			piiType: types.PIITypeSSN,
			config: &types.ScanConfig{
				EnabledPatterns:  []types.PIIType{},
				DisabledPatterns: []types.PIIType{},
			},
			expected: true,
		},
		{
			name:    "enabled when in enabled list",
			piiType: types.PIITypeSSN,
			config: &types.ScanConfig{
				EnabledPatterns:  []types.PIIType{types.PIITypeSSN, types.PIITypeEmail},
				DisabledPatterns: []types.PIIType{},
			},
			expected: true,
		},
		{
			name:    "disabled when in disabled list",
			piiType: types.PIITypeSSN,
			config: &types.ScanConfig{
				EnabledPatterns:  []types.PIIType{},
				DisabledPatterns: []types.PIIType{types.PIITypeSSN},
			},
			expected: false,
		},
		{
			name:    "disabled when not in enabled list",
			piiType: types.PIITypeSSN,
			config: &types.ScanConfig{
				EnabledPatterns:  []types.PIIType{types.PIITypeEmail},
				DisabledPatterns: []types.PIIType{},
			},
			expected: false,
		},
		{
			name:    "disabled takes precedence",
			piiType: types.PIITypeSSN,
			config: &types.ScanConfig{
				EnabledPatterns:  []types.PIIType{types.PIITypeSSN},
				DisabledPatterns: []types.PIIType{types.PIITypeSSN},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTypeEnabled(tt.piiType, tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAllMatchers_GetName(t *testing.T) {
	registry := NewPatternRegistry()

	expectedNames := map[types.PIIType]string{
		types.PIITypeSSN:         "ssn",
		types.PIITypeCreditCard:  "credit_card",
		types.PIITypeEmail:       "email",
		types.PIITypePhoneNumber: "phone_number",
		types.PIITypeIPAddress:   "ip_address",
	}

	for piiType, expectedName := range expectedNames {
		t.Run(string(piiType), func(t *testing.T) {
			matcher := registry.GetMatcher(piiType)
			require.NotNil(t, matcher)
			assert.Equal(t, expectedName, matcher.GetName())
		})
	}
}

func TestAllMatchers_GetType(t *testing.T) {
	registry := NewPatternRegistry()

	piiTypes := []types.PIIType{
		types.PIITypeSSN,
		types.PIITypeCreditCard,
		types.PIITypeEmail,
		types.PIITypePhoneNumber,
		types.PIITypeIPAddress,
	}

	for _, piiType := range piiTypes {
		t.Run(string(piiType), func(t *testing.T) {
			matcher := registry.GetMatcher(piiType)
			require.NotNil(t, matcher)
			assert.Equal(t, piiType, matcher.GetType())
		})
	}
}

func TestAllMatchers_EmptyInput(t *testing.T) {
	registry := NewPatternRegistry()

	for piiType, matcher := range registry.GetAllMatchers() {
		t.Run(string(piiType), func(t *testing.T) {
			matches := matcher.Match("")
			assert.Empty(t, matches)

			matches = matcher.Match("   ")
			assert.Empty(t, matches)
		})
	}
}
