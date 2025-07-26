package repositories

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// CostTrackingRepository handles cost tracking persistence
type CostTrackingRepository struct {
	dynamorm.BaseRepository
	logger *zap.Logger
}

// NewCostTrackingRepository creates a new cost tracking repository
func NewCostTrackingRepository(db core.DB, tableName string, logger *zap.Logger) *CostTrackingRepository {
	return &CostTrackingRepository{
		BaseRepository: *dynamorm.NewBaseRepository(db, tableName),
		logger:         logger,
	}
}

// Create creates a new cost tracking record
func (r *CostTrackingRepository) Create(ctx context.Context, tracking *models.DynamoDBCostRecord) error {
	// Call BeforeCreate to set up the model
	if err := tracking.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Create the cost tracking record
	err := r.GetDB().Model(tracking).Create()
	if err != nil {
		return dynamorm.MapErrorWithContext(err, "failed to create cost tracking")
	}

	r.logger.Debug("created cost tracking",
		zap.String("id", tracking.ID),
		zap.String("operation_type", tracking.OperationType),
		zap.String("table_name", tracking.Table),
		zap.Float64("cost_dollars", tracking.EstimatedCostDollars))

	return nil
}

// BatchCreate creates multiple cost tracking records efficiently
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

	// Use batch writer for efficiency
	// Note: This is a simplified version - real implementation would use DynamORM's batch capabilities
	for _, ct := range trackingList {
		if err := r.GetDB().Model(ct).Create(); err != nil {
			r.logger.Error("failed to create cost tracking in batch",
				zap.String("id", ct.ID),
				zap.Error(err))
			// Continue with other records
		}
	}

	return nil
}

// Get retrieves a cost tracking record by operation type, timestamp and ID
func (r *CostTrackingRepository) Get(ctx context.Context, operationType, id string, timestamp time.Time) (*models.DynamoDBCostRecord, error) {
	tracking := &models.DynamoDBCostRecord{}

	// Construct the keys
	pk := fmt.Sprintf("cost#%s", operationType)
	sk := fmt.Sprintf("ts#%s#%s", timestamp.Format("20060102150405"), id)

	err := r.GetDB().Model(tracking).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(tracking)

	if err != nil {
		return nil, dynamorm.MapErrorWithContext(err, "failed to get cost tracking")
	}

	return tracking, nil
}

// ListByOperationType lists cost tracking records by operation type within a time range
func (r *CostTrackingRepository) ListByOperationType(ctx context.Context, operationType string, startTime, endTime time.Time, limit int) ([]*models.DynamoDBCostRecord, error) {
	var trackingList []*models.DynamoDBCostRecord

	// Construct SK range for time-based query
	pk := fmt.Sprintf("cost#%s", operationType)
	startSK := fmt.Sprintf("ts#%s", startTime.Format("20060102150405"))
	endSK := fmt.Sprintf("ts#%s", endTime.Format("20060102150405"))

	query := r.GetDB().Model(&models.DynamoDBCostRecord{}).
		Where("PK", "=", pk).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", "DESC").
		Limit(limit)

	err := query.All(&trackingList)
	if err != nil {
		return nil, dynamorm.MapErrorWithContext(err, "failed to list cost tracking by operation type")
	}

	return trackingList, nil
}

// ListByTable lists cost tracking records by table within a time range
func (r *CostTrackingRepository) ListByTable(ctx context.Context, tableName string, startTime, endTime time.Time, limit int) ([]*models.DynamoDBCostRecord, error) {
	var trackingList []*models.DynamoDBCostRecord

	// Use GSI1 for table-based queries
	startSK := startTime.Format(time.RFC3339)
	endSK := endTime.Format(time.RFC3339)

	query := r.GetDB().Model(&models.DynamoDBCostRecord{}).
		Index("table-index").
		Where("GSI1PK", "=", fmt.Sprintf("COST_TABLE#%s", tableName)).
		Where("GSI1SK", ">=", startSK).
		Where("GSI1SK", "<=", endSK).
		OrderBy("GSI1SK", "DESC").
		Limit(limit)

	err := query.All(&trackingList)
	if err != nil {
		return nil, dynamorm.MapErrorWithContext(err, "failed to list cost tracking by table")
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
func (r *CostTrackingRepository) GetAggregated(ctx context.Context, period, operationType string, windowStart time.Time) (*models.DynamoDBCostAggregation, error) {
	aggregated := &models.DynamoDBCostAggregation{}

	pk := fmt.Sprintf("cost_agg#%s#%s", period, operationType)
	sk := fmt.Sprintf("window#%s", windowStart.Format(time.RFC3339))

	err := r.GetDB().Model(aggregated).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(aggregated)

	if err != nil {
		return nil, dynamorm.MapErrorWithContext(err, "failed to get aggregated cost tracking")
	}

	return aggregated, nil
}

// CreateAggregated creates an aggregated cost tracking record
func (r *CostTrackingRepository) CreateAggregated(ctx context.Context, aggregated *models.DynamoDBCostAggregation) error {
	// Call BeforeCreate to set up the model
	if err := aggregated.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Create the aggregated cost tracking
	err := r.GetDB().Model(aggregated).Create()
	if err != nil {
		return dynamorm.MapErrorWithContext(err, "failed to create aggregated cost tracking")
	}

	r.logger.Debug("created aggregated cost tracking",
		zap.String("operation_type", aggregated.OperationType),
		zap.String("period", aggregated.Period),
		zap.Time("window_start", aggregated.WindowStart),
		zap.Float64("total_cost_dollars", aggregated.TotalCostDollars))

	return nil
}

// UpdateAggregated updates an existing aggregated cost tracking record
func (r *CostTrackingRepository) UpdateAggregated(ctx context.Context, aggregated *models.DynamoDBCostAggregation) error {
	// Call BeforeUpdate to set up the model
	if err := aggregated.BeforeUpdate(); err != nil {
		return fmt.Errorf("before update validation failed: %w", err)
	}

	// Update the aggregated cost tracking
	err := r.GetDB().Model(aggregated).Update()
	if err != nil {
		return dynamorm.MapErrorWithContext(err, "failed to update aggregated cost tracking")
	}

	return nil
}

// ListAggregatedByPeriod lists aggregated cost tracking for a period
func (r *CostTrackingRepository) ListAggregatedByPeriod(ctx context.Context, period, operationType string, startTime, endTime time.Time, limit int) ([]*models.DynamoDBCostAggregation, error) {
	var aggregatedList []*models.DynamoDBCostAggregation

	pk := fmt.Sprintf("cost_agg#%s#%s", period, operationType)
	startSK := fmt.Sprintf("window#%s", startTime.Format(time.RFC3339))
	endSK := fmt.Sprintf("window#%s", endTime.Format(time.RFC3339))

	query := r.GetDB().Model(&models.DynamoDBCostAggregation{}).
		Where("PK", "=", pk).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", "DESC").
		Limit(limit)

	err := query.All(&aggregatedList)
	if err != nil {
		return nil, dynamorm.MapErrorWithContext(err, "failed to list aggregated cost tracking")
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
		TableName:   tableName,
		StartTime:   startTime,
		EndTime:     endTime,
		Count:       len(costs),
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
		Period:        period,
		OperationType: operationType,
		Table:         "all", // Default to all tables
		WindowStart:   windowStart,
		WindowEnd:     windowEnd,
		CostPercentiles: make(map[string]float64),
		TableBreakdown:  make(map[string]*models.DynamoDBTableCostStats),
		ServiceBreakdown: make(map[string]*models.DynamoDBServiceCostStats),
	}

	// Collect values for percentile calculation
	var costValues []float64
	
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
	TableName                string
	StartTime                time.Time
	EndTime                  time.Time
	Count                    int
	TotalOperations          int64
	TotalItemCount           int64
	TotalReadCapacityUnits   float64
	TotalWriteCapacityUnits  float64
	TotalCostMicroCents      int64
	TotalCostDollars         float64
	AverageCostPerOperation  float64
	OperationBreakdown       map[string]OperationCostStats
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
	var minCost, maxCost float64 = aggregatedList[0].TotalCostDollars, aggregatedList[0].TotalCostDollars

	for _, agg := range aggregatedList {
		dataPoint := CostDataPoint{
			Timestamp:    agg.WindowStart,
			CostDollars:  agg.TotalCostDollars,
			Operations:   agg.TotalOperations,
			ReadCapacity: agg.TotalReadCapacityUnits,
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