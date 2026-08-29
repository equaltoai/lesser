package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_AccountRepository_AdvancedRefreshTokens_MoreBranches(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("CreateAdvancedRefreshToken create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(errors.New("create failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.CreateAdvancedRefreshToken(ctx, "user-1", "device", "127.0.0.1")
		require.Error(t, err)
	})

	t.Run("GetAdvancedRefreshToken get error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.AuthRefreshToken")).Return(errors.New("first failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.GetAdvancedRefreshToken(ctx, "token-0001")
		require.Error(t, err)
	})

	t.Run("RotateAdvancedRefreshToken token reuse triggers family revoke", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.AnythingOfType("*models.AuthRefreshToken")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*models.AuthRefreshToken)
			*dst = models.AuthRefreshToken{
				Token:     "token-0001",
				UserID:    "user-1",
				Family:    "family-1",
				ExpiresAt: baseTime.Add(time.Hour).Unix(),
				Revoked:   true,
			}
		}).Return(nil).Once()

		mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.AuthRefreshToken")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*[]models.AuthRefreshToken)
			*dst = []models.AuthRefreshToken{
				{Token: "token-0002", UserID: "user-1", Family: "family-1", ExpiresAt: baseTime.Add(time.Hour).Unix()},
			}
		}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		_, err := repo.RotateAdvancedRefreshToken(ctx, "token-0001", "127.0.0.1")
		require.ErrorIs(t, err, common.ErrTokenRevoked)
	})

	t.Run("RevokeAdvancedTokenFamily success path", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.AuthRefreshToken")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*[]models.AuthRefreshToken)
			*dst = []models.AuthRefreshToken{
				{Token: "token-0001", UserID: "user-1", Family: "family-1", ExpiresAt: baseTime.Add(time.Hour).Unix()},
				{Token: "token-0002", UserID: "user-1", Family: "family-1", ExpiresAt: baseTime.Add(time.Hour).Unix()},
			}
		}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		require.NoError(t, repo.RevokeAdvancedTokenFamily(ctx, "family-1", "logout"))
	})

	t.Run("UpdateAdvancedTokenLastUsed update error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.AuthRefreshToken")).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.UpdateAdvancedTokenLastUsed(ctx, "token-0001", "127.0.0.1"))
	})

	t.Run("UpdateAdvancedTokenLastUsed non-notfound get error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.AuthRefreshToken")).Return(errors.New("first failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.UpdateAdvancedTokenLastUsed(ctx, "token-0001", ""))
	})

	t.Run("UpdateAdvancedTokenLastUsed not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.AuthRefreshToken")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.ErrorIs(t, repo.UpdateAdvancedTokenLastUsed(ctx, "missing-token", ""), common.ErrTokenNotFound)
	})
}
