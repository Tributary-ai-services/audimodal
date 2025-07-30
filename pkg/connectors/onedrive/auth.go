package onedrive

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

// AuthManager handles OAuth2 authentication for OneDrive
type AuthManager struct {
	config      *oauth2.Config
	stateString string
	httpClient  *http.Client
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(clientID, clientSecret, redirectURL, tenantID string) *AuthManager {
	var endpoint oauth2.Endpoint
	if tenantID != "" {
		// Use tenant-specific endpoints for Azure AD
		endpoint = oauth2.Endpoint{
			AuthURL:  fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize", tenantID),
			TokenURL: fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID),
		}
	} else {
		// Use common endpoint for personal accounts
		endpoint = oauth2.Endpoint{
			AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		}
	}

	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"https://graph.microsoft.com/Files.ReadWrite.All", "https://graph.microsoft.com/Sites.ReadWrite.All"},
		Endpoint:     endpoint,
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

	// Generate authorization URL with additional parameters for Microsoft
	authURL := am.config.AuthCodeURL(state,
		oauth2.AccessTypeOffline, // Request refresh token
		oauth2.SetAuthURLParam("prompt", "consent"),      // Force consent screen
		oauth2.SetAuthURLParam("response_mode", "query"), // Use query parameters
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
	req, err := http.NewRequestWithContext(ctx, "GET", "https://graph.microsoft.com/v1.0/me", nil)
	if err != nil {
		return fmt.Errorf("failed to create validation request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

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
func (am *AuthManager) GetUserInfo(ctx context.Context, token *oauth2.Token) (*OneDriveUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://graph.microsoft.com/v1.0/me", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := am.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get user info failed with status: %d", resp.StatusCode)
	}

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to parse user info: %w", err)
	}

	userInfo := &OneDriveUserInfo{
		ID:                user.ID,
		DisplayName:       user.DisplayName,
		GivenName:         user.GivenName,
		Surname:           user.Surname,
		Mail:              user.Mail,
		UserPrincipalName: user.UserPrincipalName,
		JobTitle:          user.JobTitle,
		OfficeLocation:    user.OfficeLocation,
		PreferredLanguage: user.PreferredLanguage,
		MobilePhone:       user.MobilePhone,
		BusinessPhones:    user.BusinessPhones,
	}

	return userInfo, nil
}

// GetDriveInfo retrieves drive information
func (am *AuthManager) GetDriveInfo(ctx context.Context, token *oauth2.Token) (*OneDriveDriveInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://graph.microsoft.com/v1.0/me/drive", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := am.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get drive info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get drive info failed with status: %d", resp.StatusCode)
	}

	var drive Drive
	if err := json.NewDecoder(resp.Body).Decode(&drive); err != nil {
		return nil, fmt.Errorf("failed to parse drive info: %w", err)
	}

	driveInfo := &OneDriveDriveInfo{
		ID:          drive.ID,
		Name:        drive.Name,
		DriveType:   drive.DriveType,
		Description: drive.Description,
		WebURL:      drive.WebURL,
	}

	if drive.Quota != nil {
		driveInfo.Quota = &OneDriveQuota{
			Total:     drive.Quota.Total,
			Used:      drive.Quota.Used,
			Remaining: drive.Quota.Remaining,
			Deleted:   drive.Quota.Deleted,
			State:     drive.Quota.State,
		}
	}

	if drive.Owner != nil && drive.Owner.User != nil {
		driveInfo.Owner = &OneDriveOwner{
			DisplayName: drive.Owner.User.DisplayName,
			ID:          drive.Owner.User.ID,
		}
	}

	return driveInfo, nil
}

// CheckTokenExpiry checks if a token is near expiry
func (am *AuthManager) CheckTokenExpiry(token *oauth2.Token, threshold time.Duration) bool {
	if token.Expiry.IsZero() {
		return false // Token doesn't expire
	}

	return time.Until(token.Expiry) < threshold
}

// GetTenantInfo retrieves tenant information for business accounts
func (am *AuthManager) GetTenantInfo(ctx context.Context, token *oauth2.Token) (*TenantInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://graph.microsoft.com/v1.0/organization", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := am.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get tenant info failed with status: %d", resp.StatusCode)
	}

	var orgResponse struct {
		Value []Organization `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&orgResponse); err != nil {
		return nil, fmt.Errorf("failed to parse tenant info: %w", err)
	}

	if len(orgResponse.Value) == 0 {
		return nil, fmt.Errorf("no organization found")
	}

	org := orgResponse.Value[0]
	tenantInfo := &TenantInfo{
		ID:          org.ID,
		DisplayName: org.DisplayName,
		Domain:      "",
	}

	// Get primary domain if available
	if len(org.VerifiedDomains) > 0 {
		for _, domain := range org.VerifiedDomains {
			if domain.IsDefault {
				tenantInfo.Domain = domain.Name
				break
			}
		}
		if tenantInfo.Domain == "" {
			tenantInfo.Domain = org.VerifiedDomains[0].Name
		}
	}

	return tenantInfo, nil
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

// OneDriveUserInfo contains user information from Microsoft Graph
type OneDriveUserInfo struct {
	ID                string   `json:"id"`
	DisplayName       string   `json:"displayName"`
	GivenName         string   `json:"givenName"`
	Surname           string   `json:"surname"`
	Mail              string   `json:"mail"`
	UserPrincipalName string   `json:"userPrincipalName"`
	JobTitle          string   `json:"jobTitle"`
	OfficeLocation    string   `json:"officeLocation"`
	PreferredLanguage string   `json:"preferredLanguage"`
	MobilePhone       string   `json:"mobilePhone"`
	BusinessPhones    []string `json:"businessPhones"`
}

// OneDriveDriveInfo contains drive information
type OneDriveDriveInfo struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	DriveType   string           `json:"driveType"`
	Description string           `json:"description"`
	WebURL      string           `json:"webUrl"`
	Quota       *OneDriveQuota   `json:"quota,omitempty"`
	Owner       *OneDriveOwner   `json:"owner,omitempty"`
}

// OneDriveQuota contains quota information
type OneDriveQuota struct {
	Total     int64  `json:"total"`
	Used      int64  `json:"used"`
	Remaining int64  `json:"remaining"`
	Deleted   int64  `json:"deleted"`
	State     string `json:"state"`
}

// OneDriveOwner contains owner information  
type OneDriveOwner struct {
	DisplayName string `json:"displayName"`
	ID          string `json:"id"`
}

// TenantInfo contains tenant information
type TenantInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Domain      string `json:"domain"`
}

// Organization represents an Azure AD organization
type Organization struct {
	ID              string           `json:"id"`
	DisplayName     string           `json:"displayName"`
	VerifiedDomains []VerifiedDomain `json:"verifiedDomains"`
}

// VerifiedDomain represents a verified domain in the organization
type VerifiedDomain struct {
	Capabilities string `json:"capabilities"`
	IsDefault    bool   `json:"isDefault"`
	IsInitial    bool   `json:"isInitial"`
	Name         string `json:"name"`
	Type         string `json:"type"`
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
func NewAuthenticationService(clientID, clientSecret, redirectURL, tenantID string, encryptionKey []byte) *AuthenticationService {
	return &AuthenticationService{
		authManager:  NewAuthManager(clientID, clientSecret, redirectURL, tenantID),
		tokenManager: NewTokenManager(encryptionKey),
	}
}

// InitiateAuth initiates the OAuth2 flow
func (as *AuthenticationService) InitiateAuth() (string, string, error) {
	return as.authManager.GenerateAuthURL()
}

// CompleteAuth completes the OAuth2 flow
func (as *AuthenticationService) CompleteAuth(ctx context.Context, code, state string) (*oauth2.Token, *OneDriveUserInfo, error) {
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

// GetUserInfo retrieves user information
func (as *AuthenticationService) GetUserInfo(ctx context.Context, userID string) (*OneDriveUserInfo, error) {
	// Retrieve stored token
	token, err := as.tokenManager.RetrieveToken(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve token: %w", err)
	}

	return as.authManager.GetUserInfo(ctx, token)
}

// GetDriveInfo retrieves drive information
func (as *AuthenticationService) GetDriveInfo(ctx context.Context, userID string) (*OneDriveDriveInfo, error) {
	// Retrieve stored token
	token, err := as.tokenManager.RetrieveToken(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve token: %w", err)
	}

	return as.authManager.GetDriveInfo(ctx, token)
}