package gcp

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	gcsstorage "cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// GCSResolver implements StorageResolver for Google Cloud Storage
type GCSResolver struct {
	name    string
	version string
}

// NewGCSResolver creates a new GCS storage resolver
func NewGCSResolver() *GCSResolver {
	return &GCSResolver{
		name:    "gcp_gcs_resolver",
		version: "1.0.0",
	}
}

// ParseURL parses a GCS URL into its components
func (r *GCSResolver) ParseURL(urlStr string) (*storage.StorageURL, error) {
	// Support multiple GCS URL formats:
	// gs://bucket/object
	// https://storage.googleapis.com/bucket/object
	// https://storage.cloud.google.com/bucket/object
	// https://bucket.storage.googleapis.com/object

	parsed, err := url.Parse(urlStr)
	if err != nil {
		return nil, storage.NewStorageError(
			storage.ErrorCodeInvalidURL,
			"invalid URL format",
			storage.ProviderGCP,
			urlStr,
			err,
		)
	}

	var bucket, key, project string

	switch parsed.Scheme {
	case "gs":
		// gs://bucket/object
		bucket = parsed.Host
		key = strings.TrimPrefix(parsed.Path, "/")
		project = r.extractProjectFromQuery(parsed.Query())

	case "https":
		// Parse various HTTPS formats
		bucket, key, project = r.parseHTTPSURL(parsed)

	default:
		return nil, storage.NewStorageError(
			storage.ErrorCodeInvalidURL,
			fmt.Sprintf("unsupported URL scheme: %s", parsed.Scheme),
			storage.ProviderGCP,
			urlStr,
			nil,
		)
	}

	if bucket == "" {
		return nil, storage.NewStorageError(
			storage.ErrorCodeInvalidURL,
			"bucket name is required",
			storage.ProviderGCP,
			urlStr,
			nil,
		)
	}

	return &storage.StorageURL{
		Provider: storage.ProviderGCP,
		Bucket:   bucket,
		Key:      key,
		Project:  project,
		RawURL:   urlStr,
	}, nil
}

// GetFileInfo retrieves metadata about a GCS object
func (r *GCSResolver) GetFileInfo(ctx context.Context, storageURL *storage.StorageURL, credentials *storage.Credentials) (*storage.FileInfo, error) {
	client, err := r.createGCSClient(ctx, credentials)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	bucket := client.Bucket(storageURL.Bucket)
	obj := bucket.Object(storageURL.Key)

	attrs, err := obj.Attrs(ctx)
	if err != nil {
		return nil, r.handleGCSError(err, storageURL.RawURL)
	}

	// Convert metadata
	metadata := make(map[string]string)
	for k, v := range attrs.Metadata {
		metadata[k] = v
	}

	// Convert custom attributes
	customData := make(map[string]string)
	if attrs.CustomTime != (time.Time{}) {
		customData["custom_time"] = attrs.CustomTime.Format(time.RFC3339)
	}

	return &storage.FileInfo{
		URL:          storageURL.RawURL,
		Size:         attrs.Size,
		ContentType:  attrs.ContentType,
		LastModified: attrs.Updated,
		ETag:         attrs.Etag,
		Checksum:     r.extractChecksum(attrs),
		ChecksumType: r.getChecksumType(attrs),
		Metadata:     metadata,
		StorageClass: attrs.StorageClass,
		Encrypted:    attrs.KMSKeyName != "",
		KMSKeyID:     attrs.KMSKeyName,
	}, nil
}

// DownloadFile downloads a file from GCS
func (r *GCSResolver) DownloadFile(ctx context.Context, storageURL *storage.StorageURL, credentials *storage.Credentials, options *storage.DownloadOptions) (io.ReadCloser, error) {
	client, err := r.createGCSClient(ctx, credentials)
	if err != nil {
		return nil, err
	}

	bucket := client.Bucket(storageURL.Bucket)
	obj := bucket.Object(storageURL.Key)

	// Apply timeout if specified
	if options != nil && options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, options.Timeout)
		defer cancel()
	}

	reader, err := obj.NewReader(ctx)
	if err != nil {
		client.Close()
		return nil, r.handleGCSError(err, storageURL.RawURL)
	}

	// Apply range if specified
	if options != nil && options.Range != nil {
		reader, err = obj.NewRangeReader(ctx, options.Range.Start, options.Range.End-options.Range.Start+1)
		if err != nil {
			client.Close()
			return nil, r.handleGCSError(err, storageURL.RawURL)
		}
	}

	// Return a reader that closes both the object reader and client
	return &gcsReaderCloser{
		Reader: reader,
		client: client,
	}, nil
}

// ListFiles lists objects in a GCS bucket with a prefix
func (r *GCSResolver) ListFiles(ctx context.Context, storageURL *storage.StorageURL, credentials *storage.Credentials, options *storage.ListOptions) (*storage.ListResult, error) {
	client, err := r.createGCSClient(ctx, credentials)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	bucket := client.Bucket(storageURL.Bucket)

	prefix := storageURL.Key
	if options != nil && options.Prefix != "" {
		if prefix != "" {
			prefix = prefix + "/" + options.Prefix
		} else {
			prefix = options.Prefix
		}
	}

	query := &gcsstorage.Query{
		Prefix: prefix,
	}

	if options != nil {
		if options.Delimiter != "" {
			query.Delimiter = options.Delimiter
		}
		if options.ContinuationToken != "" {
			query.StartOffset = options.ContinuationToken
		}
	}

	it := bucket.Objects(ctx, query)

	var files []*storage.FileInfo
	var commonPrefixes []string
	keyCount := 0
	maxKeys := 1000 // Default limit

	if options != nil && options.MaxKeys > 0 {
		maxKeys = options.MaxKeys
	}

	for keyCount < maxKeys {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, r.handleGCSError(err, storageURL.RawURL)
		}

		// Handle directory prefixes
		if attrs.Prefix != "" {
			commonPrefixes = append(commonPrefixes, attrs.Prefix)
			continue
		}

		fileInfo := &storage.FileInfo{
			URL:          fmt.Sprintf("gs://%s/%s", storageURL.Bucket, attrs.Name),
			Size:         attrs.Size,
			ContentType:  attrs.ContentType,
			LastModified: attrs.Updated,
			ETag:         attrs.Etag,
			Checksum:     r.extractChecksum(attrs),
			ChecksumType: r.getChecksumType(attrs),
			StorageClass: attrs.StorageClass,
			Encrypted:    attrs.KMSKeyName != "",
			KMSKeyID:     attrs.KMSKeyName,
		}

		// Get additional metadata if requested
		if options != nil && options.IncludeMetadata {
			metadata := make(map[string]string)
			for k, v := range attrs.Metadata {
				metadata[k] = v
			}
			fileInfo.Metadata = metadata
		}

		files = append(files, fileInfo)
		keyCount++
	}

	// Check if there are more results
	isTruncated := false
	nextToken := ""
	if keyCount == maxKeys {
		// Try to get one more to see if there are more results
		if attrs, err := it.Next(); err != iterator.Done {
			isTruncated = true
			if attrs != nil {
				nextToken = attrs.Name
			}
		}
	}

	return &storage.ListResult{
		Files:             files,
		CommonPrefixes:    commonPrefixes,
		IsTruncated:       isTruncated,
		ContinuationToken: nextToken,
		KeyCount:          keyCount,
	}, nil
}

// GeneratePresignedURL generates a presigned URL for GCS object access
func (r *GCSResolver) GeneratePresignedURL(ctx context.Context, storageURL *storage.StorageURL, credentials *storage.Credentials, options *storage.PresignedURLOptions) (*storage.PresignedURL, error) {
	client, err := r.createGCSClient(ctx, credentials)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	bucket := client.Bucket(storageURL.Bucket)

	method := strings.ToUpper(options.Method)
	if method != "GET" && method != "PUT" && method != "POST" {
		return nil, storage.NewStorageError(
			storage.ErrorCodeInvalidURL,
			fmt.Sprintf("unsupported HTTP method for presigned URL: %s", options.Method),
			storage.ProviderGCP,
			storageURL.RawURL,
			nil,
		)
	}

	opts := &gcsstorage.SignedURLOptions{
		Scheme:  gcsstorage.SigningSchemeV4,
		Method:  method,
		Expires: time.Now().Add(options.Expiration),
	}

	// Add custom headers if provided
	if options.Headers != nil {
		opts.Headers = []string{}
		for k, v := range options.Headers {
			opts.Headers = append(opts.Headers, fmt.Sprintf("%s:%s", k, v))
		}
	}

	// Use service account key if available
	if credentials != nil && credentials.KeyFile != "" {
		opts.GoogleAccessID = credentials.ServiceAccount
		if keyData := []byte(credentials.KeyFile); len(keyData) > 0 {
			opts.PrivateKey = keyData
		}
	}

	signedURL, err := bucket.SignedURL(storageURL.Key, opts)
	if err != nil {
		return nil, r.handleGCSError(err, storageURL.RawURL)
	}

	return &storage.PresignedURL{
		URL:       signedURL,
		Method:    options.Method,
		Headers:   options.Headers,
		ExpiresAt: opts.Expires,
	}, nil
}

// ValidateAccess checks if the credentials can access the GCS location
func (r *GCSResolver) ValidateAccess(ctx context.Context, storageURL *storage.StorageURL, credentials *storage.Credentials) error {
	client, err := r.createGCSClient(ctx, credentials)
	if err != nil {
		return err
	}
	defer client.Close()

	bucket := client.Bucket(storageURL.Bucket)

	// Test bucket access
	_, err = bucket.Attrs(ctx)
	if err != nil {
		return r.handleGCSError(err, storageURL.RawURL)
	}

	// If object key is specified, test object access
	if storageURL.Key != "" {
		obj := bucket.Object(storageURL.Key)
		_, err = obj.Attrs(ctx)
		if err != nil {
			return r.handleGCSError(err, storageURL.RawURL)
		}
	}

	return nil
}

// GetSupportedProviders returns the providers this resolver supports
func (r *GCSResolver) GetSupportedProviders() []storage.CloudProvider {
	return []storage.CloudProvider{storage.ProviderGCP}
}

// Helper methods

func (r *GCSResolver) createGCSClient(ctx context.Context, creds *storage.Credentials) (*gcsstorage.Client, error) {
	var opts []option.ClientOption

	if creds != nil {
		if creds.KeyFile != "" {
			// Use service account key file
			opts = append(opts, option.WithCredentialsJSON([]byte(creds.KeyFile)))
		} else if creds.ServiceAccount != "" {
			// Use service account email for ADC
			opts = append(opts, option.WithServiceAccountFile(creds.ServiceAccount))
		}

		if creds.Project != "" {
			// Set project ID if provided
			opts = append(opts, option.WithQuotaProject(creds.Project))
		}
	}

	client, err := gcsstorage.NewClient(ctx, opts...)
	if err != nil {
		return nil, storage.NewStorageError(
			storage.ErrorCodeAuthenticationFailed,
			"failed to create GCS client",
			storage.ProviderGCP,
			"",
			err,
		)
	}

	return client, nil
}

func (r *GCSResolver) parseHTTPSURL(parsed *url.URL) (bucket, key, project string) {
	// Handle different HTTPS URL formats
	if strings.Contains(parsed.Host, ".storage.googleapis.com") {
		// Virtual hosted-style: https://bucket.storage.googleapis.com/object
		parts := strings.Split(parsed.Host, ".")
		if len(parts) >= 3 && parts[1] == "storage" {
			bucket = parts[0]
		}
		key = strings.TrimPrefix(parsed.Path, "/")
	} else if strings.Contains(parsed.Host, "storage.googleapis.com") || strings.Contains(parsed.Host, "storage.cloud.google.com") {
		// Path-style: https://storage.googleapis.com/bucket/object
		pathParts := strings.SplitN(strings.TrimPrefix(parsed.Path, "/"), "/", 2)
		if len(pathParts) >= 1 {
			bucket = pathParts[0]
			if len(pathParts) >= 2 {
				key = pathParts[1]
			}
		}
	}

	return bucket, key, project
}

func (r *GCSResolver) extractProjectFromQuery(query url.Values) string {
	return query.Get("project")
}

func (r *GCSResolver) extractChecksum(attrs *gcsstorage.ObjectAttrs) string {
	// GCS uses CRC32C by default
	if attrs.CRC32C != 0 {
		return fmt.Sprintf("%d", attrs.CRC32C)
	}
	if len(attrs.MD5) > 0 {
		return fmt.Sprintf("%x", attrs.MD5)
	}
	return attrs.Etag
}

func (r *GCSResolver) getChecksumType(attrs *gcsstorage.ObjectAttrs) string {
	if attrs.CRC32C != 0 {
		return "crc32c"
	}
	if len(attrs.MD5) > 0 {
		return "md5"
	}
	return "etag"
}

func (r *GCSResolver) handleGCSError(err error, url string) error {
	// Map GCS errors to storage errors
	var code string
	var message string

	switch {
	case strings.Contains(err.Error(), "object doesn't exist"):
		code = storage.ErrorCodeFileNotFound
		message = "object does not exist"
	case strings.Contains(err.Error(), "bucket doesn't exist"):
		code = storage.ErrorCodeFileNotFound
		message = "bucket does not exist"
	case strings.Contains(err.Error(), "access denied") || strings.Contains(err.Error(), "forbidden"):
		code = storage.ErrorCodeAccessDenied
		message = "access denied"
	case strings.Contains(err.Error(), "invalid credentials"):
		code = storage.ErrorCodeInvalidCredentials
		message = "invalid credentials"
	case strings.Contains(err.Error(), "timeout"):
		code = storage.ErrorCodeNetworkError
		message = "request timeout"
	case strings.Contains(err.Error(), "service unavailable"):
		code = storage.ErrorCodeServiceUnavailable
		message = "service unavailable"
	case strings.Contains(err.Error(), "quota exceeded"):
		code = storage.ErrorCodeQuotaExceeded
		message = "quota exceeded"
	default:
		code = storage.ErrorCodeInternalError
		message = "internal error"
	}

	return storage.NewStorageError(code, message, storage.ProviderGCP, url, err)
}

// gcsReaderCloser wraps a GCS reader and ensures the client is closed
type gcsReaderCloser struct {
	io.Reader
	client *gcsstorage.Client
}

func (r *gcsReaderCloser) Close() error {
	// Close both the reader and client
	if closer, ok := r.Reader.(io.Closer); ok {
		closer.Close()
	}
	return r.client.Close()
}
