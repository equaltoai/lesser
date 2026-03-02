package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_RateLimitRepository_FinalPush(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("GetLoginAttemptCount query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]*models.LoginAttempt")).Return(errors.New("all failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		_, err := repo.GetLoginAttemptCount(ctx, "user-1", baseTime.Add(-time.Hour))
		require.Error(t, err)
	})

	t.Run("GetViolationCount query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]*models.RateLimitViolation")).Return(errors.New("all failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		_, err := repo.GetViolationCount(ctx, "user-1", "", time.Hour)
		require.Error(t, err)
	})

	t.Run("ClearLoginAttempts query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]*models.LoginAttempt")).Return(errors.New("all failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		require.Error(t, repo.ClearLoginAttempts(ctx, "user-1"))
	})

	t.Run("CheckAPIRateLimit ignores update failures", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.APIRateLimit")).Run(func(args mock.Arguments) {
			rl := args.Get(0).(*models.APIRateLimit)
			rl.Count = 0
			rl.Window = time.Now().Truncate(time.Minute)
			rl.Blocked = false
		}).Return(nil).Once()
		mockQuery.On("Create").Return(errors.New("create failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		require.NoError(t, repo.CheckAPIRateLimit(ctx, "user-1", "endpoint", 10, time.Minute))
	})

	t.Run("IsDomainBlocked query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.RateLimitLockout")).Return(errors.New("get failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		_, _, err := repo.IsDomainBlocked(ctx, "example.com")
		require.Error(t, err)
	})
}
