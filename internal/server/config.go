package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/pkg/config"
	"github.com/jscharber/eAIIngest/pkg/validation"
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
	ReadTimeout     time.Duration `yaml:"read_timeout" env:"READ_TIMEOUT" default:"30s"`
	WriteTimeout    time.Duration `yaml:"write_timeout" env:"WRITE_TIMEOUT" default:"30s"`
	IdleTimeout     time.Duration `yaml:"idle_timeout" env:"IDLE_TIMEOUT" default:"120s"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env:"SHUTDOWN_TIMEOUT" default:"30s"`

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
	AuthEnabled   bool          `yaml:"auth_enabled" env:"AUTH_ENABLED" default:"true"`
	JWTSecret     string        `yaml:"jwt_secret" env:"JWT_SECRET" default:""`
	JWTExpiration time.Duration `yaml:"jwt_expiration" env:"JWT_EXPIRATION" default:"24h"`
	APIKeyHeader  string        `yaml:"api_key_header" env:"API_KEY_HEADER" default:"X-API-Key"`

	// Request logging
	LogRequests     bool   `yaml:"log_requests" env:"LOG_REQUESTS" default:"true"`
	LogRequestBody  bool   `yaml:"log_request_body" env:"LOG_REQUEST_BODY" default:"false"`
	LogResponseBody bool   `yaml:"log_response_body" env:"LOG_RESPONSE_BODY" default:"false"`
	LogHeaders      bool   `yaml:"log_headers" env:"LOG_HEADERS" default:"false"`
	LogLevel        string `yaml:"log_level" env:"LOG_LEVEL" default:"info"`
	LogFormat       string `yaml:"log_format" env:"LOG_FORMAT" default:"json"`

	// Health check
	HealthCheckPath    string        `yaml:"health_check_path" env:"HEALTH_CHECK_PATH" default:"/health"`
	HealthCheckTimeout time.Duration `yaml:"health_check_timeout" env:"HEALTH_CHECK_TIMEOUT" default:"30s"`

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

	// DeepLake API configuration
	DeepLakeAPI *DeepLakeAPIConfig `yaml:"deeplake_api"`
}

// DeepLakeAPIConfig represents the DeepLake API configuration
type DeepLakeAPIConfig struct {
	// API connection settings
	BaseURL   string        `yaml:"base_url" env:"DEEPLAKE_API_URL" default:"http://localhost:8000"`
	APIKey    string        `yaml:"api_key" env:"DEEPLAKE_API_KEY" default:""`
	TenantID  string        `yaml:"tenant_id" env:"DEEPLAKE_TENANT_ID" default:""`
	Timeout   time.Duration `yaml:"timeout" env:"DEEPLAKE_API_TIMEOUT" default:"30s"`
	Retries   int           `yaml:"retries" env:"DEEPLAKE_API_RETRIES" default:"3"`
	UserAgent string        `yaml:"user_agent" env:"DEEPLAKE_API_USER_AGENT" default:"eAIIngest-Go/1.0"`

	// Connection pool settings
	MaxIdleConns    int           `yaml:"max_idle_conns" env:"DEEPLAKE_API_MAX_IDLE_CONNS" default:"10"`
	MaxConnsPerHost int           `yaml:"max_conns_per_host" env:"DEEPLAKE_API_MAX_CONNS_PER_HOST" default:"100"`
	IdleConnTimeout time.Duration `yaml:"idle_conn_timeout" env:"DEEPLAKE_API_IDLE_CONN_TIMEOUT" default:"90s"`

	// TLS settings for API client
	TLSInsecureSkipVerify bool   `yaml:"tls_insecure_skip_verify" env:"DEEPLAKE_API_TLS_INSECURE_SKIP_VERIFY" default:"false"`
	TLSCertFile           string `yaml:"tls_cert_file" env:"DEEPLAKE_API_TLS_CERT_FILE" default:""`
	TLSKeyFile            string `yaml:"tls_key_file" env:"DEEPLAKE_API_TLS_KEY_FILE" default:""`
	TLSCAFile             string `yaml:"tls_ca_file" env:"DEEPLAKE_API_TLS_CA_FILE" default:""`

	// Health check settings
	HealthCheckEnabled  bool          `yaml:"health_check_enabled" env:"DEEPLAKE_API_HEALTH_CHECK_ENABLED" default:"true"`
	HealthCheckInterval time.Duration `yaml:"health_check_interval" env:"DEEPLAKE_API_HEALTH_CHECK_INTERVAL" default:"30s"`
	HealthCheckTimeout  time.Duration `yaml:"health_check_timeout" env:"DEEPLAKE_API_HEALTH_CHECK_TIMEOUT" default:"10s"`

	// Cache settings
	CacheEnabled bool          `yaml:"cache_enabled" env:"DEEPLAKE_API_CACHE_ENABLED" default:"true"`
	CacheTTL     time.Duration `yaml:"cache_ttl" env:"DEEPLAKE_API_CACHE_TTL" default:"5m"`
	CacheMaxSize int           `yaml:"cache_max_size" env:"DEEPLAKE_API_CACHE_MAX_SIZE" default:"1000"`

	// Rate limiting for API calls
	RateLimitEnabled bool    `yaml:"rate_limit_enabled" env:"DEEPLAKE_API_RATE_LIMIT_ENABLED" default:"false"`
	RateLimitRPS     float64 `yaml:"rate_limit_rps" env:"DEEPLAKE_API_RATE_LIMIT_RPS" default:"100"`
	RateLimitBurst   int     `yaml:"rate_limit_burst" env:"DEEPLAKE_API_RATE_LIMIT_BURST" default:"200"`
}

// GetDefaultConfig returns a default server configuration
func GetDefaultConfig() *Config {
	return &Config{
		Host:               "0.0.0.0",
		Port:               8080,
		TLSEnabled:         false,
		ReadTimeout:        30 * time.Second,
		WriteTimeout:       30 * time.Second,
		IdleTimeout:        120 * time.Second,
		ShutdownTimeout:    30 * time.Second,
		CORSEnabled:        true,
		CORSAllowedOrigins: []string{"*"},
		CORSAllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		CORSAllowedHeaders: []string{"*"},
		RateLimitEnabled:   true,
		RateLimitRPS:       100,
		RateLimitBurst:     200,
		AuthEnabled:        true,
		JWTExpiration:      24 * time.Hour,
		APIKeyHeader:       "X-API-Key",
		LogRequests:        true,
		LogRequestBody:     false,
		LogResponseBody:    false,
		LogHeaders:         false,
		LogLevel:           "info",
		LogFormat:          "json",
		HealthCheckPath:    "/health",
		HealthCheckTimeout: 30 * time.Second,
		MetricsEnabled:     true,
		MetricsPath:        "/metrics",
		APIPrefix:          "/api/v1",
		MaxRequestSize:     10 * 1024 * 1024, // 10MB
		RequestIDHeader:    "X-Request-ID",
		DefaultPageSize:    20,
		MaxPageSize:        100,
		Database:           database.GetDefaultConfig(),
		DeepLakeAPI:        GetDefaultDeepLakeAPIConfig(),
	}
}

// GetDefaultDeepLakeAPIConfig returns a default DeepLake API configuration
func GetDefaultDeepLakeAPIConfig() *DeepLakeAPIConfig {
	return &DeepLakeAPIConfig{
		BaseURL:               "http://localhost:8000",
		APIKey:                "",
		TenantID:              "",
		Timeout:               30 * time.Second,
		Retries:               3,
		UserAgent:             "eAIIngest-Go/1.0",
		MaxIdleConns:          10,
		MaxConnsPerHost:       100,
		IdleConnTimeout:       90 * time.Second,
		TLSInsecureSkipVerify: false,
		HealthCheckEnabled:    true,
		HealthCheckInterval:   30 * time.Second,
		HealthCheckTimeout:    10 * time.Second,
		CacheEnabled:          true,
		CacheTTL:              5 * time.Minute,
		CacheMaxSize:          1000,
		RateLimitEnabled:      false,
		RateLimitRPS:          100,
		RateLimitBurst:        200,
	}
}

// GetAddress returns the server address
func (c *Config) GetAddress() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Validate validates the server configuration
func (c *Config) Validate() error {
	v := validation.NewValidator()

	// Server settings validation
	v.Range("port", c.Port, 1, 65535, "").
		Range("read_timeout", int(c.ReadTimeout.Seconds()), 1, 3600, "read timeout must be between 1s and 1h").
		Range("write_timeout", int(c.WriteTimeout.Seconds()), 1, 3600, "write timeout must be between 1s and 1h").
		Range("idle_timeout", int(c.IdleTimeout.Seconds()), 1, 7200, "idle timeout must be between 1s and 2h").
		Range("shutdown_timeout", int(c.ShutdownTimeout.Seconds()), 1, 300, "shutdown timeout must be between 1s and 5m")

	// TLS validation
	v.When(c.TLSEnabled, func(v *validation.Validator) {
		v.Required("tls_cert_file", c.TLSCertFile, "TLS certificate file is required when TLS is enabled").
			Required("tls_key_file", c.TLSKeyFile, "TLS private key file is required when TLS is enabled")

		// Check if TLS files exist
		if c.TLSCertFile != "" {
			v.FileExists("tls_cert_file", c.TLSCertFile, "").
				FileExtension("tls_cert_file", c.TLSCertFile, []string{".pem", ".crt", ".cert"}, "TLS certificate must be a .pem, .crt, or .cert file")
		}
		if c.TLSKeyFile != "" {
			v.FileExists("tls_key_file", c.TLSKeyFile, "").
				FileExtension("tls_key_file", c.TLSKeyFile, []string{".pem", ".key"}, "TLS private key must be a .pem or .key file")
		}
	})

	// Authentication validation
	v.When(c.AuthEnabled, func(v *validation.Validator) {
		v.Required("jwt_secret", c.JWTSecret, "JWT secret is required when authentication is enabled").
			MinLength("jwt_secret", c.JWTSecret, 32, "JWT secret must be at least 32 characters for security")
	})

	// Rate limiting validation
	v.When(c.RateLimitEnabled, func(v *validation.Validator) {
		v.Min("rate_limit_rps", c.RateLimitRPS, 1, "rate limit RPS must be positive").
			Min("rate_limit_burst", c.RateLimitBurst, 1, "rate limit burst must be positive").
			Range("rate_limit_rps", c.RateLimitRPS, 1, 10000, "rate limit RPS should be between 1 and 10000").
			Range("rate_limit_burst", c.RateLimitBurst, 1, 50000, "rate limit burst should be between 1 and 50000")

		// Burst should be >= RPS for proper rate limiting
		if c.RateLimitBurst < c.RateLimitRPS {
			v.AddError("rate_limit_burst", fmt.Sprintf("%d", c.RateLimitBurst),
				"rate limit burst should be >= rate limit RPS", "burst_too_low")
		}
	})

	// Logging validation
	v.OneOf("log_level", c.LogLevel, []string{"debug", "info", "warn", "error", "fatal"}, "").
		OneOf("log_format", c.LogFormat, []string{"json", "text"}, "")

	// API validation
	v.Required("api_prefix", c.APIPrefix, "").
		Pattern("api_prefix", c.APIPrefix, `^/[a-zA-Z0-9/_-]+$`, "API prefix must start with / and contain only alphanumeric characters, underscores, and hyphens").
		Required("health_check_path", c.HealthCheckPath, "").
		Pattern("health_check_path", c.HealthCheckPath, `^/[a-zA-Z0-9/_-]*$`, "health check path must start with / and contain only alphanumeric characters, underscores, and hyphens")

	// Request size validation
	v.Range("max_request_size", int(c.MaxRequestSize/(1024*1024)), 1, 1000, "max request size must be between 1MB and 1GB")

	// Pagination validation
	v.Min("default_page_size", c.DefaultPageSize, 1, "").
		Min("max_page_size", c.MaxPageSize, 1, "").
		Range("default_page_size", c.DefaultPageSize, 1, 1000, "default page size should be between 1 and 1000").
		Range("max_page_size", c.MaxPageSize, 1, 10000, "max page size should be between 1 and 10000")

	if c.MaxPageSize < c.DefaultPageSize {
		v.AddError("max_page_size", fmt.Sprintf("%d", c.MaxPageSize),
			"max page size must be >= default page size", "max_less_than_default")
	}

	// Metrics validation
	v.When(c.MetricsEnabled, func(v *validation.Validator) {
		v.Required("metrics_path", c.MetricsPath, "").
			Pattern("metrics_path", c.MetricsPath, `^/[a-zA-Z0-9/_-]*$`, "metrics path must start with / and contain only alphanumeric characters, underscores, and hyphens")
	})

	// CORS validation
	v.When(c.CORSEnabled, func(v *validation.Validator) {
		// Validate CORS methods
		allowedMethods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"}
		for _, method := range c.CORSAllowedMethods {
			v.OneOf("cors_allowed_methods", method, allowedMethods, fmt.Sprintf("invalid CORS method: %s", method))
		}
	})

	// Environment-specific validation
	if c.IsProduction() {
		v.When(c.JWTSecret != "", func(v *validation.Validator) {
			v.MinLength("jwt_secret", c.JWTSecret, 64, "JWT secret should be at least 64 characters in production")
		})

		// Production should use TLS
		if !c.TLSEnabled {
			v.AddError("tls_enabled", "false", "TLS should be enabled in production", "production_security")
		}

		// Production should not log request/response bodies
		if c.LogRequestBody || c.LogResponseBody {
			v.AddError("log_request_body", fmt.Sprintf("%t", c.LogRequestBody),
				"request/response body logging should be disabled in production", "production_security")
		}

		// CORS should be restrictive in production
		if c.CORSEnabled && len(c.CORSAllowedOrigins) == 1 && c.CORSAllowedOrigins[0] == "*" {
			v.AddError("cors_allowed_origins", "*",
				"CORS should not allow all origins (*) in production", "production_security")
		}
	}

	// Validate database configuration if present
	if c.Database != nil {
		if err := c.validateDatabase(v); err != nil {
			return fmt.Errorf("database configuration validation failed: %w", err)
		}
	}

	// Validate DeepLake API configuration if present
	if c.DeepLakeAPI != nil {
		if err := c.validateDeepLakeAPI(v); err != nil {
			return fmt.Errorf("DeepLake API configuration validation failed: %w", err)
		}
	}

	if v.HasErrors() {
		return v.GetErrors()
	}

	return nil
}

// validateDatabase validates database configuration
func (c *Config) validateDatabase(v *validation.Validator) error {
	db := c.Database

	// Basic database validation
	v.Required("database.host", db.Host, "").
		Port("database.port", db.Port, "").
		Required("database.username", db.Username, "").
		Required("database.database", db.Database, "")

	// Connection pool validation
	v.Min("database.max_open_conns", db.MaxOpenConns, 1, "").
		Min("database.max_idle_conns", db.MaxIdleConns, 1, "").
		Max("database.max_open_conns", db.MaxOpenConns, 1000, "max open connections should not exceed 1000").
		Max("database.max_idle_conns", db.MaxIdleConns, 100, "max idle connections should not exceed 100")

	if db.MaxIdleConns > db.MaxOpenConns {
		v.AddError("database.max_idle_conns", fmt.Sprintf("%d", db.MaxIdleConns),
			"max idle connections cannot exceed max open connections", "idle_exceeds_max")
	}

	// SSL mode validation
	v.OneOf("database.ssl_mode", db.SSLMode, []string{"disable", "require", "verify-ca", "verify-full"}, "")

	// Duration validation
	v.Duration("database.conn_max_lifetime", db.ConnMaxLifetime.String(), "").
		Duration("database.conn_max_idle_time", db.ConnMaxIdleTime.String(), "").
		Duration("database.slow_threshold", db.SlowThreshold.String(), "")

	// Log level validation
	v.OneOf("database.log_level", db.LogLevel, []string{"silent", "error", "warn", "info"}, "")

	// Production database validation
	if c.IsProduction() {
		// Production should use SSL
		if db.SSLMode == "disable" {
			v.AddError("database.ssl_mode", db.SSLMode,
				"database SSL should be enabled in production", "production_security")
		}

		// Production should have a password
		if db.Password == "" {
			v.AddError("database.password", "",
				"database password should be set in production", "production_security")
		}
	}

	return nil
}

// validateDeepLakeAPI validates DeepLake API configuration
func (c *Config) validateDeepLakeAPI(v *validation.Validator) error {
	api := c.DeepLakeAPI

	// Basic API validation
	v.Required("deeplake_api.base_url", api.BaseURL, "").
		URL("deeplake_api.base_url", api.BaseURL, "")

	// Connection settings validation
	v.Min("deeplake_api.retries", api.Retries, 0, "").
		Max("deeplake_api.retries", api.Retries, 10, "retries should not exceed 10").
		Duration("deeplake_api.timeout", api.Timeout.String(), "").
		Range("deeplake_api.timeout", int(api.Timeout.Seconds()), 1, 300, "timeout should be between 1s and 5m")

	// Connection pool validation
	v.Min("deeplake_api.max_idle_conns", api.MaxIdleConns, 1, "").
		Min("deeplake_api.max_conns_per_host", api.MaxConnsPerHost, 1, "").
		Max("deeplake_api.max_idle_conns", api.MaxIdleConns, 1000, "max idle connections should not exceed 1000").
		Max("deeplake_api.max_conns_per_host", api.MaxConnsPerHost, 10000, "max connections per host should not exceed 10000").
		Duration("deeplake_api.idle_conn_timeout", api.IdleConnTimeout.String(), "")

	if api.MaxIdleConns > api.MaxConnsPerHost {
		v.AddError("deeplake_api.max_idle_conns", fmt.Sprintf("%d", api.MaxIdleConns),
			"max idle connections cannot exceed max connections per host", "idle_exceeds_max")
	}

	// TLS validation
	if api.TLSCertFile != "" {
		v.FileExists("deeplake_api.tls_cert_file", api.TLSCertFile, "").
			FileExtension("deeplake_api.tls_cert_file", api.TLSCertFile, []string{".pem", ".crt", ".cert"}, "TLS certificate must be a .pem, .crt, or .cert file")
	}
	if api.TLSKeyFile != "" {
		v.FileExists("deeplake_api.tls_key_file", api.TLSKeyFile, "").
			FileExtension("deeplake_api.tls_key_file", api.TLSKeyFile, []string{".pem", ".key"}, "TLS private key must be a .pem or .key file")
	}
	if api.TLSCAFile != "" {
		v.FileExists("deeplake_api.tls_ca_file", api.TLSCAFile, "").
			FileExtension("deeplake_api.tls_ca_file", api.TLSCAFile, []string{".pem", ".crt", ".cert"}, "TLS CA certificate must be a .pem, .crt, or .cert file")
	}

	// Health check validation
	v.When(api.HealthCheckEnabled, func(v *validation.Validator) {
		v.Duration("deeplake_api.health_check_interval", api.HealthCheckInterval.String(), "").
			Duration("deeplake_api.health_check_timeout", api.HealthCheckTimeout.String(), "").
			Range("deeplake_api.health_check_interval", int(api.HealthCheckInterval.Seconds()), 5, 300, "health check interval should be between 5s and 5m").
			Range("deeplake_api.health_check_timeout", int(api.HealthCheckTimeout.Seconds()), 1, 60, "health check timeout should be between 1s and 1m")

		if api.HealthCheckTimeout >= api.HealthCheckInterval {
			v.AddError("deeplake_api.health_check_timeout", api.HealthCheckTimeout.String(),
				"health check timeout must be less than health check interval", "timeout_exceeds_interval")
		}
	})

	// Cache validation
	v.When(api.CacheEnabled, func(v *validation.Validator) {
		v.Duration("deeplake_api.cache_ttl", api.CacheTTL.String(), "").
			Min("deeplake_api.cache_max_size", api.CacheMaxSize, 1, "").
			Range("deeplake_api.cache_ttl", int(api.CacheTTL.Seconds()), 1, 3600, "cache TTL should be between 1s and 1h").
			Range("deeplake_api.cache_max_size", api.CacheMaxSize, 1, 100000, "cache max size should be between 1 and 100000")
	})

	// Rate limiting validation
	v.When(api.RateLimitEnabled, func(v *validation.Validator) {
		v.Min("deeplake_api.rate_limit_rps", int(api.RateLimitRPS), 1, "rate limit RPS must be positive").
			Min("deeplake_api.rate_limit_burst", api.RateLimitBurst, 1, "rate limit burst must be positive").
			Range("deeplake_api.rate_limit_rps", int(api.RateLimitRPS), 1, 10000, "rate limit RPS should be between 1 and 10000").
			Range("deeplake_api.rate_limit_burst", api.RateLimitBurst, 1, 50000, "rate limit burst should be between 1 and 50000")

		// Burst should be >= RPS for proper rate limiting
		if api.RateLimitBurst < int(api.RateLimitRPS) {
			v.AddError("deeplake_api.rate_limit_burst", fmt.Sprintf("%d", api.RateLimitBurst),
				"rate limit burst should be >= rate limit RPS", "burst_too_low")
		}
	})

	// Production-specific validation
	if c.IsProduction() {
		// Production should have API key
		if api.APIKey == "" {
			v.AddError("deeplake_api.api_key", "",
				"DeepLake API key should be set in production", "production_security")
		}

		// Production should use HTTPS
		if api.BaseURL != "" && !strings.HasPrefix(api.BaseURL, "https://") {
			v.AddError("deeplake_api.base_url", api.BaseURL,
				"DeepLake API should use HTTPS in production", "production_security")
		}

		// Production should not skip TLS verification
		if api.TLSInsecureSkipVerify {
			v.AddError("deeplake_api.tls_insecure_skip_verify", "true",
				"TLS verification should not be skipped in production", "production_security")
		}
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

// LoadFromFile loads configuration from a file
func (c *Config) LoadFromFile(configPath string) error {
	loader := config.NewLoader("")
	return loader.LoadFromFile(configPath, c)
}

// LoadFromEnv loads configuration from environment variables
func (c *Config) LoadFromEnv() error {
	loader := config.NewLoader("")
	return loader.LoadFromEnv(c)
}

// Load loads configuration from file and environment variables
func (c *Config) Load(configPath string) error {
	loader := config.NewLoader("")
	return loader.Load(configPath, c)
}

// WriteExample writes an example configuration file
func (c *Config) WriteExample(configPath string) error {
	loader := config.NewLoader("")
	return loader.WriteExample(configPath, c)
}
