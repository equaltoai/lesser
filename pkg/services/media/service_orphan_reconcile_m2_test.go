package media

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type staticOrphanSource struct {
	ids          []string
	err          error
	recheckErr   error
	recheckFalse []string // media IDs whose re-check reports the premise changed
	recheckCalls []string
	calls        int
}

func (s *staticOrphanSource) ListOrphanedPublishedMintIDs(context.Context) ([]string, error) {
	s.calls++
	return s.ids, s.err
}

func (s *staticOrphanSource) RecheckOrphanedPublishedMint(_ context.Context, mediaID string) (bool, error) {
	s.recheckCalls = append(s.recheckCalls, mediaID)
	if s.recheckErr != nil {
		return false, s.recheckErr
	}
	for _, id := range s.recheckFalse {
		if id == mediaID {
			return false, nil
		}
	}
	return true, nil
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

	t.Run("candidate whose premise changed is re-checked and skipped", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		objectStore := newFakeMediaS3Service()
		service.SetS3Service(objectStore)
		source := &staticOrphanSource{ids: []string{"m1"}, recheckFalse: []string{"m1"}}
		service.SetOrphanPublishedMintSource(source)
		// The premise changed (the owning draft was republished or an article
		// appeared) after enumeration: the unpublish must be aborted with no
		// media writes and no object delete.
		require.NoError(t, service.ReconcileOrphanedPublishedMedia(context.Background()))
		require.Equal(t, []string{"m1"}, source.recheckCalls, "the re-check must run before unpublishing")
		mediaRepo.AssertNotCalled(t, "GetMedia")
		mediaRepo.AssertNotCalled(t, "ClearMediaPublishedState")
		require.Empty(t, objectStore.deleteCalls, "a stale enumeration must never unpublish a just-republished asset")
	})

	t.Run("re-check failure skips the candidate fail-closed", func(t *testing.T) {
		core, observed := observer.New(zap.DebugLevel)
		service, mediaRepo := observedTestService(t, core)
		objectStore := newFakeMediaS3Service()
		service.SetS3Service(objectStore)
		source := &staticOrphanSource{ids: []string{"m1"}, recheckErr: errors.New("re-check failed")}
		service.SetOrphanPublishedMintSource(source)
		require.NoError(t, service.ReconcileOrphanedPublishedMedia(context.Background()))
		mediaRepo.AssertNotCalled(t, "ClearMediaPublishedState")
		require.Empty(t, objectStore.deleteCalls, "an unverifiable candidate must not be unpublished")
		require.NotEmpty(t, observed.FilterMessage("orphan reconciliation re-check failed; skipping candidate").All())
	})

	t.Run("every candidate is re-checked before unpublish", func(t *testing.T) {
		service, mediaRepo, _, _ := createTestService(t)
		objectStore := newFakeMediaS3Service()
		service.SetS3Service(objectStore)
		source := &staticOrphanSource{ids: []string{"m1", "m2"}}
		service.SetOrphanPublishedMintSource(source)
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(published, nil).Once()
		mediaRepo.On("ClearMediaPublishedState", mock.Anything, "m1", 2).Return(nil).Once()
		mediaRepo.On("GetMedia", mock.Anything, "m1").Return(&cleared, nil).Once()
		mediaRepo.On("GetMedia", mock.Anything, "m2").Return(published, nil).Once()
		mediaRepo.On("ClearMediaPublishedState", mock.Anything, "m2", 2).Return(nil).Once()
		mediaRepo.On("GetMedia", mock.Anything, "m2").Return(&cleared, nil).Once()
		require.NoError(t, service.ReconcileOrphanedPublishedMedia(context.Background()))
		require.Equal(t, []string{"m1", "m2"}, source.recheckCalls)
		require.Len(t, objectStore.deleteCalls, 2)
	})
}
