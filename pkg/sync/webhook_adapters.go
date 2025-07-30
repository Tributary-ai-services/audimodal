package sync

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// GoogleDriveWebhookAdapter adapts Google Drive webhook events
type GoogleDriveWebhookAdapter struct{}

func (a *GoogleDriveWebhookAdapter) AdaptWebhookEvent(rawEvent []byte, headers map[string]string) (*StandardWebhookEvent, error) {
	var driveEvent GoogleDriveWebhookEvent
	if err := json.Unmarshal(rawEvent, &driveEvent); err != nil {
		return nil, fmt.Errorf("failed to parse Google Drive webhook: %w", err)
	}

	// Map Google Drive event types to standard event types
	var eventType WebhookEventType
	switch driveEvent.Kind {
	case "drive#change":
		if driveEvent.Removed {
			eventType = WebhookEventTypeFileDeleted
		} else if driveEvent.File != nil {
			eventType = WebhookEventTypeFileUpdated
		} else {
			eventType = WebhookEventTypeFileCreated
		}
	default:
		eventType = WebhookEventTypeFileUpdated
	}

	// Extract resource information
	resource := WebhookResource{
		Type: "file",
	}

	if driveEvent.File != nil {
		resource.ID = driveEvent.File.ID
		resource.Name = driveEvent.File.Name
		resource.Path = "/" + driveEvent.File.Name
		resource.URL = driveEvent.File.WebViewLink
		resource.MimeType = driveEvent.File.MimeType
		if driveEvent.File.Size != "" {
			// Convert size string to int64 if needed
		}
		if driveEvent.File.CreatedTime != "" {
			createdTime, _ := time.Parse(time.RFC3339, driveEvent.File.CreatedTime)
			resource.CreatedAt = &createdTime
		}
		if driveEvent.File.ModifiedTime != "" {
			modifiedTime, _ := time.Parse(time.RFC3339, driveEvent.File.ModifiedTime)
			resource.ModifiedAt = &modifiedTime
		}
	}

	// Create standard webhook event
	standardEvent := &StandardWebhookEvent{
		ID:            uuid.New().String(),
		EventType:     eventType,
		ConnectorType: "googledrive",
		Timestamp:     time.Now(),
		Resource:      resource,
		Metadata: map[string]interface{}{
			"google_drive": map[string]interface{}{
				"kind":      driveEvent.Kind,
				"change_id": driveEvent.ID,
				"team_drive_id": driveEvent.TeamDriveID,
			},
		},
		OriginalEvent: rawEvent,
	}

	return standardEvent, nil
}

func (a *GoogleDriveWebhookAdapter) VerifyWebhookSignature(payload []byte, signature string, secret string) bool {
	// Google Drive uses X-Goog-Channel-Token header instead of HMAC signature
	// This would need to verify the token matches the one set during subscription
	return true // Simplified for this example
}

func (a *GoogleDriveWebhookAdapter) GetConnectorType() string {
	return "googledrive"
}

// OneDriveWebhookAdapter adapts OneDrive/Microsoft Graph webhook events
type OneDriveWebhookAdapter struct{}

func (a *OneDriveWebhookAdapter) AdaptWebhookEvent(rawEvent []byte, headers map[string]string) (*StandardWebhookEvent, error) {
	var graphNotification MicrosoftGraphNotification
	if err := json.Unmarshal(rawEvent, &graphNotification); err != nil {
		return nil, fmt.Errorf("failed to parse Microsoft Graph notification: %w", err)
	}

	// Process the first notification in the batch
	if len(graphNotification.Value) == 0 {
		return nil, fmt.Errorf("no notifications in batch")
	}

	notification := graphNotification.Value[0]

	// Map change type to standard event type
	var eventType WebhookEventType
	switch strings.ToLower(notification.ChangeType) {
	case "created":
		eventType = WebhookEventTypeFileCreated
	case "updated":
		eventType = WebhookEventTypeFileUpdated
	case "deleted":
		eventType = WebhookEventTypeFileDeleted
	default:
		eventType = WebhookEventTypeFileUpdated
	}

	// Extract resource information from the resource path
	resource := WebhookResource{
		Type: "file",
	}

	// Parse resource URL to extract file information
	if notification.Resource != "" {
		// Extract file ID from resource path like "/me/drive/items/{id}"
		parts := strings.Split(notification.Resource, "/")
		if len(parts) > 0 {
			resource.ID = parts[len(parts)-1]
		}
	}

	// Create standard webhook event
	standardEvent := &StandardWebhookEvent{
		ID:            notification.ID,
		EventType:     eventType,
		ConnectorType: "onedrive",
		Timestamp:     notification.EventTime,
		Resource:      resource,
		Metadata: map[string]interface{}{
			"microsoft_graph": map[string]interface{}{
				"subscription_id":     notification.SubscriptionID,
				"subscription_expiration": notification.SubscriptionExpirationDateTime,
				"tenant_id":          notification.TenantID,
				"client_state":       notification.ClientState,
			},
		},
		OriginalEvent: rawEvent,
	}

	return standardEvent, nil
}

func (a *OneDriveWebhookAdapter) VerifyWebhookSignature(payload []byte, signature string, secret string) bool {
	// Microsoft Graph uses validationToken during subscription creation
	// and ClientState for ongoing verification
	return true // Simplified for this example
}

func (a *OneDriveWebhookAdapter) GetConnectorType() string {
	return "onedrive"
}

// BoxWebhookAdapter adapts Box webhook events
type BoxWebhookAdapter struct{}

func (a *BoxWebhookAdapter) AdaptWebhookEvent(rawEvent []byte, headers map[string]string) (*StandardWebhookEvent, error) {
	var boxEvent BoxWebhookEvent
	if err := json.Unmarshal(rawEvent, &boxEvent); err != nil {
		return nil, fmt.Errorf("failed to parse Box webhook: %w", err)
	}

	// Map Box event types to standard event types
	var eventType WebhookEventType
	switch boxEvent.Trigger {
	case "FILE.UPLOADED":
		eventType = WebhookEventTypeFileCreated
	case "FILE.PREVIEW_GENERATED", "FILE.COPIED", "FILE.MOVED", "FILE.RENAMED":
		eventType = WebhookEventTypeFileUpdated
	case "FILE.DELETED", "FILE.TRASHED":
		eventType = WebhookEventTypeFileDeleted
	case "FOLDER.CREATED":
		eventType = WebhookEventTypeFolderCreated
	case "FOLDER.MOVED", "FOLDER.RENAMED", "FOLDER.COPIED":
		eventType = WebhookEventTypeFolderUpdated
	case "FOLDER.DELETED", "FOLDER.TRASHED":
		eventType = WebhookEventTypeFolderDeleted
	default:
		eventType = WebhookEventTypeFileUpdated
	}

	// Extract resource information
	resource := WebhookResource{
		Type: strings.ToLower(boxEvent.Source.Type),
	}

	if boxEvent.Source.ID != "" {
		resource.ID = boxEvent.Source.ID
		resource.Name = boxEvent.Source.Name
		resource.Path = "/" + boxEvent.Source.Name // Simplified path
		
		// Convert timestamps
		if boxEvent.Source.CreatedAt != "" {
			createdTime, _ := time.Parse(time.RFC3339, boxEvent.Source.CreatedAt)
			resource.CreatedAt = &createdTime
		}
		if boxEvent.Source.ModifiedAt != "" {
			modifiedTime, _ := time.Parse(time.RFC3339, boxEvent.Source.ModifiedAt)
			resource.ModifiedAt = &modifiedTime
		}

		// Set file-specific properties
		if boxEvent.Source.Type == "file" {
			resource.Size = &boxEvent.Source.Size
		}
	}

	// Extract actor information
	var actor *WebhookActor
	if boxEvent.CreatedBy.ID != "" {
		actor = &WebhookActor{
			Type:  "user",
			ID:    boxEvent.CreatedBy.ID,
			Name:  boxEvent.CreatedBy.Name,
			Email: boxEvent.CreatedBy.Login,
		}
	}

	// Create standard webhook event
	standardEvent := &StandardWebhookEvent{
		ID:            boxEvent.WebhookID,
		EventType:     eventType,
		ConnectorType: "box",
		Timestamp:     boxEvent.CreatedAt,
		Resource:      resource,
		Actor:         actor,
		Metadata: map[string]interface{}{
			"box": map[string]interface{}{
				"trigger":     boxEvent.Trigger,
				"webhook_id":  boxEvent.WebhookID,
				"enterprise":  boxEvent.Enterprise,
			},
		},
		OriginalEvent: rawEvent,
	}

	return standardEvent, nil
}

func (a *BoxWebhookAdapter) VerifyWebhookSignature(payload []byte, signature string, secret string) bool {
	// Box uses HMAC-SHA256 signature
	expectedMAC := hmac.New(sha256.New, []byte(secret))
	expectedMAC.Write(payload)
	expectedSignature := hex.EncodeToString(expectedMAC.Sum(nil))
	
	// Remove "sha256=" prefix if present
	signature = strings.TrimPrefix(signature, "sha256=")
	
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

func (a *BoxWebhookAdapter) GetConnectorType() string {
	return "box"
}

// DropboxWebhookAdapter adapts Dropbox webhook events
type DropboxWebhookAdapter struct{}

func (a *DropboxWebhookAdapter) AdaptWebhookEvent(rawEvent []byte, headers map[string]string) (*StandardWebhookEvent, error) {
	var dropboxEvent DropboxWebhookEvent
	if err := json.Unmarshal(rawEvent, &dropboxEvent); err != nil {
		return nil, fmt.Errorf("failed to parse Dropbox webhook: %w", err)
	}

	// Dropbox webhooks are typically just notifications that changes occurred
	// We would need to call the API to get specific change details
	eventType := WebhookEventTypeFileUpdated // Default to file updated

	// Create standard webhook event
	standardEvent := &StandardWebhookEvent{
		ID:            uuid.New().String(),
		EventType:     eventType,
		ConnectorType: "dropbox",
		Timestamp:     time.Now(),
		Resource: WebhookResource{
			Type: "file", // Default type
		},
		Metadata: map[string]interface{}{
			"dropbox": map[string]interface{}{
				"accounts": dropboxEvent.Accounts,
				"delta":    dropboxEvent.Delta,
			},
		},
		OriginalEvent: rawEvent,
	}

	return standardEvent, nil
}

func (a *DropboxWebhookAdapter) VerifyWebhookSignature(payload []byte, signature string, secret string) bool {
	// Dropbox uses HMAC-SHA256 signature
	expectedMAC := hmac.New(sha256.New, []byte(secret))
	expectedMAC.Write(payload)
	expectedSignature := hex.EncodeToString(expectedMAC.Sum(nil))
	
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

func (a *DropboxWebhookAdapter) GetConnectorType() string {
	return "dropbox"
}

// SlackWebhookAdapter adapts Slack webhook events
type SlackWebhookAdapter struct{}

func (a *SlackWebhookAdapter) AdaptWebhookEvent(rawEvent []byte, headers map[string]string) (*StandardWebhookEvent, error) {
	var slackEvent SlackWebhookEvent
	if err := json.Unmarshal(rawEvent, &slackEvent); err != nil {
		return nil, fmt.Errorf("failed to parse Slack webhook: %w", err)
	}

	// Map Slack event types to standard event types
	var eventType WebhookEventType
	switch slackEvent.Event.Type {
	case "message":
		if slackEvent.Event.Subtype == "file_share" {
			eventType = WebhookEventTypeFileCreated
		} else {
			eventType = WebhookEventTypeFileUpdated
		}
	case "file_created":
		eventType = WebhookEventTypeFileCreated
	case "file_public", "file_shared", "file_unshared":
		eventType = WebhookEventTypeSharedUpdated
	case "file_deleted":
		eventType = WebhookEventTypeFileDeleted
	default:
		eventType = WebhookEventTypeFileUpdated
	}

	// Extract resource information
	resource := WebhookResource{
		Type: "message", // Default to message for Slack
	}

	if slackEvent.Event.File.ID != "" {
		resource.Type = "file"
		resource.ID = slackEvent.Event.File.ID
		resource.Name = slackEvent.Event.File.Name
		resource.URL = slackEvent.Event.File.URLPrivate
		resource.MimeType = slackEvent.Event.File.Mimetype
		if slackEvent.Event.File.Size > 0 {
			resource.Size = &slackEvent.Event.File.Size
		}
		if slackEvent.Event.File.Created > 0 {
			createdTime := time.Unix(slackEvent.Event.File.Created, 0)
			resource.CreatedAt = &createdTime
		}
	} else if slackEvent.Event.Channel != "" {
		resource.ID = slackEvent.Event.Channel
		resource.Name = slackEvent.Event.Channel
	}

	// Extract actor information
	var actor *WebhookActor
	if slackEvent.Event.User != "" {
		actor = &WebhookActor{
			Type: "user",
			ID:   slackEvent.Event.User,
		}
	}

	// Create standard webhook event
	standardEvent := &StandardWebhookEvent{
		ID:            slackEvent.EventID,
		EventType:     eventType,
		ConnectorType: "slack",
		Timestamp:     time.Unix(slackEvent.EventTime, 0),
		Resource:      resource,
		Actor:         actor,
		Metadata: map[string]interface{}{
			"slack": map[string]interface{}{
				"team_id":     slackEvent.TeamID,
				"api_app_id":  slackEvent.APIAppID,
				"event_type":  slackEvent.Event.Type,
				"event_subtype": slackEvent.Event.Subtype,
				"channel":     slackEvent.Event.Channel,
			},
		},
		OriginalEvent: rawEvent,
	}

	return standardEvent, nil
}

func (a *SlackWebhookAdapter) VerifyWebhookSignature(payload []byte, signature string, secret string) bool {
	// Slack uses a specific signature verification method
	timestamp := time.Now().Unix() // Would get from headers in real implementation
	basestring := fmt.Sprintf("v0:%d:%s", timestamp, payload)
	
	expectedMAC := hmac.New(sha256.New, []byte(secret))
	expectedMAC.Write([]byte(basestring))
	expectedSignature := "v0=" + hex.EncodeToString(expectedMAC.Sum(nil))
	
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

func (a *SlackWebhookAdapter) GetConnectorType() string {
	return "slack"
}

// NotionWebhookAdapter adapts Notion webhook events
type NotionWebhookAdapter struct{}

func (a *NotionWebhookAdapter) AdaptWebhookEvent(rawEvent []byte, headers map[string]string) (*StandardWebhookEvent, error) {
	// Notion doesn't currently support webhooks in the same way as other services
	// This is a placeholder for future webhook support
	return nil, fmt.Errorf("Notion webhooks not yet supported")
}

func (a *NotionWebhookAdapter) VerifyWebhookSignature(payload []byte, signature string, secret string) bool {
	return false // Not supported yet
}

func (a *NotionWebhookAdapter) GetConnectorType() string {
	return "notion"
}

// Webhook event structures for different services

type GoogleDriveWebhookEvent struct {
	Kind         string             `json:"kind"`
	ID           string             `json:"id"`
	Type         string             `json:"type"`
	Removed      bool               `json:"removed"`
	File         *GoogleDriveFile   `json:"file,omitempty"`
	TeamDriveID  string             `json:"teamDriveId,omitempty"`
}

type GoogleDriveFile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MimeType     string `json:"mimeType"`
	WebViewLink  string `json:"webViewLink"`
	Size         string `json:"size,omitempty"`
	CreatedTime  string `json:"createdTime"`
	ModifiedTime string `json:"modifiedTime"`
}

type MicrosoftGraphNotification struct {
	Value []MicrosoftGraphNotificationItem `json:"value"`
}

type MicrosoftGraphNotificationItem struct {
	ID                             string    `json:"id"`
	SubscriptionID                 string    `json:"subscriptionId"`
	SubscriptionExpirationDateTime time.Time `json:"subscriptionExpirationDateTime"`
	EventTime                      time.Time `json:"eventTime"`
	Resource                       string    `json:"resource"`
	ResourceData                   map[string]interface{} `json:"resourceData"`
	ChangeType                     string    `json:"changeType"`
	TenantID                       string    `json:"tenantId"`
	ClientState                    string    `json:"clientState,omitempty"`
}

type BoxWebhookEvent struct {
	Type       string    `json:"type"`
	WebhookID  string    `json:"webhook_id"`
	Timestamp  time.Time `json:"timestamp"`
	Trigger    string    `json:"trigger"`
	Source     BoxItem   `json:"source"`
	CreatedBy  BoxUser   `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	Enterprise BoxEnterprise `json:"enterprise,omitempty"`
}

type BoxItem struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Size       int64  `json:"size,omitempty"`
	CreatedAt  string `json:"created_at"`
	ModifiedAt string `json:"modified_at"`
	Description string `json:"description,omitempty"`
}

type BoxUser struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Login string `json:"login"`
}

type BoxEnterprise struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DropboxWebhookEvent struct {
	ListFolder DropboxListFolder `json:"list_folder"`
	Accounts   []string          `json:"accounts"`
	Delta      DropboxDelta      `json:"delta"`
}

type DropboxListFolder struct {
	Accounts []string `json:"accounts"`
}

type DropboxDelta struct {
	Users []string `json:"users"`
}

type SlackWebhookEvent struct {
	Token       string           `json:"token"`
	TeamID      string           `json:"team_id"`
	APIAppID    string           `json:"api_app_id"`
	Event       SlackEvent       `json:"event"`
	Type        string           `json:"type"`
	EventID     string           `json:"event_id"`
	EventTime   int64            `json:"event_time"`
	Challenge   string           `json:"challenge,omitempty"`
}

type SlackEvent struct {
	Type        string    `json:"type"`
	Subtype     string    `json:"subtype,omitempty"`
	Channel     string    `json:"channel,omitempty"`
	User        string    `json:"user,omitempty"`
	Text        string    `json:"text,omitempty"`
	Timestamp   string    `json:"ts,omitempty"`
	File        SlackFile `json:"file,omitempty"`
}

type SlackFile struct {
	ID          string `json:"id"`
	Created     int64  `json:"created"`
	Timestamp   int64  `json:"timestamp"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	Mimetype    string `json:"mimetype"`
	FileType    string `json:"filetype"`
	PrettyType  string `json:"pretty_type"`
	User        string `json:"user"`
	Size        int64  `json:"size"`
	Mode        string `json:"mode"`
	IsExternal  bool   `json:"is_external"`
	ExternalType string `json:"external_type"`
	IsPublic    bool   `json:"is_public"`
	PublicURLShared bool `json:"public_url_shared"`
	URLPrivate  string `json:"url_private"`
	URLPrivateDownload string `json:"url_private_download"`
	Permalink   string `json:"permalink"`
	PermalinkPublic string `json:"permalink_public"`
}