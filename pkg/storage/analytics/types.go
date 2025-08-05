package analytics

import (
	"time"

	"github.com/google/uuid"
)

// AnalyticsConfig contains configuration for storage analytics
type AnalyticsConfig struct {
	// Collection intervals
	CollectionInterval    time.Duration `yaml:"collection_interval"`
	PerformanceInterval   time.Duration `yaml:"performance_interval"`
	CostInterval          time.Duration `yaml:"cost_interval"`
	AccessPatternInterval time.Duration `yaml:"access_pattern_interval"`
	HealthInterval        time.Duration `yaml:"health_interval"`

	// Aggregation settings
	AggregationInterval time.Duration `yaml:"aggregation_interval"`
	RetentionPeriod     time.Duration `yaml:"retention_period"`

	// Cache settings
	CacheTTL             time.Duration `yaml:"cache_ttl"`
	ReportCacheTTL       time.Duration `yaml:"report_cache_ttl"`
	CacheCleanupInterval time.Duration `yaml:"cache_cleanup_interval"`

	// Analysis settings
	TrendAnalysisEnabled bool `yaml:"trend_analysis_enabled"`
	PredictionEnabled    bool `yaml:"prediction_enabled"`
	AlertsEnabled        bool `yaml:"alerts_enabled"`

	// Storage settings
	MaxSamplesPerBatch int  `yaml:"max_samples_per_batch"`
	CompressionEnabled bool `yaml:"compression_enabled"`
}

// DefaultAnalyticsConfig returns default analytics configuration
func DefaultAnalyticsConfig() *AnalyticsConfig {
	return &AnalyticsConfig{
		CollectionInterval:    5 * time.Minute,
		PerformanceInterval:   1 * time.Minute,
		CostInterval:          15 * time.Minute,
		AccessPatternInterval: 10 * time.Minute,
		HealthInterval:        2 * time.Minute,
		AggregationInterval:   15 * time.Minute,
		RetentionPeriod:       90 * 24 * time.Hour, // 90 days
		CacheTTL:              10 * time.Minute,
		ReportCacheTTL:        30 * time.Minute,
		CacheCleanupInterval:  1 * time.Hour,
		TrendAnalysisEnabled:  true,
		PredictionEnabled:     true,
		AlertsEnabled:         true,
		MaxSamplesPerBatch:    1000,
		CompressionEnabled:    true,
	}
}

// MetricType represents different types of metrics
type MetricType string

const (
	MetricTypeStorage       MetricType = "storage"
	MetricTypePerformance   MetricType = "performance"
	MetricTypeCost          MetricType = "cost"
	MetricTypeAccessPattern MetricType = "access_pattern"
	MetricTypeHealth        MetricType = "health"
	MetricTypeCustom        MetricType = "custom"
)

// MetricSample represents a single metric measurement
type MetricSample struct {
	ID         uuid.UUID              `json:"id"`
	TenantID   uuid.UUID              `json:"tenant_id"`
	MetricType MetricType             `json:"metric_type"`
	MetricName string                 `json:"metric_name"`
	Value      float64                `json:"value"`
	Unit       string                 `json:"unit"`
	Timestamp  time.Time              `json:"timestamp"`
	Labels     map[string]string      `json:"labels,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Source     string                 `json:"source"`
}

// AggregatedMetrics represents aggregated metric data
type AggregatedMetrics struct {
	ID         uuid.UUID     `json:"id"`
	TenantID   uuid.UUID     `json:"tenant_id"`
	MetricType MetricType    `json:"metric_type"`
	MetricName string        `json:"metric_name"`
	TimeWindow time.Duration `json:"time_window"`
	StartTime  time.Time     `json:"start_time"`
	EndTime    time.Time     `json:"end_time"`

	// Aggregated values
	Count       int64              `json:"count"`
	Sum         float64            `json:"sum"`
	Average     float64            `json:"average"`
	Min         float64            `json:"min"`
	Max         float64            `json:"max"`
	StdDev      float64            `json:"std_dev"`
	Percentiles map[string]float64 `json:"percentiles"` // P50, P95, P99

	Unit     string                 `json:"unit"`
	Labels   map[string]string      `json:"labels,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// TimeRange represents a time range for queries
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// MetricsQuery represents a query for metric data
type MetricsQuery struct {
	TenantID   uuid.UUID         `json:"tenant_id,omitempty"`
	MetricType MetricType        `json:"metric_type,omitempty"`
	MetricName string            `json:"metric_name,omitempty"`
	TimeRange  TimeRange         `json:"time_range"`
	Labels     map[string]string `json:"labels,omitempty"`
	Limit      int               `json:"limit,omitempty"`
	Offset     int               `json:"offset,omitempty"`
	OrderBy    string            `json:"order_by,omitempty"`
	OrderDesc  bool              `json:"order_desc,omitempty"`
}

// AggregationQuery represents a query for aggregated metrics
type AggregationQuery struct {
	TenantID   uuid.UUID         `json:"tenant_id,omitempty"`
	MetricType MetricType        `json:"metric_type,omitempty"`
	MetricName string            `json:"metric_name,omitempty"`
	TimeRange  TimeRange         `json:"time_range"`
	TimeWindow time.Duration     `json:"time_window"`
	Labels     map[string]string `json:"labels,omitempty"`
	Limit      int               `json:"limit,omitempty"`
	Offset     int               `json:"offset,omitempty"`
}

// UsageReport represents a comprehensive usage report
type UsageReport struct {
	TenantID    uuid.UUID `json:"tenant_id"`
	ReportID    uuid.UUID `json:"report_id"`
	GeneratedAt time.Time `json:"generated_at"`
	TimeRange   TimeRange `json:"time_range"`

	// Storage metrics
	TotalFiles     int64            `json:"total_files"`
	TotalSizeBytes int64            `json:"total_size_bytes"`
	TotalSizeGB    float64          `json:"total_size_gb"`
	FilesByType    map[string]int64 `json:"files_by_type"`
	SizeByType     map[string]int64 `json:"size_by_type"`

	// Storage tier distribution
	TierDistribution map[string]*TierUsage `json:"tier_distribution"`

	// Access patterns
	AccessMetrics *AccessMetrics `json:"access_metrics"`

	// Performance metrics
	PerformanceMetrics *PerformanceMetrics `json:"performance_metrics"`

	// Cost metrics
	TotalCost     float64            `json:"total_cost"`
	CostByTier    map[string]float64 `json:"cost_by_tier"`
	CostByType    map[string]float64 `json:"cost_by_type"`
	CostBreakdown *CostBreakdown     `json:"cost_breakdown"`

	// Growth and trends
	Trends *UsageTrends `json:"trends,omitempty"`

	// Predictions
	CostPredictions *CostPredictions `json:"cost_predictions,omitempty"`

	// Recommendations
	Recommendations []OptimizationRecommendation `json:"recommendations,omitempty"`

	// Compliance and governance
	ComplianceMetrics *ComplianceMetrics `json:"compliance_metrics,omitempty"`
}

// TierUsage represents usage statistics for a storage tier
type TierUsage struct {
	FileCount       int64     `json:"file_count"`
	TotalSize       int64     `json:"total_size"`
	AverageFileSize int64     `json:"average_file_size"`
	LastAccessed    time.Time `json:"last_accessed"`
	MonthlyCost     float64   `json:"monthly_cost"`
	CostPerGB       float64   `json:"cost_per_gb"`
}

// AccessMetrics represents file access pattern metrics
type AccessMetrics struct {
	TotalAccesses    int64            `json:"total_accesses"`
	UniqueFiles      int64            `json:"unique_files"`
	ReadAccesses     int64            `json:"read_accesses"`
	WriteAccesses    int64            `json:"write_accesses"`
	DeleteAccesses   int64            `json:"delete_accesses"`
	AccessesByHour   map[int]int64    `json:"accesses_by_hour"`
	AccessesByDay    map[string]int64 `json:"accesses_by_day"`
	TopAccessedFiles []FileAccessStat `json:"top_accessed_files"`
	AccessPatterns   map[string]int64 `json:"access_patterns"` // frequent, regular, infrequent, rare
}

// FileAccessStat represents access statistics for a single file
type FileAccessStat struct {
	FilePath     string    `json:"file_path"`
	AccessCount  int64     `json:"access_count"`
	LastAccessed time.Time `json:"last_accessed"`
	FileSize     int64     `json:"file_size"`
	StorageTier  string    `json:"storage_tier"`
}

// PerformanceMetrics represents storage performance metrics
type PerformanceMetrics struct {
	AverageThroughput   float64       `json:"average_throughput_mbps"`
	PeakThroughput      float64       `json:"peak_throughput_mbps"`
	AverageIOPS         float64       `json:"average_iops"`
	PeakIOPS            float64       `json:"peak_iops"`
	AverageLatency      time.Duration `json:"average_latency"`
	P95Latency          time.Duration `json:"p95_latency"`
	P99Latency          time.Duration `json:"p99_latency"`
	ErrorRate           float64       `json:"error_rate"`
	AvailabilityPercent float64       `json:"availability_percent"`
	ConcurrentOps       int64         `json:"concurrent_operations"`
}

// CostBreakdown represents detailed cost breakdown
type CostBreakdown struct {
	StorageCosts    float64 `json:"storage_costs"`
	TransferCosts   float64 `json:"transfer_costs"`
	RequestCosts    float64 `json:"request_costs"`
	RetrievalCosts  float64 `json:"retrieval_costs"`
	ManagementCosts float64 `json:"management_costs"`
	TaxesAndFees    float64 `json:"taxes_and_fees"`
}

// UsageTrends represents usage trends and growth patterns
type UsageTrends struct {
	StorageGrowthRate  float64 `json:"storage_growth_rate"`   // Monthly growth rate
	CostGrowthRate     float64 `json:"cost_growth_rate"`      // Monthly cost growth rate
	AccessPatternTrend string  `json:"access_pattern_trend"`  // increasing, stable, decreasing
	FileCountTrend     string  `json:"file_count_trend"`      // increasing, stable, decreasing
	PredictedUsage     int64   `json:"predicted_usage_bytes"` // Predicted usage next month
	TrendConfidence    float64 `json:"trend_confidence"`      // Confidence score 0-1
}

// CostPredictions represents cost predictions
type CostPredictions struct {
	NextMonthCost      float64 `json:"next_month_cost"`
	NextQuarterCost    float64 `json:"next_quarter_cost"`
	NextYearCost       float64 `json:"next_year_cost"`
	PredictionAccuracy float64 `json:"prediction_accuracy"` // Historical accuracy 0-1
}

// OptimizationRecommendation represents an optimization recommendation
type OptimizationRecommendation struct {
	Type             string        `json:"type"` // tier_optimization, compression, cleanup, etc.
	Title            string        `json:"title"`
	Description      string        `json:"description"`
	PotentialSavings float64       `json:"potential_savings"` // Monthly savings in currency
	Confidence       float64       `json:"confidence"`        // Confidence score 0-1
	Priority         string        `json:"priority"`          // high, medium, low
	Impact           string        `json:"impact"`            // high, medium, low
	Effort           string        `json:"effort"`            // high, medium, low
	Category         string        `json:"category"`          // cost, performance, compliance
	Actions          []string      `json:"actions"`           // List of recommended actions
	EstimatedTime    time.Duration `json:"estimated_time"`    // Time to implement
}

// ComplianceMetrics represents compliance and governance metrics
type ComplianceMetrics struct {
	DataRetentionCompliance float64            `json:"data_retention_compliance"` // Percentage 0-100
	EncryptionCompliance    float64            `json:"encryption_compliance"`     // Percentage 0-100
	BackupCompliance        float64            `json:"backup_compliance"`         // Percentage 0-100
	AccessControlCompliance float64            `json:"access_control_compliance"` // Percentage 0-100
	AuditTrailCompleteness  float64            `json:"audit_trail_completeness"`  // Percentage 0-100
	PolicyViolations        int64              `json:"policy_violations"`
	ComplianceScore         float64            `json:"compliance_score"` // Overall score 0-100
	ComplianceByCategory    map[string]float64 `json:"compliance_by_category"`
	RiskLevel               string             `json:"risk_level"` // low, medium, high, critical
}

// RealTimeMetrics represents real-time storage metrics
type RealTimeMetrics struct {
	TenantID  uuid.UUID `json:"tenant_id"`
	Timestamp time.Time `json:"timestamp"`

	// Current usage
	TotalFiles        int64   `json:"total_files"`
	TotalSizeBytes    int64   `json:"total_size_bytes"`
	UsedCapacity      float64 `json:"used_capacity"` // Percentage 0-1
	AvailableCapacity int64   `json:"available_capacity"`

	// Performance
	ThroughputMBps   float64       `json:"throughput_mbps"`
	IOPS             int64         `json:"iops"`
	Latency          time.Duration `json:"latency"`
	ActiveOperations int64         `json:"active_operations"`
	QueuedOperations int64         `json:"queued_operations"`

	// Cost
	CurrentMonthlyCost   float64 `json:"current_monthly_cost"`
	ProjectedMonthlyCost float64 `json:"projected_monthly_cost"`
	DailyCostRun         float64 `json:"daily_cost_run"`

	// Health
	ErrorRate           float64 `json:"error_rate"`
	AvailabilityPercent float64 `json:"availability_percent"`
	HealthScore         float64 `json:"health_score"` // 0-100

	// Alerts
	ActiveAlerts   int `json:"active_alerts"`
	CriticalAlerts int `json:"critical_alerts"`
	WarningAlerts  int `json:"warning_alerts"`
}

// AlertCondition represents an alert condition
type AlertCondition struct {
	ID                   uuid.UUID         `json:"id"`
	Name                 string            `json:"name"`
	Description          string            `json:"description"`
	MetricType           MetricType        `json:"metric_type"`
	MetricName           string            `json:"metric_name"`
	Operator             string            `json:"operator"` // gt, lt, eq, ne, gte, lte
	Threshold            float64           `json:"threshold"`
	TimeWindow           time.Duration     `json:"time_window"`
	Severity             string            `json:"severity"` // critical, warning, info
	Enabled              bool              `json:"enabled"`
	NotificationChannels []string          `json:"notification_channels"`
	Labels               map[string]string `json:"labels,omitempty"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

// Alert represents a triggered alert
type Alert struct {
	ID             uuid.UUID              `json:"id"`
	ConditionID    uuid.UUID              `json:"condition_id"`
	TenantID       uuid.UUID              `json:"tenant_id"`
	MetricType     MetricType             `json:"metric_type"`
	MetricName     string                 `json:"metric_name"`
	CurrentValue   float64                `json:"current_value"`
	ThresholdValue float64                `json:"threshold_value"`
	Severity       string                 `json:"severity"`
	Status         string                 `json:"status"` // active, resolved, suppressed
	Message        string                 `json:"message"`
	TriggeredAt    time.Time              `json:"triggered_at"`
	ResolvedAt     *time.Time             `json:"resolved_at,omitempty"`
	AcknowledgedAt *time.Time             `json:"acknowledged_at,omitempty"`
	AcknowledgedBy string                 `json:"acknowledged_by,omitempty"`
	Labels         map[string]string      `json:"labels,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// Dashboard represents a metrics dashboard configuration
type Dashboard struct {
	ID          uuid.UUID       `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	TenantID    uuid.UUID       `json:"tenant_id"`
	Widgets     []Widget        `json:"widgets"`
	Layout      DashboardLayout `json:"layout"`
	IsPublic    bool            `json:"is_public"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	CreatedBy   string          `json:"created_by"`
}

// Widget represents a dashboard widget
type Widget struct {
	ID            uuid.UUID              `json:"id"`
	Type          string                 `json:"type"` // chart, table, stat, gauge
	Title         string                 `json:"title"`
	Query         *MetricsQuery          `json:"query"`
	Visualization string                 `json:"visualization"` // line, bar, pie, table, etc.
	Position      WidgetPosition         `json:"position"`
	Size          WidgetSize             `json:"size"`
	Options       map[string]interface{} `json:"options,omitempty"`
}

// WidgetPosition represents widget position on dashboard
type WidgetPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// WidgetSize represents widget size
type WidgetSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// DashboardLayout represents dashboard layout configuration
type DashboardLayout struct {
	Columns     int    `json:"columns"`
	RowHeight   int    `json:"row_height"`
	Margin      int    `json:"margin"`
	Padding     int    `json:"padding"`
	AutoResize  bool   `json:"auto_resize"`
	RefreshRate string `json:"refresh_rate"` // 30s, 1m, 5m, etc.
}
