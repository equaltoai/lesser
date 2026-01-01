// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockMarkerRepository is a mock implementation of interfaces.MarkerRepository
// using testify/mock for expectation-based testing.
type MockMarkerRepository struct {
	mock.Mock
}

// NewMockMarkerRepository creates a new mock marker repository
func NewMockMarkerRepository() *MockMarkerRepository {
	return &MockMarkerRepository{}
}

// SaveMarker mocks the SaveMarker method
func (m *MockMarkerRepository) SaveMarker(ctx context.Context, username, timeline string, lastReadID string, version int) error {
	args := m.Called(ctx, username, timeline, lastReadID, version)
	return args.Error(0)
}

// GetMarkers mocks the GetMarkers method
func (m *MockMarkerRepository) GetMarkers(ctx context.Context, username string, timelines []string) (map[string]*storage.Marker, error) {
	args := m.Called(ctx, username, timelines)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]*storage.Marker), args.Error(1)
}

// Ensure MockMarkerRepository implements interfaces.MarkerRepository
var _ interfaces.MarkerRepository = (*MockMarkerRepository)(nil)
