package repositories

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// MetricsRepository handles metrics persistence
type MetricsRepository struct {
	*EnhancedBaseRepository[*models.Metrics]
	aggregatedRepo *EnhancedBaseRepository[*models.AggregatedMetrics]
	logger         *zap.Logger
}

// MetricRecordRepository handles new reporting table schema with extensive indexing
type MetricRecordRepository struct {
	*EnhancedBaseRepository[*models.MetricRecord]
	logger *zap.Logger
}

// NewMetricsRepository creates a new metrics repository with enhanced functionality
func NewMetricsRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *MetricsRepository {
	// Create enhanced repository optimized for metrics operations
	enhancedRepo := NewEnhancedBaseRepository[*models.Metrics](db, tableName, logger, costService, "MetricsRepository", "metrics")
	
	// Set up enhanced services for metrics operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Metrics cached for performance
	enhancedRepo.SetEventService(NewDefaultEventService()) // Important for metrics events
	
	// Create aggregated metrics repository
	aggregatedRepo := NewEnhancedBaseRepository[*models.AggregatedMetrics](db, tableName, logger, costService, "AggregatedMetricsRepository", "aggregatedmetrics")
	aggregatedRepo.SetValidationService(NewDefaultValidationService())
	aggregatedRepo.SetPermissionService(NewDefaultPermissionService())
	aggregatedRepo.SetCachingService(NewInMemoryCachingService())
	aggregatedRepo.SetEventService(NewDefaultEventService())

	return &MetricsRepository{
		EnhancedBaseRepository: enhancedRepo,
		aggregatedRepo:         aggregatedRepo,
		logger:                 logger,
	}
}

// Create creates a new metrics record
func (r *MetricsRepository) Create(ctx context.Context, metrics *models.Metrics) error {
	// Call BeforeCreate to set up the model
	if err := metrics.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, "metrics", "validation")
	}

	// Use enhanced repository for validation and creation
	err := r.ValidateAndCreate(ctx, metrics)
	if err != nil {
		return MapErrorWithContext(err, "failed to create metrics")
	}

	r.logger.Debug("created metrics",
		zap.String("id", metrics.ID),
		zap.String("type", metrics.Type),
		zap.String("service", metrics.Service))

	return nil
}

// BatchCreate creates multiple metrics records efficiently using DynamORM batch operations
func (r *MetricsRepository) BatchCreate(ctx context.Context, metricsList []*models.Metrics) error {
	if err := common.ValidateSliceNotEmpty("metrics_list", metricsList); err != nil {
		return nil
	}

	// Prepare all metrics
	for _, m := range metricsList {
		if err := m.BeforeCreate(); err != nil {
			return ErrorHandler.HandleCreateError(err, "metrics", m.ID)
		}
	}

	// Use enhanced batch create with validation
	return r.ValidateAndBatchCreate(ctx, metricsList)
}

// Get retrieves a metrics record by ID and type
func (r *MetricsRepository) Get(ctx context.Context, metricType, id string, timestamp time.Time) (*models.Metrics, error) {
	metrics := &models.Metrics{}

	// Construct the keys
	pk := fmt.Sprintf("metrics#%s", metricType)
	sk := fmt.Sprintf("ts#%s#%s", timestamp.Format("20060102150405"), id)

	err := r.BaseRepository.Get(ctx, pk, sk, metrics)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get metrics")
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

	query := r.db.WithContext(ctx).Model(&models.Metrics{}).
		Where("PK", "=", pk).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", "DESC").
		Limit(limit)

	err := query.All(&metricsList)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to list metrics by type")
	}

	return metricsList, nil
}

// ListByService lists metrics by service within a time range
func (r *MetricsRepository) ListByService(ctx context.Context, service string, startTime, endTime time.Time, limit int) ([]*models.Metrics, error) {
	var metricsList []*models.Metrics

	// Use GSI1 for service-based queries
	startSK := startTime.Format(time.RFC3339)
	endSK := endTime.Format(time.RFC3339)

	query := r.db.WithContext(ctx).Model(&models.Metrics{}).
		Index("service-index").
		Where("GSI1PK", "=", fmt.Sprintf("METRICS_SVC#%s", service)).
		Where("GSI1SK", ">=", startSK).
		Where("GSI1SK", "<=", endSK).
		OrderBy("GSI1SK", "DESC").
		Limit(limit)

	err := query.All(&metricsList)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to list metrics by service")
	}

	return metricsList, nil
}

// GetAggregated retrieves aggregated metrics
func (r *MetricsRepository) GetAggregated(ctx context.Context, period, metricType string, windowStart time.Time) (*models.AggregatedMetrics, error) {
	aggregated := &models.AggregatedMetrics{}

	pk := fmt.Sprintf("metrics_agg#%s#%s", period, metricType)
	sk := fmt.Sprintf("window#%s", windowStart.Format(time.RFC3339))

	err := r.aggregatedRepo.Get(ctx, pk, sk, aggregated)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get aggregated metrics")
	}

	return aggregated, nil
}

// CreateAggregated creates an aggregated metrics record
func (r *MetricsRepository) CreateAggregated(ctx context.Context, aggregated *models.AggregatedMetrics) error {
	// Call BeforeCreate to set up the model
	if err := aggregated.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, "aggregated metrics", "validation")
	}

	// Use enhanced repository for validation and creation
	err := r.aggregatedRepo.ValidateAndCreate(ctx, aggregated)
	if err != nil {
		return MapErrorWithContext(err, "failed to create aggregated metrics")
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
		return ErrorHandler.HandleUpdateError(err, "aggregated metrics", "validation")
	}

	// Use aggregated repository Update method
	err := r.aggregatedRepo.Update(ctx, aggregated)
	if err != nil {
		return MapErrorWithContext(err, "failed to update aggregated metrics")
	}

	return nil
}

// ListAggregatedByPeriod lists aggregated metrics for a period
func (r *MetricsRepository) ListAggregatedByPeriod(ctx context.Context, period, metricType string, startTime, endTime time.Time, limit int) ([]*models.AggregatedMetrics, error) {
	config := AggregatedQueryConfig{
		PKPrefix:    "metrics_agg",
		LogContext:  "metrics",
		ErrorPrefix: "failed to list aggregated metrics",
	}

	return ListAggregatedByPeriod[*models.AggregatedMetrics](
		ctx,
		r.aggregatedRepo.db,
		config,
		period,
		metricType,
		startTime,
		endTime,
		limit,
	)
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
		return ErrorHandler.HandleQueryError(err, "metrics", "aggregation")
	}

	if err := common.ValidateSliceNotEmpty("metrics", metrics); err != nil {
		return nil // Nothing to aggregate
	}

	// Calculate aggregated values
	aggregated := &models.AggregatedMetrics{
		Period:             period,
		Type:               metricType,
		Service:            metrics[0].Service, // Assume same service for now
		WindowStart:        windowStart,
		WindowEnd:          windowEnd,
		Percentiles:        make(map[string]float64),
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
	if err := common.ValidateSliceNotEmpty("values", values); err == nil {
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

// CleanupOldMetrics removes metrics older than the cutoff time for a specific granularity/period
func (r *MetricsRepository) CleanupOldMetrics(ctx context.Context, granularity string, cutoffTime time.Time) (int, error) {
	r.logger.Info("Starting metrics cleanup",
		zap.String("granularity", granularity),
		zap.Time("cutoff_time", cutoffTime))

	deletedCount := 0

	// Cleanup aggregated metrics based on granularity
	if granularity == "aggregated" || granularity == ConnectionTypeAll {
		periods := []string{"minute", "hour", "day", "week", "month"}
		for _, period := range periods {
			count, err := r.cleanupAggregatedMetricsByPeriod(ctx, period, cutoffTime)
			if err != nil {
				r.logger.Error("failed to cleanup aggregated metrics",
					zap.String("period", period),
					zap.Error(err))
				continue
			}
			deletedCount += count
		}
	}

	// Cleanup raw metrics if specified
	if granularity == "raw" || granularity == "all" {
		count, err := r.cleanupRawMetrics(ctx, cutoffTime)
		if err != nil {
			r.logger.Error("failed to cleanup raw metrics", zap.Error(err))
		} else {
			deletedCount += count
		}
	}

	r.logger.Info("Metrics cleanup completed",
		zap.String("granularity", granularity),
		zap.Int("deleted_count", deletedCount))

	return deletedCount, nil
}

// cleanupAggregatedMetricsByPeriod removes old aggregated metrics for a specific period
func (r *MetricsRepository) cleanupAggregatedMetricsByPeriod(ctx context.Context, period string, cutoffTime time.Time) (int, error) {
	deletedCount := 0

	// Query for old aggregated metrics using aggregate-index
	var oldMetrics []models.AggregatedMetrics

	// We need to query by period prefix since we can't easily do time-based filtering in DynamoDB
	// This is a limitation but necessary to avoid expensive scans
	err := r.aggregatedRepo.db.WithContext(ctx).Model(&models.AggregatedMetrics{}).
		Index("aggregate-index").
		Where("GSI2PK", "begins_with", fmt.Sprintf("METRICS_AGG#%s#", period)).
		Where("GSI2SK", "<", cutoffTime.Format("2006-01-02T15:04:05Z")).
		All(&oldMetrics)

	if err != nil {
		return 0, ErrorHandler.HandleQueryError(err, "aggregated metrics", "cleanup")
	}

	// Delete old metrics in batches
	for _, metric := range oldMetrics {
		if metric.WindowStart.Before(cutoffTime) {
			err := r.aggregatedRepo.Delete(ctx, metric.PK, metric.SK)
			if err != nil {
				r.logger.Warn("failed to delete aggregated metric",
					zap.String("pk", metric.PK),
					zap.String("sk", metric.SK),
					zap.Error(err))
				continue
			}
			deletedCount++
		}
	}

	return deletedCount, nil
}

// cleanupRawMetrics removes old raw metrics
func (r *MetricsRepository) cleanupRawMetrics(ctx context.Context, cutoffTime time.Time) (int, error) {
	deletedCount := 0

	// Query for old raw metrics - this is more challenging with single table design
	// We'll need to scan through metric types and clean based on timestamp
	var oldMetrics []models.Metrics

	// Use a broad query and filter in application code
	// This is not ideal but necessary with DynamoDB's query limitations
	err := r.db.WithContext(ctx).Model(&models.Metrics{}).
		Where("PK", "begins_with", "metrics#").
		All(&oldMetrics)

	if err != nil {
		return 0, ErrorHandler.HandleQueryError(err, "metrics", "cleanup")
	}

	// Filter and delete old metrics
	for _, metric := range oldMetrics {
		if metric.Timestamp.Before(cutoffTime) {
			err := r.Delete(ctx, metric.PK, metric.SK)
			if err != nil {
				r.logger.Warn("failed to delete raw metric",
					zap.String("pk", metric.PK),
					zap.String("sk", metric.SK),
					zap.Error(err))
				continue
			}
			deletedCount++
		}
	}

	return deletedCount, nil
}

// calculateMetricPercentiles calculates percentiles for a slice of metric values
// Returns a map with p50, p90, p95, and p99 percentiles
func calculateMetricPercentiles(values []float64) map[string]float64 {
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
		"p50": getMetricPercentileValue(sorted, 50),
		"p90": getMetricPercentileValue(sorted, 90),
		"p95": getMetricPercentileValue(sorted, 95),
		"p99": getMetricPercentileValue(sorted, 99),
	}

	return percentiles
}

// getMetricPercentileValue calculates the value at a specific percentile
func getMetricPercentileValue(sorted []float64, percentile float64) float64 {
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

// NewMetricRecordRepository creates a new metric record repository with enhanced functionality
func NewMetricRecordRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *MetricRecordRepository {
	// Create enhanced repository optimized for metric record operations
	enhancedRepo := NewEnhancedBaseRepository[*models.MetricRecord](db, tableName, logger, costService, "MetricRecordRepository", "metricrecord")
	
	// Set up enhanced services for metric record operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService())
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &MetricRecordRepository{
		EnhancedBaseRepository: enhancedRepo,
		logger:                 logger,
	}
}

// queryMetricsByTimeRange is a consolidated helper for GSI time range queries
func (r *MetricRecordRepository) queryMetricsByTimeRange(ctx context.Context, pkPrefix, pkValue, indexName, pkField, skField string, startTime, endTime time.Time, logContext map[string]interface{}, errorMsg string) ([]*models.MetricRecord, error) {
	var records []*models.MetricRecord

	pkValue = fmt.Sprintf("%s#%s", pkPrefix, pkValue)
	startSK := fmt.Sprintf("TIMESTAMP#%s", startTime.Format(time.RFC3339))
	endSK := fmt.Sprintf("TIMESTAMP#%s", endTime.Format(time.RFC3339))

	err := r.db.WithContext(ctx).Model(&models.MetricRecord{}).
		Index(indexName).
		Where(pkField, "=", pkValue).
		Where(skField, ">=", startSK).
		Where(skField, "<=", endSK).
		OrderBy(skField, "DESC").
		All(&records)

	if err != nil {
		logFields := []zap.Field{
			zap.Error(err),
			zap.Time("startTime", startTime),
			zap.Time("endTime", endTime),
		}
		for key, value := range logContext {
			logFields = append(logFields, zap.Any(key, value))
		}
		r.logger.Error(errorMsg, logFields...)
		return nil, MapErrorWithContext(err, errorMsg)
	}

	return records, nil
}

// GetMetricsByService queries metrics by service within a time range using GSI1
func (r *MetricRecordRepository) GetMetricsByService(ctx context.Context, serviceName string, startTime, endTime time.Time) ([]*models.MetricRecord, error) {
	return r.queryMetricsByTimeRange(ctx, "SERVICE", serviceName, "service-index", "GSI1PK", "GSI1SK", startTime, endTime,
		map[string]interface{}{"service": serviceName}, "failed to get metrics by service")
}

// GetMetricsByType queries metrics by type within a time range using GSI2
func (r *MetricRecordRepository) GetMetricsByType(ctx context.Context, metricType string, startTime, endTime time.Time) ([]*models.MetricRecord, error) {
	return r.queryMetricsByTimeRange(ctx, "METRIC_TYPE", metricType, "metric-type-index", "GSI2PK", "GSI2SK", startTime, endTime,
		map[string]interface{}{"metricType": metricType}, "failed to get metrics by type")
}

// GetMetricsByDate queries metrics by date and service using GSI3
func (r *MetricRecordRepository) GetMetricsByDate(ctx context.Context, date time.Time, serviceName string) ([]*models.MetricRecord, error) {
	var records []*models.MetricRecord

	gsi3pk := fmt.Sprintf("DATE#%s", date.Format(common.DateFormat))

	// Build the query
	query := r.db.WithContext(ctx).Model(&models.MetricRecord{}).
		Index("date-index").
		Where("GSI3PK", "=", gsi3pk)

	// If service is specified, add prefix filter
	if serviceName != "" {
		skPrefix := fmt.Sprintf("SERVICE#%s#", serviceName)
		query = query.Where("GSI3SK", "BEGINS_WITH", skPrefix)
	}

	err := query.OrderBy("GSI3SK", "DESC").All(&records)

	if err != nil {
		r.logger.Error("failed to get metrics by date",
			zap.Error(err),
			zap.Time("date", date),
			zap.String("service", serviceName))
		return nil, MapErrorWithContext(err, "failed to get metrics by date")
	}

	return records, nil
}

// GetMetricsByAggregationLevel queries metrics by aggregation level within a time range using GSI4
func (r *MetricRecordRepository) GetMetricsByAggregationLevel(ctx context.Context, level string, startTime, endTime time.Time) ([]*models.MetricRecord, error) {
	return r.queryMetricsByTimeRange(ctx, "AGGREGATION", level, "aggregation-index", "GSI4PK", "GSI4SK", startTime, endTime,
		map[string]interface{}{"level": level}, "failed to get metrics by aggregation level")
}

// CreateMetricRecord creates a new metric record
func (r *MetricRecordRepository) CreateMetricRecord(ctx context.Context, record *models.MetricRecord) error {
	// Call BeforeCreate to set up the model
	if err := record.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, "metric record", "validation")
	}

	// Use BaseRepository Create method
	err := r.Create(ctx, record)
	if err != nil {
		r.logger.Error("failed to create metric record",
			zap.Error(err),
			zap.String("metricType", record.MetricType),
			zap.String("service", record.ServiceName),
			zap.String("level", record.AggregationLevel))
		return ErrorHandler.HandleCreateError(err, "metric record", record.MetricID)
	}

	r.logger.Debug("created metric record",
		zap.String("id", record.MetricID),
		zap.String("type", record.MetricType),
		zap.String("service", record.ServiceName),
		zap.String("level", record.AggregationLevel))

	return nil
}

// BatchCreateMetricRecords creates multiple metric records efficiently
func (r *MetricRecordRepository) BatchCreateMetricRecords(ctx context.Context, records []*models.MetricRecord) error {
	if err := common.ValidateSliceNotEmpty("records", records); err != nil {
		return nil
	}

	// Prepare all records
	for _, record := range records {
		if err := record.BeforeCreate(); err != nil {
			return ErrorHandler.HandleCreateError(err, "metric record", record.MetricID)
		}
	}

	// Use enhanced batch create with validation
	return r.ValidateAndBatchCreate(ctx, records)
}

// GetMetricRecord retrieves a single metric record by its keys
func (r *MetricRecordRepository) GetMetricRecord(ctx context.Context, metricType, bucket, timestamp string) (*models.MetricRecord, error) {
	record := &models.MetricRecord{}

	pk := fmt.Sprintf("METRICS#%s#%s", metricType, bucket)
	sk := timestamp

	err := r.Get(ctx, pk, sk, record)
	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, "metric record", fmt.Sprintf("%s#%s#%s", metricType, bucket, timestamp))
	}

	return record, nil
}

// UpdateMetricRecord updates an existing metric record
func (r *MetricRecordRepository) UpdateMetricRecord(ctx context.Context, record *models.MetricRecord) error {
	// Call BeforeUpdate to set up the model
	if err := record.BeforeUpdate(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "metric record", "validation")
	}

	// Use BaseRepository Update method
	err := r.Update(ctx, record)
	if err != nil {
		r.logger.Error("failed to update metric record",
			zap.Error(err),
			zap.String("metricType", record.MetricType),
			zap.String("service", record.ServiceName))
		return ErrorHandler.HandleUpdateError(err, "metric record", record.MetricID)
	}

	return nil
}

// DeleteMetricRecord deletes a metric record by its keys
func (r *MetricRecordRepository) DeleteMetricRecord(ctx context.Context, metricType, bucket, timestamp string) error {
	pk := fmt.Sprintf("METRICS#%s#%s", metricType, bucket)
	sk := timestamp

	err := r.Delete(ctx, pk, sk)
	if err != nil {
		r.logger.Error("failed to delete metric record",
			zap.Error(err),
			zap.String("pk", pk),
			zap.String("sk", sk))
		return ErrorHandler.HandleDeleteError(err, "metric record", fmt.Sprintf("%s#%s", pk, sk))
	}

	return nil
}

// GetServiceMetricsStats calculates statistics for a service's metrics
func (r *MetricRecordRepository) GetServiceMetricsStats(ctx context.Context, serviceName string, metricType string, startTime, endTime time.Time) (*MetricRecordStats, error) {
	records, err := r.GetMetricsByService(ctx, serviceName, startTime, endTime)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "metric record", "service stats")
	}

	// Filter by metric type if specified
	if metricType != "" {
		filtered := make([]*models.MetricRecord, 0)
		for _, record := range records {
			if record.MetricType == metricType {
				filtered = append(filtered, record)
			}
		}
		records = filtered
	}

	stats := &MetricRecordStats{
		Service:   serviceName,
		Type:      metricType,
		StartTime: startTime,
		EndTime:   endTime,
		Count:     len(records),
	}

	if stats.Count == 0 {
		return stats, nil
	}

	// Calculate statistics
	var totalSum float64
	var totalCount int64
	stats.Min = records[0].Min
	stats.Max = records[0].Max

	for _, record := range records {
		totalSum += record.Sum
		totalCount += record.Count

		if record.Min < stats.Min {
			stats.Min = record.Min
		}
		if record.Max > stats.Max {
			stats.Max = record.Max
		}
	}

	if totalCount > 0 {
		stats.Average = totalSum / float64(totalCount)
	}
	stats.TotalSum = totalSum
	stats.TotalCount = totalCount

	return stats, nil
}

// MetricRecordStats represents statistics for metric records
type MetricRecordStats struct {
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
