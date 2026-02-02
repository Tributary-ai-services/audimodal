package office

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/jscharber/audimodal/pkg/core"
)

// DOCReader implements DataSourceReader for legacy Microsoft Word documents (.doc)
type DOCReader struct {
	name    string
	version string
}

// NewDOCReader creates a new DOC file reader
func NewDOCReader() *DOCReader {
	return &DOCReader{
		name:    "doc_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *DOCReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "extract_headers_footers",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract headers and footers",
		},
		{
			Name:        "extract_footnotes",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract footnotes and endnotes",
		},
		{
			Name:        "extract_comments",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract document comments",
		},
		{
			Name:        "extract_metadata",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract document metadata",
		},
		{
			Name:        "extract_tables",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract tables with structure",
		},
		{
			Name:        "extract_embedded_objects",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract embedded objects and images",
		},
		{
			Name:        "handle_password_protected",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Attempt to handle password-protected documents",
		},
		{
			Name:        "fallback_to_text_extraction",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Fallback to basic text extraction if structured parsing fails",
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *DOCReader) ValidateConfig(config map[string]any) error {
	// DOC reader configuration is mostly boolean flags, no complex validation needed
	return nil
}

// TestConnection tests if the DOC can be read
func (r *DOCReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
		Message: "DOC reader ready",
		Latency: time.Since(start),
		Details: map[string]any{
			"extract_headers_footers": config["extract_headers_footers"],
			"extract_tables":          config["extract_tables"],
		},
	}
}

// GetType returns the connector type
func (r *DOCReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *DOCReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *DOCReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the DOC file structure
func (r *DOCReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	if !r.isDOCFile(sourcePath) {
		return core.SchemaInfo{}, fmt.Errorf("not a valid DOC file")
	}

	metadata, err := r.extractDOCMetadata(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to extract DOC metadata: %w", err)
	}

	schema := core.SchemaInfo{
		Format:   "doc",
		Encoding: "binary",
		Fields: []core.FieldInfo{
			{
				Name:        "content",
				Type:        "text",
				Nullable:    false,
				Description: "Document text content",
			},
			{
				Name:        "paragraph_number",
				Type:        "integer",
				Nullable:    false,
				Description: "Paragraph number in document",
			},
			{
				Name:        "section",
				Type:        "string",
				Nullable:    true,
				Description: "Document section (body, header, footer)",
			},
			{
				Name:        "style",
				Type:        "string",
				Nullable:    true,
				Description: "Paragraph or text style",
			},
			{
				Name:        "table_info",
				Type:        "object",
				Nullable:    true,
				Description: "Table structure information if applicable",
			},
		},
		Metadata: map[string]any{
			"title":           metadata.Title,
			"author":          metadata.Author,
			"subject":         metadata.Subject,
			"keywords":        metadata.Keywords,
			"comments":        metadata.Comments,
			"created_date":    metadata.CreatedDate,
			"modified_date":   metadata.ModifiedDate,
			"word_count":      metadata.WordCount,
			"paragraph_count": metadata.ParagraphCount,
			"page_count":      metadata.PageCount,
			"doc_version":     metadata.DOCVersion,
			"has_tables":      metadata.HasTables,
			"has_images":      metadata.HasImages,
			"is_encrypted":    metadata.IsEncrypted,
		},
	}

	// Sample first paragraph
	if metadata.ParagraphCount > 0 {
		sampleText, err := r.extractSampleText(sourcePath)
		if err == nil {
			schema.SampleData = []map[string]any{
				{
					"content":          sampleText,
					"paragraph_number": 1,
					"section":          "body",
					"style":            "Normal",
				},
			}
		}
	}

	return schema, nil
}

// EstimateSize returns size estimates for the DOC file
func (r *DOCReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	metadata, err := r.extractDOCMetadata(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to extract DOC metadata: %w", err)
	}

	// Estimate based on paragraph count
	paragraphCount := int64(metadata.ParagraphCount)
	if paragraphCount == 0 {
		paragraphCount = 1
	}

	// Estimate chunks based on paragraphs (assuming ~2-3 paragraphs per chunk)
	estimatedChunks := int((paragraphCount + 2) / 3)

	// DOC files are more complex than DOCX due to binary format
	complexity := "medium"
	if stat.Size() > 1*1024*1024 || metadata.ParagraphCount > 50 { // > 1MB or > 50 paragraphs
		complexity = "high"
	}
	if stat.Size() > 10*1024*1024 || metadata.ParagraphCount > 500 { // > 10MB or > 500 paragraphs
		complexity = "very_high"
	}

	// DOC processing is typically slower due to binary format complexity
	processTime := "medium"
	if stat.Size() > 2*1024*1024 || metadata.ParagraphCount > 100 {
		processTime = "slow"
	}
	if stat.Size() > 20*1024*1024 || metadata.ParagraphCount > 1000 {
		processTime = "very_slow"
	}

	return core.SizeEstimate{
		RowCount:    &paragraphCount,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the DOC file
func (r *DOCReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	document, err := r.parseDocument(sourcePath, strategyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DOC document: %w", err)
	}

	iterator := &DOCIterator{
		sourcePath:       sourcePath,
		config:           strategyConfig,
		document:         document,
		currentParagraph: 0,
	}

	return iterator, nil
}

// SupportsStreaming indicates DOC reader supports streaming
func (r *DOCReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *DOCReader) GetSupportedFormats() []string {
	return []string{"doc"}
}

// DOCMetadata contains extracted DOC metadata
type DOCMetadata struct {
	Title          string
	Author         string
	Subject        string
	Keywords       string
	Comments       string
	CreatedDate    string
	ModifiedDate   string
	WordCount      int
	ParagraphCount int
	PageCount      int
	DOCVersion     string
	HasTables      bool
	HasImages      bool
	IsEncrypted    bool
}

// DOCDocument represents a parsed DOC document
type DOCDocument struct {
	Metadata   DOCMetadata
	Paragraphs []DOCParagraph
	Headers    []DOCParagraph
	Footers    []DOCParagraph
	Tables     []DOCTable
	Comments   []DOCComment
}

// DOCParagraph represents a document paragraph
type DOCParagraph struct {
	Text       string
	Number     int
	Section    string
	Style      string
	Properties map[string]any
}

// DOCTable represents a document table
type DOCTable struct {
	Rows    [][]string
	Headers []string
}

// DOCComment represents a document comment
type DOCComment struct {
	Text      string
	Author    string
	Date      string
	Reference string
}

// isDOCFile checks if the file is a valid DOC file by checking OLE signature
func (r *DOCReader) isDOCFile(filePath string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	// Check OLE signature: D0CF11E0A1B11AE1
	signature := make([]byte, 8)
	n, err := file.Read(signature)
	if err != nil || n != 8 {
		return false
	}

	expectedSignature := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	if !bytes.Equal(signature, expectedSignature) {
		return false
	}

	// Additional check for Word document specific structures
	// This is a simplified check - in production, you'd parse the OLE structure
	return true
}

// extractDOCMetadata extracts metadata from DOC file
func (r *DOCReader) extractDOCMetadata(sourcePath string) (DOCMetadata, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		return DOCMetadata{}, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return DOCMetadata{}, err
	}

	// This is a simplified metadata extraction
	// In production, you would parse the OLE compound document structure
	// and extract metadata from the DocumentSummaryInformation and SummaryInformation streams

	metadata := DOCMetadata{
		CreatedDate:  stat.ModTime().Format("2006-01-02 15:04:05"),
		ModifiedDate: stat.ModTime().Format("2006-01-02 15:04:05"),
		DOCVersion:   "Word 95-2003",
	}

	// Try to extract basic information by analyzing file structure
	// This is a simplified approach - production code would use proper OLE parsing
	buffer := make([]byte, 2048)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return metadata, err
	}

	// Look for common Word document indicators and extract basic info
	content := string(buffer[:n])

	// Estimate document size based on file size
	sizeCategory := stat.Size() / (1024) // Size in KB

	switch {
	case sizeCategory < 50: // < 50KB
		metadata.WordCount = 200
		metadata.ParagraphCount = 10
		metadata.PageCount = 1
	case sizeCategory < 200: // 50-200KB
		metadata.WordCount = 1000
		metadata.ParagraphCount = 50
		metadata.PageCount = 3
	case sizeCategory < 1000: // 200KB-1MB
		metadata.WordCount = 5000
		metadata.ParagraphCount = 200
		metadata.PageCount = 15
	default: // > 1MB
		metadata.WordCount = 10000
		metadata.ParagraphCount = 500
		metadata.PageCount = 30
	}

	// Check for tables (simplified heuristic)
	if strings.Contains(content, "\x07") { // Table cell marker in DOC format
		metadata.HasTables = true
	}

	// Check for images (simplified heuristic)
	if bytes.Contains(buffer, []byte{0xFF, 0xD8, 0xFF}) { // JPEG signature
		metadata.HasImages = true
	}

	// Set default values
	metadata.Title = filepath.Base(sourcePath)
	metadata.Author = "Unknown"
	metadata.Subject = ""
	metadata.Keywords = ""
	metadata.Comments = ""

	return metadata, nil
}

// extractSampleText extracts sample text from the document
func (r *DOCReader) extractSampleText(sourcePath string) (string, error) {
	text, err := r.extractAllText(sourcePath)
	if err != nil {
		return "", err
	}
	// Return first 500 characters as sample
	if len(text) > 500 {
		return text[:500] + "...", nil
	}
	return text, nil
}

// extractAllText extracts all text content from a DOC file
func (r *DOCReader) extractAllText(sourcePath string) (string, error) {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", err
	}

	// Try multiple extraction methods and combine results
	var allText strings.Builder

	// Method 1: Extract Unicode (UTF-16LE) text
	unicodeText := r.extractUnicodeText(data)
	if unicodeText != "" {
		allText.WriteString(unicodeText)
	}

	// Method 2: Extract ASCII text if Unicode didn't yield much
	if allText.Len() < 100 {
		asciiText := r.extractASCIIText(data)
		if asciiText != "" {
			if allText.Len() > 0 {
				allText.WriteString("\n")
			}
			allText.WriteString(asciiText)
		}
	}

	// Method 3: Look for Word-specific text streams
	wordText := r.extractWordDocumentText(data)
	if wordText != "" && len(wordText) > allText.Len() {
		// Use Word-specific extraction if it found more text
		return wordText, nil
	}

	result := allText.String()
	if result == "" {
		return "", fmt.Errorf("no text content found in DOC file")
	}

	return r.cleanExtractedText(result), nil
}

// extractUnicodeText extracts UTF-16LE encoded text from binary data
func (r *DOCReader) extractUnicodeText(data []byte) string {
	var result strings.Builder

	// Look for sequences of UTF-16LE encoded text
	// UTF-16LE has low byte first, high byte second (often 0x00 for ASCII range)
	i := 0
	for i < len(data)-1 {
		// Check for potential UTF-16LE sequence (printable ASCII range in UTF-16LE)
		if data[i] >= 0x20 && data[i] <= 0x7E && data[i+1] == 0x00 {
			// Found potential UTF-16LE text, collect the sequence
			start := i
			for i < len(data)-1 && data[i] >= 0x20 && data[i] <= 0x7E && data[i+1] == 0x00 {
				i += 2
			}
			// Also include common control characters (newline, tab)
			for i < len(data)-1 && ((data[i] >= 0x20 && data[i] <= 0x7E) || data[i] == 0x0D || data[i] == 0x0A || data[i] == 0x09) && data[i+1] == 0x00 {
				i += 2
			}

			// Extract the UTF-16LE text if it's long enough
			if i-start >= 6 { // At least 3 characters
				utf16Bytes := data[start:i]
				text := r.decodeUTF16LE(utf16Bytes)
				if len(text) >= 3 {
					if result.Len() > 0 {
						result.WriteString(" ")
					}
					result.WriteString(text)
				}
			}
		} else {
			i++
		}
	}

	return result.String()
}

// decodeUTF16LE decodes UTF-16LE bytes to a string
func (r *DOCReader) decodeUTF16LE(data []byte) string {
	if len(data)%2 != 0 {
		data = data[:len(data)-1]
	}

	u16s := make([]uint16, len(data)/2)
	for i := 0; i < len(u16s); i++ {
		u16s[i] = binary.LittleEndian.Uint16(data[i*2:])
	}

	return string(utf16.Decode(u16s))
}

// extractASCIIText extracts ASCII text from binary data
func (r *DOCReader) extractASCIIText(data []byte) string {
	var result strings.Builder
	var currentWord strings.Builder

	for _, b := range data {
		// Check if it's a printable ASCII character or common whitespace
		if (b >= 0x20 && b <= 0x7E) || b == 0x0A || b == 0x0D || b == 0x09 {
			currentWord.WriteByte(b)
		} else {
			// End of text sequence
			word := currentWord.String()
			if len(word) >= 4 { // Only keep sequences of 4+ characters
				if result.Len() > 0 && !strings.HasSuffix(result.String(), " ") && !strings.HasSuffix(result.String(), "\n") {
					result.WriteString(" ")
				}
				result.WriteString(word)
			}
			currentWord.Reset()
		}
	}

	// Don't forget the last word
	word := currentWord.String()
	if len(word) >= 4 {
		if result.Len() > 0 {
			result.WriteString(" ")
		}
		result.WriteString(word)
	}

	return result.String()
}

// extractWordDocumentText attempts to extract text from the Word document structure
func (r *DOCReader) extractWordDocumentText(data []byte) string {
	// DOC files store text in specific locations
	// The main text typically starts after the FIB (File Information Block)
	// and is stored in the WordDocument stream

	// Look for text start markers
	var textStart int
	var textEnd int

	// Find potential text regions by looking for high density of printable chars
	windowSize := 512
	bestStart := 0
	bestDensity := 0.0

	for i := 512; i < len(data)-windowSize; i += 256 {
		density := r.calculatePrintableDensity(data[i : i+windowSize])
		if density > bestDensity && density > 0.7 {
			bestDensity = density
			bestStart = i
		}
	}

	if bestDensity > 0.7 {
		textStart = bestStart
		// Find where the text ends
		for i := textStart; i < len(data)-windowSize; i += 256 {
			density := r.calculatePrintableDensity(data[i : i+windowSize])
			if density < 0.3 {
				textEnd = i
				break
			}
		}
		if textEnd == 0 {
			textEnd = len(data)
		}

		// Extract text from the identified region
		if textEnd > textStart {
			return r.extractUnicodeText(data[textStart:textEnd])
		}
	}

	return ""
}

// calculatePrintableDensity calculates the ratio of printable characters
func (r *DOCReader) calculatePrintableDensity(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}
	printable := 0
	for _, b := range data {
		if (b >= 0x20 && b <= 0x7E) || b == 0x0A || b == 0x0D || b == 0x09 || b == 0x00 {
			printable++
		}
	}
	return float64(printable) / float64(len(data))
}

// cleanExtractedText cleans up extracted text
func (r *DOCReader) cleanExtractedText(text string) string {
	// Remove excessive whitespace
	spacePattern := regexp.MustCompile(`\s+`)
	text = spacePattern.ReplaceAllString(text, " ")

	// Remove common DOC artifacts
	text = strings.ReplaceAll(text, "\x00", "")
	text = strings.ReplaceAll(text, "\x07", " ") // Table cell marker
	text = strings.ReplaceAll(text, "\x01", "")
	text = strings.ReplaceAll(text, "\x13", "") // Field begin
	text = strings.ReplaceAll(text, "\x14", "") // Field separator
	text = strings.ReplaceAll(text, "\x15", "") // Field end

	// Trim whitespace
	text = strings.TrimSpace(text)

	return text
}

// splitIntoParagraphs splits text into paragraphs
func (r *DOCReader) splitIntoParagraphs(text string) []string {
	// Split on common paragraph separators
	// DOC files use various markers: \r\n, \r, \n, or multiple spaces

	// First, normalize line endings
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	// Split on double newlines or single newlines
	var paragraphs []string
	parts := strings.Split(text, "\n")

	var currentPara strings.Builder
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			// Empty line - end of paragraph
			if currentPara.Len() > 0 {
				paragraphs = append(paragraphs, currentPara.String())
				currentPara.Reset()
			}
		} else {
			if currentPara.Len() > 0 {
				currentPara.WriteString(" ")
			}
			currentPara.WriteString(part)
		}
	}

	// Don't forget the last paragraph
	if currentPara.Len() > 0 {
		paragraphs = append(paragraphs, currentPara.String())
	}

	// If no paragraph breaks found, split by sentences or fixed length
	if len(paragraphs) <= 1 && len(text) > 500 {
		paragraphs = r.splitByLength(text, 500)
	}

	return paragraphs
}

// splitByLength splits text into chunks of approximately the given length
func (r *DOCReader) splitByLength(text string, maxLen int) []string {
	var result []string

	words := strings.Fields(text)
	var current strings.Builder

	for _, word := range words {
		if current.Len()+len(word)+1 > maxLen && current.Len() > 0 {
			result = append(result, current.String())
			current.Reset()
		}
		if current.Len() > 0 {
			current.WriteString(" ")
		}
		current.WriteString(word)
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// parseDocument parses the complete DOC document
func (r *DOCReader) parseDocument(sourcePath string, config map[string]any) (*DOCDocument, error) {
	metadata, err := r.extractDOCMetadata(sourcePath)
	if err != nil {
		return nil, err
	}

	// Extract actual text content
	text, err := r.extractAllText(sourcePath)
	if err != nil {
		// Fallback to empty document if text extraction fails
		text = ""
	}

	// Split into paragraphs
	paragraphTexts := r.splitIntoParagraphs(text)

	// Create paragraph structures
	paragraphs := make([]DOCParagraph, len(paragraphTexts))
	for i, pText := range paragraphTexts {
		paragraphs[i] = DOCParagraph{
			Text:    pText,
			Number:  i + 1,
			Section: "body",
			Style:   "Normal",
			Properties: map[string]any{
				"font_name": "Times New Roman",
				"font_size": 12,
			},
		}
	}

	// Update metadata with actual paragraph count
	metadata.ParagraphCount = len(paragraphs)

	// Count words
	wordCount := 0
	for _, p := range paragraphs {
		wordCount += len(strings.Fields(p.Text))
	}
	metadata.WordCount = wordCount

	return &DOCDocument{
		Metadata:   metadata,
		Paragraphs: paragraphs,
		Headers:    []DOCParagraph{},
		Footers:    []DOCParagraph{},
		Tables:     []DOCTable{},
		Comments:   []DOCComment{},
	}, nil
}

// DOCIterator implements ChunkIterator for DOC files
type DOCIterator struct {
	sourcePath       string
	config           map[string]any
	document         *DOCDocument
	currentParagraph int
}

// Next returns the next chunk of text from the DOC
func (it *DOCIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	// Check if we've processed all paragraphs
	if it.currentParagraph >= len(it.document.Paragraphs) {
		return core.Chunk{}, core.ErrIteratorExhausted
	}

	paragraph := it.document.Paragraphs[it.currentParagraph]
	it.currentParagraph++

	chunk := core.Chunk{
		Data: paragraph.Text,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:paragraph:%d", filepath.Base(it.sourcePath), paragraph.Number),
			ChunkType:   "doc_paragraph",
			SizeBytes:   int64(len(paragraph.Text)),
			ProcessedAt: time.Now(),
			ProcessedBy: "doc_reader",
			Context: map[string]string{
				"paragraph_number": strconv.Itoa(paragraph.Number),
				"section":          paragraph.Section,
				"style":            paragraph.Style,
				"file_type":        "doc",
				"document_title":   it.document.Metadata.Title,
				"document_author":  it.document.Metadata.Author,
				"word_count":       strconv.Itoa(it.document.Metadata.WordCount),
			},
		},
	}

	// Add style and formatting information
	if paragraph.Properties != nil {
		for key, value := range paragraph.Properties {
			chunk.Metadata.Context["prop_"+key] = fmt.Sprintf("%v", value)
		}
	}

	return chunk, nil
}

// Close releases DOC resources
func (it *DOCIterator) Close() error {
	// Nothing to close for DOC iterator
	return nil
}

// Reset restarts iteration from the beginning
func (it *DOCIterator) Reset() error {
	it.currentParagraph = 0
	return nil
}

// Progress returns iteration progress
func (it *DOCIterator) Progress() float64 {
	totalParagraphs := len(it.document.Paragraphs)
	if totalParagraphs == 0 {
		return 1.0
	}
	return float64(it.currentParagraph) / float64(totalParagraphs)
}

// Helper function to read a little-endian uint32
func readUint32LE(data []byte, offset int) uint32 {
	if offset+4 > len(data) {
		return 0
	}
	return binary.LittleEndian.Uint32(data[offset : offset+4])
}

// Helper function to read a little-endian uint16
func readUint16LE(data []byte, offset int) uint16 {
	if offset+2 > len(data) {
		return 0
	}
	return binary.LittleEndian.Uint16(data[offset : offset+2])
}
