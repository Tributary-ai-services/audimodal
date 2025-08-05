package chunking

import (
	"context"
	"fmt"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
	"github.com/jscharber/eAIIngest/pkg/strategies/hybrid"
	"github.com/jscharber/eAIIngest/pkg/strategies/text"
)

// Chunker interface defines chunking functionality
type Chunker interface {
	Chunk(content string) ([]Chunk, error)
}

// Chunk represents a text chunk
type Chunk struct {
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata"`
	StartPos int               `json:"start_pos"`
	EndPos   int               `json:"end_pos"`
	ChunkNum int               `json:"chunk_num"`
}

// FixedSizeChunker implements fixed-size chunking
type FixedSizeChunker struct {
	chunkSize   int
	overlapSize int
	strategy    *text.FixedSizeStrategy
}

// NewFixedSizeChunker creates a new fixed-size chunker
func NewFixedSizeChunker(chunkSize, overlapSize int) *FixedSizeChunker {
	return &FixedSizeChunker{
		chunkSize:   chunkSize,
		overlapSize: overlapSize,
		strategy:    text.NewFixedSizeStrategy(),
	}
}

// Chunk splits content into fixed-size chunks
func (c *FixedSizeChunker) Chunk(content string) ([]Chunk, error) {
	ctx := context.Background()

	metadata := core.ChunkMetadata{
		SourcePath:  "memory",
		ChunkID:     "chunk",
		ChunkType:   "text",
		ProcessedAt: time.Now(),
		ProcessedBy: "fixed_size_chunker",
		Context: map[string]string{
			"chunk_size":   fmt.Sprintf("%d", c.chunkSize),
			"overlap_size": fmt.Sprintf("%d", c.overlapSize),
		},
	}

	coreChunks, err := c.strategy.ProcessChunk(ctx, content, metadata)
	if err != nil {
		return nil, err
	}

	// Convert core chunks to chunking chunks
	chunks := make([]Chunk, len(coreChunks))
	for i, coreChunk := range coreChunks {
		chunks[i] = Chunk{
			Content:  coreChunk.Data.(string),
			Metadata: coreChunk.Metadata.Context,
			ChunkNum: i + 1,
		}

		if coreChunk.Metadata.StartPosition != nil {
			chunks[i].StartPos = int(*coreChunk.Metadata.StartPosition)
		}
		if coreChunk.Metadata.EndPosition != nil {
			chunks[i].EndPos = int(*coreChunk.Metadata.EndPosition)
		}
	}

	return chunks, nil
}

// SentenceChunker implements sentence-based chunking
type SentenceChunker struct {
	maxChunkSize int
	overlapSize  int
	strategy     *text.FixedSizeStrategy
}

// NewSentenceChunker creates a new sentence-based chunker
func NewSentenceChunker(maxChunkSize, overlapSize int) *SentenceChunker {
	return &SentenceChunker{
		maxChunkSize: maxChunkSize,
		overlapSize:  overlapSize,
		strategy:     text.NewFixedSizeStrategy(),
	}
}

// Chunk splits content into sentence-based chunks
func (c *SentenceChunker) Chunk(content string) ([]Chunk, error) {
	ctx := context.Background()

	metadata := core.ChunkMetadata{
		SourcePath:  "memory",
		ChunkID:     "chunk",
		ChunkType:   "text",
		ProcessedAt: time.Now(),
		ProcessedBy: "sentence_chunker",
		Context: map[string]string{
			"chunk_size":         fmt.Sprintf("%d", c.maxChunkSize),
			"overlap_size":       fmt.Sprintf("%d", c.overlapSize),
			"split_on_sentences": "true",
		},
	}

	coreChunks, err := c.strategy.ProcessChunk(ctx, content, metadata)
	if err != nil {
		return nil, err
	}

	// Convert core chunks to chunking chunks
	chunks := make([]Chunk, len(coreChunks))
	for i, coreChunk := range coreChunks {
		chunks[i] = Chunk{
			Content:  coreChunk.Data.(string),
			Metadata: coreChunk.Metadata.Context,
			ChunkNum: i + 1,
		}

		if coreChunk.Metadata.StartPosition != nil {
			chunks[i].StartPos = int(*coreChunk.Metadata.StartPosition)
		}
		if coreChunk.Metadata.EndPosition != nil {
			chunks[i].EndPos = int(*coreChunk.Metadata.EndPosition)
		}
	}

	return chunks, nil
}

// SemanticChunker implements semantic-based chunking
type SemanticChunker struct {
	maxChunkSize        int
	similarityThreshold float64
	strategy            *hybrid.AdaptiveStrategy
}

// NewSemanticChunker creates a new semantic chunker
func NewSemanticChunker(maxChunkSize int, similarityThreshold float64) *SemanticChunker {
	return &SemanticChunker{
		maxChunkSize:        maxChunkSize,
		similarityThreshold: similarityThreshold,
		strategy:            hybrid.NewAdaptiveStrategy(),
	}
}

// Chunk splits content into semantically coherent chunks
func (c *SemanticChunker) Chunk(content string) ([]Chunk, error) {
	ctx := context.Background()

	metadata := core.ChunkMetadata{
		SourcePath:  "memory",
		ChunkID:     "chunk",
		ChunkType:   "text",
		ProcessedAt: time.Now(),
		ProcessedBy: "semantic_chunker",
		Context: map[string]string{
			"max_chunk_size":       fmt.Sprintf("%d", c.maxChunkSize),
			"similarity_threshold": fmt.Sprintf("%.2f", c.similarityThreshold),
			"chunking_mode":        "semantic",
		},
	}

	coreChunks, err := c.strategy.ProcessChunk(ctx, content, metadata)
	if err != nil {
		return nil, err
	}

	// Convert core chunks to chunking chunks
	chunks := make([]Chunk, len(coreChunks))
	for i, coreChunk := range coreChunks {
		chunks[i] = Chunk{
			Content:  coreChunk.Data.(string),
			Metadata: coreChunk.Metadata.Context,
			ChunkNum: i + 1,
		}

		if coreChunk.Metadata.StartPosition != nil {
			chunks[i].StartPos = int(*coreChunk.Metadata.StartPosition)
		}
		if coreChunk.Metadata.EndPosition != nil {
			chunks[i].EndPos = int(*coreChunk.Metadata.EndPosition)
		}
	}

	return chunks, nil
}

// Utility functions

// ChunkTextSimple provides a simple text chunking function
func ChunkTextSimple(text string, chunkSize, overlap int) []Chunk {
	chunker := NewFixedSizeChunker(chunkSize, overlap)
	chunks, _ := chunker.Chunk(text)
	return chunks
}

// ChunkBySentences provides sentence-based chunking
func ChunkBySentences(text string, maxChunkSize int) []Chunk {
	chunker := NewSentenceChunker(maxChunkSize, 0)
	chunks, _ := chunker.Chunk(text)
	return chunks
}

// ChunkSemantica provides semantic chunking
func ChunkSemantic(text string, maxChunkSize int, threshold float64) []Chunk {
	chunker := NewSemanticChunker(maxChunkSize, threshold)
	chunks, _ := chunker.Chunk(text)
	return chunks
}
