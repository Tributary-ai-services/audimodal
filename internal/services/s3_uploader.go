package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Uploader handles S3 operations for file and chunk storage
type S3Uploader struct {
	client   *s3.Client
	endpoint string
}

// NewS3Uploader creates a new S3 uploader from environment variables
func NewS3Uploader() (*S3Uploader, error) {
	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	region := os.Getenv("AWS_REGION")

	if endpoint == "" {
		endpoint = "http://minio-shared:9000"
	}
	if region == "" {
		region = "us-east-1"
	}
	if accessKey == "" {
		accessKey = "minioadmin"
	}
	if secretKey == "" {
		secretKey = "minioadmin123"
	}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	return &S3Uploader{
		client:   client,
		endpoint: endpoint,
	}, nil
}

// EnsureBucket creates a bucket if it doesn't exist
func (u *S3Uploader) EnsureBucket(ctx context.Context, bucketName string) error {
	_, err := u.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err == nil {
		return nil // Bucket exists
	}

	_, err = u.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		// Ignore BucketAlreadyOwnedByYou (race condition)
		if strings.Contains(err.Error(), "BucketAlreadyOwnedByYou") || strings.Contains(err.Error(), "BucketAlreadyExists") {
			return nil
		}
		return fmt.Errorf("failed to create bucket %s: %w", bucketName, err)
	}

	log.Printf("[S3Uploader] Created bucket: %s", bucketName)
	return nil
}

// UploadFile uploads a file from a reader to S3
func (u *S3Uploader) UploadFile(ctx context.Context, bucket, key string, reader io.ReadSeeker, size int64) error {
	if err := u.EnsureBucket(ctx, bucket); err != nil {
		return err
	}

	_, err := u.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          reader,
		ContentLength: aws.Int64(size),
	})
	if err != nil {
		return fmt.Errorf("failed to upload %s/%s: %w", bucket, key, err)
	}

	log.Printf("[S3Uploader] Uploaded %s/%s (%d bytes)", bucket, key, size)
	return nil
}

// UploadChunkContent uploads chunk text content to S3
func (u *S3Uploader) UploadChunkContent(ctx context.Context, bucket, key, content string) error {
	if err := u.EnsureBucket(ctx, bucket); err != nil {
		return err
	}

	reader := bytes.NewReader([]byte(content))
	_, err := u.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        reader,
		ContentType: aws.String("text/plain; charset=utf-8"),
	})
	if err != nil {
		return fmt.Errorf("failed to upload chunk %s/%s: %w", bucket, key, err)
	}

	return nil
}

// GetChunkContent reads chunk text content from S3
func (u *S3Uploader) GetChunkContent(ctx context.Context, bucket, key string) (string, error) {
	resp, err := u.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get chunk %s/%s: %w", bucket, key, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read chunk content: %w", err)
	}

	return string(data), nil
}

// DownloadFile downloads a file from S3 to a local path
func (u *S3Uploader) DownloadFile(ctx context.Context, bucket, key, localPath string) error {
	resp, err := u.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to download %s/%s: %w", bucket, key, err)
	}
	defer resp.Body.Close()

	f, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file %s: %w", localPath, err)
	}
	defer f.Close()

	if _, err = io.Copy(f, resp.Body); err != nil {
		os.Remove(localPath)
		return fmt.Errorf("failed to write to local file: %w", err)
	}

	return nil
}

// FileExists checks if an object exists in S3
func (u *S3Uploader) FileExists(ctx context.Context, bucket, key string) (bool, error) {
	_, err := u.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *s3types.NoSuchKey
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "NoSuchKey") {
			return false, nil
		}
		_ = nsk
		return false, err
	}
	return true, nil
}

// GetTenantBucket returns the upload bucket name for a tenant
func GetTenantBucket(tenantShortID string) string {
	return fmt.Sprintf("upload-%s", tenantShortID)
}

// GetFileKey returns the S3 key for an uploaded file
func GetFileKey(fileID, filename string) string {
	return fmt.Sprintf("%s/%s", fileID, filename)
}

// GetChunkKey returns the S3 key for a chunk's content
func GetChunkKey(fileID, chunkID string) string {
	return fmt.Sprintf("%s/chunks/%s.txt", fileID, chunkID)
}

// GetTenantShortID extracts a short ID from a tenant name or ID
// Uses the first segment of the tenant name, or the first 10 chars of UUID
func GetTenantShortID(tenantName string) string {
	// If it looks like a UUID, use first 10 chars
	if len(tenantName) >= 36 && tenantName[8] == '-' {
		return strings.ReplaceAll(tenantName[:10], "-", "")
	}
	// Otherwise use the name as-is (cleaned)
	cleaned := strings.ReplaceAll(tenantName, " ", "-")
	cleaned = strings.ToLower(cleaned)
	if len(cleaned) > 20 {
		cleaned = cleaned[:20]
	}
	return cleaned
}
