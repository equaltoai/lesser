package monitoring

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingRecorder captures metric calls for test assertions
type recordingRecorder struct {
	mu            sync.Mutex
	latencyCalls  []latencyCall
	errorCalls    []errorCall
	capacityCalls []capacityCall
}

type latencyCall struct {
	operation string
	latencyMs float64
}

type errorCall struct {
	operation string
	errorType string
}

type capacityCall struct {
	tableName     string
	operation     string
	readCapacity  float64
	writeCapacity float64
}

func newRecordingRecorder() *recordingRecorder {
	return &recordingRecorder{}
}

func (r *recordingRecorder) RecordLatency(_ context.Context, operation string, latencyMs float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.latencyCalls = append(r.latencyCalls, latencyCall{operation: operation, latencyMs: latencyMs})
	return nil
}

func (r *recordingRecorder) RecordError(_ context.Context, operation string, errorType string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errorCalls = append(r.errorCalls, errorCall{operation: operation, errorType: errorType})
	return nil
}

func (r *recordingRecorder) RecordDynamoDBConsumedCapacity(_ context.Context, tableName string, operation string, readCapacity, writeCapacity float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capacityCalls = append(r.capacityCalls, capacityCall{
		tableName:     tableName,
		operation:     operation,
		readCapacity:  readCapacity,
		writeCapacity: writeCapacity,
	})
	return nil
}

// =============================================================================
// QueryAnalyzer.updateStats Tests
// =============================================================================

func TestUpdateStats_MinMaxTotalErrorCounts(t *testing.T) {
	tests := []struct {
		name  string
		calls []struct {
			duration time.Duration
			hasError bool
		}
		wantExecCount  int64
		wantMinTime    time.Duration
		wantMaxTime    time.Duration
		wantTotalTime  time.Duration
		wantErrorCount int64
	}{
		{
			name: "single call no error",
			calls: []struct {
				duration time.Duration
				hasError bool
			}{
				{100 * time.Millisecond, false},
			},
			wantExecCount:  1,
			wantMinTime:    100 * time.Millisecond,
			wantMaxTime:    100 * time.Millisecond,
			wantTotalTime:  100 * time.Millisecond,
			wantErrorCount: 0,
		},
		{
			name: "multiple calls varying durations",
			calls: []struct {
				duration time.Duration
				hasError bool
			}{
				{50 * time.Millisecond, false},
				{200 * time.Millisecond, false},
				{100 * time.Millisecond, false},
			},
			wantExecCount:  3,
			wantMinTime:    50 * time.Millisecond,
			wantMaxTime:    200 * time.Millisecond,
			wantTotalTime:  350 * time.Millisecond,
			wantErrorCount: 0,
		},
		{
			name: "mixed success and errors",
			calls: []struct {
				duration time.Duration
				hasError bool
			}{
				{100 * time.Millisecond, false},
				{150 * time.Millisecond, true},
				{50 * time.Millisecond, true},
				{200 * time.Millisecond, false},
			},
			wantExecCount:  4,
			wantMinTime:    50 * time.Millisecond,
			wantMaxTime:    200 * time.Millisecond,
			wantTotalTime:  500 * time.Millisecond,
			wantErrorCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := newRecordingRecorder()
			qa := NewQueryAnalyzer(recorder, 1*time.Second)

			pattern := "TestQuery"
			for _, call := range tt.calls {
				qa.updateStats(pattern, call.duration, call.hasError)
			}

			stats := qa.GetQueryStats()
			require.Contains(t, stats, pattern)

			s := stats[pattern]
			assert.Equal(t, tt.wantExecCount, s.ExecutionCount)
			assert.Equal(t, tt.wantMinTime, s.MinTime)
			assert.Equal(t, tt.wantMaxTime, s.MaxTime)
			assert.Equal(t, tt.wantTotalTime, s.TotalTime)
			assert.Equal(t, tt.wantErrorCount, s.ErrorCount)
		})
	}
}

func TestUpdateStats_MultiplePatterns(t *testing.T) {
	recorder := newRecordingRecorder()
	qa := NewQueryAnalyzer(recorder, 1*time.Second)

	qa.updateStats("PatternA", 100*time.Millisecond, false)
	qa.updateStats("PatternB", 200*time.Millisecond, true)
	qa.updateStats("PatternA", 50*time.Millisecond, false)

	stats := qa.GetQueryStats()
	require.Len(t, stats, 2)

	assert.Equal(t, int64(2), stats["PatternA"].ExecutionCount)
	assert.Equal(t, int64(1), stats["PatternB"].ExecutionCount)
	assert.Equal(t, int64(1), stats["PatternB"].ErrorCount)
}

// =============================================================================
// SlowQuery Logging Tests
// =============================================================================

func TestRecordSlowQuery_MaxQueriesTrimming(t *testing.T) {
	recorder := newRecordingRecorder()
	qa := NewQueryAnalyzer(recorder, 10*time.Millisecond)

	// Override maxQueries for testing
	qa.slowQueryLog.maxQueries = 5

	// Add 7 queries - should only keep last 5
	for i := 0; i < 7; i++ {
		qa.recordSlowQuery(SlowQuery{
			QueryPattern: "Query",
			Duration:     time.Duration(i) * time.Millisecond,
			Timestamp:    time.Now(),
		})
	}

	queries := qa.GetSlowQueries(10) // Request more than available
	require.Len(t, queries, 5)

	// Should have durations 2,3,4,5,6 (oldest 0,1 trimmed)
	assert.Equal(t, 2*time.Millisecond, queries[0].Duration)
	assert.Equal(t, 6*time.Millisecond, queries[4].Duration)
}

func TestGetSlowQueries_LimitOrdering(t *testing.T) {
	recorder := newRecordingRecorder()
	qa := NewQueryAnalyzer(recorder, 10*time.Millisecond)

	// Add 5 queries
	for i := 1; i <= 5; i++ {
		qa.recordSlowQuery(SlowQuery{
			QueryPattern: "Query",
			Duration:     time.Duration(i*100) * time.Millisecond,
			Timestamp:    time.Now(),
		})
	}

	// Request only 3 - should get the 3 most recent
	queries := qa.GetSlowQueries(3)
	require.Len(t, queries, 3)

	// Most recent 3 have durations 300ms, 400ms, 500ms
	assert.Equal(t, 300*time.Millisecond, queries[0].Duration)
	assert.Equal(t, 400*time.Millisecond, queries[1].Duration)
	assert.Equal(t, 500*time.Millisecond, queries[2].Duration)
}

func TestGetSlowQueries_EmptyLog(t *testing.T) {
	recorder := newRecordingRecorder()
	qa := NewQueryAnalyzer(recorder, 10*time.Millisecond)

	queries := qa.GetSlowQueries(5)
	assert.Empty(t, queries)
}

func TestGetSlowQueries_ZeroOrNegativeLimit(t *testing.T) {
	recorder := newRecordingRecorder()
	qa := NewQueryAnalyzer(recorder, 10*time.Millisecond)

	qa.recordSlowQuery(SlowQuery{QueryPattern: "Q1", Duration: 100 * time.Millisecond})
	qa.recordSlowQuery(SlowQuery{QueryPattern: "Q2", Duration: 200 * time.Millisecond})

	// Zero limit should return all
	queries := qa.GetSlowQueries(0)
	assert.Len(t, queries, 2)

	// Negative limit should return all
	queries = qa.GetSlowQueries(-1)
	assert.Len(t, queries, 2)
}

// =============================================================================
// QueryExecution.End Tests
// =============================================================================

func TestQueryExecutionEnd_SlowQueryPath(t *testing.T) {
	recorder := newRecordingRecorder()
	threshold := 100 * time.Millisecond
	qa := NewQueryAnalyzer(recorder, threshold)

	qe := &QueryExecution{
		analyzer:     qa,
		queryPattern: "SlowQuery",
		startTime:    time.Now().Add(-200 * time.Millisecond), // 200ms ago = slow
		parameters:   map[string]any{"key": "value"},
	}

	qe.End(context.Background(), 5.0, 2.0, nil)

	// Should have recorded slow query
	queries := qa.GetSlowQueries(10)
	require.Len(t, queries, 1)
	assert.Equal(t, "SlowQuery", queries[0].QueryPattern)
	assert.Equal(t, 5.0, queries[0].ConsumedRCU)
	assert.Equal(t, 2.0, queries[0].ConsumedWCU)

	// Should have recorded latency metric
	require.Len(t, recorder.latencyCalls, 1)
	assert.Equal(t, "DynamoDBQuery.SlowQuery", recorder.latencyCalls[0].operation)

	// Should have recorded capacity
	require.Len(t, recorder.capacityCalls, 1)
	assert.Equal(t, "main", recorder.capacityCalls[0].tableName)
	assert.Equal(t, "SlowQuery", recorder.capacityCalls[0].operation)

	// Should have recorded SlowQuery error metric
	require.Len(t, recorder.errorCalls, 1)
	assert.Equal(t, "DynamoDBQuery.SlowQuery", recorder.errorCalls[0].operation)
	assert.Equal(t, "SlowQuery", recorder.errorCalls[0].errorType)
}

func TestQueryExecutionEnd_FastQueryPath(t *testing.T) {
	recorder := newRecordingRecorder()
	threshold := 100 * time.Millisecond
	qa := NewQueryAnalyzer(recorder, threshold)

	qe := &QueryExecution{
		analyzer:     qa,
		queryPattern: "FastQuery",
		startTime:    time.Now().Add(-10 * time.Millisecond), // 10ms ago = fast
		parameters:   nil,
	}

	qe.End(context.Background(), 1.0, 0.5, nil)

	// Should NOT have recorded slow query
	queries := qa.GetSlowQueries(10)
	assert.Empty(t, queries)

	// Should still have recorded latency and capacity
	require.Len(t, recorder.latencyCalls, 1)
	require.Len(t, recorder.capacityCalls, 1)

	// Should NOT have recorded error (no slow query error, no actual error)
	assert.Empty(t, recorder.errorCalls)
}

func TestQueryExecutionEnd_ErrorPath(t *testing.T) {
	recorder := newRecordingRecorder()
	qa := NewQueryAnalyzer(recorder, 100*time.Millisecond)

	qe := &QueryExecution{
		analyzer:     qa,
		queryPattern: "FailingQuery",
		startTime:    time.Now().Add(-10 * time.Millisecond), // Fast but failing
		parameters:   nil,
	}

	qe.End(context.Background(), 0, 0, assert.AnError)

	// Stats should have error count
	stats := qa.GetQueryStats()
	require.Contains(t, stats, "FailingQuery")
	assert.Equal(t, int64(1), stats["FailingQuery"].ErrorCount)

	// Should have recorded QueryError
	require.Len(t, recorder.errorCalls, 1)
	assert.Equal(t, "DynamoDBQuery.FailingQuery", recorder.errorCalls[0].operation)
	assert.Equal(t, "QueryError", recorder.errorCalls[0].errorType)
}

func TestQueryExecutionEnd_NoCapacityWhenZero(t *testing.T) {
	recorder := newRecordingRecorder()
	qa := NewQueryAnalyzer(recorder, 100*time.Millisecond)

	qe := &QueryExecution{
		analyzer:     qa,
		queryPattern: "Query",
		startTime:    time.Now(),
		parameters:   nil,
	}

	qe.End(context.Background(), 0, 0, nil)

	// Should NOT record capacity when both are zero
	assert.Empty(t, recorder.capacityCalls)
}

// =============================================================================
// Other QueryAnalyzer Tests
// =============================================================================

func TestGetAverageQueryTime(t *testing.T) {
	recorder := newRecordingRecorder()
	qa := NewQueryAnalyzer(recorder, 1*time.Second)

	// No data yet
	avg, ok := qa.GetAverageQueryTime("Unknown")
	assert.False(t, ok)
	assert.Equal(t, time.Duration(0), avg)

	// Add some data
	qa.updateStats("Query", 100*time.Millisecond, false)
	qa.updateStats("Query", 200*time.Millisecond, false)
	qa.updateStats("Query", 300*time.Millisecond, false)

	avg, ok = qa.GetAverageQueryTime("Query")
	assert.True(t, ok)
	assert.Equal(t, 200*time.Millisecond, avg)
}

func TestResetStats(t *testing.T) {
	recorder := newRecordingRecorder()
	qa := NewQueryAnalyzer(recorder, 1*time.Second)

	qa.updateStats("Query1", 100*time.Millisecond, false)
	qa.updateStats("Query2", 200*time.Millisecond, false)

	require.Len(t, qa.GetQueryStats(), 2)

	qa.ResetStats()

	assert.Empty(t, qa.GetQueryStats())
}

func TestDetectN1Queries(t *testing.T) {
	recorder := newRecordingRecorder()
	qa := NewQueryAnalyzer(recorder, 1*time.Second)

	// Non-N+1 pattern: few executions
	qa.updateStats("RareQuery", 10*time.Millisecond, false)
	qa.updateStats("RareQuery", 10*time.Millisecond, false)

	// N+1 pattern: many fast executions
	for i := 0; i < 15; i++ {
		qa.updateStats("N1Query", 10*time.Millisecond, false)
	}

	// Slow query (not N+1 even if many executions)
	for i := 0; i < 15; i++ {
		qa.updateStats("SlowButMany", 100*time.Millisecond, false)
	}

	patterns := qa.DetectN1Queries()
	require.Len(t, patterns, 1)
	assert.Equal(t, "N1Query", patterns[0].QueryPattern)
	assert.Equal(t, int64(15), patterns[0].ExecutionCount)
}

func TestAnalyzeQueryComplexity(t *testing.T) {
	tests := []struct {
		queryType      string
		filters        map[string]any
		limit          int
		wantComplexity int
	}{
		{"Query", nil, 10, 3},                            // base(1) + Query(2) + 0 filters
		{"Scan", nil, 10, 11},                            // base(1) + Scan(10) + 0 filters
		{"BatchGetItem", nil, 10, 4},                     // base(1) + BatchGetItem(3) + 0 filters
		{"Query", map[string]any{"a": 1, "b": 2}, 10, 5}, // base(1) + Query(2) + 2 filters
		{"Query", nil, 250, 5},                           // base(1) + Query(2) + limit/100(2)
	}

	for _, tt := range tests {
		t.Run(tt.queryType, func(t *testing.T) {
			complexity := AnalyzeQueryComplexity(tt.queryType, tt.filters, tt.limit)
			assert.Equal(t, tt.wantComplexity, complexity)
		})
	}
}
