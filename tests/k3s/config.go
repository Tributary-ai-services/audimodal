package k3s

import (
	"fmt"
	"os"
	"time"
)

// Config holds the configuration for K3s integration tests
type Config struct {
	// BaseURL is the AudiModal API URL (e.g., https://audimodal.tas.scharber.com)
	BaseURL string

	// APIKey is the API key for authentication
	APIKey string

	// TenantID is the tenant ID to use for tests
	TenantID string

	// Timeout is the HTTP client timeout
	Timeout time.Duration

	// PollInterval is the interval for polling file processing status
	PollInterval time.Duration

	// MaxPollAttempts is the maximum number of poll attempts before timing out
	MaxPollAttempts int

	// TestDataPath is the path to the test data directory
	TestDataPath string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	return &Config{
		BaseURL:         getEnvOrDefault("AUDIMODAL_URL", "https://audimodal.tas.scharber.com"),
		APIKey:          os.Getenv("AUDIMODAL_API_KEY"),
		TenantID:        getEnvOrDefault("AUDIMODAL_TENANT_ID", "test-tenant"),
		Timeout:         getDurationEnv("AUDIMODAL_TIMEOUT", 30*time.Second),
		PollInterval:    getDurationEnv("AUDIMODAL_POLL_INTERVAL", 2*time.Second),
		MaxPollAttempts: getIntEnv("AUDIMODAL_MAX_POLL_ATTEMPTS", 30),
		TestDataPath:    getEnvOrDefault("TEST_DATA_PATH", "../../../tas-test-data"),
	}
}

// Validate checks that required configuration is present
func (c *Config) Validate() error {
	if c.APIKey == "" {
		return ErrMissingAPIKey
	}
	if c.BaseURL == "" {
		return ErrMissingBaseURL
	}
	return nil
}

// getEnvOrDefault returns the environment variable value or a default
func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// getDurationEnv returns a duration from an environment variable or default
func getDurationEnv(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}

// getIntEnv returns an integer from an environment variable or default
func getIntEnv(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		var i int
		if _, err := fmt.Sscanf(val, "%d", &i); err == nil {
			return i
		}
	}
	return defaultVal
}
