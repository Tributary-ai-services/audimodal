package dropbox

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

// DropboxConnector implements storage.StorageConnector for Dropbox
type DropboxConnector struct {
	config      *DropboxConfig
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
	fileCache   map[string]*DropboxFile
	folderCache map[string]*DropboxFolder
	cacheMu     sync.RWMutex

	// Sync state
	syncState *SyncState
	syncMu    sync.RWMutex

	// Metrics
	metrics *ConnectorMetrics

	// Error handling
	retryPolicy *RetryPolicy

	// Dropbox-specific
	cursor string // For delta/list_folder/continue operations
}

// DropboxConfig contains configuration for Dropbox connector
type DropboxConfig struct {
	// OAuth2 configuration
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURL  string `yaml:"redirect_url"`

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

	// Dropbox-specific features
	UseDropboxAPI    int  `yaml:"use_dropbox_api"` // 1 for v1, 2 for v2
	IncludeMediaInfo bool `yaml:"include_media_info"`
	IncludeSharing   bool `yaml:"include_sharing"`
	RecursiveSync    bool `yaml:"recursive_sync"`
	TeamSpace        bool `yaml:"team_space"`

	// Paper integration
	SyncPaperDocs     bool   `yaml:"sync_paper_docs"`
	PaperExportFormat string `yaml:"paper_export_format"` // html, markdown, pdf
}

// DefaultDropboxConfig returns default configuration
func DefaultDropboxConfig() *DropboxConfig {
	return &DropboxConfig{
		SyncInterval:       15 * time.Minute,
		FullSyncInterval:   24 * time.Hour,
		BatchSize:          2000, // Dropbox supports up to 2000 entries per request
		MaxConcurrentReqs:  10,
		FileExtensions:     []string{".pdf", ".doc", ".docx", ".txt", ".xlsx", ".pptx"},
		MaxFileSize:        350 * 1024 * 1024, // 350MB (Dropbox file size limit via API)
		SyncSharedFiles:    true,
		SyncDeletedFiles:   false,
		RequestsPerSecond:  120.0, // Dropbox allows ~120 requests per app per second
		BurstLimit:         200,
		MaxRetries:         3,
		RetryDelay:         1 * time.Second,
		ExponentialBackoff: true,
		CacheEnabled:       true,
		CacheTTL:           1 * time.Hour,
		CacheSize:          10000,
		UseDropboxAPI:      2, // Use API v2 by default
		IncludeMediaInfo:   false,
		IncludeSharing:     true,
		RecursiveSync:      true,
		TeamSpace:          false,
		SyncPaperDocs:      false,
		PaperExportFormat:  "markdown",
	}
}

// NewDropboxConnector creates a new Dropbox connector
func NewDropboxConnector(config *DropboxConfig) *DropboxConnector {
	if config == nil {
		config = DefaultDropboxConfig()
	}

	// Setup OAuth2 configuration for Dropbox
	oauthConfig := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,
		Scopes:       []string{"files.metadata.read", "files.content.read", "sharing.read"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://www.dropbox.com/oauth2/authorize",
			TokenURL: "https://api.dropboxapi.com/oauth2/token",
		},
	}

	connector := &DropboxConnector{
		config:      config,
		oauthConfig: oauthConfig,
		httpClient:  &http.Client{Timeout: 60 * time.Second}, // Longer timeout for large files
		tracer:      otel.Tracer("dropbox-connector"),
		fileCache:   make(map[string]*DropboxFile),
		folderCache: make(map[string]*DropboxFolder),
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

// Connect establishes connection to Dropbox
func (c *DropboxConnector) Connect(ctx context.Context, credentials map[string]interface{}) error {
	ctx, span := c.tracer.Start(ctx, "dropbox_connect")
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

	// Test connection
	if err := c.testConnection(ctx); err != nil {
		span.RecordError(err)
		c.isConnected = false
		return fmt.Errorf("connection test failed: %w", err)
	}

	span.SetAttributes(
		attribute.Bool("connected", c.isConnected),
	)

	return nil
}

// Disconnect closes the connection to Dropbox
func (c *DropboxConnector) Disconnect(ctx context.Context) error {
	ctx, span := c.tracer.Start(ctx, "dropbox_disconnect")
	defer span.End()

	c.isConnected = false
	c.accessToken = ""
	c.cursor = ""

	// Clear caches
	c.cacheMu.Lock()
	c.fileCache = make(map[string]*DropboxFile)
	c.folderCache = make(map[string]*DropboxFolder)
	c.cacheMu.Unlock()

	return nil
}

// IsConnected returns whether the connector is connected
func (c *DropboxConnector) IsConnected() bool {
	return c.isConnected
}

// ListFiles lists files from Dropbox
func (c *DropboxConnector) ListFiles(ctx context.Context, path string, options *storage.ConnectorListOptions) ([]*storage.ConnectorFileInfo, error) {
	ctx, span := c.tracer.Start(ctx, "dropbox_list_files")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to Dropbox")
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(
		attribute.String("path", path),
	)

	var files []*storage.ConnectorFileInfo
	cursor := ""

	for {
		// Wait for rate limit
		if err := c.rateLimiter.Wait(ctx); err != nil {
			span.RecordError(err)
			return nil, err
		}

		// Prepare API request
		var apiURL string
		var requestBody interface{}

		if cursor == "" {
			// Initial request
			apiURL = "https://api.dropboxapi.com/2/files/list_folder"
			requestBody = &ListFolderRequest{
				Path:                            c.normalizePath(path),
				Recursive:                       c.config.RecursiveSync,
				IncludeMediaInfo:                c.config.IncludeMediaInfo,
				IncludeDeleted:                  c.config.SyncDeletedFiles,
				IncludeHasExplicitSharedMembers: c.config.IncludeSharing,
				Limit:                           uint32(c.config.BatchSize),
			}
		} else {
			// Continuation request
			apiURL = "https://api.dropboxapi.com/2/files/list_folder/continue"
			requestBody = &ListFolderContinueRequest{
				Cursor: cursor,
			}
		}

		// Execute API call with retry
		response, err := c.executeDropboxAPICall(ctx, apiURL, requestBody)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to list files: %w", err)
		}

		var listResponse ListFolderResponse
		if err := json.Unmarshal(response, &listResponse); err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to parse API response: %w", err)
		}

		// Convert Dropbox entries to FileInfo
		for _, entry := range listResponse.Entries {
			if c.shouldIncludeEntry(&entry) {
				fileInfo := c.convertToFileInfo(&entry)
				files = append(files, fileInfo)

				// Cache entry
				if c.config.CacheEnabled {
					c.cacheEntry(&entry)
				}
			}
		}

		// Check if we need to fetch more
		if !listResponse.HasMore {
			break
		}
		cursor = listResponse.Cursor
	}

	// Update metrics
	c.metrics.FilesListed += int64(len(files))
	c.metrics.LastListTime = time.Now()

	span.SetAttributes(
		attribute.Int("files.count", len(files)),
	)

	return files, nil
}

// GetFile retrieves a specific file from Dropbox
func (c *DropboxConnector) GetFile(ctx context.Context, fileID string) (*storage.ConnectorFileInfo, error) {
	ctx, span := c.tracer.Start(ctx, "dropbox_get_file")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to Dropbox")
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(
		attribute.String("file.id", fileID),
	)

	// Check cache first
	if c.config.CacheEnabled {
		if cachedFile := c.getCachedFile(fileID); cachedFile != nil {
			return c.convertFileToFileInfo(cachedFile), nil
		}
	}

	// Wait for rate limit
	if err := c.rateLimiter.Wait(ctx); err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Get file metadata
	requestBody := &GetMetadataRequest{
		Path:                            fileID,
		IncludeMediaInfo:                c.config.IncludeMediaInfo,
		IncludeDeleted:                  c.config.SyncDeletedFiles,
		IncludeHasExplicitSharedMembers: c.config.IncludeSharing,
	}

	// Execute API call with retry
	response, err := c.executeDropboxAPICall(ctx, "https://api.dropboxapi.com/2/files/get_metadata", requestBody)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get file: %w", err)
	}

	var metadata DropboxEntry
	if err := json.Unmarshal(response, &metadata); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	// Cache file if it's a file type
	if c.config.CacheEnabled && metadata.Tag == "file" {
		file := &DropboxFile{
			DropboxEntry: metadata,
		}
		c.cacheFile(file)
	}

	// Update metrics
	c.metrics.FilesRetrieved++

	return c.convertToFileInfo(&metadata), nil
}

// DownloadFile downloads file content from Dropbox
func (c *DropboxConnector) DownloadFile(ctx context.Context, fileID string) (io.ReadCloser, error) {
	ctx, span := c.tracer.Start(ctx, "dropbox_download_file")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to Dropbox")
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

	// Prepare download request
	downloadRequest := &DownloadRequest{
		Path: fileID,
	}

	downloadJSON, err := json.Marshal(downloadRequest)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to marshal download request: %w", err)
	}

	// Execute download with retry
	response, err := c.executeWithRetry(ctx, func() (interface{}, error) {
		req, err := http.NewRequestWithContext(ctx, "POST",
			"https://content.dropboxapi.com/2/files/download", nil)
		if err != nil {
			return nil, err
		}

		// Add headers
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
		req.Header.Set("Dropbox-API-Arg", string(downloadJSON))

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

// SyncFiles performs synchronization with Dropbox
func (c *DropboxConnector) SyncFiles(ctx context.Context, options *storage.SyncOptions) (*storage.SyncResult, error) {
	ctx, span := c.tracer.Start(ctx, "dropbox_sync_files")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to Dropbox")
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

	// Execute sync using Dropbox delta or list_folder API
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
func (c *DropboxConnector) GetSyncState(ctx context.Context) (*storage.SyncState, error) {
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
func (c *DropboxConnector) GetMetrics(ctx context.Context) (*storage.ConnectorMetrics, error) {
	return &storage.ConnectorMetrics{
		ConnectorType:      "dropbox",
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

func (c *DropboxConnector) testConnection(ctx context.Context) error {
	// Test connection by getting current account info
	response, err := c.executeDropboxAPICall(ctx, "https://api.dropboxapi.com/2/users/get_current_account", nil)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}

	var account DropboxAccount
	if err := json.Unmarshal(response, &account); err != nil {
		return fmt.Errorf("failed to parse account info: %w", err)
	}

	c.metrics.LastConnectionTime = time.Now()
	return nil
}

func (c *DropboxConnector) normalizePath(path string) string {
	if path == "" || path == "/" {
		return ""
	}

	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	return path
}

func (c *DropboxConnector) shouldIncludeEntry(entry *DropboxEntry) bool {
	// Check if entry is deleted
	if entry.Tag == "deleted" && !c.config.SyncDeletedFiles {
		return false
	}

	// Only process files for now (folders are processed implicitly)
	if entry.Tag != "file" {
		return true // Include folders for traversal
	}

	// Check file size
	if c.config.MaxFileSize > 0 && entry.Size > c.config.MaxFileSize {
		return false
	}

	// Check file extension
	if len(c.config.FileExtensions) > 0 {
		hasValidExtension := false
		for _, ext := range c.config.FileExtensions {
			if strings.HasSuffix(strings.ToLower(entry.Name), strings.ToLower(ext)) {
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

func (c *DropboxConnector) convertToFileInfo(entry *DropboxEntry) *storage.ConnectorFileInfo {
	// Parse timestamps
	var modifiedTime time.Time
	if entry.ClientModified != "" {
		modifiedTime, _ = time.Parse(time.RFC3339, entry.ClientModified)
	} else if entry.ServerModified != "" {
		modifiedTime, _ = time.Parse(time.RFC3339, entry.ServerModified)
	}

	// Determine file type based on name/extension
	var fileType storage.FileType
	name := strings.ToLower(entry.Name)
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

	// Create file info
	fileInfo := &storage.ConnectorFileInfo{
		ID:           entry.ID,
		Name:         entry.Name,
		Path:         entry.PathLower,
		URL:          "", // Dropbox doesn't provide direct web URLs in API
		DownloadURL:  "", // Generated dynamically
		Size:         entry.Size,
		Type:         fileType,
		MimeType:     "",           // Dropbox doesn't provide MIME type in metadata
		CreatedAt:    modifiedTime, // Dropbox doesn't provide creation time
		ModifiedAt:   modifiedTime,
		LastAccessed: modifiedTime,
		IsFolder:     entry.Tag == "folder",
		Permissions: storage.FilePermissions{
			CanRead:   true,
			CanWrite:  false, // Assuming read-only for now
			CanDelete: false,
		},
		Metadata: map[string]interface{}{
			"dropbox_id":         entry.ID,
			"path_display":       entry.PathDisplay,
			"path_lower":         entry.PathLower,
			"tag":                entry.Tag,
			"content_hash":       entry.ContentHash,
			"has_shared_members": entry.HasExplicitSharedMembers,
		},
		Tags: map[string]string{
			"source": "dropbox",
			"tag":    entry.Tag,
		},
		Source:       "dropbox",
		Checksum:     entry.ContentHash,
		ChecksumType: "dropbox_content_hash",
	}

	// Add sharing information if available
	if entry.SharingInfo != nil {
		fileInfo.Metadata["sharing_info"] = entry.SharingInfo
		fileInfo.Tags["shared"] = "true"
	}

	return fileInfo
}

func (c *DropboxConnector) convertFileToFileInfo(file *DropboxFile) *storage.ConnectorFileInfo {
	return c.convertToFileInfo(&file.DropboxEntry)
}

func (c *DropboxConnector) cacheEntry(entry *DropboxEntry) {
	if !c.config.CacheEnabled {
		return
	}

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	if entry.Tag == "file" {
		file := &DropboxFile{
			DropboxEntry: *entry,
		}
		c.fileCache[entry.ID] = file
	} else if entry.Tag == "folder" {
		folder := &DropboxFolder{
			DropboxEntry: *entry,
		}
		c.folderCache[entry.ID] = folder
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

func (c *DropboxConnector) cacheFile(file *DropboxFile) {
	if !c.config.CacheEnabled {
		return
	}

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	c.fileCache[file.ID] = file
}

func (c *DropboxConnector) getCachedFile(fileID string) *DropboxFile {
	if !c.config.CacheEnabled {
		return nil
	}

	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()

	return c.fileCache[fileID]
}

func (c *DropboxConnector) executeSyncOperation(ctx context.Context, options *storage.SyncOptions, result *storage.SyncResult, isFullSync bool) error {
	// For Dropbox, we can use list_folder for full sync
	// and list_folder/longpoll + get_latest_cursor for incremental sync

	if isFullSync {
		return c.executeFullSync(ctx, options, result)
	} else {
		return c.executeIncrementalSync(ctx, options, result)
	}
}

func (c *DropboxConnector) executeFullSync(ctx context.Context, options *storage.SyncOptions, result *storage.SyncResult) error {
	// Use list_folder recursively to get all files
	files, err := c.ListFiles(ctx, "", &storage.ConnectorListOptions{})
	if err != nil {
		return err
	}

	result.FilesFound = int64(len(files))
	result.FilesChanged = int64(len(files)) // For full sync, consider all files as changed

	return nil
}

func (c *DropboxConnector) executeIncrementalSync(ctx context.Context, options *storage.SyncOptions, result *storage.SyncResult) error {
	// Use Dropbox's delta endpoint or list_folder with cursor for incremental changes
	// This would compare against the stored cursor from last sync

	// For now, fall back to full sync
	return c.executeFullSync(ctx, options, result)
}

func (c *DropboxConnector) executeDropboxAPICall(ctx context.Context, url string, requestBody interface{}) ([]byte, error) {
	return c.executeWithRetryBytes(ctx, func() ([]byte, error) {
		var reqBody io.Reader
		if requestBody != nil {
			jsonBody, err := json.Marshal(requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal request body: %w", err)
			}
			reqBody = bytes.NewReader(jsonBody)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", url, reqBody)
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

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API request failed with status: %d", resp.StatusCode)
		}

		return io.ReadAll(resp.Body)
	})
}

func (c *DropboxConnector) executeWithRetry(ctx context.Context, operation func() (interface{}, error)) (interface{}, error) {
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

func (c *DropboxConnector) executeWithRetryBytes(ctx context.Context, operation func() ([]byte, error)) ([]byte, error) {
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

func (c *DropboxConnector) isRetryableError(err error) bool {
	errStr := strings.ToLower(err.Error())
	retryableErrors := []string{
		"rate limit",
		"too_many_requests",
		"internal_error",
		"service unavailable",
		"timeout",
		"connection",
		"network",
		"502", "503", "504", // HTTP status codes
	}

	for _, retryable := range retryableErrors {
		if strings.Contains(errStr, retryable) {
			return true
		}
	}

	return false
}
