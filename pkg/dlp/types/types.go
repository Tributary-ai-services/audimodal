package types

import (
	"time"

	"github.com/google/uuid"
)

// PIIType represents different types of personally identifiable information
type PIIType string

const (
	PIITypeSSN            PIIType = "ssn"
	PIITypeCreditCard     PIIType = "credit_card"
	PIITypeEmail          PIIType = "email"
	PIITypePhoneNumber    PIIType = "phone_number"
	PIITypeIPAddress      PIIType = "ip_address"
	PIITypeBankAccount    PIIType = "bank_account"
	PIITypePassport       PIIType = "passport"
	PIITypeDriversLicense PIIType = "drivers_license"
	PIITypeDateOfBirth    PIIType = "date_of_birth"
	PIITypeAddress        PIIType = "address"
	PIITypeName           PIIType = "name"
	PIITypeCustom         PIIType = "custom"
)

// RiskLevel represents the risk level of a finding
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

// RedactionStrategy defines how PII should be redacted
type RedactionStrategy string

const (
	RedactionNone      RedactionStrategy = "none"
	RedactionMask      RedactionStrategy = "mask"
	RedactionReplace   RedactionStrategy = "replace"
	RedactionHash      RedactionStrategy = "hash"
	RedactionRemove    RedactionStrategy = "remove"
	RedactionTokenize  RedactionStrategy = "tokenize"
)

// ScanStatus represents the status of a DLP scan
type ScanStatus string

const (
	ScanStatusPending    ScanStatus = "pending"
	ScanStatusScanning   ScanStatus = "scanning"
	ScanStatusCompleted  ScanStatus = "completed"
	ScanStatusFailed     ScanStatus = "failed"
	ScanStatusSkipped    ScanStatus = "skipped"
)

// Finding represents a detected PII instance
type Finding struct {
	ID           string    `json:"id"`
	Type         PIIType   `json:"type"`
	Pattern      string    `json:"pattern"`
	Value        string    `json:"value"`
	Confidence   float64   `json:"confidence"`
	StartPos     int       `json:"start_pos"`
	EndPos       int       `json:"end_pos"`
	Context      string    `json:"context"`
	RiskLevel    RiskLevel `json:"risk_level"`
	Redacted     bool      `json:"redacted"`
	RedactedWith string    `json:"redacted_with,omitempty"`
}

// ScanResult contains the results of a DLP scan
type ScanResult struct {
	ChunkID       string      `json:"chunk_id"`
	ScannedAt     time.Time   `json:"scanned_at"`
	Scanner       string      `json:"scanner"`
	TotalMatches  int         `json:"total_matches"`
	HighRiskCount int         `json:"high_risk_count"`
	Findings      []Finding   `json:"findings"`
	RiskScore     float64     `json:"risk_score"`
	IsCompliant   bool        `json:"is_compliant"`
	Metadata      ScanMetadata `json:"metadata"`
}

// ScanMetadata contains additional scan information
type ScanMetadata struct {
	ScanDuration     time.Duration `json:"scan_duration"`
	ContentLength    int           `json:"content_length"`
	PatternsScanned  int           `json:"patterns_scanned"`
	ProcessingTime   time.Duration `json:"processing_time"`
	ScannerVersion   string        `json:"scanner_version"`
}

// ComplianceRule represents a compliance requirement
type ComplianceRule struct {
	Regulation  string            `json:"regulation"`
	Rule        string            `json:"rule"`
	PIITypes    []PIIType         `json:"pii_types"`
	Required    bool              `json:"required"`
	Parameters  map[string]string `json:"parameters"`
}

// ComplianceResult contains compliance validation results
type ComplianceResult struct {
	IsCompliant   bool                   `json:"is_compliant"`
	Violations    []ComplianceViolation  `json:"violations"`
	Regulation    string                 `json:"regulation"`
	CheckedAt     time.Time              `json:"checked_at"`
	RequiredActions []string             `json:"required_actions"`
}

// ComplianceViolation represents a compliance violation
type ComplianceViolation struct {
	Rule        string    `json:"rule"`
	PIIType     PIIType   `json:"pii_type"`
	Severity    string    `json:"severity"`
	Description string    `json:"description"`
	FindingIDs  []string  `json:"finding_ids"`
}

// ScanConfig contains configuration for DLP scanning
type ScanConfig struct {
	TenantID         uuid.UUID         `json:"tenant_id"`
	EnabledPatterns  []PIIType         `json:"enabled_patterns"`
	DisabledPatterns []PIIType         `json:"disabled_patterns"`
	MinConfidence    float64           `json:"min_confidence"`
	ScanTimeout      time.Duration     `json:"scan_timeout"`
	CustomPatterns   []CustomPattern   `json:"custom_patterns"`
	ComplianceRules  []ComplianceRule  `json:"compliance_rules"`
	RedactionMode    RedactionStrategy `json:"redaction_mode"`
	ContextWindow    int               `json:"context_window"`
	ScanMetadata     bool              `json:"scan_metadata"`
}

// CustomPattern allows tenants to define custom PII patterns
type CustomPattern struct {
	Name        string  `json:"name"`
	Type        PIIType `json:"type"`
	Regex       string  `json:"regex"`
	Description string  `json:"description"`
	Enabled     bool    `json:"enabled"`
}

// Match represents a pattern match
type Match struct {
	Value     string  `json:"value"`
	StartPos  int     `json:"start_pos"`
	EndPos    int     `json:"end_pos"`
	Context   string  `json:"context"`
	Confidence float64 `json:"confidence"`
}

// PatternInfo describes a detection pattern
type PatternInfo struct {
	Name        string  `json:"name"`
	Type        PIIType `json:"type"`
	Description string  `json:"description"`
	Regex       string  `json:"regex"`
	Examples    []string `json:"examples"`
}

// ChunkContent represents content to be scanned
type ChunkContent struct {
	ID       string            `json:"id"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata"`
	Type     string            `json:"type"`
}