// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sync"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// MarkerRepository is a thread-safe in-memory implementation of interfaces.MarkerRepository.
type MarkerRepository struct {
	mu sync.RWMutex
	// markers stores markers keyed by "username:timeline"
	markers map[string]*storage.Marker
}

// NewMarkerRepository creates a new in-memory marker repository
func NewMarkerRepository() *MarkerRepository {
	return &MarkerRepository{
		markers: make(map[string]*storage.Marker),
	}
}

// SaveMarker saves or updates a timeline position marker
func (r *MarkerRepository) SaveMarker(_ context.Context, username, timeline string, lastReadID string, version int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := username + ":" + timeline
	r.markers[key] = &storage.Marker{
		Timeline:   timeline,
		LastReadID: lastReadID,
		Version:    version,
	}
	return nil
}

// GetMarkers retrieves timeline position markers for specified timelines
func (r *MarkerRepository) GetMarkers(_ context.Context, username string, timelines []string) (map[string]*storage.Marker, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*storage.Marker)
	for _, timeline := range timelines {
		key := username + ":" + timeline
		if marker, exists := r.markers[key]; exists {
			result[timeline] = marker
		}
	}
	return result, nil
}

// Clear clears all data (test helper)
func (r *MarkerRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.markers = make(map[string]*storage.Marker)
}

// Ensure MarkerRepository implements interfaces.MarkerRepository
var _ interfaces.MarkerRepository = (*MarkerRepository)(nil)
