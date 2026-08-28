package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_RateLimitRepository_ClearLockoutAndAPICounters(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("ClearLockout ignores missing lockouts", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete").Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		require.NoError(t, repo.ClearLockout(ctx, "agent:agent-0"))
	})

	t.Run("ClearAPIRateLimitsForUser deletes matching counters", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.APIRateLimit")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*[]models.APIRateLimit)
			*dst = []models.APIRateLimit{
				{PK: "RATELIMIT#agent:agent-0:agent_posts_10s", SK: "WINDOW#2026-03-11T15:00:00Z"},
				{PK: "RATELIMIT#agent:agent-0:agent_request_total", SK: "WINDOW#2026-03-11T15:00:00Z"},
			}
		}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
		mockQuery.On("BatchDelete", mock.MatchedBy(func(keys []struct{ PK, SK string }) bool {
			return len(keys) == 2 &&
				keys[0].PK == "RATELIMIT#agent:agent-0:agent_posts_10s" &&
				keys[1].PK == "RATELIMIT#agent:agent-0:agent_request_total"
		})).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		require.NoError(t, repo.ClearAPIRateLimitsForUser(ctx, "agent:agent-0"))
	})

	t.Run("ClearAPIRateLimitsForUser returns query errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.APIRateLimit")).Return(nil, errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		require.Error(t, repo.ClearAPIRateLimitsForUser(ctx, "agent:agent-0"))
	})
}
