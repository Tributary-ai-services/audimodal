package notion

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// WebhookHandler handles Notion webhook events for real-time synchronization
// Note: Notion doesn't currently support webhooks, but this provides a framework
// for when they do, or for polling-based change detection
type WebhookHandler struct {
	connector *NotionConnector
	tracer    trace.Tracer
	secret    string
	handlers  map[string]EventHandler
}

// EventHandler defines the interface for handling specific event types
type EventHandler interface {
	HandleEvent(ctx context.Context, event *NotionWebhookEvent) error
	GetEventType() string
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(connector *NotionConnector, secret string, tracer trace.Tracer) *WebhookHandler {
	handler := &WebhookHandler{
		connector: connector,
		tracer:    tracer,
		secret:    secret,
		handlers:  make(map[string]EventHandler),
	}

	// Register default event handlers
	handler.RegisterHandler(&PageHandler{connector: connector})
	handler.RegisterHandler(&DatabaseHandler{connector: connector})
	handler.RegisterHandler(&BlockHandler{connector: connector})
	handler.RegisterHandler(&UserHandler{connector: connector})

	return handler
}

// RegisterHandler registers an event handler for a specific event type
func (wh *WebhookHandler) RegisterHandler(handler EventHandler) {
	wh.handlers[handler.GetEventType()] = handler
}

// HandleWebhook handles incoming Notion webhook requests
func (wh *WebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := wh.tracer.Start(ctx, "notion_webhook_handler")
	defer span.End()

	// Verify request method
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Validate webhook (when Notion adds webhook support)
	validator := NewNotionWebhookValidator(wh.secret)
	if err := validator.ValidateWebhook(r, body); err != nil {
		http.Error(w, "Invalid webhook", http.StatusUnauthorized)
		return
	}

	// Parse webhook event
	var event NotionWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Handle the event
	if err := wh.handleEvent(ctx, &event); err != nil {
		span.RecordError(err)
		http.Error(w, "Failed to process event", http.StatusInternalServerError)
		return
	}

	// Acknowledge receipt
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleEvent handles a webhook event
func (wh *WebhookHandler) handleEvent(ctx context.Context, event *NotionWebhookEvent) error {
	// Determine event type based on object type and properties
	eventType := wh.determineEventType(event)

	// Find and execute appropriate handler
	handler, exists := wh.handlers[eventType]
	if !exists {
		// Log unknown event type but don't fail
		return nil
	}

	return handler.HandleEvent(ctx, event)
}

// determineEventType determines the event type from the webhook event
func (wh *WebhookHandler) determineEventType(event *NotionWebhookEvent) string {
	// Since Notion doesn't have webhooks yet, this is speculative
	// based on how other platforms structure their webhooks

	switch event.Object {
	case "page":
		return "page_updated"
	case "database":
		return "database_updated"
	case "block":
		return "block_updated"
	case "user":
		return "user_updated"
	default:
		return "unknown"
	}
}

// PageHandler handles page-related events
type PageHandler struct {
	connector *NotionConnector
}

func (h *PageHandler) GetEventType() string {
	return "page_updated"
}

func (h *PageHandler) HandleEvent(ctx context.Context, event *NotionWebhookEvent) error {
	// Clear page cache to force refresh
	h.connector.cache.mu.Lock()
	delete(h.connector.cache.pages, event.ID)
	h.connector.cache.mu.Unlock()

	// If the page has blocks, also clear the blocks cache
	h.connector.cache.mu.Lock()
	delete(h.connector.cache.blocks, event.ID)
	h.connector.cache.mu.Unlock()

	return nil
}

// DatabaseHandler handles database-related events
type DatabaseHandler struct {
	connector *NotionConnector
}

func (h *DatabaseHandler) GetEventType() string {
	return "database_updated"
}

func (h *DatabaseHandler) HandleEvent(ctx context.Context, event *NotionWebhookEvent) error {
	// Clear database cache to force refresh
	h.connector.cache.mu.Lock()
	delete(h.connector.cache.databases, event.ID)
	h.connector.cache.mu.Unlock()

	return nil
}

// BlockHandler handles block-related events
type BlockHandler struct {
	connector *NotionConnector
}

func (h *BlockHandler) GetEventType() string {
	return "block_updated"
}

func (h *BlockHandler) HandleEvent(ctx context.Context, event *NotionWebhookEvent) error {
	// Clear blocks cache for the parent page
	if event.Parent.Type == "page_id" {
		h.connector.cache.mu.Lock()
		delete(h.connector.cache.blocks, event.Parent.PageID)
		h.connector.cache.mu.Unlock()
	}

	return nil
}

// UserHandler handles user-related events
type UserHandler struct {
	connector *NotionConnector
}

func (h *UserHandler) GetEventType() string {
	return "user_updated"
}

func (h *UserHandler) HandleEvent(ctx context.Context, event *NotionWebhookEvent) error {
	// Clear user cache to force refresh
	h.connector.cache.mu.Lock()
	delete(h.connector.cache.users, event.ID)
	h.connector.cache.mu.Unlock()

	return nil
}

// PollingHandler provides polling-based change detection since Notion doesn't have webhooks
type PollingHandler struct {
	connector     *NotionConnector
	tracer        trace.Tracer
	pollInterval  time.Duration
	lastPollTime  time.Time
	changeHandler func(ctx context.Context, changes []*NotionChange) error
}

// NotionChange represents a detected change in Notion
type NotionChange struct {
	Type           string      `json:"type"`   // page, database, block, user
	Action         string      `json:"action"` // created, updated, deleted
	ObjectID       string      `json:"object_id"`
	LastEditedTime time.Time   `json:"last_edited_time"`
	Object         interface{} `json:"object"`
}

// NewPollingHandler creates a new polling handler
func NewPollingHandler(connector *NotionConnector, tracer trace.Tracer, pollInterval time.Duration) *PollingHandler {
	return &PollingHandler{
		connector:    connector,
		tracer:       tracer,
		pollInterval: pollInterval,
		lastPollTime: time.Now(),
	}
}

// SetChangeHandler sets the function to call when changes are detected
func (ph *PollingHandler) SetChangeHandler(handler func(ctx context.Context, changes []*NotionChange) error) {
	ph.changeHandler = handler
}

// StartPolling starts the polling loop
func (ph *PollingHandler) StartPolling(ctx context.Context) error {
	ticker := time.NewTicker(ph.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := ph.pollForChanges(ctx); err != nil {
				// Log error but continue polling
				continue
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// pollForChanges polls for changes since the last poll
func (ph *PollingHandler) pollForChanges(ctx context.Context) error {
	_, span := ph.tracer.Start(ctx, "notion_poll_changes")
	defer span.End()

	var changes []*NotionChange

	// Poll for page changes
	pageChanges, err := ph.pollPageChanges(ctx)
	if err != nil {
		span.RecordError(err)
		return err
	}
	changes = append(changes, pageChanges...)

	// Poll for database changes
	dbChanges, err := ph.pollDatabaseChanges(ctx)
	if err != nil {
		span.RecordError(err)
		return err
	}
	changes = append(changes, dbChanges...)

	// Poll for user changes (less frequent)
	userChanges, err := ph.pollUserChanges(ctx)
	if err != nil {
		span.RecordError(err)
		return err
	}
	changes = append(changes, userChanges...)

	// Update last poll time
	ph.lastPollTime = time.Now()

	// Process changes if any were found
	if len(changes) > 0 && ph.changeHandler != nil {
		return ph.changeHandler(ctx, changes)
	}

	return nil
}

// pollPageChanges polls for page changes
func (ph *PollingHandler) pollPageChanges(ctx context.Context) ([]*NotionChange, error) {
	var changes []*NotionChange

	// Get current pages
	pages, err := ph.connector.listPages(ctx)
	if err != nil {
		return nil, err
	}

	// Check for changes since last poll
	for _, page := range pages {
		lastEdited := parseNotionTimestamp(page.LastEditedTime)
		if lastEdited.After(ph.lastPollTime) {
			changes = append(changes, &NotionChange{
				Type:           "page",
				Action:         "updated",
				ObjectID:       page.ID,
				LastEditedTime: lastEdited,
				Object:         page,
			})
		}
	}

	return changes, nil
}

// pollDatabaseChanges polls for database changes
func (ph *PollingHandler) pollDatabaseChanges(ctx context.Context) ([]*NotionChange, error) {
	var changes []*NotionChange

	// Get current databases
	databases, err := ph.connector.listDatabases(ctx)
	if err != nil {
		return nil, err
	}

	// Check for changes since last poll
	for _, database := range databases {
		lastEdited := parseNotionTimestamp(database.LastEditedTime)
		if lastEdited.After(ph.lastPollTime) {
			changes = append(changes, &NotionChange{
				Type:           "database",
				Action:         "updated",
				ObjectID:       database.ID,
				LastEditedTime: lastEdited,
				Object:         database,
			})
		}
	}

	return changes, nil
}

// pollUserChanges polls for user changes (infrequent)
func (ph *PollingHandler) pollUserChanges(ctx context.Context) ([]*NotionChange, error) {
	var changes []*NotionChange

	// User changes are rare, so we can poll less frequently
	// For now, we'll skip detailed user change detection

	return changes, nil
}

// EventRouter routes different types of Notion events
type EventRouter struct {
	handlers map[string][]EventHandler
}

// NewEventRouter creates a new event router
func NewEventRouter() *EventRouter {
	return &EventRouter{
		handlers: make(map[string][]EventHandler),
	}
}

// RegisterHandler registers a handler for a specific event type
func (r *EventRouter) RegisterHandler(eventType string, handler EventHandler) {
	r.handlers[eventType] = append(r.handlers[eventType], handler)
}

// RouteEvent routes an event to all registered handlers
func (r *EventRouter) RouteEvent(ctx context.Context, event *NotionWebhookEvent) error {
	eventType := r.determineEventType(event)

	handlers, exists := r.handlers[eventType]
	if !exists {
		return nil // No handlers for this event type
	}

	var lastError error
	for _, handler := range handlers {
		if err := handler.HandleEvent(ctx, event); err != nil {
			lastError = err
			// Continue processing other handlers even if one fails
		}
	}

	return lastError
}

// determineEventType determines the event type
func (r *EventRouter) determineEventType(event *NotionWebhookEvent) string {
	switch event.Object {
	case "page":
		if event.Archived {
			return "page_archived"
		}
		return "page_updated"
	case "database":
		if event.Archived {
			return "database_archived"
		}
		return "database_updated"
	default:
		return "unknown"
	}
}

// BatchEventProcessor processes events in batches for efficiency
type BatchEventProcessor struct {
	events        chan *NotionWebhookEvent
	router        *EventRouter
	batchSize     int
	flushInterval time.Duration
}

// NewBatchEventProcessor creates a new batch event processor
func NewBatchEventProcessor(router *EventRouter, batchSize int, flushInterval time.Duration) *BatchEventProcessor {
	return &BatchEventProcessor{
		events:        make(chan *NotionWebhookEvent, batchSize*2),
		router:        router,
		batchSize:     batchSize,
		flushInterval: flushInterval,
	}
}

// Start starts the batch processor
func (bp *BatchEventProcessor) Start(ctx context.Context) {
	ticker := time.NewTicker(bp.flushInterval)
	defer ticker.Stop()

	var batch []*NotionWebhookEvent

	for {
		select {
		case event := <-bp.events:
			batch = append(batch, event)
			if len(batch) >= bp.batchSize {
				bp.processBatch(ctx, batch)
				batch = nil
			}

		case <-ticker.C:
			if len(batch) > 0 {
				bp.processBatch(ctx, batch)
				batch = nil
			}

		case <-ctx.Done():
			// Process remaining events before shutting down
			if len(batch) > 0 {
				bp.processBatch(ctx, batch)
			}
			return
		}
	}
}

// QueueEvent queues an event for batch processing
func (bp *BatchEventProcessor) QueueEvent(event *NotionWebhookEvent) {
	select {
	case bp.events <- event:
		// Event queued successfully
	default:
		// Channel is full, could log this or implement backpressure
	}
}

// processBatch processes a batch of events
func (bp *BatchEventProcessor) processBatch(ctx context.Context, batch []*NotionWebhookEvent) {
	for _, event := range batch {
		// Process each event in the batch
		bp.router.RouteEvent(ctx, event)
	}
}

// ChangeDetector detects changes by comparing current state with cached state
type ChangeDetector struct {
	connector    *NotionConnector
	tracer       trace.Tracer
	lastSnapshot map[string]time.Time // object_id -> last_edited_time
}

// NewChangeDetector creates a new change detector
func NewChangeDetector(connector *NotionConnector, tracer trace.Tracer) *ChangeDetector {
	return &ChangeDetector{
		connector:    connector,
		tracer:       tracer,
		lastSnapshot: make(map[string]time.Time),
	}
}

// DetectChanges detects changes since the last snapshot
func (cd *ChangeDetector) DetectChanges(ctx context.Context) ([]*NotionChange, error) {
	_, span := cd.tracer.Start(ctx, "notion_detect_changes")
	defer span.End()

	var changes []*NotionChange
	currentSnapshot := make(map[string]time.Time)

	// Check pages for changes
	pages, err := cd.connector.listPages(ctx)
	if err != nil {
		return nil, err
	}

	for _, page := range pages {
		lastEdited := parseNotionTimestamp(page.LastEditedTime)
		currentSnapshot[page.ID] = lastEdited

		if lastSnapshotTime, exists := cd.lastSnapshot[page.ID]; exists {
			if lastEdited.After(lastSnapshotTime) {
				changes = append(changes, &NotionChange{
					Type:           "page",
					Action:         "updated",
					ObjectID:       page.ID,
					LastEditedTime: lastEdited,
					Object:         page,
				})
			}
		} else {
			// New page
			changes = append(changes, &NotionChange{
				Type:           "page",
				Action:         "created",
				ObjectID:       page.ID,
				LastEditedTime: lastEdited,
				Object:         page,
			})
		}
	}

	// Check databases for changes
	databases, err := cd.connector.listDatabases(ctx)
	if err != nil {
		return nil, err
	}

	for _, database := range databases {
		lastEdited := parseNotionTimestamp(database.LastEditedTime)
		currentSnapshot[database.ID] = lastEdited

		if lastSnapshotTime, exists := cd.lastSnapshot[database.ID]; exists {
			if lastEdited.After(lastSnapshotTime) {
				changes = append(changes, &NotionChange{
					Type:           "database",
					Action:         "updated",
					ObjectID:       database.ID,
					LastEditedTime: lastEdited,
					Object:         database,
				})
			}
		} else {
			// New database
			changes = append(changes, &NotionChange{
				Type:           "database",
				Action:         "created",
				ObjectID:       database.ID,
				LastEditedTime: lastEdited,
				Object:         database,
			})
		}
	}

	// Check for deleted items (items in last snapshot but not in current)
	for objectID := range cd.lastSnapshot {
		if _, exists := currentSnapshot[objectID]; !exists {
			changes = append(changes, &NotionChange{
				Type:           "unknown", // We don't know the type of deleted objects
				Action:         "deleted",
				ObjectID:       objectID,
				LastEditedTime: time.Now(),
				Object:         nil,
			})
		}
	}

	// Update snapshot
	cd.lastSnapshot = currentSnapshot

	return changes, nil
}
