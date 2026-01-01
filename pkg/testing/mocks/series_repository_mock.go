// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockSeriesRepository is a mock implementation of interfaces.SeriesRepository
// using testify/mock for expectation-based testing.
type MockSeriesRepository struct {
	mock.Mock
}

// NewMockSeriesRepository creates a new mock series repository
func NewMockSeriesRepository() *MockSeriesRepository {
	return &MockSeriesRepository{}
}

// ===== Core CRUD Operations =====

// CreateSeries mocks the CreateSeries method
func (m *MockSeriesRepository) CreateSeries(ctx context.Context, series *models.Series) error {
	args := m.Called(ctx, series)
	return args.Error(0)
}

// GetSeries mocks the GetSeries method
func (m *MockSeriesRepository) GetSeries(ctx context.Context, authorID, seriesID string) (*models.Series, error) {
	args := m.Called(ctx, authorID, seriesID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Series), args.Error(1)
}

// ===== List Operations =====

// ListSeriesByAuthor mocks the ListSeriesByAuthor method
func (m *MockSeriesRepository) ListSeriesByAuthor(ctx context.Context, authorID string, limit int) ([]*models.Series, error) {
	args := m.Called(ctx, authorID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Series), args.Error(1)
}

// ListSeriesByAuthorPaginated mocks the ListSeriesByAuthorPaginated method
func (m *MockSeriesRepository) ListSeriesByAuthorPaginated(ctx context.Context, authorID string, limit int, cursor string) ([]*models.Series, string, error) {
	args := m.Called(ctx, authorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Series), args.String(1), args.Error(2)
}

// ===== Count Operations =====

// UpdateArticleCount mocks the UpdateArticleCount method
func (m *MockSeriesRepository) UpdateArticleCount(ctx context.Context, authorID string, seriesID string, delta int) error {
	args := m.Called(ctx, authorID, seriesID, delta)
	return args.Error(0)
}

// Update mocks the Update method
func (m *MockSeriesRepository) Update(ctx context.Context, series *models.Series) error {
	args := m.Called(ctx, series)
	return args.Error(0)
}

// Delete mocks the Delete method
func (m *MockSeriesRepository) Delete(ctx context.Context, pk, sk string) error {
	args := m.Called(ctx, pk, sk)
	return args.Error(0)
}

// Ensure MockSeriesRepository implements interfaces.SeriesRepository
var _ interfaces.SeriesRepository = (*MockSeriesRepository)(nil)
