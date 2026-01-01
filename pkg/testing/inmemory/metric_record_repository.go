// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// MetricRecordRepository is a thread-safe in-memory implementation of interfaces.MetricRecordRepository.
type MetricRecordRepository struct {
	mu      sync.RWMutex
	records map[string]*models.MetricRecord // keyed by "metricType#bucket#timestamp"
}

// NewMetricRecordRepository creates a new in-memory metric record repository.
func NewMetricRecordRepository() *MetricRecordRepository {
	return &MetricRecordRepository{
		records: make(map[string]*models.MetricRecord),
	}
}

// makeKey creates a unique key for a metric record.
func makeMetricKey(metricType, timestamp string) string {
	return fmt.Sprintf("%s#%s", metricType, timestamp)
}

// ===== Core Metric Record Operations =====

// CreateMetricRecord creates a new metric record.
func (r *MetricRecordRepository) CreateMetricRecord(_ context.Context, record *models.MetricRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if record == nil {
		return fmt.Errorf("record is required")
	}

	key := makeMetricKey(record.MetricType, record.Timestamp.Format(time.RFC3339))
	if _, exists := r.records[key]; exists {
		return storage.ErrAlreadyExists
	}

	r.records[key] = record
	return nil
}

// BatchCreateMetricRecords creates multiple metric records efficiently.
func (r *MetricRecordRepository) BatchCreateMetricRecords(ctx context.Context, records []*models.MetricRecord) error {
	for _, record := range records {
		if err := r.CreateMetricRecord(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

// GetMetricRecord retrieves a single metric record by its keys.
func (r *MetricRecordRepository) GetMetricRecord(_ context.Context, metricType, _, timestamp string) (*models.MetricRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := makeMetricKey(metricType, timestamp)
	record, exists := r.records[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return record, nil
}

// UpdateMetricRecord updates an existing metric record.
func (r *MetricRecordRepository) UpdateMetricRecord(_ context.Context, record *models.MetricRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if record == nil {
		return fmt.Errorf("record is required")
	}

	key := makeMetricKey(record.MetricType, record.Timestamp.Format(time.RFC3339))
	if _, exists := r.records[key]; !exists {
		return storage.ErrNotFound
	}

	r.records[key] = record
	return nil
}

// DeleteMetricRecord deletes a metric record by its keys.
func (r *MetricRecordRepository) DeleteMetricRecord(_ context.Context, metricType, _, timestamp string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := makeMetricKey(metricType, timestamp)
	if _, exists := r.records[key]; !exists {
		return storage.ErrNotFound
	}

	delete(r.records, key)
	return nil
}

// ===== Query Operations =====

// GetMetricsByService queries metrics by service within a time range using GSI1.
func (r *MetricRecordRepository) GetMetricsByService(_ context.Context, serviceName string, startTime, endTime time.Time) ([]*models.MetricRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.MetricRecord

	for _, record := range r.records {
		if record.ServiceName != serviceName {
			continue
		}
		if record.Timestamp.Before(startTime) || record.Timestamp.After(endTime) {
			continue
		}
		results = append(results, record)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.After(results[j].Timestamp)
	})

	return results, nil
}

// GetMetricsByType queries metrics by type within a time range using GSI2.
func (r *MetricRecordRepository) GetMetricsByType(_ context.Context, metricType string, startTime, endTime time.Time) ([]*models.MetricRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.MetricRecord

	for _, record := range r.records {
		if record.MetricType != metricType {
			continue
		}
		if record.Timestamp.Before(startTime) || record.Timestamp.After(endTime) {
			continue
		}
		results = append(results, record)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.After(results[j].Timestamp)
	})

	return results, nil
}

// GetMetricsByDate queries metrics by date and service using GSI3.
func (r *MetricRecordRepository) GetMetricsByDate(_ context.Context, date time.Time, serviceName string) ([]*models.MetricRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dateStr := date.Format("2006-01-02")
	var results []*models.MetricRecord

	for _, record := range r.records {
		if record.Timestamp.Format("2006-01-02") != dateStr {
			continue
		}
		if serviceName != "" && record.ServiceName != serviceName {
			continue
		}
		results = append(results, record)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.After(results[j].Timestamp)
	})

	return results, nil
}

// GetMetricsByAggregationLevel queries metrics by aggregation level within a time range using GSI4.
func (r *MetricRecordRepository) GetMetricsByAggregationLevel(_ context.Context, level string, startTime, endTime time.Time) ([]*models.MetricRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.MetricRecord

	for _, record := range r.records {
		if record.AggregationLevel != level {
			continue
		}
		if record.Timestamp.Before(startTime) || record.Timestamp.After(endTime) {
			continue
		}
		results = append(results, record)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.After(results[j].Timestamp)
	})

	return results, nil
}

// ===== Statistics Operations =====

// GetServiceMetricsStats calculates statistics for a service's metrics.
func (r *MetricRecordRepository) GetServiceMetricsStats(ctx context.Context, serviceName string, metricType string, startTime, endTime time.Time) (*interfaces.MetricRecordStats, error) {
	records, err := r.GetMetricsByService(ctx, serviceName, startTime, endTime)
	if err != nil {
		return nil, err
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

	stats := &interfaces.MetricRecordStats{
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

// ===== Test Helper Methods =====

// Clear clears all data.
func (r *MetricRecordRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = make(map[string]*models.MetricRecord)
}

// Count returns the number of records.
func (r *MetricRecordRepository) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.records)
}

// Ensure MetricRecordRepository implements interfaces.MetricRecordRepository
var _ interfaces.MetricRecordRepository = (*MetricRecordRepository)(nil)
