package hybrid

import (
	"context"
	"fmt"
	"strings"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// AdaptiveStrategy implements ChunkingStrategy that adapts to different data types
type AdaptiveStrategy struct {
	name    string
	version string
}

// NewAdaptiveStrategy creates a new adaptive chunking strategy
func NewAdaptiveStrategy() *AdaptiveStrategy {
	return &AdaptiveStrategy{
		name:    "adaptive_hybrid",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (s *AdaptiveStrategy) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "auto_detect_type",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Whether to automatically detect data type and choose strategy",
		},
		{
			Name:        "default_text_chunk_size",
			Type:        "int",
			Required:    false,
			Default:     1000,
			Description: "Default chunk size for text content",
			MinValue:    ptr(100.0),
			MaxValue:    ptr(10000.0),
		},
		{
			Name:        "default_rows_per_chunk",
			Type:        "int",
			Required:    false,
			Default:     100,
			Description: "Default rows per chunk for structured data",
			MinValue:    ptr(1.0),
			MaxValue:    ptr(5000.0),
		},
		{
			Name:        "mixed_content_strategy",
			Type:        "string",
			Required:    false,
			Default:     "separate",
			Description: "How to handle mixed content types",
			Enum:        []string{"separate", "combine", "text_only", "structured_only"},
		},
		{
			Name:        "preserve_structure",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Whether to preserve original data structure in chunks",
		},
		{
			Name:        "quality_threshold",
			Type:        "float",
			Required:    false,
			Default:     0.5,
			Description: "Minimum quality threshold for chunks",
			MinValue:    ptr(0.0),
			MaxValue:    ptr(1.0),
		},
	}
}

// ValidateConfig validates the provided configuration
func (s *AdaptiveStrategy) ValidateConfig(config map[string]any) error {
	if dtcs, ok := config["default_text_chunk_size"]; ok {
		if size, ok := dtcs.(float64); ok {
			if size < 100 || size > 10000 {
				return fmt.Errorf("default_text_chunk_size must be between 100 and 10000")
			}
		}
	}

	if drpc, ok := config["default_rows_per_chunk"]; ok {
		if rows, ok := drpc.(float64); ok {
			if rows < 1 || rows > 5000 {
				return fmt.Errorf("default_rows_per_chunk must be between 1 and 5000")
			}
		}
	}

	if mcs, ok := config["mixed_content_strategy"]; ok {
		if strategy, ok := mcs.(string); ok {
			validStrategies := []string{"separate", "combine", "text_only", "structured_only"}
			found := false
			for _, valid := range validStrategies {
				if strategy == valid {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("invalid mixed_content_strategy: %s", strategy)
			}
		}
	}

	if qt, ok := config["quality_threshold"]; ok {
		if threshold, ok := qt.(float64); ok {
			if threshold < 0.0 || threshold > 1.0 {
				return fmt.Errorf("quality_threshold must be between 0.0 and 1.0")
			}
		}
	}

	return nil
}

// TestConnection tests the strategy configuration
func (s *AdaptiveStrategy) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
		Message: "Adaptive hybrid strategy configuration is valid",
		Details: map[string]any{
			"auto_detect":        config["auto_detect_type"],
			"mixed_strategy":     config["mixed_content_strategy"],
			"preserve_structure": config["preserve_structure"],
			"split_method":       "adaptive",
		},
	}
}

// GetType returns the connector type
func (s *AdaptiveStrategy) GetType() string {
	return "strategy"
}

// GetName returns the strategy name
func (s *AdaptiveStrategy) GetName() string {
	return s.name
}

// GetVersion returns the strategy version
func (s *AdaptiveStrategy) GetVersion() string {
	return s.version
}

// GetRequiredReaderCapabilities returns capabilities the reader must support
func (s *AdaptiveStrategy) GetRequiredReaderCapabilities() []string {
	return []string{"streaming"}
}

// ConfigureReader modifies reader configuration for this strategy
func (s *AdaptiveStrategy) ConfigureReader(readerConfig map[string]any) (map[string]any, error) {
	configured := make(map[string]any)
	for k, v := range readerConfig {
		configured[k] = v
	}

	// Adaptive strategy needs to preserve all data
	configured["skip_empty_lines"] = false
	configured["has_header"] = true

	return configured, nil
}

// ProcessChunk processes raw data into chunks using adaptive strategy
func (s *AdaptiveStrategy) ProcessChunk(ctx context.Context, rawData any, metadata core.ChunkMetadata) ([]core.Chunk, error) {
	// Get configuration
	config := s.getDefaultConfig()
	if metadata.Context != nil {
		s.updateConfigFromContext(config, metadata.Context)
	}

	// Analyze data type and choose appropriate strategy
	dataType := s.analyzeDataType(rawData)

	// Process based on detected type
	switch dataType {
	case "text":
		return s.processAsText(rawData, config, metadata)
	case "structured":
		return s.processAsStructured(rawData, config, metadata)
	case "mixed":
		return s.processAsMixed(rawData, config, metadata)
	case "json":
		return s.processAsJSON(rawData, config, metadata)
	default:
		return s.processAsGeneric(rawData, config, metadata)
	}
}

// GetOptimalChunkSize recommends chunk size based on source schema
func (s *AdaptiveStrategy) GetOptimalChunkSize(sourceSchema core.SchemaInfo) int {
	switch sourceSchema.Format {
	case "structured":
		// For structured data, optimize for row count
		if len(sourceSchema.Fields) > 20 {
			return 50 // Fewer rows for wide tables
		}
		return 100
	case "unstructured":
		// For unstructured text, use semantic chunking size
		return 1500
	case "semi_structured":
		// For JSON-like data, balance between structure and content
		return 1000
	default:
		return 1000
	}
}

// SupportsParallelProcessing indicates if chunks can be processed concurrently
func (s *AdaptiveStrategy) SupportsParallelProcessing() bool {
	return true // Adaptive chunks are designed to be independent
}

// Internal types and methods

type adaptiveConfig struct {
	AutoDetectType       bool
	DefaultTextChunkSize int
	DefaultRowsPerChunk  int
	MixedContentStrategy string
	PreserveStructure    bool
	QualityThreshold     float64
}

func (s *AdaptiveStrategy) getDefaultConfig() *adaptiveConfig {
	return &adaptiveConfig{
		AutoDetectType:       true,
		DefaultTextChunkSize: 1000,
		DefaultRowsPerChunk:  100,
		MixedContentStrategy: "separate",
		PreserveStructure:    true,
		QualityThreshold:     0.5,
	}
}

func (s *AdaptiveStrategy) updateConfigFromContext(config *adaptiveConfig, context map[string]string) {
	if adt, ok := context["auto_detect_type"]; ok {
		config.AutoDetectType = adt == "true"
	}
	if dtcs, ok := context["default_text_chunk_size"]; ok {
		if size, err := parseInt(dtcs); err == nil {
			config.DefaultTextChunkSize = size
		}
	}
	if drpc, ok := context["default_rows_per_chunk"]; ok {
		if rows, err := parseInt(drpc); err == nil {
			config.DefaultRowsPerChunk = rows
		}
	}
	if mcs, ok := context["mixed_content_strategy"]; ok {
		config.MixedContentStrategy = mcs
	}
	if ps, ok := context["preserve_structure"]; ok {
		config.PreserveStructure = ps == "true"
	}
	if qt, ok := context["quality_threshold"]; ok {
		if threshold, err := parseFloat(qt); err == nil {
			config.QualityThreshold = threshold
		}
	}
}

func (s *AdaptiveStrategy) analyzeDataType(data any) string {
	switch d := data.(type) {
	case string:
		// Check if string contains structured data
		if s.looksLikeJSON(d) {
			return "json"
		}
		if s.looksLikeCSV(d) {
			return "structured"
		}
		return "text"
	case map[string]any:
		return "structured"
	case []map[string]any:
		return "structured"
	case [][]string:
		return "structured"
	case []any:
		// Mixed array content
		return "mixed"
	default:
		return "generic"
	}
}

func (s *AdaptiveStrategy) processAsText(rawData any, config *adaptiveConfig, metadata core.ChunkMetadata) ([]core.Chunk, error) {
	text, ok := rawData.(string)
	if !ok {
		return nil, fmt.Errorf("expected string for text processing, got %T", rawData)
	}

	// Use semantic-aware text chunking
	chunks := s.chunkTextAdaptively(text, config, metadata)
	return chunks, nil
}

func (s *AdaptiveStrategy) processAsStructured(rawData any, config *adaptiveConfig, metadata core.ChunkMetadata) ([]core.Chunk, error) {
	// Convert to standard record format
	var records []map[string]any

	switch data := rawData.(type) {
	case map[string]any:
		records = []map[string]any{data}
	case []map[string]any:
		records = data
	case [][]string:
		records = s.convertTableToRecords(data)
	default:
		return nil, fmt.Errorf("unsupported structured data type: %T", rawData)
	}

	// Use row-based chunking
	chunks := s.chunkRecordsAdaptively(records, config, metadata)
	return chunks, nil
}

func (s *AdaptiveStrategy) processAsMixed(rawData any, config *adaptiveConfig, metadata core.ChunkMetadata) ([]core.Chunk, error) {
	array, ok := rawData.([]any)
	if !ok {
		return nil, fmt.Errorf("expected array for mixed processing, got %T", rawData)
	}

	switch config.MixedContentStrategy {
	case "separate":
		return s.processMixedSeparately(array, config, metadata)
	case "combine":
		return s.processMixedCombined(array, config, metadata)
	case "text_only":
		return s.processMixedTextOnly(array, config, metadata)
	case "structured_only":
		return s.processMixedStructuredOnly(array, config, metadata)
	default:
		return s.processMixedSeparately(array, config, metadata)
	}
}

func (s *AdaptiveStrategy) processAsJSON(rawData any, config *adaptiveConfig, metadata core.ChunkMetadata) ([]core.Chunk, error) {
	text, ok := rawData.(string)
	if !ok {
		return nil, fmt.Errorf("expected string for JSON processing, got %T", rawData)
	}

	// Try to parse and chunk as structured data
	// For simplicity, treat as text with special handling
	chunks := s.chunkJSONAdaptively(text, config, metadata)
	return chunks, nil
}

func (s *AdaptiveStrategy) processAsGeneric(rawData any, config *adaptiveConfig, metadata core.ChunkMetadata) ([]core.Chunk, error) {
	// Convert to string and process as text
	text := fmt.Sprintf("%v", rawData)
	return s.processAsText(text, config, metadata)
}

func (s *AdaptiveStrategy) chunkTextAdaptively(text string, config *adaptiveConfig, metadata core.ChunkMetadata) []core.Chunk {
	var chunks []core.Chunk

	// Detect text structure
	structure := s.analyzeTextStructure(text)

	// Chunk based on structure
	chunkNumber := 1
	currentChunk := ""
	currentStart := 0

	for _, section := range structure.Sections {
		if len(currentChunk)+len(section.Content) > config.DefaultTextChunkSize && len(currentChunk) > 0 {
			// Create chunk with current content
			if s.meetsQualityThreshold(currentChunk, config.QualityThreshold) {
				chunk := s.createTextChunk(currentChunk, currentStart, chunkNumber, metadata)
				chunks = append(chunks, chunk)
				chunkNumber++
			}

			// Start new chunk
			currentChunk = section.Content
			currentStart = section.Start
		} else {
			// Add to current chunk
			if currentChunk != "" {
				currentChunk += "\n\n"
			}
			currentChunk += section.Content
			if currentStart == 0 {
				currentStart = section.Start
			}
		}
	}

	// Add final chunk
	if len(currentChunk) > 0 && s.meetsQualityThreshold(currentChunk, config.QualityThreshold) {
		chunk := s.createTextChunk(currentChunk, currentStart, chunkNumber, metadata)
		chunks = append(chunks, chunk)
	}

	return chunks
}

func (s *AdaptiveStrategy) chunkRecordsAdaptively(records []map[string]any, config *adaptiveConfig, metadata core.ChunkMetadata) []core.Chunk {
	var chunks []core.Chunk

	chunkNumber := 1
	for i := 0; i < len(records); i += config.DefaultRowsPerChunk {
		end := i + config.DefaultRowsPerChunk
		if end > len(records) {
			end = len(records)
		}

		chunkRecords := records[i:end]

		chunk := core.Chunk{
			Data: chunkRecords,
			Metadata: core.ChunkMetadata{
				SourcePath:  metadata.SourcePath,
				ChunkID:     fmt.Sprintf("%s:adaptive:%d", metadata.ChunkID, chunkNumber),
				ChunkType:   "structured",
				SizeBytes:   int64(s.estimateRecordSize(chunkRecords)),
				ProcessedAt: metadata.ProcessedAt,
				ProcessedBy: s.name,
				Quality:     s.calculateAdaptiveQuality(chunkRecords, "structured"),
				Context: map[string]string{
					"chunk_strategy": "adaptive",
					"data_type":      "structured",
					"chunk_number":   fmt.Sprintf("%d", chunkNumber),
					"record_count":   fmt.Sprintf("%d", len(chunkRecords)),
				},
				SchemaInfo: map[string]any{
					"adaptive_type": "structured",
					"fields":        s.extractFieldNames(chunkRecords[0]),
				},
			},
		}

		chunks = append(chunks, chunk)
		chunkNumber++
	}

	return chunks
}

func (s *AdaptiveStrategy) chunkJSONAdaptively(jsonText string, config *adaptiveConfig, metadata core.ChunkMetadata) []core.Chunk {
	// Simple JSON chunking - split by objects/arrays
	var chunks []core.Chunk

	// For now, treat as single chunk (could be enhanced with JSON parsing)
	chunk := core.Chunk{
		Data: jsonText,
		Metadata: core.ChunkMetadata{
			SourcePath:  metadata.SourcePath,
			ChunkID:     fmt.Sprintf("%s:adaptive:json:1", metadata.ChunkID),
			ChunkType:   "semi_structured",
			SizeBytes:   int64(len(jsonText)),
			ProcessedAt: metadata.ProcessedAt,
			ProcessedBy: s.name,
			Quality:     s.calculateAdaptiveQuality(jsonText, "json"),
			Context: map[string]string{
				"chunk_strategy": "adaptive",
				"data_type":      "json",
				"chunk_number":   "1",
			},
			SchemaInfo: map[string]any{
				"adaptive_type": "json",
				"format":        "semi_structured",
			},
		},
	}

	chunks = append(chunks, chunk)
	return chunks
}

func (s *AdaptiveStrategy) processMixedSeparately(array []any, config *adaptiveConfig, metadata core.ChunkMetadata) ([]core.Chunk, error) {
	var chunks []core.Chunk
	chunkNumber := 1

	for _, item := range array {
		dataType := s.analyzeDataType(item)

		var itemChunks []core.Chunk
		var err error

		switch dataType {
		case "text":
			itemChunks, err = s.processAsText(item, config, metadata)
		case "structured":
			itemChunks, err = s.processAsStructured(item, config, metadata)
		case "json":
			itemChunks, err = s.processAsJSON(item, config, metadata)
		default:
			itemChunks, err = s.processAsGeneric(item, config, metadata)
		}

		if err != nil {
			continue // Skip problematic items
		}

		// Update chunk IDs to maintain sequence
		for _, chunk := range itemChunks {
			chunk.Metadata.ChunkID = fmt.Sprintf("%s:adaptive:mixed:%d", metadata.ChunkID, chunkNumber)
			chunk.Metadata.Context["mixed_item_type"] = dataType
			chunks = append(chunks, chunk)
			chunkNumber++
		}
	}

	return chunks, nil
}

func (s *AdaptiveStrategy) processMixedCombined(array []any, config *adaptiveConfig, metadata core.ChunkMetadata) ([]core.Chunk, error) {
	// Combine all items into a single representation
	var combined strings.Builder

	for i, item := range array {
		if i > 0 {
			combined.WriteString("\n---\n")
		}
		combined.WriteString(fmt.Sprintf("%v", item))
	}

	return s.processAsText(combined.String(), config, metadata)
}

func (s *AdaptiveStrategy) processMixedTextOnly(array []any, config *adaptiveConfig, metadata core.ChunkMetadata) ([]core.Chunk, error) {
	var textItems []string

	for _, item := range array {
		if text, ok := item.(string); ok {
			textItems = append(textItems, text)
		}
	}

	if len(textItems) == 0 {
		return []core.Chunk{}, nil
	}

	combinedText := strings.Join(textItems, "\n\n")
	return s.processAsText(combinedText, config, metadata)
}

func (s *AdaptiveStrategy) processMixedStructuredOnly(array []any, config *adaptiveConfig, metadata core.ChunkMetadata) ([]core.Chunk, error) {
	var structuredItems []map[string]any

	for _, item := range array {
		if record, ok := item.(map[string]any); ok {
			structuredItems = append(structuredItems, record)
		}
	}

	if len(structuredItems) == 0 {
		return []core.Chunk{}, nil
	}

	return s.processAsStructured(structuredItems, config, metadata)
}

// Helper methods

type textStructure struct {
	Sections []textSection
}

type textSection struct {
	Content string
	Start   int
	End     int
	Type    string
}

func (s *AdaptiveStrategy) analyzeTextStructure(text string) *textStructure {
	structure := &textStructure{}

	// Simple paragraph-based structure
	paragraphs := strings.Split(text, "\n\n")
	currentPos := 0

	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			currentPos += 2
			continue
		}

		sectionType := "paragraph"
		if s.looksLikeHeader(paragraph) {
			sectionType = "header"
		}

		section := textSection{
			Content: paragraph,
			Start:   currentPos,
			End:     currentPos + len(paragraph),
			Type:    sectionType,
		}

		structure.Sections = append(structure.Sections, section)
		currentPos += len(paragraph) + 2
	}

	return structure
}

func (s *AdaptiveStrategy) createTextChunk(content string, start, chunkNumber int, metadata core.ChunkMetadata) core.Chunk {
	return core.Chunk{
		Data: content,
		Metadata: core.ChunkMetadata{
			SourcePath:    metadata.SourcePath,
			ChunkID:       fmt.Sprintf("%s:adaptive:text:%d", metadata.ChunkID, chunkNumber),
			ChunkType:     "text",
			SizeBytes:     int64(len(content)),
			StartPosition: ptr64(int64(start)),
			EndPosition:   ptr64(int64(start + len(content))),
			ProcessedAt:   metadata.ProcessedAt,
			ProcessedBy:   s.name,
			Quality:       s.calculateAdaptiveQuality(content, "text"),
			Context: map[string]string{
				"chunk_strategy": "adaptive",
				"data_type":      "text",
				"chunk_number":   fmt.Sprintf("%d", chunkNumber),
			},
			SchemaInfo: map[string]any{
				"adaptive_type": "text",
			},
		},
	}
}

func (s *AdaptiveStrategy) convertTableToRecords(table [][]string) []map[string]any {
	if len(table) == 0 {
		return []map[string]any{}
	}

	headers := table[0]
	var records []map[string]any

	for _, row := range table[1:] {
		record := make(map[string]any)
		for i, value := range row {
			if i < len(headers) {
				record[headers[i]] = value
			}
		}
		records = append(records, record)
	}

	return records
}

func (s *AdaptiveStrategy) meetsQualityThreshold(content string, threshold float64) bool {
	quality := s.calculateAdaptiveQuality(content, "text")

	// Simple quality check
	avgQuality := (quality.Completeness + quality.Coherence + quality.Readability) / 3.0
	return avgQuality >= threshold
}

func (s *AdaptiveStrategy) calculateAdaptiveQuality(data any, dataType string) *core.QualityMetrics {
	switch dataType {
	case "text":
		return s.calculateTextQuality(data.(string))
	case "structured":
		if records, ok := data.([]map[string]any); ok {
			return s.calculateStructuredQuality(records)
		}
		return &core.QualityMetrics{Completeness: 0.8, Coherence: 0.9, Readability: 1.0}
	case "json":
		return s.calculateJSONQuality(data.(string))
	default:
		return &core.QualityMetrics{Completeness: 0.5, Coherence: 0.5, Readability: 0.5}
	}
}

func (s *AdaptiveStrategy) calculateTextQuality(text string) *core.QualityMetrics {
	words := strings.Fields(text)
	sentences := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})

	// Basic quality metrics
	completeness := 1.0
	if len(text) > 0 && !strings.HasSuffix(strings.TrimSpace(text), ".") &&
		!strings.HasSuffix(strings.TrimSpace(text), "!") &&
		!strings.HasSuffix(strings.TrimSpace(text), "?") {
		completeness = 0.8
	}

	coherence := 0.5
	if len(sentences) > 0 && len(words) > 0 {
		avgWordsPerSentence := float64(len(words)) / float64(len(sentences))
		if avgWordsPerSentence >= 5 && avgWordsPerSentence <= 30 {
			coherence = 0.8
		}
	}

	readability := 0.7
	if len(words) > 0 {
		avgWordLength := float64(len(text)) / float64(len(words))
		if avgWordLength >= 3 && avgWordLength <= 7 {
			readability = 0.8
		}
	}

	return &core.QualityMetrics{
		Completeness: completeness,
		Coherence:    coherence,
		Readability:  readability,
		Language:     "en",
		LanguageConf: 0.8,
	}
}

func (s *AdaptiveStrategy) calculateStructuredQuality(records []map[string]any) *core.QualityMetrics {
	if len(records) == 0 {
		return &core.QualityMetrics{}
	}

	// Calculate field completeness
	totalFields := 0
	nonEmptyFields := 0

	for _, record := range records {
		for _, value := range record {
			totalFields++
			if value != nil && value != "" {
				nonEmptyFields++
			}
		}
	}

	completeness := 0.0
	if totalFields > 0 {
		completeness = float64(nonEmptyFields) / float64(totalFields)
	}

	return &core.QualityMetrics{
		Completeness: completeness,
		Coherence:    0.9, // Structured data is inherently coherent
		Readability:  1.0, // Always readable
		Language:     "",
		LanguageConf: 0.0,
	}
}

func (s *AdaptiveStrategy) calculateJSONQuality(jsonText string) *core.QualityMetrics {
	// Simple JSON quality assessment
	completeness := 1.0
	if !strings.HasPrefix(strings.TrimSpace(jsonText), "{") && !strings.HasPrefix(strings.TrimSpace(jsonText), "[") {
		completeness = 0.5
	}

	return &core.QualityMetrics{
		Completeness: completeness,
		Coherence:    0.8,
		Readability:  0.9,
		Language:     "",
		LanguageConf: 0.0,
	}
}

// Utility functions

func (s *AdaptiveStrategy) looksLikeJSON(text string) bool {
	text = strings.TrimSpace(text)
	return (strings.HasPrefix(text, "{") && strings.HasSuffix(text, "}")) ||
		(strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]"))
}

func (s *AdaptiveStrategy) looksLikeCSV(text string) bool {
	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		return false
	}

	// Check if first two lines have same number of commas
	firstCommas := strings.Count(lines[0], ",")
	secondCommas := strings.Count(lines[1], ",")

	return firstCommas > 0 && firstCommas == secondCommas
}

func (s *AdaptiveStrategy) looksLikeHeader(text string) bool {
	text = strings.TrimSpace(text)
	return len(text) < 100 && !strings.Contains(text, ".") && !strings.Contains(text, ",")
}

func (s *AdaptiveStrategy) estimateRecordSize(records []map[string]any) int {
	size := 0
	for _, record := range records {
		for key, value := range record {
			size += len(key) + len(fmt.Sprintf("%v", value)) + 10
		}
	}
	return size
}

func (s *AdaptiveStrategy) extractFieldNames(record map[string]any) []string {
	var fields []string
	for key := range record {
		fields = append(fields, key)
	}
	return fields
}

func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

func parseFloat(s string) (float64, error) {
	var result float64
	_, err := fmt.Sscanf(s, "%f", &result)
	return result, err
}

func ptr(f float64) *float64 {
	return &f
}

func ptr64(i int64) *int64 {
	return &i
}
