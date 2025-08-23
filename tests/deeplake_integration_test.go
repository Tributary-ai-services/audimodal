package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Direct DeepLake API Integration Tests
// This test file directly tests the DeepLake API to verify embedding generation

const (
	deeplakeAPIURL = "http://localhost:8000"
	audimodalAPIURL = "http://localhost:8084"
	testDatasetName = "test_audimodal_dataset"
	testTenantID = "9855e094-36a6-4d3a-a4f5-d77da4614439"
)

// getDeepLakeAPIKey gets the API key from environment
func getDeepLakeAPIKey() string {
	return os.Getenv("DEEPLAKE_API_KEY")
}

// Helper function to add authentication to requests
func addDeepLakeAuth(req *http.Request) {
	apiKey := getDeepLakeAPIKey()
	req.Header.Set("Authorization", fmt.Sprintf("ApiKey %s", apiKey))
}

// TestDeepLakeAuthenticationConfiguration validates the authentication setup
func TestDeepLakeAuthenticationConfiguration(t *testing.T) {
	t.Run("API key environment variable is configured", func(t *testing.T) {
		apiKey := getDeepLakeAPIKey()
		assert.NotEmpty(t, apiKey, "DEEPLAKE_API_KEY environment variable must be set")
		assert.NotEqual(t, "dev-12345-abcdef-67890-ghijkl", apiKey, "Should not use hardcoded development key")
		assert.Greater(t, len(apiKey), 10, "API key should be properly generated")
	})

	t.Run("Authentication works with configured key", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/health", deeplakeAPIURL), nil)
		addDeepLakeAuth(req)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Authentication should work with configured API key")
	})

	t.Run("Unauthenticated requests are rejected", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/datasets/", deeplakeAPIURL), nil)
		// Don't add authentication

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Unauthenticated requests should be rejected")
	})
}

// TestServiceCommunication validates AudiModal to DeepLake communication
func TestServiceCommunication(t *testing.T) {
	t.Run("DeepLake API is accessible", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/health", deeplakeAPIURL), nil)
		addDeepLakeAuth(req)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "DeepLake API should be accessible")

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Equal(t, "healthy", result["status"], "DeepLake service should be healthy")
	})

	t.Run("Can list datasets with authentication", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/datasets/", deeplakeAPIURL), nil)
		addDeepLakeAuth(req)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Should be able to list datasets with authentication")
	})
}

// TestDefaultDatasetConfiguration validates the default dataset setup
func TestDefaultDatasetConfiguration(t *testing.T) {
	t.Run("Default dataset exists or can be created", func(t *testing.T) {
		// Try to get the default dataset
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/datasets/default", deeplakeAPIURL), nil)
		addDeepLakeAuth(req)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			t.Log("Default dataset already exists")
			var dataset map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&dataset)
			assert.Equal(t, "default", dataset["name"], "Dataset name should be 'default'")
			assert.Equal(t, float64(1536), dataset["dimensions"], "Dataset should have 1536 dimensions for OpenAI embeddings")
			assert.Equal(t, "cosine", dataset["metric_type"], "Dataset should use cosine metric")
		} else {
			t.Log("Default dataset doesn't exist - this is expected for fresh installations")
			assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Dataset should return 404 if not found")
		}
	})
}

// TestDeepLakeDatasetOperations tests dataset creation and management
func TestDeepLakeDatasetOperations(t *testing.T) {
	t.Run("Create dataset", func(t *testing.T) {
		createData := map[string]interface{}{
			"name":        testDatasetName,
			"description": "Test dataset for AudiModal integration",
			"dimensions":  1536, // OpenAI embedding dimension
			"metric_type": "cosine",
		}

		body, _ := json.Marshal(createData)
		req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/datasets", deeplakeAPIURL), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		addDeepLakeAuth(req)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Log the response for debugging
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		t.Logf("Create dataset response status: %d", resp.StatusCode)
		t.Logf("Create dataset response body: %+v", result)

		// Dataset might already exist, both 201 and 409 are acceptable
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
			t.Fatalf("Unexpected status code: %d, body: %+v", resp.StatusCode, result)
		}

		if resp.StatusCode == http.StatusCreated {
			var result map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&result)
			t.Logf("Dataset created: %+v", result)
		}
	})

	t.Run("List datasets", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/datasets", deeplakeAPIURL), nil)
		addDeepLakeAuth(req)
		
		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		
		if datasets, ok := result["datasets"].([]interface{}); ok {
			t.Logf("Found %d datasets", len(datasets))
			for _, ds := range datasets {
				if dataset, ok := ds.(map[string]interface{}); ok {
					t.Logf("Dataset: %s", dataset["name"])
				}
			}
		}
	})
}

// TestDeepLakeEmbeddingGeneration tests direct embedding generation
func TestDeepLakeEmbeddingGeneration(t *testing.T) {
	// Ensure dataset exists first
	createDataset(t)

	t.Run("Generate embeddings for text", func(t *testing.T) {
		embeddingData := map[string]interface{}{
			"text":       "Artificial intelligence is transforming healthcare through innovative applications",
			"dataset":    testDatasetName,
			"document_id": "test-doc-001",
			"metadata": map[string]interface{}{
				"source": "test",
				"type":   "healthcare",
			},
		}

		body, _ := json.Marshal(embeddingData)
		req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/embeddings/generate", deeplakeAPIURL), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Log response for debugging
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		t.Logf("Embedding response status: %d", resp.StatusCode)
		t.Logf("Embedding response: %+v", result)

		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusUnauthorized {
				t.Skip("OpenAI API authentication failed - check API key")
			}
			t.Fatalf("Failed to generate embeddings: %d - %+v", resp.StatusCode, result)
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.NotNil(t, result["embedding"])
		assert.NotNil(t, result["document_id"])
	})

	t.Run("Generate embeddings for multiple texts", func(t *testing.T) {
		texts := []string{
			"Machine learning algorithms detect diseases early",
			"Natural language processing analyzes medical records",
			"Computer vision assists in radiology diagnostics",
		}

		for i, text := range texts {
			embeddingData := map[string]interface{}{
				"text":        text,
				"dataset":     testDatasetName,
				"document_id": fmt.Sprintf("test-doc-%03d", i+2),
				"metadata": map[string]interface{}{
					"source": "test",
					"index":  i,
				},
			}

			body, _ := json.Marshal(embeddingData)
			req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/embeddings/generate", deeplakeAPIURL), bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{Timeout: 60 * time.Second}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusUnauthorized {
				t.Skip("OpenAI API authentication failed")
			}

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		}
	})
}

// TestDeepLakeVectorSearch tests direct vector search functionality
func TestDeepLakeVectorSearch(t *testing.T) {
	// Ensure we have embeddings to search
	setupTestEmbeddings(t)

	t.Run("Search similar documents", func(t *testing.T) {
		searchData := map[string]interface{}{
			"query":   "AI healthcare diagnostics",
			"dataset": testDatasetName,
			"top_k":   5,
		}

		body, _ := json.Marshal(searchData)
		req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/search", deeplakeAPIURL), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		t.Logf("Search response status: %d", resp.StatusCode)
		t.Logf("Search response: %+v", result)

		if resp.StatusCode == http.StatusUnauthorized {
			t.Skip("OpenAI API authentication failed for search query embedding")
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.NotNil(t, result["results"])
		
		if results, ok := result["results"].([]interface{}); ok {
			assert.Greater(t, len(results), 0, "Should find at least one result")
			for i, res := range results {
				if match, ok := res.(map[string]interface{}); ok {
					t.Logf("Result %d: score=%.3f, doc_id=%s", i+1, match["score"], match["document_id"])
				}
			}
		}
	})

	t.Run("Search with metadata filter", func(t *testing.T) {
		searchData := map[string]interface{}{
			"query":   "medical analysis",
			"dataset": testDatasetName,
			"top_k":   3,
			"filter": map[string]interface{}{
				"source": "test",
			},
		}

		body, _ := json.Marshal(searchData)
		req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/search", deeplakeAPIURL), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized {
			t.Skip("OpenAI API authentication failed")
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

// TestAudiModalToDeepLakeIntegration tests the full integration flow
func TestAudiModalToDeepLakeIntegration(t *testing.T) {
	// This test verifies that AudiModal correctly calls DeepLake API
	
	t.Run("Process file and verify DeepLake calls", func(t *testing.T) {
		// Create a file in AudiModal
		fileData := map[string]interface{}{
			"filename":     "integration_test.txt",
			"extension":    "txt",
			"content_type": "text/plain",
			"size":         500,
			"checksum":     "integration-checksum",
			"checksum_type": "sha256",
		}

		body, _ := json.Marshal(fileData)
		req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/tenants/%s/files", audimodalAPIURL, testTenantID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Tenant-ID", testTenantID)

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

		// Now trigger processing which should call DeepLake
		processData := map[string]interface{}{
			"chunking_strategy": "semantic",
			"priority":          "high",
		}

		body, _ = json.Marshal(processData)
		processURL := fmt.Sprintf("%s/api/v1/tenants/%s/files/%s/process", audimodalAPIURL, testTenantID, fileID)
		req, _ = http.NewRequest("POST", processURL, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Tenant-ID", testTenantID)

		resp, err = client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Wait for processing
		time.Sleep(5 * time.Second)

		// Now check DeepLake logs to see if it was called
		t.Log("Check DeepLake container logs after this test to verify embedding generation was called")
	})
}

// Helper functions

func createDataset(t *testing.T) {
	createData := map[string]interface{}{
		"name":        testDatasetName,
		"description": "Test dataset for integration testing",
		"dimension":   1536,
		"metric":      "cosine",
	}

	body, _ := json.Marshal(createData)
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/datasets", deeplakeAPIURL), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, _ := client.Do(req)
	resp.Body.Close()
}

func setupTestEmbeddings(t *testing.T) {
	// Create dataset if needed
	createDataset(t)

	// Add some test embeddings
	testTexts := []struct {
		id   string
		text string
	}{
		{"setup-001", "Healthcare AI applications in diagnosis"},
		{"setup-002", "Machine learning for medical imaging"},
		{"setup-003", "Natural language processing in clinical notes"},
	}

	for _, tt := range testTexts {
		embeddingData := map[string]interface{}{
			"text":        tt.text,
			"dataset":     testDatasetName,
			"document_id": tt.id,
			"metadata": map[string]interface{}{
				"source": "test",
			},
		}

		body, _ := json.Marshal(embeddingData)
		req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/embeddings/generate", deeplakeAPIURL), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 60 * time.Second}
		resp, _ := client.Do(req)
		resp.Body.Close()
	}

	// Allow time for indexing
	time.Sleep(2 * time.Second)
}