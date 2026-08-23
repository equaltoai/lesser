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
		MediaID:      id,
		UserID:       owner,
		FileName:     id + ".png",
		ContentType:  "image/png",
		FileSize:     12,
		ContentHash:  digest,
		S3Bucket:     "media-private",
		S3Key:        "media/2026/08/23/" + id + ".png",
		Status:       "ready",
		Width:        120,
		Height:       80,
		Visibility:   models.MediaVisibilityInternal,
		ModelVersion: 1,
		UploadedAt:   now,
		CreatedAt:    now,
		UpdatedAt:    now,
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

func TestServicePublishMediaDurablyCompensatesRecordWriteFailure(t *testing.T) {
	digest := "sha256:" + strings.Repeat("e", 64)
	internalReady := m2ReadyInternalMedia("m1", "alice", digest)

	t.Run("record write failure deletes the orphaned published object", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		objectStore := newFakeMediaS3Service()
		service.SetS3Service(objectStore)
		service.cdnDomain = "cdn.example.test"
		objectStore.objects["media-private/media/2026/08/23/m1.png"] = []byte("exact bytes")
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(internalReady, nil).Once()
		mediaRepo.On("UpdateMediaPublishedState", mock.Anything, "m1",
			"published/media/2026/08/23/m1.png",
			mock.MatchedBy(func(url string) bool { return strings.HasPrefix(url, "https://cdn.example.test/published/") }),
			mock.MatchedBy(func(at time.Time) bool { return !at.IsZero() }),
			1,
		).Return(errors.New("record write failed")).Once()

		_, err := service.PublishMediaDurably(context.Background(), "m1")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrMediaUpdateFailed)
		require.Len(t, objectStore.deleteCalls, 1, "the orphaned published object must be deleted exactly once")
		require.Equal(t, "media-private", objectStore.deleteCalls[0].bucket)
		require.Equal(t, "published/media/2026/08/23/m1.png", objectStore.deleteCalls[0].key)
		_, ok := objectStore.objects["media-private/published/media/2026/08/23/m1.png"]
		require.False(t, ok, "no world-readable orphan may survive a failed publish")
	})

	t.Run("cleanup failure does not mask the record error", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		objectStore := newFakeMediaS3Service()
		objectStore.deleteErr = errors.New("cleanup failed")
		service.SetS3Service(objectStore)
		service.cdnDomain = "cdn.example.test"
		objectStore.objects["media-private/media/2026/08/23/m1.png"] = []byte("exact bytes")
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(internalReady, nil).Once()
		mediaRepo.On("UpdateMediaPublishedState", mock.Anything, "m1",
			"published/media/2026/08/23/m1.png",
			mock.MatchedBy(func(url string) bool { return strings.HasPrefix(url, "https://cdn.example.test/published/") }),
			mock.MatchedBy(func(at time.Time) bool { return !at.IsZero() }),
			1,
		).Return(errors.New("record write failed")).Once()

		_, err := service.PublishMediaDurably(context.Background(), "m1")
		require.ErrorIs(t, err, ErrMediaUpdateFailed)
		require.NotContains(t, err.Error(), "cleanup failed", "cleanup failure must not be surfaced")
		require.Len(t, objectStore.deleteCalls, 1)
	})
}

func TestServiceUnpublishMediaDurablyClearsRecordAndDeletesObject(t *testing.T) {
	digest := "sha256:" + strings.Repeat("f", 64)
	now := time.Now().UTC()
	publishedAt := now.Add(-time.Minute)
	published := m2ReadyInternalMedia("m1", "alice", digest)
	published.ModelVersion = 2
	published.PublishedS3Key = "published/media/2026/08/23/m1.png"
	published.PublishedURL = "https://cdn.example.test/published/media/2026/08/23/m1.png"
	published.PublishedAt = &publishedAt

	t.Run("clears record then deletes object", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		objectStore := newFakeMediaS3Service()
		service.SetS3Service(objectStore)
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(published, nil).Once()
		mediaRepo.On("ClearMediaPublishedState", mock.Anything, "m1", 2).Return(nil).Once()
		require.NoError(t, service.UnpublishMediaDurably(context.Background(), "m1"))
		require.Len(t, objectStore.deleteCalls, 1)
		require.Equal(t, "media-private", objectStore.deleteCalls[0].bucket)
		require.Equal(t, "published/media/2026/08/23/m1.png", objectStore.deleteCalls[0].key)
	})

	t.Run("stale version skips the object delete", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		objectStore := newFakeMediaS3Service()
		service.SetS3Service(objectStore)
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(published, nil).Once()
		mediaRepo.On("ClearMediaPublishedState", mock.Anything, "m1", 2).Return(errors.New("conditional check failed")).Once()
		require.NoError(t, service.UnpublishMediaDurably(context.Background(), "m1"))
		require.Empty(t, objectStore.deleteCalls, "a stale rollback must not delete a concurrently re-minted serving")
	})

	t.Run("unpublished asset is a no-op", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		objectStore := newFakeMediaS3Service()
		service.SetS3Service(objectStore)
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(m2ReadyInternalMedia("m1", "alice", digest), nil).Once()
		require.NoError(t, service.UnpublishMediaDurably(context.Background(), "m1"))
		require.Empty(t, objectStore.deleteCalls)
	})

	t.Run("missing media is a no-op", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		objectStore := newFakeMediaS3Service()
		service.SetS3Service(objectStore)
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(nil, errors.New("not found")).Once()
		require.NoError(t, service.UnpublishMediaDurably(context.Background(), "m1"))
		require.Empty(t, objectStore.deleteCalls)
	})
}

func TestServiceUpdateEditorialLifecycleValidatesSupersedingAsset(t *testing.T) {
	digest := "sha256:" + strings.Repeat("g", 64)
	internalReady := m2ReadyInternalMedia("m1", "alice", digest)
	successorReady := m2ReadyInternalMedia("m2", "alice", digest)
	successorReady.ModelVersion = 1

	t.Run("missing successor rejected", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(internalReady, nil).Once()
		mediaRepo.On("GetMedia", mock.Anything, "m2").Return(nil, errors.New("not found")).Once()
		_, err := service.UpdateEditorialLifecycle(context.Background(), &UpdateEditorialLifecycleCommand{
			MediaID: "m1", UserID: "alice", Lifecycle: models.EditorialLifecycleSuperseded, SupersededByMediaID: "m2",
		})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrMediaRetrievalFailed)
	})

	t.Run("wrong-owner successor rejected", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		foreign := *successorReady
		foreign.UserID = "mallory"
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(internalReady, nil).Once()
		mediaRepo.On("GetMedia", mock.Anything, "m2").Return(&foreign, nil).Once()
		_, err := service.UpdateEditorialLifecycle(context.Background(), &UpdateEditorialLifecycleCommand{
			MediaID: "m1", UserID: "alice", Lifecycle: models.EditorialLifecycleSuperseded, SupersededByMediaID: "m2",
		})
		require.ErrorIs(t, err, ErrMediaUnauthorizedAccess)
	})

	t.Run("non-internal successor rejected", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		public := *successorReady
		public.Visibility = models.MediaVisibilityPublic
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(internalReady, nil).Once()
		mediaRepo.On("GetMedia", mock.Anything, "m2").Return(&public, nil).Once()
		_, err := service.UpdateEditorialLifecycle(context.Background(), &UpdateEditorialLifecycleCommand{
			MediaID: "m1", UserID: "alice", Lifecycle: models.EditorialLifecycleSuperseded, SupersededByMediaID: "m2",
		})
		require.ErrorContains(t, err, "internal editorial asset")
	})

	t.Run("valid internal successor accepted", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		superseded := *internalReady
		superseded.EditorialState = models.EditorialLifecycleSuperseded
		superseded.SupersededByMediaID = "m2"
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(internalReady, nil).Once()
		mediaRepo.On("GetMedia", mock.Anything, "m2").Return(successorReady, nil).Once()
		mediaRepo.On("UpdateMediaEditorialState", mock.Anything, "m1", models.EditorialLifecycleSuperseded, "m2", 1).Return(nil).Once()
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(&superseded, nil).Once()
		result, err := service.UpdateEditorialLifecycle(context.Background(), &UpdateEditorialLifecycleCommand{
			MediaID: "m1", UserID: "alice", Lifecycle: models.EditorialLifecycleSuperseded, SupersededByMediaID: "m2",
		})
		require.NoError(t, err)
		require.Equal(t, models.EditorialLifecycleSuperseded, result.EditorialState)
		require.Equal(t, "m2", result.SupersededByMediaID)
	})
}
