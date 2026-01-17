package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"sync"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// JWTManager implements the TokenManager interface
type JWTManager struct {
	privateKey    *rsa.PrivateKey
	publicKey     *rsa.PublicKey
	issuer        string
	accessTTL     time.Duration
	refreshTTL    time.Duration
	revokedTokens map[string]time.Time // token ID -> revocation time
	mu            sync.RWMutex
	cleanupTicker *time.Ticker
}

// JWTConfig contains JWT configuration
type JWTConfig struct {
	PrivateKeyPEM string        `yaml:"private_key_pem"`
	PublicKeyPEM  string        `yaml:"public_key_pem"`
	Issuer        string        `yaml:"issuer"`
	AccessTTL     time.Duration `yaml:"access_ttl"`
	RefreshTTL    time.Duration `yaml:"refresh_ttl"`
	KeySize       int           `yaml:"key_size"`
}

// DefaultJWTConfig returns default JWT configuration
func DefaultJWTConfig() *JWTConfig {
	return &JWTConfig{
		Issuer:     "eai-ingest",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 7 * 24 * time.Hour, // 7 days
		KeySize:    2048,
	}
}

// NewJWTManager creates a new JWT manager
func NewJWTManager(config *JWTConfig) (*JWTManager, error) {
	if config == nil {
		config = DefaultJWTConfig()
	}

	manager := &JWTManager{
		issuer:        config.Issuer,
		accessTTL:     config.AccessTTL,
		refreshTTL:    config.RefreshTTL,
		revokedTokens: make(map[string]time.Time),
	}

	// Load or generate keys
	if err := manager.initializeKeys(config); err != nil {
		return nil, fmt.Errorf("failed to initialize keys: %w", err)
	}

	// Start cleanup goroutine for revoked tokens
	manager.startCleanup()

	return manager, nil
}

// initializeKeys loads keys from config or generates new ones
func (j *JWTManager) initializeKeys(config *JWTConfig) error {
	if config.PrivateKeyPEM != "" && config.PublicKeyPEM != "" {
		// Load keys from config
		return j.loadKeysFromPEM(config.PrivateKeyPEM, config.PublicKeyPEM)
	}

	// Generate new keys
	return j.generateKeys(config.KeySize)
}

// loadKeysFromPEM loads RSA keys from PEM strings
func (j *JWTManager) loadKeysFromPEM(privateKeyPEM, publicKeyPEM string) error {
	// Parse private key
	privateBlock, _ := pem.Decode([]byte(privateKeyPEM))
	if privateBlock == nil {
		return fmt.Errorf("failed to decode private key PEM")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(privateBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	// Parse public key
	publicBlock, _ := pem.Decode([]byte(publicKeyPEM))
	if publicBlock == nil {
		return fmt.Errorf("failed to decode public key PEM")
	}

	publicKeyInterface, err := x509.ParsePKIXPublicKey(publicBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	publicKey, ok := publicKeyInterface.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("public key is not RSA")
	}

	j.privateKey = privateKey
	j.publicKey = publicKey

	return nil
}

// generateKeys generates new RSA key pair
func (j *JWTManager) generateKeys(keySize int) error {
	privateKey, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	j.privateKey = privateKey
	j.publicKey = &privateKey.PublicKey

	return nil
}

// GenerateToken generates a JWT token with the given claims
func (j *JWTManager) GenerateToken(claims *Claims) (string, error) {
	if claims == nil {
		return "", fmt.Errorf("claims cannot be nil")
	}

	now := time.Now()

	// Set token ID if not provided
	if claims.TokenID == "" {
		claims.TokenID = uuid.New().String()
	}

	// Set standard claims
	claims.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    j.issuer,
		Subject:   claims.UserID.String(),
		Audience:  []string{j.issuer},
		ExpiresAt: jwt.NewNumericDate(now.Add(j.getTokenTTL(claims.TokenType))),
		NotBefore: jwt.NewNumericDate(now),
		IssuedAt:  jwt.NewNumericDate(now),
		ID:        claims.TokenID,
	}

	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	// Sign token
	tokenString, err := token.SignedString(j.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// ValidateToken validates a JWT token and returns claims
func (j *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	// Parse token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return j.publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	// Extract claims
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Check if token is revoked
	if j.IsTokenRevoked(claims.TokenID) {
		return nil, ErrTokenRevoked
	}

	return claims, nil
}

// RefreshToken creates a new access token from a refresh token
func (j *JWTManager) RefreshToken(tokenString string) (string, error) {
	// Validate the refresh token
	claims, err := j.ValidateToken(tokenString)
	if err != nil {
		return "", err
	}

	// Ensure it's a refresh token
	if claims.TokenType != TokenTypeRefresh {
		return "", fmt.Errorf("token is not a refresh token")
	}

	// Create new access token claims
	newClaims := &Claims{
		UserID:      claims.UserID,
		Username:    claims.Username,
		Email:       claims.Email,
		TenantID:    claims.TenantID,
		Roles:       claims.Roles,
		TenantRole:  claims.TenantRole,
		Permissions: claims.Permissions,
		Scopes:      claims.Scopes,
		TokenType:   TokenTypeAccess,
	}

	// Generate new access token
	return j.GenerateToken(newClaims)
}

// ExtractClaims extracts claims from a token without full validation
func (j *JWTManager) ExtractClaims(tokenString string) (*Claims, error) {
	// Parse token without verification (for extracting claims only)
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, &Claims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// IsTokenRevoked checks if a token is revoked
func (j *JWTManager) IsTokenRevoked(tokenID string) bool {
	j.mu.RLock()
	defer j.mu.RUnlock()

	_, revoked := j.revokedTokens[tokenID]
	return revoked
}

// RevokeToken revokes a token by its ID
func (j *JWTManager) RevokeToken(tokenID string) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.revokedTokens[tokenID] = time.Now()
	return nil
}

// getTokenTTL returns the TTL for a token type
func (j *JWTManager) getTokenTTL(tokenType TokenType) time.Duration {
	switch tokenType {
	case TokenTypeAccess:
		return j.accessTTL
	case TokenTypeRefresh:
		return j.refreshTTL
	case TokenTypeAPI:
		return 365 * 24 * time.Hour // 1 year for API tokens
	default:
		return j.accessTTL
	}
}

// startCleanup starts the cleanup goroutine for expired revoked tokens
func (j *JWTManager) startCleanup() {
	j.cleanupTicker = time.NewTicker(1 * time.Hour)

	go func() {
		for range j.cleanupTicker.C {
			j.cleanupRevokedTokens()
		}
	}()
}

// cleanupRevokedTokens removes expired revoked tokens
func (j *JWTManager) cleanupRevokedTokens() {
	j.mu.Lock()
	defer j.mu.Unlock()

	now := time.Now()
	maxAge := j.refreshTTL // Keep revoked tokens for the longest possible token lifetime

	for tokenID, revokedAt := range j.revokedTokens {
		if now.Sub(revokedAt) > maxAge {
			delete(j.revokedTokens, tokenID)
		}
	}
}

// Stop stops the JWT manager and cleanup goroutine
func (j *JWTManager) Stop() {
	if j.cleanupTicker != nil {
		j.cleanupTicker.Stop()
	}
}

// GetPublicKeyPEM returns the public key in PEM format
func (j *JWTManager) GetPublicKeyPEM() (string, error) {
	publicKeyDER, err := x509.MarshalPKIXPublicKey(j.publicKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal public key: %w", err)
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyDER,
	})

	return string(publicKeyPEM), nil
}

// GetPrivateKeyPEM returns the private key in PEM format
func (j *JWTManager) GetPrivateKeyPEM() (string, error) {
	privateKeyDER := x509.MarshalPKCS1PrivateKey(j.privateKey)

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyDER,
	})

	return string(privateKeyPEM), nil
}

// GetRevokedTokensCount returns the number of revoked tokens
func (j *JWTManager) GetRevokedTokensCount() int {
	j.mu.RLock()
	defer j.mu.RUnlock()

	return len(j.revokedTokens)
}

// GenerateAPIKey generates a long-lived API key token
func (j *JWTManager) GenerateAPIKey(userID uuid.UUID, tenantID uuid.UUID, scopes []string, expiresAt *time.Time) (string, error) {
	claims := &Claims{
		UserID:    userID,
		TenantID:  tenantID,
		Scopes:    scopes,
		TokenType: TokenTypeAPI,
		TokenID:   uuid.New().String(),
	}

	now := time.Now()

	// Set custom expiration if provided
	var exp time.Time
	if expiresAt != nil {
		exp = *expiresAt
	} else {
		exp = now.Add(365 * 24 * time.Hour) // 1 year default
	}

	claims.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    j.issuer,
		Subject:   userID.String(),
		Audience:  []string{j.issuer},
		ExpiresAt: jwt.NewNumericDate(exp),
		NotBefore: jwt.NewNumericDate(now),
		IssuedAt:  jwt.NewNumericDate(now),
		ID:        claims.TokenID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(j.privateKey)
}

// ValidateAPIKey validates an API key token
func (j *JWTManager) ValidateAPIKey(tokenString string) (*Claims, error) {
	claims, err := j.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != TokenTypeAPI {
		return nil, fmt.Errorf("token is not an API key")
	}

	return claims, nil
}

// CreateTokenPair creates both access and refresh tokens
func (j *JWTManager) CreateTokenPair(userID uuid.UUID, username, email string, tenantID uuid.UUID, roles []Role, tenantRole TenantRole, scopes []string) (string, string, error) {
	// Get permissions from roles
	var permissions []Permission
	for _, role := range roles {
		permissions = append(permissions, GetRolePermissions(role)...)
	}

	// Add tenant role permissions
	permissions = append(permissions, GetTenantRolePermissions(tenantRole)...)

	// Remove duplicates
	permissionMap := make(map[Permission]bool)
	var uniquePermissions []Permission
	for _, perm := range permissions {
		if !permissionMap[perm] {
			permissionMap[perm] = true
			uniquePermissions = append(uniquePermissions, perm)
		}
	}

	// Create access token
	accessClaims := &Claims{
		UserID:      userID,
		Username:    username,
		Email:       email,
		TenantID:    tenantID,
		Roles:       roles,
		TenantRole:  tenantRole,
		Permissions: uniquePermissions,
		Scopes:      scopes,
		TokenType:   TokenTypeAccess,
	}

	accessToken, err := j.GenerateToken(accessClaims)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access token: %w", err)
	}

	// Create refresh token (with minimal claims)
	refreshClaims := &Claims{
		UserID:    userID,
		Username:  username,
		Email:     email,
		TenantID:  tenantID,
		TokenType: TokenTypeRefresh,
	}

	refreshToken, err := j.GenerateToken(refreshClaims)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}
