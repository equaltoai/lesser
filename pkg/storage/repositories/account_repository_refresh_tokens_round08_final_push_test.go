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

func TestRound08_AccountRepository_AdvancedRefreshTokens_FinalPush(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("RotateAdvancedRefreshToken create new token error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
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
		mockQuery.On("Create").Return(errors.New("create failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.RotateAdvancedRefreshToken(ctx, "old-token", "127.0.0.2")
		require.Error(t, err)
	})

	t.Run("GetAdvancedTokensByUser query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]models.AuthRefreshToken")).Return(errors.New("all failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.GetAdvancedTokensByUser(ctx, "user-1")
		require.Error(t, err)
	})

	t.Run("GetAdvancedTokensByFamily query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]models.AuthRefreshToken")).Return(errors.New("all failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		_, err := repo.GetAdvancedTokensByFamily(ctx, "family-1")
		require.Error(t, err)
	})
}

