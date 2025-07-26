package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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
	dynamoClient      *dynamodb.Client
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

	// Initialize DynamORM
	var err error
	db, err = dynamorm.GetClient(context.Background())
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize raw DynamoDB client for cleanup operations
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		logger.Fatal("Failed to load AWS config", zap.Error(err))
	}
	dynamoClient = dynamodb.NewFromConfig(awsCfg)

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
	logger.Info("Starting cleanup of old metrics",
		zap.Time("before", beforeTime))
	
	// Define retention periods (how long to keep raw data)
	retentionPeriods := map[string]time.Duration{
		"minute": 24 * time.Hour,       // Keep minute data for 1 day
		"hour":   7 * 24 * time.Hour,   // Keep hour data for 1 week
		"day":    30 * 24 * time.Hour,  // Keep day data for 1 month
	}
	
	totalDeleted := 0
	for granularity, retention := range retentionPeriods {
		cutoffTime := time.Now().Add(-retention)
		if cutoffTime.After(beforeTime) {
			continue // Don't clean up data that's too new
		}
		
		deleted, err := cleanupMetricsByGranularity(ctx, granularity, cutoffTime)
		if err != nil {
			logger.Error("Failed to cleanup metrics for granularity",
				zap.String("granularity", granularity),
				zap.Error(err))
			continue
		}
		
		totalDeleted += deleted
		logger.Info("Cleaned up metrics",
			zap.String("granularity", granularity),
			zap.Int("deleted_count", deleted),
			zap.Time("cutoff_time", cutoffTime))
	}
	
	logger.Info("Cleanup completed",
		zap.Int("total_deleted", totalDeleted))
	
	return nil
}

func cleanupMetricsByGranularity(ctx context.Context, granularity string, cutoffTime time.Time) (int, error) {
	const maxBatchSize = 25 // DynamoDB BatchWriteItem limit
	deletedCount := 0
	
	// Query for old metrics by scanning with time filter
	// Using GSI or time-based partition key pattern
	input := &dynamodb.ScanInput{
		TableName:        &cfg.DynamoTableName,
		FilterExpression: stringPtr("begins_with(PK, :pk_prefix) AND #ts < :cutoff_time"),
		ExpressionAttributeNames: map[string]string{
			"#ts": "timestamp",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk_prefix": &types.AttributeValueMemberS{
				Value: fmt.Sprintf("metrics#%s#", granularity),
			},
			":cutoff_time": &types.AttributeValueMemberS{
				Value: cutoffTime.Format(time.RFC3339),
			},
		},
		ProjectionExpression: stringPtr("PK, SK"),
		Limit:                int32Ptr(1000), // Process in batches
	}
	
	var lastEvaluatedKey map[string]types.AttributeValue
	
	for {
		if lastEvaluatedKey != nil {
			input.ExclusiveStartKey = lastEvaluatedKey
		}
		
		result, err := dynamoClient.Scan(ctx, input)
		if err != nil {
			return deletedCount, fmt.Errorf("failed to scan for old metrics: %w", err)
		}
		
		if len(result.Items) == 0 {
			break
		}
		
		// Delete in batches
		for i := 0; i < len(result.Items); i += maxBatchSize {
			end := i + maxBatchSize
			if end > len(result.Items) {
				end = len(result.Items)
			}
			
			batch := result.Items[i:end]
			deleted, err := deleteBatch(ctx, batch)
			if err != nil {
				logger.Warn("Failed to delete batch",
					zap.Int("batch_size", len(batch)),
					zap.Error(err))
				continue
			}
			
			deletedCount += deleted
		}
		
		lastEvaluatedKey = result.LastEvaluatedKey
		if lastEvaluatedKey == nil {
			break
		}
	}
	
	return deletedCount, nil
}

func deleteBatch(ctx context.Context, items []map[string]types.AttributeValue) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	
	writeRequests := make([]types.WriteRequest, 0, len(items))
	
	for _, item := range items {
		// Extract PK and SK for deletion
		pk, pkExists := item["PK"]
		sk, skExists := item["SK"]
		
		if !pkExists || !skExists {
			continue
		}
		
		deleteRequest := types.WriteRequest{
			DeleteRequest: &types.DeleteRequest{
				Key: map[string]types.AttributeValue{
					"PK": pk,
					"SK": sk,
				},
			},
		}
		
		writeRequests = append(writeRequests, deleteRequest)
	}
	
	if len(writeRequests) == 0 {
		return 0, nil
	}
	
	input := &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
			cfg.DynamoTableName: writeRequests,
		},
	}
	
	_, err := dynamoClient.BatchWriteItem(ctx, input)
	if err != nil {
		return 0, fmt.Errorf("failed to batch delete items: %w", err)
	}
	
	return len(writeRequests), nil
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func int32Ptr(i int32) *int32 {
	return &i
}