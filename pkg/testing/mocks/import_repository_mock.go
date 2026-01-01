// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockImportRepository is a mock implementation of interfaces.ImportRepository
// using testify/mock for expectation-based testing.
type MockImportRepository struct {
	mock.Mock
}

// NewMockImportRepository creates a new mock import repository
func NewMockImportRepository() *MockImportRepository {
	return &MockImportRepository{}
}

// ===== Core Import Operations =====

// CreateImport mocks the CreateImport method
func (m *MockImportRepository) CreateImport(ctx context.Context, importRecord *models.Import) error {
	args := m.Called(ctx, importRecord)
	return args.Error(0)
}

// GetImport mocks the GetImport method
func (m *MockImportRepository) GetImport(ctx context.Context, importID string) (*models.Import, error) {
	args := m.Called(ctx, importID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Import), args.Error(1)
}

// UpdateImportStatus mocks the UpdateImportStatus method
func (m *MockImportRepository) UpdateImportStatus(ctx context.Context, importID, status string, completionData map[string]any, errorMsg string) error {
	args := m.Called(ctx, importID, status, completionData, errorMsg)
	return args.Error(0)
}

// UpdateImportProgress mocks the UpdateImportProgress method
func (m *MockImportRepository) UpdateImportProgress(ctx context.Context, importID string, progress int) error {
	args := m.Called(ctx, importID, progress)
	return args.Error(0)
}

// ===== User Import Operations =====

// GetImportsForUser mocks the GetImportsForUser method
func (m *MockImportRepository) GetImportsForUser(ctx context.Context, username string, limit int, cursor string) ([]*models.Import, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Import), args.String(1), args.Error(2)
}

// GetUserImportsByStatus mocks the GetUserImportsByStatus method
func (m *MockImportRepository) GetUserImportsByStatus(ctx context.Context, username string, statuses []string) ([]*models.Import, error) {
	args := m.Called(ctx, username, statuses)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Import), args.Error(1)
}

// ===== Cost Tracking Operations =====

// CreateImportCostTracking mocks the CreateImportCostTracking method
func (m *MockImportRepository) CreateImportCostTracking(ctx context.Context, costTracking *models.ImportCostTracking) error {
	args := m.Called(ctx, costTracking)
	return args.Error(0)
}

// GetImportCostTracking mocks the GetImportCostTracking method
func (m *MockImportRepository) GetImportCostTracking(ctx context.Context, importID string) ([]*models.ImportCostTracking, error) {
	args := m.Called(ctx, importID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ImportCostTracking), args.Error(1)
}

// GetUserImportCosts mocks the GetUserImportCosts method
func (m *MockImportRepository) GetUserImportCosts(ctx context.Context, username string, startDate, endDate time.Time, limit int) ([]*models.ImportCostTracking, error) {
	args := m.Called(ctx, username, startDate, endDate, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ImportCostTracking), args.Error(1)
}

// GetImportCostsByDateRange mocks the GetImportCostsByDateRange method
func (m *MockImportRepository) GetImportCostsByDateRange(ctx context.Context, startDate, endDate time.Time, limit int) ([]*models.ImportCostTracking, error) {
	args := m.Called(ctx, startDate, endDate, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ImportCostTracking), args.Error(1)
}

// GetImportCostSummary mocks the GetImportCostSummary method
func (m *MockImportRepository) GetImportCostSummary(ctx context.Context, username string, startDate, endDate time.Time) (*models.ImportCostSummary, error) {
	args := m.Called(ctx, username, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ImportCostSummary), args.Error(1)
}

// GetHighCostImports mocks the GetHighCostImports method
func (m *MockImportRepository) GetHighCostImports(ctx context.Context, thresholdMicroCents int64, startDate, endDate time.Time, limit int) ([]*models.ImportCostTracking, error) {
	args := m.Called(ctx, thresholdMicroCents, startDate, endDate, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ImportCostTracking), args.Error(1)
}

// ===== Budget Management Operations =====

// CreateImportBudget mocks the CreateImportBudget method
func (m *MockImportRepository) CreateImportBudget(ctx context.Context, budget *models.ImportBudget) error {
	args := m.Called(ctx, budget)
	return args.Error(0)
}

// UpdateImportBudget mocks the UpdateImportBudget method
func (m *MockImportRepository) UpdateImportBudget(ctx context.Context, budget *models.ImportBudget) error {
	args := m.Called(ctx, budget)
	return args.Error(0)
}

// GetImportBudget mocks the GetImportBudget method
func (m *MockImportRepository) GetImportBudget(ctx context.Context, username, period string) (*models.ImportBudget, error) {
	args := m.Called(ctx, username, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ImportBudget), args.Error(1)
}

// CheckBudgetLimits mocks the CheckBudgetLimits method
func (m *MockImportRepository) CheckBudgetLimits(ctx context.Context, username string, importCostMicroCents, exportCostMicroCents int64) (*models.ImportBudget, bool, error) {
	args := m.Called(ctx, username, importCostMicroCents, exportCostMicroCents)
	if args.Get(0) == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return args.Get(0).(*models.ImportBudget), args.Bool(1), args.Error(2)
}

// UpdateBudgetUsage mocks the UpdateBudgetUsage method
func (m *MockImportRepository) UpdateBudgetUsage(ctx context.Context, username, period string, importCostMicroCents, exportCostMicroCents int64) error {
	args := m.Called(ctx, username, period, importCostMicroCents, exportCostMicroCents)
	return args.Error(0)
}

// Ensure MockImportRepository implements interfaces.ImportRepository
var _ interfaces.ImportRepository = (*MockImportRepository)(nil)
