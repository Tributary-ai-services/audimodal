package mapreduce

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/jscharber/audimodal/internal/database/models"
	"github.com/jscharber/audimodal/pkg/core"
)

// Reducer aggregates page results into chunks for the existing pipeline
type Reducer struct {
	db     *gorm.DB
	config *ReducerConfig
}

// ReducerConfig contains configuration for the reducer
type ReducerConfig struct {
	PreservePageBoundaries bool   // Keep page boundaries as chunk boundaries
	SourcePath             string // Original file path for metadata
}

// NewReducer creates a new reducer
func NewReducer(db *gorm.DB, config *ReducerConfig) *Reducer {
	if config == nil {
		config = &ReducerConfig{
			PreservePageBoundaries: true,
		}
	}
	return &Reducer{
		db:     db,
		config: config,
	}
}

// Reduce combines page results into core.Chunk objects for the pipeline
func (r *Reducer) Reduce(ctx context.Context, fileID uuid.UUID) ([]core.Chunk, error) {
	// Fetch all completed page results
	pageResults, err := r.getCompletedPageResults(ctx, fileID)
	if err != nil {
		return nil, err
	}

	if len(pageResults) == 0 {
		return nil, fmt.Errorf("no completed page results found for file %s", fileID)
	}

	// Convert to core.Chunk objects
	if r.config.PreservePageBoundaries {
		return r.reduceWithPageBoundaries(pageResults)
	}
	return r.reduceCombined(pageResults)
}

// ReduceFromResults reduces directly from ProcessingResult (no DB)
func (r *Reducer) ReduceFromResults(result *ProcessingResult) ([]core.Chunk, error) {
	if result == nil || len(result.PageResults) == 0 {
		return nil, fmt.Errorf("no page results to reduce")
	}

	if r.config.PreservePageBoundaries {
		return r.reduceWithPageBoundariesFromResults(result.PageResults)
	}
	return r.reduceCombinedFromResults(result.PageResults)
}

// getCompletedPageResults fetches completed page results from database
func (r *Reducer) getCompletedPageResults(ctx context.Context, fileID uuid.UUID) ([]models.PageResult, error) {
	var pageResults []models.PageResult
	err := r.db.WithContext(ctx).
		Where("file_id = ? AND processing_status = ?", fileID, models.PageStatusCompleted).
		Order("page_number ASC").
		Find(&pageResults).Error

	if err != nil {
		return nil, fmt.Errorf("failed to fetch page results: %w", err)
	}

	return pageResults, nil
}

// reduceWithPageBoundaries creates one chunk per page
func (r *Reducer) reduceWithPageBoundaries(pageResults []models.PageResult) ([]core.Chunk, error) {
	chunks := make([]core.Chunk, 0, len(pageResults))

	for _, pr := range pageResults {
		if pr.Content == "" {
			continue
		}

		chunk := core.Chunk{
			Data: pr.Content,
			Metadata: core.ChunkMetadata{
				SourcePath:  r.config.SourcePath,
				ChunkID:     fmt.Sprintf("page_%d", pr.PageNumber),
				ChunkType:   "pdf_page",
				SizeBytes:   int64(len(pr.Content)),
				ProcessedAt: time.Now(),
				ProcessedBy: "mapreduce_reducer",
				Context: map[string]string{
					"page_number":       strconv.Itoa(pr.PageNumber),
					"total_pages":       strconv.Itoa(pr.TotalPages),
					"extraction_method": pr.ExtractionMethod,
					"classification":    pr.Classification,
				},
			},
		}

		if pr.OCRConfidence != nil {
			chunk.Metadata.Context["ocr_confidence"] = fmt.Sprintf("%.2f", *pr.OCRConfidence)
		}

		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// reduceCombined combines all pages into a single text and creates metadata
func (r *Reducer) reduceCombined(pageResults []models.PageResult) ([]core.Chunk, error) {
	var combined strings.Builder
	var pageBreaks []int // Track where each page starts

	for _, pr := range pageResults {
		if pr.Content == "" {
			continue
		}
		pageBreaks = append(pageBreaks, combined.Len())
		combined.WriteString(pr.Content)
		combined.WriteString("\n\n") // Page separator
	}

	if combined.Len() == 0 {
		return nil, fmt.Errorf("no content to reduce")
	}

	// Create a single chunk with all content
	totalPages := 0
	if len(pageResults) > 0 {
		totalPages = pageResults[0].TotalPages
	}

	chunk := core.Chunk{
		Data: combined.String(),
		Metadata: core.ChunkMetadata{
			SourcePath:  r.config.SourcePath,
			ChunkID:     "full_document",
			ChunkType:   "pdf_document",
			SizeBytes:   int64(combined.Len()),
			ProcessedAt: time.Now(),
			ProcessedBy: "mapreduce_reducer",
			Context: map[string]string{
				"total_pages":     strconv.Itoa(totalPages),
				"pages_with_text": strconv.Itoa(len(pageBreaks)),
				"extraction_type": "map_reduce",
			},
		},
	}

	return []core.Chunk{chunk}, nil
}

// reduceWithPageBoundariesFromResults creates chunks from PageResult slice
func (r *Reducer) reduceWithPageBoundariesFromResults(pageResults []*PageResult) ([]core.Chunk, error) {
	chunks := make([]core.Chunk, 0, len(pageResults))

	for _, pr := range pageResults {
		if pr == nil || pr.Content == "" {
			continue
		}

		chunk := core.Chunk{
			Data: pr.Content,
			Metadata: core.ChunkMetadata{
				SourcePath:  r.config.SourcePath,
				ChunkID:     fmt.Sprintf("page_%d", pr.PageNumber),
				ChunkType:   "pdf_page",
				SizeBytes:   int64(len(pr.Content)),
				ProcessedAt: time.Now(),
				ProcessedBy: "mapreduce_reducer",
				Context: map[string]string{
					"page_number":       strconv.Itoa(pr.PageNumber),
					"total_pages":       strconv.Itoa(pr.TotalPages),
					"extraction_method": string(pr.ExtractionMethod),
					"classification":    string(pr.Classification),
					"ocr_confidence":    fmt.Sprintf("%.2f", pr.OCRConfidence),
				},
			},
		}

		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// reduceCombinedFromResults combines PageResult slice into single chunk
func (r *Reducer) reduceCombinedFromResults(pageResults []*PageResult) ([]core.Chunk, error) {
	var combined strings.Builder
	pagesWithText := 0
	totalPages := 0

	for _, pr := range pageResults {
		if pr == nil {
			continue
		}
		if totalPages == 0 {
			totalPages = pr.TotalPages
		}
		if pr.Content != "" {
			combined.WriteString(pr.Content)
			combined.WriteString("\n\n")
			pagesWithText++
		}
	}

	if combined.Len() == 0 {
		return nil, fmt.Errorf("no content to reduce")
	}

	chunk := core.Chunk{
		Data: combined.String(),
		Metadata: core.ChunkMetadata{
			SourcePath:  r.config.SourcePath,
			ChunkID:     "full_document",
			ChunkType:   "pdf_document",
			SizeBytes:   int64(combined.Len()),
			ProcessedAt: time.Now(),
			ProcessedBy: "mapreduce_reducer",
			Context: map[string]string{
				"total_pages":     strconv.Itoa(totalPages),
				"pages_with_text": strconv.Itoa(pagesWithText),
				"extraction_type": "map_reduce",
			},
		},
	}

	return []core.Chunk{chunk}, nil
}

// GetTextContent returns the combined text content from all pages
func (r *Reducer) GetTextContent(ctx context.Context, fileID uuid.UUID) (string, error) {
	pageResults, err := r.getCompletedPageResults(ctx, fileID)
	if err != nil {
		return "", err
	}

	var combined strings.Builder
	for _, pr := range pageResults {
		if pr.Content != "" {
			combined.WriteString(pr.Content)
			combined.WriteString("\n\n")
		}
	}

	return strings.TrimSpace(combined.String()), nil
}

// GetPageTexts returns a map of page number to text content
func (r *Reducer) GetPageTexts(ctx context.Context, fileID uuid.UUID) (map[int]string, error) {
	pageResults, err := r.getCompletedPageResults(ctx, fileID)
	if err != nil {
		return nil, err
	}

	result := make(map[int]string, len(pageResults))
	for _, pr := range pageResults {
		result[pr.PageNumber] = pr.Content
	}

	return result, nil
}
