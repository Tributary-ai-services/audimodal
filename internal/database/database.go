// Package database provides database connectivity, models, and tenant isolation
// for the eAIIngest platform. It includes comprehensive migration support,
// tenant-scoped operations, and compliance features.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/jscharber/eAIIngest/internal/database/models"
)

// Database represents the main database interface for the application
type Database struct {
	conn      *Connection
	migrator  *Migrator
	tenant    *TenantDatabase
	config    *Config
}

// New creates a new database instance with all components
func New(config *Config) (*Database, error) {
	// Create database connection
	conn, err := NewConnection(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create database connection: %w", err)
	}

	// Create migrator
	migrator, err := NewMigrator(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrator: %w", err)
	}

	// Create tenant database
	tenantDB := NewTenantDatabase(conn)

	return &Database{
		conn:     conn,
		migrator: migrator,
		tenant:   tenantDB,
		config:   config,
	}, nil
}

// Connect establishes database connection and runs initial setup
func (db *Database) Connect(ctx context.Context) error {
	// Test connection
	if err := db.conn.Ping(ctx); err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}

	// Run migrations if auto-migrate is enabled
	if db.config.AutoMigrate {
		if err := db.migrator.Migrate(ctx); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}

// Close closes all database connections
func (db *Database) Close() error {
	if err := db.conn.Close(); err != nil {
		return fmt.Errorf("failed to close connection: %w", err)
	}

	if err := db.migrator.Close(); err != nil {
		return fmt.Errorf("failed to close migrator: %w", err)
	}

	return nil
}

// DB returns the underlying GORM database instance
func (db *Database) DB() *gorm.DB {
	return db.conn.DB()
}

// Connection returns the database connection
func (db *Database) Connection() *Connection {
	return db.conn
}

// Migrator returns the database migrator
func (db *Database) Migrator() *Migrator {
	return db.migrator
}

// Tenant returns the tenant database
func (db *Database) Tenant() *TenantDatabase {
	return db.tenant
}

// HealthCheck performs a comprehensive health check
func (db *Database) HealthCheck(ctx context.Context) error {
	return db.conn.HealthCheck(ctx)
}

// GetStats returns database statistics
func (db *Database) GetStats() (map[string]interface{}, error) {
	return db.conn.GetStats()
}

// TenantService provides high-level tenant operations
type TenantService struct {
	db *Database
}

// NewTenantService creates a new tenant service
func (db *Database) NewTenantService() *TenantService {
	return &TenantService{db: db}
}

// CreateTenant creates a new tenant with default settings
func (ts *TenantService) CreateTenant(ctx context.Context, tenant *models.Tenant) error {
	// Validate tenant data
	if errors := tenant.Validate(); errors.HasErrors() {
		return fmt.Errorf("tenant validation failed: %w", errors)
	}

	// Set default values
	if tenant.Status == "" {
		tenant.Status = models.TenantStatusActive
	}

	// Create tenant
	if err := ts.db.DB().WithContext(ctx).Create(tenant).Error; err != nil {
		return fmt.Errorf("failed to create tenant: %w", err)
	}

	return nil
}

// GetTenant retrieves a tenant by ID
func (ts *TenantService) GetTenant(ctx context.Context, tenantID uuid.UUID) (*models.Tenant, error) {
	var tenant models.Tenant
	err := ts.db.DB().WithContext(ctx).Where("id = ? AND deleted_at IS NULL", tenantID).First(&tenant).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	return &tenant, nil
}

// GetTenantByName retrieves a tenant by name
func (ts *TenantService) GetTenantByName(ctx context.Context, name string) (*models.Tenant, error) {
	var tenant models.Tenant
	err := ts.db.DB().WithContext(ctx).Where("name = ? AND deleted_at IS NULL", name).First(&tenant).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant by name: %w", err)
	}
	return &tenant, nil
}

// UpdateTenant updates a tenant
func (ts *TenantService) UpdateTenant(ctx context.Context, tenant *models.Tenant) error {
	if errors := tenant.Validate(); errors.HasErrors() {
		return fmt.Errorf("tenant validation failed: %w", errors)
	}

	if err := ts.db.DB().WithContext(ctx).Save(tenant).Error; err != nil {
		return fmt.Errorf("failed to update tenant: %w", err)
	}

	return nil
}

// DeleteTenant soft deletes a tenant
func (ts *TenantService) DeleteTenant(ctx context.Context, tenantID uuid.UUID) error {
	if err := ts.db.DB().WithContext(ctx).Delete(&models.Tenant{}, tenantID).Error; err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}

	return nil
}

// ListTenants lists all active tenants
func (ts *TenantService) ListTenants(ctx context.Context, limit, offset int) ([]models.Tenant, error) {
	var tenants []models.Tenant
	err := ts.db.DB().WithContext(ctx).
		Where("deleted_at IS NULL").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&tenants).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}

	return tenants, nil
}

// GetTenantRepository creates a tenant-scoped repository
func (ts *TenantService) GetTenantRepository(ctx context.Context, tenantID uuid.UUID) (*TenantRepository, error) {
	// Get tenant information
	tenant, err := ts.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("tenant not found: %w", err)
	}

	if !tenant.IsActive() {
		return nil, fmt.Errorf("tenant is not active")
	}

	return ts.db.tenant.CreateTenantRepository(ctx, tenantID, tenant.Name), nil
}

// Repository provides generic repository operations
type Repository[T any] struct {
	db       *gorm.DB
	tenantID *uuid.UUID
}

// NewRepository creates a new generic repository
func NewRepository[T any](db *gorm.DB, tenantID *uuid.UUID) *Repository[T] {
	return &Repository[T]{
		db:       db,
		tenantID: tenantID,
	}
}

// Create creates a new record
func (r *Repository[T]) Create(ctx context.Context, record *T) error {
	return r.db.WithContext(ctx).Create(record).Error
}

// GetByID retrieves a record by ID
func (r *Repository[T]) GetByID(ctx context.Context, id uuid.UUID) (*T, error) {
	var record T
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// Update updates a record
func (r *Repository[T]) Update(ctx context.Context, record *T) error {
	return r.db.WithContext(ctx).Save(record).Error
}

// Delete soft deletes a record
func (r *Repository[T]) Delete(ctx context.Context, id uuid.UUID) error {
	var record T
	return r.db.WithContext(ctx).Delete(&record, id).Error
}

// List lists records with pagination
func (r *Repository[T]) List(ctx context.Context, limit, offset int) ([]T, error) {
	var records []T
	err := r.db.WithContext(ctx).
		Limit(limit).
		Offset(offset).
		Find(&records).Error
	return records, err
}

// Count counts total records
func (r *Repository[T]) Count(ctx context.Context) (int64, error) {
	var count int64
	var record T
	err := r.db.WithContext(ctx).Model(&record).Count(&count).Error
	return count, err
}

// FindWhere finds records matching conditions
func (r *Repository[T]) FindWhere(ctx context.Context, conditions map[string]interface{}) ([]T, error) {
	var records []T
	query := r.db.WithContext(ctx)
	
	for key, value := range conditions {
		query = query.Where(fmt.Sprintf("%s = ?", key), value)
	}
	
	err := query.Find(&records).Error
	return records, err
}

// Utility functions for common database operations

// GetDefaultConfig returns a default database configuration
func GetDefaultConfig() *Config {
	return &Config{
		Host:            "localhost",
		Port:            5432,
		Username:        "postgres",
		Password:        "",
		Database:        "eaiingest",
		SSLMode:         "disable",
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 30 * time.Minute,
		LogLevel:        "warn",
		SlowThreshold:   200 * time.Millisecond,
		AutoMigrate:     false,
	}
}

// ParseConnectionString parses a database connection string
func ParseConnectionString(connectionString string) (*Config, error) {
	// This is a placeholder for parsing connection strings
	// In a real implementation, you would parse the connection string
	// and populate the Config struct accordingly
	return GetDefaultConfig(), nil
}

// WaitForDatabase waits for the database to become available
func WaitForDatabase(ctx context.Context, config *Config, maxRetries int, retryInterval time.Duration) error {
	var lastErr error
	
	for i := 0; i < maxRetries; i++ {
		db, err := New(config)
		if err != nil {
			lastErr = err
			time.Sleep(retryInterval)
			continue
		}

		if err := db.Connect(ctx); err != nil {
			lastErr = err
			db.Close()
			time.Sleep(retryInterval)
			continue
		}

		db.Close()
		return nil
	}

	return fmt.Errorf("database not available after %d retries: %w", maxRetries, lastErr)
}

// BatchProcessor provides utilities for batch processing
type BatchProcessor struct {
	db        *gorm.DB
	batchSize int
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(db *gorm.DB, batchSize int) *BatchProcessor {
	if batchSize <= 0 {
		batchSize = 100
	}
	
	return &BatchProcessor{
		db:        db,
		batchSize: batchSize,
	}
}

// ProcessInBatches processes records in batches
func (bp *BatchProcessor) ProcessInBatches(ctx context.Context, tableName string, processor func([]map[string]interface{}) error) error {
	offset := 0
	
	for {
		var batch []map[string]interface{}
		
		err := bp.db.WithContext(ctx).
			Table(tableName).
			Limit(bp.batchSize).
			Offset(offset).
			Find(&batch).Error
			
		if err != nil {
			return fmt.Errorf("failed to fetch batch: %w", err)
		}
		
		if len(batch) == 0 {
			break
		}
		
		if err := processor(batch); err != nil {
			return fmt.Errorf("batch processing failed: %w", err)
		}
		
		offset += len(batch)
		
		if len(batch) < bp.batchSize {
			break
		}
	}
	
	return nil
}

// Add validation for all models
func init() {
	// Register model validators
	// This could be extended to include custom validation rules
}