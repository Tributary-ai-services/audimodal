package json

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
	"github.com/jscharber/eAIIngest/pkg/preprocessing"
)

// JSONReader implements DataSourceReader for JSON files with nested object handling
type JSONReader struct {
	name             string
	version          string
	encodingDetector *preprocessing.EncodingDetector
}

// NewJSONReader creates a new JSON file reader
func NewJSONReader() *JSONReader {
	return &JSONReader{
		name:             "json_reader",
		version:          "1.0.0",
		encodingDetector: preprocessing.NewEncodingDetector(),
	}
}

// GetConfigSpec returns the configuration specification
func (r *JSONReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "format",
			Type:        "string",
			Required:    false,
			Default:     "auto",
			Description: "JSON format type",
			Enum:        []string{"auto", "single", "array", "lines"},
			Examples:    []string{"single", "array", "lines"},
		},
		{
			Name:        "max_object_size",
			Type:        "int",
			Required:    false,
			Default:     10485760, // 10MB
			Description: "Maximum size of a single JSON object in bytes",
			MinValue:    ptr(1024.0),
			MaxValue:    ptr(104857600.0), // 100MB
		},
		{
			Name:        "encoding",
			Type:        "string",
			Required:    false,
			Default:     "auto",
			Description: "File encoding (auto-detect if 'auto')",
			Enum:        []string{"auto", "utf-8", "utf-16", "iso-8859-1", "windows-1252"},
		},
		{
			Name:        "strict_mode",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Whether to use strict JSON parsing",
		},
		{
			Name:        "flatten_nested",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Whether to flatten nested objects into separate chunks",
		},
		{
			Name:        "array_path",
			Type:        "string",
			Required:    false,
			Default:     "",
			Description: "JSONPath to extract array elements (for nested arrays)",
			Examples:    []string{"$.data", "$.results", "$.items"},
		},
		{
			Name:        "max_nesting_depth",
			Type:        "int",
			Required:    false,
			Default:     10,
			Description: "Maximum depth for nested object flattening",
			MinValue:    ptr(1.0),
			MaxValue:    ptr(50.0),
		},
		{
			Name:        "flatten_arrays",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Flatten nested arrays into separate chunks",
		},
		{
			Name:        "extract_keys",
			Type:        "array",
			Required:    false,
			Default:     []string{},
			Description: "Specific JSON keys to extract (empty = all)",
		},
		{
			Name:        "skip_keys",
			Type:        "array",
			Required:    false,
			Default:     []string{},
			Description: "JSON keys to skip during processing",
		},
		{
			Name:        "preserve_structure",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Preserve original JSON structure in chunks",
		},
		{
			Name:        "null_handling",
			Type:        "string",
			Required:    false,
			Default:     "keep",
			Description: "How to handle null values",
			Enum:        []string{"keep", "skip", "convert_empty"},
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *JSONReader) ValidateConfig(config map[string]any) error {
	if format, ok := config["format"]; ok {
		if str, ok := format.(string); ok {
			validFormats := []string{"auto", "single", "array", "lines"}
			for _, valid := range validFormats {
				if str == valid {
					goto formatOK
				}
			}
			return fmt.Errorf("invalid format: %s", str)
		formatOK:
		}
	}

	if maxSize, ok := config["max_object_size"]; ok {
		if num, ok := maxSize.(float64); ok {
			if num < 1024 || num > 104857600 {
				return fmt.Errorf("max_object_size must be between 1024 and 104857600")
			}
		}
	}

	if encoding, ok := config["encoding"]; ok {
		if str, ok := encoding.(string); ok {
			validEncodings := []string{"auto", "utf-8", "utf-16", "iso-8859-1", "windows-1252"}
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

	if maxDepth, ok := config["max_nesting_depth"]; ok {
		if num, ok := maxDepth.(float64); ok {
			if num < 1 || num > 50 {
				return fmt.Errorf("max_nesting_depth must be between 1 and 50")
			}
		}
	}

	if nullHandling, ok := config["null_handling"]; ok {
		if str, ok := nullHandling.(string); ok {
			validModes := []string{"keep", "skip", "convert_empty"}
			found := false
			for _, valid := range validModes {
				if str == valid {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("invalid null_handling mode: %s", str)
			}
		}
	}

	return nil
}

// TestConnection tests if the JSON file can be read
func (r *JSONReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
		Message: "JSON reader configuration is valid",
		Latency: time.Since(start),
		Details: map[string]any{
			"format":   config["format"],
			"encoding": config["encoding"],
		},
	}
}

// GetType returns the connector type
func (r *JSONReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *JSONReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *JSONReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the JSON file structure
func (r *JSONReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Read first part of file to determine format and structure
	buffer := make([]byte, 8192)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return core.SchemaInfo{}, fmt.Errorf("failed to read file: %w", err)
	}

	content := strings.TrimSpace(string(buffer[:n]))

	// Detect JSON format
	format := r.detectJSONFormat(content)

	// Parse sample data based on detected format
	var sampleObjects []map[string]any
	var fields []core.FieldInfo

	switch format {
	case "single":
		var obj map[string]any
		if err := json.Unmarshal([]byte(content), &obj); err == nil {
			sampleObjects = append(sampleObjects, obj)
			fields = r.extractFieldsFromObject(obj)
		}
	case "array":
		var arr []map[string]any
		if err := json.Unmarshal([]byte(content), &arr); err == nil {
			for i, obj := range arr {
				if i >= 5 { // Limit sample size
					break
				}
				sampleObjects = append(sampleObjects, obj)
			}
			if len(arr) > 0 {
				fields = r.extractFieldsFromObject(arr[0])
			}
		}
	case "lines":
		// Parse first few lines
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if i >= 5 || strings.TrimSpace(line) == "" {
				break
			}
			var obj map[string]any
			if err := json.Unmarshal([]byte(line), &obj); err == nil {
				sampleObjects = append(sampleObjects, obj)
				if i == 0 {
					fields = r.extractFieldsFromObject(obj)
				}
			}
		}
	}

	schema := core.SchemaInfo{
		Format:   "semi_structured",
		Encoding: "utf-8",
		Fields:   fields,
		Metadata: map[string]any{
			"json_format":    format,
			"sample_objects": len(sampleObjects),
		},
	}

	// Convert sample objects to interface slice
	sampleData := make([]map[string]any, len(sampleObjects))
	copy(sampleData, sampleObjects)
	schema.SampleData = sampleData

	return schema, nil
}

// EstimateSize returns size estimates for the JSON file
func (r *JSONReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	// Quick analysis to estimate object count
	file, err := os.Open(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	buffer := make([]byte, 4096)
	n, _ := file.Read(buffer)
	content := string(buffer[:n])

	format := r.detectJSONFormat(content)

	var estimatedObjects int64

	switch format {
	case "single":
		estimatedObjects = 1
	case "array":
		// Count opening braces in sample to estimate objects
		braceCount := strings.Count(content, "{")
		if braceCount > 0 {
			avgObjectSize := int64(n) / int64(braceCount)
			estimatedObjects = stat.Size() / avgObjectSize
		} else {
			estimatedObjects = 1
		}
	case "lines":
		// Count lines in sample
		lines := strings.Split(content, "\n")
		avgLineSize := int64(n) / int64(len(lines))
		if avgLineSize > 0 {
			estimatedObjects = stat.Size() / avgLineSize
		} else {
			estimatedObjects = stat.Size() / 100 // Fallback estimate
		}
	default:
		estimatedObjects = stat.Size() / 1000 // Rough estimate
	}

	complexity := "low"
	if stat.Size() > 10*1024*1024 || estimatedObjects > 10000 { // > 10MB or > 10k objects
		complexity = "medium"
	}
	if stat.Size() > 100*1024*1024 || estimatedObjects > 100000 { // > 100MB or > 100k objects
		complexity = "high"
	}

	processTime := "fast"
	if estimatedObjects > 50000 {
		processTime = "medium"
	}
	if estimatedObjects > 500000 {
		processTime = "slow"
	}

	// Each JSON object typically becomes one chunk
	estimatedChunks := int(estimatedObjects)

	return core.SizeEstimate{
		RowCount:    &estimatedObjects,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the JSON file
func (r *JSONReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
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

	// Detect format if set to auto
	format := "auto"
	if f, ok := strategyConfig["format"].(string); ok {
		format = f
	}

	if format == "auto" {
		buffer := make([]byte, 1024)
		n, _ := file.Read(buffer)
		content := string(buffer[:n])
		format = r.detectJSONFormat(content)
		file.Seek(0, io.SeekStart) // Reset to beginning
	}

	iterator := &JSONIterator{
		file:        file,
		sourcePath:  sourcePath,
		actualPath:  filePath,
		cleanupPath: cleanupPath,
		config:      strategyConfig,
		format:      format,
		objectNum:   0,
		reader:      r,
	}

	// Initialize based on format
	switch format {
	case "lines":
		iterator.scanner = bufio.NewScanner(file)
	case "array":
		// Skip to first array element
		decoder := json.NewDecoder(file)
		// Read opening bracket
		token, err := decoder.Token()
		if err != nil {
			file.Close()
			return nil, fmt.Errorf("failed to read array start: %w", err)
		}
		if delim, ok := token.(json.Delim); !ok || delim != '[' {
			file.Close()
			return nil, fmt.Errorf("expected array start, got %v", token)
		}
		iterator.decoder = decoder
	case "single":
		iterator.decoder = json.NewDecoder(file)
	}

	return iterator, nil
}

// SupportsStreaming indicates JSON reader supports streaming
func (r *JSONReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *JSONReader) GetSupportedFormats() []string {
	return []string{"json", "jsonl", "ndjson"}
}

// Helper methods

func (r *JSONReader) detectJSONFormat(content string) string {
	content = strings.TrimSpace(content)

	if strings.HasPrefix(content, "[") {
		return "array"
	}

	if strings.HasPrefix(content, "{") {
		// Check if it's a single object or line-delimited
		lines := strings.Split(content, "\n")
		validJSONLines := 0
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
				validJSONLines++
			}
		}

		if validJSONLines > 1 {
			return "lines"
		}
		return "single"
	}

	return "single" // Default
}

func (r *JSONReader) extractFieldsFromObject(obj map[string]any) []core.FieldInfo {
	fields := make([]core.FieldInfo, 0, len(obj))

	for key, value := range obj {
		fieldType := r.inferJSONFieldType(value)
		field := core.FieldInfo{
			Name:        key,
			Type:        fieldType,
			Nullable:    true,
			Description: fmt.Sprintf("JSON field: %s", key),
		}

		if value != nil {
			field.Examples = []any{value}
		}

		fields = append(fields, field)
	}

	return fields
}

func (r *JSONReader) inferJSONFieldType(value any) string {
	if value == nil {
		return "null"
	}

	switch reflect.TypeOf(value).Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "integer"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.String:
		return "string"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map:
		return "object"
	default:
		return "string"
	}
}

// JSONIterator implements ChunkIterator for JSON files
type JSONIterator struct {
	file        *os.File
	sourcePath  string
	actualPath  string
	cleanupPath string
	config      map[string]any
	format      string
	decoder     *json.Decoder
	scanner     *bufio.Scanner
	objectNum   int
	totalSize   int64
	readBytes   int64
	arrayDone   bool
	reader      *JSONReader
}

// Next returns the next JSON object as a chunk
func (it *JSONIterator) Next(ctx context.Context) (core.Chunk, error) {
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

	var data any
	var rawBytes []byte

	switch it.format {
	case "lines":
		if !it.scanner.Scan() {
			if err := it.scanner.Err(); err != nil {
				return core.Chunk{}, fmt.Errorf("scanner error: %w", err)
			}
			return core.Chunk{}, core.ErrIteratorExhausted
		}

		line := it.scanner.Text()
		rawBytes = []byte(line)
		it.readBytes += int64(len(rawBytes))

		if err := json.Unmarshal(rawBytes, &data); err != nil {
			return core.Chunk{}, fmt.Errorf("failed to parse JSON line: %w", err)
		}

	case "array":
		if it.arrayDone {
			return core.Chunk{}, core.ErrIteratorExhausted
		}

		if !it.decoder.More() {
			it.arrayDone = true
			return core.Chunk{}, core.ErrIteratorExhausted
		}

		if err := it.decoder.Decode(&data); err != nil {
			return core.Chunk{}, fmt.Errorf("failed to decode JSON array element: %w", err)
		}

		// Estimate bytes read (rough approximation)
		if jsonBytes, err := json.Marshal(data); err == nil {
			it.readBytes += int64(len(jsonBytes))
		}

	case "single":
		if it.objectNum > 0 {
			return core.Chunk{}, core.ErrIteratorExhausted
		}

		if err := it.decoder.Decode(&data); err != nil {
			if err == io.EOF {
				return core.Chunk{}, core.ErrIteratorExhausted
			}
			return core.Chunk{}, fmt.Errorf("failed to decode JSON object: %w", err)
		}

		it.readBytes = it.totalSize // For single object, we've read everything
	}

	it.objectNum++

	// Process nested objects if flattening is enabled
	if flattenNested, ok := it.config["flatten_nested"].(bool); ok && flattenNested {
		return it.processNestedObject(data, rawBytes)
	}

	// Apply data filtering
	data = it.filterJSONData(data)

	// Calculate chunk size
	chunkSize := int64(len(rawBytes))
	if chunkSize == 0 {
		if jsonBytes, err := json.Marshal(data); err == nil {
			chunkSize = int64(len(jsonBytes))
		}
	}

	chunk := core.Chunk{
		Data: data,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:object:%d", filepath.Base(it.sourcePath), it.objectNum),
			ChunkType:   "structured",
			SizeBytes:   chunkSize,
			ProcessedAt: time.Now(),
			ProcessedBy: "json_reader",
			Context: map[string]string{
				"object_number": strconv.Itoa(it.objectNum),
				"json_format":   it.format,
				"file_type":     "json",
				"nesting_depth": strconv.Itoa(it.calculateDepth(data)),
			},
			SchemaInfo: map[string]any{
				"format": it.format,
				"type":   reflect.TypeOf(data).String(),
			},
		},
	}

	return chunk, nil
}

// processNestedObject handles flattening of nested objects
func (it *JSONIterator) processNestedObject(data any, rawBytes []byte) (core.Chunk, error) {
	// For now, return the first chunk and store nested data for future iterations
	// This is a simplified implementation - a full version would require
	// maintaining state for nested object iteration

	flattened := it.flattenObject(data, "", 0)
	if len(flattened) > 0 {
		// Return the first flattened object
		firstKey := ""
		for key := range flattened {
			firstKey = key
			break
		}

		chunkSize := int64(len(rawBytes))
		if chunkSize == 0 {
			if jsonBytes, err := json.Marshal(flattened[firstKey]); err == nil {
				chunkSize = int64(len(jsonBytes))
			}
		}

		chunk := core.Chunk{
			Data: flattened[firstKey],
			Metadata: core.ChunkMetadata{
				SourcePath:  it.sourcePath,
				ChunkID:     fmt.Sprintf("%s:nested:%d:%s", filepath.Base(it.sourcePath), it.objectNum, firstKey),
				ChunkType:   "structured_nested",
				SizeBytes:   chunkSize,
				ProcessedAt: time.Now(),
				ProcessedBy: "json_reader",
				Context: map[string]string{
					"object_number": strconv.Itoa(it.objectNum),
					"json_format":   it.format,
					"file_type":     "json",
					"nested_path":   firstKey,
					"is_flattened":  "true",
				},
			},
		}

		return chunk, nil
	}

	// Fallback to regular processing
	return it.createRegularChunk(data, rawBytes)
}

// flattenObject recursively flattens a JSON object
func (it *JSONIterator) flattenObject(obj any, prefix string, depth int) map[string]any {
	result := make(map[string]any)

	maxDepth := 10
	if md, ok := it.config["max_nesting_depth"].(float64); ok {
		maxDepth = int(md)
	}

	if depth >= maxDepth {
		result[prefix] = obj
		return result
	}

	switch v := obj.(type) {
	case map[string]any:
		for key, value := range v {
			newKey := key
			if prefix != "" {
				newKey = prefix + "." + key
			}

			if it.shouldSkipKey(key) {
				continue
			}

			nested := it.flattenObject(value, newKey, depth+1)
			for k, val := range nested {
				result[k] = val
			}
		}
	case []any:
		if flattenArrays, ok := it.config["flatten_arrays"].(bool); ok && flattenArrays {
			for i, value := range v {
				newKey := prefix + "[" + strconv.Itoa(i) + "]"
				nested := it.flattenObject(value, newKey, depth+1)
				for k, val := range nested {
					result[k] = val
				}
			}
		} else {
			result[prefix] = obj
		}
	default:
		result[prefix] = obj
	}

	return result
}

// filterJSONData filters JSON data based on configuration
func (it *JSONIterator) filterJSONData(data any) any {
	switch v := data.(type) {
	case map[string]any:
		filtered := make(map[string]any)

		for key, value := range v {
			if it.shouldSkipKey(key) {
				continue
			}

			if it.shouldExtractKey(key) {
				filtered[key] = it.processValue(value)
			}
		}

		return filtered
	case []any:
		var filtered []any
		for _, item := range v {
			filtered = append(filtered, it.filterJSONData(item))
		}
		return filtered
	default:
		return it.processValue(data)
	}
}

// shouldSkipKey determines if a key should be skipped
func (it *JSONIterator) shouldSkipKey(key string) bool {
	if skipKeys, ok := it.config["skip_keys"].([]string); ok {
		for _, skip := range skipKeys {
			if key == skip {
				return true
			}
		}
	}
	return false
}

// shouldExtractKey determines if a key should be extracted
func (it *JSONIterator) shouldExtractKey(key string) bool {
	if extractKeys, ok := it.config["extract_keys"].([]string); ok && len(extractKeys) > 0 {
		for _, extract := range extractKeys {
			if key == extract {
				return true
			}
		}
		return false
	}
	return true // Extract all keys if no specific keys specified
}

// processValue processes a JSON value based on configuration
func (it *JSONIterator) processValue(value any) any {
	if value == nil {
		nullHandling := "keep"
		if nh, ok := it.config["null_handling"].(string); ok {
			nullHandling = nh
		}

		switch nullHandling {
		case "skip":
			return nil
		case "convert_empty":
			return ""
		default:
			return nil
		}
	}

	return value
}

// calculateDepth calculates the nesting depth of a JSON object
func (it *JSONIterator) calculateDepth(obj any) int {
	return it.calculateDepthRecursive(obj, 0)
}

// calculateDepthRecursive recursively calculates depth
func (it *JSONIterator) calculateDepthRecursive(obj any, currentDepth int) int {
	switch v := obj.(type) {
	case map[string]any:
		maxDepth := currentDepth
		for _, value := range v {
			depth := it.calculateDepthRecursive(value, currentDepth+1)
			if depth > maxDepth {
				maxDepth = depth
			}
		}
		return maxDepth
	case []any:
		maxDepth := currentDepth
		for _, item := range v {
			depth := it.calculateDepthRecursive(item, currentDepth+1)
			if depth > maxDepth {
				maxDepth = depth
			}
		}
		return maxDepth
	default:
		return currentDepth
	}
}

// createRegularChunk creates a regular chunk without nested processing
func (it *JSONIterator) createRegularChunk(data any, rawBytes []byte) (core.Chunk, error) {
	chunkSize := int64(len(rawBytes))
	if chunkSize == 0 {
		if jsonBytes, err := json.Marshal(data); err == nil {
			chunkSize = int64(len(jsonBytes))
		}
	}

	chunk := core.Chunk{
		Data: data,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:object:%d", filepath.Base(it.sourcePath), it.objectNum),
			ChunkType:   "structured",
			SizeBytes:   chunkSize,
			ProcessedAt: time.Now(),
			ProcessedBy: "json_reader",
			Context: map[string]string{
				"object_number": strconv.Itoa(it.objectNum),
				"json_format":   it.format,
				"file_type":     "json",
			},
		},
	}

	return chunk, nil
}

// Close releases file resources
func (it *JSONIterator) Close() error {
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
func (it *JSONIterator) Reset() error {
	if it.file != nil {
		_, err := it.file.Seek(0, io.SeekStart)
		if err != nil {
			return err
		}

		it.objectNum = 0
		it.readBytes = 0
		it.arrayDone = false

		// Reinitialize based on format
		switch it.format {
		case "lines":
			it.scanner = bufio.NewScanner(it.file)
		case "array":
			it.decoder = json.NewDecoder(it.file)
			// Skip opening bracket
			token, err := it.decoder.Token()
			if err != nil {
				return fmt.Errorf("failed to re-read array start: %w", err)
			}
			if delim, ok := token.(json.Delim); !ok || delim != '[' {
				return fmt.Errorf("expected array start on reset, got %v", token)
			}
		case "single":
			it.decoder = json.NewDecoder(it.file)
		}

		return nil
	}
	return fmt.Errorf("file not open")
}

// Progress returns iteration progress
func (it *JSONIterator) Progress() float64 {
	if it.totalSize == 0 {
		return 0.0
	}
	return float64(it.readBytes) / float64(it.totalSize)
}

// Helper function
func ptr(f float64) *float64 {
	return &f
}
