package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NotionAuthenticator handles Notion OAuth authentication
type NotionAuthenticator struct {
	clientID     string
	clientSecret string
	redirectURI  string
	httpClient   *http.Client
}

// NewNotionAuthenticator creates a new Notion authenticator
func NewNotionAuthenticator(config *NotionOAuthConfig) *NotionAuthenticator {
	return &NotionAuthenticator{
		clientID:     config.ClientID,
		clientSecret: config.ClientSecret,
		redirectURI:  config.RedirectURI,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetAuthorizationURL generates the OAuth authorization URL
func (auth *NotionAuthenticator) GetAuthorizationURL(state string, ownerType string) string {
	params := url.Values{}
	params.Set("client_id", auth.clientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", auth.redirectURI)

	if state != "" {
		params.Set("state", state)
	}

	if ownerType != "" {
		params.Set("owner", ownerType) // user or workspace
	}

	return "https://api.notion.com/v1/oauth/authorize?" + params.Encode()
}

// ExchangeCodeForToken exchanges authorization code for access token
func (auth *NotionAuthenticator) ExchangeCodeForToken(ctx context.Context, code string) (*NotionOAuthToken, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", auth.redirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.notion.com/v1/oauth/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	// Use basic auth for client credentials
	req.SetBasicAuth(auth.clientID, auth.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := auth.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		json.NewDecoder(resp.Body).Decode(&errorResp)
		return nil, fmt.Errorf("OAuth error: %s - %s", errorResp.Error, errorResp.ErrorDescription)
	}

	var tokenResp NotionOAuthToken
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

// ValidateToken validates an access token
func (auth *NotionAuthenticator) ValidateToken(ctx context.Context, token string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.notion.com/v1/users/me", nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Notion-Version", "2022-06-28")

	resp, err := auth.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token validation failed with status %d", resp.StatusCode)
	}

	return nil
}

// GetRequiredScopes returns the recommended scopes for the Notion connector
func GetRequiredScopes() []string {
	// Notion doesn't use traditional OAuth scopes like other platforms
	// Instead, it uses granular permissions that are configured per integration
	return []string{
		"read_content",   // Read pages, databases, and their content
		"read_user_data", // Read user information
		"read_comments",  // Read comments (if available)
	}
}

// NotionIntegrationManager manages Notion integration tokens
type NotionIntegrationManager struct {
	integrationToken string
	capabilities     []string
	workspaceID      string
	botUser          *NotionUser
}

// NewNotionIntegrationManager creates a new integration manager
func NewNotionIntegrationManager(integrationToken string) *NotionIntegrationManager {
	return &NotionIntegrationManager{
		integrationToken: integrationToken,
	}
}

// Initialize initializes the integration and loads capabilities
func (nim *NotionIntegrationManager) Initialize(ctx context.Context) error {
	// Test the integration token by making a request to the users endpoint
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.notion.com/v1/users/me", nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", nim.integrationToken))
	req.Header.Set("Notion-Version", "2022-06-28")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("integration token validation failed: status %d", resp.StatusCode)
	}

	var botUser NotionUser
	if err := json.NewDecoder(resp.Body).Decode(&botUser); err != nil {
		return err
	}

	nim.botUser = &botUser

	// Set default capabilities for integration tokens
	nim.capabilities = []string{
		"read_content",
		"read_user_data",
		"insert_content", // Some integrations can create content
		"update_content", // Some integrations can update content
	}

	return nil
}

// GetCapabilities returns the integration's capabilities
func (nim *NotionIntegrationManager) GetCapabilities() []string {
	return nim.capabilities
}

// GetBotUser returns the bot user information
func (nim *NotionIntegrationManager) GetBotUser() *NotionUser {
	return nim.botUser
}

// NotionPermissionValidator validates that the connector has required permissions
type NotionPermissionValidator struct {
	connector *NotionConnector
}

// NewNotionPermissionValidator creates a new permission validator
func NewNotionPermissionValidator(connector *NotionConnector) *NotionPermissionValidator {
	return &NotionPermissionValidator{
		connector: connector,
	}
}

// ValidatePermissions validates that the connector has all required permissions
func (npv *NotionPermissionValidator) ValidatePermissions(ctx context.Context) error {
	// Test basic API access
	if err := npv.validateBasicAccess(ctx); err != nil {
		return fmt.Errorf("basic access validation failed: %w", err)
	}

	// Test content reading permissions
	if err := npv.validateContentAccess(ctx); err != nil {
		return fmt.Errorf("content access validation failed: %w", err)
	}

	// Test user reading permissions
	if err := npv.validateUserAccess(ctx); err != nil {
		return fmt.Errorf("user access validation failed: %w", err)
	}

	return nil
}

// validateBasicAccess validates basic API access
func (npv *NotionPermissionValidator) validateBasicAccess(ctx context.Context) error {
	_, err := npv.connector.callNotionAPI(ctx, "GET", "/users/me", nil)
	return err
}

// validateContentAccess validates content reading permissions
func (npv *NotionPermissionValidator) validateContentAccess(ctx context.Context) error {
	// Try to search for content
	searchBody := map[string]interface{}{
		"page_size": 1,
	}

	bodyBytes, err := json.Marshal(searchBody)
	if err != nil {
		return err
	}

	_, err = npv.connector.callNotionAPI(ctx, "POST", "/search", strings.NewReader(string(bodyBytes)))
	return err
}

// validateUserAccess validates user reading permissions
func (npv *NotionPermissionValidator) validateUserAccess(ctx context.Context) error {
	_, err := npv.connector.callNotionAPI(ctx, "GET", "/users?page_size=1", nil)
	return err
}

// NotionAppInstaller handles Notion app installation flow
type NotionAppInstaller struct {
	authenticator *NotionAuthenticator
	config        *NotionConfig
}

// NewNotionAppInstaller creates a new app installer
func NewNotionAppInstaller(authenticator *NotionAuthenticator, config *NotionConfig) *NotionAppInstaller {
	return &NotionAppInstaller{
		authenticator: authenticator,
		config:        config,
	}
}

// StartInstallation starts the installation process
func (installer *NotionAppInstaller) StartInstallation(state string, ownerType string) string {
	return installer.authenticator.GetAuthorizationURL(state, ownerType)
}

// CompleteInstallation completes the installation process
func (installer *NotionAppInstaller) CompleteInstallation(ctx context.Context, code string) (*NotionOAuthToken, error) {
	tokenResp, err := installer.authenticator.ExchangeCodeForToken(ctx, code)
	if err != nil {
		return nil, err
	}

	// Update configuration with received token
	installer.config.OAuthToken = tokenResp.AccessToken
	installer.config.WorkspaceID = tokenResp.WorkspaceID
	installer.config.WorkspaceName = tokenResp.WorkspaceName

	return tokenResp, nil
}

// TokenManager manages Notion tokens and their lifecycle
type TokenManager struct {
	accessToken      string
	integrationToken string
	workspaceID      string
	botUser          *NotionUser
	authenticator    *NotionAuthenticator
}

// NewTokenManager creates a new token manager
func NewTokenManager(authenticator *NotionAuthenticator) *TokenManager {
	return &TokenManager{
		authenticator: authenticator,
	}
}

// SetTokens sets the access and integration tokens
func (tm *TokenManager) SetTokens(accessToken, integrationToken, workspaceID string) {
	tm.accessToken = accessToken
	tm.integrationToken = integrationToken
	tm.workspaceID = workspaceID
}

// GetActiveToken returns the active token (OAuth token takes precedence)
func (tm *TokenManager) GetActiveToken() string {
	if tm.accessToken != "" {
		return tm.accessToken
	}
	return tm.integrationToken
}

// ValidateActiveToken validates the active token
func (tm *TokenManager) ValidateActiveToken(ctx context.Context) error {
	token := tm.GetActiveToken()
	if token == "" {
		return fmt.Errorf("no active token available")
	}

	return tm.authenticator.ValidateToken(ctx, token)
}

// SetBotUser sets the bot user information
func (tm *TokenManager) SetBotUser(botUser *NotionUser) {
	tm.botUser = botUser
}

// GetBotUser returns the bot user information
func (tm *TokenManager) GetBotUser() *NotionUser {
	return tm.botUser
}

// NotionWebhookValidator validates webhook requests from Notion
type NotionWebhookValidator struct {
	secret string
}

// NewNotionWebhookValidator creates a new webhook validator
func NewNotionWebhookValidator(secret string) *NotionWebhookValidator {
	return &NotionWebhookValidator{
		secret: secret,
	}
}

// ValidateWebhook validates a webhook request
func (nwv *NotionWebhookValidator) ValidateWebhook(r *http.Request, body []byte) error {
	// Notion doesn't currently support webhook signatures like Slack/GitHub
	// This is a placeholder for when they add webhook support

	// For now, we can validate the request format and headers
	if r.Header.Get("Content-Type") != "application/json" {
		return fmt.Errorf("invalid content type")
	}

	// Add custom validation logic here when Notion adds webhook support
	return nil
}

// NotionCapabilityChecker checks what capabilities are available
type NotionCapabilityChecker struct {
	connector *NotionConnector
}

// NewNotionCapabilityChecker creates a new capability checker
func NewNotionCapabilityChecker(connector *NotionConnector) *NotionCapabilityChecker {
	return &NotionCapabilityChecker{
		connector: connector,
	}
}

// CheckCapabilities checks what capabilities are available with the current token
func (ncc *NotionCapabilityChecker) CheckCapabilities(ctx context.Context) (map[string]bool, error) {
	capabilities := map[string]bool{
		"read_users":       false,
		"search_content":   false,
		"read_pages":       false,
		"read_databases":   false,
		"read_blocks":      false,
		"create_pages":     false,
		"update_pages":     false,
		"create_databases": false,
		"update_databases": false,
	}

	// Test user reading
	if _, err := ncc.connector.callNotionAPI(ctx, "GET", "/users?page_size=1", nil); err == nil {
		capabilities["read_users"] = true
	}

	// Test content searching
	searchBody := `{"page_size": 1}`
	if _, err := ncc.connector.callNotionAPI(ctx, "POST", "/search", strings.NewReader(searchBody)); err == nil {
		capabilities["search_content"] = true
	}

	// Test page creation (this would require creating a test page)
	// For now, we'll assume this is not available unless explicitly configured
	capabilities["create_pages"] = false
	capabilities["update_pages"] = false
	capabilities["create_databases"] = false
	capabilities["update_databases"] = false

	// If we can search, we can likely read pages, databases, and blocks
	if capabilities["search_content"] {
		capabilities["read_pages"] = true
		capabilities["read_databases"] = true
		capabilities["read_blocks"] = true
	}

	return capabilities, nil
}

// HasCapability checks if a specific capability is available
func (ncc *NotionCapabilityChecker) HasCapability(ctx context.Context, capability string) (bool, error) {
	capabilities, err := ncc.CheckCapabilities(ctx)
	if err != nil {
		return false, err
	}

	return capabilities[capability], nil
}
