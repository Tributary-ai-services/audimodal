package database

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/jscharber/eAIIngest/internal/database/models"
)

// TenantContext represents the tenant context for database operations
type TenantContext struct {
	TenantID   uuid.UUID
	TenantName string
	UserID     string
	RequestID  string
}

// TenantIsolationPlugin implements GORM plugin for automatic tenant isolation
type TenantIsolationPlugin struct {
	TenantContextKey string
}

// Name returns the plugin name
func (p *TenantIsolationPlugin) Name() string {
	return "TenantIsolationPlugin"
}

// Initialize initializes the plugin
func (p *TenantIsolationPlugin) Initialize(db *gorm.DB) error {
	if p.TenantContextKey == "" {
		p.TenantContextKey = "tenant_context"
	}

	// Register callbacks for automatic tenant isolation
	db.Callback().Query().Before("gorm:query").Register("tenant_isolation:before_query", p.beforeQuery)
	db.Callback().Create().Before("gorm:create").Register("tenant_isolation:before_create", p.beforeCreate)
	db.Callback().Update().Before("gorm:update").Register("tenant_isolation:before_update", p.beforeUpdate)
	db.Callback().Delete().Before("gorm:delete").Register("tenant_isolation:before_delete", p.beforeDelete)

	return nil
}

// beforeQuery adds tenant_id filter to all queries
func (p *TenantIsolationPlugin) beforeQuery(db *gorm.DB) {
	if tenantCtx := p.getTenantContext(db); tenantCtx != nil {
		if p.hasTenantIDField(db) {
			db.Where("tenant_id = ?", tenantCtx.TenantID)
		}
	}
}

// beforeCreate sets tenant_id for new records
func (p *TenantIsolationPlugin) beforeCreate(db *gorm.DB) {
	if tenantCtx := p.getTenantContext(db); tenantCtx != nil {
		if p.hasTenantIDField(db) {
			db.Set("tenant_id", tenantCtx.TenantID)

			// Set tenant_id in the actual struct being created
			if db.Statement.Dest != nil {
				p.setTenantIDInStruct(db.Statement.Dest, tenantCtx.TenantID)
			}
		}
	}
}

// beforeUpdate adds tenant_id filter to updates
func (p *TenantIsolationPlugin) beforeUpdate(db *gorm.DB) {
	if tenantCtx := p.getTenantContext(db); tenantCtx != nil {
		if p.hasTenantIDField(db) {
			db.Where("tenant_id = ?", tenantCtx.TenantID)
		}
	}
}

// beforeDelete adds tenant_id filter to deletes
func (p *TenantIsolationPlugin) beforeDelete(db *gorm.DB) {
	if tenantCtx := p.getTenantContext(db); tenantCtx != nil {
		if p.hasTenantIDField(db) {
			db.Where("tenant_id = ?", tenantCtx.TenantID)
		}
	}
}

// getTenantContext extracts tenant context from GORM context
func (p *TenantIsolationPlugin) getTenantContext(db *gorm.DB) *TenantContext {
	if db.Statement.Context == nil {
		return nil
	}

	if tenantCtx, ok := db.Statement.Context.Value(p.TenantContextKey).(*TenantContext); ok {
		return tenantCtx
	}

	return nil
}

// hasTenantIDField checks if the current model has a tenant_id field
func (p *TenantIsolationPlugin) hasTenantIDField(db *gorm.DB) bool {
	if db.Statement.Schema == nil {
		return false
	}

	// Check if the model has a tenant_id field
	_, ok := db.Statement.Schema.FieldsByDBName["tenant_id"]
	return ok
}

// setTenantIDInStruct sets the tenant_id field in a struct using reflection
func (p *TenantIsolationPlugin) setTenantIDInStruct(dest interface{}, tenantID uuid.UUID) {
	switch v := dest.(type) {
	case *models.DataSource:
		v.TenantID = tenantID
	case *models.ProcessingSession:
		v.TenantID = tenantID
	case *models.DLPPolicy:
		v.TenantID = tenantID
	case *models.DLPViolation:
		v.TenantID = tenantID
	case *models.File:
		v.TenantID = tenantID
	case *models.Chunk:
		v.TenantID = tenantID
	case []*models.DataSource:
		for _, item := range v {
			item.TenantID = tenantID
		}
	case []*models.ProcessingSession:
		for _, item := range v {
			item.TenantID = tenantID
		}
	case []*models.DLPPolicy:
		for _, item := range v {
			item.TenantID = tenantID
		}
	case []*models.DLPViolation:
		for _, item := range v {
			item.TenantID = tenantID
		}
	case []*models.File:
		for _, item := range v {
			item.TenantID = tenantID
		}
	case []*models.Chunk:
		for _, item := range v {
			item.TenantID = tenantID
		}
	}
}

// TenantDatabase provides tenant-isolated database operations
type TenantDatabase struct {
	conn   *Connection
	plugin *TenantIsolationPlugin
}

// NewTenantDatabase creates a new tenant-isolated database instance
func NewTenantDatabase(conn *Connection) *TenantDatabase {
	plugin := &TenantIsolationPlugin{
		TenantContextKey: "tenant_context",
	}

	// Install the plugin
	conn.DB().Use(plugin)

	return &TenantDatabase{
		conn:   conn,
		plugin: plugin,
	}
}

// WithTenantContext returns a database instance with tenant context
func (td *TenantDatabase) WithTenantContext(ctx context.Context, tenantCtx *TenantContext) *gorm.DB {
	newCtx := context.WithValue(ctx, td.plugin.TenantContextKey, tenantCtx)
	return td.conn.DB().WithContext(newCtx)
}

// WithTenant returns a database instance scoped to a specific tenant
func (td *TenantDatabase) WithTenant(ctx context.Context, tenantID uuid.UUID) *gorm.DB {
	tenantCtx := &TenantContext{
		TenantID: tenantID,
	}
	return td.WithTenantContext(ctx, tenantCtx)
}

// ValidateTenantAccess validates that a record belongs to the specified tenant
func (td *TenantDatabase) ValidateTenantAccess(ctx context.Context, tenantID uuid.UUID, tableName string, recordID uuid.UUID) error {
	var count int64

	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE id = ? AND tenant_id = ?", tableName)
	err := td.conn.DB().WithContext(ctx).Raw(query, recordID, tenantID).Scan(&count).Error

	if err != nil {
		return fmt.Errorf("failed to validate tenant access: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("record not found or access denied for tenant %s", tenantID)
	}

	return nil
}

// CreateTenantRepository creates a new tenant-scoped repository
func (td *TenantDatabase) CreateTenantRepository(ctx context.Context, tenantID uuid.UUID, tenantName string) *TenantRepository {
	tenantCtx := &TenantContext{
		TenantID:   tenantID,
		TenantName: tenantName,
	}

	db := td.WithTenantContext(ctx, tenantCtx)

	return &TenantRepository{
		db:       db,
		tenantID: tenantID.String(),
	}
}

// GetTenantContext returns the tenant context from the database context
func (tr *TenantRepository) GetTenantContext() *TenantContext {
	ctx := tr.db.Statement.Context
	if ctx == nil {
		return nil
	}
	tenantCtx, ok := ctx.Value("tenant").(*TenantContext)
	if !ok {
		return nil
	}
	return tenantCtx
}

// ValidateAndCreate creates a record with tenant validation
func (tr *TenantRepository) ValidateAndCreate(record interface{}) error {
	// Ensure tenant_id is set correctly
	if err := tr.validateTenantID(record); err != nil {
		return fmt.Errorf("tenant validation failed: %w", err)
	}

	return tr.db.Create(record).Error
}

// ValidateAndUpdate updates a record with tenant validation
func (tr *TenantRepository) ValidateAndUpdate(record interface{}) error {
	// Ensure tenant_id is set correctly
	if err := tr.validateTenantID(record); err != nil {
		return fmt.Errorf("tenant validation failed: %w", err)
	}

	return tr.db.Save(record).Error
}

// validateTenantID ensures the record has the correct tenant_id
func (tr *TenantRepository) validateTenantID(record interface{}) error {
	tenantIDUUID, err := uuid.Parse(tr.tenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant ID: %w", err)
	}

	switch v := record.(type) {
	case *models.DataSource:
		if v.TenantID != tenantIDUUID {
			return fmt.Errorf("tenant ID mismatch for DataSource")
		}
	case *models.ProcessingSession:
		if v.TenantID != tenantIDUUID {
			return fmt.Errorf("tenant ID mismatch for ProcessingSession")
		}
	case *models.DLPPolicy:
		if v.TenantID != tenantIDUUID {
			return fmt.Errorf("tenant ID mismatch for DLPPolicy")
		}
	case *models.DLPViolation:
		if v.TenantID != tenantIDUUID {
			return fmt.Errorf("tenant ID mismatch for DLPViolation")
		}
	case *models.File:
		if v.TenantID != tenantIDUUID {
			return fmt.Errorf("tenant ID mismatch for File")
		}
	case *models.Chunk:
		if v.TenantID != tenantIDUUID {
			return fmt.Errorf("tenant ID mismatch for Chunk")
		}
	}

	return nil
}

// CreateWithAssociations creates a record with its associations
func (tr *TenantRepository) CreateWithAssociations(record interface{}) error {
	return tr.db.Session(&gorm.Session{FullSaveAssociations: true}).Create(record).Error
}

// BulkCreate creates multiple records efficiently
func (tr *TenantRepository) BulkCreate(records interface{}, batchSize int) error {
	return tr.db.CreateInBatches(records, batchSize).Error
}

// UpsertOnConflict performs an upsert operation
func (tr *TenantRepository) UpsertOnConflict(record interface{}, conflictColumns []string, updateColumns []string) error {
	onConflict := clause.OnConflict{
		Columns:   make([]clause.Column, len(conflictColumns)),
		DoUpdates: clause.AssignmentColumns(updateColumns),
	}

	for i, col := range conflictColumns {
		onConflict.Columns[i] = clause.Column{Name: col}
	}

	return tr.db.Clauses(onConflict).Create(record).Error
}

// GetTenantStats returns statistics for the current tenant
func (tr *TenantRepository) GetTenantStats(ctx context.Context) (map[string]interface{}, error) {
	var stats map[string]interface{}

	// Use the materialized view if available
	err := tr.db.WithContext(ctx).Raw(`
		SELECT 
			file_count,
			chunk_count,
			total_storage_bytes,
			session_count,
			violation_count,
			last_file_added,
			last_session_created
		FROM tenant_usage_stats 
		WHERE id = ?
	`, tr.GetTenantContext().TenantID).Scan(&stats).Error

	if err != nil {
		// Fallback to real-time calculation
		stats = make(map[string]interface{})

		var fileCount, chunkCount, sessionCount, violationCount int64
		var totalStorageBytes int64

		tr.db.Model(&models.File{}).Count(&fileCount)
		tr.db.Model(&models.Chunk{}).Count(&chunkCount)
		tr.db.Model(&models.ProcessingSession{}).Count(&sessionCount)
		tr.db.Model(&models.DLPViolation{}).Count(&violationCount)
		tr.db.Model(&models.File{}).Select("COALESCE(SUM(size), 0)").Scan(&totalStorageBytes)

		stats["file_count"] = fileCount
		stats["chunk_count"] = chunkCount
		stats["session_count"] = sessionCount
		stats["violation_count"] = violationCount
		stats["total_storage_bytes"] = totalStorageBytes
	}

	return stats, nil
}

// EnforceQuotas checks if the tenant is within their quotas
func (tr *TenantRepository) EnforceQuotas(ctx context.Context, operation string, additionalUsage map[string]int64) error {
	// Get tenant information
	var tenant models.Tenant
	err := tr.db.WithContext(ctx).Where("id = ?", tr.GetTenantContext().TenantID).First(&tenant).Error
	if err != nil {
		return fmt.Errorf("failed to get tenant: %w", err)
	}

	// Get current usage
	stats, err := tr.GetTenantStats(ctx)
	if err != nil {
		return fmt.Errorf("failed to get tenant stats: %w", err)
	}

	// Check file count quota
	if fileCount, ok := stats["file_count"].(int64); ok {
		if additionalFiles, exists := additionalUsage["files"]; exists {
			// This would need proper quota checking logic based on tenant quotas
			_ = fileCount + additionalFiles
			// TODO: Implement actual quota validation
		}
	}

	// Check storage quota
	if storageBytes, ok := stats["total_storage_bytes"].(int64); ok {
		if additionalStorage, exists := additionalUsage["storage_bytes"]; exists {
			totalStorage := storageBytes + additionalStorage
			quotaGB := tenant.GetQuotaLimit("storage_gb")
			if quotaGB > 0 && totalStorage > quotaGB*1024*1024*1024 {
				return fmt.Errorf("storage quota exceeded: %d bytes used, %d GB limit", totalStorage, quotaGB)
			}
		}
	}

	return nil
}

// CleanupTenantData removes old data for the tenant based on retention policies
func (tr *TenantRepository) CleanupTenantData(ctx context.Context, retentionDays int) error {
	cutoffDate := fmt.Sprintf("NOW() - INTERVAL '%d days'", retentionDays)

	// Clean up old DLP violations
	err := tr.db.WithContext(ctx).Where("created_at < ?", cutoffDate).Delete(&models.DLPViolation{}).Error
	if err != nil {
		return fmt.Errorf("failed to cleanup DLP violations: %w", err)
	}

	// Clean up old processing sessions
	err = tr.db.WithContext(ctx).Where("created_at < ? AND status IN ?", cutoffDate,
		[]string{models.SessionStatusCompleted, models.SessionStatusFailed}).Delete(&models.ProcessingSession{}).Error
	if err != nil {
		return fmt.Errorf("failed to cleanup processing sessions: %w", err)
	}

	return nil
}

// ExportTenantData exports all tenant data for compliance purposes
func (tr *TenantRepository) ExportTenantData(ctx context.Context) (map[string]interface{}, error) {
	export := make(map[string]interface{})

	// Export tenant information
	var tenant models.Tenant
	err := tr.db.WithContext(ctx).Where("id = ?", tr.GetTenantContext().TenantID).First(&tenant).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	export["tenant"] = tenant

	// Export data sources
	var dataSources []models.DataSource
	tr.db.Find(&dataSources)
	export["data_sources"] = dataSources

	// Export files
	var files []models.File
	tr.db.Find(&files)
	export["files"] = files

	// Export processing sessions
	var sessions []models.ProcessingSession
	tr.db.Find(&sessions)
	export["processing_sessions"] = sessions

	// Export DLP policies
	var policies []models.DLPPolicy
	tr.db.Find(&policies)
	export["dlp_policies"] = policies

	return export, nil
}

// DeleteTenantData permanently deletes all tenant data (GDPR compliance)
func (tr *TenantRepository) DeleteTenantData(ctx context.Context) error {
	// Delete in order to respect foreign key constraints
	tables := []interface{}{
		&models.Chunk{},
		&models.DLPViolation{},
		&models.File{},
		&models.ProcessingSession{},
		&models.DLPPolicy{},
		&models.DataSource{},
	}

	for _, table := range tables {
		if err := tr.db.WithContext(ctx).Unscoped().Delete(table, "tenant_id = ?", tr.GetTenantContext().TenantID).Error; err != nil {
			return fmt.Errorf("failed to delete tenant data from %T: %w", table, err)
		}
	}

	return nil
}
