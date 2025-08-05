package auth

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Service implements the AuthService interface
type Service struct {
	tokenManager TokenManager
	userStore    UserStore
	tenantStore  TenantStore
	config       *ServiceConfig
	metrics      *AuthMetrics

	// Rate limiting and security
	loginAttempts map[string]*LoginAttempt
	mu            sync.RWMutex

	// Password validation
	passwordPolicy *PasswordPolicy
}

// ServiceConfig contains service configuration
type ServiceConfig struct {
	DefaultTokenTTL          time.Duration `yaml:"default_token_ttl"`
	MaxLoginAttempts         int           `yaml:"max_login_attempts"`
	LockoutDuration          time.Duration `yaml:"lockout_duration"`
	PasswordCost             int           `yaml:"password_cost"`
	RequireEmailVerification bool          `yaml:"require_email_verification"`
	EnableMFA                bool          `yaml:"enable_mfa"`
	SessionTimeout           time.Duration `yaml:"session_timeout"`
	CleanupInterval          time.Duration `yaml:"cleanup_interval"`
}

// LoginAttempt tracks login attempts for rate limiting
type LoginAttempt struct {
	Count       int
	LastAttempt time.Time
	LockedUntil *time.Time
}

// DefaultServiceConfig returns default service configuration
func DefaultServiceConfig() *ServiceConfig {
	return &ServiceConfig{
		DefaultTokenTTL:          24 * time.Hour,
		MaxLoginAttempts:         5,
		LockoutDuration:          15 * time.Minute,
		PasswordCost:             12,
		RequireEmailVerification: false,
		EnableMFA:                false,
		SessionTimeout:           24 * time.Hour,
		CleanupInterval:          1 * time.Hour,
	}
}

// NewService creates a new authentication service
func NewService(tokenManager TokenManager, userStore UserStore, tenantStore TenantStore, config *ServiceConfig) *Service {
	if config == nil {
		config = DefaultServiceConfig()
	}

	service := &Service{
		tokenManager: tokenManager,
		userStore:    userStore,
		tenantStore:  tenantStore,
		config:       config,
		metrics: &AuthMetrics{
			LoginsByHour:   make(map[int]int64),
			FailureReasons: make(map[string]int64),
			LastUpdated:    time.Now(),
		},
		loginAttempts: make(map[string]*LoginAttempt),
		passwordPolicy: func() *PasswordPolicy {
			settings := DefaultTenantSettings()
			return &settings.PasswordPolicy
		}(),
	}

	// Start cleanup goroutine
	go service.startCleanup()

	return service
}

// Authenticate authenticates a user and returns tokens
func (s *Service) Authenticate(ctx context.Context, req *AuthRequest) (*AuthResponse, error) {
	switch req.Type {
	case AuthTypePassword:
		return s.authenticatePassword(ctx, req)
	case AuthTypeAPIKey:
		return s.authenticateAPIKey(ctx, req)
	case AuthTypeToken:
		return s.authenticateToken(ctx, req)
	default:
		s.recordFailure("unsupported_auth_type")
		return nil, fmt.Errorf("unsupported authentication type: %s", req.Type)
	}
}

// authenticatePassword authenticates using username/email and password
func (s *Service) authenticatePassword(ctx context.Context, req *AuthRequest) (*AuthResponse, error) {
	// Determine identifier (username or email)
	identifier := req.Username
	if identifier == "" {
		identifier = req.Email
	}

	if identifier == "" || req.Password == "" {
		s.recordFailure("missing_credentials")
		return nil, ErrInvalidCredentials
	}

	// Check rate limiting
	if err := s.checkRateLimit(identifier); err != nil {
		s.recordFailure("rate_limited")
		return nil, err
	}

	// Get user
	var user *User
	var err error

	if strings.Contains(identifier, "@") {
		user, err = s.userStore.GetUserByEmail(ctx, identifier)
	} else {
		user, err = s.userStore.GetUserByUsername(ctx, identifier)
	}

	if err != nil {
		s.recordLoginAttempt(identifier, false)
		s.recordFailure("user_not_found")
		return nil, ErrInvalidCredentials // Don't reveal if user exists
	}

	// Check user status
	if err := s.checkUserStatus(user); err != nil {
		s.recordLoginAttempt(identifier, false)
		s.recordFailure("user_inactive")
		return nil, err
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		s.recordLoginAttempt(identifier, false)
		s.recordFailure("invalid_password")
		return nil, ErrInvalidCredentials
	}

	// Get tenant if specified
	var tenant *Tenant
	tenantID := req.TenantID
	if tenantID == uuid.Nil && len(user.Roles) > 0 {
		// Get user's tenants and use the first one
		userTenants, err := s.tenantStore.GetUserTenants(ctx, user.ID)
		if err == nil && len(userTenants) > 0 {
			tenantID = userTenants[0].TenantID
		}
	}

	if tenantID != uuid.Nil {
		tenant, err = s.tenantStore.GetTenant(ctx, tenantID)
		if err != nil {
			s.recordLoginAttempt(identifier, false)
			s.recordFailure("tenant_not_found")
			return nil, ErrTenantNotFound
		}

		if err := s.checkTenantStatus(tenant); err != nil {
			s.recordLoginAttempt(identifier, false)
			s.recordFailure("tenant_inactive")
			return nil, err
		}
	}

	// Get user's role in tenant
	var tenantRole TenantRole
	if tenant != nil {
		userTenants, err := s.tenantStore.GetUserTenants(ctx, user.ID)
		if err == nil {
			for _, ut := range userTenants {
				if ut.TenantID == tenant.ID {
					tenantRole = ut.Role
					break
				}
			}
		}
	}

	// Generate tokens
	accessToken, refreshToken, err := s.tokenManager.(*JWTManager).CreateTokenPair(
		user.ID, user.Username, user.Email, tenantID, user.Roles, tenantRole, req.Scopes,
	)
	if err != nil {
		s.recordLoginAttempt(identifier, false)
		s.recordFailure("token_generation_failed")
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	// Update user's last login
	now := time.Now()
	user.LastLoginAt = &now
	s.userStore.UpdateUser(ctx, user)

	// Record successful login
	s.recordLoginAttempt(identifier, true)
	s.recordSuccess()

	// Determine expiration
	expiresIn := s.config.DefaultTokenTTL
	if req.ExpiresIn != nil {
		expiresIn = *req.ExpiresIn
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(expiresIn.Seconds()),
		ExpiresAt:    time.Now().Add(expiresIn),
		User:         user,
		Tenant:       tenant,
		Scopes:       req.Scopes,
	}, nil
}

// authenticateAPIKey authenticates using API key
func (s *Service) authenticateAPIKey(ctx context.Context, req *AuthRequest) (*AuthResponse, error) {
	if req.APIKey == "" {
		s.recordFailure("missing_api_key")
		return nil, fmt.Errorf("API key is required")
	}

	// Validate API key token
	jwtManager, ok := s.tokenManager.(*JWTManager)
	if !ok {
		s.recordFailure("invalid_token_manager")
		return nil, fmt.Errorf("invalid token manager for API key authentication")
	}

	claims, err := jwtManager.ValidateAPIKey(req.APIKey)
	if err != nil {
		s.recordFailure("invalid_api_key")
		return nil, fmt.Errorf("invalid API key: %w", err)
	}

	// Get user
	user, err := s.userStore.GetUser(ctx, claims.UserID)
	if err != nil {
		s.recordFailure("user_not_found")
		return nil, ErrUserNotFound
	}

	// Check user status
	if err := s.checkUserStatus(user); err != nil {
		s.recordFailure("user_inactive")
		return nil, err
	}

	// Get tenant if specified
	var tenant *Tenant
	if claims.TenantID != uuid.Nil {
		tenant, err = s.tenantStore.GetTenant(ctx, claims.TenantID)
		if err != nil {
			s.recordFailure("tenant_not_found")
			return nil, ErrTenantNotFound
		}

		if err := s.checkTenantStatus(tenant); err != nil {
			s.recordFailure("tenant_inactive")
			return nil, err
		}
	}

	s.recordSuccess()

	return &AuthResponse{
		AccessToken: req.APIKey,
		TokenType:   "Bearer",
		ExpiresIn:   int64(claims.RegisteredClaims.ExpiresAt.Sub(time.Now()).Seconds()),
		ExpiresAt:   claims.RegisteredClaims.ExpiresAt.Time,
		User:        user,
		Tenant:      tenant,
		Scopes:      claims.Scopes,
	}, nil
}

// authenticateToken authenticates using existing token
func (s *Service) authenticateToken(ctx context.Context, req *AuthRequest) (*AuthResponse, error) {
	if req.Token == "" {
		s.recordFailure("missing_token")
		return nil, fmt.Errorf("token is required")
	}

	// Validate token
	claims, err := s.tokenManager.ValidateToken(req.Token)
	if err != nil {
		s.recordFailure("invalid_token")
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	// Get user
	user, err := s.userStore.GetUser(ctx, claims.UserID)
	if err != nil {
		s.recordFailure("user_not_found")
		return nil, ErrUserNotFound
	}

	// Check user status
	if err := s.checkUserStatus(user); err != nil {
		s.recordFailure("user_inactive")
		return nil, err
	}

	// Get tenant if specified
	var tenant *Tenant
	if claims.TenantID != uuid.Nil {
		tenant, err = s.tenantStore.GetTenant(ctx, claims.TenantID)
		if err != nil {
			s.recordFailure("tenant_not_found")
			return nil, ErrTenantNotFound
		}

		if err := s.checkTenantStatus(tenant); err != nil {
			s.recordFailure("tenant_inactive")
			return nil, err
		}
	}

	s.recordSuccess()

	return &AuthResponse{
		AccessToken: req.Token,
		TokenType:   "Bearer",
		ExpiresIn:   int64(claims.RegisteredClaims.ExpiresAt.Sub(time.Now()).Seconds()),
		ExpiresAt:   claims.RegisteredClaims.ExpiresAt.Time,
		User:        user,
		Tenant:      tenant,
		Scopes:      claims.Scopes,
	}, nil
}

// RefreshToken refreshes an access token using a refresh token
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*AuthResponse, error) {
	// Validate refresh token and get new access token
	accessToken, err := s.tokenManager.RefreshToken(refreshToken)
	if err != nil {
		s.recordFailure("refresh_failed")
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	// Extract claims from new access token
	claims, err := s.tokenManager.ExtractClaims(accessToken)
	if err != nil {
		s.recordFailure("invalid_refreshed_token")
		return nil, fmt.Errorf("invalid refreshed token: %w", err)
	}

	// Get user
	user, err := s.userStore.GetUser(ctx, claims.UserID)
	if err != nil {
		s.recordFailure("user_not_found")
		return nil, ErrUserNotFound
	}

	// Get tenant if specified
	var tenant *Tenant
	if claims.TenantID != uuid.Nil {
		tenant, err = s.tenantStore.GetTenant(ctx, claims.TenantID)
		if err == nil {
			// Don't fail if tenant not found during refresh
		}
	}

	s.recordSuccess()

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken, // Keep the same refresh token
		TokenType:    "Bearer",
		ExpiresIn:    int64(claims.RegisteredClaims.ExpiresAt.Sub(time.Now()).Seconds()),
		ExpiresAt:    claims.RegisteredClaims.ExpiresAt.Time,
		User:         user,
		Tenant:       tenant,
		Scopes:       claims.Scopes,
	}, nil
}

// Logout logs out a user by revoking their token
func (s *Service) Logout(ctx context.Context, userID uuid.UUID, tokenID string) error {
	if tokenID != "" {
		return s.tokenManager.RevokeToken(tokenID)
	}
	return nil
}

// LogoutAll logs out a user from all sessions by revoking all their tokens
func (s *Service) LogoutAll(ctx context.Context, userID uuid.UUID) error {
	// Note: This is a simplified implementation
	// In a real system, you'd want to track all tokens per user
	// and revoke them individually
	return nil
}

// ValidateToken validates a token and returns claims
func (s *Service) ValidateToken(ctx context.Context, token string) (*Claims, error) {
	return s.tokenManager.ValidateToken(token)
}

// RevokeToken revokes a specific token
func (s *Service) RevokeToken(ctx context.Context, tokenID string) error {
	return s.tokenManager.RevokeToken(tokenID)
}

// CreateUser creates a new user
func (s *Service) CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error) {
	// Validate input
	if err := s.validateCreateUserRequest(req); err != nil {
		return nil, err
	}

	// Check if user already exists
	if existingUser, _ := s.userStore.GetUserByEmail(ctx, req.Email); existingUser != nil {
		return nil, ErrUserAlreadyExists
	}

	if existingUser, _ := s.userStore.GetUserByUsername(ctx, req.Username); existingUser != nil {
		return nil, ErrUserAlreadyExists
	}

	// Hash password
	passwordHash, err := s.hashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user
	user := &User{
		ID:           uuid.New(),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: passwordHash,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Status:       UserStatusActive,
		Roles:        req.Roles,
		Permissions:  []Permission{},
		Metadata:     req.Metadata,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		ExpiresAt:    req.ExpiresAt,
	}

	if user.Metadata == nil {
		user.Metadata = make(map[string]string)
	}

	// Calculate permissions from roles
	for _, role := range user.Roles {
		user.Permissions = append(user.Permissions, GetRolePermissions(role)...)
	}

	// Store user
	if err := s.userStore.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Add user to tenant if specified
	if req.TenantID != nil && req.TenantRole != nil {
		if err := s.tenantStore.AddUserToTenant(ctx, user.ID, *req.TenantID, *req.TenantRole); err != nil {
			// Log error but don't fail user creation
		}
	}

	s.updateMetrics()

	return user, nil
}

// GetUser retrieves a user by ID
func (s *Service) GetUser(ctx context.Context, userID uuid.UUID) (*User, error) {
	user, err := s.userStore.GetUser(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

// UpdateUser updates a user
func (s *Service) UpdateUser(ctx context.Context, userID uuid.UUID, req *UpdateUserRequest) (*User, error) {
	// Get existing user
	user, err := s.userStore.GetUser(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Update fields
	if req.Username != nil {
		user.Username = *req.Username
	}
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.FirstName != nil {
		user.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		user.LastName = *req.LastName
	}
	if req.Status != nil {
		user.Status = *req.Status
	}
	if req.Metadata != nil {
		user.Metadata = req.Metadata
	}
	if req.ExpiresAt != nil {
		user.ExpiresAt = req.ExpiresAt
	}

	user.UpdatedAt = time.Now()

	// Store updated user
	if err := s.userStore.UpdateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return user, nil
}

// DeleteUser deletes a user
func (s *Service) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	return s.userStore.DeleteUser(ctx, userID)
}

// AssignRole assigns a role to a user
func (s *Service) AssignRole(ctx context.Context, userID uuid.UUID, role Role) error {
	user, err := s.userStore.GetUser(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	// Check if role already assigned
	for _, existingRole := range user.Roles {
		if existingRole == role {
			return nil // Already has role
		}
	}

	// Add role
	user.Roles = append(user.Roles, role)

	// Update permissions
	user.Permissions = append(user.Permissions, GetRolePermissions(role)...)

	user.UpdatedAt = time.Now()

	return s.userStore.UpdateUser(ctx, user)
}

// RevokeRole revokes a role from a user
func (s *Service) RevokeRole(ctx context.Context, userID uuid.UUID, role Role) error {
	user, err := s.userStore.GetUser(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	// Remove role
	var newRoles []Role
	for _, existingRole := range user.Roles {
		if existingRole != role {
			newRoles = append(newRoles, existingRole)
		}
	}
	user.Roles = newRoles

	// Recalculate permissions
	var newPermissions []Permission
	for _, userRole := range user.Roles {
		newPermissions = append(newPermissions, GetRolePermissions(userRole)...)
	}
	user.Permissions = newPermissions

	user.UpdatedAt = time.Now()

	return s.userStore.UpdateUser(ctx, user)
}

// CheckPermission checks if a user has a specific permission
func (s *Service) CheckPermission(ctx context.Context, userID uuid.UUID, permission Permission) (bool, error) {
	user, err := s.userStore.GetUser(ctx, userID)
	if err != nil {
		return false, ErrUserNotFound
	}

	// Check direct permissions
	for _, userPerm := range user.Permissions {
		if userPerm == permission {
			return true, nil
		}
	}

	// Check role-based permissions
	for _, role := range user.Roles {
		rolePermissions := GetRolePermissions(role)
		for _, rolePerm := range rolePermissions {
			if rolePerm == permission {
				return true, nil
			}
		}
	}

	return false, nil
}

// CreateTenant creates a new tenant
func (s *Service) CreateTenant(ctx context.Context, req *CreateTenantRequest) (*Tenant, error) {
	// Validate request
	if req.Name == "" {
		return nil, fmt.Errorf("tenant name is required")
	}

	// Check if tenant already exists
	// Note: This would need to be implemented in the tenant store

	// Create tenant
	tenant := &Tenant{
		ID:          uuid.New(),
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Domain:      req.Domain,
		Status:      TenantStatusActive,
		Settings:    DefaultTenantSettings(),
		Metadata:    req.Metadata,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if req.Settings != nil {
		tenant.Settings = *req.Settings
	}

	if tenant.Metadata == nil {
		tenant.Metadata = make(map[string]string)
	}

	// Store tenant
	if err := s.tenantStore.CreateTenant(ctx, tenant); err != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}

	// Create admin user if specified
	if req.AdminUser != nil {
		req.AdminUser.TenantID = &tenant.ID
		adminRole := TenantRoleOwner
		req.AdminUser.TenantRole = &adminRole
		req.AdminUser.Roles = []Role{RoleAdmin}

		_, err := s.CreateUser(ctx, req.AdminUser)
		if err != nil {
			// Log error but don't fail tenant creation
		}
	}

	return tenant, nil
}

// GetTenant retrieves a tenant by ID
func (s *Service) GetTenant(ctx context.Context, tenantID uuid.UUID) (*Tenant, error) {
	tenant, err := s.tenantStore.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, ErrTenantNotFound
	}
	return tenant, nil
}

// AddUserToTenant adds a user to a tenant with a specific role
func (s *Service) AddUserToTenant(ctx context.Context, userID, tenantID uuid.UUID, role TenantRole) error {
	return s.tenantStore.AddUserToTenant(ctx, userID, tenantID, role)
}

// RemoveUserFromTenant removes a user from a tenant
func (s *Service) RemoveUserFromTenant(ctx context.Context, userID, tenantID uuid.UUID) error {
	return s.tenantStore.RemoveUserFromTenant(ctx, userID, tenantID)
}

// HealthCheck performs a health check
func (s *Service) HealthCheck(ctx context.Context) error {
	// Check token manager
	testClaims := &Claims{
		UserID:    uuid.New(),
		TokenType: TokenTypeAccess,
	}

	token, err := s.tokenManager.GenerateToken(testClaims)
	if err != nil {
		return fmt.Errorf("token generation failed: %w", err)
	}

	_, err = s.tokenManager.ValidateToken(token)
	if err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}

	return nil
}

// GetMetrics returns authentication metrics
func (s *Service) GetMetrics(ctx context.Context) (*AuthMetrics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Update metrics
	s.metrics.LastUpdated = time.Now()

	// Copy metrics to avoid race conditions
	metrics := &AuthMetrics{
		TotalUsers:           s.metrics.TotalUsers,
		ActiveUsers:          s.metrics.ActiveUsers,
		TotalTenants:         s.metrics.TotalTenants,
		ActiveTenants:        s.metrics.ActiveTenants,
		TotalLogins:          s.metrics.TotalLogins,
		FailedLogins:         s.metrics.FailedLogins,
		ActiveSessions:       s.metrics.ActiveSessions,
		TokensIssued:         s.metrics.TokensIssued,
		TokensRevoked:        s.metrics.TokensRevoked,
		AverageSessionLength: s.metrics.AverageSessionLength,
		LoginsByHour:         make(map[int]int64),
		FailureReasons:       make(map[string]int64),
		LastUpdated:          s.metrics.LastUpdated,
	}

	for k, v := range s.metrics.LoginsByHour {
		metrics.LoginsByHour[k] = v
	}

	for k, v := range s.metrics.FailureReasons {
		metrics.FailureReasons[k] = v
	}

	return metrics, nil
}

// Helper methods

// validateCreateUserRequest validates a create user request
func (s *Service) validateCreateUserRequest(req *CreateUserRequest) error {
	if req.Username == "" {
		return fmt.Errorf("username is required")
	}

	if req.Email == "" {
		return fmt.Errorf("email is required")
	}

	if !s.isValidEmail(req.Email) {
		return fmt.Errorf("invalid email format")
	}

	if req.Password == "" {
		return fmt.Errorf("password is required")
	}

	if err := s.validatePassword(req.Password); err != nil {
		return err
	}

	return nil
}

// isValidEmail validates email format
func (s *Service) isValidEmail(email string) bool {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

// validatePassword validates password against policy
func (s *Service) validatePassword(password string) error {
	policy := s.passwordPolicy

	if len(password) < policy.MinLength {
		return ErrWeakPassword
	}

	if policy.RequireUppercase && !regexp.MustCompile(`[A-Z]`).MatchString(password) {
		return ErrWeakPassword
	}

	if policy.RequireLowercase && !regexp.MustCompile(`[a-z]`).MatchString(password) {
		return ErrWeakPassword
	}

	if policy.RequireNumbers && !regexp.MustCompile(`[0-9]`).MatchString(password) {
		return ErrWeakPassword
	}

	if policy.RequireSymbols && !regexp.MustCompile(`[^a-zA-Z0-9]`).MatchString(password) {
		return ErrWeakPassword
	}

	return nil
}

// hashPassword hashes a password
func (s *Service) hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.config.PasswordCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// checkUserStatus checks if user status allows authentication
func (s *Service) checkUserStatus(user *User) error {
	switch user.Status {
	case UserStatusInactive:
		return ErrUserInactive
	case UserStatusSuspended:
		return ErrUserSuspended
	case UserStatusExpired:
		return ErrUserExpired
	case UserStatusPending:
		return fmt.Errorf("user account is pending activation")
	}

	// Check expiration
	if user.ExpiresAt != nil && time.Now().After(*user.ExpiresAt) {
		return ErrUserExpired
	}

	return nil
}

// checkTenantStatus checks if tenant status allows access
func (s *Service) checkTenantStatus(tenant *Tenant) error {
	switch tenant.Status {
	case TenantStatusInactive:
		return ErrTenantInactive
	case TenantStatusSuspended:
		return fmt.Errorf("tenant is suspended")
	case TenantStatusExpired:
		return fmt.Errorf("tenant has expired")
	}

	// Check expiration
	if tenant.ExpiresAt != nil && time.Now().After(*tenant.ExpiresAt) {
		return fmt.Errorf("tenant has expired")
	}

	return nil
}

// checkRateLimit checks if user is rate limited
func (s *Service) checkRateLimit(identifier string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	attempt, exists := s.loginAttempts[identifier]
	if !exists {
		return nil
	}

	// Check if locked
	if attempt.LockedUntil != nil && time.Now().Before(*attempt.LockedUntil) {
		return ErrTooManyAttempts
	}

	// Reset if lockout expired
	if attempt.LockedUntil != nil && time.Now().After(*attempt.LockedUntil) {
		delete(s.loginAttempts, identifier)
	}

	return nil
}

// recordLoginAttempt records a login attempt
func (s *Service) recordLoginAttempt(identifier string, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if success {
		// Remove from failed attempts on success
		delete(s.loginAttempts, identifier)
		return
	}

	// Record failed attempt
	attempt, exists := s.loginAttempts[identifier]
	if !exists {
		attempt = &LoginAttempt{}
		s.loginAttempts[identifier] = attempt
	}

	attempt.Count++
	attempt.LastAttempt = time.Now()

	// Lock if too many attempts
	if attempt.Count >= s.config.MaxLoginAttempts {
		lockUntil := time.Now().Add(s.config.LockoutDuration)
		attempt.LockedUntil = &lockUntil
	}
}

// recordSuccess records successful authentication
func (s *Service) recordSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.TotalLogins++
	hour := time.Now().Hour()
	s.metrics.LoginsByHour[hour]++
}

// recordFailure records authentication failure
func (s *Service) recordFailure(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.FailedLogins++
	s.metrics.FailureReasons[reason]++
}

// updateMetrics updates authentication metrics
func (s *Service) updateMetrics() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// This would be implemented to update user/tenant counts
	// from the stores
}

// startCleanup starts the cleanup goroutine
func (s *Service) startCleanup() {
	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		s.cleanup()
	}
}

// cleanup performs periodic cleanup
func (s *Service) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Clean up expired login attempts
	for identifier, attempt := range s.loginAttempts {
		if attempt.LockedUntil != nil && now.After(*attempt.LockedUntil) {
			delete(s.loginAttempts, identifier)
		} else if now.Sub(attempt.LastAttempt) > 24*time.Hour {
			delete(s.loginAttempts, identifier)
		}
	}
}
