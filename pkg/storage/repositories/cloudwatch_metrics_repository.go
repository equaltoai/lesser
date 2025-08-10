package repositories

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"go.uber.org/zap"
)

// CloudWatchMetricsRepository handles querying CloudWatch metrics
type CloudWatchMetricsRepository struct {
	client      *cloudwatch.Client
	logger      *zap.Logger
	namespace   string
	environment string
}

// CloudWatchMetrics represents metrics data from CloudWatch
type CloudWatchMetrics struct {
	MetricName string
	Value      float64
	Unit       string
	Timestamp  time.Time
	Dimensions map[string]string
}

// ServiceMetrics represents aggregated metrics for a service
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
func NewCloudWatchMetricsRepository(awsConfig aws.Config, namespace, environment string, logger *zap.Logger) *CloudWatchMetricsRepository {
	return &CloudWatchMetricsRepository{
		client:      cloudwatch.NewFromConfig(awsConfig),
		logger:      logger,
		namespace:   namespace,
		environment: environment,
	}
}

// GetServiceMetrics retrieves comprehensive metrics for a service over the specified period
func (r *CloudWatchMetricsRepository) GetServiceMetrics(ctx context.Context, serviceName string, period time.Duration) (*ServiceMetrics, error) {
	endTime := time.Now()
	startTime := endTime.Add(-period)

	metrics := &ServiceMetrics{
		ServiceName: serviceName,
	}

	// Query all metrics in parallel
	errChan := make(chan error, 6)

	// API Gateway metrics (requests, latency, errors)
	go func() {
		if err := r.getAPIGatewayMetrics(ctx, metrics, startTime, endTime); err != nil {
			r.logger.Warn("Failed to get API Gateway metrics", zap.Error(err))
		}
		errChan <- nil
	}()

	// DynamoDB metrics
	go func() {
		if err := r.getDynamoDBMetrics(ctx, metrics, startTime, endTime); err != nil {
			r.logger.Warn("Failed to get DynamoDB metrics", zap.Error(err))
		}
		errChan <- nil
	}()

	// Lambda metrics
	go func() {
		if err := r.getLambdaMetrics(ctx, metrics, startTime, endTime); err != nil {
			r.logger.Warn("Failed to get Lambda metrics", zap.Error(err))
		}
		errChan <- nil
	}()

	// S3 metrics
	go func() {
		if err := r.getS3Metrics(ctx, metrics, startTime, endTime); err != nil {
			r.logger.Warn("Failed to get S3 metrics", zap.Error(err))
		}
		errChan <- nil
	}()

	// Data transfer metrics
	go func() {
		if err := r.getDataTransferMetrics(ctx, metrics, startTime, endTime); err != nil {
			r.logger.Warn("Failed to get data transfer metrics", zap.Error(err))
		}
		errChan <- nil
	}()

	// Cost estimate
	go func() {
		metrics.EstimatedCostUSD = r.calculateEstimatedCost(metrics)
		errChan <- nil
	}()

	// Wait for all goroutines
	for i := 0; i < 6; i++ {
		<-errChan
	}

	return metrics, nil
}

// getAPIGatewayMetrics retrieves API Gateway metrics
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
func (r *CloudWatchMetricsRepository) getLambdaMetrics(ctx context.Context, metrics *ServiceMetrics, startTime, endTime time.Time) error {
	// Lambda invocations across all functions
	if invocations, err := r.getMetricSum(ctx, "AWS/Lambda", "Invocations", startTime, endTime, nil); err == nil {
		metrics.LambdaInvocations = int64(invocations)
	}

	return nil
}

// getS3Metrics retrieves S3 metrics
func (r *CloudWatchMetricsRepository) getS3Metrics(ctx context.Context, metrics *ServiceMetrics, startTime, endTime time.Time) error {
	// S3 requests
	if requests, err := r.getMetricSum(ctx, "AWS/S3", "NumberOfObjects", startTime, endTime, nil); err == nil {
		metrics.S3Requests = int64(requests)
	}

	return nil
}

// getDataTransferMetrics retrieves data transfer metrics
func (r *CloudWatchMetricsRepository) getDataTransferMetrics(ctx context.Context, metrics *ServiceMetrics, startTime, endTime time.Time) error {
	// CloudFront data transfer (if available)
	if transfer, err := r.getMetricSum(ctx, "AWS/CloudFront", "BytesDownloaded", startTime, endTime, nil); err == nil {
		metrics.DataTransferBytes = int64(transfer)
	}

	return nil
}

// getMetricSum retrieves the sum of a metric over a time period
func (r *CloudWatchMetricsRepository) getMetricSum(ctx context.Context, namespace, metricName string, startTime, endTime time.Time, dimensions map[string]string) (float64, error) {
	return r.getMetricStatistic(ctx, namespace, metricName, types.StatisticSum, startTime, endTime, dimensions)
}

// getMetricPercentile retrieves a specific percentile of a metric
func (r *CloudWatchMetricsRepository) getMetricPercentile(ctx context.Context, namespace, metricName string, percentile float64, startTime, endTime time.Time, dimensions map[string]string) (float64, error) {
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
		return 0, fmt.Errorf("failed to get metric percentile %s:%s: %w", namespace, metricName, err)
	}

	if len(result.Datapoints) == 0 {
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
		return 0, fmt.Errorf("failed to get metric statistic %s:%s: %w", namespace, metricName, err)
	}

	if len(result.Datapoints) == 0 {
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

// calculateEstimatedCost calculates estimated cost based on usage
func (r *CloudWatchMetricsRepository) calculateEstimatedCost(metrics *ServiceMetrics) float64 {
	cost := 0.0

	// DynamoDB costs (per million units)
	cost += (float64(metrics.DynamoDBReads) / 1000000.0) * 0.25  // $0.25 per million read units
	cost += (float64(metrics.DynamoDBWrites) / 1000000.0) * 1.25 // $1.25 per million write units

	// Lambda costs (per million invocations)
	cost += (float64(metrics.LambdaInvocations) / 1000000.0) * 0.20 // $0.20 per million requests
	// Add compute cost estimate (128MB, 0.5s average)
	cost += (float64(metrics.LambdaInvocations) * 0.128 * 0.5) / 1000000.0 * 0.00001667

	// API Gateway costs (per million requests)
	cost += (float64(metrics.RequestCount) / 1000000.0) * 3.50 // $3.50 per million requests

	// S3 costs (simplified)
	cost += (float64(metrics.DataTransferBytes) / 1024 / 1024 / 1024) * 0.09 // $0.09 per GB transfer

	// Add some buffer for other services (CloudWatch, etc.)
	cost *= 1.1

	return math.Max(cost, 0)
}

// GetInstanceMetrics retrieves instance-level metrics for the past period
func (r *CloudWatchMetricsRepository) GetInstanceMetrics(ctx context.Context, period time.Duration) (*ServiceMetrics, error) {
	// Get metrics for the entire instance (all services combined)
	return r.GetServiceMetrics(ctx, "instance", period)
}

// GetCostBreakdown retrieves cost breakdown for the specified period
func (r *CloudWatchMetricsRepository) GetCostBreakdown(ctx context.Context, period time.Duration) (*CostBreakdown, error) {
	metrics, err := r.GetInstanceMetrics(ctx, period)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance metrics: %w", err)
	}

	// Calculate costs for each service
	dynamoReadCost := (float64(metrics.DynamoDBReads) / 1000000.0) * 0.25
	dynamoWriteCost := (float64(metrics.DynamoDBWrites) / 1000000.0) * 1.25
	dynamoDBCost := dynamoReadCost + dynamoWriteCost

	lambdaCost := (float64(metrics.LambdaInvocations) / 1000000.0) * 0.20
	lambdaCost += (float64(metrics.LambdaInvocations) * 0.128 * 0.5) / 1000000.0 * 0.00001667

	apiGatewayCost := (float64(metrics.RequestCount) / 1000000.0) * 3.50

	s3StorageCost := 0.0 // Would need storage metrics
	s3RequestCost := (float64(metrics.S3Requests) / 1000.0) * 0.0004
	s3Cost := s3StorageCost + s3RequestCost

	dataTransferCost := (float64(metrics.DataTransferBytes) / 1024 / 1024 / 1024) * 0.09

	totalCost := dynamoDBCost + lambdaCost + apiGatewayCost + s3Cost + dataTransferCost

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
			{Operation: "Lambda Invocations", Count: int(metrics.LambdaInvocations), Cost: lambdaCost},
			{Operation: "API Gateway Requests", Count: int(metrics.RequestCount), Cost: apiGatewayCost},
			{Operation: "S3 Requests", Count: int(metrics.S3Requests), Cost: s3RequestCost},
			{Operation: "Data Transfer", Count: int(metrics.DataTransferBytes / 1024 / 1024), Cost: dataTransferCost}, // MB
		},
	}

	return breakdown, nil
}

// CostBreakdown represents cost breakdown data
type CostBreakdown struct {
	TotalCost        float64
	DynamoDBCost     float64
	LambdaCost       float64
	APIGatewayCost   float64
	S3Cost           float64
	DataTransferCost float64
	Breakdown        []*CostItem
}

// CostItem represents a single cost item
type CostItem struct {
	Operation string
	Count     int
	Cost      float64
}
