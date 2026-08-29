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

func TestRound08_AccountRepository_Social_FinalPush(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("GetFollowing query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]models.Follow")).Return(errors.New("all failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, _, err := repo.GetFollowing(ctx, "alice", 10, "")
		require.Error(t, err)
	})

	t.Run("GetPinnedAccounts skips actor lookup failures", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.AccountPin")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*[]models.AccountPin)
			*dst = []models.AccountPin{
				{Username: "alice", PinnedActorID: "https://example.com/users/bob", PinnedUsername: "bob", CreatedAt: baseTime},
			}
		}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.Actor")).Return(errors.New("actor failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		actors, err := repo.GetPinnedAccounts(ctx, "alice")
		require.NoError(t, err)
		require.Empty(t, actors)
	})

	t.Run("GetAccountPins error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.AccountPin")).Return(nil, errors.New("all failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.GetAccountPins(ctx, "alice")
		require.Error(t, err)
	})

	t.Run("GetAccountPin non-notfound error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.AccountPin")).Return(errors.New("first failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.GetAccountPin(ctx, "alice", "https://example.com/users/bob")
		require.Error(t, err)
	})

	t.Run("UnpinAccount ignores not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete").Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.NoError(t, repo.UnpinAccount(ctx, "alice", "bob"))
	})
}
