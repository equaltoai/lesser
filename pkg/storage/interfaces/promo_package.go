// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// PromoPackageRepository defines the persistence surface for the M4 promotional
// package: the reviewed package record (compose/content/release transitions)
// plus its review gate (grants and hash-bound verdicts), mirroring the M2
// draft-review repository shape.
type PromoPackageRepository interface {
	// ===== Package records =====

	// CreatePromoPackage creates a first-time promo package.
	CreatePromoPackage(ctx context.Context, pkg *models.PromoPackage) error

	// GetPromoPackage loads a package by owner and package ID.
	GetPromoPackage(ctx context.Context, ownerID, packageID string) (*models.PromoPackage, error)

	// UpdatePromoPackageContent is the field-scoped CAS writer for package
	// content (post text, visibility, article reference, ordered assets, and the
	// content hash derived from them). It is the only writer of the reviewed
	// content, so a concurrent compose cannot lose an update silently: the write
	// is version-conditioned and surfaces a conflict signal to the loser.
	UpdatePromoPackageContent(ctx context.Context, ownerID string, pkg *models.PromoPackage) error

	// MarkPromoPackageReleased stamps the outbound Status created by the release
	// transition (status, releasedStatusID, releasedAt) via a version-conditioned
	// field-scoped write; the stamp blocks re-release.
	MarkPromoPackageReleased(ctx context.Context, ownerID string, pkg *models.PromoPackage) error

	// MarkPromoPackageReleasing reserves the release transition with a
	// version-conditioned field-scoped write (status -> releasing, no outbound
	// Status yet). Exactly one concurrent releaser wins the reservation; every
	// other releaser receives a conflict BEFORE any post exists.
	MarkPromoPackageReleasing(ctx context.Context, ownerID string, pkg *models.PromoPackage) error

	// RevertPromoPackageReleasing rolls a reserved release back to draft
	// (status -> draft) via the same version-conditioned lane, used when the
	// outbound Status creation failed before any post existed.
	RevertPromoPackageReleasing(ctx context.Context, ownerID string, pkg *models.PromoPackage) error

	// ListPromoPackages lists one owner's packages, paginated by SK cursors.
	ListPromoPackages(ctx context.Context, ownerID string, limit int, cursor string) ([]*models.PromoPackage, string, error)

	// ===== Review grants =====

	// CreatePromoReviewGrant creates a first-time review grant.
	CreatePromoReviewGrant(ctx context.Context, grant *models.PromoReviewGrant) error
	// RegrantPromoReviewGrant clears revocation and refreshes the expiry.
	RegrantPromoReviewGrant(ctx context.Context, grant *models.PromoReviewGrant) error
	// RevokePromoReviewGrant persists revocation and removes the queue keys.
	RevokePromoReviewGrant(ctx context.Context, grant *models.PromoReviewGrant) error
	// GetPromoReviewGrant loads one grant by owner, package, and reviewer.
	GetPromoReviewGrant(ctx context.Context, ownerID, packageID, reviewer string) (*models.PromoReviewGrant, error)
	// ListActivePromoReviewGrants pages the sparse reviewer queue (GSI2).
	ListActivePromoReviewGrants(ctx context.Context, reviewer string, limit int, cursor string) ([]*models.PromoReviewGrant, string, error)
	// ListPromoReviewGrants returns every grant for one package.
	ListPromoReviewGrants(ctx context.Context, ownerID, packageID string) ([]*models.PromoReviewGrant, error)
	// ListPromoReviewGrantsByOwner returns every grant created by one owner.
	ListPromoReviewGrantsByOwner(ctx context.Context, ownerID string) ([]*models.PromoReviewGrant, error)

	// ===== Verdicts =====

	// CreatePromoReviewVerdict records an immutable, hash-bound verdict.
	CreatePromoReviewVerdict(ctx context.Context, verdict *models.PromoReviewVerdict) error
	// ListPromoReviewVerdicts returns ordered verdict history for one package.
	ListPromoReviewVerdicts(ctx context.Context, ownerID, packageID string) ([]*models.PromoReviewVerdict, error)
}
