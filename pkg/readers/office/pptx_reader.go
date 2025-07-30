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
			"title":       metadata.Title,
			"author":      metadata.Author,
			"subject":     metadata.Subject,
			"keywords":    metadata.Keywords,
			"description": metadata.Description,
			"creator":     metadata.Creator,
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
	Type    string
	Text    string
	X       float64
	Y       float64
	Width   float64
	Height  float64
}

// PPTXTable represents a table in a slide
type PPTXTable struct {
	Rows    [][]string
	Headers []string
}

// extractPPTXMetadata extracts metadata from PPTX file
func (r *PPTXReader) extractPPTXMetadata(sourcePath string) (PPTXMetadata, error) {
	zipReader, err := zip.OpenReader(sourcePath)
	if err != nil {
		return PPTXMetadata{}, fmt.Errorf("failed to open PPTX file: %w", err)
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
			return PPTXMetadata{
				Title:          "Sample Presentation",
				Author:         "Unknown",
				Subject:        "Presentation Subject",
				Keywords:       "sample, presentation",
				Description:    "Sample PPTX presentation",
				Creator:        "Microsoft PowerPoint",
				CreatedDate:    time.Now().Format("2006-01-02"),
				ModifiedDate:   time.Now().Format("2006-01-02"),
				SlideCount:     15,
				HasAnimations:  true,
				HasTransitions: true,
			}, nil
		}
	}

	// Default metadata if core.xml not found
	return PPTXMetadata{
		Title:          filepath.Base(sourcePath),
		SlideCount:     10,
		CreatedDate:    time.Now().Format("2006-01-02"),
		ModifiedDate:   time.Now().Format("2006-01-02"),
		HasAnimations:  false,
		HasTransitions: false,
	}, nil
}

// extractSampleSlideText extracts sample text from the first slide
func (r *PPTXReader) extractSampleSlideText(sourcePath string) (string, error) {
	// In a real implementation, would parse slide1.xml
	return "This is sample text from the first slide of the PPTX presentation. It would contain the actual extracted content including titles, bullet points, and other text elements.", nil
}

// parsePresentation parses the complete PPTX presentation
func (r *PPTXReader) parsePresentation(sourcePath string, config map[string]any) (*PPTXPresentation, error) {
	metadata, err := r.extractPPTXMetadata(sourcePath)
	if err != nil {
		return nil, err
	}

	// In a real implementation, would parse:
	// - presentation.xml for slide structure
	// - slide*.xml for slide content
	// - slideLayout*.xml for layout information
	// - slideMaster*.xml for master slides
	// - notesSlide*.xml for slide notes

	// Mock presentation structure
	slides := make([]PPTXSlide, metadata.SlideCount)
	for i := 0; i < metadata.SlideCount; i++ {
		slides[i] = PPTXSlide{
			Number:  i + 1,
			Title:   fmt.Sprintf("Slide %d Title", i+1),
			Content: fmt.Sprintf("This is the content for slide %d. It includes bullet points:\n• Point 1\n• Point 2\n• Point 3", i+1),
			Layout:  "title_content",
			Notes:   fmt.Sprintf("These are speaker notes for slide %d.", i+1),
			Shapes: []PPTXShape{
				{
					Type:   "textbox",
					Text:   fmt.Sprintf("Additional text from shape on slide %d", i+1),
					X:      100,
					Y:      200,
					Width:  400,
					Height: 100,
				},
			},
			Tables: []PPTXTable{},
			Hidden: false,
			Animations: []string{"fadeIn", "slideFromLeft"},
		}
	}

	return &PPTXPresentation{
		Metadata: metadata,
		Slides:   slides,
	}, nil
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
				"slide_number":     strconv.Itoa(slide.Number),
				"slide_title":      slide.Title,
				"slide_layout":     slide.Layout,
				"total_slides":     strconv.Itoa(len(it.presentation.Slides)),
				"file_type":        "pptx",
				"presentation_title": it.presentation.Metadata.Title,
				"presentation_author": it.presentation.Metadata.Author,
				"has_animations":   strconv.FormatBool(len(slide.Animations) > 0),
				"shape_count":      strconv.Itoa(len(slide.Shapes)),
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

