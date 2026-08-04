package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

func TestRateLimitRepository_CheckFixedWindowRateLimit_NilRepo(t *testing.T) {
	t.Parallel()

	var repo *RateLimitRepository

	allowed, remaining, resetTime, err := repo.CheckFixedWindowRateLimit(context.Background(), "user", "bucket", 10, time.Minute)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, 10, remaining)
	assert.True(t, resetTime.IsZero())
}

func TestRateLimitRepository_CheckFixedWindowRateLimit_InvalidParamsFailOpen(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	repo := &RateLimitRepository{db: mockDB, logger: zap.NewNop()}

	allowed, remaining, resetTime, err := repo.CheckFixedWindowRateLimit(context.Background(), "", "bucket", 10, time.Minute)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, 10, remaining)
	assert.True(t, resetTime.IsZero())
}

func TestRateLimitRepository_CheckFixedWindowRateLimit_AllowsUnderLimit(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdate := new(mocks.MockUpdateBuilder)
	repo := &RateLimitRepository{db: mockDB, logger: zap.NewNop()}

	ctx := context.Background()
	identifier := "user-1"
	bucket := "endpoint"
	limit := 10

	mockDB.On("Model", mock.AnythingOfType("*models.APIRateLimit")).Return(mockQuery)
	mockQuery.On("WithContext", ctx).Return(mockQuery)
	mockQuery.On("UpdateBuilder").Return(mockUpdate)

	mockUpdate.On("SetIfNotExists", "Type", nil, "APIRateLimit").Return(mockUpdate)
	mockUpdate.On("SetIfNotExists", "UserID", nil, identifier).Return(mockUpdate)
	mockUpdate.On("SetIfNotExists", "Endpoint", nil, bucket).Return(mockUpdate)
	mockUpdate.On("SetIfNotExists", "Window", nil, mock.Anything).Return(mockUpdate)
	mockUpdate.On("Set", "UpdatedAt", mock.Anything).Return(mockUpdate)
	mockUpdate.On("Set", "TTL", mock.Anything).Return(mockUpdate)
	mockUpdate.On("Increment", "Count").Return(mockUpdate)
	mockUpdate.On("ReturnValues", "ALL_NEW").Return(mockUpdate)
	mockUpdate.On("ExecuteWithResult", mock.AnythingOfType("*models.APIRateLimit")).Run(func(args mock.Arguments) {
		current := args.Get(0).(*models.APIRateLimit)
		current.Count = 1
	}).Return(nil)

	allowed, remaining, resetTime, err := repo.CheckFixedWindowRateLimit(ctx, identifier, bucket, limit, time.Minute)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, 9, remaining)
	assert.False(t, resetTime.IsZero())

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdate.AssertExpectations(t)
}

func TestRateLimitRepository_CheckFixedWindowRateLimit_BlocksOverLimit(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdate := new(mocks.MockUpdateBuilder)
	repo := &RateLimitRepository{db: mockDB, logger: zap.NewNop()}

	ctx := context.Background()
	identifier := "user-1"
	bucket := "endpoint"
	limit := 2

	mockDB.On("Model", mock.AnythingOfType("*models.APIRateLimit")).Return(mockQuery)
	mockQuery.On("WithContext", ctx).Return(mockQuery)
	mockQuery.On("UpdateBuilder").Return(mockUpdate)

	mockUpdate.On("SetIfNotExists", "Type", nil, "APIRateLimit").Return(mockUpdate)
	mockUpdate.On("SetIfNotExists", "UserID", nil, identifier).Return(mockUpdate)
	mockUpdate.On("SetIfNotExists", "Endpoint", nil, bucket).Return(mockUpdate)
	mockUpdate.On("SetIfNotExists", "Window", nil, mock.Anything).Return(mockUpdate)
	mockUpdate.On("Set", "UpdatedAt", mock.Anything).Return(mockUpdate)
	mockUpdate.On("Set", "TTL", mock.Anything).Return(mockUpdate)
	mockUpdate.On("Increment", "Count").Return(mockUpdate)
	mockUpdate.On("ReturnValues", "ALL_NEW").Return(mockUpdate)
	mockUpdate.On("ExecuteWithResult", mock.AnythingOfType("*models.APIRateLimit")).Run(func(args mock.Arguments) {
		current := args.Get(0).(*models.APIRateLimit)
		current.Count = 3
	}).Return(nil)

	allowed, remaining, resetTime, err := repo.CheckFixedWindowRateLimit(ctx, identifier, bucket, limit, time.Minute)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, 0, remaining)
	assert.False(t, resetTime.IsZero())

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdate.AssertExpectations(t)
}

func TestRateLimitRepository_CheckFixedWindowRateLimit_UpdateErrorFailsOpen(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdate := new(mocks.MockUpdateBuilder)
	repo := &RateLimitRepository{db: mockDB, logger: zap.NewNop()}

	ctx := context.Background()
	identifier := "user-1"
	bucket := "endpoint"
	limit := 10

	mockDB.On("Model", mock.AnythingOfType("*models.APIRateLimit")).Return(mockQuery)
	mockQuery.On("WithContext", ctx).Return(mockQuery)
	mockQuery.On("UpdateBuilder").Return(mockUpdate)

	mockUpdate.On("SetIfNotExists", "Type", nil, "APIRateLimit").Return(mockUpdate)
	mockUpdate.On("SetIfNotExists", "UserID", nil, identifier).Return(mockUpdate)
	mockUpdate.On("SetIfNotExists", "Endpoint", nil, bucket).Return(mockUpdate)
	mockUpdate.On("SetIfNotExists", "Window", nil, mock.Anything).Return(mockUpdate)
	mockUpdate.On("Set", "UpdatedAt", mock.Anything).Return(mockUpdate)
	mockUpdate.On("Set", "TTL", mock.Anything).Return(mockUpdate)
	mockUpdate.On("Increment", "Count").Return(mockUpdate)
	mockUpdate.On("ReturnValues", "ALL_NEW").Return(mockUpdate)
	mockUpdate.On("ExecuteWithResult", mock.AnythingOfType("*models.APIRateLimit")).Return(ErrTestMockError)

	allowed, remaining, resetTime, err := repo.CheckFixedWindowRateLimit(ctx, identifier, bucket, limit, time.Minute)
	require.Error(t, err)
	assert.True(t, allowed)
	assert.Equal(t, limit, remaining)
	assert.False(t, resetTime.IsZero())

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdate.AssertExpectations(t)
}
