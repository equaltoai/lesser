package repositories

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// AICostRepository implements AI cost tracking operations using enhanced patterns
type AICostRepository struct {
	*EnhancedBaseRepository[*models.AICost]
}

// NewAICostRepository creates a new AI cost repository with enhanced functionality
func NewAICostRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *AICostRepository {
	// Create enhanced repository optimized for AI cost operations
	enhancedRepo := NewEnhancedBaseRepository[*models.AICost](db, tableName, logger, costService, "AICostRepository", "aicost")

	// Set up enhanced services for AI cost operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // AI cost data cached for analytics
	enhancedRepo.SetEventService(NewDefaultEventService())      // Critical for cost monitoring

	return &AICostRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreateAICost creates a new AI cost record
func (r *AICostRepository) CreateAICost(ctx context.Context, aiCost *models.AICost) error {
	// Set model pricing if not already set
	if aiCost.InputTokenCost == 0 && aiCost.OutputTokenCost == 0 {
		aiCost.SetModelPricing()
	}

	err := r.db.WithContext(ctx).Model(aiCost).Create()
	if err != nil {
		r.logger.Error("Failed to create AI cost record",
			zap.String("operation_id", aiCost.OperationID),
			zap.String("operation_type", aiCost.OperationType),
			zap.String("model", aiCost.ModelName),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "ai cost", aiCost.OperationID)
	}

	r.logger.Info("Created AI cost record",
		zap.String("operation_id", aiCost.OperationID),
		zap.String("operation_type", aiCost.OperationType),
		zap.String("model", aiCost.ModelName),
		zap.Int64("input_tokens", aiCost.InputTokens),
		zap.Int64("output_tokens", aiCost.OutputTokens),
		zap.Float64("cost_dollars", aiCost.GetTotalCostDollars()),
		zap.String("cost_tier", aiCost.CostTier))

	return nil
}

// GetAICost retrieves an AI cost record by operation ID
func (r *AICostRepository) GetAICost(ctx context.Context, operationID string) (*models.AICost, error) {
	aiCost := &models.AICost{}
	pk := fmt.Sprintf("AI_COST#%s", operationID)
	sk := models.SKMetadata

	err := r.Get(ctx, pk, sk, aiCost)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(err, "ai cost", operationID)
		}
		r.logger.Error("Failed to get AI cost record",
			zap.String("operation_id", operationID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "ai cost", operationID)
	}

	return aiCost, nil
}

// GetAICostsByTimeRange retrieves AI cost records within a time range
func (r *AICostRepository) GetAICostsByTimeRange(ctx context.Context, startTime, endTime time.Time, operationType string, limit int) ([]*models.AICost, error) {
	startDate := startTime.Format(common.CompactDateFormat)
	endDate := endTime.Format(common.CompactDateFormat)

	query := r.db.WithContext(ctx).Model(&models.AICost{}).
		Index("gsi1").
		Where("gsi1PK", ">=", fmt.Sprintf("AI_COSTS#%s", startDate)).
		Where("gsi1PK", "<=", fmt.Sprintf("AI_COSTS#%s", endDate))

	if limit > 0 {
		query = query.Limit(limit)
	}

	var aiCosts []models.AICost
	err := query.Scan(&aiCosts)
	if err != nil {
		r.logger.Error("Failed to query AI costs by time range",
			zap.Time("start", startTime),
			zap.Time("end", endTime),
			zap.String("operation_type", operationType),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "ai cost", "time range")
	}

	// Filter by operation type if specified and within time range
	var filteredCosts []*models.AICost
	for i, cost := range aiCosts {
		if cost.Timestamp.Before(startTime) || cost.Timestamp.After(endTime) {
			continue
		}
		if operationType != "" && cost.OperationType != operationType {
			continue
		}
		filteredCosts = append(filteredCosts, &aiCosts[i])
	}

	r.logger.Debug("Retrieved AI costs by time range",
		zap.Int("total_found", len(filteredCosts)),
		zap.Time("start", startTime),
		zap.Time("end", endTime),
		zap.String("operation_type", operationType))

	return filteredCosts, nil
}

// GetAICostsByOperationType retrieves AI cost records by operation type
func (r *AICostRepository) GetAICostsByOperationType(ctx context.Context, operationType string, startTime time.Time, limit int) ([]*models.AICost, error) {
	query := r.db.WithContext(ctx).Model(&models.AICost{}).
		Index("gsi2").
		Where("gsi2PK", "=", fmt.Sprintf("AI_TYPE#%s", operationType))

	if !startTime.IsZero() {
		query = query.Where("gsi2SK", ">=", fmt.Sprintf("MODEL#%s", startTime.Format(common.CompactTimeFormat)))
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	var aiCosts []models.AICost
	err := query.Scan(&aiCosts)
	if err != nil {
		r.logger.Error("Failed to query AI costs by operation type",
			zap.String("operation_type", operationType),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "ai cost", "operation type")
	}

	// Convert to pointers
	result := make([]*models.AICost, len(aiCosts))
	for i := range aiCosts {
		result[i] = &aiCosts[i]
	}

	return result, nil
}

// GetTopCostlyOperations retrieves the most expensive AI operations
func (r *AICostRepository) GetTopCostlyOperations(ctx context.Context, costTier string, startTime, endTime time.Time, limit int) ([]*models.AICost, error) {
	if err := common.ValidateRequiredParam("cost_tier", costTier); err != nil {
		costTier = models.CostTierHigh // Default to high-cost operations
	}

	query := r.db.WithContext(ctx).Model(&models.AICost{}).
		Index("gsi3").
		Where("gsi3PK", "=", fmt.Sprintf("AI_COST_RANGE#%s", costTier))

	if limit > 0 {
		query = query.Limit(limit)
	}

	var aiCosts []models.AICost
	err := query.Scan(&aiCosts)
	if err != nil {
		r.logger.Error("Failed to query top costly AI operations",
			zap.String("cost_tier", costTier),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "ai cost", "top costly operations")
	}

	// Filter by time range and sort by cost descending
	var filteredCosts []*models.AICost
	for i, cost := range aiCosts {
		if cost.Timestamp.Before(startTime) || cost.Timestamp.After(endTime) {
			continue
		}
		filteredCosts = append(filteredCosts, &aiCosts[i])
	}

	// Sort by cost descending
	sort.Slice(filteredCosts, func(i, j int) bool {
		return filteredCosts[i].TotalCostMicroCents > filteredCosts[j].TotalCostMicroCents
	})

	// Apply limit after filtering
	if limit > 0 && len(filteredCosts) > limit {
		filteredCosts = filteredCosts[:limit]
	}

	return filteredCosts, nil
}

// GetAICostSummary retrieves aggregated AI cost metrics for a time period
func (r *AICostRepository) GetAICostSummary(ctx context.Context, startTime, endTime time.Time, operationType string) (*AICostSummary, error) {
	costs, err := r.GetAICostsByTimeRange(ctx, startTime, endTime, operationType, 0)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "ai cost", "summary")
	}

	summary := &AICostSummary{
		StartTime:     startTime,
		EndTime:       endTime,
		OperationType: operationType,
	}

	if err := common.ValidateSliceNotEmpty("costs", costs); err != nil {
		return summary, nil
	}

	// Calculate aggregated metrics
	var totalCost, totalInputTokens, totalOutputTokens int64
	var totalLatency, totalComplexity float64
	var successCount int
	modelCounts := make(map[string]int)
	operationCounts := make(map[string]int)

	for _, cost := range costs {
		totalCost += cost.TotalCostMicroCents
		totalInputTokens += cost.InputTokens
		totalOutputTokens += cost.OutputTokens
		totalLatency += float64(cost.RequestLatencyMs)
		totalComplexity += cost.ComplexityScore

		if cost.Success {
			successCount++
		}

		modelCounts[cost.ModelName]++
		operationCounts[cost.OperationType]++
	}

	summary.TotalOperations = int64(len(costs))
	summary.SuccessfulOperations = int64(successCount)
	summary.TotalCostMicroCents = totalCost
	summary.TotalInputTokens = totalInputTokens
	summary.TotalOutputTokens = totalOutputTokens

	// Calculate averages
	if err := common.ValidateSliceNotEmpty("costs", costs); err == nil {
		summary.AvgCostMicroCents = totalCost / int64(len(costs))
		summary.AvgLatencyMs = totalLatency / float64(len(costs))
		summary.AvgComplexityScore = totalComplexity / float64(len(costs))
		summary.SuccessRate = float64(successCount) / float64(len(costs)) * 100.0
	}

	// Calculate cost efficiency
	if totalInputTokens > 0 {
		summary.CostPerInputToken = float64(totalCost) / float64(totalInputTokens) / 1_000_000.0
	}
	if totalOutputTokens > 0 {
		summary.CostPerOutputToken = float64(totalCost) / float64(totalOutputTokens) / 1_000_000.0
	}

	summary.TotalCostDollars = float64(totalCost) / 1_000_000.0
	summary.ModelBreakdown = modelCounts
	summary.OperationBreakdown = operationCounts

	return summary, nil
}

// GetAICostTrends retrieves cost trends over time with sophisticated analysis
func (r *AICostRepository) GetAICostTrends(ctx context.Context, startTime, endTime time.Time, period string) (*AICostTrends, error) {
	// Get all costs in the time range
	costs, err := r.GetAICostsByTimeRange(ctx, startTime, endTime, "", 0)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "ai cost", "trends")
	}

	trends := &AICostTrends{
		Period:     period,
		StartTime:  startTime,
		EndTime:    endTime,
		DataPoints: []AICostDataPoint{},
	}

	if err := common.ValidateSliceNotEmpty("costs", costs); err != nil {
		return trends, nil
	}

	// Group costs by time periods
	var interval time.Duration
	switch period {
	case models.PeriodHour:
		interval = time.Hour
	case models.PeriodDay:
		interval = 24 * time.Hour
	case models.PeriodWeek:
		interval = 7 * 24 * time.Hour
	default:
		interval = 24 * time.Hour
	}

	// Create time buckets
	buckets := make(map[string][]models.AICost)
	current := startTime
	for current.Before(endTime) {
		bucketKey := current.Truncate(interval).Format(time.RFC3339)
		buckets[bucketKey] = []models.AICost{}
		current = current.Add(interval)
	}

	// Assign costs to buckets
	for _, cost := range costs {
		bucketKey := cost.Timestamp.Truncate(interval).Format(time.RFC3339)
		if bucket, exists := buckets[bucketKey]; exists {
			buckets[bucketKey] = append(bucket, *cost)
		}
	}

	// Create data points from buckets
	var bucketTimes []time.Time
	for timeStr := range buckets {
		if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
			bucketTimes = append(bucketTimes, t)
		}
	}
	sort.Slice(bucketTimes, func(i, j int) bool {
		return bucketTimes[i].Before(bucketTimes[j])
	})

	for _, bucketTime := range bucketTimes {
		bucketKey := bucketTime.Format(time.RFC3339)
		bucketCosts := buckets[bucketKey]

		dataPoint := AICostDataPoint{
			Timestamp:    bucketTime,
			TotalCost:    0,
			Operations:   int64(len(bucketCosts)),
			InputTokens:  0,
			OutputTokens: 0,
			AvgLatencyMs: 0,
			SuccessRate:  0,
		}

		if err := common.ValidateSliceNotEmpty("bucket_costs", bucketCosts); err == nil {
			var totalCost int64
			var totalLatency float64
			var successCount int

			for _, cost := range bucketCosts {
				totalCost += cost.TotalCostMicroCents
				dataPoint.InputTokens += cost.InputTokens
				dataPoint.OutputTokens += cost.OutputTokens
				totalLatency += float64(cost.RequestLatencyMs)
				if cost.Success {
					successCount++
				}
			}

			dataPoint.TotalCost = float64(totalCost) / 1_000_000.0
			dataPoint.AvgLatencyMs = totalLatency / float64(len(bucketCosts))
			dataPoint.SuccessRate = float64(successCount) / float64(len(bucketCosts)) * 100.0
		}

		trends.DataPoints = append(trends.DataPoints, dataPoint)
	}

	// Analyze trends
	trends.Analysis = r.analyzeAICostTrends(trends.DataPoints)

	return trends, nil
}

// analyzeAICostTrends performs sophisticated trend analysis
func (r *AICostRepository) analyzeAICostTrends(dataPoints []AICostDataPoint) *AICostTrendAnalysis {
	analysis := &AICostTrendAnalysis{
		TrendDirection: "stable",
		GrowthRate:     0,
	}

	if len(dataPoints) < 2 {
		return analysis
	}

	// Calculate linear regression for trend analysis
	n := float64(len(dataPoints))
	var sumX, sumY, sumXY, sumX2 float64

	for i, point := range dataPoints {
		x := float64(i)
		y := point.TotalCost
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// Linear regression slope (trend direction)
	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)

	if slope > 0.01 { // Threshold for "increasing"
		analysis.TrendDirection = "increasing"
	} else if slope < -0.01 { // Threshold for "decreasing"
		analysis.TrendDirection = "decreasing"
	}

	// Calculate growth rate (compound growth from first to last)
	if len(dataPoints) >= 2 {
		firstCost := dataPoints[0].TotalCost
		lastCost := dataPoints[len(dataPoints)-1].TotalCost
		periods := float64(len(dataPoints) - 1)

		if firstCost > 0 && periods > 0 {
			analysis.GrowthRate = (lastCost - firstCost) / firstCost / periods * 100
		}
	}

	// Find peak and low points
	var maxCost, minCost float64
	var maxTime, minTime time.Time

	for i, point := range dataPoints {
		if i == 0 || point.TotalCost > maxCost {
			maxCost = point.TotalCost
			maxTime = point.Timestamp
		}
		if i == 0 || point.TotalCost < minCost {
			minCost = point.TotalCost
			minTime = point.Timestamp
		}
	}

	analysis.PeakTime = maxTime
	analysis.LowTime = minTime
	analysis.PeakCost = maxCost
	analysis.LowCost = minCost

	// Calculate volatility (standard deviation of costs)
	if len(dataPoints) > 1 {
		avgCost := sumY / n
		var variance float64
		for _, point := range dataPoints {
			diff := point.TotalCost - avgCost
			variance += diff * diff
		}
		variance /= n
		analysis.Volatility = variance // Standard deviation squared for simplicity
	}

	// Detect seasonal patterns (simplified)
	analysis.SeasonalFactors = r.detectSeasonalPatterns(dataPoints)

	// Calculate statistical confidence
	if len(dataPoints) >= 3 {
		// Simple confidence based on R-squared
		var totalSumSquares, residualSumSquares float64
		avgY := sumY / n

		for i, point := range dataPoints {
			x := float64(i)
			predicted := slope*x + (sumY-slope*sumX)/n
			totalSumSquares += (point.TotalCost - avgY) * (point.TotalCost - avgY)
			residualSumSquares += (point.TotalCost - predicted) * (point.TotalCost - predicted)
		}

		if totalSumSquares > 0 {
			rSquared := 1 - (residualSumSquares / totalSumSquares)
			analysis.Confidence = rSquared * 100 // Convert to percentage
		}
	}

	return analysis
}

// timePatternData holds pattern analysis data
type timePatternData struct {
	averages   map[interface{}]float64
	overallAvg float64
}

// detectSeasonalPatterns detects seasonal patterns in cost data
func (r *AICostRepository) detectSeasonalPatterns(dataPoints []AICostDataPoint) []string {
	if len(dataPoints) < 7 {
		return []string{}
	}

	patterns := []string{}

	// Detect weekly patterns
	weeklyPatterns := r.detectWeeklyPatterns(dataPoints)
	patterns = append(patterns, weeklyPatterns...)

	// Detect hourly patterns
	hourlyPatterns := r.detectHourlyPatterns(dataPoints)
	patterns = append(patterns, hourlyPatterns...)

	return patterns
}

// detectWeeklyPatterns detects patterns based on day of week
func (r *AICostRepository) detectWeeklyPatterns(dataPoints []AICostDataPoint) []string {
	costsByDay := r.groupCostsByDay(dataPoints)
	patternData := r.calculateDayAverages(costsByDay)
	return r.identifyDayPatterns(patternData)
}

// detectHourlyPatterns detects patterns based on hour of day
func (r *AICostRepository) detectHourlyPatterns(dataPoints []AICostDataPoint) []string {
	costsByHour := r.groupCostsByHour(dataPoints)

	if len(costsByHour) <= 12 {
		return []string{} // Not enough hourly data
	}

	patternData := r.calculateHourAverages(costsByHour)
	overallAvg := r.calculateOverallAverage(dataPoints)
	return r.identifyHourPatterns(patternData, overallAvg)
}

// groupCostsByDay groups costs by day of week
func (r *AICostRepository) groupCostsByDay(dataPoints []AICostDataPoint) map[time.Weekday][]float64 {
	costsByDay := make(map[time.Weekday][]float64)
	for _, point := range dataPoints {
		dow := point.Timestamp.Weekday()
		costsByDay[dow] = append(costsByDay[dow], point.TotalCost)
	}
	return costsByDay
}

// groupCostsByHour groups costs by hour of day
func (r *AICostRepository) groupCostsByHour(dataPoints []AICostDataPoint) map[int][]float64 {
	costsByHour := make(map[int][]float64)
	for _, point := range dataPoints {
		hour := point.Timestamp.Hour()
		costsByHour[hour] = append(costsByHour[hour], point.TotalCost)
	}
	return costsByHour
}

// calculateDayAverages calculates averages and overall average for day-based data
func (r *AICostRepository) calculateDayAverages(costsByDay map[time.Weekday][]float64) *timePatternData {
	dayAverages := make(map[interface{}]float64)
	var totalSum float64
	var totalPoints int

	for dow, costs := range costsByDay {
		if err := common.ValidateSliceNotEmpty("costs", costs); err != nil {
			continue
		}

		sum := r.sumCosts(costs)
		dayAverages[dow] = sum / float64(len(costs))
		totalSum += sum
		totalPoints += len(costs)
	}

	overallAvg := float64(0)
	if totalPoints > 0 {
		overallAvg = totalSum / float64(totalPoints)
	}

	return &timePatternData{
		averages:   dayAverages,
		overallAvg: overallAvg,
	}
}

// calculateHourAverages calculates averages for hour-based data
func (r *AICostRepository) calculateHourAverages(costsByHour map[int][]float64) map[interface{}]float64 {
	hourAverages := make(map[interface{}]float64)
	for hour, costs := range costsByHour {
		if err := common.ValidateSliceNotEmpty("costs", costs); err == nil {
			sum := r.sumCosts(costs)
			hourAverages[hour] = sum / float64(len(costs))
		}
	}
	return hourAverages
}

// calculateOverallAverage calculates the overall average cost
func (r *AICostRepository) calculateOverallAverage(dataPoints []AICostDataPoint) float64 {
	if err := common.ValidateSliceNotEmpty("data_points", dataPoints); err != nil {
		return 0
	}

	var totalSum float64
	for _, point := range dataPoints {
		totalSum += point.TotalCost
	}

	return totalSum / float64(len(dataPoints))
}

// identifyDayPatterns identifies high/low day patterns
func (r *AICostRepository) identifyDayPatterns(patternData *timePatternData) []string {
	patterns := []string{}

	for day, avg := range patternData.averages {
		dow := day.(time.Weekday)
		if avg > patternData.overallAvg*1.2 {
			patterns = append(patterns, fmt.Sprintf("high_%s", dow.String()))
		} else if avg < patternData.overallAvg*0.8 {
			patterns = append(patterns, fmt.Sprintf("low_%s", dow.String()))
		}
	}

	return patterns
}

// identifyHourPatterns identifies peak/low hour patterns
func (r *AICostRepository) identifyHourPatterns(hourAverages map[interface{}]float64, overallAvg float64) []string {
	peakHour, peakAvg := r.findPeakHour(hourAverages)
	lowHour, lowAvg := r.findLowHour(hourAverages)

	patterns := []string{}

	if peakAvg > overallAvg*1.3 {
		patterns = append(patterns, fmt.Sprintf("peak_hour_%02d", peakHour))
	}

	if lowAvg < overallAvg*0.7 {
		patterns = append(patterns, fmt.Sprintf("low_hour_%02d", lowHour))
	}

	return patterns
}

// findPeakHour finds the hour with highest average cost
func (r *AICostRepository) findPeakHour(hourAverages map[interface{}]float64) (int, float64) {
	var peakHour int
	var peakAvg float64

	for hour, avg := range hourAverages {
		h := hour.(int)
		if h == 0 || avg > peakAvg {
			peakAvg = avg
			peakHour = h
		}
	}

	return peakHour, peakAvg
}

// findLowHour finds the hour with lowest average cost
func (r *AICostRepository) findLowHour(hourAverages map[interface{}]float64) (int, float64) {
	var lowHour int
	var lowAvg float64

	for hour, avg := range hourAverages {
		h := hour.(int)
		if h == 0 || avg < lowAvg {
			lowAvg = avg
			lowHour = h
		}
	}

	return lowHour, lowAvg
}

// sumCosts calculates the sum of costs in a slice
func (r *AICostRepository) sumCosts(costs []float64) float64 {
	sum := float64(0)
	for _, cost := range costs {
		sum += cost
	}
	return sum
}

// CreateOrUpdateAggregatedCost creates or updates aggregated cost records
func (r *AICostRepository) CreateOrUpdateAggregatedCost(ctx context.Context, aggregatedCost *models.AIAggregatedCost) error {
	// Note: This uses the AIAggregatedCost model directly since BaseRepository is typed for AICost
	err := r.db.WithContext(ctx).Model(aggregatedCost).Create()
	if err != nil {
		r.logger.Error("Failed to create/update aggregated AI cost",
			zap.String("period", aggregatedCost.Period),
			zap.String("operation_type", aggregatedCost.OperationType),
			zap.String("model", aggregatedCost.ModelName),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "ai cost aggregation", aggregatedCost.Period)
	}

	r.logger.Debug("Created/updated aggregated AI cost",
		zap.String("period", aggregatedCost.Period),
		zap.String("operation_type", aggregatedCost.OperationType),
		zap.String("model", aggregatedCost.ModelName),
		zap.Int64("total_operations", aggregatedCost.TotalOperations),
		zap.Float64("total_cost_dollars", aggregatedCost.TotalCostDollars))

	return nil
}

// GetAggregatedCosts retrieves aggregated cost data for analysis
func (r *AICostRepository) GetAggregatedCosts(ctx context.Context, period string, startTime, endTime time.Time) ([]*models.AIAggregatedCost, error) {
	// Note: This uses the AIAggregatedCost model directly since BaseRepository is typed for AICost
	query := r.db.WithContext(ctx).Model(&models.AIAggregatedCost{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("AI_AGG_TIME#%s", period))

	// Add time range filter
	if !startTime.IsZero() {
		startStr := startTime.Format(common.CompactDateFormat)
		if period == models.PeriodTimeHour {
			startStr = startTime.Format(common.CompactTimeFormat)[:13]
		}
		query = query.Where("gsi1SK", ">=", startStr)
	}

	if !endTime.IsZero() {
		endStr := endTime.Format(common.CompactDateFormat)
		if period == models.PeriodTimeHour {
			endStr = endTime.Format(common.CompactTimeFormat)[:13]
		}
		query = query.Where("gsi1SK", "<=", endStr)
	}

	var aggregatedCosts []models.AIAggregatedCost
	err := query.Scan(&aggregatedCosts)
	if err != nil {
		r.logger.Error("Failed to query aggregated AI costs",
			zap.String("period", period),
			zap.Time("start", startTime),
			zap.Time("end", endTime),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "ai cost aggregation", "period query")
	}

	// Convert to pointers
	result := make([]*models.AIAggregatedCost, len(aggregatedCosts))
	for i := range aggregatedCosts {
		result[i] = &aggregatedCosts[i]
	}

	return result, nil
}

// AICostSummary represents aggregated AI cost metrics
type AICostSummary struct {
	StartTime            time.Time      `json:"start_time"`
	EndTime              time.Time      `json:"end_time"`
	OperationType        string         `json:"operation_type"`
	TotalOperations      int64          `json:"total_operations"`
	SuccessfulOperations int64          `json:"successful_operations"`
	SuccessRate          float64        `json:"success_rate"`
	TotalCostMicroCents  int64          `json:"total_cost_micro_cents"`
	TotalCostDollars     float64        `json:"total_cost_dollars"`
	AvgCostMicroCents    int64          `json:"avg_cost_micro_cents"`
	TotalInputTokens     int64          `json:"total_input_tokens"`
	TotalOutputTokens    int64          `json:"total_output_tokens"`
	CostPerInputToken    float64        `json:"cost_per_input_token"`
	CostPerOutputToken   float64        `json:"cost_per_output_token"`
	AvgLatencyMs         float64        `json:"avg_latency_ms"`
	AvgComplexityScore   float64        `json:"avg_complexity_score"`
	ModelBreakdown       map[string]int `json:"model_breakdown"`
	OperationBreakdown   map[string]int `json:"operation_breakdown"`
}

// AICostTrends represents cost trends over time
type AICostTrends struct {
	Period     string               `json:"period"`
	StartTime  time.Time            `json:"start_time"`
	EndTime    time.Time            `json:"end_time"`
	DataPoints []AICostDataPoint    `json:"data_points"`
	Analysis   *AICostTrendAnalysis `json:"analysis"`
}

// AICostDataPoint represents a single data point in cost trends
type AICostDataPoint struct {
	Timestamp    time.Time `json:"timestamp"`
	TotalCost    float64   `json:"total_cost"`
	Operations   int64     `json:"operations"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	AvgLatencyMs float64   `json:"avg_latency_ms"`
	SuccessRate  float64   `json:"success_rate"`
}

// AICostTrendAnalysis represents sophisticated trend analysis
type AICostTrendAnalysis struct {
	TrendDirection  string      `json:"trend_direction"` // increasing, decreasing, stable
	GrowthRate      float64     `json:"growth_rate"`     // Percentage growth rate
	PeakTime        time.Time   `json:"peak_time"`
	LowTime         time.Time   `json:"low_time"`
	PeakCost        float64     `json:"peak_cost"`
	LowCost         float64     `json:"low_cost"`
	Volatility      float64     `json:"volatility"`       // Cost volatility measure
	Confidence      float64     `json:"confidence"`       // Statistical confidence in trend
	SeasonalFactors []string    `json:"seasonal_factors"` // Detected seasonal patterns
	PredictedCost   float64     `json:"predicted_cost"`   // Predicted next period cost
	Anomalies       []time.Time `json:"anomalies"`        // Detected anomalous periods
}
