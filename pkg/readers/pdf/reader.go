package pdf

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jscharber/audimodal/pkg/core"
	"github.com/jscharber/audimodal/pkg/readers/pdf/mapreduce"
)

// PDFReader implements DataSourceReader for PDF files with OCR support
type PDFReader struct {
	name    string
	version string
}

// NewPDFReader creates a new PDF file reader
func NewPDFReader() *PDFReader {
	return &PDFReader{
		name:    "pdf_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *PDFReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "processing_mode",
			Type:        "string",
			Required:    false,
			Default:     "auto",
			Description: "Processing mode: streaming (page-by-page in process), mapreduce (subprocess isolation for large PDFs), or auto (select based on page count)",
			Enum:        []string{"streaming", "mapreduce", "auto"},
		},
		{
			Name:        "mapreduce_page_threshold",
			Type:        "int",
			Required:    false,
			Default:     50,
			Description: "Use map-reduce mode for PDFs with more than this many pages (when processing_mode=auto)",
			MinValue:    ptrFloat64(1.0),
			MaxValue:    ptrFloat64(1000.0),
		},
		{
			Name:        "mapreduce_workers",
			Type:        "int",
			Required:    false,
			Default:     4,
			Description: "Number of parallel workers for map-reduce mode",
			MinValue:    ptrFloat64(1.0),
			MaxValue:    ptrFloat64(16.0),
		},
		{
			Name:        "extract_mode",
			Type:        "string",
			Required:    false,
			Default:     "auto",
			Description: "Text extraction mode: text, ocr, or auto",
			Enum:        []string{"text", "ocr", "auto"},
		},
		{
			Name:        "ocr_language",
			Type:        "string",
			Required:    false,
			Default:     "eng",
			Description: "OCR language code (ISO 639-2)",
			Enum:        []string{"eng", "spa", "fra", "deu", "ita", "por", "rus", "chi_sim", "chi_tra", "jpn", "kor"},
		},
		{
			Name:        "ocr_dpi",
			Type:        "int",
			Required:    false,
			Default:     300,
			Description: "OCR image resolution in DPI",
			MinValue:    ptrFloat64(150.0),
			MaxValue:    ptrFloat64(600.0),
		},
		{
			Name:        "include_images",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract and process embedded images",
		},
		{
			Name:        "preserve_layout",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Preserve document layout and formatting",
		},
		{
			Name:        "extract_metadata",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract PDF metadata (title, author, etc.)",
		},
		{
			Name:        "password",
			Type:        "string",
			Required:    false,
			Default:     "",
			Description: "Password for encrypted PDFs",
		},
		{
			Name:        "max_pages",
			Type:        "int",
			Required:    false,
			Default:     0,
			Description: "Maximum pages to process (0 = all pages)",
			MinValue:    ptrFloat64(0.0),
			MaxValue:    ptrFloat64(10000.0),
		},
		{
			Name:        "skip_images_larger_than_mb",
			Type:        "int",
			Required:    false,
			Default:     10,
			Description: "Skip OCR for images larger than this size in MB",
			MinValue:    ptrFloat64(1.0),
			MaxValue:    ptrFloat64(100.0),
		},
		{
			Name:        "ocr_any_image",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "If true, trigger OCR for ANY image on page regardless of size. Useful for documents with small but important image content.",
		},
		{
			Name:        "ocr_image_min_width",
			Type:        "int",
			Required:    false,
			Default:     200,
			Description: "Minimum image width in pixels to trigger OCR. Images smaller than this are ignored.",
			MinValue:    ptrFloat64(1.0),
			MaxValue:    ptrFloat64(10000.0),
		},
		{
			Name:        "ocr_image_min_height",
			Type:        "int",
			Required:    false,
			Default:     200,
			Description: "Minimum image height in pixels to trigger OCR. Images smaller than this are ignored.",
			MinValue:    ptrFloat64(1.0),
			MaxValue:    ptrFloat64(10000.0),
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *PDFReader) ValidateConfig(config map[string]any) error {
	if mode, ok := config["extract_mode"]; ok {
		if str, ok := mode.(string); ok {
			validModes := []string{"text", "ocr", "auto"}
			for _, valid := range validModes {
				if str == valid {
					goto modeOK
				}
			}
			return fmt.Errorf("invalid extract_mode: %s", str)
		modeOK:
		}
	}

	if lang, ok := config["ocr_language"]; ok {
		if str, ok := lang.(string); ok {
			validLangs := []string{"eng", "spa", "fra", "deu", "ita", "por", "rus", "chi_sim", "chi_tra", "jpn", "kor"}
			for _, valid := range validLangs {
				if str == valid {
					goto langOK
				}
			}
			return fmt.Errorf("invalid ocr_language: %s", str)
		langOK:
		}
	}

	if dpi, ok := config["ocr_dpi"]; ok {
		if num, ok := dpi.(float64); ok {
			if num < 150 || num > 600 {
				return fmt.Errorf("ocr_dpi must be between 150 and 600")
			}
		}
	}

	return nil
}

// TestConnection tests if the PDF can be read
func (r *PDFReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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

	// Test if required dependencies are available
	dependencies := r.checkDependencies()

	return core.ConnectionTestResult{
		Success: len(dependencies) == 0,
		Message: func() string {
			if len(dependencies) == 0 {
				return "PDF reader ready"
			}
			return "Missing dependencies for PDF processing"
		}(),
		Latency: time.Since(start),
		Errors:  dependencies,
		Details: map[string]any{
			"extract_mode": config["extract_mode"],
			"ocr_language": config["ocr_language"],
			"dependencies": len(dependencies) == 0,
		},
	}
}

// checkDependencies verifies required tools are available
func (r *PDFReader) checkDependencies() []string {
	var missing []string

	// Check for pdftotext (poppler-utils)
	if !r.commandExists("pdftotext") {
		missing = append(missing, "pdftotext (install poppler-utils)")
	}

	// Check for tesseract (OCR)
	if !r.commandExists("tesseract") {
		missing = append(missing, "tesseract (install tesseract-ocr)")
	}

	// Check for pdftoppm (for image conversion)
	if !r.commandExists("pdftoppm") {
		missing = append(missing, "pdftoppm (install poppler-utils)")
	}

	return missing
}

// commandExists checks if a command is available in PATH
func (r *PDFReader) commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// GetType returns the connector type
func (r *PDFReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *PDFReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *PDFReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the PDF file structure
func (r *PDFReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	// Get basic file info
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to stat file: %w", err)
	}

	// Extract basic PDF metadata
	metadata, err := r.extractPDFMetadata(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to extract PDF metadata: %w", err)
	}

	schema := core.SchemaInfo{
		Format:   "pdf",
		Encoding: "binary",
		Fields: []core.FieldInfo{
			{
				Name:        "content",
				Type:        "text",
				Nullable:    false,
				Description: "Extracted text content",
			},
			{
				Name:        "page_number",
				Type:        "integer",
				Nullable:    false,
				Description: "Page number in PDF",
			},
			{
				Name:        "page_text",
				Type:        "text",
				Nullable:    true,
				Description: "Text content for specific page",
			},
			{
				Name:        "extraction_method",
				Type:        "string",
				Nullable:    false,
				Description: "Method used for text extraction (text/ocr)",
			},
			{
				Name:        "confidence",
				Type:        "float",
				Nullable:    true,
				Description: "OCR confidence score (0-1)",
			},
		},
		Metadata: map[string]any{
			"file_size":     stat.Size(),
			"page_count":    metadata.PageCount,
			"title":         metadata.Title,
			"author":        metadata.Author,
			"subject":       metadata.Subject,
			"creator":       metadata.Creator,
			"producer":      metadata.Producer,
			"creation_date": metadata.CreationDate,
			"modified_date": metadata.ModificationDate,
			"encrypted":     metadata.Encrypted,
			"pdf_version":   metadata.PDFVersion,
		},
	}

	// Sample first page for analysis
	if metadata.PageCount > 0 {
		sampleText, method, confidence, err := r.extractPageText(sourcePath, 1, map[string]any{})
		if err == nil {
			schema.SampleData = []map[string]any{
				{
					"content":           sampleText[:min(500, len(sampleText))], // First 500 chars
					"page_number":       1,
					"page_text":         sampleText,
					"extraction_method": method,
					"confidence":        confidence,
				},
			}
		}
	}

	return schema, nil
}

// EstimateSize returns size estimates for the PDF file
func (r *PDFReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	metadata, err := r.extractPDFMetadata(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to extract PDF metadata: %w", err)
	}

	// Estimate text content based on page count and average characters per page
	avgCharsPerPage := int64(2000) // Conservative estimate
	if metadata.PageCount > 0 {
		estimatedTextSize := int64(metadata.PageCount) * avgCharsPerPage

		// Estimate chunks based on typical chunk size (1000 characters)
		chunkSize := int64(1000)
		estimatedChunks := int((estimatedTextSize + chunkSize - 1) / chunkSize)

		// Determine complexity based on file size and page count
		complexity := "low"
		if stat.Size() > 5*1024*1024 || metadata.PageCount > 50 { // > 5MB or > 50 pages
			complexity = "medium"
		}
		if stat.Size() > 50*1024*1024 || metadata.PageCount > 500 { // > 50MB or > 500 pages
			complexity = "high"
		}

		// Estimate processing time based on size and whether OCR might be needed
		processTime := "fast"
		if stat.Size() > 10*1024*1024 || metadata.PageCount > 100 {
			processTime = "medium"
		}
		if stat.Size() > 100*1024*1024 || metadata.PageCount > 1000 {
			processTime = "slow"
		}

		pageCount := int64(metadata.PageCount)
		return core.SizeEstimate{
			RowCount:    &pageCount,
			ByteSize:    stat.Size(),
			Complexity:  complexity,
			ChunkEst:    estimatedChunks,
			ProcessTime: processTime,
		}, nil
	}

	return core.SizeEstimate{
		ByteSize:    stat.Size(),
		Complexity:  "unknown",
		ChunkEst:    1,
		ProcessTime: "unknown",
	}, nil
}

// CreateIterator creates a chunk iterator for the PDF file
func (r *PDFReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	log.Printf("[PDF] CreateIterator called for %s", sourcePath)

	metadata, err := r.extractPDFMetadata(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract PDF metadata: %w", err)
	}

	// Determine processing mode
	processingMode := getConfigString(strategyConfig, "processing_mode", "auto")
	threshold := getConfigInt(strategyConfig, "mapreduce_page_threshold", 50)

	log.Printf("[PDF] Mode decision: mode=%s, pageCount=%d, threshold=%d",
		processingMode, metadata.PageCount, threshold)

	useMapReduce := processingMode == "mapreduce" ||
		(processingMode == "auto" && metadata.PageCount > threshold)

	log.Printf("[PDF] useMapReduce=%v (mode=%s, pages=%d > threshold=%d = %v)",
		useMapReduce, processingMode, metadata.PageCount, threshold, metadata.PageCount > threshold)

	if useMapReduce {
		log.Printf("[PDF] Using map-reduce mode for %s (%d pages > threshold %d)",
			sourcePath, metadata.PageCount, threshold)
		return r.createMapReduceIterator(ctx, sourcePath, strategyConfig, metadata)
	}

	log.Printf("[PDF] Using streaming mode for %s (%d pages)", sourcePath, metadata.PageCount)

	// Use streaming mode (existing behavior)
	iterator := &PDFIterator{
		sourcePath:  sourcePath,
		config:      strategyConfig,
		metadata:    metadata,
		currentPage: 0,
		totalPages:  metadata.PageCount,
	}

	return iterator, nil
}

// createMapReduceIterator creates an iterator that uses the map-reduce pipeline
func (r *PDFReader) createMapReduceIterator(ctx context.Context, sourcePath string, config map[string]any, metadata PDFMetadata) (core.ChunkIterator, error) {
	// Build map-reduce configuration
	mrConfig := mapreduce.DefaultCoordinatorConfig()
	mrConfig.MaxConcurrentWorkers = getConfigInt(config, "mapreduce_workers", 4)
	mrConfig.MapReducePageThreshold = getConfigInt(config, "mapreduce_page_threshold", 50)

	// Configure extraction settings
	mrConfig.Extraction.OCRLanguage = getConfigString(config, "ocr_language", "eng")
	mrConfig.Extraction.OCRDPI = getConfigInt(config, "ocr_dpi", 150)
	mrConfig.Extraction.PreserveLayout = getConfigBool(config, "preserve_layout", true)
	mrConfig.Extraction.TextThreshold = 100 // Min chars for text_only classification

	// Configure image detection settings for OCR triggering
	mrConfig.Extraction.OCRAnyImage = getConfigBool(config, "ocr_any_image", false)
	mrConfig.Extraction.OCRImageMinWidth = getConfigInt(config, "ocr_image_min_width", 200)
	mrConfig.Extraction.OCRImageMinHeight = getConfigInt(config, "ocr_image_min_height", 200)

	// Get worker path from config or environment
	// If PDF_WORKER_PATH is set, use subprocess isolation for memory management
	workerPath := getConfigString(config, "pdf_worker_path", "")
	if workerPath == "" {
		workerPath = os.Getenv("PDF_WORKER_PATH")
	}
	if workerPath != "" {
		log.Printf("[PDF] Using subprocess worker at: %s", workerPath)
	} else {
		log.Printf("[PDF] Using inline processing (set PDF_WORKER_PATH for subprocess isolation)")
	}

	// Parse tenant/file IDs from config if provided
	tenantID := uuid.Nil
	fileID := uuid.Nil
	if tidStr := getConfigString(config, "tenant_id", ""); tidStr != "" {
		if parsed, err := uuid.Parse(tidStr); err == nil {
			tenantID = parsed
		}
	}
	if fidStr := getConfigString(config, "file_id", ""); fidStr != "" {
		if parsed, err := uuid.Parse(fidStr); err == nil {
			fileID = parsed
		}
	}

	// Create coordinator
	coordinator := mapreduce.NewCoordinator(mrConfig, nil, workerPath)

	// Create the map-reduce iterator
	return &MapReduceIterator{
		sourcePath:  sourcePath,
		config:      config,
		metadata:    metadata,
		coordinator: coordinator,
		mrConfig:    mrConfig,
		tenantID:    tenantID,
		fileID:      fileID,
	}, nil
}

// MapReduceIterator implements ChunkIterator using the map-reduce pipeline
type MapReduceIterator struct {
	sourcePath  string
	config      map[string]any
	metadata    PDFMetadata
	coordinator *mapreduce.DefaultCoordinator
	mrConfig    *mapreduce.CoordinatorConfig
	tenantID    uuid.UUID
	fileID      uuid.UUID

	// Processing state
	processed bool
	chunks    []core.Chunk
	position  int
}

// Next returns the next chunk from the map-reduce results
func (it *MapReduceIterator) Next(ctx context.Context) (core.Chunk, error) {
	// Process all pages on first call
	if !it.processed {
		if err := it.processAllPages(ctx); err != nil {
			return core.Chunk{}, err
		}
	}

	// Return chunks one at a time
	if it.position >= len(it.chunks) {
		return core.Chunk{}, core.ErrIteratorExhausted
	}

	chunk := it.chunks[it.position]
	it.position++
	return chunk, nil
}

// processAllPages runs the map-reduce pipeline
func (it *MapReduceIterator) processAllPages(ctx context.Context) error {
	it.processed = true

	// Process using map-reduce coordinator with tenant/file context
	result, err := it.coordinator.Process(ctx, it.sourcePath,
		it.tenantID,
		it.fileID,
	)
	if err != nil {
		return fmt.Errorf("map-reduce processing failed: %w", err)
	}

	// Convert results to chunks using reducer
	reducer := mapreduce.NewReducer(nil, &mapreduce.ReducerConfig{
		PreservePageBoundaries: true,
		SourcePath:             it.sourcePath,
	})

	chunks, err := reducer.ReduceFromResults(result)
	if err != nil {
		return fmt.Errorf("reduce failed: %w", err)
	}

	log.Printf("[MapReduce] Reducer produced %d chunks from %d pages (%d failed, %d total chars)",
		len(chunks), result.TotalPages, result.FailedPages, result.TotalCharacters)

	it.chunks = chunks
	return nil
}

// Close releases resources
func (it *MapReduceIterator) Close() error {
	if it.coordinator != nil {
		return it.coordinator.Shutdown(context.Background())
	}
	return nil
}

// Reset restarts iteration
func (it *MapReduceIterator) Reset() error {
	it.position = 0
	return nil
}

// Progress returns iteration progress
func (it *MapReduceIterator) Progress() float64 {
	if !it.processed || len(it.chunks) == 0 {
		return 0.0
	}
	return float64(it.position) / float64(len(it.chunks))
}

// Helper functions for config access
func getConfigString(config map[string]any, key, defaultVal string) string {
	if v, ok := config[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultVal
}

func getConfigInt(config map[string]any, key string, defaultVal int) int {
	if v, ok := config[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case float64:
			return int(val)
		case int64:
			return int(val)
		}
	}
	return defaultVal
}

func getConfigBool(config map[string]any, key string, defaultVal bool) bool {
	if v, ok := config[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return defaultVal
}

// SupportsStreaming indicates PDF reader supports streaming
func (r *PDFReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *PDFReader) GetSupportedFormats() []string {
	return []string{"pdf"}
}

// PDFMetadata contains extracted PDF metadata
type PDFMetadata struct {
	PageCount        int
	Title            string
	Author           string
	Subject          string
	Creator          string
	Producer         string
	CreationDate     string
	ModificationDate string
	Encrypted        bool
	PDFVersion       string
}

// extractPDFMetadata extracts metadata from PDF file using pdfinfo
func (r *PDFReader) extractPDFMetadata(sourcePath string) (PDFMetadata, error) {
	metadata := PDFMetadata{
		PageCount: 1, // Default to 1 page if we can't determine
	}

	// Use pdfinfo to get metadata
	cmd := exec.Command("pdfinfo", sourcePath)
	output, err := cmd.Output()
	if err != nil {
		log.Printf("[WARN] pdfinfo failed for %s: %v, using defaults", sourcePath, err)
		return metadata, nil // Return defaults, don't fail
	}

	// Parse pdfinfo output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "Pages":
			if count, err := strconv.Atoi(value); err == nil {
				metadata.PageCount = count
			}
		case "Title":
			metadata.Title = value
		case "Author":
			metadata.Author = value
		case "Subject":
			metadata.Subject = value
		case "Creator":
			metadata.Creator = value
		case "Producer":
			metadata.Producer = value
		case "CreationDate":
			metadata.CreationDate = value
		case "ModDate":
			metadata.ModificationDate = value
		case "Encrypted":
			metadata.Encrypted = strings.ToLower(value) == "yes"
		case "PDF version":
			metadata.PDFVersion = value
		}
	}

	log.Printf("[DEBUG] PDF Metadata extracted for %s: %d pages, title=%s",
		sourcePath, metadata.PageCount, metadata.Title)

	return metadata, nil
}

// extractPageText extracts text from a specific page using hybrid approach
func (r *PDFReader) extractPageText(sourcePath string, pageNum int, config map[string]any) (string, string, float64, error) {
	// Get extraction mode
	mode := "auto"
	if m, ok := config["extract_mode"]; ok {
		if str, ok := m.(string); ok {
			mode = str
		}
	}

	// Get OCR language
	ocrLang := "eng"
	if lang, ok := config["ocr_language"].(string); ok {
		ocrLang = lang
	}

	// Get image detection settings from config (for configurable OCR triggering)
	ocrAnyImage := getConfigBool(config, "ocr_any_image", false)
	ocrImageMinWidth := getConfigInt(config, "ocr_image_min_width", 200)
	ocrImageMinHeight := getConfigInt(config, "ocr_image_min_height", 200)

	var allText []string
	var extractionMethod string
	confidence := 1.0

	// Step 1: Check if page has large images that may contain text
	hasLargeImages := false
	imageCount := 0
	if mode == "auto" || mode == "hybrid" {
		var err error
		hasLargeImages, imageCount, err = r.checkPageHasLargeImages(sourcePath, pageNum, ocrImageMinWidth, ocrImageMinHeight, ocrAnyImage)
		if err != nil {
			log.Printf("[WARN] Image detection failed for page %d: %v", pageNum, err)
		} else if imageCount > 0 {
			log.Printf("[DEBUG] Page %d has %d images (large: %v, minWidth=%d, minHeight=%d, anyImage=%v)",
				pageNum, imageCount, hasLargeImages, ocrImageMinWidth, ocrImageMinHeight, ocrAnyImage)
		}
	}

	// Step 2: Extract native text using pdftotext (unless mode is explicitly "ocr")
	if mode != "ocr" {
		nativeText, err := r.extractNativeText(sourcePath, pageNum)
		if err != nil {
			log.Printf("[WARN] pdftotext failed for page %d: %v", pageNum, err)
		} else if nativeText != "" {
			allText = append(allText, nativeText)
			extractionMethod = "text"
			log.Printf("[DEBUG] Native text extracted from page %d: %d chars", pageNum, len(nativeText))
		}
	}

	// Step 3: If page has large images, also run OCR to capture image text (hybrid mode)
	// This ensures we don't miss text embedded in images even when native text exists
	if hasLargeImages && (mode == "auto" || mode == "hybrid") {
		log.Printf("[DEBUG] Page %d has large images, running OCR for hybrid extraction", pageNum)
		ocrText, ocrConf, err := r.extractFullPageOCR(sourcePath, pageNum, ocrLang)
		if err != nil {
			log.Printf("[WARN] Hybrid OCR failed for page %d: %v", pageNum, err)
		} else if ocrText != "" {
			// For hybrid mode, use OCR result if it has more content or if native text was empty
			if len(ocrText) > len(strings.Join(allText, "")) {
				allText = []string{ocrText} // Use OCR text as it has more content
				extractionMethod = "hybrid-ocr"
				confidence = ocrConf
				log.Printf("[DEBUG] Hybrid OCR has more content for page %d: %d chars (vs %d native), confidence=%.2f",
					pageNum, len(ocrText), len(strings.Join(allText, "")), ocrConf)
			} else {
				extractionMethod = "hybrid-text"
				log.Printf("[DEBUG] Native text preferred for page %d: %d chars (vs %d OCR)",
					pageNum, len(strings.Join(allText, "")), len(ocrText))
			}
		}
	}

	// Step 4: If no text found, fall back to full-page OCR
	if len(allText) == 0 || mode == "ocr" {
		ocrText, ocrConf, err := r.extractFullPageOCR(sourcePath, pageNum, ocrLang)
		if err != nil {
			log.Printf("[WARN] Full page OCR failed for page %d: %v", pageNum, err)
		} else if ocrText != "" {
			allText = []string{ocrText} // Replace with OCR text
			extractionMethod = "ocr"
			confidence = ocrConf
			log.Printf("[DEBUG] Full page OCR extracted from page %d: %d chars, confidence=%.2f", pageNum, len(ocrText), ocrConf)
		}
	}

	// Combine all extracted text
	finalText := strings.Join(allText, "\n\n")

	// Default to "text" if we have content but no method set
	if extractionMethod == "" && finalText != "" {
		extractionMethod = "text"
	}
	if extractionMethod == "" {
		extractionMethod = "none"
	}

	log.Printf("[INFO] PDF Text Extraction - Page %d from %s: method=%s, confidence=%.2f, length=%d",
		pageNum, filepath.Base(sourcePath), extractionMethod, confidence, len(finalText))

	return finalText, extractionMethod, confidence, nil
}

// extractNativeText extracts text encoded in the PDF using pdftotext with streaming
func (r *PDFReader) extractNativeText(sourcePath string, pageNum int) (string, error) {
	cmd := exec.Command("pdftotext",
		"-f", strconv.Itoa(pageNum),
		"-l", strconv.Itoa(pageNum),
		"-layout",
		sourcePath,
		"-") // Output to stdout

	// Use streaming pipe instead of buffering entire output
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("pdftotext failed to start: %w", err)
	}

	// Stream output line by line with limited buffer
	var result strings.Builder
	scanner := bufio.NewScanner(stdoutPipe)
	// Set reasonable buffer limits: 64KB initial, 1MB max per line
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		result.WriteString(scanner.Text())
		result.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		cmd.Wait()
		return "", fmt.Errorf("error reading pdftotext output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("pdftotext failed: %w (stderr: %s)", err, stderr.String())
	}

	return strings.TrimSpace(result.String()), nil
}

// extractEmbeddedImageText extracts images from PDF page and OCRs them with streaming
func (r *PDFReader) extractEmbeddedImageText(sourcePath string, pageNum int, ocrLang string) (string, error) {
	// Create temp directory for extracted images
	tmpDir, err := os.MkdirTemp("", "pdf-images-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Extract embedded images from specific page using pdfimages
	imgPrefix := filepath.Join(tmpDir, "img")
	cmd := exec.Command("pdfimages",
		"-f", strconv.Itoa(pageNum),
		"-l", strconv.Itoa(pageNum),
		"-png",
		sourcePath,
		imgPrefix)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdfimages failed: %w", err)
	}

	// Find all extracted images
	matches, err := filepath.Glob(filepath.Join(tmpDir, "img-*.png"))
	if err != nil {
		return "", fmt.Errorf("failed to glob images: %w", err)
	}

	// OCR each image with streaming and immediate cleanup
	var allText []string
	for _, imgFile := range matches {
		// Skip tiny images (likely icons/bullets) - under 1KB
		info, err := os.Stat(imgFile)
		if err != nil {
			os.Remove(imgFile) // Cleanup even on error
			continue
		}
		if info.Size() < 1024 {
			log.Printf("[DEBUG] Skipping small image %s (%d bytes)", filepath.Base(imgFile), info.Size())
			os.Remove(imgFile) // Cleanup immediately
			continue
		}

		// Run tesseract OCR with streaming
		text, _, err := r.runTesseractPlainStreaming(imgFile, ocrLang)

		// Immediately cleanup image file to free disk space and memory
		os.Remove(imgFile)

		if err != nil {
			log.Printf("[WARN] Tesseract failed for %s: %v", filepath.Base(imgFile), err)
			continue
		}

		// Skip very short results (likely noise)
		if len(text) > 10 {
			allText = append(allText, text)
		}
	}

	return strings.Join(allText, "\n"), nil
}

// extractFullPageOCR renders the entire page as image and OCRs it with streaming
func (r *PDFReader) extractFullPageOCR(sourcePath string, pageNum int, ocrLang string) (string, float64, error) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "pdf-ocr-")
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temp dir: %w", err)
	}
	// Clean up temp directory at end
	defer os.RemoveAll(tmpDir)

	// Render page as image using pdftoppm
	imgPrefix := filepath.Join(tmpDir, "page")
	cmd := exec.Command("pdftoppm",
		"-f", strconv.Itoa(pageNum),
		"-l", strconv.Itoa(pageNum),
		"-r", "150", // 150 DPI - reduced from 300 to save memory (4x reduction in image size)
		"-png",
		sourcePath,
		imgPrefix)

	if err := cmd.Run(); err != nil {
		return "", 0, fmt.Errorf("pdftoppm failed: %w", err)
	}

	// Find the generated image (pdftoppm adds page number suffix)
	imgFile := fmt.Sprintf("%s-%d.png", imgPrefix, pageNum)
	if _, err := os.Stat(imgFile); os.IsNotExist(err) {
		// Try with zero-padded page number
		imgFile = fmt.Sprintf("%s-%02d.png", imgPrefix, pageNum)
		if _, err := os.Stat(imgFile); os.IsNotExist(err) {
			imgFile = fmt.Sprintf("%s-%03d.png", imgPrefix, pageNum)
		}
	}

	if _, err := os.Stat(imgFile); os.IsNotExist(err) {
		return "", 0, fmt.Errorf("pdftoppm did not create expected image file")
	}

	// Run tesseract OCR with streaming output
	text, confidence, err := r.runTesseractStreaming(imgFile, ocrLang)

	// Immediately cleanup image file to free disk space
	os.Remove(imgFile)

	if err != nil {
		return "", 0, err
	}

	return text, confidence, nil
}

// runTesseractStreaming runs tesseract with streaming output to reduce memory usage
func (r *PDFReader) runTesseractStreaming(imgFile, ocrLang string) (string, float64, error) {
	// Try TSV output first for confidence scores
	cmd := exec.Command("tesseract", imgFile, "stdout", "-l", ocrLang, "tsv")

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", 0, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		// Fall back to plain text output
		return r.runTesseractPlainStreaming(imgFile, ocrLang)
	}

	// Stream and parse TSV output
	var words []string
	var totalConf float64
	var confCount int

	scanner := bufio.NewScanner(stdoutPipe)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	isFirstLine := true

	for scanner.Scan() {
		if isFirstLine {
			isFirstLine = false
			continue // Skip header
		}
		line := scanner.Text()
		fields := strings.Split(line, "\t")
		if len(fields) < 12 {
			continue
		}

		conf, err := strconv.ParseFloat(fields[10], 64)
		if err != nil || conf < 0 {
			continue
		}

		text := strings.TrimSpace(fields[11])
		if text != "" && conf >= 0 {
			words = append(words, text)
			if conf > 0 {
				totalConf += conf
				confCount++
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		// Fall back to plain text output
		return r.runTesseractPlainStreaming(imgFile, ocrLang)
	}

	avgConf := 0.8
	if confCount > 0 {
		avgConf = totalConf / float64(confCount) / 100.0
	}

	return strings.Join(words, " "), avgConf, nil
}

// runTesseractPlainStreaming runs tesseract with plain text streaming output
func (r *PDFReader) runTesseractPlainStreaming(imgFile, ocrLang string) (string, float64, error) {
	cmd := exec.Command("tesseract", imgFile, "stdout", "-l", ocrLang)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", 0, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", 0, fmt.Errorf("tesseract failed to start: %w", err)
	}

	var result strings.Builder
	scanner := bufio.NewScanner(stdoutPipe)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		result.WriteString(scanner.Text())
		result.WriteString("\n")
	}

	if err := cmd.Wait(); err != nil {
		return "", 0, fmt.Errorf("tesseract failed: %w", err)
	}

	return strings.TrimSpace(result.String()), 0.8, nil
}

// parseTesseractTSV parses tesseract TSV output to extract text and average confidence
func (r *PDFReader) parseTesseractTSV(tsvOutput string) (string, float64) {
	lines := strings.Split(tsvOutput, "\n")
	var words []string
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
	}

	avgConf := 0.8 // Default
	if confCount > 0 {
		avgConf = totalConf / float64(confCount) / 100.0 // Normalize to 0-1
	}

	// Reconstruct text - tesseract TSV preserves word order
	text := strings.Join(words, " ")
	// Clean up extra whitespace
	spaceRe := regexp.MustCompile(`\s+`)
	text = spaceRe.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	return text, avgConf
}

// PDFIterator implements ChunkIterator for PDF files
type PDFIterator struct {
	sourcePath  string
	config      map[string]any
	metadata    PDFMetadata
	currentPage int
	totalPages  int
}

// Next returns the next chunk of text from the PDF
func (it *PDFIterator) Next(ctx context.Context) (core.Chunk, error) {
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

	// Extract text from current page
	reader := &PDFReader{}
	pageText, method, confidence, err := reader.extractPageText(it.sourcePath, it.currentPage, it.config)
	if err != nil {
		return core.Chunk{}, fmt.Errorf("failed to extract text from page %d: %w", it.currentPage, err)
	}

	// DEBUG: Log chunk data being created
	log.Printf("[DEBUG] PDF Chunk Creation - Page %d (length=%d): %.200s...",
		it.currentPage, len(pageText), pageText)

	chunk := core.Chunk{
		Data: pageText,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:page:%d", filepath.Base(it.sourcePath), it.currentPage),
			ChunkType:   "pdf_page",
			SizeBytes:   int64(len(pageText)),
			ProcessedAt: time.Now(),
			ProcessedBy: "pdf_reader",
			Context: map[string]string{
				"page_number":       strconv.Itoa(it.currentPage),
				"total_pages":       strconv.Itoa(it.totalPages),
				"extraction_method": method,
				"confidence":        fmt.Sprintf("%.2f", confidence),
				"file_type":         "pdf",
				"pdf_title":         it.metadata.Title,
				"pdf_author":        it.metadata.Author,
			},
		},
	}

	// MEMORY FIX: Force GC and return memory to OS after each page extraction
	// runtime.GC() marks memory as free, debug.FreeOSMemory() returns it to OS
	runtime.GC()
	debug.FreeOSMemory()

	return chunk, nil
}

// Close releases PDF resources
func (it *PDFIterator) Close() error {
	// Nothing to close for PDF iterator
	return nil
}

// Reset restarts iteration from the beginning
func (it *PDFIterator) Reset() error {
	it.currentPage = 0
	return nil
}

// Progress returns iteration progress
func (it *PDFIterator) Progress() float64 {
	if it.totalPages == 0 {
		return 1.0
	}
	return float64(it.currentPage) / float64(it.totalPages)
}

// checkPageHasLargeImages checks if a page has large images that may need OCR
// Uses pdfimages -list to detect images meeting the configured thresholds
// Parameters:
//   - sourcePath: path to the PDF file
//   - pageNum: page number to check
//   - minWidth: minimum image width in pixels to consider "large" (default: 200)
//   - minHeight: minimum image height in pixels to consider "large" (default: 200)
//   - anyImage: if true, trigger OCR for ANY image regardless of size
func (r *PDFReader) checkPageHasLargeImages(sourcePath string, pageNum int, minWidth, minHeight int, anyImage bool) (bool, int, error) {
	cmd := exec.Command("pdfimages",
		"-f", strconv.Itoa(pageNum),
		"-l", strconv.Itoa(pageNum),
		"-list",
		sourcePath)

	output, err := cmd.Output()
	if err != nil {
		return false, 0, fmt.Errorf("pdfimages failed: %w", err)
	}

	// Apply defaults if not set
	if minWidth <= 0 {
		minWidth = 200
	}
	if minHeight <= 0 {
		minHeight = 200
	}

	// Parse pdfimages -list output
	// Format: page   num  type   width height color comp bpc  enc interp  object ID x-ppi y-ppi size ratio
	lines := strings.Split(string(output), "\n")
	imageCount := 0
	largeImageCount := 0

	for i, line := range lines {
		// Skip header lines (typically 2)
		if i < 2 {
			continue
		}

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		imageCount++

		// If anyImage is true, any image triggers OCR
		if anyImage {
			largeImageCount++
			continue
		}

		// Parse width and height to determine if it's a "large" image
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			width, _ := strconv.Atoi(fields[3])
			height, _ := strconv.Atoi(fields[4])

			// Consider an image "large" if it meets the configured thresholds
			// This indicates a significant image that may contain text
			if width >= minWidth && height >= minHeight {
				largeImageCount++
			}
		}
	}

	return largeImageCount > 0, imageCount, nil
}

// Helper functions
func ptrFloat64(f float64) *float64 {
	return &f
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
