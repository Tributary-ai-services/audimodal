package scanner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jscharber/eAIIngest/pkg/dlp/patterns"
	"github.com/jscharber/eAIIngest/pkg/dlp/types"
)

// BasicDLPScanner implements a basic DLP scanner using pattern matching
type BasicDLPScanner struct {
	name            string
	version         string
	patternRegistry *patterns.PatternRegistry
	redactionEngine RedactionEngine
}

// RedactionEngine interface to avoid import cycle
type RedactionEngine interface {
	RedactContent(content string, scanResult *types.ScanResult, strategy types.RedactionStrategy) (string, error)
	GenerateRedactionMap(scanResult *types.ScanResult, strategy types.RedactionStrategy) map[string]string
}

// NewBasicDLPScanner creates a new basic DLP scanner
func NewBasicDLPScanner() *BasicDLPScanner {
	return &BasicDLPScanner{
		name:            "basic_dlp_scanner",
		version:         "1.0.0",
		patternRegistry: patterns.NewPatternRegistry(),
		redactionEngine: NewBasicRedactionEngine(),
	}
}

// ScanContent scans content for PII/sensitive data
func (s *BasicDLPScanner) ScanContent(ctx context.Context, content string, config *types.ScanConfig) (*types.ScanResult, error) {
	if config == nil {
		config = s.getDefaultConfig()
	}

	startTime := time.Now()

	// Create scan context with timeout
	scanCtx, cancel := context.WithTimeout(ctx, config.ScanTimeout)
	defer cancel()

	// Validate configuration
	if err := s.ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid scan configuration: %w", err)
	}

	result := &types.ScanResult{
		ChunkID:   uuid.New().String(),
		ScannedAt: startTime,
		Scanner:   s.name,
		Findings:  []types.Finding{},
		Metadata: types.ScanMetadata{
			ContentLength:  len(content),
			ScannerVersion: s.version,
		},
	}

	// Scan with each enabled pattern matcher
	var allFindings []types.Finding
	patternsScanned := 0

	for piiType, matcher := range s.patternRegistry.GetAllMatchers() {
		select {
		case <-scanCtx.Done():
			return nil, scanCtx.Err()
		default:
		}

		if !matcher.IsEnabled(config) {
			continue
		}

		patternsScanned++
		matches := matcher.Match(content)

		for _, match := range matches {
			if match.Confidence >= config.MinConfidence {
				finding := s.createFinding(match, matcher, piiType)
				allFindings = append(allFindings, finding)
			}
		}
	}

	// Process custom patterns if any
	if len(config.CustomPatterns) > 0 {
		customFindings, err := s.scanCustomPatterns(content, config.CustomPatterns, config.MinConfidence)
		if err == nil {
			allFindings = append(allFindings, customFindings...)
		}
	}

	// Calculate metrics and risk scores
	result.Findings = allFindings
	result.TotalMatches = len(allFindings)
	result.HighRiskCount = s.countHighRiskFindings(allFindings)
	result.RiskScore = s.calculateRiskScore(allFindings)

	// Update metadata
	result.Metadata.ScanDuration = time.Since(startTime)
	result.Metadata.PatternsScanned = patternsScanned
	result.Metadata.ProcessingTime = time.Since(startTime)

	return result, nil
}

// ScanChunk scans a chunk for PII/sensitive data
func (s *BasicDLPScanner) ScanChunk(ctx context.Context, chunk *types.ChunkContent, config *types.ScanConfig) (*types.ScanResult, error) {
	result, err := s.ScanContent(ctx, chunk.Content, config)
	if err != nil {
		return nil, err
	}

	// Update result with chunk-specific information
	result.ChunkID = chunk.ID

	// Scan metadata if enabled
	if config.ScanMetadata {
		for key, value := range chunk.Metadata {
			metadataResult, err := s.ScanContent(ctx, value, config)
			if err != nil {
				continue // Skip problematic metadata
			}

			// Add metadata findings with context
			for _, finding := range metadataResult.Findings {
				finding.Context = fmt.Sprintf("metadata:%s", key)
				result.Findings = append(result.Findings, finding)
			}
		}

		// Recalculate metrics after adding metadata findings
		result.TotalMatches = len(result.Findings)
		result.HighRiskCount = s.countHighRiskFindings(result.Findings)
		result.RiskScore = s.calculateRiskScore(result.Findings)
	}

	return result, nil
}

// GetSupportedPatterns returns patterns this scanner can detect
func (s *BasicDLPScanner) GetSupportedPatterns() []types.PatternInfo {
	var patterns []types.PatternInfo

	patterns = append(patterns, types.PatternInfo{
		Name:        "ssn",
		Type:        types.PIITypeSSN,
		Description: "Social Security Number",
		Regex:       `\b(?:\d{3}[-\s]?\d{2}[-\s]?\d{4})\b`,
		Examples:    []string{"123-45-6789", "123 45 6789", "123456789"},
	})

	patterns = append(patterns, types.PatternInfo{
		Name:        "credit_card",
		Type:        types.PIITypeCreditCard,
		Description: "Credit Card Number",
		Regex:       `\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13})\b`,
		Examples:    []string{"4111-1111-1111-1111", "5555555555554444"},
	})

	patterns = append(patterns, types.PatternInfo{
		Name:        "email",
		Type:        types.PIITypeEmail,
		Description: "Email Address",
		Regex:       `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`,
		Examples:    []string{"user@example.com", "test.email@domain.org"},
	})

	patterns = append(patterns, types.PatternInfo{
		Name:        "phone_number",
		Type:        types.PIITypePhoneNumber,
		Description: "Phone Number",
		Regex:       `\b(?:\+?1[-.\s]?)?\(?([0-9]{3})\)?[-.\s]?([0-9]{3})[-.\s]?([0-9]{4})\b`,
		Examples:    []string{"(555) 123-4567", "555-123-4567", "+1-555-123-4567"},
	})

	patterns = append(patterns, types.PatternInfo{
		Name:        "ip_address",
		Type:        types.PIITypeIPAddress,
		Description: "IP Address",
		Regex:       `\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`,
		Examples:    []string{"192.168.1.1", "10.0.0.1"},
	})

	return patterns
}

// ValidateConfig validates scanner configuration
func (s *BasicDLPScanner) ValidateConfig(config *types.ScanConfig) error {
	if config.MinConfidence < 0.0 || config.MinConfidence > 1.0 {
		return fmt.Errorf("min_confidence must be between 0.0 and 1.0")
	}

	if config.ScanTimeout <= 0 {
		return fmt.Errorf("scan_timeout must be positive")
	}

	if config.ContextWindow < 0 {
		return fmt.Errorf("context_window must be non-negative")
	}

	// Validate custom patterns
	for _, pattern := range config.CustomPatterns {
		if pattern.Regex == "" {
			return fmt.Errorf("custom pattern '%s' has empty regex", pattern.Name)
		}

		// Test regex compilation
		if _, err := matchCustomPattern(pattern.Regex, "test"); err != nil {
			return fmt.Errorf("custom pattern '%s' has invalid regex: %w", pattern.Name, err)
		}
	}

	return nil
}

// Internal methods

func (s *BasicDLPScanner) getDefaultConfig() *types.ScanConfig {
	return &types.ScanConfig{
		EnabledPatterns:  []types.PIIType{}, // Empty means all enabled
		DisabledPatterns: []types.PIIType{},
		MinConfidence:    0.7,
		ScanTimeout:      30 * time.Second,
		CustomPatterns:   []types.CustomPattern{},
		ComplianceRules:  []types.ComplianceRule{},
		RedactionMode:    types.RedactionMask,
		ContextWindow:    20,
		ScanMetadata:     true,
	}
}

func (s *BasicDLPScanner) createFinding(match types.Match, matcher patterns.PatternMatcher, piiType types.PIIType) types.Finding {
	riskLevel := s.calculateRiskLevel(piiType, match.Confidence)

	return types.Finding{
		ID:         uuid.New().String(),
		Type:       piiType,
		Pattern:    matcher.GetName(),
		Value:      match.Value,
		Confidence: match.Confidence,
		StartPos:   match.StartPos,
		EndPos:     match.EndPos,
		Context:    match.Context,
		RiskLevel:  riskLevel,
		Redacted:   false,
	}
}

func (s *BasicDLPScanner) calculateRiskLevel(piiType types.PIIType, confidence float64) types.RiskLevel {
	// Base risk by PII type
	baseRisk := map[types.PIIType]float64{
		types.PIITypeSSN:            0.9,
		types.PIITypeCreditCard:     0.9,
		types.PIITypePassport:       0.8,
		types.PIITypeDriversLicense: 0.7,
		types.PIITypeBankAccount:    0.8,
		types.PIITypeEmail:          0.4,
		types.PIITypePhoneNumber:    0.5,
		types.PIITypeIPAddress:      0.3,
		types.PIITypeAddress:        0.6,
		types.PIITypeName:           0.5,
		types.PIITypeDateOfBirth:    0.7,
	}

	base, exists := baseRisk[piiType]
	if !exists {
		base = 0.5 // Default for unknown types
	}

	// Combine base risk with confidence
	riskScore := (base + confidence) / 2.0

	// Map to risk levels
	if riskScore >= 0.8 {
		return types.RiskLevelCritical
	} else if riskScore >= 0.6 {
		return types.RiskLevelHigh
	} else if riskScore >= 0.4 {
		return types.RiskLevelMedium
	}

	return types.RiskLevelLow
}

func (s *BasicDLPScanner) countHighRiskFindings(findings []types.Finding) int {
	count := 0
	for _, finding := range findings {
		if finding.RiskLevel == types.RiskLevelHigh || finding.RiskLevel == types.RiskLevelCritical {
			count++
		}
	}
	return count
}

func (s *BasicDLPScanner) calculateRiskScore(findings []types.Finding) float64 {
	if len(findings) == 0 {
		return 0.0
	}

	totalRisk := 0.0
	for _, finding := range findings {
		switch finding.RiskLevel {
		case types.RiskLevelCritical:
			totalRisk += 1.0
		case types.RiskLevelHigh:
			totalRisk += 0.8
		case types.RiskLevelMedium:
			totalRisk += 0.5
		case types.RiskLevelLow:
			totalRisk += 0.2
		}
	}

	// Normalize by number of findings and cap at 1.0
	avgRisk := totalRisk / float64(len(findings))
	if avgRisk > 1.0 {
		avgRisk = 1.0
	}

	return avgRisk
}

func (s *BasicDLPScanner) scanCustomPatterns(content string, patterns []types.CustomPattern, minConfidence float64) ([]types.Finding, error) {
	var findings []types.Finding

	for _, pattern := range patterns {
		if !pattern.Enabled {
			continue
		}

		matches, err := matchCustomPattern(pattern.Regex, content)
		if err != nil {
			continue // Skip invalid patterns
		}

		for _, match := range matches {
			if match.Confidence >= minConfidence {
				finding := types.Finding{
					ID:         uuid.New().String(),
					Type:       pattern.Type,
					Pattern:    pattern.Name,
					Value:      match.Value,
					Confidence: match.Confidence,
					StartPos:   match.StartPos,
					EndPos:     match.EndPos,
					Context:    match.Context,
					RiskLevel:  s.calculateRiskLevel(pattern.Type, match.Confidence),
					Redacted:   false,
				}
				findings = append(findings, finding)
			}
		}
	}

	return findings, nil
}

func matchCustomPattern(regex, content string) ([]types.Match, error) {
	// This is a simplified implementation
	// In production, you'd want more sophisticated pattern matching
	var matches []types.Match

	// Basic implementation - would need proper regex compilation and matching
	if strings.Contains(content, regex) {
		matches = append(matches, types.Match{
			Value:      regex,
			StartPos:   strings.Index(content, regex),
			EndPos:     strings.Index(content, regex) + len(regex),
			Context:    content,
			Confidence: 0.7,
		})
	}

	return matches, nil
}
