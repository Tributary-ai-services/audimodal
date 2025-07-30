package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/metrics"
)

// DashboardService provides monitoring dashboards and real-time metrics
type DashboardService struct {
	registry        *metrics.MetricsRegistry
	alertingService *AlertingService
	tracer          trace.Tracer
	config          *DashboardConfig
	
	// Real-time data storage
	timeSeriesData  map[string]*TimeSeries
	dataLock        sync.RWMutex
	
	// Dashboard definitions
	dashboards      map[string]*Dashboard
	dashboardsLock  sync.RWMutex
}

// DashboardConfig contains configuration for the dashboard service
type DashboardConfig struct {
	DataRetentionPeriod time.Duration `yaml:"data_retention_period"`
	CollectionInterval  time.Duration `yaml:"collection_interval"`
	MaxDataPoints       int           `yaml:"max_data_points"`
	EnableRealTime      bool          `yaml:"enable_real_time"`
	WebSocketPort       int           `yaml:"websocket_port"`
}

// TimeSeries represents time-series data for a metric
type TimeSeries struct {
	MetricName  string      `json:"metric_name"`
	DataPoints  []DataPoint `json:"data_points"`
	LastUpdated time.Time   `json:"last_updated"`
	MaxPoints   int         `json:"-"`
}

// DataPoint represents a single data point in time series
type DataPoint struct {
	Timestamp time.Time   `json:"timestamp"`
	Value     float64     `json:"value"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// Dashboard represents a monitoring dashboard
type Dashboard struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Tags        []string    `json:"tags"`
	Panels      []Panel     `json:"panels"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	IsDefault   bool        `json:"is_default"`
}

// Panel represents a single panel in a dashboard
type Panel struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Type        PanelType         `json:"type"`
	Query       string            `json:"query"`
	Visualization VisualizationType `json:"visualization"`
	Position    PanelPosition     `json:"position"`
	Options     map[string]interface{} `json:"options,omitempty"`
	Thresholds  []Threshold       `json:"thresholds,omitempty"`
}

// PanelType defines the type of panel
type PanelType string

const (
	PanelTypeMetric    PanelType = "metric"
	PanelTypeAlert     PanelType = "alert"
	PanelTypeLog       PanelType = "log"
	PanelTypeStatus    PanelType = "status"
	PanelTypeChart     PanelType = "chart"
)

// VisualizationType defines how data is visualized
type VisualizationType string

const (
	VisualizationLine      VisualizationType = "line"
	VisualizationBar       VisualizationType = "bar"
	VisualizationGauge     VisualizationType = "gauge"
	VisualizationCounter   VisualizationType = "counter"
	VisualizationTable     VisualizationType = "table"
	VisualizationPie       VisualizationType = "pie"
	VisualizationHeatmap   VisualizationType = "heatmap"
)

// PanelPosition defines the position and size of a panel
type PanelPosition struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Threshold defines alert thresholds for panels
type Threshold struct {
	Value     float64       `json:"value"`
	Color     string        `json:"color"`
	Condition string        `json:"condition"` // gt, lt, eq
	Severity  AlertSeverity `json:"severity,omitempty"`
}

// SystemOverview provides a high-level system overview
type SystemOverview struct {
	Status          string                 `json:"status"`
	Uptime          time.Duration          `json:"uptime"`
	Version         string                 `json:"version"`
	ActiveAlerts    int                    `json:"active_alerts"`
	CriticalAlerts  int                    `json:"critical_alerts"`
	ProcessingStats ProcessingOverview     `json:"processing_stats"`
	SystemMetrics   SystemMetricsOverview  `json:"system_metrics"`
	ServiceHealth   map[string]HealthStatus `json:"service_health"`
	LastUpdated     time.Time              `json:"last_updated"`
}

// ProcessingOverview provides processing statistics overview
type ProcessingOverview struct {
	FilesProcessedToday   int64   `json:"files_processed_today"`
	FilesPerHour          float64 `json:"files_per_hour"`
	AverageProcessingTime float64 `json:"avg_processing_time_ms"`
	QueueDepth            int     `json:"queue_depth"`
	ErrorRate             float64 `json:"error_rate"`
	EmbeddingsCreated     int64   `json:"embeddings_created"`
}

// SystemMetricsOverview provides system metrics overview
type SystemMetricsOverview struct {
	CPUUsage     float64 `json:"cpu_usage"`
	MemoryUsage  float64 `json:"memory_usage"`
	DiskUsage    float64 `json:"disk_usage"`
	NetworkIO    int64   `json:"network_io_bytes"`
	Goroutines   int     `json:"goroutines"`
	GCPauses     int64   `json:"gc_pauses_ms"`
}

// HealthStatus represents the health status of a service
type HealthStatus struct {
	Status      string    `json:"status"` // healthy, degraded, unhealthy
	LastCheck   time.Time `json:"last_check"`
	ResponseTime float64   `json:"response_time_ms"`
	Message     string    `json:"message,omitempty"`
}

// DefaultDashboardConfig returns default dashboard configuration
func DefaultDashboardConfig() *DashboardConfig {
	return &DashboardConfig{
		DataRetentionPeriod: 24 * time.Hour,
		CollectionInterval:  15 * time.Second,
		MaxDataPoints:       1000,
		EnableRealTime:      true,
		WebSocketPort:       8081,
	}
}

// NewDashboardService creates a new dashboard service
func NewDashboardService(registry *metrics.MetricsRegistry, alertingService *AlertingService, config *DashboardConfig) *DashboardService {
	if config == nil {
		config = DefaultDashboardConfig()
	}

	ds := &DashboardService{
		registry:        registry,
		alertingService: alertingService,
		tracer:          otel.Tracer("dashboard-service"),
		config:          config,
		timeSeriesData:  make(map[string]*TimeSeries),
		dashboards:      make(map[string]*Dashboard),
	}

	// Create default dashboards
	ds.createDefaultDashboards()

	return ds
}

// Start starts the dashboard service
func (ds *DashboardService) Start(ctx context.Context) error {
	// Start data collection
	if ds.config.EnableRealTime {
		go ds.dataCollectionLoop(ctx)
	}

	return nil
}

// createDefaultDashboards creates default monitoring dashboards
func (ds *DashboardService) createDefaultDashboards() {
	// System Overview Dashboard
	systemDashboard := &Dashboard{
		ID:          "system_overview",
		Name:        "System Overview",
		Description: "High-level system metrics and health",
		Tags:        []string{"system", "overview"},
		IsDefault:   true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Panels: []Panel{
			{
				ID:            "cpu_usage",
				Title:         "CPU Usage",
				Type:          PanelTypeMetric,
				Query:         "system_cpu_usage",
				Visualization: VisualizationGauge,
				Position:      PanelPosition{X: 0, Y: 0, Width: 6, Height: 4},
				Thresholds: []Threshold{
					{Value: 70, Color: "yellow", Condition: "gt", Severity: SeverityWarning},
					{Value: 90, Color: "red", Condition: "gt", Severity: SeverityCritical},
				},
			},
			{
				ID:            "memory_usage",
				Title:         "Memory Usage",
				Type:          PanelTypeMetric,
				Query:         "go_memstats_heap_alloc_bytes",
				Visualization: VisualizationGauge,
				Position:      PanelPosition{X: 6, Y: 0, Width: 6, Height: 4},
				Thresholds: []Threshold{
					{Value: 0.8, Color: "yellow", Condition: "gt", Severity: SeverityWarning},
					{Value: 0.9, Color: "red", Condition: "gt", Severity: SeverityCritical},
				},
			},
			{
				ID:            "active_alerts",
				Title:         "Active Alerts",
				Type:          PanelTypeAlert,
				Query:         "alerts_active",
				Visualization: VisualizationCounter,
				Position:      PanelPosition{X: 12, Y: 0, Width: 6, Height: 4},
			},
			{
				ID:            "processing_throughput",
				Title:         "Processing Throughput",
				Type:          PanelTypeChart,
				Query:         "files_processed_total",
				Visualization: VisualizationLine,
				Position:      PanelPosition{X: 0, Y: 4, Width: 12, Height: 6},
			},
		},
	}

	// Application Performance Dashboard
	appDashboard := &Dashboard{
		ID:          "application_performance",
		Name:        "Application Performance",
		Description: "Application-specific metrics and performance",
		Tags:        []string{"application", "performance"},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Panels: []Panel{
			{
				ID:            "processing_queue",
				Title:         "Processing Queue Depth",
				Type:          PanelTypeChart,
				Query:         "processing_queue_size",
				Visualization: VisualizationLine,
				Position:      PanelPosition{X: 0, Y: 0, Width: 12, Height: 6},
			},
			{
				ID:            "error_rate",
				Title:         "Error Rate",
				Type:          PanelTypeChart,
				Query:         "processing_errors_total",
				Visualization: VisualizationLine,
				Position:      PanelPosition{X: 0, Y: 6, Width: 6, Height: 6},
				Thresholds: []Threshold{
					{Value: 0.05, Color: "yellow", Condition: "gt", Severity: SeverityWarning},
					{Value: 0.1, Color: "red", Condition: "gt", Severity: SeverityCritical},
				},
			},
			{
				ID:            "response_times",
				Title:         "API Response Times",
				Type:          PanelTypeChart,
				Query:         "api_request_duration",
				Visualization: VisualizationLine,
				Position:      PanelPosition{X: 6, Y: 6, Width: 6, Height: 6},
			},
		},
	}

	// Infrastructure Dashboard
	infraDashboard := &Dashboard{
		ID:          "infrastructure",
		Name:        "Infrastructure",
		Description: "Infrastructure components and external dependencies",
		Tags:        []string{"infrastructure", "dependencies"},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Panels: []Panel{
			{
				ID:            "kafka_health",
				Title:         "Kafka Health",
				Type:          PanelTypeStatus,
				Query:         "kafka_broker_status",
				Visualization: VisualizationTable,
				Position:      PanelPosition{X: 0, Y: 0, Width: 6, Height: 6},
			},
			{
				ID:            "database_connections",
				Title:         "Database Connections",
				Type:          PanelTypeMetric,
				Query:         "database_connections_active",
				Visualization: VisualizationGauge,
				Position:      PanelPosition{X: 6, Y: 0, Width: 6, Height: 6},
			},
			{
				ID:            "external_api_status",
				Title:         "External API Status",
				Type:          PanelTypeStatus,
				Query:         "external_api_response_time",
				Visualization: VisualizationTable,
				Position:      PanelPosition{X: 0, Y: 6, Width: 12, Height: 6},
			},
		},
	}

	ds.dashboardsLock.Lock()
	ds.dashboards["system_overview"] = systemDashboard
	ds.dashboards["application_performance"] = appDashboard
	ds.dashboards["infrastructure"] = infraDashboard
	ds.dashboardsLock.Unlock()
}

// dataCollectionLoop continuously collects metrics for time series
func (ds *DashboardService) dataCollectionLoop(ctx context.Context) {
	ticker := time.NewTicker(ds.config.CollectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ds.collectTimeSeriesData()
		}
	}
}

// collectTimeSeriesData collects current metric values for time series
func (ds *DashboardService) collectTimeSeriesData() {
	allMetrics := ds.registry.GetMetrics()
	now := time.Now()

	ds.dataLock.Lock()
	defer ds.dataLock.Unlock()

	for name, metric := range allMetrics {
		// Extract numeric value
		numericValue, err := ds.extractNumericValue(metric.GetValue())
		if err != nil {
			continue
		}

		// Get or create time series
		ts, exists := ds.timeSeriesData[name]
		if !exists {
			ts = &TimeSeries{
				MetricName: name,
				DataPoints: make([]DataPoint, 0),
				MaxPoints:  ds.config.MaxDataPoints,
			}
			ds.timeSeriesData[name] = ts
		}

		// Add new data point
		dataPoint := DataPoint{
			Timestamp: now,
			Value:     numericValue,
			Labels:    metric.GetLabels(),
		}

		ts.DataPoints = append(ts.DataPoints, dataPoint)
		ts.LastUpdated = now

		// Remove old data points if necessary
		if len(ts.DataPoints) > ts.MaxPoints {
			ts.DataPoints = ts.DataPoints[len(ts.DataPoints)-ts.MaxPoints:]
		}
	}

	// Remove old time series data
	cutoff := now.Add(-ds.config.DataRetentionPeriod)
	for name, ts := range ds.timeSeriesData {
		if ts.LastUpdated.Before(cutoff) {
			delete(ds.timeSeriesData, name)
		}
	}
}

// extractNumericValue extracts a numeric value from various metric types
func (ds *DashboardService) extractNumericValue(value interface{}) (float64, error) {
	switch v := value.(type) {
	case int64:
		return float64(v), nil
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case map[string]interface{}:
		// For complex metrics, try to extract a meaningful value
		if avg, ok := v["avg"].(float64); ok {
			return avg, nil
		}
		if count, ok := v["total_count"].(int64); ok {
			return float64(count), nil
		}
		if sum, ok := v["sum"].(int64); ok {
			return float64(sum), nil
		}
	}
	return 0, fmt.Errorf("cannot extract numeric value from %T", value)
}

// GetSystemOverview returns a high-level system overview
func (ds *DashboardService) GetSystemOverview() *SystemOverview {
	ctx, span := ds.tracer.Start(context.Background(), "get_system_overview")
	defer span.End()

	// Get active alerts
	activeAlerts := ds.alertingService.GetActiveAlerts()
	criticalCount := 0
	for _, alert := range activeAlerts {
		if alert.Severity == SeverityCritical {
			criticalCount++
		}
	}

	// Get system metrics
	allMetrics := ds.registry.GetMetrics()
	
	// Extract specific metrics
	var memoryUsage, cpuUsage float64
	var goroutines int
	
	if metric, exists := allMetrics["go_memstats_heap_alloc_bytes"]; exists {
		if val, err := ds.extractNumericValue(metric.GetValue()); err == nil {
			memoryUsage = val / (1024 * 1024 * 1024) // Convert to GB
		}
	}
	
	if metric, exists := allMetrics["go_goroutines"]; exists {
		if val, err := ds.extractNumericValue(metric.GetValue()); err == nil {
			goroutines = int(val)
		}
	}

	overview := &SystemOverview{
		Status:         ds.getOverallSystemStatus(activeAlerts),
		Uptime:         time.Since(time.Now().Add(-24 * time.Hour)), // Placeholder
		Version:        "1.0.0",
		ActiveAlerts:   len(activeAlerts),
		CriticalAlerts: criticalCount,
		ProcessingStats: ProcessingOverview{
			FilesProcessedToday:   1000, // Placeholder
			FilesPerHour:          50.0,
			AverageProcessingTime: 2500.0,
			QueueDepth:            10,
			ErrorRate:             0.02,
			EmbeddingsCreated:     5000,
		},
		SystemMetrics: SystemMetricsOverview{
			CPUUsage:    cpuUsage,
			MemoryUsage: memoryUsage,
			DiskUsage:   65.5,
			NetworkIO:   1024 * 1024 * 100, // 100MB
			Goroutines:  goroutines,
			GCPauses:    45,
		},
		ServiceHealth: map[string]HealthStatus{
			"kafka":     {Status: "healthy", LastCheck: time.Now(), ResponseTime: 5.2},
			"database":  {Status: "healthy", LastCheck: time.Now(), ResponseTime: 12.1},
			"openai_api": {Status: "healthy", LastCheck: time.Now(), ResponseTime: 850.3},
			"deeplake":  {Status: "healthy", LastCheck: time.Now(), ResponseTime: 120.5},
		},
		LastUpdated: time.Now(),
	}

	span.SetAttributes(
		attribute.String("system.status", overview.Status),
		attribute.Int("alerts.active", overview.ActiveAlerts),
		attribute.Int("alerts.critical", overview.CriticalAlerts),
	)

	return overview
}

// getOverallSystemStatus determines the overall system status based on alerts
func (ds *DashboardService) getOverallSystemStatus(alerts []*Alert) string {
	if len(alerts) == 0 {
		return "healthy"
	}

	for _, alert := range alerts {
		if alert.Severity == SeverityCritical {
			return "critical"
		}
	}

	return "warning"
}

// GetDashboard returns a dashboard by ID
func (ds *DashboardService) GetDashboard(dashboardID string) (*Dashboard, error) {
	ds.dashboardsLock.RLock()
	defer ds.dashboardsLock.RUnlock()

	dashboard, exists := ds.dashboards[dashboardID]
	if !exists {
		return nil, fmt.Errorf("dashboard not found: %s", dashboardID)
	}

	return dashboard, nil
}

// GetDashboards returns all available dashboards
func (ds *DashboardService) GetDashboards() []*Dashboard {
	ds.dashboardsLock.RLock()
	defer ds.dashboardsLock.RUnlock()

	dashboards := make([]*Dashboard, 0, len(ds.dashboards))
	for _, dashboard := range ds.dashboards {
		dashboards = append(dashboards, dashboard)
	}

	// Sort by name
	sort.Slice(dashboards, func(i, j int) bool {
		return dashboards[i].Name < dashboards[j].Name
	})

	return dashboards
}

// GetTimeSeriesData returns time series data for a metric
func (ds *DashboardService) GetTimeSeriesData(metricName string, timeRange time.Duration) (*TimeSeries, error) {
	ds.dataLock.RLock()
	defer ds.dataLock.RUnlock()

	ts, exists := ds.timeSeriesData[metricName]
	if !exists {
		return nil, fmt.Errorf("time series not found: %s", metricName)
	}

	// Filter data points by time range
	if timeRange > 0 {
		cutoff := time.Now().Add(-timeRange)
		filteredPoints := make([]DataPoint, 0)
		
		for _, point := range ts.DataPoints {
			if point.Timestamp.After(cutoff) {
				filteredPoints = append(filteredPoints, point)
			}
		}

		return &TimeSeries{
			MetricName:  ts.MetricName,
			DataPoints:  filteredPoints,
			LastUpdated: ts.LastUpdated,
		}, nil
	}

	return ts, nil
}

// GetPanelData returns data for a specific dashboard panel
func (ds *DashboardService) GetPanelData(dashboardID, panelID string) (map[string]interface{}, error) {
	dashboard, err := ds.GetDashboard(dashboardID)
	if err != nil {
		return nil, err
	}

	// Find the panel
	var panel *Panel
	for _, p := range dashboard.Panels {
		if p.ID == panelID {
			panel = &p
			break
		}
	}

	if panel == nil {
		return nil, fmt.Errorf("panel not found: %s", panelID)
	}

	// Get data based on panel type
	switch panel.Type {
	case PanelTypeMetric:
		return ds.getMetricPanelData(panel)
	case PanelTypeChart:
		return ds.getChartPanelData(panel)
	case PanelTypeAlert:
		return ds.getAlertPanelData(panel)
	case PanelTypeStatus:
		return ds.getStatusPanelData(panel)
	default:
		return nil, fmt.Errorf("unsupported panel type: %s", panel.Type)
	}
}

// getMetricPanelData returns data for a metric panel
func (ds *DashboardService) getMetricPanelData(panel *Panel) (map[string]interface{}, error) {
	allMetrics := ds.registry.GetMetrics()
	metric, exists := allMetrics[panel.Query]
	if !exists {
		return map[string]interface{}{
			"value": 0,
			"status": "no_data",
		}, nil
	}

	value, err := ds.extractNumericValue(metric.GetValue())
	if err != nil {
		return nil, err
	}

	// Determine status based on thresholds
	status := "ok"
	for _, threshold := range panel.Thresholds {
		if ds.evaluateThreshold(value, threshold) {
			status = string(threshold.Severity)
			break
		}
	}

	return map[string]interface{}{
		"value":  value,
		"status": status,
		"unit":   "", // Could be extracted from metric metadata
	}, nil
}

// getChartPanelData returns data for a chart panel
func (ds *DashboardService) getChartPanelData(panel *Panel) (map[string]interface{}, error) {
	ts, err := ds.GetTimeSeriesData(panel.Query, time.Hour) // Default to 1 hour
	if err != nil {
		return map[string]interface{}{
			"data_points": []DataPoint{},
			"status":      "no_data",
		}, nil
	}

	return map[string]interface{}{
		"data_points":  ts.DataPoints,
		"last_updated": ts.LastUpdated,
		"status":       "ok",
	}, nil
}

// getAlertPanelData returns data for an alert panel
func (ds *DashboardService) getAlertPanelData(panel *Panel) (map[string]interface{}, error) {
	activeAlerts := ds.alertingService.GetActiveAlerts()
	
	// Filter alerts if needed based on panel query
	// For now, return all active alerts
	return map[string]interface{}{
		"alerts": activeAlerts,
		"count":  len(activeAlerts),
		"status": "ok",
	}, nil
}

// getStatusPanelData returns data for a status panel
func (ds *DashboardService) getStatusPanelData(panel *Panel) (map[string]interface{}, error) {
	// This would typically check the health of various services
	// For now, return mock data
	return map[string]interface{}{
		"services": []map[string]interface{}{
			{"name": "kafka", "status": "healthy", "response_time": 5.2},
			{"name": "database", "status": "healthy", "response_time": 12.1},
			{"name": "openai_api", "status": "healthy", "response_time": 850.3},
		},
		"status": "ok",
	}, nil
}

// evaluateThreshold checks if a value meets a threshold condition
func (ds *DashboardService) evaluateThreshold(value float64, threshold Threshold) bool {
	switch threshold.Condition {
	case "gt":
		return value > threshold.Value
	case "lt":
		return value < threshold.Value
	case "eq":
		return value == threshold.Value
	default:
		return false
	}
}

// HTTPHandler returns an HTTP handler for dashboard endpoints
func (ds *DashboardService) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		switch r.URL.Path {
		case "/api/dashboard/overview":
			ds.handleSystemOverview(w, r)
		case "/api/dashboard/dashboards":
			ds.handleDashboards(w, r)
		case "/api/dashboard/metrics":
			ds.handleMetrics(w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

// handleSystemOverview handles system overview requests
func (ds *DashboardService) handleSystemOverview(w http.ResponseWriter, r *http.Request) {
	overview := ds.GetSystemOverview()
	json.NewEncoder(w).Encode(overview)
}

// handleDashboards handles dashboard requests
func (ds *DashboardService) handleDashboards(w http.ResponseWriter, r *http.Request) {
	dashboards := ds.GetDashboards()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"dashboards": dashboards,
	})
}

// handleMetrics handles metrics requests
func (ds *DashboardService) handleMetrics(w http.ResponseWriter, r *http.Request) {
	allMetrics := ds.registry.GetMetrics()
	
	// Convert metrics to a more dashboard-friendly format
	metricsData := make(map[string]interface{})
	for name, metric := range allMetrics {
		value, _ := ds.extractNumericValue(metric.GetValue())
		metricsData[name] = map[string]interface{}{
			"name":   metric.GetName(),
			"type":   metric.GetType(),
			"value":  value,
			"labels": metric.GetLabels(),
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"metrics":   metricsData,
		"timestamp": time.Now(),
	})
}

// GetMetrics returns dashboard service metrics
func (ds *DashboardService) GetMetrics() map[string]interface{} {
	ds.dataLock.RLock()
	timeSeriesCount := len(ds.timeSeriesData)
	ds.dataLock.RUnlock()

	ds.dashboardsLock.RLock()
	dashboardCount := len(ds.dashboards)
	ds.dashboardsLock.RUnlock()

	return map[string]interface{}{
		"dashboards_total":    dashboardCount,
		"time_series_total":   timeSeriesCount,
		"collection_interval": ds.config.CollectionInterval.String(),
		"data_retention":      ds.config.DataRetentionPeriod.String(),
	}
}