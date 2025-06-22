package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

type FederationAggregator struct {
	storage storage.Storage
}

// AggregationEvent represents the input for the aggregation job
type AggregationEvent struct {
	Type      string    `json:"type"` // "daily", "hourly", "realtime"
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
}

func (fa *FederationAggregator) HandleRequest(ctx context.Context, event events.CloudWatchEvent) error {
	// Parse the aggregation event
	var aggEvent AggregationEvent
	if err := json.Unmarshal(event.Detail, &aggEvent); err != nil {
		// Default to daily aggregation for scheduled events
		aggEvent = AggregationEvent{
			Type:      "daily",
			StartTime: time.Now().Add(-24 * time.Hour),
			EndTime:   time.Now(),
		}
	}

	switch aggEvent.Type {
	case "daily":
		return fa.aggregateDailyMetrics(ctx, aggEvent.StartTime, aggEvent.EndTime)
	case "hourly":
		return fa.aggregateHourlyMetrics(ctx, aggEvent.StartTime, aggEvent.EndTime)
	case "realtime":
		return fa.aggregateRealtimeMetrics(ctx)
	default:
		return fmt.Errorf("unknown aggregation type: %s", aggEvent.Type)
	}
}

func (fa *FederationAggregator) aggregateDailyMetrics(ctx context.Context, startTime, endTime time.Time) error {
	// Get all federation nodes
	nodes, err := fa.storage.GetFederationNodes(ctx, 1)
	if err != nil {
		return fmt.Errorf("failed to get federation nodes: %w", err)
	}

	// For each node, calculate daily metrics
	for _, node := range nodes {
		// Get all connections for this node
		connections, err := fa.storage.GetInstanceConnections(ctx, node.Domain, "")
		if err != nil {
			continue // Log and continue with other nodes
		}

		// Calculate aggregated metrics
		var totalInbound, totalOutbound int64
		var errorCount, successCount int64
		connectionTypes := make(map[string]int64)

		for _, conn := range connections {
			if conn.LastActivity.After(startTime) && conn.LastActivity.Before(endTime) {
				totalInbound += conn.VolumeIn
				totalOutbound += conn.VolumeOut
				connectionTypes[conn.ConnectionType]++

				if conn.Success {
					successCount++
				} else {
					errorCount++
				}
			}
		}

		// Calculate health score
		healthScore := fa.calculateHealthScore(successCount, errorCount, totalInbound, totalOutbound)

		// Update node with new metrics
		node.TotalConnections = int64(len(connections))
		node.Health = healthScore
		node.LastSeen = time.Now()
		node.ActivityVolume = totalInbound + totalOutbound

		if err := fa.storage.UpdateFederationNode(ctx, node); err != nil {
			fmt.Printf("failed to update node %s: %v\n", node.Domain, err)
		}

		// Update edges with calculated strength
		if err := fa.updateEdgeStrengths(ctx, node.Domain, connections); err != nil {
			fmt.Printf("failed to update edge strengths for %s: %v\n", node.Domain, err)
		}
	}

	// Calculate and store federation clusters
	return fa.calculateAndStoreClusters(ctx)
}

func (fa *FederationAggregator) aggregateHourlyMetrics(ctx context.Context, startTime, endTime time.Time) error {
	// Get recent federation activity
	nodes, err := fa.storage.GetFederationNodes(ctx, 1)
	if err != nil {
		return fmt.Errorf("failed to get federation nodes: %w", err)
	}

	for _, node := range nodes {
		// Get hourly connection data
		connections, err := fa.storage.GetInstanceConnections(ctx, node.Domain, "")
		if err != nil {
			continue
		}

		// Calculate hourly metrics
		hourlyData := &storage.FederationTimeSeries{
			Domain:         node.Domain,
			Timestamp:      startTime,
			Period:         "hourly",
			InboundVolume:  0,
			OutboundVolume: 0,
			ErrorRate:      0,
			ResponseTime:   0,
		}

		var totalResponseTime float64
		var errorCount, totalCount int64

		for _, conn := range connections {
			if conn.LastActivity.After(startTime) && conn.LastActivity.Before(endTime) {
				hourlyData.InboundVolume += conn.VolumeIn
				hourlyData.OutboundVolume += conn.VolumeOut
				totalResponseTime += conn.ResponseTimeMs
				totalCount++
				if !conn.Success {
					errorCount++
				}
			}
		}

		if totalCount > 0 {
			hourlyData.ErrorRate = float64(errorCount) / float64(totalCount)
			hourlyData.ResponseTime = totalResponseTime / float64(totalCount)
		}

		// Store hourly data
		if err := fa.storage.StoreFederationTimeSeries(ctx, hourlyData); err != nil {
			fmt.Printf("failed to store hourly data for %s: %v\n", node.Domain, err)
		}
	}

	return nil
}

func (fa *FederationAggregator) aggregateRealtimeMetrics(ctx context.Context) error {
	// This would process real-time events from SQS/DynamoDB Streams
	// For now, just update active connection counts

	nodes, err := fa.storage.GetFederationNodes(ctx, 1)
	if err != nil {
		return fmt.Errorf("failed to get federation nodes: %w", err)
	}

	for _, node := range nodes {
		// Count active connections in the last 5 minutes
		recentConnections, err := fa.storage.GetRecentInstanceConnections(ctx, node.Domain, 5*time.Minute)
		if err != nil {
			continue
		}

		node.ActiveConnections = int64(len(recentConnections))
		node.LastSeen = time.Now()

		if err := fa.storage.UpdateFederationNode(ctx, node); err != nil {
			fmt.Printf("failed to update node %s: %v\n", node.Domain, err)
		}
	}

	return nil
}

func (fa *FederationAggregator) calculateHealthScore(success, errors, inbound, outbound int64) string {
	if success+errors == 0 {
		return "unknown"
	}

	errorRate := float64(errors) / float64(success+errors)
	activityLevel := float64(inbound + outbound)

	// Health score based on error rate and activity level
	if errorRate > 0.5 {
		return "unhealthy"
	} else if errorRate > 0.2 {
		return "degraded"
	} else if errorRate > 0.05 || activityLevel < 100 {
		return "moderate"
	}
	return "healthy"
}

func (fa *FederationAggregator) updateEdgeStrengths(ctx context.Context, domain string, connections []*storage.InstanceConnection) error {
	// Group connections by target domain
	edgeMap := make(map[string]*storage.FederationEdge)

	for _, conn := range connections {
		key := conn.TargetDomain
		if edge, exists := edgeMap[key]; exists {
			edge.VolumeIn += conn.VolumeIn
			edge.VolumeOut += conn.VolumeOut
			edge.SharedUsers++
			if conn.LastActivity.After(edge.LastActivity) {
				edge.LastActivity = conn.LastActivity
			}
		} else {
			edgeMap[key] = &storage.FederationEdge{
				SourceDomain:   domain,
				TargetDomain:   conn.TargetDomain,
				ConnectionType: conn.ConnectionType,
				VolumeIn:       conn.VolumeIn,
				VolumeOut:      conn.VolumeOut,
				SharedUsers:    1,
				LastActivity:   conn.LastActivity,
			}
		}
	}

	// Calculate strength and update edges
	for _, edge := range edgeMap {
		// Strength calculation: combination of volume, reciprocity, and freshness
		totalVolume := float64(edge.VolumeIn + edge.VolumeOut)
		reciprocity := float64(edge.VolumeIn) / (float64(edge.VolumeOut) + 1) // Avoid division by zero
		if reciprocity > 1 {
			reciprocity = 1 / reciprocity // Normalize to 0-1
		}

		daysSinceActivity := time.Since(edge.LastActivity).Hours() / 24
		freshness := 1.0 / (1.0 + daysSinceActivity/30) // Decay over 30 days

		edge.Strength = (totalVolume/1000)*0.5 + reciprocity*0.3 + freshness*0.2

		if err := fa.storage.UpdateFederationEdge(ctx, edge); err != nil {
			return fmt.Errorf("failed to update edge %s->%s: %w", edge.SourceDomain, edge.TargetDomain, err)
		}
	}

	return nil
}

func (fa *FederationAggregator) calculateAndStoreClusters(ctx context.Context) error {
	// Get all edges with significant strength
	strongEdges, err := fa.storage.GetStrongestConnectionsByType(ctx, "all", 1000)
	if err != nil {
		return fmt.Errorf("failed to get strong edges: %w", err)
	}

	// Simple clustering algorithm - connected components
	clusters := fa.findConnectedComponents(strongEdges)

	// Store clusters
	for i, cluster := range clusters {
		instanceCluster := &storage.InstanceCluster{
			ClusterID:  fmt.Sprintf("cluster-%d-%s", i, time.Now().Format("20060102")),
			Name:       fmt.Sprintf("Federation Cluster %d", i+1),
			Instances:  cluster,
			CenterNode: fa.findCenterNode(cluster, strongEdges),
			Cohesion:   fa.calculateCohesion(cluster, strongEdges),
			UpdatedAt:  time.Now(),
		}

		if err := fa.storage.StoreInstanceCluster(ctx, instanceCluster); err != nil {
			fmt.Printf("failed to store cluster %s: %v\n", instanceCluster.ClusterID, err)
		}
	}

	return nil
}

func (fa *FederationAggregator) findConnectedComponents(edges []*storage.FederationEdge) [][]string {
	// Build adjacency list
	graph := make(map[string][]string)
	for _, edge := range edges {
		if edge.Strength > 0.3 { // Only consider strong connections
			graph[edge.SourceDomain] = append(graph[edge.SourceDomain], edge.TargetDomain)
			graph[edge.TargetDomain] = append(graph[edge.TargetDomain], edge.SourceDomain)
		}
	}

	// Find connected components using DFS
	visited := make(map[string]bool)
	var clusters [][]string

	for node := range graph {
		if !visited[node] {
			cluster := []string{}
			fa.dfs(node, graph, visited, &cluster)
			if len(cluster) > 2 { // Only keep clusters with more than 2 nodes
				clusters = append(clusters, cluster)
			}
		}
	}

	return clusters
}

func (fa *FederationAggregator) dfs(node string, graph map[string][]string, visited map[string]bool, cluster *[]string) {
	visited[node] = true
	*cluster = append(*cluster, node)

	for _, neighbor := range graph[node] {
		if !visited[neighbor] {
			fa.dfs(neighbor, graph, visited, cluster)
		}
	}
}

func (fa *FederationAggregator) findCenterNode(cluster []string, edges []*storage.FederationEdge) string {
	// Find node with highest total connection strength within cluster
	strengthMap := make(map[string]float64)

	for _, edge := range edges {
		if contains(cluster, edge.SourceDomain) && contains(cluster, edge.TargetDomain) {
			strengthMap[edge.SourceDomain] += edge.Strength
			strengthMap[edge.TargetDomain] += edge.Strength
		}
	}

	var centerNode string
	var maxStrength float64
	for node, strength := range strengthMap {
		if strength > maxStrength {
			maxStrength = strength
			centerNode = node
		}
	}

	return centerNode
}

func (fa *FederationAggregator) calculateCohesion(cluster []string, edges []*storage.FederationEdge) float64 {
	// Cohesion = actual edges / possible edges within cluster
	actualEdges := 0
	for _, edge := range edges {
		if contains(cluster, edge.SourceDomain) && contains(cluster, edge.TargetDomain) {
			actualEdges++
		}
	}

	possibleEdges := len(cluster) * (len(cluster) - 1) / 2
	if possibleEdges == 0 {
		return 0
	}

	return float64(actualEdges) / float64(possibleEdges)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func main() {
	store, err := dynamodb.New()
	if err != nil {
		panic(fmt.Sprintf("Failed to create DynamoDB storage: %v", err))
	}

	aggregator := &FederationAggregator{
		storage: store,
	}

	lambda.Start(aggregator.HandleRequest)
}
