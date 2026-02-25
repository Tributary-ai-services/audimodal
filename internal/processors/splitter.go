// Package processors provides document processing pipeline components.
// This file implements the Splitter, which splits PDFs into per-page Kafka messages.
package processors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"gorm.io/gorm"

	"github.com/jscharber/audimodal/internal/database/models"
	"github.com/jscharber/audimodal/internal/services"
	"github.com/jscharber/audimodal/pkg/events"
	"github.com/jscharber/audimodal/pkg/readers/pdf/mapreduce"
)

// Splitter splits uploaded PDFs into per-page Kafka messages for distributed processing.
type Splitter struct {
	db         *gorm.DB
	s3Uploader *services.S3Uploader
	producer   *events.SimpleProducer
}

// NewSplitter creates a new Splitter.
func NewSplitter(db *gorm.DB, s3Uploader *services.S3Uploader, producer *events.SimpleProducer) *Splitter {
	return &Splitter{
		db:         db,
		s3Uploader: s3Uploader,
		producer:   producer,
	}
}

// SplitFile downloads a PDF from S3, classifies pages, and produces one Kafka message per page.
// It returns the processing job ID for tracking.
func (s *Splitter) SplitFile(ctx context.Context, tenantID, fileID uuid.UUID, s3Bucket, s3Key, tenantShortID string, redactionMode string, dlpScanEnabled bool) (uuid.UUID, error) {
	startTime := time.Now()

	// Step 1: Download PDF from S3 to temp file
	tempDir := os.TempDir()
	tempFile := filepath.Join(tempDir, fmt.Sprintf("split-%s-%s.pdf", fileID.String()[:8], uuid.New().String()[:8]))
	defer os.Remove(tempFile)

	log.Printf("[Splitter] Downloading PDF from s3://%s/%s to %s", s3Bucket, s3Key, tempFile)
	if err := s.s3Uploader.DownloadFile(ctx, s3Bucket, s3Key, tempFile); err != nil {
		return uuid.Nil, fmt.Errorf("failed to download PDF from S3: %w", err)
	}

	// Step 2: Get page count
	totalPages, err := mapreduce.GetPageCount(ctx, tempFile)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to get page count: %w", err)
	}
	log.Printf("[Splitter] PDF has %d pages", totalPages)

	if totalPages == 0 {
		return uuid.Nil, fmt.Errorf("PDF has 0 pages")
	}

	// Step 3: Create processing job record (status: splitting)
	jobMetadata := models.JobMetadata{}
	if redactionMode != "" {
		jobMetadata["redaction_mode"] = redactionMode
	}
	if dlpScanEnabled {
		jobMetadata["dlp_scan_enabled"] = true
	}

	job := models.ProcessingJob{
		TenantID:   tenantID,
		FileID:     fileID,
		TotalPages: totalPages,
		Status:     models.JobStatusSplitting,
		Metadata:   jobMetadata,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := s.db.Create(&job).Error; err != nil {
		return uuid.Nil, fmt.Errorf("failed to create processing job: %w", err)
	}
	log.Printf("[Splitter] Created processing job %s for file %s (%d pages)", job.ID.String(), fileID.String(), totalPages)

	// Step 4: Classify all pages
	classifier := mapreduce.NewPageClassifier(mapreduce.DefaultExtractionConfig())
	classifications, err := classifier.ClassifyAllPages(ctx, tempFile, totalPages)
	if err != nil {
		s.markJobFailed(job.ID, fmt.Sprintf("classification failed: %v", err))
		return job.ID, fmt.Errorf("classification failed: %w", err)
	}

	// Step 5: Create page result records and produce Kafka messages
	pageResults := make([]models.PipelinePageResult, totalPages)
	kafkaMessages := make([]kafka.Message, 0, totalPages)

	defaultConfig := events.DefaultExtractionConfig()

	for i := 0; i < totalPages; i++ {
		pageNum := i + 1

		// Create page result record (status: pending)
		pageResults[i] = models.PipelinePageResult{
			JobID:      job.ID,
			TenantID:   tenantID,
			FileID:     fileID,
			PageNumber: pageNum,
			Status:     models.PipelinePagePending,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		// Get classification for this page
		classification := "unknown"
		if i < len(classifications) && classifications[i] != nil {
			classification = string(classifications[i].Classification)
		}

		// Create Kafka message
		pageJob := events.PageJobMessage{
			MessageID:      uuid.New().String(),
			TenantID:       tenantID.String(),
			FileID:         fileID.String(),
			JobID:          job.ID.String(),
			PageNumber:     pageNum,
			TotalPages:     totalPages,
			S3Bucket:       s3Bucket,
			S3Key:          s3Key,
			Classification: classification,
			Config:         defaultConfig,
			ProducedAt:     time.Now(),
		}

		msgData, err := json.Marshal(pageJob)
		if err != nil {
			s.markJobFailed(job.ID, fmt.Sprintf("failed to marshal page job %d: %v", pageNum, err))
			return job.ID, fmt.Errorf("failed to marshal page job: %w", err)
		}

		kafkaMessages = append(kafkaMessages, kafka.Message{
			Topic: events.TopicPageJobs,
			Key:   []byte(pageJob.PartitionKey()),
			Value: msgData,
			Headers: []kafka.Header{
				{Key: "event-type", Value: []byte("page-job")},
				{Key: "job-id", Value: []byte(job.ID.String())},
				{Key: "page-number", Value: []byte(fmt.Sprintf("%d", pageNum))},
			},
		})
	}

	// Batch insert page results
	if err := s.db.CreateInBatches(&pageResults, 100).Error; err != nil {
		s.markJobFailed(job.ID, fmt.Sprintf("failed to create page results: %v", err))
		return job.ID, fmt.Errorf("failed to create page results: %w", err)
	}

	// Step 6: Produce Kafka messages
	log.Printf("[Splitter] Producing %d page job messages to %s", len(kafkaMessages), events.TopicPageJobs)
	if err := s.producer.WriteMessages(ctx, kafkaMessages...); err != nil {
		s.markJobFailed(job.ID, fmt.Sprintf("failed to produce messages: %v", err))
		return job.ID, fmt.Errorf("failed to produce page job messages: %w", err)
	}

	// Step 7: Update job status to "processing"
	s.db.Model(&models.ProcessingJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
		"status":     models.JobStatusProcessing,
		"updated_at": time.Now(),
	})

	splitDuration := time.Since(startTime)
	log.Printf("[Splitter] File %s split into %d page jobs in %v", fileID.String(), totalPages, splitDuration)

	return job.ID, nil
}

// markJobFailed marks a processing job as failed
func (s *Splitter) markJobFailed(jobID uuid.UUID, errMsg string) {
	s.db.Model(&models.ProcessingJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"status":        models.JobStatusFailed,
		"error_message": errMsg,
		"updated_at":    time.Now(),
	})
}
