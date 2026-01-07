package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jscharber/audimodal/pkg/auth"
)

const (
	// Service account UUIDs for system-to-system communication
	serviceAccountUserID   = "00000000-0000-0000-0000-000000000001"
	serviceAccountTenantID = "00000000-0000-0000-0000-000000000001"
)

func main() {
	// Parse command line flags
	var (
		jwtSecret      = flag.String("jwt-secret", "", "JWT secret for signing tokens (required)")
		expiryYears    = flag.Int("expiry-years", 5, "API key expiry in years (default: 5)")
		scopes         = flag.String("scopes", "data:read,data:write,data:process,api:read,api:write", "Comma-separated list of scopes")
		issuer         = flag.String("issuer", "audimodal", "JWT issuer (default: audimodal)")
		showHelp       = flag.Bool("help", false, "Show help message")
	)

	flag.Parse()

	if *showHelp {
		printHelp()
		os.Exit(0)
	}

	// Validate required flags
	if *jwtSecret == "" {
		fmt.Fprintf(os.Stderr, "Error: --jwt-secret is required\n\n")
		printHelp()
		os.Exit(1)
	}

	// Parse UUIDs
	userID, err := uuid.Parse(serviceAccountUserID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing user ID: %v\n", err)
		os.Exit(1)
	}

	tenantID, err := uuid.Parse(serviceAccountTenantID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing tenant ID: %v\n", err)
		os.Exit(1)
	}

	// Create JWT manager configuration
	// The JWT secret is used to deterministically generate RSA keys
	// We'll use it as the PrivateKeyPEM (if it's in PEM format) or generate keys
	jwtConfig := &auth.JWTConfig{
		Issuer:  *issuer,
		KeySize: 2048,
	}

	// Check if jwtSecret looks like a PEM key
	if len(*jwtSecret) > 100 && (contains(*jwtSecret, "BEGIN") || contains(*jwtSecret, "PRIVATE KEY")) {
		jwtConfig.PrivateKeyPEM = *jwtSecret
	}
	// Otherwise, keys will be auto-generated

	// Create JWT manager
	jwtManager, err := auth.NewJWTManager(jwtConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating JWT manager: %v\n", err)
		os.Exit(1)
	}

	// Parse scopes (simple comma-separated string)
	scopesList := parseScopesString(*scopes)

	// Calculate expiration time
	expiresAt := time.Now().Add(time.Duration(*expiryYears) * 365 * 24 * time.Hour)

	// Generate API key
	apiKey, err := jwtManager.GenerateAPIKey(userID, tenantID, scopesList, &expiresAt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating API key: %v\n", err)
		os.Exit(1)
	}

	// Print the API key
	fmt.Println("=== AudiModal Service Account API Key ===")
	fmt.Println()
	fmt.Printf("User ID:    %s\n", serviceAccountUserID)
	fmt.Printf("Tenant ID:  %s\n", serviceAccountTenantID)
	fmt.Printf("Scopes:     %s\n", *scopes)
	fmt.Printf("Issuer:     %s\n", *issuer)
	fmt.Printf("Expires:    %s (%d years)\n", expiresAt.Format(time.RFC3339), *expiryYears)
	fmt.Println()
	fmt.Println("API Key:")
	fmt.Println(apiKey)
	fmt.Println()
	fmt.Println("=== Usage Instructions ===")
	fmt.Println("1. Store this API key in aether-secrets/apps/aether-be/service-keys.env")
	fmt.Println("2. Add to Kubernetes secrets:")
	fmt.Println("   - aether-secrets/k8s/apps/aether-be-secrets.yaml (tas-secrets namespace)")
	fmt.Println("   - aether-be/k8s/secret.yaml (aether-be namespace)")
	fmt.Println("3. Set as environment variable: AUDIMODAL_API_KEY=<api-key>")
	fmt.Println("4. Restart services to apply changes")
	fmt.Println()
	fmt.Printf("Key length: %d characters\n", len(apiKey))
}

func parseScopesString(scopesStr string) []string {
	if scopesStr == "" {
		return []string{}
	}

	scopes := []string{}
	for i := 0; i < len(scopesStr); {
		// Find next comma or end of string
		end := i
		for end < len(scopesStr) && scopesStr[end] != ',' {
			end++
		}

		// Extract scope and trim whitespace
		scope := scopesStr[i:end]
		// Simple trim
		for len(scope) > 0 && (scope[0] == ' ' || scope[0] == '\t') {
			scope = scope[1:]
		}
		for len(scope) > 0 && (scope[len(scope)-1] == ' ' || scope[len(scope)-1] == '\t') {
			scope = scope[:len(scope)-1]
		}

		if len(scope) > 0 {
			scopes = append(scopes, scope)
		}

		i = end + 1
	}

	return scopes
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func printHelp() {
	fmt.Println("AudiModal API Key Generator")
	fmt.Println()
	fmt.Println("Generate long-lived JWT API keys for service-to-service authentication.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  audimodal-admin [flags]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --jwt-secret string    JWT secret for signing tokens (required)")
	fmt.Println("  --expiry-years int     API key expiry in years (default: 5)")
	fmt.Println("  --scopes string        Comma-separated list of scopes")
	fmt.Println("                         (default: data:read,data:write,data:process,api:read,api:write)")
	fmt.Println("  --issuer string        JWT issuer (default: audimodal)")
	fmt.Println("  --help                 Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Generate API key with default settings (5 years, full scopes)")
	fmt.Println("  audimodal-admin --jwt-secret=your-secret-key")
	fmt.Println()
	fmt.Println("  # Generate API key with 3 years expiry")
	fmt.Println("  audimodal-admin --jwt-secret=your-secret-key --expiry-years=3")
	fmt.Println()
	fmt.Println("  # Generate API key with read-only scopes")
	fmt.Println("  audimodal-admin --jwt-secret=your-secret-key --scopes=data:read,api:read")
	fmt.Println()
	fmt.Println("Service Account IDs:")
	fmt.Println("  User ID:   ", serviceAccountUserID)
	fmt.Println("  Tenant ID: ", serviceAccountTenantID)
}
