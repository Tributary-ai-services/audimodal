package rtf

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// RTFReader implements DataSourceReader for Rich Text Format files
type RTFReader struct {
	name    string
	version string
}

// NewRTFReader creates a new RTF file reader
func NewRTFReader() *RTFReader {
	return &RTFReader{
		name:    "rtf_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *RTFReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "extract_text_only",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract only text content, stripping all formatting",
		},
		{
			Name:        "preserve_formatting",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Preserve basic formatting information",
		},
		{
			Name:        "extract_metadata",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract document metadata from RTF info group",
		},
		{
			Name:        "extract_tables",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract and structure table content",
		},
		{
			Name:        "max_chunk_size",
			Type:        "int",
			Required:    false,
			Default:     2000,
			Description: "Maximum characters per chunk",
			MinValue:    ptrFloat64(100.0),
			MaxValue:    ptrFloat64(10000.0),
		},
		{
			Name:        "encoding",
			Type:        "string",
			Required:    false,
			Default:     "auto",
			Description: "Character encoding for RTF file",
			Enum:        []string{"auto", "windows-1252", "utf-8", "iso-8859-1"},
		},
		{
			Name:        "skip_headers_footers",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Skip header and footer content",
		},
		{
			Name:        "extract_images",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract embedded images as base64",
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *RTFReader) ValidateConfig(config map[string]any) error {
	if maxSize, ok := config["max_chunk_size"]; ok {
		if num, ok := maxSize.(float64); ok {
			if num < 100 || num > 10000 {
				return fmt.Errorf("max_chunk_size must be between 100 and 10000")
			}
		}
	}

	if encoding, ok := config["encoding"]; ok {
		if str, ok := encoding.(string); ok {
			validEncodings := []string{"auto", "windows-1252", "utf-8", "iso-8859-1"}
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

	return nil
}

// TestConnection tests if the RTF can be read
func (r *RTFReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
		Message: "RTF reader ready",
		Latency: time.Since(start),
		Details: map[string]any{
			"extract_text_only": config["extract_text_only"],
			"encoding":          config["encoding"],
		},
	}
}

// GetType returns the connector type
func (r *RTFReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *RTFReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *RTFReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the RTF file structure
func (r *RTFReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to read file: %w", err)
	}

	// Check if it's a valid RTF file
	if !bytes.HasPrefix(content, []byte("{\\rtf")) {
		return core.SchemaInfo{}, fmt.Errorf("not a valid RTF file")
	}

	metadata := r.extractRTFMetadata(string(content))
	structure := r.analyzeRTFStructure(string(content))

	schema := core.SchemaInfo{
		Format:   "rtf",
		Encoding: "windows-1252", // Default RTF encoding
		Fields: []core.FieldInfo{
			{
				Name:        "content",
				Type:        "text",
				Nullable:    false,
				Description: "Extracted text content",
			},
			{
				Name:        "paragraph_number",
				Type:        "integer",
				Nullable:    false,
				Description: "Paragraph number in document",
			},
			{
				Name:        "formatting",
				Type:        "object",
				Nullable:    true,
				Description: "Text formatting information",
			},
			{
				Name:        "style",
				Type:        "string",
				Nullable:    true,
				Description: "Paragraph style name",
			},
		},
		Metadata: map[string]any{
			"title":           metadata.Title,
			"author":          metadata.Author,
			"subject":         metadata.Subject,
			"keywords":        metadata.Keywords,
			"comment":         metadata.Comment,
			"created_date":    metadata.CreatedDate,
			"modified_date":   metadata.ModifiedDate,
			"generator":       metadata.Generator,
			"rtf_version":     metadata.RTFVersion,
			"paragraph_count": structure.ParagraphCount,
			"has_tables":      structure.HasTables,
			"has_images":      structure.HasImages,
			"has_hyperlinks":  structure.HasHyperlinks,
		},
	}

	// Extract sample content
	sampleText := r.extractSampleText(string(content))
	if sampleText != "" {
		schema.SampleData = []map[string]any{
			{
				"content":          sampleText,
				"paragraph_number": 1,
			},
		}
	}

	return schema, nil
}

// EstimateSize returns size estimates for the RTF file
func (r *RTFReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to read file: %w", err)
	}

	// Extract plain text for size estimation
	plainText := r.extractPlainText(string(content))
	textSize := int64(len(plainText))

	// Count paragraphs
	paragraphCount := int64(r.countParagraphs(string(content)))

	// Estimate chunks based on text size
	chunkSize := int64(1000)
	estimatedChunks := int((textSize + chunkSize - 1) / chunkSize)

	complexity := "low"
	if stat.Size() > 500*1024 { // > 500KB
		complexity = "medium"
	}
	if stat.Size() > 5*1024*1024 { // > 5MB
		complexity = "high"
	}

	processTime := "fast"
	if stat.Size() > 1*1024*1024 { // > 1MB
		processTime = "medium"
	}
	if stat.Size() > 10*1024*1024 { // > 10MB
		processTime = "slow"
	}

	return core.SizeEstimate{
		RowCount:    &paragraphCount,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the RTF file
func (r *RTFReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read RTF file: %w", err)
	}

	paragraphs := r.parseRTFParagraphs(string(content), strategyConfig)

	iterator := &RTFIterator{
		sourcePath:       sourcePath,
		config:           strategyConfig,
		paragraphs:       paragraphs,
		currentParagraph: 0,
	}

	return iterator, nil
}

// SupportsStreaming indicates RTF reader supports streaming
func (r *RTFReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *RTFReader) GetSupportedFormats() []string {
	return []string{"rtf"}
}

// RTFMetadata contains extracted RTF metadata
type RTFMetadata struct {
	Title        string
	Author       string
	Subject      string
	Keywords     string
	Comment      string
	CreatedDate  string
	ModifiedDate string
	Generator    string
	RTFVersion   string
}

// RTFStructure contains RTF structure analysis
type RTFStructure struct {
	ParagraphCount int
	HasTables      bool
	HasImages      bool
	HasHyperlinks  bool
}

// RTFParagraph represents a parsed RTF paragraph
type RTFParagraph struct {
	Content    string
	Number     int
	Style      string
	Formatting map[string]any
}

// extractRTFMetadata extracts metadata from RTF content
func (r *RTFReader) extractRTFMetadata(content string) RTFMetadata {
	metadata := RTFMetadata{
		RTFVersion: "1", // Default
	}

	// Extract RTF version
	if match := regexp.MustCompile(`\\rtf(\d+)`).FindStringSubmatch(content); len(match) > 1 {
		metadata.RTFVersion = match[1]
	}

	// Look for info group - need to handle nested braces properly
	if infoStart := strings.Index(content, "{\\info"); infoStart >= 0 {
		// Find the matching closing brace
		level := 1
		end := infoStart + 7 // After "{\\info"
		infoContent := ""

		for end < len(content) && level > 0 {
			if content[end] == '{' {
				level++
			} else if content[end] == '}' {
				level--
			}
			if level > 0 {
				infoContent += string(content[end])
			}
			end++
		}

		info := infoContent

		// Extract title
		if titleMatch := regexp.MustCompile(`\\title\s*([^}\\]+)`).FindStringSubmatch(info); len(titleMatch) > 1 {
			metadata.Title = r.decodeRTFString(titleMatch[1])
		}

		// Extract author
		if authorMatch := regexp.MustCompile(`\\author\s*([^}\\]+)`).FindStringSubmatch(info); len(authorMatch) > 1 {
			metadata.Author = r.decodeRTFString(authorMatch[1])
		}

		// Extract subject
		if subjectMatch := regexp.MustCompile(`\\subject\s*([^}\\]+)`).FindStringSubmatch(info); len(subjectMatch) > 1 {
			metadata.Subject = r.decodeRTFString(subjectMatch[1])
		}

		// Extract keywords
		if keywordsMatch := regexp.MustCompile(`\\keywords\s*([^}\\]+)`).FindStringSubmatch(info); len(keywordsMatch) > 1 {
			metadata.Keywords = r.decodeRTFString(keywordsMatch[1])
		}

		// Extract comment
		if commentMatch := regexp.MustCompile(`\\comment\s*([^}\\]+)`).FindStringSubmatch(info); len(commentMatch) > 1 {
			metadata.Comment = r.decodeRTFString(commentMatch[1])
		}

		// Extract generator
		if genMatch := regexp.MustCompile(`\\generator\s*([^}\\]+)`).FindStringSubmatch(info); len(genMatch) > 1 {
			metadata.Generator = r.decodeRTFString(genMatch[1])
		}

		// Extract dates
		// RTF dates are in the format: \yrYYYY\moMM\dyDD\hrHH\minMM\secSS
		if createdMatch := regexp.MustCompile(`\\creatim\\yr(\d+)\\mo(\d+)\\dy(\d+)`).FindStringSubmatch(info); len(createdMatch) > 3 {
			metadata.CreatedDate = fmt.Sprintf("%s-%02s-%02s", createdMatch[1], createdMatch[2], createdMatch[3])
		}

		if modifiedMatch := regexp.MustCompile(`\\revtim\\yr(\d+)\\mo(\d+)\\dy(\d+)`).FindStringSubmatch(info); len(modifiedMatch) > 3 {
			metadata.ModifiedDate = fmt.Sprintf("%s-%02s-%02s", modifiedMatch[1], modifiedMatch[2], modifiedMatch[3])
		}
	}

	return metadata
}

// analyzeRTFStructure analyzes the structure of RTF content
func (r *RTFReader) analyzeRTFStructure(content string) RTFStructure {
	structure := RTFStructure{}

	// Count paragraphs (\\par commands)
	structure.ParagraphCount = strings.Count(content, "\\par")
	if structure.ParagraphCount == 0 {
		structure.ParagraphCount = 1 // At least one paragraph
	}

	// Check for tables
	structure.HasTables = strings.Contains(content, "\\trowd") || strings.Contains(content, "\\cell")

	// Check for images
	structure.HasImages = strings.Contains(content, "\\pict") || strings.Contains(content, "\\shppict")

	// Check for hyperlinks
	structure.HasHyperlinks = strings.Contains(content, "\\field") && strings.Contains(content, "HYPERLINK")

	return structure
}

// extractPlainText extracts plain text from RTF content
func (r *RTFReader) extractPlainText(content string) string {
	// Make a working copy
	text := content

	// Remove the info group specifically
	if infoStart := strings.Index(text, "{\\info"); infoStart >= 0 {
		level := 1
		end := infoStart + 7 // After "{\\info"
		for end < len(text) && level > 0 {
			if text[end] == '{' {
				level++
			} else if text[end] == '}' {
				level--
			}
			end++
		}
		if level == 0 {
			text = text[:infoStart] + text[end:]
		}
	}

	// Remove font table
	if fontStart := strings.Index(text, "{\\fonttbl"); fontStart >= 0 {
		level := 1
		end := fontStart + 9
		for end < len(text) && level > 0 {
			if text[end] == '{' {
				level++
			} else if text[end] == '}' {
				level--
			}
			end++
		}
		if level == 0 {
			text = text[:fontStart] + text[end:]
		}
	}

	// Remove style sheet
	if styleStart := strings.Index(text, "{\\stylesheet"); styleStart >= 0 {
		level := 1
		end := styleStart + 12
		for end < len(text) && level > 0 {
			if text[end] == '{' {
				level++
			} else if text[end] == '}' {
				level--
			}
			end++
		}
		if level == 0 {
			text = text[:styleStart] + text[end:]
		}
	}

	// Convert special characters before removing control words
	text = strings.ReplaceAll(text, "\\'92", "'")
	text = strings.ReplaceAll(text, "\\'93", "\"")
	text = strings.ReplaceAll(text, "\\'94", "\"")
	text = strings.ReplaceAll(text, "\\'96", "-")
	text = strings.ReplaceAll(text, "\\'97", "--")

	// Replace line breaks
	text = strings.ReplaceAll(text, "\\par", "\n")
	text = strings.ReplaceAll(text, "\\line", "\n")

	// Remove control words (but preserve text after them)
	text = regexp.MustCompile(`\\[a-z]+[0-9]*\s?`).ReplaceAllString(text, "")

	// Remove remaining braces
	text = strings.ReplaceAll(text, "{", "")
	text = strings.ReplaceAll(text, "}", "")

	// Clean up whitespace
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	text = strings.Join(lines, "\n")

	// Remove multiple spaces
	text = regexp.MustCompile(`[ \t]+`).ReplaceAllString(text, " ")

	// Remove leading/trailing whitespace from the entire text
	text = strings.TrimSpace(text)

	return text
}

// decodeRTFString decodes RTF encoded strings
func (r *RTFReader) decodeRTFString(s string) string {
	// Simple RTF string decoding
	s = strings.TrimSpace(s)

	// Convert RTF escaped characters
	s = strings.ReplaceAll(s, "\\'92", "'")
	s = strings.ReplaceAll(s, "\\'93", "\"")
	s = strings.ReplaceAll(s, "\\'94", "\"")
	s = strings.ReplaceAll(s, "\\'96", "-")
	s = strings.ReplaceAll(s, "\\'97", "--")
	s = strings.ReplaceAll(s, "\\{", "{")
	s = strings.ReplaceAll(s, "\\}", "}")
	s = strings.ReplaceAll(s, "\\\\", "\\")

	return s
}

// extractSampleText extracts sample text from RTF
func (r *RTFReader) extractSampleText(content string) string {
	text := r.extractPlainText(content)
	if len(text) > 200 {
		return text[:200] + "..."
	}
	return text
}

// countParagraphs counts the number of paragraphs in RTF content
func (r *RTFReader) countParagraphs(content string) int {
	count := strings.Count(content, "\\par")
	if count == 0 {
		return 1
	}
	return count
}

// parseRTFParagraphs parses RTF into paragraphs
func (r *RTFReader) parseRTFParagraphs(content string, config map[string]any) []RTFParagraph {
	var paragraphs []RTFParagraph

	// Split by paragraph markers
	parts := strings.Split(content, "\\par")

	paragraphNum := 0
	for _, part := range parts {
		// Extract text from paragraph
		text := r.extractPlainText(part)
		if strings.TrimSpace(text) == "" {
			continue
		}

		paragraphNum++
		paragraph := RTFParagraph{
			Content: text,
			Number:  paragraphNum,
		}

		// Extract basic formatting if enabled
		if preserveFormatting, _ := config["preserve_formatting"].(bool); preserveFormatting {
			formatting := make(map[string]any)

			// Check for bold
			if strings.Contains(part, "\\b ") || strings.Contains(part, "\\b\\") {
				formatting["bold"] = true
			}

			// Check for italic
			if strings.Contains(part, "\\i ") || strings.Contains(part, "\\i\\") {
				formatting["italic"] = true
			}

			// Check for underline
			if strings.Contains(part, "\\ul ") || strings.Contains(part, "\\ul\\") {
				formatting["underline"] = true
			}

			paragraph.Formatting = formatting
		}

		paragraphs = append(paragraphs, paragraph)
	}

	// If no paragraphs found, treat entire content as one paragraph
	if len(paragraphs) == 0 {
		text := r.extractPlainText(content)
		if text != "" {
			paragraphs = append(paragraphs, RTFParagraph{
				Content: text,
				Number:  1,
			})
		}
	}

	return paragraphs
}

// RTFIterator implements ChunkIterator for RTF files
type RTFIterator struct {
	sourcePath       string
	config           map[string]any
	paragraphs       []RTFParagraph
	currentParagraph int
}

// Next returns the next chunk of content from the RTF
func (it *RTFIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	// Check if we've processed all paragraphs
	if it.currentParagraph >= len(it.paragraphs) {
		return core.Chunk{}, core.ErrIteratorExhausted
	}

	paragraph := it.paragraphs[it.currentParagraph]
	it.currentParagraph++

	chunk := core.Chunk{
		Data: paragraph.Content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:paragraph:%d", filepath.Base(it.sourcePath), paragraph.Number),
			ChunkType:   "rtf_paragraph",
			SizeBytes:   int64(len(paragraph.Content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "rtf_reader",
			Context: map[string]string{
				"paragraph_number": strconv.Itoa(paragraph.Number),
				"total_paragraphs": strconv.Itoa(len(it.paragraphs)),
				"file_type":        "rtf",
			},
		},
	}

	// Add style information if present
	if paragraph.Style != "" {
		chunk.Metadata.Context["style"] = paragraph.Style
	}

	// Add formatting information if enabled
	if len(paragraph.Formatting) > 0 {
		chunk.Metadata.Context["formatting"] = fmt.Sprintf("%v", paragraph.Formatting)
	}

	return chunk, nil
}

// Close releases RTF resources
func (it *RTFIterator) Close() error {
	// Nothing to close for RTF iterator
	return nil
}

// Reset restarts iteration from the beginning
func (it *RTFIterator) Reset() error {
	it.currentParagraph = 0
	return nil
}

// Progress returns iteration progress
func (it *RTFIterator) Progress() float64 {
	if len(it.paragraphs) == 0 {
		return 1.0
	}
	return float64(it.currentParagraph) / float64(len(it.paragraphs))
}

// Helper functions
func ptrFloat64(f float64) *float64 {
	return &f
}
