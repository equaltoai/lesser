package cms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
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
		require.Equal(t, draftStatusFailed, got.Status)
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
		require.Equal(t, draftStatusFailed, got.Status)
		require.Equal(t, draftPublishFailureMedia, got.PublishFailureReason)
	})
}
