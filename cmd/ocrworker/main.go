// OCR Worker: Standalone binary that consumes page jobs from Kafka, runs OCR/text extraction,
// and produces page results back to Kafka. Designed to run as N independent pods.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"

	"github.com/jscharber/audimodal/internal/services"
	"github.com/jscharber/audimodal/pkg/events"
	"github.com/jscharber/audimodal/pkg/metrics"
	"github.com/jscharber/audimodal/pkg/readers/pdf/mapreduce"
)

// Package-level pipeline metrics
var pipelineMetrics *metrics.PipelineMetrics

const (
	watchdogTimeout = 45 * time.Minute // Kill page if stuck for >45 min
	healthPort      = 8081
)

func main() {
	log.Println("[OCRWorker] Starting OCR Worker...")

	// Initialize metrics
	registry := metrics.NewRegistry()
	registry.AddGlobalLabel("service", "audimodal-ocr-worker")
	pipelineMetrics = metrics.NewPipelineMetrics(registry)

	bootstrapServers := getEnv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092")
	groupID := getEnv("KAFKA_GROUP_ID", "audimodal-ocr-workers")

	// Create S3 uploader
	s3Uploader, err := services.NewS3Uploader()
	if err != nil {
		log.Fatalf("[OCRWorker] Failed to create S3 uploader: %v", err)
	}

	// Create Kafka producer for results
	producer, err := events.NewSimpleProducer(events.SimpleProducerConfig{
		BootstrapServers: bootstrapServers,
		ClientID:         "audimodal-ocr-worker",
	})
	if err != nil {
		log.Fatalf("[OCRWorker] Failed to create Kafka producer: %v", err)
	}
	defer producer.Close()

	// Create Kafka consumer
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{bootstrapServers},
		GroupID:     groupID,
		Topic:       events.TopicPageJobs,
		StartOffset: kafka.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
		MaxWait:     3 * time.Second,
	})
	defer reader.Close()

	// PDF cache: keep downloaded PDFs per file_id to avoid re-downloading
	pdfCache := &pdfFileCache{
		cache: make(map[string]string),
	}
	defer pdfCache.Cleanup()

	// Create page extractor (reuses existing OCR logic)
	extractor := mapreduce.NewPageExtractor(mapreduce.DefaultExtractionConfig())

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
		log.Printf("[OCRWorker] Received signal %v, shutting down...", sig)
		cancel()
	}()

	// Worker name for tracking
	workerPodName := getEnv("POD_NAME", fmt.Sprintf("worker-%s", uuid.New().String()[:8]))

	log.Printf("[OCRWorker] Consumer started. GroupID: %s, Bootstrap: %s, Worker: %s",
		groupID, bootstrapServers, workerPodName)

	// Main consumer loop
	for {
		select {
		case <-ctx.Done():
			log.Println("[OCRWorker] Context cancelled, exiting consumer loop")
			return
		default:
			msg, err := reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("[OCRWorker] Error fetching message: %v", err)
				time.Sleep(time.Second)
				continue
			}

			processPageJob(ctx, msg, extractor, s3Uploader, producer, pdfCache, workerPodName)

			if err := reader.CommitMessages(ctx, msg); err != nil {
				log.Printf("[OCRWorker] Error committing offset: %v", err)
			}
		}
	}
}

func processPageJob(ctx context.Context, msg kafka.Message, extractor *mapreduce.DefaultPageExtractor,
	s3Uploader *services.S3Uploader, producer *events.SimpleProducer, cache *pdfFileCache, workerPodName string) {

	startTime := time.Now()

	// Deserialize the page job message
	var pageJob events.PageJobMessage
	if err := json.Unmarshal(msg.Value, &pageJob); err != nil {
		log.Printf("[OCRWorker] Failed to unmarshal page job: %v", err)
		return
	}

	log.Printf("[OCRWorker] Processing page %d/%d for file %s (job %s)",
		pageJob.PageNumber, pageJob.TotalPages, pageJob.FileID, pageJob.JobID)

	pipelineMetrics.KafkaMessagesConsumed.Inc()
	pipelineMetrics.OCRActiveWorkers.Inc()
	defer pipelineMetrics.OCRActiveWorkers.Dec()

	// Prepare result message
	result := events.PageResultMessage{
		MessageID:     uuid.New().String(),
		TenantID:      pageJob.TenantID,
		FileID:        pageJob.FileID,
		JobID:         pageJob.JobID,
		PageNumber:    pageJob.PageNumber,
		TotalPages:    pageJob.TotalPages,
		WorkerPodName: workerPodName,
		ProducedAt:    time.Now(),
	}

	// Download PDF from S3 (cached per file_id)
	pdfPath, err := cache.GetOrDownload(ctx, s3Uploader, pageJob.S3Bucket, pageJob.S3Key, pageJob.FileID)
	if err != nil {
		log.Printf("[OCRWorker] S3 download error for file %s: %v", pageJob.FileID, err)
		result.Status = "failed"
		result.Error = fmt.Sprintf("s3_download: %v", err)
		result.ErrorType = "s3_error"
		produceResult(ctx, producer, result)
		return
	}

	// Create a PageJob for the existing extractor
	tenantID, _ := uuid.Parse(pageJob.TenantID)
	fileID, _ := uuid.Parse(pageJob.FileID)

	extractJob := &mapreduce.PageJob{
		JobID:          pageJob.JobID,
		TenantID:       tenantID,
		FileID:         fileID,
		PDFPath:        pdfPath,
		PageNumber:     pageJob.PageNumber,
		TotalPages:     pageJob.TotalPages,
		Classification: mapreduce.PageClassificationType(pageJob.Classification),
		Config:         convertConfig(pageJob.Config),
	}

	// Run extraction with watchdog timeout
	extractCtx, extractCancel := context.WithTimeout(ctx, watchdogTimeout)
	defer extractCancel()

	pageResult, err := extractor.ExtractPage(extractCtx, extractJob)
	if err != nil {
		errType := "processing_error"
		if extractCtx.Err() == context.DeadlineExceeded {
			errType = "hung_process"
		}
		log.Printf("[OCRWorker] Extraction error for page %d of %s: %v (type: %s)",
			pageJob.PageNumber, pageJob.FileID, err, errType)

		pipelineMetrics.OCRProcessingErrors.Inc()

		result.Status = "failed"
		result.Error = err.Error()
		result.ErrorType = errType
		result.ProcessingDurationMs = time.Since(startTime).Milliseconds()
		produceResult(ctx, producer, result)
		return
	}

	// Success
	result.Status = "completed"
	result.Content = pageResult.Content
	result.ExtractionMethod = string(pageResult.ExtractionMethod)
	result.OCRConfidence = pageResult.OCRConfidence
	result.ProcessingDurationMs = time.Since(startTime).Milliseconds()

	pipelineMetrics.OCRPagesProcessed.Inc()
	pipelineMetrics.OCRPageDuration.Record(time.Since(startTime))

	produceResult(ctx, producer, result)

	log.Printf("[OCRWorker] Page %d/%d of %s completed in %dms (method: %s)",
		pageJob.PageNumber, pageJob.TotalPages, pageJob.FileID,
		result.ProcessingDurationMs, result.ExtractionMethod)
}

func produceResult(ctx context.Context, producer *events.SimpleProducer, result events.PageResultMessage) {
	msgData, err := json.Marshal(result)
	if err != nil {
		log.Printf("[OCRWorker] Failed to marshal result: %v", err)
		return
	}

	msg := kafka.Message{
		Topic: events.TopicPageResults,
		Key:   []byte(result.PartitionKey()),
		Value: msgData,
		Headers: []kafka.Header{
			{Key: "event-type", Value: []byte("page-result")},
			{Key: "job-id", Value: []byte(result.JobID)},
		},
	}

	if err := producer.WriteMessages(ctx, msg); err != nil {
		log.Printf("[OCRWorker] Failed to produce result for page %d of %s: %v",
			result.PageNumber, result.FileID, err)
	}
}

func convertConfig(cfg events.ExtractionConfig) *mapreduce.ExtractionConfig {
	return &mapreduce.ExtractionConfig{
		OCRLanguage:       cfg.OCRLanguage,
		OCRDPI:            cfg.OCRDPI,
		OCRTimeout:        cfg.OCRTimeout,
		TextThreshold:     cfg.TextThreshold,
		OCRAnyImage:       cfg.OCRAnyImage,
		OCRImageMinWidth:  cfg.OCRImageMinWidth,
		OCRImageMinHeight: cfg.OCRImageMinHeight,
		MaxMemoryMB:       cfg.MaxMemoryMB,
		PreserveLayout:    cfg.PreserveLayout,
	}
}

// pdfFileCache caches downloaded PDFs per file_id
type pdfFileCache struct {
	mu    sync.Mutex
	cache map[string]string // fileID -> local path
}

func (c *pdfFileCache) GetOrDownload(ctx context.Context, s3Uploader *services.S3Uploader, bucket, key, fileID string) (string, error) {
	c.mu.Lock()
	if path, ok := c.cache[fileID]; ok {
		c.mu.Unlock()
		// Verify file still exists
		if _, err := os.Stat(path); err == nil {
			if pipelineMetrics != nil {
				pipelineMetrics.OCRPDFCacheHits.Inc()
			}
			return path, nil
		}
		// File was deleted, remove from cache
		c.mu.Lock()
		delete(c.cache, fileID)
		c.mu.Unlock()
	} else {
		c.mu.Unlock()
	}

	// Download from S3
	tempDir := os.TempDir()
	localPath := filepath.Join(tempDir, fmt.Sprintf("ocr-%s-%s.pdf", fileID[:8], uuid.New().String()[:8]))

	if err := s3Uploader.DownloadFile(ctx, bucket, key, localPath); err != nil {
		return "", err
	}

	c.mu.Lock()
	c.cache[fileID] = localPath
	c.mu.Unlock()

	return localPath, nil
}

func (c *pdfFileCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, path := range c.cache {
		os.Remove(path)
	}
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
	log.Printf("[OCRWorker] Health server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("[OCRWorker] Health server error: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
