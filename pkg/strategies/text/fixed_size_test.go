package text

import (
	"context"
	"testing"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
)

func TestFixedSizeStrategy_Basic(t *testing.T) {
	strategy := NewFixedSizeStrategy()
	
	if strategy.GetType() != "strategy" {
		t.Errorf("Expected type 'strategy', got %s", strategy.GetType())
	}
	
	if strategy.GetName() != "fixed_size_text" {
		t.Errorf("Expected name 'fixed_size_text', got %s", strategy.GetName())
	}
	
	if !strategy.SupportsParallelProcessing() {
		t.Error("Fixed size strategy should support parallel processing")
	}
	
	capabilities := strategy.GetRequiredReaderCapabilities()
	if len(capabilities) == 0 {
		t.Error("Expected reader capabilities")
	}
}

func TestFixedSizeStrategy_ConfigValidation(t *testing.T) {
	strategy := NewFixedSizeStrategy()
	
	// Valid config
	validConfig := map[string]any{
		"chunk_size":         1000.0,
		"overlap_size":       100.0,
		"split_on_sentences": true,
		"min_chunk_size":     50.0,
	}
	
	if err := strategy.ValidateConfig(validConfig); err != nil {
		t.Errorf("Valid config failed validation: %v", err)
	}
	
	// Invalid chunk size (too small)
	invalidConfig := map[string]any{
		"chunk_size": 50.0,
	}
	
	if err := strategy.ValidateConfig(invalidConfig); err == nil {
		t.Error("Invalid chunk size should have failed validation")
	}
	
	// Overlap larger than chunk size
	invalidConfig2 := map[string]any{
		"chunk_size":   1000.0,
		"overlap_size": 1500.0,
	}
	
	if err := strategy.ValidateConfig(invalidConfig2); err == nil {
		t.Error("Overlap larger than chunk size should have failed validation")
	}
}

func TestFixedSizeStrategy_ProcessChunk(t *testing.T) {
	strategy := NewFixedSizeStrategy()
	ctx := context.Background()
	
	// Test text that should be split into multiple chunks
	text := "This is the first sentence. This is the second sentence. " +
		   "This is the third sentence. This is the fourth sentence. " +
		   "This is the fifth sentence. This is the sixth sentence. " +
		   "This is the seventh sentence. This is the eighth sentence. " +
		   "This is the ninth sentence. This is the tenth sentence."
	
	metadata := core.ChunkMetadata{
		SourcePath:  "test.txt",
		ChunkID:     "test",
		ChunkType:   "text",
		ProcessedAt: time.Now(),
		Context: map[string]string{
			"chunk_size":         "200",
			"overlap_size":       "50",
			"split_on_sentences": "true",
		},
	}
	
	chunks, err := strategy.ProcessChunk(ctx, text, metadata)
	if err != nil {
		t.Fatalf("ProcessChunk failed: %v", err)
	}
	
	if len(chunks) < 2 {
		t.Errorf("Expected multiple chunks for long text, got %d", len(chunks))
	}
	
	// Verify chunk metadata
	for _, chunk := range chunks {
		if chunk.Metadata.ChunkType != "text" {
			t.Errorf("Expected chunk type 'text', got %s", chunk.Metadata.ChunkType)
		}
		
		if chunk.Metadata.ProcessedBy != strategy.GetName() {
			t.Errorf("Expected processed by %s, got %s", strategy.GetName(), chunk.Metadata.ProcessedBy)
		}
		
		if chunk.Metadata.Quality == nil {
			t.Error("Expected quality metrics in chunk")
		}
		
		// Check chunk ID format
		if !containsSubstring(chunk.Metadata.ChunkID, "test:chunk:") {
			t.Errorf("Expected chunk ID to contain 'test:chunk:', got %s", chunk.Metadata.ChunkID)
		}
	}
}

func TestFixedSizeStrategy_SmallText(t *testing.T) {
	strategy := NewFixedSizeStrategy()
	ctx := context.Background()
	
	// Test text smaller than chunk size
	text := "This is a short text."
	
	metadata := core.ChunkMetadata{
		SourcePath:  "test.txt",
		ChunkID:     "test",
		ChunkType:   "text",
		ProcessedAt: time.Now(),
	}
	
	chunks, err := strategy.ProcessChunk(ctx, text, metadata)
	if err != nil {
		t.Fatalf("ProcessChunk failed: %v", err)
	}
	
	if len(chunks) != 1 {
		t.Errorf("Expected 1 chunk for short text, got %d", len(chunks))
	}
	
	if chunks[0].Data != text {
		t.Errorf("Expected chunk data to match input text")
	}
}

func TestFixedSizeStrategy_NonStringInput(t *testing.T) {
	strategy := NewFixedSizeStrategy()
	ctx := context.Background()
	
	metadata := core.ChunkMetadata{
		SourcePath:  "test.txt",
		ChunkID:     "test",
		ProcessedAt: time.Now(),
	}
	
	// Test with non-string input
	_, err := strategy.ProcessChunk(ctx, 123, metadata)
	if err == nil {
		t.Error("Expected error for non-string input")
	}
}

func TestFixedSizeStrategy_OptimalChunkSize(t *testing.T) {
	strategy := NewFixedSizeStrategy()
	
	// Test with unstructured schema
	unstructuredSchema := core.SchemaInfo{
		Format: "unstructured",
	}
	
	size := strategy.GetOptimalChunkSize(unstructuredSchema)
	if size != 1000 {
		t.Errorf("Expected optimal size 1000 for unstructured, got %d", size)
	}
	
	// Test with other format
	otherSchema := core.SchemaInfo{
		Format: "structured",
	}
	
	size = strategy.GetOptimalChunkSize(otherSchema)
	if size != 500 {
		t.Errorf("Expected optimal size 500 for other formats, got %d", size)
	}
}

func TestFixedSizeStrategy_ConfigureReader(t *testing.T) {
	strategy := NewFixedSizeStrategy()
	
	readerConfig := map[string]any{
		"skip_empty_lines": true,
		"encoding":         "utf-8",
	}
	
	configured, err := strategy.ConfigureReader(readerConfig)
	if err != nil {
		t.Fatalf("ConfigureReader failed: %v", err)
	}
	
	// Should override skip_empty_lines to false
	if configured["skip_empty_lines"] != false {
		t.Error("Expected skip_empty_lines to be set to false")
	}
	
	// Should preserve other settings
	if configured["encoding"] != "utf-8" {
		t.Error("Expected encoding to be preserved")
	}
}

func TestFixedSizeStrategy_QualityMetrics(t *testing.T) {
	strategy := NewFixedSizeStrategy()
	
	// Test quality calculation
	quality := strategy.calculateQuality("This is a complete sentence.")
	
	if quality.Completeness <= 0 {
		t.Error("Expected positive completeness")
	}
	
	if quality.Coherence <= 0 {
		t.Error("Expected positive coherence")
	}
	
	if quality.Readability <= 0 {
		t.Error("Expected positive readability")
	}
	
	// Test with empty text
	emptyQuality := strategy.calculateQuality("")
	if emptyQuality.Completeness != 0.0 {
		t.Error("Expected zero completeness for empty text")
	}
}

// Helper function to check if string contains substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || 
		    (len(s) > len(substr) && (s[:len(substr)] == substr || 
		     s[len(s)-len(substr):] == substr || 
		     indexOfSubstring(s, substr) >= 0)))
}

func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}