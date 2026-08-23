package media

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type staticOrphanSource struct {
	ids   []string
	err   error
	calls int
}

func (s *staticOrphanSource) ListOrphanedPublishedMintIDs(context.Context) ([]string, error) {
	s.calls++
	return s.ids, s.err
}

func TestServiceReconcileOrphanedPublishedMedia(t *testing.T) {
	digest := "sha256:" + strings.Repeat("l", 64)
	now := time.Now().UTC()
	publishedAt := now.Add(-time.Minute)
	published := m2ReadyInternalMedia("m1", "alice", digest)
	published.ModelVersion = 2
	published.PublishedS3Key = "published/media/2026/08/23/m1.png"
	published.PublishedURL = "https://cdn.example.test/published/media/2026/08/23/m1.png"
	published.PublishedAt = &publishedAt
	cleared := *published
	cleared.PublishedS3Key = ""
	cleared.PublishedURL = ""
	cleared.PublishedAt = nil

	t.Run("unwired source is a no-op", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		require.NoError(t, service.ReconcileOrphanedPublishedMedia(context.Background()))
		mediaRepo.AssertNotCalled(t, "GetMedia")
	})

	t.Run("source error is surfaced", func(t *testing.T) {
		service, _, _, _ := createTestService(t)
		service.SetOrphanPublishedMintSource(&staticOrphanSource{err: errors.New("enumeration failed")})
		err := service.ReconcileOrphanedPublishedMedia(context.Background())
		require.Error(t, err)
		require.ErrorIs(t, err, ErrMediaRetrievalFailed)
	})

	t.Run("orphaned mint is cleared and deleted", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		objectStore := newFakeMediaS3Service()
		service.SetS3Service(objectStore)
		source := &staticOrphanSource{ids: []string{"m1"}}
		service.SetOrphanPublishedMintSource(source)
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(published, nil).Once()
		mediaRepo.On("ClearMediaPublishedState", mock.Anything, "m1", 2).Return(nil).Once()
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(&cleared, nil).Once()
		require.NoError(t, service.ReconcileOrphanedPublishedMedia(context.Background()))
		require.Equal(t, 1, source.calls)
		require.Len(t, objectStore.deleteCalls, 1)
		require.Equal(t, "media-private", objectStore.deleteCalls[0].bucket)
		require.Equal(t, "published/media/2026/08/23/m1.png", objectStore.deleteCalls[0].key)
	})

	t.Run("repeated reconcile is idempotent once the mint is cleared", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		objectStore := newFakeMediaS3Service()
		service.SetS3Service(objectStore)
		source := &staticOrphanSource{ids: []string{"m1"}}
		service.SetOrphanPublishedMintSource(source)
		// First reconcile clears and deletes.
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(published, nil).Once()
		mediaRepo.On("ClearMediaPublishedState", mock.Anything, "m1", 2).Return(nil).Once()
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(&cleared, nil).Once()
		require.NoError(t, service.ReconcileOrphanedPublishedMedia(context.Background()))
		require.Len(t, objectStore.deleteCalls, 1)
		// Second reconcile sees the already-cleared record: no clear, no delete.
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(&cleared, nil).Once()
		require.NoError(t, service.ReconcileOrphanedPublishedMedia(context.Background()))
		require.Len(t, objectStore.deleteCalls, 1, "a repeated reconcile must not re-delete a cleared serving")
		mediaRepo.AssertNumberOfCalls(t, "ClearMediaPublishedState", 1)
	})

	t.Run("an already-unpublished candidate is left untouched", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		objectStore := newFakeMediaS3Service()
		service.SetS3Service(objectStore)
		source := &staticOrphanSource{ids: []string{"m1"}}
		service.SetOrphanPublishedMintSource(source)
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(&cleared, nil).Once()
		require.NoError(t, service.ReconcileOrphanedPublishedMedia(context.Background()))
		require.Empty(t, objectStore.deleteCalls)
		mediaRepo.AssertNotCalled(t, "ClearMediaPublishedState")
	})
}
