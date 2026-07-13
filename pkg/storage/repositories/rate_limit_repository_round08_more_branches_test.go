package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_RateLimitRepository_MoreBranches(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("CheckCommunityNoteRateLimit error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]*models.CommunityNote")).Return(errors.New("all failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		_, _, err := repo.CheckCommunityNoteRateLimit(ctx, "user-1", 10)
		require.Error(t, err)
	})

	t.Run("IsRateLimited not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.RateLimitLockout")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		limited, until, err := repo.IsRateLimited(ctx, "user-1")
		require.NoError(t, err)
		require.False(t, limited)
		require.True(t, until.IsZero())
	})

	t.Run("ClearLoginAttempts continues on batch delete error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("BatchDelete", mock.Anything).Return(errors.New("batch delete failed")).Once()
		mockQuery.On("Delete").Return(errors.New("lockout delete failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		require.NoError(t, repo.ClearLoginAttempts(ctx, "user-1"))
	})

	t.Run("GetAPIRateLimitInfo not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.APIRateLimit")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		remaining, resetTime, err := repo.GetAPIRateLimitInfo(ctx, "user-1", "endpoint", 10, time.Minute)
		require.NoError(t, err)
		require.Equal(t, 10, remaining)
		require.False(t, resetTime.IsZero())
	})

	t.Run("CheckAPIRateLimit ignores get errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.APIRateLimit")).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		require.NoError(t, repo.CheckAPIRateLimit(ctx, "user-1", "endpoint", 10, time.Minute))
	})
}
