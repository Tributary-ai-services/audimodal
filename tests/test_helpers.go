package tests

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
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

// requireIntegrationServices skips the calling test unless both AudiModal and
// DeepLake answer on the URLs above.
//
// The defaults are Kubernetes service names, so they resolve inside the cluster
// and nowhere else — on a CI runner or a laptop the whole suite failed on DNS
// rather than on anything it was written to test. Probing and skipping keeps
// those runs honest: the tests still run wherever the services exist (point
// AUDIMODAL_URL and DEEPLAKE_API_URL at them, or port-forward), and elsewhere
// they announce what is missing instead of reporting a failure nobody can act
// on.
//
// The probe runs once per package run; each service is reported separately so a
// skip says which one was unreachable.
func requireIntegrationServices(t *testing.T) {
	t.Helper()
	integrationProbeOnce.Do(func() {
		integrationProbeErr = probeServices()
	})
	if integrationProbeErr != nil {
		t.Skipf("integration services unavailable: %v "+
			"(set AUDIMODAL_URL and DEEPLAKE_API_URL to reachable endpoints to run this test)",
			integrationProbeErr)
	}
}

var (
	integrationProbeOnce sync.Once
	integrationProbeErr  error
)

func probeServices() error {
	client := &http.Client{Timeout: 3 * time.Second}
	for _, svc := range []struct{ name, url string }{
		{"audimodal", audimodalAPIURL + "/health"},
		{"deeplake-api", deeplakeURL + "/api/v1/health"},
	} {
		resp, err := client.Get(svc.url)
		if err != nil {
			return fmt.Errorf("%s not reachable at %s: %w", svc.name, svc.url, err)
		}
		resp.Body.Close()
		if resp.StatusCode >= 500 {
			return fmt.Errorf("%s at %s returned HTTP %d", svc.name, svc.url, resp.StatusCode)
		}
	}
	return nil
}
