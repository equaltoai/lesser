// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockTrustRepository is a mock implementation of interfaces.TrustRepository
// using testify/mock for expectation-based testing.
type MockTrustRepository struct {
	mock.Mock
}

// NewMockTrustRepository creates a new mock trust repository
func NewMockTrustRepository() *MockTrustRepository {
	return &MockTrustRepository{}
}

// ===== Trust Relationship Operations =====

// CreateTrustRelationship mocks the CreateTrustRelationship method
func (m *MockTrustRepository) CreateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	args := m.Called(ctx, relationship)
	return args.Error(0)
}

// GetTrustRelationship mocks the GetTrustRelationship method
func (m *MockTrustRepository) GetTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error) {
	args := m.Called(ctx, trusterID, trusteeID, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.TrustRelationship), args.Error(1)
}

// UpdateTrustRelationship mocks the UpdateTrustRelationship method
func (m *MockTrustRepository) UpdateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	args := m.Called(ctx, relationship)
	return args.Error(0)
}

// DeleteTrustRelationship mocks the DeleteTrustRelationship method
func (m *MockTrustRepository) DeleteTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) error {
	args := m.Called(ctx, trusterID, trusteeID, category)
	return args.Error(0)
}

// GetTrustRelationships mocks the GetTrustRelationships method
func (m *MockTrustRepository) GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	args := m.Called(ctx, trusterID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TrustRelationship), args.String(1), args.Error(2)
}

// GetTrustedByRelationships mocks the GetTrustedByRelationships method
func (m *MockTrustRepository) GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	args := m.Called(ctx, trusteeID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TrustRelationship), args.String(1), args.Error(2)
}

// GetAllTrustRelationships mocks the GetAllTrustRelationships method
func (m *MockTrustRepository) GetAllTrustRelationships(ctx context.Context, limit int) ([]*storage.TrustRelationship, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrustRelationship), args.Error(1)
}

// ===== Trust Score Operations =====

// GetTrustScore mocks the GetTrustScore method
func (m *MockTrustRepository) GetTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error) {
	args := m.Called(ctx, actorID, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.TrustScore), args.Error(1)
}

// UpdateTrustScore mocks the UpdateTrustScore method
func (m *MockTrustRepository) UpdateTrustScore(ctx context.Context, score *storage.TrustScore) error {
	args := m.Called(ctx, score)
	return args.Error(0)
}

// GetUserTrustScore mocks the GetUserTrustScore method
func (m *MockTrustRepository) GetUserTrustScore(ctx context.Context, userID string) (float64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(float64), args.Error(1)
}

// ===== Trust Update Operations =====

// RecordTrustUpdate mocks the RecordTrustUpdate method
func (m *MockTrustRepository) RecordTrustUpdate(ctx context.Context, update *storage.TrustUpdate) error {
	args := m.Called(ctx, update)
	return args.Error(0)
}

// Ensure MockTrustRepository implements interfaces.TrustRepository
var _ interfaces.TrustRepository = (*MockTrustRepository)(nil)
