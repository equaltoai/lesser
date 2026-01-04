// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// AnnouncementRepository is a thread-safe in-memory implementation of interfaces.AnnouncementRepository.
type AnnouncementRepository struct {
	mu sync.RWMutex

	// Announcements: key = announcement ID
	announcements map[string]*storage.Announcement

	// Dismissals: key = "username:announcementID"
	dismissals map[string]bool

	// Reactions: key = announcementID -> emojiName -> []username
	reactions map[string]map[string][]string
}

// NewAnnouncementRepository creates a new in-memory announcement repository
func NewAnnouncementRepository() *AnnouncementRepository {
	return &AnnouncementRepository{
		announcements: make(map[string]*storage.Announcement),
		dismissals:    make(map[string]bool),
		reactions:     make(map[string]map[string][]string),
	}
}

// CreateAnnouncement creates a new announcement
func (r *AnnouncementRepository) CreateAnnouncement(_ context.Context, announcement *storage.Announcement) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.announcements[announcement.ID] = announcement
	return nil
}

// GetAnnouncement retrieves a single announcement by ID
func (r *AnnouncementRepository) GetAnnouncement(_ context.Context, id string) (*storage.Announcement, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ann, exists := r.announcements[id]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return ann, nil
}

// GetAnnouncements retrieves all announcements (for backward compatibility)
func (r *AnnouncementRepository) GetAnnouncements(_ context.Context, active bool) ([]*storage.Announcement, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	var result []*storage.Announcement
	for _, ann := range r.announcements {
		if active {
			if ann.StartsAt != nil && ann.StartsAt.After(now) {
				continue
			}
			if ann.EndsAt != nil && ann.EndsAt.Before(now) {
				continue
			}
		}
		result = append(result, ann)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].PublishedAt.After(result[j].PublishedAt)
	})
	return result, nil
}

// GetAnnouncementsPaginated retrieves announcements with pagination
func (r *AnnouncementRepository) GetAnnouncementsPaginated(_ context.Context, active bool, limit int, cursor string) ([]*storage.Announcement, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all, _ := r.GetAnnouncements(context.Background(), active)
	return paginateAnnouncements(all, limit, cursor)
}

// GetAnnouncementsByAdmin retrieves announcements created by a specific admin
func (r *AnnouncementRepository) GetAnnouncementsByAdmin(_ context.Context, _ string, limit int, cursor string) ([]*storage.Announcement, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.Announcement
	for _, ann := range r.announcements {
		result = append(result, ann)
	}
	return paginateAnnouncements(result, limit, cursor)
}

// UpdateAnnouncement updates an existing announcement
func (r *AnnouncementRepository) UpdateAnnouncement(_ context.Context, announcement *storage.Announcement) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.announcements[announcement.ID]; !exists {
		return storage.ErrNotFound
	}
	r.announcements[announcement.ID] = announcement
	return nil
}

// DeleteAnnouncement deletes an announcement
func (r *AnnouncementRepository) DeleteAnnouncement(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.announcements, id)
	delete(r.reactions, id)
	return nil
}

// DismissAnnouncement marks an announcement as dismissed by a user
func (r *AnnouncementRepository) DismissAnnouncement(_ context.Context, username, announcementID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := username + ":" + announcementID
	r.dismissals[key] = true
	return nil
}

// IsDismissed checks if a user has dismissed an announcement
func (r *AnnouncementRepository) IsDismissed(_ context.Context, username, announcementID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := username + ":" + announcementID
	return r.dismissals[key], nil
}

// GetDismissedAnnouncements gets all announcement IDs dismissed by a user
func (r *AnnouncementRepository) GetDismissedAnnouncements(_ context.Context, username string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []string
	prefix := username + ":"
	for key := range r.dismissals {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			result = append(result, key[len(prefix):])
		}
	}
	return result, nil
}

// AddAnnouncementReaction adds a user's reaction to an announcement
func (r *AnnouncementRepository) AddAnnouncementReaction(_ context.Context, username, announcementID, emojiName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.reactions[announcementID] == nil {
		r.reactions[announcementID] = make(map[string][]string)
	}
	r.reactions[announcementID][emojiName] = append(r.reactions[announcementID][emojiName], username)
	return nil
}

// RemoveAnnouncementReaction removes a user's reaction from an announcement
func (r *AnnouncementRepository) RemoveAnnouncementReaction(_ context.Context, username, announcementID, emojiName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.reactions[announcementID] == nil {
		return nil
	}
	users := r.reactions[announcementID][emojiName]
	filtered := make([]string, 0, len(users))
	for _, u := range users {
		if u != username {
			filtered = append(filtered, u)
		}
	}
	r.reactions[announcementID][emojiName] = filtered
	return nil
}

// GetAnnouncementReactions gets all reactions for an announcement
func (r *AnnouncementRepository) GetAnnouncementReactions(_ context.Context, announcementID string) (map[string][]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.reactions[announcementID] == nil {
		return make(map[string][]string), nil
	}
	result := make(map[string][]string)
	for emoji, users := range r.reactions[announcementID] {
		result[emoji] = append([]string{}, users...)
	}
	return result, nil
}

// Helper functions

func paginateAnnouncements(announcements []*storage.Announcement, limit int, cursor string) ([]*storage.Announcement, string, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	startIdx := 0
	if cursor != "" {
		for i, a := range announcements {
			if a.ID == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var result []*storage.Announcement
	var nextCursor string

	for i := startIdx; i < len(announcements) && len(result) < limit; i++ {
		result = append(result, announcements[i])
	}

	if startIdx+limit < len(announcements) && len(result) > 0 {
		nextCursor = result[len(result)-1].ID
	}

	return result, nextCursor, nil
}

// Clear clears all data (test helper)
func (r *AnnouncementRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.announcements = make(map[string]*storage.Announcement)
	r.dismissals = make(map[string]bool)
	r.reactions = make(map[string]map[string][]string)
}

// Ensure AnnouncementRepository implements interfaces.AnnouncementRepository
var _ interfaces.AnnouncementRepository = (*AnnouncementRepository)(nil)
