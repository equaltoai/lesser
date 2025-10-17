package graph

import (
	"context"
	"errors"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// CostBreakdown is the resolver for the costBreakdown field.
func (r *queryResolver) CostBreakdown(_ context.Context, period *model.Period) (*model.CostBreakdown, error) {
	p := model.PeriodDay
	if period != nil {
		p = *period
	}

	// Get actual cost breakdown from cost tracker
	var totalCost, dynamoDBCost, s3Cost, lambdaCost, transferCost float64
	var dbReads, dbWrites, s3Gets, s3Puts, lambdaInvocations int64

	if r.CostTracker != nil {
		// Calculate costs from current tracker data
		costData := r.CostTracker.CalculateCost()
		if costData != nil {
			// Store counts for later use
			dbReads = costData.DynamoDBReads
			dbWrites = costData.DynamoDBWrites
			s3Gets = costData.S3Gets
			s3Puts = costData.S3Puts
			lambdaInvocations = costData.LambdaInvocations

			// Calculate individual service costs in dollars
			// DynamoDB costs
			if dbReads > 0 || dbWrites > 0 {
				dynamoDBCost = float64(dbReads*25+dbWrites*125) / 1000000.0
			}
			// Lambda costs
			if lambdaInvocations > 0 {
				lambdaCost = float64(lambdaInvocations*20) / 1000000.0
				gbSeconds := float64(costData.LambdaDurationMs) * float64(costData.LambdaMemoryMB) / (1000 * 1024)
				lambdaCost += gbSeconds * 0.0000166667
			}
			// S3 costs
			if s3Gets > 0 || s3Puts > 0 {
				s3Cost = float64(s3Gets*40/1000+s3Puts*500/1000) / 1000000.0
			}
			// Data transfer costs
			if costData.DataTransferBytes > 0 {
				gb := float64(costData.DataTransferBytes) / (1024 * 1024 * 1024)
				transferCost = gb * 0.09
			}
			totalCost = dynamoDBCost + s3Cost + lambdaCost + transferCost
		}
	}

	// Create detailed breakdown items
	breakdownItems := []*model.CostItem{}
	if dynamoDBCost > 0 {
		breakdownItems = append(breakdownItems, &model.CostItem{
			Operation: "DynamoDB",
			Count:     int(dbReads + dbWrites),
			Cost:      dynamoDBCost,
		})
	}
	if s3Cost > 0 {
		breakdownItems = append(breakdownItems, &model.CostItem{
			Operation: "S3",
			Count:     int(s3Gets + s3Puts),
			Cost:      s3Cost,
		})
	}
	if lambdaCost > 0 {
		breakdownItems = append(breakdownItems, &model.CostItem{
			Operation: "Lambda",
			Count:     int(lambdaInvocations),
			Cost:      lambdaCost,
		})
	}
	if transferCost > 0 {
		breakdownItems = append(breakdownItems, &model.CostItem{
			Operation: "DataTransfer",
			Count:     1, // Aggregate count
			Cost:      transferCost,
		})
	}

	return &model.CostBreakdown{
		Period:           p,
		TotalCost:        totalCost,
		DynamoDBCost:     dynamoDBCost,
		S3StorageCost:    s3Cost,
		LambdaCost:       lambdaCost,
		DataTransferCost: transferCost,
		Breakdown:        breakdownItems,
	}, nil
}

// InfrastructureHealth implements QueryResolver.
func (r *queryResolver) InfrastructureHealth(ctx context.Context) (*model.InfrastructureStatus, error) {
	// Use analytics service to get real infrastructure metrics
	analytics := r.Registry.Analytics()
	if analytics == nil {
		return nil, ErrAnalyticsUnavailable
	}

	// Get health data from analytics service
	healthData, err := analytics.GetInfrastructureHealth(ctx)
	if err != nil {
		r.Logger.Error("Failed to get infrastructure health", zap.Error(err))
		return nil, errors.Join(errors.New("failed to get infrastructure health"), err)
	}

	return healthData, nil
}

// SlowQueries implements QueryResolver.
func (r *queryResolver) SlowQueries(_ context.Context, _ model.Duration) ([]*model.QueryPerformance, error) {
	// Get slow database queries
	return []*model.QueryPerformance{}, nil
}

// PerformanceMetrics returns performance metrics for a service
func (r *queryResolver) PerformanceMetrics(ctx context.Context, service model.ServiceCategory) (*model.PerformanceReport, error) {
	// Get storage from resolver
	storage := r.Storage
	if storage == nil {
		return nil, ErrStorageUnavailable
	}

	// Calculate period for metrics
	period := 24 * time.Hour // Last 24 hours for daily metrics

	// Fetch real performance metrics from CloudWatch metrics repository
	// We use CloudWatchMetrics repository which provides service metrics
	cwMetricsRepo := storage.CloudWatchMetrics()
	if cwMetricsRepo == nil {
		// Fallback to creating empty metrics if repository not available
		return &model.PerformanceReport{
			Service:    service,
			P50Latency: model.Duration(0),
			P95Latency: model.Duration(0),
			P99Latency: model.Duration(0),
			ErrorRate:  0.0,
			Throughput: 0.0,
			ColdStarts: 0,
			Period:     model.TimePeriodDay,
		}, nil
	}

	metrics, err := cwMetricsRepo.GetServiceMetrics(ctx, string(service), period)
	if err != nil {
		r.Logger.Error("failed to get service metrics",
			zap.String("service", string(service)),
			zap.Error(err))
		// Return empty metrics on error rather than failing
		return &model.PerformanceReport{
			Service:    service,
			P50Latency: model.Duration(0),
			P95Latency: model.Duration(0),
			P99Latency: model.Duration(0),
			ErrorRate:  0.0,
			Throughput: 0.0,
			ColdStarts: 0,
			Period:     model.TimePeriodDay,
		}, nil
	}

	// Extract metrics from CloudWatch response
	var requestCount int64
	var errorCount int64
	var coldStarts int64
	latencies := make([]int64, 0)

	// Use the CloudWatch metrics data
	if metrics != nil {
		// Use latency percentiles directly from ServiceMetrics
		if metrics.LatencyP50Ms > 0 {
			latencies = append(latencies, int64(metrics.LatencyP50Ms))
		}
		if metrics.LatencyP90Ms > 0 {
			latencies = append(latencies, int64(metrics.LatencyP90Ms))
		}
		if metrics.LatencyP99Ms > 0 {
			latencies = append(latencies, int64(metrics.LatencyP99Ms))
		}

		// Use request and error counts
		if metrics.RequestCount > 0 {
			requestCount = metrics.RequestCount
		}
		if metrics.ErrorCount > 0 {
			errorCount = metrics.ErrorCount
		}

		// Note: Average latency could be calculated from P50 if needed

		// Note: ColdStarts is not tracked in ServiceMetrics
		// We'd need to get this from Lambda-specific metrics
		coldStarts = 0
	}

	// Calculate average latency (not used in current model)
	// avgLatency := int64(0)
	// if requestCount > 0 {
	// 	avgLatency = totalLatency / requestCount
	// }

	// Calculate throughput (requests per second)
	duration := period.Seconds()
	throughput := float64(requestCount) / duration

	// Calculate error rate
	errorRate := 0.0
	if requestCount > 0 {
		errorRate = float64(errorCount) / float64(requestCount)
	}

	// Get percentile values directly from CloudWatch metrics if available
	var p50, p95, p99 int64
	if metrics != nil {
		p50 = int64(metrics.LatencyP50Ms)
		p95 = int64(metrics.LatencyP90Ms) // Using P90 as approximation for P95
		p99 = int64(metrics.LatencyP99Ms)
	} else if err := common.ValidateSliceNotEmpty("latencies", latencies); err == nil {
		// Fallback to calculating from sample latencies if we have them
		p50, _, p95, p99 = calculatePercentiles(latencies)
	}

	return &model.PerformanceReport{
		Service:    service,
		P50Latency: model.Duration(p50),
		P95Latency: model.Duration(p95),
		P99Latency: model.Duration(p99),
		ErrorRate:  errorRate,
		Throughput: throughput,
		ColdStarts: int(coldStarts),
		Period:     model.TimePeriodDay,
	}, nil
}

// BandwidthUsage implements QueryResolver
func (r *queryResolver) BandwidthUsage(ctx context.Context, period model.TimePeriod) (*model.BandwidthReport, error) {
	_, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Get streaming analytics service
	service := r.Registry.StreamingAnalytics()
	if service == nil {
		r.Logger.Warn("streaming analytics service not available")
		// Return empty report for graceful degradation
		return &model.BandwidthReport{
			Period:    period,
			TotalGb:   0.0,
			PeakMbps:  0.0,
			AvgMbps:   0.0,
			ByQuality: []*model.QualityBandwidth{},
			ByHour:    []*model.HourlyBandwidth{},
			Cost:      0.0,
		}, nil
	}

	// Get bandwidth report from service
	report, err := service.GetBandwidthUsage(ctx, period)
	if err != nil {
		r.Logger.Error("failed to get bandwidth usage",
			zap.String("period", string(period)),
			zap.Error(err))
		return nil, err
	}

	return report, nil
}

// CostProjections implements QueryResolver
func (r *queryResolver) CostProjections(ctx context.Context, period model.Period) (*model.CostProjection, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	costRepo := r.Registry.GetStorage().Cost()
	if costRepo == nil {
		return nil, ErrCostTrackingUnavailable
	}

	// Try to get existing projection first
	if projection := r.getExistingCostProjection(ctx, costRepo, period); projection != nil {
		return projection, nil
	}

	// Calculate new projection
	return r.calculateNewCostProjection(ctx, costRepo, period, username)
}
