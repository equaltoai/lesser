package cost

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

const (
	testRequestID     = "test-request-123"
	testOperationType = "test-operation"
	testOperation     = "test_operation"
	testTableName     = "test_table"
)

// MockDB is a mock implementation of core.DB for testing
type MockDB struct {
	mock.Mock
}

func (m *MockDB) Model(model any) core.Query {
	args := m.Called(model)
	return args.Get(0).(core.Query)
}

func (m *MockDB) Migrate() error {
	args := m.Called()
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

func (m *MockDB) WithContext(ctx context.Context) core.DB {
	args := m.Called(ctx)
	return args.Get(0).(core.DB)
}

func TestNewDynamORMCostTracker(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()

	tracker := NewDynamORMCostTracker(mockDB, logger)

	assert.NotNil(t, tracker)
	assert.NotNil(t, tracker.Tracker)
	assert.Equal(t, mockDB, tracker.client)
	assert.Equal(t, logger, tracker.logger)
}

func TestNewDynamORMCostTrackerWithRequest(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()
	requestID := testRequestID
	operationType := testOperationType

	tracker := NewDynamORMCostTrackerWithRequest(mockDB, requestID, operationType, logger)

	assert.NotNil(t, tracker)
	assert.NotNil(t, tracker.Tracker)
	assert.Equal(t, mockDB, tracker.client)
	assert.Equal(t, logger, tracker.logger)
}

func TestTrackOperation_Success(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()
	tracker := NewDynamORMCostTracker(mockDB, logger)

	ctx := context.Background()
	operation := testOperation
	executed := false

	err := tracker.TrackOperation(ctx, operation, func() error {
		executed = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, executed)
}

func TestTrackOperation_Error(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()
	tracker := NewDynamORMCostTracker(mockDB, logger)

	ctx := context.Background()
	operation := testOperation
	expectedError := errors.New("operation failed")

	err := tracker.TrackOperation(ctx, operation, func() error {
		return expectedError
	})

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func TestTrackOperation_WithContextTracker(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()
	tracker := NewDynamORMCostTracker(mockDB, logger)

	// Create context with tracker
	contextTracker := New()
	ctx := WithTracker(context.Background(), contextTracker)

	operation := testOperation

	err := tracker.TrackOperation(ctx, operation, func() error {
		// Simulate some DynamoDB operations
		_ = tracker.TrackDynamoRead(2)
		_ = tracker.TrackDynamoWrite(1)
		return nil
	})

	assert.NoError(t, err)

	// Verify that costs were tracked in both trackers
	assert.Equal(t, int64(2), tracker.dynamoReads.Load())
	assert.Equal(t, int64(1), tracker.dynamoWrites.Load())

	// Context tracker should also have the costs
	assert.Equal(t, int64(2), contextTracker.dynamoReads.Load())
	assert.Equal(t, int64(1), contextTracker.dynamoWrites.Load())
}

func TestTrackPut(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()
	tracker := NewDynamORMCostTracker(mockDB, logger)

	ctx := context.Background()
	tableName := testTableName

	err := tracker.TrackPut(ctx, tableName, func() error {
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, int64(1), tracker.dynamoWrites.Load())
}

func TestTrackUpdate(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()
	tracker := NewDynamORMCostTracker(mockDB, logger)

	ctx := context.Background()
	tableName := testTableName

	err := tracker.TrackUpdate(ctx, tableName, func() error {
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, int64(1), tracker.dynamoWrites.Load())
}

func TestTrackDelete(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()
	tracker := NewDynamORMCostTracker(mockDB, logger)

	ctx := context.Background()
	tableName := testTableName

	err := tracker.TrackDelete(ctx, tableName, func() error {
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, int64(1), tracker.dynamoWrites.Load())
}

func TestTrackBatchWrite(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()
	tracker := NewDynamORMCostTracker(mockDB, logger)

	ctx := context.Background()
	tableName := testTableName
	itemCount := 5

	err := tracker.TrackBatchWrite(ctx, tableName, itemCount, func() error {
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, int64(itemCount), tracker.dynamoWrites.Load())
}

func TestTrackTransaction(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()
	tracker := NewDynamORMCostTracker(mockDB, logger)

	ctx := context.Background()
	operationCount := 3

	err := tracker.TrackTransaction(ctx, operationCount, func() error {
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, int64(operationCount), tracker.dynamoWrites.Load())
}

func TestGetCostSummary(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()
	tracker := NewDynamORMCostTracker(mockDB, logger)

	// Add some tracked operations - use small numbers to avoid circuit breaker
	_ = tracker.TrackDynamoRead(30) // Small number to avoid circuit breaker
	_ = tracker.TrackDynamoWrite(5) // Small number to avoid circuit breaker

	summary := tracker.GetCostSummary()

	assert.NotNil(t, summary)
	assert.Equal(t, int64(30), summary.DynamoDBReads)
	assert.Equal(t, int64(5), summary.DynamoDBWrites)
	// With small numbers, cost may be 0 due to integer division, which is fine
	assert.True(t, summary.TotalCostMicroCents >= 0)
}

func TestReset(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()
	tracker := NewDynamORMCostTracker(mockDB, logger)

	// Add some tracked operations
	_ = tracker.TrackDynamoRead(5)
	_ = tracker.TrackDynamoWrite(3)

	// Verify operations were tracked
	assert.Equal(t, int64(5), tracker.dynamoReads.Load())
	assert.Equal(t, int64(3), tracker.dynamoWrites.Load())

	// Reset the tracker
	tracker.Reset()

	// Verify counters were reset
	assert.Equal(t, int64(0), tracker.dynamoReads.Load())
	assert.Equal(t, int64(0), tracker.dynamoWrites.Load())
}

func TestWrapWithCostTracking(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()

	tracker := WrapWithCostTracking(mockDB, logger)

	assert.NotNil(t, tracker)
	assert.Equal(t, mockDB, tracker.client)
	assert.Equal(t, logger, tracker.logger)
}

func TestWrapWithCostTrackingAndRequest(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()
	requestID := testRequestID
	operationType := testOperationType

	tracker := WrapWithCostTrackingAndRequest(mockDB, requestID, operationType, logger)

	assert.NotNil(t, tracker)
	assert.Equal(t, mockDB, tracker.client)
	assert.Equal(t, logger, tracker.logger)
}

func TestTrackDynamORMOperation_WithTracker(t *testing.T) {
	tracker := New()
	ctx := WithTracker(context.Background(), tracker)

	err := TrackDynamORMOperation(ctx, "put", func() error {
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, int64(1), tracker.dynamoWrites.Load())
}

func TestTrackDynamORMOperation_WithoutTracker(t *testing.T) {
	ctx := context.Background()
	executed := false

	err := TrackDynamORMOperation(ctx, "put", func() error {
		executed = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, executed)
}

func TestTrackDynamORMOperation_QueryOperation(t *testing.T) {
	tracker := New()
	ctx := WithTracker(context.Background(), tracker)

	err := TrackDynamORMOperation(ctx, "query", func() error {
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, int64(1), tracker.dynamoReads.Load())
	assert.Equal(t, int64(0), tracker.dynamoWrites.Load())
}

func TestTrackDynamORMOperation_WriteOperation(t *testing.T) {
	tracker := New()
	ctx := WithTracker(context.Background(), tracker)

	err := TrackDynamORMOperation(ctx, "update", func() error {
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, int64(0), tracker.dynamoReads.Load())
	assert.Equal(t, int64(1), tracker.dynamoWrites.Load())
}

func TestWithDynamORMCostTracking(t *testing.T) {
	requestID := testRequestID
	operationType := testOperationType

	ctx := WithDynamORMCostTracking(context.Background(), requestID, operationType)

	tracker := FromContext(ctx)
	assert.NotNil(t, tracker)
}

func TestCostCalculationAccuracy(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()
	tracker := NewDynamORMCostTracker(mockDB, logger)

	// Simulate realistic DynamoDB operations - use small numbers to avoid circuit breaker
	reads := 30
	writes := 5

	_ = tracker.TrackDynamoRead(reads)
	_ = tracker.TrackDynamoWrite(writes)

	cost := tracker.GetCostSummary()

	// Verify cost calculation
	expectedReadCost := (int64(reads) * DynamoDBReadRequestUnit) / 1000000
	expectedWriteCost := (int64(writes) * DynamoDBWriteRequestUnit) / 1000000
	expectedTotal := expectedReadCost + expectedWriteCost

	assert.Equal(t, int64(reads), cost.DynamoDBReads)
	assert.Equal(t, int64(writes), cost.DynamoDBWrites)
	assert.Equal(t, expectedTotal, cost.TotalCostMicroCents)
}

func TestConcurrentOperations(t *testing.T) {
	mockDB := &MockDB{}
	logger := zap.NewNop()
	tracker := NewDynamORMCostTracker(mockDB, logger)

	ctx := context.Background()

	// Run concurrent operations
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()

			err := tracker.TrackPut(ctx, testTableName, func() error {
				time.Sleep(1 * time.Millisecond) // Simulate work
				return nil
			})
			assert.NoError(t, err)
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all operations were tracked
	assert.Equal(t, int64(10), tracker.dynamoWrites.Load())
}

func TestNewTrackingDB(t *testing.T) {
	mockDB := &MockDB{}
	tracker := New()
	logger := zap.NewNop()

	trackingDB := NewTrackingDB(mockDB, tracker, logger)

	assert.NotNil(t, trackingDB)
	assert.Equal(t, mockDB, trackingDB.DB)
	assert.Equal(t, tracker, trackingDB.tracker)
	assert.Equal(t, logger, trackingDB.logger)
}

func TestTrackQuery(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewDynamORMCostTracker(nil, logger)
	tracker.circuitBreaker = nil

	ctx := context.Background()
	called := false

	err := tracker.TrackQuery(ctx, "test-table", func() error {
		called = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called)
}

func TestGetClient(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewDynamORMCostTracker(nil, logger)

	client := tracker.GetClient()
	assert.Nil(t, client) // We passed nil
}

func TestTrackingDB_GetTracker(t *testing.T) {
	baseTracker := New()
	logger := zap.NewNop()
	trackingDB := NewTrackingDB(nil, baseTracker, logger)

	result := trackingDB.GetTracker()
	assert.Equal(t, baseTracker, result)
}
