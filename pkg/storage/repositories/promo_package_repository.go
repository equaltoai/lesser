package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

// PromoPackageRepository implements promo package persistence: the reviewed
// package record plus its review gate (grants and verdicts). Content and
// release writes are field-scoped and version-conditioned so the M4 compose and
// release lanes cannot silently lose a concurrent update.
type PromoPackageRepository struct {
	*EnhancedBaseRepository[*models.PromoPackage]
}

// NewPromoPackageRepository creates a new promo package repository.
func NewPromoPackageRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *PromoPackageRepository {
	enhancedRepo := NewEnhancedBaseRepository[*models.PromoPackage](db, tableName, logger, costService, "PromoPackageRepository", "promo package")

	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService())
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &PromoPackageRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

func validatePromoPackageOwner(ownerID string, pkg *models.PromoPackage) error {
	ownerID = strings.TrimSpace(ownerID)
	if err := common.ValidateRequiredParam("ownerID", ownerID); err != nil {
		return err
	}
	if pkg == nil {
		return common.ValidationError{Field: "promo package", Message: "is required"}
	}
	pkgOwner := strings.TrimSpace(pkg.OwnerID)
	if err := common.ValidateRequiredParam("promo package ownerID", pkgOwner); err != nil {
		return err
	}
	if pkgOwner != ownerID {
		return common.ValidationError{Field: "ownerID", Message: "does not match promo package owner"}
	}
	pkg.OwnerID = pkgOwner
	return nil
}

// promoPackageKeys derives the primary keys for a package without mutating the
// caller's record beyond the keys.
func promoPackageKeys(ownerID, packageID string) (string, string) {
	return fmt.Sprintf("USER#%s#PROMO#PACKAGE", strings.TrimSpace(ownerID)),
		fmt.Sprintf("PACKAGE#%s", strings.TrimSpace(packageID))
}

// CreatePromoPackage creates a first-time promo package.
func (r *PromoPackageRepository) CreatePromoPackage(ctx context.Context, pkg *models.PromoPackage) error {
	if pkg == nil {
		return common.ValidationError{Field: "promo package", Message: "is required"}
	}
	if err := validatePromoPackageOwner(pkg.OwnerID, pkg); err != nil {
		return err
	}
	if err := pkg.UpdateKeys(); err != nil {
		return ErrorHandler.HandleCreateError(err, "promo package", pkg.SK)
	}
	return r.ValidateAndCreate(ctx, pkg)
}

// GetPromoPackage loads a package by owner and package ID.
func (r *PromoPackageRepository) GetPromoPackage(ctx context.Context, ownerID, packageID string) (*models.PromoPackage, error) {
	var pkg models.PromoPackage
	pk, sk := promoPackageKeys(ownerID, packageID)
	if err := r.Get(ctx, pk, sk, &pkg); err != nil {
		return nil, err
	}
	return &pkg, nil
}

// UpdatePromoPackageContent is the field-scoped CAS writer for the reviewed
// package content. It replaces only post text, visibility, the article
// reference, the ordered asset set, the derived content hash, and the update
// timestamp. The write is version-conditioned: the model version captured at
// read time must still hold (or the row may be a pre-M4 record that never
// carried the attribute — the migration-safe first write stamps version 1), and
// the winner bumps the version. A concurrent compose loser receives a conflict
// error instead of silently losing its update.
func (r *PromoPackageRepository) UpdatePromoPackageContent(ctx context.Context, ownerID string, pkg *models.PromoPackage) error {
	if err := validatePromoPackageOwner(ownerID, pkg); err != nil {
		return err
	}
	if err := pkg.UpdateKeys(); err != nil {
		return err
	}

	nextVersion := pkg.ModelVersion + 1
	if nextVersion <= 0 {
		nextVersion = 1
	}
	builder := r.db.WithContext(ctx).
		Model(pkg).
		Where("PK", "=", pkg.PK).
		Where("SK", "=", pkg.SK).
		UpdateBuilder()
	builder.Set("PostText", pkg.PostText)
	builder.Set("Visibility", pkg.Visibility)
	builder.Set("ArticleID", pkg.ArticleID)
	if len(pkg.Assets) == 0 {
		builder.Remove("Assets")
	} else {
		builder.Set("Assets", pkg.Assets)
	}
	builder.Set("ContentHash", pkg.ContentHash)
	builder.Set("UpdatedAt", pkg.UpdatedAt.UTC())
	// Migration-safe CAS: pre-M4 rows never carried the version attribute, so
	// the first write after deploy accepts them (attribute_not_exists) while a
	// versioned row must still match the read version — a concurrent compose
	// loser fails closed instead of silently losing its update. ConditionExists
	// on the primary key is the leading conjunct: without it, a missing row
	// would satisfy the attribute_not_exists disjunct and the UpdateItem would
	// upsert a new package instead of surfacing a conflict. ExecuteWithResult
	// is required here: Execute() compiles every condition as AND and would
	// silently drop the OR disjunct.
	builder.ConditionExists("PK")
	builder.ConditionNotExists("ModelVersion")
	builder.OrCondition("ModelVersion", "=", pkg.ModelVersion)
	builder.Set("ModelVersion", nextVersion)
	var updated models.PromoPackage
	if err := builder.ExecuteWithResult(&updated); err != nil {
		if dynamormerrors.IsConditionFailed(err) {
			return apperrors.DynamoDBConditionalCheckFailed("promo package " + pkg.PackageID).
				WithInternalError(errors.Join(err, storage.ErrVersionConflict))
		}
		return ErrorHandler.HandleUpdateError(err, "promo package", pkg.PackageID)
	}
	pkg.ModelVersion = nextVersion
	return nil
}

// MarkPromoPackageReleasing reserves the release transition before any outbound
// Status is created: the status moves draft -> releasing through the same
// version-conditioned field-scoped lane, and exactly one concurrent releaser
// wins the reservation (every loser conflicts BEFORE a post exists). The
// release then creates the post and finalizes to released; on post-creation
// failure the winner rolls back releasing -> draft with RevertPromoPackageReleasing.
func (r *PromoPackageRepository) MarkPromoPackageReleasing(ctx context.Context, ownerID string, pkg *models.PromoPackage) error {
	if err := validatePromoPackageOwner(ownerID, pkg); err != nil {
		return err
	}
	if err := pkg.UpdateKeys(); err != nil {
		return err
	}

	nextVersion := pkg.ModelVersion + 1
	if nextVersion <= 0 {
		nextVersion = 1
	}
	builder := r.db.WithContext(ctx).
		Model(pkg).
		Where("PK", "=", pkg.PK).
		Where("SK", "=", pkg.SK).
		UpdateBuilder()
	builder.Set("Status", pkg.Status)
	builder.Set("UpdatedAt", pkg.UpdatedAt.UTC())
	// Same migration-safe CAS and ExecuteWithResult requirement as the content
	// writer (see UpdatePromoPackageContent).
	builder.ConditionExists("PK")
	builder.ConditionNotExists("ModelVersion")
	builder.OrCondition("ModelVersion", "=", pkg.ModelVersion)
	builder.Set("ModelVersion", nextVersion)
	var updated models.PromoPackage
	if err := builder.ExecuteWithResult(&updated); err != nil {
		if dynamormerrors.IsConditionFailed(err) {
			return apperrors.DynamoDBConditionalCheckFailed("promo package " + pkg.PackageID).
				WithInternalError(errors.Join(err, storage.ErrVersionConflict))
		}
		return ErrorHandler.HandleUpdateError(err, "promo package", pkg.PackageID)
	}
	pkg.ModelVersion = nextVersion
	return nil
}

// RevertPromoPackageReleasing rolls a reserved release back to draft through
// the same version-conditioned lane. It writes only the status and the update
// timestamp (never the content or a released stamp), so only the reservation
// winner (who alone holds the post-reservation version) can roll back; a
// concurrent content write cannot be clobbered by the rollback.
func (r *PromoPackageRepository) RevertPromoPackageReleasing(ctx context.Context, ownerID string, pkg *models.PromoPackage) error {
	if err := validatePromoPackageOwner(ownerID, pkg); err != nil {
		return err
	}
	if err := pkg.UpdateKeys(); err != nil {
		return err
	}

	nextVersion := pkg.ModelVersion + 1
	if nextVersion <= 0 {
		nextVersion = 1
	}
	builder := r.db.WithContext(ctx).
		Model(pkg).
		Where("PK", "=", pkg.PK).
		Where("SK", "=", pkg.SK).
		UpdateBuilder()
	builder.Set("Status", pkg.Status)
	builder.Set("UpdatedAt", pkg.UpdatedAt.UTC())
	// Same migration-safe CAS and ExecuteWithResult requirement as the content
	// writer (see UpdatePromoPackageContent).
	builder.ConditionExists("PK")
	builder.ConditionNotExists("ModelVersion")
	builder.OrCondition("ModelVersion", "=", pkg.ModelVersion)
	builder.Set("ModelVersion", nextVersion)
	var updated models.PromoPackage
	if err := builder.ExecuteWithResult(&updated); err != nil {
		if dynamormerrors.IsConditionFailed(err) {
			return apperrors.DynamoDBConditionalCheckFailed("promo package " + pkg.PackageID).
				WithInternalError(errors.Join(err, storage.ErrVersionConflict))
		}
		return ErrorHandler.HandleUpdateError(err, "promo package", pkg.PackageID)
	}
	pkg.ModelVersion = nextVersion
	return nil
}

// MarkPromoPackageReleased stamps the outbound Status created by the release
// transition via a version-conditioned field-scoped write. The stamp blocks
// re-release; the CAS closes the double-release race.
func (r *PromoPackageRepository) MarkPromoPackageReleased(ctx context.Context, ownerID string, pkg *models.PromoPackage) error {
	if err := validatePromoPackageOwner(ownerID, pkg); err != nil {
		return err
	}
	if err := pkg.UpdateKeys(); err != nil {
		return err
	}

	nextVersion := pkg.ModelVersion + 1
	if nextVersion <= 0 {
		nextVersion = 1
	}
	builder := r.db.WithContext(ctx).
		Model(pkg).
		Where("PK", "=", pkg.PK).
		Where("SK", "=", pkg.SK).
		UpdateBuilder()
	builder.Set("Status", pkg.Status)
	builder.Set("ReleasedStatusID", pkg.ReleasedStatusID)
	if pkg.ReleasedAt != nil {
		builder.Set("ReleasedAt", pkg.ReleasedAt.UTC())
	} else {
		builder.Remove("ReleasedAt")
	}
	builder.Set("UpdatedAt", pkg.UpdatedAt.UTC())
	// Same migration-safe CAS and ExecuteWithResult requirement as the content
	// writer (see UpdatePromoPackageContent). The key-exists condition leads so
	// a missing row fails closed instead of being upserted by the
	// attribute_not_exists disjunct.
	builder.ConditionExists("PK")
	builder.ConditionNotExists("ModelVersion")
	builder.OrCondition("ModelVersion", "=", pkg.ModelVersion)
	builder.Set("ModelVersion", nextVersion)
	var updated models.PromoPackage
	if err := builder.ExecuteWithResult(&updated); err != nil {
		if dynamormerrors.IsConditionFailed(err) {
			return apperrors.DynamoDBConditionalCheckFailed("promo package " + pkg.PackageID).
				WithInternalError(errors.Join(err, storage.ErrVersionConflict))
		}
		return ErrorHandler.HandleUpdateError(err, "promo package", pkg.PackageID)
	}
	pkg.ModelVersion = nextVersion
	return nil
}

// ListPromoPackages lists one owner's packages, paginated by SK cursors.
func (r *PromoPackageRepository) ListPromoPackages(ctx context.Context, ownerID string, limit int, cursor string) ([]*models.PromoPackage, string, error) {
	ownerID = strings.TrimSpace(ownerID)
	if err := common.ValidateRequiredParam("ownerID", ownerID); err != nil {
		return nil, "", err
	}
	pk := fmt.Sprintf("USER#%s#PROMO#PACKAGE", ownerID)
	return listByPKSKPrefixPaginated[*models.PromoPackage](ctx, r.db, &models.PromoPackage{}, pk, "PACKAGE#", limit, strings.TrimSpace(cursor))
}

// CreatePromoReviewGrant creates a first-time review grant.
func (r *PromoPackageRepository) CreatePromoReviewGrant(ctx context.Context, grant *models.PromoReviewGrant) error {
	if err := grant.UpdateKeys(); err != nil {
		return ErrorHandler.HandleCreateError(err, "promo review grant", grant.SK)
	}
	err := r.db.WithContext(ctx).Model(grant).IfNotExists().Create()
	if dynamormerrors.IsConditionFailed(err) {
		return apperrors.DynamoDBConditionalCheckFailed("").WithInternalError(err)
	}
	return ErrorHandler.HandleCreateError(err, "promo review grant", grant.SK)
}

// RegrantPromoReviewGrant clears revocation and refreshes the sparse queue keys
// and expiry, version-conditioned.
func (r *PromoPackageRepository) RegrantPromoReviewGrant(ctx context.Context, grant *models.PromoReviewGrant) error {
	if grant == nil || grant.RevokedAt != nil {
		return fmt.Errorf("active promo review grant is required")
	}
	if err := grant.UpdateKeys(); err != nil {
		return ErrorHandler.HandleCreateError(err, "promo review grant", grant.SK)
	}

	nextVersion := grant.Version + 1
	if nextVersion <= 0 {
		nextVersion = 1
	}
	builder := r.db.WithContext(ctx).
		Model(grant).
		Where("PK", "=", grant.PK).
		Where("SK", "=", grant.SK).
		UpdateBuilder()
	builder.Set("GrantedAt", grant.GrantedAt.UTC())
	if grant.ExpiresAt != nil {
		expiresAt := grant.ExpiresAt.UTC()
		builder.Set("ExpiresAt", expiresAt)
	} else {
		builder.Remove("ExpiresAt")
	}
	builder.Set("GSI2PK", grant.GSI2PK)
	builder.Set("GSI2SK", grant.GSI2SK)
	builder.Remove("RevokedAt")
	builder.ConditionVersion(int64(grant.Version))
	builder.Set("Version", nextVersion)
	if err := builder.Execute(); err != nil {
		if dynamormerrors.IsConditionFailed(err) {
			return apperrors.DynamoDBConditionalCheckFailed("").WithInternalError(err)
		}
		return ErrorHandler.HandleCreateError(err, "promo review grant", grant.SK)
	}
	grant.Version = nextVersion
	return nil
}

// RevokePromoReviewGrant persists revocation and removes the sparse queue keys,
// version-conditioned.
func (r *PromoPackageRepository) RevokePromoReviewGrant(ctx context.Context, grant *models.PromoReviewGrant) error {
	if grant == nil || grant.RevokedAt == nil {
		return fmt.Errorf("revoked promo review grant is required")
	}
	if err := grant.UpdateKeys(); err != nil {
		return err
	}

	nextVersion := grant.Version + 1
	if nextVersion <= 0 {
		nextVersion = 1
	}
	builder := r.db.WithContext(ctx).
		Model(grant).
		Where("PK", "=", grant.PK).
		Where("SK", "=", grant.SK).
		UpdateBuilder()
	builder.Set("RevokedAt", grant.RevokedAt.UTC())
	builder.Remove("GSI2PK")
	builder.Remove("GSI2SK")
	builder.ConditionVersion(int64(grant.Version))
	builder.Set("Version", nextVersion)
	if err := builder.Execute(); err != nil {
		return err
	}
	grant.Version = nextVersion
	return nil
}

// GetPromoReviewGrant loads one grant by owner, package, and reviewer.
func (r *PromoPackageRepository) GetPromoReviewGrant(ctx context.Context, ownerID, packageID, reviewer string) (*models.PromoReviewGrant, error) {
	var grant models.PromoReviewGrant
	err := r.db.WithContext(ctx).Model(&models.PromoReviewGrant{}).
		Where("PK", "=", fmt.Sprintf("USER#%s#PROMO#REVIEW", ownerID)).
		Where("SK", "=", fmt.Sprintf("GRANT#%s#REVIEWER#%s", packageID, reviewer)).
		First(&grant)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return &grant, nil
}

// ListActivePromoReviewGrants returns one page from the sparse reviewer queue.
//
//nolint:dupl // the promo reviewer queue mirrors the draft-review reviewer queue (M4 issue #1446)
//nolint:dupl // the promo reviewer queue mirrors the draft-review reviewer queue (M4 issue #1446)
func (r *PromoPackageRepository) ListActivePromoReviewGrants(ctx context.Context, reviewer string, limit int, cursor string) ([]*models.PromoReviewGrant, string, error) {
	if limit <= 0 {
		limit = 25
	}
	query := r.db.WithContext(ctx).
		Model(&models.PromoReviewGrant{}).
		Index("gsi2").
		Where("gsi2PK", "=", fmt.Sprintf("PROMO#REVIEWER#%s", reviewer)).
		Filter("RevokedAt", "attribute_not_exists", nil).
		OrderBy("gsi2SK", "DESC")
	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		query = query.Where("gsi2SK", "<", cursor)
	}

	var rows []models.PromoReviewGrant
	err := query.Limit(limit + 1).All(&rows)
	if err != nil {
		return nil, "", err
	}
	nextCursor := ""
	if len(rows) > limit {
		nextCursor = rows[limit-1].GSI2SK
		rows = rows[:limit]
	}
	out := make([]*models.PromoReviewGrant, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nextCursor, nil
}

// ListPromoReviewGrants returns all grant records for one package.
func (r *PromoPackageRepository) ListPromoReviewGrants(ctx context.Context, ownerID, packageID string) ([]*models.PromoReviewGrant, error) {
	var rows []models.PromoReviewGrant
	err := r.db.WithContext(ctx).Model(&models.PromoReviewGrant{}).
		Where("PK", "=", fmt.Sprintf("USER#%s#PROMO#REVIEW", ownerID)).
		Where("SK", "begins_with", fmt.Sprintf("GRANT#%s#", packageID)).
		OrderBy("SK", "ASC").All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]*models.PromoReviewGrant, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nil
}

// ListPromoReviewGrantsByOwner returns every review assignment created by one
// package owner. Callers apply active-state filtering before pagination so
// revoked grants cannot shrink pages.
func (r *PromoPackageRepository) ListPromoReviewGrantsByOwner(ctx context.Context, ownerID string) ([]*models.PromoReviewGrant, error) {
	ownerID = strings.TrimSpace(ownerID)
	if err := common.ValidateRequiredParam("ownerID", ownerID); err != nil {
		return nil, err
	}
	var rows []models.PromoReviewGrant
	err := r.db.WithContext(ctx).Model(&models.PromoReviewGrant{}).
		Where("PK", "=", fmt.Sprintf("USER#%s#PROMO#REVIEW", ownerID)).
		Where("SK", "begins_with", "GRANT#").
		OrderBy("SK", "ASC").
		All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]*models.PromoReviewGrant, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nil
}

// CreatePromoReviewVerdict records an immutable, hash-bound verdict.
func (r *PromoPackageRepository) CreatePromoReviewVerdict(ctx context.Context, verdict *models.PromoReviewVerdict) error {
	if err := verdict.UpdateKeys(); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(verdict).Create()
}

// ListPromoReviewVerdicts returns ordered verdict history for one package.
func (r *PromoPackageRepository) ListPromoReviewVerdicts(ctx context.Context, ownerID, packageID string) ([]*models.PromoReviewVerdict, error) {
	var rows []models.PromoReviewVerdict
	err := r.db.WithContext(ctx).Model(&models.PromoReviewVerdict{}).
		Where("PK", "=", fmt.Sprintf("USER#%s#PROMO#REVIEW", ownerID)).
		Where("SK", "begins_with", fmt.Sprintf("VERDICT#%s#", packageID)).
		OrderBy("SK", "ASC").All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]*models.PromoReviewVerdict, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nil
}
