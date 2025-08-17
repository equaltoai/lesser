// Package storagecost provides DynamORM-based storage implementation for cost tracking and analytics.
package storagecost

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// DynamORMStorage handles persistence of cost data using DynamORM
type DynamORMStorage struct {
	repo   *repositories.CostTrackingRepository
	logger *zap.Logger
}

// NewDynamORMStorage creates a new DynamORM-based cost storage instance
func NewDynamORMStorage(db core.DB, tableName string, logger *zap.Logger) *DynamORMStorage {
	return &DynamORMStorage{
		repo:   repositories.NewCostTrackingRepository(db, tableName, logger),
		logger: logger,
	}
}

// SaveOperationCost saves a single operation cost to DynamoDB using DynamORM
func (s *DynamORMStorage) SaveOperationCost(ctx context.Context, opCost *cost.OperationCost) error {
	// Convert OperationCost to DynamoDBCostRecord
	record := &models.DynamoDBCostRecord{
		OperationType: opCost.OperationType,
		Table:         "main", // Default table name
		Timestamp:     opCost.Timestamp,
		Period:        "raw", // Raw data point

		// Map the costs
		ReadCapacityUnits:   float64(opCost.DynamoDBReads),
		WriteCapacityUnits:  float64(opCost.DynamoDBWrites),
		ReadCostMicroCents:  opCost.DynamoDBReads * cost.DynamoDBReadRequestUnit / 1000,
		WriteCostMicroCents: opCost.DynamoDBWrites * cost.DynamoDBWriteRequestUnit / 1000,
		TotalCostMicroCents: opCost.TotalCostMicroCents,

		// Operation details
		ItemCount:       1, // Default to 1 for single operations
		RequestDuration: opCost.LambdaDurationMs,
		RequestID:       opCost.RequestID,

		// Lambda details
		FunctionName: opCost.OperationType,

		// Store additional metrics in properties
		Properties: map[string]interface{}{
			"lambda_invocations":     opCost.LambdaInvocations,
			"lambda_memory_mb":       opCost.LambdaMemoryMB,
			"s3_gets":                opCost.S3Gets,
			"s3_puts":                opCost.S3Puts,
			"s3_storage_bytes":       opCost.S3Storage,
			"data_transfer_bytes":    opCost.DataTransferBytes,
			"dynamodb_storage_bytes": opCost.DynamoDBStorage,
		},
	}

	// Save using repository
	return s.repo.Create(ctx, record)
}

// GetDailyCosts retrieves daily cost aggregates for a date range
func (s *DynamORMStorage) GetDailyCosts(ctx context.Context, startDate, endDate time.Time) ([]cost.DailyCostAggregate, error) {
	// Query aggregated data by day
	aggregates, err := s.repo.GetAggregatedCostsByPeriod(ctx, "day", startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily costs: %w", err)
	}

	// Convert to cost.DailyCostAggregate format
	results := make([]cost.DailyCostAggregate, 0, len(aggregates))
	for _, agg := range aggregates {
		// Extract unique users count from table breakdown
		var uniqueUsers int64
		if agg.TableBreakdown != nil {
			userSet := make(map[string]bool)
			for _, tableStats := range agg.TableBreakdown {
				// Aggregate unique users across all tables
				if tableStats.UniqueUsers > 0 {
					for i := 0; i < int(tableStats.UniqueUsers); i++ {
						userSet[fmt.Sprintf("%s_%d", tableStats.TableName, i)] = true
					}
				}
			}
			uniqueUsers = int64(len(userSet))
		}

		// Extract data transfer bytes from service breakdown
		var dataTransferBytes int64
		if agg.ServiceBreakdown != nil {
			if s3Stats, ok := agg.ServiceBreakdown["s3"]; ok {
				// S3 data transfer is often the main contributor
				dataTransferBytes = s3Stats.DataTransferBytes
			}
			if cfStats, ok := agg.ServiceBreakdown["cloudfront"]; ok {
				dataTransferBytes += cfStats.DataTransferBytes
			}
		}

		results = append(results, cost.DailyCostAggregate{
			Date:                agg.WindowStart.Format(common.DateFormat),
			TotalCostMicrocents: agg.TotalCostMicroCents,
			RequestCount:        agg.TotalOperations,
			UniqueUsers:         uniqueUsers,
			DynamoDBReads:       int64(agg.TotalReadCapacityUnits),
			DynamoDBWrites:      int64(agg.TotalWriteCapacityUnits),
			LambdaInvocations:   agg.TotalOperations,
			LambdaDurationMs:    int64(agg.AverageDuration * float64(agg.TotalOperations)),
			DataTransferBytes:   dataTransferBytes,
		})
	}

	return results, nil
}

// GetMonthlyCost retrieves cost data for a specific month
func (s *DynamORMStorage) GetMonthlyCost(ctx context.Context, year int, month time.Month) (*cost.MonthlyCostAggregate, error) {
	startDate := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, -1) // Last day of the month

	// Query aggregated data for the specific month
	aggregates, err := s.repo.GetAggregatedCostsByPeriod(ctx, "month", startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get monthly cost: %w", err)
	}

	// If no data found, return empty result
	if err := common.ValidateSliceNotEmpty("aggregates", aggregates); err != nil {
		return &cost.MonthlyCostAggregate{
			Year:                    year,
			Month:                   int(month),
			TotalCostMicrocents:     0,
			ProjectedCostMicrocents: 0,
			RequestCount:            0,
		}, nil
	}

	// Convert the first (and should be only) aggregate
	agg := aggregates[0]
	// Extract unique users and data transfer
	var uniqueUsers int64
	var dataTransferBytes int64

	if agg.TableBreakdown != nil {
		userSet := make(map[string]bool)
		for _, tableStats := range agg.TableBreakdown {
			if tableStats.UniqueUsers > 0 {
				for i := 0; i < int(tableStats.UniqueUsers); i++ {
					userSet[fmt.Sprintf("%s_%d", tableStats.TableName, i)] = true
				}
			}
		}
		uniqueUsers = int64(len(userSet))
	}

	if agg.ServiceBreakdown != nil {
		for _, serviceStats := range agg.ServiceBreakdown {
			dataTransferBytes += serviceStats.DataTransferBytes
		}
	}

	// Convert bytes to GB
	dataTransferGB := float64(dataTransferBytes) / (1024 * 1024 * 1024)

	return &cost.MonthlyCostAggregate{
		Year:                    agg.WindowStart.Year(),
		Month:                   int(agg.WindowStart.Month()),
		TotalCostMicrocents:     agg.TotalCostMicroCents,
		ProjectedCostMicrocents: int64(agg.TotalCostDollars * 1000000), // Convert dollars to microcents
		RequestCount:            agg.TotalOperations,
		UniqueUsers:             uniqueUsers,
		DynamoDBReads:           int64(agg.TotalReadCapacityUnits),
		DynamoDBWrites:          int64(agg.TotalWriteCapacityUnits),
		LambdaInvocations:       agg.TotalOperations,
		LambdaDurationMs:        int64(agg.AverageDuration * float64(agg.TotalOperations)),
		DataTransferGB:          dataTransferGB,
	}, nil
}

// GetMonthlyCosts retrieves monthly cost aggregates
func (s *DynamORMStorage) GetMonthlyCosts(ctx context.Context, months int) ([]cost.MonthlyCostAggregate, error) {
	endDate := time.Now()
	startDate := endDate.AddDate(0, -months, 0)

	// Query aggregated data by month
	aggregates, err := s.repo.GetAggregatedCostsByPeriod(ctx, "month", startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get monthly costs: %w", err)
	}

	// Convert to cost.MonthlyCostAggregate format
	results := make([]cost.MonthlyCostAggregate, 0, len(aggregates))
	for _, agg := range aggregates {
		// Extract unique users from table breakdown
		var uniqueUsers int64
		if agg.TableBreakdown != nil {
			userSet := make(map[string]bool)
			for _, tableStats := range agg.TableBreakdown {
				if tableStats.UniqueUsers > 0 {
					for i := 0; i < int(tableStats.UniqueUsers); i++ {
						userSet[fmt.Sprintf("%s_%d", tableStats.TableName, i)] = true
					}
				}
			}
			uniqueUsers = int64(len(userSet))
		}

		// Extract data transfer bytes from service breakdown
		var dataTransferBytes int64
		if agg.ServiceBreakdown != nil {
			for _, serviceStats := range agg.ServiceBreakdown {
				dataTransferBytes += serviceStats.DataTransferBytes
			}
		}

		// Convert bytes to GB
		dataTransferGB := float64(dataTransferBytes) / (1024 * 1024 * 1024)

		results = append(results, cost.MonthlyCostAggregate{
			Year:                    agg.WindowStart.Year(),
			Month:                   int(agg.WindowStart.Month()),
			TotalCostMicrocents:     agg.TotalCostMicroCents,
			ProjectedCostMicrocents: int64(agg.TotalCostDollars * 1000000), // Convert dollars to microcents
			RequestCount:            agg.TotalOperations,
			UniqueUsers:             uniqueUsers,
			DynamoDBReads:           int64(agg.TotalReadCapacityUnits),
			DynamoDBWrites:          int64(agg.TotalWriteCapacityUnits),
			LambdaInvocations:       agg.TotalOperations, // Approximation
			LambdaDurationMs:        int64(agg.AverageDuration * float64(agg.TotalOperations)),
			DataTransferGB:          dataTransferGB,
		})
	}

	return results, nil
}

// GetCostByOperation retrieves costs grouped by operation type
func (s *DynamORMStorage) GetCostByOperation(ctx context.Context, startDate, endDate time.Time) (map[string]int64, error) {
	// Query costs by operation type
	operationCosts, err := s.repo.GetCostsByOperationType(ctx, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get costs by operation: %w", err)
	}

	// Convert to map format
	results := make(map[string]int64)
	for opType, cost := range operationCosts {
		results[opType] = cost.TotalCostMicroCents
	}

	return results, nil
}

// GetCostByService retrieves costs grouped by service/function
func (s *DynamORMStorage) GetCostByService(ctx context.Context, startDate, endDate time.Time) (map[string]int64, error) {
	// Query costs by service
	serviceCosts, err := s.repo.GetCostsByService(ctx, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get costs by service: %w", err)
	}

	// Convert to map format
	results := make(map[string]int64)
	for service, cost := range serviceCosts {
		results[service] = cost.TotalCostMicroCents
	}

	return results, nil
}

// GetCurrentMonthProjection projects costs for the current month
func (s *DynamORMStorage) GetCurrentMonthProjection(ctx context.Context) (*cost.MonthlyCostAggregate, error) {
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// Get costs so far this month
	dailyCosts, err := s.GetDailyCosts(ctx, startOfMonth, now)
	if err != nil {
		return nil, err
	}

	if err := common.ValidateSliceNotEmpty("dailyCosts", dailyCosts); err != nil {
		return &cost.MonthlyCostAggregate{
			Year:                    now.Year(),
			Month:                   int(now.Month()),
			TotalCostMicrocents:     0,
			ProjectedCostMicrocents: 0,
		}, nil
	}

	// Calculate projection
	var totalCost int64
	var totalRequests int64
	var totalDynamoReads int64
	var totalDynamoWrites int64
	var totalLambdaDuration int64
	var totalDataTransferBytes int64

	// Track unique users across all days
	userSet := make(map[int64]bool)

	for _, daily := range dailyCosts {
		totalCost += daily.TotalCostMicrocents
		totalRequests += daily.RequestCount
		totalDynamoReads += daily.DynamoDBReads
		totalDynamoWrites += daily.DynamoDBWrites
		totalLambdaDuration += daily.LambdaDurationMs
		totalDataTransferBytes += daily.DataTransferBytes

		// Collect unique users
		if daily.UniqueUsers > 0 {
			userSet[daily.UniqueUsers] = true
		}
	}

	// Calculate total unique users
	var uniqueUsers int64
	for userCount := range userSet {
		uniqueUsers += userCount
	}

	// Convert bytes to GB
	dataTransferGB := float64(totalDataTransferBytes) / (1024 * 1024 * 1024)

	daysInMonth := float64(daysInMonth(now.Year(), int(now.Month())))
	daysSoFar := float64(now.Day())
	projectionMultiplier := daysInMonth / daysSoFar

	return &cost.MonthlyCostAggregate{
		Year:                    now.Year(),
		Month:                   int(now.Month()),
		TotalCostMicrocents:     totalCost,
		ProjectedCostMicrocents: int64(float64(totalCost) * projectionMultiplier),
		RequestCount:            totalRequests,
		UniqueUsers:             uniqueUsers,
		DynamoDBReads:           totalDynamoReads,
		DynamoDBWrites:          totalDynamoWrites,
		LambdaInvocations:       totalRequests, // Approximation
		LambdaDurationMs:        totalLambdaDuration,
		DataTransferGB:          dataTransferGB,
	}, nil
}

// daysInMonth returns the number of days in a given month
func daysInMonth(year, month int) int {
	return time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
}
