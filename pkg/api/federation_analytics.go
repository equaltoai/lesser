// Package api provides federation analytics API endpoints and handlers.
//
//revive:disable-next-line:var-naming
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/gorilla/mux"
)

type federationAnalyticsRepository interface {
	GetFederationNodes(ctx context.Context, depth int) ([]*storage.FederationNode, error)
	GetFederationNodesByHealth(ctx context.Context, healthStatus string, limit int) ([]*storage.FederationNode, error)
	GetFederationEdges(ctx context.Context, domains []string) ([]*storage.FederationEdge, error)
	GetStrongestConnectionsByType(ctx context.Context, connectionType string, limit int) ([]*storage.FederationEdge, error)
	CalculateFederationClusters(ctx context.Context) ([]*storage.InstanceCluster, error)
	GetInstanceMetadata(ctx context.Context, domain string) (*storage.InstanceMetadata, error)
	GetInstanceConnections(ctx context.Context, domain string, connectionType string) ([]*storage.InstanceConnection, error)
	GetDetailedFederationMetrics(ctx context.Context, domain, period string, startTime, endTime time.Time) ([]*storageModels.FederationAnalyticsTimeSeries, error)
}

type federationAnalyticsHooks interface {
	GetRelationshipAnalysis(ctx context.Context, sourceDomain, targetDomain string) (*federation.RelationshipAnalysis, error)
	GetFederationRecommendations(ctx context.Context, domain string) ([]*federation.FederationRecommendation, error)
}

// FederationAnalyticsHandler handles federation analytics API endpoints
type FederationAnalyticsHandler struct {
	repo  federationAnalyticsRepository
	hooks federationAnalyticsHooks
}

// NewFederationAnalyticsHandler creates a new federation analytics handler
func NewFederationAnalyticsHandler(store core.RepositoryStorage, hooks *federation.FederationHooks) *FederationAnalyticsHandler {
	var repo federationAnalyticsRepository
	if store != nil {
		repo = store.Federation()
	}
	return &FederationAnalyticsHandler{
		repo:  repo,
		hooks: hooks,
	}
}

// RegisterRoutes registers the federation analytics routes
func (fah *FederationAnalyticsHandler) RegisterRoutes(r *mux.Router) {
	// Admin routes for federation analytics
	admin := r.PathPrefix("/admin/federation").Subrouter()
	admin.HandleFunc("/graph/nodes", fah.GetFederationNodes).Methods("GET")
	admin.HandleFunc("/graph/edges", fah.GetFederationEdges).Methods("GET")
	admin.HandleFunc("/clusters", fah.GetFederationClusters).Methods("GET")
	admin.HandleFunc("/relationships/{source}/{target}", fah.GetRelationshipAnalysis).Methods("GET")
	admin.HandleFunc("/recommendations/{domain}", fah.GetRecommendations).Methods("GET")
	admin.HandleFunc("/instances/{domain}/metadata", fah.GetInstanceMetadata).Methods("GET")
	admin.HandleFunc("/timeseries/{domain}", fah.GetTimeSeries).Methods("GET")
}

// GetFederationNodes returns federation graph nodes
func (fah *FederationAnalyticsHandler) GetFederationNodes(w http.ResponseWriter, r *http.Request) {
	depthStr := r.URL.Query().Get("depth")
	depth, err := common.ParseAndValidateIntWithBounds("depth", depthStr, 0, 10, 1)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid depth parameter: %v", err), http.StatusBadRequest)
		return
	}

	healthFilter := r.URL.Query().Get("health")

	var nodes []*storage.FederationNode

	if healthFilter != "" {
		// Use the dedicated method for health-filtered queries
		nodes, err = fah.repo.GetFederationNodesByHealth(r.Context(), healthFilter, 100)
	} else {
		nodes, err = fah.repo.GetFederationNodes(r.Context(), depth)
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get federation nodes: %v", err), http.StatusInternalServerError)
		return
	}

	if err := writeJSON(w, http.StatusOK, map[string]any{
		"nodes": nodes,
		"count": len(nodes),
	}); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// GetFederationEdges returns federation graph edges
func (fah *FederationAnalyticsHandler) GetFederationEdges(w http.ResponseWriter, r *http.Request) {
	domains := r.URL.Query()["domain"]
	connectionType := r.URL.Query().Get("type")
	limitStr := r.URL.Query().Get("limit")

	var edges []*storage.FederationEdge
	var err error

	if connectionType != "" && limitStr != "" {
		if limit, parseErr := common.ParseFederationLimit(limitStr); parseErr == nil {
			edges, err = fah.repo.GetStrongestConnectionsByType(r.Context(), connectionType, limit)
		} else {
			http.Error(w, fmt.Sprintf("Invalid limit parameter: %v", parseErr), http.StatusBadRequest)
			return
		}
	} else if len(domains) > 0 {
		edges, err = fah.repo.GetFederationEdges(r.Context(), domains)
	} else {
		// Get strongest connections overall
		edges, err = fah.repo.GetStrongestConnectionsByType(r.Context(), "all", 100)
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get federation edges: %v", err), http.StatusInternalServerError)
		return
	}

	if err := writeJSON(w, http.StatusOK, map[string]any{
		"edges": edges,
		"count": len(edges),
	}); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// GetFederationClusters returns federation clusters
func (fah *FederationAnalyticsHandler) GetFederationClusters(w http.ResponseWriter, r *http.Request) {
	clusters, err := fah.repo.CalculateFederationClusters(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get federation clusters: %v", err), http.StatusInternalServerError)
		return
	}

	if err := writeJSON(w, http.StatusOK, map[string]any{
		"clusters": clusters,
		"count":    len(clusters),
	}); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// GetRelationshipAnalysis returns analysis of relationship between two domains
func (fah *FederationAnalyticsHandler) GetRelationshipAnalysis(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	source := vars["source"]
	target := vars["target"]

	if err := common.ValidateRequiredParam("source", source); err != nil {
		http.Error(w, "Source domain is required", http.StatusBadRequest)
		return
	}
	if err := common.ValidateRequiredParam("target", target); err != nil {
		http.Error(w, "Target domain is required", http.StatusBadRequest)
		return
	}

	analysis, err := fah.hooks.GetRelationshipAnalysis(r.Context(), source, target)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to analyze relationship: %v", err), http.StatusInternalServerError)
		return
	}

	if err := writeJSON(w, http.StatusOK, analysis); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// GetRecommendations returns federation recommendations for a domain
func (fah *FederationAnalyticsHandler) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	domain := vars["domain"]

	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		http.Error(w, "Domain is required", http.StatusBadRequest)
		return
	}

	recommendations, err := fah.hooks.GetFederationRecommendations(r.Context(), domain)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get recommendations: %v", err), http.StatusInternalServerError)
		return
	}

	if err := writeJSON(w, http.StatusOK, map[string]any{
		"recommendations": recommendations,
		"count":           len(recommendations),
		"domain":          domain,
		"generated_at":    time.Now(),
	}); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// GetInstanceMetadata returns detailed metadata for an instance
func (fah *FederationAnalyticsHandler) GetInstanceMetadata(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	domain := vars["domain"]

	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		http.Error(w, "Domain is required", http.StatusBadRequest)
		return
	}

	metadata, err := fah.repo.GetInstanceMetadata(r.Context(), domain)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "Instance not found", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to get instance metadata: %v", err), http.StatusInternalServerError)
		return
	}

	// Also get connection information
	connections, err := fah.repo.GetInstanceConnections(r.Context(), domain, "")
	if err != nil {
		// Don't fail if connections can't be retrieved
		connections = []*storage.InstanceConnection{}
	}

	if err := writeJSON(w, http.StatusOK, map[string]any{
		"metadata":     metadata,
		"connections":  connections,
		"retrieved_at": time.Now(),
	}); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// GetTimeSeries returns time-series data for federation metrics
func (fah *FederationAnalyticsHandler) GetTimeSeries(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	domain := vars["domain"]

	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		http.Error(w, "Domain is required", http.StatusBadRequest)
		return
	}

	period := r.URL.Query().Get("period")
	if err := common.ValidateRequiredParam("period", period); err != nil {
		period = "daily"
	}

	// Parse time range
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	var startTime, endTime time.Time
	var err error

	if startStr != "" {
		startTime, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			http.Error(w, "Invalid start time format", http.StatusBadRequest)
			return
		}
	} else {
		// Default to last 30 days
		startTime = time.Now().Add(-30 * 24 * time.Hour)
	}

	if endStr != "" {
		endTime, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			http.Error(w, "Invalid end time format", http.StatusBadRequest)
			return
		}
	} else {
		endTime = time.Now()
	}

	// Get detailed time series data from federation repository
	timeSeries, err := fah.repo.GetDetailedFederationMetrics(r.Context(), domain, period, startTime, endTime)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get time series data: %v", err), http.StatusInternalServerError)
		return
	}

	// Calculate summary statistics
	var totalActivities, totalErrors int64
	var avgHealthScore, avgLatency float64
	var recentHealthScore float64
	healthScores := make([]float64, 0, len(timeSeries))

	for _, metric := range timeSeries {
		totalActivities += metric.ActivityCount
		totalErrors += metric.FailedActivities
		healthScores = append(healthScores, metric.HealthScore)
		avgLatency += float64(metric.InboxDeliveryP95)

		// Track most recent health score
		if metric.Timestamp.After(time.Now().Add(-10 * time.Minute)) {
			recentHealthScore = metric.HealthScore
		}
	}

	if len(timeSeries) > 0 {
		for _, score := range healthScores {
			avgHealthScore += score
		}
		avgHealthScore /= float64(len(healthScores))
		avgLatency /= float64(len(timeSeries))
	}

	// Calculate error rate
	var errorRate float64
	if totalActivities > 0 {
		errorRate = float64(totalErrors) / float64(totalActivities)
	}

	// Determine health status based on recent health score
	healthStatus := "HEALTHY"
	if recentHealthScore < 40 {
		healthStatus = "CRITICAL"
	} else if recentHealthScore < 60 {
		healthStatus = "UNHEALTHY"
	} else if recentHealthScore < 80 {
		healthStatus = "DEGRADED"
	}

	// Format response following federation analytics guidance
	response := map[string]any{
		"domain":     domain,
		"period":     period,
		"start_time": startTime,
		"end_time":   endTime,
		"data":       timeSeries,
		"summary": map[string]any{
			"total_data_points":   len(timeSeries),
			"total_activities":    totalActivities,
			"total_errors":        totalErrors,
			"error_rate":          errorRate,
			"avg_health_score":    avgHealthScore,
			"recent_health_score": recentHealthScore,
			"health_status":       healthStatus,
			"avg_p95_latency_ms":  avgLatency,
			"aggregation_level":   period,
		},
		"health_thresholds": map[string]any{
			"healthy":   80.0,
			"degraded":  60.0,
			"unhealthy": 40.0,
			"critical":  0.0,
		},
		"alert_conditions": map[string]any{
			"reachability_critical": "< 50%",
			"latency_warning":       "> 5s P95",
			"queue_depth_warning":   "> 10,000",
		},
		"data_retention": map[string]any{
			"5min":    "24 hours",
			"hourly":  "7 days",
			"daily":   "90 days",
			"monthly": "2 years",
		},
		"generated_at": time.Now(),
	}

	if err := writeJSON(w, http.StatusOK, response); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
	return nil
}
