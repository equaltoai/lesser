// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockMediaPopularityRepository is a mock implementation of interfaces.MediaPopularityRepository
// using testify/mock for expectation-based testing.
type MockMediaPopularityRepository struct {
	mock.Mock
}

// NewMockMediaPopularityRepository creates a new mock media popularity repository
func NewMockMediaPopularityRepository() *MockMediaPopularityRepository {
	return &MockMediaPopularityRepository{}
}

// ===== Core Popularity Operations =====

// UpsertPopularity mocks the UpsertPopularity method
func (m *MockMediaPopularityRepository) UpsertPopularity(ctx context.Context, popularity *models.MediaPopularity) error {
	args := m.Called(ctx, popularity)
	return args.Error(0)
}

// GetPopularityForMedia mocks the GetPopularityForMedia method
func (m *MockMediaPopularityRepository) GetPopularityForMedia(ctx context.Context, mediaID, period string) (*models.MediaPopularity, error) {
	args := m.Called(ctx, mediaID, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.MediaPopularity), args.Error(1)
}

// ===== Popular Media Queries =====

// GetPopularMediaByPeriod mocks the GetPopularMediaByPeriod method
func (m *MockMediaPopularityRepository) GetPopularMediaByPeriod(ctx context.Context, period string, limit int, cursor *string) ([]*models.MediaPopularity, error) {
	args := m.Called(ctx, period, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaPopularity), args.Error(1)
}

// ===== View Count Operations =====

// IncrementViewCount mocks the IncrementViewCount method
func (m *MockMediaPopularityRepository) IncrementViewCount(ctx context.Context, mediaID, period string, incrementBy int64) error {
	args := m.Called(ctx, mediaID, period, incrementBy)
	return args.Error(0)
}

// Ensure MockMediaPopularityRepository implements interfaces.MediaPopularityRepository
var _ interfaces.MediaPopularityRepository = (*MockMediaPopularityRepository)(nil)
