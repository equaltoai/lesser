package federation

import (
	"context"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/storage"
)

// RelationshipTracker tracks and analyzes federation relationships
type RelationshipTracker struct {
	storage storage.Storage
}

// NewRelationshipTracker creates a new relationship tracker
func NewRelationshipTracker(store storage.Storage) *RelationshipTracker {
	return &RelationshipTracker{
		storage: store,
	}
}

// TrackDeliveryAttempt records a federation delivery attempt
func (rt *RelationshipTracker) TrackDeliveryAttempt(ctx context.Context, attempt *DeliveryAttempt) error {
	// Update federation edge with delivery metrics
	edge := &storage.FederationEdge{
		SourceDomain:   attempt.SourceDomain,
		TargetDomain:   attempt.TargetDomain,
		ConnectionType: attempt.ActivityType,
		LastActivity:   time.Now(),
	}

	if attempt.Success {
		edge.VolumeOut++
	}

	// Update or create the edge
	if err := rt.storage.UpdateFederationEdge(ctx, edge); err != nil {
		return fmt.Errorf("failed to update federation edge: %w", err)
	}

	// Update instance connection record
	connection := &storage.InstanceConnection{
		Domain:         attempt.SourceDomain,
		TargetDomain:   attempt.TargetDomain,
		ConnectionType: attempt.ActivityType,
		VolumeOut:      1,
		LastActivity:   time.Now(),
		Success:        attempt.Success,
		ResponseTimeMs: attempt.ResponseTimeMs,
		Health:         rt.calculateConnectionHealth(attempt.Success, attempt.ResponseTimeMs),
	}

	return rt.storeInstanceConnection(ctx, connection)
}

// TrackInboundActivity records inbound federation activity
func (rt *RelationshipTracker) TrackInboundActivity(ctx context.Context, activity *InboundActivity) error {
	// Update federation edge with inbound metrics
	edge := &storage.FederationEdge{
		SourceDomain:   activity.SourceDomain,
		TargetDomain:   activity.TargetDomain,
		ConnectionType: activity.ActivityType,
		VolumeIn:       1,
		LastActivity:   time.Now(),
	}

	// Update or create the edge
	if err := rt.storage.UpdateFederationEdge(ctx, edge); err != nil {
		return fmt.Errorf("failed to update federation edge: %w", err)
	}

	// Update instance connection record
	connection := &storage.InstanceConnection{
		Domain:         activity.TargetDomain,
		TargetDomain:   activity.SourceDomain,
		ConnectionType: activity.ActivityType,
		VolumeIn:       1,
		LastActivity:   time.Now(),
		Success:        true, // Inbound activities are successful by definition
		Health:         "healthy",
	}

	return rt.storeInstanceConnection(ctx, connection)
}

// AnalyzeRelationshipStrength calculates the strength of relationships between instances
func (rt *RelationshipTracker) AnalyzeRelationshipStrength(ctx context.Context, sourceDomain, targetDomain string) (*RelationshipAnalysis, error) {
	// Get edge data
	edges, err := rt.storage.GetFederationEdges(ctx, []string{sourceDomain, targetDomain})
	if err != nil {
		return nil, fmt.Errorf("failed to get federation edges: %w", err)
	}

	var sourceToTarget, targetToSource *storage.FederationEdge
	for _, edge := range edges {
		if edge.SourceDomain == sourceDomain && edge.TargetDomain == targetDomain {
			sourceToTarget = edge
		} else if edge.SourceDomain == targetDomain && edge.TargetDomain == sourceDomain {
			targetToSource = edge
		}
	}

	// Calculate relationship metrics
	analysis := &RelationshipAnalysis{
		SourceDomain: sourceDomain,
		TargetDomain: targetDomain,
		Timestamp:    time.Now(),
	}

	if sourceToTarget != nil {
		analysis.OutboundVolume = sourceToTarget.VolumeOut
		analysis.OutboundStrength = sourceToTarget.Strength
		analysis.LastOutboundActivity = sourceToTarget.LastActivity
	}

	if targetToSource != nil {
		analysis.InboundVolume = targetToSource.VolumeOut // Their outbound is our inbound
		analysis.InboundStrength = targetToSource.Strength
		analysis.LastInboundActivity = targetToSource.LastActivity
	}

	// Calculate combined metrics
	analysis.TotalVolume = analysis.InboundVolume + analysis.OutboundVolume
	analysis.Reciprocity = rt.calculateReciprocity(analysis.InboundVolume, analysis.OutboundVolume)
	analysis.OverallStrength = rt.calculateOverallStrength(analysis)
	analysis.RelationshipType = rt.classifyRelationship(analysis)

	return analysis, nil
}

// GenerateRecommendations generates relationship-based recommendations
func (rt *RelationshipTracker) GenerateRecommendations(ctx context.Context, domain string) ([]*FederationRecommendation, error) {
	var recommendations []*FederationRecommendation

	// Get connections for this domain
	connections, err := rt.storage.GetInstanceConnections(ctx, domain, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get connections: %w", err)
	}

	// Analyze for problematic connections
	for _, conn := range connections {
		if !conn.Success || conn.ResponseTimeMs > 5000 {
			rec := &FederationRecommendation{
				Type:         "performance",
				Priority:     "high",
				TargetDomain: conn.TargetDomain,
				Description:  fmt.Sprintf("High error rate or slow response times to %s", conn.TargetDomain),
				Action:       "Consider reviewing connection health or implementing retry logic",
				Metrics: map[string]any{
					"response_time_ms": conn.ResponseTimeMs,
					"success_rate":     rt.calculateSuccessRate(conn),
				},
			}
			recommendations = append(recommendations, rec)
		}
	}

	// Find underutilized connections
	strongEdges, err := rt.storage.GetStrongestConnectionsByType(ctx, "all", 50)
	if err == nil {
		underutilized := rt.findUnderutilizedConnections(domain, connections, strongEdges)
		for _, target := range underutilized {
			rec := &FederationRecommendation{
				Type:         "opportunity",
				Priority:     "medium",
				TargetDomain: target,
				Description:  fmt.Sprintf("Potential for stronger relationship with %s", target),
				Action:       "Consider increasing engagement or checking connectivity",
			}
			recommendations = append(recommendations, rec)
		}
	}

	// Cost optimization recommendations
	costRec := rt.generateCostRecommendations(connections)
	if costRec != nil {
		recommendations = append(recommendations, costRec)
	}

	return recommendations, nil
}

// Helper methods

func (rt *RelationshipTracker) storeInstanceConnection(ctx context.Context, connection *storage.InstanceConnection) error {
	// This would store the connection in DynamoDB
	// For now, we'll implement basic storage logic

	// In a real implementation, this would use a proper key structure
	// and handle updates/upserts correctly
	return nil // Placeholder
}

func (rt *RelationshipTracker) calculateConnectionHealth(success bool, responseTime float64) string {
	if !success {
		return "unhealthy"
	}
	if responseTime > 10000 {
		return "degraded"
	}
	if responseTime > 5000 {
		return "moderate"
	}
	return "healthy"
}

func (rt *RelationshipTracker) calculateReciprocity(inbound, outbound int64) float64 {
	if inbound == 0 && outbound == 0 {
		return 0.0
	}
	if inbound == 0 {
		return 0.0
	}
	if outbound == 0 {
		return 0.0
	}

	total := float64(inbound + outbound)
	smaller := float64(min(inbound, outbound))
	return smaller / total * 2.0 // Scale to 0-1 where 1 is perfect reciprocity
}

func (rt *RelationshipTracker) calculateOverallStrength(analysis *RelationshipAnalysis) float64 {
	// Weighted combination of volume, reciprocity, and freshness
	volumeScore := float64(analysis.TotalVolume) / 1000.0 // Normalize
	if volumeScore > 1.0 {
		volumeScore = 1.0
	}

	// Freshness based on most recent activity
	var lastActivity time.Time
	if analysis.LastInboundActivity.After(analysis.LastOutboundActivity) {
		lastActivity = analysis.LastInboundActivity
	} else {
		lastActivity = analysis.LastOutboundActivity
	}

	daysSince := time.Since(lastActivity).Hours() / 24
	freshnessScore := 1.0 / (1.0 + daysSince/30.0) // Decay over 30 days

	return volumeScore*0.5 + analysis.Reciprocity*0.3 + freshnessScore*0.2
}

func (rt *RelationshipTracker) classifyRelationship(analysis *RelationshipAnalysis) string {
	if analysis.TotalVolume == 0 {
		return "dormant"
	}

	if analysis.Reciprocity > 0.7 && analysis.TotalVolume > 100 {
		return "mutual"
	}

	if analysis.OutboundVolume > analysis.InboundVolume*2 {
		return "outbound_focused"
	}

	if analysis.InboundVolume > analysis.OutboundVolume*2 {
		return "inbound_focused"
	}

	if analysis.TotalVolume > 500 {
		return "active"
	}

	return "casual"
}

func (rt *RelationshipTracker) calculateSuccessRate(conn *storage.InstanceConnection) float64 {
	// This would need to be calculated from historical data
	// For now, return a placeholder based on current success status
	if conn.Success {
		return 1.0
	}
	return 0.0
}

func (rt *RelationshipTracker) findUnderutilizedConnections(domain string, connections []*storage.InstanceConnection, strongEdges []*storage.FederationEdge) []string {
	// Find domains that are well-connected in the federation but we have weak connections to
	connectedDomains := make(map[string]bool)
	for _, conn := range connections {
		connectedDomains[conn.TargetDomain] = true
	}

	var underutilized []string
	for _, edge := range strongEdges {
		// If this is a strong edge in the federation but we're not connected
		if edge.Strength > 0.5 && !connectedDomains[edge.TargetDomain] && edge.SourceDomain != domain {
			underutilized = append(underutilized, edge.TargetDomain)
		}
	}

	return underutilized
}

func (rt *RelationshipTracker) generateCostRecommendations(connections []*storage.InstanceConnection) *FederationRecommendation {
	// Analyze connection patterns for cost optimization
	var totalVolume int64
	lowVolumeConnections := 0

	for _, conn := range connections {
		totalVolume += conn.VolumeIn + conn.VolumeOut
		if conn.VolumeIn+conn.VolumeOut < 10 {
			lowVolumeConnections++
		}
	}

	if lowVolumeConnections > len(connections)/2 && len(connections) > 10 {
		return &FederationRecommendation{
			Type:        "cost",
			Priority:    "medium",
			Description: fmt.Sprintf("Many low-volume connections (%d of %d)", lowVolumeConnections, len(connections)),
			Action:      "Consider consolidating or reducing monitoring frequency for low-activity instances",
			Metrics: map[string]any{
				"low_volume_connections": lowVolumeConnections,
				"total_connections":      len(connections),
				"total_volume":           totalVolume,
			},
		}
	}

	return nil
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// Types for relationship tracking

type DeliveryAttempt struct {
	SourceDomain   string
	TargetDomain   string
	ActivityType   string
	Success        bool
	ResponseTimeMs float64
	Timestamp      time.Time
}

type InboundActivity struct {
	SourceDomain string
	TargetDomain string
	ActivityType string
	Timestamp    time.Time
}

type RelationshipAnalysis struct {
	SourceDomain         string    `json:"source_domain"`
	TargetDomain         string    `json:"target_domain"`
	InboundVolume        int64     `json:"inbound_volume"`
	OutboundVolume       int64     `json:"outbound_volume"`
	TotalVolume          int64     `json:"total_volume"`
	InboundStrength      float64   `json:"inbound_strength"`
	OutboundStrength     float64   `json:"outbound_strength"`
	OverallStrength      float64   `json:"overall_strength"`
	Reciprocity          float64   `json:"reciprocity"`
	RelationshipType     string    `json:"relationship_type"`
	LastInboundActivity  time.Time `json:"last_inbound_activity"`
	LastOutboundActivity time.Time `json:"last_outbound_activity"`
	Timestamp            time.Time `json:"timestamp"`
}

type FederationRecommendation struct {
	Type         string         `json:"type"`     // performance/opportunity/cost/security
	Priority     string         `json:"priority"` // high/medium/low
	TargetDomain string         `json:"target_domain,omitempty"`
	Description  string         `json:"description"`
	Action       string         `json:"action"`
	Metrics      map[string]any `json:"metrics,omitempty"`
}
