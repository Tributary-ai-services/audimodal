#!/bin/bash

# AudioModal Development Reset Script
# This script resets the development environment

set -e

echo "=========================================="
echo "AudioModal Development Reset"
echo "=========================================="

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Stop all services
log_info "Stopping services..."
docker-compose down --remove-orphans

# Remove volumes for fresh start
log_warning "Removing database volumes for fresh start..."
docker-compose down -v

# Remove any dangling images
log_info "Cleaning up Docker images..."
docker system prune -f

# Rebuild and start
log_info "Rebuilding and starting services..."
DOCKER_BUILDKIT=0 docker-compose up -d --build

# Wait for services
log_info "Waiting for services to be ready..."
sleep 15

# Check health
if curl -f http://localhost:8084/health &> /dev/null; then
    log_success "AudioModal is ready!"
    echo "API URL: http://localhost:8084"
    echo "Health Check: http://localhost:8084/health"
else
    log_warning "AudioModal may still be starting. Check logs:"
    echo "docker-compose logs -f audimodal"
fi

log_success "Development environment reset complete!"