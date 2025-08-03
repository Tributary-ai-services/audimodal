package csv

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// CSVReader implements DataSourceReader for CSV files
type CSVReader struct {
	name    string
	version string
}

// NewCSVReader creates a new CSV file reader
func NewCSVReader() *CSVReader {
	return &CSVReader{
		name:    "csv_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *CSVReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "delimiter",
			Type:        "string",
			Required:    false,
			Default:     ",",
			Description: "Field delimiter character",
			Examples:    []string{",", ";", "\t", "|"},
		},
		{
			Name:        "has_header",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Whether the first row contains column headers",
		},
		{
			Name:        "skip_rows",
			Type:        "int",
			Required:    false,
			Default:     0,
			Description: "Number of rows to skip at the beginning",
			MinValue:    ptr(0.0),
			MaxValue:    ptr(1000.0),
		},
		{
			Name:        "max_rows",
			Type:        "int",
			Required:    false,
			Default:     -1,
			Description: "Maximum number of rows to process (-1 for all)",
			MinValue:    ptr(-1.0),
		},
		{
			Name:        "encoding",
			Type:        "string",
			Required:    false,
			Default:     "utf-8",
			Description: "File encoding",
			Enum:        []string{"utf-8", "ascii", "iso-8859-1", "windows-1252"},
		},
		{
			Name:        "quote_char",
			Type:        "string",
			Required:    false,
			Default:     "\"",
			Description: "Quote character for fields",
		},
		{
			Name:        "escape_char",
			Type:        "string",
			Required:    false,
			Default:     "\"",
			Description: "Escape character",
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *CSVReader) ValidateConfig(config map[string]any) error {
	if delimiter, ok := config["delimiter"]; ok {
		if str, ok := delimiter.(string); ok {
			if len(str) != 1 {
				return fmt.Errorf("delimiter must be a single character")
			}
		}
	}

	if skipRows, ok := config["skip_rows"]; ok {
		if num, ok := skipRows.(float64); ok {
			if num < 0 || num > 1000 {
				return fmt.Errorf("skip_rows must be between 0 and 1000")
			}
		}
	}

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

	return nil
}

// TestConnection tests if the CSV file can be read
func (r *CSVReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
	start := time.Now()

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
		Message: "CSV reader configuration is valid",
		Latency: time.Since(start),
		Details: map[string]any{
			"delimiter": config["delimiter"],
			"encoding":  config["encoding"],
		},
	}
}

// GetType returns the connector type
func (r *CSVReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *CSVReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *CSVReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the CSV file structure
func (r *CSVReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Create CSV reader with default settings
	csvReader := csv.NewReader(file)
	csvReader.Comma = ','
	csvReader.TrimLeadingSpace = true

	// Read first few rows to analyze structure
	var headers []string
	var sampleRows [][]string

	for i := 0; i < 10; i++ { // Read up to 10 rows for analysis
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return core.SchemaInfo{}, fmt.Errorf("failed to read CSV: %w", err)
		}

		if i == 0 {
			headers = record
		} else {
			sampleRows = append(sampleRows, record)
		}
	}

	if len(headers) == 0 {
		return core.SchemaInfo{}, fmt.Errorf("empty CSV file")
	}

	// Build field info
	fields := make([]core.FieldInfo, len(headers))
	for i, header := range headers {
		fieldType := r.inferFieldType(sampleRows, i)
		fields[i] = core.FieldInfo{
			Name:        header,
			Type:        fieldType,
			Nullable:    true,
			Description: fmt.Sprintf("Column %d: %s", i+1, header),
		}

		// Add field statistics if we have sample data
		if len(sampleRows) > 0 {
			stats := r.calculateFieldStats(sampleRows, i)
			fields[i].Statistics = &stats
		}
	}

	schema := core.SchemaInfo{
		Format:    "structured",
		Encoding:  "utf-8",
		Delimiter: ",",
		Fields:    fields,
		Metadata: map[string]any{
			"column_count": len(headers),
			"sample_rows":  len(sampleRows),
		},
	}

	// Convert sample rows to map format
	sampleData := make([]map[string]any, 0, len(sampleRows))
	for _, row := range sampleRows {
		if len(row) >= len(headers) {
			record := make(map[string]any)
			for i, header := range headers {
				record[header] = row[i]
			}
			sampleData = append(sampleData, record)
		}
	}
	schema.SampleData = sampleData

	return schema, nil
}

// EstimateSize returns size estimates for the CSV file
func (r *CSVReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	// Quick sample to estimate row count
	file, err := os.Open(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Read first 4KB to estimate average row size
	buffer := make([]byte, 4096)
	n, _ := file.Read(buffer)
	content := string(buffer[:n])
	lines := strings.Split(content, "\n")

	var avgRowSize int64 = 100 // Default estimate
	if len(lines) > 1 {
		avgRowSize = int64(n) / int64(len(lines))
	}

	estimatedRows := stat.Size() / avgRowSize
	columnCount := 0
	if len(lines) > 0 {
		columnCount = len(strings.Split(lines[0], ","))
	}

	// Estimate processing complexity
	complexity := "low"
	if stat.Size() > 50*1024*1024 || columnCount > 50 { // > 50MB or > 50 columns
		complexity = "medium"
	}
	if stat.Size() > 500*1024*1024 || columnCount > 200 { // > 500MB or > 200 columns
		complexity = "high"
	}

	// Estimate chunks (one chunk per row typically)
	estimatedChunks := int(estimatedRows)
	if estimatedChunks > 100000 {
		estimatedChunks = int(estimatedRows / 100) // Group rows for very large files
	}

	processTime := "fast"
	if estimatedRows > 100000 {
		processTime = "medium"
	}
	if estimatedRows > 1000000 {
		processTime = "slow"
	}

	return core.SizeEstimate{
		RowCount:    &estimatedRows,
		ByteSize:    stat.Size(),
		ColumnCount: &columnCount,
		Complexity:  complexity,
		ChunkEst:    int(estimatedChunks),
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the CSV file
func (r *CSVReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	csvReader := csv.NewReader(file)

	// Configure CSV reader based on strategy config
	if delimiter, ok := strategyConfig["delimiter"].(string); ok && len(delimiter) == 1 {
		csvReader.Comma = rune(delimiter[0])
	}

	csvReader.TrimLeadingSpace = true
	csvReader.LazyQuotes = true

	iterator := &CSVIterator{
		file:       file,
		csvReader:  csvReader,
		sourcePath: sourcePath,
		config:     strategyConfig,
		rowNumber:  0,
		hasHeader:  true,
	}

	// Check if we should read headers
	if hasHeader, ok := strategyConfig["has_header"].(bool); ok {
		iterator.hasHeader = hasHeader
	}

	// Read headers if present
	if iterator.hasHeader {
		headers, err := csvReader.Read()
		if err != nil && err != io.EOF {
			file.Close()
			return nil, fmt.Errorf("failed to read headers: %w", err)
		}
		iterator.headers = headers
	}

	return iterator, nil
}

// SupportsStreaming indicates CSV reader supports streaming
func (r *CSVReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *CSVReader) GetSupportedFormats() []string {
	return []string{"csv", "tsv", "txt"}
}

// Helper methods for schema discovery

func (r *CSVReader) inferFieldType(rows [][]string, columnIndex int) string {
	if len(rows) == 0 {
		return "string"
	}

	intCount := 0
	floatCount := 0
	totalCount := 0

	for _, row := range rows {
		if columnIndex >= len(row) {
			continue
		}

		value := strings.TrimSpace(row[columnIndex])
		if value == "" {
			continue
		}

		totalCount++

		// Try to parse as int
		if _, err := strconv.Atoi(value); err == nil {
			intCount++
			continue
		}

		// Try to parse as float
		if _, err := strconv.ParseFloat(value, 64); err == nil {
			floatCount++
			continue
		}
	}

	if totalCount == 0 {
		return "string"
	}

	// If 80% or more are integers
	if float64(intCount)/float64(totalCount) >= 0.8 {
		return "integer"
	}

	// If 80% or more are numbers (int + float)
	if float64(intCount+floatCount)/float64(totalCount) >= 0.8 {
		return "float"
	}

	return "string"
}

func (r *CSVReader) calculateFieldStats(rows [][]string, columnIndex int) core.FieldStats {
	stats := core.FieldStats{}

	values := make([]string, 0)
	for _, row := range rows {
		if columnIndex < len(row) {
			value := strings.TrimSpace(row[columnIndex])
			if value != "" {
				values = append(values, value)
			}
		}
	}

	stats.Count = int64(len(values))
	if stats.Count == 0 {
		return stats
	}

	// Count distinct values
	distinct := make(map[string]bool)
	for _, v := range values {
		distinct[v] = true
	}
	stats.Distinct = int64(len(distinct))

	// Calculate null percentage
	totalRows := int64(len(rows))
	stats.NullPercent = float64(totalRows-stats.Count) / float64(totalRows) * 100

	// Find top values (up to 5)
	valueCounts := make(map[string]int)
	for _, v := range values {
		valueCounts[v]++
	}

	topValues := make([]any, 0, 5)
	for value, count := range valueCounts {
		if len(topValues) < 5 {
			topValues = append(topValues, map[string]any{
				"value": value,
				"count": count,
			})
		}
	}
	stats.TopValues = topValues

	return stats
}

// CSVIterator implements ChunkIterator for CSV files
type CSVIterator struct {
	file       *os.File
	csvReader  *csv.Reader
	sourcePath string
	config     map[string]any
	headers    []string
	rowNumber  int
	hasHeader  bool
	totalSize  int64
	readBytes  int64
}

// Next returns the next CSV row as a chunk
func (it *CSVIterator) Next(ctx context.Context) (core.Chunk, error) {
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

	record, err := it.csvReader.Read()
	if err == io.EOF {
		return core.Chunk{}, core.ErrIteratorExhausted
	}
	if err != nil {
		return core.Chunk{}, fmt.Errorf("failed to read CSV record: %w", err)
	}

	it.rowNumber++

	// Update read bytes estimate
	rowSize := 0
	for _, field := range record {
		rowSize += len(field) + 1 // +1 for delimiter
	}
	it.readBytes += int64(rowSize)

	// Create data structure
	var data any
	if it.hasHeader && len(it.headers) > 0 {
		// Create map with headers as keys
		rowData := make(map[string]any)
		for i, header := range it.headers {
			if i < len(record) {
				rowData[header] = record[i]
			}
		}
		data = rowData
	} else {
		// Use array format
		data = record
	}

	chunk := core.Chunk{
		Data: data,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:row:%d", filepath.Base(it.sourcePath), it.rowNumber),
			ChunkType:   "structured",
			SizeBytes:   int64(rowSize),
			ProcessedAt: time.Now(),
			ProcessedBy: "csv_reader",
			Context: map[string]string{
				"row_number":   fmt.Sprintf("%d", it.rowNumber),
				"column_count": fmt.Sprintf("%d", len(record)),
				"file_type":    "csv",
			},
			SchemaInfo: map[string]any{
				"headers": it.headers,
				"row":     record,
			},
		},
	}

	return chunk, nil
}

// Close releases file resources
func (it *CSVIterator) Close() error {
	if it.file != nil {
		return it.file.Close()
	}
	return nil
}

// Reset restarts iteration from the beginning
func (it *CSVIterator) Reset() error {
	if it.file != nil {
		_, err := it.file.Seek(0, io.SeekStart)
		if err != nil {
			return err
		}

		// Recreate CSV reader
		it.csvReader = csv.NewReader(it.file)
		if delimiter, ok := it.config["delimiter"].(string); ok && len(delimiter) == 1 {
			it.csvReader.Comma = rune(delimiter[0])
		}
		it.csvReader.TrimLeadingSpace = true
		it.csvReader.LazyQuotes = true

		it.rowNumber = 0
		it.readBytes = 0

		// Re-read headers if needed
		if it.hasHeader {
			headers, err := it.csvReader.Read()
			if err != nil && err != io.EOF {
				return fmt.Errorf("failed to re-read headers: %w", err)
			}
			it.headers = headers
		}

		return nil
	}
	return fmt.Errorf("file not open")
}

// Progress returns iteration progress
func (it *CSVIterator) Progress() float64 {
	if it.totalSize == 0 {
		return 0.0
	}
	return float64(it.readBytes) / float64(it.totalSize)
}

// Helper function
func ptr(f float64) *float64 {
	return &f
}
