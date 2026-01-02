package monitoring

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-xray-sdk-go/xray"
	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// PerformanceMonitor handles performance metrics collection
type PerformanceMonitor struct {
	cloudwatch  cloudWatchAPI
	namespace   string
	environment string
}

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor(cfg aws.Config, namespace, environment string) *PerformanceMonitor {
	return &PerformanceMonitor{
		cloudwatch:  cloudwatch.NewFromConfig(cfg),
		namespace:   namespace,
		environment: environment,
	}
}

// MetricData represents a single metric data point
type MetricData struct {
	Name       string
	Value      float64
	Unit       types.StandardUnit
	Dimensions map[string]string
}

// RecordLatency records a latency metric
func (pm *PerformanceMonitor) RecordLatency(ctx context.Context, operation string, latencyMs float64) error {
	metric := MetricData{
		Name:  "OperationLatency",
		Value: latencyMs,
		Unit:  types.StandardUnitMilliseconds,
		Dimensions: map[string]string{
			"Operation":   operation,
			"Environment": pm.environment,
		},
	}

	return pm.putMetric(ctx, metric)
}

// RecordError records an error metric
func (pm *PerformanceMonitor) RecordError(ctx context.Context, operation string, errorType string) error {
	metric := MetricData{
		Name:  "OperationErrors",
		Value: 1,
		Unit:  types.StandardUnitCount,
		Dimensions: map[string]string{
			"Operation":   operation,
			"ErrorType":   errorType,
			"Environment": pm.environment,
		},
	}

	return pm.putMetric(ctx, metric)
}

// RecordDynamoDBConsumedCapacity records DynamoDB consumed capacity
func (pm *PerformanceMonitor) RecordDynamoDBConsumedCapacity(ctx context.Context, tableName string, operation string, readCapacity, writeCapacity float64) error {
	readMetric := MetricData{
		Name:  "DynamoDBReadCapacity",
		Value: readCapacity,
		Unit:  types.StandardUnitCount,
		Dimensions: map[string]string{
			"TableName":   tableName,
			"Operation":   operation,
			"Environment": pm.environment,
		},
	}

	writeMetric := MetricData{
		Name:  "DynamoDBWriteCapacity",
		Value: writeCapacity,
		Unit:  types.StandardUnitCount,
		Dimensions: map[string]string{
			"TableName":   tableName,
			"Operation":   operation,
			"Environment": pm.environment,
		},
	}

	if err := pm.putMetric(ctx, readMetric); err != nil {
		return err
	}

	return pm.putMetric(ctx, writeMetric)
}

// RecordLambdaColdStart records Lambda cold start metrics
func (pm *PerformanceMonitor) RecordLambdaColdStart(ctx context.Context, functionName string, coldStart bool, initDurationMs float64) error {
	if coldStart {
		metric := MetricData{
			Name:  "LambdaColdStarts",
			Value: 1,
			Unit:  types.StandardUnitCount,
			Dimensions: map[string]string{
				"FunctionName": functionName,
				"Environment":  pm.environment,
			},
		}

		if err := pm.putMetric(ctx, metric); err != nil {
			return err
		}

		if initDurationMs > 0 {
			initMetric := MetricData{
				Name:  "LambdaInitDuration",
				Value: initDurationMs,
				Unit:  types.StandardUnitMilliseconds,
				Dimensions: map[string]string{
					"FunctionName": functionName,
					"Environment":  pm.environment,
				},
			}

			return pm.putMetric(ctx, initMetric)
		}
	}

	return nil
}

// RecordSQSQueueDepth records SQS queue depth metrics
func (pm *PerformanceMonitor) RecordSQSQueueDepth(ctx context.Context, queueName string, depth int64) error {
	metric := MetricData{
		Name:  "SQSQueueDepth",
		Value: float64(depth),
		Unit:  types.StandardUnitCount,
		Dimensions: map[string]string{
			"QueueName":   queueName,
			"Environment": pm.environment,
		},
	}

	return pm.putMetric(ctx, metric)
}

// RecordQueryComplexity records GraphQL query complexity metrics
func (pm *PerformanceMonitor) RecordQueryComplexity(ctx context.Context, queryName string, complexity int) error {
	metric := MetricData{
		Name:  "GraphQLQueryComplexity",
		Value: float64(complexity),
		Unit:  types.StandardUnitCount,
		Dimensions: map[string]string{
			"QueryName":   queryName,
			"Environment": pm.environment,
		},
	}

	return pm.putMetric(ctx, metric)
}

// RecordCacheHit records cache hit/miss metrics
func (pm *PerformanceMonitor) RecordCacheHit(ctx context.Context, cacheName string, hit bool) error {
	metricName := "CacheMisses"
	if hit {
		metricName = "CacheHits"
	}

	metric := MetricData{
		Name:  metricName,
		Value: 1,
		Unit:  types.StandardUnitCount,
		Dimensions: map[string]string{
			"CacheName":   cacheName,
			"Environment": pm.environment,
		},
	}

	return pm.putMetric(ctx, metric)
}

// RecordFederationPerformance records federation-specific performance metrics
func (pm *PerformanceMonitor) RecordFederationPerformance(ctx context.Context, domain string, operation string, latencyMs float64, success bool) error {
	latencyMetric := MetricData{
		Name:  "FederationLatency",
		Value: latencyMs,
		Unit:  types.StandardUnitMilliseconds,
		Dimensions: map[string]string{
			"Domain":      domain,
			"Operation":   operation,
			"Environment": pm.environment,
		},
	}

	if err := pm.putMetric(ctx, latencyMetric); err != nil {
		return err
	}

	if !success {
		errorMetric := MetricData{
			Name:  "FederationErrors",
			Value: 1,
			Unit:  types.StandardUnitCount,
			Dimensions: map[string]string{
				"Domain":      domain,
				"Operation":   operation,
				"Environment": pm.environment,
			},
		}

		return pm.putMetric(ctx, errorMetric)
	}

	return nil
}

// putMetric sends a metric to CloudWatch
func (pm *PerformanceMonitor) putMetric(ctx context.Context, metric MetricData) error {
	dimensions := make([]types.Dimension, 0, len(metric.Dimensions))
	for name, value := range metric.Dimensions {
		dimensions = append(dimensions, types.Dimension{
			Name:  aws.String(name),
			Value: aws.String(value),
		})
	}

	input := &cloudwatch.PutMetricDataInput{
		Namespace: aws.String(pm.namespace),
		MetricData: []types.MetricDatum{
			{
				MetricName: aws.String(metric.Name),
				Value:      aws.Float64(metric.Value),
				Unit:       metric.Unit,
				Dimensions: dimensions,
				Timestamp:  aws.Time(time.Now()),
			},
		},
	}

	_, err := pm.cloudwatch.PutMetricData(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to put metric data: %w", err)
	}

	return nil
}

// BatchMetrics allows batching multiple metrics for efficiency
type BatchMetrics struct {
	monitor *PerformanceMonitor
	metrics []types.MetricDatum
}

// NewBatchMetrics creates a new batch metrics collector
func (pm *PerformanceMonitor) NewBatchMetrics() *BatchMetrics {
	return &BatchMetrics{
		monitor: pm,
		metrics: make([]types.MetricDatum, 0, 20), // CloudWatch allows max 20 metrics per request
	}
}

// Add adds a metric to the batch
func (bm *BatchMetrics) Add(metric MetricData) {
	dimensions := make([]types.Dimension, 0, len(metric.Dimensions))
	for name, value := range metric.Dimensions {
		dimensions = append(dimensions, types.Dimension{
			Name:  aws.String(name),
			Value: aws.String(value),
		})
	}

	bm.metrics = append(bm.metrics, types.MetricDatum{
		MetricName: aws.String(metric.Name),
		Value:      aws.Float64(metric.Value),
		Unit:       metric.Unit,
		Dimensions: dimensions,
		Timestamp:  aws.Time(time.Now()),
	})
}

// Flush sends all batched metrics to CloudWatch
func (bm *BatchMetrics) Flush(ctx context.Context) error {
	if err := common.ValidateSliceNotEmpty("bm.metrics", bm.metrics); err != nil {
		return nil
	}

	// CloudWatch has a limit of 20 metrics per request
	for i := 0; i < len(bm.metrics); i += 20 {
		end := i + 20
		if end > len(bm.metrics) {
			end = len(bm.metrics)
		}

		input := &cloudwatch.PutMetricDataInput{
			Namespace:  aws.String(bm.monitor.namespace),
			MetricData: bm.metrics[i:end],
		}

		if _, err := bm.monitor.cloudwatch.PutMetricData(ctx, input); err != nil {
			return fmt.Errorf("failed to put batch metric data: %w", err)
		}
	}

	// Clear the metrics after successful flush
	bm.metrics = bm.metrics[:0]

	return nil
}

// XRaySegment represents an X-Ray trace segment
type XRaySegment struct {
	Name        string
	Annotations map[string]any
	Metadata    map[string]any
}

// StartXRaySegment starts a new X-Ray segment
func (pm *PerformanceMonitor) StartXRaySegment(ctx context.Context, name string) (context.Context, *xray.Segment) {
	return xray.BeginSegment(ctx, name)
}

// StartXRaySubsegment starts a new X-Ray subsegment
func (pm *PerformanceMonitor) StartXRaySubsegment(ctx context.Context, name string) (context.Context, *xray.Segment) {
	return xray.BeginSubsegment(ctx, name)
}

// AddXRayAnnotation adds an annotation to the current X-Ray segment
func (pm *PerformanceMonitor) AddXRayAnnotation(ctx context.Context, key string, value any) error {
	return xray.AddAnnotation(ctx, key, value)
}

// AddXRayMetadata adds metadata to the current X-Ray segment
func (pm *PerformanceMonitor) AddXRayMetadata(ctx context.Context, namespace string, key string, value any) error {
	// Create metadata map with key-value pair
	metadata := map[string]any{
		key: value,
	}
	return xray.AddMetadata(ctx, namespace, metadata)
}

// RecordXRayError records an error in the current X-Ray segment
func (pm *PerformanceMonitor) RecordXRayError(ctx context.Context, err error) {
	if err != nil {
		if xrayErr := xray.AddError(ctx, err); xrayErr != nil {
			zap.L().Warn("failed to add X-Ray error", zap.Error(xrayErr))
		}
	}
}

// TraceDBQuery wraps a database query with X-Ray tracing
func (pm *PerformanceMonitor) TraceDBQuery(ctx context.Context, operation string, tableName string, fn func(context.Context) error) error {
	ctx, seg := xray.BeginSubsegment(ctx, "DynamoDB "+operation)
	defer seg.Close(nil)

	if err := seg.AddAnnotation("operation", operation); err != nil {
		zap.L().Warn("failed to add X-Ray annotation 'operation'", zap.Error(err))
	}
	if err := seg.AddAnnotation("table_name", tableName); err != nil {
		zap.L().Warn("failed to add X-Ray annotation 'table_name'", zap.Error(err))
	}
	seg.Namespace = ProviderAWS

	start := time.Now()
	err := fn(ctx)
	duration := time.Since(start)

	if err != nil {
		_ = seg.AddError(err)
		if recordErr := pm.RecordError(ctx, "db_query", err.Error()); recordErr != nil {
			zap.L().Warn("failed to record db error metric", zap.Error(recordErr))
		}
	}

	if err := seg.AddMetadata("dynamodb", map[string]any{"duration_ms": duration.Milliseconds()}); err != nil {
		zap.L().Warn("failed to add X-Ray metadata 'dynamodb'", zap.Error(err))
	}
	_ = pm.RecordLatency(ctx, "db_query_"+operation, float64(duration.Milliseconds()))

	return err
}

// TraceFederationCall wraps a federation call with X-Ray tracing
func (pm *PerformanceMonitor) TraceFederationCall(ctx context.Context, domain string, operation string, fn func(context.Context) error) error {
	ctx, seg := xray.BeginSubsegment(ctx, "Federation "+operation)
	defer seg.Close(nil)

	if err := seg.AddAnnotation("domain", domain); err != nil {
		zap.L().Warn("failed to add X-Ray annotation 'domain'", zap.Error(err))
	}
	if err := seg.AddAnnotation("operation", operation); err != nil {
		zap.L().Warn("failed to add X-Ray annotation 'operation'", zap.Error(err))
	}
	seg.Namespace = "remote"

	start := time.Now()
	err := fn(ctx)
	duration := time.Since(start)

	success := err == nil
	if err != nil {
		_ = seg.AddError(err)
	}

	if err := seg.AddMetadata("federation", map[string]any{
		"duration_ms": duration.Milliseconds(),
		"success":     success,
	}); err != nil {
		zap.L().Warn("failed to add X-Ray metadata 'federation'", zap.Error(err))
	}

	if err := pm.RecordFederationPerformance(ctx, domain, operation, float64(duration.Milliseconds()), success); err != nil {
		zap.L().Warn("failed to record federation performance", zap.Error(err))
	}

	return err
}

// TraceGraphQLQuery wraps a GraphQL query with X-Ray tracing
func (pm *PerformanceMonitor) TraceGraphQLQuery(ctx context.Context, queryName string, complexity int, fn func(context.Context) error) error {
	ctx, seg := xray.BeginSubsegment(ctx, "GraphQL "+queryName)
	defer seg.Close(nil)

	if err := seg.AddAnnotation("query_name", queryName); err != nil {
		zap.L().Warn("failed to add X-Ray annotation 'query_name'", zap.Error(err))
	}
	if err := seg.AddAnnotation("complexity", complexity); err != nil {
		zap.L().Warn("failed to add X-Ray annotation 'complexity'", zap.Error(err))
	}
	seg.Namespace = "graphql"

	start := time.Now()
	err := fn(ctx)
	duration := time.Since(start)

	if err != nil {
		_ = seg.AddError(err)
		if recordErr := pm.RecordError(ctx, "graphql_query", err.Error()); recordErr != nil {
			zap.L().Warn("failed to record graphql error metric", zap.Error(recordErr))
		}
	}

	if err := seg.AddMetadata("graphql", map[string]any{
		"duration_ms": duration.Milliseconds(),
		"complexity":  complexity,
	}); err != nil {
		zap.L().Warn("failed to add X-Ray metadata 'graphql'", zap.Error(err))
	}

	_ = pm.RecordLatency(ctx, "graphql_"+queryName, float64(duration.Milliseconds()))
	if recordErr := pm.RecordQueryComplexity(ctx, queryName, complexity); recordErr != nil {
		zap.L().Warn("failed to record query complexity metric", zap.Error(recordErr))
	}

	return err
}

// TraceLambdaHandler wraps a Lambda handler with X-Ray tracing and cold start detection
func (pm *PerformanceMonitor) TraceLambdaHandler(ctx context.Context, functionName string, fn func(context.Context) error) error {
	ctx, seg := xray.BeginSegment(ctx, functionName)
	defer seg.Close(nil)

	if err := seg.AddAnnotation("function_name", functionName); err != nil {
		zap.L().Warn("failed to add X-Ray annotation 'function_name'", zap.Error(err))
	}
	if err := seg.AddAnnotation("environment", pm.environment); err != nil {
		zap.L().Warn("failed to add X-Ray annotation 'environment'", zap.Error(err))
	}

	coldStart := false
	initDuration := 0.0

	if coldStart {
		if err := seg.AddAnnotation("cold_start", true); err != nil {
			zap.L().Warn("failed to add X-Ray annotation 'cold_start'", zap.Error(err))
		}
		if err := pm.RecordLambdaColdStart(ctx, functionName, coldStart, initDuration); err != nil {
			zap.L().Warn("failed to record Lambda cold start", zap.Error(err))
		}
	}

	start := time.Now()
	err := fn(ctx)
	duration := time.Since(start)

	if err != nil {
		_ = seg.AddError(err)
		if recordErr := pm.RecordError(ctx, "lambda_handler", err.Error()); recordErr != nil {
			zap.L().Warn("failed to record lambda error metric", zap.Error(recordErr))
		}
	}

	if err := seg.AddMetadata("lambda", map[string]any{
		"duration_ms": duration.Milliseconds(),
		"cold_start":  coldStart,
	}); err != nil {
		zap.L().Warn("failed to add X-Ray metadata 'lambda'", zap.Error(err))
	}

	_ = pm.RecordLatency(ctx, "lambda_"+functionName, float64(duration.Milliseconds()))

	return err
}

// GetPerformanceInsights queries CloudWatch for performance insights
func (pm *PerformanceMonitor) GetPerformanceInsights(ctx context.Context, metricName string, startTime, endTime time.Time) ([]types.Datapoint, error) {
	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String(pm.namespace),
		MetricName: aws.String(metricName),
		StartTime:  aws.Time(startTime),
		EndTime:    aws.Time(endTime),
		Period:     aws.Int32(300), // 5 minute periods
		Statistics: []types.Statistic{
			types.StatisticAverage,
			types.StatisticMaximum,
			types.StatisticMinimum,
			types.StatisticSum,
		},
	}

	result, err := pm.cloudwatch.GetMetricStatistics(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get metric statistics: %w", err)
	}

	return result.Datapoints, nil
}

// CreateAlarm creates a CloudWatch alarm for monitoring
func (pm *PerformanceMonitor) CreateAlarm(ctx context.Context, alarmName, metricName string, threshold float64, comparisonOperator types.ComparisonOperator) error {
	input := &cloudwatch.PutMetricAlarmInput{
		AlarmName:          aws.String(alarmName),
		ComparisonOperator: comparisonOperator,
		EvaluationPeriods:  aws.Int32(2),
		MetricName:         aws.String(metricName),
		Namespace:          aws.String(pm.namespace),
		Period:             aws.Int32(300),
		Statistic:          types.StatisticAverage,
		Threshold:          aws.Float64(threshold),
		ActionsEnabled:     aws.Bool(true),
		AlarmDescription:   aws.String(fmt.Sprintf("Alarm for %s metric", metricName)),
		Unit:               types.StandardUnitCount,
	}

	_, err := pm.cloudwatch.PutMetricAlarm(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create alarm: %w", err)
	}

	return nil
}
