package observability

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestPerformanceOptimization_TrackAndReport(t *testing.T) {
	logger := zaptest.NewLogger(t)
	po := NewPerformanceOptimization(logger)

	po.TrackOperation("op", 2*time.Millisecond, 100*time.Millisecond)
	po.TrackOperation("op", time.Millisecond, 200*time.Millisecond)

	metrics := po.GetMetrics()
	assert.Len(t, metrics, 1)
	assert.Equal(t, int64(2), metrics["op"].CallCount)
	assert.NotZero(t, metrics["op"].LastUpdated)

	report := po.GetPerformanceReport()
	assert.Equal(t, 1, report["total_operations"])
	assert.Equal(t, int64(2), report["total_calls"])
	assert.Contains(t, report, "violations")
}

func TestPerformanceOptimization_ValidatePerformanceTargets(t *testing.T) {
	logger := zaptest.NewLogger(t)
	po := NewPerformanceOptimization(logger)

	po.TrackOperation("fast", 100*time.Microsecond, 20*time.Millisecond)
	assert.Empty(t, po.ValidatePerformanceTargets())

	po.TrackOperation("slow", 5*time.Millisecond, time.Millisecond)
	assert.NotEmpty(t, po.ValidatePerformanceTargets())
}

func TestPerformanceOptimization_LogPerformanceSummary_NoMetrics(t *testing.T) {
	logger := zaptest.NewLogger(t)
	po := NewPerformanceOptimization(logger)
	po.LogPerformanceSummary()
}
