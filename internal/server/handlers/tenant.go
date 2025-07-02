package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/internal/database/models"
	"github.com/jscharber/eAIIngest/internal/server/response"
)

// TenantHandler handles tenant-related HTTP requests
type TenantHandler struct {
	db *database.Database
}

// NewTenantHandler creates a new tenant handler
func NewTenantHandler(db *database.Database) *TenantHandler {
	return &TenantHandler{db: db}
}

// TenantRequest represents a tenant creation/update request
type TenantRequest struct {
	Name        string                     `json:"name"`
	DisplayName string                     `json:"display_name"`
	BillingPlan string                     `json:"billing_plan"`
	BillingEmail string                    `json:"billing_email"`
	Quotas      models.TenantQuotas        `json:"quotas"`
	Compliance  models.TenantCompliance    `json:"compliance"`
	ContactInfo models.TenantContactInfo   `json:"contact_info"`
}

// TenantResponse represents a tenant response
type TenantResponse struct {
	ID          uuid.UUID                  `json:"id"`
	Name        string                     `json:"name"`
	DisplayName string                     `json:"display_name"`
	BillingPlan string                     `json:"billing_plan"`
	BillingEmail string                    `json:"billing_email"`
	Quotas      models.TenantQuotas        `json:"quotas"`
	Compliance  models.TenantCompliance    `json:"compliance"`
	ContactInfo models.TenantContactInfo   `json:"contact_info"`
	Status      string                     `json:"status"`
	CreatedAt   string                     `json:"created_at"`
	UpdatedAt   string                     `json:"updated_at"`
}

// ListTenants handles GET /api/v1/tenants
func (h *TenantHandler) ListTenants(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	page, pageSize, offset := getPagination(r.Context())
	
	tenantService := h.db.NewTenantService()
	tenants, err := tenantService.ListTenants(r.Context(), pageSize, offset)
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list tenants", err.Error())
		return
	}

	// Convert to response format
	response := make([]TenantResponse, len(tenants))
	for i, tenant := range tenants {
		response[i] = h.toTenantResponse(&tenant)
	}

	// Get total count for pagination
	totalCount := int64(len(tenants)) // This is a simplified approach
	response.WritePaginated(w, getRequestID(r), response, page, pageSize, totalCount)
}

// HandleTenant handles tenant-specific routes
func (h *TenantHandler) HandleTenant(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/tenants")
	
	if path == "" || path == "/" {
		// Handle /api/v1/tenants
		switch r.Method {
		case http.MethodPost:
			h.CreateTenant(w, r)
		default:
			response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		}
		return
	}

	// Extract tenant ID from path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		response.WriteBadRequest(w, getRequestID(r), "Invalid tenant path", nil)
		return
	}

	tenantID, err := uuid.Parse(parts[0])
	if err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid tenant ID format", nil)
		return
	}

	if len(parts) == 1 {
		// Handle /api/v1/tenants/{id}
		switch r.Method {
		case http.MethodGet:
			h.GetTenant(w, r, tenantID)
		case http.MethodPut:
			h.UpdateTenant(w, r, tenantID)
		case http.MethodDelete:
			h.DeleteTenant(w, r, tenantID)
		default:
			response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		}
		return
	}

	// Handle sub-resources
	subResource := parts[1]
	switch subResource {
	case "stats":
		h.GetTenantStats(w, r, tenantID)
	case "quotas":
		h.GetTenantQuotas(w, r, tenantID)
	case "export":
		h.ExportTenantData(w, r, tenantID)
	default:
		response.WriteNotFound(w, getRequestID(r), "Resource not found")
	}
}

// CreateTenant handles POST /api/v1/tenants
func (h *TenantHandler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	var req TenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
		return
	}

	// Convert request to model
	tenant := &models.Tenant{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		BillingPlan: req.BillingPlan,
		BillingEmail: req.BillingEmail,
		Quotas:      req.Quotas,
		Compliance:  req.Compliance,
		ContactInfo: req.ContactInfo,
		Status:      models.TenantStatusActive,
	}

	// Create tenant
	tenantService := h.db.NewTenantService()
	if err := tenantService.CreateTenant(r.Context(), tenant); err != nil {
		if strings.Contains(err.Error(), "validation failed") {
			response.WriteValidationError(w, getRequestID(r), err.Error())
			return
		}
		response.WriteInternalServerError(w, getRequestID(r), "Failed to create tenant", err.Error())
		return
	}

	response := h.toTenantResponse(tenant)
	response.WriteCreated(w, getRequestID(r), response)
}

// GetTenant handles GET /api/v1/tenants/{id}
func (h *TenantHandler) GetTenant(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	tenantService := h.db.NewTenantService()
	tenant, err := tenantService.GetTenant(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	response := h.toTenantResponse(tenant)
	response.WriteSuccess(w, getRequestID(r), response, nil)
}

// UpdateTenant handles PUT /api/v1/tenants/{id}
func (h *TenantHandler) UpdateTenant(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	var req TenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
		return
	}

	tenantService := h.db.NewTenantService()
	
	// Get existing tenant
	tenant, err := tenantService.GetTenant(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	// Update fields
	tenant.Name = req.Name
	tenant.DisplayName = req.DisplayName
	tenant.BillingPlan = req.BillingPlan
	tenant.BillingEmail = req.BillingEmail
	tenant.Quotas = req.Quotas
	tenant.Compliance = req.Compliance
	tenant.ContactInfo = req.ContactInfo

	// Save changes
	if err := tenantService.UpdateTenant(r.Context(), tenant); err != nil {
		if strings.Contains(err.Error(), "validation failed") {
			response.WriteValidationError(w, getRequestID(r), err.Error())
			return
		}
		response.WriteInternalServerError(w, getRequestID(r), "Failed to update tenant", err.Error())
		return
	}

	response := h.toTenantResponse(tenant)
	response.WriteSuccess(w, getRequestID(r), response, nil)
}

// DeleteTenant handles DELETE /api/v1/tenants/{id}
func (h *TenantHandler) DeleteTenant(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	tenantService := h.db.NewTenantService()
	
	// Check if tenant exists
	_, err := tenantService.GetTenant(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	// Delete tenant
	if err := tenantService.DeleteTenant(r.Context(), tenantID); err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to delete tenant", err.Error())
		return
	}

	response.WriteNoContent(w)
}

// GetTenantStats handles GET /api/v1/tenants/{id}/stats
func (h *TenantHandler) GetTenantStats(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
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

	stats, err := tenantRepo.GetTenantStats(r.Context())
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to get tenant stats", err.Error())
		return
	}

	response.WriteSuccess(w, getRequestID(r), stats, nil)
}

// GetTenantQuotas handles GET /api/v1/tenants/{id}/quotas
func (h *TenantHandler) GetTenantQuotas(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	tenantService := h.db.NewTenantService()
	tenant, err := tenantService.GetTenant(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	quotaInfo := map[string]interface{}{
		"quotas": tenant.Quotas,
		"usage": map[string]interface{}{
			// This would be populated with actual usage data
			"files_processed_this_hour": 0,
			"storage_used_gb":          0,
			"compute_hours_used":       0,
			"api_requests_this_minute": 0,
			"concurrent_jobs":          0,
		},
		"compliance": tenant.Compliance,
	}

	response.WriteSuccess(w, getRequestID(r), quotaInfo, nil)
}

// ExportTenantData handles GET /api/v1/tenants/{id}/export
func (h *TenantHandler) ExportTenantData(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
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

	exportData, err := tenantRepo.ExportTenantData(r.Context())
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to export tenant data", err.Error())
		return
	}

	// Set headers for file download
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=tenant_%s_export.json", tenantID))

	response.WriteSuccess(w, getRequestID(r), exportData, nil)
}

// Helper methods

func (h *TenantHandler) toTenantResponse(tenant *models.Tenant) TenantResponse {
	return TenantResponse{
		ID:          tenant.ID,
		Name:        tenant.Name,
		DisplayName: tenant.DisplayName,
		BillingPlan: tenant.BillingPlan,
		BillingEmail: tenant.BillingEmail,
		Quotas:      tenant.Quotas,
		Compliance:  tenant.Compliance,
		ContactInfo: tenant.ContactInfo,
		Status:      tenant.Status,
		CreatedAt:   tenant.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   tenant.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func getRequestID(r *http.Request) string {
	if requestID, ok := r.Context().Value("request_id").(string); ok {
		return requestID
	}
	return "unknown"
}

func getPagination(ctx context.Context) (page, pageSize, offset int) {
	if pagination, ok := ctx.Value("pagination").(map[string]int); ok {
		return pagination["page"], pagination["page_size"], pagination["offset"]
	}
	return 1, 20, 0
}

import "context"