package testing

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCalculatePercentile_BoundsAndEmpty(t *testing.T) {
	t.Parallel()

	require.Equal(t, time.Duration(0), calculatePercentile(nil, 50))

	times := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second}
	require.Equal(t, 2*time.Second, calculatePercentile(times, 50))
	require.Equal(t, 2*time.Second, calculatePercentile(times, 99))
	require.Equal(t, 3*time.Second, calculatePercentile(times, 200))
}

func TestPerformanceResults_CalculateStats_ComputesExpectedAggregates(t *testing.T) {
	t.Parallel()

	start := time.Now()
	results := &PerformanceResults{
		TestName:    "x",
		Iterations:  3,
		Concurrency: 1,
		StartTime:   start,
		EndTime:     start.Add(100 * time.Millisecond),
		Metrics: []PerformanceMetrics{
			{ExecutionTime: 3 * time.Millisecond, MemoryUsage: 10, AllocCount: 1, GCCount: 0},
			{ExecutionTime: 1 * time.Millisecond, MemoryUsage: 30, AllocCount: 2, GCCount: 1},
			{ExecutionTime: 2 * time.Millisecond, MemoryUsage: 20, AllocCount: 3, GCCount: 0},
		},
	}

	results.calculateStats()

	require.Equal(t, 1*time.Millisecond, results.MinTime)
	require.Equal(t, 3*time.Millisecond, results.MaxTime)
	require.Equal(t, int64(10), results.MinMemory)
	require.Equal(t, int64(30), results.MaxMemory)
	require.NotZero(t, results.AvgTime)
	require.Greater(t, results.Throughput, 0.0)
	require.Equal(t, uint64(6), results.TotalAllocs)
	require.Equal(t, uint32(1), results.TotalGCs)
}

func TestPerformanceTestSuite_RunPerformanceTest_SequentialAndConcurrent(t *testing.T) {
	t.Parallel()

	suite := NewPerformanceTestSuite(t)
	seq := suite.RunPerformanceTest(&PerformanceTest{
		Name:        "seq",
		Iterations:  2,
		Concurrency: 1,
		WarmupRuns:  1,
		TestFunc: func() error {
			return nil
		},
	})
	require.Len(t, seq.Metrics, 2)

	con := suite.RunPerformanceTest(&PerformanceTest{
		Name:        "con",
		Iterations:  3,
		Concurrency: 2,
		WarmupRuns:  0,
		TestFunc: func() error {
			return nil
		},
	})
	require.Len(t, con.Metrics, 3)
}

func TestPerformanceTestSuite_MeasureColdStart(t *testing.T) {
	t.Parallel()

	suite := NewPerformanceTestSuite(t)
	results := suite.MeasureColdStart(&ColdStartTest{
		Iterations:   2,
		MemorySize:   128,
		ExpectedTime: 0,
		InitFunc: func() error {
			return nil
		},
		HandlerFunc: func() error {
			return nil
		},
	})
	require.Len(t, results.TotalTimes, 2)
}

func TestMeasureMemoryUsage_TracksAllocations(t *testing.T) {
	t.Parallel()

	usage := MeasureMemoryUsage(func() {
		_ = make([]byte, 1024)
	})
	require.NotNil(t, usage)
	require.GreaterOrEqual(t, usage.MaxHeapSize, int64(0))
}

func TestPerformanceTestSuite_RunLoadTest_CollectsRequestsAndErrors(t *testing.T) {
	suite := NewPerformanceTestSuite(t)

	ok := suite.RunLoadTest(&LoadTest{
		Duration:    30 * time.Millisecond,
		Concurrency: 1,
		TestFunc: func() error {
			return nil
		},
	})
	require.Greater(t, ok.RequestCount, int64(0))
	require.Equal(t, int64(0), ok.ErrorCount)

	bad := suite.RunLoadTest(&LoadTest{
		Duration:    30 * time.Millisecond,
		Concurrency: 1,
		TestFunc: func() error {
			return errors.New("boom")
		},
	})
	require.Greater(t, bad.RequestCount, int64(0))
	require.Greater(t, bad.ErrorCount, int64(0))
	require.Greater(t, bad.ErrorRate, 0.0)
}

func TestPerformanceAssertions_ExerciseBranches(t *testing.T) {
	t.Parallel()

	results := &PerformanceResults{
		AvgTime:     1 * time.Millisecond,
		P95Time:     2 * time.Millisecond,
		MaxMemory:   10,
		Throughput:  1.0,
		Iterations:  1,
		Concurrency: 1,
	}

	AssertPerformance(t, results, &PerformanceRequirements{
		MaxAvgTime:    10 * time.Millisecond,
		MaxP95Time:    10 * time.Millisecond,
		MaxMemory:     100,
		MinThroughput: 0.1,
	})

	AssertColdStartPerformance(t, &ColdStartResults{
		AvgTotalTime: 10 * time.Millisecond,
		MaxTotalTime: 10 * time.Millisecond,
	}, 1*time.Second)
}

