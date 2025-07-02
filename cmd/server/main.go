package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/internal/server"
)

func main() {
	// Parse command line flags
	var (
		configFile = flag.String("config", "", "Path to configuration file")
		host       = flag.String("host", "0.0.0.0", "Server host")
		port       = flag.Int("port", 8080, "Server port")
		dbHost     = flag.String("db-host", "localhost", "Database host")
		dbPort     = flag.Int("db-port", 5432, "Database port")
		dbUsername = flag.String("db-username", "postgres", "Database username")
		dbPassword = flag.String("db-password", "", "Database password")
		dbName     = flag.String("db-name", "eaiingest", "Database name")
		dbSSLMode  = flag.String("db-ssl-mode", "disable", "Database SSL mode")
		jwtSecret  = flag.String("jwt-secret", "", "JWT secret for authentication")
		logLevel   = flag.String("log-level", "info", "Log level")
		version    = flag.Bool("version", false, "Show version information")
	)
	flag.Parse()

	if *version {
		fmt.Println("eAIIngest Server v1.0.0")
		os.Exit(0)
	}

	// Override with environment variables if available
	if envHost := os.Getenv("SERVER_HOST"); envHost != "" {
		*host = envHost
	}
	if envPort := os.Getenv("SERVER_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			*port = p
		}
	}
	if envDBHost := os.Getenv("DB_HOST"); envDBHost != "" {
		*dbHost = envDBHost
	}
	if envDBPort := os.Getenv("DB_PORT"); envDBPort != "" {
		if p, err := strconv.Atoi(envDBPort); err == nil {
			*dbPort = p
		}
	}
	if envDBUsername := os.Getenv("DB_USERNAME"); envDBUsername != "" {
		*dbUsername = envDBUsername
	}
	if envDBPassword := os.Getenv("DB_PASSWORD"); envDBPassword != "" {
		*dbPassword = envDBPassword
	}
	if envDBName := os.Getenv("DB_DATABASE"); envDBName != "" {
		*dbName = envDBName
	}
	if envJWTSecret := os.Getenv("JWT_SECRET"); envJWTSecret != "" {
		*jwtSecret = envJWTSecret
	}

	// Create database configuration
	dbConfig := &database.Config{
		Host:            *dbHost,
		Port:            *dbPort,
		Username:        *dbUsername,
		Password:        *dbPassword,
		Database:        *dbName,
		SSLMode:         *dbSSLMode,
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 30 * time.Minute,
		LogLevel:        "warn",
		SlowThreshold:   200 * time.Millisecond,
		AutoMigrate:     false, // Don't auto-migrate in production
	}

	// Create server configuration
	serverConfig := &server.Config{
		Host:            *host,
		Port:            *port,
		TLSEnabled:      false, // TODO: Add TLS support
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		IdleTimeout:     120 * time.Second,
		ShutdownTimeout: 30 * time.Second,
		CORSEnabled:     true,
		CORSAllowedOrigins: []string{"*"}, // TODO: Configure for production
		CORSAllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD"},
		CORSAllowedHeaders: []string{"*"},
		RateLimitEnabled: true,
		RateLimitRPS:     100,
		RateLimitBurst:   200,
		AuthEnabled:      true,
		JWTSecret:        *jwtSecret,
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
		Database:         dbConfig,
	}

	// Validate JWT secret for production
	if serverConfig.AuthEnabled && serverConfig.JWTSecret == "" {
		log.Fatal("JWT secret is required when authentication is enabled. Set JWT_SECRET environment variable or use --jwt-secret flag.")
	}

	// Initialize database
	fmt.Println("Connecting to database...")
	db, err := database.New(dbConfig)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Connect and run health checks
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Check if we need to run migrations
	migrator := db.Migrator()
	pendingMigrations, err := migrator.GetPendingMigrations(ctx)
	if err != nil {
		log.Fatalf("Failed to check migrations: %v", err)
	}

	if len(pendingMigrations) > 0 {
		fmt.Printf("Found %d pending migrations. Please run migrations first:\n", len(pendingMigrations))
		fmt.Println("  go run cmd/migrate/main.go -command migrate")
		fmt.Println("Or set DB_AUTO_MIGRATE=true to run migrations automatically.")

		if os.Getenv("DB_AUTO_MIGRATE") == "true" {
			fmt.Println("Running migrations automatically...")
			if err := migrator.Migrate(ctx); err != nil {
				log.Fatalf("Failed to run migrations: %v", err)
			}
			fmt.Println("Migrations completed successfully.")
		} else {
			os.Exit(1)
		}
	}

	// Initialize server
	fmt.Printf("Initializing server on %s:%d...\n", *host, *port)
	srv, err := server.New(serverConfig, db)
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	// Print startup information
	fmt.Println("=================================")
	fmt.Println("eAIIngest Server")
	fmt.Println("=================================")
	fmt.Printf("Server URL: http://%s:%d\n", *host, *port)
	fmt.Printf("API Prefix: %s\n", serverConfig.APIPrefix)
	fmt.Printf("Health Check: http://%s:%d%s\n", *host, *port, serverConfig.HealthCheckPath)
	if serverConfig.MetricsEnabled {
		fmt.Printf("Metrics: http://%s:%d%s\n", *host, *port, serverConfig.MetricsPath)
	}
	fmt.Printf("Documentation: http://%s:%d%s/docs\n", *host, *port, serverConfig.APIPrefix)
	fmt.Printf("Authentication: %v\n", serverConfig.AuthEnabled)
	fmt.Printf("Rate Limiting: %v (%d RPS)\n", serverConfig.RateLimitEnabled, serverConfig.RateLimitRPS)
	fmt.Printf("CORS: %v\n", serverConfig.CORSEnabled)
	fmt.Printf("Log Level: %s\n", *logLevel)
	fmt.Println("=================================")

	// Start server
	fmt.Println("Starting server...")
	if err := srv.Start(context.Background()); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// TODO: Add configuration file support
// TODO: Add TLS/HTTPS support
// TODO: Add structured logging
// TODO: Add metrics collection
// TODO: Add graceful shutdown handling
// TODO: Add health check dependencies
// TODO: Add configuration validation