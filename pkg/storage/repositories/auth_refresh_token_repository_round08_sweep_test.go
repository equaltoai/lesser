package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormErrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestRound08_AuthRefreshTokenRepository_CoverageSweep(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("GetRefreshToken non-notfound error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		_, err := repo.GetRefreshToken(ctx, "t")
		require.Error(t, err)
	})

	t.Run("RotateRefreshToken surfaces transaction failures", func(t *testing.T) {
		t.Run("old token not found", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
			_, err := repo.RotateRefreshToken(ctx, "old", "127.0.0.1")
			require.Error(t, err)
		})

		t.Run("tx wrapper returns error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
				old := args.Get(0).(*models.AuthRefreshToken)
				old.Token = "old"
				old.UserID = "user-1"
				old.Family = "family-1"
				old.Generation = 1
				old.CreatedAt = baseTime.Unix()
				old.ExpiresAt = baseTime.Add(24 * time.Hour).Unix()
				old.Revoked = false
				_ = old.UpdateKeys()
			}).Return(nil).Once()

			mockDB.On("Transaction", mock.Anything).Return(errors.New("tx failed")).Once()

			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
			_, err := repo.RotateRefreshToken(ctx, "old", "127.0.0.1")
			require.Error(t, err)
		})

		t.Run("tx update and create error branches", func(t *testing.T) {
			t.Run("update fails", func(t *testing.T) {
				mockInner := new(mocks.MockDB)
				mockDB := &round08TxDB{inner: mockInner}
				mockQuery := new(mocks.MockQuery)

				mockInner.On("WithContext", mock.Anything).Return(mockInner).Maybe()
				mockInner.On("Model", mock.Anything).Return(mockQuery).Maybe()
				mockInner.On("Transaction", mock.Anything).Return(nil).Maybe()

				mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
					old := args.Get(0).(*models.AuthRefreshToken)
					old.Token = "old"
					old.UserID = "user-1"
					old.Family = "family-1"
					old.Generation = 1
					old.CreatedAt = baseTime.Unix()
					old.ExpiresAt = baseTime.Add(24 * time.Hour).Unix()
					old.Revoked = false
					_ = old.UpdateKeys()
				}).Return(nil).Once()

				mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()

				setupPermissiveRound08Mocks(mockInner, mockQuery, nil, baseTime)

				repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
				_, err := repo.RotateRefreshToken(ctx, "old", "127.0.0.1")
				require.Error(t, err)
			})

			t.Run("create fails", func(t *testing.T) {
				mockInner := new(mocks.MockDB)
				mockDB := &round08TxDB{inner: mockInner}
				mockQuery := new(mocks.MockQuery)

				mockInner.On("WithContext", mock.Anything).Return(mockInner).Maybe()
				mockInner.On("Model", mock.Anything).Return(mockQuery).Maybe()
				mockInner.On("Transaction", mock.Anything).Return(nil).Maybe()

				mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
					old := args.Get(0).(*models.AuthRefreshToken)
					old.Token = "old"
					old.UserID = "user-1"
					old.Family = "family-1"
					old.Generation = 1
					old.CreatedAt = baseTime.Unix()
					old.ExpiresAt = baseTime.Add(24 * time.Hour).Unix()
					old.Revoked = false
					_ = old.UpdateKeys()
				}).Return(nil).Once()

				mockQuery.On("Update", mock.Anything).Return(nil).Once()
				mockQuery.On("Create").Return(errors.New("create failed")).Once()

				setupPermissiveRound08Mocks(mockInner, mockQuery, nil, baseTime)

				repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
				_, err := repo.RotateRefreshToken(ctx, "old", "127.0.0.1")
				require.Error(t, err)
			})
		})
	})

	t.Run("RevokeTokenFamily and RevokeUserTokens query/tx error branches", func(t *testing.T) {
		t.Run("GetTokensByFamily query error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
			require.Error(t, repo.RevokeTokenFamily(ctx, "family-1", "reason"))
		})

		t.Run("RevokeUserTokens query error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAuthRefreshTokenRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
			require.Error(t, repo.RevokeUserTokens(ctx, "user-1", "logout"))
		})
	})
}
