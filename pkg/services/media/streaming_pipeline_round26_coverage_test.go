package media

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/services/media/transcoding"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type fakeTranscoder struct {
	submitErr    error
	submitResult *transcoding.TranscodeResult
	submitCalls  []*transcoding.TranscodeRequest

	jobToReturn *models.TranscodingJob
}

func (t *fakeTranscoder) SubmitJob(_ context.Context, req *transcoding.TranscodeRequest) (*transcoding.TranscodeResult, error) {
	t.submitCalls = append(t.submitCalls, req)
	if t.submitErr != nil {
		return nil, t.submitErr
	}
	return t.submitResult, nil
}

func (t *fakeTranscoder) ConvertToTranscodingJob(req *transcoding.TranscodeRequest, result *transcoding.TranscodeResult) *models.TranscodingJob {
	if t.jobToReturn != nil {
		return t.jobToReturn
	}
	return &models.TranscodingJob{
		JobID:             result.JobID,
		MediaID:           req.MediaID,
		UserID:            req.UserID,
		Username:          req.Username,
		Status:            "processing",
		MediaConvertJobID: result.MediaConvertJobID,
		OutputVariants:    map[string]string{},
		OutputSizes:       map[string]int64{},
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
}

type fakeManifestService struct {
	info       *transcoding.ManifestInfo
	infoErr    error
	preloadErr error
}

func (m *fakeManifestService) GetManifestInfo(context.Context, string, string) (*transcoding.ManifestInfo, error) {
	if m.infoErr != nil {
		return nil, m.infoErr
	}
	return m.info, nil
}

func (m *fakeManifestService) PreloadManifests(context.Context, []string) error { return m.preloadErr }

type fakeCloudFrontService struct {
	signedURL string
	err       error
	calls     []fakeCloudFrontCall
}

type fakeCloudFrontCall struct {
	mediaID string
	format  string
	quality *string
	ttl     time.Duration
}

func (c *fakeCloudFrontService) SignStreamingURL(mediaID, format string, quality *string, ttl time.Duration) (string, error) {
	c.calls = append(c.calls, fakeCloudFrontCall{mediaID: mediaID, format: format, quality: quality, ttl: ttl})
	if c.err != nil {
		return "", c.err
	}
	return c.signedURL, nil
}

func TestService_SubmitTranscodeJob_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	publisher := streaming.NewMockPublisher()

	t.Run("transcoder_missing", func(t *testing.T) {
		svc := NewService(&MockMediaRepository{}, nil, publisher, &MockJobQueueService{}, logger, "bucket", "cdn.example")
		_, err := svc.SubmitTranscodeJob(ctx, &SubmitTranscodeJobCommand{MediaID: "m1", UserID: "u1"})
		require.ErrorIs(t, err, ErrTranscodingServiceUnavailable)
	})

	t.Run("submit_error_bubbles", func(t *testing.T) {
		svc := NewService(&MockMediaRepository{}, nil, publisher, &MockJobQueueService{}, logger, "bucket", "cdn.example")
		svc.transcoder = &fakeTranscoder{submitErr: stderrors.New("boom")}

		_, err := svc.SubmitTranscodeJob(ctx, &SubmitTranscodeJobCommand{MediaID: "m1", UserID: "u1"})
		require.Error(t, err)
	})

	t.Run("success_records_job_and_marks_processing_nonfatally", func(t *testing.T) {
		mediaRepo := new(MockMediaRepository)
		svc := NewService(mediaRepo, nil, publisher, &MockJobQueueService{}, logger, "bucket", "cdn.example")

		transcoder := &fakeTranscoder{
			submitResult: &transcoding.TranscodeResult{
				JobID:             "job1",
				MediaConvertJobID: "mc1",
				EstimatedCostUSD:  1.23,
				EstimatedDuration: 5 * time.Minute,
				QualityLevels:     []string{"720p"},
				Status:            "SUBMITTED",
			},
			jobToReturn: &models.TranscodingJob{JobID: "job1", MediaID: "m1", Status: "processing"},
		}
		svc.transcoder = transcoder

		mediaRepo.On("CreateTranscodingJob", ctx, mock.Anything).Return(stderrors.New("ignored"))
		mediaRepo.On("MarkMediaProcessing", ctx, "m1").Return(stderrors.New("ignored"))

		result, err := svc.SubmitTranscodeJob(ctx, &SubmitTranscodeJobCommand{
			MediaID:       "m1",
			UserID:        "u1",
			Username:      "alice",
			SourceBucket:  "bucket",
			SourceKey:     "key",
			ContentType:   "video/mp4",
			QualityLevels: []string{"720p"},
			GenerateHLS:   true,
		})
		require.NoError(t, err)
		assert.Equal(t, "job1", result.JobID)
		assert.Equal(t, "mc1", result.MediaConvertJobID)
		mediaRepo.AssertExpectations(t)
	})
}

func TestService_GetMediaRenditions_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	publisher := streaming.NewMockPublisher()

	t.Run("media_get_error", func(t *testing.T) {
		mediaRepo := new(MockMediaRepository)
		svc := NewService(mediaRepo, nil, publisher, &MockJobQueueService{}, logger, "bucket", "cdn.example")
		mediaRepo.On("GetMedia", ctx, "m1").Return((*models.Media)(nil), stderrors.New("boom"))

		_, err := svc.GetMediaRenditions(ctx, "m1")
		require.ErrorIs(t, err, ErrMediaRetrievalFailed)
	})

	t.Run("not_ready_returns_status_only", func(t *testing.T) {
		mediaRepo := new(MockMediaRepository)
		svc := NewService(mediaRepo, nil, publisher, &MockJobQueueService{}, logger, "bucket", "cdn.example")
		mediaRepo.On("GetMedia", ctx, "m1").Return(&models.Media{MediaID: "m1", Status: "processing"}, nil)

		r, err := svc.GetMediaRenditions(ctx, "m1")
		require.NoError(t, err)
		assert.Equal(t, "processing", r.TranscodingStatus)
		assert.Empty(t, r.HLSMasterURL)
	})

	t.Run("manifest_error_falls_back_to_variants", func(t *testing.T) {
		mediaRepo := new(MockMediaRepository)
		svc := NewService(mediaRepo, nil, publisher, &MockJobQueueService{}, logger, "bucket", "cdn.example")
		svc.manifestService = &fakeManifestService{infoErr: stderrors.New("boom")}

		mediaRepo.On("GetMedia", ctx, "m1").Return(&models.Media{
			MediaID:  "m1",
			Status:   "ready",
			S3Bucket: "b",
			CDNUrl:   "https://cdn/master.m3u8",
			Variants: map[string]models.MediaVariant{
				"720p": {S3Key: "m1/hls/720p.m3u8", FileSize: 10, ContentType: "application/vnd.apple.mpegurl"},
			},
		}, nil)

		r, err := svc.GetMediaRenditions(ctx, "m1")
		require.NoError(t, err)
		assert.Equal(t, "https://cdn/master.m3u8", r.HLSMasterURL)
		require.Len(t, r.Variants, 1)
	})

	t.Run("manifest_success_populates_urls", func(t *testing.T) {
		mediaRepo := new(MockMediaRepository)
		svc := NewService(mediaRepo, nil, publisher, &MockJobQueueService{}, logger, "bucket", "cdn.example")
		svc.manifestService = &fakeManifestService{
			info: &transcoding.ManifestInfo{
				MediaID:      "m1",
				HLSMasterURL: "https://cdn/master.m3u8",
				Variants: []transcoding.VariantInfo{
					{Quality: "720p", Width: 1280, Height: 720, Bitrate: 3000000, Codec: "avc1", HLSPlaylistURL: "https://cdn/720p.m3u8"},
				},
				ThumbnailURLs: []string{"https://cdn/thumb.jpg"},
			},
		}
		mediaRepo.On("GetMedia", ctx, "m1").Return(&models.Media{MediaID: "m1", Status: "ready"}, nil)

		r, err := svc.GetMediaRenditions(ctx, "m1")
		require.NoError(t, err)
		assert.Equal(t, "https://cdn/master.m3u8", r.HLSMasterURL)
		require.Len(t, r.Variants, 1)
		assert.Equal(t, "720p", r.Variants[0].Quality)
		assert.Equal(t, "https://cdn/thumb.jpg", r.ThumbnailURLs[0])
	})
}

func TestService_GenerateSignedStreamURL_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	publisher := streaming.NewMockPublisher()

	t.Run("cloudfront_missing", func(t *testing.T) {
		svc := NewService(&MockMediaRepository{}, nil, publisher, &MockJobQueueService{}, logger, "bucket", "cdn.example")
		_, err := svc.GenerateSignedStreamURL(ctx, "m1", nil)
		require.ErrorIs(t, err, ErrCloudFrontServiceUnavailable)
	})

	t.Run("media_not_ready", func(t *testing.T) {
		mediaRepo := new(MockMediaRepository)
		svc := NewService(mediaRepo, nil, publisher, &MockJobQueueService{}, logger, "bucket", "cdn.example")
		svc.cloudfrontService = &fakeCloudFrontService{signedURL: "https://signed"}
		mediaRepo.On("GetMedia", ctx, "m1").Return(&models.Media{MediaID: "m1", Status: "processing"}, nil)

		_, err := svc.GenerateSignedStreamURL(ctx, "m1", nil)
		require.ErrorIs(t, err, ErrMediaNotReadyForStreaming)
	})

	t.Run("success_quality_and_bitrate", func(t *testing.T) {
		mediaRepo := new(MockMediaRepository)
		svc := NewService(mediaRepo, nil, publisher, &MockJobQueueService{}, logger, "bucket", "cdn.example")
		cf := &fakeCloudFrontService{signedURL: "https://signed"}
		svc.cloudfrontService = cf
		mediaRepo.On("GetMedia", ctx, "m1").Return(&models.Media{MediaID: "m1", Status: "ready"}, nil)

		q := "480p"
		session, err := svc.GenerateSignedStreamURL(ctx, "m1", &q)
		require.NoError(t, err)
		assert.Equal(t, "https://signed", session.URL)
		assert.Equal(t, "480p", session.Quality)
		assert.Equal(t, 1500000, session.Bitrate)
		require.Len(t, cf.calls, 1)
	})
}

func TestService_PreloadMedia_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	publisher := streaming.NewMockPublisher()

	t.Run("all_fail_returns_error", func(t *testing.T) {
		mediaRepo := new(MockMediaRepository)
		svc := NewService(mediaRepo, nil, publisher, &MockJobQueueService{}, logger, "bucket", "cdn.example")
		svc.manifestService = &fakeManifestService{preloadErr: stderrors.New("boom")}

		mediaRepo.On("GetMedia", ctx, "m1").Return((*models.Media)(nil), stderrors.New("boom"))
		mediaRepo.On("GetMedia", ctx, "m2").Return((*models.Media)(nil), stderrors.New("boom"))

		_, err := svc.PreloadMedia(ctx, []string{"m1", "m2"})
		require.Error(t, err)
	})

	t.Run("returns_ready_ids", func(t *testing.T) {
		mediaRepo := new(MockMediaRepository)
		svc := NewService(mediaRepo, nil, publisher, &MockJobQueueService{}, logger, "bucket", "cdn.example")

		mediaRepo.On("GetMedia", ctx, "m1").Return(&models.Media{MediaID: "m1", Status: "ready"}, nil)
		mediaRepo.On("GetMedia", ctx, "m2").Return(&models.Media{MediaID: "m2", Status: "processing"}, nil)

		ids, err := svc.PreloadMedia(ctx, []string{"m1", "m2"})
		require.NoError(t, err)
		assert.Equal(t, []string{"m1"}, ids)
	})
}

func TestService_UpdateMediaFromTranscodingJob_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	publisher := streaming.NewMockPublisher()

	t.Run("job_not_found", func(t *testing.T) {
		mediaRepo := new(MockMediaRepository)
		svc := NewService(mediaRepo, nil, publisher, &MockJobQueueService{}, logger, "bucket", "cdn.example")
		mediaRepo.On("GetTranscodingJob", ctx, "j1").Return((*models.TranscodingJob)(nil), stderrors.New("boom"))
		err := svc.UpdateMediaFromTranscodingJob(ctx, "j1")
		require.ErrorIs(t, err, ErrTranscodingJobNotFound)
	})

	t.Run("completed_updates_variants", func(t *testing.T) {
		mediaRepo := new(MockMediaRepository)
		svc := NewService(mediaRepo, nil, publisher, &MockJobQueueService{}, logger, "bucket", "cdn.example")

		job := &models.TranscodingJob{
			JobID:          "j1",
			MediaID:        "m1",
			Status:         "completed",
			OutputVariants: map[string]string{"720p": "application/vnd.apple.mpegurl"},
			OutputSizes:    map[string]int64{"720p": 123},
		}
		media := &models.Media{MediaID: "m1", Status: "processing"}

		mediaRepo.On("GetTranscodingJob", ctx, "j1").Return(job, nil)
		mediaRepo.On("GetMedia", ctx, "m1").Return(media, nil)
		mediaRepo.On("UpdateMedia", ctx, mock.MatchedBy(func(updated *models.Media) bool {
			return updated.Status == "ready" && len(updated.Variants) == 1
		})).Return(nil)

		require.NoError(t, svc.UpdateMediaFromTranscodingJob(ctx, "j1"))
	})

	t.Run("update_media_error_is_wrapped", func(t *testing.T) {
		mediaRepo := new(MockMediaRepository)
		svc := NewService(mediaRepo, nil, publisher, &MockJobQueueService{}, logger, "bucket", "cdn.example")

		job := &models.TranscodingJob{JobID: "j1", MediaID: "m1", Status: "failed", ErrorMessage: "nope"}
		media := &models.Media{MediaID: "m1", Status: "processing"}

		mediaRepo.On("GetTranscodingJob", ctx, "j1").Return(job, nil)
		mediaRepo.On("GetMedia", ctx, "m1").Return(media, nil)
		mediaRepo.On("UpdateMedia", ctx, mock.Anything).Return(stderrors.New("boom"))

		err := svc.UpdateMediaFromTranscodingJob(ctx, "j1")
		require.ErrorIs(t, err, ErrMediaUpdateFailed)
	})
}
