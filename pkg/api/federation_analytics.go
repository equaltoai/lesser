package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/aron23/lesser/pkg/federation"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/gorilla/mux"
)

// FederationAnalyticsHandler handles federation analytics API endpoints
type FederationAnalyticsHandler struct {
	storage storage.Storage
	hooks   *federation.FederationHooks
}

// NewFederationAnalyticsHandler creates a new federation analytics handler
func NewFederationAnalyticsHandler(store storage.Storage, hooks *federation.FederationHooks) *FederationAnalyticsHandler {
	return &FederationAnalyticsHandler{
		storage: store,
		hooks:   hooks,
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
	depth := 1
	if depthStr != "" {
		if d, err := strconv.Atoi(depthStr); err == nil {
			depth = d
		}
	}

	healthFilter := r.URL.Query().Get("health")

	var nodes []*storage.FederationNode
	var err error

	if healthFilter != "" {
		// Get nodes filtered by health
		if healthGetter, ok := fah.storage.(interface {
			GetFederationNodesByHealth(ctx context.Context, health string) ([]*storage.FederationNode, error)
		}); ok {
			nodes, err = healthGetter.GetFederationNodesByHealth(r.Context(), healthFilter)
		} else {
			// Fallback to all nodes and filter
			nodes, err = fah.storage.GetFederationNodes(r.Context(), depth)
			if err == nil {
				filtered := make([]*storage.FederationNode, 0)
				for _, node := range nodes {
					if node.Health == healthFilter {
						filtered = append(filtered, node)
					}
				}
				nodes = filtered
			}
		}
	} else {
		nodes, err = fah.storage.GetFederationNodes(r.Context(), depth)
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get federation nodes: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes": nodes,
		"count": len(nodes),
	})
}

// GetFederationEdges returns federation graph edges
func (fah *FederationAnalyticsHandler) GetFederationEdges(w http.ResponseWriter, r *http.Request) {
	domains := r.URL.Query()["domain"]
	connectionType := r.URL.Query().Get("type")
	limitStr := r.URL.Query().Get("limit")

	var edges []*storage.FederationEdge
	var err error

	if connectionType != "" && limitStr != "" {
		if limit, parseErr := strconv.Atoi(limitStr); parseErr == nil {
			edges, err = fah.storage.GetStrongestConnectionsByType(r.Context(), connectionType, limit)
		}
	} else if len(domains) > 0 {
		edges, err = fah.storage.GetFederationEdges(r.Context(), domains)
	} else {
		// Get strongest connections overall
		edges, err = fah.storage.GetStrongestConnectionsByType(r.Context(), "all", 100)
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get federation edges: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"edges": edges,
		"count": len(edges),
	})
}

// GetFederationClusters returns federation clusters
func (fah *FederationAnalyticsHandler) GetFederationClusters(w http.ResponseWriter, r *http.Request) {
	clusters, err := fah.storage.CalculateFederationClusters(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get federation clusters: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"clusters": clusters,
		"count":    len(clusters),
	})
}

// GetRelationshipAnalysis returns analysis of relationship between two domains
func (fah *FederationAnalyticsHandler) GetRelationshipAnalysis(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	source := vars["source"]
	target := vars["target"]

	if source == "" || target == "" {
		http.Error(w, "Source and target domains are required", http.StatusBadRequest)
		return
	}

	analysis, err := fah.hooks.GetRelationshipAnalysis(r.Context(), source, target)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to analyze relationship: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(analysis)
}

// GetRecommendations returns federation recommendations for a domain
func (fah *FederationAnalyticsHandler) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	domain := vars["domain"]

	if domain == "" {
		http.Error(w, "Domain is required", http.StatusBadRequest)
		return
	}

	recommendations, err := fah.hooks.GetFederationRecommendations(r.Context(), domain)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get recommendations: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"recommendations": recommendations,
		"count":          len(recommendations),
		"domain":         domain,
		"generated_at":   time.Now(),
	})
}

// GetInstanceMetadata returns detailed metadata for an instance
func (fah *FederationAnalyticsHandler) GetInstanceMetadata(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	domain := vars["domain"]

	if domain == "" {
		http.Error(w, "Domain is required", http.StatusBadRequest)
		return
	}

	metadata, err := fah.storage.GetInstanceMetadata(r.Context(), domain)
	if err != nil {
		if err == storage.ErrNotFound {
			http.Error(w, "Instance not found", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to get instance metadata: %v", err), http.StatusInternalServerError)
		return
	}

	// Also get connection information
	connections, err := fah.storage.GetInstanceConnections(r.Context(), domain, "")
	if err != nil {
		// Don't fail if connections can't be retrieved
		connections = []*storage.InstanceConnection{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"metadata":    metadata,
		"connections": connections,
		"retrieved_at": time.Now(),
	})
}

// GetTimeSeries returns time-series data for federation metrics
func (fah *FederationAnalyticsHandler) GetTimeSeries(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	domain := vars["domain"]

	if domain == "" {
		http.Error(w, "Domain is required", http.StatusBadRequest)
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
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

	// This would require implementing GetFederationTimeSeries in storage
	// For now, return a placeholder response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"domain":     domain,
		"period":     period,
		"start_time": startTime,
		"end_time":   endTime,
		"data":       []interface{}{}, // Placeholder
		"message":    "Time series data not yet implemented",
	})
}