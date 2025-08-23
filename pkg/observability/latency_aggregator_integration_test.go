// Package observability integration test for latency aggregator with historical data
//go:build integration

package observability

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// MockHistoricalMetricsReader implements HistoricalMetricsReader for testing
type MockHistoricalMetricsReader struct {
	records []*models.MetricRecord
}

func (m *MockHistoricalMetricsReader) GetMetricsByService(ctx context.Context, serviceName string, startTime, endTime time.Time) ([]*models.MetricRecord, error) {
	var filtered []*models.MetricRecord
	for _, record := range m.records {
		if record.ServiceName == serviceName &&
			record.Timestamp.After(startTime) &&
			record.Timestamp.Before(endTime) {
			filtered = append(filtered, record)
		}
	}
	return filtered, nil
}

// MockMetricsRecorder implements MetricsRecorder for testing
type MockMetricsRecorder struct {
	recordings []MetricRecording
}

type MetricRecording struct {
	Operation string
	Table     string
	Duration  time.Duration
	Success   bool
	Dimensions map[string]string
}

func (m *MockMetricsRecorder) RecordLatency(ctx context.Context, operation, table string, duration time.Duration, success bool, dimensions map[string]string) error {
	m.recordings = append(m.recordings, MetricRecording{
		Operation:  operation,
		Table:      table,
		Duration:   duration,
		Success:    success,
		Dimensions: dimensions,
	})
	return nil
}

func TestLatencyAggregatorWithHistoricalData(t *testing.T) {
	logger := zap.NewNop()
	
	// Create mock historical data
	mockRepo := &MockHistoricalMetricsReader{
		records: []*models.MetricRecord{
			{
				MetricType:  "latency_aggregated",
				ServiceName: "test-service", 
				Timestamp:   time.Now().Add(-2 * time.Hour),
				Count:       100,
				Sum:         5000, // 50ms average
				P50:         45,
				P95:         85,
				P99:         120,
				Dimensions: map[string]string{
					"operation": "test-operation",
				},
			},
			{
				MetricType:  "latency_aggregated",
				ServiceName: "test-service",
				Timestamp:   time.Now().Add(-1 * time.Hour),
				Count:       120,
				Sum:         7200, // 60ms average
				P50:         55,
				P95:         95,
				P99:         130,
				Dimensions: map[string]string{
					"operation": "test-operation",
				},
			},
		},
	}

	mockRecorder := &MockMetricsRecorder{}

	// Create latency aggregator with historical data source
	aggregator := NewLatencyAggregator(
		logger,
		mockRecorder,
		WithMetricsRepository(mockRepo),
		WithAggregateInterval(5*time.Minute),
	)

	// Record some current latency measurements
	aggregator.RecordLatency("test-operation", "test-service", 45*time.Millisecond)
	aggregator.RecordLatency("test-operation", "test-service", 55*time.Millisecond)
	aggregator.RecordLatency("test-operation", "test-service", 65*time.Millisecond)

	// Get latency trend including historical data
	ctx := context.Background()
	startTime := time.Now().Add(-3 * time.Hour)
	endTime := time.Now()
	interval := 30 * time.Minute

	trend, err := aggregator.GetLatencyTrend(ctx, "test-operation", "test-service", startTime, endTime, interval)
	if err != nil {
		t.Fatalf("Failed to get latency trend: %v", err)
	}

	// Verify we got historical and current data
	if len(trend.DataPoints) < 2 {
		t.Errorf("Expected at least 2 data points (historical + current), got %d", len(trend.DataPoints))
	}

	// Verify trend analysis was calculated
	if trend.TrendAnalysis.TrendDirection == "" {
		t.Error("Expected trend direction to be calculated")
	}

	// Verify percentiles are included
	if len(trend.Percentiles) == 0 {
		t.Error("Expected percentile trends to be calculated")
	}

	// Test current stats
	currentStats, err := aggregator.GetCurrentStats("test-operation", "test-service")
	if err != nil {
		t.Fatalf("Failed to get current stats: %v", err)
	}

	if currentStats.Count != 3 {
		t.Errorf("Expected count=3, got %d", currentStats.Count)
	}

	if currentStats.Average == 0 {
		t.Error("Expected average to be calculated")
	}

	t.Logf("Latency trend analysis: %+v", trend.TrendAnalysis)
	t.Logf("Current stats: Count=%d, Average=%.2f", currentStats.Count, currentStats.Average)
}

func TestLatencyAggregatorDataPointAggregation(t *testing.T) {
	logger := zap.NewNop()
	mockRecorder := &MockMetricsRecorder{}

	aggregator := NewLatencyAggregator(logger, mockRecorder)

	// Test data point aggregation by interval
	dataPoints := []LatencyDataPoint{
		{
			Timestamp: time.Date(2024, 1, 1, 10, 5, 0, 0, time.UTC),
			Average:   50.0,
			Count:     10,
			Percentiles: map[string]float64{"p95": 80.0},
		},
		{
			Timestamp: time.Date(2024, 1, 1, 10, 7, 0, 0, time.UTC),
			Average:   60.0,
			Count:     15,
			Percentiles: map[string]float64{"p95": 90.0},
		},
	}

	// Aggregate into 5-minute buckets
	interval := 5 * time.Minute
	aggregated := aggregator.aggregateDataPointsByInterval(dataPoints, interval)

	if len(aggregated) != 1 {
		t.Errorf("Expected 1 aggregated bucket, got %d", len(aggregated))
	}

	if aggregated[0].Count != 25 {
		t.Errorf("Expected aggregated count=25, got %d", aggregated[0].Count)
	}

	expectedAvg := (50.0*10 + 60.0*15) / 25 // Weighted average
	if aggregated[0].Average != expectedAvg {
		t.Errorf("Expected weighted average=%.2f, got %.2f", expectedAvg, aggregated[0].Average)
	}
}

func TestLatencyAggregatorDeduplication(t *testing.T) {
	logger := zap.NewNop()
	mockRecorder := &MockMetricsRecorder{}

	aggregator := NewLatencyAggregator(logger, mockRecorder)

	// Test data point deduplication
	timestamp := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	dataPoints := []LatencyDataPoint{
		{
			Timestamp: timestamp,
			Average:   50.0,
			Count:     10,
		},
		{
			Timestamp: timestamp, // Same timestamp
			Average:   60.0,
			Count:     20,
		},
		{
			Timestamp: timestamp.Add(time.Minute), // Different timestamp
			Average:   70.0,
			Count:     5,
		},
	}

	deduplicated := aggregator.deduplicateDataPoints(dataPoints)

	if len(deduplicated) != 2 {
		t.Errorf("Expected 2 deduplicated data points, got %d", len(deduplicated))
	}

	// Find the merged point
	var mergedPoint LatencyDataPoint
	for _, dp := range deduplicated {
		if dp.Timestamp.Equal(timestamp) {
			mergedPoint = dp
			break
		}
	}

	expectedCount := int64(30) // 10 + 20
	if mergedPoint.Count != expectedCount {
		t.Errorf("Expected merged count=%d, got %d", expectedCount, mergedPoint.Count)
	}

	expectedAvg := (50.0*10 + 60.0*20) / 30 // Weighted average
	if mergedPoint.Average != expectedAvg {
		t.Errorf("Expected merged average=%.2f, got %.2f", expectedAvg, mergedPoint.Average)
	}
}