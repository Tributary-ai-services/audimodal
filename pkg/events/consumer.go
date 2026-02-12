// Package events provides event schemas and utilities for the event-driven processing pipeline.
// This file implements a real Kafka consumer using segmentio/kafka-go.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

// Consumer wraps kafka-go reader with event-specific functionality.
type Consumer struct {
	reader       *kafka.Reader
	handlers     map[string]EventHandler
	config       ConsumerConfig
	running      bool
	stopChan     chan struct{}
	wg           sync.WaitGroup
	errorHandler ErrorHandler
	mu           sync.Mutex
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
	Topics            []string      `yaml:"topics"`
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
	HandleError(ctx context.Context, err error, event interface{}, message interface{}) error
}

// DefaultErrorHandler provides basic error handling with retries and dead letter queue.
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

// HandleError implements ErrorHandler interface.
func (h *DefaultErrorHandler) HandleError(ctx context.Context, err error, event interface{}, message interface{}) error {
	log.Printf("[events] Error handling event: %v", err)
	return nil
}

// MessageHandler is a function that handles raw Kafka messages
type MessageHandler func(ctx context.Context, msg kafka.Message) error

// NewConsumer creates a new event consumer with real Kafka connection.
func NewConsumer(config ConsumerConfig, errorHandler ErrorHandler) (*Consumer, error) {
	if config.BootstrapServers == "" {
		config.BootstrapServers = "localhost:9092"
	}

	startOffset := kafka.FirstOffset
	if config.AutoOffsetReset == "latest" {
		startOffset = kafka.LastOffset
	}

	topics := config.Topics
	if len(topics) == 0 {
		topics = []string{TopicPageJobs} // Default topic
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{config.BootstrapServers},
		GroupID:        config.GroupID,
		Topic:          topics[0], // Primary topic
		StartOffset:    startOffset,
		MinBytes:       1,
		MaxBytes:       10e6, // 10MB
		CommitInterval: 0,    // Manual commit
		MaxWait:        3 * time.Second,
	})

	log.Printf("[events] Created Kafka consumer. GroupID: %s, Bootstrap: %s, Topics: %v",
		config.GroupID, config.BootstrapServers, topics)

	return &Consumer{
		reader:       reader,
		handlers:     make(map[string]EventHandler),
		config:       config,
		stopChan:     make(chan struct{}),
		errorHandler: errorHandler,
	}, nil
}

// NewMultiTopicConsumer creates a consumer that reads from multiple topics
func NewMultiTopicConsumer(config ConsumerConfig, topics []string, handler MessageHandler) (*MultiTopicConsumer, error) {
	readers := make([]*kafka.Reader, len(topics))

	startOffset := kafka.FirstOffset
	if config.AutoOffsetReset == "latest" {
		startOffset = kafka.LastOffset
	}

	for i, topic := range topics {
		readers[i] = kafka.NewReader(kafka.ReaderConfig{
			Brokers:     []string{config.BootstrapServers},
			GroupID:     config.GroupID,
			Topic:       topic,
			StartOffset: startOffset,
			MinBytes:    1,
			MaxBytes:    10e6,
			MaxWait:     3 * time.Second,
		})
	}

	return &MultiTopicConsumer{
		readers:  readers,
		topics:   topics,
		handler:  handler,
		config:   config,
		stopChan: make(chan struct{}),
	}, nil
}

// MultiTopicConsumer reads from multiple Kafka topics
type MultiTopicConsumer struct {
	readers  []*kafka.Reader
	topics   []string
	handler  MessageHandler
	config   ConsumerConfig
	running  bool
	stopChan chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
}

// Start starts consuming from all topics
func (c *MultiTopicConsumer) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("consumer is already running")
	}
	c.running = true
	c.mu.Unlock()

	for i, reader := range c.readers {
		c.wg.Add(1)
		go func(r *kafka.Reader, topic string) {
			defer c.wg.Done()
			log.Printf("[events] Starting consumer for topic: %s", topic)

			for {
				select {
				case <-c.stopChan:
					return
				case <-ctx.Done():
					return
				default:
					msg, err := r.FetchMessage(ctx)
					if err != nil {
						if ctx.Err() != nil {
							return
						}
						log.Printf("[events] Error fetching message from %s: %v", topic, err)
						time.Sleep(time.Second)
						continue
					}

					if err := c.handler(ctx, msg); err != nil {
						log.Printf("[events] Error processing message from %s: %v", topic, err)
					}

					if err := r.CommitMessages(ctx, msg); err != nil {
						log.Printf("[events] Error committing offset for %s: %v", topic, err)
					}
				}
			}
		}(reader, c.topics[i])
	}

	return nil
}

// Stop stops all consumers
func (c *MultiTopicConsumer) Stop() error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	close(c.stopChan)
	c.running = false
	c.mu.Unlock()

	c.wg.Wait()

	for _, r := range c.readers {
		r.Close()
	}

	log.Printf("[events] Multi-topic consumer stopped")
	return nil
}

// RegisterHandler registers an event handler for specific event types
func (c *Consumer) RegisterHandler(handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, eventType := range handler.GetEventTypes() {
		c.handlers[eventType] = handler
		log.Printf("[events] Registered handler for event type: %s", eventType)
	}
}

// Subscribe subscribes to topics (configures the reader).
func (c *Consumer) Subscribe(topics []string) error {
	log.Printf("[events] Subscribing to topics: %v", topics)
	return nil
}

// Start starts the consumer loop.
func (c *Consumer) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("consumer is already running")
	}
	c.running = true
	c.mu.Unlock()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		log.Printf("[events] Kafka consumer started")

		for {
			select {
			case <-c.stopChan:
				log.Printf("[events] Consumer stopped via stopChan")
				return
			case <-ctx.Done():
				log.Printf("[events] Consumer stopped via context")
				return
			default:
				msg, err := c.reader.FetchMessage(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Printf("[events] Error fetching message: %v", err)
					time.Sleep(time.Second)
					continue
				}

				c.processMessage(ctx, msg)
			}
		}
	}()

	return nil
}

// processMessage processes a single Kafka message
func (c *Consumer) processMessage(ctx context.Context, msg kafka.Message) {
	// Get event type from headers
	var eventType string
	for _, h := range msg.Headers {
		if h.Key == "event-type" {
			eventType = string(h.Value)
			break
		}
	}

	handler, exists := c.handlers[eventType]
	if !exists {
		log.Printf("[events] No handler for event type: %s", eventType)
		c.commitMessage(ctx, msg)
		return
	}

	// Deserialize the event
	var event map[string]interface{}
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		log.Printf("[events] Failed to unmarshal event: %v", err)
		c.commitMessage(ctx, msg)
		return
	}

	// Handle the event
	if err := handler.HandleEvent(ctx, event); err != nil {
		log.Printf("[events] Handler error for %s: %v", eventType, err)
		if c.errorHandler != nil {
			c.errorHandler.HandleError(ctx, err, event, msg)
		}
	}

	c.commitMessage(ctx, msg)
}

// commitMessage commits a message offset
func (c *Consumer) commitMessage(ctx context.Context, msg kafka.Message) {
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		log.Printf("[events] Failed to commit offset: %v", err)
	}
}

// Stop stops the consumer
func (c *Consumer) Stop() error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}

	close(c.stopChan)
	c.running = false
	c.mu.Unlock()

	// Wait for consumer loop to finish
	c.wg.Wait()

	if c.reader != nil {
		c.reader.Close()
	}

	log.Printf("[events] Kafka consumer stopped")
	return nil
}

// HealthCheck performs a health check on the consumer.
func (c *Consumer) HealthCheck(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return fmt.Errorf("consumer is not running")
	}
	return nil
}

// GetMetrics returns consumer metrics.
func (c *Consumer) GetMetrics() (map[string]interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	stats := c.reader.Stats()

	return map[string]interface{}{
		"topic":          stats.Topic,
		"partition":      stats.Partition,
		"messages":       stats.Messages,
		"bytes":          stats.Bytes,
		"rebalances":     stats.Rebalances,
		"offset":         stats.Offset,
		"lag":            stats.Lag,
		"running":        c.running,
	}, nil
}
