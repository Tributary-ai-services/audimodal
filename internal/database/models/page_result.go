package models

import (
	"time"

	"github.com/google/uuid"
)

// PageResult status constants
const (
	PageStatusPending     = "pending"
	PageStatusClassifying = "classifying"
	PageStatusExtracting  = "extracting"
	PageStatusCompleted   = "completed"
	PageStatusFailed      = "failed"
	PageStatusSkipped     = "skipped"
)

// PageClassification type constants
const (
	PageClassTextOnly  = "text_only"
	PageClassImageOnly = "image_only"
	PageClassHybrid    = "hybrid"
	PageClassScanned   = "scanned"
	PageClassEmpty     = "empty"
	PageClassUnknown   = "unknown"
)

// Extraction method constants
const (
	ExtractionMethodPDFToText = "pdftotext"
	ExtractionMethodOCR       = "ocr"
	ExtractionMethodHybrid    = "hybrid"
	ExtractionMethodSkipped   = "skipped"
	ExtractionMethodPending   = "pending"
)

// PageResult represents intermediate page-level extraction results
// for the map-reduce PDF processing pipeline
type PageResult struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID            uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	FileID              uuid.UUID  `gorm:"type:uuid;not null;index" json:"file_id"`
	ProcessingSessionID *uuid.UUID `gorm:"type:uuid;index" json:"processing_session_id,omitempty"`

	// Page identification
	PageNumber int `gorm:"not null" json:"page_number"`
	TotalPages int `gorm:"not null" json:"total_pages"`

	// Classification (determined before extraction)
	Classification           string   `gorm:"not null" json:"classification"`
	ClassificationConfidence *float64 `json:"classification_confidence,omitempty"`
	HasEmbeddedFonts         bool     `gorm:"default:false" json:"has_embedded_fonts"`
	HasImages                bool     `gorm:"default:false" json:"has_images"`
	ImageCount               int      `gorm:"default:0" json:"image_count"`
	LargeImageCount          int      `gorm:"default:0" json:"large_image_count"`
	TextCharCount            int      `gorm:"default:0" json:"text_char_count"`

	// Extraction results
	Content          string   `gorm:"type:text" json:"content,omitempty"`
	ContentHash      string   `json:"content_hash,omitempty"`
	ExtractionMethod string   `gorm:"not null;default:'pending'" json:"extraction_method"`
	OCRConfidence    *float64 `json:"ocr_confidence,omitempty"`
	OCRLanguage      string   `json:"ocr_language,omitempty"`
	OCRDPI           int      `json:"ocr_dpi,omitempty"`

	// Processing metadata
	ProcessingStatus      string     `gorm:"not null;default:'pending'" json:"processing_status"`
	ProcessingError       *string    `json:"processing_error,omitempty"`
	ProcessingStartedAt   *time.Time `json:"processing_started_at,omitempty"`
	ProcessingCompletedAt *time.Time `json:"processing_completed_at,omitempty"`
	ProcessingDurationMs  *int       `json:"processing_duration_ms,omitempty"`
	WorkerID              string     `json:"worker_id,omitempty"`
	RetryCount            int        `gorm:"default:0" json:"retry_count"`
	MaxRetries            int        `gorm:"default:3" json:"max_retries"`

	// Memory and performance metrics
	PeakMemoryBytes int64 `json:"peak_memory_bytes,omitempty"`
	ImageSizeBytes  int64 `json:"image_size_bytes,omitempty"`

	// Timestamps
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`

	// Relationships (for GORM)
	Tenant            *Tenant            `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	File              *File              `gorm:"foreignKey:FileID" json:"file,omitempty"`
	ProcessingSession *ProcessingSession `gorm:"foreignKey:ProcessingSessionID" json:"processing_session,omitempty"`
}

// TableName returns the table name for PageResult model
func (PageResult) TableName() string {
	return "page_results"
}

// IsCompleted checks if the page has been successfully processed
func (p *PageResult) IsCompleted() bool {
	return p.ProcessingStatus == PageStatusCompleted
}

// IsFailed checks if the page processing failed
func (p *PageResult) IsFailed() bool {
	return p.ProcessingStatus == PageStatusFailed
}

// IsSkipped checks if the page was skipped
func (p *PageResult) IsSkipped() bool {
	return p.ProcessingStatus == PageStatusSkipped
}

// IsPending checks if the page is pending processing
func (p *PageResult) IsPending() bool {
	return p.ProcessingStatus == PageStatusPending
}

// IsProcessing checks if the page is currently being processed
func (p *PageResult) IsProcessing() bool {
	return p.ProcessingStatus == PageStatusClassifying || p.ProcessingStatus == PageStatusExtracting
}

// CanRetry checks if the page can be retried
func (p *PageResult) CanRetry() bool {
	return p.ProcessingStatus == PageStatusFailed && p.RetryCount < p.MaxRetries
}

// NeedsOCR checks if the page requires OCR processing
func (p *PageResult) NeedsOCR() bool {
	switch p.Classification {
	case PageClassScanned, PageClassImageOnly:
		return true
	case PageClassHybrid:
		return true
	default:
		return false
	}
}

// MarkClassifying marks the page as being classified
func (p *PageResult) MarkClassifying(workerID string) {
	p.ProcessingStatus = PageStatusClassifying
	now := time.Now()
	p.ProcessingStartedAt = &now
	p.WorkerID = workerID
}

// MarkExtracting marks the page as being extracted
func (p *PageResult) MarkExtracting(workerID string) {
	p.ProcessingStatus = PageStatusExtracting
	if p.ProcessingStartedAt == nil {
		now := time.Now()
		p.ProcessingStartedAt = &now
	}
	p.WorkerID = workerID
}

// SetClassification sets the classification result for the page
func (p *PageResult) SetClassification(classification string, confidence float64, hasFonts, hasImages bool, imageCount, largeImageCount, textCharCount int) {
	p.Classification = classification
	p.ClassificationConfidence = &confidence
	p.HasEmbeddedFonts = hasFonts
	p.HasImages = hasImages
	p.ImageCount = imageCount
	p.LargeImageCount = largeImageCount
	p.TextCharCount = textCharCount
}

// MarkCompleted marks the page as successfully completed
func (p *PageResult) MarkCompleted(content string, method string, confidence float64) {
	p.ProcessingStatus = PageStatusCompleted
	p.Content = content
	p.ExtractionMethod = method
	p.OCRConfidence = &confidence
	now := time.Now()
	p.ProcessingCompletedAt = &now
	if p.ProcessingStartedAt != nil {
		duration := int(now.Sub(*p.ProcessingStartedAt).Milliseconds())
		p.ProcessingDurationMs = &duration
	}
}

// MarkFailed marks the page as failed
func (p *PageResult) MarkFailed(err error) {
	p.ProcessingStatus = PageStatusFailed
	errStr := err.Error()
	p.ProcessingError = &errStr
	now := time.Now()
	p.ProcessingCompletedAt = &now
	p.RetryCount++
}

// MarkSkipped marks the page as skipped (e.g., empty page)
func (p *PageResult) MarkSkipped(reason string) {
	p.ProcessingStatus = PageStatusSkipped
	p.ExtractionMethod = ExtractionMethodSkipped
	p.ProcessingError = &reason
	now := time.Now()
	p.ProcessingCompletedAt = &now
	if p.ProcessingStartedAt != nil {
		duration := int(now.Sub(*p.ProcessingStartedAt).Milliseconds())
		p.ProcessingDurationMs = &duration
	}
}

// PrepareForRetry resets the page for retry
func (p *PageResult) PrepareForRetry() {
	p.ProcessingStatus = PageStatusPending
	p.ProcessingError = nil
	p.ProcessingStartedAt = nil
	p.ProcessingCompletedAt = nil
	p.ProcessingDurationMs = nil
	p.WorkerID = ""
	// Keep RetryCount incremented from MarkFailed
}

// GetContentLength returns the length of extracted content
func (p *PageResult) GetContentLength() int {
	return len(p.Content)
}

// HasContent checks if the page has extracted content
func (p *PageResult) HasContent() bool {
	return len(p.Content) > 0
}

// SetMetrics sets memory and performance metrics
func (p *PageResult) SetMetrics(peakMemory, imageSize int64) {
	p.PeakMemoryBytes = peakMemory
	p.ImageSizeBytes = imageSize
}

// SetOCRParams sets OCR parameters used for extraction
func (p *PageResult) SetOCRParams(language string, dpi int) {
	p.OCRLanguage = language
	p.OCRDPI = dpi
}

// NewPageResult creates a new PageResult for a given page
func NewPageResult(tenantID, fileID uuid.UUID, pageNumber, totalPages int) *PageResult {
	return &PageResult{
		TenantID:         tenantID,
		FileID:           fileID,
		PageNumber:       pageNumber,
		TotalPages:       totalPages,
		Classification:   PageClassUnknown,
		ExtractionMethod: ExtractionMethodPending,
		ProcessingStatus: PageStatusPending,
		MaxRetries:       3,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
}

// NewPageResultWithSession creates a new PageResult with a processing session
func NewPageResultWithSession(tenantID, fileID uuid.UUID, sessionID *uuid.UUID, pageNumber, totalPages int) *PageResult {
	pr := NewPageResult(tenantID, fileID, pageNumber, totalPages)
	pr.ProcessingSessionID = sessionID
	return pr
}
