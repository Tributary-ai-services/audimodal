# Configuration

This directory contains example configuration files for the AudiModal server.

## Configuration Sources

The server supports loading configuration from multiple sources in the following priority order (highest to lowest):

1. **Command Line Flags** - Override specific settings
2. **Configuration File** - YAML or JSON format
3. **Environment Variables** - Fallback values
4. **Default Values** - Built-in defaults

## Configuration Files

### Supported Formats

- **YAML**: `.yaml` or `.yml` files (recommended for readability)
- **JSON**: `.json` files (useful for programmatic generation)

### Example Files

- `server.yaml` - Complete YAML configuration example
- `server.json` - Complete JSON configuration example

## Usage

### Generate Example Configuration

```bash
# Generate YAML configuration
./server --generate-config config/my-config.yaml

# Generate JSON configuration  
./server --generate-config config/my-config.json
```

### Load Configuration

```bash
# Load from YAML file
./server --config config/server.yaml

# Load from JSON file
./server --config config/server.json
```

### Validate Configuration

```bash
# Validate configuration file
./server --config config/server.yaml --validate-config
```

### Override with Command Line Flags

```bash
# Load config file but override specific values
./server --config config/server.yaml --port 9090 --log-level debug
```

## Environment Variables

All configuration options can be set via environment variables. The naming convention is:

- Server settings: `SERVER_HOST`, `SERVER_PORT`
- Database settings: `DB_HOST`, `DB_PORT`, `DB_USERNAME`, `DB_PASSWORD`
- TLS settings: `TLS_ENABLED`, `TLS_CERT_FILE`, `TLS_KEY_FILE`
- Auth settings: `JWT_SECRET`, `AUTH_ENABLED`
- Logging: `LOG_LEVEL`, `LOG_FORMAT`

### Example

```bash
export SERVER_HOST=0.0.0.0
export SERVER_PORT=8080
export DB_HOST=localhost
export DB_PASSWORD=mypassword
export JWT_SECRET=your-secret-key
export LOG_LEVEL=debug

./server
```

## Configuration Validation

The server includes comprehensive configuration validation with the following features:

### Validation Types
- **Required fields**: Essential configuration values that must be provided
- **Range validation**: Numeric values within acceptable bounds
- **Format validation**: Strings matching specific patterns (URLs, file paths, etc.)
- **File existence**: TLS certificates and other files that must exist
- **Cross-field validation**: Relationships between different configuration values
- **Environment-specific rules**: Different validation for development vs production

### Production Security Validation
When running in production mode (TLS enabled + auth enabled), additional security validations apply:
- JWT secrets must be at least 64 characters
- TLS must be enabled
- Database SSL should be enabled
- Request/response body logging should be disabled
- CORS should not allow all origins (*)

### Validation Examples

```bash
# Validate configuration and show detailed errors
./server --config config/server.yaml --validate-config

# Example validation output
Configuration validation failed:
jwt_secret: JWT secret must be at least 32 characters for security (value: short)
rate_limit_burst: rate limit burst should be >= rate limit RPS (value: 50)
tls_cert_file: file does not exist (value: /path/to/missing/cert.pem)
```

## Configuration Sections

### Server Settings
- `host`: Server bind address
- `port`: Server listen port
- `tls_enabled`: Enable HTTPS
- `tls_cert_file`: TLS certificate file path
- `tls_key_file`: TLS private key file path

### Database Settings
- `database.host`: Database host
- `database.port`: Database port  
- `database.username`: Database username
- `database.password`: Database password
- `database.database`: Database name
- `database.ssl_mode`: SSL mode (disable, require, verify-ca, verify-full)

### Security Settings
- `auth_enabled`: Enable authentication
- `jwt_secret`: JWT signing secret (required if auth enabled)
- `jwt_expiration`: JWT token expiration time
- `rate_limit_enabled`: Enable rate limiting
- `rate_limit_rps`: Requests per second limit
- `rate_limit_burst`: Burst capacity

### Logging Settings
- `log_level`: Log level (debug, info, warn, error)
- `log_format`: Log format (json, text)
- `log_requests`: Enable request logging
- `log_request_body`: Log request bodies (development only)
- `log_response_body`: Log response bodies (development only)

### CORS Settings
- `cors_enabled`: Enable CORS
- `cors_allowed_origins`: Allowed origins
- `cors_allowed_methods`: Allowed HTTP methods
- `cors_allowed_headers`: Allowed headers

## Production Configuration

For production environments:

1. **Set secure JWT secret**: Use a long, random string
2. **Configure TLS**: Enable HTTPS with valid certificates
3. **Database security**: Use strong passwords and SSL
4. **Rate limiting**: Configure appropriate limits
5. **CORS**: Restrict allowed origins
6. **Logging**: Use JSON format for structured logging

Example production settings:

```yaml
tls_enabled: true
tls_cert_file: "/path/to/cert.pem"
tls_key_file: "/path/to/key.pem"
jwt_secret: "your-secure-random-secret-key-here"
cors_allowed_origins:
  - "https://yourdomain.com"
  - "https://app.yourdomain.com"
log_format: "json"
log_level: "info"
database:
  ssl_mode: "require"
  password: "secure-database-password"
```