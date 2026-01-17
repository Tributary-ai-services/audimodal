package mapreduce

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewReducer(t *testing.T) {
	// Test with nil config
	r := NewReducer(nil, nil)
	if r == nil {
		t.Fatal("NewReducer returned nil")
	}
	if r.config == nil {
		t.Error("config is nil")
	}
	if !r.config.PreservePageBoundaries {
		t.Error("PreservePageBoundaries should default to true")
	}

	// Test with custom config
	config := &ReducerConfig{
		PreservePageBoundaries: false,
		SourcePath:             "/test/file.pdf",
	}
	r = NewReducer(nil, config)
	if r.config.PreservePageBoundaries {
		t.Error("PreservePageBoundaries should be false")
	}
	if r.config.SourcePath != "/test/file.pdf" {
		t.Errorf("SourcePath = %s, expected /test/file.pdf", r.config.SourcePath)
	}
}

func TestReduceFromResults_Empty(t *testing.T) {
	r := NewReducer(nil, nil)

	// Test with nil result
	_, err := r.ReduceFromResults(nil)
	if err == nil {
		t.Error("expected error for nil result")
	}

	// Test with empty page results
	result := &ProcessingResult{
		PageResults: []*PageResult{},
	}
	_, err = r.ReduceFromResults(result)
	if err == nil {
		t.Error("expected error for empty page results")
	}
}

func TestReduceFromResults_PreservePageBoundaries(t *testing.T) {
	r := NewReducer(nil, &ReducerConfig{
		PreservePageBoundaries: true,
		SourcePath:             "/test/file.pdf",
	})

	result := &ProcessingResult{
		FileID:     uuid.New(),
		TotalPages: 3,
		PageResults: []*PageResult{
			{
				PageNumber:       1,
				TotalPages:       3,
				Content:          "Page 1 content",
				Classification:   ClassTextOnly,
				ExtractionMethod: MethodPDFToText,
				Status:           StatusCompleted,
			},
			{
				PageNumber:       2,
				TotalPages:       3,
				Content:          "Page 2 content",
				Classification:   ClassTextOnly,
				ExtractionMethod: MethodPDFToText,
				Status:           StatusCompleted,
			},
			{
				PageNumber:       3,
				TotalPages:       3,
				Content:          "", // Empty page
				Classification:   ClassEmpty,
				ExtractionMethod: MethodSkipped,
				Status:           StatusSkipped,
			},
		},
	}

	chunks, err := r.ReduceFromResults(result)
	if err != nil {
		t.Fatalf("ReduceFromResults failed: %v", err)
	}

	// Should have 2 chunks (page 3 is empty)
	if len(chunks) != 2 {
		t.Errorf("got %d chunks, expected 2", len(chunks))
	}

	// Check chunk metadata
	for i, chunk := range chunks {
		if chunk.Metadata.ChunkType != "pdf_page" {
			t.Errorf("chunk %d: ChunkType = %s, expected pdf_page", i, chunk.Metadata.ChunkType)
		}
		if chunk.Metadata.ProcessedBy != "mapreduce_reducer" {
			t.Errorf("chunk %d: ProcessedBy = %s, expected mapreduce_reducer", i, chunk.Metadata.ProcessedBy)
		}
		if chunk.Metadata.Context["total_pages"] != "3" {
			t.Errorf("chunk %d: total_pages = %s, expected 3", i, chunk.Metadata.Context["total_pages"])
		}
	}

	// Check first chunk content
	if data, ok := chunks[0].Data.(string); ok {
		if data != "Page 1 content" {
			t.Errorf("chunk 0 content = %s, expected 'Page 1 content'", data)
		}
	} else {
		t.Error("chunk 0 data is not a string")
	}
}

func TestReduceFromResults_Combined(t *testing.T) {
	r := NewReducer(nil, &ReducerConfig{
		PreservePageBoundaries: false,
		SourcePath:             "/test/file.pdf",
	})

	result := &ProcessingResult{
		FileID:     uuid.New(),
		TotalPages: 2,
		PageResults: []*PageResult{
			{
				PageNumber:       1,
				TotalPages:       2,
				Content:          "Page 1",
				Classification:   ClassTextOnly,
				ExtractionMethod: MethodPDFToText,
				Status:           StatusCompleted,
			},
			{
				PageNumber:       2,
				TotalPages:       2,
				Content:          "Page 2",
				Classification:   ClassTextOnly,
				ExtractionMethod: MethodPDFToText,
				Status:           StatusCompleted,
			},
		},
	}

	chunks, err := r.ReduceFromResults(result)
	if err != nil {
		t.Fatalf("ReduceFromResults failed: %v", err)
	}

	// Should have 1 combined chunk
	if len(chunks) != 1 {
		t.Errorf("got %d chunks, expected 1", len(chunks))
	}

	// Check chunk type
	if chunks[0].Metadata.ChunkType != "pdf_document" {
		t.Errorf("ChunkType = %s, expected pdf_document", chunks[0].Metadata.ChunkType)
	}

	// Check combined content
	if data, ok := chunks[0].Data.(string); ok {
		if data != "Page 1\n\nPage 2\n\n" {
			t.Errorf("combined content = %q", data)
		}
	}
}

func TestReduceFromResults_NilPages(t *testing.T) {
	r := NewReducer(nil, nil)

	result := &ProcessingResult{
		TotalPages: 3,
		PageResults: []*PageResult{
			{PageNumber: 1, Content: "Page 1", Status: StatusCompleted},
			nil, // nil page
			{PageNumber: 3, Content: "Page 3", Status: StatusCompleted},
		},
	}

	chunks, err := r.ReduceFromResults(result)
	if err != nil {
		t.Fatalf("ReduceFromResults failed: %v", err)
	}

	// Should have 2 chunks (skipping nil)
	if len(chunks) != 2 {
		t.Errorf("got %d chunks, expected 2", len(chunks))
	}
}

func TestReduceFromResults_AllEmpty(t *testing.T) {
	r := NewReducer(nil, &ReducerConfig{
		PreservePageBoundaries: false,
	})

	result := &ProcessingResult{
		TotalPages: 2,
		PageResults: []*PageResult{
			{PageNumber: 1, Content: "", Status: StatusSkipped},
			{PageNumber: 2, Content: "", Status: StatusSkipped},
		},
	}

	_, err := r.ReduceFromResults(result)
	if err == nil {
		t.Error("expected error when all pages are empty")
	}
}

func TestReduceMetadata(t *testing.T) {
	r := NewReducer(nil, &ReducerConfig{
		PreservePageBoundaries: true,
		SourcePath:             "/path/to/document.pdf",
	})

	result := &ProcessingResult{
		TotalPages: 1,
		PageResults: []*PageResult{
			{
				PageNumber:       1,
				TotalPages:       1,
				Content:          "Test content",
				Classification:   ClassScanned,
				ExtractionMethod: MethodOCR,
				OCRConfidence:    0.85,
				Status:           StatusCompleted,
			},
		},
	}

	chunks, err := r.ReduceFromResults(result)
	if err != nil {
		t.Fatalf("ReduceFromResults failed: %v", err)
	}

	chunk := chunks[0]

	// Check metadata
	if chunk.Metadata.SourcePath != "/path/to/document.pdf" {
		t.Errorf("SourcePath = %s", chunk.Metadata.SourcePath)
	}
	if chunk.Metadata.Context["extraction_method"] != "ocr" {
		t.Errorf("extraction_method = %s", chunk.Metadata.Context["extraction_method"])
	}
	if chunk.Metadata.Context["classification"] != "scanned" {
		t.Errorf("classification = %s", chunk.Metadata.Context["classification"])
	}
	if chunk.Metadata.Context["ocr_confidence"] != "0.85" {
		t.Errorf("ocr_confidence = %s", chunk.Metadata.Context["ocr_confidence"])
	}
}

func BenchmarkReduceFromResults(b *testing.B) {
	r := NewReducer(nil, &ReducerConfig{
		PreservePageBoundaries: true,
	})

	// Create a 100-page result
	pageResults := make([]*PageResult, 100)
	for i := 0; i < 100; i++ {
		pageResults[i] = &PageResult{
			PageNumber:       i + 1,
			TotalPages:       100,
			Content:          "This is the content for page " + string(rune('0'+i%10)),
			Classification:   ClassTextOnly,
			ExtractionMethod: MethodPDFToText,
			Status:           StatusCompleted,
		}
	}

	result := &ProcessingResult{
		TotalPages:  100,
		PageResults: pageResults,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.ReduceFromResults(result)
	}
}
