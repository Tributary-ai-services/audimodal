package text

import (
	"context"
	"os"
	"testing"
)

func TestTextReader_GetConfigSpec(t *testing.T) {
	reader := NewTextReader()
	specs := reader.GetConfigSpec()
	
	if len(specs) == 0 {
		t.Error("Expected config specs, got none")
	}
	
	// Check for required config fields
	found := make(map[string]bool)
	for _, spec := range specs {
		found[spec.Name] = true
	}
	
	expectedFields := []string{"encoding", "max_line_length", "skip_empty_lines"}
	for _, field := range expectedFields {
		if !found[field] {
			t.Errorf("Expected config field %s not found", field)
		}
	}
}

func TestTextReader_ValidateConfig(t *testing.T) {
	reader := NewTextReader()
	
	// Valid config
	validConfig := map[string]any{
		"encoding":        "utf-8",
		"max_line_length": 1000.0,
		"skip_empty_lines": true,
	}
	
	if err := reader.ValidateConfig(validConfig); err != nil {
		t.Errorf("Valid config failed validation: %v", err)
	}
	
	// Invalid encoding
	invalidConfig := map[string]any{
		"encoding": "invalid-encoding",
	}
	
	if err := reader.ValidateConfig(invalidConfig); err == nil {
		t.Error("Invalid encoding should have failed validation")
	}
	
	// Invalid max_line_length
	invalidConfig2 := map[string]any{
		"max_line_length": 50.0, // Too small
	}
	
	if err := reader.ValidateConfig(invalidConfig2); err == nil {
		t.Error("Invalid max_line_length should have failed validation")
	}
}

func TestTextReader_TestConnection(t *testing.T) {
	reader := NewTextReader()
	ctx := context.Background()
	
	config := map[string]any{
		"encoding": "utf-8",
	}
	
	result := reader.TestConnection(ctx, config)
	if !result.Success {
		t.Errorf("Connection test failed: %s", result.Message)
	}
	
	if result.Latency <= 0 {
		t.Error("Expected positive latency")
	}
}

func TestTextReader_DiscoverSchema(t *testing.T) {
	reader := NewTextReader()
	ctx := context.Background()
	
	// Create temporary test file
	content := "Line 1\nLine 2\nLine 3\n"
	tmpfile, err := os.CreateTemp("", "test*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	
	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	
	schema, err := reader.DiscoverSchema(ctx, tmpfile.Name())
	if err != nil {
		t.Fatalf("Schema discovery failed: %v", err)
	}
	
	if schema.Format != "unstructured" {
		t.Errorf("Expected format 'unstructured', got %s", schema.Format)
	}
	
	if len(schema.Fields) == 0 {
		t.Error("Expected fields in schema")
	}
	
	if len(schema.SampleData) == 0 {
		t.Error("Expected sample data in schema")
	}
}

func TestTextReader_EstimateSize(t *testing.T) {
	reader := NewTextReader()
	ctx := context.Background()
	
	// Create temporary test file
	content := "Line 1\nLine 2\nLine 3\n"
	tmpfile, err := os.CreateTemp("", "test*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	
	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	
	estimate, err := reader.EstimateSize(ctx, tmpfile.Name())
	if err != nil {
		t.Fatalf("Size estimation failed: %v", err)
	}
	
	if estimate.ByteSize != int64(len(content)) {
		t.Errorf("Expected byte size %d, got %d", len(content), estimate.ByteSize)
	}
	
	if estimate.RowCount == nil || *estimate.RowCount <= 0 {
		t.Error("Expected positive row count estimate")
	}
	
	if estimate.ChunkEst <= 0 {
		t.Error("Expected positive chunk estimate")
	}
}

func TestTextReader_CreateIterator(t *testing.T) {
	reader := NewTextReader()
	ctx := context.Background()
	
	// Create temporary test file
	content := "Line 1\nLine 2\nLine 3\n"
	tmpfile, err := os.CreateTemp("", "test*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	
	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	
	config := map[string]any{
		"skip_empty_lines": true,
	}
	
	iterator, err := reader.CreateIterator(ctx, tmpfile.Name(), config)
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}
	defer iterator.Close()
	
	// Test iteration
	chunks := []string{}
	for {
		chunk, err := iterator.Next(ctx)
		if err != nil {
			break
		}
		
		if str, ok := chunk.Data.(string); ok {
			chunks = append(chunks, str)
		}
	}
	
	expectedLines := []string{"Line 1", "Line 2", "Line 3"}
	if len(chunks) != len(expectedLines) {
		t.Errorf("Expected %d chunks, got %d", len(expectedLines), len(chunks))
	}
	
	for i, expected := range expectedLines {
		if i < len(chunks) && chunks[i] != expected {
			t.Errorf("Expected chunk %d to be '%s', got '%s'", i, expected, chunks[i])
		}
	}
}

func TestTextReader_GetMethods(t *testing.T) {
	reader := NewTextReader()
	
	if reader.GetType() != "reader" {
		t.Errorf("Expected type 'reader', got %s", reader.GetType())
	}
	
	if reader.GetName() == "" {
		t.Error("Expected non-empty name")
	}
	
	if reader.GetVersion() == "" {
		t.Error("Expected non-empty version")
	}
	
	if !reader.SupportsStreaming() {
		t.Error("Text reader should support streaming")
	}
	
	formats := reader.GetSupportedFormats()
	if len(formats) == 0 {
		t.Error("Expected supported formats")
	}
	
	// Check for expected formats
	expectedFormats := []string{"txt", "text", "log", "md", "markdown"}
	for _, expected := range expectedFormats {
		found := false
		for _, format := range formats {
			if format == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected format %s not found in supported formats", expected)
		}
	}
}

func TestTextIterator_Progress(t *testing.T) {
	reader := NewTextReader()
	ctx := context.Background()
	
	// Create temporary test file
	content := "Line 1\nLine 2\nLine 3\n"
	tmpfile, err := os.CreateTemp("", "test*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	
	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	
	iterator, err := reader.CreateIterator(ctx, tmpfile.Name(), map[string]any{})
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}
	defer iterator.Close()
	
	// Initial progress should be 0
	if progress := iterator.Progress(); progress != 0.0 {
		t.Errorf("Expected initial progress 0.0, got %f", progress)
	}
	
	// Read one chunk
	_, err = iterator.Next(ctx)
	if err != nil {
		t.Fatalf("Failed to read first chunk: %v", err)
	}
	
	// Progress should be > 0
	if progress := iterator.Progress(); progress <= 0.0 {
		t.Errorf("Expected progress > 0.0 after reading, got %f", progress)
	}
}

func TestTextIterator_Reset(t *testing.T) {
	reader := NewTextReader()
	ctx := context.Background()
	
	// Create temporary test file
	content := "Line 1\nLine 2\nLine 3\n"
	tmpfile, err := os.CreateTemp("", "test*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	
	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	
	iterator, err := reader.CreateIterator(ctx, tmpfile.Name(), map[string]any{})
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}
	defer iterator.Close()
	
	// Read first chunk
	chunk1, err := iterator.Next(ctx)
	if err != nil {
		t.Fatalf("Failed to read first chunk: %v", err)
	}
	
	// Reset iterator
	if err := iterator.Reset(); err != nil {
		t.Fatalf("Failed to reset iterator: %v", err)
	}
	
	// Read first chunk again
	chunk2, err := iterator.Next(ctx)
	if err != nil {
		t.Fatalf("Failed to read chunk after reset: %v", err)
	}
	
	// Should be the same
	if chunk1.Data != chunk2.Data {
		t.Errorf("Expected same chunk after reset, got different data")
	}
}