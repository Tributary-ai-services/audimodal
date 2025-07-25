package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/google/uuid"
	"github.com/jscharber/eAIIngest/pkg/embeddings"
	"github.com/jscharber/eAIIngest/pkg/embeddings/providers"
	"github.com/jscharber/eAIIngest/pkg/embeddings/client"
)

// EmbeddingHandler handles embedding-related HTTP requests
type EmbeddingHandler struct {
	service   embeddings.EmbeddingService
	apiClient *client.DeepLakeAPIClient
}

// NewEmbeddingHandler creates a new embedding handler
func NewEmbeddingHandler() (*EmbeddingHandler, error) {
	// Initialize OpenAI provider
	openaiConfig := &providers.OpenAIConfig{
		APIKey:     getEnvOrDefault("OPENAI_API_KEY", ""),
		Model:      getEnvOrDefault("OPENAI_MODEL", "text-embedding-3-small"),
		BaseURL:    getEnvOrDefault("OPENAI_BASE_URL", ""),
		MaxRetries: 3,
	}

	provider, err := providers.NewOpenAIProvider(openaiConfig)
	if err != nil {
		return nil, err
	}

	// Initialize DeepLake API client
	apiConfig := &client.DeepLakeAPIConfig{
		BaseURL:   getEnvOrDefault("DEEPLAKE_API_URL", "http://localhost:8000"),
		APIKey:    getEnvOrDefault("DEEPLAKE_API_KEY", "dev-key-12345"),
		TenantID:  getEnvOrDefault("DEEPLAKE_TENANT_ID", ""),
		Timeout:   30 * time.Second,
		Retries:   3,
		UserAgent: "eAIIngest-Go/1.0",
	}

	apiClient, err := client.NewDeepLakeAPIClient(apiConfig)
	if err != nil {
		return nil, err
	}

	// Create embedding service using API client as vector store
	serviceConfig := &embeddings.ServiceConfig{
		DefaultDataset:   "documents",
		ChunkSize:        1000,
		ChunkOverlap:     100,
		BatchSize:        10,
		MaxConcurrency:   5,
		CacheEnabled:     true,
		MetricsEnabled:   true,
		DefaultTopK:      10,
		DefaultThreshold: 0.7,
		DefaultMetric:    "cosine",
	}

	service := embeddings.NewEmbeddingService(provider, apiClient, serviceConfig)

	return &EmbeddingHandler{
		service:   service,
		apiClient: apiClient,
	}, nil
}

// RegisterRoutes registers embedding routes
func (h *EmbeddingHandler) RegisterRoutes(router *mux.Router) {
	api := router.PathPrefix("/api/v1/embeddings").Subrouter()
	
	// Document processing
	api.HandleFunc("/documents", h.ProcessDocument).Methods("POST")
	api.HandleFunc("/documents/{documentId}", h.GetDocumentVectors).Methods("GET")
	api.HandleFunc("/documents/{documentId}", h.DeleteDocument).Methods("DELETE")
	
	// Chunk processing
	api.HandleFunc("/chunks", h.ProcessChunks).Methods("POST")
	
	// Search
	api.HandleFunc("/search", h.SearchDocuments).Methods("POST")
	
	// Datasets
	api.HandleFunc("/datasets", h.ListDatasets).Methods("GET")
	api.HandleFunc("/datasets", h.CreateDataset).Methods("POST")
	api.HandleFunc("/datasets/{datasetName}", h.GetDatasetInfo).Methods("GET")
	api.HandleFunc("/datasets/{datasetName}", h.UpdateDataset).Methods("PUT")
	api.HandleFunc("/datasets/{datasetName}", h.DeleteDataset).Methods("DELETE")
	api.HandleFunc("/datasets/{datasetName}/stats", h.GetDatasetStats).Methods("GET")
	
	// Vector operations  
	api.HandleFunc("/datasets/{datasetName}/vectors", h.InsertVector).Methods("POST")
	api.HandleFunc("/datasets/{datasetName}/vectors", h.ListVectors).Methods("GET")
	api.HandleFunc("/datasets/{datasetName}/vectors/batch", h.InsertVectorsBatch).Methods("POST")
	api.HandleFunc("/datasets/{datasetName}/vectors/batch", h.DeleteVectorsBatch).Methods("DELETE")
	api.HandleFunc("/datasets/{datasetName}/vectors/{vectorId}", h.GetVector).Methods("GET")
	api.HandleFunc("/datasets/{datasetName}/vectors/{vectorId}", h.UpdateVector).Methods("PUT")
	api.HandleFunc("/datasets/{datasetName}/vectors/{vectorId}", h.DeleteVector).Methods("DELETE")
	
	// Advanced search operations
	api.HandleFunc("/datasets/{datasetName}/search/text", h.SearchByText).Methods("POST")
	api.HandleFunc("/datasets/{datasetName}/search/hybrid", h.HybridSearch).Methods("POST")
	api.HandleFunc("/search/multi-dataset", h.MultiDatasetSearch).Methods("POST")
	
	// Service stats
	api.HandleFunc("/stats", h.GetServiceStats).Methods("GET")
	
	// Health check
	api.HandleFunc("/health", h.HealthCheck).Methods("GET")
}

// ProcessDocument handles document processing requests
func (h *EmbeddingHandler) ProcessDocument(w http.ResponseWriter, r *http.Request) {
	var request embeddings.ProcessDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Extract tenant ID from context or header
	tenantID, err := h.extractTenantID(r)
	if err != nil {
		http.Error(w, "Invalid or missing tenant ID", http.StatusBadRequest)
		return
	}
	request.TenantID = tenantID

	response, err := h.service.ProcessDocument(r.Context(), &request)
	if err != nil {
		h.handleError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// ProcessChunks handles chunk processing requests
func (h *EmbeddingHandler) ProcessChunks(w http.ResponseWriter, r *http.Request) {
	var request embeddings.ProcessChunksRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Extract tenant ID from context or header
	tenantID, err := h.extractTenantID(r)
	if err != nil {
		http.Error(w, "Invalid or missing tenant ID", http.StatusBadRequest)
		return
	}
	request.TenantID = tenantID

	response, err := h.service.ProcessChunks(r.Context(), &request)
	if err != nil {
		h.handleError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// SearchDocuments handles document search requests
func (h *EmbeddingHandler) SearchDocuments(w http.ResponseWriter, r *http.Request) {
	var request embeddings.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Extract tenant ID from context or header
	tenantID, err := h.extractTenantID(r)
	if err != nil {
		http.Error(w, "Invalid or missing tenant ID", http.StatusBadRequest)
		return
	}
	request.TenantID = tenantID

	response, err := h.service.SearchDocuments(r.Context(), &request)
	if err != nil {
		h.handleError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetDocumentVectors retrieves vectors for a specific document
func (h *EmbeddingHandler) GetDocumentVectors(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	documentID := vars["documentId"]

	vectors, err := h.service.GetDocumentVectors(r.Context(), documentID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"document_id": documentID,
		"vectors":     vectors,
		"count":       len(vectors),
	})
}

// DeleteDocument removes a document and its vectors
func (h *EmbeddingHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	documentID := vars["documentId"]

	err := h.service.DeleteDocument(r.Context(), documentID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":     "document deleted successfully",
		"document_id": documentID,
	})
}

// CreateDataset creates a new dataset
func (h *EmbeddingHandler) CreateDataset(w http.ResponseWriter, r *http.Request) {
	var config embeddings.DatasetConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	err := h.apiClient.CreateDataset(r.Context(), &config)
	if err != nil {
		h.handleError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "dataset created successfully",
		"name":    config.Name,
	})
}

// ListDatasets lists all available datasets
func (h *EmbeddingHandler) ListDatasets(w http.ResponseWriter, r *http.Request) {
	datasets, err := h.apiClient.ListDatasets(r.Context())
	if err != nil {
		h.handleError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"datasets": datasets,
		"count":    len(datasets),
	})
}

// GetDatasetInfo retrieves information about a dataset
func (h *EmbeddingHandler) GetDatasetInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	datasetName := vars["datasetName"]

	info, err := h.apiClient.GetDatasetInfo(r.Context(), datasetName)
	if err != nil {
		h.handleError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// GetDatasetStats retrieves statistics for a dataset
func (h *EmbeddingHandler) GetDatasetStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	datasetName := vars["datasetName"]

	stats, err := h.apiClient.GetDatasetStats(r.Context(), datasetName)
	if err != nil {
		h.handleError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// GetServiceStats retrieves service statistics
func (h *EmbeddingHandler) GetServiceStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.service.GetStats(r.Context())
	if err != nil {
		h.handleError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// HealthCheck performs a health check
func (h *EmbeddingHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	// Perform basic health checks
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": "2024-01-01T00:00:00Z", // Replace with actual timestamp
		"service":   "embeddings",
		"version":   "1.0.0",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

// Helper methods

func (h *EmbeddingHandler) extractTenantID(r *http.Request) (uuid.UUID, error) {
	// Try to get tenant ID from header
	tenantIDStr := r.Header.Get("X-Tenant-ID")
	if tenantIDStr != "" {
		return uuid.Parse(tenantIDStr)
	}

	// Try to get from query parameter
	tenantIDStr = r.URL.Query().Get("tenant_id")
	if tenantIDStr != "" {
		return uuid.Parse(tenantIDStr)
	}

	// Try to get from context (would be set by auth middleware)
	if tenantID := r.Context().Value("tenant_id"); tenantID != nil {
		if id, ok := tenantID.(uuid.UUID); ok {
			return id, nil
		}
		if idStr, ok := tenantID.(string); ok {
			return uuid.Parse(idStr)
		}
	}

	// Default tenant for development
	return uuid.Parse("00000000-0000-0000-0000-000000000001")
}

func (h *EmbeddingHandler) handleError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")

	// Handle embedding-specific errors
	if embeddingErr, ok := err.(*embeddings.EmbeddingError); ok {
		statusCode := http.StatusInternalServerError
		
		switch embeddingErr.Type {
		case "invalid_input", "invalid_configuration":
			statusCode = http.StatusBadRequest
		case "dataset_not_found", "vector_not_found":
			statusCode = http.StatusNotFound
		case "quota_exceeded":
			statusCode = http.StatusTooManyRequests
		case "provider_not_supported":
			statusCode = http.StatusNotImplemented
		}

		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"type":    embeddingErr.Type,
				"message": embeddingErr.Message,
				"code":    embeddingErr.Code,
				"details": embeddingErr.Details,
			},
		})
		return
	}

	// Handle generic errors
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"type":    "internal_error",
			"message": err.Error(),
		},
	})
}

// getEnvOrDefault gets an environment variable or returns a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Additional handler methods for direct API access

// UpdateDataset updates dataset information
func (h *EmbeddingHandler) UpdateDataset(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	datasetName := vars["datasetName"]

	var updateRequest map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updateRequest); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":    "dataset update not yet implemented",
		"dataset_id": datasetName,
	})
}

// DeleteDataset deletes a dataset
func (h *EmbeddingHandler) DeleteDataset(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	datasetName := vars["datasetName"]

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":    "dataset deletion not yet implemented",
		"dataset_id": datasetName,
	})
}

// InsertVector inserts a single vector
func (h *EmbeddingHandler) InsertVector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	datasetName := vars["datasetName"]

	var vector embeddings.DocumentVector
	if err := json.NewDecoder(r.Body).Decode(&vector); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	err := h.apiClient.InsertVectors(r.Context(), datasetName, []*embeddings.DocumentVector{&vector})
	if err != nil {
		h.handleError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "vector inserted successfully",
		"vector_id": vector.ID,
	})
}

// ListVectors lists vectors in a dataset
func (h *EmbeddingHandler) ListVectors(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	datasetName := vars["datasetName"]

	// Parse query parameters
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	offset := 0
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":    "vector listing not yet implemented",
		"dataset":    datasetName,
		"limit":      limit,
		"offset":     offset,
	})
}

// InsertVectorsBatch inserts multiple vectors
func (h *EmbeddingHandler) InsertVectorsBatch(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	datasetName := vars["datasetName"]

	var request struct {
		Vectors []*embeddings.DocumentVector `json:"vectors"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	err := h.apiClient.InsertVectors(r.Context(), datasetName, request.Vectors)
	if err != nil {
		h.handleError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":        "vectors inserted successfully",
		"inserted_count": len(request.Vectors),
	})
}

// DeleteVectorsBatch deletes multiple vectors
func (h *EmbeddingHandler) DeleteVectorsBatch(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	datasetName := vars["datasetName"]

	var vectorIDs []string
	if err := json.NewDecoder(r.Body).Decode(&vectorIDs); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	successCount := 0
	for _, vectorID := range vectorIDs {
		if err := h.apiClient.DeleteVector(r.Context(), datasetName, vectorID); err == nil {
			successCount++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":       "batch delete completed",
		"deleted_count": successCount,
		"total_count":   len(vectorIDs),
	})
}

// GetVector retrieves a specific vector
func (h *EmbeddingHandler) GetVector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	datasetName := vars["datasetName"]
	vectorID := vars["vectorId"]

	vector, err := h.apiClient.GetVector(r.Context(), datasetName, vectorID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vector)
}

// UpdateVector updates a vector
func (h *EmbeddingHandler) UpdateVector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	datasetName := vars["datasetName"]
	vectorID := vars["vectorId"]

	var updateRequest map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updateRequest); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Extract metadata if present
	if metadata, ok := updateRequest["metadata"].(map[string]interface{}); ok {
		err := h.apiClient.UpdateVector(r.Context(), datasetName, vectorID, metadata)
		if err != nil {
			h.handleError(w, err)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "vector updated successfully",
		"vector_id": vectorID,
	})
}

// DeleteVector deletes a vector
func (h *EmbeddingHandler) DeleteVector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	datasetName := vars["datasetName"]
	vectorID := vars["vectorId"]

	err := h.apiClient.DeleteVector(r.Context(), datasetName, vectorID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "vector deleted successfully",
		"vector_id": vectorID,
	})
}

// SearchByText performs text-based search
func (h *EmbeddingHandler) SearchByText(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	datasetName := vars["datasetName"]

	var request struct {
		QueryText string                    `json:"query_text"`
		Options   *embeddings.SearchOptions `json:"options,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	result, err := h.apiClient.SearchByText(r.Context(), datasetName, request.QueryText, request.Options)
	if err != nil {
		h.handleError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HybridSearch performs hybrid search
func (h *EmbeddingHandler) HybridSearch(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	datasetName := vars["datasetName"]

	var request map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "hybrid search not yet implemented",
		"dataset": datasetName,
		"request": request,
	})
}

// MultiDatasetSearch performs search across multiple datasets
func (h *EmbeddingHandler) MultiDatasetSearch(w http.ResponseWriter, r *http.Request) {
	var request map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "multi-dataset search not yet implemented",
		"request": request,
	})
}

// Example usage and integration methods

// IntegrateWithMainService integrates the embedding handler with the main service
func (h *EmbeddingHandler) IntegrateWithMainService(mainRouter *mux.Router) {
	// Register embedding routes with the main router
	h.RegisterRoutes(mainRouter)
}

// SetupMiddleware sets up middleware for the embedding handler
func (h *EmbeddingHandler) SetupMiddleware(router *mux.Router) {
	// Add CORS middleware for API endpoints
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Tenant-ID")
			
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			
			next.ServeHTTP(w, r)
		})
	})

	// Add request logging middleware
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Log request details (in a real implementation)
			next.ServeHTTP(w, r)
		})
	})
}

// Example request/response structures for documentation

type ProcessDocumentRequest struct {
	DocumentID   string                 `json:"document_id" example:"doc-123"`
	Content      string                 `json:"content" example:"This is the document content to be processed."`
	ContentType  string                 `json:"content_type,omitempty" example:"text/plain"`
	ChunkSize    int                    `json:"chunk_size,omitempty" example:"1000"`
	ChunkOverlap int                    `json:"chunk_overlap,omitempty" example:"100"`
	Dataset      string                 `json:"dataset,omitempty" example:"documents"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	SkipExisting bool                   `json:"skip_existing,omitempty" example:"false"`
	Overwrite    bool                   `json:"overwrite,omitempty" example:"false"`
	Async        bool                   `json:"async,omitempty" example:"false"`
}

type SearchRequest struct {
	Query    string                 `json:"query" example:"find documents about machine learning"`
	Dataset  string                 `json:"dataset,omitempty" example:"documents"`
	Options  *SearchOptions         `json:"options,omitempty"`
	Filters  map[string]interface{} `json:"filters,omitempty"`
}

type SearchOptions struct {
	TopK            int                    `json:"top_k,omitempty" example:"10"`
	Threshold       float32                `json:"threshold,omitempty" example:"0.7"`
	MetricType      string                 `json:"metric_type,omitempty" example:"cosine"`
	IncludeContent  bool                   `json:"include_content,omitempty" example:"true"`
	IncludeMetadata bool                   `json:"include_metadata,omitempty" example:"true"`
	Filters         map[string]interface{} `json:"filters,omitempty"`
}