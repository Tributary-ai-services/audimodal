-- ML Analysis tables migration for eAIIngest
-- Created: 2025-08-22
-- Adds ML analysis results, jobs, batches, configs, metrics, and alerts tables

-- Create ml_analysis_results table
CREATE TABLE ml_analysis_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL,
    chunk_id UUID,
    analysis_type VARCHAR(50) NOT NULL,
    result_data JSONB NOT NULL,
    confidence DECIMAL(5,4) DEFAULT 0.0,
    processing_time_ms BIGINT DEFAULT 0,
    model_versions JSONB,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    error TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create indexes for ml_analysis_results
CREATE INDEX idx_ml_analysis_results_document_id ON ml_analysis_results(document_id);
CREATE INDEX idx_ml_analysis_results_chunk_id ON ml_analysis_results(chunk_id);
CREATE INDEX idx_ml_analysis_results_analysis_type ON ml_analysis_results(analysis_type);
CREATE INDEX idx_ml_analysis_results_status ON ml_analysis_results(status);

-- Create ml_analysis_jobs table
CREATE TABLE ml_analysis_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL,
    chunk_id UUID,
    job_type VARCHAR(50) NOT NULL,
    priority INT DEFAULT 1,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    parameters JSONB,
    requested_by VARCHAR(100),
    error TEXT,
    retry_count INT DEFAULT 0,
    max_retries INT DEFAULT 3,
    scheduled_at TIMESTAMP,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create indexes for ml_analysis_jobs
CREATE INDEX idx_ml_analysis_jobs_document_id ON ml_analysis_jobs(document_id);
CREATE INDEX idx_ml_analysis_jobs_chunk_id ON ml_analysis_jobs(chunk_id);
CREATE INDEX idx_ml_analysis_jobs_job_type ON ml_analysis_jobs(job_type);
CREATE INDEX idx_ml_analysis_jobs_status ON ml_analysis_jobs(status);
CREATE INDEX idx_ml_analysis_jobs_priority ON ml_analysis_jobs(priority);

-- Create ml_analysis_batches table
CREATE TABLE ml_analysis_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255),
    description TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    total_jobs INT DEFAULT 0,
    completed_jobs INT DEFAULT 0,
    failed_jobs INT DEFAULT 0,
    parameters JSONB,
    requested_by VARCHAR(100),
    processing_time_ms BIGINT DEFAULT 0,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create indexes for ml_analysis_batches
CREATE INDEX idx_ml_analysis_batches_status ON ml_analysis_batches(status);
CREATE INDEX idx_ml_analysis_batches_requested_by ON ml_analysis_batches(requested_by);

-- Create ml_analysis_metrics table
CREATE TABLE ml_analysis_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID,
    time_frame VARCHAR(20) NOT NULL,
    date DATE NOT NULL,
    total_analyses INT DEFAULT 0,
    avg_confidence DECIMAL(5,4) DEFAULT 0.0,
    avg_processing_time_ms BIGINT DEFAULT 0,
    sentiment_positive INT DEFAULT 0,
    sentiment_negative INT DEFAULT 0,
    sentiment_neutral INT DEFAULT 0,
    topic_distribution JSONB,
    entity_counts JSONB,
    quality_scores JSONB,
    language_distribution JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create indexes for ml_analysis_metrics
CREATE INDEX idx_ml_analysis_metrics_document_id ON ml_analysis_metrics(document_id);
CREATE INDEX idx_ml_analysis_metrics_time_frame ON ml_analysis_metrics(time_frame);
CREATE INDEX idx_ml_analysis_metrics_date ON ml_analysis_metrics(date);

-- Create ml_analysis_configs table
CREATE TABLE ml_analysis_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    is_default BOOLEAN DEFAULT false,
    enable_sentiment BOOLEAN DEFAULT true,
    enable_topics BOOLEAN DEFAULT true,
    enable_entities BOOLEAN DEFAULT true,
    enable_summary BOOLEAN DEFAULT true,
    enable_quality BOOLEAN DEFAULT true,
    enable_language BOOLEAN DEFAULT true,
    enable_emotion BOOLEAN DEFAULT true,
    max_topics INT DEFAULT 5,
    summary_ratio DECIMAL(3,2) DEFAULT 0.3,
    min_confidence DECIMAL(3,2) DEFAULT 0.5,
    batch_size INT DEFAULT 10,
    processing_timeout_seconds INT DEFAULT 300,
    retry_attempts INT DEFAULT 3,
    cache_enabled BOOLEAN DEFAULT true,
    cache_ttl_seconds INT DEFAULT 3600,
    auto_analyze BOOLEAN DEFAULT false,
    analyze_triggers JSONB,
    model_preferences JSONB,
    custom_parameters JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100),
    updated_by VARCHAR(100)
);

-- Create indexes for ml_analysis_configs
CREATE INDEX idx_ml_analysis_configs_name ON ml_analysis_configs(name);
CREATE INDEX idx_ml_analysis_configs_is_default ON ml_analysis_configs(is_default);

-- Create ml_analysis_alerts table
CREATE TABLE ml_analysis_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL,
    chunk_id UUID,
    alert_type VARCHAR(50) NOT NULL,
    severity VARCHAR(20) NOT NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    condition JSONB,
    triggered BOOLEAN DEFAULT true,
    acknowledged BOOLEAN DEFAULT false,
    acknowledged_by VARCHAR(100),
    acknowledged_at TIMESTAMP,
    resolved_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create indexes for ml_analysis_alerts
CREATE INDEX idx_ml_analysis_alerts_document_id ON ml_analysis_alerts(document_id);
CREATE INDEX idx_ml_analysis_alerts_chunk_id ON ml_analysis_alerts(chunk_id);
CREATE INDEX idx_ml_analysis_alerts_alert_type ON ml_analysis_alerts(alert_type);
CREATE INDEX idx_ml_analysis_alerts_severity ON ml_analysis_alerts(severity);
CREATE INDEX idx_ml_analysis_alerts_acknowledged ON ml_analysis_alerts(acknowledged);

-- Add triggers for updated_at
CREATE TRIGGER update_ml_analysis_results_updated_at BEFORE UPDATE ON ml_analysis_results FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_ml_analysis_jobs_updated_at BEFORE UPDATE ON ml_analysis_jobs FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_ml_analysis_batches_updated_at BEFORE UPDATE ON ml_analysis_batches FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_ml_analysis_metrics_updated_at BEFORE UPDATE ON ml_analysis_metrics FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_ml_analysis_configs_updated_at BEFORE UPDATE ON ml_analysis_configs FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_ml_analysis_alerts_updated_at BEFORE UPDATE ON ml_analysis_alerts FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Add constraints
ALTER TABLE ml_analysis_results ADD CONSTRAINT check_ml_analysis_status CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'cancelled'));
ALTER TABLE ml_analysis_jobs ADD CONSTRAINT check_ml_job_status CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled'));
ALTER TABLE ml_analysis_batches ADD CONSTRAINT check_ml_batch_status CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled'));
ALTER TABLE ml_analysis_alerts ADD CONSTRAINT check_ml_alert_severity CHECK (severity IN ('low', 'medium', 'high', 'critical'));

-- Migration completion
INSERT INTO schema_migrations (version, applied_at) VALUES ('003', NOW())
ON CONFLICT (version) DO NOTHING;