package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestRecoveryRepository_Round09_FinalPush(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 2, 3, 4, 0, time.UTC)

	t.Run("StoreTrustee create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewRecoveryRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.Error(t, repo.StoreTrustee(ctx, "user-1", &storage.TrusteeConfig{ActorID: "actor-1", AddedAt: baseTime}))
	})

	t.Run("GetActiveRecoveryRequests not found and query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewRecoveryRepository(mockDB, "test-table", zap.NewNop(), nil)
		items, err := repo.GetActiveRecoveryRequests(ctx, "user-1")
		require.NoError(t, err)
		require.Empty(t, items)

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.On("All", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewRecoveryRepository(mockDB2, "test-table", zap.NewNop(), nil)
		_, err = repo2.GetActiveRecoveryRequests(ctx, "user-1")
		require.Error(t, err)
	})

	t.Run("StoreRecoveryCode create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewRecoveryRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.Error(t, repo.StoreRecoveryCode(ctx, "user-1", &storage.RecoveryCodeItem{CodeHash: "h1", CreatedAt: baseTime, Position: 1}))
	})

	t.Run("StoreRecoveryToken create error; GetRecoveryToken other error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewRecoveryRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.Error(t, repo.StoreRecoveryToken(ctx, "key", map[string]any{"x": "y"}))

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.On("First", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewRecoveryRepository(mockDB2, "test-table", zap.NewNop(), nil)
		_, err := repo2.GetRecoveryToken(ctx, "key")
		require.Error(t, err)
	})

	t.Run("CountUnusedRecoveryCodes propagates get error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewRecoveryRepository(mockDB, "test-table", zap.NewNop(), nil)
		_, err := repo.CountUnusedRecoveryCodes(ctx, "user-1")
		require.Error(t, err)
	})
}
