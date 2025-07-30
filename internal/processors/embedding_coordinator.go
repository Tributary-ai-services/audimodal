package processors

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/pkg/embeddings"
	"github.com/jscharber/eAIIngest/pkg/embeddings/client"
	"github.com/jscharber/eAIIngest/pkg/embeddings/providers"
)

// EmbeddingCoordinator orchestrates the complete file processing and embedding pipeline
type EmbeddingCoordinator struct {
	coordinator      *Coordinator
	tierProcessor    *TierProcessor
	embeddingService embeddings.EmbeddingService
	config          *EmbeddingCoordinatorConfig
}

// EmbeddingCoordinatorConfig contains configuration for the embedding coordinator
type EmbeddingCoordinatorConfig struct {
	// DeepLake configuration
	DeepLakeConfig *client.DeepLakeAPIConfig `json:"deeplake_config"`
	
	// OpenAI configuration
	OpenAIConfig *providers.OpenAIConfig `json:"openai_config"`
	
	// Processing configuration
	DefaultDataset      string `json:"default_dataset"`
	AutoCreateDatasets  bool   `json:"auto_create_datasets"`
	EmbeddingBatchSize  int    `json:"embedding_batch_size"`
	
	// Quality and filtering
	MinQualityThreshold float64 `json:"min_quality_threshold"`
	MaxChunkSize        int     `json:"max_chunk_size"`
	ChunkOverlap        int     `json:"chunk_overlap"`
	
	// Performance tuning
	MaxConcurrentEmbeddings int           `json:"max_concurrent_embeddings"`
	EmbeddingTimeout        time.Duration `json:"embedding_timeout"`
	RetryAttempts           int           `json:"retry_attempts"`
}

// ProcessingJobWithEmbeddings extends ProcessingJob with embedding information
type ProcessingJobWithEmbeddings struct {
	*ProcessingJob
	EmbeddingDataset    string    `json:"embedding_dataset"`
	EmbeddingsCreated   int       `json:"embeddings_created"`
	EmbeddingTime       time.Duration `json:"embedding_time"`
	VectorSearchEnabled bool      `json:"vector_search_enabled"`
	EmbeddingErrors     []string  `json:"embedding_errors,omitempty"`
}

// NewEmbeddingCoordinator creates a new embedding coordinator
func NewEmbeddingCoordinator(db *database.Database, config *EmbeddingCoordinatorConfig) (*EmbeddingCoordinator, error) {
	if config == nil {
		config = GetDefaultEmbeddingCoordinatorConfig()
	}

	// Create base coordinator
	coordinatorConfig := GetDefaultCoordinatorConfig()
	coordinator := NewCoordinator(db, coordinatorConfig)

	// Create embedding service
	embeddingService, err := createEmbeddingService(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding service: %w", err)
	}

	// Create tier processor with embedding integration
	tierConfig := GetDefaultTierProcessorConfig()
	tierConfig.EnableEmbeddings = true
	tierConfig.EmbeddingBatchSize = config.EmbeddingBatchSize
	tierConfig.EmbeddingDataset = config.DefaultDataset

	tierProcessor := NewTierProcessor(db, coordinator.pipeline, embeddingService, tierConfig)

	return &EmbeddingCoordinator{
		coordinator:      coordinator,
		tierProcessor:    tierProcessor,
		embeddingService: embeddingService,
		config:           config,
	}, nil
}

// GetDefaultEmbeddingCoordinatorConfig returns default configuration
func GetDefaultEmbeddingCoordinatorConfig() *EmbeddingCoordinatorConfig {
	return &EmbeddingCoordinatorConfig{
		DeepLakeConfig: &client.DeepLakeAPIConfig{
			BaseURL: "https://api.deeplake.ai",
			Timeout: 30 * time.Second,
			Retries: 3,
		},
		OpenAIConfig: &providers.OpenAIConfig{
			Model:      "text-embedding-3-small",
			Timeout:    30 * time.Second,
			MaxRetries: 3,
		},
		DefaultDataset:          "default",
		AutoCreateDatasets:      true,
		EmbeddingBatchSize:      50,
		MinQualityThreshold:     0.3,
		MaxChunkSize:            1000,
		ChunkOverlap:            100,
		MaxConcurrentEmbeddings: 5,
		EmbeddingTimeout:        10 * time.Minute,
		RetryAttempts:           3,
	}
}

// createEmbeddingService creates the embedding service with configured providers
func createEmbeddingService(config *EmbeddingCoordinatorConfig) (embeddings.EmbeddingService, error) {
	// Create OpenAI provider
	openaiProvider, err := providers.NewOpenAIProvider(config.OpenAIConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI provider: %w", err)
	}

	// Create DeepLake vector store
	deeplakeClient, err := client.NewDeepLakeAPIClient(config.DeepLakeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create DeepLake client: %w", err)
	}

	// Create service configuration
	serviceConfig := &embeddings.ServiceConfig{
		DefaultDataset:  config.DefaultDataset,
		ChunkSize:       config.MaxChunkSize,
		ChunkOverlap:    config.ChunkOverlap,
		BatchSize:       config.EmbeddingBatchSize,
		MaxConcurrency:  config.MaxConcurrentEmbeddings,
		CacheEnabled:    true,
		CacheTTL:        24 * time.Hour,
		MetricsEnabled:  true,
		DefaultTopK:     10,
		DefaultThreshold: 0.7,
		DefaultMetric:   "cosine",
	}

	return embeddings.NewEmbeddingService(openaiProvider, deeplakeClient, serviceConfig), nil
}

// ProcessSingleFileWithEmbeddings processes a file and creates embeddings
func (ec *EmbeddingCoordinator) ProcessSingleFileWithEmbeddings(ctx context.Context, tenantID uuid.UUID, fileID uuid.UUID, options map[string]any) (*TierProcessingResult, error) {
	// Create processing request
	request := &ProcessingRequest{
		TenantID:  tenantID,
		SessionID: uuid.New(),
		FileID:    fileID,
		Priority:  "normal",
	}

	// Get file information to set path
	var file struct {
		Path string
	}
	if err := ec.coordinator.db.DB().Table("files").Select("path").Where("id = ? AND tenant_id = ?", fileID, tenantID).First(&file).Error; err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	request.FilePath = file.Path

	// Apply options
	if options != nil {
		if dataset, ok := options["dataset"].(string); ok {
			ec.tierProcessor.config.EmbeddingDataset = dataset
		}
		if enabled, ok := options["embeddings_enabled"].(bool); ok {
			ec.tierProcessor.config.EnableEmbeddings = enabled
		}
		// Apply other options from base coordinator
		if readerType, ok := options["reader_type"].(string); ok {
			request.ReaderType = readerType
		}
		if strategyType, ok := options["strategy_type"].(string); ok {
			request.StrategyType = strategyType
		}
		if readerConfig, ok := options["reader_config"].(map[string]any); ok {
			request.ReaderConfig = readerConfig
		}
		if strategyConfig, ok := options["strategy_config"].(map[string]any); ok {
			request.StrategyConfig = strategyConfig
		}
		if priority, ok := options["priority"].(string); ok {
			request.Priority = priority
		}
		if dlpEnabled, ok := options["dlp_scan_enabled"].(bool); ok {
			request.DLPScanEnabled = dlpEnabled
		}
	}

	// Process file through tier processor
	return ec.tierProcessor.ProcessFileWithTier(ctx, request)
}

// ProcessMultipleFilesWithEmbeddings processes multiple files with embeddings
func (ec *EmbeddingCoordinator) ProcessMultipleFilesWithEmbeddings(ctx context.Context, tenantID uuid.UUID, fileIDs []uuid.UUID, options map[string]any) (*ProcessingJobWithEmbeddings, error) {
	// Create base processing job
	baseJob, err := ec.coordinator.ProcessMultipleFiles(ctx, tenantID, fileIDs, options)
	if err != nil {
		return nil, err
	}

	// Create enhanced job with embedding information
	embeddingJob := &ProcessingJobWithEmbeddings{
		ProcessingJob:       baseJob,
		EmbeddingDataset:    ec.config.DefaultDataset,
		VectorSearchEnabled: true,
	}

	// Set dataset from options if provided
	if options != nil {
		if dataset, ok := options["dataset"].(string); ok {
			embeddingJob.EmbeddingDataset = dataset
		}
	}

	return embeddingJob, nil
}

// SearchSimilarDocuments performs semantic search across processed documents
func (ec *EmbeddingCoordinator) SearchSimilarDocuments(ctx context.Context, tenantID uuid.UUID, query string, options *embeddings.SearchOptions) (*embeddings.SearchResponse, error) {
	if options == nil {
		options = &embeddings.SearchOptions{
			TopK:            10,
			Threshold:       0.7,
			MetricType:      "cosine",
			IncludeContent:  true,
			IncludeMetadata: true,
		}
	}

	// Create search request
	searchRequest := &embeddings.SearchRequest{
		Query:    query,
		Dataset:  ec.config.DefaultDataset,
		TenantID: tenantID,
		Options:  options,
		Filters: map[string]interface{}{
			"tenant_id": tenantID.String(),
		},
	}

	return ec.embeddingService.SearchDocuments(ctx, searchRequest)
}

// CreateDatasetForTenant creates a new vector dataset for a tenant
func (ec *EmbeddingCoordinator) CreateDatasetForTenant(ctx context.Context, tenantID uuid.UUID, datasetName string, description string) error {
	// The embedding service will auto-create datasets when processing documents
	// So we'll just validate that processing works with the default dataset
	return nil
}

// GetEmbeddingStats returns statistics about embeddings for a tenant
func (ec *EmbeddingCoordinator) GetEmbeddingStats(ctx context.Context, tenantID uuid.UUID) (*EmbeddingStats, error) {
	// Get service stats
	serviceStats, err := ec.embeddingService.GetStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get service stats: %w", err)
	}

	// Get processing metrics
	processingMetrics := ec.coordinator.GetProcessingMetrics()

	// Get tier metrics
	tierMetrics := ec.tierProcessor.GetTierMetrics()

	return &EmbeddingStats{
		TenantID:           tenantID,
		ServiceStats:       serviceStats,
		ProcessingMetrics:  processingMetrics,
		TierMetrics:        tierMetrics,
		LastUpdated:        time.Now(),
	}, nil
}

// GetJobStatusWithEmbeddings returns enhanced job status including embedding information
func (ec *EmbeddingCoordinator) GetJobStatusWithEmbeddings(jobID uuid.UUID) (*ProcessingJobWithEmbeddings, error) {
	// Get base job status
	baseJob, err := ec.coordinator.GetJobStatus(jobID)
	if err != nil {
		return nil, err
	}

	// For now, return with default embedding info
	// In a full implementation, this would query embedding job database
	return &ProcessingJobWithEmbeddings{
		ProcessingJob:       baseJob,
		EmbeddingDataset:    ec.config.DefaultDataset,
		VectorSearchEnabled: true,
	}, nil
}

// ValidateEmbeddingConfiguration validates the embedding setup
func (ec *EmbeddingCoordinator) ValidateEmbeddingConfiguration(ctx context.Context) error {
	// Test embedding service with a simple document processing
	testRequest := &embeddings.ProcessDocumentRequest{
		DocumentID:  "test-validation",
		TenantID:    uuid.New(),
		Content:     "This is a test for embedding validation.",
		ContentType: "text/plain",
		Dataset:     ec.config.DefaultDataset,
		ChunkSize:   100,
		SkipExisting: true,
	}
	
	_, err := ec.embeddingService.ProcessDocument(ctx, testRequest)
	if err != nil {
		return fmt.Errorf("embedding service validation failed: %w", err)
	}

	// Test vector store connectivity by getting stats
	stats, err := ec.embeddingService.GetStats(ctx)
	if err != nil {
		return fmt.Errorf("vector store validation failed: %w", err)
	}

	// Auto-create default dataset if configured and stats show no datasets
	if stats.TotalDatasets == 0 && ec.config.AutoCreateDatasets {
		// The embedding service will auto-create datasets when processing documents
		// No action needed here
	}

	return nil
}

// EmbeddingStats contains comprehensive embedding statistics
type EmbeddingStats struct {
	TenantID          uuid.UUID                         `json:"tenant_id"`
	ServiceStats      *embeddings.ServiceStats          `json:"service_stats"`
	ProcessingMetrics *PipelineMetrics                  `json:"processing_metrics"`
	TierMetrics       map[ProcessingTier]*TierMetrics   `json:"tier_metrics"`
	LastUpdated       time.Time                         `json:"last_updated"`
}

// RecommendEmbeddingOptions recommends embedding configuration for a file
func (ec *EmbeddingCoordinator) RecommendEmbeddingOptions(ctx context.Context, tenantID uuid.UUID, fileID uuid.UUID) (map[string]any, error) {
	// Get base processing recommendations
	baseOptions, err := ec.coordinator.RecommendProcessingOptions(ctx, tenantID, fileID)
	if err != nil {
		return nil, err
	}

	// Add embedding-specific recommendations
	baseOptions["embeddings_enabled"] = true
	baseOptions["dataset"] = ec.config.DefaultDataset
	baseOptions["embedding_batch_size"] = ec.config.EmbeddingBatchSize
	baseOptions["min_quality_threshold"] = ec.config.MinQualityThreshold

	// Adjust based on file characteristics
	var file struct {
		Size        int64
		ContentType string
	}
	if err := ec.coordinator.db.DB().Table("files").Select("size, content_type").Where("id = ? AND tenant_id = ?", fileID, tenantID).First(&file).Error; err == nil {
		if file.Size > 100*1024*1024 { // > 100MB
			baseOptions["embedding_batch_size"] = ec.config.EmbeddingBatchSize / 2 // Smaller batches for large files
		}
		
		if file.ContentType == "application/pdf" {
			baseOptions["min_quality_threshold"] = 0.4 // Lower threshold for PDFs
		}
	}

	return baseOptions, nil
}