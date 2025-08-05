package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// InMemoryEventBus provides an in-memory event bus implementation for development and testing
type InMemoryEventBus struct {
	handlers    map[string][]EventHandler
	subscribers map[EventHandler][]string
	eventQueue  chan *Event
	tracer      trace.Tracer
	metrics     *EventBusMetrics

	// Configuration
	config EventBusConfig

	// Lifecycle
	running  bool
	stopChan chan struct{}
	wg       sync.WaitGroup
	mu       sync.RWMutex
}

// EventBusConfig contains configuration for the event bus
type EventBusConfig struct {
	QueueSize         int           `yaml:"queue_size"`
	WorkerCount       int           `yaml:"worker_count"`
	ProcessingTimeout time.Duration `yaml:"processing_timeout"`
	EnableMetrics     bool          `yaml:"enable_metrics"`
	EnableTracing     bool          `yaml:"enable_tracing"`
	RetryAttempts     int           `yaml:"retry_attempts"`
	RetryDelay        time.Duration `yaml:"retry_delay"`
	MaxRetryDelay     time.Duration `yaml:"max_retry_delay"`
	DeadLetterEnabled bool          `yaml:"dead_letter_enabled"`
}

// DefaultEventBusConfig returns default configuration
func DefaultEventBusConfig() EventBusConfig {
	return EventBusConfig{
		QueueSize:         10000,
		WorkerCount:       10,
		ProcessingTimeout: 30 * time.Second,
		EnableMetrics:     true,
		EnableTracing:     true,
		RetryAttempts:     3,
		RetryDelay:        1 * time.Second,
		MaxRetryDelay:     60 * time.Second,
		DeadLetterEnabled: true,
	}
}

// NewInMemoryEventBus creates a new in-memory event bus
func NewInMemoryEventBus(config EventBusConfig) *InMemoryEventBus {
	return &InMemoryEventBus{
		handlers:    make(map[string][]EventHandler),
		subscribers: make(map[EventHandler][]string),
		eventQueue:  make(chan *Event, config.QueueSize),
		tracer:      otel.Tracer("event-bus"),
		metrics:     &EventBusMetrics{HandlerMetrics: make(map[string]*HandlerMetrics)},
		config:      config,
		stopChan:    make(chan struct{}),
	}
}

// Event represents a unified event structure for the bus
type Event struct {
	ID        uuid.UUID  `json:"id"`
	Type      string     `json:"type"`
	TenantID  uuid.UUID  `json:"tenant_id"`
	SessionID *uuid.UUID `json:"session_id,omitempty"`
	FileID    *uuid.UUID `json:"file_id,omitempty"`
	ChunkID   *uuid.UUID `json:"chunk_id,omitempty"`
	SourceID  *uuid.UUID `json:"source_id,omitempty"`

	// Event metadata
	Priority int `json:"priority"`
	Version  int `json:"version"`

	// Event payload - flexible JSON data
	Payload  map[string]interface{} `json:"payload"`
	Metadata map[string]interface{} `json:"metadata"`

	// Workflow context
	WorkflowID *uuid.UUID `json:"workflow_id,omitempty"`
	StepName   string     `json:"step_name,omitempty"`
	RetryCount int        `json:"retry_count"`
	MaxRetries int        `json:"max_retries"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`

	// Error tracking
	LastError  *string `json:"last_error,omitempty"`
	ErrorCount int     `json:"error_count"`

	// Correlation and tracing
	CorrelationID string     `json:"correlation_id,omitempty"`
	TraceID       string     `json:"trace_id,omitempty"`
	SpanID        string     `json:"span_id,omitempty"`
	ParentEventID *uuid.UUID `json:"parent_event_id,omitempty"`
}

// NewEvent creates a new event
func NewEvent(eventType string, tenantID uuid.UUID, payload map[string]interface{}) *Event {
	return &Event{
		ID:            uuid.New(),
		Type:          eventType,
		TenantID:      tenantID,
		Priority:      5, // Default priority
		Version:       1,
		Payload:       payload,
		Metadata:      make(map[string]interface{}),
		RetryCount:    0,
		MaxRetries:    3,
		CreatedAt:     time.Now(),
		ErrorCount:    0,
		CorrelationID: uuid.New().String(),
	}
}

// Publish publishes an event to the bus
func (bus *InMemoryEventBus) Publish(event *Event) error {
	if !bus.running {
		return fmt.Errorf("event bus is not running")
	}

	// Add tracing information
	if bus.config.EnableTracing {
		event.TraceID = uuid.New().String()
		event.SpanID = uuid.New().String()
	}

	// Update metrics
	if bus.config.EnableMetrics {
		bus.mu.Lock()
		bus.metrics.EventsPublished++
		bus.mu.Unlock()
	}

	// Queue the event
	select {
	case bus.eventQueue <- event:
		return nil
	default:
		return fmt.Errorf("event queue is full")
	}
}

// PublishBatch publishes multiple events atomically
func (bus *InMemoryEventBus) PublishBatch(events []*Event) error {
	if !bus.running {
		return fmt.Errorf("event bus is not running")
	}

	// For in-memory implementation, publish events sequentially
	for _, event := range events {
		if err := bus.Publish(event); err != nil {
			return fmt.Errorf("failed to publish event %s: %w", event.ID, err)
		}
	}

	return nil
}

// Subscribe subscribes a handler to specific event types
func (bus *InMemoryEventBus) Subscribe(handler EventHandler, eventTypes ...string) error {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	// Register handler for each event type
	for _, eventType := range eventTypes {
		if bus.handlers[eventType] == nil {
			bus.handlers[eventType] = make([]EventHandler, 0)
		}
		bus.handlers[eventType] = append(bus.handlers[eventType], handler)
	}

	// Track subscription
	bus.subscribers[handler] = append(bus.subscribers[handler], eventTypes...)

	// Initialize handler metrics
	if bus.config.EnableMetrics {
		if bus.metrics.HandlerMetrics[handler.GetName()] == nil {
			bus.metrics.HandlerMetrics[handler.GetName()] = &HandlerMetrics{}
		}
	}

	return nil
}

// Unsubscribe removes a handler from event types
func (bus *InMemoryEventBus) Unsubscribe(handler EventHandler, eventTypes ...string) error {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	// Remove handler from each event type
	for _, eventType := range eventTypes {
		handlers := bus.handlers[eventType]
		for i, h := range handlers {
			if h == handler {
				// Remove handler from slice
				bus.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
				break
			}
		}
	}

	// Update subscription tracking
	if subscriptions, exists := bus.subscribers[handler]; exists {
		filtered := make([]string, 0, len(subscriptions))
		for _, sub := range subscriptions {
			found := false
			for _, unsubType := range eventTypes {
				if sub == unsubType {
					found = true
					break
				}
			}
			if !found {
				filtered = append(filtered, sub)
			}
		}
		bus.subscribers[handler] = filtered
	}

	return nil
}

// Start starts the event bus
func (bus *InMemoryEventBus) Start() error {
	if bus.running {
		return fmt.Errorf("event bus is already running")
	}

	bus.running = true

	// Start worker goroutines
	for i := 0; i < bus.config.WorkerCount; i++ {
		bus.wg.Add(1)
		go func(workerID int) {
			defer bus.wg.Done()
			bus.worker(workerID)
		}(i)
	}

	// Start metrics update routine if enabled
	if bus.config.EnableMetrics {
		bus.wg.Add(1)
		go func() {
			defer bus.wg.Done()
			bus.metricsRoutine()
		}()
	}

	return nil
}

// Stop stops the event bus gracefully
func (bus *InMemoryEventBus) Stop() error {
	if !bus.running {
		return nil
	}

	close(bus.stopChan)
	bus.running = false

	// Close event queue
	close(bus.eventQueue)

	// Wait for workers to finish
	bus.wg.Wait()

	return nil
}

// worker processes events from the queue
func (bus *InMemoryEventBus) worker(workerID int) {
	for {
		select {
		case <-bus.stopChan:
			return
		case event, ok := <-bus.eventQueue:
			if !ok {
				return // Channel closed
			}

			bus.processEvent(event, workerID)
		}
	}
}

// processEvent processes a single event
func (bus *InMemoryEventBus) processEvent(event *Event, workerID int) {
	ctx := context.Background()

	// Add tracing if enabled
	if bus.config.EnableTracing {
		var span trace.Span
		ctx, span = bus.tracer.Start(ctx, "process_event")
		defer span.End()

		span.SetAttributes(
			attribute.String("event.id", event.ID.String()),
			attribute.String("event.type", event.Type),
			attribute.String("tenant.id", event.TenantID.String()),
			attribute.Int("worker.id", workerID),
		)
	}

	// Add processing timeout
	ctx, cancel := context.WithTimeout(ctx, bus.config.ProcessingTimeout)
	defer cancel()

	// Find handlers for this event type
	bus.mu.RLock()
	handlers := bus.handlers[event.Type]
	bus.mu.RUnlock()

	if len(handlers) == 0 {
		// No handlers for this event type
		return
	}

	// Process event with each handler
	for _, handler := range handlers {
		if err := bus.processEventWithHandler(ctx, event, handler); err != nil {
			// Handle processing error
			bus.handleProcessingError(ctx, event, handler, err)
		}
	}

	// Update processed timestamp
	now := time.Now()
	event.ProcessedAt = &now

	// Update metrics
	if bus.config.EnableMetrics {
		bus.mu.Lock()
		bus.metrics.EventsProcessed++
		bus.metrics.LastProcessedAt = &now
		bus.mu.Unlock()
	}
}

// processEventWithHandler processes an event with a specific handler
func (bus *InMemoryEventBus) processEventWithHandler(ctx context.Context, event *Event, handler EventHandler) error {
	startTime := time.Now()

	// Convert event to expected format for handler
	var eventInterface interface{}
	switch event.Type {
	case EventFileDiscovered:
		eventInterface = bus.convertToFileDiscoveredEvent(event)
	case EventFileClassified:
		eventInterface = bus.convertToFileClassifiedEvent(event)
	case EventDLPViolation:
		eventInterface = bus.convertToDLPViolationEvent(event)
	default:
		// For workflow events, pass the event directly
		eventInterface = event
	}

	// Process event
	err := handler.HandleEvent(ctx, eventInterface)

	// Update handler metrics
	if bus.config.EnableMetrics {
		bus.updateHandlerMetrics(handler.GetName(), startTime, err)
	}

	return err
}

// convertToFileDiscoveredEvent converts generic event to FileDiscoveredEvent
func (bus *InMemoryEventBus) convertToFileDiscoveredEvent(event *Event) *FileDiscoveredEvent {
	data := FileDiscoveredData{}
	if payload, ok := event.Payload["data"].(map[string]interface{}); ok {
		if url, ok := payload["url"].(string); ok {
			data.URL = url
		}
		if sourceID, ok := payload["source_id"].(string); ok {
			data.SourceID = sourceID
		}
		if size, ok := payload["size"].(float64); ok {
			data.Size = int64(size)
		}
		if contentType, ok := payload["content_type"].(string); ok {
			data.ContentType = contentType
		}
		if priority, ok := payload["priority"].(string); ok {
			data.Priority = priority
		}
		data.DiscoveredAt = event.CreatedAt
	}

	return &FileDiscoveredEvent{
		BaseEvent: BaseEvent{
			ID:       event.ID.String(),
			Type:     event.Type,
			Source:   "event-bus",
			Time:     event.CreatedAt,
			TenantID: event.TenantID.String(),
			TraceID:  event.TraceID,
			Metadata: event.Metadata,
		},
		Data: data,
	}
}

// convertToFileClassifiedEvent converts generic event to FileClassifiedEvent
func (bus *InMemoryEventBus) convertToFileClassifiedEvent(event *Event) *FileClassifiedEvent {
	data := FileClassifiedData{}
	if payload, ok := event.Payload["data"].(map[string]interface{}); ok {
		if url, ok := payload["url"].(string); ok {
			data.URL = url
		}
		if dataClass, ok := payload["data_class"].(string); ok {
			data.DataClass = dataClass
		}
		if piiTypes, ok := payload["pii_types"].([]interface{}); ok {
			for _, pii := range piiTypes {
				if piiStr, ok := pii.(string); ok {
					data.PIITypes = append(data.PIITypes, piiStr)
				}
			}
		}
	}

	return &FileClassifiedEvent{
		BaseEvent: BaseEvent{
			ID:       event.ID.String(),
			Type:     event.Type,
			Source:   "event-bus",
			Time:     event.CreatedAt,
			TenantID: event.TenantID.String(),
			TraceID:  event.TraceID,
			Metadata: event.Metadata,
		},
		Data: data,
	}
}

// convertToDLPViolationEvent converts generic event to DLPViolationEvent
func (bus *InMemoryEventBus) convertToDLPViolationEvent(event *Event) *DLPViolationEvent {
	data := DLPViolationData{}
	if payload, ok := event.Payload["data"].(map[string]interface{}); ok {
		if url, ok := payload["url"].(string); ok {
			data.URL = url
		}
		if policyID, ok := payload["policy_id"].(string); ok {
			data.PolicyID = policyID
		}
		if violationType, ok := payload["violation_type"].(string); ok {
			data.ViolationType = violationType
		}
		if severity, ok := payload["severity"].(string); ok {
			data.Severity = severity
		}
	}

	return &DLPViolationEvent{
		BaseEvent: BaseEvent{
			ID:       event.ID.String(),
			Type:     event.Type,
			Source:   "event-bus",
			Time:     event.CreatedAt,
			TenantID: event.TenantID.String(),
			TraceID:  event.TraceID,
			Metadata: event.Metadata,
		},
		Data: data,
	}
}

// handleProcessingError handles errors during event processing
func (bus *InMemoryEventBus) handleProcessingError(ctx context.Context, event *Event, handler EventHandler, err error) {
	event.ErrorCount++
	event.LastError = &[]string{err.Error()}[0]

	// Update metrics
	if bus.config.EnableMetrics {
		bus.mu.Lock()
		bus.metrics.EventsFailed++
		if handlerMetrics := bus.metrics.HandlerMetrics[handler.GetName()]; handlerMetrics != nil {
			handlerMetrics.EventsFailed++
		}
		bus.mu.Unlock()
	}

	// Check if we should retry
	if event.RetryCount < event.MaxRetries {
		event.RetryCount++

		// Calculate retry delay with exponential backoff
		retryDelay := bus.config.RetryDelay * time.Duration(event.RetryCount)
		if retryDelay > bus.config.MaxRetryDelay {
			retryDelay = bus.config.MaxRetryDelay
		}

		// Schedule retry
		go func() {
			time.Sleep(retryDelay)
			if bus.running {
				bus.eventQueue <- event
			}
		}()
	} else if bus.config.DeadLetterEnabled {
		// Send to dead letter queue (in this case, just log)
		// In a real implementation, this would send to a dead letter topic/queue
		fmt.Printf("Event %s sent to dead letter queue after %d failures\n", event.ID, event.ErrorCount)
	}
}

// updateHandlerMetrics updates metrics for a specific handler
func (bus *InMemoryEventBus) updateHandlerMetrics(handlerName string, startTime time.Time, err error) {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	metrics := bus.metrics.HandlerMetrics[handlerName]
	if metrics == nil {
		metrics = &HandlerMetrics{}
		bus.metrics.HandlerMetrics[handlerName] = metrics
	}

	processingTime := time.Since(startTime)
	metrics.EventsHandled++
	metrics.AvgProcessingTime = (metrics.AvgProcessingTime*time.Duration(metrics.EventsHandled-1) + processingTime) / time.Duration(metrics.EventsHandled)
	metrics.LastHandledAt = &[]time.Time{time.Now()}[0]

	if err != nil {
		metrics.EventsFailed++
	}

	if metrics.EventsHandled > 0 {
		metrics.ErrorRate = float64(metrics.EventsFailed) / float64(metrics.EventsHandled)
	}
}

// metricsRoutine periodically updates bus-level metrics
func (bus *InMemoryEventBus) metricsRoutine() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-bus.stopChan:
			return
		case <-ticker.C:
			bus.updateBusMetrics()
		}
	}
}

// updateBusMetrics updates bus-level metrics
func (bus *InMemoryEventBus) updateBusMetrics() {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	// Update queue depth
	bus.metrics.QueueDepth = len(bus.eventQueue)

	// Update active handlers count
	bus.metrics.HandlersActive = len(bus.metrics.HandlerMetrics)

	// Calculate overall error rate
	if bus.metrics.EventsProcessed > 0 {
		bus.metrics.ErrorRate = float64(bus.metrics.EventsFailed) / float64(bus.metrics.EventsProcessed)
	}

	// Calculate throughput (events per second)
	if bus.metrics.LastProcessedAt != nil {
		duration := time.Since(*bus.metrics.LastProcessedAt)
		if duration > 0 {
			bus.metrics.ThroughputPerSec = float64(bus.metrics.EventsProcessed) / duration.Seconds()
		}
	}
}

// GetMetrics returns bus metrics
func (bus *InMemoryEventBus) GetMetrics() *EventBusMetrics {
	bus.mu.RLock()
	defer bus.mu.RUnlock()

	// Create a copy of metrics to avoid race conditions
	metricsCopy := &EventBusMetrics{
		EventsPublished:  bus.metrics.EventsPublished,
		EventsProcessed:  bus.metrics.EventsProcessed,
		EventsFailed:     bus.metrics.EventsFailed,
		HandlersActive:   bus.metrics.HandlersActive,
		QueueDepth:       bus.metrics.QueueDepth,
		ProcessingTime:   bus.metrics.ProcessingTime,
		LastProcessedAt:  bus.metrics.LastProcessedAt,
		ErrorRate:        bus.metrics.ErrorRate,
		ThroughputPerSec: bus.metrics.ThroughputPerSec,
		HandlerMetrics:   make(map[string]*HandlerMetrics),
	}

	// Copy handler metrics
	for name, metrics := range bus.metrics.HandlerMetrics {
		metricsCopy.HandlerMetrics[name] = &HandlerMetrics{
			EventsHandled:     metrics.EventsHandled,
			EventsFailed:      metrics.EventsFailed,
			AvgProcessingTime: metrics.AvgProcessingTime,
			LastHandledAt:     metrics.LastHandledAt,
			ErrorRate:         metrics.ErrorRate,
		}
	}

	return metricsCopy
}

// EventBusInterface defines the interface that both InMemoryEventBus and KafkaEventBus implement
type EventBusInterface interface {
	Publish(event *Event) error
	PublishBatch(events []*Event) error
	Subscribe(handler EventHandler, eventTypes ...string) error
	Unsubscribe(handler EventHandler, eventTypes ...string) error
	Start() error
	Stop() error
	GetMetrics() *EventBusMetrics
}
