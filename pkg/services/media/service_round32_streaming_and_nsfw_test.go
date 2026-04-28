package media

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/media/transcoding"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeAccountPrefsRepoRound32 struct {
	prefs map[string]map[string]interface{}
	err   error
}

func (f *fakeAccountPrefsRepoRound32) GetAccountPreferences(_ context.Context, username string) (map[string]interface{}, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.prefs == nil {
		return map[string]interface{}{}, nil
	}
	return f.prefs[username], nil
}

type fakeTranscoderRound32 struct{}

func (f *fakeTranscoderRound32) SubmitJob(context.Context, *transcoding.TranscodeRequest) (*transcoding.TranscodeResult, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeTranscoderRound32) ConvertToTranscodingJob(*transcoding.TranscodeRequest, *transcoding.TranscodeResult) *models.TranscodingJob {
	return &models.TranscodingJob{}
}

type fakeManifestRound32 struct{}

func (f *fakeManifestRound32) GetManifestInfo(context.Context, string, string) (*transcoding.ManifestInfo, error) {
	return nil, nil
}

func (f *fakeManifestRound32) PreloadManifests(context.Context, []string) error { return nil }

type fakeCloudFrontRound32 struct{}

func (f *fakeCloudFrontRound32) SignStreamingURL(string, string, *string, time.Duration) (string, error) {
	return "signed", nil
}

func TestNewService_InitializesDefaultsAndOptionalSetters(t *testing.T) {
	t.Parallel()

	svc := NewService(new(MockMediaRepository), nil, nil, nil, nil, "bucket", "cdn.example.com")
	require.NotNil(t, svc)
	require.NotNil(t, svc.logger)

	transcoder := &fakeTranscoderRound32{}
	manifest := &fakeManifestRound32{}
	cloudfront := &fakeCloudFrontRound32{}

	svc.SetTranscodingService(transcoder)
	svc.SetManifestService(manifest)
	svc.SetCloudFrontService(cloudfront)

	require.Same(t, transcoder, svc.transcoder)
	require.Same(t, manifest, svc.manifestService)
	require.Same(t, cloudfront, svc.cloudfrontService)
}

func TestService_GetStreamingURL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("not ready returns expected error", func(t *testing.T) {
		service := NewService(new(MockMediaRepository), nil, nil, nil, zap.NewNop(), "bucket", "cdn.example.com")
		mediaRepo := service.mediaRepo.(*MockMediaRepository)
		mediaRepo.On("GetMedia", ctx, "m1").Return(&models.Media{MediaID: "m1", UserID: "alice", Status: "processing"}, nil).Once()

		_, err := service.GetStreamingURL(ctx, "m1", "alice")
		require.ErrorIs(t, err, ErrMediaNotReadyForStreaming)
	})

	t.Run("viewer must own media", func(t *testing.T) {
		service := NewService(new(MockMediaRepository), nil, nil, nil, zap.NewNop(), "bucket", "cdn.example.com")
		mediaRepo := service.mediaRepo.(*MockMediaRepository)
		mediaRepo.On("GetMedia", ctx, "m1").Return(&models.Media{MediaID: "m1", UserID: "bob", Status: "ready"}, nil).Once()

		_, err := service.GetStreamingURL(ctx, "m1", "alice")
		require.ErrorIs(t, err, ErrMediaUnauthorizedAccess)
	})

	t.Run("uses CDN URLs when available", func(t *testing.T) {
		service := NewService(new(MockMediaRepository), nil, nil, nil, zap.NewNop(), "bucket", "cdn.example.com")
		mediaRepo := service.mediaRepo.(*MockMediaRepository)
		media := &models.Media{
			MediaID:     "m2",
			UserID:      "alice",
			Status:      "ready",
			ContentType: "image/jpeg",
			CDNUrl:      "https://cdn.example.com/media/m2",
			S3Bucket:    "bucket",
			S3Key:       "key/m2",
			Duration:    10,
			Variants: map[string]models.MediaVariant{
				"thumbnail": {CDNUrl: "https://cdn.example.com/media/m2/thumb", S3Key: "thumb-key"},
			},
		}
		mediaRepo.On("GetMedia", ctx, "m2").Return(media, nil).Once()

		out, err := service.GetStreamingURL(ctx, "m2", "alice")
		require.NoError(t, err)
		require.Equal(t, "https://cdn.example.com/media/m2", out.URL)
		require.Equal(t, "https://cdn.example.com/media/m2/thumb", out.ThumbnailURL)
	})

	t.Run("falls back to S3 URLs when CDN is missing", func(t *testing.T) {
		service := NewService(new(MockMediaRepository), nil, nil, nil, zap.NewNop(), "bucket", "cdn.example.com")
		mediaRepo := service.mediaRepo.(*MockMediaRepository)
		media := &models.Media{
			MediaID:     "m3",
			UserID:      "alice",
			Status:      "ready",
			ContentType: "image/jpeg",
			CDNUrl:      "",
			S3Bucket:    "bucket",
			S3Key:       "key/m3",
			Duration:    10,
			Variants: map[string]models.MediaVariant{
				"thumbnail": {CDNUrl: "", S3Key: "thumb/m3"},
			},
		}
		mediaRepo.On("GetMedia", ctx, "m3").Return(media, nil).Once()

		out, err := service.GetStreamingURL(ctx, "m3", "alice")
		require.NoError(t, err)
		require.Equal(t, "https://bucket.s3.amazonaws.com/key/m3", out.URL)
		require.Equal(t, "https://bucket.s3.amazonaws.com/thumb/m3", out.ThumbnailURL)
	})

	t.Run("includes video bitrates for variants", func(t *testing.T) {
		service := NewService(new(MockMediaRepository), nil, nil, nil, zap.NewNop(), "bucket", "cdn.example.com")
		mediaRepo := service.mediaRepo.(*MockMediaRepository)
		media := &models.Media{
			MediaID:     "m4",
			UserID:      "alice",
			Status:      "ready",
			ContentType: "video/mp4",
			S3Bucket:    "bucket",
			S3Key:       "key/m4",
			Duration:    10,
			Variants: map[string]models.MediaVariant{
				"LOW": {Width: 320, Height: 180, FileSize: 1000, ContentType: "video/mp4"},
				"HD":  {Width: 1280, Height: 720, FileSize: 5000, ContentType: "video/mp4"},
			},
		}
		mediaRepo.On("GetMedia", ctx, "m4").Return(media, nil).Once()

		out, err := service.GetStreamingURL(ctx, "m4", "alice")
		require.NoError(t, err)
		require.Len(t, out.Bitrates, 2)

		qualities := make([]model.StreamQuality, 0, len(out.Bitrates))
		for _, b := range out.Bitrates {
			qualities = append(qualities, b.Quality)
		}
		sort.Slice(qualities, func(i, j int) bool { return qualities[i] < qualities[j] })
		require.Equal(t, []model.StreamQuality{model.StreamQualityHigh, model.StreamQualityLow}, qualities)
	})
}

func TestService_checkNSFWPermissions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("unauthenticated user is blocked", func(t *testing.T) {
		service := NewService(nil, nil, nil, nil, zap.NewNop(), "bucket", "cdn.example.com")
		allow, warn, err := service.checkNSFWPermissions(ctx, "")
		require.NoError(t, err)
		require.False(t, allow)
		require.True(t, warn)
	})

	t.Run("repo errors fall back to safe defaults", func(t *testing.T) {
		service := NewService(nil, &fakeAccountPrefsRepoRound32{err: errors.New("boom")}, nil, nil, zap.NewNop(), "bucket", "cdn.example.com")
		allow, warn, err := service.checkNSFWPermissions(ctx, "alice")
		require.NoError(t, err)
		require.False(t, allow)
		require.True(t, warn)
	})

	t.Run("valid preference keys are honored", func(t *testing.T) {
		service := NewService(nil, &fakeAccountPrefsRepoRound32{
			prefs: map[string]map[string]interface{}{
				"alice": {
					"allow_nsfw":            true,
					"require_nsfw_warning":  false,
					"ignored_unrelated_key": "x",
				},
			},
		}, nil, nil, zap.NewNop(), "bucket", "cdn.example.com")
		allow, warn, err := service.checkNSFWPermissions(ctx, "alice")
		require.NoError(t, err)
		require.True(t, allow)
		require.False(t, warn)
	})
}

func TestNSFWBlockedError(t *testing.T) {
	t.Parallel()

	err := NewNSFWBlockedError("blocked")
	require.Equal(t, "blocked", err.Error())
	require.True(t, IsNSFWBlocked(err))
	require.False(t, IsNSFWBlocked(errors.New("other")))
}
