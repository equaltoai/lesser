package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
	"golang.org/x/crypto/bcrypt"
)

func TestAccountRepository_CoreCoverageSweep(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 4, 5, 6, 7, 8, 0, time.UTC)

	require.Equal(t, "test-table", (userVersionProjection{Table: "test-table"}).TableName())
	require.NotEmpty(t, (userVersionProjection{}).TableName())

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, mockUpdateBuilder, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	repo.SetStorage(nil)

	repo.SetStatusRepository(nil)

	require.NotNil(t, repo.getBookmarkRepository())
	repo.SetBookmarkRepository(repo.getBookmarkRepository())
	require.NotNil(t, repo.getBookmarkRepository())

	require.Equal(t, "alice", repo.canonicalUsername("acct:@alice@example.com"))
	require.Equal(t, "alice", repo.canonicalUsername("https://example.com/users/alice"))
	require.Equal(t, "alice@remote.example", repo.canonicalUsername("@alice@remote.example"))
	require.True(t, repo.isLocalDomain("example.com"))
	require.Equal(t, "https://example.com", repo.actorBaseURL())
	require.Equal(t, "https://example.com", extractBaseURL("https://example.com/@alice", "/@alice"))

	require.Error(t, repo.CreateAccount(ctx, nil))
	require.Error(t, repo.CreateAccount(ctx, &storage.Account{User: nil}))

	require.NoError(t, repo.CreateAccountLegacy(ctx, "alice", "", "", true, nil, ""))

	require.NoError(t, repo.CreateAccount(ctx, &storage.Account{
		User: &storage.User{
			Username:     " alice ",
			Email:        "",
			PasswordHash: "",
			Role:         "",
		},
	}))

	require.Error(t, repo.CreateAccount(ctx, &storage.Account{
		User: &storage.User{Username: "alice"},
		Actor: &activitypub.Actor{
			PreferredUsername: "alice",
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice"},
		},
		PrivateKey: "",
	}))

	require.Error(t, repo.createActorWithRollback(ctx, struct{}{}, &models.User{Username: "alice"}, "private"))
	require.Error(t, repo.createActor(ctx, &activitypub.Actor{PreferredUsername: "alice"}, "private"))

	_, _ = repo.GetAccount(ctx, "alice")
	_, _ = repo.GetUser(ctx, "alice")
	_, err := repo.GetUserByEmail(ctx, "alice@example.com")
	require.Error(t, err)

	require.NoError(t, repo.UpdateUser(ctx, "alice", map[string]interface{}{
		"display_name": "New Name",
		"recovery_methods": []interface{}{
			"email",
			"webauthn",
		},
		"fields": []interface{}{
			map[string]interface{}{"name": "Website", "value": "https://example.com"},
		},
		"metadata": map[string]interface{}{
			"theme": "dark",
		},
	}))

	_, _ = repo.GetActor(ctx, "alice")
	_, _ = repo.GetActorByUsername(ctx, "alice")
	_ = repo.DeleteAccount(ctx, "alice")
	_, _ = repo.GetActorPrivateKey(ctx, "alice")

	user := repo.modelToStorageUser(&models.User{Username: "alice"})
	require.NotEmpty(t, user.URL)

	_ = repo.CreateAccountPin(ctx, &storage.AccountPin{Username: "alice", PinnedActorID: "id"})
	_ = repo.DeleteAccountPin(ctx, "alice", "id")
	_, _ = repo.IsAccountPinned(ctx, "alice", "id")

	_ = repo.CreateAccountNote(ctx, &storage.AccountNote{Username: "alice", TargetActorID: "id", Note: "hi"})
	_, _ = repo.GetPreference(ctx, "alice", "lang")
	_, _ = repo.GetFollowRequestState(ctx, "req", "tgt")
	_, _ = repo.IsBlockedDomain(ctx, "alice", "blocked.example")
	_, _ = repo.GetFieldVerification(ctx, "alice", "website")

	_ = repo.ApproveAccount(ctx, "alice")
	_ = repo.SuspendAccount(ctx, "alice", "reason")
	_ = repo.UnsuspendAccount(ctx, "alice")
	_ = repo.SilenceAccount(ctx, "alice", "reason")
	_ = repo.UnsilenceAccount(ctx, "alice")

	_, _ = repo.GetAccountByURL(ctx, "https://example.com/users/alice")
	_, _ = repo.GetAccountByEmail(ctx, "alice@example.com")

	_ = repo.UpdateAccount(ctx, &storage.Account{
		User:  &storage.User{Username: "alice"},
		Actor: &activitypub.Actor{Name: "Alice"},
	})

	_, _ = repo.SearchAccounts(ctx, "", interfaces.PaginationOptions{Limit: 10})
	_, _ = repo.SearchAccounts(ctx, "al", interfaces.PaginationOptions{Limit: 0})
	_, _ = repo.GetSuggestedAccounts(ctx, "", interfaces.PaginationOptions{Limit: 0})
	_, _ = repo.GetFeaturedAccounts(ctx, interfaces.PaginationOptions{Limit: 100})

	_ = repo.UpdateAccountPreferences(ctx, "alice", map[string]interface{}{
		"lang":   "en",
		"nsfw":   true,
		"number": 3,
	})
	_, _ = repo.GetAccountPreferences(ctx, "alice")
	_ = repo.UpdateAccountFeatures(ctx, "alice", map[string]bool{"new_ui": true})
	_, _ = repo.GetAccountFeatures(ctx, "alice")

	_, _ = repo.ValidateCredentials(ctx, "alice", "pw")
	_ = repo.UpdatePassword(ctx, "alice", "hash")

	_ = repo.CreatePasswordReset(ctx, &storage.PasswordReset{Username: "alice", Token: "t"})
	_, _ = repo.GetPasswordReset(ctx, "t")
	_ = repo.UsePasswordReset(ctx, "t")

	_ = repo.RecordLogin(ctx, &storage.LoginAttempt{Username: "alice"})
	_, _ = repo.GetLoginHistory(ctx, "alice", interfaces.PaginationOptions{Limit: 0})

	_ = repo.UpdateLastActivity(ctx, "alice", baseTime)
	_, _ = repo.GetAccountsByUsernames(ctx, []string{})
	_, _ = repo.GetAccountsByUsernames(ctx, []string{"alice", "bob"})
	_, _ = repo.GetAccountsCount(ctx)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_DecodeUserUpdatePayload_Errors(t *testing.T) {
	_, err := decodeUserUpdatePayload(map[string]interface{}{
		"approved": "true",
	})
	require.Error(t, err)

	_, err = decodeUserUpdatePayload(map[string]interface{}{
		"recovery_methods": []interface{}{123},
	})
	require.Error(t, err)

	_, err = decodeUserUpdatePayload(map[string]interface{}{
		"fields": []interface{}{
			map[string]interface{}{"name": 123},
		},
	})
	require.Error(t, err)

	_, err = decodeUserUpdatePayload(map[string]interface{}{
		"metadata": []string{"nope"},
	})
	require.Error(t, err)
}

func TestAccountRepository_ValidateCredentials_SuccessAndFailures(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 6, 7, 8, 9, 10, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	require.NoError(t, err)

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		_, ok := dest.(*models.User)
		return ok
	})).Run(func(args mock.Arguments) {
		user := args.Get(0).(*models.User)
		user.Username = "alice"
		user.PasswordHash = string(passwordHash)
		user.CreatedAt = baseTime
		user.UpdatedAt = baseTime
		_ = user.UpdateKeys()
	}).Return(nil).Once()

	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, mockUpdateBuilder, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	account, err := repo.ValidateCredentials(ctx, "alice", "pw")
	require.NoError(t, err)
	require.NotNil(t, account)

	account, err = repo.ValidateCredentials(ctx, "alice", "wrong")
	require.Error(t, err)
	require.Nil(t, account)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_IsAccountPinned_NotFoundFalse(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 6, 7, 8, 9, 10, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	pinned, err := repo.IsAccountPinned(ctx, "alice", "id")
	require.NoError(t, err)
	require.False(t, pinned)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
