package federationgraph

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeFederationGraphRepo struct {
	lastDepth           int
	nodes               []*storage.FederationNode
	nodesErr            error
	allEdges            []*storage.FederationEdge
	allEdgesErr         error
	clusters            []*storage.InstanceCluster
	clustersErr         error
	lastInstanceDomain  string
	lastConnectionType  string
	connections         []*storage.InstanceConnection
	connectionsErr      error
	instanceEdges       []*storage.FederationEdge
	instanceEdgesErr    error
	lastFlowStart       time.Time
	lastFlowEnd         time.Time
	activities          []*models.FederationCostActivity
	activitiesErr       error
	costs               []*storage.FederationCost
	costsErr            error
	costsCursorObserved string
}

func (f *fakeFederationGraphRepo) GetFederationNodes(_ context.Context, depth int) ([]*storage.FederationNode, error) {
	f.lastDepth = depth
	return f.nodes, f.nodesErr
}

func (f *fakeFederationGraphRepo) GetAllFederationEdges(_ context.Context, _ int) ([]*storage.FederationEdge, error) {
	return f.allEdges, f.allEdgesErr
}

func (f *fakeFederationGraphRepo) GetFederationClusters(_ context.Context, _ int) ([]*storage.InstanceCluster, error) {
	return f.clusters, f.clustersErr
}

func (f *fakeFederationGraphRepo) GetInstanceConnections(_ context.Context, domain string, connectionType string) ([]*storage.InstanceConnection, error) {
	f.lastInstanceDomain = domain
	f.lastConnectionType = connectionType
	return f.connections, f.connectionsErr
}

func (f *fakeFederationGraphRepo) GetFederationEdges(_ context.Context, _ []string) ([]*storage.FederationEdge, error) {
	return f.instanceEdges, f.instanceEdgesErr
}

func (f *fakeFederationGraphRepo) GetFederationActivitiesByTimeRange(_ context.Context, start, end time.Time, _ int) ([]*models.FederationCostActivity, error) {
	f.lastFlowStart = start
	f.lastFlowEnd = end
	return f.activities, f.activitiesErr
}

func (f *fakeFederationGraphRepo) GetFederationCosts(_ context.Context, start, end time.Time, _ int, cursor string) ([]*storage.FederationCost, string, error) {
	f.lastFlowStart = start
	f.lastFlowEnd = end
	f.costsCursorObserved = cursor
	return f.costs, "", f.costsErr
}

func TestService_Round25_GetFederationMap(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := &fakeFederationGraphRepo{
		nodes: []*storage.FederationNode{
			{Domain: "a.example", Health: "healthy", ActiveConnections: 10, ActiveUsers: 100},
			{Domain: "b.example", Health: "warning", ActiveConnections: 5, ActiveUsers: 50},
			{Domain: "c.example", Health: "critical", ActiveConnections: 1, ActiveUsers: 10},
			{Domain: "d.example", Health: "offline", ActiveConnections: 0, ActiveUsers: 0},
		},
		allEdges: []*storage.FederationEdge{
			{SourceDomain: "a.example", TargetDomain: "b.example", Strength: 0.9, VolumeIn: 10, VolumeOut: 20, SuccessRate: 0.95},
			{SourceDomain: "b.example", TargetDomain: "c.example", Strength: 0.2, VolumeIn: 0, VolumeOut: 1, SuccessRate: 0.5},
		},
		clusters: []*storage.InstanceCluster{
			{ClusterID: "c1", Name: "cluster", Instances: []string{"a.example", "b.example"}, Cohesion: 0.7},
		},
	}
	svc := NewService(repo, zap.NewNop(), "local.test")

	t.Run("depth clamps to [1,3]", func(t *testing.T) {
		_, err := svc.GetFederationMap(ctx, 0)
		require.NoError(t, err)
		assert.Equal(t, 1, repo.lastDepth)

		_, err = svc.GetFederationMap(ctx, 99)
		require.NoError(t, err)
		assert.Equal(t, 3, repo.lastDepth)
	})

	t.Run("nodes error surfaces", func(t *testing.T) {
		repo.nodesErr = errors.New("boom")
		_, err := svc.GetFederationMap(ctx, 1)
		require.Error(t, err)
		repo.nodesErr = nil
	})

	t.Run("edges error surfaces", func(t *testing.T) {
		repo.allEdgesErr = errors.New("boom")
		_, err := svc.GetFederationMap(ctx, 1)
		require.Error(t, err)
		repo.allEdgesErr = nil
	})

	t.Run("clusters error is non-fatal", func(t *testing.T) {
		repo.clustersErr = errors.New("boom")
		graph, err := svc.GetFederationMap(ctx, 1)
		require.NoError(t, err)
		require.NotNil(t, graph)
		assert.Empty(t, graph.Clusters)
		assert.GreaterOrEqual(t, graph.HealthScore, 0.0)
		assert.LessOrEqual(t, graph.HealthScore, 1.0)
		repo.clustersErr = nil
	})
}

func TestService_Round25_GetInstanceRelationships(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := &fakeFederationGraphRepo{
		connections: []*storage.InstanceConnection{
			{TargetDomain: "x.example", Direction: "outbound", ConnectionType: "follows", VolumeIn: 10, VolumeOut: 20, LastActivity: time.Now()},
			{TargetDomain: "y.example", Direction: "inbound", ConnectionType: "mentions", VolumeIn: 1, VolumeOut: 0, LastActivity: time.Now()},
		},
	}
	svc := NewService(repo, zap.NewNop(), "local.test")

	_, err := svc.GetInstanceRelationships(ctx, "")
	require.Error(t, err)

	repo.connectionsErr = errors.New("boom")
	_, err = svc.GetInstanceRelationships(ctx, "a.example")
	require.Error(t, err)
	repo.connectionsErr = nil

	// Trigger recommendation branches:
	// - low connectivity (<5 connections)
	// - cost optimization (>50 edges)
	// - performance rec (low success rate for >1/3 edges)
	edges := make([]*storage.FederationEdge, 0, 51)
	for i := 0; i < 51; i++ {
		edges = append(edges, &storage.FederationEdge{SourceDomain: "a.example", TargetDomain: "z.example", SuccessRate: 0.7})
	}
	repo.instanceEdges = edges

	res, err := svc.GetInstanceRelationships(ctx, "a.example")
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "a.example", res.Domain)
	assert.Equal(t, "", repo.lastConnectionType, "service should request all connection types")
	assert.Len(t, res.DirectConnections, 1)
	assert.Len(t, res.IndirectConnections, 1)
	assert.GreaterOrEqual(t, res.FederationScore, 0.0)
	assert.LessOrEqual(t, res.FederationScore, 1.0)
	assert.Len(t, res.Recommendations, 3)

	// Edges error is non-fatal.
	repo.instanceEdgesErr = errors.New("boom")
	res, err = svc.GetInstanceRelationships(ctx, "a.example")
	require.NoError(t, err)
	require.NotNil(t, res)
	repo.instanceEdgesErr = nil
}

func TestService_Round25_GetFederationFlow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Now()

	repo := &fakeFederationGraphRepo{
		activities: []*models.FederationCostActivity{
			{Domain: "a.example", Type: "ingress", ByteSize: 100, Success: true, ResponseTime: 100, Timestamp: now.Add(-10 * time.Minute)},
			{Domain: "a.example", Type: "egress", ByteSize: 200, Success: false, ResponseTime: 200, Timestamp: now.Add(-20 * time.Minute)},
			{Domain: "b.example", Type: "egress", ByteSize: 300, Success: true, ResponseTime: 10, Timestamp: now.Add(-30 * time.Minute)},
		},
	}
	svc := NewService(repo, zap.NewNop(), "local.test")

	repo.activitiesErr = errors.New("boom")
	_, err := svc.GetFederationFlow(ctx, model.TimePeriodDay)
	require.Error(t, err)
	repo.activitiesErr = nil

	repo.costsErr = errors.New("boom")
	flow, err := svc.GetFederationFlow(ctx, model.TimePeriodHour)
	require.NoError(t, err)
	require.NotNil(t, flow)
	assert.NotEmpty(t, flow.VolumeByHour)
	assert.Empty(t, flow.CostByInstance)
	assert.True(t, repo.lastFlowStart.Before(repo.lastFlowEnd))
	repo.costsErr = nil

	repo.costs = []*storage.FederationCost{
		{Domain: "a.example", EstimatedCostUSD: 1.0},
		{Domain: "b.example", EstimatedCostUSD: 3.0},
	}
	flow, err = svc.GetFederationFlow(ctx, model.TimePeriodWeek)
	require.NoError(t, err)
	require.NotNil(t, flow)
	assert.Len(t, flow.CostByInstance, 2)

	// Switch defaults and conversions
	_, err = svc.GetFederationFlow(ctx, model.TimePeriod("unknown"))
	require.NoError(t, err)

	assert.Equal(t, model.InstanceHealthStatusHealthy, svc.convertHealthStatus("healthy"))
	assert.Equal(t, model.InstanceHealthStatusWarning, svc.convertHealthStatus("warning"))
	assert.Equal(t, model.InstanceHealthStatusCritical, svc.convertHealthStatus("critical"))
	assert.Equal(t, model.InstanceHealthStatusOffline, svc.convertHealthStatus("offline"))
	assert.Equal(t, model.InstanceHealthStatusUnknown, svc.convertHealthStatus("something-else"))

	assert.Equal(t, model.ConnectionTypeFollows, svc.convertConnectionType("follows"))
	assert.Equal(t, model.ConnectionTypeMentions, svc.convertConnectionType("mentions"))
	assert.Equal(t, model.ConnectionTypeReplies, svc.convertConnectionType("replies"))
	assert.Equal(t, model.ConnectionTypeBoosts, svc.convertConnectionType("boosts"))
	assert.Equal(t, model.ConnectionTypeQuotes, svc.convertConnectionType("quotes"))
	assert.Equal(t, model.ConnectionTypeMixed, svc.convertConnectionType("other"))
}
