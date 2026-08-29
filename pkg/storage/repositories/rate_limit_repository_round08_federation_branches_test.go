package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_RateLimitRepository_FederationAndLoginAttemptBranches(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("RecordLoginAttempt create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(errors.New("create failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		require.Error(t, repo.RecordLoginAttempt(ctx, "user-1", true))
	})

	t.Run("CheckFederationRateLimit blocked", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.APIRateLimit")).Run(func(args mock.Arguments) {
			rl := args.Get(0).(*models.APIRateLimit)
			rl.PK = "RATELIMIT#DOMAIN#example.com:endpoint"
			rl.SK = "WINDOW#2025-01-01T00:00:00Z"
			rl.Window = time.Now().Truncate(time.Minute)
			rl.Blocked = true
			rl.BlockedUntil = time.Now().Add(time.Minute)
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		require.Error(t, repo.CheckFederationRateLimit(ctx, "example.com", "endpoint", 10, time.Minute))
	})

	t.Run("CheckFederationRateLimit ignores get errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.RateLimitLockout")).Return(dynamormerrors.ErrItemNotFound).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.APIRateLimit")).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		require.NoError(t, repo.CheckFederationRateLimit(ctx, "example.com", "endpoint", 10, time.Minute))
	})
}

func TestRound08_RateLimitRepository_FederationPenaltyPath(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Now().UTC()

	t.Run("new window resets, increments, and applies escalating penalty", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		// No active lockout: the fast-path check clears.
		mockQuery.On("First", mock.AnythingOfType("*models.RateLimitLockout")).Return(dynamormerrors.ErrItemNotFound).Once()
		// Stale window from a previous window: forces the reset branch, then
		// Count++ pushes the record over the (zero) limit into the penalty path.
		mockQuery.On("First", mock.AnythingOfType("*models.APIRateLimit")).Run(func(args mock.Arguments) {
			rl := args.Get(0).(*models.APIRateLimit)
			rl.Domain = "example.com"
			rl.Endpoint = "endpoint"
			rl.Window = time.Now().Add(-48 * time.Hour)
			rl.Count = 0
			rl.Blocked = false
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		err := repo.CheckFederationRateLimit(ctx, "example.com", "endpoint", 0, time.Minute)
		require.Error(t, err)
	})
}
