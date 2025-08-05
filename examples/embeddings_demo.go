package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jscharber/eAIIngest/pkg/embeddings"
	"github.com/jscharber/eAIIngest/pkg/embeddings/client"
	"github.com/jscharber/eAIIngest/pkg/embeddings/providers"
)

func runEmbeddingsDemo() {
	fmt.Println("🚀 eAIIngest Vector Embeddings Demo")
	fmt.Println("=====================================")

	// Run the demo
	if err := runEmbeddingsDemoLogic(); err != nil {
		log.Fatalf("Demo failed: %v", err)
	}

	fmt.Println("\n✅ Demo completed successfully!")
}

func runEmbeddingsDemoLogic() error {
	ctx := context.Background()

	// Step 1: Initialize the embedding provider (OpenAI)
	fmt.Println("\n📡 Step 1: Initializing OpenAI embedding provider...")

	openaiConfig := &providers.OpenAIConfig{
		APIKey:     "demo-key", // In real usage, get from environment variable
		Model:      "text-embedding-3-small",
		MaxRetries: 3,
	}

	provider, err := providers.NewOpenAIProvider(openaiConfig)
	if err != nil {
		return fmt.Errorf("failed to create OpenAI provider: %v", err)
	}

	modelInfo := provider.GetModelInfo()
	fmt.Printf("   Provider: %s\n", modelInfo.Provider)
	fmt.Printf("   Model: %s\n", modelInfo.Name)
	fmt.Printf("   Dimensions: %d\n", modelInfo.Dimensions)
	fmt.Printf("   Max Tokens: %d\n", modelInfo.MaxTokens)

	// Step 2: Initialize the DeepLake API client
	fmt.Println("\n🗄️  Step 2: Initializing DeepLake API client...")

	apiConfig := &client.DeepLakeAPIConfig{
		BaseURL:   getEnvOrDefault("DEEPLAKE_API_URL", "http://localhost:8000"),
		APIKey:    getEnvOrDefault("DEEPLAKE_API_KEY", "dev-key-12345"),
		TenantID:  "",
		Timeout:   30 * time.Second,
		Retries:   3,
		UserAgent: "eAIIngest-Demo/1.0",
	}

	vectorStore, err := client.NewDeepLakeAPIClient(apiConfig)
	if err != nil {
		return fmt.Errorf("failed to create DeepLake API client: %v", err)
	}
	defer vectorStore.Close()

	// Step 3: Create the embedding service
	fmt.Println("\n🔧 Step 3: Creating embedding service...")

	serviceConfig := &embeddings.ServiceConfig{
		DefaultDataset:   "demo-docs",
		ChunkSize:        500,
		ChunkOverlap:     50,
		BatchSize:        5,
		MaxConcurrency:   3,
		CacheEnabled:     true,
		MetricsEnabled:   true,
		DefaultTopK:      5,
		DefaultThreshold: 0.7,
		DefaultMetric:    "cosine",
	}

	service := embeddings.NewEmbeddingService(provider, vectorStore, serviceConfig)
	fmt.Println("   ✅ Embedding service created successfully")

	// Step 4: Create a dataset
	fmt.Println("\n📚 Step 4: Creating demo dataset...")

	datasetConfig := &embeddings.DatasetConfig{
		Name:        "demo-docs",
		Description: "Demo dataset for vector embeddings",
		Dimensions:  provider.GetDimensions(),
		MetricType:  "cosine",
		IndexType:   "hnsw",
		Metadata: map[string]interface{}{
			"created_by": "demo",
			"purpose":    "demonstration",
		},
	}

	if err := vectorStore.CreateDataset(ctx, datasetConfig); err != nil {
		return fmt.Errorf("failed to create dataset: %v", err)
	}
	fmt.Println("   ✅ Dataset 'demo-docs' created")

	// Step 5: Process sample documents
	fmt.Println("\n📄 Step 5: Processing sample documents...")

	sampleDocuments := []struct {
		ID      string
		Content string
		Type    string
	}{
		{
			ID:      "doc-ai-intro",
			Content: "Artificial Intelligence (AI) is a branch of computer science that aims to create intelligent machines capable of performing tasks that typically require human intelligence. These tasks include learning, reasoning, problem-solving, perception, and language understanding. AI has applications in various fields including healthcare, finance, transportation, and entertainment.",
			Type:    "article",
		},
		{
			ID:      "doc-ml-basics",
			Content: "Machine Learning is a subset of artificial intelligence that focuses on algorithms and statistical models that enable computer systems to improve their performance on a specific task through experience. Common types of machine learning include supervised learning, unsupervised learning, and reinforcement learning. Popular algorithms include neural networks, decision trees, and support vector machines.",
			Type:    "educational",
		},
		{
			ID:      "doc-nlp-overview",
			Content: "Natural Language Processing (NLP) is a field of AI that deals with the interaction between computers and human language. It involves the development of algorithms and models that can understand, interpret, and generate human language. NLP applications include text analysis, sentiment analysis, machine translation, and chatbots. Recent advances in transformer models have significantly improved NLP capabilities.",
			Type:    "technical",
		},
		{
			ID:      "doc-deep-learning",
			Content: "Deep Learning is a subset of machine learning that uses artificial neural networks with multiple layers to model and understand complex patterns in data. Deep learning has achieved remarkable success in image recognition, speech recognition, and natural language processing. Popular frameworks include TensorFlow, PyTorch, and Keras.",
			Type:    "advanced",
		},
	}

	tenantID := uuid.New()

	for i, doc := range sampleDocuments {
		fmt.Printf("   Processing document %d/%d: %s\n", i+1, len(sampleDocuments), doc.ID)

		request := &embeddings.ProcessDocumentRequest{
			DocumentID:  doc.ID,
			TenantID:    tenantID,
			Content:     doc.Content,
			ContentType: "text/plain",
			Dataset:     "demo-docs",
			Metadata: map[string]interface{}{
				"type":      doc.Type,
				"processed": time.Now().Format(time.RFC3339),
				"demo":      true,
			},
		}

		response, err := service.ProcessDocument(ctx, request)
		if err != nil {
			return fmt.Errorf("failed to process document %s: %v", doc.ID, err)
		}

		fmt.Printf("      ✅ Created %d chunks, %d vectors in %.2fms\n",
			response.ChunksCreated, response.VectorsCreated, response.ProcessingTime)
	}

	// Step 6: Demonstrate semantic search
	fmt.Println("\n🔍 Step 6: Demonstrating semantic search...")

	searchQueries := []string{
		"What is artificial intelligence?",
		"How do neural networks work?",
		"Tell me about machine learning algorithms",
		"What are NLP applications?",
	}

	for i, query := range searchQueries {
		fmt.Printf("\n   Query %d: \"%s\"\n", i+1, query)

		searchRequest := &embeddings.SearchRequest{
			Query:    query,
			Dataset:  "demo-docs",
			TenantID: tenantID,
			Options: &embeddings.SearchOptions{
				TopK:            3,
				Threshold:       0.5,
				MetricType:      "cosine",
				IncludeContent:  true,
				IncludeMetadata: true,
			},
		}

		searchResponse, err := service.SearchDocuments(ctx, searchRequest)
		if err != nil {
			return fmt.Errorf("search failed for query '%s': %v", query, err)
		}

		fmt.Printf("   Found %d results in %.2fms:\n", len(searchResponse.Results), searchResponse.QueryTime)

		for j, result := range searchResponse.Results {
			contentPreview := result.DocumentVector.Content
			if len(contentPreview) > 100 {
				contentPreview = contentPreview[:100] + "..."
			}

			docType := "unknown"
			if t, ok := result.DocumentVector.Metadata["type"].(string); ok {
				docType = t
			}

			fmt.Printf("      %d. Document: %s (Score: %.3f, Type: %s)\n",
				j+1, result.DocumentVector.DocumentID, result.Score, docType)
			fmt.Printf("         Preview: %s\n", contentPreview)
		}
	}

	// Step 7: Get service statistics
	fmt.Println("\n📊 Step 7: Service statistics...")

	stats, err := service.GetStats(ctx)
	if err != nil {
		return fmt.Errorf("failed to get stats: %v", err)
	}

	fmt.Printf("   Total Documents: %d\n", stats.TotalDocuments)
	fmt.Printf("   Total Vectors: %d\n", stats.TotalVectors)
	fmt.Printf("   Total Searches: %d\n", stats.TotalSearches)
	fmt.Printf("   Avg Processing Time: %.2fms\n", stats.AvgProcessingTime)
	fmt.Printf("   Avg Search Time: %.2fms\n", stats.AvgSearchTime)
	fmt.Printf("   Storage Usage: %d bytes\n", stats.StorageUsage)

	// Step 8: Demonstrate document retrieval and deletion
	fmt.Println("\n🗑️  Step 8: Document management...")

	// Get vectors for a specific document
	docID := "doc-ai-intro"
	vectors, err := service.GetDocumentVectors(ctx, docID)
	if err != nil {
		return fmt.Errorf("failed to get document vectors: %v", err)
	}
	fmt.Printf("   Document '%s' has %d vectors\n", docID, len(vectors))

	// Delete a document
	if err := service.DeleteDocument(ctx, docID); err != nil {
		return fmt.Errorf("failed to delete document: %v", err)
	}
	fmt.Printf("   ✅ Document '%s' deleted successfully\n", docID)

	// Verify deletion
	vectors, err = service.GetDocumentVectors(ctx, docID)
	if err != nil {
		return fmt.Errorf("failed to verify deletion: %v", err)
	}
	fmt.Printf("   Document '%s' now has %d vectors (should be 0)\n", docID, len(vectors))

	return nil
}

// Additional demo functions

func demonstrateAdvancedFeatures(service embeddings.EmbeddingService) error {
	ctx := context.Background()
	tenantID := uuid.New()

	fmt.Println("\n🚀 Advanced Features Demo")
	fmt.Println("========================")

	// Batch processing demo
	fmt.Println("\n📦 Batch Processing Demo:")

	chunks := []*embeddings.ChunkInput{
		{
			ID:          "chunk-1",
			DocumentID:  "batch-doc",
			Content:     "This is the first chunk of content for batch processing.",
			ChunkIndex:  0,
			ContentType: "text/plain",
		},
		{
			ID:          "chunk-2",
			DocumentID:  "batch-doc",
			Content:     "This is the second chunk with different content for testing.",
			ChunkIndex:  1,
			ContentType: "text/plain",
		},
		{
			ID:          "chunk-3",
			DocumentID:  "batch-doc",
			Content:     "The third chunk contains more test content for the demo.",
			ChunkIndex:  2,
			ContentType: "text/plain",
		},
	}

	batchRequest := &embeddings.ProcessChunksRequest{
		Chunks:    chunks,
		Dataset:   "demo-docs",
		TenantID:  tenantID,
		BatchSize: 2,
	}

	batchResponse, err := service.ProcessChunks(ctx, batchRequest)
	if err != nil {
		return fmt.Errorf("batch processing failed: %v", err)
	}

	fmt.Printf("   Processed %d chunks, created %d vectors in %.2fms\n",
		batchResponse.ChunksProcessed, batchResponse.VectorsCreated, batchResponse.ProcessingTime)

	// Advanced search with filters
	fmt.Println("\n🔍 Advanced Search with Filters:")

	searchRequest := &embeddings.SearchRequest{
		Query:    "artificial intelligence and machine learning",
		Dataset:  "demo-docs",
		TenantID: tenantID,
		Options: &embeddings.SearchOptions{
			TopK:            5,
			Threshold:       0.3,
			MetricType:      "cosine",
			IncludeContent:  true,
			IncludeMetadata: true,
			GroupByDoc:      true,
			Deduplicate:     true,
		},
		Filters: map[string]interface{}{
			"type": "technical",
		},
	}

	searchResponse, err := service.SearchDocuments(ctx, searchRequest)
	if err != nil {
		return fmt.Errorf("advanced search failed: %v", err)
	}

	fmt.Printf("   Found %d unique documents in %.2fms\n",
		len(searchResponse.Results), searchResponse.QueryTime)

	if searchResponse.SearchStats != nil {
		fmt.Printf("   Search Stats:\n")
		fmt.Printf("      Vectors Scanned: %d\n", searchResponse.SearchStats.VectorsScanned)
		fmt.Printf("      Embedding Time: %.2fms\n", searchResponse.SearchStats.EmbeddingTime)
		fmt.Printf("      Database Time: %.2fms\n", searchResponse.SearchStats.DatabaseTime)
	}

	return nil
}

func demonstrateErrorHandling() {
	fmt.Println("\n⚠️  Error Handling Demo")
	fmt.Println("======================")

	// This would demonstrate various error scenarios:
	// - Invalid API keys
	// - Network timeouts
	// - Invalid input data
	// - Dataset not found
	// - Dimension mismatches
	// etc.

	fmt.Println("   Error handling examples would go here...")
	fmt.Println("   (Skipped in demo to avoid actual errors)")
}

func printDemoSummary() {
	fmt.Println("\n📋 Demo Summary")
	fmt.Println("===============")
	fmt.Println("✅ Initialized OpenAI embedding provider")
	fmt.Println("✅ Set up Deeplake vector store")
	fmt.Println("✅ Created embedding service with configuration")
	fmt.Println("✅ Created and configured dataset")
	fmt.Println("✅ Processed multiple documents with chunking")
	fmt.Println("✅ Performed semantic search with various queries")
	fmt.Println("✅ Retrieved service statistics")
	fmt.Println("✅ Demonstrated document management (retrieval/deletion)")
	fmt.Println("")
	fmt.Println("🎯 This demo shows the core capabilities of the eAIIngest")
	fmt.Println("   vector embeddings system for semantic document processing")
	fmt.Println("   and search functionality.")
}

// Helper function to get environment variable or default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
