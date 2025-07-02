-- Performance indexes and optimizations
-- Created: 2025-01-03

-- Create schema_migrations table if it doesn't exist
CREATE TABLE IF NOT EXISTS schema_migrations (
    version VARCHAR(255) PRIMARY KEY,
    applied_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Additional composite indexes for common query patterns

-- Tenant-based queries (most common pattern)
CREATE INDEX CONCURRENTLY idx_data_sources_tenant_status ON data_sources(tenant_id, status) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY idx_files_tenant_status ON files(tenant_id, status) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY idx_chunks_tenant_embedding ON chunks(tenant_id, embedding_status) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY idx_dlp_violations_tenant_severity ON dlp_violations(tenant_id, severity);

-- Processing workflow indexes
CREATE INDEX CONCURRENTLY idx_files_processing_workflow ON files(tenant_id, status, processing_tier) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY idx_chunks_processing_workflow ON chunks(tenant_id, file_id, embedding_status) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY idx_processing_sessions_workflow ON processing_sessions(tenant_id, status, created_at) WHERE deleted_at IS NULL;

-- DLP and compliance indexes
CREATE INDEX CONCURRENTLY idx_files_pii_compliance ON files(tenant_id, pii_detected, sensitivity_level) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY idx_chunks_pii_compliance ON chunks(tenant_id, pii_detected, dlp_scan_status) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY idx_dlp_policies_tenant_enabled ON dlp_policies(tenant_id, enabled, priority) WHERE deleted_at IS NULL;

-- Time-based indexes for analytics and cleanup
CREATE INDEX CONCURRENTLY idx_files_created_at ON files(created_at);
CREATE INDEX CONCURRENTLY idx_chunks_created_at ON chunks(created_at);
CREATE INDEX CONCURRENTLY idx_dlp_violations_created_at ON dlp_violations(created_at);
CREATE INDEX CONCURRENTLY idx_processing_sessions_timing ON processing_sessions(created_at, completed_at);

-- Content search indexes
CREATE INDEX CONCURRENTLY idx_files_content_type_size ON files(content_type, size) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY idx_chunks_content_hash ON chunks(content_hash) WHERE content_hash IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_chunks_type_size ON chunks(chunk_type, size_bytes);

-- Data source sync indexes
CREATE INDEX CONCURRENTLY idx_data_sources_sync ON data_sources(tenant_id, last_sync_at, last_sync_status) WHERE deleted_at IS NULL;

-- File hierarchy and relationships
CREATE INDEX CONCURRENTLY idx_chunks_parent_child ON chunks(parent_chunk_id, file_id) WHERE parent_chunk_id IS NOT NULL;

-- Embedding and vector search preparation
CREATE INDEX CONCURRENTLY idx_chunks_embedding_model ON chunks(embedding_model, embedding_dimension) WHERE embedding_status = 'completed';

-- GIN indexes for JSONB columns (for fast JSON queries)
CREATE INDEX CONCURRENTLY idx_tenants_quotas_gin ON tenants USING GIN (quotas);
CREATE INDEX CONCURRENTLY idx_tenants_compliance_gin ON tenants USING GIN (compliance);
CREATE INDEX CONCURRENTLY idx_data_sources_config_gin ON data_sources USING GIN (config);
CREATE INDEX CONCURRENTLY idx_files_metadata_gin ON files USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_files_classifications_gin ON files USING GIN (classifications);
CREATE INDEX CONCURRENTLY idx_chunks_metadata_gin ON chunks USING GIN (metadata);
CREATE INDEX CONCURRENTLY idx_chunks_quality_gin ON chunks USING GIN (quality);
CREATE INDEX CONCURRENTLY idx_dlp_policies_rules_gin ON dlp_policies USING GIN (content_rules);
CREATE INDEX CONCURRENTLY idx_dlp_policies_conditions_gin ON dlp_policies USING GIN (conditions);

-- Partial indexes for active records only (common filter)
CREATE INDEX CONCURRENTLY idx_tenants_active ON tenants(id, name) WHERE status = 'active' AND deleted_at IS NULL;
CREATE INDEX CONCURRENTLY idx_data_sources_active ON data_sources(id, tenant_id, type) WHERE status = 'active' AND deleted_at IS NULL;
CREATE INDEX CONCURRENTLY idx_dlp_policies_active ON dlp_policies(id, tenant_id, priority) WHERE enabled = true AND status = 'active' AND deleted_at IS NULL;

-- Full-text search indexes for content (using PostgreSQL's built-in text search)
CREATE INDEX CONCURRENTLY idx_chunks_content_fts ON chunks USING GIN (to_tsvector('english', content));
CREATE INDEX CONCURRENTLY idx_files_filename_fts ON files USING GIN (to_tsvector('english', filename));

-- Create statistics for query planner optimization
CREATE STATISTICS IF NOT EXISTS stat_files_tenant_type ON tenant_id, content_type FROM files;
CREATE STATISTICS IF NOT EXISTS stat_chunks_tenant_embedding ON tenant_id, embedding_status FROM chunks;
CREATE STATISTICS IF NOT EXISTS stat_dlp_violations_tenant_severity ON tenant_id, severity FROM dlp_violations;

-- Create covering indexes for frequently accessed columns
CREATE INDEX CONCURRENTLY idx_files_status_covering ON files(tenant_id, status) 
    INCLUDE (id, filename, size, content_type, created_at) WHERE deleted_at IS NULL;

CREATE INDEX CONCURRENTLY idx_chunks_embedding_covering ON chunks(tenant_id, embedding_status) 
    INCLUDE (id, file_id, chunk_type, size_bytes, created_at) WHERE deleted_at IS NULL;

-- Create hash indexes for exact matches (faster than btree for equality)
CREATE INDEX CONCURRENTLY idx_files_checksum_hash ON files USING HASH (checksum) WHERE checksum IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_chunks_content_hash_hash ON chunks USING HASH (content_hash) WHERE content_hash IS NOT NULL;

-- Create expression indexes for common calculated fields
CREATE INDEX CONCURRENTLY idx_files_size_tier ON files((
    CASE 
        WHEN size < 10485760 THEN 'tier1'  -- < 10MB
        WHEN size < 1073741824 THEN 'tier2' -- < 1GB
        ELSE 'tier3'
    END
)) WHERE deleted_at IS NULL;

-- Create partial unique indexes to enforce business rules
CREATE UNIQUE INDEX CONCURRENTLY idx_tenants_name_unique_active ON tenants(name) 
    WHERE status = 'active' AND deleted_at IS NULL;

CREATE UNIQUE INDEX CONCURRENTLY idx_data_sources_name_tenant_unique ON data_sources(tenant_id, name) 
    WHERE deleted_at IS NULL;

-- Optimize vacuum and analyze settings for large tables
ALTER TABLE files SET (
    autovacuum_vacuum_scale_factor = 0.1,
    autovacuum_analyze_scale_factor = 0.05
);

ALTER TABLE chunks SET (
    autovacuum_vacuum_scale_factor = 0.1,
    autovacuum_analyze_scale_factor = 0.05
);

-- Set table storage parameters for better performance
ALTER TABLE chunks SET (
    fillfactor = 90  -- Leave room for updates
);

ALTER TABLE files SET (
    fillfactor = 90
);

-- Create check constraints for data validation
ALTER TABLE tenants ADD CONSTRAINT check_tenant_quotas_valid 
    CHECK (quotas ? 'files_per_hour' AND (quotas->>'files_per_hour')::int >= 0);

ALTER TABLE processing_sessions ADD CONSTRAINT check_session_files_count 
    CHECK (processed_files + failed_files <= total_files);

ALTER TABLE dlp_violations ADD CONSTRAINT check_violation_offsets 
    CHECK (start_offset IS NULL OR end_offset IS NULL OR start_offset <= end_offset);

-- Add foreign key constraints with proper cascading
ALTER TABLE chunks ADD CONSTRAINT fk_chunks_parent 
    FOREIGN KEY (parent_chunk_id) REFERENCES chunks(id) ON DELETE SET NULL;

-- Create materialized view for tenant usage statistics (for analytics)
CREATE MATERIALIZED VIEW tenant_usage_stats AS
SELECT 
    t.id,
    t.name,
    t.display_name,
    COUNT(DISTINCT ds.id) as data_source_count,
    COUNT(DISTINCT f.id) as file_count,
    COUNT(DISTINCT c.id) as chunk_count,
    SUM(f.size) as total_storage_bytes,
    COUNT(DISTINCT ps.id) as session_count,
    COUNT(DISTINCT dv.id) as violation_count,
    MAX(f.created_at) as last_file_added,
    MAX(ps.created_at) as last_session_created
FROM tenants t
LEFT JOIN data_sources ds ON t.id = ds.tenant_id AND ds.deleted_at IS NULL
LEFT JOIN files f ON t.id = f.tenant_id AND f.deleted_at IS NULL
LEFT JOIN chunks c ON t.id = c.tenant_id AND c.deleted_at IS NULL
LEFT JOIN processing_sessions ps ON t.id = ps.tenant_id AND ps.deleted_at IS NULL
LEFT JOIN dlp_violations dv ON t.id = dv.tenant_id
WHERE t.deleted_at IS NULL
GROUP BY t.id, t.name, t.display_name;

-- Create unique index on materialized view
CREATE UNIQUE INDEX idx_tenant_usage_stats_id ON tenant_usage_stats(id);

-- Create function to refresh materialized view
CREATE OR REPLACE FUNCTION refresh_tenant_usage_stats()
RETURNS VOID AS $$
BEGIN
    REFRESH MATERIALIZED VIEW CONCURRENTLY tenant_usage_stats;
END;
$$ LANGUAGE plpgsql;

-- Migration completion
INSERT INTO schema_migrations (version, applied_at) VALUES ('002', NOW())
ON CONFLICT (version) DO NOTHING;