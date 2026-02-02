package patterns

import (
	"strings"
	"testing"

	"github.com/jscharber/audimodal/pkg/dlp/types"
)

// Sample documents for benchmarking
var (
	smallDocument = `
Customer Record
Name: John Smith
Email: john.smith@example.com
Phone: (555) 123-4567
SSN: 123-45-6789
`

	mediumDocument = strings.Repeat(`
Employee Data Record
====================
Name: Jane Doe
Email: jane.doe@company.com
Phone: (555) 987-6543
SSN: 234-56-7890
Credit Card: 4111-1111-1111-1111
IP Address: 192.168.1.100
Address: 123 Main Street, Anytown, ST 12345

Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor
incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud
exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.

`, 50) // ~15KB

	largeDocument = strings.Repeat(`
Comprehensive Data Record
=========================
Personal Information:
- Name: Test User
- Email: test.user@example.org
- Phone: (555) 111-2222
- SSN: 345-67-8901
- IP Address: 10.0.0.50

Financial Information:
- Visa: 4532-0151-1283-0366
- Mastercard: 5425-2334-3010-9903
- Amex: 3782-822463-10005

Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor
incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud
exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure
dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.
Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt
mollit anim id est laborum.

`, 500) // ~150KB

	// High density PII document
	highDensityDocument = strings.Repeat(`SSN: 123-45-6789 Email: user@test.com Card: 4111111111111111 IP: 192.168.1.1
`, 100)
)

// BenchmarkSSNMatcher_Match benchmarks SSN detection
func BenchmarkSSNMatcher_Match(b *testing.B) {
	matcher := NewSSNMatcher()
	content := "Employee SSN: 123-45-6789 and backup SSN: 234-56-7890"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.Match(content)
	}
}

// BenchmarkSSNMatcher_Match_LargeDocument benchmarks SSN detection on large documents
func BenchmarkSSNMatcher_Match_LargeDocument(b *testing.B) {
	matcher := NewSSNMatcher()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.Match(largeDocument)
	}
}

// BenchmarkSSNMatcher_GetConfidenceScore benchmarks SSN confidence scoring
func BenchmarkSSNMatcher_GetConfidenceScore(b *testing.B) {
	matcher := NewSSNMatcher()
	ssn := "123-45-6789"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.GetConfidenceScore(ssn)
	}
}

// BenchmarkCreditCardMatcher_Match benchmarks credit card detection
func BenchmarkCreditCardMatcher_Match(b *testing.B) {
	matcher := NewCreditCardMatcher()
	content := "Card: 4532015112830366 and 5425233430109903"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.Match(content)
	}
}

// BenchmarkCreditCardMatcher_Match_LargeDocument benchmarks credit card detection on large documents
func BenchmarkCreditCardMatcher_Match_LargeDocument(b *testing.B) {
	matcher := NewCreditCardMatcher()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.Match(largeDocument)
	}
}

// BenchmarkCreditCardMatcher_LuhnValidation benchmarks Luhn algorithm validation
func BenchmarkCreditCardMatcher_LuhnValidation(b *testing.B) {
	matcher := NewCreditCardMatcher()
	card := "4532015112830366"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.isValidCreditCard(card)
	}
}

// BenchmarkEmailMatcher_Match benchmarks email detection
func BenchmarkEmailMatcher_Match(b *testing.B) {
	matcher := NewEmailMatcher()
	content := "Contact: user@example.com, support@company.org, admin@test.net"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.Match(content)
	}
}

// BenchmarkEmailMatcher_Match_LargeDocument benchmarks email detection on large documents
func BenchmarkEmailMatcher_Match_LargeDocument(b *testing.B) {
	matcher := NewEmailMatcher()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.Match(largeDocument)
	}
}

// BenchmarkPhoneNumberMatcher_Match benchmarks phone number detection
func BenchmarkPhoneNumberMatcher_Match(b *testing.B) {
	matcher := NewPhoneNumberMatcher()
	content := "Call: (555) 123-4567, Fax: 555-987-6543, Mobile: 555.111.2222"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.Match(content)
	}
}

// BenchmarkPhoneNumberMatcher_Match_LargeDocument benchmarks phone number detection on large documents
func BenchmarkPhoneNumberMatcher_Match_LargeDocument(b *testing.B) {
	matcher := NewPhoneNumberMatcher()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.Match(largeDocument)
	}
}

// BenchmarkIPAddressMatcher_Match benchmarks IP address detection
func BenchmarkIPAddressMatcher_Match(b *testing.B) {
	matcher := NewIPAddressMatcher()
	content := "Server: 192.168.1.1, Gateway: 10.0.0.1, DNS: 8.8.8.8"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.Match(content)
	}
}

// BenchmarkIPAddressMatcher_Match_LargeDocument benchmarks IP address detection on large documents
func BenchmarkIPAddressMatcher_Match_LargeDocument(b *testing.B) {
	matcher := NewIPAddressMatcher()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matcher.Match(largeDocument)
	}
}

// BenchmarkAllMatchers_Match benchmarks all matchers on a small document
func BenchmarkAllMatchers_Match_SmallDocument(b *testing.B) {
	registry := NewPatternRegistry()
	matchers := registry.GetAllMatchers()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, matcher := range matchers {
			matcher.Match(smallDocument)
		}
	}
}

// BenchmarkAllMatchers_Match_MediumDocument benchmarks all matchers on a medium document
func BenchmarkAllMatchers_Match_MediumDocument(b *testing.B) {
	registry := NewPatternRegistry()
	matchers := registry.GetAllMatchers()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, matcher := range matchers {
			matcher.Match(mediumDocument)
		}
	}
}

// BenchmarkAllMatchers_Match_LargeDocument benchmarks all matchers on a large document
func BenchmarkAllMatchers_Match_LargeDocument(b *testing.B) {
	registry := NewPatternRegistry()
	matchers := registry.GetAllMatchers()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, matcher := range matchers {
			matcher.Match(largeDocument)
		}
	}
}

// BenchmarkAllMatchers_Match_HighDensityPII benchmarks all matchers on high-density PII document
func BenchmarkAllMatchers_Match_HighDensityPII(b *testing.B) {
	registry := NewPatternRegistry()
	matchers := registry.GetAllMatchers()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, matcher := range matchers {
			matcher.Match(highDensityDocument)
		}
	}
}

// BenchmarkPatternRegistry_GetMatcher benchmarks matcher lookup
func BenchmarkPatternRegistry_GetMatcher(b *testing.B) {
	registry := NewPatternRegistry()
	piiTypes := []types.PIIType{
		types.PIITypeSSN,
		types.PIITypeCreditCard,
		types.PIITypeEmail,
		types.PIITypePhoneNumber,
		types.PIITypeIPAddress,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, piiType := range piiTypes {
			registry.GetMatcher(piiType)
		}
	}
}

// BenchmarkPatternRegistry_GetAllMatchers benchmarks getting all matchers
func BenchmarkPatternRegistry_GetAllMatchers(b *testing.B) {
	registry := NewPatternRegistry()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.GetAllMatchers()
	}
}

// BenchmarkNewPatternRegistry benchmarks registry creation
func BenchmarkNewPatternRegistry(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewPatternRegistry()
	}
}

// Benchmark document sizes
func BenchmarkAllMatchers_DocumentSizes(b *testing.B) {
	registry := NewPatternRegistry()
	matchers := registry.GetAllMatchers()

	documents := map[string]string{
		"1KB":   smallDocument,
		"15KB":  mediumDocument,
		"150KB": largeDocument,
	}

	for name, doc := range documents {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				for _, matcher := range matchers {
					matcher.Match(doc)
				}
			}
		})
	}
}

// BenchmarkSSNMatcher_IsValidSSN benchmarks internal SSN validation
func BenchmarkSSNMatcher_IsValidSSN(b *testing.B) {
	matcher := NewSSNMatcher()
	ssns := []string{
		"123456789",  // valid
		"000123456",  // invalid prefix
		"666123456",  // invalid prefix
		"900123456",  // invalid prefix
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, ssn := range ssns {
			matcher.isValidSSN(ssn)
		}
	}
}

// BenchmarkIPAddressMatcher_IsValidIP benchmarks internal IP validation
func BenchmarkIPAddressMatcher_IsValidIP(b *testing.B) {
	matcher := NewIPAddressMatcher()
	ips := []string{
		"192.168.1.1",   // valid
		"10.0.0.1",      // valid
		"256.1.1.1",     // invalid
		"192.168.1",     // invalid
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, ip := range ips {
			matcher.isValidIP(ip)
		}
	}
}
