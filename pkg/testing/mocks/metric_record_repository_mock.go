// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockMetricRecordRepository is a mock implementation of interfaces.MetricRecordRepository
// using testify/mock for expectation-based testing.
type MockMetricRecordRepository struct {
	mock.Mock
}

// NewMockMetricRecordRepository creates a new mock metric record repository
func NewMockMetricRecordRepository() *MockMetricRecordRepository {
	return &MockMetricRecordRepository{}
}

// ===== Core Metric Record Operations =====

// CreateMetricRecord mocks the CreateMetricRecord method
func (m *MockMetricRecordRepository) CreateMetricRecord(ctx context.Context, record *models.MetricRecord) error {
	args := m.Called(ctx, record)
	return args.Error(0)
}

// BatchCreateMetricRecords mocks the BatchCreateMetricRecords method
func (m *MockMetricRecordRepository) BatchCreateMetricRecords(ctx context.Context, records []*models.MetricRecord) error {
	args := m.Called(ctx, records)
	return args.Error(0)
}

// GetMetricRecord mocks the GetMetricRecord method
func (m *MockMetricRecordRepository) GetMetricRecord(ctx context.Context, metricType, bucket, timestamp string) (*models.MetricRecord, error) {
	args := m.Called(ctx, metricType, bucket, timestamp)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.MetricRecord), args.Error(1)
}

// UpdateMetricRecord mocks the UpdateMetricRecord method
func (m *MockMetricRecordRepository) UpdateMetricRecord(ctx context.Context, record *models.MetricRecord) error {
	args := m.Called(ctx, record)
	return args.Error(0)
}

// DeleteMetricRecord mocks the DeleteMetricRecord method
func (m *MockMetricRecordRepository) DeleteMetricRecord(ctx context.Context, metricType, bucket, timestamp string) error {
	args := m.Called(ctx, metricType, bucket, timestamp)
	return args.Error(0)
}

// ===== Query Operations =====

// GetMetricsByService mocks the GetMetricsByService method
func (m *MockMetricRecordRepository) GetMetricsByService(ctx context.Context, serviceName string, startTime, endTime time.Time) ([]*models.MetricRecord, error) {
	args := m.Called(ctx, serviceName, startTime, endTime)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MetricRecord), args.Error(1)
}

// GetMetricsByType mocks the GetMetricsByType method
func (m *MockMetricRecordRepository) GetMetricsByType(ctx context.Context, metricType string, startTime, endTime time.Time) ([]*models.MetricRecord, error) {
	args := m.Called(ctx, metricType, startTime, endTime)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MetricRecord), args.Error(1)
}

// GetMetricsByDate mocks the GetMetricsByDate method
func (m *MockMetricRecordRepository) GetMetricsByDate(ctx context.Context, date time.Time, serviceName string) ([]*models.MetricRecord, error) {
	args := m.Called(ctx, date, serviceName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MetricRecord), args.Error(1)
}

// GetMetricsByAggregationLevel mocks the GetMetricsByAggregationLevel method
func (m *MockMetricRecordRepository) GetMetricsByAggregationLevel(ctx context.Context, level string, startTime, endTime time.Time) ([]*models.MetricRecord, error) {
	args := m.Called(ctx, level, startTime, endTime)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MetricRecord), args.Error(1)
}

// ===== Statistics Operations =====

// GetServiceMetricsStats mocks the GetServiceMetricsStats method
func (m *MockMetricRecordRepository) GetServiceMetricsStats(ctx context.Context, serviceName string, metricType string, startTime, endTime time.Time) (*interfaces.MetricRecordStats, error) {
	args := m.Called(ctx, serviceName, metricType, startTime, endTime)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.MetricRecordStats), args.Error(1)
}

// Ensure MockMetricRecordRepository implements interfaces.MetricRecordRepository
var _ interfaces.MetricRecordRepository = (*MockMetricRecordRepository)(nil)
