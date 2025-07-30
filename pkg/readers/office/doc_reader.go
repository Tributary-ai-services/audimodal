package office

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
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
		config:          strategyConfig,
		document:        document,
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
	Text      string
	Number    int
	Section   string
	Style     string
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
	// In a real implementation, you would parse the Word document binary format
	// and extract the actual text content from the WordDocument stream
	return "This is sample text from the DOC document. It would contain the actual extracted content from the first paragraph.", nil
}

// parseDocument parses the complete DOC document
func (r *DOCReader) parseDocument(sourcePath string, config map[string]any) (*DOCDocument, error) {
	metadata, err := r.extractDOCMetadata(sourcePath)
	if err != nil {
		return nil, err
	}

	// In a real implementation, you would:
	// 1. Parse the OLE compound document structure
	// 2. Extract the WordDocument stream
	// 3. Parse the FIB (File Information Block)
	// 4. Extract text from the document stream
	// 5. Parse character and paragraph formatting
	// 6. Extract tables, headers, footers, etc.

	// Mock document structure for demonstration
	paragraphs := make([]DOCParagraph, metadata.ParagraphCount)
	for i := 0; i < metadata.ParagraphCount; i++ {
		paragraphs[i] = DOCParagraph{
			Text:    fmt.Sprintf("This is paragraph %d content from the DOC document.", i+1),
			Number:  i + 1,
			Section: "body",
			Style:   "Normal",
			Properties: map[string]any{
				"font_name":  "Times New Roman",
				"font_size":  12,
				"is_bold":    false,
				"is_italic":  false,
			},
		}
	}

	// Mock tables if present
	var tables []DOCTable
	if metadata.HasTables {
		tables = append(tables, DOCTable{
			Headers: []string{"Column 1", "Column 2", "Column 3"},
			Rows: [][]string{
				{"Row 1 Cell 1", "Row 1 Cell 2", "Row 1 Cell 3"},
				{"Row 2 Cell 1", "Row 2 Cell 2", "Row 2 Cell 3"},
			},
		})
	}

	return &DOCDocument{
		Metadata:   metadata,
		Paragraphs: paragraphs,
		Headers:    []DOCParagraph{},
		Footers:    []DOCParagraph{},
		Tables:     tables,
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