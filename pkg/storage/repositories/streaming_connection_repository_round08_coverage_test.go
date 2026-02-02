package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestStreamingConnectionRepository_Round08_CreateAndUpdateLifecycle(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Unix(0, 0).UTC()

	t.Run("WriteConnection succeeds when counts are under limits", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		setupPermissiveRound08Mocks(mockSubDB, mockSubQuery, nil, baseTime)

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)
		repo.subscriptionRepo.SetValidationService(nil)
		repo.subscriptionRepo.SetPermissionService(nil)
		repo.subscriptionRepo.SetCachingService(nil)
		repo.subscriptionRepo.SetEventService(nil)

		conn, err := repo.WriteConnection(ctx, "c1", "u1", "alice", []string{"stream1"})
		require.NoError(t, err)
		require.NotNil(t, conn)
		assert.Equal(t, "c1", conn.ConnectionID)
		assert.Equal(t, models.ConnectionStateConnecting, conn.State)
	})

	t.Run("WriteConnection rejects when user connection count reaches limit", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "USER#u1").Return(mockQuery).Once()
		mockQuery.On("Count").Return(int64(MaxConnectionsPerUser), nil).Once()

		setupPermissiveRound08Mocks(mockSubDB, mockSubQuery, nil, baseTime)

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		_, err := repo.WriteConnection(ctx, "c1", "u1", "alice", nil)
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("UpdateConnectionState loads connection and updates with reason", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)

		// GetConnection (BaseRepository.Get) uses First(result) and should populate fields.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.AnythingOfType("*models.WebSocketConnection")).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.WebSocketConnection)
			*dest = models.WebSocketConnection{
				ConnectionID:   "c1",
				UserID:         "u1",
				Username:       "alice",
				State:          models.ConnectionStateConnected,
				Established:    baseTime,
				LastActivity:   baseTime,
				RateLimit:      100,
				RateLimitReset: baseTime.Add(time.Hour),
				MaxMessageSize: 1024,
			}
			_ = dest.UpdateKeys()
		}).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()

		setupPermissiveRound08Mocks(mockSubDB, mockSubQuery, nil, baseTime)

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		err := repo.UpdateConnectionState(ctx, "c1", models.ConnectionStateClosing, "done")
		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestStreamingConnectionRepository_Round08_SubscriptionsAndQueries(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Unix(0, 0).UTC()

	t.Run("WriteSubscription maps create errors", func(t *testing.T) {
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)

		mockSubDB.On("WithContext", mock.Anything).Return(mockSubDB).Once()
		mockSubDB.On("Model", mock.Anything).Return(mockSubQuery).Once()
		mockSubQuery.On("Create").Return(assert.AnError).Once()

		repo := NewStreamingConnectionRepository(nil, "table", mockSubDB, "subs", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)
		repo.subscriptionRepo.SetValidationService(nil)
		repo.subscriptionRepo.SetPermissionService(nil)
		repo.subscriptionRepo.SetCachingService(nil)
		repo.subscriptionRepo.SetEventService(nil)

		err := repo.WriteSubscription(ctx, "c1", "u1", "stream1")
		require.Error(t, err)

		mockSubDB.AssertExpectations(t)
		mockSubQuery.AssertExpectations(t)
	})

	t.Run("DeleteAllSubscriptions not found is ignored", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		setupPermissiveRound08Mocks(mockSubDB, mockSubQuery, nil, baseTime)
		mockSubQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
		err := repo.DeleteAllSubscriptions(ctx, "c1")
		require.NoError(t, err)
	})

	t.Run("GetSubscriptionsForStream paginates once and converts to values", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		mockSubDB.On("WithContext", mock.Anything).Return(mockSubDB).Once()
		mockSubDB.On("Model", mock.Anything).Return(mockSubQuery).Once()
		mockSubQuery.On("Where", "PK", "=", "SUB#stream1").Return(mockSubQuery).Once()
		mockSubQuery.On("Limit", 101).Return(mockSubQuery).Once()
		mockSubQuery.On("OrderBy", "SK", SortOrderAsc).Return(mockSubQuery).Once()
		mockSubQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.WebSocketSubscription)
			sub := &models.WebSocketSubscription{ConnectionID: "c1", UserID: "u1", Stream: "stream1", SubscribedAt: baseTime}
			_ = sub.UpdateKeys()
			*dest = []*models.WebSocketSubscription{sub}
		}).Once()

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
		repo.subscriptionRepo.SetValidationService(nil)
		repo.subscriptionRepo.SetPermissionService(nil)
		repo.subscriptionRepo.SetCachingService(nil)
		repo.subscriptionRepo.SetEventService(nil)

		subs, err := repo.GetSubscriptionsForStream(ctx, "stream1")
		require.NoError(t, err)
		require.Len(t, subs, 1)
		assert.Equal(t, "stream1", subs[0].Stream)
	})

	t.Run("GetConnectionsByUser treats missing index as empty set", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)

		setupPermissiveRound08Mocks(mockSubDB, mockSubQuery, nil, baseTime)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "USER#u1").Return(mockQuery).Once()
		mockQuery.On("Limit", connectionQueryLimit).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(errors.New("requested resource not found: index not found")).Once()

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
		items, err := repo.GetConnectionsByUser(ctx, "u1")
		require.NoError(t, err)
		assert.Empty(t, items)
	})
}

func TestStreamingConnectionRepository_Round08_ScansAndResourceLimits(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Unix(0, 0).UTC()

	t.Run("GetIdleConnections filters scan results", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)

		setupPermissiveRound08Mocks(mockSubDB, mockSubQuery, nil, baseTime)

		threshold := baseTime.Add(10 * time.Minute)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Scan", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.WebSocketConnection)
			*dest = []models.WebSocketConnection{
				{ConnectionID: "c1", LastActivity: baseTime.Add(-time.Hour)},
				{ConnectionID: "c2", LastActivity: baseTime.Add(20 * time.Minute)},
			}
		}).Once()

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
		idle, err := repo.GetIdleConnections(ctx, threshold)
		require.NoError(t, err)
		require.Len(t, idle, 1)
		assert.Equal(t, "c1", idle[0].ConnectionID)
	})

	t.Run("GetStaleConnections filters by last activity and ttl", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)

		setupPermissiveRound08Mocks(mockSubDB, mockSubQuery, nil, baseTime)

		staleThreshold := baseTime.Add(-time.Minute)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Scan", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.WebSocketConnection)
			*dest = []models.WebSocketConnection{
				{ConnectionID: "c1", LastActivity: baseTime.Add(-time.Hour), TTL: 0},
				{ConnectionID: "c2", LastActivity: baseTime.Add(time.Hour), TTL: baseTime.Add(-time.Second).Unix()},
				{ConnectionID: "c3", LastActivity: baseTime.Add(time.Hour), TTL: baseTime.Add(time.Hour).Unix()},
			}
		}).Once()

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
		stale, err := repo.GetStaleConnections(ctx, staleThreshold)
		require.NoError(t, err)
		require.Len(t, stale, 2)
	})

	t.Run("EnforceResourceLimits checks size and rate limits", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockSubDB, mockSubQuery, nil, baseTime)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Times(3)
		mockDB.On("Model", mock.Anything).Return(mockQuery).Times(3)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

		// 1) message size exceeded
		mockQuery.On("First", mock.AnythingOfType("*models.WebSocketConnection")).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.WebSocketConnection)
			*dest = models.WebSocketConnection{
				ConnectionID:   "c1",
				UserID:         "u1",
				State:          models.ConnectionStateConnected,
				Established:    baseTime,
				LastActivity:   baseTime,
				MaxMessageSize: 10,
				RateLimit:      1,
				RateLimitReset: baseTime.Add(time.Hour),
			}
			_ = dest.UpdateKeys()
		}).Once()

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
		err := repo.EnforceResourceLimits(ctx, "c1", 100)
		require.Error(t, err)

		// 2) rate limit exceeded
		mockQuery.On("First", mock.AnythingOfType("*models.WebSocketConnection")).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.WebSocketConnection)
			*dest = models.WebSocketConnection{
				ConnectionID:   "c2",
				UserID:         "u1",
				State:          models.ConnectionStateConnected,
				Established:    baseTime,
				LastActivity:   baseTime,
				MaxMessageSize: 1000,
				RateLimit:      1,
				CurrentRate:    1,
				RateLimitReset: time.Now().Add(time.Minute),
			}
			_ = dest.UpdateKeys()
		}).Once()
		err = repo.EnforceResourceLimits(ctx, "c2", 10)
		require.Error(t, err)

		// 3) reset window makes request allowed
		mockQuery.On("First", mock.AnythingOfType("*models.WebSocketConnection")).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.WebSocketConnection)
			*dest = models.WebSocketConnection{
				ConnectionID:   "c3",
				UserID:         "u1",
				State:          models.ConnectionStateConnected,
				Established:    baseTime,
				LastActivity:   baseTime,
				MaxMessageSize: 1000,
				RateLimit:      1,
				CurrentRate:    0,
				RateLimitReset: time.Now().Add(-time.Minute),
			}
			_ = dest.UpdateKeys()
		}).Once()
		err = repo.EnforceResourceLimits(ctx, "c3", 10)
		require.NoError(t, err)
	})
}

func TestStreamingConnectionRepository_Round08_StateAndHealthViews(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Unix(0, 0).UTC()

	t.Run("GetConnectionsByState not found returns empty slice", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)

		setupPermissiveRound08Mocks(mockSubDB, mockSubQuery, nil, baseTime)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi2PK", "=", "STATE#idle").Return(mockQuery).Once()
		mockQuery.On("Limit", connectionQueryLimit).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
		items, err := repo.GetConnectionsByState(ctx, models.ConnectionStateIdle)
		require.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("GetHealthyConnections and GetUnhealthyConnections split by IsHealthy", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		queryConnected := new(mocks.MockQuery)
		queryIdle := new(mocks.MockQuery)
		queryError := new(mocks.MockQuery)
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)

		setupPermissiveRound08Mocks(mockSubDB, mockSubQuery, nil, baseTime)

		// Connected/idle/error calls across healthy + unhealthy views.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(queryConnected).Once()
		mockDB.On("Model", mock.Anything).Return(queryIdle).Once()
		mockDB.On("Model", mock.Anything).Return(queryError).Once()
		mockDB.On("Model", mock.Anything).Return(queryConnected).Once()
		mockDB.On("Model", mock.Anything).Return(queryIdle).Once()

		queryConnected.On("Index", "gsi2").Return(queryConnected).Twice()
		queryConnected.On("Where", "gsi2PK", "=", "STATE#connected").Return(queryConnected).Twice()
		queryConnected.On("Limit", connectionQueryLimit).Return(queryConnected).Twice()
		queryConnected.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.WebSocketConnection)
			*dest = []models.WebSocketConnection{
				{ConnectionID: "c1", State: models.ConnectionStateConnected, Metrics: models.ConnectionMetrics{ErrorCount: 0}},
				{ConnectionID: "c2", State: models.ConnectionStateConnected, Metrics: models.ConnectionMetrics{ErrorCount: 15}},
			}
		}).Twice()

		queryIdle.On("Index", "gsi2").Return(queryIdle).Twice()
		queryIdle.On("Where", "gsi2PK", "=", "STATE#idle").Return(queryIdle).Twice()
		queryIdle.On("Limit", connectionQueryLimit).Return(queryIdle).Twice()
		queryIdle.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.WebSocketConnection)
			*dest = []models.WebSocketConnection{
				{ConnectionID: "c3", State: models.ConnectionStateIdle, Metrics: models.ConnectionMetrics{ErrorCount: 0}},
			}
		}).Twice()

		queryError.On("Index", "gsi2").Return(queryError).Once()
		queryError.On("Where", "gsi2PK", "=", "STATE#error").Return(queryError).Once()
		queryError.On("Limit", connectionQueryLimit).Return(queryError).Once()
		queryError.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.WebSocketConnection)
			*dest = []models.WebSocketConnection{
				{ConnectionID: "c4", State: models.ConnectionStateError, Metrics: models.ConnectionMetrics{ErrorCount: 99}},
			}
		}).Once()

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
		healthy, err := repo.GetHealthyConnections(ctx)
		require.NoError(t, err)
		require.Len(t, healthy, 1)
		assert.Equal(t, "c1", healthy[0].ConnectionID)

		unhealthy, err := repo.GetUnhealthyConnections(ctx)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(unhealthy), 2)

		queryConnected.AssertExpectations(t)
		queryIdle.AssertExpectations(t)
		queryError.AssertExpectations(t)
	})
}

func TestStreamingConnectionRepository_Round08_IdleAndMaintenance(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Unix(0, 0).UTC()

	t.Run("MarkConnectionsIdle and CloseTimedOutConnections update matching connections", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		queryConnected := new(mocks.MockQuery)
		queryIdle := new(mocks.MockQuery)
		queryUpdate := new(mocks.MockQuery)

		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockSubDB, mockSubQuery, nil, baseTime)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(queryConnected).Once()
		mockDB.On("Model", mock.Anything).Return(queryUpdate).Once()
		mockDB.On("Model", mock.Anything).Return(queryIdle).Once()
		mockDB.On("Model", mock.Anything).Return(queryUpdate).Once()

		queryConnected.On("Index", "gsi2").Return(queryConnected).Once()
		queryConnected.On("Where", "gsi2PK", "=", "STATE#connected").Return(queryConnected).Once()
		queryConnected.On("Limit", connectionQueryLimit).Return(queryConnected).Once()
		queryConnected.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.WebSocketConnection)
			*dest = []models.WebSocketConnection{
				{ConnectionID: "c1", LastActivity: time.Now().Add(-2 * time.Hour), IdleTimeout: time.Hour, State: models.ConnectionStateConnected},
				{ConnectionID: "c2", LastActivity: time.Now().Add(-5 * time.Minute), IdleTimeout: time.Hour, State: models.ConnectionStateConnected},
			}
		}).Once()

		queryIdle.On("Index", "gsi2").Return(queryIdle).Once()
		queryIdle.On("Where", "gsi2PK", "=", "STATE#idle").Return(queryIdle).Once()
		queryIdle.On("Limit", connectionQueryLimit).Return(queryIdle).Once()
		queryIdle.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.WebSocketConnection)
			*dest = []models.WebSocketConnection{
				{ConnectionID: "c3", LastActivity: time.Now().Add(-3 * time.Hour), IdleTimeout: time.Hour, State: models.ConnectionStateIdle},
				{ConnectionID: "c4", LastActivity: time.Now().Add(-10 * time.Minute), IdleTimeout: 24 * time.Hour, State: models.ConnectionStateIdle},
			}
		}).Once()

		queryUpdate.On("Update", mock.Anything).Return(nil).Twice()

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		marked, err := repo.MarkConnectionsIdle(ctx, time.Hour)
		require.NoError(t, err)
		assert.Equal(t, 1, marked)

		closed, err := repo.CloseTimedOutConnections(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, closed)
	})

	t.Run("UpdateConnectionActivity and record helpers load and update connection", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockSubDB, mockSubQuery, nil, baseTime)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

		// Five GetConnection calls in order.
		mockQuery.On("First", mock.AnythingOfType("*models.WebSocketConnection")).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.WebSocketConnection)
			*dest = models.WebSocketConnection{ConnectionID: "c1", UserID: "u1", State: models.ConnectionStateIdle, Established: baseTime, LastActivity: baseTime, RateLimit: 10, RateLimitReset: time.Now().Add(time.Hour), MaxMessageSize: 1024}
			_ = dest.UpdateKeys()
		}).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.WebSocketConnection")).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.WebSocketConnection)
			*dest = models.WebSocketConnection{ConnectionID: "c2", UserID: "u1", State: models.ConnectionStateIdle, Established: baseTime, LastActivity: baseTime, RateLimit: 10, RateLimitReset: time.Now().Add(-time.Minute), MaxMessageSize: 1024}
			_ = dest.UpdateKeys()
		}).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.WebSocketConnection")).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.WebSocketConnection)
			*dest = models.WebSocketConnection{ConnectionID: "c3", UserID: "u1", State: models.ConnectionStateConnected, Established: baseTime, LastActivity: baseTime, Metrics: models.ConnectionMetrics{ErrorCount: 9}}
			_ = dest.UpdateKeys()
		}).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.WebSocketConnection")).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.WebSocketConnection)
			*dest = models.WebSocketConnection{ConnectionID: "c4", UserID: "u1", State: models.ConnectionStateConnected, Established: baseTime, LastActivity: baseTime}
			_ = dest.UpdateKeys()
		}).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.WebSocketConnection")).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.WebSocketConnection)
			*dest = models.WebSocketConnection{ConnectionID: "c5", UserID: "u1", State: models.ConnectionStateConnected, Established: baseTime, LastActivity: baseTime}
			_ = dest.UpdateKeys()
		}).Once()

		mockQuery.On("Update", mock.Anything).Return(nil).Times(5)

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)

		require.NoError(t, repo.UpdateConnectionActivity(ctx, "c1"))
		require.NoError(t, repo.RecordConnectionMessage(ctx, "c2", true, 10))
		require.NoError(t, repo.RecordConnectionError(ctx, "c3", "boom"))
		require.NoError(t, repo.RecordPing(ctx, "c4"))
		require.NoError(t, repo.RecordPong(ctx, "c5"))
	})

	t.Run("GetConnectionPool and cleanup helpers return stable results", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockSubDB, mockSubQuery, nil, baseTime)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Count").Return(int64(1), nil).Maybe()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			if dest, ok := args.Get(0).(*[]models.WebSocketConnection); ok {
				*dest = nil
			}
		}).Maybe()

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
		stats, err := repo.GetConnectionPool(ctx)
		require.NoError(t, err)
		assert.Equal(t, MaxConnectionsPerUser, stats["max_per_user"])

		n, err := repo.CleanupExpiredConnections(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, n)
	})

	t.Run("DeleteConnection and DeleteSubscription use base delete helpers", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("Delete").Return(nil).Once()

		mockSubDB.On("WithContext", mock.Anything).Return(mockSubDB).Once()
		mockSubDB.On("Model", mock.Anything).Return(mockSubQuery).Once()
		mockSubQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockSubQuery).Twice()
		mockSubQuery.On("Delete").Return(nil).Once()

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)
		repo.subscriptionRepo.SetValidationService(nil)
		repo.subscriptionRepo.SetPermissionService(nil)
		repo.subscriptionRepo.SetCachingService(nil)
		repo.subscriptionRepo.SetEventService(nil)

		require.NoError(t, repo.DeleteConnection(ctx, "c1"))
		require.NoError(t, repo.DeleteSubscription(ctx, "c1", "stream1"))
	})
}

func TestStreamingConnectionRepository_Round08_Sweep(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockSubDB := new(mocks.MockDB)
	mockSubQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Count").Return(int64(1), nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()

	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest, ok := args.Get(0).(*models.WebSocketConnection)
		if !ok {
			return
		}
		*dest = models.WebSocketConnection{
			ConnectionID:   "c1",
			UserID:         "u1",
			Username:       "alice",
			State:          models.ConnectionStateConnected,
			Established:    now.Add(-time.Hour),
			LastActivity:   now.Add(-time.Minute),
			IdleTimeout:    time.Minute,
			MaxMessageSize: 1024,
			RateLimit:      100,
			CurrentRate:    0,
			RateLimitReset: now.Add(-time.Minute),
		}
		_ = dest.UpdateKeys()
	}).Maybe()

	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]models.WebSocketConnection:
			*dest = []models.WebSocketConnection{
				{ConnectionID: "c1", UserID: "u1", Username: "alice", State: models.ConnectionStateConnected, LastActivity: now.Add(-2 * time.Hour), IdleTimeout: time.Minute},
				{ConnectionID: "c2", UserID: "u1", Username: "alice", State: models.ConnectionStateIdle, LastActivity: now.Add(-time.Hour), IdleTimeout: time.Minute},
			}
		}
	}).Maybe()

	mockQuery.On("Scan", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.WebSocketConnection)
		*dest = []models.WebSocketConnection{
			{ConnectionID: "c1", UserID: "u1", Username: "alice", LastActivity: now.Add(-2 * time.Hour), TTL: now.Add(-time.Second).Unix()},
			{ConnectionID: "c2", UserID: "u1", Username: "alice", LastActivity: now.Add(time.Minute), TTL: now.Add(time.Hour).Unix()},
		}
	}).Maybe()

	mockSubDB.On("WithContext", mock.Anything).Return(mockSubDB).Maybe()
	mockSubDB.On("Model", mock.Anything).Return(mockSubQuery).Maybe()
	mockSubQuery.On("Index", mock.Anything).Return(mockSubQuery).Maybe()
	mockSubQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockSubQuery).Maybe()
	mockSubQuery.On("Limit", mock.Anything).Return(mockSubQuery).Maybe()
	mockSubQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockSubQuery).Maybe()
	mockSubQuery.On("Cursor", mock.Anything).Return(mockSubQuery).Maybe()
	mockSubQuery.On("Create").Return(nil).Maybe()
	mockSubQuery.On("Delete").Return(nil).Maybe()

	mockSubQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]*models.WebSocketSubscription:
			sub := &models.WebSocketSubscription{ConnectionID: "c1", UserID: "u1", Stream: "stream1", SubscribedAt: now}
			_ = sub.UpdateKeys()
			*dest = []*models.WebSocketSubscription{sub}
		case *[]models.WebSocketSubscription:
			*dest = []models.WebSocketSubscription{
				{ConnectionID: "c1", UserID: "u1", Stream: "stream1", SubscribedAt: now},
			}
		}
	}).Maybe()

	repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)
	repo.subscriptionRepo.SetValidationService(nil)
	repo.subscriptionRepo.SetPermissionService(nil)
	repo.subscriptionRepo.SetCachingService(nil)
	repo.subscriptionRepo.SetEventService(nil)

	_, _ = repo.WriteConnection(ctx, "c1", "u1", "alice", []string{"stream1"})
	_, _ = repo.GetConnection(ctx, "c1")
	_ = repo.UpdateConnection(ctx, &models.WebSocketConnection{ConnectionID: "c1"})
	_ = repo.UpdateConnectionState(ctx, "c1", models.ConnectionStateIdle, "")
	_ = repo.DeleteConnection(ctx, "c1")

	_ = repo.WriteSubscription(ctx, "c1", "u1", "stream1")
	_ = repo.DeleteSubscription(ctx, "c1", "stream1")
	_ = repo.DeleteAllSubscriptions(ctx, "c1")

	_, _ = repo.GetConnectionsByUser(ctx, "u1")
	_, _ = repo.GetSubscriptionsForStream(ctx, "stream1")
	_, _ = repo.GetIdleConnections(ctx, now.Add(-time.Hour))
	_, _ = repo.GetStaleConnections(ctx, now.Add(-time.Hour))
	_, _ = repo.MarkConnectionsIdle(ctx, time.Minute)
	_, _ = repo.CloseTimedOutConnections(ctx)

	_, _ = repo.GetConnectionPool(ctx)
	_, _ = repo.ReclaimIdleConnections(ctx, 0)

	_ = repo.EnforceResourceLimits(ctx, "c1", 1)
	_ = repo.UpdateConnectionActivity(ctx, "c1")
	_ = repo.RecordConnectionMessage(ctx, "c1", true, 1)
	_ = repo.RecordConnectionError(ctx, "c1", "err")
	_ = repo.RecordPing(ctx, "c1")
	_ = repo.RecordPong(ctx, "c1")

	_, _ = repo.GetConnectionCountByState(ctx, models.ConnectionStateConnected)
	_, _ = repo.GetUserConnectionCount(ctx, "u1")
	_, _ = repo.GetConnectionsByState(ctx, models.ConnectionStateConnected)
	_, _ = repo.GetHealthyConnections(ctx)
	_, _ = repo.GetUnhealthyConnections(ctx)
	_, _ = repo.CleanupExpiredConnections(ctx)

	assert.True(t, isResourceNotFound(errors.New("index not found")))
	assert.False(t, isResourceNotFound(nil))
}

func TestStreamingConnectionRepository_Round08_CountAndStateErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("GetConnectionCountByState handles not found, missing index, and errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewStreamingConnectionRepository(mockDB, "table", mockDB, "subs", zap.NewNop(), nil)

		// not found -> 0, nil
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Count").Return(int64(0), dynamormerrors.ErrItemNotFound).Once()
		n, err := repo.GetConnectionCountByState(ctx, models.ConnectionStateConnected)
		require.NoError(t, err)
		assert.Equal(t, 0, n)

		// missing index -> 0, nil
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Count").Return(int64(0), errors.New("does not have the specified index")).Once()
		n, err = repo.GetConnectionCountByState(ctx, models.ConnectionStateConnected)
		require.NoError(t, err)
		assert.Equal(t, 0, n)

		// other error -> error
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Count").Return(int64(0), assert.AnError).Once()
		_, err = repo.GetConnectionCountByState(ctx, models.ConnectionStateConnected)
		require.Error(t, err)
	})

	t.Run("GetUserConnectionCount handles not found, missing index, and errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewStreamingConnectionRepository(mockDB, "table", mockDB, "subs", zap.NewNop(), nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Count").Return(int64(0), dynamormerrors.ErrItemNotFound).Once()
		n, err := repo.GetUserConnectionCount(ctx, "u1")
		require.NoError(t, err)
		assert.Equal(t, 0, n)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Count").Return(int64(0), errors.New("requested resource not found")).Once()
		n, err = repo.GetUserConnectionCount(ctx, "u1")
		require.NoError(t, err)
		assert.Equal(t, 0, n)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Count").Return(int64(0), assert.AnError).Once()
		_, err = repo.GetUserConnectionCount(ctx, "u1")
		require.Error(t, err)
	})

	t.Run("GetConnectionsByState handles missing index, warns on truncation, and maps errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewStreamingConnectionRepository(mockDB, "table", mockDB, "subs", zap.NewNop(), nil)

		// missing index
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", connectionQueryLimit).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(errors.New("index not found")).Once()
		items, err := repo.GetConnectionsByState(ctx, models.ConnectionStateConnected)
		require.NoError(t, err)
		assert.Empty(t, items)

		// truncation warning branch (len == connectionQueryLimit)
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", connectionQueryLimit).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.WebSocketConnection)
			conns := make([]models.WebSocketConnection, connectionQueryLimit)
			for i := range conns {
				conns[i] = models.WebSocketConnection{ConnectionID: "c"}
			}
			*dest = conns
		}).Once()
		items, err = repo.GetConnectionsByState(ctx, models.ConnectionStateConnected)
		require.NoError(t, err)
		assert.Len(t, items, connectionQueryLimit)

		// other error
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", connectionQueryLimit).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(assert.AnError).Once()
		_, err = repo.GetConnectionsByState(ctx, models.ConnectionStateConnected)
		require.Error(t, err)
	})

	t.Run("DeleteAllSubscriptions returns query error when list fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewStreamingConnectionRepository(nil, "table", mockDB, "subs", zap.NewNop(), nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(assert.AnError).Once()

		err := repo.DeleteAllSubscriptions(ctx, "c1")
		require.Error(t, err)
	})
}

func TestStreamingConnectionRepository_Round08_MoreErrorBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Unix(0, 0).UTC()

	t.Run("WriteConnection returns error when user count query fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockSubDB, mockSubQuery, nil, baseTime)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Count").Return(int64(0), assert.AnError).Once()

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
		_, err := repo.WriteConnection(ctx, "c1", "u1", "alice", nil)
		require.Error(t, err)
	})

	t.Run("WriteConnection rejects when global connection limit reached", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		queryUser := new(mocks.MockQuery)
		queryConnected := new(mocks.MockQuery)
		queryIdle := new(mocks.MockQuery)
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockSubDB, mockSubQuery, nil, baseTime)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(queryUser).Once()
		mockDB.On("Model", mock.Anything).Return(queryConnected).Once()
		mockDB.On("Model", mock.Anything).Return(queryIdle).Once()

		queryUser.On("Index", "gsi1").Return(queryUser).Once()
		queryUser.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(queryUser).Once()
		queryUser.On("Count").Return(int64(0), nil).Once()

		queryConnected.On("Index", "gsi2").Return(queryConnected).Once()
		queryConnected.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(queryConnected).Once()
		queryConnected.On("Count").Return(int64(MaxTotalConnections), nil).Once()

		queryIdle.On("Index", "gsi2").Return(queryIdle).Once()
		queryIdle.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(queryIdle).Once()
		queryIdle.On("Count").Return(int64(0), nil).Once()

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
		_, err := repo.WriteConnection(ctx, "c1", "u1", "alice", nil)
		require.Error(t, err)
	})

	t.Run("GetConnectionsByUser maps non-notfound query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewStreamingConnectionRepository(mockDB, "table", mockDB, "subs", zap.NewNop(), nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", connectionQueryLimit).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(assert.AnError).Once()

		_, err := repo.GetConnectionsByUser(ctx, "u1")
		require.Error(t, err)
	})

	t.Run("GetSubscriptionsForStream maps pagination error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		mockSubDB.On("WithContext", mock.Anything).Return(mockSubDB).Once()
		mockSubDB.On("Model", mock.Anything).Return(mockSubQuery).Once()
		mockSubQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockSubQuery).Once()
		mockSubQuery.On("Limit", 101).Return(mockSubQuery).Once()
		mockSubQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockSubQuery).Once()
		mockSubQuery.On("All", mock.Anything).Return(assert.AnError).Once()

		repo := NewStreamingConnectionRepository(mockDB, "table", mockSubDB, "subs", zap.NewNop(), nil)
		_, err := repo.GetSubscriptionsForStream(ctx, "stream1")
		require.Error(t, err)
	})

	t.Run("GetIdleConnections and GetStaleConnections map scan errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewStreamingConnectionRepository(mockDB, "table", mockDB, "subs", zap.NewNop(), nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("Scan", mock.Anything).Return(assert.AnError).Twice()

		_, err := repo.GetIdleConnections(ctx, time.Now())
		require.Error(t, err)

		_, err = repo.GetStaleConnections(ctx, time.Now())
		require.Error(t, err)
	})

	t.Run("GetHealthyConnections returns error when state query fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewStreamingConnectionRepository(mockDB, "table", mockDB, "subs", zap.NewNop(), nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", connectionQueryLimit).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(assert.AnError).Once()

		_, err := repo.GetHealthyConnections(ctx)
		require.Error(t, err)
	})
}

func TestStreamingConnectionRepository_Round08_PoolAndUnhealthyErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("GetConnectionPool returns error when count fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewStreamingConnectionRepository(mockDB, "table", mockDB, "subs", zap.NewNop(), nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Count").Return(int64(0), assert.AnError).Once()

		_, err := repo.GetConnectionPool(ctx)
		require.Error(t, err)
	})

	t.Run("GetConnectionPool returns error when state listing fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		qCountConnected := new(mocks.MockQuery)
		qCountIdle := new(mocks.MockQuery)
		qListConnected := new(mocks.MockQuery)

		repo := NewStreamingConnectionRepository(mockDB, "table", mockDB, "subs", zap.NewNop(), nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(qCountConnected).Once()
		mockDB.On("Model", mock.Anything).Return(qCountIdle).Once()
		mockDB.On("Model", mock.Anything).Return(qListConnected).Once()

		qCountConnected.On("Index", "gsi2").Return(qCountConnected).Once()
		qCountConnected.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(qCountConnected).Once()
		qCountConnected.On("Count").Return(int64(0), nil).Once()

		qCountIdle.On("Index", "gsi2").Return(qCountIdle).Once()
		qCountIdle.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(qCountIdle).Once()
		qCountIdle.On("Count").Return(int64(0), nil).Once()

		qListConnected.On("Index", "gsi2").Return(qListConnected).Once()
		qListConnected.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(qListConnected).Once()
		qListConnected.On("Limit", connectionQueryLimit).Return(qListConnected).Once()
		qListConnected.On("All", mock.Anything).Return(assert.AnError).Once()

		_, err := repo.GetConnectionPool(ctx)
		require.Error(t, err)
	})

	t.Run("GetUnhealthyConnections returns error when error-state query fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewStreamingConnectionRepository(mockDB, "table", mockDB, "subs", zap.NewNop(), nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", connectionQueryLimit).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(assert.AnError).Once()

		_, err := repo.GetUnhealthyConnections(ctx)
		require.Error(t, err)
	})
}

func TestStreamingConnectionRepository_Round08_CrudErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("UpdateConnection maps update errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewStreamingConnectionRepository(mockDB, "table", mockDB, "subs", zap.NewNop(), nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Update", mock.Anything).Return(assert.AnError).Once()

		err := repo.UpdateConnection(ctx, &models.WebSocketConnection{ConnectionID: "c1"})
		require.Error(t, err)
	})

	t.Run("DeleteConnection maps delete errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewStreamingConnectionRepository(mockDB, "table", mockDB, "subs", zap.NewNop(), nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("Delete").Return(assert.AnError).Once()

		err := repo.DeleteConnection(ctx, "c1")
		require.Error(t, err)
	})

	t.Run("DeleteSubscription maps delete errors", func(t *testing.T) {
		mockSubDB := new(mocks.MockDB)
		mockSubQuery := new(mocks.MockQuery)
		repo := NewStreamingConnectionRepository(nil, "table", mockSubDB, "subs", zap.NewNop(), nil)
		repo.subscriptionRepo.SetValidationService(nil)
		repo.subscriptionRepo.SetPermissionService(nil)
		repo.subscriptionRepo.SetCachingService(nil)
		repo.subscriptionRepo.SetEventService(nil)

		mockSubDB.On("WithContext", mock.Anything).Return(mockSubDB).Once()
		mockSubDB.On("Model", mock.Anything).Return(mockSubQuery).Once()
		mockSubQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockSubQuery).Twice()
		mockSubQuery.On("Delete").Return(assert.AnError).Once()

		err := repo.DeleteSubscription(ctx, "c1", "stream1")
		require.Error(t, err)
	})

	t.Run("RecordPing returns error when GetConnection fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewStreamingConnectionRepository(mockDB, "table", mockDB, "subs", zap.NewNop(), nil)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(assert.AnError).Once()

		err := repo.RecordPing(ctx, "c1")
		require.Error(t, err)
	})
}
