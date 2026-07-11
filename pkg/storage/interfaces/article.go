// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormcore "github.com/theory-cloud/tabletheory/v2/pkg/core"
)

// ArticleRepository defines the interface for article operations.
// This handles CMS article management including CRUD operations and pagination.
type ArticleRepository interface {
	// ===== Database Access =====

	// GetDB returns the underlying DynamoDB connection for advanced operations
	GetDB() dynamormcore.DB

	// ===== Core CRUD Operations =====

	// CreateArticle creates a new article
	CreateArticle(ctx context.Context, article *models.Article) error

	// GetArticle retrieves an article by ID
	GetArticle(ctx context.Context, id string) (*models.Article, error)

	// UpdateArticle updates an existing article
	UpdateArticle(ctx context.Context, article *models.Article) error

	// DeleteArticle deletes an article
	DeleteArticle(ctx context.Context, id string) error

	// ===== List Operations =====

	// ListArticles lists articles with a limit
	ListArticles(ctx context.Context, limit int) ([]*models.Article, error)

	// ListArticlesPaginated lists articles with cursor pagination
	ListArticlesPaginated(ctx context.Context, limit int, cursor string) ([]*models.Article, string, error)

	// ===== Filtered List Operations =====

	// ListArticlesByAuthorPaginated lists articles for a specific author with pagination
	ListArticlesByAuthorPaginated(ctx context.Context, authorActorID string, limit int, cursor string) ([]*models.Article, string, error)

	// ListArticlesBySeriesPaginated lists articles for a specific series with pagination
	ListArticlesBySeriesPaginated(ctx context.Context, seriesID string, limit int, cursor string) ([]*models.Article, string, error)

	// ListArticlesByCategoryPaginated lists articles for a specific category with pagination
	ListArticlesByCategoryPaginated(ctx context.Context, categoryID string, limit int, cursor string) ([]*models.Article, string, error)
}
