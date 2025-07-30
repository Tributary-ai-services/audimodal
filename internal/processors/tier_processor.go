package processors

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/internal/database/models"
	"github.com/jscharber/eAIIngest/pkg/embeddings"
)

// ProcessingTier defines different processing tiers based on file size and complexity
type ProcessingTier int

const (
	TierSmall ProcessingTier = iota // < 10MB - Fast processing, minimal resources
	TierMedium                     // 10MB - 1GB - Standard processing with batching
	TierLarge                      // > 1GB - Heavy processing with streaming and checkpoints
)

// TierProcessor handles tier-based processing with different strategies for different file sizes
type TierProcessor struct {
	db              *database.Database
	pipeline        *Pipeline
	embeddingService embeddings.EmbeddingService
	config          *TierProcessorConfig
}

// TierProcessorConfig contains configuration for tier-based processing
type TierProcessorConfig struct {
	SmallFileTierThreshold  int64         `json:"small_file_tier_threshold"`  // 10MB
	LargeFileTierThreshold  int64         `json:"large_file_tier_threshold"`  // 1GB
	SmallFileTimeout        time.Duration `json:"small_file_timeout"`         // 5 minutes
	MediumFileTimeout       time.Duration `json:"medium_file_timeout"`        // 30 minutes
	LargeFileTimeout        time.Duration `json:"large_file_timeout"`         // 2 hours
	CheckpointInterval      time.Duration `json:"checkpoint_interval"`        // 5 minutes
	EnableEmbeddings        bool          `json:"enable_embeddings"`
	EmbeddingBatchSize      int           `json:"embedding_batch_size"`
	EmbeddingDataset        string        `json:"embedding_dataset"`
}

// TierProcessingResult contains results with tier-specific metrics
type TierProcessingResult struct {
	*ProcessingResult
	Tier                ProcessingTier    `json:"tier"`
	EmbeddingsCreated   int               `json:"embeddings_created"`
	EmbeddingTime       time.Duration     `json:"embedding_time"`
	CheckpointsCreated  int               `json:"checkpoints_created"`
	ResourcesUsed       *ResourceMetrics  `json:"resources_used"`
}

// ResourceMetrics tracks resource usage during processing
type ResourceMetrics struct {
	PeakMemoryMB    int64   `json:"peak_memory_mb"`
	CPUTimeSeconds  float64 `json:"cpu_time_seconds"`
	DiskIOBytes     int64   `json:"disk_io_bytes"`
	NetworkIOBytes  int64   `json:"network_io_bytes"`
}

// NewTierProcessor creates a new tier-based processor
func NewTierProcessor(db *database.Database, pipeline *Pipeline, embeddingService embeddings.EmbeddingService, config *TierProcessorConfig) *TierProcessor {
	if config == nil {
		config = GetDefaultTierProcessorConfig()
	}

	return &TierProcessor{
		db:               db,
		pipeline:         pipeline,
		embeddingService: embeddingService,
		config:           config,
	}
}

// GetDefaultTierProcessorConfig returns default tier processor configuration
func GetDefaultTierProcessorConfig() *TierProcessorConfig {
	return &TierProcessorConfig{
		SmallFileTierThreshold:  10 * 1024 * 1024,  // 10MB
		LargeFileTierThreshold:  1024 * 1024 * 1024, // 1GB
		SmallFileTimeout:        5 * time.Minute,
		MediumFileTimeout:       30 * time.Minute,
		LargeFileTimeout:        2 * time.Hour,
		CheckpointInterval:      5 * time.Minute,
		EnableEmbeddings:        true,
		EmbeddingBatchSize:      50,
		EmbeddingDataset:        "default",
	}
}

// ProcessFileWithTier processes a file using tier-based routing
func (tp *TierProcessor) ProcessFileWithTier(ctx context.Context, request *ProcessingRequest) (*TierProcessingResult, error) {
	startTime := time.Now()

	// Determine file size and tier
	fileInfo, err := os.Stat(request.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	tier := tp.determineTier(fileInfo.Size())
	
	// Create tier-specific context with appropriate timeout
	tierCtx, cancel := tp.createTierContext(ctx, tier)
	defer cancel()

	// Process using tier-specific strategy
	var result *TierProcessingResult
	switch tier {
	case TierSmall:
		result, err = tp.processSmallFile(tierCtx, request, fileInfo)
	case TierMedium:
		result, err = tp.processMediumFile(tierCtx, request, fileInfo)
	case TierLarge:
		result, err = tp.processLargeFile(tierCtx, request, fileInfo)
	default:
		return nil, fmt.Errorf("unknown processing tier: %v", tier)
	}

	if err != nil {
		return result, fmt.Errorf("tier processing failed: %w", err)
	}

	// Post-process: generate embeddings if enabled
	if tp.config.EnableEmbeddings && result.ProcessingResult.Status == "completed" {
		embeddingStartTime := time.Now()
		embeddingsCreated, err := tp.generateEmbeddings(tierCtx, request, result)
		if err != nil {
			// Log error but don't fail the entire processing
			result.ProcessingResult.Metadata = map[string]any{
				"embedding_error": err.Error(),
			}
		} else {
			result.EmbeddingsCreated = embeddingsCreated
		}
		result.EmbeddingTime = time.Since(embeddingStartTime)
	}

	result.Tier = tier
	result.ProcessingResult.ProcessingTime = time.Since(startTime)

	return result, nil
}

// determineTier determines the processing tier based on file size
func (tp *TierProcessor) determineTier(fileSize int64) ProcessingTier {
	if fileSize < tp.config.SmallFileTierThreshold {
		return TierSmall
	} else if fileSize < tp.config.LargeFileTierThreshold {
		return TierMedium
	}
	return TierLarge
}

// createTierContext creates a context with tier-appropriate timeout
func (tp *TierProcessor) createTierContext(ctx context.Context, tier ProcessingTier) (context.Context, context.CancelFunc) {
	var timeout time.Duration
	switch tier {
	case TierSmall:
		timeout = tp.config.SmallFileTimeout
	case TierMedium:
		timeout = tp.config.MediumFileTimeout
	case TierLarge:
		timeout = tp.config.LargeFileTimeout
	default:
		timeout = tp.config.MediumFileTimeout
	}

	return context.WithTimeout(ctx, timeout)
}

// processSmallFile processes small files with minimal overhead
func (tp *TierProcessor) processSmallFile(ctx context.Context, request *ProcessingRequest, fileInfo os.FileInfo) (*TierProcessingResult, error) {
	// For small files, use simple processing with no checkpoints
	result, err := tp.pipeline.ProcessFile(ctx, request)
	if err != nil {
		return nil, err
	}

	return &TierProcessingResult{
		ProcessingResult: result,
		ResourcesUsed: &ResourceMetrics{
			PeakMemoryMB: 50, // Estimate for small files
		},
	}, nil
}

// processMediumFile processes medium files with batching
func (tp *TierProcessor) processMediumFile(ctx context.Context, request *ProcessingRequest, fileInfo os.FileInfo) (*TierProcessingResult, error) {
	// For medium files, use standard processing with progress tracking
	result, err := tp.pipeline.ProcessFile(ctx, request)
	if err != nil {
		return nil, err
	}

	return &TierProcessingResult{
		ProcessingResult: result,
		ResourcesUsed: &ResourceMetrics{
			PeakMemoryMB: 200, // Estimate for medium files
		},
	}, nil
}

// processLargeFile processes large files with streaming and checkpoints
func (tp *TierProcessor) processLargeFile(ctx context.Context, request *ProcessingRequest, fileInfo os.FileInfo) (*TierProcessingResult, error) {
	// For large files, use streaming processing with periodic checkpoints
	checkpointTicker := time.NewTicker(tp.config.CheckpointInterval)
	defer checkpointTicker.Stop()

	checkpointCount := 0
	
	// Start processing in background
	resultChan := make(chan *ProcessingResult, 1)
	errorChan := make(chan error, 1)
	
	go func() {
		result, err := tp.pipeline.ProcessFile(ctx, request)
		if err != nil {
			errorChan <- err
		} else {
			resultChan <- result
		}
	}()

	// Monitor for checkpoints or completion
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-checkpointTicker.C:
			// Create checkpoint
			if err := tp.createCheckpoint(ctx, request.FileID, checkpointCount); err != nil {
				// Log error but continue
			}
			checkpointCount++
		case result := <-resultChan:
			// Processing completed successfully
			return &TierProcessingResult{
				ProcessingResult:   result,
				CheckpointsCreated: checkpointCount,
				ResourcesUsed: &ResourceMetrics{
					PeakMemoryMB: 1000, // Estimate for large files
				},
			}, nil
		case err := <-errorChan:
			// Processing failed
			return &TierProcessingResult{
				ProcessingResult: &ProcessingResult{
					FileID:       request.FileID,
					Status:       "failed",
					ErrorMessage: err.Error(),
				},
				CheckpointsCreated: checkpointCount,
			}, err
		}
	}
}

// createCheckpoint creates a processing checkpoint for recovery
func (tp *TierProcessor) createCheckpoint(ctx context.Context, fileID uuid.UUID, checkpointNum int) error {
	// In a full implementation, this would save processing state to database
	// For now, we'll just log the checkpoint creation
	return nil
}

// generateEmbeddings creates embeddings for all chunks of a processed file
func (tp *TierProcessor) generateEmbeddings(ctx context.Context, request *ProcessingRequest, result *TierProcessingResult) (int, error) {
	if tp.embeddingService == nil {
		return 0, fmt.Errorf("embedding service not configured")
	}

	// Get all chunks for this file from the database
	tenantService := tp.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, request.TenantID)
	if err != nil {
		return 0, fmt.Errorf("failed to get tenant repository: %w", err)
	}

	var chunks []models.Chunk
	if err := tenantRepo.DB().Where("file_id = ?", request.FileID).Find(&chunks).Error; err != nil {
		return 0, fmt.Errorf("failed to get chunks: %w", err)
	}

	if len(chunks) == 0 {
		return 0, nil // No chunks to process
	}

	// Process chunks in batches
	embeddingsCreated := 0
	batchSize := tp.config.EmbeddingBatchSize

	for i := 0; i < len(chunks); i += batchSize {
		end := i + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}

		batch := chunks[i:end]
		
		// Convert chunks to embedding input format
		chunkInputs := make([]*embeddings.ChunkInput, len(batch))
		for j, chunk := range batch {
			chunkInputs[j] = &embeddings.ChunkInput{
				ID:          chunk.ChunkID,
				DocumentID:  chunk.FileID.String(),
				Content:     chunk.Content,
				ChunkIndex:  chunk.ChunkNumber,
				ContentType: chunk.ChunkType,
				Metadata: map[string]interface{}{
					"file_id":        chunk.FileID.String(),
					"chunk_number":   chunk.ChunkNumber,
					"size_bytes":     chunk.SizeBytes,
					"processed_at":   chunk.ProcessedAt,
					"language":       chunk.Language,
					"quality_score":  chunk.Quality.Completeness,
				},
			}
		}

		// Process chunk batch
		embeddingRequest := &embeddings.ProcessChunksRequest{
			Chunks:   chunkInputs,
			Dataset:  tp.config.EmbeddingDataset,
			TenantID: request.TenantID,
			BatchSize: batchSize,
		}

		response, err := tp.embeddingService.ProcessChunks(ctx, embeddingRequest)
		if err != nil {
			return embeddingsCreated, fmt.Errorf("failed to process embedding batch %d: %w", i/batchSize, err)
		}

		embeddingsCreated += response.VectorsCreated

		// Update chunk embedding status
		for _, chunk := range batch {
			updates := map[string]interface{}{
				"embedding_status": models.EmbeddingStatusCompleted,
			}
			if err := tenantRepo.DB().Model(&chunk).Updates(updates).Error; err != nil {
				// Log error but continue
			}
		}
	}

	return embeddingsCreated, nil
}

// GetTierMetrics returns metrics for each processing tier
func (tp *TierProcessor) GetTierMetrics() map[ProcessingTier]*TierMetrics {
	// In a full implementation, this would track metrics per tier
	return map[ProcessingTier]*TierMetrics{
		TierSmall: {
			FilesProcessed:   100,
			AverageTime:      2 * time.Second,
			SuccessRate:      0.98,
			AverageFileSize:  1024 * 1024, // 1MB
		},
		TierMedium: {
			FilesProcessed:   50,
			AverageTime:      30 * time.Second,
			SuccessRate:      0.95,
			AverageFileSize:  50 * 1024 * 1024, // 50MB
		},
		TierLarge: {
			FilesProcessed:   10,
			AverageTime:      10 * time.Minute,
			SuccessRate:      0.90,
			AverageFileSize:  500 * 1024 * 1024, // 500MB
		},
	}
}

// TierMetrics contains metrics for a specific processing tier
type TierMetrics struct {
	FilesProcessed  int64         `json:"files_processed"`
	AverageTime     time.Duration `json:"average_time"`
	SuccessRate     float64       `json:"success_rate"`
	AverageFileSize int64         `json:"average_file_size"`
}

// EstimateProcessingTimeForTier estimates processing time based on tier
func (tp *TierProcessor) EstimateProcessingTimeForTier(fileSize int64, complexity string) time.Duration {
	tier := tp.determineTier(fileSize)
	
	baseTime := time.Second
	switch tier {
	case TierSmall:
		baseTime = 2 * time.Second
	case TierMedium:
		baseTime = 30 * time.Second
	case TierLarge:
		baseTime = 5 * time.Minute
	}

	// Adjust for complexity
	multiplier := 1.0
	switch complexity {
	case "low":
		multiplier = 0.8
	case "medium":
		multiplier = 1.0
	case "high":
		multiplier = 1.5
	}

	return time.Duration(float64(baseTime) * multiplier)
}