package encryption

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/storage"
	"github.com/jscharber/eAIIngest/pkg/storage/cache"
)

// EncryptedStorageResolver wraps a storage resolver with encryption capabilities
type EncryptedStorageResolver struct {
	baseResolver  storage.StorageResolver
	policyManager *PolicyManager
	encryptor     *Encryptor
	cache         cache.Cache
	config        *EncryptionIntegrationConfig
	tracer        trace.Tracer
}

// EncryptionIntegrationConfig contains configuration for storage integration
type EncryptionIntegrationConfig struct {
	// Encryption settings
	EnableEncryption bool `yaml:"enable_encryption"`
	TransparentMode  bool `yaml:"transparent_mode"` // Auto encrypt/decrypt
	CacheDecrypted   bool `yaml:"cache_decrypted"`  // Cache decrypted content

	// Performance settings
	EncryptionThreshold int64 `yaml:"encryption_threshold"` // Min file size to encrypt
	MaxConcurrentOps    int   `yaml:"max_concurrent_ops"`
	StreamingThreshold  int64 `yaml:"streaming_threshold"` // Use streaming for large files

	// Storage settings
	EncryptedPrefix   string `yaml:"encrypted_prefix"`    // Prefix for encrypted files
	MetadataPrefix    string `yaml:"metadata_prefix"`     // Prefix for encryption metadata
	KeyMetadataPrefix string `yaml:"key_metadata_prefix"` // Prefix for key metadata

	// Security settings
	RequireEncryption  bool `yaml:"require_encryption"` // Fail if encryption not possible
	AuditAllOperations bool `yaml:"audit_all_operations"`
	SecureDelete       bool `yaml:"secure_delete"`

	// Cache settings
	CacheTTL     time.Duration `yaml:"cache_ttl"`
	MaxCacheSize int64         `yaml:"max_cache_size"`
}

// DefaultEncryptionIntegrationConfig returns default integration configuration
func DefaultEncryptionIntegrationConfig() *EncryptionIntegrationConfig {
	return &EncryptionIntegrationConfig{
		EnableEncryption:    true,
		TransparentMode:     true,
		CacheDecrypted:      true,
		EncryptionThreshold: 0, // Encrypt all files
		MaxConcurrentOps:    10,
		StreamingThreshold:  10 * 1024 * 1024, // 10MB
		EncryptedPrefix:     ".encrypted/",
		MetadataPrefix:      ".metadata/",
		KeyMetadataPrefix:   ".keys/",
		RequireEncryption:   false,
		AuditAllOperations:  true,
		SecureDelete:        true,
		CacheTTL:            1 * time.Hour,
		MaxCacheSize:        100 * 1024 * 1024, // 100MB
	}
}

// NewEncryptedStorageResolver creates a new encrypted storage resolver
func NewEncryptedStorageResolver(
	baseResolver storage.StorageResolver,
	policyManager *PolicyManager,
	encryptor *Encryptor,
	cache cache.Cache,
	config *EncryptionIntegrationConfig,
) *EncryptedStorageResolver {
	if config == nil {
		config = DefaultEncryptionIntegrationConfig()
	}

	return &EncryptedStorageResolver{
		baseResolver:  baseResolver,
		policyManager: policyManager,
		encryptor:     encryptor,
		cache:         cache,
		config:        config,
		tracer:        otel.Tracer("encrypted-storage-resolver"),
	}
}

// ParseURL delegates to the base resolver
func (r *EncryptedStorageResolver) ParseURL(urlStr string) (*storage.StorageURL, error) {
	return r.baseResolver.ParseURL(urlStr)
}

// GetFileInfo retrieves file information, handling encryption metadata
func (r *EncryptedStorageResolver) GetFileInfo(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials) (*storage.FileInfo, error) {
	ctx, span := r.tracer.Start(ctx, "encrypted_storage_get_file_info")
	defer span.End()

	span.SetAttributes(
		attribute.String("storage.url", storageURL.String()),
		attribute.String("storage.provider", string(storageURL.Provider)),
	)

	// Check if this is an encrypted file
	encryptedURL := r.buildEncryptedURL(storageURL)
	encryptedInfo, err := r.baseResolver.GetFileInfo(ctx, encryptedURL, &credentials)
	if err == nil {
		// File is encrypted, load metadata
		metadata, err := r.loadEncryptionMetadata(ctx, storageURL, credentials)
		if err == nil {
			// Update file info with original size and encryption details
			fileInfo := *encryptedInfo
			fileInfo.Size = metadata.OriginalSize
			fileInfo.Encrypted = true
			fileInfo.Metadata["encrypted"] = "true"
			fileInfo.Metadata["encryption_algorithm"] = string(metadata.Algorithm)
			fileInfo.Metadata["encryption_key_id"] = metadata.KeyID.String()
			fileInfo.Metadata["original_size"] = fmt.Sprintf("%d", metadata.OriginalSize)
			fileInfo.Metadata["encrypted_size"] = fmt.Sprintf("%d", metadata.EncryptedSize)

			span.SetAttributes(
				attribute.Bool("file.encrypted", true),
				attribute.String("encryption.algorithm", string(metadata.Algorithm)),
			)

			return &fileInfo, nil
		}
	}

	// Fall back to unencrypted file
	return r.baseResolver.GetFileInfo(ctx, storageURL, &credentials)
}

// DownloadFile downloads a file, decrypting if necessary
func (r *EncryptedStorageResolver) DownloadFile(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials, options *storage.DownloadOptions) (io.ReadCloser, error) {
	ctx, span := r.tracer.Start(ctx, "encrypted_storage_download_file")
	defer span.End()

	span.SetAttributes(
		attribute.String("storage.url", storageURL.String()),
		attribute.String("storage.provider", string(storageURL.Provider)),
	)

	// Get file info to check if encrypted
	fileInfo, err := r.GetFileInfo(ctx, storageURL, credentials)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Check if file is encrypted
	if encrypted, ok := fileInfo.Metadata["encrypted"]; ok && encrypted == "true" {
		return r.downloadEncryptedFile(ctx, storageURL, credentials, options, fileInfo)
	}

	// Download unencrypted file
	return r.baseResolver.DownloadFile(ctx, storageURL, &credentials, options)
}

// downloadEncryptedFile downloads and decrypts an encrypted file
func (r *EncryptedStorageResolver) downloadEncryptedFile(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials, options *storage.DownloadOptions, fileInfo *storage.FileInfo) (io.ReadCloser, error) {
	ctx, span := r.tracer.Start(ctx, "download_encrypted_file")
	defer span.End()

	// Load encryption metadata
	metadata, err := r.loadEncryptionMetadata(ctx, storageURL, credentials)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to load encryption metadata: %w", err)
	}

	// Check cache for decrypted content
	if r.cache != nil && r.config.CacheDecrypted {
		cacheKey := r.buildContentCacheKey(storageURL.String(), metadata.Checksum)
		if content, err := r.cache.GetContent(ctx, storageURL.String(), cacheKey); err == nil {
			span.SetAttributes(attribute.Bool("cache.hit", true))
			return &readCloser{data: content}, nil
		}
	}

	// Download encrypted file
	encryptedURL := r.buildEncryptedURL(storageURL)
	reader, err := r.baseResolver.DownloadFile(ctx, encryptedURL, &credentials, options)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to download encrypted file: %w", err)
	}
	defer reader.Close()

	// Read encrypted data
	encryptedData, err := io.ReadAll(reader)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to read encrypted data: %w", err)
	}

	// Decrypt
	result, err := r.encryptor.DecryptDocument(ctx, encryptedData, metadata)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to decrypt file: %w", err)
	}

	// Cache decrypted content
	if r.cache != nil && r.config.CacheDecrypted && int64(len(result.DecryptedData)) <= r.config.MaxCacheSize {
		cacheKey := r.buildContentCacheKey(storageURL.String(), metadata.Checksum)
		if err := r.cache.SetContent(ctx, storageURL.String(), cacheKey, result.DecryptedData); err != nil {
			// Log but don't fail
			span.AddEvent("cache_set_failed", trace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
	}

	span.SetAttributes(
		attribute.Int("encrypted.size", len(encryptedData)),
		attribute.Int("decrypted.size", len(result.DecryptedData)),
		attribute.Float64("decryption.duration_ms", float64(result.Duration.Milliseconds())),
	)

	return &readCloser{data: result.DecryptedData}, nil
}

// UploadFile uploads a file, encrypting based on policy
func (r *EncryptedStorageResolver) UploadFile(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials, data []byte, tenantID uuid.UUID) error {
	ctx, span := r.tracer.Start(ctx, "encrypted_storage_upload_file")
	defer span.End()

	if !r.config.EnableEncryption {
		return fmt.Errorf("upload not implemented in base resolver")
	}

	span.SetAttributes(
		attribute.String("storage.url", storageURL.String()),
		attribute.String("tenant.id", tenantID.String()),
		attribute.Int("data.size", len(data)),
	)

	// Create file info for policy evaluation
	fileInfo := &storage.FileInfo{
		URL:      storageURL.String(),
		Size:     int64(len(data)),
		Name:     storageURL.Path,
		Metadata: make(map[string]string),
	}

	// Apply encryption based on policy
	result, err := r.policyManager.ApplyEncryption(ctx, data, fileInfo, tenantID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to apply encryption: %w", err)
	}

	if result == nil {
		// No encryption required
		span.SetAttributes(attribute.Bool("encrypted", false))
		return fmt.Errorf("upload without encryption not implemented")
	}

	// Store encrypted file
	encryptedURL := r.buildEncryptedURL(storageURL)
	if err := r.storeData(ctx, encryptedURL, credentials, result.EncryptedData); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to store encrypted file: %w", err)
	}

	// Store encryption metadata
	if err := r.storeEncryptionMetadata(ctx, storageURL, credentials, result.Metadata); err != nil {
		span.RecordError(err)
		// Log error but don't fail the operation
		span.AddEvent("metadata_store_failed", trace.WithAttributes(
			attribute.String("error", err.Error()),
		))
	}

	span.SetAttributes(
		attribute.Bool("encrypted", true),
		attribute.String("encryption.algorithm", string(result.Metadata.Algorithm)),
		attribute.Int("encrypted.size", len(result.EncryptedData)),
		attribute.Float64("encryption.duration_ms", float64(result.Duration.Milliseconds())),
	)

	return nil
}

// ListFiles lists files, including encryption information
func (r *EncryptedStorageResolver) ListFiles(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials, options *storage.ListOptions) (*storage.ListResult, error) {
	ctx, span := r.tracer.Start(ctx, "encrypted_storage_list_files")
	defer span.End()

	// Get base listing
	result, err := r.baseResolver.ListFiles(ctx, storageURL, &credentials, options)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Filter out encrypted files and metadata
	var filteredFiles []*storage.FileInfo
	encryptedFiles := make(map[string]bool)

	for _, file := range result.Files {
		// Skip internal encrypted files and metadata
		if strings.HasPrefix(file.Name, r.config.EncryptedPrefix) ||
			strings.HasPrefix(file.Name, r.config.MetadataPrefix) ||
			strings.HasPrefix(file.Name, r.config.KeyMetadataPrefix) {
			continue
		}

		// Check if encrypted version exists
		encryptedPath := r.config.EncryptedPrefix + file.Name + ".enc"
		for _, f := range result.Files {
			if f.Name == encryptedPath {
				encryptedFiles[file.Name] = true
				file.Encrypted = true
				file.Metadata["has_encrypted_version"] = "true"
				break
			}
		}

		filteredFiles = append(filteredFiles, file)
	}

	result.Files = filteredFiles
	result.KeyCount = len(filteredFiles)

	span.SetAttributes(
		attribute.Int("files.total", len(result.Files)),
		attribute.Int("files.encrypted", len(encryptedFiles)),
	)

	return result, nil
}

// GeneratePresignedURL generates presigned URLs, handling encryption
func (r *EncryptedStorageResolver) GeneratePresignedURL(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials, options *storage.PresignedURLOptions) (*storage.PresignedURL, error) {
	// For encrypted files, we can't provide direct presigned URLs
	// as they would return encrypted content
	fileInfo, err := r.GetFileInfo(ctx, storageURL, credentials)
	if err != nil {
		return nil, err
	}

	if encrypted, ok := fileInfo.Metadata["encrypted"]; ok && encrypted == "true" {
		return nil, fmt.Errorf("presigned URLs not supported for encrypted files")
	}

	return r.baseResolver.GeneratePresignedURL(ctx, storageURL, &credentials, options)
}

// ValidateAccess validates access to storage
func (r *EncryptedStorageResolver) ValidateAccess(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials) error {
	return r.baseResolver.ValidateAccess(ctx, storageURL, &credentials)
}

// GetSupportedProviders returns supported providers
func (r *EncryptedStorageResolver) GetSupportedProviders() []storage.CloudProvider {
	return r.baseResolver.GetSupportedProviders()
}

// Helper methods

func (r *EncryptedStorageResolver) buildEncryptedURL(storageURL *storage.StorageURL) *storage.StorageURL {
	encryptedURL := *storageURL
	encryptedURL.Path = r.config.EncryptedPrefix + storageURL.Path + ".enc"
	return &encryptedURL
}

func (r *EncryptedStorageResolver) buildMetadataURL(storageURL *storage.StorageURL) *storage.StorageURL {
	metadataURL := *storageURL
	metadataURL.Path = r.config.MetadataPrefix + storageURL.Path + ".meta.json"
	return &metadataURL
}

func (r *EncryptedStorageResolver) buildContentCacheKey(url, checksum string) string {
	return fmt.Sprintf("decrypted:%s:%s", url, checksum)
}

func (r *EncryptedStorageResolver) loadEncryptionMetadata(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials) (*EncryptionMetadata, error) {
	metadataURL := r.buildMetadataURL(storageURL)
	reader, err := r.baseResolver.DownloadFile(ctx, metadataURL, &credentials, nil)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var metadata EncryptionMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

func (r *EncryptedStorageResolver) storeEncryptionMetadata(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials, metadata *EncryptionMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	metadataURL := r.buildMetadataURL(storageURL)
	return r.storeData(ctx, metadataURL, credentials, data)
}

func (r *EncryptedStorageResolver) storeData(ctx context.Context, storageURL *storage.StorageURL, credentials storage.Credentials, data []byte) error {
	// This would need to be implemented based on the specific storage provider
	// For now, this is a placeholder
	return fmt.Errorf("storeData not implemented - would need upload capability in base resolver")
}

// readCloser implements io.ReadCloser for in-memory data
type readCloser struct {
	data   []byte
	pos    int
	closed bool
}

func (rc *readCloser) Read(p []byte) (n int, err error) {
	if rc.closed {
		return 0, fmt.Errorf("reader is closed")
	}

	if rc.pos >= len(rc.data) {
		return 0, io.EOF
	}

	n = copy(p, rc.data[rc.pos:])
	rc.pos += n
	return n, nil
}

func (rc *readCloser) Close() error {
	rc.closed = true
	return nil
}

// TransparentEncryption provides transparent encryption/decryption
type TransparentEncryption struct {
	resolver *EncryptedStorageResolver
}

// NewTransparentEncryption creates a new transparent encryption layer
func NewTransparentEncryption(resolver *EncryptedStorageResolver) *TransparentEncryption {
	return &TransparentEncryption{
		resolver: resolver,
	}
}

// WrapReader wraps a reader with transparent decryption
func (te *TransparentEncryption) WrapReader(ctx context.Context, reader io.ReadCloser, metadata *EncryptionMetadata) (io.ReadCloser, error) {
	if metadata == nil {
		// Not encrypted
		return reader, nil
	}

	// Read all data for decryption
	// For large files, this would use streaming decryption
	data, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		return nil, err
	}

	// Decrypt
	result, err := te.resolver.encryptor.DecryptDocument(ctx, data, metadata)
	if err != nil {
		return nil, err
	}

	return &readCloser{data: result.DecryptedData}, nil
}
