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

// Extended types for connectors

// FileType represents different types of files
type FileType string

const (
	FileTypeDocument     FileType = "document"
	FileTypeSpreadsheet  FileType = "spreadsheet"
	FileTypePresentation FileType = "presentation"
	FileTypePDF          FileType = "pdf"
	FileTypeImage        FileType = "image"
	FileTypeVideo        FileType = "video"
	FileTypeAudio        FileType = "audio"
	FileTypeArchive      FileType = "archive"
	FileTypeOther        FileType = "other"
)

// FilePermissions represents file access permissions
type FilePermissions struct {
	CanRead   bool `json:"can_read"`
	CanWrite  bool `json:"can_write"`
	CanDelete bool `json:"can_delete"`
	CanShare  bool `json:"can_share"`
}

// ConnectorFileInfo represents extended file info for connectors
type ConnectorFileInfo struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Path         string                 `json:"path"`
	URL          string                 `json:"url"`
	DownloadURL  string                 `json:"download_url,omitempty"`
	Size         int64                  `json:"size"`
	Type         FileType               `json:"type"`
	MimeType     string                 `json:"mime_type"`
	CreatedAt    time.Time              `json:"created_at"`
	ModifiedAt   time.Time              `json:"modified_at"`
	LastAccessed time.Time              `json:"last_accessed"`
	IsFolder     bool                   `json:"is_folder"`
	Permissions  FilePermissions        `json:"permissions"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	Tags         map[string]string      `json:"tags,omitempty"`
	Source       string                 `json:"source"`
	Checksum     string                 `json:"checksum,omitempty"`
	ChecksumType string                 `json:"checksum_type,omitempty"`
}

// ConnectorListOptions for file listing (extended)
type ConnectorListOptions struct {
	Prefix          string    `json:"prefix,omitempty"`
	MaxResults      int       `json:"max_results,omitempty"`
	PageToken       string    `json:"page_token,omitempty"`
	Recursive       bool      `json:"recursive,omitempty"`
	IncludeMetadata bool      `json:"include_metadata,omitempty"`
	ModifiedSince   time.Time `json:"modified_since,omitempty"`
	MaxSize         int64     `json:"max_size,omitempty"`
	FileTypes       []string  `json:"file_types,omitempty"`
}

// SyncOptions for synchronization
type SyncOptions struct {
	FullSync     bool      `json:"full_sync"`
	Since        time.Time `json:"since"`
	Paths        []string  `json:"paths,omitempty"`
	DryRun       bool      `json:"dry_run"`
	MaxFiles     int64     `json:"max_files,omitempty"`
	MaxSize      int64     `json:"max_size,omitempty"`
}

// SyncResult contains synchronization results
type SyncResult struct {
	StartTime     time.Time                    `json:"start_time"`
	EndTime       time.Time                    `json:"end_time"`
	Duration      time.Duration                `json:"duration"`
	SyncType      string                       `json:"sync_type"` // full, incremental
	TotalFiles    int                          `json:"total_files"`
	SyncedFiles   int                          `json:"synced_files"`
	FailedFiles   int                          `json:"failed_files"`
	Files         map[string]*SyncFileResult   `json:"files"`
	Error         string                       `json:"error,omitempty"`
}

// SyncFileResult contains results for a single file sync
type SyncFileResult struct {
	Path           string        `json:"path"`
	StartTime      time.Time     `json:"start_time"`
	EndTime        time.Time     `json:"end_time"`
	Duration       time.Duration `json:"duration"`
	Status         string        `json:"status"` // completed, failed, skipped
	ProcessedBytes int64         `json:"processed_bytes"`
	Error          string        `json:"error,omitempty"`
}

// SyncState tracks synchronization state
type SyncState struct {
	IsRunning        bool          `json:"is_running"`
	LastSyncStart    time.Time     `json:"last_sync_start"`
	LastSyncEnd      time.Time     `json:"last_sync_end"`
	LastSyncDuration time.Duration `json:"last_sync_duration"`
	TotalFiles       int64         `json:"total_files"`
	ChangedFiles     int64         `json:"changed_files"`
	ErrorCount       int64         `json:"error_count"`
	LastError        string        `json:"last_error,omitempty"`
}

// ConnectorMetrics tracks connector performance
type ConnectorMetrics struct {
	ConnectorType      string        `json:"connector_type"`
	IsConnected        bool          `json:"is_connected"`
	LastConnectionTime time.Time     `json:"last_connection_time"`
	FilesListed        int64         `json:"files_listed"`
	FilesRetrieved     int64         `json:"files_retrieved"`
	FilesDownloaded    int64         `json:"files_downloaded"`
	BytesDownloaded    int64         `json:"bytes_downloaded"`
	SyncCount          int64         `json:"sync_count"`
	LastSyncTime       time.Time     `json:"last_sync_time"`
	LastSyncDuration   time.Duration `json:"last_sync_duration"`
	ErrorCount         int64         `json:"error_count"`
	LastError          string        `json:"last_error,omitempty"`
}

// StorageConnector interface for external storage systems
type StorageConnector interface {
	// Connect establishes connection to the storage system
	Connect(ctx context.Context, credentials map[string]interface{}) error
	
	// Disconnect closes the connection
	Disconnect(ctx context.Context) error
	
	// IsConnected returns connection status
	IsConnected() bool
	
	// ListFiles lists files in the storage system
	ListFiles(ctx context.Context, path string, options *ConnectorListOptions) ([]*ConnectorFileInfo, error)
	
	// GetFile retrieves file metadata
	GetFile(ctx context.Context, fileID string) (*ConnectorFileInfo, error)
	
	// DownloadFile downloads file content
	DownloadFile(ctx context.Context, fileID string) (io.ReadCloser, error)
	
	// SyncFiles performs synchronization
	SyncFiles(ctx context.Context, options *SyncOptions) (*SyncResult, error)
	
	// GetSyncState returns current sync state
	GetSyncState(ctx context.Context) (*SyncState, error)
	
	// GetMetrics returns connector metrics
	GetMetrics(ctx context.Context) (*ConnectorMetrics, error)
}

// LocalStore interface for local storage operations
type LocalStore interface {
	// SaveFile saves file content to local storage
	SaveFile(ctx context.Context, path string, reader io.Reader) error
	
	// LoadFile loads file content from local storage
	LoadFile(ctx context.Context, path string) (io.ReadCloser, error)
	
	// DeleteFile deletes a file from local storage
	DeleteFile(ctx context.Context, path string) error
	
	// MoveFile moves a file to a new location
	MoveFile(ctx context.Context, oldPath, newPath string) error
	
	// GetFileInfo gets file metadata
	GetFileInfo(ctx context.Context, path string) (*ConnectorFileInfo, error)
	
	// ListFiles lists files in a directory
	ListFiles(ctx context.Context, path string, options *ConnectorListOptions) ([]*ConnectorFileInfo, error)
}

// ConnectorStorageManager interface for managing storage operations
type ConnectorStorageManager interface {
	// GetLocalStore returns the local storage interface
	GetLocalStore() LocalStore
	
	// GetConnector returns a connector for the specified type
	GetConnector(connectorType string) (StorageConnector, error)
	
	// RegisterConnector registers a new connector
	RegisterConnector(connectorType string, connector StorageConnector) error
}