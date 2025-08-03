package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ProcessingSession represents a processing session for a set of files
type ProcessingSession struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Name        string    `gorm:"not null;index" json:"name"`
	DisplayName string    `gorm:"not null" json:"display_name"`

	// Session configuration
	FileSpecs []ProcessingFileSpec `gorm:"type:jsonb" json:"file_specs"`
	Options   ProcessingOptions    `gorm:"type:jsonb" json:"options"`

	// Status and progress
	Status         string  `gorm:"not null;default:'pending'" json:"status"`
	Progress       float64 `gorm:"default:0" json:"progress"`
	TotalFiles     int     `gorm:"default:0" json:"total_files"`
	ProcessedFiles int     `gorm:"default:0" json:"processed_files"`
	FailedFiles    int     `gorm:"default:0" json:"failed_files"`

	// Timing
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"not null" json:"updated_at"`
	DeletedAt   *time.Time `gorm:"index" json:"deleted_at,omitempty"`

	// Error handling
	LastError  *string `json:"last_error,omitempty"`
	ErrorCount int     `gorm:"default:0" json:"error_count"`
	RetryCount int     `gorm:"default:0" json:"retry_count"`
	MaxRetries int     `gorm:"default:3" json:"max_retries"`

	// Resource usage
	ProcessedBytes int64 `gorm:"default:0" json:"processed_bytes"`
	TotalBytes     int64 `gorm:"default:0" json:"total_bytes"`
	ChunksCreated  int64 `gorm:"default:0" json:"chunks_created"`

	// Relationships
	Tenant *Tenant `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Files  []File  `gorm:"foreignKey:ProcessingSessionID" json:"session_files,omitempty"`
}

// ProcessingFileSpec represents a file specification for processing
type ProcessingFileSpec struct {
	URL         string            `json:"url"`
	Size        int64             `json:"size"`
	ContentType string            `json:"content_type"`
	Checksum    string            `json:"checksum,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ProcessingOptions represents options for processing a session
type ProcessingOptions struct {
	ChunkingStrategy   string                 `json:"chunking_strategy"`
	EmbeddingTypes     []string               `json:"embedding_types"`
	DLPScanEnabled     bool                   `json:"dlp_scan_enabled"`
	Priority           string                 `json:"priority"`
	CustomOptions      map[string]interface{} `json:"custom_options,omitempty"`
	MaxChunkSize       int                    `json:"max_chunk_size,omitempty"`
	OverlapSize        int                    `json:"overlap_size,omitempty"`
	ParallelProcessing bool                   `json:"parallel_processing"`
	BatchSize          int                    `json:"batch_size"`
}

// GORM hooks for JSON serialization
func (p *ProcessingFileSpec) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), p)
}

func (p ProcessingFileSpec) Value() (interface{}, error) {
	return json.Marshal(p)
}

func (p *ProcessingOptions) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), p)
}

func (p ProcessingOptions) Value() (interface{}, error) {
	return json.Marshal(p)
}

// Custom slice types for GORM JSON handling
type ProcessingFileSpecs []ProcessingFileSpec

func (p *ProcessingFileSpecs) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), p)
}

func (p ProcessingFileSpecs) Value() (interface{}, error) {
	return json.Marshal(p)
}

// TableName returns the table name for ProcessingSession model
func (ProcessingSession) TableName() string {
	return "processing_sessions"
}

// IsActive checks if the processing session is active
func (p *ProcessingSession) IsActive() bool {
	return p.Status != "completed" && p.Status != "failed" && p.Status != "cancelled" && p.DeletedAt == nil
}

// IsCompleted checks if the processing session is completed
func (p *ProcessingSession) IsCompleted() bool {
	return p.Status == "completed"
}

// IsFailed checks if the processing session has failed
func (p *ProcessingSession) IsFailed() bool {
	return p.Status == "failed"
}

// GetProgress calculates the progress percentage
func (p *ProcessingSession) GetProgress() float64 {
	if p.TotalFiles == 0 {
		return 0.0
	}
	return float64(p.ProcessedFiles) / float64(p.TotalFiles) * 100.0
}

// GetPriority returns the processing priority
func (p *ProcessingSession) GetPriority() string {
	if p.Options.Priority == "" {
		return "normal"
	}
	return p.Options.Priority
}

// GetBatchSize returns the batch size for processing
func (p *ProcessingSession) GetBatchSize() int {
	if p.Options.BatchSize <= 0 {
		return 10 // Default batch size
	}
	return p.Options.BatchSize
}

// CanRetry checks if the session can be retried
func (p *ProcessingSession) CanRetry() bool {
	return p.RetryCount < p.MaxRetries && p.Status == "failed"
}

// UpdateProgress updates the progress and related fields
func (p *ProcessingSession) UpdateProgress(processedFiles, failedFiles int, processedBytes int64) {
	p.ProcessedFiles = processedFiles
	p.FailedFiles = failedFiles
	p.ProcessedBytes = processedBytes
	p.Progress = p.GetProgress()

	// Update status based on progress
	if p.ProcessedFiles+p.FailedFiles >= p.TotalFiles {
		if p.FailedFiles == 0 {
			p.Status = "completed"
			now := time.Now()
			p.CompletedAt = &now
		} else if p.ProcessedFiles == 0 {
			p.Status = "failed"
			now := time.Now()
			p.CompletedAt = &now
		} else {
			p.Status = "completed_with_errors"
			now := time.Now()
			p.CompletedAt = &now
		}
	}
}

// Start marks the session as started
func (p *ProcessingSession) Start() {
	p.Status = "running"
	now := time.Now()
	p.StartedAt = &now
}

// Fail marks the session as failed
func (p *ProcessingSession) Fail(errorMsg string) {
	p.Status = "failed"
	p.LastError = &errorMsg
	p.ErrorCount++
	now := time.Now()
	p.CompletedAt = &now
}

// Cancel marks the session as cancelled
func (p *ProcessingSession) Cancel() {
	p.Status = "cancelled"
	now := time.Now()
	p.CompletedAt = &now
}
