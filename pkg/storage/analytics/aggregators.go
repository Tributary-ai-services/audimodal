package analytics

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TimeSeriesAggregator aggregates metrics over time windows
type TimeSeriesAggregator struct {
	name       string
	timeWindow time.Duration
	tracer     trace.Tracer
}

// NewTimeSeriesAggregator creates a new time series aggregator
func NewTimeSeriesAggregator(name string, timeWindow time.Duration) *TimeSeriesAggregator {
	return &TimeSeriesAggregator{
		name:       name,
		timeWindow: timeWindow,
		tracer:     otel.Tracer("timeseries-aggregator"),
	}
}

// Aggregate aggregates metric samples into aggregated metrics
func (a *TimeSeriesAggregator) Aggregate(ctx context.Context, samples []*MetricSample) (*AggregatedMetrics, error) {
	ctx, span := a.tracer.Start(ctx, "aggregate_timeseries")
	defer span.End()

	if len(samples) == 0 {
		return nil, fmt.Errorf("no samples provided for aggregation")
	}

	span.SetAttributes(
		attribute.String("aggregator.name", a.name),
		attribute.String("time_window", a.timeWindow.String()),
		attribute.Int("samples.count", len(samples)),
	)

	// Group samples by metric type and name
	groupedSamples := a.groupSamples(samples)

	// For this implementation, we'll aggregate the first group
	// In a real implementation, you might want to aggregate all groups
	var firstGroup []*MetricSample
	var firstKey string
	for key, group := range groupedSamples {
		firstGroup = group
		firstKey = key
		break
	}

	if len(firstGroup) == 0 {
		return nil, fmt.Errorf("no samples in first group")
	}

	// Determine time window bounds
	endTime := time.Now()
	startTime := endTime.Add(-a.timeWindow)

	// Calculate aggregations
	aggregated := &AggregatedMetrics{
		ID:         uuid.New(),
		TimeWindow: a.timeWindow,
		StartTime:  startTime,
		EndTime:    endTime,
		MetricType: firstGroup[0].MetricType,
		MetricName: firstGroup[0].MetricName,
		Unit:       firstGroup[0].Unit,
		Labels:     firstGroup[0].Labels,
	}

	// Extract values and calculate statistics
	values := make([]float64, len(firstGroup))
	var sum float64
	for i, sample := range firstGroup {
		values[i] = sample.Value
		sum += sample.Value
	}

	aggregated.Count = int64(len(values))
	aggregated.Sum = sum
	aggregated.Average = sum / float64(len(values))
	aggregated.Min = a.calculateMin(values)
	aggregated.Max = a.calculateMax(values)
	aggregated.StdDev = a.calculateStdDev(values, aggregated.Average)
	aggregated.Percentiles = a.calculatePercentiles(values)

	// Set tenant ID if available
	if len(firstGroup) > 0 && firstGroup[0].TenantID != uuid.Nil {
		aggregated.TenantID = firstGroup[0].TenantID
	}

	span.SetAttributes(
		attribute.String("metric.key", firstKey),
		attribute.Float64("aggregated.average", aggregated.Average),
		attribute.Float64("aggregated.min", aggregated.Min),
		attribute.Float64("aggregated.max", aggregated.Max),
	)

	return aggregated, nil
}

// GetName returns the aggregator name
func (a *TimeSeriesAggregator) GetName() string {
	return a.name
}

// GetTimeWindow returns the time window for aggregation
func (a *TimeSeriesAggregator) GetTimeWindow() time.Duration {
	return a.timeWindow
}

// Helper methods

func (a *TimeSeriesAggregator) groupSamples(samples []*MetricSample) map[string][]*MetricSample {
	groups := make(map[string][]*MetricSample)
	
	for _, sample := range samples {
		key := fmt.Sprintf("%s:%s", sample.MetricType, sample.MetricName)
		groups[key] = append(groups[key], sample)
	}
	
	return groups
}

func (a *TimeSeriesAggregator) calculateMin(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func (a *TimeSeriesAggregator) calculateMax(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

func (a *TimeSeriesAggregator) calculateStdDev(values []float64, mean float64) float64 {
	if len(values) <= 1 {
		return 0
	}
	
	var sumSquaredDiffs float64
	for _, v := range values {
		diff := v - mean
		sumSquaredDiffs += diff * diff
	}
	
	variance := sumSquaredDiffs / float64(len(values)-1)
	return math.Sqrt(variance)
}

func (a *TimeSeriesAggregator) calculatePercentiles(values []float64) map[string]float64 {
	if len(values) == 0 {
		return map[string]float64{}
	}
	
	// Sort values for percentile calculation
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	
	percentiles := map[string]float64{
		"P50": a.percentile(sorted, 0.50),
		"P90": a.percentile(sorted, 0.90),
		"P95": a.percentile(sorted, 0.95),
		"P99": a.percentile(sorted, 0.99),
	}
	
	return percentiles
}

func (a *TimeSeriesAggregator) percentile(sortedValues []float64, p float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}
	
	if len(sortedValues) == 1 {
		return sortedValues[0]
	}
	
	index := p * float64(len(sortedValues)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))
	
	if lower == upper {
		return sortedValues[lower]
	}
	
	weight := index - float64(lower)
	return sortedValues[lower]*(1-weight) + sortedValues[upper]*weight
}

// StatisticalAggregator provides advanced statistical aggregations
type StatisticalAggregator struct {
	name       string
	timeWindow time.Duration
	tracer     trace.Tracer
}

// NewStatisticalAggregator creates a new statistical aggregator
func NewStatisticalAggregator(name string, timeWindow time.Duration) *StatisticalAggregator {
	return &StatisticalAggregator{
		name:       name,
		timeWindow: timeWindow,
		tracer:     otel.Tracer("statistical-aggregator"),
	}
}

// Aggregate performs advanced statistical aggregation
func (a *StatisticalAggregator) Aggregate(ctx context.Context, samples []*MetricSample) (*AggregatedMetrics, error) {
	ctx, span := a.tracer.Start(ctx, "aggregate_statistical")
	defer span.End()

	if len(samples) == 0 {
		return nil, fmt.Errorf("no samples provided for aggregation")
	}

	// Group samples by metric
	groupedSamples := a.groupSamplesByMetric(samples)
	
	// For now, aggregate the first group
	var firstGroup []*MetricSample
	for _, group := range groupedSamples {
		firstGroup = group
		break
	}

	if len(firstGroup) == 0 {
		return nil, fmt.Errorf("no samples in group")
	}

	values := make([]float64, len(firstGroup))
	for i, sample := range firstGroup {
		values[i] = sample.Value
	}

	aggregated := &AggregatedMetrics{
		ID:         uuid.New(),
		TimeWindow: a.timeWindow,
		StartTime:  time.Now().Add(-a.timeWindow),
		EndTime:    time.Now(),
		MetricType: firstGroup[0].MetricType,
		MetricName: firstGroup[0].MetricName,
		Unit:       firstGroup[0].Unit,
		Count:      int64(len(values)),
	}

	// Calculate advanced statistics
	aggregated.Sum = a.calculateSum(values)
	aggregated.Average = aggregated.Sum / float64(len(values))
	aggregated.Min = a.calculateMin(values)
	aggregated.Max = a.calculateMax(values)
	aggregated.StdDev = a.calculateStdDev(values, aggregated.Average)
	aggregated.Percentiles = a.calculateAdvancedPercentiles(values)

	// Add statistical metadata
	aggregated.Metadata = map[string]interface{}{
		"median":    aggregated.Percentiles["P50"],
		"iqr":       aggregated.Percentiles["P75"] - aggregated.Percentiles["P25"],
		"skewness":  a.calculateSkewness(values, aggregated.Average, aggregated.StdDev),
		"kurtosis":  a.calculateKurtosis(values, aggregated.Average, aggregated.StdDev),
		"variance":  aggregated.StdDev * aggregated.StdDev,
		"range":     aggregated.Max - aggregated.Min,
	}

	span.SetAttributes(
		attribute.Int("samples.count", len(values)),
		attribute.Float64("mean", aggregated.Average),
		attribute.Float64("stddev", aggregated.StdDev),
	)

	return aggregated, nil
}

func (a *StatisticalAggregator) GetName() string {
	return a.name
}

func (a *StatisticalAggregator) GetTimeWindow() time.Duration {
	return a.timeWindow
}

func (a *StatisticalAggregator) groupSamplesByMetric(samples []*MetricSample) map[string][]*MetricSample {
	groups := make(map[string][]*MetricSample)
	
	for _, sample := range samples {
		key := fmt.Sprintf("%s:%s", sample.MetricType, sample.MetricName)
		
		// Include labels in grouping
		if len(sample.Labels) > 0 {
			for k, v := range sample.Labels {
				key += fmt.Sprintf(":%s=%s", k, v)
			}
		}
		
		groups[key] = append(groups[key], sample)
	}
	
	return groups
}

func (a *StatisticalAggregator) calculateSum(values []float64) float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum
}

func (a *StatisticalAggregator) calculateMin(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func (a *StatisticalAggregator) calculateMax(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

func (a *StatisticalAggregator) calculateStdDev(values []float64, mean float64) float64 {
	if len(values) <= 1 {
		return 0
	}
	
	var sumSquaredDiffs float64
	for _, v := range values {
		diff := v - mean
		sumSquaredDiffs += diff * diff
	}
	
	variance := sumSquaredDiffs / float64(len(values)-1)
	return math.Sqrt(variance)
}

func (a *StatisticalAggregator) calculateAdvancedPercentiles(values []float64) map[string]float64 {
	if len(values) == 0 {
		return map[string]float64{}
	}
	
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	
	percentiles := map[string]float64{
		"P1":  a.percentile(sorted, 0.01),
		"P5":  a.percentile(sorted, 0.05),
		"P10": a.percentile(sorted, 0.10),
		"P25": a.percentile(sorted, 0.25),
		"P50": a.percentile(sorted, 0.50),
		"P75": a.percentile(sorted, 0.75),
		"P90": a.percentile(sorted, 0.90),
		"P95": a.percentile(sorted, 0.95),
		"P99": a.percentile(sorted, 0.99),
	}
	
	return percentiles
}

func (a *StatisticalAggregator) percentile(sortedValues []float64, p float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}
	
	if len(sortedValues) == 1 {
		return sortedValues[0]
	}
	
	index := p * float64(len(sortedValues)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))
	
	if lower == upper {
		return sortedValues[lower]
	}
	
	weight := index - float64(lower)
	return sortedValues[lower]*(1-weight) + sortedValues[upper]*weight
}

func (a *StatisticalAggregator) calculateSkewness(values []float64, mean, stddev float64) float64 {
	if len(values) <= 2 || stddev == 0 {
		return 0
	}
	
	var sumCubedDeviations float64
	for _, v := range values {
		deviation := (v - mean) / stddev
		sumCubedDeviations += deviation * deviation * deviation
	}
	
	n := float64(len(values))
	return (n / ((n - 1) * (n - 2))) * sumCubedDeviations
}

func (a *StatisticalAggregator) calculateKurtosis(values []float64, mean, stddev float64) float64 {
	if len(values) <= 3 || stddev == 0 {
		return 0
	}
	
	var sumFourthPowers float64
	for _, v := range values {
		deviation := (v - mean) / stddev
		fourthPower := deviation * deviation * deviation * deviation
		sumFourthPowers += fourthPower
	}
	
	n := float64(len(values))
	kurtosis := (n*(n+1))/((n-1)*(n-2)*(n-3))*sumFourthPowers - 3*(n-1)*(n-1)/((n-2)*(n-3))
	
	return kurtosis
}

// TrendAnalysisAggregator analyzes trends in metric data
type TrendAnalysisAggregator struct {
	name       string
	timeWindow time.Duration
	tracer     trace.Tracer
}

// NewTrendAnalysisAggregator creates a new trend analysis aggregator
func NewTrendAnalysisAggregator(name string, timeWindow time.Duration) *TrendAnalysisAggregator {
	return &TrendAnalysisAggregator{
		name:       name,
		timeWindow: timeWindow,
		tracer:     otel.Tracer("trend-analysis-aggregator"),
	}
}

// Aggregate performs trend analysis on metric samples
func (a *TrendAnalysisAggregator) Aggregate(ctx context.Context, samples []*MetricSample) (*AggregatedMetrics, error) {
	ctx, span := a.tracer.Start(ctx, "aggregate_trend_analysis")
	defer span.End()

	if len(samples) < 2 {
		return nil, fmt.Errorf("need at least 2 samples for trend analysis")
	}

	// Sort samples by timestamp
	sort.Slice(samples, func(i, j int) bool {
		return samples[i].Timestamp.Before(samples[j].Timestamp)
	})

	values := make([]float64, len(samples))
	timestamps := make([]float64, len(samples))
	
	baseTime := samples[0].Timestamp
	for i, sample := range samples {
		values[i] = sample.Value
		timestamps[i] = float64(sample.Timestamp.Sub(baseTime).Seconds())
	}

	aggregated := &AggregatedMetrics{
		ID:         uuid.New(),
		TimeWindow: a.timeWindow,
		StartTime:  samples[0].Timestamp,
		EndTime:    samples[len(samples)-1].Timestamp,
		MetricType: samples[0].MetricType,
		MetricName: samples[0].MetricName,
		Unit:       samples[0].Unit,
		Count:      int64(len(values)),
	}

	// Calculate basic statistics
	aggregated.Sum = a.sum(values)
	aggregated.Average = aggregated.Sum / float64(len(values))
	aggregated.Min = a.min(values)
	aggregated.Max = a.max(values)

	// Perform linear regression for trend analysis
	slope, intercept, r2 := a.linearRegression(timestamps, values)
	
	// Calculate trend direction and strength
	trendDirection := "stable"
	if slope > 0.001 {
		trendDirection = "increasing"
	} else if slope < -0.001 {
		trendDirection = "decreasing"
	}

	// Calculate rate of change
	rateOfChange := 0.0
	if len(values) > 1 {
		rateOfChange = (values[len(values)-1] - values[0]) / values[0] * 100
	}

	// Add trend analysis metadata
	aggregated.Metadata = map[string]interface{}{
		"trend_direction":    trendDirection,
		"trend_slope":        slope,
		"trend_intercept":    intercept,
		"trend_r_squared":    r2,
		"rate_of_change":     rateOfChange,
		"trend_strength":     a.categorizeTrendStrength(r2),
		"volatility":         a.calculateVolatility(values),
		"momentum":           a.calculateMomentum(values),
	}

	span.SetAttributes(
		attribute.String("trend.direction", trendDirection),
		attribute.Float64("trend.slope", slope),
		attribute.Float64("trend.r_squared", r2),
	)

	return aggregated, nil
}

func (a *TrendAnalysisAggregator) GetName() string {
	return a.name
}

func (a *TrendAnalysisAggregator) GetTimeWindow() time.Duration {
	return a.timeWindow
}

func (a *TrendAnalysisAggregator) sum(values []float64) float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum
}

func (a *TrendAnalysisAggregator) min(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func (a *TrendAnalysisAggregator) max(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

func (a *TrendAnalysisAggregator) linearRegression(x, y []float64) (slope, intercept, r2 float64) {
	if len(x) != len(y) || len(x) < 2 {
		return 0, 0, 0
	}

	n := float64(len(x))
	
	// Calculate means
	var sumX, sumY float64
	for i := 0; i < len(x); i++ {
		sumX += x[i]
		sumY += y[i]
	}
	meanX := sumX / n
	meanY := sumY / n

	// Calculate slope and intercept
	var sumXY, sumXX float64
	for i := 0; i < len(x); i++ {
		dx := x[i] - meanX
		dy := y[i] - meanY
		sumXY += dx * dy
		sumXX += dx * dx
	}

	if sumXX == 0 {
		return 0, meanY, 0
	}

	slope = sumXY / sumXX
	intercept = meanY - slope*meanX

	// Calculate R-squared
	var ssRes, ssTot float64
	for i := 0; i < len(y); i++ {
		predicted := slope*x[i] + intercept
		ssRes += (y[i] - predicted) * (y[i] - predicted)
		ssTot += (y[i] - meanY) * (y[i] - meanY)
	}

	if ssTot == 0 {
		r2 = 1
	} else {
		r2 = 1 - (ssRes / ssTot)
	}

	return slope, intercept, r2
}

func (a *TrendAnalysisAggregator) categorizeTrendStrength(r2 float64) string {
	if r2 >= 0.8 {
		return "strong"
	} else if r2 >= 0.5 {
		return "moderate"
	} else if r2 >= 0.2 {
		return "weak"
	}
	return "very_weak"
}

func (a *TrendAnalysisAggregator) calculateVolatility(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	// Calculate returns (percentage change between consecutive values)
	returns := make([]float64, len(values)-1)
	for i := 1; i < len(values); i++ {
		if values[i-1] != 0 {
			returns[i-1] = (values[i] - values[i-1]) / values[i-1]
		}
	}

	// Calculate standard deviation of returns
	if len(returns) == 0 {
		return 0
	}

	mean := a.sum(returns) / float64(len(returns))
	var sumSquaredDiffs float64
	for _, r := range returns {
		diff := r - mean
		sumSquaredDiffs += diff * diff
	}

	variance := sumSquaredDiffs / float64(len(returns))
	return math.Sqrt(variance)
}

func (a *TrendAnalysisAggregator) calculateMomentum(values []float64) float64 {
	if len(values) < 10 {
		return 0
	}

	// Calculate momentum as the rate of change over the last 10% of data points
	recentCount := int(math.Max(float64(len(values))*0.1, 2))
	recentValues := values[len(values)-recentCount:]
	
	if len(recentValues) < 2 {
		return 0
	}

	start := recentValues[0]
	end := recentValues[len(recentValues)-1]
	
	if start == 0 {
		return 0
	}

	return (end - start) / start * 100
}