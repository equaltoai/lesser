package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRound09_AccountRepository_UpdateOAuthClient_Branches(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()
	ctx := context.Background()
	clientID := "client-1"

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.NoError(t, repo.UpdateOAuthClient(ctx, clientID, map[string]any{
			FieldName:         "new-name",
			FieldWebsite:      "https://example.com",
			FieldRedirectURIs: []string{"https://example.com/cb"},
			FieldScopes:       []string{"read"},
			"ignored":         123,
		}))
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

		require.Error(t, repo.UpdateOAuthClient(ctx, "missing", map[string]any{FieldName: "x"}))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Update", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.Error(t, repo.UpdateOAuthClient(ctx, clientID, map[string]any{FieldName: "x"}))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Update", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.Error(t, repo.UpdateOAuthClient(ctx, clientID, map[string]any{FieldName: "x"}))
	}
}

func TestRound09_AccountRepository_OAuthClient_OtherBranches(t *testing.T) {
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

		require.Error(t, repo.CreateOAuthClient(ctx, &storage.OAuthClient{
			Name:         "app",
			RedirectURIs: []string{"https://example.com/cb"},
		}))
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

		_, err := repo.GetOAuthClient(ctx, "client-err")
		require.Error(t, err)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete", mock.Anything).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.NoError(t, repo.DeleteOAuthClient(ctx, "client-ok"))
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

		_, err := repo.GetOAuthApp(ctx, "missing")
		require.Error(t, err)
	}
}
