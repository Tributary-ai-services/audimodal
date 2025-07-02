package auth

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryUserStore implements UserStore interface using in-memory storage
type MemoryUserStore struct {
	users map[uuid.UUID]*User
	mu    sync.RWMutex
}

// NewMemoryUserStore creates a new memory-based user store
func NewMemoryUserStore() *MemoryUserStore {
	return &MemoryUserStore{
		users: make(map[uuid.UUID]*User),
	}
}

// CreateUser creates a new user
func (s *MemoryUserStore) CreateUser(ctx context.Context, user *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Check if user already exists
	for _, existingUser := range s.users {
		if existingUser.Email == user.Email {
			return ErrUserAlreadyExists
		}
		if existingUser.Username == user.Username {
			return ErrUserAlreadyExists
		}
	}
	
	// Create copy to avoid mutations
	userCopy := *user
	s.users[user.ID] = &userCopy
	
	return nil
}

// GetUser retrieves a user by ID
func (s *MemoryUserStore) GetUser(ctx context.Context, userID uuid.UUID) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	user, exists := s.users[userID]
	if !exists {
		return nil, ErrUserNotFound
	}
	
	// Return copy to avoid mutations
	userCopy := *user
	return &userCopy, nil
}

// GetUserByEmail retrieves a user by email
func (s *MemoryUserStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	for _, user := range s.users {
		if user.Email == email {
			// Return copy to avoid mutations
			userCopy := *user
			return &userCopy, nil
		}
	}
	
	return nil, ErrUserNotFound
}

// GetUserByUsername retrieves a user by username
func (s *MemoryUserStore) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	for _, user := range s.users {
		if user.Username == username {
			// Return copy to avoid mutations
			userCopy := *user
			return &userCopy, nil
		}
	}
	
	return nil, ErrUserNotFound
}

// UpdateUser updates an existing user
func (s *MemoryUserStore) UpdateUser(ctx context.Context, user *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if _, exists := s.users[user.ID]; !exists {
		return ErrUserNotFound
	}
	
	// Create copy to avoid mutations
	userCopy := *user
	userCopy.UpdatedAt = time.Now()
	s.users[user.ID] = &userCopy
	
	return nil
}

// DeleteUser deletes a user
func (s *MemoryUserStore) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if _, exists := s.users[userID]; !exists {
		return ErrUserNotFound
	}
	
	delete(s.users, userID)
	return nil
}

// ListUsers lists users with optional filtering
func (s *MemoryUserStore) ListUsers(ctx context.Context, filter *UserFilter) ([]*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var users []*User
	
	for _, user := range s.users {
		// Apply filters
		if filter != nil {
			if filter.Status != nil && user.Status != *filter.Status {
				continue
			}
			
			if filter.Role != nil {
				hasRole := false
				for _, role := range user.Roles {
					if role == *filter.Role {
						hasRole = true
						break
					}
				}
				if !hasRole {
					continue
				}
			}
			
			if filter.Search != "" {
				search := strings.ToLower(filter.Search)
				if !strings.Contains(strings.ToLower(user.Username), search) &&
					!strings.Contains(strings.ToLower(user.Email), search) &&
					!strings.Contains(strings.ToLower(user.FirstName), search) &&
					!strings.Contains(strings.ToLower(user.LastName), search) {
					continue
				}
			}
		}
		
		// Create copy to avoid mutations
		userCopy := *user
		users = append(users, &userCopy)
	}
	
	// Apply pagination
	if filter != nil {
		start := filter.Offset
		if start > len(users) {
			start = len(users)
		}
		
		end := start + filter.Limit
		if filter.Limit == 0 || end > len(users) {
			end = len(users)
		}
		
		if start < end {
			users = users[start:end]
		} else {
			users = []*User{}
		}
	}
	
	return users, nil
}

// MemoryTenantStore implements TenantStore interface using in-memory storage
type MemoryTenantStore struct {
	tenants     map[uuid.UUID]*Tenant
	userTenants map[uuid.UUID][]*UserTenant // userID -> UserTenants
	tenantUsers map[uuid.UUID][]*TenantUser // tenantID -> TenantUsers
	mu          sync.RWMutex
}

// NewMemoryTenantStore creates a new memory-based tenant store
func NewMemoryTenantStore() *MemoryTenantStore {
	return &MemoryTenantStore{
		tenants:     make(map[uuid.UUID]*Tenant),
		userTenants: make(map[uuid.UUID][]*UserTenant),
		tenantUsers: make(map[uuid.UUID][]*TenantUser),
	}
}

// CreateTenant creates a new tenant
func (s *MemoryTenantStore) CreateTenant(ctx context.Context, tenant *Tenant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Check if tenant already exists by name or domain
	for _, existingTenant := range s.tenants {
		if existingTenant.Name == tenant.Name {
			return ErrTenantAlreadyExists
		}
		if tenant.Domain != "" && existingTenant.Domain == tenant.Domain {
			return ErrTenantAlreadyExists
		}
	}
	
	// Create copy to avoid mutations
	tenantCopy := *tenant
	s.tenants[tenant.ID] = &tenantCopy
	
	return nil
}

// GetTenant retrieves a tenant by ID
func (s *MemoryTenantStore) GetTenant(ctx context.Context, tenantID uuid.UUID) (*Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	tenant, exists := s.tenants[tenantID]
	if !exists {
		return nil, ErrTenantNotFound
	}
	
	// Return copy to avoid mutations
	tenantCopy := *tenant
	return &tenantCopy, nil
}

// UpdateTenant updates an existing tenant
func (s *MemoryTenantStore) UpdateTenant(ctx context.Context, tenant *Tenant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if _, exists := s.tenants[tenant.ID]; !exists {
		return ErrTenantNotFound
	}
	
	// Create copy to avoid mutations
	tenantCopy := *tenant
	tenantCopy.UpdatedAt = time.Now()
	s.tenants[tenant.ID] = &tenantCopy
	
	return nil
}

// DeleteTenant deletes a tenant
func (s *MemoryTenantStore) DeleteTenant(ctx context.Context, tenantID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if _, exists := s.tenants[tenantID]; !exists {
		return ErrTenantNotFound
	}
	
	delete(s.tenants, tenantID)
	
	// Clean up user-tenant relationships
	delete(s.tenantUsers, tenantID)
	for userID, userTenants := range s.userTenants {
		var filteredTenants []*UserTenant
		for _, ut := range userTenants {
			if ut.TenantID != tenantID {
				filteredTenants = append(filteredTenants, ut)
			}
		}
		s.userTenants[userID] = filteredTenants
	}
	
	return nil
}

// ListTenants lists tenants with optional filtering
func (s *MemoryTenantStore) ListTenants(ctx context.Context, filter *TenantFilter) ([]*Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var tenants []*Tenant
	
	for _, tenant := range s.tenants {
		// Apply filters
		if filter != nil {
			if filter.Status != nil && tenant.Status != *filter.Status {
				continue
			}
			
			if filter.Domain != "" && tenant.Domain != filter.Domain {
				continue
			}
			
			if filter.Search != "" {
				search := strings.ToLower(filter.Search)
				if !strings.Contains(strings.ToLower(tenant.Name), search) &&
					!strings.Contains(strings.ToLower(tenant.DisplayName), search) &&
					!strings.Contains(strings.ToLower(tenant.Domain), search) {
					continue
				}
			}
		}
		
		// Create copy to avoid mutations
		tenantCopy := *tenant
		tenants = append(tenants, &tenantCopy)
	}
	
	// Apply pagination
	if filter != nil {
		start := filter.Offset
		if start > len(tenants) {
			start = len(tenants)
		}
		
		end := start + filter.Limit
		if filter.Limit == 0 || end > len(tenants) {
			end = len(tenants)
		}
		
		if start < end {
			tenants = tenants[start:end]
		} else {
			tenants = []*Tenant{}
		}
	}
	
	return tenants, nil
}

// AddUserToTenant adds a user to a tenant with a specific role
func (s *MemoryTenantStore) AddUserToTenant(ctx context.Context, userID, tenantID uuid.UUID, role TenantRole) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Check if tenant exists
	if _, exists := s.tenants[tenantID]; !exists {
		return ErrTenantNotFound
	}
	
	// Check if relationship already exists
	userTenants := s.userTenants[userID]
	for _, ut := range userTenants {
		if ut.TenantID == tenantID {
			// Update existing relationship
			ut.Role = role
			ut.Status = UserTenantStatusActive
			ut.UpdatedAt = time.Now()
			return nil
		}
	}
	
	// Create new relationship
	now := time.Now()
	userTenant := &UserTenant{
		UserID:    userID,
		TenantID:  tenantID,
		Role:      role,
		Status:    UserTenantStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	
	s.userTenants[userID] = append(s.userTenants[userID], userTenant)
	
	// Add to tenant users (would need user info for TenantUser)
	// This is simplified - in a real implementation, you'd fetch user details
	tenantUser := &TenantUser{
		User:      nil, // Would populate from user store
		Role:      role,
		Status:    UserTenantStatusActive,
		CreatedAt: now,
	}
	
	s.tenantUsers[tenantID] = append(s.tenantUsers[tenantID], tenantUser)
	
	return nil
}

// RemoveUserFromTenant removes a user from a tenant
func (s *MemoryTenantStore) RemoveUserFromTenant(ctx context.Context, userID, tenantID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Remove from user tenants
	userTenants := s.userTenants[userID]
	var filteredUserTenants []*UserTenant
	for _, ut := range userTenants {
		if ut.TenantID != tenantID {
			filteredUserTenants = append(filteredUserTenants, ut)
		}
	}
	s.userTenants[userID] = filteredUserTenants
	
	// Remove from tenant users
	tenantUsers := s.tenantUsers[tenantID]
	var filteredTenantUsers []*TenantUser
	for _, tu := range tenantUsers {
		if tu.User != nil && tu.User.ID != userID {
			filteredTenantUsers = append(filteredTenantUsers, tu)
		}
	}
	s.tenantUsers[tenantID] = filteredTenantUsers
	
	return nil
}

// GetUserTenants retrieves all tenants for a user
func (s *MemoryTenantStore) GetUserTenants(ctx context.Context, userID uuid.UUID) ([]*UserTenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	userTenants := s.userTenants[userID]
	if userTenants == nil {
		return []*UserTenant{}, nil
	}
	
	// Return copies to avoid mutations
	var result []*UserTenant
	for _, ut := range userTenants {
		utCopy := *ut
		result = append(result, &utCopy)
	}
	
	return result, nil
}

// GetTenantUsers retrieves all users for a tenant
func (s *MemoryTenantStore) GetTenantUsers(ctx context.Context, tenantID uuid.UUID) ([]*TenantUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	tenantUsers := s.tenantUsers[tenantID]
	if tenantUsers == nil {
		return []*TenantUser{}, nil
	}
	
	// Return copies to avoid mutations
	var result []*TenantUser
	for _, tu := range tenantUsers {
		tuCopy := *tu
		if tu.User != nil {
			userCopy := *tu.User
			tuCopy.User = &userCopy
		}
		result = append(result, &tuCopy)
	}
	
	return result, nil
}

// SetUserStore sets the user store reference for populating tenant users
func (s *MemoryTenantStore) SetUserStore(userStore UserStore) {
	// This would be used to populate user details in TenantUser
	// Implementation would depend on specific requirements
}

// SeedData creates some initial test data
func (s *MemoryUserStore) SeedData() error {
	// Create a default admin user
	adminUser := &User{
		ID:           uuid.New(),
		Username:     "admin",
		Email:        "admin@example.com",
		PasswordHash: "$2a$12$LCfGI4o5c7KnQlLHYiEjUeJNw9rQ8xKHG7cHlKW8Xr7D8kPqL6Y9O", // "password123"
		FirstName:    "System",
		LastName:     "Administrator",
		Status:       UserStatusActive,
		Roles:        []Role{RoleSuperAdmin},
		Permissions:  GetRolePermissions(RoleSuperAdmin),
		Metadata:     make(map[string]string),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	
	s.users[adminUser.ID] = adminUser
	
	// Create a test user
	testUser := &User{
		ID:           uuid.New(),
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "$2a$12$LCfGI4o5c7KnQlLHYiEjUeJNw9rQ8xKHG7cHlKW8Xr7D8kPqL6Y9O", // "password123"
		FirstName:    "Test",
		LastName:     "User",
		Status:       UserStatusActive,
		Roles:        []Role{RoleUser},
		Permissions:  GetRolePermissions(RoleUser),
		Metadata:     make(map[string]string),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	
	s.users[testUser.ID] = testUser
	
	return nil
}

// SeedData creates some initial test data for tenants
func (s *MemoryTenantStore) SeedData() error {
	// Create a default tenant
	defaultTenant := &Tenant{
		ID:          uuid.New(),
		Name:        "default",
		DisplayName: "Default Organization",
		Domain:      "default.local",
		Status:      TenantStatusActive,
		Settings:    DefaultTenantSettings(),
		Metadata:    make(map[string]string),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	
	s.tenants[defaultTenant.ID] = defaultTenant
	
	return nil
}

// GetDefaultTenant returns the default tenant (for testing)
func (s *MemoryTenantStore) GetDefaultTenant() *Tenant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	for _, tenant := range s.tenants {
		if tenant.Name == "default" {
			tenantCopy := *tenant
			return &tenantCopy
		}
	}
	
	return nil
}

// GetAdminUser returns the admin user (for testing)
func (s *MemoryUserStore) GetAdminUser() *User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	for _, user := range s.users {
		if user.Username == "admin" {
			userCopy := *user
			return &userCopy
		}
	}
	
	return nil
}