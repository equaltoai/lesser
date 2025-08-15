package repositories

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// Use common time format constants from pkg/common/time_formats.go
// common.CompactTimeFormat is replaced by common.CompactTimeFormat

// CostTrackingRepository handles cost tracking persistence
type CostTrackingRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewCostTrackingRepository creates a new cost tracking repository
func NewCostTrackingRepository(db core.DB, tableName string, logger *zap.Logger) *CostTrackingRepository {
	return &CostTrackingRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// Create creates a new cost tracking record
func (r *CostTrackingRepository) Create(_ context.Context, tracking *models.DynamoDBCostRecord) error {
	// Call BeforeCreate to set up the model
	if err := tracking.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Create the cost tracking record
	err := r.db.Model(tracking).Create()
	if err != nil {
		return MapErrorWithContext(err, "failed to create cost tracking")
	}

	r.logger.Debug("created cost tracking",
		zap.String("id", tracking.ID),
		zap.String("operation_type", tracking.OperationType),
		zap.String("table_name", tracking.Table),
		zap.Float64("cost_dollars", tracking.EstimatedCostDollars))

	return nil
}

// BatchCreate creates multiple cost tracking records efficiently using DynamORM BatchCreate
func (r *CostTrackingRepository) BatchCreate(ctx context.Context, trackingList []*models.DynamoDBCostRecord) error {
	if len(trackingList) == 0 {
		return nil
	}

	// Prepare all records
	for _, ct := range trackingList {
		if err := ct.BeforeCreate(); err != nil {
			return fmt.Errorf("before create validation failed for tracking %s: %w", ct.ID, err)
		}
	}

	// Use DynamORM's efficient BatchCreate - splits into chunks of 25 automatically
	err := r.db.WithContext(ctx).Model(&trackingList[0]).BatchCreate(trackingList)
	if err != nil {
		r.logger.Error("failed to batch create cost tracking records",
			zap.Int("record_count", len(trackingList)),
			zap.Error(err))
		return MapErrorWithContext(err, "failed to batch create cost tracking records")
	}

	r.logger.Debug("batch created cost tracking records",
		zap.Int("record_count", len(trackingList)))

	return nil
}

// Get retrieves a cost tracking record by operation type, timestamp and ID
func (r *CostTrackingRepository) Get(_ context.Context, operationType, id string, timestamp time.Time) (*models.DynamoDBCostRecord, error) {
	tracking := &models.DynamoDBCostRecord{}

	// Construct the keys
	pk := fmt.Sprintf("cost#%s", operationType)
	sk := fmt.Sprintf("ts#%s#%s", timestamp.Format(common.CompactTimeFormat), id)

	err := r.db.Model(tracking).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(tracking)

	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get cost tracking")
	}

	return tracking, nil
}

// ListByOperationType lists cost tracking records by operation type within a time range
func (r *CostTrackingRepository) ListByOperationType(_ context.Context, operationType string, startTime, endTime time.Time, limit int) ([]*models.DynamoDBCostRecord, error) {
	var trackingList []*models.DynamoDBCostRecord

	// Construct SK range for time-based query
	pk := fmt.Sprintf("cost#%s", operationType)
	startSK := fmt.Sprintf("ts#%s", startTime.Format(common.CompactTimeFormat))
	endSK := fmt.Sprintf("ts#%s", endTime.Format(common.CompactTimeFormat))

	query := r.db.Model(&models.DynamoDBCostRecord{}).
		Where("PK", "=", pk).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", "DESC").
		Limit(limit)

	err := query.All(&trackingList)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to list cost tracking by operation type")
	}

	return trackingList, nil
}

// ListByTable lists cost tracking records by table within a time range
func (r *CostTrackingRepository) ListByTable(_ context.Context, tableName string, startTime, endTime time.Time, limit int) ([]*models.DynamoDBCostRecord, error) {
	var trackingList []*models.DynamoDBCostRecord

	// Use GSI1 for table-based queries
	startSK := startTime.Format(time.RFC3339)
	endSK := endTime.Format(time.RFC3339)

	query := r.db.Model(&models.DynamoDBCostRecord{}).
		Index("table-index").
		Where("GSI1PK", "=", fmt.Sprintf("COST_TABLE#%s", tableName)).
		Where("GSI1SK", ">=", startSK).
		Where("GSI1SK", "<=", endSK).
		OrderBy("GSI1SK", "DESC").
		Limit(limit)

	err := query.All(&trackingList)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to list cost tracking by table")
	}

	return trackingList, nil
}

// GetRecentCosts retrieves recent cost tracking records across all operations
func (r *CostTrackingRepository) GetRecentCosts(ctx context.Context, since time.Time, limit int) ([]*models.DynamoDBCostRecord, error) {
	var allCosts []*models.DynamoDBCostRecord

	// Query each operation type (we need to iterate through known types)
	operationTypes := []string{
		"GetItem", "PutItem", "UpdateItem", "DeleteItem",
		"Query", "Scan", "BatchGetItem", "BatchWriteItem",
		"TransactGetItems", "TransactWriteItems",
	}

	for _, opType := range operationTypes {
		costs, err := r.ListByOperationType(ctx, opType, since, time.Now(), limit/len(operationTypes))
		if err != nil {
			r.logger.Warn("failed to get costs for operation type",
				zap.String("operation_type", opType),
				zap.Error(err))
			continue
		}
		allCosts = append(allCosts, costs...)
	}

	// Sort by timestamp (newest first)
	// Note: In production, you might want to use a more efficient sorting approach

	return allCosts, nil
}

// GetAggregated retrieves aggregated cost tracking
func (r *CostTrackingRepository) GetAggregated(_ context.Context, period, operationType string, windowStart time.Time) (*models.DynamoDBCostAggregation, error) {
	aggregated := &models.DynamoDBCostAggregation{}

	pk := fmt.Sprintf("cost_agg#%s#%s", period, operationType)
	sk := fmt.Sprintf("window#%s", windowStart.Format(time.RFC3339))

	err := r.db.Model(aggregated).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(aggregated)

	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get aggregated cost tracking")
	}

	return aggregated, nil
}

// CreateAggregated creates an aggregated cost tracking record
func (r *CostTrackingRepository) CreateAggregated(_ context.Context, aggregated *models.DynamoDBCostAggregation) error {
	// Call BeforeCreate to set up the model
	if err := aggregated.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Create the aggregated cost tracking
	err := r.db.Model(aggregated).Create()
	if err != nil {
		return MapErrorWithContext(err, "failed to create aggregated cost tracking")
	}

	r.logger.Debug("created aggregated cost tracking",
		zap.String("operation_type", aggregated.OperationType),
		zap.String("period", aggregated.Period),
		zap.Time("window_start", aggregated.WindowStart),
		zap.Float64("total_cost_dollars", aggregated.TotalCostDollars))

	return nil
}

// UpdateAggregated updates an existing aggregated cost tracking record
func (r *CostTrackingRepository) UpdateAggregated(_ context.Context, aggregated *models.DynamoDBCostAggregation) error {
	// Call BeforeUpdate to set up the model
	if err := aggregated.BeforeUpdate(); err != nil {
		return fmt.Errorf("before update validation failed: %w", err)
	}

	// Update the aggregated cost tracking
	err := r.db.Model(aggregated).Update()
	if err != nil {
		return MapErrorWithContext(err, "failed to update aggregated cost tracking")
	}

	return nil
}

// ListAggregatedByPeriod lists aggregated cost tracking for a period
func (r *CostTrackingRepository) ListAggregatedByPeriod(_ context.Context, period, operationType string, startTime, endTime time.Time, limit int) ([]*models.DynamoDBCostAggregation, error) {
	var aggregatedList []*models.DynamoDBCostAggregation

	pk := fmt.Sprintf("cost_agg#%s#%s", period, operationType)
	startSK := fmt.Sprintf("window#%s", startTime.Format(time.RFC3339))
	endSK := fmt.Sprintf("window#%s", endTime.Format(time.RFC3339))

	query := r.db.Model(&models.DynamoDBCostAggregation{}).
		Where("PK", "=", pk).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", "DESC").
		Limit(limit)

	err := query.All(&aggregatedList)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to list aggregated cost tracking")
	}

	return aggregatedList, nil
}

// GetTableCostStats calculates cost statistics for a table
func (r *CostTrackingRepository) GetTableCostStats(ctx context.Context, tableName string, startTime, endTime time.Time) (*TableCostStats, error) {
	costs, err := r.ListByTable(ctx, tableName, startTime, endTime, 10000)
	if err != nil {
		return nil, err
	}

	stats := &TableCostStats{
		TableName:          tableName,
		StartTime:          startTime,
		EndTime:            endTime,
		Count:              len(costs),
		OperationBreakdown: make(map[string]OperationCostStats),
	}

	if stats.Count == 0 {
		return stats, nil
	}

	// Calculate statistics
	for _, ct := range costs {
		stats.TotalReadCapacityUnits += ct.ReadCapacityUnits
		stats.TotalWriteCapacityUnits += ct.WriteCapacityUnits
		stats.TotalCostMicroCents += ct.TotalCostMicroCents
		stats.TotalOperations++
		stats.TotalItemCount += int64(ct.ItemCount)

		// Track by operation type
		opStats, exists := stats.OperationBreakdown[ct.OperationType]
		if !exists {
			opStats = OperationCostStats{
				OperationType: ct.OperationType,
			}
		}

		opStats.Count++
		opStats.TotalCostMicroCents += ct.TotalCostMicroCents
		opStats.TotalReadCapacityUnits += ct.ReadCapacityUnits
		opStats.TotalWriteCapacityUnits += ct.WriteCapacityUnits

		stats.OperationBreakdown[ct.OperationType] = opStats
	}

	// Calculate totals and averages
	stats.TotalCostDollars = float64(stats.TotalCostMicroCents) / 1_000_000.0
	if stats.TotalOperations > 0 {
		stats.AverageCostPerOperation = stats.TotalCostDollars / float64(stats.TotalOperations)
	}

	// Calculate operation averages
	for opType, opStats := range stats.OperationBreakdown {
		if opStats.Count > 0 {
			opStats.AverageCostMicroCents = opStats.TotalCostMicroCents / opStats.Count
			opStats.TotalCostDollars = float64(opStats.TotalCostMicroCents) / 1_000_000.0
			stats.OperationBreakdown[opType] = opStats
		}
	}

	return stats, nil
}

// Aggregate performs aggregation of raw cost tracking data
func (r *CostTrackingRepository) Aggregate(ctx context.Context, operationType, period string, windowStart, windowEnd time.Time) error {
	// Get all cost tracking records in the window
	costs, err := r.ListByOperationType(ctx, operationType, windowStart, windowEnd, 10000)
	if err != nil {
		return fmt.Errorf("failed to list costs for aggregation: %w", err)
	}

	if len(costs) == 0 {
		return nil // Nothing to aggregate
	}

	// Calculate aggregated values
	aggregated := &models.DynamoDBCostAggregation{
		Period:           period,
		OperationType:    operationType,
		Table:            "all", // Default to all tables
		WindowStart:      windowStart,
		WindowEnd:        windowEnd,
		CostPercentiles:  make(map[string]float64),
		TableBreakdown:   make(map[string]*models.DynamoDBTableCostStats),
		ServiceBreakdown: make(map[string]*models.DynamoDBServiceCostStats),
	}

	// Collect values for percentile calculation
	costValues := make([]float64, 0, len(costs))

	for _, ct := range costs {
		aggregated.TotalOperations++
		aggregated.TotalReadCapacityUnits += ct.ReadCapacityUnits
		aggregated.TotalWriteCapacityUnits += ct.WriteCapacityUnits
		aggregated.TotalReadCostMicroCents += ct.ReadCostMicroCents
		aggregated.TotalWriteCostMicroCents += ct.WriteCostMicroCents
		aggregated.TotalCostMicroCents += ct.TotalCostMicroCents
		aggregated.TotalItemCount += int64(ct.ItemCount)
		aggregated.AverageDuration += float64(ct.RequestDuration)

		// Collect cost values for percentiles
		costValues = append(costValues, ct.EstimatedCostDollars)

		// Aggregate by table
		tableStats, exists := aggregated.TableBreakdown[ct.Table]
		if !exists {
			tableStats = &models.DynamoDBTableCostStats{
				TableName: ct.Table,
			}
			aggregated.TableBreakdown[ct.Table] = tableStats
		}

		tableStats.OperationCount++
		tableStats.ReadCapacityUnits += ct.ReadCapacityUnits
		tableStats.WriteCapacityUnits += ct.WriteCapacityUnits
		tableStats.TotalCostMicroCents += ct.TotalCostMicroCents

		// Aggregate by service
		if ct.ServiceName != "" {
			serviceStats, exists := aggregated.ServiceBreakdown[ct.ServiceName]
			if !exists {
				serviceStats = &models.DynamoDBServiceCostStats{
					ServiceName: ct.ServiceName,
				}
				aggregated.ServiceBreakdown[ct.ServiceName] = serviceStats
			}

			serviceStats.OperationCount++
			serviceStats.TotalCostMicroCents += ct.TotalCostMicroCents
		}
	}

	// Calculate averages
	if aggregated.TotalOperations > 0 {
		aggregated.AverageDuration = aggregated.AverageDuration / float64(aggregated.TotalOperations)
	}

	// Calculate table totals
	for _, tableStats := range aggregated.TableBreakdown {
		tableStats.TotalCostDollars = float64(tableStats.TotalCostMicroCents) / 1_000_000.0
	}

	// Calculate service averages
	for _, serviceStats := range aggregated.ServiceBreakdown {
		serviceStats.TotalCostDollars = float64(serviceStats.TotalCostMicroCents) / 1_000_000.0
		if serviceStats.OperationCount > 0 {
			serviceStats.AverageCostPerOp = serviceStats.TotalCostDollars / float64(serviceStats.OperationCount)
		}
	}

	// Calculate percentiles from costValues
	if len(costValues) > 0 {
		aggregated.CostPercentiles = calculatePercentiles(costValues)
	}

	// Check if aggregation already exists
	existing, err := r.GetAggregated(ctx, period, operationType, windowStart)
	if err == nil && existing != nil {
		// Update existing
		aggregated.CreatedAt = existing.CreatedAt
		return r.UpdateAggregated(ctx, aggregated)
	}

	// Create new aggregation
	return r.CreateAggregated(ctx, aggregated)
}

// TableCostStats represents cost statistics for a table
type TableCostStats struct {
	TableName               string
	StartTime               time.Time
	EndTime                 time.Time
	Count                   int
	TotalOperations         int64
	TotalItemCount          int64
	TotalReadCapacityUnits  float64
	TotalWriteCapacityUnits float64
	TotalCostMicroCents     int64
	TotalCostDollars        float64
	AverageCostPerOperation float64
	OperationBreakdown      map[string]OperationCostStats
}

// OperationCostStats represents cost statistics for a specific operation type
type OperationCostStats struct {
	OperationType           string
	Count                   int64
	TotalCostMicroCents     int64
	TotalCostDollars        float64
	AverageCostMicroCents   int64
	TotalReadCapacityUnits  float64
	TotalWriteCapacityUnits float64
}

// GetHighCostOperations returns operations that exceed a cost threshold
func (r *CostTrackingRepository) GetHighCostOperations(ctx context.Context, thresholdDollars float64, startTime, endTime time.Time, limit int) ([]*models.DynamoDBCostRecord, error) {
	// First get all recent costs
	allCosts, err := r.GetRecentCosts(ctx, startTime, limit*10) // Get more to filter
	if err != nil {
		return nil, err
	}

	// Filter by threshold and time range
	var highCostOps []*models.DynamoDBCostRecord
	for _, ct := range allCosts {
		if ct.Timestamp.After(endTime) {
			continue
		}
		if ct.EstimatedCostDollars >= thresholdDollars {
			highCostOps = append(highCostOps, ct)
			if len(highCostOps) >= limit {
				break
			}
		}
	}

	return highCostOps, nil
}

// GetCostTrends calculates cost trends over time
func (r *CostTrackingRepository) GetCostTrends(ctx context.Context, period string, operationType string, lookbackDays int) (*CostTrend, error) {
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -lookbackDays)

	// Get aggregated data for the period
	aggregatedList, err := r.ListAggregatedByPeriod(ctx, period, operationType, startTime, endTime, 1000)
	if err != nil {
		return nil, err
	}

	if len(aggregatedList) == 0 {
		return &CostTrend{
			Period:        period,
			OperationType: operationType,
			StartTime:     startTime,
			EndTime:       endTime,
		}, nil
	}

	trend := &CostTrend{
		Period:        period,
		OperationType: operationType,
		StartTime:     startTime,
		EndTime:       endTime,
		DataPoints:    make([]CostDataPoint, 0, len(aggregatedList)),
	}

	// Calculate trend statistics
	var totalCost float64
	var minCost, maxCost = aggregatedList[0].TotalCostDollars, aggregatedList[0].TotalCostDollars

	for _, agg := range aggregatedList {
		dataPoint := CostDataPoint{
			Timestamp:     agg.WindowStart,
			CostDollars:   agg.TotalCostDollars,
			Operations:    agg.TotalOperations,
			ReadCapacity:  agg.TotalReadCapacityUnits,
			WriteCapacity: agg.TotalWriteCapacityUnits,
		}
		trend.DataPoints = append(trend.DataPoints, dataPoint)

		totalCost += agg.TotalCostDollars
		if agg.TotalCostDollars < minCost {
			minCost = agg.TotalCostDollars
		}
		if agg.TotalCostDollars > maxCost {
			maxCost = agg.TotalCostDollars
		}
	}

	// Calculate statistics
	trend.TotalCost = totalCost
	trend.AverageCost = totalCost / float64(len(aggregatedList))
	trend.MinCost = minCost
	trend.MaxCost = maxCost

	// Calculate trend direction (simple linear regression would be better)
	if len(trend.DataPoints) >= 2 {
		firstCost := trend.DataPoints[0].CostDollars
		lastCost := trend.DataPoints[len(trend.DataPoints)-1].CostDollars
		trend.TrendPercentage = ((lastCost - firstCost) / firstCost) * 100
	}

	return trend, nil
}

// CostTrend represents cost trend analysis
type CostTrend struct {
	Period          string
	OperationType   string
	StartTime       time.Time
	EndTime         time.Time
	DataPoints      []CostDataPoint
	TotalCost       float64
	AverageCost     float64
	MinCost         float64
	MaxCost         float64
	TrendPercentage float64 // Positive = increasing, Negative = decreasing
}

// CostDataPoint represents a single point in the cost trend
type CostDataPoint struct {
	Timestamp     time.Time
	CostDollars   float64
	Operations    int64
	ReadCapacity  float64
	WriteCapacity float64
}

// calculatePercentiles calculates percentiles for a slice of values
// Returns a map with p50, p90, p95, and p99 percentiles
func calculatePercentiles(values []float64) map[string]float64 {
	if len(values) == 0 {
		return map[string]float64{
			"p50": 0,
			"p90": 0,
			"p95": 0,
			"p99": 0,
		}
	}

	// Sort values
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	// Calculate percentiles
	percentiles := map[string]float64{
		"p50": getPercentileValue(sorted, 50),
		"p90": getPercentileValue(sorted, 90),
		"p95": getPercentileValue(sorted, 95),
		"p99": getPercentileValue(sorted, 99),
	}

	return percentiles
}

// getPercentileValue calculates the value at a specific percentile
func getPercentileValue(sorted []float64, percentile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	if len(sorted) == 1 {
		return sorted[0]
	}

	// Calculate the index
	index := (percentile / 100.0) * float64(len(sorted)-1)
	lowerIndex := int(math.Floor(index))
	upperIndex := int(math.Ceil(index))

	if lowerIndex == upperIndex {
		return sorted[lowerIndex]
	}

	// Linear interpolation between two values
	lowerValue := sorted[lowerIndex]
	upperValue := sorted[upperIndex]
	fraction := index - float64(lowerIndex)

	return lowerValue + (upperValue-lowerValue)*fraction
}

// GetAggregatedCostsByPeriod retrieves aggregated costs for a specific period
func (r *CostTrackingRepository) GetAggregatedCostsByPeriod(ctx context.Context, period string, startDate, endDate time.Time) ([]*models.DynamoDBCostAggregation, error) {
	// Query all operation types for the period
	operationTypes := []string{"GetItem", "PutItem", "UpdateItem", "DeleteItem", "Query", "Scan",
		"BatchGetItem", "BatchWriteItem", "TransactGetItems", "TransactWriteItems"}

	var allAggregates []*models.DynamoDBCostAggregation

	for _, opType := range operationTypes {
		aggregates, err := r.ListAggregatedByPeriod(ctx, period, opType, startDate, endDate, 1000)
		if err != nil {
			r.logger.Warn("failed to get aggregates for operation type",
				zap.String("operation_type", opType),
				zap.Error(err))
			continue
		}
		allAggregates = append(allAggregates, aggregates...)
	}

	// Merge aggregates by window
	mergedByWindow := make(map[string]*models.DynamoDBCostAggregation)

	for _, agg := range allAggregates {
		windowKey := agg.WindowStart.Format(time.RFC3339)

		if existing, exists := mergedByWindow[windowKey]; exists {
			// Merge the aggregates
			existing.TotalOperations += agg.TotalOperations
			existing.TotalReadCapacityUnits += agg.TotalReadCapacityUnits
			existing.TotalWriteCapacityUnits += agg.TotalWriteCapacityUnits
			existing.TotalReadCostMicroCents += agg.TotalReadCostMicroCents
			existing.TotalWriteCostMicroCents += agg.TotalWriteCostMicroCents
			existing.TotalCostMicroCents += agg.TotalCostMicroCents
			existing.TotalItemCount += agg.TotalItemCount

			// Merge table breakdown
			for table, stats := range agg.TableBreakdown {
				if existingStats, exists := existing.TableBreakdown[table]; exists {
					existingStats.OperationCount += stats.OperationCount
					existingStats.ReadCapacityUnits += stats.ReadCapacityUnits
					existingStats.WriteCapacityUnits += stats.WriteCapacityUnits
					existingStats.TotalCostMicroCents += stats.TotalCostMicroCents
					existingStats.TotalCostDollars += stats.TotalCostDollars
				} else {
					existing.TableBreakdown[table] = stats
				}
			}

			// Merge service breakdown
			for service, stats := range agg.ServiceBreakdown {
				if existingStats, exists := existing.ServiceBreakdown[service]; exists {
					existingStats.OperationCount += stats.OperationCount
					existingStats.TotalCostMicroCents += stats.TotalCostMicroCents
					existingStats.TotalCostDollars += stats.TotalCostDollars
				} else {
					existing.ServiceBreakdown[service] = stats
				}
			}
		} else {
			mergedByWindow[windowKey] = agg
		}
	}

	// Convert map back to slice
	result := make([]*models.DynamoDBCostAggregation, 0, len(mergedByWindow))
	for _, agg := range mergedByWindow {
		// Recalculate averages and totals
		agg.TotalCostDollars = float64(agg.TotalCostMicroCents) / 1_000_000.0
		if agg.TotalOperations > 0 {
			agg.AverageCostPerOperation = agg.TotalCostDollars / float64(agg.TotalOperations)
		}
		result = append(result, agg)
	}

	// Sort by window start time
	sort.Slice(result, func(i, j int) bool {
		return result[i].WindowStart.Before(result[j].WindowStart)
	})

	return result, nil
}

// GetCostsByOperationType retrieves costs grouped by operation type
func (r *CostTrackingRepository) GetCostsByOperationType(ctx context.Context, startDate, endDate time.Time) (map[string]*models.DynamoDBServiceCostStats, error) {
	operationTypes := []string{"GetItem", "PutItem", "UpdateItem", "DeleteItem", "Query", "Scan",
		"BatchGetItem", "BatchWriteItem", "TransactGetItems", "TransactWriteItems"}

	result := make(map[string]*models.DynamoDBServiceCostStats)

	for _, opType := range operationTypes {
		costs, err := r.ListByOperationType(ctx, opType, startDate, endDate, 10000)
		if err != nil {
			r.logger.Warn("failed to get costs for operation type",
				zap.String("operation_type", opType),
				zap.Error(err))
			continue
		}

		if len(costs) == 0 {
			continue
		}

		stats := &models.DynamoDBServiceCostStats{
			ServiceName: opType,
		}

		for _, cost := range costs {
			stats.OperationCount++
			stats.TotalCostMicroCents += cost.TotalCostMicroCents
		}

		stats.TotalCostDollars = float64(stats.TotalCostMicroCents) / 1_000_000.0
		if stats.OperationCount > 0 {
			stats.AverageCostPerOp = stats.TotalCostDollars / float64(stats.OperationCount)
		}

		result[opType] = stats
	}

	return result, nil
}

// GetCostsByService retrieves costs grouped by service/function
func (r *CostTrackingRepository) GetCostsByService(ctx context.Context, startDate, endDate time.Time) (map[string]*models.DynamoDBServiceCostStats, error) {
	// Get all costs in the time range
	costs, err := r.GetRecentCosts(ctx, startDate, 10000)
	if err != nil {
		return nil, err
	}

	// Group by service
	result := make(map[string]*models.DynamoDBServiceCostStats)

	for _, cost := range costs {
		if cost.Timestamp.After(endDate) {
			continue
		}

		serviceName := cost.ServiceName
		if serviceName == "" {
			serviceName = StatusUnknown
		}

		stats, exists := result[serviceName]
		if !exists {
			stats = &models.DynamoDBServiceCostStats{
				ServiceName: serviceName,
			}
			result[serviceName] = stats
		}

		stats.OperationCount++
		stats.TotalCostMicroCents += cost.TotalCostMicroCents
	}

	// Calculate totals and averages
	for _, stats := range result {
		stats.TotalCostDollars = float64(stats.TotalCostMicroCents) / 1_000_000.0
		if stats.OperationCount > 0 {
			stats.AverageCostPerOp = stats.TotalCostDollars / float64(stats.OperationCount)
		}
	}

	return result, nil
}

// GetCostsByDateRange returns individual cost records for the specified date range
func (r *CostTrackingRepository) GetCostsByDateRange(ctx context.Context, startDate, _ time.Time) ([]*models.DynamoDBCostRecord, error) {
	return r.GetRecentCosts(ctx, startDate, 1000) // Use existing method with reasonable limit
}

// DailyAggregate represents aggregated costs for a single day
type DailyAggregate struct {
	Date             time.Time
	TotalRequests    int64
	UniqueUsers      int64
	TotalReads       int64
	TotalWrites      int64
	TotalDurationMs  int64
	TotalCostDollars float64
}

// GetDailyAggregates returns aggregated daily costs for the specified date range
func (r *CostTrackingRepository) GetDailyAggregates(ctx context.Context, startDate, endDate time.Time) ([]*DailyAggregate, error) {
	// Get aggregated data by day
	aggregations, err := r.GetAggregatedCostsByPeriod(ctx, "day", startDate, endDate)
	if err != nil {
		return nil, err
	}

	// Convert to DailyAggregate format
	dailyAggregates := make([]*DailyAggregate, 0, len(aggregations))
	for _, agg := range aggregations {
		daily := &DailyAggregate{
			Date:             agg.WindowStart,
			TotalRequests:    agg.TotalOperations,
			TotalReads:       int64(agg.TotalReadCapacityUnits),
			TotalWrites:      int64(agg.TotalWriteCapacityUnits),
			TotalDurationMs:  int64(agg.AverageDuration * float64(agg.TotalOperations)),
			TotalCostDollars: agg.TotalCostDollars,
		}
		dailyAggregates = append(dailyAggregates, daily)
	}

	return dailyAggregates, nil
}

// MonthlyAggregate represents aggregated costs for a single month
type MonthlyAggregate struct {
	Year             int
	Month            int
	TotalCostDollars float64
	TotalRequests    int64
	TotalReads       int64
	TotalWrites      int64
}

// GetMonthlyAggregate returns aggregated costs for the specified month
func (r *CostTrackingRepository) GetMonthlyAggregate(ctx context.Context, year, month int) (*MonthlyAggregate, error) {
	// Calculate month boundaries
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, 0).Add(-time.Second)

	// Get aggregated data for the month
	aggregations, err := r.GetAggregatedCostsByPeriod(ctx, "month", startDate, endDate)
	if err != nil {
		return nil, err
	}

	if len(aggregations) == 0 {
		return nil, nil
	}

	// Use the first (and should be only) aggregation for the month
	agg := aggregations[0]
	return &MonthlyAggregate{
		Year:             year,
		Month:            month,
		TotalCostDollars: agg.TotalCostDollars,
		TotalRequests:    agg.TotalOperations,
		TotalReads:       int64(agg.TotalReadCapacityUnits),
		TotalWrites:      int64(agg.TotalWriteCapacityUnits),
	}, nil
}

// GetCostProjections retrieves the most recent cost projection for the given period
func (r *CostTrackingRepository) GetCostProjections(ctx context.Context, period string) (*storage.CostProjection, error) {
	r.logger.Debug("getting cost projections",
		zap.String("period", period))

	var projections []*models.CostProjection

	// Query by PK and SK prefix to get projections for the specified period
	pk, skPrefix := models.GetLatestProjectionKeys(period)

	// Create SK range - from prefix to prefix + next character (e.g., "DAILY#" to "DAILY#~")
	skEnd := skPrefix + "~" // Using tilde as it's lexicographically after all common characters

	err := r.db.WithContext(ctx).Model(&models.CostProjection{}).
		Where("PK", "=", pk).
		Where("SK", ">=", skPrefix).
		Where("SK", "<", skEnd).
		OrderBy("SK", "DESC").
		Limit(1).
		All(&projections)

	if err != nil {
		r.logger.Error("failed to query cost projections",
			zap.String("period", period),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query cost projections: %w", err)
	}

	// If no projection exists, return a default projection with zero values
	if len(projections) == 0 {
		r.logger.Debug("no cost projections found, returning default",
			zap.String("period", period))

		return &storage.CostProjection{
			Period:          period,
			CurrentCost:     0.0,
			ProjectedCost:   0.0,
			Variance:        0.0,
			TopDrivers:      []storage.CostDriver{},
			Recommendations: []string{},
		}, nil
	}

	// Convert models.CostProjection to storage.CostProjection
	projection := projections[0]

	// Convert models.CostDriver to storage.CostDriver
	topDrivers := make([]storage.CostDriver, 0, len(projection.TopDrivers))
	for _, driver := range projection.TopDrivers {
		topDrivers = append(topDrivers, storage.CostDriver{
			Name:           driver.Type, // Use Type as Name
			Impact:         driver.Cost, // Use Cost as Impact
			Description:    fmt.Sprintf("%.1f%% of total cost", driver.PercentOfTotal),
			Optimization:   "", // Not available in model
			Type:           driver.Type,
			Domain:         driver.Domain,
			Cost:           driver.Cost,
			PercentOfTotal: driver.PercentOfTotal,
			Trend:          driver.Trend,
		})
	}

	result := &storage.CostProjection{
		Period:          projection.Period,
		CurrentCost:     projection.CurrentCost,
		ProjectedCost:   projection.ProjectedCost,
		Variance:        projection.Variance,
		TopDrivers:      topDrivers,
		Recommendations: projection.Recommendations,
	}

	r.logger.Debug("retrieved cost projection",
		zap.String("period", period),
		zap.Float64("current_cost", result.CurrentCost),
		zap.Float64("projected_cost", result.ProjectedCost),
		zap.Float64("variance", result.Variance),
		zap.Int("driver_count", len(result.TopDrivers)),
		zap.Int("recommendation_count", len(result.Recommendations)))

	return result, nil
}

// Relay Cost Tracking Methods

// CreateRelayCost creates a new relay cost record
func (r *CostTrackingRepository) CreateRelayCost(ctx context.Context, relayCost *models.RelayCost) error {
	// Call BeforeCreate to set up the model
	if err := relayCost.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Create the relay cost record
	err := r.db.WithContext(ctx).Model(relayCost).Create()
	if err != nil {
		return MapErrorWithContext(err, "failed to create relay cost")
	}

	r.logger.Debug("created relay cost record",
		zap.String("relay_url", relayCost.RelayURL),
		zap.String("operation_type", relayCost.OperationType),
		zap.String("direction", relayCost.Direction),
		zap.Int64("total_cost_micro_cents", relayCost.TotalCostMicroCents))

	return nil
}

// GetRelayCostsByURL retrieves relay costs for a specific relay URL within a time range
func (r *CostTrackingRepository) GetRelayCostsByURL(ctx context.Context, relayURL string, startTime, endTime time.Time, limit int) ([]*models.RelayCost, error) {
	var costs []*models.RelayCost

	startSK := fmt.Sprintf("TS#%s", startTime.Format(common.CompactTimeFormat))
	endSK := fmt.Sprintf("TS#%s", endTime.Format(common.CompactTimeFormat))

	query := r.db.WithContext(ctx).Model(&models.RelayCost{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("RELAY_COSTS#%s", relayURL)).
		Where("GSI1SK", ">=", startSK).
		Where("GSI1SK", "<=", endSK).
		OrderBy("GSI1SK", "DESC").
		Limit(limit)

	err := query.All(&costs)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get relay costs by URL")
	}

	return costs, nil
}

// GetRelayCostsByDateRange retrieves relay costs for all relays within a date range
func (r *CostTrackingRepository) GetRelayCostsByDateRange(ctx context.Context, startDate, endDate time.Time, limit int) ([]*models.RelayCost, error) {
	var allCosts []*models.RelayCost

	// Query by daily partitions
	currentDate := startDate
	for currentDate.Before(endDate) || currentDate.Equal(endDate) {
		dateStr := currentDate.Format("20060102")

		var dailyCosts []*models.RelayCost
		query := r.db.WithContext(ctx).Model(&models.RelayCost{}).
			Index("GSI2").
			Where("GSI2PK", "=", fmt.Sprintf("RELAY_COSTS_DAILY#%s", dateStr)).
			OrderBy("GSI2SK", "DESC").
			Limit(limit)

		err := query.All(&dailyCosts)
		if err != nil {
			r.logger.Warn("failed to get relay costs for date",
				zap.String("date", dateStr),
				zap.Error(err))
			// Continue with next date
		} else {
			allCosts = append(allCosts, dailyCosts...)
		}

		// Move to next day
		currentDate = currentDate.AddDate(0, 0, 1)

		// Break if we have enough results
		if len(allCosts) >= limit {
			break
		}
	}

	// Sort by timestamp (newest first) and limit
	sort.Slice(allCosts, func(i, j int) bool {
		return allCosts[i].Timestamp.After(allCosts[j].Timestamp)
	})

	if len(allCosts) > limit {
		allCosts = allCosts[:limit]
	}

	return allCosts, nil
}

// CreateRelayMetrics creates or updates relay metrics
func (r *CostTrackingRepository) CreateRelayMetrics(ctx context.Context, metrics *models.RelayMetrics) error {
	// Call BeforeCreate to set up the model
	if err := metrics.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Create the relay metrics record
	err := r.db.WithContext(ctx).Model(metrics).Create()
	if err != nil {
		return MapErrorWithContext(err, "failed to create relay metrics")
	}

	r.logger.Debug("created relay metrics",
		zap.String("relay_url", metrics.RelayURL),
		zap.String("period", metrics.Period),
		zap.Time("window_start", metrics.WindowStart),
		zap.Int64("total_operations", metrics.TotalOperations),
		zap.Int64("total_cost_micro_cents", metrics.TotalCostMicroCents))

	return nil
}

// UpdateRelayMetrics updates existing relay metrics
func (r *CostTrackingRepository) UpdateRelayMetrics(ctx context.Context, metrics *models.RelayMetrics) error {
	// Call BeforeUpdate to set up the model
	if err := metrics.BeforeUpdate(); err != nil {
		return fmt.Errorf("before update validation failed: %w", err)
	}

	// Update the relay metrics record
	err := r.db.WithContext(ctx).Model(metrics).Update()
	if err != nil {
		return MapErrorWithContext(err, "failed to update relay metrics")
	}

	return nil
}

// GetRelayMetrics retrieves relay metrics for a specific relay and period
func (r *CostTrackingRepository) GetRelayMetrics(ctx context.Context, relayURL, period string, windowStart time.Time) (*models.RelayMetrics, error) {
	var metrics models.RelayMetrics

	pk := fmt.Sprintf("RELAY_METRICS#%s#%s", relayURL, period)
	sk := fmt.Sprintf("WINDOW#%s", windowStart.Format(common.CompactTimeFormat))

	err := r.db.WithContext(ctx).Model(&models.RelayMetrics{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&metrics)

	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get relay metrics")
	}

	return &metrics, nil
}

// GetRelayMetricsHistory retrieves metrics history for a relay
func (r *CostTrackingRepository) GetRelayMetricsHistory(ctx context.Context, relayURL string, startTime, endTime time.Time, limit int) ([]*models.RelayMetrics, error) {
	var metricsHistory []*models.RelayMetrics

	startSK := fmt.Sprintf("daily#%s", startTime.Format(common.CompactTimeFormat))
	endSK := fmt.Sprintf("daily#%s", endTime.Format(common.CompactTimeFormat))

	query := r.db.WithContext(ctx).Model(&models.RelayMetrics{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("RELAY_METRICS#%s", relayURL)).
		Where("GSI1SK", ">=", startSK).
		Where("GSI1SK", "<=", endSK).
		OrderBy("GSI1SK", "DESC").
		Limit(limit)

	err := query.All(&metricsHistory)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get relay metrics history")
	}

	return metricsHistory, nil
}

// CreateRelayBudget creates a new relay budget configuration
func (r *CostTrackingRepository) CreateRelayBudget(ctx context.Context, budget *models.RelayBudget) error {
	// Call BeforeCreate to set up the model
	if err := budget.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Create the relay budget record
	err := r.db.WithContext(ctx).Model(budget).Create()
	if err != nil {
		return MapErrorWithContext(err, "failed to create relay budget")
	}

	r.logger.Info("created relay budget",
		zap.String("relay_url", budget.RelayURL),
		zap.String("period", budget.Period),
		zap.Int64("limit_micro_cents", budget.LimitMicroCents))

	return nil
}

// UpdateRelayBudget updates an existing relay budget
func (r *CostTrackingRepository) UpdateRelayBudget(ctx context.Context, budget *models.RelayBudget) error {
	// Call BeforeUpdate to set up the model
	if err := budget.BeforeUpdate(); err != nil {
		return fmt.Errorf("before update validation failed: %w", err)
	}

	// Update the relay budget record
	err := r.db.WithContext(ctx).Model(budget).Update()
	if err != nil {
		return MapErrorWithContext(err, "failed to update relay budget")
	}

	return nil
}

// GetRelayBudget retrieves relay budget configuration
func (r *CostTrackingRepository) GetRelayBudget(ctx context.Context, relayURL, period string) (*models.RelayBudget, error) {
	var budget models.RelayBudget

	pk := fmt.Sprintf("RELAY_BUDGET#%s#%s", relayURL, period)
	sk := models.SKConfig

	err := r.db.WithContext(ctx).Model(&models.RelayBudget{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&budget)

	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get relay budget")
	}

	return &budget, nil
}

// AggregateRelayCosts aggregates raw relay cost data into metrics
func (r *CostTrackingRepository) AggregateRelayCosts(ctx context.Context, relayURL, period string, windowStart, windowEnd time.Time) error {
	// Get all relay costs in the window
	costs, err := r.GetRelayCostsByURL(ctx, relayURL, windowStart, windowEnd, 10000)
	if err != nil {
		return fmt.Errorf("failed to get relay costs for aggregation: %w", err)
	}

	if len(costs) == 0 {
		return nil // Nothing to aggregate
	}

	// Calculate aggregated values
	metrics := &models.RelayMetrics{
		RelayURL:           relayURL,
		Period:             period,
		WindowStart:        windowStart,
		WindowEnd:          windowEnd,
		OperationBreakdown: make(map[string]*models.RelayOperationStats),
	}

	// Extract domain from first cost record
	if len(costs) > 0 {
		metrics.Domain = costs[0].Domain
	}

	// Aggregate costs by operation type
	opStats := make(map[string]*models.RelayOperationStats)
	var totalResponseTime float64

	for _, cost := range costs {
		metrics.TotalOperations++
		metrics.TotalHTTPRequests += cost.HTTPRequestCount
		metrics.TotalDataTransferBytes += cost.DataTransferBytes
		metrics.TotalLambdaDurationMs += cost.LambdaDurationMs
		metrics.TotalDynamoDBOps += cost.DynamoDBOperations
		metrics.TotalSQSMessages += cost.SQSMessages

		metrics.TotalHTTPRequestCost += cost.HTTPRequestCost
		metrics.TotalDataTransferCost += cost.DataTransferCost
		metrics.TotalLambdaCost += cost.LambdaCost
		metrics.TotalDynamoDBCost += cost.DynamoDBCost
		metrics.TotalSQSCost += cost.SQSCost
		metrics.TotalCostMicroCents += cost.TotalCostMicroCents

		if cost.Success {
			metrics.SuccessfulOperations++
		} else {
			metrics.FailedOperations++
		}

		totalResponseTime += float64(cost.ResponseTimeMs)

		// Track by operation type
		stats, exists := opStats[cost.OperationType]
		if !exists {
			stats = &models.RelayOperationStats{
				OperationType: cost.OperationType,
			}
			opStats[cost.OperationType] = stats
		}

		stats.Count++
		stats.TotalCostMicroCents += cost.TotalCostMicroCents
		totalResponseTime += float64(cost.ResponseTimeMs)
		if cost.Success {
			stats.SuccessCount++
		} else {
			stats.FailureCount++
		}
	}

	// Calculate averages
	if metrics.TotalOperations > 0 {
		metrics.AverageResponseTimeMs = totalResponseTime / float64(metrics.TotalOperations)
	}

	// Calculate operation averages
	for opType, stats := range opStats {
		if stats.Count > 0 {
			stats.AverageResponseTime = totalResponseTime / float64(stats.Count)
			stats.SuccessRate = float64(stats.SuccessCount) / float64(stats.Count)
		}
		metrics.OperationBreakdown[opType] = stats
	}

	// Check if metrics already exist for this window
	existing, err := r.GetRelayMetrics(ctx, relayURL, period, windowStart)
	if err == nil && existing != nil {
		// Update existing metrics
		metrics.CreatedAt = existing.CreatedAt
		return r.UpdateRelayMetrics(ctx, metrics)
	}

	// Create new metrics
	return r.CreateRelayMetrics(ctx, metrics)
}

// GetRelayCostSummary calculates cost summary for a relay
func (r *CostTrackingRepository) GetRelayCostSummary(ctx context.Context, relayURL string, startTime, endTime time.Time) (*RelayCostSummary, error) {
	costs, err := r.GetRelayCostsByURL(ctx, relayURL, startTime, endTime, 10000)
	if err != nil {
		return nil, err
	}

	summary := &RelayCostSummary{
		RelayURL:           relayURL,
		StartTime:          startTime,
		EndTime:            endTime,
		Count:              len(costs),
		OperationBreakdown: make(map[string]*RelayOperationCostStats),
	}

	if summary.Count == 0 {
		return summary, nil
	}

	// Extract domain from first cost record
	summary.Domain = costs[0].Domain

	// Calculate statistics
	for _, cost := range costs {
		summary.TotalHTTPRequests += cost.HTTPRequestCount
		summary.TotalDataTransferBytes += cost.DataTransferBytes
		summary.TotalLambdaDurationMs += cost.LambdaDurationMs
		summary.TotalDynamoDBOps += cost.DynamoDBOperations
		summary.TotalSQSMessages += cost.SQSMessages
		summary.TotalCostMicroCents += cost.TotalCostMicroCents
		summary.TotalOperations++

		if cost.Success {
			summary.SuccessfulOperations++
		} else {
			summary.FailedOperations++
		}

		// Track by operation type
		opStats, exists := summary.OperationBreakdown[cost.OperationType]
		if !exists {
			opStats = &RelayOperationCostStats{
				OperationType: cost.OperationType,
			}
		}

		opStats.Count++
		opStats.TotalCostMicroCents += cost.TotalCostMicroCents
		opStats.TotalHTTPRequests += cost.HTTPRequestCount
		opStats.TotalDataTransferBytes += cost.DataTransferBytes

		summary.OperationBreakdown[cost.OperationType] = opStats
	}

	// Calculate totals and averages
	summary.TotalCostDollars = float64(summary.TotalCostMicroCents) / 1_000_000.0
	if summary.TotalOperations > 0 {
		summary.AverageCostPerOperation = summary.TotalCostDollars / float64(summary.TotalOperations)
		summary.SuccessRate = float64(summary.SuccessfulOperations) / float64(summary.TotalOperations)
	}

	// Calculate operation averages
	for opType, opStats := range summary.OperationBreakdown {
		if opStats.Count > 0 {
			opStats.AverageCostMicroCents = opStats.TotalCostMicroCents / opStats.Count
			opStats.TotalCostDollars = float64(opStats.TotalCostMicroCents) / 1_000_000.0
		}
		summary.OperationBreakdown[opType] = opStats
	}

	return summary, nil
}

// GetHighCostRelayOperations returns relay operations that exceed a cost threshold
func (r *CostTrackingRepository) GetHighCostRelayOperations(ctx context.Context, thresholdMicroCents int64, startTime, endTime time.Time, limit int) ([]*models.RelayCost, error) {
	// Get all recent costs
	allCosts, err := r.GetRelayCostsByDateRange(ctx, startTime, endTime, limit*10) // Get more to filter
	if err != nil {
		return nil, err
	}

	// Filter by threshold
	var highCostOps []*models.RelayCost
	for _, cost := range allCosts {
		if cost.TotalCostMicroCents >= thresholdMicroCents {
			highCostOps = append(highCostOps, cost)
			if len(highCostOps) >= limit {
				break
			}
		}
	}

	return highCostOps, nil
}

// RelayCostSummary represents cost summary for a relay
type RelayCostSummary struct {
	RelayURL                string
	Domain                  string
	StartTime               time.Time
	EndTime                 time.Time
	Count                   int
	TotalOperations         int64
	SuccessfulOperations    int64
	FailedOperations        int64
	TotalHTTPRequests       int64
	TotalDataTransferBytes  int64
	TotalLambdaDurationMs   int64
	TotalDynamoDBOps        int64
	TotalSQSMessages        int64
	TotalCostMicroCents     int64
	TotalCostDollars        float64
	AverageCostPerOperation float64
	SuccessRate             float64
	OperationBreakdown      map[string]*RelayOperationCostStats
}

// RelayOperationCostStats represents cost statistics for a specific relay operation type
type RelayOperationCostStats struct {
	OperationType          string
	Count                  int64
	TotalCostMicroCents    int64
	TotalCostDollars       float64
	AverageCostMicroCents  int64
	TotalHTTPRequests      int64
	TotalDataTransferBytes int64
}

// Import/Export Cost Aggregation Methods

// GetImportExportCostsByUser retrieves combined import and export costs for a user
func (r *CostTrackingRepository) GetImportExportCostsByUser(ctx context.Context, username string, startDate, endDate time.Time) (*ImportExportUserCostSummary, error) {
	// Get import costs
	importCosts, err := r.GetCostsByService(ctx, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get import costs: %w", err)
	}

	// Get export costs
	exportCosts, err := r.GetCostsByService(ctx, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get export costs: %w", err)
	}

	summary := &ImportExportUserCostSummary{
		Username:    username,
		StartDate:   startDate,
		EndDate:     endDate,
		ImportCosts: &UserServiceCosts{},
		ExportCosts: &UserServiceCosts{},
	}

	// Aggregate import-processor costs
	if importProcessorStats, exists := importCosts["import-processor"]; exists {
		summary.ImportCosts.TotalOperations = importProcessorStats.OperationCount
		summary.ImportCosts.TotalCostMicroCents = importProcessorStats.TotalCostMicroCents
		summary.ImportCosts.TotalCostDollars = importProcessorStats.TotalCostDollars
		summary.ImportCosts.AverageCostPerOperation = importProcessorStats.AverageCostPerOp
	}

	// Aggregate export-generator costs
	if exportGeneratorStats, exists := exportCosts["export-generator"]; exists {
		summary.ExportCosts.TotalOperations = exportGeneratorStats.OperationCount
		summary.ExportCosts.TotalCostMicroCents = exportGeneratorStats.TotalCostMicroCents
		summary.ExportCosts.TotalCostDollars = exportGeneratorStats.TotalCostDollars
		summary.ExportCosts.AverageCostPerOperation = exportGeneratorStats.AverageCostPerOp
	}

	// Calculate combined totals
	summary.CombinedCosts = &UserServiceCosts{
		TotalOperations:     summary.ImportCosts.TotalOperations + summary.ExportCosts.TotalOperations,
		TotalCostMicroCents: summary.ImportCosts.TotalCostMicroCents + summary.ExportCosts.TotalCostMicroCents,
		TotalCostDollars:    summary.ImportCosts.TotalCostDollars + summary.ExportCosts.TotalCostDollars,
	}

	if summary.CombinedCosts.TotalOperations > 0 {
		summary.CombinedCosts.AverageCostPerOperation = summary.CombinedCosts.TotalCostDollars / float64(summary.CombinedCosts.TotalOperations)
	}

	return summary, nil
}

// GetImportExportTrends calculates cost trends for import/export operations
func (r *CostTrackingRepository) GetImportExportTrends(ctx context.Context, lookbackDays int) (*ImportExportTrends, error) {
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -lookbackDays)

	// Get daily aggregated data for import and export services
	importTrend, err := r.GetCostTrends(ctx, "daily", "ImportProcessing", lookbackDays)
	if err != nil {
		return nil, fmt.Errorf("failed to get import trends: %w", err)
	}

	exportTrend, err := r.GetCostTrends(ctx, "daily", "ExportGeneration", lookbackDays)
	if err != nil {
		return nil, fmt.Errorf("failed to get export trends: %w", err)
	}

	trends := &ImportExportTrends{
		StartTime:   startTime,
		EndTime:     endTime,
		ImportTrend: importTrend,
		ExportTrend: exportTrend,
		CombinedTrend: &CostTrend{
			Period:        "daily",
			OperationType: "ImportExport",
			StartTime:     startTime,
			EndTime:       endTime,
			DataPoints:    make([]CostDataPoint, 0),
		},
	}

	// Combine trends by date
	combinedDataMap := make(map[string]CostDataPoint)

	// Add import data points
	for _, dp := range importTrend.DataPoints {
		dateKey := dp.Timestamp.Format(common.DateFormat)
		combinedDataMap[dateKey] = dp
	}

	// Add export data points
	for _, dp := range exportTrend.DataPoints {
		dateKey := dp.Timestamp.Format(common.DateFormat)
		if existing, exists := combinedDataMap[dateKey]; exists {
			existing.CostDollars += dp.CostDollars
			existing.Operations += dp.Operations
			existing.ReadCapacity += dp.ReadCapacity
			existing.WriteCapacity += dp.WriteCapacity
			combinedDataMap[dateKey] = existing
		} else {
			combinedDataMap[dateKey] = dp
		}
	}

	// Convert map to sorted slice
	for _, dp := range combinedDataMap {
		trends.CombinedTrend.DataPoints = append(trends.CombinedTrend.DataPoints, dp)
	}

	// Sort by timestamp
	sort.Slice(trends.CombinedTrend.DataPoints, func(i, j int) bool {
		return trends.CombinedTrend.DataPoints[i].Timestamp.Before(trends.CombinedTrend.DataPoints[j].Timestamp)
	})

	// Calculate combined statistics
	var totalCost float64
	var minCost, maxCost float64

	if len(trends.CombinedTrend.DataPoints) > 0 {
		minCost = trends.CombinedTrend.DataPoints[0].CostDollars
		maxCost = trends.CombinedTrend.DataPoints[0].CostDollars
	}

	for _, dp := range trends.CombinedTrend.DataPoints {
		totalCost += dp.CostDollars
		if dp.CostDollars < minCost {
			minCost = dp.CostDollars
		}
		if dp.CostDollars > maxCost {
			maxCost = dp.CostDollars
		}
	}

	trends.CombinedTrend.TotalCost = totalCost
	trends.CombinedTrend.MinCost = minCost
	trends.CombinedTrend.MaxCost = maxCost

	if len(trends.CombinedTrend.DataPoints) > 0 {
		trends.CombinedTrend.AverageCost = totalCost / float64(len(trends.CombinedTrend.DataPoints))

		// Calculate trend percentage
		if len(trends.CombinedTrend.DataPoints) >= 2 {
			firstCost := trends.CombinedTrend.DataPoints[0].CostDollars
			lastCost := trends.CombinedTrend.DataPoints[len(trends.CombinedTrend.DataPoints)-1].CostDollars
			if firstCost > 0 {
				trends.CombinedTrend.TrendPercentage = ((lastCost - firstCost) / firstCost) * 100
			}
		}
	}

	return trends, nil
}

// GetTopCostlyUsers returns users with highest import/export costs
func (r *CostTrackingRepository) GetTopCostlyUsers(ctx context.Context, startDate, endDate time.Time, limit int) ([]*UserCostRanking, error) {
	// Get all costs in the time range for import and export services
	costs, err := r.GetRecentCosts(ctx, startDate, 10000)
	if err != nil {
		return nil, err
	}

	userCostMap := make(map[string]*UserCostRanking)

	for _, cost := range costs {
		if cost.Timestamp.After(endDate) {
			continue
		}

		// Only include import and export services
		if cost.ServiceName != "import-processor" && cost.ServiceName != "export-generator" {
			continue
		}

		// Extract username from properties or tags
		username := ""
		if cost.Properties != nil {
			if u, ok := cost.Properties["username"].(string); ok {
				username = u
			}
		}
		if username == "" && cost.Tags != nil {
			username = cost.Tags["username"]
		}
		if username == "" {
			continue
		}

		ranking, exists := userCostMap[username]
		if !exists {
			ranking = &UserCostRanking{
				Username: username,
			}
			userCostMap[username] = ranking
		}

		ranking.TotalCostMicroCents += cost.TotalCostMicroCents
		ranking.TotalOperations++

		switch cost.ServiceName {
		case "import-processor":
			ranking.ImportOperations++
			ranking.ImportCostMicroCents += cost.TotalCostMicroCents
		case "export-generator":
			ranking.ExportOperations++
			ranking.ExportCostMicroCents += cost.TotalCostMicroCents
		}
	}

	// Convert map to slice and calculate totals
	rankings := make([]*UserCostRanking, 0, len(userCostMap))
	for _, ranking := range userCostMap {
		ranking.TotalCostDollars = float64(ranking.TotalCostMicroCents) / 1_000_000.0
		ranking.ImportCostDollars = float64(ranking.ImportCostMicroCents) / 1_000_000.0
		ranking.ExportCostDollars = float64(ranking.ExportCostMicroCents) / 1_000_000.0

		if ranking.TotalOperations > 0 {
			ranking.AverageCostPerOperation = ranking.TotalCostDollars / float64(ranking.TotalOperations)
		}

		rankings = append(rankings, ranking)
	}

	// Sort by total cost (highest first)
	sort.Slice(rankings, func(i, j int) bool {
		return rankings[i].TotalCostMicroCents > rankings[j].TotalCostMicroCents
	})

	// Limit results
	if len(rankings) > limit {
		rankings = rankings[:limit]
	}

	return rankings, nil
}

// GetImportExportMetrics calculates key metrics for import/export operations
func (r *CostTrackingRepository) GetImportExportMetrics(ctx context.Context, startDate, endDate time.Time) (*ImportExportMetrics, error) {
	// Get costs by service
	serviceCosts, err := r.GetCostsByService(ctx, startDate, endDate)
	if err != nil {
		return nil, err
	}

	metrics := &ImportExportMetrics{
		StartDate: startDate,
		EndDate:   endDate,
	}

	// Calculate import metrics
	if importStats, exists := serviceCosts["import-processor"]; exists {
		metrics.TotalImportOperations = importStats.OperationCount
		metrics.TotalImportCostMicroCents = importStats.TotalCostMicroCents
		metrics.TotalImportCostDollars = importStats.TotalCostDollars
		metrics.AverageImportCost = importStats.AverageCostPerOp
	}

	// Calculate export metrics
	if exportStats, exists := serviceCosts["export-generator"]; exists {
		metrics.TotalExportOperations = exportStats.OperationCount
		metrics.TotalExportCostMicroCents = exportStats.TotalCostMicroCents
		metrics.TotalExportCostDollars = exportStats.TotalCostDollars
		metrics.AverageExportCost = exportStats.AverageCostPerOp
	}

	// Calculate combined metrics
	metrics.TotalOperations = metrics.TotalImportOperations + metrics.TotalExportOperations
	metrics.TotalCostMicroCents = metrics.TotalImportCostMicroCents + metrics.TotalExportCostMicroCents
	metrics.TotalCostDollars = metrics.TotalImportCostDollars + metrics.TotalExportCostDollars

	if metrics.TotalOperations > 0 {
		metrics.AverageCostPerOperation = metrics.TotalCostDollars / float64(metrics.TotalOperations)
	}

	// Calculate cost efficiency (operations per dollar)
	if metrics.TotalCostDollars > 0 {
		metrics.OperationsPerDollar = float64(metrics.TotalOperations) / metrics.TotalCostDollars
	}

	// Calculate cost distribution
	if metrics.TotalCostDollars > 0 {
		metrics.ImportCostPercentage = (metrics.TotalImportCostDollars / metrics.TotalCostDollars) * 100
		metrics.ExportCostPercentage = (metrics.TotalExportCostDollars / metrics.TotalCostDollars) * 100
	}

	return metrics, nil
}

// ImportExportUserCostSummary represents cost summary for a user's import/export operations
type ImportExportUserCostSummary struct {
	Username      string            `json:"username"`
	StartDate     time.Time         `json:"start_date"`
	EndDate       time.Time         `json:"end_date"`
	ImportCosts   *UserServiceCosts `json:"import_costs"`
	ExportCosts   *UserServiceCosts `json:"export_costs"`
	CombinedCosts *UserServiceCosts `json:"combined_costs"`
}

// UserServiceCosts represents cost statistics for a user's service usage
type UserServiceCosts struct {
	TotalOperations         int64   `json:"total_operations"`
	TotalCostMicroCents     int64   `json:"total_cost_micro_cents"`
	TotalCostDollars        float64 `json:"total_cost_dollars"`
	AverageCostPerOperation float64 `json:"average_cost_per_operation"`
}

// ImportExportTrends represents cost trends for import/export operations
type ImportExportTrends struct {
	StartTime     time.Time  `json:"start_time"`
	EndTime       time.Time  `json:"end_time"`
	ImportTrend   *CostTrend `json:"import_trend"`
	ExportTrend   *CostTrend `json:"export_trend"`
	CombinedTrend *CostTrend `json:"combined_trend"`
}

// UserCostRanking represents a user's cost ranking
type UserCostRanking struct {
	Username                string  `json:"username"`
	TotalOperations         int64   `json:"total_operations"`
	ImportOperations        int64   `json:"import_operations"`
	ExportOperations        int64   `json:"export_operations"`
	TotalCostMicroCents     int64   `json:"total_cost_micro_cents"`
	ImportCostMicroCents    int64   `json:"import_cost_micro_cents"`
	ExportCostMicroCents    int64   `json:"export_cost_micro_cents"`
	TotalCostDollars        float64 `json:"total_cost_dollars"`
	ImportCostDollars       float64 `json:"import_cost_dollars"`
	ExportCostDollars       float64 `json:"export_cost_dollars"`
	AverageCostPerOperation float64 `json:"average_cost_per_operation"`
}

// GetActivityCost retrieves cost tracking data for a specific activity
func (r *CostTrackingRepository) GetActivityCost(ctx context.Context, activityID string) (*models.DynamoDBCostRecord, error) {
	r.logger.Debug("Getting activity cost",
		zap.String("activity_id", activityID))

	// Query for cost record with activity-specific partition key
	var costRecord models.DynamoDBCostRecord

	// Try to find cost record by activity ID
	// Using the primary key pattern for activity costs
	pk := fmt.Sprintf("ACTIVITY_COST#%s", activityID)
	sk := "COST_RECORD"

	err := r.db.WithContext(ctx).Model(&models.DynamoDBCostRecord{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&costRecord)

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("Activity cost not found",
				zap.String("activity_id", activityID))
			return nil, nil
		}
		r.logger.Error("Failed to query activity cost",
			zap.String("activity_id", activityID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query activity cost: %w", err)
	}

	r.logger.Debug("Retrieved activity cost",
		zap.String("activity_id", activityID),
		zap.Int64("total_cost_microcents", costRecord.TotalCostMicroCents))

	return &costRecord, nil
}

// ImportExportMetrics represents key metrics for import/export operations
type ImportExportMetrics struct {
	StartDate                 time.Time `json:"start_date"`
	EndDate                   time.Time `json:"end_date"`
	TotalOperations           int64     `json:"total_operations"`
	TotalImportOperations     int64     `json:"total_import_operations"`
	TotalExportOperations     int64     `json:"total_export_operations"`
	TotalCostMicroCents       int64     `json:"total_cost_micro_cents"`
	TotalImportCostMicroCents int64     `json:"total_import_cost_micro_cents"`
	TotalExportCostMicroCents int64     `json:"total_export_cost_micro_cents"`
	TotalCostDollars          float64   `json:"total_cost_dollars"`
	TotalImportCostDollars    float64   `json:"total_import_cost_dollars"`
	TotalExportCostDollars    float64   `json:"total_export_cost_dollars"`
	AverageCostPerOperation   float64   `json:"average_cost_per_operation"`
	AverageImportCost         float64   `json:"average_import_cost"`
	AverageExportCost         float64   `json:"average_export_cost"`
	OperationsPerDollar       float64   `json:"operations_per_dollar"`
	ImportCostPercentage      float64   `json:"import_cost_percentage"`
	ExportCostPercentage      float64   `json:"export_cost_percentage"`
}
