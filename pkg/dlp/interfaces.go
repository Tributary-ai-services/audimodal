package dlp

import (
	"context"

	"github.com/jscharber/eAIIngest/pkg/dlp/types"
)

// DLPScanner is the main interface for data loss prevention scanning
type DLPScanner interface {
	// ScanContent scans content for PII/sensitive data
	ScanContent(ctx context.Context, content string, config *types.ScanConfig) (*types.ScanResult, error)

	// ScanChunk scans a chunk for PII/sensitive data
	ScanChunk(ctx context.Context, chunk *types.ChunkContent, config *types.ScanConfig) (*types.ScanResult, error)

	// GetSupportedPatterns returns patterns this scanner can detect
	GetSupportedPatterns() []types.PatternInfo

	// ValidateConfig validates scanner configuration
	ValidateConfig(config *types.ScanConfig) error
}

// PatternMatcher defines an interface for detecting specific PII patterns
type PatternMatcher interface {
	// GetName returns the pattern name
	GetName() string

	// GetType returns the PII type this matcher detects
	GetType() types.PIIType

	// Match finds instances of this pattern in content
	Match(content string) []types.Match

	// GetConfidenceScore returns confidence in the match
	GetConfidenceScore(match string) float64

	// IsEnabled checks if this matcher should be used
	IsEnabled(config *types.ScanConfig) bool
}

// RedactionEngine handles redaction/masking of detected PII
type RedactionEngine interface {
	// RedactContent redacts PII from content based on scan results
	RedactContent(content string, scanResult *types.ScanResult, strategy types.RedactionStrategy) (string, error)

	// GenerateRedactionMap creates a mapping of original to redacted values
	GenerateRedactionMap(scanResult *types.ScanResult, strategy types.RedactionStrategy) map[string]string
}

// ComplianceChecker validates content against compliance rules
type ComplianceChecker interface {
	// CheckCompliance validates content against specific compliance requirements
	CheckCompliance(ctx context.Context, scanResult *types.ScanResult, rules []types.ComplianceRule) (*types.ComplianceResult, error)

	// GetSupportedRegulations returns supported compliance frameworks
	GetSupportedRegulations() []string
}

// Core interfaces and utility types are defined in the types package
