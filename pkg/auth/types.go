package auth

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AuthService defines the main authentication service interface
type AuthService interface {
	// Authentication
	Authenticate(ctx context.Context, req *AuthRequest) (*AuthResponse, error)
	RefreshToken(ctx context.Context, refreshToken string) (*AuthResponse, error)
	Logout(ctx context.Context, userID uuid.UUID, tokenID string) error
	LogoutAll(ctx context.Context, userID uuid.UUID) error

	// Token management
	ValidateToken(ctx context.Context, token string) (*Claims, error)
	RevokeToken(ctx context.Context, tokenID string) error

	// User management
	CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error)
	GetUser(ctx context.Context, userID uuid.UUID) (*User, error)
	UpdateUser(ctx context.Context, userID uuid.UUID, req *UpdateUserRequest) (*User, error)
	DeleteUser(ctx context.Context, userID uuid.UUID) error

	// Role and permission management
	AssignRole(ctx context.Context, userID uuid.UUID, role Role) error
	RevokeRole(ctx context.Context, userID uuid.UUID, role Role) error
	CheckPermission(ctx context.Context, userID uuid.UUID, permission Permission) (bool, error)

	// Tenant management
	CreateTenant(ctx context.Context, req *CreateTenantRequest) (*Tenant, error)
	GetTenant(ctx context.Context, tenantID uuid.UUID) (*Tenant, error)
	AddUserToTenant(ctx context.Context, userID, tenantID uuid.UUID, role TenantRole) error
	RemoveUserFromTenant(ctx context.Context, userID, tenantID uuid.UUID) error

	// Health and metrics
	HealthCheck(ctx context.Context) error
	GetMetrics(ctx context.Context) (*AuthMetrics, error)
}

// TokenManager handles JWT token operations
type TokenManager interface {
	GenerateToken(claims *Claims) (string, error)
	ValidateToken(tokenString string) (*Claims, error)
	RefreshToken(tokenString string) (string, error)
	ExtractClaims(tokenString string) (*Claims, error)
	IsTokenRevoked(tokenID string) bool
	RevokeToken(tokenID string) error
}

// UserStore defines user storage interface
type UserStore interface {
	CreateUser(ctx context.Context, user *User) error
	GetUser(ctx context.Context, userID uuid.UUID) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	UpdateUser(ctx context.Context, user *User) error
	DeleteUser(ctx context.Context, userID uuid.UUID) error
	ListUsers(ctx context.Context, filter *UserFilter) ([]*User, error)
}

// TenantStore defines tenant storage interface
type TenantStore interface {
	CreateTenant(ctx context.Context, tenant *Tenant) error
	GetTenant(ctx context.Context, tenantID uuid.UUID) (*Tenant, error)
	UpdateTenant(ctx context.Context, tenant *Tenant) error
	DeleteTenant(ctx context.Context, tenantID uuid.UUID) error
	ListTenants(ctx context.Context, filter *TenantFilter) ([]*Tenant, error)

	AddUserToTenant(ctx context.Context, userID, tenantID uuid.UUID, role TenantRole) error
	RemoveUserFromTenant(ctx context.Context, userID, tenantID uuid.UUID) error
	GetUserTenants(ctx context.Context, userID uuid.UUID) ([]*UserTenant, error)
	GetTenantUsers(ctx context.Context, tenantID uuid.UUID) ([]*TenantUser, error)
}

// AuthRequest represents an authentication request
type AuthRequest struct {
	Type      AuthType       `json:"type"`
	Username  string         `json:"username,omitempty"`
	Email     string         `json:"email,omitempty"`
	Password  string         `json:"password,omitempty"`
	APIKey    string         `json:"api_key,omitempty"`
	Token     string         `json:"token,omitempty"`
	TenantID  uuid.UUID      `json:"tenant_id,omitempty"`
	ExpiresIn *time.Duration `json:"expires_in,omitempty"`
	Scopes    []string       `json:"scopes,omitempty"`
}

// AuthResponse represents an authentication response
type AuthResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int64     `json:"expires_in"`
	ExpiresAt    time.Time `json:"expires_at"`
	User         *User     `json:"user"`
	Tenant       *Tenant   `json:"tenant,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
}

// Claims represents JWT claims
type Claims struct {
	UserID      uuid.UUID    `json:"user_id"`
	Username    string       `json:"username"`
	Email       string       `json:"email"`
	TenantID    uuid.UUID    `json:"tenant_id"`
	Roles       []Role       `json:"roles"`
	TenantRole  TenantRole   `json:"tenant_role"`
	Permissions []Permission `json:"permissions"`
	Scopes      []string     `json:"scopes,omitempty"`
	TokenID     string       `json:"token_id"`
	TokenType   TokenType    `json:"token_type"`
	jwt.RegisteredClaims
}

// User represents a user in the system
type User struct {
	ID           uuid.UUID         `json:"id" db:"id"`
	Username     string            `json:"username" db:"username"`
	Email        string            `json:"email" db:"email"`
	PasswordHash string            `json:"-" db:"password_hash"`
	FirstName    string            `json:"first_name" db:"first_name"`
	LastName     string            `json:"last_name" db:"last_name"`
	Status       UserStatus        `json:"status" db:"status"`
	Roles        []Role            `json:"roles" db:"roles"`
	Permissions  []Permission      `json:"permissions" db:"permissions"`
	Metadata     map[string]string `json:"metadata" db:"metadata"`
	CreatedAt    time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at" db:"updated_at"`
	LastLoginAt  *time.Time        `json:"last_login_at" db:"last_login_at"`
	ExpiresAt    *time.Time        `json:"expires_at" db:"expires_at"`
}

// Tenant represents a tenant in the system
type Tenant struct {
	ID          uuid.UUID         `json:"id" db:"id"`
	Name        string            `json:"name" db:"name"`
	DisplayName string            `json:"display_name" db:"display_name"`
	Domain      string            `json:"domain" db:"domain"`
	Status      TenantStatus      `json:"status" db:"status"`
	Settings    TenantSettings    `json:"settings" db:"settings"`
	Metadata    map[string]string `json:"metadata" db:"metadata"`
	CreatedAt   time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at" db:"updated_at"`
	ExpiresAt   *time.Time        `json:"expires_at" db:"expires_at"`
}

// TenantSettings represents tenant configuration
type TenantSettings struct {
	MaxUsers         int              `json:"max_users"`
	MaxStorage       int64            `json:"max_storage"`
	MaxAPIRequests   int              `json:"max_api_requests"`
	AllowedDomains   []string         `json:"allowed_domains"`
	RequireMFA       bool             `json:"require_mfa"`
	SessionTimeout   time.Duration    `json:"session_timeout"`
	PasswordPolicy   PasswordPolicy   `json:"password_policy"`
	SecuritySettings SecuritySettings `json:"security_settings"`
}

// PasswordPolicy defines password requirements
type PasswordPolicy struct {
	MinLength        int            `json:"min_length"`
	RequireUppercase bool           `json:"require_uppercase"`
	RequireLowercase bool           `json:"require_lowercase"`
	RequireNumbers   bool           `json:"require_numbers"`
	RequireSymbols   bool           `json:"require_symbols"`
	MaxAge           *time.Duration `json:"max_age,omitempty"`
	PreventReuse     int            `json:"prevent_reuse"`
}

// SecuritySettings defines security configuration
type SecuritySettings struct {
	AllowedIPs       []string      `json:"allowed_ips,omitempty"`
	BlockedIPs       []string      `json:"blocked_ips,omitempty"`
	MaxLoginAttempts int           `json:"max_login_attempts"`
	LockoutDuration  time.Duration `json:"lockout_duration"`
	RequireHTTPS     bool          `json:"require_https"`
	CORSOrigins      []string      `json:"cors_origins,omitempty"`
}

// UserTenant represents a user's association with a tenant
type UserTenant struct {
	UserID    uuid.UUID        `json:"user_id" db:"user_id"`
	TenantID  uuid.UUID        `json:"tenant_id" db:"tenant_id"`
	Role      TenantRole       `json:"role" db:"role"`
	Status    UserTenantStatus `json:"status" db:"status"`
	CreatedAt time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt time.Time        `json:"updated_at" db:"updated_at"`
}

// TenantUser represents a tenant's user
type TenantUser struct {
	User      *User            `json:"user"`
	Role      TenantRole       `json:"role"`
	Status    UserTenantStatus `json:"status"`
	CreatedAt time.Time        `json:"created_at"`
}

// CreateUserRequest represents a user creation request
type CreateUserRequest struct {
	Username   string            `json:"username"`
	Email      string            `json:"email"`
	Password   string            `json:"password"`
	FirstName  string            `json:"first_name"`
	LastName   string            `json:"last_name"`
	Roles      []Role            `json:"roles,omitempty"`
	TenantID   *uuid.UUID        `json:"tenant_id,omitempty"`
	TenantRole *TenantRole       `json:"tenant_role,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	ExpiresAt  *time.Time        `json:"expires_at,omitempty"`
}

// UpdateUserRequest represents a user update request
type UpdateUserRequest struct {
	Username  *string           `json:"username,omitempty"`
	Email     *string           `json:"email,omitempty"`
	FirstName *string           `json:"first_name,omitempty"`
	LastName  *string           `json:"last_name,omitempty"`
	Status    *UserStatus       `json:"status,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	ExpiresAt *time.Time        `json:"expires_at,omitempty"`
}

// CreateTenantRequest represents a tenant creation request
type CreateTenantRequest struct {
	Name        string             `json:"name"`
	DisplayName string             `json:"display_name"`
	Domain      string             `json:"domain"`
	Settings    *TenantSettings    `json:"settings,omitempty"`
	Metadata    map[string]string  `json:"metadata,omitempty"`
	AdminUser   *CreateUserRequest `json:"admin_user,omitempty"`
}

// UserFilter represents user filtering options
type UserFilter struct {
	TenantID *uuid.UUID  `json:"tenant_id,omitempty"`
	Status   *UserStatus `json:"status,omitempty"`
	Role     *Role       `json:"role,omitempty"`
	Search   string      `json:"search,omitempty"`
	Limit    int         `json:"limit,omitempty"`
	Offset   int         `json:"offset,omitempty"`
}

// TenantFilter represents tenant filtering options
type TenantFilter struct {
	Status *TenantStatus `json:"status,omitempty"`
	Domain string        `json:"domain,omitempty"`
	Search string        `json:"search,omitempty"`
	Limit  int           `json:"limit,omitempty"`
	Offset int           `json:"offset,omitempty"`
}

// AuthMetrics represents authentication metrics
type AuthMetrics struct {
	TotalUsers           int64            `json:"total_users"`
	ActiveUsers          int64            `json:"active_users"`
	TotalTenants         int64            `json:"total_tenants"`
	ActiveTenants        int64            `json:"active_tenants"`
	TotalLogins          int64            `json:"total_logins"`
	FailedLogins         int64            `json:"failed_logins"`
	ActiveSessions       int64            `json:"active_sessions"`
	TokensIssued         int64            `json:"tokens_issued"`
	TokensRevoked        int64            `json:"tokens_revoked"`
	AverageSessionLength time.Duration    `json:"average_session_length"`
	LoginsByHour         map[int]int64    `json:"logins_by_hour"`
	FailureReasons       map[string]int64 `json:"failure_reasons"`
	LastUpdated          time.Time        `json:"last_updated"`
}

// Enums and constants

// AuthType represents authentication method
type AuthType string

const (
	AuthTypePassword AuthType = "password"
	AuthTypeAPIKey   AuthType = "api_key"
	AuthTypeToken    AuthType = "token"
	AuthTypeMFA      AuthType = "mfa"
	AuthTypeSSO      AuthType = "sso"
)

// TokenType represents token type
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
	TokenTypeAPI     TokenType = "api"
)

// Role represents user roles
type Role string

const (
	RoleSuperAdmin     Role = "super_admin"
	RoleAdmin          Role = "admin"
	RoleUser           Role = "user"
	RoleViewer         Role = "viewer"
	RoleAPIClient      Role = "api_client"
	RoleServiceAccount Role = "service_account"
)

// TenantRole represents user role within a tenant
type TenantRole string

const (
	TenantRoleOwner  TenantRole = "owner"
	TenantRoleAdmin  TenantRole = "admin"
	TenantRoleMember TenantRole = "member"
	TenantRoleViewer TenantRole = "viewer"
	TenantRoleGuest  TenantRole = "guest"
)

// Permission represents specific permissions
type Permission string

const (
	// System permissions
	PermissionSystemRead  Permission = "system:read"
	PermissionSystemWrite Permission = "system:write"
	PermissionSystemAdmin Permission = "system:admin"

	// User permissions
	PermissionUserRead   Permission = "user:read"
	PermissionUserWrite  Permission = "user:write"
	PermissionUserDelete Permission = "user:delete"
	PermissionUserAdmin  Permission = "user:admin"

	// Tenant permissions
	PermissionTenantRead   Permission = "tenant:read"
	PermissionTenantWrite  Permission = "tenant:write"
	PermissionTenantDelete Permission = "tenant:delete"
	PermissionTenantAdmin  Permission = "tenant:admin"

	// Data permissions
	PermissionDataRead    Permission = "data:read"
	PermissionDataWrite   Permission = "data:write"
	PermissionDataDelete  Permission = "data:delete"
	PermissionDataProcess Permission = "data:process"
	PermissionDataExport  Permission = "data:export"

	// API permissions
	PermissionAPIRead  Permission = "api:read"
	PermissionAPIWrite Permission = "api:write"
	PermissionAPIAdmin Permission = "api:admin"

	// Workflow permissions
	PermissionWorkflowRead    Permission = "workflow:read"
	PermissionWorkflowWrite   Permission = "workflow:write"
	PermissionWorkflowExecute Permission = "workflow:execute"
	PermissionWorkflowAdmin   Permission = "workflow:admin"

	// Classification permissions
	PermissionClassifyRead  Permission = "classify:read"
	PermissionClassifyWrite Permission = "classify:write"
	PermissionClassifyAdmin Permission = "classify:admin"
)

// UserStatus represents user status
type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusInactive  UserStatus = "inactive"
	UserStatusSuspended UserStatus = "suspended"
	UserStatusPending   UserStatus = "pending"
	UserStatusExpired   UserStatus = "expired"
)

// TenantStatus represents tenant status
type TenantStatus string

const (
	TenantStatusActive    TenantStatus = "active"
	TenantStatusInactive  TenantStatus = "inactive"
	TenantStatusSuspended TenantStatus = "suspended"
	TenantStatusTrial     TenantStatus = "trial"
	TenantStatusExpired   TenantStatus = "expired"
)

// UserTenantStatus represents user-tenant association status
type UserTenantStatus string

const (
	UserTenantStatusActive   UserTenantStatus = "active"
	UserTenantStatusInactive UserTenantStatus = "inactive"
	UserTenantStatusPending  UserTenantStatus = "pending"
	UserTenantStatusRevoked  UserTenantStatus = "revoked"
)

// AuthError represents authentication errors
type AuthError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func (e *AuthError) Error() string {
	return e.Message
}

// Common authentication errors
var (
	ErrInvalidCredentials = &AuthError{
		Code:    "INVALID_CREDENTIALS",
		Message: "Invalid username or password",
	}

	ErrUserNotFound = &AuthError{
		Code:    "USER_NOT_FOUND",
		Message: "User not found",
	}

	ErrUserInactive = &AuthError{
		Code:    "USER_INACTIVE",
		Message: "User account is inactive",
	}

	ErrUserSuspended = &AuthError{
		Code:    "USER_SUSPENDED",
		Message: "User account is suspended",
	}

	ErrUserExpired = &AuthError{
		Code:    "USER_EXPIRED",
		Message: "User account has expired",
	}

	ErrInvalidToken = &AuthError{
		Code:    "INVALID_TOKEN",
		Message: "Invalid or expired token",
	}

	ErrTokenRevoked = &AuthError{
		Code:    "TOKEN_REVOKED",
		Message: "Token has been revoked",
	}

	ErrPermissionDenied = &AuthError{
		Code:    "PERMISSION_DENIED",
		Message: "Permission denied",
	}

	ErrTenantNotFound = &AuthError{
		Code:    "TENANT_NOT_FOUND",
		Message: "Tenant not found",
	}

	ErrTenantInactive = &AuthError{
		Code:    "TENANT_INACTIVE",
		Message: "Tenant is inactive",
	}

	ErrUserAlreadyExists = &AuthError{
		Code:    "USER_ALREADY_EXISTS",
		Message: "User already exists",
	}

	ErrTenantAlreadyExists = &AuthError{
		Code:    "TENANT_ALREADY_EXISTS",
		Message: "Tenant already exists",
	}

	ErrWeakPassword = &AuthError{
		Code:    "WEAK_PASSWORD",
		Message: "Password does not meet security requirements",
	}

	ErrTooManyAttempts = &AuthError{
		Code:    "TOO_MANY_ATTEMPTS",
		Message: "Too many failed login attempts",
	}
)

// DefaultTenantSettings returns default tenant settings
func DefaultTenantSettings() TenantSettings {
	return TenantSettings{
		MaxUsers:       100,
		MaxStorage:     10 * 1024 * 1024 * 1024, // 10GB
		MaxAPIRequests: 10000,
		RequireMFA:     false,
		SessionTimeout: 24 * time.Hour,
		PasswordPolicy: PasswordPolicy{
			MinLength:        8,
			RequireUppercase: true,
			RequireLowercase: true,
			RequireNumbers:   true,
			RequireSymbols:   false,
			PreventReuse:     5,
		},
		SecuritySettings: SecuritySettings{
			MaxLoginAttempts: 5,
			LockoutDuration:  15 * time.Minute,
			RequireHTTPS:     true,
		},
	}
}

// GetRolePermissions returns the permissions for a given role
func GetRolePermissions(role Role) []Permission {
	switch role {
	case RoleSuperAdmin:
		return []Permission{
			PermissionSystemRead, PermissionSystemWrite, PermissionSystemAdmin,
			PermissionUserRead, PermissionUserWrite, PermissionUserDelete, PermissionUserAdmin,
			PermissionTenantRead, PermissionTenantWrite, PermissionTenantDelete, PermissionTenantAdmin,
			PermissionDataRead, PermissionDataWrite, PermissionDataDelete, PermissionDataProcess, PermissionDataExport,
			PermissionAPIRead, PermissionAPIWrite, PermissionAPIAdmin,
			PermissionWorkflowRead, PermissionWorkflowWrite, PermissionWorkflowExecute, PermissionWorkflowAdmin,
			PermissionClassifyRead, PermissionClassifyWrite, PermissionClassifyAdmin,
		}
	case RoleAdmin:
		return []Permission{
			PermissionUserRead, PermissionUserWrite, PermissionUserAdmin,
			PermissionTenantRead, PermissionTenantWrite,
			PermissionDataRead, PermissionDataWrite, PermissionDataProcess, PermissionDataExport,
			PermissionAPIRead, PermissionAPIWrite,
			PermissionWorkflowRead, PermissionWorkflowWrite, PermissionWorkflowExecute,
			PermissionClassifyRead, PermissionClassifyWrite,
		}
	case RoleUser:
		return []Permission{
			PermissionDataRead, PermissionDataWrite, PermissionDataProcess,
			PermissionAPIRead, PermissionAPIWrite,
			PermissionWorkflowRead, PermissionWorkflowExecute,
			PermissionClassifyRead,
		}
	case RoleViewer:
		return []Permission{
			PermissionDataRead,
			PermissionAPIRead,
			PermissionWorkflowRead,
			PermissionClassifyRead,
		}
	case RoleAPIClient:
		return []Permission{
			PermissionAPIRead, PermissionAPIWrite,
			PermissionDataRead, PermissionDataWrite, PermissionDataProcess,
			PermissionClassifyRead, PermissionClassifyWrite,
		}
	case RoleServiceAccount:
		return []Permission{
			PermissionAPIRead, PermissionAPIWrite,
			PermissionDataRead, PermissionDataWrite, PermissionDataProcess,
			PermissionWorkflowRead, PermissionWorkflowExecute,
			PermissionClassifyRead, PermissionClassifyWrite,
		}
	default:
		return []Permission{}
	}
}

// GetTenantRolePermissions returns permissions for a tenant role
func GetTenantRolePermissions(role TenantRole) []Permission {
	switch role {
	case TenantRoleOwner:
		return []Permission{
			PermissionTenantRead, PermissionTenantWrite, PermissionTenantAdmin,
			PermissionUserRead, PermissionUserWrite, PermissionUserAdmin,
			PermissionDataRead, PermissionDataWrite, PermissionDataDelete, PermissionDataProcess, PermissionDataExport,
			PermissionWorkflowRead, PermissionWorkflowWrite, PermissionWorkflowExecute, PermissionWorkflowAdmin,
			PermissionClassifyRead, PermissionClassifyWrite, PermissionClassifyAdmin,
		}
	case TenantRoleAdmin:
		return []Permission{
			PermissionTenantRead, PermissionTenantWrite,
			PermissionUserRead, PermissionUserWrite,
			PermissionDataRead, PermissionDataWrite, PermissionDataProcess, PermissionDataExport,
			PermissionWorkflowRead, PermissionWorkflowWrite, PermissionWorkflowExecute,
			PermissionClassifyRead, PermissionClassifyWrite,
		}
	case TenantRoleMember:
		return []Permission{
			PermissionDataRead, PermissionDataWrite, PermissionDataProcess,
			PermissionWorkflowRead, PermissionWorkflowExecute,
			PermissionClassifyRead, PermissionClassifyWrite,
		}
	case TenantRoleViewer:
		return []Permission{
			PermissionDataRead,
			PermissionWorkflowRead,
			PermissionClassifyRead,
		}
	case TenantRoleGuest:
		return []Permission{
			PermissionDataRead,
		}
	default:
		return []Permission{}
	}
}
