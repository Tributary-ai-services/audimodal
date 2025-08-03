package patterns

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/jscharber/eAIIngest/pkg/dlp/types"
)

// PatternMatcher defines the interface for pattern matchers (avoiding import cycle)
type PatternMatcher interface {
	GetName() string
	GetType() types.PIIType
	Match(content string) []types.Match
	GetConfidenceScore(match string) float64
	IsEnabled(config *types.ScanConfig) bool
}

// PatternRegistry manages all available pattern matchers
type PatternRegistry struct {
	matchers map[types.PIIType]PatternMatcher
}

// NewPatternRegistry creates a new pattern registry with built-in matchers
func NewPatternRegistry() *PatternRegistry {
	registry := &PatternRegistry{
		matchers: make(map[types.PIIType]PatternMatcher),
	}

	// Register built-in matchers
	registry.RegisterMatcher(NewSSNMatcher())
	registry.RegisterMatcher(NewCreditCardMatcher())
	registry.RegisterMatcher(NewEmailMatcher())
	registry.RegisterMatcher(NewPhoneNumberMatcher())
	registry.RegisterMatcher(NewIPAddressMatcher())

	return registry
}

// RegisterMatcher registers a new pattern matcher
func (pr *PatternRegistry) RegisterMatcher(matcher PatternMatcher) {
	pr.matchers[matcher.GetType()] = matcher
}

// GetMatcher returns a matcher for the given PII type
func (pr *PatternRegistry) GetMatcher(piiType types.PIIType) PatternMatcher {
	return pr.matchers[piiType]
}

// GetAllMatchers returns all registered matchers
func (pr *PatternRegistry) GetAllMatchers() map[types.PIIType]PatternMatcher {
	return pr.matchers
}

// SSNMatcher detects Social Security Numbers
type SSNMatcher struct {
	name    string
	pattern *regexp.Regexp
}

func NewSSNMatcher() *SSNMatcher {
	// Matches patterns like: 123-45-6789, 123 45 6789, 123456789
	pattern := regexp.MustCompile(`\b(?:\d{3}[-\s]?\d{2}[-\s]?\d{4})\b`)
	return &SSNMatcher{
		name:    "ssn",
		pattern: pattern,
	}
}

func (m *SSNMatcher) GetName() string {
	return m.name
}

func (m *SSNMatcher) GetType() types.PIIType {
	return types.PIITypeSSN
}

func (m *SSNMatcher) Match(content string) []types.Match {
	var matches []types.Match

	found := m.pattern.FindAllStringSubmatch(content, -1)
	indices := m.pattern.FindAllStringIndex(content, -1)

	for i, match := range found {
		if len(match) > 0 {
			value := strings.ReplaceAll(strings.ReplaceAll(match[0], "-", ""), " ", "")

			// Basic validation: should be 9 digits
			if len(value) == 9 && m.isValidSSN(value) {
				start := indices[i][0]
				end := indices[i][1]
				context := extractContext(content, start, end, 20)

				matches = append(matches, types.Match{
					Value:      match[0],
					StartPos:   start,
					EndPos:     end,
					Context:    context,
					Confidence: m.GetConfidenceScore(match[0]),
				})
			}
		}
	}

	return matches
}

func (m *SSNMatcher) GetConfidenceScore(match string) float64 {
	// Remove formatting
	digits := strings.ReplaceAll(strings.ReplaceAll(match, "-", ""), " ", "")

	// Basic validation
	if len(digits) != 9 {
		return 0.0
	}

	if !m.isValidSSN(digits) {
		return 0.3
	}

	// Check formatting
	if strings.Contains(match, "-") || strings.Contains(match, " ") {
		return 0.9
	}

	return 0.7
}

func (m *SSNMatcher) IsEnabled(config *types.ScanConfig) bool {
	return isTypeEnabled(types.PIITypeSSN, config)
}

func (m *SSNMatcher) isValidSSN(ssn string) bool {
	// SSN cannot start with 9, 666, or 000
	if len(ssn) != 9 {
		return false
	}

	first3 := ssn[:3]
	if first3 == "000" || first3 == "666" || first3[0] == '9' {
		return false
	}

	// Middle 2 digits cannot be 00
	if ssn[3:5] == "00" {
		return false
	}

	// Last 4 digits cannot be 0000
	if ssn[5:] == "0000" {
		return false
	}

	return true
}

// CreditCardMatcher detects credit card numbers
type CreditCardMatcher struct {
	name    string
	pattern *regexp.Regexp
}

func NewCreditCardMatcher() *CreditCardMatcher {
	// Matches patterns like: 4111-1111-1111-1111, 4111 1111 1111 1111, 4111111111111111
	pattern := regexp.MustCompile(`\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|3[0-9]{13}|6(?:011|5[0-9]{2})[0-9]{12})(?:[-\s]?[0-9]{4})*\b`)
	return &CreditCardMatcher{
		name:    "credit_card",
		pattern: pattern,
	}
}

func (m *CreditCardMatcher) GetName() string {
	return m.name
}

func (m *CreditCardMatcher) GetType() types.PIIType {
	return types.PIITypeCreditCard
}

func (m *CreditCardMatcher) Match(content string) []types.Match {
	var matches []types.Match

	found := m.pattern.FindAllStringSubmatch(content, -1)
	indices := m.pattern.FindAllStringIndex(content, -1)

	for i, match := range found {
		if len(match) > 0 {
			// Remove formatting for validation
			digits := strings.ReplaceAll(strings.ReplaceAll(match[0], "-", ""), " ", "")

			if m.isValidCreditCard(digits) {
				start := indices[i][0]
				end := indices[i][1]
				context := extractContext(content, start, end, 20)

				matches = append(matches, types.Match{
					Value:      match[0],
					StartPos:   start,
					EndPos:     end,
					Context:    context,
					Confidence: m.GetConfidenceScore(match[0]),
				})
			}
		}
	}

	return matches
}

func (m *CreditCardMatcher) GetConfidenceScore(match string) float64 {
	digits := strings.ReplaceAll(strings.ReplaceAll(match, "-", ""), " ", "")

	if !m.isValidCreditCard(digits) {
		return 0.0
	}

	// Higher confidence for formatted numbers
	if strings.Contains(match, "-") || strings.Contains(match, " ") {
		return 0.9
	}

	return 0.8
}

func (m *CreditCardMatcher) IsEnabled(config *types.ScanConfig) bool {
	return isTypeEnabled(types.PIITypeCreditCard, config)
}

func (m *CreditCardMatcher) isValidCreditCard(number string) bool {
	// Luhn algorithm validation
	sum := 0
	alternate := false

	for i := len(number) - 1; i >= 0; i-- {
		digit, err := strconv.Atoi(string(number[i]))
		if err != nil {
			return false
		}

		if alternate {
			digit *= 2
			if digit > 9 {
				digit = (digit % 10) + 1
			}
		}

		sum += digit
		alternate = !alternate
	}

	return sum%10 == 0
}

// EmailMatcher detects email addresses
type EmailMatcher struct {
	name    string
	pattern *regexp.Regexp
}

func NewEmailMatcher() *EmailMatcher {
	pattern := regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`)
	return &EmailMatcher{
		name:    "email",
		pattern: pattern,
	}
}

func (m *EmailMatcher) GetName() string {
	return m.name
}

func (m *EmailMatcher) GetType() types.PIIType {
	return types.PIITypeEmail
}

func (m *EmailMatcher) Match(content string) []types.Match {
	var matches []types.Match

	found := m.pattern.FindAllStringSubmatch(content, -1)
	indices := m.pattern.FindAllStringIndex(content, -1)

	for i, match := range found {
		if len(match) > 0 {
			start := indices[i][0]
			end := indices[i][1]
			context := extractContext(content, start, end, 20)

			matches = append(matches, types.Match{
				Value:      match[0],
				StartPos:   start,
				EndPos:     end,
				Context:    context,
				Confidence: m.GetConfidenceScore(match[0]),
			})
		}
	}

	return matches
}

func (m *EmailMatcher) GetConfidenceScore(match string) float64 {
	// Basic email validation
	if !strings.Contains(match, "@") || !strings.Contains(match, ".") {
		return 0.0
	}

	parts := strings.Split(match, "@")
	if len(parts) != 2 {
		return 0.0
	}

	return 0.9
}

func (m *EmailMatcher) IsEnabled(config *types.ScanConfig) bool {
	return isTypeEnabled(types.PIITypeEmail, config)
}

// PhoneNumberMatcher detects phone numbers
type PhoneNumberMatcher struct {
	name    string
	pattern *regexp.Regexp
}

func NewPhoneNumberMatcher() *PhoneNumberMatcher {
	// Matches US phone numbers in various formats
	pattern := regexp.MustCompile(`\b(?:\+?1[-.\s]?)?\(?([0-9]{3})\)?[-.\s]?([0-9]{3})[-.\s]?([0-9]{4})\b`)
	return &PhoneNumberMatcher{
		name:    "phone_number",
		pattern: pattern,
	}
}

func (m *PhoneNumberMatcher) GetName() string {
	return m.name
}

func (m *PhoneNumberMatcher) GetType() types.PIIType {
	return types.PIITypePhoneNumber
}

func (m *PhoneNumberMatcher) Match(content string) []types.Match {
	var matches []types.Match

	found := m.pattern.FindAllStringSubmatch(content, -1)
	indices := m.pattern.FindAllStringIndex(content, -1)

	for i, match := range found {
		if len(match) > 0 {
			start := indices[i][0]
			end := indices[i][1]
			context := extractContext(content, start, end, 20)

			matches = append(matches, types.Match{
				Value:      match[0],
				StartPos:   start,
				EndPos:     end,
				Context:    context,
				Confidence: m.GetConfidenceScore(match[0]),
			})
		}
	}

	return matches
}

func (m *PhoneNumberMatcher) GetConfidenceScore(match string) float64 {
	// Higher confidence for well-formatted numbers
	if strings.Contains(match, "(") && strings.Contains(match, ")") {
		return 0.9
	}
	if strings.Contains(match, "-") || strings.Contains(match, ".") {
		return 0.8
	}
	return 0.7
}

func (m *PhoneNumberMatcher) IsEnabled(config *types.ScanConfig) bool {
	return isTypeEnabled(types.PIITypePhoneNumber, config)
}

// IPAddressMatcher detects IP addresses
type IPAddressMatcher struct {
	name    string
	pattern *regexp.Regexp
}

func NewIPAddressMatcher() *IPAddressMatcher {
	pattern := regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`)
	return &IPAddressMatcher{
		name:    "ip_address",
		pattern: pattern,
	}
}

func (m *IPAddressMatcher) GetName() string {
	return m.name
}

func (m *IPAddressMatcher) GetType() types.PIIType {
	return types.PIITypeIPAddress
}

func (m *IPAddressMatcher) Match(content string) []types.Match {
	var matches []types.Match

	found := m.pattern.FindAllStringSubmatch(content, -1)
	indices := m.pattern.FindAllStringIndex(content, -1)

	for i, match := range found {
		if len(match) > 0 && m.isValidIP(match[0]) {
			start := indices[i][0]
			end := indices[i][1]
			context := extractContext(content, start, end, 20)

			matches = append(matches, types.Match{
				Value:      match[0],
				StartPos:   start,
				EndPos:     end,
				Context:    context,
				Confidence: m.GetConfidenceScore(match[0]),
			})
		}
	}

	return matches
}

func (m *IPAddressMatcher) GetConfidenceScore(match string) float64 {
	if !m.isValidIP(match) {
		return 0.0
	}
	return 0.8
}

func (m *IPAddressMatcher) IsEnabled(config *types.ScanConfig) bool {
	return isTypeEnabled(types.PIITypeIPAddress, config)
}

func (m *IPAddressMatcher) isValidIP(ip string) bool {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return false
	}

	for _, part := range parts {
		num, err := strconv.Atoi(part)
		if err != nil || num < 0 || num > 255 {
			return false
		}
	}

	return true
}

// Helper functions

func extractContext(content string, start, end, windowSize int) string {
	contextStart := start - windowSize
	if contextStart < 0 {
		contextStart = 0
	}

	contextEnd := end + windowSize
	if contextEnd > len(content) {
		contextEnd = len(content)
	}

	return content[contextStart:contextEnd]
}

func isTypeEnabled(piiType types.PIIType, config *types.ScanConfig) bool {
	// Check if explicitly disabled
	for _, disabled := range config.DisabledPatterns {
		if disabled == piiType {
			return false
		}
	}

	// If enabled patterns is empty, all are enabled by default
	if len(config.EnabledPatterns) == 0 {
		return true
	}

	// Check if explicitly enabled
	for _, enabled := range config.EnabledPatterns {
		if enabled == piiType {
			return true
		}
	}

	return false
}
