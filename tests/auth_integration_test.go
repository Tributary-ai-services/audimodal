package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jscharber/eAIIngest/pkg/auth"
)

func TestAuthIntegration(t *testing.T) {
	ctx := context.Background()
	
	// Setup authentication system
	jwtManager, err := auth.NewJWTManager(nil)
	require.NoError(t, err)
	
	userStore := auth.NewMemoryUserStore()
	tenantStore := auth.NewMemoryTenantStore()
	
	// Seed test data
	require.NoError(t, userStore.SeedData())
	require.NoError(t, tenantStore.SeedData())
	
	authService := auth.NewService(jwtManager, userStore, tenantStore, nil)
	
	t.Run("UserLifecycle", func(t *testing.T) {
		testUserLifecycle(t, ctx, authService)
	})
	
	t.Run("TenantManagement", func(t *testing.T) {
		testTenantManagement(t, ctx, authService)
	})
	
	t.Run("TokenManagement", func(t *testing.T) {
		testTokenManagement(t, ctx, authService)
	})
	
	t.Run("PermissionSystem", func(t *testing.T) {
		testPermissionSystem(t, ctx, authService)
	})
	
	t.Run("MiddlewareIntegration", func(t *testing.T) {
		testMiddlewareIntegration(t, ctx, authService)
	})
	
	t.Run("MultiTenantAuth", func(t *testing.T) {
		testMultiTenantAuth(t, ctx, authService)
	})
}

func testUserLifecycle(t *testing.T, ctx context.Context, authService auth.AuthService) {
	// Create user
	createReq := &auth.CreateUserRequest{
		Username:  "lifecycle_user",
		Email:     "lifecycle@test.com",
		Password:  "SecurePass123!",
		FirstName: "Lifecycle",
		LastName:  "User",
		Roles:     []auth.Role{auth.RoleUser},
	}
	
	user, err := authService.CreateUser(ctx, createReq)
	require.NoError(t, err)
	assert.Equal(t, "lifecycle_user", user.Username)
	assert.Equal(t, auth.UserStatusActive, user.Status)
	
	// Authenticate user
	authReq := &auth.AuthRequest{
		Type:     auth.AuthTypePassword,
		Username: "lifecycle_user",
		Password: "SecurePass123!",
	}
	
	authResp, err := authService.Authenticate(ctx, authReq)
	require.NoError(t, err)
	assert.NotEmpty(t, authResp.AccessToken)
	
	// Update user
	updateReq := &auth.UpdateUserRequest{
		FirstName: stringPtr("Updated"),
		LastName:  stringPtr("Name"),
	}
	
	updatedUser, err := authService.UpdateUser(ctx, user.ID, updateReq)
	require.NoError(t, err)
	assert.Equal(t, "Updated", updatedUser.FirstName)
	assert.Equal(t, "Name", updatedUser.LastName)
	
	// Assign role
	err = authService.AssignRole(ctx, user.ID, auth.RoleAdmin)
	require.NoError(t, err)
	
	// Check permission
	hasPermission, err := authService.CheckPermission(ctx, user.ID, auth.PermissionUserRead)
	require.NoError(t, err)
	assert.True(t, hasPermission)
	
	// Revoke role
	err = authService.RevokeRole(ctx, user.ID, auth.RoleAdmin)
	require.NoError(t, err)
	
	// Delete user
	err = authService.DeleteUser(ctx, user.ID)
	require.NoError(t, err)
	
	// Verify user is deleted
	_, err = authService.GetUser(ctx, user.ID)
	assert.Error(t, err)
}

func testTenantManagement(t *testing.T, ctx context.Context, authService auth.AuthService) {
	// Create tenant with admin user
	adminUserReq := &auth.CreateUserRequest{
		Username:  "tenant_admin",
		Email:     "admin@tenant.com",
		Password:  "AdminPass123!",
		FirstName: "Tenant",
		LastName:  "Admin",
		Roles:     []auth.Role{auth.RoleAdmin},
	}
	
	createTenantReq := &auth.CreateTenantRequest{
		Name:        "test-tenant",
		DisplayName: "Test Tenant Organization",
		Domain:      "test-tenant.com",
		AdminUser:   adminUserReq,
	}
	
	tenant, err := authService.CreateTenant(ctx, createTenantReq)
	require.NoError(t, err)
	assert.Equal(t, "test-tenant", tenant.Name)
	assert.Equal(t, auth.TenantStatusActive, tenant.Status)
	
	// Create regular user
	userReq := &auth.CreateUserRequest{
		Username:  "tenant_user",
		Email:     "user@tenant.com",
		Password:  "UserPass123!",
		FirstName: "Tenant",
		LastName:  "User",
		Roles:     []auth.Role{auth.RoleUser},
	}
	
	user, err := authService.CreateUser(ctx, userReq)
	require.NoError(t, err)
	
	// Add user to tenant
	err = authService.AddUserToTenant(ctx, user.ID, tenant.ID, auth.TenantRoleMember)
	require.NoError(t, err)
	
	// Remove user from tenant
	err = authService.RemoveUserFromTenant(ctx, user.ID, tenant.ID)
	require.NoError(t, err)
}

func testTokenManagement(t *testing.T, ctx context.Context, authService auth.AuthService) {
	// Create test user
	createReq := &auth.CreateUserRequest{
		Username: "token_user",
		Email:    "token@test.com",
		Password: "TokenPass123!",
		Roles:    []auth.Role{auth.RoleUser},
	}
	
	user, err := authService.CreateUser(ctx, createReq)
	require.NoError(t, err)
	
	// Authenticate to get tokens
	authReq := &auth.AuthRequest{
		Type:     auth.AuthTypePassword,
		Username: "token_user",
		Password: "TokenPass123!",
	}
	
	authResp, err := authService.Authenticate(ctx, authReq)
	require.NoError(t, err)
	
	// Validate access token
	claims, err := authService.ValidateToken(ctx, authResp.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, user.ID, claims.UserID)
	
	// Refresh token
	newAuthResp, err := authService.RefreshToken(ctx, authResp.RefreshToken)
	require.NoError(t, err)
	assert.NotEmpty(t, newAuthResp.AccessToken)
	assert.NotEqual(t, authResp.AccessToken, newAuthResp.AccessToken)
	
	// Revoke token
	err = authService.RevokeToken(ctx, claims.TokenID)
	require.NoError(t, err)
	
	// Validate revoked token should fail
	_, err = authService.ValidateToken(ctx, authResp.AccessToken)
	assert.Error(t, err)
}

func testPermissionSystem(t *testing.T, ctx context.Context, authService auth.AuthService) {
	// Test role permissions
	adminPermissions := auth.GetRolePermissions(auth.RoleAdmin)
	assert.Contains(t, adminPermissions, auth.PermissionUserRead)
	assert.Contains(t, adminPermissions, auth.PermissionUserWrite)
	
	userPermissions := auth.GetRolePermissions(auth.RoleUser)
	assert.Contains(t, userPermissions, auth.PermissionDataRead)
	assert.NotContains(t, userPermissions, auth.PermissionUserDelete)
	
	// Test tenant role permissions
	ownerPermissions := auth.GetTenantRolePermissions(auth.TenantRoleOwner)
	assert.Contains(t, ownerPermissions, auth.PermissionTenantAdmin)
	
	memberPermissions := auth.GetTenantRolePermissions(auth.TenantRoleMember)
	assert.Contains(t, memberPermissions, auth.PermissionDataRead)
	assert.NotContains(t, memberPermissions, auth.PermissionTenantAdmin)
}

func testMiddlewareIntegration(t *testing.T, ctx context.Context, authService auth.AuthService) {
	// Create test user and authenticate
	createReq := &auth.CreateUserRequest{
		Username: "middleware_user",
		Email:    "middleware@test.com",
		Password: "MiddlewarePass123!",
		Roles:    []auth.Role{auth.RoleUser},
	}
	
	user, err := authService.CreateUser(ctx, createReq)
	require.NoError(t, err)
	
	authReq := &auth.AuthRequest{
		Type:     auth.AuthTypePassword,
		Username: "middleware_user",
		Password: "MiddlewarePass123!",
	}
	
	authResp, err := authService.Authenticate(ctx, authReq)
	require.NoError(t, err)
	
	// Setup Gin router with middleware
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	middleware := auth.NewAuthMiddleware(authService, nil)
	router.Use(middleware.AuthenticateGin())
	
	// Protected endpoint
	router.GET("/protected", func(c *gin.Context) {
		user := auth.GetUserFromContext(c)
		if user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "no user"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"user_id": user.ID, "username": user.Username})
	})
	
	// Test without token (should fail)
	req := httptest.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	
	// Test with valid token (should succeed)
	req = httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+authResp.AccessToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), user.Username)
	
	// Test permission-based middleware
	adminRouter := gin.New()
	adminRouter.Use(middleware.AuthenticateGin())
	adminRouter.Use(middleware.RequirePermission(auth.PermissionUserAdmin))
	
	adminRouter.GET("/admin", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin access granted"})
	})
	
	// Test user without admin permission (should fail)
	req = httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+authResp.AccessToken)
	w = httptest.NewRecorder()
	adminRouter.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func testMultiTenantAuth(t *testing.T, ctx context.Context, authService auth.AuthService) {
	// Create two tenants
	tenant1Req := &auth.CreateTenantRequest{
		Name:        "tenant1",
		DisplayName: "Tenant One",
		Domain:      "tenant1.com",
	}
	
	tenant1, err := authService.CreateTenant(ctx, tenant1Req)
	require.NoError(t, err)
	
	tenant2Req := &auth.CreateTenantRequest{
		Name:        "tenant2",
		DisplayName: "Tenant Two",
		Domain:      "tenant2.com",
	}
	
	tenant2, err := authService.CreateTenant(ctx, tenant2Req)
	require.NoError(t, err)
	
	// Create user and add to both tenants with different roles
	userReq := &auth.CreateUserRequest{
		Username: "multi_tenant_user",
		Email:    "multi@test.com",
		Password: "MultiPass123!",
		Roles:    []auth.Role{auth.RoleUser},
	}
	
	user, err := authService.CreateUser(ctx, userReq)
	require.NoError(t, err)
	
	// Add user to tenant1 as admin
	err = authService.AddUserToTenant(ctx, user.ID, tenant1.ID, auth.TenantRoleAdmin)
	require.NoError(t, err)
	
	// Add user to tenant2 as member
	err = authService.AddUserToTenant(ctx, user.ID, tenant2.ID, auth.TenantRoleMember)
	require.NoError(t, err)
	
	// Authenticate with tenant1 context
	authReq1 := &auth.AuthRequest{
		Type:     auth.AuthTypePassword,
		Username: "multi_tenant_user",
		Password: "MultiPass123!",
		TenantID: tenant1.ID,
	}
	
	authResp1, err := authService.Authenticate(ctx, authReq1)
	require.NoError(t, err)
	
	claims1, err := authService.ValidateToken(ctx, authResp1.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, tenant1.ID, claims1.TenantID)
	assert.Equal(t, auth.TenantRoleAdmin, claims1.TenantRole)
	
	// Authenticate with tenant2 context
	authReq2 := &auth.AuthRequest{
		Type:     auth.AuthTypePassword,
		Username: "multi_tenant_user",
		Password: "MultiPass123!",
		TenantID: tenant2.ID,
	}
	
	authResp2, err := authService.Authenticate(ctx, authReq2)
	require.NoError(t, err)
	
	claims2, err := authService.ValidateToken(ctx, authResp2.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, tenant2.ID, claims2.TenantID)
	assert.Equal(t, auth.TenantRoleMember, claims2.TenantRole)
	
	// Verify different permissions based on tenant role
	adminPerms := auth.GetTenantRolePermissions(auth.TenantRoleAdmin)
	memberPerms := auth.GetTenantRolePermissions(auth.TenantRoleMember)
	
	assert.Contains(t, adminPerms, auth.PermissionTenantWrite)
	assert.NotContains(t, memberPerms, auth.PermissionTenantWrite)
}

func TestAuthHealthCheck(t *testing.T) {
	ctx := context.Background()
	
	// Initialize auth service
	jwtManager, err := auth.NewJWTManager(nil)
	require.NoError(t, err)
	
	userStore := auth.NewMemoryUserStore()
	tenantStore := auth.NewMemoryTenantStore()
	authService := auth.NewService(jwtManager, userStore, tenantStore, nil)
	
	// Test health check
	err = authService.HealthCheck(ctx)
	assert.NoError(t, err)
}

func TestAuthMetrics(t *testing.T) {
	ctx := context.Background()
	
	// Initialize auth service
	jwtManager, err := auth.NewJWTManager(nil)
	require.NoError(t, err)
	
	userStore := auth.NewMemoryUserStore()
	tenantStore := auth.NewMemoryTenantStore()
	authService := auth.NewService(jwtManager, userStore, tenantStore, nil)
	
	// Create some test data
	createReq := &auth.CreateUserRequest{
		Username: "metrics_user",
		Email:    "metrics@test.com",
		Password: "MetricsPass123!",
		Roles:    []auth.Role{auth.RoleUser},
	}
	
	_, err = authService.CreateUser(ctx, createReq)
	require.NoError(t, err)
	
	// Authenticate to generate metrics
	authReq := &auth.AuthRequest{
		Type:     auth.AuthTypePassword,
		Username: "metrics_user",
		Password: "MetricsPass123!",
	}
	
	_, err = authService.Authenticate(ctx, authReq)
	require.NoError(t, err)
	
	// Get metrics
	metrics, err := authService.GetMetrics(ctx)
	require.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Greater(t, metrics.TotalLogins, int64(0))
}

// Helper function
func stringPtr(s string) *string {
	return &s
}