package onedrive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// OneDriveConnector implements storage.StorageConnector for Microsoft OneDrive
type OneDriveConnector struct {
	config      *OneDriveConfig
	oauthConfig *oauth2.Config
	httpClient  *http.Client
	tracer      trace.Tracer

	// Connection state
	isConnected bool
	lastSync    time.Time
	accessToken string

	// Rate limiting and throttling
	rateLimiter *RateLimiter

	// Caching
	fileCache   map[string]*DriveItem
	folderCache map[string]*DriveItem
	cacheMu     sync.RWMutex

	// Sync state
	syncState *SyncState
	syncMu    sync.RWMutex

	// Metrics
	metrics *ConnectorMetrics

	// Error handling
	retryPolicy *RetryPolicy

	// OneDrive-specific
	driveID    string // Current drive ID
	deltaToken string // For delta/change tracking operations
}

// OneDriveConfig contains configuration for OneDrive connector
type OneDriveConfig struct {
	// OAuth2 configuration
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURL  string `yaml:"redirect_url"`
	TenantID     string `yaml:"tenant_id"`

	// Sync configuration
	SyncInterval      time.Duration `yaml:"sync_interval"`
	FullSyncInterval  time.Duration `yaml:"full_sync_interval"`
	BatchSize         int           `yaml:"batch_size"`
	MaxConcurrentReqs int           `yaml:"max_concurrent_requests"`

	// Filter configuration
	IncludeFolders   []string `yaml:"include_folders"`
	ExcludeFolders   []string `yaml:"exclude_folders"`
	FileExtensions   []string `yaml:"file_extensions"`
	MaxFileSize      int64    `yaml:"max_file_size"`
	SyncSharedFiles  bool     `yaml:"sync_shared_files"`
	SyncDeletedFiles bool     `yaml:"sync_deleted_files"`

	// Rate limiting
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	BurstLimit        int     `yaml:"burst_limit"`

	// Retry configuration
	MaxRetries         int           `yaml:"max_retries"`
	RetryDelay         time.Duration `yaml:"retry_delay"`
	ExponentialBackoff bool          `yaml:"exponential_backoff"`

	// Cache configuration
	CacheEnabled bool          `yaml:"cache_enabled"`
	CacheTTL     time.Duration `yaml:"cache_ttl"`
	CacheSize    int           `yaml:"cache_size"`

	// OneDrive-specific features
	UsePersonalOneDrive bool `yaml:"use_personal_onedrive"`
	UseBusinessOneDrive bool `yaml:"use_business_onedrive"`
	IncludeMetadata     bool `yaml:"include_metadata"`
	IncludeVersions     bool `yaml:"include_versions"`
	RecursiveSync       bool `yaml:"recursive_sync"`

	// SharePoint integration
	SharePointSiteURL string `yaml:"sharepoint_site_url"`
	SharePointListID  string `yaml:"sharepoint_list_id"`

	// Office 365 integration
	EnableOfficeIntegration bool     `yaml:"enable_office_integration"`
	OfficeFileFormats       []string `yaml:"office_file_formats"`
}

// DefaultOneDriveConfig returns default configuration
func DefaultOneDriveConfig() *OneDriveConfig {
	return &OneDriveConfig{
		SyncInterval:            15 * time.Minute,
		FullSyncInterval:        24 * time.Hour,
		BatchSize:               200, // Microsoft Graph supports up to 999 items per request
		MaxConcurrentReqs:       10,
		FileExtensions:          []string{".pdf", ".doc", ".docx", ".txt", ".xlsx", ".pptx"},
		MaxFileSize:             4 * 1024 * 1024 * 1024, // 4GB (OneDrive file size limit)
		SyncSharedFiles:         true,
		SyncDeletedFiles:        false,
		RequestsPerSecond:       10.0, // Microsoft Graph throttling limits
		BurstLimit:              50,
		MaxRetries:              3,
		RetryDelay:              2 * time.Second,
		ExponentialBackoff:      true,
		CacheEnabled:            true,
		CacheTTL:                1 * time.Hour,
		CacheSize:               10000,
		UsePersonalOneDrive:     true,
		UseBusinessOneDrive:     true,
		IncludeMetadata:         true,
		IncludeVersions:         false,
		RecursiveSync:           true,
		EnableOfficeIntegration: true,
		OfficeFileFormats:       []string{".docx", ".xlsx", ".pptx", ".doc", ".xls", ".ppt"},
	}
}

// NewOneDriveConnector creates a new OneDrive connector
func NewOneDriveConnector(config *OneDriveConfig) *OneDriveConnector {
	if config == nil {
		config = DefaultOneDriveConfig()
	}

	// Setup OAuth2 configuration for Microsoft Graph
	var endpoint oauth2.Endpoint
	if config.TenantID != "" {
		// Use tenant-specific endpoints for Azure AD
		endpoint = oauth2.Endpoint{
			AuthURL:  fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize", config.TenantID),
			TokenURL: fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", config.TenantID),
		}
	} else {
		// Use common endpoint for personal accounts
		endpoint = oauth2.Endpoint{
			AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		}
	}

	oauthConfig := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,
		Scopes:       []string{"https://graph.microsoft.com/Files.ReadWrite.All", "https://graph.microsoft.com/Sites.ReadWrite.All"},
		Endpoint:     endpoint,
	}

	connector := &OneDriveConnector{
		config:      config,
		oauthConfig: oauthConfig,
		httpClient:  &http.Client{Timeout: 60 * time.Second}, // Longer timeout for large files
		tracer:      otel.Tracer("onedrive-connector"),
		fileCache:   make(map[string]*DriveItem),
		folderCache: make(map[string]*DriveItem),
		syncState:   &SyncState{},
		metrics:     &ConnectorMetrics{},
		rateLimiter: NewRateLimiter(config.RequestsPerSecond, config.BurstLimit),
		retryPolicy: &RetryPolicy{
			MaxRetries:         config.MaxRetries,
			InitialDelay:       config.RetryDelay,
			ExponentialBackoff: config.ExponentialBackoff,
		},
	}

	return connector
}

// Connect establishes connection to OneDrive
func (c *OneDriveConnector) Connect(ctx context.Context, credentials map[string]interface{}) error {
	ctx, span := c.tracer.Start(ctx, "onedrive_connect")
	defer span.End()

	// Extract token from credentials
	tokenData, ok := credentials["oauth_token"]
	if !ok {
		err := fmt.Errorf("oauth_token not found in credentials")
		span.RecordError(err)
		return err
	}

	// Parse token
	token, ok := tokenData.(*oauth2.Token)
	if !ok {
		err := fmt.Errorf("invalid oauth token format")
		span.RecordError(err)
		return err
	}

	c.accessToken = token.AccessToken
	c.httpClient = c.oauthConfig.Client(ctx, token)
	c.isConnected = true

	// Test connection and get drive information
	if err := c.testConnection(ctx); err != nil {
		span.RecordError(err)
		c.isConnected = false
		return fmt.Errorf("connection test failed: %w", err)
	}

	span.SetAttributes(
		attribute.Bool("connected", c.isConnected),
		attribute.String("drive_id", c.driveID),
	)

	return nil
}

// Disconnect closes the connection to OneDrive
func (c *OneDriveConnector) Disconnect(ctx context.Context) error {
	ctx, span := c.tracer.Start(ctx, "onedrive_disconnect")
	defer span.End()

	c.isConnected = false
	c.accessToken = ""
	c.driveID = ""
	c.deltaToken = ""

	// Clear caches
	c.cacheMu.Lock()
	c.fileCache = make(map[string]*DriveItem)
	c.folderCache = make(map[string]*DriveItem)
	c.cacheMu.Unlock()

	return nil
}

// IsConnected returns whether the connector is connected
func (c *OneDriveConnector) IsConnected() bool {
	return c.isConnected
}

// ListFiles lists files from OneDrive
func (c *OneDriveConnector) ListFiles(ctx context.Context, path string, options *storage.ConnectorListOptions) ([]*storage.ConnectorFileInfo, error) {
	ctx, span := c.tracer.Start(ctx, "onedrive_list_files")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to OneDrive")
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(
		attribute.String("path", path),
	)

	var files []*storage.ConnectorFileInfo
	nextLink := ""

	for {
		// Wait for rate limit
		if err := c.rateLimiter.Wait(ctx); err != nil {
			span.RecordError(err)
			return nil, err
		}

		// Build API URL
		var apiURL string
		if nextLink != "" {
			apiURL = nextLink
		} else {
			if path == "" || path == "/" {
				// List root drive items
				apiURL = "https://graph.microsoft.com/v1.0/me/drive/root/children"
			} else {
				// List items in specified path
				apiURL = fmt.Sprintf("https://graph.microsoft.com/v1.0/me/drive/root:/%s:/children", strings.Trim(path, "/"))
			}

			// Add query parameters
			params := []string{
				fmt.Sprintf("$top=%d", c.config.BatchSize),
				"$expand=thumbnails",
			}

			if c.config.IncludeMetadata {
				params = append(params, "$select=*")
			}

			apiURL += "?" + strings.Join(params, "&")
		}

		// Execute API call with retry
		response, err := c.executeGraphAPICall(ctx, "GET", apiURL, nil)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to list files: %w", err)
		}

		var listResponse DriveItemCollection
		if err := json.Unmarshal(response, &listResponse); err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to parse API response: %w", err)
		}

		// Convert OneDrive items to FileInfo
		for _, item := range listResponse.Value {
			if c.shouldIncludeItem(&item) {
				fileInfo := c.convertToFileInfo(&item)
				files = append(files, fileInfo)

				// Cache item
				if c.config.CacheEnabled {
					c.cacheItem(&item)
				}
			}
		}

		// Check if we need to fetch more
		if listResponse.ODataNextLink == "" {
			break
		}
		nextLink = listResponse.ODataNextLink
	}

	// Update metrics
	c.metrics.FilesListed += int64(len(files))
	c.metrics.LastListTime = time.Now()

	span.SetAttributes(
		attribute.Int("files.count", len(files)),
	)

	return files, nil
}

// GetFile retrieves a specific file from OneDrive
func (c *OneDriveConnector) GetFile(ctx context.Context, fileID string) (*storage.ConnectorFileInfo, error) {
	ctx, span := c.tracer.Start(ctx, "onedrive_get_file")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to OneDrive")
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(
		attribute.String("file.id", fileID),
	)

	// Check cache first
	if c.config.CacheEnabled {
		if cachedFile := c.getCachedFile(fileID); cachedFile != nil {
			return c.convertToFileInfo(cachedFile), nil
		}
	}

	// Wait for rate limit
	if err := c.rateLimiter.Wait(ctx); err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Build API URL
	apiURL := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/drive/items/%s", fileID)
	if c.config.IncludeMetadata {
		apiURL += "?$expand=thumbnails&$select=*"
	}

	// Execute API call with retry
	response, err := c.executeGraphAPICall(ctx, "GET", apiURL, nil)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get file: %w", err)
	}

	var driveItem DriveItem
	if err := json.Unmarshal(response, &driveItem); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	// Cache file
	if c.config.CacheEnabled {
		c.cacheItem(&driveItem)
	}

	// Update metrics
	c.metrics.FilesRetrieved++

	return c.convertToFileInfo(&driveItem), nil
}

// DownloadFile downloads file content from OneDrive
func (c *OneDriveConnector) DownloadFile(ctx context.Context, fileID string) (io.ReadCloser, error) {
	ctx, span := c.tracer.Start(ctx, "onedrive_download_file")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to OneDrive")
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

	// Build download URL
	apiURL := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/drive/items/%s/content", fileID)

	// Execute download with retry
	response, err := c.executeWithRetry(ctx, func() (interface{}, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, err
		}

		// Add authorization header
		req.Header.Set("Authorization", "Bearer "+c.accessToken)

		return c.httpClient.Do(req)
	})

	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to download file: %w", err)
	}

	resp := response.(*http.Response)

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		err := fmt.Errorf("download failed with status: %d", resp.StatusCode)
		span.RecordError(err)
		return nil, err
	}

	// Update metrics
	c.metrics.FilesDownloaded++
	c.metrics.BytesDownloaded += resp.ContentLength

	return resp.Body, nil
}

// SyncFiles performs synchronization with OneDrive
func (c *OneDriveConnector) SyncFiles(ctx context.Context, options *storage.SyncOptions) (*storage.SyncResult, error) {
	ctx, span := c.tracer.Start(ctx, "onedrive_sync_files")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to OneDrive")
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

	// Execute sync using OneDrive delta API or full listing
	if err := c.executeSyncOperation(ctx, options, result, isFullSync); err != nil {
		span.RecordError(err)
		result.Errors = append(result.Errors, err.Error())
		return result, err
	}

	// Update sync state
	c.lastSync = syncStart
	c.syncState.TotalFiles = result.FilesFound
	c.syncState.ChangedFiles = result.FilesChanged

	// Update metrics
	c.metrics.SyncCount++
	c.metrics.LastSyncTime = syncStart
	c.metrics.LastSyncDuration = result.EndTime.Sub(result.StartTime)

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	span.SetAttributes(
		attribute.Int64("files.found", result.FilesFound),
		attribute.Int64("files.changed", result.FilesChanged),
		attribute.Float64("duration.seconds", result.Duration.Seconds()),
	)

	return result, nil
}

// GetSyncState returns the current synchronization state
func (c *OneDriveConnector) GetSyncState(ctx context.Context) (*storage.SyncState, error) {
	c.syncMu.RLock()
	defer c.syncMu.RUnlock()

	return &storage.SyncState{
		IsRunning:        c.syncState.IsRunning,
		LastSyncStart:    c.syncState.LastSyncStart,
		LastSyncEnd:      c.syncState.LastSyncEnd,
		LastSyncDuration: c.syncState.LastSyncDuration,
		TotalFiles:       c.syncState.TotalFiles,
		ChangedFiles:     c.syncState.ChangedFiles,
		ErrorCount:       c.syncState.ErrorCount,
		LastError:        c.syncState.LastError,
	}, nil
}

// GetMetrics returns connector metrics
func (c *OneDriveConnector) GetMetrics(ctx context.Context) (*storage.ConnectorMetrics, error) {
	return &storage.ConnectorMetrics{
		ConnectorType:      "onedrive",
		IsConnected:        c.isConnected,
		LastConnectionTime: c.metrics.LastConnectionTime,
		FilesListed:        c.metrics.FilesListed,
		FilesRetrieved:     c.metrics.FilesRetrieved,
		FilesDownloaded:    c.metrics.FilesDownloaded,
		BytesDownloaded:    c.metrics.BytesDownloaded,
		SyncCount:          c.metrics.SyncCount,
		LastSyncTime:       c.metrics.LastSyncTime,
		LastSyncDuration:   c.metrics.LastSyncDuration,
		ErrorCount:         c.metrics.ErrorCount,
		LastError:          c.metrics.LastError,
	}, nil
}

// Helper methods

func (c *OneDriveConnector) testConnection(ctx context.Context) error {
	// Test connection by getting user info and drive info
	userResponse, err := c.executeGraphAPICall(ctx, "GET", "https://graph.microsoft.com/v1.0/me", nil)
	if err != nil {
		return fmt.Errorf("user info request failed: %w", err)
	}

	var user User
	if err := json.Unmarshal(userResponse, &user); err != nil {
		return fmt.Errorf("failed to parse user info: %w", err)
	}

	// Get drive information
	driveResponse, err := c.executeGraphAPICall(ctx, "GET", "https://graph.microsoft.com/v1.0/me/drive", nil)
	if err != nil {
		return fmt.Errorf("drive info request failed: %w", err)
	}

	var drive Drive
	if err := json.Unmarshal(driveResponse, &drive); err != nil {
		return fmt.Errorf("failed to parse drive info: %w", err)
	}

	c.driveID = drive.ID
	c.metrics.LastConnectionTime = time.Now()
	return nil
}

func (c *OneDriveConnector) shouldIncludeItem(item *DriveItem) bool {
	// Check if item is deleted
	if item.Deleted != nil && !c.config.SyncDeletedFiles {
		return false
	}

	// Only process files for now (folders are processed implicitly)
	if item.File == nil {
		return true // Include folders for traversal
	}

	// Check file size
	if c.config.MaxFileSize > 0 && item.Size > c.config.MaxFileSize {
		return false
	}

	// Check file extension
	if len(c.config.FileExtensions) > 0 {
		hasValidExtension := false
		for _, ext := range c.config.FileExtensions {
			if strings.HasSuffix(strings.ToLower(item.Name), strings.ToLower(ext)) {
				hasValidExtension = true
				break
			}
		}
		if !hasValidExtension {
			return false
		}
	}

	return true
}

func (c *OneDriveConnector) convertToFileInfo(item *DriveItem) *storage.ConnectorFileInfo {
	// Parse timestamps
	var createdTime, modifiedTime time.Time
	if item.CreatedDateTime != "" {
		createdTime, _ = time.Parse(time.RFC3339, item.CreatedDateTime)
	}
	if item.LastModifiedDateTime != "" {
		modifiedTime, _ = time.Parse(time.RFC3339, item.LastModifiedDateTime)
	}

	// Determine file type based on name/extension and metadata
	var fileType storage.FileType
	name := strings.ToLower(item.Name)
	if strings.HasSuffix(name, ".pdf") {
		fileType = storage.FileTypePDF
	} else if strings.HasSuffix(name, ".doc") || strings.HasSuffix(name, ".docx") {
		fileType = storage.FileTypeDocument
	} else if strings.HasSuffix(name, ".xls") || strings.HasSuffix(name, ".xlsx") {
		fileType = storage.FileTypeSpreadsheet
	} else if strings.HasSuffix(name, ".ppt") || strings.HasSuffix(name, ".pptx") {
		fileType = storage.FileTypePresentation
	} else if strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".png") || strings.HasSuffix(name, ".gif") {
		fileType = storage.FileTypeImage
	} else if strings.HasSuffix(name, ".mp4") || strings.HasSuffix(name, ".mov") || strings.HasSuffix(name, ".avi") {
		fileType = storage.FileTypeVideo
	} else if strings.HasSuffix(name, ".mp3") || strings.HasSuffix(name, ".wav") || strings.HasSuffix(name, ".flac") {
		fileType = storage.FileTypeAudio
	} else {
		fileType = storage.FileTypeOther
	}

	// Build path from parent reference
	path := "/" + item.Name
	if item.ParentReference != nil && item.ParentReference.Path != "" {
		// Clean up the path
		parentPath := strings.TrimPrefix(item.ParentReference.Path, "/drive/root:")
		if parentPath != "" {
			path = parentPath + "/" + item.Name
		}
	}

	// Determine MIME type
	mimeType := ""
	if item.File != nil {
		mimeType = item.File.MimeType
	}

	// Create file info
	fileInfo := &storage.ConnectorFileInfo{
		ID:           item.ID,
		Name:         item.Name,
		Path:         path,
		URL:          item.WebURL,
		DownloadURL:  item.DownloadURL,
		Size:         item.Size,
		Type:         fileType,
		MimeType:     mimeType,
		CreatedAt:    createdTime,
		ModifiedAt:   modifiedTime,
		LastAccessed: modifiedTime, // OneDrive doesn't track last access
		IsFolder:     item.Folder != nil,
		Permissions: storage.FilePermissions{
			CanRead:   true,
			CanWrite:  false, // Assuming read-only for now
			CanDelete: false,
		},
		Metadata: map[string]interface{}{
			"onedrive_id": item.ID,
			"web_url":     item.WebURL,
			"ctag":        item.CTag,
			"etag":        item.ETag,
		},
		Tags: map[string]string{
			"source": "onedrive",
		},
		Source: "onedrive",
	}

	// Add file-specific metadata
	if item.File != nil && item.File.Hashes != nil {
		if item.File.Hashes.SHA1Hash != "" {
			fileInfo.Checksum = item.File.Hashes.SHA1Hash
			fileInfo.ChecksumType = "sha1"
		} else if item.File.Hashes.QuickXorHash != "" {
			fileInfo.Checksum = item.File.Hashes.QuickXorHash
			fileInfo.ChecksumType = "quickxor"
		}
	}

	// Add sharing information if available
	if item.Shared != nil {
		fileInfo.Metadata["shared_info"] = item.Shared
		fileInfo.Tags["shared"] = "true"
	}

	return fileInfo
}

func (c *OneDriveConnector) cacheItem(item *DriveItem) {
	if !c.config.CacheEnabled {
		return
	}

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	if item.File != nil {
		c.fileCache[item.ID] = item
	} else if item.Folder != nil {
		c.folderCache[item.ID] = item
	}

	// Simple cache size management
	if len(c.fileCache) > c.config.CacheSize {
		// Remove oldest entries (simple FIFO)
		count := 0
		for id := range c.fileCache {
			delete(c.fileCache, id)
			count++
			if count >= c.config.CacheSize/10 { // Remove 10% of cache
				break
			}
		}
	}
}

func (c *OneDriveConnector) getCachedFile(fileID string) *DriveItem {
	if !c.config.CacheEnabled {
		return nil
	}

	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()

	return c.fileCache[fileID]
}

func (c *OneDriveConnector) executeSyncOperation(ctx context.Context, options *storage.SyncOptions, result *storage.SyncResult, isFullSync bool) error {
	// For OneDrive, we can use the delta API for incremental sync
	// or traverse the drive for full sync

	if isFullSync {
		return c.executeFullSync(ctx, options, result)
	} else {
		return c.executeIncrementalSync(ctx, options, result)
	}
}

func (c *OneDriveConnector) executeFullSync(ctx context.Context, options *storage.SyncOptions, result *storage.SyncResult) error {
	// Use standard listing to get all files
	files, err := c.ListFiles(ctx, "", &storage.ConnectorListOptions{})
	if err != nil {
		return err
	}

	result.FilesFound = int64(len(files))
	result.FilesChanged = int64(len(files)) // For full sync, consider all files as changed

	return nil
}

func (c *OneDriveConnector) executeIncrementalSync(ctx context.Context, options *storage.SyncOptions, result *storage.SyncResult) error {
	// Use OneDrive's delta endpoint for incremental changes
	// This would compare against the stored delta token from last sync

	// Build delta API URL
	var apiURL string
	if c.deltaToken != "" {
		apiURL = c.deltaToken // Use the saved delta link
	} else {
		apiURL = "https://graph.microsoft.com/v1.0/me/drive/root/delta"
	}

	for {
		// Wait for rate limit
		if err := c.rateLimiter.Wait(ctx); err != nil {
			return err
		}

		// Execute delta API call
		response, err := c.executeGraphAPICall(ctx, "GET", apiURL, nil)
		if err != nil {
			return fmt.Errorf("delta API call failed: %w", err)
		}

		var deltaResponse DeltaLink
		if err := json.Unmarshal(response, &deltaResponse); err != nil {
			return fmt.Errorf("failed to parse delta response: %w", err)
		}

		// Process changes
		for _, item := range deltaResponse.Value {
			if c.shouldIncludeItem(&item) {
				result.FilesChanged++
			}
		}

		result.FilesFound += int64(len(deltaResponse.Value))

		// Check for next page or delta link
		if deltaResponse.ODataNextLink != "" {
			apiURL = deltaResponse.ODataNextLink
		} else if deltaResponse.ODataDeltaLink != "" {
			// Save delta link for next incremental sync
			c.deltaToken = deltaResponse.ODataDeltaLink
			c.syncState.DeltaToken = c.deltaToken
			break
		} else {
			break
		}
	}

	return nil
}

func (c *OneDriveConnector) executeGraphAPICall(ctx context.Context, method, url string, body []byte) ([]byte, error) {
	return c.executeWithRetryBytes(ctx, func() ([]byte, error) {
		var reqBody io.Reader
		if body != nil {
			reqBody = bytes.NewReader(body)
		}

		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return nil, err
		}

		// Add headers
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
		req.Header.Set("Content-Type", "application/json")

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

func (c *OneDriveConnector) executeWithRetry(ctx context.Context, operation func() (interface{}, error)) (interface{}, error) {
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

func (c *OneDriveConnector) executeWithRetryBytes(ctx context.Context, operation func() ([]byte, error)) ([]byte, error) {
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

func (c *OneDriveConnector) isRetryableError(err error) bool {
	errStr := strings.ToLower(err.Error())
	retryableErrors := []string{
		"throttle",
		"too many requests",
		"rate limit",
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
