package database

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/jscharber/eAIIngest/internal/database/models"
)

// Config represents database configuration
type Config struct {
	Host     string `yaml:"host" env:"DB_HOST" default:"localhost"`
	Port     int    `yaml:"port" env:"DB_PORT" default:"5432"`
	Username string `yaml:"username" env:"DB_USERNAME" default:"postgres"`
	Password string `yaml:"password" env:"DB_PASSWORD" default:""`
	Database string `yaml:"database" env:"DB_DATABASE" default:"audimodal"`
	SSLMode  string `yaml:"ssl_mode" env:"DB_SSL_MODE" default:"disable"`

	// Connection pool settings
	MaxOpenConns    int           `yaml:"max_open_conns" env:"DB_MAX_OPEN_CONNS" default:"25"`
	MaxIdleConns    int           `yaml:"max_idle_conns" env:"DB_MAX_IDLE_CONNS" default:"5"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" env:"DB_CONN_MAX_LIFETIME" default:"1h"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time" env:"DB_CONN_MAX_IDLE_TIME" default:"30m"`

	// Performance settings
	LogLevel      string        `yaml:"log_level" env:"DB_LOG_LEVEL" default:"warn"`
	SlowThreshold time.Duration `yaml:"slow_threshold" env:"DB_SLOW_THRESHOLD" default:"200ms"`

	// Migration settings
	AutoMigrate bool `yaml:"auto_migrate" env:"DB_AUTO_MIGRATE" default:"false"`
}

// Connection represents a database connection with tenant isolation
type Connection struct {
	db     *gorm.DB
	config *Config
}

// NewConnection creates a new database connection
func NewConnection(config *Config) (*Connection, error) {
	dsn := buildDSN(config)

	// Configure GORM
	gormConfig := &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: false, // Use plural table names
		},
		Logger: getLogger(config.LogLevel, config.SlowThreshold),
	}

	// Open database connection
	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying SQL DB for connection pool configuration
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(config.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	conn := &Connection{
		db:     db,
		config: config,
	}

	// Auto-migrate if enabled
	if config.AutoMigrate {
		if err := conn.AutoMigrate(); err != nil {
			return nil, fmt.Errorf("auto-migration failed: %w", err)
		}
	}

	return conn, nil
}

// DB returns the underlying GORM database instance
func (c *Connection) DB() *gorm.DB {
	return c.db
}

// Close closes the database connection
func (c *Connection) Close() error {
	sqlDB, err := c.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}
	return sqlDB.Close()
}

// Ping tests the database connection
func (c *Connection) Ping(ctx context.Context) error {
	sqlDB, err := c.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}
	return sqlDB.PingContext(ctx)
}

// AutoMigrate runs automatic migrations for all models
func (c *Connection) AutoMigrate() error {
	return c.db.AutoMigrate(
		&models.Tenant{},
		&models.DataSource{},
		&models.ProcessingSession{},
		&models.DLPPolicy{},
		&models.DLPViolation{},
		&models.File{},
		&models.Chunk{},
	)
}

// WithTenant returns a database instance scoped to a specific tenant
func (c *Connection) WithTenant(tenantID string) *gorm.DB {
	return c.db.Where("tenant_id = ?", tenantID)
}

// Transaction executes a function within a database transaction
func (c *Connection) Transaction(fn func(*gorm.DB) error) error {
	return c.db.Transaction(fn)
}

// TransactionWithTenant executes a function within a tenant-scoped transaction
func (c *Connection) TransactionWithTenant(tenantID string, fn func(*gorm.DB) error) error {
	return c.db.Transaction(func(tx *gorm.DB) error {
		return fn(tx.Where("tenant_id = ?", tenantID))
	})
}

// GetStats returns database connection statistics
func (c *Connection) GetStats() (map[string]interface{}, error) {
	sqlDB, err := c.db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	stats := sqlDB.Stats()

	return map[string]interface{}{
		"max_open_connections": stats.MaxOpenConnections,
		"open_connections":     stats.OpenConnections,
		"in_use":               stats.InUse,
		"idle":                 stats.Idle,
		"wait_count":           stats.WaitCount,
		"wait_duration":        stats.WaitDuration,
		"max_idle_closed":      stats.MaxIdleClosed,
		"max_idle_time_closed": stats.MaxIdleTimeClosed,
		"max_lifetime_closed":  stats.MaxLifetimeClosed,
	}, nil
}

// buildDSN builds the PostgreSQL Data Source Name
func buildDSN(config *Config) string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host,
		config.Port,
		config.Username,
		config.Password,
		config.Database,
		config.SSLMode,
	)
}

// getLogger configures the GORM logger based on level and slow threshold
func getLogger(level string, slowThreshold time.Duration) logger.Interface {
	var logLevel logger.LogLevel

	switch level {
	case "silent":
		logLevel = logger.Silent
	case "error":
		logLevel = logger.Error
	case "warn", "warning":
		logLevel = logger.Warn
	case "info":
		logLevel = logger.Info
	default:
		logLevel = logger.Warn
	}

	return logger.New(
		nil, // Use default writer (stdout)
		logger.Config{
			SlowThreshold:             slowThreshold,
			LogLevel:                  logLevel,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false, // Disable color for production logs
		},
	)
}

// HealthCheck performs a comprehensive health check of the database
func (c *Connection) HealthCheck(ctx context.Context) error {
	// Test basic connectivity
	if err := c.Ping(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	// Test a simple query
	var count int64
	if err := c.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM tenants").Scan(&count).Error; err != nil {
		return fmt.Errorf("query test failed: %w", err)
	}

	// Check connection pool health
	sqlDB, err := c.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}

	stats := sqlDB.Stats()
	if stats.OpenConnections == 0 {
		return fmt.Errorf("no open database connections")
	}

	return nil
}

// TenantRepository provides tenant-scoped database operations
type TenantRepository struct {
	db       *gorm.DB
	tenantID string
}

// NewTenantRepository creates a new tenant-scoped repository
func (c *Connection) NewTenantRepository(tenantID string) *TenantRepository {
	return &TenantRepository{
		db:       c.WithTenant(tenantID),
		tenantID: tenantID,
	}
}

// DB returns the tenant-scoped database instance
func (tr *TenantRepository) DB() *gorm.DB {
	return tr.db
}

// TenantID returns the tenant ID for this repository
func (tr *TenantRepository) TenantID() string {
	return tr.tenantID
}

// Transaction executes a function within a tenant-scoped transaction
func (tr *TenantRepository) Transaction(fn func(*gorm.DB) error) error {
	return tr.db.Transaction(fn)
}

// ValidateTenantAccess ensures that a record belongs to the current tenant
func (tr *TenantRepository) ValidateTenantAccess(record interface{}) error {
	// This is a placeholder for tenant validation logic
	// In a real implementation, you would check if the record's tenant_id
	// matches the repository's tenant_id
	return nil
}

// Cleanup performs database cleanup operations
func (c *Connection) Cleanup(ctx context.Context) error {
	// Run VACUUM ANALYZE on large tables periodically
	tables := []string{"files", "chunks", "dlp_violations"}

	for _, table := range tables {
		if err := c.db.WithContext(ctx).Exec(fmt.Sprintf("VACUUM ANALYZE %s", table)).Error; err != nil {
			return fmt.Errorf("failed to vacuum table %s: %w", table, err)
		}
	}

	// Refresh materialized views
	if err := c.db.WithContext(ctx).Exec("SELECT refresh_tenant_usage_stats()").Error; err != nil {
		return fmt.Errorf("failed to refresh materialized views: %w", err)
	}

	return nil
}
