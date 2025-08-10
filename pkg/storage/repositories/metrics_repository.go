package repositories

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// MetricsRepository handles metrics persistence
type MetricsRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// MetricRecordRepository handles new reporting table schema with extensive indexing
type MetricRecordRepository struct {
	*BaseRepository[*models.MetricRecord]
	logger *zap.Logger
}

// NewMetricsRepository creates a new metrics repository
func NewMetricsRepository(db core.DB, tableName string, logger *zap.Logger) *MetricsRepository {
	return &MetricsRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// Create creates a new metrics record
func (r *MetricsRepository) Create(_ context.Context, metrics *models.Metrics) error {
	// Call BeforeCreate to set up the model
	if err := metrics.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Create the metrics
	err := r.db.Model(metrics).Create()
	if err != nil {
		return MapErrorWithContext(err, "failed to create metrics")
	}

	r.logger.Debug("created metrics",
		zap.String("id", metrics.ID),
		zap.String("type", metrics.Type),
		zap.String("service", metrics.Service))

	return nil
}

// BatchCreate creates multiple metrics records efficiently
func (r *MetricsRepository) BatchCreate(_ context.Context, metricsList []*models.Metrics) error {
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
		if err := r.db.Model(m).Create(); err != nil {
			r.logger.Error("failed to create metric in batch",
				zap.String("id", m.ID),
				zap.Error(err))
			// Continue with other metrics
		}
	}

	return nil
}

// Get retrieves a metrics record by ID and type
func (r *MetricsRepository) Get(_ context.Context, metricType, id string, timestamp time.Time) (*models.Metrics, error) {
	metrics := &models.Metrics{}

	// Construct the keys
	pk := fmt.Sprintf("metrics#%s", metricType)
	sk := fmt.Sprintf("ts#%s#%s", timestamp.Format("20060102150405"), id)

	err := r.db.Model(metrics).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(metrics)

	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get metrics")
	}

	return metrics, nil
}

// ListByType lists metrics by type within a time range
func (r *MetricsRepository) ListByType(_ context.Context, metricType string, startTime, endTime time.Time, limit int) ([]*models.Metrics, error) {
	var metricsList []*models.Metrics

	// Construct SK range for time-based query
	pk := fmt.Sprintf("metrics#%s", metricType)
	startSK := fmt.Sprintf("ts#%s", startTime.Format("20060102150405"))
	endSK := fmt.Sprintf("ts#%s", endTime.Format("20060102150405"))

	query := r.db.Model(&models.Metrics{}).
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
func (r *MetricsRepository) ListByService(_ context.Context, service string, startTime, endTime time.Time, limit int) ([]*models.Metrics, error) {
	var metricsList []*models.Metrics

	// Use GSI1 for service-based queries
	startSK := startTime.Format(time.RFC3339)
	endSK := endTime.Format(time.RFC3339)

	query := r.db.Model(&models.Metrics{}).
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
func (r *MetricsRepository) GetAggregated(_ context.Context, period, metricType string, windowStart time.Time) (*models.AggregatedMetrics, error) {
	aggregated := &models.AggregatedMetrics{}

	pk := fmt.Sprintf("metrics_agg#%s#%s", period, metricType)
	sk := fmt.Sprintf("window#%s", windowStart.Format(time.RFC3339))

	err := r.db.Model(aggregated).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(aggregated)

	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get aggregated metrics")
	}

	return aggregated, nil
}

// CreateAggregated creates an aggregated metrics record
func (r *MetricsRepository) CreateAggregated(_ context.Context, aggregated *models.AggregatedMetrics) error {
	// Call BeforeCreate to set up the model
	if err := aggregated.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Create the aggregated metrics
	err := r.db.Model(aggregated).Create()
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
func (r *MetricsRepository) UpdateAggregated(_ context.Context, aggregated *models.AggregatedMetrics) error {
	// Call BeforeUpdate to set up the model
	if err := aggregated.BeforeUpdate(); err != nil {
		return fmt.Errorf("before update validation failed: %w", err)
	}

	// Update the aggregated metrics
	err := r.db.Model(aggregated).Update()
	if err != nil {
		return MapErrorWithContext(err, "failed to update aggregated metrics")
	}

	return nil
}

// ListAggregatedByPeriod lists aggregated metrics for a period
func (r *MetricsRepository) ListAggregatedByPeriod(_ context.Context, period, metricType string, startTime, endTime time.Time, limit int) ([]*models.AggregatedMetrics, error) {
	var aggregatedList []*models.AggregatedMetrics

	pk := fmt.Sprintf("metrics_agg#%s#%s", period, metricType)
	startSK := fmt.Sprintf("window#%s", startTime.Format(time.RFC3339))
	endSK := fmt.Sprintf("window#%s", endTime.Format(time.RFC3339))

	query := r.db.Model(&models.AggregatedMetrics{}).
		Where("PK", "=", pk).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", "DESC").
		Limit(limit)

	err := query.All(&aggregatedList)
	if err != nil {
		return nil, MapErrorWithContext(err, "failed to list aggregated metrics")
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

// NewMetricRecordRepository creates a new metric record repository following BaseRepository pattern
func NewMetricRecordRepository(db core.DB, tableName string, logger *zap.Logger) *MetricRecordRepository {
	return &MetricRecordRepository{
		BaseRepository: NewBaseRepository[*models.MetricRecord](db, tableName, logger),
		logger:         logger,
	}
}

// GetMetricsByService queries metrics by service within a time range using GSI1
func (r *MetricRecordRepository) GetMetricsByService(ctx context.Context, serviceName string, startTime, endTime time.Time) ([]*models.MetricRecord, error) {
	var records []*models.MetricRecord

	gsi1pk := fmt.Sprintf("SERVICE#%s", serviceName)
	startSK := fmt.Sprintf("TIMESTAMP#%s", startTime.Format(time.RFC3339))
	endSK := fmt.Sprintf("TIMESTAMP#%s", endTime.Format(time.RFC3339))

	err := r.BaseRepository.db.WithContext(ctx).Model(&models.MetricRecord{}).
		Index("service-index").
		Where("GSI1PK", "=", gsi1pk).
		Where("GSI1SK", ">=", startSK).
		Where("GSI1SK", "<=", endSK).
		OrderBy("GSI1SK", "DESC").
		All(&records)

	if err != nil {
		r.logger.Error("failed to get metrics by service",
			zap.Error(err),
			zap.String("service", serviceName),
			zap.Time("startTime", startTime),
			zap.Time("endTime", endTime))
		return nil, MapErrorWithContext(err, "failed to get metrics by service")
	}

	return records, nil
}

// GetMetricsByType queries metrics by type within a time range using GSI2
func (r *MetricRecordRepository) GetMetricsByType(ctx context.Context, metricType string, startTime, endTime time.Time) ([]*models.MetricRecord, error) {
	var records []*models.MetricRecord

	gsi2pk := fmt.Sprintf("METRIC_TYPE#%s", metricType)
	startSK := fmt.Sprintf("TIMESTAMP#%s", startTime.Format(time.RFC3339))
	endSK := fmt.Sprintf("TIMESTAMP#%s", endTime.Format(time.RFC3339))

	err := r.BaseRepository.db.WithContext(ctx).Model(&models.MetricRecord{}).
		Index("metric-type-index").
		Where("GSI2PK", "=", gsi2pk).
		Where("GSI2SK", ">=", startSK).
		Where("GSI2SK", "<=", endSK).
		OrderBy("GSI2SK", "DESC").
		All(&records)

	if err != nil {
		r.logger.Error("failed to get metrics by type",
			zap.Error(err),
			zap.String("metricType", metricType),
			zap.Time("startTime", startTime),
			zap.Time("endTime", endTime))
		return nil, MapErrorWithContext(err, "failed to get metrics by type")
	}

	return records, nil
}

// GetMetricsByDate queries metrics by date and service using GSI3
func (r *MetricRecordRepository) GetMetricsByDate(ctx context.Context, date time.Time, serviceName string) ([]*models.MetricRecord, error) {
	var records []*models.MetricRecord

	gsi3pk := fmt.Sprintf("DATE#%s", date.Format(common.DateFormat))

	// Build the query
	query := r.BaseRepository.db.WithContext(ctx).Model(&models.MetricRecord{}).
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
	var records []*models.MetricRecord

	gsi4pk := fmt.Sprintf("AGGREGATION#%s", level)
	startSK := fmt.Sprintf("TIMESTAMP#%s", startTime.Format(time.RFC3339))
	endSK := fmt.Sprintf("TIMESTAMP#%s", endTime.Format(time.RFC3339))

	err := r.BaseRepository.db.WithContext(ctx).Model(&models.MetricRecord{}).
		Index("aggregation-index").
		Where("GSI4PK", "=", gsi4pk).
		Where("GSI4SK", ">=", startSK).
		Where("GSI4SK", "<=", endSK).
		OrderBy("GSI4SK", "DESC").
		All(&records)

	if err != nil {
		r.logger.Error("failed to get metrics by aggregation level",
			zap.Error(err),
			zap.String("level", level),
			zap.Time("startTime", startTime),
			zap.Time("endTime", endTime))
		return nil, MapErrorWithContext(err, "failed to get metrics by aggregation level")
	}

	return records, nil
}

// CreateMetricRecord creates a new metric record
func (r *MetricRecordRepository) CreateMetricRecord(ctx context.Context, record *models.MetricRecord) error {
	// Call BeforeCreate to set up the model
	if err := record.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Use BaseRepository Create method
	err := r.Create(ctx, record)
	if err != nil {
		r.logger.Error("failed to create metric record",
			zap.Error(err),
			zap.String("metricType", record.MetricType),
			zap.String("service", record.ServiceName),
			zap.String("level", record.AggregationLevel))
		return fmt.Errorf("failed to create metric record: %w", err)
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
	if len(records) == 0 {
		return nil
	}

	// Prepare all records
	for _, record := range records {
		if err := record.BeforeCreate(); err != nil {
			return fmt.Errorf("before create validation failed for record %s: %w", record.MetricID, err)
		}
	}

	// Create each record (in a real implementation, this would use DynamORM's batch capabilities)
	var errors []error
	for _, record := range records {
		if err := r.Create(ctx, record); err != nil {
			r.logger.Error("failed to create metric record in batch",
				zap.String("id", record.MetricID),
				zap.Error(err))
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("batch create had %d errors, first: %w", len(errors), errors[0])
	}

	r.logger.Debug("batch created metric records", zap.Int("count", len(records)))
	return nil
}

// GetMetricRecord retrieves a single metric record by its keys
func (r *MetricRecordRepository) GetMetricRecord(ctx context.Context, metricType, bucket, timestamp string) (*models.MetricRecord, error) {
	record := &models.MetricRecord{}

	pk := fmt.Sprintf("METRICS#%s#%s", metricType, bucket)
	sk := timestamp

	err := r.Get(ctx, pk, sk, record)
	if err != nil {
		return nil, fmt.Errorf("failed to get metric record: %w", err)
	}

	return record, nil
}

// UpdateMetricRecord updates an existing metric record
func (r *MetricRecordRepository) UpdateMetricRecord(ctx context.Context, record *models.MetricRecord) error {
	// Call BeforeUpdate to set up the model
	if err := record.BeforeUpdate(); err != nil {
		return fmt.Errorf("before update validation failed: %w", err)
	}

	// Use BaseRepository Update method
	err := r.Update(ctx, record)
	if err != nil {
		r.logger.Error("failed to update metric record",
			zap.Error(err),
			zap.String("metricType", record.MetricType),
			zap.String("service", record.ServiceName))
		return fmt.Errorf("failed to update metric record: %w", err)
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
		return fmt.Errorf("failed to delete metric record: %w", err)
	}

	return nil
}

// GetServiceMetricsStats calculates statistics for a service's metrics
func (r *MetricRecordRepository) GetServiceMetricsStats(ctx context.Context, serviceName string, metricType string, startTime, endTime time.Time) (*MetricRecordStats, error) {
	records, err := r.GetMetricsByService(ctx, serviceName, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get service metrics: %w", err)
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
