// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// MetricRecordStats represents statistics for metric records.
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

// MetricRecordRepository defines the interface for metric record operations.
// This handles new reporting table schema with extensive indexing for metrics storage.
type MetricRecordRepository interface {
	// ===== Core Metric Record Operations =====

	// CreateMetricRecord creates a new metric record
	CreateMetricRecord(ctx context.Context, record *models.MetricRecord) error

	// BatchCreateMetricRecords creates multiple metric records efficiently
	BatchCreateMetricRecords(ctx context.Context, records []*models.MetricRecord) error

	// GetMetricRecord retrieves a single metric record by its keys
	GetMetricRecord(ctx context.Context, metricType, bucket, timestamp string) (*models.MetricRecord, error)

	// UpdateMetricRecord updates an existing metric record
	UpdateMetricRecord(ctx context.Context, record *models.MetricRecord) error

	// DeleteMetricRecord deletes a metric record by its keys
	DeleteMetricRecord(ctx context.Context, metricType, bucket, timestamp string) error

	// ===== Query Operations =====

	// GetMetricsByService queries metrics by service within a time range using GSI1
	GetMetricsByService(ctx context.Context, serviceName string, startTime, endTime time.Time) ([]*models.MetricRecord, error)

	// GetMetricsByType queries metrics by type within a time range using GSI2
	GetMetricsByType(ctx context.Context, metricType string, startTime, endTime time.Time) ([]*models.MetricRecord, error)

	// GetMetricsByDate queries metrics by date and service using GSI3
	GetMetricsByDate(ctx context.Context, date time.Time, serviceName string) ([]*models.MetricRecord, error)

	// GetMetricsByAggregationLevel queries metrics by aggregation level within a time range using GSI4
	GetMetricsByAggregationLevel(ctx context.Context, level string, startTime, endTime time.Time) ([]*models.MetricRecord, error)

	// ===== Statistics Operations =====

	// GetServiceMetricsStats calculates statistics for a service's metrics
	GetServiceMetricsStats(ctx context.Context, serviceName string, metricType string, startTime, endTime time.Time) (*MetricRecordStats, error)
}
