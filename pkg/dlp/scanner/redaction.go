package scanner

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/jscharber/eAIIngest/pkg/dlp/types"
)

// BasicRedactionEngine implements basic redaction/masking functionality
type BasicRedactionEngine struct {
	name    string
	version string
}

// NewBasicRedactionEngine creates a new basic redaction engine
func NewBasicRedactionEngine() *BasicRedactionEngine {
	return &BasicRedactionEngine{
		name:    "basic_redaction_engine",
		version: "1.0.0",
	}
}

// RedactContent redacts PII from content based on scan results
func (r *BasicRedactionEngine) RedactContent(content string, scanResult *types.ScanResult, strategy types.RedactionStrategy) (string, error) {
	if strategy == types.RedactionNone {
		return content, nil
	}
	
	// Sort findings by position (descending) to avoid position shifts during redaction
	findings := make([]types.Finding, len(scanResult.Findings))
	copy(findings, scanResult.Findings)
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].StartPos > findings[j].StartPos
	})
	
	redactedContent := content
	
	for _, finding := range findings {
		// Skip if already redacted
		if finding.Redacted {
			continue
		}
		
		// Apply redaction strategy
		redactedValue, err := r.applyRedactionStrategy(finding.Value, finding.Type, strategy)
		if err != nil {
			continue // Skip problematic redactions
		}
		
		// Replace in content
		before := redactedContent[:finding.StartPos]
		after := redactedContent[finding.EndPos:]
		redactedContent = before + redactedValue + after
	}
	
	return redactedContent, nil
}

// GenerateRedactionMap creates a mapping of original to redacted values
func (r *BasicRedactionEngine) GenerateRedactionMap(scanResult *types.ScanResult, strategy types.RedactionStrategy) map[string]string {
	redactionMap := make(map[string]string)
	
	for _, finding := range scanResult.Findings {
		if finding.Redacted {
			continue
		}
		
		redactedValue, err := r.applyRedactionStrategy(finding.Value, finding.Type, strategy)
		if err != nil {
			continue
		}
		
		redactionMap[finding.Value] = redactedValue
	}
	
	return redactionMap
}

// applyRedactionStrategy applies the specified redaction strategy to a value
func (r *BasicRedactionEngine) applyRedactionStrategy(value string, piiType types.PIIType, strategy types.RedactionStrategy) (string, error) {
	switch strategy {
	case types.RedactionNone:
		return value, nil
		
	case types.RedactionMask:
		return r.maskValue(value, piiType), nil
		
	case types.RedactionReplace:
		return r.replaceValue(value, piiType), nil
		
	case types.RedactionHash:
		return r.hashValue(value), nil
		
	case types.RedactionRemove:
		return "", nil
		
	case types.RedactionTokenize:
		return r.tokenizeValue(value, piiType), nil
		
	default:
		return "", fmt.Errorf("unsupported redaction strategy: %s", strategy)
	}
}

// maskValue masks a value with asterisks, preserving some structure
func (r *BasicRedactionEngine) maskValue(value string, piiType types.PIIType) string {
	switch piiType {
	case types.PIITypeSSN:
		return r.maskSSN(value)
	case types.PIITypeCreditCard:
		return r.maskCreditCard(value)
	case types.PIITypeEmail:
		return r.maskEmail(value)
	case types.PIITypePhoneNumber:
		return r.maskPhoneNumber(value)
	case types.PIITypeIPAddress:
		return r.maskIPAddress(value)
	default:
		return r.maskGeneric(value)
	}
}

// replaceValue replaces a value with a generic placeholder
func (r *BasicRedactionEngine) replaceValue(value string, piiType types.PIIType) string {
	switch piiType {
	case types.PIITypeSSN:
		return "[SSN]"
	case types.PIITypeCreditCard:
		return "[CREDIT_CARD]"
	case types.PIITypeEmail:
		return "[EMAIL]"
	case types.PIITypePhoneNumber:
		return "[PHONE]"
	case types.PIITypeIPAddress:
		return "[IP_ADDRESS]"
	case types.PIITypeName:
		return "[NAME]"
	case types.PIITypeAddress:
		return "[ADDRESS]"
	case types.PIITypeDateOfBirth:
		return "[DATE_OF_BIRTH]"
	case types.PIITypePassport:
		return "[PASSPORT]"
	case types.PIITypeDriversLicense:
		return "[DRIVERS_LICENSE]"
	case types.PIITypeBankAccount:
		return "[BANK_ACCOUNT]"
	default:
		return "[PII]"
	}
}

// hashValue creates a SHA-256 hash of the value
func (r *BasicRedactionEngine) hashValue(value string) string {
	hash := sha256.Sum256([]byte(value))
	return fmt.Sprintf("[HASH:%x]", hash[:8]) // Use first 8 bytes for readability
}

// tokenizeValue creates a deterministic token for the value
func (r *BasicRedactionEngine) tokenizeValue(value string, piiType types.PIIType) string {
	// Simple tokenization - in production, use proper tokenization
	hash := sha256.Sum256([]byte(value))
	token := fmt.Sprintf("%x", hash[:4])
	
	switch piiType {
	case types.PIITypeSSN:
		return fmt.Sprintf("SSN_%s", token)
	case types.PIITypeCreditCard:
		return fmt.Sprintf("CC_%s", token)
	case types.PIITypeEmail:
		return fmt.Sprintf("EMAIL_%s", token)
	case types.PIITypePhoneNumber:
		return fmt.Sprintf("PHONE_%s", token)
	default:
		return fmt.Sprintf("PII_%s", token)
	}
}

// Specific masking functions

func (r *BasicRedactionEngine) maskSSN(ssn string) string {
	// Remove formatting
	digits := strings.ReplaceAll(strings.ReplaceAll(ssn, "-", ""), " ", "")
	
	if len(digits) == 9 {
		// Show last 4 digits: XXX-XX-1234
		if strings.Contains(ssn, "-") {
			return "XXX-XX-" + digits[5:]
		} else if strings.Contains(ssn, " ") {
			return "XXX XX " + digits[5:]
		} else {
			return "XXXXX" + digits[5:]
		}
	}
	
	return strings.Repeat("X", len(ssn))
}

func (r *BasicRedactionEngine) maskCreditCard(cc string) string {
	// Remove formatting
	digits := strings.ReplaceAll(strings.ReplaceAll(cc, "-", ""), " ", "")
	
	if len(digits) >= 12 {
		// Show last 4 digits: XXXX-XXXX-XXXX-1234
		masked := strings.Repeat("X", len(digits)-4) + digits[len(digits)-4:]
		
		// Preserve original formatting
		if strings.Contains(cc, "-") {
			return r.formatWithSeparator(masked, "-", 4)
		} else if strings.Contains(cc, " ") {
			return r.formatWithSeparator(masked, " ", 4)
		}
		
		return masked
	}
	
	return strings.Repeat("X", len(cc))
}

func (r *BasicRedactionEngine) maskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return strings.Repeat("X", len(email))
	}
	
	username := parts[0]
	domain := parts[1]
	
	// Mask username but keep first and last character if long enough
	var maskedUsername string
	if len(username) <= 2 {
		maskedUsername = strings.Repeat("X", len(username))
	} else {
		maskedUsername = string(username[0]) + strings.Repeat("X", len(username)-2) + string(username[len(username)-1])
	}
	
	return maskedUsername + "@" + domain
}

func (r *BasicRedactionEngine) maskPhoneNumber(phone string) string {
	// Keep area code visible, mask the rest
	digits := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(phone, "(", ""), ")", ""), "-", "")
	digits = strings.ReplaceAll(strings.ReplaceAll(digits, " ", ""), ".", "")
	
	if len(digits) >= 10 {
		// Show area code: (555) XXX-XXXX
		areaCode := digits[:3]
		if strings.Contains(phone, "(") && strings.Contains(phone, ")") {
			return fmt.Sprintf("(%s) XXX-XXXX", areaCode)
		} else if strings.Contains(phone, "-") {
			return fmt.Sprintf("%s-XXX-XXXX", areaCode)
		}
		return areaCode + "XXXXXXX"
	}
	
	return strings.Repeat("X", len(phone))
}

func (r *BasicRedactionEngine) maskIPAddress(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		// Show first octet: 192.XXX.XXX.XXX
		return parts[0] + ".XXX.XXX.XXX"
	}
	
	return strings.Repeat("X", len(ip))
}

func (r *BasicRedactionEngine) maskGeneric(value string) string {
	length := len(value)
	if length <= 4 {
		return strings.Repeat("X", length)
	}
	
	// Show first and last character for longer values
	return string(value[0]) + strings.Repeat("X", length-2) + string(value[length-1])
}

// Helper function to format masked values with separators
func (r *BasicRedactionEngine) formatWithSeparator(value, separator string, groupSize int) string {
	var formatted strings.Builder
	
	for i, char := range value {
		if i > 0 && i%groupSize == 0 {
			formatted.WriteString(separator)
		}
		formatted.WriteRune(char)
	}
	
	return formatted.String()
}