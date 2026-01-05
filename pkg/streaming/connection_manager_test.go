package streaming

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestMarshalMessage(t *testing.T) {
	data, err := marshalMessage(map[string]interface{}{"type": "ping", "timestamp": 1})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(data), "{"))
	assert.Contains(t, string(data), "\"type\"")

	_, err = marshalMessage(struct{}{})
	require.Error(t, err)
}

func TestConnectionManager_SendPing(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()
	cm := NewConnectionManager(repo, nil, zap.NewNop(), &ConnectionManagerConfig{
		HealthCheckInterval: time.Hour,
		CleanupInterval:     time.Hour,
		PingTimeout:         time.Second,
		IdleThreshold:       time.Hour,
		MaxIdleConnections:  10,
	})

	err := cm.sendPing(context.Background(), &models.WebSocketConnection{ConnectionID: "c1"})
	require.Error(t, err)

	repo.On("RecordPing", mock.Anything, "c1").Return(nil).Once()
	cm.apiClient = &stubStreamerClient{}
	require.NoError(t, cm.sendPing(context.Background(), &models.WebSocketConnection{ConnectionID: "c1"}))
}

func TestConnectionManager_RunHealthCheckAndCleanup(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()

	api := &stubStreamerClient{
		post: func(_ context.Context, connectionID string, _ []byte) error {
			if connectionID == "c3" {
				return errors.New("send failed")
			}
			return nil
		},
	}

	cm := NewConnectionManager(repo, api, zap.NewNop(), &ConnectionManagerConfig{
		HealthCheckInterval: time.Hour,
		CleanupInterval:     time.Hour,
		PingTimeout:         time.Second,
		IdleThreshold:       time.Second,
		MaxIdleConnections:  10,
	})

	conn1 := models.WebSocketConnection{ConnectionID: "c1", UserID: "u1", State: models.ConnectionStateConnected, LastActivity: time.Now()}
	conn1.Metrics.ErrorCount = 0
	conn1.Metrics.ConnectionQuality = 1

	conn2 := models.WebSocketConnection{ConnectionID: "c2", UserID: "u2", State: models.ConnectionStateConnected, LastActivity: time.Now().Add(-time.Minute)}
	conn2.Metrics.ErrorCount = 0
	conn2.Metrics.ConnectionQuality = 1

	conn3 := models.WebSocketConnection{ConnectionID: "c3", UserID: "u3", State: models.ConnectionStateConnected, LastActivity: time.Now()}
	conn3.Metrics.ErrorCount = 0
	conn3.Metrics.ConnectionQuality = 1

	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateConnected).Return([]models.WebSocketConnection{conn1, conn2, conn3}, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateIdle).Return([]models.WebSocketConnection{}, nil).Once()

	repo.On("UpdateConnection", mock.Anything, mock.MatchedBy(func(c *models.WebSocketConnection) bool {
		return c.ConnectionID == "c2" && c.State == models.ConnectionStateIdle
	})).Return(nil).Once()

	repo.On("RecordPing", mock.Anything, "c1").Return(nil).Once()

	repo.On("RecordConnectionError", mock.Anything, "c3", mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "ping failed")
	})).Return(nil).Once()

	require.NoError(t, cm.runHealthCheck(context.Background()))

	// Cleanup path.
	repo.On("MarkConnectionsIdle", mock.Anything, cm.idleThreshold).Return(1, nil).Once()
	repo.On("CloseTimedOutConnections", mock.Anything).Return(2, errors.New("boom")).Once()
	repo.On("ReclaimIdleConnections", mock.Anything, cm.maxIdleConnections).Return(3, nil).Once()
	repo.On("CleanupExpiredConnections", mock.Anything).Return(4, nil).Once()

	require.NoError(t, cm.runCleanup(context.Background()))
}

func TestConnectionManager_StatsAndLifecycle(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()
	repo.On("GetConnectionPool", mock.Anything).Return(map[string]interface{}{"active": 1}, nil).Once()

	cm := NewConnectionManager(repo, nil, zap.NewNop(), &ConnectionManagerConfig{
		HealthCheckInterval: time.Hour,
		CleanupInterval:     time.Hour,
		PingTimeout:         time.Second,
		IdleThreshold:       time.Hour,
		MaxIdleConnections:  10,
	})

	stats, err := cm.GetConnectionStats(context.Background())
	require.NoError(t, err)
	assert.Equal(t, false, stats["manager_running"])
	assert.Equal(t, 10, stats["max_idle_connections"])

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, cm.Start(ctx))
	require.Error(t, cm.Start(ctx))
	cancel()
	require.NoError(t, cm.Stop())
	require.False(t, cm.IsRunning())
	require.NoError(t, cm.Stop())
}

func TestConnectionManager_DefaultConfigAndForceWrappers(t *testing.T) {
	cfg := DefaultConnectionManagerConfig()
	require.NotNil(t, cfg)
	assert.True(t, cfg.HealthCheckInterval > 0)
	assert.True(t, cfg.CleanupInterval > 0)

	repo := testmocks.NewMockStreamingConnectionRepository()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateConnected).Return([]models.WebSocketConnection{}, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateIdle).Return([]models.WebSocketConnection{}, nil).Once()

	cm := NewConnectionManager(repo, nil, zap.NewNop(), cfg)
	require.NoError(t, cm.ForceHealthCheck(context.Background()))

	repo.On("MarkConnectionsIdle", mock.Anything, cm.idleThreshold).Return(0, nil).Once()
	repo.On("CloseTimedOutConnections", mock.Anything).Return(0, nil).Once()
	repo.On("ReclaimIdleConnections", mock.Anything, cm.maxIdleConnections).Return(0, nil).Once()
	repo.On("CleanupExpiredConnections", mock.Anything).Return(0, nil).Once()
	require.NoError(t, cm.ForceCleanup(context.Background()))
}
