package mapreduce

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/google/uuid"
)

func checkOCRTools(t *testing.T) {
	tools := []string{"pdftotext", "pdftoppm", "tesseract"}
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("Skipping test: %s not available", tool)
		}
	}
}

func TestNewPageExtractor(t *testing.T) {
	// Test with nil config
	e := NewPageExtractor(nil)
	if e == nil {
		t.Fatal("NewPageExtractor returned nil")
	}
	if e.config == nil {
		t.Error("config is nil")
	}

	// Test with custom config
	config := &ExtractionConfig{
		OCRLanguage: "spa",
		OCRDPI:      200,
	}
	e = NewPageExtractor(config)
	if e.config.OCRLanguage != "spa" {
		t.Errorf("OCRLanguage = %s, expected spa", e.config.OCRLanguage)
	}
}

func TestGetWorkerID(t *testing.T) {
	id := getWorkerID()
	if id == "" {
		t.Error("getWorkerID returned empty string")
	}

	// Should contain hostname and PID
	hostname, _ := os.Hostname()
	if hostname != "" {
		// ID format: hostname-pid
		if len(id) <= len(hostname) {
			t.Error("worker ID seems malformed")
		}
	}
}

func TestHashContent(t *testing.T) {
	tests := []struct {
		content string
	}{
		{""},
		{"Hello, World!"},
		{"This is a longer text with special chars: !@#$%^&*()"},
		{"Unicode: 你好世界 🌍"},
	}

	for _, tt := range tests {
		hash := hashContent(tt.content)
		if hash == "" {
			t.Errorf("hashContent(%q) returned empty string", tt.content)
		}
		// MD5 hash should be 32 characters (hex encoded)
		if len(hash) != 32 {
			t.Errorf("hashContent(%q) length = %d, expected 32", tt.content, len(hash))
		}
	}

	// Same content should produce same hash
	hash1 := hashContent("test")
	hash2 := hashContent("test")
	if hash1 != hash2 {
		t.Error("hashContent produced different hashes for same content")
	}

	// Different content should produce different hash
	hash3 := hashContent("different")
	if hash1 == hash3 {
		t.Error("hashContent produced same hash for different content")
	}
}

func TestParseTesseractTSV(t *testing.T) {
	// Test with valid TSV
	tsv := `level	page_num	block_num	par_num	line_num	word_num	left	top	width	height	conf	text
1	1	1	1	1	1	100	100	50	20	95.5	Hello
1	1	1	1	1	2	160	100	60	20	90.0	World
`
	text, conf, err := parseTesseractTSV(tsv)
	if err != nil {
		t.Fatalf("parseTesseractTSV failed: %v", err)
	}

	if text != "Hello World" {
		t.Errorf("text = %q, expected 'Hello World'", text)
	}

	// Confidence should be average of (95.5 + 90.0) / 100 / 2 = 0.9275
	expectedConf := (95.5 + 90.0) / 2 / 100
	if conf < expectedConf-0.01 || conf > expectedConf+0.01 {
		t.Errorf("confidence = %.4f, expected ~%.4f", conf, expectedConf)
	}
}

func TestParseTesseractTSV_EmptyInput(t *testing.T) {
	_, _, err := parseTesseractTSV("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestParseTesseractTSV_HeaderOnly(t *testing.T) {
	tsv := `level	page_num	block_num	par_num	line_num	word_num	left	top	width	height	conf	text
`
	text, conf, err := parseTesseractTSV(tsv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "" {
		t.Errorf("expected empty text, got %q", text)
	}
	if conf != 0.5 { // Default confidence when no words
		t.Errorf("expected default confidence 0.5, got %.2f", conf)
	}
}

func TestExtractPage_TextOnly(t *testing.T) {
	checkOCRTools(t)

	testPDF := os.Getenv("TEST_PDF_PATH")
	if testPDF == "" {
		t.Skip("Skipping: TEST_PDF_PATH not set")
	}

	e := NewPageExtractor(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	job := &PageJob{
		JobID:          uuid.New().String(),
		TenantID:       uuid.Nil,
		FileID:         uuid.Nil,
		PDFPath:        testPDF,
		PageNumber:     1,
		TotalPages:     1,
		Classification: ClassTextOnly,
	}

	result, err := e.ExtractPage(ctx, job)
	if err != nil {
		t.Fatalf("ExtractPage failed: %v", err)
	}

	if result.Status != StatusCompleted {
		t.Errorf("status = %s, expected %s", result.Status, StatusCompleted)
	}

	if result.ExtractionMethod != MethodPDFToText {
		t.Errorf("method = %s, expected %s", result.ExtractionMethod, MethodPDFToText)
	}

	if result.ContentHash == "" {
		t.Error("ContentHash is empty")
	}
}

func TestExtractPage_Empty(t *testing.T) {
	e := NewPageExtractor(nil)
	ctx := context.Background()

	job := &PageJob{
		JobID:          uuid.New().String(),
		TenantID:       uuid.Nil,
		FileID:         uuid.Nil,
		PDFPath:        "/nonexistent.pdf",
		PageNumber:     1,
		TotalPages:     1,
		Classification: ClassEmpty,
	}

	result, err := e.ExtractPage(ctx, job)
	if err != nil {
		t.Fatalf("ExtractPage failed: %v", err)
	}

	if result.Status != StatusSkipped {
		t.Errorf("status = %s, expected %s", result.Status, StatusSkipped)
	}

	if result.ExtractionMethod != MethodSkipped {
		t.Errorf("method = %s, expected %s", result.ExtractionMethod, MethodSkipped)
	}

	if result.Content != "" {
		t.Error("Content should be empty for skipped page")
	}
}

func TestExtractPage_InvalidPDF(t *testing.T) {
	checkOCRTools(t)

	e := NewPageExtractor(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	job := &PageJob{
		JobID:          uuid.New().String(),
		TenantID:       uuid.Nil,
		FileID:         uuid.Nil,
		PDFPath:        "/nonexistent.pdf",
		PageNumber:     1,
		TotalPages:     1,
		Classification: ClassTextOnly,
	}

	result, err := e.ExtractPage(ctx, job)
	// Should not return error, but status should be failed
	if err != nil {
		t.Fatalf("ExtractPage returned error: %v", err)
	}

	if result.Status != StatusFailed {
		t.Errorf("status = %s, expected %s", result.Status, StatusFailed)
	}

	if result.Error == "" {
		t.Error("Error message should be set for failed extraction")
	}
}

func TestExtractSinglePage(t *testing.T) {
	checkOCRTools(t)

	testPDF := os.Getenv("TEST_PDF_PATH")
	if testPDF == "" {
		t.Skip("Skipping: TEST_PDF_PATH not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := ExtractSinglePage(ctx, testPDF, 1, ClassTextOnly, nil)
	if err != nil {
		t.Fatalf("ExtractSinglePage failed: %v", err)
	}

	if result.PageNumber != 1 {
		t.Errorf("PageNumber = %d, expected 1", result.PageNumber)
	}
}

func BenchmarkHashContent(b *testing.B) {
	content := "This is a test content that needs to be hashed repeatedly for benchmarking purposes."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hashContent(content)
	}
}

func BenchmarkParseTesseractTSV(b *testing.B) {
	tsv := `level	page_num	block_num	par_num	line_num	word_num	left	top	width	height	conf	text
1	1	1	1	1	1	100	100	50	20	95.5	Hello
1	1	1	1	1	2	160	100	60	20	90.0	World
1	1	1	1	2	1	100	130	50	20	88.0	This
1	1	1	1	2	2	160	130	60	20	92.0	is
1	1	1	1	2	3	230	130	40	20	91.0	a
1	1	1	1	2	4	280	130	50	20	89.0	test
`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseTesseractTSV(tsv)
	}
}
