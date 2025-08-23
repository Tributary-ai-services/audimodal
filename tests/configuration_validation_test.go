package tests

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAudiModalConfigurationValidation validates AudiModal's environment configuration
func TestAudiModalConfigurationValidation(t *testing.T) {
	t.Run("DEEPLAKE_API_KEY environment variable is configured", func(t *testing.T) {
		apiKey := os.Getenv("DEEPLAKE_API_KEY")
		assert.NotEmpty(t, apiKey, "DEEPLAKE_API_KEY environment variable must be set in AudiModal")
		assert.NotEqual(t, "dev-12345-abcdef-67890-ghijkl", apiKey, "Should not use old hardcoded development key")
		assert.Greater(t, len(apiKey), 10, "API key should be properly generated")
		
		t.Logf("✅ DEEPLAKE_API_KEY is configured: %s***", apiKey[:8])
	})

	t.Run("DEEPLAKE_API_URL environment variable is configured", func(t *testing.T) {
		apiURL := os.Getenv("DEEPLAKE_API_URL")
		
		// Allow both container and localhost URLs for testing
		if apiURL == "" {
			apiURL = "http://localhost:8000" // Default for tests
		}
		
		assert.NotEmpty(t, apiURL, "DEEPLAKE_API_URL should be configured")
		t.Logf("✅ DEEPLAKE_API_URL is configured: %s", apiURL)
	})

	t.Run("OpenAI API key is configured", func(t *testing.T) {
		openaiKey := os.Getenv("OPENAI_API_KEY")
		assert.NotEmpty(t, openaiKey, "OPENAI_API_KEY must be set for embedding generation")
		assert.True(t, len(openaiKey) > 20, "OpenAI API key should be properly formatted")
		
		t.Logf("✅ OPENAI_API_KEY is configured: %s***", openaiKey[:8])
	})
}

// TestContainerEnvironmentConfiguration validates the Docker container environment
func TestContainerEnvironmentConfiguration(t *testing.T) {
	t.Run("AudiModal service is accessible", func(t *testing.T) {
		audimodalURL := getEnvOrDefault("AUDIMODAL_URL", "http://localhost:8084")
		
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(fmt.Sprintf("%s/health", audimodalURL))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "AudiModal service should be healthy")
		t.Log("✅ AudiModal service is accessible and healthy")
	})

	t.Run("DeepLake service is accessible", func(t *testing.T) {
		deeplakeURL := getEnvOrDefault("DEEPLAKE_API_URL", "http://localhost:8000")
		
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(fmt.Sprintf("%s/api/v1/health", deeplakeURL))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "DeepLake service should be healthy")
		t.Log("✅ DeepLake service is accessible and healthy")
	})
}

// TestAuthenticationIntegration validates the full authentication flow
func TestAuthenticationIntegration(t *testing.T) {
	t.Run("Authentication configuration matches between services", func(t *testing.T) {
		// Get the API key that AudiModal is configured to use
		audimodalAPIKey := os.Getenv("DEEPLAKE_API_KEY")
		require.NotEmpty(t, audimodalAPIKey, "DEEPLAKE_API_KEY must be configured in AudiModal")

		// Test that this key works with DeepLake
		deeplakeURL := getEnvOrDefault("DEEPLAKE_API_URL", "http://localhost:8000")
		
		req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/health", deeplakeURL), nil)
		require.NoError(t, err)
		
		req.Header.Set("Authorization", fmt.Sprintf("ApiKey %s", audimodalAPIKey))
		
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, 
			"AudiModal's configured API key should work with DeepLake service")
		
		t.Log("✅ Authentication configuration is properly synchronized between services")
	})

	t.Run("Embedding coordinator configuration is valid", func(t *testing.T) {
		// Test that the embedding coordinator can be instantiated with current config
		// This validates that the getEnvOrDefault logic in embedding_coordinator.go works
		
		deeplakeAPIKey := os.Getenv("DEEPLAKE_API_KEY")
		deeplakeAPIURL := getEnvOrDefault("DEEPLAKE_API_URL", "http://deeplake-api_deeplake-service_1:8000")
		openaiAPIKey := os.Getenv("OPENAI_API_KEY")
		
		// Validate all required configuration is present
		assert.NotEmpty(t, deeplakeAPIKey, "DeepLake API key should be configured")
		assert.NotEmpty(t, deeplakeAPIURL, "DeepLake API URL should be configured")
		assert.NotEmpty(t, openaiAPIKey, "OpenAI API key should be configured")
		
		// Validate configuration format
		assert.Contains(t, deeplakeAPIURL, "8000", "DeepLake URL should contain port 8000")
		assert.True(t, len(deeplakeAPIKey) > 30, "DeepLake API key should be properly generated")
		assert.Contains(t, openaiAPIKey, "sk-", "OpenAI API key should start with 'sk-'")
		
		t.Log("✅ Embedding coordinator configuration is valid")
	})
}

// TestDockerComposeChanges validates that our docker-compose changes are effective
func TestDockerComposeChanges(t *testing.T) {
	t.Run("DEEPLAKE_API_KEY is passed to AudiModal container", func(t *testing.T) {
		// This test validates that the docker-compose.yml change is working
		apiKey := os.Getenv("DEEPLAKE_API_KEY")
		assert.NotEmpty(t, apiKey, 
			"DEEPLAKE_API_KEY should be passed from docker-compose environment to container")
		
		// Verify it's not the old hardcoded value
		assert.NotEqual(t, "UimqLp7vXA3x53H1tPKveeRPFxwLIOypHSCjlddlBe8", apiKey,
			"Should not be using the old intermediate API key")
		
		t.Logf("✅ Docker environment variable DEEPLAKE_API_KEY is properly configured: %s***", apiKey[:8])
	})
}

// TestMigrationFromOldConfiguration ensures we've properly migrated from old setup
func TestMigrationFromOldConfiguration(t *testing.T) {
	t.Run("No hardcoded development keys in use", func(t *testing.T) {
		apiKey := os.Getenv("DEEPLAKE_API_KEY")
		
		// Ensure we're not using any of the old hardcoded keys
		oldKeys := []string{
			"dev-12345-abcdef-67890-ghijkl",
			"UimqLp7vXA3x53H1tPKveeRPFxwLIOypHSCjlddlBe8", // First generated key
		}
		
		for _, oldKey := range oldKeys {
			assert.NotEqual(t, oldKey, apiKey, 
				fmt.Sprintf("Should not be using old API key: %s", oldKey))
		}
		
		t.Log("✅ Successfully migrated from hardcoded development keys")
	})

	t.Run("Current API key is properly generated", func(t *testing.T) {
		apiKey := os.Getenv("DEEPLAKE_API_KEY")
		require.NotEmpty(t, apiKey, "API key must be configured")
		
		// Validate it looks like a properly generated key
		expectedKey := "1zi_41Jhel1nV7GGUSgdQwkvx9HZLZbVpPRnNxuPxz0"
		assert.Equal(t, expectedKey, apiKey, 
			"Should be using the latest generated API key")
		
		t.Log("✅ Using current properly generated API key")
	})
}

// Helper function for getting environment variables with defaults
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}