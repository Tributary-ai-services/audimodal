package text

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jscharber/audimodal/pkg/core"
)

// BenchmarkProcessChunk_LargeText benchmarks processing of large text
func BenchmarkProcessChunk_LargeText(b *testing.B) {
	strategy := NewFixedSizeStrategy()

	// Simulate a page of text (about 3000 chars)
	pageText := strings.Repeat("This is a sentence with some content. ", 100)

	metadata := core.ChunkMetadata{
		SourcePath:  "/test/file.pdf",
		ChunkID:     "test-chunk-1",
		ProcessedAt: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := strategy.ProcessChunk(context.Background(), pageText, metadata)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkProcessChunk_ManyPages simulates processing 100 pages
func BenchmarkProcessChunk_ManyPages(b *testing.B) {
	strategy := NewFixedSizeStrategy()

	// Simulate page text
	pageText := strings.Repeat("This is a sentence with some content. ", 100)

	metadata := core.ChunkMetadata{
		SourcePath:  "/test/file.pdf",
		ChunkID:     "test-chunk",
		ProcessedAt: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Process 100 pages
		for p := 0; p < 100; p++ {
			_, err := strategy.ProcessChunk(context.Background(), pageText, metadata)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}
