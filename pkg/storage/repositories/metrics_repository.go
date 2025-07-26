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

// MetricsRepository handles metrics persistence
type MetricsRepository struct {
	dynamorm.BaseRepository
	logger *zap.Logger
}

// NewMetricsRepository creates a new metrics repository
func NewMetricsRepository(db core.DB, tableName string, logger *zap.Logger) *MetricsRepository {
	return &MetricsRepository{
		BaseRepository: *dynamorm.NewBaseRepository(db, tableName),
		logger:         logger,
	}
}

// Create creates a new metrics record
func (r *MetricsRepository) Create(ctx context.Context, metrics *models.Metrics) error {
	// Call BeforeCreate to set up the model
	if err := metrics.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Create the metrics
	err := r.GetDB().Model(metrics).Create()
	if err != nil {
		return dynamorm.MapErrorWithContext(err, "failed to create metrics")
	}

	r.logger.Debug("created metrics",
		zap.String("id", metrics.ID),
		zap.String("type", metrics.Type),
		zap.String("service", metrics.Service))

	return nil
}

// BatchCreate creates multiple metrics records efficiently
func (r *MetricsRepository) BatchCreate(ctx context.Context, metricsList []*models.Metrics) error {
	if len(metricsList) == 0 {
		return nil
	}

	// Prepare all metrics
	for _, m := range metricsList {
		if err := m.BeforeCreate(); err != nil {
			return fmt.Errorf("before create validation failed for metric %s: %w", m.ID, err)
		}
	}

	// Use batch writer for efficiency
	// Note: This is a simplified version - real implementation would use DynamORM's batch capabilities
	for _, m := range metricsList {
		if err := r.GetDB().Model(m).Create(); err != nil {
			r.logger.Error("failed to create metric in batch",
				zap.String("id", m.ID),
				zap.Error(err))
			// Continue with other metrics
		}
	}

	return nil
}

// Get retrieves a metrics record by ID and type
func (r *MetricsRepository) Get(ctx context.Context, metricType, id string, timestamp time.Time) (*models.Metrics, error) {
	metrics := &models.Metrics{}

	// Construct the keys
	pk := fmt.Sprintf("metrics#%s", metricType)
	sk := fmt.Sprintf("ts#%s#%s", timestamp.Format("20060102150405"), id)

	err := r.GetDB().Model(metrics).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(metrics)

	if err != nil {
		return nil, dynamorm.MapErrorWithContext(err, "failed to get metrics")
	}

	return metrics, nil
}

// ListByType lists metrics by type within a time range
func (r *MetricsRepository) ListByType(ctx context.Context, metricType string, startTime, endTime time.Time, limit int) ([]*models.Metrics, error) {
	var metricsList []*models.Metrics

	// Construct SK range for time-based query
	pk := fmt.Sprintf("metrics#%s", metricType)
	startSK := fmt.Sprintf("ts#%s", startTime.Format("20060102150405"))
	endSK := fmt.Sprintf("ts#%s", endTime.Format("20060102150405"))

	query := r.GetDB().Model(&models.Metrics{}).
		Where("PK", "=", pk).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", "DESC").
		Limit(limit)

	err := query.All(&metricsList)
	if err != nil {
		return nil, dynamorm.MapErrorWithContext(err, "failed to list metrics by type")
	}

	return metricsList, nil
}

// ListByService lists metrics by service within a time range
func (r *MetricsRepository) ListByService(ctx context.Context, service string, startTime, endTime time.Time, limit int) ([]*models.Metrics, error) {
	var metricsList []*models.Metrics

	// Use GSI1 for service-based queries
	startSK := startTime.Format(time.RFC3339)
	endSK := endTime.Format(time.RFC3339)

	query := r.GetDB().Model(&models.Metrics{}).
		Index("service-index").
		Where("GSI1PK", "=", fmt.Sprintf("METRICS_SVC#%s", service)).
		Where("GSI1SK", ">=", startSK).
		Where("GSI1SK", "<=", endSK).
		OrderBy("GSI1SK", "DESC").
		Limit(limit)

	err := query.All(&metricsList)
	if err != nil {
		return nil, dynamorm.MapErrorWithContext(err, "failed to list metrics by service")
	}

	return metricsList, nil
}

// GetAggregated retrieves aggregated metrics
func (r *MetricsRepository) GetAggregated(ctx context.Context, period, metricType string, windowStart time.Time) (*models.AggregatedMetrics, error) {
	aggregated := &models.AggregatedMetrics{}

	pk := fmt.Sprintf("metrics_agg#%s#%s", period, metricType)
	sk := fmt.Sprintf("window#%s", windowStart.Format(time.RFC3339))

	err := r.GetDB().Model(aggregated).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(aggregated)

	if err != nil {
		return nil, dynamorm.MapErrorWithContext(err, "failed to get aggregated metrics")
	}

	return aggregated, nil
}

// CreateAggregated creates an aggregated metrics record
func (r *MetricsRepository) CreateAggregated(ctx context.Context, aggregated *models.AggregatedMetrics) error {
	// Call BeforeCreate to set up the model
	if err := aggregated.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Create the aggregated metrics
	err := r.GetDB().Model(aggregated).Create()
	if err != nil {
		return dynamorm.MapErrorWithContext(err, "failed to create aggregated metrics")
	}

	r.logger.Debug("created aggregated metrics",
		zap.String("type", aggregated.Type),
		zap.String("period", aggregated.Period),
		zap.Time("window_start", aggregated.WindowStart))

	return nil
}

// UpdateAggregated updates an existing aggregated metrics record
func (r *MetricsRepository) UpdateAggregated(ctx context.Context, aggregated *models.AggregatedMetrics) error {
	// Call BeforeUpdate to set up the model
	if err := aggregated.BeforeUpdate(); err != nil {
		return fmt.Errorf("before update validation failed: %w", err)
	}

	// Update the aggregated metrics
	err := r.GetDB().Model(aggregated).Update()
	if err != nil {
		return dynamorm.MapErrorWithContext(err, "failed to update aggregated metrics")
	}

	return nil
}

// ListAggregatedByPeriod lists aggregated metrics for a period
func (r *MetricsRepository) ListAggregatedByPeriod(ctx context.Context, period, metricType string, startTime, endTime time.Time, limit int) ([]*models.AggregatedMetrics, error) {
	var aggregatedList []*models.AggregatedMetrics

	pk := fmt.Sprintf("metrics_agg#%s#%s", period, metricType)
	startSK := fmt.Sprintf("window#%s", startTime.Format(time.RFC3339))
	endSK := fmt.Sprintf("window#%s", endTime.Format(time.RFC3339))

	query := r.GetDB().Model(&models.AggregatedMetrics{}).
		Where("PK", "=", pk).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", "DESC").
		Limit(limit)

	err := query.All(&aggregatedList)
	if err != nil {
		return nil, dynamorm.MapErrorWithContext(err, "failed to list aggregated metrics")
	}

	return aggregatedList, nil
}

// GetServiceStats calculates statistics for a service
func (r *MetricsRepository) GetServiceStats(ctx context.Context, service string, metricType string, startTime, endTime time.Time) (*ServiceStats, error) {
	metrics, err := r.ListByService(ctx, service, startTime, endTime, 10000)
	if err != nil {
		return nil, err
	}

	// Filter by metric type if specified
	if metricType != "" {
		filtered := make([]*models.Metrics, 0)
		for _, m := range metrics {
			if m.Type == metricType {
				filtered = append(filtered, m)
			}
		}
		metrics = filtered
	}

	stats := &ServiceStats{
		Service:   service,
		Type:      metricType,
		StartTime: startTime,
		EndTime:   endTime,
		Count:     len(metrics),
	}

	if stats.Count == 0 {
		return stats, nil
	}

	// Calculate statistics
	var totalSum float64
	var totalCount int64
	stats.Min = metrics[0].Min
	stats.Max = metrics[0].Max

	for _, m := range metrics {
		totalSum += m.Sum
		totalCount += m.Count
		
		if m.Min < stats.Min {
			stats.Min = m.Min
		}
		if m.Max > stats.Max {
			stats.Max = m.Max
		}
	}

	if totalCount > 0 {
		stats.Average = totalSum / float64(totalCount)
	}
	stats.TotalSum = totalSum
	stats.TotalCount = totalCount

	return stats, nil
}

// Aggregate performs aggregation of raw metrics
func (r *MetricsRepository) Aggregate(ctx context.Context, metricType, period string, windowStart, windowEnd time.Time) error {
	// Get all metrics in the window
	metrics, err := r.ListByType(ctx, metricType, windowStart, windowEnd, 10000)
	if err != nil {
		return fmt.Errorf("failed to list metrics for aggregation: %w", err)
	}

	if len(metrics) == 0 {
		return nil // Nothing to aggregate
	}

	// Calculate aggregated values
	aggregated := &models.AggregatedMetrics{
		Period:      period,
		Type:        metricType,
		Service:     metrics[0].Service, // Assume same service for now
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		Percentiles: make(map[string]float64),
		DimensionBreakdown: make(map[string]models.DimensionStats),
	}

	var values []float64
	aggregated.Min = metrics[0].Min
	aggregated.Max = metrics[0].Max

	for _, m := range metrics {
		aggregated.TotalCount += m.Count
		aggregated.TotalSum += m.Sum
		
		if m.Min < aggregated.Min {
			aggregated.Min = m.Min
		}
		if m.Max > aggregated.Max {
			aggregated.Max = m.Max
		}

		// Collect values for percentile calculation
		for i := int64(0); i < m.Count; i++ {
			values = append(values, m.Average)
		}

		// Aggregate by dimensions
		for dimKey, dimValue := range m.Dimensions {
			key := fmt.Sprintf("%s=%s", dimKey, dimValue)
			dimStats := aggregated.DimensionBreakdown[key]
			dimStats.Value = dimValue
			dimStats.Count += m.Count
			dimStats.Sum += m.Sum
			if m.Min < dimStats.Min || dimStats.Min == 0 {
				dimStats.Min = m.Min
			}
			if m.Max > dimStats.Max {
				dimStats.Max = m.Max
			}
			aggregated.DimensionBreakdown[key] = dimStats
		}
	}

	// Calculate average
	if aggregated.TotalCount > 0 {
		aggregated.Average = aggregated.TotalSum / float64(aggregated.TotalCount)
	}

	// Calculate dimension averages
	for key, dimStats := range aggregated.DimensionBreakdown {
		if dimStats.Count > 0 {
			dimStats.Average = dimStats.Sum / float64(dimStats.Count)
			aggregated.DimensionBreakdown[key] = dimStats
		}
	}

	// Calculate percentiles and standard deviation
	if len(values) > 0 {
		aggregated.Percentiles = calculateMetricPercentiles(values)
		aggregated.StdDev = calculateStandardDeviation(values, aggregated.Average)
	}

	// Check if aggregation already exists
	existing, err := r.GetAggregated(ctx, period, metricType, windowStart)
	if err == nil && existing != nil {
		// Update existing
		aggregated.CreatedAt = existing.CreatedAt
		return r.UpdateAggregated(ctx, aggregated)
	}

	// Create new aggregation
	return r.CreateAggregated(ctx, aggregated)
}

// ServiceStats represents statistics for a service
type ServiceStats struct {
	Service    string
	Type       string
	StartTime  time.Time
	EndTime    time.Time
	Count      int
	TotalCount int64
	TotalSum   float64
	Average    float64
	Min        float64
	Max        float64
}

// calculateMetricPercentiles calculates percentiles for a slice of metric values
// Returns a map with p50, p90, p95, and p99 percentiles
func calculateMetricPercentiles(values []float64) map[string]float64 {
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
		"p50": getMetricPercentileValue(sorted, 50),
		"p90": getMetricPercentileValue(sorted, 90),
		"p95": getMetricPercentileValue(sorted, 95),
		"p99": getMetricPercentileValue(sorted, 99),
	}

	return percentiles
}

// getMetricPercentileValue calculates the value at a specific percentile
func getMetricPercentileValue(sorted []float64, percentile float64) float64 {
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

// calculateStandardDeviation calculates the standard deviation for a slice of values
func calculateStandardDeviation(values []float64, mean float64) float64 {
	if len(values) <= 1 {
		return 0
	}

	var sumSquaredDiff float64
	for _, value := range values {
		diff := value - mean
		sumSquaredDiff += diff * diff
	}

	variance := sumSquaredDiff / float64(len(values))
	return math.Sqrt(variance)
}