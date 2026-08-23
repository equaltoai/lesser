package media

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/services/media/transcoding"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func m2ReadyInternalMedia(id, owner, digest string) *models.Media {
	now := time.Now().UTC()
	return &models.Media{
		MediaID:     id,
		UserID:      owner,
		FileName:    id + ".png",
		ContentType: "image/png",
		FileSize:    12,
		ContentHash: digest,
		S3Bucket:    "media-private",
		S3Key:       "media/2026/08/23/" + id + ".png",
		Status:      "ready",
		Width:       120,
		Height:      80,
		Visibility:  models.MediaVisibilityInternal,
		ModelVersion: 1,
		UploadedAt:  now,
		CreatedAt:   now,
		UpdatedAt:   now,
		Provenance: &models.MediaProvenance{
			Origin:           models.EditorialMediaOriginSupplied,
			ResponsibleActor: owner,
			RecordedAt:       now,
			ContentIntegrity: digest,
		},
	}
}

func TestServicePublishMediaDurablyMintsExactBytes(t *testing.T) {
	service, mediaRepo, _, _ := createTestService(t)
	objectStore := newFakeMediaS3Service()
	service.SetS3Service(objectStore)
	service.SetEditorialKMSKeyID("alias/lesser-test")
	// NewService in the harness passes an empty CDN domain; set the field directly
	// like the registry does with the configured CloudFront domain.
	service.cdnDomain = "cdn.example.test"
	ctx := context.Background()

	digest := "sha256:" + strings.Repeat("a", 64)
	media := m2ReadyInternalMedia("m1", "alice", digest)
	objectStore.objects["media-private/media/2026/08/23/m1.png"] = []byte("exact approved bytes")

	mediaRepo.On("GetMedia", mock.Anything, "m1").Return(media, nil).Once()
	var storedKey, storedURL string
	var storedAt time.Time
	mediaRepo.On("UpdateMediaPublishedState", mock.Anything, "m1",
		mock.MatchedBy(func(key string) bool { storedKey = key; return key == "published/media/2026/08/23/m1.png" }),
		mock.MatchedBy(func(url string) bool {
			storedURL = url
			return strings.HasPrefix(url, "https://cdn.example.test/published/")
		}),
		mock.MatchedBy(func(at time.Time) bool { storedAt = at; return !at.IsZero() }),
		mock.MatchedBy(func(version int) bool { return version == 1 }),
	).Return(nil).Once()

	published, err := service.PublishMediaDurably(ctx, "m1")
	require.NoError(t, err)
	require.NotNil(t, published)
	require.Equal(t, "m1", published.MediaID)
	require.Equal(t, digest, published.ContentHash)
	require.Equal(t, "https://cdn.example.test/published/media/2026/08/23/m1.png", published.URL)
	require.Equal(t, "published/media/2026/08/23/m1.png", published.S3Key)
	require.Equal(t, "image/png", published.ContentType)
	require.Equal(t, storedURL, published.URL)
	require.Equal(t, storedKey, published.S3Key)
	require.False(t, storedAt.IsZero())
	require.Equal(t, []byte("exact approved bytes"), objectStore.objects["media-private/published/media/2026/08/23/m1.png"],
		"the durable copy must carry the exact approved original bytes")
}

func TestServicePublishMediaDurablyFailsClosed(t *testing.T) {
	digest := "sha256:" + strings.Repeat("b", 64)
	now := time.Now().UTC()
	internalReady := m2ReadyInternalMedia("m1", "alice", digest)

	t.Run("non-internal asset", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		service.SetS3Service(newFakeMediaS3Service())
		service.cdnDomain = "cdn.example.test"
		public := *internalReady
		public.Visibility = models.MediaVisibilityPublic
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(&public, nil).Once()
		_, err := service.PublishMediaDurably(context.Background(), "m1")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrMediaUnauthorizedAccess)
	})

	t.Run("not ready", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		service.SetS3Service(newFakeMediaS3Service())
		service.cdnDomain = "cdn.example.test"
		pending := *internalReady
		pending.Status = models.StatusPending
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(&pending, nil).Once()
		_, err := service.PublishMediaDurably(context.Background(), "m1")
		require.ErrorIs(t, err, ErrMediaNotReady)
	})

	t.Run("withdrawn lifecycle", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		service.SetS3Service(newFakeMediaS3Service())
		service.cdnDomain = "cdn.example.test"
		withdrawn := *internalReady
		withdrawn.EditorialState = models.EditorialLifecycleWithdrawn
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(&withdrawn, nil).Once()
		_, err := service.PublishMediaDurably(context.Background(), "m1")
		require.ErrorContains(t, err, "lifecycle does not allow publication")
	})

	t.Run("integrity mismatch", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		service.SetS3Service(newFakeMediaS3Service())
		service.cdnDomain = "cdn.example.test"
		broken := *internalReady
		broken.Provenance = &models.MediaProvenance{Origin: models.EditorialMediaOriginSupplied, ResponsibleActor: "alice", RecordedAt: now, ContentIntegrity: "sha256:" + strings.Repeat("c", 64)}
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(&broken, nil).Once()
		_, err := service.PublishMediaDurably(context.Background(), "m1")
		require.ErrorContains(t, err, "integrity is unavailable")
	})

	t.Run("no copier", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		service.cdnDomain = "cdn.example.test"
		service.SetS3Service(nil)
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(internalReady, nil).Once()
		_, err := service.PublishMediaDurably(context.Background(), "m1")
		require.ErrorContains(t, err, "durable published copy capability is unavailable")
	})

	t.Run("no CDN domain", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		service.SetS3Service(newFakeMediaS3Service())
		service.cdnDomain = ""
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(internalReady, nil).Once()
		_, err := service.PublishMediaDurably(context.Background(), "m1")
		require.ErrorContains(t, err, "CDN domain is required")
	})
}

func TestServiceUpdateEditorialLifecycleEnforcesOwnerAndState(t *testing.T) {
	digest := "sha256:" + strings.Repeat("d", 64)
	internalReady := m2ReadyInternalMedia("m1", "alice", digest)

	t.Run("owner enforced", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(internalReady, nil).Once()
		_, err := service.UpdateEditorialLifecycle(context.Background(), &UpdateEditorialLifecycleCommand{
			MediaID: "m1", UserID: "mallory", Lifecycle: models.EditorialLifecycleWithdrawn,
		})
		require.ErrorIs(t, err, ErrMediaUnauthorizedAccess)
	})

	t.Run("withdrawn persists and is inspectable", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		updated := *internalReady
		updated.EditorialState = models.EditorialLifecycleWithdrawn
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(internalReady, nil).Once()
		mediaRepo.On("UpdateMediaEditorialState", mock.Anything, "m1", models.EditorialLifecycleWithdrawn, "", 1).Return(nil).Once()
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(&updated, nil).Once()
		result, err := service.UpdateEditorialLifecycle(context.Background(), &UpdateEditorialLifecycleCommand{
			MediaID: "m1", UserID: "alice", Lifecycle: models.EditorialLifecycleWithdrawn,
		})
		require.NoError(t, err)
		require.Equal(t, models.EditorialLifecycleWithdrawn, result.EditorialState)
	})

	t.Run("invalid lifecycle rejected", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(internalReady, nil).Once()
		_, err := service.UpdateEditorialLifecycle(context.Background(), &UpdateEditorialLifecycleCommand{
			MediaID: "m1", UserID: "alice", Lifecycle: models.EditorialLifecycle("hidden"),
		})
		require.ErrorContains(t, err, "invalid editorial lifecycle")
	})

	t.Run("non-internal rejected", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		public := *internalReady
		public.Visibility = models.MediaVisibilityPublic
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(&public, nil).Once()
		_, err := service.UpdateEditorialLifecycle(context.Background(), &UpdateEditorialLifecycleCommand{
			MediaID: "m1", UserID: "alice", Lifecycle: models.EditorialLifecycleWithdrawn,
		})
		require.ErrorContains(t, err, "internal editorial media")
	})

	t.Run("superseded requires successor", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(internalReady, nil).Once()
		_, err := service.UpdateEditorialLifecycle(context.Background(), &UpdateEditorialLifecycleCommand{
			MediaID: "m1", UserID: "alice", Lifecycle: models.EditorialLifecycleSuperseded,
		})
		require.ErrorContains(t, err, "must name the superseding asset")
	})
}

// createTestService is defined in the package's existing tests; this guard
// documents that the mocks below stay type-compatible with the media service.
var _ = errors.New
var _ = transcoding.TranscodeRequest{}
var _ = zap.NewNop
