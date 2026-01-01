// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// SeriesRepository is a thread-safe in-memory implementation of interfaces.SeriesRepository.
type SeriesRepository struct {
	mu sync.RWMutex

	// Series by composite key: authorID:seriesID -> series
	series map[string]*models.Series

	// Series by author: authorID -> []seriesKey
	seriesByAuthor map[string][]string
}

// NewSeriesRepository creates a new in-memory series repository
func NewSeriesRepository() *SeriesRepository {
	return &SeriesRepository{
		series:         make(map[string]*models.Series),
		seriesByAuthor: make(map[string][]string),
	}
}

// seriesKey creates a composite key for a series
func seriesKey(authorID, seriesID string) string {
	return fmt.Sprintf("%s:%s", authorID, seriesID)
}

// CreateSeries creates a new series
func (r *SeriesRepository) CreateSeries(_ context.Context, series *models.Series) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if series == nil || series.AuthorID == "" || series.ID == "" {
		return storage.ErrInvalidInput
	}

	key := seriesKey(series.AuthorID, series.ID)
	if _, exists := r.series[key]; exists {
		return storage.ErrAlreadyExists
	}

	// Store series
	r.series[key] = series

	// Index by author
	r.seriesByAuthor[series.AuthorID] = append(r.seriesByAuthor[series.AuthorID], key)

	return nil
}

// GetSeries retrieves a series by author ID and series ID
func (r *SeriesRepository) GetSeries(_ context.Context, authorID, seriesID string) (*models.Series, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := seriesKey(authorID, seriesID)
	series, exists := r.series[key]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return series, nil
}

// ListSeriesByAuthor lists series for an author
func (r *SeriesRepository) ListSeriesByAuthor(ctx context.Context, authorID string, limit int) ([]*models.Series, error) {
	items, _, err := r.ListSeriesByAuthorPaginated(ctx, authorID, limit, "")
	return items, err
}

// ListSeriesByAuthorPaginated lists series for an author with cursor pagination
func (r *SeriesRepository) ListSeriesByAuthorPaginated(_ context.Context, authorID string, limit int, cursor string) ([]*models.Series, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	authorID = strings.TrimSpace(authorID)
	if authorID == "" {
		return nil, "", storage.ErrInvalidInput
	}

	if limit <= 0 {
		limit = 25
	}

	// Get series for author
	keys := r.seriesByAuthor[authorID]
	seriesList := make([]*models.Series, 0, len(keys))
	for _, key := range keys {
		if s, exists := r.series[key]; exists {
			seriesList = append(seriesList, s)
		}
	}

	// Sort by SK (ID#...) ascending
	sort.Slice(seriesList, func(i, j int) bool {
		return seriesList[i].SK < seriesList[j].SK
	})

	// Apply cursor
	startIdx := 0
	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		if !strings.HasPrefix(cursor, "ID#") {
			cursor = "ID#" + cursor
		}
		for i, s := range seriesList {
			if s.SK > cursor {
				startIdx = i
				break
			}
		}
	}

	// Apply limit
	endIdx := startIdx + limit
	if endIdx > len(seriesList) {
		endIdx = len(seriesList)
	}

	result := seriesList[startIdx:endIdx]
	nextCursor := ""
	if endIdx < len(seriesList) && len(result) > 0 {
		nextCursor = result[len(result)-1].SK
	}

	return result, nextCursor, nil
}

// UpdateArticleCount atomically increments/decrements a series's ArticleCount
func (r *SeriesRepository) UpdateArticleCount(_ context.Context, authorID string, seriesID string, delta int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	authorID = strings.TrimSpace(authorID)
	seriesID = strings.TrimSpace(seriesID)
	if authorID == "" || seriesID == "" || delta == 0 {
		return nil
	}

	key := seriesKey(authorID, seriesID)
	series, exists := r.series[key]
	if !exists {
		// Treat missing series as no-op (matches real implementation)
		return nil
	}

	newCount := series.ArticleCount + delta
	if newCount < 0 {
		newCount = 0
	}
	series.ArticleCount = newCount

	return nil
}

// Update updates an existing series
func (r *SeriesRepository) Update(_ context.Context, series *models.Series) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if series == nil || series.AuthorID == "" || series.ID == "" {
		return storage.ErrInvalidInput
	}

	key := seriesKey(series.AuthorID, series.ID)
	if _, exists := r.series[key]; !exists {
		return storage.ErrNotFound
	}

	r.series[key] = series
	return nil
}

// Delete deletes a series by PK and SK
func (r *SeriesRepository) Delete(_ context.Context, pk, sk string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Extract authorID from PK (format: AUTHOR#<authorID>#SERIES)
	pk = strings.TrimSpace(pk)
	if !strings.HasPrefix(pk, "AUTHOR#") || !strings.HasSuffix(pk, "#SERIES") {
		return storage.ErrInvalidInput
	}
	authorID := strings.TrimSuffix(strings.TrimPrefix(pk, "AUTHOR#"), "#SERIES")

	// Extract seriesID from SK (format: ID#<seriesID>)
	sk = strings.TrimSpace(sk)
	if !strings.HasPrefix(sk, "ID#") {
		return storage.ErrInvalidInput
	}
	seriesID := strings.TrimPrefix(sk, "ID#")

	key := seriesKey(authorID, seriesID)
	if _, exists := r.series[key]; !exists {
		return storage.ErrNotFound
	}

	// Remove from series map
	delete(r.series, key)

	// Remove from seriesByAuthor index
	keys := r.seriesByAuthor[authorID]
	for i, k := range keys {
		if k == key {
			r.seriesByAuthor[authorID] = append(keys[:i], keys[i+1:]...)
			break
		}
	}

	return nil
}

// Clear clears all data (test helper)
func (r *SeriesRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.series = make(map[string]*models.Series)
	r.seriesByAuthor = make(map[string][]string)
}

// Ensure SeriesRepository implements interfaces.SeriesRepository
var _ interfaces.SeriesRepository = (*SeriesRepository)(nil)
