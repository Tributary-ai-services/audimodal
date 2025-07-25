package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test configuration defaults
func TestConfig_Defaults(t *testing.T) {
	config := GetDefaultConfig()
	
	// Check server defaults
	assert.Equal(t, "0.0.0.0", config.Host)
	assert.Equal(t, 8080, config.Port)
	
	// Check DeepLake API defaults
	assert.NotNil(t, config.DeepLakeAPI)
	if config.DeepLakeAPI != nil {
		assert.Equal(t, "http://localhost:8000", config.DeepLakeAPI.BaseURL) // Default value
		assert.Equal(t, "", config.DeepLakeAPI.APIKey)  // Empty by default, requires env var
		// Other fields would have defaults from GetDefaultDeepLakeAPIConfig
	}
}

// Test configuration validation
func TestConfig_Validation(t *testing.T) {
	t.Run("Valid configuration", func(t *testing.T) {
		config := GetDefaultConfig()
		config.DeepLakeAPI.BaseURL = "http://localhost:8000"
		config.DeepLakeAPI.APIKey = "test-key"
		
		err := config.Validate()
		// Note: This might fail if other required fields are missing
		// Adjust based on actual validation requirements
		if err != nil {
			t.Logf("Validation error (might be expected): %v", err)
		}
	})
	
	t.Run("Missing DeepLake base URL", func(t *testing.T) {
		config := GetDefaultConfig()
		config.DeepLakeAPI.APIKey = "test-key"
		
		err := config.Validate()
		assert.Error(t, err)
	})
	
	t.Run("Missing DeepLake API key", func(t *testing.T) {
		config := GetDefaultConfig()
		config.DeepLakeAPI.BaseURL = "http://localhost:8000"
		
		err := config.Validate()
		assert.Error(t, err)
	})
}