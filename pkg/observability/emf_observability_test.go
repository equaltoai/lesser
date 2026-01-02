package observability

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestEMFMetrics_PutMetric_AndFlush(t *testing.T) {
	logger := zaptest.NewLogger(t)

	emf := NewEMFMetrics(logger, "ns", "svc")
	require.True(t, emf.IsEnabled())

	emf.PutMetric("m", 1.25, "Count", map[string]string{
		"Operation": "op",
		"X":         "y",
	})

	require.Len(t, emf.metrics, 1)
	assert.Equal(t, "op", emf.metadata["Operation"])
	assert.Equal(t, "y", emf.metadata["X"])

	emf.SetProperty("x", "y") // short key exercises isMetricMetadata length checks
	dimSets := emf.buildDimensionSets()
	require.Len(t, dimSets, 1)
	assert.Contains(t, dimSets[0], "x")

	emf.Flush()
	assert.Empty(t, emf.metrics)
	assert.Empty(t, emf.metadata)

	// Flush on empty should be a no-op.
	emf.Flush()
}

func TestEMFMetrics_RecordLatency_DistributionAndSLA(t *testing.T) {
	logger := zaptest.NewLogger(t)
	emf := &EMFMetrics{
		logger:    logger,
		namespace: "ns",
		service:   "svc",
		metrics:   make(map[string]float64),
		metadata:  make(map[string]interface{}),
		enabled:   true,
	}

	// recordLatencyDistribution covers bucket classification and SLA compliance logic.
	buckets := []struct {
		durationMs float64
		expect     string
	}{
		{durationMs: 1, expect: "under_10ms"},
		{durationMs: 10, expect: "10ms_to_50ms"},
		{durationMs: 50, expect: "50ms_to_100ms"},
		{durationMs: 100, expect: "100ms_to_200ms"},
		{durationMs: 200, expect: "200ms_to_500ms"},
		{durationMs: 500, expect: "500ms_to_1s"},
		{durationMs: 1000, expect: "1s_to_2s"},
		{durationMs: 2000, expect: "2s_to_5s"},
		{durationMs: 5000, expect: "over_5s"},
	}

	for _, tc := range buckets {
		emf.metadata = make(map[string]interface{})
		emf.recordLatencyDistribution("op", tc.durationMs, map[string]string{
			"Operation": "op",
			"Service":   "svc",
		})
		assert.Equal(t, tc.expect, emf.metadata["LatencyBucket"])
	}

	emf.RecordLatency("op", 123*time.Millisecond)
	assert.NotEmpty(t, emf.metrics)
}

func TestEMFMetrics_QueueDepthBranches(t *testing.T) {
	logger := zaptest.NewLogger(t)
	emf := &EMFMetrics{
		logger:    logger,
		namespace: "ns",
		service:   "svc",
		metrics:   make(map[string]float64),
		metadata:  make(map[string]interface{}),
		enabled:   true,
	}

	emf.RecordQueueDepth("q", 10)
	emf.RecordQueueDepth("q", 2000)
	emf.RecordQueueDepth("q", 20000)
	assert.NotEmpty(t, emf.metrics)
}

func TestLatencyMetric_FinishPaths(t *testing.T) {
	logger := zaptest.NewLogger(t)
	emf := &EMFMetrics{
		logger:    logger,
		namespace: "ns",
		service:   "svc",
		metrics:   make(map[string]float64),
		metadata:  make(map[string]interface{}),
		enabled:   true,
	}

	lm := emf.StartLatencyTimer(nil, "op")
	lm.Start = time.Now().Add(-10 * time.Millisecond)
	lm.Finish(emf, true)
	lm.Start = time.Now().Add(-10 * time.Millisecond)
	lm.FinishWithError(emf, "boom")
	assert.NotEmpty(t, emf.metrics)
}

func TestEMFMetrics_EnableFlag(t *testing.T) {
	logger := zaptest.NewLogger(t)
	emf := &EMFMetrics{
		logger:    logger,
		namespace: "ns",
		service:   "svc",
		metrics:   make(map[string]float64),
		metadata:  make(map[string]interface{}),
		enabled:   false,
	}

	emf.PutMetric("m", 1, "Count", map[string]string{"k": "v"})
	assert.Empty(t, emf.metrics)

	emf.SetEnabled(true)
	assert.True(t, emf.IsEnabled())
	emf.PutMetric("m", 1, "Count", map[string]string{"k": "v"})
	assert.Len(t, emf.metrics, 1)
}

func TestEMFMetrics_OtherRecorders(t *testing.T) {
	logger := zaptest.NewLogger(t)
	emf := &EMFMetrics{
		logger:    logger,
		namespace: "ns",
		service:   "svc",
		metrics:   make(map[string]float64),
		metadata:  make(map[string]interface{}),
		enabled:   true,
	}

	emf.RecordThroughput("op", 123)
	emf.RecordCost("op", 1.23)
	emf.RecordBusinessMetric("ActiveUsers", 10, "Count", nil)
	emf.RecordBusinessMetric("ActiveUsers", 11, "Count", map[string]string{"Region": "us-east-1"})
	emf.RecordFederationMetric("inbox", "remote.example", true, 12.3)
	emf.RecordFederationMetric("inbox", "remote.example", false, 0)
	emf.RecordConcurrency("op", 5)
	emf.AddDimension("Region", "us-east-1")

	require.NotEmpty(t, emf.metrics)

	emf.enabled = false
	emf.SetProperty("k", "v")
}
