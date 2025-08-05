package text

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// SemanticStrategy implements ChunkingStrategy for semantic text chunks
type SemanticStrategy struct {
	name    string
	version string
}

// NewSemanticStrategy creates a new semantic text chunking strategy
func NewSemanticStrategy() *SemanticStrategy {
	return &SemanticStrategy{
		name:    "semantic_text",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (s *SemanticStrategy) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "max_chunk_size",
			Type:        "int",
			Required:    false,
			Default:     2000,
			Description: "Maximum size of each chunk in characters",
			MinValue:    ptr(500.0),
			MaxValue:    ptr(10000.0),
		},
		{
			Name:        "min_chunk_size",
			Type:        "int",
			Required:    false,
			Default:     100,
			Description: "Minimum chunk size to create",
			MinValue:    ptr(50.0),
			MaxValue:    ptr(1000.0),
		},
		{
			Name:        "overlap_sentences",
			Type:        "int",
			Required:    false,
			Default:     1,
			Description: "Number of sentences to overlap between chunks",
			MinValue:    ptr(0.0),
			MaxValue:    ptr(5.0),
		},
		{
			Name:        "respect_sections",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Whether to respect document sections (headers, paragraphs)",
		},
		{
			Name:        "prefer_sentence_breaks",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Whether to prefer breaking at sentence boundaries",
		},
		{
			Name:        "detect_lists",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Whether to detect and preserve list structures",
		},
	}
}

// ValidateConfig validates the provided configuration
func (s *SemanticStrategy) ValidateConfig(config map[string]any) error {
	maxChunkSize := 2000
	minChunkSize := 100

	if mcs, ok := config["max_chunk_size"]; ok {
		if size, ok := mcs.(float64); ok {
			if size < 500 || size > 10000 {
				return fmt.Errorf("max_chunk_size must be between 500 and 10000")
			}
			maxChunkSize = int(size)
		}
	}

	if mcs, ok := config["min_chunk_size"]; ok {
		if size, ok := mcs.(float64); ok {
			if size < 50 || size > 1000 {
				return fmt.Errorf("min_chunk_size must be between 50 and 1000")
			}
			minChunkSize = int(size)
		}
	}

	if os, ok := config["overlap_sentences"]; ok {
		if overlap, ok := os.(float64); ok {
			if overlap < 0 || overlap > 5 {
				return fmt.Errorf("overlap_sentences must be between 0 and 5")
			}
		}
	}

	// Validate size relationship
	if minChunkSize >= maxChunkSize {
		return fmt.Errorf("min_chunk_size must be less than max_chunk_size")
	}

	return nil
}

// TestConnection tests the strategy configuration
func (s *SemanticStrategy) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
		Message: "Semantic text strategy configuration is valid",
		Details: map[string]any{
			"max_chunk_size": config["max_chunk_size"],
			"min_chunk_size": config["min_chunk_size"],
			"split_method":   "semantic",
		},
	}
}

// GetType returns the connector type
func (s *SemanticStrategy) GetType() string {
	return "strategy"
}

// GetName returns the strategy name
func (s *SemanticStrategy) GetName() string {
	return s.name
}

// GetVersion returns the strategy version
func (s *SemanticStrategy) GetVersion() string {
	return s.version
}

// GetRequiredReaderCapabilities returns capabilities the reader must support
func (s *SemanticStrategy) GetRequiredReaderCapabilities() []string {
	return []string{"streaming"}
}

// ConfigureReader modifies reader configuration for this strategy
func (s *SemanticStrategy) ConfigureReader(readerConfig map[string]any) (map[string]any, error) {
	configured := make(map[string]any)
	for k, v := range readerConfig {
		configured[k] = v
	}

	// Preserve all text structure for semantic analysis
	configured["skip_empty_lines"] = false
	return configured, nil
}

// ProcessChunk processes raw data into chunks using semantic strategy
func (s *SemanticStrategy) ProcessChunk(ctx context.Context, rawData any, metadata core.ChunkMetadata) ([]core.Chunk, error) {
	text, ok := rawData.(string)
	if !ok {
		return nil, fmt.Errorf("semantic strategy expects string input, got %T", rawData)
	}

	// Get configuration
	config := s.getDefaultConfig()
	if metadata.Context != nil {
		s.updateConfigFromContext(config, metadata.Context)
	}

	// Process text into semantic chunks
	chunks := s.chunkTextSemantically(text, config, metadata)
	return chunks, nil
}

// GetOptimalChunkSize recommends chunk size based on source schema
func (s *SemanticStrategy) GetOptimalChunkSize(sourceSchema core.SchemaInfo) int {
	// For unstructured text, use larger chunks for better semantic coherence
	if sourceSchema.Format == "unstructured" {
		return 1500
	}

	return 1000
}

// SupportsParallelProcessing indicates if chunks can be processed concurrently
func (s *SemanticStrategy) SupportsParallelProcessing() bool {
	return true // Semantic chunks are independent once created
}

// Internal types and methods

type semanticConfig struct {
	MaxChunkSize         int
	MinChunkSize         int
	OverlapSentences     int
	RespectSections      bool
	PreferSentenceBreaks bool
	DetectLists          bool
}

func (s *SemanticStrategy) getDefaultConfig() *semanticConfig {
	return &semanticConfig{
		MaxChunkSize:         2000,
		MinChunkSize:         100,
		OverlapSentences:     1,
		RespectSections:      true,
		PreferSentenceBreaks: true,
		DetectLists:          true,
	}
}

func (s *SemanticStrategy) updateConfigFromContext(config *semanticConfig, context map[string]string) {
	if mcs, ok := context["max_chunk_size"]; ok {
		if size, err := parseInt(mcs); err == nil {
			config.MaxChunkSize = size
		}
	}
	if mcs, ok := context["min_chunk_size"]; ok {
		if size, err := parseInt(mcs); err == nil {
			config.MinChunkSize = size
		}
	}
	if os, ok := context["overlap_sentences"]; ok {
		if overlap, err := parseInt(os); err == nil {
			config.OverlapSentences = overlap
		}
	}
	if rs, ok := context["respect_sections"]; ok {
		config.RespectSections = rs == "true"
	}
	if psb, ok := context["prefer_sentence_breaks"]; ok {
		config.PreferSentenceBreaks = psb == "true"
	}
	if dl, ok := context["detect_lists"]; ok {
		config.DetectLists = dl == "true"
	}
}

func (s *SemanticStrategy) chunkTextSemantically(text string, config *semanticConfig, metadata core.ChunkMetadata) []core.Chunk {
	var chunks []core.Chunk

	// Analyze text structure
	structure := s.analyzeTextStructure(text, config)

	// Create chunks based on semantic units
	chunks = s.createSemanticChunks(structure, config, metadata)

	// Add overlap if requested
	if config.OverlapSentences > 0 {
		chunks = s.addSentenceOverlap(chunks, config.OverlapSentences)
	}

	return chunks
}

type textStructure struct {
	Sections   []textSection
	Paragraphs []textParagraph
	Sentences  []textSentence
	Lists      []textList
}

type textSection struct {
	Title   string
	Level   int
	Start   int
	End     int
	Content string
}

type textParagraph struct {
	Start   int
	End     int
	Content string
	Type    string // "normal", "list", "header"
}

type textSentence struct {
	Start   int
	End     int
	Content string
	InList  bool
}

type textList struct {
	Start int
	End   int
	Items []string
	Type  string // "ordered", "unordered"
}

func (s *SemanticStrategy) analyzeTextStructure(text string, config *semanticConfig) *textStructure {
	structure := &textStructure{}

	// Split into paragraphs
	structure.Paragraphs = s.identifyParagraphs(text)

	// Identify sections/headers
	if config.RespectSections {
		structure.Sections = s.identifySections(text)
	}

	// Split into sentences
	structure.Sentences = s.identifySentences(text)

	// Detect lists
	if config.DetectLists {
		structure.Lists = s.identifyLists(text)
	}

	return structure
}

func (s *SemanticStrategy) identifyParagraphs(text string) []textParagraph {
	var paragraphs []textParagraph

	// Split on double newlines
	parts := strings.Split(text, "\n\n")
	currentPos := 0

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			currentPos += 2 // Account for the double newline
			continue
		}

		paragraphType := "normal"

		// Check if it's a header (simple heuristic)
		if s.looksLikeHeader(part) {
			paragraphType = "header"
		} else if s.looksLikeList(part) {
			paragraphType = "list"
		}

		paragraph := textParagraph{
			Start:   currentPos,
			End:     currentPos + len(part),
			Content: part,
			Type:    paragraphType,
		}

		paragraphs = append(paragraphs, paragraph)
		currentPos += len(part) + 2
	}

	return paragraphs
}

func (s *SemanticStrategy) identifySections(text string) []textSection {
	var sections []textSection

	// Simple markdown-style header detection
	headerRegex := regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)
	matches := headerRegex.FindAllStringSubmatch(text, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			level := len(match[1]) // Number of # characters
			title := strings.TrimSpace(match[2])

			// Find position in text
			start := strings.Index(text, match[0])
			if start >= 0 {
				section := textSection{
					Title: title,
					Level: level,
					Start: start,
					End:   start + len(match[0]),
				}
				sections = append(sections, section)
			}
		}
	}

	return sections
}

func (s *SemanticStrategy) identifySentences(text string) []textSentence {
	var sentences []textSentence

	// Simple sentence splitting
	sentenceRegex := regexp.MustCompile(`[.!?]+\s+`)
	parts := sentenceRegex.Split(text, -1)

	currentPos := 0
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Add back the punctuation except for the last part
		if i < len(parts)-1 {
			part += "."
		}

		sentence := textSentence{
			Start:   currentPos,
			End:     currentPos + len(part),
			Content: part,
		}

		sentences = append(sentences, sentence)
		currentPos += len(part) + 1
	}

	return sentences
}

func (s *SemanticStrategy) identifyLists(text string) []textList {
	var lists []textList

	// Find bullet lists and numbered lists
	listItemRegex := regexp.MustCompile(`(?m)^[\s]*[-\*\+][\s]+(.+)$|^[\s]*\d+\.[\s]+(.+)$`)
	matches := listItemRegex.FindAllStringSubmatch(text, -1)

	if len(matches) > 0 {
		var items []string
		start := strings.Index(text, matches[0][0])
		end := start

		for _, match := range matches {
			item := match[1]
			if item == "" {
				item = match[2] // Numbered list item
			}
			items = append(items, strings.TrimSpace(item))
			end = strings.Index(text, match[0]) + len(match[0])
		}

		listType := "unordered"
		if strings.Contains(matches[0][0], ".") {
			listType = "ordered"
		}

		list := textList{
			Start: start,
			End:   end,
			Items: items,
			Type:  listType,
		}
		lists = append(lists, list)
	}

	return lists
}

func (s *SemanticStrategy) createSemanticChunks(structure *textStructure, config *semanticConfig, metadata core.ChunkMetadata) []core.Chunk {
	var chunks []core.Chunk

	// Start with paragraphs as base units
	currentChunk := ""
	chunkStart := 0
	chunkNumber := 1

	for _, paragraph := range structure.Paragraphs {
		// Check if adding this paragraph would exceed max size
		proposedChunk := currentChunk
		if proposedChunk != "" {
			proposedChunk += "\n\n"
		}
		proposedChunk += paragraph.Content

		if len(proposedChunk) > config.MaxChunkSize && len(currentChunk) >= config.MinChunkSize {
			// Create chunk with current content
			chunk := s.createChunk(currentChunk, chunkStart, chunkNumber, metadata)
			chunks = append(chunks, chunk)

			// Start new chunk
			currentChunk = paragraph.Content
			chunkStart = paragraph.Start
			chunkNumber++
		} else {
			// Add to current chunk
			if currentChunk != "" {
				currentChunk += "\n\n"
			}
			currentChunk += paragraph.Content
			if chunkStart == 0 {
				chunkStart = paragraph.Start
			}
		}
	}

	// Add final chunk if it has content
	if len(currentChunk) >= config.MinChunkSize {
		chunk := s.createChunk(currentChunk, chunkStart, chunkNumber, metadata)
		chunks = append(chunks, chunk)
	}

	// Update total chunks in context
	for i := range chunks {
		chunks[i].Metadata.Context["total_chunks"] = fmt.Sprintf("%d", len(chunks))
	}

	return chunks
}

func (s *SemanticStrategy) createChunk(content string, start, chunkNumber int, metadata core.ChunkMetadata) core.Chunk {
	return core.Chunk{
		Data: content,
		Metadata: core.ChunkMetadata{
			SourcePath:    metadata.SourcePath,
			ChunkID:       fmt.Sprintf("%s:semantic:%d", metadata.ChunkID, chunkNumber),
			ChunkType:     "text",
			SizeBytes:     int64(len(content)),
			StartPosition: ptr64(int64(start)),
			EndPosition:   ptr64(int64(start + len(content))),
			ProcessedAt:   metadata.ProcessedAt,
			ProcessedBy:   s.name,
			Quality:       s.calculateSemanticQuality(content),
			Context: map[string]string{
				"chunk_strategy": "semantic",
				"chunk_number":   fmt.Sprintf("%d", chunkNumber),
				"start_pos":      fmt.Sprintf("%d", start),
			},
		},
	}
}

func (s *SemanticStrategy) addSentenceOverlap(chunks []core.Chunk, overlapSentences int) []core.Chunk {
	if len(chunks) <= 1 || overlapSentences <= 0 {
		return chunks
	}

	for i := 1; i < len(chunks); i++ {
		prevContent := chunks[i-1].Data.(string)
		currContent := chunks[i].Data.(string)

		// Get last sentences from previous chunk
		prevSentences := s.getLastSentences(prevContent, overlapSentences)
		if prevSentences != "" {
			// Prepend to current chunk
			chunks[i].Data = prevSentences + " " + currContent
			chunks[i].Metadata.SizeBytes = int64(len(chunks[i].Data.(string)))
		}
	}

	return chunks
}

func (s *SemanticStrategy) getLastSentences(text string, count int) string {
	sentences := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})

	if len(sentences) <= count {
		return text
	}

	// Get last 'count' sentences
	lastSentences := sentences[len(sentences)-count:]
	return strings.Join(lastSentences, ". ") + "."
}

func (s *SemanticStrategy) calculateSemanticQuality(text string) *core.QualityMetrics {
	// Enhanced quality calculation for semantic chunks
	words := strings.Fields(text)
	sentences := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})
	paragraphs := strings.Split(text, "\n\n")

	// Completeness: based on paragraph and sentence structure
	completeness := 1.0
	if !s.endsWithPunctuation(text) {
		completeness = 0.7
	}

	// Coherence: based on semantic structure
	coherence := 0.5
	if len(paragraphs) > 1 {
		coherence = 0.8 // Multiple paragraphs suggest better structure
	}
	if s.hasGoodSentenceFlow(sentences) {
		coherence += 0.1
	}

	// Readability
	readability := 0.5
	if len(words) > 0 && len(sentences) > 0 {
		avgWordsPerSentence := float64(len(words)) / float64(len(sentences))
		if avgWordsPerSentence >= 8 && avgWordsPerSentence <= 22 {
			readability = 0.8
		}
	}

	// Uniqueness: simple approximation
	uniqueness := 0.7
	uniqueWords := make(map[string]bool)
	for _, word := range words {
		uniqueWords[strings.ToLower(word)] = true
	}
	if len(words) > 0 {
		uniqueness = float64(len(uniqueWords)) / float64(len(words))
	}

	return &core.QualityMetrics{
		Completeness: completeness,
		Coherence:    coherence,
		Uniqueness:   uniqueness,
		Readability:  readability,
		Language:     "en",
		LanguageConf: 0.8,
	}
}

// Helper functions

func (s *SemanticStrategy) looksLikeHeader(text string) bool {
	// Simple heuristics for header detection
	text = strings.TrimSpace(text)

	// Markdown headers
	if strings.HasPrefix(text, "#") {
		return true
	}

	// Short lines that don't end with punctuation
	if len(text) < 100 && !s.endsWithPunctuation(text) {
		return true
	}

	// All caps (likely a header)
	if text == strings.ToUpper(text) && len(text) < 100 {
		return true
	}

	return false
}

func (s *SemanticStrategy) looksLikeList(text string) bool {
	// Check for list markers
	listMarkers := []string{"- ", "* ", "+ "}
	for _, marker := range listMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}

	// Check for numbered lists
	numberedList := regexp.MustCompile(`\d+\.\s`)
	return numberedList.MatchString(text)
}

func (s *SemanticStrategy) endsWithPunctuation(text string) bool {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return false
	}

	lastChar := rune(text[len(text)-1])
	return lastChar == '.' || lastChar == '!' || lastChar == '?' || lastChar == ':'
}

func (s *SemanticStrategy) hasGoodSentenceFlow(sentences []string) bool {
	if len(sentences) < 2 {
		return true
	}

	// Simple check: sentences should vary in length
	lengths := make([]int, len(sentences))
	for i, sentence := range sentences {
		lengths[i] = len(strings.Fields(sentence))
	}

	// Calculate variance
	if len(lengths) > 1 {
		sum := 0
		for _, length := range lengths {
			sum += length
		}
		avg := float64(sum) / float64(len(lengths))

		variance := 0.0
		for _, length := range lengths {
			variance += (float64(length) - avg) * (float64(length) - avg)
		}
		variance /= float64(len(lengths))

		// If variance is reasonable, it suggests good flow
		return variance > 4.0 && variance < 100.0
	}

	return true
}
