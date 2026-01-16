// Package events provides event schemas and utilities for the event-driven processing pipeline.
// This file contains stub implementations for the Kafka producer when the Confluent library is not available.
// For full Kafka support, rename producer.go.confluent to producer.go and ensure CGO is enabled.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// Producer wraps Kafka producer with event-specific functionality.
// This is a stub implementation that logs events instead of sending to Kafka.
// For production use with Kafka, use the Confluent implementation.
type Producer struct {
	validator *EventValidator
	config    ProducerConfig
	mu        sync.Mutex
	closed    bool
}

// ProducerConfig contains configuration for the event producer
type ProducerConfig struct {
	BootstrapServers string        `yaml:"bootstrap_servers"`
	SecurityProtocol string        `yaml:"security_protocol"`
	SASLMechanism    string        `yaml:"sasl_mechanism"`
	SASLUsername     string        `yaml:"sasl_username"`
	SASLPassword     string        `yaml:"sasl_password"`
	ClientID         string        `yaml:"client_id"`
	Acks             string        `yaml:"acks"` // all, 1, 0
	Retries          int           `yaml:"retries"`
	RetryBackoff     time.Duration `yaml:"retry_backoff"`
	BatchSize        int           `yaml:"batch_size"`
	LingerMS         int           `yaml:"linger_ms"`
	CompressionType  string        `yaml:"compression_type"` // gzip, snappy, lz4, zstd
	EnableTracing    bool          `yaml:"enable_tracing"`
}

// DefaultProducerConfig returns a production-ready default configuration
func DefaultProducerConfig() ProducerConfig {
	return ProducerConfig{
		BootstrapServers: "localhost:9092",
		SecurityProtocol: "PLAINTEXT",
		ClientID:         "document-processing-producer",
		Acks:             "all",
		Retries:          3,
		RetryBackoff:     100 * time.Millisecond,
		BatchSize:        16384,
		LingerMS:         5,
		CompressionType:  "gzip",
		EnableTracing:    true,
	}
}

// NewProducer creates a new event producer.
// This stub implementation logs events locally instead of sending to Kafka.
func NewProducer(config ProducerConfig) (*Producer, error) {
	log.Printf("[events] Creating stub producer (Kafka disabled). Events will be logged locally. Bootstrap: %s", config.BootstrapServers)

	return &Producer{
		validator: NewEventValidator(),
		config:    config,
	}, nil
}

// PublishEvent publishes an event.
// Stub implementation: logs the event instead of sending to Kafka.
func (p *Producer) PublishEvent(ctx context.Context, event interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return fmt.Errorf("producer is closed")
	}

	// Validate event
	if err := p.validator.ValidateEvent(event); err != nil {
		return fmt.Errorf("event validation failed: %w", err)
	}

	// Determine topic and event type
	topic, _, eventType := p.getTopicAndKey(event)

	// Serialize event for logging
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to serialize event: %w", err)
	}

	// Log the event (stub behavior)
	log.Printf("[events] STUB PublishEvent: topic=%s type=%s tenant=%s data=%s",
		topic, eventType, p.extractTenantID(event), string(eventData))

	return nil
}

// PublishBatch publishes multiple events in a batch.
// Stub implementation: logs each event.
func (p *Producer) PublishBatch(ctx context.Context, events []interface{}) error {
	var errors []error

	for _, event := range events {
		if err := p.PublishEvent(ctx, event); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("batch publish failed with %d errors: %v", len(errors), errors[0])
	}

	return nil
}

// getTopicAndKey determines the Kafka topic and partition key for an event
func (p *Producer) getTopicAndKey(event interface{}) (topic, partitionKey, eventType string) {
	switch e := event.(type) {
	case *FileDiscoveredEvent:
		return "file.discovery", e.TenantID, EventFileDiscovered
	case *FileResolvedEvent:
		return "file.resolved", e.TenantID, EventFileResolved
	case *FileClassifiedEvent:
		return "file.classified", e.TenantID, EventFileClassified
	case *FileChunkedEvent:
		return "file.chunked", e.TenantID, EventFileChunked
	case *ChunkEmbeddedEvent:
		return "chunk.embedded", e.TenantID, EventChunkEmbedded
	case *DLPViolationEvent:
		return "dlp.violations", e.TenantID, EventDLPViolation
	case *ProcessingCompleteEvent:
		return "processing.complete", e.TenantID, EventProcessingComplete
	case *ProcessingFailedEvent:
		return "processing.failed", e.TenantID, EventProcessingFailed
	case *SessionStartedEvent:
		return "session.lifecycle", e.TenantID, EventSessionStarted
	case *SessionCompletedEvent:
		return "session.lifecycle", e.TenantID, EventSessionCompleted
	default:
		return "unknown", "default", "unknown"
	}
}

// extractTenantID extracts tenant ID from any event type
func (p *Producer) extractTenantID(event interface{}) string {
	switch e := event.(type) {
	case *FileDiscoveredEvent:
		return e.TenantID
	case *FileResolvedEvent:
		return e.TenantID
	case *FileClassifiedEvent:
		return e.TenantID
	case *FileChunkedEvent:
		return e.TenantID
	case *ChunkEmbeddedEvent:
		return e.TenantID
	case *DLPViolationEvent:
		return e.TenantID
	case *ProcessingCompleteEvent:
		return e.TenantID
	case *ProcessingFailedEvent:
		return e.TenantID
	case *SessionStartedEvent:
		return e.TenantID
	case *SessionCompletedEvent:
		return e.TenantID
	default:
		return "unknown"
	}
}

// Flush waits for all outstanding messages to be delivered.
// Stub implementation: no-op since there's no real queue.
func (p *Producer) Flush(timeout time.Duration) error {
	return nil
}

// Close closes the producer and releases resources
func (p *Producer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	log.Printf("[events] Stub producer closed")
	return nil
}

// GetMetrics returns producer metrics.
// Stub implementation: returns empty metrics.
func (p *Producer) GetMetrics() (map[string]interface{}, error) {
	return map[string]interface{}{
		"broker_count":  0,
		"topic_count":   0,
		"out_queue_len": 0,
		"stub_mode":     true,
	}, nil
}

// HealthCheck performs a health check on the producer.
// Stub implementation: always returns healthy.
func (p *Producer) HealthCheck(ctx context.Context) error {
	if p.closed {
		return fmt.Errorf("producer is closed")
	}
	return nil
}
