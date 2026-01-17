package dropbox

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// AuthManager handles OAuth2 authentication for Dropbox
type AuthManager struct {
	config      *oauth2.Config
	stateString string
	httpClient  *http.Client
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(clientID, clientSecret, redirectURL string) *AuthManager {
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"files.metadata.read", "files.content.read", "sharing.read"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://www.dropbox.com/oauth2/authorize",
			TokenURL: "https://api.dropboxapi.com/oauth2/token",
		},
	}

	return &AuthManager{
		config:     config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// GenerateAuthURL generates an OAuth2 authorization URL
func (am *AuthManager) GenerateAuthURL() (string, string, error) {
	// Generate a random state string for security
	state, err := am.generateRandomState()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate state: %w", err)
	}

	am.stateString = state

	// Generate authorization URL with additional parameters for Dropbox
	authURL := am.config.AuthCodeURL(state,
		oauth2.AccessTypeOffline,                               // Request refresh token
		oauth2.SetAuthURLParam("token_access_type", "offline"), // Dropbox-specific parameter
		oauth2.SetAuthURLParam("force_reapprove", "true"),      // Force user to reapprove
	)

	return authURL, state, nil
}

// ExchangeCodeForToken exchanges authorization code for access token
func (am *AuthManager) ExchangeCodeForToken(ctx context.Context, code, state string) (*oauth2.Token, error) {
	// Verify state to prevent CSRF attacks
	if state != am.stateString {
		return nil, fmt.Errorf("invalid state parameter")
	}

	// Exchange code for token
	token, err := am.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}

	return token, nil
}

// RefreshToken refreshes an expired access token
func (am *AuthManager) RefreshToken(ctx context.Context, token *oauth2.Token) (*oauth2.Token, error) {
	if token.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	// Create token source
	tokenSource := am.config.TokenSource(ctx, token)

	// Get new token
	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	return newToken, nil
}

// ValidateToken validates an access token
func (am *AuthManager) ValidateToken(ctx context.Context, token *oauth2.Token) error {
	// Check if token is expired
	if !token.Valid() {
		return fmt.Errorf("token is expired")
	}

	// Test the token by making a simple API call
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.dropboxapi.com/2/users/get_current_account", nil)
	if err != nil {
		return fmt.Errorf("failed to create validation request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := am.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token validation failed with status: %d", resp.StatusCode)
	}

	return nil
}

// GetUserInfo retrieves user information using the access token
func (am *AuthManager) GetUserInfo(ctx context.Context, token *oauth2.Token) (*DropboxUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.dropboxapi.com/2/users/get_current_account", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := am.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get user info failed with status: %d", resp.StatusCode)
	}

	var account DropboxAccount
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return nil, fmt.Errorf("failed to parse user info: %w", err)
	}

	userInfo := &DropboxUserInfo{
		AccountID:       account.AccountID,
		Name:            account.Name,
		Email:           account.Email,
		EmailVerified:   account.EmailVerified,
		Disabled:        account.Disabled,
		Locale:          account.Locale,
		ReferralLink:    account.ReferralLink,
		IsPaired:        account.IsPaired,
		AccountType:     account.AccountType.Tag,
		Country:         account.Country,
		ProfilePhotoURL: account.ProfilePhotoURL,
	}

	if account.Team != nil {
		userInfo.Team = &TeamInfo{
			ID:   account.Team.ID,
			Name: account.Team.Name,
		}
		userInfo.TeamMemberID = account.TeamMemberID
	}

	return userInfo, nil
}

// GetSpaceUsage retrieves space usage information
func (am *AuthManager) GetSpaceUsage(ctx context.Context, token *oauth2.Token) (*DropboxSpaceUsage, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.dropboxapi.com/2/users/get_space_usage", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := am.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get space usage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get space usage failed with status: %d", resp.StatusCode)
	}

	var spaceUsage DropboxSpaceUsage
	if err := json.NewDecoder(resp.Body).Decode(&spaceUsage); err != nil {
		return nil, fmt.Errorf("failed to parse space usage: %w", err)
	}

	return &spaceUsage, nil
}

// RevokeToken revokes an access token
func (am *AuthManager) RevokeToken(ctx context.Context, token *oauth2.Token) error {
	if token.AccessToken == "" {
		return fmt.Errorf("no access token to revoke")
	}

	// Create HTTP request to revoke token
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.dropboxapi.com/2/auth/token/revoke", nil)
	if err != nil {
		return fmt.Errorf("failed to create revoke request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := am.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token revocation failed with status: %d", resp.StatusCode)
	}

	return nil
}

// CheckTokenExpiry checks if a token is near expiry
func (am *AuthManager) CheckTokenExpiry(token *oauth2.Token, threshold time.Duration) bool {
	if token.Expiry.IsZero() {
		return false // Token doesn't expire
	}

	return time.Until(token.Expiry) < threshold
}

// Helper methods

func (am *AuthManager) generateRandomState() (string, error) {
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// DropboxUserInfo contains user information from Dropbox
type DropboxUserInfo struct {
	AccountID       string    `json:"account_id"`
	Name            Name      `json:"name"`
	Email           string    `json:"email"`
	EmailVerified   bool      `json:"email_verified"`
	Disabled        bool      `json:"disabled"`
	Locale          string    `json:"locale"`
	ReferralLink    string    `json:"referral_link"`
	IsPaired        bool      `json:"is_paired"`
	AccountType     string    `json:"account_type"`
	Country         string    `json:"country"`
	ProfilePhotoURL string    `json:"profile_photo_url"`
	Team            *TeamInfo `json:"team,omitempty"`
	TeamMemberID    string    `json:"team_member_id,omitempty"`
}

// TeamInfo contains team information
type TeamInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DropboxSpaceUsage contains space usage information
type DropboxSpaceUsage struct {
	Used       uint64          `json:"used"`
	Allocation SpaceAllocation `json:"allocation"`
}

// SpaceAllocation represents space allocation information
type SpaceAllocation struct {
	Tag                          string `json:".tag"`
	Allocated                    uint64 `json:"allocated,omitempty"`
	UserWithinTeamSpaceAllocated uint64 `json:"user_within_team_space_allocated,omitempty"`
	UserWithinTeamSpaceLimitType string `json:"user_within_team_space_limit_type,omitempty"`
}

// TokenManager handles secure storage and retrieval of OAuth tokens
type TokenManager struct {
	encryptionKey []byte
}

// NewTokenManager creates a new token manager
func NewTokenManager(encryptionKey []byte) *TokenManager {
	return &TokenManager{
		encryptionKey: encryptionKey,
	}
}

// StoreToken securely stores an OAuth token
func (tm *TokenManager) StoreToken(userID string, token *oauth2.Token) error {
	// This would implement secure token storage
	// For now, this is a placeholder
	return nil
}

// RetrieveToken securely retrieves an OAuth token
func (tm *TokenManager) RetrieveToken(userID string) (*oauth2.Token, error) {
	// This would implement secure token retrieval
	// For now, this is a placeholder
	return nil, fmt.Errorf("token not found")
}

// DeleteToken securely deletes an OAuth token
func (tm *TokenManager) DeleteToken(userID string) error {
	// This would implement secure token deletion
	// For now, this is a placeholder
	return nil
}

// AuthenticationService provides high-level authentication services
type AuthenticationService struct {
	authManager  *AuthManager
	tokenManager *TokenManager
}

// NewAuthenticationService creates a new authentication service
func NewAuthenticationService(clientID, clientSecret, redirectURL string, encryptionKey []byte) *AuthenticationService {
	return &AuthenticationService{
		authManager:  NewAuthManager(clientID, clientSecret, redirectURL),
		tokenManager: NewTokenManager(encryptionKey),
	}
}

// InitiateAuth initiates the OAuth2 flow
func (as *AuthenticationService) InitiateAuth() (string, string, error) {
	return as.authManager.GenerateAuthURL()
}

// CompleteAuth completes the OAuth2 flow
func (as *AuthenticationService) CompleteAuth(ctx context.Context, code, state string) (*oauth2.Token, *DropboxUserInfo, error) {
	// Exchange code for token
	token, err := as.authManager.ExchangeCodeForToken(ctx, code, state)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to complete auth: %w", err)
	}

	// Get user info
	userInfo, err := as.authManager.GetUserInfo(ctx, token)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get user info: %w", err)
	}

	return token, userInfo, nil
}

// RefreshUserToken refreshes a user's token
func (as *AuthenticationService) RefreshUserToken(ctx context.Context, userID string) (*oauth2.Token, error) {
	// Retrieve stored token
	currentToken, err := as.tokenManager.RetrieveToken(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve token: %w", err)
	}

	// Refresh token
	newToken, err := as.authManager.RefreshToken(ctx, currentToken)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	// Store updated token
	if err := as.tokenManager.StoreToken(userID, newToken); err != nil {
		return nil, fmt.Errorf("failed to store refreshed token: %w", err)
	}

	return newToken, nil
}

// ValidateUserToken validates a user's token
func (as *AuthenticationService) ValidateUserToken(ctx context.Context, userID string) error {
	// Retrieve stored token
	token, err := as.tokenManager.RetrieveToken(userID)
	if err != nil {
		return fmt.Errorf("failed to retrieve token: %w", err)
	}

	// Validate token
	return as.authManager.ValidateToken(ctx, token)
}

// RevokeUserToken revokes a user's token
func (as *AuthenticationService) RevokeUserToken(ctx context.Context, userID string) error {
	// Retrieve stored token
	token, err := as.tokenManager.RetrieveToken(userID)
	if err != nil {
		return fmt.Errorf("failed to retrieve token: %w", err)
	}

	// Revoke token
	if err := as.authManager.RevokeToken(ctx, token); err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	// Delete stored token
	if err := as.tokenManager.DeleteToken(userID); err != nil {
		return fmt.Errorf("failed to delete token: %w", err)
	}

	return nil
}
