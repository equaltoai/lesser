package repositories

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// Use common time format constants from pkg/common/time_formats.go
// common.CompactTimeFormat is replaced by common.CompactTimeFormat

// TrackingRepository handles cost tracking persistence
type TrackingRepository struct {
	*EnhancedBaseRepository[*models.DynamoDBCostRecord]

	listAggregatedByPeriodFn func(ctx context.Context, period, operationType string, startTime, endTime time.Time, limit int, cursor string) ([]*models.DynamoDBCostAggregation, string, error)
}

const (
	costTableDefaultLimit    = 100
	costTableMaxLimit        = 1000
	relayCostDefaultLimit    = 100
	relayCostMaxLimit        = 1000
	relayCostDateCap         = 1000
	relayCostPerDayCap       = 200
	relayMetricsDefaultLimit = 100
	relayMetricsMaxLimit     = 500
	aggregatedPageMaxLimit   = 500
)

func clampCostTableLimit(limit int) int {
	if limit <= 0 {
		return costTableDefaultLimit
	}
	if limit > costTableMaxLimit {
		return costTableMaxLimit
	}
	return limit
}

func clampRelayCostLimit(limit int) int {
	if limit <= 0 {
		return relayCostDefaultLimit
	}
	if limit > relayCostMaxLimit {
		return relayCostMaxLimit
	}
	return limit
}

func clampRelayMetricsLimit(limit int) int {
	if limit <= 0 {
		return relayMetricsDefaultLimit
	}
	if limit > relayMetricsMaxLimit {
		return relayMetricsMaxLimit
	}
	return limit
}

// NewTrackingRepository creates a new cost tracking repository with enhanced functionality
func NewTrackingRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *TrackingRepository {
	// Create enhanced repository optimized for cost tracking operations
	enhancedRepo := NewEnhancedBaseRepository[*models.DynamoDBCostRecord](db, tableName, logger, costService, "TrackingRepository", "costtracking")

	// Set up enhanced services for cost tracking operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Cost data cached for performance
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for cost tracking events

	return &TrackingRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// Create creates a new cost tracking record
func (r *TrackingRepository) Create(ctx context.Context, tracking *models.DynamoDBCostRecord) error {
	return r.ValidateAndCreate(ctx, tracking)
}

// BatchCreate creates multiple cost tracking records efficiently using enhanced validation
func (r *TrackingRepository) BatchCreate(ctx context.Context, trackingList []*models.DynamoDBCostRecord) error {
	return r.ValidateAndBatchCreate(ctx, trackingList)
}

// Get retrieves a cost tracking record by operation type, timestamp and ID
func (r *TrackingRepository) Get(ctx context.Context, operationType, id string, timestamp time.Time) (*models.DynamoDBCostRecord, error) {
	// Construct the keys using the same pattern as the model
	pk := fmt.Sprintf("cost#%s", operationType)
	sk := fmt.Sprintf("ts#%s#%s", timestamp.Format("20060102150405"), id)

	tracking := &models.DynamoDBCostRecord{}
	err := r.BaseRepository.Get(ctx, pk, sk, tracking)
	return tracking, err
}

// ListByOperationType lists cost tracking records by operation type within a time range
func (r *TrackingRepository) ListByOperationType(ctx context.Context, operationType string, startTime, endTime time.Time, limit int) ([]*models.DynamoDBCostRecord, error) {
	// Construct SK range for time-based query
	pk := fmt.Sprintf("cost#%s", operationType)
	startSK := fmt.Sprintf("ts#%s", startTime.Format("20060102150405"))
	endSK := fmt.Sprintf("ts#%s", endTime.Format("20060102150405"))

	return r.QueryBetween(ctx, pk, startSK, endSK, limit)
}

// ListByTable lists cost tracking records by table within a time range
func (r *TrackingRepository) ListByTable(ctx context.Context, tableName string, startTime, endTime time.Time, limit int, cursor string) ([]*models.DynamoDBCostRecord, string, error) {
	var trackingList []*models.DynamoDBCostRecord

	safeLimit := clampCostTableLimit(limit)

	// Use GSI1 for table-based queries
	startSK := startTime.Format(time.RFC3339)
	endSK := endTime.Format(time.RFC3339)

	query := r.db.WithContext(ctx).Model(&models.DynamoDBCostRecord{}).
		Index("table-index").
		Where("gsI1PK", "=", fmt.Sprintf("COST_TABLE#%s", tableName)).
		Where("gsI1SK", ">=", startSK).
		Where("gsI1SK", "<=", endSK).
		OrderBy("GSI1SK", "DESC").
		Limit(safeLimit + 1)

	if cursor != "" {
		query = query.Where("gsI1SK", "<", cursor)
	}

	err := query.All(&trackingList)
	if err != nil {
		return nil, "", MapErrorWithContext(err, "failed to list cost tracking by table")
	}

	var nextCursor string
	if len(trackingList) > safeLimit {
		nextCursor = trackingList[safeLimit-1].GSI1SK
		trackingList = trackingList[:safeLimit]
	}

	return trackingList, nextCursor, nil
}

// GetRecentCosts retrieves recent cost tracking records across all operations
func (r *TrackingRepository) GetRecentCosts(ctx context.Context, since time.Time, limit int) ([]*models.DynamoDBCostRecord, error) {
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
func (r *TrackingRepository) GetAggregated(ctx context.Context, period, operationType string, windowStart time.Time) (*models.DynamoDBCostAggregation, error) {
	aggregated := &models.DynamoDBCostAggregation{}

	pk := fmt.Sprintf("cost_agg#%s#%s", period, operationType)
	sk := fmt.Sprintf("window#%s", windowStart.Format(time.RFC3339))

	err := r.db.WithContext(ctx).Model(aggregated).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(aggregated)

	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get aggregated cost tracking")
	}

	return aggregated, nil
}

// CreateAggregated creates an aggregated cost tracking record
func (r *TrackingRepository) CreateAggregated(ctx context.Context, aggregated *models.DynamoDBCostAggregation) error {
	// Call BeforeCreate to set up the model
	if err := aggregated.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, "cost aggregation", "validation")
	}

	// Create the aggregated cost tracking
	err := r.db.WithContext(ctx).Model(aggregated).Create()
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
func (r *TrackingRepository) UpdateAggregated(ctx context.Context, aggregated *models.DynamoDBCostAggregation) error {
	// Call BeforeUpdate to set up the model
	if err := aggregated.BeforeUpdate(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "cost aggregation", "validation")
	}

	// Update the aggregated cost tracking
	err := r.db.WithContext(ctx).Model(aggregated).Update()
	if err != nil {
		return MapErrorWithContext(err, "failed to update aggregated cost tracking")
	}

	return nil
}

// ListAggregatedByPeriod lists aggregated cost tracking for a period
func (r *TrackingRepository) ListAggregatedByPeriod(ctx context.Context, period, operationType string, startTime, endTime time.Time, limit int, cursor string) ([]*models.DynamoDBCostAggregation, string, error) {
	if r.listAggregatedByPeriodFn != nil {
		return r.listAggregatedByPeriodFn(ctx, period, operationType, startTime, endTime, limit, cursor)
	}

	config := AggregatedQueryConfig{
		PKPrefix:    "cost_agg",
		LogContext:  "cost tracking",
		ErrorPrefix: "failed to list aggregated cost tracking",
	}

	aggregated, nextCursor, err := ListAggregatedByPeriod[*models.DynamoDBCostAggregation](
		ctx,
		r.db,
		config,
		period,
		operationType,
		startTime,
		endTime,
		limit,
		cursor,
	)
	if err != nil {
		return nil, "", err
	}

	return aggregated, nextCursor, nil
}

// GetTableCostStats calculates cost statistics for a table
func (r *TrackingRepository) GetTableCostStats(ctx context.Context, tableName string, startTime, endTime time.Time) (*TableCostStats, error) {
	const maxStatsRecords = 10000

	var (
		allCosts  []*models.DynamoDBCostRecord
		cursor    string
		remaining = maxStatsRecords
	)

	for remaining > 0 {
		pageLimit := remaining
		if pageLimit > costTableMaxLimit {
			pageLimit = costTableMaxLimit
		}

		costs, nextCursor, err := r.ListByTable(ctx, tableName, startTime, endTime, pageLimit, cursor)
		if err != nil {
			return nil, err
		}

		allCosts = append(allCosts, costs...)
		if nextCursor == "" || len(costs) == 0 {
			break
		}

		cursor = nextCursor
		remaining -= len(costs)
	}

	stats := &TableCostStats{
		TableName:          tableName,
		StartTime:          startTime,
		EndTime:            endTime,
		Count:              len(allCosts),
		OperationBreakdown: make(map[string]OperationCostStats),
	}

	if stats.Count == 0 {
		return stats, nil
	}

	// Calculate statistics
	for _, ct := range allCosts {
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
func (r *TrackingRepository) Aggregate(ctx context.Context, operationType, period string, windowStart, windowEnd time.Time) error {
	// Get all cost tracking records in the window
	costs, err := r.ListByOperationType(ctx, operationType, windowStart, windowEnd, 10000)
	if err != nil {
		return ErrorHandler.HandleQueryError(err, "cost tracking", "aggregation")
	}

	if err := common.ValidateSliceNotEmpty("costs", costs); err != nil {
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
				Table: ct.Table,
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
	if err := common.ValidateSliceNotEmpty("cost_values", costValues); err == nil {
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
func (r *TrackingRepository) GetHighCostOperations(ctx context.Context, thresholdDollars float64, startTime, endTime time.Time, limit int) ([]*models.DynamoDBCostRecord, error) {
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
func (r *TrackingRepository) GetCostTrends(ctx context.Context, period string, operationType string, lookbackDays int) (*CostTrend, error) {
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -lookbackDays)

	// Get aggregated data for the period using paginated fetches
	var (
		aggregatedList []*models.DynamoDBCostAggregation
		cursor         string
		remaining      = 1000
	)

	for remaining > 0 {
		pageSize := remaining
		if pageSize > aggregatedPageMaxLimit {
			pageSize = aggregatedPageMaxLimit
		}

		chunk, nextCursor, err := r.ListAggregatedByPeriod(ctx, period, operationType, startTime, endTime, pageSize, cursor)
		if err != nil {
			return nil, err
		}

		aggregatedList = append(aggregatedList, chunk...)
		if nextCursor == "" || len(chunk) == 0 {
			break
		}

		cursor = nextCursor
		remaining -= len(chunk)
	}

	if err := common.ValidateSliceNotEmpty("aggregated_list", aggregatedList); err != nil {
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

	// Calculate enhanced trend analysis with linear regression
	if len(trend.DataPoints) >= 2 {
		// Simple percentage for backward compatibility
		firstCost := trend.DataPoints[0].CostDollars
		lastCost := trend.DataPoints[len(trend.DataPoints)-1].CostDollars
		if firstCost > 0 {
			trend.TrendPercentage = ((lastCost - firstCost) / firstCost) * 100
		}

		// Enhanced statistical analysis
		trend.LinearRegression = r.calculateLinearRegressionStats(trend.DataPoints)
		trend.StatisticalTests = r.calculateStatisticalTests(trend.DataPoints, trend.LinearRegression)
		trend.Anomalies = r.detectCostAnomalies(trend.DataPoints, trend.LinearRegression)
		trend.Forecast = r.generateCostForecast(trend.DataPoints, trend.LinearRegression)
		trend.Seasonality = r.analyzeSeasonality(trend.DataPoints)
	}

	return trend, nil
}

// CostTrend represents cost trend analysis with statistical analysis
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
	TrendPercentage float64 // Positive = increasing, Negative = decreasing (simple)

	// Enhanced statistical analysis
	LinearRegression *LinearRegressionStats `json:"linear_regression,omitempty"`
	StatisticalTests *StatisticalTests      `json:"statistical_tests,omitempty"`
	Anomalies        []CostAnomaly          `json:"anomalies,omitempty"`
	Forecast         *CostForecast          `json:"forecast,omitempty"`
	Seasonality      *SeasonalityAnalysis   `json:"seasonality,omitempty"`
}

// CostDataPoint represents a single point in the cost trend
type CostDataPoint struct {
	Timestamp     time.Time
	CostDollars   float64
	Operations    int64
	ReadCapacity  float64
	WriteCapacity float64
}

// Statistical analysis types for cost trends

// LinearRegressionStats represents linear regression analysis results
type LinearRegressionStats struct {
	Slope           float64 `json:"slope"`            // Rate of change per time unit
	Intercept       float64 `json:"intercept"`        // Y-intercept
	RSquared        float64 `json:"r_squared"`        // Coefficient of determination (0-1)
	Correlation     float64 `json:"correlation"`      // Correlation coefficient (-1 to 1)
	SlopeStdError   float64 `json:"slope_std_error"`  // Standard error of slope
	TStatistic      float64 `json:"t_statistic"`      // T-statistic for slope
	PValue          float64 `json:"p_value"`          // P-value for slope significance
	ConfidenceLevel float64 `json:"confidence_level"` // Confidence level (e.g., 0.95)
	TrendDirection  string  `json:"trend_direction"`  // "increasing", "decreasing", "stable"
	TrendStrength   string  `json:"trend_strength"`   // "weak", "moderate", "strong"
	IsSignificant   bool    `json:"is_significant"`   // Whether trend is statistically significant
}

// StatisticalTests represents statistical significance tests
type StatisticalTests struct {
	MannKendallTau    float64        `json:"mann_kendall_tau"` // Mann-Kendall trend test statistic
	MannKendallPValue float64        `json:"mann_kendall_p"`   // Mann-Kendall p-value
	TheilSenSlope     float64        `json:"theil_sen_slope"`  // Theil-Sen robust slope estimator
	DurbinWatson      float64        `json:"durbin_watson"`    // Durbin-Watson test for autocorrelation
	JarqueBera        float64        `json:"jarque_bera"`      // Jarque-Bera test for normality
	JarqueBeraP       float64        `json:"jarque_bera_p"`    // Jarque-Bera p-value
	ResidualStats     *ResidualStats `json:"residual_stats"`
}

// ResidualStats represents analysis of regression residuals
type ResidualStats struct {
	Mean           float64 `json:"mean"`
	StandardError  float64 `json:"standard_error"`
	Skewness       float64 `json:"skewness"`
	Kurtosis       float64 `json:"kurtosis"`
	OutlierCount   int     `json:"outlier_count"`
	NormalityScore float64 `json:"normality_score"` // 0-1, higher = more normal
}

// CostAnomaly represents a detected cost anomaly
type CostAnomaly struct {
	Timestamp      time.Time `json:"timestamp"`
	ActualCost     float64   `json:"actual_cost"`
	ExpectedCost   float64   `json:"expected_cost"`
	DeviationScore float64   `json:"deviation_score"` // Standard deviations from expected
	AnomalyType    string    `json:"anomaly_type"`    // "spike", "drop", "outlier"
	Severity       string    `json:"severity"`        // "low", "medium", "high", "critical"
	Confidence     float64   `json:"confidence"`      // 0-1
}

// CostForecast represents cost forecasting results
type CostForecast struct {
	ForecastHorizon     int                `json:"forecast_horizon"` // Number of periods ahead
	Predictions         []CostPrediction   `json:"predictions"`
	ConfidenceLevel     float64            `json:"confidence_level"` // e.g., 0.95
	MeanAbsoluteError   float64            `json:"mean_absolute_error"`
	RootMeanSquareError float64            `json:"root_mean_square_error"`
	ModelType           string             `json:"model_type"` // "linear", "exponential", "seasonal"
	SeasonalFactors     map[string]float64 `json:"seasonal_factors,omitempty"`
}

// CostPrediction represents a single cost prediction
type CostPrediction struct {
	Timestamp     time.Time `json:"timestamp"`
	PredictedCost float64   `json:"predicted_cost"`
	LowerBound    float64   `json:"lower_bound"` // Lower confidence interval
	UpperBound    float64   `json:"upper_bound"` // Upper confidence interval
	StandardError float64   `json:"standard_error"`
}

// SeasonalityAnalysis represents seasonal pattern analysis
type SeasonalityAnalysis struct {
	HasSeasonality    bool               `json:"has_seasonality"`
	SeasonalStrength  float64            `json:"seasonal_strength"` // 0-1
	SeasonalPeriod    int                `json:"seasonal_period"`   // Detected period (e.g., 7 for weekly)
	SeasonalPatterns  map[string]float64 `json:"seasonal_patterns"` // Pattern coefficients
	TrendComponent    []float64          `json:"trend_component"`
	SeasonalComponent []float64          `json:"seasonal_component"`
	ResidualComponent []float64          `json:"residual_component"`
	DecompositionR2   float64            `json:"decomposition_r2"` // Quality of decomposition
}

// calculatePercentiles calculates percentiles for a slice of values
// Returns a map with p50, p90, p95, and p99 percentiles
func calculatePercentiles(values []float64) map[string]float64 {
	if err := common.ValidateSliceNotEmpty("values", values); err != nil {
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
	if err := common.ValidateSliceNotEmpty("sorted", sorted); err != nil {
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

// fetchAggregatesForOperation retrieves all aggregated costs for a single operation type within a given time range.
// It handles pagination internally to fetch all records.
func (r *TrackingRepository) fetchAggregatesForOperation(ctx context.Context, period, opType string, startDate, endDate time.Time) ([]*models.DynamoDBCostAggregation, error) {
	var (
		opAggregates []*models.DynamoDBCostAggregation
		cursor       string
		remaining    = 1000 // A reasonable starting limit for pagination fetches.
	)

	for {
		pageSize := remaining
		if pageSize > aggregatedPageMaxLimit {
			pageSize = aggregatedPageMaxLimit
		}
		if pageSize == 0 { // Should not happen, but as a safeguard.
			break
		}

		chunk, nextCursor, err := r.ListAggregatedByPeriod(ctx, period, opType, startDate, endDate, pageSize, cursor)
		if err != nil {
			r.logger.Warn("failed to get aggregates for operation type",
				zap.String("operation_type", opType),
				zap.Error(err))
			return nil, err // Return error to let the caller decide how to proceed.
		}

		opAggregates = append(opAggregates, chunk...)
		if nextCursor == "" || len(chunk) == 0 {
			break // No more data to fetch.
		}

		cursor = nextCursor
		// We don't decrement `remaining` as we want to fetch all pages.
	}

	return opAggregates, nil
}

// mergeAggregatesByWindow merges a list of cost aggregations into a map keyed by the window start time.
// This function combines stats from different operation types that fall into the same time window.
func mergeAggregatesByWindow(allAggregates []*models.DynamoDBCostAggregation) map[string]*models.DynamoDBCostAggregation {
	mergedByWindow := make(map[string]*models.DynamoDBCostAggregation)

	for _, agg := range allAggregates {
		windowKey := agg.WindowStart.Format(time.RFC3339)

		existing, exists := mergedByWindow[windowKey]
		if !exists {
			mergedByWindow[windowKey] = agg
			continue
		}

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
			if existingStats, ok := existing.TableBreakdown[table]; ok {
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
			if existingStats, ok := existing.ServiceBreakdown[service]; ok {
				existingStats.OperationCount += stats.OperationCount
				existingStats.TotalCostMicroCents += stats.TotalCostMicroCents
				existingStats.TotalCostDollars += stats.TotalCostDollars
			} else {
				existing.ServiceBreakdown[service] = stats
			}
		}
	}

	return mergedByWindow
}

// finalizeCostMetrics converts the merged map of aggregations into a sorted slice,
// calculating final derived metrics like total cost in dollars and average cost per operation.
func finalizeCostMetrics(mergedByWindow map[string]*models.DynamoDBCostAggregation) []*models.DynamoDBCostAggregation {
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

	return result
}

// GetAggregatedCostsByPeriod retrieves aggregated costs for a specific period
func (r *TrackingRepository) GetAggregatedCostsByPeriod(ctx context.Context, period string, startDate, endDate time.Time) ([]*models.DynamoDBCostAggregation, error) {
	// Query all operation types for the period
	operationTypes := []string{"GetItem", "PutItem", "UpdateItem", "DeleteItem", "Query", "Scan",
		"BatchGetItem", "BatchWriteItem", "TransactGetItems", "TransactWriteItems"}

	var allAggregates []*models.DynamoDBCostAggregation

	for _, opType := range operationTypes {
		opAggregates, err := r.fetchAggregatesForOperation(ctx, period, opType, startDate, endDate)
		if err != nil {
			// Depending on desired behavior, we could either continue or fail fast.
			// The original implementation continued, so we'll log and continue here.
			r.logger.Warn("skipping operation type due to fetch error",
				zap.String("operation_type", opType),
				zap.Error(err))
			continue
		}
		allAggregates = append(allAggregates, opAggregates...)
	}

	// Merge aggregates by window
	merged := mergeAggregatesByWindow(allAggregates)

	// Finalize and sort the results
	result := finalizeCostMetrics(merged)

	return result, nil
}

// GetCostsByOperationType retrieves costs grouped by operation type
func (r *TrackingRepository) GetCostsByOperationType(ctx context.Context, startDate, endDate time.Time) (map[string]*models.DynamoDBServiceCostStats, error) {
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

		if err := common.ValidateSliceNotEmpty("costs", costs); err != nil {
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
func (r *TrackingRepository) GetCostsByService(ctx context.Context, startDate, endDate time.Time) (map[string]*models.DynamoDBServiceCostStats, error) {
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
		if err := common.ValidateRequiredParam("service_name", serviceName); err != nil {
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
func (r *TrackingRepository) GetCostsByDateRange(ctx context.Context, startDate, _ time.Time) ([]*models.DynamoDBCostRecord, error) {
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
func (r *TrackingRepository) GetDailyAggregates(ctx context.Context, startDate, endDate time.Time) ([]*DailyAggregate, error) {
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
func (r *TrackingRepository) GetMonthlyAggregate(ctx context.Context, year, month int) (*MonthlyAggregate, error) {
	// Calculate month boundaries
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, 0).Add(-time.Second)

	// Get aggregated data for the month
	aggregations, err := r.GetAggregatedCostsByPeriod(ctx, "month", startDate, endDate)
	if err != nil {
		return nil, err
	}

	if err := common.ValidateSliceNotEmpty("aggregations", aggregations); err != nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "cost aggregate", fmt.Sprintf("%d-%02d", year, month))
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
func (r *TrackingRepository) GetCostProjections(ctx context.Context, period string) (*storage.CostProjection, error) {
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
		return nil, ErrorHandler.HandleQueryError(err, "cost projection", "query")
	}

	// If no projection exists, return a default projection with zero values
	if err := common.ValidateSliceNotEmpty("projections", projections); err != nil {
		r.logger.Debug("no cost projections found, returning default",
			zap.String("period", period))

		return &storage.CostProjection{
			Period:          period,
			CurrentCost:     0.0,
			ProjectedCost:   0.0,
			Variance:        0.0,
			TopDrivers:      []storage.Driver{},
			Recommendations: []string{},
		}, nil
	}

	// Convert models.CostProjection to storage.CostProjection
	projection := projections[0]

	// Convert models.Driver to storage.Driver
	topDrivers := make([]storage.Driver, 0, len(projection.TopDrivers))
	for _, driver := range projection.TopDrivers {
		topDrivers = append(topDrivers, storage.Driver{
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
func (r *TrackingRepository) CreateRelayCost(ctx context.Context, relayCost *models.RelayCost) error {
	// Call BeforeCreate to set up the model
	if err := relayCost.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, "relay cost", "validation")
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
func (r *TrackingRepository) GetRelayCostsByURL(ctx context.Context, relayURL string, startTime, endTime time.Time, limit int, cursor string, operationType string) ([]*models.RelayCost, string, error) {
	var costs []*models.RelayCost

	safeLimit := clampRelayCostLimit(limit)

	startSK := fmt.Sprintf("TS#%s", startTime.Format(common.CompactTimeFormat))
	endSK := fmt.Sprintf("TS#%s", endTime.Format(common.CompactTimeFormat))

	query := r.db.WithContext(ctx).Model(&models.RelayCost{}).
		Index("GSI1").
		Where("gsI1PK", "=", fmt.Sprintf("RELAY_COSTS#%s", relayURL)).
		Where("gsI1SK", ">=", startSK).
		Where("gsI1SK", "<=", endSK).
		OrderBy("GSI1SK", "DESC").
		Limit(safeLimit + 1)

	if cursor != "" {
		query = query.Where("gsI1SK", "<", cursor)
	}

	if operationType != "" {
		query = query.Filter("OperationType", "=", operationType)
	}

	err := query.All(&costs)
	if err != nil {
		return nil, "", MapErrorWithContext(err, "failed to get relay costs by URL")
	}

	var nextCursor string
	if len(costs) > safeLimit {
		nextCursor = costs[safeLimit-1].GSI1SK
		costs = costs[:safeLimit]
	}

	return costs, nextCursor, nil
}

func (r *TrackingRepository) collectRelayCosts(ctx context.Context, relayURL string, startTime, endTime time.Time, limit int, operationType string) ([]*models.RelayCost, error) {
	remaining := limit
	if remaining <= 0 {
		remaining = relayCostMaxLimit
	}

	var (
		allCosts []*models.RelayCost
		cursor   string
	)

	for remaining > 0 {
		pageLimit := remaining
		if pageLimit > relayCostMaxLimit {
			pageLimit = relayCostMaxLimit
		}

		batch, nextCursor, err := r.GetRelayCostsByURL(ctx, relayURL, startTime, endTime, pageLimit, cursor, operationType)
		if err != nil {
			return nil, err
		}

		allCosts = append(allCosts, batch...)
		if nextCursor == "" || len(batch) == 0 {
			break
		}

		cursor = nextCursor
		remaining -= len(batch)
	}

	return allCosts, nil
}

// GetRelayCostsByDateRange retrieves relay costs for all relays within a date range
func (r *TrackingRepository) GetRelayCostsByDateRange(ctx context.Context, startDate, endDate time.Time, limit int) ([]*models.RelayCost, error) {
	var allCosts []*models.RelayCost

	safeTotal := limit
	if safeTotal <= 0 || safeTotal > relayCostDateCap {
		safeTotal = relayCostDateCap
	}

	// Query by daily partitions
	currentDate := startDate
	for currentDate.Before(endDate) || currentDate.Equal(endDate) {
		if len(allCosts) >= safeTotal {
			break
		}

		remaining := safeTotal - len(allCosts)
		dayLimit := relayCostPerDayCap
		if remaining < dayLimit {
			dayLimit = remaining
		}

		if dayLimit <= 0 {
			break
		}

		dateStr := currentDate.Format("20060102")

		var dailyCosts []*models.RelayCost
		query := r.db.WithContext(ctx).Model(&models.RelayCost{}).
			Index("GSI2").
			Where("gsI2PK", "=", fmt.Sprintf("RELAY_COSTS_DAILY#%s", dateStr)).
			OrderBy("GSI2SK", "DESC").
			Limit(dayLimit + 1)

		err := query.All(&dailyCosts)
		if err != nil {
			r.logger.Warn("failed to get relay costs for date",
				zap.String("date", dateStr),
				zap.Error(err))
			// Continue with next date
		} else {
			if len(dailyCosts) > dayLimit {
				dailyCosts = dailyCosts[:dayLimit]
			}
			allCosts = append(allCosts, dailyCosts...)
		}

		// Move to next day
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	// Sort by timestamp (newest first) and limit
	sort.Slice(allCosts, func(i, j int) bool {
		return allCosts[i].Timestamp.After(allCosts[j].Timestamp)
	})

	if len(allCosts) > safeTotal {
		allCosts = allCosts[:safeTotal]
	}

	return allCosts, nil
}

// CreateRelayMetrics creates or updates relay metrics
func (r *TrackingRepository) CreateRelayMetrics(ctx context.Context, metrics *models.RelayMetrics) error {
	// Call BeforeCreate to set up the model
	if err := metrics.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, "relay metrics", "validation")
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
func (r *TrackingRepository) UpdateRelayMetrics(ctx context.Context, metrics *models.RelayMetrics) error {
	// Call BeforeUpdate to set up the model
	if err := metrics.BeforeUpdate(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "relay metrics", "validation")
	}

	// Update the relay metrics record
	err := r.db.WithContext(ctx).Model(metrics).Update()
	if err != nil {
		return MapErrorWithContext(err, "failed to update relay metrics")
	}

	return nil
}

// GetRelayMetrics retrieves relay metrics for a specific relay and period
func (r *TrackingRepository) GetRelayMetrics(ctx context.Context, relayURL, period string, windowStart time.Time) (*models.RelayMetrics, error) {
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
func (r *TrackingRepository) GetRelayMetricsHistory(ctx context.Context, relayURL string, startTime, endTime time.Time, limit int, cursor string) ([]*models.RelayMetrics, string, error) {
	var metricsHistory []*models.RelayMetrics

	safeLimit := clampRelayMetricsLimit(limit)

	startSK := fmt.Sprintf("daily#%s", startTime.Format(common.CompactTimeFormat))
	endSK := fmt.Sprintf("daily#%s", endTime.Format(common.CompactTimeFormat))

	query := r.db.WithContext(ctx).Model(&models.RelayMetrics{}).
		Index("GSI1").
		Where("gsI1PK", "=", fmt.Sprintf("RELAY_METRICS#%s", relayURL)).
		Where("gsI1SK", ">=", startSK).
		Where("gsI1SK", "<=", endSK).
		OrderBy("GSI1SK", "DESC").
		Limit(safeLimit + 1)

	if cursor != "" {
		query = query.Where("gsI1SK", "<", cursor)
	}

	err := query.All(&metricsHistory)
	if err != nil {
		return nil, "", MapErrorWithContext(err, "failed to get relay metrics history")
	}

	var nextCursor string
	if len(metricsHistory) > safeLimit {
		nextCursor = metricsHistory[safeLimit-1].GSI1SK
		metricsHistory = metricsHistory[:safeLimit]
	}

	return metricsHistory, nextCursor, nil
}

// CreateRelayBudget creates a new relay budget configuration
func (r *TrackingRepository) CreateRelayBudget(ctx context.Context, budget *models.RelayBudget) error {
	// Call BeforeCreate to set up the model
	if err := budget.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, "relay budget", "validation")
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
func (r *TrackingRepository) UpdateRelayBudget(ctx context.Context, budget *models.RelayBudget) error {
	// Call BeforeUpdate to set up the model
	if err := budget.BeforeUpdate(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "relay budget", "validation")
	}

	// Update the relay budget record
	err := r.db.WithContext(ctx).Model(budget).Update()
	if err != nil {
		return MapErrorWithContext(err, "failed to update relay budget")
	}

	return nil
}

// GetRelayBudget retrieves relay budget configuration
func (r *TrackingRepository) GetRelayBudget(ctx context.Context, relayURL, period string) (*models.RelayBudget, error) {
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
func (r *TrackingRepository) AggregateRelayCosts(ctx context.Context, relayURL, period string, windowStart, windowEnd time.Time) error {
	// Get all relay costs in the window
	costs, err := r.collectRelayCosts(ctx, relayURL, windowStart, windowEnd, 10000, "")
	if err != nil {
		return ErrorHandler.HandleQueryError(err, "relay cost", "aggregation")
	}

	if err := common.ValidateSliceNotEmpty("costs", costs); err != nil {
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
	metrics.Domain = costs[0].Domain

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

// GetRelayCostSummary aggregates relay cost metrics between the provided timestamps.
func (r *TrackingRepository) GetRelayCostSummary(ctx context.Context, relayURL string, startTime, endTime time.Time) (*RelayCostSummary, error) {
	costs, err := r.collectRelayCosts(ctx, relayURL, startTime, endTime, 10000, "")
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
func (r *TrackingRepository) GetHighCostRelayOperations(ctx context.Context, thresholdMicroCents int64, startTime, endTime time.Time, limit int) ([]*models.RelayCost, error) {
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
func (r *TrackingRepository) GetImportExportCostsByUser(ctx context.Context, username string, startDate, endDate time.Time) (*ImportExportUserCostSummary, error) {
	// Get import costs
	importCosts, err := r.GetCostsByService(ctx, startDate, endDate)
	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, "import cost", "query")
	}

	// Get export costs
	exportCosts, err := r.GetCostsByService(ctx, startDate, endDate)
	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, "export cost", "query")
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
func (r *TrackingRepository) GetImportExportTrends(ctx context.Context, lookbackDays int) (*ImportExportTrends, error) {
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -lookbackDays)

	// Get daily aggregated data for import and export services
	importTrend, err := r.GetCostTrends(ctx, "daily", "ImportProcessing", lookbackDays)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "import trend", "query")
	}

	exportTrend, err := r.GetCostTrends(ctx, "daily", "ExportGeneration", lookbackDays)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "export trend", "query")
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

	if err := common.ValidateSliceNotEmpty("data_points", trends.CombinedTrend.DataPoints); err == nil {
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

	if err := common.ValidateSliceNotEmpty("data_points", trends.CombinedTrend.DataPoints); err == nil {
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
func (r *TrackingRepository) GetTopCostlyUsers(ctx context.Context, startDate, endDate time.Time, limit int) ([]*UserCostRanking, error) {
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
		if err := common.ValidateRequiredParam("username", username); err != nil {
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
func (r *TrackingRepository) GetImportExportMetrics(ctx context.Context, startDate, endDate time.Time) (*ImportExportMetrics, error) {
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
func (r *TrackingRepository) GetActivityCost(ctx context.Context, activityID string) (*models.DynamoDBCostRecord, error) {
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
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "activity cost", activityID)
		}
		r.logger.Error("Failed to query activity cost",
			zap.String("activity_id", activityID),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "activity cost", "query")
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

// Statistical helper functions

// mean calculates the arithmetic mean of a slice of float64 values
func mean(values []float64) float64 {
	if err := common.ValidateSliceNotEmpty("values", values); err != nil {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// standardDeviation calculates the standard deviation
func standardDeviation(values []float64, mean float64) float64 {
	if len(values) <= 1 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		diff := v - mean
		sum += diff * diff
	}
	return math.Sqrt(sum / float64(len(values)-1))
}

// approximateTTestPValue provides an approximate p-value for t-test
func (r *TrackingRepository) approximateTTestPValue(tStat float64, df int) float64 {
	// Simplified p-value approximation for t-distribution
	// This is a rough approximation - in production, use a proper statistical library
	absTStat := math.Abs(tStat)

	if df >= 30 {
		// Use normal approximation for large degrees of freedom
		if absTStat > 2.576 {
			return 0.01
		} else if absTStat > 1.96 {
			return 0.05
		} else if absTStat > 1.645 {
			return 0.10
		}
		return 0.20
	}

	// Conservative approximation for small samples
	if absTStat > 3.0 {
		return 0.01
	} else if absTStat > 2.0 {
		return 0.05
	} else if absTStat > 1.5 {
		return 0.10
	}
	return 0.20
}

// mannKendallTest performs Mann-Kendall trend test
func (r *TrackingRepository) mannKendallTest(values []float64) (tau, pValue float64) {
	n := len(values)
	if n < 3 {
		return 0, 1
	}

	// Calculate Mann-Kendall statistic
	s := 0
	for i := 0; i < n-1; i++ {
		for j := i + 1; j < n; j++ {
			if values[j] > values[i] {
				s++
			} else if values[j] < values[i] {
				s--
			}
		}
	}

	// Calculate tau
	tau = float64(s) / (float64(n) * float64(n-1) / 2.0)

	// Approximate p-value (simplified)
	varS := float64(n*(n-1)*(2*n+5)) / 18.0
	z := float64(s) / math.Sqrt(varS)

	// Rough p-value approximation
	absZ := math.Abs(z)
	if absZ > 2.576 {
		pValue = 0.01
	} else if absZ > 1.96 {
		pValue = 0.05
	} else if absZ > 1.645 {
		pValue = 0.10
	} else {
		pValue = 0.20
	}

	return tau, pValue
}

// theilSenSlope calculates Theil-Sen slope estimator
func (r *TrackingRepository) theilSenSlope(values []float64) float64 {
	n := len(values)
	if n < 2 {
		return 0
	}

	var slopes []float64
	for i := 0; i < n-1; i++ {
		for j := i + 1; j < n; j++ {
			if j != i {
				slope := (values[j] - values[i]) / float64(j-i)
				slopes = append(slopes, slope)
			}
		}
	}

	if err := common.ValidateSliceNotEmpty("slopes", slopes); err != nil {
		return 0
	}

	// Return median slope
	sort.Float64s(slopes)
	n = len(slopes)
	if n%2 == 0 {
		return (slopes[n/2-1] + slopes[n/2]) / 2.0
	}
	return slopes[n/2]
}

// durbinWatsonTest calculates Durbin-Watson test statistic
func (r *TrackingRepository) durbinWatsonTest(residuals []float64) float64 {
	n := len(residuals)
	if n < 2 {
		return 2.0 // No autocorrelation
	}

	numerator := 0.0
	denominator := 0.0

	for i := 1; i < n; i++ {
		diff := residuals[i] - residuals[i-1]
		numerator += diff * diff
	}

	for _, r := range residuals {
		denominator += r * r
	}

	if denominator == 0 {
		return 2.0
	}

	return numerator / denominator
}

// jarqueBeraTest performs Jarque-Bera test for normality
func (r *TrackingRepository) jarqueBeraTest(values []float64) (jb, pValue float64) {
	n := float64(len(values))
	if n < 4 {
		return 0, 1
	}

	// Calculate mean and standard deviation
	meanVal := mean(values)
	stdDev := standardDeviation(values, meanVal)

	if stdDev == 0 {
		return 0, 1
	}

	// Calculate skewness and kurtosis
	skewness := 0.0
	kurtosis := 0.0

	for _, v := range values {
		standardized := (v - meanVal) / stdDev
		skewness += standardized * standardized * standardized
		kurtosis += standardized * standardized * standardized * standardized
	}

	skewness /= n
	kurtosis = kurtosis/n - 3.0 // Excess kurtosis

	// Calculate Jarque-Bera statistic
	jb = (n / 6.0) * (skewness*skewness + kurtosis*kurtosis/4.0)

	// Approximate p-value (chi-squared with 2 degrees of freedom)
	if jb > 9.21 {
		pValue = 0.01
	} else if jb > 5.99 {
		pValue = 0.05
	} else if jb > 4.61 {
		pValue = 0.10
	} else {
		pValue = 0.20
	}

	return jb, pValue
}

// analyzeResiduals analyzes regression residuals
func (r *TrackingRepository) analyzeResiduals(residuals []float64) *ResidualStats {
	if len(residuals) < 3 {
		return nil
	}

	meanVal := mean(residuals)
	stdError := standardDeviation(residuals, meanVal)

	// Calculate skewness and kurtosis
	skewness := 0.0
	kurtosis := 0.0
	outlierCount := 0

	for _, r := range residuals {
		if stdError > 0 {
			standardized := (r - meanVal) / stdError
			skewness += standardized * standardized * standardized
			kurtosis += standardized * standardized * standardized * standardized

			// Count outliers (beyond 2 standard deviations)
			if math.Abs(standardized) > 2.0 {
				outlierCount++
			}
		}
	}

	n := float64(len(residuals))
	skewness /= n
	kurtosis = kurtosis/n - 3.0 // Excess kurtosis

	// Calculate normality score (0-1, higher = more normal)
	normalityScore := 1.0
	if math.Abs(skewness) > 0.5 {
		normalityScore -= 0.3
	}
	if math.Abs(kurtosis) > 1.0 {
		normalityScore -= 0.3
	}
	if float64(outlierCount)/n > 0.05 { // More than 5% outliers
		normalityScore -= 0.4
	}
	normalityScore = math.Max(0, normalityScore)

	return &ResidualStats{
		Mean:           meanVal,
		StandardError:  stdError,
		Skewness:       skewness,
		Kurtosis:       kurtosis,
		OutlierCount:   outlierCount,
		NormalityScore: normalityScore,
	}
}

// calculateSeasonalStrength calculates seasonal strength for a given period
func (r *TrackingRepository) calculateSeasonalStrength(values []float64, period int) float64 {
	n := len(values)
	if n < 2*period {
		return 0
	}

	// Calculate seasonal means
	seasonalSums := make([]float64, period)
	seasonalCounts := make([]int, period)

	for i, v := range values {
		seasonIndex := i % period
		seasonalSums[seasonIndex] += v
		seasonalCounts[seasonIndex]++
	}

	seasonalMeans := make([]float64, period)
	for i := 0; i < period; i++ {
		if seasonalCounts[i] > 0 {
			seasonalMeans[i] = seasonalSums[i] / float64(seasonalCounts[i])
		}
	}

	// Calculate overall mean
	overallMean := mean(values)

	// Calculate seasonal variance and total variance
	seasonalVariance := 0.0
	totalVariance := 0.0

	for i, v := range values {
		seasonIndex := i % period
		seasonalResidual := v - seasonalMeans[seasonIndex]
		totalResidual := v - overallMean

		seasonalVariance += seasonalResidual * seasonalResidual
		totalVariance += totalResidual * totalResidual
	}

	if totalVariance == 0 {
		return 0
	}

	// Seasonal strength = 1 - (seasonal variance / total variance)
	seasonalStrength := 1.0 - (seasonalVariance / totalVariance)
	return math.Max(0, seasonalStrength)
}

// simpleSeasonalDecomposition performs simple seasonal decomposition
func (r *TrackingRepository) simpleSeasonalDecomposition(values []float64, period int) (trend, seasonal, residual []float64) {
	n := len(values)
	trend = make([]float64, n)
	seasonal = make([]float64, n)
	residual = make([]float64, n)

	// Simple moving average for trend
	halfPeriod := period / 2
	for i := 0; i < n; i++ {
		start := math.Max(0, float64(i-halfPeriod))
		end := math.Min(float64(n-1), float64(i+halfPeriod))

		sum := 0.0
		count := 0
		for j := int(start); j <= int(end); j++ {
			sum += values[j]
			count++
		}
		trend[i] = sum / float64(count)
	}

	// Calculate seasonal component
	seasonalSums := make([]float64, period)
	seasonalCounts := make([]int, period)

	for i := 0; i < n; i++ {
		seasonIndex := i % period
		detrended := values[i] - trend[i]
		seasonalSums[seasonIndex] += detrended
		seasonalCounts[seasonIndex]++
	}

	seasonalMeans := make([]float64, period)
	for i := 0; i < period; i++ {
		if seasonalCounts[i] > 0 {
			seasonalMeans[i] = seasonalSums[i] / float64(seasonalCounts[i])
		}
	}

	// Assign seasonal components and calculate residuals
	for i := 0; i < n; i++ {
		seasonIndex := i % period
		seasonal[i] = seasonalMeans[seasonIndex]
		residual[i] = values[i] - trend[i] - seasonal[i]
	}

	return trend, seasonal, residual
}

// calculateDecompositionR2 calculates R-squared for seasonal decomposition
func (r *TrackingRepository) calculateDecompositionR2(original, trend, seasonal []float64) float64 {
	n := len(original)
	if n == 0 {
		return 0
	}

	originalMean := mean(original)
	ssTotal := 0.0
	ssResidual := 0.0

	for i := 0; i < n; i++ {
		predicted := trend[i] + seasonal[i]
		ssTotal += (original[i] - originalMean) * (original[i] - originalMean)
		ssResidual += (original[i] - predicted) * (original[i] - predicted)
	}

	if ssTotal == 0 {
		return 0
	}

	return 1.0 - (ssResidual / ssTotal)
}

// extractSeasonalPatterns extracts seasonal patterns from decomposition
func (r *TrackingRepository) extractSeasonalPatterns(seasonal []float64, period int) map[string]float64 {
	patterns := make(map[string]float64)

	seasonalSums := make([]float64, period)
	seasonalCounts := make([]int, period)

	for i, s := range seasonal {
		seasonIndex := i % period
		seasonalSums[seasonIndex] += s
		seasonalCounts[seasonIndex]++
	}

	for i := 0; i < period; i++ {
		if seasonalCounts[i] > 0 {
			key := fmt.Sprintf("period_%d", i)
			patterns[key] = seasonalSums[i] / float64(seasonalCounts[i])
		}
	}

	return patterns
}

// analyzeSeasonality analyzes seasonal patterns in cost data
func (r *TrackingRepository) analyzeSeasonality(dataPoints []CostDataPoint) *SeasonalityAnalysis {
	n := len(dataPoints)
	if n < 14 { // Need at least 2 weeks of data for seasonal analysis
		return nil
	}

	y := make([]float64, n)
	for i, point := range dataPoints {
		y[i] = point.CostDollars
	}

	// Simple seasonal decomposition
	// Test for weekly seasonality (period = 7)
	weeklyStrength := r.calculateSeasonalStrength(y, 7)

	// Test for monthly seasonality (period = 30)
	monthlyStrength := 0.0
	if n >= 60 {
		monthlyStrength = r.calculateSeasonalStrength(y, 30)
	}

	// Determine strongest seasonality
	hasSeasonality := false
	seasonalPeriod := 7
	seasonalStrength := weeklyStrength

	if monthlyStrength > weeklyStrength {
		seasonalPeriod = 30
		seasonalStrength = monthlyStrength
	}

	hasSeasonality = seasonalStrength > 0.3 // Threshold for significant seasonality

	// Perform decomposition if seasonality is detected
	var trendComponent, seasonalComponent, residualComponent []float64
	var decompositionR2 float64
	var seasonalPatterns map[string]float64

	if hasSeasonality {
		trendComponent, seasonalComponent, residualComponent = r.simpleSeasonalDecomposition(y, seasonalPeriod)
		decompositionR2 = r.calculateDecompositionR2(y, trendComponent, seasonalComponent)
		seasonalPatterns = r.extractSeasonalPatterns(seasonalComponent, seasonalPeriod)
	}

	return &SeasonalityAnalysis{
		HasSeasonality:    hasSeasonality,
		SeasonalStrength:  seasonalStrength,
		SeasonalPeriod:    seasonalPeriod,
		SeasonalPatterns:  seasonalPatterns,
		TrendComponent:    trendComponent,
		SeasonalComponent: seasonalComponent,
		ResidualComponent: residualComponent,
		DecompositionR2:   decompositionR2,
	}
}

// calculateLinearRegressionStats calculates comprehensive linear regression statistics
func (r *TrackingRepository) calculateLinearRegressionStats(dataPoints []CostDataPoint) *LinearRegressionStats {
	n := len(dataPoints)
	if n < 2 {
		return nil
	}

	// Prepare data for regression
	x := make([]float64, n)
	y := make([]float64, n)

	for i, point := range dataPoints {
		x[i] = float64(i) // Time index
		y[i] = point.CostDollars
	}

	// Calculate means
	xMean := mean(x)
	yMean := mean(y)

	// Calculate slope and intercept
	numerator := 0.0
	denominator := 0.0

	for i := 0; i < n; i++ {
		xDiff := x[i] - xMean
		yDiff := y[i] - yMean
		numerator += xDiff * yDiff
		denominator += xDiff * xDiff
	}

	if denominator == 0 {
		return &LinearRegressionStats{
			TrendDirection: "stable",
			TrendStrength:  "none",
		}
	}

	slope := numerator / denominator
	intercept := yMean - slope*xMean

	// Calculate R-squared and correlation
	ssRes := 0.0 // Sum of squares of residuals
	ssTot := 0.0 // Total sum of squares

	for i := 0; i < n; i++ {
		predicted := slope*x[i] + intercept
		residual := y[i] - predicted
		ssRes += residual * residual
		ssTot += (y[i] - yMean) * (y[i] - yMean)
	}

	rSquared := 0.0
	if ssTot > 0 {
		rSquared = 1.0 - (ssRes / ssTot)
	}

	correlation := math.Sqrt(math.Abs(rSquared))
	if slope < 0 {
		correlation = -correlation
	}

	// Calculate standard error and t-statistic
	slopeStdError := 0.0
	tStatistic := 0.0
	pValue := 1.0

	if n > 2 {
		mse := ssRes / float64(n-2) // Mean squared error
		slopeStdError = math.Sqrt(mse / denominator)

		if slopeStdError > 0 {
			tStatistic = slope / slopeStdError
			// Approximate p-value using t-distribution (simplified)
			pValue = r.approximateTTestPValue(tStatistic, n-2)
		}
	}

	// Determine trend direction and strength
	trendDirection := "stable"
	trendStrength := RepliesPolicyNone
	isSignificant := pValue < 0.05

	if math.Abs(slope) > 0.001 { // Threshold for non-zero slope
		if slope > 0 {
			trendDirection = "increasing"
		} else {
			trendDirection = "decreasing"
		}

		// Classify strength based on R-squared and significance
		if rSquared > 0.7 && isSignificant {
			trendStrength = "strong"
		} else if rSquared > 0.3 && isSignificant {
			trendStrength = "moderate"
		} else if rSquared > 0.1 {
			trendStrength = "weak"
		}
	}

	return &LinearRegressionStats{
		Slope:           slope,
		Intercept:       intercept,
		RSquared:        rSquared,
		Correlation:     correlation,
		SlopeStdError:   slopeStdError,
		TStatistic:      tStatistic,
		PValue:          pValue,
		ConfidenceLevel: 0.95,
		TrendDirection:  trendDirection,
		TrendStrength:   trendStrength,
		IsSignificant:   isSignificant,
	}
}

// calculateStatisticalTests performs additional statistical tests
func (r *TrackingRepository) calculateStatisticalTests(dataPoints []CostDataPoint, regression *LinearRegressionStats) *StatisticalTests {
	if regression == nil || len(dataPoints) < 3 {
		return nil
	}

	n := len(dataPoints)
	y := make([]float64, n)
	residuals := make([]float64, n)

	for i, point := range dataPoints {
		y[i] = point.CostDollars
		predicted := regression.Slope*float64(i) + regression.Intercept
		residuals[i] = y[i] - predicted
	}

	// Mann-Kendall trend test (non-parametric)
	mkTau, mkP := r.mannKendallTest(y)

	// Theil-Sen slope estimator (robust)
	theilSenSlope := r.theilSenSlope(y)

	// Durbin-Watson test for autocorrelation
	durbinWatson := r.durbinWatsonTest(residuals)

	// Jarque-Bera test for normality of residuals
	jarqueBera, jarqueBeraP := r.jarqueBeraTest(residuals)

	// Residual analysis
	residualStats := r.analyzeResiduals(residuals)

	return &StatisticalTests{
		MannKendallTau:    mkTau,
		MannKendallPValue: mkP,
		TheilSenSlope:     theilSenSlope,
		DurbinWatson:      durbinWatson,
		JarqueBera:        jarqueBera,
		JarqueBeraP:       jarqueBeraP,
		ResidualStats:     residualStats,
	}
}

// detectCostAnomalies detects anomalies in cost data
func (r *TrackingRepository) detectCostAnomalies(dataPoints []CostDataPoint, regression *LinearRegressionStats) []CostAnomaly {
	if regression == nil || len(dataPoints) < 5 {
		return nil
	}

	var anomalies []CostAnomaly
	n := len(dataPoints)
	residuals := make([]float64, n)

	// Calculate residuals
	for i, point := range dataPoints {
		predicted := regression.Slope*float64(i) + regression.Intercept
		residuals[i] = point.CostDollars - predicted
	}

	// Calculate threshold for anomaly detection (using IQR method)
	residualsCopy := make([]float64, len(residuals))
	copy(residualsCopy, residuals)
	sort.Float64s(residualsCopy)

	q1 := residualsCopy[n/4]
	q3 := residualsCopy[3*n/4]
	iqr := q3 - q1
	lowerThreshold := q1 - 1.5*iqr
	upperThreshold := q3 + 1.5*iqr

	// Calculate standard deviation for z-score
	residualMean := mean(residuals)
	residualStdDev := standardDeviation(residuals, residualMean)

	// Detect anomalies
	for i, point := range dataPoints {
		expected := regression.Slope*float64(i) + regression.Intercept
		residual := residuals[i]

		// Check if it is an outlier
		isOutlier := residual < lowerThreshold || residual > upperThreshold

		if isOutlier {
			// Calculate deviation score (z-score)
			deviationScore := 0.0
			if residualStdDev > 0 {
				deviationScore = math.Abs(residual-residualMean) / residualStdDev
			}

			// Determine anomaly type
			anomalyType := "outlier"
			if residual > upperThreshold {
				anomalyType = "spike"
			} else if residual < lowerThreshold {
				anomalyType = "drop"
			}

			// Determine severity
			severity := StatusLow
			if deviationScore > 3.0 {
				severity = StatusCritical
			} else if deviationScore > 2.5 {
				severity = StatusHigh
			} else if deviationScore > 2.0 {
				severity = StatusMedium
			}

			// Calculate confidence based on deviation
			confidence := math.Min(deviationScore/3.0, 1.0)

			anomalies = append(anomalies, CostAnomaly{
				Timestamp:      point.Timestamp,
				ActualCost:     point.CostDollars,
				ExpectedCost:   expected,
				DeviationScore: deviationScore,
				AnomalyType:    anomalyType,
				Severity:       severity,
				Confidence:     confidence,
			})
		}
	}

	return anomalies
}

// generateCostForecast generates cost forecasts based on trend analysis
func (r *TrackingRepository) generateCostForecast(dataPoints []CostDataPoint, regression *LinearRegressionStats) *CostForecast {
	if regression == nil || len(dataPoints) < 3 {
		return nil
	}

	// Forecast parameters
	forecastHorizon := 7 // Forecast 7 periods ahead
	confidenceLevel := 0.95
	n := len(dataPoints)

	// Calculate prediction intervals
	residuals := make([]float64, n)
	for i, point := range dataPoints {
		predicted := regression.Slope*float64(i) + regression.Intercept
		residuals[i] = point.CostDollars - predicted
	}

	residualMean := mean(residuals)
	residualStdDev := standardDeviation(residuals, residualMean)

	// T-value for confidence interval (approximation)
	tValue := 1.96 // For 95% confidence, approximate
	if n < 30 {
		tValue = 2.0 // More conservative for small samples
	}

	// Generate predictions
	var predictions []CostPrediction
	lastTimestamp := dataPoints[n-1].Timestamp

	for i := 1; i <= forecastHorizon; i++ {
		// Predict next cost value
		xNext := float64(n + i - 1)
		predictedCost := regression.Slope*xNext + regression.Intercept

		// Calculate prediction interval
		// Standard error increases with distance from mean
		xMean := float64(n-1) / 2.0
		xDist := xNext - xMean
		predictionError := residualStdDev * math.Sqrt(1.0+1.0/float64(n)+xDist*xDist/float64(n))

		// Calculate bounds
		marginOfError := tValue * predictionError
		lowerBound := math.Max(0, predictedCost-marginOfError) // Cost cannot be negative
		upperBound := predictedCost + marginOfError

		// Determine next timestamp (assume daily data)
		nextTimestamp := lastTimestamp.Add(time.Duration(i) * 24 * time.Hour)

		predictions = append(predictions, CostPrediction{
			Timestamp:     nextTimestamp,
			PredictedCost: predictedCost,
			LowerBound:    lowerBound,
			UpperBound:    upperBound,
			StandardError: predictionError,
		})
	}

	// Calculate model accuracy metrics
	meanAbsoluteError := 0.0
	rootMeanSquareError := 0.0

	for _, residual := range residuals {
		meanAbsoluteError += math.Abs(residual)
		rootMeanSquareError += residual * residual
	}

	meanAbsoluteError /= float64(n)
	rootMeanSquareError = math.Sqrt(rootMeanSquareError / float64(n))

	return &CostForecast{
		ForecastHorizon:     forecastHorizon,
		Predictions:         predictions,
		ConfidenceLevel:     confidenceLevel,
		MeanAbsoluteError:   meanAbsoluteError,
		RootMeanSquareError: rootMeanSquareError,
		ModelType:           "linear",
	}
}
