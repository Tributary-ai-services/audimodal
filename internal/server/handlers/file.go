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

// FileHandler handles file HTTP requests
type FileHandler struct {
	db *database.Database
}

// NewFileHandler creates a new file handler
func NewFileHandler(db *database.Database) *FileHandler {
	return &FileHandler{db: db}
}

// FileResponse represents a file response
type FileResponse struct {
	ID                   uuid.UUID                  `json:"id"`
	TenantID             uuid.UUID                  `json:"tenant_id"`
	DataSourceID         *uuid.UUID                 `json:"data_source_id,omitempty"`
	ProcessingSessionID  *uuid.UUID                 `json:"processing_session_id,omitempty"`
	URL                  string                     `json:"url"`
	Path                 string                     `json:"path"`
	Filename             string                     `json:"filename"`
	Extension            string                     `json:"extension"`
	ContentType          string                     `json:"content_type"`
	Size                 int64                      `json:"size"`
	Checksum             string                     `json:"checksum"`
	ChecksumType         string                     `json:"checksum_type"`
	LastModified         string                     `json:"last_modified"`
	Status               string                     `json:"status"`
	ProcessingTier       string                     `json:"processing_tier"`
	ProcessedAt          *string                    `json:"processed_at,omitempty"`
	ProcessingError      *string                    `json:"processing_error,omitempty"`
	ProcessingDuration   *int64                     `json:"processing_duration,omitempty"`
	Language             string                     `json:"language,omitempty"`
	LanguageConf         float64                    `json:"language_confidence,omitempty"`
	ContentCategory      string                     `json:"content_category,omitempty"`
	SensitivityLevel     string                     `json:"sensitivity_level,omitempty"`
	Classifications      []string                   `json:"classifications,omitempty"`
	SchemaInfo           models.FileSchemaInfo      `json:"schema_info"`
	ChunkCount           int                        `json:"chunk_count"`
	ChunkingStrategy     string                     `json:"chunking_strategy,omitempty"`
	PIIDetected          bool                       `json:"pii_detected"`
	ComplianceFlags      []string                   `json:"compliance_flags,omitempty"`
	EncryptionStatus     string                     `json:"encryption_status"`
	Metadata             map[string]interface{}     `json:"metadata,omitempty"`
	CustomFields         map[string]interface{}     `json:"custom_fields,omitempty"`
	CreatedAt            string                     `json:"created_at"`
	UpdatedAt            string                     `json:"updated_at"`
}

// ServeHTTP implements the http.Handler interface
func (h *FileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tenantCtx := getTenantContext(r.Context())
	if tenantCtx == nil {
		response.WriteBadRequest(w, getRequestID(r), "Tenant context required", nil)
		return
	}

	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	
	// Find files in the path
	var fileIndex int = -1
	for i, part := range parts {
		if part == "files" {
			fileIndex = i
			break
		}
	}

	if fileIndex == -1 {
		response.WriteNotFound(w, getRequestID(r), "File endpoint not found")
		return
	}

	// Handle different routes
	if len(parts) == fileIndex+1 {
		// /api/v1/tenants/{tenant_id}/files
		switch r.Method {
		case http.MethodGet:
			h.ListFiles(w, r, tenantCtx.TenantID)
		case http.MethodPost:
			h.CreateFile(w, r, tenantCtx.TenantID)
		default:
			response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		}
		return
	}

	if len(parts) >= fileIndex+2 {
		fileID, err := uuid.Parse(parts[fileIndex+1])
		if err != nil {
			response.WriteBadRequest(w, getRequestID(r), "Invalid file ID format", nil)
			return
		}

		if len(parts) == fileIndex+2 {
			// /api/v1/tenants/{tenant_id}/files/{file_id}
			switch r.Method {
			case http.MethodGet:
				h.GetFile(w, r, tenantCtx.TenantID, fileID)
			case http.MethodPut:
				h.UpdateFile(w, r, tenantCtx.TenantID, fileID)
			case http.MethodDelete:
				h.DeleteFile(w, r, tenantCtx.TenantID, fileID)
			default:
				response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
			}
			return
		}

		// Handle sub-resources
		if len(parts) >= fileIndex+3 {
			subResource := parts[fileIndex+2]
			switch subResource {
			case "chunks":
				h.ListFileChunks(w, r, tenantCtx.TenantID, fileID)
			case "process":
				h.ProcessFile(w, r, tenantCtx.TenantID, fileID)
			case "download":
				h.DownloadFile(w, r, tenantCtx.TenantID, fileID)
			case "metadata":
				h.GetFileMetadata(w, r, tenantCtx.TenantID, fileID)
			case "violations":
				h.ListFileViolations(w, r, tenantCtx.TenantID, fileID)
			default:
				response.WriteNotFound(w, getRequestID(r), "Resource not found")
			}
			return
		}
	}

	response.WriteNotFound(w, getRequestID(r), "Endpoint not found")
}

// ListFiles handles GET /api/v1/tenants/{tenant_id}/files
func (h *FileHandler) ListFiles(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	page, pageSize, offset := getPagination(r.Context())
	
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var files []models.File
	query := tenantRepo.DB().Limit(pageSize).Offset(offset).Order("created_at DESC")
	
	// Apply filters
	if status := r.URL.Query().Get("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	
	if contentType := r.URL.Query().Get("content_type"); contentType != "" {
		query = query.Where("content_type = ?", contentType)
	}
	
	if extension := r.URL.Query().Get("extension"); extension != "" {
		query = query.Where("extension = ?", extension)
	}
	
	if dataSourceID := r.URL.Query().Get("data_source_id"); dataSourceID != "" {
		if dsID, err := uuid.Parse(dataSourceID); err == nil {
			query = query.Where("data_source_id = ?", dsID)
		}
	}
	
	if sessionID := r.URL.Query().Get("session_id"); sessionID != "" {
		if sID, err := uuid.Parse(sessionID); err == nil {
			query = query.Where("processing_session_id = ?", sID)
		}
	}
	
	if piiDetected := r.URL.Query().Get("pii_detected"); piiDetected != "" {
		detected := piiDetected == "true"
		query = query.Where("pii_detected = ?", detected)
	}
	
	err = query.Find(&files).Error
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list files", err.Error())
		return
	}

	// Convert to response format
	response := make([]FileResponse, len(files))
	for i, file := range files {
		response[i] = h.toFileResponse(&file)
	}

	// Get total count
	var totalCount int64
	countQuery := tenantRepo.DB().Model(&models.File{})
	// Apply same filters for count
	if status := r.URL.Query().Get("status"); status != "" {
		countQuery = countQuery.Where("status = ?", status)
	}
	if contentType := r.URL.Query().Get("content_type"); contentType != "" {
		countQuery = countQuery.Where("content_type = ?", contentType)
	}
	// ... other filters
	countQuery.Count(&totalCount)

	response.WritePaginated(w, getRequestID(r), response, page, pageSize, totalCount)
}

// CreateFile handles POST /api/v1/tenants/{tenant_id}/files
func (h *FileHandler) CreateFile(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	var req struct {
		URL                 string                     `json:"url"`
		Path                string                     `json:"path"`
		Filename            string                     `json:"filename"`
		Extension           string                     `json:"extension"`
		ContentType         string                     `json:"content_type"`
		Size                int64                      `json:"size"`
		Checksum            string                     `json:"checksum"`
		ChecksumType        string                     `json:"checksum_type"`
		DataSourceID        *uuid.UUID                 `json:"data_source_id,omitempty"`
		ProcessingSessionID *uuid.UUID                 `json:"processing_session_id,omitempty"`
		Metadata            map[string]interface{}     `json:"metadata,omitempty"`
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

	// Create file record
	file := &models.File{
		TenantID:            tenantID,
		DataSourceID:        req.DataSourceID,
		ProcessingSessionID: req.ProcessingSessionID,
		URL:                 req.URL,
		Path:                req.Path,
		Filename:            req.Filename,
		Extension:           req.Extension,
		ContentType:         req.ContentType,
		Size:                req.Size,
		Checksum:            req.Checksum,
		ChecksumType:        req.ChecksumType,
		Status:              models.FileStatusDiscovered,
		EncryptionStatus:    "none",
		Metadata:            req.Metadata,
	}

	if err := tenantRepo.ValidateAndCreate(file); err != nil {
		if strings.Contains(err.Error(), "validation") {
			response.WriteValidationError(w, getRequestID(r), err.Error())
			return
		}
		response.WriteInternalServerError(w, getRequestID(r), "Failed to create file", err.Error())
		return
	}

	response := h.toFileResponse(file)
	response.WriteCreated(w, getRequestID(r), response)
}

// GetFile handles GET /api/v1/tenants/{tenant_id}/files/{id}
func (h *FileHandler) GetFile(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var file models.File
	err = tenantRepo.DB().Where("id = ?", fileID).First(&file).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "File not found")
		return
	}

	response := h.toFileResponse(&file)
	response.WriteSuccess(w, getRequestID(r), response, nil)
}

// UpdateFile handles PUT /api/v1/tenants/{tenant_id}/files/{id}
func (h *FileHandler) UpdateFile(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
	var req struct {
		Status           string                 `json:"status"`
		ContentCategory  string                 `json:"content_category"`
		SensitivityLevel string                 `json:"sensitivity_level"`
		Classifications  []string               `json:"classifications"`
		ComplianceFlags  []string               `json:"compliance_flags"`
		Metadata         map[string]interface{} `json:"metadata"`
		CustomFields     map[string]interface{} `json:"custom_fields"`
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

	var file models.File
	err = tenantRepo.DB().Where("id = ?", fileID).First(&file).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "File not found")
		return
	}

	// Update allowed fields
	if req.Status != "" {
		file.Status = req.Status
	}
	if req.ContentCategory != "" {
		file.ContentCategory = req.ContentCategory
	}
	if req.SensitivityLevel != "" {
		file.SensitivityLevel = req.SensitivityLevel
	}
	if req.Classifications != nil {
		file.Classifications = req.Classifications
	}
	if req.ComplianceFlags != nil {
		file.ComplianceFlags = req.ComplianceFlags
	}
	if req.Metadata != nil {
		file.Metadata = req.Metadata
	}
	if req.CustomFields != nil {
		file.CustomFields = req.CustomFields
	}

	if err := tenantRepo.ValidateAndUpdate(&file); err != nil {
		if strings.Contains(err.Error(), "validation") {
			response.WriteValidationError(w, getRequestID(r), err.Error())
			return
		}
		response.WriteInternalServerError(w, getRequestID(r), "Failed to update file", err.Error())
		return
	}

	response := h.toFileResponse(&file)
	response.WriteSuccess(w, getRequestID(r), response, nil)
}

// DeleteFile handles DELETE /api/v1/tenants/{tenant_id}/files/{id}
func (h *FileHandler) DeleteFile(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var file models.File
	err = tenantRepo.DB().Where("id = ?", fileID).First(&file).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "File not found")
		return
	}

	if err := tenantRepo.DB().Delete(&file).Error; err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to delete file", err.Error())
		return
	}

	response.WriteNoContent(w)
}

// ListFileChunks handles GET /api/v1/tenants/{tenant_id}/files/{id}/chunks
func (h *FileHandler) ListFileChunks(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
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

	// Check if file exists
	var file models.File
	err = tenantRepo.DB().Where("id = ?", fileID).First(&file).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "File not found")
		return
	}

	// Get chunks for this file
	var chunks []models.Chunk
	err = tenantRepo.DB().Where("file_id = ?", fileID).
		Limit(pageSize).Offset(offset).Order("chunk_number ASC").Find(&chunks).Error
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list chunks", err.Error())
		return
	}

	// Get total count
	var totalCount int64
	tenantRepo.DB().Model(&models.Chunk{}).Where("file_id = ?", fileID).Count(&totalCount)

	response.WritePaginated(w, getRequestID(r), chunks, page, pageSize, totalCount)
}

// ProcessFile handles POST /api/v1/tenants/{tenant_id}/files/{id}/process
func (h *FileHandler) ProcessFile(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
	if r.Method != http.MethodPost {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	var req struct {
		ChunkingStrategy string `json:"chunking_strategy"`
		Priority         string `json:"priority"`
		DLPScanEnabled   bool   `json:"dlp_scan_enabled"`
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

	var file models.File
	err = tenantRepo.DB().Where("id = ?", fileID).First(&file).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "File not found")
		return
	}

	// Start processing
	file.StartProcessing()
	if req.ChunkingStrategy != "" {
		file.ChunkingStrategy = req.ChunkingStrategy
	}

	if err := tenantRepo.DB().Save(&file).Error; err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to start processing", err.Error())
		return
	}

	// TODO: Trigger actual file processing pipeline

	response := map[string]interface{}{
		"message":  "File processing started",
		"file_id":  fileID,
		"status":   file.Status,
		"strategy": file.ChunkingStrategy,
	}

	response.WriteSuccess(w, getRequestID(r), response, nil)
}

// DownloadFile handles GET /api/v1/tenants/{tenant_id}/files/{id}/download
func (h *FileHandler) DownloadFile(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
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

	var file models.File
	err = tenantRepo.DB().Where("id = ?", fileID).First(&file).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "File not found")
		return
	}

	// TODO: Implement actual file download
	// This would involve resolving the file URL and streaming the content
	downloadInfo := map[string]interface{}{
		"download_url": file.URL,
		"filename":     file.Filename,
		"content_type": file.ContentType,
		"size":         file.Size,
		"expires_at":   "2025-01-03T13:00:00Z", // Temporary download link
	}

	response.WriteSuccess(w, getRequestID(r), downloadInfo, nil)
}

// GetFileMetadata handles GET /api/v1/tenants/{tenant_id}/files/{id}/metadata
func (h *FileHandler) GetFileMetadata(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
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

	var file models.File
	err = tenantRepo.DB().Where("id = ?", fileID).First(&file).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "File not found")
		return
	}

	metadata := map[string]interface{}{
		"schema_info":        file.SchemaInfo,
		"metadata":           file.Metadata,
		"custom_fields":      file.CustomFields,
		"classifications":    file.Classifications,
		"compliance_flags":   file.ComplianceFlags,
		"processing_tier":    file.GetProcessingTier(),
		"sensitivity_level":  file.GetSensitivityLevel(),
		"pii_detected":       file.HasPII(),
		"is_structured":      file.IsStructuredData(),
		"is_unstructured":    file.IsUnstructuredData(),
		"chunk_count":        file.ChunkCount,
		"chunking_strategy":  file.ChunkingStrategy,
	}

	response.WriteSuccess(w, getRequestID(r), metadata, nil)
}

// ListFileViolations handles GET /api/v1/tenants/{tenant_id}/files/{id}/violations
func (h *FileHandler) ListFileViolations(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
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

	// Check if file exists
	var file models.File
	err = tenantRepo.DB().Where("id = ?", fileID).First(&file).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "File not found")
		return
	}

	// Get violations for this file
	var violations []models.DLPViolation
	err = tenantRepo.DB().Where("file_id = ?", fileID).
		Limit(pageSize).Offset(offset).Order("created_at DESC").Find(&violations).Error
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list violations", err.Error())
		return
	}

	// Get total count
	var totalCount int64
	tenantRepo.DB().Model(&models.DLPViolation{}).Where("file_id = ?", fileID).Count(&totalCount)

	response.WritePaginated(w, getRequestID(r), violations, page, pageSize, totalCount)
}

// Helper methods

func (h *FileHandler) toFileResponse(file *models.File) FileResponse {
	response := FileResponse{
		ID:                   file.ID,
		TenantID:             file.TenantID,
		DataSourceID:         file.DataSourceID,
		ProcessingSessionID:  file.ProcessingSessionID,
		URL:                  file.URL,
		Path:                 file.Path,
		Filename:             file.Filename,
		Extension:            file.Extension,
		ContentType:          file.ContentType,
		Size:                 file.Size,
		Checksum:             file.Checksum,
		ChecksumType:         file.ChecksumType,
		LastModified:         file.LastModified.Format("2006-01-02T15:04:05Z"),
		Status:               file.Status,
		ProcessingTier:       file.ProcessingTier,
		ProcessingError:      file.ProcessingError,
		ProcessingDuration:   file.ProcessingDuration,
		Language:             file.Language,
		LanguageConf:         file.LanguageConf,
		ContentCategory:      file.ContentCategory,
		SensitivityLevel:     file.SensitivityLevel,
		Classifications:      file.Classifications,
		SchemaInfo:           file.SchemaInfo,
		ChunkCount:           file.ChunkCount,
		ChunkingStrategy:     file.ChunkingStrategy,
		PIIDetected:          file.PIIDetected,
		ComplianceFlags:      file.ComplianceFlags,
		EncryptionStatus:     file.EncryptionStatus,
		Metadata:             file.Metadata,
		CustomFields:         file.CustomFields,
		CreatedAt:            file.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:            file.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if file.ProcessedAt != nil {
		processedAt := file.ProcessedAt.Format("2006-01-02T15:04:05Z")
		response.ProcessedAt = &processedAt
	}

	return response
}