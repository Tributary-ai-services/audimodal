package structured

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// RowBasedStrategy implements ChunkingStrategy for structured data (CSV, database tables)
type RowBasedStrategy struct {
	name    string
	version string
}

// NewRowBasedStrategy creates a new row-based chunking strategy
func NewRowBasedStrategy() *RowBasedStrategy {
	return &RowBasedStrategy{
		name:    "row_based_structured",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (s *RowBasedStrategy) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "rows_per_chunk",
			Type:        "int",
			Required:    false,
			Default:     100,
			Description: "Number of rows to include in each chunk",
			MinValue:    ptr(1.0),
			MaxValue:    ptr(10000.0),
		},
		{
			Name:        "include_headers",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Whether to include column headers in each chunk",
		},
		{
			Name:        "preserve_relationships",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Whether to try to keep related rows together",
		},
		{
			Name:        "group_by_column",
			Type:        "string",
			Required:    false,
			Default:     "",
			Description: "Column name to group rows by (keeps groups together)",
		},
		{
			Name:        "max_chunk_size_mb",
			Type:        "int",
			Required:    false,
			Default:     10,
			Description: "Maximum chunk size in megabytes",
			MinValue:    ptr(1.0),
			MaxValue:    ptr(100.0),
		},
		{
			Name:        "output_format",
			Type:        "string",
			Required:    false,
			Default:     "records",
			Description: "Output format for chunks",
			Enum:        []string{"records", "table", "json"},
		},
	}
}

// ValidateConfig validates the provided configuration
func (s *RowBasedStrategy) ValidateConfig(config map[string]any) error {
	if rpc, ok := config["rows_per_chunk"]; ok {
		if rows, ok := rpc.(float64); ok {
			if rows < 1 || rows > 10000 {
				return fmt.Errorf("rows_per_chunk must be between 1 and 10000")
			}
		}
	}

	if mcs, ok := config["max_chunk_size_mb"]; ok {
		if size, ok := mcs.(float64); ok {
			if size < 1 || size > 100 {
				return fmt.Errorf("max_chunk_size_mb must be between 1 and 100")
			}
		}
	}

	if of, ok := config["output_format"]; ok {
		if format, ok := of.(string); ok {
			validFormats := []string{"records", "table", "json"}
			found := false
			for _, valid := range validFormats {
				if format == valid {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("invalid output_format: %s", format)
			}
		}
	}

	return nil
}

// TestConnection tests the strategy configuration
func (s *RowBasedStrategy) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
	err := s.ValidateConfig(config)
	if err != nil {
		return core.ConnectionTestResult{
			Success: false,
			Message: "Configuration validation failed",
			Errors:  []string{err.Error()},
		}
	}

	return core.ConnectionTestResult{
		Success: true,
		Message: "Row-based structured strategy configuration is valid",
		Details: map[string]any{
			"rows_per_chunk": config["rows_per_chunk"],
			"output_format":  config["output_format"],
			"split_method":   "row_based",
		},
	}
}

// GetType returns the connector type
func (s *RowBasedStrategy) GetType() string {
	return "strategy"
}

// GetName returns the strategy name
func (s *RowBasedStrategy) GetName() string {
	return s.name
}

// GetVersion returns the strategy version
func (s *RowBasedStrategy) GetVersion() string {
	return s.version
}

// GetRequiredReaderCapabilities returns capabilities the reader must support
func (s *RowBasedStrategy) GetRequiredReaderCapabilities() []string {
	return []string{"streaming"}
}

// ConfigureReader modifies reader configuration for this strategy
func (s *RowBasedStrategy) ConfigureReader(readerConfig map[string]any) (map[string]any, error) {
	configured := make(map[string]any)
	for k, v := range readerConfig {
		configured[k] = v
	}
	
	// Ensure headers are available if needed
	configured["has_header"] = true
	return configured, nil
}

// ProcessChunk processes raw data into chunks using row-based strategy
func (s *RowBasedStrategy) ProcessChunk(ctx context.Context, rawData any, metadata core.ChunkMetadata) ([]core.Chunk, error) {
	// Get configuration
	config := s.getDefaultConfig()
	if metadata.Context != nil {
		s.updateConfigFromContext(config, metadata.Context)
	}

	// Handle different input types
	switch data := rawData.(type) {
	case map[string]any:
		// Single row/record
		return s.processSingleRecord(data, config, metadata), nil
	case []map[string]any:
		// Multiple records
		return s.processMultipleRecords(data, config, metadata), nil
	case [][]string:
		// Raw table data
		return s.processTableData(data, config, metadata), nil
	default:
		return nil, fmt.Errorf("row-based strategy expects map, slice of maps, or table data, got %T", rawData)
	}
}

// GetOptimalChunkSize recommends chunk size based on source schema
func (s *RowBasedStrategy) GetOptimalChunkSize(sourceSchema core.SchemaInfo) int {
	// Calculate based on estimated row size and column count
	rowSize := 100 // Default estimate
	
	if len(sourceSchema.Fields) > 0 {
		// Estimate row size based on field types
		rowSize = 0
		for _, field := range sourceSchema.Fields {
			switch field.Type {
			case "string":
				rowSize += 50 // Average string length
			case "text":
				rowSize += 200 // Longer text
			case "integer":
				rowSize += 10
			case "float":
				rowSize += 15
			case "boolean":
				rowSize += 5
			case "date", "datetime":
				rowSize += 20
			default:
				rowSize += 30
			}
		}
	}
	
	// Target 1MB chunks
	targetSize := 1024 * 1024
	optimalRows := targetSize / rowSize
	
	// Clamp to reasonable range
	if optimalRows < 10 {
		optimalRows = 10
	} else if optimalRows > 1000 {
		optimalRows = 1000
	}
	
	return optimalRows
}

// SupportsParallelProcessing indicates if chunks can be processed concurrently
func (s *RowBasedStrategy) SupportsParallelProcessing() bool {
	return true // Row-based chunks are independent
}

// Internal types and methods

type rowBasedConfig struct {
	RowsPerChunk         int
	IncludeHeaders       bool
	PreserveRelationships bool
	GroupByColumn        string
	MaxChunkSizeMB       int
	OutputFormat         string
}

func (s *RowBasedStrategy) getDefaultConfig() *rowBasedConfig {
	return &rowBasedConfig{
		RowsPerChunk:         100,
		IncludeHeaders:       true,
		PreserveRelationships: false,
		GroupByColumn:        "",
		MaxChunkSizeMB:       10,
		OutputFormat:         "records",
	}
}

func (s *RowBasedStrategy) updateConfigFromContext(config *rowBasedConfig, context map[string]string) {
	if rpc, ok := context["rows_per_chunk"]; ok {
		if rows, err := parseInt(rpc); err == nil {
			config.RowsPerChunk = rows
		}
	}
	if ih, ok := context["include_headers"]; ok {
		config.IncludeHeaders = ih == "true"
	}
	if pr, ok := context["preserve_relationships"]; ok {
		config.PreserveRelationships = pr == "true"
	}
	if gbc, ok := context["group_by_column"]; ok {
		config.GroupByColumn = gbc
	}
	if mcs, ok := context["max_chunk_size_mb"]; ok {
		if size, err := parseInt(mcs); err == nil {
			config.MaxChunkSizeMB = size
		}
	}
	if of, ok := context["output_format"]; ok {
		config.OutputFormat = of
	}
}

func (s *RowBasedStrategy) processSingleRecord(record map[string]any, config *rowBasedConfig, metadata core.ChunkMetadata) []core.Chunk {
	var chunks []core.Chunk
	
	chunkData := s.formatChunkData([]map[string]any{record}, config)
	
	chunk := core.Chunk{
		Data: chunkData,
		Metadata: core.ChunkMetadata{
			SourcePath:  metadata.SourcePath,
			ChunkID:     fmt.Sprintf("%s:row:1", metadata.ChunkID),
			ChunkType:   "structured",
			SizeBytes:   int64(s.estimateSize(chunkData)),
			ProcessedAt: metadata.ProcessedAt,
			ProcessedBy: s.name,
			Quality:     s.calculateStructuredQuality([]map[string]any{record}),
			Context: map[string]string{
				"chunk_strategy": "row_based",
				"chunk_number":   "1",
				"total_chunks":   "1",
				"record_count":   "1",
				"output_format":  config.OutputFormat,
			},
			SchemaInfo: map[string]any{
				"fields": s.extractFieldNames(record),
				"types":  s.extractFieldTypes(record),
			},
		},
	}
	
	chunks = append(chunks, chunk)
	return chunks
}

func (s *RowBasedStrategy) processMultipleRecords(records []map[string]any, config *rowBasedConfig, metadata core.ChunkMetadata) []core.Chunk {
	var chunks []core.Chunk
	
	if len(records) == 0 {
		return chunks
	}
	
	// Group records if needed
	if config.GroupByColumn != "" {
		records = s.groupRecordsByColumn(records, config.GroupByColumn)
	}
	
	// Split into chunks
	chunkNumber := 1
	maxSizeBytes := config.MaxChunkSizeMB * 1024 * 1024
	
	for i := 0; i < len(records); i += config.RowsPerChunk {
		end := i + config.RowsPerChunk
		if end > len(records) {
			end = len(records)
		}
		
		chunkRecords := records[i:end]
		chunkData := s.formatChunkData(chunkRecords, config)
		chunkSize := s.estimateSize(chunkData)
		
		// Split further if chunk is too large
		if chunkSize > maxSizeBytes && len(chunkRecords) > 1 {
			subChunks := s.splitLargeChunk(chunkRecords, maxSizeBytes, config, metadata, &chunkNumber)
			chunks = append(chunks, subChunks...)
		} else {
			chunk := core.Chunk{
				Data: chunkData,
				Metadata: core.ChunkMetadata{
					SourcePath:  metadata.SourcePath,
					ChunkID:     fmt.Sprintf("%s:row:%d", metadata.ChunkID, chunkNumber),
					ChunkType:   "structured",
					SizeBytes:   int64(chunkSize),
					ProcessedAt: metadata.ProcessedAt,
					ProcessedBy: s.name,
					Quality:     s.calculateStructuredQuality(chunkRecords),
					Context: map[string]string{
						"chunk_strategy": "row_based",
						"chunk_number":   fmt.Sprintf("%d", chunkNumber),
						"record_count":   fmt.Sprintf("%d", len(chunkRecords)),
						"start_row":      fmt.Sprintf("%d", i+1),
						"end_row":        fmt.Sprintf("%d", end),
						"output_format":  config.OutputFormat,
					},
					SchemaInfo: map[string]any{
						"fields": s.extractFieldNames(chunkRecords[0]),
						"types":  s.extractFieldTypes(chunkRecords[0]),
					},
				},
			}
			chunks = append(chunks, chunk)
			chunkNumber++
		}
	}
	
	// Update total chunks in context
	for i := range chunks {
		chunks[i].Metadata.Context["total_chunks"] = fmt.Sprintf("%d", len(chunks))
	}
	
	return chunks
}

func (s *RowBasedStrategy) processTableData(table [][]string, config *rowBasedConfig, metadata core.ChunkMetadata) []core.Chunk {
	if len(table) == 0 {
		return []core.Chunk{}
	}
	
	// Convert table to records
	var headers []string
	var records []map[string]any
	
	if config.IncludeHeaders && len(table) > 0 {
		headers = table[0]
		for _, row := range table[1:] {
			record := make(map[string]any)
			for i, value := range row {
				if i < len(headers) {
					record[headers[i]] = value
				} else {
					record[fmt.Sprintf("col_%d", i)] = value
				}
			}
			records = append(records, record)
		}
	} else {
		// Generate headers
		if len(table) > 0 {
			for i := range table[0] {
				headers = append(headers, fmt.Sprintf("col_%d", i))
			}
		}
		
		for _, row := range table {
			record := make(map[string]any)
			for i, value := range row {
				if i < len(headers) {
					record[headers[i]] = value
				}
			}
			records = append(records, record)
		}
	}
	
	return s.processMultipleRecords(records, config, metadata)
}

func (s *RowBasedStrategy) formatChunkData(records []map[string]any, config *rowBasedConfig) any {
	switch config.OutputFormat {
	case "records":
		return records
	case "table":
		return s.convertToTable(records)
	case "json":
		return s.convertToJSON(records)
	default:
		return records
	}
}

func (s *RowBasedStrategy) convertToTable(records []map[string]any) map[string]any {
	if len(records) == 0 {
		return map[string]any{
			"headers": []string{},
			"rows":    [][]any{},
		}
	}
	
	// Extract headers from first record
	var headers []string
	for key := range records[0] {
		headers = append(headers, key)
	}
	
	// Convert records to rows
	var rows [][]any
	for _, record := range records {
		var row []any
		for _, header := range headers {
			row = append(row, record[header])
		}
		rows = append(rows, row)
	}
	
	return map[string]any{
		"headers": headers,
		"rows":    rows,
	}
}

func (s *RowBasedStrategy) convertToJSON(records []map[string]any) string {
	// Simple JSON conversion (in real implementation, use proper JSON marshaling)
	var parts []string
	for _, record := range records {
		var fields []string
		for key, value := range record {
			fields = append(fields, fmt.Sprintf(`"%s": "%v"`, key, value))
		}
		parts = append(parts, "{"+strings.Join(fields, ", ")+"}")
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func (s *RowBasedStrategy) groupRecordsByColumn(records []map[string]any, columnName string) []map[string]any {
	// Group records by the specified column value
	groups := make(map[string][]map[string]any)
	
	for _, record := range records {
		key := fmt.Sprintf("%v", record[columnName])
		groups[key] = append(groups[key], record)
	}
	
	// Flatten groups back to a single slice
	var result []map[string]any
	for _, group := range groups {
		result = append(result, group...)
	}
	
	return result
}

func (s *RowBasedStrategy) splitLargeChunk(records []map[string]any, maxSize int, config *rowBasedConfig, metadata core.ChunkMetadata, chunkNumber *int) []core.Chunk {
	var chunks []core.Chunk
	
	// Binary search for optimal chunk size
	left, right := 1, len(records)
	for left < right {
		mid := (left + right + 1) / 2
		testData := s.formatChunkData(records[:mid], config)
		if s.estimateSize(testData) <= maxSize {
			left = mid
		} else {
			right = mid - 1
		}
	}
	
	optimalSize := left
	if optimalSize < 1 {
		optimalSize = 1
	}
	
	// Create chunks of optimal size
	for i := 0; i < len(records); i += optimalSize {
		end := i + optimalSize
		if end > len(records) {
			end = len(records)
		}
		
		chunkRecords := records[i:end]
		chunkData := s.formatChunkData(chunkRecords, config)
		
		chunk := core.Chunk{
			Data: chunkData,
			Metadata: core.ChunkMetadata{
				SourcePath:  metadata.SourcePath,
				ChunkID:     fmt.Sprintf("%s:row:%d", metadata.ChunkID, *chunkNumber),
				ChunkType:   "structured",
				SizeBytes:   int64(s.estimateSize(chunkData)),
				ProcessedAt: metadata.ProcessedAt,
				ProcessedBy: s.name,
				Quality:     s.calculateStructuredQuality(chunkRecords),
				Context: map[string]string{
					"chunk_strategy": "row_based",
					"chunk_number":   fmt.Sprintf("%d", *chunkNumber),
					"record_count":   fmt.Sprintf("%d", len(chunkRecords)),
					"output_format":  config.OutputFormat,
					"size_limited":   "true",
				},
				SchemaInfo: map[string]any{
					"fields": s.extractFieldNames(chunkRecords[0]),
					"types":  s.extractFieldTypes(chunkRecords[0]),
				},
			},
		}
		
		chunks = append(chunks, chunk)
		*chunkNumber++
	}
	
	return chunks
}

func (s *RowBasedStrategy) estimateSize(data any) int {
	// Simple size estimation
	switch d := data.(type) {
	case string:
		return len(d)
	case []map[string]any:
		size := 0
		for _, record := range d {
			for key, value := range record {
				size += len(key) + len(fmt.Sprintf("%v", value)) + 10 // overhead
			}
		}
		return size
	case map[string]any:
		size := 0
		if headers, ok := d["headers"].([]string); ok {
			for _, header := range headers {
				size += len(header)
			}
		}
		if rows, ok := d["rows"].([][]any); ok {
			for _, row := range rows {
				for _, cell := range row {
					size += len(fmt.Sprintf("%v", cell)) + 5
				}
			}
		}
		return size
	default:
		return len(fmt.Sprintf("%v", data))
	}
}

func (s *RowBasedStrategy) extractFieldNames(record map[string]any) []string {
	var fields []string
	for key := range record {
		fields = append(fields, key)
	}
	return fields
}

func (s *RowBasedStrategy) extractFieldTypes(record map[string]any) map[string]string {
	types := make(map[string]string)
	for key, value := range record {
		if value == nil {
			types[key] = "null"
		} else {
			types[key] = reflect.TypeOf(value).String()
		}
	}
	return types
}

func (s *RowBasedStrategy) calculateStructuredQuality(records []map[string]any) *core.QualityMetrics {
	if len(records) == 0 {
		return &core.QualityMetrics{
			Completeness: 0.0,
			Coherence:    0.0,
			Uniqueness:   0.0,
		}
	}
	
	// Calculate completeness (non-null values)
	totalFields := 0
	nonNullFields := 0
	
	for _, record := range records {
		for _, value := range record {
			totalFields++
			if value != nil && value != "" {
				nonNullFields++
			}
		}
	}
	
	completeness := 0.0
	if totalFields > 0 {
		completeness = float64(nonNullFields) / float64(totalFields)
	}
	
	// Coherence: consistent field types across records
	coherence := 1.0
	if len(records) > 1 {
		firstRecord := records[0]
		for _, record := range records[1:] {
			for key, value := range record {
				if firstValue, ok := firstRecord[key]; ok {
					if reflect.TypeOf(value) != reflect.TypeOf(firstValue) {
						coherence -= 0.1
					}
				}
			}
		}
		if coherence < 0 {
			coherence = 0
		}
	}
	
	// Uniqueness: unique records
	uniqueness := 1.0
	if len(records) > 1 {
		unique := make(map[string]bool)
		for _, record := range records {
			key := fmt.Sprintf("%v", record)
			unique[key] = true
		}
		uniqueness = float64(len(unique)) / float64(len(records))
	}
	
	return &core.QualityMetrics{
		Completeness: completeness,
		Coherence:    coherence,
		Uniqueness:   uniqueness,
		Readability:  1.0, // Structured data is always "readable"
		Language:     "",  // Not applicable
		LanguageConf: 0.0,
	}
}

// Helper functions

func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

func ptr(f float64) *float64 {
	return &f
}