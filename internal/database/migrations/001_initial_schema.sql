-- Initial schema migration for eAIIngest
-- Created: 2025-01-03

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Create tenants table
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) UNIQUE NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    billing_plan VARCHAR(50) NOT NULL,
    billing_email VARCHAR(255) NOT NULL,
    quotas JSONB NOT NULL DEFAULT '{}',
    compliance JSONB NOT NULL DEFAULT '{}',
    contact_info JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP
);

-- Create indexes for tenants
CREATE INDEX idx_tenants_name ON tenants(name);
CREATE INDEX idx_tenants_status ON tenants(status);
CREATE INDEX idx_tenants_deleted_at ON tenants(deleted_at);

-- Create data_sources table
CREATE TABLE data_sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    config JSONB NOT NULL DEFAULT '{}',
    credentials_ref JSONB NOT NULL DEFAULT '{}',
    sync_settings JSONB NOT NULL DEFAULT '{}',
    processing_settings JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    last_sync_at TIMESTAMP,
    last_sync_status VARCHAR(50) DEFAULT 'pending',
    last_sync_error TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP
);

-- Create indexes for data_sources
CREATE INDEX idx_data_sources_tenant_id ON data_sources(tenant_id);
CREATE INDEX idx_data_sources_name ON data_sources(name);
CREATE INDEX idx_data_sources_type ON data_sources(type);
CREATE INDEX idx_data_sources_status ON data_sources(status);
CREATE INDEX idx_data_sources_deleted_at ON data_sources(deleted_at);

-- Create processing_sessions table
CREATE TABLE processing_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    files JSONB NOT NULL DEFAULT '[]',
    options JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    progress DECIMAL(5,2) DEFAULT 0,
    total_files INTEGER DEFAULT 0,
    processed_files INTEGER DEFAULT 0,
    failed_files INTEGER DEFAULT 0,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP,
    last_error TEXT,
    error_count INTEGER DEFAULT 0,
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    processed_bytes BIGINT DEFAULT 0,
    total_bytes BIGINT DEFAULT 0,
    chunks_created BIGINT DEFAULT 0
);

-- Create indexes for processing_sessions
CREATE INDEX idx_processing_sessions_tenant_id ON processing_sessions(tenant_id);
CREATE INDEX idx_processing_sessions_name ON processing_sessions(name);
CREATE INDEX idx_processing_sessions_status ON processing_sessions(status);
CREATE INDEX idx_processing_sessions_deleted_at ON processing_sessions(deleted_at);

-- Create dlp_policies table
CREATE TABLE dlp_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT true,
    priority INTEGER NOT NULL DEFAULT 50,
    content_rules JSONB NOT NULL DEFAULT '[]',
    actions JSONB NOT NULL DEFAULT '[]',
    conditions JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP,
    triggered_count BIGINT DEFAULT 0,
    last_triggered TIMESTAMP
);

-- Create indexes for dlp_policies
CREATE INDEX idx_dlp_policies_tenant_id ON dlp_policies(tenant_id);
CREATE INDEX idx_dlp_policies_name ON dlp_policies(name);
CREATE INDEX idx_dlp_policies_enabled ON dlp_policies(enabled);
CREATE INDEX idx_dlp_policies_status ON dlp_policies(status);
CREATE INDEX idx_dlp_policies_priority ON dlp_policies(priority);
CREATE INDEX idx_dlp_policies_deleted_at ON dlp_policies(deleted_at);

-- Create files table
CREATE TABLE files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    data_source_id UUID REFERENCES data_sources(id) ON DELETE SET NULL,
    processing_session_id UUID REFERENCES processing_sessions(id) ON DELETE SET NULL,
    url TEXT NOT NULL,
    path TEXT NOT NULL,
    filename VARCHAR(255) NOT NULL,
    extension VARCHAR(50),
    content_type VARCHAR(255) NOT NULL,
    size BIGINT NOT NULL,
    checksum VARCHAR(255),
    checksum_type VARCHAR(50) DEFAULT 'md5',
    last_modified TIMESTAMP NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'discovered',
    processing_tier VARCHAR(50),
    processed_at TIMESTAMP,
    processing_error TEXT,
    processing_duration BIGINT,
    language VARCHAR(10),
    language_confidence DECIMAL(3,2),
    content_category VARCHAR(100),
    sensitivity_level VARCHAR(50),
    classifications JSONB DEFAULT '[]',
    schema_info JSONB DEFAULT '{}',
    chunk_count INTEGER DEFAULT 0,
    chunking_strategy VARCHAR(100),
    pii_detected BOOLEAN DEFAULT false,
    compliance_flags JSONB DEFAULT '[]',
    encryption_status VARCHAR(50) DEFAULT 'none',
    metadata JSONB DEFAULT '{}',
    custom_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP
);

-- Create indexes for files
CREATE INDEX idx_files_tenant_id ON files(tenant_id);
CREATE INDEX idx_files_data_source_id ON files(data_source_id);
CREATE INDEX idx_files_processing_session_id ON files(processing_session_id);
CREATE INDEX idx_files_url ON files(url);
CREATE INDEX idx_files_path ON files(path);
CREATE INDEX idx_files_filename ON files(filename);
CREATE INDEX idx_files_extension ON files(extension);
CREATE INDEX idx_files_content_type ON files(content_type);
CREATE INDEX idx_files_status ON files(status);
CREATE INDEX idx_files_processing_tier ON files(processing_tier);
CREATE INDEX idx_files_checksum ON files(checksum);
CREATE INDEX idx_files_pii_detected ON files(pii_detected);
CREATE INDEX idx_files_deleted_at ON files(deleted_at);

-- Create chunks table
CREATE TABLE chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    chunk_id VARCHAR(255) NOT NULL,
    chunk_type VARCHAR(50) NOT NULL,
    chunk_number INTEGER NOT NULL,
    content TEXT NOT NULL,
    content_hash VARCHAR(255),
    size_bytes BIGINT NOT NULL,
    start_position BIGINT,
    end_position BIGINT,
    page_number INTEGER,
    line_number INTEGER,
    parent_chunk_id UUID REFERENCES chunks(id) ON DELETE SET NULL,
    relationships JSONB DEFAULT '[]',
    processed_at TIMESTAMP NOT NULL,
    processed_by VARCHAR(255) NOT NULL,
    processing_time BIGINT,
    quality JSONB DEFAULT '{}',
    language VARCHAR(10),
    language_confidence DECIMAL(3,2),
    content_category VARCHAR(100),
    sensitivity_level VARCHAR(50),
    classifications JSONB DEFAULT '[]',
    embedding_status VARCHAR(50) DEFAULT 'pending',
    embedding_model VARCHAR(100),
    embedding_vector JSONB,
    embedding_dimension INTEGER,
    embedded_at TIMESTAMP,
    pii_detected BOOLEAN DEFAULT false,
    compliance_flags JSONB DEFAULT '[]',
    dlp_scan_status VARCHAR(50) DEFAULT 'pending',
    dlp_scan_result VARCHAR(100),
    context JSONB DEFAULT '{}',
    schema_info JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    custom_fields JSONB DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP
);

-- Create indexes for chunks
CREATE INDEX idx_chunks_tenant_id ON chunks(tenant_id);
CREATE INDEX idx_chunks_file_id ON chunks(file_id);
CREATE INDEX idx_chunks_chunk_id ON chunks(chunk_id);
CREATE INDEX idx_chunks_chunk_type ON chunks(chunk_type);
CREATE INDEX idx_chunks_content_hash ON chunks(content_hash);
CREATE INDEX idx_chunks_parent_chunk_id ON chunks(parent_chunk_id);
CREATE INDEX idx_chunks_embedding_status ON chunks(embedding_status);
CREATE INDEX idx_chunks_pii_detected ON chunks(pii_detected);
CREATE INDEX idx_chunks_dlp_scan_status ON chunks(dlp_scan_status);
CREATE INDEX idx_chunks_deleted_at ON chunks(deleted_at);

-- Create dlp_violations table
CREATE TABLE dlp_violations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    policy_id UUID NOT NULL REFERENCES dlp_policies(id) ON DELETE CASCADE,
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    chunk_id UUID REFERENCES chunks(id) ON DELETE SET NULL,
    rule_name VARCHAR(255) NOT NULL,
    severity VARCHAR(50) NOT NULL,
    confidence DECIMAL(3,2) NOT NULL,
    matched_text TEXT,
    context TEXT,
    start_offset BIGINT,
    end_offset BIGINT,
    line_number INTEGER,
    actions_taken JSONB DEFAULT '[]',
    status VARCHAR(50) NOT NULL DEFAULT 'detected',
    acknowledged BOOLEAN DEFAULT false,
    acknowledged_by VARCHAR(255),
    acknowledged_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create indexes for dlp_violations
CREATE INDEX idx_dlp_violations_tenant_id ON dlp_violations(tenant_id);
CREATE INDEX idx_dlp_violations_policy_id ON dlp_violations(policy_id);
CREATE INDEX idx_dlp_violations_file_id ON dlp_violations(file_id);
CREATE INDEX idx_dlp_violations_chunk_id ON dlp_violations(chunk_id);
CREATE INDEX idx_dlp_violations_severity ON dlp_violations(severity);
CREATE INDEX idx_dlp_violations_status ON dlp_violations(status);
CREATE INDEX idx_dlp_violations_acknowledged ON dlp_violations(acknowledged);

-- Create updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create triggers for updated_at
CREATE TRIGGER update_tenants_updated_at BEFORE UPDATE ON tenants FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_data_sources_updated_at BEFORE UPDATE ON data_sources FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_processing_sessions_updated_at BEFORE UPDATE ON processing_sessions FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_dlp_policies_updated_at BEFORE UPDATE ON dlp_policies FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_files_updated_at BEFORE UPDATE ON files FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_chunks_updated_at BEFORE UPDATE ON chunks FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_dlp_violations_updated_at BEFORE UPDATE ON dlp_violations FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Create tenant isolation function
CREATE OR REPLACE FUNCTION ensure_tenant_isolation()
RETURNS TRIGGER AS $$
BEGIN
    -- Ensure that operations are only performed within the same tenant context
    -- This function can be extended to include more complex tenant isolation logic
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Add comments for documentation
COMMENT ON TABLE tenants IS 'Multi-tenant organizations with quotas and compliance settings';
COMMENT ON TABLE data_sources IS 'Configured data sources for each tenant';
COMMENT ON TABLE processing_sessions IS 'Document processing sessions with files and options';
COMMENT ON TABLE dlp_policies IS 'Data Loss Prevention policies and rules';
COMMENT ON TABLE files IS 'Individual files discovered and processed';
COMMENT ON TABLE chunks IS 'Chunks of content extracted from files';
COMMENT ON TABLE dlp_violations IS 'Violations detected by DLP policies';

-- Add constraints for data integrity
ALTER TABLE tenants ADD CONSTRAINT check_tenant_status CHECK (status IN ('active', 'suspended', 'deleted'));
ALTER TABLE data_sources ADD CONSTRAINT check_data_source_status CHECK (status IN ('active', 'inactive', 'error'));
ALTER TABLE processing_sessions ADD CONSTRAINT check_session_status CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled', 'completed_with_errors'));
ALTER TABLE dlp_policies ADD CONSTRAINT check_dlp_policy_status CHECK (status IN ('active', 'inactive', 'draft'));
ALTER TABLE files ADD CONSTRAINT check_file_status CHECK (status IN ('discovered', 'processing', 'processed', 'completed', 'error', 'failed'));
ALTER TABLE chunks ADD CONSTRAINT check_embedding_status CHECK (embedding_status IN ('pending', 'processing', 'completed', 'failed', 'skipped'));
ALTER TABLE chunks ADD CONSTRAINT check_dlp_scan_status CHECK (dlp_scan_status IN ('pending', 'processing', 'completed', 'failed', 'skipped'));
ALTER TABLE dlp_violations ADD CONSTRAINT check_violation_status CHECK (status IN ('detected', 'resolved', 'false_positive', 'accepted_risk'));
ALTER TABLE dlp_violations ADD CONSTRAINT check_violation_severity CHECK (severity IN ('low', 'medium', 'high', 'critical'));

-- Add performance constraints
ALTER TABLE files ADD CONSTRAINT check_file_size_positive CHECK (size >= 0);
ALTER TABLE chunks ADD CONSTRAINT check_chunk_size_positive CHECK (size_bytes >= 0);
ALTER TABLE processing_sessions ADD CONSTRAINT check_progress_range CHECK (progress >= 0 AND progress <= 100);
ALTER TABLE dlp_violations ADD CONSTRAINT check_confidence_range CHECK (confidence >= 0 AND confidence <= 1);

-- Migration completion
INSERT INTO public.schema_migrations (version, applied_at) VALUES ('001', NOW())
ON CONFLICT (version) DO NOTHING;