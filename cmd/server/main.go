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
	"github.com/jscharber/eAIIngest/pkg/config"
	"github.com/jscharber/eAIIngest/pkg/logger"
)

func main() {
	// Parse command line flags
	var (
		configFile       = flag.String("config", "", "Path to configuration file")
		generateConfig   = flag.String("generate-config", "", "Generate example configuration file at specified path")
		validateConfig   = flag.Bool("validate-config", false, "Validate configuration and exit")
		host            = flag.String("host", "0.0.0.0", "Server host")
		port            = flag.Int("port", 8080, "Server port")
		tlsEnabled      = flag.Bool("tls", false, "Enable TLS/HTTPS")
		tlsCertFile     = flag.String("tls-cert", "", "TLS certificate file")
		tlsKeyFile      = flag.String("tls-key", "", "TLS private key file")
		dbHost          = flag.String("db-host", "localhost", "Database host")
		dbPort          = flag.Int("db-port", 5432, "Database port")
		dbUsername      = flag.String("db-username", "postgres", "Database username")
		dbPassword      = flag.String("db-password", "", "Database password")
		dbName          = flag.String("db-name", "audimodal", "Database name")
		dbSSLMode       = flag.String("db-ssl-mode", "disable", "Database SSL mode")
		jwtSecret       = flag.String("jwt-secret", "", "JWT secret for authentication")
		logLevel        = flag.String("log-level", "info", "Log level")
		version         = flag.Bool("version", false, "Show version information")
	)
	flag.Parse()

	if *version {
		fmt.Println("eAIIngest Server v1.0.0")
		os.Exit(0)
	}

	// Handle config file generation
	if *generateConfig != "" {
		if err := config.ValidateConfigPath(*generateConfig); err != nil {
			log.Fatalf("Invalid config path: %v", err)
		}

		// Create default configuration
		defaultConfig := server.GetDefaultConfig()
		defaultConfig.Database = database.GetDefaultConfig()

		// Write example configuration
		if err := defaultConfig.WriteExample(*generateConfig); err != nil {
			log.Fatalf("Failed to generate config file: %v", err)
		}

		fmt.Printf("Example configuration file generated at: %s\n", *generateConfig)
		fmt.Println("Edit the file to customize your configuration.")
		os.Exit(0)
	}

	// Validate config file path if provided
	if *configFile != "" {
		if err := config.ValidateConfigPath(*configFile); err != nil {
			log.Fatalf("Invalid config file: %v", err)
		}
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
	if envTLSEnabled := os.Getenv("TLS_ENABLED"); envTLSEnabled == "true" {
		*tlsEnabled = true
	}
	if envTLSCert := os.Getenv("TLS_CERT_FILE"); envTLSCert != "" {
		*tlsCertFile = envTLSCert
	}
	if envTLSKey := os.Getenv("TLS_KEY_FILE"); envTLSKey != "" {
		*tlsKeyFile = envTLSKey
	}

	// Create server configuration - start with defaults
	serverConfig := server.GetDefaultConfig()
	serverConfig.Database = database.GetDefaultConfig()

	// Load configuration from file if specified
	if *configFile != "" {
		if err := serverConfig.Load(*configFile); err != nil {
			log.Fatalf("Failed to load configuration from file: %v", err)
		}
	} else {
		// Load from environment variables only if no config file
		if err := serverConfig.LoadFromEnv(); err != nil {
			log.Fatalf("Failed to load configuration from environment: %v", err)
		}
	}

	// Override with command line flags (highest priority)
	if *host != "0.0.0.0" {
		serverConfig.Host = *host
	}
	if *port != 8080 {
		serverConfig.Port = *port
	}
	if *tlsEnabled {
		serverConfig.TLSEnabled = *tlsEnabled
	}
	if *tlsCertFile != "" {
		serverConfig.TLSCertFile = *tlsCertFile
	}
	if *tlsKeyFile != "" {
		serverConfig.TLSKeyFile = *tlsKeyFile
	}
	if *jwtSecret != "" {
		serverConfig.JWTSecret = *jwtSecret
	}
	if *logLevel != "info" {
		serverConfig.LogLevel = *logLevel
	}

	// Override database config with command line flags
	if *dbHost != "localhost" {
		serverConfig.Database.Host = *dbHost
	}
	if *dbPort != 5432 {
		serverConfig.Database.Port = *dbPort
	}
	if *dbUsername != "postgres" {
		serverConfig.Database.Username = *dbUsername
	}
	if *dbPassword != "" {
		serverConfig.Database.Password = *dbPassword
	}
	if *dbName != "audimodal" {
		serverConfig.Database.Database = *dbName
	}
	if *dbSSLMode != "disable" {
		serverConfig.Database.SSLMode = *dbSSLMode
	}

	// Validate configuration if requested
	if *validateConfig {
		if err := serverConfig.Validate(); err != nil {
			fmt.Printf("Configuration validation failed:\n%v\n", err)
			os.Exit(1)
		}
		fmt.Println("Configuration validation passed successfully.")
		os.Exit(0)
	}

	// Initialize structured logging early
	parsedLogLevel := logger.ParseLogLevel(serverConfig.LogLevel)
	logFormat := logger.JSONFormat
	if serverConfig.LogFormat == "text" {
		logFormat = logger.TextFormat
	}

	appLogger := logger.NewLogger(&logger.Config{
		Level:        parsedLogLevel,
		Format:       logFormat,
		Output:       os.Stdout,
		Service:      "audimodal",
		Version:      "1.0.0",
		EnableCaller: serverConfig.IsDevelopment(),
		Fields:       make(map[string]interface{}),
	})

	// Set as default logger
	logger.SetDefault(appLogger)

	// Validate JWT secret for production
	if serverConfig.AuthEnabled && serverConfig.JWTSecret == "" {
		appLogger.Fatal("JWT secret is required when authentication is enabled. Set JWT_SECRET environment variable or use --jwt-secret flag.")
	}

	// Initialize database
	appLogger.Info("Connecting to database", "host", serverConfig.Database.Host, "port", serverConfig.Database.Port, "database", serverConfig.Database.Database)
	db, err := database.New(serverConfig.Database)
	if err != nil {
		appLogger.Fatal("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Connect and run health checks
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.Connect(ctx); err != nil {
		appLogger.Fatal("Failed to connect to database: %v", err)
	}

	// Check if we need to run migrations
	migrator := db.Migrator()
	pendingMigrations, err := migrator.GetPendingMigrations(ctx)
	if err != nil {
		appLogger.Fatal("Failed to check migrations: %v", err)
	}

	if len(pendingMigrations) > 0 {
		appLogger.WithField("pending_count", len(pendingMigrations)).Warn("Found pending migrations")
		appLogger.Info("Please run migrations first: go run cmd/migrate/main.go -command migrate")
		appLogger.Info("Or set DB_AUTO_MIGRATE=true to run migrations automatically")

		if os.Getenv("DB_AUTO_MIGRATE") == "true" {
			appLogger.Info("Running migrations automatically")
			if err := migrator.Migrate(ctx); err != nil {
				appLogger.Fatal("Failed to run migrations: %v", err)
			}
			appLogger.Info("Migrations completed successfully")
		} else {
			os.Exit(1)
		}
	}

	// Initialize server
	appLogger.WithFields(map[string]interface{}{
		"host": serverConfig.Host,
		"port": serverConfig.Port,
		"tls_enabled": serverConfig.TLSEnabled,
	}).Info("Initializing server")
	srv, err := server.New(serverConfig, db)
	if err != nil {
		appLogger.Fatal("Failed to initialize server: %v", err)
	}

	// Log startup information
	protocol := "http"
	if serverConfig.TLSEnabled {
		protocol = "https"
	}

	startupInfo := map[string]interface{}{
		"server_url":       fmt.Sprintf("%s://%s:%d", protocol, serverConfig.Host, serverConfig.Port),
		"api_prefix":       serverConfig.APIPrefix,
		"health_check":     fmt.Sprintf("%s://%s:%d%s", protocol, serverConfig.Host, serverConfig.Port, serverConfig.HealthCheckPath),
		"tls_enabled":      serverConfig.TLSEnabled,
		"authentication":   serverConfig.AuthEnabled,
		"rate_limiting":    serverConfig.RateLimitEnabled,
		"rate_limit_rps":   serverConfig.RateLimitRPS,
		"cors_enabled":     serverConfig.CORSEnabled,
		"log_level":        serverConfig.LogLevel,
		"log_format":       serverConfig.LogFormat,
		"shutdown_timeout": serverConfig.ShutdownTimeout.String(),
	}

	if serverConfig.MetricsEnabled {
		startupInfo["metrics"] = fmt.Sprintf("%s://%s:%d%s", protocol, serverConfig.Host, serverConfig.Port, serverConfig.MetricsPath)
	}

	if serverConfig.TLSEnabled {
		startupInfo["tls_cert_file"] = serverConfig.TLSCertFile
		startupInfo["tls_key_file"] = serverConfig.TLSKeyFile
	}

	appLogger.WithFields(startupInfo).Info("eAIIngest Server Configuration")

	// Start server
	appLogger.Info("Starting eAIIngest server")
	if err := srv.Start(context.Background()); err != nil {
		appLogger.Fatal("Server failed: %v", err)
	}
}

// COMPLETED: Add configuration file support
// COMPLETED: Add TLS/HTTPS support
// COMPLETED: Add structured logging
// COMPLETED: Add metrics collection
// COMPLETED: Add graceful shutdown handling
// COMPLETED: Add health check dependencies
// COMPLETED: Add configuration validation