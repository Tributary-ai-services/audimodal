package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// SyncMetricsCollector collects and aggregates sync metrics
type SyncMetricsCollector struct {
	db     *sql.DB
	config *MetricsConfig
	
	// In-memory metrics for real-time access
	realtimeMetrics map[string]*RealtimeMetrics
	metricsMutex    sync.RWMutex
	
	// Background aggregation
	aggregationTicker *time.Ticker
	stopChan          chan struct{}
	
	tracer trace.Tracer
}

// MetricsConfig contains metrics collection configuration
type MetricsConfig struct {
	RetentionDays        int           `json:"retention_days"`
	AggregationInterval  time.Duration `json:"aggregation_interval"`
	RealtimeWindowSize   time.Duration `json:"realtime_window_size"`
	EnableDetailedMetrics bool         `json:"enable_detailed_metrics"`
	EnablePredictiveAnalytics bool     `json:"enable_predictive_analytics"`
}

// RealtimeMetrics holds in-memory metrics for fast access
type RealtimeMetrics struct {
	DataSourceID    uuid.UUID                 `json:"data_source_id,omitempty"`
	ConnectorType   string                    `json:"connector_type,omitempty"`
	WindowStart     time.Time                 `json:"window_start"`
	WindowEnd       time.Time                 `json:"window_end"`
	ActiveSyncs     int                       `json:"active_syncs"`
	CompletedSyncs  int                       `json:"completed_syncs"`
	FailedSyncs     int                       `json:"failed_syncs"`
	TotalFiles      int                       `json:"total_files"`
	TotalBytes      int64                     `json:"total_bytes"`
	AverageSpeed    float64                   `json:"average_speed_mbps"`
	ErrorRate       float64                   `json:"error_rate"`
	LastUpdated     time.Time                 `json:"last_updated"`
	TrendData       *TrendAnalysis            `json:"trend_data,omitempty"`
}

// TrendAnalysis provides trend analysis for metrics
type TrendAnalysis struct {
	SyncRateTrend       TrendDirection `json:"sync_rate_trend"`
	PerformanceTrend    TrendDirection `json:"performance_trend"`
	ErrorRateTrend      TrendDirection `json:"error_rate_trend"`
	VolumeProjection    *VolumeProjection `json:"volume_projection,omitempty"`
	Anomalies           []*Anomaly      `json:"anomalies,omitempty"`
	Recommendations     []string        `json:"recommendations,omitempty"`
}

// TrendDirection indicates the direction of a trend
type TrendDirection string

const (
	TrendDirectionUp       TrendDirection = "up"
	TrendDirectionDown     TrendDirection = "down"
	TrendDirectionStable   TrendDirection = "stable"
	TrendDirectionVolatile TrendDirection = "volatile"
)

// VolumeProjection provides volume projections
type VolumeProjection struct {
	ProjectedSyncs        int       `json:"projected_syncs_next_24h"`
	ProjectedFiles        int       `json:"projected_files_next_24h"`
	ProjectedBytes        int64     `json:"projected_bytes_next_24h"`
	ConfidenceLevel       float64   `json:"confidence_level"`
	ProjectionDate        time.Time `json:"projection_date"`
}

// Anomaly represents detected anomalies in sync patterns
type Anomaly struct {
	Type        AnomalyType `json:"type"`
	Severity    string      `json:"severity"` // low, medium, high, critical
	Description string      `json:"description"`
	DetectedAt  time.Time   `json:"detected_at"`
	Value       float64     `json:"value"`
	Expected    float64     `json:"expected"`
	Deviation   float64     `json:"deviation"`
	Context     map[string]interface{} `json:"context,omitempty"`
}

// AnomalyType defines types of anomalies
type AnomalyType string

const (
	AnomalyTypePerformanceDegradation AnomalyType = "performance_degradation"
	AnomalyTypeErrorSpike            AnomalyType = "error_spike"
	AnomalyTypeVolumeAnomaly         AnomalyType = "volume_anomaly"
	AnomalyTypeLatencySpike          AnomalyType = "latency_spike"
	AnomalyTypeUnusualPattern        AnomalyType = "unusual_pattern"
)

// NewSyncMetricsCollector creates a new metrics collector
func NewSyncMetricsCollector(retentionDays int) *SyncMetricsCollector {
	config := &MetricsConfig{
		RetentionDays:           retentionDays,
		AggregationInterval:     5 * time.Minute,
		RealtimeWindowSize:      1 * time.Hour,
		EnableDetailedMetrics:   true,
		EnablePredictiveAnalytics: true,
	}

	collector := &SyncMetricsCollector{
		config:          config,
		realtimeMetrics: make(map[string]*RealtimeMetrics),
		stopChan:        make(chan struct{}),
		tracer:          otel.Tracer("sync-metrics-collector"),
	}

	// Start background aggregation
	collector.startBackgroundAggregation()

	return collector
}

// SetDB sets the database connection
func (smc *SyncMetricsCollector) SetDB(db *sql.DB) {
	smc.db = db
}

// InitializeSchema creates the necessary database tables for metrics
func (smc *SyncMetricsCollector) InitializeSchema(ctx context.Context) error {
	ctx, span := smc.tracer.Start(ctx, "metrics_collector.initialize_schema")
	defer span.End()

	if smc.db == nil {
		return fmt.Errorf("database not configured")
	}

	// Create sync_metrics_daily table for daily aggregations
	createDailyMetricsTable := `
	CREATE TABLE IF NOT EXISTS sync_metrics_daily (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		date DATE NOT NULL,
		data_source_id UUID,
		connector_type VARCHAR(100),
		total_syncs INTEGER DEFAULT 0,
		successful_syncs INTEGER DEFAULT 0,
		failed_syncs INTEGER DEFAULT 0,
		cancelled_syncs INTEGER DEFAULT 0,
		total_files INTEGER DEFAULT 0,
		total_bytes BIGINT DEFAULT 0,
		average_duration INTERVAL,
		average_speed_mbps DECIMAL(10,2),
		error_rate DECIMAL(5,2),
		peak_concurrent_syncs INTEGER DEFAULT 0,
		total_conflicts INTEGER DEFAULT 0,
		resolved_conflicts INTEGER DEFAULT 0,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		UNIQUE(date, data_source_id, connector_type),
		INDEX(date),
		INDEX(data_source_id),
		INDEX(connector_type)
	)`

	if _, err := smc.db.ExecContext(ctx, createDailyMetricsTable); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to create sync_metrics_daily table: %w", err)
	}

	// Create sync_metrics_hourly table for hourly aggregations
	createHourlyMetricsTable := `
	CREATE TABLE IF NOT EXISTS sync_metrics_hourly (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		hour_bucket TIMESTAMP WITH TIME ZONE NOT NULL,
		data_source_id UUID,
		connector_type VARCHAR(100),
		total_syncs INTEGER DEFAULT 0,
		successful_syncs INTEGER DEFAULT 0,
		failed_syncs INTEGER DEFAULT 0,
		total_files INTEGER DEFAULT 0,
		total_bytes BIGINT DEFAULT 0,
		average_duration INTERVAL,
		average_speed_mbps DECIMAL(10,2),
		error_rate DECIMAL(5,2),
		concurrent_syncs INTEGER DEFAULT 0,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		UNIQUE(hour_bucket, data_source_id, connector_type),
		INDEX(hour_bucket),
		INDEX(data_source_id),
		INDEX(connector_type)
	)`

	if _, err := smc.db.ExecContext(ctx, createHourlyMetricsTable); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to create sync_metrics_hourly table: %w", err)
	}

	// Create sync_anomalies table
	createAnomaliesTable := `
	CREATE TABLE IF NOT EXISTS sync_anomalies (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		anomaly_type VARCHAR(100) NOT NULL,
		severity VARCHAR(20) NOT NULL,
		description TEXT NOT NULL,
		detected_at TIMESTAMP WITH TIME ZONE NOT NULL,
		data_source_id UUID,
		connector_type VARCHAR(100),
		value DECIMAL(15,4),
		expected_value DECIMAL(15,4),
		deviation_percentage DECIMAL(10,2),
		context JSONB,
		resolved_at TIMESTAMP WITH TIME ZONE,
		resolved_by VARCHAR(255),
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		INDEX(detected_at),
		INDEX(anomaly_type),
		INDEX(severity),
		INDEX(data_source_id),
		INDEX(resolved_at)
	)`

	if _, err := smc.db.ExecContext(ctx, createAnomaliesTable); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to create sync_anomalies table: %w", err)
	}

	// Create sync_performance_baselines table
	createBaselinesTable := `
	CREATE TABLE IF NOT EXISTS sync_performance_baselines (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		data_source_id UUID,
		connector_type VARCHAR(100),
		metric_name VARCHAR(100) NOT NULL,
		baseline_value DECIMAL(15,4) NOT NULL,
		confidence_interval_low DECIMAL(15,4),
		confidence_interval_high DECIMAL(15,4),
		sample_size INTEGER,
		calculation_date DATE NOT NULL,
		valid_until DATE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		UNIQUE(data_source_id, connector_type, metric_name, calculation_date),
		INDEX(data_source_id),
		INDEX(connector_type),
		INDEX(metric_name),
		INDEX(calculation_date)
	)`

	if _, err := smc.db.ExecContext(ctx, createBaselinesTable); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to create sync_performance_baselines table: %w", err)
	}

	return nil
}

// RecordSyncStarted records when a sync job starts
func (smc *SyncMetricsCollector) RecordSyncStarted(job *SyncJob) {
	ctx, span := smc.tracer.Start(context.Background(), "metrics_collector.record_sync_started")
	defer span.End()

	span.SetAttributes(
		attribute.String("job_id", job.ID.String()),
		attribute.String("data_source_id", job.DataSourceID.String()),
		attribute.String("connector_type", job.ConnectorType),
	)

	smc.updateRealtimeMetrics(job.DataSourceID, job.ConnectorType, func(metrics *RealtimeMetrics) {
		metrics.ActiveSyncs++
		metrics.LastUpdated = time.Now()
	})
}

// RecordSyncCompleted records when a sync job completes successfully
func (smc *SyncMetricsCollector) RecordSyncCompleted(job *SyncJob) {
	ctx, span := smc.tracer.Start(context.Background(), "metrics_collector.record_sync_completed")
	defer span.End()

	span.SetAttributes(
		attribute.String("job_id", job.ID.String()),
		attribute.String("data_source_id", job.DataSourceID.String()),
		attribute.String("connector_type", job.ConnectorType),
	)

	smc.updateRealtimeMetrics(job.DataSourceID, job.ConnectorType, func(metrics *RealtimeMetrics) {
		metrics.ActiveSyncs--
		metrics.CompletedSyncs++
		metrics.LastUpdated = time.Now()

		if job.Status.Progress != nil {
			metrics.TotalFiles += job.Status.Progress.ProcessedFiles
		}

		if job.Status.Metrics != nil {
			metrics.TotalBytes += job.Status.Metrics.BytesDownloaded + job.Status.Metrics.BytesUploaded
			if job.Status.Metrics.AverageTransferRate > 0 {
				// Update running average
				metrics.AverageSpeed = (metrics.AverageSpeed + job.Status.Metrics.AverageTransferRate) / 2
			}
		}

		// Update error rate
		totalSyncs := metrics.CompletedSyncs + metrics.FailedSyncs
		if totalSyncs > 0 {
			metrics.ErrorRate = float64(metrics.FailedSyncs) / float64(totalSyncs) * 100
		}
	})

	// Store detailed metrics in database
	if smc.config.EnableDetailedMetrics {
		go smc.storeJobMetrics(context.Background(), job)
	}

	// Check for anomalies
	if smc.config.EnablePredictiveAnalytics {
		go smc.detectAnomalies(context.Background(), job)
	}
}

// RecordSyncFailed records when a sync job fails
func (smc *SyncMetricsCollector) RecordSyncFailed(job *SyncJob, err error) {
	ctx, span := smc.tracer.Start(context.Background(), "metrics_collector.record_sync_failed")
	defer span.End()

	span.SetAttributes(
		attribute.String("job_id", job.ID.String()),
		attribute.String("data_source_id", job.DataSourceID.String()),
		attribute.String("connector_type", job.ConnectorType),
		attribute.String("error", err.Error()),
	)

	smc.updateRealtimeMetrics(job.DataSourceID, job.ConnectorType, func(metrics *RealtimeMetrics) {
		metrics.ActiveSyncs--
		metrics.FailedSyncs++
		metrics.LastUpdated = time.Now()

		// Update error rate
		totalSyncs := metrics.CompletedSyncs + metrics.FailedSyncs
		if totalSyncs > 0 {
			metrics.ErrorRate = float64(metrics.FailedSyncs) / float64(totalSyncs) * 100
		}
	})

	// Store failure details
	if smc.config.EnableDetailedMetrics {
		go smc.storeJobMetrics(context.Background(), job)
	}

	// Check for error rate anomalies
	if smc.config.EnablePredictiveAnalytics {
		go smc.detectErrorSpike(context.Background(), job.DataSourceID, job.ConnectorType)
	}
}

// RecordSyncCancelled records when a sync job is cancelled
func (smc *SyncMetricsCollector) RecordSyncCancelled(job *SyncJob) {
	ctx, span := smc.tracer.Start(context.Background(), "metrics_collector.record_sync_cancelled")
	defer span.End()

	span.SetAttributes(
		attribute.String("job_id", job.ID.String()),
		attribute.String("data_source_id", job.DataSourceID.String()),
		attribute.String("connector_type", job.ConnectorType),
	)

	smc.updateRealtimeMetrics(job.DataSourceID, job.ConnectorType, func(metrics *RealtimeMetrics) {
		metrics.ActiveSyncs--
		metrics.LastUpdated = time.Now()
	})
}

// GetMetrics retrieves aggregated metrics based on request parameters
func (smc *SyncMetricsCollector) GetMetrics(ctx context.Context, request *SyncMetricsRequest) (*SyncMetricsResult, error) {
	ctx, span := smc.tracer.Start(ctx, "metrics_collector.get_metrics")
	defer span.End()

	result := &SyncMetricsResult{
		Summary:     &SyncMetricsSummary{},
		ByConnector: make(map[string]*SyncMetricsSummary),
	}

	// Get summary metrics
	summary, err := smc.calculateSummaryMetrics(ctx, request)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to calculate summary metrics: %w", err)
	}
	result.Summary = summary

	// Get time series data if requested
	if request.Granularity != "" {
		timeSeries, err := smc.getTimeSeriesMetrics(ctx, request)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to get time series metrics: %w", err)
		}
		result.TimeSeries = timeSeries
	}

	// Get per-connector breakdown
	byConnector, err := smc.getMetricsByConnector(ctx, request)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get metrics by connector: %w", err)
	}
	result.ByConnector = byConnector

	return result, nil
}

// GetRealtimeMetrics returns current real-time metrics
func (smc *SyncMetricsCollector) GetRealtimeMetrics(ctx context.Context, dataSourceID *uuid.UUID, connectorType string) (*RealtimeMetrics, error) {
	ctx, span := smc.tracer.Start(ctx, "metrics_collector.get_realtime_metrics")
	defer span.End()

	key := smc.buildMetricsKey(dataSourceID, connectorType)

	smc.metricsMutex.RLock()
	metrics, exists := smc.realtimeMetrics[key]
	smc.metricsMutex.RUnlock()

	if !exists {
		// Return empty metrics if none exist
		return &RealtimeMetrics{
			WindowStart: time.Now().Add(-smc.config.RealtimeWindowSize),
			WindowEnd:   time.Now(),
			LastUpdated: time.Now(),
		}, nil
	}

	// Create a copy to avoid race conditions
	metricsCopy := *metrics
	
	// Update trend analysis if predictive analytics is enabled
	if smc.config.EnablePredictiveAnalytics {
		trendData, err := smc.calculateTrendAnalysis(ctx, dataSourceID, connectorType)
		if err == nil {
			metricsCopy.TrendData = trendData
		}
	}

	return &metricsCopy, nil
}

// GetAnomalies retrieves detected anomalies
func (smc *SyncMetricsCollector) GetAnomalies(ctx context.Context, filters *AnomalyFilters) ([]*Anomaly, error) {
	ctx, span := smc.tracer.Start(ctx, "metrics_collector.get_anomalies")
	defer span.End()

	if smc.db == nil {
		return []*Anomaly{}, nil
	}

	whereClause := "WHERE 1=1"
	args := make([]interface{}, 0)
	argIndex := 1

	if filters != nil {
		if filters.DataSourceID != uuid.Nil {
			whereClause += fmt.Sprintf(" AND data_source_id = $%d", argIndex)
			args = append(args, filters.DataSourceID)
			argIndex++
		}

		if filters.ConnectorType != "" {
			whereClause += fmt.Sprintf(" AND connector_type = $%d", argIndex)
			args = append(args, filters.ConnectorType)
			argIndex++
		}

		if filters.AnomalyType != "" {
			whereClause += fmt.Sprintf(" AND anomaly_type = $%d", argIndex)
			args = append(args, filters.AnomalyType)
			argIndex++
		}

		if filters.Severity != "" {
			whereClause += fmt.Sprintf(" AND severity = $%d", argIndex)
			args = append(args, filters.Severity)
			argIndex++
		}

		if filters.Since != nil {
			whereClause += fmt.Sprintf(" AND detected_at >= $%d", argIndex)
			args = append(args, *filters.Since)
			argIndex++
		}

		if !filters.IncludeResolved {
			whereClause += " AND resolved_at IS NULL"
		}
	}

	query := fmt.Sprintf(`
		SELECT anomaly_type, severity, description, detected_at, 
		       data_source_id, connector_type, value, expected_value,
		       deviation_percentage, context
		FROM sync_anomalies
		%s
		ORDER BY detected_at DESC
		LIMIT 100
	`, whereClause)

	rows, err := smc.db.QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to query anomalies: %w", err)
	}
	defer rows.Close()

	var anomalies []*Anomaly
	for rows.Next() {
		var anomaly Anomaly
		var contextJSON []byte
		var dataSourceID *uuid.UUID
		var connectorType *string

		err := rows.Scan(
			&anomaly.Type,
			&anomaly.Severity,
			&anomaly.Description,
			&anomaly.DetectedAt,
			&dataSourceID,
			&connectorType,
			&anomaly.Value,
			&anomaly.Expected,
			&anomaly.Deviation,
			&contextJSON,
		)

		if err != nil {
			span.RecordError(err)
			continue
		}

		// Unmarshal context
		if len(contextJSON) > 0 {
			json.Unmarshal(contextJSON, &anomaly.Context)
		}

		anomalies = append(anomalies, &anomaly)
	}

	span.SetAttributes(attribute.Int("anomalies_count", len(anomalies)))
	return anomalies, nil
}

// Internal helper methods

func (smc *SyncMetricsCollector) buildMetricsKey(dataSourceID *uuid.UUID, connectorType string) string {
	if dataSourceID != nil {
		return fmt.Sprintf("%s:%s", dataSourceID.String(), connectorType)
	}
	return fmt.Sprintf("global:%s", connectorType)
}

func (smc *SyncMetricsCollector) updateRealtimeMetrics(dataSourceID uuid.UUID, connectorType string, updateFunc func(*RealtimeMetrics)) {
	key := smc.buildMetricsKey(&dataSourceID, connectorType)

	smc.metricsMutex.Lock()
	defer smc.metricsMutex.Unlock()

	metrics, exists := smc.realtimeMetrics[key]
	if !exists {
		metrics = &RealtimeMetrics{
			DataSourceID:  dataSourceID,
			ConnectorType: connectorType,
			WindowStart:   time.Now().Add(-smc.config.RealtimeWindowSize),
			WindowEnd:     time.Now(),
			LastUpdated:   time.Now(),
		}
		smc.realtimeMetrics[key] = metrics
	}

	updateFunc(metrics)
}

func (smc *SyncMetricsCollector) calculateSummaryMetrics(ctx context.Context, request *SyncMetricsRequest) (*SyncMetricsSummary, error) {
	// This would query the database for summary metrics
	// For now, return mock data
	return &SyncMetricsSummary{
		TotalSyncs:            100,
		SuccessfulSyncs:       95,
		FailedSyncs:           5,
		CancelledSyncs:        0,
		SuccessRate:           95.0,
		AverageDuration:       10 * time.Minute,
		TotalBytesTransferred: 1024 * 1024 * 1024, // 1GB
		TotalFilesProcessed:   5000,
	}, nil
}

func (smc *SyncMetricsCollector) getTimeSeriesMetrics(ctx context.Context, request *SyncMetricsRequest) ([]*SyncMetricsPoint, error) {
	// This would query hourly/daily metrics tables
	// For now, return mock data
	var points []*SyncMetricsPoint
	
	now := time.Now()
	for i := 0; i < 24; i++ {
		timestamp := now.Add(-time.Duration(i) * time.Hour)
		points = append(points, &SyncMetricsPoint{
			Timestamp: timestamp,
			Metrics: &SyncMetricsSummary{
				TotalSyncs:      10,
				SuccessfulSyncs: 9,
				FailedSyncs:     1,
				SuccessRate:     90.0,
			},
		})
	}
	
	return points, nil
}

func (smc *SyncMetricsCollector) getMetricsByConnector(ctx context.Context, request *SyncMetricsRequest) (map[string]*SyncMetricsSummary, error) {
	// This would aggregate metrics by connector type
	// For now, return mock data
	return map[string]*SyncMetricsSummary{
		"googledrive": {
			TotalSyncs:      50,
			SuccessfulSyncs: 48,
			FailedSyncs:     2,
			SuccessRate:     96.0,
		},
		"onedrive": {
			TotalSyncs:      30,
			SuccessfulSyncs: 29,
			FailedSyncs:     1,
			SuccessRate:     96.7,
		},
		"dropbox": {
			TotalSyncs:      20,
			SuccessfulSyncs: 18,
			FailedSyncs:     2,
			SuccessRate:     90.0,
		},
	}, nil
}

func (smc *SyncMetricsCollector) calculateTrendAnalysis(ctx context.Context, dataSourceID *uuid.UUID, connectorType string) (*TrendAnalysis, error) {
	// This would calculate trends based on historical data
	// For now, return mock trend data
	return &TrendAnalysis{
		SyncRateTrend:    TrendDirectionUp,
		PerformanceTrend: TrendDirectionStable,
		ErrorRateTrend:   TrendDirectionDown,
		VolumeProjection: &VolumeProjection{
			ProjectedSyncs:  48,
			ProjectedFiles:  2400,
			ProjectedBytes:  120 * 1024 * 1024,
			ConfidenceLevel: 0.85,
			ProjectionDate:  time.Now().Add(24 * time.Hour),
		},
		Recommendations: []string{
			"Consider increasing concurrent sync limit for improved throughput",
			"Error rate trending down - sync stability improving",
		},
	}, nil
}

func (smc *SyncMetricsCollector) storeJobMetrics(ctx context.Context, job *SyncJob) {
	// Store detailed job metrics in database
	// Implementation would depend on specific requirements
}

func (smc *SyncMetricsCollector) detectAnomalies(ctx context.Context, job *SyncJob) {
	// Implement anomaly detection algorithms
	// This could include statistical analysis, machine learning models, etc.
}

func (smc *SyncMetricsCollector) detectErrorSpike(ctx context.Context, dataSourceID uuid.UUID, connectorType string) {
	// Detect sudden spikes in error rates
	key := smc.buildMetricsKey(&dataSourceID, connectorType)
	
	smc.metricsMutex.RLock()
	metrics, exists := smc.realtimeMetrics[key]
	smc.metricsMutex.RUnlock()
	
	if !exists || metrics.ErrorRate < 20.0 { // Threshold for error spike
		return
	}
	
	// Record anomaly
	anomaly := &Anomaly{
		Type:        AnomalyTypeErrorSpike,
		Severity:    "medium",
		Description: fmt.Sprintf("Error rate spike detected: %.1f%%", metrics.ErrorRate),
		DetectedAt:  time.Now(),
		Value:       metrics.ErrorRate,
		Expected:    5.0, // Expected error rate
		Deviation:   metrics.ErrorRate - 5.0,
		Context: map[string]interface{}{
			"data_source_id":  dataSourceID.String(),
			"connector_type":  connectorType,
			"failed_syncs":    metrics.FailedSyncs,
			"completed_syncs": metrics.CompletedSyncs,
		},
	}
	
	go smc.storeAnomaly(context.Background(), anomaly)
}

func (smc *SyncMetricsCollector) storeAnomaly(ctx context.Context, anomaly *Anomaly) {
	if smc.db == nil {
		return
	}
	
	contextJSON, _ := json.Marshal(anomaly.Context)
	
	query := `
		INSERT INTO sync_anomalies (
			anomaly_type, severity, description, detected_at,
			value, expected_value, deviation_percentage, context
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	
	smc.db.ExecContext(ctx, query,
		anomaly.Type,
		anomaly.Severity,
		anomaly.Description,
		anomaly.DetectedAt,
		anomaly.Value,
		anomaly.Expected,
		anomaly.Deviation,
		contextJSON,
	)
}

func (smc *SyncMetricsCollector) startBackgroundAggregation() {
	smc.aggregationTicker = time.NewTicker(smc.config.AggregationInterval)
	
	go func() {
		for {
			select {
			case <-smc.stopChan:
				return
			case <-smc.aggregationTicker.C:
				smc.performAggregation()
			}
		}
	}()
}

func (smc *SyncMetricsCollector) performAggregation() {
	ctx := context.Background()
	
	// Clean up old real-time metrics
	cutoff := time.Now().Add(-smc.config.RealtimeWindowSize)
	
	smc.metricsMutex.Lock()
	for key, metrics := range smc.realtimeMetrics {
		if metrics.LastUpdated.Before(cutoff) {
			delete(smc.realtimeMetrics, key)
		}
	}
	smc.metricsMutex.Unlock()
	
	// Aggregate hourly metrics from jobs table
	if smc.db != nil {
		smc.aggregateHourlyMetrics(ctx)
		smc.aggregateDailyMetrics(ctx)
		smc.cleanupOldMetrics(ctx)
	}
}

func (smc *SyncMetricsCollector) aggregateHourlyMetrics(ctx context.Context) {
	// Implementation for aggregating hourly metrics
}

func (smc *SyncMetricsCollector) aggregateDailyMetrics(ctx context.Context) {
	// Implementation for aggregating daily metrics
}

func (smc *SyncMetricsCollector) cleanupOldMetrics(ctx context.Context) {
	cutoff := time.Now().AddDate(0, 0, -smc.config.RetentionDays)
	
	// Clean up old metrics
	tables := []string{"sync_metrics_hourly", "sync_metrics_daily", "sync_anomalies"}
	for _, table := range tables {
		query := fmt.Sprintf("DELETE FROM %s WHERE created_at < $1", table)
		smc.db.ExecContext(ctx, query, cutoff)
	}
}

// Shutdown gracefully shuts down the metrics collector
func (smc *SyncMetricsCollector) Shutdown(ctx context.Context) error {
	if smc.aggregationTicker != nil {
		smc.aggregationTicker.Stop()
	}
	
	close(smc.stopChan)
	return nil
}

// AnomalyFilters contains filters for querying anomalies
type AnomalyFilters struct {
	DataSourceID     uuid.UUID   `json:"data_source_id,omitempty"`
	ConnectorType    string      `json:"connector_type,omitempty"`
	AnomalyType      AnomalyType `json:"anomaly_type,omitempty"`
	Severity         string      `json:"severity,omitempty"`
	Since            *time.Time  `json:"since,omitempty"`
	IncludeResolved  bool        `json:"include_resolved"`
}