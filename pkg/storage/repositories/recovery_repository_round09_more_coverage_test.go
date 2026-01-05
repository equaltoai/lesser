package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormErrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRecoveryRepository_Round09_MoreCoverage(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 1, 2, 3, 0, time.UTC)

	t.Run("UpdateRecoveryRequest delegates to StoreRecoveryRequest", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewRecoveryRepository(mockDB, "test-table", zap.NewNop(), nil)

		req := &storage.SocialRecoveryRequest{
			ID:            "r1",
			Username:      "user-1",
			InitiatedAt:   baseTime,
			ExpiresAt:     baseTime.Add(time.Hour),
			RequiredVotes: 2,
			TrusteeVotes:  []string{"t1"},
			Status:        models.StatusPending,
		}
		require.NoError(t, repo.UpdateRecoveryRequest(ctx, req))
	})

	t.Run("DeleteRecoveryRequest calls base delete", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete").Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewRecoveryRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.NoError(t, repo.DeleteRecoveryRequest(ctx, "r1"))
	})

	t.Run("MarkRecoveryCodeUsed target not found and first error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.
			On("All", mockMatchedByType[*[]models.RecoveryCode]()).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]models.RecoveryCode)
				*out = append(*out, models.RecoveryCode{Username: "user-1", Position: 1, CodeHash: "h1", CreatedAt: baseTime})
			}).
			Return(nil).
			Maybe()
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewRecoveryRepository(mockDB, "test-table", zap.NewNop(), nil)

		require.Error(t, repo.MarkRecoveryCodeUsed(ctx, "user-1", "missing"))
		require.Error(t, repo.MarkRecoveryCodeUsed(ctx, "user-1", "h1"))
	})

	t.Run("DeleteAllRecoveryCodes delete error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.
			On("All", mockMatchedByType[*[]models.RecoveryCode]()).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]models.RecoveryCode)
				*out = append(*out, models.RecoveryCode{Username: "user-1", Position: 1, CodeHash: "h1", CreatedAt: baseTime})
			}).
			Return(nil).
			Once()
		mockQuery.On("Delete").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewRecoveryRepository(mockDB, "test-table", zap.NewNop(), nil)

		require.Error(t, repo.DeleteAllRecoveryCodes(ctx, "user-1"))
	})

	t.Run("DeleteRecoveryToken delete error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewRecoveryRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.Error(t, repo.DeleteRecoveryToken(ctx, "key"))
	})
}

