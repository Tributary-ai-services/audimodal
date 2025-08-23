-- Initialize AudioModal database
-- This file is executed when the PostgreSQL container starts for the first time

-- Connect to the default database first
\c postgres;

-- Create the audimodal database if it doesn't exist
SELECT 'CREATE DATABASE audimodal' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'audimodal')\gexec

-- Connect to the audimodal database
\c audimodal;

-- Create extensions if needed
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";
CREATE EXTENSION IF NOT EXISTS "btree_gin";
CREATE EXTENSION IF NOT EXISTS "btree_gist";

-- Basic setup complete
SELECT 'AudioModal database initialized successfully' as status;