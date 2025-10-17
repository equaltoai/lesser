package performance

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"go.uber.org/zap"
)

func TestNewQueryTracker(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewQueryTracker(logger)

	if tracker == nil {
		t.Fatal("NewQueryTracker returned nil")
	}

	if tracker.logger != logger {
		t.Error("Logger not set correctly")
	}

	if tracker.queries == nil {
		t.Error("Queries map not initialized")
	}

	if tracker.maxQueries != 1000 {
		t.Errorf("Expected maxQueries 1000, got %d", tracker.maxQueries)
	}

	if tracker.slowThreshold != time.Second {
		t.Errorf("Expected slowThreshold 1s, got %v", tracker.slowThreshold)
	}
}

func TestRecordQuery(t *testing.T) {
	ctx := context.Background()
	tracker := NewQueryTracker(zap.NewNop())

	t.Run("Record single query", func(t *testing.T) {
		tracker.RecordQuery(ctx, "testQuery", 100*time.Millisecond, false)

		tracker.mu.RLock()
		stats, exists := tracker.queries["testQuery"]
		tracker.mu.RUnlock()

		if !exists {
			t.Fatal("Query not recorded")
		}

		if stats.Count != 1 {
			t.Errorf("Expected count 1, got %d", stats.Count)
		}

		if stats.ErrorCount != 0 {
			t.Errorf("Expected error count 0, got %d", stats.ErrorCount)
		}

		if len(stats.Durations) != 1 {
			t.Errorf("Expected 1 duration recorded, got %d", len(stats.Durations))
		}

		if stats.Durations[0] != 100*time.Millisecond {
			t.Errorf("Expected duration 100ms, got %v", stats.Durations[0])
		}
	})

	t.Run("Record query with error", func(t *testing.T) {
		tracker.RecordQuery(ctx, "errorQuery", 200*time.Millisecond, true)

		tracker.mu.RLock()
		stats, exists := tracker.queries["errorQuery"]
		tracker.mu.RUnlock()

		if !exists {
			t.Fatal("Error query not recorded")
		}

		if stats.ErrorCount != 1 {
			t.Errorf("Expected error count 1, got %d", stats.ErrorCount)
		}
	})

	t.Run("Record multiple executions", func(t *testing.T) {
		queryName := "multiQuery"
		for i := 0; i < 5; i++ {
			tracker.RecordQuery(ctx, queryName, time.Duration(i+1)*100*time.Millisecond, false)
		}

		tracker.mu.RLock()
		stats := tracker.queries[queryName]
		tracker.mu.RUnlock()

		if stats.Count != 5 {
			t.Errorf("Expected count 5, got %d", stats.Count)
		}

		if len(stats.Durations) != 5 {
			t.Errorf("Expected 5 durations recorded, got %d", len(stats.Durations))
		}

		expectedTotal := 100*time.Millisecond + 200*time.Millisecond + 300*time.Millisecond + 400*time.Millisecond + 500*time.Millisecond
		if stats.TotalDuration != expectedTotal {
			t.Errorf("Expected total duration %v, got %v", expectedTotal, stats.TotalDuration)
		}
	})

	t.Run("Rolling window for durations", func(t *testing.T) {
		queryName := "rollingQuery"
		// Record 101 queries to test rolling window (max is 100)
		for i := 0; i < 101; i++ {
			tracker.RecordQuery(ctx, queryName, time.Duration(i+1)*time.Millisecond, false)
		}

		tracker.mu.RLock()
		stats := tracker.queries[queryName]
		tracker.mu.RUnlock()

		if len(stats.Durations) != 100 {
			t.Errorf("Expected 100 durations (rolling window), got %d", len(stats.Durations))
		}

		// First duration should be removed (was 1ms), so first should now be 2ms
		if stats.Durations[0] != 2*time.Millisecond {
			t.Errorf("Expected first duration 2ms (after rolling), got %v", stats.Durations[0])
		}
	})

	t.Run("Empty query name", func(t *testing.T) {
		initialLen := len(tracker.queries)
		tracker.RecordQuery(ctx, "", 100*time.Millisecond, false)

		// Should not add empty query name due to validation
		tracker.mu.RLock()
		newLen := len(tracker.queries)
		tracker.mu.RUnlock()

		if newLen != initialLen {
			t.Error("Empty query name should not be recorded")
		}
	})
}

func TestGetSlowQueries(t *testing.T) {
	ctx := context.Background()
	tracker := NewQueryTracker(zap.NewNop())

	// Record some queries with different durations
	tracker.RecordQuery(ctx, "fastQuery1", 50*time.Millisecond, false)
	tracker.RecordQuery(ctx, "fastQuery2", 75*time.Millisecond, false)
	tracker.RecordQuery(ctx, "slowQuery1", 500*time.Millisecond, false)
	tracker.RecordQuery(ctx, "slowQuery2", 800*time.Millisecond, false)
	tracker.RecordQuery(ctx, "slowQuery3", 1200*time.Millisecond, false)

	t.Run("Get queries above 100ms threshold", func(t *testing.T) {
		slowQueries, err := tracker.GetSlowQueries(ctx, model.Duration(100*time.Millisecond))
		if err != nil {
			t.Fatalf("GetSlowQueries failed: %v", err)
		}

		// Should return slowQuery1, slowQuery2, slowQuery3
		if len(slowQueries) != 3 {
			t.Errorf("Expected 3 slow queries, got %d", len(slowQueries))
		}

		// Should be sorted by duration (slowest first)
		if len(slowQueries) > 0 && slowQueries[0].Query != "slowQuery3" {
			t.Errorf("Expected slowest query to be slowQuery3, got %s", slowQueries[0].Query)
		}
	})

	t.Run("Get queries above 1s threshold", func(t *testing.T) {
		slowQueries, err := tracker.GetSlowQueries(ctx, model.Duration(1*time.Second))
		if err != nil {
			t.Fatalf("GetSlowQueries failed: %v", err)
		}

		// Should return only slowQuery3
		if len(slowQueries) != 1 {
			t.Errorf("Expected 1 slow query, got %d", len(slowQueries))
		}

		if len(slowQueries) > 0 && slowQueries[0].Query != "slowQuery3" {
			t.Errorf("Expected slowQuery3, got %s", slowQueries[0].Query)
		}
	})

	t.Run("High threshold returns empty", func(t *testing.T) {
		slowQueries, err := tracker.GetSlowQueries(ctx, model.Duration(2*time.Second))
		if err != nil {
			t.Fatalf("GetSlowQueries failed: %v", err)
		}

		if len(slowQueries) != 0 {
			t.Errorf("Expected 0 slow queries with high threshold, got %d", len(slowQueries))
		}
	})
}

func TestCalculateP95(t *testing.T) {
	tracker := NewQueryTracker(zap.NewNop())

	t.Run("Empty durations", func(t *testing.T) {
		result := tracker.calculateP95([]time.Duration{})
		if result != 0 {
			t.Errorf("Expected 0 for empty slice, got %v", result)
		}
	})

	t.Run("Single duration", func(t *testing.T) {
		result := tracker.calculateP95([]time.Duration{100 * time.Millisecond})
		if result != 100*time.Millisecond {
			t.Errorf("Expected 100ms, got %v", result)
		}
	})

	t.Run("Multiple durations", func(t *testing.T) {
		durations := []time.Duration{
			100 * time.Millisecond,
			200 * time.Millisecond,
			300 * time.Millisecond,
			400 * time.Millisecond,
			500 * time.Millisecond,
			600 * time.Millisecond,
			700 * time.Millisecond,
			800 * time.Millisecond,
			900 * time.Millisecond,
			1000 * time.Millisecond,
		}

		result := tracker.calculateP95(durations)
		// P95 of 10 items should be index 9 (0.95 * 10 = 9.5 -> 9)
		// So it should be 1000ms
		if result != 1000*time.Millisecond {
			t.Errorf("Expected 1000ms for P95, got %v", result)
		}
	})

	t.Run("Unsorted durations", func(t *testing.T) {
		durations := []time.Duration{
			500 * time.Millisecond,
			100 * time.Millisecond,
			300 * time.Millisecond,
			200 * time.Millisecond,
			400 * time.Millisecond,
		}

		result := tracker.calculateP95(durations)
		// P95 of 5 items should be index 4 (0.95 * 5 = 4.75 -> 4)
		// After sorting: [100, 200, 300, 400, 500], so P95 is 500ms
		if result != 500*time.Millisecond {
			t.Errorf("Expected 500ms for P95, got %v", result)
		}
	})
}

func TestGetAllQueries(t *testing.T) {
	ctx := context.Background()
	tracker := NewQueryTracker(zap.NewNop())

	// Record some queries
	tracker.RecordQuery(ctx, "query1", 100*time.Millisecond, false)
	tracker.RecordQuery(ctx, "query2", 200*time.Millisecond, false)
	tracker.RecordQuery(ctx, "query3", 300*time.Millisecond, true)

	queries := tracker.GetAllQueries()

	if len(queries) != 3 {
		t.Errorf("Expected 3 queries, got %d", len(queries))
	}

	// Check that all queries are present
	queryNames := make(map[string]bool)
	for _, q := range queries {
		queryNames[q.Query] = true

		if q.Count != 1 {
			t.Errorf("Expected count 1 for query %s, got %d", q.Query, q.Count)
		}

		if q.Query == "query3" && q.ErrorCount != 1 {
			t.Errorf("Expected error count 1 for query3, got %d", q.ErrorCount)
		}
	}

	if !queryNames["query1"] || !queryNames["query2"] || !queryNames["query3"] {
		t.Error("Not all queries returned by GetAllQueries")
	}
}

func TestCleanup(t *testing.T) {
	ctx := context.Background()
	tracker := NewQueryTracker(zap.NewNop())

	// Record a query and manually set its LastSeen to the past
	tracker.RecordQuery(ctx, "oldQuery", 100*time.Millisecond, false)
	tracker.mu.Lock()
	tracker.queries["oldQuery"].LastSeen = time.Now().Add(-2 * time.Hour)
	tracker.mu.Unlock()

	// Record a recent query
	tracker.RecordQuery(ctx, "recentQuery", 100*time.Millisecond, false)

	// Cleanup queries older than 1 hour
	tracker.Cleanup(1 * time.Hour)

	tracker.mu.RLock()
	_, oldExists := tracker.queries["oldQuery"]
	_, recentExists := tracker.queries["recentQuery"]
	tracker.mu.RUnlock()

	if oldExists {
		t.Error("Old query should have been cleaned up")
	}

	if !recentExists {
		t.Error("Recent query should not have been cleaned up")
	}
}

func TestQueryPerformanceStats(t *testing.T) {
	ctx := context.Background()
	tracker := NewQueryTracker(zap.NewNop())

	// Record multiple executions with varying durations
	queryName := "perfQuery"
	durations := []time.Duration{
		100 * time.Millisecond,
		150 * time.Millisecond,
		200 * time.Millisecond,
		250 * time.Millisecond,
		300 * time.Millisecond,
	}

	for i, d := range durations {
		tracker.RecordQuery(ctx, queryName, d, i == 4) // Last one has error
	}

	slowQueries, err := tracker.GetSlowQueries(ctx, model.Duration(50*time.Millisecond))
	if err != nil {
		t.Fatalf("GetSlowQueries failed: %v", err)
	}

	if len(slowQueries) != 1 {
		t.Fatalf("Expected 1 slow query, got %d", len(slowQueries))
	}

	perf := slowQueries[0]

	if perf.Query != queryName {
		t.Errorf("Expected query name %s, got %s", queryName, perf.Query)
	}

	if perf.Count != 5 {
		t.Errorf("Expected count 5, got %d", perf.Count)
	}

	if perf.ErrorCount != 1 {
		t.Errorf("Expected error count 1, got %d", perf.ErrorCount)
	}

	// Average should be (100+150+200+250+300)/5 = 200ms
	expectedAvg := model.Duration(200 * time.Millisecond)
	if perf.AvgDuration != expectedAvg {
		t.Errorf("Expected average duration %v, got %v", expectedAvg, perf.AvgDuration)
	}

	// P95 should be close to 300ms
	if perf.P95Duration != model.Duration(300*time.Millisecond) {
		t.Errorf("Expected P95 duration %v, got %v", model.Duration(300*time.Millisecond), perf.P95Duration)
	}
}

func TestConcurrentRecording(t *testing.T) {
	ctx := context.Background()
	tracker := NewQueryTracker(zap.NewNop())

	// Test concurrent recording
	done := make(chan bool)
	queryName := "concurrentQuery"

	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				tracker.RecordQuery(ctx, queryName, 100*time.Millisecond, false)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	tracker.mu.RLock()
	stats := tracker.queries[queryName]
	tracker.mu.RUnlock()

	if stats.Count != 1000 {
		t.Errorf("Expected count 1000 from concurrent recording, got %d", stats.Count)
	}
}
