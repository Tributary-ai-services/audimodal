package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jscharber/audimodal/internal/database"
)

// TestFileHandler_TenantContextValidation tests the first level of validation - tenant context
// This can be tested without database access since it returns early on missing tenant context
func TestFileHandler_TenantContextValidation(t *testing.T) {
	handler := &FileHandler{
		db:                   nil, // Will not be accessed due to early return
		embeddingCoordinator: nil,
	}

	tenantID := uuid.New()
	url := "/api/v1/tenants/" + tenantID.String() + "/files"

	t.Run("missing_tenant_context", func(t *testing.T) {
		// Create request without tenant context
		req, err := http.NewRequest("GET", url, nil)
		require.NoError(t, err)

		// Add only request ID, not tenant context
		ctx := context.WithValue(req.Context(), "request_id", uuid.New().String())
		req = req.WithContext(ctx)

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)

		// Should return 400 for missing tenant context
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		
		// Should contain error about tenant context
		body := recorder.Body.String()
		assert.Contains(t, body, "Tenant context required")
	})
}

// TestFileHandler_RouteNotFound tests routes that should return 404 immediately
func TestFileHandler_RouteNotFound(t *testing.T) {
	handler := &FileHandler{
		db:                   nil,
		embeddingCoordinator: nil,
	}

	tenantID := uuid.New()

	tests := []struct {
		name        string
		path        string
		description string
	}{
		{
			name:        "no_files_in_path",
			path:        "/api/v1/tenants/" + tenantID.String() + "/documents",
			description: "Should return 404 when 'files' is not in path",
		},
		{
			name:        "root_tenant_path",
			path:        "/api/v1/tenants/" + tenantID.String(),
			description: "Should return 404 for tenant root without 'files'",
		},
		{
			name:        "invalid_path_structure",
			path:        "/api/v1/something/else",
			description: "Should return 404 for completely different path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createTestRequestWithTenantContext("GET", tt.path, nil, tenantID)
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, req)

			// Should return 404 for routes that don't contain "files"
			assert.Equal(t, http.StatusNotFound, recorder.Code, tt.description)
			
			// Should contain "File endpoint not found" message
			body := recorder.Body.String()
			assert.Contains(t, body, "File endpoint not found")
		})
	}
}

// TestFileHandler_InvalidFileUUID tests handling of invalid file UUIDs
// This should fail at UUID parsing before hitting the database
func TestFileHandler_InvalidFileUUID(t *testing.T) {
	handler := &FileHandler{
		db:                   nil, // Will not be accessed due to UUID parsing error
		embeddingCoordinator: nil,
	}

	tenantID := uuid.New()
	invalidUUIDs := []string{
		"invalid-uuid",
		"not-a-uuid-at-all", 
		"123-456-789",
		"too-short",
	}

	for _, invalidUUID := range invalidUUIDs {
		t.Run("invalid_uuid_"+invalidUUID, func(t *testing.T) {
			url := "/api/v1/tenants/" + tenantID.String() + "/files/" + invalidUUID
			req := createTestRequestWithTenantContext("GET", url, nil, tenantID)
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, req)

			// Should return 400 for invalid UUID format
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			
			// Should contain error about invalid file ID format
			body := recorder.Body.String()
			assert.Contains(t, body, "Invalid file ID format")
		})
	}

	// Note: Empty file ID case ("/files/") routes to ListFiles which requires database access
}

// TestFileHandler_SearchRouteRecognition tests that search routes are properly recognized
// Search endpoints have special handling before database access
func TestFileHandler_SearchRouteRecognition(t *testing.T) {
	handler := &FileHandler{
		db:                   nil,
		embeddingCoordinator: nil, // No embeddings, which is a valid test case
	}

	tenantID := uuid.New()

	tests := []struct {
		name        string
		method      string
		url         string
		description string
	}{
		{
			name:        "search_without_query",
			method:      "GET",
			url:         "/api/v1/tenants/" + tenantID.String() + "/files/search",
			description: "Should recognize search route without query",
		},
		{
			name:        "search_with_empty_query",
			method:      "GET",
			url:         "/api/v1/tenants/" + tenantID.String() + "/files/search?q=",
			description: "Should recognize search route with empty query",
		},
		{
			name:        "search_with_query",
			method:      "GET",
			url:         "/api/v1/tenants/" + tenantID.String() + "/files/search?q=test",
			description: "Should recognize search route with query",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createTestRequestWithTenantContext(tt.method, tt.url, nil, tenantID)
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, req)

			// Should NOT return "File endpoint not found" - route should be recognized
			body := recorder.Body.String()
			assert.False(t, strings.Contains(body, "File endpoint not found"),
				"Search route should be recognized: %s", tt.description)
			assert.False(t, strings.Contains(body, "Endpoint not found"),
				"Search route should be recognized: %s", tt.description)

			// Might return various other errors (400 for missing query, 500 for no embeddings)
			// but should not be a routing error
			assert.NotEqual(t, http.StatusNotFound, recorder.Code,
				"Search route should not return 404: %s", tt.description)
		})
	}
}

// TestFileHandler_MethodNotAllowed tests method validation for routes that support it
func TestFileHandler_MethodNotAllowed(t *testing.T) {
	handler := &FileHandler{
		db:                   nil,
		embeddingCoordinator: nil,
	}

	tenantID := uuid.New()
	
	// Test only methods that should return method not allowed before hitting database
	tests := []struct {
		name           string
		method         string
		url            string
		description    string
	}{
		{
			name:        "files_list_patch",
			method:      "PATCH",
			url:         "/api/v1/tenants/" + tenantID.String() + "/files",
			description: "PATCH should not be valid for files list",
		},
		{
			name:        "files_list_delete",
			method:      "DELETE",
			url:         "/api/v1/tenants/" + tenantID.String() + "/files",
			description: "DELETE should not be valid for files list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createTestRequestWithTenantContext(tt.method, tt.url, nil, tenantID)
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, req)

			// Should return method not allowed
			assert.Equal(t, http.StatusMethodNotAllowed, recorder.Code, tt.description)
			
			body := recorder.Body.String()
			assert.Contains(t, body, "Method not allowed")
		})
	}
	
	// Note: GET and POST on /files route to ListFiles/CreateFile which require database access
}

// Helper function to create test requests with tenant context
func createTestRequestWithTenantContext(method, url string, body []byte, tenantID uuid.UUID) *http.Request {
	var req *http.Request
	var err error

	if body != nil {
		req, err = http.NewRequest(method, url, strings.NewReader(string(body)))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		panic(err)
	}

	// Add tenant context
	ctx := context.WithValue(req.Context(), "tenant_context", &database.TenantContext{
		TenantID: tenantID,
	})
	req = req.WithContext(ctx)

	// Add request ID
	ctx = context.WithValue(req.Context(), "request_id", uuid.New().String())
	req = req.WithContext(ctx)

	return req
}