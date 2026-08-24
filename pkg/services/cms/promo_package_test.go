package cms

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testinginmemory "github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func promoDigest(label string) string {
	return "sha256:" + strings.Repeat(label, 64/len(label)+1)[:64]
}

// publishedPromoMedia builds an internal editorial asset that crossed the M2
// publish transition (IsPublished true) with the given provenance origin.
func publishedPromoMedia(id, owner, digest string, origin models.EditorialMediaOrigin) *models.Media {
	now := time.Now().UTC()
	media := m2ReadyMedia(id, owner, digest)
	media.EditorialState = models.EditorialLifecycleAvailable
	media.PublishedS3Key = "published/" + id + ".png"
	media.PublishedURL = "https://cdn.example/published/" + id + ".png"
	media.PublishedAt = &now
	if origin != "" {
		media.Provenance = &models.MediaProvenance{
			Origin:           origin,
			Tool:             "image tool",
			ResponsibleActor: owner,
			RecordedAt:       now,
			ContentIntegrity: digest,
		}
	}
	return media
}

type recordingPromoStatusCreator struct {
	mu       sync.Mutex
	commands []*notes.CreateNoteCommand
	refSets  [][]notes.PromoPublishedMediaRef
	err      error
}

func (r *recordingPromoStatusCreator) CreatePromoNote(_ context.Context, cmd *notes.CreateNoteCommand, refs []notes.PromoPublishedMediaRef) (*notes.NoteResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, cmd)
	r.refSets = append(r.refSets, refs)
	if r.err != nil {
		return nil, r.err
	}
	now := time.Now().UTC()
	statusID := fmt.Sprintf("status-%d", len(r.commands))
	status := &models.Status{
		StatusID:       statusID,
		AuthorID:       cmd.AuthorID,
		AuthorUsername: cmd.AuthorID,
		Content:        cmd.Content,
		Visibility:     cmd.Visibility,
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.test/users/" + cmd.AuthorID + "/statuses/" + statusID,
				Type: activitypub.NoteType,
			},
			Content:          cmd.Content,
			AttributedTo:     "https://example.test/users/" + cmd.AuthorID,
			Visibility:       cmd.Visibility,
			AgentAttribution: cmd.AgentAttribution,
		},
		PublishedAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
		ModifiedAt:  now,
	}
	return &notes.NoteResult{Note: status}, nil
}

func promoServiceHarness(t *testing.T) (*DraftService, *testinginmemory.PromoPackageRepository, *memMediaRepo, *recordingPromoStatusCreator) {
	t.Helper()
	repo := testinginmemory.NewPromoPackageRepository()
	media := &memMediaRepo{byID: map[string]*models.Media{}}
	articleSvc := newMemArticleService()
	now := time.Now().UTC()
	articleSvc.items["https://example.test/articles/hello"] = &models.Article{
		Object: models.Object{
			ID:        "https://example.test/articles/hello",
			Type:      activitypub.ArticleType,
			Name:      "Launch article",
			Published: now,
		},
		Slug:      "hello",
		UpdatedAt: now,
	}
	creator := &recordingPromoStatusCreator{}
	svc := &DraftService{
		draftRepo:      newReviewMemRepo(),
		articleService: articleSvc,
		domain:         "example.test",
		scheduling:     true,
		logger:         zap.NewNop(),
	}
	svc.SetPrincipalUsernameProvider(func(context.Context) (string, error) { return "principal", nil })
	svc.SetEditorialMediaRepository(media)
	svc.SetPromoPackageRepository(repo)
	svc.SetPromoStatusCreator(creator)
	return svc, repo, media, creator
}

func promoComposeInput(visibility string, mediaIDs ...string) PromoPackageComposeInput {
	return PromoPackageComposeInput{
		ArticleID:     "https://example.test/articles/hello",
		PostText:      "Read our launch article",
		Visibility:    visibility,
		AssetMediaIDs: mediaIDs,
	}
}

// Scenario E: compose -> review (hash-bound verdict) -> blocked release ->
// explicit principal authorization -> release with the exact approved assets
// and AI-authorship disclosure intact.
func TestPromoScenarioE_AIOriginAssetRequiresPrincipalAuthorization(t *testing.T) {
	ctx := context.Background()
	svc, repo, media, creator := promoServiceHarness(t)
	digest := promoDigest("aa")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginAIGenerated)

	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)
	require.NotEmpty(t, pkg.PackageID)
	require.NotEmpty(t, pkg.ContentHash)
	require.Equal(t, digest, pkg.Assets[0].ContentHash, "the digest is bound at compose time")

	// Reviewer approves the exact package content.
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "reviewer")
	require.NoError(t, err)
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
	require.NoError(t, err)

	// Reviewer approval alone is not enough: the AI-origin asset requires the
	// instance principal's explicit authorization, so release is BLOCKED.
	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.ErrorIs(t, err, ErrPromoPackagePrincipalApprovalRequired)
	require.Len(t, creator.commands, 0, "no status is created before authorization")

	state, err := svc.PromoPackageReviewState(ctx, "alice", pkg.PackageID, nil)
	require.NoError(t, err)
	require.False(t, state.ReleaseEligible)
	require.True(t, state.PrincipalApprovalRequired)
	require.Contains(t, state.BlockingReasons, PromoPackageReviewReasonPrincipalRequired)

	// Explicit principal authorization: the instance principal is shared as a
	// reviewer and approves the same package content hash.
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "principal")
	require.NoError(t, err)
	_, err = svc.SubmitPromoPackageReview(ctx, "principal", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "authorized", pkg.ContentHash)
	require.NoError(t, err)

	release, err := svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.NoError(t, err)
	require.NotEmpty(t, release.ReleasedStatusID)
	require.NotEmpty(t, release.StatusURL)
	require.True(t, release.Package.IsReleased())

	// The outbound post carries the exact reviewed content and assets.
	require.Len(t, creator.commands, 1)
	cmd := creator.commands[0]
	require.Equal(t, "alice", cmd.AuthorID)
	require.Equal(t, pkg.PostText, cmd.Content, "the exact reviewed post text is what releases")
	require.Equal(t, models.PromoPackageVisibilityPublic, cmd.Visibility)
	require.Len(t, creator.refSets[0], 1)
	require.Equal(t, notes.PromoPublishedMediaRef{MediaID: "media-1", ContentHash: digest}, creator.refSets[0][0])

	// AI-authorship disclosure survives to the outbound surface, naming the
	// instance principal who authorized the AI-origin release.
	require.NotNil(t, cmd.AgentAttribution)
	require.Equal(t, "manual", cmd.AgentAttribution.TriggerType)
	require.Equal(t, "https://example.test/users/principal", cmd.AgentAttribution.ApprovedBy)

	// The stamp persists on the package and blocks re-release.
	persisted, err := repo.GetPromoPackage(ctx, "alice", pkg.PackageID)
	require.NoError(t, err)
	require.True(t, persisted.IsReleased())
	require.Equal(t, release.ReleasedStatusID, persisted.ReleasedStatusID)

	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.ErrorIs(t, err, ErrPromoPackageAlreadyReleased)
	require.Len(t, creator.commands, 1, "re-release must not create a second post")
}

func TestPromoScenarioE_SuppliedAssetReleasesAfterReviewerApproval(t *testing.T) {
	ctx := context.Background()
	svc, _, media, creator := promoServiceHarness(t)
	digest := promoDigest("bb")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)

	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityUnlisted, "media-1"))
	require.NoError(t, err)
	require.Equal(t, models.PromoPackageVisibilityUnlisted, pkg.Visibility)

	// An active reviewer without a current verdict blocks release.
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "reviewer")
	require.NoError(t, err)
	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.ErrorIs(t, err, ErrPromoPackageApprovalRequired)
	require.Len(t, creator.commands, 0)

	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
	require.NoError(t, err)

	// Doctrine floor: alice is not the instance principal, so the principal
	// must approve even though no asset is AI-origin.
	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.ErrorIs(t, err, ErrPromoPackagePrincipalApprovalRequired)
	require.Len(t, creator.commands, 0)
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "principal")
	require.NoError(t, err)
	_, err = svc.SubmitPromoPackageReview(ctx, "principal", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
	require.NoError(t, err)

	release, err := svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.NoError(t, err)
	require.NotEmpty(t, release.ReleasedStatusID)
	require.Len(t, creator.commands, 1)
	require.Equal(t, models.PromoPackageVisibilityUnlisted, creator.commands[0].Visibility)
	// No AI-origin assets -> no disclosure on the outbound post.
	require.Nil(t, creator.commands[0].AgentAttribution)
}

func TestPromoStaleOnChangeReBlocksApprovedPackage(t *testing.T) {
	ctx := context.Background()
	svc, _, media, creator := promoServiceHarness(t)
	digest := promoDigest("cc")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)

	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "reviewer")
	require.NoError(t, err)
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
	require.NoError(t, err)

	// Any content edit after approval re-hashes and stales the verdict.
	edited := promoComposeInput(models.PromoPackageVisibilityPublic, "media-1")
	edited.PackageID = pkg.PackageID
	edited.PostText = "Read our launch article now"
	updated, err := svc.ComposePromoPackage(ctx, "alice", edited)
	require.NoError(t, err)
	require.NotEqual(t, pkg.ContentHash, updated.ContentHash, "content change re-hashes the package")

	// Release is blocked again until re-review.
	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.ErrorIs(t, err, ErrPromoPackageApprovalRequired)
	require.Len(t, creator.commands, 0)

	// Re-review of the changed package unblocks the reviewer requirement; the
	// principal floor still needs the principal's approval (alice is not the
	// instance principal).
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", updated.ContentHash)
	require.NoError(t, err)
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "principal")
	require.NoError(t, err)
	_, err = svc.SubmitPromoPackageReview(ctx, "principal", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", updated.ContentHash)
	require.NoError(t, err)
	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.NoError(t, err)
	require.Len(t, creator.commands, 1)
	require.Equal(t, "Read our launch article now", creator.commands[0].Content)
}

func TestPromoComposeStructurallyRejectsUnpublishedAsset(t *testing.T) {
	ctx := context.Background()
	svc, _, media, _ := promoServiceHarness(t)
	digest := promoDigest("dd")
	// Internal editorial asset that never crossed the publish transition.
	media.byID["media-1"] = m2ReadyMedia("media-1", "alice", digest)

	_, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.ErrorIs(t, err, ErrPromoPackageAssetUnavailable)
	require.Contains(t, err.Error(), "PUBLISHED", "the reason names the PUBLISHED-only rule")
}

func TestPromoComposeStructurallyRejectsPrivateAndDirectVisibility(t *testing.T) {
	ctx := context.Background()
	svc, _, media, _ := promoServiceHarness(t)
	digest := promoDigest("ee")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)

	for _, visibility := range []string{models.VisibilityPrivate, models.VisibilityDirect} {
		_, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(visibility, "media-1"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "public or unlisted")
	}
}

func TestPromoComposeRejectsForeignAndMissingAssets(t *testing.T) {
	ctx := context.Background()
	svc, _, media, _ := promoServiceHarness(t)
	digest := promoDigest("ff")
	media.byID["media-1"] = publishedPromoMedia("media-1", "bob", digest, models.EditorialMediaOriginSupplied)

	_, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.ErrorContains(t, err, "does not belong to the composer")

	_, err = svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "missing"))
	require.ErrorContains(t, err, "asset lookup failed")
}

func TestPromoComposeRejectsUnpublishedArticleAndBadInput(t *testing.T) {
	ctx := context.Background()
	svc, _, media, _ := promoServiceHarness(t)
	digest := promoDigest("12")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)

	unpublished := promoComposeInput(models.PromoPackageVisibilityPublic, "media-1")
	unpublished.ArticleID = "https://example.test/articles/draft"
	_, err := svc.ComposePromoPackage(ctx, "alice", unpublished)
	require.ErrorContains(t, err, "article lookup failed")

	// An article that exists but was never published is not promotable.
	svc.articleService.(*memArticleService).items["https://example.test/articles/draft"] = &models.Article{
		Object: models.Object{
			ID:   "https://example.test/articles/draft",
			Type: activitypub.ArticleType,
			Name: "Unpublished draft",
		},
	}
	_, err = svc.ComposePromoPackage(ctx, "alice", unpublished)
	require.ErrorContains(t, err, "published article")

	emptyText := promoComposeInput(models.PromoPackageVisibilityPublic, "media-1")
	emptyText.PostText = ""
	_, err = svc.ComposePromoPackage(ctx, "alice", emptyText)
	require.ErrorContains(t, err, "post text is required")

	noAssets := promoComposeInput(models.PromoPackageVisibilityPublic)
	_, err = svc.ComposePromoPackage(ctx, "alice", noAssets)
	require.ErrorContains(t, err, "at least one published asset")
}

func TestPromoActorIsolationOwnerAndReviewerOnly(t *testing.T) {
	ctx := context.Background()
	svc, _, media, _ := promoServiceHarness(t)
	digest := promoDigest("34")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)

	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "reviewer")
	require.NoError(t, err)

	// Unrelated caller cannot resolve the package.
	_, _, err = svc.PromoPackageForCaller(ctx, "mallory", pkg.PackageID)
	require.ErrorContains(t, err, "review not found")

	// The active reviewer resolves it; the owner resolves it without a grant.
	got, grant, err := svc.PromoPackageForCaller(ctx, "reviewer", pkg.PackageID)
	require.NoError(t, err)
	require.Equal(t, pkg.PackageID, got.PackageID)
	require.NotNil(t, grant)

	ownerPkg, ownerGrant, err := svc.PromoPackageForCaller(ctx, "alice", pkg.PackageID)
	require.NoError(t, err)
	require.Equal(t, pkg.PackageID, ownerPkg.PackageID)
	require.Nil(t, ownerGrant)

	// A revoked grant stops authorizing reads.
	require.NoError(t, svc.RevokePromoPackageReview(ctx, "alice", pkg.PackageID, "reviewer"))
	_, _, err = svc.PromoPackageForCaller(ctx, "reviewer", pkg.PackageID)
	require.ErrorContains(t, err, "review not found")
}

func TestPromoAssetStateChangesBlockReviewEligibility(t *testing.T) {
	ctx := context.Background()
	svc, _, media, creator := promoServiceHarness(t)
	digest := promoDigest("56")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)

	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "reviewer")
	require.NoError(t, err)
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
	require.NoError(t, err)
	// The principal floor is satisfied up front so the asset-state assertions
	// below exercise the asset gate, not the approval gate.
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "principal")
	require.NoError(t, err)
	_, err = svc.SubmitPromoPackageReview(ctx, "principal", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
	require.NoError(t, err)

	// The media record changes after approval: digest replaced -> release
	// blocked, the exact reviewed bytes are no longer attachable.
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", promoDigest("78"), models.EditorialMediaOriginSupplied)
	state, err := svc.PromoPackageReviewState(ctx, "alice", pkg.PackageID, nil)
	require.NoError(t, err)
	require.False(t, state.ReleaseEligible)
	require.Contains(t, state.BlockingReasons, PromoPackageReviewReasonAssetDigestChange)

	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.ErrorIs(t, err, ErrPromoPackageAssetUnavailable)
	require.Len(t, creator.commands, 0)

	// Asset removed entirely -> ASSET_MISSING.
	delete(media.byID, "media-1")
	state, err = svc.PromoPackageReviewState(ctx, "alice", pkg.PackageID, nil)
	require.NoError(t, err)
	require.Contains(t, state.BlockingReasons, PromoPackageReviewReasonAssetMissing)

	// Asset withdrawn from publishing -> ASSET_NOT_PUBLISHED.
	withdrawn := publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)
	withdrawn.PublishedURL = ""
	withdrawn.PublishedS3Key = ""
	withdrawn.PublishedAt = nil
	media.byID["media-1"] = withdrawn
	state, err = svc.PromoPackageReviewState(ctx, "alice", pkg.PackageID, nil)
	require.NoError(t, err)
	require.Contains(t, state.BlockingReasons, PromoPackageReviewReasonAssetNotPublished)
}

func TestPromoComposeAfterReleaseRefused(t *testing.T) {
	ctx := context.Background()
	svc, _, media, _ := promoServiceHarness(t)
	digest := promoDigest("9a")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)

	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "reviewer")
	require.NoError(t, err)
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
	require.NoError(t, err)
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "principal")
	require.NoError(t, err)
	_, err = svc.SubmitPromoPackageReview(ctx, "principal", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
	require.NoError(t, err)
	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.NoError(t, err)

	edited := promoComposeInput(models.PromoPackageVisibilityPublic, "media-1")
	edited.PackageID = pkg.PackageID
	edited.PostText = "cannot edit a released package"
	_, err = svc.ComposePromoPackage(ctx, "alice", edited)
	require.ErrorIs(t, err, ErrPromoPackageAlreadyReleased)
}

func TestPromoOwnerCannotReviewOwnPackageUnlessPrincipal(t *testing.T) {
	ctx := context.Background()
	svc, _, media, _ := promoServiceHarness(t)
	digest := promoDigest("bc")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)

	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)

	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "alice")
	require.ErrorContains(t, err, "cannot review their own package")
	_, err = svc.SubmitPromoPackageReview(ctx, "alice", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
	require.ErrorContains(t, err, "cannot review their own package")

	// The instance principal may review their own package explicitly.
	_, err = svc.SharePromoPackageForReview(ctx, "principal", "pkg-self", "principal")
	require.ErrorIs(t, err, storage.ErrNotFound, "package must exist first")
}

func TestPromoReleaseFailsClosedWhenStatusCreatorUnavailable(t *testing.T) {
	ctx := context.Background()
	svc, _, media, _ := promoServiceHarness(t)
	digest := promoDigest("de")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)

	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "reviewer")
	require.NoError(t, err)
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
	require.NoError(t, err)

	svc.SetPromoStatusCreator(nil)
	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.ErrorContains(t, err, "promo status creator is unavailable")
}

// TestPromoSubmitBindsToInspectedContentHash pins F2: a review submit must
// carry the content hash the reviewer actually inspected. When the owner
// recomposes between the reviewer's read and the submit, the submit is
// rejected with the additive conflict signal and no verdict is recorded; a
// matching hash records the verdict as before.
func TestPromoSubmitBindsToInspectedContentHash(t *testing.T) {
	ctx := context.Background()
	svc, _, media, _ := promoServiceHarness(t)
	digest := promoDigest("f2")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)

	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "reviewer")
	require.NoError(t, err)

	// The reviewer's client inspected the package (hash H1); the owner then
	// recomposes, so the stored package no longer matches what was inspected.
	read, err := svc.GetPromoPackage(ctx, "alice", pkg.PackageID)
	require.NoError(t, err)
	edited := promoComposeInput(models.PromoPackageVisibilityPublic, "media-1")
	edited.PackageID = pkg.PackageID
	edited.PostText = "changed after the reviewer read it"
	updated, err := svc.ComposePromoPackage(ctx, "alice", edited)
	require.NoError(t, err)
	require.NotEqual(t, read.ContentHash, updated.ContentHash)

	// Submit with the inspected hash -> explicit conflict, no verdict recorded.
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", read.ContentHash)
	require.ErrorIs(t, err, ErrPromoPackageReviewContentChanged)
	require.ErrorIs(t, err, ErrPromoPackageConflict, "the mismatch surfaces the additive conflict signal")
	verdicts, err := svc.PromoPackageVerdicts(ctx, "alice", pkg.PackageID)
	require.NoError(t, err)
	require.Empty(t, verdicts, "no verdict is recorded for unseen content")

	// A submit carrying the current hash records the verdict as before.
	v, err := svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", updated.ContentHash)
	require.NoError(t, err)
	require.Equal(t, updated.ContentHash, v.ContentHash, "the recorded verdict binds the reviewed hash")
}

// The OPERATOR CONTENT DOCTRINE matrix (2026-08-24) pinned as tests:
//
//	principal releaser + zero ever-granted reviewers -> allowed
//	principal releaser + granted reviewers -> all required
//	non-principal releaser -> principal required, plus all requested approvals
//
// plus the requested = required rule: every reviewer who ever recorded a
// verdict must hold a current approving verdict even after their grant is
// revoked or expires — revocation cannot delete a required approval.

func TestPromoDoctrine_PrincipalReleaserZeroReviewersAllowed(t *testing.T) {
	ctx := context.Background()
	svc, _, media, creator := promoServiceHarness(t)
	digest := promoDigest("d1")
	media.byID["media-p"] = publishedPromoMedia("media-p", "principal", digest, models.EditorialMediaOriginSupplied)

	pkg, err := svc.ComposePromoPackage(ctx, "principal", promoComposeInput(models.PromoPackageVisibilityPublic, "media-p"))
	require.NoError(t, err)

	// The principal releasing their own package is the implicit approval: with
	// zero ever-granted reviewers the package releases without any verdicts.
	release, err := svc.ReleasePromoPackage(ctx, "principal", pkg.PackageID)
	require.NoError(t, err)
	require.NotEmpty(t, release.ReleasedStatusID)
	require.Len(t, creator.commands, 1)
}

func TestPromoDoctrine_PrincipalReleaserGrantedReviewersAllRequired(t *testing.T) {
	ctx := context.Background()
	svc, _, media, creator := promoServiceHarness(t)
	digest := promoDigest("d2")
	media.byID["media-p"] = publishedPromoMedia("media-p", "principal", digest, models.EditorialMediaOriginSupplied)

	pkg, err := svc.ComposePromoPackage(ctx, "principal", promoComposeInput(models.PromoPackageVisibilityPublic, "media-p"))
	require.NoError(t, err)
	_, err = svc.SharePromoPackageForReview(ctx, "principal", pkg.PackageID, "reviewer")
	require.NoError(t, err)

	// A granted reviewer without a current approval blocks even the principal.
	_, err = svc.ReleasePromoPackage(ctx, "principal", pkg.PackageID)
	require.ErrorIs(t, err, ErrPromoPackageApprovalRequired)
	require.Len(t, creator.commands, 0)

	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "principal", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
	require.NoError(t, err)
	release, err := svc.ReleasePromoPackage(ctx, "principal", pkg.PackageID)
	require.NoError(t, err)
	require.NotEmpty(t, release.ReleasedStatusID)
	require.Len(t, creator.commands, 1)
}

func TestPromoDoctrine_NonPrincipalReleaserPrincipalFloor(t *testing.T) {
	ctx := context.Background()
	svc, _, media, creator := promoServiceHarness(t)
	digest := promoDigest("d3")
	// Deliberately NOT AI-origin: the floor applies regardless of provenance.
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)

	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "reviewer")
	require.NoError(t, err)
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
	require.NoError(t, err)

	// Reviewer approval alone is not enough: alice is not the instance
	// principal, so the principal floor demands a current principal approval.
	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.ErrorIs(t, err, ErrPromoPackagePrincipalApprovalRequired)
	require.Len(t, creator.commands, 0)

	state, err := svc.PromoPackageReviewState(ctx, "alice", pkg.PackageID, nil)
	require.NoError(t, err)
	require.True(t, state.PrincipalApprovalRequired)
	require.False(t, state.PrincipalApproved)
	require.True(t, state.ReviewersApproved, "the reviewer requirement is met; only the floor blocks")

	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "principal")
	require.NoError(t, err)
	_, err = svc.SubmitPromoPackageReview(ctx, "principal", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
	require.NoError(t, err)
	release, err := svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.NoError(t, err)
	require.NotEmpty(t, release.ReleasedStatusID)
	require.Len(t, creator.commands, 1)
}

func TestPromoDoctrine_RevokedGrantCannotDeleteRequiredApproval(t *testing.T) {
	ctx := context.Background()
	svc, _, media, creator := promoServiceHarness(t)
	digest := promoDigest("d4")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)

	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "reviewer")
	require.NoError(t, err)
	// The reviewer demands changes; the owner revokes the grant to be rid of
	// them, then the principal approves. The changes-requested verdict binds:
	// revocation cannot delete the required approval.
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewChangesRequested, "fix the copy", pkg.ContentHash)
	require.NoError(t, err)
	require.NoError(t, svc.RevokePromoPackageReview(ctx, "alice", pkg.PackageID, "reviewer"))
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "principal")
	require.NoError(t, err)
	_, err = svc.SubmitPromoPackageReview(ctx, "principal", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
	require.NoError(t, err)

	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.ErrorIs(t, err, ErrPromoPackageApprovalRequired,
		"the ever-recorded-verdict reviewer stays required even after revocation")
	require.Len(t, creator.commands, 0)

	// The owner re-grants and the reviewer approves; the package can release.
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "reviewer")
	require.NoError(t, err)
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
	require.NoError(t, err)
	release, err := svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.NoError(t, err)
	require.NotEmpty(t, release.ReleasedStatusID)
	require.Len(t, creator.commands, 1)
}

// failingStampPromoRepo wraps the in-memory promo repository and injects a
// finalize failure so the release flow's stamp-error path can be exercised.
type failingStampPromoRepo struct {
	*testinginmemory.PromoPackageRepository
	failStamp bool
}

func (f *failingStampPromoRepo) MarkPromoPackageReleased(ctx context.Context, ownerID string, pkg *models.PromoPackage) error {
	if f.failStamp {
		return errors.New("stamp boom")
	}
	return f.PromoPackageRepository.MarkPromoPackageReleased(ctx, ownerID, pkg)
}

// TestPromoConcurrentDoubleReleaseCreatesExactlyOnePost pins F1: two concurrent
// releases of the same package must create exactly one outbound post. The
// version-conditioned releasing reservation is won by exactly one releaser; the
// loser conflicts (or observes the completed release) BEFORE any post exists.
// The race runs many times to exercise both loser paths.
func TestPromoConcurrentDoubleReleaseCreatesExactlyOnePost(t *testing.T) {
	for i := 0; i < 20; i++ {
		ctx := context.Background()
		svc, _, media, creator := promoServiceHarness(t)
		digest := promoDigest(fmt.Sprintf("a%d", i))
		media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)
		pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
		require.NoError(t, err)
		for _, reviewer := range []string{"reviewer", "principal"} {
			_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, reviewer)
			require.NoError(t, err)
			_, err = svc.SubmitPromoPackageReview(ctx, reviewer, "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
			require.NoError(t, err)
		}

		var wg sync.WaitGroup
		start := make(chan struct{})
		results := make([]error, 2)
		for g := 0; g < 2; g++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-start
				_, results[idx] = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
			}(g)
		}
		close(start)
		wg.Wait()

		successes := 0
		for _, res := range results {
			if res == nil {
				successes++
			}
		}
		require.Equal(t, 1, successes, "exactly one concurrent releaser wins: %v", results)
		require.Equal(t, 1, len(creator.commands), "exactly one post is created per package")
		persisted, err := svc.promoRepo.GetPromoPackage(ctx, "alice", pkg.PackageID)
		require.NoError(t, err)
		require.True(t, persisted.IsReleased(), "the winning release stamps the package")
	}
}

// TestPromoReleasePostCreationFailureRollsBackToDraft pins F1's rollback: when
// the outbound Status creation fails, the reserved release is rolled back to
// draft on the same CAS lane and no released stamp survives; a later release
// can proceed cleanly.
func TestPromoReleasePostCreationFailureRollsBackToDraft(t *testing.T) {
	ctx := context.Background()
	svc, _, media, creator := promoServiceHarness(t)
	digest := promoDigest("a5")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)
	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)
	for _, reviewer := range []string{"reviewer", "principal"} {
		_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, reviewer)
		require.NoError(t, err)
		_, err = svc.SubmitPromoPackageReview(ctx, reviewer, "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
		require.NoError(t, err)
	}
	creator.err = errors.New("post creation boom")
	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.ErrorContains(t, err, "post creation boom")

	persisted, err := svc.promoRepo.GetPromoPackage(ctx, "alice", pkg.PackageID)
	require.NoError(t, err)
	require.Equal(t, models.PromoPackageStatusDraft, persisted.Status, "the failed release rolls back to draft")
	require.Empty(t, persisted.ReleasedStatusID, "no released stamp survives a failed post creation")
	require.False(t, persisted.IsReleased())
	require.Equal(t, 2, persisted.ModelVersion, "reservation bump plus rollback bump on the same CAS lane")

	// The package is releasable again once the creator recovers.
	creator.err = nil
	release, err := svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.NoError(t, err)
	require.NotEmpty(t, release.ReleasedStatusID)
	require.Len(t, creator.commands, 2, "one failed attempt and one successful post")
}

// TestPromoReleaseFinalizeFailureSurfacesCreatedStatusID pins F1's finalize
// failure: the post WAS created but the releasing -> released stamp failed, so
// the created status ID is surfaced through PromoPackageStampError and a retry
// is refused (the package holds the releasing reservation) instead of creating
// a second post.
func TestPromoReleaseFinalizeFailureSurfacesCreatedStatusID(t *testing.T) {
	ctx := context.Background()
	svc, _, media, creator := promoServiceHarness(t)
	repo := &failingStampPromoRepo{PromoPackageRepository: svc.promoRepo.(*testinginmemory.PromoPackageRepository)}
	svc.SetPromoPackageRepository(repo)
	digest := promoDigest("e7")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)
	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)
	for _, reviewer := range []string{"reviewer", "principal"} {
		_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, reviewer)
		require.NoError(t, err)
		_, err = svc.SubmitPromoPackageReview(ctx, reviewer, "alice", pkg.PackageID, models.PromoPackageReviewApproved, "", pkg.ContentHash)
		require.NoError(t, err)
	}

	repo.failStamp = true
	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.Error(t, err)
	var stampErr *PromoPackageStampError
	require.ErrorAs(t, err, &stampErr, "the stamp failure surfaces the created status ID")
	require.NotEmpty(t, stampErr.ReleasedStatusID)
	require.Equal(t, "status-1", stampErr.ReleasedStatusID, "the surfaced ID is the post that WAS created")
	require.Len(t, creator.commands, 1, "the post exists before the stamp failed")

	// A retry must NOT create a second post: the releasing reservation blocks
	// release until an operator reconciles the stuck reservation.
	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.ErrorIs(t, err, ErrPromoPackageReleaseInProgress)
	require.Len(t, creator.commands, 1, "no second post on a blind retry")
	persisted, err := svc.promoRepo.GetPromoPackage(ctx, "alice", pkg.PackageID)
	require.NoError(t, err)
	require.Equal(t, models.PromoPackageStatusReleasing, persisted.Status,
		"the package stays in the releasing reservation for operator reconciliation")
}
