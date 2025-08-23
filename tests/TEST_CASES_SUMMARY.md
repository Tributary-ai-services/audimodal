# Document Processing Test Cases Summary

## Overview
This document outlines all test cases for validating the document upload, embedding generation, and search functionality in the AudiModal system.

## Test Categories

### 1. File Upload Tests (`TestFileUpload`)
- **Upload text document**: Validates basic text file upload with metadata
- **Upload PDF document**: Tests PDF file handling
- **Upload JSON document**: Tests structured data upload
- **Upload without metadata**: Tests minimal upload requirements
- **Upload large document**: Tests 1MB file upload handling

### 2. Embedding Generation Tests (`TestEmbeddingGeneration`)
- **Generate embeddings for short text**: Tests basic embedding generation
- **Generate embeddings with chunking**: Tests automatic text chunking for large content
- **Generate embeddings for technical content**: Tests domain-specific content processing

### 3. Vector Search Tests (`TestVectorSearch`)
- **Search healthcare content**: Tests semantic search for medical/healthcare queries
- **Search with metadata filter**: Tests filtered search using metadata constraints
- **Semantic similarity search**: Tests high-threshold similarity matching
- **Search with low threshold**: Tests broad search with relaxed similarity requirements

### 4. End-to-End Workflow Tests (`TestEndToEndWorkflow`)
- Complete pipeline test:
  1. Upload a document
  2. Wait for processing
  3. Generate embeddings
  4. Search for related content
  5. Validate uploaded document appears in results

### 5. Error Handling Tests (`TestErrorHandling`)
- **Upload without tenant ID**: Tests missing authentication
- **Search with invalid dataset**: Tests non-existent dataset handling
- **Upload file too large**: Tests file size limit enforcement

### 6. Concurrent Operations Tests (`TestConcurrentOperations`)
- **Concurrent file uploads**: Tests system under parallel upload load (10 concurrent)
- **Concurrent searches**: Tests system under parallel search load (20 concurrent)

### 7. Data Persistence Tests (`TestDataPersistence`)
- Tests that uploaded data and embeddings persist across multiple operations
- Validates search consistency over time

## Test Infrastructure

### Helper Functions
- `generateLargeContent(size)`: Creates test content of specified size
- `createTestDataset()`: Ensures test dataset exists in DeepLake
- `setupSearchTestData()`: Populates test documents for search tests
- `uploadTestFile()`: Helper for file upload operations
- `generateEmbeddingsForFile()`: Helper for embedding generation
- `performSearch()`: Helper for search operations

### Test Configuration
- Base URL: `http://localhost:8084`
- DeepLake URL: `http://localhost:8000`
- Test Tenant ID: `550e8400-e29b-41d4-a716-446655440000`
- Test Dataset: `test_documents`
- Default timeout: 30-60 seconds per operation

## Running the Tests

### Prerequisites
1. Start all required services:
   ```bash
   docker-compose up -d
   ```

2. Set environment variables:
   ```bash
   export OPENAI_API_KEY="your-api-key"
   ```

### Execute Tests
Run all tests:
```bash
./tests/run_document_tests.sh
```

Run specific test category:
```bash
go test -v -run TestFileUpload ./tests/document_processing_test.go
```

## Expected Outcomes

### Success Criteria
- All file uploads return 201 status
- Embeddings are generated for all content
- Search returns relevant results based on semantic similarity
- System handles concurrent operations without errors
- Data persists across operations

### Performance Expectations
- File upload: < 2 seconds for files up to 1MB
- Embedding generation: < 5 seconds for typical documents
- Search operations: < 1 second response time
- Concurrent operations: No degradation up to 20 parallel requests

## Test Data Examples

### Sample Documents
1. Healthcare AI content
2. Technical machine learning documentation
3. Medical diagnosis information
4. Natural language processing examples
5. Data processing descriptions

### Sample Queries
- "artificial intelligence in medical diagnosis"
- "machine learning applications"
- "patient treatment outcomes"
- "data processing"
- "healthcare technology"

## Notes
- Tests use fixed UUIDs for consistency
- DeepLake dataset is created automatically if it doesn't exist
- Tests include both positive and negative scenarios
- Concurrent tests validate system stability under load