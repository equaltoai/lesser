// Package repositories provides batch operation repositories with cost tracking for DynamORM operations.
package repositories

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/batch"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// costTrackerAdapter adapts cost.Tracker to batch.CostTracker interface
type costTrackerAdapter struct {
	tracker *cost.Tracker
}

func (a *costTrackerAdapter) CalculateCost() batch.CostMetrics {
	if a.tracker == nil {
		return batch.CostMetrics{}
	}
	opCost := a.tracker.CalculateCost()
	if opCost == nil {
		return batch.CostMetrics{}
	}
	return batch.CostMetrics{
		DynamoDBReads:  opCost.DynamoDBReads,
		DynamoDBWrites: opCost.DynamoDBWrites,
	}
}

func (a *costTrackerAdapter) TrackDynamoWrite(items int) {
	if a.tracker != nil {
		if err := a.tracker.TrackDynamoWrite(items); err != nil {
			zap.L().Warn("failed to track DynamoDB write cost", zap.Error(err))
		}
	}
}

func (a *costTrackerAdapter) TrackDynamoRead(items int) {
	if a.tracker != nil {
		if err := a.tracker.TrackDynamoRead(items); err != nil {
			zap.L().Warn("failed to track DynamoDB read cost", zap.Error(err))
		}
	}
}

// BatchRepository extends BaseRepository with advanced batch operations
type BatchRepository struct {
	*dynamorm.BaseRepository
	batchWriter  *batch.BatchWriter
	batchReader  *batch.BatchReader
	batchDeleter *batch.BatchDeleter
	logger       *zap.Logger
	tracker      *cost.Tracker
}

// NewBatchRepository creates a new repository with batch capabilities
func NewBatchRepository(db core.DB, tableName string, logger *zap.Logger, tracker *cost.Tracker) *BatchRepository {
	// Create adapter for cost tracker
	var costTrackerInterface batch.CostTracker
	if tracker != nil {
		costTrackerInterface = &costTrackerAdapter{tracker: tracker}
	}

	batchWriter := batch.NewBatchWriter(db, batch.BatchWriterConfig{
		BatchSize: batch.DefaultBatchSize,
		Logger:    logger,
		Tracker:   costTrackerInterface,
	})

	batchReader := batch.NewBatchReader(db, batch.BatchReaderConfig{
		BatchSize: batch.MaxBatchReadSize,
		Logger:    logger,
		Tracker:   costTrackerInterface,
	})

	batchDeleter := batch.NewBatchDeleter(db, batch.BatchDeleterConfig{
		BatchSize: batch.DefaultBatchSize,
		Logger:    logger,
		Tracker:   costTrackerInterface,
	})

	return &BatchRepository{
		BaseRepository: dynamorm.NewBaseRepository(db, tableName),
		batchWriter:    batchWriter,
		batchReader:    batchReader,
		batchDeleter:   batchDeleter,
		logger:         logger,
		tracker:        tracker,
	}
}

// BatchDelete provides direct access to batch delete functionality
func (br *BatchRepository) BatchDelete(ctx context.Context, keys []any) (*batch.BatchDeleteResult, error) {
	return br.batchDeleter.DeleteItems(ctx, keys)
}

// BatchDeleteParallel provides direct access to parallel batch delete functionality
func (br *BatchRepository) BatchDeleteParallel(ctx context.Context, keys []any, workers int) (*batch.BatchDeleteResult, error) {
	return br.batchDeleter.DeleteItemsParallel(ctx, keys, workers)
}

// BatchDeleteWithRetry provides direct access to batch delete with retry functionality
func (br *BatchRepository) BatchDeleteWithRetry(ctx context.Context, keys []any, maxRetries int) (*batch.BatchDeleteResult, error) {
	return br.batchDeleter.DeleteItemsWithRetry(ctx, keys, maxRetries)
}

// TimelineBatchOperations provides specialized operations for timeline management
type TimelineBatchOperations struct {
	*BatchRepository
}

// NewTimelineBatchOperations creates batch operations optimized for timeline operations
func NewTimelineBatchOperations(db core.DB, logger *zap.Logger, tracker *cost.Tracker) *TimelineBatchOperations {
	return &TimelineBatchOperations{
		BatchRepository: NewBatchRepository(db, "timeline", logger, tracker),
	}
}

// BatchInsertTimelineEntries efficiently inserts timeline entries for multiple followers
func (tbo *TimelineBatchOperations) BatchInsertTimelineEntries(ctx context.Context, followerIDs []string, statusID, authorID string, createdAt time.Time) error {
	if err := common.ValidateSliceNotEmpty("follower_ids", followerIDs); err != nil {
		return nil
	}

	// Create timeline entries
	entries := make([]any, 0, len(followerIDs))
	for _, followerID := range followerIDs {
		entry := map[string]any{
			"PK":        fmt.Sprintf("USER#%s", followerID),
			"SK":        fmt.Sprintf("TIMELINE#%s#%s", createdAt.Format("20060102150405"), statusID),
			"StatusID":  statusID,
			"AuthorID":  authorID,
			"CreatedAt": createdAt,
			"Type":      "home",
		}
		entries = append(entries, entry)
	}

	// Use parallel batch operations for large follower lists
	if err := common.ValidateSliceLength("entries", entries, 100); err != nil {
		_, err := tbo.batchWriter.WriteItemsParallel(ctx, entries, 4)
		return err
	}

	// Sequential batch for smaller lists
	result, err := tbo.batchWriter.WriteItems(ctx, entries)
	if err != nil {
		return fmt.Errorf("failed to batch insert timeline entries: %w", err)
	}

	if tbo.logger != nil {
		tbo.logger.Info("timeline_entries_inserted",
			zap.Int("total_entries", len(entries)),
			zap.Int("processed", result.ProcessedItems),
			zap.Int("failed", result.FailedItems),
			zap.Duration("duration", result.Duration),
		)
	}

	return nil
}

// BatchRemoveTimelineEntries removes timeline entries for unfollowed users
func (tbo *TimelineBatchOperations) BatchRemoveTimelineEntries(ctx context.Context, followerIDs []string, authorID string) error {
	if err := common.ValidateSliceNotEmpty("follower_ids", followerIDs); err != nil {
		return nil
	}

	// Create delete keys for timeline entries by author
	deleteKeys := make([]any, 0, len(followerIDs))
	for _, followerID := range followerIDs {
		// Create key for timeline entries to delete
		key := map[string]any{
			"PK": fmt.Sprintf("USER#%s", followerID),
			"SK": fmt.Sprintf("TIMELINE_AUTHOR#%s", authorID),
		}
		deleteKeys = append(deleteKeys, key)
	}

	result, err := tbo.batchDeleter.DeleteItems(ctx, deleteKeys)
	if err != nil {
		return fmt.Errorf("failed to batch remove timeline entries: %w", err)
	}

	if tbo.logger != nil {
		tbo.logger.Info("timeline_entries_removed",
			zap.Int("total_entries", len(deleteKeys)),
			zap.Int("processed", result.ProcessedItems),
			zap.Int("failed", result.FailedItems),
			zap.Duration("duration", result.Duration),
		)
	}

	return nil
}

// NotificationBatchOperations provides specialized operations for notifications
type NotificationBatchOperations struct {
	*BatchRepository
}

// NewNotificationBatchOperations creates batch operations for notifications
func NewNotificationBatchOperations(db core.DB, logger *zap.Logger, tracker *cost.Tracker) *NotificationBatchOperations {
	return &NotificationBatchOperations{
		BatchRepository: NewBatchRepository(db, "notifications", logger, tracker),
	}
}

// BatchCreateMentionNotifications creates mention notifications for multiple users
func (nbo *NotificationBatchOperations) BatchCreateMentionNotifications(ctx context.Context, mentionedUserIDs []string, statusID, authorID string) error {
	if err := common.ValidateSliceNotEmpty("mentioned_user_ids", mentionedUserIDs); err != nil {
		return nil
	}

	now := time.Now()
	notifications := make([]any, 0, len(mentionedUserIDs))

	for _, userID := range mentionedUserIDs {
		notification := map[string]any{
			"PK":         fmt.Sprintf("USER#%s", userID),
			"SK":         fmt.Sprintf("NOTIF#%s#%s", now.Format("20060102150405"), statusID),
			"ID":         fmt.Sprintf("%s_%s", statusID, userID),
			"Type":       "mention",
			"ActorID":    authorID,
			"TargetID":   statusID,
			"TargetType": "status",
			"CreatedAt":  now,
			"IsRead":     false,
			"ExpiresAt":  now.Add(30 * 24 * time.Hour), // 30 days TTL
		}
		notifications = append(notifications, notification)
	}

	result, err := nbo.batchWriter.WriteItems(ctx, notifications)
	if err != nil {
		return fmt.Errorf("failed to batch create mention notifications: %w", err)
	}

	if nbo.logger != nil {
		nbo.logger.Info("mention_notifications_created",
			zap.Int("total_notifications", len(notifications)),
			zap.Int("processed", result.ProcessedItems),
			zap.Duration("duration", result.Duration),
		)
	}

	return nil
}

// BatchMarkNotificationsRead marks multiple notifications as read
func (nbo *NotificationBatchOperations) BatchMarkNotificationsRead(ctx context.Context, userID string, notificationIDs []string) error {
	if err := common.ValidateSliceNotEmpty("notification_ids", notificationIDs); err != nil {
		return nil
	}

	now := time.Now()
	updates := make([]any, 0, len(notificationIDs))

	for _, notifID := range notificationIDs {
		update := map[string]any{
			"PK":     fmt.Sprintf("USER#%s", userID),
			"SK":     fmt.Sprintf("NOTIF#%s", notifID),
			"IsRead": true,
			"ReadAt": now,
		}
		updates = append(updates, update)
	}

	result, err := nbo.batchWriter.WriteItems(ctx, updates) // This would use batch update in real implementation
	if err != nil {
		return fmt.Errorf("failed to batch mark notifications read: %w", err)
	}

	if nbo.logger != nil {
		nbo.logger.Info("notifications_marked_read",
			zap.Int("total_notifications", len(updates)),
			zap.Int("processed", result.ProcessedItems),
			zap.Duration("duration", result.Duration),
		)
	}

	return nil
}

// MediaBatchOperations provides specialized operations for media management
type MediaBatchOperations struct {
	*BatchRepository
}

// NewMediaBatchOperations creates batch operations for media
func NewMediaBatchOperations(db core.DB, logger *zap.Logger, tracker *cost.Tracker) *MediaBatchOperations {
	return &MediaBatchOperations{
		BatchRepository: NewBatchRepository(db, "media", logger, tracker),
	}
}

// BatchUpdateMediaStatus updates the processing status of multiple media items
func (mbo *MediaBatchOperations) BatchUpdateMediaStatus(ctx context.Context, mediaIDs []string, status string, processedAt *time.Time) error {
	if err := common.ValidateSliceNotEmpty("media_ids", mediaIDs); err != nil {
		return nil
	}

	updates := make([]any, 0, len(mediaIDs))
	for _, mediaID := range mediaIDs {
		update := map[string]any{
			"PK":          fmt.Sprintf("MEDIA#%s", mediaID),
			"SK":          "VERSION#original",
			"Status":      status,
			"UpdatedAt":   time.Now(),
			"ProcessedAt": processedAt,
		}
		updates = append(updates, update)
	}

	result, err := mbo.batchWriter.WriteItems(ctx, updates)
	if err != nil {
		return fmt.Errorf("failed to batch update media status: %w", err)
	}

	if mbo.logger != nil {
		mbo.logger.Info("media_status_updated",
			zap.Int("total_media", len(updates)),
			zap.Int("processed", result.ProcessedItems),
			zap.String("status", status),
			zap.Duration("duration", result.Duration),
		)
	}

	return nil
}

// BatchCleanupExpiredMedia removes expired unused media items
func (mbo *MediaBatchOperations) BatchCleanupExpiredMedia(ctx context.Context, expiredMediaKeys []map[string]any) error {
	if err := common.ValidateSliceNotEmpty("expired_media_keys", expiredMediaKeys); err != nil {
		return nil
	}

	// Convert to delete operations
	deleteKeys := make([]any, 0, len(expiredMediaKeys))
	for _, key := range expiredMediaKeys {
		deleteKeys = append(deleteKeys, key)
	}

	result, err := mbo.batchDeleter.DeleteItemsWithRetry(ctx, deleteKeys, 3) // Use retry logic for cleanup
	if err != nil {
		return fmt.Errorf("failed to batch cleanup expired media: %w", err)
	}

	if mbo.logger != nil {
		mbo.logger.Info("expired_media_cleaned",
			zap.Int("total_media", len(deleteKeys)),
			zap.Int("processed", result.ProcessedItems),
			zap.Int("failed", result.FailedItems),
			zap.Duration("duration", result.Duration),
		)
	}

	return nil
}

// AdvancedBatchOperations provides advanced batch patterns
type AdvancedBatchOperations struct {
	*BatchRepository
	transactionMgr *TransactionManager
}

// NewAdvancedBatchOperations creates advanced batch operations with transaction support
func NewAdvancedBatchOperations(db core.DB, tableName string, logger *zap.Logger, tracker *cost.Tracker) *AdvancedBatchOperations {
	return &AdvancedBatchOperations{
		BatchRepository: NewBatchRepository(db, tableName, logger, tracker),
		transactionMgr:  NewTransactionManager(db, logger, tracker),
	}
}

// BatchUpsertWithConflictResolution performs batch upsert with conflict resolution
func (abo *AdvancedBatchOperations) BatchUpsertWithConflictResolution(ctx context.Context, items []any, conflictResolver func(existing, newItem any) any) error {
	if err := common.ValidateSliceNotEmpty("items", items); err != nil {
		return nil
	}

	// Process in smaller batches to handle conflicts
	batchSize := 10 // Smaller batches for better conflict handling
	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}

		batch := items[i:end]
		if err := abo.processBatchWithConflictResolution(ctx, batch, conflictResolver); err != nil {
			return fmt.Errorf("failed to process batch at index %d: %w", i, err)
		}
	}

	return nil
}

// processBatchWithConflictResolution handles conflicts for a single batch
func (abo *AdvancedBatchOperations) processBatchWithConflictResolution(ctx context.Context, batch []any, conflictResolver func(existing, newItem any) any) error {
	maxRetries := 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, err := abo.batchWriter.WriteItems(ctx, batch)

		if err == nil {
			// Success - no conflicts
			return nil
		}

		// Check if error indicates conflicts or retryable issues
		if !isConflictError(err) && !isRetryableError(err) {
			// Non-retryable error
			return fmt.Errorf("non-retryable batch error: %w", err)
		}

		if abo.logger != nil {
			abo.logger.Warn("batch_conflicts_detected",
				zap.Int("failed_items", result.FailedItems),
				zap.Int("attempt", attempt+1),
				zap.Error(err),
			)
		}

		// If this is the last attempt, return the error
		if attempt == maxRetries {
			return fmt.Errorf("batch failed after %d attempts: %w", maxRetries+1, err)
		}

		// Try to resolve conflicts by reading existing items and applying resolution
		if conflictResolver != nil && result.FailedItems > 0 {
			resolvedBatch, resolveErr := abo.resolveConflicts(ctx, batch, conflictResolver)
			if resolveErr != nil {
				abo.logger.Warn("conflict resolution failed, retrying original batch",
					zap.Error(resolveErr))
			} else {
				batch = resolvedBatch
			}
		}

		// Exponential backoff before retry
		backoffDuration := time.Duration(attempt+1) * 100 * time.Millisecond
		select {
		case <-time.After(backoffDuration):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// isConflictError checks if an error indicates a conflict (conditional check failure, etc.)
func isConflictError(err error) bool {
	if err == nil {
		return false
	}

	errorStr := strings.ToLower(err.Error())
	conflictPatterns := []string{
		"conditionalcheckfailedexception",
		"conditional check failed",
		"transactionconflictexception",
		"conflict",
		"optimistic locking",
		"version mismatch",
		"concurrent modification",
	}

	for _, pattern := range conflictPatterns {
		if strings.Contains(errorStr, pattern) {
			return true
		}
	}

	return false
}

// resolveConflicts attempts to resolve conflicts by reading existing items and applying conflict resolution
func (abo *AdvancedBatchOperations) resolveConflicts(ctx context.Context, originalBatch []any, conflictResolver func(existing, newItem any) any) ([]any, error) {
	if err := common.ValidateSliceNotEmpty("original_batch", originalBatch); err != nil {
		return originalBatch, nil
	}

	resolvedBatch := make([]any, 0, len(originalBatch))

	for _, item := range originalBatch {
		// Extract key information from item to read existing version
		// This is a simplified approach - in practice you'd need to extract
		// the primary key based on the item type/structure

		// Try to read the existing item
		existingItem, err := abo.readExistingItem(ctx, item)
		if err != nil {
			// If item doesn't exist, use original item
			if isNotFoundError(err) {
				resolvedBatch = append(resolvedBatch, item)
				continue
			}
			// For other errors, log and use original item
			if abo.logger != nil {
				abo.logger.Warn("failed to read existing item for conflict resolution",
					zap.Error(err))
			}
			resolvedBatch = append(resolvedBatch, item)
			continue
		}

		// Apply conflict resolution
		resolvedItem := conflictResolver(existingItem, item)
		resolvedBatch = append(resolvedBatch, resolvedItem)
	}

	if abo.logger != nil {
		abo.logger.Debug("resolved batch conflicts",
			zap.Int("original_count", len(originalBatch)),
			zap.Int("resolved_count", len(resolvedBatch)))
	}

	return resolvedBatch, nil
}

// readExistingItem attempts to read an existing item from the database
func (abo *AdvancedBatchOperations) readExistingItem(ctx context.Context, item any) (any, error) {
	// Extract the primary key and sort key from the item using reflection and interface checking
	var pk, sk string

	// Check if item implements a KeyProvider interface
	if keyProvider, ok := item.(interface {
		GetPK() string
		GetSK() string
	}); ok {
		pk = keyProvider.GetPK()
		sk = keyProvider.GetSK()
	} else if itemMap, ok := item.(map[string]any); ok {
		// Handle map-based items
		if pkVal, exists := itemMap["PK"]; exists {
			pk = fmt.Sprintf("%v", pkVal)
		}
		if skVal, exists := itemMap["SK"]; exists {
			sk = fmt.Sprintf("%v", skVal)
		}
	} else {
		// Use reflection to find PK and SK fields
		pkField, skField := extractKeysFromStruct(item)
		pk = pkField
		sk = skField
	}

	if err := common.ValidateRequiredParam("pk", pk); err != nil {
		return nil, fmt.Errorf("could not extract primary key from item")
	}

	// Note: tableName not needed for current implementation

	// Create a new instance of the same type for reading
	existingItem := createSameTypeInstance(item)
	if existingItem == nil {
		return nil, fmt.Errorf("could not create instance for reading existing item")
	}

	// We need to get the database connection from the parent repository
	// Since AdvancedBatchOperations doesn't have direct DB access, we need to use the batch reader
	if abo.batchReader == nil {
		return nil, fmt.Errorf("batch reader not available for conflict resolution")
	}

	// Use batch reader to read the single item
	keys := []any{
		map[string]any{
			"PK": pk,
			"SK": sk,
		},
	}

	// Create a slice to hold the results
	var resultItems []any
	_, err := abo.batchReader.ReadItems(ctx, keys, &resultItems)
	if err != nil {
		if isDynamoDBNotFoundError(err) {
			return nil, fmt.Errorf("item not found")
		}
		return nil, fmt.Errorf("failed to read existing item: %w", err)
	}

	if err := common.ValidateSliceNotEmpty("result_items", resultItems); err != nil {
		return nil, fmt.Errorf("item not found")
	}

	return resultItems[0], nil
}

// extractKeysFromStruct uses reflection to extract PK and SK fields from a struct
func extractKeysFromStruct(item any) (pk, sk string) {
	if item == nil {
		return "", ""
	}

	val := reflect.ValueOf(item)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return "", ""
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Check field name or DynamoDB tag
		fieldName := fieldType.Name
		tag := fieldType.Tag.Get("dynamodbav")

		if fieldName == "PK" || tag == "PK" || strings.Contains(tag, "PK") {
			if field.CanInterface() && field.Kind() == reflect.String {
				pk = field.String()
			}
		}

		if fieldName == "SK" || tag == "SK" || strings.Contains(tag, "SK") {
			if field.CanInterface() && field.Kind() == reflect.String {
				sk = field.String()
			}
		}
	}

	return pk, sk
}

// createSameTypeInstance creates a new instance of the same type as the given item
func createSameTypeInstance(item any) any {
	if item == nil {
		return nil
	}

	typ := reflect.TypeOf(item)

	// Handle pointer types
	if typ.Kind() == reflect.Ptr {
		// Create new instance of the element type, then get its address
		elemType := typ.Elem()
		newElem := reflect.New(elemType)
		return newElem.Interface()
	}

	// Handle value types
	if typ.Kind() == reflect.Struct {
		newVal := reflect.New(typ)
		return newVal.Interface()
	}

	// Handle map types
	if typ.Kind() == reflect.Map {
		newMap := reflect.MakeMap(typ)
		return newMap.Interface()
	}

	// For other types, try to create a zero value
	newVal := reflect.New(typ)
	return newVal.Interface()
}

// isDynamoDBNotFoundError checks if an error is a DynamoDB not found error
func isDynamoDBNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	errorStr := strings.ToLower(err.Error())
	notFoundPatterns := []string{
		"not found",
		"does not exist",
		"item not found",
		"record not found",
		"resourcenotfoundexception",
		"no records found",
		"no items found",
		"empty result",
	}

	for _, pattern := range notFoundPatterns {
		if strings.Contains(errorStr, pattern) {
			return true
		}
	}

	return false
}

// isNotFoundError checks if an error indicates an item was not found
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	errorStr := strings.ToLower(err.Error())
	notFoundPatterns := []string{
		"not found",
		"does not exist",
		"item not found",
		"record not found",
		"resourcenotfoundexception",
	}

	for _, pattern := range notFoundPatterns {
		if strings.Contains(errorStr, pattern) {
			return true
		}
	}

	return false
}

// BatchProcessWithRetry processes items with exponential backoff retry
func (abo *AdvancedBatchOperations) BatchProcessWithRetry(ctx context.Context, items []any, maxRetries int, processor func([]any) error) error {
	if err := common.ValidateSliceNotEmpty("items", items); err != nil {
		return nil
	}

	var lastErr error
	backoff := 100 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with safe uint conversion
			attemptShift := attempt - 1
			var shiftAmount uint
			if attemptShift < 0 {
				shiftAmount = 0
			} else if attemptShift > 63 { // Prevent overflow in bitshift
				shiftAmount = 63
			} else {
				shiftAmount = uint(attemptShift)
			}
			delay := backoff * time.Duration(1<<shiftAmount)
			if abo.logger != nil {
				abo.logger.Info("retrying_batch_operation",
					zap.Int("attempt", attempt),
					zap.Duration("delay", delay),
				)
			}

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		err := processor(items)
		if err == nil {
			if abo.logger != nil && attempt > 0 {
				abo.logger.Info("batch_operation_succeeded_after_retry",
					zap.Int("attempts", attempt+1),
				)
			}
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			break
		}
	}

	return fmt.Errorf("batch operation failed after %d attempts: %w", maxRetries+1, lastErr)
}

// ParallelBatchProcessor processes large datasets with parallel workers and progress tracking
type ParallelBatchProcessor struct {
	repository *BatchRepository
	workers    int
	batchSize  int
	logger     *zap.Logger
}

// NewParallelBatchProcessor creates a new parallel batch processor
func NewParallelBatchProcessor(repository *BatchRepository, workers, batchSize int, logger *zap.Logger) *ParallelBatchProcessor {
	if workers <= 0 {
		workers = 4
	}
	if batchSize <= 0 {
		batchSize = batch.DefaultBatchSize
	}

	return &ParallelBatchProcessor{
		repository: repository,
		workers:    workers,
		batchSize:  batchSize,
		logger:     logger,
	}
}

// ProcessWithProgress processes items with progress tracking
func (pbp *ParallelBatchProcessor) ProcessWithProgress(ctx context.Context, items []any, progressCallback func(processed, total int)) error {
	if err := common.ValidateSliceNotEmpty("items", items); err != nil {
		return nil
	}

	// Create work channels
	workChan := make(chan []any, pbp.workers)
	resultChan := make(chan batchResult, pbp.workers)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < pbp.workers; i++ {
		wg.Add(1)
		go pbp.worker(ctx, workChan, resultChan, &wg)
	}

	// Send work
	go func() {
		defer close(workChan)
		for i := 0; i < len(items); i += pbp.batchSize {
			end := i + pbp.batchSize
			if end > len(items) {
				end = len(items)
			}

			select {
			case workChan <- items[i:end]:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Collect results and track progress
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	processed := 0
	totalBatches := (len(items) + pbp.batchSize - 1) / pbp.batchSize

	for result := range resultChan {
		processed += len(result.items)
		if progressCallback != nil {
			progressCallback(processed, len(items))
		}

		if len(result.errors) > 0 && pbp.logger != nil {
			pbp.logger.Warn("batch_processing_errors",
				zap.Int("error_count", len(result.errors)),
			)
		}
	}

	if pbp.logger != nil {
		pbp.logger.Info("parallel_batch_processing_completed",
			zap.Int("total_items", len(items)),
			zap.Int("processed_items", processed),
			zap.Int("workers", pbp.workers),
			zap.Int("total_batches", totalBatches),
		)
	}

	return nil
}

// batchResult represents the result of processing a batch
type batchResult struct {
	items  []any
	errors []error
}

// worker processes batches from the work channel
func (pbp *ParallelBatchProcessor) worker(ctx context.Context, workChan <-chan []any, resultChan chan<- batchResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for batch := range workChan {
		select {
		case <-ctx.Done():
			return
		default:
		}

		result := batchResult{
			items:  batch,
			errors: make([]error, 0),
		}

		// Process the batch
		_, err := pbp.repository.batchWriter.WriteItems(ctx, batch)
		if err != nil {
			result.errors = append(result.errors, err)
		}

		select {
		case resultChan <- result:
		case <-ctx.Done():
			return
		}
	}
}

// StreamingBatchProcessor has been replaced with SQS-based event processing
// Use pkg/storage/dynamorm/batch/sqs_processor.go for event-driven batch processing

// BatchValidationProcessor validates items before batch processing
type BatchValidationProcessor struct {
	repository *BatchRepository
	validator  func(any) error
	logger     *zap.Logger
}

// NewBatchValidationProcessor creates a batch processor with validation
func NewBatchValidationProcessor(repository *BatchRepository, validator func(any) error, logger *zap.Logger) *BatchValidationProcessor {
	return &BatchValidationProcessor{
		repository: repository,
		validator:  validator,
		logger:     logger,
	}
}

// ProcessWithValidation validates and processes items in batches
func (bvp *BatchValidationProcessor) ProcessWithValidation(ctx context.Context, items []any) (*ValidationResult, error) {
	if err := common.ValidateSliceNotEmpty("items", items); err != nil {
		return &ValidationResult{}, nil
	}

	result := &ValidationResult{
		TotalItems:   len(items),
		ValidItems:   make([]any, 0, len(items)),
		InvalidItems: make([]ValidationError, 0),
	}

	// Validate all items first
	for i, item := range items {
		if err := bvp.validator(item); err != nil {
			result.InvalidItems = append(result.InvalidItems, ValidationError{
				Index: i,
				Item:  item,
				Error: err,
			})
		} else {
			result.ValidItems = append(result.ValidItems, item)
		}
	}

	result.ValidCount = len(result.ValidItems)
	result.InvalidCount = len(result.InvalidItems)

	// Process valid items
	if err := common.ValidateSliceNotEmpty("valid_items", result.ValidItems); err == nil {
		batchResult, err := bvp.repository.batchWriter.WriteItems(ctx, result.ValidItems)
		if err != nil {
			return result, fmt.Errorf("batch processing failed: %w", err)
		}
		result.Duration = batchResult.Duration
		result.ProcessedCount = batchResult.ProcessedItems
		result.FailedCount = batchResult.FailedItems
	}

	if bvp.logger != nil {
		bvp.logger.Info("batch_validation_completed",
			zap.Int("total_items", result.TotalItems),
			zap.Int("valid_items", result.ValidCount),
			zap.Int("invalid_items", result.InvalidCount),
			zap.Int("processed_items", result.ProcessedCount),
			zap.Duration("duration", result.Duration),
		)
	}

	return result, nil
}

// ValidationResult contains the results of batch validation and processing
type ValidationResult struct {
	TotalItems     int
	ValidCount     int
	InvalidCount   int
	ProcessedCount int
	FailedCount    int
	ValidItems     []any
	InvalidItems   []ValidationError
	Duration       time.Duration
}

// ValidationError represents a validation error for a specific item
type ValidationError struct {
	Index int
	Item  any
	Error error
}

// GetValidationSummary returns a summary of validation results
func (vr *ValidationResult) GetValidationSummary() map[string]any {
	return map[string]any{
		"total_items":     vr.TotalItems,
		"valid_count":     vr.ValidCount,
		"invalid_count":   vr.InvalidCount,
		"processed_count": vr.ProcessedCount,
		"failed_count":    vr.FailedCount,
		"success_rate":    float64(vr.ProcessedCount) / float64(vr.ValidCount) * 100,
		"validation_rate": float64(vr.ValidCount) / float64(vr.TotalItems) * 100,
		"duration":        vr.Duration.String(),
	}
}
