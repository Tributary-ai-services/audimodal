package compression

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/storage"
	"github.com/jscharber/eAIIngest/pkg/storage/cache"
)

// CompressedStorageResolver wraps a storage resolver with compression capabilities
type CompressedStorageResolver struct {
	baseResolver   storage.StorageResolver
	policyManager  *PolicyManager
	cache          cache.Cache
	tracer         trace.Tracer
	config         *CompressionIntegrationConfig
}

// CompressionIntegrationConfig contains configuration for storage integration
type CompressionIntegrationConfig struct {
	// Compression settings
	EnableCompression     bool          `yaml:"enable_compression"`
	CacheCompressed       bool          `yaml:"cache_compressed"`
	StoreOriginal         bool          `yaml:"store_original"`
	AsyncCompression      bool          `yaml:"async_compression"`
	
	// Performance settings
	CompressionThreshold  int64         `yaml:"compression_threshold"`  // Min file size to compress
	CacheThreshold        int64         `yaml:"cache_threshold"`        // Min file size to cache
	MaxConcurrentJobs     int           `yaml:"max_concurrent_jobs"`
	CompressionTimeout    time.Duration `yaml:"compression_timeout"`
	
	// Storage settings
	CompressedPrefix      string        `yaml:"compressed_prefix"`      // Prefix for compressed files
	MetadataPrefix        string        `yaml:"metadata_prefix"`        // Prefix for compression metadata
	OriginalPrefix        string        `yaml:"original_prefix"`        // Prefix for original files
	
	// Cleanup settings
	CleanupOriginals      bool          `yaml:"cleanup_originals"`
	CleanupDelay          time.Duration `yaml:"cleanup_delay"`
	RetentionPeriod       time.Duration `yaml:"retention_period"`
}

// DefaultCompressionIntegrationConfig returns default integration configuration
func DefaultCompressionIntegrationConfig() *CompressionIntegrationConfig {
	return &CompressionIntegrationConfig{
		EnableCompression:    true,
		CacheCompressed:      true,
		StoreOriginal:        false,
		AsyncCompression:     true,
		CompressionThreshold: 1024,      // 1KB
		CacheThreshold:       10 * 1024, // 10KB
		MaxConcurrentJobs:    10,
		CompressionTimeout:   30 * time.Second,
		CompressedPrefix:     "compressed/",
		MetadataPrefix:       "metadata/",
		OriginalPrefix:       "original/",
		CleanupOriginals:     true,
		CleanupDelay:         24 * time.Hour,
		RetentionPeriod:      30 * 24 * time.Hour, // 30 days
	}
}

// NewCompressedStorageResolver creates a new compressed storage resolver
func NewCompressedStorageResolver(
	baseResolver storage.StorageResolver,
	policyManager *PolicyManager,
	cache cache.Cache,
	config *CompressionIntegrationConfig,
) *CompressedStorageResolver {
	if config == nil {
		config = DefaultCompressionIntegrationConfig()
	}

	return &CompressedStorageResolver{
		baseResolver:  baseResolver,
		policyManager: policyManager,
		cache:         cache,
		config:        config,
		tracer:        otel.Tracer("compressed-storage-resolver"),
	}
}

// ParseURL delegates to the base resolver
func (r *CompressedStorageResolver) ParseURL(urlStr string) (*storage.StorageURL, error) {
	return r.baseResolver.ParseURL(urlStr)
}

// GetFileInfo retrieves file information, checking for compressed versions
func (r *CompressedStorageResolver) GetFileInfo(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials) (*storage.FileInfo, error) {
	ctx, span := r.tracer.Start(ctx, "compressed_storage_get_file_info")
	defer span.End()

	span.SetAttributes(
		attribute.String("storage.url", storageURL.String()),
		attribute.String("storage.provider", string(storageURL.Provider)),
	)

	// Check cache first
	if r.cache != nil {
		if fileInfo, err := r.cache.GetFileInfo(ctx, storageURL.String()); err == nil {
			span.SetAttributes(attribute.Bool("cache.hit", true))
			return fileInfo, nil
		}
	}

	// Get file info from base resolver
	fileInfo, err := r.baseResolver.GetFileInfo(ctx, storageURL, credentials)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Check if compressed version exists
	compressedURL := r.buildCompressedURL(storageURL)
	if compressedInfo, err := r.baseResolver.GetFileInfo(ctx, compressedURL, credentials); err == nil {
		// Check if compressed version is newer or preferred
		if r.shouldUseCompressed(fileInfo, compressedInfo) {
			// Load compression metadata
			if metadata, err := r.loadCompressionMetadata(ctx, storageURL, credentials); err == nil {
				fileInfo.Metadata["compression"] = metadata
				fileInfo.Metadata["compressed"] = true
				fileInfo.Metadata["compressed_size"] = compressedInfo.Size
				fileInfo.Metadata["compression_ratio"] = metadata.CompressionRatio
			}
		}
	}

	// Cache the result
	if r.cache != nil && fileInfo.Size >= r.config.CacheThreshold {
		if err := r.cache.SetFileInfo(ctx, storageURL.String(), fileInfo); err != nil {
			// Log but don't fail on cache errors
			span.AddEvent("cache_set_failed", trace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
	}

	span.SetAttributes(
		attribute.Int64("file.size", fileInfo.Size),
		attribute.String("file.mime_type", fileInfo.MimeType),
		attribute.Bool("file.compressed", fileInfo.Metadata["compressed"] == true),
	)

	return fileInfo, nil
}

// DownloadFile downloads a file, decompressing if necessary
func (r *CompressedStorageResolver) DownloadFile(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials, options *storage.DownloadOptions) (io.ReadCloser, error) {
	ctx, span := r.tracer.Start(ctx, "compressed_storage_download_file")
	defer span.End()

	span.SetAttributes(
		attribute.String("storage.url", storageURL.String()),
		attribute.String("storage.provider", string(storageURL.Provider)),
	)

	// Get file info to check for compression
	fileInfo, err := r.GetFileInfo(ctx, storageURL, credentials)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Check if file is compressed
	if compressed, ok := fileInfo.Metadata["compressed"].(bool); ok && compressed {
		return r.downloadCompressedFile(ctx, storageURL, credentials, options, fileInfo)
	}

	// Download normally
	return r.baseResolver.DownloadFile(ctx, storageURL, credentials, options)
}

// downloadCompressedFile downloads and decompresses a file
func (r *CompressedStorageResolver) downloadCompressedFile(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials, options *storage.DownloadOptions, fileInfo *storage.FileInfo) (io.ReadCloser, error) {
	ctx, span := r.tracer.Start(ctx, "download_compressed_file")
	defer span.End()

	// Load compression metadata
	metadata, err := r.loadCompressionMetadata(ctx, storageURL, credentials)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to load compression metadata: %w", err)
	}

	// Check cache for decompressed content
	if r.cache != nil {
		cacheKey := r.buildContentCacheKey(storageURL.String(), metadata.ChecksumCompressed)
		if content, err := r.cache.GetContent(ctx, storageURL.String(), cacheKey); err == nil {
			span.SetAttributes(attribute.Bool("cache.hit", true))
			return &readCloser{data: content}, nil
		}
	}

	// Download compressed file
	compressedURL := r.buildCompressedURL(storageURL)
	reader, err := r.baseResolver.DownloadFile(ctx, compressedURL, credentials, options)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to download compressed file: %w", err)
	}
	defer reader.Close()

	// Read compressed data
	compressedData, err := io.ReadAll(reader)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to read compressed data: %w", err)
	}

	// Decompress
	compressor := r.policyManager.compressor
	decompressed, err := compressor.DecompressDocument(ctx, compressedData, metadata)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to decompress file: %w", err)
	}

	// Cache decompressed content
	if r.cache != nil && int64(len(decompressed)) >= r.config.CacheThreshold {
		cacheKey := r.buildContentCacheKey(storageURL.String(), metadata.ChecksumCompressed)
		if err := r.cache.SetContent(ctx, storageURL.String(), cacheKey, decompressed); err != nil {
			// Log but don't fail on cache errors
			span.AddEvent("cache_set_failed", trace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
	}

	span.SetAttributes(
		attribute.Int("compressed.size", len(compressedData)),
		attribute.Int("decompressed.size", len(decompressed)),
		attribute.Float64("compression.ratio", metadata.CompressionRatio),
	)

	return &readCloser{data: decompressed}, nil
}

// ListFiles lists files, including compression information
func (r *CompressedStorageResolver) ListFiles(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials, options *storage.ListOptions) (*storage.ListResult, error) {
	ctx, span := r.tracer.Start(ctx, "compressed_storage_list_files")
	defer span.End()

	// Get base listing
	result, err := r.baseResolver.ListFiles(ctx, storageURL, credentials, options)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Enrich with compression information
	for i, file := range result.Files {
		// Check if compressed version exists
		compressedURL := r.buildCompressedURLFromPath(storageURL, file.Name)
		if _, err := r.baseResolver.GetFileInfo(ctx, compressedURL, credentials); err == nil {
			file.Metadata["has_compressed_version"] = true
			
			// Load compression metadata if available
			if metadata, err := r.loadCompressionMetadataFromPath(ctx, storageURL, file.Name, credentials); err == nil {
				file.Metadata["compression_ratio"] = metadata.CompressionRatio
				file.Metadata["compression_strategy"] = metadata.Strategy
			}
		}
		result.Files[i] = file
	}

	span.SetAttributes(
		attribute.Int("files.count", len(result.Files)),
		attribute.Bool("has_more", result.HasMore),
	)

	return result, nil
}

// GeneratePresignedURL generates presigned URLs, handling compression
func (r *CompressedStorageResolver) GeneratePresignedURL(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials, options *storage.PresignedURLOptions) (*storage.PresignedURL, error) {
	// For now, delegate to base resolver
	// In a full implementation, this would need to handle compressed file URLs
	return r.baseResolver.GeneratePresignedURL(ctx, storageURL, credentials, options)
}

// ValidateAccess validates access to storage
func (r *CompressedStorageResolver) ValidateAccess(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials) error {
	return r.baseResolver.ValidateAccess(ctx, storageURL, credentials)
}

// CompressAndStore compresses a file and stores it
func (r *CompressedStorageResolver) CompressAndStore(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials, data []byte, tenantID uuid.UUID) (*CompressionResult, error) {
	ctx, span := r.tracer.Start(ctx, "compress_and_store")
	defer span.End()

	if !r.config.EnableCompression {
		return nil, fmt.Errorf("compression is disabled")
	}

	if int64(len(data)) < r.config.CompressionThreshold {
		return nil, fmt.Errorf("file size below compression threshold")
	}

	// Get file info for policy evaluation
	fileInfo := &storage.FileInfo{
		URL:  storageURL.String(),
		Size: int64(len(data)),
		Name: storageURL.Path,
	}

	// Apply compression based on policy
	result, err := r.policyManager.ApplyCompression(ctx, data, fileInfo, tenantID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Store compressed file
	compressedURL := r.buildCompressedURL(storageURL)
	if err := r.storeData(ctx, compressedURL, credentials, result.Data); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to store compressed file: %w", err)
	}

	// Store compression metadata
	metadata := &CompressionMetadata{
		Strategy:           result.Strategy,
		Algorithm:          result.Algorithm,
		OriginalSize:       int64(result.OriginalSize),
		CompressedSize:     int64(result.CompressedSize),
		CompressionRatio:   result.CompressionRatio,
		CompressionTime:    result.CompressionTime,
		OriginalFilename:   fileInfo.Name,
		CompressedAt:       time.Now(),
	}

	if err := r.storeCompressionMetadata(ctx, storageURL, credentials, metadata); err != nil {
		span.RecordError(err)
		// Log error but don't fail the operation
		span.AddEvent("metadata_store_failed", trace.WithAttributes(
			attribute.String("error", err.Error()),
		))
	}

	// Store original if configured
	if r.config.StoreOriginal {
		originalURL := r.buildOriginalURL(storageURL)
		if err := r.storeData(ctx, originalURL, credentials, data); err != nil {
			span.RecordError(err)
			// Log error but don't fail the operation
			span.AddEvent("original_store_failed", trace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
	}

	span.SetAttributes(
		attribute.String("compression.strategy", string(result.Strategy)),
		attribute.Float64("compression.ratio", result.CompressionRatio),
		attribute.Int("original.size", result.OriginalSize),
		attribute.Int("compressed.size", result.CompressedSize),
	)

	return result, nil
}

// Helper methods

func (r *CompressedStorageResolver) buildCompressedURL(storageURL *storage.StorageURL) *storage.StorageURL {
	compressedURL := *storageURL
	compressedURL.Path = r.config.CompressedPrefix + storageURL.Path + ".gz"
	return &compressedURL
}

func (r *CompressedStorageResolver) buildCompressedURLFromPath(storageURL *storage.StorageURL, filename string) *storage.StorageURL {
	compressedURL := *storageURL
	compressedURL.Path = r.config.CompressedPrefix + filename + ".gz"
	return &compressedURL
}

func (r *CompressedStorageResolver) buildOriginalURL(storageURL *storage.StorageURL) *storage.StorageURL {
	originalURL := *storageURL
	originalURL.Path = r.config.OriginalPrefix + storageURL.Path
	return &originalURL
}

func (r *CompressedStorageResolver) buildMetadataURL(storageURL *storage.StorageURL) *storage.StorageURL {
	metadataURL := *storageURL
	metadataURL.Path = r.config.MetadataPrefix + storageURL.Path + ".meta.json"
	return &metadataURL
}

func (r *CompressedStorageResolver) buildContentCacheKey(url, checksum string) string {
	return fmt.Sprintf("content:%s:%s", url, checksum)
}

func (r *CompressedStorageResolver) shouldUseCompressed(original, compressed *storage.FileInfo) bool {
	if !r.config.EnableCompression {
		return false
	}
	
	// Prefer compressed if it's significantly smaller or newer
	sizeRatio := float64(compressed.Size) / float64(original.Size)
	return sizeRatio < 0.8 || compressed.ModifiedAt.After(original.ModifiedAt)
}

func (r *CompressedStorageResolver) loadCompressionMetadata(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials) (*CompressionMetadata, error) {
	metadataURL := r.buildMetadataURL(storageURL)
	reader, err := r.baseResolver.DownloadFile(ctx, metadataURL, credentials, nil)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var metadata CompressionMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

func (r *CompressedStorageResolver) loadCompressionMetadataFromPath(ctx context.Context, storageURL *storage.StorageURL, filename string, credentials storage.Credentials) (*CompressionMetadata, error) {
	metadataURL := *storageURL
	metadataURL.Path = r.config.MetadataPrefix + filename + ".meta.json"
	
	reader, err := r.baseResolver.DownloadFile(ctx, &metadataURL, credentials, nil)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var metadata CompressionMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

func (r *CompressedStorageResolver) storeCompressionMetadata(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials, metadata *CompressionMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	metadataURL := r.buildMetadataURL(storageURL)
	return r.storeData(ctx, metadataURL, credentials, data)
}

func (r *CompressedStorageResolver) storeData(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials, data []byte) error {
	// This would need to be implemented based on the specific storage provider
	// For now, this is a placeholder that would delegate to a upload method on the base resolver
	return fmt.Errorf("storeData not implemented - would need upload capability in base resolver")
}

// readCloser implements io.ReadCloser for in-memory data
type readCloser struct {
	data   []byte
	pos    int
	closed bool
}

func (r *readCloser) Read(p []byte) (n int, err error) {
	if r.closed {
		return 0, fmt.Errorf("reader is closed")
	}
	
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (r *readCloser) Close() error {
	r.closed = true
	return nil
}