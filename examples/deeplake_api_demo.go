package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jscharber/eAIIngest/pkg/embeddings"
	"github.com/jscharber/eAIIngest/pkg/embeddings/client"
)

// Example program to test the DeepLake API integration
func main() {
	fmt.Println("Testing DeepLake API Integration...")

	// Create API client configuration
	config := &client.DeepLakeAPIConfig{
		BaseURL:   "http://localhost:8000",
		APIKey:    "dev-key-12345",
		TenantID:  "",
		Timeout:   30 * time.Second,
		Retries:   3,
		UserAgent: "eAIIngest-Test/1.0",
	}

	// Create API client
	apiClient, err := client.NewDeepLakeAPIClient(config)
	if err != nil {
		log.Fatalf("Failed to create API client: %v", err)
	}
	defer apiClient.Close()

	ctx := context.Background()

	// Test 1: List existing datasets
	fmt.Println("\n=== Test 1: List Datasets ===")
	datasets, err := apiClient.ListDatasets(ctx)
	if err != nil {
		log.Printf("Error listing datasets: %v", err)
	} else {
		fmt.Printf("Found %d datasets:\n", len(datasets))
		for i, dataset := range datasets {
			fmt.Printf("  %d. %s (%d dimensions, %d vectors)\n",
				i+1, dataset.Name, dataset.Dimensions, dataset.VectorCount)
		}
	}

	// Test 2: Create a test dataset
	fmt.Println("\n=== Test 2: Create Dataset ===")
	datasetConfig := &embeddings.DatasetConfig{
		Name:        "test_dataset",
		Description: "Test dataset for API integration",
		Dimensions:  1536, // OpenAI embedding dimensions
		MetricType:  "cosine",
		IndexType:   "hnsw",
		Metadata: map[string]interface{}{
			"created_by": "integration_test",
			"purpose":    "testing",
		},
		Overwrite: true,
	}

	err = apiClient.CreateDataset(ctx, datasetConfig)
	if err != nil {
		log.Printf("Error creating dataset: %v", err)
	} else {
		fmt.Printf("Successfully created dataset: %s\n", datasetConfig.Name)
	}

	// Test 3: Get dataset info
	fmt.Println("\n=== Test 3: Get Dataset Info ===")
	datasetInfo, err := apiClient.GetDatasetInfo(ctx, "test_dataset")
	if err != nil {
		log.Printf("Error getting dataset info: %v", err)
	} else {
		fmt.Printf("Dataset Info:\n")
		fmt.Printf("  Name: %s\n", datasetInfo.Name)
		fmt.Printf("  Description: %s\n", datasetInfo.Description)
		fmt.Printf("  Dimensions: %d\n", datasetInfo.Dimensions)
		fmt.Printf("  Vector Count: %d\n", datasetInfo.VectorCount)
		fmt.Printf("  Metric Type: %s\n", datasetInfo.MetricType)
		fmt.Printf("  Index Type: %s\n", datasetInfo.IndexType)
		fmt.Printf("  Created At: %s\n", datasetInfo.CreatedAt.Format(time.RFC3339))
	}

	// Test 4: Insert test vectors
	fmt.Println("\n=== Test 4: Insert Vectors ===")
	testVectors := []*embeddings.DocumentVector{
		{
			ID:         "vec_1",
			DocumentID: "doc_1",
			ChunkID:    "chunk_1",
			Content:    "This is a test document about machine learning.",
			Vector:     generateTestVector(1536),
			Metadata: map[string]interface{}{
				"category": "ml",
				"author":   "test",
				"tags":     []string{"machine learning", "test"},
			},
			ContentType: "text/plain",
			Language:    "en",
		},
		{
			ID:         "vec_2",
			DocumentID: "doc_2",
			ChunkID:    "chunk_1",
			Content:    "This document discusses artificial intelligence concepts.",
			Vector:     generateTestVector(1536),
			Metadata: map[string]interface{}{
				"category": "ai",
				"author":   "test",
				"tags":     []string{"artificial intelligence", "test"},
			},
			ContentType: "text/plain",
			Language:    "en",
		},
		{
			ID:         "vec_3",
			DocumentID: "doc_3",
			ChunkID:    "chunk_1",
			Content:    "Natural language processing is a subfield of AI.",
			Vector:     generateTestVector(1536),
			Metadata: map[string]interface{}{
				"category": "nlp",
				"author":   "test",
				"tags":     []string{"nlp", "ai", "test"},
			},
			ContentType: "text/plain",
			Language:    "en",
		},
	}

	err = apiClient.InsertVectors(ctx, "test_dataset", testVectors)
	if err != nil {
		log.Printf("Error inserting vectors: %v", err)
	} else {
		fmt.Printf("Successfully inserted %d vectors\n", len(testVectors))
	}

	// Test 5: Vector similarity search
	fmt.Println("\n=== Test 5: Vector Similarity Search ===")
	queryVector := generateTestVector(1536)
	searchOptions := &embeddings.SearchOptions{
		TopK:            3,
		Threshold:       0.5,
		MetricType:      "cosine",
		IncludeContent:  true,
		IncludeMetadata: true,
	}

	searchResult, err := apiClient.SearchSimilar(ctx, "test_dataset", queryVector, searchOptions)
	if err != nil {
		log.Printf("Error performing similarity search: %v", err)
	} else {
		fmt.Printf("Search Results (%d found, query time: %.2fms):\n",
			searchResult.TotalFound, searchResult.QueryTime)
		for i, result := range searchResult.Results {
			fmt.Printf("  %d. ID: %s, Score: %.4f, Distance: %.4f\n",
				i+1, result.DocumentVector.ID, result.Score, result.Distance)
			fmt.Printf("     Content: %s\n", result.DocumentVector.Content)
			if category, ok := result.DocumentVector.Metadata["category"]; ok {
				fmt.Printf("     Category: %s\n", category)
			}
		}
	}

	// Test 6: Text-based search (if supported)
	fmt.Println("\n=== Test 6: Text-Based Search ===")
	textSearchResult, err := apiClient.SearchByText(ctx, "test_dataset", "machine learning concepts", searchOptions)
	if err != nil {
		log.Printf("Text search not available or error: %v", err)
	} else {
		fmt.Printf("Text Search Results (%d found, query time: %.2fms):\n",
			textSearchResult.TotalFound, textSearchResult.QueryTime)
		for i, result := range textSearchResult.Results {
			fmt.Printf("  %d. ID: %s, Score: %.4f\n",
				i+1, result.DocumentVector.ID, result.Score)
			fmt.Printf("     Content: %s\n", result.DocumentVector.Content)
		}
	}

	// Test 7: Get specific vector
	fmt.Println("\n=== Test 7: Get Specific Vector ===")
	vector, err := apiClient.GetVector(ctx, "test_dataset", "vec_1")
	if err != nil {
		log.Printf("Error getting vector: %v", err)
	} else {
		fmt.Printf("Retrieved Vector:\n")
		fmt.Printf("  ID: %s\n", vector.ID)
		fmt.Printf("  Document ID: %s\n", vector.DocumentID)
		fmt.Printf("  Content: %s\n", vector.Content)
		fmt.Printf("  Vector dimensions: %d\n", len(vector.Vector))
	}

	// Test 8: Update vector metadata
	fmt.Println("\n=== Test 8: Update Vector Metadata ===")
	updateMetadata := map[string]interface{}{
		"updated_at": time.Now(),
		"version":    "1.1",
		"test_flag":  true,
	}

	err = apiClient.UpdateVector(ctx, "test_dataset", "vec_1", updateMetadata)
	if err != nil {
		log.Printf("Error updating vector metadata: %v", err)
	} else {
		fmt.Printf("Successfully updated vector metadata\n")
	}

	// Test 9: Get dataset statistics
	fmt.Println("\n=== Test 9: Get Dataset Statistics ===")
	stats, err := apiClient.GetDatasetStats(ctx, "test_dataset")
	if err != nil {
		log.Printf("Error getting dataset stats: %v", err)
	} else {
		fmt.Printf("Dataset Statistics:\n")
		for key, value := range stats {
			fmt.Printf("  %s: %v\n", key, value)
		}
	}

	// Test 10: Delete vector
	fmt.Println("\n=== Test 10: Delete Vector ===")
	err = apiClient.DeleteVector(ctx, "test_dataset", "vec_3")
	if err != nil {
		log.Printf("Error deleting vector: %v", err)
	} else {
		fmt.Printf("Successfully deleted vector vec_3\n")
	}

	fmt.Println("\n=== Integration Test Complete ===")
	fmt.Println("Review the results above to verify API integration is working correctly.")
	fmt.Println("Note: Some tests may fail if the DeepLake API service is not running or not configured properly.")
}

// generateTestVector creates a random test vector of the specified dimensions
func generateTestVector(dimensions int) []float32 {
	vector := make([]float32, dimensions)

	// Generate a simple pattern to create consistent but varied test vectors
	for i := 0; i < dimensions; i++ {
		// Create a simple sine wave pattern with some noise
		base := float32(i) / float32(dimensions) * 6.28 // 2π
		vector[i] = float32(0.5 + 0.3*sin(float64(base)) + 0.2*sin(float64(base*3)))
	}

	return vector
}

// Simple sin function approximation for test vector generation
func sin(x float64) float64 {
	// Taylor series approximation for sin(x)
	// sin(x) ≈ x - x³/3! + x⁵/5! - x⁷/7! + ...

	// Normalize x to [-π, π] range
	for x > 3.14159 {
		x -= 6.28318
	}
	for x < -3.14159 {
		x += 6.28318
	}

	x2 := x * x
	result := x
	term := x

	// Calculate first few terms of Taylor series
	for i := 1; i < 10; i++ {
		term *= -x2 / float64((2*i)*(2*i+1))
		result += term
		if abs(term) < 1e-10 {
			break
		}
	}

	return result
}

// Simple absolute value function
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
