-- Migration 005: Add FK constraints to ML analysis tables
-- Created: 2026-01-17
-- Resolves GitHub Issue #5: Orphaned ml_analysis_results when files deleted
--
-- Problem: ML analysis tables have document_id/chunk_id columns referencing
-- files and chunks tables, but no foreign key constraints. This allows
-- orphaned records when parent files/chunks are deleted.
--
-- Solution: Clean up orphaned records and add proper FK constraints with
-- appropriate ON DELETE behavior (CASCADE for non-nullable, SET NULL for nullable).

-- ============================================================================
-- STEP 1: Clean up orphaned records in ml_analysis_results
-- ============================================================================

-- Delete records where document_id references non-existent files
DELETE FROM ml_analysis_results
WHERE document_id NOT IN (SELECT id FROM files);

-- Set chunk_id to NULL where it references non-existent chunks
UPDATE ml_analysis_results
SET chunk_id = NULL
WHERE chunk_id IS NOT NULL
  AND chunk_id NOT IN (SELECT id FROM chunks);

-- ============================================================================
-- STEP 2: Clean up orphaned records in ml_analysis_jobs
-- ============================================================================

-- Delete records where document_id references non-existent files
DELETE FROM ml_analysis_jobs
WHERE document_id NOT IN (SELECT id FROM files);

-- Set chunk_id to NULL where it references non-existent chunks
UPDATE ml_analysis_jobs
SET chunk_id = NULL
WHERE chunk_id IS NOT NULL
  AND chunk_id NOT IN (SELECT id FROM chunks);

-- ============================================================================
-- STEP 3: Clean up orphaned records in ml_analysis_alerts
-- ============================================================================

-- Delete records where document_id references non-existent files
DELETE FROM ml_analysis_alerts
WHERE document_id NOT IN (SELECT id FROM files);

-- Set chunk_id to NULL where it references non-existent chunks
UPDATE ml_analysis_alerts
SET chunk_id = NULL
WHERE chunk_id IS NOT NULL
  AND chunk_id NOT IN (SELECT id FROM chunks);

-- ============================================================================
-- STEP 4: Clean up orphaned records in ml_analysis_metrics
-- ============================================================================

-- Set document_id to NULL where it references non-existent files
-- (document_id is nullable in ml_analysis_metrics)
UPDATE ml_analysis_metrics
SET document_id = NULL
WHERE document_id IS NOT NULL
  AND document_id NOT IN (SELECT id FROM files);

-- ============================================================================
-- STEP 5: Add FK constraints to ml_analysis_results
-- ============================================================================

-- document_id: NOT NULL, CASCADE delete (delete analysis when file deleted)
ALTER TABLE ml_analysis_results
ADD CONSTRAINT ml_analysis_results_document_id_fkey
FOREIGN KEY (document_id) REFERENCES files(id) ON DELETE CASCADE;

-- chunk_id: nullable, SET NULL on delete (preserve analysis, just unlink chunk)
ALTER TABLE ml_analysis_results
ADD CONSTRAINT ml_analysis_results_chunk_id_fkey
FOREIGN KEY (chunk_id) REFERENCES chunks(id) ON DELETE SET NULL;

-- ============================================================================
-- STEP 6: Add FK constraints to ml_analysis_jobs
-- ============================================================================

-- document_id: NOT NULL, CASCADE delete (delete job when file deleted)
ALTER TABLE ml_analysis_jobs
ADD CONSTRAINT ml_analysis_jobs_document_id_fkey
FOREIGN KEY (document_id) REFERENCES files(id) ON DELETE CASCADE;

-- chunk_id: nullable, SET NULL on delete (preserve job record, just unlink chunk)
ALTER TABLE ml_analysis_jobs
ADD CONSTRAINT ml_analysis_jobs_chunk_id_fkey
FOREIGN KEY (chunk_id) REFERENCES chunks(id) ON DELETE SET NULL;

-- ============================================================================
-- STEP 7: Add FK constraints to ml_analysis_alerts
-- ============================================================================

-- document_id: NOT NULL, CASCADE delete (delete alert when file deleted)
ALTER TABLE ml_analysis_alerts
ADD CONSTRAINT ml_analysis_alerts_document_id_fkey
FOREIGN KEY (document_id) REFERENCES files(id) ON DELETE CASCADE;

-- chunk_id: nullable, SET NULL on delete (preserve alert, just unlink chunk)
ALTER TABLE ml_analysis_alerts
ADD CONSTRAINT ml_analysis_alerts_chunk_id_fkey
FOREIGN KEY (chunk_id) REFERENCES chunks(id) ON DELETE SET NULL;

-- ============================================================================
-- STEP 8: Add FK constraints to ml_analysis_metrics
-- ============================================================================

-- document_id: nullable, SET NULL on delete (preserve metrics, just unlink file)
ALTER TABLE ml_analysis_metrics
ADD CONSTRAINT ml_analysis_metrics_document_id_fkey
FOREIGN KEY (document_id) REFERENCES files(id) ON DELETE SET NULL;

-- ============================================================================
-- STEP 9: Add documentation comments
-- ============================================================================

COMMENT ON CONSTRAINT ml_analysis_results_document_id_fkey ON ml_analysis_results
IS 'Ensures ml_analysis_results are deleted when parent file is deleted';

COMMENT ON CONSTRAINT ml_analysis_results_chunk_id_fkey ON ml_analysis_results
IS 'Sets chunk_id to NULL when referenced chunk is deleted';

COMMENT ON CONSTRAINT ml_analysis_jobs_document_id_fkey ON ml_analysis_jobs
IS 'Ensures ml_analysis_jobs are deleted when parent file is deleted';

COMMENT ON CONSTRAINT ml_analysis_jobs_chunk_id_fkey ON ml_analysis_jobs
IS 'Sets chunk_id to NULL when referenced chunk is deleted';

COMMENT ON CONSTRAINT ml_analysis_alerts_document_id_fkey ON ml_analysis_alerts
IS 'Ensures ml_analysis_alerts are deleted when parent file is deleted';

COMMENT ON CONSTRAINT ml_analysis_alerts_chunk_id_fkey ON ml_analysis_alerts
IS 'Sets chunk_id to NULL when referenced chunk is deleted';

COMMENT ON CONSTRAINT ml_analysis_metrics_document_id_fkey ON ml_analysis_metrics
IS 'Sets document_id to NULL when referenced file is deleted (preserves aggregated metrics)';

-- ============================================================================
-- Migration completion
-- ============================================================================
INSERT INTO schema_migrations (version, applied_at) VALUES ('005', NOW())
ON CONFLICT (version) DO NOTHING;
