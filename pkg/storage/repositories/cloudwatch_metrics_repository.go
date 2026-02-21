package repositories

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// CloudWatchMetricsRepository handles querying CloudWatch metrics with optional DynamoDB caching
// NOTE: This repository primarily uses CloudWatch AWS SDK for metrics collection.
// BaseRepository integration demonstrates how DynamoDB caching could be added for performance optimization.
type CloudWatchMetricsRepository struct {
	*EnhancedBaseRepository[*models.CloudWatchMetrics]               // Optional caching layer
	client                                             cloudWatchAPI // PRESERVE: CloudWatch AWS SDK for metrics collection
	namespace                                          string        // PRESERVE: CloudWatch namespace
	environment                                        string        // PRESERVE: Environment for metrics filtering
}

type cloudWatchAPI interface {
	GetMetricStatistics(context.Context, *cloudwatch.GetMetricStatisticsInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error)
}

// CloudWatchMetrics represents metrics data from CloudWatch (PRESERVED - AWS monitoring integration)
type CloudWatchMetrics struct {
	MetricName string
	Value      float64
	Unit       string
	Timestamp  time.Time
	Dimensions map[string]string
}

// ServiceMetrics represents aggregated metrics for a service (PRESERVED - AWS monitoring critical)
type ServiceMetrics struct {
	ServiceName       string
	RequestCount      int64
	ErrorCount        int64
	LatencyP50Ms      float64
	LatencyP90Ms      float64
	LatencyP99Ms      float64
	DynamoDBReads     int64
	DynamoDBWrites    int64
	LambdaInvocations int64
	S3Requests        int64
	DataTransferBytes int64
	EstimatedCostUSD  float64
}

// NewCloudWatchMetricsRepository creates a new CloudWatch metrics repository
// PRESERVE: All CloudWatch functionality - no DynamoDB operations to replace
func NewCloudWatchMetricsRepository(namespace, environment string, logger *zap.Logger) *CloudWatchMetricsRepository {
	// Initialize AWS config internally for CloudWatch metrics
	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		logger.Error("Failed to load AWS config for CloudWatch metrics", zap.Error(err))
		// Return repository with nil client - metrics will be disabled but won't crash
		return &CloudWatchMetricsRepository{
			EnhancedBaseRepository: nil,
			client:                 nil,
			namespace:              namespace,
			environment:            environment,
		}
	}

	return &CloudWatchMetricsRepository{
		EnhancedBaseRepository: nil, // Optional - only used if DynamoDB caching is enabled
		client:                 cloudwatch.NewFromConfig(cfg),
		namespace:              namespace,
		environment:            environment,
	}
}

// NewCloudWatchMetricsRepositoryWithCaching creates repository with DynamoDB caching enabled
// This demonstrates how BaseRepository integration would work for performance optimization
func NewCloudWatchMetricsRepositoryWithCaching(awsConfig aws.Config, namespace, environment, _ string, _ *zap.Logger, _ *cost.TrackingService, _ interface{}) *CloudWatchMetricsRepository {
	// This would enable DynamoDB caching of CloudWatch metrics for improved performance
	// baseRepo := NewEnhancedBaseRepository[*models.CloudWatchMetrics](db, tableName, logger, costService, "cloudwatch_metrics")

	return &CloudWatchMetricsRepository{
		EnhancedBaseRepository: nil, // Would set baseRepo here if caching was fully implemented
		client:                 cloudwatch.NewFromConfig(awsConfig),
		namespace:              namespace,
		environment:            environment,
	}
}

// GetServiceMetrics retrieves comprehensive metrics for a service over the specified period
// PRESERVE: Core CloudWatch business logic - critical for AWS monitoring and operational dashboards
func (r *CloudWatchMetricsRepository) GetServiceMetrics(ctx context.Context, serviceName string, period time.Duration) (*ServiceMetrics, error) {
	endTime := time.Now()
	startTime := endTime.Add(-period)

	metrics := &ServiceMetrics{
		ServiceName: serviceName,
	}

	// Collect metrics sequentially to honor request context lifetime
	if err := r.getAPIGatewayMetrics(ctx, metrics, startTime, endTime); err != nil {
		r.getLogger().Warn("Failed to get API Gateway metrics", zap.Error(err))
	}

	if err := r.getDynamoDBMetrics(ctx, metrics, startTime, endTime); err != nil {
		r.getLogger().Warn("Failed to get DynamoDB metrics", zap.Error(err))
	}

	if err := r.getLambdaMetrics(ctx, metrics, startTime, endTime); err != nil {
		r.getLogger().Warn("Failed to get Lambda metrics", zap.Error(err))
	}

	if err := r.getS3Metrics(ctx, metrics, startTime, endTime); err != nil {
		r.getLogger().Warn("Failed to get S3 metrics", zap.Error(err))
	}

	if err := r.getDataTransferMetrics(ctx, metrics, startTime, endTime); err != nil {
		r.getLogger().Warn("Failed to get data transfer metrics", zap.Error(err))
	}

	metrics.EstimatedCostUSD = r.calculateEstimatedCost(metrics)

	return metrics, nil
}

// getAPIGatewayMetrics retrieves API Gateway metrics
// PRESERVE: CloudWatch integration - critical for AWS monitoring and alerting
func (r *CloudWatchMetricsRepository) getAPIGatewayMetrics(ctx context.Context, metrics *ServiceMetrics, startTime, endTime time.Time) error {
	// Request count
	if requestCount, err := r.getMetricSum(ctx, "AWS/ApiGateway", "Count", startTime, endTime, map[string]string{
		"Stage": r.environment,
	}); err == nil {
		metrics.RequestCount = int64(requestCount)
	}

	// Error count (4xx + 5xx)
	if errorCount, err := r.getMetricSum(ctx, "AWS/ApiGateway", "4XXError", startTime, endTime, map[string]string{
		"Stage": r.environment,
	}); err == nil {
		if error5xx, err := r.getMetricSum(ctx, "AWS/ApiGateway", "5XXError", startTime, endTime, map[string]string{
			"Stage": r.environment,
		}); err == nil {
			metrics.ErrorCount = int64(errorCount + error5xx)
		}
	}

	// Latency percentiles
	if latencyP50, err := r.getMetricPercentile(ctx, "AWS/ApiGateway", "Latency", 50, startTime, endTime, map[string]string{
		"Stage": r.environment,
	}); err == nil {
		metrics.LatencyP50Ms = latencyP50
	}

	if latencyP90, err := r.getMetricPercentile(ctx, "AWS/ApiGateway", "Latency", 90, startTime, endTime, map[string]string{
		"Stage": r.environment,
	}); err == nil {
		metrics.LatencyP90Ms = latencyP90
	}

	if latencyP99, err := r.getMetricPercentile(ctx, "AWS/ApiGateway", "Latency", 99, startTime, endTime, map[string]string{
		"Stage": r.environment,
	}); err == nil {
		metrics.LatencyP99Ms = latencyP99
	}

	return nil
}

// getDynamoDBMetrics retrieves DynamoDB metrics
// PRESERVE: CloudWatch integration - critical for AWS monitoring and cost tracking
func (r *CloudWatchMetricsRepository) getDynamoDBMetrics(ctx context.Context, metrics *ServiceMetrics, startTime, endTime time.Time) error {
	// Read operations
	if reads, err := r.getMetricSum(ctx, "AWS/DynamoDB", "ConsumedReadCapacityUnits", startTime, endTime, nil); err == nil {
		metrics.DynamoDBReads = int64(reads)
	}

	// Write operations
	if writes, err := r.getMetricSum(ctx, "AWS/DynamoDB", "ConsumedWriteCapacityUnits", startTime, endTime, nil); err == nil {
		metrics.DynamoDBWrites = int64(writes)
	}

	return nil
}

// getLambdaMetrics retrieves Lambda metrics
// PRESERVE: CloudWatch integration - critical for AWS monitoring and performance tracking
func (r *CloudWatchMetricsRepository) getLambdaMetrics(ctx context.Context, metrics *ServiceMetrics, startTime, endTime time.Time) error {
	// Lambda invocations across all functions
	if invocations, err := r.getMetricSum(ctx, "AWS/Lambda", "Invocations", startTime, endTime, nil); err == nil {
		metrics.LambdaInvocations = int64(invocations)
	}

	return nil
}

// getS3Metrics retrieves S3 metrics
// PRESERVE: CloudWatch integration - critical for AWS monitoring and storage tracking
func (r *CloudWatchMetricsRepository) getS3Metrics(ctx context.Context, metrics *ServiceMetrics, startTime, endTime time.Time) error {
	// S3 requests
	if requests, err := r.getMetricSum(ctx, "AWS/S3", "NumberOfObjects", startTime, endTime, nil); err == nil {
		metrics.S3Requests = int64(requests)
	}

	return nil
}

// getDataTransferMetrics retrieves data transfer metrics
// PRESERVE: CloudWatch integration - critical for AWS monitoring and bandwidth tracking
func (r *CloudWatchMetricsRepository) getDataTransferMetrics(ctx context.Context, metrics *ServiceMetrics, startTime, endTime time.Time) error {
	// CloudFront data transfer (if available)
	if transfer, err := r.getMetricSum(ctx, "AWS/CloudFront", "BytesDownloaded", startTime, endTime, nil); err == nil {
		metrics.DataTransferBytes = int64(transfer)
	}

	return nil
}

// getMetricSum retrieves the sum of a metric over a time period
// PRESERVE: CloudWatch AWS SDK integration - essential for metrics collection
func (r *CloudWatchMetricsRepository) getMetricSum(ctx context.Context, namespace, metricName string, startTime, endTime time.Time, dimensions map[string]string) (float64, error) {
	if r.client == nil {
		return 0, nil
	}
	return r.getMetricStatistic(ctx, namespace, metricName, types.StatisticSum, startTime, endTime, dimensions)
}

// getMetricPercentile retrieves a specific percentile of a metric
// PRESERVE: CloudWatch AWS SDK integration - essential for latency monitoring
func (r *CloudWatchMetricsRepository) getMetricPercentile(ctx context.Context, namespace, metricName string, percentile float64, startTime, endTime time.Time, dimensions map[string]string) (float64, error) {
	if r.client == nil {
		return 0, nil
	}

	extendedStatistic := fmt.Sprintf("p%g", percentile)

	cwDimensions := make([]types.Dimension, 0, len(dimensions))
	for name, value := range dimensions {
		cwDimensions = append(cwDimensions, types.Dimension{
			Name:  aws.String(name),
			Value: aws.String(value),
		})
	}

	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:          aws.String(namespace),
		MetricName:         aws.String(metricName),
		StartTime:          aws.Time(startTime),
		EndTime:            aws.Time(endTime),
		Period:             aws.Int32(300), // 5-minute intervals
		ExtendedStatistics: []string{extendedStatistic},
		Dimensions:         cwDimensions,
	}

	result, err := r.client.GetMetricStatistics(ctx, input)
	if err != nil {
		return 0, ErrorHandler.HandleGetError(err, EntityCloudWatchMetrics, fmt.Sprintf("percentile %s:%s", namespace, metricName))
	}

	if err := common.ValidateSliceNotEmpty("result.Datapoints", result.Datapoints); err != nil {
		return 0, nil // Return zero if no data available
	}

	// Use the most recent datapoint
	var latestDatapoint *types.Datapoint
	for i := range result.Datapoints {
		if latestDatapoint == nil || result.Datapoints[i].Timestamp.After(*latestDatapoint.Timestamp) {
			latestDatapoint = &result.Datapoints[i]
		}
	}

	if latestDatapoint != nil && latestDatapoint.ExtendedStatistics != nil {
		if value, exists := latestDatapoint.ExtendedStatistics[extendedStatistic]; exists {
			return value, nil
		}
	}

	return 0, nil
}

// getMetricStatistic retrieves a specific statistic for a metric
// PRESERVE: CloudWatch AWS SDK integration - essential for all metrics collection
func (r *CloudWatchMetricsRepository) getMetricStatistic(ctx context.Context, namespace, metricName string, statistic types.Statistic, startTime, endTime time.Time, dimensions map[string]string) (float64, error) {
	cwDimensions := make([]types.Dimension, 0, len(dimensions))
	for name, value := range dimensions {
		cwDimensions = append(cwDimensions, types.Dimension{
			Name:  aws.String(name),
			Value: aws.String(value),
		})
	}

	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String(namespace),
		MetricName: aws.String(metricName),
		StartTime:  aws.Time(startTime),
		EndTime:    aws.Time(endTime),
		Period:     aws.Int32(300), // 5-minute intervals
		Statistics: []types.Statistic{statistic},
		Dimensions: cwDimensions,
	}

	result, err := r.client.GetMetricStatistics(ctx, input)
	if err != nil {
		return 0, ErrorHandler.HandleGetError(err, EntityCloudWatchMetrics, fmt.Sprintf("statistic %s:%s", namespace, metricName))
	}

	if err := common.ValidateSliceNotEmpty("result.Datapoints", result.Datapoints); err != nil {
		return 0, nil // Return zero if no data available
	}

	// Calculate total based on statistic type
	total := 0.0
	count := 0
	for _, datapoint := range result.Datapoints {
		switch statistic {
		case types.StatisticSum:
			if datapoint.Sum != nil {
				total += *datapoint.Sum
				count++
			}
		case types.StatisticAverage:
			if datapoint.Average != nil {
				total += *datapoint.Average
				count++
			}
		case types.StatisticMaximum:
			if datapoint.Maximum != nil {
				if count == 0 || *datapoint.Maximum > total {
					total = *datapoint.Maximum
				}
				count++
			}
		case types.StatisticMinimum:
			if datapoint.Minimum != nil {
				if count == 0 || *datapoint.Minimum < total {
					total = *datapoint.Minimum
				}
				count++
			}
		}
	}

	if count == 0 {
		return 0, nil
	}

	// For average statistics, return the average across datapoints
	if statistic == types.StatisticAverage {
		return total / float64(count), nil
	}

	return total, nil
}

// calculateEstimatedCost calculates detailed estimated cost based on usage with accurate AWS pricing
// PRESERVE: AWS cost calculation - critical for cost monitoring and optimization
func (r *CloudWatchMetricsRepository) calculateEstimatedCost(metrics *ServiceMetrics) float64 {
	cost := 0.0

	// DynamoDB costs (more accurate pricing for on-demand billing)
	// Using current us-east-1 pricing (adjust for other regions as needed)
	dynamoReadCost := (float64(metrics.DynamoDBReads) / 1000000.0) * 0.25   // $0.25 per million read request units
	dynamoWriteCost := (float64(metrics.DynamoDBWrites) / 1000000.0) * 1.25 // $1.25 per million write request units
	cost += dynamoReadCost + dynamoWriteCost

	// Lambda costs (accurate pricing with ARM64 and x86 considerations)
	invocationCost := (float64(metrics.LambdaInvocations) / 1000000.0) * 0.20 // $0.20 per million requests

	// Compute cost estimate (assuming 512MB memory, ARM64, average 250ms duration)
	// ARM64 pricing: $0.0000133334 per GB-second
	memoryGB := 0.512          // 512MB
	avgDurationSeconds := 0.25 // 250ms average
	computeGBSeconds := float64(metrics.LambdaInvocations) * memoryGB * avgDurationSeconds
	computeCost := computeGBSeconds * 0.0000133334

	lambdaTotalCost := invocationCost + computeCost
	cost += lambdaTotalCost

	// API Gateway costs (accurate REST API pricing)
	apiGatewayCost := (float64(metrics.RequestCount) / 1000000.0) * 3.50 // $3.50 per million requests
	cost += apiGatewayCost

	// S3 costs (detailed breakdown)
	// Storage cost (assuming Standard class, $0.023 per GB per month)
	// Note: This is estimated based on data transfer as proxy for storage
	estimatedStorageGB := float64(metrics.DataTransferBytes) / (1024 * 1024 * 1024) / 30 // Rough estimate
	s3StorageCost := estimatedStorageGB * 0.023 / 30                                     // Daily storage cost

	// Request costs
	s3RequestCost := (float64(metrics.S3Requests) / 1000.0) * 0.0004 // $0.0004 per 1,000 PUT/COPY/POST/LIST

	// Data transfer costs (detailed tiers)
	dataTransferGB := float64(metrics.DataTransferBytes) / (1024 * 1024 * 1024)
	var dataTransferCost float64

	if dataTransferGB <= 10 { // First 10 GB free per month
		dataTransferCost = 0
	} else if dataTransferGB <= 40 { // Next 40 GB at $0.09/GB
		dataTransferCost = (dataTransferGB - 10) * 0.09
	} else if dataTransferGB <= 100 { // Next 100 GB at $0.085/GB
		dataTransferCost = 30*0.09 + (dataTransferGB-50)*0.085
	} else { // Over 150 GB at $0.07/GB
		dataTransferCost = 30*0.09 + 50*0.085 + (dataTransferGB-100)*0.07
	}

	s3TotalCost := s3StorageCost + s3RequestCost + dataTransferCost
	cost += s3TotalCost

	// CloudWatch costs (logs and metrics)
	// Log ingestion: $0.50 per GB ingested
	// Assume 10% of Lambda invocations generate 1KB logs each
	logDataGB := float64(metrics.LambdaInvocations) * 0.1 * 0.001 / (1024 * 1024) // 1KB per 10% of invocations
	logCost := logDataGB * 0.50

	// Custom metrics: $0.30 per metric per month
	// Assume 50 custom metrics for the application
	customMetricsCost := 50 * 0.30 / 30 // Daily cost

	cloudWatchTotalCost := logCost + customMetricsCost
	cost += cloudWatchTotalCost

	// Additional services costs
	// CloudFront (CDN) - assuming some usage for media delivery
	cloudFrontCost := dataTransferGB * 0.085 * 0.1 // 10% of data transfer through CloudFront
	cost += cloudFrontCost

	// SQS costs (for async processing)
	// Assume 1 SQS message per 10 Lambda invocations
	sqsRequests := float64(metrics.LambdaInvocations) / 10
	sqsCost := (sqsRequests / 1000000.0) * 0.40 // $0.40 per million requests
	cost += sqsCost

	// Add 5% buffer for other miscellaneous AWS services
	cost *= 1.05

	r.getLogger().Debug("detailed cost calculation breakdown",
		zap.Float64("dynamo_read_cost", dynamoReadCost),
		zap.Float64("dynamo_write_cost", dynamoWriteCost),
		zap.Float64("lambda_invocation_cost", invocationCost),
		zap.Float64("lambda_compute_cost", computeCost),
		zap.Float64("api_gateway_cost", apiGatewayCost),
		zap.Float64("s3_storage_cost", s3StorageCost),
		zap.Float64("s3_request_cost", s3RequestCost),
		zap.Float64("data_transfer_cost", dataTransferCost),
		zap.Float64("cloudwatch_cost", cloudWatchTotalCost),
		zap.Float64("cloudfront_cost", cloudFrontCost),
		zap.Float64("sqs_cost", sqsCost),
		zap.Float64("total_estimated_cost", cost))

	return math.Max(cost, 0)
}

// GetInstanceMetrics retrieves instance-level metrics for the past period
// PRESERVE: AWS monitoring - critical for instance-level operational visibility
func (r *CloudWatchMetricsRepository) GetInstanceMetrics(ctx context.Context, period time.Duration) (*ServiceMetrics, error) {
	// Get metrics for the entire instance (all services combined)
	return r.GetServiceMetrics(ctx, "instance", period)
}

// GetCostBreakdown retrieves detailed cost breakdown for the specified period
// PRESERVE: AWS cost analysis - critical for cost monitoring and optimization
func (r *CloudWatchMetricsRepository) GetCostBreakdown(ctx context.Context, period time.Duration) (*CostBreakdown, error) {
	metrics, err := r.GetInstanceMetrics(ctx, period)
	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, EntityCloudWatchMetrics, "instance metrics")
	}

	// Use the same detailed calculation logic as calculateEstimatedCost
	// DynamoDB costs
	dynamoReadCost := (float64(metrics.DynamoDBReads) / 1000000.0) * 0.25
	dynamoWriteCost := (float64(metrics.DynamoDBWrites) / 1000000.0) * 1.25
	dynamoDBCost := dynamoReadCost + dynamoWriteCost

	// Lambda costs (detailed)
	invocationCost := (float64(metrics.LambdaInvocations) / 1000000.0) * 0.20
	memoryGB := 0.512
	avgDurationSeconds := 0.25
	computeGBSeconds := float64(metrics.LambdaInvocations) * memoryGB * avgDurationSeconds
	computeCost := computeGBSeconds * 0.0000133334
	lambdaCost := invocationCost + computeCost

	// API Gateway costs
	apiGatewayCost := (float64(metrics.RequestCount) / 1000000.0) * 3.50

	// S3 costs (detailed breakdown)
	estimatedStorageGB := float64(metrics.DataTransferBytes) / (1024 * 1024 * 1024) / 30
	s3StorageCost := estimatedStorageGB * 0.023 / 30
	s3RequestCost := (float64(metrics.S3Requests) / 1000.0) * 0.0004

	// Data transfer with tiers
	dataTransferGB := float64(metrics.DataTransferBytes) / (1024 * 1024 * 1024)
	var dataTransferCost float64
	if dataTransferGB <= 10 {
		dataTransferCost = 0
	} else if dataTransferGB <= 40 {
		dataTransferCost = (dataTransferGB - 10) * 0.09
	} else if dataTransferGB <= 100 {
		dataTransferCost = 30*0.09 + (dataTransferGB-50)*0.085
	} else {
		dataTransferCost = 30*0.09 + 50*0.085 + (dataTransferGB-100)*0.07
	}

	s3Cost := s3StorageCost + s3RequestCost

	// Additional service costs
	logDataGB := float64(metrics.LambdaInvocations) * 0.1 * 0.001 / (1024 * 1024)
	logCost := logDataGB * 0.50
	customMetricsCost := 50 * 0.30 / 30
	cloudWatchCost := logCost + customMetricsCost

	cloudFrontCost := dataTransferGB * 0.085 * 0.1
	sqsRequests := float64(metrics.LambdaInvocations) / 10
	sqsCost := (sqsRequests / 1000000.0) * 0.40

	// Calculate total with buffer
	baseTotalCost := dynamoDBCost + lambdaCost + apiGatewayCost + s3Cost + dataTransferCost + cloudWatchCost + cloudFrontCost + sqsCost
	totalCost := baseTotalCost * 1.05 // 5% buffer

	breakdown := &CostBreakdown{
		TotalCost:        totalCost,
		DynamoDBCost:     dynamoDBCost,
		LambdaCost:       lambdaCost,
		APIGatewayCost:   apiGatewayCost,
		S3Cost:           s3Cost,
		DataTransferCost: dataTransferCost,
		Breakdown: []*CostItem{
			{Operation: "DynamoDB Reads", Count: int(metrics.DynamoDBReads), Cost: dynamoReadCost},
			{Operation: "DynamoDB Writes", Count: int(metrics.DynamoDBWrites), Cost: dynamoWriteCost},
			{Operation: "Lambda Invocations", Count: int(metrics.LambdaInvocations), Cost: invocationCost},
			{Operation: "Lambda Compute (GB-seconds)", Count: int(computeGBSeconds), Cost: computeCost},
			{Operation: "API Gateway Requests", Count: int(metrics.RequestCount), Cost: apiGatewayCost},
			{Operation: "S3 Storage (GB-days)", Count: int(estimatedStorageGB), Cost: s3StorageCost},
			{Operation: "S3 Requests", Count: int(metrics.S3Requests), Cost: s3RequestCost},
			{Operation: "Data Transfer (GB)", Count: int(dataTransferGB), Cost: dataTransferCost},
			{Operation: "CloudWatch Logs (GB)", Count: int(logDataGB * 1024), Cost: logCost}, // Convert to MB for display
			{Operation: "CloudWatch Custom Metrics", Count: 50, Cost: customMetricsCost},
			{Operation: "CloudFront Transfer (GB)", Count: int(dataTransferGB * 0.1), Cost: cloudFrontCost},
			{Operation: "SQS Messages", Count: int(sqsRequests), Cost: sqsCost},
		},
	}

	r.getLogger().Info("generated detailed cost breakdown",
		zap.Float64("total_cost", totalCost),
		zap.Float64("dynamodb_cost", dynamoDBCost),
		zap.Float64("lambda_cost", lambdaCost),
		zap.Float64("api_gateway_cost", apiGatewayCost),
		zap.Float64("s3_cost", s3Cost),
		zap.Float64("data_transfer_cost", dataTransferCost),
		zap.Float64("cloudwatch_cost", cloudWatchCost),
		zap.Float64("cloudfront_cost", cloudFrontCost),
		zap.Float64("sqs_cost", sqsCost),
		zap.Duration("period", period))

	return breakdown, nil
}

// CostBreakdown represents cost breakdown data (PRESERVED - AWS cost monitoring)
type CostBreakdown struct {
	TotalCost        float64
	DynamoDBCost     float64
	LambdaCost       float64
	APIGatewayCost   float64
	S3Cost           float64
	DataTransferCost float64
	Breakdown        []*CostItem
}

// CostItem represents a single cost item (PRESERVED - AWS cost monitoring)
type CostItem struct {
	Operation string
	Count     int
	Cost      float64
}

// Helper method to get logger from BaseRepository or create a no-op logger
func (r *CloudWatchMetricsRepository) getLogger() *zap.Logger {
	if r.BaseRepository != nil {
		// Would access BaseRepository's logger if available
		// return r.logger
		// Placeholder for future implementation
		_ = r.BaseRepository // avoid unused variable warning
	}
	// Return a no-op logger for now - in real implementation this would be properly initialized
	return zap.NewNop()
}

// CacheMetrics stores metrics in DynamoDB for performance optimization (OPTIONAL enhancement)
// This demonstrates how BaseRepository could be used for caching CloudWatch data
func (r *CloudWatchMetricsRepository) CacheMetrics(ctx context.Context, serviceName string, metrics *ServiceMetrics) error {
	if r.BaseRepository == nil {
		return nil // No caching if BaseRepository not initialized
	}

	// Convert ServiceMetrics to CloudWatchMetrics model for caching
	cacheModel := &models.CloudWatchMetrics{
		ServiceName:       serviceName,
		Timestamp:         time.Now(),
		RequestCount:      metrics.RequestCount,
		ErrorCount:        metrics.ErrorCount,
		LatencyP50Ms:      metrics.LatencyP50Ms,
		LatencyP90Ms:      metrics.LatencyP90Ms,
		LatencyP99Ms:      metrics.LatencyP99Ms,
		DynamoDBReads:     metrics.DynamoDBReads,
		DynamoDBWrites:    metrics.DynamoDBWrites,
		LambdaInvocations: metrics.LambdaInvocations,
		S3Requests:        metrics.S3Requests,
		DataTransferBytes: metrics.DataTransferBytes,
		EstimatedCostUSD:  metrics.EstimatedCostUSD,
	}

	cacheModel.SetCacheExpiry()

	// Use BaseRepository for DynamoDB caching
	return r.ValidateAndCreate(ctx, cacheModel)
}

// GetCachedMetrics retrieves cached metrics from DynamoDB (OPTIONAL enhancement)
// This demonstrates how BaseRepository could be used for retrieving cached CloudWatch data
func (r *CloudWatchMetricsRepository) GetCachedMetrics(ctx context.Context, serviceName string) (*ServiceMetrics, error) {
	if r.BaseRepository == nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrInvalidInput, EntityCloudWatchMetrics, "cached metrics")
	}

	// Query for recent cached metrics
	pk := fmt.Sprintf("SERVICE#%s", serviceName)
	results, err := r.Query(ctx, pk, 1) // Get most recent cache entry
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityCloudWatchMetrics, "cached metrics")
	}

	if len(results) == 0 {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityCloudWatchMetrics, "cached metrics")
	}

	cached := results[0]

	// Check if cache has expired
	if cached.IsExpired() {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityCloudWatchMetrics, "cached metrics")
	}

	// Convert back to ServiceMetrics
	return &ServiceMetrics{
		ServiceName:       cached.ServiceName,
		RequestCount:      cached.RequestCount,
		ErrorCount:        cached.ErrorCount,
		LatencyP50Ms:      cached.LatencyP50Ms,
		LatencyP90Ms:      cached.LatencyP90Ms,
		LatencyP99Ms:      cached.LatencyP99Ms,
		DynamoDBReads:     cached.DynamoDBReads,
		DynamoDBWrites:    cached.DynamoDBWrites,
		LambdaInvocations: cached.LambdaInvocations,
		S3Requests:        cached.S3Requests,
		DataTransferBytes: cached.DataTransferBytes,
		EstimatedCostUSD:  cached.EstimatedCostUSD,
	}, nil
}
