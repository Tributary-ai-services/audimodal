package tiering

import (
	"time"

	"github.com/google/uuid"
)

// StorageTier represents different storage performance and cost tiers
type StorageTier string

const (
	TierHot     StorageTier = "hot"     // High performance, frequently accessed
	TierWarm    StorageTier = "warm"    // Medium performance, occasionally accessed
	TierCold    StorageTier = "cold"    // Lower performance, rarely accessed
	TierArchive StorageTier = "archive" // Lowest cost, long-term retention
	TierGlacier StorageTier = "glacier" // Ultra-low cost, deep archive
)

// TierAccessPattern represents how data is accessed
type TierAccessPattern string

const (
	AccessPatternFrequent   TierAccessPattern = "frequent"   // Daily access
	AccessPatternRegular    TierAccessPattern = "regular"    // Weekly access
	AccessPatternInfrequent TierAccessPattern = "infrequent" // Monthly access
	AccessPatternRare       TierAccessPattern = "rare"       // Quarterly access
	AccessPatternArchival   TierAccessPattern = "archival"   // Yearly or less
)

// TierTransitionTrigger represents what triggers a tier transition
type TierTransitionTrigger string

const (
	TriggerAge           TierTransitionTrigger = "age"            // Based on file age
	TriggerAccessPattern TierTransitionTrigger = "access_pattern" // Based on access frequency
	TriggerSize          TierTransitionTrigger = "size"           // Based on file size
	TriggerCost          TierTransitionTrigger = "cost"           // Based on cost optimization
	TriggerManual        TierTransitionTrigger = "manual"         // Manual trigger
	TriggerPolicy        TierTransitionTrigger = "policy"         // Policy-based
	TriggerCapacity      TierTransitionTrigger = "capacity"       // Based on storage capacity
)

// TierStatus represents the status of tiering operations
type TierStatus string

const (
	StatusActive      TierStatus = "active"
	StatusTransitioning TierStatus = "transitioning"
	StatusCompleted   TierStatus = "completed"
	StatusFailed      TierStatus = "failed"
	StatusPending     TierStatus = "pending"
	StatusCancelled   TierStatus = "cancelled"
)

// TieringConfig contains configuration for multi-tier storage
type TieringConfig struct {
	// General settings
	Enabled              bool          `yaml:"enabled"`
	AutoTiering          bool          `yaml:"auto_tiering"`
	MaxConcurrentJobs    int           `yaml:"max_concurrent_jobs"`
	JobTimeout           time.Duration `yaml:"job_timeout"`
	RetryAttempts        int           `yaml:"retry_attempts"`
	RetryDelay           time.Duration `yaml:"retry_delay"`
	
	// Monitoring and analysis
	AccessTrackingEnabled bool          `yaml:"access_tracking_enabled"`
	AnalysisInterval      time.Duration `yaml:"analysis_interval"`
	MetricsRetention      time.Duration `yaml:"metrics_retention"`
	
	// Default tier transition rules
	HotToWarmAge         time.Duration `yaml:"hot_to_warm_age"`         // 30 days
	WarmToColdAge        time.Duration `yaml:"warm_to_cold_age"`        // 90 days
	ColdToArchiveAge     time.Duration `yaml:"cold_to_archive_age"`     // 365 days
	ArchiveToGlacierAge  time.Duration `yaml:"archive_to_glacier_age"`  // 7 years
	
	// Access-based thresholds
	HotAccessThreshold   int           `yaml:"hot_access_threshold"`    // Accesses per week
	WarmAccessThreshold  int           `yaml:"warm_access_threshold"`   // Accesses per month
	ColdAccessThreshold  int           `yaml:"cold_access_threshold"`   // Accesses per quarter
	
	// Size-based rules
	LargeFileThreshold   int64         `yaml:"large_file_threshold"`    // Large files go to cold faster
	SmallFileThreshold   int64         `yaml:"small_file_threshold"`    // Keep small files in hot longer
	
	// Cost optimization
	CostOptimizationEnabled bool        `yaml:"cost_optimization_enabled"`
	CostThresholds          map[StorageTier]float64 `yaml:"cost_thresholds"`
	
	// Provider-specific settings
	ProviderConfigs      map[string]ProviderTieringConfig `yaml:"provider_configs"`
	
	// Safety and compliance
	MinRetentionPeriods  map[StorageTier]time.Duration `yaml:"min_retention_periods"`
	RequireApproval      bool                          `yaml:"require_approval"`
	NotifyOnTransition   bool                          `yaml:"notify_on_transition"`
	
	// Performance settings
	BatchSize            int                           `yaml:"batch_size"`
	ThrottleLimit        float64                       `yaml:"throttle_limit"` // Operations per second
}

// ProviderTieringConfig contains provider-specific tiering configuration
type ProviderTieringConfig struct {
	StorageClasses       map[StorageTier]string        `yaml:"storage_classes"`
	TransitionCosts      map[string]float64            `yaml:"transition_costs"`
	StorageCosts         map[StorageTier]float64       `yaml:"storage_costs"`
	RetrievalCosts       map[StorageTier]float64       `yaml:"retrieval_costs"`
	MinimumRetention     map[StorageTier]time.Duration `yaml:"minimum_retention"`
	SupportedTransitions map[StorageTier][]StorageTier `yaml:"supported_transitions"`
}

// DefaultTieringConfig returns default tiering configuration
func DefaultTieringConfig() *TieringConfig {
	return &TieringConfig{
		Enabled:              true,
		AutoTiering:          true,
		MaxConcurrentJobs:    5,
		JobTimeout:           2 * time.Hour,
		RetryAttempts:        3,
		RetryDelay:           5 * time.Minute,
		AccessTrackingEnabled: true,
		AnalysisInterval:     6 * time.Hour,
		MetricsRetention:     90 * 24 * time.Hour, // 90 days
		HotToWarmAge:         30 * 24 * time.Hour,  // 30 days
		WarmToColdAge:        90 * 24 * time.Hour,  // 90 days
		ColdToArchiveAge:     365 * 24 * time.Hour, // 1 year
		ArchiveToGlacierAge:  7 * 365 * 24 * time.Hour, // 7 years
		HotAccessThreshold:   7,  // 7 accesses per week
		WarmAccessThreshold:  4,  // 4 accesses per month
		ColdAccessThreshold:  1,  // 1 access per quarter
		LargeFileThreshold:   100 * 1024 * 1024, // 100MB
		SmallFileThreshold:   1024 * 1024,       // 1MB
		CostOptimizationEnabled: true,
		CostThresholds: map[StorageTier]float64{
			TierHot:     0.023, // $0.023 per GB per month
			TierWarm:    0.0125, // $0.0125 per GB per month
			TierCold:    0.004, // $0.004 per GB per month
			TierArchive: 0.00099, // $0.00099 per GB per month
			TierGlacier: 0.0004, // $0.0004 per GB per month
		},
		MinRetentionPeriods: map[StorageTier]time.Duration{
			TierHot:     0,
			TierWarm:    30 * 24 * time.Hour,  // 30 days
			TierCold:    90 * 24 * time.Hour,  // 90 days
			TierArchive: 180 * 24 * time.Hour, // 180 days
			TierGlacier: 365 * 24 * time.Hour, // 365 days
		},
		RequireApproval:    false,
		NotifyOnTransition: true,
		BatchSize:          1000,
		ThrottleLimit:      10.0, // 10 operations per second
	}
}

// TieringPolicy defines rules for automatic tier transitions
type TieringPolicy struct {
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
	
	// Tier transition rules
	TransitionRules  []TierTransitionRule  `json:"transition_rules"`
	
	// Policy settings
	Settings         TieringPolicySettings `json:"settings"`
	
	// Scheduling
	Schedule         string                `json:"schedule"`         // Cron expression
	NextRun          time.Time             `json:"next_run"`
	LastRun          *time.Time            `json:"last_run,omitempty"`
}

// TierTransitionRule defines when and how to transition between tiers
type TierTransitionRule struct {
	ID               uuid.UUID             `json:"id"`
	Name             string                `json:"name"`
	FromTier         StorageTier           `json:"from_tier"`
	ToTier           StorageTier           `json:"to_tier"`
	
	// Conditions for transition
	Conditions       []TieringCondition    `json:"conditions"`
	
	// Transition settings
	DelayAfterMatch  time.Duration         `json:"delay_after_match"`
	RequireApproval  bool                  `json:"require_approval"`
	NotifyOnTransition bool                `json:"notify_on_transition"`
	
	// Cost and performance considerations
	CostSavingsThreshold float64           `json:"cost_savings_threshold"`
	MinDataSize          int64             `json:"min_data_size"`
	MaxDataSize          int64             `json:"max_data_size"`
	
	// Transition metadata
	Priority         int                   `json:"priority"`
	Enabled          bool                  `json:"enabled"`
}

// TieringCondition defines conditions for tier transitions
type TieringCondition struct {
	Type             TieringConditionType  `json:"type"`
	Operator         string                `json:"operator"`
	Value            interface{}           `json:"value"`
	Field            string                `json:"field,omitempty"`
	
	// Time-based conditions
	TimeWindow       *TimeWindow           `json:"time_window,omitempty"`
	
	// Access-based conditions
	AccessMetrics    *AccessMetricsCondition `json:"access_metrics,omitempty"`
	
	// Cost-based conditions
	CostMetrics      *CostMetricsCondition `json:"cost_metrics,omitempty"`
}

// TieringConditionType represents types of tiering conditions
type TieringConditionType string

const (
	ConditionAge              TieringConditionType = "age"
	ConditionAccessCount      TieringConditionType = "access_count"
	ConditionAccessPattern    TieringConditionType = "access_pattern"
	ConditionFileSize         TieringConditionType = "file_size"
	ConditionFileType         TieringConditionType = "file_type"
	ConditionCostPerGB        TieringConditionType = "cost_per_gb"
	ConditionStorageUsage     TieringConditionType = "storage_usage"
	ConditionRetrievalCost    TieringConditionType = "retrieval_cost"
	ConditionLastAccessed     TieringConditionType = "last_accessed"
	ConditionDataClassification TieringConditionType = "data_classification"
	ConditionCompliance       TieringConditionType = "compliance"
)

// TimeWindow defines a time-based condition window
type TimeWindow struct {
	Start    time.Time     `json:"start"`
	End      time.Time     `json:"end"`
	Duration time.Duration `json:"duration"`
	Timezone string        `json:"timezone"`
}

// AccessMetricsCondition defines access-based conditions
type AccessMetricsCondition struct {
	MinAccesses      int           `json:"min_accesses"`
	MaxAccesses      int           `json:"max_accesses"`
	TimeWindow       time.Duration `json:"time_window"`
	AccessTypes      []string      `json:"access_types"`  // read, write, metadata
	PatternType      TierAccessPattern `json:"pattern_type"`
}

// CostMetricsCondition defines cost-based conditions
type CostMetricsCondition struct {
	MaxCostPerGB     float64       `json:"max_cost_per_gb"`
	MaxMonthlyCost   float64       `json:"max_monthly_cost"`
	SavingsThreshold float64       `json:"savings_threshold"`
	IncludeRetrieval bool          `json:"include_retrieval"`
}

// TieringPolicySettings contains policy-wide settings
type TieringPolicySettings struct {
	AutoExecute          bool          `yaml:"auto_execute"`
	DryRun               bool          `yaml:"dry_run"`
	RequireApproval      bool          `yaml:"require_approval"`
	NotificationChannels []string      `yaml:"notification_channels"`
	
	// Safety settings
	GracePeriod          time.Duration `yaml:"grace_period"`
	MaxFilesPerRun       int           `yaml:"max_files_per_run"`
	RateLimitPerSecond   float64       `yaml:"rate_limit_per_second"`
	
	// Error handling
	ErrorHandling        string        `yaml:"error_handling"`    // continue, stop, retry
	MaxErrors            int           `yaml:"max_errors"`
	ErrorNotification    bool          `yaml:"error_notification"`
	
	// Cost controls
	MaxCostPerJob        float64       `yaml:"max_cost_per_job"`
	CostApprovalThreshold float64      `yaml:"cost_approval_threshold"`
}

// TieringJob represents a tiering operation job
type TieringJob struct {
	ID               uuid.UUID             `json:"id"`
	PolicyID         *uuid.UUID            `json:"policy_id,omitempty"`
	TenantID         uuid.UUID             `json:"tenant_id"`
	
	// Job details
	Type             string                `json:"type"`             // transition, analysis, optimization
	FromTier         StorageTier           `json:"from_tier"`
	ToTier           StorageTier           `json:"to_tier"`
	Status           TierStatus            `json:"status"`
	CreatedAt        time.Time             `json:"created_at"`
	StartedAt        *time.Time            `json:"started_at,omitempty"`
	CompletedAt      *time.Time            `json:"completed_at,omitempty"`
	
	// Job configuration
	Config           TieringJobConfig      `json:"config"`
	
	// Progress tracking
	Progress         TieringProgress       `json:"progress"`
	
	// Results
	Results          TieringResults        `json:"results"`
	ErrorMessage     string                `json:"error_message,omitempty"`
	
	// Metadata
	CreatedBy        string                `json:"created_by"`
	Tags             map[string]string     `json:"tags,omitempty"`
}

// TieringJobConfig contains configuration for a specific tiering job
type TieringJobConfig struct {
	DryRun           bool                  `json:"dry_run"`
	MaxConcurrency   int                   `json:"max_concurrency"`
	BatchSize        int                   `json:"batch_size"`
	Timeout          time.Duration         `json:"timeout"`
	RetryAttempts    int                   `json:"retry_attempts"`
	
	// Filters
	PathFilters      []string              `json:"path_filters"`
	FileFilters      []string              `json:"file_filters"`
	SizeFilters      *SizeFilter           `json:"size_filters,omitempty"`
	DateFilters      *TimeWindow           `json:"date_filters,omitempty"`
	
	// Cost controls
	MaxCost          float64               `json:"max_cost"`
	CostEstimation   bool                  `json:"cost_estimation"`
	
	// Performance controls
	ThrottleLimit    float64               `json:"throttle_limit"`    // Operations per second
	OffPeakOnly      bool                  `json:"off_peak_only"`
}

// SizeFilter defines size-based filters
type SizeFilter struct {
	MinSize      int64   `json:"min_size"`
	MaxSize      int64   `json:"max_size"`
	SizeUnit     string  `json:"size_unit"`     // bytes, KB, MB, GB, TB
}

// TieringProgress tracks tiering job progress
type TieringProgress struct {
	TotalFiles       int64     `json:"total_files"`
	ProcessedFiles   int64     `json:"processed_files"`
	SuccessfulFiles  int64     `json:"successful_files"`
	FailedFiles      int64     `json:"failed_files"`
	SkippedFiles     int64     `json:"skipped_files"`
	
	TotalBytes       int64     `json:"total_bytes"`
	ProcessedBytes   int64     `json:"processed_bytes"`
	
	EstimatedCost    float64   `json:"estimated_cost"`
	ActualCost       float64   `json:"actual_cost"`
	EstimatedSavings float64   `json:"estimated_savings"`
	
	StartTime        time.Time `json:"start_time"`
	LastUpdate       time.Time `json:"last_update"`
	EstimatedETA     *time.Time `json:"estimated_eta,omitempty"`
	
	CurrentOperation string    `json:"current_operation"`
	CurrentFile      string    `json:"current_file"`
	CurrentBatch     int       `json:"current_batch"`
}

// TieringResults contains tiering job results
type TieringResults struct {
	TotalFilesProcessed  int64                 `json:"total_files_processed"`
	TotalBytesProcessed  int64                 `json:"total_bytes_processed"`
	TransitionsPerformed map[string]int64      `json:"transitions_performed"`
	
	// Cost metrics
	TotalCost            float64               `json:"total_cost"`
	TransitionCosts      float64               `json:"transition_costs"`
	StorageSavings       float64               `json:"storage_savings"`
	MonthlySavings       float64               `json:"monthly_savings"`
	
	// Performance metrics
	Duration             time.Duration         `json:"duration"`
	ThroughputMBps       float64               `json:"throughput_mbps"`
	TransitionsPerSecond float64               `json:"transitions_per_second"`
	
	// Detailed results
	TransitionedFiles    []TierTransitionResult `json:"transitioned_files,omitempty"`
	FailedFiles          []TieringError        `json:"failed_files,omitempty"`
}

// TierTransitionResult represents the result of a single tier transition
type TierTransitionResult struct {
	FilePath         string                `json:"file_path"`
	FromTier         StorageTier           `json:"from_tier"`
	ToTier           StorageTier           `json:"to_tier"`
	FileSize         int64                 `json:"file_size"`
	TransitionCost   float64               `json:"transition_cost"`
	MonthlySavings   float64               `json:"monthly_savings"`
	CompletedAt      time.Time             `json:"completed_at"`
	Metadata         map[string]string     `json:"metadata,omitempty"`
}

// TieringError represents an error during tiering operations
type TieringError struct {
	FilePath         string                `json:"file_path"`
	FromTier         StorageTier           `json:"from_tier"`
	ToTier           StorageTier           `json:"to_tier"`
	ErrorCode        string                `json:"error_code"`
	ErrorMessage     string                `json:"error_message"`
	Timestamp        time.Time             `json:"timestamp"`
	RetryCount       int                   `json:"retry_count"`
	Recoverable      bool                  `json:"recoverable"`
}

// FileAccessMetrics tracks access patterns for intelligent tiering
type FileAccessMetrics struct {
	FilePath         string                `json:"file_path"`
	FileSize         int64                 `json:"file_size"`
	CurrentTier      StorageTier           `json:"current_tier"`
	
	// Access statistics
	TotalAccesses    int64                 `json:"total_accesses"`
	ReadAccesses     int64                 `json:"read_accesses"`
	WriteAccesses    int64                 `json:"write_accesses"`
	MetadataAccesses int64                 `json:"metadata_accesses"`
	
	// Time-based access patterns
	LastAccessed     time.Time             `json:"last_accessed"`
	FirstAccessed    time.Time             `json:"first_accessed"`
	AccessFrequency  TierAccessPattern     `json:"access_frequency"`
	
	// Access pattern analysis
	DailyAccesses    []int                 `json:"daily_accesses"`      // Last 30 days
	WeeklyAccesses   []int                 `json:"weekly_accesses"`     // Last 12 weeks
	MonthlyAccesses  []int                 `json:"monthly_accesses"`    // Last 12 months
	
	// Predictive metrics
	PredictedPattern TierAccessPattern     `json:"predicted_pattern"`
	RecommendedTier  StorageTier           `json:"recommended_tier"`
	ConfidenceScore  float64               `json:"confidence_score"`
	
	// Cost metrics
	CurrentCost      float64               `json:"current_cost"`
	OptimalCost      float64               `json:"optimal_cost"`
	PotentialSavings float64               `json:"potential_savings"`
	
	// Metadata
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
	Tags             map[string]string     `json:"tags,omitempty"`
}

// TieringMetrics tracks overall tiering system metrics
type TieringMetrics struct {
	// Storage distribution
	TierDistribution     map[StorageTier]TierStats `json:"tier_distribution"`
	
	// Job statistics
	TotalJobs            int64                     `json:"total_jobs"`
	ActiveJobs           int64                     `json:"active_jobs"`
	CompletedJobs        int64                     `json:"completed_jobs"`
	FailedJobs           int64                     `json:"failed_jobs"`
	
	// Transition statistics
	TotalTransitions     int64                     `json:"total_transitions"`
	TransitionsByTier    map[string]int64          `json:"transitions_by_tier"`
	FailedTransitions    int64                     `json:"failed_transitions"`
	
	// Cost metrics
	TotalStorageCost     float64                   `json:"total_storage_cost"`
	MonthlyCostSavings   float64                   `json:"monthly_cost_savings"`
	TransitionCosts      float64                   `json:"transition_costs"`
	NetSavings           float64                   `json:"net_savings"`
	
	// Performance metrics
	AverageJobDuration   time.Duration             `json:"average_job_duration"`
	ThroughputMBps       float64                   `json:"throughput_mbps"`
	ErrorRate            float64                   `json:"error_rate"`
	
	// Access pattern metrics
	FilesAnalyzed        int64                     `json:"files_analyzed"`
	OptimizationOpportunities int64               `json:"optimization_opportunities"`
}

// TierStats represents statistics for a specific tier
type TierStats struct {
	FileCount        int64   `json:"file_count"`
	TotalSize        int64   `json:"total_size"`
	AverageFileSize  int64   `json:"average_file_size"`
	MonthlyCost      float64 `json:"monthly_cost"`
	AccessCount      int64   `json:"access_count"`
	LastAccessTime   time.Time `json:"last_access_time"`
	StorageClass     string  `json:"storage_class"`
}

// TieringRecommendation represents a tier optimization recommendation
type TieringRecommendation struct {
	ID               uuid.UUID             `json:"id"`
	FilePath         string                `json:"file_path"`
	CurrentTier      StorageTier           `json:"current_tier"`
	RecommendedTier  StorageTier           `json:"recommended_tier"`
	
	// Recommendation details
	Reason           string                `json:"reason"`
	ConfidenceScore  float64               `json:"confidence_score"`
	PotentialSavings float64               `json:"potential_savings"`
	TransitionCost   float64               `json:"transition_cost"`
	PaybackPeriod    time.Duration         `json:"payback_period"`
	
	// Supporting data
	AccessMetrics    *FileAccessMetrics    `json:"access_metrics"`
	CostAnalysis     *CostAnalysis         `json:"cost_analysis"`
	
	// Metadata
	GeneratedAt      time.Time             `json:"generated_at"`
	ValidUntil       time.Time             `json:"valid_until"`
	Status           string                `json:"status"`  // pending, approved, rejected, implemented
}

// CostAnalysis provides detailed cost analysis for tier recommendations
type CostAnalysis struct {
	CurrentMonthlyCost   float64   `json:"current_monthly_cost"`
	RecommendedMonthlyCost float64 `json:"recommended_monthly_cost"`
	TransitionCost       float64   `json:"transition_cost"`
	MonthlySavings       float64   `json:"monthly_savings"`
	AnnualSavings        float64   `json:"annual_savings"`
	ROI                  float64   `json:"roi"`
	PaybackMonths        int       `json:"payback_months"`
	
	// Cost breakdown
	StorageCost          float64   `json:"storage_cost"`
	RetrievalCost        float64   `json:"retrieval_cost"`
	OperationCost        float64   `json:"operation_cost"`
	DataTransferCost     float64   `json:"data_transfer_cost"`
}