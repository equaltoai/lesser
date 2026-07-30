package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/cms"
	storagecore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormcore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
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

func trimDraftReviewCursor(after *model.Cursor) string {
	if after == nil {
		return ""
	}
	return strings.TrimSpace(string(*after))
}

func cmsResolveBySlug[E any](
	ctx context.Context,
	db dynamormcore.DB,
	slug string,
	pk string,
	legacyPK string,
	tenant string,
	fetchByID func(string) (E, error),
	fetchLegacy func() (E, error),
	extractID func(E) string,
	belongsToTenant func(E, string) bool,
) (E, error) {
	var zero E

	tenant = strings.ToLower(strings.TrimSpace(tenant))
	if strings.TrimSpace(pk) != "" {
		entity, ok, err := cmsFetchEntityFromSlugIndex(ctx, db, pk, tenant, fetchByID, belongsToTenant)
		if err != nil {
			return zero, err
		}
		if ok {
			return entity, nil
		}
	}

	if tenant != "" && strings.TrimSpace(legacyPK) != "" && !strings.EqualFold(pk, legacyPK) {
		entity, ok, err := cmsFetchEntityFromSlugIndex(ctx, db, legacyPK, tenant, fetchByID, belongsToTenant)
		if err != nil {
			return zero, err
		}
		if ok {
			cmsBackfillSlugIndex(ctx, db, pk, slug, extractID(entity))
			return entity, nil
		}
	}

	entity, err := fetchLegacy()
	if err != nil {
		return zero, err
	}
	if tenant != "" && belongsToTenant != nil && !belongsToTenant(entity, tenant) {
		return zero, nil
	}

	cmsBackfillSlugIndex(ctx, db, pk, slug, extractID(entity))
	return entity, nil
}

func cmsFetchEntityFromSlugIndex[E any](
	ctx context.Context,
	db dynamormcore.DB,
	indexPK string,
	tenant string,
	fetchByID func(string) (E, error),
	belongsToTenant func(E, string) bool,
) (E, bool, error) {
	var zero E

	var idx models.CMSSlugIndex
	err := db.WithContext(ctx).Model(&models.CMSSlugIndex{}).
		Where("PK", "=", indexPK).
		Where("SK", "=", models.CMSSlugIndexSK()).
		First(&idx)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return zero, false, nil
		}
		return zero, false, err
	}

	targetID := strings.TrimSpace(idx.TargetID)
	if targetID != "" {
		entity, err := fetchByID(targetID)
		if err != nil {
			return zero, false, err
		}
		if tenant != "" && belongsToTenant != nil && !belongsToTenant(entity, tenant) {
			return zero, false, nil
		}
		return entity, true, nil
	}

	return zero, false, nil
}

func cmsBackfillSlugIndex(ctx context.Context, db dynamormcore.DB, pk string, slug string, targetID string) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return
	}
	backfill := &models.CMSSlugIndex{
		PK:       pk,
		Slug:     slug,
		TargetID: targetID,
	}
	if err := backfill.UpdateKeys(); err == nil {
		_ = db.WithContext(ctx).Model(backfill).IfNotExists().Create()
	}
}

func cmsStorageIDBelongsToTenant(id string, tenant string) bool {
	tenant = strings.ToLower(strings.TrimSpace(tenant))
	if tenant == "" {
		return true
	}
	host := cmsHostFromID(id)
	return host != "" && strings.EqualFold(host, tenant)
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
	scopedPK func(string, string) string,
	legacyPK func(string) string,
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
	tenant := strings.ToLower(strings.TrimSpace(domain))

	entity, err := cmsResolveBySlug(ctx, store.GetDB(), slug, scopedPK(tenant, slug),
		legacyPK(slug),
		tenant,
		func(id string) (E, error) { return getByID(ctx, id) },
		func() (E, error) { return getByID(ctx, legacy) },
		extractID,
		func(entity E, tenant string) bool {
			return cmsStorageIDBelongsToTenant(extractID(entity), tenant)
		},
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

func (r *queryResolver) DraftPreview(ctx context.Context, id string) (*model.DraftPreview, error) {
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

	draft, _, err := drafts.DraftReviewForCaller(ctx, username, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}

	rendered, renderErr := cms.RenderDraftPreview(draft)
	return r.convertCMSDraftPreview(draft, rendered, renderErr), nil
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
	cursor := trimDraftReviewCursor(after)

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
		HasPreviousPage: cursor != "",
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

	tenant := strings.ToLower(strings.TrimSpace(domain))
	fetchIndexedArticle := func(indexPK string) (*models.Article, bool, error) {
		var idx models.CMSSlugIndex
		err := store.GetDB().WithContext(ctx).Model(&models.CMSSlugIndex{}).
			Where("PK", "=", indexPK).
			Where("SK", "=", models.CMSSlugIndexSK()).
			First(&idx)
		if err != nil {
			if dynamormerrors.IsNotFound(err) {
				return nil, false, nil
			}
			return nil, false, err
		}
		targetID := strings.TrimSpace(idx.TargetID)
		if targetID == "" {
			return nil, false, nil
		}
		article, err := store.Article().GetArticle(ctx, targetID)
		if err != nil {
			return nil, false, err
		}
		if article == nil {
			return nil, false, nil
		}
		if !cmsStorageIDBelongsToTenant(article.ID, tenant) {
			return nil, false, nil
		}
		return article, true, nil
	}

	// Preferred path: use a tenant-scoped slug index so IDs are stable and
	// slugs are editable without exposing another tenant's same-slug object.
	if article, ok, err := fetchIndexedArticle(models.CMSTenantArticleSlugIndexPK(tenant, slug)); err != nil {
		return nil, err
	} else if ok {
		return r.convertCMSArticle(ctx, article, true), nil
	}

	// Back-compat path: read the legacy global index only after verifying that
	// the resolved target belongs to this resolver's tenant, then backfill.
	if article, ok, err := fetchIndexedArticle(models.CMSArticleSlugIndexPK(slug)); err != nil {
		return nil, err
	} else if ok {
		backfill := &models.CMSSlugIndex{
			PK:       models.CMSTenantArticleSlugIndexPK(tenant, slug),
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

	legacyID := cmsArticleID(domain, slug)
	article, err := store.Article().GetArticle(ctx, legacyID)
	if err != nil {
		return nil, err
	}

	// Best-effort backfill for legacy slug-derived IDs.
	backfill := &models.CMSSlugIndex{
		PK:       models.CMSTenantArticleSlugIndexPK(tenant, slug),
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
	tenant := strings.ToLower(strings.TrimSpace(r.getDomain()))

	// Preferred path: use a tenant-scoped slug index to avoid scans at scale.
	if series, ok, err := cmsFetchSeriesByIndex(ctx, store, models.CMSTenantSeriesSlugIndexPK(tenant, slug), tenant); err != nil {
		return nil, err
	} else if ok {
		return r.convertCMSSeries(ctx, series), nil
	}

	// Back-compat path: read the legacy global index, then backfill the scoped
	// index after the resolved series is accepted for this tenant.
	if series, ok, err := cmsFetchSeriesByIndex(ctx, store, models.CMSSeriesSlugIndexPK(slug), tenant); err != nil {
		return nil, err
	} else if ok {
		backfillSeriesSlugIndex(ctx, store.GetDB(), tenant, series)
		return r.convertCMSSeries(ctx, series), nil
	}

	// Fast fallback: if authenticated, search within the viewer's series (no scan).
	if viewer := r.optionalAuth(ctx); viewer != "" {
		series, ok, err := cmsFindViewerSeriesBySlug(ctx, store, viewer, slug, tenant)
		if err == nil && ok {
			backfillSeriesSlugIndex(ctx, store.GetDB(), tenant, series)
			return r.convertCMSSeries(ctx, series), nil
		}
	}

	// Legacy fallback: scan for the first matching slug across all authors and backfill the index.
	series, ok, err := cmsScanSeriesBySlug(ctx, store, slug, tenant)
	if err != nil {
		return nil, err
	}
	if ok {
		backfillSeriesSlugIndex(ctx, store.GetDB(), tenant, series)
		return r.convertCMSSeries(ctx, series), nil
	}

	return nil, nil
}

func cmsFetchSeriesByIndex(ctx context.Context, store storagecore.RepositoryStorage, indexPK string, tenant string) (*models.Series, bool, error) {
	var idx models.CMSSeriesSlugIndex
	err := store.GetDB().WithContext(ctx).Model(&models.CMSSeriesSlugIndex{}).
		Where("PK", "=", indexPK).
		Where("SK", "=", models.CMSSeriesSlugIndexSK()).
		First(&idx)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if strings.TrimSpace(idx.AuthorID) == "" || strings.TrimSpace(idx.SeriesID) == "" {
		return nil, false, nil
	}
	series, err := store.Series().GetSeries(ctx, idx.AuthorID, idx.SeriesID)
	if err != nil {
		return nil, false, err
	}
	if series == nil || !cmsSeriesBelongsToTenant(series, tenant) {
		return nil, false, nil
	}
	return series, true, nil
}

func cmsFindViewerSeriesBySlug(ctx context.Context, store storagecore.RepositoryStorage, viewer string, slug string, tenant string) (*models.Series, bool, error) {
	items, err := store.Series().ListSeriesByAuthor(ctx, viewer, 500)
	if err != nil {
		return nil, false, err
	}
	for _, item := range items {
		if item != nil && strings.EqualFold(item.Slug, slug) && cmsSeriesBelongsToTenant(item, tenant) {
			return item, true, nil
		}
	}
	return nil, false, nil
}

func cmsScanSeriesBySlug(ctx context.Context, store storagecore.RepositoryStorage, slug string, tenant string) (*models.Series, bool, error) {
	var seriesModels []models.Series
	scanErr := store.GetDB().WithContext(ctx).Model(&models.Series{}).
		Where("SK", "BEGINS_WITH", "ID#").
		Limit(1000).
		All(&seriesModels)
	if scanErr != nil {
		return nil, false, scanErr
	}

	for i := range seriesModels {
		if strings.EqualFold(seriesModels[i].Slug, slug) && cmsSeriesBelongsToTenant(&seriesModels[i], tenant) {
			return &seriesModels[i], true, nil
		}
	}

	return nil, false, nil
}

func cmsSeriesBelongsToTenant(series *models.Series, tenant string) bool {
	if series == nil {
		return false
	}
	tenant = strings.ToLower(strings.TrimSpace(tenant))
	if tenant == "" {
		return true
	}
	storedTenant := strings.ToLower(strings.TrimSpace(series.Tenant))
	// Legacy series rows created before tenant stamping have no tenant value.
	// Keep them readable/mutable for compatibility; all new rows are stamped
	// and must match the current tenant before slug lookup or direct mutation.
	return storedTenant == "" || strings.EqualFold(storedTenant, tenant)
}

func backfillSeriesSlugIndex(ctx context.Context, db dynamormcore.DB, tenant string, item *models.Series) {
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
	tenant = strings.ToLower(strings.TrimSpace(tenant))
	if tenant != "" {
		idx.PK = models.CMSTenantSeriesSlugIndexPK(tenant, item.Slug)
	}
	_ = db.WithContext(ctx).Model(idx).IfNotExists().Create()
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
	category, err := cmsFetchBySlug(ctx, r, slug, (*Resolver).requireCMSCategoriesEnabled, models.CMSTenantCategorySlugIndexPK, models.CMSCategorySlugIndexPK, cmsCategoryGetter, cmsCategoryID, cmsCategoryIDFromStorage)
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
	pub, err := cmsFetchBySlug(ctx, r, slug, (*Resolver).requireCMSLongFormEnabled, models.CMSTenantPublicationSlugIndexPK, models.CMSPublicationSlugIndexPK, cmsPublicationGetter, cmsPublicationID, cmsPublicationIDFromStorage)
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
			membershipItems = append(membershipItems, &members[i])
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

func (r *queryResolver) SharedDraftReviews(ctx context.Context, first *int, after *model.Cursor) (*model.DraftReviewConnection, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return nil, err
	}
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	svc := r.Registry.Drafts()
	if svc == nil {
		return nil, errors.New("draft service is not available")
	}
	cursor := trimDraftReviewCursor(after)
	grants, nextCursor, err := svc.SharedDraftReviews(ctx, username, clampCMSPageSize(first), cursor)
	if err != nil {
		return nil, err
	}
	totalCount, err := svc.CountSharedDraftReviews(ctx, username)
	if err != nil {
		return nil, err
	}
	edges := make([]*model.DraftReviewEdge, 0, len(grants))
	for _, g := range grants {
		d, e := svc.GetDraft(ctx, g.OwnerID, g.DraftID)
		if e != nil {
			return nil, e
		}
		vs, e := svc.DraftReviewVerdicts(ctx, g.OwnerID, g.DraftID)
		if e != nil {
			return nil, e
		}
		edges = append(edges, &model.DraftReviewEdge{Node: r.convertCMSDraftReview(ctx, d, g, vs), Cursor: model.Cursor(g.GSI2SK)})
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
	}
	return &model.DraftReviewConnection{Edges: edges, PageInfo: pageInfo, TotalCount: totalCount}, nil
}
func (r *queryResolver) DraftReview(ctx context.Context, id string) (*model.DraftReview, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return nil, err
	}
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	svc := r.Registry.Drafts()
	if svc == nil {
		return nil, errors.New("draft service is not available")
	}
	d, g, err := svc.DraftReviewForCaller(ctx, username, id)
	if err != nil {
		return nil, err
	}
	vs, err := svc.DraftReviewVerdicts(ctx, d.AuthorID, d.ID)
	if err != nil {
		return nil, err
	}
	return r.convertCMSDraftReview(ctx, d, g, vs), nil
}
