// Assembler: Consumes page results from Kafka, tracks completion per file,
// and when all pages are done: writes chunk content to S3 + metadata to PostgreSQL.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/jscharber/audimodal/internal/database/models"
	"github.com/jscharber/audimodal/internal/services"
	"github.com/jscharber/audimodal/pkg/dlp"
	"github.com/jscharber/audimodal/pkg/dlp/scanner"
	"github.com/jscharber/audimodal/pkg/dlp/types"
	"github.com/jscharber/audimodal/pkg/events"
	"github.com/jscharber/audimodal/pkg/metrics"
	"github.com/jscharber/audimodal/pkg/preprocessing"
)

// Package-level pipeline metrics
var pipelineMetrics *metrics.PipelineMetrics

const (
	healthPort         = 8081
	staleJobTimeout    = 4 * time.Hour
	staleJobCheckDelay = 5 * time.Minute
)

func main() {
	log.Println("[Assembler] Starting Assembler...")

	// Initialize metrics
	registry := metrics.NewRegistry()
	registry.AddGlobalLabel("service", "audimodal-assembler")
	pipelineMetrics = metrics.NewPipelineMetrics(registry)

	bootstrapServers := getEnv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092")
	groupID := getEnv("KAFKA_GROUP_ID", "audimodal-assemblers")

	// Connect to PostgreSQL
	db, err := connectDB()
	if err != nil {
		log.Fatalf("[Assembler] Failed to connect to database: %v", err)
	}

	// Create S3 uploader
	s3Uploader, err := services.NewS3Uploader()
	if err != nil {
		log.Fatalf("[Assembler] Failed to create S3 uploader: %v", err)
	}

	// Create Kafka producer for events
	producer, err := events.NewSimpleProducer(events.SimpleProducerConfig{
		BootstrapServers: bootstrapServers,
		ClientID:         "audimodal-assembler",
	})
	if err != nil {
		log.Fatalf("[Assembler] Failed to create Kafka producer: %v", err)
	}
	defer producer.Close()

	// Create Kafka consumer for page results
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{bootstrapServers},
		GroupID:     groupID,
		Topic:       events.TopicPageResults,
		StartOffset: kafka.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
		MaxWait:     3 * time.Second,
	})
	defer reader.Close()

	// Start health check server with metrics
	go startHealthServer(registry)

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		log.Printf("[Assembler] Received signal %v, shutting down...", sig)
		cancel()
	}()

	// Start stale job detector
	go detectStaleJobs(ctx, db)

	log.Printf("[Assembler] Consumer started. GroupID: %s, Bootstrap: %s", groupID, bootstrapServers)

	// Main consumer loop
	for {
		select {
		case <-ctx.Done():
			log.Println("[Assembler] Context cancelled, exiting consumer loop")
			return
		default:
			msg, err := reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("[Assembler] Error fetching message: %v", err)
				time.Sleep(time.Second)
				continue
			}

			processPageResult(ctx, msg, db, s3Uploader, producer)

			if err := reader.CommitMessages(ctx, msg); err != nil {
				log.Printf("[Assembler] Error committing offset: %v", err)
			}
		}
	}
}

func processPageResult(ctx context.Context, msg kafka.Message, db *gorm.DB, s3Uploader *services.S3Uploader, producer *events.SimpleProducer) {
	var result events.PageResultMessage
	if err := json.Unmarshal(msg.Value, &result); err != nil {
		log.Printf("[Assembler] Failed to unmarshal page result: %v", err)
		return
	}

	log.Printf("[Assembler] Received page %d/%d for file %s (status: %s)",
		result.PageNumber, result.TotalPages, result.FileID, result.Status)

	pipelineMetrics.KafkaMessagesConsumed.Inc()

	jobID, _ := uuid.Parse(result.JobID)
	fileID, _ := uuid.Parse(result.FileID)
	tenantID, _ := uuid.Parse(result.TenantID)

	// If there's inline content, store it to S3 for later assembly
	contentS3Key := result.ContentS3Key
	if result.Status == "completed" && result.Content != "" && contentS3Key == "" {
		// Store page content to S3
		s3Bucket := services.GetTenantBucket(services.GetTenantShortID(tenantID.String()))
		pageKey := fmt.Sprintf("%s/pages/page_%d.txt", fileID.String(), result.PageNumber)

		if err := s3Uploader.UploadChunkContent(ctx, s3Bucket, pageKey, result.Content); err != nil {
			log.Printf("[Assembler] Failed to store page %d content to S3: %v", result.PageNumber, err)
		} else {
			contentS3Key = pageKey
		}
	}

	// Update page result record
	pageUpdate := map[string]any{
		"status":     result.Status,
		"updated_at": time.Now(),
	}
	if contentS3Key != "" {
		pageUpdate["content_s3_key"] = contentS3Key
	}
	if result.ExtractionMethod != "" {
		pageUpdate["extraction_method"] = result.ExtractionMethod
	}
	if result.OCRConfidence > 0 {
		pageUpdate["ocr_confidence"] = result.OCRConfidence
	}
	if result.ProcessingDurationMs > 0 {
		pageUpdate["processing_duration_ms"] = result.ProcessingDurationMs
	}
	if result.Error != "" {
		pageUpdate["error_message"] = result.Error
	}
	if result.ErrorType != "" {
		pageUpdate["error_type"] = result.ErrorType
	}

	db.Model(&models.PipelinePageResult{}).
		Where("job_id = ? AND page_number = ?", jobID, result.PageNumber).
		Updates(pageUpdate)

	// Increment completed or failed pages on the processing job
	if result.Status == "completed" {
		db.Model(&models.ProcessingJob{}).Where("id = ?", jobID).
			UpdateColumn("completed_pages", gorm.Expr("completed_pages + 1")).
			UpdateColumn("updated_at", time.Now())
	} else {
		db.Model(&models.ProcessingJob{}).Where("id = ?", jobID).
			UpdateColumn("failed_pages", gorm.Expr("failed_pages + 1")).
			UpdateColumn("updated_at", time.Now())
	}

	// Check if all pages are done
	var job models.ProcessingJob
	if err := db.Where("id = ?", jobID).First(&job).Error; err != nil {
		log.Printf("[Assembler] Failed to load job %s: %v", jobID.String(), err)
		return
	}

	if !job.IsComplete() {
		return // Still waiting for more pages
	}

	log.Printf("[Assembler] All pages done for job %s (completed: %d, failed: %d, total: %d)",
		jobID.String(), job.CompletedPages, job.FailedPages, job.TotalPages)

	// Determine final status based on failure rate
	assembleFile(ctx, db, s3Uploader, producer, &job, tenantID, fileID)
}

func assembleFile(ctx context.Context, db *gorm.DB, s3Uploader *services.S3Uploader,
	producer *events.SimpleProducer, job *models.ProcessingJob, tenantID, fileID uuid.UUID) {

	// Update job to assembling
	db.Model(job).Updates(map[string]any{
		"status":     models.JobStatusAssembling,
		"updated_at": time.Now(),
	})

	// Check failure rate
	if job.FailureRate() >= 0.2 {
		errMsg := fmt.Sprintf("too many failed pages: %d/%d (%.0f%%)",
			job.FailedPages, job.TotalPages, job.FailureRate()*100)
		log.Printf("[Assembler] %s for job %s", errMsg, job.ID.String())

		now := time.Now()
		db.Model(job).Updates(map[string]any{
			"status":        models.JobStatusFailed,
			"error_message": errMsg,
			"completed_at":  &now,
			"updated_at":    now,
		})
		updateFileStatus(db, tenantID, fileID, "error", errMsg)
		processingDuration := now.Sub(job.CreatedAt).Milliseconds()
		db.Model(&models.File{}).Where("id = ? AND tenant_id = ?", fileID, tenantID).
			Updates(map[string]any{
				"processed_at":        &now,
				"processing_duration": processingDuration,
			})
		return
	}

	// Load all completed page results (from pipeline_page_results, with content from Kafka messages)
	var pageResults []models.PipelinePageResult
	db.Where("job_id = ? AND status = ?", job.ID, models.PipelinePageCompleted).
		Order("page_number ASC").Find(&pageResults)

	// We need the actual page content. Since page results come from Kafka messages,
	// the content is stored in the PageResultMessage. We load the page results
	// and reconstruct content by re-querying or using stored content_s3_key.
	// For now, the OCR worker puts content directly in the Kafka message.
	// The assembler should have cached it or we store it during processPageResult.
	// Let's store page content to S3 during processPageResult.

	// Get the tenant bucket
	var file models.File
	if err := db.Where("id = ? AND tenant_id = ?", fileID, tenantID).First(&file).Error; err != nil {
		log.Printf("[Assembler] Failed to load file %s: %v", fileID.String(), err)
		markJobFailed(db, job.ID, fmt.Sprintf("file not found: %v", err))
		return
	}

	// Parse bucket from file URL
	s3Bucket := ""
	if strings.HasPrefix(file.URL, "s3://") {
		s3URL := strings.TrimPrefix(file.URL, "s3://")
		if idx := strings.Index(s3URL, "/"); idx > 0 {
			s3Bucket = s3URL[:idx]
		}
	}
	if s3Bucket == "" {
		s3Bucket = services.GetTenantBucket(services.GetTenantShortID(tenantID.String()))
	}

	// Read redaction settings from job metadata
	redactionMode := ""
	dlpScanEnabled := false
	if job.Metadata != nil {
		if rm, ok := job.Metadata["redaction_mode"]; ok {
			if rmStr, ok := rm.(string); ok {
				redactionMode = rmStr
			}
		}
		if de, ok := job.Metadata["dlp_scan_enabled"]; ok {
			if deBool, ok := de.(bool); ok {
				dlpScanEnabled = deBool
			}
		}
	}

	// Initialize DLP service and redaction engine if redaction is enabled
	var dlpService *dlp.DLPService
	var redactionEngine *scanner.BasicRedactionEngine
	redactionEnabled := dlpScanEnabled && redactionMode != "" && redactionMode != "none"
	if redactionEnabled {
		dlpService = dlp.NewDLPService(nil)
		redactionEngine = scanner.NewBasicRedactionEngine()
		log.Printf("[Assembler] DLP redaction enabled for job %s (mode: %s)", job.ID.String(), redactionMode)
	}

	// Create chunks: one chunk per completed page
	chunks := make([]models.Chunk, 0, len(pageResults))
	chunkCount := 0

	for i, pr := range pageResults {
		// Get the content from the S3 key if already stored, or skip
		content := ""
		if pr.ContentS3Key != "" {
			var err error
			content, err = s3Uploader.GetChunkContent(ctx, s3Bucket, pr.ContentS3Key)
			if err != nil {
				log.Printf("[Assembler] Failed to read page content from S3 for page %d: %v", pr.PageNumber, err)
				continue
			}
		}

		if content == "" {
			continue
		}

		// Sanitize content to ensure valid UTF-8 before any DB insertion
		content = preprocessing.SanitizeUTF8String(content)

		// Apply DLP scanning and redaction before storing
		chunkDLPStatus := "pending"
		piiDetected := false
		var dlpScanResultJSON string

		if redactionEnabled && dlpService != nil {
			scanResponse, scanErr := dlpService.ScanContent(ctx, &dlp.DLPScanRequest{
				TenantID:  tenantID,
				ContentID: fmt.Sprintf("page_%d", pr.PageNumber),
				Content:   content,
				Metadata: map[string]string{
					"file_id":     fileID.String(),
					"page_number": strconv.Itoa(pr.PageNumber),
					"pipeline":    "kafka",
				},
			})
			if scanErr != nil {
				log.Printf("[Assembler] DLP scan failed for page %d: %v", pr.PageNumber, scanErr)
			} else {
				if scanResponse.ScanResult != nil {
					if resultBytes, jsonErr := json.Marshal(scanResponse.ScanResult); jsonErr == nil {
						dlpScanResultJSON = string(resultBytes)
					}
					if scanResponse.ScanResult.TotalMatches > 0 {
						piiDetected = true

						// Apply redaction
						redactedContent, redactErr := redactionEngine.RedactContent(content, scanResponse.ScanResult, types.RedactionStrategy(redactionMode))
						if redactErr != nil {
							log.Printf("[Assembler] Redaction failed for page %d: %v", pr.PageNumber, redactErr)
						} else {
							log.Printf("[Assembler] Redacted %d findings on page %d using %s mode",
								scanResponse.ScanResult.TotalMatches, pr.PageNumber, redactionMode)
							content = redactedContent
						}
					}
				}
				chunkDLPStatus = "completed"
			}
		}

		chunkID := uuid.New()
		chunkKey := services.GetChunkKey(fileID.String(), chunkID.String())

		// Upload chunk content to S3 (redacted if applicable)
		s3Start := time.Now()
		if err := s3Uploader.UploadChunkContent(ctx, s3Bucket, chunkKey, content); err != nil {
			log.Printf("[Assembler] Failed to upload chunk %d to S3: %v", pr.PageNumber, err)
			continue
		}
		pipelineMetrics.AssemblerS3UploadDuration.Record(time.Since(s3Start))

		// Generate content preview (first 500 runes to avoid splitting multi-byte chars)
		runes := []rune(content)
		if len(runes) > 500 {
			runes = runes[:500]
		}
		preview := string(runes)

		// Generate content hash
		hash := sha256.Sum256([]byte(content))

		pageNum := pr.PageNumber
		chunk := models.Chunk{
			TenantID:        tenantID,
			FileID:          fileID,
			ChunkID:         fmt.Sprintf("page_%d", pr.PageNumber),
			ChunkType:       "pdf_page",
			ChunkNumber:     i + 1,
			S3Bucket:        s3Bucket,
			S3Key:           chunkKey,
			ContentPreview:  preview,
			ContentHash:     hex.EncodeToString(hash[:]),
			SizeBytes:       int64(len(content)),
			PageNumber:      &pageNum,
			ProcessedAt:     time.Now(),
			ProcessedBy:     "kafka_pipeline",
			ProcessingTime:  derefInt64(pr.ProcessingDurationMs),
			EmbeddingStatus: "pending",
			DLPScanStatus:   chunkDLPStatus,
			DLPScanResult:   dlpScanResultJSON,
			PIIDetected:     piiDetected,
			Context: models.ChunkContext{
				"page_number":       strconv.Itoa(pr.PageNumber),
				"extraction_method": preprocessing.SanitizeUTF8String(pr.ExtractionMethod),
				"pipeline":          "kafka",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		chunks = append(chunks, chunk)
		chunkCount++
	}

	// Batch insert chunks
	if len(chunks) > 0 {
		if err := db.CreateInBatches(&chunks, 50).Error; err != nil {
			log.Printf("[Assembler] Failed to insert chunks: %v", err)
			markJobFailed(db, job.ID, fmt.Sprintf("chunk insert failed: %v", err))
			return
		}
	}

	// Publish DLP jobs for each chunk (triggers DLP → Embedding pipeline)
	publishDLPJobs(ctx, producer, chunks, tenantID.String(), fileID.String())

	// Update job to completed
	now := time.Now()
	finalStatus := models.JobStatusCompleted
	if job.FailedPages > 0 {
		finalStatus = models.JobStatusPartial
	}
	db.Model(job).Updates(map[string]any{
		"status":       finalStatus,
		"completed_at": &now,
		"updated_at":   now,
	})

	// Update file status with processing duration
	processingDuration := now.Sub(job.CreatedAt).Milliseconds()
	updateFileStatus(db, tenantID, fileID, "processed", "")
	db.Model(&models.File{}).Where("id = ? AND tenant_id = ?", fileID, tenantID).
		Updates(map[string]any{
			"chunk_count":          chunkCount,
			"processed_at":         &now,
			"processing_duration":  processingDuration,
		})

	pipelineMetrics.AssemblerFilesCompleted.Inc()
	pipelineMetrics.AssemblerChunksStored.Add(int64(chunkCount))

	log.Printf("[Assembler] File %s assembled: %d chunks created, %d pages failed",
		fileID.String(), chunkCount, job.FailedPages)

	// Publish processing complete event
	publishCompleteEvent(ctx, producer, tenantID, fileID, file, chunkCount, job)
}

func publishCompleteEvent(ctx context.Context, producer *events.SimpleProducer,
	tenantID, fileID uuid.UUID, file models.File, chunkCount int, job *models.ProcessingJob) {

	var documentID string
	if file.Neo4jDocumentID != nil {
		documentID = *file.Neo4jDocumentID
	}

	eventData := events.ProcessingCompleteData{
		FileID:              fileID.String(),
		DocumentID:          documentID,
		URL:                 file.URL,
		ChunksCreated:       chunkCount,
		DLPViolationsFound:  0,
		FinalDataClass:      "internal",
		StorageLocation:     file.Path,
		Success:             job.FailedPages == 0,
	}

	event := events.NewProcessingCompleteEvent("audimodal-assembler", tenantID.String(), eventData)
	if err := producer.PublishEvent(ctx, event); err != nil {
		log.Printf("[Assembler] Failed to publish processing event: %v", err)
	} else {
		log.Printf("[Assembler] Published processing complete event for file %s", fileID.String())
	}
}

func publishDLPJobs(ctx context.Context, producer *events.SimpleProducer, chunks []models.Chunk, tenantID, fileID string) {
	published := 0
	for _, chunk := range chunks {
		dlpMsg := events.DLPJobMessage{
			MessageID:      uuid.New().String(),
			TenantID:       tenantID,
			FileID:         fileID,
			ChunkID:        chunk.ID.String(),
			S3Bucket:       chunk.S3Bucket,
			S3Key:          chunk.S3Key,
			ContentPreview: chunk.ContentPreview,
			ProducedAt:     time.Now(),
		}

		msgData, err := json.Marshal(dlpMsg)
		if err != nil {
			log.Printf("[Assembler] Failed to marshal DLP job for chunk %s: %v", chunk.ID.String(), err)
			continue
		}

		kafkaMsg := kafka.Message{
			Topic: events.TopicDLPJobs,
			Key:   []byte(dlpMsg.PartitionKey()),
			Value: msgData,
			Headers: []kafka.Header{
				{Key: "event-type", Value: []byte("dlp-job")},
				{Key: "chunk-id", Value: []byte(chunk.ID.String())},
			},
		}

		if err := producer.WriteMessages(ctx, kafkaMsg); err != nil {
			log.Printf("[Assembler] Failed to publish DLP job for chunk %s: %v", chunk.ID.String(), err)
			continue
		}
		published++
	}

	if published > 0 {
		log.Printf("[Assembler] Published %d DLP jobs for file %s", published, fileID)
	}
}

func updateFileStatus(db *gorm.DB, tenantID, fileID uuid.UUID, status, errMsg string) {
	updates := map[string]any{
		"status":     status,
		"updated_at": time.Now(),
	}
	if errMsg != "" {
		updates["error_message"] = errMsg
	}
	db.Model(&models.File{}).Where("id = ? AND tenant_id = ?", fileID, tenantID).Updates(updates)
}

func markJobFailed(db *gorm.DB, jobID uuid.UUID, errMsg string) {
	now := time.Now()
	db.Model(&models.ProcessingJob{}).Where("id = ?", jobID).Updates(map[string]any{
		"status":        models.JobStatusFailed,
		"error_message": errMsg,
		"completed_at":  &now,
		"updated_at":    now,
	})
}

func detectStaleJobs(ctx context.Context, db *gorm.DB) {
	ticker := time.NewTicker(staleJobCheckDelay)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-staleJobTimeout)
			result := db.Model(&models.ProcessingJob{}).
				Where("status IN (?, ?) AND updated_at < ?",
					models.JobStatusSplitting, models.JobStatusProcessing, cutoff).
				Updates(map[string]any{
					"status":     models.JobStatusTimedOut,
					"updated_at": time.Now(),
				})
			if result.RowsAffected > 0 {
				pipelineMetrics.AssemblerStaleJobs.Add(result.RowsAffected)
				log.Printf("[Assembler] Marked %d stale jobs as timed_out", result.RowsAffected)
			}
		}
	}
}

func connectDB() (*gorm.DB, error) {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USERNAME", "postgres")
	pass := getEnv("DB_PASSWORD", "")
	name := getEnv("DB_DATABASE", "audimodal")
	sslMode := getEnv("DB_SSL_MODE", "disable")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, pass, name, sslMode)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func startHealthServer(registry *metrics.MetricsRegistry) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
	})
	mux.HandleFunc("/metrics", metrics.HTTPMetricsHandler(registry))

	addr := fmt.Sprintf(":%d", healthPort)
	log.Printf("[Assembler] Health server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("[Assembler] Health server error: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
