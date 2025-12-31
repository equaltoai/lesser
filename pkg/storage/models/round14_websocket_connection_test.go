package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSocketConnection_KeysAndQuality(t *testing.T) {
	t.Run("UpdateKeys sets PK/SK, user GSI, and state GSI", func(t *testing.T) {
		w := &WebSocketConnection{
			ConnectionID: "c1",
			UserID:       "u1",
			Established:  time.Unix(1700000000, 0).UTC(),
			State:        ConnectionStateConnected,
		}
		require.NoError(t, w.UpdateKeys())
		assert.Equal(t, "CONN#c1", w.PK)
		assert.Equal(t, "CONN#c1", w.SK)
		assert.Equal(t, "USER#u1", w.GSI1PK)
		assert.Contains(t, w.GSI1SK, "CONN#")
		assert.Equal(t, "STATE#connected", w.GSI2PK)
		assert.Equal(t, "CONN#c1", w.GSI2SK)
		assert.Equal(t, w.PK, w.GetPK())
		assert.Equal(t, w.SK, w.GetSK())
		assert.Equal(t, MainTableName, w.TableName())
		assert.Equal(t, MainTableName, (ConnectionMetrics{}).TableName())
		assert.Equal(t, MainTableName, (ConnectionInfo{}).TableName())
	})

	t.Run("UpdateState bumps timestamp and updates state GSI", func(t *testing.T) {
		w := &WebSocketConnection{ConnectionID: "c1", State: ConnectionStateConnecting}
		prev := w.StateChangedAt
		w.UpdateState(ConnectionStateError)
		assert.Equal(t, ConnectionStateError, w.State)
		assert.True(t, w.StateChangedAt.After(prev) || prev.IsZero())
		assert.Equal(t, "STATE#error", w.GSI2PK)
	})

	t.Run("IsHealthy/IsActive/ShouldRetry", func(t *testing.T) {
		w := &WebSocketConnection{State: ConnectionStateConnected}
		w.Metrics.ErrorCount = 9
		assert.True(t, w.IsHealthy())
		w.Metrics.ErrorCount = 10
		assert.False(t, w.IsHealthy())

		w.LastActivity = time.Now().Add(-time.Minute)
		assert.True(t, w.IsActive(2*time.Minute))
		assert.False(t, w.IsActive(30*time.Second))

		w.State = ConnectionStateError
		w.RetryCount = 1
		w.MaxRetries = 2
		assert.True(t, w.ShouldRetry())
		w.RetryCount = 2
		assert.False(t, w.ShouldRetry())
	})

	t.Run("CalculateConnectionQuality covers branches and clamps", func(t *testing.T) {
		w := &WebSocketConnection{}
		assert.InDelta(t, 1.0, w.CalculateConnectionQuality(), 0.000001)

		w = &WebSocketConnection{}
		w.Metrics.MessagesReceived = 10
		w.Metrics.MessagesSent = 0
		w.Metrics.ErrorCount = 2
		w.Metrics.PingLatencyMs = 2000
		q := w.CalculateConnectionQuality()
		assert.InDelta(t, 0.6, q, 0.000001)
		assert.InDelta(t, q, w.Metrics.ConnectionQuality, 0.000001)

		w.Metrics.ErrorCount = 100
		assert.Equal(t, 0.0, w.CalculateConnectionQuality())

		// Force clamp-high path via negative error count.
		w.Metrics.MessagesReceived = 1
		w.Metrics.ErrorCount = -1
		assert.Equal(t, 1.0, w.CalculateConnectionQuality())
	})

	t.Run("IncrementError and message/ping recording mutate metrics", func(t *testing.T) {
		w := &WebSocketConnection{}
		w.Metrics.MessagesReceived = 10

		w.IncrementError("boom")
		assert.Equal(t, int32(1), w.Metrics.ErrorCount)
		assert.Equal(t, "boom", w.Metrics.LastError)

		prevActivity := w.LastActivity
		w.RecordMessage(true, 100)
		assert.Equal(t, int64(1), w.Metrics.MessagesSent)
		assert.Equal(t, int64(100), w.Metrics.BytesSent)
		assert.True(t, w.LastActivity.After(prevActivity) || prevActivity.IsZero())

		prevReceived := w.Metrics.MessagesReceived
		w.RecordMessage(false, 50)
		assert.Equal(t, prevReceived+1, w.Metrics.MessagesReceived)
		assert.Equal(t, int64(50), w.Metrics.BytesReceived)

		w.RecordPing()
		assert.False(t, w.Metrics.LastPingTime.IsZero())

		w.RecordPong()
		assert.False(t, w.Metrics.LastPongTime.IsZero())
		assert.True(t, w.Metrics.PingLatencyMs >= 0)
	})
}

func TestWebSocketSubscription_Keys(t *testing.T) {
	sub := &WebSocketSubscription{
		ConnectionID: "c1",
		Stream:       "home",
	}
	require.NoError(t, sub.UpdateKeys())
	assert.Equal(t, "SUB#home", sub.PK)
	assert.Equal(t, "CONN#c1", sub.SK)
	assert.Equal(t, "CONN#c1", sub.GSI1PK)
	assert.Equal(t, "STREAM#home", sub.GSI1SK)
	assert.Equal(t, sub.PK, sub.GetPK())
	assert.Equal(t, sub.SK, sub.GetSK())
	assert.Equal(t, MainTableName, sub.TableName())
}
