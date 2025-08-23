#!/bin/bash

# AudioModal Setup Script
# This script initializes a new AudioModal instance from scratch

set -e

echo "=========================================="
echo "AudioModal Setup Script"
echo "=========================================="

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if Docker and Docker Compose are installed
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed. Please install Docker first."
        exit 1
    fi
    
    if ! command -v docker-compose &> /dev/null; then
        log_error "Docker Compose is not installed. Please install Docker Compose first."
        exit 1
    fi
    
    # Check if Docker daemon is running
    if ! docker info &> /dev/null; then
        log_error "Docker daemon is not running. Please start Docker first."
        exit 1
    fi
    
    log_success "Prerequisites check passed"
}

# Create necessary directories
create_directories() {
    log_info "Creating necessary directories..."
    
    mkdir -p data/storage
    mkdir -p logs
    mkdir -p temp
    mkdir -p deployments/postgres
    mkdir -p configs
    
    log_success "Directories created"
}

# Generate environment configuration
generate_env_config() {
    log_info "Generating environment configuration..."
    
    # Generate random secrets
    JWT_SECRET=$(openssl rand -base64 32)
    ENCRYPTION_KEY=$(openssl rand -base64 32)
    DB_PASSWORD=$(openssl rand -base64 16 | tr -d "=+/" | cut -c1-16)
    
    cat > .env << EOF
# AudioModal Environment Configuration
# Generated on $(date)

# Application Environment
EAI_ENV=production
LOG_LEVEL=info
LOG_FORMAT=json

# Database Configuration
DB_HOST=postgres
DB_PORT=5432
DB_DATABASE=audimodal
DB_USERNAME=audimodal-admin
DB_PASSWORD=$DB_PASSWORD
DB_AUTO_MIGRATE=true
DB_LOG_LEVEL=warn

# Security Configuration
JWT_SECRET=$JWT_SECRET
AUTH_ENABLED=true
EAI_ENCRYPTION_KEY=$ENCRYPTION_KEY

# Storage Configuration
EAI_STORAGE_LOCAL_PATH=/app/data/storage
EAI_TEMP_DIR=/app/temp

# Redis Configuration (optional)
EAI_REDIS_HOST=redis-shared
EAI_REDIS_PORT=6379

# Server Configuration
SERVER_PORT=8080
SERVER_HOST=0.0.0.0

# Metrics and Monitoring
METRICS_ENABLED=true
HEALTH_CHECK_TIMEOUT=30s

# CORS Configuration
CORS_ALLOWED_ORIGINS=*
CORS_ALLOWED_METHODS=GET,POST,PUT,DELETE,OPTIONS
CORS_ALLOWED_HEADERS=Content-Type,Authorization

# Rate Limiting
RATE_LIMIT_ENABLED=true
RATE_LIMIT_RPS=100
EOF

    log_success "Environment configuration generated (.env file created)"
}

# Update docker-compose.yml with generated values
update_docker_compose() {
    log_info "Updating Docker Compose configuration..."
    
    # Source the .env file to get the generated password
    source .env
    
    # Update docker-compose.yml with the new password
    sed -i "s/DB_PASSWORD=eaipassword/DB_PASSWORD=$DB_PASSWORD/g" docker-compose.yml
    sed -i "s/POSTGRES_PASSWORD=eaipassword/POSTGRES_PASSWORD=$DB_PASSWORD/g" docker-compose.yml
    sed -i "s/DB_DATABASE=eaiingest/DB_DATABASE=audimodal/g" docker-compose.yml
    sed -i "s/POSTGRES_DB=eaiingest/POSTGRES_DB=audimodal/g" docker-compose.yml
    sed -i "s/pg_isready -U audimodal-admin -d eaiingest/pg_isready -U audimodal-admin -d audimodal/g" docker-compose.yml
    
    log_success "Docker Compose configuration updated"
}

# Create database initialization script
create_db_init() {
    log_info "Creating database initialization script..."
    
    cat > deployments/postgres/init.sql << EOF
-- AudioModal Database Initialization
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

-- Create basic indexes for performance
-- Additional indexes will be created by migrations

-- Log successful initialization
SELECT 'AudioModal database initialized successfully' as status;
EOF

    log_success "Database initialization script created"
}

# Create network if it doesn't exist
create_network() {
    log_info "Creating Docker network..."
    
    if ! docker network inspect tas-shared-network &> /dev/null; then
        docker network create tas-shared-network
        log_success "Docker network 'tas-shared-network' created"
    else
        log_info "Docker network 'tas-shared-network' already exists"
    fi
}

# Build and start services
start_services() {
    log_info "Building and starting services..."
    
    # Stop any existing containers
    docker-compose down --remove-orphans 2>/dev/null || true
    
    # Remove any existing volumes for fresh start (optional)
    read -p "Do you want to remove existing database data for a fresh start? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        docker-compose down -v 2>/dev/null || true
        log_warning "Existing database data removed"
    fi
    
    # Build and start services
    log_info "Building application..."
    DOCKER_BUILDKIT=0 docker-compose build
    
    log_info "Starting services..."
    docker-compose up -d
    
    log_success "Services started"
}

# Wait for services to be healthy
wait_for_services() {
    log_info "Waiting for services to be healthy..."
    
    # Wait for PostgreSQL
    log_info "Waiting for PostgreSQL to be ready..."
    for i in {1..30}; do
        if docker-compose exec -T postgres pg_isready -U audimodal-admin -d audimodal &> /dev/null; then
            log_success "PostgreSQL is ready"
            break
        fi
        if [ $i -eq 30 ]; then
            log_error "PostgreSQL failed to start within 30 seconds"
            exit 1
        fi
        sleep 1
    done
    
    # Wait for AudioModal API
    log_info "Waiting for AudioModal API to be ready..."
    for i in {1..60}; do
        if curl -f http://localhost:8084/health &> /dev/null; then
            log_success "AudioModal API is ready"
            break
        fi
        if [ $i -eq 60 ]; then
            log_error "AudioModal API failed to start within 60 seconds"
            docker-compose logs audimodal
            exit 1
        fi
        sleep 1
    done
}

# Run database migrations
run_migrations() {
    log_info "Running database migrations..."
    
    # Check if migration binary exists
    if [ -f "./bin/migrate" ]; then
        source .env
        DB_HOST=localhost DB_PORT=5433 DB_DATABASE=audimodal DB_USERNAME=audimodal-admin DB_PASSWORD=$DB_PASSWORD ./bin/migrate -command migrate
        log_success "Database migrations completed"
    else
        log_warning "Migration binary not found. Migrations will run automatically via DB_AUTO_MIGRATE=true"
    fi
}

# Create initial admin user/tenant
create_initial_setup() {
    log_info "Creating initial setup..."
    
    # Create a default tenant
    TENANT_PAYLOAD='{"name":"default","display_name":"Default Organization","billing_plan":"enterprise","billing_email":"admin@audimodal.local","contact_info":{"admin_email":"admin@audimodal.local","billing_email":"admin@audimodal.local"}}'
    
    TENANT_RESPONSE=$(curl -s -X POST "http://localhost:8084/api/v1/tenants" \
        -H "Content-Type: application/json" \
        -d "$TENANT_PAYLOAD")
    
    if echo "$TENANT_RESPONSE" | grep -q '"success":true'; then
        TENANT_ID=$(echo "$TENANT_RESPONSE" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
        log_success "Default tenant created with ID: $TENANT_ID"
        
        # Save tenant ID for future reference
        echo "DEFAULT_TENANT_ID=$TENANT_ID" >> .env
    else
        log_error "Failed to create default tenant"
        echo "$TENANT_RESPONSE"
    fi
}

# Display final information
display_final_info() {
    log_success "Setup completed successfully!"
    echo
    echo "=========================================="
    echo "AudioModal Instance Information"
    echo "=========================================="
    echo "API URL: http://localhost:8084"
    echo "Health Check: http://localhost:8084/health"
    echo "API Documentation: http://localhost:8084/api/v1/docs"
    echo "Database: PostgreSQL on localhost:5433"
    echo
    echo "Configuration files:"
    echo "  - Environment: .env"
    echo "  - Docker Compose: docker-compose.yml"
    echo "  - Database Init: deployments/postgres/init.sql"
    echo
    echo "Default Credentials (change in production):"
    echo "  - Database User: audimodal-admin"
    echo "  - Database Password: (see .env file)"
    echo
    echo "Next Steps:"
    echo "  1. Review and customize the .env file"
    echo "  2. Set up proper SSL certificates for production"
    echo "  3. Configure external storage if needed"
    echo "  4. Set up monitoring and logging"
    echo "  5. Create additional tenants and users as needed"
    echo
    echo "To check service status:"
    echo "  docker-compose ps"
    echo
    echo "To view logs:"
    echo "  docker-compose logs -f"
    echo
    log_success "AudioModal is ready to use!"
}

# Cleanup function for failed setups
cleanup_on_failure() {
    log_error "Setup failed. Cleaning up..."
    docker-compose down --remove-orphans 2>/dev/null || true
    exit 1
}

# Trap for cleanup on failure
trap cleanup_on_failure ERR

# Main execution
main() {
    echo "Starting AudioModal setup process..."
    echo
    
    check_prerequisites
    create_directories
    generate_env_config
    update_docker_compose
    create_db_init
    create_network
    start_services
    wait_for_services
    run_migrations
    create_initial_setup
    display_final_info
}

# Run main function
main "$@"