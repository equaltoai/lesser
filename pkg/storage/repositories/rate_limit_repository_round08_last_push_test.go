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

func TestRound08_RateLimitRepository_LastPush(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("updateAPIRateLimit error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(errors.New("create failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		require.Error(t, repo.updateAPIRateLimit(ctx, &models.APIRateLimit{PK: "pk", SK: "sk"}))
	})

	t.Run("IsUserBlocked returns false with no active blocks", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]*models.APIRateLimit")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*[]*models.APIRateLimit)
			*dst = []*models.APIRateLimit{
				{Blocked: false},
				{Blocked: true, BlockedUntil: time.Now().Add(-time.Minute)},
			}
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		blocked, until, err := repo.IsUserBlocked(ctx, "user-1")
		require.NoError(t, err)
		require.False(t, blocked)
		require.True(t, until.IsZero())
	})
}
