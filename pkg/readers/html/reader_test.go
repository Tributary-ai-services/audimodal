package html

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jscharber/eAIIngest/pkg/core"
)

func TestHTMLReader_GetConfigSpec(t *testing.T) {
	reader := NewHTMLReader()
	specs := reader.GetConfigSpec()

	expectedSpecs := []string{
		"extract_text_only", "preserve_structure", "extract_links",
		"extract_images", "extract_metadata", "remove_scripts",
		"remove_styles", "chunk_by_element", "max_chunk_size",
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

func TestHTMLReader_ValidateConfig(t *testing.T) {
	reader := NewHTMLReader()

	tests := []struct {
		name        string
		config      map[string]any
		expectError bool
	}{
		{
			name: "valid config",
			config: map[string]any{
				"chunk_by_element": "paragraph",
				"max_chunk_size":   1000.0,
			},
			expectError: false,
		},
		{
			name: "invalid chunk_by_element",
			config: map[string]any{
				"chunk_by_element": "invalid",
			},
			expectError: true,
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

func TestHTMLReader_ExtractMetadata(t *testing.T) {
	reader := &HTMLReader{}
	
	testHTML := `
	<!DOCTYPE html>
	<html>
	<head>
		<title>Test Page Title</title>
		<meta name="description" content="Test description">
		<meta name="keywords" content="test, keywords, html">
		<meta name="author" content="Test Author">
		<meta charset="UTF-8">
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
		<meta property="og:title" content="OG Title">
		<meta property="og:description" content="OG Description">
	</head>
	<body>
		<h1>Test Heading</h1>
		<p>Test paragraph</p>
	</body>
	</html>
	`

	metadata := reader.extractHTMLMetadata(testHTML)

	if metadata.Title != "Test Page Title" {
		t.Errorf("Expected title 'Test Page Title', got '%s'", metadata.Title)
	}
	if metadata.Description != "Test description" {
		t.Errorf("Expected description 'Test description', got '%s'", metadata.Description)
	}
	if metadata.Keywords != "test, keywords, html" {
		t.Errorf("Expected keywords 'test, keywords, html', got '%s'", metadata.Keywords)
	}
	if metadata.Author != "Test Author" {
		t.Errorf("Expected author 'Test Author', got '%s'", metadata.Author)
	}
	if metadata.Charset != "UTF-8" {
		t.Errorf("Expected charset 'UTF-8', got '%s'", metadata.Charset)
	}
	if metadata.OGTitle != "OG Title" {
		t.Errorf("Expected OG title 'OG Title', got '%s'", metadata.OGTitle)
	}
}

func TestHTMLReader_AnalyzeStructure(t *testing.T) {
	reader := &HTMLReader{}
	
	testHTML := `
	<html>
	<body>
		<h1>Heading 1</h1>
		<h2>Heading 2</h2>
		<h3>Heading 3</h3>
		<p>Paragraph 1</p>
		<p>Paragraph 2</p>
		<a href="link1.html">Link 1</a>
		<a href="link2.html">Link 2</a>
		<img src="image.jpg">
		<table><tr><td>Table</td></tr></table>
		<form><input type="text"></form>
	</body>
	</html>
	`

	structure := reader.analyzeHTMLStructure(testHTML)

	if structure.HeadingCount != 3 {
		t.Errorf("Expected 3 headings, got %d", structure.HeadingCount)
	}
	if structure.ParagraphCount != 2 {
		t.Errorf("Expected 2 paragraphs, got %d", structure.ParagraphCount)
	}
	if structure.LinkCount != 2 {
		t.Errorf("Expected 2 links, got %d", structure.LinkCount)
	}
	if structure.ImageCount != 1 {
		t.Errorf("Expected 1 image, got %d", structure.ImageCount)
	}
	if !structure.HasTables {
		t.Error("Expected HasTables to be true")
	}
	if !structure.HasForms {
		t.Error("Expected HasForms to be true")
	}
}

func TestHTMLReader_StripTags(t *testing.T) {
	reader := &HTMLReader{}
	
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple paragraph",
			input:    "<p>Hello World</p>",
			expected: "Hello World",
		},
		{
			name:     "nested tags",
			input:    "<div><p>Hello <strong>World</strong></p></div>",
			expected: "Hello World",
		},
		{
			name:     "script removal",
			input:    "<p>Text</p><script>alert('test');</script><p>More</p>",
			expected: "Text More",
		},
		{
			name:     "style removal",
			input:    "<p>Text</p><style>body { color: red; }</style><p>More</p>",
			expected: "Text More",
		},
		{
			name:     "multiple spaces",
			input:    "<p>Text    with     spaces</p>",
			expected: "Text with spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reader.stripHTMLTags(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestHTMLReader_ParseElements(t *testing.T) {
	reader := &HTMLReader{}
	
	testHTML := `
	<html>
	<body>
		<h1>Main Heading</h1>
		<p>First paragraph</p>
		<p>Second paragraph</p>
		<h2>Subheading</h2>
		<p>Third paragraph</p>
	</body>
	</html>
	`

	config := map[string]any{
		"remove_scripts": true,
		"remove_styles":  true,
	}

	elements := reader.parseHTMLElements(testHTML, config)

	if len(elements) != 5 {
		t.Errorf("Expected 5 elements, got %d", len(elements))
	}

	// Check first element (h1)
	if elements[0].Type != "h1" {
		t.Errorf("Expected first element type 'h1', got '%s'", elements[0].Type)
	}
	if elements[0].Content != "Main Heading" {
		t.Errorf("Expected first element content 'Main Heading', got '%s'", elements[0].Content)
	}

	// Check paragraph count
	pCount := 0
	for _, elem := range elements {
		if elem.Type == "p" {
			pCount++
		}
	}
	if pCount != 3 {
		t.Errorf("Expected 3 paragraphs, got %d", pCount)
	}
}

func TestHTMLReader_CreateIterator(t *testing.T) {
	// Create temporary HTML file
	tempDir := t.TempDir()
	htmlPath := filepath.Join(tempDir, "test.html")
	
	testHTML := `
	<html>
	<head><title>Test</title></head>
	<body>
		<h1>Test Document</h1>
		<p>This is a test paragraph.</p>
		<p>This is another paragraph.</p>
	</body>
	</html>
	`
	
	err := os.WriteFile(htmlPath, []byte(testHTML), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	reader := NewHTMLReader()
	config := map[string]any{
		"extract_text_only": true,
		"chunk_by_element":  "paragraph",
	}

	iterator, err := reader.CreateIterator(context.Background(), htmlPath, config)
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
		if chunk.Metadata.ChunkType != "html_element" {
			t.Errorf("Expected chunk type 'html_element', got '%s'", chunk.Metadata.ChunkType)
		}
	}

	if chunkCount < 1 {
		t.Error("Expected at least 1 chunk")
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

func TestHTMLReader_GetBasicInfo(t *testing.T) {
	reader := NewHTMLReader()

	if reader.GetType() != "reader" {
		t.Errorf("Expected type 'reader', got '%s'", reader.GetType())
	}

	if reader.GetName() != "html_reader" {
		t.Errorf("Expected name 'html_reader', got '%s'", reader.GetName())
	}

	if reader.GetVersion() != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", reader.GetVersion())
	}

	if !reader.SupportsStreaming() {
		t.Error("Expected HTML reader to support streaming")
	}

	formats := reader.GetSupportedFormats()
	expectedFormats := []string{"html", "htm", "xhtml"}
	if len(formats) != len(expectedFormats) {
		t.Errorf("Expected %d supported formats, got %d", len(expectedFormats), len(formats))
	}
}

func BenchmarkHTMLReader_StripTags(b *testing.B) {
	reader := &HTMLReader{}
	testHTML := `<div class="content"><h1>Title</h1><p>This is a <strong>test</strong> paragraph with <a href="#">links</a> and <em>emphasis</em>.</p></div>`
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader.stripHTMLTags(testHTML)
	}
}

func BenchmarkHTMLReader_ParseElements(b *testing.B) {
	reader := &HTMLReader{}
	testHTML := `<html><body>` + 
		`<h1>Title</h1><p>Paragraph 1</p><p>Paragraph 2</p>` +
		`<h2>Subtitle</h2><p>Paragraph 3</p><p>Paragraph 4</p>` +
		`</body></html>`
	config := map[string]any{}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader.parseHTMLElements(testHTML, config)
	}
}