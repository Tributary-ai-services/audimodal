// Embedding Worker: Consumes embedding job messages from Kafka, generates vector
// embeddings using OpenAI, stores them in DeepLake, and updates chunk status in PostgreSQL.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/jscharber/audimodal/internal/database/models"
	"github.com/jscharber/audimodal/internal/services"
	"github.com/jscharber/audimodal/pkg/embeddings"
	"github.com/jscharber/audimodal/pkg/embeddings/client"
	"github.com/jscharber/audimodal/pkg/embeddings/providers"
	"github.com/jscharber/audimodal/pkg/events"
	"github.com/jscharber/audimodal/pkg/metrics"
)

var pipelineMetrics *metrics.PipelineMetrics

const healthPort = 8081

func main() {
	log.Println("[EmbeddingWorker] Starting Embedding Worker...")

	// Initialize metrics
	registry := metrics.NewRegistry()
	registry.AddGlobalLabel("service", "audimodal-embedding-worker")
	pipelineMetrics = metrics.NewPipelineMetrics(registry)

	bootstrapServers := getEnv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092")
	groupID := getEnv("KAFKA_GROUP_ID", "audimodal-embedding-workers")

	// Connect to PostgreSQL
	db, err := connectDB()
	if err != nil {
		log.Fatalf("[EmbeddingWorker] Failed to connect to database: %v", err)
	}

	// Create S3 uploader for reading chunk content
	s3Uploader, err := services.NewS3Uploader()
	if err != nil {
		log.Fatalf("[EmbeddingWorker] Failed to create S3 uploader: %v", err)
	}

	// Create embedding service
	embeddingService, err := createEmbeddingService()
	if err != nil {
		log.Fatalf("[EmbeddingWorker] Failed to create embedding service: %v", err)
	}

	// Create Kafka consumer for embedding jobs
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{bootstrapServers},
		GroupID:     groupID,
		Topic:       events.TopicEmbeddingJobs,
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
		log.Printf("[EmbeddingWorker] Received signal %v, shutting down...", sig)
		cancel()
	}()

	log.Printf("[EmbeddingWorker] Consumer started. GroupID: %s, Bootstrap: %s", groupID, bootstrapServers)

	// Main consumer loop
	for {
		select {
		case <-ctx.Done():
			log.Println("[EmbeddingWorker] Context cancelled, exiting consumer loop")
			return
		default:
			msg, err := reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("[EmbeddingWorker] Error fetching message: %v", err)
				time.Sleep(time.Second)
				continue
			}

			processEmbeddingJob(ctx, msg, db, s3Uploader, embeddingService)

			if err := reader.CommitMessages(ctx, msg); err != nil {
				log.Printf("[EmbeddingWorker] Error committing offset: %v", err)
			}
		}
	}
}

func processEmbeddingJob(ctx context.Context, msg kafka.Message, db *gorm.DB,
	s3Uploader *services.S3Uploader, embeddingService embeddings.EmbeddingService) {

	startTime := time.Now()

	var job events.EmbeddingJobMessage
	if err := json.Unmarshal(msg.Value, &job); err != nil {
		log.Printf("[EmbeddingWorker] Failed to unmarshal embedding job: %v", err)
		return
	}

	log.Printf("[EmbeddingWorker] Processing embedding for chunk %s (file %s)", job.ChunkID, job.FileID)
	pipelineMetrics.KafkaMessagesConsumed.Inc()

	chunkID, _ := uuid.Parse(job.ChunkID)
	fileID, _ := uuid.Parse(job.FileID)
	tenantID, _ := uuid.Parse(job.TenantID)

	// Update chunk status to processing
	db.Model(&models.Chunk{}).Where("id = ?", chunkID).
		Update("embedding_status", models.EmbeddingStatusProcessing)

	// Load chunk content from S3
	content, err := s3Uploader.GetChunkContent(ctx, job.S3Bucket, job.S3Key)
	if err != nil {
		log.Printf("[EmbeddingWorker] Failed to read chunk content from S3: %v", err)
		pipelineMetrics.EmbeddingProcessingErrors.Inc()
		db.Model(&models.Chunk{}).Where("id = ?", chunkID).
			Updates(map[string]any{
				"embedding_status": models.EmbeddingStatusFailed,
				"updated_at":      time.Now(),
			})
		return
	}

	if content == "" {
		log.Printf("[EmbeddingWorker] Empty content for chunk %s, skipping", job.ChunkID)
		db.Model(&models.Chunk{}).Where("id = ?", chunkID).
			Updates(map[string]any{
				"embedding_status": models.EmbeddingStatusSkipped,
				"updated_at":      time.Now(),
			})
		return
	}

	// Build dataset name from tenant
	dataset := fmt.Sprintf("tenant-%s", services.GetTenantShortID(job.TenantID))

	// Process the chunk through the embedding service
	chunkInput := &embeddings.ChunkInput{
		ID:         job.ChunkID,
		DocumentID: job.FileID,
		Content:    content,
		ChunkIndex: 0,
	}

	response, err := embeddingService.ProcessChunks(ctx, &embeddings.ProcessChunksRequest{
		Chunks:   []*embeddings.ChunkInput{chunkInput},
		Dataset:  dataset,
		TenantID: tenantID,
	})
	if err != nil {
		log.Printf("[EmbeddingWorker] Embedding failed for chunk %s: %v", job.ChunkID, err)
		pipelineMetrics.EmbeddingProcessingErrors.Inc()
		db.Model(&models.Chunk{}).Where("id = ?", chunkID).
			Updates(map[string]any{
				"embedding_status": models.EmbeddingStatusFailed,
				"updated_at":      time.Now(),
			})
		return
	}

	// Determine embedding model from response
	embeddingModel := getEnv("EMBEDDING_MODEL", "text-embedding-3-small")

	// Update chunk in DB
	now := time.Now()
	db.Model(&models.Chunk{}).Where("id = ?", chunkID).
		Updates(map[string]any{
			"embedding_status": models.EmbeddingStatusCompleted,
			"embedding_model":  embeddingModel,
			"embedded_at":      &now,
			"updated_at":       now,
		})

	pipelineMetrics.EmbeddingChunksProcessed.Inc()
	pipelineMetrics.EmbeddingChunkDuration.Record(time.Since(startTime))

	log.Printf("[EmbeddingWorker] Chunk %s embedded: vectors=%d, status=%s, duration=%dms",
		job.ChunkID, response.VectorsCreated, response.Status, time.Since(startTime).Milliseconds())

	// Check if ALL chunks for this file are now embedded
	checkFileEmbeddingComplete(db, fileID)
}

func checkFileEmbeddingComplete(db *gorm.DB, fileID uuid.UUID) {
	var totalChunks int64
	var embeddedChunks int64

	db.Model(&models.Chunk{}).Where("file_id = ?", fileID).Count(&totalChunks)
	db.Model(&models.Chunk{}).Where("file_id = ? AND embedding_status = ?",
		fileID, models.EmbeddingStatusCompleted).Count(&embeddedChunks)

	if totalChunks > 0 && embeddedChunks == totalChunks {
		log.Printf("[EmbeddingWorker] All %d chunks for file %s are embedded", totalChunks, fileID.String())
		pipelineMetrics.EmbeddingFilesCompleted.Inc()
	}
}

func createEmbeddingService() (embeddings.EmbeddingService, error) {
	openaiKey := getEnv("OPENAI_API_KEY", "")
	if openaiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required")
	}

	deeplakeURL := getEnv("DEEPLAKE_API_URL", "")
	if deeplakeURL == "" {
		return nil, fmt.Errorf("DEEPLAKE_API_URL is required")
	}

	deeplakeKey := getEnv("DEEPLAKE_API_KEY", "")
	if deeplakeKey == "" {
		return nil, fmt.Errorf("DEEPLAKE_API_KEY is required")
	}

	// Create OpenAI embedding provider
	openaiProvider, err := providers.NewOpenAIProvider(&providers.OpenAIConfig{
		APIKey:  openaiKey,
		Model:   getEnv("EMBEDDING_MODEL", "text-embedding-3-small"),
		Timeout: 60 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI provider: %w", err)
	}

	// Create DeepLake vector store client
	deeplakeClient, err := client.NewDeepLakeAPIClient(&client.DeepLakeAPIConfig{
		BaseURL: deeplakeURL,
		APIKey:  deeplakeKey,
		Timeout: 60 * time.Second,
		Retries: 3,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create DeepLake client: %w", err)
	}

	// Create embedding service
	service := embeddings.NewEmbeddingService(openaiProvider, deeplakeClient, &embeddings.ServiceConfig{
		DefaultDataset:   "default",
		ChunkSize:        1000,
		ChunkOverlap:     100,
		BatchSize:        10,
		MaxConcurrency:   5,
		CacheEnabled:     false,
		MetricsEnabled:   true,
		DefaultTopK:      10,
		DefaultThreshold: 0.7,
		DefaultMetric:    "cosine",
	})

	return service, nil
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
	log.Printf("[EmbeddingWorker] Health server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("[EmbeddingWorker] Health server error: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
