package graph

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

type cmsArticleListFn func(cursor string, limit int) ([]*models.Article, string, error)
type cmsArticleEdgeCursorFn func(article *models.Article, fallback string) model.Cursor

func cmsIndexArticleCursor(article *models.Article, fallback string) model.Cursor {
	if article != nil {
		if value := models.CMSArticleIndexSK(article.Published, article.ID); strings.TrimSpace(value) != "" {
			return model.Cursor(value)
		}
	}
	return model.Cursor(fallback)
}

func (r *queryResolver) cmsArticlesListStrategy(ctx context.Context, store core.RepositoryStorage, authorFilter, seriesFilter, categoryFilter string) (cmsArticleListFn, cmsArticleEdgeCursorFn) {
	switch {
	case strings.TrimSpace(seriesFilter) != "":
		return func(cur string, l int) ([]*models.Article, string, error) {
			return store.Article().ListArticlesBySeriesPaginated(ctx, seriesFilter, l, cur)
		}, cmsIndexArticleCursor
	case strings.TrimSpace(categoryFilter) != "":
		return func(cur string, l int) ([]*models.Article, string, error) {
			return store.Article().ListArticlesByCategoryPaginated(ctx, categoryFilter, l, cur)
		}, cmsIndexArticleCursor
	}

	authorFilter = strings.TrimSpace(authorFilter)
	authorActorID := ""
	if authorFilter != "" {
		if strings.Contains(authorFilter, "://") {
			authorActorID = authorFilter
		} else {
			authorActorID = cmsLocalActorID(r.getDomain(), cmsNormalizeUsername(authorFilter))
		}
	}

	if authorActorID != "" {
		return func(cur string, l int) ([]*models.Article, string, error) {
			return store.Article().ListArticlesByAuthorPaginated(ctx, authorActorID, l, cur)
		}, cmsIndexArticleCursor
	}

	return func(cur string, l int) ([]*models.Article, string, error) {
		return store.Article().ListArticlesPaginated(ctx, l, cur)
	}, cmsArticleCursor
}

func (r *queryResolver) cmsCollectArticleEdges(
	ctx context.Context,
	list cmsArticleListFn,
	cursorFn cmsArticleEdgeCursorFn,
	authorFilter, seriesFilter, categoryFilter string,
	limit int,
	cursor string,
) ([]*model.ArticleEdge, bool, string, error) {
	if limit <= 0 {
		limit = defaultCMSPageSize
	}

	if limit > maxCMSPageSize {
		limit = maxCMSPageSize
	}

	fetchLimit := limit

	edges := make([]*model.ArticleEdge, 0, maxCMSPageSize)
	nextCursor := cursor
	hasMore := false

	for attempts := 0; attempts < 5 && len(edges) < limit; attempts++ {
		items, cursorOut, err := list(nextCursor, fetchLimit)
		if err != nil {
			return nil, false, "", err
		}

		stoppedEarly := false
		for i, article := range items {
			if !cmsArticleMatchesFilters(article, authorFilter, seriesFilter, categoryFilter) {
				continue
			}
			node := r.convertCMSArticle(ctx, article, false)
			if node == nil {
				continue
			}
			edges = append(edges, &model.ArticleEdge{
				Node:   node,
				Cursor: cursorFn(article, node.ID),
			})
			if len(edges) == limit {
				stoppedEarly = i < len(items)-1
				break
			}
		}

		nextCursor = cursorOut
		hasMore = stoppedEarly || nextCursor != ""
		if nextCursor == "" || len(edges) == limit {
			break
		}
	}

	return edges, hasMore, nextCursor, nil
}
