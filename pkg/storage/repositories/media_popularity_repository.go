package repositories

import (
	"context"
	stdErrors "errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

// MediaPopularityRepository handles media popularity aggregates using DynamORM
type MediaPopularityRepository struct {
	*EnhancedBaseRepository[*models.MediaPopularity]
}

// NewMediaPopularityRepository creates a new media popularity repository
func NewMediaPopularityRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *MediaPopularityRepository {
	enhancedRepo := NewEnhancedBaseRepository[*models.MediaPopularity](db, tableName, logger, costService, "MediaPopularityRepository", "media_popularity")

	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService())
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &MediaPopularityRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// UpsertPopularity creates or updates media popularity record
func (r *MediaPopularityRepository) UpsertPopularity(ctx context.Context, popularity *models.MediaPopularity) error {
	if err := common.ValidateRequiredParam("mediaID", popularity.MediaID); err != nil {
		return err
	}

	// Ensure keys are set
	if popularity.PK == "" || popularity.SK == "" {
		_ = popularity.UpdateKeys()
	}

	// Try to get existing record
	var existing models.MediaPopularity
	getErr := r.Get(ctx, popularity.PK, popularity.SK, &existing)

	if getErr != nil {
		var appErr *apperrors.AppError
		isNotFound := dynamormErrors.IsNotFound(getErr) || (stdErrors.As(getErr, &appErr) && appErr.Code == apperrors.CodeNotFound)

		// Distinguish between "not found" and "real error"
		if !isNotFound {
			// Real error (permissions, throttling, network, etc.)
			r.logger.Error("Failed to check existing popularity record",
				zap.String("media_id", popularity.MediaID),
				zap.String("period", popularity.Period),
				zap.Error(getErr))
			return ErrorHandler.HandleGetError(getErr, EntityMedia, popularity.MediaID)
		}

		// Record doesn't exist, create it
		err := r.ValidateAndCreate(ctx, popularity)
		if err != nil {
			r.logger.Error("Failed to create media popularity",
				zap.String("media_id", popularity.MediaID),
				zap.String("period", popularity.Period),
				zap.Error(err))
			return ErrorHandler.HandleCreateError(err, EntityMedia, popularity.MediaID)
		}
	} else {
		// Record exists - update it in place
		// Update fields on existing record
		existing.ViewCount = popularity.ViewCount
		existing.UniqueViewers = popularity.UniqueViewers
		existing.CompletionCount = popularity.CompletionCount
		existing.TotalWatchTime = popularity.TotalWatchTime
		existing.BufferingEvents = popularity.BufferingEvents
		existing.LastViewed = time.Now()

		// Merge quality views
		if existing.QualityViews == nil {
			existing.QualityViews = make(map[string]int64)
		}
		for quality, count := range popularity.QualityViews {
			existing.QualityViews[quality] = count
		}

		// Recalculate popularity score
		existing.PopularityScore = float64(existing.ViewCount)
		existing.TrendScore = float64(existing.ViewCount)

		// Update TTL
		switch existing.Period {
		case "DAY":
			existing.TTL = time.Now().Add(7 * 24 * time.Hour).Unix()
		case "WEEK":
			existing.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
		case "MONTH":
			existing.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()
		}

		// Recalculate GSI keys (GSI1SK changes, but PK/SK stay same)
		_ = existing.UpdateKeys()

		// Use ValidateAndUpdate - works because PK/SK don't change
		err := r.ValidateAndUpdate(ctx, &existing)
		if err != nil {
			r.logger.Error("Failed to update media popularity",
				zap.String("media_id", existing.MediaID),
				zap.String("period", existing.Period),
				zap.Error(err))
			return err
		}
	}

	// Track cost
	if r.GetCostService() != nil {
		if trackErr := r.TrackWrite(ctx, "UpsertPopularity", 1); trackErr != nil {
			r.logger.Warn("failed to track write cost", zap.Error(trackErr))
		}
	}

	return nil
}

// GetPopularMediaByPeriod retrieves popular media for a given period with cursor pagination
// Queries GSI1 which is sorted by inverted view count for descending popularity order
func (r *MediaPopularityRepository) GetPopularMediaByPeriod(ctx context.Context, period string, limit int, cursor *string) ([]*models.MediaPopularity, error) {
	if limit < 1 {
		limit = 10
	}

	gsi1pk := fmt.Sprintf("PERIOD#%s", period)

	// Fetch limit+1 for pagination
	pageLimit := limit + 1

	var popularityRecords []*models.MediaPopularity
	query := r.GetDB().WithContext(ctx).Model(&models.MediaPopularity{}).
		Where("gsi1PK", "=", gsi1pk).
		Limit(pageLimit)

	// Apply cursor if provided
	if cursor != nil && *cursor != "" {
		query = query.Cursor(*cursor)
	}

	err := query.All(&popularityRecords)
	if err != nil {
		if dynamormErrors.IsNotFound(err) {
			return []*models.MediaPopularity{}, nil
		}
		r.logger.Error("Failed to get popular media by period",
			zap.String("period", period),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityMedia, "popular media")
	}

	// Track cost
	if r.GetCostService() != nil {
		if trackErr := r.TrackRead(ctx, "GetPopularMediaByPeriod", int64(len(popularityRecords))); trackErr != nil {
			r.logger.Warn("failed to track query cost", zap.Error(trackErr))
		}
	}

	return popularityRecords, nil
}

// GetPopularityForMedia retrieves popularity record for specific media
func (r *MediaPopularityRepository) GetPopularityForMedia(ctx context.Context, mediaID, period string) (*models.MediaPopularity, error) {
	if err := common.ValidateRequiredParam("mediaID", mediaID); err != nil {
		return nil, err
	}

	// Use stable primary key to get the record directly
	pk := fmt.Sprintf("MEDIA_POPULARITY#%s", period)
	sk := fmt.Sprintf("MEDIA#%s", mediaID)

	var popularity models.MediaPopularity
	err := r.Get(ctx, pk, sk, &popularity)

	if err != nil {
		var appErr *apperrors.AppError
		if dynamormErrors.IsNotFound(err) || (stdErrors.As(err, &appErr) && appErr.Code == apperrors.CodeNotFound) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityMedia, fmt.Sprintf("popularity %s#%s", mediaID, period))
		}
		return nil, ErrorHandler.HandleQueryError(err, EntityMedia, "media popularity")
	}

	return &popularity, nil
}

// IncrementViewCount atomically increments view count for media
func (r *MediaPopularityRepository) IncrementViewCount(ctx context.Context, mediaID, period string, incrementBy int64) error {
	if err := common.ValidateRequiredParam("mediaID", mediaID); err != nil {
		return err
	}

	// Get existing record or create new one
	existing, err := r.GetPopularityForMedia(ctx, mediaID, period)
	if err != nil {
		return err
	}

	var popularity *models.MediaPopularity
	if existing != nil {
		popularity = existing
		popularity.IncrementViews(incrementBy)
	} else {
		// Create new record
		popularity = &models.MediaPopularity{}
		popularity.SetForPeriod(mediaID, period, incrementBy)
	}

	// Upsert the updated record
	return r.UpsertPopularity(ctx, popularity)
}
