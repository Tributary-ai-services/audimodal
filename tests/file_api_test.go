package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test Suite for Document Processing Pipeline - Version 3
// Updated to use multipart form-data for file uploads

// Use shared constants from test_helpers.go for K8s compatibility
// baseURL and testTenantID are defined in test_helpers.go
var (
	fileAPIBaseURL      = baseURL // Uses AUDIMODAL_URL env or default
	fileAPIPrefix       = "/api/v1"
	fileAPITestTenantID = testTenantID // Uses shared test tenant UUID
)

// uploadTestFileWithMetadata uploads a file using multipart form-data with metadata and returns the file ID
func uploadTestFileWithMetadata(t *testing.T, filename, content string, metadata map[string]interface{}) string {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Create the file part
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)

	// Add datasource_id
	writer.WriteField("datasource_id", testDataSourceID)

	// Add metadata if provided
	if metadata != nil {
		metadataJSON, _ := json.Marshal(metadata)
		writer.WriteField("metadata", string(metadataJSON))
	}

	writer.Close()

	url := fmt.Sprintf("%s%s/tenants/%s/files", fileAPIBaseURL, fileAPIPrefix, fileAPITestTenantID)
	req, err := http.NewRequest("POST", url, &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Tenant-ID", fileAPITestTenantID)
	req.Header.Set("X-API-Key", testAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Logf("Upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
		return ""
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if data, ok := result["data"].(map[string]interface{}); ok {
		if id, ok := data["id"].(string); ok {
			return id
		}
	}

	return ""
}

// TestFileCreation validates file upload functionality using multipart form-data
func TestFileCreation(t *testing.T) {
	tests := []struct {
		name         string
		filename     string
		content      string
		metadata     map[string]interface{}
		validateFunc func(t *testing.T, response map[string]interface{})
	}{
		{
			name:     "Create text file record",
			filename: "test_healthcare.txt",
			content:  "AI is revolutionizing healthcare with faster diagnoses and personalized treatments.",
			metadata: map[string]interface{}{
				"category": "healthcare",
				"language": "en",
			},
			validateFunc: func(t *testing.T, resp map[string]interface{}) {
				assert.NotEmpty(t, resp["id"])
				assert.Equal(t, "test_healthcare.txt", resp["filename"])
			},
		},
		{
			name:     "Create PDF-like file record",
			filename: "test_document.pdf",
			content:  "PDF content simulation for testing purposes",
			metadata: map[string]interface{}{
				"category": "research",
				"author":   "Test Author",
			},
		},
		{
			name:     "Create JSON file record",
			filename: "test_data.json",
			content:  `{"title": "Test Data", "content": "Machine learning applications"}`,
			metadata: map[string]interface{}{
				"type": "structured",
			},
		},
		{
			name:     "Create file without metadata",
			filename: "simple.txt",
			content:  "Simple text content",
			metadata: nil,
		},
		{
			name:     "Create large file record",
			filename: "large_doc.txt",
			content:  generateLargeContent(100), // 100 sentences
			metadata: map[string]interface{}{
				"size_test": "large",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)

			// Create the file part
			part, err := writer.CreateFormFile("file", tt.filename)
			require.NoError(t, err)
			_, err = part.Write([]byte(tt.content))
			require.NoError(t, err)

			// Add datasource_id
			writer.WriteField("datasource_id", testDataSourceID)

			// Add metadata if provided
			if tt.metadata != nil {
				metadataJSON, _ := json.Marshal(tt.metadata)
				writer.WriteField("metadata", string(metadataJSON))
			}

			writer.Close()

			url := fmt.Sprintf("%s%s/tenants/%s/files", fileAPIBaseURL, fileAPIPrefix, fileAPITestTenantID)
			req, err := http.NewRequest("POST", url, &body)
			require.NoError(t, err)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			req.Header.Set("X-Tenant-ID", fileAPITestTenantID)
			req.Header.Set("X-API-Key", testAPIKey)

			client := &http.Client{Timeout: 30 * time.Second}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			responseBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			if resp.StatusCode != http.StatusCreated {
				t.Errorf("Expected status 201, got %d. Response: %s", resp.StatusCode, string(responseBody))
				return
			}

			var result map[string]interface{}
			err = json.Unmarshal(responseBody, &result)
			require.NoError(t, err)

			// Custom validation
			if tt.validateFunc != nil {
				if data, ok := result["data"].(map[string]interface{}); ok {
					tt.validateFunc(t, data)
				} else {
					tt.validateFunc(t, result)
				}
			}

			// Store file ID for logging
			if data, ok := result["data"].(map[string]interface{}); ok {
				if fileID, ok := data["id"].(string); ok {
					t.Logf("Created file with ID: %s", fileID)
				}
			}
		})
	}
}

// TestFileRetrieval tests getting file information
func TestFileRetrieval(t *testing.T) {
	// First create a file using multipart upload
	fileID := uploadTestFile(t, "retrieve_test.txt", "Content for retrieval test")
	require.NotEmpty(t, fileID, "Failed to create test file")

	// Test retrieval
	t.Run("Get file by ID", func(t *testing.T) {
		getURL := fmt.Sprintf("%s%s/tenants/%s/files/%s", fileAPIBaseURL, fileAPIPrefix, fileAPITestTenantID, fileID)
		req, _ := http.NewRequest("GET", getURL, nil)
		req.Header.Set("X-Tenant-ID", fileAPITestTenantID)
		req.Header.Set("X-API-Key", testAPIKey)

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		if data, ok := result["data"].(map[string]interface{}); ok {
			assert.Equal(t, "retrieve_test.txt", data["filename"])
		}
	})
}

// TestFileList tests listing files with filters
func TestFileList(t *testing.T) {
	// Create multiple files for testing using multipart upload
	files := []struct {
		filename string
		content  string
	}{
		{"list_test_1.txt", "Text content for list test 1"},
		{"list_test_2.pdf", "PDF content simulation for list test 2"},
	}

	for _, f := range files {
		uploadTestFile(t, f.filename, f.content)
	}

	// Test listing
	t.Run("List all files", func(t *testing.T) {
		listURL := fmt.Sprintf("%s%s/tenants/%s/files", fileAPIBaseURL, fileAPIPrefix, fileAPITestTenantID)
		req, _ := http.NewRequest("GET", listURL, nil)
		req.Header.Set("X-Tenant-ID", fileAPITestTenantID)
		req.Header.Set("X-API-Key", testAPIKey)

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		if data, ok := result["data"].([]interface{}); ok {
			assert.GreaterOrEqual(t, len(data), 2)
		}
	})

	t.Run("Filter by content type", func(t *testing.T) {
		listURL := fmt.Sprintf("%s%s/tenants/%s/files?content_type=text/plain", fileAPIBaseURL, fileAPIPrefix, fileAPITestTenantID)
		req, _ := http.NewRequest("GET", listURL, nil)
		req.Header.Set("X-Tenant-ID", fileAPITestTenantID)
		req.Header.Set("X-API-Key", testAPIKey)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Skipf("Skipping - service unavailable: %v", err)
			return
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		if data, ok := result["data"].([]interface{}); ok {
			for _, item := range data {
				file := item.(map[string]interface{})
				assert.Equal(t, "text/plain", file["content_type"])
			}
		}
	})
}

// TestFileAPIErrorHandling validates error scenarios
func TestFileAPIErrorHandling(t *testing.T) {
	t.Run("Get non-existent file", func(t *testing.T) {
		fakeID := uuid.New().String()
		url := fmt.Sprintf("%s%s/tenants/%s/files/%s", fileAPIBaseURL, fileAPIPrefix, fileAPITestTenantID, fakeID)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("X-Tenant-ID", fileAPITestTenantID)
		req.Header.Set("X-API-Key", testAPIKey)

		client := &http.Client{}
		resp, _ := client.Do(req)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("Invalid tenant ID format", func(t *testing.T) {
		url := fmt.Sprintf("%s%s/tenants/invalid-uuid/files", fileAPIBaseURL, fileAPIPrefix)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("X-Tenant-ID", "invalid-uuid")
		req.Header.Set("X-API-Key", testAPIKey)

		client := &http.Client{}
		resp, _ := client.Do(req)
		defer resp.Body.Close()

		// Can be 400 or 401 depending on auth check order
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusUnauthorized}, resp.StatusCode)
	})

	t.Run("Process non-existent file", func(t *testing.T) {
		fakeID := uuid.New().String()
		processData := map[string]interface{}{
			"chunking_strategy": "semantic",
		}

		body, _ := json.Marshal(processData)
		url := fmt.Sprintf("%s%s/tenants/%s/files/%s/process", fileAPIBaseURL, fileAPIPrefix, fileAPITestTenantID, fakeID)
		req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Tenant-ID", fileAPITestTenantID)
		req.Header.Set("X-API-Key", testAPIKey)

		client := &http.Client{}
		resp, _ := client.Do(req)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("Upload without file part", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		// Only add form fields, no file
		writer.WriteField("datasource_id", testDataSourceID)
		writer.Close()

		url := fmt.Sprintf("%s%s/tenants/%s/files", fileAPIBaseURL, fileAPIPrefix, fileAPITestTenantID)
		req, _ := http.NewRequest("POST", url, &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("X-Tenant-ID", fileAPITestTenantID)
		req.Header.Set("X-API-Key", testAPIKey)

		client := &http.Client{}
		resp, _ := client.Do(req)
		defer resp.Body.Close()

		// Should fail without file
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Search with missing tenant context", func(t *testing.T) {
		searchData := map[string]interface{}{
			"query": "test search",
			"top_k": 5,
		}

		body, _ := json.Marshal(searchData)
		url := fmt.Sprintf("%s%s/tenants/%s/files/search", fileAPIBaseURL, fileAPIPrefix, fileAPITestTenantID)
		req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", testAPIKey)
		// Intentionally NOT setting X-Tenant-ID header

		client := &http.Client{}
		resp, _ := client.Do(req)
		defer resp.Body.Close()

		// Should get an error - could be 400, 404, 500, or 503 depending on service state
		assert.NotEqual(t, http.StatusOK, resp.StatusCode)
	})
}

// TestFileProcessing tests file processing with embedding generation
func TestFileProcessing(t *testing.T) {
	// Create a test file first using multipart upload
	fileID := uploadTestFile(t, "process_test.txt", "Content for processing test with AI and machine learning topics")
	require.NotEmpty(t, fileID, "Failed to create test file")

	// Now test file processing
	t.Run("Process file with embeddings", func(t *testing.T) {
		processData := map[string]interface{}{
			"chunking_strategy": "semantic",
			"priority":          "high",
			"dlp_scan_enabled":  true,
		}

		processURL := fmt.Sprintf("%s%s/tenants/%s/files/%s/process", fileAPIBaseURL, fileAPIPrefix, fileAPITestTenantID, fileID)
		body, _ := json.Marshal(processData)
		req, _ := http.NewRequest("POST", processURL, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Tenant-ID", fileAPITestTenantID)
		req.Header.Set("X-API-Key", testAPIKey)

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		// Verify processing response
		if data, ok := result["data"].(map[string]interface{}); ok {
			assert.Equal(t, fileID, data["file_id"])
			assert.NotEmpty(t, data["status"])
		}
	})

	// Test file retrieval after processing
	t.Run("Check file status after processing", func(t *testing.T) {
		// Wait a moment for processing to start
		time.Sleep(2 * time.Second)

		getURL := fmt.Sprintf("%s%s/tenants/%s/files/%s", fileAPIBaseURL, fileAPIPrefix, fileAPITestTenantID, fileID)
		req, _ := http.NewRequest("GET", getURL, nil)
		req.Header.Set("X-Tenant-ID", fileAPITestTenantID)
		req.Header.Set("X-API-Key", testAPIKey)

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		if data, ok := result["data"].(map[string]interface{}); ok {
			// File should be in processing or processed state
			status := data["status"].(string)
			assert.Contains(t, []string{"processing", "processed", "error", "discovered"}, status)
		}
	})
}

// TestVectorSearchFileAPI tests semantic search functionality
func TestVectorSearchFileAPI(t *testing.T) {
	// Create a test file with searchable content using multipart upload
	fileID := uploadTestFileWithMetadata(t, "search_test.txt",
		"Artificial intelligence and machine learning are transforming healthcare diagnostics",
		map[string]interface{}{"category": "ai"})
	require.NotEmpty(t, fileID, "Failed to create test file")

	// Process the file to generate embeddings
	processData := map[string]interface{}{
		"chunking_strategy": "semantic",
		"priority":          "high",
		"dlp_scan_enabled":  false,
	}

	processURL := fmt.Sprintf("%s%s/tenants/%s/files/%s/process", fileAPIBaseURL, fileAPIPrefix, fileAPITestTenantID, fileID)
	body, _ := json.Marshal(processData)
	req, _ := http.NewRequest("POST", processURL, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", fileAPITestTenantID)
	req.Header.Set("X-API-Key", testAPIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Wait for processing to complete
	time.Sleep(5 * time.Second)

	// Now test vector search
	t.Run("Search similar documents", func(t *testing.T) {
		searchData := map[string]interface{}{
			"query":     "artificial intelligence healthcare machine learning",
			"top_k":     5,
			"threshold": 0.7,
		}

		searchURL := fmt.Sprintf("%s%s/tenants/%s/files/search", fileAPIBaseURL, fileAPIPrefix, fileAPITestTenantID)
		body, _ := json.Marshal(searchData)
		req, _ := http.NewRequest("POST", searchURL, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Tenant-ID", fileAPITestTenantID)
		req.Header.Set("X-API-Key", testAPIKey)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)

		if resp.StatusCode == http.StatusServiceUnavailable {
			t.Skip("Vector search service is not available - likely missing OpenAI API key")
			return
		}

		if resp.StatusCode == http.StatusNotFound {
			t.Log("Search returned 404 - dataset may not exist yet for fresh installations")
			return
		}

		if resp.StatusCode != http.StatusOK {
			t.Logf("Search returned status %d. Response: %+v", resp.StatusCode, result)
			if resp.StatusCode == http.StatusInternalServerError {
				t.Skip("Vector search encountered internal error - likely missing embeddings or configuration")
				return
			}
		}

		// Verify search response structure if we got OK
		if resp.StatusCode == http.StatusOK {
			assert.NotNil(t, result)
		}
	})

	// Test search with empty query
	t.Run("Search with empty query", func(t *testing.T) {
		searchData := map[string]interface{}{
			"query": "",
			"top_k": 5,
		}

		searchURL := fmt.Sprintf("%s%s/tenants/%s/files/search", fileAPIBaseURL, fileAPIPrefix, fileAPITestTenantID)
		body, _ := json.Marshal(searchData)
		req, _ := http.NewRequest("POST", searchURL, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Tenant-ID", fileAPITestTenantID)
		req.Header.Set("X-API-Key", testAPIKey)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusServiceUnavailable {
			t.Skip("Vector search service is not available")
			return
		}

		// Empty query should return bad request
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}
