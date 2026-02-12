package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Processing job statuses
const (
	JobStatusSplitting  = "splitting"
	JobStatusProcessing = "processing"
	JobStatusAssembling = "assembling"
	JobStatusCompleted  = "completed"
	JobStatusPartial    = "processed_partial"
	JobStatusFailed     = "failed"
	JobStatusTimedOut   = "timed_out"
)

// Pipeline page result statuses
const (
	PipelinePagePending    = "pending"
	PipelinePageProcessing = "processing"
	PipelinePageCompleted  = "completed"
	PipelinePageFailed     = "failed"
)

// Error types for page processing failures
const (
	ErrorTypeCrash           = "crash"
	ErrorTypeProcessingError = "processing_error"
	ErrorTypeHungProcess     = "hung_process"
	ErrorTypeS3Error         = "s3_error"
	ErrorTypeKafkaError      = "kafka_error"
)

// ProcessingJob tracks Kafka pipeline processing for a file
type ProcessingJob struct {
	ID             uuid.UUID       `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID       `gorm:"type:uuid;not null" json:"tenant_id"`
	FileID         uuid.UUID       `gorm:"type:uuid;not null" json:"file_id"`
	TotalPages     int             `gorm:"not null" json:"total_pages"`
	CompletedPages int             `gorm:"default:0" json:"completed_pages"`
	FailedPages    int             `gorm:"default:0" json:"failed_pages"`
	Status         string          `gorm:"not null;default:'splitting'" json:"status"`
	CreatedAt      time.Time       `gorm:"not null" json:"created_at"`
	UpdatedAt      time.Time       `gorm:"not null" json:"updated_at"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
	ErrorMessage   *string         `json:"error_message,omitempty"`
	Metadata       JobMetadata     `gorm:"type:jsonb" json:"metadata,omitempty"`

	// Relationships
	File        *File              `gorm:"foreignKey:FileID" json:"file,omitempty"`
	PageResults []PipelinePageResult `gorm:"foreignKey:JobID" json:"page_results,omitempty"`
}

// JobMetadata stores additional processing metadata
type JobMetadata map[string]interface{}

func (m *JobMetadata) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), m)
}

func (m JobMetadata) Value() (interface{}, error) {
	return json.Marshal(m)
}

// TableName returns the table name
func (ProcessingJob) TableName() string {
	return "processing_jobs"
}

// IsComplete checks if all pages are done
func (j *ProcessingJob) IsComplete() bool {
	return j.CompletedPages+j.FailedPages >= j.TotalPages
}

// FailureRate returns the fraction of pages that failed
func (j *ProcessingJob) FailureRate() float64 {
	if j.TotalPages == 0 {
		return 0
	}
	return float64(j.FailedPages) / float64(j.TotalPages)
}

// PipelinePageResult tracks individual page processing in the Kafka pipeline
type PipelinePageResult struct {
	ID                   uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	JobID                uuid.UUID  `gorm:"type:uuid;not null" json:"job_id"`
	TenantID             uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	FileID               uuid.UUID  `gorm:"type:uuid;not null" json:"file_id"`
	PageNumber           int        `gorm:"not null" json:"page_number"`
	Status               string     `gorm:"not null;default:'pending'" json:"status"`
	ContentS3Key         string     `gorm:"column:content_s3_key" json:"content_s3_key,omitempty"`
	ExtractionMethod     string     `json:"extraction_method,omitempty"`
	OCRConfidence        *float64   `json:"ocr_confidence,omitempty"`
	ProcessingDurationMs *int64     `json:"processing_duration_ms,omitempty"`
	ErrorType            string     `json:"error_type,omitempty"`
	ErrorMessage         *string    `json:"error_message,omitempty"`
	CreatedAt            time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt            time.Time  `gorm:"not null" json:"updated_at"`

	// Relationships
	Job *ProcessingJob `gorm:"foreignKey:JobID" json:"job,omitempty"`
}

// TableName returns the table name
func (PipelinePageResult) TableName() string {
	return "pipeline_page_results"
}
