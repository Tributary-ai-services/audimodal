package response

import (
	"encoding/json"
	"net/http"
	"time"
)

// APIResponse represents a standard API response
type APIResponse struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Error     *APIError   `json:"error,omitempty"`
	Meta      *Meta       `json:"meta,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	RequestID string      `json:"request_id,omitempty"`
}

// APIError represents an API error
type APIError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// Meta represents response metadata
type Meta struct {
	Pagination *Pagination `json:"pagination,omitempty"`
	Count      *int64      `json:"count,omitempty"`
	Version    string      `json:"version,omitempty"`
}

// Pagination represents pagination metadata
type Pagination struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalPages int   `json:"total_pages"`
	TotalCount int64 `json:"total_count"`
	HasNext    bool  `json:"has_next"`
	HasPrev    bool  `json:"has_prev"`
}

// ResponseWriter provides utility methods for writing API responses
type ResponseWriter struct {
	w         http.ResponseWriter
	requestID string
}

// NewResponseWriter creates a new response writer
func NewResponseWriter(w http.ResponseWriter, requestID string) *ResponseWriter {
	return &ResponseWriter{
		w:         w,
		requestID: requestID,
	}
}

// Success writes a successful response
func (rw *ResponseWriter) Success(data interface{}, meta *Meta) {
	response := APIResponse{
		Success:   true,
		Data:      data,
		Meta:      meta,
		Timestamp: time.Now(),
		RequestID: rw.requestID,
	}
	
	rw.writeJSON(http.StatusOK, response)
}

// Created writes a created response (201)
func (rw *ResponseWriter) Created(data interface{}) {
	response := APIResponse{
		Success:   true,
		Data:      data,
		Timestamp: time.Now(),
		RequestID: rw.requestID,
	}
	
	rw.writeJSON(http.StatusCreated, response)
}

// NoContent writes a no content response (204)
func (rw *ResponseWriter) NoContent() {
	rw.w.WriteHeader(http.StatusNoContent)
}

// Error writes an error response
func (rw *ResponseWriter) Error(statusCode int, code, message string, details interface{}) {
	response := APIResponse{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
		Timestamp: time.Now(),
		RequestID: rw.requestID,
	}
	
	rw.writeJSON(statusCode, response)
}

// BadRequest writes a bad request error (400)
func (rw *ResponseWriter) BadRequest(message string, details interface{}) {
	rw.Error(http.StatusBadRequest, "BAD_REQUEST", message, details)
}

// Unauthorized writes an unauthorized error (401)
func (rw *ResponseWriter) Unauthorized(message string) {
	rw.Error(http.StatusUnauthorized, "UNAUTHORIZED", message, nil)
}

// Forbidden writes a forbidden error (403)
func (rw *ResponseWriter) Forbidden(message string) {
	rw.Error(http.StatusForbidden, "FORBIDDEN", message, nil)
}

// NotFound writes a not found error (404)
func (rw *ResponseWriter) NotFound(message string) {
	rw.Error(http.StatusNotFound, "NOT_FOUND", message, nil)
}

// Conflict writes a conflict error (409)
func (rw *ResponseWriter) Conflict(message string, details interface{}) {
	rw.Error(http.StatusConflict, "CONFLICT", message, details)
}

// UnprocessableEntity writes an unprocessable entity error (422)
func (rw *ResponseWriter) UnprocessableEntity(message string, details interface{}) {
	rw.Error(http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", message, details)
}

// TooManyRequests writes a too many requests error (429)
func (rw *ResponseWriter) TooManyRequests(message string) {
	rw.Error(http.StatusTooManyRequests, "TOO_MANY_REQUESTS", message, nil)
}

// InternalServerError writes an internal server error (500)
func (rw *ResponseWriter) InternalServerError(message string, details interface{}) {
	rw.Error(http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", message, details)
}

// ServiceUnavailable writes a service unavailable error (503)
func (rw *ResponseWriter) ServiceUnavailable(message string) {
	rw.Error(http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", message, nil)
}

// ValidationError writes a validation error response
func (rw *ResponseWriter) ValidationError(errors interface{}) {
	rw.Error(http.StatusBadRequest, "VALIDATION_ERROR", "Validation failed", errors)
}

// Paginated writes a paginated response
func (rw *ResponseWriter) Paginated(data interface{}, page, pageSize int, totalCount int64) {
	totalPages := int((totalCount + int64(pageSize) - 1) / int64(pageSize))
	
	pagination := &Pagination{
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
		TotalCount: totalCount,
		HasNext:    page < totalPages,
		HasPrev:    page > 1,
	}
	
	meta := &Meta{
		Pagination: pagination,
		Count:      &totalCount,
	}
	
	rw.Success(data, meta)
}

// writeJSON writes a JSON response
func (rw *ResponseWriter) writeJSON(statusCode int, data interface{}) {
	rw.w.Header().Set("Content-Type", "application/json")
	rw.w.WriteHeader(statusCode)
	
	if err := json.NewEncoder(rw.w).Encode(data); err != nil {
		// If encoding fails, write a basic error response
		http.Error(rw.w, "Internal server error", http.StatusInternalServerError)
	}
}

// Helper functions for common responses

// WriteSuccess writes a success response with optional metadata
func WriteSuccess(w http.ResponseWriter, requestID string, data interface{}, meta *Meta) {
	rw := NewResponseWriter(w, requestID)
	rw.Success(data, meta)
}

// WriteError writes an error response
func WriteError(w http.ResponseWriter, requestID string, statusCode int, code, message string, details interface{}) {
	rw := NewResponseWriter(w, requestID)
	rw.Error(statusCode, code, message, details)
}

// WriteBadRequest writes a bad request error
func WriteBadRequest(w http.ResponseWriter, requestID string, message string, details interface{}) {
	rw := NewResponseWriter(w, requestID)
	rw.BadRequest(message, details)
}

// WriteNotFound writes a not found error
func WriteNotFound(w http.ResponseWriter, requestID string, message string) {
	rw := NewResponseWriter(w, requestID)
	rw.NotFound(message)
}

// WriteInternalServerError writes an internal server error
func WriteInternalServerError(w http.ResponseWriter, requestID string, message string, details interface{}) {
	rw := NewResponseWriter(w, requestID)
	rw.InternalServerError(message, details)
}

// WriteValidationError writes a validation error
func WriteValidationError(w http.ResponseWriter, requestID string, errors interface{}) {
	rw := NewResponseWriter(w, requestID)
	rw.ValidationError(errors)
}

// WritePaginated writes a paginated response
func WritePaginated(w http.ResponseWriter, requestID string, data interface{}, page, pageSize int, totalCount int64) {
	rw := NewResponseWriter(w, requestID)
	rw.Paginated(data, page, pageSize, totalCount)
}

// WriteCreated writes a created response
func WriteCreated(w http.ResponseWriter, requestID string, data interface{}) {
	rw := NewResponseWriter(w, requestID)
	rw.Created(data)
}

// WriteNoContent writes a no content response
func WriteNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// HealthResponse represents a health check response
type HealthResponse struct {
	Status    string            `json:"status"`
	Version   string            `json:"version"`
	Timestamp time.Time         `json:"timestamp"`
	Checks    map[string]string `json:"checks"`
}

// WriteHealthCheck writes a health check response
func WriteHealthCheck(w http.ResponseWriter, status string, version string, checks map[string]string) {
	response := HealthResponse{
		Status:    status,
		Version:   version,
		Timestamp: time.Now(),
		Checks:    checks,
	}
	
	statusCode := http.StatusOK
	if status != "healthy" {
		statusCode = http.StatusServiceUnavailable
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// ErrorCode constants
const (
	ErrorCodeBadRequest          = "BAD_REQUEST"
	ErrorCodeUnauthorized        = "UNAUTHORIZED"
	ErrorCodeForbidden          = "FORBIDDEN"
	ErrorCodeNotFound           = "NOT_FOUND"
	ErrorCodeConflict           = "CONFLICT"
	ErrorCodeValidationError    = "VALIDATION_ERROR"
	ErrorCodeInternalError      = "INTERNAL_SERVER_ERROR"
	ErrorCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	ErrorCodeTooManyRequests    = "TOO_MANY_REQUESTS"
	ErrorCodeUnprocessableEntity = "UNPROCESSABLE_ENTITY"
	
	// Business logic error codes
	ErrorCodeTenantNotFound     = "TENANT_NOT_FOUND"
	ErrorCodeTenantInactive     = "TENANT_INACTIVE"
	ErrorCodeQuotaExceeded      = "QUOTA_EXCEEDED"
	ErrorCodeDataSourceNotFound = "DATA_SOURCE_NOT_FOUND"
	ErrorCodeSessionNotFound    = "SESSION_NOT_FOUND"
	ErrorCodeFileNotFound      = "FILE_NOT_FOUND"
	ErrorCodeChunkNotFound     = "CHUNK_NOT_FOUND"
	ErrorCodePolicyNotFound    = "POLICY_NOT_FOUND"
	ErrorCodeViolationNotFound = "VIOLATION_NOT_FOUND"
	ErrorCodeInvalidFileFormat = "INVALID_FILE_FORMAT"
	ErrorCodeProcessingFailed  = "PROCESSING_FAILED"
	ErrorCodeDuplicateResource = "DUPLICATE_RESOURCE"
)