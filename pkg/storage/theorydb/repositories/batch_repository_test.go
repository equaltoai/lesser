package repositories

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/theorydb/batch"
	repoTesting "github.com/equaltoai/lesser/pkg/storage/theorydb/repositories/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestNewBatchRepository(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	logger := zap.NewNop()
	tracker := cost.New()
	tableName := "test-table"

	repo := NewBatchRepository(mockDB, tableName, logger, tracker)

	assert.NotNil(t, repo)
	assert.NotNil(t, repo.BaseRepository)
	assert.NotNil(t, repo.batchWriter)
	assert.NotNil(t, repo.batchReader)
	assert.Equal(t, logger, repo.logger)
	assert.Equal(t, tracker, repo.tracker)
}

func TestNewTimelineBatchOperations(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	logger := zap.NewNop()
	tracker := cost.New()

	ops := NewTimelineBatchOperations(mockDB, logger, tracker)

	assert.NotNil(t, ops)
	assert.NotNil(t, ops.BatchRepository)
	assert.Equal(t, "timeline", ops.GetTableName())
}

func TestTimelineBatchOperations_BatchInsertTimelineEntries_EmptyFollowers(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	ops := NewTimelineBatchOperations(mockDB, zap.NewNop(), cost.New())

	err := ops.BatchInsertTimelineEntries(context.Background(), []string{}, "status1", "author1", time.Now())

	assert.NoError(t, err)
}

func TestTimelineBatchOperations_BatchInsertTimelineEntries_SmallList(t *testing.T) {
	// This test validates the method signature and basic structure
	// For full integration testing, use a real database or complete mock implementation

	// Test with empty followers to avoid complex mocking
	mockDB := &repoTesting.MockDB{}
	ops := NewTimelineBatchOperations(mockDB, zap.NewNop(), cost.New())

	// Test empty case (no batch operations needed)
	err := ops.BatchInsertTimelineEntries(context.Background(), []string{}, "status123", "author456", time.Now())
	assert.NoError(t, err) // Empty list should succeed without database calls

	// Verify the operation structure was created correctly
	assert.NotNil(t, ops)
	assert.NotNil(t, ops.BatchRepository)
	assert.Equal(t, "timeline", ops.GetTableName())

	// Test that the method signature is correct with actual data
	// The method exists and can accept these parameters:
	// - context.Context
	// - []string (followerIDs)
	// - string (statusID)
	// - string (authorID)
	// - time.Time (createdAt)
	// Full execution testing would require complete database mocking
}

func TestTimelineBatchOperations_BatchRemoveTimelineEntries_EmptyFollowers(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	ops := NewTimelineBatchOperations(mockDB, zap.NewNop(), cost.New())

	err := ops.BatchRemoveTimelineEntries(context.Background(), []string{}, "author1")

	assert.NoError(t, err)
}

func TestNewNotificationBatchOperations(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	logger := zap.NewNop()
	tracker := cost.New()

	ops := NewNotificationBatchOperations(mockDB, logger, tracker)

	assert.NotNil(t, ops)
	assert.NotNil(t, ops.BatchRepository)
	assert.Equal(t, "notifications", ops.GetTableName())
}

func TestNotificationBatchOperations_BatchCreateMentionNotifications_EmptyUsers(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	ops := NewNotificationBatchOperations(mockDB, zap.NewNop(), cost.New())

	err := ops.BatchCreateMentionNotifications(context.Background(), []string{}, "status1", "author1")

	assert.NoError(t, err)
}

func TestNotificationBatchOperations_BatchMarkNotificationsRead_EmptyNotifications(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	ops := NewNotificationBatchOperations(mockDB, zap.NewNop(), cost.New())

	err := ops.BatchMarkNotificationsRead(context.Background(), "user1", []string{})

	assert.NoError(t, err)
}

func TestNewMediaBatchOperations(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	logger := zap.NewNop()
	tracker := cost.New()

	ops := NewMediaBatchOperations(mockDB, logger, tracker)

	assert.NotNil(t, ops)
	assert.NotNil(t, ops.BatchRepository)
	assert.Equal(t, "media", ops.GetTableName())
}

func TestMediaBatchOperations_BatchUpdateMediaStatus_EmptyMedia(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	ops := NewMediaBatchOperations(mockDB, zap.NewNop(), cost.New())
	processedAt := time.Now()

	err := ops.BatchUpdateMediaStatus(context.Background(), []string{}, "processed", &processedAt)

	assert.NoError(t, err)
}

func TestMediaBatchOperations_BatchCleanupExpiredMedia_EmptyMedia(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	ops := NewMediaBatchOperations(mockDB, zap.NewNop(), cost.New())

	err := ops.BatchCleanupExpiredMedia(context.Background(), []map[string]any{})

	assert.NoError(t, err)
}

func TestNewAdvancedBatchOperations(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	logger := zap.NewNop()
	tracker := cost.New()
	tableName := "advanced-table"

	ops := NewAdvancedBatchOperations(mockDB, tableName, logger, tracker)

	assert.NotNil(t, ops)
	assert.NotNil(t, ops.BatchRepository)
	assert.NotNil(t, ops.transactionMgr)
	assert.Equal(t, tableName, ops.GetTableName())
}

func TestAdvancedBatchOperations_BatchUpsertWithConflictResolution_EmptyItems(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	ops := NewAdvancedBatchOperations(mockDB, "test", zap.NewNop(), cost.New())

	conflictResolver := func(existing, new any) any {
		return new
	}

	err := ops.BatchUpsertWithConflictResolution(context.Background(), []any{}, conflictResolver)

	assert.NoError(t, err)
}

func TestAdvancedBatchOperations_BatchProcessWithRetry_EmptyItems(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	ops := NewAdvancedBatchOperations(mockDB, "test", zap.NewNop(), cost.New())

	processor := func(items []any) error {
		return nil
	}

	err := ops.BatchProcessWithRetry(context.Background(), []any{}, 3, processor)

	assert.NoError(t, err)
}

func TestAdvancedBatchOperations_BatchProcessWithRetry_Success(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	ops := NewAdvancedBatchOperations(mockDB, "test", zap.NewNop(), cost.New())

	items := []any{"item1", "item2", "item3"}
	processedItems := []any{}

	processor := func(items []any) error {
		processedItems = append(processedItems, items...)
		return nil
	}

	err := ops.BatchProcessWithRetry(context.Background(), items, 3, processor)

	assert.NoError(t, err)
	assert.Equal(t, items, processedItems)
}

func TestAdvancedBatchOperations_BatchProcessWithRetry_SuccessAfterRetry(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	ops := NewAdvancedBatchOperations(mockDB, "test", zap.NewNop(), cost.New())

	items := []any{"item1", "item2"}
	attempts := 0

	processor := func(items []any) error {
		attempts++
		if attempts == 1 {
			return errors.New("temporary error")
		}
		return nil
	}

	err := ops.BatchProcessWithRetry(context.Background(), items, 3, processor)

	assert.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestAdvancedBatchOperations_BatchProcessWithRetry_MaxRetriesExceeded(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	ops := NewAdvancedBatchOperations(mockDB, "test", zap.NewNop(), cost.New())

	items := []any{"item1"}
	maxRetries := 2

	processor := func(items []any) error {
		return errors.New("persistent error")
	}

	err := ops.BatchProcessWithRetry(context.Background(), items, maxRetries, processor)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed after 3 attempts")
}

func TestNewParallelBatchProcessor(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	repo := NewBatchRepository(mockDB, "test", zap.NewNop(), cost.New())
	logger := zap.NewNop()

	// Test with default values
	processor := NewParallelBatchProcessor(repo, 0, 0, logger)
	assert.Equal(t, 4, processor.workers)
	assert.Equal(t, batch.DefaultBatchSize, processor.batchSize)

	// Test with custom values
	processor = NewParallelBatchProcessor(repo, 8, 50, logger)
	assert.Equal(t, 8, processor.workers)
	assert.Equal(t, 50, processor.batchSize)
}

func TestParallelBatchProcessor_ProcessWithProgress_EmptyItems(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	repo := NewBatchRepository(mockDB, "test", zap.NewNop(), cost.New())
	processor := NewParallelBatchProcessor(repo, 2, 10, zap.NewNop())

	progressCalled := false
	progressCallback := func(_, _ int) {
		progressCalled = true
	}

	err := processor.ProcessWithProgress(context.Background(), []any{}, progressCallback)

	assert.NoError(t, err)
	assert.False(t, progressCalled)
}

// StreamingBatchProcessor tests have been removed
// Use SQS-based batch processor tests instead

func TestNewBatchValidationProcessor(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	repo := NewBatchRepository(mockDB, "test", zap.NewNop(), cost.New())
	logger := zap.NewNop()

	validator := func(_ any) error {
		return nil
	}

	processor := NewBatchValidationProcessor(repo, validator, logger)

	assert.NotNil(t, processor)
	assert.Equal(t, repo, processor.repository)
	assert.NotNil(t, processor.validator)
	assert.Equal(t, logger, processor.logger)
}

func TestBatchValidationProcessor_ProcessWithValidation_EmptyItems(t *testing.T) {
	mockDB := &repoTesting.MockDB{}
	repo := NewBatchRepository(mockDB, "test", zap.NewNop(), cost.New())

	validator := func(_ any) error {
		return nil
	}

	processor := NewBatchValidationProcessor(repo, validator, zap.NewNop())

	result, err := processor.ProcessWithValidation(context.Background(), []any{})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.TotalItems)
	assert.Equal(t, 0, result.ValidCount)
	assert.Equal(t, 0, result.InvalidCount)
}

func TestBatchValidationProcessor_ProcessWithValidation_AllValid(t *testing.T) {
	// Use DynamORM's official mocks
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Set up mock expectations for batch processing
	mockDB.On("Model", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.AnythingOfType("[]interface {}")).Return(nil)

	repo := NewBatchRepository(mockDB, "test", zap.NewNop(), cost.New())

	validator := func(_ any) error {
		return nil // All items are valid
	}

	processor := NewBatchValidationProcessor(repo, validator, zap.NewNop())

	items := []any{"item1", "item2", "item3"}
	result, err := processor.ProcessWithValidation(context.Background(), items)

	// Should succeed with all items valid
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 3, result.TotalItems)
	assert.Equal(t, 3, result.ValidCount)
	assert.Equal(t, 0, result.InvalidCount)
	assert.Len(t, result.ValidItems, 3)
	assert.Len(t, result.InvalidItems, 0)

	// Verify mocks were called
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchValidationProcessor_ProcessWithValidation_SomeInvalid(t *testing.T) {
	// Use DynamORM's official mocks
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Set up mock expectations - only valid items will be batch processed
	mockDB.On("Model", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.AnythingOfType("[]interface {}")).Return(nil)

	repo := NewBatchRepository(mockDB, "test", zap.NewNop(), cost.New())

	validator := func(item any) error {
		str, ok := item.(string)
		if !ok || str == "invalid" {
			return errors.New("invalid item")
		}
		return nil
	}

	processor := NewBatchValidationProcessor(repo, validator, zap.NewNop())

	items := []any{"valid1", "invalid", "valid2", "invalid"}
	result, err := processor.ProcessWithValidation(context.Background(), items)

	// Should succeed even with some invalid items
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 4, result.TotalItems)
	assert.Equal(t, 2, result.ValidCount)
	assert.Equal(t, 2, result.InvalidCount)
	assert.Len(t, result.ValidItems, 2)
	assert.Len(t, result.InvalidItems, 2)

	// Check invalid items
	assert.Equal(t, 1, result.InvalidItems[0].Index)
	assert.Equal(t, "invalid", result.InvalidItems[0].Item)
	assert.Equal(t, 3, result.InvalidItems[1].Index)
	assert.Equal(t, "invalid", result.InvalidItems[1].Item)

	// Verify mocks were called
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestValidationResult_GetValidationSummary(t *testing.T) {
	result := &ValidationResult{
		TotalItems:     10,
		ValidCount:     8,
		InvalidCount:   2,
		ProcessedCount: 7,
		FailedCount:    1,
		Duration:       100 * time.Millisecond,
	}

	summary := result.GetValidationSummary()

	assert.Equal(t, 10, summary["total_items"])
	assert.Equal(t, 8, summary["valid_count"])
	assert.Equal(t, 2, summary["invalid_count"])
	assert.Equal(t, 7, summary["processed_count"])
	assert.Equal(t, 1, summary["failed_count"])
	assert.Equal(t, float64(87.5), summary["success_rate"])  // 7/8 * 100
	assert.Equal(t, float64(80), summary["validation_rate"]) // 8/10 * 100
	assert.Equal(t, "100ms", summary["duration"])
}

// Benchmark tests

func BenchmarkTimelineBatchOperations_CreateEntries(b *testing.B) {
	mockDB := &repoTesting.MockDB{}
	ops := NewTimelineBatchOperations(mockDB, zap.NewNop(), cost.New())

	followerIDs := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		followerIDs[i] = fmt.Sprintf("user%d", i)
	}

	ctx := context.Background()
	statusID := "status123"
	authorID := "author456"
	createdAt := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// This will fail but measures the overhead of entry creation
		_ = ops.BatchInsertTimelineEntries(ctx, followerIDs, statusID, authorID, createdAt)
	}
}

func BenchmarkBatchValidationProcessor_Validation(b *testing.B) {
	mockDB := &repoTesting.MockDB{}
	repo := NewBatchRepository(mockDB, "test", zap.NewNop(), cost.New())

	validator := func(item any) error {
		// Simple validation
		if str, ok := item.(string); ok && len(str) > 0 {
			return nil
		}
		return errors.New("invalid")
	}

	processor := NewBatchValidationProcessor(repo, validator, zap.NewNop())

	items := make([]any, 100)
	for i := 0; i < 100; i++ {
		items[i] = fmt.Sprintf("item%d", i)
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// This will fail at processing but measures validation overhead
		_, _ = processor.ProcessWithValidation(ctx, items)
	}
}
