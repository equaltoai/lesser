package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	storagecore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormcore "github.com/pay-theory/dynamorm/pkg/core"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
)

const (
	defaultCMSPageSize = 25
	maxCMSPageSize     = 200
)

func clampCMSPageSize(first *int) int {
	limit := defaultCMSPageSize
	if first != nil && *first > 0 {
		limit = *first
	}
	if limit > maxCMSPageSize {
		limit = maxCMSPageSize
	}
	return limit
}

func trimStringPtr(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func cmsResolveBySlug[E any](
	ctx context.Context,
	db dynamormcore.DB,
	slug string,
	pk string,
	fetchByID func(string) (E, error),
	fetchLegacy func() (E, error),
	extractID func(E) string,
) (E, error) {
	var zero E

	var idx models.CMSSlugIndex
	err := db.WithContext(ctx).Model(&models.CMSSlugIndex{}).
		Where("PK", "=", pk).
		Where("SK", "=", models.CMSSlugIndexSK()).
		First(&idx)
	if err == nil {
		if targetID := strings.TrimSpace(idx.TargetID); targetID != "" {
			return fetchByID(targetID)
		}
	}
	if err != nil && !dynamormerrors.IsNotFound(err) {
		return zero, err
	}

	entity, err := fetchLegacy()
	if err != nil {
		return zero, err
	}

	targetID := strings.TrimSpace(extractID(entity))
	if targetID != "" {
		backfill := &models.CMSSlugIndex{
			PK:       pk,
			Slug:     slug,
			TargetID: targetID,
		}
		if err := backfill.UpdateKeys(); err == nil {
			_ = db.WithContext(ctx).Model(backfill).IfNotExists().Create()
		}
	}

	return entity, nil
}

func cmsCategoryGetter(store storagecore.RepositoryStorage) func(context.Context, string) (*models.Category, error) {
	if store == nil {
		return nil
	}
	repo := store.Category()
	if repo == nil {
		return nil
	}
	return repo.GetCategory
}

func cmsPublicationGetter(store storagecore.RepositoryStorage) func(context.Context, string) (*models.Publication, error) {
	if store == nil {
		return nil
	}
	repo := store.Publication()
	if repo == nil {
		return nil
	}
	return repo.GetPublication
}

func cmsCategoryIDFromStorage(category *models.Category) string {
	if category == nil {
		return ""
	}
	return category.ID
}

func cmsPublicationIDFromStorage(pub *models.Publication) string {
	if pub == nil {
		return ""
	}
	return pub.ID
}

func cmsFetchBySlug[E any](
	ctx context.Context,
	r *queryResolver,
	rawSlug string,
	require func(*Resolver) error,
	pk func(string) string,
	getter func(storagecore.RepositoryStorage) func(context.Context, string) (E, error),
	legacyID func(domain, slug string) string,
	extractID func(E) string,
) (E, error) {
	var zero E

	if r == nil || r.Resolver == nil {
		return zero, ErrStorageUnavailable
	}
	if require != nil {
		if err := require(r.Resolver); err != nil {
			return zero, err
		}
	}

	store := r.cmsStorage()
	getByID := getter(store)
	if store == nil || getByID == nil {
		return zero, ErrStorageUnavailable
	}

	slug := cmsSlugify(rawSlug)
	if strings.TrimSpace(slug) == "" {
		return zero, errors.New("slug is required")
	}

	domain := r.getDomain()
	legacy := legacyID(domain, slug)

	entity, err := cmsResolveBySlug(ctx, store.GetDB(), slug, pk(slug),
		func(id string) (E, error) { return getByID(ctx, id) },
		func() (E, error) { return getByID(ctx, legacy) },
		extractID,
	)
	if err != nil {
		return zero, err
	}
	return entity, nil
}

func cmsArticleMatchesFilters(article *models.Article, authorFilter string, seriesFilter string, categoryFilter string) bool {
	if article == nil {
		return false
	}

	if authorFilter != "" {
		attributedTo := strings.TrimSpace(article.AttributedTo)
		if !strings.EqualFold(attributedTo, authorFilter) && !strings.EqualFold(cmsNormalizeUsername(attributedTo), cmsNormalizeUsername(authorFilter)) {
			return false
		}
	}

	if seriesFilter != "" {
		if article.SeriesID == nil || !strings.EqualFold(strings.TrimSpace(*article.SeriesID), seriesFilter) {
			return false
		}
	}

	if categoryFilter != "" {
		for _, categoryID := range article.CategoryIDs {
			if strings.EqualFold(strings.TrimSpace(categoryID), categoryFilter) {
				return true
			}
		}
		return false
	}

	return true
}

func cmsArticleCursor(article *models.Article, fallback string) model.Cursor {
	if article != nil {
		value := strings.TrimSpace(article.GSI2SK)
		if value != "" {
			return model.Cursor(value)
		}
	}
	return model.Cursor(fallback)
}

func (r *queryResolver) Draft(ctx context.Context, id string) (*model.Draft, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	drafts := r.Registry.Drafts()
	if drafts == nil {
		return nil, errors.New("draft service is not available")
	}

	draft, err := drafts.GetDraft(ctx, username, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}

	return r.convertCMSDraft(ctx, draft), nil
}

func (r *queryResolver) MyDrafts(ctx context.Context, contentType *model.ObjectType, status *model.DraftStatus, first *int, after *model.Cursor) (*model.DraftConnection, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	store := r.cmsStorage()
	if store == nil || store.Draft() == nil {
		return nil, ErrStorageUnavailable
	}

	limit := clampCMSPageSize(first)
	cursor := ""
	if after != nil {
		cursor = string(*after)
	}

	items, nextCursor, err := store.Draft().ListDraftsByAuthorPaginated(ctx, username, limit, cursor)
	if err != nil {
		return nil, err
	}

	filtered := make([]*models.Draft, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if contentType != nil && cmsObjectTypeFromStorage(item.ContentType) != *contentType {
			continue
		}
		if status != nil && cmsDraftStatusFromStorage(item.Status) != *status {
			continue
		}
		filtered = append(filtered, item)
	}

	edges := make([]*model.DraftEdge, 0, len(filtered))
	for _, draft := range filtered {
		node := r.convertCMSDraft(ctx, draft)
		if node == nil {
			continue
		}
		edges = append(edges, &model.DraftEdge{
			Node:   node,
			Cursor: model.Cursor(node.ID),
		})
	}

	pageInfo := &model.PageInfo{
		HasNextPage:     nextCursor != "",
		HasPreviousPage: after != nil,
	}
	if len(edges) > 0 {
		start := edges[0].Cursor
		end := edges[len(edges)-1].Cursor
		pageInfo.StartCursor = &start
		pageInfo.EndCursor = &end
	}

	return &model.DraftConnection{
		Edges:      edges,
		PageInfo:   pageInfo,
		TotalCount: len(edges),
	}, nil
}

func (r *queryResolver) Revisions(ctx context.Context, objectID string, first *int, after *model.Cursor) (*model.RevisionConnection, error) {
	if err := r.requireCMSRevisionsEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	store := r.cmsStorage()
	if store == nil || store.Revision() == nil || store.Article() == nil {
		return nil, ErrStorageUnavailable
	}

	article, err := store.Article().GetArticle(ctx, strings.TrimSpace(objectID))
	if err != nil {
		return nil, err
	}
	if err := r.ensureAuthorCanWriteCMS(ctx, username, article.AttributedTo); err != nil {
		return nil, err
	}

	limit := clampCMSPageSize(first)
	cursor := ""
	if after != nil {
		cursor = string(*after)
	}

	items, nextCursor, err := store.Revision().ListRevisionsPaginated(ctx, strings.TrimSpace(objectID), limit, cursor)
	if err != nil {
		return nil, err
	}

	edges := make([]*model.RevisionEdge, 0, len(items))
	for _, revision := range items {
		node := r.convertCMSRevision(ctx, revision)
		if node == nil {
			continue
		}
		cursor := model.Cursor(revision.SK)
		edges = append(edges, &model.RevisionEdge{
			Node:   node,
			Cursor: cursor,
		})
	}

	pageInfo := &model.PageInfo{
		HasNextPage:     nextCursor != "",
		HasPreviousPage: after != nil,
	}
	if len(edges) > 0 {
		start := edges[0].Cursor
		end := edges[len(edges)-1].Cursor
		pageInfo.StartCursor = &start
		pageInfo.EndCursor = &end
	}

	return &model.RevisionConnection{
		Edges:      edges,
		PageInfo:   pageInfo,
		TotalCount: len(edges),
	}, nil
}

func (r *queryResolver) Revision(ctx context.Context, objectID string, version int) (*model.Revision, error) {
	if err := r.requireCMSRevisionsEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	store := r.cmsStorage()
	if store == nil || store.Revision() == nil || store.Article() == nil {
		return nil, ErrStorageUnavailable
	}

	article, err := store.Article().GetArticle(ctx, strings.TrimSpace(objectID))
	if err != nil {
		return nil, err
	}
	if err := r.ensureAuthorCanWriteCMS(ctx, username, article.AttributedTo); err != nil {
		return nil, err
	}

	revision, err := store.Revision().GetRevision(ctx, strings.TrimSpace(objectID), version)
	if err != nil {
		return nil, err
	}

	return r.convertCMSRevision(ctx, revision), nil
}

func (r *queryResolver) Article(ctx context.Context, id string) (*model.Article, error) {
	if err := r.requireCMSLongFormEnabled(); err != nil {
		return nil, err
	}

	store := r.cmsStorage()
	if store == nil || store.Article() == nil {
		return nil, ErrStorageUnavailable
	}

	article, err := store.Article().GetArticle(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}

	return r.convertCMSArticle(ctx, article, true), nil
}

func (r *queryResolver) ArticleBySlug(ctx context.Context, slug string) (*model.Article, error) {
	if err := r.requireCMSLongFormEnabled(); err != nil {
		return nil, err
	}

	store := r.cmsStorage()
	if store == nil || store.Article() == nil {
		return nil, ErrStorageUnavailable
	}

	domain := r.getDomain()
	slug = cmsSlugify(slug)
	if strings.TrimSpace(slug) == "" {
		return nil, errors.New("slug is required")
	}

	// Preferred path: use a slug index so IDs are stable and slugs are editable.
	var idx models.CMSSlugIndex
	err := store.GetDB().WithContext(ctx).Model(&models.CMSSlugIndex{}).
		Where("PK", "=", models.CMSArticleSlugIndexPK(slug)).
		Where("SK", "=", models.CMSSlugIndexSK()).
		First(&idx)
	if err == nil && strings.TrimSpace(idx.TargetID) != "" {
		article, err := store.Article().GetArticle(ctx, strings.TrimSpace(idx.TargetID))
		if err == nil && article != nil {
			return r.convertCMSArticle(ctx, article, true), nil
		}
	}
	if err != nil && !dynamormerrors.IsNotFound(err) {
		return nil, err
	}

	legacyID := cmsArticleID(domain, slug)
	article, err := store.Article().GetArticle(ctx, legacyID)
	if err != nil {
		return nil, err
	}

	// Best-effort backfill for legacy slug-derived IDs.
	backfill := &models.CMSSlugIndex{
		PK:       models.CMSArticleSlugIndexPK(slug),
		Slug:     slug,
		TargetID: strings.TrimSpace(article.ID),
	}
	if backfill.TargetID != "" {
		if err := backfill.UpdateKeys(); err == nil {
			_ = store.GetDB().WithContext(ctx).Model(backfill).IfNotExists().Create()
		}
	}

	return r.convertCMSArticle(ctx, article, true), nil
}

func (r *queryResolver) Articles(ctx context.Context, authorID *string, seriesID *string, categoryID *string, first *int, after *model.Cursor) (*model.ArticleConnection, error) {
	if err := r.requireCMSLongFormEnabled(); err != nil {
		return nil, err
	}

	store := r.cmsStorage()
	if store == nil || store.Article() == nil {
		return nil, ErrStorageUnavailable
	}

	limit := clampCMSPageSize(first)
	cursor := ""
	if after != nil {
		cursor = string(*after)
	}

	authorFilter := trimStringPtr(authorID)
	seriesFilter := trimStringPtr(seriesID)
	categoryFilter := trimStringPtr(categoryID)

	if seriesFilter != "" && !r.cmsSeriesEnabled() {
		return nil, errCMSSeriesDisabled
	}
	if categoryFilter != "" && !r.cmsCategoriesEnabled() {
		return nil, errCMSCategoriesDisabled
	}

	list, cursorFn := r.cmsArticlesListStrategy(ctx, store, authorFilter, seriesFilter, categoryFilter)
	edges, hasMore, nextCursor, err := r.cmsCollectArticleEdges(ctx, list, cursorFn, authorFilter, seriesFilter, categoryFilter, limit, cursor)
	if err != nil {
		return nil, err
	}

	pageInfo := &model.PageInfo{
		HasNextPage:     hasMore,
		HasPreviousPage: after != nil,
	}
	if len(edges) > 0 {
		start := edges[0].Cursor
		end := edges[len(edges)-1].Cursor
		pageInfo.StartCursor = &start
		pageInfo.EndCursor = &end
	} else if nextCursor != "" {
		// Avoid trapping clients on an empty page when additional pages exist.
		c := model.Cursor(nextCursor)
		pageInfo.EndCursor = &c
	}

	return &model.ArticleConnection{
		Edges:      edges,
		PageInfo:   pageInfo,
		TotalCount: len(edges),
	}, nil
}

func (r *queryResolver) Series(ctx context.Context, id string) (*model.Series, error) {
	if err := r.requireCMSSeriesEnabled(); err != nil {
		return nil, err
	}

	store := r.cmsStorage()
	if store == nil || store.Series() == nil {
		return nil, ErrStorageUnavailable
	}

	authorID, seriesID, ok := parseSeriesGraphQLID(id)
	if !ok {
		return nil, errors.New("invalid series id")
	}

	series, err := store.Series().GetSeries(ctx, authorID, seriesID)
	if err != nil {
		return nil, err
	}

	return r.convertCMSSeries(ctx, series), nil
}

func (r *queryResolver) SeriesBySlug(ctx context.Context, slug string) (*model.Series, error) {
	if err := r.requireCMSSeriesEnabled(); err != nil {
		return nil, err
	}

	store := r.cmsStorage()
	if store == nil || store.Series() == nil {
		return nil, ErrStorageUnavailable
	}

	slug = cmsSlugify(slug)
	if strings.TrimSpace(slug) == "" {
		return nil, errors.New("slug is required")
	}

	// Preferred path: use a slug index to avoid scans at scale.
	var idx models.CMSSeriesSlugIndex
	err := store.GetDB().WithContext(ctx).Model(&models.CMSSeriesSlugIndex{}).
		Where("PK", "=", models.CMSSeriesSlugIndexPK(slug)).
		Where("SK", "=", models.CMSSeriesSlugIndexSK()).
		First(&idx)
	if err == nil && strings.TrimSpace(idx.AuthorID) != "" && strings.TrimSpace(idx.SeriesID) != "" {
		series, err := store.Series().GetSeries(ctx, idx.AuthorID, idx.SeriesID)
		if err == nil && series != nil {
			return r.convertCMSSeries(ctx, series), nil
		}
	}
	if err != nil && !dynamormerrors.IsNotFound(err) {
		return nil, err
	}

	// Backfill helper for instances that have legacy series rows without slug index entries.
	backfillIndex := func(item *models.Series) {
		if item == nil {
			return
		}
		idx := &models.CMSSeriesSlugIndex{
			Slug:     item.Slug,
			AuthorID: item.AuthorID,
			SeriesID: item.ID,
		}
		if err := idx.UpdateKeys(); err != nil {
			return
		}
		_ = store.GetDB().WithContext(ctx).Model(idx).IfNotExists().Create()
	}

	// Fast fallback: if authenticated, search within the viewer's series (no scan).
	if viewer := r.optionalAuth(ctx); viewer != "" {
		items, err := store.Series().ListSeriesByAuthor(ctx, viewer, 500)
		if err == nil {
			for _, item := range items {
				if item != nil && strings.EqualFold(item.Slug, slug) {
					backfillIndex(item)
					return r.convertCMSSeries(ctx, item), nil
				}
			}
		}
	}

	// Legacy fallback: scan for the first matching slug across all authors and backfill the index.
	var seriesModels []models.Series
	scanErr := store.GetDB().WithContext(ctx).Model(&models.Series{}).
		Where("SK", "BEGINS_WITH", "ID#").
		Limit(1000).
		All(&seriesModels)
	if scanErr != nil {
		return nil, scanErr
	}

	for i := range seriesModels {
		if strings.EqualFold(seriesModels[i].Slug, slug) {
			backfillIndex(&seriesModels[i])
			return r.convertCMSSeries(ctx, &seriesModels[i]), nil
		}
	}

	return nil, nil
}

func (r *queryResolver) AllSeries(ctx context.Context, authorID *string, first *int, after *model.Cursor) (*model.SeriesConnection, error) {
	if err := r.requireCMSSeriesEnabled(); err != nil {
		return nil, err
	}

	store := r.cmsStorage()
	if store == nil || store.Series() == nil {
		return nil, ErrStorageUnavailable
	}

	limit := clampCMSPageSize(first)
	cursor := ""
	if after != nil {
		cursor = string(*after)
	}

	username := ""
	if authorID != nil && strings.TrimSpace(*authorID) != "" {
		username = cmsNormalizeUsername(*authorID)
	}
	if username == "" {
		username = r.optionalAuth(ctx)
	}

	var items []*models.Series
	var (
		err        error
		nextCursor string
	)

	if username != "" {
		items, nextCursor, err = store.Series().ListSeriesByAuthorPaginated(ctx, username, limit, cursor)
		if err != nil {
			return nil, err
		}
	} else {
		var seriesModels []models.Series
		err = store.GetDB().WithContext(ctx).Model(&models.Series{}).
			Where("SK", "BEGINS_WITH", "ID#").
			Limit(limit).
			All(&seriesModels)
		if err != nil {
			return nil, err
		}
		items = make([]*models.Series, 0, len(seriesModels))
		for i := range seriesModels {
			items = append(items, &seriesModels[i])
		}
	}

	edges := make([]*model.SeriesEdge, 0, len(items))
	for _, item := range items {
		node := r.convertCMSSeries(ctx, item)
		if node == nil {
			continue
		}
		edgeCursor := model.Cursor(node.ID)
		if item != nil && strings.TrimSpace(item.SK) != "" {
			edgeCursor = model.Cursor(item.SK)
		}
		edges = append(edges, &model.SeriesEdge{
			Node:   node,
			Cursor: edgeCursor,
		})
	}

	pageInfo := &model.PageInfo{
		HasNextPage:     nextCursor != "",
		HasPreviousPage: after != nil,
	}
	if len(edges) > 0 {
		start := edges[0].Cursor
		end := edges[len(edges)-1].Cursor
		pageInfo.StartCursor = &start
		pageInfo.EndCursor = &end
	}

	return &model.SeriesConnection{
		Edges:      edges,
		PageInfo:   pageInfo,
		TotalCount: len(edges),
	}, nil
}

func (r *queryResolver) Category(ctx context.Context, id string) (*model.Category, error) {
	if err := r.requireCMSCategoriesEnabled(); err != nil {
		return nil, err
	}

	store := r.cmsStorage()
	if store == nil || store.Category() == nil {
		return nil, ErrStorageUnavailable
	}

	category, err := store.Category().GetCategory(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}

	return r.convertCMSCategory(ctx, category, true), nil
}

func (r *queryResolver) CategoryBySlug(ctx context.Context, slug string) (*model.Category, error) {
	category, err := cmsFetchBySlug(ctx, r, slug, (*Resolver).requireCMSCategoriesEnabled, models.CMSCategorySlugIndexPK, cmsCategoryGetter, cmsCategoryID, cmsCategoryIDFromStorage)
	if err != nil {
		return nil, err
	}
	return r.convertCMSCategory(ctx, category, true), nil
}

func (r *queryResolver) Categories(ctx context.Context, parentID *string) ([]*model.Category, error) {
	if err := r.requireCMSCategoriesEnabled(); err != nil {
		return nil, err
	}

	store := r.cmsStorage()
	if store == nil || store.Category() == nil {
		return nil, ErrStorageUnavailable
	}

	limit := 1000
	items, err := store.Category().ListCategories(ctx, parentID, limit)
	if err != nil {
		return nil, err
	}

	out := make([]*model.Category, 0, len(items))
	for _, item := range items {
		out = append(out, r.convertCMSCategory(ctx, item, false))
	}

	return out, nil
}

func (r *queryResolver) RootCategories(ctx context.Context) ([]*model.Category, error) {
	if err := r.requireCMSCategoriesEnabled(); err != nil {
		return nil, err
	}

	store := r.cmsStorage()
	if store == nil || store.Category() == nil {
		return nil, ErrStorageUnavailable
	}

	root := ""
	items, err := store.Category().ListCategories(ctx, &root, 500)
	if err != nil {
		return nil, err
	}

	out := make([]*model.Category, 0, len(items))
	for _, item := range items {
		out = append(out, r.convertCMSCategory(ctx, item, false))
	}

	return out, nil
}

func (r *queryResolver) Publication(ctx context.Context, id string) (*model.Publication, error) {
	if err := r.requireCMSLongFormEnabled(); err != nil {
		return nil, err
	}

	store := r.cmsStorage()
	if store == nil || store.Publication() == nil {
		return nil, ErrStorageUnavailable
	}

	pub, err := store.Publication().GetPublication(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}

	return r.convertCMSPublication(ctx, pub, true), nil
}

func (r *queryResolver) PublicationBySlug(ctx context.Context, slug string) (*model.Publication, error) {
	pub, err := cmsFetchBySlug(ctx, r, slug, (*Resolver).requireCMSLongFormEnabled, models.CMSPublicationSlugIndexPK, cmsPublicationGetter, cmsPublicationID, cmsPublicationIDFromStorage)
	if err != nil {
		return nil, err
	}
	return r.convertCMSPublication(ctx, pub, true), nil
}

func (r *queryResolver) MyPublications(ctx context.Context) ([]*model.Publication, error) {
	if err := r.requireCMSLongFormEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	store := r.cmsStorage()
	if store == nil || store.Publication() == nil || store.PublicationMember() == nil {
		return nil, ErrStorageUnavailable
	}

	membershipItems := []*models.PublicationMember{}
	if store.PublicationMember() != nil {
		items, _, err := store.PublicationMember().ListMembershipsForUserPaginated(ctx, username, 500, "")
		if err == nil {
			membershipItems = items
		}
	}

	// Back-compat fallback: scan membership items for this user.
	if len(membershipItems) == 0 {
		var members []models.PublicationMember
		err = store.GetDB().WithContext(ctx).Model(&models.PublicationMember{}).
			Where("SK", "=", fmt.Sprintf("USER#%s", username)).
			Limit(1000).
			All(&members)
		if err != nil {
			return nil, err
		}

		for i := range members {
			member := &members[i]
			membershipItems = append(membershipItems, member)

			// Best-effort self-heal: ensure GSI keys exist so future lookups use the index.
			if member.GSI1PK == "" || member.GSI1SK == "" {
				if err := member.UpdateKeys(); err == nil {
					_ = store.PublicationMember().Update(ctx, member)
				}
			}
		}
	}

	out := make([]*model.Publication, 0, len(membershipItems))
	for _, member := range membershipItems {
		pub, err := store.Publication().GetPublication(ctx, member.PublicationID)
		if err != nil {
			continue
		}
		out = append(out, r.convertCMSPublication(ctx, pub, false))
	}

	return out, nil
}
