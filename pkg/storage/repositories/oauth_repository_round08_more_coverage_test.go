package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
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

	t.Run("RotateOAuthClientSecret updates stored rotation state", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewOAuthRepository(mockDB, zaptest.NewLogger(t))
		err := repo.RotateOAuthClientSecret(ctx, "client-1", storage.OAuthClientSecretRotation{
			ActiveClientSecretHash:             "hash-new",
			PreviousClientSecretHash:           "hash-old",
			PreviousClientSecretGraceExpiresAt: baseTime.Add(24 * time.Hour),
			RotatedAt:                          baseTime,
			RotatedBy:                          "owner",
		})
		require.NoError(t, err)
	})

	t.Run("RotateOAuthClientSecret returns not found when client lookup misses", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.OAuthClient")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewOAuthRepository(mockDB, zaptest.NewLogger(t))
		err := repo.RotateOAuthClientSecret(ctx, "missing", storage.OAuthClientSecretRotation{
			ActiveClientSecretHash: "hash-new",
		})
		require.Error(t, err)
		require.ErrorIs(t, err, storage.ErrNotFound)
	})
}
