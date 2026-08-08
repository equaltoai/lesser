package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/cms"
	"github.com/equaltoai/lesser/pkg/storage"
	storagecore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormcore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	dynamormquery "github.com/theory-cloud/tabletheory/v3/pkg/query"
)

const (
	defaultCMSPageSize = 25
	maxCMSPageSize     = 200
)

type cmsDraftLister interface {
	ListDraftsByAuthorPaginated(context.Context, string, int, string) ([]*models.Draft, string, error)
}

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

// cmsArticleMatchesSearch performs the intentionally simple public article
// search promised by the GraphQL contract. Keep this list limited to fields
// already exposed on the public Article type: editorial workflow metadata must
// never influence anonymous search results.
func cmsArticleMatchesSearch(article *models.Article, search string) bool {
	if article == nil {
		return false
	}

	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return true
	}

	publicText := []string{
		article.Name,
		article.Summary,
		article.Content,
		article.Slug,
		article.Subtitle,
		article.Excerpt,
		article.SEOTitle,
		article.SEODescription,
	}
	for _, value := range publicText {
		if strings.Contains(strings.ToLower(value), search) {
			return true
		}
	}

	return false
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

	cursor := trimDraftReviewCursor(after)
	items, err := listAllCMSDraftsByAuthor(ctx, store.Draft(), username)
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
	totalCount := len(filtered)
	page, hasNextPage := paginateCMSDrafts(filtered, clampCMSPageSize(first), cursor)

	edges := make([]*model.DraftEdge, 0, len(page))
	for _, draft := range page {
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
		HasNextPage:     hasNextPage,
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
		TotalCount: totalCount,
	}, nil
}

func listAllCMSDraftsByAuthor(ctx context.Context, repo cmsDraftLister, username string) ([]*models.Draft, error) {
	items := make([]*models.Draft, 0)
	cursor := ""
	for {
		page, nextCursor, err := repo.ListDraftsByAuthorPaginated(ctx, username, maxCMSPageSize, cursor)
		if err != nil {
			return nil, err
		}
		items = append(items, page...)
		if nextCursor == "" {
			return items, nil
		}
		if nextCursor == cursor {
			return nil, errors.New("draft pagination did not advance")
		}
		cursor = nextCursor
	}
}

func paginateCMSDrafts(items []*models.Draft, limit int, cursor string) ([]*models.Draft, bool) {
	cursor = strings.TrimSpace(cursor)
	if cursor != "" && !strings.HasPrefix(cursor, "ID#") {
		cursor = "ID#" + cursor
	}
	start := 0
	for start < len(items) && draftCursorKey(items[start]) <= cursor {
		start++
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], end < len(items)
}

func draftCursorKey(draft *models.Draft) string {
	if draft == nil {
		return ""
	}
	if key := strings.TrimSpace(draft.SK); key != "" {
		return key
	}
	return "ID#" + strings.TrimSpace(draft.ID)
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
		if cmsArticleNotFound(err) {
			return r.deletedCMSArticle(ctx, strings.TrimSpace(id))
		}
		return nil, err
	}

	return r.convertCMSArticle(ctx, article, true), nil
}

func (r *queryResolver) deletedCMSArticle(ctx context.Context, id string) (*model.Article, error) {
	store := r.cmsStorage()
	if store == nil || store.GetDB() == nil {
		return nil, ErrStorageUnavailable
	}

	var tombstone models.Tombstone
	err := store.GetDB().WithContext(ctx).Model(&models.Tombstone{}).
		Where("PK", "=", "OBJECT#"+strings.TrimSpace(id)).
		Where("SK", "=", "TOMBSTONE").
		First(&tombstone)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(tombstone.FormerType), "Article") || !r.cmsArticleTombstoneVisible(ctx, &tombstone) {
		return nil, nil
	}

	deletedAt := model.Time(tombstone.Deleted)
	createdAt := tombstone.CreatedAt
	if createdAt.IsZero() {
		createdAt = tombstone.Deleted
	}
	authorID := strings.TrimSpace(tombstone.AttributedTo)
	if authorID == "" {
		authorID = strings.TrimSpace(tombstone.DeletedBy)
	}

	return &model.Article{
		ID:              tombstone.ID,
		DeletedAt:       &deletedAt,
		Slug:            cmsExtractSlugFromURL(tombstone.ID),
		AuthorID:        authorID,
		Author:          r.resolveActorByID(ctx, authorID),
		ContentFormat:   model.ContentFormatHTML,
		TableOfContents: []*model.TOCEntry{},
		Categories:      []*model.Category{},
		PublishedAt:     deletedAt,
		CreatedAt:       model.Time(createdAt),
		UpdatedAt:       deletedAt,
	}, nil
}

func (r *queryResolver) cmsArticleTombstoneVisible(ctx context.Context, tombstone *models.Tombstone) bool {
	if tombstone == nil {
		return false
	}
	if tombstone.IsPublic {
		return true
	}
	attributedTo := strings.TrimSpace(tombstone.AttributedTo)
	if attributedTo == "" {
		return false
	}

	username := strings.TrimSpace(getUsernameFromContext(ctx))
	if username == "" {
		return false
	}
	viewerActorID := cmsLocalActorID(r.getDomain(), username)
	return cmsArticleTombstoneVisibleToActor(attributedTo, viewerActorID)
}

func cmsArticleTombstoneVisibleToActor(attributedTo, viewerActorID string) bool {
	attributedTo = strings.TrimSpace(attributedTo)
	// Defense-in-depth: an unattributed tombstone must never become visible if
	// an identity resolver can also return an empty actor ID in the future.
	if attributedTo == "" {
		return false
	}
	viewerActorID = strings.TrimSpace(viewerActorID)
	return strings.EqualFold(viewerActorID, attributedTo)
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

	// Preferred path: use a tenant-scoped slug index so IDs are stable and
	// slugs are editable without exposing another tenant's same-slug object.
	if article, targetID, ok, err := cmsIndexedArticle(ctx, store, tenant, models.CMSTenantArticleSlugIndexPK(tenant, slug)); err != nil {
		return nil, err
	} else if ok {
		if article == nil {
			return r.deletedCMSArticle(ctx, targetID)
		}
		return r.convertCMSArticle(ctx, article, true), nil
	}

	// Back-compat path: read the legacy global index only after verifying that
	// the resolved target belongs to this resolver's tenant, then backfill.
	if article, targetID, ok, err := cmsIndexedArticle(ctx, store, tenant, models.CMSArticleSlugIndexPK(slug)); err != nil {
		return nil, err
	} else if ok {
		if article == nil {
			return r.deletedCMSArticle(ctx, targetID)
		}
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

	return r.cmsArticleByLegacySlug(ctx, store, domain, tenant, slug)
}

func cmsIndexedArticle(ctx context.Context, store storagecore.RepositoryStorage, tenant, indexPK string) (*models.Article, string, bool, error) {
	var idx models.CMSSlugIndex
	err := store.GetDB().WithContext(ctx).Model(&models.CMSSlugIndex{}).
		Where("PK", "=", indexPK).
		Where("SK", "=", models.CMSSlugIndexSK()).
		First(&idx)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return nil, "", false, nil
		}
		return nil, "", false, err
	}

	targetID := strings.TrimSpace(idx.TargetID)
	if targetID == "" || !cmsStorageIDBelongsToTenant(targetID, tenant) {
		return nil, "", false, nil
	}

	article, err := store.Article().GetArticle(ctx, targetID)
	if err != nil {
		if cmsArticleNotFound(err) {
			return nil, targetID, true, nil
		}
		return nil, "", false, err
	}
	if article == nil {
		return nil, targetID, true, nil
	}
	if !cmsStorageIDBelongsToTenant(article.ID, tenant) {
		return nil, "", false, nil
	}
	return article, targetID, true, nil
}

func (r *queryResolver) cmsArticleByLegacySlug(ctx context.Context, store storagecore.RepositoryStorage, domain, tenant, slug string) (*model.Article, error) {
	legacyID := cmsArticleID(domain, slug)
	article, err := store.Article().GetArticle(ctx, legacyID)
	if err != nil {
		if cmsArticleNotFound(err) {
			return r.deletedCMSArticle(ctx, legacyID)
		}
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

func cmsArticleNotFound(err error) bool {
	return common.IsNotFound(err) || errors.Is(err, storage.ErrNotFound) || dynamormerrors.IsNotFound(err)
}

func (r *queryResolver) Articles(ctx context.Context, authorID *string, seriesID *string, categoryID *string, search *string, first *int, after *model.Cursor) (*model.ArticleConnection, error) {
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
	searchFilter := trimStringPtr(search)

	if seriesFilter != "" && !r.cmsSeriesEnabled() {
		return nil, errCMSSeriesDisabled
	}
	if categoryFilter != "" && !r.cmsCategoriesEnabled() {
		return nil, errCMSCategoriesDisabled
	}

	list, cursorFn := r.cmsArticlesListStrategy(ctx, store, authorFilter, seriesFilter, categoryFilter)
	edges, hasMore, nextCursor, err := r.cmsCollectArticleEdges(ctx, list, cursorFn, authorFilter, seriesFilter, categoryFilter, searchFilter, limit, cursor)
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
		authorCursor, cursorErr := cmsSeriesAuthorCursor(cursor, username)
		if cursorErr != nil {
			return nil, cursorErr
		}
		items, nextCursor, err = store.Series().ListSeriesByAuthorPaginated(ctx, username, limit, authorCursor)
		if err != nil {
			return nil, err
		}
		if nextCursor != "" {
			encodedCursor, cursorErr := cmsSeriesEdgeCursor(&models.Series{
				AuthorID: username,
				SK:       strings.TrimSpace(nextCursor),
			})
			if cursorErr != nil {
				return nil, cursorErr
			}
			nextCursor = string(encodedCursor)
		}
	} else {
		items, nextCursor, err = listGlobalSeriesPaginated(ctx, store.GetDB(), limit, cursor)
		if err != nil {
			return nil, err
		}
	}

	edges := make([]*model.SeriesEdge, 0, len(items))
	for _, item := range items {
		node := r.convertCMSSeries(ctx, item)
		if node == nil {
			continue
		}
		edgeCursor, cursorErr := cmsSeriesEdgeCursor(item)
		if cursorErr != nil {
			return nil, cursorErr
		}
		edges = append(edges, &model.SeriesEdge{
			Node:   node,
			Cursor: edgeCursor,
		})
	}
	if nextCursor != "" && len(edges) > 0 {
		edges[len(edges)-1].Cursor = model.Cursor(nextCursor)
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

func cmsSeriesEdgeCursor(item *models.Series) (model.Cursor, error) {
	if item == nil {
		return "", errors.New("series is required to build cursor")
	}

	pk := strings.TrimSpace(item.PK)
	if pk == "" && strings.TrimSpace(item.AuthorID) != "" {
		pk = fmt.Sprintf("AUTHOR#%s#SERIES", strings.TrimSpace(item.AuthorID))
	}
	sk := strings.TrimSpace(item.SK)
	if sk == "" && strings.TrimSpace(item.ID) != "" {
		sk = "ID#" + strings.TrimSpace(item.ID)
	}
	if pk == "" || sk == "" {
		return "", errors.New("series keys are required to build cursor")
	}

	encoded, err := dynamormquery.EncodeCursor(map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: pk},
		"SK": &types.AttributeValueMemberS{Value: sk},
	}, "", "")
	if err != nil {
		return "", fmt.Errorf("encode series cursor: %w", err)
	}
	return model.Cursor(encoded), nil
}

func cmsSeriesAuthorCursor(cursor, authorID string) (string, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return "", nil
	}

	decoded, err := dynamormquery.DecodeCursor(cursor)
	if err != nil {
		return "", ErrInvalidAfterCursorWithContext(fmt.Errorf("decode series cursor: %w", err))
	}
	key, err := decoded.ToAttributeValues()
	if err != nil {
		return "", ErrInvalidAfterCursorWithContext(fmt.Errorf("decode series cursor key: %w", err))
	}

	expectedPK := fmt.Sprintf("AUTHOR#%s#SERIES", strings.TrimSpace(authorID))
	pk, ok := key["PK"].(*types.AttributeValueMemberS)
	if !ok || strings.TrimSpace(pk.Value) != expectedPK {
		return "", ErrInvalidAfterCursorWithContext(errors.New("series cursor does not match author partition"))
	}
	sk, ok := key["SK"].(*types.AttributeValueMemberS)
	if !ok || strings.TrimSpace(sk.Value) == "" {
		return "", ErrInvalidAfterCursorWithContext(errors.New("series cursor is missing its sort key"))
	}
	return strings.TrimSpace(sk.Value), nil
}

func listGlobalSeriesPaginated(ctx context.Context, db dynamormcore.DB, limit int, cursor string) ([]*models.Series, string, error) {
	query := db.WithContext(ctx).Model(&models.Series{}).
		Where("SK", "BEGINS_WITH", "ID#").
		Limit(limit)
	if strings.TrimSpace(cursor) != "" {
		query = query.Cursor(strings.TrimSpace(cursor))
	}

	var seriesModels []models.Series
	page, err := query.AllPaginated(&seriesModels)
	if err != nil {
		return nil, "", err
	}

	items := make([]*models.Series, 0, len(seriesModels))
	for i := range seriesModels {
		items = append(items, &seriesModels[i])
	}
	nextCursor := ""
	if page != nil {
		nextCursor = page.NextCursor
	}
	return items, nextCursor, nil
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

func (r *queryResolver) MyDraftReviews(ctx context.Context, first *int, after *model.Cursor) (*model.DraftReviewConnection, error) {
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
	grants, err := svc.OwnedDraftReviews(ctx, username)
	if err != nil {
		return nil, err
	}
	resolved, err := r.resolveDraftReviewGrants(ctx, username, grants)
	if err != nil {
		return nil, err
	}
	cursor := trimDraftReviewCursor(after)
	page, hasNextPage := paginateResolvedDraftReviews(resolved, clampCMSPageSize(first), cursor)
	edges := make([]*model.DraftReviewEdge, 0, len(page))
	for _, review := range page {
		verdicts, verdictErr := svc.DraftReviewVerdicts(ctx, username, review.grant.DraftID)
		if verdictErr != nil {
			return nil, verdictErr
		}
		node, buildErr := r.buildCMSDraftReview(ctx, review.draft, review.grant, verdicts)
		if buildErr != nil {
			return nil, buildErr
		}
		edges = append(edges, &model.DraftReviewEdge{
			Node:   node,
			Cursor: model.Cursor(draftReviewGrantCursorKey(review.grant)),
		})
	}
	pageInfo := &model.PageInfo{HasNextPage: hasNextPage, HasPreviousPage: cursor != ""}
	if len(edges) > 0 {
		start := edges[0].Cursor
		end := edges[len(edges)-1].Cursor
		pageInfo.StartCursor = &start
		pageInfo.EndCursor = &end
	}
	return &model.DraftReviewConnection{Edges: edges, PageInfo: pageInfo, TotalCount: len(resolved)}, nil
}

type resolvedDraftReview struct {
	grant *models.DraftReviewGrant
	draft *models.Draft
}

func draftReviewDraftMissing(err error) bool {
	return errors.Is(err, storage.ErrNotFound) || apperrors.HasCode(err, apperrors.CodeNotFound)
}

func (r *queryResolver) resolveDraftReviewGrants(ctx context.Context, owner string, grants []*models.DraftReviewGrant) ([]resolvedDraftReview, error) {
	resolved := make([]resolvedDraftReview, 0, len(grants))
	for _, grant := range grants {
		if grant == nil {
			continue
		}
		draftOwner := strings.TrimSpace(owner)
		if draftOwner == "" {
			draftOwner = strings.TrimSpace(grant.OwnerID)
		}
		draft, err := r.Registry.Drafts().GetDraft(ctx, draftOwner, grant.DraftID)
		if draftReviewDraftMissing(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, resolvedDraftReview{grant: grant, draft: draft})
	}
	return resolved, nil
}

func paginateResolvedDraftReviews(reviews []resolvedDraftReview, limit int, cursor string) ([]resolvedDraftReview, bool) {
	cursor = strings.TrimSpace(cursor)
	start := 0
	if cursor != "" {
		for start < len(reviews) {
			key := draftReviewGrantCursorKey(reviews[start].grant)
			if key == cursor || key > cursor {
				if key == cursor {
					start++
				}
				break
			}
			start++
		}
	}
	end := start + limit
	if end > len(reviews) {
		end = len(reviews)
	}
	return reviews[start:end], end < len(reviews)
}

func sharedDraftReviewCursorKey(grant *models.DraftReviewGrant) string {
	if grant == nil {
		return ""
	}
	if key := strings.TrimSpace(grant.GSI2SK); key != "" {
		return key
	}
	return draftReviewGrantCursorKey(grant)
}

func draftReviewGrantCursorKey(grant *models.DraftReviewGrant) string {
	if grant == nil {
		return ""
	}
	if key := strings.TrimSpace(grant.SK); key != "" {
		return key
	}
	return fmt.Sprintf("GRANT#%s#REVIEWER#%s", strings.TrimSpace(grant.DraftID), strings.TrimSpace(grant.Reviewer))
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
	resolved, err := r.resolveDraftReviewGrants(ctx, "", grants)
	if err != nil {
		return nil, err
	}
	edges := make([]*model.DraftReviewEdge, 0, len(resolved))
	for _, review := range resolved {
		vs, e := svc.DraftReviewVerdicts(ctx, review.grant.OwnerID, review.grant.DraftID)
		if e != nil {
			return nil, e
		}
		node, buildErr := r.buildCMSDraftReview(ctx, review.draft, review.grant, vs)
		if buildErr != nil {
			return nil, buildErr
		}
		edges = append(edges, &model.DraftReviewEdge{Node: node, Cursor: model.Cursor(sharedDraftReviewCursorKey(review.grant))})
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
	totalCount, err := svc.CountSharedDraftReviews(ctx, username)
	if err != nil {
		return nil, err
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
	return r.buildCMSDraftReview(ctx, d, g, vs)
}
