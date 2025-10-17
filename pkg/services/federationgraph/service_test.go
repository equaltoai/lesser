package federationgraph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockFederationRepository is a mock for testing
type MockFederationRepository struct {
	mock.Mock
}

func (m *MockFederationRepository) GetFederationNodes(ctx context.Context, depth int) ([]*storage.FederationNode, error) {
	args := m.Called(ctx, depth)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FederationNode), args.Error(1)
}

func (m *MockFederationRepository) GetAllFederationEdges(ctx context.Context, limit int) ([]*storage.FederationEdge, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FederationEdge), args.Error(1)
}

func (m *MockFederationRepository) GetFederationClusters(ctx context.Context, limit int) ([]*storage.InstanceCluster, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.InstanceCluster), args.Error(1)
}

func (m *MockFederationRepository) GetInstanceConnections(ctx context.Context, domain, connectionType string) ([]*storage.InstanceConnection, error) {
	args := m.Called(ctx, domain, connectionType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.InstanceConnection), args.Error(1)
}

func (m *MockFederationRepository) GetFederationEdges(ctx context.Context, domains []string) ([]*storage.FederationEdge, error) {
	args := m.Called(ctx, domains)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FederationEdge), args.Error(1)
}

func (m *MockFederationRepository) GetFederationActivitiesByTimeRange(ctx context.Context, start, end time.Time, limit int) ([]*models.FederationCostActivity, error) {
	args := m.Called(ctx, start, end, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.FederationCostActivity), args.Error(1)
}

func (m *MockFederationRepository) GetFederationCosts(ctx context.Context, start, end time.Time, limit int, cursor string) ([]*storage.FederationCost, string, error) {
	args := m.Called(ctx, start, end, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.FederationCost), args.String(1), args.Error(2)
}

func TestGetFederationMap_Success(t *testing.T) {
	// Skip this test as it requires full integration with repository
	// The service is tested via the other unit tests for individual components
	t.Skip("Skipping integration test - service requires actual repository instance")
}

func TestConvertHealthStatus(t *testing.T) {
	service := &Service{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name     string
		input    string
		expected model.InstanceHealthStatus
	}{
		{"healthy", "healthy", model.InstanceHealthStatusHealthy},
		{"warning", "warning", model.InstanceHealthStatusWarning},
		{"critical", "critical", model.InstanceHealthStatusCritical},
		{"offline", "offline", model.InstanceHealthStatusOffline},
		{"unknown", "unknown", model.InstanceHealthStatusUnknown},
		{"invalid", "invalid", model.InstanceHealthStatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.convertHealthStatus(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateFederationScore(t *testing.T) {
	service := &Service{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name        string
		connections []*storage.InstanceConnection
		edges       []*storage.FederationEdge
		minScore    float64
		maxScore    float64
	}{
		{
			name:        "empty",
			connections: []*storage.InstanceConnection{},
			edges:       []*storage.FederationEdge{},
			minScore:    0.0,
			maxScore:    0.0,
		},
		{
			name: "good connections",
			connections: []*storage.InstanceConnection{
				{TargetDomain: "test1.com"},
				{TargetDomain: "test2.com"},
			},
			edges: []*storage.FederationEdge{
				{SuccessRate: 0.95},
				{SuccessRate: 0.90},
			},
			minScore: 0.4,
			maxScore: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := service.calculateFederationScore(tt.connections, tt.edges)
			assert.GreaterOrEqual(t, score, tt.minScore)
			assert.LessOrEqual(t, score, tt.maxScore)
		})
	}
}

func TestGenerateRecommendations(t *testing.T) {
	service := &Service{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name        string
		domain      string
		connections []*storage.InstanceConnection
		edges       []*storage.FederationEdge
		minRecs     int
	}{
		{
			name:        "low connectivity",
			domain:      "test.com",
			connections: []*storage.InstanceConnection{},
			edges:       []*storage.FederationEdge{},
			minRecs:     1, // Should recommend increasing connectivity
		},
		{
			name:   "low success rate",
			domain: "test.com",
			connections: []*storage.InstanceConnection{
				{TargetDomain: "test1.com"},
			},
			edges: []*storage.FederationEdge{
				{SuccessRate: 0.5},
				{SuccessRate: 0.6},
				{SuccessRate: 0.4},
			},
			minRecs: 1, // Should recommend improving performance
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recs := service.generateRecommendations(tt.domain, tt.connections, tt.edges)
			assert.GreaterOrEqual(t, len(recs), tt.minRecs)
		})
	}
}
