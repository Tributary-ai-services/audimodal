package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfigSpec(t *testing.T) {
	spec := ConfigSpec{
		Name:        "test_param",
		Type:        "string",
		Required:    true,
		Description: "Test parameter",
		Default:     "default_value",
		Enum:        []string{"option1", "option2"},
		Examples:    []string{"example1", "example2"},
	}

	assert.Equal(t, "test_param", spec.Name)
	assert.Equal(t, "string", spec.Type)
	assert.True(t, spec.Required)
	assert.Equal(t, "Test parameter", spec.Description)
	assert.Equal(t, "default_value", spec.Default)
	assert.Contains(t, spec.Enum, "option1")
	assert.Contains(t, spec.Examples, "example1")
}

func TestConnectionTestResult(t *testing.T) {
	result := ConnectionTestResult{
		Success: true,
		Message: "Connection successful",
		Details: map[string]any{"server": "localhost"},
		Latency: 100 * time.Millisecond,
		Errors:  []string{},
	}

	assert.True(t, result.Success)
	assert.Equal(t, "Connection successful", result.Message)
	assert.Equal(t, "localhost", result.Details["server"])
	assert.Equal(t, 100*time.Millisecond, result.Latency)
	assert.Empty(t, result.Errors)
}

func TestChunk(t *testing.T) {
	metadata := ChunkMetadata{
		SourcePath:  "/path/to/file.txt",
		ChunkID:     "chunk-123",
		ChunkType:   "text",
		SizeBytes:   1024,
		ProcessedAt: time.Now(),
		ProcessedBy: "test-strategy",
	}

	chunk := Chunk{
		Data:     "This is test chunk content",
		Metadata: metadata,
	}

	assert.Equal(t, "This is test chunk content", chunk.Data)
	assert.Equal(t, "chunk-123", chunk.Metadata.ChunkID)
	assert.Equal(t, "text", chunk.Metadata.ChunkType)
	assert.Equal(t, int64(1024), chunk.Metadata.SizeBytes)
}

func TestSchemaInfo(t *testing.T) {
	schema := SchemaInfo{
		Format:   "structured",
		Encoding: "UTF-8",
		Fields: []FieldInfo{
			{
				Name:     "id",
				Type:     "integer",
				Nullable: false,
				Primary:  true,
			},
		},
		Metadata: map[string]any{"table_name": "users"},
	}

	assert.Equal(t, "structured", schema.Format)
	assert.Equal(t, "UTF-8", schema.Encoding)
	assert.Len(t, schema.Fields, 1)
	assert.Equal(t, "id", schema.Fields[0].Name)
	assert.Equal(t, "users", schema.Metadata["table_name"])
}

func TestSizeEstimate(t *testing.T) {
	rowCount := int64(1000)
	columnCount := 5

	estimate := SizeEstimate{
		RowCount:    &rowCount,
		ByteSize:    1024,
		ColumnCount: &columnCount,
		Complexity:  "medium",
		ChunkEst:    10,
		ProcessTime: "5 minutes",
	}

	assert.Equal(t, int64(1000), *estimate.RowCount)
	assert.Equal(t, int64(1024), estimate.ByteSize)
	assert.Equal(t, 5, *estimate.ColumnCount)
	assert.Equal(t, "medium", estimate.Complexity)
	assert.Equal(t, 10, estimate.ChunkEst)
	assert.Equal(t, "5 minutes", estimate.ProcessTime)
}

func TestFieldInfo(t *testing.T) {
	field := FieldInfo{
		Name:        "username",
		Type:        "string",
		Nullable:    false,
		Primary:     false,
		Description: "User's login name",
		Examples:    []any{"john_doe", "jane_smith"},
	}

	assert.Equal(t, "username", field.Name)
	assert.Equal(t, "string", field.Type)
	assert.False(t, field.Nullable)
	assert.False(t, field.Primary)
	assert.Equal(t, "User's login name", field.Description)
	assert.Contains(t, field.Examples, "john_doe")
}

func TestQualityMetrics(t *testing.T) {
	quality := QualityMetrics{
		Completeness: 0.95,
		Coherence:    0.87,
		Uniqueness:   0.92,
		Readability:  0.78,
		LanguageConf: 0.99,
		Language:     "en",
	}

	assert.Equal(t, 0.95, quality.Completeness)
	assert.Equal(t, 0.87, quality.Coherence)
	assert.Equal(t, 0.92, quality.Uniqueness)
	assert.Equal(t, 0.78, quality.Readability)
	assert.Equal(t, 0.99, quality.LanguageConf)
	assert.Equal(t, "en", quality.Language)
}

func TestDiscoveryHints(t *testing.T) {
	hints := DiscoveryHints{
		HasTimestamps:   true,
		HasTextContent:  true,
		ContentPatterns: []string{"email", "phone"},
		Languages:       []string{"en", "es"},
		DataClasses:     []string{"personal", "contact"},
		Confidence:      0.85,
	}

	assert.True(t, hints.HasTimestamps)
	assert.True(t, hints.HasTextContent)
	assert.Contains(t, hints.ContentPatterns, "email")
	assert.Contains(t, hints.Languages, "en")
	assert.Contains(t, hints.DataClasses, "personal")
	assert.Equal(t, 0.85, hints.Confidence)
}

func TestRecommendedStrategy(t *testing.T) {
	strategy := RecommendedStrategy{
		StrategyType: "semantic_chunking",
		Confidence:   0.92,
		Config:       map[string]any{"chunk_size": 512},
		Reasoning:    "High text content with natural language structure",
		Alternatives: []string{"fixed_size", "paragraph_based"},
	}

	assert.Equal(t, "semantic_chunking", strategy.StrategyType)
	assert.Equal(t, 0.92, strategy.Confidence)
	assert.Equal(t, 512, strategy.Config["chunk_size"])
	assert.Contains(t, strategy.Reasoning, "text content")
	assert.Contains(t, strategy.Alternatives, "fixed_size")
}

func TestPluginError(t *testing.T) {
	err := NewPluginError("TEST_ERROR", "This is a test error")
	assert.Equal(t, "TEST_ERROR", err.Code)
	assert.Equal(t, "This is a test error", err.Message)
	assert.Equal(t, "This is a test error", err.Error())

	// Test with details
	errWithDetails := err.WithDetails(map[string]string{"file": "test.txt"})
	assert.Equal(t, "TEST_ERROR", errWithDetails.Code)
	assert.NotNil(t, errWithDetails.Details)
}

func TestPredefinedErrors(t *testing.T) {
	assert.Equal(t, "UNSUPPORTED_FORMAT", ErrUnsupportedFormat.Code)
	assert.Equal(t, "INVALID_CONFIG", ErrInvalidConfig.Code)
	assert.Equal(t, "CONNECTION_FAILED", ErrConnectionFailed.Code)
	assert.Equal(t, "ITERATOR_EXHAUSTED", ErrIteratorExhausted.Code)
	assert.Equal(t, "PROCESSING_FAILED", ErrProcessingFailed.Code)
}

func TestProcessingContext(t *testing.T) {
	deadline := time.Now().Add(1 * time.Hour)
	context := ProcessingContext{
		TenantID:       "tenant-123",
		SessionID:      "session-456",
		RequestID:      "req-789",
		UserID:         "user-001",
		Priority:       "high",
		Deadline:       &deadline,
		Metadata:       map[string]string{"source": "api"},
		ComplianceReqs: []string{"GDPR", "HIPAA"},
		DataResidency:  []string{"EU", "US"},
	}

	assert.Equal(t, "tenant-123", context.TenantID)
	assert.Equal(t, "high", context.Priority)
	assert.Equal(t, "api", context.Metadata["source"])
	assert.Contains(t, context.ComplianceReqs, "GDPR")
	assert.Contains(t, context.DataResidency, "EU")
}
