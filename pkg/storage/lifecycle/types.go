package lifecycle

import (
	"time"

	"github.com/google/uuid"
)

// LifecycleAction represents an action to take on data
type LifecycleAction string

const (
	ActionArchive     LifecycleAction = "archive"
	ActionDelete      LifecycleAction = "delete"
	ActionCompress    LifecycleAction = "compress"
	ActionMove        LifecycleAction = "move"
	ActionEncrypt     LifecycleAction = "encrypt"
	ActionReplicate   LifecycleAction = "replicate"
	ActionNotify      LifecycleAction = "notify"
	ActionTag         LifecycleAction = "tag"
	ActionAudit       LifecycleAction = "audit"
)

// LifecycleStatus represents the status of a lifecycle operation
type LifecycleStatus string

const (
	StatusPending    LifecycleStatus = "pending"
	StatusInProgress LifecycleStatus = "in_progress"
	StatusCompleted  LifecycleStatus = "completed"
	StatusFailed     LifecycleStatus = "failed"
	StatusCancelled  LifecycleStatus = "cancelled"
	StatusSkipped    LifecycleStatus = "skipped"
)

// LifecycleTrigger represents what triggers a lifecycle action
type LifecycleTrigger string

const (
	TriggerAge        LifecycleTrigger = "age"         // Based on file age
	TriggerSize       LifecycleTrigger = "size"        // Based on file size
	TriggerAccess     LifecycleTrigger = "access"      // Based on access patterns
	TriggerStorage    LifecycleTrigger = "storage"     // Based on storage usage
	TriggerCost       LifecycleTrigger = "cost"        // Based on cost optimization
	TriggerCompliance LifecycleTrigger = "compliance"  // Based on compliance rules
	TriggerManual     LifecycleTrigger = "manual"      // Manual trigger
	TriggerScheduled  LifecycleTrigger = "scheduled"   // Scheduled execution
)

// StorageTier represents different storage tiers
type StorageTier string

const (
	TierHot      StorageTier = "hot"       // Frequently accessed
	TierWarm     StorageTier = "warm"      // Occasionally accessed
	TierCold     StorageTier = "cold"      // Rarely accessed
	TierArchive  StorageTier = "archive"   // Long-term archive
	TierGlacier  StorageTier = "glacier"   // Deep archive
)

// LifecycleConfig contains configuration for the lifecycle management system
type LifecycleConfig struct {
	// General settings
	Enabled              bool          `yaml:"enabled"`
	DryRun               bool          `yaml:"dry_run"`
	MaxConcurrentJobs    int           `yaml:"max_concurrent_jobs"`
	JobTimeout           time.Duration `yaml:"job_timeout"`
	RetryAttempts        int           `yaml:"retry_attempts"`
	RetryDelay           time.Duration `yaml:"retry_delay"`
	
	// Scheduling
	DefaultSchedule      string        `yaml:"default_schedule"`        // Cron expression
	ArchiveSchedule      string        `yaml:"archive_schedule"`
	DeletionSchedule     string        `yaml:"deletion_schedule"`
	ComplianceSchedule   string        `yaml:"compliance_schedule"`
	
	// Default policies
	DefaultArchiveAge    time.Duration `yaml:"default_archive_age"`     // 90 days
	DefaultDeletionAge   time.Duration `yaml:"default_deletion_age"`    // 7 years
	DefaultCompressAge   time.Duration `yaml:"default_compress_age"`    // 30 days
	
	// Storage settings
	ArchiveStorageClass  string        `yaml:"archive_storage_class"`
	GlacierStorageClass  string        `yaml:"glacier_storage_class"`
	DeletedPrefix        string        `yaml:"deleted_prefix"`
	ArchivedPrefix       string        `yaml:"archived_prefix"`
	
	// Safety settings
	RequireConfirmation  bool          `yaml:"require_confirmation"`
	GracePeriod          time.Duration `yaml:"grace_period"`
	MinRetentionPeriod   time.Duration `yaml:"min_retention_period"`
	
	// Audit and notification
	AuditAllActions      bool          `yaml:"audit_all_actions"`
	NotifyOnDeletion     bool          `yaml:"notify_on_deletion"`
	NotifyOnArchival     bool          `yaml:"notify_on_archival"`
	NotificationChannels []string      `yaml:"notification_channels"`
}

// DefaultLifecycleConfig returns default lifecycle configuration
func DefaultLifecycleConfig() *LifecycleConfig {
	return &LifecycleConfig{
		Enabled:              true,
		DryRun:               false,
		MaxConcurrentJobs:    10,
		JobTimeout:           1 * time.Hour,
		RetryAttempts:        3,
		RetryDelay:           5 * time.Minute,
		DefaultSchedule:      "0 2 * * *",        // Daily at 2 AM
		ArchiveSchedule:      "0 3 * * 0",        // Weekly on Sunday at 3 AM
		DeletionSchedule:     "0 4 * * 0",        // Weekly on Sunday at 4 AM
		ComplianceSchedule:   "0 1 * * 1",        // Weekly on Monday at 1 AM
		DefaultArchiveAge:    90 * 24 * time.Hour,   // 90 days
		DefaultDeletionAge:   7 * 365 * 24 * time.Hour, // 7 years
		DefaultCompressAge:   30 * 24 * time.Hour,   // 30 days
		ArchiveStorageClass:  "STANDARD_IA",
		GlacierStorageClass:  "GLACIER",
		DeletedPrefix:        ".deleted/",
		ArchivedPrefix:       ".archived/",
		RequireConfirmation:  true,
		GracePeriod:          24 * time.Hour,     // 24 hours
		MinRetentionPeriod:   30 * 24 * time.Hour, // 30 days
		AuditAllActions:      true,
		NotifyOnDeletion:     true,
		NotifyOnArchival:     false,
		NotificationChannels: []string{"email", "slack"},
	}
}

// LifecyclePolicy defines rules for data lifecycle management
type LifecyclePolicy struct {
	ID               uuid.UUID             `json:"id"`
	Name             string                `json:"name"`
	Description      string                `json:"description"`
	TenantID         uuid.UUID             `json:"tenant_id"`
	
	// Policy metadata
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
	CreatedBy        string                `json:"created_by"`
	Enabled          bool                  `json:"enabled"`
	Priority         int                   `json:"priority"`
	
	// Policy rules
	Rules            []LifecycleRule       `json:"rules"`
	DefaultActions   []LifecycleAction     `json:"default_actions"`
	
	// Scheduling
	Schedule         string                `json:"schedule"`         // Cron expression
	NextRun          time.Time             `json:"next_run"`
	LastRun          *time.Time            `json:"last_run,omitempty"`
	
	// Policy settings
	Settings         LifecyclePolicySettings `json:"settings"`
}

// LifecycleRule defines when and how to apply lifecycle actions
type LifecycleRule struct {
	ID               uuid.UUID             `json:"id"`
	Name             string                `json:"name"`
	Conditions       []LifecycleCondition  `json:"conditions"`
	Actions          []LifecycleActionSpec `json:"actions"`
	Priority         int                   `json:"priority"`
	Enabled          bool                  `json:"enabled"`
	
	// Execution settings
	DelayAfterMatch  time.Duration         `json:"delay_after_match"`
	RequireManualApproval bool             `json:"require_manual_approval"`
	NotificationRequired  bool             `json:"notification_required"`
}

// LifecycleCondition defines conditions for applying lifecycle actions
type LifecycleCondition struct {
	Type             ConditionType         `json:"type"`
	Operator         string                `json:"operator"`
	Value            interface{}           `json:"value"`
	Field            string                `json:"field,omitempty"`
	
	// Advanced conditions
	TimeWindow       *TimeWindow           `json:"time_window,omitempty"`
	AccessPattern    *AccessPattern        `json:"access_pattern,omitempty"`
	SizeThreshold    *SizeThreshold        `json:"size_threshold,omitempty"`
}

// ConditionType for lifecycle policies
type ConditionType string

const (
	ConditionAge              ConditionType = "age"
	ConditionLastAccessed     ConditionType = "last_accessed"
	ConditionFileSize         ConditionType = "file_size"
	ConditionFileType         ConditionType = "file_type"
	ConditionPath             ConditionType = "path"
	ConditionStorageClass     ConditionType = "storage_class"
	ConditionTags             ConditionType = "tags"
	ConditionMetadata         ConditionType = "metadata"
	ConditionAccessCount      ConditionType = "access_count"
	ConditionCost             ConditionType = "cost"
	ConditionCompliance       ConditionType = "compliance"
	ConditionDataClassification ConditionType = "data_classification"
)

// TimeWindow defines a time-based condition window
type TimeWindow struct {
	Start    time.Time     `json:"start"`
	End      time.Time     `json:"end"`
	Duration time.Duration `json:"duration"`
	Timezone string        `json:"timezone"`
}

// AccessPattern defines access-based conditions
type AccessPattern struct {
	MinAccesses      int           `json:"min_accesses"`
	MaxAccesses      int           `json:"max_accesses"`
	TimeWindow       time.Duration `json:"time_window"`
	AccessTypes      []string      `json:"access_types"`  // read, write, metadata
	UserPatterns     []string      `json:"user_patterns"`
}

// SizeThreshold defines size-based conditions
type SizeThreshold struct {
	MinSize      int64   `json:"min_size"`
	MaxSize      int64   `json:"max_size"`
	SizeUnit     string  `json:"size_unit"`     // bytes, KB, MB, GB, TB
	CompareType  string  `json:"compare_type"`  // absolute, percentage
}

// LifecycleActionSpec defines a specific action to take
type LifecycleActionSpec struct {
	Action           LifecycleAction       `json:"action"`
	Priority         int                   `json:"priority"`
	DelayBefore      time.Duration         `json:"delay_before"`
	Timeout          time.Duration         `json:"timeout"`
	RetryAttempts    int                   `json:"retry_attempts"`
	ContinueOnError  bool                  `json:"continue_on_error"`
	
	// Action-specific parameters
	Parameters       map[string]interface{} `json:"parameters"`
	
	// Conditions for this action
	Prerequisites    []string              `json:"prerequisites"`    // Other actions that must complete first
	ConditionalOn    []LifecycleCondition  `json:"conditional_on"`   // Additional conditions for this action
}

// LifecyclePolicySettings contains policy-wide settings
type LifecyclePolicySettings struct {
	DryRun               bool          `yaml:"dry_run"`
	RequireApproval      bool          `yaml:"require_approval"`
	NotificationChannels []string      `yaml:"notification_channels"`
	AuditLevel           string        `yaml:"audit_level"`       // none, basic, detailed
	
	// Safety settings
	GracePeriod          time.Duration `yaml:"grace_period"`
	MaxFilesPerRun       int           `yaml:"max_files_per_run"`
	RateLimitPerSecond   float64       `yaml:"rate_limit_per_second"`
	
	// Error handling
	ErrorHandling        string        `yaml:"error_handling"`    // continue, stop, retry
	MaxErrors            int           `yaml:"max_errors"`
	ErrorNotification    bool          `yaml:"error_notification"`
}

// LifecycleJob represents a lifecycle management job
type LifecycleJob struct {
	ID               uuid.UUID             `json:"id"`
	PolicyID         uuid.UUID             `json:"policy_id"`
	TenantID         uuid.UUID             `json:"tenant_id"`
	
	// Job details
	Type             string                `json:"type"`             // archive, delete, compress, etc.
	Status           LifecycleStatus       `json:"status"`
	CreatedAt        time.Time             `json:"created_at"`
	StartedAt        *time.Time            `json:"started_at,omitempty"`
	CompletedAt      *time.Time            `json:"completed_at,omitempty"`
	
	// Job configuration
	Config           LifecycleJobConfig    `json:"config"`
	
	// Progress tracking
	Progress         LifecycleProgress     `json:"progress"`
	
	// Results
	Results          LifecycleResults      `json:"results"`
	ErrorMessage     string                `json:"error_message,omitempty"`
	
	// Metadata
	CreatedBy        string                `json:"created_by"`
	Tags             map[string]string     `json:"tags,omitempty"`
}

// LifecycleJobConfig contains configuration for a specific job
type LifecycleJobConfig struct {
	DryRun           bool                  `json:"dry_run"`
	MaxConcurrency   int                   `json:"max_concurrency"`
	BatchSize        int                   `json:"batch_size"`
	Timeout          time.Duration         `json:"timeout"`
	RetryAttempts    int                   `json:"retry_attempts"`
	
	// Filters
	PathFilters      []string              `json:"path_filters"`
	FileFilters      []string              `json:"file_filters"`
	SizeFilters      *SizeThreshold        `json:"size_filters,omitempty"`
	DateFilters      *TimeWindow           `json:"date_filters,omitempty"`
	
	// Actions
	Actions          []LifecycleActionSpec `json:"actions"`
}

// LifecycleProgress tracks job progress
type LifecycleProgress struct {
	TotalFiles       int64     `json:"total_files"`
	ProcessedFiles   int64     `json:"processed_files"`
	SuccessfulFiles  int64     `json:"successful_files"`
	FailedFiles      int64     `json:"failed_files"`
	SkippedFiles     int64     `json:"skipped_files"`
	
	TotalBytes       int64     `json:"total_bytes"`
	ProcessedBytes   int64     `json:"processed_bytes"`
	
	StartTime        time.Time `json:"start_time"`
	LastUpdate       time.Time `json:"last_update"`
	EstimatedETA     *time.Time `json:"estimated_eta,omitempty"`
	
	CurrentOperation string    `json:"current_operation"`
	CurrentFile      string    `json:"current_file"`
}

// LifecycleResults contains job results
type LifecycleResults struct {
	TotalFilesProcessed  int64                 `json:"total_files_processed"`
	TotalBytesProcessed  int64                 `json:"total_bytes_processed"`
	ActionsPerformed     map[string]int64      `json:"actions_performed"`
	StorageSaved         int64                 `json:"storage_saved"`
	CostSaved            float64               `json:"cost_saved"`
	
	// Detailed results
	ArchivedFiles        []string              `json:"archived_files,omitempty"`
	DeletedFiles         []string              `json:"deleted_files,omitempty"`
	CompressedFiles      []string              `json:"compressed_files,omitempty"`
	FailedFiles          []LifecycleError      `json:"failed_files,omitempty"`
	
	// Performance metrics
	Duration             time.Duration         `json:"duration"`
	ThroughputMBps       float64               `json:"throughput_mbps"`
	OperationsPerSecond  float64               `json:"operations_per_second"`
}

// LifecycleError represents an error during lifecycle operations
type LifecycleError struct {
	FilePath         string                `json:"file_path"`
	Action           LifecycleAction       `json:"action"`
	ErrorCode        string                `json:"error_code"`
	ErrorMessage     string                `json:"error_message"`
	Timestamp        time.Time             `json:"timestamp"`
	RetryCount       int                   `json:"retry_count"`
	Recoverable      bool                  `json:"recoverable"`
}

// LifecycleEvent represents an event in the lifecycle system
type LifecycleEvent struct {
	ID               uuid.UUID             `json:"id"`
	Type             string                `json:"type"`
	Timestamp        time.Time             `json:"timestamp"`
	TenantID         uuid.UUID             `json:"tenant_id"`
	JobID            *uuid.UUID            `json:"job_id,omitempty"`
	PolicyID         *uuid.UUID            `json:"policy_id,omitempty"`
	
	// Event details
	Action           LifecycleAction       `json:"action"`
	Status           LifecycleStatus       `json:"status"`
	FilePath         string                `json:"file_path,omitempty"`
	Message          string                `json:"message"`
	
	// Metadata
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	UserID           string                `json:"user_id,omitempty"`
	Source           string                `json:"source"`
}

// LifecycleMetrics tracks lifecycle system metrics
type LifecycleMetrics struct {
	// Job statistics
	TotalJobs            int64                 `json:"total_jobs"`
	ActiveJobs           int64                 `json:"active_jobs"`
	CompletedJobs        int64                 `json:"completed_jobs"`
	FailedJobs           int64                 `json:"failed_jobs"`
	
	// File statistics
	TotalFilesProcessed  int64                 `json:"total_files_processed"`
	FilesArchived        int64                 `json:"files_archived"`
	FilesDeleted         int64                 `json:"files_deleted"`
	FilesCompressed      int64                 `json:"files_compressed"`
	
	// Storage statistics
	TotalBytesProcessed  int64                 `json:"total_bytes_processed"`
	BytesArchived        int64                 `json:"bytes_archived"`
	BytesDeleted         int64                 `json:"bytes_deleted"`
	BytesCompressed      int64                 `json:"bytes_compressed"`
	StorageSaved         int64                 `json:"storage_saved"`
	
	// Performance metrics
	AverageJobDuration   time.Duration         `json:"average_job_duration"`
	ThroughputMBps       float64               `json:"throughput_mbps"`
	ErrorRate            float64               `json:"error_rate"`
	
	// Cost metrics
	CostSaved            float64               `json:"cost_saved"`
	StorageCostReduction float64               `json:"storage_cost_reduction"`
}

// RetentionPolicy defines data retention requirements
type RetentionPolicy struct {
	ID               uuid.UUID             `json:"id"`
	Name             string                `json:"name"`
	TenantID         uuid.UUID             `json:"tenant_id"`
	
	// Retention rules
	MinRetentionPeriod   time.Duration     `json:"min_retention_period"`
	MaxRetentionPeriod   time.Duration     `json:"max_retention_period"`
	LegalHoldPeriod      time.Duration     `json:"legal_hold_period"`
	
	// Classification-based retention
	ClassificationRules  map[string]time.Duration `json:"classification_rules"`
	
	// Compliance settings
	ComplianceFramework  string            `json:"compliance_framework"`  // GDPR, HIPAA, SOX, etc.
	ImmutablePeriod      time.Duration     `json:"immutable_period"`
	RequireJustification bool              `json:"require_justification"`
	
	// Audit settings
	AuditRetentionChanges bool             `json:"audit_retention_changes"`
	NotifyBeforeExpiry    time.Duration    `json:"notify_before_expiry"`
}