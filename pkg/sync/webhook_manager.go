package sync

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// WebhookManager manages webhook subscriptions and event processing
type WebhookManager struct {
	config        *WebhookConfig
	subscriptions map[string]*WebhookSubscription
	eventHandlers map[string]WebhookEventHandler
	mutex         sync.RWMutex
	tracer        trace.Tracer
}

// WebhookConfig contains webhook management configuration
type WebhookConfig struct {
	BaseURL            string        `json:"base_url"`
	Secret             string        `json:"secret"`
	MaxRetries         int           `json:"max_retries"`
	RetryInterval      time.Duration `json:"retry_interval"`
	EventBufferSize    int           `json:"event_buffer_size"`
	EnableVerification bool          `json:"enable_verification"`
	TimeoutDuration    time.Duration `json:"timeout_duration"`
}

// WebhookSubscription represents a webhook subscription
type WebhookSubscription struct {
	ID            string                 `json:"id"`
	ConnectorType string                 `json:"connector_type"`
	DataSourceID  uuid.UUID              `json:"data_source_id"`
	URL           string                 `json:"url"`
	EventTypes    []WebhookEventType     `json:"event_types"`
	Secret        string                 `json:"secret"`
	Headers       map[string]string      `json:"headers"`
	IsActive      bool                   `json:"is_active"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	LastEventAt   *time.Time             `json:"last_event_at,omitempty"`
	ExpiresAt     *time.Time             `json:"expires_at,omitempty"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// WebhookEventType defines types of webhook events
type WebhookEventType string

const (
	WebhookEventTypeFileCreated       WebhookEventType = "file.created"
	WebhookEventTypeFileUpdated       WebhookEventType = "file.updated"
	WebhookEventTypeFileDeleted       WebhookEventType = "file.deleted"
	WebhookEventTypeFileMoved         WebhookEventType = "file.moved"
	WebhookEventTypeFileRenamed       WebhookEventType = "file.renamed"
	WebhookEventTypeFolderCreated     WebhookEventType = "folder.created"
	WebhookEventTypeFolderUpdated     WebhookEventType = "folder.updated"
	WebhookEventTypeFolderDeleted     WebhookEventType = "folder.deleted"
	WebhookEventTypeSyncStarted       WebhookEventType = "sync.started"
	WebhookEventTypeSyncCompleted     WebhookEventType = "sync.completed"
	WebhookEventTypeSyncFailed        WebhookEventType = "sync.failed"
	WebhookEventTypePermissionChanged WebhookEventType = "permission.changed"
	WebhookEventTypeSharedUpdated     WebhookEventType = "share.updated"
)

// StandardWebhookEvent represents a standardized webhook event
type StandardWebhookEvent struct {
	ID            string                 `json:"id"`
	EventType     WebhookEventType       `json:"event_type"`
	ConnectorType string                 `json:"connector_type"`
	DataSourceID  uuid.UUID              `json:"data_source_id"`
	Timestamp     time.Time              `json:"timestamp"`
	Resource      WebhookResource        `json:"resource"`
	Changes       []WebhookChange        `json:"changes,omitempty"`
	Actor         *WebhookActor          `json:"actor,omitempty"`
	Metadata      map[string]interface{} `json:"metadata"`
	OriginalEvent json.RawMessage        `json:"original_event,omitempty"`
}

// WebhookResource represents the resource affected by the event
type WebhookResource struct {
	Type       string                 `json:"type"` // file, folder, sync_job, etc.
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Path       string                 `json:"path"`
	URL        string                 `json:"url,omitempty"`
	Size       *int64                 `json:"size,omitempty"`
	MimeType   string                 `json:"mime_type,omitempty"`
	CreatedAt  *time.Time             `json:"created_at,omitempty"`
	ModifiedAt *time.Time             `json:"modified_at,omitempty"`
	Checksum   string                 `json:"checksum,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// WebhookChange represents a specific change within an event
type WebhookChange struct {
	Field     string      `json:"field"`
	OldValue  interface{} `json:"old_value,omitempty"`
	NewValue  interface{} `json:"new_value"`
	Operation string      `json:"operation"` // created, updated, deleted
}

// WebhookActor represents who or what triggered the event
type WebhookActor struct {
	Type     string                 `json:"type"` // user, application, system
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Email    string                 `json:"email,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// WebhookEventHandler defines how to handle webhook events
type WebhookEventHandler interface {
	HandleEvent(ctx context.Context, event *StandardWebhookEvent) error
	GetSupportedEventTypes() []WebhookEventType
}

// ConnectorWebhookAdapter defines how connectors should adapt their webhook events
type ConnectorWebhookAdapter interface {
	AdaptWebhookEvent(rawEvent []byte, headers map[string]string) (*StandardWebhookEvent, error)
	VerifyWebhookSignature(payload []byte, signature string, secret string) bool
	GetConnectorType() string
}

// NewWebhookManager creates a new webhook manager
func NewWebhookManager(config *WebhookConfig) *WebhookManager {
	if config == nil {
		config = &WebhookConfig{
			MaxRetries:         3,
			RetryInterval:      5 * time.Second,
			EventBufferSize:    1000,
			EnableVerification: true,
			TimeoutDuration:    30 * time.Second,
		}
	}

	return &WebhookManager{
		config:        config,
		subscriptions: make(map[string]*WebhookSubscription),
		eventHandlers: make(map[string]WebhookEventHandler),
		tracer:        otel.Tracer("webhook-manager"),
	}
}

// RegisterSubscription registers a new webhook subscription
func (wm *WebhookManager) RegisterSubscription(ctx context.Context, subscription *WebhookSubscription) error {
	ctx, span := wm.tracer.Start(ctx, "webhook_manager.register_subscription")
	defer span.End()

	span.SetAttributes(
		attribute.String("connector_type", subscription.ConnectorType),
		attribute.String("data_source_id", subscription.DataSourceID.String()),
		attribute.String("url", subscription.URL),
	)

	wm.mutex.Lock()
	defer wm.mutex.Unlock()

	if subscription.ID == "" {
		subscription.ID = uuid.New().String()
	}

	subscription.CreatedAt = time.Now()
	subscription.UpdatedAt = time.Now()

	wm.subscriptions[subscription.ID] = subscription

	return nil
}

// UnregisterSubscription removes a webhook subscription
func (wm *WebhookManager) UnregisterSubscription(ctx context.Context, subscriptionID string) error {
	ctx, span := wm.tracer.Start(ctx, "webhook_manager.unregister_subscription")
	defer span.End()

	span.SetAttributes(attribute.String("subscription_id", subscriptionID))

	wm.mutex.Lock()
	defer wm.mutex.Unlock()

	delete(wm.subscriptions, subscriptionID)

	return nil
}

// GetSubscription retrieves a webhook subscription
func (wm *WebhookManager) GetSubscription(ctx context.Context, subscriptionID string) (*WebhookSubscription, error) {
	ctx, span := wm.tracer.Start(ctx, "webhook_manager.get_subscription")
	defer span.End()

	span.SetAttributes(attribute.String("subscription_id", subscriptionID))

	wm.mutex.RLock()
	defer wm.mutex.RUnlock()

	subscription, exists := wm.subscriptions[subscriptionID]
	if !exists {
		return nil, fmt.Errorf("subscription not found")
	}

	return subscription, nil
}

// ListSubscriptions returns all webhook subscriptions with optional filtering
func (wm *WebhookManager) ListSubscriptions(ctx context.Context, filters *WebhookSubscriptionFilters) ([]*WebhookSubscription, error) {
	ctx, span := wm.tracer.Start(ctx, "webhook_manager.list_subscriptions")
	defer span.End()

	wm.mutex.RLock()
	defer wm.mutex.RUnlock()

	var subscriptions []*WebhookSubscription
	for _, subscription := range wm.subscriptions {
		// Apply filters
		if filters != nil {
			if filters.ConnectorType != "" && subscription.ConnectorType != filters.ConnectorType {
				continue
			}
			if filters.DataSourceID != uuid.Nil && subscription.DataSourceID != filters.DataSourceID {
				continue
			}
			if filters.IsActive != nil && subscription.IsActive != *filters.IsActive {
				continue
			}
		}

		subscriptions = append(subscriptions, subscription)
	}

	span.SetAttributes(attribute.Int("subscriptions_count", len(subscriptions)))

	return subscriptions, nil
}

// RegisterEventHandler registers an event handler for specific event types
func (wm *WebhookManager) RegisterEventHandler(eventType string, handler WebhookEventHandler) {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()

	wm.eventHandlers[eventType] = handler
}

// ProcessIncomingWebhook processes an incoming webhook event
func (wm *WebhookManager) ProcessIncomingWebhook(ctx context.Context, connectorType string, payload []byte, headers map[string]string) error {
	ctx, span := wm.tracer.Start(ctx, "webhook_manager.process_incoming_webhook")
	defer span.End()

	span.SetAttributes(
		attribute.String("connector_type", connectorType),
		attribute.Int("payload_size", len(payload)),
	)

	// Get the appropriate adapter for this connector type
	adapter, err := wm.getConnectorAdapter(connectorType)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("no adapter found for connector type %s: %w", connectorType, err)
	}

	// Verify webhook signature if enabled
	if wm.config.EnableVerification {
		signature := headers["X-Hub-Signature-256"]
		if signature == "" {
			signature = headers["X-Signature"] // Alternative header name
		}

		if signature != "" && !adapter.VerifyWebhookSignature(payload, signature, wm.config.Secret) {
			err := fmt.Errorf("webhook signature verification failed")
			span.RecordError(err)
			return err
		}
	}

	// Adapt the raw webhook event to our standard format
	standardEvent, err := adapter.AdaptWebhookEvent(payload, headers)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to adapt webhook event: %w", err)
	}

	// Process the standard event
	return wm.processStandardEvent(ctx, standardEvent)
}

// processStandardEvent processes a standardized webhook event
func (wm *WebhookManager) processStandardEvent(ctx context.Context, event *StandardWebhookEvent) error {
	ctx, span := wm.tracer.Start(ctx, "webhook_manager.process_standard_event")
	defer span.End()

	span.SetAttributes(
		attribute.String("event.id", event.ID),
		attribute.String("event.type", string(event.EventType)),
		attribute.String("connector_type", event.ConnectorType),
		attribute.String("resource.type", event.Resource.Type),
		attribute.String("resource.id", event.Resource.ID),
	)

	// Find relevant subscriptions
	subscriptions := wm.findMatchingSubscriptions(event)
	if len(subscriptions) == 0 {
		span.AddEvent("No matching subscriptions found")
		return nil
	}

	// Update subscription metadata
	now := time.Now()
	for _, subscription := range subscriptions {
		subscription.LastEventAt = &now
		subscription.UpdatedAt = now
	}

	// Find and execute event handlers
	handler, exists := wm.eventHandlers[string(event.EventType)]
	if !exists {
		// Try wildcard handler
		handler, exists = wm.eventHandlers["*"]
	}

	if exists {
		if err := handler.HandleEvent(ctx, event); err != nil {
			span.RecordError(err)
			return fmt.Errorf("event handler failed: %w", err)
		}
	}

	// Notify registered webhooks
	for _, subscription := range subscriptions {
		go wm.notifyWebhook(context.Background(), subscription, event)
	}

	span.SetAttributes(
		attribute.Int("matching_subscriptions", len(subscriptions)),
		attribute.Bool("handler_executed", exists),
	)

	return nil
}

// findMatchingSubscriptions finds subscriptions that should receive this event
func (wm *WebhookManager) findMatchingSubscriptions(event *StandardWebhookEvent) []*WebhookSubscription {
	wm.mutex.RLock()
	defer wm.mutex.RUnlock()

	var matches []*WebhookSubscription
	for _, subscription := range wm.subscriptions {
		if !subscription.IsActive {
			continue
		}

		// Check if subscription has expired
		if subscription.ExpiresAt != nil && subscription.ExpiresAt.Before(time.Now()) {
			continue
		}

		// Check connector type match
		if subscription.ConnectorType != "" && subscription.ConnectorType != event.ConnectorType {
			continue
		}

		// Check data source match
		if subscription.DataSourceID != uuid.Nil && subscription.DataSourceID != event.DataSourceID {
			continue
		}

		// Check event type match
		eventTypeMatch := false
		for _, eventType := range subscription.EventTypes {
			if eventType == event.EventType {
				eventTypeMatch = true
				break
			}
		}
		if !eventTypeMatch {
			continue
		}

		matches = append(matches, subscription)
	}

	return matches
}

// notifyWebhook sends the event to a specific webhook subscription
func (wm *WebhookManager) notifyWebhook(ctx context.Context, subscription *WebhookSubscription, event *StandardWebhookEvent) {
	ctx, span := wm.tracer.Start(ctx, "webhook_manager.notify_webhook")
	defer span.End()

	span.SetAttributes(
		attribute.String("subscription.id", subscription.ID),
		attribute.String("webhook.url", subscription.URL),
	)

	// Prepare webhook payload
	payload, err := json.Marshal(event)
	if err != nil {
		span.RecordError(err)
		return
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", subscription.URL, strings.NewReader(string(payload)))
	if err != nil {
		span.RecordError(err)
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AudiModal-Webhook/1.0")
	req.Header.Set("X-Event-Type", string(event.EventType))
	req.Header.Set("X-Event-ID", event.ID)
	req.Header.Set("X-Timestamp", event.Timestamp.Format(time.RFC3339))

	// Add custom headers from subscription
	for key, value := range subscription.Headers {
		req.Header.Set(key, value)
	}

	// Add signature if secret is provided
	if subscription.Secret != "" {
		signature := wm.generateSignature(payload, subscription.Secret)
		req.Header.Set("X-Hub-Signature-256", signature)
	}

	// Execute webhook with retries
	client := &http.Client{Timeout: wm.config.TimeoutDuration}

	var lastErr error
	for attempt := 0; attempt <= wm.config.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(wm.config.RetryInterval)
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			span.SetAttributes(
				attribute.Int("http.status_code", resp.StatusCode),
				attribute.Int("attempt_number", attempt+1),
			)
			return // Success
		}

		lastErr = fmt.Errorf("webhook returned status %d", resp.StatusCode)

		// Don't retry client errors (4xx)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			break
		}
	}

	span.RecordError(lastErr)
	span.SetAttributes(
		attribute.Bool("delivery_failed", true),
		attribute.String("last_error", lastErr.Error()),
	)
}

// generateSignature generates HMAC-SHA256 signature for webhook verification
func (wm *WebhookManager) generateSignature(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// getConnectorAdapter returns the appropriate webhook adapter for a connector type
func (wm *WebhookManager) getConnectorAdapter(connectorType string) (ConnectorWebhookAdapter, error) {
	// This would typically be populated from a registry of adapters
	// For now, return an error indicating the adapter needs to be implemented
	return nil, fmt.Errorf("webhook adapter for %s not implemented", connectorType)
}

// CleanupExpiredSubscriptions removes expired webhook subscriptions
func (wm *WebhookManager) CleanupExpiredSubscriptions(ctx context.Context) error {
	ctx, span := wm.tracer.Start(ctx, "webhook_manager.cleanup_expired_subscriptions")
	defer span.End()

	wm.mutex.Lock()
	defer wm.mutex.Unlock()

	now := time.Now()
	var expiredCount int

	for id, subscription := range wm.subscriptions {
		if subscription.ExpiresAt != nil && subscription.ExpiresAt.Before(now) {
			delete(wm.subscriptions, id)
			expiredCount++
		}
	}

	span.SetAttributes(attribute.Int("expired_subscriptions", expiredCount))

	return nil
}

// GetWebhookStats returns statistics about webhook operations
func (wm *WebhookManager) GetWebhookStats(ctx context.Context) (*WebhookStats, error) {
	ctx, span := wm.tracer.Start(ctx, "webhook_manager.get_webhook_stats")
	defer span.End()

	wm.mutex.RLock()
	defer wm.mutex.RUnlock()

	stats := &WebhookStats{
		TotalSubscriptions:       len(wm.subscriptions),
		ActiveSubscriptions:      0,
		SubscriptionsByConnector: make(map[string]int),
		RecentEvents:             0, // Would track recent events in a production system
	}

	now := time.Now()
	for _, subscription := range wm.subscriptions {
		if subscription.IsActive && (subscription.ExpiresAt == nil || subscription.ExpiresAt.After(now)) {
			stats.ActiveSubscriptions++
		}
		stats.SubscriptionsByConnector[subscription.ConnectorType]++
	}

	return stats, nil
}

// WebhookSubscriptionFilters contains filters for listing subscriptions
type WebhookSubscriptionFilters struct {
	ConnectorType string           `json:"connector_type,omitempty"`
	DataSourceID  uuid.UUID        `json:"data_source_id,omitempty"`
	IsActive      *bool            `json:"is_active,omitempty"`
	EventType     WebhookEventType `json:"event_type,omitempty"`
}

// WebhookStats contains webhook operation statistics
type WebhookStats struct {
	TotalSubscriptions       int            `json:"total_subscriptions"`
	ActiveSubscriptions      int            `json:"active_subscriptions"`
	SubscriptionsByConnector map[string]int `json:"subscriptions_by_connector"`
	RecentEvents             int            `json:"recent_events"`
	LastEventAt              *time.Time     `json:"last_event_at,omitempty"`
}

// Shutdown gracefully shuts down the webhook manager
func (wm *WebhookManager) Shutdown(ctx context.Context) error {
	// In a production system, this would:
	// 1. Stop accepting new webhook events
	// 2. Wait for pending notifications to complete
	// 3. Clean up resources
	return nil
}
