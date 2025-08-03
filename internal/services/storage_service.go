package services

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/internal/database/models"
	"github.com/jscharber/eAIIngest/pkg/storage"
	"github.com/jscharber/eAIIngest/pkg/storage/aws"
	"github.com/jscharber/eAIIngest/pkg/storage/credentials"
	"github.com/jscharber/eAIIngest/pkg/storage/gcp"
	"github.com/jscharber/eAIIngest/pkg/storage/local"
)

// StorageService provides high-level storage operations for the Audimodal.ai platform
type StorageService struct {
	storageManager *storage.StorageManager
	db             *database.Database
}

// NewStorageService creates a new storage service
func NewStorageService(db *database.Database, encryptionKey []byte) *StorageService {
	// Create resolver registry
	registry := storage.NewResolverRegistry()

	// Register storage resolvers
	s3Resolver := aws.NewS3Resolver()
	registry.RegisterResolver(s3Resolver.GetSupportedProviders(), s3Resolver)

	gcsResolver := gcp.NewGCSResolver()
	registry.RegisterResolver(gcsResolver.GetSupportedProviders(), gcsResolver)

	localResolver := local.NewLocalResolver("", nil) // Allow all local paths for now
	registry.RegisterResolver(localResolver.GetSupportedProviders(), localResolver)

	// Create credential provider
	credProvider := credentials.NewDatabaseCredentialProvider(db, encryptionKey)

	// Create storage manager
	storageManager := storage.NewStorageManager(registry, credProvider)

	return &StorageService{
		storageManager: storageManager,
		db:             db,
	}
}

// ValidateDataSourceURL validates that a data source URL is accessible
func (s *StorageService) ValidateDataSourceURL(ctx context.Context, tenantID uuid.UUID, url string) error {
	return s.storageManager.ValidateURLAccess(ctx, url, tenantID)
}

// GetFileInfoFromURL retrieves file information from a cloud storage URL
func (s *StorageService) GetFileInfoFromURL(ctx context.Context, tenantID uuid.UUID, url string) (*storage.FileInfo, error) {
	return s.storageManager.GetFileInfoFromURL(ctx, url, tenantID)
}

// DiscoverFilesFromURL discovers files from a cloud storage URL (directory listing)
func (s *StorageService) DiscoverFilesFromURL(ctx context.Context, tenantID uuid.UUID, url string, options *storage.ListOptions) (*storage.ListResult, error) {
	return s.storageManager.ListFromURL(ctx, url, tenantID, options)
}

// CreateFileFromURL creates a file record from a cloud storage URL
func (s *StorageService) CreateFileFromURL(ctx context.Context, tenantID uuid.UUID, url string, dataSourceID *uuid.UUID, sessionID *uuid.UUID) (*models.File, error) {
	// Get file info from storage
	fileInfo, err := s.GetFileInfoFromURL(ctx, tenantID, url)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Extract filename from URL
	filename := s.extractFilenameFromURL(url)

	// Create file record
	file := &models.File{
		TenantID:            tenantID,
		DataSourceID:        dataSourceID,
		ProcessingSessionID: sessionID,
		URL:                 url,
		Path:                url,
		Filename:            filename,
		Size:                fileInfo.Size,
		ContentType:         fileInfo.ContentType,
		Checksum:            fileInfo.Checksum,
		ChecksumType:        fileInfo.ChecksumType,
		Status:              models.FileStatusDiscovered,
		EncryptionStatus:    s.getEncryptionStatus(fileInfo),
		Metadata:            s.convertMetadata(fileInfo.Metadata),
		LastModified:        fileInfo.LastModified,
	}

	// Store in database
	tenantService := s.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant repository: %w", err)
	}

	if err := tenantRepo.ValidateAndCreate(file); err != nil {
		return nil, fmt.Errorf("failed to create file record: %w", err)
	}

	return file, nil
}

// DownloadFileContent downloads file content from a cloud storage URL
func (s *StorageService) DownloadFileContent(ctx context.Context, tenantID uuid.UUID, url string, options *storage.DownloadOptions) (io.ReadCloser, error) {
	return s.storageManager.DownloadFromURL(ctx, url, tenantID, options)
}

// GeneratePresignedURL generates a presigned URL for file access
func (s *StorageService) GeneratePresignedURL(ctx context.Context, tenantID uuid.UUID, url string, method string, expiration time.Duration) (*storage.PresignedURL, error) {
	options := &storage.PresignedURLOptions{
		Method:     method,
		Expiration: expiration,
	}

	return s.storageManager.GeneratePresignedURLFromURL(ctx, url, tenantID, options)
}

// BulkDiscoverFiles discovers multiple files concurrently
func (s *StorageService) BulkDiscoverFiles(ctx context.Context, tenantID uuid.UUID, urls []string, dataSourceID *uuid.UUID, sessionID *uuid.UUID) ([]*models.File, []error) {
	type result struct {
		File  *models.File
		Error error
		Index int
	}

	resultChan := make(chan result, len(urls))

	// Launch discovery goroutines
	for i, url := range urls {
		go func(index int, u string) {
			file, err := s.CreateFileFromURL(ctx, tenantID, u, dataSourceID, sessionID)
			resultChan <- result{
				File:  file,
				Error: err,
				Index: index,
			}
		}(i, url)
	}

	// Collect results
	files := make([]*models.File, len(urls))
	errors := make([]error, len(urls))

	for i := 0; i < len(urls); i++ {
		res := <-resultChan
		files[res.Index] = res.File
		errors[res.Index] = res.Error
	}

	return files, errors
}

// SyncDataSource synchronizes files from a data source URL
func (s *StorageService) SyncDataSource(ctx context.Context, tenantID uuid.UUID, dataSource *models.DataSource) (*DataSourceSyncResult, error) {
	startTime := time.Now()

	result := &DataSourceSyncResult{
		DataSourceID: dataSource.ID,
		StartTime:    startTime,
		Status:       "running",
	}

	// Convert sync settings to map for parsing
	syncSettingsMap := map[string]interface{}{
		"prefix":           "",
		"recursive":        dataSource.SyncSettings.IncrementalSync,
		"max_files":        float64(dataSource.SyncSettings.BatchSize),
		"include_patterns": []interface{}{dataSource.SyncSettings.FilePattern},
		"exclude_patterns": []interface{}{dataSource.SyncSettings.ExcludePattern},
	}

	// Parse sync configuration
	syncConfig, err := s.parseSyncConfig(syncSettingsMap)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("invalid sync configuration: %v", err)
		result.EndTime = time.Now()
		return result, err
	}

	// Discover files
	listOptions := &storage.ListOptions{
		Prefix:          syncConfig.Prefix,
		Recursive:       syncConfig.Recursive,
		IncludeMetadata: true,
		MaxKeys:         syncConfig.MaxFiles,
	}

	// Construct URL from datasource config
	dataSourceURL := s.constructDataSourceURL(dataSource)

	listResult, err := s.DiscoverFilesFromURL(ctx, tenantID, dataSourceURL, listOptions)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to discover files: %v", err)
		result.EndTime = time.Now()
		return result, err
	}

	result.FilesDiscovered = len(listResult.Files)

	// Filter files based on sync settings
	filesToSync := s.filterFiles(listResult.Files, syncConfig)
	result.FilesToSync = len(filesToSync)

	// Create file records for new/updated files
	urls := make([]string, len(filesToSync))
	for i, fileInfo := range filesToSync {
		urls[i] = fileInfo.URL
	}

	files, fileErrors := s.BulkDiscoverFiles(ctx, tenantID, urls, &dataSource.ID, nil)

	// Count successes and failures
	for i, err := range fileErrors {
		if err != nil {
			result.FilesFailed++
			result.Errors = append(result.Errors, fmt.Sprintf("File %s: %v", urls[i], err))
		} else if files[i] != nil {
			result.FilesCreated++
			result.FileIDs = append(result.FileIDs, files[i].ID)
		}
	}

	result.Status = "completed"
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result, nil
}

// GetSupportedProviders returns all supported cloud providers
func (s *StorageService) GetSupportedProviders() []storage.CloudProvider {
	return s.storageManager.GetSupportedProviders()
}

// ParseURL parses a storage URL to determine its components
func (s *StorageService) ParseURL(url string) (*storage.StorageURL, error) {
	return s.storageManager.ParseURL(url)
}

// Helper methods

func (s *StorageService) extractFilenameFromURL(url string) string {
	storageURL, err := s.storageManager.ParseURL(url)
	if err != nil {
		return "unknown"
	}

	// Extract filename from key
	parts := strings.Split(storageURL.Key, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return storageURL.Key
}

func (s *StorageService) getEncryptionStatus(fileInfo *storage.FileInfo) string {
	if fileInfo.Encrypted {
		return "encrypted"
	}
	return "none"
}

func (s *StorageService) convertMetadata(metadata map[string]string) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range metadata {
		result[k] = v
	}
	return result
}

func (s *StorageService) parseSyncConfig(syncSettings map[string]interface{}) (*SyncConfig, error) {
	config := &SyncConfig{
		Recursive: true,
		MaxFiles:  1000,
	}

	if prefix, ok := syncSettings["prefix"].(string); ok {
		config.Prefix = prefix
	}

	if recursive, ok := syncSettings["recursive"].(bool); ok {
		config.Recursive = recursive
	}

	if maxFiles, ok := syncSettings["max_files"].(float64); ok {
		config.MaxFiles = int(maxFiles)
	}

	if patterns, ok := syncSettings["include_patterns"].([]interface{}); ok {
		for _, pattern := range patterns {
			if str, ok := pattern.(string); ok {
				config.IncludePatterns = append(config.IncludePatterns, str)
			}
		}
	}

	if patterns, ok := syncSettings["exclude_patterns"].([]interface{}); ok {
		for _, pattern := range patterns {
			if str, ok := pattern.(string); ok {
				config.ExcludePatterns = append(config.ExcludePatterns, str)
			}
		}
	}

	return config, nil
}

func (s *StorageService) filterFiles(files []*storage.FileInfo, config *SyncConfig) []*storage.FileInfo {
	var filtered []*storage.FileInfo

	for _, file := range files {
		// Apply include patterns
		if len(config.IncludePatterns) > 0 {
			included := false
			for _, pattern := range config.IncludePatterns {
				if matched, _ := filepath.Match(pattern, file.URL); matched {
					included = true
					break
				}
			}
			if !included {
				continue
			}
		}

		// Apply exclude patterns
		excluded := false
		for _, pattern := range config.ExcludePatterns {
			if matched, _ := filepath.Match(pattern, file.URL); matched {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		filtered = append(filtered, file)
	}

	return filtered
}

func (s *StorageService) constructDataSourceURL(dataSource *models.DataSource) string {
	switch dataSource.Type {
	case "s3", "aws":
		if dataSource.Config.Bucket != "" {
			url := "s3://" + dataSource.Config.Bucket
			if dataSource.Config.Path != "" {
				url += "/" + strings.TrimPrefix(dataSource.Config.Path, "/")
			}
			return url
		}
	case "gcs", "gcp":
		if dataSource.Config.Bucket != "" {
			url := "gs://" + dataSource.Config.Bucket
			if dataSource.Config.Path != "" {
				url += "/" + strings.TrimPrefix(dataSource.Config.Path, "/")
			}
			return url
		}
	case "local", "file":
		if dataSource.Config.RootPath != "" {
			return "file://" + dataSource.Config.RootPath
		}
	}

	// Fallback - this should not happen in production
	return ""
}

// SyncConfig represents data source sync configuration
type SyncConfig struct {
	Prefix          string   `json:"prefix,omitempty"`
	Recursive       bool     `json:"recursive"`
	MaxFiles        int      `json:"max_files"`
	IncludePatterns []string `json:"include_patterns,omitempty"`
	ExcludePatterns []string `json:"exclude_patterns,omitempty"`
}

// DataSourceSyncResult represents the result of a data source sync operation
type DataSourceSyncResult struct {
	DataSourceID    uuid.UUID     `json:"data_source_id"`
	Status          string        `json:"status"`
	StartTime       time.Time     `json:"start_time"`
	EndTime         time.Time     `json:"end_time"`
	Duration        time.Duration `json:"duration"`
	FilesDiscovered int           `json:"files_discovered"`
	FilesToSync     int           `json:"files_to_sync"`
	FilesCreated    int           `json:"files_created"`
	FilesFailed     int           `json:"files_failed"`
	FileIDs         []uuid.UUID   `json:"file_ids"`
	Errors          []string      `json:"errors,omitempty"`
	Error           string        `json:"error,omitempty"`
}
