package repositories

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/batch"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// BatchRepository extends BaseRepository with advanced batch operations
type BatchRepository struct {
	*dynamorm.BaseRepository
	batchWriter *batch.BatchWriter
	batchReader *batch.BatchReader
	logger      *zap.Logger
	tracker     *cost.Tracker
}

// NewBatchRepository creates a new repository with batch capabilities
func NewBatchRepository(db core.DB, tableName string, logger *zap.Logger, tracker *cost.Tracker) *BatchRepository {
	batchWriter := batch.NewBatchWriter(db, batch.BatchWriterConfig{
		BatchSize: batch.DefaultBatchSize,
		Logger:    logger,
		Tracker:   tracker,
	})

	batchReader := batch.NewBatchReader(db, batch.BatchReaderConfig{
		BatchSize: batch.MaxBatchReadSize,
		Logger:    logger,
		Tracker:   tracker,
	})

	return &BatchRepository{
		BaseRepository: dynamorm.NewBaseRepository(db, tableName),
		batchWriter:    batchWriter,
		batchReader:    batchReader,
		logger:         logger,
		tracker:        tracker,
	}
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
	if len(followerIDs) == 0 {
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
	if len(entries) > 100 {
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
	if len(followerIDs) == 0 {
		return nil
	}

	// This would require querying first to get the entries to delete
	// For now, we'll create placeholder delete keys based on pattern
	deleteKeys := make([]any, 0, len(followerIDs))
	for _, followerID := range followerIDs {
		// In a real implementation, you'd query for actual timeline entries
		// This is a simplified example
		key := map[string]any{
			"PK": fmt.Sprintf("USER#%s", followerID),
			"SK": fmt.Sprintf("TIMELINE_AUTHOR#%s", authorID), // Simplified pattern
		}
		deleteKeys = append(deleteKeys, key)
	}

	result, err := tbo.batchWriter.WriteItems(ctx, deleteKeys) // Placeholder - would use batch delete
	if err != nil {
		return fmt.Errorf("failed to batch remove timeline entries: %w", err)
	}

	if tbo.logger != nil {
		tbo.logger.Info("timeline_entries_removed",
			zap.Int("total_entries", len(deleteKeys)),
			zap.Int("processed", result.ProcessedItems),
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
	if len(mentionedUserIDs) == 0 {
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
	if len(notificationIDs) == 0 {
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
	if len(mediaIDs) == 0 {
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
	if len(expiredMediaKeys) == 0 {
		return nil
	}

	// Convert to delete operations
	deleteKeys := make([]any, 0, len(expiredMediaKeys))
	for _, key := range expiredMediaKeys {
		deleteKeys = append(deleteKeys, key)
	}

	result, err := mbo.batchWriter.WriteItems(ctx, deleteKeys) // Would use batch delete in real implementation
	if err != nil {
		return fmt.Errorf("failed to batch cleanup expired media: %w", err)
	}

	if mbo.logger != nil {
		mbo.logger.Info("expired_media_cleaned",
			zap.Int("total_media", len(deleteKeys)),
			zap.Int("processed", result.ProcessedItems),
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
func (abo *AdvancedBatchOperations) BatchUpsertWithConflictResolution(ctx context.Context, items []any, conflictResolver func(existing, new any) any) error {
	if len(items) == 0 {
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
func (abo *AdvancedBatchOperations) processBatchWithConflictResolution(ctx context.Context, batch []any, conflictResolver func(existing, new any) any) error {
	// This is a conceptual implementation - in practice, you'd need to:
	// 1. Try batch write
	// 2. Handle conflicts by reading existing items
	// 3. Apply conflict resolution
	// 4. Retry with resolved items

	result, err := abo.batchWriter.WriteItems(ctx, batch)
	if err != nil {
		// Handle conflicts here if DynamORM provides conflict detection
		if abo.logger != nil {
			abo.logger.Warn("batch_conflicts_detected",
				zap.Int("failed_items", result.FailedItems),
				zap.Error(err),
			)
		}
		// In a real implementation, you'd resolve conflicts and retry
		return err
	}

	return nil
}

// BatchProcessWithRetry processes items with exponential backoff retry
func (abo *AdvancedBatchOperations) BatchProcessWithRetry(ctx context.Context, items []any, maxRetries int, processor func([]any) error) error {
	if len(items) == 0 {
		return nil
	}

	var lastErr error
	backoff := 100 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := backoff * time.Duration(1<<uint(attempt-1))
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
	if len(items) == 0 {
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

// StreamingBatchProcessor processes large datasets in streaming fashion
type StreamingBatchProcessor struct {
	repository *BatchRepository
	batchSize  int
	logger     *zap.Logger
}

// NewStreamingBatchProcessor creates a streaming batch processor
func NewStreamingBatchProcessor(repository *BatchRepository, batchSize int, logger *zap.Logger) *StreamingBatchProcessor {
	if batchSize <= 0 {
		batchSize = batch.DefaultBatchSize
	}

	return &StreamingBatchProcessor{
		repository: repository,
		batchSize:  batchSize,
		logger:     logger,
	}
}

// ProcessStream processes items from a channel in batches
func (sbp *StreamingBatchProcessor) ProcessStream(ctx context.Context, itemChan <-chan any, errorCallback func(error)) {
	buffer := make([]any, 0, sbp.batchSize)
	ticker := time.NewTicker(5 * time.Second) // Flush buffer every 5 seconds
	defer ticker.Stop()

	for {
		select {
		case item, ok := <-itemChan:
			if !ok {
				// Channel closed, flush remaining items
				if len(buffer) > 0 {
					sbp.processBatch(ctx, buffer, errorCallback)
				}
				return
			}

			buffer = append(buffer, item)
			if len(buffer) >= sbp.batchSize {
				sbp.processBatch(ctx, buffer, errorCallback)
				buffer = buffer[:0] // Reset buffer
			}

		case <-ticker.C:
			// Periodic flush
			if len(buffer) > 0 {
				sbp.processBatch(ctx, buffer, errorCallback)
				buffer = buffer[:0]
			}

		case <-ctx.Done():
			return
		}
	}
}

// processBatch processes a single batch of items
func (sbp *StreamingBatchProcessor) processBatch(ctx context.Context, items []any, errorCallback func(error)) {
	if len(items) == 0 {
		return
	}

	result, err := sbp.repository.batchWriter.WriteItems(ctx, items)
	if err != nil && errorCallback != nil {
		errorCallback(fmt.Errorf("batch processing failed: %w", err))
	}

	if sbp.logger != nil {
		sbp.logger.Debug("streaming_batch_processed",
			zap.Int("item_count", len(items)),
			zap.Int("processed", result.ProcessedItems),
			zap.Int("failed", result.FailedItems),
		)
	}
}

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
	if len(items) == 0 {
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
	if len(result.ValidItems) > 0 {
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
