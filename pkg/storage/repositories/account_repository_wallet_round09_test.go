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
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound09_AccountRepository_WalletCredentialOps(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()
	ctx := context.Background()

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.Error(t, repo.StoreWalletCredential(ctx, &storage.WalletCredential{Address: "0xabc"}))
		require.Error(t, repo.StoreWalletCredential(ctx, &storage.WalletCredential{Username: "user-1"}))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(nil).Once()
		mockQuery.On("Create").Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.NoError(t, repo.StoreWalletCredential(ctx, &storage.WalletCredential{
			Username: "user-1",
			Address:  "0xABC",
			Type:     "",
		}))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.Error(t, repo.StoreWalletCredential(ctx, &storage.WalletCredential{
			Username: "user-1",
			Address:  "0xabc",
			Type:     "ethereum",
		}))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		call := 0
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			if indexes, ok := args.Get(0).(*[]models.WalletIndex); ok {
				call++
				if call == 2 {
					*indexes = append(*indexes, models.WalletIndex{Username: "user-1"})
				}
			}
		}).Return(nil).Maybe()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		cred, err := repo.GetWalletCredential(ctx, "0xABC")
		require.NoError(t, err)
		require.NotNil(t, cred)

		_, err = repo.GetWalletByAddress(ctx, "solana", "0xABC")
		require.Error(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Maybe()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		_, err := repo.GetWalletCredential(ctx, "0xabc")
		require.Error(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			if indexes, ok := args.Get(0).(*[]models.WalletIndex); ok {
				*indexes = append(*indexes, models.WalletIndex{Username: "user-1"})
			}
		}).Return(nil).Once()
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		_, err := repo.GetWalletCredential(ctx, "0xabc")
		require.Error(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		_, err := repo.GetUserWalletCredentials(ctx, "user-1")
		require.Error(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		wallets, err := repo.GetUserWalletCredentials(ctx, "user-1")
		require.NoError(t, err)
		require.NotEmpty(t, wallets)

		users, err := repo.GetAllUsersForWallet(ctx, "ethereum", "0xabc")
		require.NoError(t, err)
		require.NotEmpty(t, users)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		users, err := repo.GetAllUsersForWallet(ctx, "ethereum", "0xabc")
		require.NoError(t, err)
		require.Empty(t, users)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		_, err := repo.GetAllUsersForWallet(ctx, "ethereum", "0xabc")
		require.Error(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.Error(t, repo.DeleteWalletCredentialByAddress(ctx, "user-1", "0xabc"))
	}
}

func TestRound09_AccountRepository_WalletChallengesAndDeletes(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()
	ctx := context.Background()

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.Error(t, repo.StoreWalletChallenge(ctx, &storage.WalletChallenge{
			ID:        "c1",
			Address:   "0xabc",
			Nonce:     "n",
			Message:   "m",
			IssuedAt:  baseTime,
			ExpiresAt: baseTime.Add(10 * time.Minute),
		}))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		_, err := repo.GetWalletChallenge(ctx, "missing")
		require.Error(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Times(4)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		_, err := repo.GetWalletChallenge(ctx, "err")
		require.Error(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			if target, ok := args.Get(0).(*models.WalletChallenge); ok {
				target.ID = "expired"
				target.Address = "0xabc"
				target.IssuedAt = baseTime.Add(-2 * time.Hour)
				target.ExpiresAt = time.Now().Add(-1 * time.Minute)
				_ = target.UpdateKeys()
			}
		}).Return(nil).Once()
		mockQuery.On("Delete", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		_, err := repo.GetWalletChallenge(ctx, "expired")
		require.Error(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		_, err := repo.GetWalletChallenge(ctx, "ok")
		require.NoError(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			if target, ok := args.Get(0).(*models.WalletChallenge); ok {
				target.ID = "registered"
				target.Username = "alice"
				target.Address = "0xabc"
				target.IssuedAt = baseTime
				target.ExpiresAt = time.Now().Add(time.Minute)
				target.RegistrationCompleted = true
				_ = target.UpdateKeys()
			}
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		challenge, err := repo.GetWalletChallenge(ctx, "registered")
		require.NoError(t, err)
		require.NotNil(t, challenge)
		require.True(t, challenge.RegistrationCompleted)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.Error(t, repo.DeleteWalletChallenge(ctx, "c1"))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.Error(t, repo.MarkWalletChallengeUsed(ctx, "c1"))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.Error(t, repo.MarkWalletChallengeRegistrationCompleted(ctx, "c1"))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.Error(t, repo.MarkWalletChallengeSpent(ctx, "c1"))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.Error(t, repo.ResetWalletChallengeSpent(ctx, "c1"))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Update", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.Error(t, repo.MarkWalletChallengeUsed(ctx, "c1"))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Update", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.Error(t, repo.MarkWalletChallengeRegistrationCompleted(ctx, "c1"))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Update", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.Error(t, repo.ResetWalletChallengeSpent(ctx, "c1"))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.NoError(t, repo.MarkWalletChallengeUsed(ctx, "c1"))
		require.NoError(t, repo.MarkWalletChallengeRegistrationCompleted(ctx, "c1"))
		require.NoError(t, repo.MarkWalletChallengeSpent(ctx, "c1"))
		require.NoError(t, repo.ResetWalletChallengeSpent(ctx, "c1"))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete", mock.Anything).Return(nil).Once()
		mockQuery.On("Delete", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		mockQuery.On("Delete", mock.Anything).Return(errors.New("boom")).Once()
		mockQuery.On("Delete", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.NoError(t, repo.DeleteWalletCredential(ctx, "user-1", "0xabc"))
	}
}
