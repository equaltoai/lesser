package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type fakeMediaS3Service struct {
	objects      map[string][]byte
	contentTypes map[string]string
	uploadErr    error
	deleteErr    error
	deleteCalls  []mediaS3DeleteCall
}

type mediaS3DeleteCall struct {
	bucket string
	key    string
}

func newFakeMediaS3Service() *fakeMediaS3Service {
	return &fakeMediaS3Service{
		objects:      make(map[string][]byte),
		contentTypes: make(map[string]string),
	}
}

func (f *fakeMediaS3Service) UploadFile(
	_ context.Context,
	bucket string,
	key string,
	data []byte,
	contentType string,
) (string, error) {
	if f.uploadErr != nil {
		return "", f.uploadErr
	}
	objectKey := bucket + "/" + key
	f.objects[objectKey] = bytes.Clone(data)
	f.contentTypes[objectKey] = contentType
	return "s3://" + objectKey, nil
}

func (f *fakeMediaS3Service) DeleteFile(_ context.Context, bucket, key string) error {
	f.deleteCalls = append(f.deleteCalls, mediaS3DeleteCall{bucket: bucket, key: key})
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.objects, bucket+"/"+key)
	return nil
}

func (f *fakeMediaS3Service) GeneratePresignedURL(
	_ context.Context,
	bucket string,
	key string,
	_ time.Duration,
) (string, error) {
	return fmt.Sprintf("https://example.invalid/%s/%s", bucket, key), nil
}

func TestServiceUploadMediaPersistsExactBytesHashAndProcessingJob(t *testing.T) {
	service, mediaRepo, jobQueue, _ := createTestService(t)
	objectStore := newFakeMediaS3Service()
	service.SetS3Service(objectStore)

	fileData := []byte("\x89PNG\r\n\x1a\nbyte-pipeline-regression-payload")
	cmd := &UploadMediaCommand{
		UserID:        "alice",
		FileName:      "evidence.png",
		ContentType:   "image/png",
		FileData:      fileData,
		Description:   "byte pipeline regression fixture",
		MediaCategory: models.MediaCategoryImage,
	}
	digest := sha256.Sum256(fileData)
	wantHash := "sha256:" + hex.EncodeToString(digest[:])

	var storedMedia *models.Media
	mediaRepo.On("CreateMedia", mock.Anything, mock.MatchedBy(func(media *models.Media) bool {
		storedMedia = media
		return media.ContentHash == wantHash
	})).Return(nil).Once()

	var storedJob *models.MediaJob
	mediaRepo.On("CreateMediaJob", mock.Anything, mock.MatchedBy(func(job *models.MediaJob) bool {
		storedJob = job
		return job.FileHash == wantHash &&
			job.S3Key != "" &&
			job.MimeType == cmd.ContentType &&
			job.FileSize == int64(len(fileData))
	})).Return(nil).Once()

	jobQueue.On("QueueMediaJob", mock.Anything, mock.MatchedBy(func(msg JobMessage) bool {
		return storedJob != nil && msg.JobID == storedJob.JobID && msg.MediaID == storedJob.MediaID
	})).Return(nil).Once()

	result, err := service.UploadMedia(context.Background(), cmd)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Same(t, storedMedia, result.Media)
	require.Equal(t, wantHash, result.Media.ContentHash)
	require.Equal(t, wantHash, storedJob.FileHash)
	require.Equal(t, result.Media.MediaID, storedJob.MediaID)
	require.Equal(t, result.Media.S3Key, storedJob.S3Key)

	objectKey := result.Media.S3Bucket + "/" + result.Media.S3Key
	require.Equal(t, fileData, objectStore.objects[objectKey])
	require.Equal(t, cmd.ContentType, objectStore.contentTypes[objectKey])
	require.Equal(t, "https://cdn.example.com/"+result.Media.S3Key, result.Media.CDNUrl)

	mediaRepo.AssertExpectations(t)
	jobQueue.AssertExpectations(t)
}

func TestServiceUploadMediaStopsBeforeRecordsAndQueueWhenObjectUploadFails(t *testing.T) {
	service, mediaRepo, jobQueue, _ := createTestService(t)
	objectStore := newFakeMediaS3Service()
	objectStore.uploadErr = fmt.Errorf("put object failed")
	service.SetS3Service(objectStore)

	result, err := service.UploadMedia(context.Background(), createValidUploadCommand())
	require.Error(t, err)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrMediaStorageFailed)
	mediaRepo.AssertNotCalled(t, "CreateMedia", mock.Anything, mock.Anything)
	mediaRepo.AssertNotCalled(t, "CreateMediaJob", mock.Anything, mock.Anything)
	jobQueue.AssertNotCalled(t, "QueueMediaJob", mock.Anything, mock.Anything)
}

func TestServiceUploadMediaValidationStillFailsBeforeObjectStorage(t *testing.T) {
	service, mediaRepo, jobQueue, _ := createTestService(t)
	objectStore := newFakeMediaS3Service()
	service.SetS3Service(objectStore)
	cmd := createValidUploadCommand()
	cmd.FileData = nil

	result, err := service.UploadMedia(context.Background(), cmd)
	require.Error(t, err)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrMediaValidationFailed)
	require.Contains(t, err.Error(), "required")
	require.Empty(t, objectStore.objects)
	mediaRepo.AssertNotCalled(t, "CreateMedia", mock.Anything, mock.Anything)
	mediaRepo.AssertNotCalled(t, "CreateMediaJob", mock.Anything, mock.Anything)
	jobQueue.AssertNotCalled(t, "QueueMediaJob", mock.Anything, mock.Anything)
}
