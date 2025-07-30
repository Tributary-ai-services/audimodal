package analytics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// UsageMonitor provides comprehensive storage usage monitoring and alerting
type UsageMonitor struct {
	config         *AnalyticsConfig
	metricsStore   MetricsStore
	alertStore     AlertStore
	notifier       AlertNotifier
	tracer         trace.Tracer

	// Alert management
	alertConditions map[uuid.UUID]*AlertCondition
	activeAlerts    map[uuid.UUID]*Alert
	alertRules      []*AlertRule

	// Monitoring state
	isRunning       bool
	stopCh          chan struct{}
	wg              sync.WaitGroup
	mu              sync.RWMutex

	// Thresholds and limits
	thresholds      *MonitoringThresholds
	usageLimits     *UsageLimits

	// Analytics
	trendAnalyzer   *TrendAnalyzer
	anomalyDetector *AnomalyDetector
}

// AlertStore interface for persistent alert storage
type AlertStore interface {
	StoreAlert(ctx context.Context, alert *Alert) error
	UpdateAlert(ctx context.Context, alert *Alert) error
	GetAlert(ctx context.Context, alertID uuid.UUID) (*Alert, error)
	ListActiveAlerts(ctx context.Context, tenantID uuid.UUID) ([]*Alert, error)
	GetAlertHistory(ctx context.Context, filters AlertHistoryFilters) ([]*Alert, error)
	
	StoreAlertCondition(ctx context.Context, condition *AlertCondition) error
	UpdateAlertCondition(ctx context.Context, condition *AlertCondition) error
	GetAlertCondition(ctx context.Context, conditionID uuid.UUID) (*AlertCondition, error)
	ListAlertConditions(ctx context.Context, tenantID uuid.UUID) ([]*AlertCondition, error)
}

// AlertNotifier interface for sending alert notifications
type AlertNotifier interface {
	SendAlert(ctx context.Context, alert *Alert, channels []string) error
	SendAlertResolved(ctx context.Context, alert *Alert, channels []string) error
}

// AlertRule represents a rule for generating alerts
type AlertRule struct {
	ID              uuid.UUID         `json:"id"`
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	MetricQuery     *MetricsQuery     `json:"metric_query"`
	Condition       string            `json:"condition"`       // expression like "value > 1000"
	TimeWindow      time.Duration     `json:"time_window"`
	EvaluationInterval time.Duration  `json:"evaluation_interval"`
	Severity        string            `json:"severity"`
	Enabled         bool              `json:"enabled"`
	NotificationChannels []string     `json:"notification_channels"`
	Labels          map[string]string `json:"labels,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// MonitoringThresholds defines various monitoring thresholds
type MonitoringThresholds struct {
	StorageUsageWarning     float64 `json:"storage_usage_warning"`     // 80%
	StorageUsageCritical    float64 `json:"storage_usage_critical"`    // 95%
	CostIncreaseWarning     float64 `json:"cost_increase_warning"`     // 20%
	CostIncreaseCritical    float64 `json:"cost_increase_critical"`    // 50%
	PerformanceDegradation  float64 `json:"performance_degradation"`   // 30%
	ErrorRateWarning        float64 `json:"error_rate_warning"`        // 5%
	ErrorRateCritical       float64 `json:"error_rate_critical"`       // 10%
	LatencyWarningMs        int64   `json:"latency_warning_ms"`        // 1000ms
	LatencyCriticalMs       int64   `json:"latency_critical_ms"`       // 5000ms
}

// UsageLimits defines usage limits and quotas
type UsageLimits struct {
	MaxStorageBytes         int64   `json:"max_storage_bytes"`
	MaxFiles               int64   `json:"max_files"`
	MaxMonthlyCost         float64 `json:"max_monthly_cost"`
	MaxDailyCost           float64 `json:"max_daily_cost"`
	MaxTransferBytesDaily  int64   `json:"max_transfer_bytes_daily"`
	MaxOperationsPerSecond int64   `json:"max_operations_per_second"`
}

// AlertHistoryFilters for querying alert history
type AlertHistoryFilters struct {
	TenantID    *uuid.UUID  `json:"tenant_id,omitempty"`
	Severity    *string     `json:"severity,omitempty"`
	Status      *string     `json:"status,omitempty"`
	StartTime   *time.Time  `json:"start_time,omitempty"`
	EndTime     *time.Time  `json:"end_time,omitempty"`
	Limit       int         `json:"limit,omitempty"`
	Offset      int         `json:"offset,omitempty"`
}

// NewUsageMonitor creates a new usage monitor
func NewUsageMonitor(
	config *AnalyticsConfig,
	metricsStore MetricsStore,
	alertStore AlertStore,
	notifier AlertNotifier,
) *UsageMonitor {
	if config == nil {
		config = DefaultAnalyticsConfig()
	}

	monitor := &UsageMonitor{
		config:          config,
		metricsStore:    metricsStore,
		alertStore:      alertStore,
		notifier:        notifier,
		tracer:          otel.Tracer("usage-monitor"),
		alertConditions: make(map[uuid.UUID]*AlertCondition),
		activeAlerts:    make(map[uuid.UUID]*Alert),
		stopCh:          make(chan struct{}),
		thresholds:      DefaultMonitoringThresholds(),
		usageLimits:     DefaultUsageLimits(),
		trendAnalyzer:   NewTrendAnalyzer(),
		anomalyDetector: NewAnomalyDetector(),
	}

	// Initialize default alert rules
	monitor.initializeDefaultAlertRules()

	return monitor
}

// Start starts the usage monitor
func (m *UsageMonitor) Start(ctx context.Context) error {
	ctx, span := m.tracer.Start(ctx, "start_usage_monitor")
	defer span.End()

	m.mu.Lock()
	if m.isRunning {
		m.mu.Unlock()
		return fmt.Errorf("usage monitor is already running")
	}
	m.isRunning = true
	m.mu.Unlock()

	// Load existing alert conditions
	if err := m.loadAlertConditions(ctx); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to load alert conditions: %w", err)
	}

	// Start monitoring goroutines
	m.wg.Add(1)
	go m.runAlertEvaluation(ctx)

	m.wg.Add(1)
	go m.runUsageAnalysis(ctx)

	m.wg.Add(1)
	go m.runTrendAnalysis(ctx)

	m.wg.Add(1)
	go m.runAnomalyDetection(ctx)

	span.SetAttributes(
		attribute.Int("alert_conditions", len(m.alertConditions)),
		attribute.Bool("alerts_enabled", m.config.AlertsEnabled),
	)

	return nil
}

// Stop stops the usage monitor
func (m *UsageMonitor) Stop(ctx context.Context) error {
	ctx, span := m.tracer.Start(ctx, "stop_usage_monitor")
	defer span.End()

	m.mu.Lock()
	if !m.isRunning {
		m.mu.Unlock()
		return fmt.Errorf("usage monitor is not running")
	}
	m.isRunning = false
	m.mu.Unlock()

	// Signal shutdown
	close(m.stopCh)

	// Wait for goroutines to finish
	m.wg.Wait()

	return nil
}

// CreateAlertCondition creates a new alert condition
func (m *UsageMonitor) CreateAlertCondition(ctx context.Context, condition *AlertCondition) error {
	ctx, span := m.tracer.Start(ctx, "create_alert_condition")
	defer span.End()

	if condition.ID == uuid.Nil {
		condition.ID = uuid.New()
	}

	condition.CreatedAt = time.Now()
	condition.UpdatedAt = time.Now()

	// Validate condition
	if err := m.validateAlertCondition(condition); err != nil {
		span.RecordError(err)
		return fmt.Errorf("invalid alert condition: %w", err)
	}

	// Store condition
	if err := m.alertStore.StoreAlertCondition(ctx, condition); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to store alert condition: %w", err)
	}

	// Add to active conditions
	m.mu.Lock()
	m.alertConditions[condition.ID] = condition
	m.mu.Unlock()

	span.SetAttributes(
		attribute.String("condition.id", condition.ID.String()),
		attribute.String("condition.name", condition.Name),
		attribute.String("metric.type", string(condition.MetricType)),
	)

	return nil
}

// CheckUsageLimits checks if usage limits are exceeded
func (m *UsageMonitor) CheckUsageLimits(ctx context.Context, tenantID uuid.UUID) (*UsageLimitStatus, error) {
	ctx, span := m.tracer.Start(ctx, "check_usage_limits")
	defer span.End()

	span.SetAttributes(attribute.String("tenant.id", tenantID.String()))

	status := &UsageLimitStatus{
		TenantID:    tenantID,
		CheckedAt:   time.Now(),
		Violations:  []UsageLimitViolation{},
		OverallStatus: "ok",
	}

	// Check storage usage
	if err := m.checkStorageLimit(ctx, tenantID, status); err != nil {
		span.RecordError(err)
	}

	// Check cost limits
	if err := m.checkCostLimits(ctx, tenantID, status); err != nil {
		span.RecordError(err)
	}

	// Check file count limits
	if err := m.checkFileCountLimit(ctx, tenantID, status); err != nil {
		span.RecordError(err)
	}

	// Check operations per second limits
	if err := m.checkOpsLimit(ctx, tenantID, status); err != nil {
		span.RecordError(err)
	}

	// Determine overall status
	if len(status.Violations) > 0 {
		hasWarning := false
		hasCritical := false
		for _, violation := range status.Violations {
			if violation.Severity == "critical" {
				hasCritical = true
			} else if violation.Severity == "warning" {
				hasWarning = true
			}
		}
		
		if hasCritical {
			status.OverallStatus = "critical"
		} else if hasWarning {
			status.OverallStatus = "warning"
		}
	}

	span.SetAttributes(
		attribute.String("overall.status", status.OverallStatus),
		attribute.Int("violations.count", len(status.Violations)),
	)

	return status, nil
}

// GetUsageInsights provides usage insights and recommendations
func (m *UsageMonitor) GetUsageInsights(ctx context.Context, tenantID uuid.UUID) (*UsageInsights, error) {
	ctx, span := m.tracer.Start(ctx, "get_usage_insights")
	defer span.End()

	insights := &UsageInsights{
		TenantID:      tenantID,
		GeneratedAt:   time.Now(),
		Recommendations: []UsageRecommendation{},
		Patterns:       []UsagePattern{},
		Predictions:    []UsagePrediction{},
	}

	// Analyze usage trends
	if err := m.analyzeUsageTrends(ctx, tenantID, insights); err != nil {
		span.RecordError(err)
	}

	// Detect usage patterns
	if err := m.detectUsagePatterns(ctx, tenantID, insights); err != nil {
		span.RecordError(err)
	}

	// Generate predictions
	if err := m.generateUsagePredictions(ctx, tenantID, insights); err != nil {
		span.RecordError(err)
	}

	// Generate recommendations
	if err := m.generateUsageRecommendations(ctx, tenantID, insights); err != nil {
		span.RecordError(err)
	}

	span.SetAttributes(
		attribute.String("tenant.id", tenantID.String()),
		attribute.Int("recommendations.count", len(insights.Recommendations)),
		attribute.Int("patterns.count", len(insights.Patterns)),
	)

	return insights, nil
}

// Helper methods

func (m *UsageMonitor) initializeDefaultAlertRules() {
	m.alertRules = []*AlertRule{
		{
			ID:          uuid.New(),
			Name:        "High Storage Usage",
			Description: "Alert when storage usage exceeds 80%",
			MetricQuery: &MetricsQuery{
				MetricType: MetricTypeStorage,
				MetricName: "storage_usage_percent",
			},
			Condition:          "value > 80",
			TimeWindow:         15 * time.Minute,
			EvaluationInterval: 5 * time.Minute,
			Severity:           "warning",
			Enabled:            true,
		},
		{
			ID:          uuid.New(),
			Name:        "High Cost Increase",
			Description: "Alert when cost increases by more than 20%",
			MetricQuery: &MetricsQuery{
				MetricType: MetricTypeCost,
				MetricName: "monthly_cost_change_percent",
			},
			Condition:          "value > 20",
			TimeWindow:         24 * time.Hour,
			EvaluationInterval: 1 * time.Hour,
			Severity:           "warning",
			Enabled:            true,
		},
		{
			ID:          uuid.New(),
			Name:        "High Error Rate",
			Description: "Alert when error rate exceeds 5%",
			MetricQuery: &MetricsQuery{
				MetricType: MetricTypePerformance,
				MetricName: "error_rate",
			},
			Condition:          "value > 5",
			TimeWindow:         10 * time.Minute,
			EvaluationInterval: 2 * time.Minute,
			Severity:           "critical",
			Enabled:            true,
		},
	}
}

func (m *UsageMonitor) loadAlertConditions(ctx context.Context) error {
	// This would load from the alert store
	// For now, using placeholder implementation
	return nil
}

func (m *UsageMonitor) runAlertEvaluation(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(1 * time.Minute) // Evaluate alerts every minute
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.evaluateAlerts(ctx)
		}
	}
}

func (m *UsageMonitor) evaluateAlerts(ctx context.Context) {
	m.mu.RLock()
	conditions := make([]*AlertCondition, 0, len(m.alertConditions))
	for _, condition := range m.alertConditions {
		if condition.Enabled {
			conditions = append(conditions, condition)
		}
	}
	m.mu.RUnlock()

	for _, condition := range conditions {
		go m.evaluateAlertCondition(ctx, condition)
	}
}

func (m *UsageMonitor) evaluateAlertCondition(ctx context.Context, condition *AlertCondition) {
	ctx, span := m.tracer.Start(ctx, "evaluate_alert_condition")
	defer span.End()

	span.SetAttributes(
		attribute.String("condition.id", condition.ID.String()),
		attribute.String("condition.name", condition.Name),
	)

	// Query metrics for the condition
	query := &MetricsQuery{
		MetricType: condition.MetricType,
		MetricName: condition.MetricName,
		TimeRange: TimeRange{
			Start: time.Now().Add(-condition.TimeWindow),
			End:   time.Now(),
		},
		Labels: condition.Labels,
	}

	samples, err := m.metricsStore.GetMetrics(ctx, query)
	if err != nil {
		span.RecordError(err)
		return
	}

	if len(samples) == 0 {
		return
	}

	// Evaluate condition against latest sample
	latestSample := samples[len(samples)-1]
	shouldAlert := m.evaluateConditionThreshold(condition, latestSample.Value)

	// Check if alert already exists
	m.mu.RLock()
	existingAlert, exists := m.activeAlerts[condition.ID]
	m.mu.RUnlock()

	if shouldAlert && !exists {
		// Create new alert
		alert := &Alert{
			ID:             uuid.New(),
			ConditionID:    condition.ID,
			TenantID:       latestSample.TenantID,
			MetricType:     condition.MetricType,
			MetricName:     condition.MetricName,
			CurrentValue:   latestSample.Value,
			ThresholdValue: condition.Threshold,
			Severity:       condition.Severity,
			Status:         "active",
			Message:        fmt.Sprintf("%s: %s is %.2f (threshold: %.2f)", condition.Name, condition.MetricName, latestSample.Value, condition.Threshold),
			TriggeredAt:    time.Now(),
			Labels:         condition.Labels,
		}

		if err := m.alertStore.StoreAlert(ctx, alert); err != nil {
			span.RecordError(err)
			return
		}

		m.mu.Lock()
		m.activeAlerts[condition.ID] = alert
		m.mu.Unlock()

		// Send notification
		if m.notifier != nil {
			go m.notifier.SendAlert(ctx, alert, condition.NotificationChannels)
		}

	} else if !shouldAlert && exists && existingAlert.Status == "active" {
		// Resolve existing alert
		now := time.Now()
		existingAlert.Status = "resolved"
		existingAlert.ResolvedAt = &now

		if err := m.alertStore.UpdateAlert(ctx, existingAlert); err != nil {
			span.RecordError(err)
			return
		}

		m.mu.Lock()
		delete(m.activeAlerts, condition.ID)
		m.mu.Unlock()

		// Send resolution notification
		if m.notifier != nil {
			go m.notifier.SendAlertResolved(ctx, existingAlert, condition.NotificationChannels)
		}
	}
}

func (m *UsageMonitor) runUsageAnalysis(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(15 * time.Minute) // Analyze usage every 15 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.analyzeUsage(ctx)
		}
	}
}

func (m *UsageMonitor) analyzeUsage(ctx context.Context) {
	// Placeholder for usage analysis logic
	// This would analyze current usage patterns and generate insights
}

func (m *UsageMonitor) runTrendAnalysis(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(1 * time.Hour) // Trend analysis every hour
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			if m.config.TrendAnalysisEnabled {
				m.trendAnalyzer.AnalyzeTrends(ctx)
			}
		}
	}
}

func (m *UsageMonitor) runAnomalyDetection(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(10 * time.Minute) // Anomaly detection every 10 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.anomalyDetector.DetectAnomalies(ctx)
		}
	}
}

func (m *UsageMonitor) validateAlertCondition(condition *AlertCondition) error {
	if condition.Name == "" {
		return fmt.Errorf("condition name is required")
	}
	if condition.MetricType == "" {
		return fmt.Errorf("metric type is required")
	}
	if condition.MetricName == "" {
		return fmt.Errorf("metric name is required")
	}
	if condition.Operator == "" {
		return fmt.Errorf("operator is required")
	}
	if condition.TimeWindow <= 0 {
		return fmt.Errorf("time window must be positive")
	}
	return nil
}

func (m *UsageMonitor) evaluateConditionThreshold(condition *AlertCondition, value float64) bool {
	switch condition.Operator {
	case "gt", "greater_than":
		return value > condition.Threshold
	case "lt", "less_than":
		return value < condition.Threshold
	case "eq", "equals":
		return value == condition.Threshold
	case "ne", "not_equals":
		return value != condition.Threshold
	case "gte", "greater_equal":
		return value >= condition.Threshold
	case "lte", "less_equal":
		return value <= condition.Threshold
	default:
		return false
	}
}

// Default configurations

func DefaultMonitoringThresholds() *MonitoringThresholds {
	return &MonitoringThresholds{
		StorageUsageWarning:    80.0,
		StorageUsageCritical:   95.0,
		CostIncreaseWarning:    20.0,
		CostIncreaseCritical:   50.0,
		PerformanceDegradation: 30.0,
		ErrorRateWarning:       5.0,
		ErrorRateCritical:      10.0,
		LatencyWarningMs:       1000,
		LatencyCriticalMs:      5000,
	}
}

func DefaultUsageLimits() *UsageLimits {
	return &UsageLimits{
		MaxStorageBytes:        1024 * 1024 * 1024 * 1024, // 1TB
		MaxFiles:              1000000,                     // 1M files
		MaxMonthlyCost:         10000.0,                    // $10,000
		MaxDailyCost:           500.0,                      // $500
		MaxTransferBytesDaily:  100 * 1024 * 1024 * 1024,  // 100GB daily
		MaxOperationsPerSecond: 1000,                       // 1000 ops/sec
	}
}

// Placeholder implementations for checking limits
func (m *UsageMonitor) checkStorageLimit(ctx context.Context, tenantID uuid.UUID, status *UsageLimitStatus) error {
	// Placeholder implementation
	return nil
}

func (m *UsageMonitor) checkCostLimits(ctx context.Context, tenantID uuid.UUID, status *UsageLimitStatus) error {
	// Placeholder implementation
	return nil
}

func (m *UsageMonitor) checkFileCountLimit(ctx context.Context, tenantID uuid.UUID, status *UsageLimitStatus) error {
	// Placeholder implementation
	return nil
}

func (m *UsageMonitor) checkOpsLimit(ctx context.Context, tenantID uuid.UUID, status *UsageLimitStatus) error {
	// Placeholder implementation
	return nil
}

// Placeholder analysis methods
func (m *UsageMonitor) analyzeUsageTrends(ctx context.Context, tenantID uuid.UUID, insights *UsageInsights) error {
	return nil
}

func (m *UsageMonitor) detectUsagePatterns(ctx context.Context, tenantID uuid.UUID, insights *UsageInsights) error {
	return nil
}

func (m *UsageMonitor) generateUsagePredictions(ctx context.Context, tenantID uuid.UUID, insights *UsageInsights) error {
	return nil
}

func (m *UsageMonitor) generateUsageRecommendations(ctx context.Context, tenantID uuid.UUID, insights *UsageInsights) error {
	return nil
}

// Placeholder types for insights
type UsageLimitStatus struct {
	TenantID      uuid.UUID              `json:"tenant_id"`
	CheckedAt     time.Time              `json:"checked_at"`
	OverallStatus string                 `json:"overall_status"`
	Violations    []UsageLimitViolation  `json:"violations"`
}

type UsageLimitViolation struct {
	Type        string    `json:"type"`
	CurrentValue float64  `json:"current_value"`
	LimitValue   float64  `json:"limit_value"`
	Severity     string   `json:"severity"`
	Message      string   `json:"message"`
	DetectedAt   time.Time `json:"detected_at"`
}

type UsageInsights struct {
	TenantID        uuid.UUID              `json:"tenant_id"`
	GeneratedAt     time.Time              `json:"generated_at"`
	Recommendations []UsageRecommendation  `json:"recommendations"`
	Patterns        []UsagePattern         `json:"patterns"`
	Predictions     []UsagePrediction      `json:"predictions"`
}

type UsageRecommendation struct {
	Type            string    `json:"type"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	Priority        string    `json:"priority"`
	EstimatedSavings float64  `json:"estimated_savings"`
	Confidence      float64   `json:"confidence"`
}

type UsagePattern struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Confidence  float64                `json:"confidence"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type UsagePrediction struct {
	Metric      string    `json:"metric"`
	CurrentValue float64  `json:"current_value"`
	PredictedValue float64 `json:"predicted_value"`
	TimeHorizon string    `json:"time_horizon"`
	Confidence  float64   `json:"confidence"`
}

// Placeholder analyzer types
type TrendAnalyzer struct{}
func NewTrendAnalyzer() *TrendAnalyzer { return &TrendAnalyzer{} }
func (ta *TrendAnalyzer) AnalyzeTrends(ctx context.Context) error { return nil }

type AnomalyDetector struct{}
func NewAnomalyDetector() *AnomalyDetector { return &AnomalyDetector{} }
func (ad *AnomalyDetector) DetectAnomalies(ctx context.Context) error { return nil }