package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Tenant represents a tenant in the multi-tenant system
type Tenant struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Name        string    `gorm:"unique;not null;index" json:"name"`
	DisplayName string    `gorm:"not null" json:"display_name"`

	// Billing information
	BillingPlan  string `gorm:"not null" json:"billing_plan"`
	BillingEmail string `gorm:"not null" json:"billing_email"`

	// Quotas (stored as JSON for flexibility)
	Quotas TenantQuotas `gorm:"type:jsonb" json:"quotas"`

	// Compliance requirements
	Compliance TenantCompliance `gorm:"type:jsonb" json:"compliance"`

	// Contact information
	ContactInfo TenantContactInfo `gorm:"type:jsonb" json:"contact_info"`

	// Status and metadata
	Status    string     `gorm:"not null;default:'active'" json:"status"`
	CreatedAt time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time  `gorm:"not null" json:"updated_at"`
	DeletedAt *time.Time `gorm:"index" json:"deleted_at,omitempty"`

	// Relationships
	DataSources        []DataSource        `gorm:"foreignKey:TenantID" json:"data_sources,omitempty"`
	ProcessingSessions []ProcessingSession `gorm:"foreignKey:TenantID" json:"processing_sessions,omitempty"`
	DLPPolicies        []DLPPolicy         `gorm:"foreignKey:TenantID" json:"dlp_policies,omitempty"`
	Files              []File              `gorm:"foreignKey:TenantID" json:"files,omitempty"`
	Chunks             []Chunk             `gorm:"foreignKey:TenantID" json:"chunks,omitempty"`
}

// TenantQuotas represents the quotas for a tenant
type TenantQuotas struct {
	FilesPerHour         int64 `json:"files_per_hour"`
	StorageGB            int64 `json:"storage_gb"`
	ComputeHours         int64 `json:"compute_hours"`
	APIRequestsPerMinute int64 `json:"api_requests_per_minute"`
	MaxConcurrentJobs    int64 `json:"max_concurrent_jobs"`
	MaxFileSize          int64 `json:"max_file_size"`
	MaxChunksPerFile     int64 `json:"max_chunks_per_file"`
	VectorStorageGB      int64 `json:"vector_storage_gb"`
}

// TenantCompliance represents compliance requirements for a tenant
type TenantCompliance struct {
	GDPR               bool     `json:"gdpr"`
	HIPAA              bool     `json:"hipaa"`
	SOX                bool     `json:"sox"`
	PCI                bool     `json:"pci"`
	DataResidency      []string `json:"data_residency"`
	RetentionDays      int      `json:"retention_days"`
	EncryptionRequired bool     `json:"encryption_required"`
}

// TenantContactInfo represents contact information for a tenant
type TenantContactInfo struct {
	AdminEmail     string `json:"admin_email"`
	SecurityEmail  string `json:"security_email"`
	BillingEmail   string `json:"billing_email"`
	TechnicalEmail string `json:"technical_email"`
}

// GORM hooks for JSON serialization
func (t *TenantQuotas) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), t)
}

func (t TenantQuotas) Value() (interface{}, error) {
	return json.Marshal(t)
}

func (t *TenantCompliance) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), t)
}

func (t TenantCompliance) Value() (interface{}, error) {
	return json.Marshal(t)
}

func (t *TenantContactInfo) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), t)
}

func (t TenantContactInfo) Value() (interface{}, error) {
	return json.Marshal(t)
}

// TableName returns the table name for Tenant model
func (Tenant) TableName() string {
	return "tenants"
}

// IsActive checks if the tenant is active
func (t *Tenant) IsActive() bool {
	return t.Status == "active" && t.DeletedAt == nil
}

// HasComplianceRequirement checks if tenant has a specific compliance requirement
func (t *Tenant) HasComplianceRequirement(requirement string) bool {
	switch requirement {
	case "gdpr":
		return t.Compliance.GDPR
	case "hipaa":
		return t.Compliance.HIPAA
	case "sox":
		return t.Compliance.SOX
	case "pci":
		return t.Compliance.PCI
	default:
		return false
	}
}

// GetQuotaLimit returns the quota limit for a specific resource
func (t *Tenant) GetQuotaLimit(resource string) int64 {
	switch resource {
	case "files_per_hour":
		return t.Quotas.FilesPerHour
	case "storage_gb":
		return t.Quotas.StorageGB
	case "compute_hours":
		return t.Quotas.ComputeHours
	case "api_requests_per_minute":
		return t.Quotas.APIRequestsPerMinute
	case "max_concurrent_jobs":
		return t.Quotas.MaxConcurrentJobs
	case "max_file_size":
		return t.Quotas.MaxFileSize
	case "max_chunks_per_file":
		return t.Quotas.MaxChunksPerFile
	case "vector_storage_gb":
		return t.Quotas.VectorStorageGB
	default:
		return 0
	}
}

// Validate validates the tenant model
func (t *Tenant) Validate() ValidationErrors {
	var errors ValidationErrors

	// Validate required fields
	if t.Name == "" {
		errors.Add("name", "name is required")
	}

	if t.DisplayName == "" {
		errors.Add("display_name", "display name is required")
	}

	if t.BillingPlan == "" {
		errors.Add("billing_plan", "billing plan is required")
	}

	if t.BillingEmail == "" {
		errors.Add("billing_email", "billing email is required")
	}

	// Validate status
	if !IsValidStatus("tenant", t.Status) {
		errors.Add("status", "invalid status value")
	}

	// Validate quotas
	if t.Quotas.FilesPerHour < 0 {
		errors.Add("quotas.files_per_hour", "files per hour must be non-negative")
	}

	if t.Quotas.StorageGB < 0 {
		errors.Add("quotas.storage_gb", "storage GB must be non-negative")
	}

	// Validate email format for contact info
	if t.ContactInfo.AdminEmail == "" {
		errors.Add("contact_info.admin_email", "admin email is required")
	}

	return errors
}
