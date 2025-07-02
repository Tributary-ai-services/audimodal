package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Producer wraps Kafka producer with event-specific functionality
type Producer struct {
	producer  *kafka.Producer
	validator *EventValidator
	tracer    trace.Tracer
	config    ProducerConfig
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

// NewProducer creates a new event producer
func NewProducer(config ProducerConfig) (*Producer, error) {
	// Build Kafka configuration
	kafkaConfig := &kafka.ConfigMap{
		"bootstrap.servers":                     config.BootstrapServers,
		"security.protocol":                     config.SecurityProtocol,
		"client.id":                             config.ClientID,
		"acks":                                  config.Acks,
		"retries":                               config.Retries,
		"retry.backoff.ms":                      int(config.RetryBackoff.Milliseconds()),
		"batch.size":                            config.BatchSize,
		"linger.ms":                             config.LingerMS,
		"compression.type":                      config.CompressionType,
		"enable.idempotence":                    true,
		"max.in.flight.requests.per.connection": 1,
	}

	// Add SASL configuration if needed
	if config.SecurityProtocol != "PLAINTEXT" {
		kafkaConfig.SetKey("sasl.mechanism", config.SASLMechanism)
		kafkaConfig.SetKey("sasl.username", config.SASLUsername)
		kafkaConfig.SetKey("sasl.password", config.SASLPassword)
	}

	// Create Kafka producer
	producer, err := kafka.NewProducer(kafkaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	return &Producer{
		producer:  producer,
		validator: NewEventValidator(),
		tracer:    otel.Tracer("event-producer"),
		config:    config,
	}, nil
}

// PublishEvent publishes an event to the appropriate topic
func (p *Producer) PublishEvent(ctx context.Context, event interface{}) error {
	ctx, span := p.tracer.Start(ctx, "publish_event")
	defer span.End()

	// Validate event
	if err := p.validator.ValidateEvent(event); err != nil {
		span.RecordError(err)
		return fmt.Errorf("event validation failed: %w", err)
	}

	// Determine topic and partition key
	topic, partitionKey, eventType := p.getTopicAndKey(event)

	// Serialize event
	eventData, err := json.Marshal(event)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to serialize event: %w", err)
	}

	// Create envelope with metadata
	envelope := EventEnvelope{
		Event:       eventData,
		EventType:   eventType,
		TenantID:    p.extractTenantID(event),
		Priority:    p.getEventPriority(event),
		PublishedAt: time.Now(),
		Retries:     0,
	}

	envelopeData, err := json.Marshal(envelope)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to serialize envelope: %w", err)
	}

	// Add tracing headers
	headers := p.addTracingHeaders(ctx)

	// Create Kafka message
	message := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: kafka.PartitionAny,
		},
		Key:     []byte(partitionKey),
		Value:   envelopeData,
		Headers: headers,
	}

	// Publish message
	deliveryChan := make(chan kafka.Event)
	err = p.producer.Produce(message, deliveryChan)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to produce message: %w", err)
	}

	// Wait for delivery confirmation
	select {
	case e := <-deliveryChan:
		m := e.(*kafka.Message)
		if m.TopicPartition.Error != nil {
			span.RecordError(m.TopicPartition.Error)
			return fmt.Errorf("message delivery failed: %w", m.TopicPartition.Error)
		}

		// Add success attributes to span
		span.SetAttributes(
			attribute.String("kafka.topic", topic),
			attribute.String("kafka.partition", fmt.Sprintf("%d", m.TopicPartition.Partition)),
			attribute.String("kafka.offset", fmt.Sprintf("%d", m.TopicPartition.Offset)),
			attribute.String("event.type", eventType),
			attribute.String("tenant.id", envelope.TenantID),
		)

	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// PublishBatch publishes multiple events in a batch for better performance
func (p *Producer) PublishBatch(ctx context.Context, events []interface{}) error {
	ctx, span := p.tracer.Start(ctx, "publish_batch")
	defer span.End()

	span.SetAttributes(attribute.Int("batch.size", len(events)))

	results := make(chan error, len(events))

	// Publish all events concurrently
	for _, event := range events {
		go func(e interface{}) {
			results <- p.PublishEvent(ctx, e)
		}(event)
	}

	// Collect results
	var errors []error
	for i := 0; i < len(events); i++ {
		if err := <-results; err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		span.SetAttributes(attribute.Int("batch.errors", len(errors)))
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

// getEventPriority determines event priority for processing order
func (p *Producer) getEventPriority(event interface{}) int {
	switch event.(type) {
	case *DLPViolationEvent:
		return 10 // Highest priority
	case *ProcessingFailedEvent:
		return 8
	case *FileDiscoveredEvent:
		return 5 // Normal priority
	case *SessionStartedEvent, *SessionCompletedEvent:
		return 3
	default:
		return 5 // Default priority
	}
}

// addTracingHeaders adds OpenTelemetry tracing headers to Kafka message
func (p *Producer) addTracingHeaders(ctx context.Context) []kafka.Header {
	if !p.config.EnableTracing {
		return nil
	}

	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return nil
	}

	spanContext := span.SpanContext()
	traceID := spanContext.TraceID().String()
	spanID := spanContext.SpanID().String()

	return []kafka.Header{
		{Key: "trace-id", Value: []byte(traceID)},
		{Key: "span-id", Value: []byte(spanID)},
		{Key: "trace-flags", Value: []byte(fmt.Sprintf("%d", spanContext.TraceFlags()))},
	}
}

// Flush waits for all outstanding messages to be delivered
func (p *Producer) Flush(timeout time.Duration) error {
	remaining := p.producer.Flush(int(timeout.Milliseconds()))
	if remaining > 0 {
		return fmt.Errorf("failed to flush %d messages within timeout", remaining)
	}
	return nil
}

// Close closes the producer and releases resources
func (p *Producer) Close() error {
	// Flush any remaining messages
	p.producer.Flush(5000) // 5 second timeout

	// Close producer
	p.producer.Close()
	return nil
}

// GetMetrics returns producer metrics
func (p *Producer) GetMetrics() (map[string]interface{}, error) {
	metadata, err := p.producer.GetMetadata(nil, false, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	return map[string]interface{}{
		"broker_count":  len(metadata.Brokers),
		"topic_count":   len(metadata.Topics),
		"out_queue_len": p.producer.Len(),
	}, nil
}

// HealthCheck performs a health check on the producer
func (p *Producer) HealthCheck(ctx context.Context) error {
	// Try to get metadata as a health check
	_, err := p.producer.GetMetadata(nil, false, 1000)
	if err != nil {
		return fmt.Errorf("producer health check failed: %w", err)
	}
	return nil
}
