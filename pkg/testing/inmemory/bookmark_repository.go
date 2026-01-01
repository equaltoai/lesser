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
)

// BookmarkRepository is a thread-safe in-memory implementation of interfaces.BookmarkRepository.
type BookmarkRepository struct {
	mu sync.RWMutex

	// Bookmarks: key = "username:objectID"
	bookmarks map[string]*models.Bookmark

	// Index by user: username -> []bookmarkKey
	byUser map[string][]string

	// Index by object: objectID -> []bookmarkKey
	byObject map[string][]string
}

// NewBookmarkRepository creates a new in-memory bookmark repository
func NewBookmarkRepository() *BookmarkRepository {
	return &BookmarkRepository{
		bookmarks: make(map[string]*models.Bookmark),
		byUser:    make(map[string][]string),
		byObject:  make(map[string][]string),
	}
}

// bookmarkKey generates a unique key for a bookmark
func bookmarkKey(username, objectID string) string {
	return fmt.Sprintf("%s:%s", username, objectID)
}

// CreateBookmark creates a new bookmark for a user
func (r *BookmarkRepository) CreateBookmark(_ context.Context, username, objectID string) (*models.Bookmark, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := bookmarkKey(username, objectID)

	// Check if already exists
	if existing, exists := r.bookmarks[key]; exists {
		return existing, nil
	}

	now := time.Now()
	bookmark := &models.Bookmark{
		PK:        fmt.Sprintf("BOOKMARK#%s", username),
		SK:        fmt.Sprintf("TIME#%s#%s", now.Format(time.RFC3339Nano), objectID),
		Username:  username,
		ObjectID:  objectID,
		CreatedAt: now,
	}

	r.bookmarks[key] = bookmark
	r.byUser[username] = append(r.byUser[username], key)
	r.byObject[objectID] = append(r.byObject[objectID], key)

	return bookmark, nil
}

// DeleteBookmark removes a bookmark
func (r *BookmarkRepository) DeleteBookmark(_ context.Context, username, objectID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := bookmarkKey(username, objectID)

	if _, exists := r.bookmarks[key]; !exists {
		return nil // Idempotent
	}

	delete(r.bookmarks, key)
	r.byUser[username] = removeBookmarkKeyFromSlice(r.byUser[username], key)
	r.byObject[objectID] = removeBookmarkKeyFromSlice(r.byObject[objectID], key)

	return nil
}

// GetBookmark retrieves a specific bookmark
func (r *BookmarkRepository) GetBookmark(_ context.Context, username, objectID string) (*models.Bookmark, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := bookmarkKey(username, objectID)
	bookmark, exists := r.bookmarks[key]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return bookmark, nil
}

// GetUserBookmarks retrieves all bookmarks for a user with pagination
func (r *BookmarkRepository) GetUserBookmarks(_ context.Context, username string, limit int, cursor string) ([]*models.Bookmark, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := r.byUser[username]
	if len(keys) == 0 {
		return []*models.Bookmark{}, "", nil
	}

	// Sort by creation time (descending)
	sortedBookmarks := make([]*models.Bookmark, 0, len(keys))
	for _, key := range keys {
		if b, exists := r.bookmarks[key]; exists {
			sortedBookmarks = append(sortedBookmarks, b)
		}
	}
	sort.Slice(sortedBookmarks, func(i, j int) bool {
		return sortedBookmarks[i].CreatedAt.After(sortedBookmarks[j].CreatedAt)
	})

	safeLimit := clampBookmarkLimit(limit)

	// Find start index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, b := range sortedBookmarks {
			if b.SK == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var results []*models.Bookmark
	var nextCursor string

	for i := startIdx; i < len(sortedBookmarks) && len(results) < safeLimit; i++ {
		results = append(results, sortedBookmarks[i])
	}

	if startIdx+safeLimit < len(sortedBookmarks) && len(results) > 0 {
		nextCursor = results[len(results)-1].SK
	}

	return results, nextCursor, nil
}

// IsBookmarked checks if a user has bookmarked an object
func (r *BookmarkRepository) IsBookmarked(_ context.Context, username, objectID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := bookmarkKey(username, objectID)
	_, exists := r.bookmarks[key]
	return exists, nil
}

// CountUserBookmarks returns the total number of bookmarks by a user
func (r *BookmarkRepository) CountUserBookmarks(_ context.Context, username string) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return int64(len(r.byUser[username])), nil
}

// CheckBookmarksForStatuses returns a map of statusID -> bookmarked for the provided IDs
func (r *BookmarkRepository) CheckBookmarksForStatuses(_ context.Context, username string, statusIDs []string) (map[string]bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]bool)
	for _, statusID := range statusIDs {
		key := bookmarkKey(username, statusID)
		_, exists := r.bookmarks[key]
		result[statusID] = exists
	}

	return result, nil
}

// AddBookmark provides Storage interface compatibility
func (r *BookmarkRepository) AddBookmark(ctx context.Context, username, objectID string) error {
	_, err := r.CreateBookmark(ctx, username, objectID)
	return err
}

// RemoveBookmark provides Storage interface compatibility
func (r *BookmarkRepository) RemoveBookmark(ctx context.Context, username, objectID string) error {
	return r.DeleteBookmark(ctx, username, objectID)
}

// GetBookmarks provides Storage interface compatibility - returns storage.Bookmark format
func (r *BookmarkRepository) GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]*storage.Bookmark, string, error) {
	bookmarks, nextCursor, err := r.GetUserBookmarks(ctx, username, limit, cursor)
	if err != nil {
		return nil, "", err
	}

	result := make([]*storage.Bookmark, len(bookmarks))
	for i, b := range bookmarks {
		result[i] = &storage.Bookmark{
			Username:  b.Username,
			ObjectID:  b.ObjectID,
			CreatedAt: b.CreatedAt,
		}
	}

	return result, nextCursor, nil
}

// CascadeDeleteUserBookmarks deletes all bookmarks for a user
func (r *BookmarkRepository) CascadeDeleteUserBookmarks(_ context.Context, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	keys := r.byUser[username]
	for _, key := range keys {
		if b, exists := r.bookmarks[key]; exists {
			r.byObject[b.ObjectID] = removeBookmarkKeyFromSlice(r.byObject[b.ObjectID], key)
			delete(r.bookmarks, key)
		}
	}
	delete(r.byUser, username)

	return nil
}

// CascadeDeleteObjectBookmarks deletes all bookmarks for an object
func (r *BookmarkRepository) CascadeDeleteObjectBookmarks(_ context.Context, objectID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	keys := r.byObject[objectID]
	for _, key := range keys {
		if b, exists := r.bookmarks[key]; exists {
			r.byUser[b.Username] = removeBookmarkKeyFromSlice(r.byUser[b.Username], key)
			delete(r.bookmarks, key)
		}
	}
	delete(r.byObject, objectID)

	return nil
}

// Helper functions

func removeBookmarkKeyFromSlice(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

func clampBookmarkLimit(limit int) int {
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
func (r *BookmarkRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.bookmarks = make(map[string]*models.Bookmark)
	r.byUser = make(map[string][]string)
	r.byObject = make(map[string][]string)
}

// GetBookmarkCount returns the number of bookmarks (test helper)
func (r *BookmarkRepository) GetBookmarkCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.bookmarks)
}

// Ensure BookmarkRepository implements interfaces.BookmarkRepository
var _ interfaces.BookmarkRepository = (*BookmarkRepository)(nil)
