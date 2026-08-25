package graph

import (
	"context"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/cms"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// convertCMSPromoAssetState maps a resolved package asset onto the conspicuous
// state enum. Only the PUBLISHED durable state renders PUBLISHED; a missing
// binding renders MISSING, a non-published asset renders its explicit editorial
// lifecycle (withdrawn/superseded) or UNAVAILABLE, and a digest change renders
// REJECTED (the exact approved bytes are no longer attachable).
func convertCMSPromoAssetState(resolved cms.PromoPackageResolvedAsset) model.PromoPackageAssetState {
	if resolved.Reason == "" {
		return model.PromoPackageAssetStatePublished
	}
	switch resolved.Reason {
	case cms.PromoPackageReviewReasonAssetMissing:
		return model.PromoPackageAssetStateMissing
	case cms.PromoPackageReviewReasonAssetNotPublished:
		if resolved.Media == nil {
			return model.PromoPackageAssetStateUnavailable
		}
		switch models.EditorialLifecycle(strings.ToLower(strings.TrimSpace(string(resolved.Media.EditorialState)))) {
		case models.EditorialLifecycleWithdrawn:
			return model.PromoPackageAssetStateWithdrawn
		case models.EditorialLifecycleSuperseded:
			return model.PromoPackageAssetStateSuperseded
		default:
			return model.PromoPackageAssetStateUnavailable
		}
	case cms.PromoPackageReviewReasonAssetDigestChange:
		return model.PromoPackageAssetStateRejected
	default:
		return model.PromoPackageAssetStateRejected
	}
}

// convertCMSPromoAsset maps one resolved package binding onto the review asset
// surface. Provenance is internal editorial evidence shown only to the owner
// and active reviewers (the package itself is never world-readable).
func (r *Resolver) convertCMSPromoAsset(ctx context.Context, resolved cms.PromoPackageResolvedAsset) *model.PromoPackageAsset {
	out := &model.PromoPackageAsset{
		MediaID:      resolved.Binding.MediaID,
		ContentHash:  cmsOptionalString(resolved.Binding.ContentHash),
		PublishedURL: cmsOptionalString(resolved.Binding.PublishedURL),
		State:        convertCMSPromoAssetState(resolved),
	}
	if resolved.Media != nil {
		out.Width = cmsOptionalPositiveInt(resolved.Media.Width)
		out.Height = cmsOptionalPositiveInt(resolved.Media.Height)
		out.MimeType = cmsOptionalString(resolved.Media.ContentType)
		out.Provenance = r.convertCMSEditorialMediaProvenance(ctx, resolved.Media.Provenance)
	}
	return out
}

func (r *Resolver) convertCMSPromoAssets(ctx context.Context, resolved []cms.PromoPackageResolvedAsset) []*model.PromoPackageAsset {
	out := make([]*model.PromoPackageAsset, 0, len(resolved))
	for _, asset := range resolved {
		out = append(out, r.convertCMSPromoAsset(ctx, asset))
	}
	return out
}

// convertCMSPromoGrant maps a promo review grant onto the review surface.
// Status recomputes EXPIRED at read time from the bounded expiry.
func convertCMSPromoGrant(grant *models.PromoReviewGrant) *model.PromoPackageReviewGrant {
	if grant == nil {
		return nil
	}
	status := model.PromoPackageGrantStatusActive
	var revokedAt *model.Time
	var expiresAt *model.Time
	if grant.RevokedAt != nil {
		status = model.PromoPackageGrantStatusRevoked
		value := model.Time(*grant.RevokedAt)
		revokedAt = &value
	} else if grant.Expired(time.Now().UTC()) {
		status = model.PromoPackageGrantStatusExpired
	}
	if grant.ExpiresAt != nil {
		value := model.Time(*grant.ExpiresAt)
		expiresAt = &value
	}
	return &model.PromoPackageReviewGrant{
		ReviewerID: grant.Reviewer,
		GrantedAt:  model.Time(grant.GrantedAt),
		Status:     status,
		RevokedAt:  revokedAt,
		ExpiresAt:  expiresAt,
	}
}

// convertCMSPromoVerdict maps an immutable verdict onto the review surface,
// marking whether it is valid for the current package content and active grant.
func convertCMSPromoVerdict(v *models.PromoReviewVerdict, state *cms.PromoPackageReviewReadState) *model.PromoPackageVerdictRecord {
	if v == nil {
		return nil
	}
	current := state.CurrentVerdicts[v.Reviewer]
	isCurrent := current != nil && current.RecordedAt.Equal(v.RecordedAt) && current.ContentHash == v.ContentHash
	return &model.PromoPackageVerdictRecord{
		Verdict:     model.PromoPackageReviewVerdict(v.Verdict),
		Notes:       cmsOptionalString(v.Notes),
		ContentHash: cmsOptionalString(v.ContentHash),
		ReviewerID:  v.Reviewer,
		RecordedAt:  model.Time(v.RecordedAt),
		Current:     isCurrent,
		Stale:       !isCurrent,
	}
}

// buildCMSPromoReview maps the service review state plus the ordered verdict
// history onto the GraphQL review surface. Non-owner callers (active reviewers)
// see only their own grant and verdict records — other reviewers' identities
// and notes stay private, mirroring the draft-review viewer filter; the owner
// sees the full surface.
func (r *Resolver) buildCMSPromoReview(ctx context.Context, ownerID, packageID string, state *cms.PromoPackageReviewReadState, verdicts []*models.PromoReviewVerdict) *model.PromoPackageReview {
	if state == nil {
		return nil
	}
	viewer := strings.TrimSpace(getUsernameFromContext(ctx))
	viewerIsOwner := strings.EqualFold(viewer, strings.TrimSpace(ownerID))
	grants := make([]*model.PromoPackageReviewGrant, 0, len(state.Grants))
	activeReviewerIDs := make([]string, 0, len(state.Grants))
	for _, grant := range state.Grants {
		if grant == nil || (!viewerIsOwner && !strings.EqualFold(grant.Reviewer, viewer)) {
			continue
		}
		grants = append(grants, convertCMSPromoGrant(grant))
		if grant.IsActive(time.Now().UTC()) {
			activeReviewerIDs = append(activeReviewerIDs, grant.Reviewer)
		}
	}
	grantCount := state.GrantCount
	grantsTruncated := state.GrantsTruncated
	if !viewerIsOwner {
		grantCount = len(grants)
		grantsTruncated = false
	}
	verdictRecords := make([]*model.PromoPackageVerdictRecord, 0, len(verdicts))
	for _, verdict := range verdicts {
		if verdict == nil || (!viewerIsOwner && !strings.EqualFold(verdict.Reviewer, viewer)) {
			continue
		}
		verdictRecords = append(verdictRecords, convertCMSPromoVerdict(verdict, state))
	}
	return &model.PromoPackageReview{
		PackageID:                 packageID,
		ContentHash:               state.ContentHash,
		Assets:                    r.convertCMSPromoAssets(ctx, state.ResolvedAssets),
		ActiveReviewerIds:         activeReviewerIDs,
		ReleaseEligible:           state.ReleaseEligible,
		ReleaseBlockingReasons:    state.BlockingReasons,
		ReviewersApproved:         state.ReviewersApproved,
		PrincipalApprovalRequired: state.PrincipalApprovalRequired,
		PrincipalApproved:         state.PrincipalApproved,
		GrantCount:                grantCount,
		GrantsTruncated:           grantsTruncated,
		Grants:                    grants,
		Verdicts:                  verdictRecords,
		ReleaseEligibility: &model.PromoPackageReleaseEligibility{
			Eligible:                  state.ReleaseEligible,
			BlockingReasons:           state.BlockingReasons,
			ReviewersApproved:         state.ReviewersApproved,
			PrincipalApprovalRequired: state.PrincipalApprovalRequired,
			PrincipalApproved:         state.PrincipalApproved,
		},
	}
}

// convertCMSPromoPackage maps a package record plus its resolved asset state
// onto the GraphQL package surface.
func (r *Resolver) convertCMSPromoPackage(ctx context.Context, pkg *models.PromoPackage, resolved []cms.PromoPackageResolvedAsset, review *model.PromoPackageReview) *model.PromoPackage {
	if pkg == nil {
		return nil
	}
	out := &model.PromoPackage{
		ID:          pkg.PackageID,
		OwnerID:     pkg.OwnerID,
		ArticleID:   pkg.ArticleID,
		PostText:    pkg.PostText,
		Visibility:  model.PromoPackageVisibility(strings.ToUpper(pkg.Visibility)),
		ContentHash: pkg.ContentHash,
		Status:      model.PromoPackageStatus(strings.ToUpper(pkg.Status)),
		Assets:      r.convertCMSPromoAssets(ctx, resolved),
		CreatedAt:   model.Time(pkg.CreatedAt),
		UpdatedAt:   model.Time(pkg.UpdatedAt),
		Review:      review,
	}
	if strings.TrimSpace(pkg.ReleasedStatusID) != "" {
		released := pkg.ReleasedStatusID
		out.ReleasedStatusID = &released
	}
	return out
}
