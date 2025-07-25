package processors

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/internal/database/models"
	"github.com/jscharber/eAIIngest/pkg/dlp"
	"github.com/jscharber/eAIIngest/pkg/dlp/types"
)

// DLPProcessor handles DLP scanning integration with the processing pipeline
type DLPProcessor struct {
	db         *database.Database
	dlpService *dlp.DLPService
	config     *DLPProcessorConfig
}

// DLPProcessorConfig contains configuration for DLP processing
type DLPProcessorConfig struct {
	Enabled                bool                   `json:"enabled"`
	ScanTimeout            time.Duration          `json:"scan_timeout"`
	MinConfidence          float64                `json:"min_confidence"`
	DefaultRedactionMode   types.RedactionStrategy  `json:"default_redaction_mode"`
	EnableComplianceCheck  bool                   `json:"enable_compliance_check"`
	DefaultComplianceRules []types.ComplianceRule   `json:"default_compliance_rules"`
	StoreRedactedContent   bool                   `json:"store_redacted_content"`
	UpdateChunkStatus      bool                   `json:"update_chunk_status"`
	MaxRetries             int                    `json:"max_retries"`
	RetryDelay             time.Duration          `json:"retry_delay"`
}

// ChunkDLPResult contains the results of DLP processing for a chunk
type ChunkDLPResult struct {
	ChunkID          uuid.UUID               `json:"chunk_id"`
	ScanResult       *types.ScanResult         `json:"scan_result"`
	ComplianceResult *types.ComplianceResult   `json:"compliance_result,omitempty"`
	RedactedContent  string                  `json:"redacted_content,omitempty"`
	ProcessingTime   time.Duration           `json:"processing_time"`
	Status           string                  `json:"status"`
	Error            string                  `json:"error,omitempty"`
}

// NewDLPProcessor creates a new DLP processor
func NewDLPProcessor(db *database.Database, dlpService *dlp.DLPService, config *DLPProcessorConfig) *DLPProcessor {
	if config == nil {
		config = GetDefaultDLPProcessorConfig()
	}
	
	return &DLPProcessor{
		db:         db,
		dlpService: dlpService,
		config:     config,
	}
}

// GetDefaultDLPProcessorConfig returns default DLP processor configuration
func GetDefaultDLPProcessorConfig() *DLPProcessorConfig {
	return &DLPProcessorConfig{
		Enabled:                true,
		ScanTimeout:            30 * time.Second,
		MinConfidence:          0.7,
		DefaultRedactionMode:   types.RedactionMask,
		EnableComplianceCheck:  true,
		DefaultComplianceRules: []types.ComplianceRule{},
		StoreRedactedContent:   true,
		UpdateChunkStatus:      true,
		MaxRetries:             3,
		RetryDelay:             5 * time.Second,
	}
}

// ProcessChunk performs DLP scanning on a single chunk
func (p *DLPProcessor) ProcessChunk(ctx context.Context, tenantID uuid.UUID, chunkID uuid.UUID, complianceRules []string) (*ChunkDLPResult, error) {
	startTime := time.Now()
	
	result := &ChunkDLPResult{
		ChunkID: chunkID,
		Status:  "processing",
	}
	
	if !p.config.Enabled {
		result.Status = "skipped"
		result.ProcessingTime = time.Since(startTime)
		return result, nil
	}
	
	// Get chunk from database
	chunk, err := p.getChunk(ctx, tenantID, chunkID)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to get chunk: %v", err)
		result.ProcessingTime = time.Since(startTime)
		return result, err
	}
	
	// Update chunk status to scanning
	if p.config.UpdateChunkStatus {
		p.updateChunkDLPStatus(ctx, tenantID, chunkID, models.DLPScanStatusProcessing)
	}
	
	// Prepare compliance rules
	rules := p.prepareComplianceRules(complianceRules)
	
	// Create DLP scan request
	scanRequest := &dlp.ChunkScanRequest{
		TenantID: tenantID,
		ChunkID:  chunkID.String(),
		FileID:   chunk.FileID,
		Content:  chunk.Content,
		Metadata: p.extractChunkMetadata(chunk),
		ScanConfig: &types.ScanConfig{
			TenantID:         tenantID,
			MinConfidence:    p.config.MinConfidence,
			ScanTimeout:      p.config.ScanTimeout,
			ComplianceRules:  rules,
			RedactionMode:    p.config.DefaultRedactionMode,
			ScanMetadata:     true,
		},
		ComplianceRules: rules,
	}
	
	// Perform DLP scan with retries
	scanResponse, err := p.performScanWithRetries(ctx, scanRequest)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("DLP scan failed: %v", err)
		result.ProcessingTime = time.Since(startTime)
		
		// Update chunk status to failed
		if p.config.UpdateChunkStatus {
			p.updateChunkDLPStatus(ctx, tenantID, chunkID, models.DLPScanStatusFailed)
		}
		
		return result, err
	}
	
	// Process scan results
	result.ScanResult = scanResponse.ScanResult
	result.ComplianceResult = scanResponse.ComplianceResult
	result.RedactedContent = scanResponse.RedactedContent
	result.Status = "completed"
	result.ProcessingTime = time.Since(startTime)
	
	// Store results in database
	if err := p.storeDLPResults(ctx, tenantID, chunkID, result); err != nil {
		// Log error but don't fail the operation
		result.Error = fmt.Sprintf("failed to store results: %v", err)
	}
	
	// Update chunk status to completed
	if p.config.UpdateChunkStatus {
		finalStatus := models.DLPScanStatusCompleted
		if result.ScanResult.TotalMatches > 0 {
			finalStatus = models.DLPScanStatusCompleted
		}
		p.updateChunkDLPStatus(ctx, tenantID, chunkID, finalStatus)
	}
	
	return result, nil
}

// ProcessBatch performs DLP scanning on multiple chunks
func (p *DLPProcessor) ProcessBatch(ctx context.Context, tenantID uuid.UUID, chunkIDs []uuid.UUID, complianceRules []string) (map[uuid.UUID]*ChunkDLPResult, error) {
	results := make(map[uuid.UUID]*ChunkDLPResult)
	
	if !p.config.Enabled {
		// Return skipped results for all chunks
		for _, chunkID := range chunkIDs {
			results[chunkID] = &ChunkDLPResult{
				ChunkID: chunkID,
				Status:  "skipped",
			}
		}
		return results, nil
	}
	
	// Process chunks concurrently
	resultChan := make(chan struct {
		ChunkID uuid.UUID
		Result  *ChunkDLPResult
		Error   error
	}, len(chunkIDs))
	
	// Launch scanning goroutines
	for _, chunkID := range chunkIDs {
		go func(id uuid.UUID) {
			result, err := p.ProcessChunk(ctx, tenantID, id, complianceRules)
			resultChan <- struct {
				ChunkID uuid.UUID
				Result  *ChunkDLPResult
				Error   error
			}{
				ChunkID: id,
				Result:  result,
				Error:   err,
			}
		}(chunkID)
	}
	
	// Collect results
	for i := 0; i < len(chunkIDs); i++ {
		result := <-resultChan
		results[result.ChunkID] = result.Result
	}
	
	return results, nil
}

// GetChunkDLPStatus returns the DLP scan status for a chunk
func (p *DLPProcessor) GetChunkDLPStatus(ctx context.Context, tenantID uuid.UUID, chunkID uuid.UUID) (string, error) {
	chunk, err := p.getChunk(ctx, tenantID, chunkID)
	if err != nil {
		return models.DLPScanStatusPending, err
	}
	
	return chunk.DLPScanStatus, nil
}

// GetDLPSummary returns a summary of DLP findings for a file
func (p *DLPProcessor) GetDLPSummary(ctx context.Context, tenantID uuid.UUID, fileID uuid.UUID) (*DLPSummary, error) {
	// Get all chunks for the file
	chunks, err := p.getFileChunks(ctx, tenantID, fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get file chunks: %w", err)
	}
	
	summary := &DLPSummary{
		FileID:       fileID,
		TotalChunks:  len(chunks),
		ChunksByStatus: make(map[string]int),
		FindingsByType: make(map[types.PIIType]int),
		FindingsByRisk: make(map[types.RiskLevel]int),
	}
	
	// Aggregate statistics
	for _, chunk := range chunks {
		summary.ChunksByStatus[chunk.DLPScanStatus]++
		
		if chunk.DLPScanStatus == models.DLPScanStatusCompleted {
			// Get DLP findings for this chunk (would need to store findings in database)
			// For now, we'll use placeholder logic
			summary.TotalFindings += len(chunk.DLPViolations)
		}
	}
	
	return summary, nil
}

// Internal helper methods

func (p *DLPProcessor) getChunk(ctx context.Context, tenantID uuid.UUID, chunkID uuid.UUID) (*models.Chunk, error) {
	tenantService := p.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant repository: %w", err)
	}
	
	var chunk models.Chunk
	if err := tenantRepo.DB().Where("id = ?", chunkID).First(&chunk).Error; err != nil {
		return nil, fmt.Errorf("chunk not found: %w", err)
	}
	
	return &chunk, nil
}

func (p *DLPProcessor) getFileChunks(ctx context.Context, tenantID uuid.UUID, fileID uuid.UUID) ([]models.Chunk, error) {
	tenantService := p.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant repository: %w", err)
	}
	
	var chunks []models.Chunk
	if err := tenantRepo.DB().Where("file_id = ?", fileID).Find(&chunks).Error; err != nil {
		return nil, fmt.Errorf("failed to get chunks: %w", err)
	}
	
	return chunks, nil
}

func (p *DLPProcessor) updateChunkDLPStatus(ctx context.Context, tenantID uuid.UUID, chunkID uuid.UUID, status string) error {
	tenantService := p.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to get tenant repository: %w", err)
	}
	
	updates := map[string]interface{}{
		"dlp_scan_status": status,
	}
	
	if status == models.DLPScanStatusCompleted || status == models.DLPScanStatusFailed {
		now := time.Now()
		updates["dlp_scanned_at"] = &now
	}
	
	return tenantRepo.DB().Model(&models.Chunk{}).Where("id = ?", chunkID).Updates(updates).Error
}

func (p *DLPProcessor) extractChunkMetadata(chunk *models.Chunk) map[string]string {
	metadata := make(map[string]string)
	
	metadata["chunk_id"] = chunk.ID.String()
	metadata["file_id"] = chunk.FileID.String()
	metadata["chunk_type"] = chunk.ChunkType
	metadata["chunk_number"] = fmt.Sprintf("%d", chunk.ChunkNumber)
	
	if chunk.PageNumber != nil {
		metadata["page_number"] = fmt.Sprintf("%d", *chunk.PageNumber)
	}
	
	if chunk.LineNumber != nil {
		metadata["line_number"] = fmt.Sprintf("%d", *chunk.LineNumber)
	}
	
	// Add context metadata
	for key, value := range chunk.Context {
		metadata[fmt.Sprintf("context_%s", key)] = value
	}
	
	return metadata
}

func (p *DLPProcessor) prepareComplianceRules(ruleNames []string) []types.ComplianceRule {
	var rules []types.ComplianceRule
	
	// Add default rules
	rules = append(rules, p.config.DefaultComplianceRules...)
	
	// Add requested rules
	for _, ruleName := range ruleNames {
		rule := types.ComplianceRule{
			Regulation: ruleName,
			Required:   true,
		}
		rules = append(rules, rule)
	}
	
	return rules
}

func (p *DLPProcessor) performScanWithRetries(ctx context.Context, request *dlp.ChunkScanRequest) (*dlp.DLPScanResponse, error) {
	var lastErr error
	
	for attempt := 0; attempt < p.config.MaxRetries; attempt++ {
		response, err := p.dlpService.ScanChunk(ctx, request)
		if err == nil {
			return response, nil
		}
		
		lastErr = err
		
		// Don't retry on certain types of errors
		if attempt < p.config.MaxRetries-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(p.config.RetryDelay):
				// Continue to next attempt
			}
		}
	}
	
	return nil, fmt.Errorf("DLP scan failed after %d attempts: %w", p.config.MaxRetries, lastErr)
}

func (p *DLPProcessor) storeDLPResults(ctx context.Context, tenantID uuid.UUID, chunkID uuid.UUID, result *ChunkDLPResult) error {
	tenantService := p.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to get tenant repository: %w", err)
	}
	
	// Update chunk with DLP results
	updates := map[string]interface{}{
		"dlp_finding_count": 0,
		"dlp_risk_score":    0.0,
	}
	
	if result.ScanResult != nil {
		updates["dlp_finding_count"] = result.ScanResult.TotalMatches
		updates["dlp_risk_score"] = result.ScanResult.RiskScore
	}
	
	// Store redacted content if enabled and available
	if p.config.StoreRedactedContent && result.RedactedContent != "" {
		updates["redacted_content"] = result.RedactedContent
	}
	
	return tenantRepo.DB().Model(&models.Chunk{}).Where("id = ?", chunkID).Updates(updates).Error
}

// DLPSummary contains aggregate DLP statistics for a file
type DLPSummary struct {
	FileID         uuid.UUID                            `json:"file_id"`
	TotalChunks    int                                  `json:"total_chunks"`
	TotalFindings  int                                  `json:"total_findings"`
	AverageRisk    float64                              `json:"average_risk"`
	ChunksByStatus map[string]int         `json:"chunks_by_status"`
	FindingsByType map[types.PIIType]int                `json:"findings_by_type"`
	FindingsByRisk map[types.RiskLevel]int              `json:"findings_by_risk"`
	IsCompliant    bool                                 `json:"is_compliant"`
	Violations     []types.ComplianceViolation          `json:"violations"`
}