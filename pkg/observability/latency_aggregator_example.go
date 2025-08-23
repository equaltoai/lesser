// Package observability provides example integration of LatencyAggregator with existing infrastructure
package observability

import (
	"context"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// ProductionLatencyAggregator creates a production-ready latency aggregator with all integrations
func ProductionLatencyAggregator(ctx context.Context, metricsRepo HistoricalMetricsReader, createMetricFn func(ctx context.Context, metric *models.MetricRecord) error, logger *zap.Logger) (*LatencyAggregator, error) {
	// Create metrics recorder for storing aggregated data
	serviceName := os.Getenv("AWS_LAMBDA_FUNCTION_NAME")
	if serviceName == "" {
		serviceName = "lesser-observability"
	}
	metricsRecorder := NewDefaultMetricsRecorder(createMetricFn, serviceName)

	// Load AWS config for CloudWatch
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	cloudWatchClient := cloudwatch.NewFromConfig(awsCfg)

	// Create production latency aggregator with all data sources
	aggregator := NewLatencyAggregator(
		logger,
		metricsRecorder,
		WithAggregateInterval(5*time.Minute),        // 5-minute buckets
		WithRetentionPeriod(24*time.Hour),           // Keep 24 hours in memory
		WithMaxBuckets(500),                         // Memory limit
		WithMetricsRepository(metricsRepo),          // Historical DynamORM data
		WithCloudWatch(cloudWatchClient, "Lesser/Observability"), // CloudWatch fallback
	)

	return aggregator, nil
}

// ExampleUsage demonstrates how to use the latency aggregator in practice
func ExampleUsage() {
	ctx := context.Background()
	logger, _ := zap.NewProduction()
	
	// In real usage, these would be injected dependencies
	var metricsRepo HistoricalMetricsReader // Actual metrics repository instance
	createMetricFn := func(_ context.Context, _ *models.MetricRecord) error {
		// Implement metric storage logic
		return nil
	}
	
	// Create production aggregator
	aggregator, err := ProductionLatencyAggregator(ctx, metricsRepo, createMetricFn, logger)
	if err != nil {
		logger.Fatal("Failed to create latency aggregator", zap.Error(err))
	}

	// Start the aggregator
	aggregator.Start()
	defer aggregator.Stop()

	// Record latency measurements
	aggregator.RecordLatency("user.create", "api", 45*time.Millisecond)
	aggregator.RecordLatency("user.update", "api", 38*time.Millisecond)
	aggregator.RecordLatency("status.create", "api", 52*time.Millisecond)

	// Get current performance stats
	currentStats, err := aggregator.GetCurrentStats("user.create", "api")
	if err == nil {
		logger.Info("Current performance stats",
			zap.String("operation", "user.create"),
			zap.Float64("average_ms", currentStats.Average),
			zap.Int64("count", currentStats.Count),
			zap.Float64("p95", currentStats.Percentiles["p95"]))
	}

	// Get historical trend analysis
	endTime := time.Now()
	startTime := endTime.Add(-6 * time.Hour)
	trend, err := aggregator.GetLatencyTrend(ctx, "user.create", "api", startTime, endTime, 30*time.Minute)
	if err == nil {
		logger.Info("Latency trend analysis",
			zap.String("operation", "user.create"),
			zap.String("trend_direction", trend.TrendAnalysis.TrendDirection),
			zap.Float64("percent_change", trend.TrendAnalysis.PercentChange),
			zap.String("classification", trend.TrendAnalysis.ChangeClassification),
			zap.Int("data_points", len(trend.DataPoints)))
	}

	// Get aggregated stats for all operations in a service
	serviceStats, err := aggregator.GetAggregatedStats("api", 1*time.Hour)
	if err == nil {
		for operation, stats := range serviceStats {
			logger.Info("Service operation stats",
				zap.String("operation", operation),
				zap.Float64("average_ms", stats.Average),
				zap.Int64("count", stats.Count),
				zap.Float64("p99", stats.Percentiles["p99"]))
		}
	}
}

// IntegrateWithExistingMetrics shows how to integrate with existing metrics infrastructure
func IntegrateWithExistingMetrics(
	metricsCollector *MetricsCollector,
	aggregator *LatencyAggregator,
	operation string,
	serviceName string,
) {
	// Start timing
	startTime := time.Now()
	
	// ... perform operation ...
	
	// Record latency in both systems
	duration := time.Since(startTime)
	
	// Record in CloudWatch via MetricsCollector
	metricsCollector.RecordLatency(operation, duration)
	
	// Record in LatencyAggregator for detailed analysis
	aggregator.RecordLatency(operation, serviceName, duration)
}

// MonitorLatencyAlerts shows how to integrate with alerting system
func MonitorLatencyAlerts(ctx context.Context, aggregator *LatencyAggregator, logger *zap.Logger) {
	// Monitor for latency degradations
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Check recent trends for all operations
			endTime := time.Now()
			startTime := endTime.Add(-30 * time.Minute)
			
			// Get service stats (example for API service)
			stats, err := aggregator.GetAggregatedStats("api", 30*time.Minute)
			if err != nil {
				logger.Error("Failed to get service stats", zap.Error(err))
				continue
			}
			
			for operation, operationStats := range stats {
				// Alert if P95 latency is high
				if p95, exists := operationStats.Percentiles["p95"]; exists && p95 > 1000 { // 1 second
					logger.Warn("High P95 latency detected",
						zap.String("operation", operation),
						zap.Float64("p95_ms", p95),
						zap.Int64("count", operationStats.Count))
				}
				
				// Get trend analysis
				trend, err := aggregator.GetLatencyTrend(ctx, operation, "api", startTime, endTime, 5*time.Minute)
				if err != nil {
					continue
				}
				
				// Alert on significant degradations
				if trend.TrendAnalysis.ChangeClassification == "significant_degradation" {
					logger.Error("Latency degradation detected",
						zap.String("operation", operation),
						zap.Float64("percent_change", trend.TrendAnalysis.PercentChange),
						zap.Float64("trend_strength", trend.TrendAnalysis.RSquared))
				}
			}
			
		case <-ctx.Done():
			return
		}
	}
}