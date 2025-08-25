package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// BookmarkRepository implements bookmark operations using enhanced DynamORM patterns
type BookmarkRepository struct {
	*EnhancedBaseRepository[*models.Bookmark]
}

// NewBookmarkRepository creates a new bookmark repository with enhanced functionality
func NewBookmarkRepository(db core.DB, tableName string, logger *zap.Logger) *BookmarkRepository {
	// Create enhanced repository optimized for bookmark operations
	enhancedRepo := NewEnhancedBaseRepository[*models.Bookmark](db, tableName, logger, nil, "BookmarkRepository", "bookmark")

	// Set up enhanced services for bookmark operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Bookmarks are frequently checked
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for user notifications

	return &BookmarkRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// NewBookmarkRepositoryWithCostTracking creates a new bookmark repository with cost tracking
func NewBookmarkRepositoryWithCostTracking(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *BookmarkRepository {
	// Create enhanced repository with cost tracking
	enhancedRepo := NewEnhancedBaseRepository[*models.Bookmark](db, tableName, logger, costService, "BookmarkRepository", "bookmark")

	// Set up enhanced services for bookmark operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Bookmarks are frequently checked
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for user notifications

	return &BookmarkRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreateBookmark creates a new bookmark
func (r *BookmarkRepository) CreateBookmark(ctx context.Context, username, objectID string) (*models.Bookmark, error) {
	bookmark := &models.Bookmark{
		Username:  username,
		ObjectID:  objectID,
		CreatedAt: time.Now(),
	}
	if err := bookmark.UpdateKeys(); err != nil {
		return nil, ErrorHandler.HandleCreateError(err, EntityBookmark, "key generation")
	}

	// Use enhanced validation and creation with automatic permission checking and event emission
	if err := r.ValidateAndCreate(ctx, bookmark); err != nil {
		// Check if it's a duplicate key error (already bookmarked)
		if errors.IsConditionFailed(err) {
			r.logger.Debug("bookmark already exists",
				zap.String("username", username),
				zap.String("object_id", objectID),
				zap.Bool("validation_enabled", r.HasValidation()),
				zap.Bool("events_enabled", r.HasEvents()))
			return bookmark, nil
		}
		r.logger.Error("failed to create bookmark with enhanced validation",
			zap.String("username", username),
			zap.String("object_id", objectID),
			zap.Bool("validation_enabled", r.HasValidation()),
			zap.Bool("events_enabled", r.HasEvents()),
			zap.Error(err))
		return nil, ErrorHandler.HandleCreateError(err, EntityBookmark, objectID)
	}

	r.logger.Info("created bookmark with enhanced patterns",
		zap.String("bookmark_id", fmt.Sprintf("%s:%s", username, objectID)),
		zap.String("username", username),
		zap.String("object_id", objectID))

	return bookmark, nil
}

// DeleteBookmark removes a bookmark
func (r *BookmarkRepository) DeleteBookmark(ctx context.Context, username, objectID string) error {
	// We need to find the bookmark first since SK includes timestamp
	var bookmarks []models.Bookmark
	err := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		Where("PK", "=", fmt.Sprintf("BOOKMARK#%s", username)).
		Where("SK", "CONTAINS", objectID).
		All(&bookmarks)

	if err != nil {
		if errors.IsNotFound(err) {
			// No bookmark found, operation is idempotent
			return nil
		}
		r.logger.Error("failed to find bookmark for deletion",
			zap.String("username", username),
			zap.String("object_id", objectID),
			zap.Error(err))
		return ErrorHandler.HandleQueryError(err, EntityBookmark, "deletion search")
	}

	// Delete all found bookmarks (should typically be 0 or 1)
	for _, bookmark := range bookmarks {
		err = r.Delete(ctx, bookmark.PK, bookmark.SK)
		if err != nil {
			r.logger.Error("failed to delete bookmark",
				zap.String("pk", bookmark.PK),
				zap.String("sk", bookmark.SK),
				zap.Error(err))
			return ErrorHandler.HandleDeleteError(err, EntityBookmark, objectID)
		}
	}

	r.logger.Info("deleted bookmark",
		zap.String("username", username),
		zap.String("object_id", objectID),
		zap.Int("deleted_count", len(bookmarks)))

	return nil
}

// GetBookmark retrieves a specific bookmark
func (r *BookmarkRepository) GetBookmark(ctx context.Context, username, objectID string) (*models.Bookmark, error) {
	var bookmarks []models.Bookmark
	err := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		Where("PK", "=", fmt.Sprintf("BOOKMARK#%s", username)).
		Where("SK", "CONTAINS", objectID).
		Limit(1).
		All(&bookmarks)

	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, EntityBookmark, objectID)
	}

	if err := common.ValidateSliceNotEmpty("bookmarks", bookmarks); err != nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityBookmark, objectID)
	}

	return &bookmarks[0], nil
}

// GetUserBookmarks retrieves all bookmarks for a user with pagination
func (r *BookmarkRepository) GetUserBookmarks(ctx context.Context, username string, limit int, cursor string) ([]*models.Bookmark, string, error) {
	pk := fmt.Sprintf("BOOKMARK#%s", username)

	opts := BasePaginationOptions{
		Limit:  limit,
		Cursor: cursor,
		Order:  "DESC", // Most recent first
	}

	result, err := r.FindWithPagination(ctx, pk, opts)
	if err != nil {
		r.logger.Error("failed to get user bookmarks",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, EntityBookmark, "user bookmarks")
	}

	// Convert to pointer slice
	bookmarkPtrs := make([]*models.Bookmark, len(result.Items))
	copy(bookmarkPtrs, result.Items)

	return bookmarkPtrs, result.NextCursor, nil
}

// IsBookmarked checks if a user has bookmarked an object
func (r *BookmarkRepository) IsBookmarked(ctx context.Context, username, objectID string) (bool, error) {
	bookmark, err := r.GetBookmark(ctx, username, objectID)
	if err != nil {
		if err.Error() == "bookmark not found" {
			return false, nil
		}
		return false, err
	}
	return bookmark != nil, nil
}

// CountUserBookmarks returns the total number of bookmarks by a user
func (r *BookmarkRepository) CountUserBookmarks(ctx context.Context, username string) (int64, error) {
	pk := fmt.Sprintf("BOOKMARK#%s", username)
	count, err := r.Count(ctx, pk)
	if err != nil {
		r.logger.Error("failed to count user bookmarks",
			zap.String("username", username),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityBookmark, "count")
	}
	return int64(count), nil
}

// Storage interface compatibility methods

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

	// Convert to storage.Bookmark format
	result := make([]*storage.Bookmark, len(bookmarks))
	for i, bookmark := range bookmarks {
		result[i] = &storage.Bookmark{
			Username:  bookmark.Username,
			ObjectID:  bookmark.ObjectID,
			CreatedAt: bookmark.CreatedAt,
		}
	}

	return result, nextCursor, nil
}

// CascadeDeleteUserBookmarks deletes all bookmarks for a user
func (r *BookmarkRepository) CascadeDeleteUserBookmarks(ctx context.Context, username string) error {
	pk := fmt.Sprintf("BOOKMARK#%s", username)

	// Keep deleting in batches until no more bookmarks remain
	for {
		// Query bookmarks with a reasonable batch size
		bookmarks, err := r.Query(ctx, pk, 100)
		if err != nil {
			return ErrorHandler.HandleQueryError(err, EntityBookmark, "cascade deletion")
		}

		// If no bookmarks found, we're done
		if err := common.ValidateSliceNotEmpty("bookmarks", bookmarks); err != nil {
			break
		}

		// Prepare keys for batch deletion
		keys := make([]struct{ PK, SK string }, len(bookmarks))
		for i, bookmark := range bookmarks {
			keys[i] = struct{ PK, SK string }{PK: bookmark.GetPK(), SK: bookmark.GetSK()}
		}

		// Use batch delete
		if err := r.BatchDelete(ctx, keys); err != nil {
			r.logger.Warn("failed to batch delete bookmarks during cascade",
				zap.String("username", username),
				zap.Int("count", len(keys)),
				zap.Error(err))
			// Continue with individual deletes as fallback
			for _, key := range keys {
				if delErr := r.Delete(ctx, key.PK, key.SK); delErr != nil {
					r.logger.Warn("failed to delete bookmark during cascade",
						zap.String("pk", key.PK),
						zap.String("sk", key.SK),
						zap.Error(delErr))
				}
			}
		}

		// If we got less than the limit, we're done
		if len(bookmarks) < 100 {
			break
		}
	}

	r.logger.Info("cascade deleted bookmarks for user",
		zap.String("username", username))

	return nil
}

// CascadeDeleteObjectBookmarks deletes all bookmarks for an object (when object is deleted)
func (r *BookmarkRepository) CascadeDeleteObjectBookmarks(ctx context.Context, objectID string) error {
	// Since bookmark keys are BOOKMARK#{username} / timestamp#{objectID},
	// we need to scan for bookmarks containing the objectID
	// This is less efficient than a GSI but works with current schema

	r.logger.Info("starting cascade delete for object bookmarks",
		zap.String("object_id", objectID))

	deletedCount := 0

	// Use scan operation to find all bookmarks with the specified objectID
	// We need to scan through all bookmark records and filter by SK containing objectID
	var bookmarks []models.Bookmark

	// Perform scan with filter on PK prefix to limit to bookmark records only
	err := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		Where("PK", "BEGINS_WITH", "BOOKMARK#").
		Where("SK", "CONTAINS", fmt.Sprintf("#%s", objectID)).
		Scan(&bookmarks)

	if err != nil {
		r.logger.Error("failed to scan bookmarks for object deletion",
			zap.String("object_id", objectID),
			zap.Error(err))
		return ErrorHandler.HandleQueryError(err, EntityBookmark, "cascade scan")
	}

	// Filter results to ensure exact objectID match (since CONTAINS might match partial IDs)
	var exactMatches []models.Bookmark
	for _, bookmark := range bookmarks {
		if bookmark.ObjectID == objectID {
			exactMatches = append(exactMatches, bookmark)
		}
	}

	if len(exactMatches) == 0 {
		r.logger.Debug("no bookmarks found for object",
			zap.String("object_id", objectID))
		return nil
	}

	// Delete matching bookmarks in batches
	const batchSize = 25 // DynamoDB batch write limit
	for i := 0; i < len(exactMatches); i += batchSize {
		end := i + batchSize
		if end > len(exactMatches) {
			end = len(exactMatches)
		}

		batch := exactMatches[i:end]
		keys := make([]struct{ PK, SK string }, len(batch))
		for j, bookmark := range batch {
			keys[j] = struct{ PK, SK string }{PK: bookmark.PK, SK: bookmark.SK}
		}

		// Use batch delete
		if err := r.BatchDelete(ctx, keys); err != nil {
			r.logger.Warn("failed to batch delete bookmarks during object cascade",
				zap.String("object_id", objectID),
				zap.Int("batch_size", len(keys)),
				zap.Error(err))
			// Continue with individual deletes as fallback
			for _, key := range keys {
				if delErr := r.Delete(ctx, key.PK, key.SK); delErr != nil {
					r.logger.Warn("failed to delete individual bookmark during cascade",
						zap.String("object_id", objectID),
						zap.String("pk", key.PK),
						zap.String("sk", key.SK),
						zap.Error(delErr))
				} else {
					deletedCount++
				}
			}
		} else {
			deletedCount += len(keys)
		}
	}

	r.logger.Info("cascade deleted object bookmarks",
		zap.String("object_id", objectID),
		zap.Int("deleted_count", deletedCount))

	return nil
}
