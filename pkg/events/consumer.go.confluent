package events

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// EventHandler interface is now defined in types.go

// Consumer wraps Kafka consumer with event-specific functionality
type Consumer struct {
	consumer     *kafka.Consumer
	handlers     map[string]EventHandler
	tracer       trace.Tracer
	config       ConsumerConfig
	running      bool
	stopChan     chan struct{}
	wg           sync.WaitGroup
	errorHandler ErrorHandler
}

// ConsumerConfig contains configuration for the event consumer
type ConsumerConfig struct {
	BootstrapServers  string        `yaml:"bootstrap_servers"`
	SecurityProtocol  string        `yaml:"security_protocol"`
	SASLMechanism     string        `yaml:"sasl_mechanism"`
	SASLUsername      string        `yaml:"sasl_username"`
	SASLPassword      string        `yaml:"sasl_password"`
	GroupID           string        `yaml:"group_id"`
	ClientID          string        `yaml:"client_id"`
	AutoOffsetReset   string        `yaml:"auto_offset_reset"` // earliest, latest
	EnableAutoCommit  bool          `yaml:"enable_auto_commit"`
	MaxPollRecords    int           `yaml:"max_poll_records"`
	SessionTimeout    time.Duration `yaml:"session_timeout"`
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`
	EnableTracing     bool          `yaml:"enable_tracing"`
	RetryAttempts     int           `yaml:"retry_attempts"`
	RetryDelay        time.Duration `yaml:"retry_delay"`
}

// DefaultConsumerConfig returns a production-ready default configuration
func DefaultConsumerConfig(groupID string) ConsumerConfig {
	return ConsumerConfig{
		BootstrapServers:  "localhost:9092",
		SecurityProtocol:  "PLAINTEXT",
		GroupID:           groupID,
		ClientID:          fmt.Sprintf("document-processing-consumer-%s", groupID),
		AutoOffsetReset:   "earliest",
		EnableAutoCommit:  false, // Manual commit for reliability
		MaxPollRecords:    500,
		SessionTimeout:    30 * time.Second,
		HeartbeatInterval: 3 * time.Second,
		EnableTracing:     true,
		RetryAttempts:     3,
		RetryDelay:        1 * time.Second,
	}
}

// ErrorHandler defines how to handle processing errors
type ErrorHandler interface {
	HandleError(ctx context.Context, err error, event interface{}, message *kafka.Message) error
}

// DefaultErrorHandler provides basic error handling with retries and dead letter queue
type DefaultErrorHandler struct {
	producer        *Producer
	maxRetries      int
	deadLetterTopic string
}

// NewDefaultErrorHandler creates a new default error handler
func NewDefaultErrorHandler(producer *Producer, maxRetries int, deadLetterTopic string) *DefaultErrorHandler {
	return &DefaultErrorHandler{
		producer:        producer,
		maxRetries:      maxRetries,
		deadLetterTopic: deadLetterTopic,
	}
}

// HandleError implements ErrorHandler interface
func (h *DefaultErrorHandler) HandleError(ctx context.Context, err error, event interface{}, message *kafka.Message) error {
	// Extract retry count from headers
	retryCount := 0
	for _, header := range message.Headers {
		if header.Key == "retry-count" {
			fmt.Sscanf(string(header.Value), "%d", &retryCount)
			break
		}
	}

	if retryCount < h.maxRetries {
		// Retry the message
		return h.retryMessage(ctx, message, retryCount+1)
	}

	// Send to dead letter queue
	return h.sendToDeadLetter(ctx, message, err)
}

func (h *DefaultErrorHandler) retryMessage(ctx context.Context, message *kafka.Message, retryCount int) error {
	// Add retry headers
	headers := append(message.Headers, kafka.Header{
		Key:   "retry-count",
		Value: []byte(fmt.Sprintf("%d", retryCount)),
	})

	retryMessage := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     message.TopicPartition.Topic,
			Partition: kafka.PartitionAny,
		},
		Key:     message.Key,
		Value:   message.Value,
		Headers: headers,
	}

	return h.producer.producer.Produce(retryMessage, nil)
}

func (h *DefaultErrorHandler) sendToDeadLetter(ctx context.Context, message *kafka.Message, processingError error) error {
	deadLetterMessage := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &h.deadLetterTopic,
			Partition: kafka.PartitionAny,
		},
		Key:   message.Key,
		Value: message.Value,
		Headers: append(message.Headers,
			kafka.Header{Key: "original-topic", Value: []byte(*message.TopicPartition.Topic)},
			kafka.Header{Key: "error", Value: []byte(processingError.Error())},
			kafka.Header{Key: "failed-at", Value: []byte(time.Now().Format(time.RFC3339))},
		),
	}

	return h.producer.producer.Produce(deadLetterMessage, nil)
}

// NewConsumer creates a new event consumer
func NewConsumer(config ConsumerConfig, errorHandler ErrorHandler) (*Consumer, error) {
	// Build Kafka configuration
	kafkaConfig := &kafka.ConfigMap{
		"bootstrap.servers":               config.BootstrapServers,
		"security.protocol":               config.SecurityProtocol,
		"group.id":                        config.GroupID,
		"client.id":                       config.ClientID,
		"auto.offset.reset":               config.AutoOffsetReset,
		"enable.auto.commit":              config.EnableAutoCommit,
		"max.poll.records":                config.MaxPollRecords,
		"session.timeout.ms":              int(config.SessionTimeout.Milliseconds()),
		"heartbeat.interval.ms":           int(config.HeartbeatInterval.Milliseconds()),
		"go.application.rebalance.enable": true,
	}

	// Add SASL configuration if needed
	if config.SecurityProtocol != "PLAINTEXT" {
		kafkaConfig.SetKey("sasl.mechanism", config.SASLMechanism)
		kafkaConfig.SetKey("sasl.username", config.SASLUsername)
		kafkaConfig.SetKey("sasl.password", config.SASLPassword)
	}

	// Create Kafka consumer
	consumer, err := kafka.NewConsumer(kafkaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka consumer: %w", err)
	}

	return &Consumer{
		consumer:     consumer,
		handlers:     make(map[string]EventHandler),
		tracer:       otel.Tracer("event-consumer"),
		config:       config,
		stopChan:     make(chan struct{}),
		errorHandler: errorHandler,
	}, nil
}

// RegisterHandler registers an event handler for specific event types
func (c *Consumer) RegisterHandler(handler EventHandler) {
	for _, eventType := range handler.GetEventTypes() {
		c.handlers[eventType] = handler
	}
}

// Subscribe subscribes to topics and starts consuming
func (c *Consumer) Subscribe(topics []string) error {
	return c.consumer.SubscribeTopics(topics, nil)
}

// Start starts the consumer loop
func (c *Consumer) Start(ctx context.Context) error {
	if c.running {
		return fmt.Errorf("consumer is already running")
	}

	c.running = true
	c.wg.Add(1)

	go func() {
		defer c.wg.Done()
		c.consume(ctx)
	}()

	return nil
}

// consume is the main consumer loop
func (c *Consumer) consume(ctx context.Context) {
	for {
		select {
		case <-c.stopChan:
			return
		case <-ctx.Done():
			return
		default:
			message, err := c.consumer.ReadMessage(100 * time.Millisecond)
			if err != nil {
				if err.(kafka.Error).Code() == kafka.ErrTimedOut {
					continue // Timeout is expected, continue polling
				}
				fmt.Printf("Consumer error: %v\n", err)
				continue
			}

			if err := c.processMessage(ctx, message); err != nil {
				fmt.Printf("Failed to process message: %v\n", err)
			}
		}
	}
}

// processMessage processes a single Kafka message
func (c *Consumer) processMessage(ctx context.Context, message *kafka.Message) error {
	ctx, span := c.tracer.Start(ctx, "process_message")
	defer span.End()

	// Extract tracing information if available
	if c.config.EnableTracing {
		ctx = c.extractTracingContext(ctx, message)
	}

	// Deserialize envelope
	var envelope EventEnvelope
	if err := json.Unmarshal(message.Value, &envelope); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to deserialize envelope: %w", err)
	}

	// Add span attributes
	span.SetAttributes(
		attribute.String("kafka.topic", *message.TopicPartition.Topic),
		attribute.String("kafka.partition", fmt.Sprintf("%d", message.TopicPartition.Partition)),
		attribute.String("kafka.offset", fmt.Sprintf("%d", message.TopicPartition.Offset)),
		attribute.String("event.type", envelope.EventType),
		attribute.String("tenant.id", envelope.TenantID),
	)

	// Find handler for event type
	handler, exists := c.handlers[envelope.EventType]
	if !exists {
		return fmt.Errorf("no handler registered for event type: %s", envelope.EventType)
	}

	// Deserialize actual event based on type
	event, err := c.deserializeEvent(envelope.EventType, envelope.Event)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to deserialize event: %w", err)
	}

	// Process event with retry logic
	var lastErr error
	for attempt := 0; attempt <= c.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			// Wait before retry
			time.Sleep(c.config.RetryDelay * time.Duration(attempt))
			span.AddEvent("retrying", trace.WithAttributes(
				attribute.Int("attempt", attempt),
			))
		}

		err = handler.HandleEvent(ctx, event)
		if err == nil {
			// Success - commit offset
			if !c.config.EnableAutoCommit {
				_, err := c.consumer.CommitMessage(message)
				if err != nil {
					span.RecordError(err)
					return fmt.Errorf("failed to commit offset: %w", err)
				}
			}
			return nil
		}

		lastErr = err
		span.RecordError(err)
	}

	// All retries failed - use error handler
	if c.errorHandler != nil {
		return c.errorHandler.HandleError(ctx, lastErr, event, message)
	}

	return lastErr
}

// deserializeEvent deserializes an event based on its type
func (c *Consumer) deserializeEvent(eventType string, eventData json.RawMessage) (interface{}, error) {
	switch eventType {
	case EventFileDiscovered:
		var event FileDiscoveredEvent
		err := json.Unmarshal(eventData, &event)
		return &event, err
	case EventFileResolved:
		var event FileResolvedEvent
		err := json.Unmarshal(eventData, &event)
		return &event, err
	case EventFileClassified:
		var event FileClassifiedEvent
		err := json.Unmarshal(eventData, &event)
		return &event, err
	case EventFileChunked:
		var event FileChunkedEvent
		err := json.Unmarshal(eventData, &event)
		return &event, err
	case EventChunkEmbedded:
		var event ChunkEmbeddedEvent
		err := json.Unmarshal(eventData, &event)
		return &event, err
	case EventDLPViolation:
		var event DLPViolationEvent
		err := json.Unmarshal(eventData, &event)
		return &event, err
	case EventProcessingComplete:
		var event ProcessingCompleteEvent
		err := json.Unmarshal(eventData, &event)
		return &event, err
	case EventProcessingFailed:
		var event ProcessingFailedEvent
		err := json.Unmarshal(eventData, &event)
		return &event, err
	case EventSessionStarted:
		var event SessionStartedEvent
		err := json.Unmarshal(eventData, &event)
		return &event, err
	case EventSessionCompleted:
		var event SessionCompletedEvent
		err := json.Unmarshal(eventData, &event)
		return &event, err
	default:
		return nil, fmt.Errorf("unknown event type: %s", eventType)
	}
}

// extractTracingContext extracts OpenTelemetry context from Kafka headers
func (c *Consumer) extractTracingContext(ctx context.Context, message *kafka.Message) context.Context {
	var traceID, spanID string

	for _, header := range message.Headers {
		switch header.Key {
		case "trace-id":
			traceID = string(header.Value)
		case "span-id":
			spanID = string(header.Value)
		}
	}

	if traceID != "" && spanID != "" {
		// In a real implementation, you would reconstruct the trace context
		// This is simplified for the example
		ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{}))
	}

	return ctx
}

// Stop stops the consumer
func (c *Consumer) Stop() error {
	if !c.running {
		return nil
	}

	close(c.stopChan)
	c.running = false

	// Wait for consumer loop to finish
	c.wg.Wait()

	// Close consumer
	return c.consumer.Close()
}

// HealthCheck performs a health check on the consumer
func (c *Consumer) HealthCheck(ctx context.Context) error {
	// Check if consumer is running
	if !c.running {
		return fmt.Errorf("consumer is not running")
	}

	// Try to get metadata as a health check
	_, err := c.consumer.GetMetadata(nil, false, 1000)
	if err != nil {
		return fmt.Errorf("consumer health check failed: %w", err)
	}

	return nil
}

// GetMetrics returns consumer metrics
func (c *Consumer) GetMetrics() (map[string]interface{}, error) {
	metadata, err := c.consumer.GetMetadata(nil, false, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	assignment, err := c.consumer.Assignment()
	if err != nil {
		return nil, fmt.Errorf("failed to get assignment: %w", err)
	}

	return map[string]interface{}{
		"broker_count":        len(metadata.Brokers),
		"topic_count":         len(metadata.Topics),
		"assigned_partitions": len(assignment),
		"running":             c.running,
	}, nil
}
