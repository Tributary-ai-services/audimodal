package server

import (
	"time"

	"github.com/jscharber/eAIIngest/internal/database"
)

// Config represents the server configuration
type Config struct {
	// Server settings
	Host string `yaml:"host" env:"SERVER_HOST" default:"0.0.0.0"`
	Port int    `yaml:"port" env:"SERVER_PORT" default:"8080"`
	
	// TLS settings
	TLSEnabled  bool   `yaml:"tls_enabled" env:"TLS_ENABLED" default:"false"`
	TLSCertFile string `yaml:"tls_cert_file" env:"TLS_CERT_FILE" default:""`
	TLSKeyFile  string `yaml:"tls_key_file" env:"TLS_KEY_FILE" default:""`
	
	// Timeouts
	ReadTimeout       time.Duration `yaml:"read_timeout" env:"READ_TIMEOUT" default:"30s"`
	WriteTimeout      time.Duration `yaml:"write_timeout" env:"WRITE_TIMEOUT" default:"30s"`
	IdleTimeout       time.Duration `yaml:"idle_timeout" env:"IDLE_TIMEOUT" default:"120s"`
	ShutdownTimeout   time.Duration `yaml:"shutdown_timeout" env:"SHUTDOWN_TIMEOUT" default:"30s"`
	
	// CORS settings
	CORSEnabled        bool     `yaml:"cors_enabled" env:"CORS_ENABLED" default:"true"`
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins" env:"CORS_ALLOWED_ORIGINS" default:"*"`
	CORSAllowedMethods []string `yaml:"cors_allowed_methods" env:"CORS_ALLOWED_METHODS" default:"GET,POST,PUT,DELETE,OPTIONS"`
	CORSAllowedHeaders []string `yaml:"cors_allowed_headers" env:"CORS_ALLOWED_HEADERS" default:"*"`
	
	// Rate limiting
	RateLimitEnabled bool `yaml:"rate_limit_enabled" env:"RATE_LIMIT_ENABLED" default:"true"`
	RateLimitRPS     int  `yaml:"rate_limit_rps" env:"RATE_LIMIT_RPS" default:"100"`
	RateLimitBurst   int  `yaml:"rate_limit_burst" env:"RATE_LIMIT_BURST" default:"200"`
	
	// Authentication
	AuthEnabled    bool   `yaml:"auth_enabled" env:"AUTH_ENABLED" default:"true"`
	JWTSecret      string `yaml:"jwt_secret" env:"JWT_SECRET" default:""`
	JWTExpiration  time.Duration `yaml:"jwt_expiration" env:"JWT_EXPIRATION" default:"24h"`
	APIKeyHeader   string `yaml:"api_key_header" env:"API_KEY_HEADER" default:"X-API-Key"`
	
	// Request logging
	LogRequests    bool `yaml:"log_requests" env:"LOG_REQUESTS" default:"true"`
	LogRequestBody bool `yaml:"log_request_body" env:"LOG_REQUEST_BODY" default:"false"`
	
	// Health check
	HealthCheckPath string `yaml:"health_check_path" env:"HEALTH_CHECK_PATH" default:"/health"`
	
	// Metrics
	MetricsEnabled bool   `yaml:"metrics_enabled" env:"METRICS_ENABLED" default:"true"`
	MetricsPath    string `yaml:"metrics_path" env:"METRICS_PATH" default:"/metrics"`
	
	// API settings
	APIPrefix       string `yaml:"api_prefix" env:"API_PREFIX" default:"/api/v1"`
	MaxRequestSize  int64  `yaml:"max_request_size" env:"MAX_REQUEST_SIZE" default:"10485760"` // 10MB
	RequestIDHeader string `yaml:"request_id_header" env:"REQUEST_ID_HEADER" default:"X-Request-ID"`
	
	// Pagination defaults
	DefaultPageSize int `yaml:"default_page_size" env:"DEFAULT_PAGE_SIZE" default:"20"`
	MaxPageSize     int `yaml:"max_page_size" env:"MAX_PAGE_SIZE" default:"100"`
	
	// Database configuration
	Database *database.Config `yaml:"database"`
}

// GetDefaultConfig returns a default server configuration
func GetDefaultConfig() *Config {
	return &Config{
		Host:             "0.0.0.0",
		Port:             8080,
		TLSEnabled:       false,
		ReadTimeout:      30 * time.Second,
		WriteTimeout:     30 * time.Second,
		IdleTimeout:      120 * time.Second,
		ShutdownTimeout:  30 * time.Second,
		CORSEnabled:      true,
		CORSAllowedOrigins: []string{"*"},
		CORSAllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		CORSAllowedHeaders: []string{"*"},
		RateLimitEnabled: true,
		RateLimitRPS:     100,
		RateLimitBurst:   200,
		AuthEnabled:      true,
		JWTExpiration:    24 * time.Hour,
		APIKeyHeader:     "X-API-Key",
		LogRequests:      true,
		LogRequestBody:   false,
		HealthCheckPath:  "/health",
		MetricsEnabled:   true,
		MetricsPath:      "/metrics",
		APIPrefix:        "/api/v1",
		MaxRequestSize:   10 * 1024 * 1024, // 10MB
		RequestIDHeader:  "X-Request-ID",
		DefaultPageSize:  20,
		MaxPageSize:      100,
		Database:         database.GetDefaultConfig(),
	}
}

// GetAddress returns the server address
func (c *Config) GetAddress() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Validate validates the server configuration
func (c *Config) Validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}
	
	if c.TLSEnabled {
		if c.TLSCertFile == "" {
			return fmt.Errorf("TLS cert file is required when TLS is enabled")
		}
		if c.TLSKeyFile == "" {
			return fmt.Errorf("TLS key file is required when TLS is enabled")
		}
	}
	
	if c.AuthEnabled && c.JWTSecret == "" {
		return fmt.Errorf("JWT secret is required when authentication is enabled")
	}
	
	if c.RateLimitRPS <= 0 {
		return fmt.Errorf("rate limit RPS must be positive")
	}
	
	if c.RateLimitBurst <= 0 {
		return fmt.Errorf("rate limit burst must be positive")
	}
	
	if c.DefaultPageSize <= 0 {
		return fmt.Errorf("default page size must be positive")
	}
	
	if c.MaxPageSize <= 0 || c.MaxPageSize < c.DefaultPageSize {
		return fmt.Errorf("max page size must be positive and >= default page size")
	}
	
	return nil
}

// IsDevelopment checks if the server is running in development mode
func (c *Config) IsDevelopment() bool {
	return !c.TLSEnabled && c.LogRequestBody
}

// IsProduction checks if the server is running in production mode
func (c *Config) IsProduction() bool {
	return c.TLSEnabled && c.AuthEnabled && !c.LogRequestBody
}

import "fmt"