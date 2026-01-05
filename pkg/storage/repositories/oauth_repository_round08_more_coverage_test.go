package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestRound08_OAuthRepository_MoreBranches(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("DeleteOAuthClient propagates delete error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete").Return(errors.New("delete failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewOAuthRepository(mockDB, zaptest.NewLogger(t))
		require.Error(t, repo.DeleteOAuthClient(ctx, "client-1"))
	})

	t.Run("UpdateOAuthClient returns error when GetOAuthClient fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.OAuthClient")).Return(errors.New("get failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewOAuthRepository(mockDB, zaptest.NewLogger(t))
		require.Error(t, repo.UpdateOAuthClient(ctx, "client-1", map[string]any{"name": "x"}))
	})

	t.Run("UpdateOAuthClient ignores wrong types", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewOAuthRepository(mockDB, zaptest.NewLogger(t))
		require.NoError(t, repo.UpdateOAuthClient(ctx, "client-1", map[string]any{
			"name":          123,
			"description":   456,
			"redirect_uris": "not-a-slice",
			"grant_types":   "not-a-slice",
			"scopes":        "not-a-slice",
			"website":       789,
			"confidential":  "nope",
		}))
	})

	t.Run("CreateOAuthClient uses provided secret", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewOAuthRepository(mockDB, zaptest.NewLogger(t))
		client := &storage.OAuthClient{
			ClientID:     "client-1",
			ClientSecret: "pre-set",
			Name:         "App",
			RedirectURIs: []string{"https://example.com/cb"},
			OwnerID:      "user-1",
			CreatedAt:    baseTime,
			UpdatedAt:    baseTime,
		}
		require.NoError(t, repo.CreateOAuthClient(ctx, client))
		require.Equal(t, "pre-set", client.ClientSecret)
	})
}

