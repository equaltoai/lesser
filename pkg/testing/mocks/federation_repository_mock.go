// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockFederationRepository is a mock implementation of interfaces.FederationRepository
// using testify/mock for expectation-based testing.
type MockFederationRepository struct {
	mock.Mock
}

// NewMockFederationRepository creates a new mock federation repository
func NewMockFederationRepository() *MockFederationRepository {
	return &MockFederationRepository{}
}

// GetInstanceInfo mocks the GetInstanceInfo method
func (m *MockFederationRepository) GetInstanceInfo(ctx context.Context, domain string) (*storage.InstanceInfo, error) {
	args := m.Called(ctx, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceInfo), args.Error(1)
}

// UpsertInstanceInfo mocks the UpsertInstanceInfo method
func (m *MockFederationRepository) UpsertInstanceInfo(ctx context.Context, info *storage.InstanceInfo) error {
	args := m.Called(ctx, info)
	return args.Error(0)
}

// GetKnownInstances mocks the GetKnownInstances method
func (m *MockFederationRepository) GetKnownInstances(ctx context.Context, limit int, cursor string) ([]*storage.InstanceInfo, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.InstanceInfo), args.String(1), args.Error(2)
}

// GetFederationStatistics mocks the GetFederationStatistics method
func (m *MockFederationRepository) GetFederationStatistics(ctx context.Context, startTime, endTime time.Time) (*storage.FederationStats, error) {
	args := m.Called(ctx, startTime, endTime)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.FederationStats), args.Error(1)
}

// GetInstanceStats mocks the GetInstanceStats method
func (m *MockFederationRepository) GetInstanceStats(ctx context.Context, domain string) (*storage.InstanceStats, error) {
	args := m.Called(ctx, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceStats), args.Error(1)
}


// RecordFederationActivity mocks the RecordFederationActivity method
func (m *MockFederationRepository) RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

// GetFederationCosts mocks the GetFederationCosts method
func (m *MockFederationRepository) GetFederationCosts(ctx context.Context, startTime, endTime time.Time, limit int, cursor string) ([]*storage.FederationCost, string, error) {
	args := m.Called(ctx, startTime, endTime, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.FederationCost), args.String(1), args.Error(2)
}

// GetInstanceHealthReport mocks the GetInstanceHealthReport method
func (m *MockFederationRepository) GetInstanceHealthReport(ctx context.Context, domain string, period time.Duration) (*storage.InstanceHealthReport, error) {
	args := m.Called(ctx, domain, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceHealthReport), args.Error(1)
}

// GetCostProjections mocks the GetCostProjections method
func (m *MockFederationRepository) GetCostProjections(ctx context.Context, period string) (*storage.CostProjection, error) {
	args := m.Called(ctx, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.CostProjection), args.Error(1)
}

// GetFederationNodes mocks the GetFederationNodes method
func (m *MockFederationRepository) GetFederationNodes(ctx context.Context, depth int) ([]*storage.FederationNode, error) {
	args := m.Called(ctx, depth)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FederationNode), args.Error(1)
}

// GetFederationNodesByHealth mocks the GetFederationNodesByHealth method
func (m *MockFederationRepository) GetFederationNodesByHealth(ctx context.Context, healthStatus string, limit int) ([]*storage.FederationNode, error) {
	args := m.Called(ctx, healthStatus, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FederationNode), args.Error(1)
}

// GetFederationEdges mocks the GetFederationEdges method
func (m *MockFederationRepository) GetFederationEdges(ctx context.Context, domains []string) ([]*storage.FederationEdge, error) {
	args := m.Called(ctx, domains)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FederationEdge), args.Error(1)
}

// GetInstanceMetadata mocks the GetInstanceMetadata method
func (m *MockFederationRepository) GetInstanceMetadata(ctx context.Context, domain string) (*storage.InstanceMetadata, error) {
	args := m.Called(ctx, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceMetadata), args.Error(1)
}

// CalculateFederationClusters mocks the CalculateFederationClusters method
func (m *MockFederationRepository) CalculateFederationClusters(ctx context.Context) ([]*storage.InstanceCluster, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.InstanceCluster), args.Error(1)
}

// Ensure MockFederationRepository implements interfaces.FederationRepository
var _ interfaces.FederationRepository = (*MockFederationRepository)(nil)
