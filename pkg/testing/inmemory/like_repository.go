// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
)

// LikeRepository is a thread-safe in-memory implementation of interfaces.LikeRepository.
type LikeRepository struct {
	mu sync.RWMutex

	// Likes: key = "actor:object"
	likes map[string]*models.Like

	// Index by actor: actorID -> []likeKey
	byActor map[string][]string

	// Index by object: objectID -> []likeKey
	byObject map[string][]string

	// Tombstones: key = objectID
	tombstones map[string]*storage.Tombstone

	// Reblog counts: key = objectID
	reblogCounts map[string]int64

	// Reblogs: key = "actorID:statusID"
	reblogs map[string]bool
}

// NewLikeRepository creates a new in-memory like repository
func NewLikeRepository() *LikeRepository {
	return &LikeRepository{
		likes:        make(map[string]*models.Like),
		byActor:      make(map[string][]string),
		byObject:     make(map[string][]string),
		tombstones:   make(map[string]*storage.Tombstone),
		reblogCounts: make(map[string]int64),
		reblogs:      make(map[string]bool),
	}
}

// likeKey generates a unique key for a like
func likeKey(actor, object string) string {
	return fmt.Sprintf("%s:%s", actor, object)
}

// CreateLike creates a new like
func (r *LikeRepository) CreateLike(_ context.Context, actor, object, statusAuthorID string) (*models.Like, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := likeKey(actor, object)

	// Check if already exists
	if existing, exists := r.likes[key]; exists {
		return existing, nil
	}

	like := models.NewLike(actor, object, statusAuthorID)
	like.ID = uuid.New().String()

	r.likes[key] = like
	r.byActor[actor] = append(r.byActor[actor], key)
	r.byObject[object] = append(r.byObject[object], key)

	return like, nil
}

// DeleteLike removes a like
func (r *LikeRepository) DeleteLike(_ context.Context, actor, object string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := likeKey(actor, object)

	if _, exists := r.likes[key]; !exists {
		return nil // Idempotent
	}

	delete(r.likes, key)
	r.byActor[actor] = removeLikeKeyFromSlice(r.byActor[actor], key)
	r.byObject[object] = removeLikeKeyFromSlice(r.byObject[object], key)

	return nil
}

// GetLike retrieves a specific like
func (r *LikeRepository) GetLike(_ context.Context, actor, object string) (*models.Like, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := likeKey(actor, object)
	like, exists := r.likes[key]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return like, nil
}

// GetObjectLikes retrieves all likes for an object with pagination
func (r *LikeRepository) GetObjectLikes(_ context.Context, objectID string, limit int, cursor string) ([]*models.Like, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := r.byObject[objectID]
	if len(keys) == 0 {
		return []*models.Like{}, "", nil
	}

	// Sort by creation time
	sortedLikes := make([]*models.Like, 0, len(keys))
	for _, key := range keys {
		if l, exists := r.likes[key]; exists {
			sortedLikes = append(sortedLikes, l)
		}
	}
	sort.Slice(sortedLikes, func(i, j int) bool {
		return sortedLikes[i].CreatedAt.Before(sortedLikes[j].CreatedAt)
	})

	safeLimit := clampLikeLimit(limit)

	// Find start index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, l := range sortedLikes {
			if l.ID == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var results []*models.Like
	var nextCursor string

	for i := startIdx; i < len(sortedLikes) && len(results) < safeLimit; i++ {
		results = append(results, sortedLikes[i])
	}

	if startIdx+safeLimit < len(sortedLikes) && len(results) > 0 {
		nextCursor = results[len(results)-1].ID
	}

	return results, nextCursor, nil
}

// GetActorLikes retrieves all likes by an actor with pagination
func (r *LikeRepository) GetActorLikes(_ context.Context, actorID string, limit int, cursor string) ([]*models.Like, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := r.byActor[actorID]
	if len(keys) == 0 {
		return []*models.Like{}, "", nil
	}

	// Sort by creation time
	sortedLikes := make([]*models.Like, 0, len(keys))
	for _, key := range keys {
		if l, exists := r.likes[key]; exists {
			sortedLikes = append(sortedLikes, l)
		}
	}
	sort.Slice(sortedLikes, func(i, j int) bool {
		return sortedLikes[i].CreatedAt.Before(sortedLikes[j].CreatedAt)
	})

	safeLimit := clampLikeLimit(limit)

	// Find start index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, l := range sortedLikes {
			if l.ID == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var results []*models.Like
	var nextCursor string

	for i := startIdx; i < len(sortedLikes) && len(results) < safeLimit; i++ {
		results = append(results, sortedLikes[i])
	}

	if startIdx+safeLimit < len(sortedLikes) && len(results) > 0 {
		nextCursor = results[len(results)-1].ID
	}

	return results, nextCursor, nil
}

// CountActorLikes returns the total number of likes by an actor
func (r *LikeRepository) CountActorLikes(_ context.Context, actorID string) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return int64(len(r.byActor[actorID])), nil
}

// HasLiked checks if an actor has liked an object
func (r *LikeRepository) HasLiked(_ context.Context, actor, object string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := likeKey(actor, object)
	_, exists := r.likes[key]
	return exists, nil
}

// CascadeDeleteLikes deletes all likes for an object
func (r *LikeRepository) CascadeDeleteLikes(_ context.Context, objectID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	keys := r.byObject[objectID]
	for _, key := range keys {
		if l, exists := r.likes[key]; exists {
			r.byActor[l.Actor] = removeLikeKeyFromSlice(r.byActor[l.Actor], key)
			delete(r.likes, key)
		}
	}
	delete(r.byObject, objectID)

	return nil
}

// TombstoneObject creates a tombstone for a deleted object
func (r *LikeRepository) TombstoneObject(_ context.Context, objectID string, deletedBy string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tombstones[objectID] = &storage.Tombstone{
		ID:         objectID,
		Type:       "Tombstone",
		FormerType: "Object",
		Deleted:    time.Now(),
		DeletedBy:  deletedBy,
	}

	return nil
}

// GetTombstone retrieves a tombstone by object ID
func (r *LikeRepository) GetTombstone(_ context.Context, objectID string) (*storage.Tombstone, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tombstone, exists := r.tombstones[objectID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return tombstone, nil
}

// GetLikeCount counts likes for a status
func (r *LikeRepository) GetLikeCount(_ context.Context, statusID string) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return int64(len(r.byObject[statusID])), nil
}

// GetBoostCount counts boosts/announces for a status
func (r *LikeRepository) GetBoostCount(_ context.Context, statusID string) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.reblogCounts[statusID], nil
}

// IncrementReblogCount increments the reblog count on a status
func (r *LikeRepository) IncrementReblogCount(_ context.Context, objectID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.reblogCounts[objectID]++
	return nil
}

// HasReblogged checks if a user has reblogged/boosted a specific status
func (r *LikeRepository) HasReblogged(_ context.Context, actorID, statusID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", actorID, statusID)
	return r.reblogs[key], nil
}

// CountForObject provides Storage interface compatibility for CountObjectLikes
func (r *LikeRepository) CountForObject(ctx context.Context, objectID string) (int64, error) {
	return r.GetLikeCount(ctx, objectID)
}

// GetForObject provides Storage interface compatibility for GetObjectLikes
func (r *LikeRepository) GetForObject(ctx context.Context, objectID string, limit int, cursor string) ([]*models.Like, string, error) {
	return r.GetObjectLikes(ctx, objectID, limit, cursor)
}

// GetLikedObjects provides Storage interface compatibility
func (r *LikeRepository) GetLikedObjects(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Like, string, error) {
	return r.GetActorLikes(ctx, actorID, limit, cursor)
}

// Helper functions

func removeLikeKeyFromSlice(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

func clampLikeLimit(limit int) int {
	const defaultLimit = 20
	const maxLimit = 100

	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// Test helper methods

// Clear clears all data (test helper)
func (r *LikeRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.likes = make(map[string]*models.Like)
	r.byActor = make(map[string][]string)
	r.byObject = make(map[string][]string)
	r.tombstones = make(map[string]*storage.Tombstone)
	r.reblogCounts = make(map[string]int64)
	r.reblogs = make(map[string]bool)
}

// GetLikeCount returns the number of likes (test helper)
func (r *LikeRepository) GetLikeCountTotal() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.likes)
}

// SetReblog sets a reblog status (test helper)
func (r *LikeRepository) SetReblog(actorID, statusID string, reblogged bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%s:%s", actorID, statusID)
	if reblogged {
		r.reblogs[key] = true
	} else {
		delete(r.reblogs, key)
	}
}

// Ensure LikeRepository implements interfaces.LikeRepository
var _ interfaces.LikeRepository = (*LikeRepository)(nil)
