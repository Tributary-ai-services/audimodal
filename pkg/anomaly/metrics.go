package anomaly

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// MetricsCollector collects and exposes anomaly detection metrics
type MetricsCollector struct {
	config     *MetricsConfig
	tracer     trace.Tracer
	meter      metric.Meter
	mutex      sync.RWMutex
	
	// Metric instruments
	anomaliesDetected      metric.Int64Counter
	anomaliesByType        metric.Int64Counter
	anomaliesBySeverity    metric.Int64Counter
	anomaliesByStatus      metric.Int64Counter
	anomaliesByDetector    metric.Int64Counter
	detectionLatency       metric.Float64Histogram
	detectorLatency        metric.Float64Histogram
	falsePositiveRate      metric.Float64Gauge
	detectionAccuracy      metric.Float64Gauge
	baselineUpdates        metric.Int64Counter
	alertsGenerated        metric.Int64Counter
	alertsDelivered        metric.Int64Counter
	alertDeliveryLatency   metric.Float64Histogram
	
	// Aggregated metrics
	statistics             map[string]*DetectionStatistics
	detectorMetrics        map[string]*DetectorMetrics
	tenantMetrics          map[uuid.UUID]*TenantMetrics
	
	// Real-time metrics
	activeDetections       int64
	queueDepth            int64
	lastUpdateTime        time.Time
}

// MetricsConfig contains configuration for metrics collection
type MetricsConfig struct {
	Enabled              bool          `json:"enabled"`
	CollectionInterval   time.Duration `json:"collection_interval"`
	RetentionPeriod      time.Duration `json:"retention_period"`
	EnableHistograms     bool          `json:"enable_histograms"`
	EnableDetailedMetrics bool          `json:"enable_detailed_metrics"`
	ExportToPrometheus   bool          `json:"export_to_prometheus"`
	ExportToOpenTelemetry bool          `json:"export_to_opentelemetry"`
	CustomLabels         map[string]string `json:"custom_labels"`
	MetricPrefix         string        `json:"metric_prefix"`
}

// DetectionStatistics contains aggregated detection statistics
type DetectionStatistics struct {
	TotalDetections       int64                           `json:"total_detections"`
	DetectionsByType      map[AnomalyType]int64           `json:"detections_by_type"`
	DetectionsBySeverity  map[AnomalySeverity]int64       `json:"detections_by_severity"`
	DetectionsByStatus    map[AnomalyStatus]int64         `json:"detections_by_status"`
	DetectionsByDetector  map[string]int64                `json:"detections_by_detector"`
	AverageScore          float64                         `json:"average_score"`
	AverageConfidence     float64                         `json:"average_confidence"`
	AverageLatency        time.Duration                   `json:"average_latency"`
	FalsePositiveRate     float64                         `json:"false_positive_rate"`
	TruePositiveRate      float64                         `json:"true_positive_rate"`
	Precision             float64                         `json:"precision"`
	Recall                float64                         `json:"recall"`
	F1Score               float64                         `json:"f1_score"`
	LastUpdated           time.Time                       `json:"last_updated"`
}

// DetectorMetrics contains metrics for individual detectors
type DetectorMetrics struct {
	DetectorName          string        `json:"detector_name"`
	DetectorVersion       string        `json:"detector_version"`
	TotalInvocations      int64         `json:"total_invocations"`
	SuccessfulDetections  int64         `json:"successful_detections"`
	FailedDetections      int64         `json:"failed_detections"`
	AverageLatency        time.Duration `json:"average_latency"`
	MedianLatency         time.Duration `json:"median_latency"`
	P95Latency            time.Duration `json:"p95_latency"`
	P99Latency            time.Duration `json:"p99_latency"`
	ErrorRate             float64       `json:"error_rate"`
	AverageScore          float64       `json:"average_score"`
	AverageConfidence     float64       `json:"average_confidence"`
	BaselineUpdates       int64         `json:"baseline_updates"`
	LastInvocation        time.Time     `json:"last_invocation"`
	IsEnabled             bool          `json:"is_enabled"`
	ConfigurationHash     string        `json:"configuration_hash"`
}

// TenantMetrics contains metrics for individual tenants
type TenantMetrics struct {
	TenantID              uuid.UUID                     `json:"tenant_id"`
	TotalAnomalies        int64                         `json:"total_anomalies"`
	AnomaliesByType       map[AnomalyType]int64         `json:"anomalies_by_type"`
	AnomaliesBySeverity   map[AnomalySeverity]int64     `json:"anomalies_by_severity"`
	ResolvedAnomalies     int64                         `json:"resolved_anomalies"`
	FalsePositives        int64                         `json:"false_positives"`
	AverageResolutionTime time.Duration                 `json:"average_resolution_time"`
	AlertsGenerated       int64                         `json:"alerts_generated"`
	AlertsDelivered       int64                         `json:"alerts_delivered"`
	DataSources           int                           `json:"data_sources"`
	ActiveUsers           int                           `json:"active_users"`
	StorageUsage          int64                         `json:"storage_usage_bytes"`
	LastActivity          time.Time                     `json:"last_activity"`
}

// MetricPoint represents a single metric data point
type MetricPoint struct {
	Name      string                 `json:"name"`
	Value     float64                `json:"value"`
	Timestamp time.Time              `json:"timestamp"`
	Labels    map[string]string      `json:"labels"`
	Unit      string                 `json:"unit"`
	Type      string                 `json:"type"` // counter, gauge, histogram
}

// MetricSeries represents a time series of metric points
type MetricSeries struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Unit        string                 `json:"unit"`
	Type        string                 `json:"type"`
	Labels      map[string]string      `json:"labels"`
	Points      []MetricPoint          `json:"points"`
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(config *MetricsConfig) *MetricsCollector {
	if config == nil {
		config = &MetricsConfig{
			Enabled:               true,
			CollectionInterval:    1 * time.Minute,
			RetentionPeriod:       24 * time.Hour,
			EnableHistograms:      true,
			EnableDetailedMetrics: true,
			ExportToPrometheus:    true,
			ExportToOpenTelemetry: true,
			MetricPrefix:          "anomaly_detection",
		}
	}

	collector := &MetricsCollector{
		config:          config,
		tracer:          otel.Tracer("anomaly-metrics"),
		meter:           otel.Meter("anomaly-metrics"),
		statistics:      make(map[string]*DetectionStatistics),
		detectorMetrics: make(map[string]*DetectorMetrics),
		tenantMetrics:   make(map[uuid.UUID]*TenantMetrics),
		lastUpdateTime:  time.Now(),
	}

	if config.Enabled {
		collector.initializeMetrics()
	}

	return collector
}

// initializeMetrics initializes OpenTelemetry metric instruments
func (mc *MetricsCollector) initializeMetrics() error {
	var err error
	
	// Counters
	mc.anomaliesDetected, err = mc.meter.Int64Counter(
		mc.config.MetricPrefix+"_anomalies_detected_total",
		metric.WithDescription("Total number of anomalies detected"),
	)
	if err != nil {
		return fmt.Errorf("failed to create anomalies_detected counter: %w", err)
	}

	mc.anomaliesByType, err = mc.meter.Int64Counter(
		mc.config.MetricPrefix+"_anomalies_by_type_total",
		metric.WithDescription("Total number of anomalies by type"),
	)
	if err != nil {
		return fmt.Errorf("failed to create anomalies_by_type counter: %w", err)
	}

	mc.anomaliesBySeverity, err = mc.meter.Int64Counter(
		mc.config.MetricPrefix+"_anomalies_by_severity_total",
		metric.WithDescription("Total number of anomalies by severity"),
	)
	if err != nil {
		return fmt.Errorf("failed to create anomalies_by_severity counter: %w", err)
	}

	mc.anomaliesByDetector, err = mc.meter.Int64Counter(
		mc.config.MetricPrefix+"_anomalies_by_detector_total",
		metric.WithDescription("Total number of anomalies by detector"),
	)
	if err != nil {
		return fmt.Errorf("failed to create anomalies_by_detector counter: %w", err)
	}

	mc.alertsGenerated, err = mc.meter.Int64Counter(
		mc.config.MetricPrefix+"_alerts_generated_total",
		metric.WithDescription("Total number of alerts generated"),
	)
	if err != nil {
		return fmt.Errorf("failed to create alerts_generated counter: %w", err)
	}

	mc.alertsDelivered, err = mc.meter.Int64Counter(
		mc.config.MetricPrefix+"_alerts_delivered_total",
		metric.WithDescription("Total number of alerts delivered"),
	)
	if err != nil {
		return fmt.Errorf("failed to create alerts_delivered counter: %w", err)
	}

	mc.baselineUpdates, err = mc.meter.Int64Counter(
		mc.config.MetricPrefix+"_baseline_updates_total",
		metric.WithDescription("Total number of baseline updates"),
	)
	if err != nil {
		return fmt.Errorf("failed to create baseline_updates counter: %w", err)
	}

	// Histograms
	if mc.config.EnableHistograms {
		mc.detectionLatency, err = mc.meter.Float64Histogram(
			mc.config.MetricPrefix+"_detection_latency_seconds",
			metric.WithDescription("Detection latency in seconds"),
			metric.WithUnit("s"),
		)
		if err != nil {
			return fmt.Errorf("failed to create detection_latency histogram: %w", err)
		}

		mc.detectorLatency, err = mc.meter.Float64Histogram(
			mc.config.MetricPrefix+"_detector_latency_seconds",
			metric.WithDescription("Individual detector latency in seconds"),
			metric.WithUnit("s"),
		)
		if err != nil {
			return fmt.Errorf("failed to create detector_latency histogram: %w", err)
		}

		mc.alertDeliveryLatency, err = mc.meter.Float64Histogram(
			mc.config.MetricPrefix+"_alert_delivery_latency_seconds",
			metric.WithDescription("Alert delivery latency in seconds"),
			metric.WithUnit("s"),
		)
		if err != nil {
			return fmt.Errorf("failed to create alert_delivery_latency histogram: %w", err)
		}
	}

	// Gauges
	mc.falsePositiveRate, err = mc.meter.Float64Gauge(
		mc.config.MetricPrefix+"_false_positive_rate",
		metric.WithDescription("False positive rate for anomaly detection"),
	)
	if err != nil {
		return fmt.Errorf("failed to create false_positive_rate gauge: %w", err)
	}

	mc.detectionAccuracy, err = mc.meter.Float64Gauge(
		mc.config.MetricPrefix+"_detection_accuracy",
		metric.WithDescription("Overall detection accuracy"),
	)
	if err != nil {
		return fmt.Errorf("failed to create detection_accuracy gauge: %w", err)
	}

	return nil
}

// RecordAnomalyDetected records metrics when an anomaly is detected
func (mc *MetricsCollector) RecordAnomalyDetected(ctx context.Context, anomaly *Anomaly, detectionLatency time.Duration) {
	if !mc.config.Enabled {
		return
	}

	ctx, span := mc.tracer.Start(ctx, "metrics_collector.record_anomaly_detected")
	defer span.End()

	// Create common labels
	labels := mc.createLabels(map[string]string{
		"tenant_id":     anomaly.TenantID.String(),
		"anomaly_type":  string(anomaly.Type),
		"severity":      string(anomaly.Severity),
		"detector_name": anomaly.DetectorName,
	})

	// Record counters
	mc.anomaliesDetected.Add(ctx, 1, metric.WithAttributes(labels...))
	mc.anomaliesByType.Add(ctx, 1, metric.WithAttributes(
		attribute.String("type", string(anomaly.Type)),
	))
	mc.anomaliesBySeverity.Add(ctx, 1, metric.WithAttributes(
		attribute.String("severity", string(anomaly.Severity)),
	))
	mc.anomaliesByDetector.Add(ctx, 1, metric.WithAttributes(
		attribute.String("detector", anomaly.DetectorName),
	))

	// Record latency
	if mc.config.EnableHistograms && mc.detectionLatency != nil {
		mc.detectionLatency.Record(ctx, detectionLatency.Seconds(), metric.WithAttributes(labels...))
	}

	// Update aggregated statistics
	mc.updateDetectionStatistics(anomaly, detectionLatency)
	mc.updateDetectorMetrics(anomaly, detectionLatency, true)
	mc.updateTenantMetrics(anomaly)

	span.SetAttributes(
		attribute.String("anomaly_id", anomaly.ID.String()),
		attribute.String("anomaly_type", string(anomaly.Type)),
		attribute.Float64("detection_latency_ms", float64(detectionLatency.Milliseconds())),
	)
}

// RecordDetectorInvocation records metrics for detector invocations
func (mc *MetricsCollector) RecordDetectorInvocation(ctx context.Context, detectorName string, latency time.Duration, success bool, errorMsg string) {
	if !mc.config.Enabled {
		return
	}

	labels := mc.createLabels(map[string]string{
		"detector_name": detectorName,
		"success":       fmt.Sprintf("%t", success),
	})

	// Record detector latency
	if mc.config.EnableHistograms && mc.detectorLatency != nil {
		mc.detectorLatency.Record(ctx, latency.Seconds(), metric.WithAttributes(labels...))
	}

	// Update detector metrics
	mc.updateDetectorInvocationMetrics(detectorName, latency, success, errorMsg)
}

// RecordAlertGenerated records metrics when an alert is generated
func (mc *MetricsCollector) RecordAlertGenerated(ctx context.Context, alert *Alert) {
	if !mc.config.Enabled {
		return
	}

	labels := mc.createLabels(map[string]string{
		"tenant_id": alert.TenantID.String(),
		"severity":  string(alert.Severity),
		"rule_id":   alert.RuleID.String(),
	})

	mc.alertsGenerated.Add(ctx, 1, metric.WithAttributes(labels...))

	// Update tenant metrics
	mc.updateTenantAlertMetrics(alert.TenantID, true, false)
}

// RecordAlertDelivered records metrics when an alert is delivered
func (mc *MetricsCollector) RecordAlertDelivered(ctx context.Context, alert *Alert, channel string, deliveryLatency time.Duration) {
	if !mc.config.Enabled {
		return
	}

	labels := mc.createLabels(map[string]string{
		"tenant_id": alert.TenantID.String(),
		"severity":  string(alert.Severity),
		"channel":   channel,
	})

	mc.alertsDelivered.Add(ctx, 1, metric.WithAttributes(labels...))

	// Record delivery latency
	if mc.config.EnableHistograms && mc.alertDeliveryLatency != nil {
		mc.alertDeliveryLatency.Record(ctx, deliveryLatency.Seconds(), metric.WithAttributes(labels...))
	}

	// Update tenant metrics
	mc.updateTenantAlertMetrics(alert.TenantID, false, true)
}

// RecordBaselineUpdate records metrics when a baseline is updated
func (mc *MetricsCollector) RecordBaselineUpdate(ctx context.Context, detectorName string, tenantID uuid.UUID) {
	if !mc.config.Enabled {
		return
	}

	labels := mc.createLabels(map[string]string{
		"detector_name": detectorName,
		"tenant_id":     tenantID.String(),
	})

	mc.baselineUpdates.Add(ctx, 1, metric.WithAttributes(labels...))

	// Update detector metrics
	mc.updateDetectorBaselineMetrics(detectorName)
}

// GetDetectionStatistics returns aggregated detection statistics
func (mc *MetricsCollector) GetDetectionStatistics(ctx context.Context) *DetectionStatistics {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	// Return overall statistics
	if stats, exists := mc.statistics["overall"]; exists {
		return stats
	}

	return &DetectionStatistics{
		LastUpdated: time.Now(),
	}
}

// GetDetectorMetrics returns metrics for a specific detector
func (mc *MetricsCollector) GetDetectorMetrics(ctx context.Context, detectorName string) *DetectorMetrics {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	if metrics, exists := mc.detectorMetrics[detectorName]; exists {
		return metrics
	}

	return nil
}

// GetTenantMetrics returns metrics for a specific tenant
func (mc *MetricsCollector) GetTenantMetrics(ctx context.Context, tenantID uuid.UUID) *TenantMetrics {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	if metrics, exists := mc.tenantMetrics[tenantID]; exists {
		return metrics
	}

	return nil
}

// GetAllMetrics returns all collected metrics
func (mc *MetricsCollector) GetAllMetrics(ctx context.Context) map[string]interface{} {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	return map[string]interface{}{
		"detection_statistics": mc.statistics,
		"detector_metrics":     mc.detectorMetrics,
		"tenant_metrics":       mc.tenantMetrics,
		"collection_info": map[string]interface{}{
			"last_updated":     mc.lastUpdateTime,
			"active_detections": mc.activeDetections,
			"queue_depth":      mc.queueDepth,
		},
	}
}

// ExportMetrics exports metrics in Prometheus format
func (mc *MetricsCollector) ExportMetrics(ctx context.Context) ([]MetricSeries, error) {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	var series []MetricSeries

	// Export detection statistics
	for name, stats := range mc.statistics {
		series = append(series, MetricSeries{
			Name:        fmt.Sprintf("%s_total_detections", mc.config.MetricPrefix),
			Description: "Total number of anomaly detections",
			Type:        "counter",
			Unit:        "count",
			Labels:      map[string]string{"category": name},
			Points: []MetricPoint{
				{
					Name:      "total_detections",
					Value:     float64(stats.TotalDetections),
					Timestamp: stats.LastUpdated,
					Labels:    map[string]string{"category": name},
					Unit:      "count",
					Type:      "counter",
				},
			},
		})

		series = append(series, MetricSeries{
			Name:        fmt.Sprintf("%s_average_score", mc.config.MetricPrefix),
			Description: "Average anomaly score",
			Type:        "gauge",
			Unit:        "ratio",
			Labels:      map[string]string{"category": name},
			Points: []MetricPoint{
				{
					Name:      "average_score",
					Value:     stats.AverageScore,
					Timestamp: stats.LastUpdated,
					Labels:    map[string]string{"category": name},
					Unit:      "ratio",
					Type:      "gauge",
				},
			},
		})
	}

	// Export detector metrics
	for name, metrics := range mc.detectorMetrics {
		series = append(series, MetricSeries{
			Name:        fmt.Sprintf("%s_detector_invocations", mc.config.MetricPrefix),
			Description: "Total detector invocations",
			Type:        "counter",
			Unit:        "count",
			Labels:      map[string]string{"detector": name},
			Points: []MetricPoint{
				{
					Name:      "invocations",
					Value:     float64(metrics.TotalInvocations),
					Timestamp: metrics.LastInvocation,
					Labels:    map[string]string{"detector": name},
					Unit:      "count",
					Type:      "counter",
				},
			},
		})

		series = append(series, MetricSeries{
			Name:        fmt.Sprintf("%s_detector_latency", mc.config.MetricPrefix),
			Description: "Detector latency",
			Type:        "histogram",
			Unit:        "seconds",
			Labels:      map[string]string{"detector": name},
			Points: []MetricPoint{
				{
					Name:      "average_latency",
					Value:     metrics.AverageLatency.Seconds(),
					Timestamp: metrics.LastInvocation,
					Labels:    map[string]string{"detector": name, "percentile": "average"},
					Unit:      "seconds",
					Type:      "histogram",
				},
			},
		})
	}

	return series, nil
}

// Internal methods

func (mc *MetricsCollector) createLabels(additionalLabels map[string]string) []attribute.KeyValue {
	var labels []attribute.KeyValue

	// Add custom labels from config
	for key, value := range mc.config.CustomLabels {
		labels = append(labels, attribute.String(key, value))
	}

	// Add additional labels
	for key, value := range additionalLabels {
		labels = append(labels, attribute.String(key, value))
	}

	return labels
}

func (mc *MetricsCollector) updateDetectionStatistics(anomaly *Anomaly, latency time.Duration) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	// Update overall statistics
	stats := mc.getOrCreateStatistics("overall")
	stats.TotalDetections++
	stats.DetectionsByType[anomaly.Type]++
	stats.DetectionsBySeverity[anomaly.Severity]++
	stats.DetectionsByStatus[anomaly.Status]++
	stats.DetectionsByDetector[anomaly.DetectorName]++

	// Update averages (simple moving average)
	count := float64(stats.TotalDetections)
	stats.AverageScore = (stats.AverageScore*(count-1) + anomaly.Score) / count
	stats.AverageConfidence = (stats.AverageConfidence*(count-1) + anomaly.Confidence) / count
	stats.AverageLatency = time.Duration(
		(float64(stats.AverageLatency)*(count-1) + float64(latency)) / count,
	)

	stats.LastUpdated = time.Now()
}

func (mc *MetricsCollector) updateDetectorMetrics(anomaly *Anomaly, latency time.Duration, success bool) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	metrics := mc.getOrCreateDetectorMetrics(anomaly.DetectorName)
	metrics.TotalInvocations++

	if success {
		metrics.SuccessfulDetections++
	} else {
		metrics.FailedDetections++
	}

	// Update latency metrics (simple moving average)
	count := float64(metrics.TotalInvocations)
	metrics.AverageLatency = time.Duration(
		(float64(metrics.AverageLatency)*(count-1) + float64(latency)) / count,
	)

	if success {
		successCount := float64(metrics.SuccessfulDetections)
		metrics.AverageScore = (metrics.AverageScore*(successCount-1) + anomaly.Score) / successCount
		metrics.AverageConfidence = (metrics.AverageConfidence*(successCount-1) + anomaly.Confidence) / successCount
	}

	metrics.ErrorRate = float64(metrics.FailedDetections) / float64(metrics.TotalInvocations)
	metrics.LastInvocation = time.Now()
}

func (mc *MetricsCollector) updateDetectorInvocationMetrics(detectorName string, latency time.Duration, success bool, errorMsg string) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	metrics := mc.getOrCreateDetectorMetrics(detectorName)
	metrics.TotalInvocations++

	if success {
		metrics.SuccessfulDetections++
	} else {
		metrics.FailedDetections++
	}

	// Update latency
	count := float64(metrics.TotalInvocations)
	metrics.AverageLatency = time.Duration(
		(float64(metrics.AverageLatency)*(count-1) + float64(latency)) / count,
	)

	metrics.ErrorRate = float64(metrics.FailedDetections) / float64(metrics.TotalInvocations)
	metrics.LastInvocation = time.Now()
}

func (mc *MetricsCollector) updateDetectorBaselineMetrics(detectorName string) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	metrics := mc.getOrCreateDetectorMetrics(detectorName)
	metrics.BaselineUpdates++
}

func (mc *MetricsCollector) updateTenantMetrics(anomaly *Anomaly) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	metrics := mc.getOrCreateTenantMetrics(anomaly.TenantID)
	metrics.TotalAnomalies++
	metrics.AnomaliesByType[anomaly.Type]++
	metrics.AnomaliesBySeverity[anomaly.Severity]++
	metrics.LastActivity = time.Now()
}

func (mc *MetricsCollector) updateTenantAlertMetrics(tenantID uuid.UUID, generated bool, delivered bool) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	metrics := mc.getOrCreateTenantMetrics(tenantID)
	
	if generated {
		metrics.AlertsGenerated++
	}
	if delivered {
		metrics.AlertsDelivered++
	}
	
	metrics.LastActivity = time.Now()
}

func (mc *MetricsCollector) getOrCreateStatistics(category string) *DetectionStatistics {
	if stats, exists := mc.statistics[category]; exists {
		return stats
	}

	stats := &DetectionStatistics{
		DetectionsByType:     make(map[AnomalyType]int64),
		DetectionsBySeverity: make(map[AnomalySeverity]int64),
		DetectionsByStatus:   make(map[AnomalyStatus]int64),
		DetectionsByDetector: make(map[string]int64),
		LastUpdated:          time.Now(),
	}
	mc.statistics[category] = stats
	return stats
}

func (mc *MetricsCollector) getOrCreateDetectorMetrics(detectorName string) *DetectorMetrics {
	if metrics, exists := mc.detectorMetrics[detectorName]; exists {
		return metrics
	}

	metrics := &DetectorMetrics{
		DetectorName:    detectorName,
		LastInvocation:  time.Now(),
		IsEnabled:       true,
	}
	mc.detectorMetrics[detectorName] = metrics
	return metrics
}

func (mc *MetricsCollector) getOrCreateTenantMetrics(tenantID uuid.UUID) *TenantMetrics {
	if metrics, exists := mc.tenantMetrics[tenantID]; exists {
		return metrics
	}

	metrics := &TenantMetrics{
		TenantID:            tenantID,
		AnomaliesByType:     make(map[AnomalyType]int64),
		AnomaliesBySeverity: make(map[AnomalySeverity]int64),
		LastActivity:        time.Now(),
	}
	mc.tenantMetrics[tenantID] = metrics
	return metrics
}