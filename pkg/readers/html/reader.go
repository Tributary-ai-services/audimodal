package html

import (
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

// HTMLReader implements DataSourceReader for HTML/XHTML files
type HTMLReader struct {
	name    string
	version string
}

// NewHTMLReader creates a new HTML file reader
func NewHTMLReader() *HTMLReader {
	return &HTMLReader{
		name:    "html_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *HTMLReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "extract_text_only",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract only text content, stripping all HTML tags",
		},
		{
			Name:        "preserve_structure",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Preserve document structure (headings, lists, etc.)",
		},
		{
			Name:        "extract_links",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract and preserve hyperlinks",
		},
		{
			Name:        "extract_images",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract image references and alt text",
		},
		{
			Name:        "extract_metadata",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract meta tags and document metadata",
		},
		{
			Name:        "remove_scripts",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Remove script tags and their content",
		},
		{
			Name:        "remove_styles",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Remove style tags and their content",
		},
		{
			Name:        "chunk_by_element",
			Type:        "string",
			Required:    false,
			Default:     "auto",
			Description: "How to chunk HTML content",
			Enum:        []string{"auto", "paragraph", "section", "heading", "div"},
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
	}
}

// ValidateConfig validates the provided configuration
func (r *HTMLReader) ValidateConfig(config map[string]any) error {
	if chunkBy, ok := config["chunk_by_element"]; ok {
		if str, ok := chunkBy.(string); ok {
			validModes := []string{"auto", "paragraph", "section", "heading", "div"}
			found := false
			for _, valid := range validModes {
				if str == valid {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("invalid chunk_by_element: %s", str)
			}
		}
	}

	if maxSize, ok := config["max_chunk_size"]; ok {
		if num, ok := maxSize.(float64); ok {
			if num < 100 || num > 10000 {
				return fmt.Errorf("max_chunk_size must be between 100 and 10000")
			}
		}
	}

	return nil
}

// TestConnection tests if the HTML can be read
func (r *HTMLReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
		Message: "HTML reader ready",
		Latency: time.Since(start),
		Details: map[string]any{
			"extract_text_only": config["extract_text_only"],
			"chunk_by_element":  config["chunk_by_element"],
		},
	}
}

// GetType returns the connector type
func (r *HTMLReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *HTMLReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *HTMLReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the HTML file structure
func (r *HTMLReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to read file: %w", err)
	}

	metadata := r.extractHTMLMetadata(string(content))
	structure := r.analyzeHTMLStructure(string(content))

	schema := core.SchemaInfo{
		Format:   "html",
		Encoding: "utf-8",
		Fields: []core.FieldInfo{
			{
				Name:        "content",
				Type:        "text",
				Nullable:    false,
				Description: "Extracted text content",
			},
			{
				Name:        "element_type",
				Type:        "string",
				Nullable:    false,
				Description: "HTML element type (p, h1, div, etc.)",
			},
			{
				Name:        "element_id",
				Type:        "string",
				Nullable:    true,
				Description: "Element ID attribute if present",
			},
			{
				Name:        "element_class",
				Type:        "string",
				Nullable:    true,
				Description: "Element class attribute if present",
			},
			{
				Name:        "links",
				Type:        "array",
				Nullable:    true,
				Description: "Hyperlinks found in content",
			},
			{
				Name:        "images",
				Type:        "array",
				Nullable:    true,
				Description: "Images found in content",
			},
		},
		Metadata: map[string]any{
			"title":           metadata.Title,
			"description":     metadata.Description,
			"keywords":        metadata.Keywords,
			"author":          metadata.Author,
			"charset":         metadata.Charset,
			"viewport":        metadata.Viewport,
			"og_title":        metadata.OGTitle,
			"og_description":  metadata.OGDescription,
			"heading_count":   structure.HeadingCount,
			"paragraph_count": structure.ParagraphCount,
			"link_count":      structure.LinkCount,
			"image_count":     structure.ImageCount,
			"has_tables":      structure.HasTables,
			"has_forms":       structure.HasForms,
		},
	}

	// Extract sample content
	sampleText := r.extractSampleText(string(content))
	if sampleText != "" {
		schema.SampleData = []map[string]any{
			{
				"content":      sampleText,
				"element_type": "p",
				"links":        []string{},
			},
		}
	}

	return schema, nil
}

// EstimateSize returns size estimates for the HTML file
func (r *HTMLReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to read file: %w", err)
	}

	// Remove tags for text size estimation
	textContent := r.stripHTMLTags(string(content))
	textSize := int64(len(textContent))

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

	elementCount := int64(r.countHTMLElements(string(content)))
	return core.SizeEstimate{
		RowCount:    &elementCount,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the HTML file
func (r *HTMLReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read HTML file: %w", err)
	}

	elements := r.parseHTMLElements(string(content), strategyConfig)

	iterator := &HTMLIterator{
		sourcePath:     sourcePath,
		config:         strategyConfig,
		elements:       elements,
		currentElement: 0,
	}

	return iterator, nil
}

// SupportsStreaming indicates HTML reader supports streaming
func (r *HTMLReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *HTMLReader) GetSupportedFormats() []string {
	return []string{"html", "htm", "xhtml"}
}

// HTMLMetadata contains extracted HTML metadata
type HTMLMetadata struct {
	Title         string
	Description   string
	Keywords      string
	Author        string
	Charset       string
	Viewport      string
	OGTitle       string
	OGDescription string
}

// HTMLStructure contains HTML structure analysis
type HTMLStructure struct {
	HeadingCount   int
	ParagraphCount int
	LinkCount      int
	ImageCount     int
	HasTables      bool
	HasForms       bool
}

// HTMLElement represents a parsed HTML element
type HTMLElement struct {
	Type    string
	ID      string
	Class   string
	Content string
	Links   []string
	Images  []string
}

// extractHTMLMetadata extracts metadata from HTML content
func (r *HTMLReader) extractHTMLMetadata(content string) HTMLMetadata {
	metadata := HTMLMetadata{
		Charset: "utf-8", // Default
	}

	// Extract title
	if match := regexp.MustCompile(`<title[^>]*>([^<]+)</title>`).FindStringSubmatch(content); len(match) > 1 {
		metadata.Title = strings.TrimSpace(match[1])
	}

	// Extract meta tags
	metaRegex := regexp.MustCompile(`<meta\s+([^>]+)>`)
	for _, match := range metaRegex.FindAllStringSubmatch(content, -1) {
		attrs := match[1]

		// Extract description
		if strings.Contains(attrs, `name="description"`) || strings.Contains(attrs, `name='description'`) {
			if contentMatch := regexp.MustCompile(`content=["']([^"']+)["']`).FindStringSubmatch(attrs); len(contentMatch) > 1 {
				metadata.Description = contentMatch[1]
			}
		}

		// Extract keywords
		if strings.Contains(attrs, `name="keywords"`) || strings.Contains(attrs, `name='keywords'`) {
			if contentMatch := regexp.MustCompile(`content=["']([^"']+)["']`).FindStringSubmatch(attrs); len(contentMatch) > 1 {
				metadata.Keywords = contentMatch[1]
			}
		}

		// Extract author
		if strings.Contains(attrs, `name="author"`) || strings.Contains(attrs, `name='author'`) {
			if contentMatch := regexp.MustCompile(`content=["']([^"']+)["']`).FindStringSubmatch(attrs); len(contentMatch) > 1 {
				metadata.Author = contentMatch[1]
			}
		}

		// Extract charset
		if strings.Contains(attrs, `charset=`) {
			if charsetMatch := regexp.MustCompile(`charset=["']?([^"'\s>]+)`).FindStringSubmatch(attrs); len(charsetMatch) > 1 {
				metadata.Charset = charsetMatch[1]
			}
		}

		// Extract viewport
		if strings.Contains(attrs, `name="viewport"`) || strings.Contains(attrs, `name='viewport'`) {
			if contentMatch := regexp.MustCompile(`content=["']([^"']+)["']`).FindStringSubmatch(attrs); len(contentMatch) > 1 {
				metadata.Viewport = contentMatch[1]
			}
		}

		// Extract Open Graph tags
		if strings.Contains(attrs, `property="og:title"`) || strings.Contains(attrs, `property='og:title'`) {
			if contentMatch := regexp.MustCompile(`content=["']([^"']+)["']`).FindStringSubmatch(attrs); len(contentMatch) > 1 {
				metadata.OGTitle = contentMatch[1]
			}
		}

		if strings.Contains(attrs, `property="og:description"`) || strings.Contains(attrs, `property='og:description'`) {
			if contentMatch := regexp.MustCompile(`content=["']([^"']+)["']`).FindStringSubmatch(attrs); len(contentMatch) > 1 {
				metadata.OGDescription = contentMatch[1]
			}
		}
	}

	return metadata
}

// analyzeHTMLStructure analyzes the structure of HTML content
func (r *HTMLReader) analyzeHTMLStructure(content string) HTMLStructure {
	structure := HTMLStructure{}

	// Count headings
	headingRegex := regexp.MustCompile(`<h[1-6][^>]*>`)
	structure.HeadingCount = len(headingRegex.FindAllString(content, -1))

	// Count paragraphs
	paragraphRegex := regexp.MustCompile(`<p[^>]*>`)
	structure.ParagraphCount = len(paragraphRegex.FindAllString(content, -1))

	// Count links
	linkRegex := regexp.MustCompile(`<a[^>]*>`)
	structure.LinkCount = len(linkRegex.FindAllString(content, -1))

	// Count images
	imageRegex := regexp.MustCompile(`<img[^>]*>`)
	structure.ImageCount = len(imageRegex.FindAllString(content, -1))

	// Check for tables
	structure.HasTables = strings.Contains(content, "<table")

	// Check for forms
	structure.HasForms = strings.Contains(content, "<form")

	return structure
}

// stripHTMLTags removes HTML tags from content
func (r *HTMLReader) stripHTMLTags(content string) string {
	// Remove script and style content
	content = regexp.MustCompile(`<script[^>]*>[\s\S]*?</script>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`<style[^>]*>[\s\S]*?</style>`).ReplaceAllString(content, "")

	// Remove all HTML tags
	content = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(content, " ")

	// Clean up whitespace
	content = regexp.MustCompile(`\s+`).ReplaceAllString(content, " ")

	return strings.TrimSpace(content)
}

// extractSampleText extracts sample text from HTML
func (r *HTMLReader) extractSampleText(content string) string {
	// Find first paragraph
	if match := regexp.MustCompile(`<p[^>]*>([^<]+)</p>`).FindStringSubmatch(content); len(match) > 1 {
		text := r.stripHTMLTags(match[1])
		if len(text) > 200 {
			return text[:200] + "..."
		}
		return text
	}

	// Fall back to stripping all tags and taking first 200 chars
	text := r.stripHTMLTags(content)
	if len(text) > 200 {
		return text[:200] + "..."
	}
	return text
}

// countHTMLElements counts significant HTML elements
func (r *HTMLReader) countHTMLElements(content string) int {
	count := 0

	// Count various block elements
	elements := []string{"p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "li", "tr", "section", "article"}
	for _, elem := range elements {
		regex := regexp.MustCompile(fmt.Sprintf(`<%s[^>]*>`, elem))
		count += len(regex.FindAllString(content, -1))
	}

	return count
}

// parseHTMLElements parses HTML into processable elements
func (r *HTMLReader) parseHTMLElements(content string, config map[string]any) []HTMLElement {
	var elements []HTMLElement

	// Remove scripts and styles if configured
	if removeScripts, ok := config["remove_scripts"].(bool); !ok || removeScripts {
		content = regexp.MustCompile(`<script[^>]*>[\s\S]*?</script>`).ReplaceAllString(content, "")
	}
	if removeStyles, ok := config["remove_styles"].(bool); !ok || removeStyles {
		content = regexp.MustCompile(`<style[^>]*>[\s\S]*?</style>`).ReplaceAllString(content, "")
	}

	// For mock implementation, extract basic elements
	// In production, use a proper HTML parser like golang.org/x/net/html

	// Track element positions for proper ordering
	type elementPos struct {
		element HTMLElement
		pos     int
	}
	var elementsWithPos []elementPos

	// Extract headings
	for i := 1; i <= 6; i++ {
		headingRegex := regexp.MustCompile(fmt.Sprintf(`<h%d[^>]*>([^<]+)</h%d>`, i, i))
		for _, match := range headingRegex.FindAllStringSubmatchIndex(content, -1) {
			if len(match) >= 4 {
				elementsWithPos = append(elementsWithPos, elementPos{
					element: HTMLElement{
						Type:    fmt.Sprintf("h%d", i),
						Content: strings.TrimSpace(content[match[2]:match[3]]),
					},
					pos: match[0],
				})
			}
		}
	}

	// Extract paragraphs
	paragraphRegex := regexp.MustCompile(`<p[^>]*>([^<]+)</p>`)
	for _, match := range paragraphRegex.FindAllStringSubmatchIndex(content, -1) {
		if len(match) >= 4 {
			elementsWithPos = append(elementsWithPos, elementPos{
				element: HTMLElement{
					Type:    "p",
					Content: strings.TrimSpace(content[match[2]:match[3]]),
				},
				pos: match[0],
			})
		}
	}

	// Sort elements by position
	for i := 0; i < len(elementsWithPos); i++ {
		for j := i + 1; j < len(elementsWithPos); j++ {
			if elementsWithPos[j].pos < elementsWithPos[i].pos {
				elementsWithPos[i], elementsWithPos[j] = elementsWithPos[j], elementsWithPos[i]
			}
		}
	}

	// Extract sorted elements
	for _, ep := range elementsWithPos {
		elements = append(elements, ep.element)
	}

	// If no elements found, treat entire content as one element
	if len(elements) == 0 {
		text := r.stripHTMLTags(content)
		if text != "" {
			elements = append(elements, HTMLElement{
				Type:    "body",
				Content: text,
			})
		}
	}

	return elements
}

// HTMLIterator implements ChunkIterator for HTML files
type HTMLIterator struct {
	sourcePath     string
	config         map[string]any
	elements       []HTMLElement
	currentElement int
}

// Next returns the next chunk of content from the HTML
func (it *HTMLIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	// Check if we've processed all elements
	if it.currentElement >= len(it.elements) {
		return core.Chunk{}, core.ErrIteratorExhausted
	}

	element := it.elements[it.currentElement]
	it.currentElement++

	chunk := core.Chunk{
		Data: element.Content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:element:%d", filepath.Base(it.sourcePath), it.currentElement),
			ChunkType:   "html_element",
			SizeBytes:   int64(len(element.Content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "html_reader",
			Context: map[string]string{
				"element_type":   element.Type,
				"element_index":  strconv.Itoa(it.currentElement),
				"total_elements": strconv.Itoa(len(it.elements)),
				"file_type":      "html",
			},
		},
	}

	// Add element attributes if present
	if element.ID != "" {
		chunk.Metadata.Context["element_id"] = element.ID
	}
	if element.Class != "" {
		chunk.Metadata.Context["element_class"] = element.Class
	}
	if len(element.Links) > 0 {
		chunk.Metadata.Context["link_count"] = strconv.Itoa(len(element.Links))
	}
	if len(element.Images) > 0 {
		chunk.Metadata.Context["image_count"] = strconv.Itoa(len(element.Images))
	}

	return chunk, nil
}

// Close releases HTML resources
func (it *HTMLIterator) Close() error {
	// Nothing to close for HTML iterator
	return nil
}

// Reset restarts iteration from the beginning
func (it *HTMLIterator) Reset() error {
	it.currentElement = 0
	return nil
}

// Progress returns iteration progress
func (it *HTMLIterator) Progress() float64 {
	if len(it.elements) == 0 {
		return 1.0
	}
	return float64(it.currentElement) / float64(len(it.elements))
}

// Helper functions
func ptrFloat64(f float64) *float64 {
	return &f
}
