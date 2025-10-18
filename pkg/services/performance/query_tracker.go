package performance

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// QueryTracker tracks GraphQL query performance for identifying slow operations
type QueryTracker struct {
	mu            sync.RWMutex
	queries       map[string]*queryStats
	logger        *zap.Logger
	maxQueries    int
	slowThreshold time.Duration
}

// queryStats holds statistics for a specific query
type queryStats struct {
	QueryName     string
	Count         int64
	ErrorCount    int64
	Durations     []time.Duration
	LastSeen      time.Time
	TotalDuration time.Duration
}

// NewQueryTracker creates a new query performance tracker
func NewQueryTracker(logger *zap.Logger) *QueryTracker {
	return &QueryTracker{
		queries:       make(map[string]*queryStats),
		logger:        logger,
		maxQueries:    1000, // Keep top 1000 queries
		slowThreshold: 1 * time.Second,
	}
}

// RecordQuery records a query execution with its duration
func (qt *QueryTracker) RecordQuery(_ context.Context, queryName string, duration time.Duration, hasError bool) {
	if err := common.ValidateRequiredParam("queryName", queryName); err != nil {
		return
	}

	qt.mu.Lock()
	defer qt.mu.Unlock()

	stats, exists := qt.queries[queryName]
	if !exists {
		stats = &queryStats{
			QueryName: queryName,
			Durations: make([]time.Duration, 0, 100),
		}
		qt.queries[queryName] = stats
	}

	stats.Count++
	stats.LastSeen = time.Now()
	stats.TotalDuration += duration

	if hasError {
		stats.ErrorCount++
	}

	// Keep last 100 durations for percentile calculation
	if len(stats.Durations) < 100 {
		stats.Durations = append(stats.Durations, duration)
	} else {
		// Rolling window - remove oldest
		stats.Durations = append(stats.Durations[1:], duration)
	}

	// Log slow queries
	if duration > qt.slowThreshold {
		qt.logger.Warn("slow GraphQL query detected",
			zap.String("query", queryName),
			zap.Duration("duration", duration),
			zap.Duration("threshold", qt.slowThreshold))
	}
}

// GetSlowQueries retrieves queries that exceed the specified threshold
func (qt *QueryTracker) GetSlowQueries(_ context.Context, threshold model.Duration) ([]*model.QueryPerformance, error) {
	qt.mu.RLock()
	defer qt.mu.RUnlock()

	thresholdDuration := time.Duration(threshold)
	slowQueries := make([]*model.QueryPerformance, 0)

	for _, stats := range qt.queries {
		avgDuration := time.Duration(0)
		if stats.Count > 0 {
			avgDuration = stats.TotalDuration / time.Duration(stats.Count)
		}

		// Only include queries that have average duration above threshold
		if avgDuration >= thresholdDuration {
			p95Duration := qt.calculateP95(stats.Durations)

			slowQueries = append(slowQueries, &model.QueryPerformance{
				Query:       stats.QueryName,
				Count:       int(stats.Count),
				AvgDuration: model.Duration(avgDuration),
				P95Duration: model.Duration(p95Duration),
				ErrorCount:  int(stats.ErrorCount),
				LastSeen:    model.Time(stats.LastSeen),
			})
		}
	}

	// Sort by average duration (slowest first)
	sort.Slice(slowQueries, func(i, j int) bool {
		return slowQueries[i].AvgDuration > slowQueries[j].AvgDuration
	})

	// Limit results
	if len(slowQueries) > 50 {
		slowQueries = slowQueries[:50]
	}

	return slowQueries, nil
}

// calculateP95 calculates the 95th percentile from a slice of durations
func (qt *QueryTracker) calculateP95(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	// Copy and sort durations
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	// Calculate 95th percentile index
	index := int(float64(len(sorted)) * 0.95)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

// GetAllQueries returns all tracked queries (for debugging/monitoring)
func (qt *QueryTracker) GetAllQueries() []*model.QueryPerformance {
	qt.mu.RLock()
	defer qt.mu.RUnlock()

	queries := make([]*model.QueryPerformance, 0, len(qt.queries))
	for _, stats := range qt.queries {
		avgDuration := time.Duration(0)
		if stats.Count > 0 {
			avgDuration = stats.TotalDuration / time.Duration(stats.Count)
		}

		p95Duration := qt.calculateP95(stats.Durations)

		queries = append(queries, &model.QueryPerformance{
			Query:       stats.QueryName,
			Count:       int(stats.Count),
			AvgDuration: model.Duration(avgDuration),
			P95Duration: model.Duration(p95Duration),
			ErrorCount:  int(stats.ErrorCount),
			LastSeen:    model.Time(stats.LastSeen),
		})
	}

	return queries
}

// Cleanup removes old query statistics to prevent memory leaks
func (qt *QueryTracker) Cleanup(maxAge time.Duration) {
	qt.mu.Lock()
	defer qt.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for queryName, stats := range qt.queries {
		if stats.LastSeen.Before(cutoff) {
			delete(qt.queries, queryName)
		}
	}

	qt.logger.Info("cleaned up old query statistics",
		zap.Int("remaining_queries", len(qt.queries)),
		zap.Duration("max_age", maxAge))
}
