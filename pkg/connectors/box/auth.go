package box

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jws"
)

// AuthManager handles OAuth2 and JWT authentication for Box
type AuthManager struct {
	config          *oauth2.Config
	enterpriseConfig *BoxEnterpriseConfig
	stateString     string
	httpClient      *http.Client
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(clientID, clientSecret, redirectURL string) *AuthManager {
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"root_readonly", "root_readwrite"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://account.box.com/api/oauth2/authorize",
			TokenURL: "https://api.box.com/oauth2/token",
		},
	}

	return &AuthManager{
		config:     config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// NewEnterpriseAuthManager creates a new authentication manager for enterprise apps
func NewEnterpriseAuthManager(enterpriseConfig *BoxEnterpriseConfig) *AuthManager {
	return &AuthManager{
		enterpriseConfig: enterpriseConfig,
		httpClient:       &http.Client{Timeout: 30 * time.Second},
	}
}

// GenerateAuthURL generates an OAuth2 authorization URL
func (am *AuthManager) GenerateAuthURL() (string, string, error) {
	if am.config == nil {
		return "", "", fmt.Errorf("OAuth2 config not available for enterprise auth")
	}

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
	if am.config == nil {
		return nil, fmt.Errorf("OAuth2 config not available for enterprise auth")
	}

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
	if am.config == nil {
		return nil, fmt.Errorf("OAuth2 config not available for enterprise auth")
	}

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

// GenerateJWTToken generates a JWT token for enterprise authentication
func (am *AuthManager) GenerateJWTToken(ctx context.Context, userID string) (*oauth2.Token, error) {
	if am.enterpriseConfig == nil {
		return nil, fmt.Errorf("enterprise config not available")
	}

	// Parse private key
	privateKey, err := am.parsePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Create JWT claims
	now := time.Now()
	claims := &JWTClaims{
		Issuer:     am.enterpriseConfig.ClientID,
		Subject:    userID,
		BoxSubType: "enterprise",
		Audience:   "https://api.box.com/oauth2/token",
		JTI:        am.generateJTI(),
		IssuedAt:   now.Unix(),
		ExpiresAt:  now.Add(time.Minute * 5).Unix(), // JWT should expire quickly
	}

	// Create JWT header
	header := &jws.Header{
		Algorithm: "RS256",
		KeyID:     am.enterpriseConfig.PublicKeyID,
	}

	// Convert claims to ClaimSet
	claimSet := &jws.ClaimSet{
		Iss: claims.Issuer,
		Sub: claims.Subject,
		Aud: claims.Audience,
		Iat: claims.IssuedAt,
		Exp: claims.ExpiresAt,
		PrivateClaims: map[string]interface{}{
			"box_sub_type": claims.BoxSubType,
			"jti":          claims.JTI,
		},
	}

	// Create and sign JWT
	token, err := jws.Encode(header, claimSet, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create JWT: %w", err)
	}

	// Exchange JWT for access token
	return am.exchangeJWTForToken(ctx, token)
}

// GenerateAppToken generates an application token for service account
func (am *AuthManager) GenerateAppToken(ctx context.Context) (*oauth2.Token, error) {
	if am.enterpriseConfig == nil {
		return nil, fmt.Errorf("enterprise config not available")
	}

	// Parse private key
	privateKey, err := am.parsePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Create JWT claims for app token
	now := time.Now()
	claims := &JWTClaims{
		Issuer:     am.enterpriseConfig.ClientID,
		Subject:    am.enterpriseConfig.EnterpriseID,
		BoxSubType: "enterprise",
		Audience:   "https://api.box.com/oauth2/token",
		JTI:        am.generateJTI(),
		IssuedAt:   now.Unix(),
		ExpiresAt:  now.Add(time.Minute * 5).Unix(),
	}

	// Create JWT header
	header := &jws.Header{
		Algorithm: "RS256",
		KeyID:     am.enterpriseConfig.PublicKeyID,
	}

	// Convert claims to ClaimSet
	claimSet := &jws.ClaimSet{
		Iss: claims.Issuer,
		Sub: claims.Subject,
		Aud: claims.Audience,
		Iat: claims.IssuedAt,
		Exp: claims.ExpiresAt,
		PrivateClaims: map[string]interface{}{
			"box_sub_type": claims.BoxSubType,
			"jti":          claims.JTI,
		},
	}

	// Create and sign JWT
	token, err := jws.Encode(header, claimSet, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create JWT: %w", err)
	}

	// Exchange JWT for access token
	return am.exchangeJWTForToken(ctx, token)
}

// ValidateToken validates an access token
func (am *AuthManager) ValidateToken(ctx context.Context, token *oauth2.Token) error {
	// Check if token is expired
	if !token.Valid() {
		return fmt.Errorf("token is expired")
	}

	// Test the token by making a simple API call
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.box.com/2.0/users/me", nil)
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
func (am *AuthManager) GetUserInfo(ctx context.Context, token *oauth2.Token) (*BoxUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.box.com/2.0/users/me", nil)
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

	userInfo := &BoxUserInfo{
		ID:           user.ID,
		Name:         user.Name,
		Login:        user.Login,
		Language:     user.Language,
		Timezone:     user.Timezone,
		SpaceAmount:  user.SpaceAmount,
		SpaceUsed:    user.SpaceUsed,
		MaxUploadSize: user.MaxUploadSize,
		Status:       user.Status,
		JobTitle:     user.JobTitle,
		Phone:        user.Phone,
		Address:      user.Address,
		AvatarURL:    user.AvatarURL,
		Role:         user.Role,
	}

	if user.Enterprise != nil {
		userInfo.Enterprise = &EnterpriseInfo{
			ID:   user.Enterprise.ID,
			Name: user.Enterprise.Name,
		}
	}

	return userInfo, nil
}

// RevokeToken revokes an access token
func (am *AuthManager) RevokeToken(ctx context.Context, token *oauth2.Token) error {
	if token.AccessToken == "" {
		return fmt.Errorf("no access token to revoke")
	}

	// Create form data for revocation
	data := url.Values{}
	data.Set("token", token.AccessToken)
	data.Set("client_id", am.config.ClientID)
	data.Set("client_secret", am.config.ClientSecret)

	// Create HTTP request to revoke token
	req, err := http.NewRequestWithContext(ctx, "POST", 
		"https://api.box.com/oauth2/revoke", strings.NewReader(data.Encode()))
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

func (am *AuthManager) generateJTI() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return base64.URLEncoding.EncodeToString(bytes)
}

func (am *AuthManager) parsePrivateKey() (*rsa.PrivateKey, error) {
	if am.enterpriseConfig.PrivateKey == "" {
		return nil, fmt.Errorf("private key not configured")
	}

	// Decode PEM block
	block, _ := pem.Decode([]byte(am.enterpriseConfig.PrivateKey))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block containing the private key")
	}

	// Parse private key
	var privateKey *rsa.PrivateKey
	var err error

	if am.enterpriseConfig.PrivateKeyPassword != "" {
		// Decrypt private key with password
		decryptedPEM, err := x509.DecryptPEMBlock(block, []byte(am.enterpriseConfig.PrivateKeyPassword))
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt private key: %w", err)
		}
		privateKey, err = x509.ParsePKCS1PrivateKey(decryptedPEM)
	} else {
		// Parse unencrypted private key
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return privateKey, nil
}

func (am *AuthManager) exchangeJWTForToken(ctx context.Context, jwtToken string) (*oauth2.Token, error) {
	// Create form data for token exchange
	data := url.Values{}
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	data.Set("assertion", jwtToken)
	data.Set("client_id", am.enterpriseConfig.ClientID)
	data.Set("client_secret", am.enterpriseConfig.ClientSecret)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", 
		"https://api.box.com/oauth2/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Execute request
	resp, err := am.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange JWT for token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status: %d", resp.StatusCode)
	}

	// Parse token response
	var tokenResponse TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	// Convert to oauth2.Token
	token := &oauth2.Token{
		AccessToken:  tokenResponse.AccessToken,
		TokenType:    tokenResponse.TokenType,
		RefreshToken: tokenResponse.RefreshToken,
	}

	if tokenResponse.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second)
	}

	return token, nil
}

// JWT claims structure for Box
type JWTClaims struct {
	Issuer     string `json:"iss"`
	Subject    string `json:"sub"`
	BoxSubType string `json:"box_sub_type"`
	Audience   string `json:"aud"`
	JTI        string `json:"jti"`
	IssuedAt   int64  `json:"iat"`
	ExpiresAt  int64  `json:"exp"`
}

// TokenResponse represents the token response from Box
type TokenResponse struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int    `json:"expires_in"`
	TokenType             string `json:"token_type"`
	Scope                 string `json:"scope"`
	RefreshToken          string `json:"refresh_token"`
	RestrictedTo          []interface{} `json:"restricted_to"`
	IssuedTokenType       string `json:"issued_token_type"`
}

// BoxUserInfo contains user information from Box
type BoxUserInfo struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Login         string         `json:"login"`
	Language      string         `json:"language"`
	Timezone      string         `json:"timezone"`
	SpaceAmount   int64          `json:"space_amount"`
	SpaceUsed     int64          `json:"space_used"`
	MaxUploadSize int64          `json:"max_upload_size"`
	Status        string         `json:"status"`
	JobTitle      string         `json:"job_title"`
	Phone         string         `json:"phone"`
	Address       string         `json:"address"`
	AvatarURL     string         `json:"avatar_url"`
	Role          string         `json:"role"`
	Enterprise    *EnterpriseInfo `json:"enterprise,omitempty"`
}

// EnterpriseInfo contains enterprise information
type EnterpriseInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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

// NewEnterpriseAuthenticationService creates a new enterprise authentication service
func NewEnterpriseAuthenticationService(enterpriseConfig *BoxEnterpriseConfig, encryptionKey []byte) *AuthenticationService {
	return &AuthenticationService{
		authManager:  NewEnterpriseAuthManager(enterpriseConfig),
		tokenManager: NewTokenManager(encryptionKey),
	}
}

// InitiateAuth initiates the OAuth2 flow
func (as *AuthenticationService) InitiateAuth() (string, string, error) {
	return as.authManager.GenerateAuthURL()
}

// CompleteAuth completes the OAuth2 flow
func (as *AuthenticationService) CompleteAuth(ctx context.Context, code, state string) (*oauth2.Token, *BoxUserInfo, error) {
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

// AuthenticateEnterprise authenticates using enterprise JWT
func (as *AuthenticationService) AuthenticateEnterprise(ctx context.Context, userID string) (*oauth2.Token, error) {
	return as.authManager.GenerateJWTToken(ctx, userID)
}

// AuthenticateApp authenticates using app token
func (as *AuthenticationService) AuthenticateApp(ctx context.Context) (*oauth2.Token, error) {
	return as.authManager.GenerateAppToken(ctx)
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