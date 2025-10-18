// Package observability provides DynamORM operation latency tracking
package observability

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// Context key types for type-safe context values
type contextKey string

const (
	latencyOperationKey contextKey = "latency_operation"
	latencyStartKey     contextKey = "latency_start"
)

// DynamORMTracker wraps DynamORM operations with latency tracking
type DynamORMTracker struct {
	db              core.DB
	logger          *zap.Logger
	metricsRecorder MetricsRecorder
}

// MetricsRecorder interface for recording latency metrics
type MetricsRecorder interface {
	RecordLatency(ctx context.Context, operation, table string, duration time.Duration, success bool, dimensions map[string]string) error
}

// DefaultMetricsRecorder implements MetricsRecorder using the MetricRecord repository pattern
type DefaultMetricsRecorder struct {
	createMetricFn func(ctx context.Context, metric *models.MetricRecord) error
	serviceName    string
}

// NewDynamORMTracker creates a new DynamORM operation tracker
func NewDynamORMTracker(db core.DB, logger *zap.Logger, recorder MetricsRecorder) *DynamORMTracker {
	return &DynamORMTracker{
		db:              db,
		logger:          logger,
		metricsRecorder: recorder,
	}
}

// NewDefaultMetricsRecorder creates a default metrics recorder
func NewDefaultMetricsRecorder(createMetricFn func(ctx context.Context, metric *models.MetricRecord) error, serviceName string) *DefaultMetricsRecorder {
	return &DefaultMetricsRecorder{
		createMetricFn: createMetricFn,
		serviceName:    serviceName,
	}
}

// RecordLatency records latency metrics for database operations
func (r *DefaultMetricsRecorder) RecordLatency(ctx context.Context, operation, table string, duration time.Duration, success bool, dimensions map[string]string) error {
	// Create metric record
	metric := &models.MetricRecord{
		MetricType:       "database_operation",
		ServiceName:      r.serviceName,
		Timestamp:        time.Now(),
		AggregationLevel: "raw",
		Unit:             "ms",
		Dimensions:       dimensions,
		// Statistical values for single measurement
		Count: 1,
		Sum:   float64(duration.Milliseconds()),
		Min:   float64(duration.Milliseconds()),
		Max:   float64(duration.Milliseconds()),
		P50:   float64(duration.Milliseconds()),
		P95:   float64(duration.Milliseconds()),
		P99:   float64(duration.Milliseconds()),
	}

	// Add operation details to dimensions
	metric.AddDimension("operation", operation)
	metric.AddDimension("table", table)
	metric.AddDimension("success", fmt.Sprintf("%t", success))

	// Record the metric
	return r.createMetricFn(ctx, metric)
}

// TrackQuery wraps a DynamORM query operation with latency tracking
func (t *DynamORMTracker) TrackQuery(ctx context.Context, operation string, table string, queryFn func() error) error {
	startTime := time.Now()
	err := queryFn()
	duration := time.Since(startTime)

	// Prepare dimensions
	dimensions := map[string]string{
		"operation_type": "query",
	}

	// Record the latency
	success := err == nil
	if recordErr := t.metricsRecorder.RecordLatency(ctx, operation, table, duration, success, dimensions); recordErr != nil {
		t.logger.Warn("failed to record query latency",
			zap.String("operation", operation),
			zap.String("table", table),
			zap.Duration("duration", duration),
			zap.Error(recordErr))
	}

	// Log the query performance
	logLevel := zap.DebugLevel
	if duration > 1*time.Second {
		logLevel = zap.WarnLevel
	} else if duration > 500*time.Millisecond {
		logLevel = zap.InfoLevel
	}

	t.logger.Log(logLevel, "DynamoDB query completed",
		zap.String("operation", operation),
		zap.String("table", table),
		zap.Duration("duration", duration),
		zap.Bool("success", success),
		zap.Error(err))

	return err
}

// TrackCreate wraps a DynamORM create operation with latency tracking
func (t *DynamORMTracker) TrackCreate(ctx context.Context, table string, createFn func() error) error {
	return t.TrackQuery(ctx, "create", table, createFn)
}

// TrackUpdate wraps a DynamORM update operation with latency tracking
func (t *DynamORMTracker) TrackUpdate(ctx context.Context, table string, updateFn func() error) error {
	return t.TrackQuery(ctx, "update", table, updateFn)
}

// TrackDelete wraps a DynamORM delete operation with latency tracking
func (t *DynamORMTracker) TrackDelete(ctx context.Context, table string, deleteFn func() error) error {
	return t.TrackQuery(ctx, "delete", table, deleteFn)
}

// TrackBatch wraps a DynamORM batch operation with latency tracking
func (t *DynamORMTracker) TrackBatch(ctx context.Context, operation string, table string, count int, batchFn func() error) error {
	startTime := time.Now()
	err := batchFn()
	duration := time.Since(startTime)

	// Prepare dimensions
	dimensions := map[string]string{
		"operation_type": "batch",
		"batch_size":     fmt.Sprintf("%d", count),
	}

	// Record the latency
	success := err == nil
	if recordErr := t.metricsRecorder.RecordLatency(ctx, operation, table, duration, success, dimensions); recordErr != nil {
		t.logger.Warn("failed to record batch latency",
			zap.String("operation", operation),
			zap.String("table", table),
			zap.Int("count", count),
			zap.Duration("duration", duration),
			zap.Error(recordErr))
	}

	// Log batch operation performance
	logLevel := zap.DebugLevel
	avgLatencyPerItem := duration / time.Duration(count)
	if avgLatencyPerItem > 100*time.Millisecond {
		logLevel = zap.WarnLevel
	} else if avgLatencyPerItem > 50*time.Millisecond {
		logLevel = zap.InfoLevel
	}

	t.logger.Log(logLevel, "DynamoDB batch operation completed",
		zap.String("operation", operation),
		zap.String("table", table),
		zap.Int("count", count),
		zap.Duration("total_duration", duration),
		zap.Duration("avg_duration_per_item", avgLatencyPerItem),
		zap.Bool("success", success),
		zap.Error(err))

	return err
}

// GetLatencyContext extracts latency tracking information from context
func GetLatencyContext(ctx context.Context) (operation string, startTime time.Time, ok bool) {
	if op, opOk := ctx.Value("latency_operation").(string); opOk {
		if start, startOk := ctx.Value("latency_start").(time.Time); startOk {
			return op, start, true
		}
	}
	return "", time.Time{}, false
}

// WithLatencyContext adds latency tracking information to context
func WithLatencyContext(ctx context.Context, operation string) context.Context {
	ctx = context.WithValue(ctx, latencyOperationKey, operation)
	ctx = context.WithValue(ctx, latencyStartKey, time.Now())
	return ctx
}

// RecordRepositoryLatency records latency for repository operations
func RecordRepositoryLatency(ctx context.Context, repository, method string, duration time.Duration, success bool, logger *zap.Logger, recorder MetricsRecorder) {
	if recorder == nil {
		return
	}

	dimensions := map[string]string{
		"repository":     repository,
		"method":         method,
		"operation_type": "repository",
	}

	if err := recorder.RecordLatency(ctx, fmt.Sprintf("%s.%s", repository, method), "main", duration, success, dimensions); err != nil {
		logger.Warn("failed to record repository latency",
			zap.String("repository", repository),
			zap.String("method", method),
			zap.Duration("duration", duration),
			zap.Bool("success", success),
			zap.Error(err))
	}

	// Log repository method performance
	logLevel := zap.DebugLevel
	if duration > 2*time.Second {
		logLevel = zap.WarnLevel
	} else if duration > 1*time.Second {
		logLevel = zap.InfoLevel
	}

	logger.Log(logLevel, "Repository method completed",
		zap.String("repository", repository),
		zap.String("method", method),
		zap.Duration("duration", duration),
		zap.Bool("success", success))
}

// DynamORMMetrics provides pre-configured metrics tracking for common patterns
type DynamORMMetrics struct {
	tracker   *DynamORMTracker
	tableName string
}

// NewDynamORMMetrics creates pre-configured DynamORM metrics tracker
func NewDynamORMMetrics(db core.DB, tableName string, logger *zap.Logger, recorder MetricsRecorder) *DynamORMMetrics {
	tracker := NewDynamORMTracker(db, logger, recorder)
	return &DynamORMMetrics{
		tracker:   tracker,
		tableName: tableName,
	}
}

// TrackRepositoryMethod tracks a repository method execution
func (dm *DynamORMMetrics) TrackRepositoryMethod(ctx context.Context, repository, method string, fn func() error) error {
	operation := fmt.Sprintf("%s.%s", repository, method)
	return dm.tracker.TrackQuery(ctx, operation, dm.tableName, fn)
}

// TrackCreate tracks a create operation
func (dm *DynamORMMetrics) TrackCreate(ctx context.Context, _ string, fn func() error) error {
	return dm.tracker.TrackCreate(ctx, dm.tableName, fn)
}

// TrackUpdate tracks an update operation
func (dm *DynamORMMetrics) TrackUpdate(ctx context.Context, _ string, fn func() error) error {
	return dm.tracker.TrackUpdate(ctx, dm.tableName, fn)
}

// TrackDelete tracks a delete operation
func (dm *DynamORMMetrics) TrackDelete(ctx context.Context, _ string, fn func() error) error {
	return dm.tracker.TrackDelete(ctx, dm.tableName, fn)
}

// TrackQuery tracks a query operation
func (dm *DynamORMMetrics) TrackQuery(ctx context.Context, repository, method string, fn func() error) error {
	operation := fmt.Sprintf("%s.%s", repository, method)
	return dm.tracker.TrackQuery(ctx, operation, dm.tableName, fn)
}

// TrackBatch tracks a batch operation
func (dm *DynamORMMetrics) TrackBatch(ctx context.Context, repository string, operation string, count int, fn func() error) error {
	op := fmt.Sprintf("%s.%s", repository, operation)
	return dm.tracker.TrackBatch(ctx, op, dm.tableName, count, fn)
}
