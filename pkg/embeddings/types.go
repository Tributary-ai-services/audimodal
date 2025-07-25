package embeddings

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// EmbeddingProvider defines the interface for different embedding providers
type EmbeddingProvider interface {
	// GenerateEmbedding creates an embedding vector for the given text
	GenerateEmbedding(ctx context.Context, text string) (*EmbeddingVector, error)
	
	// GenerateBatchEmbeddings creates embeddings for multiple texts efficiently
	GenerateBatchEmbeddings(ctx context.Context, texts []string) ([]*EmbeddingVector, error)
	
	// GetModelInfo returns information about the embedding model
	GetModelInfo() *ModelInfo
	
	// GetDimensions returns the dimensionality of the embeddings
	GetDimensions() int
	
	// GetMaxTokens returns the maximum number of tokens supported
	GetMaxTokens() int
	
	// GetProviderName returns the name of the provider
	GetProviderName() string
}

// VectorStore defines the interface for vector storage systems
type VectorStore interface {
	// CreateDataset creates a new dataset for storing vectors
	CreateDataset(ctx context.Context, config *DatasetConfig) error
	
	// InsertVectors inserts vectors into the specified dataset
	InsertVectors(ctx context.Context, datasetName string, vectors []*DocumentVector) error
	
	// SearchSimilar finds similar vectors based on query vector
	SearchSimilar(ctx context.Context, datasetName string, queryVector []float32, options *SearchOptions) (*SearchResult, error)
	
	// SearchByText finds similar vectors using text query (automatically embeds the text)
	SearchByText(ctx context.Context, datasetName string, queryText string, options *SearchOptions) (*SearchResult, error)
	
	// GetVector retrieves a specific vector by ID
	GetVector(ctx context.Context, datasetName string, vectorID string) (*DocumentVector, error)
	
	// DeleteVector removes a vector from the dataset
	DeleteVector(ctx context.Context, datasetName string, vectorID string) error
	
	// UpdateVector updates metadata for an existing vector
	UpdateVector(ctx context.Context, datasetName string, vectorID string, metadata map[string]interface{}) error
	
	// GetDatasetInfo returns information about a dataset
	GetDatasetInfo(ctx context.Context, datasetName string) (*DatasetInfo, error)
	
	// ListDatasets returns all available datasets
	ListDatasets(ctx context.Context) ([]*DatasetInfo, error)
	
	// Close closes the connection to the vector store
	Close() error
}

// EmbeddingService combines embedding generation and vector storage
type EmbeddingService interface {
	// ProcessDocument processes a document and stores its embeddings
	ProcessDocument(ctx context.Context, request *ProcessDocumentRequest) (*ProcessDocumentResponse, error)
	
	// ProcessChunks processes multiple text chunks and stores their embeddings
	ProcessChunks(ctx context.Context, request *ProcessChunksRequest) (*ProcessChunksResponse, error)
	
	// SearchDocuments finds similar documents based on text query
	SearchDocuments(ctx context.Context, request *SearchRequest) (*SearchResponse, error)
	
	// GetDocumentVectors retrieves all vectors for a specific document
	GetDocumentVectors(ctx context.Context, documentID string) ([]*DocumentVector, error)
	
	// DeleteDocument removes all vectors associated with a document
	DeleteDocument(ctx context.Context, documentID string) error
	
	// GetStats returns statistics about the embedding service
	GetStats(ctx context.Context) (*ServiceStats, error)
}

// EmbeddingVector represents a single embedding vector
type EmbeddingVector struct {
	Vector     []float32              `json:"vector"`
	Dimensions int                    `json:"dimensions"`
	Model      string                 `json:"model"`
	CreatedAt  time.Time              `json:"created_at"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// DocumentVector represents a document chunk with its embedding
type DocumentVector struct {
	ID           string                 `json:"id"`
	DocumentID   string                 `json:"document_id"`
	ChunkID      string                 `json:"chunk_id"`
	TenantID     uuid.UUID              `json:"tenant_id"`
	Content      string                 `json:"content"`
	Vector       []float32              `json:"vector"`
	ContentHash  string                 `json:"content_hash"`
	Metadata     map[string]interface{} `json:"metadata"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	
	// Content analysis metadata
	ContentType    string  `json:"content_type,omitempty"`
	Language       string  `json:"language,omitempty"`
	Sentiment      string  `json:"sentiment,omitempty"`
	Topics         []string `json:"topics,omitempty"`
	Keywords       []string `json:"keywords,omitempty"`
	ChunkIndex     int     `json:"chunk_index"`
	ChunkCount     int     `json:"chunk_count"`
	
	// Vector specific metadata
	Model          string  `json:"model"`
	Dimensions     int     `json:"dimensions"`
	EmbeddingTime  float64 `json:"embedding_time_ms"`
}

// ModelInfo contains information about an embedding model
type ModelInfo struct {
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	Dimensions   int    `json:"dimensions"`
	MaxTokens    int    `json:"max_tokens"`
	Description  string `json:"description"`
	Version      string `json:"version"`
	ContextSize  int    `json:"context_size"`
	CostPerToken float64 `json:"cost_per_token,omitempty"`
}

// DatasetConfig defines configuration for creating a dataset
type DatasetConfig struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Dimensions  int                    `json:"dimensions"`
	MetricType  string                 `json:"metric_type"` // cosine, euclidean, dot_product
	IndexType   string                 `json:"index_type"`  // hnsw, ivf, brute_force
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	
	// Deeplake specific options
	StorageLocation string `json:"storage_location,omitempty"` // local, s3, gcs, azure
	Credentials     string `json:"credentials,omitempty"`
	PublicRead      bool   `json:"public_read,omitempty"`
	Overwrite       bool   `json:"overwrite,omitempty"`
}

// DatasetInfo contains information about a dataset
type DatasetInfo struct {
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	Dimensions     int                    `json:"dimensions"`
	VectorCount    int64                  `json:"vector_count"`
	MetricType     string                 `json:"metric_type"`
	IndexType      string                 `json:"index_type"`
	StorageSize    int64                  `json:"storage_size_bytes"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	LastAccessedAt time.Time              `json:"last_accessed_at"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// SearchOptions defines options for vector similarity search
type SearchOptions struct {
	TopK           int                    `json:"top_k"`           // Number of results to return
	Threshold      float32                `json:"threshold"`       // Minimum similarity threshold
	MetricType     string                 `json:"metric_type"`     // cosine, euclidean, dot_product
	IncludeContent bool                   `json:"include_content"` // Include original content in results
	IncludeMetadata bool                  `json:"include_metadata"` // Include metadata in results
	Filters        map[string]interface{} `json:"filters,omitempty"` // Metadata filters
	
	// Advanced search options
	EfSearch       int     `json:"ef_search,omitempty"`       // HNSW search parameter
	Nprobe         int     `json:"nprobe,omitempty"`          // IVF search parameter
	MaxDistance    float32 `json:"max_distance,omitempty"`    // Maximum distance for results
	MinScore       float32 `json:"min_score,omitempty"`       // Minimum similarity score
	
	// Result processing
	Deduplicate    bool    `json:"deduplicate"`     // Remove duplicate results
	GroupByDoc     bool    `json:"group_by_doc"`    // Group results by document
	Rerank         bool    `json:"rerank"`          // Apply reranking to results
}

// SearchResult contains the results of a similarity search
type SearchResult struct {
	Results    []*SimilarityResult `json:"results"`
	Query      string              `json:"query,omitempty"`
	QueryTime  float64             `json:"query_time_ms"`
	TotalFound int                 `json:"total_found"`
	HasMore    bool                `json:"has_more"`
}

// SimilarityResult represents a single similarity search result
type SimilarityResult struct {
	DocumentVector *DocumentVector        `json:"document_vector"`
	Score          float32                `json:"score"`         // Similarity score (0-1)
	Distance       float32                `json:"distance"`      // Distance metric
	Rank           int                    `json:"rank"`          // Result ranking
	Explanation    map[string]interface{} `json:"explanation,omitempty"` // Debug info
}

// ProcessDocumentRequest contains parameters for processing a document
type ProcessDocumentRequest struct {
	DocumentID   string                 `json:"document_id"`
	TenantID     uuid.UUID              `json:"tenant_id"`
	Content      string                 `json:"content"`
	ContentType  string                 `json:"content_type"`
	ChunkSize    int                    `json:"chunk_size,omitempty"`
	ChunkOverlap int                    `json:"chunk_overlap,omitempty"`
	Dataset      string                 `json:"dataset"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	
	// Processing options
	SkipExisting bool `json:"skip_existing"` // Skip if document already processed
	Overwrite    bool `json:"overwrite"`     // Overwrite existing vectors
	Async        bool `json:"async"`         // Process asynchronously
}

// ProcessDocumentResponse contains the result of document processing
type ProcessDocumentResponse struct {
	DocumentID      string    `json:"document_id"`
	ChunksCreated   int       `json:"chunks_created"`
	VectorsCreated  int       `json:"vectors_created"`
	ProcessingTime  float64   `json:"processing_time_ms"`
	TotalTokens     int       `json:"total_tokens"`
	EstimatedCost   float64   `json:"estimated_cost,omitempty"`
	Status          string    `json:"status"`
	Message         string    `json:"message,omitempty"`
	Errors          []string  `json:"errors,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// ProcessChunksRequest contains parameters for processing text chunks
type ProcessChunksRequest struct {
	Chunks   []*ChunkInput `json:"chunks"`
	Dataset  string        `json:"dataset"`
	TenantID uuid.UUID     `json:"tenant_id"`
	
	// Processing options
	BatchSize    int  `json:"batch_size,omitempty"`
	SkipExisting bool `json:"skip_existing"`
	Overwrite    bool `json:"overwrite"`
	Async        bool `json:"async"`
}

// ChunkInput represents a text chunk to be processed
type ChunkInput struct {
	ID           string                 `json:"id"`
	DocumentID   string                 `json:"document_id"`
	Content      string                 `json:"content"`
	ChunkIndex   int                    `json:"chunk_index"`
	ContentType  string                 `json:"content_type,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ProcessChunksResponse contains the result of chunk processing
type ProcessChunksResponse struct {
	ChunksProcessed int       `json:"chunks_processed"`
	VectorsCreated  int       `json:"vectors_created"`
	ProcessingTime  float64   `json:"processing_time_ms"`
	TotalTokens     int       `json:"total_tokens"`
	EstimatedCost   float64   `json:"estimated_cost,omitempty"`
	Status          string    `json:"status"`
	Errors          []string  `json:"errors,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// SearchRequest contains parameters for semantic search
type SearchRequest struct {
	Query       string                 `json:"query"`
	Dataset     string                 `json:"dataset"`
	TenantID    uuid.UUID              `json:"tenant_id"`
	Options     *SearchOptions         `json:"options,omitempty"`
	Filters     map[string]interface{} `json:"filters,omitempty"`
	
	// Advanced search features
	HybridSearch   bool     `json:"hybrid_search,omitempty"`   // Combine vector and keyword search
	BoostFactors   map[string]float32 `json:"boost_factors,omitempty"` // Boost certain fields
	QueryExpansion bool     `json:"query_expansion,omitempty"` // Expand query with synonyms
}

// SearchResponse contains the result of a semantic search
type SearchResponse struct {
	Results      []*SimilarityResult `json:"results"`
	Query        string              `json:"query"`
	Dataset      string              `json:"dataset"`
	QueryTime    float64             `json:"query_time_ms"`
	TotalFound   int                 `json:"total_found"`
	HasMore      bool                `json:"has_more"`
	SearchStats  *SearchStats        `json:"search_stats,omitempty"`
}

// SearchStats contains statistics about a search operation
type SearchStats struct {
	VectorsScanned    int64   `json:"vectors_scanned"`
	IndexHits         int64   `json:"index_hits"`
	FilteredResults   int64   `json:"filtered_results"`
	RerankingTime     float64 `json:"reranking_time_ms,omitempty"`
	EmbeddingTime     float64 `json:"embedding_time_ms"`
	DatabaseTime      float64 `json:"database_time_ms"`
	PostProcessingTime float64 `json:"post_processing_time_ms"`
}

// ServiceStats contains statistics about the embedding service
type ServiceStats struct {
	TotalDocuments    int64     `json:"total_documents"`
	TotalVectors      int64     `json:"total_vectors"`
	TotalDatasets     int       `json:"total_datasets"`
	TotalSearches     int64     `json:"total_searches"`
	AvgSearchTime     float64   `json:"avg_search_time_ms"`
	AvgProcessingTime float64   `json:"avg_processing_time_ms"`
	StorageUsage      int64     `json:"storage_usage_bytes"`
	TokensProcessed   int64     `json:"tokens_processed"`
	EstimatedCost     float64   `json:"estimated_cost"`
	LastUpdated       time.Time `json:"last_updated"`
	
	// Provider-specific stats
	ProviderStats map[string]interface{} `json:"provider_stats,omitempty"`
}

// VectorMetrics contains metrics for monitoring vector operations
type VectorMetrics struct {
	// Operation counts
	EmbeddingsGenerated   int64 `json:"embeddings_generated"`
	VectorsInserted       int64 `json:"vectors_inserted"`
	SearchesPerformed     int64 `json:"searches_performed"`
	
	// Performance metrics
	AvgEmbeddingTime      float64 `json:"avg_embedding_time_ms"`
	AvgInsertTime         float64 `json:"avg_insert_time_ms"`
	AvgSearchTime         float64 `json:"avg_search_time_ms"`
	
	// Error rates
	EmbeddingErrors       int64 `json:"embedding_errors"`
	InsertErrors          int64 `json:"insert_errors"`
	SearchErrors          int64 `json:"search_errors"`
	
	// Resource usage
	MemoryUsage           int64 `json:"memory_usage_bytes"`
	DiskUsage            int64 `json:"disk_usage_bytes"`
	NetworkBandwidth     int64 `json:"network_bandwidth_bytes"`
	
	// Cost tracking
	TokensConsumed       int64   `json:"tokens_consumed"`
	EstimatedMonthlyCost float64 `json:"estimated_monthly_cost"`
}

// Configuration types

// EmbeddingConfig contains configuration for embedding providers
type EmbeddingConfig struct {
	Provider    string                 `yaml:"provider"`     // openai, huggingface, local
	Model       string                 `yaml:"model"`        // Model name/ID
	APIKey      string                 `yaml:"api_key"`      // API key for external providers
	BaseURL     string                 `yaml:"base_url"`     // Custom API endpoint
	Timeout     time.Duration          `yaml:"timeout"`      // Request timeout
	RetryCount  int                    `yaml:"retry_count"`  // Number of retries
	BatchSize   int                    `yaml:"batch_size"`   // Batch size for processing
	MaxTokens   int                    `yaml:"max_tokens"`   // Maximum tokens per request
	Dimensions  int                    `yaml:"dimensions"`   // Expected embedding dimensions
	Options     map[string]interface{} `yaml:"options"`      // Provider-specific options
}

// DeeplakeConfig contains configuration for Deeplake vector store
type DeeplakeConfig struct {
	StorageLocation string                 `yaml:"storage_location"` // Local path or cloud URL
	Token           string                 `yaml:"token"`            // Deeplake authentication token
	ReadOnly        bool                   `yaml:"read_only"`        // Read-only access
	MemoryCache     bool                   `yaml:"memory_cache"`     // Enable memory caching
	LocalCache      string                 `yaml:"local_cache"`      // Local cache directory
	Compression     string                 `yaml:"compression"`      // Compression algorithm
	ChunkSize       int                    `yaml:"chunk_size"`       // Chunk size for storage
	Timeout         time.Duration          `yaml:"timeout"`          // Operation timeout
	Retries         int                    `yaml:"retries"`          // Number of retries
	Options         map[string]interface{} `yaml:"options"`          // Additional options
}

// ServiceConfig contains configuration for the embedding service
type ServiceConfig struct {
	Embedding       *EmbeddingConfig `yaml:"embedding"`
	VectorStore     *DeeplakeConfig  `yaml:"vector_store"`
	DefaultDataset  string           `yaml:"default_dataset"`
	ChunkSize       int              `yaml:"chunk_size"`
	ChunkOverlap    int              `yaml:"chunk_overlap"`
	BatchSize       int              `yaml:"batch_size"`
	MaxConcurrency  int              `yaml:"max_concurrency"`
	CacheEnabled    bool             `yaml:"cache_enabled"`
	CacheTTL        time.Duration    `yaml:"cache_ttl"`
	MetricsEnabled  bool             `yaml:"metrics_enabled"`
	
	// Search defaults
	DefaultTopK     int     `yaml:"default_top_k"`
	DefaultThreshold float32 `yaml:"default_threshold"`
	DefaultMetric   string  `yaml:"default_metric"`
}

// Error types for the embeddings package
type EmbeddingError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

func (e *EmbeddingError) Error() string {
	return e.Message
}

// Common error types
var (
	ErrProviderNotSupported = &EmbeddingError{
		Type:    "provider_not_supported",
		Message: "embedding provider is not supported",
		Code:    "PROVIDER_NOT_SUPPORTED",
	}
	
	ErrInvalidModel = &EmbeddingError{
		Type:    "invalid_model",
		Message: "invalid or unsupported model",
		Code:    "INVALID_MODEL",
	}
	
	ErrDatasetNotFound = &EmbeddingError{
		Type:    "dataset_not_found",
		Message: "dataset not found",
		Code:    "DATASET_NOT_FOUND",
	}
	
	ErrVectorNotFound = &EmbeddingError{
		Type:    "vector_not_found",
		Message: "vector not found",
		Code:    "VECTOR_NOT_FOUND",
	}
	
	ErrInvalidDimensions = &EmbeddingError{
		Type:    "invalid_dimensions",
		Message: "vector dimensions do not match expected dimensions",
		Code:    "INVALID_DIMENSIONS",
	}
	
	ErrQuotaExceeded = &EmbeddingError{
		Type:    "quota_exceeded",
		Message: "API quota or rate limit exceeded",
		Code:    "QUOTA_EXCEEDED",
	}
	
	ErrInvalidConfiguration = &EmbeddingError{
		Type:    "invalid_configuration",
		Message: "invalid configuration provided",
		Code:    "INVALID_CONFIGURATION",
	}
)