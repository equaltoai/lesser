package observability

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestEnhancedMetricsCollector_Basics(t *testing.T) {
	logger := zaptest.NewLogger(t)
	collector := NewEnhancedMetricsCollector(logger)

	collector.RecordLatency("op", 10*time.Millisecond)
	collector.RecordLatency("op", 20*time.Millisecond)
	collector.RecordLatency("op", 30*time.Millisecond)
	collector.RecordError("boom", 500)
	collector.RecordError("bad_request", 400)

	percentiles := collector.GetLatencyPercentiles()
	assert.Greater(t, percentiles.P95, 0.0)

	errorRates := collector.GetErrorRates()
	assert.Greater(t, errorRates.Total, int64(0))
	assert.Equal(t, int64(1), errorRates.By5xx)
	assert.Equal(t, int64(1), errorRates.By4xx)

	collector.RecordCacheHit(true, "primary")
	collector.RecordCacheHit(false, "primary")

	current := collector.GetCurrentMetrics()
	require.NotEmpty(t, current)
	assert.Contains(t, current, "error_rate")
	assert.Contains(t, current, "latency_p95")
	assert.Contains(t, current, "uptime")

	raw, err := collector.GetMetricsJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, raw)
}

func TestEnhancedMetricsCollector_DynamoDBCapacityAndMiddleware(t *testing.T) {
	logger := zaptest.NewLogger(t)
	collector := NewEnhancedMetricsCollector(logger)

	collector.RecordDynamoDBCapacity("GetItem", 1.5, 2.5)
	assert.Equal(t, 1.5, collector.capacityMetrics.ConsumedReadCapacity)
	assert.Equal(t, 2.5, collector.capacityMetrics.ConsumedWriteCapacity)

	found := false
	for _, metric := range collector.metrics {
		if metric.Name == "dynamodb_consumed_capacity" {
			found = true
			assert.Equal(t, 4.0, metric.Value)
		}
	}
	require.True(t, found)

	called := false
	wrapped := MetricsMiddleware(collector)(func() { called = true })
	wrapped()
	assert.True(t, called)
	assert.GreaterOrEqual(t, len(collector.requestLatencies), 1)
}

func TestEnhancedMetricsCollector_FlushAndReset(t *testing.T) {
	logger := zaptest.NewLogger(t)
	collector := NewEnhancedMetricsCollector(logger)

	collector.RecordLatency("op", 10*time.Millisecond) // critical
	collector.RecordError("boom", 500)                 // high

	collector.lastFlush = time.Now().Add(-collector.flushInterval).Add(-time.Second)
	assert.True(t, collector.ShouldFlush())

	collector.MarkFlushed()
	assert.False(t, collector.ShouldFlush())

	collector.Reset()
	for _, metric := range collector.metrics {
		assert.Equal(t, MetricLevelCritical, metric.Level)
	}
	assert.Len(t, collector.requestLatencies, 0)
}

func TestGetGlobalMetricsCollector_Singleton(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Reset global state to keep this test hermetic.
	globalMetricsCollector = nil
	once = sync.Once{}

	first := GetGlobalMetricsCollector(logger)
	second := GetGlobalMetricsCollector(logger)
	assert.Same(t, first, second)
	require.NotNil(t, first)
}
