// Package performance provides performance monitoring and metrics aggregation services.
// It integrates with AWS CloudWatch to collect Lambda, DynamoDB, and service metrics,
// providing comprehensive performance insights for operational dashboards and alerting.
package performance

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

type cloudWatchAPI interface {
	GetMetricStatistics(ctx context.Context, params *cloudwatch.GetMetricStatisticsInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error)
}

// Service provides performance monitoring functionality
type Service struct {
	cloudWatch  cloudWatchAPI
	logger      *zap.Logger
	environment string
}

// NewService creates a new performance monitoring service
func NewService(cloudWatch cloudWatchAPI, environment string, logger *zap.Logger) *Service {
	return &Service{
		cloudWatch:  cloudWatch,
		logger:      logger,
		environment: environment,
	}
}

// GetPerformanceMetrics retrieves comprehensive performance metrics for a service category
func (s *Service) GetPerformanceMetrics(ctx context.Context, serviceCategory model.ServiceCategory, period model.TimePeriod) (*model.PerformanceReport, error) {
	if err := common.ValidateRequiredParam("serviceCategory", string(serviceCategory)); err != nil {
		return nil, err
	}

	// Convert period to time range
	duration := s.periodToDuration(period)
	endTime := time.Now()
	startTime := endTime.Add(-duration)

	// Get Lambda function names for the service
	functionNames := s.getServiceFunctionNames(serviceCategory)
	if len(functionNames) == 0 {
		s.logger.Warn("no Lambda functions found for service category",
			zap.String("category", string(serviceCategory)))
		return s.emptyReport(serviceCategory, period), nil
	}

	// Collect metrics from all functions in parallel
	metrics := s.aggregateMetricsFromFunctions(ctx, functionNames, startTime, endTime)

	// Calculate percentiles
	p50 := s.calculatePercentile(metrics.durations, 0.50)
	p95 := s.calculatePercentile(metrics.durations, 0.95)
	p99 := s.calculatePercentile(metrics.durations, 0.99)

	// Calculate error rate
	errorRate := float64(0)
	if metrics.totalInvocations > 0 {
		errorRate = float64(metrics.totalErrors) / float64(metrics.totalInvocations)
	}

	// Calculate throughput (requests per second)
	throughput := float64(metrics.totalInvocations) / duration.Seconds()

	return &model.PerformanceReport{
		Service:    serviceCategory,
		P50Latency: model.Duration(p50),
		P95Latency: model.Duration(p95),
		P99Latency: model.Duration(p99),
		ErrorRate:  errorRate,
		Throughput: throughput,
		ColdStarts: int(metrics.coldStarts),
		Period:     period,
	}, nil
}

// metricsAggregator holds aggregated metrics data
type metricsAggregator struct {
	durations        []float64
	totalInvocations int64
	totalErrors      int64
	coldStarts       int64
}

// aggregateMetricsFromFunctions collects metrics from multiple Lambda functions
func (s *Service) aggregateMetricsFromFunctions(ctx context.Context, functionNames []string, startTime, endTime time.Time) *metricsAggregator {
	aggregator := &metricsAggregator{
		durations: make([]float64, 0),
	}

	for _, functionName := range functionNames {
		// Get invocation count
		invocations, err := s.getMetricSum(ctx, "AWS/Lambda", "Invocations", startTime, endTime, functionName)
		if err != nil {
			s.logger.Warn("failed to get invocations metric",
				zap.String("function", functionName),
				zap.Error(err))
		} else {
			aggregator.totalInvocations += int64(invocations)
		}

		// Get error count
		errors, err := s.getMetricSum(ctx, "AWS/Lambda", "Errors", startTime, endTime, functionName)
		if err != nil {
			s.logger.Warn("failed to get errors metric",
				zap.String("function", functionName),
				zap.Error(err))
		} else {
			aggregator.totalErrors += int64(errors)
		}

		// Get cold starts (if available - this is a custom metric)
		coldStarts, err := s.getMetricSum(ctx, "AWS/Lambda", "ColdStarts", startTime, endTime, functionName)
		if err == nil {
			aggregator.coldStarts += int64(coldStarts)
		}

		// Get duration statistics for percentile calculation
		durations, err := s.getMetricDatapoints(ctx, "AWS/Lambda", "Duration", startTime, endTime, functionName)
		if err != nil {
			s.logger.Warn("failed to get duration datapoints",
				zap.String("function", functionName),
				zap.Error(err))
		} else {
			aggregator.durations = append(aggregator.durations, durations...)
		}
	}

	return aggregator
}

// getMetricSum retrieves the sum of a metric for a Lambda function
func (s *Service) getMetricSum(ctx context.Context, namespace, metricName string, startTime, endTime time.Time, functionName string) (float64, error) {
	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String(namespace),
		MetricName: aws.String(metricName),
		Dimensions: []types.Dimension{
			{
				Name:  aws.String("FunctionName"),
				Value: aws.String(functionName),
			},
		},
		StartTime:  aws.Time(startTime),
		EndTime:    aws.Time(endTime),
		Period:     aws.Int32(300), // 5-minute intervals
		Statistics: []types.Statistic{types.StatisticSum},
	}

	result, err := s.cloudWatch.GetMetricStatistics(ctx, input)
	if err != nil {
		return 0, fmt.Errorf("failed to get CloudWatch metric: %w", err)
	}

	if err := common.ValidateSliceNotEmpty("datapoints", result.Datapoints); err != nil {
		return 0, nil // No data available
	}

	// Sum all datapoints
	var total float64
	for _, datapoint := range result.Datapoints {
		if datapoint.Sum != nil {
			total += *datapoint.Sum
		}
	}

	return total, nil
}

// getMetricDatapoints retrieves all datapoint values for a metric
func (s *Service) getMetricDatapoints(ctx context.Context, namespace, metricName string, startTime, endTime time.Time, functionName string) ([]float64, error) {
	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String(namespace),
		MetricName: aws.String(metricName),
		Dimensions: []types.Dimension{
			{
				Name:  aws.String("FunctionName"),
				Value: aws.String(functionName),
			},
		},
		StartTime:  aws.Time(startTime),
		EndTime:    aws.Time(endTime),
		Period:     aws.Int32(60), // 1-minute intervals for better granularity
		Statistics: []types.Statistic{types.StatisticAverage},
	}

	result, err := s.cloudWatch.GetMetricStatistics(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get CloudWatch metric datapoints: %w", err)
	}

	if err := common.ValidateSliceNotEmpty("datapoints", result.Datapoints); err != nil {
		return []float64{}, nil // No data available
	}

	values := make([]float64, 0, len(result.Datapoints))
	for _, datapoint := range result.Datapoints {
		if datapoint.Average != nil {
			values = append(values, *datapoint.Average)
		}
	}

	return values, nil
}

// calculatePercentile calculates a percentile from a slice of values
func (s *Service) calculatePercentile(values []float64, percentile float64) time.Duration {
	if len(values) == 0 {
		return 0
	}

	// Sort values for percentile calculation
	sorted := make([]float64, len(values))
	copy(sorted, values)

	// Simple bubble sort (sufficient for typical dataset sizes)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Calculate index
	index := int(float64(len(sorted)) * percentile)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	// Convert milliseconds to duration
	return time.Duration(sorted[index]) * time.Millisecond
}

// getServiceFunctionNames maps service categories to Lambda function names
func (s *Service) getServiceFunctionNames(category model.ServiceCategory) []string {
	prefix := fmt.Sprintf("lesser-%s-", s.environment)

	switch category {
	case model.ServiceCategoryGraphqlAPI:
		return []string{
			prefix + "graphql",
			prefix + "api",
		}
	case model.ServiceCategoryFederationDelivery:
		return []string{
			prefix + "federation-delivery",
			prefix + "federation-tracker",
			prefix + "inbox",
			prefix + "outbox",
		}
	case model.ServiceCategoryMediaProcessor:
		return []string{
			prefix + "media-processor",
		}
	case model.ServiceCategoryModerationEngine:
		return []string{
			prefix + "moderation-processor",
			prefix + "ai-processor",
		}
	case model.ServiceCategorySearchIndexer:
		return []string{
			prefix + "search-indexer",
			prefix + "status-indexer",
		}
	case model.ServiceCategoryStreamingService:
		return []string{
			prefix + "streaming",
			prefix + "stream-router",
		}
	default:
		return []string{}
	}
}

// periodToDuration converts a TimePeriod to a time.Duration
func (s *Service) periodToDuration(period model.TimePeriod) time.Duration {
	switch period {
	case model.TimePeriodHour:
		return time.Hour
	case model.TimePeriodDay:
		return 24 * time.Hour
	case model.TimePeriodWeek:
		return 7 * 24 * time.Hour
	case model.TimePeriodMonth:
		return 30 * 24 * time.Hour
	default:
		return time.Hour
	}
}

// emptyReport creates an empty performance report
func (s *Service) emptyReport(category model.ServiceCategory, period model.TimePeriod) *model.PerformanceReport {
	return &model.PerformanceReport{
		Service:    category,
		P50Latency: 0,
		P95Latency: 0,
		P99Latency: 0,
		ErrorRate:  0,
		Throughput: 0,
		ColdStarts: 0,
		Period:     period,
	}
}
