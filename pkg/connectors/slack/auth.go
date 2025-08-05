package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SlackAuthenticator handles Slack OAuth authentication
type SlackAuthenticator struct {
	clientID     string
	clientSecret string
	redirectURI  string
	scopes       []string
	httpClient   *http.Client
}

// OAuthConfig represents OAuth configuration
type OAuthConfig struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURI  string   `json:"redirect_uri"`
	Scopes       []string `json:"scopes"`
}

// TokenResponse represents OAuth token response
type TokenResponse struct {
	OK          bool      `json:"ok"`
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	Scope       string    `json:"scope"`
	BotUserID   string    `json:"bot_user_id"`
	AppID       string    `json:"app_id"`
	Team        SlackTeam `json:"team"`
	AuthedUser  struct {
		ID          string `json:"id"`
		Scope       string `json:"scope"`
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	} `json:"authed_user"`
	IncomingWebhook struct {
		Channel          string `json:"channel"`
		ChannelID        string `json:"channel_id"`
		ConfigurationURL string `json:"configuration_url"`
		URL              string `json:"url"`
	} `json:"incoming_webhook"`
	Enterprise struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"enterprise"`
	IsEnterpriseInstall bool   `json:"is_enterprise_install"`
	Error               string `json:"error,omitempty"`
	ErrorDescription    string `json:"error_description,omitempty"`
}

// RefreshTokenResponse represents refresh token response
type RefreshTokenResponse struct {
	OK           bool   `json:"ok"`
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	Error        string `json:"error,omitempty"`
}

// NewSlackAuthenticator creates a new Slack authenticator
func NewSlackAuthenticator(config *OAuthConfig) *SlackAuthenticator {
	return &SlackAuthenticator{
		clientID:     config.ClientID,
		clientSecret: config.ClientSecret,
		redirectURI:  config.RedirectURI,
		scopes:       config.Scopes,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetAuthorizationURL generates the OAuth authorization URL
func (auth *SlackAuthenticator) GetAuthorizationURL(state string, userScopes []string) string {
	params := url.Values{}
	params.Set("client_id", auth.clientID)
	params.Set("scope", strings.Join(auth.scopes, ","))
	params.Set("redirect_uri", auth.redirectURI)
	params.Set("response_type", "code")

	if state != "" {
		params.Set("state", state)
	}

	if len(userScopes) > 0 {
		params.Set("user_scope", strings.Join(userScopes, ","))
	}

	return "https://slack.com/oauth/v2/authorize?" + params.Encode()
}

// ExchangeCodeForToken exchanges authorization code for access token
func (auth *SlackAuthenticator) ExchangeCodeForToken(ctx context.Context, code string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("client_id", auth.clientID)
	data.Set("client_secret", auth.clientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", auth.redirectURI)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://slack.com/api/oauth.v2.access", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := auth.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	if !tokenResp.OK {
		return nil, fmt.Errorf("OAuth error: %s - %s", tokenResp.Error, tokenResp.ErrorDescription)
	}

	return &tokenResp, nil
}

// RefreshToken refreshes an access token using a refresh token
func (auth *SlackAuthenticator) RefreshToken(ctx context.Context, refreshToken string) (*RefreshTokenResponse, error) {
	data := url.Values{}
	data.Set("client_id", auth.clientID)
	data.Set("client_secret", auth.clientSecret)
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://slack.com/api/oauth.v2.access", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := auth.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var refreshResp RefreshTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&refreshResp); err != nil {
		return nil, err
	}

	if !refreshResp.OK {
		return nil, fmt.Errorf("token refresh error: %s", refreshResp.Error)
	}

	return &refreshResp, nil
}

// ValidateToken validates an access token
func (auth *SlackAuthenticator) ValidateToken(ctx context.Context, token string) (*SlackAuth, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", "https://slack.com/api/auth.test", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := auth.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var authResp SlackAuth
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return nil, err
	}

	if !authResp.OK {
		return nil, fmt.Errorf("token validation failed")
	}

	return &authResp, nil
}

// RevokeToken revokes an access token
func (auth *SlackAuthenticator) RevokeToken(ctx context.Context, token string) error {
	data := url.Values{}
	data.Set("token", token)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://slack.com/api/auth.revoke", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := auth.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var revokeResp struct {
		OK      bool   `json:"ok"`
		Revoked bool   `json:"revoked"`
		Error   string `json:"error,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&revokeResp); err != nil {
		return err
	}

	if !revokeResp.OK {
		return fmt.Errorf("token revocation failed: %s", revokeResp.Error)
	}

	return nil
}

// GetRequiredScopes returns the recommended scopes for the Slack connector
func GetRequiredScopes() []string {
	return []string{
		// Bot token scopes
		"channels:history",
		"channels:read",
		"groups:history",
		"groups:read",
		"im:history",
		"im:read",
		"mpim:history",
		"mpim:read",
		"files:read",
		"users:read",
		"users:read.email",
		"team:read",
		"emoji:read",
		"reactions:read",
		"pins:read",
		"bookmarks:read",
		"usergroups:read",
		"chat:write", // For posting messages (optional)
	}
}

// GetUserScopes returns user-level scopes (if needed)
func GetUserScopes() []string {
	return []string{
		"channels:history",
		"groups:history",
		"im:history",
		"mpim:history",
		"search:read", // For search functionality
	}
}

// TokenManager manages access tokens and their lifecycle
type TokenManager struct {
	botToken      string
	userToken     string
	refreshToken  string
	expiresAt     time.Time
	authenticator *SlackAuthenticator
}

// NewTokenManager creates a new token manager
func NewTokenManager(authenticator *SlackAuthenticator) *TokenManager {
	return &TokenManager{
		authenticator: authenticator,
	}
}

// SetTokens sets the bot and user tokens
func (tm *TokenManager) SetTokens(botToken, userToken, refreshToken string, expiresAt time.Time) {
	tm.botToken = botToken
	tm.userToken = userToken
	tm.refreshToken = refreshToken
	tm.expiresAt = expiresAt
}

// GetBotToken returns the bot token, refreshing if necessary
func (tm *TokenManager) GetBotToken(ctx context.Context) (string, error) {
	if tm.needsRefresh() {
		if err := tm.refreshTokens(ctx); err != nil {
			return "", err
		}
	}
	return tm.botToken, nil
}

// GetUserToken returns the user token, refreshing if necessary
func (tm *TokenManager) GetUserToken(ctx context.Context) (string, error) {
	if tm.needsRefresh() {
		if err := tm.refreshTokens(ctx); err != nil {
			return "", err
		}
	}
	return tm.userToken, nil
}

// needsRefresh checks if tokens need to be refreshed
func (tm *TokenManager) needsRefresh() bool {
	if tm.expiresAt.IsZero() {
		return false // No expiration set
	}

	// Refresh if token expires within 5 minutes
	return time.Until(tm.expiresAt) < 5*time.Minute
}

// refreshTokens refreshes the access tokens
func (tm *TokenManager) refreshTokens(ctx context.Context) error {
	if tm.refreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	resp, err := tm.authenticator.RefreshToken(ctx, tm.refreshToken)
	if err != nil {
		return err
	}

	tm.botToken = resp.AccessToken
	tm.refreshToken = resp.RefreshToken

	if resp.ExpiresIn > 0 {
		tm.expiresAt = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	}

	return nil
}

// SlackAppInstaller handles Slack app installation flow
type SlackAppInstaller struct {
	authenticator *SlackAuthenticator
	config        *SlackConfig
}

// NewSlackAppInstaller creates a new app installer
func NewSlackAppInstaller(authenticator *SlackAuthenticator, config *SlackConfig) *SlackAppInstaller {
	return &SlackAppInstaller{
		authenticator: authenticator,
		config:        config,
	}
}

// StartInstallation starts the installation process
func (installer *SlackAppInstaller) StartInstallation(state string) string {
	userScopes := GetUserScopes()

	return installer.authenticator.GetAuthorizationURL(state, userScopes)
}

// CompleteInstallation completes the installation process
func (installer *SlackAppInstaller) CompleteInstallation(ctx context.Context, code string) (*TokenResponse, error) {
	tokenResp, err := installer.authenticator.ExchangeCodeForToken(ctx, code)
	if err != nil {
		return nil, err
	}

	// Update configuration with received tokens
	installer.config.BotToken = tokenResp.AccessToken
	installer.config.UserToken = tokenResp.AuthedUser.AccessToken
	installer.config.WorkspaceID = tokenResp.Team.ID
	installer.config.WorkspaceName = tokenResp.Team.Name

	return tokenResp, nil
}

// PermissionValidator validates that the connector has required permissions
type PermissionValidator struct {
	connector *SlackConnector
}

// NewPermissionValidator creates a new permission validator
func NewPermissionValidator(connector *SlackConnector) *PermissionValidator {
	return &PermissionValidator{
		connector: connector,
	}
}

// ValidatePermissions validates that the connector has all required permissions
func (pv *PermissionValidator) ValidatePermissions(ctx context.Context) error {
	// Test bot token permissions
	if err := pv.validateBotPermissions(ctx); err != nil {
		return fmt.Errorf("bot permissions validation failed: %w", err)
	}

	// Test user token permissions if available
	if pv.connector.config.UserToken != "" {
		if err := pv.validateUserPermissions(ctx); err != nil {
			return fmt.Errorf("user permissions validation failed: %w", err)
		}
	}

	return nil
}

// validateBotPermissions validates bot token permissions
func (pv *PermissionValidator) validateBotPermissions(ctx context.Context) error {
	requiredMethods := []string{
		"auth.test",
		"conversations.list",
		"conversations.history",
		"users.list",
		"files.list",
		"team.info",
	}

	for _, method := range requiredMethods {
		if err := pv.testAPIMethod(ctx, method, pv.connector.config.BotToken); err != nil {
			return fmt.Errorf("missing permission for %s: %w", method, err)
		}
	}

	return nil
}

// validateUserPermissions validates user token permissions
func (pv *PermissionValidator) validateUserPermissions(ctx context.Context) error {
	userMethods := []string{
		"auth.test",
		"search.messages", // User-level search
	}

	for _, method := range userMethods {
		if err := pv.testAPIMethod(ctx, method, pv.connector.config.UserToken); err != nil {
			// User permissions are optional, so we just log warnings
			continue
		}
	}

	return nil
}

// testAPIMethod tests if an API method is accessible
func (pv *PermissionValidator) testAPIMethod(ctx context.Context, method, token string) error {
	// Make a test call to the API method
	_, err := pv.connector.callSlackAPI(ctx, method, nil, token)
	return err
}
