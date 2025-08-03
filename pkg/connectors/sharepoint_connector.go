package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/events"
)

// SharePointConnector handles integration with Microsoft SharePoint
type SharePointConnector struct {
	config   *SharePointConfig
	client   *http.Client
	tracer   trace.Tracer
	producer *events.Producer

	// Authentication
	accessToken  string
	tokenExpiry  time.Time
	refreshToken string
}

// SharePointConfig contains configuration for SharePoint integration
type SharePointConfig struct {
	// OAuth Configuration
	TenantID     string `yaml:"tenant_id"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	Scope        string `yaml:"scope"`

	// SharePoint URLs
	BaseURL string `yaml:"base_url"` // https://tenant.sharepoint.com
	SiteURL string `yaml:"site_url"` // /sites/sitename
	AuthURL string `yaml:"auth_url"` // https://login.microsoftonline.com/{tenant}/oauth2/v2.0/token

	// Sync Configuration
	SyncInterval       time.Duration `yaml:"sync_interval"`
	InitialSyncEnabled bool          `yaml:"initial_sync_enabled"`
	DeltaSyncEnabled   bool          `yaml:"delta_sync_enabled"`
	WebhooksEnabled    bool          `yaml:"webhooks_enabled"`

	// File Filtering
	LibraryNames    []string `yaml:"library_names"`    // Document libraries to sync
	FileExtensions  []string `yaml:"file_extensions"`  // .docx, .pdf, .txt, etc.
	ExcludePatterns []string `yaml:"exclude_patterns"` // Patterns to exclude
	MaxFileSizeMB   int      `yaml:"max_file_size_mb"`

	// Processing Options
	EnableVersions    bool `yaml:"enable_versions"`    // Include version history
	EnableMetadata    bool `yaml:"enable_metadata"`    // Include SharePoint metadata
	EnablePermissions bool `yaml:"enable_permissions"` // Include permission info

	// Rate Limiting
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	MaxConcurrentReqs int     `yaml:"max_concurrent_requests"`

	// Error Handling
	RetryAttempts           int           `yaml:"retry_attempts"`
	RetryDelay              time.Duration `yaml:"retry_delay"`
	CircuitBreakerThreshold int           `yaml:"circuit_breaker_threshold"`
}

// SharePointFile represents a file in SharePoint
type SharePointFile struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	ServerRelativeURL string                 `json:"serverRelativeUrl"`
	Size              int64                  `json:"length"`
	Created           time.Time              `json:"timeCreated"`
	Modified          time.Time              `json:"timeLastModified"`
	CreatedBy         SharePointUser         `json:"createdBy"`
	ModifiedBy        SharePointUser         `json:"lastModifiedBy"`
	ContentType       string                 `json:"mimeType"`
	Checksum          string                 `json:"checksum,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	Permissions       []SharePointPermission `json:"permissions,omitempty"`
	Versions          []SharePointVersion    `json:"versions,omitempty"`
}

// SharePointUser represents a SharePoint user
type SharePointUser struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

// SharePointPermission represents file permissions
type SharePointPermission struct {
	PrincipalID   string   `json:"principalId"`
	PrincipalName string   `json:"principalName"`
	PrincipalType string   `json:"principalType"` // User, Group, etc.
	Roles         []string `json:"roles"`
}

// SharePointVersion represents a file version
type SharePointVersion struct {
	ID      string    `json:"id"`
	Label   string    `json:"label"`
	Size    int64     `json:"size"`
	Created time.Time `json:"created"`
	IsMinor bool      `json:"isMinor"`
	URL     string    `json:"url"`
}

// SharePointWebhook represents a SharePoint webhook subscription
type SharePointWebhook struct {
	ID              string    `json:"id"`
	Resource        string    `json:"resource"`
	NotificationURL string    `json:"notificationUrl"`
	ExpirationDate  time.Time `json:"expirationDateTime"`
	ClientState     string    `json:"clientState"`
}

// SharePointEvent represents a change event from SharePoint
type SharePointEvent struct {
	SubscriptionID  string    `json:"subscriptionId"`
	Resource        string    `json:"resource"`
	TenantID        string    `json:"tenantId"`
	SiteURL         string    `json:"siteUrl"`
	WebID           string    `json:"webId"`
	EventType       string    `json:"eventType"` // ItemAdded, ItemUpdated, ItemDeleted
	EventTime       time.Time `json:"eventTime"`
	UniqueID        string    `json:"uniqueId"`
	ItemID          string    `json:"itemId"`
	ListID          string    `json:"listId"`
	UserDisplayName string    `json:"userDisplayName"`
	UserLoginName   string    `json:"userLoginName"`
}

// SyncResult represents the result of a sync operation
type SyncResult struct {
	StartTime      time.Time     `json:"start_time"`
	EndTime        time.Time     `json:"end_time"`
	Duration       time.Duration `json:"duration"`
	FilesFound     int           `json:"files_found"`
	FilesProcessed int           `json:"files_processed"`
	FilesSkipped   int           `json:"files_skipped"`
	FilesErrored   int           `json:"files_errored"`
	LastSyncToken  string        `json:"last_sync_token,omitempty"`
	Errors         []string      `json:"errors,omitempty"`
}

// DefaultSharePointConfig returns default SharePoint configuration
func DefaultSharePointConfig() *SharePointConfig {
	return &SharePointConfig{
		Scope:                   "https://graph.microsoft.com/.default",
		SyncInterval:            15 * time.Minute,
		InitialSyncEnabled:      true,
		DeltaSyncEnabled:        true,
		WebhooksEnabled:         true,
		FileExtensions:          []string{".docx", ".pdf", ".txt", ".pptx", ".xlsx"},
		MaxFileSizeMB:           100,
		EnableVersions:          false,
		EnableMetadata:          true,
		EnablePermissions:       false,
		RequestsPerSecond:       10.0,
		MaxConcurrentReqs:       5,
		RetryAttempts:           3,
		RetryDelay:              5 * time.Second,
		CircuitBreakerThreshold: 10,
	}
}

// NewSharePointConnector creates a new SharePoint connector
func NewSharePointConnector(config *SharePointConfig, producer *events.Producer) *SharePointConnector {
	if config == nil {
		config = DefaultSharePointConfig()
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &SharePointConnector{
		config:   config,
		client:   client,
		tracer:   otel.Tracer("sharepoint-connector"),
		producer: producer,
	}
}

// Start starts the SharePoint connector
func (sp *SharePointConnector) Start(ctx context.Context) error {
	// Authenticate
	if err := sp.authenticate(ctx); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Perform initial sync if enabled
	if sp.config.InitialSyncEnabled {
		go sp.performInitialSync(ctx)
	}

	// Start delta sync loop if enabled
	if sp.config.DeltaSyncEnabled {
		go sp.deltaSyncLoop(ctx)
	}

	// Set up webhooks if enabled
	if sp.config.WebhooksEnabled {
		go sp.setupWebhooks(ctx)
	}

	return nil
}

// authenticate authenticates with Microsoft Graph API
func (sp *SharePointConnector) authenticate(ctx context.Context) error {
	ctx, span := sp.tracer.Start(ctx, "sharepoint_authenticate")
	defer span.End()

	data := url.Values{
		"client_id":     {sp.config.ClientID},
		"client_secret": {sp.config.ClientSecret},
		"scope":         {sp.config.Scope},
		"grant_type":    {"client_credentials"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", sp.config.AuthURL, strings.NewReader(data.Encode()))
	if err != nil {
		span.RecordError(err)
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := sp.client.Do(req)
	if err != nil {
		span.RecordError(err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("authentication failed: %s", string(body))
		span.RecordError(err)
		return err
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		span.RecordError(err)
		return err
	}

	sp.accessToken = tokenResp.AccessToken
	sp.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	span.SetAttributes(
		attribute.String("token.type", tokenResp.TokenType),
		attribute.Int("token.expires_in", tokenResp.ExpiresIn),
	)

	return nil
}

// performInitialSync performs the initial full sync
func (sp *SharePointConnector) performInitialSync(ctx context.Context) {
	ctx, span := sp.tracer.Start(ctx, "sharepoint_initial_sync")
	defer span.End()

	result := &SyncResult{
		StartTime: time.Now(),
	}

	defer func() {
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)

		span.SetAttributes(
			attribute.Int("sync.files_found", result.FilesFound),
			attribute.Int("sync.files_processed", result.FilesProcessed),
			attribute.Int("sync.files_errored", result.FilesErrored),
			attribute.Float64("sync.duration_seconds", result.Duration.Seconds()),
		)
	}()

	// Get all document libraries
	for _, libraryName := range sp.config.LibraryNames {
		files, err := sp.getLibraryFiles(ctx, libraryName)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to get files from library %s: %v", libraryName, err))
			result.FilesErrored++
			continue
		}

		result.FilesFound += len(files)

		for _, file := range files {
			if sp.shouldProcessFile(file) {
				if err := sp.processFile(ctx, file); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("Failed to process file %s: %v", file.Name, err))
					result.FilesErrored++
				} else {
					result.FilesProcessed++
				}
			} else {
				result.FilesSkipped++
			}
		}
	}
}

// deltaSyncLoop performs periodic delta syncs
func (sp *SharePointConnector) deltaSyncLoop(ctx context.Context) {
	ticker := time.NewTicker(sp.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sp.performDeltaSync(ctx)
		}
	}
}

// performDeltaSync performs a delta sync for changes
func (sp *SharePointConnector) performDeltaSync(ctx context.Context) {
	ctx, span := sp.tracer.Start(ctx, "sharepoint_delta_sync")
	defer span.End()

	// Implementation would use SharePoint delta query API
	// to get only changed files since last sync
	// For now, this is a placeholder
}

// getLibraryFiles retrieves all files from a document library
func (sp *SharePointConnector) getLibraryFiles(ctx context.Context, libraryName string) ([]*SharePointFile, error) {
	ctx, span := sp.tracer.Start(ctx, "get_library_files")
	defer span.End()

	span.SetAttributes(attribute.String("library.name", libraryName))

	// Check token expiry
	if time.Now().After(sp.tokenExpiry.Add(-5 * time.Minute)) {
		if err := sp.authenticate(ctx); err != nil {
			return nil, err
		}
	}

	// Build Graph API URL
	graphURL := fmt.Sprintf("https://graph.microsoft.com/v1.0/sites/%s:%s:/lists/%s/items",
		sp.extractHostFromURL(sp.config.BaseURL),
		sp.config.SiteURL,
		libraryName,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", graphURL, nil)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+sp.accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := sp.client.Do(req)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("failed to get library files: %s", string(body))
		span.RecordError(err)
		return nil, err
	}

	var response struct {
		Value []SharePointFile `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		span.RecordError(err)
		return nil, err
	}

	files := make([]*SharePointFile, len(response.Value))
	for i := range response.Value {
		files[i] = &response.Value[i]
	}

	span.SetAttributes(attribute.Int("files.count", len(files)))
	return files, nil
}

// shouldProcessFile determines if a file should be processed
func (sp *SharePointConnector) shouldProcessFile(file *SharePointFile) bool {
	// Check file extension
	if len(sp.config.FileExtensions) > 0 {
		fileExt := strings.ToLower(getFileExtension(file.Name))
		allowed := false
		for _, ext := range sp.config.FileExtensions {
			if strings.ToLower(ext) == fileExt {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	// Check file size
	maxSizeBytes := int64(sp.config.MaxFileSizeMB) * 1024 * 1024
	if file.Size > maxSizeBytes {
		return false
	}

	// Check exclude patterns
	for _, pattern := range sp.config.ExcludePatterns {
		if strings.Contains(strings.ToLower(file.Name), strings.ToLower(pattern)) {
			return false
		}
	}

	return true
}

// processFile processes a SharePoint file and emits a discovery event
func (sp *SharePointConnector) processFile(ctx context.Context, file *SharePointFile) error {
	ctx, span := sp.tracer.Start(ctx, "process_sharepoint_file")
	defer span.End()

	span.SetAttributes(
		attribute.String("file.id", file.ID),
		attribute.String("file.name", file.Name),
		attribute.Int64("file.size", file.Size),
	)

	// Create file URL
	fileURL := sp.config.BaseURL + file.ServerRelativeURL

	// Prepare metadata
	metadata := map[string]string{
		"source":              "sharepoint",
		"site_url":            sp.config.SiteURL,
		"file_id":             file.ID,
		"created_by":          file.CreatedBy.DisplayName,
		"modified_by":         file.ModifiedBy.DisplayName,
		"server_relative_url": file.ServerRelativeURL,
	}

	// Add custom metadata if enabled
	if sp.config.EnableMetadata && file.Metadata != nil {
		for k, v := range file.Metadata {
			metadata[fmt.Sprintf("sp_%s", k)] = fmt.Sprintf("%v", v)
		}
	}

	// Create file discovered event
	discoveryEvent := events.NewFileDiscoveredEvent("sharepoint-connector", sp.config.TenantID, events.FileDiscoveredData{
		URL:          fileURL,
		SourceID:     fmt.Sprintf("sharepoint-%s", sp.extractHostFromURL(sp.config.BaseURL)),
		DiscoveredAt: time.Now(),
		Size:         file.Size,
		ContentType:  file.ContentType,
		Metadata:     metadata,
		Priority:     "normal",
	})

	// Publish event
	return sp.producer.PublishEvent(ctx, discoveryEvent)
}

// setupWebhooks sets up SharePoint webhooks for real-time notifications
func (sp *SharePointConnector) setupWebhooks(ctx context.Context) {
	ctx, span := sp.tracer.Start(ctx, "setup_sharepoint_webhooks")
	defer span.End()

	// Implementation would create webhook subscriptions
	// for each document library to receive real-time change notifications
	// For now, this is a placeholder
}

// HandleWebhook handles incoming webhook notifications from SharePoint
func (sp *SharePointConnector) HandleWebhook(ctx context.Context, event *SharePointEvent) error {
	ctx, span := sp.tracer.Start(ctx, "handle_sharepoint_webhook")
	defer span.End()

	span.SetAttributes(
		attribute.String("event.type", event.EventType),
		attribute.String("event.item_id", event.ItemID),
		attribute.String("event.user", event.UserDisplayName),
	)

	switch event.EventType {
	case "ItemAdded", "ItemUpdated":
		// Get file details and process
		file, err := sp.getFileByID(ctx, event.ItemID)
		if err != nil {
			span.RecordError(err)
			return err
		}

		if sp.shouldProcessFile(file) {
			return sp.processFile(ctx, file)
		}

	case "ItemDeleted":
		// Handle file deletion
		// Could emit a file deleted event
	}

	return nil
}

// getFileByID retrieves a file by its ID
func (sp *SharePointConnector) getFileByID(ctx context.Context, fileID string) (*SharePointFile, error) {
	// Implementation would retrieve file details from SharePoint by ID
	// For now, return a placeholder
	return &SharePointFile{
		ID:   fileID,
		Name: "placeholder.docx",
		Size: 1024,
	}, nil
}

// extractHostFromURL extracts the host portion from a URL
func (sp *SharePointConnector) extractHostFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host
}

// getFileExtension extracts the file extension from a filename
func getFileExtension(filename string) string {
	parts := strings.Split(filename, ".")
	if len(parts) > 1 {
		return "." + parts[len(parts)-1]
	}
	return ""
}

// GetSyncStatus returns the current sync status
func (sp *SharePointConnector) GetSyncStatus() map[string]interface{} {
	return map[string]interface{}{
		"authenticated":    sp.accessToken != "",
		"token_expires_at": sp.tokenExpiry,
		"last_sync":        time.Now(), // Placeholder
		"files_synced":     1000,       // Placeholder
		"sync_errors":      0,          // Placeholder
	}
}

// GetMetrics returns SharePoint connector metrics
func (sp *SharePointConnector) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"sync_interval":        sp.config.SyncInterval.String(),
		"max_file_size_mb":     sp.config.MaxFileSizeMB,
		"supported_extensions": len(sp.config.FileExtensions),
		"libraries_monitored":  len(sp.config.LibraryNames),
		"webhooks_enabled":     sp.config.WebhooksEnabled,
		"delta_sync_enabled":   sp.config.DeltaSyncEnabled,
	}
}

// HealthCheck performs a health check on the SharePoint connector
func (sp *SharePointConnector) HealthCheck(ctx context.Context) error {
	// Check authentication
	if sp.accessToken == "" {
		return fmt.Errorf("not authenticated")
	}

	if time.Now().After(sp.tokenExpiry) {
		return fmt.Errorf("access token expired")
	}

	// Test connectivity by making a simple API call
	testURL := "https://graph.microsoft.com/v1.0/me"
	req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+sp.accessToken)

	resp, err := sp.client.Do(req)
	if err != nil {
		return fmt.Errorf("connectivity check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API health check failed with status: %d", resp.StatusCode)
	}

	return nil
}
