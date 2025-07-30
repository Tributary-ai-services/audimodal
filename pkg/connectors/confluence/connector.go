package confluence

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// ConfluenceConnector implements storage.StorageConnector for Atlassian Confluence
type ConfluenceConnector struct {
	config       *ConfluenceConfig
	httpClient   *http.Client
	tracer       trace.Tracer
	
	// Connection state
	isConnected  bool
	lastSync     time.Time
	baseURL      string
	
	// Authentication
	authMethod   string // "basic", "oauth2", "pat"
	credentials  map[string]string
	
	// Rate limiting and throttling
	rateLimiter  *RateLimiter
	
	// Caching
	contentCache map[string]*Content
	spaceCache   map[string]*Space
	cacheMu      sync.RWMutex
	
	// Sync state
	syncState    *SyncState
	syncMu       sync.RWMutex
	
	// Metrics
	metrics      *ConnectorMetrics
	
	// Error handling
	retryPolicy  *RetryPolicy
	
	// Confluence-specific
	includedSpaces []string
	excludedSpaces []string
}

// ConfluenceConfig contains configuration for Confluence connector
type ConfluenceConfig struct {
	// Connection configuration
	BaseURL            string   `yaml:"base_url"`
	CloudInstance      bool     `yaml:"cloud_instance"`
	
	// Authentication configuration
	AuthMethod         string   `yaml:"auth_method"` // "basic", "oauth2", "pat"
	Username           string   `yaml:"username"`
	Password           string   `yaml:"password"`
	APIToken           string   `yaml:"api_token"`
	PersonalAccessToken string  `yaml:"personal_access_token"`
	
	// OAuth2 configuration (for OAuth2 auth method)
	ClientID           string   `yaml:"client_id"`
	ClientSecret       string   `yaml:"client_secret"`
	RedirectURL        string   `yaml:"redirect_url"`
	
	// Sync configuration
	SyncInterval       time.Duration `yaml:"sync_interval"`
	FullSyncInterval   time.Duration `yaml:"full_sync_interval"`
	BatchSize          int           `yaml:"batch_size"`
	MaxConcurrentReqs  int           `yaml:"max_concurrent_requests"`
	
	// Content filtering
	IncludeSpaces      []string      `yaml:"include_spaces"`
	ExcludeSpaces      []string      `yaml:"exclude_spaces"`
	IncludeContentTypes []string     `yaml:"include_content_types"`
	ExcludeContentTypes []string     `yaml:"exclude_content_types"`
	IncludeStatuses    []string      `yaml:"include_statuses"`
	ExcludeStatuses    []string      `yaml:"exclude_statuses"`
	MaxContentSize     int64         `yaml:"max_content_size"`
	SyncAttachments    bool          `yaml:"sync_attachments"`
	SyncComments       bool          `yaml:"sync_comments"`
	SyncDrafts         bool          `yaml:"sync_drafts"`
	SyncArchived       bool          `yaml:"sync_archived"`
	
	// Rate limiting
	RequestsPerSecond  float64       `yaml:"requests_per_second"`
	BurstLimit         int           `yaml:"burst_limit"`
	
	// Retry configuration
	MaxRetries         int           `yaml:"max_retries"`
	RetryDelay         time.Duration `yaml:"retry_delay"`
	ExponentialBackoff bool          `yaml:"exponential_backoff"`
	
	// Cache configuration
	CacheEnabled       bool          `yaml:"cache_enabled"`
	CacheTTL           time.Duration `yaml:"cache_ttl"`
	CacheSize          int           `yaml:"cache_size"`
	
	// Confluence-specific features
	ExpandContent      []string      `yaml:"expand_content"`
	IncludeMetadata    bool          `yaml:"include_metadata"`
	IncludeVersions    bool          `yaml:"include_versions"`
	IncludeRestrictions bool         `yaml:"include_restrictions"`
	UseStorage         bool          `yaml:"use_storage"` // Use storage format instead of view
	
	// Export configuration
	ExportFormat       string        `yaml:"export_format"` // "view", "storage", "export_view"
	IncludeLabels      bool          `yaml:"include_labels"`
	IncludeBreadcrumbs bool          `yaml:"include_breadcrumbs"`
}

// DefaultConfluenceConfig returns default configuration
func DefaultConfluenceConfig() *ConfluenceConfig {
	return &ConfluenceConfig{
		CloudInstance:      true,
		AuthMethod:         "basic",
		SyncInterval:       15 * time.Minute,
		FullSyncInterval:   24 * time.Hour,
		BatchSize:          50, // Confluence typically supports smaller batches
		MaxConcurrentReqs:  5,
		IncludeContentTypes: []string{"page", "blogpost"},
		IncludeStatuses:    []string{"current"},
		MaxContentSize:     50 * 1024 * 1024, // 50MB
		SyncAttachments:    true,
		SyncComments:       false,
		SyncDrafts:         false,
		SyncArchived:       false,
		RequestsPerSecond:  5.0, // Conservative rate limit for Confluence
		BurstLimit:         20,
		MaxRetries:         3,
		RetryDelay:         2 * time.Second,
		ExponentialBackoff: true,
		CacheEnabled:       true,
		CacheTTL:           1 * time.Hour,
		CacheSize:          5000,
		ExpandContent:      []string{"body.storage", "version", "space"},
		IncludeMetadata:    true,
		IncludeVersions:    false,
		IncludeRestrictions: false,
		UseStorage:         true,
		ExportFormat:       "storage",
		IncludeLabels:      true,
		IncludeBreadcrumbs: true,
	}
}

// NewConfluenceConnector creates a new Confluence connector
func NewConfluenceConnector(config *ConfluenceConfig) *ConfluenceConnector {
	if config == nil {
		config = DefaultConfluenceConfig()
	}

	connector := &ConfluenceConnector{
		config:        config,
		httpClient:    &http.Client{Timeout: 60 * time.Second},
		tracer:        otel.Tracer("confluence-connector"),
		baseURL:       strings.TrimSuffix(config.BaseURL, "/"),
		authMethod:    config.AuthMethod,
		contentCache:  make(map[string]*Content),
		spaceCache:    make(map[string]*Space),
		syncState:     &SyncState{},
		metrics:       &ConnectorMetrics{},
		rateLimiter:   NewRateLimiter(config.RequestsPerSecond, config.BurstLimit),
		retryPolicy: &RetryPolicy{
			MaxRetries:         config.MaxRetries,
			InitialDelay:       config.RetryDelay,
			ExponentialBackoff: config.ExponentialBackoff,
		},
		includedSpaces: config.IncludeSpaces,
		excludedSpaces: config.ExcludeSpaces,
	}

	// Setup credentials based on auth method
	connector.credentials = make(map[string]string)
	switch config.AuthMethod {
	case "basic":
		if config.APIToken != "" {
			connector.credentials["username"] = config.Username
			connector.credentials["password"] = config.APIToken
		} else {
			connector.credentials["username"] = config.Username
			connector.credentials["password"] = config.Password
		}
	case "pat":
		connector.credentials["token"] = config.PersonalAccessToken
	case "oauth2":
		// OAuth2 setup would be handled separately
	}

	return connector
}

// Connect establishes connection to Confluence
func (c *ConfluenceConnector) Connect(ctx context.Context, credentials map[string]interface{}) error {
	ctx, span := c.tracer.Start(ctx, "confluence_connect")
	defer span.End()

	// Override credentials if provided
	if len(credentials) > 0 {
		c.credentials = make(map[string]string)
		for k, v := range credentials {
			if str, ok := v.(string); ok {
				c.credentials[k] = str
			}
		}
	}

	// Test connection
	if err := c.testConnection(ctx); err != nil {
		span.RecordError(err)
		c.isConnected = false
		return fmt.Errorf("connection test failed: %w", err)
	}

	c.isConnected = true

	span.SetAttributes(
		attribute.Bool("connected", c.isConnected),
		attribute.String("base_url", c.baseURL),
		attribute.String("auth_method", c.authMethod),
	)

	return nil
}

// Disconnect closes the connection to Confluence
func (c *ConfluenceConnector) Disconnect(ctx context.Context) error {
	ctx, span := c.tracer.Start(ctx, "confluence_disconnect")
	defer span.End()

	c.isConnected = false
	
	// Clear caches
	c.cacheMu.Lock()
	c.contentCache = make(map[string]*Content)
	c.spaceCache = make(map[string]*Space)
	c.cacheMu.Unlock()

	return nil
}

// IsConnected returns whether the connector is connected
func (c *ConfluenceConnector) IsConnected() bool {
	return c.isConnected
}

// ListFiles lists content from Confluence (pages, blog posts, attachments)
func (c *ConfluenceConnector) ListFiles(ctx context.Context, path string, options *storage.ConnectorListOptions) ([]*storage.ConnectorFileInfo, error) {
	ctx, span := c.tracer.Start(ctx, "confluence_list_files")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to Confluence")
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(
		attribute.String("path", path),
	)

	// If path is empty, list all spaces first, then content
	if path == "" || path == "/" {
		return c.listAllContent(ctx)
	} else {
		// List content in specific space
		return c.listSpaceContent(ctx, path)
	}
}

// GetFile retrieves a specific content item from Confluence
func (c *ConfluenceConnector) GetFile(ctx context.Context, fileID string) (*storage.ConnectorFileInfo, error) {
	ctx, span := c.tracer.Start(ctx, "confluence_get_file")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to Confluence")
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(
		attribute.String("file.id", fileID),
	)

	// Check cache first
	if c.config.CacheEnabled {
		if cachedContent := c.getCachedContent(fileID); cachedContent != nil {
			return c.convertToFileInfo(cachedContent), nil
		}
	}

	// Wait for rate limit
	if err := c.rateLimiter.Wait(ctx); err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Build API URL
	apiURL := fmt.Sprintf("%s/rest/api/content/%s", c.baseURL, fileID)
	
	// Add expand parameters
	if len(c.config.ExpandContent) > 0 {
		params := url.Values{}
		params.Set("expand", strings.Join(c.config.ExpandContent, ","))
		apiURL += "?" + params.Encode()
	}

	// Execute API call with retry
	response, err := c.executeConfluenceAPICall(ctx, "GET", apiURL, nil)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get content: %w", err)
	}

	var content Content
	if err := json.Unmarshal(response, &content); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}
	
	// Cache content
	if c.config.CacheEnabled {
		c.cacheContent(&content)
	}

	// Update metrics
	c.metrics.ContentRetrieved++

	return c.convertToFileInfo(&content), nil
}

// DownloadFile downloads content from Confluence (exports as PDF or other formats)
func (c *ConfluenceConnector) DownloadFile(ctx context.Context, fileID string) (io.ReadCloser, error) {
	ctx, span := c.tracer.Start(ctx, "confluence_download_file")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to Confluence")
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(
		attribute.String("file.id", fileID),
	)

	// Wait for rate limit
	if err := c.rateLimiter.Wait(ctx); err != nil {
		span.RecordError(err)
		return nil, err
	}

	// For Confluence, we'll export the content as PDF
	// First get the content to check if it's an attachment
	content, err := c.GetFile(ctx, fileID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	var downloadURL string
	
	if content.Type == "attachment" {
		// For attachments, use the download URL
		downloadURL = fmt.Sprintf("%s/rest/api/content/%s/download", c.baseURL, fileID)
	} else {
		// For pages and blog posts, export as PDF
		downloadURL = fmt.Sprintf("%s/spaces/flyingpdf/pdfpageexport.action?pageId=%s", c.baseURL, fileID)
	}

	// Execute download with retry
	response, err := c.executeWithRetry(ctx, func() (interface{}, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
		if err != nil {
			return nil, err
		}
		
		// Add authentication
		c.addAuthHeaders(req)
		
		return c.httpClient.Do(req)
	})

	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to download content: %w", err)
	}

	resp := response.(*http.Response)

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		err := fmt.Errorf("download failed with status: %d", resp.StatusCode)
		span.RecordError(err)
		return nil, err
	}

	// Update metrics
	c.metrics.ContentDownloaded++
	c.metrics.BytesDownloaded += resp.ContentLength

	return resp.Body, nil
}

// SyncFiles performs synchronization with Confluence
func (c *ConfluenceConnector) SyncFiles(ctx context.Context, options *storage.SyncOptions) (*storage.SyncResult, error) {
	ctx, span := c.tracer.Start(ctx, "confluence_sync_files")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to Confluence")
		span.RecordError(err)
		return nil, err
	}

	c.syncMu.Lock()
	defer c.syncMu.Unlock()

	syncStart := time.Now()
	c.syncState.IsRunning = true
	c.syncState.LastSyncStart = syncStart

	defer func() {
		c.syncState.IsRunning = false
		c.syncState.LastSyncEnd = time.Now()
		c.syncState.LastSyncDuration = time.Since(syncStart)
	}()

	// Determine sync type
	isFullSync := options.FullSync || time.Since(c.lastSync) > c.config.FullSyncInterval

	span.SetAttributes(
		attribute.Bool("full_sync", isFullSync),
		attribute.String("since", options.Since.Format(time.RFC3339)),
	)

	result := &storage.SyncResult{
		StartTime:    syncStart,
		SyncType:     "incremental",
		FilesFound:   0,
		FilesChanged: 0,
		FilesDeleted: 0,
		Errors:       []string{},
	}

	if isFullSync {
		result.SyncType = "full"
	}

	// Execute sync
	if err := c.executeSyncOperation(ctx, options, result, isFullSync); err != nil {
		span.RecordError(err)
		result.Errors = append(result.Errors, err.Error())
		return result, err
	}

	// Update sync state
	c.lastSync = syncStart
	c.syncState.TotalContent = result.FilesFound
	c.syncState.ChangedContent = result.FilesChanged

	// Update metrics
	c.metrics.SyncCount++
	c.metrics.LastSyncTime = syncStart
	c.metrics.LastSyncDuration = result.EndTime.Sub(result.StartTime)

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	span.SetAttributes(
		attribute.Int64("content.found", result.FilesFound),
		attribute.Int64("content.changed", result.FilesChanged),
		attribute.Float64("duration.seconds", result.Duration.Seconds()),
	)

	return result, nil
}

// GetSyncState returns the current synchronization state
func (c *ConfluenceConnector) GetSyncState(ctx context.Context) (*storage.SyncState, error) {
	c.syncMu.RLock()
	defer c.syncMu.RUnlock()

	return &storage.SyncState{
		IsRunning:        c.syncState.IsRunning,
		LastSyncStart:    c.syncState.LastSyncStart,
		LastSyncEnd:      c.syncState.LastSyncEnd,
		LastSyncDuration: c.syncState.LastSyncDuration,
		TotalFiles:       c.syncState.TotalContent,
		ChangedFiles:     c.syncState.ChangedContent,
		ErrorCount:       c.syncState.ErrorCount,
		LastError:        c.syncState.LastError,
	}, nil
}

// GetMetrics returns connector metrics
func (c *ConfluenceConnector) GetMetrics(ctx context.Context) (*storage.ConnectorMetrics, error) {
	return &storage.ConnectorMetrics{
		ConnectorType:      "confluence",
		IsConnected:        c.isConnected,
		LastConnectionTime: c.metrics.LastConnectionTime,
		FilesListed:        c.metrics.ContentListed,
		FilesRetrieved:     c.metrics.ContentRetrieved,
		FilesDownloaded:    c.metrics.ContentDownloaded,
		BytesDownloaded:    c.metrics.BytesDownloaded,
		SyncCount:          c.metrics.SyncCount,
		LastSyncTime:       c.metrics.LastSyncTime,
		LastSyncDuration:   c.metrics.LastSyncDuration,
		ErrorCount:         c.metrics.ErrorCount,
		LastError:          c.metrics.LastError,
	}, nil
}

// Helper methods

func (c *ConfluenceConnector) testConnection(ctx context.Context) error {
	// Test connection by getting server info or user info
	var testURL string
	if c.config.CloudInstance {
		testURL = fmt.Sprintf("%s/rest/api/user/current", c.baseURL)
	} else {
		testURL = fmt.Sprintf("%s/rest/api/user/current", c.baseURL)
	}

	response, err := c.executeConfluenceAPICall(ctx, "GET", testURL, nil)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}

	// Try to parse the response to ensure it's valid
	var user User
	if err := json.Unmarshal(response, &user); err != nil {
		return fmt.Errorf("failed to parse user info: %w", err)
	}
	
	c.metrics.LastConnectionTime = time.Now()
	return nil
}

func (c *ConfluenceConnector) listAllContent(ctx context.Context) ([]*storage.ConnectorFileInfo, error) {
	var files []*storage.ConnectorFileInfo
	
	// First get all spaces
	spaces, err := c.listSpaces(ctx)
	if err != nil {
		return nil, err
	}

	// Then get content from each space
	for _, space := range spaces {
		if c.shouldIncludeSpace(space.Key) {
			spaceContent, err := c.listSpaceContent(ctx, space.Key)
			if err != nil {
				// Log error but continue with other spaces
				continue
			}
			files = append(files, spaceContent...)
		}
	}

	return files, nil
}

func (c *ConfluenceConnector) listSpaces(ctx context.Context) ([]Space, error) {
	var allSpaces []Space
	start := 0
	limit := c.config.BatchSize

	for {
		// Wait for rate limit
		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, err
		}

		// Build API URL
		apiURL := fmt.Sprintf("%s/rest/api/space", c.baseURL)
		params := url.Values{}
		params.Set("start", fmt.Sprintf("%d", start))
		params.Set("limit", fmt.Sprintf("%d", limit))
		params.Set("expand", "description.plain,homepage")
		apiURL += "?" + params.Encode()

		// Execute API call
		response, err := c.executeConfluenceAPICall(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list spaces: %w", err)
		}

		var spaceCollection SpaceCollection
		if err := json.Unmarshal(response, &spaceCollection); err != nil {
			return nil, fmt.Errorf("failed to parse spaces response: %w", err)
		}

		allSpaces = append(allSpaces, spaceCollection.Results...)

		// Check if we have more results
		if len(spaceCollection.Results) < limit {
			break
		}
		start += limit
	}

	return allSpaces, nil
}

func (c *ConfluenceConnector) listSpaceContent(ctx context.Context, spaceKey string) ([]*storage.ConnectorFileInfo, error) {
	var files []*storage.ConnectorFileInfo
	start := 0
	limit := c.config.BatchSize

	for {
		// Wait for rate limit
		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, err
		}

		// Build API URL for content in space
		apiURL := fmt.Sprintf("%s/rest/api/content", c.baseURL)
		params := url.Values{}
		params.Set("spaceKey", spaceKey)
		params.Set("start", fmt.Sprintf("%d", start))
		params.Set("limit", fmt.Sprintf("%d", limit))
		
		if len(c.config.ExpandContent) > 0 {
			params.Set("expand", strings.Join(c.config.ExpandContent, ","))
		}
		
		// Filter by content types
		if len(c.config.IncludeContentTypes) > 0 {
			params.Set("type", strings.Join(c.config.IncludeContentTypes, ","))
		}
		
		// Filter by status
		if len(c.config.IncludeStatuses) > 0 {
			params.Set("status", strings.Join(c.config.IncludeStatuses, ","))
		}
		
		apiURL += "?" + params.Encode()

		// Execute API call
		response, err := c.executeConfluenceAPICall(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list content in space %s: %w", spaceKey, err)
		}

		var contentCollection ContentCollection
		if err := json.Unmarshal(response, &contentCollection); err != nil {
			return nil, fmt.Errorf("failed to parse content response: %w", err)
		}

		// Convert content to file info
		for _, content := range contentCollection.Results {
			if c.shouldIncludeContent(&content) {
				fileInfo := c.convertToFileInfo(&content)
				files = append(files, fileInfo)
				
				// Cache content
				if c.config.CacheEnabled {
					c.cacheContent(&content)
				}
				
				// Get attachments if enabled
				if c.config.SyncAttachments {
					attachments, err := c.listContentAttachments(ctx, content.ID)
					if err == nil {
						files = append(files, attachments...)
					}
				}
			}
		}

		// Check if we have more results
		if len(contentCollection.Results) < limit {
			break
		}
		start += limit
	}

	return files, nil
}

func (c *ConfluenceConnector) listContentAttachments(ctx context.Context, contentID string) ([]*storage.ConnectorFileInfo, error) {
	// Wait for rate limit
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, err
	}

	// Build API URL for attachments
	apiURL := fmt.Sprintf("%s/rest/api/content/%s/child/attachment", c.baseURL, contentID)
	params := url.Values{}
	params.Set("expand", "version,container,metadata.mediaType")
	apiURL += "?" + params.Encode()

	// Execute API call
	response, err := c.executeConfluenceAPICall(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list attachments for content %s: %w", contentID, err)
	}

	var contentCollection ContentCollection
	if err := json.Unmarshal(response, &contentCollection); err != nil {
		return nil, fmt.Errorf("failed to parse attachments response: %w", err)
	}

	var files []*storage.ConnectorFileInfo
	for _, attachment := range contentCollection.Results {
		fileInfo := c.convertToFileInfo(&attachment)
		files = append(files, fileInfo)
		
		// Cache attachment
		if c.config.CacheEnabled {
			c.cacheContent(&attachment)
		}
	}

	c.metrics.AttachmentsProcessed += int64(len(files))
	return files, nil
}

func (c *ConfluenceConnector) shouldIncludeSpace(spaceKey string) bool {
	// Check excluded spaces first
	for _, excluded := range c.excludedSpaces {
		if excluded == spaceKey {
			return false
		}
	}

	// If include list is specified, check if space is in it
	if len(c.includedSpaces) > 0 {
		for _, included := range c.includedSpaces {
			if included == spaceKey {
				return true
			}
		}
		return false
	}

	return true
}

func (c *ConfluenceConnector) shouldIncludeContent(content *Content) bool {
	// Check content type
	if len(c.config.IncludeContentTypes) > 0 {
		found := false
		for _, contentType := range c.config.IncludeContentTypes {
			if content.Type == contentType {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check excluded content types
	for _, excluded := range c.config.ExcludeContentTypes {
		if content.Type == excluded {
			return false
		}
	}

	// Check status
	if len(c.config.IncludeStatuses) > 0 {
		found := false
		for _, status := range c.config.IncludeStatuses {
			if content.Status == status {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check excluded statuses
	for _, excluded := range c.config.ExcludeStatuses {
		if content.Status == excluded {
			return false
		}
	}

	return true
}

func (c *ConfluenceConnector) convertToFileInfo(content *Content) *storage.ConnectorFileInfo {
	// Parse timestamps
	var createdTime, modifiedTime time.Time
	if content.History != nil && content.History.CreatedDate != "" {
		createdTime, _ = time.Parse(time.RFC3339, content.History.CreatedDate)
	}
	if content.Version != nil && content.Version.When != "" {
		modifiedTime, _ = time.Parse(time.RFC3339, content.Version.When)
	}

	// Determine file type
	var fileType storage.FileType
	switch content.Type {
	case "page":
		fileType = storage.FileTypeDocument
	case "blogpost":
		fileType = storage.FileTypeDocument
	case "comment":
		fileType = storage.FileTypeOther
	case "attachment":
		// Try to determine type from title/extension
		name := strings.ToLower(content.Title)
		if strings.Contains(name, ".pdf") {
			fileType = storage.FileTypePDF
		} else if strings.Contains(name, ".doc") || strings.Contains(name, ".docx") {
			fileType = storage.FileTypeDocument
		} else if strings.Contains(name, ".xls") || strings.Contains(name, ".xlsx") {
			fileType = storage.FileTypeSpreadsheet
		} else if strings.Contains(name, ".ppt") || strings.Contains(name, ".pptx") {
			fileType = storage.FileTypePresentation
		} else if strings.Contains(name, ".jpg") || strings.Contains(name, ".png") || strings.Contains(name, ".gif") {
			fileType = storage.FileTypeImage
		} else {
			fileType = storage.FileTypeOther
		}
	default:
		fileType = storage.FileTypeOther
	}

	// Build path
	path := "/" + content.Title
	if content.Space != nil {
		path = "/" + content.Space.Key + "/" + content.Title
	}

	// Build web URL
	webURL := ""
	if content.Links != nil && content.Links.Webui != "" {
		webURL = c.baseURL + content.Links.Webui
	}

	// Get content size (estimate based on body length)
	var size int64
	if content.Body != nil && content.Body.Storage != nil {
		size = int64(len(content.Body.Storage.Value))
	}

	// Create file info
	fileInfo := &storage.ConnectorFileInfo{
		ID:           content.ID,
		Name:         content.Title,
		Path:         path,
		URL:          webURL,
		DownloadURL:  "", // Will be set when downloading
		Size:         size,
		Type:         fileType,
		MimeType:     "", // Confluence doesn't provide MIME type for content
		CreatedAt:    createdTime,
		ModifiedAt:   modifiedTime,
		LastAccessed: modifiedTime,
		IsFolder:     false,
		Permissions: storage.FilePermissions{
			CanRead:   true,
			CanWrite:  false, // Assuming read-only for now
			CanDelete: false,
		},
		Metadata: map[string]interface{}{
			"confluence_id":   content.ID,
			"content_type":    content.Type,
			"status":          content.Status,
			"space_key":       "",
		},
		Tags: map[string]string{
			"source":       "confluence",
			"content_type": content.Type,
			"status":       content.Status,
		},
		Source: "confluence",
	}

	// Add space information
	if content.Space != nil {
		fileInfo.Metadata["space_key"] = content.Space.Key
		fileInfo.Metadata["space_name"] = content.Space.Name
		fileInfo.Tags["space"] = content.Space.Key
	}

	// Add version information
	if content.Version != nil {
		fileInfo.Metadata["version"] = content.Version.Number
		fileInfo.Metadata["version_message"] = content.Version.Message
	}

	return fileInfo
}

func (c *ConfluenceConnector) cacheContent(content *Content) {
	if !c.config.CacheEnabled {
		return
	}

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	c.contentCache[content.ID] = content
	
	// Simple cache size management
	if len(c.contentCache) > c.config.CacheSize {
		// Remove oldest entries (simple FIFO)
		count := 0
		for id := range c.contentCache {
			delete(c.contentCache, id)
			count++
			if count >= c.config.CacheSize/10 { // Remove 10% of cache
				break
			}
		}
	}
}

func (c *ConfluenceConnector) getCachedContent(contentID string) *Content {
	if !c.config.CacheEnabled {
		return nil
	}

	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()

	return c.contentCache[contentID]
}

func (c *ConfluenceConnector) executeSyncOperation(ctx context.Context, options *storage.SyncOptions, result *storage.SyncResult, isFullSync bool) error {
	if isFullSync {
		return c.executeFullSync(ctx, options, result)
	} else {
		return c.executeIncrementalSync(ctx, options, result)
	}
}

func (c *ConfluenceConnector) executeFullSync(ctx context.Context, options *storage.SyncOptions, result *storage.SyncResult) error {
	// List all content
	files, err := c.ListFiles(ctx, "", &storage.ConnectorListOptions{})
	if err != nil {
		return err
	}
	
	result.FilesFound = int64(len(files))
	result.FilesChanged = int64(len(files)) // For full sync, consider all content as changed
	
	return nil
}

func (c *ConfluenceConnector) executeIncrementalSync(ctx context.Context, options *storage.SyncOptions, result *storage.SyncResult) error {
	// For incremental sync, we would use CQL queries to find content modified since last sync
	// This is a simplified implementation
	return c.executeFullSync(ctx, options, result)
}

func (c *ConfluenceConnector) executeConfluenceAPICall(ctx context.Context, method, url string, body []byte) ([]byte, error) {
	return c.executeWithRetryBytes(ctx, func() ([]byte, error) {
		var reqBody io.Reader
		if body != nil {
			reqBody = bytes.NewReader(body)
		}

		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return nil, err
		}
		
		// Add authentication headers
		c.addAuthHeaders(req)
		
		// Add content type header
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("API request failed with status: %d", resp.StatusCode)
		}
		
		return io.ReadAll(resp.Body)
	})
}

func (c *ConfluenceConnector) addAuthHeaders(req *http.Request) {
	switch c.authMethod {
	case "basic":
		if username, ok := c.credentials["username"]; ok {
			if password, ok := c.credentials["password"]; ok {
				req.SetBasicAuth(username, password)
			}
		}
	case "pat":
		if token, ok := c.credentials["token"]; ok {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	case "oauth2":
		// OAuth2 token would be handled here
		if token, ok := c.credentials["access_token"]; ok {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
}

func (c *ConfluenceConnector) executeWithRetry(ctx context.Context, operation func() (interface{}, error)) (interface{}, error) {
	var lastErr error
	
	for attempt := 0; attempt <= c.retryPolicy.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate delay
			delay := c.retryPolicy.InitialDelay
			if c.retryPolicy.ExponentialBackoff {
				delay = delay * time.Duration(1<<uint(attempt-1))
			}
			
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		result, err := operation()
		if err == nil {
			return result, nil
		}

		lastErr = err
		
		// Check if error is retryable
		if !c.isRetryableError(err) {
			break
		}
	}

	c.metrics.ErrorCount++
	c.metrics.LastError = lastErr.Error()
	
	return nil, lastErr
}

func (c *ConfluenceConnector) executeWithRetryBytes(ctx context.Context, operation func() ([]byte, error)) ([]byte, error) {
	var lastErr error
	
	for attempt := 0; attempt <= c.retryPolicy.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate delay
			delay := c.retryPolicy.InitialDelay
			if c.retryPolicy.ExponentialBackoff {
				delay = delay * time.Duration(1<<uint(attempt-1))
			}
			
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		result, err := operation()
		if err == nil {
			return result, nil
		}

		lastErr = err
		
		// Check if error is retryable
		if !c.isRetryableError(err) {
			break
		}
	}

	c.metrics.ErrorCount++
	c.metrics.LastError = lastErr.Error()
	
	return nil, lastErr
}

func (c *ConfluenceConnector) isRetryableError(err error) bool {
	errStr := strings.ToLower(err.Error())
	retryableErrors := []string{
		"rate limit",
		"too many requests",
		"internal server error",
		"service unavailable",
		"timeout",
		"connection",
		"network",
		"502", "503", "504", "429", // HTTP status codes
	}

	for _, retryable := range retryableErrors {
		if strings.Contains(errStr, retryable) {
			return true
		}
	}

	return false
}