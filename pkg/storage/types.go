package storage

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"
)

// CloudProvider represents different cloud storage providers
type CloudProvider string

const (
	ProviderAWS   CloudProvider = "aws"
	ProviderGCP   CloudProvider = "gcp"
	ProviderAzure CloudProvider = "azure"
	ProviderLocal CloudProvider = "local"
)

// StorageURL represents a parsed cloud storage URL
type StorageURL struct {
	Provider   CloudProvider `json:"provider"`
	Bucket     string        `json:"bucket"`
	Key        string        `json:"key"`
	Region     string        `json:"region,omitempty"`
	Project    string        `json:"project,omitempty"`
	RawURL     string        `json:"raw_url"`
	Metadata   URLMetadata   `json:"metadata,omitempty"`
}

// URLMetadata contains additional metadata about the storage URL
type URLMetadata struct {
	Version    string            `json:"version,omitempty"`
	Encrypted  bool              `json:"encrypted,omitempty"`
	KMSKeyID   string            `json:"kms_key_id,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
	CustomData map[string]string `json:"custom_data,omitempty"`
}

// FileInfo represents metadata about a file in cloud storage
type FileInfo struct {
	URL          string            `json:"url"`
	Size         int64             `json:"size"`
	ContentType  string            `json:"content_type"`
	LastModified time.Time         `json:"last_modified"`
	ETag         string            `json:"etag"`
	Checksum     string            `json:"checksum"`
	ChecksumType string            `json:"checksum_type"`
	Metadata     map[string]string `json:"metadata"`
	Tags         map[string]string `json:"tags"`
	StorageClass string            `json:"storage_class"`
	Encrypted    bool              `json:"encrypted"`
	KMSKeyID     string            `json:"kms_key_id,omitempty"`
}

// DownloadOptions configures how files are downloaded
type DownloadOptions struct {
	Range       *ByteRange `json:"range,omitempty"`
	BufferSize  int        `json:"buffer_size,omitempty"`
	Timeout     time.Duration `json:"timeout,omitempty"`
	RetryCount  int        `json:"retry_count,omitempty"`
	ChecksumValidation bool `json:"checksum_validation,omitempty"`
}

// ByteRange represents a byte range for partial downloads
type ByteRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// ListOptions configures how directory listings are performed
type ListOptions struct {
	Prefix       string `json:"prefix,omitempty"`
	Delimiter    string `json:"delimiter,omitempty"`
	MaxKeys      int    `json:"max_keys,omitempty"`
	ContinuationToken string `json:"continuation_token,omitempty"`
	Recursive    bool   `json:"recursive,omitempty"`
	IncludeMetadata bool `json:"include_metadata,omitempty"`
}

// ListResult contains the results of a directory listing
type ListResult struct {
	Files             []*FileInfo `json:"files"`
	CommonPrefixes    []string    `json:"common_prefixes,omitempty"`
	IsTruncated       bool        `json:"is_truncated"`
	ContinuationToken string      `json:"continuation_token,omitempty"`
	KeyCount          int         `json:"key_count"`
}

// PresignedURLOptions configures presigned URL generation
type PresignedURLOptions struct {
	Method     string        `json:"method"`
	Expiration time.Duration `json:"expiration"`
	Headers    map[string]string `json:"headers,omitempty"`
}

// PresignedURL contains a presigned URL and its metadata
type PresignedURL struct {
	URL        string            `json:"url"`
	Method     string            `json:"method"`
	Headers    map[string]string `json:"headers,omitempty"`
	ExpiresAt  time.Time         `json:"expires_at"`
}

// CredentialProvider represents different ways to authenticate with cloud storage
type CredentialProvider interface {
	// GetCredentials returns the credentials for the specified tenant and provider
	GetCredentials(ctx context.Context, tenantID uuid.UUID, provider CloudProvider) (*Credentials, error)
	
	// ValidateCredentials checks if credentials are valid
	ValidateCredentials(ctx context.Context, credentials *Credentials) error
	
	// RefreshCredentials refreshes temporary credentials if needed
	RefreshCredentials(ctx context.Context, credentials *Credentials) (*Credentials, error)
}

// Credentials contains authentication information for cloud storage
type Credentials struct {
	Provider        CloudProvider     `json:"provider"`
	AccessKeyID     string            `json:"access_key_id,omitempty"`
	SecretAccessKey string            `json:"secret_access_key,omitempty"`
	SessionToken    string            `json:"session_token,omitempty"`
	Region          string            `json:"region,omitempty"`
	Project         string            `json:"project,omitempty"`
	ServiceAccount  string            `json:"service_account,omitempty"`
	KeyFile         string            `json:"key_file,omitempty"`
	ExpiresAt       *time.Time        `json:"expires_at,omitempty"`
	Extra           map[string]string `json:"extra,omitempty"`
}

// StorageResolver resolves cloud storage URLs and provides access to files
type StorageResolver interface {
	// ParseURL parses a cloud storage URL into its components
	ParseURL(url string) (*StorageURL, error)
	
	// GetFileInfo retrieves metadata about a file
	GetFileInfo(ctx context.Context, storageURL *StorageURL, credentials *Credentials) (*FileInfo, error)
	
	// DownloadFile downloads a file from cloud storage
	DownloadFile(ctx context.Context, storageURL *StorageURL, credentials *Credentials, options *DownloadOptions) (io.ReadCloser, error)
	
	// ListFiles lists files in a directory/prefix
	ListFiles(ctx context.Context, storageURL *StorageURL, credentials *Credentials, options *ListOptions) (*ListResult, error)
	
	// GeneratePresignedURL generates a presigned URL for file access
	GeneratePresignedURL(ctx context.Context, storageURL *StorageURL, credentials *Credentials, options *PresignedURLOptions) (*PresignedURL, error)
	
	// ValidateAccess checks if the credentials can access the storage location
	ValidateAccess(ctx context.Context, storageURL *StorageURL, credentials *Credentials) error
	
	// GetSupportedProviders returns the cloud providers this resolver supports
	GetSupportedProviders() []CloudProvider
}

// ResolverRegistry manages multiple storage resolvers
type ResolverRegistry interface {
	// RegisterResolver registers a resolver for specific providers
	RegisterResolver(providers []CloudProvider, resolver StorageResolver) error
	
	// GetResolver returns the appropriate resolver for a provider
	GetResolver(provider CloudProvider) (StorageResolver, error)
	
	// ParseURL parses any supported storage URL
	ParseURL(url string) (*StorageURL, error)
	
	// GetSupportedProviders returns all supported providers
	GetSupportedProviders() []CloudProvider
}

// StorageError represents errors that occur during storage operations
type StorageError struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Provider CloudProvider `json:"provider"`
	URL      string `json:"url,omitempty"`
	Cause    error  `json:"-"`
}

func (e *StorageError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *StorageError) Unwrap() error {
	return e.Cause
}

// Common error codes
const (
	ErrorCodeInvalidURL        = "INVALID_URL"
	ErrorCodeUnsupportedProvider = "UNSUPPORTED_PROVIDER"
	ErrorCodeAuthenticationFailed = "AUTHENTICATION_FAILED"
	ErrorCodeAccessDenied      = "ACCESS_DENIED"
	ErrorCodeFileNotFound      = "FILE_NOT_FOUND"
	ErrorCodeNetworkError      = "NETWORK_ERROR"
	ErrorCodeInternalError     = "INTERNAL_ERROR"
	ErrorCodeQuotaExceeded     = "QUOTA_EXCEEDED"
	ErrorCodeInvalidCredentials = "INVALID_CREDENTIALS"
	ErrorCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
)

// NewStorageError creates a new storage error
func NewStorageError(code, message string, provider CloudProvider, url string, cause error) *StorageError {
	return &StorageError{
		Code:     code,
		Message:  message,
		Provider: provider,
		URL:      url,
		Cause:    cause,
	}
}