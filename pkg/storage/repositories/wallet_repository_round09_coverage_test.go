package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestWalletRepository_Round09_WalletChallenge_CRUD(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.
		On("First", mockAnyWalletChallenge()).
		Run(func(args mock.Arguments) {
			ch := args.Get(0).(*models.WalletChallenge)
			ch.ID = "challenge-1"
			ch.Username = "user-1"
			ch.Address = "0xabc"
			ch.ChainID = 1
			ch.Nonce = "n"
			ch.Message = "m"
			ch.IssuedAt = baseTime
			ch.ExpiresAt = baseTime.Add(time.Minute)
		}).
		Return(nil).
		Once()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewWalletRepository(mockDB, "test-table", zap.NewNop(), newRound09CostService())

	require.NoError(t, repo.StoreWalletChallenge(ctx, &storage.WalletChallenge{
		ID:        "challenge-1",
		Username:  "user-1",
		Address:   "0xabc",
		ChainID:   1,
		Nonce:     "n",
		Message:   "m",
		IssuedAt:  baseTime,
		ExpiresAt: baseTime.Add(time.Minute),
	}))

	got, err := repo.GetWalletChallenge(ctx, "challenge-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "challenge-1", got.ID)

	require.NoError(t, repo.DeleteWalletChallenge(ctx, "challenge-1"))
}

func TestWalletRepository_Round09_StoreWalletCredential_IndexFailure_CleansUp(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Credential create succeeds; index create fails.
	mockQuery.On("Create").Return(nil).Once()
	mockQuery.On("Create").Return(ErrTestMockError).Once()
	mockQuery.On("Delete").Return(nil).Maybe()

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
	require.Error(t, err)
}

func TestWalletRepository_Round09_GetWalletCredential_SuccessAndNotFound(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC)

	t.Run("success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("All", mockAnyWalletIndexSlice()).
			Run(func(args mock.Arguments) {
				indexes := args.Get(0).(*[]models.WalletIndex)
				*indexes = append(*indexes, models.WalletIndex{Username: "user-1"})
			}).
			Return(nil).
			Once()

		mockQuery.
			On("First", mockAnyWalletCredential()).
			Run(func(args mock.Arguments) {
				w := args.Get(0).(*models.WalletCredential)
				w.Username = "user-1"
				w.Address = "0xabc"
				w.ChainID = 1
				w.Type = "ethereum"
				w.ENS = "u.eth"
				w.LinkedAt = baseTime
				w.LastUsed = baseTime
			}).
			Return(nil).
			Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWalletRepository(mockDB, "test-table", zap.NewNop(), newRound09CostService())
		got, err := repo.GetWalletCredential(ctx, "ethereum", "0xAbC")
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, "user-1", got.Username)
	})

	t.Run("index empty returns not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mockAnyWalletIndexSlice()).Return(nil).Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWalletRepository(mockDB, "test-table", zap.NewNop(), nil)
		got, err := repo.GetWalletCredential(ctx, "ethereum", "0xAbC")
		require.Error(t, err)
		require.Nil(t, got)
	})
}

func TestWalletRepository_Round09_UserCredentials_DeleteAndLastUsed(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC)

	t.Run("GetUserWalletCredentials returns slice", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("All", mockAnyWalletCredentialSlice()).
			Run(func(args mock.Arguments) {
				items := args.Get(0).(*[]models.WalletCredential)
				*items = append(*items,
					models.WalletCredential{Username: "user-1", Address: "0xabc", Type: "ethereum", LinkedAt: baseTime},
					models.WalletCredential{Username: "user-1", Address: "0xdef", Type: "ethereum", LinkedAt: baseTime},
				)
			}).
			Return(nil).
			Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWalletRepository(mockDB, "test-table", zap.NewNop(), newRound09CostService())
		got, err := repo.GetUserWalletCredentials(ctx, "user-1")
		require.NoError(t, err)
		require.Len(t, got, 2)
	})

	t.Run("DeleteWalletCredential uses default type when lookup fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mockAnyWalletCredential()).Return(ErrTestMockError).Once()
		mockQuery.On("Delete").Return(nil).Maybe()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWalletRepository(mockDB, "test-table", zap.NewNop(), newRound09CostService())
		require.NoError(t, repo.DeleteWalletCredential(ctx, "user-1", "0xAbC"))
	})

	t.Run("UpdateWalletLastUsed not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mockAnyWalletCredential()).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWalletRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.Error(t, repo.UpdateWalletLastUsed(ctx, "user-1", "0xabc"))
	})
}

func mockAnyWalletChallenge() interface{} {
	return mockMatchedByType[*models.WalletChallenge]()
}

func mockAnyWalletCredential() interface{} {
	return mockMatchedByType[*models.WalletCredential]()
}

func mockAnyWalletIndexSlice() interface{} {
	return mockMatchedByType[*[]models.WalletIndex]()
}

func mockAnyWalletCredentialSlice() interface{} {
	return mockMatchedByType[*[]models.WalletCredential]()
}
