package observability

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type historicalMetricsReaderStub struct {
	metrics []*models.MetricRecord
	err     error
}

func (s *historicalMetricsReaderStub) GetMetricsByService(_ context.Context, _ string, _, _ time.Time) ([]*models.MetricRecord, error) {
	return s.metrics, s.err
}

func TestLatencyAggregator_StartStopAndStats(t *testing.T) {
	logger := zaptest.NewLogger(t)

	la := NewLatencyAggregator(logger, nil, WithAggregateInterval(time.Hour))
	la.Start()
	la.Start() // idempotent
	la.Stop()
	la.Stop() // idempotent

	_, err := la.GetCurrentStats("missing", "svc")
	require.Error(t, err)

	la = NewLatencyAggregator(logger, nil, WithAggregateInterval(time.Hour))
	la.RecordLatency("op", "svc", 10*time.Millisecond)
	stats, err := la.GetCurrentStats("op", "svc")
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Count)

	agg, err := la.GetAggregatedStats("svc", time.Hour)
	require.NoError(t, err)
	require.Contains(t, agg, "op")
}

func TestLatencyAggregator_CleanupRemovesOldAndExcessBuckets(t *testing.T) {
	logger := zaptest.NewLogger(t)
	la := NewLatencyAggregator(logger, nil, WithMaxBuckets(1), WithRetentionPeriod(time.Hour))

	now := time.Now()

	// Add a stale bucket (will be removed by retention cutoff).
	la.buckets["stale"] = &LatencyBucket{Operation: "op", Service: "svc", WindowStart: now.Add(-2 * time.Hour), WindowEnd: now.Add(-2*time.Hour + time.Minute)}

	// Add multiple fresh buckets to trigger maxBuckets eviction.
	la.buckets["fresh1"] = &LatencyBucket{Operation: "op", Service: "svc", WindowStart: now.Add(-50 * time.Minute), WindowEnd: now.Add(-49 * time.Minute)}
	la.buckets["fresh2"] = &LatencyBucket{Operation: "op2", Service: "svc", WindowStart: now.Add(-40 * time.Minute), WindowEnd: now.Add(-39 * time.Minute)}
	la.buckets["fresh3"] = &LatencyBucket{Operation: "op3", Service: "svc", WindowStart: now.Add(-30 * time.Minute), WindowEnd: now.Add(-29 * time.Minute)}

	la.cleanup()
	assert.LessOrEqual(t, len(la.buckets), la.maxBuckets)
}

func TestLatencyAggregator_AggregateAndFlushAndFlushBucket(t *testing.T) {
	logger := zaptest.NewLogger(t)
	la := NewLatencyAggregator(logger, nil, WithAggregateInterval(10*time.Millisecond))

	// Old bucket should be flushed and removed.
	oldStart := time.Now().Add(-100 * time.Millisecond)
	oldBucket := la.createBucket("op", "svc", oldStart)
	oldBucket.addMeasurement(10)
	la.buckets[la.getBucketKey("op", "svc", oldStart)] = oldBucket

	la.aggregateAndFlush()
	assert.Empty(t, la.buckets)

	// flushBucket uses DefaultMetricsRecorder path.
	la2 := NewLatencyAggregator(logger, nil)
	var recorded []*models.MetricRecord
	recorder := NewDefaultMetricsRecorder(func(_ context.Context, metric *models.MetricRecord) error {
		recorded = append(recorded, metric)
		return nil
	}, "svc")
	la2.metricsRecorder = recorder
	la2.flushBucket(oldBucket)
	require.NotEmpty(t, recorded)

	// Error path in createMetricFn.
	la2.metricsRecorder = NewDefaultMetricsRecorder(func(_ context.Context, _ *models.MetricRecord) error {
		return errors.New("boom")
	}, "svc")
	la2.flushBucket(oldBucket)
}

func TestLatencyAggregator_TrendsWithMemoryAndHistoricalData(t *testing.T) {
	logger := zaptest.NewLogger(t)
	la := NewLatencyAggregator(logger, nil, WithAggregateInterval(time.Minute))

	start := time.Now().Add(-10 * time.Minute)
	end := time.Now()

	// Two buckets so trend analysis has sufficient points.
	b1 := la.createBucket("op", "svc", start.Add(2*time.Minute))
	b1.addMeasurement(10)
	b2 := la.createBucket("op", "svc", start.Add(4*time.Minute))
	b2.addMeasurement(20)
	la.buckets["b1"] = b1
	la.buckets["b2"] = b2

	// Historical metric is mixed in and sorted.
	la.metricsReader = &historicalMetricsReaderStub{
		metrics: []*models.MetricRecord{
			{
				MetricType:  "latency_aggregated",
				ServiceName: "svc",
				Timestamp:   start.Add(1 * time.Minute),
				Count:       2,
				Sum:         40,
				P50:         10,
				P95:         20,
				P99:         30,
				Dimensions:  map[string]string{"operation": "op"},
			},
			{
				MetricType:  "other",
				ServiceName: "svc",
				Timestamp:   start.Add(3 * time.Minute),
				Count:       1,
				Sum:         1,
				Dimensions:  map[string]string{"operation": "op"},
			},
		},
	}

	trend, err := la.GetLatencyTrend(context.Background(), "op", "svc", start, end, time.Minute)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(trend.DataPoints), 2)
	assert.NotEmpty(t, trend.TrendAnalysis.TrendDirection)

	// Error retrieving historical metrics is tolerated.
	la.metricsReader = &historicalMetricsReaderStub{err: errors.New("boom")}
	_, err = la.GetLatencyTrend(context.Background(), "op", "svc", start, end, time.Minute)
	require.NoError(t, err)

	// Insufficient data path.
	la2 := NewLatencyAggregator(logger, nil)
	trend, err = la2.GetLatencyTrend(context.Background(), "op", "svc", start, end, time.Minute)
	require.NoError(t, err)
	assert.Equal(t, "insufficient_data", trend.TrendAnalysis.TrendDirection)
}

func TestLatencyAggregator_DataPointHelpers(t *testing.T) {
	logger := zaptest.NewLogger(t)
	la := NewLatencyAggregator(logger, nil)

	metric := &models.MetricRecord{
		Timestamp:  time.Now(),
		Count:      0,
		Sum:        0,
		P50:        0,
		P95:        1,
		P99:        2,
		Dimensions: map[string]string{"operation": "op"},
		MetricType: "latency_aggregated",
	}

	assert.True(t, la.isRelevantHistoricalMetric(metric, "op"))
	assert.False(t, la.isRelevantHistoricalMetric(metric, "other"))

	dp := la.convertMetricToDataPoint(metric)
	assert.Equal(t, 0.0, dp.Average)
	assert.Contains(t, dp.Percentiles, "p95")
	assert.Contains(t, dp.Percentiles, "p99")
}
