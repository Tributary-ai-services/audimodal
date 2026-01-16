// Package events provides event schemas and utilities for the event-driven processing pipeline.
// This file contains stub implementations for the Kafka consumer when the Confluent library is not available.
// For full Kafka support, rename consumer.go.confluent to consumer.go and ensure CGO is enabled.
package events

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Consumer wraps Kafka consumer with event-specific functionality.
// This is a stub implementation that doesn't actually consume from Kafka.
// For production use with Kafka, use the Confluent implementation.
type Consumer struct {
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
// This is a stub implementation.
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
// Stub implementation: logs the error.
func (h *DefaultErrorHandler) HandleError(ctx context.Context, err error, event interface{}, message interface{}) error {
	log.Printf("[events] STUB HandleError: error=%v event=%T", err, event)
	return nil
}

// NewConsumer creates a new event consumer.
// This stub implementation doesn't actually connect to Kafka.
func NewConsumer(config ConsumerConfig, errorHandler ErrorHandler) (*Consumer, error) {
	log.Printf("[events] Creating stub consumer (Kafka disabled). GroupID: %s, Bootstrap: %s",
		config.GroupID, config.BootstrapServers)

	return &Consumer{
		handlers:     make(map[string]EventHandler),
		config:       config,
		stopChan:     make(chan struct{}),
		errorHandler: errorHandler,
	}, nil
}

// RegisterHandler registers an event handler for specific event types
func (c *Consumer) RegisterHandler(handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, eventType := range handler.GetEventTypes() {
		c.handlers[eventType] = handler
		log.Printf("[events] STUB: Registered handler for event type: %s", eventType)
	}
}

// Subscribe subscribes to topics.
// Stub implementation: logs the subscription.
func (c *Consumer) Subscribe(topics []string) error {
	log.Printf("[events] STUB Subscribe: topics=%v", topics)
	return nil
}

// Start starts the consumer loop.
// Stub implementation: starts a goroutine that does nothing but wait.
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
		log.Printf("[events] STUB consumer started (no-op mode)")

		select {
		case <-c.stopChan:
			log.Printf("[events] STUB consumer stopped via stopChan")
		case <-ctx.Done():
			log.Printf("[events] STUB consumer stopped via context")
		}
	}()

	return nil
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

	log.Printf("[events] STUB consumer stopped")
	return nil
}

// HealthCheck performs a health check on the consumer.
// Stub implementation: returns healthy if running.
func (c *Consumer) HealthCheck(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return fmt.Errorf("consumer is not running")
	}
	return nil
}

// GetMetrics returns consumer metrics.
// Stub implementation: returns empty metrics.
func (c *Consumer) GetMetrics() (map[string]interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return map[string]interface{}{
		"broker_count":        0,
		"topic_count":         0,
		"assigned_partitions": 0,
		"running":             c.running,
		"stub_mode":           true,
	}, nil
}
