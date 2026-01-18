// Package main provides a data migration script to backfill neo4j_document_id
// for existing files in the AudiModal database.
//
// This script resolves GitHub Issue #7 by extracting Neo4j Document IDs from
// MinIO storage paths and updating the corresponding file records.
//
// MinIO path pattern: .../documents/{neo4j_document_id}/filename
//
// Usage:
//   go run scripts/backfill_neo4j_document_ids.go --dry-run  # Preview changes
//   go run scripts/backfill_neo4j_document_ids.go            # Apply changes
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// File represents a minimal file record for migration
type File struct {
	ID              uuid.UUID `gorm:"type:uuid;primary_key"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null"`
	URL             string    `gorm:"not null"`
	Path            string    `gorm:"not null"`
	Neo4jDocumentID *string   `gorm:"type:varchar(36)"`
	UpdatedAt       time.Time
}

func (File) TableName() string {
	return "files"
}

var (
	dryRun  = flag.Bool("dry-run", false, "Preview changes without applying them")
	verbose = flag.Bool("verbose", false, "Enable verbose output")
	limit   = flag.Int("limit", 0, "Limit number of records to process (0 = no limit)")
)

// Regex pattern to extract Neo4j Document ID from storage path
// Pattern: .../documents/{neo4j_document_id}/... or .../documents/{neo4j_document_id}
var documentIDPattern = regexp.MustCompile(`/documents/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})(?:/|$)`)

func main() {
	flag.Parse()

	fmt.Println("=== Neo4j Document ID Backfill Script ===")
	fmt.Printf("Mode: %s\n", map[bool]string{true: "DRY RUN (no changes)", false: "LIVE (applying changes)"}[*dryRun])
	fmt.Println()

	// Get database connection string from environment
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Fallback to individual env vars
		host := os.Getenv("POSTGRES_HOST")
		if host == "" {
			host = "localhost"
		}
		port := os.Getenv("POSTGRES_PORT")
		if port == "" {
			port = "5432"
		}
		user := os.Getenv("POSTGRES_USER")
		if user == "" {
			user = "tasuser"
		}
		password := os.Getenv("POSTGRES_PASSWORD")
		if password == "" {
			password = "taspassword"
		}
		database := os.Getenv("POSTGRES_DB")
		if database == "" {
			database = "tas_shared"
		}
		dbURL = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			host, port, user, password, database)
	}

	// Connect to database
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		fmt.Printf("ERROR: Failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Get files that need backfilling (neo4j_document_id is NULL)
	var files []File
	query := db.WithContext(ctx).Where("neo4j_document_id IS NULL")
	if *limit > 0 {
		query = query.Limit(*limit)
	}

	if err := query.Find(&files).Error; err != nil {
		fmt.Printf("ERROR: Failed to query files: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d files with NULL neo4j_document_id\n\n", len(files))

	if len(files) == 0 {
		fmt.Println("No files need backfilling.")
		return
	}

	// Process files
	updatedCount := 0
	skippedCount := 0
	errorCount := 0

	for _, file := range files {
		// Try to extract document ID from URL first, then Path
		documentID := extractDocumentID(file.URL)
		if documentID == "" {
			documentID = extractDocumentID(file.Path)
		}

		if documentID == "" {
			if *verbose {
				fmt.Printf("SKIP: File %s - no document ID found in URL (%s) or Path (%s)\n",
					file.ID.String(), file.URL, file.Path)
			}
			skippedCount++
			continue
		}

		// Validate extracted UUID
		if _, err := uuid.Parse(documentID); err != nil {
			if *verbose {
				fmt.Printf("SKIP: File %s - invalid document ID format: %s\n",
					file.ID.String(), documentID)
			}
			skippedCount++
			continue
		}

		if *verbose || *dryRun {
			fmt.Printf("UPDATE: File %s -> neo4j_document_id = %s\n",
				file.ID.String(), documentID)
		}

		if !*dryRun {
			// Apply update
			if err := db.WithContext(ctx).Model(&file).
				Update("neo4j_document_id", documentID).Error; err != nil {
				fmt.Printf("ERROR: Failed to update file %s: %v\n", file.ID.String(), err)
				errorCount++
				continue
			}
		}

		updatedCount++
	}

	// Print summary
	fmt.Println()
	fmt.Println("=== Summary ===")
	fmt.Printf("Total files processed: %d\n", len(files))
	fmt.Printf("Files updated: %d\n", updatedCount)
	fmt.Printf("Files skipped: %d (no document ID in path)\n", skippedCount)
	fmt.Printf("Errors: %d\n", errorCount)

	if *dryRun {
		fmt.Println()
		fmt.Println("This was a DRY RUN. No changes were made.")
		fmt.Println("Run without --dry-run to apply changes.")
	}
}

// extractDocumentID extracts a Neo4j Document ID from a path string
// Expected path pattern: .../documents/{uuid}/...
func extractDocumentID(path string) string {
	matches := documentIDPattern.FindStringSubmatch(path)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}
