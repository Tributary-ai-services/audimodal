package json

import (
	"context"
	"os"
	"testing"
)

func TestJSONReader_Basic(t *testing.T) {
	reader := NewJSONReader()

	// Test basic properties
	if reader.GetType() != "reader" {
		t.Errorf("Expected type 'reader', got %s", reader.GetType())
	}

	if reader.GetName() != "json_reader" {
		t.Errorf("Expected name 'json_reader', got %s", reader.GetName())
	}

	if !reader.SupportsStreaming() {
		t.Error("JSON reader should support streaming")
	}

	formats := reader.GetSupportedFormats()
	expectedFormats := []string{"json", "jsonl", "ndjson"}
	if len(formats) != len(expectedFormats) {
		t.Errorf("Expected %d formats, got %d", len(expectedFormats), len(formats))
	}
}

func TestJSONReader_ConfigValidation(t *testing.T) {
	reader := NewJSONReader()

	// Valid config
	validConfig := map[string]any{
		"format":          "auto",
		"max_object_size": 1048576.0,
		"encoding":        "utf-8",
		"strict_mode":     false,
		"flatten_nested":  false,
	}

	if err := reader.ValidateConfig(validConfig); err != nil {
		t.Errorf("Valid config failed validation: %v", err)
	}

	// Invalid format
	invalidConfig := map[string]any{
		"format": "invalid-format",
	}

	if err := reader.ValidateConfig(invalidConfig); err == nil {
		t.Error("Invalid format should have failed validation")
	}
}

func TestJSONReader_DetectFormat(t *testing.T) {
	reader := NewJSONReader()

	// Test single object detection
	singleJSON := `{"name": "John", "age": 25}`
	format := reader.detectJSONFormat(singleJSON)
	if format != "single" {
		t.Errorf("Expected 'single' format, got %s", format)
	}

	// Test array detection
	arrayJSON := `[{"name": "John"}, {"name": "Jane"}]`
	format = reader.detectJSONFormat(arrayJSON)
	if format != "array" {
		t.Errorf("Expected 'array' format, got %s", format)
	}

	// Test lines detection
	linesJSON := `{"name": "John"}
{"name": "Jane"}`
	format = reader.detectJSONFormat(linesJSON)
	if format != "lines" {
		t.Errorf("Expected 'lines' format, got %s", format)
	}
}

func TestJSONReader_DiscoverSchema_SingleObject(t *testing.T) {
	reader := NewJSONReader()
	ctx := context.Background()

	// Create temporary JSON file with single object
	content := `{"name": "John", "age": 25, "active": true}`
	tmpfile, err := os.CreateTemp("", "test*.json")
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

	if schema.Format != "semi_structured" {
		t.Errorf("Expected format 'semi_structured', got %s", schema.Format)
	}

	if len(schema.Fields) == 0 {
		t.Error("Expected fields in schema")
	}

	// Check for expected fields
	fieldNames := make(map[string]bool)
	for _, field := range schema.Fields {
		fieldNames[field.Name] = true
	}

	expectedFields := []string{"name", "age", "active"}
	for _, expected := range expectedFields {
		if !fieldNames[expected] {
			t.Errorf("Expected field '%s' not found", expected)
		}
	}
}

func TestJSONReader_CreateIterator_Array(t *testing.T) {
	reader := NewJSONReader()
	ctx := context.Background()

	// Create temporary JSON file with array
	content := `[{"name": "John", "age": 25}, {"name": "Jane", "age": 30}]`
	tmpfile, err := os.CreateTemp("", "test*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	config := map[string]any{
		"format": "array",
	}

	iterator, err := reader.CreateIterator(ctx, tmpfile.Name(), config)
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}
	defer iterator.Close()

	// Test iteration
	chunks := []map[string]any{}
	for {
		chunk, err := iterator.Next(ctx)
		if err != nil {
			break
		}

		if data, ok := chunk.Data.(map[string]any); ok {
			chunks = append(chunks, data)
		}
	}

	if len(chunks) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(chunks))
	}

	// Check first object
	if len(chunks) > 0 {
		firstObj := chunks[0]
		if firstObj["name"] != "John" {
			t.Errorf("Expected name 'John', got %v", firstObj["name"])
		}
		if firstObj["age"] != float64(25) { // JSON numbers are float64
			t.Errorf("Expected age 25, got %v", firstObj["age"])
		}
	}
}

func TestJSONReader_CreateIterator_Lines(t *testing.T) {
	reader := NewJSONReader()
	ctx := context.Background()

	// Create temporary JSONL file
	content := `{"name": "John", "age": 25}
{"name": "Jane", "age": 30}
{"name": "Bob", "age": 35}`
	tmpfile, err := os.CreateTemp("", "test*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	config := map[string]any{
		"format": "lines",
	}

	iterator, err := reader.CreateIterator(ctx, tmpfile.Name(), config)
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}
	defer iterator.Close()

	// Test iteration
	chunks := []map[string]any{}
	for {
		chunk, err := iterator.Next(ctx)
		if err != nil {
			break
		}

		if data, ok := chunk.Data.(map[string]any); ok {
			chunks = append(chunks, data)
		}
	}

	if len(chunks) != 3 {
		t.Errorf("Expected 3 objects, got %d", len(chunks))
	}

	// Check objects
	expectedNames := []string{"John", "Jane", "Bob"}
	for i, chunk := range chunks {
		if i < len(expectedNames) && chunk["name"] != expectedNames[i] {
			t.Errorf("Expected name '%s' at position %d, got %v", expectedNames[i], i, chunk["name"])
		}
	}
}

func TestJSONReader_InferFieldType(t *testing.T) {
	reader := NewJSONReader()

	tests := []struct {
		value    any
		expected string
	}{
		{nil, "null"},
		{true, "boolean"},
		{false, "boolean"},
		{42, "integer"},
		{int64(42), "integer"},
		{3.14, "float"},
		{"hello", "string"},
		{[]any{1, 2, 3}, "array"},
		{map[string]any{"key": "value"}, "object"},
	}

	for _, test := range tests {
		result := reader.inferJSONFieldType(test.value)
		if result != test.expected {
			t.Errorf("For value %v, expected type %s, got %s", test.value, test.expected, result)
		}
	}
}
