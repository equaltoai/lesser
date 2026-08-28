package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound09_WebSocketSubscriptionManagerRepository_ErrorBranches(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()
	ctx := context.Background()

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWebSocketSubscriptionManagerRepository(mockDB, "test-table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)
		require.Error(t, repo.HandleConnect(ctx, "conn-1", "user-1"))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWebSocketSubscriptionManagerRepository(mockDB, "test-table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)
		require.Error(t, repo.CreateSubscription(ctx, "conn-1", "notifications", map[string]any{}))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWebSocketSubscriptionManagerRepository(mockDB, "test-table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)
		require.Error(t, repo.DeleteSubscription(ctx, "conn-1", "notifications"))
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("AllPaginated", mock.Anything).Return(nil, dynamormerrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWebSocketSubscriptionManagerRepository(mockDB, "test-table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		subs, err := repo.GetSubscriptionsForConnection(ctx, "conn-1")
		require.NoError(t, err)
		require.Empty(t, subs)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("AllPaginated", mock.Anything).Return(nil, errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWebSocketSubscriptionManagerRepository(mockDB, "test-table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		_, err := repo.GetSubscriptionsForType(ctx, "notifications")
		require.Error(t, err)
	}
}

func TestRound09_WebSocketSubscriptionManagerRepository_DisconnectCleanupErrors(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("AllPaginated", mock.Anything).Return(nil, errors.New("boom")).Once()
	mockQuery.On("Delete", mock.Anything).Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewWebSocketSubscriptionManagerRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	require.Error(t, repo.HandleDisconnect(ctx, "conn-1"))
}

func TestRound09_WebSocketSubscriptionManagerRepository_GetAllConnectionsNotFound(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("AllPaginated", mock.Anything).Return(nil, dynamormerrors.ErrItemNotFound).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewWebSocketSubscriptionManagerRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	connections, err := repo.GetAllConnections(ctx)
	require.NoError(t, err)
	require.Empty(t, connections)
}

func TestRound09_WebSocketSubscriptionManagerRepository_GetUserConnectionsNotFoundAndQueryError(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()
	ctx := context.Background()

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("AllPaginated", mock.Anything).Return(nil, dynamormerrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWebSocketSubscriptionManagerRepository(mockDB, "test-table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		connections, err := repo.GetUserConnections(ctx, "user-1")
		require.NoError(t, err)
		require.Empty(t, connections)
	}

	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("AllPaginated", mock.Anything).Return(nil, errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewWebSocketSubscriptionManagerRepository(mockDB, "test-table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetEventService(nil)
		repo.SetCachingService(nil)

		_, err := repo.GetUserConnections(ctx, "user-1")
		require.Error(t, err)
	}
}
