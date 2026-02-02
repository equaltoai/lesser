package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestAccountRepository_CreateAccount_ConflictOnExistingUser(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 11, 12, 13, 14, 15, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Create").Return(dynamormErrors.ErrConditionFailed).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	err := repo.CreateAccount(ctx, &storage.Account{
		User: &storage.User{Username: "alice"},
	})
	require.Error(t, err)
}

func TestAccountRepository_CreateActorWithRollback_RollbackFailureIsLogged(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 11, 12, 13, 14, 15, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Actor create fails.
	mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
	// User rollback delete fails (warn branch).
	mockQuery.On("Delete").Return(fmt.Errorf("delete boom")).Once()

	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	repo.SetEncryptor(testEncryptor{})

	err := repo.createActorWithRollback(ctx, &activitypub.Actor{
		PreferredUsername: "alice",
		BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice"},
	}, &models.User{Username: "alice"}, "private")
	require.Error(t, err)
}

func TestAccountRepository_GetAccount_ActorNotFoundAndErrorBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 11, 12, 13, 14, 15, 0, time.UTC)

	t.Run("actor_not_found_returns_account_without_actor", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
			_, ok := dest.(*models.Actor)
			return ok
		})).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		account, err := repo.GetAccount(ctx, "alice")
		require.NoError(t, err)
		require.NotNil(t, account)
		require.NotNil(t, account.User)
		require.Nil(t, account.Actor)
	})

	t.Run("actor_query_error_returns_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
			_, ok := dest.(*models.Actor)
			return ok
		})).Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		account, err := repo.GetAccount(ctx, "alice")
		require.Error(t, err)
		require.Nil(t, account)
	})
}

func TestAccountRepository_DeleteAccount_UserDeleteError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 11, 12, 13, 14, 15, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Delete user fails (DeleteAccount should return an error).
	mockQuery.On("Delete").Return(nil).Once()
	mockQuery.On("Delete").Return(fmt.Errorf("boom")).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	err := repo.DeleteAccount(ctx, "alice")
	require.Error(t, err)
}

func TestAccountRepository_UpdateAccount_OptimisticLockingAndExecuteError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 11, 12, 13, 14, 15, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)

	// Hydrate user with an existing version so UpdateAccount uses optimistic locking.
	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.User)
		return ok
	})).Run(func(args mock.Arguments) {
		user := args.Get(0).(*models.User)
		user.Username = "alice"
		user.Version = 2
		user.CreatedAt = baseTime
		user.UpdatedAt = baseTime
		user.Role = "user"
		_ = user.UpdateKeys()
	}).Return(nil).Once()

	mockUpdateBuilder.On("ConditionVersion", int64(2)).Return(mockUpdateBuilder).Once()
	mockUpdateBuilder.On("Execute").Return(fmt.Errorf("boom")).Once()

	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, mockUpdateBuilder, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	repo.actorRepo = nil // keep actor update path out for deterministic test

	err := repo.UpdateAccount(ctx, &storage.Account{
		User: &storage.User{Username: "alice"},
	})
	require.Error(t, err)
}
