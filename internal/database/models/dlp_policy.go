package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// DLPPolicy represents a Data Loss Prevention policy for a tenant
type DLPPolicy struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Name        string    `gorm:"not null;index" json:"name"`
	DisplayName string    `gorm:"not null" json:"display_name"`
	Description string    `json:"description,omitempty"`
	
	// Policy configuration
	Enabled  bool `gorm:"not null;default:true" json:"enabled"`
	Priority int  `gorm:"not null;default:50" json:"priority"`
	
	// Content rules
	ContentRules []DLPContentRule `gorm:"type:jsonb" json:"content_rules"`
	
	// Actions to take when policy is triggered
	Actions []DLPAction `gorm:"type:jsonb" json:"actions"`
	
	// Conditions for when this policy applies
	Conditions DLPConditions `gorm:"type:jsonb" json:"conditions"`
	
	// Status and metadata
	Status     string    `gorm:"not null;default:'active'" json:"status"`
	CreatedAt  time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt  time.Time `gorm:"not null" json:"updated_at"`
	DeletedAt  *time.Time `gorm:"index" json:"deleted_at,omitempty"`
	
	// Statistics
	TriggeredCount int64     `gorm:"default:0" json:"triggered_count"`
	LastTriggered  *time.Time `json:"last_triggered,omitempty"`
	
	// Relationships
	Tenant     *Tenant        `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Violations []DLPViolation `gorm:"foreignKey:PolicyID" json:"violations,omitempty"`
}

// DLPContentRule represents a content detection rule
type DLPContentRule struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`         // regex, keyword, ml_model, builtin
	Pattern     string   `json:"pattern,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	ModelName   string   `json:"model_name,omitempty"`
	Sensitivity string   `json:"sensitivity"`  // low, medium, high, critical
	Confidence  float64  `json:"confidence"`   // Minimum confidence threshold
	BuiltinType string   `json:"builtin_type,omitempty"` // ssn, credit_card, email, etc.
	
	// Context requirements
	Context        string `json:"context,omitempty"`        // Additional context requirements
	ContextWindow  int    `json:"context_window,omitempty"` // Number of surrounding words to consider
	CaseSensitive  bool   `json:"case_sensitive"`
	WholeWordOnly  bool   `json:"whole_word_only"`
	
	// Exclusions
	Exclusions []string `json:"exclusions,omitempty"` // Patterns to exclude from matches
}

// DLPAction represents an action to take when a policy is triggered
type DLPAction struct {
	Type     string                 `json:"type"`     // encrypt, redact, alert, block, audit
	Config   map[string]interface{} `json:"config,omitempty"`
	Priority int                    `json:"priority"` // Order of execution
	
	// Notification settings
	NotifyRoles  []string `json:"notify_roles,omitempty"`
	NotifyEmails []string `json:"notify_emails,omitempty"`
	
	// Redaction settings
	RedactionMethod string `json:"redaction_method,omitempty"` // mask, replace, remove
	RedactionValue  string `json:"redaction_value,omitempty"`  // Replacement value
	
	// Encryption settings
	EncryptionKey string `json:"encryption_key,omitempty"`
	
	// Audit settings
	AuditLevel string `json:"audit_level,omitempty"` // info, warning, error, critical
}

// DLPConditions represents conditions for when a policy applies
type DLPConditions struct {
	FileTypes     []string `json:"file_types,omitempty"`     // File types to apply policy to
	FileSizeMin   int64    `json:"file_size_min,omitempty"`  // Minimum file size in bytes
	FileSizeMax   int64    `json:"file_size_max,omitempty"`  // Maximum file size in bytes
	DataSources   []string `json:"data_sources,omitempty"`   // Data source IDs to apply policy to
	Classifications []string `json:"classifications,omitempty"` // Content classifications
	
	// Time-based conditions
	TimeRestriction string `json:"time_restriction,omitempty"` // Cron expression for when policy applies
	
	// Geography-based conditions
	AllowedRegions []string `json:"allowed_regions,omitempty"`
	BlockedRegions []string `json:"blocked_regions,omitempty"`
	
	// User-based conditions
	AllowedUsers []string `json:"allowed_users,omitempty"`
	BlockedUsers []string `json:"blocked_users,omitempty"`
	
	// Content-based conditions
	ContentLanguages []string `json:"content_languages,omitempty"`
}

// DLPViolation represents a violation of a DLP policy
type DLPViolation struct {
	ID       uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	PolicyID uuid.UUID `gorm:"type:uuid;not null;index" json:"policy_id"`
	FileID   uuid.UUID `gorm:"type:uuid;not null;index" json:"file_id"`
	ChunkID  *uuid.UUID `gorm:"type:uuid;index" json:"chunk_id,omitempty"`
	
	// Violation details
	RuleName     string  `gorm:"not null" json:"rule_name"`
	Severity     string  `gorm:"not null" json:"severity"`
	Confidence   float64 `gorm:"not null" json:"confidence"`
	MatchedText  string  `gorm:"type:text" json:"matched_text,omitempty"`
	Context      string  `gorm:"type:text" json:"context,omitempty"`
	
	// Location information
	StartOffset int64 `json:"start_offset,omitempty"`
	EndOffset   int64 `json:"end_offset,omitempty"`
	LineNumber  int   `json:"line_number,omitempty"`
	
	// Actions taken
	ActionsTaken []string `gorm:"type:jsonb" json:"actions_taken"`
	
	// Status
	Status       string    `gorm:"not null;default:'detected'" json:"status"`
	Acknowledged bool      `gorm:"default:false" json:"acknowledged"`
	AcknowledgedBy string  `json:"acknowledged_by,omitempty"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	
	// Metadata
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
	
	// Relationships
	Tenant *Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Policy *DLPPolicy `gorm:"foreignKey:PolicyID" json:"policy,omitempty"`
	File   *File      `gorm:"foreignKey:FileID" json:"file,omitempty"`
	Chunk  *Chunk     `gorm:"foreignKey:ChunkID" json:"chunk,omitempty"`
}

// GORM hooks for JSON serialization
func (d *DLPContentRule) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), d)
}

func (d DLPContentRule) Value() (interface{}, error) {
	return json.Marshal(d)
}

func (d *DLPAction) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), d)
}

func (d DLPAction) Value() (interface{}, error) {
	return json.Marshal(d)
}

func (d *DLPConditions) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), d)
}

func (d DLPConditions) Value() (interface{}, error) {
	return json.Marshal(d)
}

// Custom slice types for GORM JSON handling
type DLPContentRules []DLPContentRule
type DLPActions []DLPAction

func (d *DLPContentRules) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), d)
}

func (d DLPContentRules) Value() (interface{}, error) {
	return json.Marshal(d)
}

func (d *DLPActions) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), d)
}

func (d DLPActions) Value() (interface{}, error) {
	return json.Marshal(d)
}

// TableName returns the table name for DLPPolicy model
func (DLPPolicy) TableName() string {
	return "dlp_policies"
}

// TableName returns the table name for DLPViolation model
func (DLPViolation) TableName() string {
	return "dlp_violations"
}

// IsActive checks if the DLP policy is active
func (d *DLPPolicy) IsActive() bool {
	return d.Enabled && d.Status == "active" && d.DeletedAt == nil
}

// GetHighestSeverityRule returns the content rule with the highest severity
func (d *DLPPolicy) GetHighestSeverityRule() *DLPContentRule {
	if len(d.ContentRules) == 0 {
		return nil
	}
	
	severityOrder := map[string]int{
		"low": 1, "medium": 2, "high": 3, "critical": 4,
	}
	
	var highest *DLPContentRule
	highestValue := 0
	
	for i := range d.ContentRules {
		rule := &d.ContentRules[i]
		if value, ok := severityOrder[rule.Sensitivity]; ok && value > highestValue {
			highest = rule
			highestValue = value
		}
	}
	
	return highest
}

// ShouldApplyToFile checks if the policy should apply to a given file
func (d *DLPPolicy) ShouldApplyToFile(fileType string, fileSize int64, dataSourceID string) bool {
	if !d.IsActive() {
		return false
	}
	
	// Check file type conditions
	if len(d.Conditions.FileTypes) > 0 {
		found := false
		for _, ft := range d.Conditions.FileTypes {
			if ft == fileType {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	// Check file size conditions
	if d.Conditions.FileSizeMin > 0 && fileSize < d.Conditions.FileSizeMin {
		return false
	}
	if d.Conditions.FileSizeMax > 0 && fileSize > d.Conditions.FileSizeMax {
		return false
	}
	
	// Check data source conditions
	if len(d.Conditions.DataSources) > 0 {
		found := false
		for _, ds := range d.Conditions.DataSources {
			if ds == dataSourceID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	return true
}

// RecordViolation records a new violation for this policy
func (d *DLPPolicy) RecordViolation() {
	d.TriggeredCount++
	now := time.Now()
	d.LastTriggered = &now
}