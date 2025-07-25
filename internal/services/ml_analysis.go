package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/internal/database/models"
	"github.com/jscharber/eAIIngest/pkg/analysis"
	"github.com/jscharber/eAIIngest/pkg/logger"
)

// MLAnalysisService provides ML-based content analysis capabilities
type MLAnalysisService struct {
	db       *database.Database
	analyzer *analysis.MLAnalyzer
	logger   *logger.Logger
	config   *MLAnalysisServiceConfig
}

// MLAnalysisServiceConfig configures the ML analysis service
type MLAnalysisServiceConfig struct {
	EnableAsync        bool          `json:"enable_async"`
	BatchSize          int           `json:"batch_size"`
	ProcessingTimeout  time.Duration `json:"processing_timeout"`
	RetryAttempts      int           `json:"retry_attempts"`
	RetryDelay         time.Duration `json:"retry_delay"`
	PersistResults     bool          `json:"persist_results"`
	CacheEnabled       bool          `json:"cache_enabled"`
	CacheTTL           time.Duration `json:"cache_ttl"`
	MinTextLength      int           `json:"min_text_length"`
	MaxTextLength      int           `json:"max_text_length"`
	EnabledAnalysis    []string      `json:"enabled_analysis"`
}

// MLAnalysisRequest represents a request for ML analysis
type MLAnalysisRequest struct {
	DocumentID   uuid.UUID `json:"document_id"`
	ChunkID      *uuid.UUID `json:"chunk_id,omitempty"`
	Content      string    `json:"content"`
	ContentType  string    `json:"content_type"`
	Language     string    `json:"language,omitempty"`
	TenantID     uuid.UUID `json:"tenant_id"`
	Priority     int       `json:"priority"`
	RequestedBy  string    `json:"requested_by"`
	Options      map[string]interface{} `json:"options,omitempty"`
}

// MLAnalysisResponse represents the response from ML analysis
type MLAnalysisResponse struct {
	RequestID      uuid.UUID                `json:"request_id"`
	DocumentID     uuid.UUID                `json:"document_id"`
	ChunkID        *uuid.UUID               `json:"chunk_id,omitempty"`
	TenantID       uuid.UUID                `json:"tenant_id"`
	Status         string                   `json:"status"`
	Result         *analysis.MLAnalysisResult `json:"result,omitempty"`
	Error          string                   `json:"error,omitempty"`
	ProcessingTime int64                    `json:"processing_time_ms"`
	Timestamp      time.Time                `json:"timestamp"`
	RetryCount     int                      `json:"retry_count"`
}

// BatchAnalysisRequest represents a batch analysis request
type BatchAnalysisRequest struct {
	BatchID   uuid.UUID            `json:"batch_id"`
	TenantID  uuid.UUID            `json:"tenant_id"`
	Requests  []MLAnalysisRequest  `json:"requests"`
	Options   map[string]interface{} `json:"options,omitempty"`
}

// BatchAnalysisResponse represents a batch analysis response
type BatchAnalysisResponse struct {
	BatchID        uuid.UUID             `json:"batch_id"`
	TenantID       uuid.UUID             `json:"tenant_id"`
	TotalRequests  int                   `json:"total_requests"`
	Completed      int                   `json:"completed"`
	Failed         int                   `json:"failed"`
	Responses      []MLAnalysisResponse  `json:"responses"`
	ProcessingTime int64                 `json:"processing_time_ms"`
	Status         string                `json:"status"`
	Timestamp      time.Time             `json:"timestamp"`
}

// NewMLAnalysisService creates a new ML analysis service
func NewMLAnalysisService(db *database.Database, logger *logger.Logger, config *MLAnalysisServiceConfig) *MLAnalysisService {
	if config == nil {
		config = DefaultMLAnalysisServiceConfig()
	}

	// Create ML analyzer config based on service config
	analyzerConfig := &analysis.MLAnalysisConfig{
		EnableSentiment:    contains(config.EnabledAnalysis, "sentiment"),
		EnableTopics:       contains(config.EnabledAnalysis, "topics"),
		EnableEntities:     contains(config.EnabledAnalysis, "entities"),
		EnableSummary:      contains(config.EnabledAnalysis, "summary"),
		EnableQuality:      contains(config.EnabledAnalysis, "quality"),
		EnableLanguage:     contains(config.EnabledAnalysis, "language"),
		EnableEmotion:      contains(config.EnabledAnalysis, "emotion"),
		MaxTopics:          5,
		SummaryRatio:       0.3,
		MinConfidence:      0.5,
		ModelTimeout:       30 * time.Second,
		CacheEnabled:       config.CacheEnabled,
		CacheTTL:           config.CacheTTL,
		ParallelProcessing: true,
		BatchSize:          config.BatchSize,
	}

	analyzer := analysis.NewMLAnalyzer(analyzerConfig)

	return &MLAnalysisService{
		db:       db,
		analyzer: analyzer,
		logger:   logger,
		config:   config,
	}
}

// DefaultMLAnalysisServiceConfig returns default configuration
func DefaultMLAnalysisServiceConfig() *MLAnalysisServiceConfig {
	return &MLAnalysisServiceConfig{
		EnableAsync:       true,
		BatchSize:         10,
		ProcessingTimeout: 5 * time.Minute,
		RetryAttempts:     3,
		RetryDelay:        1 * time.Second,
		PersistResults:    true,
		CacheEnabled:      true,
		CacheTTL:          1 * time.Hour,
		MinTextLength:     10,
		MaxTextLength:     100000,
		EnabledAnalysis:   []string{"sentiment", "topics", "entities", "summary", "quality", "language", "emotion"},
	}
}

// AnalyzeContent performs ML analysis on a single piece of content
func (s *MLAnalysisService) AnalyzeContent(ctx context.Context, request *MLAnalysisRequest) (*MLAnalysisResponse, error) {
	startTime := time.Now()
	requestID := uuid.New()

	s.logger.WithFields(map[string]interface{}{
		"request_id":  requestID.String(),
		"document_id": request.DocumentID.String(),
		"tenant_id":   request.TenantID.String(),
		"content_len": len(request.Content),
	}).Info("Starting ML analysis")

	response := &MLAnalysisResponse{
		RequestID:  requestID,
		DocumentID: request.DocumentID,
		ChunkID:    request.ChunkID,
		TenantID:   request.TenantID,
		Status:     "processing",
		Timestamp:  time.Now(),
	}

	// Validate request
	if err := s.validateRequest(request); err != nil {
		response.Status = "failed"
		response.Error = err.Error()
		s.logger.WithField("error", err).Error("Request validation failed")
		return response, err
	}

	// Check cache if enabled
	if s.config.CacheEnabled {
		if cached, err := s.getCachedResult(ctx, request); err == nil && cached != nil {
			s.logger.WithField("request_id", requestID).Info("Returning cached ML analysis result")
			response.Status = "completed"
			response.Result = cached
			response.ProcessingTime = time.Since(startTime).Milliseconds()
			return response, nil
		}
	}

	// Perform analysis with retry logic
	var result *analysis.MLAnalysisResult
	var err error

	for attempt := 0; attempt <= s.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			s.logger.WithFields(map[string]interface{}{
				"request_id": requestID,
				"attempt":    attempt,
			}).Warn("Retrying ML analysis")
			time.Sleep(s.config.RetryDelay)
		}

		// Create analysis context with timeout
		analysisCtx, cancel := context.WithTimeout(ctx, s.config.ProcessingTimeout)
		
		chunkID := ""
		if request.ChunkID != nil {
			chunkID = request.ChunkID.String()
		}

		result, err = s.analyzer.AnalyzeContent(
			analysisCtx, 
			request.DocumentID.String(), 
			chunkID,
			request.Content,
		)
		
		cancel()

		if err == nil {
			break
		}

		response.RetryCount = attempt + 1
		s.logger.WithFields(map[string]interface{}{
			"request_id": requestID,
			"attempt":    attempt,
			"error":      err,
		}).Error("ML analysis attempt failed")
	}

	// Handle final result
	if err != nil {
		response.Status = "failed"
		response.Error = err.Error()
		s.logger.WithField("error", err).Error("ML analysis failed after all retries")
		return response, err
	}

	response.Status = "completed"
	response.Result = result
	response.ProcessingTime = time.Since(startTime).Milliseconds()

	// Cache result if enabled
	if s.config.CacheEnabled {
		if err := s.cacheResult(ctx, request, result); err != nil {
			s.logger.WithField("error", err).Warn("Failed to cache ML analysis result")
		}
	}

	// Persist result if enabled
	if s.config.PersistResults {
		if err := s.persistResult(ctx, response); err != nil {
			s.logger.WithField("error", err).Warn("Failed to persist ML analysis result")
		}
	}

	s.logger.WithFields(map[string]interface{}{
		"request_id":      requestID,
		"processing_time": response.ProcessingTime,
		"confidence":      result.Confidence,
	}).Info("ML analysis completed successfully")

	return response, nil
}

// AnalyzeBatch performs ML analysis on a batch of content
func (s *MLAnalysisService) AnalyzeBatch(ctx context.Context, request *BatchAnalysisRequest) (*BatchAnalysisResponse, error) {
	startTime := time.Now()

	s.logger.WithFields(map[string]interface{}{
		"batch_id":       request.BatchID.String(),
		"tenant_id":      request.TenantID.String(),
		"total_requests": len(request.Requests),
	}).Info("Starting batch ML analysis")

	response := &BatchAnalysisResponse{
		BatchID:       request.BatchID,
		TenantID:      request.TenantID,
		TotalRequests: len(request.Requests),
		Status:        "processing",
		Timestamp:     time.Now(),
		Responses:     make([]MLAnalysisResponse, 0, len(request.Requests)),
	}

	// Process requests in batches
	batchSize := s.config.BatchSize
	if batchSize <= 0 {
		batchSize = 10
	}

	for i := 0; i < len(request.Requests); i += batchSize {
		end := min(i+batchSize, len(request.Requests))
		batch := request.Requests[i:end]

		// Process batch concurrently if async is enabled
		if s.config.EnableAsync {
			batchResponses := s.processBatchConcurrently(ctx, batch)
			response.Responses = append(response.Responses, batchResponses...)
		} else {
			batchResponses := s.processBatchSequentially(ctx, batch)
			response.Responses = append(response.Responses, batchResponses...)
		}
	}

	// Calculate final statistics
	for _, resp := range response.Responses {
		if resp.Status == "completed" {
			response.Completed++
		} else {
			response.Failed++
		}
	}

	response.ProcessingTime = time.Since(startTime).Milliseconds()
	response.Status = "completed"

	s.logger.WithFields(map[string]interface{}{
		"batch_id":        request.BatchID,
		"total_requests":  response.TotalRequests,
		"completed":       response.Completed,
		"failed":          response.Failed,
		"processing_time": response.ProcessingTime,
	}).Info("Batch ML analysis completed")

	return response, nil
}

// AnalyzeDocument performs ML analysis on all chunks of a document
func (s *MLAnalysisService) AnalyzeDocument(ctx context.Context, tenantID, documentID uuid.UUID) (*BatchAnalysisResponse, error) {
	s.logger.WithFields(map[string]interface{}{
		"tenant_id":   tenantID.String(),
		"document_id": documentID.String(),
	}).Info("Starting document ML analysis")

	// Get tenant repository
	tenantService := s.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant repository: %w", err)
	}

	// Get document chunks
	var chunks []models.Chunk
	if err := tenantRepo.DB().Where("file_id = ?", documentID).Find(&chunks).Error; err != nil {
		return nil, fmt.Errorf("failed to get document chunks: %w", err)
	}

	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks found for document %s", documentID)
	}

	// Create batch request
	batchID := uuid.New()
	requests := make([]MLAnalysisRequest, 0, len(chunks))

	for _, chunk := range chunks {
		requests = append(requests, MLAnalysisRequest{
			DocumentID:  documentID,
			ChunkID:     &chunk.ID,
			Content:     chunk.Content,
			ContentType: "text",
			TenantID:    tenantID,
			Priority:    1,
			RequestedBy: "document_analysis",
		})
	}

	batchRequest := &BatchAnalysisRequest{
		BatchID:  batchID,
		TenantID: tenantID,
		Requests: requests,
	}

	return s.AnalyzeBatch(ctx, batchRequest)
}

// GetAnalysisResult retrieves a stored analysis result
func (s *MLAnalysisService) GetAnalysisResult(ctx context.Context, tenantID, documentID uuid.UUID, chunkID *uuid.UUID) (*analysis.MLAnalysisResult, error) {
	tenantService := s.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant repository: %w", err)
	}

	var analysisRecord models.MLAnalysisResult
	query := tenantRepo.DB().Where("document_id = ?", documentID)
	
	if chunkID != nil {
		query = query.Where("chunk_id = ?", *chunkID)
	}

	if err := query.First(&analysisRecord).Error; err != nil {
		return nil, fmt.Errorf("analysis result not found: %w", err)
	}

	var result analysis.MLAnalysisResult
	if err := json.Unmarshal([]byte(analysisRecord.ResultData), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal analysis result: %w", err)
	}

	return &result, nil
}

// GetDocumentAnalysisSummary retrieves aggregated analysis for a document
func (s *MLAnalysisService) GetDocumentAnalysisSummary(ctx context.Context, tenantID, documentID uuid.UUID) (*DocumentAnalysisSummary, error) {
	tenantService := s.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant repository: %w", err)
	}

	var records []models.MLAnalysisResult
	if err := tenantRepo.DB().Where("document_id = ?", documentID).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to get analysis records: %w", err)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("no analysis results found for document %s", documentID)
	}

	return s.aggregateAnalysisResults(records)
}

// Helper methods

func (s *MLAnalysisService) validateRequest(request *MLAnalysisRequest) error {
	if request.DocumentID == uuid.Nil {
		return fmt.Errorf("document ID is required")
	}
	if request.TenantID == uuid.Nil {
		return fmt.Errorf("tenant ID is required")
	}
	if len(request.Content) < s.config.MinTextLength {
		return fmt.Errorf("content too short: minimum %d characters required", s.config.MinTextLength)
	}
	if len(request.Content) > s.config.MaxTextLength {
		return fmt.Errorf("content too long: maximum %d characters allowed", s.config.MaxTextLength)
	}
	return nil
}

func (s *MLAnalysisService) getCachedResult(ctx context.Context, request *MLAnalysisRequest) (*analysis.MLAnalysisResult, error) {
	// TODO: Implement caching mechanism (Redis, in-memory, etc.)
	return nil, fmt.Errorf("cache not implemented")
}

func (s *MLAnalysisService) cacheResult(ctx context.Context, request *MLAnalysisRequest, result *analysis.MLAnalysisResult) error {
	// TODO: Implement caching mechanism
	return nil
}

func (s *MLAnalysisService) persistResult(ctx context.Context, response *MLAnalysisResponse) error {
	if response.Result == nil {
		return nil
	}

	tenantService := s.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, response.TenantID)
	if err != nil {
		return fmt.Errorf("failed to get tenant repository: %w", err)
	}

	resultData, err := json.Marshal(response.Result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	record := models.MLAnalysisResult{
		ID:             response.RequestID,
		DocumentID:     response.DocumentID,
		ChunkID:        response.ChunkID,
		AnalysisType:   "comprehensive",
		ResultData:     string(resultData),
		Confidence:     response.Result.Confidence,
		ProcessingTime: response.ProcessingTime,
		ModelVersions:  toJSONString(response.Result.ModelVersions),
		Status:         response.Status,
		Error:          response.Error,
		CreatedAt:      time.Now(),
	}

	return tenantRepo.DB().Create(&record).Error
}

func (s *MLAnalysisService) processBatchConcurrently(ctx context.Context, requests []MLAnalysisRequest) []MLAnalysisResponse {
	responses := make([]MLAnalysisResponse, len(requests))
	responseChan := make(chan struct {
		index    int
		response *MLAnalysisResponse
	}, len(requests))

	// Process requests concurrently
	for i, request := range requests {
		go func(index int, req MLAnalysisRequest) {
			resp, _ := s.AnalyzeContent(ctx, &req)
			responseChan <- struct {
				index    int
				response *MLAnalysisResponse
			}{index: index, response: resp}
		}(i, request)
	}

	// Collect responses
	for i := 0; i < len(requests); i++ {
		result := <-responseChan
		responses[result.index] = *result.response
	}

	return responses
}

func (s *MLAnalysisService) processBatchSequentially(ctx context.Context, requests []MLAnalysisRequest) []MLAnalysisResponse {
	responses := make([]MLAnalysisResponse, len(requests))

	for i, request := range requests {
		resp, _ := s.AnalyzeContent(ctx, &request)
		responses[i] = *resp
	}

	return responses
}

func (s *MLAnalysisService) aggregateAnalysisResults(records []models.MLAnalysisResult) (*DocumentAnalysisSummary, error) {
	summary := &DocumentAnalysisSummary{
		DocumentID:     records[0].DocumentID,
		TotalChunks:    len(records),
		AvgConfidence:  0.0,
		ProcessingTime: 0,
		Timestamp:      time.Now(),
	}

	totalConfidence := 0.0
	var sentiments []string
	var topics []string
	var entities []string

	for _, record := range records {
		var result analysis.MLAnalysisResult
		if err := json.Unmarshal([]byte(record.ResultData), &result); err != nil {
			continue
		}

		totalConfidence += result.Confidence
		summary.ProcessingTime += record.ProcessingTime

		// Aggregate sentiments
		if result.Sentiment.Label != "" {
			sentiments = append(sentiments, result.Sentiment.Label)
		}

		// Aggregate topics
		for _, topic := range result.Topics {
			topics = append(topics, topic.Topic)
		}

		// Aggregate entities
		for _, entity := range result.Entities {
			entities = append(entities, entity.Text)
		}
	}

	summary.AvgConfidence = totalConfidence / float64(len(records))
	summary.DominantSentiment = getMostFrequent(sentiments)
	summary.MainTopics = getUniqueItems(topics, 5)
	summary.KeyEntities = getUniqueItems(entities, 10)

	return summary, nil
}

// DocumentAnalysisSummary represents aggregated analysis for a document
type DocumentAnalysisSummary struct {
	DocumentID        uuid.UUID `json:"document_id"`
	TotalChunks       int       `json:"total_chunks"`
	AvgConfidence     float64   `json:"avg_confidence"`
	DominantSentiment string    `json:"dominant_sentiment"`
	MainTopics        []string  `json:"main_topics"`
	KeyEntities       []string  `json:"key_entities"`
	ProcessingTime    int64     `json:"processing_time_ms"`
	Timestamp         time.Time `json:"timestamp"`
}

// Utility functions

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func toJSONString(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data)
}

func getMostFrequent(items []string) string {
	if len(items) == 0 {
		return ""
	}

	counts := make(map[string]int)
	for _, item := range items {
		counts[item]++
	}

	maxCount := 0
	mostFrequent := ""
	for item, count := range counts {
		if count > maxCount {
			maxCount = count
			mostFrequent = item
		}
	}

	return mostFrequent
}

func getUniqueItems(items []string, limit int) []string {
	seen := make(map[string]bool)
	unique := []string{}

	for _, item := range items {
		if !seen[item] && len(unique) < limit {
			seen[item] = true
			unique = append(unique, item)
		}
	}

	return unique
}