// Package main implements the metrics-aggregator Lambda function for aggregating application metrics.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/common"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/stream"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// MetricsAggregator implements the DynamoDBStreamHandler interface for Lift
type MetricsAggregator struct {
	db                core.DB
	tableName         string
	logger            *zap.Logger
	metricsRepository *repositories.MetricsRepository
}

// NewMetricsAggregator creates a new metrics aggregator instance
func NewMetricsAggregator(db core.DB, tableName string, logger *zap.Logger) *MetricsAggregator {
	metricsRepository := repositories.NewMetricsRepository(db, tableName, logger)

	return &MetricsAggregator{
		db:                db,
		tableName:         tableName,
		logger:            logger,
		metricsRepository: metricsRepository,
	}
}

// HandleStreamWithContext implements the DynamoDBStreamHandler interface for Lift
func (ma *MetricsAggregator) HandleStreamWithContext(ctx context.Context, liftCtx *lift.Context, event events.DynamoDBEvent) error {
	ma.logger.Info("processing metrics stream batch",
		zap.String("request_id", liftCtx.GetRequestID()),
		zap.Int("record_count", len(event.Records)),
	)

	// Process records for real-time metrics aggregation
	metrics := make([]*models.Metrics, 0, len(event.Records))

	for _, record := range event.Records {
		// Only process INSERT events that represent new metrics
		if record.EventName != "INSERT" {
			continue
		}

		// Check if this is a metrics record
		pk, pkExists := record.Change.NewImage["PK"]
		if !pkExists || pk.DataType() != events.DataTypeString {
			continue
		}

		pkStr := pk.String()
		if !ma.isMetricsRecord(pkStr) {
			continue
		}

		// Extract metric data using DynamORM stream utilities
		metric, err := ma.extractMetricFromRecord(record)
		if err != nil {
			ma.logger.Warn("failed to extract metric from record",
				zap.String("request_id", liftCtx.GetRequestID()),
				zap.String("event_id", record.EventID),
				zap.Error(err))
			continue
		}

		metrics = append(metrics, metric)
	}

	// Process real-time metrics if any found
	if err := common.ValidateSliceNotEmpty("metrics", metrics); err == nil {
		return ma.processRealtimeMetricsWithContext(ctx, liftCtx, metrics)
	}

	return nil
}

var (
	processor *MetricsAggregator
)

// AggregationEvent represents the input for the aggregation job
type AggregationEvent struct {
	Type      string    `json:"type"` // "realtime", "minute", "hour", "day"
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Services  []string  `json:"services,omitempty"` // Optional: specific services to aggregate
	Metrics   []string  `json:"metrics,omitempty"`  // Optional: specific metrics to aggregate
}

func main() {
	lambdaCtx := common.MustInitializeLambda(common.LambdaConfig{
		ServiceName: "metrics-aggregator",
		LambdaType:  common.LambdaTypeProcessor,
		Version:     "1.0.0",
	})

	// Initialize storage independently to avoid import cycles
	db, err := dynamorm.GetClient(context.Background())
	if err != nil {
		lambdaCtx.Logger.Fatal("failed to initialize DynamORM database", zap.Error(err))
	}

	// Initialize processor
	processor = NewMetricsAggregator(db, lambdaCtx.Config.DynamoTableName, lambdaCtx.Logger)

	lambda.Start(func(ctx context.Context, event events.DynamoDBEvent) error {
		liftCtx := &lift.Context{
			Request:   &lift.Request{},
			RequestID: fmt.Sprintf("metrics-%d", time.Now().UnixNano()),
		}
		return processor.HandleStreamWithContext(ctx, liftCtx, event)
	})
}

// Additional methods for handling scheduled aggregation events
// These would be called by separate Lambda functions for scheduled tasks

// HandleCloudWatchEvent processes CloudWatch scheduled events for metrics aggregation
func (ma *MetricsAggregator) HandleCloudWatchEvent(ctx context.Context, event events.CloudWatchEvent) error {
	ma.logger.Info("Processing CloudWatch scheduled event",
		zap.String("detail_type", event.DetailType),
		zap.String("source", event.Source))

	// Parse the aggregation configuration from the event
	var aggEvent AggregationEvent
	if err := json.Unmarshal(event.Detail, &aggEvent); err != nil {
		// Default to hourly aggregation for scheduled events
		now := time.Now()
		aggEvent = AggregationEvent{
			Type:      "hour",
			StartTime: now.Add(-1 * time.Hour).Truncate(time.Hour),
			EndTime:   now.Truncate(time.Hour),
		}
	}

	return ma.handleAggregationEvent(ctx, aggEvent)
}

func (ma *MetricsAggregator) handleAggregationEvent(ctx context.Context, event AggregationEvent) error {
	ma.logger.Info("Processing aggregation event",
		zap.String("type", event.Type),
		zap.Time("start_time", event.StartTime),
		zap.Time("end_time", event.EndTime),
		zap.Strings("services", event.Services),
		zap.Strings("metrics", event.Metrics))

	// Determine what to aggregate
	services := event.Services
	if err := common.ValidateSliceNotEmpty("services", services); err != nil {
		// Default to all known services
		services = []string{"api", "auth", "federation", "graphql", "websocket", "processor"}
	}

	metricTypes := event.Metrics
	if err := common.ValidateSliceNotEmpty("metricTypes", metricTypes); err != nil {
		// Default to key metrics
		metricTypes = []string{"request", "error", "latency", "throughput"}
	}

	// Perform aggregation for each service and metric type
	for _, service := range services {
		for _, metricType := range metricTypes {
			if err := ma.aggregateMetrics(ctx, service, metricType, event.Type, event.StartTime, event.EndTime); err != nil {
				ma.logger.Error("failed to aggregate metrics",
					zap.String("service", service),
					zap.String("metric_type", metricType),
					zap.String("period", event.Type),
					zap.Error(err))
				// Continue with other aggregations
			}
		}
	}

	// Clean up old raw metrics if aggregating hourly or daily
	if event.Type == "hour" || event.Type == "day" {
		if err := ma.cleanupOldMetrics(ctx, event.StartTime); err != nil {
			ma.logger.Warn("failed to cleanup old metrics", zap.Error(err))
		}
	}

	return nil
}

func (ma *MetricsAggregator) aggregateMetrics(ctx context.Context, service, metricType, period string, startTime, endTime time.Time) error {
	ma.logger.Debug("aggregating metrics",
		zap.String("service", service),
		zap.String("type", metricType),
		zap.String("period", period))

	// Get service stats for the period
	stats, err := ma.metricsRepository.GetServiceStats(ctx, service, metricType, startTime, endTime)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to get service stats")
	}

	if stats.Count == 0 {
		ma.logger.Debug("no metrics to aggregate",
			zap.String("service", service),
			zap.String("type", metricType))
		return nil
	}

	// Perform aggregation using the repository
	if err := ma.metricsRepository.Aggregate(ctx, metricType, period, startTime, endTime); err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to aggregate metrics")
	}

	ma.logger.Info("aggregated metrics",
		zap.String("service", service),
		zap.String("type", metricType),
		zap.String("period", period),
		zap.Int("count", stats.Count),
		zap.Float64("average", stats.Average))

	return nil
}

func (ma *MetricsAggregator) processRealtimeMetricsWithContext(ctx context.Context, liftCtx *lift.Context, metrics []*models.Metrics) error {
	// Group metrics by service and type for efficient aggregation
	grouped := make(map[string][]*models.Metrics)

	for _, m := range metrics {
		key := fmt.Sprintf("%s:%s", m.Service, m.Type)
		grouped[key] = append(grouped[key], m)
	}

	// Create minute-level aggregations
	now := time.Now()
	windowStart := now.Truncate(time.Minute)
	windowEnd := windowStart.Add(time.Minute)

	for _, groupMetrics := range grouped {
		if err := common.ValidateSliceNotEmpty("groupMetrics", groupMetrics); err != nil {
			continue
		}

		// Extract service and type
		service := groupMetrics[0].Service
		metricType := groupMetrics[0].Type

		// Create aggregated metric
		aggregated := &models.AggregatedMetrics{
			Period:      "minute",
			Type:        metricType,
			Service:     service,
			WindowStart: windowStart,
			WindowEnd:   windowEnd,
			Percentiles: make(map[string]float64),
		}

		// Calculate aggregations
		aggregated.Min = groupMetrics[0].Min
		aggregated.Max = groupMetrics[0].Max

		for _, m := range groupMetrics {
			aggregated.TotalCount += m.Count
			aggregated.TotalSum += m.Sum

			if m.Min < aggregated.Min {
				aggregated.Min = m.Min
			}
			if m.Max > aggregated.Max {
				aggregated.Max = m.Max
			}
		}

		if aggregated.TotalCount > 0 {
			aggregated.Average = aggregated.TotalSum / float64(aggregated.TotalCount)
		}

		// Store or update the aggregation
		if err := ma.metricsRepository.CreateAggregated(ctx, aggregated); err != nil {
			ma.logger.Error("failed to create real-time aggregation",
				zap.String("request_id", liftCtx.GetRequestID()),
				zap.String("service", service),
				zap.String("type", metricType),
				zap.Error(err))
		}
	}

	return nil
}

func (ma *MetricsAggregator) isMetricsRecord(pk string) bool {
	// Check if this is a metrics record based on PK pattern
	return len(pk) > 8 && pk[:8] == "metrics#"
}

func (ma *MetricsAggregator) extractMetricFromRecord(record events.DynamoDBEventRecord) (*models.Metrics, error) {
	// Use DynamORM stream utilities for proper unmarshaling
	var metric models.Metrics

	if err := stream.UnmarshalItem(record, &metric); err != nil {
		return nil, pkgErrors.WrapError(err, pkgErrors.CodeEventProcessingFailed, pkgErrors.CategoryLambda, "Failed to unmarshal metric from stream record")
	}

	// Basic validation with structured logging
	if err := common.ValidateRequiredParam("metric.Type", metric.Type); err != nil {
		ma.logger.Error("missing required metric type",
			zap.String("type", metric.Type),
			zap.String("service", metric.Service))
		return nil, pkgErrors.MetricsAggregatorMissingRequiredFields()
	}
	if err := common.ValidateRequiredParam("metric.Service", metric.Service); err != nil {
		ma.logger.Error("missing required metric service",
			zap.String("type", metric.Type),
			zap.String("service", metric.Service))
		return nil, pkgErrors.MetricsAggregatorMissingRequiredFields()
	}

	return &metric, nil
}

func (ma *MetricsAggregator) cleanupOldMetrics(ctx context.Context, beforeTime time.Time) error {
	ma.logger.Info("Starting cleanup of old metrics",
		zap.Time("before", beforeTime))

	// Define retention periods (how long to keep raw data)
	retentionPeriods := map[string]time.Duration{
		"minute": 24 * time.Hour,      // Keep minute data for 1 day
		"hour":   7 * 24 * time.Hour,  // Keep hour data for 1 week
		"day":    30 * 24 * time.Hour, // Keep day data for 1 month
	}

	totalDeleted := 0
	for granularity, retention := range retentionPeriods {
		cutoffTime := time.Now().Add(-retention)
		if cutoffTime.After(beforeTime) {
			continue // Don't clean up data that's too new
		}

		deleted, err := ma.cleanupMetricsByGranularity(ctx, granularity, cutoffTime)
		if err != nil {
			ma.logger.Error("Failed to cleanup metrics for granularity",
				zap.String("granularity", granularity),
				zap.Error(err))
			continue
		}

		totalDeleted += deleted
		ma.logger.Info("Cleaned up metrics",
			zap.String("granularity", granularity),
			zap.Int("deleted_count", deleted),
			zap.Time("cutoff_time", cutoffTime))
	}

	ma.logger.Info("Cleanup completed",
		zap.Int("total_deleted", totalDeleted))

	return nil
}

func (ma *MetricsAggregator) cleanupMetricsByGranularity(ctx context.Context, granularity string, cutoffTime time.Time) (int, error) {
	// Delegate cleanup to the metrics repository which has proper DynamORM implementation
	ma.logger.Info("Delegating cleanup to metrics repository",
		zap.String("granularity", granularity),
		zap.Time("cutoff_time", cutoffTime))

	// Use the repository's cleanup method
	deletedCount, err := ma.metricsRepository.CleanupOldMetrics(ctx, granularity, cutoffTime)
	if err != nil {
		ma.logger.Error("cleanup operation failed",
			zap.String("granularity", granularity),
			zap.Time("cutoff_time", cutoffTime),
			zap.Error(err))
		return 0, pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to cleanup metrics")
	}

	ma.logger.Info("Cleanup operation completed successfully",
		zap.String("granularity", granularity),
		zap.Time("cutoff_time", cutoffTime),
		zap.Int("deleted_count", deletedCount))

	return deletedCount, nil
}
