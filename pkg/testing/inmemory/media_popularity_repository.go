// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// MediaPopularityRepository is a thread-safe in-memory implementation of interfaces.MediaPopularityRepository.
type MediaPopularityRepository struct {
	mu sync.RWMutex

	// Popularity by key: mediaID_period -> MediaPopularity
	popularity map[string]*models.MediaPopularity

	// Popularity by period: period -> []MediaPopularity (sorted by view count desc)
	popularityByPeriod map[string][]*models.MediaPopularity
}

// NewMediaPopularityRepository creates a new in-memory media popularity repository
func NewMediaPopularityRepository() *MediaPopularityRepository {
	return &MediaPopularityRepository{
		popularity:         make(map[string]*models.MediaPopularity),
		popularityByPeriod: make(map[string][]*models.MediaPopularity),
	}
}

// ===== Core Popularity Operations =====

// UpsertPopularity creates or updates media popularity record
func (r *MediaPopularityRepository) UpsertPopularity(_ context.Context, popularity *models.MediaPopularity) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := popularity.MediaID + "_" + popularity.Period
	existing, exists := r.popularity[key]

	if exists {
		// Update existing record
		existing.ViewCount = popularity.ViewCount
		existing.UniqueViewers = popularity.UniqueViewers
		existing.CompletionCount = popularity.CompletionCount
		existing.TotalWatchTime = popularity.TotalWatchTime
		existing.BufferingEvents = popularity.BufferingEvents
		existing.LastViewed = time.Now()

		if existing.QualityViews == nil {
			existing.QualityViews = make(map[string]int64)
		}
		for quality, count := range popularity.QualityViews {
			existing.QualityViews[quality] = count
		}

		existing.PopularityScore = float64(existing.ViewCount)
		existing.TrendScore = float64(existing.ViewCount)
	} else {
		// Create new record
		r.popularity[key] = popularity

		// Add to period index
		r.popularityByPeriod[popularity.Period] = append(r.popularityByPeriod[popularity.Period], popularity)
	}

	// Re-sort the period index by view count (descending)
	r.sortPeriodIndex(popularity.Period)

	return nil
}

// GetPopularityForMedia retrieves popularity record for specific media
func (r *MediaPopularityRepository) GetPopularityForMedia(_ context.Context, mediaID, period string) (*models.MediaPopularity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := mediaID + "_" + period
	popularity, exists := r.popularity[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return popularity, nil
}

// ===== Popular Media Queries =====

// GetPopularMediaByPeriod retrieves popular media for a given period with cursor pagination
func (r *MediaPopularityRepository) GetPopularMediaByPeriod(_ context.Context, period string, limit int, cursor *string) ([]*models.MediaPopularity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit < 1 {
		limit = 10
	}

	records := r.popularityByPeriod[period]
	if len(records) == 0 {
		return []*models.MediaPopularity{}, nil
	}

	startIdx := 0
	if cursor != nil && *cursor != "" {
		for i, p := range records {
			if p.MediaID == *cursor {
				startIdx = i + 1
				break
			}
		}
	}

	endIdx := startIdx + limit
	if endIdx > len(records) {
		endIdx = len(records)
	}

	if startIdx >= len(records) {
		return []*models.MediaPopularity{}, nil
	}

	return records[startIdx:endIdx], nil
}

// ===== View Count Operations =====

// IncrementViewCount atomically increments view count for media
func (r *MediaPopularityRepository) IncrementViewCount(_ context.Context, mediaID, period string, incrementBy int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := mediaID + "_" + period
	existing, exists := r.popularity[key]

	if exists {
		existing.ViewCount += incrementBy
		existing.PopularityScore = float64(existing.ViewCount)
		existing.TrendScore = float64(existing.ViewCount)
		existing.LastViewed = time.Now()
	} else {
		// Create new record
		newPopularity := &models.MediaPopularity{
			MediaID:         mediaID,
			Period:          period,
			ViewCount:       incrementBy,
			PopularityScore: float64(incrementBy),
			TrendScore:      float64(incrementBy),
			LastViewed:      time.Now(),
			QualityViews:    make(map[string]int64),
		}
		r.popularity[key] = newPopularity
		r.popularityByPeriod[period] = append(r.popularityByPeriod[period], newPopularity)
	}

	// Re-sort the period index
	r.sortPeriodIndex(period)

	return nil
}

// sortPeriodIndex sorts the period index by view count (descending)
func (r *MediaPopularityRepository) sortPeriodIndex(period string) {
	records := r.popularityByPeriod[period]
	// Simple bubble sort for in-memory implementation
	for i := 0; i < len(records)-1; i++ {
		for j := 0; j < len(records)-i-1; j++ {
			if records[j].ViewCount < records[j+1].ViewCount {
				records[j], records[j+1] = records[j+1], records[j]
			}
		}
	}
}

// Clear clears all data (test helper)
func (r *MediaPopularityRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.popularity = make(map[string]*models.MediaPopularity)
	r.popularityByPeriod = make(map[string][]*models.MediaPopularity)
}

// Ensure MediaPopularityRepository implements interfaces.MediaPopularityRepository
var _ interfaces.MediaPopularityRepository = (*MediaPopularityRepository)(nil)
