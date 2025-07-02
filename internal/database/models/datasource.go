package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// DataSource represents a data source configuration for a tenant
type DataSource struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Name        string    `gorm:"not null;index" json:"name"`
	DisplayName string    `gorm:"not null" json:"display_name"`
	Type        string    `gorm:"not null;index" json:"type"` // s3, gcs, azure, gdrive, etc.
	
	// Configuration (stored as JSON for flexibility)
	Config DataSourceConfig `gorm:"type:jsonb" json:"config"`
	
	// Credentials reference
	CredentialsRef DataSourceCredentials `gorm:"type:jsonb" json:"credentials_ref"`
	
	// Sync settings
	SyncSettings DataSourceSyncSettings `gorm:"type:jsonb" json:"sync_settings"`
	
	// Processing settings
	ProcessingSettings DataSourceProcessingSettings `gorm:"type:jsonb" json:"processing_settings"`
	
	// Status and metadata
	Status          string    `gorm:"not null;default:'active'" json:"status"`
	LastSyncAt      *time.Time `json:"last_sync_at,omitempty"`
	LastSyncStatus  string    `gorm:"default:'pending'" json:"last_sync_status"`
	LastSyncError   *string   `json:"last_sync_error,omitempty"`
	CreatedAt       time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time `gorm:"not null" json:"updated_at"`
	DeletedAt       *time.Time `gorm:"index" json:"deleted_at,omitempty"`
	
	// Relationships
	Tenant *Tenant `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Files  []File  `gorm:"foreignKey:DataSourceID" json:"files,omitempty"`
}

// DataSourceConfig represents the configuration for a data source
type DataSourceConfig struct {
	// Common fields
	Region   string `json:"region,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	
	// S3/GCS/Azure specific
	Bucket    string `json:"bucket,omitempty"`
	Container string `json:"container,omitempty"`
	Path      string `json:"path,omitempty"`
	
	// Google Drive specific
	DriveID string `json:"drive_id,omitempty"`
	
	// SharePoint specific
	SiteURL string `json:"site_url,omitempty"`
	
	// Database specific
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Database string `json:"database,omitempty"`
	
	// File system specific
	RootPath string `json:"root_path,omitempty"`
	
	// Additional custom configuration
	CustomConfig map[string]interface{} `json:"custom_config,omitempty"`
}

// DataSourceCredentials represents credentials reference for a data source
type DataSourceCredentials struct {
	SecretName      string `json:"secret_name"`
	SecretNamespace string `json:"secret_namespace"`
	SecretKey       string `json:"secret_key,omitempty"`
	CredentialType  string `json:"credential_type"` // oauth2, api_key, certificate, etc.
}

// DataSourceSyncSettings represents sync settings for a data source
type DataSourceSyncSettings struct {
	Enabled         bool   `json:"enabled"`
	Schedule        string `json:"schedule"`         // Cron expression
	IncrementalSync bool   `json:"incremental_sync"`
	BatchSize       int    `json:"batch_size"`
	MaxDepth        int    `json:"max_depth"`        // Directory traversal depth
	FilePattern     string `json:"file_pattern"`     // Regex pattern for file matching
	ExcludePattern  string `json:"exclude_pattern"`  // Regex pattern for file exclusion
	SizeLimit       int64  `json:"size_limit"`       // Maximum file size in bytes
}

// DataSourceProcessingSettings represents processing settings for a data source
type DataSourceProcessingSettings struct {
	AutoClassify       bool     `json:"auto_classify"`
	DLPScanEnabled     bool     `json:"dlp_scan_enabled"`
	EmbeddingTypes     []string `json:"embedding_types"`
	ChunkingStrategy   string   `json:"chunking_strategy"`
	PreprocessingSteps []string `json:"preprocessing_steps"`
	Priority           string   `json:"priority"` // low, normal, high, critical
}

// GORM hooks for JSON serialization
func (d *DataSourceConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), d)
}

func (d DataSourceConfig) Value() (interface{}, error) {
	return json.Marshal(d)
}

func (d *DataSourceCredentials) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), d)
}

func (d DataSourceCredentials) Value() (interface{}, error) {
	return json.Marshal(d)
}

func (d *DataSourceSyncSettings) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), d)
}

func (d DataSourceSyncSettings) Value() (interface{}, error) {
	return json.Marshal(d)
}

func (d *DataSourceProcessingSettings) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), d)
}

func (d DataSourceProcessingSettings) Value() (interface{}, error) {
	return json.Marshal(d)
}

// TableName returns the table name for DataSource model
func (DataSource) TableName() string {
	return "data_sources"
}

// IsActive checks if the data source is active
func (d *DataSource) IsActive() bool {
	return d.Status == "active" && d.DeletedAt == nil
}

// IsSyncEnabled checks if sync is enabled for the data source
func (d *DataSource) IsSyncEnabled() bool {
	return d.SyncSettings.Enabled
}

// GetCredentialType returns the credential type for the data source
func (d *DataSource) GetCredentialType() string {
	return d.CredentialsRef.CredentialType
}

// GetBatchSize returns the batch size for sync operations
func (d *DataSource) GetBatchSize() int {
	if d.SyncSettings.BatchSize <= 0 {
		return 100 // Default batch size
	}
	return d.SyncSettings.BatchSize
}

// GetPriority returns the processing priority for the data source
func (d *DataSource) GetPriority() string {
	if d.ProcessingSettings.Priority == "" {
		return "normal"
	}
	return d.ProcessingSettings.Priority
}

// ShouldProcessFile checks if a file should be processed based on patterns
func (d *DataSource) ShouldProcessFile(filename string, size int64) bool {
	// Check size limit
	if d.SyncSettings.SizeLimit > 0 && size > d.SyncSettings.SizeLimit {
		return false
	}
	
	// TODO: Add regex pattern matching for file inclusion/exclusion
	// This would require importing regexp package
	
	return true
}