package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AuthMiddleware provides authentication middleware for HTTP handlers
type AuthMiddleware struct {
	authService AuthService
	config      *MiddlewareConfig
}

// MiddlewareConfig contains middleware configuration
type MiddlewareConfig struct {
	RequireAuth       bool     `yaml:"require_auth"`
	AllowedPaths      []string `yaml:"allowed_paths"`
	RequiredScopes    []string `yaml:"required_scopes,omitempty"`
	RequiredPermissions []Permission `yaml:"required_permissions,omitempty"`
	TokenHeader       string   `yaml:"token_header"`
	TokenPrefix       string   `yaml:"token_prefix"`
	APIKeyHeader      string   `yaml:"api_key_header"`
	CookieName        string   `yaml:"cookie_name"`
	SkipPaths         []string `yaml:"skip_paths"`
}

// DefaultMiddlewareConfig returns default middleware configuration
func DefaultMiddlewareConfig() *MiddlewareConfig {
	return &MiddlewareConfig{
		RequireAuth:    true,
		AllowedPaths:   []string{"/health", "/metrics", "/auth/login", "/auth/register"},
		TokenHeader:    "Authorization",
		TokenPrefix:    "Bearer ",
		APIKeyHeader:   "X-API-Key",
		CookieName:     "auth_token",
		SkipPaths:      []string{"/health", "/metrics", "/swagger"},
	}
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(authService AuthService, config *MiddlewareConfig) *AuthMiddleware {
	if config == nil {
		config = DefaultMiddlewareConfig()
	}
	
	return &AuthMiddleware{
		authService: authService,
		config:      config,
	}
}

// AuthenticateGin returns a Gin middleware function for authentication
func (m *AuthMiddleware) AuthenticateGin() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip authentication for certain paths
		if m.shouldSkipAuth(c.Request.URL.Path) {
			c.Next()
			return
		}
		
		// Extract token
		token, authType := m.extractToken(c.Request)
		if token == "" {
			if m.config.RequireAuth {
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": "missing authentication token",
					"code":  "MISSING_TOKEN",
				})
				c.Abort()
				return
			}
			c.Next()
			return
		}
		
		// Authenticate
		authReq := &AuthRequest{
			Type: authType,
		}
		
		switch authType {
		case AuthTypeAPIKey:
			authReq.APIKey = token
		case AuthTypeToken:
			authReq.Token = token
		default:
			authReq.Token = token
		}
		
		authResp, err := m.authService.Authenticate(c.Request.Context(), authReq)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "authentication failed",
				"code":  "AUTH_FAILED",
				"details": err.Error(),
			})
			c.Abort()
			return
		}
		
		// Check scopes if required
		if len(m.config.RequiredScopes) > 0 {
			if !m.hasRequiredScopes(authResp.Scopes, m.config.RequiredScopes) {
				c.JSON(http.StatusForbidden, gin.H{
					"error": "insufficient scopes",
					"code":  "INSUFFICIENT_SCOPES",
				})
				c.Abort()
				return
			}
		}
		
		// Check permissions if required
		if len(m.config.RequiredPermissions) > 0 {
			claims, err := m.authService.ValidateToken(c.Request.Context(), token)
			if err != nil || !m.hasRequiredPermissions(claims.Permissions, m.config.RequiredPermissions) {
				c.JSON(http.StatusForbidden, gin.H{
					"error": "insufficient permissions",
					"code":  "INSUFFICIENT_PERMISSIONS",
				})
				c.Abort()
				return
			}
		}
		
		// Set user context
		m.setUserContext(c, authResp)
		
		c.Next()
	}
}

// RequirePermission returns middleware that requires specific permissions
func (m *AuthMiddleware) RequirePermission(permissions ...Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := GetUserFromContext(c)
		if user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required",
				"code":  "AUTH_REQUIRED",
			})
			c.Abort()
			return
		}
		
		// Get claims from context
		claims := GetClaimsFromContext(c)
		if claims == nil {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "invalid authentication context",
				"code":  "INVALID_AUTH_CONTEXT",
			})
			c.Abort()
			return
		}
		
		// Check permissions
		if !m.hasRequiredPermissions(claims.Permissions, permissions) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "insufficient permissions",
				"code":  "INSUFFICIENT_PERMISSIONS",
				"required": permissions,
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// RequireRole returns middleware that requires specific roles
func (m *AuthMiddleware) RequireRole(roles ...Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := GetUserFromContext(c)
		if user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required",
				"code":  "AUTH_REQUIRED",
			})
			c.Abort()
			return
		}
		
		// Check if user has any of the required roles
		hasRole := false
		for _, requiredRole := range roles {
			for _, userRole := range user.Roles {
				if userRole == requiredRole {
					hasRole = true
					break
				}
			}
			if hasRole {
				break
			}
		}
		
		if !hasRole {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "insufficient role",
				"code":  "INSUFFICIENT_ROLE",
				"required": roles,
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// RequireTenantRole returns middleware that requires specific tenant roles
func (m *AuthMiddleware) RequireTenantRole(tenantRoles ...TenantRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := GetClaimsFromContext(c)
		if claims == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required",
				"code":  "AUTH_REQUIRED",
			})
			c.Abort()
			return
		}
		
		// Check if user has any of the required tenant roles
		hasRole := false
		for _, requiredRole := range tenantRoles {
			if claims.TenantRole == requiredRole {
				hasRole = true
				break
			}
		}
		
		if !hasRole {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "insufficient tenant role",
				"code":  "INSUFFICIENT_TENANT_ROLE",
				"required": tenantRoles,
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// AuthenticateHTTP returns a standard HTTP middleware function
func (m *AuthMiddleware) AuthenticateHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip authentication for certain paths
		if m.shouldSkipAuth(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		
		// Extract token
		token, authType := m.extractToken(r)
		if token == "" {
			if m.config.RequireAuth {
				http.Error(w, "missing authentication token", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		
		// Authenticate
		authReq := &AuthRequest{
			Type: authType,
		}
		
		switch authType {
		case AuthTypeAPIKey:
			authReq.APIKey = token
		case AuthTypeToken:
			authReq.Token = token
		default:
			authReq.Token = token
		}
		
		authResp, err := m.authService.Authenticate(r.Context(), authReq)
		if err != nil {
			http.Error(w, "authentication failed: "+err.Error(), http.StatusUnauthorized)
			return
		}
		
		// Check permissions if required
		if len(m.config.RequiredPermissions) > 0 {
			claims, err := m.authService.ValidateToken(r.Context(), token)
			if err != nil || !m.hasRequiredPermissions(claims.Permissions, m.config.RequiredPermissions) {
				http.Error(w, "insufficient permissions", http.StatusForbidden)
				return
			}
		}
		
		// Set user context
		ctx := m.setHTTPUserContext(r.Context(), authResp)
		r = r.WithContext(ctx)
		
		next.ServeHTTP(w, r)
	})
}

// extractToken extracts authentication token from request
func (m *AuthMiddleware) extractToken(r *http.Request) (string, AuthType) {
	// Try Authorization header first
	authHeader := r.Header.Get(m.config.TokenHeader)
	if authHeader != "" {
		if strings.HasPrefix(authHeader, m.config.TokenPrefix) {
			token := strings.TrimPrefix(authHeader, m.config.TokenPrefix)
			return strings.TrimSpace(token), AuthTypeToken
		}
		// If no prefix, assume it's a raw token
		return strings.TrimSpace(authHeader), AuthTypeToken
	}
	
	// Try API key header
	apiKey := r.Header.Get(m.config.APIKeyHeader)
	if apiKey != "" {
		return strings.TrimSpace(apiKey), AuthTypeAPIKey
	}
	
	// Try cookie
	if m.config.CookieName != "" {
		cookie, err := r.Cookie(m.config.CookieName)
		if err == nil && cookie.Value != "" {
			return cookie.Value, AuthTypeToken
		}
	}
	
	// Try query parameter (less secure, but sometimes needed)
	token := r.URL.Query().Get("token")
	if token != "" {
		return token, AuthTypeToken
	}
	
	apiKey = r.URL.Query().Get("api_key")
	if apiKey != "" {
		return apiKey, AuthTypeAPIKey
	}
	
	return "", AuthTypeToken
}

// shouldSkipAuth checks if authentication should be skipped for a path
func (m *AuthMiddleware) shouldSkipAuth(path string) bool {
	for _, skipPath := range m.config.SkipPaths {
		if strings.HasPrefix(path, skipPath) {
			return true
		}
	}
	
	for _, allowedPath := range m.config.AllowedPaths {
		if path == allowedPath || strings.HasPrefix(path, allowedPath) {
			return true
		}
	}
	
	return false
}

// hasRequiredScopes checks if user has required scopes
func (m *AuthMiddleware) hasRequiredScopes(userScopes, requiredScopes []string) bool {
	if len(requiredScopes) == 0 {
		return true
	}
	
	scopeMap := make(map[string]bool)
	for _, scope := range userScopes {
		scopeMap[scope] = true
	}
	
	for _, required := range requiredScopes {
		if !scopeMap[required] {
			return false
		}
	}
	
	return true
}

// hasRequiredPermissions checks if user has required permissions
func (m *AuthMiddleware) hasRequiredPermissions(userPermissions, requiredPermissions []Permission) bool {
	if len(requiredPermissions) == 0 {
		return true
	}
	
	permissionMap := make(map[Permission]bool)
	for _, permission := range userPermissions {
		permissionMap[permission] = true
	}
	
	for _, required := range requiredPermissions {
		if !permissionMap[required] {
			return false
		}
	}
	
	return true
}

// setUserContext sets user information in Gin context
func (m *AuthMiddleware) setUserContext(c *gin.Context, authResp *AuthResponse) {
	c.Set("user", authResp.User)
	c.Set("tenant", authResp.Tenant)
	c.Set("scopes", authResp.Scopes)
	
	// Extract and set claims if possible
	if claims, err := m.authService.ValidateToken(c.Request.Context(), authResp.AccessToken); err == nil {
		c.Set("claims", claims)
		c.Set("user_id", claims.UserID)
		c.Set("tenant_id", claims.TenantID)
		c.Set("roles", claims.Roles)
		c.Set("tenant_role", claims.TenantRole)
		c.Set("permissions", claims.Permissions)
	}
}

// setHTTPUserContext sets user information in HTTP context
func (m *AuthMiddleware) setHTTPUserContext(ctx context.Context, authResp *AuthResponse) context.Context {
	ctx = context.WithValue(ctx, "user", authResp.User)
	ctx = context.WithValue(ctx, "tenant", authResp.Tenant)
	ctx = context.WithValue(ctx, "scopes", authResp.Scopes)
	
	// Extract and set claims if possible
	if claims, err := m.authService.ValidateToken(ctx, authResp.AccessToken); err == nil {
		ctx = context.WithValue(ctx, "claims", claims)
		ctx = context.WithValue(ctx, "user_id", claims.UserID)
		ctx = context.WithValue(ctx, "tenant_id", claims.TenantID)
		ctx = context.WithValue(ctx, "roles", claims.Roles)
		ctx = context.WithValue(ctx, "tenant_role", claims.TenantRole)
		ctx = context.WithValue(ctx, "permissions", claims.Permissions)
	}
	
	return ctx
}

// Context helper functions

// GetUserFromContext extracts user from Gin context
func GetUserFromContext(c *gin.Context) *User {
	if user, exists := c.Get("user"); exists {
		if u, ok := user.(*User); ok {
			return u
		}
	}
	return nil
}

// GetTenantFromContext extracts tenant from Gin context
func GetTenantFromContext(c *gin.Context) *Tenant {
	if tenant, exists := c.Get("tenant"); exists {
		if t, ok := tenant.(*Tenant); ok {
			return t
		}
	}
	return nil
}

// GetClaimsFromContext extracts claims from Gin context
func GetClaimsFromContext(c *gin.Context) *Claims {
	if claims, exists := c.Get("claims"); exists {
		if cl, ok := claims.(*Claims); ok {
			return cl
		}
	}
	return nil
}

// GetUserIDFromContext extracts user ID from Gin context
func GetUserIDFromContext(c *gin.Context) uuid.UUID {
	if userID, exists := c.Get("user_id"); exists {
		if id, ok := userID.(uuid.UUID); ok {
			return id
		}
	}
	return uuid.Nil
}

// GetTenantIDFromContext extracts tenant ID from Gin context
func GetTenantIDFromContext(c *gin.Context) uuid.UUID {
	if tenantID, exists := c.Get("tenant_id"); exists {
		if id, ok := tenantID.(uuid.UUID); ok {
			return id
		}
	}
	return uuid.Nil
}

// GetRolesFromContext extracts roles from Gin context
func GetRolesFromContext(c *gin.Context) []Role {
	if roles, exists := c.Get("roles"); exists {
		if r, ok := roles.([]Role); ok {
			return r
		}
	}
	return []Role{}
}

// GetPermissionsFromContext extracts permissions from Gin context
func GetPermissionsFromContext(c *gin.Context) []Permission {
	if permissions, exists := c.Get("permissions"); exists {
		if p, ok := permissions.([]Permission); ok {
			return p
		}
	}
	return []Permission{}
}

// GetScopesFromContext extracts scopes from Gin context
func GetScopesFromContext(c *gin.Context) []string {
	if scopes, exists := c.Get("scopes"); exists {
		if s, ok := scopes.([]string); ok {
			return s
		}
	}
	return []string{}
}

// HTTP Context helper functions

// GetUserFromHTTPContext extracts user from HTTP context
func GetUserFromHTTPContext(ctx context.Context) *User {
	if user := ctx.Value("user"); user != nil {
		if u, ok := user.(*User); ok {
			return u
		}
	}
	return nil
}

// GetTenantFromHTTPContext extracts tenant from HTTP context
func GetTenantFromHTTPContext(ctx context.Context) *Tenant {
	if tenant := ctx.Value("tenant"); tenant != nil {
		if t, ok := tenant.(*Tenant); ok {
			return t
		}
	}
	return nil
}

// GetClaimsFromHTTPContext extracts claims from HTTP context
func GetClaimsFromHTTPContext(ctx context.Context) *Claims {
	if claims := ctx.Value("claims"); claims != nil {
		if cl, ok := claims.(*Claims); ok {
			return cl
		}
	}
	return nil
}

// GetUserIDFromHTTPContext extracts user ID from HTTP context
func GetUserIDFromHTTPContext(ctx context.Context) uuid.UUID {
	if userID := ctx.Value("user_id"); userID != nil {
		if id, ok := userID.(uuid.UUID); ok {
			return id
		}
	}
	return uuid.Nil
}

// CORS middleware for authentication endpoints
func (m *AuthMiddleware) CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-API-Key")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")
		
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		
		c.Next()
	}
}

// RateLimitMiddleware provides basic rate limiting
func (m *AuthMiddleware) RateLimitMiddleware(requestsPerMinute int) gin.HandlerFunc {
	// This is a simplified rate limiter
	// In production, you'd use Redis or a proper rate limiting library
	clients := make(map[string][]int64)
	
	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		now := time.Now().Unix()
		
		// Clean old entries
		if requests, exists := clients[clientIP]; exists {
			var validRequests []int64
			for _, timestamp := range requests {
				if now-timestamp < 60 { // Within last minute
					validRequests = append(validRequests, timestamp)
				}
			}
			clients[clientIP] = validRequests
		}
		
		// Check rate limit
		if len(clients[clientIP]) >= requestsPerMinute {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
				"code":  "RATE_LIMIT_EXCEEDED",
			})
			c.Abort()
			return
		}
		
		// Add current request
		clients[clientIP] = append(clients[clientIP], now)
		
		c.Next()
	}
}