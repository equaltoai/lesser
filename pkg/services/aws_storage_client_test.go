package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAWSS3StorageClient_getContentType(t *testing.T) {
	client := &AWSS3StorageClient{}

	assert.Equal(t, "application/gzip", client.getContentType("backup.tar.gz"))
	assert.Equal(t, "application/zip", client.getContentType("backup.ZIP"))
	assert.Equal(t, "application/x-tar", client.getContentType("backup.tar"))
	assert.Equal(t, "text/csv", client.getContentType("export.csv"))
	assert.Equal(t, "application/json", client.getContentType("report.json"))
	assert.Equal(t, "application/octet-stream", client.getContentType("unknown.bin"))
}

func TestAWSS3StorageClient_UploadFile_InputValidation(t *testing.T) {
	client := &AWSS3StorageClient{}

	err := client.UploadFile(context.Background(), "", []byte("data"))
	require.Error(t, err)

	err = client.UploadFile(context.Background(), "key", []byte{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotUploadEmptyData)
}

func TestAWSS3StorageClient_GetFile_InputValidation(t *testing.T) {
	client := &AWSS3StorageClient{}

	_, err := client.GetFile(context.Background(), "")
	require.Error(t, err)
}

type failingCredentialsProvider struct {
	err error
}

func (p failingCredentialsProvider) Retrieve(context.Context) (aws.Credentials, error) {
	return aws.Credentials{}, p.err
}

type fakeUploadAPIClient struct {
	putObjectFn func(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

func (c *fakeUploadAPIClient) PutObject(ctx context.Context, input *s3.PutObjectInput, options ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if c.putObjectFn != nil {
		return c.putObjectFn(ctx, input, options...)
	}
	return &s3.PutObjectOutput{}, nil
}

func (*fakeUploadAPIClient) UploadPart(context.Context, *s3.UploadPartInput, ...func(*s3.Options)) (*s3.UploadPartOutput, error) {
	return nil, errors.New("unexpected multipart UploadPart call")
}

func (*fakeUploadAPIClient) CreateMultipartUpload(context.Context, *s3.CreateMultipartUploadInput, ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error) {
	return nil, errors.New("unexpected multipart CreateMultipartUpload call")
}

func (*fakeUploadAPIClient) CompleteMultipartUpload(context.Context, *s3.CompleteMultipartUploadInput, ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error) {
	return nil, errors.New("unexpected multipart CompleteMultipartUpload call")
}

func (*fakeUploadAPIClient) AbortMultipartUpload(context.Context, *s3.AbortMultipartUploadInput, ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error) {
	return nil, errors.New("unexpected multipart AbortMultipartUpload call")
}

type fakeDownloadAPIClient struct {
	getObjectFn func(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

func (c *fakeDownloadAPIClient) GetObject(ctx context.Context, input *s3.GetObjectInput, options ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if c.getObjectFn != nil {
		return c.getObjectFn(ctx, input, options...)
	}
	return &s3.GetObjectOutput{}, nil
}

func TestAWSS3StorageClient_GeneratePresignedURL_Success(t *testing.T) {
	s3Client := s3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
	})

	client := &AWSS3StorageClient{
		client:     s3Client,
		bucketName: "bucket",
		logger:     zap.NewNop(),
	}

	got, err := client.GeneratePresignedURL(context.Background(), "exports/report.json", 5*time.Minute)
	require.NoError(t, err)
	require.NotEmpty(t, got)

	parsed, err := url.Parse(got)
	require.NoError(t, err)
	assert.NotEmpty(t, parsed.Host)
	assert.Contains(t, parsed.Path, "report.json")
	assert.NotEmpty(t, parsed.Query().Get("X-Amz-Signature"))
}

func TestAWSS3StorageClient_GeneratePresignedURL_CredentialError(t *testing.T) {
	credErr := errors.New("no credentials")
	s3Client := s3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: failingCredentialsProvider{err: credErr},
	})

	client := &AWSS3StorageClient{
		client:     s3Client,
		bucketName: "bucket",
		logger:     zap.NewNop(),
	}

	_, err := client.GeneratePresignedURL(context.Background(), "exports/report.json", 5*time.Minute)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPresignedURLCreationFailed)
	assert.ErrorIs(t, err, credErr)
}

func TestAWSS3StorageClient_UploadFile_Success(t *testing.T) {
	var putCalls int
	fakeClient := &fakeUploadAPIClient{
		putObjectFn: func(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			putCalls++
			require.Equal(t, "bucket", aws.ToString(input.Bucket))
			require.Equal(t, "export.json", aws.ToString(input.Key))
			require.Equal(t, "application/json", aws.ToString(input.ContentType))
			require.Equal(t, types.ServerSideEncryptionAes256, input.ServerSideEncryption)
			require.Equal(t, types.StorageClassStandardIa, input.StorageClass)
			require.Equal(t, types.ChecksumAlgorithmSha256, input.ChecksumAlgorithm)
			require.Equal(t, "import-export-service", input.Metadata["upload-source"])
			require.NotEmpty(t, input.Metadata["upload-time"])
			require.Equal(t, "5", input.Metadata["content-length"])
			require.Equal(t, "export.json", input.Metadata["original-filename"])

			body, err := io.ReadAll(input.Body)
			require.NoError(t, err)
			require.Equal(t, []byte("hello"), body)

			return &s3.PutObjectOutput{ETag: aws.String("\"etag\"")}, nil
		},
	}

	uploader := manager.NewUploader(fakeClient, func(u *manager.Uploader) {
		u.PartSize = 10 * 1024 * 1024
		u.Concurrency = 1
	})

	client := &AWSS3StorageClient{
		uploader:   uploader,
		bucketName: "bucket",
		logger:     zap.NewNop(),
	}

	require.NoError(t, client.UploadFile(context.Background(), "export.json", []byte("hello")))
	require.Equal(t, 1, putCalls)
}

func TestAWSS3StorageClient_UploadFile_UploaderError(t *testing.T) {
	putErr := errors.New("put failed")
	fakeClient := &fakeUploadAPIClient{
		putObjectFn: func(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			return nil, putErr
		},
	}

	uploader := manager.NewUploader(fakeClient, func(u *manager.Uploader) {
		u.PartSize = 10 * 1024 * 1024
		u.Concurrency = 1
	})

	client := &AWSS3StorageClient{
		uploader:   uploader,
		bucketName: "bucket",
		logger:     zap.NewNop(),
	}

	err := client.UploadFile(context.Background(), "export.json", []byte("hello"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrS3UploadFailed)
	assert.ErrorIs(t, err, putErr)
}

func TestAWSS3StorageClient_GetFile_Success(t *testing.T) {
	payload := []byte("world")
	fakeClient := &fakeDownloadAPIClient{
		getObjectFn: func(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			require.Equal(t, "bucket", aws.ToString(input.Bucket))
			require.Equal(t, "export.json", aws.ToString(input.Key))
			return &s3.GetObjectOutput{
				Body:          io.NopCloser(bytes.NewReader(payload)),
				ContentLength: aws.Int64(int64(len(payload))),
			}, nil
		},
	}

	downloader := manager.NewDownloader(fakeClient, func(d *manager.Downloader) {
		d.PartSize = 10 * 1024 * 1024
		d.Concurrency = 1
		d.DisableValidateParts = true
	})

	client := &AWSS3StorageClient{
		downloader: downloader,
		bucketName: "bucket",
		logger:     zap.NewNop(),
	}

	got, err := client.GetFile(context.Background(), "export.json")
	require.NoError(t, err)
	require.Equal(t, payload, got)
}

func TestAWSS3StorageClient_GetFile_DownloaderError(t *testing.T) {
	getErr := errors.New("get failed")
	fakeClient := &fakeDownloadAPIClient{
		getObjectFn: func(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return nil, getErr
		},
	}

	downloader := manager.NewDownloader(fakeClient, func(d *manager.Downloader) {
		d.PartSize = 10 * 1024 * 1024
		d.Concurrency = 1
		d.DisableValidateParts = true
	})

	client := &AWSS3StorageClient{
		downloader: downloader,
		bucketName: "bucket",
		logger:     zap.NewNop(),
	}

	_, err := client.GetFile(context.Background(), "export.json")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrS3DownloadFailed)
	assert.ErrorIs(t, err, getErr)
}
