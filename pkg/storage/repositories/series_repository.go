package repositories

import (
	"context"
	"fmt"

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
	var seriesList []models.Series
	pk := fmt.Sprintf("AUTHOR#%s#SERIES", authorID)

	err := r.db.WithContext(ctx).Model(&models.Series{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", "ID#").
		Limit(limit).
		All(&seriesList)

	if err != nil {
		return nil, err
	}

	result := make([]*models.Series, len(seriesList))
	for i := range seriesList {
		result[i] = &seriesList[i]
	}
	return result, nil
}
