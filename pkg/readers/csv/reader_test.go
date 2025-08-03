package csv

import (
	"context"
	"os"
	"testing"
)

func TestCSVReader_Basic(t *testing.T) {
	reader := NewCSVReader()

	// Test basic properties
	if reader.GetType() != "reader" {
		t.Errorf("Expected type 'reader', got %s", reader.GetType())
	}

	if reader.GetName() != "csv_reader" {
		t.Errorf("Expected name 'csv_reader', got %s", reader.GetName())
	}

	if !reader.SupportsStreaming() {
		t.Error("CSV reader should support streaming")
	}

	formats := reader.GetSupportedFormats()
	expectedFormats := []string{"csv", "tsv", "txt"}
	if len(formats) != len(expectedFormats) {
		t.Errorf("Expected %d formats, got %d", len(expectedFormats), len(formats))
	}
}

func TestCSVReader_ConfigValidation(t *testing.T) {
	reader := NewCSVReader()

	// Valid config
	validConfig := map[string]any{
		"delimiter":  ",",
		"has_header": true,
		"skip_rows":  0.0,
		"encoding":   "utf-8",
	}

	if err := reader.ValidateConfig(validConfig); err != nil {
		t.Errorf("Valid config failed validation: %v", err)
	}

	// Invalid delimiter (too long)
	invalidConfig := map[string]any{
		"delimiter": ",,",
	}

	if err := reader.ValidateConfig(invalidConfig); err == nil {
		t.Error("Invalid delimiter should have failed validation")
	}
}

func TestCSVReader_DiscoverSchema(t *testing.T) {
	reader := NewCSVReader()
	ctx := context.Background()

	// Create temporary CSV file
	content := "name,age,city\nJohn,25,NYC\nJane,30,LA\nBob,35,Chicago\n"
	tmpfile, err := os.CreateTemp("", "test*.csv")
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

	if schema.Format != "structured" {
		t.Errorf("Expected format 'structured', got %s", schema.Format)
	}

	expectedFields := []string{"name", "age", "city"}
	if len(schema.Fields) != len(expectedFields) {
		t.Errorf("Expected %d fields, got %d", len(expectedFields), len(schema.Fields))
	}

	for i, field := range schema.Fields {
		if i < len(expectedFields) && field.Name != expectedFields[i] {
			t.Errorf("Expected field %d to be '%s', got '%s'", i, expectedFields[i], field.Name)
		}
	}
}

func TestCSVReader_CreateIterator(t *testing.T) {
	reader := NewCSVReader()
	ctx := context.Background()

	// Create temporary CSV file
	content := "name,age,city\nJohn,25,NYC\nJane,30,LA\n"
	tmpfile, err := os.CreateTemp("", "test*.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	config := map[string]any{
		"has_header": true,
		"delimiter":  ",",
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
		t.Errorf("Expected 2 data rows, got %d", len(chunks))
	}

	// Check first row
	if len(chunks) > 0 {
		firstRow := chunks[0]
		if firstRow["name"] != "John" {
			t.Errorf("Expected name 'John', got %v", firstRow["name"])
		}
		if firstRow["age"] != "25" {
			t.Errorf("Expected age '25', got %v", firstRow["age"])
		}
		if firstRow["city"] != "NYC" {
			t.Errorf("Expected city 'NYC', got %v", firstRow["city"])
		}
	}
}
