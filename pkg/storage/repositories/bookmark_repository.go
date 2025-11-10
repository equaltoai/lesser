package repositories

import (
	"context"
	stdErrors "errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/core"
	errors "github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// BookmarkRepository implements bookmark operations using enhanced DynamORM patterns
type BookmarkRepository struct {
	*EnhancedBaseRepository[*models.Bookmark]

	getObjectBookmarkFn  func(ctx context.Context, username, objectID string) (*models.Bookmark, error)
	findTimeBookmarkFn   func(ctx context.Context, username, objectID string) (*models.Bookmark, error)
	transactWriteFn      func(ctx context.Context, fn func(core.TransactionBuilder) error) error
	batchGetFn           func(ctx context.Context, keys []any) ([]*models.Bookmark, error)
	queryTimeBookmarksFn func(ctx context.Context, username string, limit int, cursor string) ([]models.Bookmark, string, error)
}

type transactionalDB interface {
	core.DB
	TransactWrite(ctx context.Context, fn func(core.TransactionBuilder) error) error
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

	repo := &BookmarkRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
	repo.initHooks()
	return repo
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

	repo := &BookmarkRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
	repo.initHooks()
	return repo
}

func (r *BookmarkRepository) initHooks() {
	r.getObjectBookmarkFn = r.dynamoGetObjectBookmark
	r.findTimeBookmarkFn = r.dynamoFindTimeBookmarkByObject
	r.transactWriteFn = r.transactWrite
	r.batchGetFn = r.batchGetBookmarks
	r.queryTimeBookmarksFn = r.queryUnlockedTimeBookmarks
}

// CreateBookmark creates a new bookmark using the TIME/OBJECT dual-write pattern.
func (r *BookmarkRepository) CreateBookmark(ctx context.Context, username, objectID string) (*models.Bookmark, error) {
	if existing, err := r.getObjectBookmarkFn(ctx, username, objectID); err == nil {
		return existing, nil
	} else if !errors.IsNotFound(err) {
		return nil, ErrorHandler.HandleGetError(err, EntityBookmark, objectID)
	}

	now := time.Now().UTC()
	var (
		timeRecord        *models.Bookmark
		createTimeRecord  bool
		legacyTimeAttempt bool
	)

	if legacy, legacyErr := r.findTimeBookmarkFn(ctx, username, objectID); legacyErr == nil && legacy != nil && legacy.Locked {
		timeRecord = legacy
		createTimeRecord = false
		legacyTimeAttempt = true
	} else {
		var err error
		timeRecord, err = models.NewTimeOrderedBookmark(username, objectID, now)
		if err != nil {
			return nil, ErrorHandler.HandleCreateError(err, EntityBookmark, "time record build")
		}
		createTimeRecord = true
	}

	objectRecord, err := models.NewObjectIndexedBookmark(username, objectID, timeRecord.CreatedAt, timeRecord.SK)
	if err != nil {
		return nil, ErrorHandler.HandleCreateError(err, EntityBookmark, "object record build")
	}

	writeErr := r.transactWriteFn(ctx, func(tx core.TransactionBuilder) error {
		if createTimeRecord {
			tx.Create(timeRecord, dynamorm.IfNotExists())
		}
		tx.Create(objectRecord, dynamorm.IfNotExists())
		if timeRecord.Locked {
			tx.UpdateWithBuilder(newBookmarkKey(timeRecord.PK, timeRecord.SK), func(ub core.UpdateBuilder) error {
				ub.Set("Locked", false)
				return nil
			}, dynamorm.Condition("Locked", "=", true))
		}
		return nil
	})

	if writeErr != nil {
		r.logTransactionError("bookmark dual-write failed", writeErr,
			zap.String("username", username),
			zap.String("object_id", objectID))

		if errors.IsConditionFailed(writeErr) {
			if existing, err := r.getObjectBookmarkFn(ctx, username, objectID); err == nil {
				return existing, nil
			} else if errors.IsNotFound(err) {
				if repaired, repairErr := r.repairLegacyBookmark(ctx, username, objectID); repairErr == nil && repaired != nil {
					return repaired, nil
				}
			} else {
				return nil, ErrorHandler.HandleCreateError(err, EntityBookmark, objectID)
			}
		}

		return nil, ErrorHandler.HandleCreateError(writeErr, EntityBookmark, objectID)
	}

	r.logger.Info("created bookmark with transactional dual-write",
		zap.String("username", username),
		zap.String("object_id", objectID),
		zap.String("time_record_sk", timeRecord.SK),
		zap.Bool("legacy_time_reused", legacyTimeAttempt))

	return objectRecord, nil
}

// DeleteBookmark removes both the OBJECT and TIME bookmark records.
func (r *BookmarkRepository) DeleteBookmark(ctx context.Context, username, objectID string) error {
	objectBookmark, err := r.getObjectBookmarkFn(ctx, username, objectID)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.deleteLegacyTimeBookmark(ctx, username, objectID)
		}
		return ErrorHandler.HandleGetError(err, EntityBookmark, objectID)
	}

	timeSK := objectBookmark.TimeRecordSK
	if timeSK == "" {
		if fallback, findErr := r.findTimeBookmarkFn(ctx, username, objectID); findErr == nil && fallback != nil {
			timeSK = fallback.SK
		} else if findErr != nil && !errors.IsNotFound(findErr) {
			return ErrorHandler.HandleDeleteError(findErr, EntityBookmark, objectID)
		}
	}

	deleteErr := r.transactWriteFn(ctx, func(tx core.TransactionBuilder) error {
		tx.Delete(newBookmarkKey(objectBookmark.PK, objectBookmark.SK))
		if timeSK != "" {
			tx.Delete(newBookmarkKey(objectBookmark.PK, timeSK))
		}
		return nil
	})

	if deleteErr != nil {
		r.logTransactionError("bookmark delete transaction failed", deleteErr,
			zap.String("username", username),
			zap.String("object_id", objectID))
		if errors.IsNotFound(deleteErr) {
			return nil
		}
		return ErrorHandler.HandleDeleteError(deleteErr, EntityBookmark, objectID)
	}

	r.logger.Info("deleted bookmark",
		zap.String("username", username),
		zap.String("object_id", objectID))
	return nil
}

// GetBookmark retrieves a specific bookmark
func (r *BookmarkRepository) GetBookmark(ctx context.Context, username, objectID string) (*models.Bookmark, error) {
	bookmark, err := r.getObjectBookmarkFn(ctx, username, objectID)
	if err != nil {
		if errors.IsNotFound(err) {
			legacy, legacyErr := r.findTimeBookmarkFn(ctx, username, objectID)
			if legacyErr != nil && !errors.IsNotFound(legacyErr) {
				return nil, ErrorHandler.HandleGetError(legacyErr, EntityBookmark, objectID)
			}
			if legacy == nil || legacy.Locked {
				return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityBookmark, objectID)
			}
			return legacy, nil
		}
		return nil, ErrorHandler.HandleGetError(err, EntityBookmark, objectID)
	}

	return bookmark, nil
}

// GetUserBookmarks retrieves all bookmarks for a user with pagination
func (r *BookmarkRepository) GetUserBookmarks(ctx context.Context, username string, limit int, cursor string) ([]*models.Bookmark, string, error) {
	bookmarks, nextCursor, err := r.queryTimeBookmarksFn(ctx, username, limit, cursor)
	if err != nil {
		return nil, "", err
	}

	result := make([]*models.Bookmark, len(bookmarks))
	for i := range bookmarks {
		result[i] = &bookmarks[i]
	}

	return result, nextCursor, nil
}

// IsBookmarked checks if a user has bookmarked an object
func (r *BookmarkRepository) IsBookmarked(ctx context.Context, username, objectID string) (bool, error) {
	if _, err := r.getObjectBookmarkFn(ctx, username, objectID); err != nil {
		if errors.IsNotFound(err) {
			legacy, legacyErr := r.findTimeBookmarkFn(ctx, username, objectID)
			if legacyErr != nil && !errors.IsNotFound(legacyErr) {
				return false, legacyErr
			}
			if legacy != nil && !legacy.Locked {
				return true, nil
			}
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CountUserBookmarks returns the total number of bookmarks by a user
func (r *BookmarkRepository) CountUserBookmarks(ctx context.Context, username string) (int64, error) {
	pk := buildBookmarkPK(username)

	query := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", models.BookmarkSortKeyPrefixTime).
		Filter("Locked", "=", false)

	count, err := query.Count()
	if err != nil {
		r.logger.Error("failed to count user bookmarks",
			zap.String("username", username),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityBookmark, "count")
	}
	return count, nil
}

// CheckBookmarksForStatuses returns a map of statusID -> bookmarked for the provided IDs.
func (r *BookmarkRepository) CheckBookmarksForStatuses(ctx context.Context, username string, statusIDs []string) (map[string]bool, error) {
	result := make(map[string]bool)
	if len(statusIDs) == 0 {
		return result, nil
	}

	pk := buildBookmarkPK(username)
	uniqueIDs := deduplicate(statusIDs)

	for start := 0; start < len(uniqueIDs); start += 100 {
		end := start + 100
		if end > len(uniqueIDs) {
			end = len(uniqueIDs)
		}

		batch := uniqueIDs[start:end]
		keys := make([]any, 0, len(batch))
		for _, statusID := range batch {
			keys = append(keys, dynamorm.NewKeyPair(pk, buildObjectSK(statusID)))
		}

		items, err := r.batchGetFn(ctx, keys)
		if err != nil {
			return nil, ErrorHandler.HandleQueryError(err, EntityBookmark, "batch bookmark lookup")
		}

		for _, bookmark := range items {
			if bookmark == nil {
				continue
			}
			result[bookmark.ObjectID] = true
		}

		for _, statusID := range batch {
			if result[statusID] {
				continue
			}
			fallback, legacyErr := r.findTimeBookmarkFn(ctx, username, statusID)
			if legacyErr != nil && !errors.IsNotFound(legacyErr) {
				return nil, ErrorHandler.HandleQueryError(legacyErr, EntityBookmark, "legacy bookmark lookup")
			}
			if fallback != nil && !fallback.Locked {
				result[statusID] = true
			}
		}
	}

	return result, nil
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

// Helper utilities ----------------------------------------------------------

func buildBookmarkPK(username string) string {
	return fmt.Sprintf("%s#%s", models.BookmarkPartitionPrefix, username)
}

func buildObjectSK(objectID string) string {
	return fmt.Sprintf("%s#%s", models.BookmarkSortKeyPrefixObject, objectID)
}

func sanitizeLimit(limit, defaultLimit, maxSize int) int {
	switch {
	case limit <= 0:
		return defaultLimit
	case limit > maxSize:
		return maxSize
	default:
		return limit
	}
}

func deduplicate(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		result = append(result, v)
	}
	return result
}

func (r *BookmarkRepository) queryUnlockedTimeBookmarks(ctx context.Context, username string, limit int, cursor string) ([]models.Bookmark, string, error) {
	pk := buildBookmarkPK(username)
	limit = sanitizeLimit(limit, 20, 100)

	var bookmarks []models.Bookmark
	query := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", models.BookmarkSortKeyPrefixTime).
		Filter("Locked", "=", false).
		OrderBy("SK", SortOrderDesc).
		Limit(limit + 1)

	if cursor != "" {
		query = query.Where("SK", "<", cursor)
	}

	if err := query.All(&bookmarks); err != nil {
		r.logger.Error("failed to get user bookmarks",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, EntityBookmark, "user bookmarks")
	}

	hasMore := len(bookmarks) > limit
	if hasMore {
		bookmarks = bookmarks[:limit]
	}

	nextCursor := ""
	if hasMore && len(bookmarks) > 0 {
		nextCursor = bookmarks[len(bookmarks)-1].SK
	}

	return bookmarks, nextCursor, nil
}

func (r *BookmarkRepository) repairLegacyBookmark(ctx context.Context, username, objectID string) (*models.Bookmark, error) {
	legacy, err := r.findTimeBookmarkFn(ctx, username, objectID)
	if err != nil || legacy == nil {
		return nil, err
	}

	objectRecord, err := models.NewObjectIndexedBookmark(username, objectID, legacy.CreatedAt, legacy.SK)
	if err != nil {
		return nil, err
	}

	recoverErr := r.transactWriteFn(ctx, func(tx core.TransactionBuilder) error {
		tx.Create(objectRecord, dynamorm.IfNotExists())
		if legacy.Locked {
			tx.UpdateWithBuilder(newBookmarkKey(legacy.PK, legacy.SK), func(ub core.UpdateBuilder) error {
				ub.Set("Locked", false)
				return nil
			}, dynamorm.Condition("Locked", "=", true))
		}
		return nil
	})
	if recoverErr != nil {
		return nil, recoverErr
	}

	return objectRecord, nil
}

func (r *BookmarkRepository) deleteLegacyTimeBookmark(ctx context.Context, username, objectID string) error {
	legacy, err := r.findTimeBookmarkFn(ctx, username, objectID)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return ErrorHandler.HandleDeleteError(err, EntityBookmark, objectID)
	}
	if legacy == nil {
		return nil
	}

	deleteErr := r.transactWriteFn(ctx, func(tx core.TransactionBuilder) error {
		tx.Delete(newBookmarkKey(legacy.PK, legacy.SK))
		return nil
	})
	if deleteErr != nil && !errors.IsNotFound(deleteErr) {
		return ErrorHandler.HandleDeleteError(deleteErr, EntityBookmark, objectID)
	}

	r.logger.Info("deleted legacy bookmark time record",
		zap.String("username", username),
		zap.String("object_id", objectID))
	return nil
}

func (r *BookmarkRepository) transactWrite(ctx context.Context, fn func(core.TransactionBuilder) error) error {
	txDB, err := r.transactionalDB()
	if err != nil {
		return err
	}
	return txDB.TransactWrite(ctx, fn)
}

func (r *BookmarkRepository) transactionalDB() (transactionalDB, error) {
	if txDB, ok := r.db.(transactionalDB); ok && txDB != nil {
		return txDB, nil
	}
	return nil, fmt.Errorf("database does not support transact write operations")
}

func (r *BookmarkRepository) batchGetBookmarks(ctx context.Context, keys []any) ([]*models.Bookmark, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	builder := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		BatchGetBuilder().
		Keys(keys).
		Parallel(4).
		WithRetry(&core.RetryPolicy{
			MaxRetries:    5,
			InitialDelay:  75 * time.Millisecond,
			MaxDelay:      2 * time.Second,
			BackoffFactor: 1.8,
			Jitter:        0.35,
		}).
		OnError(func(chunk []any, err error) error {
			r.logger.Warn("bookmark batch chunk failed",
				zap.Int("chunk_size", len(chunk)),
				zap.Error(err))
			return err
		})

	var raw []models.Bookmark
	if err := builder.Execute(&raw); err != nil {
		return nil, err
	}

	results := make([]*models.Bookmark, 0, len(raw))
	for i := range raw {
		results = append(results, &raw[i])
	}
	return results, nil
}

func (r *BookmarkRepository) logTransactionError(message string, err error, fields ...zap.Field) {
	var txErr *errors.TransactionError
	if stdErrors.As(err, &txErr) {
		fields = append(fields,
			zap.Int("transaction_op_index", txErr.OperationIndex),
			zap.String("transaction_operation", txErr.Operation),
			zap.String("transaction_reason", txErr.Reason),
		)
	}

	r.logger.Warn(message, append(fields, zap.Error(err))...)
}

func newBookmarkKey(pk, sk string) *models.Bookmark {
	return &models.Bookmark{
		PK: pk,
		SK: sk,
	}
}

func (r *BookmarkRepository) dynamoGetObjectBookmark(ctx context.Context, username, objectID string) (*models.Bookmark, error) {
	pk := buildBookmarkPK(username)
	var bookmark models.Bookmark
	err := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		ConsistentRead().
		Where("PK", "=", pk).
		Where("SK", "=", buildObjectSK(objectID)).
		First(&bookmark)
	if err != nil {
		return nil, err
	}
	return &bookmark, nil
}

func (r *BookmarkRepository) dynamoFindTimeBookmarkByObject(ctx context.Context, username, objectID string) (*models.Bookmark, error) {
	pk := buildBookmarkPK(username)
	var bookmarks []models.Bookmark
	err := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", models.BookmarkSortKeyPrefixTime).
		Filter("ObjectID", "=", objectID).
		Limit(1).
		All(&bookmarks)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	if len(bookmarks) == 0 {
		return nil, nil
	}
	return &bookmarks[0], nil
}
