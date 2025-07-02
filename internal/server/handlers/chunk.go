package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/internal/database/models"
	"github.com/jscharber/eAIIngest/internal/server/response"
)

// ChunkHandler handles chunk HTTP requests
type ChunkHandler struct {
	db *database.Database
}

// NewChunkHandler creates a new chunk handler
func NewChunkHandler(db *database.Database) *ChunkHandler {
	return &ChunkHandler{db: db}
}

// ChunkResponse represents a chunk response
type ChunkResponse struct {
	ID               uuid.UUID                      `json:"id"`
	TenantID         uuid.UUID                      `json:"tenant_id"`
	FileID           uuid.UUID                      `json:"file_id"`
	ChunkID          string                         `json:"chunk_id"`
	ChunkType        string                         `json:"chunk_type"`
	ChunkNumber      int                            `json:"chunk_number"`
	Content          string                         `json:"content"`
	ContentHash      string                         `json:"content_hash"`
	SizeBytes        int64                          `json:"size_bytes"`
	StartPosition    *int64                         `json:"start_position,omitempty"`
	EndPosition      *int64                         `json:"end_position,omitempty"`
	PageNumber       *int                           `json:"page_number,omitempty"`
	LineNumber       *int                           `json:"line_number,omitempty"`
	ParentChunkID    *uuid.UUID                     `json:"parent_chunk_id,omitempty"`
	Relationships    []string                       `json:"relationships,omitempty"`
	ProcessedAt      string                         `json:"processed_at"`
	ProcessedBy      string                         `json:"processed_by"`
	ProcessingTime   int64                          `json:"processing_time"`
	Quality          models.ChunkQualityMetrics     `json:"quality"`
	Language         string                         `json:"language,omitempty"`
	LanguageConf     float64                        `json:"language_confidence,omitempty"`
	ContentCategory  string                         `json:"content_category,omitempty"`
	SensitivityLevel string                         `json:"sensitivity_level,omitempty"`
	Classifications  []string                       `json:"classifications,omitempty"`
	EmbeddingStatus  string                         `json:"embedding_status"`
	EmbeddingModel   string                         `json:"embedding_model,omitempty"`
	EmbeddingDim     int                            `json:"embedding_dimension,omitempty"`
	EmbeddedAt       *string                        `json:"embedded_at,omitempty"`
	PIIDetected      bool                           `json:"pii_detected"`
	ComplianceFlags  []string                       `json:"compliance_flags,omitempty"`
	DLPScanStatus    string                         `json:"dlp_scan_status"`
	DLPScanResult    string                         `json:"dlp_scan_result,omitempty"`
	Context          map[string]string              `json:"context,omitempty"`
	SchemaInfo       map[string]interface{}         `json:"schema_info,omitempty"`
	Metadata         map[string]interface{}         `json:"metadata,omitempty"`
	CustomFields     map[string]interface{}         `json:"custom_fields,omitempty"`
	CreatedAt        string                         `json:"created_at"`
	UpdatedAt        string                         `json:"updated_at"`
}

// ChunkSearchRequest represents a chunk search request
type ChunkSearchRequest struct {
	Query           string   `json:"query"`
	FileIDs         []string `json:"file_ids,omitempty"`
	ChunkTypes      []string `json:"chunk_types,omitempty"`
	Classifications []string `json:"classifications,omitempty"`
	MinSimilarity   float64  `json:"min_similarity,omitempty"`
	Limit           int      `json:"limit,omitempty"`
}

// ServeHTTP implements the http.Handler interface
func (h *ChunkHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tenantCtx := getTenantContext(r.Context())
	if tenantCtx == nil {
		response.WriteBadRequest(w, getRequestID(r), "Tenant context required", nil)
		return
	}

	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	
	// Find chunks in the path
	var chunkIndex int = -1
	for i, part := range parts {
		if part == "chunks" {
			chunkIndex = i
			break
		}
	}

	if chunkIndex == -1 {
		response.WriteNotFound(w, getRequestID(r), "Chunk endpoint not found")
		return
	}

	// Handle different routes
	if len(parts) == chunkIndex+1 {
		// /api/v1/tenants/{tenant_id}/chunks
		switch r.Method {
		case http.MethodGet:
			h.ListChunks(w, r, tenantCtx.TenantID)
		case http.MethodPost:
			// Handle search
			if r.Header.Get("Content-Type") == "application/json" {
				h.SearchChunks(w, r, tenantCtx.TenantID)
			} else {
				response.WriteBadRequest(w, getRequestID(r), "Content-Type must be application/json for search", nil)
			}
		default:
			response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		}
		return
	}

	if len(parts) >= chunkIndex+2 {
		// Handle search endpoint
		if parts[chunkIndex+1] == "search" {
			switch r.Method {
			case http.MethodPost:
				h.SearchChunks(w, r, tenantCtx.TenantID)
			default:
				response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
			}
			return
		}

		// Handle specific chunk ID
		chunkID, err := uuid.Parse(parts[chunkIndex+1])
		if err != nil {
			response.WriteBadRequest(w, getRequestID(r), "Invalid chunk ID format", nil)
			return
		}

		if len(parts) == chunkIndex+2 {
			// /api/v1/tenants/{tenant_id}/chunks/{chunk_id}
			switch r.Method {
			case http.MethodGet:
				h.GetChunk(w, r, tenantCtx.TenantID, chunkID)
			case http.MethodPut:
				h.UpdateChunk(w, r, tenantCtx.TenantID, chunkID)
			case http.MethodDelete:
				h.DeleteChunk(w, r, tenantCtx.TenantID, chunkID)
			default:
				response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
			}
			return
		}

		// Handle sub-resources
		if len(parts) >= chunkIndex+3 {
			subResource := parts[chunkIndex+2]
			switch subResource {
			case "embedding":
				h.GetChunkEmbedding(w, r, tenantCtx.TenantID, chunkID)
			case "similar":
				h.FindSimilarChunks(w, r, tenantCtx.TenantID, chunkID)
			case "violations":
				h.ListChunkViolations(w, r, tenantCtx.TenantID, chunkID)
			case "relationships":
				h.GetChunkRelationships(w, r, tenantCtx.TenantID, chunkID)
			default:
				response.WriteNotFound(w, getRequestID(r), "Resource not found")
			}
			return
		}
	}

	response.WriteNotFound(w, getRequestID(r), "Endpoint not found")
}

// ListChunks handles GET /api/v1/tenants/{tenant_id}/chunks
func (h *ChunkHandler) ListChunks(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	page, pageSize, offset := getPagination(r.Context())
	
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var chunks []models.Chunk
	query := tenantRepo.DB().Limit(pageSize).Offset(offset).Order("created_at DESC")
	
	// Apply filters
	if fileID := r.URL.Query().Get("file_id"); fileID != "" {
		if fID, err := uuid.Parse(fileID); err == nil {
			query = query.Where("file_id = ?", fID)
		}
	}
	
	if chunkType := r.URL.Query().Get("chunk_type"); chunkType != "" {
		query = query.Where("chunk_type = ?", chunkType)
	}
	
	if embeddingStatus := r.URL.Query().Get("embedding_status"); embeddingStatus != "" {
		query = query.Where("embedding_status = ?", embeddingStatus)
	}
	
	if dlpStatus := r.URL.Query().Get("dlp_scan_status"); dlpStatus != "" {
		query = query.Where("dlp_scan_status = ?", dlpStatus)
	}
	
	if piiDetected := r.URL.Query().Get("pii_detected"); piiDetected != "" {
		detected := piiDetected == "true"
		query = query.Where("pii_detected = ?", detected)
	}
	
	if language := r.URL.Query().Get("language"); language != "" {
		query = query.Where("language = ?", language)
	}
	
	err = query.Find(&chunks).Error
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list chunks", err.Error())
		return
	}

	// Convert to response format
	response := make([]ChunkResponse, len(chunks))
	for i, chunk := range chunks {
		response[i] = h.toChunkResponse(&chunk)
	}

	// Get total count
	var totalCount int64
	countQuery := tenantRepo.DB().Model(&models.Chunk{})
	// Apply same filters for count
	if fileID := r.URL.Query().Get("file_id"); fileID != "" {
		if fID, err := uuid.Parse(fileID); err == nil {
			countQuery = countQuery.Where("file_id = ?", fID)
		}
	}
	// ... other filters
	countQuery.Count(&totalCount)

	response.WritePaginated(w, getRequestID(r), response, page, pageSize, totalCount)
}

// GetChunk handles GET /api/v1/tenants/{tenant_id}/chunks/{id}
func (h *ChunkHandler) GetChunk(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, chunkID uuid.UUID) {
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var chunk models.Chunk
	err = tenantRepo.DB().Where("id = ?", chunkID).First(&chunk).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Chunk not found")
		return
	}

	response := h.toChunkResponse(&chunk)
	response.WriteSuccess(w, getRequestID(r), response, nil)
}

// UpdateChunk handles PUT /api/v1/tenants/{tenant_id}/chunks/{id}
func (h *ChunkHandler) UpdateChunk(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, chunkID uuid.UUID) {
	var req struct {
		ContentCategory  string                     `json:"content_category"`
		SensitivityLevel string                     `json:"sensitivity_level"`
		Classifications  []string                   `json:"classifications"`
		ComplianceFlags  []string                   `json:"compliance_flags"`
		Quality          models.ChunkQualityMetrics `json:"quality"`
		Context          map[string]string          `json:"context"`
		Metadata         map[string]interface{}     `json:"metadata"`
		CustomFields     map[string]interface{}     `json:"custom_fields"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
		return
	}

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var chunk models.Chunk
	err = tenantRepo.DB().Where("id = ?", chunkID).First(&chunk).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Chunk not found")
		return
	}

	// Update allowed fields
	if req.ContentCategory != "" {
		chunk.ContentCategory = req.ContentCategory
	}
	if req.SensitivityLevel != "" {
		chunk.SensitivityLevel = req.SensitivityLevel
	}
	if req.Classifications != nil {
		chunk.Classifications = req.Classifications
	}
	if req.ComplianceFlags != nil {
		chunk.ComplianceFlags = req.ComplianceFlags
	}
	if req.Context != nil {
		chunk.Context = req.Context
	}
	if req.Metadata != nil {
		chunk.Metadata = req.Metadata
	}
	if req.CustomFields != nil {
		chunk.CustomFields = req.CustomFields
	}

	// Update quality metrics if provided
	chunk.Quality = req.Quality

	if err := tenantRepo.ValidateAndUpdate(&chunk); err != nil {
		if strings.Contains(err.Error(), "validation") {
			response.WriteValidationError(w, getRequestID(r), err.Error())
			return
		}
		response.WriteInternalServerError(w, getRequestID(r), "Failed to update chunk", err.Error())
		return
	}

	response := h.toChunkResponse(&chunk)
	response.WriteSuccess(w, getRequestID(r), response, nil)
}

// DeleteChunk handles DELETE /api/v1/tenants/{tenant_id}/chunks/{id}
func (h *ChunkHandler) DeleteChunk(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, chunkID uuid.UUID) {
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var chunk models.Chunk
	err = tenantRepo.DB().Where("id = ?", chunkID).First(&chunk).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Chunk not found")
		return
	}

	if err := tenantRepo.DB().Delete(&chunk).Error; err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to delete chunk", err.Error())
		return
	}

	response.WriteNoContent(w)
}

// SearchChunks handles POST /api/v1/tenants/{tenant_id}/chunks/search
func (h *ChunkHandler) SearchChunks(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	var req ChunkSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
		return
	}

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	// Build search query
	query := tenantRepo.DB()
	
	// Text search using PostgreSQL full-text search
	if req.Query != "" {
		query = query.Where("to_tsvector('english', content) @@ plainto_tsquery('english', ?)", req.Query)
	}

	// Filter by file IDs
	if len(req.FileIDs) > 0 {
		fileUUIDs := make([]uuid.UUID, 0, len(req.FileIDs))
		for _, fid := range req.FileIDs {
			if fileUUID, err := uuid.Parse(fid); err == nil {
				fileUUIDs = append(fileUUIDs, fileUUID)
			}
		}
		if len(fileUUIDs) > 0 {
			query = query.Where("file_id IN ?", fileUUIDs)
		}
	}

	// Filter by chunk types
	if len(req.ChunkTypes) > 0 {
		query = query.Where("chunk_type IN ?", req.ChunkTypes)
	}

	// Filter by classifications
	if len(req.Classifications) > 0 {
		for _, classification := range req.Classifications {
			query = query.Where("classifications @> ?", `["`+classification+`"]`)
		}
	}

	// Set limit
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query = query.Limit(limit)

	var chunks []models.Chunk
	err = query.Order("created_at DESC").Find(&chunks).Error
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to search chunks", err.Error())
		return
	}

	// Convert to response format
	response := make([]ChunkResponse, len(chunks))
	for i, chunk := range chunks {
		response[i] = h.toChunkResponse(&chunk)
	}

	searchResult := map[string]interface{}{
		"query":   req.Query,
		"results": response,
		"count":   len(response),
		"limit":   limit,
	}

	response.WriteSuccess(w, getRequestID(r), searchResult, nil)
}

// GetChunkEmbedding handles GET /api/v1/tenants/{tenant_id}/chunks/{id}/embedding
func (h *ChunkHandler) GetChunkEmbedding(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, chunkID uuid.UUID) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var chunk models.Chunk
	err = tenantRepo.DB().Where("id = ?", chunkID).First(&chunk).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Chunk not found")
		return
	}

	if !chunk.IsEmbedded() {
		response.WriteNotFound(w, getRequestID(r), "Chunk embedding not available")
		return
	}

	embedding := map[string]interface{}{
		"chunk_id":  chunk.ID,
		"vector":    chunk.EmbeddingVector,
		"dimension": chunk.EmbeddingDim,
		"model":     chunk.EmbeddingModel,
		"status":    chunk.EmbeddingStatus,
		"embedded_at": chunk.EmbeddedAt,
	}

	response.WriteSuccess(w, getRequestID(r), embedding, nil)
}

// FindSimilarChunks handles GET /api/v1/tenants/{tenant_id}/chunks/{id}/similar
func (h *ChunkHandler) FindSimilarChunks(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, chunkID uuid.UUID) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var chunk models.Chunk
	err = tenantRepo.DB().Where("id = ?", chunkID).First(&chunk).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Chunk not found")
		return
	}

	if !chunk.IsEmbedded() {
		response.WriteBadRequest(w, getRequestID(r), "Chunk must have embedding for similarity search", nil)
		return
	}

	// TODO: Implement actual vector similarity search
	// This would involve comparing embedding vectors using cosine similarity or other metrics
	
	// For now, return a mock response
	similarChunks := []map[string]interface{}{
		// Mock similar chunks
	}

	result := map[string]interface{}{
		"chunk_id":       chunkID,
		"similar_chunks": similarChunks,
		"count":          len(similarChunks),
	}

	response.WriteSuccess(w, getRequestID(r), result, nil)
}

// ListChunkViolations handles GET /api/v1/tenants/{tenant_id}/chunks/{id}/violations
func (h *ChunkHandler) ListChunkViolations(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, chunkID uuid.UUID) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	page, pageSize, offset := getPagination(r.Context())

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	// Check if chunk exists
	var chunk models.Chunk
	err = tenantRepo.DB().Where("id = ?", chunkID).First(&chunk).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Chunk not found")
		return
	}

	// Get violations for this chunk
	var violations []models.DLPViolation
	err = tenantRepo.DB().Where("chunk_id = ?", chunkID).
		Limit(pageSize).Offset(offset).Order("created_at DESC").Find(&violations).Error
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list violations", err.Error())
		return
	}

	// Get total count
	var totalCount int64
	tenantRepo.DB().Model(&models.DLPViolation{}).Where("chunk_id = ?", chunkID).Count(&totalCount)

	response.WritePaginated(w, getRequestID(r), violations, page, pageSize, totalCount)
}

// GetChunkRelationships handles GET /api/v1/tenants/{tenant_id}/chunks/{id}/relationships
func (h *ChunkHandler) GetChunkRelationships(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, chunkID uuid.UUID) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var chunk models.Chunk
	err = tenantRepo.DB().Where("id = ?", chunkID).First(&chunk).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Chunk not found")
		return
	}

	// Get parent chunk if exists
	var parentChunk *models.Chunk
	if chunk.ParentChunkID != nil {
		var parent models.Chunk
		if err := tenantRepo.DB().Where("id = ?", *chunk.ParentChunkID).First(&parent).Error; err == nil {
			parentChunk = &parent
		}
	}

	// Get child chunks
	var childChunks []models.Chunk
	tenantRepo.DB().Where("parent_chunk_id = ?", chunkID).Find(&childChunks)

	// Get related chunks based on relationships
	relatedChunks := make([]models.Chunk, 0)
	for _, relationship := range chunk.Relationships {
		parts := strings.Split(relationship, ":")
		if len(parts) == 2 {
			if relatedID, err := uuid.Parse(parts[1]); err == nil {
				var related models.Chunk
				if err := tenantRepo.DB().Where("id = ?", relatedID).First(&related).Error; err == nil {
					relatedChunks = append(relatedChunks, related)
				}
			}
		}
	}

	relationships := map[string]interface{}{
		"chunk_id":      chunkID,
		"parent_chunk":  parentChunk,
		"child_chunks":  childChunks,
		"related_chunks": relatedChunks,
		"relationships": chunk.Relationships,
	}

	response.WriteSuccess(w, getRequestID(r), relationships, nil)
}

// Helper methods

func (h *ChunkHandler) toChunkResponse(chunk *models.Chunk) ChunkResponse {
	response := ChunkResponse{
		ID:               chunk.ID,
		TenantID:         chunk.TenantID,
		FileID:           chunk.FileID,
		ChunkID:          chunk.ChunkID,
		ChunkType:        chunk.ChunkType,
		ChunkNumber:      chunk.ChunkNumber,
		Content:          chunk.Content,
		ContentHash:      chunk.ContentHash,
		SizeBytes:        chunk.SizeBytes,
		StartPosition:    chunk.StartPosition,
		EndPosition:      chunk.EndPosition,
		PageNumber:       chunk.PageNumber,
		LineNumber:       chunk.LineNumber,
		ParentChunkID:    chunk.ParentChunkID,
		Relationships:    chunk.Relationships,
		ProcessedAt:      chunk.ProcessedAt.Format("2006-01-02T15:04:05Z"),
		ProcessedBy:      chunk.ProcessedBy,
		ProcessingTime:   chunk.ProcessingTime,
		Quality:          chunk.Quality,
		Language:         chunk.Language,
		LanguageConf:     chunk.LanguageConf,
		ContentCategory:  chunk.ContentCategory,
		SensitivityLevel: chunk.SensitivityLevel,
		Classifications:  chunk.Classifications,
		EmbeddingStatus:  chunk.EmbeddingStatus,
		EmbeddingModel:   chunk.EmbeddingModel,
		EmbeddingDim:     chunk.EmbeddingDim,
		PIIDetected:      chunk.PIIDetected,
		ComplianceFlags:  chunk.ComplianceFlags,
		DLPScanStatus:    chunk.DLPScanStatus,
		DLPScanResult:    chunk.DLPScanResult,
		Context:          chunk.Context,
		SchemaInfo:       chunk.SchemaInfo,
		Metadata:         chunk.Metadata,
		CustomFields:     chunk.CustomFields,
		CreatedAt:        chunk.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:        chunk.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if chunk.EmbeddedAt != nil {
		embeddedAt := chunk.EmbeddedAt.Format("2006-01-02T15:04:05Z")
		response.EmbeddedAt = &embeddedAt
	}

	return response
}