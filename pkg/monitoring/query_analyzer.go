package monitoring

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// QueryAnalyzer tracks and analyzes query performance
type QueryAnalyzer struct {
	monitor      *PerformanceMonitor
	slowQueryLog *SlowQueryLog
	mu           sync.RWMutex
	queryStats   map[string]*QueryStats
}

// QueryStats tracks statistics for a specific query pattern
type QueryStats struct {
	QueryPattern   string
	ExecutionCount int64
	TotalTime      time.Duration
	MinTime        time.Duration
	MaxTime        time.Duration
	LastExecution  time.Time
	ErrorCount     int64
}

// SlowQueryLog tracks slow queries
type SlowQueryLog struct {
	mu         sync.RWMutex
	threshold  time.Duration
	queries    []SlowQuery
	maxQueries int
}

// SlowQuery represents a slow query
type SlowQuery struct {
	QueryPattern string
	Duration     time.Duration
	Timestamp    time.Time
	Parameters   map[string]any
	ConsumedRCU  float64
	ConsumedWCU  float64
}

// NewQueryAnalyzer creates a new query analyzer
func NewQueryAnalyzer(monitor *PerformanceMonitor, slowQueryThreshold time.Duration) *QueryAnalyzer {
	return &QueryAnalyzer{
		monitor:    monitor,
		queryStats: make(map[string]*QueryStats),
		slowQueryLog: &SlowQueryLog{
			threshold:  slowQueryThreshold,
			queries:    make([]SlowQuery, 0),
			maxQueries: 1000, // Keep last 1000 slow queries
		},
	}
}

// QueryExecution tracks a query execution
type QueryExecution struct {
	analyzer     *QueryAnalyzer
	queryPattern string
	startTime    time.Time
	parameters   map[string]any
}

// StartQuery begins tracking a query execution
func (qa *QueryAnalyzer) StartQuery(queryPattern string, parameters map[string]any) *QueryExecution {
	return &QueryExecution{
		analyzer:     qa,
		queryPattern: queryPattern,
		startTime:    time.Now(),
		parameters:   parameters,
	}
}

// End completes the query execution tracking
func (qe *QueryExecution) End(ctx context.Context, consumedRCU, consumedWCU float64, err error) {
	duration := time.Since(qe.startTime)

	// Update query statistics
	qe.analyzer.updateStats(qe.queryPattern, duration, err != nil)

	// Record metrics
	if err := qe.analyzer.monitor.RecordLatency(ctx, "DynamoDBQuery."+qe.queryPattern, float64(duration.Milliseconds())); err != nil {
		zap.L().Warn("failed to record query latency", zap.Error(err))
	}

	if consumedRCU > 0 || consumedWCU > 0 {
		if err := qe.analyzer.monitor.RecordDynamoDBConsumedCapacity(ctx, "main", qe.queryPattern, consumedRCU, consumedWCU); err != nil {
			zap.L().Warn("failed to record DynamoDB consumed capacity", zap.Error(err))
		}
	}

	// Check if this is a slow query
	if duration > qe.analyzer.slowQueryLog.threshold {
		qe.analyzer.recordSlowQuery(SlowQuery{
			QueryPattern: qe.queryPattern,
			Duration:     duration,
			Timestamp:    qe.startTime,
			Parameters:   qe.parameters,
			ConsumedRCU:  consumedRCU,
			ConsumedWCU:  consumedWCU,
		})

		// Record slow query metric
		if err := qe.analyzer.monitor.RecordError(ctx, "DynamoDBQuery."+qe.queryPattern, "SlowQuery"); err != nil {
			zap.L().Warn("failed to record slow query metric", zap.Error(err))
		}
	}

	if err != nil {
		if recordErr := qe.analyzer.monitor.RecordError(ctx, "DynamoDBQuery."+qe.queryPattern, "QueryError"); recordErr != nil {
			zap.L().Warn("failed to record query error metric", zap.Error(recordErr))
		}
	}
}

// updateStats updates statistics for a query pattern
func (qa *QueryAnalyzer) updateStats(queryPattern string, duration time.Duration, hasError bool) {
	qa.mu.Lock()
	defer qa.mu.Unlock()

	stats, exists := qa.queryStats[queryPattern]
	if !exists {
		stats = &QueryStats{
			QueryPattern: queryPattern,
			MinTime:      duration,
			MaxTime:      duration,
		}
		qa.queryStats[queryPattern] = stats
	}

	stats.ExecutionCount++
	stats.TotalTime += duration
	stats.LastExecution = time.Now()

	if duration < stats.MinTime {
		stats.MinTime = duration
	}
	if duration > stats.MaxTime {
		stats.MaxTime = duration
	}

	if hasError {
		stats.ErrorCount++
	}
}

// recordSlowQuery adds a query to the slow query log
func (qa *QueryAnalyzer) recordSlowQuery(query SlowQuery) {
	qa.slowQueryLog.mu.Lock()
	defer qa.slowQueryLog.mu.Unlock()

	qa.slowQueryLog.queries = append(qa.slowQueryLog.queries, query)

	// Keep only the most recent queries
	if len(qa.slowQueryLog.queries) > qa.slowQueryLog.maxQueries {
		qa.slowQueryLog.queries = qa.slowQueryLog.queries[len(qa.slowQueryLog.queries)-qa.slowQueryLog.maxQueries:]
	}
}

// GetQueryStats returns statistics for all tracked queries
func (qa *QueryAnalyzer) GetQueryStats() map[string]QueryStats {
	qa.mu.RLock()
	defer qa.mu.RUnlock()

	// Create a copy to avoid holding the lock
	statsCopy := make(map[string]QueryStats)
	for pattern, stats := range qa.queryStats {
		statsCopy[pattern] = *stats
	}

	return statsCopy
}

// GetSlowQueries returns recent slow queries
func (qa *QueryAnalyzer) GetSlowQueries(limit int) []SlowQuery {
	qa.slowQueryLog.mu.RLock()
	defer qa.slowQueryLog.mu.RUnlock()

	if limit <= 0 || limit > len(qa.slowQueryLog.queries) {
		limit = len(qa.slowQueryLog.queries)
	}

	// Return the most recent queries
	start := len(qa.slowQueryLog.queries) - limit
	if start < 0 {
		start = 0
	}

	result := make([]SlowQuery, limit)
	copy(result, qa.slowQueryLog.queries[start:])

	return result
}

// GetAverageQueryTime returns the average execution time for a query pattern
func (qa *QueryAnalyzer) GetAverageQueryTime(queryPattern string) (time.Duration, bool) {
	qa.mu.RLock()
	defer qa.mu.RUnlock()

	stats, exists := qa.queryStats[queryPattern]
	if !exists || stats.ExecutionCount == 0 {
		return 0, false
	}

	return stats.TotalTime / time.Duration(stats.ExecutionCount), true
}

// DetectN1Queries analyzes patterns that might indicate N+1 query problems
func (qa *QueryAnalyzer) DetectN1Queries() []N1QueryPattern {
	qa.mu.RLock()
	defer qa.mu.RUnlock()

	patterns := make([]N1QueryPattern, 0)

	// Look for query patterns that are executed many times in quick succession
	for pattern, stats := range qa.queryStats {
		avgTime := stats.TotalTime / time.Duration(stats.ExecutionCount)

		// If a query is executed more than 10 times with low average time,
		// it might be an N+1 problem
		if stats.ExecutionCount > 10 && avgTime < 50*time.Millisecond {
			patterns = append(patterns, N1QueryPattern{
				QueryPattern:   pattern,
				ExecutionCount: stats.ExecutionCount,
				AverageTime:    avgTime,
				TotalTime:      stats.TotalTime,
			})
		}
	}

	return patterns
}

// N1QueryPattern represents a potential N+1 query problem
type N1QueryPattern struct {
	QueryPattern   string
	ExecutionCount int64
	AverageTime    time.Duration
	TotalTime      time.Duration
}

// ResetStats clears all query statistics
func (qa *QueryAnalyzer) ResetStats() {
	qa.mu.Lock()
	defer qa.mu.Unlock()

	qa.queryStats = make(map[string]*QueryStats)
}

// AnalyzeQueryComplexity calculates the complexity score for a query
func AnalyzeQueryComplexity(queryType string, filters map[string]any, limit int) int {
	complexity := 1 // Base complexity

	// Add complexity based on query type
	switch queryType {
	case "Scan":
		complexity += 10 // Scans are expensive
	case "Query":
		complexity += 2
	case "BatchGetItem":
		complexity += 3
	}

	// Add complexity based on filters
	complexity += len(filters)

	// Add complexity based on limit
	if limit > 100 {
		complexity += (limit / 100)
	}

	return complexity
}

// QueryPlan represents an analyzed query plan
type QueryPlan struct {
	QueryType      string
	IndexUsed      string
	EstimatedRCU   float64
	EstimatedItems int
	Warnings       []string
}

// AnalyzeQueryPlan analyzes a query and provides optimization suggestions
func AnalyzeQueryPlan(_, queryType, indexName string, _ map[string]any) QueryPlan {
	plan := QueryPlan{
		QueryType: queryType,
		IndexUsed: indexName,
		Warnings:  make([]string, 0),
	}

	// Check for scan operations
	if queryType == "Scan" {
		plan.Warnings = append(plan.Warnings, "Scan operation detected - consider using Query with appropriate index")
		plan.EstimatedRCU = 100 // Rough estimate
	}

	// Check for missing index
	if indexName == "" && queryType == "Query" {
		plan.Warnings = append(plan.Warnings, "No index specified - using primary key")
	}

	// Check for large result sets
	if plan.EstimatedItems > 1000 {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("Large result set expected (%d items) - consider pagination", plan.EstimatedItems))
	}

	return plan
}
