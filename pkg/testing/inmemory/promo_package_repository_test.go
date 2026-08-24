package inmemory

import (
	"context"
	"strings"
	"testing"
	"time"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func inmemPromoDigest(label string) string {
	return "sha256:" + strings.Repeat(label, 64/len(label)+1)[:64]
}

func inmemPromoFixture(packageID, owner string) *models.PromoPackage {
	return &models.PromoPackage{
		PackageID:  packageID,
		OwnerID:    owner,
		ArticleID:  "https://example.test/articles/hello",
		PostText:   "Read our launch article",
		Visibility: models.PromoPackageVisibilityPublic,
		Assets: []models.PromoPackageAsset{
			{MediaID: "m1", ContentHash: inmemPromoDigest("aa"), PublishedURL: "https://cdn.example/published/m1.png"},
		},
		ContentHash: inmemPromoDigest("bb"),
		Status:      models.PromoPackageStatusDraft,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
}

func TestInMemoryPromoPackageRepositoryLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := NewPromoPackageRepository()

	// Create + get + owner-scoped reads.
	require.ErrorIs(t, repo.CreatePromoPackage(ctx, &models.PromoPackage{PackageID: "p"}),
		storage.ErrInvalidInput, "missing owner fails closed")
	pkg := inmemPromoFixture("pkg-1", "alice")
	require.NoError(t, repo.CreatePromoPackage(ctx, pkg))
	require.ErrorIs(t, repo.CreatePromoPackage(ctx, inmemPromoFixture("pkg-1", "alice")), storage.ErrAlreadyExists)

	got, err := repo.GetPromoPackage(ctx, "alice", "pkg-1")
	require.NoError(t, err)
	require.Equal(t, "pkg-1", got.PackageID)
	_, err = repo.GetPromoPackage(ctx, "mallory", "pkg-1")
	require.ErrorIs(t, err, storage.ErrNotFound)

	// CAS content update: bump + stale conflict.
	updated := inmemPromoFixture("pkg-1", "alice")
	updated.ContentHash = inmemPromoDigest("cc")
	require.NoError(t, repo.UpdatePromoPackageContent(ctx, "alice", updated))
	require.Equal(t, 1, updated.ModelVersion)

	stale := inmemPromoFixture("pkg-1", "alice")
	err = repo.UpdatePromoPackageContent(ctx, "alice", stale)
	require.True(t, apperrors.HasCode(err, apperrors.CodeConflict), "stale content write conflicts: %v", err)
	require.ErrorIs(t, err, storage.ErrVersionConflict)

	// Cross-owner writes fail: a mismatched record is rejected up front, and a
	// matching record under another owner's keys cannot address alice's row.
	foreign := inmemPromoFixture("pkg-1", "mallory")
	require.ErrorIs(t, repo.UpdatePromoPackageContent(ctx, "alice", foreign), storage.ErrInvalidInput)
	require.ErrorIs(t, repo.UpdatePromoPackageContent(ctx, "mallory", foreign), storage.ErrNotFound)

	// Release stamp CAS + re-stamp conflict.
	now := time.Now().UTC()
	released := inmemPromoFixture("pkg-1", "alice")
	released.ModelVersion = 1
	released.Status = models.PromoPackageStatusReleased
	released.ReleasedStatusID = "status-1"
	released.ReleasedAt = &now
	require.NoError(t, repo.MarkPromoPackageReleased(ctx, "alice", released))

	staleRelease := inmemPromoFixture("pkg-1", "alice")
	staleRelease.ModelVersion = 1
	staleRelease.Status = models.PromoPackageStatusReleased
	staleRelease.ReleasedStatusID = "status-2"
	err = repo.MarkPromoPackageReleased(ctx, "alice", staleRelease)
	require.ErrorIs(t, err, storage.ErrVersionConflict)

	persisted, err := repo.GetPromoPackage(ctx, "alice", "pkg-1")
	require.NoError(t, err)
	require.True(t, persisted.IsReleased())
	require.Equal(t, "status-1", persisted.ReleasedStatusID)

	// Missing rows fail closed.
	require.ErrorIs(t, repo.UpdatePromoPackageContent(ctx, "alice", inmemPromoFixture("missing", "alice")), storage.ErrNotFound)
	require.ErrorIs(t, repo.MarkPromoPackageReleased(ctx, "alice", inmemPromoFixture("missing", "alice")), storage.ErrNotFound)

	// Owner list with cursor + past-end termination.
	second := inmemPromoFixture("pkg-2", "alice")
	require.NoError(t, repo.CreatePromoPackage(ctx, second))
	rows, next, err := repo.ListPromoPackages(ctx, "alice", 1, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotEmpty(t, next)
	rows, next, err = repo.ListPromoPackages(ctx, "alice", 10, next)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Empty(t, next)
	rows, next, err = repo.ListPromoPackages(ctx, "alice", 10, "zzz-past-end")
	require.NoError(t, err)
	require.Empty(t, rows)
	require.Empty(t, next)
}

func TestInMemoryPromoReviewGrantsAndVerdicts(t *testing.T) {
	ctx := context.Background()
	repo := NewPromoPackageRepository()
	now := time.Now().UTC()

	// Create grant + validation failures.
	require.ErrorIs(t, repo.CreatePromoReviewGrant(ctx, &models.PromoReviewGrant{OwnerID: "alice"}),
		storage.ErrInvalidInput)
	grant := &models.PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", GrantedAt: now}
	require.NoError(t, repo.CreatePromoReviewGrant(ctx, grant))
	require.ErrorIs(t, repo.CreatePromoReviewGrant(ctx, grant), storage.ErrAlreadyExists)

	got, err := repo.GetPromoReviewGrant(ctx, "alice", "pkg-1", "reviewer")
	require.NoError(t, err)
	require.Equal(t, "pkg-1", got.PackageID)
	_, err = repo.GetPromoReviewGrant(ctx, "alice", "pkg-1", "nobody")
	require.ErrorIs(t, err, storage.ErrNotFound)

	// Queue + owner listings.
	queue, _, err := repo.ListActivePromoReviewGrants(ctx, "reviewer", 10, "")
	require.NoError(t, err)
	require.Len(t, queue, 1)
	byOwner, err := repo.ListPromoReviewGrantsByOwner(ctx, "alice")
	require.NoError(t, err)
	require.Len(t, byOwner, 1)
	byPkg, err := repo.ListPromoReviewGrants(ctx, "alice", "pkg-1")
	require.NoError(t, err)
	require.Len(t, byPkg, 1)

	// Regrant (version CAS) + stale regrant conflict.
	expires := now.Add(time.Hour)
	regrant := &models.PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", GrantedAt: now, ExpiresAt: &expires, Version: got.Version}
	require.NoError(t, repo.RegrantPromoReviewGrant(ctx, regrant))
	staleRegrant := &models.PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", GrantedAt: now, ExpiresAt: &expires, Version: got.Version + 5}
	require.Error(t, repo.RegrantPromoReviewGrant(ctx, staleRegrant))
	require.Error(t, repo.RegrantPromoReviewGrant(ctx, &models.PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", GrantedAt: now, RevokedAt: &now}))

	// Revoke removes the queue entry; stale revoke conflicts.
	revokedAt := now.Add(time.Minute)
	revoke := &models.PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", GrantedAt: now, RevokedAt: &revokedAt, Version: regrant.Version}
	require.NoError(t, repo.RevokePromoReviewGrant(ctx, revoke))
	staleRevoke := &models.PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", GrantedAt: now, RevokedAt: &revokedAt, Version: 0}
	require.Error(t, repo.RevokePromoReviewGrant(ctx, staleRevoke))
	queue, _, err = repo.ListActivePromoReviewGrants(ctx, "reviewer", 10, "")
	require.NoError(t, err)
	require.Empty(t, queue)

	// Verdicts: create + ordered list + validation.
	require.ErrorIs(t, repo.CreatePromoReviewVerdict(ctx, &models.PromoReviewVerdict{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer"}),
		storage.ErrInvalidInput)
	verdict := &models.PromoReviewVerdict{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", Verdict: models.PromoPackageReviewApproved, ContentHash: inmemPromoDigest("cc"), RecordedAt: now}
	require.NoError(t, repo.CreatePromoReviewVerdict(ctx, verdict))
	verdicts, err := repo.ListPromoReviewVerdicts(ctx, "alice", "pkg-1")
	require.NoError(t, err)
	require.Len(t, verdicts, 1)
	require.Equal(t, models.PromoPackageReviewApproved, verdicts[0].Verdict)
}
