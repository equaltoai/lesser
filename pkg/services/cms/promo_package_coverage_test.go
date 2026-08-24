package cms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

// These tests exercise the remaining branches of the promo service surface
// (list/queue helpers, error paths, and fail-closed wiring) that the scenario E
// contract tests do not reach, keeping the pkg coverage gate green.

func TestPromoServiceListAndVerdictHelpers(t *testing.T) {
	ctx := context.Background()
	svc, _, media, _ := promoServiceHarness(t)
	digest := promoDigest("11")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)

	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "reviewer")
	require.NoError(t, err)

	// Owner list + reviewer queue + owned review assignments + verdict history.
	packages, next, err := svc.ListPromoPackages(ctx, "alice", 10, "")
	require.NoError(t, err)
	require.Len(t, packages, 1)
	require.Empty(t, next)
	require.Equal(t, pkg.PackageID, packages[0].PackageID)

	queue, nextQ, err := svc.SharedPromoPackageReviews(ctx, "reviewer", 10, "")
	require.NoError(t, err)
	require.Len(t, queue, 1)
	require.Empty(t, nextQ)

	owned, err := svc.OwnedPromoPackageReviews(ctx, "alice")
	require.NoError(t, err)
	require.Len(t, owned, 1)

	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "ok")
	require.NoError(t, err)
	verdicts, err := svc.PromoPackageVerdicts(ctx, "alice", pkg.PackageID)
	require.NoError(t, err)
	require.Len(t, verdicts, 1)

	// The reviewer queue disappears after revocation.
	require.NoError(t, svc.RevokePromoPackageReview(ctx, "alice", pkg.PackageID, "reviewer"))
	queue, _, err = svc.SharedPromoPackageReviews(ctx, "reviewer", 10, "")
	require.NoError(t, err)
	require.Empty(t, queue)
	owned, err = svc.OwnedPromoPackageReviews(ctx, "alice")
	require.NoError(t, err)
	require.Empty(t, owned)

	// Owner pagination past-end terminates.
	_, _, err = svc.ListPromoPackages(ctx, "alice", 10, "zzz-past-end")
	require.NoError(t, err)
}

func TestPromoServiceReviewGuardrails(t *testing.T) {
	ctx := context.Background()
	svc, _, media, _ := promoServiceHarness(t)
	digest := promoDigest("22")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)

	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)

	// Owner cannot share with themselves unless they are the principal.
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "alice")
	require.ErrorContains(t, err, "cannot review their own package")

	// Invalid verdicts are rejected.
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "reviewer")
	require.NoError(t, err)
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, "MAYBE", "")
	require.ErrorContains(t, err, "invalid promo package review verdict")

	// Expired grants fail closed.
	expired := time.Now().UTC().Add(-time.Minute)
	repo := svc.promoRepo
	g, err := repo.GetPromoReviewGrant(ctx, "alice", pkg.PackageID, "reviewer")
	require.NoError(t, err)
	g.ExpiresAt = &expired
	require.NoError(t, repo.RegrantPromoReviewGrant(ctx, g))
	_, err = svc.ActivePromoPackageReviewGrant(ctx, "alice", pkg.PackageID, "reviewer")
	require.ErrorContains(t, err, "not active")

	// Missing packages surface not-found from the owner-scoped read.
	_, err = svc.GetPromoPackage(ctx, "alice", "missing")
	require.Error(t, err)

	// A package resolves for the owner with no grant.
	ownerPkg, ownerGrant, err := svc.PromoPackageForCaller(ctx, "alice", pkg.PackageID)
	require.NoError(t, err)
	require.Equal(t, pkg.PackageID, ownerPkg.PackageID)
	require.Nil(t, ownerGrant)
}

func TestPromoServiceUnwiredAndFailClosed(t *testing.T) {
	ctx := context.Background()
	svc, _, media, _ := promoServiceHarness(t)
	digest := promoDigest("33")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)

	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)

	// Unwired review storage fails closed.
	svc.SetPromoPackageRepository(nil)
	err = svc.RevokePromoPackageReview(ctx, "alice", pkg.PackageID, "reviewer")
	require.ErrorIs(t, err, errPromoReviewStorageUnavailable)
	_, _, err = svc.SharedPromoPackageReviews(ctx, "reviewer", 10, "")
	require.ErrorIs(t, err, errPromoReviewStorageUnavailable)
	_, err = svc.OwnedPromoPackageReviews(ctx, "alice")
	require.ErrorIs(t, err, errPromoReviewStorageUnavailable)
	_, err = svc.PromoPackageVerdicts(ctx, "alice", pkg.PackageID)
	require.ErrorIs(t, err, errPromoReviewStorageUnavailable)
	_, _, err = svc.ListPromoPackages(ctx, "alice", 10, "")
	require.ErrorIs(t, err, errPromoReviewStorageUnavailable)
	_, err = svc.GetPromoPackage(ctx, "alice", pkg.PackageID)
	require.ErrorIs(t, err, errPromoReviewStorageUnavailable)

	// Re-wire for the release failure paths.
	svc, _, media, creator := promoServiceHarness(t)
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)
	pkg, err = svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "reviewer")
	require.NoError(t, err)
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "")
	require.NoError(t, err)

	// A failing status creator blocks release without stamping.
	creator.err = errors.New("status creator boom")
	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.ErrorContains(t, err, "status creator boom")

	// A nil status creator fails closed.
	creator.err = nil
	svc.SetPromoStatusCreator(nil)
	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.ErrorContains(t, err, "promo status creator is unavailable")

	// A creator returning no status fails closed.
	svc.SetPromoStatusCreator(&nilResultPromoStatusCreator{})
	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.ErrorContains(t, err, "created no status")
}

type nilResultPromoStatusCreator struct{}

func (n *nilResultPromoStatusCreator) CreatePromoNote(_ context.Context, _ *notes.CreateNoteCommand, _ []notes.PromoPublishedMediaRef) (*notes.NoteResult, error) {
	return nil, nil
}

func TestPromoServicePrincipalUnavailableBlocksAIOriginRelease(t *testing.T) {
	ctx := context.Background()
	svc, _, media, _ := promoServiceHarness(t)
	digest := promoDigest("44")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginAIGenerated)

	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)
	_, err = svc.SharePromoPackageForReview(ctx, "alice", pkg.PackageID, "reviewer")
	require.NoError(t, err)
	_, err = svc.SubmitPromoPackageReview(ctx, "reviewer", "alice", pkg.PackageID, models.PromoPackageReviewApproved, "")
	require.NoError(t, err)

	// Without a principal provider, the AI-origin gate reports the principal
	// unavailable and blocks release.
	svc.SetPrincipalUsernameProvider(nil)
	state, err := svc.PromoPackageReviewState(ctx, "alice", pkg.PackageID, nil)
	require.NoError(t, err)
	require.False(t, state.ReleaseEligible)
	require.Contains(t, state.BlockingReasons, PromoPackageReviewReasonPrincipalMissing)

	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.Error(t, err)
	require.Len(t, svc.promoStatusCreator.(*recordingPromoStatusCreator).commands, 0)

	// Re-wire the provider; the missing principal approval still blocks.
	svc.SetPrincipalUsernameProvider(func(context.Context) (string, error) { return "principal", nil })
	_, err = svc.ReleasePromoPackage(ctx, "alice", pkg.PackageID)
	require.ErrorIs(t, err, ErrPromoPackagePrincipalApprovalRequired)
}

func TestPromoServiceRemainingFailClosedBranches(t *testing.T) {
	ctx := context.Background()
	svc, _, media, _ := promoServiceHarness(t)
	digest := promoDigest("55")
	media.byID["media-1"] = publishedPromoMedia("media-1", "alice", digest, models.EditorialMediaOriginSupplied)

	pkg, err := svc.ComposePromoPackage(ctx, "alice", promoComposeInput(models.PromoPackageVisibilityPublic, "media-1"))
	require.NoError(t, err)

	// Sharing a package that does not exist fails.
	_, err = svc.SharePromoPackageForReview(ctx, "alice", "missing", "reviewer")
	require.Error(t, err)

	// A reviewer-queue lookup for a package the caller cannot see fails closed.
	_, _, err = svc.PromoPackageForCaller(ctx, "mallory", pkg.PackageID)
	require.Error(t, err)

	// An unwired media repository fails closed at review-state resolution.
	svc.SetEditorialMediaRepository(nil)
	_, err = svc.PromoPackageReviewState(ctx, "alice", pkg.PackageID, nil)
	require.ErrorContains(t, err, "editorial media repository is unavailable")
	svc.SetEditorialMediaRepository(media)

	// A generic media lookup failure fails closed (not classified as missing).
	failing := &failingMediaRepo{inner: media}
	svc.SetEditorialMediaRepository(failing)
	_, err = svc.PromoPackageReviewState(ctx, "alice", pkg.PackageID, nil)
	require.Error(t, err)
	svc.SetEditorialMediaRepository(media)

	// The instance principal may share with themselves explicitly.
	media.byID["media-principal"] = publishedPromoMedia("media-principal", "principal", digest, models.EditorialMediaOriginSupplied)
	principalInput := promoComposeInput(models.PromoPackageVisibilityUnlisted, "media-principal")
	principalPkg, err := svc.ComposePromoPackage(ctx, "principal", principalInput)
	require.NoError(t, err)
	g, err := svc.SharePromoPackageForReview(ctx, "principal", principalPkg.PackageID, "principal")
	require.NoError(t, err)
	require.Equal(t, "principal", g.Reviewer)

	// An active self-grant resolves the package with the grant for the owner.
	selfPkg, selfGrant, err := svc.PromoPackageForCaller(ctx, "principal", principalPkg.PackageID)
	require.NoError(t, err)
	require.Equal(t, principalPkg.PackageID, selfPkg.PackageID)
	require.NotNil(t, selfGrant)
}

type failingMediaRepo struct {
	inner *memMediaRepo
}

func (f *failingMediaRepo) GetMedia(ctx context.Context, mediaID string) (*models.Media, error) {
	return nil, errFailingMediaLookup
}

var errFailingMediaLookup = errors.New("media lookup boom")
