package detectors

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/TAS/audimodal/pkg/anomaly"
)

// StatisticalDetector implements statistical anomaly detection methods
type StatisticalDetector struct {
	name           string
	version        string
	enabled        bool
	config         *StatisticalConfig
	baselines      map[string]*anomaly.BaselineData
	lastUpdate     time.Time
}

// StatisticalConfig contains configuration for statistical detection
type StatisticalConfig struct {
	ZScoreThreshold     float64 `json:"z_score_threshold"`
	IQRMultiplier       float64 `json:"iqr_multiplier"`
	ModifiedZThreshold  float64 `json:"modified_z_threshold"`
	MinSampleSize       int     `json:"min_sample_size"`
	ConfidenceLevel     float64 `json:"confidence_level"`
	SlidingWindowSize   int     `json:"sliding_window_size"`
	UpdateFrequency     time.Duration `json:"update_frequency"`
	EnableTrendDetection bool   `json:"enable_trend_detection"`
	EnableSeasonalDetection bool `json:"enable_seasonal_detection"`
}

// NewStatisticalDetector creates a new statistical anomaly detector
func NewStatisticalDetector() *StatisticalDetector {
	return &StatisticalDetector{
		name:    "statistical_detector",
		version: "1.0.0",
		enabled: true,
		config: &StatisticalConfig{
			ZScoreThreshold:     2.5,
			IQRMultiplier:       1.5,
			ModifiedZThreshold:  3.5,
			MinSampleSize:       30,
			ConfidenceLevel:     0.95,
			SlidingWindowSize:   100,
			UpdateFrequency:     24 * time.Hour,
			EnableTrendDetection: true,
			EnableSeasonalDetection: true,
		},
		baselines: make(map[string]*anomaly.BaselineData),
	}
}

func (d *StatisticalDetector) GetName() string {
	return d.name
}

func (d *StatisticalDetector) GetVersion() string {
	return d.version
}

func (d *StatisticalDetector) GetSupportedTypes() []anomaly.AnomalyType {
	return []anomaly.AnomalyType{
		anomaly.AnomalyTypeOutlier,
		anomaly.AnomalyTypeTrend,
		anomaly.AnomalyTypeDistribution,
		anomaly.AnomalyTypeSeasonality,
		anomaly.AnomalyTypeContentSize,
		anomaly.AnomalyTypePerformance,
		anomaly.AnomalyTypeFrequency,
	}
}

func (d *StatisticalDetector) IsEnabled() bool {
	return d.enabled
}

func (d *StatisticalDetector) Configure(config map[string]interface{}) error {
	// Update configuration from provided map
	if threshold, ok := config["z_score_threshold"].(float64); ok {
		d.config.ZScoreThreshold = threshold
	}
	if multiplier, ok := config["iqr_multiplier"].(float64); ok {
		d.config.IQRMultiplier = multiplier
	}
	if enabled, ok := config["enabled"].(bool); ok {
		d.enabled = enabled
	}
	return nil
}

func (d *StatisticalDetector) DetectAnomalies(ctx context.Context, input *anomaly.DetectionInput) ([]*anomaly.Anomaly, error) {
	var anomalies []*anomaly.Anomaly

	// Get baseline for this tenant
	baselineKey := d.getBaselineKey(input.TenantID, "general")
	baseline, exists := d.baselines[baselineKey]
	if !exists || baseline.SampleCount < int64(d.config.MinSampleSize) {
		// Not enough baseline data yet
		return anomalies, nil
	}

	// Detect various statistical anomalies
	contentAnomalies, err := d.detectContentAnomalies(ctx, input, baseline)
	if err != nil {
		return nil, fmt.Errorf("content anomaly detection failed: %w", err)
	}
	anomalies = append(anomalies, contentAnomalies...)

	performanceAnomalies, err := d.detectPerformanceAnomalies(ctx, input, baseline)
	if err != nil {
		return nil, fmt.Errorf("performance anomaly detection failed: %w", err)
	}
	anomalies = append(anomalies, performanceAnomalies...)

	behavioralAnomalies, err := d.detectBehavioralAnomalies(ctx, input, baseline)
	if err != nil {
		return nil, fmt.Errorf("behavioral anomaly detection failed: %w", err)
	}
	anomalies = append(anomalies, behavioralAnomalies...)

	if d.config.EnableTrendDetection {
		trendAnomalies, err := d.detectTrendAnomalies(ctx, input, baseline)
		if err != nil {
			return nil, fmt.Errorf("trend anomaly detection failed: %w", err)
		}
		anomalies = append(anomalies, trendAnomalies...)
	}

	if d.config.EnableSeasonalDetection {
		seasonalAnomalies, err := d.detectSeasonalAnomalies(ctx, input, baseline)
		if err != nil {
			return nil, fmt.Errorf("seasonal anomaly detection failed: %w", err)
		}
		anomalies = append(anomalies, seasonalAnomalies...)
	}

	return anomalies, nil
}

func (d *StatisticalDetector) detectContentAnomalies(ctx context.Context, input *anomaly.DetectionInput, baseline *anomaly.BaselineData) ([]*anomaly.Anomaly, error) {
	var anomalies []*anomaly.Anomaly

	// Content size anomaly detection
	contentLength := float64(len(input.Content))
	if baseline.AvgContentLength > 0 {
		zScore := (contentLength - baseline.AvgContentLength) / baseline.StandardDev
		
		if math.Abs(zScore) > d.config.ZScoreThreshold {
			anomaly := &anomaly.Anomaly{
				ID:             uuid.New(),
				Type:           anomaly.AnomalyTypeContentSize,
				Severity:       d.calculateSeverity(math.Abs(zScore)),
				Status:         anomaly.StatusDetected,
				Title:          "Unusual Content Size",
				Description:    fmt.Sprintf("Content size (%d chars) deviates significantly from baseline (avg: %.0f chars, z-score: %.2f)", int(contentLength), baseline.AvgContentLength, zScore),
				DetectedAt:     time.Now(),
				UpdatedAt:      time.Now(),
				TenantID:       input.TenantID,
				DataSourceID:   input.DataSourceID,
				DocumentID:     input.DocumentID,
				ChunkID:        input.ChunkID,
				UserID:         input.UserID,
				Score:          math.Abs(zScore) / 5.0, // Normalize to 0-1 scale
				Confidence:     d.calculateConfidence(math.Abs(zScore), baseline.SampleCount),
				Threshold:      d.config.ZScoreThreshold,
				DetectorName:   d.name,
				DetectorVersion: d.version,
				Baseline: map[string]interface{}{
					"avg_content_length": baseline.AvgContentLength,
					"standard_dev":       baseline.StandardDev,
				},
				Detected: map[string]interface{}{
					"content_length": contentLength,
					"z_score":        zScore,
				},
				Metadata: map[string]interface{}{
					"detection_method": "z_score",
					"sample_count":     baseline.SampleCount,
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	// File size anomaly (if available)
	if input.FileSize > 0 {
		fileSizeMB := float64(input.FileSize) / (1024 * 1024)
		if baseline.Metadata != nil {
			if avgSizeInterface, exists := baseline.Metadata["avg_file_size_mb"]; exists {
				if avgSize, ok := avgSizeInterface.(float64); ok {
					if stdDevInterface, exists := baseline.Metadata["file_size_std_dev"]; exists {
						if stdDev, ok := stdDevInterface.(float64); ok && stdDev > 0 {
							zScore := (fileSizeMB - avgSize) / stdDev
							
							if math.Abs(zScore) > d.config.ZScoreThreshold {
								anomaly := &anomaly.Anomaly{
									ID:             uuid.New(),
									Type:           anomaly.AnomalyTypeContentSize,
									Severity:       d.calculateSeverity(math.Abs(zScore)),
									Status:         anomaly.StatusDetected,
									Title:          "Unusual File Size",
									Description:    fmt.Sprintf("File size (%.2f MB) deviates significantly from baseline (avg: %.2f MB, z-score: %.2f)", fileSizeMB, avgSize, zScore),
									DetectedAt:     time.Now(),
									UpdatedAt:      time.Now(),
									TenantID:       input.TenantID,
									DataSourceID:   input.DataSourceID,
									DocumentID:     input.DocumentID,
									UserID:         input.UserID,
									Score:          math.Abs(zScore) / 5.0,
									Confidence:     d.calculateConfidence(math.Abs(zScore), baseline.SampleCount),
									Threshold:      d.config.ZScoreThreshold,
									DetectorName:   d.name,
									DetectorVersion: d.version,
									Baseline: map[string]interface{}{
										"avg_file_size_mb": avgSize,
										"std_dev":          stdDev,
									},
									Detected: map[string]interface{}{
										"file_size_mb": fileSizeMB,
										"z_score":      zScore,
									},
									Metadata: map[string]interface{}{
										"detection_method": "z_score",
										"file_name":        input.FileName,
									},
								}
								anomalies = append(anomalies, anomaly)
							}
						}
					}
				}
			}
		}
	}

	return anomalies, nil
}

func (d *StatisticalDetector) detectPerformanceAnomalies(ctx context.Context, input *anomaly.DetectionInput, baseline *anomaly.BaselineData) ([]*anomaly.Anomaly, error) {
	var anomalies []*anomaly.Anomaly

	if input.UsageMetrics == nil {
		return anomalies, nil
	}

	metrics := input.UsageMetrics
	
	// Processing time anomaly
	if baseline.Metadata != nil {
		if avgProcessingInterface, exists := baseline.Metadata["avg_processing_time_ms"]; exists {
			if avgProcessing, ok := avgProcessingInterface.(float64); ok {
				if stdDevInterface, exists := baseline.Metadata["processing_time_std_dev"]; exists {
					if stdDev, ok := stdDevInterface.(float64); ok && stdDev > 0 {
						processingTimeMs := float64(metrics.ProcessingTime.Milliseconds())
						zScore := (processingTimeMs - avgProcessing) / stdDev
						
						if math.Abs(zScore) > d.config.ZScoreThreshold {
							anomaly := &anomaly.Anomaly{
								ID:             uuid.New(),
								Type:           anomaly.AnomalyTypePerformance,
								Severity:       d.calculateSeverity(math.Abs(zScore)),
								Status:         anomaly.StatusDetected,
								Title:          "Processing Time Anomaly",
								Description:    fmt.Sprintf("Processing time (%.0f ms) deviates significantly from baseline (avg: %.0f ms, z-score: %.2f)", processingTimeMs, avgProcessing, zScore),
								DetectedAt:     time.Now(),
								UpdatedAt:      time.Now(),
								TenantID:       input.TenantID,
								DataSourceID:   input.DataSourceID,
								DocumentID:     input.DocumentID,
								ChunkID:        input.ChunkID,
								UserID:         input.UserID,
								Score:          math.Abs(zScore) / 5.0,
								Confidence:     d.calculateConfidence(math.Abs(zScore), baseline.SampleCount),
								Threshold:      d.config.ZScoreThreshold,
								DetectorName:   d.name,
								DetectorVersion: d.version,
								Baseline: map[string]interface{}{
									"avg_processing_time_ms": avgProcessing,
									"std_dev":                stdDev,
								},
								Detected: map[string]interface{}{
									"processing_time_ms": processingTimeMs,
									"z_score":            zScore,
								},
								Metadata: map[string]interface{}{
									"detection_method": "z_score",
									"cpu_usage":        metrics.CPUUsage,
									"memory_usage":     metrics.MemoryUsage,
								},
							}
							anomalies = append(anomalies, anomaly)
						}
					}
				}
			}
		}

		// CPU usage anomaly
		if avgCPUInterface, exists := baseline.Metadata["avg_cpu_usage"]; exists {
			if avgCPU, ok := avgCPUInterface.(float64); ok {
				if stdDevInterface, exists := baseline.Metadata["cpu_usage_std_dev"]; exists {
					if stdDev, ok := stdDevInterface.(float64); ok && stdDev > 0 {
						zScore := (metrics.CPUUsage - avgCPU) / stdDev
						
						if math.Abs(zScore) > d.config.ZScoreThreshold {
							anomaly := &anomaly.Anomaly{
								ID:             uuid.New(),
								Type:           anomaly.AnomalyTypePerformance,
								Severity:       d.calculateSeverity(math.Abs(zScore)),
								Status:         anomaly.StatusDetected,
								Title:          "CPU Usage Anomaly",
								Description:    fmt.Sprintf("CPU usage (%.2f%%) deviates significantly from baseline (avg: %.2f%%, z-score: %.2f)", metrics.CPUUsage*100, avgCPU*100, zScore),
								DetectedAt:     time.Now(),
								UpdatedAt:      time.Now(),
								TenantID:       input.TenantID,
								Score:          math.Abs(zScore) / 5.0,
								Confidence:     d.calculateConfidence(math.Abs(zScore), baseline.SampleCount),
								Threshold:      d.config.ZScoreThreshold,
								DetectorName:   d.name,
								DetectorVersion: d.version,
								Baseline: map[string]interface{}{
									"avg_cpu_usage": avgCPU,
									"std_dev":       stdDev,
								},
								Detected: map[string]interface{}{
									"cpu_usage": metrics.CPUUsage,
									"z_score":   zScore,
								},
							}
							anomalies = append(anomalies, anomaly)
						}
					}
				}
			}
		}
	}

	return anomalies, nil
}

func (d *StatisticalDetector) detectBehavioralAnomalies(ctx context.Context, input *anomaly.DetectionInput, baseline *anomaly.BaselineData) ([]*anomaly.Anomaly, error) {
	var anomalies []*anomaly.Anomaly

	if input.AccessPattern == nil {
		return anomalies, nil
	}

	pattern := input.AccessPattern
	
	// Access frequency anomaly
	if baseline.TypicalAccess != nil {
		typical := baseline.TypicalAccess
		
		// View count anomaly
		if typical.ViewCount > 0 {
			ratio := float64(pattern.ViewCount) / float64(typical.ViewCount)
			if ratio > 3.0 || ratio < 0.3 { // More than 3x or less than 30% of typical
				severity := anomaly.SeverityMedium
				if ratio > 5.0 || ratio < 0.1 {
					severity = anomaly.SeverityHigh
				}
				
				anomaly := &anomaly.Anomaly{
					ID:             uuid.New(),
					Type:           anomaly.AnomalyTypeAccessPattern,
					Severity:       severity,
					Status:         anomaly.StatusDetected,
					Title:          "Unusual View Pattern",
					Description:    fmt.Sprintf("View count (%d) is %.1fx the typical count (%d)", pattern.ViewCount, ratio, typical.ViewCount),
					DetectedAt:     time.Now(),
					UpdatedAt:      time.Now(),
					TenantID:       input.TenantID,
					DataSourceID:   input.DataSourceID,
					DocumentID:     input.DocumentID,
					UserID:         input.UserID,
					Score:          math.Min(math.Abs(math.Log(ratio))/2.0, 1.0),
					Confidence:     0.8,
					Threshold:      3.0,
					DetectorName:   d.name,
					DetectorVersion: d.version,
					Baseline: map[string]interface{}{
						"typical_view_count": typical.ViewCount,
					},
					Detected: map[string]interface{}{
						"view_count": pattern.ViewCount,
						"ratio":      ratio,
					},
					Metadata: map[string]interface{}{
						"detection_method": "ratio_threshold",
					},
				}
				anomalies = append(anomalies, anomaly)
			}
		}

		// Unique users anomaly
		if typical.UniqueUsers > 0 {
			ratio := float64(pattern.UniqueUsers) / float64(typical.UniqueUsers)
			if ratio > 2.0 || ratio < 0.5 {
				severity := anomaly.SeverityLow
				if ratio > 4.0 || ratio < 0.25 {
					severity = anomaly.SeverityMedium
				}
				
				anomaly := &anomaly.Anomaly{
					ID:             uuid.New(),
					Type:           anomaly.AnomalyTypeAccessPattern,
					Severity:       severity,
					Status:         anomaly.StatusDetected,
					Title:          "Unusual User Access Pattern",
					Description:    fmt.Sprintf("Unique user count (%d) is %.1fx the typical count (%d)", pattern.UniqueUsers, ratio, typical.UniqueUsers),
					DetectedAt:     time.Now(),
					UpdatedAt:      time.Now(),
					TenantID:       input.TenantID,
					DataSourceID:   input.DataSourceID,
					DocumentID:     input.DocumentID,
					Score:          math.Min(math.Abs(math.Log(ratio))/1.5, 1.0),
					Confidence:     0.7,
					Threshold:      2.0,
					DetectorName:   d.name,
					DetectorVersion: d.version,
					Baseline: map[string]interface{}{
						"typical_unique_users": typical.UniqueUsers,
					},
					Detected: map[string]interface{}{
						"unique_users": pattern.UniqueUsers,
						"ratio":        ratio,
					},
				}
				anomalies = append(anomalies, anomaly)
			}
		}
	}

	return anomalies, nil
}

func (d *StatisticalDetector) detectTrendAnomalies(ctx context.Context, input *anomaly.DetectionInput, baseline *anomaly.BaselineData) ([]*anomaly.Anomaly, error) {
	var anomalies []*anomaly.Anomaly

	// Check for trend anomalies in time-series data
	if baseline.HourlyPattern != nil && len(baseline.HourlyPattern) == 24 {
		currentHour := input.Timestamp.Hour()
		expectedValue := baseline.HourlyPattern[currentHour]
		
		// For trend detection, we need a metric to compare against the pattern
		// Using access frequency as an example
		if input.AccessPattern != nil {
			actualValue := input.AccessPattern.AccessFrequency
			
			if expectedValue > 0 {
				deviation := math.Abs(actualValue-expectedValue) / expectedValue
				
				if deviation > 0.5 { // 50% deviation from expected hourly pattern
					severity := anomaly.SeverityLow
					if deviation > 1.0 {
						severity = anomaly.SeverityMedium
					}
					if deviation > 2.0 {
						severity = anomaly.SeverityHigh
					}
					
					anomaly := &anomaly.Anomaly{
						ID:             uuid.New(),
						Type:           anomaly.AnomalyTypeTrend,
						Severity:       severity,
						Status:         anomaly.StatusDetected,
						Title:          "Hourly Pattern Deviation",
						Description:    fmt.Sprintf("Access frequency (%.2f) deviates %.1f%% from expected hourly pattern (%.2f) for hour %d", actualValue, deviation*100, expectedValue, currentHour),
						DetectedAt:     time.Now(),
						UpdatedAt:      time.Now(),
						TenantID:       input.TenantID,
						DataSourceID:   input.DataSourceID,
						DocumentID:     input.DocumentID,
						Score:          math.Min(deviation/2.0, 1.0),
						Confidence:     0.75,
						Threshold:      0.5,
						DetectorName:   d.name,
						DetectorVersion: d.version,
						Baseline: map[string]interface{}{
							"expected_hourly_value": expectedValue,
							"hour":                  currentHour,
						},
						Detected: map[string]interface{}{
							"actual_value": actualValue,
							"deviation":    deviation,
						},
						Metadata: map[string]interface{}{
							"pattern_type": "hourly",
							"detection_method": "pattern_deviation",
						},
					}
					anomalies = append(anomalies, anomaly)
				}
			}
		}
	}

	return anomalies, nil
}

func (d *StatisticalDetector) detectSeasonalAnomalies(ctx context.Context, input *anomaly.DetectionInput, baseline *anomaly.BaselineData) ([]*anomaly.Anomaly, error) {
	var anomalies []*anomaly.Anomaly

	// Detect seasonal anomalies based on day of week patterns
	if baseline.WeeklyPattern != nil && len(baseline.WeeklyPattern) == 7 {
		dayOfWeek := int(input.Timestamp.Weekday())
		expectedValue := baseline.WeeklyPattern[dayOfWeek]
		
		if input.AccessPattern != nil && expectedValue > 0 {
			actualValue := input.AccessPattern.AccessFrequency
			deviation := math.Abs(actualValue-expectedValue) / expectedValue
			
			if deviation > 0.6 { // 60% deviation from expected weekly pattern
				severity := anomaly.SeverityLow
				if deviation > 1.2 {
					severity = anomaly.SeverityMedium
				}
				
				anomaly := &anomaly.Anomaly{
					ID:             uuid.New(),
					Type:           anomaly.AnomalyTypeSeasonality,
					Severity:       severity,
					Status:         anomaly.StatusDetected,
					Title:          "Weekly Pattern Deviation",
					Description:    fmt.Sprintf("Access frequency (%.2f) deviates %.1f%% from expected weekly pattern (%.2f) for %s", actualValue, deviation*100, expectedValue, input.Timestamp.Weekday().String()),
					DetectedAt:     time.Now(),
					UpdatedAt:      time.Now(),
					TenantID:       input.TenantID,
					DataSourceID:   input.DataSourceID,
					DocumentID:     input.DocumentID,
					Score:          math.Min(deviation/2.0, 1.0),
					Confidence:     0.7,
					Threshold:      0.6,
					DetectorName:   d.name,
					DetectorVersion: d.version,
					Baseline: map[string]interface{}{
						"expected_weekly_value": expectedValue,
						"day_of_week":           dayOfWeek,
					},
					Detected: map[string]interface{}{
						"actual_value": actualValue,
						"deviation":    deviation,
					},
					Metadata: map[string]interface{}{
						"pattern_type": "weekly",
						"detection_method": "seasonal_deviation",
					},
				}
				anomalies = append(anomalies, anomaly)
			}
		}
	}

	return anomalies, nil
}

func (d *StatisticalDetector) UpdateBaseline(ctx context.Context, data *anomaly.BaselineData) error {
	key := d.getBaselineKey(data.TenantID, data.DataType)
	data.UpdatedAt = time.Now()
	d.baselines[key] = data
	d.lastUpdate = time.Now()
	return nil
}

func (d *StatisticalDetector) GetBaseline(ctx context.Context) (*anomaly.BaselineData, error) {
	// Return a combined baseline or the most recent one
	// For simplicity, return the first baseline found
	for _, baseline := range d.baselines {
		return baseline, nil
	}
	return nil, fmt.Errorf("no baseline data available")
}

// Helper methods

func (d *StatisticalDetector) getBaselineKey(tenantID uuid.UUID, dataType string) string {
	return fmt.Sprintf("%s:%s", tenantID.String(), dataType)
}

func (d *StatisticalDetector) calculateSeverity(zScore float64) anomaly.AnomalySeverity {
	absZScore := math.Abs(zScore)
	if absZScore >= 4.0 {
		return anomaly.SeverityCritical
	} else if absZScore >= 3.0 {
		return anomaly.SeverityHigh
	} else if absZScore >= 2.0 {
		return anomaly.SeverityMedium
	}
	return anomaly.SeverityLow
}

func (d *StatisticalDetector) calculateConfidence(zScore float64, sampleCount int64) float64 {
	// Confidence increases with higher z-score and larger sample size
	absZScore := math.Abs(zScore)
	
	// Base confidence from z-score
	zScoreConfidence := math.Min(absZScore/5.0, 0.9)
	
	// Sample size confidence factor
	sampleFactor := math.Min(float64(sampleCount)/100.0, 1.0)
	
	// Combined confidence
	confidence := zScoreConfidence * (0.5 + 0.5*sampleFactor)
	
	return math.Max(0.1, math.Min(confidence, 0.95))
}

// IQRDetector implements Interquartile Range-based outlier detection
type IQRDetector struct {
	multiplier float64
}

func NewIQRDetector(multiplier float64) *IQRDetector {
	return &IQRDetector{
		multiplier: multiplier,
	}
}

func (d *IQRDetector) DetectOutliers(values []float64) []int {
	if len(values) < 4 {
		return nil
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := len(sorted)
	q1 := sorted[n/4]
	q3 := sorted[3*n/4]
	iqr := q3 - q1

	lowerBound := q1 - d.multiplier*iqr
	upperBound := q3 + d.multiplier*iqr

	var outliers []int
	for i, value := range values {
		if value < lowerBound || value > upperBound {
			outliers = append(outliers, i)
		}
	}

	return outliers
}

// ModifiedZScoreDetector implements Modified Z-Score method for outlier detection
type ModifiedZScoreDetector struct {
	threshold float64
}

func NewModifiedZScoreDetector(threshold float64) *ModifiedZScoreDetector {
	return &ModifiedZScoreDetector{
		threshold: threshold,
	}
}

func (d *ModifiedZScoreDetector) DetectOutliers(values []float64) []int {
	if len(values) < 3 {
		return nil
	}

	// Calculate median
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	
	var median float64
	n := len(sorted)
	if n%2 == 0 {
		median = (sorted[n/2-1] + sorted[n/2]) / 2
	} else {
		median = sorted[n/2]
	}

	// Calculate MAD (Median Absolute Deviation)
	deviations := make([]float64, len(values))
	for i, value := range values {
		deviations[i] = math.Abs(value - median)
	}
	
	sort.Float64s(deviations)
	var mad float64
	if n%2 == 0 {
		mad = (deviations[n/2-1] + deviations[n/2]) / 2
	} else {
		mad = deviations[n/2]
	}

	if mad == 0 {
		return nil // All values are the same
	}

	// Calculate modified z-scores and find outliers
	var outliers []int
	for i, value := range values {
		modifiedZScore := 0.6745 * (value - median) / mad
		if math.Abs(modifiedZScore) > d.threshold {
			outliers = append(outliers, i)
		}
	}

	return outliers
}