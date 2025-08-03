package text

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
	"github.com/jscharber/eAIIngest/pkg/preprocessing"
)

// TextReader implements DataSourceReader for plain text files with encoding support
type TextReader struct {
	name             string
	version          string
	encodingDetector *preprocessing.EncodingDetector
}

// NewTextReader creates a new text file reader
func NewTextReader() *TextReader {
	return &TextReader{
		name:             "text_reader",
		version:          "1.0.0",
		encodingDetector: preprocessing.NewEncodingDetector(),
	}
}

// GetConfigSpec returns the configuration specification
func (r *TextReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "encoding",
			Type:        "string",
			Required:    false,
			Default:     "auto",
			Description: "Text file encoding (auto-detect if 'auto')",
			Enum:        []string{"auto", "utf-8", "utf-16", "utf-16be", "utf-16le", "iso-8859-1", "iso-8859-15", "windows-1252", "windows-1250", "windows-1251", "windows-1253", "windows-1254", "windows-1255", "windows-1256", "shift_jis", "euc-jp", "euc-kr", "gb2312", "gbk", "gb18030", "big5"},
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
		{
			Name:        "chunk_by",
			Type:        "string",
			Required:    false,
			Default:     "line",
			Description: "How to chunk the text",
			Enum:        []string{"line", "paragraph", "fixed_size"},
		},
		{
			Name:        "chunk_size",
			Type:        "int",
			Required:    false,
			Default:     1000,
			Description: "Characters per chunk (for fixed_size mode)",
			MinValue:    ptr(100.0),
			MaxValue:    ptr(50000.0),
		},
		{
			Name:        "preserve_whitespace",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Preserve leading/trailing whitespace",
		},
		{
			Name:        "detect_structure",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Attempt to detect document structure (headers, lists, etc.)",
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *TextReader) ValidateConfig(config map[string]any) error {
	if encoding, ok := config["encoding"]; ok {
		if str, ok := encoding.(string); ok {
			validEncodings := []string{"auto", "utf-8", "utf-16", "utf-16be", "utf-16le", "iso-8859-1", "iso-8859-15", "windows-1252", "windows-1250", "windows-1251", "windows-1253", "windows-1254", "windows-1255", "windows-1256", "shift_jis", "euc-jp", "euc-kr", "gb2312", "gbk", "gb18030", "big5"}
			found := false
			for _, valid := range validEncodings {
				if str == valid {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("invalid encoding: %s", str)
			}
		}
	}

	if maxLen, ok := config["max_line_length"]; ok {
		if num, ok := maxLen.(float64); ok {
			if num < 100 || num > 100000 {
				return fmt.Errorf("max_line_length must be between 100 and 100000")
			}
		}
	}

	if chunkSize, ok := config["chunk_size"]; ok {
		if num, ok := chunkSize.(float64); ok {
			if num < 100 || num > 50000 {
				return fmt.Errorf("chunk_size must be between 100 and 50000")
			}
		}
	}

	if chunkBy, ok := config["chunk_by"]; ok {
		if str, ok := chunkBy.(string); ok {
			validModes := []string{"line", "paragraph", "fixed_size"}
			found := false
			for _, valid := range validModes {
				if str == valid {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("invalid chunk_by mode: %s", str)
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
	// Detect encoding first
	encodingInfo, err := r.encodingDetector.DetectEncoding(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to detect encoding: %w", err)
	}

	// Convert to UTF-8 if needed
	var filePath string
	if encodingInfo.Name != "utf-8" {
		filePath, err = r.encodingDetector.ConvertToUTF8(sourcePath, encodingInfo.Name)
		if err != nil {
			return core.SchemaInfo{}, fmt.Errorf("failed to convert encoding: %w", err)
		}
		defer os.Remove(filePath) // Clean up temp file
	} else {
		filePath = sourcePath
	}

	file, err := os.Open(filePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Read first few KB to analyze structure
	buffer := make([]byte, 8192)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return core.SchemaInfo{}, fmt.Errorf("failed to read file: %w", err)
	}

	content := string(buffer[:n])
	lines := strings.Split(content, "\n")

	// Analyze document structure
	docInfo := r.analyzeDocumentStructure(content)

	schema := core.SchemaInfo{
		Format:   docInfo.Format,
		Encoding: encodingInfo.Name,
		Fields: []core.FieldInfo{
			{
				Name:        "content",
				Type:        "text",
				Nullable:    false,
				Description: "Text content chunk",
			},
			{
				Name:        "chunk_type",
				Type:        "string",
				Nullable:    true,
				Description: "Type of content (header, paragraph, list_item, etc.)",
			},
			{
				Name:        "line_number",
				Type:        "integer",
				Nullable:    true,
				Description: "Starting line number for chunk",
			},
			{
				Name:        "structure_level",
				Type:        "integer",
				Nullable:    true,
				Description: "Document hierarchy level (for headers)",
			},
		},
		Metadata: map[string]any{
			"detected_encoding":   encodingInfo.Name,
			"encoding_confidence": encodingInfo.Confidence,
			"has_bom":             len(encodingInfo.BOM) > 0,
			"sample_lines":        len(lines),
			"avg_line_length":     docInfo.AvgLineLength,
			"has_headers":         docInfo.HasHeaders,
			"has_lists":           docInfo.HasLists,
			"has_code_blocks":     docInfo.HasCodeBlocks,
			"paragraph_count":     docInfo.ParagraphCount,
			"empty_line_ratio":    docInfo.EmptyLineRatio,
			"max_line_length":     docInfo.MaxLineLength,
		},
	}

	// Sample some content for analysis
	sampleData := make([]map[string]any, 0, min(len(lines), 5))
	for i, line := range lines {
		if i >= 5 {
			break
		}
		if strings.TrimSpace(line) != "" {
			chunkType := r.classifyLine(line)
			sampleData = append(sampleData, map[string]any{
				"content":         line,
				"chunk_type":      chunkType,
				"line_number":     i + 1,
				"structure_level": r.getHeaderLevel(line),
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

// DocumentInfo contains analyzed document structure information
type DocumentInfo struct {
	Format         string
	AvgLineLength  float64
	MaxLineLength  int
	HasHeaders     bool
	HasLists       bool
	HasCodeBlocks  bool
	ParagraphCount int
	EmptyLineRatio float64
}

// analyzeDocumentStructure analyzes the document structure patterns
func (r *TextReader) analyzeDocumentStructure(content string) DocumentInfo {
	lines := strings.Split(content, "\n")

	info := DocumentInfo{
		Format: "plain_text",
	}

	if len(lines) == 0 {
		return info
	}

	totalLength := 0
	emptyLines := 0
	paragraphs := 0
	inParagraph := false

	for _, line := range lines {
		lineLen := len(line)
		totalLength += lineLen

		if lineLen > info.MaxLineLength {
			info.MaxLineLength = lineLen
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			emptyLines++
			inParagraph = false
		} else {
			if !inParagraph {
				paragraphs++
				inParagraph = true
			}

			// Check for headers (# prefix or all caps short lines)
			if strings.HasPrefix(trimmed, "#") ||
				(len(trimmed) < 60 && strings.ToUpper(trimmed) == trimmed && len(trimmed) > 5) {
				info.HasHeaders = true
			}

			// Check for lists
			if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") ||
				strings.HasPrefix(trimmed, "+ ") {
				info.HasLists = true
			}

			// Check for numbered lists
			if len(trimmed) > 3 && trimmed[1] == '.' && trimmed[2] == ' ' {
				if c := trimmed[0]; c >= '0' && c <= '9' {
					info.HasLists = true
				}
			}

			// Check for code blocks (indented lines or fenced)
			if strings.HasPrefix(line, "    ") || strings.HasPrefix(trimmed, "```") ||
				strings.HasPrefix(trimmed, "~~~") {
				info.HasCodeBlocks = true
			}
		}
	}

	if len(lines) > 0 {
		info.AvgLineLength = float64(totalLength) / float64(len(lines))
		info.EmptyLineRatio = float64(emptyLines) / float64(len(lines))
	}

	info.ParagraphCount = paragraphs

	// Determine document format
	if info.HasHeaders && info.HasLists {
		info.Format = "structured_text"
	} else if info.HasCodeBlocks {
		info.Format = "code_or_markdown"
	} else if info.EmptyLineRatio < 0.1 && info.AvgLineLength > 60 {
		info.Format = "continuous_text"
	}

	return info
}

// classifyLine determines the type of a line of text
func (r *TextReader) classifyLine(line string) string {
	trimmed := strings.TrimSpace(line)

	if trimmed == "" {
		return "empty"
	}

	// Headers
	if strings.HasPrefix(trimmed, "#") {
		return "header"
	}

	// All caps short lines (potential headers)
	if len(trimmed) < 60 && strings.ToUpper(trimmed) == trimmed && len(trimmed) > 5 {
		return "header"
	}

	// Lists
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") ||
		strings.HasPrefix(trimmed, "+ ") {
		return "list_item"
	}

	// Numbered lists
	if len(trimmed) > 3 && trimmed[1] == '.' && trimmed[2] == ' ' {
		if c := trimmed[0]; c >= '0' && c <= '9' {
			return "list_item"
		}
	}

	// Code blocks
	if strings.HasPrefix(line, "    ") || strings.HasPrefix(trimmed, "```") ||
		strings.HasPrefix(trimmed, "~~~") {
		return "code"
	}

	return "paragraph"
}

// getHeaderLevel determines the header level (1-6 for markdown-style headers)
func (r *TextReader) getHeaderLevel(line string) int {
	trimmed := strings.TrimSpace(line)

	if strings.HasPrefix(trimmed, "#") {
		level := 0
		for i, c := range trimmed {
			if c == '#' && i < 6 {
				level++
			} else {
				break
			}
		}
		if level > 0 && level <= 6 {
			return level
		}
	}

	return 0
}

// CreateIterator creates a chunk iterator for the text file
func (r *TextReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	// Detect and handle encoding
	var filePath string
	var cleanupPath string

	if encoding, ok := strategyConfig["encoding"].(string); ok && encoding != "auto" {
		// Use specified encoding
		if encoding != "utf-8" {
			convertedPath, err := r.encodingDetector.ConvertToUTF8(sourcePath, encoding)
			if err != nil {
				return nil, fmt.Errorf("failed to convert from %s encoding: %w", encoding, err)
			}
			filePath = convertedPath
			cleanupPath = convertedPath
		} else {
			filePath = sourcePath
		}
	} else {
		// Auto-detect encoding
		encodingInfo, err := r.encodingDetector.DetectEncoding(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("failed to detect encoding: %w", err)
		}

		if encodingInfo.Name != "utf-8" {
			convertedPath, err := r.encodingDetector.ConvertToUTF8(sourcePath, encodingInfo.Name)
			if err != nil {
				return nil, fmt.Errorf("failed to convert from %s encoding: %w", encodingInfo.Name, err)
			}
			filePath = convertedPath
			cleanupPath = convertedPath
		} else {
			filePath = sourcePath
		}
	}

	file, err := os.Open(filePath)
	if err != nil {
		if cleanupPath != "" {
			os.Remove(cleanupPath)
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	iterator := &TextIterator{
		file:        file,
		sourcePath:  sourcePath,
		actualPath:  filePath,
		cleanupPath: cleanupPath,
		config:      strategyConfig,
		reader:      r,
		lineNumber:  0,
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
	file        *os.File
	sourcePath  string
	actualPath  string
	cleanupPath string
	config      map[string]any
	reader      *TextReader
	lineNumber  int
	totalSize   int64
	readBytes   int64
	buffer      []string
	bufferPos   int
	chunkMode   string
}

// Next returns the next chunk of text
func (it *TextIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	// Initialize on first call
	if it.totalSize == 0 {
		if stat, err := it.file.Stat(); err == nil {
			it.totalSize = stat.Size()
		}

		// Determine chunking mode
		if chunkBy, ok := it.config["chunk_by"].(string); ok {
			it.chunkMode = chunkBy
		} else {
			it.chunkMode = "line"
		}
	}

	switch it.chunkMode {
	case "paragraph":
		return it.nextParagraph(ctx)
	case "fixed_size":
		return it.nextFixedSize(ctx)
	default:
		return it.nextLine(ctx)
	}
}

// nextLine reads the next line
func (it *TextIterator) nextLine(ctx context.Context) (core.Chunk, error) {
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
			return it.nextLine(ctx) // Recursively get next non-empty line
		}
	}

	// Process whitespace
	if preserve, ok := it.config["preserve_whitespace"].(bool); !ok || !preserve {
		content = strings.TrimSpace(content)
	}

	chunkType := "text"
	structureLevel := 0

	if detect, ok := it.config["detect_structure"].(bool); !ok || detect {
		chunkType = it.reader.classifyLine(content)
		structureLevel = it.reader.getHeaderLevel(content)
	}

	chunk := core.Chunk{
		Data: content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:line:%d", filepath.Base(it.sourcePath), it.lineNumber),
			ChunkType:   "text_line",
			SizeBytes:   int64(len(content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "text_reader",
			Context: map[string]string{
				"line_number":     strconv.Itoa(it.lineNumber),
				"chunk_type":      chunkType,
				"structure_level": strconv.Itoa(structureLevel),
				"chunking_mode":   it.chunkMode,
			},
		},
	}

	return chunk, nil
}

// nextParagraph reads the next paragraph (lines until empty line)
func (it *TextIterator) nextParagraph(ctx context.Context) (core.Chunk, error) {
	var paragraphLines []string
	startLine := it.lineNumber + 1

	for {
		// Read next line
		buffer := make([]byte, 1024)
		line := strings.Builder{}

		for {
			n, err := it.file.Read(buffer)
			if n > 0 {
				it.readBytes += int64(n)
				content := string(buffer[:n])

				if idx := strings.Index(content, "\n"); idx >= 0 {
					line.WriteString(content[:idx])
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
				if line.Len() == 0 && len(paragraphLines) == 0 {
					return core.Chunk{}, core.ErrIteratorExhausted
				}
				break
			}
			if err != nil {
				return core.Chunk{}, fmt.Errorf("failed to read file: %w", err)
			}
		}

		it.lineNumber++
		currentLine := line.String()

		// Check if line is empty (end of paragraph)
		if strings.TrimSpace(currentLine) == "" {
			if len(paragraphLines) > 0 {
				break // End of paragraph
			}
			// Skip empty lines between paragraphs
			continue
		}

		paragraphLines = append(paragraphLines, currentLine)
	}

	if len(paragraphLines) == 0 {
		return core.Chunk{}, core.ErrIteratorExhausted
	}

	content := strings.Join(paragraphLines, "\n")

	chunk := core.Chunk{
		Data: content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:para:%d", filepath.Base(it.sourcePath), startLine),
			ChunkType:   "text_paragraph",
			SizeBytes:   int64(len(content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "text_reader",
			Context: map[string]string{
				"start_line":    strconv.Itoa(startLine),
				"end_line":      strconv.Itoa(it.lineNumber),
				"line_count":    strconv.Itoa(len(paragraphLines)),
				"chunking_mode": it.chunkMode,
			},
		},
	}

	return chunk, nil
}

// nextFixedSize reads a fixed number of characters
func (it *TextIterator) nextFixedSize(ctx context.Context) (core.Chunk, error) {
	chunkSize := 1000
	if size, ok := it.config["chunk_size"].(float64); ok {
		chunkSize = int(size)
	}

	buffer := make([]byte, chunkSize)
	n, err := it.file.Read(buffer)

	if n == 0 {
		if err == io.EOF {
			return core.Chunk{}, core.ErrIteratorExhausted
		}
		return core.Chunk{}, fmt.Errorf("failed to read file: %w", err)
	}

	it.readBytes += int64(n)
	content := string(buffer[:n])

	// Count lines for metadata
	lineCount := strings.Count(content, "\n")
	it.lineNumber += lineCount

	chunk := core.Chunk{
		Data: content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:chunk:%d", filepath.Base(it.sourcePath), int(it.readBytes/int64(chunkSize))),
			ChunkType:   "text_chunk",
			SizeBytes:   int64(len(content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "text_reader",
			Context: map[string]string{
				"chunk_size":    strconv.Itoa(chunkSize),
				"bytes_read":    strconv.FormatInt(it.readBytes, 10),
				"line_count":    strconv.Itoa(lineCount),
				"chunking_mode": it.chunkMode,
			},
		},
	}

	return chunk, nil
}

// Close releases file resources
func (it *TextIterator) Close() error {
	var err error
	if it.file != nil {
		err = it.file.Close()
	}

	// Clean up temporary encoding conversion file
	if it.cleanupPath != "" {
		os.Remove(it.cleanupPath)
	}

	return err
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
		it.bufferPos = 0
		it.buffer = nil
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
