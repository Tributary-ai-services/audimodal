package googledrive

import (
	"context"
	"sync"
	"time"
)

// SyncState tracks the synchronization state
type SyncState struct {
	IsRunning        bool          `json:"is_running"`
	LastSyncStart    time.Time     `json:"last_sync_start"`
	LastSyncEnd      time.Time     `json:"last_sync_end"`
	LastSyncDuration time.Duration `json:"last_sync_duration"`
	TotalFiles       int64         `json:"total_files"`
	ChangedFiles     int64         `json:"changed_files"`
	ErrorCount       int64         `json:"error_count"`
	LastError        string        `json:"last_error,omitempty"`
}

// ConnectorMetrics tracks connector performance metrics
type ConnectorMetrics struct {
	LastConnectionTime time.Time     `json:"last_connection_time"`
	FilesListed        int64         `json:"files_listed"`
	FilesRetrieved     int64         `json:"files_retrieved"`
	FilesDownloaded    int64         `json:"files_downloaded"`
	BytesDownloaded    int64         `json:"bytes_downloaded"`
	SyncCount          int64         `json:"sync_count"`
	LastSyncTime       time.Time     `json:"last_sync_time"`
	LastSyncDuration   time.Duration `json:"last_sync_duration"`
	LastListTime       time.Time     `json:"last_list_time"`
	ErrorCount         int64         `json:"error_count"`
	LastError          string        `json:"last_error,omitempty"`
}

// RetryPolicy defines retry behavior for API calls
type RetryPolicy struct {
	MaxRetries         int           `json:"max_retries"`
	InitialDelay       time.Duration `json:"initial_delay"`
	ExponentialBackoff bool          `json:"exponential_backoff"`
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	rate       float64
	burst      int
	tokens     float64
	lastUpdate time.Time
	mu         sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	return &RateLimiter{
		rate:       rate,
		burst:      burst,
		tokens:     float64(burst),
		lastUpdate: time.Now(),
	}
}

// Wait blocks until a token is available
func (rl *RateLimiter) Wait(ctx context.Context) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastUpdate).Seconds()

	// Add tokens based on elapsed time
	rl.tokens = min(float64(rl.burst), rl.tokens+elapsed*rl.rate)
	rl.lastUpdate = now

	if rl.tokens >= 1.0 {
		rl.tokens--
		return nil
	}

	// Calculate wait time
	waitTime := time.Duration((1.0-rl.tokens)/rl.rate) * time.Second

	// Release lock and wait
	rl.mu.Unlock()

	select {
	case <-ctx.Done():
		rl.mu.Lock() // Re-acquire lock for defer
		return ctx.Err()
	case <-time.After(waitTime):
		rl.mu.Lock()  // Re-acquire lock for defer
		rl.tokens = 0 // Consume the token
		return nil
	}
}

// Allow returns true if a token is available without blocking
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastUpdate).Seconds()

	// Add tokens based on elapsed time
	rl.tokens = min(float64(rl.burst), rl.tokens+elapsed*rl.rate)
	rl.lastUpdate = now

	if rl.tokens >= 1.0 {
		rl.tokens--
		return true
	}

	return false
}

// AuthenticationConfig contains OAuth2 authentication configuration
type AuthenticationConfig struct {
	ClientID     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	Scopes       []string `yaml:"scopes"`
	RedirectURL  string   `yaml:"redirect_url"`
}

// SyncConfiguration contains synchronization settings
type SyncConfiguration struct {
	Enabled           bool          `yaml:"enabled"`
	SyncInterval      time.Duration `yaml:"sync_interval"`
	FullSyncInterval  time.Duration `yaml:"full_sync_interval"`
	BatchSize         int           `yaml:"batch_size"`
	MaxConcurrentReqs int           `yaml:"max_concurrent_requests"`

	// Filters
	IncludeFolders   []string `yaml:"include_folders"`
	ExcludeFolders   []string `yaml:"exclude_folders"`
	FileExtensions   []string `yaml:"file_extensions"`
	MaxFileSize      int64    `yaml:"max_file_size"`
	SyncSharedFiles  bool     `yaml:"sync_shared_files"`
	SyncTrashedFiles bool     `yaml:"sync_trashed_files"`
}

// PerformanceConfig contains performance and optimization settings
type PerformanceConfig struct {
	// Rate limiting
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	BurstLimit        int     `yaml:"burst_limit"`

	// Retry configuration
	MaxRetries         int           `yaml:"max_retries"`
	RetryDelay         time.Duration `yaml:"retry_delay"`
	ExponentialBackoff bool          `yaml:"exponential_backoff"`

	// Caching
	CacheEnabled bool          `yaml:"cache_enabled"`
	CacheTTL     time.Duration `yaml:"cache_ttl"`
	CacheSize    int           `yaml:"cache_size"`

	// Connection management
	ConnectionTimeout time.Duration `yaml:"connection_timeout"`
	RequestTimeout    time.Duration `yaml:"request_timeout"`
	KeepAliveTimeout  time.Duration `yaml:"keep_alive_timeout"`
}

// SecurityConfig contains security and privacy settings
type SecurityConfig struct {
	// Encryption
	EncryptCredentials bool   `yaml:"encrypt_credentials"`
	EncryptionKey      string `yaml:"encryption_key,omitempty"`

	// Access control
	ReadOnlyMode bool     `yaml:"read_only_mode"`
	AllowedUsers []string `yaml:"allowed_users,omitempty"`
	BlockedUsers []string `yaml:"blocked_users,omitempty"`

	// Audit logging
	AuditLogging bool   `yaml:"audit_logging"`
	LogLevel     string `yaml:"log_level"`

	// Data handling
	RedactSensitiveData bool `yaml:"redact_sensitive_data"`
	PIIDetection        bool `yaml:"pii_detection"`
}

// MonitoringConfig contains monitoring and alerting settings
type MonitoringConfig struct {
	// Health checks
	HealthCheckEnabled  bool          `yaml:"health_check_enabled"`
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`

	// Metrics collection
	MetricsEnabled  bool          `yaml:"metrics_enabled"`
	MetricsInterval time.Duration `yaml:"metrics_interval"`

	// Alerting
	AlertsEnabled    bool          `yaml:"alerts_enabled"`
	ErrorThreshold   int           `yaml:"error_threshold"`
	LatencyThreshold time.Duration `yaml:"latency_threshold"`

	// Notifications
	NotificationChannels []string `yaml:"notification_channels"`
	SlackWebhook         string   `yaml:"slack_webhook,omitempty"`
	EmailNotifications   []string `yaml:"email_notifications,omitempty"`
}

// FileFilter represents file filtering criteria
type FileFilter struct {
	// Name patterns
	IncludePatterns []string `yaml:"include_patterns"`
	ExcludePatterns []string `yaml:"exclude_patterns"`

	// File types
	AllowedMimeTypes []string `yaml:"allowed_mime_types"`
	BlockedMimeTypes []string `yaml:"blocked_mime_types"`

	// Size constraints
	MinSize int64 `yaml:"min_size"`
	MaxSize int64 `yaml:"max_size"`

	// Time constraints
	CreatedAfter   *time.Time `yaml:"created_after,omitempty"`
	CreatedBefore  *time.Time `yaml:"created_before,omitempty"`
	ModifiedAfter  *time.Time `yaml:"modified_after,omitempty"`
	ModifiedBefore *time.Time `yaml:"modified_before,omitempty"`

	// Access constraints
	LastAccessedAfter  *time.Time `yaml:"last_accessed_after,omitempty"`
	LastAccessedBefore *time.Time `yaml:"last_accessed_before,omitempty"`
}

// ConnectionStatus represents the connection status
type ConnectionStatus struct {
	IsConnected      bool      `json:"is_connected"`
	ConnectedAt      time.Time `json:"connected_at"`
	LastPingAt       time.Time `json:"last_ping_at"`
	LastError        string    `json:"last_error,omitempty"`
	TokenExpiry      time.Time `json:"token_expiry"`
	TokenRefreshable bool      `json:"token_refreshable"`
}

// SyncStatistics contains detailed synchronization statistics
type SyncStatistics struct {
	TotalSyncs      int64         `json:"total_syncs"`
	SuccessfulSyncs int64         `json:"successful_syncs"`
	FailedSyncs     int64         `json:"failed_syncs"`
	AverageDuration time.Duration `json:"average_duration"`
	TotalFilesSync  int64         `json:"total_files_sync"`
	TotalBytesSync  int64         `json:"total_bytes_sync"`
	LastFullSync    time.Time     `json:"last_full_sync"`
	LastIncSync     time.Time     `json:"last_incremental_sync"`
}

// OperationResult represents the result of a connector operation
type OperationResult struct {
	Success        bool                   `json:"success"`
	ErrorMessage   string                 `json:"error_message,omitempty"`
	ErrorCode      string                 `json:"error_code,omitempty"`
	Duration       time.Duration          `json:"duration"`
	ItemsProcessed int64                  `json:"items_processed"`
	BytesProcessed int64                  `json:"bytes_processed"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// BatchOperationResult represents the result of a batch operation
type BatchOperationResult struct {
	TotalItems   int64             `json:"total_items"`
	SuccessItems int64             `json:"success_items"`
	FailedItems  int64             `json:"failed_items"`
	SkippedItems int64             `json:"skipped_items"`
	Duration     time.Duration     `json:"duration"`
	Errors       []string          `json:"errors,omitempty"`
	ItemResults  []OperationResult `json:"item_results,omitempty"`
}

// ConnectorHealth represents the health status of the connector
type ConnectorHealth struct {
	Status           string                 `json:"status"` // healthy, degraded, unhealthy
	CheckTime        time.Time              `json:"check_time"`
	ResponseTime     time.Duration          `json:"response_time"`
	ConnectivityOK   bool                   `json:"connectivity_ok"`
	AuthenticationOK bool                   `json:"authentication_ok"`
	RateLimitOK      bool                   `json:"rate_limit_ok"`
	ErrorRate        float64                `json:"error_rate"`
	Issues           []string               `json:"issues,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// QuotaInfo represents API quota information
type QuotaInfo struct {
	RequestsRemaining int64         `json:"requests_remaining"`
	RequestsLimit     int64         `json:"requests_limit"`
	WindowDuration    time.Duration `json:"window_duration"`
	ResetTime         time.Time     `json:"reset_time"`
	Consumed          float64       `json:"consumed"` // Percentage consumed
}

// FileMetadata contains extended file metadata
type FileMetadata struct {
	// Google Drive specific
	DriveID           string   `json:"drive_id"`
	SharedWithMe      bool     `json:"shared_with_me"`
	Owners            []string `json:"owners"`
	LastModifyingUser string   `json:"last_modifying_user"`
	Version           int64    `json:"version"`
	Thumbnail         string   `json:"thumbnail,omitempty"`

	// Sharing information
	SharingUser        string    `json:"sharing_user,omitempty"`
	SharedTime         time.Time `json:"shared_time,omitempty"`
	LinkShareable      bool      `json:"link_shareable"`
	ViewersCanDownload bool      `json:"viewers_can_download"`

	// Content information
	MD5Checksum   string `json:"md5_checksum,omitempty"`
	SHA1Checksum  string `json:"sha1_checksum,omitempty"`
	ContentType   string `json:"content_type"`
	FileExtension string `json:"file_extension"`

	// Additional metadata
	Labels        map[string]string `json:"labels,omitempty"`
	Properties    map[string]string `json:"properties,omitempty"`
	AppProperties map[string]string `json:"app_properties,omitempty"`
}

// Helper function for min operation
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
