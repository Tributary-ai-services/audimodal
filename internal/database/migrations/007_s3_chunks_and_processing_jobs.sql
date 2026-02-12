-- Migration 007: S3 Chunks and Processing Jobs for Kafka Pipeline
-- Clean slate: drop existing documents and chunks for dev mode

-- Clean slate: drop existing documents and chunks
TRUNCATE chunks CASCADE;
TRUNCATE files CASCADE;

-- Replace content TEXT with S3 reference on chunks table
ALTER TABLE chunks DROP COLUMN IF EXISTS content;
ALTER TABLE chunks ADD COLUMN IF NOT EXISTS s3_bucket VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE chunks ADD COLUMN IF NOT EXISTS s3_key VARCHAR(1024) NOT NULL DEFAULT '';
ALTER TABLE chunks ADD COLUMN IF NOT EXISTS content_preview VARCHAR(500);
CREATE INDEX IF NOT EXISTS idx_chunks_s3_key ON chunks(s3_key);

-- Processing jobs table for Kafka pipeline tracking
CREATE TABLE IF NOT EXISTS processing_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    file_id UUID NOT NULL REFERENCES files(id),
    total_pages INT NOT NULL,
    completed_pages INT DEFAULT 0,
    failed_pages INT DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'splitting',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP,
    error_message TEXT,
    metadata JSONB
);
CREATE INDEX IF NOT EXISTS idx_processing_jobs_file ON processing_jobs(file_id);
CREATE INDEX IF NOT EXISTS idx_processing_jobs_status ON processing_jobs(status);

-- Page results table for tracking individual page processing
CREATE TABLE IF NOT EXISTS pipeline_page_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES processing_jobs(id),
    tenant_id UUID NOT NULL,
    file_id UUID NOT NULL,
    page_number INT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    content_s3_key VARCHAR(1024),
    extraction_method VARCHAR(20),
    ocr_confidence FLOAT,
    processing_duration_ms BIGINT,
    error_type VARCHAR(30),
    error_message TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(job_id, page_number)
);
CREATE INDEX IF NOT EXISTS idx_pipeline_page_results_job ON pipeline_page_results(job_id);

-- Add trigger for updated_at on new tables
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

DROP TRIGGER IF EXISTS update_processing_jobs_updated_at ON processing_jobs;
CREATE TRIGGER update_processing_jobs_updated_at
    BEFORE UPDATE ON processing_jobs
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_pipeline_page_results_updated_at ON pipeline_page_results;
CREATE TRIGGER update_pipeline_page_results_updated_at
    BEFORE UPDATE ON pipeline_page_results
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
