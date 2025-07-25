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

// ProcessingSessionHandler handles processing session HTTP requests
type ProcessingSessionHandler struct {
	db *database.Database
}

// NewProcessingSessionHandler creates a new processing session handler
func NewProcessingSessionHandler(db *database.Database) *ProcessingSessionHandler {
	return &ProcessingSessionHandler{db: db}
}

// ProcessingSessionRequest represents a processing session creation/update request
type ProcessingSessionRequest struct {
	Name        string                      `json:"name"`
	DisplayName string                      `json:"display_name"`
	Files       []models.ProcessingFileSpec `json:"files"`
	Options     models.ProcessingOptions    `json:"options"`
}

// ProcessingSessionResponse represents a processing session response
type ProcessingSessionResponse struct {
	ID             uuid.UUID                   `json:"id"`
	TenantID       uuid.UUID                   `json:"tenant_id"`
	Name           string                      `json:"name"`
	DisplayName    string                      `json:"display_name"`
	Files          []models.ProcessingFileSpec `json:"files"`
	Options        models.ProcessingOptions    `json:"options"`
	Status         string                      `json:"status"`
	Progress       float64                     `json:"progress"`
	TotalFiles     int                         `json:"total_files"`
	ProcessedFiles int                         `json:"processed_files"`
	FailedFiles    int                         `json:"failed_files"`
	StartedAt      *string                     `json:"started_at,omitempty"`
	CompletedAt    *string                     `json:"completed_at,omitempty"`
	CreatedAt      string                      `json:"created_at"`
	UpdatedAt      string                      `json:"updated_at"`
	LastError      *string                     `json:"last_error,omitempty"`
	ErrorCount     int                         `json:"error_count"`
	RetryCount     int                         `json:"retry_count"`
	ProcessedBytes int64                       `json:"processed_bytes"`
	TotalBytes     int64                       `json:"total_bytes"`
	ChunksCreated  int64                       `json:"chunks_created"`
}

// ServeHTTP implements the http.Handler interface
func (h *ProcessingSessionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tenantCtx := getTenantContext(r.Context())
	if tenantCtx == nil {
		response.WriteBadRequest(w, getRequestID(r), "Tenant context required", nil)
		return
	}

	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	
	// Find sessions in the path
	var sessionIndex int = -1
	for i, part := range parts {
		if part == "sessions" {
			sessionIndex = i
			break
		}
	}

	if sessionIndex == -1 {
		response.WriteNotFound(w, getRequestID(r), "Session endpoint not found")
		return
	}

	// Handle different routes
	if len(parts) == sessionIndex+1 {
		// /api/v1/tenants/{tenant_id}/sessions
		switch r.Method {
		case http.MethodGet:
			h.ListSessions(w, r, tenantCtx.TenantID)
		case http.MethodPost:
			h.CreateSession(w, r, tenantCtx.TenantID)
		default:
			response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		}
		return
	}

	if len(parts) >= sessionIndex+2 {
		sessionID, err := uuid.Parse(parts[sessionIndex+1])
		if err != nil {
			response.WriteBadRequest(w, getRequestID(r), "Invalid session ID format", nil)
			return
		}

		if len(parts) == sessionIndex+2 {
			// /api/v1/tenants/{tenant_id}/sessions/{session_id}
			switch r.Method {
			case http.MethodGet:
				h.GetSession(w, r, tenantCtx.TenantID, sessionID)
			case http.MethodPut:
				h.UpdateSession(w, r, tenantCtx.TenantID, sessionID)
			case http.MethodDelete:
				h.DeleteSession(w, r, tenantCtx.TenantID, sessionID)
			default:
				response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
			}
			return
		}

		// Handle sub-resources
		if len(parts) >= sessionIndex+3 {
			subResource := parts[sessionIndex+2]
			switch subResource {
			case "start":
				h.StartSession(w, r, tenantCtx.TenantID, sessionID)
			case "stop":
				h.StopSession(w, r, tenantCtx.TenantID, sessionID)
			case "retry":
				h.RetrySession(w, r, tenantCtx.TenantID, sessionID)
			case "files":
				h.ListSessionFiles(w, r, tenantCtx.TenantID, sessionID)
			case "status":
				h.GetSessionStatus(w, r, tenantCtx.TenantID, sessionID)
			default:
				response.WriteNotFound(w, getRequestID(r), "Resource not found")
			}
			return
		}
	}

	response.WriteNotFound(w, getRequestID(r), "Endpoint not found")
}

// ListSessions handles GET /api/v1/tenants/{tenant_id}/sessions
func (h *ProcessingSessionHandler) ListSessions(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	page, pageSize, offset := getPagination(r.Context())
	
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var sessions []models.ProcessingSession
	query := tenantRepo.DB().Limit(pageSize).Offset(offset).Order("created_at DESC")
	
	// Filter by status if provided
	if status := r.URL.Query().Get("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	
	err = query.Find(&sessions).Error
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list sessions", err.Error())
		return
	}

	// Convert to response format
	responseData := make([]ProcessingSessionResponse, len(sessions))
	for i, session := range sessions {
		responseData[i] = h.toSessionResponse(&session)
	}

	// Get total count
	var totalCount int64
	countQuery := tenantRepo.DB().Model(&models.ProcessingSession{})
	if status := r.URL.Query().Get("status"); status != "" {
		countQuery = countQuery.Where("status = ?", status)
	}
	countQuery.Count(&totalCount)

	response.WritePaginated(w, getRequestID(r), responseData, page, pageSize, totalCount)
}

// CreateSession handles POST /api/v1/tenants/{tenant_id}/sessions
func (h *ProcessingSessionHandler) CreateSession(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	var req ProcessingSessionRequest
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

	// Calculate total files and bytes
	totalFiles := len(req.Files)
	var totalBytes int64
	for _, file := range req.Files {
		totalBytes += file.Size
	}

	// Create session
	session := &models.ProcessingSession{
		TenantID:    tenantID,
		Name:        req.Name,
		DisplayName: req.DisplayName,
		FileSpecs:   req.Files,
		Options:     req.Options,
		Status:      models.SessionStatusPending,
		TotalFiles:  totalFiles,
		TotalBytes:  totalBytes,
	}

	if err := tenantRepo.ValidateAndCreate(session); err != nil {
		if strings.Contains(err.Error(), "validation") {
			response.WriteValidationError(w, getRequestID(r), err.Error())
			return
		}
		response.WriteInternalServerError(w, getRequestID(r), "Failed to create session", err.Error())
		return
	}

	responseData := h.toSessionResponse(session)
	response.WriteCreated(w, getRequestID(r), responseData)
}

// GetSession handles GET /api/v1/tenants/{tenant_id}/sessions/{id}
func (h *ProcessingSessionHandler) GetSession(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, sessionID uuid.UUID) {
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var session models.ProcessingSession
	err = tenantRepo.DB().Where("id = ?", sessionID).First(&session).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Session not found")
		return
	}

	responseData := h.toSessionResponse(&session)
	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// UpdateSession handles PUT /api/v1/tenants/{tenant_id}/sessions/{id}
func (h *ProcessingSessionHandler) UpdateSession(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, sessionID uuid.UUID) {
	var req ProcessingSessionRequest
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

	var session models.ProcessingSession
	err = tenantRepo.DB().Where("id = ?", sessionID).First(&session).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Session not found")
		return
	}

	// Only allow updates if session is not running
	if session.Status == models.SessionStatusRunning {
		response.WriteBadRequest(w, getRequestID(r), "Cannot update running session", nil)
		return
	}

	// Update fields
	session.Name = req.Name
	session.DisplayName = req.DisplayName
	session.FileSpecs = req.Files
	session.Options = req.Options
	session.TotalFiles = len(req.Files)
	
	var totalBytes int64
	for _, file := range req.Files {
		totalBytes += file.Size
	}
	session.TotalBytes = totalBytes

	if err := tenantRepo.ValidateAndUpdate(&session); err != nil {
		if strings.Contains(err.Error(), "validation") {
			response.WriteValidationError(w, getRequestID(r), err.Error())
			return
		}
		response.WriteInternalServerError(w, getRequestID(r), "Failed to update session", err.Error())
		return
	}

	responseData := h.toSessionResponse(&session)
	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// DeleteSession handles DELETE /api/v1/tenants/{tenant_id}/sessions/{id}
func (h *ProcessingSessionHandler) DeleteSession(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, sessionID uuid.UUID) {
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var session models.ProcessingSession
	err = tenantRepo.DB().Where("id = ?", sessionID).First(&session).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Session not found")
		return
	}

	// Only allow deletion if session is not running
	if session.Status == models.SessionStatusRunning {
		response.WriteBadRequest(w, getRequestID(r), "Cannot delete running session", nil)
		return
	}

	if err := tenantRepo.DB().Delete(&session).Error; err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to delete session", err.Error())
		return
	}

	response.WriteNoContent(w)
}

// StartSession handles POST /api/v1/tenants/{tenant_id}/sessions/{id}/start
func (h *ProcessingSessionHandler) StartSession(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, sessionID uuid.UUID) {
	if r.Method != http.MethodPost {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var session models.ProcessingSession
	err = tenantRepo.DB().Where("id = ?", sessionID).First(&session).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Session not found")
		return
	}

	if session.Status != models.SessionStatusPending && session.Status != models.SessionStatusFailed {
		response.WriteBadRequest(w, getRequestID(r), "Session cannot be started in current state", nil)
		return
	}

	// Start the session
	session.Start()
	
	if err := tenantRepo.DB().Save(&session).Error; err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to start session", err.Error())
		return
	}

	// TODO: Trigger actual processing pipeline
	
	responseData := h.toSessionResponse(&session)
	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// StopSession handles POST /api/v1/tenants/{tenant_id}/sessions/{id}/stop
func (h *ProcessingSessionHandler) StopSession(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, sessionID uuid.UUID) {
	if r.Method != http.MethodPost {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var session models.ProcessingSession
	err = tenantRepo.DB().Where("id = ?", sessionID).First(&session).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Session not found")
		return
	}

	if session.Status != models.SessionStatusRunning {
		response.WriteBadRequest(w, getRequestID(r), "Session is not running", nil)
		return
	}

	// Cancel the session
	session.Cancel()
	
	if err := tenantRepo.DB().Save(&session).Error; err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to stop session", err.Error())
		return
	}

	responseData := h.toSessionResponse(&session)
	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// RetrySession handles POST /api/v1/tenants/{tenant_id}/sessions/{id}/retry
func (h *ProcessingSessionHandler) RetrySession(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, sessionID uuid.UUID) {
	if r.Method != http.MethodPost {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var session models.ProcessingSession
	err = tenantRepo.DB().Where("id = ?", sessionID).First(&session).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Session not found")
		return
	}

	if !session.CanRetry() {
		response.WriteBadRequest(w, getRequestID(r), "Session cannot be retried", nil)
		return
	}

	// Reset session for retry
	session.Status = models.SessionStatusPending
	session.RetryCount++
	session.ProcessedFiles = 0
	session.FailedFiles = 0
	session.Progress = 0
	session.StartedAt = nil
	session.CompletedAt = nil
	session.LastError = nil
	
	if err := tenantRepo.DB().Save(&session).Error; err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to retry session", err.Error())
		return
	}

	responseData := h.toSessionResponse(&session)
	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// ListSessionFiles handles GET /api/v1/tenants/{tenant_id}/sessions/{id}/files
func (h *ProcessingSessionHandler) ListSessionFiles(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, sessionID uuid.UUID) {
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

	// Check if session exists
	var session models.ProcessingSession
	err = tenantRepo.DB().Where("id = ?", sessionID).First(&session).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Session not found")
		return
	}

	// Get files for this session
	var files []models.File
	err = tenantRepo.DB().Where("processing_session_id = ?", sessionID).
		Limit(pageSize).Offset(offset).Find(&files).Error
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list files", err.Error())
		return
	}

	// Get total count
	var totalCount int64
	tenantRepo.DB().Model(&models.File{}).Where("processing_session_id = ?", sessionID).Count(&totalCount)

	response.WritePaginated(w, getRequestID(r), files, page, pageSize, totalCount)
}

// GetSessionStatus handles GET /api/v1/tenants/{tenant_id}/sessions/{id}/status
func (h *ProcessingSessionHandler) GetSessionStatus(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, sessionID uuid.UUID) {
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

	var session models.ProcessingSession
	err = tenantRepo.DB().Where("id = ?", sessionID).First(&session).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Session not found")
		return
	}

	status := map[string]interface{}{
		"id":              session.ID,
		"status":          session.Status,
		"progress":        session.Progress,
		"total_files":     session.TotalFiles,
		"processed_files": session.ProcessedFiles,
		"failed_files":    session.FailedFiles,
		"processed_bytes": session.ProcessedBytes,
		"total_bytes":     session.TotalBytes,
		"chunks_created":  session.ChunksCreated,
		"error_count":     session.ErrorCount,
		"retry_count":     session.RetryCount,
		"last_error":      session.LastError,
		"started_at":      session.StartedAt,
		"completed_at":    session.CompletedAt,
		"updated_at":      session.UpdatedAt,
	}

	response.WriteSuccess(w, getRequestID(r), status, nil)
}

// Helper methods

func (h *ProcessingSessionHandler) toSessionResponse(session *models.ProcessingSession) ProcessingSessionResponse {
	responseData := ProcessingSessionResponse{
		ID:             session.ID,
		TenantID:       session.TenantID,
		Name:           session.Name,
		DisplayName:    session.DisplayName,
		Files:          session.FileSpecs,
		Options:        session.Options,
		Status:         session.Status,
		Progress:       session.Progress,
		TotalFiles:     session.TotalFiles,
		ProcessedFiles: session.ProcessedFiles,
		FailedFiles:    session.FailedFiles,
		CreatedAt:      session.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      session.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		LastError:      session.LastError,
		ErrorCount:     session.ErrorCount,
		RetryCount:     session.RetryCount,
		ProcessedBytes: session.ProcessedBytes,
		TotalBytes:     session.TotalBytes,
		ChunksCreated:  session.ChunksCreated,
	}

	if session.StartedAt != nil {
		startedAt := session.StartedAt.Format("2006-01-02T15:04:05Z")
		responseData.StartedAt = &startedAt
	}

	if session.CompletedAt != nil {
		completedAt := session.CompletedAt.Format("2006-01-02T15:04:05Z")
		responseData.CompletedAt = &completedAt
	}

	return responseData
}