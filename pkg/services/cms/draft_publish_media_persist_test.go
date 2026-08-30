package cms

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/cmsrender"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

// approveDraftForPublish secures the reviewer verdict plus the operator
// doctrine principal floor for one draft, the approval shape the interactive
// publish gate requires.
func approveDraftForPublish(ctx context.Context, t *testing.T, svc *DraftService, repo *reviewMemRepo, owner, draftID string) {
	t.Helper()
	require.NoError(t, repo.storeGrant(&models.DraftReviewGrant{
		OwnerID: owner, DraftID: draftID, Reviewer: "reviewer", GrantedAt: time.Now().UTC(),
		ExpiresAt: ptrTime(time.Now().UTC().Add(time.Hour)),
	}))
	_, err := svc.SubmitDraftReview(ctx, "reviewer", owner, draftID, DraftReviewApproved, "approved")
	require.NoError(t, err)
	_, err = svc.ShareDraftForReview(ctx, owner, draftID, "principal")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(ctx, "principal", owner, draftID, DraftReviewApproved, "operator approval")
	require.NoError(t, err)
}

// TestDraftReviewPublishPersistsInlineBindingsAcrossDraftDeletion proves the
// inline bindings survive publication: the draft is deleted after publish, so
// the article record durably carries the bindings with the exact minted
// servings, and the persisted bindings compose inline images into article HTML.
func TestDraftReviewPublishPersistsInlineBindingsAcrossDraftDeletion(t *testing.T) {
	svc, repo, media, minter := m2ReviewService(t)
	ctx := context.Background()
	digestA := m2Digest("a")
	digestB := m2Digest("b")
	digestC := m2Digest("c")
	media.byID["hero"] = m2ReadyMedia("hero", "owner", digestA)
	media.byID["inline-0"] = m2ReadyMedia("inline-0", "owner", digestB)
	media.byID["inline-2"] = m2ReadyMedia("inline-2", "owner", digestC)
	minter.seedFromMedia(media)

	pos0, pos2 := 0, 2
	draft := &models.Draft{
		ID: "persist", AuthorID: "owner", ContentType: activitypub.ArticleType, Title: "Persist", Slug: "persist",
		Content: "# T\n\nOne.\n\nTwo.\n\nThree.", ContentFormat: "markdown",
	}
	require.NoError(t, svc.CreateDraft(ctx, draft))
	_, err := svc.SetEditorialMedia(ctx, "owner", draft.ID, []models.DraftMediaUsage{
		{MediaID: "hero", Role: models.EditorialMediaRoleHero, Caption: "hero cap", AltText: "hero alt"},
		{MediaID: "inline-0", Role: models.EditorialMediaRoleInline, InlinePosition: &pos0, Caption: "one cap", CreditLine: "Alice", AltText: "one alt"},
		{MediaID: "inline-2", Role: models.EditorialMediaRoleInline, InlinePosition: &pos2, AltText: "two alt"},
	})
	require.NoError(t, err)
	approveDraftForPublish(ctx, t, svc, repo, "owner", draft.ID)

	article, err := svc.PublishDraft(ctx, "owner", draft.ID)
	require.NoError(t, err)

	// The full bound set survives on the article record in canonical order:
	// hero, inline by position, then social card.
	require.Len(t, article.EditorialMedia, 3)
	require.Equal(t, models.EditorialMediaRoleHero, article.EditorialMedia[0].Role)
	require.Equal(t, models.EditorialMediaRoleInline, article.EditorialMedia[1].Role)
	require.NotNil(t, article.EditorialMedia[1].InlinePosition)
	require.Equal(t, 0, *article.EditorialMedia[1].InlinePosition)
	require.Equal(t, "one cap", article.EditorialMedia[1].Caption)
	require.Equal(t, "Alice", article.EditorialMedia[1].CreditLine)
	require.Equal(t, models.EditorialMediaRoleInline, article.EditorialMedia[2].Role)
	require.Equal(t, 2, *article.EditorialMedia[2].InlinePosition)

	// URLs are the digest-verified minted servings; the article never references
	// internal keys.
	for _, m := range article.EditorialMedia {
		require.Equal(t, minter.mints[m.MediaID].URL, m.URL)
		require.Equal(t, minter.mints[m.MediaID].ContentHash, m.ContentHash)
		require.Equal(t, minter.mints[m.MediaID].ContentType, m.ContentType)
		require.Equal(t, minter.mints[m.MediaID].Width, m.Width)
		require.Equal(t, minter.mints[m.MediaID].Height, m.Height)
	}

	// The draft is deleted after publish; the bindings survive on the article.
	_, err = svc.GetDraft(ctx, "owner", draft.ID)
	require.Error(t, err)
	require.ErrorContains(t, err, "not found")

	// The persisted inline bindings compose images into the article HTML; the
	// hero composes only in previews and never duplicates into the published
	// body (it lives on Article.featuredImage).
	rendered, err := cmsrender.RenderArticleContentWithMedia(article.Content, article.ContentFormat, article.RenderMediaList())
	require.NoError(t, err)
	require.Contains(t, rendered.HTML, minter.mints["inline-0"].URL)
	require.Contains(t, rendered.HTML, minter.mints["inline-2"].URL)
	require.NotContains(t, rendered.HTML, minter.mints["hero"].URL)
}

// TestDraftReviewPublishUpdateExistingReplacesPersistedBindings proves the
// update-existing-article lane replaces (never merges) the persisted bindings
// and clears them when a revision carries no media.
func TestDraftReviewPublishUpdateExistingReplacesPersistedBindings(t *testing.T) {
	svc, repo, media, minter := m2ReviewService(t)
	ctx := context.Background()
	digestA := m2Digest("a")
	digestB := m2Digest("b")
	digestC := m2Digest("c")
	media.byID["hero"] = m2ReadyMedia("hero", "owner", digestA)
	media.byID["inline-a"] = m2ReadyMedia("inline-a", "owner", digestB)
	media.byID["inline-b"] = m2ReadyMedia("inline-b", "owner", digestC)
	minter.seedFromMedia(media)

	pos0 := 0
	d1 := &models.Draft{ID: "d1", AuthorID: "owner", ContentType: activitypub.ArticleType, Title: "One", Slug: "one", Content: "# One\n\nBody.", ContentFormat: "markdown"}
	require.NoError(t, svc.CreateDraft(ctx, d1))
	_, err := svc.SetEditorialMedia(ctx, "owner", d1.ID, []models.DraftMediaUsage{
		{MediaID: "hero", Role: models.EditorialMediaRoleHero, AltText: "hero alt"},
		{MediaID: "inline-a", Role: models.EditorialMediaRoleInline, InlinePosition: &pos0, AltText: "a alt"},
	})
	require.NoError(t, err)
	approveDraftForPublish(ctx, t, svc, repo, "owner", d1.ID)
	article, err := svc.PublishDraft(ctx, "owner", d1.ID)
	require.NoError(t, err)
	require.Len(t, article.EditorialMedia, 2)

	// A follow-up revision drops the hero and swaps the inline asset; the
	// persisted bindings must be replaced, not merged.
	objectID := article.ID
	d2 := &models.Draft{ID: "d2", AuthorID: "owner", ObjectID: &objectID, ContentType: activitypub.ArticleType, Title: "Two", Slug: "two", Content: "# Two\n\nUpdated body.", ContentFormat: "markdown"}
	require.NoError(t, svc.CreateDraft(ctx, d2))
	_, err = svc.SetEditorialMedia(ctx, "owner", d2.ID, []models.DraftMediaUsage{
		{MediaID: "inline-b", Role: models.EditorialMediaRoleInline, InlinePosition: &pos0, AltText: "b alt"},
	})
	require.NoError(t, err)
	approveDraftForPublish(ctx, t, svc, repo, "owner", d2.ID)
	updated, err := svc.PublishDraft(ctx, "owner", d2.ID)
	require.NoError(t, err)
	require.Len(t, updated.EditorialMedia, 1)
	require.Equal(t, "inline-b", updated.EditorialMedia[0].MediaID)
	require.Equal(t, minter.mints["inline-b"].URL, updated.EditorialMedia[0].URL)
	require.Equal(t, minter.mints["inline-b"].ContentHash, updated.EditorialMedia[0].ContentHash)

	// A follow-up revision with no media clears the persisted bindings.
	objectID = updated.ID
	d3 := &models.Draft{ID: "d3", AuthorID: "owner", ObjectID: &objectID, ContentType: activitypub.ArticleType, Title: "Three", Slug: "three", Content: "# Three\n\nNo media.", ContentFormat: "markdown"}
	require.NoError(t, svc.CreateDraft(ctx, d3))
	approveDraftForPublish(ctx, t, svc, repo, "owner", d3.ID)
	cleared, err := svc.PublishDraft(ctx, "owner", d3.ID)
	require.NoError(t, err)
	require.Empty(t, cleared.EditorialMedia)
}

// TestRenderDraftPreviewWithMediaComposes proves the draft preview renders its
// bound media: hero as the leading image and inline at its block position,
// using the caller-provided descriptors.
func TestRenderDraftPreviewWithMediaComposes(t *testing.T) {
	draft := &models.Draft{ContentType: activitypub.ArticleType, Content: "# T\n\nBody.", ContentFormat: "markdown"}
	rendered, err := RenderDraftPreviewWithMedia(draft, []cmsrender.ArticleMedia{
		{Role: cmsrender.ArticleMediaRoleHero, URL: "https://cdn.example.test/hero.png", AltText: "hero"},
		{Role: cmsrender.ArticleMediaRoleInline, InlinePosition: 0, URL: "https://cdn.example.test/inline.png", AltText: "inline"},
	})
	require.NoError(t, err)
	require.Contains(t, rendered.HTML, "hero.png")
	require.Contains(t, rendered.HTML, "inline.png")
	// Hero leads; an inline at position 0 is the second leading figure before
	// the title block; an insertion point at index N is before block N.
	heroIdx := strings.Index(rendered.HTML, "hero.png")
	inlineIdx := strings.Index(rendered.HTML, "inline.png")
	titleIdx := strings.Index(rendered.HTML, "<h1")
	require.True(t, heroIdx < inlineIdx && inlineIdx < titleIdx, "hero leads, then inline at position 0, then the title")

	// Without descriptors the preview renders content only, unchanged.
	plain, err := RenderDraftPreview(draft)
	require.NoError(t, err)
	require.NotContains(t, plain.HTML, "hero.png")
	require.NotContains(t, plain.HTML, "inline.png")
}
