package cms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

// TestDraftReviewPublishUpdateArticleFailurePathsCompensate proves the three
// update-existing-article failure paths (GetArticle error, attribution
// mismatch, UpdateArticle error) each mark the draft failed, roll back the
// batch's mints, and leave no minted state behind — mirroring the already
// tested CreateArticle failure path.
func TestDraftReviewPublishUpdateArticleFailurePathsCompensate(t *testing.T) {
	digestA := m2Digest("a")
	objectID := common.GenerateObjectID("example.test", "articles", "existing")

	seedUpdateDraft := func(t *testing.T, svc *DraftService, repo *reviewMemRepo, draftID string) *models.Draft {
		t.Helper()
		ctx := context.Background()
		objectIDValue := objectID
		draft := &models.Draft{
			ID: draftID, AuthorID: "owner", ContentType: activitypub.ArticleType,
			Title: draftID, Slug: draftID, Content: "draft", ContentFormat: "markdown",
			ObjectID: &objectIDValue,
		}
		require.NoError(t, svc.CreateDraft(ctx, draft))
		_, err := svc.SetEditorialMedia(ctx, "owner", draft.ID, []models.DraftMediaUsage{{MediaID: "hero", Role: models.EditorialMediaRoleHero}})
		require.NoError(t, err)
		require.NoError(t, repo.storeGrant(&models.DraftReviewGrant{
			OwnerID: "owner", DraftID: draft.ID, Reviewer: "reviewer", GrantedAt: time.Now().UTC(),
			ExpiresAt: ptrTime(time.Now().UTC().Add(time.Hour)),
		}))
		_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", draft.ID, DraftReviewApproved, "approved")
		require.NoError(t, err)
		// The releasing owner is not the instance principal, so the doctrine
		// floor demands a current principal approval for the exact approved bytes.
		_, err = svc.ShareDraftForReview(ctx, "owner", draft.ID, "principal")
		require.NoError(t, err)
		_, err = svc.SubmitDraftReview(ctx, "principal", "owner", draft.ID, DraftReviewApproved, "operator approval")
		require.NoError(t, err)
		return draft
	}

	seedArticle := func(attributedTo string) *memArticleService {
		base := newMemArticleService()
		base.items[objectID] = &models.Article{
			Object: models.Object{
				ID: objectID, Type: activitypub.ArticleType, Name: "Existing",
				Content: "existing", AttributedTo: attributedTo,
				Published: time.Now().UTC(), Updated: time.Now().UTC(), CreatedAt: time.Now().UTC(),
			},
			Slug: "existing",
		}
		base.slugIndex["existing"] = objectID
		return base
	}

	assertCompensated := func(t *testing.T, svc *DraftService, minter *recordingPublishMinter, draftID string) {
		t.Helper()
		got, getErr := svc.GetDraft(context.Background(), "owner", draftID)
		require.NoError(t, getErr)
		require.Equal(t, DraftStatusFailed, got.Status, "the update-path failure must mark the draft failed")
		require.NotEmpty(t, got.PublishFailureReason, "the update-path failure must record a classified reason")
		require.Equal(t, []string{"hero"}, minter.unpublishCalls,
			"the update-path failure must roll back the batch's mints so no minted state remains")
	}

	t.Run("GetArticle error rolls back and marks failed", func(t *testing.T) {
		svc, repo, media, minter := m2ReviewService(t)
		ctx := context.Background()
		media.byID["hero"] = m2ReadyMedia("hero", "owner", digestA)
		minter.seedFromMedia(media)
		svc.articleService = &memArticleServiceWithErrors{base: seedArticle(common.GenerateActorID("example.test", "owner")), getErr: errors.New("get article failed")}
		seedUpdateDraft(t, svc, repo, "update-get-fail")

		_, err := svc.PublishDraft(ctx, "owner", "update-get-fail")
		require.ErrorContains(t, err, "get article failed")
		assertCompensated(t, svc, minter, "update-get-fail")
	})

	t.Run("attribution mismatch rolls back and marks failed", func(t *testing.T) {
		svc, repo, media, minter := m2ReviewService(t)
		ctx := context.Background()
		media.byID["hero"] = m2ReadyMedia("hero", "owner", digestA)
		minter.seedFromMedia(media)
		svc.articleService = &memArticleServiceWithErrors{base: seedArticle(common.GenerateActorID("example.test", "mallory"))}
		seedUpdateDraft(t, svc, repo, "update-attribution-fail")

		_, err := svc.PublishDraft(ctx, "owner", "update-attribution-fail")
		require.ErrorContains(t, err, "does not have permission to update this article")
		assertCompensated(t, svc, minter, "update-attribution-fail")
	})

	t.Run("UpdateArticle error rolls back and marks failed", func(t *testing.T) {
		svc, repo, media, minter := m2ReviewService(t)
		ctx := context.Background()
		media.byID["hero"] = m2ReadyMedia("hero", "owner", digestA)
		minter.seedFromMedia(media)
		svc.articleService = &memArticleServiceWithErrors{base: seedArticle(common.GenerateActorID("example.test", "owner")), updateErr: errors.New("update article failed")}
		seedUpdateDraft(t, svc, repo, "update-write-fail")

		_, err := svc.PublishDraft(ctx, "owner", "update-write-fail")
		require.ErrorContains(t, err, "update article failed")
		assertCompensated(t, svc, minter, "update-write-fail")
	})
}
