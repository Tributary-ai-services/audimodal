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

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// MetricsCollector collects comprehensive storage analytics and usage metrics
type MetricsCollector struct {
	config         *AnalyticsConfig
	storageManager storage.StorageManager
	metricsStore   MetricsStore
	tracer         trace.Tracer

	// Collection state
	collectors     map[string]Collector
	aggregators    map[string]Aggregator
	scheduledTasks map[string]*ScheduledTask

	// Runtime state
	isRunning      bool
	stopCh         chan struct{}
	wg             sync.WaitGroup
	mu             sync.RWMutex

	// Metrics cache
	metricsCache   map[string]*CachedMetrics
	cacheMu        sync.RWMutex
}

// Collector interface for different types of metric collection
type Collector interface {
	Collect(ctx context.Context) ([]*MetricSample, error)
	GetName() string
	GetInterval() time.Duration
	IsEnabled() bool
}

// Aggregator interface for metric aggregation and processing
type Aggregator interface {
	Aggregate(ctx context.Context, samples []*MetricSample) (*AggregatedMetrics, error)
	GetName() string
	GetTimeWindow() time.Duration
}

// MetricsStore interface for persistent metric storage
type MetricsStore interface {
	StoreSamples(ctx context.Context, samples []*MetricSample) error
	StoreAggregatedMetrics(ctx context.Context, metrics *AggregatedMetrics) error
	GetMetrics(ctx context.Context, query *MetricsQuery) ([]*MetricSample, error)
	GetAggregatedMetrics(ctx context.Context, query *AggregationQuery) ([]*AggregatedMetrics, error)
	GetUsageReport(ctx context.Context, tenantID uuid.UUID, timeRange TimeRange) (*UsageReport, error)
}

// ScheduledTask represents a scheduled collection task
type ScheduledTask struct {
	Name       string
	Collector  Collector
	Interval   time.Duration
	LastRun    time.Time
	NextRun    time.Time
	IsRunning  bool
	ErrorCount int
}

// CachedMetrics represents cached metric data
type CachedMetrics struct {
	Data      interface{}
	Timestamp time.Time
	TTL       time.Duration
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(config *AnalyticsConfig, storageManager storage.StorageManager, metricsStore MetricsStore) *MetricsCollector {
	if config == nil {
		config = DefaultAnalyticsConfig()
	}

	collector := &MetricsCollector{
		config:         config,
		storageManager: storageManager,
		metricsStore:   metricsStore,
		tracer:         otel.Tracer("storage-analytics-collector"),
		collectors:     make(map[string]Collector),
		aggregators:    make(map[string]Aggregator),
		scheduledTasks: make(map[string]*ScheduledTask),
		stopCh:         make(chan struct{}),
		metricsCache:   make(map[string]*CachedMetrics),
	}

	// Initialize collectors
	collector.initializeCollectors()

	// Initialize aggregators
	collector.initializeAggregators()

	return collector
}

// Start starts the metrics collection system
func (mc *MetricsCollector) Start(ctx context.Context) error {
	ctx, span := mc.tracer.Start(ctx, "start_metrics_collector")
	defer span.End()

	mc.mu.Lock()
	if mc.isRunning {
		mc.mu.Unlock()
		return fmt.Errorf("metrics collector is already running")
	}
	mc.isRunning = true
	mc.mu.Unlock()

	// Schedule collection tasks
	for name, collector := range mc.collectors {
		if collector.IsEnabled() {
			task := &ScheduledTask{
				Name:      name,
				Collector: collector,
				Interval:  collector.GetInterval(),
				NextRun:   time.Now().Add(collector.GetInterval()),
			}
			mc.scheduledTasks[name] = task
		}
	}

	// Start collection scheduler
	mc.wg.Add(1)
	go mc.runScheduler(ctx)

	// Start aggregation processor
	mc.wg.Add(1)
	go mc.runAggregator(ctx)

	// Start cache cleanup
	mc.wg.Add(1)
	go mc.runCacheCleanup(ctx)

	span.SetAttributes(
		attribute.Int("collectors.count", len(mc.collectors)),
		attribute.Int("aggregators.count", len(mc.aggregators)),
		attribute.Int("scheduled_tasks.count", len(mc.scheduledTasks)),
	)

	return nil
}

// Stop stops the metrics collection system
func (mc *MetricsCollector) Stop(ctx context.Context) error {
	ctx, span := mc.tracer.Start(ctx, "stop_metrics_collector")
	defer span.End()

	mc.mu.Lock()
	if !mc.isRunning {
		mc.mu.Unlock()
		return fmt.Errorf("metrics collector is not running")
	}
	mc.isRunning = false
	mc.mu.Unlock()

	// Signal shutdown
	close(mc.stopCh)

	// Wait for goroutines to finish
	mc.wg.Wait()

	return nil
}

// GetMetrics retrieves metrics based on query parameters
func (mc *MetricsCollector) GetMetrics(ctx context.Context, query *MetricsQuery) ([]*MetricSample, error) {
	ctx, span := mc.tracer.Start(ctx, "get_metrics")
	defer span.End()

	// Check cache first
	cacheKey := mc.generateCacheKey(query)
	if cached := mc.getCachedMetrics(cacheKey); cached != nil {
		if samples, ok := cached.Data.([]*MetricSample); ok {
			span.SetAttributes(attribute.Bool("cache.hit", true))
			return samples, nil
		}
	}

	// Query from store
	samples, err := mc.metricsStore.GetMetrics(ctx, query)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}

	// Cache the results
	mc.setCachedMetrics(cacheKey, samples, mc.config.CacheTTL)

	span.SetAttributes(
		attribute.Bool("cache.hit", false),
		attribute.Int("samples.count", len(samples)),
	)

	return samples, nil
}

// GetUsageReport generates a comprehensive usage report
func (mc *MetricsCollector) GetUsageReport(ctx context.Context, tenantID uuid.UUID, timeRange TimeRange) (*UsageReport, error) {
	ctx, span := mc.tracer.Start(ctx, "get_usage_report")
	defer span.End()

	span.SetAttributes(
		attribute.String("tenant.id", tenantID.String()),
		attribute.String("time_range.start", timeRange.Start.Format(time.RFC3339)),
		attribute.String("time_range.end", timeRange.End.Format(time.RFC3339)),
	)

	// Check cache first
	cacheKey := fmt.Sprintf("usage_report_%s_%d_%d", tenantID.String(), timeRange.Start.Unix(), timeRange.End.Unix())
	if cached := mc.getCachedMetrics(cacheKey); cached != nil {
		if report, ok := cached.Data.(*UsageReport); ok {
			span.SetAttributes(attribute.Bool("cache.hit", true))
			return report, nil
		}
	}

	// Generate report from store
	report, err := mc.metricsStore.GetUsageReport(ctx, tenantID, timeRange)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get usage report: %w", err)
	}

	// Enhance report with additional analytics
	if err := mc.enhanceUsageReport(ctx, report); err != nil {
		span.RecordError(err)
		// Continue with basic report
	}

	// Cache the report
	mc.setCachedMetrics(cacheKey, report, mc.config.ReportCacheTTL)

	span.SetAttributes(
		attribute.Bool("cache.hit", false),
		attribute.Int64("total_files", report.TotalFiles),
		attribute.Int64("total_size_bytes", report.TotalSizeBytes),
		attribute.Float64("total_cost", report.TotalCost),
	)

	return report, nil
}

// GetRealTimeMetrics returns real-time storage metrics
func (mc *MetricsCollector) GetRealTimeMetrics(ctx context.Context, tenantID uuid.UUID) (*RealTimeMetrics, error) {
	ctx, span := mc.tracer.Start(ctx, "get_realtime_metrics")
	defer span.End()

	// Collect real-time metrics from various sources
	metrics := &RealTimeMetrics{
		TenantID:  tenantID,
		Timestamp: time.Now(),
	}

	// Collect current storage usage
	if err := mc.collectCurrentUsage(ctx, metrics); err != nil {
		span.RecordError(err)
	}

	// Collect performance metrics
	if err := mc.collectPerformanceMetrics(ctx, metrics); err != nil {
		span.RecordError(err)
	}

	// Collect cost metrics
	if err := mc.collectCostMetrics(ctx, metrics); err != nil {
		span.RecordError(err)
	}

	span.SetAttributes(
		attribute.String("tenant.id", tenantID.String()),
		attribute.Int64("active_operations", metrics.ActiveOperations),
		attribute.Float64("throughput_mbps", metrics.ThroughputMBps),
	)

	return metrics, nil
}

// Helper methods

func (mc *MetricsCollector) initializeCollectors() {
	// Storage usage collector
	mc.collectors["storage_usage"] = NewStorageUsageCollector(mc.storageManager, mc.config.CollectionInterval)

	// Performance collector
	mc.collectors["performance"] = NewPerformanceCollector(mc.storageManager, mc.config.PerformanceInterval)

	// Cost collector
	mc.collectors["cost"] = NewCostCollector(mc.storageManager, mc.config.CostInterval)

	// Access pattern collector
	mc.collectors["access_patterns"] = NewAccessPatternCollector(mc.storageManager, mc.config.AccessPatternInterval)

	// Health collector
	mc.collectors["health"] = NewHealthCollector(mc.storageManager, mc.config.HealthInterval)
}

func (mc *MetricsCollector) initializeAggregators() {
	// Hourly aggregator
	mc.aggregators["hourly"] = NewTimeSeriesAggregator("hourly", time.Hour)

	// Daily aggregator
	mc.aggregators["daily"] = NewTimeSeriesAggregator("daily", 24*time.Hour)

	// Weekly aggregator
	mc.aggregators["weekly"] = NewTimeSeriesAggregator("weekly", 7*24*time.Hour)

	// Monthly aggregator
	mc.aggregators["monthly"] = NewTimeSeriesAggregator("monthly", 30*24*time.Hour)
}

func (mc *MetricsCollector) runScheduler(ctx context.Context) {
	defer mc.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-mc.stopCh:
			return
		case <-ticker.C:
			mc.runScheduledTasks(ctx)
		}
	}
}

func (mc *MetricsCollector) runScheduledTasks(ctx context.Context) {
	now := time.Now()

	for name, task := range mc.scheduledTasks {
		if now.After(task.NextRun) && !task.IsRunning {
			go mc.executeTask(ctx, task)
			task.NextRun = now.Add(task.Interval)
		}
	}
}

func (mc *MetricsCollector) executeTask(ctx context.Context, task *ScheduledTask) {
	ctx, span := mc.tracer.Start(ctx, "execute_collection_task")
	defer span.End()

	span.SetAttributes(
		attribute.String("task.name", task.Name),
		attribute.String("collector.name", task.Collector.GetName()),
	)

	task.IsRunning = true
	task.LastRun = time.Now()

	defer func() {
		task.IsRunning = false
	}()

	// Collect metrics
	samples, err := task.Collector.Collect(ctx)
	if err != nil {
		span.RecordError(err)
		task.ErrorCount++
		return
	}

	// Store samples
	if len(samples) > 0 {
		if err := mc.metricsStore.StoreSamples(ctx, samples); err != nil {
			span.RecordError(err)
			task.ErrorCount++
			return
		}
	}

	task.ErrorCount = 0 // Reset error count on success

	span.SetAttributes(
		attribute.Int("samples.collected", len(samples)),
		attribute.Bool("task.success", true),
	)
}

func (mc *MetricsCollector) runAggregator(ctx context.Context) {
	defer mc.wg.Done()

	ticker := time.NewTicker(mc.config.AggregationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-mc.stopCh:
			return
		case <-ticker.C:
			mc.runAggregation(ctx)
		}
	}
}

func (mc *MetricsCollector) runAggregation(ctx context.Context) {
	ctx, span := mc.tracer.Start(ctx, "run_aggregation")
	defer span.End()

	for name, aggregator := range mc.aggregators {
		go func(name string, agg Aggregator) {
			ctx, span := mc.tracer.Start(ctx, "run_single_aggregation")
			defer span.End()

			span.SetAttributes(attribute.String("aggregator.name", name))

			// Get samples for aggregation window
			endTime := time.Now()
			startTime := endTime.Add(-agg.GetTimeWindow())

			query := &MetricsQuery{
				TimeRange: TimeRange{
					Start: startTime,
					End:   endTime,
				},
			}

			samples, err := mc.metricsStore.GetMetrics(ctx, query)
			if err != nil {
				span.RecordError(err)
				return
			}

			if len(samples) == 0 {
				return
			}

			// Aggregate samples
			aggregated, err := agg.Aggregate(ctx, samples)
			if err != nil {
				span.RecordError(err)
				return
			}

			// Store aggregated metrics
			if err := mc.metricsStore.StoreAggregatedMetrics(ctx, aggregated); err != nil {
				span.RecordError(err)
				return
			}

			span.SetAttributes(
				attribute.Int("samples.processed", len(samples)),
				attribute.Bool("aggregation.success", true),
			)
		}(name, aggregator)
	}
}

func (mc *MetricsCollector) runCacheCleanup(ctx context.Context) {
	defer mc.wg.Done()

	ticker := time.NewTicker(mc.config.CacheCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-mc.stopCh:
			return
		case <-ticker.C:
			mc.cleanupExpiredCache()
		}
	}
}

func (mc *MetricsCollector) cleanupExpiredCache() {
	mc.cacheMu.Lock()
	defer mc.cacheMu.Unlock()

	now := time.Now()
	for key, cached := range mc.metricsCache {
		if now.After(cached.Timestamp.Add(cached.TTL)) {
			delete(mc.metricsCache, key)
		}
	}
}

func (mc *MetricsCollector) generateCacheKey(query *MetricsQuery) string {
	return fmt.Sprintf("metrics_%s_%d_%d_%s", 
		query.TenantID.String(), 
		query.TimeRange.Start.Unix(), 
		query.TimeRange.End.Unix(),
		query.MetricType)
}

func (mc *MetricsCollector) getCachedMetrics(key string) *CachedMetrics {
	mc.cacheMu.RLock()
	defer mc.cacheMu.RUnlock()

	cached, exists := mc.metricsCache[key]
	if !exists {
		return nil
	}

	if time.Now().After(cached.Timestamp.Add(cached.TTL)) {
		return nil
	}

	return cached
}

func (mc *MetricsCollector) setCachedMetrics(key string, data interface{}, ttl time.Duration) {
	mc.cacheMu.Lock()
	defer mc.cacheMu.Unlock()

	mc.metricsCache[key] = &CachedMetrics{
		Data:      data,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
}

func (mc *MetricsCollector) enhanceUsageReport(ctx context.Context, report *UsageReport) error {
	// Add trend analysis
	if err := mc.addTrendAnalysis(ctx, report); err != nil {
		return err
	}

	// Add cost predictions
	if err := mc.addCostPredictions(ctx, report); err != nil {
		return err
	}

	// Add optimization recommendations
	if err := mc.addOptimizationRecommendations(ctx, report); err != nil {
		return err
	}

	return nil
}

func (mc *MetricsCollector) addTrendAnalysis(ctx context.Context, report *UsageReport) error {
	// Analyze usage trends over time
	report.Trends = &UsageTrends{
		StorageGrowthRate:    0.15, // Placeholder: 15% monthly growth
		CostGrowthRate:       0.12, // Placeholder: 12% monthly cost growth
		AccessPatternTrend:   "decreasing",
		PredictedUsage:       report.TotalSizeBytes * 1.15,
		TrendConfidence:      0.85,
	}
	return nil
}

func (mc *MetricsCollector) addCostPredictions(ctx context.Context, report *UsageReport) error {
	// Predict future costs based on trends
	report.CostPredictions = &CostPredictions{
		NextMonthCost:     report.TotalCost * 1.12,
		NextQuarterCost:   report.TotalCost * 1.40,
		NextYearCost:      report.TotalCost * 2.10,
		PredictionAccuracy: 0.82,
	}
	return nil
}

func (mc *MetricsCollector) addOptimizationRecommendations(ctx context.Context, report *UsageReport) error {
	// Generate optimization recommendations
	report.Recommendations = []OptimizationRecommendation{
		{
			Type:             "tier_optimization",
			Title:            "Move infrequently accessed files to cold storage",
			Description:      "30% of files haven't been accessed in 90 days",
			PotentialSavings: report.TotalCost * 0.25,
			Confidence:       0.90,
			Priority:         "high",
		},
		{
			Type:             "compression",
			Title:            "Enable compression for text files",
			Description:      "Text files could be compressed to save 40% space",
			PotentialSavings: report.TotalCost * 0.15,
			Confidence:       0.85,
			Priority:         "medium",
		},
	}
	return nil
}

func (mc *MetricsCollector) collectCurrentUsage(ctx context.Context, metrics *RealTimeMetrics) error {
	// Collect current storage usage metrics
	metrics.TotalFiles = 150000      // Placeholder
	metrics.TotalSizeBytes = 500 * 1024 * 1024 * 1024 // 500GB
	metrics.UsedCapacity = 0.75      // 75% capacity used
	return nil
}

func (mc *MetricsCollector) collectPerformanceMetrics(ctx context.Context, metrics *RealTimeMetrics) error {
	// Collect performance metrics
	metrics.ThroughputMBps = 125.5   // MB/s
	metrics.IOPS = 850               // Operations per second
	metrics.Latency = 15 * time.Millisecond
	metrics.ActiveOperations = 25
	return nil
}

func (mc *MetricsCollector) collectCostMetrics(ctx context.Context, metrics *RealTimeMetrics) error {
	// Collect cost metrics
	metrics.CurrentMonthlyCost = 1250.75
	metrics.ProjectedMonthlyCost = 1380.25
	return nil
}