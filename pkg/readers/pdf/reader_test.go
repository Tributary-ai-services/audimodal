package pdf

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jscharber/audimodal/pkg/core"
)

// requireBinaries skips the test unless every named external binary is on PATH.
// The PDF reader shells out to poppler-utils and tesseract; tests that exercise
// the real extraction path cannot run without them.
func requireBinaries(t *testing.T, names ...string) {
	t.Helper()
	var missing []string
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Skipf("skipping: required external binaries not found on PATH: %s (install poppler-utils)",
			strings.Join(missing, ", "))
	}
}

// writeTestPDF builds a minimal, valid PDF containing one text-bearing page per
// entry in pageTexts and writes it to the test's temp directory. The file is
// generated rather than committed so the fixture stays readable and diffable.
func writeTestPDF(t *testing.T, pageTexts ...string) string {
	t.Helper()

	var objects []string
	kids := make([]string, 0, len(pageTexts))
	for i := range pageTexts {
		kids = append(kids, fmt.Sprintf("%d 0 R", 5+2*i))
	}
	objects = append(objects,
		"<< /Type /Catalog /Pages 2 0 R >>",
		fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pageTexts)),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		"<< /Title (Audimodal Fixture) /Author (audimodal tests) >>",
	)
	for i, text := range pageTexts {
		stream := fmt.Sprintf("BT /F1 24 Tf 72 700 Td (%s) Tj ET\n", text)
		objects = append(objects,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] "+
				"/Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", 6+2*i),
			fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream))
	}

	var buf strings.Builder
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, body := range objects {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R /Info 4 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(objects)+1, xrefOffset)

	path := filepath.Join(t.TempDir(), "test.pdf")
	if err := os.WriteFile(path, []byte(buf.String()), 0o644); err != nil {
		t.Fatalf("failed to write test PDF: %v", err)
	}
	return path
}

func TestPDFReader_GetConfigSpec(t *testing.T) {
	reader := NewPDFReader()
	specs := reader.GetConfigSpec()

	// Verify we have expected config specs
	expectedSpecs := []string{
		"processing_mode", "mapreduce_page_threshold", "mapreduce_workers",
		"extract_mode", "ocr_language", "ocr_dpi", "include_images",
		"preserve_layout", "extract_metadata", "password", "max_pages",
		"skip_images_larger_than_mb", "ocr_any_image", "ocr_image_min_width",
		"ocr_image_min_height",
	}

	if len(specs) != len(expectedSpecs) {
		t.Errorf("Expected %d config specs, got %d", len(expectedSpecs), len(specs))
	}

	specMap := make(map[string]core.ConfigSpec)
	for _, spec := range specs {
		specMap[spec.Name] = spec
	}

	for _, expected := range expectedSpecs {
		if _, exists := specMap[expected]; !exists {
			t.Errorf("Missing config spec: %s", expected)
		}
	}
}

func TestPDFReader_ValidateConfig(t *testing.T) {
	reader := NewPDFReader()

	tests := []struct {
		name        string
		config      map[string]any
		expectError bool
	}{
		{
			name: "valid config",
			config: map[string]any{
				"extract_mode": "auto",
				"ocr_language": "eng",
				"ocr_dpi":      300.0,
			},
			expectError: false,
		},
		{
			name: "invalid extract_mode",
			config: map[string]any{
				"extract_mode": "invalid",
			},
			expectError: true,
		},
		{
			name: "invalid ocr_language",
			config: map[string]any{
				"ocr_language": "invalid",
			},
			expectError: true,
		},
		{
			name: "invalid ocr_dpi",
			config: map[string]any{
				"ocr_dpi": 100.0, // Too low
			},
			expectError: true,
		},
		{
			name:        "empty config",
			config:      map[string]any{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := reader.ValidateConfig(tt.config)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestPDFReader_TestConnection(t *testing.T) {
	reader := NewPDFReader()
	ctx := context.Background()

	config := map[string]any{
		"extract_mode": "auto",
		"ocr_language": "eng",
	}

	result := reader.TestConnection(ctx, config)

	// Test should pass even without actual dependencies for this test
	if result.Latency <= 0 {
		t.Error("Expected positive latency")
	}

	if result.Details == nil {
		t.Error("Expected details in connection test result")
	}
}

func TestPDFReader_GetBasicInfo(t *testing.T) {
	reader := NewPDFReader()

	if reader.GetType() != "reader" {
		t.Errorf("Expected type 'reader', got '%s'", reader.GetType())
	}

	if reader.GetName() != "pdf_reader" {
		t.Errorf("Expected name 'pdf_reader', got '%s'", reader.GetName())
	}

	if reader.GetVersion() != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", reader.GetVersion())
	}

	if !reader.SupportsStreaming() {
		t.Error("Expected PDF reader to support streaming")
	}

	formats := reader.GetSupportedFormats()
	if len(formats) != 1 || formats[0] != "pdf" {
		t.Errorf("Expected supported formats ['pdf'], got %v", formats)
	}
}

func TestPDFMetadata_Extraction(t *testing.T) {
	requireBinaries(t, "pdfinfo")

	reader := &PDFReader{}
	path := writeTestPDF(t, "Audimodal test page 1", "Audimodal test page 2")

	metadata, err := reader.extractPDFMetadata(path)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if metadata.PageCount != 2 {
		t.Errorf("Expected page count 2, got %d", metadata.PageCount)
	}

	if metadata.PDFVersion == "" {
		t.Error("Expected PDF version to be set")
	}

	if metadata.Title != "Audimodal Fixture" {
		t.Errorf("Expected title 'Audimodal Fixture', got '%s'", metadata.Title)
	}

	if metadata.Encrypted {
		t.Error("Expected fixture PDF to be unencrypted")
	}
}

func TestPDFReader_ExtractPageText(t *testing.T) {
	requireBinaries(t, "pdftotext")

	reader := &PDFReader{}

	config := map[string]any{
		"extract_mode": "auto",
	}
	path := writeTestPDF(t, "Audimodal test page 1", "Audimodal test page 2")

	text, method, confidence, err := reader.extractPageText(path, 2, config)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !strings.Contains(text, "Audimodal test page 2") {
		t.Errorf("Expected text of page 2, got %q", text)
	}

	if method != "text" && method != "ocr" {
		t.Errorf("Expected method 'text' or 'ocr', got '%s'", method)
	}

	if confidence < 0 || confidence > 1 {
		t.Errorf("Expected confidence between 0 and 1, got %f", confidence)
	}
}

func TestPDFIterator_Lifecycle(t *testing.T) {
	requireBinaries(t, "pdftotext")

	path := writeTestPDF(t, "Audimodal test page 1", "Audimodal test page 2", "Audimodal test page 3")
	iterator := &PDFIterator{
		sourcePath:  path,
		config:      map[string]any{},
		metadata:    PDFMetadata{PageCount: 3},
		currentPage: 0,
		totalPages:  3,
	}

	ctx := context.Background()

	// Test initial progress
	if iterator.Progress() != 0.0 {
		t.Errorf("Expected initial progress 0.0, got %f", iterator.Progress())
	}

	// Test iteration
	for i := 1; i <= 3; i++ {
		chunk, err := iterator.Next(ctx)
		if err != nil {
			t.Errorf("Unexpected error on iteration %d: %v", i, err)
		}

		data, ok := chunk.Data.(string)
		if !ok {
			t.Errorf("Expected string chunk data on iteration %d, got %T", i, chunk.Data)
		}
		expectedText := fmt.Sprintf("Audimodal test page %d", i)
		if !strings.Contains(data, expectedText) {
			t.Errorf("Expected chunk data to contain %q on iteration %d, got %q", expectedText, i, data)
		}

		if chunk.Metadata.ChunkType != "pdf_page" {
			t.Errorf("Expected chunk type 'pdf_page', got '%s'", chunk.Metadata.ChunkType)
		}

		expectedProgress := float64(i) / 3.0
		if iterator.Progress() != expectedProgress {
			t.Errorf("Expected progress %f, got %f", expectedProgress, iterator.Progress())
		}
	}

	// Test exhaustion
	_, err := iterator.Next(ctx)
	if err != core.ErrIteratorExhausted {
		t.Errorf("Expected ErrIteratorExhausted, got %v", err)
	}

	// Test reset
	err = iterator.Reset()
	if err != nil {
		t.Errorf("Unexpected error on reset: %v", err)
	}

	if iterator.currentPage != 0 {
		t.Errorf("Expected current page 0 after reset, got %d", iterator.currentPage)
	}

	// Test close
	err = iterator.Close()
	if err != nil {
		t.Errorf("Unexpected error on close: %v", err)
	}
}

func TestPDFIterator_MaxPages(t *testing.T) {
	// Test max_pages configuration
	iterator := &PDFIterator{
		sourcePath:  "/mock/path/test.pdf",
		config:      map[string]any{"max_pages": 2.0},
		metadata:    PDFMetadata{PageCount: 5},
		currentPage: 0,
		totalPages:  5,
	}

	ctx := context.Background()

	// Should only process 2 pages
	for i := 1; i <= 2; i++ {
		_, err := iterator.Next(ctx)
		if err != nil {
			t.Errorf("Unexpected error on iteration %d: %v", i, err)
		}
	}

	// Third call should exhaust
	_, err := iterator.Next(ctx)
	if err != core.ErrIteratorExhausted {
		t.Errorf("Expected ErrIteratorExhausted after max_pages, got %v", err)
	}
}

func TestPDFIterator_ContextCancellation(t *testing.T) {
	iterator := &PDFIterator{
		sourcePath:  "/mock/path/test.pdf",
		config:      map[string]any{},
		metadata:    PDFMetadata{PageCount: 3},
		currentPage: 0,
		totalPages:  3,
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Should return context error
	_, err := iterator.Next(ctx)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func BenchmarkPDFReader_ExtractPageText(b *testing.B) {
	reader := &PDFReader{}
	config := map[string]any{
		"extract_mode": "auto",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, err := reader.extractPageText("/mock/path/test.pdf", 1, config)
		if err != nil {
			b.Errorf("Unexpected error: %v", err)
		}
	}
}

func BenchmarkPDFIterator_Next(b *testing.B) {
	iterator := &PDFIterator{
		sourcePath:  "/mock/path/test.pdf",
		config:      map[string]any{},
		metadata:    PDFMetadata{PageCount: 1000},
		currentPage: 0,
		totalPages:  1000,
	}

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if iterator.currentPage >= iterator.totalPages {
			iterator.Reset()
		}
		_, err := iterator.Next(ctx)
		if err != nil && err != core.ErrIteratorExhausted {
			b.Errorf("Unexpected error: %v", err)
		}
	}
}
