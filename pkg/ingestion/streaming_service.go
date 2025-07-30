package ingestion

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/internal/processors"
	"github.com/jscharber/eAIIngest/pkg/events"
)

// StreamingIngestionService handles real-time document processing via Kafka streams
type StreamingIngestionService struct {
	consumer            *events.Consumer
	producer            *events.Producer
	embeddingCoordinator *processors.EmbeddingCoordinator
	db                  *database.Database
	tracer              trace.Tracer
	config              *StreamingConfig
}

// StreamingConfig contains configuration for streaming ingestion
type StreamingConfig struct {
	// Kafka configuration
	KafkaConsumerGroup string `yaml:"kafka_consumer_group"`
	KafkaTopics        []string `yaml:"kafka_topics"`
	
	// Processing configuration
	EnableEmbeddings    bool   `yaml:"enable_embeddings"`
	DefaultDataset      string `yaml:"default_dataset"`
	ProcessingTimeout   time.Duration `yaml:"processing_timeout"`
	MaxConcurrentFiles  int    `yaml:"max_concurrent_files"`
	
	// Error handling
	MaxRetries          int           `yaml:"max_retries"`
	RetryDelay          time.Duration `yaml:"retry_delay"`
	DeadLetterTopic     string        `yaml:"dead_letter_topic"`
}

// DefaultStreamingConfig returns default configuration for streaming ingestion
func DefaultStreamingConfig() *StreamingConfig {
	return &StreamingConfig{
		KafkaConsumerGroup: "document-processing-stream",
		KafkaTopics: []string{
			"file.discovery",
			"file.upload",
			"external.datasource.changes",
		},
		EnableEmbeddings:   true,
		DefaultDataset:     "streaming",
		ProcessingTimeout:  10 * time.Minute,
		MaxConcurrentFiles: 10,
		MaxRetries:         3,
		RetryDelay:         5 * time.Second,
		DeadLetterTopic:    "document-processing-dlq",
	}
}

// NewStreamingIngestionService creates a new streaming ingestion service
func NewStreamingIngestionService(
	db *database.Database,
	embeddingCoordinator *processors.EmbeddingCoordinator,
	consumerConfig events.ConsumerConfig,
	producerConfig events.ProducerConfig,
	streamingConfig *StreamingConfig,
) (*StreamingIngestionService, error) {
	
	if streamingConfig == nil {
		streamingConfig = DefaultStreamingConfig()
	}

	// Create producer for outbound events
	producer, err := events.NewProducer(producerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create event producer: %w", err)
	}

	// Create error handler for consumer
	errorHandler := events.NewDefaultErrorHandler(producer, streamingConfig.MaxRetries, streamingConfig.DeadLetterTopic)

	// Create consumer
	consumer, err := events.NewConsumer(consumerConfig, errorHandler)
	if err != nil {
		return nil, fmt.Errorf("failed to create event consumer: %w", err)
	}

	service := &StreamingIngestionService{
		consumer:            consumer,
		producer:            producer,
		embeddingCoordinator: embeddingCoordinator,
		db:                  db,
		tracer:              otel.Tracer("streaming-ingestion"),
		config:              streamingConfig,
	}

	// Register event handlers
	service.registerHandlers()

	return service, nil
}

// registerHandlers registers all event handlers for the streaming service
func (s *StreamingIngestionService) registerHandlers() {
	// Register file discovery handler
	s.consumer.RegisterHandler(&FileDiscoveryHandler{
		service: s,
	})

	// Register file upload handler
	s.consumer.RegisterHandler(&FileUploadHandler{
		service: s,
	})

	// Register external datasource change handler
	s.consumer.RegisterHandler(&ExternalDataSourceHandler{
		service: s,
	})
}

// Start starts the streaming ingestion service
func (s *StreamingIngestionService) Start(ctx context.Context) error {
	// Subscribe to topics
	if err := s.consumer.Subscribe(s.config.KafkaTopics); err != nil {
		return fmt.Errorf("failed to subscribe to topics: %w", err)
	}

	// Start consumer
	return s.consumer.Start(ctx)
}

// Stop stops the streaming ingestion service
func (s *StreamingIngestionService) Stop() error {
	if err := s.consumer.Stop(); err != nil {
		return fmt.Errorf("failed to stop consumer: %w", err)
	}

	if err := s.producer.Close(); err != nil {
		return fmt.Errorf("failed to close producer: %w", err)
	}

	return nil
}

// FileDiscoveryHandler handles file discovery events
type FileDiscoveryHandler struct {
	service *StreamingIngestionService
}

func (h *FileDiscoveryHandler) GetEventTypes() []string {
	return []string{events.EventFileDiscovered}
}

func (h *FileDiscoveryHandler) GetName() string {
	return "file-discovery-handler"
}

func (h *FileDiscoveryHandler) HandleEvent(ctx context.Context, event interface{}) error {
	ctx, span := h.service.tracer.Start(ctx, "handle_file_discovery")
	defer span.End()

	fileEvent, ok := event.(*events.FileDiscoveredEvent)
	if !ok {
		return fmt.Errorf("invalid event type for file discovery handler")
	}

	span.SetAttributes(
		attribute.String("file.url", fileEvent.Data.URL),
		attribute.String("tenant.id", fileEvent.TenantID),
		attribute.String("source.id", fileEvent.Data.SourceID),
		attribute.Int64("file.size", fileEvent.Data.Size),
	)

	return h.processDiscoveredFile(ctx, fileEvent)
}

func (h *FileDiscoveryHandler) processDiscoveredFile(ctx context.Context, fileEvent *events.FileDiscoveredEvent) error {
	// Convert tenant ID to UUID
	tenantID, err := uuid.Parse(fileEvent.TenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant ID: %w", err)
	}

	// Create a new file ID for tracking
	fileID := uuid.New()

	// Process file through embedding coordinator
	options := map[string]any{
		"source_id":            fileEvent.Data.SourceID,
		"discovered_at":        fileEvent.Data.DiscoveredAt,
		"priority":             fileEvent.Data.Priority,
		"embeddings_enabled":   h.service.config.EnableEmbeddings,
		"dataset":              h.service.config.DefaultDataset,
		"content_type":         fileEvent.Data.ContentType,
		"metadata":             fileEvent.Data.Metadata,
		"streaming_mode":       true,
	}

	// Process the file
	result, err := h.service.embeddingCoordinator.ProcessSingleFileWithEmbeddings(ctx, tenantID, fileID, options)
	if err != nil {
		// Publish processing failed event
		failedEvent := events.NewProcessingFailedEvent("streaming-ingestion", fileEvent.TenantID, events.ProcessingFailedData{
			URL:          fileEvent.Data.URL,
			FailureStage: "processing",
			ErrorCode:    "PROCESSING_ERROR",
			ErrorMessage: err.Error(),
			RetryCount:   0,
			Retryable:    true,
			Context: map[string]interface{}{
				"file_id":   fileID.String(),
				"source_id": fileEvent.Data.SourceID,
			},
		})

		if publishErr := h.service.producer.PublishEvent(ctx, failedEvent); publishErr != nil {
			return fmt.Errorf("failed to publish processing failed event: %w (original error: %v)", publishErr, err)
		}

		return err
	}

	// Publish processing complete event
	completeEvent := events.NewProcessingCompleteEvent("streaming-ingestion", fileEvent.TenantID, events.ProcessingCompleteData{
		URL:                 fileEvent.Data.URL,
		TotalProcessingTime: result.ProcessingResult.ProcessingTime,
		ChunksCreated:       result.ProcessingResult.ChunksCreated,
		EmbeddingsCreated:   result.EmbeddingsCreated,
		DLPViolationsFound:  0, // TODO: Extract from result
		FinalDataClass:      "processed",
		StorageLocation:     result.ProcessingResult.OutputPath,
		Success:             true,
	})

	return h.service.producer.PublishEvent(ctx, completeEvent)
}

// FileUploadHandler handles direct file upload events
type FileUploadHandler struct {
	service *StreamingIngestionService
}

func (h *FileUploadHandler) GetEventTypes() []string {
	return []string{"file.uploaded"} // Custom event type for uploads
}

func (h *FileUploadHandler) GetName() string {
	return "file-upload-handler"
}

func (h *FileUploadHandler) HandleEvent(ctx context.Context, event interface{}) error {
	ctx, span := h.service.tracer.Start(ctx, "handle_file_upload")
	defer span.End()

	// Similar processing to file discovery but for direct uploads
	// Implementation would depend on the specific upload event structure
	return nil
}

// ExternalDataSourceHandler handles changes from external data sources
type ExternalDataSourceHandler struct {
	service *StreamingIngestionService
}

func (h *ExternalDataSourceHandler) GetEventTypes() []string {
	return []string{
		"sharepoint.file.added",
		"sharepoint.file.modified",
		"gdrive.file.added",
		"gdrive.file.modified",
		"box.file.added",
		"onedrive.file.added",
	}
}

func (h *ExternalDataSourceHandler) GetName() string {
	return "external-datasource-handler"
}

func (h *ExternalDataSourceHandler) HandleEvent(ctx context.Context, event interface{}) error {
	ctx, span := h.service.tracer.Start(ctx, "handle_external_datasource")
	defer span.End()

	// Convert external data source events to internal file discovered events
	// and process them through the standard pipeline
	return nil
}

// ProcessingMetrics contains metrics for the streaming ingestion service
type ProcessingMetrics struct {
	FilesProcessed      int64         `json:"files_processed"`
	FilesSucceeded      int64         `json:"files_succeeded"`
	FilesFailed         int64         `json:"files_failed"`
	EmbeddingsCreated   int64         `json:"embeddings_created"`
	AvgProcessingTime   time.Duration `json:"avg_processing_time"`
	ThroughputPerMinute float64       `json:"throughput_per_minute"`
	LastProcessedAt     *time.Time    `json:"last_processed_at"`
}

// GetMetrics returns processing metrics
func (s *StreamingIngestionService) GetMetrics() (*ProcessingMetrics, error) {
	// In a real implementation, this would query actual metrics from database or monitoring
	return &ProcessingMetrics{
		FilesProcessed:      100,
		FilesSucceeded:      95,
		FilesFailed:         5,
		EmbeddingsCreated:   2000,
		AvgProcessingTime:   2 * time.Minute,
		ThroughputPerMinute: 10.5,
		LastProcessedAt:     &[]time.Time{time.Now()}[0],
	}, nil
}

// HealthCheck performs a health check on the streaming service
func (s *StreamingIngestionService) HealthCheck(ctx context.Context) error {
	// Check consumer health
	if err := s.consumer.HealthCheck(ctx); err != nil {
		return fmt.Errorf("consumer health check failed: %w", err)
	}

	// Check producer health
	if err := s.producer.HealthCheck(ctx); err != nil {
		return fmt.Errorf("producer health check failed: %w", err)
	}

	// Check embedding coordinator health
	if err := s.embeddingCoordinator.ValidateEmbeddingConfiguration(ctx); err != nil {
		return fmt.Errorf("embedding coordinator health check failed: %w", err)
	}

	return nil
}

// CreateStreamSession creates a new streaming session for bulk processing
func (s *StreamingIngestionService) CreateStreamSession(ctx context.Context, tenantID uuid.UUID, sessionName string) (*processors.SessionContext, error) {
	sessionConfig := &processors.SessionConfig{
		Priority:           "normal",
		MaxConcurrentFiles: s.config.MaxConcurrentFiles,
		RetryEnabled:       true,
		RetryAttempts:      s.config.MaxRetries,
		DLPScanEnabled:     true,
		ProcessingOptions: map[string]any{
			"embeddings_enabled": s.config.EnableEmbeddings,
			"dataset":           s.config.DefaultDataset,
			"streaming_mode":    true,
		},
	}

	// Create session through the coordinator's session manager
	// This would require access to the session manager from the coordinator
	// For now, return a placeholder implementation
	return &processors.SessionContext{
		SessionID: uuid.New(),
		TenantID:  tenantID,
		Status:    "created",
		StartedAt: time.Now(),
		Config:    sessionConfig,
	}, nil
}