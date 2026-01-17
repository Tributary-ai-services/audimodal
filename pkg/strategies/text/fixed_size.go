package text

import (
	"context"
	"fmt"
	"log"
	"strings"
	"unicode"

	"github.com/jscharber/audimodal/pkg/core"
)

// FixedSizeStrategy implements ChunkingStrategy for fixed-size text chunks
type FixedSizeStrategy struct {
	name    string
	version string
}

// NewFixedSizeStrategy creates a new fixed-size text chunking strategy
func NewFixedSizeStrategy() *FixedSizeStrategy {
	return &FixedSizeStrategy{
		name:    "fixed_size_text",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (s *FixedSizeStrategy) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "chunk_size",
			Type:        "int",
			Required:    false,
			Default:     1000,
			Description: "Target size of each chunk in characters",
			MinValue:    ptr(100.0),
			MaxValue:    ptr(10000.0),
		},
		{
			Name:        "overlap_size",
			Type:        "int",
			Required:    false,
			Default:     100,
			Description: "Number of characters to overlap between chunks",
			MinValue:    ptr(0.0),
			MaxValue:    ptr(500.0),
		},
		{
			Name:        "split_on_sentences",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Whether to try to split on sentence boundaries",
		},
		{
			Name:        "preserve_paragraphs",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Whether to avoid splitting paragraphs when possible",
		},
		{
			Name:        "min_chunk_size",
			Type:        "int",
			Required:    false,
			Default:     50,
			Description: "Minimum chunk size to create",
			MinValue:    ptr(10.0),
			MaxValue:    ptr(1000.0),
		},
	}
}

// ValidateConfig validates the provided configuration
func (s *FixedSizeStrategy) ValidateConfig(config map[string]any) error {
	chunkSize := 1000
	overlapSize := 100
	minChunkSize := 50

	if cs, ok := config["chunk_size"]; ok {
		if size, ok := cs.(float64); ok {
			if size < 100 || size > 10000 {
				return fmt.Errorf("chunk_size must be between 100 and 10000")
			}
			chunkSize = int(size)
		}
	}

	if os, ok := config["overlap_size"]; ok {
		if size, ok := os.(float64); ok {
			if size < 0 || size > 500 {
				return fmt.Errorf("overlap_size must be between 0 and 500")
			}
			overlapSize = int(size)
		}
	}

	if mcs, ok := config["min_chunk_size"]; ok {
		if size, ok := mcs.(float64); ok {
			if size < 10 || size > 1000 {
				return fmt.Errorf("min_chunk_size must be between 10 and 1000")
			}
			minChunkSize = int(size)
		}
	}

	// Validate relationships between sizes
	if overlapSize >= chunkSize {
		return fmt.Errorf("overlap_size must be less than chunk_size")
	}

	if minChunkSize > chunkSize {
		return fmt.Errorf("min_chunk_size must be less than or equal to chunk_size")
	}

	return nil
}

// TestConnection tests the strategy configuration
func (s *FixedSizeStrategy) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
		Message: "Fixed-size text strategy configuration is valid",
		Details: map[string]any{
			"chunk_size":   config["chunk_size"],
			"overlap_size": config["overlap_size"],
			"split_method": "fixed_size",
		},
	}
}

// GetType returns the connector type
func (s *FixedSizeStrategy) GetType() string {
	return "strategy"
}

// GetName returns the strategy name
func (s *FixedSizeStrategy) GetName() string {
	return s.name
}

// GetVersion returns the strategy version
func (s *FixedSizeStrategy) GetVersion() string {
	return s.version
}

// GetRequiredReaderCapabilities returns capabilities the reader must support
func (s *FixedSizeStrategy) GetRequiredReaderCapabilities() []string {
	return []string{"streaming"}
}

// ConfigureReader modifies reader configuration for this strategy
func (s *FixedSizeStrategy) ConfigureReader(readerConfig map[string]any) (map[string]any, error) {
	// For text chunking, we want to ensure we don't skip empty lines
	// as they might be important for paragraph boundaries
	configured := make(map[string]any)
	for k, v := range readerConfig {
		configured[k] = v
	}

	configured["skip_empty_lines"] = false
	return configured, nil
}

// ProcessChunk processes raw data into chunks using fixed-size strategy
func (s *FixedSizeStrategy) ProcessChunk(ctx context.Context, rawData any, metadata core.ChunkMetadata) ([]core.Chunk, error) {
	log.Printf("[STRATEGY] ProcessChunk called with %d bytes", len(fmt.Sprintf("%v", rawData)))

	// Extract text content
	text, ok := rawData.(string)
	if !ok {
		return nil, fmt.Errorf("fixed-size strategy expects string input, got %T", rawData)
	}
	log.Printf("[STRATEGY] Text extracted, length=%d", len(text))

	// Get configuration
	config := s.getDefaultConfig()
	if metadata.Context != nil {
		if cs, ok := metadata.Context["chunk_size"]; ok {
			if size, err := parseInt(cs); err == nil {
				config.ChunkSize = size
			}
		}
		if os, ok := metadata.Context["overlap_size"]; ok {
			if size, err := parseInt(os); err == nil {
				config.OverlapSize = size
			}
		}
		if ss, ok := metadata.Context["split_on_sentences"]; ok {
			config.SplitOnSentences = ss == "true"
		}
		if pp, ok := metadata.Context["preserve_paragraphs"]; ok {
			config.PreserveParagraphs = pp == "true"
		}
		if mcs, ok := metadata.Context["min_chunk_size"]; ok {
			if size, err := parseInt(mcs); err == nil {
				config.MinChunkSize = size
			}
		}
	}

	// Process text into chunks
	log.Printf("[STRATEGY] Calling chunkText with text length=%d", len(text))
	chunks := s.chunkText(text, config, metadata)
	log.Printf("[STRATEGY] chunkText returned %d chunks", len(chunks))
	return chunks, nil
}

// GetOptimalChunkSize recommends chunk size based on source schema
func (s *FixedSizeStrategy) GetOptimalChunkSize(sourceSchema core.SchemaInfo) int {
	// For unstructured text, use default size
	if sourceSchema.Format == "unstructured" {
		return 1000
	}

	// For other formats, use smaller chunks
	return 500
}

// SupportsParallelProcessing indicates if chunks can be processed concurrently
func (s *FixedSizeStrategy) SupportsParallelProcessing() bool {
	return true // Fixed-size chunks are independent
}

// Internal types and methods

type chunkConfig struct {
	ChunkSize          int
	OverlapSize        int
	SplitOnSentences   bool
	PreserveParagraphs bool
	MinChunkSize       int
}

func (s *FixedSizeStrategy) getDefaultConfig() chunkConfig {
	return chunkConfig{
		ChunkSize:          1000,
		OverlapSize:        100,
		SplitOnSentences:   true,
		PreserveParagraphs: false,
		MinChunkSize:       50,
	}
}

func (s *FixedSizeStrategy) chunkText(text string, config chunkConfig, metadata core.ChunkMetadata) []core.Chunk {
	var chunks []core.Chunk

	log.Printf("[STRATEGY] chunkText: calling normalizeText on %d bytes", len(text))
	// Remove excessive whitespace but preserve paragraph breaks
	text = normalizeText(text)
	log.Printf("[STRATEGY] chunkText: normalizeText returned %d bytes", len(text))

	if len(text) <= config.ChunkSize {
		// Text is smaller than chunk size, return as single chunk
		// MEMORY FIX: Clone to avoid holding reference to potentially larger string
		chunk := core.Chunk{
			Data: strings.Clone(text),
			Metadata: core.ChunkMetadata{
				SourcePath:  metadata.SourcePath,
				ChunkID:     fmt.Sprintf("%s:chunk:1", metadata.ChunkID),
				ChunkType:   "text",
				SizeBytes:   int64(len(text)),
				ProcessedAt: metadata.ProcessedAt,
				ProcessedBy: s.name,
				Quality:     s.calculateQuality(text),
				Context: map[string]string{
					"chunk_strategy": "fixed_size",
					"chunk_number":   "1",
					"total_chunks":   "1",
				},
			},
		}
		chunks = append(chunks, chunk)
		return chunks
	}

	// Split text into chunks
	chunkNumber := 1
	start := 0
	loopCount := 0
	maxLoops := 10000 // Safety limit to prevent infinite loops

	for start < len(text) {
		loopCount++
		if loopCount > maxLoops {
			log.Printf("[STRATEGY] ERROR: Exceeded max loop count %d, breaking to prevent hang", maxLoops)
			break
		}
		if loopCount%100 == 0 {
			log.Printf("[STRATEGY] chunkText loop iteration %d, start=%d, textLen=%d", loopCount, start, len(text))
		}

		end := start + config.ChunkSize

		// Don't go beyond text length
		if end > len(text) {
			end = len(text)
		}

		// Try to find better break point if requested
		if config.SplitOnSentences && end < len(text) {
			betterEnd := s.findSentenceBreak(text, start, end)
			if betterEnd > start+config.MinChunkSize {
				end = betterEnd
			}
		}

		if config.PreserveParagraphs && end < len(text) {
			betterEnd := s.findParagraphBreak(text, start, end)
			if betterEnd > start+config.MinChunkSize {
				end = betterEnd
			}
		}

		// Safety check for slice bounds
		if start < 0 {
			start = 0
		}
		if end > len(text) {
			end = len(text)
		}
		if start >= end {
			break
		}

		// MEMORY FIX: Use strings.Clone() to copy the substring
		// String slicing in Go creates a reference to the original string's backing array
		// Without cloning, the entire original text stays in memory
		chunkText := strings.Clone(strings.TrimSpace(text[start:end]))

		// Only create chunk if it meets minimum size
		if len(chunkText) >= config.MinChunkSize {
			chunk := core.Chunk{
				Data: chunkText,
				Metadata: core.ChunkMetadata{
					SourcePath:    metadata.SourcePath,
					ChunkID:       fmt.Sprintf("%s:chunk:%d", metadata.ChunkID, chunkNumber),
					ChunkType:     "text",
					SizeBytes:     int64(len(chunkText)),
					StartPosition: ptr64(int64(start)),
					EndPosition:   ptr64(int64(end)),
					ProcessedAt:   metadata.ProcessedAt,
					ProcessedBy:   s.name,
					Quality:       s.calculateQuality(chunkText),
					Context: map[string]string{
						"chunk_strategy": "fixed_size",
						"chunk_number":   fmt.Sprintf("%d", chunkNumber),
						"start_pos":      fmt.Sprintf("%d", start),
						"end_pos":        fmt.Sprintf("%d", end),
					},
				},
			}
			chunks = append(chunks, chunk)
			chunkNumber++
		}

		// Move to next chunk with overlap
		oldStart := start
		if config.OverlapSize > 0 && end < len(text) {
			start = end - config.OverlapSize
		} else {
			start = end
		}

		// CRITICAL FIX: Ensure start always advances to prevent infinite loop
		// This can happen when findSentenceBreak returns end close to start + OverlapSize
		// When stuck, skip overlap entirely and advance to end position
		if start <= oldStart {
			start = end // Skip overlap, advance to end position
			log.Printf("[STRATEGY] WARNING: overlap caused no progress, skipping to end=%d (was %d)", start, oldStart)
		}

		// Avoid infinite loop
		if start >= len(text) {
			break
		}
	}

	// Update total chunks in context
	for i := range chunks {
		chunks[i].Metadata.Context["total_chunks"] = fmt.Sprintf("%d", len(chunks))
	}

	return chunks
}

func (s *FixedSizeStrategy) findSentenceBreak(text string, start, end int) int {
	// Validate input parameters
	if start < 0 {
		start = 0
	}
	if end > len(text) {
		end = len(text)
	}
	if start >= end {
		return end
	}

	// PERFORMANCE FIX: Use strings.LastIndexAny instead of char-by-char iteration
	// Search backwards from end position for sentence-ending punctuation
	searchText := text[start:end]

	// Limit iterations to prevent slow processing on text with many non-sentence periods
	maxIterations := 50

	// Find the last occurrence of any sentence ender
	for i := 0; i < maxIterations; i++ {
		idx := strings.LastIndexAny(searchText, ".!?")
		if idx == -1 {
			break
		}

		// Calculate absolute position
		absPos := start + idx

		// Check if next character is whitespace or end of text
		if absPos+1 >= len(text) || unicode.IsSpace(rune(text[absPos+1])) {
			return absPos + 1
		}

		// Not a valid sentence break (e.g., decimal point), search earlier
		if idx == 0 {
			break
		}
		searchText = searchText[:idx]
	}

	return end
}

func (s *FixedSizeStrategy) findParagraphBreak(text string, start, end int) int {
	// Validate input parameters
	if start < 0 {
		start = 0
	}
	if end > len(text) {
		end = len(text)
	}
	if start >= end {
		return end
	}

	// Look for double newlines (paragraph breaks)
	for i := end - 1; i > start+1; i-- {
		if text[i] == '\n' && text[i-1] == '\n' {
			return i + 1
		}
	}

	return end
}

func (s *FixedSizeStrategy) calculateQuality(text string) *core.QualityMetrics {
	if text == "" {
		return &core.QualityMetrics{
			Completeness: 0.0,
			Coherence:    0.0,
			Readability:  0.0,
		}
	}

	// PERFORMANCE FIX: Single-pass counting instead of multiple allocating operations
	// Count words and sentences in one pass without allocating arrays
	wordCount := 0
	sentenceCount := 0
	inWord := false
	textLen := len(text)

	for i := 0; i < textLen; i++ {
		c := text[i]

		// Count words (transitions from whitespace to non-whitespace)
		isSpace := c == ' ' || c == '\t' || c == '\n' || c == '\r'
		if !isSpace {
			if !inWord {
				wordCount++
				inWord = true
			}
		} else {
			inWord = false
		}

		// Count sentences
		if c == '.' || c == '!' || c == '?' {
			sentenceCount++
		}
	}

	// Completeness: based on sentence completeness
	completeness := 1.0
	if textLen > 0 {
		// Check last non-whitespace character
		lastChar := byte(0)
		for i := textLen - 1; i >= 0; i-- {
			c := text[i]
			if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
				lastChar = c
				break
			}
		}
		if lastChar != '.' && lastChar != '!' && lastChar != '?' {
			completeness = 0.8 // Incomplete sentence
		}
	}

	// Coherence: based on word-to-sentence ratio
	coherence := 0.5 // Default
	if sentenceCount > 0 {
		avgWordsPerSentence := float64(wordCount) / float64(sentenceCount)
		if avgWordsPerSentence >= 10 && avgWordsPerSentence <= 25 {
			coherence = 0.9 // Good sentence structure
		} else if avgWordsPerSentence >= 5 && avgWordsPerSentence <= 35 {
			coherence = 0.7 // Acceptable
		}
	}

	// Readability: simple approximation based on word and sentence length
	readability := 0.5 // Default
	if wordCount > 0 && sentenceCount > 0 {
		avgWordLength := float64(textLen) / float64(wordCount)
		avgSentenceLength := float64(wordCount) / float64(sentenceCount)

		// Flesch-like approximation
		if avgWordLength < 6 && avgSentenceLength < 20 {
			readability = 0.8
		} else if avgWordLength < 8 && avgSentenceLength < 30 {
			readability = 0.6
		}
	}

	return &core.QualityMetrics{
		Completeness: completeness,
		Coherence:    coherence,
		Readability:  readability,
		Language:     "en", // Default to English
		LanguageConf: 0.8,  // Default confidence
	}
}

// Helper functions

func normalizeText(text string) string {
	// PERFORMANCE FIX: Use streaming approach instead of strings.Fields
	// which allocates a slice of all words.
	// This is critical for large documents (500+ pages)

	if len(text) == 0 {
		return text
	}

	// Pre-allocate result buffer with estimated size
	var result strings.Builder
	result.Grow(len(text))

	inWhitespace := false
	pendingNewline := false

	for i := 0; i < len(text); i++ {
		c := text[i]

		if c == ' ' || c == '\t' || c == '\r' {
			// Regular whitespace - collapse to single space
			if !inWhitespace {
				result.WriteByte(' ')
				inWhitespace = true
			}
			pendingNewline = false
		} else if c == '\n' {
			// Newline handling - preserve paragraph breaks (double newlines)
			if pendingNewline {
				// Second newline - this is a paragraph break
				// We already wrote a space for the first newline, overwrite it
				// by writing \n\n (the space will be trimmed or absorbed)
				result.WriteString("\n\n")
				inWhitespace = true
				pendingNewline = false
			} else {
				// First newline - write a space and mark as pending
				if !inWhitespace {
					result.WriteByte(' ')
					inWhitespace = true
				}
				pendingNewline = true
			}
		} else {
			// Regular character
			result.WriteByte(c)
			inWhitespace = false
			pendingNewline = false
		}
	}

	return strings.TrimSpace(result.String())
}

func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

func ptr(f float64) *float64 {
	return &f
}

func ptr64(i int64) *int64 {
	return &i
}
