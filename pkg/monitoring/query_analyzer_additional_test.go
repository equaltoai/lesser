package monitoring

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingRecorder struct{}

func (f failingRecorder) RecordLatency(context.Context, string, float64) error {
	return errors.New("latency failed")
}

func (f failingRecorder) RecordError(context.Context, string, string) error {
	return errors.New("error failed")
}

func (f failingRecorder) RecordDynamoDBConsumedCapacity(context.Context, string, string, float64, float64) error {
	return errors.New("capacity failed")
}

func TestQueryAnalyzerStartQueryAndEndRecorderFailures(t *testing.T) {
	qa := NewQueryAnalyzer(failingRecorder{}, 0)

	qe := qa.StartQuery("Query", map[string]any{"k": "v"})
	require.NotNil(t, qe)
	assert.Equal(t, "Query", qe.queryPattern)
	assert.Equal(t, "v", qe.parameters["k"])

	// Force a slow query to exercise slow-query metrics path.
	qe.startTime = time.Now().Add(-10 * time.Millisecond)
	qe.End(context.Background(), 1, 1, errors.New("boom"))

	stats := qa.GetQueryStats()
	require.Contains(t, stats, "Query")
	assert.Equal(t, int64(1), stats["Query"].ErrorCount)
}

func TestAnalyzeQueryPlan(t *testing.T) {
	plan := AnalyzeQueryPlan("table", "Scan", "", nil)
	assert.Equal(t, "Scan", plan.QueryType)
	assert.NotEmpty(t, plan.Warnings)

	plan = AnalyzeQueryPlan("table", "Query", "", nil)
	assert.Equal(t, "Query", plan.QueryType)
	assert.NotEmpty(t, plan.Warnings)
}
