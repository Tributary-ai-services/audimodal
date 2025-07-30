package backup

import (
	"time"

	"github.com/google/uuid"
)

// BackupConfig contains configuration for backup operations
type BackupConfig struct {
	// General settings
	Enabled                bool          `yaml:"enabled"`
	MaxConcurrentBackups   int           `yaml:"max_concurrent_backups"`
	MaxConcurrentRestores  int           `yaml:"max_concurrent_restores"`
	BackupTimeout          time.Duration `yaml:"backup_timeout"`
	RestoreTimeout         time.Duration `yaml:"restore_timeout"`
	RetryAttempts          int           `yaml:"retry_attempts"`
	RetryDelay             time.Duration `yaml:"retry_delay"`

	// Storage settings
	BackupStorageType      string        `yaml:"backup_storage_type"`      // local, s3, gcs, azure
	BackupStorageConfig    map[string]interface{} `yaml:"backup_storage_config"`
	CompressionEnabled     bool          `yaml:"compression_enabled"`
	EncryptionEnabled      bool          `yaml:"encryption_enabled"`
	EncryptionKey          string        `yaml:"encryption_key"`

	// Retention settings
	RetentionPolicy        *RetentionPolicy `yaml:"retention_policy"`
	AutoCleanupEnabled     bool          `yaml:"auto_cleanup_enabled"`
	CleanupInterval        time.Duration `yaml:"cleanup_interval"`

	// Validation settings
	IntegrityCheckEnabled  bool          `yaml:"integrity_check_enabled"`
	ChecksumAlgorithm      string        `yaml:"checksum_algorithm"`      // md5, sha256, sha512
	ValidationInterval     time.Duration `yaml:"validation_interval"`

	// Replication settings
	ReplicationEnabled     bool          `yaml:"replication_enabled"`
	ReplicationRegions     []string      `yaml:"replication_regions"`
	CrossRegionReplication bool          `yaml:"cross_region_replication"`

	// Performance settings
	ChunkSize              int64         `yaml:"chunk_size"`              // Backup chunk size
	ParallelUploads        int           `yaml:"parallel_uploads"`
	BandwidthLimit         int64         `yaml:"bandwidth_limit"`         // Bytes per second
	ThrottleLimit          float64       `yaml:"throttle_limit"`          // Operations per second
}

// DefaultBackupConfig returns default backup configuration
func DefaultBackupConfig() *BackupConfig {
	return &BackupConfig{
		Enabled:               true,
		MaxConcurrentBackups:  3,
		MaxConcurrentRestores: 2,
		BackupTimeout:         4 * time.Hour,
		RestoreTimeout:        2 * time.Hour,
		RetryAttempts:         3,
		RetryDelay:            5 * time.Minute,
		BackupStorageType:     "s3",
		CompressionEnabled:    true,
		EncryptionEnabled:     true,
		RetentionPolicy:       DefaultRetentionPolicy(),
		AutoCleanupEnabled:    true,
		CleanupInterval:       24 * time.Hour,
		IntegrityCheckEnabled: true,
		ChecksumAlgorithm:     "sha256",
		ValidationInterval:    7 * 24 * time.Hour, // Weekly validation
		ReplicationEnabled:    false,
		CrossRegionReplication: false,
		ChunkSize:             64 * 1024 * 1024, // 64MB chunks
		ParallelUploads:       4,
		BandwidthLimit:        0, // No limit
		ThrottleLimit:         10.0,
	}
}

// RetentionPolicy defines backup retention rules
type RetentionPolicy struct {
	// Time-based retention
	DailyRetentionDays    int `yaml:"daily_retention_days"`    // Keep daily backups for X days
	WeeklyRetentionWeeks  int `yaml:"weekly_retention_weeks"`  // Keep weekly backups for X weeks
	MonthlyRetentionMonths int `yaml:"monthly_retention_months"` // Keep monthly backups for X months
	YearlyRetentionYears  int `yaml:"yearly_retention_years"`  // Keep yearly backups for X years

	// Count-based retention
	MaxDailyBackups       int `yaml:"max_daily_backups"`
	MaxWeeklyBackups      int `yaml:"max_weekly_backups"`
	MaxMonthlyBackups     int `yaml:"max_monthly_backups"`
	MaxYearlyBackups      int `yaml:"max_yearly_backups"`

	// Special retention rules
	KeepFirstBackup       bool `yaml:"keep_first_backup"`       // Always keep the first backup
	KeepLastNBackups      int  `yaml:"keep_last_n_backups"`     // Always keep the last N backups
	MinRetentionPeriod    time.Duration `yaml:"min_retention_period"` // Minimum time to keep any backup
}

// DefaultRetentionPolicy returns default retention policy
func DefaultRetentionPolicy() *RetentionPolicy {
	return &RetentionPolicy{
		DailyRetentionDays:     30,  // 30 days of daily backups
		WeeklyRetentionWeeks:   12,  // 12 weeks of weekly backups
		MonthlyRetentionMonths: 12,  // 12 months of monthly backups
		YearlyRetentionYears:   7,   // 7 years of yearly backups
		MaxDailyBackups:        31,
		MaxWeeklyBackups:       52,
		MaxMonthlyBackups:      12,
		MaxYearlyBackups:       10,
		KeepFirstBackup:        true,
		KeepLastNBackups:       5,
		MinRetentionPeriod:     7 * 24 * time.Hour, // 7 days minimum
	}
}

// BackupType represents different types of backups
type BackupType string

const (
	BackupTypeFull        BackupType = "full"        // Complete backup of all data
	BackupTypeIncremental BackupType = "incremental" // Only changed files since last backup
	BackupTypeDifferential BackupType = "differential" // Changed files since last full backup
	BackupTypeSnapshot    BackupType = "snapshot"    // Point-in-time snapshot
)

// BackupStatus represents backup operation status
type BackupStatus string

const (
	BackupStatusPending    BackupStatus = "pending"
	BackupStatusInProgress BackupStatus = "in_progress"
	BackupStatusCompleted  BackupStatus = "completed"
	BackupStatusFailed     BackupStatus = "failed"
	BackupStatusCancelled  BackupStatus = "cancelled"
	BackupStatusValidating BackupStatus = "validating"
)

// RestoreStatus represents restore operation status
type RestoreStatus string

const (
	RestoreStatusPending    RestoreStatus = "pending"
	RestoreStatusInProgress RestoreStatus = "in_progress"
	RestoreStatusCompleted  RestoreStatus = "completed"
	RestoreStatusFailed     RestoreStatus = "failed"
	RestoreStatusCancelled  RestoreStatus = "cancelled"
	RestoreStatusValidating RestoreStatus = "validating"
)

// BackupPolicy defines automatic backup rules and settings
type BackupPolicy struct {
	ID               uuid.UUID        `json:"id"`
	Name             string           `json:"name"`
	Description      string           `json:"description"`
	TenantID         uuid.UUID        `json:"tenant_id"`

	// Policy settings
	BackupType       BackupType       `json:"backup_type"`
	Schedule         string           `json:"schedule"`         // Cron expression
	Enabled          bool             `json:"enabled"`
	Priority         int              `json:"priority"`

	// Backup configuration
	BackupConfig     BackupJobConfig  `json:"backup_config"`

	// Source configuration
	SourcePaths      []string         `json:"source_paths"`
	IncludePatterns  []string         `json:"include_patterns"`
	ExcludePatterns  []string         `json:"exclude_patterns"`
	FollowSymlinks   bool             `json:"follow_symlinks"`

	// Retention and cleanup
	RetentionPolicy  *RetentionPolicy `json:"retention_policy"`
	AutoCleanup      bool             `json:"auto_cleanup"`

	// Notification settings
	NotifyOnSuccess  bool             `json:"notify_on_success"`
	NotifyOnFailure  bool             `json:"notify_on_failure"`
	NotificationChannels []string     `json:"notification_channels"`

	// Metadata
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	CreatedBy        string           `json:"created_by"`
	LastRun          *time.Time       `json:"last_run,omitempty"`
	NextRun          *time.Time       `json:"next_run,omitempty"`
}

// Backup represents a backup instance
type Backup struct {
	ID               uuid.UUID        `json:"id"`
	PolicyID         *uuid.UUID       `json:"policy_id,omitempty"`
	TenantID         uuid.UUID        `json:"tenant_id"`
	Name             string           `json:"name"`
	Description      string           `json:"description"`

	// Backup details
	Type             BackupType       `json:"type"`
	Status           BackupStatus     `json:"status"`
	StartTime        time.Time        `json:"start_time"`
	EndTime          *time.Time       `json:"end_time,omitempty"`
	Duration         time.Duration    `json:"duration"`

	// Source information
	SourcePaths      []string         `json:"source_paths"`
	TotalFiles       int64            `json:"total_files"`
	TotalSize        int64            `json:"total_size"`
	ProcessedFiles   int64            `json:"processed_files"`
	ProcessedSize    int64            `json:"processed_size"`

	// Backup storage information
	BackupLocation   string           `json:"backup_location"`
	BackupSize       int64            `json:"backup_size"`
	CompressedSize   int64            `json:"compressed_size"`
	CompressionRatio float64          `json:"compression_ratio"`

	// Integrity and validation
	Checksum         string           `json:"checksum"`
	ChecksumAlgorithm string          `json:"checksum_algorithm"`
	Encrypted        bool             `json:"encrypted"`
	EncryptionAlgorithm string        `json:"encryption_algorithm,omitempty"`

	// Metadata
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	Tags             map[string]string      `json:"tags,omitempty"`
	CreatedBy        string           `json:"created_by"`
	Version          string           `json:"version"`

	// Parent backup (for incremental/differential)
	ParentBackupID   *uuid.UUID       `json:"parent_backup_id,omitempty"`
	BackupChain      []uuid.UUID      `json:"backup_chain,omitempty"`

	// Expiration and retention
	ExpiresAt        *time.Time       `json:"expires_at,omitempty"`
	RetentionPolicy  string           `json:"retention_policy"`
	
	// Replication status
	ReplicationStatus map[string]string `json:"replication_status,omitempty"`
	
	// Error information
	ErrorMessage     string           `json:"error_message,omitempty"`
	ErrorDetails     map[string]interface{} `json:"error_details,omitempty"`
}

// BackupJob represents a backup operation job
type BackupJob struct {
	ID               uuid.UUID        `json:"id"`
	PolicyID         *uuid.UUID       `json:"policy_id,omitempty"`
	TenantID         uuid.UUID        `json:"tenant_id"`
	Type             BackupType       `json:"type"`
	Status           BackupStatus     `json:"status"`
	CreatedAt        time.Time        `json:"created_at"`
	StartedAt        *time.Time       `json:"started_at,omitempty"`
	CompletedAt      *time.Time       `json:"completed_at,omitempty"`

	// Job configuration
	Config           BackupJobConfig  `json:"config"`

	// Progress tracking
	Progress         BackupProgress   `json:"progress"`

	// Results
	BackupID         *uuid.UUID       `json:"backup_id,omitempty"`
	ErrorMessage     string           `json:"error_message,omitempty"`
	ErrorDetails     map[string]interface{} `json:"error_details,omitempty"`

	// Metadata
	CreatedBy        string           `json:"created_by"`
	Tags             map[string]string `json:"tags,omitempty"`
}

// BackupJobConfig contains configuration for a specific backup job
type BackupJobConfig struct {
	SourcePaths      []string         `json:"source_paths"`
	BackupName       string           `json:"backup_name"`
	BackupType       BackupType       `json:"backup_type"`
	
	// Filters
	IncludePatterns  []string         `json:"include_patterns"`
	ExcludePatterns  []string         `json:"exclude_patterns"`
	FollowSymlinks   bool             `json:"follow_symlinks"`
	
	// Processing options
	CompressionEnabled bool           `json:"compression_enabled"`
	EncryptionEnabled  bool           `json:"encryption_enabled"`
	VerifyChecksum     bool           `json:"verify_checksum"`
	
	// Performance options
	ChunkSize        int64            `json:"chunk_size"`
	ParallelWorkers  int              `json:"parallel_workers"`
	BandwidthLimit   int64            `json:"bandwidth_limit"`
	
	// Storage options
	StorageLocation  string           `json:"storage_location"`
	StorageClass     string           `json:"storage_class"`
	
	// Parent backup for incremental/differential
	ParentBackupID   *uuid.UUID       `json:"parent_backup_id,omitempty"`
}

// BackupProgress tracks backup job progress
type BackupProgress struct {
	StartTime        time.Time        `json:"start_time"`
	LastUpdate       time.Time        `json:"last_update"`
	EstimatedETA     *time.Time       `json:"estimated_eta,omitempty"`

	// File progress
	TotalFiles       int64            `json:"total_files"`
	ProcessedFiles   int64            `json:"processed_files"`
	FailedFiles      int64            `json:"failed_files"`
	SkippedFiles     int64            `json:"skipped_files"`

	// Size progress
	TotalBytes       int64            `json:"total_bytes"`
	ProcessedBytes   int64            `json:"processed_bytes"`
	CompressedBytes  int64            `json:"compressed_bytes"`

	// Performance metrics
	ThroughputMBps   float64          `json:"throughput_mbps"`
	FilesPerSecond   float64          `json:"files_per_second"`

	// Current operation
	CurrentPhase     string           `json:"current_phase"`
	CurrentFile      string           `json:"current_file"`
	CurrentOperation string           `json:"current_operation"`
}

// RestoreJob represents a restore operation job
type RestoreJob struct {
	ID               uuid.UUID        `json:"id"`
	TenantID         uuid.UUID        `json:"tenant_id"`
	BackupID         uuid.UUID        `json:"backup_id"`
	Status           RestoreStatus    `json:"status"`
	CreatedAt        time.Time        `json:"created_at"`
	StartedAt        *time.Time       `json:"started_at,omitempty"`
	CompletedAt      *time.Time       `json:"completed_at,omitempty"`

	// Job configuration
	Config           RestoreJobConfig `json:"config"`

	// Progress tracking
	Progress         RestoreProgress  `json:"progress"`

	// Error information
	ErrorMessage     string           `json:"error_message,omitempty"`
	ErrorDetails     map[string]interface{} `json:"error_details,omitempty"`

	// Metadata
	CreatedBy        string           `json:"created_by"`
	Tags             map[string]string `json:"tags,omitempty"`
}

// RestoreJobConfig contains configuration for a restore job
type RestoreJobConfig struct {
	// Target configuration
	TargetPath       string           `json:"target_path"`
	OverwriteExisting bool            `json:"overwrite_existing"`
	RestorePermissions bool           `json:"restore_permissions"`
	RestoreTimestamps bool            `json:"restore_timestamps"`

	// Filters
	IncludePatterns  []string         `json:"include_patterns"`
	ExcludePatterns  []string         `json:"exclude_patterns"`
	RestorePaths     []string         `json:"restore_paths"` // Specific paths to restore

	// Point-in-time restore
	RestoreToTime    *time.Time       `json:"restore_to_time,omitempty"`

	// Performance options
	ParallelWorkers  int              `json:"parallel_workers"`
	BandwidthLimit   int64            `json:"bandwidth_limit"`
	VerifyChecksum   bool             `json:"verify_checksum"`

	// Conflict resolution
	ConflictResolution string         `json:"conflict_resolution"` // skip, overwrite, rename
}

// RestoreProgress tracks restore job progress
type RestoreProgress struct {
	StartTime        time.Time        `json:"start_time"`
	LastUpdate       time.Time        `json:"last_update"`
	EstimatedETA     *time.Time       `json:"estimated_eta,omitempty"`

	// File progress
	TotalFiles       int64            `json:"total_files"`
	ProcessedFiles   int64            `json:"processed_files"`
	FailedFiles      int64            `json:"failed_files"`
	SkippedFiles     int64            `json:"skipped_files"`

	// Size progress
	TotalBytes       int64            `json:"total_bytes"`
	ProcessedBytes   int64            `json:"processed_bytes"`

	// Performance metrics
	ThroughputMBps   float64          `json:"throughput_mbps"`
	FilesPerSecond   float64          `json:"files_per_second"`

	// Current operation
	CurrentPhase     string           `json:"current_phase"`
	CurrentFile      string           `json:"current_file"`
	CurrentOperation string           `json:"current_operation"`
}

// BackupRequest represents a request to create a backup
type BackupRequest struct {
	TenantID         uuid.UUID        `json:"tenant_id"`
	PolicyID         uuid.UUID        `json:"policy_id,omitempty"`
	Name             string           `json:"name"`
	Description      string           `json:"description"`
	Type             BackupType       `json:"type"`
	Config           BackupJobConfig  `json:"config"`
	CreatedBy        string           `json:"created_by"`
	Tags             map[string]string `json:"tags,omitempty"`
}

// RestoreRequest represents a request to restore from a backup
type RestoreRequest struct {
	TenantID         uuid.UUID        `json:"tenant_id"`
	BackupID         uuid.UUID        `json:"backup_id"`
	Name             string           `json:"name"`
	Description      string           `json:"description"`
	Config           RestoreJobConfig `json:"config"`
	CreatedBy        string           `json:"created_by"`
	Tags             map[string]string `json:"tags,omitempty"`
}

// PointInTimeRestoreRequest represents a point-in-time restore request
type PointInTimeRestoreRequest struct {
	TenantID         uuid.UUID        `json:"tenant_id"`
	RestoreToTime    time.Time        `json:"restore_to_time"`
	SourcePaths      []string         `json:"source_paths"`
	Config           RestoreJobConfig `json:"config"`
	CreatedBy        string           `json:"created_by"`
}

// BackupFilters for querying backups
type BackupFilters struct {
	TenantID         *uuid.UUID       `json:"tenant_id,omitempty"`
	PolicyID         *uuid.UUID       `json:"policy_id,omitempty"`
	BackupType       *BackupType      `json:"backup_type,omitempty"`
	Status           BackupStatus     `json:"status,omitempty"`
	StartTime        time.Time        `json:"start_time,omitempty"`
	EndTime          time.Time        `json:"end_time,omitempty"`
	SourcePath       string           `json:"source_path,omitempty"`
	Tags             map[string]string `json:"tags,omitempty"`
	Limit            int              `json:"limit,omitempty"`
	Offset           int              `json:"offset,omitempty"`
	OrderBy          string           `json:"order_by,omitempty"`
	OrderDesc        bool             `json:"order_desc,omitempty"`
}

// BackupValidation represents backup integrity validation results
type BackupValidation struct {
	BackupID         uuid.UUID        `json:"backup_id"`
	StartedAt        time.Time        `json:"started_at"`
	CompletedAt      time.Time        `json:"completed_at"`
	Status           string           `json:"status"` // validating, passed, failed
	Issues           []ValidationIssue `json:"issues"`
	FilesValidated   int64            `json:"files_validated"`
	ChecksumMatches  int64            `json:"checksum_matches"`
	ChecksumMismatches int64          `json:"checksum_mismatches"`
	MissingFiles     int64            `json:"missing_files"`
}

// ValidationIssue represents a validation issue
type ValidationIssue struct {
	Type             string           `json:"type"`        // metadata, files, checksum, corruption
	Severity         string           `json:"severity"`    // info, warning, critical
	Description      string           `json:"description"`
	FilePath         string           `json:"file_path,omitempty"`
	Details          map[string]interface{} `json:"details,omitempty"`
}

// RestoreValidation represents restore operation validation results
type RestoreValidation struct {
	RestoreJobID     uuid.UUID        `json:"restore_job_id"`
	BackupID         uuid.UUID        `json:"backup_id"`
	StartedAt        time.Time        `json:"started_at"`
	CompletedAt      time.Time        `json:"completed_at"`
	Status           string           `json:"status"`
	Issues           []ValidationIssue `json:"issues"`
	FilesRestored    int64            `json:"files_restored"`
	FilesVerified    int64            `json:"files_verified"`
	VerificationFailures int64        `json:"verification_failures"`
}

// ReplicationStatus represents backup replication status
type ReplicationStatus struct {
	BackupID         uuid.UUID        `json:"backup_id"`
	SourceRegion     string           `json:"source_region"`
	TargetRegions    map[string]ReplicationRegionStatus `json:"target_regions"`
	OverallStatus    string           `json:"overall_status"` // pending, in_progress, completed, failed
	StartedAt        time.Time        `json:"started_at"`
	CompletedAt      *time.Time       `json:"completed_at,omitempty"`
}

// ReplicationRegionStatus represents replication status for a specific region
type ReplicationRegionStatus struct {
	Region           string           `json:"region"`
	Status           string           `json:"status"` // pending, in_progress, completed, failed
	Progress         float64          `json:"progress"` // 0.0 to 1.0
	StartedAt        time.Time        `json:"started_at"`
	CompletedAt      *time.Time       `json:"completed_at,omitempty"`
	ReplicatedSize   int64            `json:"replicated_size"`
	ErrorMessage     string           `json:"error_message,omitempty"`
}

// DisasterRecoveryPlan represents a disaster recovery plan
type DisasterRecoveryPlan struct {
	ID               uuid.UUID        `json:"id"`
	Name             string           `json:"name"`
	Description      string           `json:"description"`
	TenantID         uuid.UUID        `json:"tenant_id"`

	// Recovery objectives
	RPO              time.Duration    `json:"rpo"` // Recovery Point Objective
	RTO              time.Duration    `json:"rto"` // Recovery Time Objective

	// Recovery steps
	RecoverySteps    []RecoveryStep   `json:"recovery_steps"`
	
	// Backup requirements
	RequiredBackups  []BackupRequirement `json:"required_backups"`
	
	// Testing schedule
	TestingSchedule  string           `json:"testing_schedule"` // Cron expression
	LastTest         *time.Time       `json:"last_test,omitempty"`
	NextTest         *time.Time       `json:"next_test,omitempty"`

	// Metadata
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	CreatedBy        string           `json:"created_by"`
	Enabled          bool             `json:"enabled"`
}

// RecoveryStep represents a step in a disaster recovery plan
type RecoveryStep struct {
	ID               int              `json:"id"`
	Name             string           `json:"name"`
	Description      string           `json:"description"`
	Type             string           `json:"type"` // restore, validate, notify, custom
	Order            int              `json:"order"`
	Parallel         bool             `json:"parallel"` // Can be executed in parallel with other steps
	Config           map[string]interface{} `json:"config"`
	EstimatedDuration time.Duration   `json:"estimated_duration"`
	Dependencies     []int            `json:"dependencies"` // IDs of steps that must complete first
}

// BackupRequirement represents backup requirements for disaster recovery
type BackupRequirement struct {
	Name             string           `json:"name"`
	BackupType       BackupType       `json:"backup_type"`
	SourcePaths      []string         `json:"source_paths"`
	MaxAge           time.Duration    `json:"max_age"` // Maximum age of backup to be usable
	Required         bool             `json:"required"` // Whether this backup is required for recovery
}

// DisasterRecoveryTest represents a disaster recovery test execution
type DisasterRecoveryTest struct {
	ID               uuid.UUID        `json:"id"`
	PlanID           uuid.UUID        `json:"plan_id"`
	TenantID         uuid.UUID        `json:"tenant_id"`
	Status           string           `json:"status"` // scheduled, running, completed, failed
	StartedAt        time.Time        `json:"started_at"`
	CompletedAt      *time.Time       `json:"completed_at,omitempty"`
	
	// Test results
	TotalSteps       int              `json:"total_steps"`
	CompletedSteps   int              `json:"completed_steps"`
	FailedSteps      int              `json:"failed_steps"`
	TestResults      []TestStepResult `json:"test_results"`
	
	// Performance metrics
	ActualRTO        time.Duration    `json:"actual_rto"`
	ActualRPO        time.Duration    `json:"actual_rpo"`
	
	// Metadata
	CreatedBy        string           `json:"created_by"`
	Notes            string           `json:"notes"`
}

// TestStepResult represents the result of a disaster recovery test step
type TestStepResult struct {
	StepID           int              `json:"step_id"`
	StepName         string           `json:"step_name"`
	Status           string           `json:"status"` // pending, running, completed, failed, skipped
	StartedAt        time.Time        `json:"started_at"`
	CompletedAt      *time.Time       `json:"completed_at,omitempty"`
	Duration         time.Duration    `json:"duration"`
	ErrorMessage     string           `json:"error_message,omitempty"`
	Output           string           `json:"output,omitempty"`
	Metrics          map[string]interface{} `json:"metrics,omitempty"`
}