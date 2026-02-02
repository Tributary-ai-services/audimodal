package processors

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/jscharber/audimodal/internal/database"
	"github.com/jscharber/audimodal/internal/database/models"
	"github.com/jscharber/audimodal/pkg/dlp"
	"github.com/jscharber/audimodal/pkg/dlp/scanner"
	"github.com/jscharber/audimodal/pkg/dlp/types"
	"github.com/jscharber/audimodal/pkg/embeddings"
)

// ProcessingTier defines different processing tiers based on file size and complexity
type ProcessingTier int

const (
	TierSmall  ProcessingTier = iota // < 10MB - Fast processing, minimal resources
	TierMedium                       // 10MB - 1GB - Standard processing with batching
	TierLarge                        // > 1GB - Heavy processing with streaming and checkpoints
)

// TierProcessor handles tier-based processing with different strategies for different file sizes
type TierProcessor struct {
	db               *database.Database
	pipeline         *Pipeline
	embeddingService embeddings.EmbeddingService
	dlpService       *dlp.DLPService
	config           *TierProcessorConfig
}

// TierProcessorConfig contains configuration for tier-based processing
type TierProcessorConfig struct {
	SmallFileTierThreshold int64         `json:"small_file_tier_threshold"` // 10MB
	LargeFileTierThreshold int64         `json:"large_file_tier_threshold"` // 1GB
	SmallFileTimeout       time.Duration `json:"small_file_timeout"`        // 5 minutes
	MediumFileTimeout      time.Duration `json:"medium_file_timeout"`       // 30 minutes
	LargeFileTimeout       time.Duration `json:"large_file_timeout"`        // 2 hours
	CheckpointInterval     time.Duration `json:"checkpoint_interval"`       // 5 minutes
	EnableEmbeddings       bool          `json:"enable_embeddings"`
	EmbeddingBatchSize     int           `json:"embedding_batch_size"`
	EmbeddingDataset       string        `json:"embedding_dataset"`
	EnableDLP              bool          `json:"enable_dlp"`
	DLPBatchSize           int           `json:"dlp_batch_size"`
}

// TierProcessingResult contains results with tier-specific metrics
type TierProcessingResult struct {
	*ProcessingResult
	Tier               ProcessingTier   `json:"tier"`
	EmbeddingsCreated  int              `json:"embeddings_created"`
	EmbeddingTime      time.Duration    `json:"embedding_time"`
	DLPViolationsFound int              `json:"dlp_violations_found"`
	DLPScanTime        time.Duration    `json:"dlp_scan_time"`
	CheckpointsCreated int              `json:"checkpoints_created"`
	ResourcesUsed      *ResourceMetrics `json:"resources_used"`
}

// ResourceMetrics tracks resource usage during processing
type ResourceMetrics struct {
	PeakMemoryMB   int64   `json:"peak_memory_mb"`
	CPUTimeSeconds float64 `json:"cpu_time_seconds"`
	DiskIOBytes    int64   `json:"disk_io_bytes"`
	NetworkIOBytes int64   `json:"network_io_bytes"`
}

// NewTierProcessor creates a new tier-based processor
func NewTierProcessor(db *database.Database, pipeline *Pipeline, embeddingService embeddings.EmbeddingService, config *TierProcessorConfig) *TierProcessor {
	if config == nil {
		config = GetDefaultTierProcessorConfig()
	}

	// Initialize DLP service if DLP is enabled
	var dlpService *dlp.DLPService
	if config.EnableDLP {
		dlpService = dlp.NewDLPService(nil) // Use default DLP config
		fmt.Printf("DEBUG: TierProcessor created with DLP service enabled\n")
	} else {
		fmt.Printf("DEBUG: TierProcessor created with DLP DISABLED (config.EnableDLP=%v)\n", config.EnableDLP)
	}

	return &TierProcessor{
		db:               db,
		pipeline:         pipeline,
		embeddingService: embeddingService,
		dlpService:       dlpService,
		config:           config,
	}
}

// SetDLPService allows setting a custom DLP service
func (tp *TierProcessor) SetDLPService(dlpService *dlp.DLPService) {
	tp.dlpService = dlpService
}

// GetDefaultTierProcessorConfig returns default tier processor configuration
func GetDefaultTierProcessorConfig() *TierProcessorConfig {
	return &TierProcessorConfig{
		SmallFileTierThreshold: 10 * 1024 * 1024,   // 10MB
		LargeFileTierThreshold: 1024 * 1024 * 1024, // 1GB
		SmallFileTimeout:       5 * time.Minute,
		MediumFileTimeout:      30 * time.Minute,
		LargeFileTimeout:       2 * time.Hour,
		CheckpointInterval:     5 * time.Minute,
		EnableEmbeddings:       true,
		EmbeddingBatchSize:     50,
		EmbeddingDataset:       "default",
		EnableDLP:              true, // Enable DLP scanning by default
		DLPBatchSize:           20,
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

	// Post-process: perform DLP scanning if enabled (either by config or request flag)
	dlpEnabled := tp.config.EnableDLP || request.DLPScanEnabled
	fmt.Printf("DEBUG DLP: config.EnableDLP=%v, request.DLPScanEnabled=%v, dlpEnabled=%v, dlpService=%v, status=%s\n",
		tp.config.EnableDLP, request.DLPScanEnabled, dlpEnabled, tp.dlpService != nil, result.ProcessingResult.Status)
	if dlpEnabled && tp.dlpService != nil && result.ProcessingResult.Status == "completed" {
		dlpStartTime := time.Now()
		violationsFound, err := tp.performDLPScan(tierCtx, request, result)
		if err != nil {
			// Log error but don't fail the entire processing
			if result.ProcessingResult.Metadata == nil {
				result.ProcessingResult.Metadata = map[string]any{}
			}
			result.ProcessingResult.Metadata["dlp_error"] = err.Error()
			fmt.Printf("WARNING: DLP scan failed for file %s: %v\n", request.FileID.String(), err)
		} else {
			result.DLPViolationsFound = violationsFound
			fmt.Printf("INFO: DLP scan completed for file %s: %d violations found\n", request.FileID.String(), violationsFound)
		}
		result.DLPScanTime = time.Since(dlpStartTime)
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

	// Get the file to check for Neo4j Document ID for cross-service consistency
	var file models.File
	if err := tenantRepo.DB().Where("id = ?", request.FileID).First(&file).Error; err != nil {
		return 0, fmt.Errorf("failed to get file: %w", err)
	}

	// Use Neo4j Document ID if available, otherwise fall back to AudiModal file ID
	// This ensures embeddings in DeepLake use the same document ID as Neo4j (Aether-BE)
	documentID := request.FileID.String()
	if file.Neo4jDocumentID != nil && *file.Neo4jDocumentID != "" {
		documentID = *file.Neo4jDocumentID
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
				DocumentID:  documentID, // Use Neo4j Document ID for cross-service consistency
				Content:     chunk.Content,
				ChunkIndex:  chunk.ChunkNumber,
				ContentType: chunk.ChunkType,
				Metadata: map[string]interface{}{
					"file_id":           chunk.FileID.String(), // Keep AudiModal file ID in metadata for reference
					"neo4j_document_id": documentID,            // Explicit Neo4j Document ID for cross-service queries
					"chunk_number":      chunk.ChunkNumber,
					"size_bytes":        chunk.SizeBytes,
					"processed_at":      chunk.ProcessedAt,
					"language":          chunk.Language,
					"quality_score":     chunk.Quality.Completeness,
				},
			}
		}

		// Process chunk batch
		embeddingRequest := &embeddings.ProcessChunksRequest{
			Chunks:    chunkInputs,
			Dataset:   tp.config.EmbeddingDataset,
			TenantID:  request.TenantID,
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

// performDLPScan scans all chunks of a processed file for PII and creates violations
func (tp *TierProcessor) performDLPScan(ctx context.Context, request *ProcessingRequest, result *TierProcessingResult) (int, error) {
	if tp.dlpService == nil {
		return 0, fmt.Errorf("DLP service not configured")
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
		return 0, nil // No chunks to scan
	}

	// Build compliance rules from request or use defaults
	// These map to the regulations defined in pkg/dlp/compliance/checker.go
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
		// GDPR, HIPAA, PCI-DSS, CCPA are implemented in pkg/dlp/compliance/checker.go
		complianceRules = []types.ComplianceRule{
			{Regulation: "GDPR", Required: true},
			{Regulation: "HIPAA", Required: true},
			{Regulation: "PCI-DSS", Required: true},
			{Regulation: "CCPA", Required: true},
		}
	}

	totalViolations := 0
	batchSize := tp.config.DLPBatchSize
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
				Content:         chunk.Content,
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

		batchResponse, err := tp.dlpService.ScanBatch(ctx, batchRequest)
		if err != nil {
			fmt.Printf("WARNING: DLP batch scan failed for file %s batch %d: %v\n", request.FileID.String(), i/batchSize, err)
			continue
		}

		// Process results and create violations
		for chunkID, scanResponse := range batchResponse.Results {
			// Update chunk DLP status
			chunkUpdates := map[string]interface{}{
				"dlp_scan_status": models.DLPScanStatusCompleted,
			}
			if scanResponse.ScanResult != nil {
				chunkUpdates["dlp_finding_count"] = scanResponse.ScanResult.TotalMatches
				chunkUpdates["dlp_risk_score"] = scanResponse.ScanResult.RiskScore
			}
			now := time.Now()
			chunkUpdates["dlp_scanned_at"] = &now

			// Apply redaction if enabled and there are findings to redact
			if request.RedactionMode != "" && request.RedactionMode != types.RedactionNone &&
				scanResponse.ScanResult != nil && scanResponse.ScanResult.TotalMatches > 0 {

				// Find original chunk content
				var originalContent string
				for _, c := range batch {
					if c.ChunkID == chunkID {
						originalContent = c.Content
						break
					}
				}

				if originalContent != "" {
					redactionEngine := scanner.NewBasicRedactionEngine()
					redactedContent, redactErr := redactionEngine.RedactContent(originalContent, scanResponse.ScanResult, request.RedactionMode)
					if redactErr != nil {
						fmt.Printf("WARNING: Failed to redact chunk %s: %v\n", chunkID, redactErr)
					} else if redactedContent != originalContent {
						chunkUpdates["content"] = redactedContent
						chunkUpdates["redaction_applied"] = true
						chunkUpdates["redaction_mode"] = string(request.RedactionMode)
						fmt.Printf("DEBUG: Redacted %d findings in chunk %s using %s mode\n",
							scanResponse.ScanResult.TotalMatches, chunkID, request.RedactionMode)
					}
				}
			}

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
					policyID, policyErr := tp.getOrCreateCompliancePolicy(tenantRepo, request.TenantID)
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
func (tp *TierProcessor) getOrCreateCompliancePolicy(tenantRepo *database.TenantRepository, tenantID uuid.UUID) (uuid.UUID, error) {
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

// GetTierMetrics returns metrics for each processing tier
func (tp *TierProcessor) GetTierMetrics() map[ProcessingTier]*TierMetrics {
	// In a full implementation, this would track metrics per tier
	return map[ProcessingTier]*TierMetrics{
		TierSmall: {
			FilesProcessed:  100,
			AverageTime:     2 * time.Second,
			SuccessRate:     0.98,
			AverageFileSize: 1024 * 1024, // 1MB
		},
		TierMedium: {
			FilesProcessed:  50,
			AverageTime:     30 * time.Second,
			SuccessRate:     0.95,
			AverageFileSize: 50 * 1024 * 1024, // 50MB
		},
		TierLarge: {
			FilesProcessed:  10,
			AverageTime:     10 * time.Minute,
			SuccessRate:     0.90,
			AverageFileSize: 500 * 1024 * 1024, // 500MB
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
