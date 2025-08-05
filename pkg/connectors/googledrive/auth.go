package googledrive

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// AuthManager handles OAuth2 authentication for Google Drive
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
		Scopes: []string{
			drive.DriveReadonlyScope,
			drive.DriveMetadataReadonlyScope,
			drive.DriveFileScope, // For file content access
		},
		Endpoint: google.Endpoint,
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

	// Generate authorization URL
	authURL := am.config.AuthCodeURL(state,
		oauth2.AccessTypeOffline, // Request refresh token
		oauth2.ApprovalForce,     // Force approval prompt
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

	// Create HTTP client with token
	client := am.config.Client(ctx, token)

	// Test the token by making a simple API call
	driveService, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return fmt.Errorf("failed to create Drive service: %w", err)
	}

	// Make a test call to validate the token
	_, err = driveService.About.Get().Fields("user").Do()
	if err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}

	return nil
}

// GetUserInfo retrieves user information using the access token
func (am *AuthManager) GetUserInfo(ctx context.Context, token *oauth2.Token) (*UserInfo, error) {
	// Create HTTP client with token
	client := am.config.Client(ctx, token)

	// Create Drive service
	driveService, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("failed to create Drive service: %w", err)
	}

	// Get user info
	about, err := driveService.About.Get().Fields("user, storageQuota").Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	userInfo := &UserInfo{
		ID:       about.User.EmailAddress, // Use email as user ID
		Name:     about.User.DisplayName,
		Email:    about.User.EmailAddress,
		PhotoURL: about.User.PhotoLink,
		Verified: about.User.EmailAddress != "",
	}

	// Add storage quota information if available
	if about.StorageQuota != nil {
		userInfo.StorageQuota = &StorageQuota{
			Limit:             about.StorageQuota.Limit,
			Usage:             about.StorageQuota.Usage,
			UsageInDrive:      about.StorageQuota.UsageInDrive,
			UsageInDriveTrash: about.StorageQuota.UsageInDriveTrash,
		}
	}

	return userInfo, nil
}

// RevokeToken revokes an access token
func (am *AuthManager) RevokeToken(ctx context.Context, token *oauth2.Token) error {
	if token.AccessToken == "" {
		return fmt.Errorf("no access token to revoke")
	}

	// Create HTTP request to revoke token
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://oauth2.googleapis.com/revoke?token="+token.AccessToken, nil)
	if err != nil {
		return fmt.Errorf("failed to create revoke request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

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

// UserInfo contains user information from Google Drive
type UserInfo struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Email        string        `json:"email"`
	PhotoURL     string        `json:"photo_url"`
	Verified     bool          `json:"verified"`
	StorageQuota *StorageQuota `json:"storage_quota,omitempty"`
}

// StorageQuota contains storage quota information
type StorageQuota struct {
	Limit             int64 `json:"limit"`
	Usage             int64 `json:"usage"`
	UsageInDrive      int64 `json:"usage_in_drive"`
	UsageInDriveTrash int64 `json:"usage_in_drive_trash"`
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
func (as *AuthenticationService) CompleteAuth(ctx context.Context, code, state string) (*oauth2.Token, *UserInfo, error) {
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
