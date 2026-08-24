package repositories

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

type promoRecordingDynamo struct {
	*fakedb.Fake
	updateInputs []*dynamodb.UpdateItemInput
}

func (d *promoRecordingDynamo) UpdateItem(ctx context.Context, input *dynamodb.UpdateItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	d.updateInputs = append(d.updateInputs, input)
	return d.Fake.UpdateItem(ctx, input, opts...)
}

func newPromoRecordingDB(t *testing.T) (*PromoPackageRepository, *promoRecordingDynamo) {
	t.Helper()
	client := &promoRecordingDynamo{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.PromoPackage{}))
	require.NoError(t, db.CreateTable(&models.PromoReviewGrant{}))
	require.NoError(t, db.CreateTable(&models.PromoReviewVerdict{}))
	repo := NewPromoPackageRepository(db, "test-table", zap.NewNop(), nil)
	return repo, client
}

func promoFixture(packageID, owner string) *models.PromoPackage {
	return &models.PromoPackage{
		PackageID:  packageID,
		OwnerID:    owner,
		ArticleID:  "https://example.com/articles/hello",
		PostText:   "Read our launch article",
		Visibility: models.PromoPackageVisibilityPublic,
		Assets: []models.PromoPackageAsset{
			{MediaID: "media-1", ContentHash: "sha256:" + strings.Repeat("a", 64), PublishedURL: "https://cdn.example/published/a.png"},
		},
		ContentHash: "sha256:" + strings.Repeat("b", 64),
		Status:      models.PromoPackageStatusDraft,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
}

func TestPromoPackageRepositoryCreateGetListRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo, _ := newPromoRecordingDB(t)

	pkg := promoFixture("pkg-1", "alice")
	require.NoError(t, repo.CreatePromoPackage(ctx, pkg))

	got, err := repo.GetPromoPackage(ctx, "alice", "pkg-1")
	require.NoError(t, err)
	require.Equal(t, "pkg-1", got.PackageID)
	require.Equal(t, "alice", got.OwnerID)
	require.Equal(t, "USER#alice#PROMO#PACKAGE", got.PK)
	require.Equal(t, "PACKAGE#pkg-1", got.SK)
	require.Equal(t, "sha256:"+strings.Repeat("a", 64), got.Assets[0].ContentHash)

	// owner-scoped: another owner cannot see it
	_, err = repo.GetPromoPackage(ctx, "mallory", "pkg-1")
	require.ErrorIs(t, err, storage.ErrNotFound)

	rows, next, err := repo.ListPromoPackages(ctx, "alice", 10, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Empty(t, next)

	require.ErrorIs(t, repo.CreatePromoPackage(ctx, promoFixture("pkg-1", "alice")), storage.ErrAlreadyExists)
}

func TestPromoPackageRepositoryUpdateContentCASBumpsVersion(t *testing.T) {
	ctx := context.Background()
	repo, client := newPromoRecordingDB(t)

	pkg := promoFixture("pkg-1", "alice")
	require.NoError(t, repo.CreatePromoPackage(ctx, pkg))
	require.Equal(t, 0, pkg.ModelVersion, "fresh rows start at version 0")

	// Content update with the read version succeeds and bumps the version.
	updated := promoFixture("pkg-1", "alice")
	updated.ContentHash = "sha256:" + strings.Repeat("c", 64)
	require.NoError(t, repo.UpdatePromoPackageContent(ctx, "alice", updated))
	require.Equal(t, 1, updated.ModelVersion)

	persisted, err := repo.GetPromoPackage(ctx, "alice", "pkg-1")
	require.NoError(t, err)
	require.Equal(t, "sha256:"+strings.Repeat("c", 64), persisted.ContentHash)
	require.Equal(t, 1, persisted.ModelVersion)

	input := client.updateInputs[len(client.updateInputs)-1]
	condition := ""
	if input.ConditionExpression != nil {
		condition = *input.ConditionExpression
	}
	require.Contains(t, condition, "attribute_not_exists", "the CAS must tolerate pre-M4 rows via attribute_not_exists")
	require.Contains(t, condition, "OR", "the CAS must accept the read version as the alternative disjunct")
}

func TestPromoPackageRepositoryUpdateContentStaleVersionConflicts(t *testing.T) {
	ctx := context.Background()
	repo, _ := newPromoRecordingDB(t)

	pkg := promoFixture("pkg-1", "alice")
	require.NoError(t, repo.CreatePromoPackage(ctx, pkg))

	first := promoFixture("pkg-1", "alice")
	first.ContentHash = "sha256:" + strings.Repeat("c", 64)
	require.NoError(t, repo.UpdatePromoPackageContent(ctx, "alice", first)) // bumps to 1

	// A stale snapshot (version 0) must conflict, not silently lose the update.
	stale := promoFixture("pkg-1", "alice")
	stale.ContentHash = "sha256:" + strings.Repeat("d", 64)
	err := repo.UpdatePromoPackageContent(ctx, "alice", stale)
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeConflict), "conflict surfaces with the CONFLICT code: %v", err)
	require.ErrorIs(t, err, storage.ErrVersionConflict)

	persisted, err := repo.GetPromoPackage(ctx, "alice", "pkg-1")
	require.NoError(t, err)
	require.Equal(t, "sha256:"+strings.Repeat("c", 64), persisted.ContentHash, "the losing write must not land")
	require.Equal(t, 1, persisted.ModelVersion)
}

func TestPromoPackageRepositoryUpdateContentMigratesPreVersionRow(t *testing.T) {
	ctx := context.Background()
	repo, _ := newPromoRecordingDB(t)

	pkg := promoFixture("pkg-1", "alice")
	require.NoError(t, repo.CreatePromoPackage(ctx, pkg))

	// Simulate a pre-M4 row that never carried the version attribute: remove it
	// through a raw field-scoped builder (mirrors the storage-layer idiom).
	remove := repo.db.WithContext(ctx).
		Model(&models.PromoPackage{}).
		Where("PK", "=", "USER#alice#PROMO#PACKAGE").
		Where("SK", "=", "PACKAGE#pkg-1").
		UpdateBuilder()
	remove.Remove("ModelVersion")
	remove.ConditionExists("PK")
	require.NoError(t, remove.Execute())

	// The migration-safe CAS accepts the first write on a row without the
	// attribute and stamps version 1.
	migrated := promoFixture("pkg-1", "alice")
	migrated.ContentHash = "sha256:" + strings.Repeat("e", 64)
	require.NoError(t, repo.UpdatePromoPackageContent(ctx, "alice", migrated))
	require.Equal(t, 1, migrated.ModelVersion)

	persisted, err := repo.GetPromoPackage(ctx, "alice", "pkg-1")
	require.NoError(t, err)
	require.Equal(t, 1, persisted.ModelVersion)

	// Once versioned, stale writes fail closed.
	stale := promoFixture("pkg-1", "alice")
	err = repo.UpdatePromoPackageContent(ctx, "alice", stale)
	require.ErrorIs(t, err, storage.ErrVersionConflict)

	// A current write keeps working.
	next := promoFixture("pkg-1", "alice")
	next.ModelVersion = 1
	require.NoError(t, repo.UpdatePromoPackageContent(ctx, "alice", next))
}

func TestPromoPackageRepositoryMarkReleasedStampsAndBlocksStale(t *testing.T) {
	ctx := context.Background()
	repo, _ := newPromoRecordingDB(t)

	pkg := promoFixture("pkg-1", "alice")
	require.NoError(t, repo.CreatePromoPackage(ctx, pkg))

	now := time.Now().UTC()
	released := promoFixture("pkg-1", "alice")
	released.Status = models.PromoPackageStatusReleased
	released.ReleasedStatusID = "status-1"
	released.ReleasedAt = &now
	require.NoError(t, repo.MarkPromoPackageReleased(ctx, "alice", released))

	persisted, err := repo.GetPromoPackage(ctx, "alice", "pkg-1")
	require.NoError(t, err)
	require.Equal(t, models.PromoPackageStatusReleased, persisted.Status)
	require.Equal(t, "status-1", persisted.ReleasedStatusID)
	require.True(t, persisted.IsReleased())

	// A stale release stamp (same version as the first) must conflict.
	stale := promoFixture("pkg-1", "alice")
	stale.Status = models.PromoPackageStatusReleased
	stale.ReleasedStatusID = "status-2"
	err = repo.MarkPromoPackageReleased(ctx, "alice", stale)
	require.ErrorIs(t, err, storage.ErrVersionConflict)

	after, err := repo.GetPromoPackage(ctx, "alice", "pkg-1")
	require.NoError(t, err)
	require.Equal(t, "status-1", after.ReleasedStatusID, "the double-release stamp must not land")
}

func TestPromoPackageRepositoryReviewGrantLifecycle(t *testing.T) {
	ctx := context.Background()
	repo, _ := newPromoRecordingDB(t)

	now := time.Now().UTC()
	first := &models.PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", GrantedAt: now}
	require.NoError(t, repo.CreatePromoReviewGrant(ctx, first))
	require.Equal(t, "USER#alice#PROMO#REVIEW", first.PK)
	require.Equal(t, "GRANT#pkg-1#REVIEWER#reviewer", first.SK)
	require.Equal(t, "PROMO#REVIEWER#reviewer", first.GSI2PK)

	got, err := repo.GetPromoReviewGrant(ctx, "alice", "pkg-1", "reviewer")
	require.NoError(t, err)
	require.Equal(t, "pkg-1", got.PackageID)

	// reviewer queue
	rows, next, err := repo.ListActivePromoReviewGrants(ctx, "reviewer", 10, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Empty(t, next)

	// regrant refreshes expiry and version
	expiresAt := now.Add(time.Hour)
	regrant := &models.PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", GrantedAt: now, ExpiresAt: &expiresAt, Version: got.Version}
	require.NoError(t, repo.RegrantPromoReviewGrant(ctx, regrant))

	active, err := repo.GetPromoReviewGrant(ctx, "alice", "pkg-1", "reviewer")
	require.NoError(t, err)
	require.True(t, active.IsActive(time.Now().UTC()))

	// revoke removes the queue keys and the grant from the reviewer page
	revokedAt := now.Add(time.Minute)
	revoke := &models.PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", GrantedAt: now, RevokedAt: &revokedAt, Version: active.Version}
	require.NoError(t, repo.RevokePromoReviewGrant(ctx, revoke))
	rows, _, err = repo.ListActivePromoReviewGrants(ctx, "reviewer", 10, "")
	require.NoError(t, err)
	require.Empty(t, rows)

	// regrant after revoke must fail (the service re-shares instead)
	staleRevoked := &models.PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", GrantedAt: now}
	require.Error(t, repo.RegrantPromoReviewGrant(ctx, staleRevoked), "a revoked grant cannot be regranted; re-share creates a fresh grant")

	// owner listing
	byOwner, err := repo.ListPromoReviewGrantsByOwner(ctx, "alice")
	require.NoError(t, err)
	require.Len(t, byOwner, 1)
	require.NotNil(t, byOwner[0].RevokedAt)
}

func TestPromoPackageRepositoryVerdictLifecycle(t *testing.T) {
	ctx := context.Background()
	repo, _ := newPromoRecordingDB(t)

	recorded := time.Now().UTC()
	first := &models.PromoReviewVerdict{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", Verdict: models.PromoPackageReviewApproved, ContentHash: "sha256:" + strings.Repeat("b", 64), RecordedAt: recorded}
	require.NoError(t, repo.CreatePromoReviewVerdict(ctx, first))
	require.Contains(t, first.SK, "VERDICT#pkg-1#TIME#")

	verdicts, err := repo.ListPromoReviewVerdicts(ctx, "alice", "pkg-1")
	require.NoError(t, err)
	require.Len(t, verdicts, 1)
	require.Equal(t, models.PromoPackageReviewApproved, verdicts[0].Verdict)
	require.Equal(t, "sha256:"+strings.Repeat("b", 64), verdicts[0].ContentHash)

	// scope: another package's verdicts are invisible
	other, err := repo.ListPromoReviewVerdicts(ctx, "alice", "pkg-2")
	require.NoError(t, err)
	require.Empty(t, other)
}

func TestPromoPackageRepositoryWriteOwnerValidation(t *testing.T) {
	ctx := context.Background()
	repo, _ := newPromoRecordingDB(t)

	pkg := promoFixture("pkg-1", "alice")
	require.NoError(t, repo.CreatePromoPackage(ctx, pkg))

	// A record that claims a different owner than the caller is rejected up
	// front by the owner validation.
	notOwner := promoFixture("pkg-1", "mallory")
	err := repo.UpdatePromoPackageContent(ctx, "alice", notOwner)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match promo package owner")

	err = repo.MarkPromoPackageReleased(ctx, "alice", notOwner)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match promo package owner")

	// A caller writing under their own identity cannot address alice's package:
	// the owner-scoped keys do not exist for them, so the write fails closed
	// with a conflict rather than touching alice's row.
	mallory := promoFixture("pkg-1", "mallory")
	err = repo.UpdatePromoPackageContent(ctx, "mallory", mallory)
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeConflict), "cross-owner address resolves to a conflict: %v", err)

	persisted, err := repo.GetPromoPackage(ctx, "alice", "pkg-1")
	require.NoError(t, err)
	require.Equal(t, "sha256:"+strings.Repeat("b", 64), persisted.ContentHash, "alice's package must be untouched")
}

func TestPromoPackageRepositoryMissingPackageNotFound(t *testing.T) {
	ctx := context.Background()
	repo, _ := newPromoRecordingDB(t)

	_, err := repo.GetPromoPackage(ctx, "alice", "missing")
	require.ErrorIs(t, err, storage.ErrNotFound)

	// A content write against a missing row is indistinguishable from a stale
	// version at the storage layer and surfaces as a conflict signal.
	err = repo.UpdatePromoPackageContent(ctx, "alice", promoFixture("missing", "alice"))
	require.True(t, apperrors.HasCode(err, apperrors.CodeConflict), "missing-row CAS write surfaces a conflict: %v", err)

	_, err = repo.GetPromoReviewGrant(ctx, "alice", "missing", "reviewer")
	require.ErrorIs(t, err, storage.ErrNotFound)
}

func TestPromoPackageRepositoryListGrantsByPackageAndVerdictErrors(t *testing.T) {
	ctx := context.Background()
	repo, _ := newPromoRecordingDB(t)

	now := time.Now().UTC()
	grant := &models.PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", GrantedAt: now}
	require.NoError(t, repo.CreatePromoReviewGrant(ctx, grant))
	other := &models.PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-2", Reviewer: "reviewer", GrantedAt: now}
	require.NoError(t, repo.CreatePromoReviewGrant(ctx, other))

	// Package-scoped grant listing excludes other packages.
	grants, err := repo.ListPromoReviewGrants(ctx, "alice", "pkg-1")
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.Equal(t, "pkg-1", grants[0].PackageID)

	// Duplicate grant creation conflicts.
	err = repo.CreatePromoReviewGrant(ctx, grant)
	require.True(t, apperrors.HasCode(err, apperrors.CodeConflict))

	// Missing required fields fail closed at key preparation.
	err = repo.CreatePromoReviewGrant(ctx, &models.PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-3"})
	require.Error(t, err)
	err = repo.CreatePromoReviewVerdict(ctx, &models.PromoReviewVerdict{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer"})
	require.Error(t, err)

	// Revoking a stale grant version conflicts.
	stored, err := repo.GetPromoReviewGrant(ctx, "alice", "pkg-1", "reviewer")
	require.NoError(t, err)
	revokedAt := now.Add(time.Minute)
	stale := &models.PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", GrantedAt: now, RevokedAt: &revokedAt, Version: stored.Version + 7}
	err = repo.RevokePromoReviewGrant(ctx, stale)
	require.Error(t, err)

	// A current revoke succeeds and the package-scoped list still returns the row.
	current := &models.PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", GrantedAt: now, RevokedAt: &revokedAt, Version: stored.Version}
	require.NoError(t, repo.RevokePromoReviewGrant(ctx, current))
	grants, err = repo.ListPromoReviewGrants(ctx, "alice", "pkg-1")
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.NotNil(t, grants[0].RevokedAt)

	// Regrant of a revoked grant is refused (re-share creates a fresh grant).
	err = repo.RegrantPromoReviewGrant(ctx, &models.PromoReviewGrant{OwnerID: "alice", PackageID: "pkg-1", Reviewer: "reviewer", GrantedAt: now, RevokedAt: &revokedAt})
	require.Error(t, err)
}
