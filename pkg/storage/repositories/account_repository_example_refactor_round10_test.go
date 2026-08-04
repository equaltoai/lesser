package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound10_AccountRepositoryExampleRefactor_GetUserRefactored(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Now().UTC()

	t.Run("success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		user, err := repo.GetUserRefactored(ctx, "user-1")
		require.NoError(t, err)
		require.NotNil(t, user)
	})

	t.Run("error is mapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("boom"))

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		_, err := repo.GetUserRefactored(ctx, "user-1")
		require.Error(t, err)
	})
}

func TestRound10_AccountRepositoryExampleRefactor_GetUserByEmailAndStoreOAuthState(t *testing.T) {
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

	user, err := repo.GetUserByEmailRefactored(ctx, "User@Example.com")
	require.NoError(t, err)
	require.NotNil(t, user)

	state := &storage.OAuthState{
		Provider:    "github",
		RedirectURI: "https://example.com/cb",
		Username:    "user-1",
		ClientID:    "client-1",
		Scopes:      []string{"read"},
		CreatedAt:   baseTime,
	}
	require.NoError(t, repo.StoreOAuthStateRefactored(ctx, "state-1", state))
	require.False(t, state.ExpiresAt.IsZero())
}

func TestRound10_AccountRepositoryExampleRefactor_CreateFollowRefactored(t *testing.T) {
	ctx := context.Background()

	t.Run("validation errors", func(t *testing.T) {
		repo := &AccountRepository{}
		err := repo.CreateFollowRefactored(ctx, "", "bob")
		require.Error(t, err)
		var ve common.ValidationError
		require.ErrorAs(t, err, &ve)
	})

	t.Run("not found is mapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		err := repo.CreateFollowRefactored(ctx, "alice", "bob")
		require.Error(t, err)
	})

	t.Run("success creates follow record", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Now().UTC())

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		require.NoError(t, repo.CreateFollowRefactored(ctx, "alice", "bob"))
	})
}
