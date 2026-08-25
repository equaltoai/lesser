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
	// The releasing owner is not the instance principal, so the doctrine floor
	// demands a current principal approval for the exact approved bytes.
	_, err = svc.ShareDraftForReview(ctx, "owner", draft.ID, "principal")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(ctx, "principal", "owner", draft.ID, DraftReviewApproved, "operator approval")
	require.NoError(t, err)
	state, err := svc.DraftReviewState(ctx, "owner", draft.ID, nil)
	require.NoError(t, err)
	require.True(t, state.PublishEligible)
}

func TestScheduleDraftBlocksUnreadyBoundMediaWithExplicitReason(t *testing.T) {
	svc, repo, media, minter := m2ReviewService(t)
	ctx := context.Background()
	digestA := m2Digest("a")
	media.byID["hero"] = m2ReadyMedia("hero", "owner", digestA)
	minter.seedFromMedia(media)

	draft := &models.Draft{ID: "schedule-gate", AuthorID: "owner", ContentType: activitypub.ArticleType, Title: "Gate", Slug: "gate", Content: "draft", ContentFormat: "markdown"}
	require.NoError(t, svc.CreateDraft(ctx, draft))
	_, err := svc.SetEditorialMedia(ctx, "owner", draft.ID, []models.DraftMediaUsage{{MediaID: "hero", Role: models.EditorialMediaRoleHero}})
	require.NoError(t, err)
	require.NoError(t, repo.storeGrant(&models.DraftReviewGrant{
		OwnerID: "owner", DraftID: draft.ID, Reviewer: "reviewer", GrantedAt: time.Now().UTC(),
		ExpiresAt: ptrTime(time.Now().UTC().Add(time.Hour)),
	}))

	// Withdrawal changes EditorialState, not the content digest. An approval
	// recorded against the withdrawn state is hash-current, so without the
	// schedule-time media gate it would sail through and the scheduler would
	// later burn attempts and fail the draft with no media reason.
	withdrawn := *media.byID["hero"]
	withdrawn.EditorialState = models.EditorialLifecycleWithdrawn
	media.byID["hero"] = &withdrawn
	_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", draft.ID, DraftReviewApproved, "approved while withdrawn")
	require.NoError(t, err)
	// The scheduling owner is not the instance principal, so the doctrine floor
	// demands a current principal approval for the exact reviewed bytes.
	_, err = svc.ShareDraftForReview(ctx, "owner", draft.ID, "principal")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(ctx, "principal", "owner", draft.ID, DraftReviewApproved, "operator approval")
	require.NoError(t, err)

	err = svc.ScheduleDraft(ctx, "owner", draft.ID, time.Now().UTC().Add(time.Hour))
	require.ErrorIs(t, err, ErrDraftReviewMediaRequired)
	require.Contains(t, err.Error(), DraftReviewMediaReasonWithdrawn)

	scheduled, err := svc.GetDraft(ctx, "owner", draft.ID)
	require.NoError(t, err)
	require.Nil(t, scheduled.ScheduledAt, "a media-blocked draft must not be scheduled")
	require.NotEqual(t, DraftStatusScheduled, scheduled.Status)

	// A missing bound asset blocks at schedule time with the missing reason.
	delete(media.byID, "hero")
	_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", draft.ID, DraftReviewApproved, "approved while missing")
	require.NoError(t, err)
	err = svc.ScheduleDraft(ctx, "owner", draft.ID, time.Now().UTC().Add(time.Hour))
	require.ErrorIs(t, err, ErrDraftReviewMediaRequired)
	require.Contains(t, err.Error(), DraftReviewMediaReasonMissing)

	scheduled, err = svc.GetDraft(ctx, "owner", draft.ID)
	require.NoError(t, err)
	require.Nil(t, scheduled.ScheduledAt)

	// Restoring a servable asset and re-approving lets the draft schedule. The
	// digest is bound again, so the principal's withdrawn-stage approval went
	// stale with the restored bytes and must be re-recorded at the ready hash.
	media.byID["hero"] = m2ReadyMedia("hero", "owner", digestA)
	_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", draft.ID, DraftReviewApproved, "approved while ready")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(ctx, "principal", "owner", draft.ID, DraftReviewApproved, "operator approval")
	require.NoError(t, err)
	require.NoError(t, svc.ScheduleDraft(ctx, "owner", draft.ID, time.Now().UTC().Add(time.Hour)))
	scheduled, err = svc.GetDraft(ctx, "owner", draft.ID)
	require.NoError(t, err)
	require.Equal(t, DraftStatusScheduled, scheduled.Status)
	require.NotNil(t, scheduled.ScheduledAt)
}

func TestExpiredGrantsExcludedFromCountAndActiveSort(t *testing.T) {
	svc, repo, media, minter := m2ReviewService(t)
	ctx := context.Background()
	digestA := m2Digest("a")
	media.byID["hero"] = m2ReadyMedia("hero", "owner", digestA)
	minter.seedFromMedia(media)

	now := time.Now().UTC()
	activeExpiry := now.Add(time.Hour)
	expiredExpiry := now.Add(-time.Hour)
	reviewer := "reviewer"

	t.Run("count excludes expired grants", func(t *testing.T) {
		require.NoError(t, repo.storeGrant(&models.DraftReviewGrant{
			OwnerID: "owner", DraftID: "d1", Reviewer: reviewer, GrantedAt: now, ExpiresAt: &activeExpiry,
		}))
		require.NoError(t, repo.storeGrant(&models.DraftReviewGrant{
			OwnerID: "owner", DraftID: "d2", Reviewer: reviewer, GrantedAt: now, ExpiresAt: &expiredExpiry,
		}))

		count, err := svc.CountSharedDraftReviews(ctx, reviewer)
		require.NoError(t, err)
		require.Equal(t, 1, count, "totalCount must not include grants the list would exclude")

		listed, _, err := svc.SharedDraftReviews(ctx, reviewer, 10, "")
		require.NoError(t, err)
		require.Equal(t, count, len(listed), "count must equal the active queue edges")
	})

	t.Run("read state sorts active grants first", func(t *testing.T) {
		draft := &models.Draft{ID: "sort-grants", AuthorID: "owner", ContentType: activitypub.ArticleType, Title: "Sort", Slug: "sort", Content: "draft", ContentFormat: "markdown"}
		require.NoError(t, svc.CreateDraft(ctx, draft))
		require.NoError(t, repo.storeGrant(&models.DraftReviewGrant{
			OwnerID: "owner", DraftID: draft.ID, Reviewer: "expired-reviewer", GrantedAt: now.Add(-2 * time.Hour), ExpiresAt: &expiredExpiry,
		}))
		require.NoError(t, repo.storeGrant(&models.DraftReviewGrant{
			OwnerID: "owner", DraftID: draft.ID, Reviewer: "active-reviewer", GrantedAt: now.Add(-time.Hour), ExpiresAt: &activeExpiry,
		}))

		state, err := svc.DraftReviewState(ctx, "owner", draft.ID, nil)
		require.NoError(t, err)
		require.Len(t, state.Grants, 2)
		require.Equal(t, "active-reviewer", state.Grants[0].Reviewer,
			"expired grants must sort after active grants in the read state")
		require.Equal(t, "expired-reviewer", state.Grants[1].Reviewer)
	})
}
