package repositories

import (
	"context"
	"encoding/base64"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	errors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
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

	if legacy, legacyErr := r.findTimeBookmarkFn(ctx, username, objectID); legacyErr == nil && legacy != nil {
		timeRecord = legacy
		if timeRecord.CreatedAt.IsZero() {
			timeRecord.CreatedAt = bookmarkCreatedAt(*timeRecord)
		}
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

	unlockExistingTimeRecord := !createTimeRecord && timeRecord.Locked
	if createTimeRecord && timeRecord.Locked {
		// New records can be written in the unlocked state because the object record
		// is created in the same transaction — no need for a follow-up update.
		timeRecord.Locked = false
	}

	writeErr := r.transactWriteFn(ctx, func(tx core.TransactionBuilder) error {
		if createTimeRecord {
			tx.Create(timeRecord, tabletheory.IfNotExists())
		}
		tx.Create(objectRecord, tabletheory.IfNotExists())
		if unlockExistingTimeRecord {
			tx.UpdateWithBuilder(newBookmarkKey(timeRecord.PK, timeRecord.SK), func(ub core.UpdateBuilder) error {
				ub.Set("Locked", false)
				return nil
			}, tabletheory.Condition("Locked", "=", true))
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

	var bookmarks []models.Bookmark
	err := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		Where("PK", "=", pk).
		All(&bookmarks)
	if err != nil {
		r.logger.Error("failed to count user bookmarks",
			zap.String("username", username),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityBookmark, "count")
	}

	var count int64
	for _, bookmark := range bookmarks {
		if isReadableTimeBookmark(bookmark) {
			count++
		}
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
			keys = append(keys, tabletheory.NewKeyPair(pk, buildObjectSK(statusID)))
		}

		items, err := r.batchGetFn(ctx, keys)
		if err != nil {
			return nil, ErrorHandler.HandleQueryError(err, EntityBookmark, "batch bookmark lookup")
		}

		for _, bookmark := range items {
			if bookmark == nil {
				continue
			}
			if objectID := bookmarkObjectID(*bookmark); objectID != "" {
				result[objectID] = true
			}
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
	r.logger.Info("starting cascade delete for object bookmarks",
		zap.String("object_id", objectID))

	gsi8PK := fmt.Sprintf("BOOKMARK_OBJECT#%s", objectID)
	cursor := ""
	deletedCount := 0

	for {
		page, err := r.QueryGSIPaginated(ctx, "gsi8", gsi8PK, BasePaginationOptions{
			Limit:  250,
			Cursor: cursor,
			Order:  SortOrderAsc,
		})
		if err != nil {
			r.logger.Error("failed to query bookmark object index for cascade delete",
				zap.String("object_id", objectID),
				zap.Error(err))
			return ErrorHandler.HandleQueryError(err, EntityBookmark, "cascade query")
		}

		if len(page.Items) == 0 {
			break
		}

		keys := make([]struct{ PK, SK string }, 0, len(page.Items)*2)
		for _, bookmark := range page.Items {
			if bookmark == nil {
				continue
			}
			if bookmark.RecordType != models.BookmarkRecordTypeObject {
				r.logger.Warn("skipping unexpected bookmark record in object index",
					zap.String("pk", bookmark.PK),
					zap.String("sk", bookmark.SK),
					zap.String("object_id", objectID),
					zap.String("record_type", bookmark.RecordType))
				continue
			}

			keys = append(keys, struct{ PK, SK string }{PK: bookmark.PK, SK: bookmark.SK})
			if bookmark.TimeRecordSK != "" {
				keys = append(keys, struct{ PK, SK string }{PK: bookmark.PK, SK: bookmark.TimeRecordSK})
			}
		}

		if err := r.BatchDelete(ctx, keys); err != nil {
			return ErrorHandler.HandleDeleteError(err, EntityBookmark, "cascade delete")
		}
		deletedCount += len(keys)

		if !page.HasMore || page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
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

func isReadableTimeBookmark(bookmark models.Bookmark) bool {
	if bookmark.Locked {
		return false
	}
	if bookmark.RecordType == models.BookmarkRecordTypeObject || strings.HasPrefix(bookmark.SK, models.BookmarkSortKeyPrefixObject+"#") {
		return false
	}
	if bookmark.RecordType == models.BookmarkRecordTypeTime || strings.HasPrefix(bookmark.SK, models.BookmarkSortKeyPrefixTime+"#") {
		return true
	}
	return isLegacyBookmarkTimestampSK(bookmark.SK)
}

func isLegacyBookmarkTimestampSK(sk string) bool {
	trimmed := strings.TrimSpace(sk)
	timestamp, objectID, ok := legacyBookmarkSKParts(trimmed)
	if !ok || timestamp == "" || strings.TrimSpace(objectID) == "" {
		return false
	}

	return isBookmarkTimestampSegment(timestamp)
}

func legacyBookmarkSKParts(sk string) (string, string, bool) {
	trimmed := strings.TrimSpace(sk)
	if trimmed == "" {
		return "", "", false
	}

	timestamp, objectID, ok := strings.Cut(trimmed, "#")
	if !ok {
		return "", "", false
	}
	if timestamp == models.BookmarkSortKeyPrefixTime || timestamp == models.BookmarkSortKeyPrefixObject {
		return "", "", false
	}
	return timestamp, objectID, true
}

func isBookmarkTimestampSegment(segment string) bool {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, segment); err == nil {
		return true
	}
	if _, err := time.Parse(time.RFC3339, segment); err == nil {
		return true
	}
	return false
}

func bookmarkCreatedAt(bookmark models.Bookmark) time.Time {
	if !bookmark.CreatedAt.IsZero() {
		return bookmark.CreatedAt
	}

	sk := strings.TrimSpace(bookmark.SK)
	if strings.HasPrefix(sk, models.BookmarkSortKeyPrefixTime+"#") {
		remainder := strings.TrimPrefix(sk, models.BookmarkSortKeyPrefixTime+"#")
		if idx := strings.Index(remainder, "#"); idx >= 0 {
			sk = remainder[:idx]
		} else {
			sk = remainder
		}
	} else if timestamp, _, ok := legacyBookmarkSKParts(sk); ok {
		sk = timestamp
	}

	if parsed, err := time.Parse(time.RFC3339Nano, sk); err == nil {
		return parsed
	}
	if parsed, err := time.Parse(time.RFC3339, sk); err == nil {
		return parsed
	}
	return time.Time{}
}

func bookmarkObjectID(bookmark models.Bookmark) string {
	if strings.TrimSpace(bookmark.ObjectID) != "" {
		return bookmark.ObjectID
	}

	sk := strings.TrimSpace(bookmark.SK)
	if strings.HasPrefix(sk, models.BookmarkSortKeyPrefixObject+"#") {
		return strings.TrimPrefix(sk, models.BookmarkSortKeyPrefixObject+"#")
	}
	if strings.HasPrefix(sk, models.BookmarkSortKeyPrefixTime+"#") {
		remainder := strings.TrimPrefix(sk, models.BookmarkSortKeyPrefixTime+"#")
		if _, objectID, ok := strings.Cut(remainder, "#"); ok {
			return objectID
		}
		return ""
	}
	if _, objectID, ok := legacyBookmarkSKParts(sk); ok {
		return objectID
	}
	return ""
}

func filterBookmarkObjectID(query core.Query, objectID string) core.Query {
	return query.FilterGroup(func(group core.Query) {
		group.Filter("ObjectID", "=", objectID)
		group.OrFilter("object_id", "=", objectID)
	})
}

const (
	bookmarkPageCursorPrefix = "bm2:"
	bookmarkTimeUpperBound   = models.BookmarkSortKeyPrefixTime + "$"
	bookmarkLegacyUpperBound = "A"
)

type bookmarkPageCursor struct {
	Version  int    `json:"v"`
	TimeSK   string `json:"t,omitempty"`
	LegacySK string `json:"l,omitempty"`
}

type bookmarkStreamKind int

const (
	bookmarkStreamTime bookmarkStreamKind = iota
	bookmarkStreamLegacy
)

func (r *BookmarkRepository) queryUnlockedTimeBookmarks(ctx context.Context, username string, limit int, cursor string) ([]models.Bookmark, string, error) {
	pk := buildBookmarkPK(username)
	limit = sanitizeLimit(limit, 20, 100)

	pageCursor, err := parseBookmarkPageCursor(cursor)
	if err != nil {
		return nil, "", err
	}

	target := limit + 1
	timeBookmarks, err := r.queryReadableBookmarkStream(ctx, pk, bookmarkStreamTime, pageCursor.TimeSK, target)
	if err != nil {
		r.logger.Error("failed to get user time bookmarks",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, EntityBookmark, "user bookmarks")
	}

	legacyBookmarks, err := r.queryReadableBookmarkStream(ctx, pk, bookmarkStreamLegacy, pageCursor.LegacySK, target)
	if err != nil {
		r.logger.Error("failed to get user legacy bookmarks",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, EntityBookmark, "user bookmarks")
	}

	filtered := make([]models.Bookmark, 0, len(timeBookmarks)+len(legacyBookmarks))
	filtered = append(filtered, timeBookmarks...)
	filtered = append(filtered, legacyBookmarks...)

	sort.SliceStable(filtered, func(i, j int) bool {
		left := bookmarkCreatedAt(filtered[i])
		right := bookmarkCreatedAt(filtered[j])
		if left.Equal(right) {
			return filtered[i].SK > filtered[j].SK
		}
		return left.After(right)
	})

	hasMore := len(filtered) > limit
	if hasMore {
		filtered = filtered[:limit]
	}

	nextCursor := ""
	if hasMore && len(filtered) > 0 {
		nextPageCursor := pageCursor
		for _, bookmark := range filtered {
			if isTimeBookmarkSK(bookmark.SK) || bookmark.RecordType == models.BookmarkRecordTypeTime {
				nextPageCursor.TimeSK = bookmark.SK
				continue
			}
			if isLegacyBookmarkTimestampSK(bookmark.SK) {
				nextPageCursor.LegacySK = bookmark.SK
			}
		}
		nextCursor = encodeBookmarkPageCursor(nextPageCursor)
	}

	return filtered, nextCursor, nil
}

func (r *BookmarkRepository) queryReadableBookmarkStream(
	ctx context.Context,
	pk string,
	stream bookmarkStreamKind,
	cursor string,
	target int,
) ([]models.Bookmark, error) {
	if target <= 0 {
		return nil, nil
	}

	upperBound := bookmarkStreamInitialUpperBound(stream)
	if strings.TrimSpace(cursor) != "" {
		upperBound = cursor
	}

	batchLimit := target * 4
	if batchLimit < 100 {
		batchLimit = 100
	}
	if batchLimit > 500 {
		batchLimit = 500
	}

	result := make([]models.Bookmark, 0, target)
	for len(result) < target {
		page, err := r.queryBookmarkStreamPage(ctx, pk, upperBound, batchLimit)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}

		var stop bool
		result, stop = appendReadableBookmarkStreamPage(result, page, stream, target)
		if stop {
			break
		}

		nextUpperBound, ok := nextBookmarkStreamUpperBound(page, upperBound, batchLimit)
		if !ok {
			break
		}
		upperBound = nextUpperBound
	}

	return result, nil
}

func (r *BookmarkRepository) queryBookmarkStreamPage(ctx context.Context, pk, upperBound string, limit int) ([]models.Bookmark, error) {
	var page []models.Bookmark
	err := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		Where("PK", "=", pk).
		Where("SK", "<", upperBound).
		OrderBy("SK", SortOrderDesc).
		Limit(limit).
		All(&page)
	return page, err
}

func appendReadableBookmarkStreamPage(
	result []models.Bookmark,
	page []models.Bookmark,
	stream bookmarkStreamKind,
	target int,
) ([]models.Bookmark, bool) {
	for _, bookmark := range page {
		readable, stop := readableBookmarkForStream(bookmark, stream)
		if stop {
			return result, true
		}
		if !readable {
			continue
		}
		if bookmark.CreatedAt.IsZero() {
			bookmark.CreatedAt = bookmarkCreatedAt(bookmark)
		}
		if bookmark.ObjectID == "" {
			bookmark.ObjectID = bookmarkObjectID(bookmark)
		}
		result = append(result, bookmark)
		if len(result) >= target {
			return result, true
		}
	}
	return result, false
}

func readableBookmarkForStream(bookmark models.Bookmark, stream bookmarkStreamKind) (bool, bool) {
	switch stream {
	case bookmarkStreamTime:
		if !isTimeBookmarkSK(bookmark.SK) && bookmark.RecordType != models.BookmarkRecordTypeTime {
			return false, true
		}
	case bookmarkStreamLegacy:
		if !isLegacyBookmarkTimestampSK(bookmark.SK) {
			return false, false
		}
	}
	return isReadableTimeBookmark(bookmark), false
}

func nextBookmarkStreamUpperBound(page []models.Bookmark, upperBound string, batchLimit int) (string, bool) {
	lastSK := page[len(page)-1].SK
	if len(page) < batchLimit || lastSK == upperBound {
		return "", false
	}
	return lastSK, true
}

func bookmarkStreamInitialUpperBound(stream bookmarkStreamKind) string {
	if stream == bookmarkStreamTime {
		return bookmarkTimeUpperBound
	}
	return bookmarkLegacyUpperBound
}

func isTimeBookmarkSK(sk string) bool {
	return strings.HasPrefix(sk, models.BookmarkSortKeyPrefixTime+"#")
}

func parseBookmarkPageCursor(cursor string) (bookmarkPageCursor, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return bookmarkPageCursor{Version: 2}, nil
	}
	if !strings.HasPrefix(cursor, bookmarkPageCursorPrefix) {
		return legacyBookmarkPageCursor(cursor), nil
	}

	encoded := strings.TrimPrefix(cursor, bookmarkPageCursorPrefix)
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return bookmarkPageCursor{}, fmt.Errorf("invalid bookmark cursor: %w", err)
	}

	var pageCursor bookmarkPageCursor
	if err := json.Unmarshal(payload, &pageCursor); err != nil {
		return bookmarkPageCursor{}, fmt.Errorf("invalid bookmark cursor: %w", err)
	}
	if pageCursor.Version == 0 {
		pageCursor.Version = 2
	}
	if err := validateBookmarkPageCursor(pageCursor); err != nil {
		return bookmarkPageCursor{}, err
	}
	return pageCursor, nil
}

func legacyBookmarkPageCursor(cursor string) bookmarkPageCursor {
	pageCursor := bookmarkPageCursor{Version: 2}
	if isTimeBookmarkSK(cursor) {
		pageCursor.TimeSK = cursor
		if createdAt := bookmarkTimestampFromSK(cursor); !createdAt.IsZero() {
			pageCursor.LegacySK = createdAt.Format(time.RFC3339Nano) + "\xff"
		}
		return pageCursor
	}
	if isLegacyBookmarkTimestampSK(cursor) {
		pageCursor.LegacySK = cursor
		if createdAt := bookmarkTimestampFromSK(cursor); !createdAt.IsZero() {
			pageCursor.TimeSK = models.BookmarkSortKeyPrefixTime + "#" + createdAt.Format(time.RFC3339Nano)
		}
		return pageCursor
	}
	return pageCursor
}

func validateBookmarkPageCursor(cursor bookmarkPageCursor) error {
	if cursor.TimeSK != "" && !isTimeBookmarkSK(cursor.TimeSK) {
		return fmt.Errorf("invalid bookmark cursor: time key out of range")
	}
	if cursor.LegacySK != "" && !isLegacyBookmarkCursorSK(cursor.LegacySK) {
		return fmt.Errorf("invalid bookmark cursor: legacy key out of range")
	}
	return nil
}

func isLegacyBookmarkCursorSK(sk string) bool {
	trimmed := strings.TrimSuffix(sk, "\xff")
	return isLegacyBookmarkTimestampSK(trimmed) || isBookmarkTimestampSegment(trimmed)
}

func encodeBookmarkPageCursor(cursor bookmarkPageCursor) string {
	cursor.Version = 2
	payload, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return bookmarkPageCursorPrefix + base64.RawURLEncoding.EncodeToString(payload)
}

func bookmarkTimestampFromSK(sk string) time.Time {
	return bookmarkCreatedAt(models.Bookmark{SK: sk})
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
		tx.Create(objectRecord, tabletheory.IfNotExists())
		if legacy.Locked {
			tx.UpdateWithBuilder(newBookmarkKey(legacy.PK, legacy.SK), func(ub core.UpdateBuilder) error {
				ub.Set("Locked", false)
				return nil
			}, tabletheory.Condition("Locked", "=", true))
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
	timeQuery := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", models.BookmarkSortKeyPrefixTime)
	err := filterBookmarkObjectID(timeQuery, objectID).
		Limit(1).
		All(&bookmarks)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	for _, bookmark := range bookmarks {
		if bookmarkObjectID(bookmark) == objectID && (strings.HasPrefix(bookmark.SK, models.BookmarkSortKeyPrefixTime+"#") || bookmark.RecordType == models.BookmarkRecordTypeTime) {
			if bookmark.CreatedAt.IsZero() {
				bookmark.CreatedAt = bookmarkCreatedAt(bookmark)
			}
			if bookmark.ObjectID == "" {
				bookmark.ObjectID = bookmarkObjectID(bookmark)
			}
			return &bookmark, nil
		}
	}

	bookmarks = nil
	legacyQuery := r.db.WithContext(ctx).Model(&models.Bookmark{}).
		Where("PK", "=", pk)
	err = filterBookmarkObjectID(legacyQuery, objectID).
		Limit(25).
		All(&bookmarks)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	for _, bookmark := range bookmarks {
		if bookmarkObjectID(bookmark) == objectID && isReadableTimeBookmark(bookmark) {
			if bookmark.CreatedAt.IsZero() {
				bookmark.CreatedAt = bookmarkCreatedAt(bookmark)
			}
			if bookmark.ObjectID == "" {
				bookmark.ObjectID = bookmarkObjectID(bookmark)
			}
			return &bookmark, nil
		}
	}
	return nil, nil
}
