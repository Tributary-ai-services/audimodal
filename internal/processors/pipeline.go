package processors

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/internal/database/models"
	"github.com/jscharber/eAIIngest/pkg/core"
	"github.com/jscharber/eAIIngest/pkg/readers"
	"github.com/jscharber/eAIIngest/pkg/registry"
	"github.com/jscharber/eAIIngest/pkg/strategies"
)

// Pipeline orchestrates the complete file processing workflow
type Pipeline struct {
	db       *database.Database
	registry *registry.Registry
	config   *PipelineConfig
	metrics  *PipelineMetrics
	mu       sync.RWMutex
}

// PipelineConfig contains configuration for the processing pipeline
type PipelineConfig struct {
	MaxConcurrentFiles  int           `json:"max_concurrent_files"`
	MaxConcurrentChunks int           `json:"max_concurrent_chunks"`
	ChunkBatchSize      int           `json:"chunk_batch_size"`
	ProcessingTimeout   time.Duration `json:"processing_timeout"`
	RetryAttempts       int           `json:"retry_attempts"`
	RetryDelay          time.Duration `json:"retry_delay"`
	AutoSelectReader    bool          `json:"auto_select_reader"`
	AutoSelectStrategy  bool          `json:"auto_select_strategy"`
	EnableQualityFilter bool          `json:"enable_quality_filter"`
	MinQualityThreshold float64       `json:"min_quality_threshold"`
	TempStoragePath     string        `json:"temp_storage_path"`
	EnableMetrics       bool          `json:"enable_metrics"`
}

// PipelineMetrics tracks processing statistics
type PipelineMetrics struct {
	FilesProcessed   int64     `json:"files_processed"`
	ChunksCreated    int64     `json:"chunks_created"`
	BytesProcessed   int64     `json:"bytes_processed"`
	ErrorCount       int64     `json:"error_count"`
	AverageFileTime  float64   `json:"average_file_time_ms"`
	AverageChunkTime float64   `json:"average_chunk_time_ms"`
	LastProcessedAt  time.Time `json:"last_processed_at"`
	mu               sync.RWMutex
}

// ProcessingRequest represents a file processing request
type ProcessingRequest struct {
	TenantID        uuid.UUID      `json:"tenant_id"`
	SessionID       uuid.UUID      `json:"session_id"`
	FileID          uuid.UUID      `json:"file_id"`
	FilePath        string         `json:"file_path"`
	ReaderType      string         `json:"reader_type,omitempty"`
	StrategyType    string         `json:"strategy_type,omitempty"`
	ReaderConfig    map[string]any `json:"reader_config,omitempty"`
	StrategyConfig  map[string]any `json:"strategy_config,omitempty"`
	Priority        string         `json:"priority"`
	DLPScanEnabled  bool           `json:"dlp_scan_enabled"`
	ComplianceRules []string       `json:"compliance_rules,omitempty"`
}

// ProcessingResult contains the results of file processing
type ProcessingResult struct {
	FileID         uuid.UUID      `json:"file_id"`
	Status         string         `json:"status"`
	ChunksCreated  int            `json:"chunks_created"`
	BytesProcessed int64          `json:"bytes_processed"`
	ProcessingTime time.Duration  `json:"processing_time"`
	ReaderUsed     string         `json:"reader_used"`
	StrategyUsed   string         `json:"strategy_used"`
	QualityScore   float64        `json:"quality_score"`
	ErrorMessage   string         `json:"error_message,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// NewPipeline creates a new processing pipeline
func NewPipeline(db *database.Database, config *PipelineConfig) *Pipeline {
	if config == nil {
		config = GetDefaultPipelineConfig()
	}

	return &Pipeline{
		db:       db,
		registry: registry.GlobalRegistry,
		config:   config,
		metrics:  &PipelineMetrics{},
	}
}

// GetDefaultPipelineConfig returns default pipeline configuration
func GetDefaultPipelineConfig() *PipelineConfig {
	return &PipelineConfig{
		MaxConcurrentFiles:  5,
		MaxConcurrentChunks: 20,
		ChunkBatchSize:      100,
		ProcessingTimeout:   30 * time.Minute,
		RetryAttempts:       3,
		RetryDelay:          5 * time.Second,
		AutoSelectReader:    true,
		AutoSelectStrategy:  true,
		EnableQualityFilter: true,
		MinQualityThreshold: 0.3,
		TempStoragePath:     "/tmp/audimodal",
		EnableMetrics:       true,
	}
}

// ProcessFile processes a single file through the complete pipeline
func (p *Pipeline) ProcessFile(ctx context.Context, request *ProcessingRequest) (*ProcessingResult, error) {
	startTime := time.Now()

	result := &ProcessingResult{
		FileID: request.FileID,
		Status: "processing",
	}

	// Update file status to processing
	if err := p.updateFileStatus(ctx, request.FileID, models.FileStatusProcessing, ""); err != nil {
		return nil, fmt.Errorf("failed to update file status: %w", err)
	}

	// Process with timeout
	processCtx, cancel := context.WithTimeout(ctx, p.config.ProcessingTimeout)
	defer cancel()

	// Execute processing pipeline
	err := p.executeProcessingPipeline(processCtx, request, result)

	// Calculate processing time
	result.ProcessingTime = time.Since(startTime)

	// Update metrics
	if p.config.EnableMetrics {
		p.updateMetrics(result)
	}

	// Update final file status
	finalStatus := models.FileStatusProcessed
	if err != nil {
		finalStatus = models.FileStatusFailed
		result.Status = "failed"
		result.ErrorMessage = err.Error()
	} else {
		result.Status = "completed"
	}

	if updateErr := p.updateFileStatus(ctx, request.FileID, finalStatus, result.ErrorMessage); updateErr != nil {
		// Log error but don't override original error
		if err == nil {
			err = fmt.Errorf("failed to update final file status: %w", updateErr)
		}
	}

	return result, err
}

// ProcessFiles processes multiple files concurrently
func (p *Pipeline) ProcessFiles(ctx context.Context, requests []*ProcessingRequest) ([]*ProcessingResult, error) {
	results := make([]*ProcessingResult, len(requests))
	errors := make([]error, len(requests))

	// Create semaphore for concurrent processing
	sem := make(chan struct{}, p.config.MaxConcurrentFiles)
	var wg sync.WaitGroup

	for i, request := range requests {
		wg.Add(1)
		go func(idx int, req *ProcessingRequest) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Process file
			result, err := p.ProcessFile(ctx, req)
			results[idx] = result
			errors[idx] = err
		}(i, request)
	}

	wg.Wait()

	// Check for any errors
	var combinedError error
	errorCount := 0
	for i, err := range errors {
		if err != nil {
			errorCount++
			if combinedError == nil {
				combinedError = fmt.Errorf("processing errors: file %d: %w", i, err)
			}
		}
	}

	if errorCount > 0 && errorCount == len(requests) {
		return results, combinedError
	}

	return results, nil
}

// executeProcessingPipeline runs the complete processing pipeline for a file
func (p *Pipeline) executeProcessingPipeline(ctx context.Context, request *ProcessingRequest, result *ProcessingResult) error {
	// Step 1: Auto-select reader if needed
	readerType := request.ReaderType
	if readerType == "" && p.config.AutoSelectReader {
		ext := strings.ToLower(filepath.Ext(request.FilePath))
		readerType = readers.GetReaderForExtension(ext)
		if readerType == "" {
			return fmt.Errorf("no suitable reader found for file extension: %s", ext)
		}
	}

	// Step 2: Get reader instance
	reader, err := p.registry.GetReader(readerType)
	if err != nil {
		return fmt.Errorf("failed to get reader %s: %w", readerType, err)
	}
	result.ReaderUsed = readerType

	// Step 3: Discover schema
	schema, err := reader.DiscoverSchema(ctx, request.FilePath)
	if err != nil {
		return fmt.Errorf("schema discovery failed: %w", err)
	}

	// Step 4: Auto-select strategy if needed
	strategyType := request.StrategyType
	if strategyType == "" && p.config.AutoSelectStrategy {
		strategyType = strategies.GetStrategyForDataType(schema.Format)
	}

	// Step 5: Get strategy instance
	strategy, err := p.registry.GetStrategy(strategyType)
	if err != nil {
		return fmt.Errorf("failed to get strategy %s: %w", strategyType, err)
	}
	result.StrategyUsed = strategyType

	// Step 6: Configure reader for strategy
	readerConfig := request.ReaderConfig
	if readerConfig == nil {
		readerConfig = make(map[string]any)
	}

	configuredReaderConfig, err := strategy.ConfigureReader(readerConfig)
	if err != nil {
		return fmt.Errorf("failed to configure reader: %w", err)
	}

	// Step 7: Create iterator
	iterator, err := reader.CreateIterator(ctx, request.FilePath, configuredReaderConfig)
	if err != nil {
		return fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iterator.Close()

	// Step 8: Process chunks
	return p.processChunks(ctx, iterator, strategy, request, result)
}

// processChunks processes all chunks from the iterator
func (p *Pipeline) processChunks(ctx context.Context, iterator core.ChunkIterator, strategy core.ChunkingStrategy, request *ProcessingRequest, result *ProcessingResult) error {
	// Get tenant repository
	tenantService := p.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, request.TenantID)
	if err != nil {
		return fmt.Errorf("failed to get tenant repository: %w", err)
	}

	var totalQuality float64
	var qualityCount int
	chunkNumber := 0

	// Process chunks in batches
	chunkBatch := make([]*models.Chunk, 0, p.config.ChunkBatchSize)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Get next chunk from iterator
		rawChunk, err := iterator.Next(ctx)
		if err != nil {
			if err == core.ErrIteratorExhausted {
				break
			}
			return fmt.Errorf("iterator error: %w", err)
		}

		// Create chunk metadata
		metadata := core.ChunkMetadata{
			SourcePath:  request.FilePath,
			ChunkID:     fmt.Sprintf("file_%s_chunk_%d", request.FileID.String(), chunkNumber),
			ProcessedAt: time.Now(),
			Context:     p.buildChunkContext(request),
		}

		// Apply chunking strategy
		processedChunks, err := strategy.ProcessChunk(ctx, rawChunk.Data, metadata)
		if err != nil {
			return fmt.Errorf("strategy processing failed: %w", err)
		}

		// Convert to database models and apply quality filter
		for _, processedChunk := range processedChunks {
			dbChunk := p.convertToDBChunk(processedChunk, request)

			// Apply quality filter if enabled
			if p.config.EnableQualityFilter && dbChunk.Quality.Completeness < p.config.MinQualityThreshold {
				continue // Skip low-quality chunks
			}

			chunkBatch = append(chunkBatch, dbChunk)

			// Update quality tracking
			if dbChunk.Quality.Completeness > 0 {
				totalQuality += dbChunk.Quality.Completeness
				qualityCount++
			}

			result.BytesProcessed += dbChunk.SizeBytes
			chunkNumber++

			// Process batch if full
			if len(chunkBatch) >= p.config.ChunkBatchSize {
				if err := p.saveBatch(ctx, tenantRepo, chunkBatch); err != nil {
					return fmt.Errorf("failed to save chunk batch: %w", err)
				}
				result.ChunksCreated += len(chunkBatch)
				chunkBatch = chunkBatch[:0] // Reset batch
			}
		}
	}

	// Process remaining chunks in batch
	if len(chunkBatch) > 0 {
		if err := p.saveBatch(ctx, tenantRepo, chunkBatch); err != nil {
			return fmt.Errorf("failed to save final chunk batch: %w", err)
		}
		result.ChunksCreated += len(chunkBatch)
	}

	// Calculate average quality
	if qualityCount > 0 {
		result.QualityScore = totalQuality / float64(qualityCount)
	}

	// Update file chunk count
	return p.updateFileChunkCount(ctx, request.FileID, result.ChunksCreated)
}

// saveBatch saves a batch of chunks to the database
func (p *Pipeline) saveBatch(ctx context.Context, tenantRepo *database.TenantRepository, chunks []*models.Chunk) error {
	if len(chunks) == 0 {
		return nil
	}

	// Use batch insert for better performance
	return tenantRepo.DB().CreateInBatches(chunks, len(chunks)).Error
}

// convertToDBChunk converts a core.Chunk to a database models.Chunk
func (p *Pipeline) convertToDBChunk(chunk core.Chunk, request *ProcessingRequest) *models.Chunk {
	dbChunk := &models.Chunk{
		TenantID:        request.TenantID,
		FileID:          request.FileID,
		ChunkID:         chunk.Metadata.ChunkID,
		ChunkType:       chunk.Metadata.ChunkType,
		ChunkNumber:     p.extractChunkNumber(chunk.Metadata.ChunkID),
		Content:         fmt.Sprintf("%v", chunk.Data),
		ContentHash:     p.calculateContentHash(chunk.Data),
		SizeBytes:       chunk.Metadata.SizeBytes,
		StartPosition:   chunk.Metadata.StartPosition,
		EndPosition:     chunk.Metadata.EndPosition,
		PageNumber:      nil, // Could be extracted from context if needed
		LineNumber:      nil, // Could be extracted from context if needed
		Relationships:   chunk.Metadata.Relationships,
		ProcessedAt:     chunk.Metadata.ProcessedAt,
		ProcessedBy:     chunk.Metadata.ProcessedBy,
		ProcessingTime:  0, // Could be calculated if needed
		Context:         chunk.Metadata.Context,
		SchemaInfo:      chunk.Metadata.SchemaInfo,
		EmbeddingStatus: models.EmbeddingStatusPending,
		DLPScanStatus:   models.DLPScanStatusPending,
	}

	// Set quality metrics if available
	if chunk.Metadata.Quality != nil {
		dbChunk.Quality = models.ChunkQualityMetrics{
			Completeness: chunk.Metadata.Quality.Completeness,
			Coherence:    chunk.Metadata.Quality.Coherence,
			Uniqueness:   chunk.Metadata.Quality.Uniqueness,
			Readability:  chunk.Metadata.Quality.Readability,
		}
		dbChunk.Language = chunk.Metadata.Quality.Language
		dbChunk.LanguageConf = chunk.Metadata.Quality.LanguageConf
	}

	return dbChunk
}

// buildChunkContext creates context information for chunks
func (p *Pipeline) buildChunkContext(request *ProcessingRequest) map[string]string {
	context := make(map[string]string)
	context["tenant_id"] = request.TenantID.String()
	context["session_id"] = request.SessionID.String()
	context["file_id"] = request.FileID.String()
	context["priority"] = request.Priority

	if request.DLPScanEnabled {
		context["dlp_scan_enabled"] = "true"
	}

	// Add strategy config to context
	for key, value := range request.StrategyConfig {
		context[key] = fmt.Sprintf("%v", value)
	}

	return context
}

// updateFileStatus updates the processing status of a file
func (p *Pipeline) updateFileStatus(ctx context.Context, fileID uuid.UUID, status string, errorMsg string) error {
	tenantService := p.db.NewTenantService()

	// We need to find the tenant for this file first
	var file models.File
	if err := p.db.DB().Where("id = ?", fileID).First(&file).Error; err != nil {
		return fmt.Errorf("failed to find file: %w", err)
	}

	tenantRepo, err := tenantService.GetTenantRepository(ctx, file.TenantID)
	if err != nil {
		return fmt.Errorf("failed to get tenant repository: %w", err)
	}

	// Update file status
	updates := map[string]interface{}{
		"status": status,
	}

	if status == models.FileStatusProcessed {
		now := time.Now()
		updates["processed_at"] = &now
	}

	if errorMsg != "" {
		updates["processing_error"] = &errorMsg
	}

	return tenantRepo.DB().Model(&file).Updates(updates).Error
}

// updateFileChunkCount updates the chunk count for a file
func (p *Pipeline) updateFileChunkCount(ctx context.Context, fileID uuid.UUID, chunkCount int) error {
	tenantService := p.db.NewTenantService()

	var file models.File
	if err := p.db.DB().Where("id = ?", fileID).First(&file).Error; err != nil {
		return fmt.Errorf("failed to find file: %w", err)
	}

	tenantRepo, err := tenantService.GetTenantRepository(ctx, file.TenantID)
	if err != nil {
		return fmt.Errorf("failed to get tenant repository: %w", err)
	}

	return tenantRepo.DB().Model(&file).Update("chunk_count", chunkCount).Error
}

// updateMetrics updates pipeline processing metrics
func (p *Pipeline) updateMetrics(result *ProcessingResult) {
	p.metrics.mu.Lock()
	defer p.metrics.mu.Unlock()

	p.metrics.FilesProcessed++
	p.metrics.ChunksCreated += int64(result.ChunksCreated)
	p.metrics.BytesProcessed += result.BytesProcessed

	if result.Status == "failed" {
		p.metrics.ErrorCount++
	}

	// Update average processing times (simple moving average)
	fileTimeMs := float64(result.ProcessingTime.Nanoseconds()) / 1e6
	if p.metrics.AverageFileTime == 0 {
		p.metrics.AverageFileTime = fileTimeMs
	} else {
		p.metrics.AverageFileTime = (p.metrics.AverageFileTime + fileTimeMs) / 2
	}

	if result.ChunksCreated > 0 {
		chunkTimeMs := fileTimeMs / float64(result.ChunksCreated)
		if p.metrics.AverageChunkTime == 0 {
			p.metrics.AverageChunkTime = chunkTimeMs
		} else {
			p.metrics.AverageChunkTime = (p.metrics.AverageChunkTime + chunkTimeMs) / 2
		}
	}

	p.metrics.LastProcessedAt = time.Now()
}

// GetMetrics returns current pipeline metrics
func (p *Pipeline) GetMetrics() *PipelineMetrics {
	p.metrics.mu.RLock()
	defer p.metrics.mu.RUnlock()

	// Return a copy to avoid race conditions
	return &PipelineMetrics{
		FilesProcessed:   p.metrics.FilesProcessed,
		ChunksCreated:    p.metrics.ChunksCreated,
		BytesProcessed:   p.metrics.BytesProcessed,
		ErrorCount:       p.metrics.ErrorCount,
		AverageFileTime:  p.metrics.AverageFileTime,
		AverageChunkTime: p.metrics.AverageChunkTime,
		LastProcessedAt:  p.metrics.LastProcessedAt,
	}
}

// Helper functions

func (p *Pipeline) extractChunkNumber(chunkID string) int {
	// Extract chunk number from chunk ID pattern
	parts := strings.Split(chunkID, "_")
	if len(parts) > 0 {
		for i := len(parts) - 1; i >= 0; i-- {
			if strings.HasPrefix(parts[i], "chunk") {
				// Try to extract number
				var num int
				if _, err := fmt.Sscanf(parts[i], "chunk_%d", &num); err == nil {
					return num
				}
				if _, err := fmt.Sscanf(parts[i], "%d", &num); err == nil {
					return num
				}
			}
		}
	}
	return 0
}

func (p *Pipeline) calculateContentHash(content any) string {
	// Simple hash calculation (in production, use proper hashing)
	contentStr := fmt.Sprintf("%v", content)
	hash := 0
	for _, char := range contentStr {
		hash = hash*31 + int(char)
	}
	return fmt.Sprintf("%x", hash)
}
