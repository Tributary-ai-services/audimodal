package services

import (
	"context"
	"fmt"

	"github.com/jscharber/audimodal/internal/database/models"
)

// ChunkContentService reads chunk content from S3
type ChunkContentService struct {
	s3 *S3Uploader
}

// NewChunkContentService creates a new chunk content service
func NewChunkContentService(s3 *S3Uploader) *ChunkContentService {
	return &ChunkContentService{s3: s3}
}

// GetContent reads the full content of a chunk from S3
func (s *ChunkContentService) GetContent(ctx context.Context, chunk *models.Chunk) (string, error) {
	if chunk.S3Bucket == "" || chunk.S3Key == "" {
		return "", fmt.Errorf("chunk %s has no S3 reference", chunk.ID)
	}
	return s.s3.GetChunkContent(ctx, chunk.S3Bucket, chunk.S3Key)
}
