package repositories

import (
	"context"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// SeriesRepository implements series operations
type SeriesRepository struct {
	*EnhancedBaseRepository[*models.Series]
}

// NewSeriesRepository creates a new series repository
func NewSeriesRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *SeriesRepository {
	enhancedRepo := NewEnhancedBaseRepository[*models.Series](db, tableName, logger, costService, "SeriesRepository", "series")

	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService())
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &SeriesRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreateSeries creates a new series
func (r *SeriesRepository) CreateSeries(ctx context.Context, series *models.Series) error {
	return r.ValidateAndCreate(ctx, series)
}

// GetSeries retrieves a series by author ID and series ID
func (r *SeriesRepository) GetSeries(ctx context.Context, authorID, seriesID string) (*models.Series, error) {
	var series models.Series
	pk := fmt.Sprintf("AUTHOR#%s#SERIES", authorID)
	sk := fmt.Sprintf("ID#%s", seriesID)

	err := r.Get(ctx, pk, sk, &series)
	if err != nil {
		return nil, err
	}
	return &series, nil
}

// ListSeriesByAuthor lists series for an author
func (r *SeriesRepository) ListSeriesByAuthor(ctx context.Context, authorID string, limit int) ([]*models.Series, error) {
	items, _, err := r.ListSeriesByAuthorPaginated(ctx, authorID, limit, "")
	return items, err
}

// ListSeriesByAuthorPaginated lists series for an author with cursor pagination.
// Cursor values are either full SK values (ID#...) or raw series IDs.
func (r *SeriesRepository) ListSeriesByAuthorPaginated(ctx context.Context, authorID string, limit int, cursor string) ([]*models.Series, string, error) {
	authorID = strings.TrimSpace(authorID)
	if err := common.ValidateRequiredParam("authorID", authorID); err != nil {
		return nil, "", err
	}
	if limit <= 0 {
		limit = 25
	}

	pk := fmt.Sprintf("AUTHOR#%s#SERIES", authorID)
	cursor = strings.TrimSpace(cursor)
	if cursor != "" && !strings.HasPrefix(cursor, "ID#") {
		cursor = "ID#" + cursor
	}

	return listByPKSKPrefixPaginated[*models.Series](ctx, r.db, &models.Series{}, pk, "ID#", limit, cursor)
}
