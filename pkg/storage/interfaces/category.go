// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormcore "github.com/pay-theory/dynamorm/pkg/core"
)

// CategoryRepository defines the interface for category operations.
// This handles CMS category management for hierarchical content organization.
type CategoryRepository interface {
	// ===== Database Access =====

	// GetDB returns the underlying DynamoDB connection for advanced operations
	GetDB() dynamormcore.DB

	// ===== Core CRUD Operations =====

	// CreateCategory creates a new category
	CreateCategory(ctx context.Context, category *models.Category) error

	// GetCategory retrieves a category by ID
	GetCategory(ctx context.Context, id string) (*models.Category, error)

	// Update updates an existing category
	Update(ctx context.Context, category *models.Category) error

	// Delete deletes a category by PK and SK
	Delete(ctx context.Context, pk, sk string) error

	// ===== List Operations =====

	// ListCategories lists all categories (optionally filtered by parent)
	ListCategories(ctx context.Context, parentID *string, limit int) ([]*models.Category, error)

	// ===== Count Operations =====

	// UpdateArticleCount atomically increments/decrements a category's ArticleCount
	UpdateArticleCount(ctx context.Context, categoryID string, delta int) error
}
