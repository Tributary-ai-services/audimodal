-- Migration 004: Page Results for Map-Reduce PDF Processing
-- Created: 2026-01-15
-- Purpose: Intermediate storage for page-level PDF processing results
--          to support memory-efficient map-reduce processing pipeline

-- Create page_results table for intermediate PDF processing results
CREATE TABLE page_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    processing_session_id UUID REFERENCES processing_sessions(id) ON DELETE SET NULL,

    -- Page identification
    page_number INTEGER NOT NULL,
    total_pages INTEGER NOT NULL,

    -- Classification (determined before extraction)
    classification VARCHAR(50) NOT NULL, -- text_only, image_only, hybrid, scanned, empty
    classification_confidence DECIMAL(3,2),
    has_embedded_fonts BOOLEAN DEFAULT false,
    has_images BOOLEAN DEFAULT false,
    image_count INTEGER DEFAULT 0,
    large_image_count INTEGER DEFAULT 0, -- Images > 50% of page size
    text_char_count INTEGER DEFAULT 0,   -- Character count from quick pdftotext check

    -- Extraction results
    content TEXT,
    content_hash VARCHAR(255),
    extraction_method VARCHAR(50) NOT NULL DEFAULT 'pending', -- pdftotext, ocr, hybrid, skipped, pending
    ocr_confidence DECIMAL(3,2),
    ocr_language VARCHAR(10),
    ocr_dpi INTEGER,

    -- Processing metadata
    processing_status VARCHAR(50) NOT NULL DEFAULT 'pending', -- pending, classifying, extracting, completed, failed, skipped
    processing_error TEXT,
    processing_started_at TIMESTAMP,
    processing_completed_at TIMESTAMP,
    processing_duration_ms INTEGER,
    worker_id VARCHAR(100),
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,

    -- Memory and performance metrics
    peak_memory_bytes BIGINT,
    image_size_bytes BIGINT,

    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Indexes for efficient querying
CREATE INDEX idx_page_results_tenant_id ON page_results(tenant_id);
CREATE INDEX idx_page_results_file_id ON page_results(file_id);
CREATE INDEX idx_page_results_session_id ON page_results(processing_session_id);
CREATE INDEX idx_page_results_processing_status ON page_results(processing_status);
CREATE INDEX idx_page_results_classification ON page_results(classification);

-- Composite index for common query pattern: get all pages for a file
CREATE INDEX idx_page_results_file_page ON page_results(file_id, page_number);

-- Composite index for finding pending pages to process
CREATE INDEX idx_page_results_pending ON page_results(file_id, processing_status)
    WHERE processing_status IN ('pending', 'classifying', 'extracting');

-- Unique constraint: one result per page per file
CREATE UNIQUE INDEX idx_page_results_unique_page ON page_results(file_id, page_number);

-- Add constraint for valid classification
ALTER TABLE page_results ADD CONSTRAINT check_page_classification
    CHECK (classification IN ('text_only', 'image_only', 'hybrid', 'scanned', 'empty', 'unknown'));

-- Add constraint for valid status
ALTER TABLE page_results ADD CONSTRAINT check_page_status
    CHECK (processing_status IN ('pending', 'classifying', 'extracting', 'completed', 'failed', 'skipped'));

-- Add constraint for valid extraction method
ALTER TABLE page_results ADD CONSTRAINT check_extraction_method
    CHECK (extraction_method IN ('pdftotext', 'ocr', 'hybrid', 'skipped', 'pending'));

-- Add updated_at trigger (using existing function from 001_initial_schema.sql)
CREATE TRIGGER update_page_results_updated_at
    BEFORE UPDATE ON page_results
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Add comments for documentation
COMMENT ON TABLE page_results IS 'Intermediate storage for page-level PDF processing results in map-reduce pipeline';
COMMENT ON COLUMN page_results.classification IS 'Page type determined during classification: text_only (pdftotext sufficient), image_only (full OCR needed), hybrid (both text and images), scanned (no fonts, full page OCR), empty (skip)';
COMMENT ON COLUMN page_results.extraction_method IS 'Method used for extraction: pdftotext (fast text), ocr (tesseract), hybrid (both), skipped (empty page), pending (not yet extracted)';
COMMENT ON COLUMN page_results.processing_status IS 'Current processing state: pending, classifying, extracting, completed, failed, skipped';
COMMENT ON COLUMN page_results.worker_id IS 'Identifier of the subprocess worker that processed this page';
COMMENT ON COLUMN page_results.large_image_count IS 'Number of images larger than 50% of page dimensions (indicates possible scanned page)';
