package processors

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/jscharber/audimodal/internal/database"
	"github.com/jscharber/audimodal/internal/database/models"
	"github.com/jscharber/audimodal/pkg/core"
	"github.com/jscharber/audimodal/pkg/dlp"
	"github.com/jscharber/audimodal/pkg/dlp/types"
	"github.com/jscharber/audimodal/pkg/preprocessing"
	"github.com/jscharber/audimodal/pkg/readers"
	"github.com/jscharber/audimodal/pkg/registry"
	"github.com/jscharber/audimodal/pkg/strategies"

	"log"
)

// Pipeline orchestrates the complete file processing workflow
type Pipeline struct {
	db         *database.Database
	registry   *registry.Registry
	config     *PipelineConfig
	metrics    *PipelineMetrics
	dlpService *dlp.DLPService
	mu         sync.RWMutex
}

// RegisterBasicPlugins ensures all basic readers and strategies are registered
func RegisterBasicPlugins() {
	// Register readers
	readers.RegisterBasicReaders()
	// Register strategies
	strategies.RegisterBasicStrategies()
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
	EnableDLP           bool          `json:"enable_dlp"`
	DLPBatchSize        int           `json:"dlp_batch_size"`
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
	TenantID        uuid.UUID              `json:"tenant_id"`
	SessionID       uuid.UUID              `json:"session_id"`
	FileID          uuid.UUID              `json:"file_id"`
	FilePath        string                 `json:"file_path"`
	ReaderType      string                 `json:"reader_type,omitempty"`
	StrategyType    string                 `json:"strategy_type,omitempty"`
	ReaderConfig    map[string]any         `json:"reader_config,omitempty"`
	StrategyConfig  map[string]any         `json:"strategy_config,omitempty"`
	Priority        string                 `json:"priority"`
	DLPScanEnabled  bool                   `json:"dlp_scan_enabled"`
	RedactionMode   types.RedactionStrategy `json:"redaction_mode,omitempty"` // mask, replace, hash, remove, tokenize, none
	ComplianceRules []string               `json:"compliance_rules,omitempty"`
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

	// Ensure basic strategies and readers are registered
	// This is safe to call multiple times as it will just overwrite existing registrations
	RegisterBasicPlugins()

	// Initialize DLP service if DLP is enabled
	var dlpService *dlp.DLPService
	if config.EnableDLP {
		dlpService = dlp.NewDLPService(nil) // Use default DLP config
		fmt.Printf("DEBUG: Pipeline created with DLP service enabled\n")
	} else {
		fmt.Printf("DEBUG: Pipeline created with DLP DISABLED (config.EnableDLP=%v)\n", config.EnableDLP)
	}

	return &Pipeline{
		db:         db,
		registry:   registry.GlobalRegistry,
		config:     config,
		metrics:    &PipelineMetrics{},
		dlpService: dlpService,
	}
}

// GetDefaultPipelineConfig returns default pipeline configuration
func GetDefaultPipelineConfig() *PipelineConfig {
	return &PipelineConfig{
		MaxConcurrentFiles:  2, // Reduced from 5 to limit peak memory (60% reduction)
		MaxConcurrentChunks: 20,
		ChunkBatchSize:      5, // Reduced from 10 for faster GC cycles and lower peak memory
		ProcessingTimeout:   6 * time.Hour, // Must accommodate large OCR PDFs (122-page scan can take hours)
		RetryAttempts:       3,
		RetryDelay:          5 * time.Second,
		AutoSelectReader:    true,
		AutoSelectStrategy:  true,
		EnableQualityFilter: true,
		MinQualityThreshold: 0.3,
		TempStoragePath:     "/tmp/audimodal",
		EnableMetrics:       true,
		EnableDLP:           true, // Enable DLP scanning by default
		DLPBatchSize:        20,
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

	// Use a fresh context for post-processing (status updates, DLP scan).
	// The extraction may have taken hours, causing the original context to expire,
	// but we still need to save results and update status.
	postCtx, postCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer postCancel()

	// Update final file status
	finalStatus := models.FileStatusProcessed
	if err != nil {
		finalStatus = models.FileStatusFailed
		result.Status = "failed"
		result.ErrorMessage = err.Error()
	} else {
		result.Status = "completed"
	}

	// Perform DLP scanning if enabled and processing succeeded
	dlpEnabled := p.config.EnableDLP || request.DLPScanEnabled
	fmt.Printf("DEBUG Pipeline DLP: config.EnableDLP=%v, request.DLPScanEnabled=%v, dlpEnabled=%v, dlpService=%v, status=%s\n",
		p.config.EnableDLP, request.DLPScanEnabled, dlpEnabled, p.dlpService != nil, result.Status)
	if dlpEnabled && p.dlpService != nil && result.Status == "completed" {
		violationsFound, dlpErr := p.performDLPScan(postCtx, request)
		if dlpErr != nil {
			fmt.Printf("WARNING: DLP scan failed for file %s: %v\n", request.FileID.String(), dlpErr)
			if result.Metadata == nil {
				result.Metadata = map[string]any{}
			}
			result.Metadata["dlp_error"] = dlpErr.Error()
		} else {
			fmt.Printf("INFO: DLP scan completed for file %s: %d violations found\n", request.FileID.String(), violationsFound)
			if result.Metadata == nil {
				result.Metadata = map[string]any{}
			}
			result.Metadata["dlp_violations_found"] = violationsFound
		}
	}

	if updateErr := p.updateFileStatus(postCtx, request.FileID, finalStatus, result.ErrorMessage); updateErr != nil {
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

	// Add tenant/file context for map-reduce processing
	configuredReaderConfig["tenant_id"] = request.TenantID.String()
	configuredReaderConfig["file_id"] = request.FileID.String()

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
	// Use a fresh context for chunk processing. The extraction phase (MapReduce
	// OCR) can take hours and already manages its own per-page timeouts. The
	// parent context may expire during extraction, but we must continue to
	// iterate chunks and save them to the database.
	chunkCtx, chunkCancel := context.WithCancel(context.Background())
	defer chunkCancel()

	// Propagate only explicit cancellation from parent
	go func() {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.Canceled {
				chunkCancel()
			}
		case <-chunkCtx.Done():
		}
	}()

	// Get tenant repository
	tenantService := p.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(chunkCtx, request.TenantID)
	if err != nil {
		return fmt.Errorf("failed to get tenant repository: %w", err)
	}

	var totalQuality float64
	var qualityCount int
	chunkNumber := 0

	// Process chunks in batches
	chunkBatch := make([]*models.Chunk, 0, p.config.ChunkBatchSize)

	iteratorCallCount := 0
	for {
		select {
		case <-chunkCtx.Done():
			return chunkCtx.Err()
		default:
		}

		// Get next chunk from iterator
		iteratorCallCount++
		log.Printf("[PIPELINE] Calling iterator.Next() #%d, current chunks=%d", iteratorCallCount, chunkNumber)
		rawChunk, err := iterator.Next(chunkCtx)
		if err != nil {
			if err == core.ErrIteratorExhausted {
				log.Printf("[PIPELINE] Iterator exhausted after %d calls, total chunks=%d", iteratorCallCount, chunkNumber)
				break
			}
			return fmt.Errorf("iterator error: %w", err)
		}
		log.Printf("[PIPELINE] iterator.Next() #%d returned chunk with %d bytes", iteratorCallCount, len(fmt.Sprintf("%v", rawChunk.Data)))

		// Create chunk metadata
		metadata := core.ChunkMetadata{
			SourcePath:  request.FilePath,
			ChunkID:     fmt.Sprintf("file_%s_chunk_%d", request.FileID.String(), chunkNumber),
			ProcessedAt: time.Now(),
			Context:     p.buildChunkContext(request),
		}

		// DEBUG: Log raw chunk data before strategy processing
		log.Printf("[DEBUG] Raw iterator chunk data (length=%d): %.200s...",
			len(fmt.Sprintf("%v", rawChunk.Data)),
			fmt.Sprintf("%v", rawChunk.Data))

		// Apply chunking strategy
		log.Printf("[PIPELINE] Calling strategy.ProcessChunk() for iterator #%d", iteratorCallCount)
		processedChunks, err := strategy.ProcessChunk(chunkCtx, rawChunk.Data, metadata)
		if err != nil {
			return fmt.Errorf("strategy processing failed: %w", err)
		}
		log.Printf("[PIPELINE] strategy.ProcessChunk() returned %d chunks", len(processedChunks))

		// Convert to database models and apply quality filter
		for _, processedChunk := range processedChunks {
			// DEBUG: Log input chunk data before conversion
			log.Printf("[DEBUG] Raw chunk data (length=%d): %.200s...",
				len(fmt.Sprintf("%v", processedChunk.Data)),
				fmt.Sprintf("%v", processedChunk.Data))

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
				batchNum := result.ChunksCreated/p.config.ChunkBatchSize + 1
				log.Printf("[PIPELINE] Saving batch #%d with %d chunks (total so far: %d)", batchNum, len(chunkBatch), result.ChunksCreated)
				if err := p.saveBatch(chunkCtx, tenantRepo, chunkBatch); err != nil {
					return fmt.Errorf("failed to save chunk batch: %w", err)
				}
				result.ChunksCreated += len(chunkBatch)
				log.Printf("[PIPELINE] Batch #%d saved successfully, total chunks: %d", batchNum, result.ChunksCreated)
				chunkBatch = chunkBatch[:0] // Reset batch
				runtime.GC()                // Force GC to release memory between batches
				log.Printf("[PIPELINE] GC complete, continuing to next iterator chunk")
			}
		}
	}

	// Process remaining chunks in batch
	if len(chunkBatch) > 0 {
		if err := p.saveBatch(chunkCtx, tenantRepo, chunkBatch); err != nil {
			return fmt.Errorf("failed to save final chunk batch: %w", err)
		}
		result.ChunksCreated += len(chunkBatch)
	}

	// Calculate average quality
	if qualityCount > 0 {
		result.QualityScore = totalQuality / float64(qualityCount)
	}

	// Update file chunk count
	return p.updateFileChunkCount(chunkCtx, request.FileID, result.ChunksCreated)
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
	// Convert chunk data to string and sanitize UTF-8 to prevent PostgreSQL encoding errors
	// This handles PDFs and other documents that may contain malformed UTF-8 sequences
	content := fmt.Sprintf("%v", chunk.Data)
	sanitizedContent := preprocessing.SanitizeUTF8String(content)

	// Generate content preview (first 500 chars)
	preview := sanitizedContent
	if len(preview) > 500 {
		preview = preview[:500]
	}

	dbChunk := &models.Chunk{
		TenantID:        request.TenantID,
		FileID:          request.FileID,
		ChunkID:         chunk.Metadata.ChunkID,
		ChunkType:       chunk.Metadata.ChunkType,
		ChunkNumber:     p.extractChunkNumber(chunk.Metadata.ChunkID),
		ContentPreview:  preview,
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

// performDLPScan scans all chunks of a processed file for PII and creates violations
func (p *Pipeline) performDLPScan(ctx context.Context, request *ProcessingRequest) (int, error) {
	tenantService := p.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, request.TenantID)
	if err != nil {
		return 0, fmt.Errorf("failed to get tenant repository: %w", err)
	}

	var chunks []models.Chunk
	if err := tenantRepo.DB().Where("file_id = ?", request.FileID).Find(&chunks).Error; err != nil {
		return 0, fmt.Errorf("failed to get chunks: %w", err)
	}

	if len(chunks) == 0 {
		return 0, nil // No chunks to scan
	}

	// Build compliance rules from request or use defaults
	var complianceRules []types.ComplianceRule
	if len(request.ComplianceRules) > 0 {
		for _, ruleName := range request.ComplianceRules {
			complianceRules = append(complianceRules, types.ComplianceRule{
				Regulation: ruleName,
				Required:   true,
			})
		}
	} else {
		// Default: check all major compliance frameworks
		complianceRules = []types.ComplianceRule{
			{Regulation: "GDPR", Required: true},
			{Regulation: "HIPAA", Required: true},
			{Regulation: "PCI-DSS", Required: true},
			{Regulation: "CCPA", Required: true},
		}
	}

	totalViolations := 0
	batchSize := p.config.DLPBatchSize
	if batchSize <= 0 {
		batchSize = 20
	}

	// Process chunks in batches
	for i := 0; i < len(chunks); i += batchSize {
		end := i + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}

		batch := chunks[i:end]

		// Create batch scan request
		chunkRequests := make([]dlp.ChunkScanRequest, len(batch))
		for j, chunk := range batch {
			chunkRequests[j] = dlp.ChunkScanRequest{
				TenantID:        request.TenantID,
				ChunkID:         chunk.ChunkID,
				FileID:          request.FileID,
				Content:         chunk.ContentPreview, // Use preview for DLP scanning in legacy pipeline
				Metadata:        map[string]string{"chunk_number": fmt.Sprintf("%d", chunk.ChunkNumber)},
				ComplianceRules: complianceRules,
			}
		}

		batchRequest := &dlp.BatchScanRequest{
			TenantID:        request.TenantID,
			Chunks:          chunkRequests,
			ComplianceRules: complianceRules,
			MaxConcurrency:  5,
		}

		batchResponse, err := p.dlpService.ScanBatch(ctx, batchRequest)
		if err != nil {
			return totalViolations, fmt.Errorf("batch DLP scan failed: %w", err)
		}

		// Process scan results and create violations
		for chunkID, scanResponse := range batchResponse.Results {
			now := time.Now()

			// Update chunk DLP status
			chunkUpdates := map[string]interface{}{
				"dlp_scan_status": models.DLPScanStatusCompleted,
			}
			if scanResponse.ScanResult != nil {
				chunkUpdates["dlp_finding_count"] = scanResponse.ScanResult.TotalMatches
				chunkUpdates["dlp_risk_score"] = scanResponse.ScanResult.RiskScore
			}
			chunkUpdates["dlp_scanned_at"] = &now

			if err := tenantRepo.DB().Model(&models.Chunk{}).Where("chunk_id = ?", chunkID).Updates(chunkUpdates).Error; err != nil {
				fmt.Printf("WARNING: Failed to update chunk %s DLP status: %v\n", chunkID, err)
			}

			// Create violations for compliance results
			if scanResponse.ComplianceResult != nil && len(scanResponse.ComplianceResult.Violations) > 0 {
				for _, violation := range scanResponse.ComplianceResult.Violations {
					// Find the chunk to get its UUID
					var chunkUUID uuid.UUID
					for _, c := range batch {
						if c.ChunkID == chunkID {
							chunkUUID = c.ID
							break
						}
					}

					// Derive regulation from rule (e.g., "GDPR-001" -> "GDPR")
					regulation := violation.Rule
					if idx := len(violation.Rule); idx > 4 && violation.Rule[idx-4] == '-' {
						regulation = violation.Rule[:idx-4]
					}

					// Get or create the default compliance policy for this tenant
					policyID, policyErr := p.getOrCreateCompliancePolicy(tenantRepo, request.TenantID)
					if policyErr != nil {
						fmt.Printf("WARNING: Failed to get/create compliance policy: %v\n", policyErr)
						continue
					}

					dlpViolation := models.DLPViolation{
						TenantID:       request.TenantID,
						PolicyID:       policyID,
						FileID:         request.FileID,
						ChunkID:        &chunkUUID,
						RuleName:       violation.Rule,
						Severity:       violation.Severity,
						ComplianceType: regulation,
						Confidence:     0.9, // High confidence for pattern matches
						Context:        violation.Description,
						Status:         "detected",
						Acknowledged:   false,
					}

					if err := tenantRepo.DB().Create(&dlpViolation).Error; err != nil {
						fmt.Printf("WARNING: Failed to create DLP violation for chunk %s: %v\n", chunkID, err)
					} else {
						totalViolations++
					}
				}
			}
		}
	}

	return totalViolations, nil
}

// getOrCreateCompliancePolicy gets or creates the default compliance scanner policy for a tenant
func (p *Pipeline) getOrCreateCompliancePolicy(tenantRepo *database.TenantRepository, tenantID uuid.UUID) (uuid.UUID, error) {
	const defaultPolicyName = "compliance-auto-scanner"

	// Try to find existing policy
	var policy models.DLPPolicy
	err := tenantRepo.DB().Where("tenant_id = ? AND name = ?", tenantID, defaultPolicyName).First(&policy).Error
	if err == nil {
		return policy.ID, nil
	}

	// Create default compliance policy if not found
	policy = models.DLPPolicy{
		TenantID:    tenantID,
		Name:        defaultPolicyName,
		DisplayName: "Compliance Auto-Scanner",
		Description: "Automatically created policy for compliance scanning (GDPR, HIPAA, PCI-DSS, CCPA)",
		Enabled:     true,
		Priority:    100,
		ContentRules: models.DLPContentRules{
			{Name: "SSN Detection", Type: "builtin", BuiltinType: "ssn", Sensitivity: "critical", Confidence: 0.9},
			{Name: "Credit Card Detection", Type: "builtin", BuiltinType: "credit_card", Sensitivity: "critical", Confidence: 0.9},
			{Name: "Email Detection", Type: "builtin", BuiltinType: "email", Sensitivity: "medium", Confidence: 0.8},
			{Name: "Phone Detection", Type: "builtin", BuiltinType: "phone", Sensitivity: "medium", Confidence: 0.8},
			{Name: "IP Address Detection", Type: "builtin", BuiltinType: "ip_address", Sensitivity: "low", Confidence: 0.7},
		},
		Actions: models.DLPActions{
			{Type: "audit", Priority: 1, AuditLevel: "warning"},
		},
		Status: "active",
	}

	if err := tenantRepo.DB().Create(&policy).Error; err != nil {
		return uuid.Nil, fmt.Errorf("failed to create default compliance policy: %w", err)
	}

	fmt.Printf("Created default compliance policy for tenant %s: %s\n", tenantID, policy.ID)
	return policy.ID, nil
}
