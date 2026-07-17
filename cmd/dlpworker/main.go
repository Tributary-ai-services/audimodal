// DLP Worker: Consumes DLP job messages from Kafka, scans chunk content for PII
// using the built-in DLP scanner, updates chunk status in PostgreSQL, and publishes
// embedding jobs for downstream processing.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	"github.com/jscharber/audimodal/pkg/dlp/compliance"
	"github.com/jscharber/audimodal/pkg/dlp/scanner"
	"github.com/jscharber/audimodal/pkg/dlp/shadow"
	dlpTypes "github.com/jscharber/audimodal/pkg/dlp/types"
	"github.com/jscharber/audimodal/pkg/events"
	"github.com/jscharber/audimodal/pkg/metrics"
)

var pipelineMetrics *metrics.PipelineMetrics

const healthPort = 8081

func main() {
	log.Println("[DLPWorker] Starting DLP Worker...")

	// Initialize metrics
	registry := metrics.NewRegistry()
	registry.AddGlobalLabel("service", "audimodal-dlp-worker")
	pipelineMetrics = metrics.NewPipelineMetrics(registry)

	bootstrapServers := getEnv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092")
	groupID := getEnv("KAFKA_GROUP_ID", "audimodal-dlp-workers")

	// Connect to PostgreSQL
	db, err := connectDB()
	if err != nil {
		log.Fatalf("[DLPWorker] Failed to connect to database: %v", err)
	}

	// Create S3 uploader for reading chunk content
	s3Uploader, err := services.NewS3Uploader()
	if err != nil {
		log.Fatalf("[DLPWorker] Failed to create S3 uploader: %v", err)
	}

	// Create Kafka producer for publishing embedding jobs
	producer, err := events.NewSimpleProducer(events.SimpleProducerConfig{
		BootstrapServers: bootstrapServers,
		ClientID:         "audimodal-dlp-worker",
	})
	if err != nil {
		log.Fatalf("[DLPWorker] Failed to create Kafka producer: %v", err)
	}
	defer producer.Close()

	// Create DLP service (built-in PII scanner, no external API needed).
	//
	// DLP_SHADOW_SCAN=true wraps the scanner in a Gatekeeper shadow (audimodal#26,
	// G6): it dual-runs Gatekeeper on every scan and LOGS the per-type finding
	// diff, while audimodal stays authoritative (result unchanged). This measures
	// the coverage delta — Gatekeeper detects cloud secrets audimodal has no
	// matcher for — before any cutover. Off by default; enabling it changes
	// nothing audimodal detects or redacts.
	var dlpService *dlp.DLPService
	if os.Getenv("DLP_SHADOW_SCAN") == "true" {
		shadowLog := log.New(os.Stderr, "[DLPWorker][gk-shadow] ", log.LstdFlags)
		primary := shadow.New(scanner.NewBasicDLPScanner(), shadowLog)
		dlpService = dlp.NewDLPServiceWithComponents(
			primary,
			scanner.NewBasicRedactionEngine(),
			compliance.NewBasicComplianceChecker(),
			nil,
		)
		log.Println("[DLPWorker] Gatekeeper shadow scanning ENABLED (log-only diff; audimodal authoritative)")
	} else {
		dlpService = dlp.NewDLPService(nil)
	}

	// Create Kafka consumer for DLP jobs
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{bootstrapServers},
		GroupID:     groupID,
		Topic:       events.TopicDLPJobs,
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
		log.Printf("[DLPWorker] Received signal %v, shutting down...", sig)
		cancel()
	}()

	log.Printf("[DLPWorker] Consumer started. GroupID: %s, Bootstrap: %s", groupID, bootstrapServers)

	// Main consumer loop
	for {
		select {
		case <-ctx.Done():
			log.Println("[DLPWorker] Context cancelled, exiting consumer loop")
			return
		default:
			msg, err := reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("[DLPWorker] Error fetching message: %v", err)
				time.Sleep(time.Second)
				continue
			}

			processDLPJob(ctx, msg, db, s3Uploader, dlpService, producer)

			if err := reader.CommitMessages(ctx, msg); err != nil {
				log.Printf("[DLPWorker] Error committing offset: %v", err)
			}
		}
	}
}

func processDLPJob(ctx context.Context, msg kafka.Message, db *gorm.DB,
	s3Uploader *services.S3Uploader, dlpService *dlp.DLPService, producer *events.SimpleProducer) {

	startTime := time.Now()

	var job events.DLPJobMessage
	if err := json.Unmarshal(msg.Value, &job); err != nil {
		log.Printf("[DLPWorker] Failed to unmarshal DLP job: %v", err)
		return
	}

	log.Printf("[DLPWorker] Processing DLP scan for chunk %s (file %s)", job.ChunkID, job.FileID)
	pipelineMetrics.KafkaMessagesConsumed.Inc()

	chunkID, _ := uuid.Parse(job.ChunkID)
	tenantID, _ := uuid.Parse(job.TenantID)

	// Skip chunks already scanned by the assembler (inline redaction)
	var existingChunk models.Chunk
	if err := db.Where("id = ?", chunkID).Select("dlp_scan_status").First(&existingChunk).Error; err == nil {
		if existingChunk.DLPScanStatus == models.DLPScanStatusCompleted {
			log.Printf("[DLPWorker] Chunk %s already scanned (by assembler), skipping", job.ChunkID)
			// Still publish embedding job so downstream processing continues
			publishEmbeddingJob(ctx, producer, job)
			return
		}
	}

	// Update chunk status to processing
	db.Model(&models.Chunk{}).Where("id = ?", chunkID).
		Update("dlp_scan_status", models.DLPScanStatusProcessing)

	// Load chunk content from S3
	content, err := s3Uploader.GetChunkContent(ctx, job.S3Bucket, job.S3Key)
	if err != nil {
		log.Printf("[DLPWorker] Failed to read chunk content from S3: %v", err)
		pipelineMetrics.DLPProcessingErrors.Inc()
		db.Model(&models.Chunk{}).Where("id = ?", chunkID).
			Updates(map[string]any{
				"dlp_scan_status": models.DLPScanStatusFailed,
				"updated_at":      time.Now(),
			})
		return
	}

	// Run DLP scan
	scanResponse, err := dlpService.ScanContent(ctx, &dlp.DLPScanRequest{
		TenantID:  tenantID,
		ContentID: job.ChunkID,
		Content:   content,
		Metadata: map[string]string{
			"file_id":  job.FileID,
			"chunk_id": job.ChunkID,
			"pipeline": "kafka",
		},
	})
	if err != nil {
		log.Printf("[DLPWorker] DLP scan failed for chunk %s: %v", job.ChunkID, err)
		pipelineMetrics.DLPProcessingErrors.Inc()
		db.Model(&models.Chunk{}).Where("id = ?", chunkID).
			Updates(map[string]any{
				"dlp_scan_status": models.DLPScanStatusFailed,
				"updated_at":      time.Now(),
			})
		return
	}

	// Extract scan results
	findingCount := 0
	riskScore := 0.0
	hasPII := false
	scanResultJSON := ""

	if scanResponse.ScanResult != nil {
		findingCount = scanResponse.ScanResult.TotalMatches
		riskScore = scanResponse.ScanResult.RiskScore
		hasPII = findingCount > 0
		if resultBytes, err := json.Marshal(scanResponse.ScanResult); err == nil {
			scanResultJSON = string(resultBytes)
		}
	}

	// Update chunk in DB
	db.Model(&models.Chunk{}).Where("id = ?", chunkID).
		Updates(map[string]any{
			"dlp_scan_status":   models.DLPScanStatusCompleted,
			"dlp_scan_result":   scanResultJSON,
			"dlp_finding_count": findingCount,
			"dlp_risk_score":    riskScore,
			"pii_detected":      hasPII,
			"updated_at":        time.Now(),
		})

	pipelineMetrics.DLPChunksScanned.Inc()
	pipelineMetrics.DLPChunkDuration.Record(time.Since(startTime))
	if findingCount > 0 {
		pipelineMetrics.DLPFindingsDetected.Add(int64(findingCount))
	}

	// Create DLP violation records for each finding
	if scanResponse.ScanResult != nil && len(scanResponse.ScanResult.Findings) > 0 {
		fileID, _ := uuid.Parse(job.FileID)
		systemPolicyID := getOrCreateSystemPolicy(db, tenantID)
		createViolationRecords(db, tenantID, fileID, chunkID, systemPolicyID, scanResponse.ScanResult)
	}

	log.Printf("[DLPWorker] Chunk %s scanned: pii=%v, findings=%d, risk=%.2f, duration=%dms",
		job.ChunkID, hasPII, findingCount, riskScore, time.Since(startTime).Milliseconds())

	// Publish embedding job for downstream processing
	publishEmbeddingJob(ctx, producer, job)
}

// publishEmbeddingJob publishes an embedding job to Kafka for downstream processing
func publishEmbeddingJob(ctx context.Context, producer *events.SimpleProducer, job events.DLPJobMessage) {
	embeddingMsg := events.EmbeddingJobMessage{
		MessageID:  uuid.New().String(),
		TenantID:   job.TenantID,
		FileID:     job.FileID,
		ChunkID:    job.ChunkID,
		S3Bucket:   job.S3Bucket,
		S3Key:      job.S3Key,
		ProducedAt: time.Now(),
	}

	msgData, err := json.Marshal(embeddingMsg)
	if err != nil {
		log.Printf("[DLPWorker] Failed to marshal embedding job: %v", err)
		return
	}

	kafkaMsg := kafka.Message{
		Topic: events.TopicEmbeddingJobs,
		Key:   []byte(embeddingMsg.PartitionKey()),
		Value: msgData,
		Headers: []kafka.Header{
			{Key: "event-type", Value: []byte("embedding-job")},
			{Key: "chunk-id", Value: []byte(job.ChunkID)},
		},
	}

	if err := producer.WriteMessages(ctx, kafkaMsg); err != nil {
		log.Printf("[DLPWorker] Failed to publish embedding job for chunk %s: %v", job.ChunkID, err)
	}
}

// systemPolicyCache caches system policy IDs per tenant to avoid repeated DB lookups
var systemPolicyCache = make(map[uuid.UUID]uuid.UUID)

// getOrCreateSystemPolicy returns the system DLP policy for a tenant, creating one if needed
func getOrCreateSystemPolicy(db *gorm.DB, tenantID uuid.UUID) uuid.UUID {
	if policyID, ok := systemPolicyCache[tenantID]; ok {
		return policyID
	}

	var policy models.DLPPolicy
	err := db.Where("tenant_id = ? AND name = ?", tenantID, "system-auto-scan").First(&policy).Error
	if err == nil {
		systemPolicyCache[tenantID] = policy.ID
		return policy.ID
	}

	// Create system policy
	policy = models.DLPPolicy{
		TenantID:     tenantID,
		Name:         "system-auto-scan",
		DisplayName:  "Automatic PII Scanner",
		Description:  "System policy for automatic DLP scanning of uploaded documents",
		Enabled:      true,
		Priority:     100,
		ContentRules: models.DLPContentRules{},
		Actions:      models.DLPActions{},
		Conditions:   models.DLPConditions{},
		Status:       "active",
	}
	if err := db.Create(&policy).Error; err != nil {
		log.Printf("[DLPWorker] Failed to create system policy for tenant %s: %v", tenantID, err)
		return uuid.Nil
	}

	systemPolicyCache[tenantID] = policy.ID
	log.Printf("[DLPWorker] Created system DLP policy %s for tenant %s", policy.ID, tenantID)
	return policy.ID
}

// createViolationRecords inserts DLPViolation records for each finding
func createViolationRecords(db *gorm.DB, tenantID, fileID, chunkID, policyID uuid.UUID, result *dlpTypes.ScanResult) {
	if policyID == uuid.Nil {
		return
	}

	for _, finding := range result.Findings {
		severity := string(finding.RiskLevel)
		// Map DLP risk levels to violation severity
		switch finding.RiskLevel {
		case "critical":
			severity = "critical"
		case "high":
			severity = "high"
		case "medium":
			severity = "medium"
		default:
			severity = "low"
		}

		complianceType := "pii"
		ruleName := fmt.Sprintf("%s_DETECTED", strings.ToUpper(string(finding.Type)))

		violation := models.DLPViolation{
			TenantID:       tenantID,
			PolicyID:       policyID,
			FileID:         fileID,
			ChunkID:        &chunkID,
			RuleName:       ruleName,
			Severity:       severity,
			ComplianceType: complianceType,
			Confidence:     finding.Confidence,
			MatchedText:    finding.Value,
			Context:        finding.Context,
			StartOffset:    int64(finding.StartPos),
			EndOffset:      int64(finding.EndPos),
			Status:         "detected",
			Acknowledged:   false,
		}

		if err := db.Create(&violation).Error; err != nil {
			log.Printf("[DLPWorker] Failed to create violation for chunk %s: %v", chunkID, err)
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
	log.Printf("[DLPWorker] Health server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("[DLPWorker] Health server error: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
