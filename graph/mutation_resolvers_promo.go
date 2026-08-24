package graph

import (
	"context"
	"errors"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/cms"
)

// composePromoPackageInputToService maps the GraphQL compose input onto the
// service input, canonicalizing the visibility enum to the storage value.
func composePromoPackageInputToService(input model.ComposePromoPackageInput) cms.PromoPackageComposeInput {
	packageID := ""
	if input.PackageID != nil {
		packageID = strings.TrimSpace(*input.PackageID)
	}
	return cms.PromoPackageComposeInput{
		PackageID:     packageID,
		ArticleID:     strings.TrimSpace(input.ArticleID),
		PostText:      strings.TrimSpace(input.PostText),
		Visibility:    strings.ToLower(strings.TrimSpace(string(input.Visibility))),
		AssetMediaIDs: input.AssetMediaIds,
	}
}

// ComposePromoPackage creates a new promo package or replaces the content of an
// existing one. Every content change re-hashes and stales prior approvals, so
// release stays blocked until the changed package is re-reviewed and
// re-authorized.
func (r *mutationResolver) ComposePromoPackage(ctx context.Context, input model.ComposePromoPackageInput) (*model.PromoPackage, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return nil, err
	}
	owner, acting, err := r.requireActingIdentity(ctx)
	if err != nil {
		return nil, err
	}
	svc := r.Registry.Drafts()
	if svc == nil {
		return nil, errors.New("draft service is not available")
	}
	pkg, err := svc.ComposePromoPackage(ctx, owner, composePromoPackageInputToService(input))
	if err != nil {
		return nil, err
	}
	r.auditActAs(ctx, acting, "cms.promo.compose", pkg.PackageID, map[string]any{
		"visibility": pkg.Visibility, "assets": len(pkg.Assets),
	})
	state, err := svc.PromoPackageReviewState(ctx, owner, pkg.PackageID, pkg)
	if err != nil {
		return nil, err
	}
	verdicts, err := svc.PromoPackageVerdicts(ctx, owner, pkg.PackageID)
	if err != nil {
		return nil, err
	}
	return r.convertCMSPromoPackage(ctx, pkg, state.ResolvedAssets,
		r.buildCMSPromoReview(ctx, owner, pkg.PackageID, state, verdicts)), nil
}

// SharePromoPackageForReview shares a package with a reviewer (7-day bounded
// grant) and returns the reviewer-visible review state.
func (r *mutationResolver) SharePromoPackageForReview(ctx context.Context, packageID string, reviewer string) (*model.PromoPackageReview, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return nil, err
	}
	owner, acting, err := r.requireActingIdentity(ctx)
	if err != nil {
		return nil, err
	}
	svc := r.Registry.Drafts()
	if svc == nil {
		return nil, errors.New("draft service is not available")
	}
	if _, err := svc.SharePromoPackageForReview(ctx, owner, strings.TrimSpace(packageID), strings.TrimSpace(reviewer)); err != nil {
		return nil, err
	}
	r.auditActAs(ctx, acting, "cms.promo.share_review", strings.TrimSpace(packageID), map[string]any{"reviewer": strings.TrimSpace(reviewer)})
	return r.promoPackageReviewForOwner(ctx, svc, owner, strings.TrimSpace(packageID))
}

// RevokePromoPackageReview immediately disables a reviewer grant.
func (r *mutationResolver) RevokePromoPackageReview(ctx context.Context, packageID string, reviewer string) (bool, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return false, err
	}
	owner, acting, err := r.requireActingIdentity(ctx)
	if err != nil {
		return false, err
	}
	svc := r.Registry.Drafts()
	if svc == nil {
		return false, errors.New("draft service is not available")
	}
	if err := svc.RevokePromoPackageReview(ctx, owner, strings.TrimSpace(packageID), strings.TrimSpace(reviewer)); err != nil {
		return false, err
	}
	r.auditActAs(ctx, acting, "cms.promo.revoke_review", strings.TrimSpace(packageID), map[string]any{"reviewer": strings.TrimSpace(reviewer)})
	return true, nil
}

// SubmitPromoPackageReview records a hash-bound reviewer verdict on the exact
// current package content and returns the caller-authorized review state. The
// optional contentHash argument carries the hash the reviewer actually
// inspected; when it no longer matches the stored package, the submit is
// rejected with a conflict signal instead of blessing unseen content.
func (r *mutationResolver) SubmitPromoPackageReview(ctx context.Context, packageID string, verdict model.PromoPackageReviewVerdict, notes *string, contentHash *string) (*model.PromoPackageReview, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return nil, err
	}
	caller, acting, err := r.requireActingIdentity(ctx)
	if err != nil {
		return nil, err
	}
	svc := r.Registry.Drafts()
	if svc == nil {
		return nil, errors.New("draft service is not available")
	}
	pkg, grant, err := svc.PromoPackageForCaller(ctx, caller, strings.TrimSpace(packageID))
	if err != nil {
		return nil, err
	}
	if grant == nil {
		return nil, errors.New("promo package owner cannot review their own package")
	}
	if _, err := svc.SubmitPromoPackageReview(ctx, caller, pkg.OwnerID, pkg.PackageID, string(verdict), trimStringPtr(notes), trimStringPtr(contentHash)); err != nil {
		return nil, err
	}
	r.auditActAs(ctx, acting, "cms.promo.review_verdict", pkg.PackageID, map[string]any{"verdict": string(verdict)})
	return r.promoPackageReviewForOwner(ctx, svc, pkg.OwnerID, pkg.PackageID)
}

// ReleasePromoPackage releases an approved package: the outbound public/unlisted
// Status is created with the exact approved PUBLISHED assets and AI-authorship
// disclosure intact. The release is blocked with explicit reasons until the
// review gate (and, for AI-origin assets, the instance principal's explicit
// authorization) is current for the exact reviewed content.
func (r *mutationResolver) ReleasePromoPackage(ctx context.Context, packageID string) (*model.PromoPackageReleaseResult, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return nil, err
	}
	owner, acting, err := r.requireActingIdentity(ctx)
	if err != nil {
		return nil, err
	}
	svc := r.Registry.Drafts()
	if svc == nil {
		return nil, errors.New("draft service is not available")
	}
	release, err := svc.ReleasePromoPackage(ctx, owner, strings.TrimSpace(packageID))
	if err != nil {
		return nil, err
	}
	r.auditActAs(ctx, acting, "cms.promo.release", release.Package.PackageID, map[string]any{
		"status_id": release.ReleasedStatusID,
	})
	state, err := svc.PromoPackageReviewState(ctx, owner, release.Package.PackageID, release.Package)
	if err != nil {
		return nil, err
	}
	verdicts, err := svc.PromoPackageVerdicts(ctx, owner, release.Package.PackageID)
	if err != nil {
		return nil, err
	}
	return &model.PromoPackageReleaseResult{
		Package: r.convertCMSPromoPackage(ctx, release.Package, state.ResolvedAssets,
			r.buildCMSPromoReview(ctx, owner, release.Package.PackageID, state, verdicts)),
		StatusID: release.ReleasedStatusID,
		URL:      cmsOptionalString(release.StatusURL),
	}, nil
}

// promoPackageReviewForOwner builds the owner-authorized review state for one
// package.
func (r *Resolver) promoPackageReviewForOwner(ctx context.Context, svc *cms.DraftService, owner, packageID string) (*model.PromoPackageReview, error) {
	pkg, err := svc.GetPromoPackage(ctx, owner, packageID)
	if err != nil {
		return nil, err
	}
	state, err := svc.PromoPackageReviewState(ctx, owner, packageID, pkg)
	if err != nil {
		return nil, err
	}
	verdicts, err := svc.PromoPackageVerdicts(ctx, owner, packageID)
	if err != nil {
		return nil, err
	}
	return r.buildCMSPromoReview(ctx, owner, packageID, state, verdicts), nil
}
