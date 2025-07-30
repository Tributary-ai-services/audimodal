package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jscharber/eAIIngest/pkg/storage"
	"go.opentelemetry.io/otel/trace"
)

// NotionConnector implements StorageConnector for Notion workspaces
type NotionConnector struct {
	config      *NotionConfig
	httpClient  *http.Client
	tracer      trace.Tracer
	rateLimiter *RateLimiter
	cache       *ConnectorCache
	mu          sync.RWMutex
}

// NotionConfig contains Notion API configuration
type NotionConfig struct {
	// Authentication
	IntegrationToken string `json:"integration_token" validate:"required"`
	OAuthToken       string `json:"oauth_token,omitempty"` // For OAuth apps
	
	// Workspace info
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	
	// Sync configuration
	SyncPages      bool     `json:"sync_pages"`
	SyncDatabases  bool     `json:"sync_databases"`
	SyncBlocks     bool     `json:"sync_blocks"`
	SyncUsers      bool     `json:"sync_users"`
	PageFilter     []string `json:"page_filter,omitempty"`     // Specific pages to sync
	DatabaseFilter []string `json:"database_filter,omitempty"` // Specific databases to sync
	
	// Content options
	IncludeChildPages bool   `json:"include_child_pages"`
	IncludeContent    bool   `json:"include_content"`
	MaxDepth          int    `json:"max_depth"` // Maximum page hierarchy depth
	ContentFormat     string `json:"content_format"` // markdown, plain_text, rich_text
	
	// Database options
	SyncDatabaseRows    bool `json:"sync_database_rows"`
	MaxRowsPerDatabase  int  `json:"max_rows_per_database"`
	IncludeProperties   bool `json:"include_properties"`
	
	// Rate limiting
	RequestsPerSecond int `json:"requests_per_second"`
	BurstLimit        int `json:"burst_limit"`
	
	// Webhook configuration for real-time sync
	WebhookURL    string `json:"webhook_url,omitempty"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
	
	// Advanced options
	EnableDataLossPrevention bool `json:"enable_dlp"`
	RetentionDays           int  `json:"retention_days"` // 0 = no retention limit
	ExportFormat            string `json:"export_format"` // json, markdown, html
	IncludeArchived         bool `json:"include_archived"`
}

// RateLimiter handles Notion API rate limiting
type RateLimiter struct {
	tokens    chan struct{}
	refill    time.Duration
	lastRefill time.Time
	mu        sync.Mutex
}

// ConnectorCache provides caching for Notion data
type ConnectorCache struct {
	pages     map[string]*NotionPage
	databases map[string]*NotionDatabase
	users     map[string]*NotionUser
	blocks    map[string][]*NotionBlock
	mu        sync.RWMutex
	ttl       time.Duration
}

// NewNotionConnector creates a new Notion connector
func NewNotionConnector(config *NotionConfig) *NotionConnector {
	// Setup HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:       10,
			IdleConnTimeout:    30 * time.Second,
			DisableCompression: false,
		},
	}
	
	// Setup rate limiter (Notion allows 3 requests per second)
	rps := config.RequestsPerSecond
	if rps == 0 {
		rps = 3 // Notion's default limit
	}
	
	rateLimiter := &RateLimiter{
		tokens: make(chan struct{}, config.BurstLimit),
		refill: time.Second / time.Duration(rps),
	}
	
	// Initialize rate limiter tokens
	for i := 0; i < config.BurstLimit; i++ {
		rateLimiter.tokens <- struct{}{}
	}
	
	// Setup cache
	cache := &ConnectorCache{
		pages:     make(map[string]*NotionPage),
		databases: make(map[string]*NotionDatabase),
		users:     make(map[string]*NotionUser),
		blocks:    make(map[string][]*NotionBlock),
		ttl:       15 * time.Minute,
	}
	
	return &NotionConnector{
		config:      config,
		httpClient:  client,
		rateLimiter: rateLimiter,
		cache:       cache,
	}
}

// Connect establishes connection to Notion workspace
func (c *NotionConnector) Connect(ctx context.Context, credentials map[string]interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Update credentials from map
	if token, ok := credentials["integration_token"].(string); ok && token != "" {
		c.config.IntegrationToken = token
	}
	if oauthToken, ok := credentials["oauth_token"].(string); ok {
		c.config.OAuthToken = oauthToken
	}
	
	// Validate token
	token := c.getAuthToken()
	if token == "" {
		return fmt.Errorf("integration_token or oauth_token is required for Notion connection")
	}
	
	// Test connection by listing users (this validates the token)
	users, err := c.listUsers(ctx)
	if err != nil {
		return fmt.Errorf("failed to authenticate with Notion: %w", err)
	}
	
	// Set workspace info from the first user (workspace owner)
	if len(users) > 0 {
		c.config.WorkspaceName = "Notion Workspace"
		c.config.WorkspaceID = "notion-workspace"
	}
	
	return nil
}

// ListFiles lists files from Notion workspace
func (c *NotionConnector) ListFiles(ctx context.Context, path string, options *storage.ConnectorListOptions) ([]*storage.ConnectorFileInfo, error) {
	var allFiles []*storage.ConnectorFileInfo
	
	// List pages if syncing pages
	if c.config.SyncPages {
		pages, err := c.listPages(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list pages: %w", err)
		}
		
		for _, page := range pages {
			if c.shouldSyncPage(page) {
				file := &storage.ConnectorFileInfo{
					ID:           page.ID,
					Name:         c.getPageTitle(page),
					Path:         fmt.Sprintf("/pages/%s", page.ID),
					Size:         c.estimatePageSize(page),
					ModifiedAt:   parseNotionTimestamp(page.LastEditedTime),
					IsFolder:     c.pageHasChildren(page),
					MimeType:     "application/x-notion-page",
					Metadata: map[string]interface{}{
						"page_id":        page.ID,
						"created_time":   page.CreatedTime,
						"last_edited":    page.LastEditedTime,
						"created_by":     page.CreatedBy.ID,
						"last_edited_by": page.LastEditedBy.ID,
						"archived":       page.Archived,
						"url":            page.URL,
						"public_url":     page.PublicURL,
						"parent_type":    page.Parent.Type,
					},
				}
				allFiles = append(allFiles, file)
			}
		}
	}
	
	// List databases if syncing databases
	if c.config.SyncDatabases {
		databases, err := c.listDatabases(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list databases: %w", err)
		}
		
		for _, database := range databases {
			if c.shouldSyncDatabase(database) {
				file := &storage.ConnectorFileInfo{
					ID:           database.ID,
					Name:         c.getDatabaseTitle(database),
					Path:         fmt.Sprintf("/databases/%s", database.ID),
					Size:         c.estimateDatabaseSize(database),
					ModifiedAt:   parseNotionTimestamp(database.LastEditedTime),
					IsFolder:     true, // Databases contain rows
					MimeType:     "application/x-notion-database",
					Metadata: map[string]interface{}{
						"database_id":    database.ID,
						"created_time":   database.CreatedTime,
						"last_edited":    database.LastEditedTime,
						"created_by":     database.CreatedBy.ID,
						"last_edited_by": database.LastEditedBy.ID,
						"archived":       database.Archived,
						"url":            database.URL,
						"public_url":     database.PublicURL,
						"properties":     len(database.Properties),
					},
				}
				allFiles = append(allFiles, file)
			}
		}
	}
	
	// List users if syncing users
	if c.config.SyncUsers {
		users, err := c.listUsers(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list users: %w", err)
		}
		
		for _, user := range users {
			file := &storage.ConnectorFileInfo{
				ID:           user.ID,
				Name:         user.Name,
				Path:         fmt.Sprintf("/users/%s", user.ID),
				Size:         int64(len(user.Name + user.Email)),
				ModifiedAt:   time.Now(), // Users don't have modification time
				IsFolder:     false,
				MimeType:     "application/x-notion-user",
				Metadata: map[string]interface{}{
					"user_id":    user.ID,
					"type":       user.Type,
					"name":       user.Name,
					"email":      user.Email,
					"avatar_url": user.AvatarURL,
				},
			}
			allFiles = append(allFiles, file)
		}
	}
	
	return allFiles, nil
}

// SyncFiles synchronizes files from Notion workspace
func (c *NotionConnector) SyncFiles(ctx context.Context, options *storage.SyncOptions) (*storage.SyncResult, error) {
	result := &storage.SyncResult{
		StartTime: time.Now(),
		Files:     make(map[string]*storage.SyncFileResult),
	}
	
	// Get list of files to sync
	files, err := c.ListFiles(ctx, "", &storage.ConnectorListOptions{})
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	
	result.TotalFiles = len(files)
	
	// Sync each file
	for _, file := range files {
		fileResult := &storage.SyncFileResult{
			Path:      file.Path,
			StartTime: time.Now(),
		}
		
		switch file.MimeType {
		case "application/x-notion-page":
			err = c.syncPage(ctx, file, fileResult)
		case "application/x-notion-database":
			err = c.syncDatabase(ctx, file, fileResult)
		case "application/x-notion-user":
			err = c.syncUser(ctx, file, fileResult)
		default:
			err = fmt.Errorf("unknown file type: %s", file.MimeType)
		}
		
		fileResult.EndTime = time.Now()
		fileResult.Duration = fileResult.EndTime.Sub(fileResult.StartTime)
		
		if err != nil {
			fileResult.Error = err.Error()
			result.FailedFiles++
		} else {
			result.SyncedFiles++
		}
		
		result.Files[file.Path] = fileResult
	}
	
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	
	return result, nil
}

// callNotionAPI makes authenticated API calls to Notion
func (c *NotionConnector) callNotionAPI(ctx context.Context, method, endpoint string, body io.Reader) ([]byte, error) {
	// Rate limiting
	select {
	case <-c.rateLimiter.tokens:
		defer func() {
			go func() {
				time.Sleep(c.rateLimiter.refill)
				c.rateLimiter.tokens <- struct{}{}
			}()
		}()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	
	// Build request URL
	apiURL := fmt.Sprintf("https://api.notion.com/v1%s", endpoint)
	
	req, err := http.NewRequestWithContext(ctx, method, apiURL, body)
	if err != nil {
		return nil, err
	}
	
	// Add headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAuthToken()))
	req.Header.Set("Notion-Version", "2022-06-28") // Use latest API version
	req.Header.Set("User-Agent", "AudiModal-NotionConnector/1.0")
	
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	
	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	// Handle rate limiting
	if resp.StatusCode == 429 {
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			if seconds, err := strconv.Atoi(retryAfter); err == nil {
				time.Sleep(time.Duration(seconds) * time.Second)
				return c.callNotionAPI(ctx, method, endpoint, body) // Retry
			}
		}
		return nil, fmt.Errorf("rate limited by Notion API")
	}
	
	// Read response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Parse error response
		var errorResp NotionError
		if err := json.Unmarshal(responseBody, &errorResp); err == nil {
			return nil, fmt.Errorf("Notion API error (%d): %s - %s", resp.StatusCode, errorResp.Code, errorResp.Message)
		}
		return nil, fmt.Errorf("Notion API returned status %d", resp.StatusCode)
	}
	
	return responseBody, nil
}

// getAuthToken returns the appropriate authentication token
func (c *NotionConnector) getAuthToken() string {
	if c.config.OAuthToken != "" {
		return c.config.OAuthToken
	}
	return c.config.IntegrationToken
}

// listPages retrieves all pages from the workspace
func (c *NotionConnector) listPages(ctx context.Context) ([]*NotionPage, error) {
	// Check cache first
	c.cache.mu.RLock()
	if len(c.cache.pages) > 0 {
		var pages []*NotionPage
		for _, page := range c.cache.pages {
			pages = append(pages, page)
		}
		c.cache.mu.RUnlock()
		return pages, nil
	}
	c.cache.mu.RUnlock()
	
	var allPages []*NotionPage
	nextCursor := ""
	
	for {
		// Build request body
		requestBody := map[string]interface{}{
			"page_size": 100,
		}
		
		if nextCursor != "" {
			requestBody["start_cursor"] = nextCursor
		}
		
		bodyBytes, err := json.Marshal(requestBody)
		if err != nil {
			return nil, err
		}
		
		resp, err := c.callNotionAPI(ctx, "POST", "/search", strings.NewReader(string(bodyBytes)))
		if err != nil {
			return nil, err
		}
		
		var searchResp struct {
			Object      string        `json:"object"`
			Results     []*NotionPage `json:"results"`
			NextCursor  string        `json:"next_cursor"`
			HasMore     bool          `json:"has_more"`
		}
		
		if err := json.Unmarshal(resp, &searchResp); err != nil {
			return nil, err
		}
		
		// Filter to only include pages (not databases)
		for _, result := range searchResp.Results {
			if result.Object == "page" {
				allPages = append(allPages, result)
			}
		}
		
		if !searchResp.HasMore {
			break
		}
		
		nextCursor = searchResp.NextCursor
	}
	
	// Cache pages
	c.cache.mu.Lock()
	for _, page := range allPages {
		c.cache.pages[page.ID] = page
	}
	c.cache.mu.Unlock()
	
	return allPages, nil
}

// listDatabases retrieves all databases from the workspace
func (c *NotionConnector) listDatabases(ctx context.Context) ([]*NotionDatabase, error) {
	// Check cache first
	c.cache.mu.RLock()
	if len(c.cache.databases) > 0 {
		var databases []*NotionDatabase
		for _, db := range c.cache.databases {
			databases = append(databases, db)
		}
		c.cache.mu.RUnlock()
		return databases, nil
	}
	c.cache.mu.RUnlock()
	
	var allDatabases []*NotionDatabase
	nextCursor := ""
	
	for {
		// Build request body for database search
		requestBody := map[string]interface{}{
			"filter": map[string]interface{}{
				"value":    "database",
				"property": "object",
			},
			"page_size": 100,
		}
		
		if nextCursor != "" {
			requestBody["start_cursor"] = nextCursor
		}
		
		bodyBytes, err := json.Marshal(requestBody)
		if err != nil {
			return nil, err
		}
		
		resp, err := c.callNotionAPI(ctx, "POST", "/search", strings.NewReader(string(bodyBytes)))
		if err != nil {
			return nil, err
		}
		
		var searchResp struct {
			Object      string            `json:"object"`
			Results     []*NotionDatabase `json:"results"`
			NextCursor  string            `json:"next_cursor"`
			HasMore     bool              `json:"has_more"`
		}
		
		if err := json.Unmarshal(resp, &searchResp); err != nil {
			return nil, err
		}
		
		allDatabases = append(allDatabases, searchResp.Results...)
		
		if !searchResp.HasMore {
			break
		}
		
		nextCursor = searchResp.NextCursor
	}
	
	// Cache databases
	c.cache.mu.Lock()
	for _, db := range allDatabases {
		c.cache.databases[db.ID] = db
	}
	c.cache.mu.Unlock()
	
	return allDatabases, nil
}

// listUsers retrieves all users from the workspace
func (c *NotionConnector) listUsers(ctx context.Context) ([]*NotionUser, error) {
	// Check cache first
	c.cache.mu.RLock()
	if len(c.cache.users) > 0 {
		var users []*NotionUser
		for _, user := range c.cache.users {
			users = append(users, user)
		}
		c.cache.mu.RUnlock()
		return users, nil
	}
	c.cache.mu.RUnlock()
	
	var allUsers []*NotionUser
	nextCursor := ""
	
	for {
		endpoint := "/users?page_size=100"
		if nextCursor != "" {
			endpoint += "&start_cursor=" + url.QueryEscape(nextCursor)
		}
		
		resp, err := c.callNotionAPI(ctx, "GET", endpoint, nil)
		if err != nil {
			return nil, err
		}
		
		var usersResp struct {
			Object      string        `json:"object"`
			Results     []*NotionUser `json:"results"`
			NextCursor  string        `json:"next_cursor"`
			HasMore     bool          `json:"has_more"`
		}
		
		if err := json.Unmarshal(resp, &usersResp); err != nil {
			return nil, err
		}
		
		allUsers = append(allUsers, usersResp.Results...)
		
		if !usersResp.HasMore {
			break
		}
		
		nextCursor = usersResp.NextCursor
	}
	
	// Cache users
	c.cache.mu.Lock()
	for _, user := range allUsers {
		c.cache.users[user.ID] = user
	}
	c.cache.mu.Unlock()
	
	return allUsers, nil
}

// shouldSyncPage determines if a page should be synced
func (c *NotionConnector) shouldSyncPage(page *NotionPage) bool {
	// Skip archived pages if not including archived
	if !c.config.IncludeArchived && page.Archived {
		return false
	}
	
	// Check page filter
	if len(c.config.PageFilter) > 0 {
		for _, filterPage := range c.config.PageFilter {
			if page.ID == filterPage || c.getPageTitle(page) == filterPage {
				return true
			}
		}
		return false
	}
	
	return true
}

// shouldSyncDatabase determines if a database should be synced
func (c *NotionConnector) shouldSyncDatabase(database *NotionDatabase) bool {
	// Skip archived databases if not including archived
	if !c.config.IncludeArchived && database.Archived {
		return false
	}
	
	// Check database filter
	if len(c.config.DatabaseFilter) > 0 {
		for _, filterDB := range c.config.DatabaseFilter {
			if database.ID == filterDB || c.getDatabaseTitle(database) == filterDB {
				return true
			}
		}
		return false
	}
	
	return true
}

// syncPage syncs a specific page
func (c *NotionConnector) syncPage(ctx context.Context, pageFile *storage.ConnectorFileInfo, result *storage.SyncFileResult) error {
	pageID := pageFile.ID
	
	// Get page blocks if syncing content
	if c.config.IncludeContent {
		blocks, err := c.getPageBlocks(ctx, pageID)
		if err != nil {
			return err
		}
		result.ProcessedBytes = int64(len(blocks) * 100) // Estimate
	}
	
	result.Status = "completed"
	return nil
}

// syncDatabase syncs a specific database
func (c *NotionConnector) syncDatabase(ctx context.Context, dbFile *storage.ConnectorFileInfo, result *storage.SyncFileResult) error {
	databaseID := dbFile.ID
	
	// Get database rows if syncing rows
	if c.config.SyncDatabaseRows {
		rows, err := c.getDatabaseRows(ctx, databaseID)
		if err != nil {
			return err
		}
		result.ProcessedBytes = int64(len(rows) * 200) // Estimate
	}
	
	result.Status = "completed"
	return nil
}

// syncUser syncs user information
func (c *NotionConnector) syncUser(ctx context.Context, userFile *storage.ConnectorFileInfo, result *storage.SyncFileResult) error {
	result.ProcessedBytes = userFile.Size
	result.Status = "completed"
	return nil
}

// getPageBlocks retrieves blocks for a page
func (c *NotionConnector) getPageBlocks(ctx context.Context, pageID string) ([]*NotionBlock, error) {
	// Check cache first
	c.cache.mu.RLock()
	if blocks, exists := c.cache.blocks[pageID]; exists {
		c.cache.mu.RUnlock()
		return blocks, nil
	}
	c.cache.mu.RUnlock()
	
	var allBlocks []*NotionBlock
	nextCursor := ""
	
	for {
		endpoint := fmt.Sprintf("/blocks/%s/children?page_size=100", pageID)
		if nextCursor != "" {
			endpoint += "&start_cursor=" + url.QueryEscape(nextCursor)
		}
		
		resp, err := c.callNotionAPI(ctx, "GET", endpoint, nil)
		if err != nil {
			return nil, err
		}
		
		var blocksResp struct {
			Object      string         `json:"object"`
			Results     []*NotionBlock `json:"results"`
			NextCursor  string         `json:"next_cursor"`
			HasMore     bool           `json:"has_more"`
		}
		
		if err := json.Unmarshal(resp, &blocksResp); err != nil {
			return nil, err
		}
		
		allBlocks = append(allBlocks, blocksResp.Results...)
		
		if !blocksResp.HasMore {
			break
		}
		
		nextCursor = blocksResp.NextCursor
	}
	
	// Cache blocks
	c.cache.mu.Lock()
	c.cache.blocks[pageID] = allBlocks
	c.cache.mu.Unlock()
	
	return allBlocks, nil
}

// getDatabaseRows retrieves rows from a database
func (c *NotionConnector) getDatabaseRows(ctx context.Context, databaseID string) ([]*NotionPage, error) {
	var allRows []*NotionPage
	nextCursor := ""
	
	for {
		requestBody := map[string]interface{}{
			"page_size": 100,
		}
		
		if nextCursor != "" {
			requestBody["start_cursor"] = nextCursor
		}
		
		bodyBytes, err := json.Marshal(requestBody)
		if err != nil {
			return nil, err
		}
		
		endpoint := fmt.Sprintf("/databases/%s/query", databaseID)
		resp, err := c.callNotionAPI(ctx, "POST", endpoint, strings.NewReader(string(bodyBytes)))
		if err != nil {
			return nil, err
		}
		
		var queryResp struct {
			Object      string        `json:"object"`
			Results     []*NotionPage `json:"results"`
			NextCursor  string        `json:"next_cursor"`
			HasMore     bool          `json:"has_more"`
		}
		
		if err := json.Unmarshal(resp, &queryResp); err != nil {
			return nil, err
		}
		
		allRows = append(allRows, queryResp.Results...)
		
		if !queryResp.HasMore {
			break
		}
		
		nextCursor = queryResp.NextCursor
		
		// Respect max rows limit
		if c.config.MaxRowsPerDatabase > 0 && len(allRows) >= c.config.MaxRowsPerDatabase {
			break
		}
	}
	
	return allRows, nil
}

// Helper functions
func (c *NotionConnector) getPageTitle(page *NotionPage) string {
	if page.Properties != nil {
		if title, exists := page.Properties["title"]; exists {
			if titleObj, ok := title.(map[string]interface{}); ok {
				if titleArray, ok := titleObj["title"].([]interface{}); ok && len(titleArray) > 0 {
					if firstTitle, ok := titleArray[0].(map[string]interface{}); ok {
						if text, ok := firstTitle["text"].(map[string]interface{}); ok {
							if content, ok := text["content"].(string); ok {
								return content
							}
						}
					}
				}
			}
		}
	}
	return "Untitled"
}

func (c *NotionConnector) getDatabaseTitle(database *NotionDatabase) string {
	if len(database.Title) > 0 {
		return database.Title[0].Text.Content
	}
	return "Untitled Database"
}

func (c *NotionConnector) estimatePageSize(page *NotionPage) int64 {
	// Rough estimate based on page properties and title
	size := int64(len(c.getPageTitle(page)) * 10) // Assume average content multiplier
	if page.Properties != nil {
		size += int64(len(page.Properties) * 50) // Estimate property size
	}
	return size
}

func (c *NotionConnector) estimateDatabaseSize(database *NotionDatabase) int64 {
	// Rough estimate based on database properties
	size := int64(len(c.getDatabaseTitle(database)) * 10)
	size += int64(len(database.Properties) * 100) // Properties are more complex
	return size
}

func (c *NotionConnector) pageHasChildren(page *NotionPage) bool {
	// In a real implementation, you would check if the page has child blocks
	return false // Simplified for now
}

// parseNotionTimestamp converts Notion timestamp to time.Time
func parseNotionTimestamp(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	
	// Notion timestamps are in ISO 8601 format
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t
	}
	
	return time.Time{}
}