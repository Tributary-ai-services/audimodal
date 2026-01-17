// Package models contains all database models for the Audimodal.ai platform.
// These models represent the core entities in the system and provide
// a foundation for multi-tenant document processing with compliance features.
package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// Common constants for model validation
const (
	// Status values for tenants
	TenantStatusActive    = "active"
	TenantStatusSuspended = "suspended"
	TenantStatusDeleted   = "deleted"

	// Status values for data sources
	DataSourceStatusActive   = "active"
	DataSourceStatusInactive = "inactive"
	DataSourceStatusError    = "error"

	// Status values for processing sessions
	SessionStatusPending             = "pending"
	SessionStatusRunning             = "running"
	SessionStatusCompleted           = "completed"
	SessionStatusFailed              = "failed"
	SessionStatusCancelled           = "cancelled"
	SessionStatusCompletedWithErrors = "completed_with_errors"

	// Status values for files
	FileStatusDiscovered = "discovered"
	FileStatusProcessing = "processing"
	FileStatusProcessed  = "processed"
	FileStatusCompleted  = "completed"
	FileStatusError      = "error"
	FileStatusFailed     = "failed"

	// Processing tiers
	ProcessingTier1 = "tier1" // < 10MB
	ProcessingTier2 = "tier2" // 10MB - 1GB
	ProcessingTier3 = "tier3" // > 1GB

	// Chunk types
	ChunkTypeText  = "text"
	ChunkTypeTable = "table"
	ChunkTypeImage = "image"
	ChunkTypeCode  = "code"
	ChunkTypeOther = "other"

	// Embedding status
	EmbeddingStatusPending    = "pending"
	EmbeddingStatusProcessing = "processing"
	EmbeddingStatusCompleted  = "completed"
	EmbeddingStatusFailed     = "failed"
	EmbeddingStatusSkipped    = "skipped"

	// DLP scan status
	DLPScanStatusPending    = "pending"
	DLPScanStatusProcessing = "processing"
	DLPScanStatusCompleted  = "completed"
	DLPScanStatusFailed     = "failed"
	DLPScanStatusSkipped    = "skipped"

	// Sensitivity levels
	SensitivityLevelUnknown  = "unknown"
	SensitivityLevelLow      = "low"
	SensitivityLevelMedium   = "medium"
	SensitivityLevelHigh     = "high"
	SensitivityLevelCritical = "critical"

	// DLP violation severity
	ViolationSeverityLow      = "low"
	ViolationSeverityMedium   = "medium"
	ViolationSeverityHigh     = "high"
	ViolationSeverityCritical = "critical"

	// File formats
	FormatStructured     = "structured"
	FormatUnstructured   = "unstructured"
	FormatSemiStructured = "semi_structured"
)

// JSONMap is a helper type for JSONB fields
type JSONMap map[string]interface{}

// Scan implements the sql.Scanner interface for JSONMap
func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = make(JSONMap)
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into JSONMap", value)
	}

	return json.Unmarshal(bytes, j)
}

// Value implements the driver.Valuer interface for JSONMap
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// StringSlice is a helper type for JSONB string arrays
type StringSlice []string

// Scan implements the sql.Scanner interface for StringSlice
func (s *StringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = make(StringSlice, 0)
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into StringSlice", value)
	}

	return json.Unmarshal(bytes, s)
}

// Value implements the driver.Valuer interface for StringSlice
func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return json.Marshal([]string{})
	}
	return json.Marshal(s)
}

// Contains checks if the slice contains a specific string
func (s StringSlice) Contains(item string) bool {
	for _, v := range s {
		if v == item {
			return true
		}
	}
	return false
}

// Add adds a string to the slice if it doesn't already exist
func (s *StringSlice) Add(item string) {
	if !s.Contains(item) {
		*s = append(*s, item)
	}
}

// Remove removes a string from the slice
func (s *StringSlice) Remove(item string) {
	for i, v := range *s {
		if v == item {
			*s = append((*s)[:i], (*s)[i+1:]...)
			break
		}
	}
}

// ValidationError represents a model validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (v ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", v.Field, v.Message)
}

// ValidationErrors represents multiple validation errors
type ValidationErrors []ValidationError

func (v ValidationErrors) Error() string {
	if len(v) == 0 {
		return "no validation errors"
	}
	if len(v) == 1 {
		return v[0].Error()
	}
	return fmt.Sprintf("%d validation errors: %s (and %d more)", len(v), v[0].Error(), len(v)-1)
}

// HasErrors returns true if there are validation errors
func (v ValidationErrors) HasErrors() bool {
	return len(v) > 0
}

// Add adds a validation error
func (v *ValidationErrors) Add(field, message string) {
	*v = append(*v, ValidationError{Field: field, Message: message})
}

// Model represents the base model interface
type Model interface {
	Validate() ValidationErrors
	TableName() string
}

// GetValidTenantStatuses returns all valid tenant status values
func GetValidTenantStatuses() []string {
	return []string{TenantStatusActive, TenantStatusSuspended, TenantStatusDeleted}
}

// GetValidDataSourceStatuses returns all valid data source status values
func GetValidDataSourceStatuses() []string {
	return []string{DataSourceStatusActive, DataSourceStatusInactive, DataSourceStatusError}
}

// GetValidSessionStatuses returns all valid processing session status values
func GetValidSessionStatuses() []string {
	return []string{
		SessionStatusPending, SessionStatusRunning, SessionStatusCompleted,
		SessionStatusFailed, SessionStatusCancelled, SessionStatusCompletedWithErrors,
	}
}

// GetValidFileStatuses returns all valid file status values
func GetValidFileStatuses() []string {
	return []string{
		FileStatusDiscovered, FileStatusProcessing, FileStatusProcessed,
		FileStatusCompleted, FileStatusError, FileStatusFailed,
	}
}

// GetValidProcessingTiers returns all valid processing tier values
func GetValidProcessingTiers() []string {
	return []string{ProcessingTier1, ProcessingTier2, ProcessingTier3}
}

// GetValidChunkTypes returns all valid chunk type values
func GetValidChunkTypes() []string {
	return []string{ChunkTypeText, ChunkTypeTable, ChunkTypeImage, ChunkTypeCode, ChunkTypeOther}
}

// GetValidSensitivityLevels returns all valid sensitivity level values
func GetValidSensitivityLevels() []string {
	return []string{
		SensitivityLevelUnknown, SensitivityLevelLow, SensitivityLevelMedium,
		SensitivityLevelHigh, SensitivityLevelCritical,
	}
}

// GetValidViolationSeverities returns all valid DLP violation severity values
func GetValidViolationSeverities() []string {
	return []string{
		ViolationSeverityLow, ViolationSeverityMedium,
		ViolationSeverityHigh, ViolationSeverityCritical,
	}
}

// IsValidStatus checks if a status value is valid for the given model type
func IsValidStatus(modelType, status string) bool {
	switch modelType {
	case "tenant":
		return contains(GetValidTenantStatuses(), status)
	case "data_source":
		return contains(GetValidDataSourceStatuses(), status)
	case "session":
		return contains(GetValidSessionStatuses(), status)
	case "file":
		return contains(GetValidFileStatuses(), status)
	default:
		return false
	}
}

// IsValidProcessingTier checks if a processing tier value is valid
func IsValidProcessingTier(tier string) bool {
	return contains(GetValidProcessingTiers(), tier)
}

// IsValidChunkType checks if a chunk type value is valid
func IsValidChunkType(chunkType string) bool {
	return contains(GetValidChunkTypes(), chunkType)
}

// IsValidSensitivityLevel checks if a sensitivity level value is valid
func IsValidSensitivityLevel(level string) bool {
	return contains(GetValidSensitivityLevels(), level)
}

// IsValidViolationSeverity checks if a violation severity value is valid
func IsValidViolationSeverity(severity string) bool {
	return contains(GetValidViolationSeverities(), severity)
}

// contains checks if a slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
