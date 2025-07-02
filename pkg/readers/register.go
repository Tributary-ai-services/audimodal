package readers

import (
	"github.com/jscharber/eAIIngest/pkg/core"
	"github.com/jscharber/eAIIngest/pkg/readers/csv"
	"github.com/jscharber/eAIIngest/pkg/readers/json"
	"github.com/jscharber/eAIIngest/pkg/readers/text"
	"github.com/jscharber/eAIIngest/pkg/registry"
)

// RegisterBasicReaders registers all basic file readers with the global registry
func RegisterBasicReaders() {
	// Register text reader
	registry.GlobalRegistry.RegisterReader("text", func() core.DataSourceReader {
		return text.NewTextReader()
	})

	// Register CSV reader
	registry.GlobalRegistry.RegisterReader("csv", func() core.DataSourceReader {
		return csv.NewCSVReader()
	})

	// Register JSON reader
	registry.GlobalRegistry.RegisterReader("json", func() core.DataSourceReader {
		return json.NewJSONReader()
	})
}

// GetReaderForExtension returns the appropriate reader name for a file extension
func GetReaderForExtension(extension string) string {
	switch extension {
	case ".txt", ".text", ".log", ".md", ".markdown":
		return "text"
	case ".csv", ".tsv":
		return "csv"
	case ".json", ".jsonl", ".ndjson":
		return "json"
	default:
		return ""
	}
}

// GetSupportedExtensions returns all file extensions supported by the basic readers
func GetSupportedExtensions() []string {
	return []string{
		// Text files
		".txt", ".text", ".log", ".md", ".markdown",
		// CSV files
		".csv", ".tsv",
		// JSON files
		".json", ".jsonl", ".ndjson",
	}
}

// ValidateBasicReaders validates all registered basic readers
func ValidateBasicReaders() error {
	readers := []string{"text", "csv", "json"}
	
	for _, name := range readers {
		if err := registry.GlobalRegistry.ValidatePlugin("reader", name); err != nil {
			return err
		}
	}
	
	return nil
}