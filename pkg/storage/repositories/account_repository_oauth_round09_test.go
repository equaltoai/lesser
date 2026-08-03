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

func TestRound09_AccountRepository_OAuthStateCRUD(t *testing.T) {
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

	state := &storage.OAuthState{
		Provider:    "github",
		RedirectURI: "https://example.com/callback",
		Username:    "user-1",
		ExpiresAt:   time.Time{},
	}
	require.NoError(t, repo.StoreOAuthState(ctx, "state-1", state))

	got, err := repo.GetOAuthState(ctx, "state-1")
	require.NoError(t, err)
	require.Equal(t, "github", got.Provider)

	require.NoError(t, repo.DeleteOAuthState(ctx, "state-1"))

	mockDBNotFound := new(mocks.MockDB)
	mockQueryNotFound := new(mocks.MockQuery)
	mockQueryNotFound.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	setupPermissiveRound08Mocks(mockDBNotFound, mockQueryNotFound, nil, baseTime)
	repoNotFound := NewAccountRepository(mockDBNotFound, "test-table", "example.com", zap.NewNop())
	repoNotFound.SetValidationService(nil)
	repoNotFound.SetPermissionService(nil)
	repoNotFound.SetEventService(nil)
	repoNotFound.SetCachingService(nil)
	_, err = repoNotFound.GetOAuthState(ctx, "missing")
	require.Error(t, err)

	mockDBExpired := new(mocks.MockDB)
	mockQueryExpired := new(mocks.MockQuery)
	mockQueryExpired.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if target, ok := args.Get(0).(*models.OAuthState); ok {
			target.State = "expired"
			target.Provider = "github"
			target.ExpiresAt = time.Now().Add(-1 * time.Minute)
			_ = target.UpdateKeys()
		}
	}).Return(nil).Once()
	setupPermissiveRound08Mocks(mockDBExpired, mockQueryExpired, nil, baseTime)
	repoExpired := NewAccountRepository(mockDBExpired, "test-table", "example.com", zap.NewNop())
	repoExpired.SetValidationService(nil)
	repoExpired.SetPermissionService(nil)
	repoExpired.SetEventService(nil)
	repoExpired.SetCachingService(nil)
	_, err = repoExpired.GetOAuthState(ctx, "expired")
	require.Error(t, err)
}

func TestRound09_AccountRepository_OAuthClientOps(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Create").Return(errors.New("ConditionalCheckFailed")).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	ctx := context.Background()

	require.Error(t, repo.CreateOAuthClient(ctx, &storage.OAuthClient{}))
	require.Error(t, repo.CreateOAuthClient(ctx, &storage.OAuthClient{Name: "app"}))

	err := repo.CreateOAuthClient(ctx, &storage.OAuthClient{
		Name:         "app",
		RedirectURIs: []string{"https://example.com/cb"},
	})
	require.Error(t, err)

	client := &storage.OAuthClient{Name: "app", RedirectURIs: []string{"https://example.com/cb"}}
	require.NoError(t, repo.CreateOAuthClient(ctx, client))
	require.NotEmpty(t, client.ClientID)
	require.NotEmpty(t, client.ClientSecret)

	got, err := repo.GetOAuthClient(ctx, client.ClientID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Empty(t, got.PreviousClientSecretHash)
	require.True(t, got.PreviousClientSecretGraceExpiresAt.IsZero())

	require.Error(t, repo.UpdateOAuthClient(ctx, client.ClientID, map[string]any{}))
	require.NoError(t, repo.UpdateOAuthClient(ctx, client.ClientID, map[string]any{
		FieldName:         "new",
		FieldRedirectURIs: []string{"https://example.com/new"},
		"ignored":         "x",
	}))

	mockDBUpdate := new(mocks.MockDB)
	mockQueryUpdate := new(mocks.MockQuery)
	mockQueryUpdate.On("Update", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	setupPermissiveRound08Mocks(mockDBUpdate, mockQueryUpdate, nil, baseTime)
	repoUpdate := NewAccountRepository(mockDBUpdate, "test-table", "example.com", zap.NewNop())
	repoUpdate.SetValidationService(nil)
	repoUpdate.SetPermissionService(nil)
	repoUpdate.SetEventService(nil)
	repoUpdate.SetCachingService(nil)
	require.Error(t, repoUpdate.UpdateOAuthClient(ctx, client.ClientID, map[string]any{FieldWebsite: "https://example.com"}))

	mockDBDelete := new(mocks.MockDB)
	mockQueryDelete := new(mocks.MockQuery)
	mockQueryDelete.On("Delete", mock.Anything).Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDBDelete, mockQueryDelete, nil, baseTime)
	repoDelete := NewAccountRepository(mockDBDelete, "test-table", "example.com", zap.NewNop())
	repoDelete.SetValidationService(nil)
	repoDelete.SetPermissionService(nil)
	repoDelete.SetEventService(nil)
	repoDelete.SetCachingService(nil)
	require.Error(t, repoDelete.DeleteOAuthClient(ctx, "client-err"))

	app, err := repo.GetOAuthApp(ctx, client.ClientID)
	require.NoError(t, err)
	require.NotNil(t, app)
}

func TestRound09_AccountRepository_ListOAuthClientsCursorValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	_, _, err := repo.ListOAuthClients(ctx, 0, "not-a-cursor")
	require.Error(t, err)

	badPK := Utils.Pagination.EncodeCursor("WRONG_PK", "x")
	_, _, err = repo.ListOAuthClients(ctx, 10, badPK)
	require.Error(t, err)

	missingSK := Utils.Pagination.EncodeCursor("OAUTH_CLIENTS", "")
	_, _, err = repo.ListOAuthClients(ctx, 10, missingSK)
	require.Error(t, err)

	mockDBList := new(mocks.MockDB)
	mockQueryList := new(mocks.MockQuery)
	mockQueryList.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr, ok := args.Get(0).(*[]*models.OAuthClient)
		if !ok {
			return
		}
		a := &models.OAuthClient{ClientID: "client-1", Name: "a", RedirectURIs: []string{"https://example.com/cb"}}
		_ = a.UpdateKeys()
		a.OAuthClientsPK = "OAUTH_CLIENTS"
		a.OAuthClientsSK = "CLIENT#a"
		b := &models.OAuthClient{ClientID: "client-2", Name: "b", RedirectURIs: []string{"https://example.com/cb"}}
		_ = b.UpdateKeys()
		b.OAuthClientsPK = "OAUTH_CLIENTS"
		b.OAuthClientsSK = "CLIENT#b"
		*ptr = append(*ptr, a, b)
	}).Return(nil).Once()
	setupPermissiveRound08Mocks(mockDBList, mockQueryList, nil, baseTime)

	repoList := NewAccountRepository(mockDBList, "test-table", "example.com", zap.NewNop())
	repoList.SetValidationService(nil)
	repoList.SetPermissionService(nil)
	repoList.SetEventService(nil)
	repoList.SetCachingService(nil)

	clients, next, err := repoList.ListOAuthClients(ctx, 1, "")
	require.NoError(t, err)
	require.Len(t, clients, 1)
	require.NotEmpty(t, next)
}
