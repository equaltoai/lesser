// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ScheduledStatusRepository is a thread-safe in-memory implementation of interfaces.ScheduledStatusRepository.
type ScheduledStatusRepository struct {
	mu sync.RWMutex

	// Scheduled statuses: key = ID
	statuses map[string]*storage.ScheduledStatus

	// Index by user: username -> []ID
	byUser map[string][]string

	// Media repository for fetching media
	mediaRepo interfaces.MediaRepositoryInterface
}

// NewScheduledStatusRepository creates a new in-memory scheduled status repository
func NewScheduledStatusRepository() *ScheduledStatusRepository {
	return &ScheduledStatusRepository{
		statuses: make(map[string]*storage.ScheduledStatus),
		byUser:   make(map[string][]string),
	}
}

// CreateScheduledStatus creates a new scheduled status
func (r *ScheduledStatusRepository) CreateScheduledStatus(_ context.Context, scheduled *storage.ScheduledStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.statuses[scheduled.ID] = scheduled
	r.byUser[scheduled.Username] = append(r.byUser[scheduled.Username], scheduled.ID)
	return nil
}

// GetScheduledStatus retrieves a scheduled status by ID
func (r *ScheduledStatusRepository) GetScheduledStatus(_ context.Context, id string) (*storage.ScheduledStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status, exists := r.statuses[id]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return status, nil
}

// GetScheduledStatuses retrieves scheduled statuses for a user
func (r *ScheduledStatusRepository) GetScheduledStatuses(_ context.Context, username string, limit int, cursor string) ([]*storage.ScheduledStatus, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := r.byUser[username]
	var statuses []*storage.ScheduledStatus
	for _, id := range ids {
		if s, exists := r.statuses[id]; exists {
			statuses = append(statuses, s)
		}
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].ScheduledAt.Before(statuses[j].ScheduledAt)
	})

	return paginateScheduledStatuses(statuses, limit, cursor)
}

// UpdateScheduledStatus updates a scheduled status
func (r *ScheduledStatusRepository) UpdateScheduledStatus(_ context.Context, scheduled *storage.ScheduledStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.statuses[scheduled.ID]; !exists {
		return storage.ErrNotFound
	}
	r.statuses[scheduled.ID] = scheduled
	return nil
}

// DeleteScheduledStatus deletes a scheduled status
func (r *ScheduledStatusRepository) DeleteScheduledStatus(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	status, exists := r.statuses[id]
	if !exists {
		return nil
	}

	delete(r.statuses, id)
	r.byUser[status.Username] = removeStringFromSlice(r.byUser[status.Username], id)
	return nil
}

// GetDueScheduledStatuses retrieves scheduled statuses that are due to be published
func (r *ScheduledStatusRepository) GetDueScheduledStatuses(_ context.Context, before time.Time, limit int) ([]*storage.ScheduledStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var due []*storage.ScheduledStatus
	for _, s := range r.statuses {
		if s.ScheduledAt.Before(before) && !s.Published {
			due = append(due, s)
		}
	}

	sort.Slice(due, func(i, j int) bool {
		return due[i].ScheduledAt.Before(due[j].ScheduledAt)
	})

	if limit > 0 && len(due) > limit {
		due = due[:limit]
	}
	return due, nil
}

// MarkScheduledStatusPublished marks a scheduled status as published
func (r *ScheduledStatusRepository) MarkScheduledStatusPublished(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	status, exists := r.statuses[id]
	if !exists {
		return storage.ErrNotFound
	}
	status.Published = true
	return nil
}

// GetScheduledStatusMedia gets media for scheduled status
func (r *ScheduledStatusRepository) GetScheduledStatusMedia(ctx context.Context, id string) ([]*models.Media, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status, exists := r.statuses[id]
	if !exists {
		return nil, storage.ErrNotFound
	}

	if r.mediaRepo == nil || len(status.MediaIDs) == 0 {
		return []*models.Media{}, nil
	}

	var media []*models.Media
	for _, mediaID := range status.MediaIDs {
		m, err := r.mediaRepo.GetMedia(ctx, mediaID)
		if err == nil {
			media = append(media, m)
		}
	}
	return media, nil
}

// SetMediaRepository sets the media repository dependency
func (r *ScheduledStatusRepository) SetMediaRepository(mediaRepo interfaces.MediaRepositoryInterface) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mediaRepo = mediaRepo
}

// Helper functions

func paginateScheduledStatuses(statuses []*storage.ScheduledStatus, limit int, cursor string) ([]*storage.ScheduledStatus, string, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	startIdx := 0
	if cursor != "" {
		for i, s := range statuses {
			if s.ID == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var result []*storage.ScheduledStatus
	var nextCursor string

	for i := startIdx; i < len(statuses) && len(result) < limit; i++ {
		result = append(result, statuses[i])
	}

	if startIdx+limit < len(statuses) && len(result) > 0 {
		nextCursor = result[len(result)-1].ID
	}

	return result, nextCursor, nil
}

func removeStringFromSlice(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

// Clear clears all data (test helper)
func (r *ScheduledStatusRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.statuses = make(map[string]*storage.ScheduledStatus)
	r.byUser = make(map[string][]string)
}

// Ensure ScheduledStatusRepository implements interfaces.ScheduledStatusRepository
var _ interfaces.ScheduledStatusRepository = (*ScheduledStatusRepository)(nil)
