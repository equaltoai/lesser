package monitoring

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestOptimizationTracker_BasicAccounting(t *testing.T) {
	t.Parallel()

	ot := NewOptimizationTracker(zap.NewNop())
	require.Equal(t, 0.0, ot.GetColdStartRatio())

	ot.TrackColdStart(context.Background())
	require.Equal(t, 1.0, ot.GetColdStartRatio())

	ot.TrackWarmStart(context.Background())
	require.Equal(t, 0.5, ot.GetColdStartRatio())

	ot.TrackLatency(context.Background(), "op", 10*time.Millisecond)
	ot.TrackDBQuery(context.Background(), "table", "query", 5*time.Millisecond)
}
