package text

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// TextReader implements DataSourceReader for plain text files
type TextReader struct {
	name    string
	version string
}

// NewTextReader creates a new text file reader
func NewTextReader() *TextReader {
	return &TextReader{
		name:    "text_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *TextReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "encoding",
			Type:        "string",
			Required:    false,
			Default:     "utf-8",
			Description: "Text file encoding",
			Enum:        []string{"utf-8", "ascii", "iso-8859-1", "windows-1252"},
		},
		{
			Name:        "max_line_length",
			Type:        "int",
			Required:    false,
			Default:     10000,
			Description: "Maximum line length to process",
			MinValue:    ptr(100.0),
			MaxValue:    ptr(100000.0),
		},
		{
			Name:        "skip_empty_lines",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Whether to skip empty lines",
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *TextReader) ValidateConfig(config map[string]any) error {
	if encoding, ok := config["encoding"]; ok {
		if str, ok := encoding.(string); ok {
			validEncodings := []string{"utf-8", "ascii", "iso-8859-1", "windows-1252"}
			for _, valid := range validEncodings {
				if str == valid {
					goto encodingOK
				}
			}
			return fmt.Errorf("invalid encoding: %s", str)
		encodingOK:
		}
	}

	if maxLen, ok := config["max_line_length"]; ok {
		if num, ok := maxLen.(float64); ok {
			if num < 100 || num > 100000 {
				return fmt.Errorf("max_line_length must be between 100 and 100000")
			}
		}
	}

	return nil
}

// TestConnection tests if the file can be read
func (r *TextReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
	start := time.Now()

	// For text reader, we don't have a persistent connection to test
	// We'll just validate the config and return success
	err := r.ValidateConfig(config)
	if err != nil {
		return core.ConnectionTestResult{
			Success: false,
			Message: "Configuration validation failed",
			Latency: time.Since(start),
			Errors:  []string{err.Error()},
		}
	}

	return core.ConnectionTestResult{
		Success: true,
		Message: "Text reader configuration is valid",
		Latency: time.Since(start),
		Details: map[string]any{
			"encoding": config["encoding"],
		},
	}
}

// GetType returns the connector type
func (r *TextReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *TextReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *TextReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the text file structure
func (r *TextReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Read first few lines to analyze structure
	buffer := make([]byte, 4096)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return core.SchemaInfo{}, fmt.Errorf("failed to read file: %w", err)
	}

	content := string(buffer[:n])
	lines := strings.Split(content, "\n")

	schema := core.SchemaInfo{
		Format:   "unstructured",
		Encoding: "utf-8",
		Fields: []core.FieldInfo{
			{
				Name:        "content",
				Type:        "text",
				Nullable:    false,
				Description: "Text content",
			},
			{
				Name:        "line_number",
				Type:        "integer",
				Nullable:    false,
				Description: "Line number in source file",
			},
		},
		Metadata: map[string]any{
			"sample_lines": len(lines),
			"avg_line_length": func() float64 {
				if len(lines) == 0 {
					return 0
				}
				total := 0
				for _, line := range lines {
					total += len(line)
				}
				return float64(total) / float64(len(lines))
			}(),
		},
	}

	// Sample some lines for analysis
	sampleData := make([]map[string]any, 0, min(len(lines), 5))
	for i, line := range lines {
		if i >= 5 {
			break
		}
		if strings.TrimSpace(line) != "" {
			sampleData = append(sampleData, map[string]any{
				"content":     line,
				"line_number": i + 1,
			})
		}
	}
	schema.SampleData = sampleData

	return schema, nil
}

// EstimateSize returns size estimates for the text file
func (r *TextReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	// Estimate line count based on average line length (assume ~80 chars per line)
	avgLineLength := int64(80)
	estimatedLines := stat.Size() / avgLineLength
	if estimatedLines == 0 && stat.Size() > 0 {
		estimatedLines = 1 // At least 1 line for non-empty files
	}

	// Estimate chunks based on common chunk size (1000 characters)
	chunkSize := int64(1000)
	estimatedChunks := int((stat.Size() + chunkSize - 1) / chunkSize)

	complexity := "low"
	if stat.Size() > 10*1024*1024 { // > 10MB
		complexity = "medium"
	}
	if stat.Size() > 100*1024*1024 { // > 100MB
		complexity = "high"
	}

	processTime := "fast"
	if stat.Size() > 50*1024*1024 { // > 50MB
		processTime = "medium"
	}
	if stat.Size() > 200*1024*1024 { // > 200MB
		processTime = "slow"
	}

	return core.SizeEstimate{
		RowCount:    &estimatedLines,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the text file
func (r *TextReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	iterator := &TextIterator{
		file:       file,
		sourcePath: sourcePath,
		config:     strategyConfig,
		lineNumber: 0,
	}

	return iterator, nil
}

// SupportsStreaming indicates text reader supports streaming
func (r *TextReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *TextReader) GetSupportedFormats() []string {
	return []string{"txt", "text", "log", "md", "markdown"}
}

// TextIterator implements ChunkIterator for text files
type TextIterator struct {
	file       *os.File
	sourcePath string
	config     map[string]any
	lineNumber int
	totalSize  int64
	readBytes  int64
	buffer     []string
	bufferPos  int
}

// Next returns the next chunk of text
func (it *TextIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	// Initialize total size on first call
	if it.totalSize == 0 {
		if stat, err := it.file.Stat(); err == nil {
			it.totalSize = stat.Size()
		}
	}

	// Read next line
	buffer := make([]byte, 1024)
	line := strings.Builder{}
	
	for {
		n, err := it.file.Read(buffer)
		if n > 0 {
			it.readBytes += int64(n)
			content := string(buffer[:n])
			
			// Look for newline
			if idx := strings.Index(content, "\n"); idx >= 0 {
				line.WriteString(content[:idx])
				// Seek back to after the newline
				remaining := int64(len(content) - idx - 1)
				if remaining > 0 {
					it.file.Seek(-remaining, io.SeekCurrent)
					it.readBytes -= remaining
				}
				break
			} else {
				line.WriteString(content)
			}
		}
		
		if err == io.EOF {
			if line.Len() == 0 {
				return core.Chunk{}, core.ErrIteratorExhausted
			}
			break
		}
		if err != nil {
			return core.Chunk{}, fmt.Errorf("failed to read file: %w", err)
		}
	}

	it.lineNumber++
	content := line.String()

	// Skip empty lines if configured
	if skipEmpty, ok := it.config["skip_empty_lines"].(bool); ok && skipEmpty {
		if strings.TrimSpace(content) == "" {
			return it.Next(ctx) // Recursively get next non-empty line
		}
	}

	chunk := core.Chunk{
		Data: content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:line:%d", filepath.Base(it.sourcePath), it.lineNumber),
			ChunkType:   "text",
			SizeBytes:   int64(len(content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "text_reader",
			Context: map[string]string{
				"line_number": fmt.Sprintf("%d", it.lineNumber),
				"file_type":   "text",
			},
		},
	}

	return chunk, nil
}

// Close releases file resources
func (it *TextIterator) Close() error {
	if it.file != nil {
		return it.file.Close()
	}
	return nil
}

// Reset restarts iteration from the beginning
func (it *TextIterator) Reset() error {
	if it.file != nil {
		_, err := it.file.Seek(0, io.SeekStart)
		if err != nil {
			return err
		}
		it.lineNumber = 0
		it.readBytes = 0
		return nil
	}
	return fmt.Errorf("file not open")
}

// Progress returns iteration progress
func (it *TextIterator) Progress() float64 {
	if it.totalSize == 0 {
		return 0.0
	}
	return float64(it.readBytes) / float64(it.totalSize)
}

// Helper functions
func ptr(f float64) *float64 {
	return &f
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}