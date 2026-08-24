package graph

import (
	"context"
	"errors"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
)

// PromoPackage resolves one promo package for its owner or an active reviewer
// grant. Pre-release packages are never world-readable: an unrelated caller
// receives not-found.
func (r *queryResolver) PromoPackage(ctx context.Context, id string) (*model.PromoPackage, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return nil, err
	}
	caller, _, err := r.requireActingIdentity(ctx)
	if err != nil {
		return nil, err
	}
	svc := r.Registry.Drafts()
	if svc == nil {
		return nil, errors.New("draft service is not available")
	}
	pkg, _, err := svc.PromoPackageForCaller(ctx, caller, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	state, err := svc.PromoPackageReviewState(ctx, pkg.OwnerID, pkg.PackageID, pkg)
	if err != nil {
		return nil, err
	}
	verdicts, err := svc.PromoPackageVerdicts(ctx, pkg.OwnerID, pkg.PackageID)
	if err != nil {
		return nil, err
	}
	return r.convertCMSPromoPackage(ctx, pkg, state.ResolvedAssets,
		r.buildCMSPromoReview(ctx, pkg.OwnerID, pkg.PackageID, state, verdicts)), nil
}

// PromoPackages lists the authenticated owner's promo packages.
func (r *queryResolver) PromoPackages(ctx context.Context, first *int, after *model.Cursor) (*model.PromoPackageConnection, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return nil, err
	}
	owner, _, err := r.requireActingIdentity(ctx)
	if err != nil {
		return nil, err
	}
	svc := r.Registry.Drafts()
	if svc == nil {
		return nil, errors.New("draft service is not available")
	}
	cursor := trimPromoCursor(after)
	packages, nextCursor, err := svc.ListPromoPackages(ctx, owner, clampCMSPageSize(first), cursor)
	if err != nil {
		return nil, err
	}
	edges := make([]*model.PromoPackageEdge, 0, len(packages))
	for _, pkg := range packages {
		state, stateErr := svc.PromoPackageReviewState(ctx, pkg.OwnerID, pkg.PackageID, pkg)
		if stateErr != nil {
			return nil, stateErr
		}
		verdicts, verdictErr := svc.PromoPackageVerdicts(ctx, pkg.OwnerID, pkg.PackageID)
		if verdictErr != nil {
			return nil, verdictErr
		}
		edges = append(edges, &model.PromoPackageEdge{
			Node: r.convertCMSPromoPackage(ctx, pkg, state.ResolvedAssets,
				r.buildCMSPromoReview(ctx, pkg.OwnerID, pkg.PackageID, state, verdicts)),
			Cursor: model.Cursor(pkg.SK),
		})
	}
	pageInfo := &model.PageInfo{
		HasNextPage:     nextCursor != "",
		HasPreviousPage: cursor != "",
	}
	if len(edges) > 0 {
		start := edges[0].Cursor
		end := edges[len(edges)-1].Cursor
		pageInfo.StartCursor = &start
		pageInfo.EndCursor = &end
	} else if nextCursor != "" {
		end := model.Cursor(nextCursor)
		pageInfo.EndCursor = &end
	}
	return &model.PromoPackageConnection{
		Edges:      edges,
		PageInfo:   pageInfo,
		TotalCount: len(edges),
	}, nil
}

// SharedPromoPackageReviews lists the reviewer queue: packages shared for
// review with the authenticated reviewer, with their caller-authorized review
// state.
func (r *queryResolver) SharedPromoPackageReviews(ctx context.Context, first *int, after *model.Cursor) (*model.PromoPackageReviewConnection, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return nil, err
	}
	reviewer, _, err := r.requireActingIdentity(ctx)
	if err != nil {
		return nil, err
	}
	svc := r.Registry.Drafts()
	if svc == nil {
		return nil, errors.New("draft service is not available")
	}
	cursor := trimPromoCursor(after)
	grants, nextCursor, err := svc.SharedPromoPackageReviews(ctx, reviewer, clampCMSPageSize(first), cursor)
	if err != nil {
		return nil, err
	}
	edges := make([]*model.PromoPackageReviewEdge, 0, len(grants))
	for _, grant := range grants {
		if grant == nil {
			continue
		}
		pkg, getErr := svc.GetPromoPackage(ctx, grant.OwnerID, grant.PackageID)
		if getErr != nil {
			return nil, getErr
		}
		state, stateErr := svc.PromoPackageReviewState(ctx, grant.OwnerID, grant.PackageID, pkg)
		if stateErr != nil {
			return nil, stateErr
		}
		verdicts, verdictErr := svc.PromoPackageVerdicts(ctx, grant.OwnerID, grant.PackageID)
		if verdictErr != nil {
			return nil, verdictErr
		}
		edges = append(edges, &model.PromoPackageReviewEdge{
			Node:   r.buildCMSPromoReview(ctx, grant.OwnerID, grant.PackageID, state, verdicts),
			Cursor: model.Cursor(grant.GSI2SK),
		})
	}
	pageInfo := &model.PageInfo{
		HasNextPage:     nextCursor != "",
		HasPreviousPage: cursor != "",
	}
	if len(edges) > 0 {
		start := edges[0].Cursor
		end := edges[len(edges)-1].Cursor
		pageInfo.StartCursor = &start
		pageInfo.EndCursor = &end
	} else if nextCursor != "" {
		end := model.Cursor(nextCursor)
		pageInfo.EndCursor = &end
	}
	return &model.PromoPackageReviewConnection{
		Edges:      edges,
		PageInfo:   pageInfo,
		TotalCount: len(edges),
	}, nil
}

// trimPromoCursor normalizes the optional after cursor.
func trimPromoCursor(after *model.Cursor) string {
	if after == nil {
		return ""
	}
	return strings.TrimSpace(string(*after))
}
