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

	// maxPromoPostTextBytes mirrors the notes service content limit so a
	// reviewed package can always be released as composed.
	maxPromoPostTextBytes = 5000

	// maxPromoPackageReadGrants bounds the reviewer queue read like the draft
	// review surface.
	maxPromoPackageReadGrants = 200
)

var (
	// ErrPromoPackageApprovalRequired means an active reviewer is missing a
	// current approval for the exact reviewed package content.
	ErrPromoPackageApprovalRequired = errors.New("promo package requires approval from every active reviewer")
	// ErrPromoPackagePrincipalApprovalRequired means the package binds an
	// AI-origin asset (per provenance) and the instance principal is missing a
	// current approval.
	ErrPromoPackagePrincipalApprovalRequired = errors.New("promo package with AI-origin assets requires an active approval from the instance principal")
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
// exact approved bytes are still attachable.
type PromoPackageResolvedAsset struct {
	Binding models.PromoPackageAsset
	Media   *models.Media // nil when the binding cannot resolve
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

type promoPackageRepository interface {
	CreatePromoPackage(context.Context, *models.PromoPackage) error
	GetPromoPackage(context.Context, string, string) (*models.PromoPackage, error)
	UpdatePromoPackageContent(context.Context, string, *models.PromoPackage) error
	MarkPromoPackageReleased(context.Context, string, *models.PromoPackage) error
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
			resolved = append(resolved, PromoPackageResolvedAsset{Binding: binding})
			continue
		}
		if getErr != nil {
			return nil, nil, getErr
		}
		if media == nil || !strings.EqualFold(strings.TrimSpace(media.UserID), strings.TrimSpace(pkg.OwnerID)) {
			reasons = append(reasons, PromoPackageReviewReasonAssetMissing)
			resolved = append(resolved, PromoPackageResolvedAsset{Binding: binding})
			continue
		}
		if !media.IsPublished() {
			reasons = append(reasons, PromoPackageReviewReasonAssetNotPublished)
			resolved = append(resolved, PromoPackageResolvedAsset{Binding: binding})
			continue
		}
		if strings.TrimSpace(media.ContentHash) != strings.TrimSpace(binding.ContentHash) {
			reasons = append(reasons, PromoPackageReviewReasonAssetDigestChange)
			resolved = append(resolved, PromoPackageResolvedAsset{Binding: binding})
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
// exact current package content hash.
func (s *DraftService) SubmitPromoPackageReview(ctx context.Context, caller, owner, packageID, verdict, notes string) (*models.PromoReviewVerdict, error) {
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
	active       map[string]*models.PromoReviewGrant
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

	reviewersApproved := allActiveReviewersApprovedPromo(approval)
	principalUnavailable := false
	principalApproved := false
	if approval.aiOrigin {
		principal, principalErr := s.instancePrincipal(ctx)
		if principalErr != nil {
			principalUnavailable = true
		} else {
			verdict := approval.latest[principal]
			principalApproved = approval.active[principal] != nil && verdict != nil && verdict.Verdict == models.PromoPackageReviewApproved
		}
	}

	blockingReasons := make([]string, 0, 8)
	if pkg.IsReleased() {
		blockingReasons = append(blockingReasons, PromoPackageReviewReasonReleased)
	}
	if !reviewersApproved {
		blockingReasons = append(blockingReasons, PromoPackageReviewReasonApprovalRequired)
	}
	if approval.aiOrigin {
		if principalUnavailable {
			blockingReasons = append(blockingReasons, PromoPackageReviewReasonPrincipalMissing)
		} else if !principalApproved {
			blockingReasons = append(blockingReasons, PromoPackageReviewReasonPrincipalRequired)
		}
	}
	blockingReasons = append(blockingReasons, approval.assetReasons...)
	return &PromoPackageReviewReadState{
		ContentHash:               approval.contentHash,
		Grants:                    grants,
		GrantCount:                grantCount,
		GrantsTruncated:           grantsTruncated,
		CurrentVerdicts:           approval.latest,
		ReviewersApproved:         reviewersApproved,
		PrincipalApprovalRequired: approval.aiOrigin,
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
	for _, grant := range grants {
		if grant != nil && grant.IsActive(now) {
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
	latest := make(map[string]*models.PromoReviewVerdict, len(active))
	if len(active) > 0 {
		verdicts, listErr := repo.ListPromoReviewVerdicts(ctx, owner, packageID)
		if listErr != nil {
			return nil, listErr
		}
		for _, verdict := range verdicts {
			if verdict == nil {
				continue
			}
			grant := active[verdict.Reviewer]
			// A re-grant deliberately requires a verdict recorded after the new
			// grant; a hash mismatch stales the approval on any content change.
			if grant != nil && verdict.RecordedAt.After(grant.GrantedAt) &&
				verdict.ContentHash == currentHash {
				if current := latest[verdict.Reviewer]; current == nil || verdict.RecordedAt.After(current.RecordedAt) {
					latest[verdict.Reviewer] = verdict
				}
			}
		}
	}
	return &promoReviewApprovalState{
		active:       active,
		latest:       latest,
		contentHash:  currentHash,
		resolved:     resolved,
		assetReasons: reasons,
		aiOrigin:     promoPackageHasAIOriginAssets(resolved),
	}, nil
}

func allActiveReviewersApprovedPromo(state *promoReviewApprovalState) bool {
	if state == nil {
		return false
	}
	for reviewer := range state.active {
		if verdict := state.latest[reviewer]; verdict == nil || verdict.Verdict != models.PromoPackageReviewApproved {
			return false
		}
	}
	return true
}

// promoReviewGateApprovals derives the approval snapshot once for the release
// gate: unanimous active-reviewer approval plus, when any bound asset is
// AI-origin per provenance, an active approval from the instance principal.
func (s *DraftService) promoReviewGateApprovals(ctx context.Context, owner, packageID string, pkg *models.PromoPackage) (bool, bool, *promoReviewApprovalState, error) {
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
	if !allActiveReviewersApprovedPromo(state) {
		return false, false, state, nil
	}
	if !state.aiOrigin {
		return true, true, state, nil
	}
	principal, err := s.instancePrincipal(ctx)
	if err != nil {
		return false, false, state, err
	}
	grant := state.active[principal]
	verdict := state.latest[principal]
	return true, grant != nil && verdict != nil && verdict.Verdict == models.PromoPackageReviewApproved, state, nil
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

// ReleasePromoPackage releases an approved package: the gate (unanimous
// active-reviewer approval plus principal approval when any bound asset is
// AI-origin per provenance) must be current for the exact reviewed content, and
// every bound asset must still be in the PUBLISHED durable state carrying the
// digest bound at review time. The outbound Status is then created with the
// exact approved assets attached (reusing the M2 published serving, no
// re-upload) and AI-authorship disclosure intact; the release stamps the
// created Status on the package and creates nothing else (no boosts, likes, or
// synthetic engagement). A released package cannot release again.
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
	approved, principalApproved, approval, err := s.promoReviewGateApprovals(ctx, owner, packageID, pkg)
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
	result, err := s.promoStatusCreator.CreatePromoNote(ctx, &notes.CreateNoteCommand{
		AuthorID:         owner,
		Content:          pkg.PostText,
		Visibility:       pkg.Visibility,
		AgentAttribution: promoDisclosureForPackage(common.GenerateActorID(s.domain, principal), approval.aiOrigin),
	}, refs)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Note == nil {
		return nil, errors.New("promo release created no status")
	}

	now := time.Now().UTC()
	pkg.Status = models.PromoPackageStatusReleased
	pkg.ReleasedStatusID = result.Note.StatusID
	pkg.ReleasedAt = &now
	pkg.UpdatedAt = now
	if err := repo.MarkPromoPackageReleased(ctx, owner, pkg); err != nil {
		if apperrors.HasCode(err, apperrors.CodeConflict) || errors.Is(err, storage.ErrVersionConflict) {
			return nil, errors.Join(ErrPromoPackageConflict, err)
		}
		return nil, err
	}

	statusURL := ""
	if result.Note.Note != nil {
		statusURL = result.Note.Note.ID
	}
	return &PromoPackageRelease{
		Package:          pkg,
		ReleasedStatusID: result.Note.StatusID,
		StatusURL:        statusURL,
	}, nil
}

// promoBlockingReasonsForGate renders the gate-side blocking reasons for the
// approval error path. principalBlocked selects the principal reason when the
// release gate failed on principal approval.
func promoBlockingReasonsForGate(state *promoReviewApprovalState, principalBlocked bool) []string {
	reasons := make([]string, 0, 4)
	if state != nil && !allActiveReviewersApprovedPromo(state) {
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
