package box

import (
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

// BoxConnector implements storage.StorageConnector for Box
type BoxConnector struct {
	config       *BoxConfig
	oauthConfig  *oauth2.Config
	httpClient   *http.Client
	tracer       trace.Tracer
	
	// Connection state
	isConnected  bool
	lastSync     time.Time
	accessToken  string
	
	// Rate limiting and throttling
	rateLimiter  *RateLimiter
	
	// Caching
	folderCache  map[string]*BoxFolder
	fileCache    map[string]*BoxFile
	cacheMu      sync.RWMutex
	
	// Sync state
	syncState    *SyncState
	syncMu       sync.RWMutex
	
	// Metrics
	metrics      *ConnectorMetrics
	
	// Error handling
	retryPolicy  *RetryPolicy
}

// BoxConfig contains configuration for Box connector
type BoxConfig struct {
	// OAuth2 configuration
	ClientID           string   `yaml:"client_id"`
	ClientSecret       string   `yaml:"client_secret"`
	RedirectURL        string   `yaml:"redirect_url"`
	
	// Sync configuration
	SyncInterval       time.Duration `yaml:"sync_interval"`
	FullSyncInterval   time.Duration `yaml:"full_sync_interval"`
	BatchSize          int           `yaml:"batch_size"`
	MaxConcurrentReqs  int           `yaml:"max_concurrent_requests"`
	
	// Filter configuration
	IncludeFolders     []string      `yaml:"include_folders"`
	ExcludeFolders     []string      `yaml:"exclude_folders"`
	FileExtensions     []string      `yaml:"file_extensions"`
	MaxFileSize        int64         `yaml:"max_file_size"`
	SyncSharedFiles    bool          `yaml:"sync_shared_files"`
	SyncTrashedFiles   bool          `yaml:"sync_trashed_files"`
	
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
	
	// Enterprise features
	EnterpriseEventsEnabled bool   `yaml:"enterprise_events_enabled"`
	ContentInsightsEnabled  bool   `yaml:"content_insights_enabled"`
	DLPEnabled             bool   `yaml:"dlp_enabled"`
}

// DefaultBoxConfig returns default configuration
func DefaultBoxConfig() *BoxConfig {
	return &BoxConfig{
		SyncInterval:       15 * time.Minute,
		FullSyncInterval:   24 * time.Hour,
		BatchSize:          100,
		MaxConcurrentReqs:  10,
		FileExtensions:     []string{".pdf", ".doc", ".docx", ".txt", ".xlsx", ".pptx"},
		MaxFileSize:        100 * 1024 * 1024, // 100MB
		SyncSharedFiles:    true,
		SyncTrashedFiles:   false,
		RequestsPerSecond:  10.0,
		BurstLimit:         50,
		MaxRetries:         3,
		RetryDelay:         5 * time.Second,
		ExponentialBackoff: true,
		CacheEnabled:       true,
		CacheTTL:           1 * time.Hour,
		CacheSize:          10000,
		EnterpriseEventsEnabled: true,
		ContentInsightsEnabled:  false,
		DLPEnabled:             false,
	}
}

// NewBoxConnector creates a new Box connector
func NewBoxConnector(config *BoxConfig) *BoxConnector {
	if config == nil {
		config = DefaultBoxConfig()
	}

	// Setup OAuth2 configuration for Box
	oauthConfig := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,
		Scopes:       []string{"root_readonly", "root_readwrite"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://account.box.com/api/oauth2/authorize",
			TokenURL: "https://api.box.com/oauth2/token",
		},
	}

	connector := &BoxConnector{
		config:      config,
		oauthConfig: oauthConfig,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		tracer:      otel.Tracer("box-connector"),
		folderCache: make(map[string]*BoxFolder),
		fileCache:   make(map[string]*BoxFile),
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

// Connect establishes connection to Box
func (c *BoxConnector) Connect(ctx context.Context, credentials map[string]interface{}) error {
	ctx, span := c.tracer.Start(ctx, "box_connect")
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

// Disconnect closes the connection to Box
func (c *BoxConnector) Disconnect(ctx context.Context) error {
	ctx, span := c.tracer.Start(ctx, "box_disconnect")
	defer span.End()

	c.isConnected = false
	c.accessToken = ""
	
	// Clear caches
	c.cacheMu.Lock()
	c.folderCache = make(map[string]*BoxFolder)
	c.fileCache = make(map[string]*BoxFile)
	c.cacheMu.Unlock()

	return nil
}

// IsConnected returns whether the connector is connected
func (c *BoxConnector) IsConnected() bool {
	return c.isConnected
}

// ListFiles lists files from Box
func (c *BoxConnector) ListFiles(ctx context.Context, path string, options *storage.ConnectorListOptions) ([]*storage.ConnectorFileInfo, error) {
	ctx, span := c.tracer.Start(ctx, "box_list_files")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to Box")
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(
		attribute.String("path", path),
	)

	var files []*storage.ConnectorFileInfo
	var offset int64 = 0
	limit := int64(c.config.BatchSize)
	
	for {
		// Wait for rate limit
		if err := c.rateLimiter.Wait(ctx); err != nil {
			span.RecordError(err)
			return nil, err
		}

		// Build API URL for folder listing
		folderID := c.pathToFolderID(path)
		apiURL := fmt.Sprintf("https://api.box.com/2.0/folders/%s/items", folderID)
		
		// Execute API call with retry
		response, err := c.executeWithRetry(ctx, func() (interface{}, error) {
			req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
			if err != nil {
				return nil, err
			}
			
			// Add query parameters
			q := req.URL.Query()
			q.Set("limit", fmt.Sprintf("%d", limit))
			q.Set("offset", fmt.Sprintf("%d", offset))
			q.Set("fields", "id,name,type,size,created_at,modified_at,path_collection,shared_link,trashed_at,content_created_at,content_modified_at")
			req.URL.RawQuery = q.Encode()
			
			// Add authorization header
			req.Header.Set("Authorization", "Bearer "+c.accessToken)
			
			return c.httpClient.Do(req)
		})
		
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to list files: %w", err)
		}

		resp := response.(*http.Response)
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			err := fmt.Errorf("API request failed with status: %d", resp.StatusCode)
			span.RecordError(err)
			return nil, err
		}

		var listResponse BoxFolderItemsResponse
		if err := json.NewDecoder(resp.Body).Decode(&listResponse); err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to parse API response: %w", err)
		}
		
		// Convert Box items to FileInfo
		for _, item := range listResponse.Entries {
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
		if len(listResponse.Entries) < int(limit) || offset+limit >= int64(listResponse.TotalCount) {
			break
		}
		offset += limit
	}

	// Update metrics
	c.metrics.FilesListed += int64(len(files))
	c.metrics.LastListTime = time.Now()

	span.SetAttributes(
		attribute.Int("files.count", len(files)),
	)

	return files, nil
}

// GetFile retrieves a specific file from Box
func (c *BoxConnector) GetFile(ctx context.Context, fileID string) (*storage.ConnectorFileInfo, error) {
	ctx, span := c.tracer.Start(ctx, "box_get_file")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to Box")
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

	// Execute API call with retry
	response, err := c.executeWithRetry(ctx, func() (interface{}, error) {
		apiURL := fmt.Sprintf("https://api.box.com/2.0/files/%s", fileID)
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, err
		}
		
		// Add query parameters for fields
		q := req.URL.Query()
		q.Set("fields", "id,name,type,size,created_at,modified_at,path_collection,shared_link,trashed_at,content_created_at,content_modified_at,sha1")
		req.URL.RawQuery = q.Encode()
		
		// Add authorization header
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
		
		return c.httpClient.Do(req)
	})

	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get file: %w", err)
	}

	resp := response.(*http.Response)
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("API request failed with status: %d", resp.StatusCode)
		span.RecordError(err)
		return nil, err
	}

	var boxFile BoxFile
	if err := json.NewDecoder(resp.Body).Decode(&boxFile); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}
	
	// Cache file
	if c.config.CacheEnabled {
		c.cacheFile(&boxFile)
	}

	// Update metrics
	c.metrics.FilesRetrieved++

	return c.convertFileToFileInfo(&boxFile), nil
}

// DownloadFile downloads file content from Box
func (c *BoxConnector) DownloadFile(ctx context.Context, fileID string) (io.ReadCloser, error) {
	ctx, span := c.tracer.Start(ctx, "box_download_file")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to Box")
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

	// Execute download with retry
	response, err := c.executeWithRetry(ctx, func() (interface{}, error) {
		apiURL := fmt.Sprintf("https://api.box.com/2.0/files/%s/content", fileID)
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

// SyncFiles performs synchronization with Box
func (c *BoxConnector) SyncFiles(ctx context.Context, options *storage.SyncOptions) (*storage.SyncResult, error) {
	ctx, span := c.tracer.Start(ctx, "box_sync_files")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to Box")
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

	// Execute sync using Box's events API or folder traversal
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
func (c *BoxConnector) GetSyncState(ctx context.Context) (*storage.SyncState, error) {
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
func (c *BoxConnector) GetMetrics(ctx context.Context) (*storage.ConnectorMetrics, error) {
	return &storage.ConnectorMetrics{
		ConnectorType:      "box",
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

func (c *BoxConnector) testConnection(ctx context.Context) error {
	// Test connection by getting user info
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.box.com/2.0/users/me", nil)
	if err != nil {
		return err
	}
	
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("connection test failed with status: %d", resp.StatusCode)
	}
	
	c.metrics.LastConnectionTime = time.Now()
	return nil
}

func (c *BoxConnector) pathToFolderID(path string) string {
	// Convert path to Box folder ID
	// Root folder is "0"
	if path == "" || path == "/" {
		return "0"
	}
	
	// For now, treat the path as a folder ID
	// In a full implementation, this would traverse the folder hierarchy
	return strings.TrimPrefix(path, "/")
}

func (c *BoxConnector) shouldIncludeItem(item *BoxItem) bool {
	// Check if item is trashed
	if item.TrashedAt != "" && !c.config.SyncTrashedFiles {
		return false
	}

	// Only process files for now
	if item.Type != "file" {
		return false
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

func (c *BoxConnector) convertToFileInfo(item *BoxItem) *storage.ConnectorFileInfo {
	// Parse timestamps
	var createdTime, modifiedTime time.Time
	if item.CreatedAt != "" {
		createdTime, _ = time.Parse(time.RFC3339, item.CreatedAt)
	}
	if item.ModifiedAt != "" {
		modifiedTime, _ = time.Parse(time.RFC3339, item.ModifiedAt)
	}

	// Determine file type based on name/extension
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
	} else {
		fileType = storage.FileTypeOther
	}

	// Build path from path collection
	path := "/" + item.Name
	if item.PathCollection != nil && len(item.PathCollection.Entries) > 0 {
		var pathParts []string
		for _, entry := range item.PathCollection.Entries {
			if entry.Name != "" && entry.Name != "All Files" {
				pathParts = append(pathParts, entry.Name)
			}
		}
		if len(pathParts) > 0 {
			path = "/" + strings.Join(pathParts, "/") + "/" + item.Name
		}
	}

	return &storage.ConnectorFileInfo{
		ID:           item.ID,
		Name:         item.Name,
		Path:         path,
		URL:          fmt.Sprintf("https://app.box.com/file/%s", item.ID),
		DownloadURL:  fmt.Sprintf("https://api.box.com/2.0/files/%s/content", item.ID),
		Size:         item.Size,
		Type:         fileType,
		MimeType:     "", // Box doesn't always provide mime type in list response
		CreatedAt:    createdTime,
		ModifiedAt:   modifiedTime,
		LastAccessed: modifiedTime, // Box doesn't track last access
		IsFolder:     item.Type == "folder",
		Permissions: storage.FilePermissions{
			CanRead:   true,
			CanWrite:  false, // Assuming read-only for now
			CanDelete: false,
		},
		Metadata: map[string]interface{}{
			"box_id":      item.ID,
			"type":        item.Type,
			"sequence_id": item.SequenceID,
		},
		Tags: map[string]string{
			"source": "box",
			"type":   item.Type,
		},
		Source: "box",
	}
}

func (c *BoxConnector) convertFileToFileInfo(file *BoxFile) *storage.ConnectorFileInfo {
	// Parse timestamps
	var createdTime, modifiedTime time.Time
	if file.CreatedAt != "" {
		createdTime, _ = time.Parse(time.RFC3339, file.CreatedAt)
	}
	if file.ModifiedAt != "" {
		modifiedTime, _ = time.Parse(time.RFC3339, file.ModifiedAt)
	}

	// Determine file type
	var fileType storage.FileType
	name := strings.ToLower(file.Name)
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
	} else {
		fileType = storage.FileTypeOther
	}

	// Build path
	path := "/" + file.Name
	if file.PathCollection != nil && len(file.PathCollection.Entries) > 0 {
		var pathParts []string
		for _, entry := range file.PathCollection.Entries {
			if entry.Name != "" && entry.Name != "All Files" {
				pathParts = append(pathParts, entry.Name)
			}
		}
		if len(pathParts) > 0 {
			path = "/" + strings.Join(pathParts, "/") + "/" + file.Name
		}
	}

	return &storage.ConnectorFileInfo{
		ID:           file.ID,
		Name:         file.Name,
		Path:         path,
		URL:          fmt.Sprintf("https://app.box.com/file/%s", file.ID),
		DownloadURL:  fmt.Sprintf("https://api.box.com/2.0/files/%s/content", file.ID),
		Size:         file.Size,
		Type:         fileType,
		MimeType:     "", // Box doesn't always provide mime type
		CreatedAt:    createdTime,
		ModifiedAt:   modifiedTime,
		LastAccessed: modifiedTime,
		IsFolder:     false,
		Permissions: storage.FilePermissions{
			CanRead:   true,
			CanWrite:  false,
			CanDelete: false,
		},
		Metadata: map[string]interface{}{
			"box_id":      file.ID,
			"sequence_id": file.SequenceID,
			"sha1":        file.SHA1,
		},
		Tags: map[string]string{
			"source": "box",
			"type":   "file",
		},
		Source:   "box",
		Checksum: file.SHA1,
		ChecksumType: "sha1",
	}
}

func (c *BoxConnector) cacheItem(item *BoxItem) {
	if !c.config.CacheEnabled {
		return
	}

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	if item.Type == "file" {
		// Convert to BoxFile for caching
		boxFile := &BoxFile{
			ID:         item.ID,
			Name:       item.Name,
			Size:       item.Size,
			CreatedAt:  item.CreatedAt,
			ModifiedAt: item.ModifiedAt,
			SequenceID: item.SequenceID,
			PathCollection: item.PathCollection,
		}
		c.fileCache[item.ID] = boxFile
	} else if item.Type == "folder" {
		boxFolder := &BoxFolder{
			ID:         item.ID,
			Name:       item.Name,
			CreatedAt:  item.CreatedAt,
			ModifiedAt: item.ModifiedAt,
			SequenceID: item.SequenceID,
			PathCollection: item.PathCollection,
		}
		c.folderCache[item.ID] = boxFolder
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

func (c *BoxConnector) cacheFile(file *BoxFile) {
	if !c.config.CacheEnabled {
		return
	}

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	c.fileCache[file.ID] = file
}

func (c *BoxConnector) getCachedFile(fileID string) *BoxFile {
	if !c.config.CacheEnabled {
		return nil
	}

	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()

	return c.fileCache[fileID]
}

func (c *BoxConnector) executeSyncOperation(ctx context.Context, options *storage.SyncOptions, result *storage.SyncResult, isFullSync bool) error {
	// For Box, we can use the Events API to get incremental changes
	// or traverse folders for full sync
	
	if isFullSync {
		return c.executeFullSync(ctx, options, result)
	} else {
		return c.executeIncrementalSync(ctx, options, result)
	}
}

func (c *BoxConnector) executeFullSync(ctx context.Context, options *storage.SyncOptions, result *storage.SyncResult) error {
	// Traverse all folders starting from root
	return c.traverseFolder(ctx, "0", result)
}

func (c *BoxConnector) executeIncrementalSync(ctx context.Context, options *storage.SyncOptions, result *storage.SyncResult) error {
	// Use Box Events API to get changes since last sync
	// This is a simplified implementation
	
	// For now, fall back to full sync
	return c.executeFullSync(ctx, options, result)
}

func (c *BoxConnector) traverseFolder(ctx context.Context, folderID string, result *storage.SyncResult) error {
	// List all items in folder
	files, err := c.ListFiles(ctx, folderID, &storage.ConnectorListOptions{})
	if err != nil {
		return err
	}
	
	result.FilesFound += int64(len(files))
	result.FilesChanged += int64(len(files)) // For full sync, consider all files as changed
	
	return nil
}

func (c *BoxConnector) executeWithRetry(ctx context.Context, operation func() (interface{}, error)) (interface{}, error) {
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

func (c *BoxConnector) isRetryableError(err error) bool {
	errStr := strings.ToLower(err.Error())
	retryableErrors := []string{
		"rate limit",
		"quota exceeded", 
		"internal error",
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