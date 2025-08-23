#!/bin/bash

# Document Processing Test Runner
# This script runs the comprehensive test suite for document upload,
# embedding generation, and search functionality

set -e

echo "=== Document Processing Test Suite ==="
echo "Testing document upload, embedding generation, and search functionality"
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if services are running
echo "Checking service health..."
services=(
    "http://localhost:8084/health|audimodal"
    "http://localhost:8000/api/v1/health|deeplake-api"
)

all_healthy=true
for service in "${services[@]}"; do
    IFS='|' read -r url name <<< "$service"
    if curl -s "$url" > /dev/null; then
        echo -e "${GREEN}✓${NC} $name is healthy"
    else
        echo -e "${RED}✗${NC} $name is not responding"
        all_healthy=false
    fi
done

if [ "$all_healthy" = false ]; then
    echo -e "\n${RED}Some services are not healthy. Please ensure all services are running.${NC}"
    exit 1
fi

echo -e "\n${GREEN}All services are healthy!${NC}\n"

# Set environment variables
export OPENAI_API_KEY=${OPENAI_API_KEY:-"your-api-key-here"}
export DEEPLAKE_API_URL=${DEEPLAKE_API_URL:-"http://localhost:8000"}
export TEST_TIMEOUT=${TEST_TIMEOUT:-"5m"}

# Run different test categories
test_categories=(
    "TestFileUpload|File Upload Tests"
    "TestEmbeddingGeneration|Embedding Generation Tests"
    "TestVectorSearch|Vector Search Tests"
    "TestEndToEndWorkflow|End-to-End Workflow Tests"
    "TestErrorHandling|Error Handling Tests"
    "TestConcurrentOperations|Concurrent Operations Tests"
    "TestDataPersistence|Data Persistence Tests"
)

total_tests=0
passed_tests=0
failed_tests=0

echo "Running test categories..."
echo "========================="

for category in "${test_categories[@]}"; do
    IFS='|' read -r test_name description <<< "$category"
    
    echo -e "\n${YELLOW}Running: $description${NC}"
    echo "Test pattern: $test_name"
    
    if go test -v -timeout $TEST_TIMEOUT -run "^$test_name$" ./tests/document_processing_test.go; then
        echo -e "${GREEN}✓ $description passed${NC}"
        ((passed_tests++))
    else
        echo -e "${RED}✗ $description failed${NC}"
        ((failed_tests++))
    fi
    ((total_tests++))
done

# Summary
echo -e "\n========================="
echo "Test Summary"
echo "========================="
echo -e "Total test categories: $total_tests"
echo -e "${GREEN}Passed: $passed_tests${NC}"
echo -e "${RED}Failed: $failed_tests${NC}"

if [ $failed_tests -eq 0 ]; then
    echo -e "\n${GREEN}All tests passed! 🎉${NC}"
    exit 0
else
    echo -e "\n${RED}Some tests failed. Please check the output above.${NC}"
    exit 1
fi