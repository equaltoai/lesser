package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestRound08_RateLimitRepository_CorePaths(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("login attempts and lockouts", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)
		mockQuery.On("First", mock.AnythingOfType("*models.RateLimitLockout")).Run(func(args mock.Arguments) {
			lockout := args.Get(0).(*models.RateLimitLockout)
			lockout.PK = "RATELIMIT#user-1"
			lockout.SK = "LOCKOUT"
			lockout.UnlockTime = time.Now().Add(2 * time.Minute)
			_ = lockout.UpdateKeys()
		}).Return(nil).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.RateLimitLockout")).Run(func(args mock.Arguments) {
			lockout := args.Get(0).(*models.RateLimitLockout)
			lockout.PK = "RATELIMIT#user-1"
			lockout.SK = "LOCKOUT"
			lockout.UnlockTime = time.Now().Add(-time.Minute)
			_ = lockout.UpdateKeys()
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, mockUpdateBuilder, baseTime)
		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)

		require.NoError(t, repo.RecordLoginAttempt(ctx, "user-1", true))

		count, err := repo.GetLoginAttemptCount(ctx, "user-1", baseTime.Add(-time.Hour))
		require.NoError(t, err)
		require.GreaterOrEqual(t, count, 0)

		isLimited, until, err := repo.IsRateLimited(ctx, "user-1")
		require.NoError(t, err)
		require.True(t, isLimited)
		require.False(t, until.IsZero())

		isLimited, _, err = repo.IsRateLimited(ctx, "user-1")
		require.NoError(t, err)
		require.False(t, isLimited)

		require.NoError(t, repo.ClearLoginAttempts(ctx, "user-1"))
	})

	t.Run("community note limits", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]*models.CommunityNote")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*[]*models.CommunityNote)
			*dst = make([]*models.CommunityNote, 2)
		}).Return(nil).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.CommunityNote")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*[]*models.CommunityNote)
			*dst = make([]*models.CommunityNote, 15)
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)

		canCreate, remaining, err := repo.CheckCommunityNoteRateLimit(ctx, "user-1", 10)
		require.NoError(t, err)
		require.True(t, canCreate)
		require.GreaterOrEqual(t, remaining, 0)

		canCreate, remaining, err = repo.CheckCommunityNoteRateLimit(ctx, "user-1", 10)
		require.NoError(t, err)
		require.False(t, canCreate)
		require.Equal(t, 0, remaining)
	})

	t.Run("api rate limiting", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.APIRateLimit")).Run(func(args mock.Arguments) {
			rl := args.Get(0).(*models.APIRateLimit)
			rl.PK = "RATELIMIT#user-1:endpoint"
			rl.SK = "WINDOW#2025-01-01T00:00:00Z"
			rl.Count = 99
			rl.Window = time.Now().Truncate(time.Minute)
			rl.Blocked = true
			rl.BlockedUntil = time.Now().Add(time.Minute)
		}).Return(nil).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.APIRateLimit")).Run(func(args mock.Arguments) {
			rl := args.Get(0).(*models.APIRateLimit)
			rl.PK = "RATELIMIT#user-1:endpoint"
			rl.SK = "WINDOW#2025-01-01T00:00:00Z"
			rl.Count = 5
			rl.Window = time.Now().Truncate(time.Minute)
			rl.Blocked = false
		}).Return(nil).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.APIRateLimit")).Run(func(args mock.Arguments) {
			rl := args.Get(0).(*models.APIRateLimit)
			rl.PK = "RATELIMIT#user-1:endpoint"
			rl.SK = "WINDOW#2025-01-01T00:00:00Z"
			rl.Count = 4
			rl.Window = time.Now().Truncate(time.Minute)
			rl.Blocked = false
		}).Return(nil).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.APIRateLimit")).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)

		require.Error(t, repo.CheckAPIRateLimit(ctx, "user-1", "endpoint", 5, time.Minute))

		require.Error(t, repo.CheckAPIRateLimit(ctx, "user-1", "endpoint", 5, time.Minute))

		require.NoError(t, repo.CheckAPIRateLimit(ctx, "user-1", "endpoint", 5, time.Minute))

		remaining, reset, err := repo.GetAPIRateLimitInfo(ctx, "user-1", "endpoint", 5, time.Minute)
		require.NoError(t, err)
		require.Equal(t, 5, remaining)
		require.False(t, reset.IsZero())
	})

	t.Run("federation rate limiting", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.APIRateLimit")).Run(func(args mock.Arguments) {
			rl := args.Get(0).(*models.APIRateLimit)
			rl.PK = "RATELIMIT#DOMAIN#example.com:endpoint"
			rl.SK = "WINDOW#2025-01-01T00:00:00Z"
			rl.Count = 1
			rl.Window = time.Now().Truncate(time.Minute)
			rl.Blocked = false
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)

		require.Error(t, repo.CheckFederationRateLimit(ctx, "example.com", "endpoint", 0, time.Minute))

		remaining, reset, err := repo.GetFederationRateLimitInfo(ctx, "example.com", "endpoint", 10, time.Minute)
		require.NoError(t, err)
		require.GreaterOrEqual(t, remaining, 0)
		require.False(t, reset.IsZero())
	})

	t.Run("penalty and blocked checks", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)

		require.Equal(t, time.Minute, repo.calculatePenaltyDuration(1))
		require.Equal(t, 5*time.Minute, repo.calculatePenaltyDuration(2))
		require.Equal(t, 15*time.Minute, repo.calculatePenaltyDuration(3))
		require.Equal(t, time.Hour, repo.calculatePenaltyDuration(4))

		blocked, until, err := repo.IsUserBlocked(ctx, "user-1")
		require.NoError(t, err)
		require.True(t, blocked)
		require.False(t, until.IsZero())

		blocked, until, err = repo.IsDomainBlocked(ctx, "example.com")
		require.NoError(t, err)
		require.True(t, blocked)
		require.False(t, until.IsZero())
	})
}
