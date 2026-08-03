package main

import (
	"bytes"
	"context"
	"encoding/json"
	stdErrors "errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/mediaconvert"
	mctypes "github.com/aws/aws-sdk-go-v2/service/mediaconvert/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/monitoring"
	"github.com/equaltoai/lesser/pkg/observability"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
)

type fakeS3Client struct {
	objects map[string][]byte
	getErr  error
	putErr  error
}

func (f *fakeS3Client) GetObject(_ context.Context, params *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.objects == nil {
		f.objects = make(map[string][]byte)
	}
	key := aws.ToString(params.Key)
	data, ok := f.objects[key]
	if !ok {
		return nil, stdErrors.New("not found")
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data))}, nil
}

func (f *fakeS3Client) PutObject(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if f.putErr != nil {
		return nil, f.putErr
	}
	if f.objects == nil {
		f.objects = make(map[string][]byte)
	}
	key := aws.ToString(params.Key)
	bodyBytes, err := io.ReadAll(params.Body)
	if err != nil {
		return nil, err
	}
	f.objects[key] = bodyBytes
	return &s3.PutObjectOutput{}, nil
}

type fakeMediaConvertClient struct {
	jobID string
	err   error
}

func (f *fakeMediaConvertClient) CreateJob(_ context.Context, _ *mediaconvert.CreateJobInput, _ ...func(*mediaconvert.Options)) (*mediaconvert.CreateJobOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &mediaconvert.CreateJobOutput{
		Job: &mctypes.Job{Id: aws.String(f.jobID)},
	}, nil
}

type fakeUnifiedTracker struct {
	calls int
	err   error
}

func (f *fakeUnifiedTracker) TrackS3Put(_ context.Context, _ string, _ int64) error {
	f.calls++
	return f.err
}

type fakeMediaMetadataRepo struct {
	startErr    error
	failErr     error
	completeErr error

	started   []string
	failed    []struct{ id, reason string }
	completed []struct {
		id     string
		result repositories.ProcessingResult
	}
}

func (f *fakeMediaMetadataRepo) MarkProcessingStarted(_ context.Context, mediaID string) error {
	f.started = append(f.started, mediaID)
	return f.startErr
}

func (f *fakeMediaMetadataRepo) MarkProcessingFailed(_ context.Context, mediaID, reason string) error {
	f.failed = append(f.failed, struct{ id, reason string }{id: mediaID, reason: reason})
	return f.failErr
}

func (f *fakeMediaMetadataRepo) MarkProcessingComplete(_ context.Context, mediaID string, results repositories.ProcessingResult) error {
	f.completed = append(f.completed, struct {
		id     string
		result repositories.ProcessingResult
	}{id: mediaID, result: results})
	return f.completeErr
}

type fakeMediaAnalyticsRepo struct {
	err       error
	analytics []*models.MediaAnalytics
}

func (f *fakeMediaAnalyticsRepo) RecordMediaAnalytics(_ context.Context, analytics *models.MediaAnalytics) error {
	f.analytics = append(f.analytics, analytics)
	return f.err
}

type fakeMediaRepo struct {
	jobs    map[string]*models.MediaJob
	media   map[string]*models.Media
	configs map[string]*models.UserMediaConfig

	getJobErr        error
	updateJobErr     error
	updateJobFn      func(ctx context.Context, job *models.MediaJob) error
	getMediaErr      error
	updateMediaErr   error
	getConfigErr     error
	createConfigErr  error
	updateConfigErr  error
	getSpendingErr   error
	addTxnErr        error
	addTxnErrBySvc   map[string]error
	userByUsername   map[string]*models.UserMediaConfig
	spendingByPeriod map[string]*models.MediaSpending

	transactions []*models.MediaSpendingTransaction
}

func (f *fakeMediaRepo) GetMediaJob(_ context.Context, jobID string) (*models.MediaJob, error) {
	if f.getJobErr != nil {
		return nil, f.getJobErr
	}
	if f.jobs == nil {
		f.jobs = make(map[string]*models.MediaJob)
	}
	job, ok := f.jobs[jobID]
	if !ok {
		return nil, stdErrors.New("not found")
	}
	return job, nil
}

func (f *fakeMediaRepo) UpdateMediaJob(ctx context.Context, job *models.MediaJob) error {
	if f.updateJobFn != nil {
		return f.updateJobFn(ctx, job)
	}
	if f.updateJobErr != nil {
		return f.updateJobErr
	}
	if f.jobs == nil {
		f.jobs = make(map[string]*models.MediaJob)
	}
	f.jobs[job.JobID] = job
	return nil
}

func (f *fakeMediaRepo) GetMedia(_ context.Context, mediaID string) (*models.Media, error) {
	if f.getMediaErr != nil {
		return nil, f.getMediaErr
	}
	if f.media == nil {
		f.media = make(map[string]*models.Media)
	}
	m, ok := f.media[mediaID]
	if !ok {
		return nil, stdErrors.New("not found")
	}
	return m, nil
}

func (f *fakeMediaRepo) UpdateMedia(_ context.Context, media *models.Media) error {
	if f.updateMediaErr != nil {
		return f.updateMediaErr
	}
	if f.media == nil {
		f.media = make(map[string]*models.Media)
	}
	f.media[media.MediaID] = media
	return nil
}

func (f *fakeMediaRepo) GetUserMediaConfig(_ context.Context, userID string) (*models.UserMediaConfig, error) {
	if f.getConfigErr != nil {
		return nil, f.getConfigErr
	}
	if f.configs == nil {
		f.configs = make(map[string]*models.UserMediaConfig)
	}
	cfg, ok := f.configs[userID]
	if !ok {
		return nil, stdErrors.New("not found")
	}
	return cfg, nil
}

func (f *fakeMediaRepo) CreateUserMediaConfig(_ context.Context, cfg *models.UserMediaConfig) error {
	if f.createConfigErr != nil {
		return f.createConfigErr
	}
	if f.configs == nil {
		f.configs = make(map[string]*models.UserMediaConfig)
	}
	f.configs[cfg.UserID] = cfg
	if f.userByUsername == nil {
		f.userByUsername = make(map[string]*models.UserMediaConfig)
	}
	f.userByUsername[cfg.Username] = cfg
	return nil
}

func (f *fakeMediaRepo) UpdateUserMediaConfig(_ context.Context, cfg *models.UserMediaConfig) error {
	if f.updateConfigErr != nil {
		return f.updateConfigErr
	}
	if f.configs == nil {
		f.configs = make(map[string]*models.UserMediaConfig)
	}
	f.configs[cfg.UserID] = cfg
	return nil
}

func (f *fakeMediaRepo) GetUserMediaConfigByUsername(_ context.Context, username string) (*models.UserMediaConfig, error) {
	if f.userByUsername == nil {
		return nil, stdErrors.New("not found")
	}
	cfg, ok := f.userByUsername[username]
	if !ok {
		return nil, stdErrors.New("not found")
	}
	return cfg, nil
}

func (f *fakeMediaRepo) GetMediaSpending(_ context.Context, userID, period string) (*models.MediaSpending, error) {
	if f.getSpendingErr != nil {
		return nil, f.getSpendingErr
	}
	if f.spendingByPeriod == nil {
		return nil, stdErrors.New("not found")
	}
	key := userID + "|" + period
	sp, ok := f.spendingByPeriod[key]
	if !ok {
		return nil, stdErrors.New("not found")
	}
	return sp, nil
}

func (f *fakeMediaRepo) AddSpendingTransaction(_ context.Context, txn *models.MediaSpendingTransaction) error {
	if f.addTxnErrBySvc != nil {
		if err := f.addTxnErrBySvc[txn.Service]; err != nil {
			return err
		}
	}
	if f.addTxnErr != nil {
		return f.addTxnErr
	}
	f.transactions = append(f.transactions, txn)
	return nil
}

func (f *fakeMediaRepo) UpdateUserMediaConfigByUsername(_ context.Context, _ *models.UserMediaConfig) error {
	return nil
}

func mustJPEGBytes(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

func testMP4Header(t *testing.T) []byte {
	t.Helper()
	data := []byte{
		0x00, 0x00, 0x00, 0x18,
		'f', 't', 'y', 'p',
		'm', 'p', '4', '2',
		0x00, 0x00, 0x00, 0x00,
		'm', 'p', '4', '2',
		'i', 's', 'o', 'm',
	}
	require.Equal(t, "video/mp4", http.DetectContentType(data))
	return data
}

func testMP3Header(t *testing.T) []byte {
	t.Helper()
	data := append([]byte("ID3"), bytes.Repeat([]byte{0x00}, 64)...)
	require.Equal(t, "audio/mpeg", http.DetectContentType(data))
	return data
}

func TestMediaProcessor_initializeAWSClients(t *testing.T) {
	prevLoad := loadAWSConfig
	prevNewS3 := newS3ClientFromConfig
	prevNewMC := newMediaConvertClientFromConfig
	t.Cleanup(func() {
		loadAWSConfig = prevLoad
		newS3ClientFromConfig = prevNewS3
		newMediaConvertClientFromConfig = prevNewMC
	})

	mp := &MediaProcessor{logger: zaptest.NewLogger(t)}

	loadAWSConfig = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, stdErrors.New("nope")
	}
	err := mp.initializeAWSClients(context.Background())
	require.Error(t, err)

	fakeS3 := &fakeS3Client{}
	fakeMC := &fakeMediaConvertClient{jobID: "job-123"}
	loadAWSConfig = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, nil
	}
	newS3ClientFromConfig = func(_ aws.Config) s3Client { return fakeS3 }
	newMediaConvertClientFromConfig = func(_ aws.Config) mediaConvertClient { return fakeMC }

	mp.mediaConvertEndpoint = "https://example.test"
	require.NoError(t, mp.initializeAWSClients(context.Background()))
	require.NotNil(t, mp.s3Client)
	require.NotNil(t, mp.mediaConvertClient)
}

func TestMediaProcessor_processMediaJob_BudgetSkipAndHappyPathImage(t *testing.T) {
	logger := zaptest.NewLogger(t)

	mediaID := "media-1"
	jobID := "job-1"
	username := "alice"

	jpegData := mustJPEGBytes(t, 1, 1)

	fakeRepo := &fakeMediaRepo{
		jobs: map[string]*models.MediaJob{
			jobID: {
				JobID:             jobID,
				MediaID:           mediaID,
				Username:          username,
				Status:            models.StatusPending,
				S3Key:             "uploads/original.jpg",
				MimeType:          "image/jpeg",
				FileSize:          int64(len(jpegData)),
				MaxProcessingTime: 2 * time.Second,
				MaxRetries:        3,
			},
			"job-2": {
				JobID:             "job-2",
				MediaID:           "media-2",
				Username:          username,
				Status:            models.StatusPending,
				S3Key:             "uploads/original2.jpg",
				MimeType:          "image/jpeg",
				FileSize:          int64(len(jpegData)),
				MaxProcessingTime: 2 * time.Second,
				MaxRetries:        3,
			},
		},
		configs: map[string]*models.UserMediaConfig{
			username: {
				UserID:                 username,
				Username:               username,
				MonthlyBudgetMicros:    50, // force budget skip (estimate is 100 for images)
				VideoProcessingEnabled: true,
				AudioProcessingEnabled: true,
				VideoThumbnailsEnabled: true,
				MaxVideoDuration:       0,
			},
		},
	}

	fakeS3 := &fakeS3Client{
		objects: map[string][]byte{
			"uploads/original.jpg":  jpegData,
			"uploads/original2.jpg": jpegData,
		},
	}
	fakeMeta := &fakeMediaMetadataRepo{completeErr: stdErrors.New("non-fatal metadata failure")}

	mp := &MediaProcessor{
		mediaRepo:         fakeRepo,
		mediaMetadataRepo: fakeMeta,
		mediaAnalyticsRepo: &fakeMediaAnalyticsRepo{
			err: nil,
		},
		s3Client:       fakeS3,
		unifiedTracker: &fakeUnifiedTracker{},
		bucketName:     "bucket",
		logger:         logger,
		emfMetrics:     observability.NewEMFMetrics(logger, "test", "media-processor"),
	}

	err := mp.processMediaJob(context.Background(), MediaProcessingEvent{
		JobID:    jobID,
		MediaID:  mediaID,
		Username: username,
	})
	require.NoError(t, err)

	// Now bump budget and exercise the full image pipeline on a separate job.
	fakeRepo.configs[username].MonthlyBudgetMicros = 5_000_000
	err = mp.processMediaJob(context.Background(), MediaProcessingEvent{
		JobID:    "job-2",
		MediaID:  "media-2",
		Username: username,
	})
	require.NoError(t, err)

	require.NotEmpty(t, fakeS3.objects)
}

func TestMediaProcessor_processMediaJob_IdempotencyAndLocking(t *testing.T) {
	logger := zaptest.NewLogger(t)
	now := time.Now()
	old := now.Add(-2 * time.Hour)

	repo := &fakeMediaRepo{
		jobs: map[string]*models.MediaJob{
			"completed": {
				JobID:    "completed",
				MediaID:  "m",
				Username: "u",
				Status:   models.StatusCompleted,
				S3Key:    "k",
				MimeType: "image/jpeg",
			},
			"cancelled": {
				JobID:    "cancelled",
				MediaID:  "m",
				Username: "u",
				Status:   models.MediaStatusCancelled,
				S3Key:    "k",
				MimeType: "image/jpeg",
			},
			"processing": {
				JobID:               "processing",
				MediaID:             "m",
				Username:            "u",
				Status:              models.StatusProcessing,
				S3Key:               "k",
				MimeType:            "image/jpeg",
				ProcessingStartedAt: &now,
				LastAttemptAt:       &now,
			},
			"abandoned": {
				JobID:               "abandoned",
				MediaID:             "m",
				Username:            "u",
				Status:              models.StatusProcessing,
				S3Key:               "k",
				MimeType:            "image/jpeg",
				ProcessingStartedAt: &old,
				LastAttemptAt:       &old,
				MaxProcessingTime:   2 * time.Second,
				MaxRetries:          1,
			},
		},
		updateJobErr: stdErrors.New("optimistic lock failed"),
	}

	mp := &MediaProcessor{
		mediaRepo:         repo,
		mediaMetadataRepo: &fakeMediaMetadataRepo{},
		s3Client:          &fakeS3Client{objects: map[string][]byte{"k": mustJPEGBytes(t, 1, 1)}},
		unifiedTracker:    &fakeUnifiedTracker{},
		bucketName:        "bucket",
		logger:            logger,
	}

	require.NoError(t, mp.processMediaJob(context.Background(), MediaProcessingEvent{JobID: "completed", MediaID: "m", Username: "u"}))
	require.NoError(t, mp.processMediaJob(context.Background(), MediaProcessingEvent{JobID: "cancelled", MediaID: "m", Username: "u"}))
	require.NoError(t, mp.processMediaJob(context.Background(), MediaProcessingEvent{JobID: "processing", MediaID: "m", Username: "u"}))

	// Abandoned job attempts to acquire lock; our repo returns an update error and the processor should not fail hard.
	require.NoError(t, mp.processMediaJob(context.Background(), MediaProcessingEvent{JobID: "abandoned", MediaID: "m", Username: "u"}))
}

func TestMediaProcessor_processVideo_Audio_And_CostHelpers(t *testing.T) {
	logger := zaptest.NewLogger(t)

	repo := &fakeMediaRepo{
		configs: map[string]*models.UserMediaConfig{
			"alice": {
				UserID:                 "alice",
				Username:               "alice",
				MonthlyBudgetMicros:    5_000_000,
				VideoProcessingEnabled: true,
				AudioProcessingEnabled: true,
				VideoThumbnailsEnabled: true,
				MaxVideoDuration:       0,
			},
		},
	}

	fakeS3 := &fakeS3Client{}
	fakeMC := &fakeMediaConvertClient{jobID: "mc-job"}

	mp := &MediaProcessor{
		mediaRepo:            repo,
		mediaMetadataRepo:    &fakeMediaMetadataRepo{},
		mediaAnalyticsRepo:   &fakeMediaAnalyticsRepo{},
		s3Client:             fakeS3,
		mediaConvertClient:   fakeMC,
		unifiedTracker:       &fakeUnifiedTracker{},
		bucketName:           "bucket",
		cdnDomain:            "cdn.example.test",
		mediaConvertRole:     "arn:aws:iam::123:role/test",
		mediaConvertQueue:    "https://queue.example.test",
		mediaConvertEndpoint: "https://mediaconvert.example.test",
		logger:               logger,
		emfMetrics:           observability.NewEMFMetrics(logger, "test", "media-processor"),
	}

	videoRes, err := mp.processVideo(context.Background(), testMP4Header(t), MediaProcessingEvent{
		JobID:    "job-v",
		MediaID:  "media-v",
		Username: "alice",
	}, []string{"thumbnails"})
	require.NoError(t, err)
	require.Contains(t, videoRes.Sizes, "original")
	require.NotEmpty(t, videoRes.ProcessingJobID)

	audioRes, err := mp.processAudio(context.Background(), testMP3Header(t), MediaProcessingEvent{
		JobID:    "job-a",
		MediaID:  "media-a",
		Username: "alice",
	}, nil)
	require.NoError(t, err)
	require.Contains(t, audioRes.Sizes, "original")

	assert.Equal(t, costCategoryProcessing, mp.getCategoryFromService(serviceMediaConvert))
	assert.Equal(t, costCategoryCompute, mp.getCategoryFromService("something_else"))
	assert.Equal(t, "video_transcode", mp.getOperationFromService(serviceMediaConvert))
	assert.Equal(t, "media_process", mp.getOperationFromService("something_else"))

	w, h := getResolutionFromMetrics(&TranscodingJobMetrics{InputSize: 10 * 1024 * 1024})
	require.Equal(t, 854, w)
	require.Equal(t, 480, h)
}

func TestTrackTranscodingCosts_VariantsAndNoVariants(t *testing.T) {
	logger := zaptest.NewLogger(t)

	repo := &fakeMediaRepo{
		addTxnErrBySvc: map[string]error{
			serviceRekognition: stdErrors.New("txn write failed"),
		},
	}
	analyticsRepo := &fakeMediaAnalyticsRepo{}

	mp := &MediaProcessor{
		mediaRepo:          repo,
		mediaAnalyticsRepo: analyticsRepo,
		logger:             logger,
	}

	withVariants := &TranscodingJobMetrics{
		JobID:          "job",
		MediaID:        "media",
		Username:       "alice",
		InputFormat:    "video/mp4",
		InputSize:      123,
		InputDuration:  61 * 1000,
		OutputVariants: map[string]string{"720p_h264_1000": "mp4"},
		OutputSizes:    map[string]int64{"720p_h264_1000": 1000},
		CostBreakdown: map[string]int64{
			serviceMediaConvert: 1000,
			serviceS3Storage:    200,
			serviceRekognition:  10,
		},
		ProcessingTimeMs: 50,
		TotalCostMicros:  1210,
		Status:           "completed",
	}
	mp.trackTranscodingCosts(context.Background(), withVariants)
	require.NotEmpty(t, analyticsRepo.analytics)

	noVariants := &TranscodingJobMetrics{
		JobID:          "job2",
		MediaID:        "media2",
		Username:       "alice",
		InputFormat:    "video/mp4",
		InputSize:      456,
		InputDuration:  0,
		OutputVariants: map[string]string{},
		OutputSizes:    map[string]int64{},
		CostBreakdown: map[string]int64{
			serviceMediaConvert: 1000,
			serviceS3Storage:    200,
			"lambda_processing": 50,
		},
		ProcessingTimeMs: 10,
		TotalCostMicros:  1250,
		Status:           "failed",
		ErrorMessage:     "oops",
	}
	mp.trackTranscodingCosts(context.Background(), noVariants)
	require.GreaterOrEqual(t, len(analyticsRepo.analytics), 2)
}

func TestCreateEnhancedMediaConvertJob_RoleMissingAndSuccess(t *testing.T) {
	logger := zaptest.NewLogger(t)

	mp := &MediaProcessor{
		mediaRepo:            &fakeMediaRepo{},
		mediaConvertClient:   &fakeMediaConvertClient{jobID: "mc-123"},
		bucketName:           "bucket",
		mediaConvertQueue:    "queue",
		mediaConvertRole:     "",
		logger:               logger,
		emfMetrics:           observability.NewEMFMetrics(logger, "test", "media-processor"),
		mediaAnalyticsRepo:   &fakeMediaAnalyticsRepo{},
		mediaMetadataRepo:    &fakeMediaMetadataRepo{},
		unifiedTracker:       &fakeUnifiedTracker{},
		s3Client:             &fakeS3Client{},
		mediaConvertEndpoint: "endpoint",
	}

	_, err := mp.createEnhancedMediaConvertJob(context.Background(), "in", MediaProcessingEvent{JobID: "j", MediaID: "m", Username: "u"}, &TranscodingPlan{
		QualityLevels:   []string{"480p"},
		ExpectedOutputs: map[string]int64{"480p": 1},
	})
	require.Error(t, err)

	mp.mediaConvertRole = "arn:aws:iam::123:role/test"
	jobID, err := mp.createEnhancedMediaConvertJob(context.Background(), "in", MediaProcessingEvent{JobID: "j", MediaID: "m", Username: "u"}, &TranscodingPlan{
		QualityLevels:    []string{"480p", "unknown"},
		ExpectedOutputs:  map[string]int64{"480p": 1},
		ThumbnailCount:   int(^int32(0)) + 1,
		AnalysisEnabled:  true,
		MediaConvertCost: 42,
	})
	require.NoError(t, err)
	require.Equal(t, "mc-123", jobID)
}

func TestMediaJobCostTracker_WarningsBudgetExceededAndFinish(t *testing.T) {
	logger := zaptest.NewLogger(t)

	repo := &fakeMediaRepo{
		jobs: map[string]*models.MediaJob{
			"job": {JobID: "job", MediaID: "m", Username: "u", Status: models.StatusPending, S3Key: "k", MimeType: "image/jpeg"},
		},
	}

	mp := &MediaProcessor{mediaRepo: repo, logger: logger}
	ct := mp.NewMediaJobCostTracker("job", "u", "image/jpeg", 11*1024*1024)
	require.NotNil(t, ct)

	// Cross warning thresholds and exceed budget.
	require.NoError(t, ct.AddCost(models.MediaCostUpload, ct.BudgetMicros/2))
	require.NoError(t, ct.AddCost(models.MediaCostUpload, ct.BudgetMicros/4))

	err := ct.AddCost(models.MediaCostUpload, ct.BudgetMicros)
	require.Error(t, err)

	// Finish tracking should persist a spending transaction.
	ct.TotalCostMicros = 1234
	require.NoError(t, ct.FinishTracking())
	require.NotEmpty(t, repo.transactions)
}

func TestMediaProcessor_HandleSQSMessage_AWSInitError(t *testing.T) {
	prevLoad := loadAWSConfig
	t.Cleanup(func() { loadAWSConfig = prevLoad })

	loadAWSConfig = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, stdErrors.New("no aws")
	}

	mp := &MediaProcessor{logger: zaptest.NewLogger(t)}
	err := mp.HandleSQSMessage(nil, events.SQSMessage{MessageId: "m1", Body: `{}`})
	require.Error(t, err)
}

func TestMediaProcessor_HandleSQSMessage_ParsesMessagesAndRetriesOnFailure(t *testing.T) {
	logger := zaptest.NewLogger(t)

	prevLoad := loadAWSConfig
	prevNewS3 := newS3ClientFromConfig
	prevNewMC := newMediaConvertClientFromConfig
	t.Cleanup(func() {
		loadAWSConfig = prevLoad
		newS3ClientFromConfig = prevNewS3
		newMediaConvertClientFromConfig = prevNewMC
	})

	loadAWSConfig = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, nil
	}
	fakeS3 := &fakeS3Client{getErr: stdErrors.New("s3 down")}
	newS3ClientFromConfig = func(_ aws.Config) s3Client { return fakeS3 }
	newMediaConvertClientFromConfig = func(_ aws.Config) mediaConvertClient { return &fakeMediaConvertClient{jobID: "mc"} }

	job := &models.MediaJob{
		JobID:             "job",
		MediaID:           "media",
		Username:          "alice",
		Status:            models.StatusPending,
		S3Key:             "k",
		MimeType:          "image/jpeg",
		MaxProcessingTime: 2 * time.Second,
		MaxRetries:        1,
	}

	repo := &fakeMediaRepo{
		jobs: map[string]*models.MediaJob{"job": job},
		configs: map[string]*models.UserMediaConfig{
			"alice": {UserID: "alice", Username: "alice", MonthlyBudgetMicros: 5_000_000, MaxVideoDuration: 0, VideoProcessingEnabled: true, AudioProcessingEnabled: true, VideoThumbnailsEnabled: true},
		},
		updateJobFn: func(_ context.Context, j *models.MediaJob) error {
			// Fail updates after the first attempt to cover the retry handler logging path.
			if j.RetryCount > 0 {
				return stdErrors.New("update failed")
			}
			return nil
		},
	}

	mp := &MediaProcessor{
		mediaRepo:         repo,
		mediaMetadataRepo: &fakeMediaMetadataRepo{},
		mediaAnalyticsRepo: &fakeMediaAnalyticsRepo{
			err: nil,
		},
		unifiedTracker: &fakeUnifiedTracker{},
		bucketName:     "bucket",
		logger:         logger,
	}

	validBody, _ := json.Marshal(MediaProcessingEvent{JobID: "job", MediaID: "media", Username: "alice"})
	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{Body: `{"not":"json"`},
			{Body: string(validBody)},
		},
	}

	for _, msg := range event.Records {
		require.NoError(t, mp.HandleSQSMessage(nil, msg))
	}
}

func TestErrorsFile_Constructors(t *testing.T) {
	assert.NotNil(t, MediaConvertRoleNotConfigured())
	assert.NotNil(t, AWSConfigLoadFailed(stdErrors.New("x")))
	assert.NotNil(t, S3GetObjectFailed(stdErrors.New("x")))
	assert.NotNil(t, S3ReadObjectFailed(stdErrors.New("x")))
	assert.NotNil(t, S3UploadVideoFailed(stdErrors.New("x")))
	assert.NotNil(t, S3UploadOriginalFailed(stdErrors.New("x")))
	assert.NotNil(t, S3KeySanitizationFailed(stdErrors.New("x")))
	assert.NotNil(t, S3KeySanitizationFailed(nil))
	assert.NotNil(t, JobGetFailed(stdErrors.New("x")))
	assert.NotNil(t, JobUpdateStatusFailed(stdErrors.New("x")))
	assert.NotNil(t, JobUpdateWarningFailed(stdErrors.New("x")))
	assert.NotNil(t, MediaDownloadFailed(stdErrors.New("x")))
	assert.NotNil(t, MediaRecordUpdateFailed(stdErrors.New("x")))
	assert.NotNil(t, ImageProcessingFailed(stdErrors.New("x")))
	assert.NotNil(t, VideoValidationFailed(stdErrors.New("x")))
	assert.NotNil(t, VideoValidationFailed(nil))
	assert.NotNil(t, AudioMetadataReadFailed(stdErrors.New("x")))
	assert.NotNil(t, AudioMetadataReadFailed(nil))
	assert.NotNil(t, EmptyFileError())
	assert.NotNil(t, FileTypeNotAllowedError("x"))
	assert.NotNil(t, UnsupportedMediaTypeError("x"))
	assert.NotNil(t, UnknownFileTypeError("x"))
	assert.NotNil(t, FileTooLargeError(1, 2))
	assert.NotNil(t, VideoDurationExceededError(10, 3))
	assert.NotNil(t, UnsupportedMediaTypeForUserError("x"))
	assert.NotNil(t, FileSizeExceedsUserLimitError(1, 2))
	assert.NotNil(t, FileValidationFailedError(stdErrors.New("x")))
	assert.NotNil(t, FileValidationFailedError(nil))
	assert.NotNil(t, InvalidMimeTypeFormatError("x"))
	assert.NotNil(t, DetectedMimeTypeInvalidError("x"))
	assert.NotNil(t, UnableToDetermineAudioDurationError(stdErrors.New("x")))
	assert.NotNil(t, UnableToDetermineAudioDurationError(nil))
	assert.NotNil(t, InvalidUsernameForS3KeyError("x"))
	assert.NotNil(t, InvalidMediaIDForS3KeyError("x"))
	assert.NotNil(t, InvalidFilenameForS3KeyError("x"))
	assert.NotNil(t, MimeTypeMismatchError("a", "b"))
	assert.NotNil(t, BudgetExceededError(1, 2))
	assert.NotNil(t, UnsupportedMediaTypeProcessingError("x"))
	assert.NotNil(t, FileTooLargeForTypeError(1, 2, "x"))
	assert.NotNil(t, MimeTypeMismatchDetailedError("a", "b"))
	assert.NotNil(t, UnknownFileTypeForProcessingError("x"))
	assert.NotNil(t, FileSizeExceedsLimitError(1, 2))
	assert.NotNil(t, VideoDurationTooLongError(10, 3))
	assert.NotNil(t, UnsupportedForUserError("x"))
	assert.NotNil(t, BudgetExceededForJobError("j", 1, 2))
	assert.NotNil(t, EnhancedMediaConvertJobCreationFailed(stdErrors.New("x")))
	assert.NotNil(t, S3KeySanitizationAudioFailed(stdErrors.New("x")))
	assert.NotNil(t, S3KeySanitizationAudioFailed(nil))
	assert.NotNil(t, AudioUploadFailed(stdErrors.New("x")))
}

func TestMediaProcessor_handleProcessingError_ClassifiesPermanence(t *testing.T) {
	logger := zaptest.NewLogger(t)

	job := &models.MediaJob{
		JobID:      "job",
		MediaID:    "media",
		Username:   "alice",
		Status:     models.StatusProcessing,
		MimeType:   "image/jpeg",
		MaxRetries: 1,
	}

	repo := &fakeMediaRepo{
		jobs:         map[string]*models.MediaJob{"job": job},
		updateJobErr: stdErrors.New("update failed"),
	}

	mp := &MediaProcessor{
		mediaRepo:         repo,
		logger:            logger,
		emfMetrics:        observability.NewEMFMetrics(logger, "test", "media-processor"),
		alertManager:      monitoring.NewAlertManager(logger),
		mediaMetadataRepo: &fakeMediaMetadataRepo{},
	}

	mp.handleProcessingError(context.Background(), job, stdErrors.New("invalid format"))
	require.True(t, mp.isPermanentError(stdErrors.New("unsupported file type")))
	require.False(t, mp.isPermanentError(stdErrors.New("transient network timeout")))
}

func TestMediaProcessor_Main_RegistersSQSHandlerAndStartsLambda(t *testing.T) {
	prevProcessor := processor
	prevStart := startLambda
	t.Cleanup(func() {
		processor = prevProcessor
		startLambda = prevStart
	})

	startCalls := 0
	startLambda = func(handler interface{}) {
		startCalls++
		h, ok := handler.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)

		event := events.SQSEvent{
			Records: []events.SQSMessage{
				{
					MessageId:      "1",
					Body:           "{bad json",
					EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-dev-media-processor-queue",
					EventSource:    "aws:sqs",
				},
			},
		}

		raw, err := json.Marshal(event)
		require.NoError(t, err)

		respAny, err := h(context.Background(), raw)
		require.NoError(t, err)
		resp, ok := respAny.(events.SQSEventResponse)
		require.True(t, ok)
		require.Empty(t, resp.BatchItemFailures)
	}

	t.Setenv("APP_NAME", "lesser")
	t.Setenv("STAGE", "dev")
	t.Setenv("ENVIRONMENT", "dev")

	processor = &MediaProcessor{
		logger:             zaptest.NewLogger(t),
		s3Client:           &fakeS3Client{},
		mediaConvertClient: &fakeMediaConvertClient{jobID: "mc"},
	}

	main()
	require.Equal(t, 1, startCalls)
}

func TestMediaProcessor_ConfigAndValidationBranches(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// getUserMediaConfig: not found -> create default -> returned config reflects stored model.
	repo := &fakeMediaRepo{}
	mp := &MediaProcessor{mediaRepo: repo, logger: logger}
	cfg := mp.getUserMediaConfig(context.Background(), "alice")
	require.NotNil(t, cfg)

	// getUserMediaConfig: create default fails -> in-memory defaults.
	repo.createConfigErr = stdErrors.New("create failed")
	cfg = mp.getUserMediaConfig(context.Background(), "bob")
	require.True(t, cfg.VideoProcessingEnabled)

	// getUserMediaConfig: non-not-found error -> in-memory defaults.
	repo.createConfigErr = nil
	repo.getConfigErr = stdErrors.New("db error")
	cfg = mp.getUserMediaConfig(context.Background(), "carol")
	require.True(t, cfg.AudioProcessingEnabled)

	// getUserRemainingBudget branches.
	repo.getConfigErr = nil
	repo.configs = map[string]*models.UserMediaConfig{
		"dave": {UserID: "dave", Username: "dave", MonthlyBudgetMicros: 1000},
	}
	require.Equal(t, int64(1000), mp.getUserRemainingBudget(context.Background(), "dave"))

	period := time.Now().Format(common.MonthFormat)
	repo.spendingByPeriod = map[string]*models.MediaSpending{
		"dave|" + period: {UserID: "dave", Period: period, TotalSpendMicros: 250},
		"erin|" + period: {UserID: "erin", Period: period, TotalSpendMicros: 5000},
	}
	repo.configs["erin"] = &models.UserMediaConfig{UserID: "erin", Username: "erin", MonthlyBudgetMicros: 1000}
	require.Equal(t, int64(750), mp.getUserRemainingBudget(context.Background(), "dave"))
	require.Equal(t, int64(0), mp.getUserRemainingBudget(context.Background(), "erin"))

	repo.getSpendingErr = stdErrors.New("db down")
	require.Equal(t, int64(1000), mp.getUserRemainingBudget(context.Background(), "dave"))

	// validateFileForUser unsupported type branch (mimeType empty).
	err := mp.validateFileForUser(mustJPEGBytes(t, 1, 1), "", mp.getDefaultMediaConfig(), "user", "media")
	require.Error(t, err)

	// validateFileForUser video duration branch (metadata parsing error).
	mp.logger = logger
	videoCfg := &MediaConfig{MaxVideoDuration: 1, VideoProcessingEnabled: true}
	err = mp.validateFileForUser(testMP4Header(t), "video/mp4", videoCfg, "user", "media")
	require.Error(t, err)

	// validateFileForUser user limit branch for GIF: >10MB but <15MB should pass type check and fail user max.
	gifData := make([]byte, 11*1024*1024)
	copy(gifData, []byte("GIF89a"))
	err = mp.validateFileForUser(gifData, "image/gif", mp.getDefaultMediaConfig(), "user", "media")
	require.Error(t, err)

	// getVideoMetadata error path (invalid MP4 data).
	_, err = mp.getVideoMetadata(testMP4Header(t), "video/mp4")
	require.Error(t, err)
}

func TestMediaProcessor_handleJobFailure_SchedulesRetry(t *testing.T) {
	logger := zaptest.NewLogger(t)

	job := &models.MediaJob{
		JobID:      "job",
		MediaID:    "media",
		Username:   "alice",
		Status:     models.StatusProcessing,
		MimeType:   "image/jpeg",
		MaxRetries: 20,
		RetryCount: -1,
	}

	repo := &fakeMediaRepo{jobs: map[string]*models.MediaJob{"job": job}}
	mp := &MediaProcessor{mediaRepo: repo, logger: logger}

	require.NoError(t, mp.handleJobFailure(context.Background(), job, stdErrors.New("transient timeout")))
	require.Equal(t, models.StatusPending, job.Status)

	// Permanent error: no retry scheduling.
	job.Status = models.StatusProcessing
	job.RetryCount = 0
	require.NoError(t, mp.handleJobFailure(context.Background(), job, stdErrors.New("invalid format")))
}

func TestMediaProcessor_uploadOriginalOnly_UpdateStorageAndHelpers(t *testing.T) {
	logger := zaptest.NewLogger(t)

	repo := &fakeMediaRepo{
		configs: map[string]*models.UserMediaConfig{
			"user": {UserID: "user", Username: "user", MonthlyBudgetMicros: 1000},
		},
	}
	mp := &MediaProcessor{
		mediaRepo:      repo,
		logger:         logger,
		bucketName:     "bucket",
		s3Client:       &fakeS3Client{},
		unifiedTracker: &fakeUnifiedTracker{err: stdErrors.New("track failed")},
	}

	_, err := mp.uploadOriginalOnly(context.Background(), mustJPEGBytes(t, 1, 1), MediaProcessingEvent{JobID: "j", MediaID: "m", Username: "user"}, mimeJPEG)
	require.NoError(t, err)
	_, err = mp.uploadOriginalOnly(context.Background(), createTestPNGData(), MediaProcessingEvent{JobID: "j", MediaID: "m", Username: "user"}, mimePNG)
	require.NoError(t, err)
	_, err = mp.uploadOriginalOnly(context.Background(), testMP3Header(t), MediaProcessingEvent{JobID: "j", MediaID: "m", Username: "user"}, "audio/mpeg")
	require.NoError(t, err)
	_, err = mp.uploadOriginalOnly(context.Background(), []byte("data"), MediaProcessingEvent{JobID: "j", MediaID: "m", Username: "user"}, "application/octet-stream")
	require.NoError(t, err)

	// Sanitization error (username path traversal).
	_, err = mp.uploadOriginalOnly(context.Background(), mustJPEGBytes(t, 1, 1), MediaProcessingEvent{JobID: "j", MediaID: "m", Username: "../bad"}, mimeJPEG)
	require.Error(t, err)

	// Upload error.
	mp.s3Client = &fakeS3Client{putErr: stdErrors.New("put failed")}
	_, err = mp.uploadOriginalOnly(context.Background(), mustJPEGBytes(t, 1, 1), MediaProcessingEvent{JobID: "j", MediaID: "m", Username: "user"}, "video/mp4")
	require.Error(t, err)

	// updateStorageUsageForUser: missing config is non-fatal.
	repoMissing := &fakeMediaRepo{}
	mp2 := &MediaProcessor{mediaRepo: repoMissing, logger: logger}
	require.NoError(t, mp2.updateStorageUsageForUser(context.Background(), "missing", 123))

	// updateStorageUsageForUser: update error propagates.
	repoUpdateErr := &fakeMediaRepo{
		configs:         map[string]*models.UserMediaConfig{"user": {UserID: "user", Username: "user"}},
		updateConfigErr: stdErrors.New("update failed"),
	}
	mp3 := &MediaProcessor{mediaRepo: repoUpdateErr, logger: logger}
	require.Error(t, mp3.updateStorageUsageForUser(context.Background(), "user", 123))
}

func TestMediaProcessor_processMediaJob_ErrorPaths(t *testing.T) {
	logger := zaptest.NewLogger(t)

	meta := &fakeMediaMetadataRepo{
		startErr: stdErrors.New("start failed"),
		failErr:  stdErrors.New("mark failed"),
	}

	jpegHeaderOnly := createTestJPEGData()
	repo := &fakeMediaRepo{
		jobs: map[string]*models.MediaJob{
			"bad-image": {
				JobID:             "bad-image",
				MediaID:           "m1",
				Username:          "user",
				Status:            models.StatusPending,
				S3Key:             "k1",
				MimeType:          "image/jpeg",
				FileSize:          int64(len(jpegHeaderOnly)),
				MaxProcessingTime: 2 * time.Second,
				MaxRetries:        1,
			},
			"mismatch": {
				JobID:             "mismatch",
				MediaID:           "m2",
				Username:          "user",
				Status:            models.StatusPending,
				S3Key:             "k2",
				MimeType:          "image/jpeg",
				FileSize:          int64(len(mustJPEGBytes(t, 1, 1))),
				MaxProcessingTime: 2 * time.Second,
				MaxRetries:        1,
			},
		},
		configs: map[string]*models.UserMediaConfig{
			"user": {UserID: "user", Username: "user", MonthlyBudgetMicros: 5_000_000, MaxVideoDuration: 0, VideoProcessingEnabled: true, AudioProcessingEnabled: true, VideoThumbnailsEnabled: true},
		},
	}

	s3c := &fakeS3Client{
		objects: map[string][]byte{
			"k1": jpegHeaderOnly,
			"k2": []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, // PNG header, will mismatch claimed image/jpeg
		},
	}

	mp := &MediaProcessor{
		mediaRepo:         repo,
		mediaMetadataRepo: meta,
		s3Client:          s3c,
		bucketName:        "bucket",
		logger:            logger,
		emfMetrics:        observability.NewEMFMetrics(logger, "test", "media-processor"),
	}

	// Image decoding failure triggers processing error handling.
	err := mp.processMediaJob(context.Background(), MediaProcessingEvent{JobID: "bad-image", MediaID: "m1", Username: "user"})
	require.Error(t, err)

	// MIME mismatch fails validation early.
	err = mp.processMediaJob(context.Background(), MediaProcessingEvent{JobID: "mismatch", MediaID: "m2", Username: "user"})
	require.Error(t, err)
}

func TestTranscodingHelpers_MoreBranchCoverage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mp := &MediaProcessor{
		mediaRepo:          &fakeMediaRepo{},
		mediaAnalyticsRepo: &fakeMediaAnalyticsRepo{},
		logger:             logger,
	}

	// getResolutionFromMetrics branches.
	w, h := getResolutionFromMetrics(&TranscodingJobMetrics{InputSize: 120 * 1024 * 1024})
	require.Equal(t, 1920, w)
	require.Equal(t, 1080, h)
	w, h = getResolutionFromMetrics(&TranscodingJobMetrics{InputSize: 60 * 1024 * 1024})
	require.Equal(t, 1280, w)
	require.Equal(t, 720, h)

	// createQualityOutput branches.
	for _, q := range []string{"2160p", "1080p", "720p", "480p", "unknown"} {
		out := mp.createQualityOutput(q)
		require.NotNil(t, out.VideoDescription)
	}

	// sliceContains branches.
	require.True(t, sliceContains([]string{"a", "b"}, "a"))
	require.False(t, sliceContains([]string{"a", "b"}, "c"))

	// getCategoryFromService and getOperationFromService remaining cases.
	require.Equal(t, costCategoryStorage, mp.getCategoryFromService(serviceS3Upload))
	require.Equal(t, costCategoryStorage, mp.getCategoryFromService(serviceS3Storage))
	require.Equal(t, costCategoryBandwidth, mp.getCategoryFromService(serviceCloudFront))
	require.Equal(t, costCategoryProcessing, mp.getCategoryFromService(serviceThumbnails))
	require.Equal(t, "storage_put", mp.getOperationFromService(serviceS3Upload))
	require.Equal(t, "storage_monthly", mp.getOperationFromService(serviceS3Storage))
	require.Equal(t, "cdn_transfer", mp.getOperationFromService(serviceCloudFront))
	require.Equal(t, "thumbnail_generation", mp.getOperationFromService(serviceThumbnails))

	// getUnitsFromService thumbnails cap.
	units := mp.getUnitsFromService(serviceThumbnails, &TranscodingJobMetrics{InputDuration: 30 * 60 * 1000})
	require.Equal(t, int64(10), units)

	// createEnhancedMediaConvertJob: no thumbnails group.
	mp.mediaConvertClient = &fakeMediaConvertClient{jobID: "mc"}
	mp.bucketName = "bucket"
	mp.mediaConvertRole = "arn:aws:iam::123:role/test"
	mp.mediaConvertQueue = "queue"
	_, err := mp.createEnhancedMediaConvertJob(context.Background(), "in", MediaProcessingEvent{JobID: "j", MediaID: "m", Username: "u"}, &TranscodingPlan{
		QualityLevels:   []string{"480p"},
		ExpectedOutputs: map[string]int64{"480p": 1},
		ThumbnailCount:  0,
	})
	require.NoError(t, err)
}

func TestMediaProcessor_estimateTranscodingCosts_QualityThumbnailsAndAnalysis(t *testing.T) {
	mp := &MediaProcessor{logger: zaptest.NewLogger(t)}

	metrics := &TranscodingJobMetrics{
		JobID:          "job",
		MediaID:        "media",
		Username:       "user",
		InputSize:      120 * 1024 * 1024,   // triggers 1080p plan
		InputDuration:  20 * 60 * 1000,      // 20 minutes -> thumbnail cap at 10
		CostBreakdown:  map[string]int64{},  // populated by caller in real flows
		OutputVariants: map[string]string{}, // unused here
		OutputSizes:    map[string]int64{},  // unused here
	}

	cfg := &MediaConfig{
		VideoThumbnailsEnabled:   true,
		ContentModerationEnabled: true,
	}

	plan, total := mp.estimateTranscodingCosts(metrics, cfg)
	require.NotNil(t, plan)
	require.Greater(t, total, int64(0))
	require.Contains(t, plan.QualityLevels, "1080p")
	require.Contains(t, plan.QualityLevels, "720p")
	require.Contains(t, plan.QualityLevels, "480p")
	require.Equal(t, 10, plan.ThumbnailCount)
	require.True(t, plan.AnalysisEnabled)
}

func TestMediaProcessor_processAudioWithCostTracking_Branches(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Audio disabled -> uploadOriginalOnly
	repo := &fakeMediaRepo{
		configs: map[string]*models.UserMediaConfig{
			"user": {UserID: "user", Username: "user", MonthlyBudgetMicros: 5_000_000, AudioProcessingEnabled: false},
		},
	}
	mp := &MediaProcessor{
		mediaRepo:      repo,
		logger:         logger,
		bucketName:     "bucket",
		s3Client:       &fakeS3Client{},
		unifiedTracker: &fakeUnifiedTracker{},
	}
	_, err := mp.processAudioWithCostTracking(context.Background(), testMP3Header(t), MediaProcessingEvent{JobID: "j", MediaID: "m", Username: "user"}, nil)
	require.NoError(t, err)

	// Budget exceeded -> uploadOriginalOnly (need non-zero estimated cost).
	audioData := make([]byte, 10*1024*1024)
	copy(audioData, []byte("ID3"))
	repo.configs["user"].AudioProcessingEnabled = true
	repo.configs["user"].MonthlyBudgetMicros = 1
	_, err = mp.processAudioWithCostTracking(context.Background(), audioData, MediaProcessingEvent{JobID: "j", MediaID: "m", Username: "user"}, nil)
	require.NoError(t, err)

	// Sanitization error.
	repo.configs["user"].MonthlyBudgetMicros = 5_000_000
	_, err = mp.processAudioWithCostTracking(context.Background(), audioData, MediaProcessingEvent{JobID: "j", MediaID: "../bad", Username: "user"}, nil)
	require.Error(t, err)

	// Upload error.
	mp.s3Client = &fakeS3Client{putErr: stdErrors.New("put failed")}
	_, err = mp.processAudioWithCostTracking(context.Background(), audioData, MediaProcessingEvent{JobID: "j", MediaID: "m", Username: "user"}, nil)
	require.Error(t, err)
}

func TestMediaProcessor_validateFileForUser_CommonValidationError(t *testing.T) {
	mp := &MediaProcessor{logger: zaptest.NewLogger(t)}
	err := mp.validateFileForUser(mustJPEGBytes(t, 1, 1), "image/jpeg", mp.getDefaultMediaConfig(), "user", "")
	require.Error(t, err)
}

func TestMediaProcessor_isJobAbandoned_Branches(t *testing.T) {
	mp := &MediaProcessor{}
	now := time.Now()

	require.False(t, mp.isJobAbandoned(&models.MediaJob{Status: models.StatusPending}))
	require.False(t, mp.isJobAbandoned(&models.MediaJob{Status: models.StatusProcessing}))
	require.True(t, mp.isJobAbandoned(&models.MediaJob{Status: models.StatusProcessing, LastAttemptAt: ptrTime(now.Add(-2 * time.Hour))}))
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func TestCreateEnhancedMediaConvertJob_ThumbnailWithinRangeAndClientError(t *testing.T) {
	logger := zaptest.NewLogger(t)

	mp := &MediaProcessor{
		logger:             logger,
		bucketName:         "bucket",
		mediaConvertQueue:  "queue",
		mediaConvertRole:   "arn:aws:iam::123:role/test",
		mediaConvertClient: &fakeMediaConvertClient{jobID: "mc-ok"},
	}

	plan := &TranscodingPlan{
		QualityLevels:   []string{"480p"},
		ExpectedOutputs: map[string]int64{"480p": 1},
		ThumbnailCount:  2,
	}
	jobID, err := mp.createEnhancedMediaConvertJob(context.Background(), "in", MediaProcessingEvent{JobID: "j", MediaID: "m", Username: "u"}, plan)
	require.NoError(t, err)
	require.Equal(t, "mc-ok", jobID)

	mp.mediaConvertClient = &fakeMediaConvertClient{err: stdErrors.New("mediaconvert down")}
	_, err = mp.createEnhancedMediaConvertJob(context.Background(), "in", MediaProcessingEvent{JobID: "j", MediaID: "m", Username: "u"}, plan)
	require.Error(t, err)
}

func TestMediaProcessor_processMediaJob_JobUpdateStatusFailure(t *testing.T) {
	logger := zaptest.NewLogger(t)

	jpegData := mustJPEGBytes(t, 1, 1)
	job := &models.MediaJob{
		JobID:             "job",
		MediaID:           "media",
		Username:          "user",
		Status:            models.StatusPending,
		S3Key:             "k",
		MimeType:          "image/jpeg",
		FileSize:          int64(len(jpegData)),
		MaxProcessingTime: 2 * time.Second,
		MaxRetries:        1,
	}

	repo := &fakeMediaRepo{
		jobs: map[string]*models.MediaJob{"job": job},
		configs: map[string]*models.UserMediaConfig{
			"user": {UserID: "user", Username: "user", MonthlyBudgetMicros: 50, MaxVideoDuration: 0, VideoProcessingEnabled: true, AudioProcessingEnabled: true, VideoThumbnailsEnabled: true},
		},
		updateJobFn: func(_ context.Context, j *models.MediaJob) error {
			// Allow locking updates, but fail completion persistence.
			if j.IsCompleted() {
				return stdErrors.New("update failed")
			}
			return nil
		},
	}

	mp := &MediaProcessor{
		mediaRepo:         repo,
		mediaMetadataRepo: &fakeMediaMetadataRepo{},
		s3Client:          &fakeS3Client{objects: map[string][]byte{"k": jpegData}},
		bucketName:        "bucket",
		unifiedTracker:    &fakeUnifiedTracker{},
		logger:            logger,
	}

	err := mp.processMediaJob(context.Background(), MediaProcessingEvent{JobID: "job", MediaID: "media", Username: "user"})
	require.Error(t, err)
}

func TestNewMediaProcessor_BadContextReturnsEmptyProcessor(t *testing.T) {
	mp := NewMediaProcessor(nil)
	require.NotNil(t, mp)

	ctx := &common.LambdaContext{
		DynamoDB: struct{}{},
		Repos:    struct{}{},
	}
	mp = NewMediaProcessor(ctx)
	require.NotNil(t, mp)
}

func TestNewMediaProcessor_UsesLambdaContextDependencies(t *testing.T) {
	db := dynamormmocks.NewMockExtendedDB()
	repoStorage, err := factory.NewRepositoryFactory(db, "media-table", zaptest.NewLogger(t))
	require.NoError(t, err)

	start := time.Date(2026, time.March, 17, 12, 0, 0, 0, time.UTC)
	ctx := &common.LambdaContext{
		DynamoDB: db,
		Repos:    repoStorage,
		Config: &config.Config{
			DynamoTableName:        "media-table",
			S3MediaBucket:          "media-bucket",
			CloudFrontDomain:       "cdn.example.com",
			MediaConvertEndpoint:   "https://mediaconvert.example.com",
			MediaConvertRoleArn:    "arn:aws:iam::123456789012:role/media",
			MediaProcessorQueueURL: "https://sqs.example.com/media",
		},
		StartTime: start,
		Logger:    zaptest.NewLogger(t),
	}

	mp := NewMediaProcessor(ctx)
	require.Equal(t, db, mp.db)
	require.Equal(t, repoStorage, mp.repos)
	require.NotNil(t, mp.mediaRepo)
	require.NotNil(t, mp.mediaAnalyticsRepo)
	require.NotNil(t, mp.mediaMetadataRepo)
	require.Equal(t, "media-table", mp.tableName)
	require.Equal(t, "media-bucket", mp.bucketName)
	require.Equal(t, "cdn.example.com", mp.cdnDomain)
	require.Equal(t, "https://mediaconvert.example.com", mp.mediaConvertEndpoint)
	require.Equal(t, "arn:aws:iam::123456789012:role/media", mp.mediaConvertRole)
	require.Equal(t, "https://sqs.example.com/media", mp.mediaConvertQueue)
	require.Equal(t, start, mp.startTime)
	require.Equal(t, ctx.Logger, mp.logger)
}
