package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormErrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestAccountRepository_GetAccountByURL_SuccessAndErrors(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 10, 11, 12, 13, 14, 0, time.UTC)

	t.Run("success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Actor)
			dest.Username = "alice"
		}).Return(nil).Once()

		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, mockUpdateBuilder, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		account, err := repo.GetAccountByURL(ctx, "https://example.com/users/alice")
		require.NoError(t, err)
		require.NotNil(t, account)
		require.NotNil(t, account.User)
	})

	t.Run("notfound", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		account, err := repo.GetAccountByURL(ctx, "https://example.com/users/missing")
		require.Error(t, err)
		require.Nil(t, account)
	})

	t.Run("query_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		account, err := repo.GetAccountByURL(ctx, "https://example.com/users/alice")
		require.Error(t, err)
		require.Nil(t, account)
	})
}

func TestAccountRepository_UpdateAccountPreferences_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 10, 11, 12, 13, 14, 0, time.UTC)

	t.Run("check_existing_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.UpdateAccountPreferences(ctx, "alice", map[string]interface{}{"lang": "en"}))
	})

	t.Run("create_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.UpdateAccountPreferences(ctx, "alice", map[string]interface{}{"lang": "en"}))
	})

	t.Run("update_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(fmt.Errorf("boom")).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.Error(t, repo.UpdateAccountPreferences(ctx, "alice", map[string]interface{}{"lang": "en"}))
	})
}

func TestAccountRepository_ValidateCredentials_NoPasswordHash(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 10, 11, 12, 13, 14, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.User)
		return ok
	})).Run(func(args mock.Arguments) {
		user := args.Get(0).(*models.User)
		user.Username = "alice"
		user.PasswordHash = ""
		user.CreatedAt = baseTime
		user.UpdatedAt = baseTime
		_ = user.UpdateKeys()
	}).Return(nil).Once()

	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	account, err := repo.ValidateCredentials(ctx, "alice", "pw")
	require.Error(t, err)
	require.Nil(t, account)
}

func TestAccountRepository_UpdateUser_NotFound(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 10, 11, 12, 13, 14, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	require.Error(t, repo.UpdateUser(ctx, "alice", map[string]interface{}{"display_name": "x"}))
}

func TestAccountRepository_GetSuggestedAccounts_InvalidCursor(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 10, 11, 12, 13, 14, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	result, err := repo.GetSuggestedAccounts(ctx, "", interfaces.PaginationOptions{Limit: 1, Cursor: "invalid@@"})
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestAccountRepository_GetSuggestedAccounts_ScanError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 10, 11, 12, 13, 14, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Scan", mock.Anything).Return(fmt.Errorf("boom")).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	result, err := repo.GetSuggestedAccounts(ctx, "", interfaces.PaginationOptions{Limit: 1})
	require.Error(t, err)
	require.Nil(t, result)
}
