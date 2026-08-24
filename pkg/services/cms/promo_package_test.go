package cms

import (
	"context"
	"fmt"
	"strings"
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
	commands []*notes.CreateNoteCommand
	refSets  [][]notes.PromoPublishedMediaRef
	err      error
}

func (r *recordingPromoStatusCreator) CreatePromoNote(_ context.Context, cmd *notes.CreateNoteCommand, refs []notes.PromoPublishedMediaRef) (*notes.NoteResult, error) {
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
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "")
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
	_, err = svc.SubmitPromoPackageReview(ctx, "principal", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "authorized")
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

	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "")
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
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "")
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

	// Re-review of the changed package unblocks it.
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "")
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
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "")
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
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "")
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
	_, err = svc.SubmitPromoPackageReview(ctx, "alice", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "")
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
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "")
	require.NoError(t, err)

	svc.SetPromoStatusCreator(nil)
	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.ErrorContains(t, err, "promo status creator is unavailable")
}
