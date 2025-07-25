package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/internal/database/models"
	"github.com/jscharber/eAIIngest/internal/server/response"
)

// DataSourceHandler handles data source HTTP requests
type DataSourceHandler struct {
	db *database.Database
}

// NewDataSourceHandler creates a new data source handler
func NewDataSourceHandler(db *database.Database) *DataSourceHandler {
	return &DataSourceHandler{db: db}
}

// DataSourceRequest represents a data source creation/update request
type DataSourceRequest struct {
	Name               string                                `json:"name"`
	DisplayName        string                                `json:"display_name"`
	Type               string                                `json:"type"`
	Config             models.DataSourceConfig               `json:"config"`
	CredentialsRef     models.DataSourceCredentials          `json:"credentials_ref"`
	SyncSettings       models.DataSourceSyncSettings         `json:"sync_settings"`
	ProcessingSettings models.DataSourceProcessingSettings   `json:"processing_settings"`
}

// DataSourceResponse represents a data source response
type DataSourceResponse struct {
	ID                 uuid.UUID                             `json:"id"`
	TenantID           uuid.UUID                             `json:"tenant_id"`
	Name               string                                `json:"name"`
	DisplayName        string                                `json:"display_name"`
	Type               string                                `json:"type"`
	Config             models.DataSourceConfig               `json:"config"`
	CredentialsRef     models.DataSourceCredentials          `json:"credentials_ref"`
	SyncSettings       models.DataSourceSyncSettings         `json:"sync_settings"`
	ProcessingSettings models.DataSourceProcessingSettings   `json:"processing_settings"`
	Status             string                                `json:"status"`
	LastSyncAt         *string                               `json:"last_sync_at,omitempty"`
	LastSyncStatus     string                                `json:"last_sync_status"`
	LastSyncError      *string                               `json:"last_sync_error,omitempty"`
	CreatedAt          string                                `json:"created_at"`
	UpdatedAt          string                                `json:"updated_at"`
}

// ServeHTTP implements the http.Handler interface
func (h *DataSourceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract tenant ID and data source path
	tenantCtx := getTenantContext(r.Context())
	if tenantCtx == nil {
		response.WriteBadRequest(w, getRequestID(r), "Tenant context required", nil)
		return
	}

	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	
	// Find data-sources in the path
	var dataSourceIndex int = -1
	for i, part := range parts {
		if part == "data-sources" {
			dataSourceIndex = i
			break
		}
	}

	if dataSourceIndex == -1 {
		response.WriteNotFound(w, getRequestID(r), "Data source endpoint not found")
		return
	}

	// Handle different routes
	if len(parts) == dataSourceIndex+1 {
		// /api/v1/tenants/{tenant_id}/data-sources
		switch r.Method {
		case http.MethodGet:
			h.ListDataSources(w, r, tenantCtx.TenantID)
		case http.MethodPost:
			h.CreateDataSource(w, r, tenantCtx.TenantID)
		default:
			response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		}
		return
	}

	if len(parts) >= dataSourceIndex+2 {
		// /api/v1/tenants/{tenant_id}/data-sources/{data_source_id}
		dataSourceID, err := uuid.Parse(parts[dataSourceIndex+1])
		if err != nil {
			response.WriteBadRequest(w, getRequestID(r), "Invalid data source ID format", nil)
			return
		}

		if len(parts) == dataSourceIndex+2 {
			// /api/v1/tenants/{tenant_id}/data-sources/{data_source_id}
			switch r.Method {
			case http.MethodGet:
				h.GetDataSource(w, r, tenantCtx.TenantID, dataSourceID)
			case http.MethodPut:
				h.UpdateDataSource(w, r, tenantCtx.TenantID, dataSourceID)
			case http.MethodDelete:
				h.DeleteDataSource(w, r, tenantCtx.TenantID, dataSourceID)
			default:
				response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
			}
			return
		}

		// Handle sub-resources
		if len(parts) >= dataSourceIndex+3 {
			subResource := parts[dataSourceIndex+2]
			switch subResource {
			case "sync":
				h.TriggerSync(w, r, tenantCtx.TenantID, dataSourceID)
			case "test":
				h.TestConnection(w, r, tenantCtx.TenantID, dataSourceID)
			case "files":
				h.ListDataSourceFiles(w, r, tenantCtx.TenantID, dataSourceID)
			default:
				response.WriteNotFound(w, getRequestID(r), "Resource not found")
			}
			return
		}
	}

	response.WriteNotFound(w, getRequestID(r), "Endpoint not found")
}

// ListDataSources handles GET /api/v1/tenants/{tenant_id}/data-sources
func (h *DataSourceHandler) ListDataSources(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	page, pageSize, offset := getPagination(r.Context())
	
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var dataSources []models.DataSource
	err = tenantRepo.DB().Limit(pageSize).Offset(offset).Find(&dataSources).Error
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list data sources", err.Error())
		return
	}

	// Convert to response format
	responseData := make([]DataSourceResponse, len(dataSources))
	for i, ds := range dataSources {
		responseData[i] = h.toDataSourceResponse(&ds)
	}

	// Get total count
	var totalCount int64
	tenantRepo.DB().Model(&models.DataSource{}).Count(&totalCount)

	response.WritePaginated(w, getRequestID(r), responseData, page, pageSize, totalCount)
}

// CreateDataSource handles POST /api/v1/tenants/{tenant_id}/data-sources
func (h *DataSourceHandler) CreateDataSource(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	var req DataSourceRequest
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

	// Create data source
	dataSource := &models.DataSource{
		TenantID:           tenantID,
		Name:               req.Name,
		DisplayName:        req.DisplayName,
		Type:               req.Type,
		Config:             req.Config,
		CredentialsRef:     req.CredentialsRef,
		SyncSettings:       req.SyncSettings,
		ProcessingSettings: req.ProcessingSettings,
		Status:             models.DataSourceStatusActive,
		LastSyncStatus:     "pending",
	}

	if err := tenantRepo.ValidateAndCreate(dataSource); err != nil {
		if strings.Contains(err.Error(), "validation") {
			response.WriteValidationError(w, getRequestID(r), err.Error())
			return
		}
		response.WriteInternalServerError(w, getRequestID(r), "Failed to create data source", err.Error())
		return
	}

	responseData := h.toDataSourceResponse(dataSource)
	response.WriteCreated(w, getRequestID(r), responseData)
}

// GetDataSource handles GET /api/v1/tenants/{tenant_id}/data-sources/{id}
func (h *DataSourceHandler) GetDataSource(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, dataSourceID uuid.UUID) {
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var dataSource models.DataSource
	err = tenantRepo.DB().Where("id = ?", dataSourceID).First(&dataSource).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Data source not found")
		return
	}

	responseData := h.toDataSourceResponse(&dataSource)
	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// UpdateDataSource handles PUT /api/v1/tenants/{tenant_id}/data-sources/{id}
func (h *DataSourceHandler) UpdateDataSource(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, dataSourceID uuid.UUID) {
	var req DataSourceRequest
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

	var dataSource models.DataSource
	err = tenantRepo.DB().Where("id = ?", dataSourceID).First(&dataSource).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Data source not found")
		return
	}

	// Update fields
	dataSource.Name = req.Name
	dataSource.DisplayName = req.DisplayName
	dataSource.Type = req.Type
	dataSource.Config = req.Config
	dataSource.CredentialsRef = req.CredentialsRef
	dataSource.SyncSettings = req.SyncSettings
	dataSource.ProcessingSettings = req.ProcessingSettings

	if err := tenantRepo.ValidateAndUpdate(&dataSource); err != nil {
		if strings.Contains(err.Error(), "validation") {
			response.WriteValidationError(w, getRequestID(r), err.Error())
			return
		}
		response.WriteInternalServerError(w, getRequestID(r), "Failed to update data source", err.Error())
		return
	}

	responseData := h.toDataSourceResponse(&dataSource)
	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// DeleteDataSource handles DELETE /api/v1/tenants/{tenant_id}/data-sources/{id}
func (h *DataSourceHandler) DeleteDataSource(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, dataSourceID uuid.UUID) {
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	// Check if data source exists
	var dataSource models.DataSource
	err = tenantRepo.DB().Where("id = ?", dataSourceID).First(&dataSource).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Data source not found")
		return
	}

	// Delete data source
	if err := tenantRepo.DB().Delete(&dataSource).Error; err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to delete data source", err.Error())
		return
	}

	response.WriteNoContent(w)
}

// TriggerSync handles POST /api/v1/tenants/{tenant_id}/data-sources/{id}/sync
func (h *DataSourceHandler) TriggerSync(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, dataSourceID uuid.UUID) {
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

	var dataSource models.DataSource
	err = tenantRepo.DB().Where("id = ?", dataSourceID).First(&dataSource).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Data source not found")
		return
	}

	// TODO: Trigger actual sync operation
	// For now, we'll just update the sync status
	dataSource.LastSyncStatus = "running"
	if err := tenantRepo.DB().Save(&dataSource).Error; err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to trigger sync", err.Error())
		return
	}

	responseData := map[string]interface{}{
		"message": "Sync triggered successfully",
		"sync_status": "running",
	}

	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// TestConnection handles POST /api/v1/tenants/{tenant_id}/data-sources/{id}/test
func (h *DataSourceHandler) TestConnection(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, dataSourceID uuid.UUID) {
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

	var dataSource models.DataSource
	err = tenantRepo.DB().Where("id = ?", dataSourceID).First(&dataSource).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Data source not found")
		return
	}

	// TODO: Implement actual connection testing
	// For now, we'll return a mock response
	testResult := map[string]interface{}{
		"success": true,
		"message": "Connection test successful",
		"details": map[string]interface{}{
			"latency_ms": 150,
			"tested_at":  "2025-01-03T12:00:00Z",
		},
	}

	response.WriteSuccess(w, getRequestID(r), testResult, nil)
}

// ListDataSourceFiles handles GET /api/v1/tenants/{tenant_id}/data-sources/{id}/files
func (h *DataSourceHandler) ListDataSourceFiles(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, dataSourceID uuid.UUID) {
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

	// Check if data source exists
	var dataSource models.DataSource
	err = tenantRepo.DB().Where("id = ?", dataSourceID).First(&dataSource).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Data source not found")
		return
	}

	// Get files for this data source
	var files []models.File
	err = tenantRepo.DB().Where("data_source_id = ?", dataSourceID).
		Limit(pageSize).Offset(offset).Find(&files).Error
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list files", err.Error())
		return
	}

	// Get total count
	var totalCount int64
	tenantRepo.DB().Model(&models.File{}).Where("data_source_id = ?", dataSourceID).Count(&totalCount)

	response.WritePaginated(w, getRequestID(r), files, page, pageSize, totalCount)
}

// Helper methods

func (h *DataSourceHandler) toDataSourceResponse(ds *models.DataSource) DataSourceResponse {
	response := DataSourceResponse{
		ID:                 ds.ID,
		TenantID:           ds.TenantID,
		Name:               ds.Name,
		DisplayName:        ds.DisplayName,
		Type:               ds.Type,
		Config:             ds.Config,
		CredentialsRef:     ds.CredentialsRef,
		SyncSettings:       ds.SyncSettings,
		ProcessingSettings: ds.ProcessingSettings,
		Status:             ds.Status,
		LastSyncStatus:     ds.LastSyncStatus,
		LastSyncError:      ds.LastSyncError,
		CreatedAt:          ds.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:          ds.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if ds.LastSyncAt != nil {
		lastSync := ds.LastSyncAt.Format("2006-01-02T15:04:05Z")
		response.LastSyncAt = &lastSync
	}

	return response
}

func getTenantContext(ctx context.Context) *database.TenantContext {
	if tenantCtx, ok := ctx.Value("tenant_context").(*database.TenantContext); ok {
		return tenantCtx
	}
	return nil
}