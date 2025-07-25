package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/jscharber/eAIIngest/pkg/embeddings"
)

// DeepLakeAPIClient implements the VectorStore interface using the DeepLake API
type DeepLakeAPIClient struct {
	config     *DeepLakeAPIConfig
	httpClient *http.Client
	baseURL    string
}

// DeepLakeAPIConfig holds configuration for the DeepLake API client
type DeepLakeAPIConfig struct {
	BaseURL    string        `json:"base_url"`
	APIKey     string        `json:"api_key"`
	TenantID   string        `json:"tenant_id,omitempty"`
	Timeout    time.Duration `json:"timeout"`
	Retries    int           `json:"retries"`
	UserAgent  string        `json:"user_agent,omitempty"`
}

// NewDeepLakeAPIClient creates a new DeepLake API client
func NewDeepLakeAPIClient(config *DeepLakeAPIConfig) (*DeepLakeAPIClient, error) {
	if err := validateAPIConfig(config); err != nil {
		return nil, err
	}

	client := &DeepLakeAPIClient{
		config:  config,
		baseURL: config.BaseURL,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}

	return client, nil
}

// validateAPIConfig validates the API configuration
func validateAPIConfig(config *DeepLakeAPIConfig) error {
	if config.BaseURL == "" {
		return &embeddings.EmbeddingError{
			Type:    "invalid_configuration",
			Message: "base URL is required",
			Code:    "MISSING_BASE_URL",
		}
	}

	if config.APIKey == "" {
		return &embeddings.EmbeddingError{
			Type:    "invalid_configuration",
			Message: "API key is required",
			Code:    "MISSING_API_KEY",
		}
	}

	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}

	if config.Retries < 0 {
		config.Retries = 3
	}

	if config.UserAgent == "" {
		config.UserAgent = "eAIIngest-Go-Client/1.0"
	}

	return nil
}

// API Models - matching the OpenAPI specification

// DatasetCreateRequest represents a dataset creation request
type DatasetCreateRequest struct {
	Name            string            `json:"name"`
	Description     string            `json:"description,omitempty"`
	Dimensions      int               `json:"dimensions"`
	MetricType      string            `json:"metric_type,omitempty"`
	IndexType       string            `json:"index_type,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	StorageLocation string            `json:"storage_location,omitempty"`
	Overwrite       bool              `json:"overwrite,omitempty"`
}

// DatasetResponse represents a dataset response
type DatasetResponse struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Description     string                 `json:"description,omitempty"`
	Dimensions      int                    `json:"dimensions"`
	MetricType      string                 `json:"metric_type"`
	IndexType       string                 `json:"index_type"`
	Metadata        map[string]interface{} `json:"metadata"`
	StorageLocation string                 `json:"storage_location"`
	VectorCount     int                    `json:"vector_count"`
	StorageSize     int64                  `json:"storage_size"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	TenantID        string                 `json:"tenant_id,omitempty"`
}

// VectorCreateRequest represents a vector creation request
type VectorCreateRequest struct {
	ID          string                 `json:"id,omitempty"`
	DocumentID  string                 `json:"document_id"`
	ChunkID     string                 `json:"chunk_id,omitempty"`
	Values      []float32              `json:"values"`
	Content     string                 `json:"content,omitempty"`
	ContentHash string                 `json:"content_hash,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	ContentType string                 `json:"content_type,omitempty"`
	Language    string                 `json:"language,omitempty"`
	ChunkIndex  *int                   `json:"chunk_index,omitempty"`
	ChunkCount  *int                   `json:"chunk_count,omitempty"`
	Model       string                 `json:"model,omitempty"`
}

// VectorBatchInsertRequest represents a batch vector insertion request
type VectorBatchInsertRequest struct {
	Vectors       []VectorCreateRequest `json:"vectors"`
	SkipExisting  bool                  `json:"skip_existing,omitempty"`
	Overwrite     bool                  `json:"overwrite,omitempty"`
	BatchSize     *int                  `json:"batch_size,omitempty"`
}

// VectorResponse represents a vector response
type VectorResponse struct {
	ID          string                 `json:"id"`
	DatasetID   string                 `json:"dataset_id"`
	DocumentID  string                 `json:"document_id"`
	ChunkID     string                 `json:"chunk_id,omitempty"`
	Values      []float32              `json:"values"`
	Content     string                 `json:"content,omitempty"`
	ContentHash string                 `json:"content_hash,omitempty"`
	Metadata    map[string]interface{} `json:"metadata"`
	ContentType string                 `json:"content_type,omitempty"`
	Language    string                 `json:"language,omitempty"`
	ChunkIndex  *int                   `json:"chunk_index,omitempty"`
	ChunkCount  *int                   `json:"chunk_count,omitempty"`
	Model       string                 `json:"model,omitempty"`
	Dimensions  int                    `json:"dimensions"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	TenantID    string                 `json:"tenant_id,omitempty"`
}

// SearchRequest represents a vector search request
type SearchRequest struct {
	QueryVector []float32      `json:"query_vector"`
	Options     *SearchOptions `json:"options,omitempty"`
}

// TextSearchRequest represents a text-based search request
type TextSearchRequest struct {
	QueryText string         `json:"query_text"`
	Options   *SearchOptions `json:"options,omitempty"`
}

// SearchOptions represents search options
type SearchOptions struct {
	TopK            int                    `json:"top_k,omitempty"`
	Threshold       *float32               `json:"threshold,omitempty"`
	MetricType      string                 `json:"metric_type,omitempty"`
	IncludeContent  bool                   `json:"include_content,omitempty"`
	IncludeMetadata bool                   `json:"include_metadata,omitempty"`
	Filters         map[string]interface{} `json:"filters,omitempty"`
	Deduplicate     bool                   `json:"deduplicate,omitempty"`
	GroupByDocument bool                   `json:"group_by_document,omitempty"`
	Rerank          bool                   `json:"rerank,omitempty"`
	EfSearch        *int                   `json:"ef_search,omitempty"`
	Nprobe          *int                   `json:"nprobe,omitempty"`
	MaxDistance     *float32               `json:"max_distance,omitempty"`
	MinScore        *float32               `json:"min_score,omitempty"`
}

// SearchResponse represents a search response
type SearchResponse struct {
	Results          []SearchResultItem `json:"results"`
	TotalFound       int                `json:"total_found"`
	HasMore          bool               `json:"has_more"`
	QueryTimeMs      float64            `json:"query_time_ms"`
	EmbeddingTimeMs  float64            `json:"embedding_time_ms"`
	Stats            SearchStats        `json:"stats"`
}

// SearchResultItem represents a single search result
type SearchResultItem struct {
	Vector      VectorResponse         `json:"vector"`
	Score       float32                `json:"score"`
	Distance    float32                `json:"distance"`
	Rank        int                    `json:"rank"`
	Explanation map[string]string      `json:"explanation,omitempty"`
}

// SearchStats represents search statistics
type SearchStats struct {
	VectorsScanned       int     `json:"vectors_scanned"`
	IndexHits            int     `json:"index_hits"`
	FilteredResults      int     `json:"filtered_results"`
	RerankingTimeMs      float64 `json:"reranking_time_ms"`
	DatabaseTimeMs       float64 `json:"database_time_ms"`
	PostProcessingTimeMs float64 `json:"post_processing_time_ms"`
}

// DatasetStats represents dataset statistics
type DatasetStats struct {
	Dataset       DatasetResponse            `json:"dataset"`
	VectorCount   int                        `json:"vector_count"`
	StorageSize   int64                      `json:"storage_size"`
	MetadataStats map[string]int             `json:"metadata_stats"`
	IndexStats    map[string]interface{}     `json:"index_stats,omitempty"`
}

// VectorBatchResponse represents a batch operation response
type VectorBatchResponse struct {
	InsertedCount    int      `json:"inserted_count"`
	SkippedCount     int      `json:"skipped_count"`
	FailedCount      int      `json:"failed_count"`
	ErrorMessages    []string `json:"error_messages"`
	ProcessingTimeMs float64  `json:"processing_time_ms"`
}

// Implementation of VectorStore interface methods

// CreateDataset creates a new dataset
func (c *DeepLakeAPIClient) CreateDataset(ctx context.Context, config *embeddings.DatasetConfig) error {
	request := DatasetCreateRequest{
		Name:        config.Name,
		Description: config.Description,
		Dimensions:  config.Dimensions,
		MetricType:  config.MetricType,
		IndexType:   config.IndexType,
		Metadata:    convertMetadata(config.Metadata),
		Overwrite:   config.Overwrite,
	}

	err := c.makeRequest(ctx, "POST", "/api/v1/datasets/", request, nil)
	return err
}

// InsertVectors inserts vectors into a dataset
func (c *DeepLakeAPIClient) InsertVectors(ctx context.Context, datasetName string, vectors []*embeddings.DocumentVector) error {
	// First, get the dataset ID from the name
	datasetID, err := c.getDatasetIDByName(ctx, datasetName)
	if err != nil {
		return err
	}

	// Convert vectors to API format
	apiVectors := make([]VectorCreateRequest, len(vectors))
	for i, v := range vectors {
		apiVectors[i] = VectorCreateRequest{
			ID:          v.ID,
			DocumentID:  v.DocumentID,
			ChunkID:     v.ChunkID,
			Values:      v.Vector,
			Content:     v.Content,
			ContentHash: v.ContentHash,
			Metadata:    v.Metadata,
			ContentType: v.ContentType,
			Language:    v.Language,
			ChunkIndex:  getIntPtr(v.ChunkIndex),
			ChunkCount:  getIntPtr(v.ChunkCount),
		}
	}

	request := VectorBatchInsertRequest{
		Vectors: apiVectors,
	}

	endpoint := fmt.Sprintf("/api/v1/datasets/%s/vectors/batch", datasetID)
	err = c.makeRequest(ctx, "POST", endpoint, request, nil)
	return err
}

// SearchSimilar performs vector similarity search
func (c *DeepLakeAPIClient) SearchSimilar(ctx context.Context, datasetName string, queryVector []float32, options *embeddings.SearchOptions) (*embeddings.SearchResult, error) {
	// Get dataset ID from name
	datasetID, err := c.getDatasetIDByName(ctx, datasetName)
	if err != nil {
		return nil, err
	}

	// Convert search options
	apiOptions := c.convertSearchOptions(options)

	request := SearchRequest{
		QueryVector: queryVector,
		Options:     apiOptions,
	}

	var response SearchResponse
	endpoint := fmt.Sprintf("/api/v1/datasets/%s/search", datasetID)
	err = c.makeRequest(ctx, "POST", endpoint, request, &response)
	if err != nil {
		return nil, err
	}

	return c.convertSearchResponse(&response), nil
}

// SearchByText performs text-based search
func (c *DeepLakeAPIClient) SearchByText(ctx context.Context, datasetName string, queryText string, options *embeddings.SearchOptions) (*embeddings.SearchResult, error) {
	// Get dataset ID from name
	datasetID, err := c.getDatasetIDByName(ctx, datasetName)
	if err != nil {
		return nil, err
	}

	// Convert search options
	apiOptions := c.convertSearchOptions(options)

	request := TextSearchRequest{
		QueryText: queryText,
		Options:   apiOptions,
	}

	var response SearchResponse
	endpoint := fmt.Sprintf("/api/v1/datasets/%s/search/text", datasetID)
	err = c.makeRequest(ctx, "POST", endpoint, request, &response)
	if err != nil {
		return nil, err
	}

	return c.convertSearchResponse(&response), nil
}

// GetVector retrieves a specific vector
func (c *DeepLakeAPIClient) GetVector(ctx context.Context, datasetName string, vectorID string) (*embeddings.DocumentVector, error) {
	datasetID, err := c.getDatasetIDByName(ctx, datasetName)
	if err != nil {
		return nil, err
	}

	var response VectorResponse
	endpoint := fmt.Sprintf("/api/v1/datasets/%s/vectors/%s", datasetID, vectorID)
	err = c.makeRequest(ctx, "GET", endpoint, nil, &response)
	if err != nil {
		return nil, err
	}

	return c.convertVectorResponse(&response), nil
}

// DeleteVector removes a vector
func (c *DeepLakeAPIClient) DeleteVector(ctx context.Context, datasetName string, vectorID string) error {
	datasetID, err := c.getDatasetIDByName(ctx, datasetName)
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("/api/v1/datasets/%s/vectors/%s", datasetID, vectorID)
	err = c.makeRequest(ctx, "DELETE", endpoint, nil, nil)
	return err
}

// UpdateVector updates vector metadata
func (c *DeepLakeAPIClient) UpdateVector(ctx context.Context, datasetName string, vectorID string, metadata map[string]interface{}) error {
	datasetID, err := c.getDatasetIDByName(ctx, datasetName)
	if err != nil {
		return err
	}

	request := map[string]interface{}{
		"metadata": metadata,
	}

	endpoint := fmt.Sprintf("/api/v1/datasets/%s/vectors/%s", datasetID, vectorID)
	err = c.makeRequest(ctx, "PUT", endpoint, request, nil)
	return err
}

// GetDatasetInfo returns dataset information
func (c *DeepLakeAPIClient) GetDatasetInfo(ctx context.Context, datasetName string) (*embeddings.DatasetInfo, error) {
	datasetID, err := c.getDatasetIDByName(ctx, datasetName)
	if err != nil {
		return nil, err
	}

	var response DatasetResponse
	endpoint := fmt.Sprintf("/api/v1/datasets/%s", datasetID)
	err = c.makeRequest(ctx, "GET", endpoint, nil, &response)
	if err != nil {
		return nil, err
	}

	return c.convertDatasetResponse(&response), nil
}

// ListDatasets returns all datasets
func (c *DeepLakeAPIClient) ListDatasets(ctx context.Context) ([]*embeddings.DatasetInfo, error) {
	var response []DatasetResponse
	err := c.makeRequest(ctx, "GET", "/api/v1/datasets/", nil, &response)
	if err != nil {
		return nil, err
	}

	datasets := make([]*embeddings.DatasetInfo, len(response))
	for i, dataset := range response {
		datasets[i] = c.convertDatasetResponse(&dataset)
	}

	return datasets, nil
}

// GetDatasetStats returns dataset statistics
func (c *DeepLakeAPIClient) GetDatasetStats(ctx context.Context, datasetName string) (map[string]interface{}, error) {
	datasetID, err := c.getDatasetIDByName(ctx, datasetName)
	if err != nil {
		return nil, err
	}

	var response DatasetStats
	endpoint := fmt.Sprintf("/api/v1/datasets/%s/stats", datasetID)
	err = c.makeRequest(ctx, "GET", endpoint, nil, &response)
	if err != nil {
		return nil, err
	}

	// Convert to map format expected by the interface
	stats := map[string]interface{}{
		"name":          response.Dataset.Name,
		"vector_count":  response.VectorCount,
		"dimensions":    response.Dataset.Dimensions,
		"storage_size":  response.StorageSize,
		"metric_type":   response.Dataset.MetricType,
		"index_type":    response.Dataset.IndexType,
		"created_at":    response.Dataset.CreatedAt,
		"updated_at":    response.Dataset.UpdatedAt,
		"metadata_stats": response.MetadataStats,
		"index_stats":   response.IndexStats,
	}

	return stats, nil
}

// Close closes the client connection
func (c *DeepLakeAPIClient) Close() error {
	// No persistent connections to close for HTTP client
	return nil
}

// Helper methods

// getDatasetIDByName retrieves dataset ID by name
func (c *DeepLakeAPIClient) getDatasetIDByName(ctx context.Context, name string) (string, error) {
	var datasets []DatasetResponse
	err := c.makeRequest(ctx, "GET", "/api/v1/datasets/", nil, &datasets)
	if err != nil {
		return "", err
	}

	for _, dataset := range datasets {
		if dataset.Name == name {
			return dataset.ID, nil
		}
	}

	return "", &embeddings.EmbeddingError{
		Type:    "dataset_not_found",
		Message: fmt.Sprintf("dataset '%s' not found", name),
		Code:    "DATASET_NOT_FOUND",
	}
}

// makeRequest makes an HTTP request to the API
func (c *DeepLakeAPIClient) makeRequest(ctx context.Context, method, endpoint string, body interface{}, result interface{}) error {
	// Build URL
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return err
	}
	u.Path = path.Join(u.Path, endpoint)

	// Prepare body
	var bodyReader io.Reader
	if body != nil {
		bodyData, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(bodyData)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("ApiKey %s", c.config.APIKey))
	req.Header.Set("User-Agent", c.config.UserAgent)

	if c.config.TenantID != "" {
		req.Header.Set("X-Tenant-ID", c.config.TenantID)
	}

	// Make request with retries
	var resp *http.Response
	for i := 0; i <= c.config.Retries; i++ {
		resp, err = c.httpClient.Do(req)
		if err == nil && resp.StatusCode < 500 {
			break
		}
		if i < c.config.Retries {
			time.Sleep(time.Duration(i+1) * time.Second)
		}
	}

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Check status code
	if resp.StatusCode >= 400 {
		return c.handleErrorResponse(resp.StatusCode, respBody)
	}

	// Parse response
	if result != nil {
		return json.Unmarshal(respBody, result)
	}

	return nil
}

// handleErrorResponse handles API error responses
func (c *DeepLakeAPIClient) handleErrorResponse(statusCode int, body []byte) error {
	var errorResp map[string]interface{}
	if err := json.Unmarshal(body, &errorResp); err != nil {
		return &embeddings.EmbeddingError{
			Type:    "api_error",
			Message: fmt.Sprintf("HTTP %d: %s", statusCode, string(body)),
			Code:    fmt.Sprintf("HTTP_%d", statusCode),
		}
	}

	// Extract error details if available
	errorType := "api_error"
	message := fmt.Sprintf("HTTP %d", statusCode)
	code := fmt.Sprintf("HTTP_%d", statusCode)

	if detail, ok := errorResp["detail"]; ok {
		if detailStr, ok := detail.(string); ok {
			message = detailStr
		}
	}

	return &embeddings.EmbeddingError{
		Type:    errorType,
		Message: message,
		Code:    code,
		Details: errorResp,
	}
}

// Conversion helper methods

func convertMetadata(metadata map[string]interface{}) map[string]string {
	if metadata == nil {
		return nil
	}
	
	result := make(map[string]string)
	for k, v := range metadata {
		result[k] = fmt.Sprintf("%v", v)
	}
	return result
}

func (c *DeepLakeAPIClient) convertSearchOptions(options *embeddings.SearchOptions) *SearchOptions {
	if options == nil {
		return nil
	}

	return &SearchOptions{
		TopK:            options.TopK,
		Threshold:       &options.Threshold,
		MetricType:      options.MetricType,
		IncludeContent:  options.IncludeContent,
		IncludeMetadata: options.IncludeMetadata,
		Filters:         options.Filters,
	}
}

func (c *DeepLakeAPIClient) convertSearchResponse(response *SearchResponse) *embeddings.SearchResult {
	results := make([]*embeddings.SimilarityResult, len(response.Results))
	for i, item := range response.Results {
		results[i] = &embeddings.SimilarityResult{
			DocumentVector: c.convertVectorResponse(&item.Vector),
			Score:          item.Score,
			Distance:       item.Distance,
			Rank:           item.Rank,
		}
	}

	return &embeddings.SearchResult{
		Results:    results,
		QueryTime:  response.QueryTimeMs,
		TotalFound: response.TotalFound,
		HasMore:    response.HasMore,
	}
}

func (c *DeepLakeAPIClient) convertVectorResponse(response *VectorResponse) *embeddings.DocumentVector {
	return &embeddings.DocumentVector{
		ID:          response.ID,
		DocumentID:  response.DocumentID,
		ChunkID:     response.ChunkID,
		Vector:      response.Values,
		Content:     response.Content,
		ContentHash: response.ContentHash,
		Metadata:    response.Metadata,
		ContentType: response.ContentType,
		Language:    response.Language,
		ChunkIndex:  getIntValue(response.ChunkIndex),
		ChunkCount:  getIntValue(response.ChunkCount),
		CreatedAt:   response.CreatedAt,
		UpdatedAt:   response.UpdatedAt,
	}
}

func (c *DeepLakeAPIClient) convertDatasetResponse(response *DatasetResponse) *embeddings.DatasetInfo {
	return &embeddings.DatasetInfo{
		Name:           response.Name,
		Description:    response.Description,
		Dimensions:     response.Dimensions,
		VectorCount:    int64(response.VectorCount),
		MetricType:     response.MetricType,
		IndexType:      response.IndexType,
		StorageSize:    response.StorageSize,
		CreatedAt:      response.CreatedAt,
		UpdatedAt:      response.UpdatedAt,
		LastAccessedAt: response.UpdatedAt, // Use UpdatedAt as approximation
		Metadata:       response.Metadata,
	}
}

// Helper functions for pointer conversions

// getIntPtr converts int to *int
func getIntPtr(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

// getIntValue converts *int to int
func getIntValue(ptr *int) int {
	if ptr == nil {
		return 0
	}
	return *ptr
}