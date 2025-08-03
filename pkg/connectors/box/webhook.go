package box

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// WebhookHandler handles Box webhook notifications for real-time sync
type WebhookHandler struct {
	connector   *BoxConnector
	syncManager *SyncManager
	config      *BoxWebhookConfig
	tracer      trace.Tracer

	// Event processing
	eventProcessor *EventProcessor
	eventQueue     chan *WebhookEvent
	stopCh         chan struct{}
}

// WebhookEvent represents a webhook event from Box
type WebhookEvent struct {
	WebhookID  string            `json:"webhook_id"`
	Timestamp  time.Time         `json:"timestamp"`
	Event      *BoxEvent         `json:"event"`
	Source     interface{}       `json:"source"`
	CreatedBy  *User             `json:"created_by"`
	Trigger    string            `json:"trigger"`
	RawPayload []byte            `json:"-"`
	Headers    map[string]string `json:"-"`
}

// WebhookResponse represents the response to a webhook
type WebhookResponse struct {
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// EventProcessor processes webhook events
type EventProcessor struct {
	connector   *BoxConnector
	syncManager *SyncManager
	tracer      trace.Tracer
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(connector *BoxConnector, syncManager *SyncManager, config *BoxWebhookConfig) *WebhookHandler {
	handler := &WebhookHandler{
		connector:   connector,
		syncManager: syncManager,
		config:      config,
		tracer:      otel.Tracer("box-webhook"),
		eventQueue:  make(chan *WebhookEvent, 1000),
		stopCh:      make(chan struct{}),
	}

	handler.eventProcessor = &EventProcessor{
		connector:   connector,
		syncManager: syncManager,
		tracer:      otel.Tracer("box-event-processor"),
	}

	return handler
}

// Start starts the webhook handler
func (wh *WebhookHandler) Start(ctx context.Context) error {
	// Start event processor
	go wh.processEvents(ctx)

	return nil
}

// Stop stops the webhook handler
func (wh *WebhookHandler) Stop() error {
	close(wh.stopCh)
	return nil
}

// HandleWebhook handles incoming webhook requests
func (wh *WebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	_, span := wh.tracer.Start(r.Context(), "handle_webhook")
	defer span.End()

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		span.RecordError(err)
		wh.writeErrorResponse(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	defer r.Body.Close()

	span.SetAttributes(
		attribute.String("webhook.id", wh.config.WebhookID),
		attribute.Int("payload.size", len(body)),
	)

	// Verify webhook signature
	if err := wh.verifySignature(r.Header, body); err != nil {
		span.RecordError(err)
		wh.writeErrorResponse(w, http.StatusUnauthorized, "Invalid signature")
		return
	}

	// Parse webhook payload
	var webhookEvent WebhookEvent
	if err := json.Unmarshal(body, &webhookEvent); err != nil {
		span.RecordError(err)
		wh.writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// Add metadata
	webhookEvent.WebhookID = wh.config.WebhookID
	webhookEvent.Timestamp = time.Now()
	webhookEvent.RawPayload = body
	webhookEvent.Headers = make(map[string]string)
	for key, values := range r.Header {
		if len(values) > 0 {
			webhookEvent.Headers[key] = values[0]
		}
	}

	// Queue event for processing
	select {
	case wh.eventQueue <- &webhookEvent:
		span.SetAttributes(attribute.Bool("event.queued", true))
	default:
		span.SetAttributes(attribute.Bool("event.queued", false))
		wh.writeErrorResponse(w, http.StatusServiceUnavailable, "Event queue full")
		return
	}

	// Send success response
	response := &WebhookResponse{
		Status:    "success",
		Message:   "Event received and queued for processing",
		Timestamp: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	span.SetAttributes(
		attribute.String("response.status", "success"),
		attribute.String("event.type", webhookEvent.Event.EventType),
	)
}

// processEvents processes events from the queue
func (wh *WebhookHandler) processEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-wh.stopCh:
			return
		case event := <-wh.eventQueue:
			if event != nil {
				wh.processEvent(ctx, event)
			}
		}
	}
}

// processEvent processes a single webhook event
func (wh *WebhookHandler) processEvent(ctx context.Context, event *WebhookEvent) {
	ctx, span := wh.tracer.Start(ctx, "process_webhook_event")
	defer span.End()

	span.SetAttributes(
		attribute.String("event.type", event.Event.EventType),
		attribute.String("event.id", event.Event.EventID),
		attribute.String("webhook.id", event.WebhookID),
	)

	// Process the event based on type
	err := wh.eventProcessor.ProcessEvent(ctx, event)
	if err != nil {
		span.RecordError(err)
		// Log error but don't fail - we've already acknowledged the webhook
	}

	span.SetAttributes(
		attribute.Bool("processing.success", err == nil),
	)
}

// verifySignature verifies the webhook signature
func (wh *WebhookHandler) verifySignature(headers http.Header, body []byte) error {
	if wh.config.WebhookKey == "" {
		return nil // Skip verification if no key is configured
	}

	// Get signature from headers
	signature := headers.Get("BOX-SIGNATURE-PRIMARY")
	if signature == "" {
		signature = headers.Get("BOX-SIGNATURE-SECONDARY")
	}
	if signature == "" {
		return fmt.Errorf("no signature found in headers")
	}

	// Get timestamp from headers
	timestamp := headers.Get("BOX-DELIVERY-TIMESTAMP")
	if timestamp == "" {
		return fmt.Errorf("no timestamp found in headers")
	}

	// Create message to verify
	message := string(body) + timestamp

	// Calculate expected signature
	mac := hmac.New(sha256.New, []byte(wh.config.WebhookKey))
	mac.Write([]byte(message))
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	// Compare signatures
	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

// writeErrorResponse writes an error response
func (wh *WebhookHandler) writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	response := &WebhookResponse{
		Status:    "error",
		Message:   message,
		Timestamp: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// ProcessEvent processes a webhook event
func (ep *EventProcessor) ProcessEvent(ctx context.Context, event *WebhookEvent) error {
	ctx, span := ep.tracer.Start(ctx, "process_event")
	defer span.End()

	span.SetAttributes(
		attribute.String("event.type", event.Event.EventType),
		attribute.String("event.id", event.Event.EventID),
	)

	// Process based on event type
	switch event.Event.EventType {
	case "ITEM_CREATE":
		return ep.handleItemCreate(ctx, event)
	case "ITEM_UPLOAD":
		return ep.handleItemUpload(ctx, event)
	case "ITEM_MODIFY":
		return ep.handleItemModify(ctx, event)
	case "ITEM_MOVE":
		return ep.handleItemMove(ctx, event)
	case "ITEM_COPY":
		return ep.handleItemCopy(ctx, event)
	case "ITEM_TRASH":
		return ep.handleItemTrash(ctx, event)
	case "ITEM_UNDELETE_VIA_TRASH":
		return ep.handleItemRestore(ctx, event)
	case "ITEM_RENAME":
		return ep.handleItemRename(ctx, event)
	case "COMMENT_CREATE":
		return ep.handleCommentCreate(ctx, event)
	case "COMMENT_DELETE":
		return ep.handleCommentDelete(ctx, event)
	case "TASK_ASSIGNMENT_CREATE":
		return ep.handleTaskCreate(ctx, event)
	case "TASK_ASSIGNMENT_DELETE":
		return ep.handleTaskDelete(ctx, event)
	case "COLLABORATION_INVITE":
		return ep.handleCollaborationInvite(ctx, event)
	case "COLLABORATION_ACCEPT":
		return ep.handleCollaborationAccept(ctx, event)
	case "COLLABORATION_ROLE_CHANGE":
		return ep.handleCollaborationRoleChange(ctx, event)
	case "COLLABORATION_REMOVE":
		return ep.handleCollaborationRemove(ctx, event)
	case "SHARED_LINK_CREATE":
		return ep.handleSharedLinkCreate(ctx, event)
	case "SHARED_LINK_DELETE":
		return ep.handleSharedLinkDelete(ctx, event)
	default:
		span.SetAttributes(attribute.Bool("event.handled", false))
		// Unknown event type, log but don't fail
		return nil
	}
}

// Event handlers

func (ep *EventProcessor) handleItemCreate(ctx context.Context, event *WebhookEvent) error {
	// Extract file information from event source
	sourceData, err := ep.extractSourceData(event)
	if err != nil {
		return fmt.Errorf("failed to extract source data: %w", err)
	}

	// Trigger incremental sync for the new item
	return ep.triggerIncrementalSync(ctx, sourceData.ID, "create")
}

func (ep *EventProcessor) handleItemUpload(ctx context.Context, event *WebhookEvent) error {
	sourceData, err := ep.extractSourceData(event)
	if err != nil {
		return fmt.Errorf("failed to extract source data: %w", err)
	}

	return ep.triggerIncrementalSync(ctx, sourceData.ID, "upload")
}

func (ep *EventProcessor) handleItemModify(ctx context.Context, event *WebhookEvent) error {
	sourceData, err := ep.extractSourceData(event)
	if err != nil {
		return fmt.Errorf("failed to extract source data: %w", err)
	}

	return ep.triggerIncrementalSync(ctx, sourceData.ID, "modify")
}

func (ep *EventProcessor) handleItemMove(ctx context.Context, event *WebhookEvent) error {
	sourceData, err := ep.extractSourceData(event)
	if err != nil {
		return fmt.Errorf("failed to extract source data: %w", err)
	}

	return ep.triggerIncrementalSync(ctx, sourceData.ID, "move")
}

func (ep *EventProcessor) handleItemCopy(ctx context.Context, event *WebhookEvent) error {
	sourceData, err := ep.extractSourceData(event)
	if err != nil {
		return fmt.Errorf("failed to extract source data: %w", err)
	}

	return ep.triggerIncrementalSync(ctx, sourceData.ID, "copy")
}

func (ep *EventProcessor) handleItemTrash(ctx context.Context, event *WebhookEvent) error {
	sourceData, err := ep.extractSourceData(event)
	if err != nil {
		return fmt.Errorf("failed to extract source data: %w", err)
	}

	return ep.triggerIncrementalSync(ctx, sourceData.ID, "trash")
}

func (ep *EventProcessor) handleItemRestore(ctx context.Context, event *WebhookEvent) error {
	sourceData, err := ep.extractSourceData(event)
	if err != nil {
		return fmt.Errorf("failed to extract source data: %w", err)
	}

	return ep.triggerIncrementalSync(ctx, sourceData.ID, "restore")
}

func (ep *EventProcessor) handleItemRename(ctx context.Context, event *WebhookEvent) error {
	sourceData, err := ep.extractSourceData(event)
	if err != nil {
		return fmt.Errorf("failed to extract source data: %w", err)
	}

	return ep.triggerIncrementalSync(ctx, sourceData.ID, "rename")
}

func (ep *EventProcessor) handleCommentCreate(ctx context.Context, event *WebhookEvent) error {
	// Handle comment creation if sync comments is enabled
	if !ep.syncManager.config.SyncComments {
		return nil
	}

	sourceData, err := ep.extractSourceData(event)
	if err != nil {
		return fmt.Errorf("failed to extract source data: %w", err)
	}

	return ep.triggerIncrementalSync(ctx, sourceData.ID, "comment_create")
}

func (ep *EventProcessor) handleCommentDelete(ctx context.Context, event *WebhookEvent) error {
	// Handle comment deletion if sync comments is enabled
	if !ep.syncManager.config.SyncComments {
		return nil
	}

	sourceData, err := ep.extractSourceData(event)
	if err != nil {
		return fmt.Errorf("failed to extract source data: %w", err)
	}

	return ep.triggerIncrementalSync(ctx, sourceData.ID, "comment_delete")
}

func (ep *EventProcessor) handleTaskCreate(ctx context.Context, event *WebhookEvent) error {
	// Handle task creation - could be used for workflow integration
	return nil
}

func (ep *EventProcessor) handleTaskDelete(ctx context.Context, event *WebhookEvent) error {
	// Handle task deletion
	return nil
}

func (ep *EventProcessor) handleCollaborationInvite(ctx context.Context, event *WebhookEvent) error {
	// Handle collaboration invitation if sync collaborations is enabled
	if !ep.syncManager.config.SyncCollaborations {
		return nil
	}

	sourceData, err := ep.extractSourceData(event)
	if err != nil {
		return fmt.Errorf("failed to extract source data: %w", err)
	}

	return ep.triggerIncrementalSync(ctx, sourceData.ID, "collaboration_invite")
}

func (ep *EventProcessor) handleCollaborationAccept(ctx context.Context, event *WebhookEvent) error {
	if !ep.syncManager.config.SyncCollaborations {
		return nil
	}

	sourceData, err := ep.extractSourceData(event)
	if err != nil {
		return fmt.Errorf("failed to extract source data: %w", err)
	}

	return ep.triggerIncrementalSync(ctx, sourceData.ID, "collaboration_accept")
}

func (ep *EventProcessor) handleCollaborationRoleChange(ctx context.Context, event *WebhookEvent) error {
	if !ep.syncManager.config.SyncCollaborations {
		return nil
	}

	sourceData, err := ep.extractSourceData(event)
	if err != nil {
		return fmt.Errorf("failed to extract source data: %w", err)
	}

	return ep.triggerIncrementalSync(ctx, sourceData.ID, "collaboration_role_change")
}

func (ep *EventProcessor) handleCollaborationRemove(ctx context.Context, event *WebhookEvent) error {
	if !ep.syncManager.config.SyncCollaborations {
		return nil
	}

	sourceData, err := ep.extractSourceData(event)
	if err != nil {
		return fmt.Errorf("failed to extract source data: %w", err)
	}

	return ep.triggerIncrementalSync(ctx, sourceData.ID, "collaboration_remove")
}

func (ep *EventProcessor) handleSharedLinkCreate(ctx context.Context, event *WebhookEvent) error {
	sourceData, err := ep.extractSourceData(event)
	if err != nil {
		return fmt.Errorf("failed to extract source data: %w", err)
	}

	return ep.triggerIncrementalSync(ctx, sourceData.ID, "shared_link_create")
}

func (ep *EventProcessor) handleSharedLinkDelete(ctx context.Context, event *WebhookEvent) error {
	sourceData, err := ep.extractSourceData(event)
	if err != nil {
		return fmt.Errorf("failed to extract source data: %w", err)
	}

	return ep.triggerIncrementalSync(ctx, sourceData.ID, "shared_link_delete")
}

// Helper methods

func (ep *EventProcessor) extractSourceData(event *WebhookEvent) (*EventSourceData, error) {
	// Parse source data from event
	sourceBytes, err := json.Marshal(event.Source)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal source data: %w", err)
	}

	var sourceData EventSourceData
	if err := json.Unmarshal(sourceBytes, &sourceData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal source data: %w", err)
	}

	return &sourceData, nil
}

func (ep *EventProcessor) triggerIncrementalSync(ctx context.Context, itemID, action string) error {
	// This would trigger a targeted sync for the specific item
	// For now, this is a placeholder implementation

	// In a real implementation, this would:
	// 1. Queue the item for sync
	// 2. Update local sync state
	// 3. Optionally trigger immediate sync for high-priority items

	return nil
}

// EventSourceData represents the source data from a webhook event
type EventSourceData struct {
	Type              string           `json:"type"`
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	Parent            *EventSourceData `json:"parent,omitempty"`
	OwnedBy           *User            `json:"owned_by,omitempty"`
	CreatedAt         string           `json:"created_at,omitempty"`
	ModifiedAt        string           `json:"modified_at,omitempty"`
	TrashedAt         string           `json:"trashed_at,omitempty"`
	PurgedAt          string           `json:"purged_at,omitempty"`
	ContentCreatedAt  string           `json:"content_created_at,omitempty"`
	ContentModifiedAt string           `json:"content_modified_at,omitempty"`
	SequenceID        string           `json:"sequence_id,omitempty"`
	ETag              string           `json:"etag,omitempty"`
	SHA1              string           `json:"sha1,omitempty"`
	Size              int64            `json:"size,omitempty"`
}

// WebhookRegistrar handles webhook registration and management
type WebhookRegistrar struct {
	connector *BoxConnector
	config    *BoxWebhookConfig
	tracer    trace.Tracer
}

// NewWebhookRegistrar creates a new webhook registrar
func NewWebhookRegistrar(connector *BoxConnector, config *BoxWebhookConfig) *WebhookRegistrar {
	return &WebhookRegistrar{
		connector: connector,
		config:    config,
		tracer:    otel.Tracer("box-webhook-registrar"),
	}
}

// RegisterWebhook registers a webhook with Box
func (wr *WebhookRegistrar) RegisterWebhook(ctx context.Context) error {
	ctx, span := wr.tracer.Start(ctx, "register_webhook")
	defer span.End()

	// Prepare webhook registration payload
	payload := map[string]interface{}{
		"target": map[string]interface{}{
			"type": "folder",
			"id":   "0", // Root folder
		},
		"address":  wr.config.WebhookURL,
		"triggers": wr.config.Triggers,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	// Make API request to register webhook
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.box.com/2.0/webhooks", strings.NewReader(string(payloadBytes)))
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to create webhook registration request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+wr.connector.accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := wr.connector.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to register webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		span.RecordError(fmt.Errorf("webhook registration failed with status: %d", resp.StatusCode))
		return fmt.Errorf("webhook registration failed with status: %d", resp.StatusCode)
	}

	// Parse response to get webhook ID
	var webhookResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&webhookResponse); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to parse webhook registration response: %w", err)
	}

	if id, ok := webhookResponse["id"].(string); ok {
		wr.config.WebhookID = id
	}

	span.SetAttributes(
		attribute.String("webhook.id", wr.config.WebhookID),
		attribute.String("webhook.url", wr.config.WebhookURL),
	)

	return nil
}

// UnregisterWebhook unregisters a webhook from Box
func (wr *WebhookRegistrar) UnregisterWebhook(ctx context.Context) error {
	ctx, span := wr.tracer.Start(ctx, "unregister_webhook")
	defer span.End()

	if wr.config.WebhookID == "" {
		return fmt.Errorf("no webhook ID configured")
	}

	// Make API request to delete webhook
	req, err := http.NewRequestWithContext(ctx, "DELETE",
		fmt.Sprintf("https://api.box.com/2.0/webhooks/%s", wr.config.WebhookID), nil)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to create webhook deletion request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+wr.connector.accessToken)

	resp, err := wr.connector.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to unregister webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		span.RecordError(fmt.Errorf("webhook unregistration failed with status: %d", resp.StatusCode))
		return fmt.Errorf("webhook unregistration failed with status: %d", resp.StatusCode)
	}

	span.SetAttributes(
		attribute.String("webhook.id", wr.config.WebhookID),
	)

	return nil
}
