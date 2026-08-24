package cms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestClassifyDraftPublishFailureReason(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: draftPublishFailureGeneric},
		{name: "approval", err: ErrDraftReviewApprovalRequired, want: draftPublishFailureApproval},
		{name: "principal approval", err: ErrDraftReviewPrincipalApprovalRequired, want: draftPublishFailureApproval},
		{name: "media gate", err: ErrDraftReviewMediaRequired, want: draftPublishFailureMedia},
		{name: "media category", err: apperrors.MediaAttachmentValidationFailed("bad asset"), want: draftPublishFailureMedia},
		{name: "storage category", err: apperrors.FailedToStore("article", errors.New("db down")), want: draftPublishFailureStorage},
		{name: "unknown", err: errors.New("boom"), want: draftPublishFailureGeneric},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, classifyDraftPublishFailureReason(tc.err))
		})
	}
}

// TestInteractivePublishFailureRecordsClassifiedReason proves that a failed
// interactive publish records a non-empty, classified PublishFailureReason:
// storage when the article write fails, media when a minted asset no longer
// serves the exact approved bytes.
func TestInteractivePublishFailureRecordsClassifiedReason(t *testing.T) {
	digestA := m2Digest("a")

	newDraft := func(t *testing.T, id string) (*DraftService, *reviewMemRepo, *memMediaRepo, *recordingPublishMinter) {
		t.Helper()
		svc, repo, media, minter := m2ReviewService(t)
		ctx := context.Background()
		media.byID["hero"] = m2ReadyMedia("hero", "owner", digestA)
		minter.seedFromMedia(media)
		draft := &models.Draft{ID: id, AuthorID: "owner", ContentType: activitypub.ArticleType, Title: id, Slug: id, Content: "draft", ContentFormat: "markdown"}
		require.NoError(t, svc.CreateDraft(ctx, draft))
		_, err := svc.SetEditorialMedia(ctx, "owner", draft.ID, []models.DraftMediaUsage{{MediaID: "hero", Role: models.EditorialMediaRoleHero}})
		require.NoError(t, err)
		require.NoError(t, repo.storeGrant(&models.DraftReviewGrant{
			OwnerID: "owner", DraftID: draft.ID, Reviewer: "reviewer", GrantedAt: time.Now().UTC(),
			ExpiresAt: ptrTime(time.Now().UTC().Add(time.Hour)),
		}))
		_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", draft.ID, DraftReviewApproved, "approved")
		require.NoError(t, err)
		return svc, repo, media, minter
	}

	t.Run("article write failure is classified as storage", func(t *testing.T) {
		svc, _, _, _ := newDraft(t, "reason-storage")
		svc.articleService = &memArticleServiceWithErrors{
			base:      newMemArticleService(),
			createErr: apperrors.FailedToStore("article", errors.New("article store failed")),
		}
		_, err := svc.PublishDraft(context.Background(), "owner", "reason-storage")
		require.Error(t, err)

		got, getErr := svc.GetDraft(context.Background(), "owner", "reason-storage")
		require.NoError(t, getErr)
		require.Equal(t, DraftStatusFailed, got.Status)
		require.Equal(t, draftPublishFailureStorage, got.PublishFailureReason)
	})

	t.Run("plain article-write error classifies as storage at the publish boundary", func(t *testing.T) {
		// Production ArticleRepository writes surface raw storage-layer errors
		// without an application category; the publish boundary must classify a
		// real DynamoDB outage as storage rather than falling through to generic.
		svc, _, _, _ := newDraft(t, "reason-storage-plain")
		svc.articleService = &memArticleServiceWithErrors{
			base:      newMemArticleService(),
			createErr: errors.New("dynamodb write timed out"),
		}
		_, err := svc.PublishDraft(context.Background(), "owner", "reason-storage-plain")
		require.Error(t, err)
		require.ErrorContains(t, err, "dynamodb write timed out", "the categorized error must preserve the original message")
		require.True(t, apperrors.HasCategory(err, apperrors.CategoryStorage),
			"the publish boundary must categorize an uncategorized article-write error as storage")

		got, getErr := svc.GetDraft(context.Background(), "owner", "reason-storage-plain")
		require.NoError(t, getErr)
		require.Equal(t, DraftStatusFailed, got.Status)
		require.Equal(t, draftPublishFailureStorage, got.PublishFailureReason)
	})

	t.Run("plain update-article error classifies as storage on the update path", func(t *testing.T) {
		svc, repo, media, minter := m2ReviewService(t)
		ctx := context.Background()
		digestA := m2Digest("a")
		media.byID["hero"] = m2ReadyMedia("hero", "owner", digestA)
		minter.seedFromMedia(media)
		objectID := common.GenerateObjectID("example.test", "articles", "existing")
		objectIDValue := objectID
		base := newMemArticleService()
		base.items[objectID] = &models.Article{
			Object: models.Object{
				ID: objectID, Type: activitypub.ArticleType, Name: "Existing",
				Content: "existing", AttributedTo: common.GenerateActorID("example.test", "owner"),
				Published: time.Now().UTC(), Updated: time.Now().UTC(), CreatedAt: time.Now().UTC(),
			},
			Slug: "existing",
		}
		base.slugIndex["existing"] = objectID
		svc.articleService = &memArticleServiceWithErrors{base: base, updateErr: errors.New("dynamodb update timed out")}

		draft := &models.Draft{ID: "reason-storage-plain-update", AuthorID: "owner", ContentType: activitypub.ArticleType, Title: "Update", Slug: "update", Content: "draft", ContentFormat: "markdown", ObjectID: &objectIDValue}
		require.NoError(t, svc.CreateDraft(ctx, draft))
		_, err := svc.SetEditorialMedia(ctx, "owner", draft.ID, []models.DraftMediaUsage{{MediaID: "hero", Role: models.EditorialMediaRoleHero}})
		require.NoError(t, err)
		require.NoError(t, repo.storeGrant(&models.DraftReviewGrant{
			OwnerID: "owner", DraftID: draft.ID, Reviewer: "reviewer", GrantedAt: time.Now().UTC(),
			ExpiresAt: ptrTime(time.Now().UTC().Add(time.Hour)),
		}))
		_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", draft.ID, DraftReviewApproved, "approved")
		require.NoError(t, err)

		_, err = svc.PublishDraft(ctx, "owner", draft.ID)
		require.Error(t, err)
		require.ErrorContains(t, err, "dynamodb update timed out", "the categorized error must preserve the original message")
		require.True(t, apperrors.HasCategory(err, apperrors.CategoryStorage),
			"the publish boundary must categorize an uncategorized article-write error as storage")

		got, getErr := svc.GetDraft(ctx, "owner", draft.ID)
		require.NoError(t, getErr)
		require.Equal(t, DraftStatusFailed, got.Status)
		require.Equal(t, draftPublishFailureStorage, got.PublishFailureReason)
	})

	t.Run("minted bytes changed after approval is classified as media", func(t *testing.T) {
		svc, _, _, minter := newDraft(t, "reason-media")
		mint := minter.mints["hero"]
		mint.ContentHash = m2Digest("different")
		minter.mints["hero"] = mint

		_, err := svc.PublishDraft(context.Background(), "owner", "reason-media")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrDraftReviewMediaRequired)

		got, getErr := svc.GetDraft(context.Background(), "owner", "reason-media")
		require.NoError(t, getErr)
		require.Equal(t, DraftStatusFailed, got.Status)
		require.Equal(t, draftPublishFailureMedia, got.PublishFailureReason)
	})
}
