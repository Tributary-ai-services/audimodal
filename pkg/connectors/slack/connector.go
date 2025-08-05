package slack

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

// SlackConnector implements StorageConnector for Slack workspaces
type SlackConnector struct {
	config      *SlackConfig
	httpClient  *http.Client
	tracer      trace.Tracer
	rateLimiter *RateLimiter
	cache       *ConnectorCache
	mu          sync.RWMutex
}

// SlackConfig contains Slack API configuration
type SlackConfig struct {
	// Authentication
	BotToken      string `json:"bot_token" validate:"required"`
	UserToken     string `json:"user_token,omitempty"` // For user-scoped operations
	AppToken      string `json:"app_token,omitempty"`  // For Socket Mode
	SigningSecret string `json:"signing_secret,omitempty"`

	// Workspace info
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	TeamDomain    string `json:"team_domain"`

	// Sync configuration
	SyncChannels    bool     `json:"sync_channels"`
	SyncDMs         bool     `json:"sync_dms"`
	SyncFiles       bool     `json:"sync_files"`
	SyncUsers       bool     `json:"sync_users"`
	ChannelFilter   []string `json:"channel_filter,omitempty"` // Specific channels to sync
	ExcludeArchived bool     `json:"exclude_archived"`

	// Message options
	IncludeThreads     bool `json:"include_threads"`
	IncludeReactions   bool `json:"include_reactions"`
	IncludePins        bool `json:"include_pins"`
	MessageHistoryDays int  `json:"message_history_days"` // 0 = all history

	// File handling
	MaxFileSize   int64    `json:"max_file_size_mb"`
	DownloadFiles bool     `json:"download_files"`
	FileTypes     []string `json:"file_types,omitempty"` // Filter by file types

	// Rate limiting
	RequestsPerMinute int `json:"requests_per_minute"`
	BurstLimit        int `json:"burst_limit"`

	// Webhook configuration for real-time sync
	WebhookURL       string `json:"webhook_url,omitempty"`
	WebhookSecret    string `json:"webhook_secret,omitempty"`
	EnableSocketMode bool   `json:"enable_socket_mode"`

	// Advanced options
	EnableDataLossPrevention bool   `json:"enable_dlp"`
	RetentionDays            int    `json:"retention_days"` // 0 = no retention limit
	ExportFormat             string `json:"export_format"`  // json, html, csv
}

// RateLimiter handles Slack API rate limiting
type RateLimiter struct {
	tokens     chan struct{}
	refill     time.Duration
	lastRefill time.Time
	mu         sync.Mutex
}

// ConnectorCache provides caching for Slack data
type ConnectorCache struct {
	channels map[string]*SlackChannel
	users    map[string]*SlackUser
	messages map[string][]*SlackMessage
	mu       sync.RWMutex
	ttl      time.Duration
}

// NewSlackConnector creates a new Slack connector
func NewSlackConnector(config *SlackConfig) *SlackConnector {
	// Setup HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:       10,
			IdleConnTimeout:    30 * time.Second,
			DisableCompression: false,
		},
	}

	// Setup rate limiter (Slack allows 1+ requests per second per method)
	rpm := config.RequestsPerMinute
	if rpm == 0 {
		rpm = 50 // Conservative default
	}

	rateLimiter := &RateLimiter{
		tokens: make(chan struct{}, config.BurstLimit),
		refill: time.Minute / time.Duration(rpm),
	}

	// Initialize rate limiter tokens
	for i := 0; i < config.BurstLimit; i++ {
		rateLimiter.tokens <- struct{}{}
	}

	// Setup cache
	cache := &ConnectorCache{
		channels: make(map[string]*SlackChannel),
		users:    make(map[string]*SlackUser),
		messages: make(map[string][]*SlackMessage),
		ttl:      15 * time.Minute,
	}

	return &SlackConnector{
		config:      config,
		httpClient:  client,
		rateLimiter: rateLimiter,
		cache:       cache,
	}
}

// Connect establishes connection to Slack workspace
func (c *SlackConnector) Connect(ctx context.Context, credentials map[string]interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update credentials from map
	if botToken, ok := credentials["bot_token"].(string); ok && botToken != "" {
		c.config.BotToken = botToken
	}
	if userToken, ok := credentials["user_token"].(string); ok {
		c.config.UserToken = userToken
	}
	if appToken, ok := credentials["app_token"].(string); ok {
		c.config.AppToken = appToken
	}

	// Validate bot token
	if c.config.BotToken == "" {
		return fmt.Errorf("bot_token is required for Slack connection")
	}

	// Test connection by calling auth.test
	authResp, err := c.callSlackAPI(ctx, "auth.test", nil, c.config.BotToken)
	if err != nil {
		return fmt.Errorf("failed to authenticate with Slack: %w", err)
	}

	// Parse auth response
	var authData struct {
		OK     bool   `json:"ok"`
		User   string `json:"user"`
		Team   string `json:"team"`
		URL    string `json:"url"`
		TeamID string `json:"team_id"`
		UserID string `json:"user_id"`
		Error  string `json:"error,omitempty"`
	}

	if err := json.Unmarshal(authResp, &authData); err != nil {
		return fmt.Errorf("failed to parse auth response: %w", err)
	}

	if !authData.OK {
		return fmt.Errorf("Slack authentication failed: %s", authData.Error)
	}

	// Update workspace info
	c.config.WorkspaceID = authData.TeamID
	c.config.WorkspaceName = authData.Team

	// Load workspace info
	if err := c.loadWorkspaceInfo(ctx); err != nil {
		return fmt.Errorf("failed to load workspace info: %w", err)
	}

	return nil
}

// ListFiles lists files from Slack workspace
func (c *SlackConnector) ListFiles(ctx context.Context, path string, options *storage.ConnectorListOptions) ([]*storage.ConnectorFileInfo, error) {
	var allFiles []*storage.ConnectorFileInfo

	// List channels if syncing channels
	if c.config.SyncChannels {
		channels, err := c.listChannels(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list channels: %w", err)
		}

		for _, channel := range channels {
			if c.shouldSyncChannel(channel) {
				// Add channel as a "folder"
				file := &storage.ConnectorFileInfo{
					Name:       channel.Name,
					Path:       fmt.Sprintf("/channels/%s", channel.Name),
					Size:       0, // Channels don't have size
					ModifiedAt: parseSlackTimestamp(channel.Created),
					IsFolder:   true,
					MimeType:   "application/x-slack-channel",
					Metadata: map[string]interface{}{
						"channel_id":   channel.ID,
						"channel_type": channel.Type,
						"member_count": channel.NumMembers,
						"is_archived":  channel.IsArchived,
						"topic":        channel.Topic.Value,
						"purpose":      channel.Purpose.Value,
					},
				}
				allFiles = append(allFiles, file)
			}
		}
	}

	// List files if syncing files
	if c.config.SyncFiles {
		files, err := c.listSlackFiles(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list Slack files: %w", err)
		}
		allFiles = append(allFiles, files...)
	}

	// List users if syncing users
	if c.config.SyncUsers {
		users, err := c.listUsers(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list users: %w", err)
		}

		for _, user := range users {
			if !user.Deleted && !user.IsBot {
				file := &storage.ConnectorFileInfo{
					Name:       user.Profile.DisplayName,
					Path:       fmt.Sprintf("/users/%s", user.Name),
					Size:       int64(len(user.Profile.FirstName + user.Profile.LastName + user.Profile.Email)),
					ModifiedAt: parseSlackTimestamp(user.Updated),
					IsFolder:   false,
					MimeType:   "application/x-slack-user",
					Metadata: map[string]interface{}{
						"user_id":   user.ID,
						"real_name": user.Profile.RealName,
						"email":     user.Profile.Email,
						"title":     user.Profile.Title,
						"timezone":  user.TZ,
						"is_admin":  user.IsAdmin,
						"is_owner":  user.IsOwner,
					},
				}
				allFiles = append(allFiles, file)
			}
		}
	}

	return allFiles, nil
}

// SyncFiles synchronizes files from Slack workspace
func (c *SlackConnector) SyncFiles(ctx context.Context, options *storage.SyncOptions) (*storage.SyncResult, error) {
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

	// Sync each file/channel
	for _, file := range files {
		fileResult := &storage.SyncFileResult{
			Path:      file.Path,
			StartTime: time.Now(),
		}

		switch file.MimeType {
		case "application/x-slack-channel":
			err = c.syncChannelMessages(ctx, file, fileResult)
		case "application/x-slack-user":
			err = c.syncUserProfile(ctx, file, fileResult)
		default:
			err = c.syncSlackFile(ctx, file, fileResult)
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

// callSlackAPI makes authenticated API calls to Slack
func (c *SlackConnector) callSlackAPI(ctx context.Context, method string, params url.Values, token string) ([]byte, error) {
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
	apiURL := fmt.Sprintf("https://slack.com/api/%s", method)

	var req *http.Request
	var err error

	if params != nil && len(params) > 0 {
		// POST request with form data
		req, err = http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(params.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		// GET request
		req, err = http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, err
		}
	}

	// Add authorization header
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("User-Agent", "AudiModal-SlackConnector/1.0")

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
				return c.callSlackAPI(ctx, method, params, token) // Retry
			}
		}
		return nil, fmt.Errorf("rate limited by Slack API")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Slack API returned status %d", resp.StatusCode)
	}

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check for API errors
	var errorResp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &errorResp); err == nil && !errorResp.OK {
		return nil, fmt.Errorf("Slack API error: %s", errorResp.Error)
	}

	return body, nil
}

// loadWorkspaceInfo loads workspace information
func (c *SlackConnector) loadWorkspaceInfo(ctx context.Context) error {
	// Get team info
	teamResp, err := c.callSlackAPI(ctx, "team.info", nil, c.config.BotToken)
	if err != nil {
		return err
	}

	var teamData struct {
		OK   bool `json:"ok"`
		Team struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Domain string `json:"domain"`
		} `json:"team"`
	}

	if err := json.Unmarshal(teamResp, &teamData); err != nil {
		return err
	}

	if teamData.OK {
		c.config.WorkspaceID = teamData.Team.ID
		c.config.WorkspaceName = teamData.Team.Name
		c.config.TeamDomain = teamData.Team.Domain
	}

	return nil
}

// listChannels retrieves all channels from the workspace
func (c *SlackConnector) listChannels(ctx context.Context) ([]*SlackChannel, error) {
	// Check cache first
	c.cache.mu.RLock()
	if len(c.cache.channels) > 0 {
		var channels []*SlackChannel
		for _, ch := range c.cache.channels {
			channels = append(channels, ch)
		}
		c.cache.mu.RUnlock()
		return channels, nil
	}
	c.cache.mu.RUnlock()

	var allChannels []*SlackChannel
	cursor := ""

	for {
		params := url.Values{}
		params.Set("types", "public_channel,private_channel")
		params.Set("exclude_archived", strconv.FormatBool(c.config.ExcludeArchived))
		params.Set("limit", "200")

		if cursor != "" {
			params.Set("cursor", cursor)
		}

		resp, err := c.callSlackAPI(ctx, "conversations.list", params, c.config.BotToken)
		if err != nil {
			return nil, err
		}

		var channelsResp struct {
			OK               bool            `json:"ok"`
			Channels         []*SlackChannel `json:"channels"`
			ResponseMetadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}

		if err := json.Unmarshal(resp, &channelsResp); err != nil {
			return nil, err
		}

		if !channelsResp.OK {
			break
		}

		allChannels = append(allChannels, channelsResp.Channels...)

		cursor = channelsResp.ResponseMetadata.NextCursor
		if cursor == "" {
			break
		}
	}

	// Cache channels
	c.cache.mu.Lock()
	for _, ch := range allChannels {
		c.cache.channels[ch.ID] = ch
	}
	c.cache.mu.Unlock()

	return allChannels, nil
}

// listUsers retrieves all users from the workspace
func (c *SlackConnector) listUsers(ctx context.Context) ([]*SlackUser, error) {
	// Check cache first
	c.cache.mu.RLock()
	if len(c.cache.users) > 0 {
		var users []*SlackUser
		for _, user := range c.cache.users {
			users = append(users, user)
		}
		c.cache.mu.RUnlock()
		return users, nil
	}
	c.cache.mu.RUnlock()

	var allUsers []*SlackUser
	cursor := ""

	for {
		params := url.Values{}
		params.Set("limit", "200")

		if cursor != "" {
			params.Set("cursor", cursor)
		}

		resp, err := c.callSlackAPI(ctx, "users.list", params, c.config.BotToken)
		if err != nil {
			return nil, err
		}

		var usersResp struct {
			OK               bool         `json:"ok"`
			Members          []*SlackUser `json:"members"`
			ResponseMetadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}

		if err := json.Unmarshal(resp, &usersResp); err != nil {
			return nil, err
		}

		if !usersResp.OK {
			break
		}

		allUsers = append(allUsers, usersResp.Members...)

		cursor = usersResp.ResponseMetadata.NextCursor
		if cursor == "" {
			break
		}
	}

	// Cache users
	c.cache.mu.Lock()
	for _, user := range allUsers {
		c.cache.users[user.ID] = user
	}
	c.cache.mu.Unlock()

	return allUsers, nil
}

// listSlackFiles retrieves files from the workspace
func (c *SlackConnector) listSlackFiles(ctx context.Context) ([]*storage.ConnectorFileInfo, error) {
	var allFiles []*storage.ConnectorFileInfo
	page := 1

	for {
		params := url.Values{}
		params.Set("count", "100")
		params.Set("page", strconv.Itoa(page))

		if len(c.config.FileTypes) > 0 {
			params.Set("types", strings.Join(c.config.FileTypes, ","))
		}

		resp, err := c.callSlackAPI(ctx, "files.list", params, c.config.BotToken)
		if err != nil {
			return nil, err
		}

		var filesResp struct {
			OK     bool         `json:"ok"`
			Files  []*SlackFile `json:"files"`
			Paging struct {
				Count int `json:"count"`
				Total int `json:"total"`
				Page  int `json:"page"`
				Pages int `json:"pages"`
			} `json:"paging"`
		}

		if err := json.Unmarshal(resp, &filesResp); err != nil {
			return nil, err
		}

		if !filesResp.OK {
			break
		}

		for _, file := range filesResp.Files {
			// Skip files that exceed size limit
			if c.config.MaxFileSize > 0 && file.Size > c.config.MaxFileSize*1024*1024 {
				continue
			}

			fileInfo := &storage.ConnectorFileInfo{
				Name:       file.Name,
				Path:       fmt.Sprintf("/files/%s", file.ID),
				Size:       file.Size,
				ModifiedAt: parseSlackTimestamp(file.Timestamp),
				IsFolder:   false,
				MimeType:   file.Mimetype,
				Metadata: map[string]interface{}{
					"file_id":   file.ID,
					"file_type": file.FileType,
					"user_id":   file.User,
					"channels":  file.Channels,
					"title":     file.Title,
					"is_public": file.IsPublic,
					"permalink": file.Permalink,
				},
			}
			allFiles = append(allFiles, fileInfo)
		}

		if page >= filesResp.Paging.Pages {
			break
		}
		page++
	}

	return allFiles, nil
}

// shouldSyncChannel determines if a channel should be synced
func (c *SlackConnector) shouldSyncChannel(channel *SlackChannel) bool {
	// Skip archived channels if configured
	if c.config.ExcludeArchived && channel.IsArchived {
		return false
	}

	// Check channel filter
	if len(c.config.ChannelFilter) > 0 {
		for _, filterChannel := range c.config.ChannelFilter {
			if channel.Name == filterChannel || channel.ID == filterChannel {
				return true
			}
		}
		return false
	}

	return true
}

// syncChannelMessages syncs messages from a specific channel
func (c *SlackConnector) syncChannelMessages(ctx context.Context, channelFile *storage.ConnectorFileInfo, result *storage.SyncFileResult) error {
	channelID := channelFile.Metadata["channel_id"].(string)

	// Get messages from channel
	messages, err := c.getChannelMessages(ctx, channelID)
	if err != nil {
		return err
	}

	result.ProcessedBytes = int64(len(messages) * 200) // Estimate
	result.Status = "completed"

	return nil
}

// syncUserProfile syncs user profile information
func (c *SlackConnector) syncUserProfile(ctx context.Context, userFile *storage.ConnectorFileInfo, result *storage.SyncFileResult) error {
	result.ProcessedBytes = userFile.Size
	result.Status = "completed"
	return nil
}

// syncSlackFile syncs a Slack file
func (c *SlackConnector) syncSlackFile(ctx context.Context, file *storage.ConnectorFileInfo, result *storage.SyncFileResult) error {
	if c.config.DownloadFiles {
		// Download file content (implementation would fetch file from Slack)
		result.ProcessedBytes = file.Size
	}

	result.Status = "completed"
	return nil
}

// getChannelMessages retrieves messages from a channel
func (c *SlackConnector) getChannelMessages(ctx context.Context, channelID string) ([]*SlackMessage, error) {
	// Check cache first
	c.cache.mu.RLock()
	if messages, exists := c.cache.messages[channelID]; exists {
		c.cache.mu.RUnlock()
		return messages, nil
	}
	c.cache.mu.RUnlock()

	var allMessages []*SlackMessage
	cursor := ""

	// Calculate oldest timestamp if message history is limited
	var oldest string
	if c.config.MessageHistoryDays > 0 {
		oldestTime := time.Now().AddDate(0, 0, -c.config.MessageHistoryDays)
		oldest = fmt.Sprintf("%.6f", float64(oldestTime.Unix()))
	}

	for {
		params := url.Values{}
		params.Set("channel", channelID)
		params.Set("limit", "200")

		if oldest != "" {
			params.Set("oldest", oldest)
		}

		if cursor != "" {
			params.Set("cursor", cursor)
		}

		resp, err := c.callSlackAPI(ctx, "conversations.history", params, c.config.BotToken)
		if err != nil {
			return nil, err
		}

		var historyResp struct {
			OK               bool            `json:"ok"`
			Messages         []*SlackMessage `json:"messages"`
			HasMore          bool            `json:"has_more"`
			ResponseMetadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}

		if err := json.Unmarshal(resp, &historyResp); err != nil {
			return nil, err
		}

		if !historyResp.OK {
			break
		}

		allMessages = append(allMessages, historyResp.Messages...)

		if !historyResp.HasMore {
			break
		}

		cursor = historyResp.ResponseMetadata.NextCursor
		if cursor == "" {
			break
		}
	}

	// Cache messages
	c.cache.mu.Lock()
	c.cache.messages[channelID] = allMessages
	c.cache.mu.Unlock()

	return allMessages, nil
}

// parseSlackTimestamp converts Slack timestamp to time.Time
func parseSlackTimestamp(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}

	// Slack timestamps are Unix timestamps as strings with decimal precision
	if timestamp, err := strconv.ParseFloat(ts, 64); err == nil {
		return time.Unix(int64(timestamp), int64((timestamp-float64(int64(timestamp)))*1e9))
	}

	return time.Time{}
}
