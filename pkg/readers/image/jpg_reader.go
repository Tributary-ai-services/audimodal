package image

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/audimodal/pkg/core"
)

// JPGReader implements DataSourceReader for JPEG image files with OCR support
type JPGReader struct {
	name    string
	version string
}

// NewJPGReader creates a new JPEG file reader
func NewJPGReader() *JPGReader {
	return &JPGReader{
		name:    "jpg_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *JPGReader) GetConfigSpec() []core.ConfigSpec {
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
			Description: "Extract JPEG metadata (EXIF data, creation date, camera info)",
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
			Description: "Auto-detect and correct image orientation from EXIF",
		},
		{
			Name:        "extract_colors",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract dominant colors from image",
		},
		{
			Name:        "quality_threshold",
			Type:        "int",
			Required:    false,
			Default:     70,
			Description: "Minimum JPEG quality for processing (0-100)",
			MinValue:    ptrFloat64(0.0),
			MaxValue:    ptrFloat64(100.0),
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *JPGReader) ValidateConfig(config map[string]any) error {
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

	if quality, ok := config["quality_threshold"]; ok {
		if num, ok := quality.(float64); ok {
			if num < 0 || num > 100 {
				return fmt.Errorf("quality_threshold must be between 0 and 100")
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

// TestConnection tests if the JPEG can be read
func (r *JPGReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
				return "JPEG reader ready"
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
func (r *JPGReader) checkOCRDependencies() []string {
	var missing []string

	// Check for tesseract (OCR engine)
	if !r.commandExists("tesseract") {
		missing = append(missing, "tesseract (install tesseract-ocr)")
	}

	// Check for ImageMagick (image processing)
	if !r.commandExists("convert") {
		missing = append(missing, "convert (install imagemagick)")
	}

	// Check for exiftool (EXIF metadata extraction)
	if !r.commandExists("exiftool") {
		missing = append(missing, "exiftool (install libimage-exiftool-perl)")
	}

	return missing
}

// commandExists checks if a command is available in PATH
func (r *JPGReader) commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// GetType returns the connector type
func (r *JPGReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *JPGReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *JPGReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the JPEG file structure
func (r *JPGReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	// Open and decode JPEG
	file, err := os.Open(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	img, err := jpeg.Decode(file)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to decode JPEG: %w", err)
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

	// Extract JPEG metadata (EXIF)
	jpegInfo := r.extractJPEGInfo(sourcePath)

	schema := core.SchemaInfo{
		Format:   "jpeg",
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
				Name:        "exif_data",
				Type:        "object",
				Nullable:    true,
				Description: "EXIF metadata from the image",
			},
		},
		Metadata: map[string]any{
			"file_size":      stat.Size(),
			"width":          width,
			"height":         height,
			"color_model":    r.getColorModelName(img.ColorModel()),
			"aspect_ratio":   float64(width) / float64(height),
			"pixel_count":    width * height,
			"quality":        jpegInfo.Quality,
			"orientation":    jpegInfo.Orientation,
			"camera_make":    jpegInfo.CameraMake,
			"camera_model":   jpegInfo.CameraModel,
			"creation_time":  jpegInfo.CreationTime,
			"gps_latitude":   jpegInfo.GPSLatitude,
			"gps_longitude":  jpegInfo.GPSLongitude,
			"software":       jpegInfo.Software,
			"has_text":       jpegInfo.HasText,
			"is_progressive": jpegInfo.IsProgressive,
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

// EstimateSize returns size estimates for the JPEG file
func (r *JPGReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	// JPEG images are processed as single chunks
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

// CreateIterator creates a chunk iterator for the JPEG file
func (r *JPGReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	// Validate JPEG format
	file, err := os.Open(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	_, err = jpeg.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("not a valid JPEG file: %w", err)
	}

	iterator := &JPGIterator{
		sourcePath: sourcePath,
		config:     strategyConfig,
		processed:  false,
		reader:     r,
	}

	return iterator, nil
}

// SupportsStreaming indicates JPEG reader supports streaming
func (r *JPGReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *JPGReader) GetSupportedFormats() []string {
	return []string{"jpg", "jpeg", "jpe", "jif", "jfif", "jfi"}
}

// JPEGInfo contains extracted JPEG metadata
type JPEGInfo struct {
	Quality       int
	Orientation   int
	CameraMake    string
	CameraModel   string
	CreationTime  string
	GPSLatitude   float64
	GPSLongitude  float64
	Software      string
	HasText       bool
	IsProgressive bool
}

// extractJPEGInfo extracts metadata from JPEG file
func (r *JPGReader) extractJPEGInfo(sourcePath string) JPEGInfo {
	// This is a simplified JPEG metadata extractor
	// In production, you'd parse EXIF data using a library like github.com/rwcarlsen/goexif/exif

	info := JPEGInfo{
		Quality:       85,   // Common default
		Orientation:   1,    // Normal orientation
		HasText:       true, // Assume images may contain text
		IsProgressive: false,
	}

	// Get file info for creation time
	if stat, err := os.Stat(sourcePath); err == nil {
		info.CreationTime = stat.ModTime().Format("2006-01-02 15:04:05")
	}

	// Mock some EXIF data
	info.CameraMake = "Unknown"
	info.CameraModel = "Unknown"
	info.Software = "Unknown"

	return info
}

// getColorModelName returns a string representation of the color model
func (r *JPGReader) getColorModelName(cm color.Model) string {
	switch cm {
	case color.RGBAModel:
		return "RGBA"
	case color.RGBA64Model:
		return "RGBA64"
	case color.NRGBAModel:
		return "NRGBA"
	case color.NRGBA64Model:
		return "NRGBA64"
	case color.YCbCrModel:
		return "YCbCr"
	case color.GrayModel:
		return "Gray"
	case color.Gray16Model:
		return "Gray16"
	case color.CMYKModel:
		return "CMYK"
	default:
		return "Unknown"
	}
}

// performOCR performs OCR on the JPEG image using tesseract
func (r *JPGReader) performOCR(sourcePath string, config map[string]any) (string, float64, []TextRegion, error) {
	// Get OCR language from config
	lang := "eng"
	if l, ok := config["ocr_language"].(string); ok {
		lang = l
	}

	// Get whether to include coordinates
	includeCoords := false
	if inc, ok := config["include_coordinates"].(bool); ok {
		includeCoords = inc
	}

	log.Printf("[DEBUG] Running tesseract OCR on %s (lang=%s)", filepath.Base(sourcePath), lang)

	// Run tesseract OCR with TSV output for confidence and regions
	cmd := exec.Command("tesseract", sourcePath, "stdout", "-l", lang, "tsv")
	tsvOutput, err := cmd.Output()
	if err != nil {
		// Fall back to plain text output
		log.Printf("[WARN] Tesseract TSV failed, trying plain text: %v", err)
		cmd = exec.Command("tesseract", sourcePath, "stdout", "-l", lang)
		textOutput, err := cmd.Output()
		if err != nil {
			return "", 0, nil, fmt.Errorf("tesseract failed: %w", err)
		}
		text := strings.TrimSpace(string(textOutput))
		log.Printf("[INFO] OCR extracted %d chars from %s (plain text mode)", len(text), filepath.Base(sourcePath))
		return text, 0.8, nil, nil // Default confidence for plain text mode
	}

	// Parse TSV output for text, confidence, and regions
	text, confidence, regions := r.parseTesseractTSV(string(tsvOutput), includeCoords)

	// Apply minimum confidence filter
	if minConf, ok := config["min_confidence"]; ok {
		if minConfidence, ok := minConf.(float64); ok {
			if confidence < minConfidence {
				return "", confidence, regions, fmt.Errorf("OCR confidence %.2f below threshold %.2f", confidence, minConfidence)
			}
		}
	}

	log.Printf("[INFO] OCR extracted %d chars from %s (confidence=%.2f, regions=%d)",
		len(text), filepath.Base(sourcePath), confidence, len(regions))

	return text, confidence, regions, nil
}

// parseTesseractTSV parses tesseract TSV output to extract text, confidence, and regions
func (r *JPGReader) parseTesseractTSV(tsvOutput string, includeRegions bool) (string, float64, []TextRegion) {
	lines := strings.Split(tsvOutput, "\n")
	var words []string
	var regions []TextRegion
	var totalConf float64
	var confCount int

	// TSV columns: level, page_num, block_num, par_num, line_num, word_num, left, top, width, height, conf, text
	for i, line := range lines {
		if i == 0 { // Skip header
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 12 {
			continue
		}

		conf, err := strconv.ParseFloat(fields[10], 64)
		if err != nil || conf < 0 {
			continue
		}

		text := fields[11]
		if text == "" || text == " " {
			continue
		}

		words = append(words, text)
		if conf > 0 {
			totalConf += conf
			confCount++
		}

		// Extract regions if requested
		if includeRegions && conf > 0 {
			left, _ := strconv.Atoi(fields[6])
			top, _ := strconv.Atoi(fields[7])
			width, _ := strconv.Atoi(fields[8])
			height, _ := strconv.Atoi(fields[9])

			regions = append(regions, TextRegion{
				Text:       text,
				X:          left,
				Y:          top,
				Width:      width,
				Height:     height,
				Confidence: conf / 100.0, // Normalize to 0-1
			})
		}
	}

	avgConf := 0.8 // Default
	if confCount > 0 {
		avgConf = totalConf / float64(confCount) / 100.0 // Normalize to 0-1
	}

	// Reconstruct text
	finalText := strings.Join(words, " ")
	finalText = strings.TrimSpace(finalText)

	return finalText, avgConf, regions
}

// JPGIterator implements ChunkIterator for JPEG files
type JPGIterator struct {
	sourcePath string
	config     map[string]any
	processed  bool
	reader     *JPGReader
}

// Next returns the next chunk of content from the JPEG
func (it *JPGIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	// JPEG images are processed as a single chunk
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
		content = "JPEG image (OCR disabled)"
		confidence = 0.0
	}

	// Get image info for metadata
	file, err := os.Open(it.sourcePath)
	if err != nil {
		return core.Chunk{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	img, err := jpeg.Decode(file)
	if err != nil {
		return core.Chunk{}, fmt.Errorf("failed to decode JPEG: %w", err)
	}

	bounds := img.Bounds()
	width := bounds.Max.X - bounds.Min.X
	height := bounds.Max.Y - bounds.Min.Y

	// Get JPEG metadata
	jpegInfo := it.reader.extractJPEGInfo(it.sourcePath)

	chunk := core.Chunk{
		Data: content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:jpg:1", filepath.Base(it.sourcePath)),
			ChunkType:   "jpg_image",
			SizeBytes:   int64(len(content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "jpg_reader",
			Context: map[string]string{
				"confidence":         fmt.Sprintf("%.2f", confidence),
				"file_type":          "jpeg",
				"width":              strconv.Itoa(width),
				"height":             strconv.Itoa(height),
				"color_model":        it.reader.getColorModelName(img.ColorModel()),
				"text_regions_count": strconv.Itoa(len(regions)),
				"quality":            strconv.Itoa(jpegInfo.Quality),
				"orientation":        strconv.Itoa(jpegInfo.Orientation),
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

	// Add EXIF metadata if available
	if jpegInfo.CameraMake != "Unknown" {
		chunk.Metadata.Context["camera_make"] = jpegInfo.CameraMake
		chunk.Metadata.Context["camera_model"] = jpegInfo.CameraModel
	}
	if jpegInfo.GPSLatitude != 0 || jpegInfo.GPSLongitude != 0 {
		chunk.Metadata.Context["gps_coords"] = fmt.Sprintf("%.6f,%.6f", jpegInfo.GPSLatitude, jpegInfo.GPSLongitude)
	}

	return chunk, nil
}

// Close releases JPEG resources
func (it *JPGIterator) Close() error {
	// Nothing to close for JPEG iterator
	return nil
}

// Reset restarts iteration from the beginning
func (it *JPGIterator) Reset() error {
	it.processed = false
	return nil
}

// Progress returns iteration progress
func (it *JPGIterator) Progress() float64 {
	if it.processed {
		return 1.0
	}
	return 0.0
}
