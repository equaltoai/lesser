// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// HashtagRepository is a thread-safe in-memory implementation of interfaces.HashtagRepository.
type HashtagRepository struct {
	mu sync.RWMutex

	// Hashtag info: key = normalized hashtag
	hashtags map[string]*storage.Hashtag

	// Status index: hashtag -> []statusID
	statusIndex map[string][]string

	// Follows: key = "userID:hashtag"
	follows map[string]*models.HashtagFollow

	// Mutes: key = "userID:hashtag"
	mutes map[string]*models.HashtagMute

	// Activity records: hashtag -> []Activity
	activities map[string][]*storage.Activity
}

// NewHashtagRepository creates a new in-memory hashtag repository
func NewHashtagRepository() *HashtagRepository {
	return &HashtagRepository{
		hashtags:    make(map[string]*storage.Hashtag),
		statusIndex: make(map[string][]string),
		follows:     make(map[string]*models.HashtagFollow),
		mutes:       make(map[string]*models.HashtagMute),
		activities:  make(map[string][]*storage.Activity),
	}
}

// IndexHashtag indexes a hashtag when used in a status
func (r *HashtagRepository) IndexHashtag(_ context.Context, hashtag string, statusID string, _ string, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	normalized := strings.ToLower(hashtag)
	r.statusIndex[normalized] = append(r.statusIndex[normalized], statusID)
	return nil
}

// IndexStatusHashtags indexes a status with its hashtags for efficient search
func (r *HashtagRepository) IndexStatusHashtags(_ context.Context, statusID string, _ string, _ string, _ string, _ string, hashtags []string, _ time.Time, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, tag := range hashtags {
		normalized := strings.ToLower(tag)
		r.statusIndex[normalized] = append(r.statusIndex[normalized], statusID)

		if _, exists := r.hashtags[normalized]; !exists {
			r.hashtags[normalized] = &storage.Hashtag{
				Name: normalized,
			}
		}
	}
	return nil
}

// RemoveStatusFromHashtagIndex removes a status from all hashtag indexes
func (r *HashtagRepository) RemoveStatusFromHashtagIndex(_ context.Context, statusID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for tag, statuses := range r.statusIndex {
		filtered := make([]string, 0, len(statuses))
		for _, id := range statuses {
			if id != statusID {
				filtered = append(filtered, id)
			}
		}
		r.statusIndex[tag] = filtered
	}
	return nil
}

// GetHashtagInfo retrieves information about a specific hashtag
func (r *HashtagRepository) GetHashtagInfo(_ context.Context, hashtag string) (*storage.Hashtag, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	normalized := strings.ToLower(hashtag)
	info, exists := r.hashtags[normalized]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return info, nil
}

// GetHashtagUsageHistory retrieves recent usage history for a hashtag
func (r *HashtagRepository) GetHashtagUsageHistory(_ context.Context, _ string, days int) ([]int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]int64, days)
	return result, nil
}

// GetHashtagActivity retrieves activities for a hashtag since a specific time
func (r *HashtagRepository) GetHashtagActivity(_ context.Context, hashtag string, since time.Time) ([]*storage.Activity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	normalized := strings.ToLower(hashtag)
	activities := r.activities[normalized]
	var result []*storage.Activity
	for _, a := range activities {
		if a.Published.After(since) {
			result = append(result, a)
		}
	}
	return result, nil
}

// GetHashtagStats retrieves hashtag statistics
func (r *HashtagRepository) GetHashtagStats(_ context.Context, hashtag string) (any, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	normalized := strings.ToLower(hashtag)
	count := len(r.statusIndex[normalized])
	return map[string]int{"count": count}, nil
}

// GetHashtagTimelineAdvanced retrieves hashtag timeline with advanced filtering
func (r *HashtagRepository) GetHashtagTimelineAdvanced(_ context.Context, _ string, _ *string, _ int, _ string) ([]*storage.StatusSearchResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return []*storage.StatusSearchResult{}, nil
}

// GetMultiHashtagTimeline retrieves timeline for multiple hashtags
func (r *HashtagRepository) GetMultiHashtagTimeline(_ context.Context, _ []string, _ *string, _ int, _ string) ([]*storage.StatusSearchResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return []*storage.StatusSearchResult{}, nil
}

// GetSuggestedHashtags gets suggested hashtags for a user
func (r *HashtagRepository) GetSuggestedHashtags(_ context.Context, _ string, _ int) ([]*storage.HashtagSearchResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return []*storage.HashtagSearchResult{}, nil
}

// FollowHashtag creates a hashtag follow relationship
func (r *HashtagRepository) FollowHashtag(_ context.Context, userID, hashtag string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := userID + ":" + strings.ToLower(hashtag)
	r.follows[key] = &models.HashtagFollow{
		UserID:    userID,
		Hashtag:   strings.ToLower(hashtag),
		CreatedAt: time.Now(),
	}
	return nil
}

// UnfollowHashtag removes a hashtag follow relationship
func (r *HashtagRepository) UnfollowHashtag(_ context.Context, userID, hashtag string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := userID + ":" + strings.ToLower(hashtag)
	delete(r.follows, key)
	return nil
}

// IsFollowingHashtag checks if a user is following a hashtag
func (r *HashtagRepository) IsFollowingHashtag(_ context.Context, userID, hashtag string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := userID + ":" + strings.ToLower(hashtag)
	_, exists := r.follows[key]
	return exists, nil
}

// GetHashtagFollow retrieves the hashtag follow record for a user
func (r *HashtagRepository) GetHashtagFollow(_ context.Context, userID string, hashtag string) (*models.HashtagFollow, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := userID + ":" + strings.ToLower(hashtag)
	follow, exists := r.follows[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return follow, nil
}

// GetHashtagMute retrieves the hashtag mute record for a user
func (r *HashtagRepository) GetHashtagMute(_ context.Context, userID string, hashtag string) (*models.HashtagMute, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := userID + ":" + strings.ToLower(hashtag)
	mute, exists := r.mutes[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return mute, nil
}

// Clear clears all data (test helper)
func (r *HashtagRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.hashtags = make(map[string]*storage.Hashtag)
	r.statusIndex = make(map[string][]string)
	r.follows = make(map[string]*models.HashtagFollow)
	r.mutes = make(map[string]*models.HashtagMute)
	r.activities = make(map[string][]*storage.Activity)
}

// Ensure HashtagRepository implements interfaces.HashtagRepository
var _ interfaces.HashtagRepository = (*HashtagRepository)(nil)
