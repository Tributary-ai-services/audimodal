package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jscharber/eAIIngest/pkg/embeddings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock HTTP server for testing
type mockDeepLakeServer struct {
	server   *httptest.Server
	requests []mockRequest
	mu       sync.RWMutex
}

type mockRequest struct {
	Method string
	Path   string
	Body   string
}

func newMockDeepLakeServer() *mockDeepLakeServer {
	mock := &mockDeepLakeServer{
		requests: make([]mockRequest, 0),
	}
	
	// Use a custom handler that can handle dynamic paths
	mock.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		
		switch {
		case path == "/api/v1/health":
			mock.handleHealth(w, r)
		case path == "/api/v1/datasets/" || path == "/api/v1/datasets":
			mock.handleDatasets(w, r)
		case path == "/api/v1/datasets/test-dataset" || path == "/api/v1/datasets/test-dataset-id":
			mock.handleDataset(w, r)
		case path == "/api/v1/datasets/test-dataset/stats" || path == "/api/v1/datasets/test-dataset-id/stats":
			mock.handleDatasetStats(w, r)
		case path == "/api/v1/datasets/test-dataset-id/vectors/batch":
			mock.handleVectorsBatch(w, r)
		case path == "/api/v1/datasets/test-dataset-id/vectors/vec-1":
			mock.handleVector(w, r)
		case path == "/api/v1/datasets/test-dataset-id/search":
			mock.handleSearch(w, r)
		case path == "/api/v1/datasets/test-dataset-id/search/text":
			mock.handleTextSearch(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	return mock
}

func (m *mockDeepLakeServer) close() {
	m.server.Close()
}

func (m *mockDeepLakeServer) recordRequest(method, path, body string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, mockRequest{
		Method: method,
		Path:   path,
		Body:   body,
	})
}

func (m *mockDeepLakeServer) getRequests() []mockRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]mockRequest(nil), m.requests...)
}

func (m *mockDeepLakeServer) clearRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = m.requests[:0]
}

func (m *mockDeepLakeServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	m.recordRequest(r.Method, r.URL.Path, "")
	
	response := map[string]interface{}{
		"status":    "healthy",
		"service":   "deeplake-api",
		"version":   "1.0.0",
		"timestamp": time.Now().Format(time.RFC3339),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (m *mockDeepLakeServer) handleDatasets(w http.ResponseWriter, r *http.Request) {
	body := ""
	if r.Body != nil {
		bodyBytes, _ := io.ReadAll(r.Body)
		body = string(bodyBytes)
	}
	m.recordRequest(r.Method, r.URL.Path, body)
	
	switch r.Method {
	case "GET":
		// List datasets
		datasets := []DatasetResponse{
			{
				ID:          "test-dataset-id",
				Name:        "test-dataset",
				Description: "Test dataset",
				Dimensions:  1536,
				MetricType:  "cosine",
				IndexType:   "hnsw",
				Metadata:    map[string]interface{}{"test": "true"},
				VectorCount: 10,
				StorageSize: 1024,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(datasets)
		
	case "POST":
		// Create dataset
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		response := DatasetResponse{
			ID:          "new-dataset-id",
			Name:        "new-dataset",
			Description: "New test dataset",
			Dimensions:  1536,
			MetricType:  "cosine",
			IndexType:   "hnsw",
			Metadata:    map[string]interface{}{},
			VectorCount: 0,
			StorageSize: 0,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		json.NewEncoder(w).Encode(response)
		
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *mockDeepLakeServer) handleDataset(w http.ResponseWriter, r *http.Request) {
	m.recordRequest(r.Method, r.URL.Path, "")
	
	switch r.Method {
	case "GET":
		// Get dataset info
		dataset := DatasetResponse{
			ID:          "test-dataset-id",
			Name:        "test-dataset",
			Description: "Test dataset",
			Dimensions:  1536,
			MetricType:  "cosine",
			IndexType:   "hnsw",
			Metadata:    map[string]interface{}{"test": "true"},
			VectorCount: 10,
			StorageSize: 1024,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dataset)
		
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *mockDeepLakeServer) handleDatasetStats(w http.ResponseWriter, r *http.Request) {
	m.recordRequest(r.Method, r.URL.Path, "")
	
	// Return DatasetStats structure that the client expects
	stats := DatasetStats{
		Dataset: DatasetResponse{
			ID:          "test-dataset-id",
			Name:        "test-dataset",
			Description: "Test dataset",
			Dimensions:  1536,
			MetricType:  "cosine",
			IndexType:   "hnsw",
			Metadata:    map[string]interface{}{"test": "true"},
			VectorCount: 10,
			StorageSize: 1024,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		VectorCount: 10,
		StorageSize: 1024,
		MetadataStats: map[string]int{
			"test": 10,
		},
		IndexStats: map[string]interface{}{
			"index_size": 512,
		},
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (m *mockDeepLakeServer) handleVectorsBatch(w http.ResponseWriter, r *http.Request) {
	body := ""
	if r.Body != nil {
		bodyBytes, _ := io.ReadAll(r.Body)
		body = string(bodyBytes)
	}
	m.recordRequest(r.Method, r.URL.Path, body)
	
	switch r.Method {
	case "POST":
		// Insert vectors batch
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		response := VectorBatchResponse{
			InsertedCount:    3,
			SkippedCount:     0,
			FailedCount:      0,
			ErrorMessages:    []string{},
			ProcessingTimeMs: 150.5,
		}
		json.NewEncoder(w).Encode(response)
		
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *mockDeepLakeServer) handleVector(w http.ResponseWriter, r *http.Request) {
	body := ""
	if r.Body != nil {
		bodyBytes, _ := io.ReadAll(r.Body)
		body = string(bodyBytes)
	}
	m.recordRequest(r.Method, r.URL.Path, body)
	
	switch r.Method {
	case "GET":
		// Get vector
		vector := VectorResponse{
			ID:         "vec-1",
			DatasetID:  "test-dataset-id",
			DocumentID: "doc-1",
			ChunkID:    "chunk-1",
			Values:     []float32{0.1, 0.2, 0.3},
			Content:    "Test content",
			Metadata:   map[string]interface{}{"test": "true"},
			Dimensions: 3,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(vector)
		
	case "PUT":
		// Update vector
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"message":   "vector updated successfully",
			"vector_id": "vec-1",
		}
		json.NewEncoder(w).Encode(response)
		
	case "DELETE":
		// Delete vector
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"message":   "vector deleted successfully",
			"vector_id": "vec-1",
		}
		json.NewEncoder(w).Encode(response)
		
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *mockDeepLakeServer) handleSearch(w http.ResponseWriter, r *http.Request) {
	body := ""
	if r.Body != nil {
		bodyBytes, _ := io.ReadAll(r.Body)
		body = string(bodyBytes)
	}
	m.recordRequest(r.Method, r.URL.Path, body)
	
	// Mock search response
	searchResponse := SearchResponse{
		Results: []SearchResultItem{
			{
				Vector: VectorResponse{
					ID:         "vec-1",
					DatasetID:  "test-dataset-id",
					DocumentID: "doc-1",
					Values:     []float32{0.1, 0.2, 0.3},
					Content:    "Test content 1",
					Metadata:   map[string]interface{}{"category": "test"},
					Dimensions: 3,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				},
				Score:    0.95,
				Distance: 0.05,
				Rank:     1,
			},
		},
		TotalFound:      1,
		HasMore:         false,
		QueryTimeMs:     25.5,
		EmbeddingTimeMs: 10.2,
		Stats: SearchStats{
			VectorsScanned:  100,
			IndexHits:       10,
			FilteredResults: 1,
		},
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(searchResponse)
}

func (m *mockDeepLakeServer) handleTextSearch(w http.ResponseWriter, r *http.Request) {
	body := ""
	if r.Body != nil {
		bodyBytes, _ := io.ReadAll(r.Body)
		body = string(bodyBytes)
	}
	m.recordRequest(r.Method, r.URL.Path, body)
	
	// Mock text search response (similar to vector search)
	searchResponse := SearchResponse{
		Results: []SearchResultItem{
			{
				Vector: VectorResponse{
					ID:         "vec-2",
					DatasetID:  "test-dataset-id",
					DocumentID: "doc-2",
					Values:     []float32{0.4, 0.5, 0.6},
					Content:    "Text search result",
					Metadata:   map[string]interface{}{"category": "text"},
					Dimensions: 3,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				},
				Score:    0.88,
				Distance: 0.12,
				Rank:     1,
			},
		},
		TotalFound:      1,
		HasMore:         false,
		QueryTimeMs:     30.8,
		EmbeddingTimeMs: 15.3,
		Stats: SearchStats{
			VectorsScanned:  50,
			IndexHits:       5,
			FilteredResults: 1,
		},
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(searchResponse)
}

// Test client creation and configuration
func TestNewDeepLakeAPIClient(t *testing.T) {
	tests := []struct {
		name        string
		config      *DeepLakeAPIConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid configuration",
			config: &DeepLakeAPIConfig{
				BaseURL:   "http://localhost:8000",
				APIKey:    "test-key",
				TenantID:  "test-tenant",
				Timeout:   30 * time.Second,
				Retries:   3,
				UserAgent: "test-client",
			},
			expectError: false,
		},
		{
			name: "Missing base URL",
			config: &DeepLakeAPIConfig{
				APIKey:    "test-key",
				TenantID:  "test-tenant",
				Timeout:   30 * time.Second,
				Retries:   3,
				UserAgent: "test-client",
			},
			expectError: true,
			errorMsg:    "base URL is required",
		},
		{
			name: "Missing API key",
			config: &DeepLakeAPIConfig{
				BaseURL:   "http://localhost:8000",
				TenantID:  "test-tenant",
				Timeout:   30 * time.Second,
				Retries:   3,
				UserAgent: "test-client",
			},
			expectError: true,
			errorMsg:    "API key is required",
		},
		{
			name: "Default values applied",
			config: &DeepLakeAPIConfig{
				BaseURL: "http://localhost:8000",
				APIKey:  "test-key",
				// Other fields should get defaults - zero values will be replaced
				Timeout: 0,   // Should become 30*time.Second
				Retries: 0,   // Should become 3 (when 0, not when negative)
			},
			expectError: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewDeepLakeAPIClient(tt.config)
			
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, client)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				
				// Verify defaults were applied
				if tt.name == "Default values applied" {
					assert.Equal(t, 30*time.Second, tt.config.Timeout) // Timeout 0 becomes 30s
					assert.Equal(t, 0, tt.config.Retries)              // Retries 0 stays 0 (only negative becomes 3)
					assert.Equal(t, "eAIIngest-Go-Client/1.0", tt.config.UserAgent)
				}
				
				// Clean up
				client.Close()
			}
		})
	}
}

// Test dataset operations
func TestDeepLakeAPIClient_DatasetOperations(t *testing.T) {
	mock := newMockDeepLakeServer()
	defer mock.close()
	
	config := &DeepLakeAPIConfig{
		BaseURL:   mock.server.URL,
		APIKey:    "test-key",
		Timeout:   5 * time.Second,
		Retries:   1,
		UserAgent: "test-client",
	}
	
	client, err := NewDeepLakeAPIClient(config)
	require.NoError(t, err)
	defer client.Close()
	
	ctx := context.Background()
	
	t.Run("CreateDataset", func(t *testing.T) {
		mock.clearRequests()
		
		datasetConfig := &embeddings.DatasetConfig{
			Name:        "test-dataset",
			Description: "Test dataset",
			Dimensions:  1536,
			MetricType:  "cosine",
			IndexType:   "hnsw",
			Metadata: map[string]interface{}{
				"test": "true",
			},
		}
		
		err := client.CreateDataset(ctx, datasetConfig)
		assert.NoError(t, err)
		
		requests := mock.getRequests()
		assert.Len(t, requests, 1) // Just one POST request to create dataset
		
		// Check that the create request was made
		createRequest := requests[0]
		assert.Equal(t, "POST", createRequest.Method)
		// Note: HTTP client may normalize trailing slashes
		assert.Contains(t, createRequest.Path, "/api/v1/datasets")
		
		// Verify request body contains dataset config
		assert.Contains(t, createRequest.Body, "test-dataset")
		assert.Contains(t, createRequest.Body, "cosine")
	})
	
	t.Run("ListDatasets", func(t *testing.T) {
		mock.clearRequests()
		
		datasets, err := client.ListDatasets(ctx)
		assert.NoError(t, err)
		assert.Len(t, datasets, 1)
		
		dataset := datasets[0]
		assert.Equal(t, "test-dataset", dataset.Name)
		assert.Equal(t, 1536, dataset.Dimensions)
		assert.Equal(t, "cosine", dataset.MetricType)
		
		requests := mock.getRequests()
		assert.Len(t, requests, 1)
		assert.Equal(t, "GET", requests[0].Method)
		assert.Contains(t, requests[0].Path, "/api/v1/datasets")
	})
	
	t.Run("GetDatasetInfo", func(t *testing.T) {
		mock.clearRequests()
		
		info, err := client.GetDatasetInfo(ctx, "test-dataset")
		assert.NoError(t, err)
		assert.Equal(t, "test-dataset", info.Name)
		assert.Equal(t, 1536, info.Dimensions)
		
		requests := mock.getRequests()
		assert.Len(t, requests, 2) // One for ID lookup, one for info
		assert.Equal(t, "GET", requests[1].Method)
		assert.Equal(t, "/api/v1/datasets/test-dataset-id", requests[1].Path)
	})
	
	t.Run("GetDatasetStats", func(t *testing.T) {
		mock.clearRequests()
		
		stats, err := client.GetDatasetStats(ctx, "test-dataset")
		assert.NoError(t, err)
		assert.Equal(t, "test-dataset", stats["name"])
		assert.EqualValues(t, 10, stats["vector_count"])
		assert.EqualValues(t, 1536, stats["dimensions"])
		
		requests := mock.getRequests()
		assert.Len(t, requests, 2) // One for ID lookup, one for stats
	})
}

// Test vector operations
func TestDeepLakeAPIClient_VectorOperations(t *testing.T) {
	mock := newMockDeepLakeServer()
	defer mock.close()
	
	config := &DeepLakeAPIConfig{
		BaseURL:   mock.server.URL,
		APIKey:    "test-key",
		Timeout:   5 * time.Second,
		Retries:   1,
		UserAgent: "test-client",
	}
	
	client, err := NewDeepLakeAPIClient(config)
	require.NoError(t, err)
	defer client.Close()
	
	ctx := context.Background()
	
	t.Run("InsertVectors", func(t *testing.T) {
		mock.clearRequests()
		
		vectors := []*embeddings.DocumentVector{
			{
				ID:         "vec-1",
				DocumentID: "doc-1",
				ChunkID:    "chunk-1",
				Vector:     []float32{0.1, 0.2, 0.3},
				Content:    "Test content 1",
				Metadata:   map[string]interface{}{"test": "true"},
			},
			{
				ID:         "vec-2",
				DocumentID: "doc-2",
				ChunkID:    "chunk-1",
				Vector:     []float32{0.4, 0.5, 0.6},
				Content:    "Test content 2",
				Metadata:   map[string]interface{}{"test": "true"},
			},
		}
		
		err := client.InsertVectors(ctx, "test-dataset", vectors)
		assert.NoError(t, err)
		
		requests := mock.getRequests()
		assert.Len(t, requests, 2) // One for dataset lookup, one for insert
		
		insertRequest := requests[1]
		assert.Equal(t, "POST", insertRequest.Method)
		assert.Equal(t, "/api/v1/datasets/test-dataset-id/vectors/batch", insertRequest.Path)
		assert.Contains(t, insertRequest.Body, "vec-1")
		assert.Contains(t, insertRequest.Body, "doc-1")
	})
	
	t.Run("GetVector", func(t *testing.T) {
		mock.clearRequests()
		
		vector, err := client.GetVector(ctx, "test-dataset", "vec-1")
		assert.NoError(t, err)
		assert.Equal(t, "vec-1", vector.ID)
		assert.Equal(t, "doc-1", vector.DocumentID)
		assert.Equal(t, []float32{0.1, 0.2, 0.3}, vector.Vector)
		
		requests := mock.getRequests()
		assert.Len(t, requests, 2) // One for dataset lookup, one for get
	})
	
	t.Run("UpdateVector", func(t *testing.T) {
		mock.clearRequests()
		
		metadata := map[string]interface{}{
			"updated": true,
			"version": "1.1",
		}
		
		err := client.UpdateVector(ctx, "test-dataset", "vec-1", metadata)
		assert.NoError(t, err)
		
		requests := mock.getRequests()
		assert.Len(t, requests, 2) // One for dataset lookup, one for update
		
		updateRequest := requests[1]
		assert.Equal(t, "PUT", updateRequest.Method)
		assert.Contains(t, updateRequest.Body, "updated")
		assert.Contains(t, updateRequest.Body, "version")
	})
	
	t.Run("DeleteVector", func(t *testing.T) {
		mock.clearRequests()
		
		err := client.DeleteVector(ctx, "test-dataset", "vec-1")
		assert.NoError(t, err)
		
		requests := mock.getRequests()
		assert.Len(t, requests, 2) // One for dataset lookup, one for delete
		
		deleteRequest := requests[1]
		assert.Equal(t, "DELETE", deleteRequest.Method)
		assert.Equal(t, "/api/v1/datasets/test-dataset-id/vectors/vec-1", deleteRequest.Path)
	})
}

// Test search operations
func TestDeepLakeAPIClient_SearchOperations(t *testing.T) {
	mock := newMockDeepLakeServer()
	defer mock.close()
	
	config := &DeepLakeAPIConfig{
		BaseURL:   mock.server.URL,
		APIKey:    "test-key",
		Timeout:   5 * time.Second,
		Retries:   1,
		UserAgent: "test-client",
	}
	
	client, err := NewDeepLakeAPIClient(config)
	require.NoError(t, err)
	defer client.Close()
	
	ctx := context.Background()
	
	t.Run("SearchSimilar", func(t *testing.T) {
		mock.clearRequests()
		
		queryVector := []float32{0.1, 0.2, 0.3}
		options := &embeddings.SearchOptions{
			TopK:            5,
			Threshold:       0.7,
			MetricType:      "cosine",
			IncludeContent:  true,
			IncludeMetadata: true,
		}
		
		result, err := client.SearchSimilar(ctx, "test-dataset", queryVector, options)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result.Results, 1)
		assert.Equal(t, 1, result.TotalFound)
		assert.False(t, result.HasMore)
		assert.Greater(t, result.QueryTime, float64(0))
		
		// Check result details
		searchResult := result.Results[0]
		assert.Equal(t, "vec-1", searchResult.DocumentVector.ID)
		assert.Equal(t, float32(0.95), searchResult.Score)
		assert.Equal(t, 1, searchResult.Rank)
		
		requests := mock.getRequests()
		assert.Len(t, requests, 2) // One for dataset lookup, one for search
		
		searchRequest := requests[1]
		assert.Equal(t, "POST", searchRequest.Method)
		assert.Equal(t, "/api/v1/datasets/test-dataset-id/search", searchRequest.Path)
		assert.Contains(t, searchRequest.Body, "query_vector")
		assert.Contains(t, searchRequest.Body, "options")
	})
	
	t.Run("SearchByText", func(t *testing.T) {
		mock.clearRequests()
		
		queryText := "test search query"
		options := &embeddings.SearchOptions{
			TopK:            3,
			IncludeContent:  true,
			IncludeMetadata: true,
		}
		
		result, err := client.SearchByText(ctx, "test-dataset", queryText, options)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result.Results, 1)
		
		// Check result details
		searchResult := result.Results[0]
		assert.Equal(t, "vec-2", searchResult.DocumentVector.ID)
		assert.Equal(t, "Text search result", searchResult.DocumentVector.Content)
		
		requests := mock.getRequests()
		assert.Len(t, requests, 2)
		
		searchRequest := requests[1]
		assert.Equal(t, "POST", searchRequest.Method)
		assert.Equal(t, "/api/v1/datasets/test-dataset-id/search/text", searchRequest.Path)
		assert.Contains(t, searchRequest.Body, "query_text")
		assert.Contains(t, searchRequest.Body, "test search query")
	})
}

// Test error handling
func TestDeepLakeAPIClient_ErrorHandling(t *testing.T) {
	// Test with a server that returns errors
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/datasets/":
			if r.Method == "GET" {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		case "/api/v1/datasets/nonexistent":
			http.Error(w, "Dataset not found", http.StatusNotFound)
		default:
			http.Error(w, "Not found", http.StatusNotFound)
		}
	}))
	defer errorServer.Close()
	
	config := &DeepLakeAPIConfig{
		BaseURL:   errorServer.URL,
		APIKey:    "test-key",
		Timeout:   1 * time.Second,
		Retries:   0, // No retries for faster tests
		UserAgent: "test-client",
	}
	
	client, err := NewDeepLakeAPIClient(config)
	require.NoError(t, err)
	defer client.Close()
	
	ctx := context.Background()
	
	t.Run("ServerError", func(t *testing.T) {
		_, err := client.ListDatasets(ctx)
		assert.Error(t, err)
		
		// Should be an EmbeddingError
		embeddingErr, ok := err.(*embeddings.EmbeddingError)
		assert.True(t, ok)
		assert.Equal(t, "api_error", embeddingErr.Type)
	})
	
	t.Run("DatasetNotFound", func(t *testing.T) {
		_, err := client.GetDatasetInfo(ctx, "nonexistent")
		assert.Error(t, err)
		
		embeddingErr, ok := err.(*embeddings.EmbeddingError)
		assert.True(t, ok)
		// The client returns api_error for HTTP errors
		assert.Equal(t, "api_error", embeddingErr.Type)
	})
}

// Test timeout and retry logic
func TestDeepLakeAPIClient_TimeoutAndRetry(t *testing.T) {
	// Create a slow server
	retryCount := 0
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		retryCount++
		if retryCount < 3 {
			// Simulate timeout by sleeping
			time.Sleep(2 * time.Second)
		}
		
		// Return success on 3rd attempt
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]DatasetResponse{})
	}))
	defer slowServer.Close()
	
	config := &DeepLakeAPIConfig{
		BaseURL:   slowServer.URL,
		APIKey:    "test-key",
		Timeout:   1 * time.Second, // Short timeout
		Retries:   2,               // Allow retries
		UserAgent: "test-client",
	}
	
	client, err := NewDeepLakeAPIClient(config)
	require.NoError(t, err)
	defer client.Close()
	
	ctx := context.Background()
	
	t.Run("RetriesOnTimeout", func(t *testing.T) {
		retryCount = 0
		start := time.Now()
		
		_, err := client.ListDatasets(ctx)
		
		duration := time.Since(start)
		
		// Should succeed after retries
		assert.NoError(t, err)
		
		// Should have retried
		assert.Equal(t, 3, retryCount)
		
		// Should have taken time for retries
		assert.Greater(t, duration, 2*time.Second)
	})
}

// Test helper functions
func TestHelperFunctions(t *testing.T) {
	t.Run("getIntPtr", func(t *testing.T) {
		// Test with zero value
		ptr := getIntPtr(0)
		assert.Nil(t, ptr)
		
		// Test with non-zero value
		ptr = getIntPtr(42)
		assert.NotNil(t, ptr)
		assert.Equal(t, 42, *ptr)
	})
	
	t.Run("getIntValue", func(t *testing.T) {
		// Test with nil pointer
		value := getIntValue(nil)
		assert.Equal(t, 0, value)
		
		// Test with valid pointer
		intVal := 42
		value = getIntValue(&intVal)
		assert.Equal(t, 42, value)
	})
	
	t.Run("convertMetadata", func(t *testing.T) {
		// Test with nil metadata
		result := convertMetadata(nil)
		assert.Nil(t, result)
		
		// Test with metadata
		metadata := map[string]interface{}{
			"string": "test",
			"int":    42,
			"bool":   true,
			"float":  3.14,
		}
		
		result = convertMetadata(metadata)
		assert.Equal(t, "test", result["string"])
		assert.Equal(t, "42", result["int"])
		assert.Equal(t, "true", result["bool"])
		assert.Equal(t, "3.14", result["float"])
	})
}

// Benchmark tests
func BenchmarkDeepLakeAPIClient_SearchSimilar(b *testing.B) {
	mock := newMockDeepLakeServer()
	defer mock.close()
	
	config := &DeepLakeAPIConfig{
		BaseURL:   mock.server.URL,
		APIKey:    "test-key",
		Timeout:   30 * time.Second,
		Retries:   1,
		UserAgent: "benchmark-client",
	}
	
	client, err := NewDeepLakeAPIClient(config)
	require.NoError(b, err)
	defer client.Close()
	
	ctx := context.Background()
	queryVector := []float32{0.1, 0.2, 0.3}
	options := &embeddings.SearchOptions{
		TopK:            10,
		IncludeContent:  true,
		IncludeMetadata: true,
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.SearchSimilar(ctx, "test-dataset", queryVector, options)
		if err != nil {
			b.Fatal(err)
		}
	}
}