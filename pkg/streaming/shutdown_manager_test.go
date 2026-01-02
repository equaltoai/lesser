package streaming

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestShutdownManager_GracefulShutdown_NoConnections(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateConnected).Return([]models.WebSocketConnection{}, nil).Twice()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateIdle).Return([]models.WebSocketConnection{}, nil).Twice()

	sm := NewShutdownManager(repo, nil, zap.NewNop(), &ShutdownManagerConfig{
		ShutdownTimeout: 50 * time.Millisecond,
		DrainTimeout:    10 * time.Millisecond,
		Backpressure:    DefaultShutdownManagerConfig().Backpressure,
	})

	require.NoError(t, sm.InitiateGracefulShutdown(context.Background()))
	require.Error(t, sm.InitiateGracefulShutdown(context.Background()))
	require.NoError(t, sm.WaitForShutdown())
	assert.True(t, sm.IsShuttingDown())
}

func TestShutdownManager_SendDrainNotificationAndForceClose(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()

	api := &stubStreamerClient{}
	sm := NewShutdownManager(repo, api, zap.NewNop(), &ShutdownManagerConfig{
		ShutdownTimeout: 10 * time.Millisecond,
		DrainTimeout:    time.Second,
		Backpressure:    DefaultShutdownManagerConfig().Backpressure,
	})

	repo.On("UpdateConnectionState", mock.Anything, "c1", models.ConnectionStateClosing, mock.Anything).Return(nil).Once()
	require.NoError(t, sm.sendDrainNotification(context.Background(), &models.WebSocketConnection{ConnectionID: "c1"}))

	remaining := []models.WebSocketConnection{{ConnectionID: "c2"}}
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateConnected).Return(remaining, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateIdle).Return([]models.WebSocketConnection{}, nil).Once()
	repo.On("UpdateConnectionState", mock.Anything, "c2", models.ConnectionStateClosed, mock.Anything).Return(nil).Once()
	repo.On("DeleteAllSubscriptions", mock.Anything, "c2").Return(errors.New("subs down")).Once()

	require.NoError(t, sm.forceCloseConnections(context.Background()))
}

func TestShutdownManager_ApplyBackpressure(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()

	sm := NewShutdownManager(repo, nil, zap.NewNop(), DefaultShutdownManagerConfig())
	sm.rateLimiter = &RateLimiter{capacity: 2, tokens: 2, refillRate: 0, lastRefill: time.Now()}

	// Shutting down rejects.
	sm.mu.Lock()
	sm.isShuttingDown = true
	sm.mu.Unlock()
	action, err := sm.ApplyBackpressure("c1", 1)
	require.Error(t, err)
	assert.Equal(t, BackpressureReject, action)

	// Rate limited delays.
	sm.mu.Lock()
	sm.isShuttingDown = false
	sm.mu.Unlock()
	sm.rateLimiter.mu.Lock()
	sm.rateLimiter.tokens = 0
	sm.rateLimiter.mu.Unlock()

	action, err = sm.ApplyBackpressure("c1", 1)
	require.Error(t, err)
	assert.Equal(t, BackpressureDelay, action)

	// Connection-specific checks.
	sm.rateLimiter.mu.Lock()
	sm.rateLimiter.tokens = 2
	sm.rateLimiter.mu.Unlock()

	repo.On("GetConnection", mock.Anything, "c1").Return(nil, errors.New("missing")).Once()
	action, err = sm.ApplyBackpressure("c1", 1)
	require.Error(t, err)
	assert.Equal(t, BackpressureDelay, action)

	sm.rateLimiter.mu.Lock()
	sm.rateLimiter.tokens = 2
	sm.rateLimiter.mu.Unlock()

	conn := &models.WebSocketConnection{ConnectionID: "c1", State: models.ConnectionStateConnected}
	conn.Metrics.ConnectionQuality = 0.2
	repo.On("GetConnection", mock.Anything, "c1").Return(conn, nil).Once()
	action, err = sm.ApplyBackpressure("c1", 1)
	require.Error(t, err)
	assert.Equal(t, BackpressureDelay, action)

	sm.rateLimiter.mu.Lock()
	sm.rateLimiter.tokens = 2
	sm.rateLimiter.mu.Unlock()

	connOK := &models.WebSocketConnection{ConnectionID: "c1", State: models.ConnectionStateConnected}
	connOK.Metrics.ConnectionQuality = 1.0
	repo.On("GetConnection", mock.Anything, "c1").Return(connOK, nil).Once()
	action, err = sm.ApplyBackpressure("c1", 1)
	require.NoError(t, err)
	assert.Equal(t, BackpressureAllow, action)

	// Oversized message rejects.
	sm.rateLimiter.mu.Lock()
	sm.rateLimiter.tokens = 2
	sm.rateLimiter.mu.Unlock()

	repo.On("GetConnection", mock.Anything, "c1").Return(connOK, nil).Once()
	action, err = sm.ApplyBackpressure("c1", 128*1024)
	require.Error(t, err)
	assert.Equal(t, BackpressureReject, action)
}

func TestShutdownManager_Stats(t *testing.T) {
	sm := NewShutdownManager(testmocks.NewMockStreamingConnectionRepository(), nil, zap.NewNop(), DefaultShutdownManagerConfig())

	sm.rateLimiter = &RateLimiter{capacity: 10, tokens: 5, refillRate: 1, lastRefill: time.Now()}
	backpressure := sm.GetBackpressureStats()
	assert.Equal(t, false, backpressure["is_shutting_down"])

	sm.mu.Lock()
	sm.isShuttingDown = true
	sm.shutdownStarted = time.Now().Add(-time.Second)
	sm.mu.Unlock()

	stats := sm.GetShutdownStats()
	assert.Equal(t, true, stats["is_shutting_down"])
	assert.NotNil(t, stats["shutdown_started"])
}

func TestShutdownManager_WaitForConnectionDrain_ContextTimeout(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateConnected).Return([]models.WebSocketConnection{}, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateIdle).Return([]models.WebSocketConnection{}, nil).Once()

	sm := NewShutdownManager(repo, nil, zap.NewNop(), DefaultShutdownManagerConfig())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	require.NoError(t, sm.waitForConnectionDrain(ctx, []models.WebSocketConnection{{ConnectionID: "c1"}}))
}

func TestShutdownManager_DrainConnections_DrainsOnTicker(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()

	api := &stubStreamerClient{
		post: func(_ context.Context, connectionID string, _ []byte) error {
			if connectionID == "bad" {
				return errors.New("send failed")
			}
			return nil
		},
	}

	sm := NewShutdownManager(repo, api, zap.NewNop(), &ShutdownManagerConfig{
		ShutdownTimeout: time.Second,
		DrainTimeout:    3 * time.Second,
		Backpressure:    DefaultShutdownManagerConfig().Backpressure,
	})

	// Initial active connections (1 good, 1 bad).
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateConnected).
		Return([]models.WebSocketConnection{{ConnectionID: "good"}, {ConnectionID: "bad"}}, nil).
		Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateIdle).
		Return([]models.WebSocketConnection{}, nil).
		Twice()

	// Second check on ticker: all connections drained.
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateConnected).
		Return([]models.WebSocketConnection{}, nil).
		Once()

	repo.On("UpdateConnectionState", mock.Anything, "good", models.ConnectionStateClosing, mock.Anything).Return(nil).Once()

	require.NoError(t, sm.drainConnections(context.Background()))
}
