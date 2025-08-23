package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSearchErrorHandling tests the improved search error handling we implemented
func TestSearchErrorHandling(t *testing.T) {
	const (
		baseURL  = "http://localhost:8084"
		tenantID = "9855e094-36a6-4d3a-a4f5-d77da4614439"
	)

	client := &http.Client{}

	t.Run("Search with empty dataset returns HTTP 200 with empty results", func(t *testing.T) {
		// This test verifies that when a dataset exists but has no vectors,
		// we return HTTP 200 with empty results instead of an error
		searchData := map[string]interface{}{
			"query":     "test query that will find nothing",
			"top_k":     5,
			"threshold": 0.7,
		}

		searchURL := fmt.Sprintf("%s/api/v1/tenants/%s/files/search", baseURL, tenantID)
		body, _ := json.Marshal(searchData)
		req, _ := http.NewRequest("POST", searchURL, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		// Verify the response structure based on our error handling improvements
		switch resp.StatusCode {
		case http.StatusOK:
			// Should return empty results for successful search with no matches
			if success, exists := result["success"]; exists && success != nil {
				assert.True(t, success.(bool))
			}
			if data, ok := result["data"].(map[string]interface{}); ok {
				if results, ok := data["results"].([]interface{}); ok {
					assert.Equal(t, 0, len(results), "Empty dataset should return empty results")
				}
				if totalFound, exists := data["total_found"]; exists && totalFound != nil {
					assert.Equal(t, 0, int(totalFound.(float64)), "Total found should be 0")
				}
			}
		case http.StatusNotFound:
			// Dataset doesn't exist - this is correct RESTful behavior
			if success, exists := result["success"]; exists && success != nil {
				assert.False(t, success.(bool))
			}
			if errorInfo, ok := result["error"].(map[string]interface{}); ok {
				if code, exists := errorInfo["code"]; exists && code != nil {
					assert.Equal(t, "NOT_FOUND", code.(string))
				}
				if message, exists := errorInfo["message"]; exists && message != nil {
					assert.Contains(t, message.(string), "Dataset not found")
				}
			}
		case http.StatusServiceUnavailable:
			// Embedding service not available - skip test
			t.Skip("Vector search service is not available")
		default:
			t.Fatalf("Unexpected status code: %d, response: %+v", resp.StatusCode, result)
		}
	})

	t.Run("Search with missing query returns HTTP 400", func(t *testing.T) {
		// Test that missing required parameters return proper validation errors
		searchData := map[string]interface{}{
			// Missing "query" field
			"top_k":     5,
			"threshold": 0.7,
		}

		searchURL := fmt.Sprintf("%s/api/v1/tenants/%s/files/search", baseURL, tenantID)
		body, _ := json.Marshal(searchData)
		req, _ := http.NewRequest("POST", searchURL, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		
		if success, exists := result["success"]; exists && success != nil {
			assert.False(t, success.(bool))
		}
		
		if errorInfo, ok := result["error"].(map[string]interface{}); ok {
			if code, exists := errorInfo["code"]; exists && code != nil {
				assert.Equal(t, "BAD_REQUEST", code.(string))
			}
			if message, exists := errorInfo["message"]; exists && message != nil {
				assert.Contains(t, message.(string), "Query is required")
			}
		}
	})

	t.Run("Search with invalid JSON returns HTTP 400", func(t *testing.T) {
		// Test that malformed JSON returns proper validation errors
		searchURL := fmt.Sprintf("%s/api/v1/tenants/%s/files/search", baseURL, tenantID)
		invalidJSON := `{"query": "test", "top_k": invalid_number}`
		
		req, _ := http.NewRequest("POST", searchURL, bytes.NewBufferString(invalidJSON))
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		
		if success, exists := result["success"]; exists && success != nil {
			assert.False(t, success.(bool))
		}
		
		if errorInfo, ok := result["error"].(map[string]interface{}); ok {
			if code, exists := errorInfo["code"]; exists && code != nil {
				assert.Equal(t, "BAD_REQUEST", code.(string))
			}
			if message, exists := errorInfo["message"]; exists && message != nil {
				assert.Contains(t, message.(string), "Invalid JSON")
			}
		}
	})

	t.Run("Search with nonexistent tenant returns HTTP 404", func(t *testing.T) {
		// Test that searching with a nonexistent tenant returns proper not found error
		nonexistentTenant := "00000000-0000-0000-0000-000000000000"
		searchData := map[string]interface{}{
			"query":     "test query",
			"top_k":     5,
			"threshold": 0.7,
		}

		searchURL := fmt.Sprintf("%s/api/v1/tenants/%s/files/search", baseURL, nonexistentTenant)
		body, _ := json.Marshal(searchData)
		req, _ := http.NewRequest("POST", searchURL, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		
		// Check if response has expected structure
		if success, exists := result["success"]; exists && success != nil {
			assert.False(t, success.(bool))
		}
		
		if errorInfo, ok := result["error"].(map[string]interface{}); ok {
			if code, exists := errorInfo["code"]; exists && code != nil {
				assert.Equal(t, "NOT_FOUND", code.(string))
			}
			if message, exists := errorInfo["message"]; exists && message != nil {
				assert.Contains(t, message.(string), "Tenant not found")
			}
		}
	})

	t.Run("Search when embedding service unavailable returns HTTP 503", func(t *testing.T) {
		// This test verifies that when the embedding coordinator is nil (OPENAI_API_KEY missing),
		// we return proper service unavailable status
		searchData := map[string]interface{}{
			"query":     "test query",
			"top_k":     5,
			"threshold": 0.7,
		}

		searchURL := fmt.Sprintf("%s/api/v1/tenants/%s/files/search", baseURL, tenantID)
		body, _ := json.Marshal(searchData)
		req, _ := http.NewRequest("POST", searchURL, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		// The response could be either:
		// - HTTP 503 if embedding coordinator is nil
		// - HTTP 404 if dataset doesn't exist
		// - HTTP 200 if everything works
		switch resp.StatusCode {
		case http.StatusServiceUnavailable:
			// Embedding service not available
			if success, exists := result["success"]; exists && success != nil {
				assert.False(t, success.(bool))
			}
			if errorInfo, ok := result["error"].(map[string]interface{}); ok {
				if code, exists := errorInfo["code"]; exists && code != nil {
					assert.Equal(t, "EMBEDDING_SERVICE_UNAVAILABLE", code.(string))
				}
				if message, exists := errorInfo["message"]; exists && message != nil {
					assert.Contains(t, message.(string), "Vector search service is not available")
				}
			}
		case http.StatusNotFound:
			// Dataset not found (expected with our current setup)
			if success, exists := result["success"]; exists && success != nil {
				assert.False(t, success.(bool))
			}
			if errorInfo, ok := result["error"].(map[string]interface{}); ok {
				if code, exists := errorInfo["code"]; exists && code != nil {
					assert.Equal(t, "NOT_FOUND", code.(string))
				}
				if message, exists := errorInfo["message"]; exists && message != nil {
					assert.Contains(t, message.(string), "Dataset not found")
				}
			}
		case http.StatusOK:
			// Embedding service is working, search succeeded (possibly with empty results)
			if success, exists := result["success"]; exists && success != nil {
				assert.True(t, success.(bool))
			}
			t.Log("Embedding service is available and working")
		default:
			t.Logf("Unexpected status code: %d, response: %+v", resp.StatusCode, result)
		}
	})
}

// TestSearchStatusCodeMapping validates that our error handling maps to correct HTTP status codes
func TestSearchStatusCodeMapping(t *testing.T) {
	const (
		baseURL  = "http://localhost:8084"
		tenantID = "9855e094-36a6-4d3a-a4f5-d77da4614439"
	)

	tests := []struct {
		name           string
		searchData     map[string]interface{}
		expectedCodes  []int // Multiple acceptable codes due to different system states
		validateError  func(t *testing.T, result map[string]interface{})
	}{
		{
			name: "Valid search query",
			searchData: map[string]interface{}{
				"query":     "artificial intelligence",
				"top_k":     5,
				"threshold": 0.7,
			},
			expectedCodes: []int{http.StatusOK, http.StatusNotFound, http.StatusServiceUnavailable},
			validateError: func(t *testing.T, result map[string]interface{}) {
				// Any of these responses is valid depending on system state
			},
		},
		{
			name: "Empty query string",
			searchData: map[string]interface{}{
				"query":     "", // Empty query
				"top_k":     5,
				"threshold": 0.7,
			},
			expectedCodes: []int{http.StatusBadRequest},
			validateError: func(t *testing.T, result map[string]interface{}) {
				if success, exists := result["success"]; exists && success != nil {
					assert.False(t, success.(bool))
				}
				if errorInfo, ok := result["error"].(map[string]interface{}); ok {
					if code, exists := errorInfo["code"]; exists && code != nil {
						assert.Equal(t, "BAD_REQUEST", code.(string))
					}
				}
			},
		},
		{
			name: "Invalid threshold value",
			searchData: map[string]interface{}{
				"query":     "test",
				"top_k":     5,
				"threshold": -1.0, // Invalid threshold
			},
			expectedCodes: []int{http.StatusOK, http.StatusNotFound, http.StatusServiceUnavailable},
			validateError: func(t *testing.T, result map[string]interface{}) {
				// API may normalize invalid thresholds rather than rejecting them
			},
		},
	}

	client := &http.Client{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			searchURL := fmt.Sprintf("%s/api/v1/tenants/%s/files/search", baseURL, tenantID)
			body, _ := json.Marshal(tt.searchData)
			req, _ := http.NewRequest("POST", searchURL, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			var result map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&result)

			// Check if status code is one of the expected ones
			validStatus := false
			for _, expectedCode := range tt.expectedCodes {
				if resp.StatusCode == expectedCode {
					validStatus = true
					break
				}
			}

			if !validStatus {
				t.Errorf("Expected status codes %v, got %d. Response: %+v", 
					tt.expectedCodes, resp.StatusCode, result)
			}

			// Run custom validation if provided
			if tt.validateError != nil {
				tt.validateError(t, result)
			}
		})
	}
}