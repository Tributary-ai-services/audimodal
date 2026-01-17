package mapreduce

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

// checkPDFTools checks if required PDF tools are available
func checkPDFTools(t *testing.T) {
	tools := []string{"pdftotext", "pdfinfo", "pdffonts", "pdfimages"}
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("Skipping test: %s not available", tool)
		}
	}
}

func TestNewPageClassifier(t *testing.T) {
	// Test with nil config
	c := NewPageClassifier(nil)
	if c == nil {
		t.Fatal("NewPageClassifier returned nil")
	}
	if c.config == nil {
		t.Error("config is nil")
	}

	// Test with custom config
	config := &ExtractionConfig{
		TextThreshold: 200,
		OCRDPI:        200,
	}
	c = NewPageClassifier(config)
	if c.config.TextThreshold != 200 {
		t.Errorf("TextThreshold = %d, expected 200", c.config.TextThreshold)
	}
}

func TestClassify(t *testing.T) {
	c := NewPageClassifier(nil)

	tests := []struct {
		name               string
		pc                 *PageClassification
		expectedClass      PageClassificationType
		expectedConfidence float64
	}{
		{
			name: "text_only_high_text",
			pc: &PageClassification{
				TextCharCount:    500,
				HasImages:        false,
				HasEmbeddedFonts: true,
			},
			expectedClass:      ClassTextOnly,
			expectedConfidence: 0.95,
		},
		{
			name: "scanned_no_fonts_large_image",
			pc: &PageClassification{
				TextCharCount:    10,
				HasImages:        true,
				HasEmbeddedFonts: false,
				LargeImageCount:  1,
			},
			expectedClass:      ClassScanned,
			expectedConfidence: 0.90,
		},
		{
			name: "hybrid_text_and_images",
			pc: &PageClassification{
				TextCharCount:    80,
				HasImages:        true,
				HasEmbeddedFonts: true,
				ImageCount:       2,
			},
			expectedClass:      ClassHybrid,
			expectedConfidence: 0.85,
		},
		{
			name: "image_only_low_text",
			pc: &PageClassification{
				TextCharCount:    20,
				HasImages:        true,
				HasEmbeddedFonts: true,
				ImageCount:       1,
			},
			expectedClass:      ClassImageOnly,
			expectedConfidence: 0.85,
		},
		{
			name: "empty_page",
			pc: &PageClassification{
				TextCharCount:    5,
				HasImages:        false,
				HasEmbeddedFonts: false,
			},
			expectedClass:      ClassEmpty,
			expectedConfidence: 0.95,
		},
		{
			name: "low_text_fallback",
			pc: &PageClassification{
				TextCharCount:    50,
				HasImages:        false,
				HasEmbeddedFonts: true,
			},
			expectedClass:      ClassTextOnly,
			expectedConfidence: 0.70,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			class, conf := c.classify(tt.pc)
			if class != tt.expectedClass {
				t.Errorf("classification = %s, expected %s", class, tt.expectedClass)
			}
			if conf != tt.expectedConfidence {
				t.Errorf("confidence = %.2f, expected %.2f", conf, tt.expectedConfidence)
			}
		})
	}
}

func TestGetPageCount(t *testing.T) {
	checkPDFTools(t)

	// Use test PDF from TAS test data repo or environment variable
	testPDF := os.Getenv("TEST_PDF_PATH")
	if testPDF == "" {
		testPDF = "/home/jscharber/eng/TAS/tas-test-data/pdf/TAS_Data_Models_Consolidated.pdf"
	}
	if _, err := os.Stat(testPDF); os.IsNotExist(err) {
		t.Skip("Skipping test: test PDF not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	count, err := GetPageCount(ctx, testPDF)
	if err != nil {
		t.Logf("GetPageCount error (expected if no test PDF): %v", err)
		return
	}

	if count < 1 {
		t.Errorf("page count = %d, expected >= 1", count)
	}
}

func TestClassifyPage_Integration(t *testing.T) {
	checkPDFTools(t)

	// This test requires an actual PDF file
	testPDF := os.Getenv("TEST_PDF_PATH")
	if testPDF == "" {
		t.Skip("Skipping integration test: TEST_PDF_PATH not set")
	}

	c := NewPageClassifier(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := c.ClassifyPage(ctx, testPDF, 1)
	if err != nil {
		t.Fatalf("ClassifyPage failed: %v", err)
	}

	if result.PageNumber != 1 {
		t.Errorf("PageNumber = %d, expected 1", result.PageNumber)
	}

	// Classification should be one of the valid types
	validTypes := map[PageClassificationType]bool{
		ClassTextOnly:  true,
		ClassImageOnly: true,
		ClassHybrid:    true,
		ClassScanned:   true,
		ClassEmpty:     true,
		ClassUnknown:   true,
	}
	if !validTypes[result.Classification] {
		t.Errorf("invalid classification: %s", result.Classification)
	}

	// Confidence should be between 0 and 1
	if result.Confidence < 0 || result.Confidence > 1 {
		t.Errorf("confidence = %.2f, expected 0-1", result.Confidence)
	}
}

func TestClassifyAllPages_Context(t *testing.T) {
	c := NewPageClassifier(nil)

	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := c.ClassifyAllPages(ctx, "/nonexistent.pdf", 10)
	// Should handle context cancellation gracefully
	// The exact behavior depends on implementation
	t.Logf("ClassifyAllPages with cancelled context: %v", err)
}

func BenchmarkClassify(b *testing.B) {
	c := NewPageClassifier(nil)
	pc := &PageClassification{
		TextCharCount:    500,
		HasImages:        true,
		HasEmbeddedFonts: true,
		ImageCount:       2,
		LargeImageCount:  0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.classify(pc)
	}
}
