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
)

func main() {
	var (
		host     = flag.String("host", "localhost", "Database host")
		port     = flag.Int("port", 5432, "Database port")
		username = flag.String("username", "postgres", "Database username")
		password = flag.String("password", "", "Database password")
		dbname   = flag.String("database", "eaiingest", "Database name")
		sslmode  = flag.String("sslmode", "disable", "SSL mode")
		command  = flag.String("command", "migrate", "Command to run: migrate, status, reset, up")
		count    = flag.Int("count", 0, "Number of migrations to run (for 'up' command)")
	)
	flag.Parse()

	// Override with environment variables if available
	if envHost := os.Getenv("DB_HOST"); envHost != "" {
		*host = envHost
	}
	if envPort := os.Getenv("DB_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			*port = p
		}
	}
	if envUsername := os.Getenv("DB_USERNAME"); envUsername != "" {
		*username = envUsername
	}
	if envPassword := os.Getenv("DB_PASSWORD"); envPassword != "" {
		*password = envPassword
	}
	if envDatabase := os.Getenv("DB_DATABASE"); envDatabase != "" {
		*dbname = envDatabase
	}
	if envSSLMode := os.Getenv("DB_SSL_MODE"); envSSLMode != "" {
		*sslmode = envSSLMode
	}

	config := &database.Config{
		Host:     *host,
		Port:     *port,
		Username: *username,
		Password: *password,
		Database: *dbname,
		SSLMode:  *sslmode,
	}

	migrator, err := database.NewMigrator(config)
	if err != nil {
		log.Fatalf("Failed to create migrator: %v", err)
	}
	defer migrator.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch *command {
	case "migrate":
		if err := migrator.Migrate(ctx); err != nil {
			log.Fatalf("Migration failed: %v", err)
		}
		fmt.Println("Migrations completed successfully")

	case "status":
		status, err := migrator.GetMigrationStatus(ctx)
		if err != nil {
			log.Fatalf("Failed to get migration status: %v", err)
		}

		fmt.Println("Migration Status:")
		fmt.Println("=================")
		for _, migration := range status {
			status := "❌ Pending"
			if migration.Applied {
				status = fmt.Sprintf("✅ Applied (%s)", migration.AppliedAt.Format("2006-01-02 15:04:05"))
			}
			fmt.Printf("Version %s: %s\n", migration.Version, status)
		}

	case "up":
		if *count <= 0 {
			log.Fatal("Count must be greater than 0 for 'up' command")
		}
		if err := migrator.MigrateUp(ctx, *count); err != nil {
			log.Fatalf("Migration up failed: %v", err)
		}
		fmt.Printf("Successfully applied %d migrations\n", *count)

	case "reset":
		fmt.Println("WARNING: This will drop all tables and data!")
		fmt.Print("Are you sure? (y/N): ")
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "y" && confirm != "Y" {
			fmt.Println("Operation cancelled")
			return
		}

		if err := migrator.Reset(ctx); err != nil {
			log.Fatalf("Reset failed: %v", err)
		}
		fmt.Println("Database reset completed successfully")

	case "validate":
		if err := migrator.ValidateDatabase(ctx); err != nil {
			log.Fatalf("Database validation failed: %v", err)
		}
		fmt.Println("Database schema is valid")

	default:
		fmt.Printf("Unknown command: %s\n", *command)
		fmt.Println("Available commands: migrate, status, up, reset, validate")
		os.Exit(1)
	}
}