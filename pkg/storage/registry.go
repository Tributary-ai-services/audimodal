package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// DefaultResolverRegistry implements ResolverRegistry
type DefaultResolverRegistry struct {
	resolvers map[CloudProvider]StorageResolver
	mu        sync.RWMutex
}

// NewResolverRegistry creates a new resolver registry
func NewResolverRegistry() *DefaultResolverRegistry {
	return &DefaultResolverRegistry{
		resolvers: make(map[CloudProvider]StorageResolver),
	}
}

// RegisterResolver registers a resolver for specific providers
func (r *DefaultResolverRegistry) RegisterResolver(providers []CloudProvider, resolver StorageResolver) error {
	if resolver == nil {
		return NewStorageError(
			ErrorCodeInternalError,
			"resolver cannot be nil",
			"",
			"",
			nil,
		)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, provider := range providers {
		r.resolvers[provider] = resolver
	}

	return nil
}

// GetResolver returns the appropriate resolver for a provider
func (r *DefaultResolverRegistry) GetResolver(provider CloudProvider) (StorageResolver, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	resolver, exists := r.resolvers[provider]
	if !exists {
		return nil, NewStorageError(
			ErrorCodeUnsupportedProvider,
			fmt.Sprintf("no resolver found for provider: %s", provider),
			provider,
			"",
			nil,
		)
	}

	return resolver, nil
}

// ParseURL parses any supported storage URL
func (r *DefaultResolverRegistry) ParseURL(urlStr string) (*StorageURL, error) {
	provider, err := r.detectProvider(urlStr)
	if err != nil {
		return nil, err
	}

	resolver, err := r.GetResolver(provider)
	if err != nil {
		return nil, err
	}

	return resolver.ParseURL(urlStr)
}

// GetSupportedProviders returns all supported providers
func (r *DefaultResolverRegistry) GetSupportedProviders() []CloudProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := make([]CloudProvider, 0, len(r.resolvers))
	for provider := range r.resolvers {
		providers = append(providers, provider)
	}

	return providers
}

// detectProvider detects the cloud provider from a URL
func (r *DefaultResolverRegistry) detectProvider(urlStr string) (CloudProvider, error) {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return "", NewStorageError(
			ErrorCodeInvalidURL,
			"invalid URL format",
			"",
			urlStr,
			err,
		)
	}

	// Detect by scheme
	switch parsed.Scheme {
	case "s3":
		return ProviderAWS, nil
	case "gs":
		return ProviderGCP, nil
	case "az", "abfs", "abfss":
		return ProviderAzure, nil
	case "file":
		return ProviderLocal, nil
	}

	// Detect by hostname for HTTPS URLs
	if parsed.Scheme == "https" {
		host := strings.ToLower(parsed.Host)

		// AWS S3 detection
		if strings.Contains(host, "amazonaws.com") || strings.Contains(host, ".s3.") {
			return ProviderAWS, nil
		}

		// Google Cloud Storage detection
		if strings.Contains(host, "googleapis.com") || strings.Contains(host, "storage.cloud.google.com") {
			return ProviderGCP, nil
		}

		// Azure Blob Storage detection
		if strings.Contains(host, "blob.core.windows.net") || strings.Contains(host, "dfs.core.windows.net") {
			return ProviderAzure, nil
		}
	}

	return "", NewStorageError(
		ErrorCodeUnsupportedProvider,
		fmt.Sprintf("unable to detect provider for URL: %s", urlStr),
		"",
		urlStr,
		nil,
	)
}

// StorageManager provides high-level storage operations across multiple providers
type StorageManager struct {
	registry           ResolverRegistry
	credentialProvider CredentialProvider
}

// NewStorageManager creates a new storage manager
func NewStorageManager(registry ResolverRegistry, credentialProvider CredentialProvider) *StorageManager {
	return &StorageManager{
		registry:           registry,
		credentialProvider: credentialProvider,
	}
}

// ParseURL parses a storage URL using the registry
func (m *StorageManager) ParseURL(urlStr string) (*StorageURL, error) {
	return m.registry.ParseURL(urlStr)
}

// GetFileInfoFromURL retrieves file information from a URL
func (m *StorageManager) GetFileInfoFromURL(ctx context.Context, urlStr string, tenantID uuid.UUID) (*FileInfo, error) {
	storageURL, err := m.ParseURL(urlStr)
	if err != nil {
		return nil, err
	}

	resolver, err := m.registry.GetResolver(storageURL.Provider)
	if err != nil {
		return nil, err
	}

	credentials, err := m.credentialProvider.GetCredentials(ctx, tenantID, storageURL.Provider)
	if err != nil {
		return nil, err
	}

	return resolver.GetFileInfo(ctx, storageURL, credentials)
}

// DownloadFromURL downloads a file from a URL
func (m *StorageManager) DownloadFromURL(ctx context.Context, urlStr string, tenantID uuid.UUID, options *DownloadOptions) (io.ReadCloser, error) {
	storageURL, err := m.ParseURL(urlStr)
	if err != nil {
		return nil, err
	}

	resolver, err := m.registry.GetResolver(storageURL.Provider)
	if err != nil {
		return nil, err
	}

	credentials, err := m.credentialProvider.GetCredentials(ctx, tenantID, storageURL.Provider)
	if err != nil {
		return nil, err
	}

	return resolver.DownloadFile(ctx, storageURL, credentials, options)
}

// ListFromURL lists files from a URL
func (m *StorageManager) ListFromURL(ctx context.Context, urlStr string, tenantID uuid.UUID, options *ListOptions) (*ListResult, error) {
	storageURL, err := m.ParseURL(urlStr)
	if err != nil {
		return nil, err
	}

	resolver, err := m.registry.GetResolver(storageURL.Provider)
	if err != nil {
		return nil, err
	}

	credentials, err := m.credentialProvider.GetCredentials(ctx, tenantID, storageURL.Provider)
	if err != nil {
		return nil, err
	}

	return resolver.ListFiles(ctx, storageURL, credentials, options)
}

// GeneratePresignedURLFromURL generates a presigned URL
func (m *StorageManager) GeneratePresignedURLFromURL(ctx context.Context, urlStr string, tenantID uuid.UUID, options *PresignedURLOptions) (*PresignedURL, error) {
	storageURL, err := m.ParseURL(urlStr)
	if err != nil {
		return nil, err
	}

	resolver, err := m.registry.GetResolver(storageURL.Provider)
	if err != nil {
		return nil, err
	}

	credentials, err := m.credentialProvider.GetCredentials(ctx, tenantID, storageURL.Provider)
	if err != nil {
		return nil, err
	}

	return resolver.GeneratePresignedURL(ctx, storageURL, credentials, options)
}

// ValidateURLAccess validates access to a storage URL
func (m *StorageManager) ValidateURLAccess(ctx context.Context, urlStr string, tenantID uuid.UUID) error {
	storageURL, err := m.ParseURL(urlStr)
	if err != nil {
		return err
	}

	resolver, err := m.registry.GetResolver(storageURL.Provider)
	if err != nil {
		return err
	}

	credentials, err := m.credentialProvider.GetCredentials(ctx, tenantID, storageURL.Provider)
	if err != nil {
		return err
	}

	return resolver.ValidateAccess(ctx, storageURL, credentials)
}

// GetSupportedProviders returns all supported providers
func (m *StorageManager) GetSupportedProviders() []CloudProvider {
	return m.registry.GetSupportedProviders()
}

// BulkDownload downloads multiple files concurrently
func (m *StorageManager) BulkDownload(ctx context.Context, urls []string, tenantID uuid.UUID, options *DownloadOptions) (map[string]DownloadResult, error) {
	results := make(map[string]DownloadResult)
	resultChan := make(chan struct {
		URL    string
		Result DownloadResult
	}, len(urls))

	// Launch download goroutines
	for _, url := range urls {
		go func(u string) {
			reader, err := m.DownloadFromURL(ctx, u, tenantID, options)
			resultChan <- struct {
				URL    string
				Result DownloadResult
			}{
				URL: u,
				Result: DownloadResult{
					Reader: reader,
					Error:  err,
				},
			}
		}(url)
	}

	// Collect results
	for i := 0; i < len(urls); i++ {
		result := <-resultChan
		results[result.URL] = result.Result
	}

	return results, nil
}

// DownloadResult represents the result of a download operation
type DownloadResult struct {
	Reader io.ReadCloser
	Error  error
}

// GetFileInfoBatch retrieves file information for multiple URLs
func (m *StorageManager) GetFileInfoBatch(ctx context.Context, urls []string, tenantID uuid.UUID) (map[string]FileInfoResult, error) {
	results := make(map[string]FileInfoResult)
	resultChan := make(chan struct {
		URL    string
		Result FileInfoResult
	}, len(urls))

	// Launch info retrieval goroutines
	for _, url := range urls {
		go func(u string) {
			info, err := m.GetFileInfoFromURL(ctx, u, tenantID)
			resultChan <- struct {
				URL    string
				Result FileInfoResult
			}{
				URL: u,
				Result: FileInfoResult{
					Info:  info,
					Error: err,
				},
			}
		}(url)
	}

	// Collect results
	for i := 0; i < len(urls); i++ {
		result := <-resultChan
		results[result.URL] = result.Result
	}

	return results, nil
}

// FileInfoResult represents the result of a file info operation
type FileInfoResult struct {
	Info  *FileInfo
	Error error
}

// GetLocalStore returns the local storage interface
func (m *StorageManager) GetLocalStore() LocalStore {
	// For now, return a simple implementation
	// In practice, this should be injected or configured
	return nil // TODO: Implement proper local store
}

// GetConnector returns a connector for the specified type
func (m *StorageManager) GetConnector(connectorType string) (StorageConnector, error) {
	// Convert connector type to provider
	var provider CloudProvider
	switch connectorType {
	case "aws", "s3":
		provider = ProviderAWS
	case "gcp", "gcs":
		provider = ProviderGCP
	case "azure":
		provider = ProviderAzure
	case "local":
		provider = ProviderLocal
	default:
		return nil, NewStorageError(
			ErrorCodeUnsupportedProvider,
			fmt.Sprintf("unsupported connector type: %s", connectorType),
			"",
			"",
			nil,
		)
	}

	_, err := m.registry.GetResolver(provider)
	if err != nil {
		return nil, err
	}

	// For now, we cannot convert resolvers to connectors due to interface conflicts
	// TODO: Implement proper connector registry or adapter pattern
	return nil, NewStorageError(
		ErrorCodeUnsupportedProvider,
		fmt.Sprintf("connector retrieval not yet implemented for %s", connectorType),
		"",
		"",
		nil,
	)
}

// RegisterConnector registers a new connector
func (m *StorageManager) RegisterConnector(connectorType string, connector StorageConnector) error {
	// Convert connector type to provider (not used currently but may be needed for future implementation)
	_ = connectorType
	_ = connector

	// For now, we cannot convert connectors to resolvers due to interface conflicts
	// TODO: Implement proper connector/resolver registration mechanism
	return NewStorageError(
		ErrorCodeUnsupportedProvider,
		fmt.Sprintf("connector registration not yet implemented for %s", connectorType),
		"",
		"",
		nil,
	)
}
