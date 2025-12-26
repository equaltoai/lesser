package repositories

import (
	"context"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
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
