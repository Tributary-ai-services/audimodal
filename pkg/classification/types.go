package classification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ContentType represents the type of content being classified
type ContentType string

const (
	ContentTypeDocument     ContentType = "document"
	ContentTypeSpreadsheet  ContentType = "spreadsheet"
	ContentTypePresentation ContentType = "presentation"
	ContentTypeImage        ContentType = "image"
	ContentTypeVideo        ContentType = "video"
	ContentTypeAudio        ContentType = "audio"
	ContentTypeCode         ContentType = "code"
	ContentTypeEmail        ContentType = "email"
	ContentTypeWeb          ContentType = "web"
	ContentTypeDatabase     ContentType = "database"
	ContentTypeLog          ContentType = "log"
	ContentTypeUnknown      ContentType = "unknown"
)

// DocumentType represents specific document types
type DocumentType string

const (
	DocumentTypePDF        DocumentType = "pdf"
	DocumentTypeWord       DocumentType = "word"
	DocumentTypeText       DocumentType = "text"
	DocumentTypeMarkdown   DocumentType = "markdown"
	DocumentTypeRTF        DocumentType = "rtf"
	DocumentTypeHTML       DocumentType = "html"
	DocumentTypeXML        DocumentType = "xml"
	DocumentTypeJSON       DocumentType = "json"
	DocumentTypeCSV        DocumentType = "csv"
	DocumentTypeExcel      DocumentType = "excel"
	DocumentTypePowerPoint DocumentType = "powerpoint"
)

// SensitivityLevel represents the data sensitivity classification
type SensitivityLevel string

const (
	SensitivityPublic       SensitivityLevel = "public"
	SensitivityInternal     SensitivityLevel = "internal"
	SensitivityConfidential SensitivityLevel = "confidential"
	SensitivityRestricted   SensitivityLevel = "restricted"
	SensitivityTopSecret    SensitivityLevel = "top_secret"
)

// ComplianceFramework represents regulatory compliance frameworks
type ComplianceFramework string

const (
	ComplianceGDPR     ComplianceFramework = "gdpr"
	ComplianceHIPAA    ComplianceFramework = "hipaa"
	CompliancePCI      ComplianceFramework = "pci"
	ComplianceSOX      ComplianceFramework = "sox"
	ComplianceISO27001 ComplianceFramework = "iso27001"
	ComplianceFISMA    ComplianceFramework = "fisma"
	ComplianceCCPA     ComplianceFramework = "ccpa"
)

// Language represents detected language
type Language string

const (
	LanguageEnglish    Language = "en"
	LanguageSpanish    Language = "es"
	LanguageFrench     Language = "fr"
	LanguageGerman     Language = "de"
	LanguageItalian    Language = "it"
	LanguagePortuguese Language = "pt"
	LanguageDutch      Language = "nl"
	LanguageRussian    Language = "ru"
	LanguageChinese    Language = "zh"
	LanguageJapanese   Language = "ja"
	LanguageKorean     Language = "ko"
	LanguageUnknown    Language = "unknown"
)

// ClassificationInput represents input for content classification
type ClassificationInput struct {
	Content     string            `json:"content"`
	URL         string            `json:"url,omitempty"`
	Filename    string            `json:"filename,omitempty"`
	MimeType    string            `json:"mime_type,omitempty"`
	Size        int64             `json:"size,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	TenantID    uuid.UUID         `json:"tenant_id"`
	FileID      *uuid.UUID        `json:"file_id,omitempty"`
	ChunkID     *uuid.UUID        `json:"chunk_id,omitempty"`
	Options     *ClassificationOptions `json:"options,omitempty"`
}

// ClassificationOptions contains configuration for classification
type ClassificationOptions struct {
	EnableLanguageDetection bool     `json:"enable_language_detection"`
	EnableSentimentAnalysis bool     `json:"enable_sentiment_analysis"`
	EnableEntityExtraction  bool     `json:"enable_entity_extraction"`
	EnableKeywordExtraction bool     `json:"enable_keyword_extraction"`
	EnableTopicModeling     bool     `json:"enable_topic_modeling"`
	EnableStructureAnalysis bool     `json:"enable_structure_analysis"`
	ConfidenceThreshold     float64  `json:"confidence_threshold"`
	MaxKeywords             int      `json:"max_keywords"`
	MaxEntities             int      `json:"max_entities"`
	MaxTopics               int      `json:"max_topics"`
	CustomLabels            []string `json:"custom_labels,omitempty"`
}

// DefaultClassificationOptions returns default classification options
func DefaultClassificationOptions() *ClassificationOptions {
	return &ClassificationOptions{
		EnableLanguageDetection: true,
		EnableSentimentAnalysis: true,
		EnableEntityExtraction:  true,
		EnableKeywordExtraction: true,
		EnableTopicModeling:     true,
		EnableStructureAnalysis: true,
		ConfidenceThreshold:     0.7,
		MaxKeywords:             20,
		MaxEntities:             50,
		MaxTopics:               10,
		CustomLabels:            []string{},
	}
}

// ClassificationResult represents the result of content classification
type ClassificationResult struct {
	ID                uuid.UUID               `json:"id"`
	TenantID          uuid.UUID               `json:"tenant_id"`
	FileID            *uuid.UUID              `json:"file_id,omitempty"`
	ChunkID           *uuid.UUID              `json:"chunk_id,omitempty"`
	
	// Content type classification
	ContentType       ContentType             `json:"content_type"`
	DocumentType      DocumentType            `json:"document_type"`
	MimeType          string                  `json:"mime_type"`
	
	// Language detection
	Language          Language                `json:"language"`
	LanguageConfidence float64                `json:"language_confidence"`
	
	// Sensitivity classification
	SensitivityLevel  SensitivityLevel        `json:"sensitivity_level"`
	SensitivityScore  float64                 `json:"sensitivity_score"`
	SensitivityReasons []string               `json:"sensitivity_reasons"`
	
	// Compliance analysis
	ComplianceFlags   []ComplianceFramework   `json:"compliance_flags"`
	ComplianceRisks   []ComplianceRisk        `json:"compliance_risks"`
	
	// Content analysis
	Sentiment         *SentimentAnalysis      `json:"sentiment,omitempty"`
	Keywords          []Keyword               `json:"keywords,omitempty"`
	Entities          []Entity                `json:"entities,omitempty"`
	Topics            []Topic                 `json:"topics,omitempty"`
	Categories        []Category              `json:"categories,omitempty"`
	
	// Processing metadata
	Confidence        float64                 `json:"confidence"`
	ProcessingTime    time.Duration           `json:"processing_time"`
	ModelVersion      string                  `json:"model_version"`
	ProcessedAt       time.Time               `json:"processed_at"`
	ProcessedBy       string                  `json:"processed_by"`
	
	// Additional metadata
	Metadata          map[string]interface{}  `json:"metadata,omitempty"`
	Warnings          []string                `json:"warnings,omitempty"`
	Errors            []string                `json:"errors,omitempty"`
}

// SentimentAnalysis represents sentiment analysis results
type SentimentAnalysis struct {
	Overall    SentimentScore `json:"overall"`
	Sentences  []SentimentScore `json:"sentences,omitempty"`
	Confidence float64        `json:"confidence"`
}

// SentimentScore represents a sentiment score
type SentimentScore struct {
	Label      string  `json:"label"`      // positive, negative, neutral
	Score      float64 `json:"score"`      // -1.0 to 1.0
	Magnitude  float64 `json:"magnitude"`  // 0.0 to 1.0 (intensity)
}

// Keyword represents an extracted keyword
type Keyword struct {
	Text       string  `json:"text"`
	Relevance  float64 `json:"relevance"`
	Frequency  int     `json:"frequency"`
	TfIdf      float64 `json:"tf_idf,omitempty"`
	Position   []int   `json:"position,omitempty"` // Character positions
}

// Entity represents an extracted entity
type Entity struct {
	Text       string             `json:"text"`
	Type       EntityType         `json:"type"`
	SubType    string             `json:"sub_type,omitempty"`
	Confidence float64            `json:"confidence"`
	Positions  []EntityPosition   `json:"positions"`
	Metadata   map[string]string  `json:"metadata,omitempty"`
}

// EntityType represents the type of entity
type EntityType string

const (
	EntityTypePerson       EntityType = "person"
	EntityTypeOrganization EntityType = "organization"
	EntityTypeLocation     EntityType = "location"
	EntityTypeDate         EntityType = "date"
	EntityTypeMoney        EntityType = "money"
	EntityTypePercentage   EntityType = "percentage"
	EntityTypePhone        EntityType = "phone"
	EntityTypeEmail        EntityType = "email"
	EntityTypeURL          EntityType = "url"
	EntityTypeIPAddress    EntityType = "ip_address"
	EntityTypeCreditCard   EntityType = "credit_card"
	EntityTypeSSN          EntityType = "ssn"
	EntityTypeProduct      EntityType = "product"
	EntityTypeEvent        EntityType = "event"
	EntityTypeOther        EntityType = "other"
)

// EntityPosition represents the position of an entity in text
type EntityPosition struct {
	Start  int `json:"start"`
	End    int `json:"end"`
	Line   int `json:"line,omitempty"`
	Column int `json:"column,omitempty"`
}

// Topic represents a detected topic
type Topic struct {
	Name       string    `json:"name"`
	Keywords   []string  `json:"keywords"`
	Relevance  float64   `json:"relevance"`
	Confidence float64   `json:"confidence"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// Category represents a content category
type Category struct {
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
	Hierarchy  []string `json:"hierarchy,omitempty"` // Parent categories
	Tags       []string `json:"tags,omitempty"`
}

// ComplianceRisk represents a compliance risk
type ComplianceRisk struct {
	Framework   ComplianceFramework `json:"framework"`
	RiskLevel   string              `json:"risk_level"` // low, medium, high, critical
	Description string              `json:"description"`
	Mitigation  []string            `json:"mitigation,omitempty"`
	References  []string            `json:"references,omitempty"`
}

// Classifier defines the interface for content classifiers
type Classifier interface {
	// Classify performs content classification
	Classify(ctx context.Context, input *ClassificationInput) (*ClassificationResult, error)
	
	// GetName returns the classifier name
	GetName() string
	
	// GetVersion returns the classifier version
	GetVersion() string
	
	// GetSupportedTypes returns supported content types
	GetSupportedTypes() []ContentType
	
	// HealthCheck performs a health check
	HealthCheck(ctx context.Context) error
}

// LanguageDetector defines the interface for language detection
type LanguageDetector interface {
	// DetectLanguage detects the language of text
	DetectLanguage(ctx context.Context, text string) (*LanguageResult, error)
	
	// GetSupportedLanguages returns supported languages
	GetSupportedLanguages() []Language
}

// LanguageResult represents language detection results
type LanguageResult struct {
	Language   Language `json:"language"`
	Confidence float64  `json:"confidence"`
	Candidates []LanguageCandidate `json:"candidates,omitempty"`
}

// LanguageCandidate represents a language detection candidate
type LanguageCandidate struct {
	Language   Language `json:"language"`
	Confidence float64  `json:"confidence"`
}

// SentimentAnalyzer defines the interface for sentiment analysis
type SentimentAnalyzer interface {
	// AnalyzeSentiment analyzes sentiment of text
	AnalyzeSentiment(ctx context.Context, text string) (*SentimentAnalysis, error)
	
	// GetSupportedLanguages returns supported languages
	GetSupportedLanguages() []Language
}

// EntityExtractor defines the interface for entity extraction
type EntityExtractor interface {
	// ExtractEntities extracts entities from text
	ExtractEntities(ctx context.Context, text string) ([]Entity, error)
	
	// GetSupportedEntityTypes returns supported entity types
	GetSupportedEntityTypes() []EntityType
}

// KeywordExtractor defines the interface for keyword extraction
type KeywordExtractor interface {
	// ExtractKeywords extracts keywords from text
	ExtractKeywords(ctx context.Context, text string, maxKeywords int) ([]Keyword, error)
	
	// GetAlgorithm returns the extraction algorithm name
	GetAlgorithm() string
}

// TopicModeler defines the interface for topic modeling
type TopicModeler interface {
	// ExtractTopics extracts topics from text
	ExtractTopics(ctx context.Context, text string, maxTopics int) ([]Topic, error)
	
	// GetModelName returns the model name
	GetModelName() string
}

// ClassificationService defines the main classification service interface
type ClassificationService interface {
	// Classify performs comprehensive content classification
	Classify(ctx context.Context, input *ClassificationInput) (*ClassificationResult, error)
	
	// ClassifyBatch performs batch classification
	ClassifyBatch(ctx context.Context, inputs []*ClassificationInput) ([]*ClassificationResult, error)
	
	// RegisterClassifier registers a custom classifier
	RegisterClassifier(classifier Classifier) error
	
	// GetClassifiers returns all registered classifiers
	GetClassifiers() []Classifier
	
	// GetMetrics returns classification metrics
	GetMetrics() *ClassificationMetrics
	
	// HealthCheck performs a service health check
	HealthCheck(ctx context.Context) error
}

// ClassificationMetrics represents classification performance metrics
type ClassificationMetrics struct {
	TotalClassifications  int64                    `json:"total_classifications"`
	SuccessfulClassifications int64                `json:"successful_classifications"`
	FailedClassifications int64                    `json:"failed_classifications"`
	AverageProcessingTime time.Duration            `json:"average_processing_time"`
	ClassificationsByType map[ContentType]int64    `json:"classifications_by_type"`
	ClassificationsByLanguage map[Language]int64   `json:"classifications_by_language"`
	LastClassificationAt  *time.Time               `json:"last_classification_at"`
	ErrorRate            float64                   `json:"error_rate"`
	ThroughputPerSecond  float64                   `json:"throughput_per_second"`
}

// ClassificationEvent represents a classification event for the event system
type ClassificationEvent struct {
	ID           uuid.UUID              `json:"id"`
	Type         string                 `json:"type"`
	TenantID     uuid.UUID              `json:"tenant_id"`
	FileID       *uuid.UUID             `json:"file_id,omitempty"`
	ChunkID      *uuid.UUID             `json:"chunk_id,omitempty"`
	Result       *ClassificationResult  `json:"result"`
	ProcessingTime time.Duration        `json:"processing_time"`
	Timestamp    time.Time              `json:"timestamp"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ClassificationError represents classification errors
type ClassificationError struct {
	Code        string    `json:"code"`
	Message     string    `json:"message"`
	Component   string    `json:"component"`
	Retryable   bool      `json:"retryable"`
	Timestamp   time.Time `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Error codes for classification
const (
	ErrorCodeInvalidInput     = "INVALID_INPUT"
	ErrorCodeProcessingFailed = "PROCESSING_FAILED"
	ErrorCodeModelError       = "MODEL_ERROR"
	ErrorCodeTimeout          = "TIMEOUT"
	ErrorCodeQuotaExceeded    = "QUOTA_EXCEEDED"
	ErrorCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
)

// NewClassificationError creates a new classification error
func NewClassificationError(code, message, component string, retryable bool) *ClassificationError {
	return &ClassificationError{
		Code:      code,
		Message:   message,
		Component: component,
		Retryable: retryable,
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
}

// Error implements the error interface
func (e *ClassificationError) Error() string {
	return fmt.Sprintf("[%s] %s: %s", e.Component, e.Code, e.Message)
}

// NewClassificationResult creates a new classification result
func NewClassificationResult(tenantID uuid.UUID) *ClassificationResult {
	return &ClassificationResult{
		ID:                 uuid.New(),
		TenantID:           tenantID,
		SensitivityReasons: []string{},
		ComplianceFlags:    []ComplianceFramework{},
		ComplianceRisks:    []ComplianceRisk{},
		Keywords:           []Keyword{},
		Entities:           []Entity{},
		Topics:             []Topic{},
		Categories:         []Category{},
		Metadata:           make(map[string]interface{}),
		Warnings:           []string{},
		Errors:             []string{},
		ProcessedAt:        time.Now(),
	}
}