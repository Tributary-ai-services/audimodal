// Backfill utility: publishes DLP job messages for all chunks with pending DLP scan status.
// This is a one-time script for chunks created before the DLP worker was deployed.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/jscharber/audimodal/internal/database/models"
	"github.com/jscharber/audimodal/pkg/events"
)

func main() {
	log.Println("[Backfill] Starting DLP backfill for pending chunks...")

	db, err := connectDB()
	if err != nil {
		log.Fatalf("[Backfill] Failed to connect to database: %v", err)
	}

	bootstrapServers := getEnv("KAFKA_BOOTSTRAP_SERVERS", "kafka-shared.tas-shared.svc.cluster.local:9092")

	writer := &kafka.Writer{
		Addr:         kafka.TCP(bootstrapServers),
		Topic:        events.TopicDLPJobs,
		Balancer:     &kafka.Hash{},
		BatchTimeout: 100 * time.Millisecond,
		RequiredAcks: kafka.RequireOne,
	}
	defer writer.Close()

	// Query all chunks with pending DLP status, joining files for tenant_id
	type chunkRow struct {
		ID             uuid.UUID
		FileID         uuid.UUID
		S3Bucket       string
		S3Key          string
		ContentPreview string
		TenantID       uuid.UUID
	}

	var rows []chunkRow
	err = db.Raw(`
		SELECT c.id, c.file_id, c.s3_bucket, c.s3_key, c.content_preview, f.tenant_id
		FROM chunks c
		JOIN files f ON c.file_id = f.id
		WHERE c.dlp_scan_status = ?
	`, models.DLPScanStatusPending).Scan(&rows).Error
	if err != nil {
		log.Fatalf("[Backfill] Failed to query pending chunks: %v", err)
	}

	log.Printf("[Backfill] Found %d pending chunks to backfill", len(rows))

	ctx := context.Background()
	published := 0
	failed := 0

	for _, row := range rows {
		msg := events.DLPJobMessage{
			MessageID:      uuid.New().String(),
			TenantID:       row.TenantID.String(),
			FileID:         row.FileID.String(),
			ChunkID:        row.ID.String(),
			S3Bucket:       row.S3Bucket,
			S3Key:          row.S3Key,
			ContentPreview: row.ContentPreview,
			ProducedAt:     time.Now(),
		}

		value, _ := json.Marshal(msg)
		err := writer.WriteMessages(ctx, kafka.Message{
			Key:   []byte(msg.PartitionKey()),
			Value: value,
		})
		if err != nil {
			log.Printf("[Backfill] Failed to publish DLP job for chunk %s: %v", row.ID, err)
			failed++
			continue
		}
		published++
	}

	log.Printf("[Backfill] Done. Published: %d, Failed: %d", published, failed)
}

func connectDB() (*gorm.DB, error) {
	host := getEnv("DB_HOST", "postgres-shared.tas-shared.svc.cluster.local")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USERNAME", "tasuser")
	pass := getEnv("DB_PASSWORD", "taspassword")
	name := getEnv("DB_DATABASE", "audimodal")
	sslMode := getEnv("DB_SSL_MODE", "disable")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, pass, name, sslMode)

	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
