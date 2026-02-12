package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

// SimpleProducer is a pure Go Kafka producer that doesn't require CGO
type SimpleProducer struct {
	writer *kafka.Writer
	config SimpleProducerConfig
}

// SimpleProducerConfig contains configuration for the simple producer
type SimpleProducerConfig struct {
	BootstrapServers string
	ClientID         string
}

// NewSimpleProducer creates a new simple Kafka producer using pure Go library
func NewSimpleProducer(cfg SimpleProducerConfig) (*SimpleProducer, error) {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(cfg.BootstrapServers),
		Balancer:     &kafka.LeastBytes{},
		BatchTimeout: 10 * time.Millisecond,
		MaxAttempts:  3,
		WriteTimeout: 10 * time.Second,
		RequiredAcks: kafka.RequireAll,
	}

	return &SimpleProducer{
		writer: writer,
		config: cfg,
	}, nil
}

// EventWithBase is an interface for events that have a BaseEvent
type EventWithBase interface {
	GetBase() BaseEvent
}

// PublishEvent publishes an event to the appropriate Kafka topic
func (p *SimpleProducer) PublishEvent(ctx context.Context, event EventWithBase) error {
	base := event.GetBase()

	// Determine topic based on event type
	topic := p.getTopicForEvent(base.Type)

	// Serialize the full event (not just base)
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to serialize event: %w", err)
	}

	// Create Kafka message
	message := kafka.Message{
		Topic: topic,
		Key:   []byte(base.TenantID),
		Value: eventData,
		Headers: []kafka.Header{
			{Key: "event-type", Value: []byte(base.Type)},
			{Key: "event-id", Value: []byte(base.ID)},
			{Key: "source", Value: []byte(base.Source)},
			{Key: "version", Value: []byte("1.0")},
		},
		Time: base.Time,
	}

	// Write message
	err = p.writer.WriteMessages(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to write message to Kafka: %w", err)
	}

	return nil
}

// getTopicForEvent returns the appropriate topic for an event type
func (p *SimpleProducer) getTopicForEvent(eventType string) string {
	topicMap := map[string]string{
		EventProcessingComplete: "processing.complete",
		EventProcessingFailed:   "processing.failed",
	}

	if topic, exists := topicMap[eventType]; exists {
		return topic
	}

	return "events"
}

// WriteMessages writes raw Kafka messages directly (for batch operations like Splitter).
func (p *SimpleProducer) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	return p.writer.WriteMessages(ctx, msgs...)
}

// Close closes the producer
func (p *SimpleProducer) Close() error {
	return p.writer.Close()
}
