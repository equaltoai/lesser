package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestWalletRepository_Round09_FinalPush(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 2, 3, 4, 0, time.UTC)

	t.Run("StoreWalletCredential success hits cost tracking branches", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		// Two creates: credential + index.
		mockQuery.On("Create").Return(nil).Twice()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWalletRepository(mockDB, "test-table", zap.NewNop(), newRound09CostService())
		err := repo.StoreWalletCredential(ctx, &storage.WalletCredential{
			Username: "user-1",
			Address:  "0xAbC",
			ChainID:  1,
			Type:     "ethereum",
			ENS:      "u.eth",
			LinkedAt: baseTime,
			LastUsed: baseTime,
		})
		require.NoError(t, err)
	})

	t.Run("StoreWalletCredential first create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWalletRepository(mockDB, "test-table", zap.NewNop(), newRound09CostService())
		require.Error(t, repo.StoreWalletCredential(ctx, &storage.WalletCredential{
			Username: "user-1",
			Address:  "0xabc",
			ChainID:  1,
			Type:     "ethereum",
		}))
	})

	t.Run("StoreWalletCredential index failure + cleanup delete failure", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		// credential create ok, index create fails, cleanup delete fails.
		mockQuery.On("Create").Return(nil).Once()
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		mockQuery.On("Delete").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWalletRepository(mockDB, "test-table", zap.NewNop(), newRound09CostService())
		require.Error(t, repo.StoreWalletCredential(ctx, &storage.WalletCredential{
			Username: "user-1",
			Address:  "0xabc",
			ChainID:  1,
			Type:     "ethereum",
		}))
	})
}
