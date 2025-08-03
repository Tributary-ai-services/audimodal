package readers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jscharber/eAIIngest/pkg/core"
	"github.com/jscharber/eAIIngest/pkg/readers/csv"
	"github.com/jscharber/eAIIngest/pkg/readers/json"
	"github.com/jscharber/eAIIngest/pkg/readers/text"
	"github.com/jscharber/eAIIngest/pkg/registry"
)

func TestBasicReadersIntegration(t *testing.T) {
	// Create a new registry for testing
	testRegistry := registry.NewRegistry()

	// Register readers with test registry
	testRegistry.RegisterReader("text", func() core.DataSourceReader {
		return text.NewTextReader()
	})
	testRegistry.RegisterReader("csv", func() core.DataSourceReader {
		return csv.NewCSVReader()
	})
	testRegistry.RegisterReader("json", func() core.DataSourceReader {
		return json.NewJSONReader()
	})

	// Test that all readers are registered
	readers := testRegistry.ListReaders()
	expectedReaders := []string{"csv", "json", "text"}

	if len(readers) != len(expectedReaders) {
		t.Errorf("Expected %d readers, got %d", len(expectedReaders), len(readers))
	}

	for _, expected := range expectedReaders {
		found := false
		for _, reader := range readers {
			if reader == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected reader '%s' not found", expected)
		}
	}

	// Test reader retrieval and validation
	for _, readerName := range expectedReaders {
		reader, err := testRegistry.GetReader(readerName)
		if err != nil {
			t.Errorf("Failed to get reader '%s': %v", readerName, err)
			continue
		}

		// Validate reader implements interface correctly
		if err := testRegistry.ValidatePlugin("reader", readerName); err != nil {
			t.Errorf("Reader '%s' failed validation: %v", readerName, err)
		}

		// Test basic methods
		if reader.GetName() == "" {
			t.Errorf("Reader '%s' has empty name", readerName)
		}

		if reader.GetVersion() == "" {
			t.Errorf("Reader '%s' has empty version", readerName)
		}

		if reader.GetType() != "reader" {
			t.Errorf("Reader '%s' has wrong type: %s", readerName, reader.GetType())
		}

		if len(reader.GetSupportedFormats()) == 0 {
			t.Errorf("Reader '%s' has no supported formats", readerName)
		}

		if len(reader.GetConfigSpec()) == 0 {
			t.Errorf("Reader '%s' has no config spec", readerName)
		}
	}
}

func TestReaderFormatMapping(t *testing.T) {
	tests := []struct {
		extension string
		expected  string
	}{
		{".txt", "text"},
		{".text", "text"},
		{".log", "text"},
		{".md", "markdown"},
		{".markdown", "markdown"},
		{".csv", "csv"},
		{".tsv", "csv"},
		{".json", "json"},
		{".jsonl", "json"},
		{".ndjson", "json"},
		{".unknown", ""},
	}

	for _, test := range tests {
		result := GetReaderForExtension(test.extension)
		if result != test.expected {
			t.Errorf("For extension %s, expected reader %s, got %s", test.extension, test.expected, result)
		}
	}
}

func TestEndToEndFileProcessing(t *testing.T) {
	// Register readers
	RegisterBasicReaders()

	ctx := context.Background()

	// Test data for different file types
	testFiles := []struct {
		name     string
		content  string
		reader   string
		expected int // expected number of chunks
	}{
		{
			name:     "test.txt",
			content:  "Line 1\nLine 2\nLine 3\n",
			reader:   "text",
			expected: 3,
		},
		{
			name:     "test.csv",
			content:  "name,age\nJohn,25\nJane,30\n",
			reader:   "csv",
			expected: 2, // Data rows only (header not counted as chunk)
		},
		{
			name:     "test.json",
			content:  `[{"name": "John"}, {"name": "Jane"}]`,
			reader:   "json",
			expected: 2,
		},
	}

	for _, testFile := range testFiles {
		t.Run(testFile.name, func(t *testing.T) {
			// Create temporary file
			tmpfile, err := os.CreateTemp("", testFile.name)
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpfile.Name())

			if _, err := tmpfile.Write([]byte(testFile.content)); err != nil {
				t.Fatal(err)
			}
			tmpfile.Close()

			// Get appropriate reader
			ext := filepath.Ext(testFile.name)
			readerName := GetReaderForExtension(ext)
			if readerName != testFile.reader {
				t.Errorf("Expected reader %s for %s, got %s", testFile.reader, ext, readerName)
			}

			// Get reader instance
			reader, err := registry.GlobalRegistry.GetReader(readerName)
			if err != nil {
				t.Fatalf("Failed to get reader %s: %v", readerName, err)
			}

			// Test schema discovery
			schema, err := reader.DiscoverSchema(ctx, tmpfile.Name())
			if err != nil {
				t.Errorf("Schema discovery failed for %s: %v", testFile.name, err)
			} else {
				if len(schema.Fields) == 0 {
					t.Errorf("No fields discovered for %s", testFile.name)
				}
			}

			// Test size estimation
			estimate, err := reader.EstimateSize(ctx, tmpfile.Name())
			if err != nil {
				t.Errorf("Size estimation failed for %s: %v", testFile.name, err)
			} else {
				if estimate.ByteSize <= 0 {
					t.Errorf("Invalid byte size for %s: %d", testFile.name, estimate.ByteSize)
				}
			}

			// Test iteration
			iterator, err := reader.CreateIterator(ctx, tmpfile.Name(), map[string]any{})
			if err != nil {
				t.Fatalf("Failed to create iterator for %s: %v", testFile.name, err)
			}
			defer iterator.Close()

			chunkCount := 0
			for {
				chunk, err := iterator.Next(ctx)
				if err != nil {
					break
				}
				chunkCount++

				// Validate chunk metadata
				if chunk.Metadata.SourcePath != tmpfile.Name() {
					t.Errorf("Wrong source path in chunk metadata")
				}
				if chunk.Metadata.ChunkID == "" {
					t.Errorf("Empty chunk ID")
				}
				if chunk.Metadata.ProcessedBy == "" {
					t.Errorf("Empty processed by field")
				}
				if chunk.Data == nil {
					t.Errorf("Nil chunk data")
				}
			}

			if chunkCount != testFile.expected {
				t.Errorf("Expected %d chunks for %s, got %d", testFile.expected, testFile.name, chunkCount)
			}
		})
	}
}

func TestReaderValidation(t *testing.T) {
	// Register readers
	RegisterBasicReaders()

	// Validate all readers
	if err := ValidateBasicReaders(); err != nil {
		t.Errorf("Reader validation failed: %v", err)
	}
}

func TestRegistryStats(t *testing.T) {
	// Register readers
	RegisterBasicReaders()

	stats := registry.GlobalRegistry.GetStats()

	if stats.ReaderCount < 3 {
		t.Errorf("Expected at least 3 readers, got %d", stats.ReaderCount)
	}

	if stats.TotalPlugins < 3 {
		t.Errorf("Expected at least 3 total plugins, got %d", stats.TotalPlugins)
	}

	expectedReaders := []string{"csv", "json", "text"}
	for _, expected := range expectedReaders {
		found := false
		for _, name := range stats.ReaderNames {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected reader '%s' not found in stats", expected)
		}
	}
}
