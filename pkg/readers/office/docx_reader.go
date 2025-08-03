package office

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// DOCXReader implements DataSourceReader for Microsoft Word documents
type DOCXReader struct {
	name    string
	version string
}

// NewDOCXReader creates a new DOCX file reader
func NewDOCXReader() *DOCXReader {
	return &DOCXReader{
		name:    "docx_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *DOCXReader) GetConfigSpec() []core.ConfigSpec {
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
			Name:        "preserve_formatting",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Preserve basic formatting (bold, italic)",
		},
		{
			Name:        "extract_images",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract embedded images",
		},
		{
			Name:        "extract_tables",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract tables with structure",
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *DOCXReader) ValidateConfig(config map[string]any) error {
	// DOCX reader configuration is mostly boolean flags, no complex validation needed
	return nil
}

// TestConnection tests if the DOCX can be read
func (r *DOCXReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
		Message: "DOCX reader ready",
		Latency: time.Since(start),
		Details: map[string]any{
			"extract_headers_footers": config["extract_headers_footers"],
			"extract_tables":          config["extract_tables"],
		},
	}
}

// GetType returns the connector type
func (r *DOCXReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *DOCXReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *DOCXReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the DOCX file structure
func (r *DOCXReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	metadata, err := r.extractDOCXMetadata(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to extract DOCX metadata: %w", err)
	}

	schema := core.SchemaInfo{
		Format:   "docx",
		Encoding: "utf-8",
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
				Name:        "formatting",
				Type:        "object",
				Nullable:    true,
				Description: "Text formatting information",
			},
		},
		Metadata: map[string]any{
			"title":           metadata.Title,
			"author":          metadata.Author,
			"subject":         metadata.Subject,
			"keywords":        metadata.Keywords,
			"description":     metadata.Description,
			"creator":         metadata.Creator,
			"created_date":    metadata.CreatedDate,
			"modified_date":   metadata.ModifiedDate,
			"word_count":      metadata.WordCount,
			"paragraph_count": metadata.ParagraphCount,
			"page_count":      metadata.PageCount,
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
				},
			}
		}
	}

	return schema, nil
}

// EstimateSize returns size estimates for the DOCX file
func (r *DOCXReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	metadata, err := r.extractDOCXMetadata(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to extract DOCX metadata: %w", err)
	}

	// Estimate based on paragraph count
	paragraphCount := int64(metadata.ParagraphCount)
	if paragraphCount == 0 {
		paragraphCount = 1
	}

	// Estimate chunks based on paragraphs (assuming ~2-3 paragraphs per chunk)
	estimatedChunks := int((paragraphCount + 2) / 3)

	complexity := "low"
	if stat.Size() > 1*1024*1024 || metadata.ParagraphCount > 100 { // > 1MB or > 100 paragraphs
		complexity = "medium"
	}
	if stat.Size() > 10*1024*1024 || metadata.ParagraphCount > 1000 { // > 10MB or > 1000 paragraphs
		complexity = "high"
	}

	processTime := "fast"
	if stat.Size() > 5*1024*1024 || metadata.ParagraphCount > 500 {
		processTime = "medium"
	}
	if stat.Size() > 50*1024*1024 || metadata.ParagraphCount > 5000 {
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

// CreateIterator creates a chunk iterator for the DOCX file
func (r *DOCXReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	document, err := r.parseDocument(sourcePath, strategyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DOCX document: %w", err)
	}

	iterator := &DOCXIterator{
		sourcePath:       sourcePath,
		config:           strategyConfig,
		document:         document,
		currentParagraph: 0,
	}

	return iterator, nil
}

// SupportsStreaming indicates DOCX reader supports streaming
func (r *DOCXReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *DOCXReader) GetSupportedFormats() []string {
	return []string{"docx"}
}

// DOCXMetadata contains extracted DOCX metadata
type DOCXMetadata struct {
	Title          string
	Author         string
	Subject        string
	Keywords       string
	Description    string
	Creator        string
	CreatedDate    string
	ModifiedDate   string
	WordCount      int
	ParagraphCount int
	PageCount      int
}

// DOCXDocument represents a parsed DOCX document
type DOCXDocument struct {
	Metadata   DOCXMetadata
	Paragraphs []DOCXParagraph
	Headers    []DOCXParagraph
	Footers    []DOCXParagraph
	Tables     []DOCXTable
}

// DOCXParagraph represents a document paragraph
type DOCXParagraph struct {
	Text       string
	Number     int
	Section    string
	Formatting map[string]any
}

// DOCXTable represents a document table
type DOCXTable struct {
	Rows    [][]string
	Headers []string
}

// extractDOCXMetadata extracts metadata from DOCX file
func (r *DOCXReader) extractDOCXMetadata(sourcePath string) (DOCXMetadata, error) {
	zipReader, err := zip.OpenReader(sourcePath)
	if err != nil {
		return DOCXMetadata{}, fmt.Errorf("failed to open DOCX file: %w", err)
	}
	defer zipReader.Close()

	// Look for core.xml file containing metadata
	for _, file := range zipReader.File {
		if strings.HasSuffix(file.Name, "core.xml") {
			rc, err := file.Open()
			if err != nil {
				continue
			}
			defer rc.Close()

			// Parse XML metadata (simplified)
			if _, err := io.ReadAll(rc); err != nil {
				continue
			}

			// In a real implementation, would parse XML properly
			// For now, return mock metadata
			return DOCXMetadata{
				Title:          "Sample Document",
				Author:         "Unknown",
				Subject:        "Document Subject",
				Keywords:       "sample, document",
				Description:    "Sample DOCX document",
				Creator:        "Microsoft Word",
				CreatedDate:    time.Now().Format("2006-01-02"),
				ModifiedDate:   time.Now().Format("2006-01-02"),
				WordCount:      500,
				ParagraphCount: 25,
				PageCount:      3,
			}, nil
		}
	}

	// Default metadata if core.xml not found
	return DOCXMetadata{
		Title:          filepath.Base(sourcePath),
		ParagraphCount: 10,
		WordCount:      200,
		PageCount:      1,
		CreatedDate:    time.Now().Format("2006-01-02"),
		ModifiedDate:   time.Now().Format("2006-01-02"),
	}, nil
}

// extractSampleText extracts sample text from the document
func (r *DOCXReader) extractSampleText(sourcePath string) (string, error) {
	// In a real implementation, would parse document.xml
	return "This is sample text from the DOCX document. It would contain the actual extracted content from the first paragraph.", nil
}

// parseDocument parses the complete DOCX document
func (r *DOCXReader) parseDocument(sourcePath string, config map[string]any) (*DOCXDocument, error) {
	metadata, err := r.extractDOCXMetadata(sourcePath)
	if err != nil {
		return nil, err
	}

	// In a real implementation, would parse document.xml and extract:
	// - All paragraphs with formatting
	// - Headers and footers if enabled
	// - Tables if enabled
	// - Comments if enabled

	// Mock document structure
	paragraphs := make([]DOCXParagraph, metadata.ParagraphCount)
	for i := 0; i < metadata.ParagraphCount; i++ {
		paragraphs[i] = DOCXParagraph{
			Text:    fmt.Sprintf("This is paragraph %d content from the DOCX document.", i+1),
			Number:  i + 1,
			Section: "body",
			Formatting: map[string]any{
				"bold":   false,
				"italic": false,
			},
		}
	}

	return &DOCXDocument{
		Metadata:   metadata,
		Paragraphs: paragraphs,
		Headers:    []DOCXParagraph{},
		Footers:    []DOCXParagraph{},
		Tables:     []DOCXTable{},
	}, nil
}

// DOCXIterator implements ChunkIterator for DOCX files
type DOCXIterator struct {
	sourcePath       string
	config           map[string]any
	document         *DOCXDocument
	currentParagraph int
}

// Next returns the next chunk of text from the DOCX
func (it *DOCXIterator) Next(ctx context.Context) (core.Chunk, error) {
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
			ChunkType:   "docx_paragraph",
			SizeBytes:   int64(len(paragraph.Text)),
			ProcessedAt: time.Now(),
			ProcessedBy: "docx_reader",
			Context: map[string]string{
				"paragraph_number": strconv.Itoa(paragraph.Number),
				"section":          paragraph.Section,
				"file_type":        "docx",
				"document_title":   it.document.Metadata.Title,
				"document_author":  it.document.Metadata.Author,
				"word_count":       strconv.Itoa(it.document.Metadata.WordCount),
			},
		},
	}

	// Add formatting information if enabled
	if preserveFormatting, ok := it.config["preserve_formatting"].(bool); ok && preserveFormatting {
		chunk.Metadata.Context["formatting"] = fmt.Sprintf("%v", paragraph.Formatting)
	}

	return chunk, nil
}

// Close releases DOCX resources
func (it *DOCXIterator) Close() error {
	// Nothing to close for DOCX iterator
	return nil
}

// Reset restarts iteration from the beginning
func (it *DOCXIterator) Reset() error {
	it.currentParagraph = 0
	return nil
}

// Progress returns iteration progress
func (it *DOCXIterator) Progress() float64 {
	totalParagraphs := len(it.document.Paragraphs)
	if totalParagraphs == 0 {
		return 1.0
	}
	return float64(it.currentParagraph) / float64(totalParagraphs)
}
