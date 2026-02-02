package k3s

import (
	"errors"
	"time"

	"github.com/jscharber/audimodal/pkg/dlp/types"
)

// Errors
var (
	ErrMissingAPIKey  = errors.New("AUDIMODAL_API_KEY environment variable is required")
	ErrMissingBaseURL = errors.New("AUDIMODAL_URL is required")
	ErrNotFound       = errors.New("resource not found")
	ErrUnauthorized   = errors.New("unauthorized - check API key")
	ErrTimeout        = errors.New("operation timed out")
)

// FileUploadRequest represents a file upload request
type FileUploadRequest struct {
	Filename    string            `json:"filename"`
	ContentType string            `json:"content_type"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// FileResponse represents the API response for a file
type FileResponse struct {
	FileID      string    `json:"file_id"`
	TenantID    string    `json:"tenant_id"`
	Filename    string    `json:"filename"`
	Status      string    `json:"status"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// FileStatus represents the processing status of a file
type FileStatus struct {
	FileID      string    `json:"file_id"`
	Status      string    `json:"status"` // pending, processing, completed, failed
	Error       string    `json:"error,omitempty"`
	ProcessedAt time.Time `json:"processed_at,omitempty"`
	Progress    int       `json:"progress,omitempty"` // 0-100
}

// ProcessingStatus constants
const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

// Violation represents a compliance violation found during scanning
type Violation struct {
	Rule        string        `json:"rule"`        // e.g., "GDPR-001"
	Regulation  string        `json:"regulation"`  // e.g., "GDPR"
	Severity    string        `json:"severity"`    // critical, high, medium, low
	PIIType     types.PIIType `json:"pii_type"`    // e.g., "email", "ssn"
	Description string        `json:"description"`
	FindingIDs  []string      `json:"finding_ids"`
}

// ViolationsResponse represents the API response for violations
type ViolationsResponse struct {
	FileID      string      `json:"file_id"`
	TenantID    string      `json:"tenant_id"`
	IsCompliant bool        `json:"is_compliant"`
	Violations  []Violation `json:"violations"`
	ScannedAt   time.Time   `json:"scanned_at"`
}

// Finding represents a PII finding from the scan
type Finding struct {
	ID         string        `json:"id"`
	Type       types.PIIType `json:"type"`
	Value      string        `json:"value"`
	Confidence float64       `json:"confidence"`
	StartPos   int           `json:"start_pos"`
	EndPos     int           `json:"end_pos"`
	Context    string        `json:"context"`
	RiskLevel  string        `json:"risk_level"`
}

// ScanResult represents the scan result from the API
type ScanResult struct {
	FileID       string    `json:"file_id"`
	TenantID     string    `json:"tenant_id"`
	TotalMatches int       `json:"total_matches"`
	Findings     []Finding `json:"findings"`
	RiskScore    float64   `json:"risk_score"`
	IsCompliant  bool      `json:"is_compliant"`
	ScannedAt    time.Time `json:"scanned_at"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

// APIError represents an error response from the API
type APIError struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    int    `json:"code,omitempty"`
}
