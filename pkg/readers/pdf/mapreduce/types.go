// Package mapreduce implements a memory-efficient map-reduce style PDF processing pipeline.
// It addresses OOM issues with large PDFs by:
// 1. Classifying pages before extraction to skip OCR on text-only pages
// 2. Using subprocess isolation for OCR to allow OS memory reclamation
// 3. Storing intermediate results to database immediately
package mapreduce

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// PageClassificationType represents the type of content on a PDF page
type PageClassificationType string

const (
	// ClassTextOnly indicates a page with extractable text (use pdftotext)
	ClassTextOnly PageClassificationType = "text_only"
	// ClassImageOnly indicates a page with only images (needs full OCR)
	ClassImageOnly PageClassificationType = "image_only"
	// ClassHybrid indicates a page with both text and images
	ClassHybrid PageClassificationType = "hybrid"
	// ClassScanned indicates a scanned page (no fonts, needs full OCR)
	ClassScanned PageClassificationType = "scanned"
	// ClassEmpty indicates an empty or nearly empty page (skip)
	ClassEmpty PageClassificationType = "empty"
	// ClassUnknown indicates classification has not been performed
	ClassUnknown PageClassificationType = "unknown"
)

// ExtractionMethod represents how content was extracted from a page
type ExtractionMethod string

const (
	// MethodPDFToText uses pdftotext for fast text extraction
	MethodPDFToText ExtractionMethod = "pdftotext"
	// MethodOCR uses tesseract for optical character recognition
	MethodOCR ExtractionMethod = "ocr"
	// MethodHybrid uses both pdftotext and OCR
	MethodHybrid ExtractionMethod = "hybrid"
	// MethodSkipped indicates the page was skipped
	MethodSkipped ExtractionMethod = "skipped"
)

// ProcessingStatus represents the current processing state of a page
type ProcessingStatus string

const (
	StatusPending     ProcessingStatus = "pending"
	StatusClassifying ProcessingStatus = "classifying"
	StatusExtracting  ProcessingStatus = "extracting"
	StatusCompleted   ProcessingStatus = "completed"
	StatusFailed      ProcessingStatus = "failed"
	StatusSkipped     ProcessingStatus = "skipped"
)

// PageClassification contains the results of page classification
type PageClassification struct {
	PageNumber       int                    `json:"page_number"`
	Classification   PageClassificationType `json:"classification"`
	Confidence       float64                `json:"confidence"`
	HasEmbeddedFonts bool                   `json:"has_embedded_fonts"`
	HasImages        bool                   `json:"has_images"`
	ImageCount       int                    `json:"image_count"`
	LargeImageCount  int                    `json:"large_image_count"` // Images > 50% of page size
	TextCharCount    int                    `json:"text_char_count"`   // From quick pdftotext
}

// NeedsOCR returns true if the page classification indicates OCR is required
func (pc *PageClassification) NeedsOCR() bool {
	switch pc.Classification {
	case ClassScanned, ClassImageOnly:
		return true
	case ClassHybrid:
		return true
	default:
		return false
	}
}

// PageJob represents a job for processing a single PDF page
type PageJob struct {
	// Identification
	JobID     string     `json:"job_id"`
	TenantID  uuid.UUID  `json:"tenant_id"`
	FileID    uuid.UUID  `json:"file_id"`
	SessionID *uuid.UUID `json:"session_id,omitempty"`

	// Page info
	PDFPath        string                 `json:"pdf_path"`
	PageNumber     int                    `json:"page_number"`
	TotalPages     int                    `json:"total_pages"`
	Classification PageClassificationType `json:"classification"`

	// Configuration
	Config *ExtractionConfig `json:"config"`
}

// ExtractionConfig contains configuration for page extraction
type ExtractionConfig struct {
	// OCR settings
	OCRLanguage string `json:"ocr_language"` // e.g., "eng", "spa", "fra"
	OCRDPI      int    `json:"ocr_dpi"`      // e.g., 150, 200, 300
	OCRTimeout  int    `json:"ocr_timeout"`  // Timeout in seconds

	// Classification settings
	TextThreshold       int `json:"text_threshold"`        // Min chars for text_only (default: 100)
	LargeImageThreshold int `json:"large_image_threshold"` // % of page for "large" (default: 50)

	// Image detection settings for OCR triggering
	OCRAnyImage       bool `json:"ocr_any_image"`        // If true, trigger OCR for ANY image on page (regardless of size)
	OCRImageMinWidth  int  `json:"ocr_image_min_width"`  // Minimum image width in pixels to trigger OCR (default: 200)
	OCRImageMinHeight int  `json:"ocr_image_min_height"` // Minimum image height in pixels to trigger OCR (default: 200)

	// Memory settings
	MaxMemoryMB int `json:"max_memory_mb"` // Memory limit per worker

	// Feature flags
	PreserveLayout bool `json:"preserve_layout"` // Use -layout flag in pdftotext
	ExtractImages  bool `json:"extract_images"`  // Extract embedded images
}

// DefaultExtractionConfig returns the default extraction configuration
func DefaultExtractionConfig() *ExtractionConfig {
	return &ExtractionConfig{
		OCRLanguage:         "eng",
		OCRDPI:              150, // Lower = less memory
		OCRTimeout:          300, // 5 minutes per page
		TextThreshold:       100, // 100 chars to classify as text_only
		LargeImageThreshold: 50,  // 50% of page = large image
		OCRAnyImage:         false,
		OCRImageMinWidth:    200, // Min 200px width to trigger OCR
		OCRImageMinHeight:   200, // Min 200px height to trigger OCR
		MaxMemoryMB:         1024,
		PreserveLayout:      true,
		ExtractImages:       false, // Disabled by default for memory
	}
}

// PageResult represents the result of processing a single page
type PageResult struct {
	// Identification
	JobID      string    `json:"job_id"`
	TenantID   uuid.UUID `json:"tenant_id"`
	FileID     uuid.UUID `json:"file_id"`
	PageNumber int       `json:"page_number"`
	TotalPages int       `json:"total_pages"`

	// Classification
	Classification           PageClassificationType `json:"classification"`
	ClassificationConfidence float64                `json:"classification_confidence"`

	// Extraction results
	Content          string           `json:"content"`
	ContentHash      string           `json:"content_hash,omitempty"`
	ExtractionMethod ExtractionMethod `json:"extraction_method"`
	OCRConfidence    float64          `json:"ocr_confidence,omitempty"`
	OCRLanguage      string           `json:"ocr_language,omitempty"`
	OCRDPI           int              `json:"ocr_dpi,omitempty"`

	// Status
	Status ProcessingStatus `json:"status"`
	Error  string           `json:"error,omitempty"`

	// Metrics
	ProcessingDurationMs int64 `json:"processing_duration_ms"`
	PeakMemoryBytes      int64 `json:"peak_memory_bytes,omitempty"`
	ImageSizeBytes       int64 `json:"image_size_bytes,omitempty"`

	// Metadata
	WorkerID    string    `json:"worker_id,omitempty"`
	ProcessedAt time.Time `json:"processed_at"`
}

// IsSuccess returns true if the page was successfully processed
func (pr *PageResult) IsSuccess() bool {
	return pr.Status == StatusCompleted
}

// IsFailed returns true if the page processing failed
func (pr *PageResult) IsFailed() bool {
	return pr.Status == StatusFailed
}

// CoordinatorConfig contains configuration for the map-reduce coordinator
type CoordinatorConfig struct {
	// Worker pool settings
	MaxConcurrentWorkers int `json:"max_concurrent_workers"` // Default: 4
	WorkerTimeoutSeconds int `json:"worker_timeout_seconds"` // Default: 300

	// Retry settings
	MaxRetries int `json:"max_retries"` // Default: 3
	RetryDelay int `json:"retry_delay"` // Seconds between retries

	// Checkpoint settings
	CheckpointInterval int `json:"checkpoint_interval"` // Save progress every N pages

	// Mode selection threshold
	MapReducePageThreshold int `json:"mapreduce_page_threshold"` // Use map-reduce for > N pages

	// Extraction config
	Extraction *ExtractionConfig `json:"extraction"`
}

// DefaultCoordinatorConfig returns the default coordinator configuration
func DefaultCoordinatorConfig() *CoordinatorConfig {
	return &CoordinatorConfig{
		MaxConcurrentWorkers:   4,
		WorkerTimeoutSeconds:   300,
		MaxRetries:             3,
		RetryDelay:             5,
		CheckpointInterval:     10,
		MapReducePageThreshold: 50,
		Extraction:             DefaultExtractionConfig(),
	}
}

// ProcessingResult contains the overall result of processing a PDF
type ProcessingResult struct {
	FileID     uuid.UUID `json:"file_id"`
	TotalPages int       `json:"total_pages"`

	// Counts by classification
	TextOnlyPages int `json:"text_only_pages"`
	OCRPages      int `json:"ocr_pages"`
	SkippedPages  int `json:"skipped_pages"`
	FailedPages   int `json:"failed_pages"`

	// Aggregated content stats
	TotalCharacters int `json:"total_characters"`
	TotalDuration   int `json:"total_duration_ms"`

	// Per-page results (for detailed analysis)
	PageResults []*PageResult `json:"page_results,omitempty"`

	// Errors
	Errors []ProcessingError `json:"errors,omitempty"`
}

// ProcessingError represents an error during processing
type ProcessingError struct {
	PageNumber int    `json:"page_number"`
	Error      string `json:"error"`
	Retryable  bool   `json:"retryable"`
	RetryCount int    `json:"retry_count"`
}

// PageClassifier is the interface for classifying PDF pages
type PageClassifier interface {
	// ClassifyPage determines the type of content on a page
	ClassifyPage(ctx context.Context, pdfPath string, pageNum int) (*PageClassification, error)

	// ClassifyAllPages classifies all pages in a PDF
	ClassifyAllPages(ctx context.Context, pdfPath string, totalPages int) ([]*PageClassification, error)
}

// PageExtractor is the interface for extracting content from PDF pages
type PageExtractor interface {
	// ExtractPage extracts content from a single page
	ExtractPage(ctx context.Context, job *PageJob) (*PageResult, error)
}

// WorkerPool is the interface for managing extraction workers
type WorkerPool interface {
	// ProcessPage sends a job to a worker and returns the result
	ProcessPage(ctx context.Context, job *PageJob) (*PageResult, error)

	// Shutdown gracefully shuts down the worker pool
	Shutdown(ctx context.Context) error
}

// PageResultStore is the interface for storing page results
type PageResultStore interface {
	// SavePageResult saves a page result to storage
	SavePageResult(ctx context.Context, result *PageResult) error

	// GetPageResult retrieves a page result
	GetPageResult(ctx context.Context, fileID uuid.UUID, pageNum int) (*PageResult, error)

	// GetAllPageResults retrieves all page results for a file
	GetAllPageResults(ctx context.Context, fileID uuid.UUID) ([]*PageResult, error)

	// GetPendingPages returns pages that need processing
	GetPendingPages(ctx context.Context, fileID uuid.UUID) ([]int, error)

	// GetFailedPages returns pages that failed and can be retried
	GetFailedPages(ctx context.Context, fileID uuid.UUID, maxRetries int) ([]int, error)
}

// Coordinator orchestrates the map-reduce PDF processing
type Coordinator interface {
	// Process processes a PDF using the map-reduce pipeline
	Process(ctx context.Context, pdfPath string, tenantID, fileID uuid.UUID) (*ProcessingResult, error)

	// Resume resumes processing from a checkpoint
	Resume(ctx context.Context, fileID uuid.UUID) (*ProcessingResult, error)
}
