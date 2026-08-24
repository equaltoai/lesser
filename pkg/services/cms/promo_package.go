package cms

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Promo package blocking reasons exposed through the review state surface,
// mirroring the draft-review vocabulary. The ASSET_* reasons name exactly why
// the exact approved bytes cannot be released.
const (
	PromoPackageReviewReasonApprovalRequired  = "REVIEW_APPROVAL_REQUIRED"
	PromoPackageReviewReasonPrincipalRequired = "PRINCIPAL_APPROVAL_REQUIRED"
	PromoPackageReviewReasonPrincipalMissing  = "PRINCIPAL_APPROVAL_UNAVAILABLE"
	PromoPackageReviewReasonAssetMissing      = "ASSET_MISSING"
	PromoPackageReviewReasonAssetNotOwned     = "ASSET_NOT_OWNED"
	PromoPackageReviewReasonAssetNotPublished = "ASSET_NOT_PUBLISHED"
	PromoPackageReviewReasonAssetDigestChange = "ASSET_DIGEST_CHANGED"
	PromoPackageReviewReasonReleased          = "PACKAGE_RELEASED"
	PromoPackageReviewReasonReleasing         = "PACKAGE_RELEASING"

	// maxPromoPostTextBytes mirrors the notes service content limit so a
	// reviewed package can always be released as composed.
	maxPromoPostTextBytes = 5000

	// maxPromoPackageReadGrants bounds the reviewer queue read like the draft
	// review surface.
	maxPromoPackageReadGrants = 200
)

var (
	// ErrPromoPackageApprovalRequired means a required reviewer is missing a
	// current approval for the exact reviewed package content.
	ErrPromoPackageApprovalRequired = errors.New("promo package requires approval from every required reviewer")
	// ErrPromoPackagePrincipalApprovalRequired means the release gate's operator
	// doctrine floor is unmet: the releasing actor is not the instance
	// principal, and the principal does not hold a current approving verdict
	// (regardless of asset provenance).
	ErrPromoPackagePrincipalApprovalRequired = errors.New("promo package release requires an active approval from the instance principal")
	// ErrPromoPackageAssetUnavailable means a bound asset cannot serve the exact
	// approved bytes (missing, not owned, not in the PUBLISHED durable state, or
	// its digest changed after review). The wrapped message names the reasons.
	ErrPromoPackageAssetUnavailable = errors.New("promo package asset cannot serve the exact approved bytes")
	// ErrPromoPackageAlreadyReleased means the release transition already
	// stamped an outbound Status; re-release and post-release composition are
	// refused.
	ErrPromoPackageAlreadyReleased = errors.New("promo package is already released")
	// ErrPromoPackageConflict is the additive conflict signal surfaced when the
	// version-conditioned content or release write loses a concurrent update;
	// the caller re-reads and retries.
	ErrPromoPackageConflict = errors.New("promo package changed concurrently")
	// ErrPromoPackageReviewContentChanged is the additive conflict signal
	// surfaced when a review submit carries an expected content hash that no
	// longer matches the stored package: the owner recomposed after the reviewer
	// inspected the package, so the verdict must not bless unseen content.
	ErrPromoPackageReviewContentChanged = errors.New("promo package content changed since the reviewer inspected it")
	// ErrPromoPackageReleaseInProgress means the package holds the transient
	// releasing reservation (a previous release reserved it but never stamped
	// an outbound Status, or is mid-flight). Release and composition are
	// refused until an operator reconciles the reservation; the reservation
	// guarantees the loser of a concurrent release never creates a post.
	ErrPromoPackageReleaseInProgress = errors.New("promo package release is already in progress")

	errPromoReviewStorageUnavailable = errors.New("promo package review storage is not available")
)

// PromoPackageComposeInput carries the full reviewed content of a promo
// package. The same shape composes a new package (PackageID empty) or replaces
// the content of an existing one (PackageID set); every content change
// re-hashes and stales prior approvals.
type PromoPackageComposeInput struct {
	PackageID     string
	ArticleID     string
	PostText      string
	Visibility    string
	AssetMediaIDs []string
}

// PromoPackageResolvedAsset is one bound asset with its live media record, used
// by the review surface to render state and by the release gate to verify the
// exact approved bytes are still attachable. Reason carries the per-asset
// blocking reason (empty when the asset resolves to the PUBLISHED state) so the
// review projection can render conspicuous per-asset states.
type PromoPackageResolvedAsset struct {
	Binding models.PromoPackageAsset
	Media   *models.Media // nil when the binding cannot resolve
	Reason  string
}

// PromoPackageReviewReadState is the complete, hash-bound review state exposed
// to authorized clients. CurrentVerdicts contains only verdicts that apply to
// the present package digest and were recorded after the active grant.
type PromoPackageReviewReadState struct {
	ContentHash               string
	Grants                    []*models.PromoReviewGrant
	GrantCount                int
	GrantsTruncated           bool
	CurrentVerdicts           map[string]*models.PromoReviewVerdict
	ReviewersApproved         bool
	PrincipalApprovalRequired bool
	PrincipalApproved         bool
	PrincipalUnavailable      bool
	ResolvedAssets            []PromoPackageResolvedAsset
	ReleaseEligible           bool
	BlockingReasons           []string
}

// PromoPackageRelease is the outcome of a successful release: the stamped
// package and the created outbound Status.
type PromoPackageRelease struct {
	Package          *models.PromoPackage
	ReleasedStatusID string
	StatusURL        string
}

// PromoPackageStampError surfaces a release whose outbound Status WAS created
// but could not be stamped onto the package (the final releasing -> released
// write failed). The caller must NOT blindly retry — a retry would create a
// second post — so the created status ID is carried for operator
// reconciliation. The package stays in the transient releasing reservation,
// which blocks further release attempts until it is reconciled.
type PromoPackageStampError struct {
	ReleasedStatusID string
	Err              error
}

func (e *PromoPackageStampError) Error() string {
	return fmt.Sprintf("promo package release created status %s but could not stamp it: %v", e.ReleasedStatusID, e.Err)
}

func (e *PromoPackageStampError) Unwrap() error { return e.Err }

type promoPackageRepository interface {
	CreatePromoPackage(context.Context, *models.PromoPackage) error
	GetPromoPackage(context.Context, string, string) (*models.PromoPackage, error)
	UpdatePromoPackageContent(context.Context, string, *models.PromoPackage) error
	MarkPromoPackageReleased(context.Context, string, *models.PromoPackage) error
	MarkPromoPackageReleasing(context.Context, string, *models.PromoPackage) error
	RevertPromoPackageReleasing(context.Context, string, *models.PromoPackage) error
	ListPromoPackages(context.Context, string, int, string) ([]*models.PromoPackage, string, error)
	CreatePromoReviewGrant(context.Context, *models.PromoReviewGrant) error
	RegrantPromoReviewGrant(context.Context, *models.PromoReviewGrant) error
	RevokePromoReviewGrant(context.Context, *models.PromoReviewGrant) error
	GetPromoReviewGrant(context.Context, string, string, string) (*models.PromoReviewGrant, error)
	ListActivePromoReviewGrants(context.Context, string, int, string) ([]*models.PromoReviewGrant, string, error)
	ListPromoReviewGrants(context.Context, string, string) ([]*models.PromoReviewGrant, error)
	ListPromoReviewGrantsByOwner(context.Context, string) ([]*models.PromoReviewGrant, error)
	CreatePromoReviewVerdict(context.Context, *models.PromoReviewVerdict) error
	ListPromoReviewVerdicts(context.Context, string, string) ([]*models.PromoReviewVerdict, error)
}

// promoStatusCreator is the outbound release seam implemented by the notes
// service. It creates the public/unlisted Status with the exact PUBLISHED
// assets attached and preserves AI-authorship disclosure.
type promoStatusCreator interface {
	CreatePromoNote(context.Context, *notes.CreateNoteCommand, []notes.PromoPublishedMediaRef) (*notes.NoteResult, error)
}

// SetPromoPackageRepository wires the promo package persistence used by the
// compose, review, and release operations.
func (s *DraftService) SetPromoPackageRepository(repo promoPackageRepository) {
	s.promoRepo = repo
}

// SetPromoStatusCreator wires the outbound Status creation used by the release
// transition. Without it, a package with approved assets cannot release.
func (s *DraftService) SetPromoStatusCreator(creator promoStatusCreator) {
	s.promoStatusCreator = creator
}

func (s *DraftService) promoReviewRepository() (promoPackageRepository, error) {
	repo := s.promoRepo
	if repo == nil {
		return nil, errPromoReviewStorageUnavailable
	}
	return repo, nil
}

// ComposePromoPackage creates a new promo package or replaces the content of an
// existing one (PackageID set). Every content change re-hashes the package, so
// prior review verdicts and principal authorization go stale through the
// verdict-vs-hash comparison and release stays blocked until the changed
// package is re-reviewed and re-authorized. Only PUBLISHED assets owned by the
// composer can bind (structurally: any other lifecycle state is rejected), and
// visibility is restricted to public/unlisted (issue #1446 scope). A released
// package cannot be re-composed.
func (s *DraftService) ComposePromoPackage(ctx context.Context, owner string, input PromoPackageComposeInput) (*models.PromoPackage, error) {
	owner = strings.TrimSpace(owner)
	repo, err := s.promoReviewRepository()
	if err != nil {
		return nil, err
	}
	pkg, err := s.buildPromoPackageContent(ctx, owner, input)
	if err != nil {
		return nil, err
	}

	packageID := strings.TrimSpace(input.PackageID)
	if packageID == "" {
		pkg.PackageID = uuid.New().String()
		pkg.Status = models.PromoPackageStatusDraft
		now := time.Now().UTC()
		pkg.CreatedAt = now
		pkg.UpdatedAt = now
		pkg.ContentHash = models.PromoPackageContentHash(pkg)
		if err := repo.CreatePromoPackage(ctx, pkg); err != nil {
			return nil, err
		}
		return pkg, nil
	}

	existing, err := repo.GetPromoPackage(ctx, owner, packageID)
	if err != nil {
		return nil, err
	}
	if existing.IsReleased() {
		return nil, ErrPromoPackageAlreadyReleased
	}
	if existing.IsReleasing() {
		return nil, ErrPromoPackageReleaseInProgress
	}
	pkg.PackageID = packageID
	pkg.OwnerID = owner
	pkg.Status = existing.Status
	pkg.CreatedAt = existing.CreatedAt
	pkg.ModelVersion = existing.ModelVersion
	pkg.UpdatedAt = time.Now().UTC()
	pkg.ContentHash = models.PromoPackageContentHash(pkg)
	if err := repo.UpdatePromoPackageContent(ctx, owner, pkg); err != nil {
		if apperrors.HasCode(err, apperrors.CodeConflict) || errors.Is(err, storage.ErrVersionConflict) {
			return nil, errors.Join(ErrPromoPackageConflict, err)
		}
		return nil, err
	}
	return pkg, nil
}

// buildPromoPackageContent validates the compose input and derives the content
// record: the referenced article must exist and be published, every asset must
// exist, belong to the composer, and be in the PUBLISHED durable state with its
// canonical digest bound at compose time, and visibility must be public or
// unlisted. This is the enforcement point where internal/unpublished assets are
// structurally rejected for outbound attachment.
func (s *DraftService) buildPromoPackageContent(ctx context.Context, owner string, input PromoPackageComposeInput) (*models.PromoPackage, error) {
	if strings.TrimSpace(input.PostText) == "" {
		return nil, errors.New("promo package post text is required")
	}
	if len(input.PostText) > maxPromoPostTextBytes {
		return nil, fmt.Errorf("promo package post text exceeds %d bytes", maxPromoPostTextBytes)
	}
	visibility, err := models.NormalizePromoPackageVisibility(input.Visibility)
	if err != nil {
		return nil, err
	}
	articleID := strings.TrimSpace(input.ArticleID)
	if articleID == "" {
		return nil, errors.New("promo package article reference is required")
	}
	if s.articleService == nil {
		return nil, errors.New("article service is unavailable")
	}
	article, err := s.articleService.GetArticle(ctx, articleID)
	if err != nil {
		return nil, fmt.Errorf("promo package article lookup failed: %w", err)
	}
	if article == nil || article.Published.IsZero() {
		return nil, errors.New("promo package must reference a published article")
	}
	if len(input.AssetMediaIDs) == 0 {
		return nil, errors.New("promo package requires at least one published asset")
	}
	if s.mediaRepo == nil {
		return nil, errors.New("editorial media repository is unavailable")
	}

	assets := make([]models.PromoPackageAsset, 0, len(input.AssetMediaIDs))
	for _, mediaID := range input.AssetMediaIDs {
		mediaID = strings.TrimSpace(mediaID)
		media, getErr := s.mediaRepo.GetMedia(ctx, mediaID)
		if getErr != nil {
			return nil, fmt.Errorf("promo package asset lookup failed: %w", getErr)
		}
		if media == nil || !strings.EqualFold(strings.TrimSpace(media.UserID), owner) {
			return nil, fmt.Errorf("promo package asset %q does not belong to the composer", mediaID)
		}
		// Load-bearing PUBLISHED-only guard: only assets in the M2 PUBLISHED
		// durable state may attach to outbound posts; their published URLs are
		// already world-served by design. Internal/unpublished lifecycle states
		// are structurally rejected here, not merely discouraged (documented in
		// docs/architecture/cms/promo-package.md).
		if !media.IsPublished() {
			return nil, fmt.Errorf("%w: asset %q is not in the PUBLISHED durable state", ErrPromoPackageAssetUnavailable, mediaID)
		}
		assets = append(assets, models.PromoPackageAsset{
			MediaID:      mediaID,
			ContentHash:  media.ContentHash,
			PublishedURL: media.PublishedURL,
		})
	}
	normalized, err := models.NormalizePromoPackageAssets(assets)
	if err != nil {
		return nil, err
	}
	return &models.PromoPackage{
		OwnerID:    owner,
		ArticleID:  articleID,
		PostText:   strings.TrimSpace(input.PostText),
		Visibility: visibility,
		Assets:     normalized,
	}, nil
}

// GetPromoPackage loads a package for its owner.
func (s *DraftService) GetPromoPackage(ctx context.Context, owner, packageID string) (*models.PromoPackage, error) {
	repo, err := s.promoReviewRepository()
	if err != nil {
		return nil, err
	}
	return repo.GetPromoPackage(ctx, strings.TrimSpace(owner), strings.TrimSpace(packageID))
}

// ListPromoPackages lists one owner's promo packages, paginated.
func (s *DraftService) ListPromoPackages(ctx context.Context, owner string, limit int, cursor string) ([]*models.PromoPackage, string, error) {
	repo, err := s.promoReviewRepository()
	if err != nil {
		return nil, "", err
	}
	return repo.ListPromoPackages(ctx, strings.TrimSpace(owner), limit, strings.TrimSpace(cursor))
}

// resolvePromoPackageAssets resolves every bound asset to its live media record
// and reports, in binding order, which assets cannot serve the exact approved
// bytes at the review or release boundary. A missing or foreign asset is
// reported as ASSET_MISSING (the draft-review posture: grants never reveal
// unbound records); a non-PUBLISHED asset and a digest change are named
// explicitly. A resolution failure (including an unwired media repository) is
// returned as an error so the review state and release gate fail closed instead
// of approving against unresolvable bytes.
func (s *DraftService) resolvePromoPackageAssets(ctx context.Context, pkg *models.PromoPackage) ([]PromoPackageResolvedAsset, []string, error) {
	if pkg == nil {
		return nil, nil, nil
	}
	if s.mediaRepo == nil {
		return nil, nil, errors.New("editorial media repository is unavailable")
	}
	resolved := make([]PromoPackageResolvedAsset, 0, len(pkg.Assets))
	var reasons []string
	for _, binding := range pkg.Assets {
		media, getErr := s.mediaRepo.GetMedia(ctx, binding.MediaID)
		if getErr != nil && (errors.Is(getErr, storage.ErrNotFound) || apperrors.HasCode(getErr, apperrors.CodeNotFound)) {
			reasons = append(reasons, PromoPackageReviewReasonAssetMissing)
			resolved = append(resolved, PromoPackageResolvedAsset{Binding: binding, Reason: PromoPackageReviewReasonAssetMissing})
			continue
		}
		if getErr != nil {
			return nil, nil, getErr
		}
		if media == nil || !strings.EqualFold(strings.TrimSpace(media.UserID), strings.TrimSpace(pkg.OwnerID)) {
			reasons = append(reasons, PromoPackageReviewReasonAssetMissing)
			resolved = append(resolved, PromoPackageResolvedAsset{Binding: binding, Reason: PromoPackageReviewReasonAssetMissing})
			continue
		}
		if !media.IsPublished() {
			reasons = append(reasons, PromoPackageReviewReasonAssetNotPublished)
			// The record is retained for the review surface so its explicit
			// editorial lifecycle can render as WITHDRAWN / SUPERSEDED /
			// UNAVAILABLE rather than a bare rejection.
			resolved = append(resolved, PromoPackageResolvedAsset{Binding: binding, Media: media, Reason: PromoPackageReviewReasonAssetNotPublished})
			continue
		}
		if strings.TrimSpace(media.ContentHash) != strings.TrimSpace(binding.ContentHash) {
			reasons = append(reasons, PromoPackageReviewReasonAssetDigestChange)
			resolved = append(resolved, PromoPackageResolvedAsset{Binding: binding, Reason: PromoPackageReviewReasonAssetDigestChange})
			continue
		}
		resolved = append(resolved, PromoPackageResolvedAsset{Binding: binding, Media: media})
	}
	return resolved, reasons, nil
}

// promoPackageHasAIOriginAssets reports whether any resolved bound asset is
// AI-origin per its provenance. Unresolvable assets contribute nothing (their
// own blocking reason already stops release).
func promoPackageHasAIOriginAssets(resolved []PromoPackageResolvedAsset) bool {
	for _, asset := range resolved {
		if asset.Media == nil || asset.Media.Provenance == nil {
			continue
		}
		switch models.EditorialMediaOrigin(strings.ToLower(strings.TrimSpace(string(asset.Media.Provenance.Origin)))) {
		case models.EditorialMediaOriginAIGenerated, models.EditorialMediaOriginAIEdited:
			return true
		}
	}
	return false
}

// SharePromoPackageForReview creates or refreshes an owner-authorized reviewer
// grant (7-day bound expiry, fail-closed, matching the draft-review posture).
func (s *DraftService) SharePromoPackageForReview(ctx context.Context, owner, packageID, reviewer string) (*models.PromoReviewGrant, error) {
	owner = strings.TrimSpace(owner)
	packageID = strings.TrimSpace(packageID)
	reviewer = strings.TrimSpace(reviewer)
	if owner == "" || packageID == "" || reviewer == "" {
		return nil, errors.New("owner, package, and reviewer are required")
	}
	if owner == reviewer {
		principal, err := s.instancePrincipal(ctx)
		if err != nil || principal != owner {
			return nil, errors.New("promo package owner cannot review their own package")
		}
	}
	repo, err := s.promoReviewRepository()
	if err != nil {
		return nil, err
	}
	if _, err := repo.GetPromoPackage(ctx, owner, packageID); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(models.PromoPackageGrantLifetime)
	g := &models.PromoReviewGrant{OwnerID: owner, PackageID: packageID, Reviewer: reviewer, GrantedAt: now, ExpiresAt: &expiresAt}
	existing, getErr := repo.GetPromoReviewGrant(ctx, owner, packageID, reviewer)
	if getErr == nil && existing != nil {
		g.Version = existing.Version
		if err := repo.RegrantPromoReviewGrant(ctx, g); err != nil {
			return nil, err
		}
		return g, nil
	}
	if getErr != nil && !errors.Is(getErr, storage.ErrNotFound) {
		return nil, getErr
	}
	if err := repo.CreatePromoReviewGrant(ctx, g); err != nil {
		return nil, err
	}
	return g, nil
}

// RevokePromoPackageReview immediately disables a reviewer grant.
func (s *DraftService) RevokePromoPackageReview(ctx context.Context, owner, packageID, reviewer string) error {
	repo, err := s.promoReviewRepository()
	if err != nil {
		return err
	}
	g, err := repo.GetPromoReviewGrant(ctx, strings.TrimSpace(owner), strings.TrimSpace(packageID), strings.TrimSpace(reviewer))
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	g.RevokedAt = &now
	return repo.RevokePromoReviewGrant(ctx, g)
}

// ActivePromoPackageReviewGrant returns a non-revoked, non-expired grant for
// one reviewer. Expired grants fail closed.
func (s *DraftService) ActivePromoPackageReviewGrant(ctx context.Context, owner, packageID, reviewer string) (*models.PromoReviewGrant, error) {
	repo, err := s.promoReviewRepository()
	if err != nil {
		return nil, err
	}
	g, err := repo.GetPromoReviewGrant(ctx, owner, packageID, reviewer)
	if err != nil {
		return nil, err
	}
	if !g.IsActive(time.Now().UTC()) {
		return nil, errors.New("promo package review grant is not active")
	}
	return g, nil
}

// SharedPromoPackageReviews lists one cursor page of active review queue grants.
func (s *DraftService) SharedPromoPackageReviews(ctx context.Context, reviewer string, limit int, cursor string) ([]*models.PromoReviewGrant, string, error) {
	repo, err := s.promoReviewRepository()
	if err != nil {
		return nil, "", err
	}
	grants, nextCursor, err := repo.ListActivePromoReviewGrants(ctx, strings.TrimSpace(reviewer), limit, strings.TrimSpace(cursor))
	if err != nil {
		return nil, "", err
	}
	now := time.Now().UTC()
	active := make([]*models.PromoReviewGrant, 0, len(grants))
	for _, grant := range grants {
		if grant != nil && grant.IsActive(now) {
			active = append(active, grant)
		}
	}
	return active, nextCursor, nil
}

// OwnedPromoPackageReviews returns active review assignments created by one
// package owner.
func (s *DraftService) OwnedPromoPackageReviews(ctx context.Context, owner string) ([]*models.PromoReviewGrant, error) {
	repo, err := s.promoReviewRepository()
	if err != nil {
		return nil, err
	}
	grants, err := repo.ListPromoReviewGrantsByOwner(ctx, strings.TrimSpace(owner))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	active := make([]*models.PromoReviewGrant, 0, len(grants))
	for _, grant := range grants {
		if grant != nil && grant.IsActive(now) {
			active = append(active, grant)
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i].SK < active[j].SK })
	return active, nil
}

// PromoPackageForCaller resolves a package only for its owner or an active
// reviewer. Pre-release packages are never world-readable.
func (s *DraftService) PromoPackageForCaller(ctx context.Context, caller, packageID string) (*models.PromoPackage, *models.PromoReviewGrant, error) {
	caller = strings.TrimSpace(caller)
	packageID = strings.TrimSpace(packageID)
	if pkg, e := s.GetPromoPackage(ctx, caller, packageID); e == nil {
		if g, grantErr := s.ActivePromoPackageReviewGrant(ctx, caller, packageID, caller); grantErr == nil {
			return pkg, g, nil
		}
		return pkg, nil, nil
	}
	repo, err := s.promoReviewRepository()
	if err != nil {
		return nil, nil, err
	}
	cursor := ""
	for {
		grants, nextCursor, e := repo.ListActivePromoReviewGrants(ctx, caller, maxPromoPackageReadGrants, cursor)
		if e != nil {
			return nil, nil, e
		}
		for _, g := range grants {
			if g != nil && g.PackageID == packageID {
				pkg, getErr := repo.GetPromoPackage(ctx, g.OwnerID, packageID)
				return pkg, g, getErr
			}
		}
		if nextCursor == "" {
			break
		}
		if nextCursor == cursor {
			return nil, nil, errors.New("promo package review pagination did not advance")
		}
		cursor = nextCursor
	}
	return nil, nil, errors.New("promo package review not found")
}

// SubmitPromoPackageReview records an immutable reviewer verdict bound to the
// exact package content hash. The caller carries the expectedContentHash it
// actually inspected; when the stored package no longer matches (the owner
// recomposed between the reviewer's read and this submit), the submit is
// rejected with a conflict signal instead of silently blessing unseen content.
// An empty expectedContentHash applies no constraint (legacy callers); the
// GraphQL surface always supplies it.
func (s *DraftService) SubmitPromoPackageReview(ctx context.Context, caller, owner, packageID, verdict, notes, expectedContentHash string) (*models.PromoReviewVerdict, error) {
	caller = strings.TrimSpace(caller)
	owner = strings.TrimSpace(owner)
	packageID = strings.TrimSpace(packageID)
	if caller == owner {
		principal, err := s.instancePrincipal(ctx)
		if err != nil || principal != owner {
			return nil, errors.New("promo package owner cannot review their own package")
		}
	}
	if _, err := s.ActivePromoPackageReviewGrant(ctx, owner, packageID, caller); err != nil {
		return nil, err
	}
	verdict = strings.ToUpper(strings.TrimSpace(verdict))
	if verdict != models.PromoPackageReviewApproved && verdict != models.PromoPackageReviewChangesRequested {
		return nil, errors.New("invalid promo package review verdict")
	}
	repo, err := s.promoReviewRepository()
	if err != nil {
		return nil, err
	}
	pkg, err := repo.GetPromoPackage(ctx, owner, packageID)
	if err != nil {
		return nil, err
	}
	if expectedContentHash = strings.TrimSpace(expectedContentHash); expectedContentHash != "" && expectedContentHash != pkg.ContentHash {
		return nil, errors.Join(ErrPromoPackageConflict, ErrPromoPackageReviewContentChanged)
	}
	v := &models.PromoReviewVerdict{
		OwnerID:     owner,
		PackageID:   packageID,
		Reviewer:    caller,
		Verdict:     verdict,
		Notes:       strings.TrimSpace(notes),
		ContentHash: models.PromoPackageContentHash(pkg),
		RecordedAt:  time.Now().UTC(),
	}
	if err := repo.CreatePromoReviewVerdict(ctx, v); err != nil {
		return nil, err
	}
	return v, nil
}

// PromoPackageVerdicts lists ordered verdict history for a package.
func (s *DraftService) PromoPackageVerdicts(ctx context.Context, owner, packageID string) ([]*models.PromoReviewVerdict, error) {
	repo, err := s.promoReviewRepository()
	if err != nil {
		return nil, err
	}
	return repo.ListPromoReviewVerdicts(ctx, owner, packageID)
}

type promoReviewApprovalState struct {
	// active maps every currently active grant (not revoked, not expired) to
	// its grant record.
	active map[string]*models.PromoReviewGrant
	// required maps every reviewer whose approval the release gate demands:
	// holders of an active grant plus every reviewer who EVER recorded a
	// verdict — revocation or expiry cannot delete a required approval
	// (operator doctrine, "requested = required").
	required     map[string]*models.PromoReviewGrant
	latest       map[string]*models.PromoReviewVerdict
	contentHash  string
	resolved     []PromoPackageResolvedAsset
	assetReasons []string
	aiOrigin     bool
}

// PromoPackageReviewState returns review grants, current approvals, resolved
// asset state, and the same eligibility decision enforced by the release gate.
func (s *DraftService) PromoPackageReviewState(ctx context.Context, owner, packageID string, pkg *models.PromoPackage) (*PromoPackageReviewReadState, error) {
	if pkg == nil {
		var err error
		pkg, err = s.GetPromoPackage(ctx, owner, packageID)
		if err != nil {
			return nil, err
		}
	}
	repo, err := s.promoReviewRepository()
	if err != nil {
		return nil, err
	}
	grants, err := repo.ListPromoReviewGrants(ctx, owner, packageID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	sort.SliceStable(grants, func(i, j int) bool {
		leftActive := grants[i] != nil && grants[i].IsActive(now)
		rightActive := grants[j] != nil && grants[j].IsActive(now)
		if leftActive != rightActive {
			return leftActive
		}
		if grants[i] == nil || grants[j] == nil {
			return grants[i] != nil
		}
		return grants[i].GrantedAt.After(grants[j].GrantedAt)
	})
	grantCount := len(grants)
	grantsTruncated := grantCount > maxPromoPackageReadGrants
	if grantsTruncated {
		grants = grants[:maxPromoPackageReadGrants]
	}
	approval, err := s.promoReviewApprovalState(ctx, owner, packageID, pkg)
	if err != nil {
		return nil, err
	}

	// The owner is the presumed releaser when rendering the read state; the
	// doctrine answers are derived for that actor (a principal owner releases
	// with implicit approval, a non-principal owner triggers the principal
	// floor).
	principal, principalErr := s.instancePrincipal(ctx)
	principalUnavailable := principalErr != nil
	principalApproved := false
	principalRequired := principalUnavailable || !strings.EqualFold(strings.TrimSpace(owner), principal)
	var reviewersApproved bool
	if principalUnavailable {
		reviewersApproved = allRequiredReviewersApprovedPromo(approval)
	} else {
		reviewersApproved, principalApproved = promoApprovalAnswers(approval, owner, principal)
	}

	blockingReasons := make([]string, 0, 8)
	if pkg.IsReleased() {
		blockingReasons = append(blockingReasons, PromoPackageReviewReasonReleased)
	}
	if pkg.IsReleasing() {
		blockingReasons = append(blockingReasons, PromoPackageReviewReasonReleasing)
	}
	if !reviewersApproved {
		blockingReasons = append(blockingReasons, PromoPackageReviewReasonApprovalRequired)
	}
	if principalUnavailable {
		blockingReasons = append(blockingReasons, PromoPackageReviewReasonPrincipalMissing)
	} else if principalRequired && !principalApproved {
		blockingReasons = append(blockingReasons, PromoPackageReviewReasonPrincipalRequired)
	}
	blockingReasons = append(blockingReasons, approval.assetReasons...)
	return &PromoPackageReviewReadState{
		ContentHash:               approval.contentHash,
		Grants:                    grants,
		GrantCount:                grantCount,
		GrantsTruncated:           grantsTruncated,
		CurrentVerdicts:           approval.latest,
		ReviewersApproved:         reviewersApproved,
		PrincipalApprovalRequired: principalRequired,
		PrincipalApproved:         principalApproved,
		PrincipalUnavailable:      principalUnavailable,
		ResolvedAssets:            approval.resolved,
		ReleaseEligible:           len(blockingReasons) == 0,
		BlockingReasons:           blockingReasons,
	}, nil
}

func (s *DraftService) promoReviewApprovalState(ctx context.Context, owner, packageID string, pkg *models.PromoPackage) (*promoReviewApprovalState, error) {
	repo, err := s.promoReviewRepository()
	if err != nil {
		return nil, err
	}
	grants, err := repo.ListPromoReviewGrants(ctx, owner, packageID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	active := make(map[string]*models.PromoReviewGrant, len(grants))
	grantByReviewer := make(map[string]*models.PromoReviewGrant, len(grants))
	for _, grant := range grants {
		if grant == nil || strings.TrimSpace(grant.Reviewer) == "" {
			continue
		}
		grantByReviewer[grant.Reviewer] = grant
		if grant.IsActive(now) {
			active[grant.Reviewer] = grant
		}
	}
	if pkg == nil {
		pkg, err = repo.GetPromoPackage(ctx, owner, packageID)
		if err != nil {
			return nil, err
		}
	}
	resolved, reasons, resolveErr := s.resolvePromoPackageAssets(ctx, pkg)
	if resolveErr != nil {
		return nil, resolveErr
	}
	currentHash := models.PromoPackageContentHash(pkg)
	verdicts, listErr := repo.ListPromoReviewVerdicts(ctx, owner, packageID)
	if listErr != nil {
		return nil, listErr
	}
	// "Requested = required" (operator doctrine): every active grant is
	// required, and every reviewer who ever recorded a verdict stays required
	// even when their grant was later revoked or expired — revocation cannot
	// delete a required approval.
	required := make(map[string]*models.PromoReviewGrant, len(grantByReviewer))
	for reviewer, grant := range grantByReviewer {
		if _, ok := active[reviewer]; ok {
			required[reviewer] = grant
		}
	}
	for _, verdict := range verdicts {
		if verdict == nil || strings.TrimSpace(verdict.Reviewer) == "" {
			continue
		}
		if _, ok := required[verdict.Reviewer]; !ok {
			if grant := grantByReviewer[verdict.Reviewer]; grant != nil {
				required[verdict.Reviewer] = grant
			}
		}
	}
	latest := make(map[string]*models.PromoReviewVerdict, len(required))
	for _, verdict := range verdicts {
		if verdict == nil {
			continue
		}
		grant := required[verdict.Reviewer]
		// A re-grant deliberately requires a verdict recorded after the new
		// grant; a hash mismatch stales the approval on any content change.
		if grant != nil && verdict.RecordedAt.After(grant.GrantedAt) &&
			verdict.ContentHash == currentHash {
			if current := latest[verdict.Reviewer]; current == nil || verdict.RecordedAt.After(current.RecordedAt) {
				latest[verdict.Reviewer] = verdict
			}
		}
	}
	return &promoReviewApprovalState{
		active:       active,
		required:     required,
		latest:       latest,
		contentHash:  currentHash,
		resolved:     resolved,
		assetReasons: reasons,
		aiOrigin:     promoPackageHasAIOriginAssets(resolved),
	}, nil
}

// allRequiredReviewersApprovedPromo reports whether every required reviewer
// (active grants plus ever-recorded-verdict reviewers) holds a current
// approving verdict for the exact reviewed content.
func allRequiredReviewersApprovedPromo(state *promoReviewApprovalState) bool {
	if state == nil {
		return false
	}
	for reviewer := range state.required {
		if verdict := state.latest[reviewer]; verdict == nil || verdict.Verdict != models.PromoPackageReviewApproved {
			return false
		}
	}
	return true
}

// promoApprovalAnswers applies the operator doctrine matrix to an already
// derived approval snapshot for one releasing actor. When the releaser IS the
// principal, their own requirement is implicit (their action is the approval)
// and only other required reviewers must approve; for a non-principal releaser
// the principal floor demands an active grant with a current approving verdict,
// regardless of asset provenance.
func promoApprovalAnswers(state *promoReviewApprovalState, releaser, principal string) (reviewersApproved, principalApproved bool) {
	required := state.required
	if releaser == principal {
		required = promoRequiredWithoutPrincipal(state.required, principal)
	}
	for reviewer := range required {
		if verdict := state.latest[reviewer]; verdict == nil || verdict.Verdict != models.PromoPackageReviewApproved {
			return false, false
		}
	}
	if releaser == principal {
		return true, true
	}
	grant := state.active[principal]
	verdict := state.latest[principal]
	return true, grant != nil && verdict != nil && verdict.Verdict == models.PromoPackageReviewApproved
}

// promoRequiredWithoutPrincipal returns the required set with the principal's
// own requirement dropped (their release action is the implicit approval).
func promoRequiredWithoutPrincipal(required map[string]*models.PromoReviewGrant, principal string) map[string]*models.PromoReviewGrant {
	if required == nil {
		return nil
	}
	out := make(map[string]*models.PromoReviewGrant, len(required))
	for reviewer, grant := range required {
		if reviewer == principal {
			continue
		}
		out[reviewer] = grant
	}
	return out
}

// promoReviewGateApprovals derives the approval snapshot once for the release
// gate, then answers the reviewer-approval and principal-approval requirements
// of the operator doctrine from that same snapshot: every required reviewer
// (active grants plus ever-recorded-verdict reviewers — revocation cannot
// delete a required approval) must hold a current approving verdict, and the
// principal is required for any non-principal release regardless of asset
// provenance.
func (s *DraftService) promoReviewGateApprovals(ctx context.Context, releaser, owner, packageID string, pkg *models.PromoPackage) (bool, bool, *promoReviewApprovalState, error) {
	if pkg == nil {
		var err error
		pkg, err = s.GetPromoPackage(ctx, owner, packageID)
		if err != nil {
			return false, false, nil, err
		}
	}
	state, err := s.promoReviewApprovalState(ctx, owner, packageID, pkg)
	if err != nil {
		return false, false, nil, err
	}
	principal, err := s.instancePrincipal(ctx)
	if err != nil {
		return false, false, state, err
	}
	reviewersApproved, principalApproved := promoApprovalAnswers(state, strings.TrimSpace(releaser), principal)
	return reviewersApproved, principalApproved, state, nil
}

// promoDisclosureForPackage builds the outbound AI-authorship disclosure for a
// released package. When any bound asset is AI-origin per provenance, the
// created Status carries an AgentPostAttribution recording that the release was
// manually triggered and approved by the instance principal — the same
// disclosed-AI posture the article surface expresses through GeneratedBy /
// ReviewedBy, transported on the post surface. Packages with no AI-origin
// assets release with no attribution.
func promoDisclosureForPackage(principal string, aiOrigin bool) *activitypub.AgentPostAttribution {
	if !aiOrigin || strings.TrimSpace(principal) == "" {
		return nil
	}
	return &activitypub.AgentPostAttribution{
		TriggerType:   "manual",
		ApprovedBy:    principal,
		SchemaVersion: activitypub.AgentAttributionSchemaVersion,
	}
}

// ReleasePromoPackage releases an approved package: the operator doctrine gate
// (every required reviewer holds a current approving verdict — active grants
// plus ever-recorded-verdict reviewers, since revocation cannot delete a
// required approval — and the instance principal holds a current approval for
// any non-principal release, regardless of asset provenance) must be current
// for the exact reviewed content, and every bound asset must still be in the
// PUBLISHED durable state carrying the digest bound at review time. The release
// transition reserves the package FIRST (draft -> releasing through a
// version-conditioned write, so a concurrent double-release has exactly one
// winner and every loser conflicts before any post exists), then creates the
// outbound Status with the exact approved assets attached (reusing the M2
// published serving, no re-upload) and AI-authorship disclosure intact, then
// finalizes releasing -> released recording the created Status. On post-creation
// failure the reservation is rolled back to draft (same CAS lane); on finalize
// failure the created Status ID is surfaced via PromoPackageStampError so the
// caller cannot blindly retry into a second post. The release creates the post
// and nothing else (no boosts, likes, or synthetic engagement). A released
// package cannot release again.
func (s *DraftService) ReleasePromoPackage(ctx context.Context, owner, packageID string) (*PromoPackageRelease, error) {
	owner = strings.TrimSpace(owner)
	packageID = strings.TrimSpace(packageID)
	if s.promoStatusCreator == nil {
		return nil, errors.New("promo status creator is unavailable")
	}
	repo, err := s.promoReviewRepository()
	if err != nil {
		return nil, err
	}
	pkg, err := repo.GetPromoPackage(ctx, owner, packageID)
	if err != nil {
		return nil, err
	}
	if pkg.IsReleased() {
		return nil, ErrPromoPackageAlreadyReleased
	}
	if pkg.IsReleasing() {
		// A previous release reserved the transition but never stamped an
		// outbound Status (crash between reservation and stamp). Releasing
		// again would risk a duplicate post; an operator must reconcile the
		// reservation.
		return nil, ErrPromoPackageReleaseInProgress
	}
	approved, principalApproved, approval, err := s.promoReviewGateApprovals(ctx, owner, owner, packageID, pkg)
	if err != nil {
		return nil, err
	}
	if !approved {
		return nil, errors.Join(ErrPromoPackageApprovalRequired,
			fmt.Errorf("blocking reasons: %s", strings.Join(promoBlockingReasonsForGate(approval, false), ", ")))
	}
	if !principalApproved {
		return nil, errors.Join(ErrPromoPackagePrincipalApprovalRequired,
			fmt.Errorf("blocking reasons: %s", strings.Join(promoBlockingReasonsForGate(approval, true), ", ")))
	}
	if len(approval.assetReasons) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrPromoPackageAssetUnavailable, strings.Join(approval.assetReasons, ", "))
	}

	refs := make([]notes.PromoPublishedMediaRef, 0, len(pkg.Assets))
	for _, asset := range pkg.Assets {
		refs = append(refs, notes.PromoPublishedMediaRef{MediaID: asset.MediaID, ContentHash: asset.ContentHash})
	}

	var principal string
	if approval.aiOrigin {
		principal, err = s.instancePrincipal(ctx)
		if err != nil {
			return nil, errors.Join(ErrPromoPackagePrincipalApprovalRequired, err)
		}
		if strings.TrimSpace(s.domain) == "" {
			return nil, errors.New("domain is required to release promo packages")
		}
	}

	// Reserve the release transition FIRST, before any outbound Status exists.
	// The version-conditioned reservation means exactly one concurrent releaser
	// wins; every loser conflicts here — before a post can be created — instead
	// of racing a plain read and both creating public posts (the CAS on the
	// final stamp alone could not prevent the loser's post from going live).
	now := time.Now().UTC()
	reserving := *pkg
	reserving.Status = models.PromoPackageStatusReleasing
	reserving.UpdatedAt = now
	if err := repo.MarkPromoPackageReleasing(ctx, owner, &reserving); err != nil {
		if apperrors.HasCode(err, apperrors.CodeConflict) || errors.Is(err, storage.ErrVersionConflict) {
			return nil, errors.Join(ErrPromoPackageConflict, err)
		}
		return nil, err
	}

	rollbackReleasing := func() {
		// Best-effort rollback on the same version-conditioned lane: only the
		// reservation winner holds the post-reservation version, so only it can
		// return the package to draft. The post was never created, so there is
		// nothing live to retract; a rollback conflict leaves the package in the
		// releasing reservation, which blocks future release attempts (safe).
		rollback := reserving
		rollback.Status = models.PromoPackageStatusDraft
		rollback.UpdatedAt = time.Now().UTC()
		if rbErr := repo.RevertPromoPackageReleasing(ctx, owner, &rollback); rbErr != nil {
			if s.logger != nil {
				s.logger.Warn("promo release rollback failed; package left in releasing reservation",
					zap.String("owner_id", owner), zap.String("package_id", packageID), zap.Error(rbErr))
			}
		}
	}

	result, err := s.promoStatusCreator.CreatePromoNote(ctx, &notes.CreateNoteCommand{
		AuthorID:         owner,
		Content:          pkg.PostText,
		Visibility:       pkg.Visibility,
		AgentAttribution: promoDisclosureForPackage(common.GenerateActorID(s.domain, principal), approval.aiOrigin),
	}, refs)
	if err != nil {
		rollbackReleasing()
		return nil, err
	}
	if result == nil || result.Note == nil {
		rollbackReleasing()
		return nil, errors.New("promo release created no status")
	}

	stampTime := time.Now().UTC()
	released := reserving
	released.Status = models.PromoPackageStatusReleased
	released.ReleasedStatusID = result.Note.StatusID
	released.ReleasedAt = &stampTime
	released.UpdatedAt = stampTime
	if err := repo.MarkPromoPackageReleased(ctx, owner, &released); err != nil {
		// The post IS live but the package could not be stamped. Surface the
		// created status ID so the caller cannot blindly retry (a retry would
		// create a second post); the package stays in the releasing reservation,
		// which blocks further release attempts until an operator reconciles.
		return nil, &PromoPackageStampError{ReleasedStatusID: result.Note.StatusID, Err: err}
	}

	statusURL := ""
	if result.Note.Note != nil {
		statusURL = result.Note.Note.ID
	}
	return &PromoPackageRelease{
		Package:          &released,
		ReleasedStatusID: result.Note.StatusID,
		StatusURL:        statusURL,
	}, nil
}

// promoBlockingReasonsForGate renders the gate-side blocking reasons for the
// approval error path. principalBlocked selects the principal reason when the
// release gate failed on principal approval.
func promoBlockingReasonsForGate(state *promoReviewApprovalState, principalBlocked bool) []string {
	reasons := make([]string, 0, 4)
	if state != nil && !allRequiredReviewersApprovedPromo(state) {
		reasons = append(reasons, PromoPackageReviewReasonApprovalRequired)
	}
	if principalBlocked {
		reasons = append(reasons, PromoPackageReviewReasonPrincipalRequired)
	}
	if state != nil {
		reasons = append(reasons, state.assetReasons...)
	}
	return reasons
}
