package detectors

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jscharber/eAIIngest/pkg/anomaly"
)

// ContentDetector implements content-based anomaly detection
type ContentDetector struct {
	name           string
	version        string
	enabled        bool
	config         *ContentDetectorConfig
	baselines      map[string]*anomaly.BaselineData
	duplicateCache map[string][]string // Content hash -> document IDs
}

// ContentDetectorConfig contains configuration for content detection
type ContentDetectorConfig struct {
	MinContentLength             int                `json:"min_content_length"`
	MaxContentLength             int                `json:"max_content_length"`
	SentimentDeviationThreshold  float64            `json:"sentiment_deviation_threshold"`
	QualityThreshold             float64            `json:"quality_threshold"`
	LanguageConsistencyThreshold float64            `json:"language_consistency_threshold"`
	DuplicateThreshold           float64            `json:"duplicate_threshold"`
	StructureAnalysisEnabled     bool               `json:"structure_analysis_enabled"`
	TopicCoherenceThreshold      float64            `json:"topic_coherence_threshold"`
	EmotionalToneThreshold       float64            `json:"emotional_tone_threshold"`
	ReadabilityBounds            *ReadabilityBounds `json:"readability_bounds"`
	SuspiciousPatterns           []string           `json:"suspicious_patterns"`
	EnableSemanticAnalysis       bool               `json:"enable_semantic_analysis"`
}

// ReadabilityBounds defines acceptable readability score ranges
type ReadabilityBounds struct {
	MinFleschKincaid float64 `json:"min_flesch_kincaid"`
	MaxFleschKincaid float64 `json:"max_flesch_kincaid"`
	MinFleschReading float64 `json:"min_flesch_reading"`
	MaxFleschReading float64 `json:"max_flesch_reading"`
	MinGunningFog    float64 `json:"min_gunning_fog"`
	MaxGunningFog    float64 `json:"max_gunning_fog"`
}

// NewContentDetector creates a new content-based anomaly detector
func NewContentDetector() *ContentDetector {
	return &ContentDetector{
		name:    "content_detector",
		version: "1.0.0",
		enabled: true,
		config: &ContentDetectorConfig{
			MinContentLength:             10,
			MaxContentLength:             10000000, // 10MB
			SentimentDeviationThreshold:  0.7,
			QualityThreshold:             0.3,
			LanguageConsistencyThreshold: 0.8,
			DuplicateThreshold:           0.95,
			StructureAnalysisEnabled:     true,
			TopicCoherenceThreshold:      0.4,
			EmotionalToneThreshold:       0.8,
			ReadabilityBounds: &ReadabilityBounds{
				MinFleschKincaid: -10,
				MaxFleschKincaid: 20,
				MinFleschReading: 0,
				MaxFleschReading: 100,
				MinGunningFog:    5,
				MaxGunningFog:    25,
			},
			SuspiciousPatterns: []string{
				`(?i)(password|pwd|pass)\s*[:=]\s*\S+`,
				`(?i)(api[_-]?key|token)\s*[:=]\s*\S+`,
				`(?i)(secret|private[_-]?key)\s*[:=]\s*\S+`,
				`\b\d{4}[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}\b`, // Credit card pattern
				`\b\d{3}-\d{2}-\d{4}\b`,                      // SSN pattern
			},
			EnableSemanticAnalysis: true,
		},
		baselines:      make(map[string]*anomaly.BaselineData),
		duplicateCache: make(map[string][]string),
	}
}

func (d *ContentDetector) GetName() string {
	return d.name
}

func (d *ContentDetector) GetVersion() string {
	return d.version
}

func (d *ContentDetector) GetSupportedTypes() []anomaly.AnomalyType {
	return []anomaly.AnomalyType{
		anomaly.AnomalyTypeContentSize,
		anomaly.AnomalyTypeContentStructure,
		anomaly.AnomalyTypeContentSentiment,
		anomaly.AnomalyTypeContentLanguage,
		anomaly.AnomalyTypeContentQuality,
		anomaly.AnomalyTypeContentDuplication,
		anomaly.AnomalyTypeSuspiciousContent,
	}
}

func (d *ContentDetector) IsEnabled() bool {
	return d.enabled
}

func (d *ContentDetector) Configure(config map[string]interface{}) error {
	if enabled, ok := config["enabled"].(bool); ok {
		d.enabled = enabled
	}
	if threshold, ok := config["sentiment_deviation_threshold"].(float64); ok {
		d.config.SentimentDeviationThreshold = threshold
	}
	if threshold, ok := config["quality_threshold"].(float64); ok {
		d.config.QualityThreshold = threshold
	}
	return nil
}

func (d *ContentDetector) DetectAnomalies(ctx context.Context, input *anomaly.DetectionInput) ([]*anomaly.Anomaly, error) {
	var anomalies []*anomaly.Anomaly

	// Get baseline for this tenant
	baselineKey := d.getBaselineKey(input.TenantID, "content")
	baseline, exists := d.baselines[baselineKey]

	// Basic content validation anomalies
	basicAnomalies := d.detectBasicContentAnomalies(ctx, input)
	anomalies = append(anomalies, basicAnomalies...)

	// Suspicious pattern detection
	suspiciousAnomalies := d.detectSuspiciousPatterns(ctx, input)
	anomalies = append(anomalies, suspiciousAnomalies...)

	// Content duplication detection
	duplicateAnomalies := d.detectContentDuplication(ctx, input)
	anomalies = append(anomalies, duplicateAnomalies...)

	if exists && baseline.SampleCount > 10 {
		// Sentiment anomalies (requires baseline)
		sentimentAnomalies := d.detectSentimentAnomalies(ctx, input, baseline)
		anomalies = append(anomalies, sentimentAnomalies...)

		// Language consistency anomalies
		languageAnomalies := d.detectLanguageAnomalies(ctx, input, baseline)
		anomalies = append(anomalies, languageAnomalies...)

		// Quality anomalies
		qualityAnomalies := d.detectQualityAnomalies(ctx, input, baseline)
		anomalies = append(anomalies, qualityAnomalies...)

		// Topic coherence anomalies
		topicAnomalies := d.detectTopicAnomalies(ctx, input, baseline)
		anomalies = append(anomalies, topicAnomalies...)
	}

	// Structure analysis anomalies
	if d.config.StructureAnalysisEnabled {
		structureAnomalies := d.detectStructureAnomalies(ctx, input)
		anomalies = append(anomalies, structureAnomalies...)
	}

	return anomalies, nil
}

func (d *ContentDetector) detectBasicContentAnomalies(ctx context.Context, input *anomaly.DetectionInput) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	contentLength := len(input.Content)

	// Content length anomalies
	if contentLength < d.config.MinContentLength {
		anomaly := &anomaly.Anomaly{
			ID:              uuid.New(),
			Type:            anomaly.AnomalyTypeContentSize,
			Severity:        anomaly.SeverityLow,
			Status:          anomaly.StatusDetected,
			Title:           "Content Too Short",
			Description:     fmt.Sprintf("Content length (%d chars) is below minimum threshold (%d chars)", contentLength, d.config.MinContentLength),
			DetectedAt:      time.Now(),
			UpdatedAt:       time.Now(),
			TenantID:        input.TenantID,
			DataSourceID:    input.DataSourceID,
			DocumentID:      input.DocumentID,
			ChunkID:         input.ChunkID,
			UserID:          input.UserID,
			Score:           float64(d.config.MinContentLength-contentLength) / float64(d.config.MinContentLength),
			Confidence:      0.9,
			Threshold:       float64(d.config.MinContentLength),
			DetectorName:    d.name,
			DetectorVersion: d.version,
			Detected: map[string]interface{}{
				"content_length": contentLength,
			},
			Metadata: map[string]interface{}{
				"detection_method": "threshold",
				"file_name":        input.FileName,
			},
		}
		anomalies = append(anomalies, anomaly)
	}

	if contentLength > d.config.MaxContentLength {
		anomaly := &anomaly.Anomaly{
			ID:              uuid.New(),
			Type:            anomaly.AnomalyTypeContentSize,
			Severity:        anomaly.SeverityMedium,
			Status:          anomaly.StatusDetected,
			Title:           "Content Too Large",
			Description:     fmt.Sprintf("Content length (%d chars) exceeds maximum threshold (%d chars)", contentLength, d.config.MaxContentLength),
			DetectedAt:      time.Now(),
			UpdatedAt:       time.Now(),
			TenantID:        input.TenantID,
			DataSourceID:    input.DataSourceID,
			DocumentID:      input.DocumentID,
			ChunkID:         input.ChunkID,
			UserID:          input.UserID,
			Score:           math.Min(float64(contentLength-d.config.MaxContentLength)/float64(d.config.MaxContentLength), 1.0),
			Confidence:      0.95,
			Threshold:       float64(d.config.MaxContentLength),
			DetectorName:    d.name,
			DetectorVersion: d.version,
			Detected: map[string]interface{}{
				"content_length": contentLength,
			},
			Metadata: map[string]interface{}{
				"detection_method": "threshold",
				"severity_reason":  "excessive_size",
			},
		}
		anomalies = append(anomalies, anomaly)
	}

	// Empty or whitespace-only content
	trimmedContent := strings.TrimSpace(input.Content)
	if len(trimmedContent) == 0 && contentLength > 0 {
		anomaly := &anomaly.Anomaly{
			ID:              uuid.New(),
			Type:            anomaly.AnomalyTypeContentStructure,
			Severity:        anomaly.SeverityMedium,
			Status:          anomaly.StatusDetected,
			Title:           "Empty Content with Size",
			Description:     fmt.Sprintf("Document has size (%d chars) but contains only whitespace", contentLength),
			DetectedAt:      time.Now(),
			UpdatedAt:       time.Now(),
			TenantID:        input.TenantID,
			DataSourceID:    input.DataSourceID,
			DocumentID:      input.DocumentID,
			ChunkID:         input.ChunkID,
			UserID:          input.UserID,
			Score:           0.8,
			Confidence:      0.95,
			DetectorName:    d.name,
			DetectorVersion: d.version,
			Detected: map[string]interface{}{
				"content_length":         contentLength,
				"trimmed_content_length": len(trimmedContent),
			},
			Metadata: map[string]interface{}{
				"detection_method": "content_analysis",
				"issue_type":       "whitespace_only",
			},
		}
		anomalies = append(anomalies, anomaly)
	}

	return anomalies
}

func (d *ContentDetector) detectSuspiciousPatterns(ctx context.Context, input *anomaly.DetectionInput) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	for _, pattern := range d.config.SuspiciousPatterns {
		regex, err := regexp.Compile(pattern)
		if err != nil {
			continue // Skip invalid patterns
		}

		matches := regex.FindAllString(input.Content, -1)
		if len(matches) > 0 {
			// Determine severity based on pattern type
			severity := anomaly.SeverityMedium
			patternType := "unknown"

			if strings.Contains(pattern, "password|pwd|pass") {
				severity = anomaly.SeverityHigh
				patternType = "password"
			} else if strings.Contains(pattern, "api[_-]?key|token") {
				severity = anomaly.SeverityCritical
				patternType = "api_key"
			} else if strings.Contains(pattern, "secret|private[_-]?key") {
				severity = anomaly.SeverityCritical
				patternType = "secret_key"
			} else if strings.Contains(pattern, "\\d{4}[-\\s]?\\d{4}") {
				severity = anomaly.SeverityHigh
				patternType = "credit_card"
			} else if strings.Contains(pattern, "\\d{3}-\\d{2}-\\d{4}") {
				severity = anomaly.SeverityCritical
				patternType = "ssn"
			}

			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeSuspiciousContent,
				Severity:        severity,
				Status:          anomaly.StatusDetected,
				Title:           "Suspicious Pattern Detected",
				Description:     fmt.Sprintf("Found %d matches of potentially sensitive pattern (%s)", len(matches), patternType),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				DataSourceID:    input.DataSourceID,
				DocumentID:      input.DocumentID,
				ChunkID:         input.ChunkID,
				UserID:          input.UserID,
				Score:           math.Min(float64(len(matches))/5.0, 1.0),
				Confidence:      0.85,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				Detected: map[string]interface{}{
					"pattern_type": patternType,
					"match_count":  len(matches),
					"first_match":  matches[0],
				},
				Metadata: map[string]interface{}{
					"detection_method": "regex_pattern",
					"pattern":          pattern,
					"all_matches":      matches,
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies
}

func (d *ContentDetector) detectContentDuplication(ctx context.Context, input *anomaly.DetectionInput) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	// Generate content hash (simplified)
	contentHash := d.generateContentHash(input.Content)

	// Check for duplicates
	if existingDocuments, exists := d.duplicateCache[contentHash]; exists {
		// Found duplicate content
		similarity := 1.0 // Exact match in this simplified implementation

		if similarity >= d.config.DuplicateThreshold {
			severity := anomaly.SeverityLow
			if len(existingDocuments) > 2 {
				severity = anomaly.SeverityMedium
			}
			if len(existingDocuments) > 5 {
				severity = anomaly.SeverityHigh
			}

			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeContentDuplication,
				Severity:        severity,
				Status:          anomaly.StatusDetected,
				Title:           "Duplicate Content Detected",
				Description:     fmt.Sprintf("Content is %.1f%% similar to %d existing documents", similarity*100, len(existingDocuments)),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				DataSourceID:    input.DataSourceID,
				DocumentID:      input.DocumentID,
				ChunkID:         input.ChunkID,
				UserID:          input.UserID,
				Score:           similarity,
				Confidence:      0.9,
				Threshold:       d.config.DuplicateThreshold,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				Detected: map[string]interface{}{
					"similarity_score":   similarity,
					"duplicate_count":    len(existingDocuments),
					"existing_documents": existingDocuments,
				},
				Metadata: map[string]interface{}{
					"detection_method": "content_hash",
					"content_hash":     contentHash,
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	// Add current document to cache
	if input.DocumentID != nil {
		d.duplicateCache[contentHash] = append(d.duplicateCache[contentHash], input.DocumentID.String())
	}

	return anomalies
}

func (d *ContentDetector) detectSentimentAnomalies(ctx context.Context, input *anomaly.DetectionInput, baseline *anomaly.BaselineData) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	if input.SentimentResult == nil || baseline.SentimentBaseline == nil {
		return anomalies
	}

	sentiment := input.SentimentResult

	// Check sentiment score deviation
	if avgScore, exists := baseline.SentimentBaseline["avg_score"]; exists {
		deviation := math.Abs(sentiment.Score - avgScore)

		if deviation > d.config.SentimentDeviationThreshold {
			severity := anomaly.SeverityLow
			if deviation > 0.8 {
				severity = anomaly.SeverityMedium
			}
			if deviation > 0.9 {
				severity = anomaly.SeverityHigh
			}

			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeContentSentiment,
				Severity:        severity,
				Status:          anomaly.StatusDetected,
				Title:           "Unusual Sentiment Score",
				Description:     fmt.Sprintf("Sentiment score (%.2f) deviates significantly from baseline (%.2f), deviation: %.2f", sentiment.Score, avgScore, deviation),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				DataSourceID:    input.DataSourceID,
				DocumentID:      input.DocumentID,
				ChunkID:         input.ChunkID,
				UserID:          input.UserID,
				Score:           deviation,
				Confidence:      0.75,
				Threshold:       d.config.SentimentDeviationThreshold,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				Baseline: map[string]interface{}{
					"avg_sentiment_score": avgScore,
				},
				Detected: map[string]interface{}{
					"sentiment_score": sentiment.Score,
					"sentiment_label": sentiment.Label,
					"magnitude":       sentiment.Magnitude,
					"deviation":       deviation,
				},
				Metadata: map[string]interface{}{
					"detection_method": "sentiment_deviation",
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	// Check for extreme sentiment with high magnitude
	if sentiment.Magnitude > 0.9 && (sentiment.Label == "positive" || sentiment.Label == "negative") {
		anomaly := &anomaly.Anomaly{
			ID:              uuid.New(),
			Type:            anomaly.AnomalyTypeContentSentiment,
			Severity:        anomaly.SeverityMedium,
			Status:          anomaly.StatusDetected,
			Title:           "Extreme Sentiment Detected",
			Description:     fmt.Sprintf("Document has extreme %s sentiment with high magnitude (%.2f)", sentiment.Label, sentiment.Magnitude),
			DetectedAt:      time.Now(),
			UpdatedAt:       time.Now(),
			TenantID:        input.TenantID,
			DataSourceID:    input.DataSourceID,
			DocumentID:      input.DocumentID,
			ChunkID:         input.ChunkID,
			UserID:          input.UserID,
			Score:           sentiment.Magnitude,
			Confidence:      0.8,
			Threshold:       0.9,
			DetectorName:    d.name,
			DetectorVersion: d.version,
			Detected: map[string]interface{}{
				"sentiment_label": sentiment.Label,
				"magnitude":       sentiment.Magnitude,
				"score":           sentiment.Score,
			},
			Metadata: map[string]interface{}{
				"detection_method": "extreme_sentiment",
			},
		}
		anomalies = append(anomalies, anomaly)
	}

	return anomalies
}

func (d *ContentDetector) detectLanguageAnomalies(ctx context.Context, input *anomaly.DetectionInput, baseline *anomaly.BaselineData) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	if input.LanguageResult == nil || baseline.CommonLanguages == nil {
		return anomalies
	}

	language := input.LanguageResult

	// Check if detected language is uncommon for this tenant
	if freq, exists := baseline.CommonLanguages[language.Language]; exists && freq < d.config.LanguageConsistencyThreshold {
		severity := anomaly.SeverityLow
		if freq < 0.1 {
			severity = anomaly.SeverityMedium
		}

		anomaly := &anomaly.Anomaly{
			ID:              uuid.New(),
			Type:            anomaly.AnomalyTypeContentLanguage,
			Severity:        severity,
			Status:          anomaly.StatusDetected,
			Title:           "Unusual Language Detected",
			Description:     fmt.Sprintf("Document language (%s) is uncommon for this tenant (frequency: %.2f%%)", language.Language, freq*100),
			DetectedAt:      time.Now(),
			UpdatedAt:       time.Now(),
			TenantID:        input.TenantID,
			DataSourceID:    input.DataSourceID,
			DocumentID:      input.DocumentID,
			ChunkID:         input.ChunkID,
			UserID:          input.UserID,
			Score:           1.0 - freq,
			Confidence:      language.Confidence,
			Threshold:       d.config.LanguageConsistencyThreshold,
			DetectorName:    d.name,
			DetectorVersion: d.version,
			Baseline: map[string]interface{}{
				"common_languages": baseline.CommonLanguages,
			},
			Detected: map[string]interface{}{
				"language":   language.Language,
				"confidence": language.Confidence,
				"dialect":    language.Dialect,
				"script":     language.Script,
			},
			Metadata: map[string]interface{}{
				"detection_method": "language_consistency",
			},
		}
		anomalies = append(anomalies, anomaly)
	} else {
		// Language not seen before in baseline
		anomaly := &anomaly.Anomaly{
			ID:              uuid.New(),
			Type:            anomaly.AnomalyTypeContentLanguage,
			Severity:        anomaly.SeverityMedium,
			Status:          anomaly.StatusDetected,
			Title:           "New Language Detected",
			Description:     fmt.Sprintf("Document language (%s) has not been seen before for this tenant", language.Language),
			DetectedAt:      time.Now(),
			UpdatedAt:       time.Now(),
			TenantID:        input.TenantID,
			DataSourceID:    input.DataSourceID,
			DocumentID:      input.DocumentID,
			ChunkID:         input.ChunkID,
			UserID:          input.UserID,
			Score:           0.8,
			Confidence:      language.Confidence,
			DetectorName:    d.name,
			DetectorVersion: d.version,
			Detected: map[string]interface{}{
				"language":   language.Language,
				"confidence": language.Confidence,
				"dialect":    language.Dialect,
				"script":     language.Script,
			},
			Metadata: map[string]interface{}{
				"detection_method": "new_language",
			},
		}
		anomalies = append(anomalies, anomaly)
	}

	return anomalies
}

func (d *ContentDetector) detectQualityAnomalies(ctx context.Context, input *anomaly.DetectionInput, baseline *anomaly.BaselineData) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	if input.QualityMetrics == nil {
		return anomalies
	}

	quality := input.QualityMetrics

	// Overall quality too low
	if quality.OverallScore < d.config.QualityThreshold {
		severity := anomaly.SeverityLow
		if quality.OverallScore < 0.2 {
			severity = anomaly.SeverityMedium
		}
		if quality.OverallScore < 0.1 {
			severity = anomaly.SeverityHigh
		}

		anomaly := &anomaly.Anomaly{
			ID:              uuid.New(),
			Type:            anomaly.AnomalyTypeContentQuality,
			Severity:        severity,
			Status:          anomaly.StatusDetected,
			Title:           "Low Content Quality",
			Description:     fmt.Sprintf("Content quality score (%.2f) is below threshold (%.2f)", quality.OverallScore, d.config.QualityThreshold),
			DetectedAt:      time.Now(),
			UpdatedAt:       time.Now(),
			TenantID:        input.TenantID,
			DataSourceID:    input.DataSourceID,
			DocumentID:      input.DocumentID,
			ChunkID:         input.ChunkID,
			UserID:          input.UserID,
			Score:           1.0 - quality.OverallScore,
			Confidence:      0.7,
			Threshold:       d.config.QualityThreshold,
			DetectorName:    d.name,
			DetectorVersion: d.version,
			Detected: map[string]interface{}{
				"overall_score":      quality.OverallScore,
				"coherence":          quality.Coherence,
				"clarity":            quality.Clarity,
				"completeness":       quality.Completeness,
				"informativeness":    quality.Informativeness,
				"structural_quality": quality.StructuralQuality,
				"issues":             quality.Issues,
			},
			Metadata: map[string]interface{}{
				"detection_method": "quality_threshold",
			},
		}
		anomalies = append(anomalies, anomaly)
	}

	// High redundancy
	if quality.Redundancy > 0.8 {
		anomaly := &anomaly.Anomaly{
			ID:              uuid.New(),
			Type:            anomaly.AnomalyTypeContentQuality,
			Severity:        anomaly.SeverityLow,
			Status:          anomaly.StatusDetected,
			Title:           "High Content Redundancy",
			Description:     fmt.Sprintf("Content has high redundancy score (%.2f), indicating repetitive content", quality.Redundancy),
			DetectedAt:      time.Now(),
			UpdatedAt:       time.Now(),
			TenantID:        input.TenantID,
			DataSourceID:    input.DataSourceID,
			DocumentID:      input.DocumentID,
			ChunkID:         input.ChunkID,
			UserID:          input.UserID,
			Score:           quality.Redundancy,
			Confidence:      0.75,
			Threshold:       0.8,
			DetectorName:    d.name,
			DetectorVersion: d.version,
			Detected: map[string]interface{}{
				"redundancy": quality.Redundancy,
			},
			Metadata: map[string]interface{}{
				"detection_method": "redundancy_threshold",
			},
		}
		anomalies = append(anomalies, anomaly)
	}

	return anomalies
}

func (d *ContentDetector) detectTopicAnomalies(ctx context.Context, input *anomaly.DetectionInput, baseline *anomaly.BaselineData) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	if input.TopicResults == nil || len(input.TopicResults) == 0 || baseline.CommonTopics == nil {
		return anomalies
	}

	// Check topic coherence
	for _, topic := range input.TopicResults {
		if topic.Coherence < d.config.TopicCoherenceThreshold {
			severity := anomaly.SeverityLow
			if topic.Coherence < 0.3 {
				severity = anomaly.SeverityMedium
			}

			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeContentStructure,
				Severity:        severity,
				Status:          anomaly.StatusDetected,
				Title:           "Low Topic Coherence",
				Description:     fmt.Sprintf("Topic '%s' has low coherence score (%.2f)", topic.Topic, topic.Coherence),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				DataSourceID:    input.DataSourceID,
				DocumentID:      input.DocumentID,
				ChunkID:         input.ChunkID,
				UserID:          input.UserID,
				Score:           1.0 - topic.Coherence,
				Confidence:      topic.Probability,
				Threshold:       d.config.TopicCoherenceThreshold,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				Detected: map[string]interface{}{
					"topic":       topic.Topic,
					"coherence":   topic.Coherence,
					"probability": topic.Probability,
					"keywords":    topic.Keywords,
				},
				Metadata: map[string]interface{}{
					"detection_method": "topic_coherence",
				},
			}
			anomalies = append(anomalies, anomaly)
		}

		// Check if topic is uncommon for this tenant
		if freq, exists := baseline.CommonTopics[topic.Topic]; exists && freq < 0.1 && topic.Probability > 0.7 {
			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeContentStructure,
				Severity:        anomaly.SeverityLow,
				Status:          anomaly.StatusDetected,
				Title:           "Unusual Topic Detected",
				Description:     fmt.Sprintf("High-probability topic '%s' (%.2f) is uncommon for this tenant (frequency: %.2f%%)", topic.Topic, topic.Probability, freq*100),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				DataSourceID:    input.DataSourceID,
				DocumentID:      input.DocumentID,
				ChunkID:         input.ChunkID,
				UserID:          input.UserID,
				Score:           topic.Probability * (1.0 - freq),
				Confidence:      0.6,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				Baseline: map[string]interface{}{
					"topic_frequency": freq,
				},
				Detected: map[string]interface{}{
					"topic":       topic.Topic,
					"probability": topic.Probability,
					"keywords":    topic.Keywords,
				},
				Metadata: map[string]interface{}{
					"detection_method": "unusual_topic",
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies
}

func (d *ContentDetector) detectStructureAnomalies(ctx context.Context, input *anomaly.DetectionInput) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	content := input.Content

	// Detect unusual character patterns
	nonPrintableCount := 0
	specialCharCount := 0
	digitCount := 0
	letterCount := 0

	for _, r := range content {
		if r < 32 || r > 126 {
			nonPrintableCount++
		} else if r >= '0' && r <= '9' {
			digitCount++
		} else if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			letterCount++
		} else {
			specialCharCount++
		}
	}

	totalChars := len(content)
	if totalChars > 0 {
		nonPrintableRatio := float64(nonPrintableCount) / float64(totalChars)
		digitRatio := float64(digitCount) / float64(totalChars)

		// High non-printable character ratio
		if nonPrintableRatio > 0.1 {
			severity := anomaly.SeverityMedium
			if nonPrintableRatio > 0.3 {
				severity = anomaly.SeverityHigh
			}

			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeContentStructure,
				Severity:        severity,
				Status:          anomaly.StatusDetected,
				Title:           "High Non-Printable Character Ratio",
				Description:     fmt.Sprintf("Content contains %.1f%% non-printable characters", nonPrintableRatio*100),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				DataSourceID:    input.DataSourceID,
				DocumentID:      input.DocumentID,
				ChunkID:         input.ChunkID,
				UserID:          input.UserID,
				Score:           nonPrintableRatio,
				Confidence:      0.85,
				Threshold:       0.1,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				Detected: map[string]interface{}{
					"non_printable_ratio": nonPrintableRatio,
					"non_printable_count": nonPrintableCount,
					"total_chars":         totalChars,
				},
				Metadata: map[string]interface{}{
					"detection_method": "character_analysis",
				},
			}
			anomalies = append(anomalies, anomaly)
		}

		// Unusually high digit ratio (might indicate data dumps, encoded content, etc.)
		if digitRatio > 0.7 && totalChars > 100 {
			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeContentStructure,
				Severity:        anomaly.SeverityLow,
				Status:          anomaly.StatusDetected,
				Title:           "High Digit Density",
				Description:     fmt.Sprintf("Content contains %.1f%% digits, suggesting data or encoded content", digitRatio*100),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				DataSourceID:    input.DataSourceID,
				DocumentID:      input.DocumentID,
				ChunkID:         input.ChunkID,
				UserID:          input.UserID,
				Score:           digitRatio,
				Confidence:      0.7,
				Threshold:       0.7,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				Detected: map[string]interface{}{
					"digit_ratio": digitRatio,
					"digit_count": digitCount,
					"total_chars": totalChars,
				},
				Metadata: map[string]interface{}{
					"detection_method": "digit_density",
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies
}

func (d *ContentDetector) UpdateBaseline(ctx context.Context, data *anomaly.BaselineData) error {
	key := d.getBaselineKey(data.TenantID, data.DataType)
	data.UpdatedAt = time.Now()
	d.baselines[key] = data
	return nil
}

func (d *ContentDetector) GetBaseline(ctx context.Context) (*anomaly.BaselineData, error) {
	for _, baseline := range d.baselines {
		return baseline, nil
	}
	return nil, fmt.Errorf("no baseline data available")
}

// Helper methods

func (d *ContentDetector) getBaselineKey(tenantID uuid.UUID, dataType string) string {
	return fmt.Sprintf("%s:%s", tenantID.String(), dataType)
}

func (d *ContentDetector) generateContentHash(content string) string {
	// Simplified hash generation - in production, use a proper hash function
	// This could be implemented with SHA-256 or similar
	normalized := strings.ToLower(strings.TrimSpace(content))
	hash := 0
	for _, r := range normalized {
		hash = hash*31 + int(r)
	}
	return fmt.Sprintf("%x", hash)
}
