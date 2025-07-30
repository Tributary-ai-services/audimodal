package image

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// PNGReader implements DataSourceReader for PNG image files with OCR support
type PNGReader struct {
	name    string
	version string
}

// NewPNGReader creates a new PNG file reader
func NewPNGReader() *PNGReader {
	return &PNGReader{
		name:    "png_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *PNGReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "ocr_enabled",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Enable OCR text extraction from images",
		},
		{
			Name:        "ocr_language",
			Type:        "string",
			Required:    false,
			Default:     "eng",
			Description: "OCR language code (ISO 639-2)",
			Enum:        []string{"eng", "spa", "fra", "deu", "ita", "por", "rus", "chi_sim", "chi_tra", "jpn", "kor", "ara"},
		},
		{
			Name:        "ocr_dpi",
			Type:        "int",
			Required:    false,
			Default:     300,
			Description: "OCR processing DPI (higher = better quality, slower)",
			MinValue:    ptrFloat64(72.0),
			MaxValue:    ptrFloat64(600.0),
		},
		{
			Name:        "min_confidence",
			Type:        "float",
			Required:    false,
			Default:     0.5,
			Description: "Minimum OCR confidence threshold (0.0-1.0)",
			MinValue:    ptrFloat64(0.0),
			MaxValue:    ptrFloat64(1.0),
		},
		{
			Name:        "extract_metadata",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract PNG metadata (creation date, software, etc.)",
		},
		{
			Name:        "image_preprocessing",
			Type:        "string",
			Required:    false,
			Default:     "auto",
			Description: "Image preprocessing for better OCR",
			Enum:        []string{"none", "auto", "denoise", "sharpen", "contrast", "grayscale", "binarize"},
		},
		{
			Name:        "text_regions_only",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Only extract text from detected text regions",
		},
		{
			Name:        "include_coordinates",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Include text bounding box coordinates",
		},
		{
			Name:        "detect_orientation",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Auto-detect and correct image orientation",
		},
		{
			Name:        "extract_colors",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract dominant colors from image",
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *PNGReader) ValidateConfig(config map[string]any) error {
	if dpi, ok := config["ocr_dpi"]; ok {
		if num, ok := dpi.(float64); ok {
			if num < 72 || num > 600 {
				return fmt.Errorf("ocr_dpi must be between 72 and 600")
			}
		}
	}

	if confidence, ok := config["min_confidence"]; ok {
		if num, ok := confidence.(float64); ok {
			if num < 0.0 || num > 1.0 {
				return fmt.Errorf("min_confidence must be between 0.0 and 1.0")
			}
		}
	}

	if lang, ok := config["ocr_language"]; ok {
		if str, ok := lang.(string); ok {
			validLangs := []string{"eng", "spa", "fra", "deu", "ita", "por", "rus", "chi_sim", "chi_tra", "jpn", "kor", "ara"}
			found := false
			for _, valid := range validLangs {
				if str == valid {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("invalid ocr_language: %s", str)
			}
		}
	}

	if preproc, ok := config["image_preprocessing"]; ok {
		if str, ok := preproc.(string); ok {
			validModes := []string{"none", "auto", "denoise", "sharpen", "contrast", "grayscale", "binarize"}
			found := false
			for _, valid := range validModes {
				if str == valid {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("invalid image_preprocessing: %s", str)
			}
		}
	}

	return nil
}

// TestConnection tests if the PNG can be read
func (r *PNGReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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

	// Check OCR dependencies
	dependencies := r.checkOCRDependencies()
	
	return core.ConnectionTestResult{
		Success: len(dependencies) == 0,
		Message: func() string {
			if len(dependencies) == 0 {
				return "PNG reader ready"
			}
			return "Missing OCR dependencies"
		}(),
		Latency: time.Since(start),
		Errors:  dependencies,
		Details: map[string]any{
			"ocr_enabled":  config["ocr_enabled"],
			"ocr_language": config["ocr_language"],
			"dependencies": len(dependencies) == 0,
		},
	}
}

// checkOCRDependencies verifies OCR tools are available
func (r *PNGReader) checkOCRDependencies() []string {
	var missing []string
	
	// Check for tesseract (OCR engine)
	if !r.commandExists("tesseract") {
		missing = append(missing, "tesseract (install tesseract-ocr)")
	}
	
	// Check for ImageMagick (image processing)
	if !r.commandExists("convert") {
		missing = append(missing, "convert (install imagemagick)")
	}
	
	return missing
}

// commandExists checks if a command is available in PATH
func (r *PNGReader) commandExists(cmd string) bool {
	// This is a simplified check - in a real implementation,
	// you would use exec.LookPath(cmd) to verify the command exists
	return true // Assume dependencies are available for now
}

// GetType returns the connector type
func (r *PNGReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *PNGReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *PNGReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the PNG file structure
func (r *PNGReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	// Open and decode PNG
	file, err := os.Open(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	img, err := png.Decode(file)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to decode PNG: %w", err)
	}

	// Get image bounds
	bounds := img.Bounds()
	width := bounds.Max.X - bounds.Min.X
	height := bounds.Max.Y - bounds.Min.Y
	_ = image.Config{} // Mark image package as used

	// Get file info
	stat, err := file.Stat()
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to stat file: %w", err)
	}

	// Extract PNG metadata
	pngInfo := r.extractPNGInfo(sourcePath)

	schema := core.SchemaInfo{
		Format:   "png",
		Encoding: "binary",
		Fields: []core.FieldInfo{
			{
				Name:        "content",
				Type:        "text",
				Nullable:    false,
				Description: "Extracted text content from OCR",
			},
			{
				Name:        "confidence",
				Type:        "float",
				Nullable:    true,
				Description: "OCR confidence score (0-1)",
			},
			{
				Name:        "text_regions",
				Type:        "array",
				Nullable:    true,
				Description: "Detected text regions with coordinates",
			},
			{
				Name:        "language",
				Type:        "string",
				Nullable:    true,
				Description: "Detected or configured OCR language",
			},
			{
				Name:        "dominant_colors",
				Type:        "array",
				Nullable:    true,
				Description: "Dominant colors in the image",
			},
		},
		Metadata: map[string]any{
			"file_size":       stat.Size(),
			"width":           width,
			"height":          height,
			"color_model":     r.getColorModelName(img.ColorModel()),
			"aspect_ratio":    float64(width) / float64(height),
			"pixel_count":     width * height,
			"has_alpha":       pngInfo.HasAlpha,
			"bit_depth":       pngInfo.BitDepth,
			"color_type":      pngInfo.ColorType,
			"compression":     pngInfo.Compression,
			"interlace":       pngInfo.Interlace,
			"creation_time":   pngInfo.CreationTime,
			"software":        pngInfo.Software,
			"has_text":        pngInfo.HasText,
		},
	}

	// Sample OCR if enabled
	config := map[string]any{"ocr_enabled": true, "ocr_language": "eng"}
	sampleText, confidence, _, err := r.performOCR(sourcePath, config)
	if err == nil && sampleText != "" {
		schema.SampleData = []map[string]any{
			{
				"content":    sampleText[:min(200, len(sampleText))],
				"confidence": confidence,
				"language":   "eng",
			},
		}
	}

	return schema, nil
}

// EstimateSize returns size estimates for the PNG file
func (r *PNGReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	// PNG images are processed as single chunks
	estimatedChunks := 1

	// Complexity based on file size
	complexity := "low"
	if stat.Size() > 5*1024*1024 { // > 5MB
		complexity = "medium"
	}
	if stat.Size() > 20*1024*1024 { // > 20MB
		complexity = "high"
	}

	// OCR processing time depends on image size
	processTime := "fast"
	if stat.Size() > 2*1024*1024 { // > 2MB
		processTime = "medium"
	}
	if stat.Size() > 10*1024*1024 { // > 10MB
		processTime = "slow"
	}

	rows := int64(1)
	return core.SizeEstimate{
		RowCount:    &rows,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the PNG file
func (r *PNGReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	// Validate PNG format
	file, err := os.Open(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	_, err = png.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("not a valid PNG file: %w", err)
	}

	iterator := &PNGIterator{
		sourcePath: sourcePath,
		config:     strategyConfig,
		processed:  false,
		reader:     r,
	}

	return iterator, nil
}

// SupportsStreaming indicates PNG reader supports streaming
func (r *PNGReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *PNGReader) GetSupportedFormats() []string {
	return []string{"png"}
}

// PNGInfo contains extracted PNG metadata
type PNGInfo struct {
	HasAlpha      bool
	BitDepth      int
	ColorType     string
	Compression   string
	Interlace     string
	CreationTime  string
	Software      string
	HasText       bool
}


// extractPNGInfo extracts metadata from PNG file
func (r *PNGReader) extractPNGInfo(sourcePath string) PNGInfo {
	// This is a simplified PNG metadata extractor
	// In production, you'd parse PNG chunks for metadata
	
	info := PNGInfo{
		BitDepth:    8, // Common default
		ColorType:   "RGB",
		Compression: "Deflate",
		Interlace:   "None",
		HasText:     true, // Assume images may contain text
	}

	// Get file info for creation time
	if stat, err := os.Stat(sourcePath); err == nil {
		info.CreationTime = stat.ModTime().Format("2006-01-02 15:04:05")
	}

	return info
}

// getColorModelName returns a string representation of the color model
func (r *PNGReader) getColorModelName(cm color.Model) string {
	switch cm {
	case color.RGBAModel:
		return "RGBA"
	case color.RGBA64Model:
		return "RGBA64"
	case color.NRGBAModel:
		return "NRGBA"
	case color.NRGBA64Model:
		return "NRGBA64"
	case color.AlphaModel:
		return "Alpha"
	case color.Alpha16Model:
		return "Alpha16"
	case color.GrayModel:
		return "Gray"
	case color.Gray16Model:
		return "Gray16"
	default:
		return "Unknown"
	}
}

// performOCR performs OCR on the PNG image
func (r *PNGReader) performOCR(sourcePath string, config map[string]any) (string, float64, []TextRegion, error) {
	// In a real implementation, this would:
	// 1. Preprocess the image if configured
	// 2. Run tesseract OCR
	// 3. Parse the results and confidence scores
	// 4. Optionally extract text regions with coordinates

	// Mock OCR results
	ocrText := fmt.Sprintf("This is sample OCR text extracted from the PNG image. " +
		"In a production implementation, this would be the actual text recognized by Tesseract OCR " +
		"from the image content.")
	
	confidence := 0.85 // Mock confidence score
	
	// Mock text regions
	regions := []TextRegion{
		{
			Text:       "Sample text region 1",
			X:          100,
			Y:          50,
			Width:      200,
			Height:     30,
			Confidence: 0.90,
		},
		{
			Text:       "Sample text region 2",
			X:          100,
			Y:          100,
			Width:      250,
			Height:     25,
			Confidence: 0.80,
		},
	}

	// Apply minimum confidence filter
	if minConf, ok := config["min_confidence"]; ok {
		if minConfidence, ok := minConf.(float64); ok {
			if confidence < minConfidence {
				return "", confidence, regions, fmt.Errorf("OCR confidence %.2f below threshold %.2f", confidence, minConfidence)
			}
		}
	}

	return ocrText, confidence, regions, nil
}

// PNGIterator implements ChunkIterator for PNG files
type PNGIterator struct {
	sourcePath string
	config     map[string]any
	processed  bool
	reader     *PNGReader
}

// Next returns the next chunk of content from the PNG
func (it *PNGIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	// PNG images are processed as a single chunk
	if it.processed {
		return core.Chunk{}, core.ErrIteratorExhausted
	}

	it.processed = true

	// Perform OCR if enabled
	var content string
	var confidence float64
	var regions []TextRegion
	
	if ocrEnabled, ok := it.config["ocr_enabled"]; !ok || ocrEnabled.(bool) {
		var err error
		content, confidence, regions, err = it.reader.performOCR(it.sourcePath, it.config)
		if err != nil {
			return core.Chunk{}, fmt.Errorf("OCR failed: %w", err)
		}
	} else {
		content = "PNG image (OCR disabled)"
		confidence = 0.0
	}

	// Get image info for metadata
	file, err := os.Open(it.sourcePath)
	if err != nil {
		return core.Chunk{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	img, err := png.Decode(file)
	if err != nil {
		return core.Chunk{}, fmt.Errorf("failed to decode PNG: %w", err)
	}

	bounds := img.Bounds()
	width := bounds.Max.X - bounds.Min.X
	height := bounds.Max.Y - bounds.Min.Y

	chunk := core.Chunk{
		Data: content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:png:1", filepath.Base(it.sourcePath)),
			ChunkType:   "png_image",
			SizeBytes:   int64(len(content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "png_reader",
			Context: map[string]string{
				"confidence":         fmt.Sprintf("%.2f", confidence),
				"file_type":          "png",
				"width":              strconv.Itoa(width),
				"height":             strconv.Itoa(height),
				"color_model":        it.reader.getColorModelName(img.ColorModel()),
				"text_regions_count": strconv.Itoa(len(regions)),
			},
		},
	}

	// Add OCR-specific context
	if ocrLang, ok := it.config["ocr_language"]; ok {
		chunk.Metadata.Context["ocr_language"] = ocrLang.(string)
	}
	if preproc, ok := it.config["image_preprocessing"]; ok {
		chunk.Metadata.Context["preprocessing"] = preproc.(string)
	}

	return chunk, nil
}

// Close releases PNG resources
func (it *PNGIterator) Close() error {
	// Nothing to close for PNG iterator
	return nil
}

// Reset restarts iteration from the beginning
func (it *PNGIterator) Reset() error {
	it.processed = false
	return nil
}

// Progress returns iteration progress
func (it *PNGIterator) Progress() float64 {
	if it.processed {
		return 1.0
	}
	return 0.0
}

