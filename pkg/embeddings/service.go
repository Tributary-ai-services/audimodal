package embeddings

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/chunking"
)

const (
	// NanosecondsToMilliseconds converts nanoseconds to milliseconds
	NanosecondsToMilliseconds = 1e6
	// AverageWeightFactor for calculating running averages
	AverageWeightFactor = 2
)

// embeddingService implements the EmbeddingService interface
type embeddingService struct {
	provider    EmbeddingProvider
	vectorStore VectorStore
	config      *ServiceConfig
	stats       *ServiceStats
}

// NewEmbeddingService creates a new embedding service
func NewEmbeddingService(provider EmbeddingProvider, vectorStore VectorStore, config *ServiceConfig) EmbeddingService {
	if config == nil {
		config = &ServiceConfig{
			DefaultDataset:   "default",
			ChunkSize:        1000,
			ChunkOverlap:     100,
			BatchSize:        10,
			MaxConcurrency:   5,
			CacheEnabled:     true,
			CacheTTL:         24 * time.Hour,
			MetricsEnabled:   true,
			DefaultTopK:      10,
			DefaultThreshold: 0.7,
			DefaultMetric:    "cosine",
		}
	}

	return &embeddingService{
		provider:    provider,
		vectorStore: vectorStore,
		config:      config,
		stats: &ServiceStats{
			LastUpdated:   time.Now(),
			ProviderStats: make(map[string]interface{}),
		},
	}
}

// ProcessDocument processes a document and stores its embeddings
func (s *embeddingService) ProcessDocument(ctx context.Context, request *ProcessDocumentRequest) (*ProcessDocumentResponse, error) {
	startTime := time.Now()

	// Validate request
	if err := s.validateProcessDocumentRequest(request); err != nil {
		return nil, err
	}

	// Ensure dataset exists
	if err := s.ensureDatasetExists(ctx, request.Dataset); err != nil {
		return nil, err
	}

	// Check if document already exists and handle accordingly
	if request.SkipExisting {
		exists, err := s.documentExists(ctx, request.Dataset, request.DocumentID)
		if err != nil {
			return nil, err
		}
		if exists && !request.Overwrite {
			return &ProcessDocumentResponse{
				DocumentID: request.DocumentID,
				Status:     "skipped",
				Message:    "document already exists",
				CreatedAt:  time.Now(),
			}, nil
		}
	}

	// Delete existing document if overwriting
	if request.Overwrite {
		if err := s.DeleteDocument(ctx, request.DocumentID); err != nil {
			log.Printf("Warning: failed to delete existing document %s: %v", request.DocumentID, err)
		}
	}

	// Create text chunks
	chunks, err := s.chunkDocument(request.Content, request.ChunkSize, request.ChunkOverlap)
	if err != nil {
		return nil, &EmbeddingError{
			Type:    "chunking_error",
			Message: fmt.Sprintf("failed to chunk document: %v", err),
			Code:    "CHUNKING_ERROR",
		}
	}

	// Prepare chunk inputs
	chunkInputs := make([]*ChunkInput, len(chunks))
	for i, chunk := range chunks {
		chunkInputs[i] = &ChunkInput{
			ID:          fmt.Sprintf("%s-chunk-%d", request.DocumentID, i),
			DocumentID:  request.DocumentID,
			Content:     chunk,
			ChunkIndex:  i,
			ContentType: request.ContentType,
			Metadata:    request.Metadata,
		}
	}

	// Process chunks
	processChunksRequest := &ProcessChunksRequest{
		Chunks:    chunkInputs,
		Dataset:   request.Dataset,
		TenantID:  request.TenantID,
		BatchSize: s.config.BatchSize,
		Async:     request.Async,
	}

	chunksResponse, err := s.ProcessChunks(ctx, processChunksRequest)
	if err != nil {
		return nil, err
	}

	processingTime := float64(time.Since(startTime).Nanoseconds()) / NanosecondsToMilliseconds

	// Update stats
	s.updateProcessingStats(len(chunks), chunksResponse.VectorsCreated, processingTime)

	return &ProcessDocumentResponse{
		DocumentID:     request.DocumentID,
		ChunksCreated:  len(chunks),
		VectorsCreated: chunksResponse.VectorsCreated,
		ProcessingTime: processingTime,
		TotalTokens:    chunksResponse.TotalTokens,
		EstimatedCost:  chunksResponse.EstimatedCost,
		Status:         "completed",
		CreatedAt:      time.Now(),
	}, nil
}

// ProcessChunks processes multiple text chunks and stores their embeddings
func (s *embeddingService) ProcessChunks(ctx context.Context, request *ProcessChunksRequest) (*ProcessChunksResponse, error) {
	startTime := time.Now()

	// Validate request
	if err := s.validateProcessChunksRequest(request); err != nil {
		return nil, err
	}

	// Ensure dataset exists
	if err := s.ensureDatasetExists(ctx, request.Dataset); err != nil {
		return nil, err
	}

	var totalTokens int
	var totalCost float64
	var errors []string
	vectorsCreated := 0

	// Process chunks in batches
	batchSize := request.BatchSize
	if batchSize == 0 {
		batchSize = s.config.BatchSize
	}

	for i := 0; i < len(request.Chunks); i += batchSize {
		end := i + batchSize
		if end > len(request.Chunks) {
			end = len(request.Chunks)
		}

		batch := request.Chunks[i:end]
		texts := make([]string, len(batch))
		for j, chunk := range batch {
			texts[j] = chunk.Content
		}

		// Generate embeddings for batch
		embeddings, err := s.provider.GenerateBatchEmbeddings(ctx, texts)
		if err != nil {
			errorMsg := fmt.Sprintf("failed to generate embeddings for batch %d: %v", i/batchSize, err)
			errors = append(errors, errorMsg)
			continue
		}

		// Create document vectors
		documentVectors := make([]*DocumentVector, 0, len(batch))
		for j, chunk := range batch {
			if j >= len(embeddings) || embeddings[j] == nil {
				errors = append(errors, fmt.Sprintf("missing embedding for chunk %s", chunk.ID))
				continue
			}

			embedding := embeddings[j]
			vector := &DocumentVector{
				ID:          chunk.ID,
				DocumentID:  chunk.DocumentID,
				ChunkID:     chunk.ID,
				TenantID:    request.TenantID,
				Content:     chunk.Content,
				Vector:      embedding.Vector,
				Metadata:    chunk.Metadata,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				ContentType: chunk.ContentType,
				ChunkIndex:  chunk.ChunkIndex,
				ChunkCount:  len(request.Chunks),
				Model:       embedding.Model,
				Dimensions:  embedding.Dimensions,
			}

			// Add embedding metadata
			if vector.Metadata == nil {
				vector.Metadata = make(map[string]interface{})
			}
			for key, value := range embedding.Metadata {
				vector.Metadata[key] = value
			}

			documentVectors = append(documentVectors, vector)

			// Update token count and cost
			if tokensUsed, ok := embedding.Metadata["tokens_used"].(int); ok {
				totalTokens += tokensUsed
			}
		}

		// Insert vectors into vector store
		if len(documentVectors) > 0 {
			if err := s.vectorStore.InsertVectors(ctx, request.Dataset, documentVectors); err != nil {
				errorMsg := fmt.Sprintf("failed to insert vectors for batch %d: %v", i/batchSize, err)
				errors = append(errors, errorMsg)
				continue
			}
			vectorsCreated += len(documentVectors)
		}
	}

	// Estimate cost based on tokens
	if totalTokens > 0 {
		modelInfo := s.provider.GetModelInfo()
		totalCost = float64(totalTokens) * modelInfo.CostPerToken
	}

	processingTime := float64(time.Since(startTime).Nanoseconds()) / NanosecondsToMilliseconds
	status := "completed"
	if len(errors) > 0 {
		if vectorsCreated == 0 {
			status = "failed"
		} else {
			status = "partial"
		}
	}

	// Update stats
	s.updateProcessingStats(len(request.Chunks), vectorsCreated, processingTime)

	return &ProcessChunksResponse{
		ChunksProcessed: len(request.Chunks),
		VectorsCreated:  vectorsCreated,
		ProcessingTime:  processingTime,
		TotalTokens:     totalTokens,
		EstimatedCost:   totalCost,
		Status:          status,
		Errors:          errors,
		CreatedAt:       time.Now(),
	}, nil
}

// SearchDocuments finds similar documents based on text query
func (s *embeddingService) SearchDocuments(ctx context.Context, request *SearchRequest) (*SearchResponse, error) {
	startTime := time.Now()

	// Validate request
	if err := s.validateSearchRequest(request); err != nil {
		return nil, err
	}

	// Generate embedding for query
	queryEmbedding, err := s.provider.GenerateEmbedding(ctx, request.Query)
	if err != nil {
		return nil, &EmbeddingError{
			Type:    "embedding_error",
			Message: fmt.Sprintf("failed to generate query embedding: %v", err),
			Code:    "QUERY_EMBEDDING_ERROR",
		}
	}

	embeddingTime := float64(time.Since(startTime).Nanoseconds()) / NanosecondsToMilliseconds

	// Prepare search options
	searchOptions := request.Options
	if searchOptions == nil {
		searchOptions = &SearchOptions{
			TopK:            s.config.DefaultTopK,
			Threshold:       s.config.DefaultThreshold,
			MetricType:      s.config.DefaultMetric,
			IncludeContent:  true,
			IncludeMetadata: true,
		}
	}

	// Add tenant filter
	if searchOptions.Filters == nil {
		searchOptions.Filters = make(map[string]interface{})
	}
	searchOptions.Filters["tenant_id"] = request.TenantID.String()

	// Merge additional filters
	for key, value := range request.Filters {
		searchOptions.Filters[key] = value
	}

	// Perform vector search
	searchStartTime := time.Now()
	searchResult, err := s.vectorStore.SearchSimilar(ctx, request.Dataset, queryEmbedding.Vector, searchOptions)
	if err != nil {
		return nil, err
	}
	databaseTime := float64(time.Since(searchStartTime).Nanoseconds()) / NanosecondsToMilliseconds

	// Apply post-processing if needed
	results := searchResult.Results
	if searchOptions.GroupByDoc {
		results = s.groupResultsByDocument(results)
	}
	if searchOptions.Deduplicate {
		results = s.deduplicateResults(results)
	}

	queryTime := float64(time.Since(startTime).Nanoseconds()) / NanosecondsToMilliseconds

	// Update search stats
	s.updateSearchStats(len(results), queryTime)

	// Create search stats
	searchStats := &SearchStats{
		VectorsScanned:     int64(len(searchResult.Results)),
		FilteredResults:    int64(len(results)),
		EmbeddingTime:      embeddingTime,
		DatabaseTime:       databaseTime,
		PostProcessingTime: queryTime - embeddingTime - databaseTime,
	}

	return &SearchResponse{
		Results:     results,
		Query:       request.Query,
		Dataset:     request.Dataset,
		QueryTime:   queryTime,
		TotalFound:  len(results),
		HasMore:     searchResult.HasMore,
		SearchStats: searchStats,
	}, nil
}

// GetDocumentVectors retrieves all vectors for a specific document
func (s *embeddingService) GetDocumentVectors(ctx context.Context, documentID string) ([]*DocumentVector, error) {
	// This would require a method to search by document ID in the vector store
	// For now, we'll implement a basic search by metadata filter
	datasets, err := s.vectorStore.ListDatasets(ctx)
	if err != nil {
		return nil, err
	}

	var allVectors []*DocumentVector
	for _, dataset := range datasets {
		searchOptions := &SearchOptions{
			TopK:            10000, // Large number to get all results
			IncludeContent:  true,
			IncludeMetadata: true,
			Filters: map[string]interface{}{
				"document_id": documentID,
			},
		}

		// Use a dummy query vector (this is not ideal, but works for metadata filtering)
		dummyVector := make([]float32, s.provider.GetDimensions())
		searchResult, err := s.vectorStore.SearchSimilar(ctx, dataset.Name, dummyVector, searchOptions)
		if err != nil {
			continue // Skip errors for individual datasets
		}

		for _, result := range searchResult.Results {
			if result.DocumentVector.DocumentID == documentID {
				allVectors = append(allVectors, result.DocumentVector)
			}
		}
	}

	return allVectors, nil
}

// DeleteDocument removes all vectors associated with a document
func (s *embeddingService) DeleteDocument(ctx context.Context, documentID string) error {
	vectors, err := s.GetDocumentVectors(ctx, documentID)
	if err != nil {
		return err
	}

	// Group vectors by dataset
	vectorsByDataset := make(map[string][]string)
	for _, vector := range vectors {
		// We need to determine which dataset the vector belongs to
		// For now, we'll try all datasets
		datasets, err := s.vectorStore.ListDatasets(ctx)
		if err != nil {
			continue
		}

		for _, dataset := range datasets {
			vectorsByDataset[dataset.Name] = append(vectorsByDataset[dataset.Name], vector.ID)
		}
	}

	// Delete vectors from each dataset
	var errors []string
	for datasetName, vectorIDs := range vectorsByDataset {
		for _, vectorID := range vectorIDs {
			if err := s.vectorStore.DeleteVector(ctx, datasetName, vectorID); err != nil {
				errors = append(errors, fmt.Sprintf("failed to delete vector %s from dataset %s: %v", vectorID, datasetName, err))
			}
		}
	}

	if len(errors) > 0 {
		return &EmbeddingError{
			Type:    "deletion_error",
			Message: fmt.Sprintf("errors occurred during deletion: %s", strings.Join(errors, "; ")),
			Code:    "DELETION_ERROR",
		}
	}

	return nil
}

// GetStats returns statistics about the embedding service
func (s *embeddingService) GetStats(ctx context.Context) (*ServiceStats, error) {
	// Update dataset counts
	datasets, err := s.vectorStore.ListDatasets(ctx)
	if err == nil {
		s.stats.TotalDatasets = len(datasets)

		var totalVectors int64
		var totalStorage int64

		for _, dataset := range datasets {
			totalVectors += dataset.VectorCount
			totalStorage += dataset.StorageSize
		}

		s.stats.TotalVectors = totalVectors
		s.stats.StorageUsage = totalStorage
	}

	// Update provider stats
	modelInfo := s.provider.GetModelInfo()
	s.stats.ProviderStats["provider"] = modelInfo.Provider
	s.stats.ProviderStats["model"] = modelInfo.Name
	s.stats.ProviderStats["dimensions"] = modelInfo.Dimensions
	s.stats.ProviderStats["max_tokens"] = modelInfo.MaxTokens

	s.stats.LastUpdated = time.Now()
	return s.stats, nil
}

// Helper methods

func (s *embeddingService) validateProcessDocumentRequest(request *ProcessDocumentRequest) error {
	if request.DocumentID == "" {
		return ErrInvalidConfiguration
	}
	if request.Content == "" {
		return &EmbeddingError{
			Type:    "invalid_input",
			Message: "document content cannot be empty",
			Code:    "EMPTY_CONTENT",
		}
	}
	if request.Dataset == "" {
		request.Dataset = s.config.DefaultDataset
	}
	return nil
}

func (s *embeddingService) validateProcessChunksRequest(request *ProcessChunksRequest) error {
	if len(request.Chunks) == 0 {
		return &EmbeddingError{
			Type:    "invalid_input",
			Message: "chunks array cannot be empty",
			Code:    "EMPTY_CHUNKS",
		}
	}
	if request.Dataset == "" {
		request.Dataset = s.config.DefaultDataset
	}
	return nil
}

func (s *embeddingService) validateSearchRequest(request *SearchRequest) error {
	if request.Query == "" {
		return &EmbeddingError{
			Type:    "invalid_input",
			Message: "search query cannot be empty",
			Code:    "EMPTY_QUERY",
		}
	}
	if request.Dataset == "" {
		request.Dataset = s.config.DefaultDataset
	}
	return nil
}

func (s *embeddingService) ensureDatasetExists(ctx context.Context, datasetName string) error {
	// Check if dataset exists
	_, err := s.vectorStore.GetDatasetInfo(ctx, datasetName)
	if err == nil {
		return nil // Dataset exists
	}

	// Create dataset if it doesn't exist
	config := &DatasetConfig{
		Name:        datasetName,
		Description: fmt.Sprintf("Auto-created dataset: %s", datasetName),
		Dimensions:  s.provider.GetDimensions(),
		MetricType:  s.config.DefaultMetric,
		IndexType:   "hnsw",
	}

	return s.vectorStore.CreateDataset(ctx, config)
}

func (s *embeddingService) documentExists(ctx context.Context, _ /* dataset */, documentID string) (bool, error) {
	vectors, err := s.GetDocumentVectors(ctx, documentID)
	if err != nil {
		return false, err
	}
	return len(vectors) > 0, nil
}

func (s *embeddingService) chunkDocument(content string, chunkSize, chunkOverlap int) ([]string, error) {
	if chunkSize == 0 {
		chunkSize = s.config.ChunkSize
	}
	if chunkOverlap == 0 {
		chunkOverlap = s.config.ChunkOverlap
	}

	// Use the chunking package to split the document
	chunker := chunking.NewFixedSizeChunker(chunkSize, chunkOverlap)
	chunks, err := chunker.Chunk(content)
	if err != nil {
		return nil, err
	}

	result := make([]string, len(chunks))
	for i, chunk := range chunks {
		result[i] = chunk.Content
	}

	return result, nil
}

func (s *embeddingService) groupResultsByDocument(results []*SimilarityResult) []*SimilarityResult {
	// Group results by document ID and keep the best result for each document
	docMap := make(map[string]*SimilarityResult)

	for _, result := range results {
		docID := result.DocumentVector.DocumentID
		if existing, exists := docMap[docID]; !exists || result.Score > existing.Score {
			docMap[docID] = result
		}
	}

	// Convert back to slice
	grouped := make([]*SimilarityResult, 0, len(docMap))
	for _, result := range docMap {
		grouped = append(grouped, result)
	}

	return grouped
}

func (s *embeddingService) deduplicateResults(results []*SimilarityResult) []*SimilarityResult {
	seen := make(map[string]bool)
	deduplicated := make([]*SimilarityResult, 0, len(results))

	for _, result := range results {
		// Use content hash for deduplication
		hash := result.DocumentVector.ContentHash
		if hash == "" {
			// Fallback to content if no hash
			hash = result.DocumentVector.Content
		}

		if !seen[hash] {
			seen[hash] = true
			deduplicated = append(deduplicated, result)
		}
	}

	return deduplicated
}

func (s *embeddingService) updateProcessingStats(chunksProcessed, vectorsCreated int, processingTime float64) {
	s.stats.TotalDocuments++
	s.stats.TotalVectors += int64(vectorsCreated)

	// Update average processing time
	if s.stats.TotalDocuments == 1 {
		s.stats.AvgProcessingTime = processingTime
	} else {
		s.stats.AvgProcessingTime = (s.stats.AvgProcessingTime + processingTime) / AverageWeightFactor
	}
}

func (s *embeddingService) updateSearchStats(resultCount int, queryTime float64) {
	s.stats.TotalSearches++

	// Update average search time
	if s.stats.TotalSearches == 1 {
		s.stats.AvgSearchTime = queryTime
	} else {
		s.stats.AvgSearchTime = (s.stats.AvgSearchTime + queryTime) / AverageWeightFactor
	}
}
