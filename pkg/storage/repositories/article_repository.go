package repositories

import (
	"context"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

// ArticleRepository implements article operations using DynamORM with EnhancedBaseRepository
type ArticleRepository struct {
	*EnhancedBaseRepository[*models.Article]
}

// NewArticleRepository creates a new article repository
func NewArticleRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *ArticleRepository {
	enhancedRepo := NewEnhancedBaseRepository[*models.Article](db, tableName, logger, costService, "ArticleRepository", "article")

	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService())
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &ArticleRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreateArticle creates a new article
func (r *ArticleRepository) CreateArticle(ctx context.Context, article *models.Article) error {
	// 1. Validate required fields
	if r.validator != nil {
		if err := r.validator.ValidateRequiredFields(ctx, article); err != nil {
			return errors.ValidationFailed("required fields", err.Error())
		}

		// 2. Validate business rules
		if err := r.validator.ValidateBusinessRules(ctx, article, "create"); err != nil {
			return errors.ValidationFailed("business rules", err.Error())
		}
	}

	// 3. Check permissions
	if err := r.checkCreatePermissions(ctx, article); err != nil {
		return err
	}

	// 4. Execute create with conditional check to prevent overwriting existing articles.
	if err := r.CreateIfNotExists(ctx, article); err != nil {
		return err
	}

	// 5. Emit creation event
	if r.events != nil {
		event := NewEvent("entity.created", r.entityName, article.GetPK(), "create", article)
		event.Actor = r.getActorFromContext(ctx)
		_ = r.events.Emit(ctx, event)
	}

	return nil
}

// GetArticle retrieves an article by ID
func (r *ArticleRepository) GetArticle(ctx context.Context, id string) (*models.Article, error) {
	var article models.Article
	pk := fmt.Sprintf("object#%s", id)
	sk := pk

	err := r.Get(ctx, pk, sk, &article)
	if err != nil {
		return nil, err
	}
	return &article, nil
}

// UpdateArticle updates an existing article
func (r *ArticleRepository) UpdateArticle(ctx context.Context, article *models.Article) error {
	// Ensure keys are set
	if err := article.UpdateKeys(); err != nil {
		return err
	}

	// Use UpdateBuilder for partial updates or full update via Save if appropriate
	// Here we use Save for simplicity as it handles optimistic locking if version is present
	return r.db.WithContext(ctx).Model(article).Update()
}

// DeleteArticle deletes an article
func (r *ArticleRepository) DeleteArticle(ctx context.Context, id string) error {
	pk := fmt.Sprintf("object#%s", id)
	sk := pk
	return r.Delete(ctx, pk, sk)
}

// ListArticles lists articles with pagination
func (r *ArticleRepository) ListArticles(ctx context.Context, limit int) ([]*models.Article, error) {
	articles, _, err := r.ListArticlesPaginated(ctx, limit, "")
	return articles, err
}

// ListArticlesPaginated lists articles ordered by published time (newest first).
// Cursor values are gsi2SK values.
func (r *ArticleRepository) ListArticlesPaginated(ctx context.Context, limit int, cursor string) ([]*models.Article, string, error) {
	if limit <= 0 {
		limit = 25
	}

	query := r.db.WithContext(ctx).Model(&models.Article{}).
		Index("gsi2").
		Where("gsi2PK", "=", "object#type#Article").
		OrderBy("gsi2SK", "DESC")

	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		query = query.Where("gsi2SK", "<", cursor)
	}

	query = query.Limit(limit + 1)

	var articleModels []models.Article
	if err := query.All(&articleModels); err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if err := common.ValidateSliceLength("articles", articleModels, limit); err != nil {
		nextCursor = articleModels[limit-1].GSI2SK
		articleModels = articleModels[:limit]
	}

	result := make([]*models.Article, len(articleModels))
	for i := range articleModels {
		result[i] = &articleModels[i]
	}

	return result, nextCursor, nil
}

// ListArticlesByAuthorPaginated lists articles for a specific author (actor ID) ordered by published time (newest first).
// Cursor values are CMS index SK values.
func (r *ArticleRepository) ListArticlesByAuthorPaginated(ctx context.Context, authorActorID string, limit int, cursor string) ([]*models.Article, string, error) {
	pk := models.CMSArticleIndexPKForAuthor(authorActorID)
	if strings.TrimSpace(pk) == "" {
		return []*models.Article{}, "", nil
	}
	return r.listArticlesByCMSIndexPaginated(ctx, pk, limit, cursor)
}

// ListArticlesBySeriesPaginated lists articles for a specific series (GraphQL series ID) ordered by published time (newest first).
// Cursor values are CMS index SK values.
func (r *ArticleRepository) ListArticlesBySeriesPaginated(ctx context.Context, seriesID string, limit int, cursor string) ([]*models.Article, string, error) {
	pk := models.CMSArticleIndexPKForSeries(seriesID)
	if strings.TrimSpace(pk) == "" {
		return []*models.Article{}, "", nil
	}
	return r.listArticlesByCMSIndexPaginated(ctx, pk, limit, cursor)
}

// ListArticlesByCategoryPaginated lists articles for a specific category ordered by published time (newest first).
// Cursor values are CMS index SK values.
func (r *ArticleRepository) ListArticlesByCategoryPaginated(ctx context.Context, categoryID string, limit int, cursor string) ([]*models.Article, string, error) {
	pk := models.CMSArticleIndexPKForCategory(categoryID)
	if strings.TrimSpace(pk) == "" {
		return []*models.Article{}, "", nil
	}
	return r.listArticlesByCMSIndexPaginated(ctx, pk, limit, cursor)
}

func (r *ArticleRepository) listArticlesByCMSIndexPaginated(ctx context.Context, pk string, limit int, cursor string) ([]*models.Article, string, error) {
	if limit <= 0 {
		limit = 25
	}

	query := r.db.WithContext(ctx).Model(&models.CMSArticleIndex{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", models.CMSArticleIndexSKPrefix).
		OrderBy("SK", "DESC")

	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		query = query.Where("SK", "<", cursor)
	}

	query = query.Limit(limit + 1)

	var indexModels []models.CMSArticleIndex
	if err := query.All(&indexModels); err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if err := common.ValidateSliceLength("article_index", indexModels, limit); err != nil {
		nextCursor = indexModels[limit-1].SK
		indexModels = indexModels[:limit]
	}

	articleIDs := make([]string, 0, len(indexModels))
	for i := range indexModels {
		articleID := strings.TrimSpace(indexModels[i].ArticleID)
		if articleID == "" {
			articleID = models.CMSArticleIndexExtractArticleID(indexModels[i].SK)
		}
		if articleID == "" {
			continue
		}
		articleIDs = append(articleIDs, articleID)
	}

	articles, err := r.batchGetArticlesOrdered(ctx, articleIDs)
	if err != nil {
		return nil, "", err
	}

	return articles, nextCursor, nil
}

func (r *ArticleRepository) batchGetArticlesOrdered(ctx context.Context, articleIDs []string) ([]*models.Article, error) {
	seen := map[string]struct{}{}
	uniqueIDs := make([]string, 0, len(articleIDs))
	for _, articleID := range articleIDs {
		articleID = strings.TrimSpace(articleID)
		if articleID == "" {
			continue
		}
		if _, ok := seen[articleID]; ok {
			continue
		}
		seen[articleID] = struct{}{}
		uniqueIDs = append(uniqueIDs, articleID)
	}
	if len(uniqueIDs) == 0 {
		return []*models.Article{}, nil
	}

	keys := make([]struct{ PK, SK string }, 0, len(uniqueIDs))
	for _, articleID := range uniqueIDs {
		pk := fmt.Sprintf("object#%s", articleID)
		keys = append(keys, struct{ PK, SK string }{PK: pk, SK: pk})
	}

	items, err := r.BatchGet(ctx, keys)
	if err != nil {
		return nil, err
	}

	byID := make(map[string]*models.Article, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		byID[strings.TrimSpace(item.ID)] = item
	}

	ordered := make([]*models.Article, 0, len(uniqueIDs))
	for _, articleID := range uniqueIDs {
		if article := byID[articleID]; article != nil {
			ordered = append(ordered, article)
		}
	}

	return ordered, nil
}
