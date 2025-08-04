package batch

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap/zaptest"
)

// MockCostTracker implements CostTracker interface for testing
type MockCostTracker struct {
	reads  int64
	writes int64
}

func (m *MockCostTracker) CalculateCost() CostMetrics {
	return CostMetrics{
		DynamoDBReads:  m.reads,
		DynamoDBWrites: m.writes,
	}
}

func (m *MockCostTracker) TrackDynamoWrite(items int) {
	m.writes += int64(items)
}

func (m *MockCostTracker) TrackDynamoRead(items int) {
	m.reads += int64(items)
}

// MockDB implements core.DB interface for testing
type MockDB struct {
	mock.Mock
}

func (m *MockDB) Model(model any) core.Query {
	args := m.Called(model)
	return args.Get(0).(core.Query)
}

func (m *MockDB) Transaction(fn func(*core.Tx) error) error {
	args := m.Called(fn)
	return args.Error(0)
}

func (m *MockDB) AutoMigrate(models ...any) error {
	args := m.Called(models)
	return args.Error(0)
}

func (m *MockDB) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockDB) Migrate() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockDB) WithContext(ctx context.Context) core.DB {
	args := m.Called(ctx)
	return args.Get(0).(core.DB)
}

// MockQuery implements core.Query interface for testing
type MockQuery struct {
	mock.Mock
}

// Implement all required Query interface methods
func (m *MockQuery) Where(field string, op string, value any) core.Query {
	args := m.Called(field, op, value)
	return args.Get(0).(core.Query)
}

func (m *MockQuery) Index(indexName string) core.Query {
	args := m.Called(indexName)
	return args.Get(0).(core.Query)
}

func (m *MockQuery) Filter(field string, op string, value any) core.Query {
	args := m.Called(field, op, value)
	return args.Get(0).(core.Query)
}

func (m *MockQuery) OrFilter(field string, op string, value any) core.Query {
	args := m.Called(field, op, value)
	return args.Get(0).(core.Query)
}

func (m *MockQuery) FilterGroup(fn func(core.Query)) core.Query {
	args := m.Called(fn)
	return args.Get(0).(core.Query)
}

func (m *MockQuery) OrFilterGroup(fn func(core.Query)) core.Query {
	args := m.Called(fn)
	return args.Get(0).(core.Query)
}

func (m *MockQuery) OrderBy(field string, order string) core.Query {
	args := m.Called(field, order)
	return args.Get(0).(core.Query)
}

func (m *MockQuery) Limit(limit int) core.Query {
	args := m.Called(limit)
	return args.Get(0).(core.Query)
}

func (m *MockQuery) Offset(offset int) core.Query {
	args := m.Called(offset)
	return args.Get(0).(core.Query)
}

func (m *MockQuery) Select(fields ...string) core.Query {
	args := m.Called(fields)
	return args.Get(0).(core.Query)
}

func (m *MockQuery) ConsistentRead() core.Query {
	args := m.Called()
	return args.Get(0).(core.Query)
}

func (m *MockQuery) WithRetry(maxRetries int, initialDelay time.Duration) core.Query {
	args := m.Called(maxRetries, initialDelay)
	return args.Get(0).(core.Query)
}

func (m *MockQuery) First(dest any) error {
	args := m.Called(dest)
	return args.Error(0)
}

func (m *MockQuery) All(dest any) error {
	args := m.Called(dest)
	return args.Error(0)
}

func (m *MockQuery) AllPaginated(dest any) (*core.PaginatedResult, error) {
	args := m.Called(dest)
	return args.Get(0).(*core.PaginatedResult), args.Error(1)
}

func (m *MockQuery) Count() (int64, error) {
	args := m.Called()
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockQuery) Create() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockQuery) CreateOrUpdate() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockQuery) Update(fields ...string) error {
	args := m.Called(fields)
	return args.Error(0)
}

func (m *MockQuery) UpdateBuilder() core.UpdateBuilder {
	args := m.Called()
	return args.Get(0).(core.UpdateBuilder)
}

func (m *MockQuery) Delete() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockQuery) Cursor(cursor string) core.Query {
	args := m.Called(cursor)
	return args.Get(0).(core.Query)
}

func (m *MockQuery) SetCursor(cursor string) error {
	args := m.Called(cursor)
	return args.Error(0)
}

func (m *MockQuery) WithContext(ctx context.Context) core.Query {
	args := m.Called(ctx)
	return args.Get(0).(core.Query)
}

func (m *MockQuery) Scan(dest any) error {
	args := m.Called(dest)
	return args.Error(0)
}

func (m *MockQuery) ParallelScan(segment int32, totalSegments int32) core.Query {
	args := m.Called(segment, totalSegments)
	return args.Get(0).(core.Query)
}

func (m *MockQuery) ScanAllSegments(dest any, totalSegments int32) error {
	args := m.Called(dest, totalSegments)
	return args.Error(0)
}

func (m *MockQuery) BatchGet(keys []any, dest any) error {
	args := m.Called(keys, dest)
	return args.Error(0)
}

func (m *MockQuery) BatchCreate(items any) error {
	args := m.Called(items)
	return args.Error(0)
}

func (m *MockQuery) BatchDelete(keys []any) error {
	args := m.Called(keys)
	return args.Error(0)
}

func (m *MockQuery) BatchWrite(putItems []any, deleteKeys []any) error {
	args := m.Called(putItems, deleteKeys)
	return args.Error(0)
}

func (m *MockQuery) BatchUpdateWithOptions(items []any, fields []string, options ...any) error {
	args := m.Called(items, fields, options)
	return args.Error(0)
}

// Test model for testing
type TestItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Data string `json:"data"`
}

func TestNewBatchWriter(t *testing.T) {
	tests := []struct {
		name            string
		config          BatchWriterConfig
		expectedBatch   int
		expectedLogger  bool
		expectedTracker bool
	}{
		{
			name: "default config",
			config: BatchWriterConfig{
				BatchSize: 0, // Should default to DefaultBatchSize
			},
			expectedBatch: DefaultBatchSize,
		},
		{
			name: "custom batch size",
			config: BatchWriterConfig{
				BatchSize: 10,
			},
			expectedBatch: 10,
		},
		{
			name: "batch size too large",
			config: BatchWriterConfig{
				BatchSize: 50, // Should be capped at MaxBatchWriteSize
			},
			expectedBatch: DefaultBatchSize,
		},
		{
			name: "with logger and tracker",
			config: BatchWriterConfig{
				BatchSize: 15,
				Logger:    zaptest.NewLogger(t),
				Tracker:   &MockCostTracker{},
			},
			expectedBatch:   15,
			expectedLogger:  true,
			expectedTracker: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := &MockDB{}
			writer := NewBatchWriter(mockDB, tt.config)

			assert.Equal(t, tt.expectedBatch, writer.batchSize)
			assert.Equal(t, mockDB, writer.client)

			if tt.expectedLogger {
				assert.NotNil(t, writer.logger)
			} else {
				assert.Nil(t, writer.logger)
			}

			if tt.expectedTracker {
				assert.NotNil(t, writer.tracker)
			} else {
				assert.Nil(t, writer.tracker)
			}
		})
	}
}

func TestNewDefaultBatchWriter(t *testing.T) {
	mockDB := &MockDB{}
	writer := NewDefaultBatchWriter(mockDB)

	assert.Equal(t, DefaultBatchSize, writer.batchSize)
	assert.Equal(t, mockDB, writer.client)
	assert.Nil(t, writer.logger)
	assert.Nil(t, writer.tracker)
}

func TestBatchWriter_WriteItems_EmptyItems(t *testing.T) {
	mockDB := &MockDB{}
	writer := NewDefaultBatchWriter(mockDB)

	result, err := writer.WriteItems(context.Background(), []any{})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.TotalItems)
	assert.Equal(t, 0, result.ProcessedItems)
	assert.Equal(t, 0, result.FailedItems)
	assert.Empty(t, result.Errors)
}

func TestBatchWriter_WriteItems_Success(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}

	items := []any{
		&TestItem{ID: "1", Name: "Item 1"},
		&TestItem{ID: "2", Name: "Item 2"},
		&TestItem{ID: "3", Name: "Item 3"},
	}

	// Setup mocks
	mockDB.On("Model", mock.AnythingOfType("*batch.TestItem")).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.AnythingOfType("[]interface {}")).Return(nil)

	writer := NewDefaultBatchWriter(mockDB)
	result, err := writer.WriteItems(context.Background(), items)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 3, result.TotalItems)
	assert.Equal(t, 3, result.ProcessedItems)
	assert.Equal(t, 0, result.FailedItems)
	assert.Empty(t, result.Errors)
	assert.Greater(t, result.Duration, time.Duration(0))

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchWriter_WriteItems_WithBatching(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}

	// Create 30 items to test batching (should create 2 batches of 25 and 1 batch of 5)
	items := make([]any, 30)
	for i := 0; i < 30; i++ {
		items[i] = &TestItem{ID: fmt.Sprintf("%d", i), Name: fmt.Sprintf("Item %d", i)}
	}

	// Setup mocks - expect 2 calls to BatchCreate
	mockDB.On("Model", mock.AnythingOfType("*batch.TestItem")).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.MatchedBy(func(items []any) bool {
		return len(items) == 25 // First batch
	})).Return(nil).Once()
	mockQuery.On("BatchCreate", mock.MatchedBy(func(items []any) bool {
		return len(items) == 5 // Second batch
	})).Return(nil).Once()

	writer := NewDefaultBatchWriter(mockDB)
	result, err := writer.WriteItems(context.Background(), items)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 30, result.TotalItems)
	assert.Equal(t, 30, result.ProcessedItems)
	assert.Equal(t, 0, result.FailedItems)
	assert.Empty(t, result.Errors)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchWriter_WriteItems_WithError(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}

	items := []any{
		&TestItem{ID: "1", Name: "Item 1"},
		&TestItem{ID: "2", Name: "Item 2"},
	}

	expectedError := errors.New("batch write failed")

	// Setup mocks
	mockDB.On("Model", mock.AnythingOfType("*batch.TestItem")).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.AnythingOfType("[]interface {}")).Return(expectedError)

	writer := NewDefaultBatchWriter(mockDB)
	result, err := writer.WriteItems(context.Background(), items)

	assert.NoError(t, err) // WriteItems doesn't return error, it collects them in result
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.TotalItems)
	assert.Equal(t, 0, result.ProcessedItems)
	assert.Equal(t, 2, result.FailedItems)
	assert.Len(t, result.Errors, 2)

	// Check that all items are marked as failed
	for i, batchErr := range result.Errors {
		assert.Equal(t, i, batchErr.Index)
		assert.Equal(t, items[i], batchErr.Item)
		assert.Contains(t, batchErr.Error.Error(), "batch write failed")
	}

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchWriter_WriteItems_WithCostTracking(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}
	tracker := &MockCostTracker{}
	logger := zaptest.NewLogger(t)

	items := []any{
		&TestItem{ID: "1", Name: "Item 1"},
		&TestItem{ID: "2", Name: "Item 2"},
	}

	// Setup mocks
	mockDB.On("Model", mock.AnythingOfType("*batch.TestItem")).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.AnythingOfType("[]interface {}")).Return(nil)

	writer := NewBatchWriter(mockDB, BatchWriterConfig{
		BatchSize: DefaultBatchSize,
		Logger:    logger,
		Tracker:   tracker,
	})

	result, err := writer.WriteItems(context.Background(), items)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.TotalItems)
	assert.Equal(t, 2, result.ProcessedItems)
	assert.Equal(t, 0, result.FailedItems)

	// Check that cost tracking was updated
	cost := tracker.CalculateCost()
	assert.Equal(t, int64(2), cost.DynamoDBWrites)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchWriter_WriteItemsParallel_Success(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}

	items := []any{
		&TestItem{ID: "1", Name: "Item 1"},
		&TestItem{ID: "2", Name: "Item 2"},
		&TestItem{ID: "3", Name: "Item 3"},
	}

	// Setup mocks
	mockDB.On("Model", mock.AnythingOfType("*batch.TestItem")).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.AnythingOfType("[]interface {}")).Return(nil)

	writer := NewDefaultBatchWriter(mockDB)
	result, err := writer.WriteItemsParallel(context.Background(), items, 2)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 3, result.TotalItems)
	assert.Equal(t, 3, result.ProcessedItems)
	assert.Equal(t, 0, result.FailedItems)
	assert.Empty(t, result.Errors)
	assert.Greater(t, result.Duration, time.Duration(0))

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchWriter_WriteItemsParallel_EmptyItems(t *testing.T) {
	mockDB := &MockDB{}
	writer := NewDefaultBatchWriter(mockDB)

	result, err := writer.WriteItemsParallel(context.Background(), []any{}, 2)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.TotalItems)
	assert.Equal(t, 0, result.ProcessedItems)
	assert.Equal(t, 0, result.FailedItems)
	assert.Empty(t, result.Errors)
}

func TestBatchWriter_WriteItemsParallel_DefaultWorkers(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}

	items := []any{
		&TestItem{ID: "1", Name: "Item 1"},
	}

	// Setup mocks
	mockDB.On("Model", mock.AnythingOfType("*batch.TestItem")).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.AnythingOfType("[]interface {}")).Return(nil)

	writer := NewDefaultBatchWriter(mockDB)
	result, err := writer.WriteItemsParallel(context.Background(), items, 0) // Should use DefaultWorkers

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, result.TotalItems)
	assert.Equal(t, 1, result.ProcessedItems)
	assert.Equal(t, 0, result.FailedItems)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchWriter_WriteItemsParallel_WithLargeDataset(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}

	items := make([]any, 100)
	for i := 0; i < 100; i++ {
		items[i] = &TestItem{ID: fmt.Sprintf("%d", i), Name: fmt.Sprintf("Item %d", i)}
	}

	// Setup mocks - expect multiple calls to BatchCreate
	mockDB.On("Model", mock.AnythingOfType("*batch.TestItem")).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.MatchedBy(func(items []any) bool {
		return len(items) <= 25 // Each batch should be <= 25
	})).Return(nil).Times(4) // 100 items / 25 per batch = 4 batches

	writer := NewDefaultBatchWriter(mockDB)
	result, err := writer.WriteItemsParallel(context.Background(), items, 3)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 100, result.TotalItems)
	assert.Equal(t, 100, result.ProcessedItems)
	assert.Equal(t, 0, result.FailedItems)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestNewBatchReader(t *testing.T) {
	tests := []struct {
		name            string
		config          BatchReaderConfig
		expectedBatch   int
		expectedLogger  bool
		expectedTracker bool
	}{
		{
			name: "default config",
			config: BatchReaderConfig{
				BatchSize: 0, // Should default to MaxBatchReadSize
			},
			expectedBatch: MaxBatchReadSize,
		},
		{
			name: "custom batch size",
			config: BatchReaderConfig{
				BatchSize: 50,
			},
			expectedBatch: 50,
		},
		{
			name: "batch size too large",
			config: BatchReaderConfig{
				BatchSize: 200, // Should be capped at MaxBatchReadSize
			},
			expectedBatch: MaxBatchReadSize,
		},
		{
			name: "with logger and tracker",
			config: BatchReaderConfig{
				BatchSize: 75,
				Logger:    zaptest.NewLogger(t),
				Tracker:   &MockCostTracker{},
			},
			expectedBatch:   75,
			expectedLogger:  true,
			expectedTracker: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := &MockDB{}
			reader := NewBatchReader(mockDB, tt.config)

			assert.Equal(t, tt.expectedBatch, reader.batchSize)
			assert.Equal(t, mockDB, reader.client)

			if tt.expectedLogger {
				assert.NotNil(t, reader.logger)
			} else {
				assert.Nil(t, reader.logger)
			}

			if tt.expectedTracker {
				assert.NotNil(t, reader.tracker)
			} else {
				assert.Nil(t, reader.tracker)
			}
		})
	}
}

func TestNewDefaultBatchReader(t *testing.T) {
	mockDB := &MockDB{}
	reader := NewDefaultBatchReader(mockDB)

	assert.Equal(t, MaxBatchReadSize, reader.batchSize)
	assert.Equal(t, mockDB, reader.client)
	assert.Nil(t, reader.logger)
	assert.Nil(t, reader.tracker)
}

func TestBatchReader_ReadItems_EmptyKeys(t *testing.T) {
	mockDB := &MockDB{}
	reader := NewDefaultBatchReader(mockDB)

	var dest []TestItem
	result, err := reader.ReadItems(context.Background(), []any{}, &dest)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.TotalKeys)
	assert.Equal(t, 0, result.RetrievedItems)
	assert.Equal(t, 0, result.NotFoundItems)
	assert.Empty(t, result.Errors)
}

func TestBatchReader_ReadItems_InvalidDest(t *testing.T) {
	mockDB := &MockDB{}
	reader := NewDefaultBatchReader(mockDB)

	keys := []any{"key1", "key2"}
	var dest TestItem // Not a pointer to slice

	result, err := reader.ReadItems(context.Background(), keys, dest)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dest must be a pointer to a slice")
	assert.Nil(t, result)
}

func TestBatchReader_ReadItems_Success(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}

	keys := []any{"key1", "key2", "key3"}

	// Setup mocks
	mockDB.On("Model", "key1").Return(mockQuery)
	mockQuery.On("BatchGet", mock.AnythingOfType("[]interface {}"), mock.AnythingOfType("*[]batch.TestItem")).Return(nil)

	reader := NewDefaultBatchReader(mockDB)
	var dest []TestItem
	result, err := reader.ReadItems(context.Background(), keys, &dest)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 3, result.TotalKeys)
	assert.Greater(t, result.Duration, time.Duration(0))

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchReader_ReadItems_WithError(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}

	keys := []any{"key1", "key2"}
	expectedError := errors.New("batch read failed")

	// Setup mocks
	mockDB.On("Model", "key1").Return(mockQuery)
	mockQuery.On("BatchGet", mock.AnythingOfType("[]interface {}"), mock.AnythingOfType("*[]batch.TestItem")).Return(expectedError)

	reader := NewDefaultBatchReader(mockDB)
	var dest []TestItem
	result, err := reader.ReadItems(context.Background(), keys, &dest)

	assert.NoError(t, err) // ReadItems doesn't return error, it collects them in result
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.TotalKeys)
	assert.Len(t, result.Errors, 2)

	// Check that all keys are marked as failed
	for i, batchErr := range result.Errors {
		assert.Equal(t, i, batchErr.Index)
		assert.Equal(t, keys[i], batchErr.Item)
		assert.Contains(t, batchErr.Error.Error(), "batch read failed")
	}

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchReader_ReadItems_WithCostTracking(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}
	tracker := &MockCostTracker{}
	logger := zaptest.NewLogger(t)

	keys := []any{"key1", "key2"}

	// Setup mocks
	mockDB.On("Model", "key1").Return(mockQuery)
	mockQuery.On("BatchGet", mock.AnythingOfType("[]interface {}"), mock.AnythingOfType("*[]batch.TestItem")).Return(nil)

	reader := NewBatchReader(mockDB, BatchReaderConfig{
		BatchSize: MaxBatchReadSize,
		Logger:    logger,
		Tracker:   tracker,
	})

	var dest []TestItem
	result, err := reader.ReadItems(context.Background(), keys, &dest)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.TotalKeys)

	// Check that cost tracking was updated
	cost := tracker.CalculateCost()
	assert.Equal(t, int64(2), cost.DynamoDBReads)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestProgressTracker(t *testing.T) {
	tracker := NewProgressTracker(100)

	// Test initial state
	total, processed, failed := tracker.GetProgress()
	assert.Equal(t, int64(100), total)
	assert.Equal(t, int64(0), processed)
	assert.Equal(t, int64(0), failed)
	assert.False(t, tracker.IsComplete())
	assert.Equal(t, 0.0, tracker.GetPercentComplete())

	// Test progress updates
	tracker.UpdateProcessed(30)
	total, processed, failed = tracker.GetProgress()
	assert.Equal(t, int64(100), total)
	assert.Equal(t, int64(30), processed)
	assert.Equal(t, int64(0), failed)
	assert.False(t, tracker.IsComplete())
	assert.Equal(t, 30.0, tracker.GetPercentComplete())

	tracker.UpdateFailed(20)
	total, processed, failed = tracker.GetProgress()
	assert.Equal(t, int64(100), total)
	assert.Equal(t, int64(30), processed)
	assert.Equal(t, int64(20), failed)
	assert.False(t, tracker.IsComplete())
	assert.Equal(t, 50.0, tracker.GetPercentComplete())

	// Test completion
	tracker.UpdateProcessed(50)
	total, processed, failed = tracker.GetProgress()
	assert.Equal(t, int64(100), total)
	assert.Equal(t, int64(80), processed)
	assert.Equal(t, int64(20), failed)
	assert.True(t, tracker.IsComplete())
	assert.Equal(t, 100.0, tracker.GetPercentComplete())
}

func TestProgressTracker_WithCallbacks(t *testing.T) {
	tracker := NewProgressTracker(100)

	var callbackCalled bool
	var callbackTotal, callbackProcessed, callbackFailed int64

	tracker.AddCallback(func(total, processed, failed int64) {
		callbackCalled = true
		callbackTotal = total
		callbackProcessed = processed
		callbackFailed = failed
	})

	tracker.UpdateProcessed(50)

	assert.True(t, callbackCalled)
	assert.Equal(t, int64(100), callbackTotal)
	assert.Equal(t, int64(50), callbackProcessed)
	assert.Equal(t, int64(0), callbackFailed)
}

func TestProgressTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewProgressTracker(1000)

	var wg sync.WaitGroup
	workers := 10
	itemsPerWorker := 100

	// Start workers that update progress concurrently
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < itemsPerWorker; j++ {
				tracker.UpdateProcessed(1)
			}
		}()
	}

	wg.Wait()

	total, processed, failed := tracker.GetProgress()
	assert.Equal(t, int64(1000), total)
	assert.Equal(t, int64(1000), processed)
	assert.Equal(t, int64(0), failed)
	assert.True(t, tracker.IsComplete())
	assert.Equal(t, 100.0, tracker.GetPercentComplete())
}

func TestBatchWriterWithProgress(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}

	items := []any{
		&TestItem{ID: "1", Name: "Item 1"},
		&TestItem{ID: "2", Name: "Item 2"},
		&TestItem{ID: "3", Name: "Item 3"},
	}

	// Setup mocks
	mockDB.On("Model", mock.AnythingOfType("*batch.TestItem")).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.AnythingOfType("[]interface {}")).Return(nil)

	writer := NewBatchWriterWithProgress(mockDB, BatchWriterConfig{
		BatchSize: DefaultBatchSize,
	}, int64(len(items)))

	result, err := writer.WriteItemsWithProgress(context.Background(), items)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 3, result.TotalItems)
	assert.Equal(t, 3, result.ProcessedItems)
	assert.Equal(t, 0, result.FailedItems)

	// Check progress tracker
	total, processed, failed := writer.GetProgressTracker().GetProgress()
	assert.Equal(t, int64(3), total)
	assert.Equal(t, int64(3), processed)
	assert.Equal(t, int64(0), failed)
	assert.True(t, writer.GetProgressTracker().IsComplete())

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestConvenienceFunctions(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}

	t.Run("BatchWrite", func(t *testing.T) {
		items := []any{
			&TestItem{ID: "1", Name: "Item 1"},
		}

		mockDB.On("Model", mock.AnythingOfType("*batch.TestItem")).Return(mockQuery)
		mockQuery.On("BatchCreate", mock.AnythingOfType("[]interface {}")).Return(nil)

		result, err := BatchWrite(context.Background(), mockDB, items)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 1, result.TotalItems)
		assert.Equal(t, 1, result.ProcessedItems)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("BatchWriteParallel", func(t *testing.T) {
		items := []any{
			&TestItem{ID: "1", Name: "Item 1"},
		}

		mockDB.On("Model", mock.AnythingOfType("*batch.TestItem")).Return(mockQuery)
		mockQuery.On("BatchCreate", mock.AnythingOfType("[]interface {}")).Return(nil)

		result, err := BatchWriteParallel(context.Background(), mockDB, items, 2)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 1, result.TotalItems)
		assert.Equal(t, 1, result.ProcessedItems)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("BatchRead", func(t *testing.T) {
		keys := []any{"key1"}

		mockDB.On("Model", "key1").Return(mockQuery)
		mockQuery.On("BatchGet", mock.AnythingOfType("[]interface {}"), mock.AnythingOfType("*[]batch.TestItem")).Return(nil)

		var dest []TestItem
		result, err := BatchRead(context.Background(), mockDB, keys, &dest)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 1, result.TotalKeys)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("BatchWriteWithCostTracking", func(t *testing.T) {
		items := []any{
			&TestItem{ID: "1", Name: "Item 1"},
		}
		tracker := &MockCostTracker{}
		logger := zaptest.NewLogger(t)

		mockDB.On("Model", mock.AnythingOfType("*batch.TestItem")).Return(mockQuery)
		mockQuery.On("BatchCreate", mock.AnythingOfType("[]interface {}")).Return(nil)

		result, err := BatchWriteWithCostTracking(context.Background(), mockDB, items, tracker, logger)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 1, result.TotalItems)
		assert.Equal(t, 1, result.ProcessedItems)

		// Check cost tracking
		cost := tracker.CalculateCost()
		assert.Equal(t, int64(1), cost.DynamoDBWrites)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("BatchReadWithCostTracking", func(t *testing.T) {
		keys := []any{"key1"}
		tracker := &MockCostTracker{}
		logger := zaptest.NewLogger(t)

		mockDB.On("Model", "key1").Return(mockQuery)
		mockQuery.On("BatchGet", mock.AnythingOfType("[]interface {}"), mock.AnythingOfType("*[]batch.TestItem")).Return(nil)

		var dest []TestItem
		result, err := BatchReadWithCostTracking(context.Background(), mockDB, keys, &dest, tracker, logger)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 1, result.TotalKeys)

		// Check cost tracking
		cost := tracker.CalculateCost()
		assert.Equal(t, int64(1), cost.DynamoDBReads)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestBatchOperations_LargeDataSets(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}

	t.Run("Large batch write", func(t *testing.T) {
		// Create 1000 items to test large dataset handling
		items := make([]any, 1000)
		for i := 0; i < 1000; i++ {
			items[i] = &TestItem{ID: fmt.Sprintf("%d", i), Name: fmt.Sprintf("Item %d", i)}
		}

		// Setup mocks - expect 40 calls to BatchCreate (1000 / 25 = 40)
		mockDB.On("Model", mock.AnythingOfType("*batch.TestItem")).Return(mockQuery)
		mockQuery.On("BatchCreate", mock.MatchedBy(func(items []any) bool {
			return len(items) <= 25 // Each batch should be <= 25
		})).Return(nil).Times(40)

		writer := NewDefaultBatchWriter(mockDB)
		result, err := writer.WriteItems(context.Background(), items)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 1000, result.TotalItems)
		assert.Equal(t, 1000, result.ProcessedItems)
		assert.Equal(t, 0, result.FailedItems)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("Large batch read", func(t *testing.T) {
		// Create 500 keys to test large dataset handling
		keys := make([]any, 500)
		for i := 0; i < 500; i++ {
			keys[i] = fmt.Sprintf("key%d", i)
		}

		// Setup mocks - expect 5 calls to BatchGet (500 / 100 = 5)
		mockDB.On("Model", mock.AnythingOfType("string")).Return(mockQuery)
		mockQuery.On("BatchGet", mock.MatchedBy(func(keys []any) bool {
			return len(keys) <= 100 // Each batch should be <= 100
		}), mock.AnythingOfType("*[]batch.TestItem")).Return(nil).Times(5)

		reader := NewDefaultBatchReader(mockDB)
		var dest []TestItem
		result, err := reader.ReadItems(context.Background(), keys, &dest)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 500, result.TotalKeys)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestBatchOperations_PartialFailures(t *testing.T) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}

	t.Run("Partial batch write failures", func(t *testing.T) {
		// Create items that will be processed in 2 batches
		items := make([]any, 30)
		for i := 0; i < 30; i++ {
			items[i] = &TestItem{ID: fmt.Sprintf("%d", i), Name: fmt.Sprintf("Item %d", i)}
		}

		// Setup mocks - first batch succeeds, second batch fails
		mockDB.On("Model", mock.AnythingOfType("*batch.TestItem")).Return(mockQuery)
		mockQuery.On("BatchCreate", mock.MatchedBy(func(items []any) bool {
			return len(items) == 25 // First batch
		})).Return(nil).Once()
		mockQuery.On("BatchCreate", mock.MatchedBy(func(items []any) bool {
			return len(items) == 5 // Second batch
		})).Return(errors.New("batch failed")).Once()

		writer := NewDefaultBatchWriter(mockDB)
		result, err := writer.WriteItems(context.Background(), items)

		assert.NoError(t, err) // WriteItems continues on partial failures
		assert.NotNil(t, result)
		assert.Equal(t, 30, result.TotalItems)
		assert.Equal(t, 25, result.ProcessedItems) // Only first batch succeeded
		assert.Equal(t, 5, result.FailedItems)     // Second batch failed
		assert.Len(t, result.Errors, 5)            // 5 items in failed batch

		// Check that failed items have correct indices
		for i, batchErr := range result.Errors {
			assert.Equal(t, 25+i, batchErr.Index) // Indices 25-29
		}

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

// Benchmark tests for performance validation
func BenchmarkBatchWriter_WriteItems(b *testing.B) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}

	// Setup mocks
	mockDB.On("Model", mock.AnythingOfType("*batch.TestItem")).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.AnythingOfType("[]interface {}")).Return(nil)

	writer := NewDefaultBatchWriter(mockDB)
	items := make([]any, 100)
	for i := 0; i < 100; i++ {
		items[i] = &TestItem{ID: fmt.Sprintf("%d", i), Name: fmt.Sprintf("Item %d", i)}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := writer.WriteItems(context.Background(), items)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBatchWriter_WriteItemsParallel(b *testing.B) {
	mockDB := &MockDB{}
	mockQuery := &MockQuery{}

	// Setup mocks
	mockDB.On("Model", mock.AnythingOfType("*batch.TestItem")).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.AnythingOfType("[]interface {}")).Return(nil)

	writer := NewDefaultBatchWriter(mockDB)
	items := make([]any, 100)
	for i := 0; i < 100; i++ {
		items[i] = &TestItem{ID: fmt.Sprintf("%d", i), Name: fmt.Sprintf("Item %d", i)}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := writer.WriteItemsParallel(context.Background(), items, 5)
		if err != nil {
			b.Fatal(err)
		}
	}
}
