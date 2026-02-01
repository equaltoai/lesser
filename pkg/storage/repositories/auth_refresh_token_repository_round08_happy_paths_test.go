package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_AuthRefreshTokenRepository_HappyPaths(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("RevokeTokenFamily revokes active tokens", func(t *testing.T) {
		mockInner := new(mocks.MockDB)
		mockDB := &round08TxDB{inner: mockInner}
		mockQuery := new(mocks.MockQuery)

		mockInner.On("WithContext", mock.Anything).Return(mockInner).Maybe()
		mockInner.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockInner.On("Transaction", mock.Anything).Return(nil).Maybe()

		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.AuthRefreshToken)
			*out = append(*out,
				models.AuthRefreshToken{
					Token:     "active",
					UserID:    "user-1",
					Family:    "family-1",
					CreatedAt: baseTime.Unix(),
					ExpiresAt: baseTime.Add(time.Hour).Unix(),
					Revoked:   false,
				},
				models.AuthRefreshToken{
					Token:     "already-revoked",
					UserID:    "user-1",
					Family:    "family-1",
					CreatedAt: baseTime.Unix(),
					ExpiresAt: baseTime.Add(time.Hour).Unix(),
					Revoked:   true,
				},
			)
		}).Return(nil).Once()

		setupPermissiveRound08Mocks(mockInner, mockQuery, nil, baseTime)

		repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		require.NoError(t, repo.RevokeTokenFamily(ctx, "family-1", "reason"))
	})

	t.Run("RevokeUserTokens revokes active tokens", func(t *testing.T) {
		mockInner := new(mocks.MockDB)
		mockDB := &round08TxDB{inner: mockInner}
		mockQuery := new(mocks.MockQuery)

		mockInner.On("WithContext", mock.Anything).Return(mockInner).Maybe()
		mockInner.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockInner.On("Transaction", mock.Anything).Return(nil).Maybe()

		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.AuthRefreshToken)
			*out = append(*out,
				models.AuthRefreshToken{
					Token:     "active",
					UserID:    "user-1",
					Family:    "family-1",
					CreatedAt: baseTime.Unix(),
					ExpiresAt: baseTime.Add(time.Hour).Unix(),
					Revoked:   false,
				},
				models.AuthRefreshToken{
					Token:     "already-revoked",
					UserID:    "user-1",
					Family:    "family-1",
					CreatedAt: baseTime.Unix(),
					ExpiresAt: baseTime.Add(time.Hour).Unix(),
					Revoked:   true,
				},
			)
		}).Return(nil).Once()

		setupPermissiveRound08Mocks(mockInner, mockQuery, nil, baseTime)

		repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		require.NoError(t, repo.RevokeUserTokens(ctx, "user-1", "logout"))
	})

	t.Run("GetTokensByUser filters inactive tokens", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.AuthRefreshToken)
			*out = append(*out,
				models.AuthRefreshToken{
					Token:     "active",
					UserID:    "user-1",
					Family:    "family-1",
					CreatedAt: baseTime.Unix(),
					ExpiresAt: baseTime.Add(time.Hour).Unix(),
					Revoked:   false,
				},
				models.AuthRefreshToken{
					Token:     "revoked",
					UserID:    "user-1",
					Family:    "family-1",
					CreatedAt: baseTime.Unix(),
					ExpiresAt: baseTime.Add(time.Hour).Unix(),
					Revoked:   true,
				},
				models.AuthRefreshToken{
					Token:     "expired",
					UserID:    "user-1",
					Family:    "family-1",
					CreatedAt: baseTime.Unix(),
					ExpiresAt: baseTime.Add(-time.Minute).Unix(),
					Revoked:   false,
				},
			)
		}).Return(nil).Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		tokens, err := repo.GetTokensByUser(ctx, "user-1")
		require.NoError(t, err)
		require.Len(t, tokens, 1)
		require.Equal(t, "active", tokens[0].Token)
	})
}
