package googledrive

import (
	"context"
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
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// GoogleDriveConnector implements storage.StorageConnector for Google Drive
type GoogleDriveConnector struct {
	config       *GoogleDriveConfig
	oauthConfig  *oauth2.Config
	driveService *drive.Service
	tracer       trace.Tracer
	
	// Connection state
	isConnected  bool
	lastSync     time.Time
	
	// Rate limiting and throttling
	rateLimiter  *RateLimiter
	
	// Caching
	folderCache  map[string]*drive.File
	fileCache    map[string]*drive.File
	cacheMu      sync.RWMutex
	
	// Sync state
	syncState    *SyncState
	syncMu       sync.RWMutex
	
	// Metrics
	metrics      *ConnectorMetrics
	
	// Error handling
	retryPolicy  *RetryPolicy
}

// GoogleDriveConfig contains configuration for Google Drive connector
type GoogleDriveConfig struct {
	// OAuth2 configuration
	ClientID           string   `yaml:"client_id"`
	ClientSecret       string   `yaml:"client_secret"`
	Scopes            []string `yaml:"scopes"`
	RedirectURL       string   `yaml:"redirect_url"`
	
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
}

// DefaultGoogleDriveConfig returns default configuration
func DefaultGoogleDriveConfig() *GoogleDriveConfig {
	return &GoogleDriveConfig{
		Scopes: []string{
			drive.DriveReadonlyScope,
			drive.DriveMetadataReadonlyScope,
		},
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
	}
}

// NewGoogleDriveConnector creates a new Google Drive connector
func NewGoogleDriveConnector(config *GoogleDriveConfig) *GoogleDriveConnector {
	if config == nil {
		config = DefaultGoogleDriveConfig()
	}

	// Setup OAuth2 configuration
	oauthConfig := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		Scopes:       config.Scopes,
		RedirectURL:  config.RedirectURL,
		Endpoint:     google.Endpoint,
	}

	connector := &GoogleDriveConnector{
		config:      config,
		oauthConfig: oauthConfig,
		tracer:      otel.Tracer("googledrive-connector"),
		folderCache: make(map[string]*drive.File),
		fileCache:   make(map[string]*drive.File),
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

// Connect establishes connection to Google Drive
func (c *GoogleDriveConnector) Connect(ctx context.Context, credentials map[string]interface{}) error {
	ctx, span := c.tracer.Start(ctx, "googledrive_connect")
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

	// Create HTTP client with token
	client := c.oauthConfig.Client(ctx, token)

	// Create Drive service
	driveService, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to create Drive service: %w", err)
	}

	c.driveService = driveService
	c.isConnected = true

	// Test connection
	if err := c.testConnection(ctx); err != nil {
		span.RecordError(err)
		c.isConnected = false
		return fmt.Errorf("connection test failed: %w", err)
	}

	span.SetAttributes(
		attribute.Bool("connected", c.isConnected),
		attribute.String("scopes", strings.Join(c.config.Scopes, ",")),
	)

	return nil
}

// Disconnect closes the connection to Google Drive
func (c *GoogleDriveConnector) Disconnect(ctx context.Context) error {
	ctx, span := c.tracer.Start(ctx, "googledrive_disconnect")
	defer span.End()

	c.isConnected = false
	c.driveService = nil
	
	// Clear caches
	c.cacheMu.Lock()
	c.folderCache = make(map[string]*drive.File)
	c.fileCache = make(map[string]*drive.File)
	c.cacheMu.Unlock()

	return nil
}

// IsConnected returns whether the connector is connected
func (c *GoogleDriveConnector) IsConnected() bool {
	return c.isConnected
}

// ListFiles lists files from Google Drive
func (c *GoogleDriveConnector) ListFiles(ctx context.Context, path string, options *storage.ConnectorListOptions) ([]*storage.ConnectorFileInfo, error) {
	ctx, span := c.tracer.Start(ctx, "googledrive_list_files")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to Google Drive")
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(
		attribute.String("path", path),
	)

	var files []*storage.ConnectorFileInfo
	var pageToken string
	
	for {
		// Wait for rate limit
		if err := c.rateLimiter.Wait(ctx); err != nil {
			span.RecordError(err)
			return nil, err
		}

		// Build query
		query := c.buildFileQuery(path, options)
		
		// Execute API call with retry
		fileList, err := c.executeWithRetry(ctx, func() (interface{}, error) {
			call := c.driveService.Files.List().
				Q(query).
				Fields("nextPageToken, files(id, name, mimeType, size, createdTime, modifiedTime, parents, shared, trashed, webViewLink, webContentLink)").
				PageSize(int64(c.config.BatchSize))
			
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			
			return call.Do()
		})
		
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to list files: %w", err)
		}

		list := fileList.(*drive.FileList)
		
		// Convert Drive files to FileInfo
		for _, file := range list.Files {
			if c.shouldIncludeFile(file) {
				fileInfo := c.convertToFileInfo(file)
				files = append(files, fileInfo)
				
				// Cache file
				if c.config.CacheEnabled {
					c.cacheFile(file)
				}
			}
		}

		pageToken = list.NextPageToken
		if pageToken == "" {
			break
		}
	}

	// Update metrics
	c.metrics.FilesListed += int64(len(files))
	c.metrics.LastListTime = time.Now()

	span.SetAttributes(
		attribute.Int("files.count", len(files)),
	)

	return files, nil
}

// GetFile retrieves a specific file from Google Drive
func (c *GoogleDriveConnector) GetFile(ctx context.Context, fileID string) (*storage.ConnectorFileInfo, error) {
	ctx, span := c.tracer.Start(ctx, "googledrive_get_file")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to Google Drive")
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

	// Execute API call with retry
	fileData, err := c.executeWithRetry(ctx, func() (interface{}, error) {
		return c.driveService.Files.Get(fileID).
			Fields("id, name, mimeType, size, createdTime, modifiedTime, parents, shared, trashed, webViewLink, webContentLink").
			Do()
	})

	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get file: %w", err)
	}

	file := fileData.(*drive.File)
	
	// Cache file
	if c.config.CacheEnabled {
		c.cacheFile(file)
	}

	// Update metrics
	c.metrics.FilesRetrieved++

	return c.convertToFileInfo(file), nil
}

// DownloadFile downloads file content from Google Drive
func (c *GoogleDriveConnector) DownloadFile(ctx context.Context, fileID string) (io.ReadCloser, error) {
	ctx, span := c.tracer.Start(ctx, "googledrive_download_file")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to Google Drive")
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
	resp, err := c.executeWithRetry(ctx, func() (interface{}, error) {
		return c.driveService.Files.Get(fileID).Download()
	})

	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to download file: %w", err)
	}

	response := resp.(*http.Response)

	// Update metrics
	c.metrics.FilesDownloaded++
	c.metrics.BytesDownloaded += response.ContentLength

	return response.Body, nil
}

// SyncFiles performs synchronization with Google Drive
func (c *GoogleDriveConnector) SyncFiles(ctx context.Context, options *storage.SyncOptions) (*storage.SyncResult, error) {
	ctx, span := c.tracer.Start(ctx, "googledrive_sync_files")
	defer span.End()

	if !c.isConnected {
		err := fmt.Errorf("not connected to Google Drive")
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

	var query string
	if isFullSync {
		query = c.buildFullSyncQuery()
	} else {
		query = c.buildIncrementalSyncQuery(options.Since)
	}

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
	pageToken := ""
	for {
		// Wait for rate limit
		if err := c.rateLimiter.Wait(ctx); err != nil {
			span.RecordError(err)
			result.Errors = append(result.Errors, err.Error())
			break
		}

		// Get files from API
		fileList, err := c.executeWithRetry(ctx, func() (interface{}, error) {
			call := c.driveService.Files.List().
				Q(query).
				Fields("nextPageToken, files(id, name, mimeType, size, createdTime, modifiedTime, parents, shared, trashed, webViewLink, webContentLink)").
				PageSize(int64(c.config.BatchSize))
			
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			
			return call.Do()
		})

		if err != nil {
			span.RecordError(err)
			result.Errors = append(result.Errors, err.Error())
			break
		}

		list := fileList.(*drive.FileList)

		// Process files
		for _, file := range list.Files {
			result.FilesFound++
			
			if c.shouldIncludeFile(file) {
				// Check if file is new or changed
				cachedFile := c.getCachedFile(file.Id)
				if cachedFile == nil || c.isFileChanged(cachedFile, file) {
					result.FilesChanged++
					
					// Cache updated file
					if c.config.CacheEnabled {
						c.cacheFile(file)
					}
				}
			}
		}

		pageToken = list.NextPageToken
		if pageToken == "" {
			break
		}
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
func (c *GoogleDriveConnector) GetSyncState(ctx context.Context) (*storage.SyncState, error) {
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
func (c *GoogleDriveConnector) GetMetrics(ctx context.Context) (*storage.ConnectorMetrics, error) {
	return &storage.ConnectorMetrics{
		ConnectorType:     "googledrive",
		IsConnected:       c.isConnected,
		LastConnectionTime: c.metrics.LastConnectionTime,
		FilesListed:       c.metrics.FilesListed,
		FilesRetrieved:    c.metrics.FilesRetrieved,
		FilesDownloaded:   c.metrics.FilesDownloaded,
		BytesDownloaded:   c.metrics.BytesDownloaded,
		SyncCount:         c.metrics.SyncCount,
		LastSyncTime:      c.metrics.LastSyncTime,
		LastSyncDuration:  c.metrics.LastSyncDuration,
		ErrorCount:        c.metrics.ErrorCount,
		LastError:         c.metrics.LastError,
	}, nil
}

// Helper methods

func (c *GoogleDriveConnector) testConnection(ctx context.Context) error {
	// Test connection by getting user info
	_, err := c.driveService.About.Get().Fields("user").Do()
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	
	c.metrics.LastConnectionTime = time.Now()
	return nil
}

func (c *GoogleDriveConnector) buildFileQuery(path string, options *storage.ConnectorListOptions) string {
	var conditions []string

	// Base conditions
	if !c.config.SyncTrashedFiles {
		conditions = append(conditions, "trashed=false")
	}

	// Path-based filtering (if path represents a folder ID)
	if path != "" && path != "/" {
		conditions = append(conditions, fmt.Sprintf("'%s' in parents", path))
	}

	// File type filtering
	if len(c.config.FileExtensions) > 0 {
		var extConditions []string
		for _, ext := range c.config.FileExtensions {
			extConditions = append(extConditions, fmt.Sprintf("name contains '%s'", ext))
		}
		conditions = append(conditions, "("+strings.Join(extConditions, " or ")+")")
	}

	// Size filtering
	if options != nil && options.MaxSize > 0 {
		conditions = append(conditions, fmt.Sprintf("size <= %d", options.MaxSize))
	}

	// Modified time filtering
	if options != nil && !options.ModifiedSince.IsZero() {
		conditions = append(conditions, fmt.Sprintf("modifiedTime > '%s'", options.ModifiedSince.Format(time.RFC3339)))
	}

	if len(conditions) == 0 {
		return ""
	}

	return strings.Join(conditions, " and ")
}

func (c *GoogleDriveConnector) buildFullSyncQuery() string {
	var conditions []string

	if !c.config.SyncTrashedFiles {
		conditions = append(conditions, "trashed=false")
	}

	// Include only specific file types
	if len(c.config.FileExtensions) > 0 {
		var extConditions []string
		for _, ext := range c.config.FileExtensions {
			extConditions = append(extConditions, fmt.Sprintf("name contains '%s'", ext))
		}
		conditions = append(conditions, "("+strings.Join(extConditions, " or ")+")")
	}

	return strings.Join(conditions, " and ")
}

func (c *GoogleDriveConnector) buildIncrementalSyncQuery(since time.Time) string {
	conditions := []string{
		fmt.Sprintf("modifiedTime > '%s'", since.Format(time.RFC3339)),
	}

	if !c.config.SyncTrashedFiles {
		conditions = append(conditions, "trashed=false")
	}

	return strings.Join(conditions, " and ")
}

func (c *GoogleDriveConnector) shouldIncludeFile(file *drive.File) bool {
	// Check file size
	if c.config.MaxFileSize > 0 && file.Size > c.config.MaxFileSize {
		return false
	}

	// Check file extension
	if len(c.config.FileExtensions) > 0 {
		hasValidExtension := false
		for _, ext := range c.config.FileExtensions {
			if strings.HasSuffix(strings.ToLower(file.Name), strings.ToLower(ext)) {
				hasValidExtension = true
				break
			}
		}
		if !hasValidExtension {
			return false
		}
	}

	// Check if trashed files should be included
	if file.Trashed && !c.config.SyncTrashedFiles {
		return false
	}

	// Check if shared files should be included
	if file.Shared && !c.config.SyncSharedFiles {
		return false
	}

	return true
}

func (c *GoogleDriveConnector) convertToFileInfo(file *drive.File) *storage.ConnectorFileInfo {
	// Parse timestamps
	var createdTime, modifiedTime time.Time
	if file.CreatedTime != "" {
		createdTime, _ = time.Parse(time.RFC3339, file.CreatedTime)
	}
	if file.ModifiedTime != "" {
		modifiedTime, _ = time.Parse(time.RFC3339, file.ModifiedTime)
	}

	// Determine file type
	var fileType storage.FileType
	if strings.HasPrefix(file.MimeType, "image/") {
		fileType = storage.FileTypeImage
	} else if strings.HasPrefix(file.MimeType, "video/") {
		fileType = storage.FileTypeVideo
	} else if strings.HasPrefix(file.MimeType, "audio/") {
		fileType = storage.FileTypeAudio
	} else if strings.Contains(file.MimeType, "pdf") {
		fileType = storage.FileTypePDF
	} else if strings.Contains(file.MimeType, "document") || strings.Contains(file.MimeType, "text") {
		fileType = storage.FileTypeDocument
	} else if strings.Contains(file.MimeType, "spreadsheet") {
		fileType = storage.FileTypeSpreadsheet
	} else if strings.Contains(file.MimeType, "presentation") {
		fileType = storage.FileTypePresentation
	} else {
		fileType = storage.FileTypeOther
	}

	// Build path from parents
	path := "/" + file.Name
	if len(file.Parents) > 0 {
		// This would require additional API calls to build full path
		// For now, using a simple approach
		path = "/Drive/" + file.Name
	}

	return &storage.ConnectorFileInfo{
		ID:           file.Id,
		Name:         file.Name,
		Path:         path,
		URL:          file.WebViewLink,
		DownloadURL:  file.WebContentLink,
		Size:         file.Size,
		Type:         fileType,
		MimeType:     file.MimeType,
		CreatedAt:    createdTime,
		ModifiedAt:   modifiedTime,
		LastAccessed: modifiedTime, // Google Drive doesn't track last access
		IsFolder:     file.MimeType == "application/vnd.google-apps.folder",
		Permissions: storage.FilePermissions{
			CanRead:   true,
			CanWrite:  false, // Assuming read-only for now
			CanDelete: false,
		},
		Metadata: map[string]interface{}{
			"drive_id":     file.Id,
			"shared":       file.Shared,
			"trashed":      file.Trashed,
			"mime_type":    file.MimeType,
			"web_view_link": file.WebViewLink,
		},
		Tags: map[string]string{
			"source":    "googledrive",
			"shared":    fmt.Sprintf("%t", file.Shared),
			"mime_type": file.MimeType,
		},
		Source: "googledrive",
	}
}

func (c *GoogleDriveConnector) cacheFile(file *drive.File) {
	if !c.config.CacheEnabled {
		return
	}

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	c.fileCache[file.Id] = file
	
	// Simple cache size management
	if len(c.fileCache) > c.config.CacheSize {
		// Remove oldest entries (simple FIFO, could be improved with LRU)
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

func (c *GoogleDriveConnector) getCachedFile(fileID string) *drive.File {
	if !c.config.CacheEnabled {
		return nil
	}

	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()

	return c.fileCache[fileID]
}

func (c *GoogleDriveConnector) isFileChanged(cached, current *drive.File) bool {
	return cached.ModifiedTime != current.ModifiedTime ||
		cached.Size != current.Size ||
		cached.Name != current.Name
}

func (c *GoogleDriveConnector) executeWithRetry(ctx context.Context, operation func() (interface{}, error)) (interface{}, error) {
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

func (c *GoogleDriveConnector) isRetryableError(err error) bool {
	errStr := strings.ToLower(err.Error())
	retryableErrors := []string{
		"rate limit",
		"quota exceeded",
		"internal error",
		"service unavailable",
		"timeout",
		"connection",
		"network",
	}

	for _, retryable := range retryableErrors {
		if strings.Contains(errStr, retryable) {
			return true
		}
	}

	return false
}