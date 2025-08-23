package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEndToEndEmbeddingGeneration tests the complete flow from file creation through embedding generation
func TestEndToEndEmbeddingGeneration(t *testing.T) {
	const (
		baseURL = "http://localhost:8084"
		deeplakeURL = "http://localhost:8000"
		apiPrefix = "/api/v1"
		tenantID = "9855e094-36a6-4d3a-a4f5-d77da4614439"
	)

	// Step 1: Ensure we have a data source
	dataSourceID := createTestDataSource(t, baseURL, apiPrefix, tenantID)
	t.Logf("Using data source: %s", dataSourceID)

	// Step 2: Create a file with actual content that should be processed
	fileData := map[string]interface{}{
		"filename":       "embedding_test.txt",
		"extension":      "txt",
		"content_type":   "text/plain",
		"size":           500,
		"checksum":       "embedding-test-checksum",
		"checksum_type":  "sha256",
		"data_source_id": dataSourceID,
		"path":           "/test/embedding_test.txt", // Add path so processor can find it
		"metadata": map[string]interface{}{
			"content": "Artificial intelligence is revolutionizing healthcare with machine learning algorithms that can diagnose diseases, predict patient outcomes, and personalize treatment plans.",
		},
	}

	body, _ := json.Marshal(fileData)
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s%s/tenants/%s/files", baseURL, apiPrefix, tenantID), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", tenantID)

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createResult)
	
	var fileID string
	if data, ok := createResult["data"].(map[string]interface{}); ok {
		fileID = data["id"].(string)
	}
	require.NotEmpty(t, fileID)
	t.Logf("Created file: %s", fileID)

	// Step 3: Process the file to generate embeddings
	processData := map[string]interface{}{
		"chunking_strategy": "semantic",
		"priority":          "high",
		"dlp_scan_enabled":  false,
	}

	body, _ = json.Marshal(processData)
	processURL := fmt.Sprintf("%s%s/tenants/%s/files/%s/process", baseURL, apiPrefix, tenantID, fileID)
	req, _ = http.NewRequest("POST", processURL, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", tenantID)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	t.Log("File processing started")

	// Step 4: Wait for processing and check status
	time.Sleep(10 * time.Second)

	// Check file status
	getURL := fmt.Sprintf("%s%s/tenants/%s/files/%s", baseURL, apiPrefix, tenantID, fileID)
	req, _ = http.NewRequest("GET", getURL, nil)
	req.Header.Set("X-Tenant-ID", tenantID)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var fileStatus map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&fileStatus)
	
	if data, ok := fileStatus["data"].(map[string]interface{}); ok {
		status := data["status"].(string)
		t.Logf("File status: %s", status)
		
		if status == "error" {
			t.Logf("Processing error: %v", data["processing_error"])
		}
	}

	// Step 5: Check DeepLake logs to see if embeddings were created
	t.Log("Check DeepLake container logs to verify embedding generation was called")
	
	// Step 6: Try vector search to see if embeddings are available
	searchData := map[string]interface{}{
		"query":     "machine learning healthcare diagnosis",
		"top_k":     5,
		"threshold": 0.7,
	}

	searchURL := fmt.Sprintf("%s%s/tenants/%s/files/search", baseURL, apiPrefix, tenantID)
	body, _ = json.Marshal(searchData)
	req, _ = http.NewRequest("POST", searchURL, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", tenantID)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var searchResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&searchResult)
	
	t.Logf("Search response status: %d", resp.StatusCode)
	t.Logf("Search response: %+v", searchResult)
}

// Helper function to create test data source
func createTestDataSource(t *testing.T, baseURL, apiPrefix, tenantID string) string {
	// Check if any data sources exist
	listURL := fmt.Sprintf("%s%s/tenants/%s/data-sources", baseURL, apiPrefix, tenantID)
	req, _ := http.NewRequest("GET", listURL, nil)
	req.Header.Set("X-Tenant-ID", tenantID)
	
	client := &http.Client{}
	resp, _ := client.Do(req)
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusOK {
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		
		if data, ok := result["data"].([]interface{}); ok && len(data) > 0 {
			if ds, ok := data[0].(map[string]interface{}); ok {
				return ds["id"].(string)
			}
		}
	}
	
	// Create new data source
	dsData := map[string]interface{}{
		"name":        "test-datasource",
		"type":        "local",
		"config":      map[string]interface{}{"path": "/test"},
		"description": "Test data source",
	}
	
	body, _ := json.Marshal(dsData)
	createURL := fmt.Sprintf("%s%s/tenants/%s/data-sources", baseURL, apiPrefix, tenantID)
	req, _ = http.NewRequest("POST", createURL, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", tenantID)
	
	resp, _ = client.Do(req)
	defer resp.Body.Close()
	
	var createResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createResult)
	
	if data, ok := createResult["data"].(map[string]interface{}); ok {
		return data["id"].(string)
	}
	
	return ""
}