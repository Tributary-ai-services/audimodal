#!/bin/bash

# Script to run integration tests for file upload functionality

set -e

echo "🧪 Running File Upload Integration Tests"
echo "========================================"

# Check if database is running
if ! nc -z localhost 5433 2>/dev/null; then
    echo "❌ Database not running on localhost:5433"
    echo "Please start the database with: docker-compose up -d audimodal-postgres"
    exit 1
fi

# Set test environment variables
export TEST_DB_HOST=localhost
export TEST_DB_PORT=5433
export TEST_DB_NAME=audimodal
export TEST_DB_USER=audimodal-admin
export TEST_DB_PASSWORD=eaipassword

# MinIO configuration for storage tests
export AWS_ENDPOINT_URL=http://localhost:9000
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin123
export AWS_REGION=us-east-1
export AWS_S3_BUCKET=audimodal-uploads
export AWS_S3_FORCE_PATH_STYLE=true

# Encryption key for storage service
export EAI_ENCRYPTION_KEY=test-encryption-key-32-bytes-xxx

# Check if MinIO is available
if nc -z localhost 9000 2>/dev/null; then
    echo "✅ MinIO detected on localhost:9000"
    echo "   Storage integration tests will run"
else
    echo "⚠️  MinIO not detected on localhost:9000"
    echo "   Storage integration tests will be skipped"
    echo "   To run storage tests, start MinIO with shared infrastructure"
fi

echo ""
echo "Running tests..."
echo ""

# Run integration tests
go test -v ./tests -run TestFileUpload -count=1

echo ""
echo "✅ Integration tests completed"