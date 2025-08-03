package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/internal/server/response"
	"github.com/jscharber/eAIIngest/internal/services"
	"github.com/jscharber/eAIIngest/pkg/logger"
)

// MLAnalysisHandler handles ML analysis HTTP requests
type MLAnalysisHandler struct {
	db      *database.Database
	service *services.MLAnalysisService
	logger  *logger.Logger
}

// NewMLAnalysisHandler creates a new ML analysis handler
func NewMLAnalysisHandler(db *database.Database, logger *logger.Logger) *MLAnalysisHandler {
	config := services.DefaultMLAnalysisServiceConfig()
	service := services.NewMLAnalysisService(db, logger, config)

	return &MLAnalysisHandler{
		db:      db,
		service: service,
		logger:  logger,
	}
}

// AnalyzeContentRequest represents a content analysis request
type AnalyzeContentRequest struct {
	Content     string                 `json:"content" validate:"required,min=10"`
	ContentType string                 `json:"content_type"`
	Language    string                 `json:"language,omitempty"`
	Priority    int                    `json:"priority"`
	Options     map[string]interface{} `json:"options,omitempty"`
}

// AnalyzeDocumentRequest represents a document analysis request
type AnalyzeDocumentRequest struct {
	DocumentID uuid.UUID              `json:"document_id" validate:"required"`
	Options    map[string]interface{} `json:"options,omitempty"`
}

// BatchAnalysisRequest represents a batch analysis request
type BatchAnalysisRequestBody struct {
	Requests []AnalyzeContentRequest `json:"requests" validate:"required,min=1,max=100"`
	Options  map[string]interface{}  `json:"options,omitempty"`
}

// AnalyzeContent handles POST /api/v1/tenants/{tenant_id}/ml-analysis/analyze
func (h *MLAnalysisHandler) AnalyzeContent(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	if r.Method != http.MethodPost {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	var req AnalyzeContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
		return
	}

	// Validate request
	if len(req.Content) < 10 {
		response.WriteBadRequest(w, getRequestID(r), "Content too short", "Minimum 10 characters required")
		return
	}

	// Create service request
	documentID := uuid.New() // Generate a temporary document ID for ad-hoc analysis
	serviceRequest := &services.MLAnalysisRequest{
		DocumentID:  documentID,
		Content:     req.Content,
		ContentType: req.ContentType,
		Language:    req.Language,
		TenantID:    tenantID,
		Priority:    req.Priority,
		RequestedBy: "api",
		Options:     req.Options,
	}

	// Perform analysis
	responseData, err := h.service.AnalyzeContent(r.Context(), serviceRequest)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to analyze content")
		response.WriteInternalServerError(w, getRequestID(r), "Analysis failed", err.Error())
		return
	}

	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// AnalyzeDocument handles POST /api/v1/tenants/{tenant_id}/ml-analysis/documents/{document_id}/analyze
func (h *MLAnalysisHandler) AnalyzeDocument(w http.ResponseWriter, r *http.Request, tenantID, documentID uuid.UUID) {
	if r.Method != http.MethodPost {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	// Optional request body for options
	var req AnalyzeDocumentRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
			return
		}
	}

	h.logger.WithFields(map[string]interface{}{
		"tenant_id":   tenantID,
		"document_id": documentID,
	}).Info("Starting document analysis")

	// Perform document analysis
	responseData, err := h.service.AnalyzeDocument(r.Context(), tenantID, documentID)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to analyze document")
		response.WriteInternalServerError(w, getRequestID(r), "Document analysis failed", err.Error())
		return
	}

	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// AnalyzeBatch handles POST /api/v1/tenants/{tenant_id}/ml-analysis/batch
func (h *MLAnalysisHandler) AnalyzeBatch(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	if r.Method != http.MethodPost {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	var req BatchAnalysisRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
		return
	}

	// Validate request
	if len(req.Requests) == 0 {
		response.WriteBadRequest(w, getRequestID(r), "No requests provided", "At least one analysis request is required")
		return
	}
	if len(req.Requests) > 100 {
		response.WriteBadRequest(w, getRequestID(r), "Too many requests", "Maximum 100 requests allowed per batch")
		return
	}

	// Convert to service requests
	batchID := uuid.New()
	serviceRequests := make([]services.MLAnalysisRequest, len(req.Requests))

	for i, contentReq := range req.Requests {
		documentID := uuid.New() // Generate temporary document IDs for batch analysis
		serviceRequests[i] = services.MLAnalysisRequest{
			DocumentID:  documentID,
			Content:     contentReq.Content,
			ContentType: contentReq.ContentType,
			Language:    contentReq.Language,
			TenantID:    tenantID,
			Priority:    contentReq.Priority,
			RequestedBy: "api_batch",
			Options:     contentReq.Options,
		}
	}

	batchRequest := &services.BatchAnalysisRequest{
		BatchID:  batchID,
		TenantID: tenantID,
		Requests: serviceRequests,
		Options:  req.Options,
	}

	h.logger.WithFields(map[string]interface{}{
		"tenant_id":      tenantID,
		"batch_id":       batchID,
		"total_requests": len(serviceRequests),
	}).Info("Starting batch analysis")

	// Perform batch analysis
	responseData, err := h.service.AnalyzeBatch(r.Context(), batchRequest)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to analyze batch")
		response.WriteInternalServerError(w, getRequestID(r), "Batch analysis failed", err.Error())
		return
	}

	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// GetAnalysisResult handles GET /api/v1/tenants/{tenant_id}/ml-analysis/documents/{document_id}/results
func (h *MLAnalysisHandler) GetAnalysisResult(w http.ResponseWriter, r *http.Request, tenantID, documentID uuid.UUID) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	// Check for chunk_id parameter
	var chunkID *uuid.UUID
	if chunkIDStr := r.URL.Query().Get("chunk_id"); chunkIDStr != "" {
		if parsed, err := uuid.Parse(chunkIDStr); err == nil {
			chunkID = &parsed
		} else {
			response.WriteBadRequest(w, getRequestID(r), "Invalid chunk_id format", err.Error())
			return
		}
	}

	result, err := h.service.GetAnalysisResult(r.Context(), tenantID, documentID, chunkID)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to get analysis result")
		response.WriteNotFound(w, getRequestID(r), "Analysis result not found")
		return
	}

	response.WriteSuccess(w, getRequestID(r), result, nil)
}

// GetDocumentAnalysisSummary handles GET /api/v1/tenants/{tenant_id}/ml-analysis/documents/{document_id}/summary
func (h *MLAnalysisHandler) GetDocumentAnalysisSummary(w http.ResponseWriter, r *http.Request, tenantID, documentID uuid.UUID) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	summary, err := h.service.GetDocumentAnalysisSummary(r.Context(), tenantID, documentID)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to get document analysis summary")
		response.WriteNotFound(w, getRequestID(r), "Document analysis summary not found")
		return
	}

	response.WriteSuccess(w, getRequestID(r), summary, nil)
}

// ListAnalysisResults handles GET /api/v1/tenants/{tenant_id}/ml-analysis/results
func (h *MLAnalysisHandler) ListAnalysisResults(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	// Parse pagination parameters
	page, pageSize := parsePaginationParams(r)

	// Parse filter parameters
	documentID := r.URL.Query().Get("document_id")
	analysisType := r.URL.Query().Get("analysis_type")
	status := r.URL.Query().Get("status")

	// Get tenant repository
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to get tenant repository", err.Error())
		return
	}

	// Build query
	query := tenantRepo.DB().Model(&services.MLAnalysisResponse{})

	if documentID != "" {
		if docUUID, err := uuid.Parse(documentID); err == nil {
			query = query.Where("document_id = ?", docUUID)
		}
	}

	if analysisType != "" {
		query = query.Where("analysis_type = ?", analysisType)
	}

	if status != "" {
		query = query.Where("status = ?", status)
	}

	// Get total count
	var totalCount int64
	countQuery := query
	countQuery.Count(&totalCount)

	// Get results with pagination
	var results []services.MLAnalysisResponse
	err = query.Order("created_at DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&results).Error

	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list analysis results", err.Error())
		return
	}

	response.WritePaginated(w, getRequestID(r), results, page, pageSize, totalCount)
}

// GetAnalysisStats handles GET /api/v1/tenants/{tenant_id}/ml-analysis/stats
func (h *MLAnalysisHandler) GetAnalysisStats(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	// Parse time range parameters
	timeframe := r.URL.Query().Get("timeframe")
	if timeframe == "" {
		timeframe = "30d"
	}

	// Get tenant repository
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to get tenant repository", err.Error())
		return
	}

	// Calculate stats
	stats := h.calculateAnalysisStats(tenantRepo, timeframe)

	response.WriteSuccess(w, getRequestID(r), stats, nil)
}

// ServeHTTP routes ML analysis requests
func (h *MLAnalysisHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract tenant context
	tenantCtx := getTenantContext(r.Context())
	if tenantCtx == nil {
		response.WriteBadRequest(w, getRequestID(r), "Tenant context required", nil)
		return
	}

	// Parse path to determine action
	path := r.URL.Path
	basePath := "/api/v1/tenants/" + tenantCtx.TenantID.String() + "/ml-analysis"

	switch {
	case path == basePath+"/analyze":
		h.AnalyzeContent(w, r, tenantCtx.TenantID)
	case path == basePath+"/batch":
		h.AnalyzeBatch(w, r, tenantCtx.TenantID)
	case path == basePath+"/results":
		h.ListAnalysisResults(w, r, tenantCtx.TenantID)
	case path == basePath+"/stats":
		h.GetAnalysisStats(w, r, tenantCtx.TenantID)
	default:
		// Check for document-specific routes
		if documentID, ok := extractDocumentID(path, basePath); ok {
			switch {
			case path == basePath+"/documents/"+documentID.String()+"/analyze":
				h.AnalyzeDocument(w, r, tenantCtx.TenantID, documentID)
			case path == basePath+"/documents/"+documentID.String()+"/results":
				h.GetAnalysisResult(w, r, tenantCtx.TenantID, documentID)
			case path == basePath+"/documents/"+documentID.String()+"/summary":
				h.GetDocumentAnalysisSummary(w, r, tenantCtx.TenantID, documentID)
			default:
				response.WriteNotFound(w, getRequestID(r), "Endpoint not found")
			}
		} else {
			response.WriteNotFound(w, getRequestID(r), "Endpoint not found")
		}
	}
}

// Helper functions

func parsePaginationParams(r *http.Request) (int, int) {
	page := 1
	pageSize := 50

	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if parsed, err := strconv.Atoi(ps); err == nil && parsed > 0 && parsed <= 1000 {
			pageSize = parsed
		}
	}

	return page, pageSize
}

func extractDocumentID(path, basePath string) (uuid.UUID, bool) {
	// Extract document ID from path like: /api/v1/tenants/{tenant_id}/ml-analysis/documents/{document_id}/...
	prefix := basePath + "/documents/"
	if !strings.HasPrefix(path, prefix) {
		return uuid.Nil, false
	}

	remaining := path[len(prefix):]
	parts := strings.Split(remaining, "/")
	if len(parts) == 0 {
		return uuid.Nil, false
	}

	documentID, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, false
	}

	return documentID, true
}

func (h *MLAnalysisHandler) calculateAnalysisStats(tenantRepo *database.TenantRepository, timeframe string) map[string]interface{} {
	// TODO: Implement comprehensive statistics calculation
	// This is a simplified version for demonstration

	stats := map[string]interface{}{
		"timeframe":           timeframe,
		"total_analyses":      0,
		"avg_confidence":      0.0,
		"avg_processing_time": 0,
		"sentiment_distribution": map[string]int{
			"positive": 0,
			"negative": 0,
			"neutral":  0,
		},
		"top_topics": []string{},
		"language_distribution": map[string]int{
			"en": 0,
			"es": 0,
			"fr": 0,
		},
		"quality_scores": map[string]float64{
			"avg_coherence":    0.0,
			"avg_clarity":      0.0,
			"avg_completeness": 0.0,
		},
	}

	// This would be implemented with actual database queries
	// based on the timeframe and tenant data

	return stats
}
