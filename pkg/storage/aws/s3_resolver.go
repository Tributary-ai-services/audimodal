package aws

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// S3Resolver implements StorageResolver for AWS S3
type S3Resolver struct {
	name    string
	version string
}

// NewS3Resolver creates a new S3 storage resolver
func NewS3Resolver() *S3Resolver {
	return &S3Resolver{
		name:    "aws_s3_resolver",
		version: "1.0.0",
	}
}

// ParseURL parses an S3 URL into its components
func (r *S3Resolver) ParseURL(urlStr string) (*storage.StorageURL, error) {
	// Support multiple S3 URL formats:
	// s3://bucket/key
	// https://bucket.s3.region.amazonaws.com/key
	// https://s3.region.amazonaws.com/bucket/key
	// https://bucket.s3.amazonaws.com/key (legacy)

	parsed, err := url.Parse(urlStr)
	if err != nil {
		return nil, storage.NewStorageError(
			storage.ErrorCodeInvalidURL,
			"invalid URL format",
			storage.ProviderAWS,
			urlStr,
			err,
		)
	}

	var bucket, key, region string

	switch parsed.Scheme {
	case "s3":
		// s3://bucket/key
		bucket = parsed.Host
		key = strings.TrimPrefix(parsed.Path, "/")
		region = r.extractRegionFromQuery(parsed.Query())

	case "https":
		// Parse various HTTPS formats
		bucket, key, region = r.parseHTTPSURL(parsed)

	default:
		return nil, storage.NewStorageError(
			storage.ErrorCodeInvalidURL,
			fmt.Sprintf("unsupported URL scheme: %s", parsed.Scheme),
			storage.ProviderAWS,
			urlStr,
			nil,
		)
	}

	if bucket == "" {
		return nil, storage.NewStorageError(
			storage.ErrorCodeInvalidURL,
			"bucket name is required",
			storage.ProviderAWS,
			urlStr,
			nil,
		)
	}

	return &storage.StorageURL{
		Provider: storage.ProviderAWS,
		Bucket:   bucket,
		Key:      key,
		Region:   region,
		RawURL:   urlStr,
	}, nil
}

// GetFileInfo retrieves metadata about an S3 object
func (r *S3Resolver) GetFileInfo(ctx context.Context, storageURL *storage.StorageURL, credentials *storage.Credentials) (*storage.FileInfo, error) {
	client, err := r.createS3Client(ctx, storageURL.Region, credentials)
	if err != nil {
		return nil, err
	}

	// Get object metadata
	headInput := &s3.HeadObjectInput{
		Bucket: aws.String(storageURL.Bucket),
		Key:    aws.String(storageURL.Key),
	}

	headOutput, err := client.HeadObject(ctx, headInput)
	if err != nil {
		return nil, r.handleS3Error(err, storageURL.RawURL)
	}

	// Convert metadata
	metadata := make(map[string]string)
	for k, v := range headOutput.Metadata {
		metadata[k] = v
	}

	// Get object tags if available
	tags := make(map[string]string)
	tagsInput := &s3.GetObjectTaggingInput{
		Bucket: aws.String(storageURL.Bucket),
		Key:    aws.String(storageURL.Key),
	}

	if tagsOutput, err := client.GetObjectTagging(ctx, tagsInput); err == nil {
		for _, tag := range tagsOutput.TagSet {
			if tag.Key != nil && tag.Value != nil {
				tags[*tag.Key] = *tag.Value
			}
		}
	}

	return &storage.FileInfo{
		URL:          storageURL.RawURL,
		Size:         aws.ToInt64(headOutput.ContentLength),
		ContentType:  aws.ToString(headOutput.ContentType),
		LastModified: aws.ToTime(headOutput.LastModified),
		ETag:         strings.Trim(aws.ToString(headOutput.ETag), "\""),
		Checksum:     r.extractChecksum(headOutput),
		ChecksumType: r.getChecksumType(headOutput),
		Metadata:     metadata,
		Tags:         tags,
		StorageClass: string(headOutput.StorageClass),
		Encrypted:    headOutput.ServerSideEncryption != "",
		KMSKeyID:     aws.ToString(headOutput.SSEKMSKeyId),
	}, nil
}

// DownloadFile downloads a file from S3
func (r *S3Resolver) DownloadFile(ctx context.Context, storageURL *storage.StorageURL, credentials *storage.Credentials, options *storage.DownloadOptions) (io.ReadCloser, error) {
	client, err := r.createS3Client(ctx, storageURL.Region, credentials)
	if err != nil {
		return nil, err
	}

	input := &s3.GetObjectInput{
		Bucket: aws.String(storageURL.Bucket),
		Key:    aws.String(storageURL.Key),
	}

	// Add range header if specified
	if options != nil && options.Range != nil {
		rangeHeader := fmt.Sprintf("bytes=%d-%d", options.Range.Start, options.Range.End)
		input.Range = aws.String(rangeHeader)
	}

	// Apply timeout if specified
	if options != nil && options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, options.Timeout)
		defer cancel()
	}

	output, err := client.GetObject(ctx, input)
	if err != nil {
		return nil, r.handleS3Error(err, storageURL.RawURL)
	}

	return output.Body, nil
}

// ListFiles lists objects in an S3 bucket with a prefix
func (r *S3Resolver) ListFiles(ctx context.Context, storageURL *storage.StorageURL, credentials *storage.Credentials, options *storage.ListOptions) (*storage.ListResult, error) {
	client, err := r.createS3Client(ctx, storageURL.Region, credentials)
	if err != nil {
		return nil, err
	}

	prefix := storageURL.Key
	if options != nil && options.Prefix != "" {
		if prefix != "" {
			prefix = prefix + "/" + options.Prefix
		} else {
			prefix = options.Prefix
		}
	}

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(storageURL.Bucket),
		Prefix: aws.String(prefix),
	}

	if options != nil {
		if options.MaxKeys > 0 {
			input.MaxKeys = aws.Int32(int32(options.MaxKeys))
		}
		if options.ContinuationToken != "" {
			input.ContinuationToken = aws.String(options.ContinuationToken)
		}
		if options.Delimiter != "" {
			input.Delimiter = aws.String(options.Delimiter)
		}
	}

	output, err := client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, r.handleS3Error(err, storageURL.RawURL)
	}

	// Convert results
	files := make([]*storage.FileInfo, 0, len(output.Contents))
	for _, obj := range output.Contents {
		fileInfo := &storage.FileInfo{
			URL:          fmt.Sprintf("s3://%s/%s", storageURL.Bucket, *obj.Key),
			Size:         aws.ToInt64(obj.Size),
			LastModified: aws.ToTime(obj.LastModified),
			ETag:         strings.Trim(aws.ToString(obj.ETag), "\""),
			StorageClass: string(obj.StorageClass),
		}

		// Get additional metadata if requested
		if options != nil && options.IncludeMetadata {
			if metadata, err := r.GetFileInfo(ctx, &storage.StorageURL{
				Provider: storage.ProviderAWS,
				Bucket:   storageURL.Bucket,
				Key:      *obj.Key,
				Region:   storageURL.Region,
				RawURL:   fileInfo.URL,
			}, credentials); err == nil {
				fileInfo.ContentType = metadata.ContentType
				fileInfo.Metadata = metadata.Metadata
				fileInfo.Tags = metadata.Tags
				fileInfo.Encrypted = metadata.Encrypted
				fileInfo.KMSKeyID = metadata.KMSKeyID
			}
		}

		files = append(files, fileInfo)
	}

	// Convert common prefixes
	commonPrefixes := make([]string, 0, len(output.CommonPrefixes))
	for _, prefix := range output.CommonPrefixes {
		commonPrefixes = append(commonPrefixes, aws.ToString(prefix.Prefix))
	}

	return &storage.ListResult{
		Files:             files,
		CommonPrefixes:    commonPrefixes,
		IsTruncated:       aws.ToBool(output.IsTruncated),
		ContinuationToken: aws.ToString(output.NextContinuationToken),
		KeyCount:          int(aws.ToInt32(output.KeyCount)),
	}, nil
}

// GeneratePresignedURL generates a presigned URL for S3 object access
func (r *S3Resolver) GeneratePresignedURL(ctx context.Context, storageURL *storage.StorageURL, credentials *storage.Credentials, options *storage.PresignedURLOptions) (*storage.PresignedURL, error) {
	client, err := r.createS3Client(ctx, storageURL.Region, credentials)
	if err != nil {
		return nil, err
	}

	presignClient := s3.NewPresignClient(client)

	var request interface{}
	var presignedURL *url.URL

	switch strings.ToUpper(options.Method) {
	case "GET":
		req := &s3.GetObjectInput{
			Bucket: aws.String(storageURL.Bucket),
			Key:    aws.String(storageURL.Key),
		}
		presignedReq, err := presignClient.PresignGetObject(ctx, req, func(opts *s3.PresignOptions) {
			opts.Expires = options.Expiration
		})
		if err != nil {
			return nil, r.handleS3Error(err, storageURL.RawURL)
		}
		presignedURL, _ = url.Parse(presignedReq.URL)
		request = req

	case "PUT":
		req := &s3.PutObjectInput{
			Bucket: aws.String(storageURL.Bucket),
			Key:    aws.String(storageURL.Key),
		}
		presignedReq, err := presignClient.PresignPutObject(ctx, req, func(opts *s3.PresignOptions) {
			opts.Expires = options.Expiration
		})
		if err != nil {
			return nil, r.handleS3Error(err, storageURL.RawURL)
		}
		presignedURL, _ = url.Parse(presignedReq.URL)
		request = req

	default:
		return nil, storage.NewStorageError(
			storage.ErrorCodeInvalidURL,
			fmt.Sprintf("unsupported HTTP method for presigned URL: %s", options.Method),
			storage.ProviderAWS,
			storageURL.RawURL,
			nil,
		)
	}

	_ = request // Use request if needed for additional processing

	return &storage.PresignedURL{
		URL:       presignedURL.String(),
		Method:    options.Method,
		Headers:   options.Headers,
		ExpiresAt: time.Now().Add(options.Expiration),
	}, nil
}

// ValidateAccess checks if the credentials can access the S3 location
func (r *S3Resolver) ValidateAccess(ctx context.Context, storageURL *storage.StorageURL, credentials *storage.Credentials) error {
	client, err := r.createS3Client(ctx, storageURL.Region, credentials)
	if err != nil {
		return err
	}

	// Test bucket access
	_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(storageURL.Bucket),
	})

	if err != nil {
		return r.handleS3Error(err, storageURL.RawURL)
	}

	// If key is specified, test object access
	if storageURL.Key != "" {
		_, err = client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(storageURL.Bucket),
			Key:    aws.String(storageURL.Key),
		})

		if err != nil {
			return r.handleS3Error(err, storageURL.RawURL)
		}
	}

	return nil
}

// GetSupportedProviders returns the providers this resolver supports
func (r *S3Resolver) GetSupportedProviders() []storage.CloudProvider {
	return []storage.CloudProvider{storage.ProviderAWS}
}

// Helper methods

func (r *S3Resolver) createS3Client(ctx context.Context, region string, creds *storage.Credentials) (*s3.Client, error) {
	var cfg aws.Config
	var err error

	if creds != nil {
		// Use provided credentials
		credProvider := credentials.NewStaticCredentialsProvider(
			creds.AccessKeyID,
			creds.SecretAccessKey,
			creds.SessionToken,
		)

		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithCredentialsProvider(credProvider),
			config.WithRegion(r.getRegion(region, creds.Region)),
		)
	} else {
		// Use default credential chain
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(r.getRegion(region, "")),
		)
	}

	if err != nil {
		return nil, storage.NewStorageError(
			storage.ErrorCodeAuthenticationFailed,
			"failed to load AWS configuration",
			storage.ProviderAWS,
			"",
			err,
		)
	}

	return s3.NewFromConfig(cfg), nil
}

func (r *S3Resolver) getRegion(urlRegion, credRegion string) string {
	if urlRegion != "" {
		return urlRegion
	}
	if credRegion != "" {
		return credRegion
	}
	return "us-east-1" // Default region
}

func (r *S3Resolver) parseHTTPSURL(parsed *url.URL) (bucket, key, region string) {
	// Handle different HTTPS URL formats
	if strings.Contains(parsed.Host, ".s3.") {
		// Virtual hosted-style: https://bucket.s3.region.amazonaws.com/key
		parts := strings.Split(parsed.Host, ".")
		if len(parts) >= 3 && parts[1] == "s3" {
			bucket = parts[0]
			if len(parts) >= 4 && parts[2] != "amazonaws" {
				region = parts[2]
			}
		}
	} else if strings.Contains(parsed.Host, "s3.") {
		// Path-style: https://s3.region.amazonaws.com/bucket/key
		pathParts := strings.SplitN(strings.TrimPrefix(parsed.Path, "/"), "/", 2)
		if len(pathParts) >= 1 {
			bucket = pathParts[0]
			if len(pathParts) >= 2 {
				key = pathParts[1]
			}
		}

		// Extract region from hostname
		hostParts := strings.Split(parsed.Host, ".")
		if len(hostParts) >= 3 && hostParts[0] == "s3" && hostParts[1] != "amazonaws" {
			region = hostParts[1]
		}
	}

	if key == "" {
		key = strings.TrimPrefix(parsed.Path, "/")
	}

	return bucket, key, region
}

func (r *S3Resolver) extractRegionFromQuery(query url.Values) string {
	return query.Get("region")
}

func (r *S3Resolver) extractChecksum(head *s3.HeadObjectOutput) string {
	// Try various checksum headers
	if head.ChecksumSHA256 != nil {
		return *head.ChecksumSHA256
	}
	if head.ChecksumSHA1 != nil {
		return *head.ChecksumSHA1
	}
	if head.ChecksumCRC32 != nil {
		return *head.ChecksumCRC32
	}
	if head.ChecksumCRC32C != nil {
		return *head.ChecksumCRC32C
	}
	// Fallback to ETag if no checksum headers
	return strings.Trim(aws.ToString(head.ETag), "\"")
}

func (r *S3Resolver) getChecksumType(head *s3.HeadObjectOutput) string {
	if head.ChecksumSHA256 != nil {
		return "sha256"
	}
	if head.ChecksumSHA1 != nil {
		return "sha1"
	}
	if head.ChecksumCRC32 != nil {
		return "crc32"
	}
	if head.ChecksumCRC32C != nil {
		return "crc32c"
	}
	return "etag"
}

func (r *S3Resolver) handleS3Error(err error, url string) error {
	// Map S3 errors to storage errors
	var code string
	var message string

	switch {
	case strings.Contains(err.Error(), "NoSuchBucket"):
		code = storage.ErrorCodeFileNotFound
		message = "bucket does not exist"
	case strings.Contains(err.Error(), "NoSuchKey"):
		code = storage.ErrorCodeFileNotFound
		message = "object does not exist"
	case strings.Contains(err.Error(), "AccessDenied"):
		code = storage.ErrorCodeAccessDenied
		message = "access denied"
	case strings.Contains(err.Error(), "InvalidAccessKeyId"):
		code = storage.ErrorCodeInvalidCredentials
		message = "invalid access key"
	case strings.Contains(err.Error(), "SignatureDoesNotMatch"):
		code = storage.ErrorCodeInvalidCredentials
		message = "invalid secret key"
	case strings.Contains(err.Error(), "TokenRefreshRequired"):
		code = storage.ErrorCodeInvalidCredentials
		message = "token refresh required"
	case strings.Contains(err.Error(), "RequestTimeout"):
		code = storage.ErrorCodeNetworkError
		message = "request timeout"
	case strings.Contains(err.Error(), "ServiceUnavailable"):
		code = storage.ErrorCodeServiceUnavailable
		message = "service unavailable"
	default:
		code = storage.ErrorCodeInternalError
		message = "internal error"
	}

	return storage.NewStorageError(code, message, storage.ProviderAWS, url, err)
}
