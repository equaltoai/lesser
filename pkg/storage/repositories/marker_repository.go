package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// MarkerRepository implements marker operations using DynamORM with BaseRepository
type MarkerRepository struct {
	*BaseRepository[*models.Marker]
}

// NewMarkerRepository creates a new marker repository
func NewMarkerRepository(db core.DB, tableName string, logger *zap.Logger) *MarkerRepository {
	return &MarkerRepository{
		BaseRepository: NewBaseRepository[*models.Marker](db, tableName, logger),
	}
}

// NewMarkerRepositoryWithCostTracking creates a new marker repository with cost tracking
func NewMarkerRepositoryWithCostTracking(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *MarkerRepository {
	return &MarkerRepository{
		BaseRepository: NewBaseRepositoryWithCostTracking[*models.Marker](db, tableName, logger, costService, "marker"),
	}
}

// SaveMarker saves or updates a timeline position marker
func (r *MarkerRepository) SaveMarker(ctx context.Context, username, timeline string, lastReadID string, version int) error {
	// Get current marker to check version (matches legacy behavior exactly)
	existingMarkers, err := r.GetMarkers(ctx, username, []string{timeline})
	if err != nil {
		r.logger.Error("failed to get existing marker", zap.Error(err))
		// Continue anyway, might be first marker (matches legacy behavior)
	}

	// Check version conflict (matches legacy logic exactly)
	if existingMarkers != nil && existingMarkers[timeline] != nil {
		if existingMarkers[timeline].Version >= version {
			// Don't update if the existing version is newer or same (matches legacy)
			return nil
		}
	}

	// Create the DynamORM model with exact legacy key patterns
	markerModel := &models.Marker{
		Username:   username,
		Timeline:   timeline,
		LastReadID: lastReadID,
		Version:    version,
		UpdatedAt:  time.Now(), // Set explicitly to match legacy
	}

	// Save using BaseRepository - this will call UpdateKeys() and BeforeCreate hooks
	err = r.Create(ctx, markerModel)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMarkerSaveFailed, err)
	}

	r.logger.Debug("saved marker",
		zap.String("username", username),
		zap.String("timeline", timeline),
		zap.String("last_read_id", lastReadID),
		zap.Int("version", version))

	return nil
}

// GetMarkers retrieves timeline position markers for specified timelines
func (r *MarkerRepository) GetMarkers(ctx context.Context, username string, timelines []string) (map[string]*storage.Marker, error) {
	// Default to home and notifications if no timelines specified (matches legacy exactly)
	if err := common.ValidateSliceNotEmpty("timelines", timelines); err != nil {
		timelines = []string{"home", "notifications"}
	}

	markers := make(map[string]*storage.Marker)

	// Get each marker individually (matches legacy approach exactly)
	for _, timeline := range timelines {
		// Create a model to query with the exact legacy key pattern
		queryModel := &models.Marker{
			Username: username,
			Timeline: timeline,
		}
		_ = queryModel.UpdateKeys() // Ignore error as this is internal model operation

		var markerModel models.Marker
		err := r.Get(ctx, queryModel.PK, queryModel.SK, &markerModel)

		if err != nil {
			if errors.IsNotFound(err) {
				// Skip this timeline, continue to next (matches legacy behavior)
				continue
			}
			// BaseRepository.Get wraps not found errors, so check the string
			if strings.Contains(err.Error(), "not found") {
				continue
			}
			// Log actual errors and continue (matches legacy behavior)
			continue
		}

		// Convert DynamORM model to storage.Marker (preserve exact field names)
		markers[timeline] = &storage.Marker{
			LastReadID: markerModel.LastReadID,
			UpdatedAt:  markerModel.UpdatedAt,
			Version:    markerModel.Version,
		}
	}

	return markers, nil
}
