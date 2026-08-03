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
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_AccountRepository_AdvancedRefreshTokens(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("create and get variants", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.AuthRefreshToken")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*models.AuthRefreshToken)
			*dst = models.AuthRefreshToken{
				Token:     "token-ok",
				UserID:    "user-1",
				Family:    "family",
				ExpiresAt: baseTime.Add(time.Hour).Unix(),
				Revoked:   false,
			}
		}).Return(nil).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.AuthRefreshToken")).Return(dynamormErrors.ErrItemNotFound).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.AuthRefreshToken")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*models.AuthRefreshToken)
			*dst = models.AuthRefreshToken{
				Token:     "expired-01",
				UserID:    "user-1",
				Family:    "family",
				ExpiresAt: baseTime.Add(-time.Minute).Unix(),
				Revoked:   false,
			}
		}).Return(nil).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.AuthRefreshToken")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*models.AuthRefreshToken)
			*dst = models.AuthRefreshToken{
				Token:     "revoked-01",
				UserID:    "user-1",
				Family:    "family",
				ExpiresAt: baseTime.Add(time.Hour).Unix(),
				Revoked:   true,
			}
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		token, err := repo.CreateAdvancedRefreshToken(ctx, "user-1", "device", "127.0.0.1")
		require.NoError(t, err)
		require.NotEmpty(t, token.Token)
		require.NotEmpty(t, token.Family)
		require.False(t, token.Revoked)

		_, err = repo.GetAdvancedRefreshToken(ctx, token.Token)
		require.NoError(t, err)

		_, err = repo.GetAdvancedRefreshToken(ctx, "missing")
		require.ErrorIs(t, err, common.ErrTokenNotFound)
		_, err = repo.GetAdvancedRefreshToken(ctx, "expired-01")
		require.ErrorIs(t, err, common.ErrTokenExpired)
		_, err = repo.GetAdvancedRefreshToken(ctx, "revoked-01")
		require.ErrorIs(t, err, common.ErrTokenRevoked)
	})

	t.Run("rotate token and revoke helper", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Twice()
		mockQuery.On("First", mock.AnythingOfType("*models.AuthRefreshToken")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*models.AuthRefreshToken)
			*dst = models.AuthRefreshToken{
				Token:      "old-token",
				UserID:     "user-1",
				Family:     "family-1",
				Generation: 1,
				ExpiresAt:  baseTime.Add(time.Hour).Unix(),
				Revoked:    false,
				DeviceName: "device",
				IPAddress:  "127.0.0.1",
			}
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		newToken, err := repo.RotateAdvancedRefreshToken(ctx, "old-token", "127.0.0.2")
		require.NoError(t, err)
		require.NotEmpty(t, newToken.Token)
		require.Equal(t, "family-1", newToken.Family)
		require.Equal(t, 2, newToken.Generation)

		err = repo.revokeAdvancedRefreshToken(ctx, &models.AuthRefreshToken{Token: "token-0001", ExpiresAt: baseTime.Add(time.Hour).Unix()}, "reason")
		require.Error(t, err)
	})

	t.Run("family and user revocation", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]models.AuthRefreshToken")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*[]models.AuthRefreshToken)
			*dst = []models.AuthRefreshToken{
				{Token: "token-0001", UserID: "user-1", Family: "family-1", ExpiresAt: baseTime.Add(time.Hour).Unix()},
				{Token: "token-0002", UserID: "user-1", Family: "family-1", ExpiresAt: baseTime.Add(time.Hour).Unix(), Revoked: true},
			}
		}).Return(nil).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]models.AuthRefreshToken")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*[]models.AuthRefreshToken)
			*dst = []models.AuthRefreshToken{
				{Token: "token-0003", UserID: "user-1", Family: "family-1", ExpiresAt: baseTime.Add(time.Hour).Unix()},
				{Token: "token-0004", UserID: "user-1", Family: "family-2", ExpiresAt: baseTime.Add(time.Hour).Unix()},
			}
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		err := repo.RevokeAdvancedTokenFamily(ctx, "family-1", "reason")
		require.Error(t, err)

		err = repo.RevokeAdvancedUserTokens(ctx, "user-1", "logout")
		require.NoError(t, err)
	})

	t.Run("active filtering, last-used update, cleanup, stats", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]models.AuthRefreshToken")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*[]models.AuthRefreshToken)
			nowUnix := time.Now().Unix()
			*dst = []models.AuthRefreshToken{
				{Token: "token-active-01", UserID: "user-1", Family: "family-1", ExpiresAt: nowUnix + 3600, Revoked: false, LastUsedAt: nowUnix - 5},
				{Token: "token-revoked", UserID: "user-1", Family: "family-1", ExpiresAt: nowUnix + 3600, Revoked: true, LastUsedAt: nowUnix - 10},
				{Token: "token-expired", UserID: "user-1", Family: "family-2", ExpiresAt: nowUnix - 10, Revoked: false, LastUsedAt: nowUnix - 1},
			}
		}).Return(nil).Maybe()
		mockQuery.On("First", mock.AnythingOfType("*models.AuthRefreshToken")).Return(dynamormErrors.ErrItemNotFound).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.AuthRefreshToken")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*models.AuthRefreshToken)
			*dst = models.AuthRefreshToken{
				Token:     "update",
				UserID:    "user-1",
				Family:    "family-1",
				ExpiresAt: time.Now().Add(time.Hour).Unix(),
				Revoked:   false,
			}
		}).Return(nil).Once()
		mockQuery.On("Delete").Return(errors.New("delete failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		active, err := repo.GetActiveAdvancedTokensForUser(ctx, "user-1")
		require.NoError(t, err)
		require.Len(t, active, 1)

		err = repo.UpdateAdvancedTokenLastUsed(ctx, "missing", "")
		require.ErrorIs(t, err, common.ErrTokenNotFound)

		err = repo.UpdateAdvancedTokenLastUsed(ctx, "update", "127.0.0.1")
		require.NoError(t, err)

		mockQuery.On("All", mock.AnythingOfType("*[]models.AuthRefreshToken")).Run(func(args mock.Arguments) {
			dst := args.Get(0).(*[]models.AuthRefreshToken)
			nowUnix := time.Now().Unix()
			*dst = []models.AuthRefreshToken{
				{Token: "token-expired", UserID: "user-1", Family: "family-1", ExpiresAt: nowUnix - 10},
				{Token: "token-active-02", UserID: "user-1", Family: "family-1", ExpiresAt: nowUnix + 3600},
			}
		}).Return(nil).Once()
		deleted, err := repo.CleanupExpiredAdvancedTokens(ctx)
		require.NoError(t, err)
		require.Equal(t, 0, deleted)

		stats, err := repo.GetAdvancedTokenStats(ctx, "user-1")
		require.NoError(t, err)
		require.Equal(t, "user-1", stats.UserID)
		require.Greater(t, stats.TotalTokens, 0)
	})
}
