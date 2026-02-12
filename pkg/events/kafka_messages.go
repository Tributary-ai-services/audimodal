package events

import (
	"time"
)

// Kafka topic names for the document processing pipeline
const (
	TopicPageJobs      = "audimodal.page-jobs"
	TopicPageResults   = "audimodal.page-results"
	TopicDLPJobs       = "audimodal.dlp-jobs"
	TopicEmbeddingJobs = "audimodal.embedding-jobs"
	TopicProcessingDLQ = "audimodal.processing-dlq"
)

// PageJobMessage is produced by the Splitter and consumed by OCR Workers
type PageJobMessage struct {
	MessageID      string           `json:"message_id"`
	TenantID       string           `json:"tenant_id"`
	FileID         string           `json:"file_id"`
	JobID          string           `json:"job_id"`
	PageNumber     int              `json:"page_number"`
	TotalPages     int              `json:"total_pages"`
	S3Bucket       string           `json:"s3_bucket"`
	S3Key          string           `json:"s3_key"`
	Classification string           `json:"classification"`
	Config         ExtractionConfig `json:"config"`
	ProducedAt     time.Time        `json:"produced_at"`
}

// ExtractionConfig contains OCR/extraction settings for a page job
type ExtractionConfig struct {
	OCRLanguage       string  `json:"ocr_language"`
	OCRDPI            int     `json:"ocr_dpi"`
	OCRTimeout        int     `json:"ocr_timeout_seconds"`
	TextThreshold     int     `json:"text_threshold"`
	OCRAnyImage       bool    `json:"ocr_any_image"`
	OCRImageMinWidth  int     `json:"ocr_image_min_width"`
	OCRImageMinHeight int     `json:"ocr_image_min_height"`
	MaxMemoryMB       int     `json:"max_memory_mb"`
	PreserveLayout    bool    `json:"preserve_layout"`
	LargeImagePct     float64 `json:"large_image_pct"`
}

// DefaultExtractionConfig returns default extraction configuration
func DefaultExtractionConfig() ExtractionConfig {
	return ExtractionConfig{
		OCRLanguage:       "eng",
		OCRDPI:            150,
		OCRTimeout:        300,
		TextThreshold:     100,
		OCRAnyImage:       false,
		OCRImageMinWidth:  200,
		OCRImageMinHeight: 200,
		MaxMemoryMB:       512,
		PreserveLayout:    true,
		LargeImagePct:     0.5,
	}
}

// PageResultMessage is produced by OCR Workers and consumed by the Assembler
type PageResultMessage struct {
	MessageID            string    `json:"message_id"`
	TenantID             string    `json:"tenant_id"`
	FileID               string    `json:"file_id"`
	JobID                string    `json:"job_id"`
	PageNumber           int       `json:"page_number"`
	TotalPages           int       `json:"total_pages"`
	Status               string    `json:"status"` // completed, failed
	Content              string    `json:"content,omitempty"`
	ContentS3Key         string    `json:"content_s3_key,omitempty"`
	ExtractionMethod     string    `json:"extraction_method"`
	OCRConfidence        float64   `json:"ocr_confidence"`
	ProcessingDurationMs int64     `json:"processing_duration_ms"`
	Error                string    `json:"error,omitempty"`
	ErrorType            string    `json:"error_type,omitempty"`
	WorkerPodName        string    `json:"worker_pod_name"`
	ProducedAt           time.Time `json:"produced_at"`
}

// PartitionKey returns the Kafka partition key for consistent page ordering per file
func (m *PageJobMessage) PartitionKey() string {
	return m.TenantID + ":" + m.FileID
}

// PartitionKey returns the Kafka partition key for consistent page ordering per file
func (m *PageResultMessage) PartitionKey() string {
	return m.TenantID + ":" + m.FileID
}

// DLPJobMessage is produced by the Assembler and consumed by DLP Workers.
// One message per chunk after the assembler creates chunks.
type DLPJobMessage struct {
	MessageID      string    `json:"message_id"`
	TenantID       string    `json:"tenant_id"`
	FileID         string    `json:"file_id"`
	ChunkID        string    `json:"chunk_id"`
	S3Bucket       string    `json:"s3_bucket"`
	S3Key          string    `json:"s3_key"`
	ContentPreview string    `json:"content_preview,omitempty"`
	ProducedAt     time.Time `json:"produced_at"`
}

// PartitionKey returns the Kafka partition key for DLP jobs
func (m *DLPJobMessage) PartitionKey() string {
	return m.TenantID + ":" + m.FileID
}

// EmbeddingJobMessage is produced by DLP Workers and consumed by Embedding Workers.
// Published after DLP scan completes for a chunk.
type EmbeddingJobMessage struct {
	MessageID  string    `json:"message_id"`
	TenantID   string    `json:"tenant_id"`
	FileID     string    `json:"file_id"`
	ChunkID    string    `json:"chunk_id"`
	S3Bucket   string    `json:"s3_bucket"`
	S3Key      string    `json:"s3_key"`
	ProducedAt time.Time `json:"produced_at"`
}

// PartitionKey returns the Kafka partition key for embedding jobs
func (m *EmbeddingJobMessage) PartitionKey() string {
	return m.TenantID + ":" + m.FileID
}
