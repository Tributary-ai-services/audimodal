package dlp

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/jscharber/eAIIngest/pkg/dlp/compliance"
	"github.com/jscharber/eAIIngest/pkg/dlp/scanner"
	"github.com/jscharber/eAIIngest/pkg/dlp/types"
)

// Use interfaces from the main package to avoid import cycles
type Scanner = DLPScanner
type RedactionEngineInterface = RedactionEngine
type ComplianceCheckerInterface = ComplianceChecker

// DLPService provides a high-level interface for DLP operations
type DLPService struct {
	scanner           Scanner
	redactionEngine   RedactionEngineInterface
	complianceChecker ComplianceCheckerInterface
	config            *DLPServiceConfig
}

// DLPServiceConfig contains configuration for the DLP service
type DLPServiceConfig struct {
	DefaultScanTimeout     time.Duration           `json:"default_scan_timeout"`
	DefaultMinConfidence   float64                 `json:"default_min_confidence"`
	DefaultRedactionMode   types.RedactionStrategy `json:"default_redaction_mode"`
	EnableCompliance       bool                    `json:"enable_compliance"`
	DefaultComplianceRules []types.ComplianceRule  `json:"default_compliance_rules"`
	MaxContentLength       int                     `json:"max_content_length"`
	EnableCaching          bool                    `json:"enable_caching"`
	CacheTTL               time.Duration           `json:"cache_ttl"`
}

// DLPScanRequest represents a request to scan content for PII
type DLPScanRequest struct {
	TenantID        uuid.UUID               `json:"tenant_id"`
	ContentID       string                  `json:"content_id"`
	Content         string                  `json:"content"`
	Metadata        map[string]string       `json:"metadata"`
	ScanConfig      *types.ScanConfig       `json:"scan_config,omitempty"`
	ComplianceRules []types.ComplianceRule  `json:"compliance_rules,omitempty"`
	RedactionMode   types.RedactionStrategy `json:"redaction_mode,omitempty"`
}

// DLPScanResponse contains the results of a DLP scan
type DLPScanResponse struct {
	RequestID        string                  `json:"request_id"`
	ScanResult       *types.ScanResult       `json:"scan_result"`
	ComplianceResult *types.ComplianceResult `json:"compliance_result,omitempty"`
	RedactedContent  string                  `json:"redacted_content,omitempty"`
	RedactionMap     map[string]string       `json:"redaction_map,omitempty"`
	ProcessingTime   time.Duration           `json:"processing_time"`
	Recommendations  []string                `json:"recommendations,omitempty"`
}

// ChunkScanRequest represents a request to scan a chunk
type ChunkScanRequest struct {
	TenantID        uuid.UUID              `json:"tenant_id"`
	ChunkID         string                 `json:"chunk_id"`
	FileID          uuid.UUID              `json:"file_id"`
	Content         string                 `json:"content"`
	Metadata        map[string]string      `json:"metadata"`
	ScanConfig      *types.ScanConfig      `json:"scan_config,omitempty"`
	ComplianceRules []types.ComplianceRule `json:"compliance_rules,omitempty"`
}

// BatchScanRequest represents a request to scan multiple chunks
type BatchScanRequest struct {
	TenantID        uuid.UUID              `json:"tenant_id"`
	Chunks          []ChunkScanRequest     `json:"chunks"`
	ScanConfig      *types.ScanConfig      `json:"scan_config,omitempty"`
	ComplianceRules []types.ComplianceRule `json:"compliance_rules,omitempty"`
	MaxConcurrency  int                    `json:"max_concurrency"`
}

// BatchScanResponse contains the results of a batch scan
type BatchScanResponse struct {
	RequestID      string                      `json:"request_id"`
	TotalChunks    int                         `json:"total_chunks"`
	ScannedChunks  int                         `json:"scanned_chunks"`
	FailedChunks   int                         `json:"failed_chunks"`
	Results        map[string]*DLPScanResponse `json:"results"`
	AggregateStats *AggregateStats             `json:"aggregate_stats"`
	ProcessingTime time.Duration               `json:"processing_time"`
}

// AggregateStats contains aggregated statistics from multiple scans
type AggregateStats struct {
	TotalFindings    int                         `json:"total_findings"`
	FindingsByType   map[types.PIIType]int       `json:"findings_by_type"`
	FindingsByRisk   map[types.RiskLevel]int     `json:"findings_by_risk"`
	AverageRiskScore float64                     `json:"average_risk_score"`
	ComplianceStatus map[string]bool             `json:"compliance_status"`
	TopViolations    []types.ComplianceViolation `json:"top_violations"`
}

// NewDLPService creates a new DLP service with default components
func NewDLPService(config *DLPServiceConfig) *DLPService {
	if config == nil {
		config = GetDefaultDLPServiceConfig()
	}

	return &DLPService{
		scanner:           scanner.NewBasicDLPScanner(),
		redactionEngine:   scanner.NewBasicRedactionEngine(),
		complianceChecker: compliance.NewBasicComplianceChecker(),
		config:            config,
	}
}

// NewDLPServiceWithComponents creates a DLP service with custom components
func NewDLPServiceWithComponents(scanner Scanner, redactionEngine RedactionEngineInterface, complianceChecker ComplianceCheckerInterface, config *DLPServiceConfig) *DLPService {
	if config == nil {
		config = GetDefaultDLPServiceConfig()
	}

	return &DLPService{
		scanner:           scanner,
		redactionEngine:   redactionEngine,
		complianceChecker: complianceChecker,
		config:            config,
	}
}

// GetDefaultDLPServiceConfig returns default service configuration
func GetDefaultDLPServiceConfig() *DLPServiceConfig {
	return &DLPServiceConfig{
		DefaultScanTimeout:     30 * time.Second,
		DefaultMinConfidence:   0.7,
		DefaultRedactionMode:   types.RedactionMask,
		EnableCompliance:       true,
		DefaultComplianceRules: []types.ComplianceRule{},
		MaxContentLength:       1024 * 1024, // 1MB
		EnableCaching:          false,
		CacheTTL:               1 * time.Hour,
	}
}

// ScanContent scans content for PII and optionally performs compliance checking and redaction
func (s *DLPService) ScanContent(ctx context.Context, request *DLPScanRequest) (*DLPScanResponse, error) {
	startTime := time.Now()
	requestID := uuid.New().String()

	// Validate request
	if err := s.validateScanRequest(request); err != nil {
		return nil, fmt.Errorf("invalid scan request: %w", err)
	}

	// Prepare scan configuration
	scanConfig := s.prepareScanConfig(request.TenantID, request.ScanConfig)

	// Perform DLP scan
	scanResult, err := s.scanner.ScanContent(ctx, request.Content, scanConfig)
	if err != nil {
		return nil, fmt.Errorf("DLP scan failed: %w", err)
	}

	response := &DLPScanResponse{
		RequestID:       requestID,
		ScanResult:      scanResult,
		ProcessingTime:  time.Since(startTime),
		Recommendations: []string{},
	}

	// Perform compliance checking if enabled
	if s.config.EnableCompliance && len(request.ComplianceRules) > 0 {
		complianceResult, err := s.complianceChecker.CheckCompliance(ctx, scanResult, request.ComplianceRules)
		if err == nil {
			response.ComplianceResult = complianceResult
		}
	}

	// Perform redaction if requested
	redactionMode := request.RedactionMode
	if redactionMode == "" {
		redactionMode = s.config.DefaultRedactionMode
	}

	if redactionMode != types.RedactionNone {
		redactedContent, err := s.redactionEngine.RedactContent(request.Content, scanResult, redactionMode)
		if err == nil {
			response.RedactedContent = redactedContent
			response.RedactionMap = s.redactionEngine.GenerateRedactionMap(scanResult, redactionMode)
		}
	}

	// Generate recommendations
	response.Recommendations = s.generateRecommendations(scanResult, response.ComplianceResult)

	// Update final processing time
	response.ProcessingTime = time.Since(startTime)

	return response, nil
}

// ScanChunk scans a single chunk for PII
func (s *DLPService) ScanChunk(ctx context.Context, request *ChunkScanRequest) (*DLPScanResponse, error) {
	scanRequest := &DLPScanRequest{
		TenantID:        request.TenantID,
		ContentID:       request.ChunkID,
		Content:         request.Content,
		Metadata:        request.Metadata,
		ScanConfig:      request.ScanConfig,
		ComplianceRules: request.ComplianceRules,
	}

	return s.ScanContent(ctx, scanRequest)
}

// ScanBatch scans multiple chunks concurrently
func (s *DLPService) ScanBatch(ctx context.Context, request *BatchScanRequest) (*BatchScanResponse, error) {
	startTime := time.Now()
	requestID := uuid.New().String()

	// Validate request
	if len(request.Chunks) == 0 {
		return nil, fmt.Errorf("no chunks to scan")
	}

	// Set default concurrency
	maxConcurrency := request.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 5
	}

	response := &BatchScanResponse{
		RequestID:   requestID,
		TotalChunks: len(request.Chunks),
		Results:     make(map[string]*DLPScanResponse),
	}

	// Create semaphore for concurrency control
	sem := make(chan struct{}, maxConcurrency)
	results := make(chan chunkScanResult, len(request.Chunks))

	// Launch scan goroutines
	for _, chunkReq := range request.Chunks {
		go func(chunk ChunkScanRequest) {
			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Scan chunk
			scanResp, err := s.ScanChunk(ctx, &chunk)

			results <- chunkScanResult{
				ChunkID:  chunk.ChunkID,
				Response: scanResp,
				Error:    err,
			}
		}(chunkReq)
	}

	// Collect results
	for i := 0; i < len(request.Chunks); i++ {
		result := <-results

		if result.Error != nil {
			response.FailedChunks++
		} else {
			response.ScannedChunks++
			response.Results[result.ChunkID] = result.Response
		}
	}

	// Calculate aggregate statistics
	response.AggregateStats = s.calculateAggregateStats(response.Results)
	response.ProcessingTime = time.Since(startTime)

	return response, nil
}

// GetSupportedPatterns returns all supported PII patterns
func (s *DLPService) GetSupportedPatterns() []types.PatternInfo {
	return s.scanner.GetSupportedPatterns()
}

// GetSupportedRegulations returns all supported compliance regulations
func (s *DLPService) GetSupportedRegulations() []string {
	return s.complianceChecker.GetSupportedRegulations()
}

// ValidateConfig validates a scan configuration
func (s *DLPService) ValidateConfig(config *types.ScanConfig) error {
	return s.scanner.ValidateConfig(config)
}

// Internal helper types and methods

type chunkScanResult struct {
	ChunkID  string
	Response *DLPScanResponse
	Error    error
}

func (s *DLPService) validateScanRequest(request *DLPScanRequest) error {
	if request.Content == "" {
		return fmt.Errorf("content cannot be empty")
	}

	if len(request.Content) > s.config.MaxContentLength {
		return fmt.Errorf("content exceeds maximum length of %d bytes", s.config.MaxContentLength)
	}

	if request.TenantID == uuid.Nil {
		return fmt.Errorf("tenant_id is required")
	}

	return nil
}

func (s *DLPService) prepareScanConfig(tenantID uuid.UUID, config *types.ScanConfig) *types.ScanConfig {
	if config == nil {
		config = &types.ScanConfig{}
	}

	// Set defaults
	if config.TenantID == uuid.Nil {
		config.TenantID = tenantID
	}

	if config.MinConfidence == 0 {
		config.MinConfidence = s.config.DefaultMinConfidence
	}

	if config.ScanTimeout == 0 {
		config.ScanTimeout = s.config.DefaultScanTimeout
	}

	if config.ContextWindow == 0 {
		config.ContextWindow = 20
	}

	return config
}

func (s *DLPService) generateRecommendations(scanResult *types.ScanResult, complianceResult *types.ComplianceResult) []string {
	var recommendations []string

	// Risk-based recommendations
	if scanResult.RiskScore >= 0.8 {
		recommendations = append(recommendations, "High risk content detected - consider additional security measures")
	}

	if scanResult.HighRiskCount > 0 {
		recommendations = append(recommendations, "Apply strong access controls to content with high-risk PII")
	}

	// PII-type specific recommendations
	findingsByType := make(map[types.PIIType]int)
	for _, finding := range scanResult.Findings {
		findingsByType[finding.Type]++
	}

	if findingsByType[types.PIITypeCreditCard] > 0 {
		recommendations = append(recommendations, "Credit card data detected - ensure PCI DSS compliance")
	}

	if findingsByType[types.PIITypeSSN] > 0 {
		recommendations = append(recommendations, "SSN detected - implement strong encryption and access controls")
	}

	// Compliance-based recommendations
	if complianceResult != nil && !complianceResult.IsCompliant {
		recommendations = append(recommendations, "Content not compliant - review required actions")
		recommendations = append(recommendations, complianceResult.RequiredActions...)
	}

	return recommendations
}

func (s *DLPService) calculateAggregateStats(results map[string]*DLPScanResponse) *AggregateStats {
	stats := &AggregateStats{
		FindingsByType:   make(map[types.PIIType]int),
		FindingsByRisk:   make(map[types.RiskLevel]int),
		ComplianceStatus: make(map[string]bool),
		TopViolations:    []types.ComplianceViolation{},
	}

	totalRiskScore := 0.0
	resultCount := 0

	for _, result := range results {
		if result.ScanResult == nil {
			continue
		}

		stats.TotalFindings += result.ScanResult.TotalMatches
		totalRiskScore += result.ScanResult.RiskScore
		resultCount++

		// Count findings by type and risk
		for _, finding := range result.ScanResult.Findings {
			stats.FindingsByType[finding.Type]++
			stats.FindingsByRisk[finding.RiskLevel]++
		}

		// Track compliance status
		if result.ComplianceResult != nil {
			stats.ComplianceStatus[result.ComplianceResult.Regulation] = result.ComplianceResult.IsCompliant
			stats.TopViolations = append(stats.TopViolations, result.ComplianceResult.Violations...)
		}
	}

	// Calculate average risk score
	if resultCount > 0 {
		stats.AverageRiskScore = totalRiskScore / float64(resultCount)
	}

	return stats
}
