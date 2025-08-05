package sync

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jscharber/eAIIngest/pkg/storage"
)

// Sync types and directions
type SyncType string

const (
	SyncTypeFull        SyncType = "full"        // Complete sync of all data
	SyncTypeIncremental SyncType = "incremental" // Only sync changes since last sync
	SyncTypeRealtime    SyncType = "realtime"    // Continuous real-time sync
)

type SyncDirection string

const (
	SyncDirectionBidirectional SyncDirection = "bidirectional" // Sync both ways
	SyncDirectionDownload      SyncDirection = "download"      // Only download from source
	SyncDirectionUpload        SyncDirection = "upload"        // Only upload to source
)

// Sync states
type SyncState string

const (
	SyncStatePending   SyncState = "pending"   // Sync is queued but not started
	SyncStateRunning   SyncState = "running"   // Sync is currently in progress
	SyncStateCompleted SyncState = "completed" // Sync completed successfully
	SyncStateFailed    SyncState = "failed"    // Sync failed with errors
	SyncStateCancelled SyncState = "cancelled" // Sync was cancelled by user
	SyncStatePaused    SyncState = "paused"    // Sync is temporarily paused
)

// Sync priority levels
type SyncPriority int

const (
	SyncPriorityLow    SyncPriority = 1
	SyncPriorityNormal SyncPriority = 5
	SyncPriorityHigh   SyncPriority = 10
	SyncPriorityUrgent SyncPriority = 15
)

// StartSyncRequest contains parameters for starting a sync operation
type StartSyncRequest struct {
	DataSourceID    uuid.UUID           `json:"data_source_id"`
	ConnectorType   string              `json:"connector_type"`
	Options         *UnifiedSyncOptions `json:"options"`
	ScheduleOptions *ScheduleOptions    `json:"schedule_options,omitempty"`
}

// UnifiedSyncOptions contains configuration for sync operations
type UnifiedSyncOptions struct {
	SyncType         SyncType              `json:"sync_type"`
	Direction        SyncDirection         `json:"direction"`
	ConflictStrategy ConflictStrategy      `json:"conflict_strategy"`
	Priority         SyncPriority          `json:"priority"`
	Filters          *SyncFilters          `json:"filters,omitempty"`
	Performance      *PerformanceSettings  `json:"performance,omitempty"`
	Retry            *RetrySettings        `json:"retry,omitempty"`
	Notifications    *NotificationSettings `json:"notifications,omitempty"`
}

// SyncFilters defines what content to include/exclude from sync
type SyncFilters struct {
	// Path filters
	IncludePaths []string `json:"include_paths,omitempty"`
	ExcludePaths []string `json:"exclude_paths,omitempty"`

	// File type filters
	IncludeExtensions []string `json:"include_extensions,omitempty"`
	ExcludeExtensions []string `json:"exclude_extensions,omitempty"`

	// Size filters
	MinFileSize int64 `json:"min_file_size,omitempty"`
	MaxFileSize int64 `json:"max_file_size,omitempty"`

	// Date filters
	ModifiedAfter  *time.Time `json:"modified_after,omitempty"`
	ModifiedBefore *time.Time `json:"modified_before,omitempty"`

	// Content filters
	IncludeDeleted bool `json:"include_deleted"`
	IncludeHidden  bool `json:"include_hidden"`

	// Connector-specific filters
	CustomFilters map[string]interface{} `json:"custom_filters,omitempty"`
}

// PerformanceSettings controls sync performance characteristics
type PerformanceSettings struct {
	ConcurrentDownloads int             `json:"concurrent_downloads"`
	ConcurrentUploads   int             `json:"concurrent_uploads"`
	ChunkSize           int64           `json:"chunk_size"`
	Timeout             time.Duration   `json:"timeout"`
	RateLimit           *RateLimit      `json:"rate_limit,omitempty"`
	Bandwidth           *BandwidthLimit `json:"bandwidth,omitempty"`
}

// RateLimit defines API request rate limiting
type RateLimit struct {
	RequestsPerSecond int    `json:"requests_per_second"`
	BurstLimit        int    `json:"burst_limit"`
	BackoffStrategy   string `json:"backoff_strategy"`
}

// BandwidthLimit defines data transfer limits
type BandwidthLimit struct {
	DownloadMBps int64 `json:"download_mbps"`
	UploadMBps   int64 `json:"upload_mbps"`
}

// RetrySettings controls retry behavior for failed operations
type RetrySettings struct {
	MaxRetries      int           `json:"max_retries"`
	InitialDelay    time.Duration `json:"initial_delay"`
	MaxDelay        time.Duration `json:"max_delay"`
	BackoffFactor   float64       `json:"backoff_factor"`
	RetryableErrors []string      `json:"retryable_errors,omitempty"`
}

// NotificationSettings controls sync event notifications
type NotificationSettings struct {
	WebhookURL       string   `json:"webhook_url,omitempty"`
	EmailRecipients  []string `json:"email_recipients,omitempty"`
	SlackChannel     string   `json:"slack_channel,omitempty"`
	NotifyOnStart    bool     `json:"notify_on_start"`
	NotifyOnComplete bool     `json:"notify_on_complete"`
	NotifyOnError    bool     `json:"notify_on_error"`
	NotifyOnProgress bool     `json:"notify_on_progress"`
}

// SyncJob represents an active or completed sync operation
type SyncJob struct {
	ID            uuid.UUID                `json:"id"`
	DataSourceID  uuid.UUID                `json:"data_source_id"`
	ConnectorType string                   `json:"connector_type"`
	Connector     storage.StorageConnector `json:"-"`
	Options       *UnifiedSyncOptions      `json:"options"`
	Status        *SyncJobStatus           `json:"status"`

	// Internal fields
	cancelCtx    context.Context    `json:"-"`
	cancelFunc   context.CancelFunc `json:"-"`
	orchestrator *SyncOrchestrator  `json:"-"`
}

// SyncJobStatus contains the current status of a sync job
type SyncJobStatus struct {
	JobID        uuid.UUID     `json:"job_id"`
	State        SyncState     `json:"state"`
	StartTime    time.Time     `json:"start_time"`
	EndTime      *time.Time    `json:"end_time,omitempty"`
	Duration     time.Duration `json:"duration"`
	LastActivity time.Time     `json:"last_activity"`
	Progress     *SyncProgress `json:"progress"`
	Error        string        `json:"error,omitempty"`
	Metrics      *SyncMetrics  `json:"metrics,omitempty"`
}

// SyncProgress tracks sync operation progress
type SyncProgress struct {
	TotalFiles       int        `json:"total_files"`
	ProcessedFiles   int        `json:"processed_files"`
	FailedFiles      int        `json:"failed_files"`
	SkippedFiles     int        `json:"skipped_files"`
	PercentComplete  float64    `json:"percent_complete"`
	BytesTransferred int64      `json:"bytes_transferred"`
	TotalBytes       int64      `json:"total_bytes"`
	CurrentFile      string     `json:"current_file,omitempty"`
	EstimatedETA     *time.Time `json:"estimated_eta,omitempty"`
	TransferRate     float64    `json:"transfer_rate_mbps"`
}

// SyncMetrics contains detailed sync operation metrics
type SyncMetrics struct {
	// File operations
	FilesCreated int `json:"files_created"`
	FilesUpdated int `json:"files_updated"`
	FilesDeleted int `json:"files_deleted"`
	FilesSkipped int `json:"files_skipped"`
	FilesFailed  int `json:"files_failed"`

	// Data transfer
	BytesDownloaded int64 `json:"bytes_downloaded"`
	BytesUploaded   int64 `json:"bytes_uploaded"`

	// Performance
	AverageTransferRate float64 `json:"average_transfer_rate_mbps"`
	PeakTransferRate    float64 `json:"peak_transfer_rate_mbps"`
	TotalAPIRequests    int     `json:"total_api_requests"`
	APIRequestsPerSec   float64 `json:"api_requests_per_sec"`

	// Timing
	DiscoveryDuration  time.Duration `json:"discovery_duration"`
	TransferDuration   time.Duration `json:"transfer_duration"`
	ValidationDuration time.Duration `json:"validation_duration"`

	// Errors and retries
	TotalErrors   int            `json:"total_errors"`
	RetryAttempts int            `json:"retry_attempts"`
	ErrorsByType  map[string]int `json:"errors_by_type"`

	// Conflicts
	ConflictsDetected int `json:"conflicts_detected"`
	ConflictsResolved int `json:"conflicts_resolved"`
	ConflictsPending  int `json:"conflicts_pending"`
}

// Schedule-related types
type ScheduleOptions struct {
	Type      ScheduleType  `json:"type"`
	Interval  time.Duration `json:"interval,omitempty"`
	CronExpr  string        `json:"cron_expression,omitempty"`
	StartTime *time.Time    `json:"start_time,omitempty"`
	EndTime   *time.Time    `json:"end_time,omitempty"`
	MaxRuns   *int          `json:"max_runs,omitempty"`
}

// Filter and query types
type SyncListFilters struct {
	DataSourceID  uuid.UUID   `json:"data_source_id,omitempty"`
	ConnectorType string      `json:"connector_type,omitempty"`
	States        []SyncState `json:"states,omitempty"`
	Limit         int         `json:"limit,omitempty"`
	Offset        int         `json:"offset,omitempty"`
}

type SyncHistoryFilters struct {
	DataSourceID  uuid.UUID   `json:"data_source_id,omitempty"`
	ConnectorType string      `json:"connector_type,omitempty"`
	StartTime     *time.Time  `json:"start_time,omitempty"`
	EndTime       *time.Time  `json:"end_time,omitempty"`
	States        []SyncState `json:"states,omitempty"`
	Limit         int         `json:"limit,omitempty"`
	Offset        int         `json:"offset,omitempty"`
}

type SyncHistoryResult struct {
	Jobs       []*SyncJobStatus `json:"jobs"`
	TotalCount int              `json:"total_count"`
	HasMore    bool             `json:"has_more"`
}

// Metrics and analytics types
type SyncMetricsRequest struct {
	DataSourceID  uuid.UUID  `json:"data_source_id,omitempty"`
	ConnectorType string     `json:"connector_type,omitempty"`
	StartTime     *time.Time `json:"start_time,omitempty"`
	EndTime       *time.Time `json:"end_time,omitempty"`
	Granularity   string     `json:"granularity,omitempty"` // hour, day, week, month
	Metrics       []string   `json:"metrics,omitempty"`     // Specific metrics to include
}

type SyncMetricsResult struct {
	Summary     *SyncMetricsSummary            `json:"summary"`
	TimeSeries  []*SyncMetricsPoint            `json:"time_series,omitempty"`
	ByConnector map[string]*SyncMetricsSummary `json:"by_connector,omitempty"`
}

type SyncMetricsSummary struct {
	TotalSyncs            int           `json:"total_syncs"`
	SuccessfulSyncs       int           `json:"successful_syncs"`
	FailedSyncs           int           `json:"failed_syncs"`
	CancelledSyncs        int           `json:"cancelled_syncs"`
	SuccessRate           float64       `json:"success_rate"`
	AverageDuration       time.Duration `json:"average_duration"`
	TotalBytesTransferred int64         `json:"total_bytes_transferred"`
	TotalFilesProcessed   int           `json:"total_files_processed"`
}

type SyncMetricsPoint struct {
	Timestamp time.Time           `json:"timestamp"`
	Metrics   *SyncMetricsSummary `json:"metrics"`
}

type WebhookEvent string

const (
	WebhookEventSyncStarted      WebhookEvent = "sync.started"
	WebhookEventSyncProgress     WebhookEvent = "sync.progress"
	WebhookEventSyncCompleted    WebhookEvent = "sync.completed"
	WebhookEventSyncFailed       WebhookEvent = "sync.failed"
	WebhookEventSyncCancelled    WebhookEvent = "sync.cancelled"
	WebhookEventFileProcessed    WebhookEvent = "file.processed"
	WebhookEventConflictDetected WebhookEvent = "conflict.detected"
)

type WebhookFilters struct {
	DataSourceIDs  []uuid.UUID `json:"data_source_ids,omitempty"`
	ConnectorTypes []string    `json:"connector_types,omitempty"`
	SyncTypes      []SyncType  `json:"sync_types,omitempty"`
}

type WebhookRetryConfig struct {
	MaxRetries    int           `json:"max_retries"`
	InitialDelay  time.Duration `json:"initial_delay"`
	BackoffFactor float64       `json:"backoff_factor"`
	MaxDelay      time.Duration `json:"max_delay"`
}

type ScheduledSync struct {
	Schedule *SyncSchedule `json:"schedule"`
	Status   string        `json:"status"`
}

// Method implementations for SyncJob
func (j *SyncJob) GetStatus() *SyncJobStatus {
	if j.Status.EndTime == nil && j.Status.State != SyncStatePending && j.Status.State != SyncStateRunning {
		now := time.Now()
		j.Status.EndTime = &now
	}

	if j.Status.EndTime != nil {
		j.Status.Duration = j.Status.EndTime.Sub(j.Status.StartTime)
	} else {
		j.Status.Duration = time.Since(j.Status.StartTime)
	}

	return j.Status
}

func (j *SyncJob) Cancel() {
	if j.cancelFunc != nil {
		j.cancelFunc()
	}
	j.updateState(SyncStateCancelled)
}

func (j *SyncJob) updateState(state SyncState) {
	j.Status.State = state
	j.Status.LastActivity = time.Now()

	if state == SyncStateCompleted || state == SyncStateFailed || state == SyncStateCancelled {
		now := time.Now()
		j.Status.EndTime = &now
	}
}

// Error definitions
type SyncError struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

func (e *SyncError) Error() string {
	return e.Message
}

// Common sync errors
var (
	ErrSyncJobNotFound            = &SyncError{Code: "SYNC_JOB_NOT_FOUND", Message: "sync job not found"}
	ErrInvalidDataSourceID        = &SyncError{Code: "INVALID_DATA_SOURCE_ID", Message: "invalid data source ID"}
	ErrInvalidConnectorType       = &SyncError{Code: "INVALID_CONNECTOR_TYPE", Message: "invalid connector type"}
	ErrInvalidSyncOptions         = &SyncError{Code: "INVALID_SYNC_OPTIONS", Message: "invalid sync options"}
	ErrMaxConcurrentSyncsExceeded = &SyncError{Code: "MAX_CONCURRENT_SYNCS_EXCEEDED", Message: "maximum concurrent syncs exceeded"}
	ErrSyncAlreadyInProgress      = &SyncError{Code: "SYNC_ALREADY_IN_PROGRESS", Message: "sync already in progress for this data source"}
	ErrRealtimeSyncDisabled       = &SyncError{Code: "REALTIME_SYNC_DISABLED", Message: "real-time sync is disabled"}
	ErrInsufficientPermissions    = &SyncError{Code: "INSUFFICIENT_PERMISSIONS", Message: "insufficient permissions for sync operation"}
	ErrConnectorUnavailable       = &SyncError{Code: "CONNECTOR_UNAVAILABLE", Message: "connector is unavailable"}
	ErrSyncConfigurationInvalid   = &SyncError{Code: "SYNC_CONFIGURATION_INVALID", Message: "sync configuration is invalid"}
)
