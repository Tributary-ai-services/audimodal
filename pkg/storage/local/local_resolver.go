package local

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// LocalResolver implements StorageResolver for local file system
type LocalResolver struct {
	name       string
	version    string
	basePath   string // Base path for security (optional)
	allowedDirs []string // Allowed directories for security
}

// NewLocalResolver creates a new local file system resolver
func NewLocalResolver(basePath string, allowedDirs []string) *LocalResolver {
	return &LocalResolver{
		name:        "local_file_resolver",
		version:     "1.0.0",
		basePath:    basePath,
		allowedDirs: allowedDirs,
	}
}

// ParseURL parses a local file URL into its components
func (r *LocalResolver) ParseURL(urlStr string) (*storage.StorageURL, error) {
	// Support file:// URLs and absolute paths
	var path string

	if strings.HasPrefix(urlStr, "file://") {
		parsed, err := url.Parse(urlStr)
		if err != nil {
			return nil, storage.NewStorageError(
				storage.ErrorCodeInvalidURL,
				"invalid file URL format",
				storage.ProviderLocal,
				urlStr,
				err,
			)
		}
		path = parsed.Path
	} else if filepath.IsAbs(urlStr) {
		// Absolute path
		path = urlStr
	} else {
		return nil, storage.NewStorageError(
			storage.ErrorCodeInvalidURL,
			"local paths must be absolute or file:// URLs",
			storage.ProviderLocal,
			urlStr,
			nil,
		)
	}

	// Clean the path
	path = filepath.Clean(path)

	// Security check: ensure path is within allowed directories
	if err := r.validatePath(path); err != nil {
		return nil, err
	}

	// Extract directory and filename
	dir := filepath.Dir(path)
	filename := filepath.Base(path)

	return &storage.StorageURL{
		Provider: storage.ProviderLocal,
		Bucket:   dir,
		Key:      filename,
		RawURL:   urlStr,
	}, nil
}

// GetFileInfo retrieves metadata about a local file
func (r *LocalResolver) GetFileInfo(ctx context.Context, storageURL *storage.StorageURL, credentials *storage.Credentials) (*storage.FileInfo, error) {
	fullPath := filepath.Join(storageURL.Bucket, storageURL.Key)

	// Security check
	if err := r.validatePath(fullPath); err != nil {
		return nil, err
	}

	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		return nil, r.handleLocalError(err, storageURL.RawURL)
	}

	if fileInfo.IsDir() {
		return nil, storage.NewStorageError(
			storage.ErrorCodeInvalidURL,
			"path is a directory, not a file",
			storage.ProviderLocal,
			storageURL.RawURL,
			nil,
		)
	}

	// Calculate checksum if file is not too large
	var checksum, checksumType string
	if fileInfo.Size() < 100*1024*1024 { // 100MB limit for checksum calculation
		if hash, err := r.calculateMD5(fullPath); err == nil {
			checksum = hash
			checksumType = "md5"
		}
	}

	return &storage.FileInfo{
		URL:          storageURL.RawURL,
		Size:         fileInfo.Size(),
		ContentType:  r.detectContentType(fullPath),
		LastModified: fileInfo.ModTime(),
		ETag:         fmt.Sprintf("%x-%x", fileInfo.ModTime().Unix(), fileInfo.Size()),
		Checksum:     checksum,
		ChecksumType: checksumType,
		Metadata:     make(map[string]string),
		Tags:         make(map[string]string),
		StorageClass: "standard",
		Encrypted:    false,
	}, nil
}

// DownloadFile opens a local file for reading
func (r *LocalResolver) DownloadFile(ctx context.Context, storageURL *storage.StorageURL, credentials *storage.Credentials, options *storage.DownloadOptions) (io.ReadCloser, error) {
	fullPath := filepath.Join(storageURL.Bucket, storageURL.Key)

	// Security check
	if err := r.validatePath(fullPath); err != nil {
		return nil, err
	}

	file, err := os.Open(fullPath)
	if err != nil {
		return nil, r.handleLocalError(err, storageURL.RawURL)
	}

	// Handle range requests
	if options != nil && options.Range != nil {
		_, err := file.Seek(options.Range.Start, io.SeekStart)
		if err != nil {
			file.Close()
			return nil, storage.NewStorageError(
				storage.ErrorCodeInternalError,
				"failed to seek to range start",
				storage.ProviderLocal,
				storageURL.RawURL,
				err,
			)
		}

		// Wrap with limited reader for range end
		rangeSize := options.Range.End - options.Range.Start + 1
		return &limitedReadCloser{
			Reader: io.LimitReader(file, rangeSize),
			Closer: file,
		}, nil
	}

	return file, nil
}

// ListFiles lists files in a local directory
func (r *LocalResolver) ListFiles(ctx context.Context, storageURL *storage.StorageURL, credentials *storage.Credentials, options *storage.ListOptions) (*storage.ListResult, error) {
	dirPath := storageURL.Bucket
	if storageURL.Key != "" && storageURL.Key != "." {
		dirPath = filepath.Join(dirPath, storageURL.Key)
	}

	// Apply prefix if specified
	if options != nil && options.Prefix != "" {
		dirPath = filepath.Join(dirPath, options.Prefix)
	}

	// Security check
	if err := r.validatePath(dirPath); err != nil {
		return nil, err
	}

	var files []*storage.FileInfo
	var commonPrefixes []string
	keyCount := 0
	maxKeys := 1000

	if options != nil && options.MaxKeys > 0 {
		maxKeys = options.MaxKeys
	}

	// Walk directory
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors and continue
		}

		if keyCount >= maxKeys {
			return filepath.SkipDir
		}

		// Calculate relative path
		relPath, err := filepath.Rel(storageURL.Bucket, path)
		if err != nil {
			return nil
		}

		// Handle delimiter (directory grouping)
		if options != nil && options.Delimiter != "" {
			parts := strings.Split(relPath, options.Delimiter)
			if len(parts) > 1 {
				prefix := strings.Join(parts[:len(parts)-1], options.Delimiter) + options.Delimiter
				commonPrefixes = append(commonPrefixes, prefix)
				return filepath.SkipDir
			}
		}

		if info.IsDir() {
			if options == nil || !options.Recursive {
				return filepath.SkipDir
			}
			return nil
		}

		// Create file info
		fileURL := "file://" + path
		fileInfo := &storage.FileInfo{
			URL:          fileURL,
			Size:         info.Size(),
			ContentType:  r.detectContentType(path),
			LastModified: info.ModTime(),
			ETag:         fmt.Sprintf("%x-%x", info.ModTime().Unix(), info.Size()),
			Metadata:     make(map[string]string),
			Tags:         make(map[string]string),
			StorageClass: "standard",
			Encrypted:    false,
		}

		// Add metadata if requested
		if options != nil && options.IncludeMetadata {
			if checksum, err := r.calculateMD5(path); err == nil {
				fileInfo.Checksum = checksum
				fileInfo.ChecksumType = "md5"
			}
		}

		files = append(files, fileInfo)
		keyCount++

		return nil
	})

	if err != nil {
		return nil, r.handleLocalError(err, storageURL.RawURL)
	}

	// Remove duplicates from common prefixes
	uniquePrefixes := make([]string, 0, len(commonPrefixes))
	seen := make(map[string]bool)
	for _, prefix := range commonPrefixes {
		if !seen[prefix] {
			uniquePrefixes = append(uniquePrefixes, prefix)
			seen[prefix] = true
		}
	}

	return &storage.ListResult{
		Files:             files,
		CommonPrefixes:    uniquePrefixes,
		IsTruncated:       keyCount >= maxKeys,
		ContinuationToken: "",
		KeyCount:          keyCount,
	}, nil
}

// GeneratePresignedURL returns the file path for local files (no presigning needed)
func (r *LocalResolver) GeneratePresignedURL(ctx context.Context, storageURL *storage.StorageURL, credentials *storage.Credentials, options *storage.PresignedURLOptions) (*storage.PresignedURL, error) {
	// For local files, we just return the file path
	fullPath := filepath.Join(storageURL.Bucket, storageURL.Key)

	// Security check
	if err := r.validatePath(fullPath); err != nil {
		return nil, err
	}

	// Check if file exists
	if _, err := os.Stat(fullPath); err != nil {
		return nil, r.handleLocalError(err, storageURL.RawURL)
	}

	return &storage.PresignedURL{
		URL:       "file://" + fullPath,
		Method:    options.Method,
		Headers:   options.Headers,
		ExpiresAt: time.Now().Add(options.Expiration),
	}, nil
}

// ValidateAccess checks if the local path is accessible
func (r *LocalResolver) ValidateAccess(ctx context.Context, storageURL *storage.StorageURL, credentials *storage.Credentials) error {
	fullPath := filepath.Join(storageURL.Bucket, storageURL.Key)

	// Security check
	if err := r.validatePath(fullPath); err != nil {
		return err
	}

	// Check if path exists and is accessible
	_, err := os.Stat(fullPath)
	if err != nil {
		return r.handleLocalError(err, storageURL.RawURL)
	}

	return nil
}

// GetSupportedProviders returns the providers this resolver supports
func (r *LocalResolver) GetSupportedProviders() []storage.CloudProvider {
	return []storage.CloudProvider{storage.ProviderLocal}
}

// Helper methods

func (r *LocalResolver) validatePath(path string) error {
	// Clean and resolve the path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return storage.NewStorageError(
			storage.ErrorCodeInvalidURL,
			"invalid file path",
			storage.ProviderLocal,
			path,
			err,
		)
	}

	// Check base path restriction
	if r.basePath != "" {
		baseAbs, err := filepath.Abs(r.basePath)
		if err != nil {
			return storage.NewStorageError(
				storage.ErrorCodeInternalError,
				"invalid base path configuration",
				storage.ProviderLocal,
				path,
				err,
			)
		}

		if !strings.HasPrefix(absPath, baseAbs) {
			return storage.NewStorageError(
				storage.ErrorCodeAccessDenied,
				"path outside allowed base directory",
				storage.ProviderLocal,
				path,
				nil,
			)
		}
	}

	// Check allowed directories
	if len(r.allowedDirs) > 0 {
		allowed := false
		for _, allowedDir := range r.allowedDirs {
			allowedAbs, err := filepath.Abs(allowedDir)
			if err != nil {
				continue
			}
			if strings.HasPrefix(absPath, allowedAbs) {
				allowed = true
				break
			}
		}

		if !allowed {
			return storage.NewStorageError(
				storage.ErrorCodeAccessDenied,
				"path not in allowed directories",
				storage.ProviderLocal,
				path,
				nil,
			)
		}
	}

	return nil
}

func (r *LocalResolver) detectContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".csv":
		return "text/csv"
	case ".pdf":
		return "application/pdf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xls":
		return "application/vnd.ms-excel"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".xml":
		return "application/xml"
	case ".html", ".htm":
		return "text/html"
	default:
		return "application/octet-stream"
	}
}

func (r *LocalResolver) calculateMD5(path string) (string, error) {
	// This is a placeholder - in a real implementation you'd calculate MD5
	// For now, we'll use a simple hash of the file stats
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", info.ModTime().Unix()+info.Size()), nil
}

func (r *LocalResolver) handleLocalError(err error, url string) error {
	var code string
	var message string

	if os.IsNotExist(err) {
		code = storage.ErrorCodeFileNotFound
		message = "file or directory does not exist"
	} else if os.IsPermission(err) {
		code = storage.ErrorCodeAccessDenied
		message = "permission denied"
	} else {
		code = storage.ErrorCodeInternalError
		message = "file system error"
	}

	return storage.NewStorageError(code, message, storage.ProviderLocal, url, err)
}

// limitedReadCloser wraps a limited reader with a closer
type limitedReadCloser struct {
	io.Reader
	io.Closer
}

func (r *limitedReadCloser) Read(p []byte) (n int, err error) {
	return r.Reader.Read(p)
}

func (r *limitedReadCloser) Close() error {
	return r.Closer.Close()
}