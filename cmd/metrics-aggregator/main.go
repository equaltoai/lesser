package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

var (
	logger            *zap.Logger
	cfg               *config.Config
	metricsRepository *repositories.MetricsRepository
	db                core.DB
)

// AggregationEvent represents the input for the aggregation job
type AggregationEvent struct {
	Type      string    `json:"type"`      // "realtime", "minute", "hour", "day"
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Services  []string  `json:"services,omitempty"`  // Optional: specific services to aggregate
	Metrics   []string  `json:"metrics,omitempty"`   // Optional: specific metrics to aggregate
}

func init() {
	// Initialize logger
	logger = common.Logger()

	// Load configuration
	cfg = config.Get()

	// Initialize DynamORM with Lambda optimizations
	var err error
	db, err = dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize repository
	metricsRepository = repositories.NewMetricsRepository(db, cfg.DynamoTableName, logger)
}

func main() {
	lambda.Start(handleRequest)
}

func handleRequest(ctx context.Context, event interface{}) error {
	// Determine event type and route accordingly
	switch e := event.(type) {
	case events.CloudWatchEvent:
		return handleCloudWatchEvent(ctx, e)
	case events.DynamoDBEvent:
		return handleDynamoDBStream(ctx, e)
	case json.RawMessage:
		// Try to parse as custom aggregation event
		var aggEvent AggregationEvent
		if err := json.Unmarshal(e, &aggEvent); err == nil {
			return handleAggregationEvent(ctx, aggEvent)
		}
		return fmt.Errorf("unable to parse event: %s", string(e))
	default:
		return fmt.Errorf("unknown event type: %T", event)
	}
}

func handleCloudWatchEvent(ctx context.Context, event events.CloudWatchEvent) error {
	logger.Info("Processing CloudWatch scheduled event",
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

	return handleAggregationEvent(ctx, aggEvent)
}

func handleDynamoDBStream(ctx context.Context, event events.DynamoDBEvent) error {
	logger.Info("Processing DynamoDB stream event for real-time metrics",
		zap.Int("records", len(event.Records)))

	// Collect metrics from stream records
	var metrics []*models.Metrics

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
		if !isMetricsRecord(pkStr) {
			continue
		}

		// Extract metric data
		metric, err := extractMetricFromRecord(record)
		if err != nil {
			logger.Warn("failed to extract metric from record",
				zap.String("event_id", record.EventID),
				zap.Error(err))
			continue
		}

		metrics = append(metrics, metric)
	}

	// Process real-time metrics
	if len(metrics) > 0 {
		return processRealtimeMetrics(ctx, metrics)
	}

	return nil
}

func handleAggregationEvent(ctx context.Context, event AggregationEvent) error {
	logger.Info("Processing aggregation event",
		zap.String("type", event.Type),
		zap.Time("start_time", event.StartTime),
		zap.Time("end_time", event.EndTime),
		zap.Strings("services", event.Services),
		zap.Strings("metrics", event.Metrics))

	// Determine what to aggregate
	services := event.Services
	if len(services) == 0 {
		// Default to all known services
		services = []string{"api", "auth", "federation", "graphql", "websocket", "processor"}
	}

	metricTypes := event.Metrics
	if len(metricTypes) == 0 {
		// Default to key metrics
		metricTypes = []string{"request", "error", "latency", "throughput"}
	}

	// Perform aggregation for each service and metric type
	for _, service := range services {
		for _, metricType := range metricTypes {
			if err := aggregateMetrics(ctx, service, metricType, event.Type, event.StartTime, event.EndTime); err != nil {
				logger.Error("failed to aggregate metrics",
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
		if err := cleanupOldMetrics(ctx, event.StartTime); err != nil {
			logger.Warn("failed to cleanup old metrics", zap.Error(err))
		}
	}

	return nil
}

func aggregateMetrics(ctx context.Context, service, metricType, period string, startTime, endTime time.Time) error {
	logger.Debug("aggregating metrics",
		zap.String("service", service),
		zap.String("type", metricType),
		zap.String("period", period))

	// Get service stats for the period
	stats, err := metricsRepository.GetServiceStats(ctx, service, metricType, startTime, endTime)
	if err != nil {
		return fmt.Errorf("failed to get service stats: %w", err)
	}

	if stats.Count == 0 {
		logger.Debug("no metrics to aggregate",
			zap.String("service", service),
			zap.String("type", metricType))
		return nil
	}

	// Perform aggregation using the repository
	if err := metricsRepository.Aggregate(ctx, metricType, period, startTime, endTime); err != nil {
		return fmt.Errorf("failed to aggregate: %w", err)
	}

	logger.Info("aggregated metrics",
		zap.String("service", service),
		zap.String("type", metricType),
		zap.String("period", period),
		zap.Int("count", stats.Count),
		zap.Float64("average", stats.Average))

	return nil
}

func processRealtimeMetrics(ctx context.Context, metrics []*models.Metrics) error {
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
		if len(groupMetrics) == 0 {
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
		if err := metricsRepository.CreateAggregated(ctx, aggregated); err != nil {
			logger.Error("failed to create real-time aggregation",
				zap.String("service", service),
				zap.String("type", metricType),
				zap.Error(err))
		}
	}

	return nil
}

func isMetricsRecord(pk string) bool {
	// Check if this is a metrics record based on PK pattern
	return len(pk) > 8 && pk[:8] == "metrics#"
}

func extractMetricFromRecord(record events.DynamoDBEventRecord) (*models.Metrics, error) {
	metric := &models.Metrics{}

	// Extract basic fields
	if id, ok := record.Change.NewImage["id"]; ok && id.DataType() == events.DataTypeString {
		metric.ID = id.String()
	}

	if metricType, ok := record.Change.NewImage["type"]; ok && metricType.DataType() == events.DataTypeString {
		metric.Type = metricType.String()
	}

	if service, ok := record.Change.NewImage["service"]; ok && service.DataType() == events.DataTypeString {
		metric.Service = service.String()
	}

	// Extract numeric values
	if value, ok := record.Change.NewImage["value"]; ok && value.DataType() == events.DataTypeNumber {
		if v, err := value.Float(); err == nil {
			metric.Value = v
		}
	}

	if count, ok := record.Change.NewImage["count"]; ok && count.DataType() == events.DataTypeNumber {
		if v, err := count.Integer(); err == nil {
			metric.Count = v
		}
	}

	// Extract timestamp
	if ts, ok := record.Change.NewImage["timestamp"]; ok && ts.DataType() == events.DataTypeString {
		if t, err := time.Parse(time.RFC3339, ts.String()); err == nil {
			metric.Timestamp = t
		}
	}

	// Basic validation
	if metric.Type == "" || metric.Service == "" {
		return nil, fmt.Errorf("missing required fields")
	}

	return metric, nil
}

func cleanupOldMetrics(ctx context.Context, beforeTime time.Time) error {
	// This would typically mark old raw metrics for deletion
	// For now, we'll just log the intention
	logger.Info("cleanup of old metrics requested",
		zap.Time("before", beforeTime))
	
	// TODO: Implement actual cleanup logic
	// This might involve:
	// 1. Querying for metrics older than retention period
	// 2. Verifying they've been aggregated
	// 3. Marking them for deletion or moving to cold storage
	
	return nil
}