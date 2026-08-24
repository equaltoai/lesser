package cms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// TestMarkDraftFailedLogsMarkerWriteFailure proves the failed-status marker
// write is best-effort and its own failure is logged (not silently ignored)
// while the original publish error still surfaces and rollback still runs.
func TestMarkDraftFailedLogsMarkerWriteFailure(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)
	ctx := context.Background()
	digestA := m2Digest("a")
	repo := &transitionFailingReviewRepo{reviewMemRepo: newReviewMemRepo(), failOnStatus: "failed", failErr: errors.New("marker write failed")}
	media := &memMediaRepo{byID: map[string]*models.Media{"hero": m2ReadyMedia("hero", "owner", digestA)}}
	minter := &recordingPublishMinter{mints: map[string]EditorialPublishedMedia{}}
	minter.seedFromMedia(media)
	svc := &DraftService{
		draftRepo:      repo,
		articleService: &memArticleServiceWithErrors{base: newMemArticleService(), createErr: errors.New("article create failed")},
		domain:         "example.test",
		scheduling:     true,
		logger:         zap.New(core),
	}
	svc.SetPrincipalUsernameProvider(func(context.Context) (string, error) { return "principal", nil })
	svc.SetEditorialMediaRepository(media)
	svc.SetEditorialPublishMinter(minter)

	draft := &models.Draft{ID: "marker-fail", AuthorID: "owner", ContentType: activitypub.ArticleType, Title: "MarkerFail", Slug: "marker-fail", Content: "draft", ContentFormat: "markdown"}
	require.NoError(t, svc.CreateDraft(ctx, draft))
	_, err := svc.SetEditorialMedia(ctx, "owner", draft.ID, []models.DraftMediaUsage{{MediaID: "hero", Role: models.EditorialMediaRoleHero}})
	require.NoError(t, err)
	require.NoError(t, repo.storeGrant(&models.DraftReviewGrant{
		OwnerID: "owner", DraftID: draft.ID, Reviewer: "reviewer", GrantedAt: time.Now().UTC(),
		ExpiresAt: ptrTime(time.Now().UTC().Add(time.Hour)),
	}))
	_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", draft.ID, DraftReviewApproved, "approved")
	require.NoError(t, err)
	// The releasing owner is not the instance principal, so the doctrine floor
	// demands a current principal approval for the exact approved bytes.
	_, err = svc.ShareDraftForReview(ctx, "owner", draft.ID, "principal")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(ctx, "principal", "owner", draft.ID, DraftReviewApproved, "operator approval")
	require.NoError(t, err)

	_, err = svc.PublishDraft(ctx, "owner", draft.ID)
	require.ErrorContains(t, err, "article create failed")
	require.Equal(t, []string{"hero"}, minter.unpublishCalls,
		"rollback still runs when the failed-status marker write itself fails")

	entries := observed.FilterMessage("failed to mark draft failed").All()
	require.Len(t, entries, 1, "a failed status-marker write must be logged")
	require.Equal(t, zap.WarnLevel, entries[0].Level)
	require.Equal(t, "marker-fail", entries[0].ContextMap()["draft_id"])
}
