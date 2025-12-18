package repositories

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/cost"
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
	return r.ValidateAndCreate(ctx, article)
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
	var articles []models.Article
	// Basic scan for now
	err := r.db.WithContext(ctx).Model(&models.Article{}).Limit(limit).All(&articles)
	if err != nil {
		return nil, err
	}

	result := make([]*models.Article, len(articles))
	for i := range articles {
		result[i] = &articles[i]
	}
	return result, nil
}