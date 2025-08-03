package classification

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Service implements the ClassificationService interface
type Service struct {
	classifiers         map[string]Classifier
	languageDetector    LanguageDetector
	sentimentAnalyzer   SentimentAnalyzer
	entityExtractor     EntityExtractor
	keywordExtractor    KeywordExtractor
	topicModeler        TopicModeler
	contentTypeDetector *ContentTypeDetector
	structureAnalyzer   *DocumentStructureAnalyzer

	// Configuration
	config *ServiceConfig

	// Metrics and monitoring
	metrics *ClassificationMetrics
	tracer  trace.Tracer

	// Concurrency control
	mu        sync.RWMutex
	semaphore chan struct{}
}

// ServiceConfig contains configuration for the classification service
type ServiceConfig struct {
	MaxConcurrentClassifications int           `yaml:"max_concurrent_classifications"`
	DefaultTimeout               time.Duration `yaml:"default_timeout"`
	EnableMetrics                bool          `yaml:"enable_metrics"`
	EnableTracing                bool          `yaml:"enable_tracing"`
	DefaultConfidenceThreshold   float64       `yaml:"default_confidence_threshold"`
	MaxTextLength                int           `yaml:"max_text_length"`
	MaxBatchSize                 int           `yaml:"max_batch_size"`
}

// DefaultServiceConfig returns default service configuration
func DefaultServiceConfig() *ServiceConfig {
	return &ServiceConfig{
		MaxConcurrentClassifications: 10,
		DefaultTimeout:               30 * time.Second,
		EnableMetrics:                true,
		EnableTracing:                true,
		DefaultConfidenceThreshold:   0.7,
		MaxTextLength:                1000000, // 1MB
		MaxBatchSize:                 100,
	}
}

// NewService creates a new classification service
func NewService(config *ServiceConfig) *Service {
	if config == nil {
		config = DefaultServiceConfig()
	}

	service := &Service{
		classifiers: make(map[string]Classifier),
		config:      config,
		metrics: &ClassificationMetrics{
			ClassificationsByType:     make(map[ContentType]int64),
			ClassificationsByLanguage: make(map[Language]int64),
		},
		tracer:    otel.Tracer("classification-service"),
		semaphore: make(chan struct{}, config.MaxConcurrentClassifications),
	}

	// Register default components
	service.RegisterLanguageDetector(NewBasicLanguageDetector())
	service.RegisterSentimentAnalyzer(NewBasicSentimentAnalyzer())
	service.RegisterEntityExtractor(NewBasicEntityExtractor())
	service.RegisterKeywordExtractor(NewBasicKeywordExtractor())
	service.RegisterTopicModeler(NewBasicTopicModeler())

	// Initialize content type detector and structure analyzer
	service.contentTypeDetector = NewContentTypeDetector()
	service.structureAnalyzer = NewDocumentStructureAnalyzer()

	return service
}

// RegisterLanguageDetector registers a language detector
func (s *Service) RegisterLanguageDetector(detector LanguageDetector) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.languageDetector = detector
}

// RegisterSentimentAnalyzer registers a sentiment analyzer
func (s *Service) RegisterSentimentAnalyzer(analyzer SentimentAnalyzer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sentimentAnalyzer = analyzer
}

// RegisterEntityExtractor registers an entity extractor
func (s *Service) RegisterEntityExtractor(extractor EntityExtractor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entityExtractor = extractor
}

// RegisterKeywordExtractor registers a keyword extractor
func (s *Service) RegisterKeywordExtractor(extractor KeywordExtractor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keywordExtractor = extractor
}

// RegisterTopicModeler registers a topic modeler
func (s *Service) RegisterTopicModeler(modeler TopicModeler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.topicModeler = modeler
}

// RegisterClassifier registers a custom classifier
func (s *Service) RegisterClassifier(classifier Classifier) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := classifier.GetName()
	if name == "" {
		return fmt.Errorf("classifier name cannot be empty")
	}

	s.classifiers[name] = classifier
	return nil
}

// GetClassifiers returns all registered classifiers
func (s *Service) GetClassifiers() []Classifier {
	s.mu.RLock()
	defer s.mu.RUnlock()

	classifiers := make([]Classifier, 0, len(s.classifiers))
	for _, classifier := range s.classifiers {
		classifiers = append(classifiers, classifier)
	}

	return classifiers
}

// Classify performs comprehensive content classification
func (s *Service) Classify(ctx context.Context, input *ClassificationInput) (*ClassificationResult, error) {
	// Acquire semaphore for concurrency control
	select {
	case s.semaphore <- struct{}{}:
		defer func() { <-s.semaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	startTime := time.Now()

	// Add tracing
	ctx, span := s.tracer.Start(ctx, "classify_content")
	defer span.End()

	span.SetAttributes(
		attribute.String("tenant.id", input.TenantID.String()),
		attribute.String("content.type", string(s.detectContentType(input))),
		attribute.Int("content.length", len(input.Content)),
	)

	// Validate input
	if err := s.validateInput(input); err != nil {
		span.RecordError(err)
		s.updateMetrics(false, time.Since(startTime))
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	// Apply timeout
	if input.Options == nil || input.Options.ConfidenceThreshold == 0 {
		if input.Options == nil {
			input.Options = DefaultClassificationOptions()
		} else {
			input.Options.ConfidenceThreshold = s.config.DefaultConfidenceThreshold
		}
	}

	ctx, cancel := context.WithTimeout(ctx, s.config.DefaultTimeout)
	defer cancel()

	// Create result
	result := NewClassificationResult(input.TenantID)
	result.FileID = input.FileID
	result.ChunkID = input.ChunkID

	// Perform classification
	if err := s.performClassification(ctx, input, result); err != nil {
		span.RecordError(err)
		s.updateMetrics(false, time.Since(startTime))
		return nil, fmt.Errorf("classification failed: %w", err)
	}

	// Calculate processing time
	processingTime := time.Since(startTime)
	result.ProcessingTime = processingTime
	result.ProcessedBy = "classification-service"

	// Update metrics
	s.updateMetrics(true, processingTime)
	s.updateClassificationCounters(result)

	span.SetAttributes(
		attribute.Float64("classification.confidence", result.Confidence),
		attribute.String("classification.language", string(result.Language)),
		attribute.String("classification.sensitivity", string(result.SensitivityLevel)),
		attribute.Int64("processing.time_ms", processingTime.Milliseconds()),
	)

	return result, nil
}

// ClassifyBatch performs batch classification
func (s *Service) ClassifyBatch(ctx context.Context, inputs []*ClassificationInput) ([]*ClassificationResult, error) {
	if len(inputs) == 0 {
		return []*ClassificationResult{}, nil
	}

	if len(inputs) > s.config.MaxBatchSize {
		return nil, fmt.Errorf("batch size %d exceeds maximum %d", len(inputs), s.config.MaxBatchSize)
	}

	ctx, span := s.tracer.Start(ctx, "classify_batch")
	defer span.End()

	span.SetAttributes(attribute.Int("batch.size", len(inputs)))

	results := make([]*ClassificationResult, len(inputs))
	errors := make([]error, len(inputs))

	// Process in parallel with goroutines
	var wg sync.WaitGroup
	for i, input := range inputs {
		wg.Add(1)
		go func(index int, inp *ClassificationInput) {
			defer wg.Done()
			result, err := s.Classify(ctx, inp)
			results[index] = result
			errors[index] = err
		}(i, input)
	}

	wg.Wait()

	// Check for errors
	var firstError error
	successCount := 0
	for i, err := range errors {
		if err != nil && firstError == nil {
			firstError = err
		}
		if results[i] != nil {
			successCount++
		}
	}

	span.SetAttributes(
		attribute.Int("batch.successful", successCount),
		attribute.Int("batch.failed", len(inputs)-successCount),
	)

	if firstError != nil && successCount == 0 {
		return nil, fmt.Errorf("batch classification failed: %w", firstError)
	}

	return results, nil
}

// performClassification performs the actual classification work
func (s *Service) performClassification(ctx context.Context, input *ClassificationInput, result *ClassificationResult) error {
	// Use enhanced content type detection
	if s.contentTypeDetector != nil {
		if contentResult, err := s.contentTypeDetector.DetectContentType(ctx, input); err == nil {
			result.ContentType = contentResult.ContentType
			result.DocumentType = contentResult.DocumentType
			result.MimeType = contentResult.MimeType

			// Add content detection metadata
			result.Metadata["content_detection_confidence"] = contentResult.Confidence
			result.Metadata["content_detection_method"] = contentResult.DetectionMethod
			result.Metadata["content_characteristics"] = contentResult.Characteristics
		} else {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Enhanced content detection failed: %v", err))
			// Fallback to basic detection
			result.ContentType = s.detectContentType(input)
			result.DocumentType = s.detectDocumentType(input)
			result.MimeType = input.MimeType
		}
	} else {
		// Fallback to basic detection
		result.ContentType = s.detectContentType(input)
		result.DocumentType = s.detectDocumentType(input)
		result.MimeType = input.MimeType
	}

	// Perform language detection
	if input.Options.EnableLanguageDetection && s.languageDetector != nil {
		if langResult, err := s.languageDetector.DetectLanguage(ctx, input.Content); err == nil {
			result.Language = langResult.Language
			result.LanguageConfidence = langResult.Confidence
		} else {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Language detection failed: %v", err))
		}
	}

	// Perform sentiment analysis
	if input.Options.EnableSentimentAnalysis && s.sentimentAnalyzer != nil {
		if sentiment, err := s.sentimentAnalyzer.AnalyzeSentiment(ctx, input.Content); err == nil {
			result.Sentiment = sentiment
		} else {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Sentiment analysis failed: %v", err))
		}
	}

	// Extract entities
	if input.Options.EnableEntityExtraction && s.entityExtractor != nil {
		if entities, err := s.entityExtractor.ExtractEntities(ctx, input.Content); err == nil {
			if len(entities) > input.Options.MaxEntities {
				entities = entities[:input.Options.MaxEntities]
			}
			result.Entities = entities
		} else {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Entity extraction failed: %v", err))
		}
	}

	// Extract keywords
	if input.Options.EnableKeywordExtraction && s.keywordExtractor != nil {
		if keywords, err := s.keywordExtractor.ExtractKeywords(ctx, input.Content, input.Options.MaxKeywords); err == nil {
			result.Keywords = keywords
		} else {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Keyword extraction failed: %v", err))
		}
	}

	// Extract topics
	if input.Options.EnableTopicModeling && s.topicModeler != nil {
		if topics, err := s.topicModeler.ExtractTopics(ctx, input.Content, input.Options.MaxTopics); err == nil {
			result.Topics = topics
		} else {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Topic modeling failed: %v", err))
		}
	}

	// Classify sensitivity
	s.classifySensitivity(input, result)

	// Analyze compliance
	s.analyzeCompliance(input, result)

	// Run custom classifiers
	s.runCustomClassifiers(ctx, input, result)

	// Perform document structure analysis
	if input.Options.EnableStructureAnalysis && s.structureAnalyzer != nil {
		if structureResult, err := s.structureAnalyzer.AnalyzeStructure(ctx, input.Content, result.ContentType); err == nil {
			result.Metadata["document_structure"] = structureResult
			result.Metadata["structure_statistics"] = structureResult.Statistics
			result.Metadata["structure_metadata"] = structureResult.Metadata
		} else {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Document structure analysis failed: %v", err))
		}
	}

	// Calculate overall confidence
	result.Confidence = s.calculateOverallConfidence(result)

	return nil
}

// detectContentType detects the content type from input
func (s *Service) detectContentType(input *ClassificationInput) ContentType {
	// Check MIME type first
	if input.MimeType != "" {
		switch {
		case strings.HasPrefix(input.MimeType, "text/"):
			return ContentTypeDocument
		case strings.Contains(input.MimeType, "spreadsheet") ||
			strings.Contains(input.MimeType, "excel") ||
			input.MimeType == "text/csv":
			return ContentTypeSpreadsheet
		case strings.Contains(input.MimeType, "presentation") ||
			strings.Contains(input.MimeType, "powerpoint"):
			return ContentTypePresentation
		case strings.HasPrefix(input.MimeType, "image/"):
			return ContentTypeImage
		case strings.HasPrefix(input.MimeType, "video/"):
			return ContentTypeVideo
		case strings.HasPrefix(input.MimeType, "audio/"):
			return ContentTypeAudio
		case strings.Contains(input.MimeType, "pdf"):
			return ContentTypeDocument
		}
	}

	// Check filename extension
	if input.Filename != "" {
		ext := strings.ToLower(input.Filename[strings.LastIndex(input.Filename, ".")+1:])
		switch ext {
		case "txt", "md", "html", "xml", "rtf":
			return ContentTypeDocument
		case "pdf", "doc", "docx":
			return ContentTypeDocument
		case "xls", "xlsx", "csv":
			return ContentTypeSpreadsheet
		case "ppt", "pptx":
			return ContentTypePresentation
		case "jpg", "jpeg", "png", "gif", "bmp", "svg":
			return ContentTypeImage
		case "mp4", "avi", "mov", "wmv", "flv":
			return ContentTypeVideo
		case "mp3", "wav", "flac", "aac":
			return ContentTypeAudio
		case "js", "py", "java", "cpp", "c", "go", "rb":
			return ContentTypeCode
		case "log":
			return ContentTypeLog
		case "sql", "db":
			return ContentTypeDatabase
		}
	}

	// Analyze content patterns
	content := strings.ToLower(input.Content)
	if len(content) > 0 {
		// Check for code patterns
		if strings.Contains(content, "function") ||
			strings.Contains(content, "class ") ||
			strings.Contains(content, "import ") ||
			strings.Contains(content, "#include") {
			return ContentTypeCode
		}

		// Check for email patterns
		if strings.Contains(content, "from:") &&
			strings.Contains(content, "to:") &&
			strings.Contains(content, "subject:") {
			return ContentTypeEmail
		}

		// Check for log patterns
		if strings.Contains(content, "[info]") ||
			strings.Contains(content, "[error]") ||
			strings.Contains(content, "timestamp") {
			return ContentTypeLog
		}

		// Check for HTML/web content
		if strings.Contains(content, "<html") ||
			strings.Contains(content, "<body") ||
			strings.Contains(content, "<!doctype") {
			return ContentTypeWeb
		}
	}

	return ContentTypeDocument // Default
}

// detectDocumentType detects the specific document type
func (s *Service) detectDocumentType(input *ClassificationInput) DocumentType {
	if input.MimeType != "" {
		switch input.MimeType {
		case "application/pdf":
			return DocumentTypePDF
		case "application/msword", "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
			return DocumentTypeWord
		case "text/plain":
			return DocumentTypeText
		case "text/markdown":
			return DocumentTypeMarkdown
		case "text/html":
			return DocumentTypeHTML
		case "application/xml", "text/xml":
			return DocumentTypeXML
		case "application/json":
			return DocumentTypeJSON
		case "text/csv":
			return DocumentTypeCSV
		case "application/vnd.ms-excel", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
			return DocumentTypeExcel
		case "application/vnd.ms-powerpoint", "application/vnd.openxmlformats-officedocument.presentationml.presentation":
			return DocumentTypePowerPoint
		}
	}

	if input.Filename != "" {
		ext := strings.ToLower(input.Filename[strings.LastIndex(input.Filename, ".")+1:])
		switch ext {
		case "pdf":
			return DocumentTypePDF
		case "doc", "docx":
			return DocumentTypeWord
		case "txt":
			return DocumentTypeText
		case "md":
			return DocumentTypeMarkdown
		case "html", "htm":
			return DocumentTypeHTML
		case "xml":
			return DocumentTypeXML
		case "json":
			return DocumentTypeJSON
		case "csv":
			return DocumentTypeCSV
		case "xls", "xlsx":
			return DocumentTypeExcel
		case "ppt", "pptx":
			return DocumentTypePowerPoint
		case "rtf":
			return DocumentTypeRTF
		}
	}

	return DocumentTypeText // Default
}

// classifySensitivity classifies content sensitivity
func (s *Service) classifySensitivity(input *ClassificationInput, result *ClassificationResult) {
	sensitivityScore := 0.0
	reasons := []string{}

	content := strings.ToLower(input.Content)

	// Check for high-sensitivity patterns
	highSensitivityPatterns := []string{
		"confidential", "classified", "secret", "restricted",
		"ssn", "social security", "credit card", "password",
		"medical record", "patient", "health information",
		"financial", "bank account", "routing number",
	}

	for _, pattern := range highSensitivityPatterns {
		if strings.Contains(content, pattern) {
			sensitivityScore += 0.3
			reasons = append(reasons, fmt.Sprintf("Contains sensitive pattern: %s", pattern))
		}
	}

	// Check for PII entities
	for _, entity := range result.Entities {
		switch entity.Type {
		case EntityTypeSSN, EntityTypeCreditCard, EntityTypePhone:
			sensitivityScore += 0.4
			reasons = append(reasons, fmt.Sprintf("Contains PII: %s", entity.Type))
		case EntityTypePerson, EntityTypeEmail:
			sensitivityScore += 0.2
			reasons = append(reasons, fmt.Sprintf("Contains personal information: %s", entity.Type))
		}
	}

	// Classify based on score
	if sensitivityScore >= 0.8 {
		result.SensitivityLevel = SensitivityRestricted
	} else if sensitivityScore >= 0.6 {
		result.SensitivityLevel = SensitivityConfidential
	} else if sensitivityScore >= 0.3 {
		result.SensitivityLevel = SensitivityInternal
	} else {
		result.SensitivityLevel = SensitivityPublic
	}

	result.SensitivityScore = sensitivityScore
	result.SensitivityReasons = reasons
}

// analyzeCompliance analyzes compliance requirements
func (s *Service) analyzeCompliance(input *ClassificationInput, result *ClassificationResult) {
	flags := []ComplianceFramework{}
	risks := []ComplianceRisk{}

	// Check for GDPR requirements
	if s.hasPersonalData(result) {
		flags = append(flags, ComplianceGDPR)
		risks = append(risks, ComplianceRisk{
			Framework:   ComplianceGDPR,
			RiskLevel:   "medium",
			Description: "Contains personal data subject to GDPR",
			Mitigation:  []string{"Implement data protection measures", "Obtain consent", "Enable data portability"},
		})
	}

	// Check for HIPAA requirements
	if s.hasHealthInformation(input.Content) {
		flags = append(flags, ComplianceHIPAA)
		risks = append(risks, ComplianceRisk{
			Framework:   ComplianceHIPAA,
			RiskLevel:   "high",
			Description: "Contains protected health information",
			Mitigation:  []string{"Implement HIPAA safeguards", "Encrypt data", "Audit access"},
		})
	}

	// Check for PCI requirements
	if s.hasPaymentData(result) {
		flags = append(flags, CompliancePCI)
		risks = append(risks, ComplianceRisk{
			Framework:   CompliancePCI,
			RiskLevel:   "high",
			Description: "Contains payment card information",
			Mitigation:  []string{"Implement PCI DSS controls", "Tokenize card data", "Secure transmission"},
		})
	}

	result.ComplianceFlags = flags
	result.ComplianceRisks = risks
}

// hasPersonalData checks if content contains personal data
func (s *Service) hasPersonalData(result *ClassificationResult) bool {
	for _, entity := range result.Entities {
		if entity.Type == EntityTypePerson || entity.Type == EntityTypeEmail || entity.Type == EntityTypePhone {
			return true
		}
	}
	return false
}

// hasHealthInformation checks if content contains health information
func (s *Service) hasHealthInformation(content string) bool {
	healthTerms := []string{
		"patient", "medical", "diagnosis", "treatment", "medication",
		"health record", "medical record", "hospital", "clinic", "doctor",
	}

	lowerContent := strings.ToLower(content)
	for _, term := range healthTerms {
		if strings.Contains(lowerContent, term) {
			return true
		}
	}
	return false
}

// hasPaymentData checks if content contains payment data
func (s *Service) hasPaymentData(result *ClassificationResult) bool {
	for _, entity := range result.Entities {
		if entity.Type == EntityTypeCreditCard {
			return true
		}
	}
	return false
}

// runCustomClassifiers runs all registered custom classifiers
func (s *Service) runCustomClassifiers(ctx context.Context, input *ClassificationInput, result *ClassificationResult) {
	s.mu.RLock()
	classifiers := make([]Classifier, 0, len(s.classifiers))
	for _, classifier := range s.classifiers {
		classifiers = append(classifiers, classifier)
	}
	s.mu.RUnlock()

	for _, classifier := range classifiers {
		// Check if classifier supports this content type
		supportedTypes := classifier.GetSupportedTypes()
		supported := false
		for _, supportedType := range supportedTypes {
			if supportedType == result.ContentType {
				supported = true
				break
			}
		}

		if !supported {
			continue
		}

		// Run classifier
		if classifierResult, err := classifier.Classify(ctx, input); err == nil {
			// Merge results
			s.mergeClassificationResults(result, classifierResult)
		} else {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Custom classifier %s failed: %v", classifier.GetName(), err))
		}
	}
}

// mergeClassificationResults merges results from custom classifiers
func (s *Service) mergeClassificationResults(main *ClassificationResult, custom *ClassificationResult) {
	// Merge categories
	main.Categories = append(main.Categories, custom.Categories...)

	// Merge keywords (avoid duplicates)
	keywordMap := make(map[string]*Keyword)
	for i := range main.Keywords {
		keywordMap[main.Keywords[i].Text] = &main.Keywords[i]
	}

	for _, keyword := range custom.Keywords {
		if existing, exists := keywordMap[keyword.Text]; exists {
			// Update relevance if higher
			if keyword.Relevance > existing.Relevance {
				existing.Relevance = keyword.Relevance
			}
		} else {
			main.Keywords = append(main.Keywords, keyword)
		}
	}

	// Merge metadata
	for key, value := range custom.Metadata {
		main.Metadata[key] = value
	}
}

// calculateOverallConfidence calculates overall classification confidence
func (s *Service) calculateOverallConfidence(result *ClassificationResult) float64 {
	confidenceSum := 0.0
	componentCount := 0

	// Language confidence
	if result.LanguageConfidence > 0 {
		confidenceSum += result.LanguageConfidence
		componentCount++
	}

	// Sentiment confidence
	if result.Sentiment != nil && result.Sentiment.Confidence > 0 {
		confidenceSum += result.Sentiment.Confidence
		componentCount++
	}

	// Entity confidence (average)
	if len(result.Entities) > 0 {
		entityConfidenceSum := 0.0
		for _, entity := range result.Entities {
			entityConfidenceSum += entity.Confidence
		}
		confidenceSum += entityConfidenceSum / float64(len(result.Entities))
		componentCount++
	}

	// Keyword relevance (average of top keywords)
	if len(result.Keywords) > 0 {
		keywordSum := 0.0
		maxKeywords := 5 // Use top 5 keywords
		if len(result.Keywords) < maxKeywords {
			maxKeywords = len(result.Keywords)
		}
		for i := 0; i < maxKeywords; i++ {
			keywordSum += result.Keywords[i].Relevance
		}
		confidenceSum += keywordSum / float64(maxKeywords)
		componentCount++
	}

	if componentCount == 0 {
		return 0.0
	}

	return confidenceSum / float64(componentCount)
}

// validateInput validates the classification input
func (s *Service) validateInput(input *ClassificationInput) error {
	if input == nil {
		return fmt.Errorf("input cannot be nil")
	}

	if input.TenantID == uuid.Nil {
		return fmt.Errorf("tenant ID is required")
	}

	if input.Content == "" {
		return fmt.Errorf("content cannot be empty")
	}

	if len(input.Content) > s.config.MaxTextLength {
		return fmt.Errorf("content length %d exceeds maximum %d", len(input.Content), s.config.MaxTextLength)
	}

	if input.Options != nil {
		if input.Options.ConfidenceThreshold < 0 || input.Options.ConfidenceThreshold > 1 {
			return fmt.Errorf("confidence threshold must be between 0 and 1")
		}

		if input.Options.MaxKeywords < 0 || input.Options.MaxKeywords > 1000 {
			return fmt.Errorf("max keywords must be between 0 and 1000")
		}

		if input.Options.MaxEntities < 0 || input.Options.MaxEntities > 1000 {
			return fmt.Errorf("max entities must be between 0 and 1000")
		}
	}

	return nil
}

// updateMetrics updates service metrics
func (s *Service) updateMetrics(success bool, processingTime time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.TotalClassifications++

	if success {
		s.metrics.SuccessfulClassifications++
	} else {
		s.metrics.FailedClassifications++
	}

	// Update average processing time
	totalTime := s.metrics.AverageProcessingTime * time.Duration(s.metrics.TotalClassifications-1)
	s.metrics.AverageProcessingTime = (totalTime + processingTime) / time.Duration(s.metrics.TotalClassifications)

	// Update error rate
	s.metrics.ErrorRate = float64(s.metrics.FailedClassifications) / float64(s.metrics.TotalClassifications)

	// Update last classification time
	now := time.Now()
	s.metrics.LastClassificationAt = &now
}

// updateClassificationCounters updates classification type counters
func (s *Service) updateClassificationCounters(result *ClassificationResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.ClassificationsByType[result.ContentType]++
	s.metrics.ClassificationsByLanguage[result.Language]++
}

// GetMetrics returns classification metrics
func (s *Service) GetMetrics() *ClassificationMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create a copy to avoid race conditions
	metrics := &ClassificationMetrics{
		TotalClassifications:      s.metrics.TotalClassifications,
		SuccessfulClassifications: s.metrics.SuccessfulClassifications,
		FailedClassifications:     s.metrics.FailedClassifications,
		AverageProcessingTime:     s.metrics.AverageProcessingTime,
		LastClassificationAt:      s.metrics.LastClassificationAt,
		ErrorRate:                 s.metrics.ErrorRate,
		ThroughputPerSecond:       s.metrics.ThroughputPerSecond,
		ClassificationsByType:     make(map[ContentType]int64),
		ClassificationsByLanguage: make(map[Language]int64),
	}

	for k, v := range s.metrics.ClassificationsByType {
		metrics.ClassificationsByType[k] = v
	}

	for k, v := range s.metrics.ClassificationsByLanguage {
		metrics.ClassificationsByLanguage[k] = v
	}

	return metrics
}

// HealthCheck performs a service health check
func (s *Service) HealthCheck(ctx context.Context) error {
	// Check if any classifiers are registered
	s.mu.RLock()
	hasClassifiers := len(s.classifiers) > 0
	s.mu.RUnlock()

	if !hasClassifiers {
		return fmt.Errorf("no classifiers registered")
	}

	// Test with a simple classification
	testInput := &ClassificationInput{
		Content:  "This is a test document for health check.",
		TenantID: uuid.New(),
		Options:  DefaultClassificationOptions(),
	}

	_, err := s.Classify(ctx, testInput)
	if err != nil {
		return fmt.Errorf("health check classification failed: %w", err)
	}

	return nil
}
