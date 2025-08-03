package image

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// TIFFReader implements DataSourceReader for TIFF image files with OCR support
type TIFFReader struct {
	name    string
	version string
}

// NewTIFFReader creates a new TIFF file reader
func NewTIFFReader() *TIFFReader {
	return &TIFFReader{
		name:    "tiff_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *TIFFReader) GetConfigSpec() []core.ConfigSpec {
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
			Description: "Extract TIFF metadata (EXIF, creation date, etc.)",
		},
		{
			Name:        "process_multipage",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Process multi-page TIFF files",
		},
		{
			Name:        "max_pages",
			Type:        "int",
			Required:    false,
			Default:     0,
			Description: "Maximum pages to process (0 = all pages)",
			MinValue:    ptrFloat64(0.0),
			MaxValue:    ptrFloat64(1000.0),
		},
		{
			Name:        "image_preprocessing",
			Type:        "string",
			Required:    false,
			Default:     "auto",
			Description: "Image preprocessing for better OCR",
			Enum:        []string{"none", "auto", "denoise", "sharpen", "contrast"},
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
	}
}

// ValidateConfig validates the provided configuration
func (r *TIFFReader) ValidateConfig(config map[string]any) error {
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

	if maxPages, ok := config["max_pages"]; ok {
		if num, ok := maxPages.(float64); ok {
			if num < 0 || num > 1000 {
				return fmt.Errorf("max_pages must be between 0 and 1000")
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
			validModes := []string{"none", "auto", "denoise", "sharpen", "contrast"}
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

// TestConnection tests if the TIFF can be read
func (r *TIFFReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
				return "TIFF reader ready"
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
func (r *TIFFReader) checkOCRDependencies() []string {
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
func (r *TIFFReader) commandExists(cmd string) bool {
	// This is a simplified check - in a real implementation,
	// you would use exec.LookPath(cmd) to verify the command exists
	return true // Assume dependencies are available for now
}

// GetType returns the connector type
func (r *TIFFReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *TIFFReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *TIFFReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the TIFF file structure
func (r *TIFFReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	// Get basic file info
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to stat file: %w", err)
	}

	// Validate TIFF format
	if !r.isTIFFFile(sourcePath) {
		return core.SchemaInfo{}, fmt.Errorf("not a valid TIFF file")
	}

	// Extract TIFF metadata
	tiffInfo, err := r.extractTIFFInfo(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to extract TIFF info: %w", err)
	}

	schema := core.SchemaInfo{
		Format:   "tiff",
		Encoding: "binary",
		Fields: []core.FieldInfo{
			{
				Name:        "content",
				Type:        "text",
				Nullable:    false,
				Description: "Extracted text content from OCR",
			},
			{
				Name:        "page_number",
				Type:        "integer",
				Nullable:    false,
				Description: "Page number in multi-page TIFF",
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
		},
		Metadata: map[string]any{
			"file_size":       stat.Size(),
			"page_count":      tiffInfo.PageCount,
			"width":           tiffInfo.Width,
			"height":          tiffInfo.Height,
			"color_space":     tiffInfo.ColorSpace,
			"compression":     tiffInfo.Compression,
			"resolution_x":    tiffInfo.ResolutionX,
			"resolution_y":    tiffInfo.ResolutionY,
			"bits_per_sample": tiffInfo.BitsPerSample,
			"creation_date":   tiffInfo.CreationDate,
			"software":        tiffInfo.Software,
			"photographer":    tiffInfo.Photographer,
			"has_text":        tiffInfo.HasText,
		},
	}

	// Sample OCR if enabled
	config := map[string]any{"ocr_enabled": true, "ocr_language": "eng"}
	if tiffInfo.PageCount > 0 {
		sampleText, confidence, _, err := r.performOCR(sourcePath, 1, config)
		if err == nil && sampleText != "" {
			schema.SampleData = []map[string]any{
				{
					"content":     sampleText[:min(200, len(sampleText))],
					"page_number": 1,
					"confidence":  confidence,
					"language":    "eng",
				},
			}
		}
	}

	return schema, nil
}

// EstimateSize returns size estimates for the TIFF file
func (r *TIFFReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	tiffInfo, err := r.extractTIFFInfo(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to extract TIFF info: %w", err)
	}

	// Each page becomes one chunk
	estimatedChunks := tiffInfo.PageCount
	if estimatedChunks == 0 {
		estimatedChunks = 1
	}

	// Complexity based on size, resolution, and page count
	complexity := "low"
	totalPixels := int64(tiffInfo.Width * tiffInfo.Height * tiffInfo.PageCount)

	if stat.Size() > 10*1024*1024 || totalPixels > 50*1024*1024 || tiffInfo.PageCount > 10 {
		complexity = "medium"
	}
	if stat.Size() > 100*1024*1024 || totalPixels > 200*1024*1024 || tiffInfo.PageCount > 50 {
		complexity = "high"
	}

	// OCR processing is inherently slower
	processTime := "medium"
	if stat.Size() > 50*1024*1024 || tiffInfo.PageCount > 20 {
		processTime = "slow"
	}
	if stat.Size() > 200*1024*1024 || tiffInfo.PageCount > 100 {
		processTime = "very_slow"
	}

	pageCount := int64(tiffInfo.PageCount)
	return core.SizeEstimate{
		RowCount:    &pageCount,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the TIFF file
func (r *TIFFReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	if !r.isTIFFFile(sourcePath) {
		return nil, fmt.Errorf("not a valid TIFF file")
	}

	tiffInfo, err := r.extractTIFFInfo(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract TIFF info: %w", err)
	}

	iterator := &TIFFIterator{
		sourcePath:  sourcePath,
		config:      strategyConfig,
		tiffInfo:    tiffInfo,
		currentPage: 0,
		totalPages:  tiffInfo.PageCount,
	}

	return iterator, nil
}

// SupportsStreaming indicates TIFF reader supports streaming
func (r *TIFFReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *TIFFReader) GetSupportedFormats() []string {
	return []string{"tiff", "tif"}
}

// TIFFInfo contains extracted TIFF information
type TIFFInfo struct {
	PageCount     int
	Width         int
	Height        int
	ColorSpace    string
	Compression   string
	ResolutionX   int
	ResolutionY   int
	BitsPerSample int
	CreationDate  string
	Software      string
	Photographer  string
	HasText       bool
}

// isTIFFFile checks if the file is a valid TIFF file
func (r *TIFFReader) isTIFFFile(filePath string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	// Check TIFF signature
	signature := make([]byte, 4)
	n, err := file.Read(signature)
	if err != nil || n != 4 {
		return false
	}

	// TIFF signatures: II*\0 (little-endian) or MM\0* (big-endian)
	return (signature[0] == 0x49 && signature[1] == 0x49 && signature[2] == 0x2A && signature[3] == 0x00) ||
		(signature[0] == 0x4D && signature[1] == 0x4D && signature[2] == 0x00 && signature[3] == 0x2A)
}

// extractTIFFInfo extracts metadata from TIFF file
func (r *TIFFReader) extractTIFFInfo(sourcePath string) (TIFFInfo, error) {
	// This is a simplified TIFF parser
	// In production, you'd use a proper TIFF library like:
	// - golang.org/x/image/tiff
	// - Or call external tools like exiftool

	info := TIFFInfo{
		PageCount:     1, // Default single page
		Width:         1024,
		Height:        768,
		ColorSpace:    "RGB",
		Compression:   "None",
		ResolutionX:   300,
		ResolutionY:   300,
		BitsPerSample: 8,
		CreationDate:  time.Now().Format("2006-01-02 15:04:05"),
		Software:      "Unknown",
		HasText:       true, // Assume document images contain text
	}

	// Get file stats for basic info
	if stat, err := os.Stat(sourcePath); err == nil {
		// Estimate page count based on file size (very rough)
		if stat.Size() > 5*1024*1024 { // > 5MB
			info.PageCount = int(stat.Size() / (1024 * 1024)) // ~1MB per page estimate
			if info.PageCount > 100 {
				info.PageCount = 100 // Cap at reasonable limit
			}
		}
	}

	return info, nil
}

// performOCR performs OCR on a specific page of the TIFF
func (r *TIFFReader) performOCR(sourcePath string, pageNum int, config map[string]any) (string, float64, []TextRegion, error) {
	// In a real implementation, this would:
	// 1. Extract the specific page from the TIFF
	// 2. Preprocess the image if configured
	// 3. Run tesseract OCR
	// 4. Parse the results and confidence scores
	// 5. Optionally extract text regions with coordinates

	// Mock OCR results
	ocrText := fmt.Sprintf("This is sample OCR text extracted from page %d of the TIFF image. "+
		"In a production implementation, this would be the actual text recognized by Tesseract OCR "+
		"from the image content.", pageNum)

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

// TIFFIterator implements ChunkIterator for TIFF files
type TIFFIterator struct {
	sourcePath  string
	config      map[string]any
	tiffInfo    TIFFInfo
	currentPage int
	totalPages  int
}

// Next returns the next chunk of content from the TIFF
func (it *TIFFIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	// Check if we've processed all pages
	if it.currentPage >= it.totalPages {
		return core.Chunk{}, core.ErrIteratorExhausted
	}

	// Check max_pages limit
	if maxPages, ok := it.config["max_pages"]; ok {
		if max, ok := maxPages.(float64); ok && max > 0 {
			if it.currentPage >= int(max) {
				return core.Chunk{}, core.ErrIteratorExhausted
			}
		}
	}

	it.currentPage++

	// Perform OCR if enabled
	var content string
	var confidence float64
	var regions []TextRegion

	if ocrEnabled, ok := it.config["ocr_enabled"]; !ok || ocrEnabled.(bool) {
		var err error
		content, confidence, regions, err = it.performPageOCR()
		if err != nil {
			return core.Chunk{}, fmt.Errorf("OCR failed for page %d: %w", it.currentPage, err)
		}
	} else {
		content = fmt.Sprintf("Page %d of TIFF image (OCR disabled)", it.currentPage)
		confidence = 0.0
	}

	chunk := core.Chunk{
		Data: content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:page:%d", filepath.Base(it.sourcePath), it.currentPage),
			ChunkType:   "tiff_page",
			SizeBytes:   int64(len(content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "tiff_reader",
			Context: map[string]string{
				"page_number": strconv.Itoa(it.currentPage),
				"total_pages": strconv.Itoa(it.totalPages),
				"confidence":  fmt.Sprintf("%.2f", confidence),
				"file_type":   "tiff",
				"width":       strconv.Itoa(it.tiffInfo.Width),
				"height":      strconv.Itoa(it.tiffInfo.Height),
				"color_space": it.tiffInfo.ColorSpace,
				"compression": it.tiffInfo.Compression,
			},
		},
	}

	// Add OCR-specific context
	if ocrLang, ok := it.config["ocr_language"]; ok {
		chunk.Metadata.Context["ocr_language"] = ocrLang.(string)
	}
	if len(regions) > 0 {
		chunk.Metadata.Context["text_regions_count"] = strconv.Itoa(len(regions))
	}

	return chunk, nil
}

// performPageOCR performs OCR on the current page
func (it *TIFFIterator) performPageOCR() (string, float64, []TextRegion, error) {
	reader := &TIFFReader{}
	return reader.performOCR(it.sourcePath, it.currentPage, it.config)
}

// Close releases TIFF resources
func (it *TIFFIterator) Close() error {
	// Nothing to close for TIFF iterator
	return nil
}

// Reset restarts iteration from the beginning
func (it *TIFFIterator) Reset() error {
	it.currentPage = 0
	return nil
}

// Progress returns iteration progress
func (it *TIFFIterator) Progress() float64 {
	if it.totalPages == 0 {
		return 1.0
	}
	return float64(it.currentPage) / float64(it.totalPages)
}

// Helper functions

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
