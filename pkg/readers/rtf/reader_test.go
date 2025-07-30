package rtf

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jscharber/eAIIngest/pkg/core"
)

func TestRTFReader_GetConfigSpec(t *testing.T) {
	reader := NewRTFReader()
	specs := reader.GetConfigSpec()

	expectedSpecs := []string{
		"extract_text_only", "preserve_formatting", "extract_metadata",
		"extract_tables", "max_chunk_size", "encoding",
		"skip_headers_footers", "extract_images",
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

func TestRTFReader_ValidateConfig(t *testing.T) {
	reader := NewRTFReader()

	tests := []struct {
		name        string
		config      map[string]any
		expectError bool
	}{
		{
			name: "valid config",
			config: map[string]any{
				"max_chunk_size": 1000.0,
				"encoding":       "windows-1252",
			},
			expectError: false,
		},
		{
			name: "invalid max_chunk_size too small",
			config: map[string]any{
				"max_chunk_size": 50.0,
			},
			expectError: true,
		},
		{
			name: "invalid max_chunk_size too large",
			config: map[string]any{
				"max_chunk_size": 20000.0,
			},
			expectError: true,
		},
		{
			name: "invalid encoding",
			config: map[string]any{
				"encoding": "invalid-encoding",
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

func TestRTFReader_ExtractMetadata(t *testing.T) {
	reader := &RTFReader{}
	
	testRTF := `{\rtf1\ansi\deff0 {\fonttbl{\f0 Times New Roman;}}
{\info
{\title Test Document Title}
{\author Test Author}
{\subject Test Subject}
{\keywords test, rtf, keywords}
{\comment This is a test comment}
{\generator Test RTF Generator}
{\creatim\yr2024\mo1\dy15\hr10\min30}
{\revtim\yr2024\mo1\dy20\hr14\min45}
}
\f0\fs24 This is test content.\par
}`

	metadata := reader.extractRTFMetadata(testRTF)

	if metadata.Title != "Test Document Title" {
		t.Errorf("Expected title 'Test Document Title', got '%s'", metadata.Title)
	}
	if metadata.Author != "Test Author" {
		t.Errorf("Expected author 'Test Author', got '%s'", metadata.Author)
	}
	if metadata.Subject != "Test Subject" {
		t.Errorf("Expected subject 'Test Subject', got '%s'", metadata.Subject)
	}
	if metadata.Keywords != "test, rtf, keywords" {
		t.Errorf("Expected keywords 'test, rtf, keywords', got '%s'", metadata.Keywords)
	}
	if metadata.Comment != "This is a test comment" {
		t.Errorf("Expected comment 'This is a test comment', got '%s'", metadata.Comment)
	}
	if metadata.Generator != "Test RTF Generator" {
		t.Errorf("Expected generator 'Test RTF Generator', got '%s'", metadata.Generator)
	}
	if metadata.CreatedDate != "2024-01-15" {
		t.Errorf("Expected created date '2024-01-15', got '%s'", metadata.CreatedDate)
	}
	if metadata.ModifiedDate != "2024-01-20" {
		t.Errorf("Expected modified date '2024-01-20', got '%s'", metadata.ModifiedDate)
	}
	if metadata.RTFVersion != "1" {
		t.Errorf("Expected RTF version '1', got '%s'", metadata.RTFVersion)
	}
}

func TestRTFReader_AnalyzeStructure(t *testing.T) {
	reader := &RTFReader{}
	
	testRTF := `{\rtf1\ansi\deff0
First paragraph.\par
Second paragraph.\par
Third paragraph.\par
{\trowd\cellx2000\cellx4000
\intbl Cell 1\cell Cell 2\cell\row}
{\field{\*\fldinst HYPERLINK "http://example.com"}{\fldrslt Link text}}
{\pict\pngblip\picw100\pich100 [image data]}
}`

	structure := reader.analyzeRTFStructure(testRTF)

	if structure.ParagraphCount != 3 {
		t.Errorf("Expected 3 paragraphs, got %d", structure.ParagraphCount)
	}
	if !structure.HasTables {
		t.Error("Expected HasTables to be true")
	}
	if !structure.HasHyperlinks {
		t.Error("Expected HasHyperlinks to be true")
	}
	if !structure.HasImages {
		t.Error("Expected HasImages to be true")
	}
}

func TestRTFReader_ExtractPlainText(t *testing.T) {
	reader := &RTFReader{}
	
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple text",
			input:    `{\rtf1\ansi Hello World}`,
			expected: "Hello World",
		},
		{
			name:     "paragraph",
			input:    `{\rtf1 First line.\par Second line.}`,
			expected: "First line.\nSecond line.",
		},
		{
			name:     "formatted text",
			input:    `{\rtf1 Normal {\b bold} {\i italic} text.}`,
			expected: "Normal bold italic text.",
		},
		{
			name:     "special characters",
			input:    `{\rtf1 Quote: \'92 and \'93quotes\'94}`,
			expected: "Quote: ' and \"quotes\"",
		},
		{
			name:     "info group removal",
			input:    `{\rtf1{\info{\title Test}}Content here}`,
			expected: "Content here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reader.extractPlainText(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestRTFReader_ParseParagraphs(t *testing.T) {
	reader := &RTFReader{}
	
	testRTF := `{\rtf1\ansi\deff0
{\b\fs28 Heading}\par
First paragraph with normal text.\par
{\i Second paragraph in italics.}\par
Third paragraph.\par
}`

	config := map[string]any{
		"preserve_formatting": true,
	}

	paragraphs := reader.parseRTFParagraphs(testRTF, config)

	if len(paragraphs) != 4 {
		t.Errorf("Expected 4 paragraphs, got %d", len(paragraphs))
	}

	// Check first paragraph (heading)
	if paragraphs[0].Number != 1 {
		t.Errorf("Expected first paragraph number 1, got %d", paragraphs[0].Number)
	}
	if !strings.Contains(paragraphs[0].Content, "Heading") {
		t.Errorf("Expected first paragraph to contain 'Heading', got '%s'", paragraphs[0].Content)
	}
	if formatting, ok := paragraphs[0].Formatting["bold"]; !ok || formatting != true {
		t.Error("Expected first paragraph to have bold formatting")
	}

	// Check italic paragraph
	if len(paragraphs) > 2 {
		if formatting, ok := paragraphs[2].Formatting["italic"]; !ok || formatting != true {
			t.Error("Expected third paragraph to have italic formatting")
		}
	}
}

func TestRTFReader_CreateIterator(t *testing.T) {
	// Create temporary RTF file
	tempDir := t.TempDir()
	rtfPath := filepath.Join(tempDir, "test.rtf")
	
	testRTF := `{\rtf1\ansi\deff0 {\fonttbl{\f0 Times New Roman;}}
{\info{\title Test RTF Document}}
\f0\fs24
This is the first paragraph.\par
This is the second paragraph.\par
This is the third paragraph.\par
}`
	
	err := os.WriteFile(rtfPath, []byte(testRTF), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	reader := NewRTFReader()
	config := map[string]any{
		"extract_text_only": true,
	}

	iterator, err := reader.CreateIterator(context.Background(), rtfPath, config)
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}

	// Test iteration
	chunkCount := 0
	for {
		chunk, err := iterator.Next(context.Background())
		if err == core.ErrIteratorExhausted {
			break
		}
		if err != nil {
			t.Errorf("Unexpected error during iteration: %v", err)
		}
		chunkCount++
		
		if chunk.Data == "" {
			t.Error("Expected non-empty chunk data")
		}
		if chunk.Metadata.ChunkType != "rtf_paragraph" {
			t.Errorf("Expected chunk type 'rtf_paragraph', got '%s'", chunk.Metadata.ChunkType)
		}
	}

	if chunkCount != 3 {
		t.Errorf("Expected 3 chunks, got %d", chunkCount)
	}

	// Test reset
	err = iterator.Reset()
	if err != nil {
		t.Errorf("Failed to reset iterator: %v", err)
	}

	// Test progress
	progress := iterator.Progress()
	if progress != 0.0 {
		t.Errorf("Expected progress 0.0 after reset, got %f", progress)
	}
}

func TestRTFReader_InvalidFile(t *testing.T) {
	reader := NewRTFReader()
	
	// Create temporary non-RTF file
	tempDir := t.TempDir()
	notRTFPath := filepath.Join(tempDir, "test.txt")
	
	err := os.WriteFile(notRTFPath, []byte("This is not an RTF file"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err = reader.DiscoverSchema(context.Background(), notRTFPath)
	if err == nil {
		t.Error("Expected error for non-RTF file, got none")
	}
	if !strings.Contains(err.Error(), "not a valid RTF file") {
		t.Errorf("Expected 'not a valid RTF file' error, got: %v", err)
	}
}

func TestRTFReader_GetBasicInfo(t *testing.T) {
	reader := NewRTFReader()

	if reader.GetType() != "reader" {
		t.Errorf("Expected type 'reader', got '%s'", reader.GetType())
	}

	if reader.GetName() != "rtf_reader" {
		t.Errorf("Expected name 'rtf_reader', got '%s'", reader.GetName())
	}

	if reader.GetVersion() != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", reader.GetVersion())
	}

	if !reader.SupportsStreaming() {
		t.Error("Expected RTF reader to support streaming")
	}

	formats := reader.GetSupportedFormats()
	if len(formats) != 1 || formats[0] != "rtf" {
		t.Errorf("Expected supported formats ['rtf'], got %v", formats)
	}
}