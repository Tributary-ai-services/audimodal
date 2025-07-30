package anomaly

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AnomalyType represents the type of anomaly detected
type AnomalyType string

const (
	// Content-based anomalies
	AnomalyTypeContentSize        AnomalyType = "content_size"
	AnomalyTypeContentStructure   AnomalyType = "content_structure"
	AnomalyTypeContentSentiment   AnomalyType = "content_sentiment"
	AnomalyTypeContentLanguage    AnomalyType = "content_language"
	AnomalyTypeContentQuality     AnomalyType = "content_quality"
	AnomalyTypeContentDuplication AnomalyType = "content_duplication"
	
	// Security-based anomalies
	AnomalyTypeSuspiciousContent  AnomalyType = "suspicious_content"
	AnomalyTypeDataLeak          AnomalyType = "data_leak"
	AnomalyTypeMaliciousPattern  AnomalyType = "malicious_pattern"
	AnomalyTypeUnauthorizedAccess AnomalyType = "unauthorized_access"
	AnomalyTypePrivilegeEscalation AnomalyType = "privilege_escalation"
	
	// Behavioral anomalies
	AnomalyTypeUploadPattern     AnomalyType = "upload_pattern"
	AnomalyTypeAccessPattern     AnomalyType = "access_pattern"
	AnomalyTypeUsagePattern      AnomalyType = "usage_pattern"
	AnomalyTypePerformance       AnomalyType = "performance"
	AnomalyTypeFrequency         AnomalyType = "frequency"
	
	// Statistical anomalies
	AnomalyTypeOutlier           AnomalyType = "outlier"
	AnomalyTypeTrend             AnomalyType = "trend"
	AnomalyTypeDistribution      AnomalyType = "distribution"
	AnomalyTypeCorrelation       AnomalyType = "correlation"
	AnomalyTypeSeasonality       AnomalyType = "seasonality"
)

// AnomalySeverity represents the severity level of an anomaly
type AnomalySeverity string

const (
	SeverityLow      AnomalySeverity = "low"
	SeverityMedium   AnomalySeverity = "medium"
	SeverityHigh     AnomalySeverity = "high"
	SeverityCritical AnomalySeverity = "critical"
)

// AnomalyStatus represents the current status of an anomaly
type AnomalyStatus string

const (
	StatusDetected   AnomalyStatus = "detected"
	StatusAnalyzing  AnomalyStatus = "analyzing"
	StatusConfirmed  AnomalyStatus = "confirmed"
	StatusFalsePositive AnomalyStatus = "false_positive"
	StatusResolved   AnomalyStatus = "resolved"
	StatusSuppressed AnomalyStatus = "suppressed"
)

// Anomaly represents a detected anomaly in the system
type Anomaly struct {
	ID             uuid.UUID                `json:"id"`
	Type           AnomalyType              `json:"type"`
	Severity       AnomalySeverity          `json:"severity"`
	Status         AnomalyStatus            `json:"status"`
	Title          string                   `json:"title"`
	Description    string                   `json:"description"`
	DetectedAt     time.Time                `json:"detected_at"`
	UpdatedAt      time.Time                `json:"updated_at"`
	ResolvedAt     *time.Time               `json:"resolved_at,omitempty"`
	
	// Context information
	TenantID       uuid.UUID                `json:"tenant_id"`
	DataSourceID   *uuid.UUID               `json:"data_source_id,omitempty"`
	DocumentID     *uuid.UUID               `json:"document_id,omitempty"`
	ChunkID        *uuid.UUID               `json:"chunk_id,omitempty"`
	UserID         *uuid.UUID               `json:"user_id,omitempty"`
	
	// Anomaly details
	Score          float64                  `json:"score"`          // Anomaly score (0-1)
	Confidence     float64                  `json:"confidence"`     // Detection confidence (0-1)
	Threshold      float64                  `json:"threshold"`      // Detection threshold used
	Baseline       map[string]interface{}   `json:"baseline"`       // Baseline values for comparison
	Detected       map[string]interface{}   `json:"detected"`       // Detected values
	Metadata       map[string]interface{}   `json:"metadata"`       // Additional context
	
	// Detection information
	DetectorName   string                   `json:"detector_name"`
	DetectorVersion string                  `json:"detector_version"`
	RuleName       string                   `json:"rule_name,omitempty"`
	
	// Resolution information
	ResolutionNotes string                  `json:"resolution_notes,omitempty"`
	ResolvedBy     *uuid.UUID               `json:"resolved_by,omitempty"`
	Actions        []AnomalyAction          `json:"actions,omitempty"`
	
	// Related anomalies
	RelatedAnomalies []uuid.UUID            `json:"related_anomalies,omitempty"`
	ParentAnomalyID  *uuid.UUID             `json:"parent_anomaly_id,omitempty"`
	
	// Notification and alerting
	NotificationsSent []NotificationRecord   `json:"notifications_sent,omitempty"`
	SuppressNotifications bool               `json:"suppress_notifications"`
}

// AnomalyAction represents an action taken in response to an anomaly
type AnomalyAction struct {
	ID          uuid.UUID              `json:"id"`
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	ExecutedAt  time.Time              `json:"executed_at"`
	ExecutedBy  *uuid.UUID             `json:"executed_by,omitempty"`
	Status      string                 `json:"status"`
	Result      map[string]interface{} `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

// NotificationRecord tracks notifications sent for an anomaly
type NotificationRecord struct {
	Channel     string    `json:"channel"`
	Recipients  []string  `json:"recipients"`
	SentAt      time.Time `json:"sent_at"`
	MessageID   string    `json:"message_id,omitempty"`
	Status      string    `json:"status"`
	Error       string    `json:"error,omitempty"`
}

// AnomalyDetector defines the interface for anomaly detection algorithms
type AnomalyDetector interface {
	// GetName returns the name of the detector
	GetName() string
	
	// GetVersion returns the version of the detector
	GetVersion() string
	
	// GetSupportedTypes returns the types of anomalies this detector can find
	GetSupportedTypes() []AnomalyType
	
	// DetectAnomalies analyzes input data and returns detected anomalies
	DetectAnomalies(ctx context.Context, input *DetectionInput) ([]*Anomaly, error)
	
	// UpdateBaseline updates the baseline data used for anomaly detection
	UpdateBaseline(ctx context.Context, data *BaselineData) error
	
	// GetBaseline returns the current baseline data
	GetBaseline(ctx context.Context) (*BaselineData, error)
	
	// Configure updates the detector configuration
	Configure(config map[string]interface{}) error
	
	// IsEnabled returns whether the detector is currently enabled
	IsEnabled() bool
}

// DetectionInput contains the input data for anomaly detection
type DetectionInput struct {
	// Content data
	Content           string                 `json:"content,omitempty"`
	ContentType       string                 `json:"content_type,omitempty"`
	FileSize          int64                  `json:"file_size,omitempty"`
	FileName          string                 `json:"file_name,omitempty"`
	
	// Context information
	TenantID          uuid.UUID              `json:"tenant_id"`
	DataSourceID      *uuid.UUID             `json:"data_source_id,omitempty"`
	DocumentID        *uuid.UUID             `json:"document_id,omitempty"`
	ChunkID           *uuid.UUID             `json:"chunk_id,omitempty"`
	UserID            *uuid.UUID             `json:"user_id,omitempty"`
	
	// Temporal data
	Timestamp         time.Time              `json:"timestamp"`
	TimeWindow        *TimeWindow            `json:"time_window,omitempty"`
	
	// Behavioral data
	AccessPattern     *AccessPattern         `json:"access_pattern,omitempty"`
	UsageMetrics      *UsageMetrics          `json:"usage_metrics,omitempty"`
	
	// Analysis results (from ML pipeline)
	SentimentResult   *SentimentResult       `json:"sentiment_result,omitempty"`
	TopicResults      []TopicResult          `json:"topic_results,omitempty"`
	EntityResults     []EntityResult         `json:"entity_results,omitempty"`
	QualityMetrics    *QualityMetrics        `json:"quality_metrics,omitempty"`
	LanguageResult    *LanguageResult        `json:"language_result,omitempty"`
	
	// Additional metadata
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

// TimeWindow represents a time range for analysis
type TimeWindow struct {
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Duration time.Duration `json:"duration"`
}

// AccessPattern represents document access patterns
type AccessPattern struct {
	ViewCount       int64     `json:"view_count"`
	DownloadCount   int64     `json:"download_count"`
	ShareCount      int64     `json:"share_count"`
	LastAccessed    time.Time `json:"last_accessed"`
	AccessFrequency float64   `json:"access_frequency"`
	UniqueUsers     int       `json:"unique_users"`
	PeakHours       []int     `json:"peak_hours"`
	Locations       []string  `json:"locations"`
}

// UsageMetrics represents system usage metrics
type UsageMetrics struct {
	CPUUsage        float64   `json:"cpu_usage"`
	MemoryUsage     float64   `json:"memory_usage"`
	DiskUsage       float64   `json:"disk_usage"`
	NetworkIO       float64   `json:"network_io"`
	ProcessingTime  time.Duration `json:"processing_time"`
	ErrorRate       float64   `json:"error_rate"`
	ThroughputRate  float64   `json:"throughput_rate"`
}

// BaselineData contains baseline information for anomaly detection
type BaselineData struct {
	TenantID         uuid.UUID              `json:"tenant_id"`
	DetectorName     string                 `json:"detector_name"`
	DataType         string                 `json:"data_type"`
	
	// Statistical baselines
	Mean             float64                `json:"mean,omitempty"`
	StandardDev      float64                `json:"standard_dev,omitempty"`
	Median           float64                `json:"median,omitempty"`
	Percentiles      map[string]float64     `json:"percentiles,omitempty"`
	
	// Distribution data
	Histogram        map[string]int         `json:"histogram,omitempty"`
	Frequency        map[string]int         `json:"frequency,omitempty"`
	
	// Temporal patterns
	HourlyPattern    []float64              `json:"hourly_pattern,omitempty"`
	DailyPattern     []float64              `json:"daily_pattern,omitempty"`
	WeeklyPattern    []float64              `json:"weekly_pattern,omitempty"`
	SeasonalPattern  map[string]float64     `json:"seasonal_pattern,omitempty"`
	
	// Content baselines
	AvgContentLength  float64               `json:"avg_content_length,omitempty"`
	CommonLanguages   map[string]float64    `json:"common_languages,omitempty"`
	CommonTopics      map[string]float64    `json:"common_topics,omitempty"`
	SentimentBaseline map[string]float64    `json:"sentiment_baseline,omitempty"`
	
	// Behavioral baselines
	TypicalUsers      []uuid.UUID           `json:"typical_users,omitempty"`
	TypicalAccess     *AccessPattern        `json:"typical_access,omitempty"`
	TypicalUsage      *UsageMetrics         `json:"typical_usage,omitempty"`
	
	// Metadata
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
	SampleCount      int64                  `json:"sample_count"`
	ConfidenceLevel  float64                `json:"confidence_level"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// AnomalyDetectionConfig contains configuration for anomaly detection
type AnomalyDetectionConfig struct {
	// Global settings
	Enabled              bool                           `json:"enabled"`
	DefaultSeverity      AnomalySeverity                `json:"default_severity"`
	MinConfidence        float64                        `json:"min_confidence"`
	MaxAnomaliesToKeep   int                            `json:"max_anomalies_to_keep"`
	BaselineUpdateInterval time.Duration                `json:"baseline_update_interval"`
	
	// Detection thresholds
	Thresholds           map[AnomalyType]float64        `json:"thresholds"`
	SeverityThresholds   map[AnomalySeverity]float64    `json:"severity_thresholds"`
	
	// Detector configurations
	Detectors            map[string]DetectorConfig      `json:"detectors"`
	
	// Notification settings
	NotificationConfig   *NotificationConfig            `json:"notification_config,omitempty"`
	
	// Storage settings
	RetentionPeriod      time.Duration                  `json:"retention_period"`
	ArchiveOldAnomalies  bool                           `json:"archive_old_anomalies"`
	
	// Processing settings
	BatchSize            int                            `json:"batch_size"`
	ProcessingTimeout    time.Duration                  `json:"processing_timeout"`
	MaxConcurrentDetectors int                          `json:"max_concurrent_detectors"`
	
	// Machine learning settings
	EnableMLDetection    bool                           `json:"enable_ml_detection"`
	ModelUpdateInterval  time.Duration                  `json:"model_update_interval"`
	TrainingDataSize     int                            `json:"training_data_size"`
}

// DetectorConfig contains configuration for a specific detector
type DetectorConfig struct {
	Name                 string                         `json:"name"`
	Enabled              bool                           `json:"enabled"`
	Priority             int                            `json:"priority"`
	Sensitivity          float64                        `json:"sensitivity"`
	Thresholds           map[string]float64             `json:"thresholds"`
	Parameters           map[string]interface{}         `json:"parameters"`
	UpdateInterval       time.Duration                  `json:"update_interval"`
	RequiredSampleSize   int                            `json:"required_sample_size"`
}

// NotificationConfig contains notification settings for anomalies
type NotificationConfig struct {
	Channels             []string                       `json:"channels"`
	Recipients           map[AnomalySeverity][]string   `json:"recipients"`
	Templates            map[AnomalyType]string         `json:"templates"`
	RateLimiting         *RateLimitConfig               `json:"rate_limiting,omitempty"`
	EscalationRules      []EscalationRule               `json:"escalation_rules,omitempty"`
	SuppressAfterResolve bool                           `json:"suppress_after_resolve"`
	GroupSimilarAnomalies bool                          `json:"group_similar_anomalies"`
}

// RateLimitConfig contains rate limiting settings for notifications
type RateLimitConfig struct {
	MaxPerHour           int                            `json:"max_per_hour"`
	MaxPerDay            int                            `json:"max_per_day"`
	CooldownPeriod       time.Duration                  `json:"cooldown_period"`
	GroupByType          bool                           `json:"group_by_type"`
}

// EscalationRule defines when and how to escalate anomaly notifications
type EscalationRule struct {
	Condition            string                         `json:"condition"`
	Delay                time.Duration                  `json:"delay"`
	Recipients           []string                       `json:"recipients"`
	Channels             []string                       `json:"channels"`
	RequiredSeverity     AnomalySeverity                `json:"required_severity"`
}

// AnomalyRepository defines the interface for anomaly data persistence
type AnomalyRepository interface {
	// Create stores a new anomaly
	Create(ctx context.Context, anomaly *Anomaly) error
	
	// GetByID retrieves an anomaly by ID
	GetByID(ctx context.Context, id uuid.UUID) (*Anomaly, error)
	
	// Update updates an existing anomaly
	Update(ctx context.Context, anomaly *Anomaly) error
	
	// Delete removes an anomaly
	Delete(ctx context.Context, id uuid.UUID) error
	
	// List retrieves anomalies with filtering and pagination
	List(ctx context.Context, filter *AnomalyFilter) ([]*Anomaly, error)
	
	// GetByTenant retrieves anomalies for a specific tenant
	GetByTenant(ctx context.Context, tenantID uuid.UUID, filter *AnomalyFilter) ([]*Anomaly, error)
	
	// GetRelated retrieves anomalies related to a specific document/chunk
	GetRelated(ctx context.Context, documentID uuid.UUID, chunkID *uuid.UUID) ([]*Anomaly, error)
	
	// UpdateStatus updates the status of an anomaly
	UpdateStatus(ctx context.Context, id uuid.UUID, status AnomalyStatus, notes string) error
	
	// GetStatistics returns anomaly statistics for a tenant
	GetStatistics(ctx context.Context, tenantID uuid.UUID, timeWindow *TimeWindow) (*AnomalyStatistics, error)
}

// AnomalyFilter contains filtering options for anomaly queries
type AnomalyFilter struct {
	TenantID         *uuid.UUID        `json:"tenant_id,omitempty"`
	Types            []AnomalyType     `json:"types,omitempty"`
	Severities       []AnomalySeverity `json:"severities,omitempty"`
	Statuses         []AnomalyStatus   `json:"statuses,omitempty"`
	DetectorNames    []string          `json:"detector_names,omitempty"`
	TimeRange        *TimeWindow       `json:"time_range,omitempty"`
	MinScore         *float64          `json:"min_score,omitempty"`
	MaxScore         *float64          `json:"max_score,omitempty"`
	MinConfidence    *float64          `json:"min_confidence,omitempty"`
	DocumentID       *uuid.UUID        `json:"document_id,omitempty"`
	DataSourceID     *uuid.UUID        `json:"data_source_id,omitempty"`
	UserID           *uuid.UUID        `json:"user_id,omitempty"`
	Limit            int               `json:"limit,omitempty"`
	Offset           int               `json:"offset,omitempty"`
	SortBy           string            `json:"sort_by,omitempty"`
	SortOrder        string            `json:"sort_order,omitempty"`
}

// AnomalyStatistics contains statistical information about anomalies
type AnomalyStatistics struct {
	TotalCount           int64                          `json:"total_count"`
	CountByType          map[AnomalyType]int64          `json:"count_by_type"`
	CountBySeverity      map[AnomalySeverity]int64      `json:"count_by_severity"`
	CountByStatus        map[AnomalyStatus]int64        `json:"count_by_status"`
	CountByDetector      map[string]int64               `json:"count_by_detector"`
	AverageScore         float64                        `json:"average_score"`
	AverageConfidence    float64                        `json:"average_confidence"`
	ResolutionRate       float64                        `json:"resolution_rate"`
	FalsePositiveRate    float64                        `json:"false_positive_rate"`
	TimeToResolution     map[string]time.Duration       `json:"time_to_resolution"`
	TrendData            map[string][]float64           `json:"trend_data"`
	TopAnomalySources    []string                       `json:"top_anomaly_sources"`
}

// From analysis package - imported types for compatibility
type SentimentResult struct {
	Label        string  `json:"label"`
	Score        float64 `json:"score"`
	Magnitude    float64 `json:"magnitude"`
	Subjectivity float64 `json:"subjectivity"`
}

type TopicResult struct {
	Topic       string   `json:"topic"`
	Keywords    []string `json:"keywords"`
	Probability float64  `json:"probability"`
	Coherence   float64  `json:"coherence"`
}

type EntityResult struct {
	Text       string  `json:"text"`
	Label      string  `json:"label"`
	StartPos   int     `json:"start_pos"`
	EndPos     int     `json:"end_pos"`
	Confidence float64 `json:"confidence"`
	Context    string  `json:"context"`
}

type QualityMetrics struct {
	Coherence         float64  `json:"coherence"`
	Clarity           float64  `json:"clarity"`
	Completeness      float64  `json:"completeness"`
	Informativeness   float64  `json:"informativeness"`
	Redundancy        float64  `json:"redundancy"`
	StructuralQuality float64  `json:"structural_quality"`
	Issues            []string `json:"issues"`
	OverallScore      float64  `json:"overall_score"`
}

type LanguageResult struct {
	Language     string               `json:"language"`
	Dialect      string               `json:"dialect"`
	Confidence   float64              `json:"confidence"`
	Alternatives []LanguageAlternative `json:"alternatives"`
	Script       string               `json:"script"`
}

type LanguageAlternative struct {
	Language   string  `json:"language"`
	Confidence float64 `json:"confidence"`
}