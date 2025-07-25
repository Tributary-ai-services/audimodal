package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jscharber/eAIIngest/pkg/embeddings"
)

// TestDataFactory provides methods to create test data with consistent defaults
type TestDataFactory struct {
	rand *rand.Rand
}

// NewTestDataFactory creates a new test data factory with a seeded random generator
func NewTestDataFactory() *TestDataFactory {
	return &TestDataFactory{
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// CreateDatasetConfig creates a test dataset configuration
func (f *TestDataFactory) CreateDatasetConfig(name string) *embeddings.DatasetConfig {
	if name == "" {
		name = fmt.Sprintf("test-dataset-%d", f.rand.Int31())
	}
	
	return &embeddings.DatasetConfig{
		Name:        name,
		Description: fmt.Sprintf("Test dataset %s", name),
		Dimensions:  1536,
		MetricType:  "cosine",
		IndexType:   "hnsw",
		Metadata: map[string]interface{}{
			"test":        true,
			"created_by":  "test_factory",
			"environment": "test",
		},
	}
}

// CreateDatasetInfo creates test dataset info
func (f *TestDataFactory) CreateDatasetInfo(name string) *embeddings.DatasetInfo {
	if name == "" {
		name = fmt.Sprintf("test-dataset-%d", f.rand.Int31())
	}
	
	return &embeddings.DatasetInfo{
		Name:        name,
		Description: fmt.Sprintf("Test dataset %s", name),
		Dimensions:  1536,
		MetricType:  "cosine",
		IndexType:   "hnsw",
		VectorCount: int64(f.rand.Intn(1000)),
		StorageSize: int64(f.rand.Intn(10000)),
		CreatedAt:   time.Now().Add(-time.Duration(f.rand.Intn(3600)) * time.Second),
		UpdatedAt:   time.Now(),
		Metadata: map[string]interface{}{
			"test":        true,
			"environment": "test",
		},
	}
}

// CreateDocumentVector creates a test document vector
func (f *TestDataFactory) CreateDocumentVector(id, docID string) *embeddings.DocumentVector {
	if id == "" {
		id = fmt.Sprintf("vec-%d", f.rand.Int31())
	}
	if docID == "" {
		docID = fmt.Sprintf("doc-%d", f.rand.Int31())
	}
	
	// Generate random vector with fixed dimensions
	vector := make([]float32, 5) // Small vector for testing
	for i := range vector {
		vector[i] = f.rand.Float32()
	}
	
	return &embeddings.DocumentVector{
		ID:         id,
		DocumentID: docID,
		ChunkID:    fmt.Sprintf("chunk-%d", f.rand.Int31()),
		Content:    fmt.Sprintf("Test content for vector %s", id),
		Vector:     vector,
		Metadata: map[string]interface{}{
			"test":        true,
			"chunk_index": f.rand.Intn(10),
			"source":      "test_factory",
		},
	}
}

// CreateSearchOptions creates test search options
func (f *TestDataFactory) CreateSearchOptions() *embeddings.SearchOptions {
	return &embeddings.SearchOptions{
		TopK:            5,
		Threshold:       0.7,
		MetricType:      "cosine",
		IncludeContent:  true,
		IncludeMetadata: true,
		Filters: map[string]interface{}{
			"test": true,
		},
	}
}

// CreateSearchResult creates a test search result
func (f *TestDataFactory) CreateSearchResult(count int) *embeddings.SearchResult {
	results := make([]*embeddings.SimilarityResult, count)
	
	for i := 0; i < count; i++ {
		vector := f.CreateDocumentVector("", "")
		score := 1.0 - (float32(i) * 0.1) // Decreasing scores
		
		results[i] = &embeddings.SimilarityResult{
			DocumentVector: vector,
			Score:          score,
			Distance:       1.0 - score,
			Rank:           i + 1,
		}
	}
	
	return &embeddings.SearchResult{
		Results:    results,
		QueryTime:  float64(f.rand.Intn(100)) + f.rand.Float64(),
		TotalFound: count,
		HasMore:    count >= 5, // Assume more results if we have 5 or more
	}
}

// CreateProcessDocumentRequest creates a test process document request
func (f *TestDataFactory) CreateProcessDocumentRequest(tenantID uuid.UUID) *embeddings.ProcessDocumentRequest {
	return &embeddings.ProcessDocumentRequest{
		DocumentID:   fmt.Sprintf("doc-%d", f.rand.Int31()),
		Content:      "This is test content for document processing. It contains multiple sentences to test chunking.",
		ContentType:  "text/plain",
		TenantID:     tenantID,
		Dataset:      fmt.Sprintf("test-dataset-%d", f.rand.Int31()),
		ChunkSize:    1000,
		ChunkOverlap: 100,
		Metadata: map[string]interface{}{
			"source":        "test_factory",
			"document_type": "test",
			"created_at":    time.Now().Format(time.RFC3339),
		},
	}
}

// CreateProcessDocumentResponse creates a test process document response
func (f *TestDataFactory) CreateProcessDocumentResponse(docID string, chunkCount int) *embeddings.ProcessDocumentResponse {
	return &embeddings.ProcessDocumentResponse{
		DocumentID:      docID,
		ChunksCreated:   chunkCount,
		VectorsCreated:  chunkCount,
		ProcessingTime:  float64(f.rand.Intn(1000)) + f.rand.Float64(),
		TotalTokens:     chunkCount * 100, // Approximate tokens
		EstimatedCost:   float64(chunkCount) * 0.001, // Sample cost
		Status:          "completed",
		CreatedAt:       time.Now(),
	}
}

// HTTPTestHelper provides utilities for HTTP testing
type HTTPTestHelper struct {
	t *testing.T
}

// NewHTTPTestHelper creates a new HTTP test helper
func NewHTTPTestHelper(t *testing.T) *HTTPTestHelper {
	return &HTTPTestHelper{t: t}
}

// CreateRequest creates an HTTP request with JSON body and headers
func (h *HTTPTestHelper) CreateRequest(method, url string, body interface{}) *http.Request {
	var bodyReader *bytes.Buffer
	
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("Failed to marshal request body: %v", err)
		}
		bodyReader = bytes.NewBuffer(jsonBody)
	} else {
		bodyReader = bytes.NewBuffer([]byte{})
	}
	
	req := httptest.NewRequest(method, url, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", "test-tenant-id")
	
	return req
}

// CreateRequestWithTenant creates an HTTP request with a specific tenant ID
func (h *HTTPTestHelper) CreateRequestWithTenant(method, url string, body interface{}, tenantID string) *http.Request {
	req := h.CreateRequest(method, url, body)
	req.Header.Set("X-Tenant-ID", tenantID)
	return req
}

// AssertJSONResponse checks that the response is valid JSON and returns the parsed data
func (h *HTTPTestHelper) AssertJSONResponse(rr *httptest.ResponseRecorder, expectedStatus int) map[string]interface{} {
	if rr.Code != expectedStatus {
		h.t.Errorf("Expected status %d, got %d. Body: %s", expectedStatus, rr.Code, rr.Body.String())
	}
	
	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		h.t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}
	
	var response map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		h.t.Fatalf("Failed to parse JSON response: %v. Body: %s", err, rr.Body.String())
	}
	
	return response
}

// AssertTypedJSONResponse parses the response into a specific type
func (h *HTTPTestHelper) AssertTypedJSONResponse(rr *httptest.ResponseRecorder, expectedStatus int, target interface{}) {
	if rr.Code != expectedStatus {
		h.t.Errorf("Expected status %d, got %d. Body: %s", expectedStatus, rr.Code, rr.Body.String())
	}
	
	if err := json.Unmarshal(rr.Body.Bytes(), target); err != nil {
		h.t.Fatalf("Failed to parse JSON response: %v. Body: %s", err, rr.Body.String())
	}
}

// MockExpectationHelper provides utilities for setting up mock expectations
type MockExpectationHelper struct {
	factory *TestDataFactory
}

// NewMockExpectationHelper creates a new mock expectation helper
func NewMockExpectationHelper() *MockExpectationHelper {
	return &MockExpectationHelper{
		factory: NewTestDataFactory(),
	}
}

// Note: Mock expectation helper methods have been removed.
// The actual mock types (MockDeepLakeAPIClient, MockEmbeddingService) are defined
// in embeddings_test.go and should be used directly in test files.

// TestScenarioRunner helps run common test scenarios
type TestScenarioRunner struct {
	t           *testing.T
	factory     *TestDataFactory
	httpHelper  *HTTPTestHelper
	mockHelper  *MockExpectationHelper
}

// NewTestScenarioRunner creates a new test scenario runner
func NewTestScenarioRunner(t *testing.T) *TestScenarioRunner {
	return &TestScenarioRunner{
		t:          t,
		factory:    NewTestDataFactory(),
		httpHelper: NewHTTPTestHelper(t),
		mockHelper: NewMockExpectationHelper(),
	}
}

// Note: Test scenario runner methods have been removed.
// These should be implemented directly in test files using the actual mock types.

// TestTenantID generates a consistent test tenant ID
func TestTenantID() uuid.UUID {
	// Use a fixed UUID for consistent testing
	return uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
}

// ContextWithTimeout creates a context with a reasonable timeout for tests
func ContextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// Note: Mock assertion helpers have been removed.
// Use mock.AssertExpectations directly in test files.

// RandomString generates a random string of specified length
func RandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// WaitForCondition waits for a condition to be true within a timeout
func WaitForCondition(timeout time.Duration, condition func() bool) bool {
	start := time.Now()
	for time.Since(start) < timeout {
		if condition() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// SkipIfShort skips the test if running in short mode
func SkipIfShort(t *testing.T, reason string) {
	if testing.Short() {
		t.Skipf("Skipping in short mode: %s", reason)
	}
}