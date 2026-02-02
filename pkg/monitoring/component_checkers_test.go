package monitoring

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestDynamoDBChecker_Check(t *testing.T) {
	logger := zap.NewNop()

	t.Run("not_found_is_healthy", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "HEALTH_CHECK_TEST").Return(mockQuery)
		mockQuery.On("Limit", 1).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(storage.ErrNotFound)

		checker := &DynamoDBChecker{db: mockDB, logger: logger}
		result, err := checker.Check(context.Background(), "table")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, HealthStatusHealthy, result.Status)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("error_is_critical", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "HEALTH_CHECK_TEST").Return(mockQuery)
		mockQuery.On("Limit", 1).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(errors.New("boom"))

		checker := &DynamoDBChecker{db: mockDB, logger: logger}
		result, err := checker.Check(context.Background(), "table")
		require.Error(t, err)
		require.NotNil(t, result)
		assert.Equal(t, HealthStatusCritical, result.Status)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("high_latency_is_warning", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", "PK", "=", "HEALTH_CHECK_TEST").Return(mockQuery)
		mockQuery.On("Limit", 1).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Run(func(mock.Arguments) {
			time.Sleep(600 * time.Millisecond)
		}).Return(nil)

		checker := &DynamoDBChecker{db: mockDB, logger: logger}
		result, err := checker.Check(context.Background(), "table")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, HealthStatusWarning, result.Status)
		assert.Equal(t, ErrorHighLatency, result.Metadata["warning"])
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestLambdaChecker_Check(t *testing.T) {
	logger := zap.NewNop()

	t.Run("create_error_is_critical", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(errors.New("create failed"))

		checker := &LambdaChecker{db: mockDB, logger: logger}
		result, err := checker.Check(context.Background(), "fn")
		require.Error(t, err)
		require.NotNil(t, result)
		assert.Equal(t, HealthStatusCritical, result.Status)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("slow_create_is_warning", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Run(func(mock.Arguments) {
			time.Sleep(1100 * time.Millisecond)
		}).Return(nil)

		checker := &LambdaChecker{db: mockDB, logger: logger}
		result, err := checker.Check(context.Background(), "fn")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, HealthStatusWarning, result.Status)
		assert.Equal(t, ErrorHighLatency, result.Metadata["warning"])
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestSQSChecker_Check(t *testing.T) {
	logger := zap.NewNop()

	t.Run("create_error_is_critical", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(errors.New("create failed"))

		checker := &SQSChecker{db: mockDB, logger: logger}
		result, err := checker.Check(context.Background(), "queue")
		require.Error(t, err)
		require.NotNil(t, result)
		assert.Equal(t, HealthStatusCritical, result.Status)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("high_latency_is_warning", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Run(func(mock.Arguments) {
			time.Sleep(600 * time.Millisecond)
		}).Return(nil)

		checker := &SQSChecker{db: mockDB, logger: logger}
		result, err := checker.Check(context.Background(), "queue")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, HealthStatusWarning, result.Status)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestCustomChecker(t *testing.T) {
	t.Parallel()

	checker := NewCustomChecker("custom", func(ctx context.Context, identifier string) (*ComponentHealthResult, error) {
		return &ComponentHealthResult{
			Component: identifier,
			Type:      "custom",
			Status:    HealthStatusHealthy,
			CheckTime: time.Now(),
		}, nil
	})

	assert.Equal(t, "custom", checker.GetType())
	result, err := checker.Check(context.Background(), "id")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "id", result.Component)
}
