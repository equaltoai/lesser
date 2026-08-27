package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestAccountRepository_UpdateAccountPreferences_CreateNew(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 7, 8, 9, 10, 11, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
	mockQuery.On("Create").Return(nil).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	require.NoError(t, repo.UpdateAccountPreferences(ctx, "alice", map[string]interface{}{"lang": "en"}))

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_UpdateAccountPreferences_UpdateExisting(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 7, 8, 9, 10, 11, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("First", mock.Anything).Return(nil).Once()
	mockQuery.On("Update", mock.Anything).Return(nil).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	require.NoError(t, repo.UpdateAccountPreferences(ctx, "alice", map[string]interface{}{"nsfw": true}))

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_GetAccountPreferences_ParsesBooleans(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 7, 8, 9, 10, 11, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.UserPreference)
		*dest = []models.UserPreference{
			{Key: "a", Value: "true"},
			{Key: "b", Value: "false"},
			{Key: "c", Value: "hello"},
		}
	}).Return(nil).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	prefs, err := repo.GetAccountPreferences(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, true, prefs["a"])
	require.Equal(t, false, prefs["b"])
	require.Equal(t, "hello", prefs["c"])

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_CreateAccountNote_CreateAndUpdate(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 7, 8, 9, 10, 11, 0, time.UTC)

	t.Run("create", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		mockQuery.On("Create").Return(nil).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.NoError(t, repo.CreateAccountNote(ctx, &storage.AccountNote{
			Username:      "alice",
			TargetActorID: "id",
			Note:          "hi",
		}))
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("update", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		require.NoError(t, repo.CreateAccountNote(ctx, &storage.AccountNote{
			Username:      "alice",
			TargetActorID: "id",
			Note:          "updated",
		}))
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestAccountRepository_GetPasswordReset_ExpiredAndUsedBranches(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	t.Run("expired", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.PasswordReset)
			dest.Username = "alice"
			dest.Token = "t"
			dest.CreatedAt = now.Add(-2 * time.Hour)
			dest.ExpiresAt = now.Add(-1 * time.Hour)
			dest.Used = false
		}).Return(nil).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, now)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		reset, err := repo.GetPasswordReset(ctx, "t")
		require.Error(t, err)
		require.Nil(t, reset)
	})

	t.Run("used", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.PasswordReset)
			dest.Username = "alice"
			dest.Token = "t"
			dest.CreatedAt = now.Add(-2 * time.Hour)
			dest.ExpiresAt = now.Add(1 * time.Hour)
			dest.Used = true
		}).Return(nil).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, now)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		reset, err := repo.GetPasswordReset(ctx, "t")
		require.Error(t, err)
		require.Nil(t, reset)
	})
}

func TestAccountRepository_GetFieldVerification_ExpiredAndNotFound(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	t.Run("notfound", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, now)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		field, err := repo.GetFieldVerification(ctx, "alice", "website")
		require.Error(t, err)
		require.Nil(t, field)
	})

	t.Run("expired", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.FieldVerification)
			dest.Username = "alice"
			dest.FieldName = "website"
			dest.FieldValue = "https://example.com"
			dest.VerifiedAt = now.Add(-2 * time.Hour)
			dest.ExpiresAt = now.Add(-1 * time.Hour)
			dest.UpdateKeys()
		}).Return(nil).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, now)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		field, err := repo.GetFieldVerification(ctx, "alice", "website")
		require.Error(t, err)
		require.Nil(t, field)
	})
}

func TestAccountRepository_UpdateAccount_VersionProjectionZeroInitializes(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 7, 8, 9, 10, 11, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*userVersionProjection)
		return ok
	})).Run(func(args mock.Arguments) {
		proj := args.Get(0).(*userVersionProjection)
		version := 0
		proj.Value = &version
	}).Return(nil).Once()

	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, mockUpdateBuilder, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	require.NoError(t, repo.UpdateAccount(ctx, &storage.Account{User: &storage.User{Username: "alice"}}))
}

func TestAccountRepository_SearchAccounts_CursorBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 7, 8, 9, 10, 11, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	_, err := repo.SearchAccounts(ctx, "alice", interfaces.PaginationOptions{Limit: 1, Cursor: "invalid@@"})
	require.NoError(t, err)

	cursor := Utils.Pagination.EncodeCursor("USER_HANDLE_PREFIX#zz", "zzalice")
	_, err = repo.SearchAccounts(ctx, "alice", interfaces.PaginationOptions{Limit: 1, Cursor: cursor})
	require.NoError(t, err)
}

func TestAccountRepository_GetLoginHistory_CursorBranch(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 7, 8, 9, 10, 11, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	cursor := Utils.Pagination.EncodeCursor("USER#alice", "LOGIN#cursor")
	_, err := repo.GetLoginHistory(ctx, "alice", interfaces.PaginationOptions{Limit: 1, Cursor: cursor})
	require.NoError(t, err)
}
