package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubFederationRepo struct {
	nodesByHealthCalled bool
	nodesCalled         bool
	edgesCalled         bool
	strongestCalled     bool
	metadataCalled      bool
	connectionsCalled   bool
	timeSeriesCalled    bool

	strongestType  string
	strongestLimit int

	nodesErr       error
	nodesByHealth  []*storage.FederationNode
	nodes          []*storage.FederationNode
	edgesErr       error
	edges          []*storage.FederationEdge
	strongestErr   error
	strongest      []*storage.FederationEdge
	clustersErr    error
	clusters       []*storage.InstanceCluster
	metadataErr    error
	metadata       *storage.InstanceMetadata
	connectionsErr error
	connections    []*storage.InstanceConnection
	timeSeriesErr  error
	timeSeries     []*storageModels.FederationAnalyticsTimeSeries
}

func (s *stubFederationRepo) GetFederationNodes(_ context.Context, _ int) ([]*storage.FederationNode, error) {
	s.nodesCalled = true
	return s.nodes, s.nodesErr
}

func (s *stubFederationRepo) GetFederationNodesByHealth(_ context.Context, _ string, _ int) ([]*storage.FederationNode, error) {
	s.nodesByHealthCalled = true
	return s.nodesByHealth, s.nodesErr
}

func (s *stubFederationRepo) GetFederationEdges(_ context.Context, _ []string) ([]*storage.FederationEdge, error) {
	s.edgesCalled = true
	return s.edges, s.edgesErr
}

func (s *stubFederationRepo) GetStrongestConnectionsByType(_ context.Context, connectionType string, limit int) ([]*storage.FederationEdge, error) {
	s.strongestCalled = true
	s.strongestType = connectionType
	s.strongestLimit = limit
	return s.strongest, s.strongestErr
}

func (s *stubFederationRepo) CalculateFederationClusters(_ context.Context) ([]*storage.InstanceCluster, error) {
	return s.clusters, s.clustersErr
}

func (s *stubFederationRepo) GetInstanceMetadata(_ context.Context, _ string) (*storage.InstanceMetadata, error) {
	s.metadataCalled = true
	return s.metadata, s.metadataErr
}

func (s *stubFederationRepo) GetInstanceConnections(_ context.Context, _ string, _ string) ([]*storage.InstanceConnection, error) {
	s.connectionsCalled = true
	return s.connections, s.connectionsErr
}

func (s *stubFederationRepo) GetDetailedFederationMetrics(_ context.Context, _, _ string, _, _ time.Time) ([]*storageModels.FederationAnalyticsTimeSeries, error) {
	s.timeSeriesCalled = true
	return s.timeSeries, s.timeSeriesErr
}

type stubFederationHooks struct {
	relationshipAnalysis *federation.RelationshipAnalysis
	relationshipErr      error
	recommendations      []*federation.FederationRecommendation
	recommendationsErr   error
}

func (s *stubFederationHooks) GetRelationshipAnalysis(_ context.Context, _, _ string) (*federation.RelationshipAnalysis, error) {
	return s.relationshipAnalysis, s.relationshipErr
}

func (s *stubFederationHooks) GetFederationRecommendations(_ context.Context, _ string) ([]*federation.FederationRecommendation, error) {
	return s.recommendations, s.recommendationsErr
}

func TestFederationAnalyticsHandler_GetFederationNodes(t *testing.T) {
	repo := &stubFederationRepo{
		nodes: []*storage.FederationNode{{Domain: "example.com"}},
	}
	handler := &FederationAnalyticsHandler{repo: repo, hooks: &stubFederationHooks{}}

	t.Run("invalid depth rejected", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/graph/nodes?depth=99", nil)

		handler.GetFederationNodes(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("health filter uses dedicated query", func(t *testing.T) {
		repo.nodesByHealth = []*storage.FederationNode{{Domain: "health.example"}}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/graph/nodes?health=healthy", nil)

		handler.GetFederationNodes(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.True(t, repo.nodesByHealthCalled)

		var payload map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
		assert.Equal(t, float64(1), payload["count"])
	})

	t.Run("encode errors surface as 500", func(t *testing.T) {
		repo.nodes = []*storage.FederationNode{{
			Domain:   "bad.example",
			Metadata: map[string]any{"bad": make(chan int)},
		}}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/graph/nodes", nil)

		handler.GetFederationNodes(rr, req)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})

	t.Run("repo errors surface as 500", func(t *testing.T) {
		repo.nodes = nil
		repo.nodesErr = errors.New("boom")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/graph/nodes", nil)

		handler.GetFederationNodes(rr, req)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

func TestFederationAnalyticsHandler_GetFederationEdges(t *testing.T) {
	repo := &stubFederationRepo{}
	handler := &FederationAnalyticsHandler{repo: repo, hooks: &stubFederationHooks{}}

	t.Run("invalid limit rejected", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/graph/edges?type=follow&limit=bad", nil)
		handler.GetFederationEdges(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("defaults to strongest connections", func(t *testing.T) {
		repo.strongest = []*storage.FederationEdge{{SourceDomain: "a", TargetDomain: "b"}}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/graph/edges", nil)
		handler.GetFederationEdges(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.True(t, repo.strongestCalled)
		assert.Equal(t, "all", repo.strongestType)
		assert.Equal(t, 100, repo.strongestLimit)
	})

	t.Run("type and limit calls strongest-by-type", func(t *testing.T) {
		repo.strongestCalled = false
		repo.strongest = []*storage.FederationEdge{{SourceDomain: "a", TargetDomain: "b"}}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/graph/edges?type=follow&limit=5", nil)
		handler.GetFederationEdges(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.True(t, repo.strongestCalled)
		assert.Equal(t, "follow", repo.strongestType)
		assert.Equal(t, 5, repo.strongestLimit)
	})

	t.Run("domains param calls edge lookup", func(t *testing.T) {
		repo.edgesCalled = false
		repo.edges = []*storage.FederationEdge{{SourceDomain: "a", TargetDomain: "b"}}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/graph/edges?domain=a&domain=b", nil)
		handler.GetFederationEdges(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.True(t, repo.edgesCalled)
	})

	t.Run("repo errors surface as 500", func(t *testing.T) {
		repo.strongestErr = errors.New("boom")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/graph/edges", nil)
		handler.GetFederationEdges(rr, req)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

func TestFederationAnalyticsHandler_GetRelationshipAnalysis(t *testing.T) {
	hooks := &stubFederationHooks{
		relationshipAnalysis: &federation.RelationshipAnalysis{SourceDomain: "a", TargetDomain: "b"},
	}
	handler := &FederationAnalyticsHandler{repo: &stubFederationRepo{}, hooks: hooks}

	req := httptest.NewRequest(http.MethodGet, "/admin/federation/relationships/a/b", nil)
	req = mux.SetURLVars(req, map[string]string{"source": "a", "target": "b"})

	rr := httptest.NewRecorder()
	handler.GetRelationshipAnalysis(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "\"source_domain\"")

	t.Run("missing required params rejected", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/relationships//", nil)
		req = mux.SetURLVars(req, map[string]string{"source": "", "target": ""})

		handler.GetRelationshipAnalysis(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("hook errors surface as 500", func(t *testing.T) {
		hooks := &stubFederationHooks{relationshipErr: errors.New("boom")}
		handler := &FederationAnalyticsHandler{repo: &stubFederationRepo{}, hooks: hooks}

		req := httptest.NewRequest(http.MethodGet, "/admin/federation/relationships/a/b", nil)
		req = mux.SetURLVars(req, map[string]string{"source": "a", "target": "b"})

		rr := httptest.NewRecorder()
		handler.GetRelationshipAnalysis(rr, req)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

func TestFederationAnalyticsHandler_GetRecommendations(t *testing.T) {
	hooks := &stubFederationHooks{
		recommendations: []*federation.FederationRecommendation{{Type: "performance", Priority: "high"}},
	}
	handler := &FederationAnalyticsHandler{repo: &stubFederationRepo{}, hooks: hooks}

	req := httptest.NewRequest(http.MethodGet, "/admin/federation/recommendations/example.com", nil)
	req = mux.SetURLVars(req, map[string]string{"domain": "example.com"})

	rr := httptest.NewRecorder()
	handler.GetRecommendations(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "\"recommendations\"")

	t.Run("missing domain rejected", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/recommendations/", nil)
		req = mux.SetURLVars(req, map[string]string{"domain": ""})

		handler.GetRecommendations(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("encode errors surface as 500", func(t *testing.T) {
		hooks := &stubFederationHooks{
			recommendations: []*federation.FederationRecommendation{{
				Type:     "performance",
				Priority: "high",
				Metrics:  map[string]any{"bad": make(chan int)},
			}},
		}
		handler := &FederationAnalyticsHandler{repo: &stubFederationRepo{}, hooks: hooks}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/recommendations/example.com", nil)
		req = mux.SetURLVars(req, map[string]string{"domain": "example.com"})

		handler.GetRecommendations(rr, req)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

func TestFederationAnalyticsHandler_GetInstanceMetadata(t *testing.T) {
	repo := &stubFederationRepo{
		metadata:    &storage.InstanceMetadata{Domain: "example.com"},
		connections: []*storage.InstanceConnection{{Domain: "example.com"}},
	}
	handler := &FederationAnalyticsHandler{repo: repo, hooks: &stubFederationHooks{}}

	t.Run("not found returns 404", func(t *testing.T) {
		repo.metadataErr = storage.ErrNotFound
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/instances/example.com/metadata", nil)
		req = mux.SetURLVars(req, map[string]string{"domain": "example.com"})

		handler.GetInstanceMetadata(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("connections errors degrade gracefully", func(t *testing.T) {
		repo.metadataErr = nil
		repo.connectionsErr = errors.New("boom")
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/instances/example.com/metadata", nil)
		req = mux.SetURLVars(req, map[string]string{"domain": "example.com"})

		handler.GetInstanceMetadata(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var payload map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
		assert.NotNil(t, payload["metadata"])
	})

	t.Run("encode errors surface as 500", func(t *testing.T) {
		repo.connectionsErr = nil
		repo.metadata = &storage.InstanceMetadata{
			Domain:       "bad.example",
			CustomFields: map[string]any{"bad": make(chan int)},
		}
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/instances/bad.example/metadata", nil)
		req = mux.SetURLVars(req, map[string]string{"domain": "bad.example"})

		handler.GetInstanceMetadata(rr, req)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})

	t.Run("unexpected repo errors surface as 500", func(t *testing.T) {
		repo.metadataErr = errors.New("boom")
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/instances/example.com/metadata", nil)
		req = mux.SetURLVars(req, map[string]string{"domain": "example.com"})

		handler.GetInstanceMetadata(rr, req)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

func TestFederationAnalyticsHandler_GetTimeSeries(t *testing.T) {
	now := time.Now()
	repo := &stubFederationRepo{
		timeSeries: []*storageModels.FederationAnalyticsTimeSeries{
			{Timestamp: now.Add(-5 * time.Minute), ActivityCount: 10, FailedActivities: 2, HealthScore: 35, InboxDeliveryP95: 1000},
			{Timestamp: now.Add(-time.Hour), ActivityCount: 20, FailedActivities: 0, HealthScore: 80, InboxDeliveryP95: 2000},
		},
	}
	handler := &FederationAnalyticsHandler{repo: repo, hooks: &stubFederationHooks{}}

	t.Run("invalid start time rejected", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/timeseries/example.com?start=not-a-time", nil)
		req = mux.SetURLVars(req, map[string]string{"domain": "example.com"})
		handler.GetTimeSeries(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("summarizes recent health", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/timeseries/example.com", nil)
		req = mux.SetURLVars(req, map[string]string{"domain": "example.com"})
		handler.GetTimeSeries(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var payload map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))

		summary, ok := payload["summary"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "CRITICAL", summary["health_status"])
		assert.Equal(t, float64(30), summary["total_activities"])
		assert.Equal(t, float64(2), summary["total_errors"])
		assert.Equal(t, 1500.0, summary["avg_p95_latency_ms"])
	})

	t.Run("invalid end time rejected", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/timeseries/example.com?end=not-a-time", nil)
		req = mux.SetURLVars(req, map[string]string{"domain": "example.com"})
		handler.GetTimeSeries(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("repo errors surface as 500", func(t *testing.T) {
		repo.timeSeriesErr = errors.New("boom")
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/timeseries/example.com", nil)
		req = mux.SetURLVars(req, map[string]string{"domain": "example.com"})
		handler.GetTimeSeries(rr, req)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		repo.timeSeriesErr = nil
	})

	t.Run("health status uses intermediate bands", func(t *testing.T) {
		repo.timeSeries = []*storageModels.FederationAnalyticsTimeSeries{
			{Timestamp: time.Now().Add(-time.Minute), ActivityCount: 1, FailedActivities: 0, HealthScore: 50, InboxDeliveryP95: 1000},
		}
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/timeseries/example.com", nil)
		req = mux.SetURLVars(req, map[string]string{"domain": "example.com"})
		handler.GetTimeSeries(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var payload map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
		summary := payload["summary"].(map[string]any)
		assert.Equal(t, "UNHEALTHY", summary["health_status"])

		repo.timeSeries = []*storageModels.FederationAnalyticsTimeSeries{
			{Timestamp: time.Now().Add(-time.Minute), ActivityCount: 1, FailedActivities: 0, HealthScore: 70, InboxDeliveryP95: 1000},
		}
		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/admin/federation/timeseries/example.com", nil)
		req = mux.SetURLVars(req, map[string]string{"domain": "example.com"})
		handler.GetTimeSeries(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
		summary = payload["summary"].(map[string]any)
		assert.Equal(t, "DEGRADED", summary["health_status"])

		repo.timeSeries = []*storageModels.FederationAnalyticsTimeSeries{
			{Timestamp: time.Now().Add(-time.Minute), ActivityCount: 1, FailedActivities: 0, HealthScore: 90, InboxDeliveryP95: 1000},
		}
		rr = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/admin/federation/timeseries/example.com", nil)
		req = mux.SetURLVars(req, map[string]string{"domain": "example.com"})
		handler.GetTimeSeries(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
		summary = payload["summary"].(map[string]any)
		assert.Equal(t, "HEALTHY", summary["health_status"])
	})
}

func TestFederationAnalyticsHandler_ClustersAndErrors(t *testing.T) {
	repo := &stubFederationRepo{
		clustersErr: errors.New("boom"),
	}
	handler := &FederationAnalyticsHandler{repo: repo, hooks: &stubFederationHooks{}}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/federation/clusters", nil)
	handler.GetFederationClusters(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	t.Run("success returns clusters", func(t *testing.T) {
		repo.clustersErr = nil
		repo.clusters = []*storage.InstanceCluster{{ID: "c1", Name: "Cluster 1"}}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/federation/clusters", nil)
		handler.GetFederationClusters(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}

func TestFederationAnalyticsHandler_RecommendationsError(t *testing.T) {
	hooks := &stubFederationHooks{
		recommendationsErr: errors.New("boom"),
	}
	handler := &FederationAnalyticsHandler{repo: &stubFederationRepo{}, hooks: hooks}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/federation/recommendations/example.com", nil)
	req = mux.SetURLVars(req, map[string]string{"domain": "example.com"})

	handler.GetRecommendations(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}
