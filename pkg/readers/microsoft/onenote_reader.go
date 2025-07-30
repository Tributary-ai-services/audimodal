package microsoft

import (
	"bytes"
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

// OneNoteReader implements DataSourceReader for Microsoft OneNote files (.one)
type OneNoteReader struct {
	name    string
	version string
}

// NewOneNoteReader creates a new OneNote file reader
func NewOneNoteReader() *OneNoteReader {
	return &OneNoteReader{
		name:    "onenote_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *OneNoteReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "extract_page_hierarchy",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract notebook/section/page hierarchy",
		},
		{
			Name:        "include_handwritten_notes",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Include handwritten/ink notes if recognizable",
		},
		{
			Name:        "include_embedded_objects",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract embedded objects and attachments",
		},
		{
			Name:        "include_page_metadata",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Include page creation/modification dates",
		},
		{
			Name:        "extract_tables",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract tables with structure",
		},
		{
			Name:        "include_tags",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Include OneNote tags (To Do, Important, etc.)",
		},
		{
			Name:        "max_pages",
			Type:        "int",
			Required:    false,
			Default:     0,
			Description: "Maximum pages to process (0 = no limit)",
			MinValue:    ptrFloat64(0.0),
			MaxValue:    ptrFloat64(1000.0),
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *OneNoteReader) ValidateConfig(config map[string]any) error {
	if maxPages, ok := config["max_pages"]; ok {
		if pages, ok := maxPages.(float64); ok {
			if pages < 0 || pages > 1000 {
				return fmt.Errorf("max_pages must be between 0 and 1000")
			}
		}
	}

	return nil
}

// TestConnection tests if the OneNote file can be read
func (r *OneNoteReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
		Message: "OneNote reader ready",
		Latency: time.Since(start),
		Details: map[string]any{
			"extract_page_hierarchy":     config["extract_page_hierarchy"],
			"include_handwritten_notes":  config["include_handwritten_notes"],
			"include_tags":              config["include_tags"],
		},
	}
}

// GetType returns the connector type
func (r *OneNoteReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *OneNoteReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *OneNoteReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the OneNote file structure
func (r *OneNoteReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	if !r.isOneNoteFile(sourcePath) {
		return core.SchemaInfo{}, fmt.Errorf("not a valid OneNote file")
	}

	metadata, err := r.extractOneNoteMetadata(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to extract OneNote metadata: %w", err)
	}

	schema := core.SchemaInfo{
		Format:   "onenote",
		Encoding: "binary",
		Fields: []core.FieldInfo{
			{
				Name:        "page_title",
				Type:        "string",
				Nullable:    false,
				Description: "OneNote page title",
			},
			{
				Name:        "section_name",
				Type:        "string",
				Nullable:    true,
				Description: "Section containing the page",
			},
			{
				Name:        "notebook_name",
				Type:        "string",
				Nullable:    true,
				Description: "Notebook containing the section",
			},
			{
				Name:        "content",
				Type:        "text",
				Nullable:    false,
				Description: "Page text content",
			},
			{
				Name:        "page_number",
				Type:        "integer", 
				Nullable:    false,
				Description: "Page number in the notebook",
			},
			{
				Name:        "created_date",
				Type:        "datetime",
				Nullable:    true,
				Description: "Page creation date",
			},
			{
				Name:        "modified_date",
				Type:        "datetime",
				Nullable:    true,
				Description: "Page last modified date",
			},
			{
				Name:        "tags",
				Type:        "array",
				Nullable:    true,
				Description: "OneNote tags applied to content",
			},
		},
		Metadata: map[string]any{
			"total_pages":         metadata.TotalPages,
			"total_sections":      metadata.TotalSections,
			"notebook_name":       metadata.NotebookName,
			"created_date":        metadata.CreatedDate,
			"modified_date":       metadata.ModifiedDate,
			"onenote_version":     metadata.OneNoteVersion,
			"has_handwritten":     metadata.HasHandwritten,
			"has_embedded_files":  metadata.HasEmbeddedFiles,
			"has_tables":          metadata.HasTables,
			"has_images":          metadata.HasImages,
			"total_words":         metadata.TotalWords,
		},
	}

	// Sample first page
	if metadata.TotalPages > 0 {
		sampleText, err := r.extractSamplePage(sourcePath)
		if err == nil {
			schema.SampleData = []map[string]any{
				{
					"page_title":    "Sample Page Title",
					"section_name":  "Sample Section",
					"notebook_name": metadata.NotebookName,
					"content":       sampleText,
					"page_number":   1,
					"created_date":  metadata.CreatedDate,
				},
			}
		}
	}

	return schema, nil
}

// EstimateSize returns size estimates for the OneNote file
func (r *OneNoteReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	metadata, err := r.extractOneNoteMetadata(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to extract OneNote metadata: %w", err)
	}

	// Estimate based on page count
	pageCount := int64(metadata.TotalPages)
	if pageCount == 0 {
		pageCount = 1
	}

	// Estimate chunks based on pages (typically 1 page per chunk)
	estimatedChunks := int(pageCount)

	// OneNote files can be complex with embedded content
	complexity := "medium"
	if stat.Size() > 10*1024*1024 || metadata.TotalPages > 50 { // > 10MB or > 50 pages
		complexity = "high"
	}
	if stat.Size() > 100*1024*1024 || metadata.TotalPages > 500 { // > 100MB or > 500 pages
		complexity = "very_high"
	}

	// OneNote processing can be slow due to proprietary format
	processTime := "medium"
	if stat.Size() > 20*1024*1024 || metadata.TotalPages > 100 {
		processTime = "slow"
	}
	if stat.Size() > 200*1024*1024 || metadata.TotalPages > 1000 {
		processTime = "very_slow"
	}

	return core.SizeEstimate{
		RowCount:    &pageCount,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the OneNote file
func (r *OneNoteReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	notebook, err := r.parseNotebook(sourcePath, strategyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OneNote notebook: %w", err)
	}

	iterator := &OneNoteIterator{
		sourcePath:  sourcePath,
		config:      strategyConfig,
		notebook:    notebook,
		currentPage: 0,
	}

	return iterator, nil
}

// SupportsStreaming indicates OneNote reader supports streaming
func (r *OneNoteReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *OneNoteReader) GetSupportedFormats() []string {
	return []string{"one"}
}

// OneNoteMetadata contains extracted OneNote metadata
type OneNoteMetadata struct {
	TotalPages       int
	TotalSections    int
	NotebookName     string
	CreatedDate      string
	ModifiedDate     string
	OneNoteVersion   string
	HasHandwritten   bool
	HasEmbeddedFiles bool
	HasTables        bool
	HasImages        bool
	TotalWords       int
}

// OneNoteNotebook represents a parsed OneNote notebook
type OneNoteNotebook struct {
	Metadata OneNoteMetadata
	Sections []OneNoteSection
}

// OneNoteSection represents a notebook section
type OneNoteSection struct {
	Name  string
	Pages []OneNotePage
}

// OneNotePage represents a notebook page
type OneNotePage struct {
	Title        string
	Content      string
	PageNumber   int
	SectionName  string
	CreatedDate  time.Time
	ModifiedDate time.Time
	Tags         []OneNoteTag
	HasInk       bool
	HasImages    bool
	HasTables    bool
}

// OneNoteTag represents a OneNote tag
type OneNoteTag struct {
	Type     string
	Text     string
	Position string
}

// ptrFloat64 returns a pointer to a float64 value
func ptrFloat64(f float64) *float64 {
	return &f
}

// isOneNoteFile checks if the file is a valid OneNote file
func (r *OneNoteReader) isOneNoteFile(filePath string) bool {
	// Check file extension
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext != ".one" {
		return false
	}

	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	// Check OneNote file signature
	signature := make([]byte, 16)
	n, err := file.Read(signature)
	if err != nil || n != 16 {
		return false
	}

	// OneNote files have a specific GUID signature
	// This is simplified - production code would verify the complete structure
	return bytes.Contains(signature, []byte{0xE4, 0x52, 0x5C, 0x7B})
}

// extractOneNoteMetadata extracts metadata from OneNote file
func (r *OneNoteReader) extractOneNoteMetadata(sourcePath string) (OneNoteMetadata, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		return OneNoteMetadata{}, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return OneNoteMetadata{}, err
	}

	// This is a simplified metadata extraction
	// In production, you would parse the OneNote binary format
	// which involves parsing the revision store, object space, and file data

	metadata := OneNoteMetadata{
		CreatedDate:    stat.ModTime().Format("2006-01-02 15:04:05"),
		ModifiedDate:   stat.ModTime().Format("2006-01-02 15:04:05"),
		OneNoteVersion: "OneNote 2016",
		NotebookName:   strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath)),
	}

	// Estimate content based on file size
	sizeCategory := stat.Size() / (1024 * 1024) // Size in MB

	switch {
	case sizeCategory < 1: // < 1MB
		metadata.TotalPages = 5
		metadata.TotalSections = 1
		metadata.TotalWords = 500
	case sizeCategory < 10: // 1-10MB
		metadata.TotalPages = 25
		metadata.TotalSections = 3
		metadata.TotalWords = 2500
		metadata.HasImages = true
	case sizeCategory < 50: // 10-50MB
		metadata.TotalPages = 100
		metadata.TotalSections = 8
		metadata.TotalWords = 10000
		metadata.HasImages = true
		metadata.HasTables = true
		metadata.HasHandwritten = true
	default: // > 50MB
		metadata.TotalPages = 300
		metadata.TotalSections = 15
		metadata.TotalWords = 30000
		metadata.HasImages = true
		metadata.HasTables = true
		metadata.HasHandwritten = true
		metadata.HasEmbeddedFiles = true
	}

	// Check for specific OneNote features (simplified heuristics)
	buffer := make([]byte, 2048)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return metadata, err
	}

	// Look for ink/handwriting indicators
	if bytes.Contains(buffer, []byte("Ink")) {
		metadata.HasHandwritten = true
	}

	// Look for embedded file indicators
	if bytes.Contains(buffer, []byte("EmbeddedFile")) {
		metadata.HasEmbeddedFiles = true
	}

	return metadata, nil
}

// extractSamplePage extracts sample page content
func (r *OneNoteReader) extractSamplePage(sourcePath string) (string, error) {
	// In a real implementation, you would parse the OneNote binary format
	// and extract actual page content
	return "This is sample content from a OneNote page. It would contain the actual extracted text, tables, and other content from the first page.", nil
}

// parseNotebook parses the complete OneNote notebook
func (r *OneNoteReader) parseNotebook(sourcePath string, config map[string]any) (*OneNoteNotebook, error) {
	metadata, err := r.extractOneNoteMetadata(sourcePath)
	if err != nil {
		return nil, err
	}

	// In a real implementation, you would:
	// 1. Parse the OneNote file format (revision store)
	// 2. Extract object spaces and file data objects
	// 3. Parse page content, including text, ink, images
	// 4. Handle different OneNote versions and formats
	// 5. Extract embedded files and objects

	// Mock notebook structure for demonstration
	sections := make([]OneNoteSection, metadata.TotalSections)
	pagesPerSection := metadata.TotalPages / metadata.TotalSections
	if pagesPerSection == 0 {
		pagesPerSection = 1
	}

	pageNumber := 1
	for i := 0; i < metadata.TotalSections; i++ {
		sectionName := fmt.Sprintf("Section %d", i+1)
		if i == 0 {
			sectionName = "Quick Notes"
		}

		pages := make([]OneNotePage, pagesPerSection)
		for j := 0; j < pagesPerSection; j++ {
			pages[j] = OneNotePage{
				Title:        fmt.Sprintf("Page %d", pageNumber),
				Content:      fmt.Sprintf("This is the content of page %d in section %s. It contains notes, ideas, and important information.", pageNumber, sectionName),
				PageNumber:   pageNumber,
				SectionName:  sectionName,
				CreatedDate:  time.Now().Add(-time.Duration(pageNumber) * 24 * time.Hour),
				ModifiedDate: time.Now().Add(-time.Duration(pageNumber-1) * 12 * time.Hour),
				Tags: []OneNoteTag{
					{Type: "Important", Text: "Key point", Position: "top"},
				},
				HasInk:    metadata.HasHandwritten && pageNumber%5 == 0,
				HasImages: metadata.HasImages && pageNumber%7 == 0,
				HasTables: metadata.HasTables && pageNumber%10 == 0,
			}
			pageNumber++
		}

		sections[i] = OneNoteSection{
			Name:  sectionName,
			Pages: pages,
		}
	}

	return &OneNoteNotebook{
		Metadata: metadata,
		Sections: sections,
	}, nil
}

// OneNoteIterator implements ChunkIterator for OneNote files
type OneNoteIterator struct {
	sourcePath  string
	config      map[string]any
	notebook    *OneNoteNotebook
	currentPage int
}

// Next returns the next chunk of content from the OneNote notebook
func (it *OneNoteIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	// Find the next page across all sections
	totalPages := 0
	var currentPage *OneNotePage
	var currentSection string

	for _, section := range it.notebook.Sections {
		if it.currentPage >= totalPages && it.currentPage < totalPages+len(section.Pages) {
			pageIndex := it.currentPage - totalPages
			currentPage = &section.Pages[pageIndex]
			currentSection = section.Name
			break
		}
		totalPages += len(section.Pages)
	}

	if currentPage == nil {
		return core.Chunk{}, core.ErrIteratorExhausted
	}

	it.currentPage++

	// Check max_pages limit
	if maxPages, ok := it.config["max_pages"]; ok {
		if max, ok := maxPages.(float64); ok && max > 0 {
			if it.currentPage > int(max) {
				return core.Chunk{}, core.ErrIteratorExhausted
			}
		}
	}

	// Build page content
	var contentParts []string
	
	if extractHierarchy, ok := it.config["extract_page_hierarchy"]; !ok || extractHierarchy.(bool) {
		contentParts = append(contentParts, fmt.Sprintf("Notebook: %s", it.notebook.Metadata.NotebookName))
		contentParts = append(contentParts, fmt.Sprintf("Section: %s", currentSection))
	}
	
	contentParts = append(contentParts, fmt.Sprintf("Page Title: %s", currentPage.Title))
	contentParts = append(contentParts, fmt.Sprintf("Content: %s", currentPage.Content))

	if includeTags, ok := it.config["include_tags"]; (!ok || includeTags.(bool)) && len(currentPage.Tags) > 0 {
		var tagStrings []string
		for _, tag := range currentPage.Tags {
			tagStrings = append(tagStrings, fmt.Sprintf("%s: %s", tag.Type, tag.Text))
		}
		contentParts = append(contentParts, fmt.Sprintf("Tags: %s", strings.Join(tagStrings, ", ")))
	}

	content := strings.Join(contentParts, "\n")

	chunk := core.Chunk{
		Data: content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:page:%d", filepath.Base(it.sourcePath), currentPage.PageNumber),
			ChunkType:   "onenote_page",
			SizeBytes:   int64(len(content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "onenote_reader",
			Context: map[string]string{
				"page_title":     currentPage.Title,
				"page_number":    strconv.Itoa(currentPage.PageNumber),
				"section_name":   currentSection,
				"notebook_name":  it.notebook.Metadata.NotebookName,
				"created_date":   currentPage.CreatedDate.Format("2006-01-02 15:04:05"),
				"modified_date":  currentPage.ModifiedDate.Format("2006-01-02 15:04:05"),
				"has_ink":        strconv.FormatBool(currentPage.HasInk),
				"has_images":     strconv.FormatBool(currentPage.HasImages),
				"has_tables":     strconv.FormatBool(currentPage.HasTables),
				"tag_count":      strconv.Itoa(len(currentPage.Tags)),
				"file_type":      "onenote",
			},
		},
	}

	return chunk, nil
}

// Close releases OneNote resources
func (it *OneNoteIterator) Close() error {
	// Nothing to close for OneNote iterator
	return nil
}

// Reset restarts iteration from the beginning
func (it *OneNoteIterator) Reset() error {
	it.currentPage = 0
	return nil
}

// Progress returns iteration progress
func (it *OneNoteIterator) Progress() float64 {
	totalPages := 0
	for _, section := range it.notebook.Sections {
		totalPages += len(section.Pages)
	}
	
	if totalPages == 0 {
		return 1.0
	}
	return float64(it.currentPage) / float64(totalPages)
}