package office

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/audimodal/pkg/core"
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

// XML parsing structures for DOCX OOXML format

// docxCoreProperties represents docProps/core.xml
type docxCoreProperties struct {
	XMLName      xml.Name `xml:"coreProperties"`
	Title        string   `xml:"title"`
	Subject      string   `xml:"subject"`
	Creator      string   `xml:"creator"`
	Keywords     string   `xml:"keywords"`
	Description  string   `xml:"description"`
	LastModified string   `xml:"lastModifiedBy"`
	Created      string   `xml:"created"`
	Modified     string   `xml:"modified"`
}

// docxAppProperties represents docProps/app.xml
type docxAppProperties struct {
	XMLName    xml.Name `xml:"Properties"`
	Pages      int      `xml:"Pages"`
	Words      int      `xml:"Words"`
	Characters int      `xml:"Characters"`
	Paragraphs int      `xml:"Paragraphs"`
	Company    string   `xml:"Company"`
	Application string  `xml:"Application"`
}

// extractDOCXMetadata extracts metadata from DOCX file
func (r *DOCXReader) extractDOCXMetadata(sourcePath string) (DOCXMetadata, error) {
	zipReader, err := zip.OpenReader(sourcePath)
	if err != nil {
		return DOCXMetadata{}, fmt.Errorf("failed to open DOCX file: %w", err)
	}
	defer zipReader.Close()

	metadata := DOCXMetadata{
		Title:        filepath.Base(sourcePath),
		CreatedDate:  time.Now().Format("2006-01-02"),
		ModifiedDate: time.Now().Format("2006-01-02"),
	}

	// Build file map for quick lookup
	fileMap := make(map[string]*zip.File)
	for _, file := range zipReader.File {
		fileMap[file.Name] = file
	}

	// Parse core.xml for basic metadata
	if coreFile, ok := fileMap["docProps/core.xml"]; ok {
		if data, err := r.readZipFileContent(coreFile); err == nil {
			var props docxCoreProperties
			if err := xml.Unmarshal(data, &props); err == nil {
				if props.Title != "" {
					metadata.Title = props.Title
				}
				if props.Creator != "" {
					metadata.Author = props.Creator
					metadata.Creator = props.Creator
				}
				if props.LastModified != "" {
					metadata.Author = props.LastModified
				}
				if props.Subject != "" {
					metadata.Subject = props.Subject
				}
				if props.Keywords != "" {
					metadata.Keywords = props.Keywords
				}
				if props.Description != "" {
					metadata.Description = props.Description
				}
				if props.Created != "" {
					metadata.CreatedDate = props.Created
				}
				if props.Modified != "" {
					metadata.ModifiedDate = props.Modified
				}
			}
		}
	}

	// Parse app.xml for document statistics
	if appFile, ok := fileMap["docProps/app.xml"]; ok {
		if data, err := r.readZipFileContent(appFile); err == nil {
			var props docxAppProperties
			if err := xml.Unmarshal(data, &props); err == nil {
				if props.Pages > 0 {
					metadata.PageCount = props.Pages
				}
				if props.Words > 0 {
					metadata.WordCount = props.Words
				}
				if props.Paragraphs > 0 {
					metadata.ParagraphCount = props.Paragraphs
				}
			}
		}
	}

	// If paragraph count is still 0, count from document.xml
	if metadata.ParagraphCount == 0 {
		if docFile, ok := fileMap["word/document.xml"]; ok {
			if data, err := r.readZipFileContent(docFile); err == nil {
				// Count <w:p> tags (paragraphs)
				paragraphPattern := regexp.MustCompile(`<w:p[>\s]`)
				matches := paragraphPattern.FindAll(data, -1)
				metadata.ParagraphCount = len(matches)
			}
		}
	}

	// Ensure at least 1 paragraph
	if metadata.ParagraphCount == 0 {
		metadata.ParagraphCount = 1
	}

	return metadata, nil
}

// readZipFileContent reads the content of a zip file entry
func (r *DOCXReader) readZipFileContent(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// extractSampleText extracts sample text from the document
func (r *DOCXReader) extractSampleText(sourcePath string) (string, error) {
	zipReader, err := zip.OpenReader(sourcePath)
	if err != nil {
		return "", err
	}
	defer zipReader.Close()

	// Look for document.xml
	for _, file := range zipReader.File {
		if file.Name == "word/document.xml" {
			data, err := r.readZipFileContent(file)
			if err != nil {
				return "", err
			}
			text := r.extractTextFromDocumentXML(data)
			// Return first 500 characters as sample
			if len(text) > 500 {
				return text[:500] + "...", nil
			}
			return text, nil
		}
	}
	return "", fmt.Errorf("document.xml not found")
}

// extractTextFromDocumentXML extracts all text from document.xml
func (r *DOCXReader) extractTextFromDocumentXML(data []byte) string {
	// Extract text from <w:t> tags (Word text elements)
	textPattern := regexp.MustCompile(`<w:t[^>]*>([^<]*)</w:t>`)
	matches := textPattern.FindAllSubmatch(data, -1)

	var texts []string
	for _, match := range matches {
		if len(match) > 1 {
			text := string(match[1])
			if text != "" {
				texts = append(texts, text)
			}
		}
	}

	return strings.Join(texts, " ")
}

// extractParagraphsFromDocumentXML extracts paragraphs with their text
func (r *DOCXReader) extractParagraphsFromDocumentXML(data []byte) []DOCXParagraph {
	var paragraphs []DOCXParagraph

	// Find all <w:p> paragraph elements
	// Using a simpler approach: split by paragraph tags and extract text from each
	paragraphPattern := regexp.MustCompile(`(?s)<w:p[^>]*>(.*?)</w:p>`)
	paragraphMatches := paragraphPattern.FindAllSubmatch(data, -1)

	paragraphNum := 0
	for _, match := range paragraphMatches {
		if len(match) < 2 {
			continue
		}

		paragraphContent := match[1]

		// Extract text from this paragraph
		textPattern := regexp.MustCompile(`<w:t[^>]*>([^<]*)</w:t>`)
		textMatches := textPattern.FindAllSubmatch(paragraphContent, -1)

		var textParts []string
		for _, textMatch := range textMatches {
			if len(textMatch) > 1 {
				text := string(textMatch[1])
				if text != "" {
					textParts = append(textParts, text)
				}
			}
		}

		// Only add non-empty paragraphs
		paragraphText := strings.Join(textParts, "")
		if strings.TrimSpace(paragraphText) != "" {
			paragraphNum++

			// Check for formatting
			formatting := make(map[string]any)
			if strings.Contains(string(paragraphContent), "<w:b/>") || strings.Contains(string(paragraphContent), "<w:b ") {
				formatting["bold"] = true
			}
			if strings.Contains(string(paragraphContent), "<w:i/>") || strings.Contains(string(paragraphContent), "<w:i ") {
				formatting["italic"] = true
			}
			if strings.Contains(string(paragraphContent), "<w:u ") {
				formatting["underline"] = true
			}

			paragraphs = append(paragraphs, DOCXParagraph{
				Text:       paragraphText,
				Number:     paragraphNum,
				Section:    "body",
				Formatting: formatting,
			})
		}
	}

	return paragraphs
}

// extractTablesFromDocumentXML extracts tables from document.xml
func (r *DOCXReader) extractTablesFromDocumentXML(data []byte) []DOCXTable {
	var tables []DOCXTable

	// Find all <w:tbl> table elements
	tablePattern := regexp.MustCompile(`(?s)<w:tbl[^>]*>(.*?)</w:tbl>`)
	tableMatches := tablePattern.FindAllSubmatch(data, -1)

	for _, tableMatch := range tableMatches {
		if len(tableMatch) < 2 {
			continue
		}

		tableContent := tableMatch[1]

		// Find all rows <w:tr>
		rowPattern := regexp.MustCompile(`(?s)<w:tr[^>]*>(.*?)</w:tr>`)
		rowMatches := rowPattern.FindAllSubmatch(tableContent, -1)

		var rows [][]string
		for _, rowMatch := range rowMatches {
			if len(rowMatch) < 2 {
				continue
			}

			rowContent := rowMatch[1]

			// Find all cells <w:tc>
			cellPattern := regexp.MustCompile(`(?s)<w:tc[^>]*>(.*?)</w:tc>`)
			cellMatches := cellPattern.FindAllSubmatch(rowContent, -1)

			var cells []string
			for _, cellMatch := range cellMatches {
				if len(cellMatch) < 2 {
					continue
				}

				// Extract text from cell
				cellContent := cellMatch[1]
				textPattern := regexp.MustCompile(`<w:t[^>]*>([^<]*)</w:t>`)
				textMatches := textPattern.FindAllSubmatch(cellContent, -1)

				var cellTexts []string
				for _, textMatch := range textMatches {
					if len(textMatch) > 1 {
						cellTexts = append(cellTexts, string(textMatch[1]))
					}
				}
				cells = append(cells, strings.Join(cellTexts, " "))
			}

			if len(cells) > 0 {
				rows = append(rows, cells)
			}
		}

		if len(rows) > 0 {
			table := DOCXTable{
				Rows: rows,
			}
			// First row as headers if we have multiple rows
			if len(rows) > 1 {
				table.Headers = rows[0]
			}
			tables = append(tables, table)
		}
	}

	return tables
}

// parseDocument parses the complete DOCX document
func (r *DOCXReader) parseDocument(sourcePath string, config map[string]any) (*DOCXDocument, error) {
	metadata, err := r.extractDOCXMetadata(sourcePath)
	if err != nil {
		return nil, err
	}

	zipReader, err := zip.OpenReader(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open DOCX file: %w", err)
	}
	defer zipReader.Close()

	// Build file map for quick lookup
	fileMap := make(map[string]*zip.File)
	for _, file := range zipReader.File {
		fileMap[file.Name] = file
	}

	var paragraphs []DOCXParagraph
	var tables []DOCXTable
	var headers []DOCXParagraph
	var footers []DOCXParagraph

	// Parse main document
	if docFile, ok := fileMap["word/document.xml"]; ok {
		data, err := r.readZipFileContent(docFile)
		if err == nil {
			paragraphs = r.extractParagraphsFromDocumentXML(data)

			// Extract tables if enabled
			extractTables := true
			if val, ok := config["extract_tables"].(bool); ok {
				extractTables = val
			}
			if extractTables {
				tables = r.extractTablesFromDocumentXML(data)
			}
		}
	}

	// Parse headers if enabled
	extractHeaders := true
	if val, ok := config["extract_headers_footers"].(bool); ok {
		extractHeaders = val
	}
	if extractHeaders {
		// Headers are in word/header1.xml, header2.xml, etc.
		for name, file := range fileMap {
			if strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml") {
				data, err := r.readZipFileContent(file)
				if err == nil {
					headerParagraphs := r.extractParagraphsFromDocumentXML(data)
					for i := range headerParagraphs {
						headerParagraphs[i].Section = "header"
					}
					headers = append(headers, headerParagraphs...)
				}
			}
			if strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml") {
				data, err := r.readZipFileContent(file)
				if err == nil {
					footerParagraphs := r.extractParagraphsFromDocumentXML(data)
					for i := range footerParagraphs {
						footerParagraphs[i].Section = "footer"
					}
					footers = append(footers, footerParagraphs...)
				}
			}
		}
	}

	// Update metadata with actual paragraph count
	if len(paragraphs) > 0 {
		metadata.ParagraphCount = len(paragraphs)
	}

	return &DOCXDocument{
		Metadata:   metadata,
		Paragraphs: paragraphs,
		Headers:    headers,
		Footers:    footers,
		Tables:     tables,
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
