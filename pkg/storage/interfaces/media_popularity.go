// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// MediaPopularityRepository defines the interface for media popularity operations.
// This handles aggregated popularity metrics for media items.
type MediaPopularityRepository interface {
	// Core popularity operations
	UpsertPopularity(ctx context.Context, popularity *models.MediaPopularity) error
	GetPopularityForMedia(ctx context.Context, mediaID, period string) (*models.MediaPopularity, error)

	// Popular media queries
	GetPopularMediaByPeriod(ctx context.Context, period string, limit int, cursor *string) ([]*models.MediaPopularity, error)

	// View count operations
	IncrementViewCount(ctx context.Context, mediaID, period string, incrementBy int64) error
}
