package repositories

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/batch"
	repoTesting "github.com/equaltoai/lesser/pkg/storage/dynamorm/repositories/testing"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeBatchWriter struct {
	mu sync.Mutex

	writeItemsCalls         [][]any
	writeItemsParallelCalls []struct {
		items   []any
		workers int
	}

	writeItemsFn         func(context.Context, []any) (*batch.BatchWriteResult, error)
	writeItemsParallelFn func(context.Context, []any, int) (*batch.BatchWriteResult, error)
}

func (f *fakeBatchWriter) WriteItems(ctx context.Context, items []any) (*batch.BatchWriteResult, error) {
	f.mu.Lock()
	f.writeItemsCalls = append(f.writeItemsCalls, items)
	fn := f.writeItemsFn
	f.mu.Unlock()

	if fn != nil {
		return fn(ctx, items)
	}
	return &batch.BatchWriteResult{
		TotalItems:     len(items),
		ProcessedItems: len(items),
		FailedItems:    0,
		Duration:       time.Millisecond,
	}, nil
}

func (f *fakeBatchWriter) WriteItemsParallel(ctx context.Context, items []any, workers int) (*batch.BatchWriteResult, error) {
	f.mu.Lock()
	f.writeItemsParallelCalls = append(f.writeItemsParallelCalls, struct {
		items   []any
		workers int
	}{items: items, workers: workers})
	fn := f.writeItemsParallelFn
	f.mu.Unlock()

	if fn != nil {
		return fn(ctx, items, workers)
	}
	return &batch.BatchWriteResult{
		TotalItems:     len(items),
		ProcessedItems: len(items),
		FailedItems:    0,
		Duration:       time.Millisecond,
	}, nil
}

type fakeBatchDeleter struct {
	mu sync.Mutex

	deleteItemsCalls []struct {
		keys []any
	}
	deleteItemsParallelCalls []struct {
		keys    []any
		workers int
	}
	deleteItemsWithRetryCalls []struct {
		keys       []any
		maxRetries int
	}

	deleteItemsFn          func(context.Context, []any) (*batch.BatchDeleteResult, error)
	deleteItemsParallelFn  func(context.Context, []any, int) (*batch.BatchDeleteResult, error)
	deleteItemsWithRetryFn func(context.Context, []any, int) (*batch.BatchDeleteResult, error)
}

func (f *fakeBatchDeleter) DeleteItems(ctx context.Context, keys []any) (*batch.BatchDeleteResult, error) {
	f.mu.Lock()
	f.deleteItemsCalls = append(f.deleteItemsCalls, struct{ keys []any }{keys: keys})
	fn := f.deleteItemsFn
	f.mu.Unlock()

	if fn != nil {
		return fn(ctx, keys)
	}
	return &batch.BatchDeleteResult{
		TotalItems:     len(keys),
		ProcessedItems: len(keys),
		FailedItems:    0,
		Duration:       time.Millisecond,
	}, nil
}

func (f *fakeBatchDeleter) DeleteItemsParallel(ctx context.Context, keys []any, workers int) (*batch.BatchDeleteResult, error) {
	f.mu.Lock()
	f.deleteItemsParallelCalls = append(f.deleteItemsParallelCalls, struct {
		keys    []any
		workers int
	}{keys: keys, workers: workers})
	fn := f.deleteItemsParallelFn
	f.mu.Unlock()

	if fn != nil {
		return fn(ctx, keys, workers)
	}
	return &batch.BatchDeleteResult{
		TotalItems:     len(keys),
		ProcessedItems: len(keys),
		FailedItems:    0,
		Duration:       time.Millisecond,
	}, nil
}

func (f *fakeBatchDeleter) DeleteItemsWithRetry(ctx context.Context, keys []any, maxRetries int) (*batch.BatchDeleteResult, error) {
	f.mu.Lock()
	f.deleteItemsWithRetryCalls = append(f.deleteItemsWithRetryCalls, struct {
		keys       []any
		maxRetries int
	}{keys: keys, maxRetries: maxRetries})
	fn := f.deleteItemsWithRetryFn
	f.mu.Unlock()

	if fn != nil {
		return fn(ctx, keys, maxRetries)
	}
	return &batch.BatchDeleteResult{
		TotalItems:     len(keys),
		ProcessedItems: len(keys),
		FailedItems:    0,
		Duration:       time.Millisecond,
	}, nil
}

type fakeBatchReader struct {
	mu sync.Mutex

	readItemsCalls []struct {
		keys []any
		dest any
	}
	readItemsFn func(context.Context, []any, any) (*batch.BatchReadResult, error)
}

func (f *fakeBatchReader) ReadItems(ctx context.Context, keys []any, dest any) (*batch.BatchReadResult, error) {
	f.mu.Lock()
	f.readItemsCalls = append(f.readItemsCalls, struct {
		keys []any
		dest any
	}{keys: keys, dest: dest})
	fn := f.readItemsFn
	f.mu.Unlock()

	if fn != nil {
		return fn(ctx, keys, dest)
	}
	return &batch.BatchReadResult{
		TotalKeys:      len(keys),
		RetrievedItems: 0,
		Duration:       time.Millisecond,
	}, nil
}

func TestBatchRepository_BatchDelete_DelegatesToDeleter(t *testing.T) {
	t.Parallel()

	repo := NewBatchRepository(&repoTesting.MockDB{}, "test", zap.NewNop(), cost.New())
	deleter := &fakeBatchDeleter{}
	repo.batchDeleter = deleter

	keys := []any{map[string]any{"PK": "p1", "SK": "s1"}}
	result, err := repo.BatchDelete(context.Background(), keys)
	require.NoError(t, err)
	require.NotNil(t, result)

	deleter.mu.Lock()
	require.Len(t, deleter.deleteItemsCalls, 1)
	require.Equal(t, keys, deleter.deleteItemsCalls[0].keys)
	deleter.mu.Unlock()
}

func TestBatchRepository_BatchDeleteParallel_DelegatesToDeleter(t *testing.T) {
	t.Parallel()

	repo := NewBatchRepository(&repoTesting.MockDB{}, "test", zap.NewNop(), cost.New())
	deleter := &fakeBatchDeleter{}
	repo.batchDeleter = deleter

	keys := []any{map[string]any{"PK": "p1", "SK": "s1"}}
	result, err := repo.BatchDeleteParallel(context.Background(), keys, 2)
	require.NoError(t, err)
	require.NotNil(t, result)

	deleter.mu.Lock()
	require.Len(t, deleter.deleteItemsParallelCalls, 1)
	require.Equal(t, keys, deleter.deleteItemsParallelCalls[0].keys)
	require.Equal(t, 2, deleter.deleteItemsParallelCalls[0].workers)
	deleter.mu.Unlock()
}

func TestBatchRepository_BatchDeleteWithRetry_DelegatesToDeleter(t *testing.T) {
	t.Parallel()

	repo := NewBatchRepository(&repoTesting.MockDB{}, "test", zap.NewNop(), cost.New())
	deleter := &fakeBatchDeleter{}
	repo.batchDeleter = deleter

	keys := []any{map[string]any{"PK": "p1", "SK": "s1"}}
	result, err := repo.BatchDeleteWithRetry(context.Background(), keys, 3)
	require.NoError(t, err)
	require.NotNil(t, result)

	deleter.mu.Lock()
	require.Len(t, deleter.deleteItemsWithRetryCalls, 1)
	require.Equal(t, keys, deleter.deleteItemsWithRetryCalls[0].keys)
	require.Equal(t, 3, deleter.deleteItemsWithRetryCalls[0].maxRetries)
	deleter.mu.Unlock()
}

func TestCostTrackerAdapter_TrackDynamoRead_ErrorPathIsHandled(t *testing.T) {
	t.Parallel()

	tracker := cost.New()
	adapter := &costTrackerAdapter{tracker: tracker}

	// Exceed the per-request cost threshold to force an error in the underlying tracker.
	adapter.TrackDynamoRead(41)
	adapter.TrackDynamoWrite(9)
}

func TestTimelineBatchOperations_BatchInsertTimelineEntries_SuccessSequential(t *testing.T) {
	t.Parallel()

	ops := NewTimelineBatchOperations(&repoTesting.MockDB{}, zap.NewNop(), cost.New())
	writer := &fakeBatchWriter{}
	ops.batchWriter = writer

	createdAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	err := ops.BatchInsertTimelineEntries(context.Background(), []string{"alice", "bob"}, "status123", "author456", createdAt)
	require.NoError(t, err)

	writer.mu.Lock()
	require.Len(t, writer.writeItemsCalls, 1)
	items := writer.writeItemsCalls[0]
	writer.mu.Unlock()

	require.Len(t, items, 2)
	entry0, ok := items[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "USER#alice", entry0["PK"])
	require.Equal(t, "TIMELINE#"+createdAt.Format("20060102150405")+"#status123", entry0["SK"])
	require.Equal(t, "status123", entry0["StatusID"])
	require.Equal(t, "author456", entry0["AuthorID"])
	require.Equal(t, createdAt, entry0["CreatedAt"])
	require.Equal(t, "home", entry0["Type"])
}

func TestTimelineBatchOperations_BatchInsertTimelineEntries_LargeListUsesParallel(t *testing.T) {
	t.Parallel()

	ops := NewTimelineBatchOperations(&repoTesting.MockDB{}, zap.NewNop(), cost.New())
	writer := &fakeBatchWriter{}
	ops.batchWriter = writer

	followerIDs := make([]string, 101)
	for i := range followerIDs {
		followerIDs[i] = fmt.Sprintf("user%d", i)
	}

	createdAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	err := ops.BatchInsertTimelineEntries(context.Background(), followerIDs, "status123", "author456", createdAt)
	require.NoError(t, err)

	writer.mu.Lock()
	require.Len(t, writer.writeItemsParallelCalls, 1)
	require.Equal(t, 4, writer.writeItemsParallelCalls[0].workers)
	require.Len(t, writer.writeItemsParallelCalls[0].items, 101)
	writer.mu.Unlock()
}

func TestTimelineBatchOperations_BatchInsertTimelineEntries_ParallelErrorPropagates(t *testing.T) {
	t.Parallel()

	ops := NewTimelineBatchOperations(&repoTesting.MockDB{}, zap.NewNop(), cost.New())
	writer := &fakeBatchWriter{
		writeItemsParallelFn: func(context.Context, []any, int) (*batch.BatchWriteResult, error) {
			return &batch.BatchWriteResult{FailedItems: 1}, errors.New("write failed")
		},
	}
	ops.batchWriter = writer

	followerIDs := make([]string, 101)
	for i := range followerIDs {
		followerIDs[i] = fmt.Sprintf("user%d", i)
	}

	createdAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	err := ops.BatchInsertTimelineEntries(context.Background(), followerIDs, "status123", "author456", createdAt)
	require.Error(t, err)
	require.Contains(t, err.Error(), "write failed")
}

func TestTimelineBatchOperations_BatchRemoveTimelineEntries_Success(t *testing.T) {
	t.Parallel()

	ops := NewTimelineBatchOperations(&repoTesting.MockDB{}, zap.NewNop(), cost.New())
	deleter := &fakeBatchDeleter{}
	ops.batchDeleter = deleter

	err := ops.BatchRemoveTimelineEntries(context.Background(), []string{"alice"}, "author456")
	require.NoError(t, err)

	deleter.mu.Lock()
	require.Len(t, deleter.deleteItemsCalls, 1)
	require.Len(t, deleter.deleteItemsCalls[0].keys, 1)
	key0, ok := deleter.deleteItemsCalls[0].keys[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "USER#alice", key0["PK"])
	require.Equal(t, "TIMELINE_AUTHOR#author456", key0["SK"])
	deleter.mu.Unlock()
}

func TestNotificationBatchOperations_BatchCreateMentionNotifications_Success(t *testing.T) {
	t.Parallel()

	ops := NewNotificationBatchOperations(&repoTesting.MockDB{}, zap.NewNop(), cost.New())
	writer := &fakeBatchWriter{}
	ops.batchWriter = writer

	err := ops.BatchCreateMentionNotifications(context.Background(), []string{"alice", "bob"}, "status123", "author456")
	require.NoError(t, err)

	writer.mu.Lock()
	require.Len(t, writer.writeItemsCalls, 1)
	items := writer.writeItemsCalls[0]
	writer.mu.Unlock()

	require.Len(t, items, 2)
	notif0, ok := items[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "USER#alice", notif0["PK"])
	require.Equal(t, "mention", notif0["Type"])
	require.Equal(t, "author456", notif0["ActorID"])
	require.Equal(t, "status123", notif0["TargetID"])
	require.Equal(t, "status", notif0["TargetType"])
	require.Equal(t, false, notif0["IsRead"])
	require.Equal(t, "status123_alice", notif0["ID"])

	createdAt, ok := notif0["CreatedAt"].(time.Time)
	require.True(t, ok)
	expiresAt, ok := notif0["ExpiresAt"].(time.Time)
	require.True(t, ok)
	require.Equal(t, 30*24*time.Hour, expiresAt.Sub(createdAt))
}

func TestNotificationBatchOperations_BatchMarkNotificationsRead_Success(t *testing.T) {
	t.Parallel()

	ops := NewNotificationBatchOperations(&repoTesting.MockDB{}, zap.NewNop(), cost.New())
	writer := &fakeBatchWriter{}
	ops.batchWriter = writer

	err := ops.BatchMarkNotificationsRead(context.Background(), "alice", []string{"notif1", "notif2"})
	require.NoError(t, err)

	writer.mu.Lock()
	require.Len(t, writer.writeItemsCalls, 1)
	updates := writer.writeItemsCalls[0]
	writer.mu.Unlock()

	require.Len(t, updates, 2)
	update0, ok := updates[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "USER#alice", update0["PK"])
	require.Equal(t, "NOTIF#notif1", update0["SK"])
	require.Equal(t, true, update0["IsRead"])
	_, ok = update0["ReadAt"].(time.Time)
	require.True(t, ok)
}

func TestMediaBatchOperations_BatchUpdateMediaStatus_Success(t *testing.T) {
	t.Parallel()

	ops := NewMediaBatchOperations(&repoTesting.MockDB{}, zap.NewNop(), cost.New())
	writer := &fakeBatchWriter{}
	ops.batchWriter = writer

	processedAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	err := ops.BatchUpdateMediaStatus(context.Background(), []string{"m1"}, "processed", &processedAt)
	require.NoError(t, err)

	writer.mu.Lock()
	require.Len(t, writer.writeItemsCalls, 1)
	updates := writer.writeItemsCalls[0]
	writer.mu.Unlock()

	require.Len(t, updates, 1)
	update0, ok := updates[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "MEDIA#m1", update0["PK"])
	require.Equal(t, "VERSION#original", update0["SK"])
	require.Equal(t, "processed", update0["Status"])
	require.Equal(t, &processedAt, update0["ProcessedAt"])
	_, ok = update0["UpdatedAt"].(time.Time)
	require.True(t, ok)
}

func TestMediaBatchOperations_BatchCleanupExpiredMedia_UsesRetry(t *testing.T) {
	t.Parallel()

	ops := NewMediaBatchOperations(&repoTesting.MockDB{}, zap.NewNop(), cost.New())
	deleter := &fakeBatchDeleter{}
	ops.batchDeleter = deleter

	expiredKeys := []map[string]any{
		{"PK": "MEDIA#m1", "SK": "VERSION#original"},
		{"PK": "MEDIA#m2", "SK": "VERSION#original"},
	}
	err := ops.BatchCleanupExpiredMedia(context.Background(), expiredKeys)
	require.NoError(t, err)

	deleter.mu.Lock()
	require.Len(t, deleter.deleteItemsWithRetryCalls, 1)
	require.Equal(t, 3, deleter.deleteItemsWithRetryCalls[0].maxRetries)
	require.Len(t, deleter.deleteItemsWithRetryCalls[0].keys, 2)
	deleter.mu.Unlock()
}

func TestAdvancedBatchOperations_BatchUpsertWithConflictResolution_SplitsBatches(t *testing.T) {
	t.Parallel()

	ops := NewAdvancedBatchOperations(&repoTesting.MockDB{}, "test", zap.NewNop(), cost.New())
	writer := &fakeBatchWriter{}
	ops.batchWriter = writer

	items := make([]any, 12)
	for i := range items {
		items[i] = map[string]any{"PK": fmt.Sprintf("pk%d", i), "SK": fmt.Sprintf("sk%d", i)}
	}

	err := ops.BatchUpsertWithConflictResolution(context.Background(), items, func(existing, newItem any) any { return newItem })
	require.NoError(t, err)

	writer.mu.Lock()
	require.Len(t, writer.writeItemsCalls, 2)
	require.Len(t, writer.writeItemsCalls[0], 10)
	require.Len(t, writer.writeItemsCalls[1], 2)
	writer.mu.Unlock()
}

func TestAdvancedBatchOperations_processBatchWithConflictResolution_NonRetryableError(t *testing.T) {
	t.Parallel()

	ops := NewAdvancedBatchOperations(&repoTesting.MockDB{}, "test", zap.NewNop(), cost.New())
	writer := &fakeBatchWriter{
		writeItemsFn: func(context.Context, []any) (*batch.BatchWriteResult, error) {
			return &batch.BatchWriteResult{FailedItems: 1}, errors.New("fatal")
		},
	}
	ops.batchWriter = writer

	err := ops.processBatchWithConflictResolution(context.Background(), []any{map[string]any{"PK": "p", "SK": "s"}}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "non-retryable")
}

func TestAdvancedBatchOperations_processBatchWithConflictResolution_RetryWithResolver(t *testing.T) {
	t.Parallel()

	ops := NewAdvancedBatchOperations(&repoTesting.MockDB{}, "test", zap.NewNop(), cost.New())

	reader := &fakeBatchReader{
		readItemsFn: func(_ context.Context, keys []any, dest any) (*batch.BatchReadResult, error) {
			require.Len(t, keys, 1)
			keyMap, ok := keys[0].(map[string]any)
			require.True(t, ok)
			pk := fmt.Sprintf("%v", keyMap["PK"])
			sk := fmt.Sprintf("%v", keyMap["SK"])

			destValue := reflect.ValueOf(dest)
			require.Equal(t, reflect.Ptr, destValue.Kind())
			require.Equal(t, reflect.Slice, destValue.Elem().Kind())

			existing := map[string]any{"PK": pk, "SK": sk, "Value": "existing"}
			destValue.Elem().Set(reflect.Append(destValue.Elem(), reflect.ValueOf(existing)))

			return &batch.BatchReadResult{TotalKeys: 1, RetrievedItems: 1}, nil
		},
	}
	ops.batchReader = reader

	callCount := 0
	writer := &fakeBatchWriter{
		writeItemsFn: func(_ context.Context, items []any) (*batch.BatchWriteResult, error) {
			callCount++
			if callCount == 1 {
				return &batch.BatchWriteResult{FailedItems: len(items)}, errors.New("ConditionalCheckFailedException: conflict")
			}
			return &batch.BatchWriteResult{ProcessedItems: len(items)}, nil
		},
	}
	ops.batchWriter = writer

	item := map[string]any{"PK": "pk1", "SK": "sk1", "Value": "new"}
	err := ops.processBatchWithConflictResolution(context.Background(), []any{item}, func(existing, newItem any) any {
		existingMap := existing.(map[string]any)
		newMap := newItem.(map[string]any)
		return map[string]any{"PK": newMap["PK"], "SK": newMap["SK"], "Value": fmt.Sprintf("%v+%v", existingMap["Value"], newMap["Value"])}
	})
	require.NoError(t, err)
	require.Equal(t, 2, callCount)

	writer.mu.Lock()
	require.Len(t, writer.writeItemsCalls, 2)
	resolved, ok := writer.writeItemsCalls[1][0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "existing+new", resolved["Value"])
	writer.mu.Unlock()
}

func TestAdvancedBatchOperations_readExistingItem_ReaderUnavailable(t *testing.T) {
	t.Parallel()

	ops := NewAdvancedBatchOperations(&repoTesting.MockDB{}, "test", zap.NewNop(), cost.New())
	ops.batchReader = nil

	_, err := ops.readExistingItem(context.Background(), map[string]any{"PK": "pk1", "SK": "sk1"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "batch reader not available")
}

func TestAdvancedBatchOperations_readExistingItem_NotFound(t *testing.T) {
	t.Parallel()

	ops := NewAdvancedBatchOperations(&repoTesting.MockDB{}, "test", zap.NewNop(), cost.New())
	reader := &fakeBatchReader{
		readItemsFn: func(context.Context, []any, any) (*batch.BatchReadResult, error) {
			return nil, errors.New("ResourceNotFoundException")
		},
	}
	ops.batchReader = reader

	_, err := ops.readExistingItem(context.Background(), map[string]any{"PK": "pk1", "SK": "sk1"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "item not found")
}

func TestParallelBatchProcessor_ProcessWithProgress_CallsCallback(t *testing.T) {
	t.Parallel()

	repo := NewBatchRepository(&repoTesting.MockDB{}, "test", zap.NewNop(), cost.New())
	repo.batchWriter = &fakeBatchWriter{}

	processor := NewParallelBatchProcessor(repo, 2, 1, zap.NewNop())

	var progressCalls int
	progressCallback := func(processed, total int) {
		progressCalls++
		require.GreaterOrEqual(t, processed, 1)
		require.Equal(t, 3, total)
	}

	err := processor.ProcessWithProgress(context.Background(), []any{"a", "b", "c"}, progressCallback)
	require.NoError(t, err)
	require.GreaterOrEqual(t, progressCalls, 1)
}

func TestBatchValidationProcessor_ProcessWithValidation_BatchWriteFails(t *testing.T) {
	t.Parallel()

	repo := NewBatchRepository(&repoTesting.MockDB{}, "test", zap.NewNop(), cost.New())
	repo.batchWriter = &fakeBatchWriter{
		writeItemsFn: func(context.Context, []any) (*batch.BatchWriteResult, error) {
			return &batch.BatchWriteResult{FailedItems: 1}, errors.New("write failed")
		},
	}

	processor := NewBatchValidationProcessor(repo, func(_ any) error { return nil }, zap.NewNop())
	_, err := processor.ProcessWithValidation(context.Background(), []any{"a"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "batch processing failed")
}

func TestExtractKeysFromStruct(t *testing.T) {
	t.Parallel()

	type keyed struct {
		PK string `dynamodbav:"PK"`
		SK string `dynamodbav:"SK"`
	}

	pk, sk := extractKeysFromStruct(keyed{PK: "p1", SK: "s1"})
	require.Equal(t, "p1", pk)
	require.Equal(t, "s1", sk)

	pk, sk = extractKeysFromStruct(&keyed{PK: "p2", SK: "s2"})
	require.Equal(t, "p2", pk)
	require.Equal(t, "s2", sk)

	pk, sk = extractKeysFromStruct(map[string]any{"PK": "p3", "SK": "s3"})
	require.Empty(t, pk)
	require.Empty(t, sk)

	pk, sk = extractKeysFromStruct(nil)
	require.Empty(t, pk)
	require.Empty(t, sk)
}

func TestCreateSameTypeInstance(t *testing.T) {
	t.Parallel()

	type sample struct {
		PK string
	}

	ptrInstance := createSameTypeInstance(&sample{})
	_, ok := ptrInstance.(*sample)
	require.True(t, ok)

	valInstance := createSameTypeInstance(sample{})
	_, ok = valInstance.(*sample)
	require.True(t, ok)

	mapInstance := createSameTypeInstance(map[string]int{"a": 1})
	_, ok = mapInstance.(map[string]int)
	require.True(t, ok)

	otherInstance := createSameTypeInstance(123)
	_, ok = otherInstance.(*int)
	require.True(t, ok)
}

func TestIsNotFoundError(t *testing.T) {
	t.Parallel()

	require.False(t, isNotFoundError(nil))
	require.True(t, isNotFoundError(errors.New("record not found")))
	require.False(t, isNotFoundError(errors.New("boom")))
}

func TestIsDynamoDBNotFoundError(t *testing.T) {
	t.Parallel()

	require.False(t, isDynamoDBNotFoundError(nil))
	require.True(t, isDynamoDBNotFoundError(errors.New("ResourceNotFoundException")))
	require.False(t, isDynamoDBNotFoundError(errors.New("boom")))
}

func TestCostTrackerAdapter_CalculateCost(t *testing.T) {
	t.Parallel()

	empty := (&costTrackerAdapter{}).CalculateCost()
	require.Equal(t, batch.CostMetrics{}, empty)

	tracker := cost.New()
	require.NoError(t, tracker.TrackDynamoRead(1))
	require.NoError(t, tracker.TrackDynamoWrite(2))

	metrics := (&costTrackerAdapter{tracker: tracker}).CalculateCost()
	require.EqualValues(t, 1, metrics.DynamoDBReads)
	require.EqualValues(t, 2, metrics.DynamoDBWrites)
}
