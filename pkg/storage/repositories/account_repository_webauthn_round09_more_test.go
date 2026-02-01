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
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound09_AccountRepository_WebAuthn_ErrorBranches(t *testing.T) {
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

		require.Error(t, repo.CreateWebAuthnCredential(ctx, &storage.WebAuthnCredential{
			ID:        "cred-1",
			UserID:    "user-1",
			PublicKey: []byte("pk"),
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

		_, err := repo.GetWebAuthnCredential(ctx, "missing")
		require.Error(t, err)
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

		_, err := repo.GetWebAuthnCredential(ctx, "err")
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

		_, err := repo.GetUserWebAuthnCredentials(ctx, "user-1")
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

		require.Error(t, repo.DeleteWebAuthnCredential(ctx, "cred-1"))
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

		require.Error(t, repo.UpdateWebAuthnLastUsed(ctx, "cred-1", 1))
	}

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

		require.Error(t, repo.CreateWebAuthnChallenge(ctx, &storage.WebAuthnChallenge{
			Challenge:   "c1",
			UserID:      "user-1",
			SessionData: "not-bytes",
			Type:        "authentication",
			ExpiresAt:   time.Now().Add(10 * time.Minute),
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

		_, err := repo.GetWebAuthnChallenge(ctx, "missing")
		require.Error(t, err)
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

		_, err := repo.GetWebAuthnChallenge(ctx, "err")
		require.Error(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			if target, ok := args.Get(0).(*models.WebAuthnChallenge); ok {
				target.Challenge = "expired-clean"
				target.UserID = "user-1"
				target.Type = "authentication"
				target.SessionData = []byte("s")
				target.ExpiresAt = time.Now().Add(-1 * time.Minute)
			}
		}).Return(nil).Once()
		mockQuery.On("Delete", mock.Anything).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		_, err := repo.GetWebAuthnChallenge(ctx, "expired-clean")
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

		require.Error(t, repo.DeleteWebAuthnChallenge(ctx, "c1"))
	}
}
