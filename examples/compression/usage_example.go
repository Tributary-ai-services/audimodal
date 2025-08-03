package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/jscharber/eAIIngest/pkg/storage"
	"github.com/jscharber/eAIIngest/pkg/storage/cache"
	"github.com/jscharber/eAIIngest/pkg/storage/compression"
)

func main() {
	// Example usage of the document compression system
	ctx := context.Background()

	// 1. Create compression configuration
	compressionConfig := compression.DefaultDocumentCompressionConfig()
	compressionConfig.DefaultStrategy = compression.StrategyAdaptive
	compressionConfig.MinFileSizeThreshold = 1024              // 1KB
	compressionConfig.MaxFileSizeThreshold = 100 * 1024 * 1024 // 100MB

	// 2. Create document compressor
	documentCompressor := compression.NewDocumentCompressor(compressionConfig)

	// 3. Create policy manager
	policyManager := compression.NewPolicyManager(documentCompressor)

	// 4. Create and register compression policies
	if err := setupCompressionPolicies(ctx, policyManager); err != nil {
		log.Fatalf("Failed to setup compression policies: %v", err)
	}

	// 5. Create cache for compressed data
	cacheConfig := cache.DefaultMemoryCacheConfig()
	cacheConfig.MaxSize = 512 * 1024 * 1024 // 512MB
	memoryCache, err := cache.NewMemoryCache(cacheConfig)
	if err != nil {
		log.Fatalf("Failed to create cache: %v", err)
	}
	defer memoryCache.Close()

	// 6. Example: Compress different types of documents
	exampleFiles := []struct {
		name     string
		content  string
		mimeType string
	}{
		{
			name:     "document.txt",
			content:  generateTextContent(5000), // 5KB of text
			mimeType: "text/plain",
		},
		{
			name:     "data.json",
			content:  generateJSONContent(10000), // 10KB of JSON
			mimeType: "application/json",
		},
		{
			name:     "large_log.log",
			content:  generateLogContent(50000), // 50KB of logs
			mimeType: "text/plain",
		},
		{
			name:     "small_config.xml",
			content:  generateXMLContent(800), // 800 bytes of XML
			mimeType: "application/xml",
		},
	}

	tenantID := uuid.New()

	for _, file := range exampleFiles {
		fmt.Printf("\n=== Processing file: %s ===\n", file.name)

		// Create file info
		fileInfo := &storage.FileInfo{
			URL:      fmt.Sprintf("file://%s", file.name),
			Name:     file.name,
			Size:     int64(len(file.content)),
			MimeType: file.mimeType,
			Metadata: make(map[string]string),
		}

		// Evaluate compression policy
		action, err := policyManager.EvaluateFile(ctx, fileInfo, tenantID)
		if err != nil {
			fmt.Printf("No compression policy matched: %v\n", err)
			continue
		}

		fmt.Printf("Compression strategy: %s\n", action.Strategy)
		fmt.Printf("Compression level: %d\n", action.CompressionLevel)
		fmt.Printf("Store original: %t\n", action.StoreOriginal)
		fmt.Printf("Cache result: %t\n", action.CacheResult)

		// Apply compression
		if action.Strategy != compression.StrategyNone {
			result, err := documentCompressor.CompressDocument(ctx, []byte(file.content), file.name)
			if err != nil {
				fmt.Printf("Compression failed: %v\n", err)
				continue
			}

			fmt.Printf("Original size: %d bytes\n", result.OriginalSize)
			fmt.Printf("Compressed size: %d bytes\n", result.CompressedSize)
			fmt.Printf("Compression ratio: %.2f\n", result.CompressionRatio)
			fmt.Printf("Compression time: %v\n", result.CompressionTime)
			fmt.Printf("Algorithm: %s\n", result.Algorithm)

			// Cache compressed data if configured
			if action.CacheResult {
				cacheKey := fmt.Sprintf("compressed:%s", file.name)
				if err := memoryCache.SetContent(ctx, fileInfo.URL, cacheKey, result.Data); err != nil {
					fmt.Printf("Failed to cache compressed data: %v\n", err)
				} else {
					fmt.Printf("Cached compressed data\n")
				}
			}

			// Demonstrate decompression
			metadata := &compression.CompressionMetadata{
				Strategy:         result.Strategy,
				Algorithm:        result.Algorithm,
				OriginalSize:     int64(result.OriginalSize),
				CompressedSize:   int64(result.CompressedSize),
				CompressionRatio: result.CompressionRatio,
				OriginalFilename: file.name,
			}

			decompressed, err := documentCompressor.DecompressDocument(ctx, result.Data, metadata)
			if err != nil {
				fmt.Printf("Decompression failed: %v\n", err)
				continue
			}

			// Verify data integrity
			if string(decompressed) == file.content {
				fmt.Printf("✓ Data integrity verified\n")
			} else {
				fmt.Printf("✗ Data integrity check failed\n")
			}
		} else {
			fmt.Printf("No compression applied\n")
		}
	}

	// 7. Demonstrate batch compression
	fmt.Printf("\n=== Batch Compression Example ===\n")

	batchCompressor := compression.NewBatchCompressor(documentCompressor, 2)
	batchCompressor.Start(ctx)

	// Create a batch job
	var batchFiles []storage.FileInfo
	for _, file := range exampleFiles {
		batchFiles = append(batchFiles, storage.FileInfo{
			Name: file.name,
			Size: int64(len(file.content)),
		})
	}

	job := &compression.BatchCompressionJob{
		Files:     batchFiles,
		Strategy:  compression.StrategyAdaptive,
		Priority:  1,
		CreatedAt: time.Now(),
		Status:    "pending",
	}

	if err := batchCompressor.SubmitJob(job); err != nil {
		fmt.Printf("Failed to submit batch job: %v\n", err)
	} else {
		fmt.Printf("Batch compression job submitted\n")
	}

	// 8. Display cache statistics
	fmt.Printf("\n=== Cache Statistics ===\n")
	stats, err := memoryCache.GetStats(ctx)
	if err != nil {
		fmt.Printf("Failed to get cache stats: %v\n", err)
	} else {
		fmt.Printf("Cache provider: %s\n", stats.Provider)
		fmt.Printf("Cached items: %d\n", stats.Keys)
		fmt.Printf("Memory used: %d bytes\n", stats.MemoryUsed)
		fmt.Printf("Memory limit: %d bytes\n", stats.MemoryLimit)
		fmt.Printf("Operations: %d\n", stats.Operations)
		fmt.Printf("Errors: %d\n", stats.Errors)
		fmt.Printf("Average latency: %v\n", stats.AverageLatency)
	}
}

func setupCompressionPolicies(ctx context.Context, manager *compression.PolicyManager) error {
	tenantID := uuid.New()

	// Policy 1: Text files - high compression
	textPolicy := &compression.CompressionPolicy{
		ID:          uuid.New(),
		Name:        "Text Files Policy",
		Description: "High compression for text files",
		TenantID:    tenantID,
		Enabled:     true,
		Priority:    100,
		Rules: []compression.CompressionRule{
			{
				ID:       uuid.New(),
				Name:     "Text files > 1KB",
				Enabled:  true,
				Priority: 1,
				Conditions: []compression.CompressionCondition{
					{
						Type:     compression.ConditionFileExtension,
						Operator: compression.OperatorIn,
						Value:    []string{".txt", ".log", ".json", ".xml"},
					},
					{
						Type:     compression.ConditionFileSize,
						Operator: compression.OperatorGreaterThan,
						Value:    float64(1024), // 1KB
					},
				},
				Action: compression.CompressionAction{
					Strategy:         compression.StrategyText,
					CompressionLevel: 9,
					StoreOriginal:    false,
					CacheResult:      true,
					CacheTTL:         24 * time.Hour,
					AsyncCompression: false,
				},
			},
		},
		DefaultRule: &compression.CompressionRule{
			ID:      uuid.New(),
			Name:    "Default",
			Enabled: true,
			Action: compression.CompressionAction{
				Strategy:         compression.StrategyNone,
				CompressionLevel: 0,
				StoreOriginal:    true,
				CacheResult:      false,
			},
		},
		Settings: compression.CompressionPolicySettings{
			EnableMetrics:   true,
			EnableAuditLog:  true,
			MaxConcurrency:  4,
			DefaultCacheTTL: time.Hour,
			ErrorHandling:   "skip",
		},
	}

	if err := manager.CreatePolicy(ctx, textPolicy); err != nil {
		return fmt.Errorf("failed to create text policy: %w", err)
	}

	// Policy 2: Size-based policy
	sizePolicy := &compression.CompressionPolicy{
		ID:          uuid.New(),
		Name:        "Size-Based Policy",
		Description: "Compression based on file size",
		TenantID:    tenantID,
		Enabled:     true,
		Priority:    90,
		Rules: []compression.CompressionRule{
			{
				ID:       uuid.New(),
				Name:     "Large files",
				Enabled:  true,
				Priority: 1,
				Conditions: []compression.CompressionCondition{
					{
						Type:     compression.ConditionFileSize,
						Operator: compression.OperatorGreaterThan,
						Value:    float64(10 * 1024), // 10KB
					},
				},
				Action: compression.CompressionAction{
					Strategy:         compression.StrategyAdaptive,
					CompressionLevel: 7,
					StoreOriginal:    false,
					CacheResult:      true,
					CacheTTL:         2 * time.Hour,
					AsyncCompression: true,
				},
			},
		},
		DefaultRule: &compression.CompressionRule{
			ID:      uuid.New(),
			Name:    "Default",
			Enabled: true,
			Action: compression.CompressionAction{
				Strategy:         compression.StrategyAdaptive,
				CompressionLevel: 6,
				StoreOriginal:    false,
				CacheResult:      true,
				CacheTTL:         time.Hour,
			},
		},
		Settings: compression.CompressionPolicySettings{
			EnableMetrics:   true,
			MaxConcurrency:  2,
			DefaultCacheTTL: time.Hour,
			ErrorHandling:   "retry",
		},
	}

	if err := manager.CreatePolicy(ctx, sizePolicy); err != nil {
		return fmt.Errorf("failed to create size policy: %w", err)
	}

	return nil
}

func generateTextContent(size int) string {
	content := "This is sample text content for compression testing. "
	result := ""
	for len(result) < size {
		result += content
	}
	return result[:size]
}

func generateJSONContent(size int) string {
	baseJSON := `{"name": "sample", "value": 123, "active": true, "description": "This is a sample JSON document for compression testing"}`
	result := "["
	for len(result) < size-1 {
		if len(result) > 1 {
			result += ", "
		}
		result += baseJSON
	}
	result += "]"
	return result
}

func generateLogContent(size int) string {
	logLine := "2024-01-01 12:00:00 INFO [main] Sample log message for compression testing\n"
	result := ""
	for len(result) < size {
		result += logLine
	}
	return result[:size]
}

func generateXMLContent(size int) string {
	xml := `<?xml version="1.0"?>
<data>
  <item>
    <name>Sample</name>
    <value>123</value>
    <description>XML content for testing</description>
  </item>
</data>`
	result := ""
	for len(result) < size {
		result += xml
	}
	return result[:size]
}
