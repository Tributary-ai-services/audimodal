package handlers

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"

	"github.com/jscharber/audimodal/internal/database/models"
)

// MockDB is a mock implementation of the database interface for testing
type MockDB struct {
	mock.Mock
}

// MockTenantService is a mock implementation of tenant service
type MockTenantService struct {
	mock.Mock
}

// DB returns a mock gorm.DB (not used in most tests)
func (m *MockDB) DB() *gorm.DB {
	args := m.Called()
	result := args.Get(0)
	if result == nil {
		return nil
	}
	return result.(*gorm.DB)
}

// Close mocks the Close method
func (m *MockDB) Close() error {
	args := m.Called()
	return args.Error(0)
}

// HealthCheck mocks the HealthCheck method
func (m *MockDB) HealthCheck(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// Migrate mocks the Migrate method
func (m *MockDB) Migrate() error {
	args := m.Called()
	return args.Error(0)
}

// WithContext mocks the WithContext method
func (m *MockDB) WithContext(ctx context.Context) *MockDB {
	args := m.Called(ctx)
	result := args.Get(0)
	if result == nil {
		return nil
	}
	return result.(*MockDB)
}

// Transaction mocks the Transaction method
func (m *MockDB) Transaction(fn func(*gorm.DB) error) error {
	args := m.Called(fn)
	return args.Error(0)
}

// NewTenantService mocks the NewTenantService method
func (m *MockDB) NewTenantService() *MockTenantService {
	args := m.Called()
	result := args.Get(0)
	if result == nil {
		return nil
	}
	return result.(*MockTenantService)
}

// GetTenantRepository mocks getting a tenant repository
func (m *MockTenantService) GetTenantRepository(ctx context.Context, tenantID uuid.UUID) (*MockTenantRepository, error) {
	args := m.Called(ctx, tenantID)
	result := args.Get(0)
	if result == nil {
		return nil, args.Error(1)
	}
	return result.(*MockTenantRepository), args.Error(1)
}

// CreateFile mocks file creation
func (m *MockDB) CreateFile(ctx context.Context, file *models.File) error {
	args := m.Called(ctx, file)
	return args.Error(0)
}

// FindFileByID mocks finding a file by ID
func (m *MockDB) FindFileByID(ctx context.Context, fileID string) (*models.File, error) {
	args := m.Called(ctx, fileID)
	result := args.Get(0)
	if result == nil {
		return nil, args.Error(1)
	}
	return result.(*models.File), args.Error(1)
}

// FindDataSourceByID mocks finding a data source by ID
func (m *MockDB) FindDataSourceByID(ctx context.Context, dataSourceID string) (*models.DataSource, error) {
	args := m.Called(ctx, dataSourceID)
	result := args.Get(0)
	if result == nil {
		return nil, args.Error(1)
	}
	return result.(*models.DataSource), args.Error(1)
}

// ListFiles mocks listing files
func (m *MockDB) ListFiles(ctx context.Context, filters map[string]interface{}, limit, offset int) ([]*models.File, int64, error) {
	args := m.Called(ctx, filters, limit, offset)
	result := args.Get(0)
	if result == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return result.([]*models.File), args.Get(1).(int64), args.Error(2)
}

// UpdateFile mocks updating a file
func (m *MockDB) UpdateFile(ctx context.Context, file *models.File) error {
	args := m.Called(ctx, file)
	return args.Error(0)
}

// DeleteFile mocks deleting a file
func (m *MockDB) DeleteFile(ctx context.Context, fileID string) error {
	args := m.Called(ctx, fileID)
	return args.Error(0)
}

// SearchFiles mocks searching files
func (m *MockDB) SearchFiles(ctx context.Context, query string, filters map[string]interface{}, limit, offset int) ([]*models.File, int64, error) {
	args := m.Called(ctx, query, filters, limit, offset)
	result := args.Get(0)
	if result == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return result.([]*models.File), args.Get(1).(int64), args.Error(2)
}

// GetFileProcessingStatus mocks getting file processing status
func (m *MockDB) GetFileProcessingStatus(ctx context.Context, fileID string) (map[string]interface{}, error) {
	args := m.Called(ctx, fileID)
	result := args.Get(0)
	if result == nil {
		return nil, args.Error(1)
	}
	return result.(map[string]interface{}), args.Error(1)
}

// MockTenantRepository for tenant-specific operations
type MockTenantRepository struct {
	mock.Mock
}

// DB returns a mock gorm.DB
func (m *MockTenantRepository) DB() *gorm.DB {
	args := m.Called()
	result := args.Get(0)
	if result == nil {
		return nil
	}
	return result.(*gorm.DB)
}

// FindByID mocks finding tenant by ID
func (m *MockTenantRepository) FindByID(ctx context.Context, tenantID string) (*models.Tenant, error) {
	args := m.Called(ctx, tenantID)
	result := args.Get(0)
	if result == nil {
		return nil, args.Error(1)
	}
	return result.(*models.Tenant), args.Error(1)
}

// Create mocks creating a tenant
func (m *MockTenantRepository) Create(ctx context.Context, tenant *models.Tenant) error {
	args := m.Called(ctx, tenant)
	return args.Error(0)
}

// Update mocks updating a tenant
func (m *MockTenantRepository) Update(ctx context.Context, tenant *models.Tenant) error {
	args := m.Called(ctx, tenant)
	return args.Error(0)
}

// Delete mocks deleting a tenant
func (m *MockTenantRepository) Delete(ctx context.Context, tenantID string) error {
	args := m.Called(ctx, tenantID)
	return args.Error(0)
}

// List mocks listing tenants
func (m *MockTenantRepository) List(ctx context.Context, limit, offset int) ([]*models.Tenant, int64, error) {
	args := m.Called(ctx, limit, offset)
	result := args.Get(0)
	if result == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return result.([]*models.Tenant), args.Get(1).(int64), args.Error(2)
}
