package detectors

import (
	"context"
	"crypto/md5"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jscharber/eAIIngest/pkg/anomaly"
)

// SecurityDetector implements security-focused anomaly detection
type SecurityDetector struct {
	name               string
	version            string
	enabled            bool
	config             *SecurityConfig
	baselines          map[string]*anomaly.BaselineData
	threatIntelligence *ThreatIntelligence
	dlpRules           []DLPRule
	securitySignatures map[string]SecuritySignature
}

// SecurityConfig contains configuration for security detection
type SecurityConfig struct {
	// Data loss prevention
	EnableDLP           bool `json:"enable_dlp"`
	PIIDetectionEnabled bool `json:"pii_detection_enabled"`
	CreditCardDetection bool `json:"credit_card_detection"`
	SSNDetection        bool `json:"ssn_detection"`
	EmailDetection      bool `json:"email_detection"`
	PhoneDetection      bool `json:"phone_detection"`
	IPAddressDetection  bool `json:"ip_address_detection"`

	// Malware detection
	EnableMalwareDetection bool     `json:"enable_malware_detection"`
	SuspiciousFileTypes    []string `json:"suspicious_file_types"`
	ExecutableFileTypes    []string `json:"executable_file_types"`

	// Content security
	EnableContentSecurity  bool    `json:"enable_content_security"`
	MaxEntropyThreshold    float64 `json:"max_entropy_threshold"`
	Base64DetectionEnabled bool    `json:"base64_detection_enabled"`
	HexDetectionEnabled    bool    `json:"hex_detection_enabled"`

	// Access control anomalies
	PrivilegeEscalationDetection bool `json:"privilege_escalation_detection"`
	UnauthorizedAccessDetection  bool `json:"unauthorized_access_detection"`
	SuspiciousLoginDetection     bool `json:"suspicious_login_detection"`

	// Threat intelligence
	EnableThreatIntel        bool          `json:"enable_threat_intel"`
	IOCCheckEnabled          bool          `json:"ioc_check_enabled"`
	ThreatFeedURL            string        `json:"threat_feed_url"`
	ThreatFeedUpdateInterval time.Duration `json:"threat_feed_update_interval"`

	// Encryption and obfuscation
	EncryptedContentDetection bool `json:"encrypted_content_detection"`
	ObfuscatedScriptDetection bool `json:"obfuscated_script_detection"`
	PackedFileDetection       bool `json:"packed_file_detection"`

	// Thresholds
	HighRiskThreshold     float64 `json:"high_risk_threshold"`
	CriticalRiskThreshold float64 `json:"critical_risk_threshold"`
	MinContentLength      int     `json:"min_content_length"`
	MaxScanSize           int64   `json:"max_scan_size"`
}

// ThreatIntelligence contains threat intelligence data
type ThreatIntelligence struct {
	MaliciousHashes    map[string]ThreatInfo `json:"malicious_hashes"`
	MaliciousDomains   map[string]ThreatInfo `json:"malicious_domains"`
	MaliciousIPs       map[string]ThreatInfo `json:"malicious_ips"`
	SuspiciousPatterns []PatternInfo         `json:"suspicious_patterns"`
	LastUpdated        time.Time             `json:"last_updated"`
}

// ThreatInfo contains information about a threat
type ThreatInfo struct {
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	Description string    `json:"description"`
	Source      string    `json:"source"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	Confidence  float64   `json:"confidence"`
}

// PatternInfo contains information about suspicious patterns
type PatternInfo struct {
	Pattern     string  `json:"pattern"`
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Severity    string  `json:"severity"`
	Confidence  float64 `json:"confidence"`
}

// DLPRule represents a data loss prevention rule
type DLPRule struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Pattern     *regexp.Regexp          `json:"-"`
	PatternStr  string                  `json:"pattern"`
	Type        string                  `json:"type"`
	Severity    anomaly.AnomalySeverity `json:"severity"`
	Description string                  `json:"description"`
	Enabled     bool                    `json:"enabled"`
	Confidence  float64                 `json:"confidence"`
}

// SecuritySignature represents a security detection signature
type SecuritySignature struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Pattern     *regexp.Regexp          `json:"-"`
	PatternStr  string                  `json:"pattern"`
	Type        string                  `json:"type"`
	Severity    anomaly.AnomalySeverity `json:"severity"`
	Description string                  `json:"description"`
	MITREID     string                  `json:"mitre_id,omitempty"`
	References  []string                `json:"references,omitempty"`
}

// NewSecurityDetector creates a new security-focused anomaly detector
func NewSecurityDetector() *SecurityDetector {
	detector := &SecurityDetector{
		name:    "security_detector",
		version: "1.0.0",
		enabled: true,
		config: &SecurityConfig{
			EnableDLP:                    true,
			PIIDetectionEnabled:          true,
			CreditCardDetection:          true,
			SSNDetection:                 true,
			EmailDetection:               true,
			PhoneDetection:               true,
			IPAddressDetection:           true,
			EnableMalwareDetection:       true,
			SuspiciousFileTypes:          []string{".exe", ".bat", ".cmd", ".ps1", ".vbs", ".js", ".jar"},
			ExecutableFileTypes:          []string{".exe", ".dll", ".sys", ".com", ".scr"},
			EnableContentSecurity:        true,
			MaxEntropyThreshold:          7.5,
			Base64DetectionEnabled:       true,
			HexDetectionEnabled:          true,
			PrivilegeEscalationDetection: true,
			UnauthorizedAccessDetection:  true,
			SuspiciousLoginDetection:     true,
			EnableThreatIntel:            true,
			IOCCheckEnabled:              true,
			ThreatFeedUpdateInterval:     6 * time.Hour,
			EncryptedContentDetection:    true,
			ObfuscatedScriptDetection:    true,
			PackedFileDetection:          true,
			HighRiskThreshold:            0.7,
			CriticalRiskThreshold:        0.9,
			MinContentLength:             10,
			MaxScanSize:                  10 * 1024 * 1024, // 10MB
		},
		baselines: make(map[string]*anomaly.BaselineData),
		threatIntelligence: &ThreatIntelligence{
			MaliciousHashes:  make(map[string]ThreatInfo),
			MaliciousDomains: make(map[string]ThreatInfo),
			MaliciousIPs:     make(map[string]ThreatInfo),
		},
		securitySignatures: make(map[string]SecuritySignature),
	}

	// Initialize DLP rules
	detector.initializeDLPRules()

	// Initialize security signatures
	detector.initializeSecuritySignatures()

	return detector
}

func (d *SecurityDetector) GetName() string {
	return d.name
}

func (d *SecurityDetector) GetVersion() string {
	return d.version
}

func (d *SecurityDetector) GetSupportedTypes() []anomaly.AnomalyType {
	return []anomaly.AnomalyType{
		anomaly.AnomalyTypeSuspiciousContent,
		anomaly.AnomalyTypeDataLeak,
		anomaly.AnomalyTypeMaliciousPattern,
		anomaly.AnomalyTypeUnauthorizedAccess,
		anomaly.AnomalyTypePrivilegeEscalation,
	}
}

func (d *SecurityDetector) IsEnabled() bool {
	return d.enabled
}

func (d *SecurityDetector) Configure(config map[string]interface{}) error {
	if enabled, ok := config["enabled"].(bool); ok {
		d.enabled = enabled
	}
	if threshold, ok := config["high_risk_threshold"].(float64); ok {
		d.config.HighRiskThreshold = threshold
	}
	if enableDLP, ok := config["enable_dlp"].(bool); ok {
		d.config.EnableDLP = enableDLP
	}
	return nil
}

func (d *SecurityDetector) DetectAnomalies(ctx context.Context, input *anomaly.DetectionInput) ([]*anomaly.Anomaly, error) {
	var anomalies []*anomaly.Anomaly

	// Skip if content is too large
	if input.FileSize > d.config.MaxScanSize {
		return anomalies, nil
	}

	// Data Loss Prevention (DLP) detection
	if d.config.EnableDLP {
		dlpAnomalies := d.detectDLPViolations(ctx, input)
		anomalies = append(anomalies, dlpAnomalies...)
	}

	// Malware and suspicious content detection
	if d.config.EnableMalwareDetection {
		malwareAnomalies := d.detectMalwareIndicators(ctx, input)
		anomalies = append(anomalies, malwareAnomalies...)
	}

	// Content security analysis
	if d.config.EnableContentSecurity {
		contentSecurityAnomalies := d.detectContentSecurityIssues(ctx, input)
		anomalies = append(anomalies, contentSecurityAnomalies...)
	}

	// Threat intelligence checks
	if d.config.EnableThreatIntel && d.config.IOCCheckEnabled {
		threatIntelAnomalies := d.detectThreatIntelligenceMatches(ctx, input)
		anomalies = append(anomalies, threatIntelAnomalies...)
	}

	// Security signature detection
	signatureAnomalies := d.detectSecuritySignatures(ctx, input)
	anomalies = append(anomalies, signatureAnomalies...)

	// Access control anomalies
	accessControlAnomalies := d.detectAccessControlAnomalies(ctx, input)
	anomalies = append(anomalies, accessControlAnomalies...)

	return anomalies, nil
}

func (d *SecurityDetector) detectDLPViolations(ctx context.Context, input *anomaly.DetectionInput) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	for _, rule := range d.dlpRules {
		if !rule.Enabled {
			continue
		}

		matches := rule.Pattern.FindAllString(input.Content, -1)
		if len(matches) > 0 {
			// Calculate risk score based on number of matches
			riskScore := math.Min(float64(len(matches))/10.0, 1.0)

			// Adjust severity based on number of matches
			severity := rule.Severity
			if len(matches) > 5 {
				if severity == anomaly.SeverityLow {
					severity = anomaly.SeverityMedium
				} else if severity == anomaly.SeverityMedium {
					severity = anomaly.SeverityHigh
				}
			}

			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeDataLeak,
				Severity:        severity,
				Status:          anomaly.StatusDetected,
				Title:           fmt.Sprintf("DLP Violation: %s", rule.Name),
				Description:     fmt.Sprintf("Detected %d instances of %s in content", len(matches), rule.Type),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				DataSourceID:    input.DataSourceID,
				DocumentID:      input.DocumentID,
				ChunkID:         input.ChunkID,
				UserID:          input.UserID,
				Score:           riskScore,
				Confidence:      rule.Confidence,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				RuleName:        rule.Name,
				Detected: map[string]interface{}{
					"rule_type":   rule.Type,
					"match_count": len(matches),
					"first_match": matches[0],
				},
				Metadata: map[string]interface{}{
					"detection_method": "dlp_rule",
					"rule_id":          rule.ID,
					"pattern_type":     rule.Type,
					"all_matches":      d.sanitizeMatches(matches, rule.Type),
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies
}

func (d *SecurityDetector) detectMalwareIndicators(ctx context.Context, input *anomaly.DetectionInput) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	// File type analysis
	if input.FileName != "" {
		for _, suspiciousType := range d.config.SuspiciousFileTypes {
			if strings.HasSuffix(strings.ToLower(input.FileName), suspiciousType) {
				severity := anomaly.SeverityMedium

				// Executables are higher risk
				for _, execType := range d.config.ExecutableFileTypes {
					if suspiciousType == execType {
						severity = anomaly.SeverityHigh
						break
					}
				}

				anomaly := &anomaly.Anomaly{
					ID:              uuid.New(),
					Type:            anomaly.AnomalyTypeSuspiciousContent,
					Severity:        severity,
					Status:          anomaly.StatusDetected,
					Title:           "Suspicious File Type",
					Description:     fmt.Sprintf("File has potentially dangerous extension: %s", suspiciousType),
					DetectedAt:      time.Now(),
					UpdatedAt:       time.Now(),
					TenantID:        input.TenantID,
					DataSourceID:    input.DataSourceID,
					DocumentID:      input.DocumentID,
					UserID:          input.UserID,
					Score:           0.8,
					Confidence:      0.9,
					DetectorName:    d.name,
					DetectorVersion: d.version,
					Detected: map[string]interface{}{
						"file_name":      input.FileName,
						"file_extension": suspiciousType,
					},
					Metadata: map[string]interface{}{
						"detection_method": "file_type_analysis",
					},
				}
				anomalies = append(anomalies, anomaly)
				break
			}
		}
	}

	// Content hash analysis
	if len(input.Content) > 0 {
		contentHash := fmt.Sprintf("%x", md5.Sum([]byte(input.Content)))

		if threatInfo, exists := d.threatIntelligence.MaliciousHashes[contentHash]; exists {
			severity := anomaly.SeverityHigh
			if threatInfo.Severity == "critical" {
				severity = anomaly.SeverityCritical
			}

			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeMaliciousPattern,
				Severity:        severity,
				Status:          anomaly.StatusDetected,
				Title:           "Malicious Content Hash",
				Description:     fmt.Sprintf("Content matches known malicious hash: %s", threatInfo.Type),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				DataSourceID:    input.DataSourceID,
				DocumentID:      input.DocumentID,
				UserID:          input.UserID,
				Score:           0.95,
				Confidence:      threatInfo.Confidence,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				Detected: map[string]interface{}{
					"content_hash":       contentHash,
					"threat_type":        threatInfo.Type,
					"threat_description": threatInfo.Description,
					"threat_source":      threatInfo.Source,
				},
				Metadata: map[string]interface{}{
					"detection_method": "hash_match",
					"ioc_type":         "hash",
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	// Packed/encrypted content detection
	if d.config.PackedFileDetection || d.config.EncryptedContentDetection {
		entropy := d.calculateEntropy(input.Content)
		if entropy > d.config.MaxEntropyThreshold {
			severity := anomaly.SeverityMedium
			if entropy > 7.8 {
				severity = anomaly.SeverityHigh
			}

			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeSuspiciousContent,
				Severity:        severity,
				Status:          anomaly.StatusDetected,
				Title:           "High Entropy Content",
				Description:     fmt.Sprintf("Content has high entropy (%.2f), indicating possible encryption or packing", entropy),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				DataSourceID:    input.DataSourceID,
				DocumentID:      input.DocumentID,
				UserID:          input.UserID,
				Score:           (entropy - 6.0) / 2.0, // Normalize to 0-1 scale
				Confidence:      0.7,
				Threshold:       d.config.MaxEntropyThreshold,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				Detected: map[string]interface{}{
					"entropy": entropy,
				},
				Metadata: map[string]interface{}{
					"detection_method": "entropy_analysis",
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies
}

func (d *SecurityDetector) detectContentSecurityIssues(ctx context.Context, input *anomaly.DetectionInput) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	content := input.Content

	// Base64 encoded content detection
	if d.config.Base64DetectionEnabled {
		base64Pattern := regexp.MustCompile(`[A-Za-z0-9+/]{20,}={0,2}`)
		base64Matches := base64Pattern.FindAllString(content, -1)

		if len(base64Matches) > 0 {
			// Calculate base64 density
			totalBase64Length := 0
			for _, match := range base64Matches {
				totalBase64Length += len(match)
			}
			base64Density := float64(totalBase64Length) / float64(len(content))

			if base64Density > 0.3 { // More than 30% base64 content
				severity := anomaly.SeverityLow
				if base64Density > 0.6 {
					severity = anomaly.SeverityMedium
				}
				if base64Density > 0.8 {
					severity = anomaly.SeverityHigh
				}

				anomaly := &anomaly.Anomaly{
					ID:              uuid.New(),
					Type:            anomaly.AnomalyTypeSuspiciousContent,
					Severity:        severity,
					Status:          anomaly.StatusDetected,
					Title:           "High Base64 Content Density",
					Description:     fmt.Sprintf("Content contains %.1f%% base64 encoded data (%d matches)", base64Density*100, len(base64Matches)),
					DetectedAt:      time.Now(),
					UpdatedAt:       time.Now(),
					TenantID:        input.TenantID,
					DataSourceID:    input.DataSourceID,
					DocumentID:      input.DocumentID,
					UserID:          input.UserID,
					Score:           base64Density,
					Confidence:      0.8,
					Threshold:       0.3,
					DetectorName:    d.name,
					DetectorVersion: d.version,
					Detected: map[string]interface{}{
						"base64_density": base64Density,
						"match_count":    len(base64Matches),
					},
					Metadata: map[string]interface{}{
						"detection_method": "base64_analysis",
					},
				}
				anomalies = append(anomalies, anomaly)
			}
		}
	}

	// Hexadecimal content detection
	if d.config.HexDetectionEnabled {
		hexPattern := regexp.MustCompile(`(?i)[0-9a-f]{16,}`)
		hexMatches := hexPattern.FindAllString(content, -1)

		if len(hexMatches) > 0 {
			totalHexLength := 0
			for _, match := range hexMatches {
				totalHexLength += len(match)
			}
			hexDensity := float64(totalHexLength) / float64(len(content))

			if hexDensity > 0.4 { // More than 40% hex content
				anomaly := &anomaly.Anomaly{
					ID:              uuid.New(),
					Type:            anomaly.AnomalyTypeSuspiciousContent,
					Severity:        anomaly.SeverityMedium,
					Status:          anomaly.StatusDetected,
					Title:           "High Hexadecimal Content Density",
					Description:     fmt.Sprintf("Content contains %.1f%% hexadecimal data", hexDensity*100),
					DetectedAt:      time.Now(),
					UpdatedAt:       time.Now(),
					TenantID:        input.TenantID,
					DataSourceID:    input.DataSourceID,
					DocumentID:      input.DocumentID,
					UserID:          input.UserID,
					Score:           hexDensity,
					Confidence:      0.7,
					Threshold:       0.4,
					DetectorName:    d.name,
					DetectorVersion: d.version,
					Detected: map[string]interface{}{
						"hex_density": hexDensity,
						"match_count": len(hexMatches),
					},
					Metadata: map[string]interface{}{
						"detection_method": "hex_analysis",
					},
				}
				anomalies = append(anomalies, anomaly)
			}
		}
	}

	// Obfuscated script detection
	if d.config.ObfuscatedScriptDetection {
		obfuscationIndicators := []string{
			`eval\s*\(`,
			`document\.write\s*\(`,
			`fromCharCode`,
			`unescape\s*\(`,
			`decodeURIComponent\s*\(`,
			`String\.fromCharCode`,
			`\\x[0-9a-fA-F]{2}`,
			`\\u[0-9a-fA-F]{4}`,
		}

		obfuscationScore := 0.0
		matchedIndicators := []string{}

		for _, indicator := range obfuscationIndicators {
			pattern := regexp.MustCompile(indicator)
			if pattern.MatchString(content) {
				obfuscationScore += 0.2
				matchedIndicators = append(matchedIndicators, indicator)
			}
		}

		if obfuscationScore > 0.4 {
			severity := anomaly.SeverityMedium
			if obfuscationScore > 0.8 {
				severity = anomaly.SeverityHigh
			}

			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeSuspiciousContent,
				Severity:        severity,
				Status:          anomaly.StatusDetected,
				Title:           "Obfuscated Script Content",
				Description:     fmt.Sprintf("Content shows signs of script obfuscation (score: %.1f)", obfuscationScore),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				DataSourceID:    input.DataSourceID,
				DocumentID:      input.DocumentID,
				UserID:          input.UserID,
				Score:           obfuscationScore,
				Confidence:      0.8,
				Threshold:       0.4,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				Detected: map[string]interface{}{
					"obfuscation_score": obfuscationScore,
					"indicators":        matchedIndicators,
				},
				Metadata: map[string]interface{}{
					"detection_method": "obfuscation_analysis",
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies
}

func (d *SecurityDetector) detectThreatIntelligenceMatches(ctx context.Context, input *anomaly.DetectionInput) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	// Check for suspicious patterns from threat intelligence
	for _, pattern := range d.threatIntelligence.SuspiciousPatterns {
		patternRegex, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			continue
		}

		matches := patternRegex.FindAllString(input.Content, -1)
		if len(matches) > 0 {
			severity := anomaly.SeverityMedium
			switch pattern.Severity {
			case "low":
				severity = anomaly.SeverityLow
			case "high":
				severity = anomaly.SeverityHigh
			case "critical":
				severity = anomaly.SeverityCritical
			}

			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeMaliciousPattern,
				Severity:        severity,
				Status:          anomaly.StatusDetected,
				Title:           fmt.Sprintf("Threat Intelligence Match: %s", pattern.Type),
				Description:     fmt.Sprintf("Content matches suspicious pattern from threat intelligence: %s", pattern.Description),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				DataSourceID:    input.DataSourceID,
				DocumentID:      input.DocumentID,
				UserID:          input.UserID,
				Score:           pattern.Confidence,
				Confidence:      pattern.Confidence,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				Detected: map[string]interface{}{
					"pattern_type": pattern.Type,
					"match_count":  len(matches),
					"first_match":  matches[0],
				},
				Metadata: map[string]interface{}{
					"detection_method": "threat_intelligence",
					"pattern_id":       pattern.Pattern,
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies
}

func (d *SecurityDetector) detectSecuritySignatures(ctx context.Context, input *anomaly.DetectionInput) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	for _, signature := range d.securitySignatures {
		matches := signature.Pattern.FindAllString(input.Content, -1)
		if len(matches) > 0 {
			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeMaliciousPattern,
				Severity:        signature.Severity,
				Status:          anomaly.StatusDetected,
				Title:           fmt.Sprintf("Security Signature: %s", signature.Name),
				Description:     fmt.Sprintf("%s (MITRE: %s)", signature.Description, signature.MITREID),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				DataSourceID:    input.DataSourceID,
				DocumentID:      input.DocumentID,
				UserID:          input.UserID,
				Score:           0.9,
				Confidence:      0.85,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				RuleName:        signature.Name,
				Detected: map[string]interface{}{
					"signature_id":   signature.ID,
					"signature_type": signature.Type,
					"match_count":    len(matches),
				},
				Metadata: map[string]interface{}{
					"detection_method": "security_signature",
					"mitre_id":         signature.MITREID,
					"references":       signature.References,
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies
}

func (d *SecurityDetector) detectAccessControlAnomalies(ctx context.Context, input *anomaly.DetectionInput) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	// This would typically integrate with your access control system
	// For now, we'll implement basic detection based on metadata

	if input.Metadata != nil {
		// Check for privilege escalation indicators
		if d.config.PrivilegeEscalationDetection {
			if escalationEvent, exists := input.Metadata["privilege_escalation"]; exists {
				if escalated, ok := escalationEvent.(bool); ok && escalated {
					anomaly := &anomaly.Anomaly{
						ID:              uuid.New(),
						Type:            anomaly.AnomalyTypePrivilegeEscalation,
						Severity:        anomaly.SeverityHigh,
						Status:          anomaly.StatusDetected,
						Title:           "Privilege Escalation Detected",
						Description:     "User accessed content with elevated privileges",
						DetectedAt:      time.Now(),
						UpdatedAt:       time.Now(),
						TenantID:        input.TenantID,
						UserID:          input.UserID,
						Score:           0.9,
						Confidence:      0.8,
						DetectorName:    d.name,
						DetectorVersion: d.version,
						Metadata: map[string]interface{}{
							"detection_method": "access_control",
						},
					}
					anomalies = append(anomalies, anomaly)
				}
			}
		}

		// Check for unauthorized access attempts
		if d.config.UnauthorizedAccessDetection {
			if accessDenied, exists := input.Metadata["access_denied"]; exists {
				if denied, ok := accessDenied.(bool); ok && denied {
					anomaly := &anomaly.Anomaly{
						ID:              uuid.New(),
						Type:            anomaly.AnomalyTypeUnauthorizedAccess,
						Severity:        anomaly.SeverityMedium,
						Status:          anomaly.StatusDetected,
						Title:           "Unauthorized Access Attempt",
						Description:     "User attempted to access restricted content",
						DetectedAt:      time.Now(),
						UpdatedAt:       time.Now(),
						TenantID:        input.TenantID,
						UserID:          input.UserID,
						Score:           0.7,
						Confidence:      0.9,
						DetectorName:    d.name,
						DetectorVersion: d.version,
						Metadata: map[string]interface{}{
							"detection_method": "access_control",
						},
					}
					anomalies = append(anomalies, anomaly)
				}
			}
		}
	}

	return anomalies
}

func (d *SecurityDetector) UpdateBaseline(ctx context.Context, data *anomaly.BaselineData) error {
	key := d.getBaselineKey(data.TenantID, data.DataType)
	data.UpdatedAt = time.Now()
	d.baselines[key] = data
	return nil
}

func (d *SecurityDetector) GetBaseline(ctx context.Context) (*anomaly.BaselineData, error) {
	for _, baseline := range d.baselines {
		return baseline, nil
	}
	return nil, fmt.Errorf("no baseline data available")
}

// Helper methods

func (d *SecurityDetector) getBaselineKey(tenantID uuid.UUID, dataType string) string {
	return fmt.Sprintf("%s:%s", tenantID.String(), dataType)
}

func (d *SecurityDetector) calculateEntropy(data string) float64 {
	if len(data) == 0 {
		return 0
	}

	// Count character frequencies
	freq := make(map[rune]int)
	for _, char := range data {
		freq[char]++
	}

	// Calculate entropy
	entropy := 0.0
	length := float64(len(data))

	for _, count := range freq {
		if count > 0 {
			p := float64(count) / length
			entropy -= p * math.Log2(p)
		}
	}

	return entropy
}

func (d *SecurityDetector) sanitizeMatches(matches []string, ruleType string) []string {
	// Sanitize sensitive matches for logging
	sanitized := make([]string, len(matches))

	for i, match := range matches {
		switch strings.ToLower(ruleType) {
		case "credit_card", "ssn", "phone":
			// Mask most of the sensitive data
			if len(match) > 4 {
				sanitized[i] = match[:2] + strings.Repeat("*", len(match)-4) + match[len(match)-2:]
			} else {
				sanitized[i] = strings.Repeat("*", len(match))
			}
		case "email":
			// Mask email addresses
			parts := strings.Split(match, "@")
			if len(parts) == 2 {
				username := parts[0]
				domain := parts[1]
				if len(username) > 2 {
					sanitized[i] = username[:1] + strings.Repeat("*", len(username)-2) + username[len(username)-1:] + "@" + domain
				} else {
					sanitized[i] = strings.Repeat("*", len(username)) + "@" + domain
				}
			} else {
				sanitized[i] = match
			}
		default:
			sanitized[i] = match
		}
	}

	return sanitized
}

func (d *SecurityDetector) initializeDLPRules() {
	d.dlpRules = []DLPRule{
		{
			ID:          "dlp_credit_card",
			Name:        "Credit Card Number",
			PatternStr:  `\b(?:\d{4}[-\s]?){3}\d{4}\b`,
			Type:        "credit_card",
			Severity:    anomaly.SeverityHigh,
			Description: "Detects credit card numbers",
			Enabled:     d.config.CreditCardDetection,
			Confidence:  0.9,
		},
		{
			ID:          "dlp_ssn",
			Name:        "Social Security Number",
			PatternStr:  `\b\d{3}-\d{2}-\d{4}\b`,
			Type:        "ssn",
			Severity:    anomaly.SeverityCritical,
			Description: "Detects US Social Security Numbers",
			Enabled:     d.config.SSNDetection,
			Confidence:  0.95,
		},
		{
			ID:          "dlp_email",
			Name:        "Email Address",
			PatternStr:  `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`,
			Type:        "email",
			Severity:    anomaly.SeverityLow,
			Description: "Detects email addresses",
			Enabled:     d.config.EmailDetection,
			Confidence:  0.8,
		},
		{
			ID:          "dlp_phone",
			Name:        "Phone Number",
			PatternStr:  `\b(?:\+?1[-.\s]?)?\(?([0-9]{3})\)?[-.\s]?([0-9]{3})[-.\s]?([0-9]{4})\b`,
			Type:        "phone",
			Severity:    anomaly.SeverityMedium,
			Description: "Detects phone numbers",
			Enabled:     d.config.PhoneDetection,
			Confidence:  0.7,
		},
		{
			ID:          "dlp_ip_address",
			Name:        "IP Address",
			PatternStr:  `\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`,
			Type:        "ip_address",
			Severity:    anomaly.SeverityLow,
			Description: "Detects IP addresses",
			Enabled:     d.config.IPAddressDetection,
			Confidence:  0.8,
		},
	}

	// Compile regex patterns
	for i := range d.dlpRules {
		pattern, err := regexp.Compile(d.dlpRules[i].PatternStr)
		if err == nil {
			d.dlpRules[i].Pattern = pattern
		}
	}
}

func (d *SecurityDetector) initializeSecuritySignatures() {
	signatures := []SecuritySignature{
		{
			ID:          "sig_powershell_execution",
			Name:        "PowerShell Execution",
			PatternStr:  `(?i)powershell\.exe|pwsh\.exe|-encodedcommand|invoke-expression|iex\s`,
			Type:        "execution",
			Severity:    anomaly.SeverityMedium,
			Description: "Detects PowerShell execution indicators",
			MITREID:     "T1059.001",
			References:  []string{"https://attack.mitre.org/techniques/T1059/001/"},
		},
		{
			ID:          "sig_cmd_injection",
			Name:        "Command Injection",
			PatternStr:  `(?i)(\|\s*)(whoami|netstat|ipconfig|ifconfig|ps\s|ls\s|cat\s|wget\s|curl\s)`,
			Type:        "injection",
			Severity:    anomaly.SeverityHigh,
			Description: "Detects potential command injection attempts",
			MITREID:     "T1059",
			References:  []string{"https://attack.mitre.org/techniques/T1059/"},
		},
		{
			ID:          "sig_sql_injection",
			Name:        "SQL Injection",
			PatternStr:  `(?i)(union\s+select|or\s+1\s*=\s*1|drop\s+table|insert\s+into|update\s+.*\s+set)`,
			Type:        "injection",
			Severity:    anomaly.SeverityHigh,
			Description: "Detects potential SQL injection attempts",
			MITREID:     "T1190",
			References:  []string{"https://attack.mitre.org/techniques/T1190/"},
		},
	}

	for _, sig := range signatures {
		pattern, err := regexp.Compile(sig.PatternStr)
		if err == nil {
			sig.Pattern = pattern
			d.securitySignatures[sig.ID] = sig
		}
	}
}
