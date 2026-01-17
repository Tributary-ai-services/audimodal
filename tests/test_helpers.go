package tests

import (
	"os"
	"strings"
)

// Common test constants with K8s service discovery defaults
// For local testing, set environment variables:
//
//	DEEPLAKE_API_URL=http://localhost:8000
//	AUDIMODAL_URL=http://localhost:8080
var (
	deeplakeAPIURL  = getEnvOrDefault("DEEPLAKE_API_URL", "http://deeplake-api:8000")
	audimodalAPIURL = getEnvOrDefault("AUDIMODAL_URL", "http://audimodal:8080")
	baseURL         = getEnvOrDefault("AUDIMODAL_URL", "http://audimodal:8080")
	deeplakeURL     = getEnvOrDefault("DEEPLAKE_API_URL", "http://deeplake-api:8000")
)

const (
	testDatasetName  = "test_audimodal_dataset"
	testTenantID     = "ba305c7d-cf52-475f-991b-0dea63109d25"          // UUID of test_tenant created in K8s
	testDataSourceID = "2e887333-712f-4c4b-b79e-d4a61e28cda5"          // UUID of test_datasource created in K8s
	testAPIKey       = "test-api-key-for-integration-testing-12345678" // Must be at least 32 chars
)

// extractResponseData extracts the data field from wrapped API responses
// AudiModal API returns responses in format: {"success": true, "data": {...}, "timestamp": "...", "request_id": "..."}
func extractResponseData(response map[string]interface{}) map[string]interface{} {
	if data, ok := response["data"].(map[string]interface{}); ok {
		return data
	}
	// Return original if not wrapped (for backwards compatibility)
	return response
}

// getEnvOrDefault retrieves an environment variable or returns a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// generateLargeContent generates content for testing with specified size/sentences
// The size parameter can be interpreted as either bytes or sentence count depending on context
func generateLargeContent(size int) string {
	sentence := "This is a sample sentence for testing large document processing. "

	// If size is likely meant as sentence count (< 1000), interpret as sentences
	if size < 1000 {
		return strings.Repeat(sentence, size)
	}

	// Otherwise interpret as approximate byte count
	sentences := size / len(sentence)
	if sentences == 0 {
		sentences = 1
	}
	return strings.Repeat(sentence, sentences)
}
