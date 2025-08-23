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

// Test Suite for Document Processing Pipeline
// This comprehensive test suite validates:
// 1. File upload functionality
// 2. Embedding generation through DeepLake API
// 3. Vector search capabilities
// 4. End-to-end document processing workflow

const (
	baseURL         = "http://localhost:8084"
	deeplakeURL     = "http://localhost:8000"
	testTenantID    = "550e8400-e29b-41d4-a716-446655440000" // Fixed UUID for testing
	testDatasetName = "test_documents"
)

// TestFileUpload validates file upload functionality
func TestFileUpload(t *testing.T) {
	tests := []struct {
		name           string
		fileName       string
		fileContent    string
		contentType    string
		metadata       map[string]interface{}
		expectedStatus int
		validateFunc   func(t *testing.T, response map[string]interface{})
	}{
		{
			name:        "Upload text document",
			fileName:    "test_healthcare.txt",
			fileContent: "AI is revolutionizing healthcare with faster diagnoses and personalized treatments.",
			contentType: "text/plain",
			metadata: map[string]interface{}{
				"category": "healthcare",
				"language": "en",
			},
			expectedStatus: 201,
			validateFunc: func(t *testing.T, resp map[string]interface{}) {
				assert.NotEmpty(t, resp["id"])
				assert.Equal(t, "test_healthcare.txt", resp["filename"])
				assert.Equal(t, "text/plain", resp["content_type"])
			},
		},
		{
			name:        "Upload PDF document",
			fileName:    "test_document.pdf",
			fileContent: "%PDF-1.4 test content",
			contentType: "application/pdf",
			metadata: map[string]interface{}{
				"category": "research",
				"author":   "Test Author",
			},
			expectedStatus: 201,
		},
		{
			name:        "Upload JSON document",
			fileName:    "test_data.json",
			fileContent: `{"title": "Test Data", "content": "Machine learning applications"}`,
			contentType: "application/json",
			metadata: map[string]interface{}{
				"type": "structured",
			},
			expectedStatus: 201,
		},
		{
			name:           "Upload without metadata",
			fileName:       "simple.txt",
			fileContent:    "Simple text content",
			contentType:    "text/plain",
			metadata:       nil,
			expectedStatus: 201,
		},
		{
			name:        "Upload large document",
			fileName:    "large_doc.txt",
			fileContent: generateLargeContent(1024 * 1024), // 1MB
			contentType: "text/plain",
			metadata: map[string]interface{}{
				"size_test": "large",
			},
			expectedStatus: 201,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create multipart form
			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)

			// Add file
			part, err := writer.CreateFormFile("file", tt.fileName)
			require.NoError(t, err)
			_, err = io.WriteString(part, tt.fileContent)
			require.NoError(t, err)

			// Add metadata if provided
			if tt.metadata != nil {
				metadataJSON, err := json.Marshal(tt.metadata)
				require.NoError(t, err)
				err = writer.WriteField("metadata", string(metadataJSON))
				require.NoError(t, err)
			}

			err = writer.Close()
			require.NoError(t, err)

			// Create request
			req, err := http.NewRequest("POST", fmt.Sprintf("%s/v1/files", baseURL), body)
			require.NoError(t, err)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			req.Header.Set("X-Tenant-ID", testTenantID)

			// Send request
			client := &http.Client{Timeout: 30 * time.Second}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Check status
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			// Parse response
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			// Custom validation
			if tt.validateFunc != nil {
				tt.validateFunc(t, result)
			}
		})
	}
}

// TestEmbeddingGeneration validates embedding generation through DeepLake API
func TestEmbeddingGeneration(t *testing.T) {
	tests := []struct {
		name           string
		documentID     string
		content        string
		chunkSize      int
		expectedChunks int
		validateFunc   func(t *testing.T, embeddings []map[string]interface{})
	}{
		{
			name:       "Generate embeddings for short text",
			documentID: uuid.New().String(),
			content:    "Artificial intelligence is transforming healthcare.",
			chunkSize:  1000,
			expectedChunks: 1,
		},
		{
			name:       "Generate embeddings with chunking",
			documentID: uuid.New().String(),
			content:    generateLargeContent(5000), // Content that will be chunked
			chunkSize:  1000,
			expectedChunks: 5,
		},
		{
			name:       "Generate embeddings for technical content",
			documentID: uuid.New().String(),
			content: `Machine learning algorithms analyze medical images with remarkable accuracy.
                     Natural language processing helps doctors analyze patient records quickly.
                     Deep learning models can predict disease progression and treatment outcomes.`,
			chunkSize:  500,
			expectedChunks: 1,
		},
	}

	// Ensure dataset exists
	createTestDataset(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First, process document through audimodal embeddings endpoint
			requestBody := map[string]interface{}{
				"document_id": tt.documentID,
				"content":     tt.content,
				"metadata": map[string]interface{}{
					"test_case": tt.name,
					"timestamp": time.Now().Unix(),
				},
				"chunk_size": tt.chunkSize,
			}

			body, err := json.Marshal(requestBody)
			require.NoError(t, err)

			req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/embeddings/documents", baseURL), bytes.NewBuffer(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Tenant-ID", testTenantID)

			client := &http.Client{Timeout: 60 * time.Second}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Check response
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			// Verify embeddings were created
			assert.NotNil(t, result["vectors"])
			vectors := result["vectors"].([]interface{})
			assert.Len(t, vectors, tt.expectedChunks)

			// Validate embeddings
			if tt.validateFunc != nil {
				embeddingMaps := make([]map[string]interface{}, len(vectors))
				for i, v := range vectors {
					embeddingMaps[i] = v.(map[string]interface{})
				}
				tt.validateFunc(t, embeddingMaps)
			}
		})
	}
}

// TestVectorSearch validates search functionality
func TestVectorSearch(t *testing.T) {
	// First, populate test data
	setupSearchTestData(t)

	tests := []struct {
		name          string
		query         string
		topK          int
		threshold     float64
		filters       map[string]interface{}
		expectedCount int
		validateFunc  func(t *testing.T, results []map[string]interface{})
	}{
		{
			name:          "Search healthcare content",
			query:         "artificial intelligence in medical diagnosis",
			topK:          5,
			threshold:     0.7,
			expectedCount: 3,
			validateFunc: func(t *testing.T, results []map[string]interface{}) {
				// Verify results are healthcare-related
				for _, result := range results {
					metadata := result["metadata"].(map[string]interface{})
					assert.Contains(t, []string{"healthcare", "medical"}, metadata["category"])
				}
			},
		},
		{
			name:      "Search with metadata filter",
			query:     "machine learning applications",
			topK:      10,
			threshold: 0.6,
			filters: map[string]interface{}{
				"category": "technology",
			},
			expectedCount: 2,
		},
		{
			name:          "Semantic similarity search",
			query:         "patient treatment outcomes",
			topK:          3,
			threshold:     0.8,
			expectedCount: 1,
		},
		{
			name:          "Search with low threshold",
			query:         "data processing",
			topK:          20,
			threshold:     0.3,
			expectedCount: 5, // Should return more results
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			searchRequest := map[string]interface{}{
				"query":     tt.query,
				"dataset":   testDatasetName,
				"top_k":     tt.topK,
				"threshold": tt.threshold,
			}

			if tt.filters != nil {
				searchRequest["filters"] = tt.filters
			}

			body, err := json.Marshal(searchRequest)
			require.NoError(t, err)

			req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/embeddings/search", baseURL), bytes.NewBuffer(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Tenant-ID", testTenantID)

			client := &http.Client{Timeout: 30 * time.Second}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			// Validate results
			results := result["results"].([]interface{})
			assert.GreaterOrEqual(t, len(results), tt.expectedCount)

			// Convert to map slice for validation
			if tt.validateFunc != nil {
				resultMaps := make([]map[string]interface{}, len(results))
				for i, r := range results {
					resultMaps[i] = r.(map[string]interface{})
				}
				tt.validateFunc(t, resultMaps)
			}
		})
	}
}

// TestEndToEndWorkflow validates complete document processing pipeline
func TestEndToEndWorkflow(t *testing.T) {
	// 1. Upload document
	fileContent := `Advanced Machine Learning in Healthcare

	Artificial intelligence and machine learning are revolutionizing healthcare by enabling:
	- Early disease detection through pattern recognition
	- Personalized treatment recommendations
	- Drug discovery acceleration
	- Clinical decision support systems
	
	These technologies analyze vast amounts of medical data to improve patient outcomes.`

	fileID := uploadTestFile(t, "ml_healthcare.txt", fileContent)

	// 2. Wait for processing
	time.Sleep(2 * time.Second)

	// 3. Generate embeddings
	generateEmbeddingsForFile(t, fileID, fileContent)

	// 4. Search for related content
	searchResults := performSearch(t, "machine learning disease detection", 5)
	
	// 5. Validate results
	assert.GreaterOrEqual(t, len(searchResults), 1)
	
	// Verify our uploaded document is in results
	found := false
	for _, result := range searchResults {
		metadata := result["metadata"].(map[string]interface{})
		if metadata["file_id"] == fileID {
			found = true
			break
		}
	}
	assert.True(t, found, "Uploaded document should be found in search results")
}

// TestErrorHandling validates error scenarios
func TestErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		testFunc       func(t *testing.T)
		expectedError  string
	}{
		{
			name: "Upload without tenant ID",
			testFunc: func(t *testing.T) {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)
				part, _ := writer.CreateFormFile("file", "test.txt")
				io.WriteString(part, "test content")
				writer.Close()

				req, _ := http.NewRequest("POST", fmt.Sprintf("%s/v1/files", baseURL), body)
				req.Header.Set("Content-Type", writer.FormDataContentType())
				// Intentionally not setting X-Tenant-ID

				client := &http.Client{}
				resp, _ := client.Do(req)
				defer resp.Body.Close()

				assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			},
		},
		{
			name: "Search with invalid dataset",
			testFunc: func(t *testing.T) {
				searchRequest := map[string]interface{}{
					"query":   "test query",
					"dataset": "non_existent_dataset",
				}

				body, _ := json.Marshal(searchRequest)
				req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/embeddings/search", baseURL), bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Tenant-ID", testTenantID)

				client := &http.Client{}
				resp, _ := client.Do(req)
				defer resp.Body.Close()

				assert.Equal(t, http.StatusNotFound, resp.StatusCode)
			},
		},
		{
			name: "Upload file too large",
			testFunc: func(t *testing.T) {
				// Create 100MB content (assuming limit is lower)
				largeContent := generateLargeContent(100 * 1024 * 1024)
				
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)
				part, _ := writer.CreateFormFile("file", "huge.txt")
				io.WriteString(part, largeContent)
				writer.Close()

				req, _ := http.NewRequest("POST", fmt.Sprintf("%s/v1/files", baseURL), body)
				req.Header.Set("Content-Type", writer.FormDataContentType())
				req.Header.Set("X-Tenant-ID", testTenantID)

				client := &http.Client{}
				resp, _ := client.Do(req)
				defer resp.Body.Close()

				assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

// TestConcurrentOperations validates system under concurrent load
func TestConcurrentOperations(t *testing.T) {
	t.Run("Concurrent file uploads", func(t *testing.T) {
		const numUploads = 10
		done := make(chan bool, numUploads)

		for i := 0; i < numUploads; i++ {
			go func(idx int) {
				fileName := fmt.Sprintf("concurrent_%d.txt", idx)
				content := fmt.Sprintf("Concurrent test content %d", idx)
				uploadTestFile(t, fileName, content)
				done <- true
			}(i)
		}

		// Wait for all uploads
		for i := 0; i < numUploads; i++ {
			<-done
		}
	})

	t.Run("Concurrent searches", func(t *testing.T) {
		const numSearches = 20
		done := make(chan bool, numSearches)

		queries := []string{
			"artificial intelligence",
			"machine learning",
			"healthcare technology",
			"medical diagnosis",
			"data processing",
		}

		for i := 0; i < numSearches; i++ {
			go func(idx int) {
				query := queries[idx%len(queries)]
				performSearch(t, query, 5)
				done <- true
			}(i)
		}

		// Wait for all searches
		for i := 0; i < numSearches; i++ {
			<-done
		}
	})
}

// TestDataPersistence validates data persistence across operations
func TestDataPersistence(t *testing.T) {
	// Upload file
	fileID := uploadTestFile(t, "persistence_test.txt", "Data persistence test content")
	
	// Generate embeddings
	generateEmbeddingsForFile(t, fileID, "Data persistence test content")
	
	// Search immediately
	results1 := performSearch(t, "data persistence", 10)
	
	// Wait and search again
	time.Sleep(5 * time.Second)
	results2 := performSearch(t, "data persistence", 10)
	
	// Results should be consistent
	assert.Equal(t, len(results1), len(results2))
}

// Helper Functions

func generateLargeContent(size int) string {
	const chunk = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. "
	result := ""
	for len(result) < size {
		result += chunk
	}
	return result[:size]
}

func createTestDataset(t *testing.T) {
	// Create dataset in DeepLake
	request := map[string]interface{}{
		"name":        testDatasetName,
		"description": "Test dataset for integration tests",
		"dimension":   1536, // OpenAI embedding dimension
	}

	body, _ := json.Marshal(request)
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/embeddings/datasets", baseURL), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", testTenantID)

	client := &http.Client{}
	resp, _ := client.Do(req)
	resp.Body.Close()
	// Ignore errors - dataset might already exist
}

func setupSearchTestData(t *testing.T) {
	testDocuments := []struct {
		id       string
		content  string
		category string
	}{
		{
			id:       "doc1",
			content:  "Artificial intelligence in medical diagnosis and treatment planning",
			category: "healthcare",
		},
		{
			id:       "doc2",
			content:  "Machine learning algorithms for predictive healthcare analytics",
			category: "healthcare",
		},
		{
			id:       "doc3",
			content:  "Deep learning applications in radiology and medical imaging",
			category: "medical",
		},
		{
			id:       "doc4",
			content:  "Natural language processing for clinical documentation",
			category: "technology",
		},
		{
			id:       "doc5",
			content:  "Data mining techniques in healthcare research",
			category: "technology",
		},
	}

	for _, doc := range testDocuments {
		request := map[string]interface{}{
			"document_id": doc.id,
			"content":     doc.content,
			"metadata": map[string]interface{}{
				"category": doc.category,
			},
		}

		body, _ := json.Marshal(request)
		req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/embeddings/documents", baseURL), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Tenant-ID", testTenantID)

		client := &http.Client{}
		resp, _ := client.Do(req)
		resp.Body.Close()
	}

	// Allow indexing
	time.Sleep(2 * time.Second)
}

func uploadTestFile(t *testing.T, fileName, content string) string {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", fileName)
	io.WriteString(part, content)
	writer.Close()

	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/v1/files", baseURL), body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Tenant-ID", testTenantID)

	client := &http.Client{}
	resp, _ := client.Do(req)
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	
	return result["id"].(string)
}

func generateEmbeddingsForFile(t *testing.T, fileID, content string) {
	request := map[string]interface{}{
		"document_id": fileID,
		"content":     content,
		"metadata": map[string]interface{}{
			"file_id": fileID,
		},
	}

	body, _ := json.Marshal(request)
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/embeddings/documents", baseURL), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", testTenantID)

	client := &http.Client{}
	resp, _ := client.Do(req)
	resp.Body.Close()
}

func performSearch(t *testing.T, query string, topK int) []map[string]interface{} {
	searchRequest := map[string]interface{}{
		"query":   query,
		"dataset": testDatasetName,
		"top_k":   topK,
	}

	body, _ := json.Marshal(searchRequest)
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/embeddings/search", baseURL), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", testTenantID)

	client := &http.Client{}
	resp, _ := client.Do(req)
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	
	results := result["results"].([]interface{})
	resultMaps := make([]map[string]interface{}, len(results))
	for i, r := range results {
		resultMaps[i] = r.(map[string]interface{})
	}
	
	return resultMaps
}