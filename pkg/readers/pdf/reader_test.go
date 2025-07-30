package pdf

import (
	"context"
	"testing"

	"github.com/jscharber/eAIIngest/pkg/core"
)

func TestPDFReader_GetConfigSpec(t *testing.T) {
	reader := NewPDFReader()
	specs := reader.GetConfigSpec()

	// Verify we have expected config specs
	expectedSpecs := []string{
		"extract_mode", "ocr_language", "ocr_dpi", "include_images",
		"preserve_layout", "extract_metadata", "password", "max_pages",
		"skip_images_larger_than_mb",
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
			name:        "valid config",
			config:      map[string]any{
				"extract_mode": "auto",
				"ocr_language": "eng",
				"ocr_dpi":      300.0,
			},
			expectError: false,
		},
		{
			name:        "invalid extract_mode",
			config:      map[string]any{
				"extract_mode": "invalid",
			},
			expectError: true,
		},
		{
			name:        "invalid ocr_language",
			config:      map[string]any{
				"ocr_language": "invalid",
			},
			expectError: true,
		},
		{
			name:        "invalid ocr_dpi",
			config:      map[string]any{
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

func TestPDFMetadata_MockExtraction(t *testing.T) {
	reader := &PDFReader{}
	
	// Test with mock file path
	metadata, err := reader.extractPDFMetadata("/mock/path/test.pdf")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if metadata.PageCount <= 0 {
		t.Error("Expected positive page count")
	}

	if metadata.PDFVersion == "" {
		t.Error("Expected PDF version to be set")
	}
}

func TestPDFReader_ExtractPageText(t *testing.T) {
	reader := &PDFReader{}
	
	config := map[string]any{
		"extract_mode": "auto",
	}

	text, method, confidence, err := reader.extractPageText("/mock/path/test.pdf", 1, config)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if text == "" {
		t.Error("Expected non-empty text")
	}

	if method != "text" && method != "ocr" {
		t.Errorf("Expected method 'text' or 'ocr', got '%s'", method)
	}

	if confidence < 0 || confidence > 1 {
		t.Errorf("Expected confidence between 0 and 1, got %f", confidence)
	}
}

func TestPDFIterator_Lifecycle(t *testing.T) {
	// Create mock iterator
	iterator := &PDFIterator{
		sourcePath:  "/mock/path/test.pdf",
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

		if chunk.Data == "" {
			t.Errorf("Expected non-empty chunk data on iteration %d", i)
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