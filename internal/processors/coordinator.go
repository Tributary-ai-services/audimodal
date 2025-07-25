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
	"github.com/jscharber/eAIIngest/pkg/readers"
	"github.com/jscharber/eAIIngest/pkg/strategies"
)

// Coordinator provides a high-level interface for orchestrating file processing
type Coordinator struct {
	db             *database.Database
	pipeline       *Pipeline
	sessionManager *SessionManager
	config         *CoordinatorConfig
	mu             sync.RWMutex
}

// CoordinatorConfig contains configuration for the processing coordinator
type CoordinatorConfig struct {
	DefaultPipelineConfig *PipelineConfig `json:"default_pipeline_config"`
	AutoRegisterPlugins   bool            `json:"auto_register_plugins"`
	EnableBackgroundJobs  bool            `json:"enable_background_jobs"`
	HealthCheckInterval   time.Duration   `json:"health_check_interval"`
	CleanupInterval       time.Duration   `json:"cleanup_interval"`
	MaxRetentionDays      int             `json:"max_retention_days"`
}

// ProcessingJob represents a high-level processing job
type ProcessingJob struct {
	ID              uuid.UUID              `json:"id"`
	TenantID        uuid.UUID              `json:"tenant_id"`
	Type            string                 `json:"type"`
	Status          string                 `json:"status"`
	CreatedAt       time.Time              `json:"created_at"`
	StartedAt       *time.Time             `json:"started_at,omitempty"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
	Priority        string                 `json:"priority"`
	Configuration   map[string]any         `json:"configuration"`
	Files           []uuid.UUID            `json:"files"`
	SessionID       *uuid.UUID             `json:"session_id,omitempty"`
	Progress        float64                `json:"progress"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
	Results         *ProcessingJobResults  `json:"results,omitempty"`
}

// ProcessingJobResults contains the aggregated results of a processing job
type ProcessingJobResults struct {
	FilesProcessed     int                  `json:"files_processed"`
	FilesFailed        int                  `json:"files_failed"`
	ChunksCreated      int                  `json:"chunks_created"`
	BytesProcessed     int64                `json:"bytes_processed"`
	AverageQuality     float64              `json:"average_quality"`
	ProcessingTime     time.Duration        `json:"processing_time"`
	FileResults        []*ProcessingResult  `json:"file_results,omitempty"`
}

// NewCoordinator creates a new processing coordinator
func NewCoordinator(db *database.Database, config *CoordinatorConfig) *Coordinator {
	if config == nil {
		config = GetDefaultCoordinatorConfig()
	}

	pipeline := NewPipeline(db, config.DefaultPipelineConfig)
	sessionManager := NewSessionManager(db, pipeline)

	coordinator := &Coordinator{
		db:             db,
		pipeline:       pipeline,
		sessionManager: sessionManager,
		config:         config,
	}

	// Register plugins if auto-registration is enabled
	if config.AutoRegisterPlugins {
		coordinator.registerPlugins()
	}

	// Start background jobs if enabled
	if config.EnableBackgroundJobs {
		go coordinator.startBackgroundJobs()
	}

	return coordinator
}

// GetDefaultCoordinatorConfig returns default coordinator configuration
func GetDefaultCoordinatorConfig() *CoordinatorConfig {
	return &CoordinatorConfig{
		DefaultPipelineConfig: GetDefaultPipelineConfig(),
		AutoRegisterPlugins:   true,
		EnableBackgroundJobs:  true,
		HealthCheckInterval:   30 * time.Second,
		CleanupInterval:       1 * time.Hour,
		MaxRetentionDays:      30,
	}
}

// ProcessSingleFile processes a single file immediately
func (c *Coordinator) ProcessSingleFile(ctx context.Context, tenantID uuid.UUID, fileID uuid.UUID, options map[string]any) (*ProcessingResult, error) {
	// Get file information
	var file models.File
	if err := c.db.DB().Where("id = ? AND tenant_id = ?", fileID, tenantID).First(&file).Error; err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}

	// Create processing request
	request := &ProcessingRequest{
		TenantID:  tenantID,
		SessionID: uuid.New(), // Create a temporary session
		FileID:    fileID,
		FilePath:  file.Path,
		Priority:  "normal",
	}

	// Apply options
	if options != nil {
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

	// Process file through pipeline
	return c.pipeline.ProcessFile(ctx, request)
}

// ProcessMultipleFiles processes multiple files concurrently
func (c *Coordinator) ProcessMultipleFiles(ctx context.Context, tenantID uuid.UUID, fileIDs []uuid.UUID, options map[string]any) (*ProcessingJob, error) {
	// Create processing job
	job := &ProcessingJob{
		ID:        uuid.New(),
		TenantID:  tenantID,
		Type:      "batch_processing",
		Status:    "created",
		CreatedAt: time.Now(),
		Priority:  "normal",
		Files:     fileIDs,
		Configuration: options,
	}

	if options != nil {
		if priority, ok := options["priority"].(string); ok {
			job.Priority = priority
		}
	}

	// Create session for this job
	sessionConfig := &SessionConfig{
		Priority:           job.Priority,
		MaxConcurrentFiles: c.config.DefaultPipelineConfig.MaxConcurrentFiles,
		RetryEnabled:       true,
		RetryAttempts:      c.config.DefaultPipelineConfig.RetryAttempts,
		DLPScanEnabled:     true,
		ProcessingOptions:  options,
	}

	session, err := c.sessionManager.CreateSession(ctx, tenantID, sessionConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	job.SessionID = &session.SessionID

	// Add files to session
	if err := c.sessionManager.AddFilesToSession(ctx, session.SessionID, fileIDs); err != nil {
		return nil, fmt.Errorf("failed to add files to session: %w", err)
	}

	// Start processing asynchronously
	go func() {
		if err := c.sessionManager.StartSession(context.Background(), session.SessionID); err != nil {
			// Log error in production
			job.Status = "failed"
			job.ErrorMessage = err.Error()
		}
	}()

	job.Status = "running"
	return job, nil
}

// ProcessDataSource processes all files from a data source
func (c *Coordinator) ProcessDataSource(ctx context.Context, tenantID uuid.UUID, dataSourceID uuid.UUID, options map[string]any) (*ProcessingJob, error) {
	// Get all files from the data source
	tenantService := c.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant repository: %w", err)
	}

	var files []models.File
	if err := tenantRepo.DB().Where("data_source_id = ? AND status = ?", dataSourceID, models.FileStatusDiscovered).Find(&files).Error; err != nil {
		return nil, fmt.Errorf("failed to get files from data source: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no files found in data source")
	}

	// Extract file IDs
	fileIDs := make([]uuid.UUID, len(files))
	for i, file := range files {
		fileIDs[i] = file.ID
	}

	// Process as batch
	job, err := c.ProcessMultipleFiles(ctx, tenantID, fileIDs, options)
	if err != nil {
		return nil, err
	}

	job.Type = "data_source_processing"
	return job, nil
}

// GetJobStatus returns the current status of a processing job
func (c *Coordinator) GetJobStatus(jobID uuid.UUID) (*ProcessingJob, error) {
	// For now, we'll simulate job tracking
	// In a full implementation, this would query a job database
	return nil, fmt.Errorf("job tracking not implemented")
}

// GetSessionStatus returns the current status of a processing session
func (c *Coordinator) GetSessionStatus(sessionID uuid.UUID) (*SessionProgress, error) {
	return c.sessionManager.GetSessionProgress(sessionID)
}

// ListActiveSessions returns all active processing sessions
func (c *Coordinator) ListActiveSessions() []*SessionContext {
	return c.sessionManager.ListActiveSessions()
}

// StopSession stops a running processing session
func (c *Coordinator) StopSession(ctx context.Context, sessionID uuid.UUID) error {
	return c.sessionManager.StopSession(ctx, sessionID)
}

// GetProcessingMetrics returns current processing metrics
func (c *Coordinator) GetProcessingMetrics() *PipelineMetrics {
	return c.pipeline.GetMetrics()
}

// ValidateFileForProcessing checks if a file is ready for processing
func (c *Coordinator) ValidateFileForProcessing(ctx context.Context, tenantID uuid.UUID, fileID uuid.UUID) error {
	var file models.File
	if err := c.db.DB().Where("id = ? AND tenant_id = ?", fileID, tenantID).First(&file).Error; err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	// Check file status
	if file.Status != models.FileStatusDiscovered {
		return fmt.Errorf("file is not in discoverable state: %s", file.Status)
	}

	// Check if file path exists and is accessible
	if file.Path == "" {
		return fmt.Errorf("file path is empty")
	}

	// Check if we have a suitable reader
	ext := filepath.Ext(file.Path)
	readerType := readers.GetReaderForExtension(ext)
	if readerType == "" {
		return fmt.Errorf("no suitable reader found for file extension: %s", ext)
	}

	return nil
}

// RecommendProcessingOptions returns recommended processing options for a file
func (c *Coordinator) RecommendProcessingOptions(ctx context.Context, tenantID uuid.UUID, fileID uuid.UUID) (map[string]any, error) {
	var file models.File
	if err := c.db.DB().Where("id = ? AND tenant_id = ?", fileID, tenantID).First(&file).Error; err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}

	options := make(map[string]any)

	// Recommend reader based on file extension
	ext := filepath.Ext(file.Path)
	readerType := readers.GetReaderForExtension(ext)
	if readerType != "" {
		options["reader_type"] = readerType
	}

	// Recommend strategy based on file characteristics
	if file.ContentType != "" {
		if file.ContentType == "text/csv" {
			options["strategy_type"] = "row_based_structured"
		} else if file.ContentType == "application/json" {
			options["strategy_type"] = "adaptive_hybrid"
		} else if strings.HasPrefix(file.ContentType, "text/") {
			options["strategy_type"] = "semantic_text"
		}
	}

	// Recommend configuration based on file size
	if file.Size > 0 {
		if file.Size < 1024*1024 { // < 1MB
			options["chunk_size"] = 500
		} else if file.Size > 100*1024*1024 { // > 100MB
			options["chunk_size"] = 2000
			options["rows_per_chunk"] = 50
		}
	}

	// Add default DLP scanning
	options["dlp_scan_enabled"] = true

	return options, nil
}

// EstimateProcessingTime estimates how long it will take to process files
func (c *Coordinator) EstimateProcessingTime(ctx context.Context, tenantID uuid.UUID, fileIDs []uuid.UUID) (time.Duration, error) {
	var totalSize int64
	var fileCount int

	tenantService := c.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, tenantID)
	if err != nil {
		return 0, fmt.Errorf("failed to get tenant repository: %w", err)
	}

	for _, fileID := range fileIDs {
		var file models.File
		if err := tenantRepo.DB().Where("id = ?", fileID).First(&file).Error; err != nil {
			continue // Skip missing files
		}
		totalSize += file.Size
		fileCount++
	}

	if fileCount == 0 {
		return 0, fmt.Errorf("no valid files found")
	}

	// Simple estimation based on historical averages
	metrics := c.pipeline.GetMetrics()
	
	var estimatedTime time.Duration
	if metrics.AverageFileTime > 0 {
		// Use historical average
		estimatedTime = time.Duration(float64(fileCount) * metrics.AverageFileTime * float64(time.Millisecond))
	} else {
		// Use simple heuristic: ~1MB per second
		bytesPerSecond := int64(1024 * 1024)
		estimatedTime = time.Duration(totalSize/bytesPerSecond) * time.Second
	}

	// Add overhead for parallelization
	maxConcurrent := c.config.DefaultPipelineConfig.MaxConcurrentFiles
	if fileCount > maxConcurrent {
		parallelFactor := float64(fileCount) / float64(maxConcurrent)
		estimatedTime = time.Duration(float64(estimatedTime) * parallelFactor * 0.8) // 80% efficiency
	}

	return estimatedTime, nil
}

// registerPlugins registers all available readers and strategies
func (c *Coordinator) registerPlugins() {
	// Register readers
	readers.RegisterBasicReaders()
	
	// Register strategies
	strategies.RegisterBasicStrategies()
}

// startBackgroundJobs starts background maintenance jobs
func (c *Coordinator) startBackgroundJobs() {
	// Health check ticker
	healthTicker := time.NewTicker(c.config.HealthCheckInterval)
	cleanupTicker := time.NewTicker(c.config.CleanupInterval)

	go func() {
		for {
			select {
			case <-healthTicker.C:
				c.performHealthCheck()
			case <-cleanupTicker.C:
				c.performCleanup()
			}
		}
	}()
}

// performHealthCheck performs system health checks
func (c *Coordinator) performHealthCheck() {
	// Check database connectivity
	ctx := context.Background()
	if err := c.db.HealthCheck(ctx); err != nil {
		// Log error in production
		return
	}

	// Check plugin registry
	readers := c.pipeline.registry.ListReaders()
	strategies := c.pipeline.registry.ListStrategies()
	
	if len(readers) == 0 || len(strategies) == 0 {
		// Log warning in production
		return
	}

	// Additional health checks can be added here
}

// performCleanup performs system cleanup tasks
func (c *Coordinator) performCleanup() {
	ctx := context.Background()

	// Clean up old completed sessions
	cutoffTime := time.Now().AddDate(0, 0, -c.config.MaxRetentionDays)

	// This would clean up old session records in a full implementation
	_ = cutoffTime
	_ = ctx

	// Additional cleanup tasks can be added here
}

// GetSystemStatus returns overall system status
func (c *Coordinator) GetSystemStatus() map[string]any {
	metrics := c.pipeline.GetMetrics()
	activeSessions := c.sessionManager.ListActiveSessions()
	
	readerCount, strategyCount, _ := c.pipeline.registry.Count()

	status := map[string]any{
		"status": "healthy",
		"metrics": map[string]any{
			"files_processed":    metrics.FilesProcessed,
			"chunks_created":     metrics.ChunksCreated,
			"bytes_processed":    metrics.BytesProcessed,
			"error_count":        metrics.ErrorCount,
			"average_file_time":  metrics.AverageFileTime,
			"average_chunk_time": metrics.AverageChunkTime,
			"last_processed_at":  metrics.LastProcessedAt,
		},
		"active_sessions": len(activeSessions),
		"plugins": map[string]any{
			"readers":    readerCount,
			"strategies": strategyCount,
		},
		"configuration": map[string]any{
			"max_concurrent_files":  c.config.DefaultPipelineConfig.MaxConcurrentFiles,
			"max_concurrent_chunks": c.config.DefaultPipelineConfig.MaxConcurrentChunks,
			"processing_timeout":    c.config.DefaultPipelineConfig.ProcessingTimeout,
		},
	}

	return status
}