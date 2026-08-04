package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_OAuthRepository_WrappersAndClientCRUD(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("state wrappers", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewOAuthRepository(mockDB, zaptest.NewLogger(t))

		err := repo.StoreOAuthState(ctx, "state-1", &storage.OAuthState{
			Provider:    "github",
			RedirectURI: "https://example.com/callback",
			CreatedAt:   baseTime,
		})
		require.NoError(t, err)

		_, err = repo.GetOAuthState(ctx, "state-1")
		require.NoError(t, err)

		err = repo.DeleteOAuthState(ctx, "state-1")
		require.NoError(t, err)
	})

	t.Run("authorization code wrappers", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewOAuthRepository(mockDB, zaptest.NewLogger(t))

		err := repo.CreateAuthorizationCode(ctx, &storage.AuthorizationCode{
			Code:          "code-1",
			ClientID:      "client-1",
			Username:      "user-1",
			CodeChallenge: "challenge",
			ExpiresAt:     baseTime.Add(5 * time.Minute),
			Scopes:        []string{"read"},
		})
		require.NoError(t, err)

		_, err = repo.GetAuthorizationCode(ctx, "code-1")
		require.NoError(t, err)

		err = repo.DeleteAuthorizationCode(ctx, "code-1")
		require.NoError(t, err)
	})

	t.Run("refresh token wrappers", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewOAuthRepository(mockDB, zaptest.NewLogger(t))

		err := repo.CreateRefreshToken(ctx, &storage.RefreshToken{
			Token:     "rt-1",
			ClientID:  "client-1",
			Username:  "user-1",
			ExpiresAt: baseTime.Add(30 * time.Minute),
			Scopes:    []string{"read"},
		})
		require.NoError(t, err)

		_, err = repo.GetRefreshToken(ctx, "rt-1")
		require.NoError(t, err)

		err = repo.DeleteRefreshToken(ctx, "rt-1")
		require.NoError(t, err)
	})

	t.Run("client create generates secret", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewOAuthRepository(mockDB, zaptest.NewLogger(t))

		client := &storage.OAuthClient{
			ClientID:     "client-1",
			ClientSecret: "",
			Name:         "App",
			RedirectURIs: []string{"https://example.com/cb"},
			GrantTypes:   []string{"authorization_code"},
			Scopes:       []string{"read"},
			OwnerID:      "user-1",
			Confidential: true,
			CreatedAt:    baseTime,
			UpdatedAt:    baseTime,
		}
		err := repo.CreateOAuthClient(ctx, client)
		require.NoError(t, err)
		require.NotEmpty(t, client.ClientSecret)
	})

	t.Run("client get success and not-found mapping", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
			repo := NewOAuthRepository(mockDB, zaptest.NewLogger(t))

			_, err := repo.GetOAuthClient(ctx, "client-1")
			require.NoError(t, err)
		})

		t.Run("missing", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("First", mock.AnythingOfType("*models.OAuthClient")).Return(dynamormErrors.ErrItemNotFound).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
			repo := NewOAuthRepository(mockDB, zaptest.NewLogger(t))

			_, err := repo.GetOAuthClient(ctx, "missing")
			require.Error(t, err)
		})
	})

	t.Run("update applies supported fields", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewOAuthRepository(mockDB, zaptest.NewLogger(t))

		err := repo.UpdateOAuthClient(ctx, "client-1", map[string]any{
			"name":          "New Name",
			"website":       "https://example.com",
			"redirect_uris": []string{"https://example.com/new"},
			"confidential":  false,
			"ignored":       123,
		})
		require.NoError(t, err)
	})

	t.Run("delete logs and returns nil", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewOAuthRepository(mockDB, zaptest.NewLogger(t))

		err := repo.DeleteOAuthClient(ctx, "client-1")
		require.NoError(t, err)
	})

	t.Run("delete expired tokens is a no-op", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewOAuthRepository(mockDB, zaptest.NewLogger(t))

		err := repo.DeleteExpiredTokens(ctx)
		require.NoError(t, err)
	})

	t.Run("list clients wrapper", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewOAuthRepository(mockDB, zaptest.NewLogger(t))

		clients, cursor, err := repo.ListOAuthClients(ctx, 2, "")
		require.NoError(t, err)
		require.Len(t, clients, 2)
		require.NotEmpty(t, cursor)
	})

	t.Run("consent wrappers", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewOAuthRepository(mockDB, zaptest.NewLogger(t))

		err := repo.SaveUserAppConsent(ctx, &storage.UserAppConsent{
			UserID:    "user-1",
			AppID:     "client-1",
			Scopes:    []string{"read"},
			CreatedAt: baseTime,
			UpdatedAt: baseTime,
		})
		require.NoError(t, err)

		_, err = repo.GetUserAppConsent(ctx, "user-1", "client-1", "")
		require.NoError(t, err)
	})
}

func TestRound08_OAuthRepository_Generators(t *testing.T) {
	clientID, err := generateClientID()
	require.NoError(t, err)
	require.NotEmpty(t, clientID)

	secret, err := generateClientSecret()
	require.NoError(t, err)
	require.NotEmpty(t, secret)
}
