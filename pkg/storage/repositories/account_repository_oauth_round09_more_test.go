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
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound09_AccountRepository_OAuth_OtherErrorBranches(t *testing.T) {
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

		require.Error(t, repo.StoreOAuthState(ctx, "state-err", &storage.OAuthState{Provider: "github", RedirectURI: "https://example.com/cb"}))
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

		_, err := repo.GetOAuthState(ctx, "state-err")
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

		require.Error(t, repo.DeleteOAuthState(ctx, "state-err"))
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

		require.Error(t, repo.DeleteAuthorizationCode(ctx, "code-1"))
	}
}

func TestRound09_AccountRepository_OAuth_AuthorizationCodesRefreshTokensAndConsent(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	ctx := context.Background()

	code := &storage.AuthorizationCode{
		Code:          "code-1",
		ClientID:      "client-1",
		Username:      "user-1",
		CodeChallenge: "challenge",
		ExpiresAt:     time.Now().Add(10 * time.Minute),
		Scopes:        []string{"read"},
	}
	require.NoError(t, repo.CreateAuthorizationCode(ctx, code))

	_, _ = repo.GetAuthorizationCode(ctx, "code-1")
	require.NoError(t, repo.DeleteAuthorizationCode(ctx, "code-1"))

	token := &storage.RefreshToken{
		Token:     "token-1",
		ClientID:  "client-1",
		Username:  "user-1",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Scopes:    []string{"read"},
	}
	require.NoError(t, repo.CreateRefreshToken(ctx, token))
	_, _ = repo.GetRefreshToken(ctx, "token-1")
	require.NoError(t, repo.DeleteRefreshToken(ctx, "token-1"))

	consent := &storage.UserAppConsent{
		UserID:    "user-1",
		AppID:     "client-1",
		Scopes:    []string{"read"},
		CreatedAt: time.Now(),
	}
	require.NoError(t, repo.SaveUserAppConsent(ctx, consent))
	_, _ = repo.GetUserAppConsent(ctx, consent.UserID, consent.AppID, "")
}

func TestRound09_AccountRepository_OAuth_ListOAuthClientsQueryErrorAndStartKey(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()
	ctx := context.Background()

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

		_, _, err := repo.ListOAuthClients(ctx, 10, "")
		require.Error(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			ptr, ok := args.Get(0).(*[]*models.OAuthClient)
			if !ok {
				return
			}
			a := &models.OAuthClient{ClientID: "client-1", Name: "a"}
			_ = a.UpdateKeys()
			a.OAuthClientsPK = "OAUTH_CLIENTS"
			a.OAuthClientsSK = "CLIENT#a"
			*ptr = append(*ptr, a)
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		cursor := Utils.Pagination.EncodeCursor("OAUTH_CLIENTS", "CLIENT#0")
		clients, next, err := repo.ListOAuthClients(ctx, 1, cursor)
		require.NoError(t, err)
		require.Len(t, clients, 1)
		require.Empty(t, next)
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

		_, err := repo.GetOAuthClient(ctx, "missing")
		require.Error(t, err)
	}
}
