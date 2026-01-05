// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockExportRepository is a mock implementation of interfaces.ExportRepository
// using testify/mock for expectation-based testing.
type MockExportRepository struct {
	mock.Mock
}

// NewMockExportRepository creates a new mock export repository
func NewMockExportRepository() *MockExportRepository {
	return &MockExportRepository{}
}

// ===== Core Export Operations =====

// CreateExport mocks the CreateExport method
func (m *MockExportRepository) CreateExport(ctx context.Context, export *models.Export) error {
	args := m.Called(ctx, export)
	return args.Error(0)
}

// GetExport mocks the GetExport method
func (m *MockExportRepository) GetExport(ctx context.Context, exportID string) (*models.Export, error) {
	args := m.Called(ctx, exportID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Export), args.Error(1)
}

// UpdateExportStatus mocks the UpdateExportStatus method
func (m *MockExportRepository) UpdateExportStatus(ctx context.Context, exportID, status string, completionData map[string]any, errorMsg string) error {
	args := m.Called(ctx, exportID, status, completionData, errorMsg)
	return args.Error(0)
}

// ===== User Export Operations =====

// GetExportsForUser mocks the GetExportsForUser method
func (m *MockExportRepository) GetExportsForUser(ctx context.Context, username string, limit int, cursor string) ([]*models.Export, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Export), args.String(1), args.Error(2)
}

// GetUserExportsByStatus mocks the GetUserExportsByStatus method
func (m *MockExportRepository) GetUserExportsByStatus(ctx context.Context, username string, statuses []string) ([]*models.Export, error) {
	args := m.Called(ctx, username, statuses)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Export), args.Error(1)
}

// ===== Cost Tracking Operations =====

// CreateExportCostTracking mocks the CreateExportCostTracking method
func (m *MockExportRepository) CreateExportCostTracking(ctx context.Context, costTracking *models.ExportCostTracking) error {
	args := m.Called(ctx, costTracking)
	return args.Error(0)
}

// GetExportCostTracking mocks the GetExportCostTracking method
func (m *MockExportRepository) GetExportCostTracking(ctx context.Context, exportID string) ([]*models.ExportCostTracking, error) {
	args := m.Called(ctx, exportID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ExportCostTracking), args.Error(1)
}

// GetUserExportCosts mocks the GetUserExportCosts method
func (m *MockExportRepository) GetUserExportCosts(ctx context.Context, username string, startDate, endDate time.Time, limit int) ([]*models.ExportCostTracking, error) {
	args := m.Called(ctx, username, startDate, endDate, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ExportCostTracking), args.Error(1)
}

// GetExportCostsByDateRange mocks the GetExportCostsByDateRange method
func (m *MockExportRepository) GetExportCostsByDateRange(ctx context.Context, startDate, endDate time.Time, limit int) ([]*models.ExportCostTracking, error) {
	args := m.Called(ctx, startDate, endDate, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ExportCostTracking), args.Error(1)
}

// GetExportCostSummary mocks the GetExportCostSummary method
func (m *MockExportRepository) GetExportCostSummary(ctx context.Context, username string, startDate, endDate time.Time) (*models.ExportCostSummary, error) {
	args := m.Called(ctx, username, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ExportCostSummary), args.Error(1)
}

// GetHighCostExports mocks the GetHighCostExports method
func (m *MockExportRepository) GetHighCostExports(ctx context.Context, thresholdMicroCents int64, startDate, endDate time.Time, limit int) ([]*models.ExportCostTracking, error) {
	args := m.Called(ctx, thresholdMicroCents, startDate, endDate, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ExportCostTracking), args.Error(1)
}

// Ensure MockExportRepository implements interfaces.ExportRepository
var _ interfaces.ExportRepository = (*MockExportRepository)(nil)
