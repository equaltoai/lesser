// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// BookmarkRepository defines the interface for bookmark operations.
// This handles user bookmarks for statuses and other content.
type BookmarkRepository interface {
	// CreateBookmark creates a new bookmark for a user
	CreateBookmark(ctx context.Context, username, objectID string) (*models.Bookmark, error)

	// DeleteBookmark removes a bookmark
	DeleteBookmark(ctx context.Context, username, objectID string) error

	// GetBookmark retrieves a specific bookmark
	GetBookmark(ctx context.Context, username, objectID string) (*models.Bookmark, error)

	// GetUserBookmarks retrieves all bookmarks for a user with pagination
	GetUserBookmarks(ctx context.Context, username string, limit int, cursor string) ([]*models.Bookmark, string, error)

	// IsBookmarked checks if a user has bookmarked an object
	IsBookmarked(ctx context.Context, username, objectID string) (bool, error)

	// CountUserBookmarks returns the total number of bookmarks by a user
	CountUserBookmarks(ctx context.Context, username string) (int64, error)

	// CheckBookmarksForStatuses returns a map of statusID -> bookmarked for the provided IDs
	CheckBookmarksForStatuses(ctx context.Context, username string, statusIDs []string) (map[string]bool, error)

	// Storage interface compatibility methods

	// AddBookmark provides Storage interface compatibility
	AddBookmark(ctx context.Context, username, objectID string) error

	// RemoveBookmark provides Storage interface compatibility
	RemoveBookmark(ctx context.Context, username, objectID string) error

	// GetBookmarks provides Storage interface compatibility - returns storage.Bookmark format
	GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]*storage.Bookmark, string, error)

	// CascadeDeleteUserBookmarks deletes all bookmarks for a user
	CascadeDeleteUserBookmarks(ctx context.Context, username string) error

	// CascadeDeleteObjectBookmarks deletes all bookmarks for an object
	CascadeDeleteObjectBookmarks(ctx context.Context, objectID string) error
}
