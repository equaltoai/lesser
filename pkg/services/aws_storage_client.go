package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/equaltoai/lesser/pkg/common"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	"go.uber.org/zap"
)

// AWSS3StorageClient implements StorageClient using AWS S3
type AWSS3StorageClient struct {
	client     *s3.Client
	uploader   *manager.Uploader
	downloader *manager.Downloader
	bucketName string
	logger     *zap.Logger
}

// NewAWSS3StorageClient creates a new AWS S3-based storage client
func NewAWSS3StorageClient(ctx context.Context, logger *zap.Logger) (*AWSS3StorageClient, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	// Get configuration from centralized config
	appCfg := appconfig.Get()

	// Check for bucket name - prefer explicit media bucket configuration
	bucketName := appCfg.S3MediaBucket
	if bucketName == "" {
		bucketName = appCfg.MediaBucketName // Fallback to alternative field
	}
	if bucketName == "" {
		bucketName = appCfg.MediaSourceBucketName
	}

	// Load AWS configuration with retry and region settings
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(appCfg.Region),
		config.WithRetryMaxAttempts(3),
	)
	if err != nil {
		return nil, errors.Join(ErrAWSConfigLoadFailed, err)
	}

	client := s3.NewFromConfig(cfg)

	// Test connectivity by checking if bucket exists and is accessible
	if strings.TrimSpace(bucketName) != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{
			Bucket: aws.String(bucketName),
		})
		if err != nil {
			logger.Error("failed to access S3 bucket",
				zap.String("bucket", bucketName),
				zap.Error(err))
			return nil, errors.Join(ErrS3BucketAccessFailed, err)
		}
	} else {
		logger.Warn("media bucket configuration is empty; UploadFile calls must supply a bucket explicitly")
	}

	uploader := manager.NewUploader(client, func(u *manager.Uploader) {
		u.PartSize = 10 * 1024 * 1024 // 10MB parts for better performance
		u.Concurrency = 3             // Upload up to 3 parts concurrently
	})

	downloader := manager.NewDownloader(client, func(d *manager.Downloader) {
		d.PartSize = 10 * 1024 * 1024 // 10MB parts
		d.Concurrency = 3             // Download up to 3 parts concurrently
	})

	return &AWSS3StorageClient{
		client:     client,
		uploader:   uploader,
		downloader: downloader,
		bucketName: bucketName,
		logger:     logger,
	}, nil
}

// GeneratePresignedURL generates a presigned URL for downloading a file
func (s *AWSS3StorageClient) GeneratePresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	return s.GeneratePresignedURLForBucket(ctx, "", key, expiry)
}

func (s *AWSS3StorageClient) GeneratePresignedURLForBucket(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	resolvedBucket, err := s.resolveBucket(bucket)
	if err != nil {
		return "", err
	}

	presignClient := s3.NewPresignClient(s.client)

	presignRequest, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(resolvedBucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expiry
	})

	if err != nil {
		s.logger.Error("failed to create presigned URL",
			zap.String("bucket", resolvedBucket),
			zap.String("key", key),
			zap.Duration("expiry", expiry),
			zap.Error(err))
		return "", errors.Join(ErrPresignedURLCreationFailed, err)
	}

	s.logger.Debug("generated presigned URL",
		zap.String("bucket", resolvedBucket),
		zap.String("key", key),
		zap.Duration("expiry", expiry))

	return presignRequest.URL, nil
}

// UploadFile uploads a file to S3
func (s *AWSS3StorageClient) UploadFile(ctx context.Context, key string, data []byte) error {
	_, err := s.UploadFileWithContentType(ctx, "", key, data, "")
	return err
}

func (s *AWSS3StorageClient) UploadFileWithContentType(ctx context.Context, bucket, key string, data []byte, contentType string) (string, error) {
	if err := common.ValidateRequiredParam("key", key); err != nil {
		return "", err
	}
	if err := common.ValidateSliceNotEmpty("data", data); err != nil {
		return "", ErrCannotUploadEmptyData
	}

	resolvedBucket, err := s.resolveBucket(bucket)
	if err != nil {
		return "", err
	}

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
	}

	reader := bytes.NewReader(data)
	objectContentType := strings.TrimSpace(contentType)
	if objectContentType == "" {
		objectContentType = s.getContentType(key)
	}

	uploadInput := &s3.PutObjectInput{
		Bucket:               aws.String(resolvedBucket),
		Key:                  aws.String(key),
		Body:                 reader,
		ContentType:          aws.String(objectContentType),
		ServerSideEncryption: types.ServerSideEncryptionAes256,
		Metadata: map[string]string{
			"upload-source":     "media-service",
			"upload-time":       time.Now().UTC().Format(time.RFC3339),
			"content-length":    fmt.Sprintf("%d", len(data)),
			"original-filename": key,
		},
		StorageClass:      types.StorageClassStandard,
		ChecksumAlgorithm: types.ChecksumAlgorithmSha256,
	}

	result, err := s.uploader.Upload(ctx, uploadInput)
	if err != nil {
		s.logger.Error("failed to upload file to S3",
			zap.String("bucket", resolvedBucket),
			zap.String("key", key),
			zap.Int("size", len(data)),
			zap.Error(err))
		return "", errors.Join(ErrS3UploadFailed, err)
	}

	s.logger.Info("file uploaded successfully to S3",
		zap.String("bucket", resolvedBucket),
		zap.String("key", key),
		zap.String("location", result.Location),
		zap.Int("size", len(data)),
		zap.String("etag", aws.ToString(result.ETag)))

	if result.Location != "" {
		return result.Location, nil
	}
	return fmt.Sprintf("s3://%s/%s", resolvedBucket, key), nil
}

func (s *AWSS3StorageClient) DeleteFileFromBucket(ctx context.Context, bucket, key string) error {
	resolvedBucket, err := s.resolveBucket(bucket)
	if err != nil {
		return err
	}

	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(resolvedBucket),
		Key:    aws.String(key),
	})
	if err != nil {
		s.logger.Error("failed to delete file from S3",
			zap.String("bucket", resolvedBucket),
			zap.String("key", key),
			zap.Error(err))
		return errors.Join(ErrS3DeleteFailed, err)
	}

	return nil
}

// GetFile downloads a file from S3
func (s *AWSS3StorageClient) GetFile(ctx context.Context, key string) ([]byte, error) {
	if err := common.ValidateRequiredParam("key", key); err != nil {
		return nil, err
	}

	// Add timeout to the context if not already present
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Minute) // Generous timeout for large files
		defer cancel()
	}

	// Use a buffer to write the downloaded data
	buffer := manager.NewWriteAtBuffer([]byte{})

	downloadInput := &s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
		// Request checksum validation for data integrity
		ChecksumMode: types.ChecksumModeEnabled,
	}

	numBytes, err := s.downloader.Download(ctx, buffer, downloadInput)
	if err != nil {
		s.logger.Error("failed to download file from S3",
			zap.String("bucket", s.bucketName),
			zap.String("key", key),
			zap.Error(err))
		return nil, errors.Join(ErrS3DownloadFailed, err)
	}

	if numBytes == 0 {
		s.logger.Warn("downloaded empty file from S3",
			zap.String("bucket", s.bucketName),
			zap.String("key", key))
	}

	s.logger.Debug("file downloaded successfully from S3",
		zap.String("bucket", s.bucketName),
		zap.String("key", key),
		zap.Int64("size", numBytes))

	return buffer.Bytes(), nil
}

func (s *AWSS3StorageClient) resolveBucket(bucket string) (string, error) {
	trimmed := strings.TrimSpace(bucket)
	if trimmed != "" {
		return trimmed, nil
	}
	if strings.TrimSpace(s.bucketName) != "" {
		return s.bucketName, nil
	}
	return "", ErrS3BucketConfigRequired
}

// getContentType returns the appropriate content type based on file extension
func (s *AWSS3StorageClient) getContentType(key string) string {
	// Simple content type mapping for common import/export formats
	if len(key) > 5 {
		switch key[len(key)-4:] {
		case ".zip":
			return "application/zip"
		case ".tar":
			return "application/x-tar"
		case ".csv":
			return "text/csv"
		case ".json":
			return "application/json"
		}
	}
	if len(key) > 7 && key[len(key)-7:] == ".tar.gz" {
		return "application/gzip"
	}
	// Default to binary for unknown file types
	return "application/octet-stream"
}
