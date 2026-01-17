# AudioModal Setup Guide

This guide will help you set up a new AudioModal instance from scratch.

## Quick Start

For a complete automated setup:

```bash
./setup.sh
```

This script will:
- Check prerequisites (Docker, Docker Compose)
- Generate secure configuration
- Create necessary directories and files
- Build and start all services
- Run database migrations
- Create initial tenant
- Provide access information

## Manual Setup

If you prefer to set up manually or need to customize the process:

### Prerequisites

- Docker 20.10+
- Docker Compose 1.29+
- Linux/macOS environment
- At least 4GB RAM available
- 10GB disk space

### Step 1: Environment Configuration

Create a `.env` file with your configuration:

```bash
# Copy example and customize
cp .env.example .env
nano .env
```

Key settings to configure:
- `DB_PASSWORD`: Secure database password
- `JWT_SECRET`: Secure JWT signing secret
- `EAI_ENCRYPTION_KEY`: 32-byte encryption key
- `AUTH_ENABLED`: Set to `true` for production

### Step 2: Database Setup

Update `docker-compose.yml` database settings:
- Username: `audimodal-admin`
- Database: `audimodal`
- Password: (from .env file)

### Step 3: Network Setup

Create Docker network:

```bash
docker network create tas-shared-network
```

### Step 4: Start Services

```bash
# Build and start services
DOCKER_BUILDKIT=0 docker-compose up -d --build

# Check service health
docker-compose ps
curl http://localhost:8084/health
```

### Step 5: Database Migration

If using manual migrations:

```bash
# Set database connection variables
export DB_HOST=localhost
export DB_PORT=5433
export DB_DATABASE=audimodal
export DB_USERNAME=audimodal-admin
export DB_PASSWORD=your_password

# Run migrations
./bin/migrate -command migrate
```

### Step 6: Create Initial Tenant

```bash
curl -X POST "http://localhost:8084/api/v1/tenants" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "default",
    "display_name": "Default Organization",
    "billing_plan": "enterprise",
    "billing_email": "admin@example.com",
    "contact_info": {
      "admin_email": "admin@example.com",
      "billing_email": "admin@example.com"
    }
  }'
```

## Development Environment

For development purposes, use the reset script:

```bash
./reset-dev.sh
```

This will:
- Stop all services
- Remove volumes (fresh database)
- Rebuild containers
- Start services
- Basic health check

## Configuration

### Environment Variables

Key environment variables in `.env`:

| Variable | Description | Default |
|----------|-------------|---------|
| `EAI_ENV` | Environment (development/production) | `production` |
| `DB_USERNAME` | Database username | `audimodal-admin` |
| `DB_PASSWORD` | Database password | (generated) |
| `DB_DATABASE` | Database name | `audimodal` |
| `AUTH_ENABLED` | Enable authentication | `true` |
| `LOG_LEVEL` | Logging level | `info` |
| `METRICS_ENABLED` | Enable metrics collection | `true` |

### Database Configuration

The PostgreSQL database is configured with:
- Username: `audimodal-admin`
- Database: `audimodal`
- Port: 5433 (external), 5432 (internal)
- Extensions: uuid-ossp, pg_trgm, btree_gin, btree_gist

### Security

For production deployments:
- Change all default passwords
- Enable authentication (`AUTH_ENABLED=true`)
- Use secure JWT secrets
- Set up SSL/TLS certificates
- Configure proper firewall rules
- Regular security updates

## Service URLs

After setup:
- **API**: http://localhost:8084
- **Health Check**: http://localhost:8084/health
- **API Documentation**: http://localhost:8084/api/v1/docs
- **Metrics**: http://localhost:8084/metrics
- **Database**: localhost:5433

## Troubleshooting

### Services won't start

1. Check Docker daemon is running
2. Verify port availability (8084, 5433)
3. Check logs: `docker-compose logs -f`
4. Ensure sufficient resources (RAM/disk)

### Database connection issues

1. Wait for PostgreSQL to fully initialize (30-60 seconds)
2. Check credentials in `.env` and `docker-compose.yml`
3. Verify network connectivity: `docker network ls`

### Migration failures

1. Ensure database is running and accessible
2. Check migration binary exists: `ls -la bin/migrate`
3. Verify database credentials
4. Check migration logs

### API not responding

1. Check container status: `docker-compose ps`
2. View application logs: `docker-compose logs audimodal`
3. Verify health endpoint: `curl http://localhost:8084/health`
4. Check port binding conflicts

## Maintenance

### Backup Database

```bash
docker exec audimodal-postgres pg_dump -U audimodal-admin audimodal > backup.sql
```

### Restore Database

```bash
docker exec -i audimodal-postgres psql -U audimodal-admin audimodal < backup.sql
```

### Update Application

```bash
git pull
DOCKER_BUILDKIT=0 docker-compose up -d --build
```

### Clean Up

```bash
# Stop and remove everything
docker-compose down -v --rmi all

# Clean up Docker system
docker system prune -a
```

## Support

For issues or questions:
1. Check the logs: `docker-compose logs`
2. Review configuration files
3. Verify prerequisites are met
4. Check GitHub issues/documentation