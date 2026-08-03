package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestWalletRepository_Round09_MoreCoverage(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 1, 2, 3, 0, time.UTC)

	t.Run("StoreWalletChallenge create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWalletRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.Error(t, repo.StoreWalletChallenge(ctx, &storage.WalletChallenge{ID: "c1", Username: "u", Address: "0x", ChainID: 1}))
	})

	t.Run("GetWalletChallenge not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWalletRepository(mockDB, "test-table", zap.NewNop(), nil)
		got, err := repo.GetWalletChallenge(ctx, "missing")
		require.Error(t, err)
		require.Nil(t, got)
	})

	t.Run("GetWalletCredential query error and missing username", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewWalletRepository(mockDB, "test-table", zap.NewNop(), nil)
		_, err := repo.GetWalletCredential(ctx, "ethereum", "0xabc")
		require.Error(t, err)

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.
			On("All", mock.Anything).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]models.WalletIndex)
				*out = append(*out, models.WalletIndex{Username: ""})
			}).
			Return(nil).
			Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewWalletRepository(mockDB2, "test-table", zap.NewNop(), nil)
		_, err = repo2.GetWalletCredential(ctx, "ethereum", "0xabc")
		require.Error(t, err)
	})

	t.Run("GetWalletCredential not found and other error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("All", mock.Anything).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]models.WalletIndex)
				*out = append(*out, models.WalletIndex{Username: "user-1"})
			}).
			Return(nil).
			Once()
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewWalletRepository(mockDB, "test-table", zap.NewNop(), nil)

		_, err := repo.GetWalletCredential(ctx, "ethereum", "0xabc")
		require.Error(t, err)

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.
			On("All", mock.Anything).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]models.WalletIndex)
				*out = append(*out, models.WalletIndex{Username: "user-1"})
			}).
			Return(nil).
			Once()
		mockQuery2.On("First", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewWalletRepository(mockDB2, "test-table", zap.NewNop(), nil)
		_, err = repo2.GetWalletCredential(ctx, "ethereum", "0xabc")
		require.Error(t, err)
	})

	t.Run("GetUserWalletCredentials query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewWalletRepository(mockDB, "test-table", zap.NewNop(), nil)
		_, err := repo.GetUserWalletCredentials(ctx, "user-1")
		require.Error(t, err)
	})

	t.Run("DeleteWalletCredential delete error and index delete error tolerated", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("First", mock.Anything).
			Run(func(args mock.Arguments) {
				w := args.Get(0).(*models.WalletCredential)
				w.Type = "ethereum"
			}).
			Return(nil).
			Once()
		mockQuery.On("Delete").Return(ErrTestMockError).Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewWalletRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.Error(t, repo.DeleteWalletCredential(ctx, "user-1", "0xabc"))

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.
			On("First", mock.Anything).
			Run(func(args mock.Arguments) {
				w := args.Get(0).(*models.WalletCredential)
				w.Type = "ethereum"
			}).
			Return(nil).
			Once()
		mockQuery2.On("Delete").Return(nil).Once()              // wallet credential delete
		mockQuery2.On("Delete").Return(ErrTestMockError).Once() // index delete
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewWalletRepository(mockDB2, "test-table", zap.NewNop(), newRound09CostService())
		require.NoError(t, repo2.DeleteWalletCredential(ctx, "user-1", "0xabc"))
	})

	t.Run("UpdateWalletLastUsed success and update error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWalletRepository(mockDB, "test-table", zap.NewNop(), newRound09CostService())
		require.NoError(t, repo.UpdateWalletLastUsed(ctx, "user-1", "0xabc"))

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.On("First", mock.Anything).Return(nil).Once()
		mockQuery2.On("Update", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewWalletRepository(mockDB2, "test-table", zap.NewNop(), nil)
		require.Error(t, repo2.UpdateWalletLastUsed(ctx, "user-1", "0xabc"))
	})
}
