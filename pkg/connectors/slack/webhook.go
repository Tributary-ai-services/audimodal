package slack

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// WebhookHandler handles Slack webhook events for real-time synchronization
type WebhookHandler struct {
	connector *SlackConnector
	tracer    trace.Tracer
	secret    string
	handlers  map[string]EventHandler
}

// EventHandler defines the interface for handling specific event types
type EventHandler interface {
	HandleEvent(ctx context.Context, event *SlackWebhookEvent) error
	GetEventType() string
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(connector *SlackConnector, secret string, tracer trace.Tracer) *WebhookHandler {
	handler := &WebhookHandler{
		connector: connector,
		tracer:    tracer,
		secret:    secret,
		handlers:  make(map[string]EventHandler),
	}
	
	// Register default event handlers
	handler.RegisterHandler(&MessageHandler{connector: connector})
	handler.RegisterHandler(&FileHandler{connector: connector})
	handler.RegisterHandler(&ChannelHandler{connector: connector})
	handler.RegisterHandler(&UserHandler{connector: connector})
	
	return handler
}

// RegisterHandler registers an event handler for a specific event type
func (wh *WebhookHandler) RegisterHandler(handler EventHandler) {
	wh.handlers[handler.GetEventType()] = handler
}

// HandleWebhook handles incoming Slack webhook requests
func (wh *WebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := wh.tracer.Start(ctx, "slack_webhook_handler")
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
	
	// Verify webhook signature
	if wh.secret != "" {
		if !wh.verifyWebhookSignature(r, body) {
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}
	}
	
	// Parse webhook event
	var event SlackEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	
	// Handle URL verification challenge
	if event.Type == "url_verification" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(event.Challenge))
		return
	}
	
	// Handle event callbacks
	if event.Type == "event_callback" {
		if err := wh.handleEventCallback(ctx, &event); err != nil {
			span.RecordError(err)
			http.Error(w, "Failed to process event", http.StatusInternalServerError)
			return
		}
	}
	
	// Acknowledge receipt
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// verifyWebhookSignature verifies the webhook signature
func (wh *WebhookHandler) verifyWebhookSignature(r *http.Request, body []byte) bool {
	timestamp := r.Header.Get("X-Slack-Request-Timestamp")
	signature := r.Header.Get("X-Slack-Signature")
	
	if timestamp == "" || signature == "" {
		return false
	}
	
	// Check timestamp to prevent replay attacks
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	
	if time.Since(time.Unix(ts, 0)) > 5*time.Minute {
		return false // Request is too old
	}
	
	// Generate expected signature
	baseString := fmt.Sprintf("v0:%s:%s", timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(wh.secret))
	mac.Write([]byte(baseString))
	expectedSignature := "v0=" + hex.EncodeToString(mac.Sum(nil))
	
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// handleEventCallback handles event callback webhooks
func (wh *WebhookHandler) handleEventCallback(ctx context.Context, event *SlackEvent) error {
	// Extract the actual event from the wrapper
	eventData, ok := event.Event["type"].(string)
	if !ok {
		return fmt.Errorf("missing event type")
	}
	
	// Create webhook event structure
	webhookEvent := &SlackWebhookEvent{
		Type: eventData,
	}
	
	// Parse event-specific data
	if err := wh.parseEventData(event.Event, webhookEvent); err != nil {
		return fmt.Errorf("failed to parse event data: %w", err)
	}
	
	// Find and execute appropriate handler
	handler, exists := wh.handlers[eventData]
	if !exists {
		// Log unknown event type but don't fail
		return nil
	}
	
	return handler.HandleEvent(ctx, webhookEvent)
}

// parseEventData parses event-specific data into the webhook event
func (wh *WebhookHandler) parseEventData(eventData map[string]interface{}, webhookEvent *SlackWebhookEvent) error {
	// Convert map to JSON and back to populate the struct
	jsonData, err := json.Marshal(eventData)
	if err != nil {
		return err
	}
	
	return json.Unmarshal(jsonData, webhookEvent)
}

// MessageHandler handles message-related events
type MessageHandler struct {
	connector *SlackConnector
}

func (h *MessageHandler) GetEventType() string {
	return "message"
}

func (h *MessageHandler) HandleEvent(ctx context.Context, event *SlackWebhookEvent) error {
	// Handle different message subtypes
	switch event.Subtype {
	case "message_deleted":
		return h.handleMessageDeleted(ctx, event)
	case "message_changed":
		return h.handleMessageChanged(ctx, event)
	case "file_share":
		return h.handleFileShare(ctx, event)
	default:
		return h.handleNewMessage(ctx, event)
	}
}

func (h *MessageHandler) handleNewMessage(ctx context.Context, event *SlackWebhookEvent) error {
	// Clear message cache for the channel to force refresh
	h.connector.cache.mu.Lock()
	delete(h.connector.cache.messages, event.Channel)
	h.connector.cache.mu.Unlock()
	
	return nil
}

func (h *MessageHandler) handleMessageDeleted(ctx context.Context, event *SlackWebhookEvent) error {
	// Clear message cache for the channel
	h.connector.cache.mu.Lock()
	delete(h.connector.cache.messages, event.Channel)
	h.connector.cache.mu.Unlock()
	
	return nil
}

func (h *MessageHandler) handleMessageChanged(ctx context.Context, event *SlackWebhookEvent) error {
	// Clear message cache for the channel
	h.connector.cache.mu.Lock()
	delete(h.connector.cache.messages, event.Channel)
	h.connector.cache.mu.Unlock()
	
	return nil
}

func (h *MessageHandler) handleFileShare(ctx context.Context, event *SlackWebhookEvent) error {
	// Handle file sharing events
	if event.File != nil {
		// Process the shared file
		// Could trigger file download or indexing
	}
	
	return nil
}

// FileHandler handles file-related events
type FileHandler struct {
	connector *SlackConnector
}

func (h *FileHandler) GetEventType() string {
	return "file_shared"
}

func (h *FileHandler) HandleEvent(ctx context.Context, event *SlackWebhookEvent) error {
	// Handle file shared events
	if event.File != nil {
		// Check if file should be synced based on configuration
		if h.connector.config.SyncFiles {
			// Could trigger immediate file sync
		}
	}
	
	return nil
}

// ChannelHandler handles channel-related events
type ChannelHandler struct {
	connector *SlackConnector
}

func (h *ChannelHandler) GetEventType() string {
	return "channel_created"
}

func (h *ChannelHandler) HandleEvent(ctx context.Context, event *SlackWebhookEvent) error {
	// Clear channel cache to force refresh
	h.connector.cache.mu.Lock()
	h.connector.cache.channels = make(map[string]*SlackChannel)
	h.connector.cache.mu.Unlock()
	
	return nil
}

// UserHandler handles user-related events
type UserHandler struct {
	connector *SlackConnector
}

func (h *UserHandler) GetEventType() string {
	return "user_change"
}

func (h *UserHandler) HandleEvent(ctx context.Context, event *SlackWebhookEvent) error {
	// Clear user cache to force refresh
	h.connector.cache.mu.Lock()
	h.connector.cache.users = make(map[string]*SlackUser)
	h.connector.cache.mu.Unlock()
	
	return nil
}

// SocketModeHandler handles Slack Socket Mode connections for real-time events
type SocketModeHandler struct {
	connector *SlackConnector
	tracer    trace.Tracer
	appToken  string
	socketURL string
	handlers  map[string]EventHandler
}

// NewSocketModeHandler creates a new Socket Mode handler
func NewSocketModeHandler(connector *SlackConnector, appToken string, tracer trace.Tracer) *SocketModeHandler {
	handler := &SocketModeHandler{
		connector: connector,
		tracer:    tracer,
		appToken:  appToken,
		handlers:  make(map[string]EventHandler),
	}
	
	// Register default event handlers (same as webhook)
	handler.RegisterHandler(&MessageHandler{connector: connector})
	handler.RegisterHandler(&FileHandler{connector: connector})
	handler.RegisterHandler(&ChannelHandler{connector: connector})
	handler.RegisterHandler(&UserHandler{connector: connector})
	
	return handler
}

// RegisterHandler registers an event handler
func (sm *SocketModeHandler) RegisterHandler(handler EventHandler) {
	sm.handlers[handler.GetEventType()] = handler
}

// Connect establishes Socket Mode connection
func (sm *SocketModeHandler) Connect(ctx context.Context) error {
	// Get Socket Mode URL
	params := map[string]string{
		"token": sm.appToken,
	}
	
	var requestBody bytes.Buffer
	json.NewEncoder(&requestBody).Encode(params)
	
	req, err := http.NewRequestWithContext(ctx, "POST", "https://slack.com/api/apps.connections.open", &requestBody)
	if err != nil {
		return err
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", sm.appToken))
	
	resp, err := sm.connector.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	var connectionResp struct {
		OK  bool   `json:"ok"`
		URL string `json:"url"`
		Error string `json:"error,omitempty"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&connectionResp); err != nil {
		return err
	}
	
	if !connectionResp.OK {
		return fmt.Errorf("failed to get Socket Mode URL: %s", connectionResp.Error)
	}
	
	sm.socketURL = connectionResp.URL
	
	// In a real implementation, you would establish a WebSocket connection
	// to the returned URL and handle real-time events
	
	return nil
}

// StartListening starts listening for Socket Mode events
func (sm *SocketModeHandler) StartListening(ctx context.Context) error {
	if sm.socketURL == "" {
		return fmt.Errorf("Socket Mode connection not established")
	}
	
	// In a real implementation, this would:
	// 1. Establish WebSocket connection to sm.socketURL
	// 2. Handle incoming events in real-time
	// 3. Send acknowledgments back to Slack
	// 4. Route events to appropriate handlers
	
	return nil
}

// SlackEventRouter routes different types of Slack events
type SlackEventRouter struct {
	handlers map[string][]EventHandler
}

// NewSlackEventRouter creates a new event router
func NewSlackEventRouter() *SlackEventRouter {
	return &SlackEventRouter{
		handlers: make(map[string][]EventHandler),
	}
}

// RegisterHandler registers a handler for a specific event type
func (r *SlackEventRouter) RegisterHandler(eventType string, handler EventHandler) {
	r.handlers[eventType] = append(r.handlers[eventType], handler)
}

// RouteEvent routes an event to all registered handlers
func (r *SlackEventRouter) RouteEvent(ctx context.Context, event *SlackWebhookEvent) error {
	handlers, exists := r.handlers[event.Type]
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

// BatchEventProcessor processes events in batches for efficiency
type BatchEventProcessor struct {
	events   chan *SlackWebhookEvent
	router   *SlackEventRouter
	batchSize int
	flushInterval time.Duration
}

// NewBatchEventProcessor creates a new batch event processor
func NewBatchEventProcessor(router *SlackEventRouter, batchSize int, flushInterval time.Duration) *BatchEventProcessor {
	return &BatchEventProcessor{
		events:        make(chan *SlackWebhookEvent, batchSize*2),
		router:        router,
		batchSize:     batchSize,
		flushInterval: flushInterval,
	}
}

// Start starts the batch processor
func (bp *BatchEventProcessor) Start(ctx context.Context) {
	ticker := time.NewTicker(bp.flushInterval)
	defer ticker.Stop()
	
	var batch []*SlackWebhookEvent
	
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
func (bp *BatchEventProcessor) QueueEvent(event *SlackWebhookEvent) {
	select {
	case bp.events <- event:
		// Event queued successfully
	default:
		// Channel is full, could log this or implement backpressure
	}
}

// processBatch processes a batch of events
func (bp *BatchEventProcessor) processBatch(ctx context.Context, batch []*SlackWebhookEvent) {
	for _, event := range batch {
		// Process each event in the batch
		bp.router.RouteEvent(ctx, event)
	}
}