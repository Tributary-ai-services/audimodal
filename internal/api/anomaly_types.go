package api

import (
	"time"

	"github.com/google/uuid"
	"github.com/jscharber/eAIIngest/pkg/anomaly"
)

// Request types

// DetectAnomaliesRequest represents a request for anomaly detection
type DetectAnomaliesRequest struct {
	TenantID     uuid.UUID              `json:"tenant_id" binding:"required"`
	DataSourceID *uuid.UUID             `json:"data_source_id,omitempty"`
	DocumentID   *uuid.UUID             `json:"document_id,omitempty"`
	ChunkID      *uuid.UUID             `json:"chunk_id,omitempty"`
	UserID       *uuid.UUID             `json:"user_id,omitempty"`
	Content      string                 `json:"content"`
	ContentType  string                 `json:"content_type,omitempty"`
	FileSize     int64                  `json:"file_size,omitempty"`
	FileName     string                 `json:"file_name,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// DetectAnomaliesAsyncRequest represents an async anomaly detection request
type DetectAnomaliesAsyncRequest struct {
	DetectAnomaliesRequest
	Priority    int    `json:"priority"`
	CallbackURL string `json:"callback_url,omitempty"`
}

// UpdateStatusRequest represents a request to update anomaly status
type UpdateStatusRequest struct {
	Status anomaly.AnomalyStatus `json:"status" binding:"required"`
	Notes  string                `json:"notes,omitempty"`
}

// BulkUpdateStatusRequest represents a bulk status update request
type BulkUpdateStatusRequest struct {
	AnomalyIDs []uuid.UUID           `json:"anomaly_ids" binding:"required"`
	Status     anomaly.AnomalyStatus `json:"status" binding:"required"`
	Notes      string                `json:"notes,omitempty"`
}

// BulkDetectRequest represents a bulk detection request
type BulkDetectRequest struct {
	Requests []DetectAnomaliesRequest `json:"requests" binding:"required"`
}

// Response types

// DetectionResponse represents the response from anomaly detection
type DetectionResponse struct {
	RequestID      uuid.UUID         `json:"request_id"`
	Anomalies      []AnomalyResponse `json:"anomalies"`
	ProcessedAt    time.Time         `json:"processed_at"`
	ProcessingTime time.Duration     `json:"processing_time"`
	TotalAnomalies int               `json:"total_anomalies"`
}

// AnomalyResponse represents an anomaly in API responses
type AnomalyResponse struct {
	ID           uuid.UUID               `json:"id"`
	Type         anomaly.AnomalyType     `json:"type"`
	Severity     anomaly.AnomalySeverity `json:"severity"`
	Status       anomaly.AnomalyStatus   `json:"status"`
	Title        string                  `json:"title"`
	Description  string                  `json:"description"`
	DetectedAt   time.Time               `json:"detected_at"`
	Score        float64                 `json:"score"`
	Confidence   float64                 `json:"confidence"`
	DetectorName string                  `json:"detector_name"`
	Metadata     map[string]interface{}  `json:"metadata,omitempty"`
}

// AsyncDetectionResponse represents the response from async detection submission
type AsyncDetectionResponse struct {
	RequestID           uuid.UUID `json:"request_id"`
	Status              string    `json:"status"`
	SubmittedAt         time.Time `json:"submitted_at"`
	EstimatedCompletion time.Time `json:"estimated_completion"`
}

// ListAnomaliesResponse represents the response from listing anomalies
type ListAnomaliesResponse struct {
	Anomalies []AnomalyListItem `json:"anomalies"`
	Total     int               `json:"total"`
	Limit     int               `json:"limit"`
	Offset    int               `json:"offset"`
}

// AnomalyListItem represents an anomaly in list responses
type AnomalyListItem struct {
	ID           uuid.UUID               `json:"id"`
	Type         anomaly.AnomalyType     `json:"type"`
	Severity     anomaly.AnomalySeverity `json:"severity"`
	Status       anomaly.AnomalyStatus   `json:"status"`
	Title        string                  `json:"title"`
	DetectedAt   time.Time               `json:"detected_at"`
	Score        float64                 `json:"score"`
	Confidence   float64                 `json:"confidence"`
	DetectorName string                  `json:"detector_name"`
}

// AnomalyDetailResponse represents detailed anomaly information
type AnomalyDetailResponse struct {
	ID                uuid.UUID                    `json:"id"`
	Type              anomaly.AnomalyType          `json:"type"`
	Severity          anomaly.AnomalySeverity      `json:"severity"`
	Status            anomaly.AnomalyStatus        `json:"status"`
	Title             string                       `json:"title"`
	Description       string                       `json:"description"`
	DetectedAt        time.Time                    `json:"detected_at"`
	UpdatedAt         time.Time                    `json:"updated_at"`
	ResolvedAt        *time.Time                   `json:"resolved_at,omitempty"`
	TenantID          uuid.UUID                    `json:"tenant_id"`
	DataSourceID      *uuid.UUID                   `json:"data_source_id,omitempty"`
	DocumentID        *uuid.UUID                   `json:"document_id,omitempty"`
	ChunkID           *uuid.UUID                   `json:"chunk_id,omitempty"`
	UserID            *uuid.UUID                   `json:"user_id,omitempty"`
	Score             float64                      `json:"score"`
	Confidence        float64                      `json:"confidence"`
	Threshold         float64                      `json:"threshold"`
	Baseline          map[string]interface{}       `json:"baseline,omitempty"`
	Detected          map[string]interface{}       `json:"detected"`
	Metadata          map[string]interface{}       `json:"metadata,omitempty"`
	DetectorName      string                       `json:"detector_name"`
	DetectorVersion   string                       `json:"detector_version"`
	RuleName          string                       `json:"rule_name,omitempty"`
	ResolutionNotes   string                       `json:"resolution_notes,omitempty"`
	Actions           []anomaly.AnomalyAction      `json:"actions,omitempty"`
	NotificationsSent []anomaly.NotificationRecord `json:"notifications_sent,omitempty"`
}

// BulkUpdateStatusResponse represents the response from bulk status update
type BulkUpdateStatusResponse struct {
	Results []BulkUpdateResult `json:"results"`
	Total   int                `json:"total"`
}

// BulkUpdateResult represents the result of a single bulk update operation
type BulkUpdateResult struct {
	AnomalyID uuid.UUID `json:"anomaly_id"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

// BulkDetectResponse represents the response from bulk detection
type BulkDetectResponse struct {
	Results []BulkDetectionResult `json:"results"`
	Total   int                   `json:"total"`
}

// BulkDetectionResult represents the result of a single bulk detection operation
type BulkDetectionResult struct {
	Index          int           `json:"index"`
	Success        bool          `json:"success"`
	Anomalies      int           `json:"anomalies"`
	ProcessingTime time.Duration `json:"processing_time,omitempty"`
	Error          string        `json:"error,omitempty"`
}

// Statistics and reporting types

// AnomalyTrendsResponse represents anomaly trends over time
type AnomalyTrendsResponse struct {
	Trends []TrendDataPoint `json:"trends"`
}

// TrendDataPoint represents a single data point in trend analysis
type TrendDataPoint struct {
	Date              string                      `json:"date"`
	Count             int                         `json:"count"`
	SeverityBreakdown map[string]int              `json:"severity_breakdown"`
	TypeBreakdown     map[anomaly.AnomalyType]int `json:"type_breakdown"`
}

// AnomalySummaryResponse represents a summary of anomalies
type AnomalySummaryResponse struct {
	Summary AnomalySummary `json:"summary"`
}

// AnomalySummary contains summary statistics
type AnomalySummary struct {
	TotalAnomalies    int                             `json:"total_anomalies"`
	Resolved          int                             `json:"resolved"`
	Pending           int                             `json:"pending"`
	FalsePositives    int                             `json:"false_positives"`
	ByType            map[anomaly.AnomalyType]int     `json:"by_type"`
	BySeverity        map[anomaly.AnomalySeverity]int `json:"by_severity"`
	ByStatus          map[anomaly.AnomalyStatus]int   `json:"by_status"`
	TopSources        []string                        `json:"top_sources"`
	ResolutionRate    float64                         `json:"resolution_rate"`
	AvgResolutionTime time.Duration                   `json:"avg_resolution_time"`
}

// DetectorInfoResponse represents detector information
type DetectorInfoResponse struct {
	Detectors map[string]DetectorInfo `json:"detectors"`
}

// DetectorInfo contains information about a detector
type DetectorInfo struct {
	Name           string                 `json:"name"`
	Version        string                 `json:"version"`
	Enabled        bool                   `json:"enabled"`
	SupportedTypes []anomaly.AnomalyType  `json:"supported_types"`
	Configuration  map[string]interface{} `json:"configuration,omitempty"`
	LastUpdated    time.Time              `json:"last_updated,omitempty"`
	Statistics     DetectorStatistics     `json:"statistics,omitempty"`
}

// DetectorStatistics contains statistics for a detector
type DetectorStatistics struct {
	TotalDetections   int64         `json:"total_detections"`
	AverageScore      float64       `json:"average_score"`
	AverageConfidence float64       `json:"average_confidence"`
	ProcessingTime    time.Duration `json:"avg_processing_time"`
	FalsePositiveRate float64       `json:"false_positive_rate"`
	LastRun           time.Time     `json:"last_run"`
}

// Notification types

// NotificationHistoryResponse represents notification history
type NotificationHistoryResponse struct {
	AnomalyID     uuid.UUID                    `json:"anomaly_id"`
	Notifications []anomaly.NotificationRecord `json:"notifications"`
}

// Configuration types

// DetectorConfigRequest represents a detector configuration request
type DetectorConfigRequest struct {
	Configuration map[string]interface{} `json:"configuration" binding:"required"`
}

// BaselineUpdateRequest represents a baseline update request
type BaselineUpdateRequest struct {
	Baseline anomaly.BaselineData `json:"baseline" binding:"required"`
}

// Error response types

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Error   string                 `json:"error"`
	Details string                 `json:"details,omitempty"`
	Code    string                 `json:"code,omitempty"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// ValidationError represents validation error details
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Value   string `json:"value,omitempty"`
}

// ValidationErrorResponse represents validation error response
type ValidationErrorResponse struct {
	Error  string            `json:"error"`
	Errors []ValidationError `json:"errors"`
}

// Search and filter types

// AnomalySearchRequest represents an anomaly search request
type AnomalySearchRequest struct {
	Query        string                    `json:"query"`
	Types        []anomaly.AnomalyType     `json:"types,omitempty"`
	Severities   []anomaly.AnomalySeverity `json:"severities,omitempty"`
	Statuses     []anomaly.AnomalyStatus   `json:"statuses,omitempty"`
	DateRange    *DateRange                `json:"date_range,omitempty"`
	ScoreRange   *ScoreRange               `json:"score_range,omitempty"`
	TenantID     *uuid.UUID                `json:"tenant_id,omitempty"`
	DataSourceID *uuid.UUID                `json:"data_source_id,omitempty"`
	Detectors    []string                  `json:"detectors,omitempty"`
	Limit        int                       `json:"limit,omitempty"`
	Offset       int                       `json:"offset,omitempty"`
}

// DateRange represents a date range filter
type DateRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ScoreRange represents a score range filter
type ScoreRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// AnomalySearchResponse represents the response from anomaly search
type AnomalySearchResponse struct {
	Anomalies []AnomalyListItem `json:"anomalies"`
	Total     int               `json:"total"`
	Took      time.Duration     `json:"took"`
	Facets    SearchFacets      `json:"facets,omitempty"`
}

// SearchFacets represents search result facets
type SearchFacets struct {
	Types      map[anomaly.AnomalyType]int     `json:"types"`
	Severities map[anomaly.AnomalySeverity]int `json:"severities"`
	Statuses   map[anomaly.AnomalyStatus]int   `json:"statuses"`
	Detectors  map[string]int                  `json:"detectors"`
}

// Export types

// ExportRequest represents an export request
type ExportRequest struct {
	Format   string                `json:"format" binding:"required"` // csv, json, xlsx
	Filter   *AnomalySearchRequest `json:"filter,omitempty"`
	Fields   []string              `json:"fields,omitempty"`
	Filename string                `json:"filename,omitempty"`
}

// ExportResponse represents an export response
type ExportResponse struct {
	ExportID    uuid.UUID `json:"export_id"`
	Status      string    `json:"status"`
	Format      string    `json:"format"`
	RequestedAt time.Time `json:"requested_at"`
	DownloadURL string    `json:"download_url,omitempty"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
}

// Webhook types

// WebhookRequest represents a webhook configuration request
type WebhookRequest struct {
	URL     string            `json:"url" binding:"required"`
	Events  []string          `json:"events" binding:"required"`
	Filters *WebhookFilters   `json:"filters,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Secret  string            `json:"secret,omitempty"`
	Active  bool              `json:"active"`
}

// WebhookFilters represents webhook filtering options
type WebhookFilters struct {
	Types      []anomaly.AnomalyType     `json:"types,omitempty"`
	Severities []anomaly.AnomalySeverity `json:"severities,omitempty"`
	TenantIDs  []uuid.UUID               `json:"tenant_ids,omitempty"`
}

// WebhookResponse represents a webhook configuration response
type WebhookResponse struct {
	ID        uuid.UUID       `json:"id"`
	URL       string          `json:"url"`
	Events    []string        `json:"events"`
	Filters   *WebhookFilters `json:"filters,omitempty"`
	Active    bool            `json:"active"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	LastFired *time.Time      `json:"last_fired,omitempty"`
}

// WebhookEvent represents a webhook event payload
type WebhookEvent struct {
	ID        uuid.UUID              `json:"id"`
	Event     string                 `json:"event"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
	Anomaly   *AnomalyResponse       `json:"anomaly,omitempty"`
}

// Health check types

// HealthCheckResponse represents a health check response
type HealthCheckResponse struct {
	Status    string                 `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Version   string                 `json:"version"`
	Uptime    time.Duration          `json:"uptime"`
	Checks    map[string]HealthCheck `json:"checks"`
}

// HealthCheck represents an individual health check
type HealthCheck struct {
	Status  string        `json:"status"`
	Message string        `json:"message,omitempty"`
	Took    time.Duration `json:"took"`
}

// Metrics types

// MetricsResponse represents system metrics
type MetricsResponse struct {
	Timestamp time.Time         `json:"timestamp"`
	Metrics   map[string]Metric `json:"metrics"`
}

// Metric represents a system metric
type Metric struct {
	Value       float64           `json:"value"`
	Unit        string            `json:"unit"`
	Description string            `json:"description"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// Rate limiting types

// RateLimitInfo represents rate limit information
type RateLimitInfo struct {
	Limit      int            `json:"limit"`
	Remaining  int            `json:"remaining"`
	ResetAt    time.Time      `json:"reset_at"`
	RetryAfter *time.Duration `json:"retry_after,omitempty"`
}
