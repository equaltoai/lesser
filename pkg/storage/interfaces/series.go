// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// SeriesRepository defines the interface for series operations.
// This handles CMS series management for multi-part content.
type SeriesRepository interface {
	// ===== Core CRUD Operations =====

	// CreateSeries creates a new series
	CreateSeries(ctx context.Context, series *models.Series) error

	// GetSeries retrieves a series by author ID and series ID
	GetSeries(ctx context.Context, authorID, seriesID string) (*models.Series, error)

	// Update updates an existing series
	Update(ctx context.Context, series *models.Series) error

	// Delete deletes a series by PK and SK
	Delete(ctx context.Context, pk, sk string) error

	// ===== List Operations =====

	// ListSeriesByAuthor lists series for an author
	ListSeriesByAuthor(ctx context.Context, authorID string, limit int) ([]*models.Series, error)

	// ListSeriesByAuthorPaginated lists series for an author with cursor pagination
	ListSeriesByAuthorPaginated(ctx context.Context, authorID string, limit int, cursor string) ([]*models.Series, string, error)

	// ===== Count Operations =====

	// UpdateArticleCount atomically increments/decrements a series's ArticleCount
	UpdateArticleCount(ctx context.Context, authorID string, seriesID string, delta int) error
}
