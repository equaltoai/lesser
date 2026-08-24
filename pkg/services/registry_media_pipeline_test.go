package services

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	pkgconfig "github.com/equaltoai/lesser/pkg/config"
	mediasvc "github.com/equaltoai/lesser/pkg/services/media"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type fakeMediaS3API struct {
	putInput  *s3.PutObjectInput
	putData   []byte
	copyInput *s3.CopyObjectInput
}

func (f *fakeMediaS3API) PutObject(
	_ context.Context,
	input *s3.PutObjectInput,
	_ ...func(*s3.Options),
) (*s3.PutObjectOutput, error) {
	f.putInput = input
	data, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	f.putData = data
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeMediaS3API) GetObject(
	_ context.Context,
	input *s3.GetObjectInput,
	_ ...func(*s3.Options),
) (*s3.GetObjectOutput, error) {
	return &s3.GetObjectOutput{
		Body:        io.NopCloser(bytes.NewReader(f.putData)),
		ContentType: aws.String("image/png"),
	}, nil
}

func (f *fakeMediaS3API) HeadObject(
	_ context.Context,
	_ *s3.HeadObjectInput,
	_ ...func(*s3.Options),
) (*s3.HeadObjectOutput, error) {
	return &s3.HeadObjectOutput{
		ContentLength: aws.Int64(int64(len(f.putData))),
		ContentType:   aws.String("image/png"),
	}, nil
}

func (f *fakeMediaS3API) DeleteObject(
	_ context.Context,
	_ *s3.DeleteObjectInput,
	_ ...func(*s3.Options),
) (*s3.DeleteObjectOutput, error) {
	return &s3.DeleteObjectOutput{}, nil
}

func (f *fakeMediaS3API) CopyObject(
	_ context.Context,
	input *s3.CopyObjectInput,
	_ ...func(*s3.Options),
) (*s3.CopyObjectOutput, error) {
	f.copyInput = input
	return &s3.CopyObjectOutput{}, nil
}

// headNilMediaS3API is a fake whose HeadObject reports neither a
// ContentLength nor a ContentType, exercising the fail-closed nil branches of
// mediaS3ObjectStore.HeadFile.
type headNilMediaS3API struct {
	*fakeMediaS3API
}

func (h *headNilMediaS3API) HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	return &s3.HeadObjectOutput{}, nil
}

type fakeRegistryMediaStore struct {
	objects map[string][]byte
}

func (f *fakeRegistryMediaStore) UploadFile(
	_ context.Context,
	bucket string,
	key string,
	data []byte,
	_ string,
) (string, error) {
	if f.objects == nil {
		f.objects = make(map[string][]byte)
	}
	f.objects[bucket+"/"+key] = bytes.Clone(data)
	return "s3://" + bucket + "/" + key, nil
}

func (f *fakeRegistryMediaStore) UploadInternalFile(
	ctx context.Context,
	bucket string,
	key string,
	data []byte,
	contentType string,
	_ string,
) (string, error) {
	return f.UploadFile(ctx, bucket, key, data, contentType)
}

func (f *fakeRegistryMediaStore) DeleteFile(_ context.Context, bucket, key string) error {
	delete(f.objects, bucket+"/"+key)
	return nil
}

func (f *fakeRegistryMediaStore) GeneratePresignedURL(_ context.Context, bucket, key string, _ time.Duration) (string, error) {
	return "https://signed.invalid/" + bucket + "/" + key, nil
}

func TestMediaS3ObjectStoreUsesPutObjectWithExactBytesAndContentType(t *testing.T) {
	fakeClient := &fakeMediaS3API{}
	store := &mediaS3ObjectStore{client: fakeClient}
	want := []byte("exact-upload-bytes\x00\xff")

	location, err := store.UploadFile(context.Background(), "media-bucket", "media/key.png", want, "image/png")
	require.NoError(t, err)
	require.Equal(t, "s3://media-bucket/media/key.png", location)
	require.Equal(t, want, fakeClient.putData)
	require.Equal(t, "media-bucket", aws.ToString(fakeClient.putInput.Bucket))
	require.Equal(t, "media/key.png", aws.ToString(fakeClient.putInput.Key))
	require.Equal(t, "image/png", aws.ToString(fakeClient.putInput.ContentType))
	require.Equal(t, int64(len(want)), aws.ToInt64(fakeClient.putInput.ContentLength))
}

func TestMediaS3ObjectStoreUsesKMSForInternalEditorialBytes(t *testing.T) {
	fakeClient := &fakeMediaS3API{}
	store := &mediaS3ObjectStore{client: fakeClient}

	_, err := store.UploadInternalFile(
		context.Background(),
		"media-bucket",
		"media/internal.png",
		[]byte("internal-editorial-bytes"),
		"image/png",
		"alias/lesser-shared-encryption",
	)
	require.NoError(t, err)
	require.Equal(t, "aws:kms", string(fakeClient.putInput.ServerSideEncryption))
	require.Equal(t, "alias/lesser-shared-encryption", aws.ToString(fakeClient.putInput.SSEKMSKeyId))
}

func TestMediaS3ObjectStoreHeadFileReportsLengthAndType(t *testing.T) {
	fakeClient := &fakeMediaS3API{}
	fakeClient.putData = []byte("exact-head-bytes")
	store := &mediaS3ObjectStore{client: fakeClient}

	length, contentType, err := store.HeadFile(context.Background(), "media-bucket", "media/key.png")
	require.NoError(t, err)
	require.Equal(t, int64(len("exact-head-bytes")), length)
	require.Equal(t, "image/png", contentType)
}

func TestMediaS3ObjectStoreHeadFileNilMetadataFailsClosed(t *testing.T) {
	store := &mediaS3ObjectStore{client: &headNilMediaS3API{fakeMediaS3API: &fakeMediaS3API{}}}
	length, contentType, err := store.HeadFile(context.Background(), "media-bucket", "media/key.png")
	require.NoError(t, err)
	require.Zero(t, length, "a missing ContentLength is reported as zero")
	require.Empty(t, contentType, "a missing ContentType is reported as empty")
}

func TestMediaS3ObjectStoreHeadFileUnavailableFailsClosed(t *testing.T) {
	store := &mediaS3ObjectStore{}
	_, _, err := store.HeadFile(context.Background(), "media-bucket", "media/key.png")
	require.Error(t, err)
}

func TestMediaS3ObjectStoreDownloadFileBounded(t *testing.T) {
	fakeClient := &fakeMediaS3API{}
	fakeClient.putData = bytes.Repeat([]byte{0xAB}, 64)
	store := &mediaS3ObjectStore{client: fakeClient}

	// An at-cap body downloads fully with its stored type.
	data, contentType, err := store.DownloadFile(context.Background(), "media-bucket", "media/key.png", 64)
	require.NoError(t, err)
	require.Len(t, data, 64)
	require.Equal(t, "image/png", contentType)

	// A body past the bound is refused: the read aborts at maxBytes+1.
	_, _, err = store.DownloadFile(context.Background(), "media-bucket", "media/key.png", 63)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds the declared size cap")
}

func TestMediaS3ObjectStoreDownloadFileUnavailableFailsClosed(t *testing.T) {
	store := &mediaS3ObjectStore{}
	_, _, err := store.DownloadFile(context.Background(), "media-bucket", "media/key.png", 10)
	require.Error(t, err)
}

func TestRegistryMediaWiresObjectStoreAndProcessingQueue(t *testing.T) {
	logger := zap.NewNop()
	storage := newPermissiveRegistryStorage(t, "example.com", logger)
	registry, err := NewRegistry(
		WithStorage(storage),
		WithLogger(logger),
		WithConfig(&ServiceConfig{
			BaseURL:   "https://dev.example.com",
			JWTSecret: strings.Repeat("x", 32),
			Config: &pkgconfig.Config{
				IntegrationTestMode:   true,
				S3BucketName:          "dev-media-bucket",
				CloudFrontDomain:      "media.dev.example.com",
				MediaSourceBucketName: "dev-media-bucket",
				KMSKeyID:              "alias/lesser-shared-encryption",
			},
		}),
	)
	require.NoError(t, err)

	objectStore := &fakeRegistryMediaStore{}
	var queued MediaJobMessage
	registry.mediaS3 = objectStore
	registry.jobQueue = &captureJobQueue{onMedia: func(msg MediaJobMessage) {
		queued = msg
	}}

	fileData := []byte("registry-byte-pipeline")
	result, err := registry.Media().UploadMedia(context.Background(), &mediasvc.UploadMediaCommand{
		UserID:        "alice",
		FileName:      "evidence.png",
		ContentType:   "image/png",
		FileData:      fileData,
		MediaCategory: models.MediaCategoryImage,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, queued.JobID)
	require.Equal(t, result.Media.MediaID, queued.MediaID)
	require.Equal(t, "alice", queued.Username)
	require.Equal(t, fileData, objectStore.objects[result.Media.S3Bucket+"/"+result.Media.S3Key])
}

func TestRegistryJobQueueFallbackWarnsWhenSQSInitializationFails(t *testing.T) {
	missingConfig := filepath.Join(t.TempDir(), "missing-aws-config")
	t.Setenv("AWS_PROFILE", "missing-media-queue-profile")
	t.Setenv("AWS_CONFIG_FILE", missingConfig)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", missingConfig)

	core, observed := observer.New(zapcore.WarnLevel)
	registry := &Registry{
		logger: zap.New(core),
		config: &ServiceConfig{Config: &pkgconfig.Config{}},
	}

	queue := registry.getJobQueue()

	require.IsType(t, &simpleJobQueue{}, queue)
	entries := observed.FilterMessage("failed to initialize SQS job queue; falling back to simple log-only queue").All()
	require.Len(t, entries, 1)
	require.Equal(t, ErrAWSConfigLoad.Error(), entries[0].ContextMap()["error"])
}
