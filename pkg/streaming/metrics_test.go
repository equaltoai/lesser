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

func TestMetricsCollector_StartStopAndCollect(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()

	mc := NewMetricsCollector(repo, zap.NewNop(), &MetricsCollectorConfig{
		CollectionInterval: time.Hour, // avoid ticker firing during test
		RetentionPeriod:    time.Hour,
	})

	require.False(t, mc.IsCollecting())
	require.NoError(t, mc.Start(context.Background()))
	require.True(t, mc.IsCollecting())
	require.Error(t, mc.Start(context.Background()))

	require.NoError(t, mc.Stop())
	require.False(t, mc.IsCollecting())
	require.NoError(t, mc.Stop())
}

func TestMetricsCollector_CollectAndSummaries(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()

	mc := NewMetricsCollector(repo, zap.NewNop(), &MetricsCollectorConfig{
		CollectionInterval: time.Second,
		RetentionPeriod:    time.Hour,
	})

	repo.On("GetConnectionPool", mock.Anything).Return(map[string]interface{}{"max": 1}, nil).Once()

	connected := models.WebSocketConnection{
		ConnectionID: "c1",
		State:        models.ConnectionStateConnected,
		Established:  time.Now().Add(-10 * time.Minute),
		Metrics: models.ConnectionMetrics{
			MessagesReceived:  1,
			MessagesSent:      1,
			BytesReceived:     10,
			BytesSent:         20,
			PingLatencyMs:     150,
			ErrorCount:        1,
			LastError:         "x",
			ConnectionQuality: 0.8,
		},
	}

	idle := models.WebSocketConnection{
		ConnectionID: "c2",
		State:        models.ConnectionStateIdle,
		Established:  time.Now().Add(-5 * time.Minute),
		Metrics: models.ConnectionMetrics{
			MessagesReceived:  2,
			MessagesSent:      3,
			BytesReceived:     50,
			BytesSent:         60,
			PingLatencyMs:     0,
			ErrorCount:        0,
			ConnectionQuality: 0.6,
		},
	}

	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateConnecting).Return([]models.WebSocketConnection{}, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateConnected).Return([]models.WebSocketConnection{connected}, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateIdle).Return([]models.WebSocketConnection{idle}, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateClosing).Return([]models.WebSocketConnection{}, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateClosed).Return([]models.WebSocketConnection{}, nil).Once()
	repo.On("GetConnectionsByState", mock.Anything, models.ConnectionStateError).Return(nil, errors.New("boom")).Once()

	require.NoError(t, mc.collectMetrics(context.Background()))

	current := mc.GetCurrentMetrics()
	assert.Equal(t, int64(2), current.TotalConnections)
	assert.Equal(t, int64(2), current.ActiveConnections)
	assert.Equal(t, int64(7), current.TotalMessages)
	assert.Equal(t, int64(140), current.TotalBytes)
	assert.NotEmpty(t, current.ConnectionsByState)
	assert.True(t, current.MessageRate > 0)
	assert.True(t, current.ByteRate > 0)
	assert.True(t, current.AverageQualityScore > 0)

	perf := mc.GetPerformanceMetrics()
	assert.True(t, perf.AverageConnectionDuration > 0)
	assert.True(t, perf.MedianConnectionDuration > 0)
	assert.True(t, perf.P95ConnectionDuration > 0)
	assert.True(t, perf.ConnectionSuccessRate > 0)
	assert.True(t, perf.MessageDeliveryRate > 0)
	assert.True(t, perf.AverageQualityScore > 0)

	summary := mc.GetMetricsSummary()
	assert.NotNil(t, summary["current"])
	assert.NotNil(t, summary["performance"])
	assert.NotNil(t, summary["collection"])
}

func TestMetricsCollector_PerformanceHelpers_EmptyData(t *testing.T) {
	mc := NewMetricsCollector(testmocks.NewMockStreamingConnectionRepository(), zap.NewNop(), &MetricsCollectorConfig{
		CollectionInterval: time.Second,
		RetentionPeriod:    time.Hour,
	})

	avg, median, p95, p99 := mc.calculateDurationStats()
	assert.Equal(t, time.Duration(0), avg)
	assert.Equal(t, time.Duration(0), median)
	assert.Equal(t, time.Duration(0), p95)
	assert.Equal(t, time.Duration(0), p99)

	avgRate, peak := mc.calculateMessageRateStats()
	assert.Equal(t, float64(0), avgRate)
	assert.Equal(t, float64(0), peak)

	assert.Equal(t, float64(1.0), mc.calculateConnectionSuccessRate())
	assert.Equal(t, float64(1.0), mc.calculateMessageDeliveryRate())
}

func TestDefaultMetricsCollectorConfig(t *testing.T) {
	cfg := DefaultMetricsCollectorConfig()
	require.NotNil(t, cfg)
	assert.True(t, cfg.CollectionInterval > 0)
	assert.True(t, cfg.RetentionPeriod > 0)
}
