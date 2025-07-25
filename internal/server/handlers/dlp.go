package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/internal/database/models"
	"github.com/jscharber/eAIIngest/internal/server/response"
)

// DLPHandler handles DLP policy and violation HTTP requests
type DLPHandler struct {
	db *database.Database
}

// NewDLPHandler creates a new DLP handler
func NewDLPHandler(db *database.Database) *DLPHandler {
	return &DLPHandler{db: db}
}

// DLPPolicyRequest represents a DLP policy creation/update request
type DLPPolicyRequest struct {
	Name         string                  `json:"name"`
	DisplayName  string                  `json:"display_name"`
	Description  string                  `json:"description"`
	Enabled      bool                    `json:"enabled"`
	Priority     int                     `json:"priority"`
	ContentRules []models.DLPContentRule `json:"content_rules"`
	Actions      []models.DLPAction      `json:"actions"`
	Conditions   models.DLPConditions    `json:"conditions"`
}

// DLPPolicyResponse represents a DLP policy response
type DLPPolicyResponse struct {
	ID             uuid.UUID               `json:"id"`
	TenantID       uuid.UUID               `json:"tenant_id"`
	Name           string                  `json:"name"`
	DisplayName    string                  `json:"display_name"`
	Description    string                  `json:"description"`
	Enabled        bool                    `json:"enabled"`
	Priority       int                     `json:"priority"`
	ContentRules   []models.DLPContentRule `json:"content_rules"`
	Actions        []models.DLPAction      `json:"actions"`
	Conditions     models.DLPConditions    `json:"conditions"`
	Status         string                  `json:"status"`
	TriggeredCount int64                   `json:"triggered_count"`
	LastTriggered  *string                 `json:"last_triggered,omitempty"`
	CreatedAt      string                  `json:"created_at"`
	UpdatedAt      string                  `json:"updated_at"`
}

// DLPViolationResponse represents a DLP violation response
type DLPViolationResponse struct {
	ID             uuid.UUID  `json:"id"`
	TenantID       uuid.UUID  `json:"tenant_id"`
	PolicyID       uuid.UUID  `json:"policy_id"`
	FileID         uuid.UUID  `json:"file_id"`
	ChunkID        *uuid.UUID `json:"chunk_id,omitempty"`
	RuleName       string     `json:"rule_name"`
	Severity       string     `json:"severity"`
	Confidence     float64    `json:"confidence"`
	MatchedText    string     `json:"matched_text,omitempty"`
	Context        string     `json:"context,omitempty"`
	StartOffset    int64      `json:"start_offset,omitempty"`
	EndOffset      int64      `json:"end_offset,omitempty"`
	LineNumber     int        `json:"line_number,omitempty"`
	ActionsTaken   []string   `json:"actions_taken"`
	Status         string     `json:"status"`
	Acknowledged   bool       `json:"acknowledged"`
	AcknowledgedBy string     `json:"acknowledged_by,omitempty"`
	AcknowledgedAt *string    `json:"acknowledged_at,omitempty"`
	CreatedAt      string     `json:"created_at"`
	UpdatedAt      string     `json:"updated_at"`
}

// ServeHTTP implements the http.Handler interface
func (h *DLPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tenantCtx := getTenantContext(r.Context())
	if tenantCtx == nil {
		response.WriteBadRequest(w, getRequestID(r), "Tenant context required", nil)
		return
	}

	path := r.URL.Path
	
	if strings.Contains(path, "/dlp-policies") {
		h.handlePolicyRoutes(w, r, tenantCtx.TenantID)
	} else if strings.Contains(path, "/violations") {
		h.handleViolationRoutes(w, r, tenantCtx.TenantID)
	} else {
		response.WriteNotFound(w, getRequestID(r), "DLP endpoint not found")
	}
}

// handlePolicyRoutes handles DLP policy routes
func (h *DLPHandler) handlePolicyRoutes(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	
	// Find dlp-policies in the path
	var policyIndex int = -1
	for i, part := range parts {
		if part == "dlp-policies" {
			policyIndex = i
			break
		}
	}

	if policyIndex == -1 {
		response.WriteNotFound(w, getRequestID(r), "DLP policy endpoint not found")
		return
	}

	if len(parts) == policyIndex+1 {
		// /api/v1/tenants/{tenant_id}/dlp-policies
		switch r.Method {
		case http.MethodGet:
			h.ListPolicies(w, r, tenantID)
		case http.MethodPost:
			h.CreatePolicy(w, r, tenantID)
		default:
			response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		}
		return
	}

	if len(parts) >= policyIndex+2 {
		policyID, err := uuid.Parse(parts[policyIndex+1])
		if err != nil {
			response.WriteBadRequest(w, getRequestID(r), "Invalid policy ID format", nil)
			return
		}

		if len(parts) == policyIndex+2 {
			// /api/v1/tenants/{tenant_id}/dlp-policies/{policy_id}
			switch r.Method {
			case http.MethodGet:
				h.GetPolicy(w, r, tenantID, policyID)
			case http.MethodPut:
				h.UpdatePolicy(w, r, tenantID, policyID)
			case http.MethodDelete:
				h.DeletePolicy(w, r, tenantID, policyID)
			default:
				response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
			}
			return
		}

		// Handle sub-resources
		if len(parts) >= policyIndex+3 {
			subResource := parts[policyIndex+2]
			switch subResource {
			case "test":
				h.TestPolicy(w, r, tenantID, policyID)
			case "violations":
				h.ListPolicyViolations(w, r, tenantID, policyID)
			default:
				response.WriteNotFound(w, getRequestID(r), "Resource not found")
			}
		}
	}
}

// handleViolationRoutes handles DLP violation routes
func (h *DLPHandler) handleViolationRoutes(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	
	// Find violations in the path
	var violationIndex int = -1
	for i, part := range parts {
		if part == "violations" {
			violationIndex = i
			break
		}
	}

	if violationIndex == -1 {
		response.WriteNotFound(w, getRequestID(r), "Violation endpoint not found")
		return
	}

	if len(parts) == violationIndex+1 {
		// /api/v1/tenants/{tenant_id}/violations
		switch r.Method {
		case http.MethodGet:
			h.ListViolations(w, r, tenantID)
		default:
			response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		}
		return
	}

	if len(parts) >= violationIndex+2 {
		violationID, err := uuid.Parse(parts[violationIndex+1])
		if err != nil {
			response.WriteBadRequest(w, getRequestID(r), "Invalid violation ID format", nil)
			return
		}

		if len(parts) == violationIndex+2 {
			// /api/v1/tenants/{tenant_id}/violations/{violation_id}
			switch r.Method {
			case http.MethodGet:
				h.GetViolation(w, r, tenantID, violationID)
			case http.MethodPut:
				h.UpdateViolation(w, r, tenantID, violationID)
			default:
				response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
			}
			return
		}

		// Handle sub-resources
		if len(parts) >= violationIndex+3 {
			subResource := parts[violationIndex+2]
			switch subResource {
			case "acknowledge":
				h.AcknowledgeViolation(w, r, tenantID, violationID)
			default:
				response.WriteNotFound(w, getRequestID(r), "Resource not found")
			}
		}
	}
}

// Policy handlers

// ListPolicies handles GET /api/v1/tenants/{tenant_id}/dlp-policies
func (h *DLPHandler) ListPolicies(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	page, pageSize, offset := getPagination(r.Context())
	
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var policies []models.DLPPolicy
	query := tenantRepo.DB().Limit(pageSize).Offset(offset).Order("priority DESC, created_at DESC")
	
	// Filter by enabled status if provided
	if enabledStr := r.URL.Query().Get("enabled"); enabledStr != "" {
		enabled := enabledStr == "true"
		query = query.Where("enabled = ?", enabled)
	}
	
	err = query.Find(&policies).Error
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list policies", err.Error())
		return
	}

	// Convert to response format
	responseData := make([]DLPPolicyResponse, len(policies))
	for i, policy := range policies {
		responseData[i] = h.toPolicyResponse(&policy)
	}

	// Get total count
	var totalCount int64
	countQuery := tenantRepo.DB().Model(&models.DLPPolicy{})
	if enabledStr := r.URL.Query().Get("enabled"); enabledStr != "" {
		enabled := enabledStr == "true"
		countQuery = countQuery.Where("enabled = ?", enabled)
	}
	countQuery.Count(&totalCount)

	response.WritePaginated(w, getRequestID(r), responseData, page, pageSize, totalCount)
}

// CreatePolicy handles POST /api/v1/tenants/{tenant_id}/dlp-policies
func (h *DLPHandler) CreatePolicy(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	var req DLPPolicyRequest
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

	// Create policy
	policy := &models.DLPPolicy{
		TenantID:     tenantID,
		Name:         req.Name,
		DisplayName:  req.DisplayName,
		Description:  req.Description,
		Enabled:      req.Enabled,
		Priority:     req.Priority,
		ContentRules: req.ContentRules,
		Actions:      req.Actions,
		Conditions:   req.Conditions,
		Status:       "active",
	}

	if err := tenantRepo.ValidateAndCreate(policy); err != nil {
		if strings.Contains(err.Error(), "validation") {
			response.WriteValidationError(w, getRequestID(r), err.Error())
			return
		}
		response.WriteInternalServerError(w, getRequestID(r), "Failed to create policy", err.Error())
		return
	}

	responseData := h.toPolicyResponse(policy)
	response.WriteCreated(w, getRequestID(r), responseData)
}

// GetPolicy handles GET /api/v1/tenants/{tenant_id}/dlp-policies/{id}
func (h *DLPHandler) GetPolicy(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, policyID uuid.UUID) {
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var policy models.DLPPolicy
	err = tenantRepo.DB().Where("id = ?", policyID).First(&policy).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Policy not found")
		return
	}

	responseData := h.toPolicyResponse(&policy)
	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// UpdatePolicy handles PUT /api/v1/tenants/{tenant_id}/dlp-policies/{id}
func (h *DLPHandler) UpdatePolicy(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, policyID uuid.UUID) {
	var req DLPPolicyRequest
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

	var policy models.DLPPolicy
	err = tenantRepo.DB().Where("id = ?", policyID).First(&policy).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Policy not found")
		return
	}

	// Update fields
	policy.Name = req.Name
	policy.DisplayName = req.DisplayName
	policy.Description = req.Description
	policy.Enabled = req.Enabled
	policy.Priority = req.Priority
	policy.ContentRules = req.ContentRules
	policy.Actions = req.Actions
	policy.Conditions = req.Conditions

	if err := tenantRepo.ValidateAndUpdate(&policy); err != nil {
		if strings.Contains(err.Error(), "validation") {
			response.WriteValidationError(w, getRequestID(r), err.Error())
			return
		}
		response.WriteInternalServerError(w, getRequestID(r), "Failed to update policy", err.Error())
		return
	}

	responseData := h.toPolicyResponse(&policy)
	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// DeletePolicy handles DELETE /api/v1/tenants/{tenant_id}/dlp-policies/{id}
func (h *DLPHandler) DeletePolicy(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, policyID uuid.UUID) {
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var policy models.DLPPolicy
	err = tenantRepo.DB().Where("id = ?", policyID).First(&policy).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Policy not found")
		return
	}

	if err := tenantRepo.DB().Delete(&policy).Error; err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to delete policy", err.Error())
		return
	}

	response.WriteNoContent(w)
}

// TestPolicy handles POST /api/v1/tenants/{tenant_id}/dlp-policies/{id}/test
func (h *DLPHandler) TestPolicy(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, policyID uuid.UUID) {
	if r.Method != http.MethodPost {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	var testRequest struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&testRequest); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
		return
	}

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var policy models.DLPPolicy
	err = tenantRepo.DB().Where("id = ?", policyID).First(&policy).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Policy not found")
		return
	}

	// TODO: Implement actual policy testing
	testResult := map[string]interface{}{
		"policy_id":  policyID,
		"content":    testRequest.Content,
		"violations": []map[string]interface{}{
			// Mock violation for demonstration
		},
		"matches_found": 0,
		"tested_at":     "2025-01-03T12:00:00Z",
	}

	response.WriteSuccess(w, getRequestID(r), testResult, nil)
}

// Violation handlers

// ListViolations handles GET /api/v1/tenants/{tenant_id}/violations
func (h *DLPHandler) ListViolations(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	page, pageSize, offset := getPagination(r.Context())
	
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var violations []models.DLPViolation
	query := tenantRepo.DB().Limit(pageSize).Offset(offset).Order("created_at DESC")
	
	// Filter by severity if provided
	if severity := r.URL.Query().Get("severity"); severity != "" {
		query = query.Where("severity = ?", severity)
	}
	
	// Filter by status if provided
	if status := r.URL.Query().Get("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	
	err = query.Find(&violations).Error
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list violations", err.Error())
		return
	}

	// Convert to response format
	responseData := make([]DLPViolationResponse, len(violations))
	for i, violation := range violations {
		responseData[i] = h.toViolationResponse(&violation)
	}

	// Get total count
	var totalCount int64
	countQuery := tenantRepo.DB().Model(&models.DLPViolation{})
	if severity := r.URL.Query().Get("severity"); severity != "" {
		countQuery = countQuery.Where("severity = ?", severity)
	}
	if status := r.URL.Query().Get("status"); status != "" {
		countQuery = countQuery.Where("status = ?", status)
	}
	countQuery.Count(&totalCount)

	response.WritePaginated(w, getRequestID(r), responseData, page, pageSize, totalCount)
}

// GetViolation handles GET /api/v1/tenants/{tenant_id}/violations/{id}
func (h *DLPHandler) GetViolation(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, violationID uuid.UUID) {
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var violation models.DLPViolation
	err = tenantRepo.DB().Where("id = ?", violationID).First(&violation).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Violation not found")
		return
	}

	responseData := h.toViolationResponse(&violation)
	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// ListPolicyViolations handles GET /api/v1/tenants/{tenant_id}/dlp-policies/{id}/violations
func (h *DLPHandler) ListPolicyViolations(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, policyID uuid.UUID) {
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

	var violations []models.DLPViolation
	err = tenantRepo.DB().Where("policy_id = ?", policyID).
		Limit(pageSize).Offset(offset).Order("created_at DESC").Find(&violations).Error
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list violations", err.Error())
		return
	}

	// Convert to response format
	responseData := make([]DLPViolationResponse, len(violations))
	for i, violation := range violations {
		responseData[i] = h.toViolationResponse(&violation)
	}

	// Get total count
	var totalCount int64
	tenantRepo.DB().Model(&models.DLPViolation{}).Where("policy_id = ?", policyID).Count(&totalCount)

	response.WritePaginated(w, getRequestID(r), responseData, page, pageSize, totalCount)
}

// UpdateViolation handles PUT /api/v1/tenants/{tenant_id}/violations/{id}
func (h *DLPHandler) UpdateViolation(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, violationID uuid.UUID) {
	var updateRequest struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&updateRequest); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
		return
	}

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var violation models.DLPViolation
	err = tenantRepo.DB().Where("id = ?", violationID).First(&violation).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Violation not found")
		return
	}

	// Update status
	violation.Status = updateRequest.Status

	if err := tenantRepo.DB().Save(&violation).Error; err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to update violation", err.Error())
		return
	}

	responseData := h.toViolationResponse(&violation)
	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// AcknowledgeViolation handles POST /api/v1/tenants/{tenant_id}/violations/{id}/acknowledge
func (h *DLPHandler) AcknowledgeViolation(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, violationID uuid.UUID) {
	if r.Method != http.MethodPost {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	var ackRequest struct {
		AcknowledgedBy string `json:"acknowledged_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&ackRequest); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
		return
	}

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var violation models.DLPViolation
	err = tenantRepo.DB().Where("id = ?", violationID).First(&violation).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Violation not found")
		return
	}

	// Acknowledge violation
	violation.Acknowledged = true
	violation.AcknowledgedBy = ackRequest.AcknowledgedBy
	now := time.Now()
	violation.AcknowledgedAt = &now

	if err := tenantRepo.DB().Save(&violation).Error; err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to acknowledge violation", err.Error())
		return
	}

	responseData := h.toViolationResponse(&violation)
	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// Helper methods

func (h *DLPHandler) toPolicyResponse(policy *models.DLPPolicy) DLPPolicyResponse {
	responseData := DLPPolicyResponse{
		ID:             policy.ID,
		TenantID:       policy.TenantID,
		Name:           policy.Name,
		DisplayName:    policy.DisplayName,
		Description:    policy.Description,
		Enabled:        policy.Enabled,
		Priority:       policy.Priority,
		ContentRules:   policy.ContentRules,
		Actions:        policy.Actions,
		Conditions:     policy.Conditions,
		Status:         policy.Status,
		TriggeredCount: policy.TriggeredCount,
		CreatedAt:      policy.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      policy.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if policy.LastTriggered != nil {
		lastTriggered := policy.LastTriggered.Format("2006-01-02T15:04:05Z")
		responseData.LastTriggered = &lastTriggered
	}

	return responseData
}

func (h *DLPHandler) toViolationResponse(violation *models.DLPViolation) DLPViolationResponse {
	responseData := DLPViolationResponse{
		ID:             violation.ID,
		TenantID:       violation.TenantID,
		PolicyID:       violation.PolicyID,
		FileID:         violation.FileID,
		ChunkID:        violation.ChunkID,
		RuleName:       violation.RuleName,
		Severity:       violation.Severity,
		Confidence:     violation.Confidence,
		MatchedText:    violation.MatchedText,
		Context:        violation.Context,
		StartOffset:    violation.StartOffset,
		EndOffset:      violation.EndOffset,
		LineNumber:     violation.LineNumber,
		ActionsTaken:   violation.ActionsTaken,
		Status:         violation.Status,
		Acknowledged:   violation.Acknowledged,
		AcknowledgedBy: violation.AcknowledgedBy,
		CreatedAt:      violation.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      violation.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if violation.AcknowledgedAt != nil {
		acknowledgedAt := violation.AcknowledgedAt.Format("2006-01-02T15:04:05Z")
		responseData.AcknowledgedAt = &acknowledgedAt
	}

	return responseData
}