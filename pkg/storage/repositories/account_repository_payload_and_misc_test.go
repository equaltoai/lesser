package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestAccountRepository_UserUpdatePayload_DecodeAndApply_CoversHelpers(t *testing.T) {
	updates := map[string]interface{}{
		"email":                "",
		"note":                 "bio",
		"avatar":               "a",
		"header":               "h",
		"url":                  "https://example.com/@alice",
		"password_hash":        "hash",
		"display_name":         "Alice",
		"approved":             true,
		AccountStatusSuspended: true,
		"silenced":             true,
		"role":                 "admin",
		"locked":               true,
		"discoverable":         true,
		"locale":               "en",
		"allow_nsfw":           true,
		"require_nsfw_warning": false,
		"recovery_methods":     []string{"webauthn", "wallet"},
		"fields":               []map[string]string{{"name": "Website", "value": "https://example.com"}},
		"metadata":             map[string]interface{}{"theme": "dark"},
	}

	payload, err := decodeUserUpdatePayload(updates)
	require.NoError(t, err)
	require.NotNil(t, payload)

	user := &models.User{Username: "alice"}
	payload.applyTo(user)

	require.Equal(t, "bio", user.Note)
	require.Equal(t, "a", user.Avatar)
	require.Equal(t, "h", user.Header)
	require.Equal(t, "https://example.com/@alice", user.URL)
	require.Equal(t, "hash", user.PasswordHash)
	require.Equal(t, "Alice", user.DisplayName)
	require.True(t, user.Approved)
	require.True(t, user.Suspended)
	require.True(t, user.Silenced)
	require.Equal(t, "admin", user.Role)
	require.True(t, user.Locked)
	require.True(t, user.Discoverable)
	require.Equal(t, "en", user.Locale)
	require.True(t, user.AllowNSFW)
	require.False(t, user.RequireNSFWWarning)
	require.Equal(t, []string{"webauthn", "wallet"}, user.RecoveryMethods)
	require.Len(t, user.Fields, 1)
	require.Equal(t, "dark", user.Metadata["theme"])

	// Nil updates map paths.
	emptyPayload, err := decodeUserUpdatePayload(nil)
	require.NoError(t, err)
	require.NotNil(t, emptyPayload)
}

func TestAccountRepository_UserUpdatePayload_Decode_ArrayForms(t *testing.T) {
	updates := map[string]interface{}{
		"recovery_methods": []interface{}{"email", "wallet"},
		"fields": []interface{}{
			map[string]interface{}{"name": "Website", "value": "https://example.com"},
			map[string]string{"name": "X", "value": "Y"},
		},
		"metadata": map[string]interface{}{
			"k": "v",
		},
	}

	payload, err := decodeUserUpdatePayload(updates)
	require.NoError(t, err)
	require.NotNil(t, payload)

	user := &models.User{Username: "alice"}
	payload.applyTo(user)
	require.Equal(t, []string{"email", "wallet"}, user.RecoveryMethods)
	require.Len(t, user.Fields, 2)
	require.Equal(t, "v", user.Metadata["k"])
}

func TestAccountRepository_GetFieldVerification_Success(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.FieldVerification)
		dest.Username = "alice"
		dest.FieldName = "website"
		dest.FieldValue = "https://example.com"
		dest.VerifiedAt = now.Add(-1 * time.Hour)
		dest.ExpiresAt = now.Add(1 * time.Hour)
		dest.UpdateKeys()
	}).Return(nil).Once()

	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, now)
	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	field, err := repo.GetFieldVerification(ctx, "alice", "website")
	require.NoError(t, err)
	require.NotNil(t, field)
	require.Equal(t, "website", field.Name)
}

func TestAccountRepository_PasswordReset_HappyAndUpdateError(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("get_password_reset_ok", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.PasswordReset)
			dest.Username = "alice"
			dest.Token = "t"
			dest.CreatedAt = now.Add(-1 * time.Hour)
			dest.ExpiresAt = now.Add(1 * time.Hour)
			dest.Used = false
		}).Return(nil).Once()

		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, now)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		reset, err := repo.GetPasswordReset(ctx, "t")
		require.NoError(t, err)
		require.NotNil(t, reset)
	})

	t.Run("use_password_reset_update_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.PasswordReset)
			dest.Username = "alice"
			dest.Token = "t"
			dest.ExpiresAt = now.Add(1 * time.Hour)
			dest.Used = false
		}).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(fmt.Errorf("boom")).Once()

		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, now)
		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

		require.Error(t, repo.UsePasswordReset(ctx, "t"))
	})
}

func TestAccountRepository_CreatePasswordReset_CreateError(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2025, 12, 5, 2, 3, 4, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, now)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	require.Error(t, repo.CreatePasswordReset(ctx, &storage.PasswordReset{Username: "alice", Token: "t"}))
}

func TestAccountRepository_RecordLogin_CreateError(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2025, 12, 3, 2, 3, 4, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, now)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	require.Error(t, repo.RecordLogin(ctx, &storage.LoginAttempt{Username: "alice"}))
}

func TestAccountRepository_GetAccountsCount_Error(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2025, 12, 3, 2, 3, 4, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("AllPaginated", mock.Anything).Return(nil, fmt.Errorf("boom")).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, now)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	_, err := repo.GetAccountsCount(ctx)
	require.Error(t, err)
}

func TestAccountRepository_GetAccountsByUsernames_SkipsNotFound(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2025, 12, 3, 2, 3, 4, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*userCoreProjection)
		return ok
	})).Run(func(args mock.Arguments) {
		user := args.Get(0).(*userCoreProjection)
		user.Table = "test-table"
		user.PK = "USER#alice"
		user.SK = models.SKMetadata
		user.Username = "alice"
		user.CreatedAt = now
		user.UpdatedAt = now
		user.Role = "user"
		user.Approved = true
		user.Version = 1
	}).Return(nil).Once()

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*userCoreProjection)
		return ok
	})).Return(dynamormErrors.ErrItemNotFound).Twice()

	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, now)
	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	accounts, err := repo.GetAccountsByUsernames(ctx, []string{"alice", "bob"})
	require.NoError(t, err)
	require.Len(t, accounts, 1)
}
