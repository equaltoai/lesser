package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound09_WebSocketSubscriptionManagerRepository_FlowAndFiltering(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr, ok := args.Get(0).(*[]models.WebSocketEventSubscription)
		if !ok {
			return
		}
		okSub := models.WebSocketEventSubscription{ConnectionID: "conn-1", SubscriptionType: "notifications"}
		_ = okSub.UpdateKeys()
		notSub := models.WebSocketEventSubscription{ConnectionID: "conn-1", SubscriptionType: "metadata"}
		notSub.SK = "METADATA"
		*ptr = append(*ptr, okSub, notSub)
	}).Return(nil).Twice()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewWebSocketSubscriptionManagerRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	ctx := context.Background()

	require.NoError(t, repo.HandleConnect(ctx, "conn-1", "user-1"))
	require.NoError(t, repo.HandleConnectWithPrincipal(ctx, "conn-2", "user-2", "moderator"))

	require.NoError(t, repo.CreateSubscription(ctx, "conn-1", "notifications", map[string]any{"a": 1}))
	require.NoError(t, repo.DeleteSubscription(ctx, "conn-1", "notifications"))

	subs, err := repo.GetSubscriptionsForConnection(ctx, "conn-1")
	require.NoError(t, err)
	require.Len(t, subs, 1)

	require.NoError(t, repo.CleanupSubscriptions(ctx, "conn-1"))

	require.NoError(t, repo.HandleDisconnect(ctx, "conn-1"))
}

func TestRound09_WebSocketSubscriptionManagerRepository_GetSubscriptionsForType_NotFound(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Now().UTC())

	repo := NewWebSocketSubscriptionManagerRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	got, err := repo.GetSubscriptionsForType(context.Background(), "notifications")
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestRound09_WebSocketSubscriptionManagerRepository_GetAllAndUserConnections(t *testing.T) {
	t.Parallel()

	baseTime := time.Now().UTC()
	now := time.Now()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		switch typed := args.Get(0).(type) {
		case *[]models.WebSocketConnection:
			one := models.WebSocketConnection{
				ConnectionID: "conn-1",
				UserID:       "user-1",
				Established:  baseTime,
				LastActivity: baseTime,
				State:        models.ConnectionStateConnected,
				TTL:          baseTime.Add(10 * time.Minute).Unix(),
			}
			_ = one.UpdateKeys()
			*typed = append(*typed, one)
		case *[]models.WebSocketEventConnection:
			active := models.WebSocketEventConnection{
				ConnectionID: "conn-active",
				UserID:       "user-1",
				LastSeen:     now.Add(-10 * time.Minute),
				TTL:          now.Add(10 * time.Minute).Unix(),
			}
			_ = active.UpdateKeys()
			expired := models.WebSocketEventConnection{
				ConnectionID: "conn-expired",
				UserID:       "user-1",
				LastSeen:     now.Add(-10 * time.Minute),
				TTL:          now.Add(-1 * time.Minute).Unix(),
			}
			_ = expired.UpdateKeys()
			stale := models.WebSocketEventConnection{
				ConnectionID: "conn-stale",
				UserID:       "user-1",
				LastSeen:     now.Add(-2 * time.Hour),
				TTL:          now.Add(10 * time.Minute).Unix(),
			}
			_ = stale.UpdateKeys()
			*typed = append(*typed, active, expired, stale)
		}
	}).Return(nil).Maybe()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewWebSocketSubscriptionManagerRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	ctx := context.Background()

	conns, err := repo.GetAllConnections(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, conns)

	ids, err := repo.GetUserConnections(ctx, "user-1")
	require.NoError(t, err)
	require.Equal(t, []string{"conn-active"}, ids)
}

func TestRound09_WebSocketSubscriptionManagerRepository_GetConnectionAndErrors(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Now().UTC())

	repo := NewWebSocketSubscriptionManagerRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	ctx := context.Background()

	conn, err := repo.GetConnection(ctx, "conn-1")
	require.NoError(t, err)
	require.NotNil(t, conn)

	mockDBErr := new(mocks.MockDB)
	mockQueryErr := new(mocks.MockQuery)
	mockQueryErr.On("First", mock.Anything).Return(errors.New("not found")).Once()
	setupPermissiveRound08Mocks(mockDBErr, mockQueryErr, nil, time.Now().UTC())

	repoErr := NewWebSocketSubscriptionManagerRepository(mockDBErr, "test-table", zap.NewNop(), nil)
	repoErr.SetValidationService(nil)
	repoErr.SetPermissionService(nil)
	repoErr.SetEventService(nil)
	repoErr.SetCachingService(nil)
	_, err = repoErr.GetConnection(ctx, "missing")
	require.Error(t, err)
}
