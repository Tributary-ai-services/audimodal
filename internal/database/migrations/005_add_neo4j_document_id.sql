-- Migration 005: Add neo4j_document_id for cross-service document ID consistency
-- Created: 2026-01-17
-- Purpose: Store the Neo4j Document.id from Aether-BE to enable cross-service data integrity
--          This solves the document ID mismatch issue (GitHub Issue #7) where AudiModal File.id
--          differs from Neo4j Document.id, making cross-service lookups unreliable.

-- Add neo4j_document_id column to files table
-- This column stores the UUID from Neo4j Document.id when a file is created via Aether-BE
ALTER TABLE files ADD COLUMN neo4j_document_id VARCHAR(36);

-- Create index for efficient lookups by neo4j_document_id
CREATE INDEX idx_files_neo4j_document_id ON files(neo4j_document_id);

-- Create unique index to ensure one-to-one mapping (only where value is not null)
-- This prevents duplicate mappings while allowing null values for files not created via Aether-BE
CREATE UNIQUE INDEX idx_files_neo4j_document_id_unique
    ON files(neo4j_document_id) WHERE neo4j_document_id IS NOT NULL;

-- Add comment for documentation
COMMENT ON COLUMN files.neo4j_document_id IS 'Neo4j Document.id from Aether-BE for cross-service data integrity. Null for files not created via Aether-BE.';
