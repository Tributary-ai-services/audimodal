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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/audimodal/pkg/core"
)

// OOXML namespaces used in PPTX files
const (
	nsDC    = "http://purl.org/dc/elements/1.1/"
	nsDCT   = "http://purl.org/dc/terms/"
	nsCP    = "http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
	nsA     = "http://schemas.openxmlformats.org/drawingml/2006/main"
	nsP     = "http://schemas.openxmlformats.org/presentationml/2006/main"
	nsR     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsRels  = "http://schemas.openxmlformats.org/package/2006/relationships"
)

// PPTXReader implements DataSourceReader for Microsoft PowerPoint presentations
type PPTXReader struct {
	name    string
	version string
}

// NewPPTXReader creates a new PPTX file reader
func NewPPTXReader() *PPTXReader {
	return &PPTXReader{
		name:    "pptx_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *PPTXReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "extract_slide_notes",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract slide notes and comments",
		},
		{
			Name:        "extract_master_slides",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract content from master slides",
		},
		{
			Name:        "include_hidden_slides",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Include hidden slides in extraction",
		},
		{
			Name:        "extract_shapes",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract text from shapes and text boxes",
		},
		{
			Name:        "extract_animations",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract animation and transition information",
		},
		{
			Name:        "max_slides",
			Type:        "int",
			Required:    false,
			Default:     0,
			Description: "Maximum slides to process (0 = all)",
			MinValue:    ptrFloat64(0.0),
			MaxValue:    ptrFloat64(1000.0),
		},
		{
			Name:        "extract_tables",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract tables with structure",
		},
		{
			Name:        "include_slide_layout",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Include slide layout information",
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *PPTXReader) ValidateConfig(config map[string]any) error {
	if maxSlides, ok := config["max_slides"]; ok {
		if num, ok := maxSlides.(float64); ok {
			if num < 0 || num > 1000 {
				return fmt.Errorf("max_slides must be between 0 and 1000")
			}
		}
	}
	return nil
}

// TestConnection tests if the PPTX can be read
func (r *PPTXReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
		Message: "PPTX reader ready",
		Latency: time.Since(start),
		Details: map[string]any{
			"extract_slide_notes": config["extract_slide_notes"],
			"extract_shapes":      config["extract_shapes"],
		},
	}
}

// GetType returns the connector type
func (r *PPTXReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *PPTXReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *PPTXReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the PPTX file structure
func (r *PPTXReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	metadata, err := r.extractPPTXMetadata(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to extract PPTX metadata: %w", err)
	}

	schema := core.SchemaInfo{
		Format:   "pptx",
		Encoding: "utf-8",
		Fields: []core.FieldInfo{
			{
				Name:        "content",
				Type:        "text",
				Nullable:    false,
				Description: "Slide text content",
			},
			{
				Name:        "slide_number",
				Type:        "integer",
				Nullable:    false,
				Description: "Slide number in presentation",
			},
			{
				Name:        "slide_title",
				Type:        "string",
				Nullable:    true,
				Description: "Slide title if available",
			},
			{
				Name:        "slide_layout",
				Type:        "string",
				Nullable:    true,
				Description: "Slide layout type",
			},
			{
				Name:        "notes",
				Type:        "text",
				Nullable:    true,
				Description: "Slide notes content",
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
			"slide_count":     metadata.SlideCount,
			"has_animations":  metadata.HasAnimations,
			"has_transitions": metadata.HasTransitions,
		},
	}

	// Sample first slide
	if metadata.SlideCount > 0 {
		sampleText, err := r.extractSampleSlideText(sourcePath)
		if err == nil {
			schema.SampleData = []map[string]any{
				{
					"content":      sampleText,
					"slide_number": 1,
					"slide_title":  "Sample Slide Title",
					"slide_layout": "title_content",
				},
			}
		}
	}

	return schema, nil
}

// EstimateSize returns size estimates for the PPTX file
func (r *PPTXReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	metadata, err := r.extractPPTXMetadata(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to extract PPTX metadata: %w", err)
	}

	// Estimate based on slide count
	slideCount := int64(metadata.SlideCount)
	if slideCount == 0 {
		slideCount = 1
	}

	// Each slide becomes one chunk
	estimatedChunks := int(slideCount)

	complexity := "low"
	if stat.Size() > 5*1024*1024 || metadata.SlideCount > 50 { // > 5MB or > 50 slides
		complexity = "medium"
	}
	if stat.Size() > 50*1024*1024 || metadata.SlideCount > 200 { // > 50MB or > 200 slides
		complexity = "high"
	}

	processTime := "fast"
	if stat.Size() > 10*1024*1024 || metadata.SlideCount > 100 {
		processTime = "medium"
	}
	if stat.Size() > 100*1024*1024 || metadata.SlideCount > 500 {
		processTime = "slow"
	}

	return core.SizeEstimate{
		RowCount:    &slideCount,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the PPTX file
func (r *PPTXReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	presentation, err := r.parsePresentation(sourcePath, strategyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PPTX presentation: %w", err)
	}

	iterator := &PPTXIterator{
		sourcePath:   sourcePath,
		config:       strategyConfig,
		presentation: presentation,
		currentSlide: 0,
	}

	return iterator, nil
}

// SupportsStreaming indicates PPTX reader supports streaming
func (r *PPTXReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *PPTXReader) GetSupportedFormats() []string {
	return []string{"pptx"}
}

// PPTXMetadata contains extracted PPTX metadata
type PPTXMetadata struct {
	Title          string
	Author         string
	Subject        string
	Keywords       string
	Description    string
	Creator        string
	CreatedDate    string
	ModifiedDate   string
	SlideCount     int
	HasAnimations  bool
	HasTransitions bool
}

// PPTXPresentation represents a parsed PPTX presentation
type PPTXPresentation struct {
	Metadata PPTXMetadata
	Slides   []PPTXSlide
}

// PPTXSlide represents a presentation slide
type PPTXSlide struct {
	Number     int
	Title      string
	Content    string
	Layout     string
	Notes      string
	Shapes     []PPTXShape
	Tables     []PPTXTable
	Hidden     bool
	Animations []string
}

// PPTXShape represents a shape or text box in a slide
type PPTXShape struct {
	Type   string
	Text   string
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// PPTXTable represents a table in a slide
type PPTXTable struct {
	Rows    [][]string
	Headers []string
}

// XML parsing structures for OOXML format

// coreProperties represents docProps/core.xml
type coreProperties struct {
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

// presentationXML represents ppt/presentation.xml
type presentationXML struct {
	XMLName   xml.Name         `xml:"presentation"`
	SlideList slideIdListXML   `xml:"sldIdLst"`
}

type slideIdListXML struct {
	Slides []slideIdXML `xml:"sldId"`
}

type slideIdXML struct {
	ID    string `xml:"id,attr"`
	RelID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
}

// slideXML represents ppt/slides/slideN.xml
type slideXML struct {
	XMLName    xml.Name        `xml:"sld"`
	CommonData slideCommonData `xml:"cSld"`
}

type slideCommonData struct {
	ShapeTree shapeTreeXML `xml:"spTree"`
}

type shapeTreeXML struct {
	Shapes []shapeXML `xml:"sp"`
}

type shapeXML struct {
	NvSpPr    nvSpPrXML    `xml:"nvSpPr"`
	TextBody  textBodyXML  `xml:"txBody"`
}

type nvSpPrXML struct {
	CNvPr    cNvPrXML    `xml:"cNvPr"`
	NvPr     nvPrXML     `xml:"nvPr"`
}

type cNvPrXML struct {
	Name string `xml:"name,attr"`
}

type nvPrXML struct {
	Ph phXML `xml:"ph"`
}

type phXML struct {
	Type string `xml:"type,attr"`
	Idx  string `xml:"idx,attr"`
}

type textBodyXML struct {
	Paragraphs []paragraphXML `xml:"p"`
}

type paragraphXML struct {
	Runs []runXML `xml:"r"`
}

type runXML struct {
	Text string `xml:"t"`
}

// notesSlideXML represents ppt/notesSlides/notesSlideN.xml
type notesSlideXML struct {
	XMLName    xml.Name        `xml:"notes"`
	CommonData slideCommonData `xml:"cSld"`
}

// relationshipsXML represents _rels/.rels and other .rels files
type relationshipsXML struct {
	XMLName       xml.Name          `xml:"Relationships"`
	Relationships []relationshipXML `xml:"Relationship"`
}

type relationshipXML struct {
	ID     string `xml:"Id,attr"`
	Type   string `xml:"Type,attr"`
	Target string `xml:"Target,attr"`
}

// extractPPTXMetadata extracts metadata from PPTX file
func (r *PPTXReader) extractPPTXMetadata(sourcePath string) (PPTXMetadata, error) {
	zipReader, err := zip.OpenReader(sourcePath)
	if err != nil {
		return PPTXMetadata{}, fmt.Errorf("failed to open PPTX file: %w", err)
	}
	defer zipReader.Close()

	metadata := PPTXMetadata{
		Title:          filepath.Base(sourcePath),
		CreatedDate:    time.Now().Format("2006-01-02"),
		ModifiedDate:   time.Now().Format("2006-01-02"),
		HasAnimations:  false,
		HasTransitions: false,
	}

	// Count slides by looking for slide*.xml files
	slideCount := 0
	slidePattern := regexp.MustCompile(`^ppt/slides/slide\d+\.xml$`)

	for _, file := range zipReader.File {
		if slidePattern.MatchString(file.Name) {
			slideCount++
		}
	}
	metadata.SlideCount = slideCount

	// Parse core.xml for metadata
	for _, file := range zipReader.File {
		if file.Name == "docProps/core.xml" {
			rc, err := file.Open()
			if err != nil {
				continue
			}

			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				continue
			}

			// Parse the core properties XML
			var props coreProperties
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
			break
		}
	}

	return metadata, nil
}

// extractSampleSlideText extracts sample text from the first slide
func (r *PPTXReader) extractSampleSlideText(sourcePath string) (string, error) {
	zipReader, err := zip.OpenReader(sourcePath)
	if err != nil {
		return "", err
	}
	defer zipReader.Close()

	// Look for slide1.xml
	for _, file := range zipReader.File {
		if file.Name == "ppt/slides/slide1.xml" {
			text, err := r.extractTextFromSlideFile(file)
			if err != nil {
				return "", err
			}
			return text, nil
		}
	}
	return "", fmt.Errorf("slide1.xml not found")
}

// extractTextFromSlideFile extracts all text content from a slide XML file
func (r *PPTXReader) extractTextFromSlideFile(file *zip.File) (string, error) {
	rc, err := file.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}

	return r.extractTextFromSlideXML(data)
}

// extractTextFromSlideXML extracts text from slide XML content
func (r *PPTXReader) extractTextFromSlideXML(data []byte) (string, error) {
	// Extract all text between <a:t> tags using regex
	// This handles the DrawingML text format used in PPTX files
	textPattern := regexp.MustCompile(`<a:t[^>]*>([^<]*)</a:t>`)
	matches := textPattern.FindAllSubmatch(data, -1)

	var texts []string
	for _, match := range matches {
		if len(match) > 1 {
			text := strings.TrimSpace(string(match[1]))
			if text != "" {
				texts = append(texts, text)
			}
		}
	}

	return strings.Join(texts, " "), nil
}

// extractTitleFromSlideXML attempts to extract the slide title
func (r *PPTXReader) extractTitleFromSlideXML(data []byte) string {
	// Look for title placeholder type="title" or type="ctrTitle"
	// The title is typically in a shape with ph type="title"
	titlePattern := regexp.MustCompile(`(?s)<p:sp[^>]*>.*?<p:ph[^>]*type="(?:title|ctrTitle)"[^>]*/>.*?<p:txBody>(.*?)</p:txBody>.*?</p:sp>`)
	match := titlePattern.FindSubmatch(data)
	if len(match) > 1 {
		return r.extractTextFromTxBody(match[1])
	}

	// Fallback: look for any shape with title-like content
	// Often the first text box in a slide contains the title
	return ""
}

// extractTextFromTxBody extracts text from a txBody element
func (r *PPTXReader) extractTextFromTxBody(txBodyData []byte) string {
	textPattern := regexp.MustCompile(`<a:t[^>]*>([^<]*)</a:t>`)
	matches := textPattern.FindAllSubmatch(txBodyData, -1)

	var texts []string
	for _, match := range matches {
		if len(match) > 1 {
			text := strings.TrimSpace(string(match[1]))
			if text != "" {
				texts = append(texts, text)
			}
		}
	}

	return strings.Join(texts, " ")
}

// parsePresentation parses the complete PPTX presentation
func (r *PPTXReader) parsePresentation(sourcePath string, config map[string]any) (*PPTXPresentation, error) {
	metadata, err := r.extractPPTXMetadata(sourcePath)
	if err != nil {
		return nil, err
	}

	zipReader, err := zip.OpenReader(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PPTX file: %w", err)
	}
	defer zipReader.Close()

	// Build a map of file names to zip files for quick lookup
	fileMap := make(map[string]*zip.File)
	for _, file := range zipReader.File {
		fileMap[file.Name] = file
	}

	// Find all slide files and sort them by slide number
	slidePattern := regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)
	var slideNumbers []int
	slideFileMap := make(map[int]*zip.File)

	for _, file := range zipReader.File {
		if matches := slidePattern.FindStringSubmatch(file.Name); matches != nil {
			num, _ := strconv.Atoi(matches[1])
			slideNumbers = append(slideNumbers, num)
			slideFileMap[num] = file
		}
	}

	// Sort slide numbers to process in order
	sort.Ints(slideNumbers)

	// Parse each slide
	slides := make([]PPTXSlide, 0, len(slideNumbers))
	extractNotes := true
	if val, ok := config["extract_slide_notes"].(bool); ok {
		extractNotes = val
	}

	for _, slideNum := range slideNumbers {
		slideFile := slideFileMap[slideNum]

		// Read slide content
		slideData, err := r.readZipFileContent(slideFile)
		if err != nil {
			continue
		}

		// Extract text content from slide
		content, err := r.extractTextFromSlideXML(slideData)
		if err != nil {
			content = ""
		}

		// Extract title from slide
		title := r.extractTitleFromSlideXML(slideData)
		if title == "" {
			// Use first line of content as title if no explicit title found
			lines := strings.Split(content, " ")
			if len(lines) > 0 && len(lines[0]) < 100 {
				title = lines[0]
			}
		}

		// Extract shapes with text
		shapes := r.extractShapesFromSlideXML(slideData)

		// Try to extract notes
		notes := ""
		if extractNotes {
			notesFile := fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", slideNum)
			if notesZipFile, ok := fileMap[notesFile]; ok {
				notesData, err := r.readZipFileContent(notesZipFile)
				if err == nil {
					notes, _ = r.extractTextFromSlideXML(notesData)
				}
			}
		}

		slide := PPTXSlide{
			Number:     slideNum,
			Title:      title,
			Content:    content,
			Layout:     "default",
			Notes:      notes,
			Shapes:     shapes,
			Tables:     []PPTXTable{},
			Hidden:     false,
			Animations: []string{},
		}

		slides = append(slides, slide)
	}

	return &PPTXPresentation{
		Metadata: metadata,
		Slides:   slides,
	}, nil
}

// readZipFileContent reads the content of a zip file entry
func (r *PPTXReader) readZipFileContent(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// extractShapesFromSlideXML extracts shapes with text from slide XML
func (r *PPTXReader) extractShapesFromSlideXML(data []byte) []PPTXShape {
	var shapes []PPTXShape

	// Find all sp (shape) elements with text bodies
	// Pattern to find shapes with text content
	shapePattern := regexp.MustCompile(`(?s)<p:sp[^>]*>(.*?)</p:sp>`)
	shapeMatches := shapePattern.FindAllSubmatch(data, -1)

	for _, shapeMatch := range shapeMatches {
		if len(shapeMatch) < 2 {
			continue
		}

		shapeContent := shapeMatch[1]

		// Check if this shape has a text body
		if !strings.Contains(string(shapeContent), "<p:txBody>") {
			continue
		}

		// Extract text from this shape
		text := r.extractTextFromTxBody(shapeContent)
		if text == "" {
			continue
		}

		// Determine shape type from placeholder
		shapeType := "textbox"
		if strings.Contains(string(shapeContent), `type="title"`) || strings.Contains(string(shapeContent), `type="ctrTitle"`) {
			shapeType = "title"
		} else if strings.Contains(string(shapeContent), `type="body"`) {
			shapeType = "body"
		} else if strings.Contains(string(shapeContent), `type="subTitle"`) {
			shapeType = "subtitle"
		}

		shapes = append(shapes, PPTXShape{
			Type: shapeType,
			Text: text,
		})
	}

	return shapes
}

// PPTXIterator implements ChunkIterator for PPTX files
type PPTXIterator struct {
	sourcePath   string
	config       map[string]any
	presentation *PPTXPresentation
	currentSlide int
}

// Next returns the next chunk of content from the PPTX
func (it *PPTXIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	// Check if we've processed all slides
	if it.currentSlide >= len(it.presentation.Slides) {
		return core.Chunk{}, core.ErrIteratorExhausted
	}

	// Check max_slides limit
	if maxSlides, ok := it.config["max_slides"]; ok {
		if max, ok := maxSlides.(float64); ok && max > 0 {
			if it.currentSlide >= int(max) {
				return core.Chunk{}, core.ErrIteratorExhausted
			}
		}
	}

	slide := it.presentation.Slides[it.currentSlide]
	it.currentSlide++

	// Skip hidden slides if not configured to include them
	if slide.Hidden {
		if includeHidden, ok := it.config["include_hidden_slides"].(bool); !ok || !includeHidden {
			return it.Next(ctx) // Skip this slide and get the next one
		}
	}

	// Build slide content
	var contentParts []string

	// Add title
	if slide.Title != "" {
		contentParts = append(contentParts, fmt.Sprintf("Title: %s", slide.Title))
	}

	// Add main content
	if slide.Content != "" {
		contentParts = append(contentParts, slide.Content)
	}

	// Add shape text if enabled
	if extractShapes, ok := it.config["extract_shapes"].(bool); !ok || extractShapes {
		for _, shape := range slide.Shapes {
			if shape.Text != "" {
				contentParts = append(contentParts, fmt.Sprintf("Shape (%s): %s", shape.Type, shape.Text))
			}
		}
	}

	// Add notes if enabled
	if extractNotes, ok := it.config["extract_slide_notes"].(bool); !ok || extractNotes {
		if slide.Notes != "" {
			contentParts = append(contentParts, fmt.Sprintf("Notes: %s", slide.Notes))
		}
	}

	content := strings.Join(contentParts, "\n\n")

	chunk := core.Chunk{
		Data: content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:slide:%d", filepath.Base(it.sourcePath), slide.Number),
			ChunkType:   "pptx_slide",
			SizeBytes:   int64(len(content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "pptx_reader",
			Context: map[string]string{
				"slide_number":        strconv.Itoa(slide.Number),
				"slide_title":         slide.Title,
				"slide_layout":        slide.Layout,
				"total_slides":        strconv.Itoa(len(it.presentation.Slides)),
				"file_type":           "pptx",
				"presentation_title":  it.presentation.Metadata.Title,
				"presentation_author": it.presentation.Metadata.Author,
				"has_animations":      strconv.FormatBool(len(slide.Animations) > 0),
				"shape_count":         strconv.Itoa(len(slide.Shapes)),
			},
		},
	}

	// Add layout information if enabled
	if includeLayout, ok := it.config["include_slide_layout"].(bool); !ok || includeLayout {
		chunk.Metadata.Context["slide_layout"] = slide.Layout
	}

	return chunk, nil
}

// Close releases PPTX resources
func (it *PPTXIterator) Close() error {
	// Nothing to close for PPTX iterator
	return nil
}

// Reset restarts iteration from the beginning
func (it *PPTXIterator) Reset() error {
	it.currentSlide = 0
	return nil
}

// Progress returns iteration progress
func (it *PPTXIterator) Progress() float64 {
	totalSlides := len(it.presentation.Slides)
	if totalSlides == 0 {
		return 1.0
	}
	return float64(it.currentSlide) / float64(totalSlides)
}
