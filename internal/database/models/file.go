package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// JSONBMap is a custom type for JSONB map fields that handles scanning from PostgreSQL
type JSONBMap map[string]interface{}

// Scan implements the sql.Scanner interface for JSONBMap
func (j *JSONBMap) Scan(value interface{}) error {
	if value == nil {
		*j = make(map[string]interface{})
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("cannot scan non-string/[]byte into JSONBMap")
	}

	if len(bytes) == 0 {
		*j = make(map[string]interface{})
		return nil
	}

	return json.Unmarshal(bytes, j)
}

// Value implements the driver.Valuer interface for JSONBMap
func (j JSONBMap) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return json.Marshal(j)
}

// File represents a file in the system
type File struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID            uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	DataSourceID        *uuid.UUID `gorm:"type:uuid;index" json:"data_source_id,omitempty"`
	ProcessingSessionID *uuid.UUID `gorm:"type:uuid;index" json:"processing_session_id,omitempty"`

	// File identification
	URL         string `gorm:"not null;index" json:"url"`
	Path        string `gorm:"not null;index" json:"path"`
	Filename    string `gorm:"not null;index" json:"filename"`
	Extension   string `gorm:"index" json:"extension"`
	ContentType string `gorm:"not null;index" json:"content_type"`

	// File metadata
	Size         int64     `gorm:"not null" json:"size"`
	Checksum     string    `gorm:"index" json:"checksum"`
	ChecksumType string    `gorm:"default:'md5'" json:"checksum_type"`
	LastModified time.Time `json:"last_modified"`

	// Processing status
	Status             string     `gorm:"not null;default:'discovered'" json:"status"`
	ProcessingTier     string     `gorm:"index" json:"processing_tier"` // tier1, tier2, tier3
	ProcessedAt        *time.Time `json:"processed_at,omitempty"`
	ProcessingError    *string    `json:"processing_error,omitempty"`
	ProcessingDuration *int64     `json:"processing_duration,omitempty"` // Duration in milliseconds

	// Content analysis
	Language         string          `json:"language,omitempty"`
	LanguageConf     float64         `gorm:"column:language_confidence" json:"language_confidence,omitempty"`
	ContentCategory  string          `json:"content_category,omitempty"`
	SensitivityLevel string          `json:"sensitivity_level,omitempty"`
	Classifications  Classifications `gorm:"type:jsonb" json:"classifications,omitempty"`

	// Schema and structure information
	SchemaInfo FileSchemaInfo `gorm:"type:jsonb" json:"schema_info"`

	// Chunk information
	ChunkCount       int    `gorm:"default:0" json:"chunk_count"`
	ChunkingStrategy string `json:"chunking_strategy,omitempty"`

	// Compliance and security
	PIIDetected      bool            `gorm:"column:pii_detected;default:false;index" json:"pii_detected"`
	ComplianceFlags  ComplianceFlags `gorm:"type:jsonb" json:"compliance_flags,omitempty"`
	EncryptionStatus string          `gorm:"default:'none'" json:"encryption_status"`

	// Metadata and custom fields
	Metadata     JSONBMap `gorm:"type:jsonb" json:"metadata,omitempty"`
	CustomFields JSONBMap `gorm:"type:jsonb" json:"custom_fields,omitempty"`

	// Timestamps
	CreatedAt time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time  `gorm:"not null" json:"updated_at"`
	DeletedAt *time.Time `gorm:"index" json:"deleted_at,omitempty"`

	// Relationships
	Tenant            *Tenant            `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	DataSource        *DataSource        `gorm:"foreignKey:DataSourceID" json:"data_source,omitempty"`
	ProcessingSession *ProcessingSession `gorm:"foreignKey:ProcessingSessionID" json:"processing_session,omitempty"`
	Chunks            []Chunk            `gorm:"foreignKey:FileID" json:"chunks,omitempty"`
	DLPViolations     []DLPViolation     `gorm:"foreignKey:FileID" json:"dlp_violations,omitempty"`
}

// FileSchemaInfo represents the schema information for a file
type FileSchemaInfo struct {
	Format      string                   `json:"format"`                 // structured, unstructured, semi_structured
	Encoding    string                   `json:"encoding,omitempty"`     // UTF-8, ASCII, etc.
	Delimiter   string                   `json:"delimiter,omitempty"`    // For CSV files
	Fields      []FieldInfo              `json:"fields,omitempty"`       // Field definitions
	RowCount    *int64                   `json:"row_count,omitempty"`    // Number of rows/records
	ColumnCount *int                     `json:"column_count,omitempty"` // Number of columns
	HasHeaders  bool                     `json:"has_headers,omitempty"`  // Whether file has headers
	SampleData  []map[string]interface{} `json:"sample_data,omitempty"`  // Sample records
}

// FieldInfo represents information about a field in the file
type FieldInfo struct {
	Name        string        `json:"name"`
	Type        string        `json:"type"`
	Nullable    bool          `json:"nullable"`
	Description string        `json:"description,omitempty"`
	Examples    []interface{} `json:"examples,omitempty"`
	Statistics  *FieldStats   `json:"statistics,omitempty"`
}

// FieldStats represents statistics about a field
type FieldStats struct {
	Count       int64       `json:"count"`
	Distinct    int64       `json:"distinct,omitempty"`
	Min         interface{} `json:"min,omitempty"`
	Max         interface{} `json:"max,omitempty"`
	Mean        float64     `json:"mean,omitempty"`
	Median      float64     `json:"median,omitempty"`
	StdDev      float64     `json:"std_dev,omitempty"`
	NullPercent float64     `json:"null_percent"`
}

// GORM hooks for JSON serialization
func (f *FileSchemaInfo) Scan(value interface{}) error {
	if value == nil {
		*f = FileSchemaInfo{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("cannot scan non-string/[]byte into FileSchemaInfo")
	}

	if len(bytes) == 0 {
		*f = FileSchemaInfo{}
		return nil
	}

	return json.Unmarshal(bytes, f)
}

func (f FileSchemaInfo) Value() (driver.Value, error) {
	return json.Marshal(f)
}

func (f *FieldInfo) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), f)
}

func (f FieldInfo) Value() (interface{}, error) {
	return json.Marshal(f)
}

func (f *FieldStats) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), f)
}

func (f FieldStats) Value() (interface{}, error) {
	return json.Marshal(f)
}

// Custom slice types for GORM JSON handling
type Classifications []string
type ComplianceFlags []string

func (c *Classifications) Scan(value interface{}) error {
	if value == nil {
		*c = Classifications{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("cannot scan non-string/[]byte into Classifications")
	}

	if len(bytes) == 0 {
		*c = Classifications{}
		return nil
	}

	return json.Unmarshal(bytes, c)
}

func (c Classifications) Value() (driver.Value, error) {
	return json.Marshal(c)
}

func (c *ComplianceFlags) Scan(value interface{}) error {
	if value == nil {
		*c = ComplianceFlags{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("cannot scan non-string/[]byte into ComplianceFlags")
	}

	if len(bytes) == 0 {
		*c = ComplianceFlags{}
		return nil
	}

	return json.Unmarshal(bytes, c)
}

func (c ComplianceFlags) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// TableName returns the table name for File model
func (File) TableName() string {
	return "files"
}

// IsProcessed checks if the file has been processed
func (f *File) IsProcessed() bool {
	return f.Status == "processed" || f.Status == "completed"
}

// IsProcessing checks if the file is currently being processed
func (f *File) IsProcessing() bool {
	return f.Status == "processing"
}

// HasError checks if the file has processing errors
func (f *File) HasError() bool {
	return f.Status == "error" || f.Status == "failed"
}

// GetProcessingTier returns the processing tier for the file
func (f *File) GetProcessingTier() string {
	if f.ProcessingTier != "" {
		return f.ProcessingTier
	}

	// Determine tier based on file size
	if f.Size < 10*1024*1024 { // < 10MB
		return "tier1"
	} else if f.Size < 1024*1024*1024 { // < 1GB
		return "tier2"
	} else {
		return "tier3"
	}
}

// GetFileExtension returns the file extension
func (f *File) GetFileExtension() string {
	return f.Extension
}

// IsStructuredData checks if the file contains structured data
func (f *File) IsStructuredData() bool {
	return f.SchemaInfo.Format == "structured"
}

// IsUnstructuredData checks if the file contains unstructured data
func (f *File) IsUnstructuredData() bool {
	return f.SchemaInfo.Format == "unstructured"
}

// IsSemiStructuredData checks if the file contains semi-structured data
func (f *File) IsSemiStructuredData() bool {
	return f.SchemaInfo.Format == "semi_structured"
}

// HasPII checks if the file contains personally identifiable information
func (f *File) HasPII() bool {
	return f.PIIDetected
}

// GetSensitivityLevel returns the sensitivity level of the file
func (f *File) GetSensitivityLevel() string {
	if f.SensitivityLevel == "" {
		return "unknown"
	}
	return f.SensitivityLevel
}

// AddClassification adds a classification to the file
func (f *File) AddClassification(classification string) {
	for _, c := range f.Classifications {
		if c == classification {
			return // Already exists
		}
	}
	f.Classifications = append(f.Classifications, classification)
}

// AddComplianceFlag adds a compliance flag to the file
func (f *File) AddComplianceFlag(flag string) {
	for _, c := range f.ComplianceFlags {
		if c == flag {
			return // Already exists
		}
	}
	f.ComplianceFlags = append(f.ComplianceFlags, flag)
}

// MarkAsProcessed marks the file as processed
func (f *File) MarkAsProcessed(chunkCount int, strategy string) {
	f.Status = "processed"
	f.ChunkCount = chunkCount
	f.ChunkingStrategy = strategy
	now := time.Now()
	f.ProcessedAt = &now
}

// MarkAsError marks the file as having an error
func (f *File) MarkAsError(errorMsg string) {
	f.Status = "error"
	f.ProcessingError = &errorMsg
	now := time.Now()
	f.ProcessedAt = &now
}

// StartProcessing marks the file as being processed
func (f *File) StartProcessing() {
	f.Status = "processing"
	f.ProcessingTier = f.GetProcessingTier()
}

// CalculateProcessingDuration calculates and stores the processing duration
func (f *File) CalculateProcessingDuration(startTime time.Time) {
	if f.ProcessedAt != nil {
		duration := f.ProcessedAt.Sub(startTime).Milliseconds()
		f.ProcessingDuration = &duration
	}
}
