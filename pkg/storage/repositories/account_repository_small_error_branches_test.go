package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestAccountRepository_IsAccountPinned_ErrorBranch(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 4, 5, 6, 7, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	pinned, err := repo.IsAccountPinned(ctx, "alice", "id")
	require.Error(t, err)
	require.False(t, pinned)
}

func TestAccountRepository_CreateAccountNote_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 4, 5, 6, 7, 0, time.UTC)

	t.Run("check_existing_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.CreateAccountNote(ctx, &storage.AccountNote{Username: "alice", TargetActorID: "id", Note: "x"}))
	})

	t.Run("create_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.ErrItemNotFound).Once()
		mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.CreateAccountNote(ctx, &storage.AccountNote{Username: "alice", TargetActorID: "id", Note: "x"}))
	})

	t.Run("update_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.CreateAccountNote(ctx, &storage.AccountNote{Username: "alice", TargetActorID: "id", Note: "x"}))
	})
}

func TestAccountRepository_UpdateAccount_InvalidInputsAndGetError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 4, 5, 6, 7, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	require.Error(t, repo.UpdateAccount(ctx, nil))
	require.Error(t, repo.UpdateAccount(ctx, &storage.Account{User: nil}))
	require.Error(t, repo.UpdateAccount(ctx, &storage.Account{User: &storage.User{Username: " "}}))

	// Get error
	mockDB2 := new(mocks.MockDB)
	mockQuery2 := new(mocks.MockQuery)
	mockQuery2.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
	setupPermissiveAccountRepositoryMocks(mockDB2, mockQuery2, nil, baseTime)

	repo2 := NewAccountRepository(mockDB2, "test-table", "example.com", zaptest.NewLogger(t))
	repo2.actorRepo = nil
	require.Error(t, repo2.UpdateAccount(ctx, &storage.Account{User: &storage.User{Username: "alice"}}))
}

func TestAccountRepository_ValidateCredentials_GetUserError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 4, 5, 6, 7, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("First", mock.Anything).Return(errors.ErrItemNotFound).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	account, err := repo.ValidateCredentials(ctx, "alice", "pw")
	require.Error(t, err)
	require.Nil(t, account)

	// Also exercise password update wrapper.
	require.NoError(t, repo.UpdatePassword(ctx, "alice", "new-hash"))

	// mergeActorDataForUpdate nil incoming returns existing.
	existing := &activitypub.Actor{PreferredUsername: "alice"}
	require.Equal(t, existing, repo.mergeActorDataForUpdate("alice", existing, nil))
}

func TestAccountRepository_DeleteAccountPin_And_FieldVerification_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 4, 5, 6, 7, 0, time.UTC)

	t.Run("delete_account_pin_delete_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete").Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.DeleteAccountPin(ctx, "alice", "id"))
	})

	t.Run("field_verification_query_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		field, err := repo.GetFieldVerification(ctx, "alice", "website")
		require.Error(t, err)
		require.Nil(t, field)
	})
}
