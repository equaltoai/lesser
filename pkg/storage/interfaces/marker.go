// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
)

// MarkerRepository defines the interface for timeline position marker operations.
// This handles saving and retrieving read position markers for timelines.
type MarkerRepository interface {
	// SaveMarker saves or updates a timeline position marker
	SaveMarker(ctx context.Context, username, timeline string, lastReadID string, version int) error

	// GetMarkers retrieves timeline position markers for specified timelines
	GetMarkers(ctx context.Context, username string, timelines []string) (map[string]*storage.Marker, error)
}
