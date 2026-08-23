package cms

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestDraftReviewGrantExpiryFailsClosed(t *testing.T) {
	svc, repo, media, minter := m2ReviewService(t)
	ctx := context.Background()
	digestA := m2Digest("a")
	media.byID["hero"] = m2ReadyMedia("hero", "owner", digestA)
	minter.seedFromMedia(media)

	draft := &models.Draft{ID: "grant-expiry", AuthorID: "owner", ContentType: activitypub.ArticleType, Title: "Expiry", Slug: "expiry", Content: "draft", ContentFormat: "markdown"}
	require.NoError(t, svc.CreateDraft(ctx, draft))
	_, err := svc.SetEditorialMedia(ctx, "owner", draft.ID, []models.DraftMediaUsage{{MediaID: "hero", Role: models.EditorialMediaRoleHero}})
	require.NoError(t, err)

	// ShareDraftForReview mints a bounded grant.
	grant, err := svc.ShareDraftForReview(ctx, "owner", draft.ID, "reviewer")
	require.NoError(t, err)
	require.NotNil(t, grant.ExpiresAt)
	require.True(t, grant.ExpiresAt.After(time.Now().UTC()))

	// Expire it by hand; the grant must authorize nothing.
	past := time.Now().UTC().Add(-time.Minute)
	stored := repo.grants[reviewKey("owner", draft.ID, "reviewer")]
	stored.ExpiresAt = &past
	_, err = svc.ActiveDraftReviewGrant(ctx, "owner", draft.ID, "reviewer")
	require.Error(t, err)

	shared, _, err := svc.SharedDraftReviews(ctx, "reviewer", 10, "")
	require.NoError(t, err)
	require.Empty(t, shared, "expired grants must not appear actionable")

	_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", draft.ID, DraftReviewApproved, "late")
	require.Error(t, err)

	// Re-share refreshes the expiry and restores the review lane.
	refreshed, err := svc.ShareDraftForReview(ctx, "owner", draft.ID, "reviewer")
	require.NoError(t, err)
	require.NotNil(t, refreshed.ExpiresAt)
	_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", draft.ID, DraftReviewApproved, "approved after refresh")
	require.NoError(t, err)
	state, err := svc.DraftReviewState(ctx, "owner", draft.ID, nil)
	require.NoError(t, err)
	require.True(t, state.PublishEligible)
}
