package office

import (
	"bytes"
	"context"
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

// PPTReader implements DataSourceReader for legacy Microsoft PowerPoint presentations (.ppt)
type PPTReader struct {
	name    string
	version string
}

// NewPPTReader creates a new PPT file reader
func NewPPTReader() *PPTReader {
	return &PPTReader{
		name:    "ppt_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *PPTReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "extract_slide_content",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract text content from slides",
		},
		{
			Name:        "extract_notes",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract speaker notes from slides",
		},
		{
			Name:        "extract_comments",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract slide comments",
		},
		{
			Name:        "extract_metadata",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract presentation metadata",
		},
		{
			Name:        "include_hidden_slides",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Include hidden slides in extraction",
		},
		{
			Name:        "extract_master_slides",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract content from master slides",
		},
		{
			Name:        "extract_animations",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract animation and transition information",
		},
		{
			Name:        "include_embedded_objects",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract embedded objects and media",
		},
		{
			Name:        "max_slides",
			Type:        "int",
			Required:    false,
			Default:     0,
			Description: "Maximum slides to process (0 = no limit)",
			MinValue:    ptrFloat64(0.0),
			MaxValue:    ptrFloat64(10000.0),
		},
		{
			Name:        "handle_password_protected",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Attempt to handle password-protected presentations",
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *PPTReader) ValidateConfig(config map[string]any) error {
	if maxSlides, ok := config["max_slides"]; ok {
		if num, ok := maxSlides.(float64); ok {
			if num < 0 || num > 10000 {
				return fmt.Errorf("max_slides must be between 0 and 10000")
			}
		}
	}

	return nil
}

// TestConnection tests if the PPT can be read
func (r *PPTReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
		Message: "PPT reader ready",
		Latency: time.Since(start),
		Details: map[string]any{
			"extract_slide_content": config["extract_slide_content"],
			"extract_notes":         config["extract_notes"],
			"include_hidden_slides": config["include_hidden_slides"],
		},
	}
}

// GetType returns the connector type
func (r *PPTReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *PPTReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *PPTReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the PPT file structure
func (r *PPTReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	if !r.isPPTFile(sourcePath) {
		return core.SchemaInfo{}, fmt.Errorf("not a valid PPT file")
	}

	metadata, err := r.extractPPTMetadata(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to extract PPT metadata: %w", err)
	}

	schema := core.SchemaInfo{
		Format:   "ppt",
		Encoding: "binary",
		Fields: []core.FieldInfo{
			{
				Name:        "content",
				Type:        "text",
				Nullable:    false,
				Description: "Slide content text",
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
				Description: "Slide title if present",
			},
			{
				Name:        "content_type",
				Type:        "string",
				Nullable:    false,
				Description: "Type of content (slide, notes, comment)",
			},
			{
				Name:        "layout",
				Type:        "string",
				Nullable:    true,
				Description: "Slide layout type",
			},
			{
				Name:        "speaker_notes",
				Type:        "text",
				Nullable:    true,
				Description: "Speaker notes for the slide",
			},
		},
		Metadata: map[string]any{
			"slide_count":        metadata.SlideCount,
			"title":              metadata.Title,
			"author":             metadata.Author,
			"subject":            metadata.Subject,
			"keywords":           metadata.Keywords,
			"comments":           metadata.Comments,
			"created_date":       metadata.CreatedDate,
			"modified_date":      metadata.ModifiedDate,
			"presentation_mode":  metadata.PresentationMode,
			"has_animations":     metadata.HasAnimations,
			"has_transitions":    metadata.HasTransitions,
			"has_embedded_media": metadata.HasEmbeddedMedia,
			"powerpoint_version": metadata.PowerPointVersion,
			"is_encrypted":       metadata.IsEncrypted,
			"total_words":        metadata.TotalWords,
		},
	}

	// Sample first slide
	if metadata.SlideCount > 0 {
		sampleText, err := r.extractSampleSlide(sourcePath)
		if err == nil {
			schema.SampleData = []map[string]any{
				{
					"content":      sampleText,
					"slide_number": 1,
					"slide_title":  "Sample Slide Title",
					"content_type": "slide",
					"layout":       "Title and Content",
				},
			}
		}
	}

	return schema, nil
}

// EstimateSize returns size estimates for the PPT file
func (r *PPTReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	metadata, err := r.extractPPTMetadata(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to extract PPT metadata: %w", err)
	}

	// Estimate based on slide count
	slideCount := int64(metadata.SlideCount)
	if slideCount == 0 {
		slideCount = 1
	}

	// Estimate chunks based on slides (typically 1 slide per chunk)
	estimatedChunks := int(slideCount)

	// PPT files are complex binary format with embedded media
	complexity := "medium"
	if stat.Size() > 5*1024*1024 || metadata.SlideCount > 50 { // > 5MB or > 50 slides
		complexity = "high"
	}
	if stat.Size() > 50*1024*1024 || metadata.SlideCount > 200 { // > 50MB or > 200 slides
		complexity = "very_high"
	}

	// PPT processing can be slow due to complex format
	processTime := "medium"
	if stat.Size() > 10*1024*1024 || metadata.SlideCount > 100 {
		processTime = "slow"
	}
	if stat.Size() > 100*1024*1024 || metadata.SlideCount > 500 {
		processTime = "very_slow"
	}

	return core.SizeEstimate{
		RowCount:    &slideCount,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the PPT file
func (r *PPTReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	presentation, err := r.parsePresentation(sourcePath, strategyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PPT presentation: %w", err)
	}

	iterator := &PPTIterator{
		sourcePath:   sourcePath,
		config:       strategyConfig,
		presentation: presentation,
		currentSlide: 0,
	}

	return iterator, nil
}

// SupportsStreaming indicates PPT reader supports streaming
func (r *PPTReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *PPTReader) GetSupportedFormats() []string {
	return []string{"ppt"}
}

// PPTMetadata contains extracted PPT metadata
type PPTMetadata struct {
	SlideCount        int
	Title             string
	Author            string
	Subject           string
	Keywords          string
	Comments          string
	CreatedDate       string
	ModifiedDate      string
	PresentationMode  string
	HasAnimations     bool
	HasTransitions    bool
	HasEmbeddedMedia  bool
	PowerPointVersion string
	IsEncrypted       bool
	TotalWords        int
}

// PPTPresentation represents a parsed PPT presentation
type PPTPresentation struct {
	Metadata PPTMetadata
	Slides   []PPTSlide
	Masters  []PPTMasterSlide
}

// PPTSlide represents a presentation slide
type PPTSlide struct {
	Number     int
	Title      string
	Content    string
	Notes      string
	Layout     string
	Hidden     bool
	Shapes     []PPTShape
	Animations []PPTAnimation
	Transition PPTTransition
	Comments   []PPTComment
}

// PPTShape represents a shape on a slide
type PPTShape struct {
	Type     string
	Text     string
	Position PPTPosition
	Size     PPTSize
	Style    PPTShapeStyle
}

// PPTPosition represents shape position
type PPTPosition struct {
	X float64
	Y float64
}

// PPTSize represents shape size
type PPTSize struct {
	Width  float64
	Height float64
}

// PPTShapeStyle represents shape formatting
type PPTShapeStyle struct {
	FontName   string
	FontSize   int
	Bold       bool
	Italic     bool
	Color      string
	Background string
}

// PPTAnimation represents slide animation
type PPTAnimation struct {
	Type     string
	Target   string
	Timing   string
	Duration float64
}

// PPTTransition represents slide transition
type PPTTransition struct {
	Type     string
	Duration float64
	Sound    string
}

// PPTComment represents a slide comment
type PPTComment struct {
	Author   string
	Text     string
	Date     string
	Position PPTPosition
}

// PPTMasterSlide represents a master slide
type PPTMasterSlide struct {
	Type    string
	Content string
	Layouts []string
}

// isPPTFile checks if the file is a valid PPT file
func (r *PPTReader) isPPTFile(filePath string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	// Check OLE signature for PPT files
	signature := make([]byte, 8)
	n, err := file.Read(signature)
	if err != nil || n != 8 {
		return false
	}

	expectedSignature := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	if !bytes.Equal(signature, expectedSignature) {
		return false
	}

	// Additional check for PowerPoint-specific indicators
	// This is simplified - production code would verify PowerPoint document streams
	return true
}

// extractPPTMetadata extracts metadata from PPT file
func (r *PPTReader) extractPPTMetadata(sourcePath string) (PPTMetadata, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		return PPTMetadata{}, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return PPTMetadata{}, err
	}

	// This is a simplified metadata extraction
	// In production, you would parse the OLE compound document structure
	// and extract metadata from the PowerPoint document stream

	metadata := PPTMetadata{
		CreatedDate:       stat.ModTime().Format("2006-01-02 15:04:05"),
		ModifiedDate:      stat.ModTime().Format("2006-01-02 15:04:05"),
		PowerPointVersion: "PowerPoint 97-2003",
		Author:            "Unknown",
		PresentationMode:  "On-screen Show",
	}

	// Estimate presentation size based on file size
	sizeCategory := stat.Size() / (1024) // Size in KB

	switch {
	case sizeCategory < 100: // < 100KB
		metadata.SlideCount = 5
		metadata.TotalWords = 100
	case sizeCategory < 500: // 100-500KB
		metadata.SlideCount = 15
		metadata.TotalWords = 500
	case sizeCategory < 2000: // 500KB-2MB
		metadata.SlideCount = 30
		metadata.TotalWords = 1000
	case sizeCategory < 10000: // 2-10MB
		metadata.SlideCount = 50
		metadata.TotalWords = 2000
		metadata.HasEmbeddedMedia = true
	default: // > 10MB
		metadata.SlideCount = 100
		metadata.TotalWords = 5000
		metadata.HasEmbeddedMedia = true
		metadata.HasAnimations = true
	}

	// Check for common PowerPoint features (simplified heuristics)
	buffer := make([]byte, 2048)
	_, err = file.Read(buffer)
	if err != nil && err != io.EOF {
		return metadata, err
	}

	// Look for animation indicators
	if bytes.Contains(buffer, []byte("Animation")) || bytes.Contains(buffer, []byte("ANIM")) {
		metadata.HasAnimations = true
	}

	// Look for transition indicators
	if bytes.Contains(buffer, []byte("Transition")) || bytes.Contains(buffer, []byte("TRANS")) {
		metadata.HasTransitions = true
	}

	// Set default values
	metadata.Title = filepath.Base(sourcePath)
	metadata.Subject = ""
	metadata.Keywords = ""
	metadata.Comments = ""

	return metadata, nil
}

// extractSampleSlide extracts sample slide content
func (r *PPTReader) extractSampleSlide(sourcePath string) (string, error) {
	// In a real implementation, you would parse the PowerPoint binary format
	// and extract actual slide content
	return "This is sample slide content from the PPT presentation. It would contain the actual extracted text from the first slide.", nil
}

// extractAllTextFromPPT extracts all text content from a PPT binary file
func (r *PPTReader) extractAllTextFromPPT(data []byte) []string {
	var allText []string

	// Extract UTF-16LE strings (common in Office binary formats)
	utf16Strings := r.extractUTF16LEStrings(data)
	allText = append(allText, utf16Strings...)

	// Extract ASCII text
	asciiStrings := r.extractASCIIStrings(data)
	allText = append(allText, asciiStrings...)

	// Filter and deduplicate
	seen := make(map[string]bool)
	var filtered []string
	for _, s := range allText {
		// Clean up the string
		s = strings.TrimSpace(s)
		if s == "" || len(s) < 3 {
			continue
		}
		// Skip if already seen
		if seen[s] {
			continue
		}
		seen[s] = true
		filtered = append(filtered, s)
	}

	return filtered
}

// extractUTF16LEStrings extracts UTF-16LE encoded strings from binary data
func (r *PPTReader) extractUTF16LEStrings(data []byte) []string {
	var results []string

	// Look for UTF-16LE encoded text (common in PPT files)
	for i := 0; i < len(data)-3; i += 2 {
		// Check if this looks like a UTF-16LE string start
		if data[i] >= 0x20 && data[i] < 0x7F && data[i+1] == 0x00 {
			// Try to read a UTF-16LE string
			var utf16Chars []uint16
			for j := i; j < len(data)-1; j += 2 {
				low := data[j]
				high := data[j+1]

				// Check for valid character
				char := uint16(low) | (uint16(high) << 8)
				if char == 0 {
					break
				}
				// Check for printable ASCII in UTF-16 or extended chars
				if char >= 0x20 && char < 0xFFFE {
					utf16Chars = append(utf16Chars, char)
				} else {
					break
				}

				// Stop at reasonable length
				if len(utf16Chars) > 1000 {
					break
				}
			}

			// Convert to string if we have enough characters
			if len(utf16Chars) >= 3 {
				str := string(utf16.Decode(utf16Chars))
				str = strings.TrimSpace(str)
				if len(str) >= 3 && r.isReasonableText(str) {
					results = append(results, str)
					i += len(utf16Chars) * 2 // Skip past this string
				}
			}
		}
	}

	return results
}

// extractASCIIStrings extracts ASCII text from binary data
func (r *PPTReader) extractASCIIStrings(data []byte) []string {
	var results []string
	var currentString []byte

	for _, b := range data {
		if b >= 0x20 && b < 0x7F {
			currentString = append(currentString, b)
		} else {
			if len(currentString) >= 4 {
				s := string(currentString)
				if r.isReasonableText(s) {
					results = append(results, s)
				}
			}
			currentString = currentString[:0]
		}
	}

	// Don't forget the last string
	if len(currentString) >= 4 {
		s := string(currentString)
		if r.isReasonableText(s) {
			results = append(results, s)
		}
	}

	return results
}

// isReasonableText checks if a string appears to be actual text content
func (r *PPTReader) isReasonableText(s string) bool {
	if len(s) < 3 {
		return false
	}

	// Count different character types
	letters := 0
	numbers := 0
	spaces := 0
	punctuation := 0

	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			letters++
		case c >= '0' && c <= '9':
			numbers++
		case c == ' ':
			spaces++
		case c == '.' || c == ',' || c == '!' || c == '?' || c == ':' || c == ';' || c == '-':
			punctuation++
		}
	}

	total := len(s)

	// Skip binary noise patterns
	if letters < total/4 && spaces == 0 {
		return false
	}

	// Skip strings that are mostly numbers or special chars
	if numbers > total/2 && letters < total/4 {
		return false
	}

	// Skip very short strings without spaces (likely not real text)
	if len(s) < 10 && spaces == 0 {
		return false
	}

	// Skip PowerPoint internal markers
	lowerS := strings.ToLower(s)
	internalPatterns := []string{
		"powerpoint", "microsoft", "document", "arial", "times new roman",
		"calibri", "theme", "slide layout", "placeholder",
		"_hlk", "_ref", "normal", "default", "scheme", "txstyles",
		"ppt/", "docprops", "xmlns", "xmls", "rels", "contenttype",
		"fonts used", "slide titles", "embedded ole", "ole object",
		"root entry", "summaryinformation", "documentsummaryinformation",
		"current user", "pictures", "downrev", "shapexml",
	}
	for _, pattern := range internalPatterns {
		if strings.Contains(lowerS, pattern) {
			return false
		}
	}

	// Skip patterns like "Shape 18", "Slide 10", "Text 1", "Title 1"
	// These are internal PowerPoint element names
	shapePattern := regexp.MustCompile(`^(Shape|Slide|Text|Title|Content|Subtitle|Footer|Header|Date|Picture|Table|Chart|Diagram|Media|Object|Group|Freeform|Rectangle|Oval|Line|Arrow|Callout|Action|Placeholder)\s*\d*$`)
	if shapePattern.MatchString(s) {
		return false
	}

	// Skip patterns like "On-screen Show (16:9)" - presentation settings
	if strings.Contains(lowerS, "on-screen show") || strings.Contains(lowerS, "widescreen") {
		return false
	}

	return true
}

// organizeTextIntoSlides attempts to organize extracted text into slides
func (r *PPTReader) organizeTextIntoSlides(texts []string) []PPTSlide {
	if len(texts) == 0 {
		return nil
	}

	var slides []PPTSlide

	// Group text into slides (heuristic: new slide when we see title-like text)
	var currentSlideTexts []string
	slideNumber := 1

	// Pattern for likely slide titles (short, often capitalized)
	titlePattern := regexp.MustCompile(`^[A-Z][^.!?]*$`)

	for _, text := range texts {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}

		// Check if this looks like a new slide title
		isTitle := len(text) < 100 && titlePattern.MatchString(text)

		// Start a new slide if we have a title-like text and existing content
		if isTitle && len(currentSlideTexts) > 0 {
			// Save current slide
			slide := r.createSlideFromTexts(slideNumber, currentSlideTexts)
			slides = append(slides, slide)
			slideNumber++
			currentSlideTexts = nil
		}

		currentSlideTexts = append(currentSlideTexts, text)
	}

	// Don't forget the last slide
	if len(currentSlideTexts) > 0 {
		slide := r.createSlideFromTexts(slideNumber, currentSlideTexts)
		slides = append(slides, slide)
	}

	// If we ended up with no slides or just one massive slide, split by text count
	if len(slides) <= 1 && len(texts) > 10 {
		return r.splitIntoSlidesByTextCount(texts, 5)
	}

	return slides
}

// splitIntoSlidesByTextCount splits texts into slides by grouping every N texts
func (r *PPTReader) splitIntoSlidesByTextCount(texts []string, textsPerSlide int) []PPTSlide {
	var slides []PPTSlide
	slideNumber := 1

	for i := 0; i < len(texts); i += textsPerSlide {
		end := i + textsPerSlide
		if end > len(texts) {
			end = len(texts)
		}
		slideTexts := texts[i:end]
		slide := r.createSlideFromTexts(slideNumber, slideTexts)
		slides = append(slides, slide)
		slideNumber++
	}

	return slides
}

// createSlideFromTexts creates a slide from a list of text strings
func (r *PPTReader) createSlideFromTexts(slideNumber int, texts []string) PPTSlide {
	var title string
	var contentParts []string

	for i, text := range texts {
		if i == 0 && len(text) < 100 {
			// First short text is likely the title
			title = text
		} else {
			contentParts = append(contentParts, text)
		}
	}

	content := strings.Join(contentParts, "\n\n")

	return PPTSlide{
		Number:  slideNumber,
		Title:   title,
		Content: content,
		Layout:  "Title and Content",
		Hidden:  false,
		Shapes: []PPTShape{
			{
				Type: "title",
				Text: title,
			},
			{
				Type: "content",
				Text: content,
			},
		},
	}
}

// parsePresentation parses the complete PPT presentation
func (r *PPTReader) parsePresentation(sourcePath string, config map[string]any) (*PPTPresentation, error) {
	metadata, err := r.extractPPTMetadata(sourcePath)
	if err != nil {
		return nil, err
	}

	// Read the entire file
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read PPT file: %w", err)
	}

	// Extract all text from the binary PPT file
	texts := r.extractAllTextFromPPT(data)

	// Organize text into slides
	slides := r.organizeTextIntoSlides(texts)

	// Update metadata with actual slide count
	if len(slides) > 0 {
		metadata.SlideCount = len(slides)

		// Count total words
		totalWords := 0
		for _, slide := range slides {
			totalWords += len(strings.Fields(slide.Title))
			totalWords += len(strings.Fields(slide.Content))
		}
		metadata.TotalWords = totalWords
	}

	// If no slides were extracted, create a single slide with all extracted text
	if len(slides) == 0 && len(texts) > 0 {
		allContent := strings.Join(texts, "\n\n")
		slides = []PPTSlide{
			{
				Number:  1,
				Title:   "Extracted Content",
				Content: allContent,
				Layout:  "Content",
				Shapes: []PPTShape{
					{
						Type: "content",
						Text: allContent,
					},
				},
			},
		}
		metadata.SlideCount = 1
		metadata.TotalWords = len(strings.Fields(allContent))
	}

	// If still no content, return an empty presentation
	if len(slides) == 0 {
		slides = []PPTSlide{
			{
				Number:  1,
				Title:   "No Content Extracted",
				Content: "Unable to extract text content from this PPT file. The file may be encrypted or use an unsupported format.",
				Layout:  "Content",
			},
		}
		metadata.SlideCount = 1
	}

	return &PPTPresentation{
		Metadata: metadata,
		Slides:   slides,
		Masters:  []PPTMasterSlide{},
	}, nil
}

// PPTIterator implements ChunkIterator for PPT files
type PPTIterator struct {
	sourcePath   string
	config       map[string]any
	presentation *PPTPresentation
	currentSlide int
}

// Next returns the next chunk of content from the PPT
func (it *PPTIterator) Next(ctx context.Context) (core.Chunk, error) {
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

	// Skip hidden slides unless configured to include them
	if slide.Hidden {
		if includeHidden, ok := it.config["include_hidden_slides"]; !ok || !includeHidden.(bool) {
			return it.Next(ctx) // Recursively get next slide
		}
	}

	// Build slide content
	var contentParts []string

	if slide.Title != "" {
		contentParts = append(contentParts, fmt.Sprintf("Title: %s", slide.Title))
	}

	if slide.Content != "" {
		contentParts = append(contentParts, fmt.Sprintf("Content: %s", slide.Content))
	}

	// Include speaker notes if configured
	if extractNotes, ok := it.config["extract_notes"]; (!ok || extractNotes.(bool)) && slide.Notes != "" {
		contentParts = append(contentParts, fmt.Sprintf("Speaker Notes: %s", slide.Notes))
	}

	content := strings.Join(contentParts, "\n\n")

	chunk := core.Chunk{
		Data: content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:slide:%d", filepath.Base(it.sourcePath), slide.Number),
			ChunkType:   "ppt_slide",
			SizeBytes:   int64(len(content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "ppt_reader",
			Context: map[string]string{
				"slide_number":        strconv.Itoa(slide.Number),
				"slide_title":         slide.Title,
				"layout":              slide.Layout,
				"total_slides":        strconv.Itoa(len(it.presentation.Slides)),
				"file_type":           "ppt",
				"presentation_title":  it.presentation.Metadata.Title,
				"presentation_author": it.presentation.Metadata.Author,
				"shape_count":         strconv.Itoa(len(slide.Shapes)),
				"has_animations":      strconv.FormatBool(len(slide.Animations) > 0),
				"has_notes":           strconv.FormatBool(slide.Notes != ""),
			},
		},
	}

	return chunk, nil
}

// Close releases PPT resources
func (it *PPTIterator) Close() error {
	// Nothing to close for PPT iterator
	return nil
}

// Reset restarts iteration from the beginning
func (it *PPTIterator) Reset() error {
	it.currentSlide = 0
	return nil
}

// Progress returns iteration progress
func (it *PPTIterator) Progress() float64 {
	totalSlides := len(it.presentation.Slides)
	if totalSlides == 0 {
		return 1.0
	}
	return float64(it.currentSlide) / float64(totalSlides)
}
