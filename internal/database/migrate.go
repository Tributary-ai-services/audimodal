package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// MigrationStatus represents the status of a migration
type MigrationStatus struct {
	Version   string    `json:"version"`
	AppliedAt time.Time `json:"applied_at"`
	Applied   bool      `json:"applied"`
}

// Migrator handles database migrations
type Migrator struct {
	db     *sql.DB
	config *Config
}

// NewMigrator creates a new database migrator
func NewMigrator(config *Config) (*Migrator, error) {
	dsn := buildDSN(config)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Migrator{
		db:     db,
		config: config,
	}, nil
}

// Close closes the migrator's database connection
func (m *Migrator) Close() error {
	return m.db.Close()
}

// GetMigrationFiles returns all available migration files
func (m *Migrator) GetMigrationFiles() ([]string, error) {
	var files []string

	err := fs.WalkDir(migrationFiles, "migrations", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && strings.HasSuffix(path, ".sql") {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to read migration files: %w", err)
	}

	// Sort files by version number
	sort.Slice(files, func(i, j int) bool {
		versionI := extractVersionFromFilename(files[i])
		versionJ := extractVersionFromFilename(files[j])

		numI, _ := strconv.Atoi(versionI)
		numJ, _ := strconv.Atoi(versionJ)

		return numI < numJ
	})

	return files, nil
}

// GetAppliedMigrations returns all applied migrations
func (m *Migrator) GetAppliedMigrations(ctx context.Context) ([]MigrationStatus, error) {
	// Ensure schema_migrations table exists
	if err := m.createSchemaMigrationsTable(ctx); err != nil {
		return nil, fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	rows, err := m.db.QueryContext(ctx, "SELECT version, applied_at FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	var migrations []MigrationStatus
	for rows.Next() {
		var migration MigrationStatus
		if err := rows.Scan(&migration.Version, &migration.AppliedAt); err != nil {
			return nil, fmt.Errorf("failed to scan migration row: %w", err)
		}
		migration.Applied = true
		migrations = append(migrations, migration)
	}

	return migrations, nil
}

// GetPendingMigrations returns migrations that haven't been applied yet
func (m *Migrator) GetPendingMigrations(ctx context.Context) ([]string, error) {
	availableFiles, err := m.GetMigrationFiles()
	if err != nil {
		return nil, err
	}

	appliedMigrations, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	// Create a map of applied migrations for quick lookup
	appliedMap := make(map[string]bool)
	for _, migration := range appliedMigrations {
		appliedMap[migration.Version] = true
	}

	var pending []string
	for _, file := range availableFiles {
		version := extractVersionFromFilename(file)
		if !appliedMap[version] {
			pending = append(pending, file)
		}
	}

	return pending, nil
}

// Migrate runs all pending migrations
func (m *Migrator) Migrate(ctx context.Context) error {
	pendingMigrations, err := m.GetPendingMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending migrations: %w", err)
	}

	if len(pendingMigrations) == 0 {
		fmt.Println("No pending migrations to run")
		return nil
	}

	fmt.Printf("Running %d pending migrations...\n", len(pendingMigrations))

	for _, migrationFile := range pendingMigrations {
		if err := m.runMigration(ctx, migrationFile); err != nil {
			return fmt.Errorf("failed to run migration %s: %w", migrationFile, err)
		}
		fmt.Printf("✓ Applied migration: %s\n", migrationFile)
	}

	fmt.Println("All migrations completed successfully")
	return nil
}

// MigrateUp runs a specific number of pending migrations
func (m *Migrator) MigrateUp(ctx context.Context, count int) error {
	pendingMigrations, err := m.GetPendingMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending migrations: %w", err)
	}

	if len(pendingMigrations) == 0 {
		fmt.Println("No pending migrations to run")
		return nil
	}

	if count > len(pendingMigrations) {
		count = len(pendingMigrations)
	}

	fmt.Printf("Running %d migrations...\n", count)

	for i := 0; i < count; i++ {
		migrationFile := pendingMigrations[i]
		if err := m.runMigration(ctx, migrationFile); err != nil {
			return fmt.Errorf("failed to run migration %s: %w", migrationFile, err)
		}
		fmt.Printf("✓ Applied migration: %s\n", migrationFile)
	}

	return nil
}

// GetMigrationStatus returns the status of all migrations
func (m *Migrator) GetMigrationStatus(ctx context.Context) ([]MigrationStatus, error) {
	availableFiles, err := m.GetMigrationFiles()
	if err != nil {
		return nil, err
	}

	appliedMigrations, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	// Create a map of applied migrations for quick lookup
	appliedMap := make(map[string]MigrationStatus)
	for _, migration := range appliedMigrations {
		appliedMap[migration.Version] = migration
	}

	var status []MigrationStatus
	for _, file := range availableFiles {
		version := extractVersionFromFilename(file)
		if applied, exists := appliedMap[version]; exists {
			status = append(status, applied)
		} else {
			status = append(status, MigrationStatus{
				Version: version,
				Applied: false,
			})
		}
	}

	return status, nil
}

// Reset drops all tables and re-runs all migrations (DANGEROUS!)
func (m *Migrator) Reset(ctx context.Context) error {
	fmt.Println("WARNING: This will drop all tables and data!")

	// Drop all tables
	if err := m.dropAllTables(ctx); err != nil {
		return fmt.Errorf("failed to drop tables: %w", err)
	}

	// Run all migrations
	return m.Migrate(ctx)
}

// runMigration executes a single migration file
func (m *Migrator) runMigration(ctx context.Context, migrationFile string) error {
	// Read migration file
	content, err := migrationFiles.ReadFile(migrationFile)
	if err != nil {
		return fmt.Errorf("failed to read migration file: %w", err)
	}

	version := extractVersionFromFilename(migrationFile)

	// Start transaction
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute migration SQL
	if _, err := tx.ExecContext(ctx, string(content)); err != nil {
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	// Record migration as applied (if not already recorded in the migration file)
	_, err = tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, applied_at) VALUES ($1, $2) ON CONFLICT (version) DO NOTHING",
		version, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	// Commit transaction
	return tx.Commit()
}

// createSchemaMigrationsTable creates the schema_migrations table if it doesn't exist
func (m *Migrator) createSchemaMigrationsTable(ctx context.Context) error {
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT NOW()
		)
	`

	_, err := m.db.ExecContext(ctx, createTableSQL)
	return err
}

// dropAllTables drops all tables in the database
func (m *Migrator) dropAllTables(ctx context.Context) error {
	// Get all table names
	rows, err := m.db.QueryContext(ctx, `
		SELECT tablename FROM pg_tables 
		WHERE schemaname = 'public' 
		AND tablename != 'schema_migrations'
	`)
	if err != nil {
		return fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return fmt.Errorf("failed to scan table name: %w", err)
		}
		tables = append(tables, tableName)
	}

	// Drop all tables
	for _, table := range tables {
		_, err := m.db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", table))
		if err != nil {
			return fmt.Errorf("failed to drop table %s: %w", table, err)
		}
	}

	// Also drop the schema_migrations table
	_, err = m.db.ExecContext(ctx, "DROP TABLE IF EXISTS schema_migrations")
	if err != nil {
		return fmt.Errorf("failed to drop schema_migrations table: %w", err)
	}

	return nil
}

// extractVersionFromFilename extracts the version number from a migration filename
func extractVersionFromFilename(filename string) string {
	base := filepath.Base(filename)
	parts := strings.Split(base, "_")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// Rollback rolls back the last applied migration (if supported)
func (m *Migrator) Rollback(ctx context.Context) error {
	// Note: This is a placeholder implementation
	// PostgreSQL doesn't support automatic rollbacks of DDL operations
	// This would require writing down migrations for each up migration
	return fmt.Errorf("rollback is not currently supported - would require down migrations")
}

// ValidateDatabase checks if the database schema is in a valid state
func (m *Migrator) ValidateDatabase(ctx context.Context) error {
	// Check if all required tables exist
	requiredTables := []string{
		"tenants", "data_sources", "processing_sessions",
		"dlp_policies", "files", "chunks", "dlp_violations",
	}

	for _, table := range requiredTables {
		var exists bool
		err := m.db.QueryRowContext(ctx,
			"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = $1)",
			table,
		).Scan(&exists)

		if err != nil {
			return fmt.Errorf("failed to check table %s: %w", table, err)
		}

		if !exists {
			return fmt.Errorf("required table %s does not exist", table)
		}
	}

	return nil
}
