package pdf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
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
			"extract_mode":   config["extract_mode"],
			"ocr_language":   config["ocr_language"],
			"dependencies":   len(dependencies) == 0,
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
	// This is a simplified check - in a real implementation,
	// you would use exec.LookPath(cmd) to verify the command exists
	return true // Assume dependencies are available for now
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
			"file_size":    stat.Size(),
			"page_count":   metadata.PageCount,
			"title":        metadata.Title,
			"author":       metadata.Author,
			"subject":      metadata.Subject,
			"creator":      metadata.Creator,
			"producer":     metadata.Producer,
			"creation_date": metadata.CreationDate,
			"modified_date": metadata.ModificationDate,
			"encrypted":    metadata.Encrypted,
			"pdf_version":  metadata.PDFVersion,
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
					"page_text":        sampleText,
					"extraction_method": method,
					"confidence":       confidence,
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
	metadata, err := r.extractPDFMetadata(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract PDF metadata: %w", err)
	}

	iterator := &PDFIterator{
		sourcePath:     sourcePath,
		config:         strategyConfig,
		metadata:       metadata,
		currentPage:    0,
		totalPages:     metadata.PageCount,
	}

	return iterator, nil
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

// extractPDFMetadata extracts metadata from PDF file
func (r *PDFReader) extractPDFMetadata(sourcePath string) (PDFMetadata, error) {
	// In a real implementation, this would use a PDF library like:
	// - github.com/ledongthuc/pdf for pure Go
	// - github.com/unidoc/unipdf for commercial use
	// - Or call pdfinfo command from poppler-utils
	
	// For now, return mock metadata
	return PDFMetadata{
		PageCount:        10, // Would be extracted from actual PDF
		Title:            "Sample Document",
		Author:           "Unknown",
		Subject:          "",
		Creator:          "Unknown Application",
		Producer:         "Unknown Producer",
		CreationDate:     time.Now().Format("2006-01-02"),
		ModificationDate: time.Now().Format("2006-01-02"),
		Encrypted:        false,
		PDFVersion:       "1.4",
	}, nil
}

// extractPageText extracts text from a specific page
func (r *PDFReader) extractPageText(sourcePath string, pageNum int, config map[string]any) (string, string, float64, error) {
	// Get extraction mode
	mode := "auto"
	if m, ok := config["extract_mode"]; ok {
		if str, ok := m.(string); ok {
			mode = str
		}
	}

	// In a real implementation, this would:
	// 1. Try pdftotext first for text extraction
	// 2. Fall back to OCR if text extraction fails or mode is "ocr"
	// 3. Use tesseract for OCR processing
	
	// Mock implementation
	extractedText := fmt.Sprintf("This is sample text from page %d of the PDF document.\nIt would contain the actual extracted content.", pageNum)
	extractionMethod := "text"
	confidence := 1.0

	// Simulate OCR if mode requires it
	if mode == "ocr" || (mode == "auto" && len(extractedText) < 50) {
		extractionMethod = "ocr"
		confidence = 0.95 // Mock OCR confidence
	}

	return extractedText, extractionMethod, confidence, nil
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