package cms

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// memMediaRepo is a minimal editorialMediaRepository double. The production
// doubles model field-scoped writers elsewhere; here the media records are
// seeded in place because the CMS service only reads them.
type memMediaRepo struct {
	byID map[string]*models.Media
}

func (m *memMediaRepo) GetMedia(_ context.Context, mediaID string) (*models.Media, error) {
	media, ok := m.byID[mediaID]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return media, nil
}

func m2ReadyMedia(id, owner, digest string) *models.Media {
	return &models.Media{
		MediaID:     id,
		UserID:      owner,
		ContentType: "image/png",
		FileSize:    12,
		ContentHash: digest,
		S3Bucket:    "media-private",
		S3Key:       "owner/" + id + ".png",
		Status:      "ready",
		Width:       120,
		Height:      80,
		Visibility:  models.MediaVisibilityInternal,
		Provenance: &models.MediaProvenance{
			Origin:           models.EditorialMediaOriginSupplied,
			ResponsibleActor: owner,
			RecordedAt:       time.Now().UTC(),
			ContentIntegrity: digest,
		},
	}
}

func m2Digest(label string) string {
	return "sha256:" + strings.Repeat(label, 64/len(label)+1)[:64]
}

// m2ReviewService builds a DraftService wired with a seeded media repository
// and a recording publish minter for the byte-bound revision tests.
func m2ReviewService(t *testing.T) (*DraftService, *reviewMemRepo, *memMediaRepo, *recordingPublishMinter) {
	t.Helper()
	repo := newReviewMemRepo()
	media := &memMediaRepo{byID: map[string]*models.Media{}}
	minter := &recordingPublishMinter{mints: map[string]EditorialPublishedMedia{}}
	svc := &DraftService{
		draftRepo:      repo,
		articleService: newMemArticleService(),
		domain:         "example.test",
		scheduling:     true,
		logger:         zap.NewNop(),
	}
	svc.SetPrincipalUsernameProvider(func(context.Context) (string, error) { return "principal", nil })
	svc.SetEditorialMediaRepository(media)
	svc.SetEditorialPublishMinter(minter)
	return svc, repo, media, minter
}

type recordingPublishMinter struct {
	mints   map[string]EditorialPublishedMedia
	calls   []string
	err     error
	missErr error
}

func (m *recordingPublishMinter) PublishEditorialMedia(_ context.Context, mediaID string) (*EditorialPublishedMedia, error) {
	m.calls = append(m.calls, mediaID)
	if m.err != nil {
		return nil, m.err
	}
	if m.missErr != nil && m.missErr.Error() == mediaID {
		return nil, errors.New("durable copy failed for " + mediaID)
	}
	mint, ok := m.mints[mediaID]
	if !ok {
		return nil, fmt.Errorf("no mint recorded for %s", mediaID)
	}
	return &mint, nil
}

func (m *recordingPublishMinter) seedFromMedia(media *memMediaRepo) {
	for id, record := range media.byID {
		m.mints[id] = EditorialPublishedMedia{
			MediaID:     id,
			ContentHash: record.ContentHash,
			ContentType: record.ContentType,
			FileSize:    record.FileSize,
			Width:       record.Width,
			Height:      record.Height,
			URL:         "https://cdn.example.test/published/" + id + ".png",
			S3Key:       "published/owner/" + id + ".png",
			PublishedAt: time.Now().UTC(),
		}
	}
}

func TestDraftReviewContentHashBindsBoundMediaBytes(t *testing.T) {
	digestA := m2Digest("a")
	digestB := m2Digest("b")
	position0 := 0
	position1 := 1
	base := &models.Draft{
		ContentFormat: "markdown", Slug: "s", Title: "t", Content: "c",
		EditorialMedia: []models.DraftMediaUsage{{MediaID: "hero", Role: models.EditorialMediaRoleHero}},
	}

	hashA := draftReviewContentHash(base, map[string]string{"hero": digestA})
	hashB := draftReviewContentHash(base, map[string]string{"hero": digestB})
	require.NotEqual(t, hashA, hashB, "swapping the bound asset bytes must change the revision hash")

	recaptioned := *base
	recaptioned.EditorialMedia = []models.DraftMediaUsage{{MediaID: "hero", Role: models.EditorialMediaRoleHero, Caption: "new caption"}}
	require.NotEqual(t, hashA, draftReviewContentHash(&recaptioned, map[string]string{"hero": digestA}), "recaptioning must change the revision hash")

	refocused := *base
	refocused.EditorialMedia = []models.DraftMediaUsage{{MediaID: "hero", Role: models.EditorialMediaRoleHero, Focus: "0.5,0.5"}}
	require.NotEqual(t, hashA, draftReviewContentHash(&refocused, map[string]string{"hero": digestA}), "refocusing must change the revision hash")

	// A usage whose digest cannot be resolved still contributes its binding
	// shape; the hash is deterministic but distinct from the resolved hash.
	require.NotEqual(t, hashA, draftReviewContentHash(base, nil), "unresolvable bytes must not hash as the resolved digest")

	inlineAsc := &models.Draft{
		ContentFormat: "markdown", Slug: "s", Title: "t", Content: "c",
		EditorialMedia: []models.DraftMediaUsage{
			{MediaID: "i0", Role: models.EditorialMediaRoleInline, InlinePosition: &position0},
			{MediaID: "i1", Role: models.EditorialMediaRoleInline, InlinePosition: &position1},
		},
	}
	reordered := &models.Draft{
		ContentFormat: "markdown", Slug: "s", Title: "t", Content: "c",
		EditorialMedia: []models.DraftMediaUsage{
			{MediaID: "i1", Role: models.EditorialMediaRoleInline, InlinePosition: &position1},
			{MediaID: "i0", Role: models.EditorialMediaRoleInline, InlinePosition: &position0},
		},
	}
	digests := map[string]string{"i0": digestA, "i1": digestB}
	require.Equal(t, draftReviewContentHash(inlineAsc, digests), draftReviewContentHash(reordered, digests),
		"hash order is canonical by position, not by list order")
	require.NotEqual(t, draftReviewContentHash(inlineAsc, digests), draftReviewContentHash(inlineAsc, map[string]string{"i0": digestA, "i1": digestA}),
		"changing an inline asset's bytes must change the revision hash")

	hero := &models.Draft{ContentFormat: "markdown", Slug: "s", Title: "t", Content: "c",
		EditorialMedia: []models.DraftMediaUsage{{MediaID: "hero", Role: models.EditorialMediaRoleHero}}}
	socialOnly := &models.Draft{ContentFormat: "markdown", Slug: "s", Title: "t", Content: "c",
		EditorialMedia: []models.DraftMediaUsage{{MediaID: "card", Role: models.EditorialMediaRoleSocialCard}}}
	require.NotEqual(t, draftReviewContentHash(hero, map[string]string{"hero": digestA}),
		draftReviewContentHash(socialOnly, map[string]string{"card": digestA}),
		"role changes the revision hash even with identical bytes")
}

func TestDraftReviewMediaSwapStalesPriorApprovalAndBlocksPublish(t *testing.T) {
	svc, repo, media, minter := m2ReviewService(t)
	ctx := context.Background()
	digestA := m2Digest("a")
	digestB := m2Digest("b")
	media.byID["asset-a"] = m2ReadyMedia("asset-a", "owner", digestA)
	media.byID["asset-b"] = m2ReadyMedia("asset-b", "owner", digestB)
	minter.seedFromMedia(media)

	draft := &models.Draft{ID: "media-swap", AuthorID: "owner", ContentType: activitypub.ArticleType, Title: "Media swap", Slug: "media-swap", Content: "draft", ContentFormat: "markdown"}
	require.NoError(t, svc.CreateDraft(ctx, draft))

	_, err := svc.SetEditorialMedia(ctx, "owner", draft.ID, []models.DraftMediaUsage{{MediaID: "asset-a", Role: models.EditorialMediaRoleHero, Caption: "Launch"}})
	require.NoError(t, err)
	require.NoError(t, repo.storeGrant(&models.DraftReviewGrant{
		OwnerID: "owner", DraftID: draft.ID, Reviewer: "reviewer", GrantedAt: time.Now().UTC(),
		ExpiresAt: ptrTime(time.Now().UTC().Add(time.Hour)),
	}))
	verdict, err := svc.SubmitDraftReview(ctx, "reviewer", "owner", draft.ID, DraftReviewApproved, "A approved")
	require.NoError(t, err)
	require.Equal(t, draftReviewContentHash(mustDraft(t, svc, "owner", draft.ID), map[string]string{"asset-a": digestA}), verdict.ContentHash)

	state, err := svc.DraftReviewState(ctx, "owner", draft.ID, nil)
	require.NoError(t, err)
	require.True(t, state.PublishEligible)
	require.Empty(t, state.BlockingReasons)

	// Swap image A for image B through the validated field-scoped lane.
	_, err = svc.SetEditorialMedia(ctx, "owner", draft.ID, []models.DraftMediaUsage{{MediaID: "asset-b", Role: models.EditorialMediaRoleHero, Caption: "Launch"}})
	require.NoError(t, err)

	state, err = svc.DraftReviewState(ctx, "owner", draft.ID, nil)
	require.NoError(t, err)
	require.False(t, state.PublishEligible, "the swap must stale the prior approval")
	require.Contains(t, state.BlockingReasons, "REVIEW_APPROVAL_REQUIRED")

	_, err = svc.PublishDraft(ctx, "owner", draft.ID)
	require.ErrorIs(t, err, ErrDraftReviewApprovalRequired, "publish must block on the stale approval")

	// Re-review and authorize the B revision.
	_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", draft.ID, DraftReviewApproved, "B approved")
	require.NoError(t, err)
	state, err = svc.DraftReviewState(ctx, "owner", draft.ID, nil)
	require.NoError(t, err)
	require.True(t, state.PublishEligible)

	article, err := svc.PublishDraft(ctx, "owner", draft.ID)
	require.NoError(t, err)
	require.NotNil(t, article)
	require.Equal(t, digestB, article.FeaturedImage.ContentHash, "the published featured image must carry the exact approved B bytes")
	require.Contains(t, article.FeaturedImage.CDNUrl, "asset-b", "the published featured image resolves to the durable B serving")
}

func TestDraftReviewPublishGateBlocksRequiredBoundMedia(t *testing.T) {
	svc, repo, media, minter := m2ReviewService(t)
	ctx := context.Background()
	digestA := m2Digest("a")
	media.byID["asset-a"] = m2ReadyMedia("asset-a", "owner", digestA)
	minter.seedFromMedia(media)

	draft := &models.Draft{ID: "media-gate", AuthorID: "owner", ContentType: activitypub.ArticleType, Title: "Gate", Slug: "gate", Content: "draft", ContentFormat: "markdown"}
	require.NoError(t, svc.CreateDraft(ctx, draft))
	_, err := svc.SetEditorialMedia(ctx, "owner", draft.ID, []models.DraftMediaUsage{{MediaID: "asset-a", Role: models.EditorialMediaRoleHero}})
	require.NoError(t, err)
	require.NoError(t, repo.storeGrant(&models.DraftReviewGrant{
		OwnerID: "owner", DraftID: draft.ID, Reviewer: "reviewer", GrantedAt: time.Now().UTC(),
		ExpiresAt: ptrTime(time.Now().UTC().Add(time.Hour)),
	}))
	_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", draft.ID, DraftReviewApproved, "approved")
	require.NoError(t, err)

	// Withdraw the bound asset: approvals go stale AND the media gate names the reason.
	withdrawn := *media.byID["asset-a"]
	withdrawn.EditorialState = models.EditorialLifecycleWithdrawn
	media.byID["asset-a"] = &withdrawn

	state, err := svc.DraftReviewState(ctx, "owner", draft.ID, nil)
	require.NoError(t, err)
	require.False(t, state.PublishEligible)
	require.Contains(t, state.BlockingReasons, DraftReviewMediaReasonWithdrawn)
	require.Contains(t, state.BlockingReasons, "REVIEW_APPROVAL_REQUIRED")

	_, err = svc.PublishDraft(ctx, "owner", draft.ID)
	require.ErrorIs(t, err, ErrDraftReviewApprovalRequired)

	// Re-review the withdrawn revision: approvals pass but the media gate blocks.
	_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", draft.ID, DraftReviewApproved, "approved while withdrawn")
	require.NoError(t, err)
	_, err = svc.PublishDraft(ctx, "owner", draft.ID)
	require.ErrorIs(t, err, ErrDraftReviewMediaRequired)
	require.Contains(t, err.Error(), DraftReviewMediaReasonWithdrawn)

	// A missing bound asset blocks with the missing reason after re-review.
	delete(media.byID, "asset-a")
	_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", draft.ID, DraftReviewApproved, "approved while missing")
	require.NoError(t, err)
	_, err = svc.PublishDraft(ctx, "owner", draft.ID)
	require.ErrorIs(t, err, ErrDraftReviewMediaRequired)
	require.Contains(t, err.Error(), DraftReviewMediaReasonMissing)

	// A not-ready bound asset blocks after re-review.
	media.byID["asset-a"] = m2ReadyMedia("asset-a", "owner", digestA)
	pending := *media.byID["asset-a"]
	pending.Status = models.StatusPending
	media.byID["asset-a"] = &pending
	_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", draft.ID, DraftReviewApproved, "approved while pending")
	require.NoError(t, err)
	_, err = svc.PublishDraft(ctx, "owner", draft.ID)
	require.ErrorIs(t, err, ErrDraftReviewMediaRequired)
	require.Contains(t, err.Error(), DraftReviewMediaReasonNotReady)
}

func TestDraftReviewPublishMintVerifiesApprovedBytes(t *testing.T) {
	svc, repo, media, minter := m2ReviewService(t)
	ctx := context.Background()
	digestA := m2Digest("a")
	digestB := m2Digest("b")
	media.byID["hero"] = m2ReadyMedia("hero", "owner", digestA)
	media.byID["card"] = m2ReadyMedia("card", "owner", digestB)
	// Seed the minter with the digest of the wrong asset to prove the mint
	// verification compares the minted bytes to the approved revision digest.
	minter.seedFromMedia(media)
	wrong := minter.mints["hero"]
	wrong.ContentHash = digestB
	minter.mints["hero"] = wrong

	draft := &models.Draft{ID: "mint-verify", AuthorID: "owner", ContentType: activitypub.ArticleType, Title: "Mint", Slug: "mint", Content: "draft", ContentFormat: "markdown"}
	require.NoError(t, svc.CreateDraft(ctx, draft))
	_, err := svc.SetEditorialMedia(ctx, "owner", draft.ID, []models.DraftMediaUsage{
		{MediaID: "hero", Role: models.EditorialMediaRoleHero},
		{MediaID: "card", Role: models.EditorialMediaRoleSocialCard},
	})
	require.NoError(t, err)
	require.NoError(t, repo.storeGrant(&models.DraftReviewGrant{
		OwnerID: "owner", DraftID: draft.ID, Reviewer: "reviewer", GrantedAt: time.Now().UTC(),
		ExpiresAt: ptrTime(time.Now().UTC().Add(time.Hour)),
	}))
	_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", draft.ID, DraftReviewApproved, "approved")
	require.NoError(t, err)

	_, err = svc.PublishDraft(ctx, "owner", draft.ID)
	require.ErrorIs(t, err, ErrDraftReviewMediaRequired)
	require.Contains(t, err.Error(), "bytes changed after approval")

	// Correct the minter and publish: hero flows into featuredImage, the social
	// card into ogImage, and the durable URLs carry the approved digests.
	minter.calls = nil
	correct := minter.mints["hero"]
	correct.ContentHash = digestA
	minter.mints["hero"] = correct
	article, err := svc.PublishDraft(ctx, "owner", draft.ID)
	require.NoError(t, err)
	require.NotNil(t, article.FeaturedImage)
	require.Equal(t, digestA, article.FeaturedImage.ContentHash)
	require.Equal(t, minter.mints["hero"].URL, article.FeaturedImage.CDNUrl)
	require.Equal(t, minter.mints["card"].URL, article.OGImage)
	require.Equal(t, []string{"hero", "card"}, minter.calls)
}

func TestDraftReviewMediaResolutionFailureFailsClosed(t *testing.T) {
	svc, repo := newReviewService(t)
	ctx := context.Background()
	digestA := m2Digest("a")
	draft := &models.Draft{ID: "no-media-repo", AuthorID: "owner", ContentType: activitypub.ArticleType, Title: "No repo", Slug: "no-repo", Content: "draft", ContentFormat: "markdown"}
	require.NoError(t, svc.CreateDraft(ctx, draft))
	// Wire a media repository for the bind, then remove it to simulate a broken
	// wiring while a draft still carries bound media.
	media := &memMediaRepo{byID: map[string]*models.Media{"hero": m2ReadyMedia("hero", "owner", digestA)}}
	svc.SetEditorialMediaRepository(media)
	_, err := svc.SetEditorialMedia(ctx, "owner", draft.ID, []models.DraftMediaUsage{{MediaID: "hero", Role: models.EditorialMediaRoleHero}})
	require.NoError(t, err)
	svc.SetEditorialMediaRepository(nil)

	_, err = svc.DraftReviewState(ctx, "owner", draft.ID, nil)
	require.ErrorContains(t, err, "editorial media repository is unavailable")

	require.NoError(t, repo.storeGrant(&models.DraftReviewGrant{
		OwnerID: "owner", DraftID: draft.ID, Reviewer: "reviewer", GrantedAt: time.Now().UTC(),
		ExpiresAt: ptrTime(time.Now().UTC().Add(time.Hour)),
	}))
	_, err = svc.PublishDraft(ctx, "owner", draft.ID)
	require.ErrorContains(t, err, "editorial media repository is unavailable")
}

func mustDraft(t *testing.T, svc *DraftService, owner, draftID string) *models.Draft {
	t.Helper()
	draft, err := svc.GetDraft(context.Background(), owner, draftID)
	require.NoError(t, err)
	return draft
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
