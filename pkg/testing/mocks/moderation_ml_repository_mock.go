// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockModerationMLRepository is a mock implementation of interfaces.ModerationMLRepository
// using testify/mock for expectation-based testing.
type MockModerationMLRepository struct {
	mock.Mock
}

// NewMockModerationMLRepository creates a new mock moderation ML repository
func NewMockModerationMLRepository() *MockModerationMLRepository {
	return &MockModerationMLRepository{}
}

// ===== Sample Operations =====

// CreateSample mocks the CreateSample method
func (m *MockModerationMLRepository) CreateSample(ctx context.Context, sample *models.ModerationSample) error {
	args := m.Called(ctx, sample)
	return args.Error(0)
}

// GetSample mocks the GetSample method
func (m *MockModerationMLRepository) GetSample(ctx context.Context, sampleID string) (*models.ModerationSample, error) {
	args := m.Called(ctx, sampleID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ModerationSample), args.Error(1)
}

// ListSamplesByLabel mocks the ListSamplesByLabel method
func (m *MockModerationMLRepository) ListSamplesByLabel(ctx context.Context, label string, limit int) ([]*models.ModerationSample, error) {
	args := m.Called(ctx, label, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ModerationSample), args.Error(1)
}

// ListSamplesByReviewer mocks the ListSamplesByReviewer method
func (m *MockModerationMLRepository) ListSamplesByReviewer(ctx context.Context, reviewerID string, limit int) ([]*models.ModerationSample, error) {
	args := m.Called(ctx, reviewerID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ModerationSample), args.Error(1)
}

// ===== Model Version Operations =====

// CreateModelVersion mocks the CreateModelVersion method
func (m *MockModerationMLRepository) CreateModelVersion(ctx context.Context, version *models.ModerationModelVersion) error {
	args := m.Called(ctx, version)
	return args.Error(0)
}

// GetModelVersion mocks the GetModelVersion method
func (m *MockModerationMLRepository) GetModelVersion(ctx context.Context, versionID string) (*models.ModerationModelVersion, error) {
	args := m.Called(ctx, versionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ModerationModelVersion), args.Error(1)
}

// GetActiveModelVersion mocks the GetActiveModelVersion method
func (m *MockModerationMLRepository) GetActiveModelVersion(ctx context.Context) (*models.ModerationModelVersion, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ModerationModelVersion), args.Error(1)
}

// UpdateModelVersion mocks the UpdateModelVersion method
func (m *MockModerationMLRepository) UpdateModelVersion(ctx context.Context, version *models.ModerationModelVersion) error {
	args := m.Called(ctx, version)
	return args.Error(0)
}

// ===== Effectiveness Metrics Operations =====

// CreateEffectivenessMetric mocks the CreateEffectivenessMetric method
func (m *MockModerationMLRepository) CreateEffectivenessMetric(ctx context.Context, metric *models.ModerationEffectivenessMetric) error {
	args := m.Called(ctx, metric)
	return args.Error(0)
}

// GetEffectivenessMetric mocks the GetEffectivenessMetric method
func (m *MockModerationMLRepository) GetEffectivenessMetric(ctx context.Context, patternID, period string, startTime time.Time) (*models.ModerationEffectivenessMetric, error) {
	args := m.Called(ctx, patternID, period, startTime)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ModerationEffectivenessMetric), args.Error(1)
}

// ListEffectivenessMetricsByPattern mocks the ListEffectivenessMetricsByPattern method
func (m *MockModerationMLRepository) ListEffectivenessMetricsByPattern(ctx context.Context, patternID string, limit int) ([]*models.ModerationEffectivenessMetric, error) {
	args := m.Called(ctx, patternID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ModerationEffectivenessMetric), args.Error(1)
}

// ListEffectivenessMetricsByPeriod mocks the ListEffectivenessMetricsByPeriod method
func (m *MockModerationMLRepository) ListEffectivenessMetricsByPeriod(ctx context.Context, period string, limit int) ([]*models.ModerationEffectivenessMetric, error) {
	args := m.Called(ctx, period, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ModerationEffectivenessMetric), args.Error(1)
}

// Ensure MockModerationMLRepository implements interfaces.ModerationMLRepository
var _ interfaces.ModerationMLRepository = (*MockModerationMLRepository)(nil)
