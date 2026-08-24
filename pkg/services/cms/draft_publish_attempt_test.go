package cms

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// F1 remediation proving suite: the stale-publishing sweep horizon keys on the
// publish-attempt timestamp (PublishAttemptedAt) that the transition alone
// writes, so ordinary content writes on a publishing draft (update, autosave,
// editorial-media set) advance UpdatedAt without re-arming the horizon, and a
// fresh transition does re-arm it.

func newPublishAttemptTestService(t *testing.T) (*DraftService, *memDraftRepo) {
	t.Helper()
	repo := newMemDraftRepo()
	svc := NewDraftService(repo, nil, "example.test", false, zap.NewNop())
	return svc, repo
}

// seedPublishingDraft stores a publishing draft whose publish attempt is old
// (stale) but whose UpdatedAt is fresh, mirroring an author who kept editing a
// crash-stuck publishing draft.
func seedPublishingDraft(t *testing.T, repo *memDraftRepo, attemptAt, updatedAt time.Time) *models.Draft {
	t.Helper()
	stamp := attemptAt
	draft := &models.Draft{
		ID: "draft-1", AuthorID: "alice", ContentType: "Note", Content: "body",
		Status: DraftStatusPublishing, UpdatedAt: updatedAt, PublishAttemptedAt: &stamp,
	}
	require.NoError(t, repo.CreateDraft(context.Background(), draft))
	return draft
}

func TestUpdateDraftDoesNotRearmPublishAttemptHorizon(t *testing.T) {
	svc, repo := newPublishAttemptTestService(t)
	attemptAt := time.Now().UTC().Add(-25 * time.Hour)
	seedPublishingDraft(t, repo, attemptAt, attemptAt)

	loaded, err := repo.GetDraft(context.Background(), "alice", "draft-1")
	require.NoError(t, err)
	loaded.Content = "edited while publishing"
	require.NoError(t, svc.UpdateDraft(context.Background(), "alice", loaded))

	stored, err := repo.GetDraft(context.Background(), "alice", "draft-1")
	require.NoError(t, err)
	require.Equal(t, "edited while publishing", stored.Content)
	require.Equal(t, attemptAt, *stored.PublishAttemptedAt,
		"a content update must not advance the publish-attempt stamp (the sweep horizon)")
	require.False(t, stored.UpdatedAt.IsZero())
	require.True(t, stored.UpdatedAt.After(attemptAt),
		"the content update still advances UpdatedAt; the horizon must not follow it")
}

func TestAutosaveDoesNotRearmPublishAttemptHorizon(t *testing.T) {
	svc, repo := newPublishAttemptTestService(t)
	attemptAt := time.Now().UTC().Add(-25 * time.Hour)
	seedPublishingDraft(t, repo, attemptAt, attemptAt)

	loaded, err := repo.GetDraft(context.Background(), "alice", "draft-1")
	require.NoError(t, err)
	loaded.Content = "autosaved while publishing"
	require.NoError(t, svc.Autosave(context.Background(), "alice", loaded))

	stored, err := repo.GetDraft(context.Background(), "alice", "draft-1")
	require.NoError(t, err)
	require.Equal(t, "autosaved while publishing", stored.Content)
	require.Equal(t, attemptAt, *stored.PublishAttemptedAt,
		"an autosave must not advance the publish-attempt stamp (the sweep horizon)")
	require.True(t, stored.UpdatedAt.After(attemptAt))
}

func TestSetEditorialMediaDoesNotRearmPublishAttemptHorizon(t *testing.T) {
	svc, repo := newPublishAttemptTestService(t)
	svc.SetEditorialMediaRepository(&editorialMediaMemRepo{items: map[string]*models.Media{
		"hero": internalEditorialMedia("hero", "alice"),
	}})
	attemptAt := time.Now().UTC().Add(-25 * time.Hour)
	seedPublishingDraft(t, repo, attemptAt, attemptAt)

	draft, err := svc.SetEditorialMedia(context.Background(), "alice", "draft-1",
		[]models.DraftMediaUsage{{MediaID: "hero", Role: models.EditorialMediaRoleHero}})
	require.NoError(t, err)
	require.Len(t, draft.EditorialMedia, 1)

	stored, err := repo.GetDraft(context.Background(), "alice", "draft-1")
	require.NoError(t, err)
	require.Equal(t, attemptAt, *stored.PublishAttemptedAt,
		"an editorial-media set must not advance the publish-attempt stamp (the sweep horizon)")
	require.True(t, stored.UpdatedAt.After(attemptAt))
}

func TestTransitionDraftToPublishingStampsPublishAttempt(t *testing.T) {
	svc, repo := newPublishAttemptTestService(t)
	require.NoError(t, repo.CreateDraft(context.Background(), &models.Draft{
		ID: "draft-1", AuthorID: "alice", ContentType: "Note", Content: "body",
		Status: DraftStatusScheduled, UpdatedAt: time.Now().UTC().Add(-2 * time.Hour),
	}))

	loaded, err := repo.GetDraft(context.Background(), "alice", "draft-1")
	require.NoError(t, err)

	firstAttempt := time.Now().UTC()
	require.NoError(t, svc.transitionDraftToPublishing(context.Background(), "alice", loaded, firstAttempt))

	stored, err := repo.GetDraft(context.Background(), "alice", "draft-1")
	require.NoError(t, err)
	require.Equal(t, DraftStatusPublishing, stored.Status)
	require.Nil(t, stored.ScheduledAt)
	require.Equal(t, firstAttempt, *stored.PublishAttemptedAt,
		"the transition stamps the publish attempt; the sweep horizon starts here")
	require.Equal(t, firstAttempt, stored.UpdatedAt)

	// A fresh transition (a retry after the first attempt crashed) re-arms the
	// horizon by writing a new stamp.
	retryAt := firstAttempt.Add(2 * time.Minute)
	require.NoError(t, svc.transitionDraftToPublishing(context.Background(), "alice", stored, retryAt))
	stored, err = repo.GetDraft(context.Background(), "alice", "draft-1")
	require.NoError(t, err)
	require.Equal(t, retryAt, *stored.PublishAttemptedAt,
		"a fresh transition advances the publish-attempt stamp (the horizon re-arms)")
}
