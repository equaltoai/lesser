package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// CostBreakdown is the resolver for the costBreakdown field.
func (r *queryResolver) CostBreakdown(ctx context.Context, period *model.Period) (*model.CostBreakdown, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

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
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

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
func (r *queryResolver) SlowQueries(ctx context.Context, threshold model.Duration) ([]*model.QueryPerformance, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Get query tracker from registry
	queryTracker := r.Registry.QueryTracker()
	if queryTracker == nil {
		r.Logger.Warn("query tracker not available")
		return []*model.QueryPerformance{}, nil
	}

	// Get slow queries above the threshold
	slowQueries, err := queryTracker.GetSlowQueries(ctx, threshold)
	if err != nil {
		r.Logger.Error("Failed to get slow queries", zap.Error(err))
		return nil, errors.Join(errors.New("failed to get slow queries"), err)
	}

	return slowQueries, nil
}

// PerformanceMetrics returns performance metrics for a service
func (r *queryResolver) PerformanceMetrics(ctx context.Context, service model.ServiceCategory) (*model.PerformanceReport, error) {
	_, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Get performance service from registry
	perfService := r.Registry.Performance()
	if perfService == nil {
		r.Logger.Warn("performance service not available")
		return nil, errors.New("performance service not available")
	}

	// Default to daily metrics
	period := model.TimePeriodDay

	// Fetch real performance metrics from CloudWatch via performance service
	report, err := perfService.GetPerformanceMetrics(ctx, service, period)
	if err != nil {
		r.Logger.Error("failed to get performance metrics",
			zap.String("service", string(service)),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get performance metrics"), err)
	}

	return report, nil
}

// BandwidthUsage implements QueryResolver
func (r *queryResolver) BandwidthUsage(ctx context.Context, period model.TimePeriod) (*model.BandwidthReport, error) {
	_, err := r.requireAdmin(ctx)
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
	username, err := r.requireAdmin(ctx)
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
