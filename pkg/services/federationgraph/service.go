// Package federationgraph provides federation graph visualization services.
// It builds and analyzes inter-instance relationships, generates graph layouts,
// calculates health scores, and provides flow analysis for federation activities.
package federationgraph

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

type federationGraphRepository interface {
	GetFederationNodes(ctx context.Context, depth int) ([]*storage.FederationNode, error)
	GetAllFederationEdges(ctx context.Context, limit int) ([]*storage.FederationEdge, error)
	GetFederationClusters(ctx context.Context, limit int) ([]*storage.InstanceCluster, error)
	GetInstanceConnections(ctx context.Context, domain string, connectionType string) ([]*storage.InstanceConnection, error)
	GetFederationEdges(ctx context.Context, domains []string) ([]*storage.FederationEdge, error)
	GetFederationActivitiesByTimeRange(ctx context.Context, start, end time.Time, limit int) ([]*models.FederationCostActivity, error)
	GetFederationCosts(ctx context.Context, start, end time.Time, limit int, cursor string) ([]*storage.FederationCost, string, error)
}

var _ federationGraphRepository = (*repositories.FederationRepository)(nil)

// Service provides federation graph visualization functionality
type Service struct {
	federationRepo federationGraphRepository
	logger         *zap.Logger
	localDomain    string
}

// NewService creates a new federation graph service
func NewService(federationRepo federationGraphRepository, logger *zap.Logger, localDomain string) *Service {
	return &Service{
		federationRepo: federationRepo,
		logger:         logger,
		localDomain:    localDomain,
	}
}

// GetFederationMap builds a graph of federation connections up to the specified depth
func (s *Service) GetFederationMap(ctx context.Context, depth int) (*model.FederationGraph, error) {
	// Validate depth
	if depth < 1 {
		depth = 1
	}
	if depth > 3 {
		depth = 3 // Cap at 3 to prevent excessive queries
	}

	// Get federation nodes (instances we know about)
	nodeModels, err := s.federationRepo.GetFederationNodes(ctx, depth)
	if err != nil {
		s.logger.Error("failed to get federation nodes",
			zap.Error(err))
		return nil, fmt.Errorf("failed to get federation nodes: %w", err)
	}

	// Get all federation edges (connections between instances)
	// Note: existing GetFederationEdges takes []string domains as parameter
	// Use GetAllFederationEdges which we just added
	edgeModels, err := s.federationRepo.GetAllFederationEdges(ctx, 1000)
	if err != nil {
		s.logger.Error("failed to get federation edges",
			zap.Error(err))
		return nil, fmt.Errorf("failed to get federation edges: %w", err)
	}

	// Get clusters
	clusterModels, err := s.federationRepo.GetFederationClusters(ctx, 50)
	if err != nil {
		s.logger.Warn("failed to get federation clusters, continuing without clusters",
			zap.Error(err))
		clusterModels = []*storage.InstanceCluster{}
	}

	// Convert nodes to GraphQL model
	nodes := s.convertNodesToGraphQL(nodeModels)

	// Convert edges to GraphQL model
	edges := s.convertEdgesToGraphQL(edgeModels)

	// Convert clusters to GraphQL model
	clusters := s.convertClustersToGraphQL(clusterModels)

	// Calculate overall health score
	healthScore := s.calculateOverallHealthScore(nodes)

	return &model.FederationGraph{
		Nodes:       nodes,
		Edges:       edges,
		Clusters:    clusters,
		HealthScore: healthScore,
	}, nil
}

// GetInstanceRelationships retrieves detailed relationships for a specific instance
func (s *Service) GetInstanceRelationships(ctx context.Context, domain string) (*model.InstanceRelations, error) {
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		return nil, err
	}

	// Get instance connections (existing method takes connectionType as parameter)
	// Get all connection types by calling with empty string
	connections, err := s.federationRepo.GetInstanceConnections(ctx, domain, "")
	if err != nil {
		s.logger.Error("failed to get instance connections",
			zap.String("domain", domain),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get instance connections: %w", err)
	}

	// Get federation edges for this instance (existing method takes []string of domains)
	edges, err := s.federationRepo.GetFederationEdges(ctx, []string{domain})
	if err != nil {
		s.logger.Warn("failed to get federation edges, continuing with connections only",
			zap.String("domain", domain),
			zap.Error(err))
		edges = []*storage.FederationEdge{}
	}

	// Separate direct and indirect connections
	directConnections := make([]*model.InstanceConnection, 0)
	indirectConnections := make([]*model.InstanceConnection, 0)

	for _, conn := range connections {
		graphConn := s.convertStorageConnectionToGraphQL(conn)
		// Direct connections are outbound, indirect are inbound
		if conn.Direction == "outbound" {
			directConnections = append(directConnections, graphConn)
		} else {
			indirectConnections = append(indirectConnections, graphConn)
		}
	}

	// Get blocked/blocking lists (would come from moderation data)
	blockedBy := []string{}
	blocking := []string{}

	// Calculate federation score
	federationScore := s.calculateFederationScore(connections, edges)

	// Generate recommendations
	recommendations := s.generateRecommendations(domain, connections, edges)

	return &model.InstanceRelations{
		Domain:              domain,
		DirectConnections:   directConnections,
		IndirectConnections: indirectConnections,
		BlockedBy:           blockedBy,
		Blocking:            blocking,
		FederationScore:     federationScore,
		Recommendations:     recommendations,
	}, nil
}

// GetFederationFlow analyzes federation activity flow over a time period
func (s *Service) GetFederationFlow(ctx context.Context, period model.TimePeriod) (*model.FederationFlow, error) {
	// Calculate time range based on period
	endTime := time.Now()
	var startTime time.Time
	switch period {
	case model.TimePeriodHour:
		startTime = endTime.Add(-1 * time.Hour)
	case model.TimePeriodDay:
		startTime = endTime.Add(-24 * time.Hour)
	case model.TimePeriodWeek:
		startTime = endTime.Add(-7 * 24 * time.Hour)
	case model.TimePeriodMonth:
		startTime = endTime.Add(-30 * 24 * time.Hour)
	default:
		startTime = endTime.Add(-24 * time.Hour)
	}

	// Get federation activities for the period
	activities, err := s.federationRepo.GetFederationActivitiesByTimeRange(ctx, startTime, endTime, 10000)
	if err != nil {
		s.logger.Error("failed to get federation activities",
			zap.Time("start", startTime),
			zap.Time("end", endTime),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get federation activities: %w", err)
	}

	// Aggregate by domain
	sourceVolumes := make(map[string]*flowStats)
	destVolumes := make(map[string]*flowStats)
	hourlyVolumes := make(map[int64]*hourlyStats)

	for _, activity := range activities {
		domain := activity.Domain

		// Track source volumes (ingress)
		if activity.Type == "ingress" {
			if _, exists := sourceVolumes[domain]; !exists {
				sourceVolumes[domain] = &flowStats{}
			}
			sourceVolumes[domain].volume++
			sourceVolumes[domain].totalSize += activity.ByteSize
		}

		// Track destination volumes (egress)
		if activity.Type == "egress" {
			if _, exists := destVolumes[domain]; !exists {
				destVolumes[domain] = &flowStats{}
			}
			destVolumes[domain].volume++
			destVolumes[domain].totalSize += activity.ByteSize
		}

		// Track hourly volumes
		hourKey := activity.Timestamp.Truncate(time.Hour).Unix()
		if _, exists := hourlyVolumes[hourKey]; !exists {
			hourlyVolumes[hourKey] = &hourlyStats{
				hour: activity.Timestamp.Truncate(time.Hour),
			}
		}
		stats := hourlyVolumes[hourKey]
		if activity.Type == "ingress" {
			stats.inbound++
		} else {
			stats.outbound++
		}
		if !activity.Success {
			stats.errors++
		}
		stats.totalLatency += float64(activity.ResponseTime)
		stats.count++
	}

	// Convert to GraphQL types and sort
	topSources := s.convertToFlowNodes(sourceVolumes, 10)
	topDestinations := s.convertToFlowNodes(destVolumes, 10)
	volumeByHour := s.convertToHourlyVolumes(hourlyVolumes)

	// Get cost breakdown by instance
	costs, _, err := s.federationRepo.GetFederationCosts(ctx, startTime, endTime, 50, "")
	if err != nil {
		s.logger.Warn("failed to get federation costs, continuing without cost data",
			zap.Error(err))
		costs = []*storage.FederationCost{}
	}
	costByInstance := s.convertToInstanceCosts(costs)

	return &model.FederationFlow{
		TopSources:      topSources,
		TopDestinations: topDestinations,
		VolumeByHour:    volumeByHour,
		CostByInstance:  costByInstance,
	}, nil
}

// Helper types

type flowStats struct {
	volume    int
	totalSize int64
}

type hourlyStats struct {
	hour         time.Time
	inbound      int
	outbound     int
	errors       int
	totalLatency float64
	count        int
}

// Conversion helpers

func (s *Service) convertNodesToGraphQL(nodeModels []*storage.FederationNode) []*model.InstanceNode {
	nodes := make([]*model.InstanceNode, 0, len(nodeModels))
	for i, node := range nodeModels {
		// Calculate simple force-directed layout coordinates
		angle := float64(i) * 2 * math.Pi / float64(len(nodeModels))
		radius := 100.0

		nodes = append(nodes, &model.InstanceNode{
			Domain:         node.Domain,
			DisplayName:    node.DisplayName,
			Software:       node.Software,
			Version:        node.Version,
			UserCount:      int(node.UserCount),
			StatusCount:    int(node.StatusCount),
			FederatingWith: int(node.ActiveConnections),
			HealthStatus:   s.convertHealthStatus(node.Health),
			Coordinates:    &model.Coordinates{X: radius * math.Cos(angle), Y: radius * math.Sin(angle)},
			Metadata:       s.convertMetadata(node),
		})
	}
	return nodes
}

func (s *Service) convertEdgesToGraphQL(edgeModels []*storage.FederationEdge) []*model.FederationEdge {
	edges := make([]*model.FederationEdge, 0, len(edgeModels))
	for _, edge := range edgeModels {
		volumePerDay := int(edge.VolumeIn+edge.VolumeOut) / 7 // Rough estimate

		edges = append(edges, &model.FederationEdge{
			Source:        edge.SourceDomain,
			Target:        edge.TargetDomain,
			Weight:        edge.Strength,
			VolumePerDay:  volumePerDay,
			ErrorRate:     1.0 - edge.SuccessRate,
			Latency:       0.0, // Would need to track separately
			Bidirectional: edge.VolumeIn > 0 && edge.VolumeOut > 0,
			HealthScore:   edge.SuccessRate,
		})
	}
	return edges
}

func (s *Service) convertClustersToGraphQL(clusterModels []*storage.InstanceCluster) []*model.InstanceCluster {
	clusters := make([]*model.InstanceCluster, 0, len(clusterModels))
	for _, cluster := range clusterModels {
		clusters = append(clusters, &model.InstanceCluster{
			ID:             cluster.ClusterID,
			Name:           cluster.Name,
			Members:        cluster.Instances,
			Commonality:    "software", // Could be enhanced
			AvgHealthScore: cluster.Cohesion,
			TotalVolume:    0, // Would need to aggregate
			Description:    cluster.Description,
		})
	}
	return clusters
}

func (s *Service) convertStorageConnectionToGraphQL(conn *storage.InstanceConnection) *model.InstanceConnection {
	return &model.InstanceConnection{
		Domain:         conn.TargetDomain,
		ConnectionType: s.convertConnectionType(conn.ConnectionType),
		Strength:       float64(conn.VolumeIn+conn.VolumeOut) / 1000.0, // Normalize
		VolumeIn:       int(conn.VolumeIn),
		VolumeOut:      int(conn.VolumeOut),
		SharedUsers:    int(0), // Would need relationship data
		LastActivity:   model.Time(conn.LastActivity),
	}
}

func (s *Service) convertToFlowNodes(stats map[string]*flowStats, limit int) []*model.FlowNode {
	// Sort by volume
	type domainStats struct {
		domain string
		stats  *flowStats
	}
	sortedStats := make([]domainStats, 0, len(stats))
	totalVolume := int64(0)
	for domain, st := range stats {
		sortedStats = append(sortedStats, domainStats{domain, st})
		totalVolume += int64(st.volume)
	}
	sort.Slice(sortedStats, func(i, j int) bool {
		return sortedStats[i].stats.volume > sortedStats[j].stats.volume
	})

	// Take top N
	if len(sortedStats) > limit {
		sortedStats = sortedStats[:limit]
	}

	// Convert to GraphQL
	nodes := make([]*model.FlowNode, 0, len(sortedStats))
	for _, ds := range sortedStats {
		percentage := 0.0
		if totalVolume > 0 {
			percentage = float64(ds.stats.volume) / float64(totalVolume) * 100.0
		}
		avgSize := int64(0)
		if ds.stats.volume > 0 {
			avgSize = ds.stats.totalSize / int64(ds.stats.volume)
		}

		nodes = append(nodes, &model.FlowNode{
			Domain:         ds.domain,
			Volume:         ds.stats.volume,
			Percentage:     percentage,
			Trend:          model.TrendStable, // Would need historical data
			AvgMessageSize: int(avgSize),
		})
	}

	return nodes
}

func (s *Service) convertToHourlyVolumes(hourlyStats map[int64]*hourlyStats) []*model.HourlyVolume {
	// Sort by hour
	hours := make([]int64, 0, len(hourlyStats))
	for h := range hourlyStats {
		hours = append(hours, h)
	}
	sort.Slice(hours, func(i, j int) bool {
		return hours[i] < hours[j]
	})

	// Convert to GraphQL
	volumes := make([]*model.HourlyVolume, 0, len(hours))
	for _, h := range hours {
		stats := hourlyStats[h]
		avgLatency := 0.0
		if stats.count > 0 {
			avgLatency = stats.totalLatency / float64(stats.count)
		}

		volumes = append(volumes, &model.HourlyVolume{
			Hour:       model.Time(stats.hour),
			Inbound:    stats.inbound,
			Outbound:   stats.outbound,
			Errors:     stats.errors,
			AvgLatency: avgLatency,
		})
	}

	return volumes
}

func (s *Service) convertToInstanceCosts(costs []*storage.FederationCost) []*model.InstanceCost {
	totalCost := 0.0
	for _, c := range costs {
		totalCost += c.EstimatedCostUSD
	}

	result := make([]*model.InstanceCost, 0, len(costs))
	for _, cost := range costs {
		percentage := 0.0
		if totalCost > 0 {
			percentage = (cost.EstimatedCostUSD / totalCost) * 100.0
		}

		result = append(result, &model.InstanceCost{
			Domain:     cost.Domain,
			CostUsd:    cost.EstimatedCostUSD,
			Percentage: percentage,
			Breakdown: &model.CostBreakdown{
				Period:           model.PeriodDay, // Use appropriate period
				TotalCost:        cost.EstimatedCostUSD,
				DynamoDBCost:     cost.EstimatedCostUSD * 0.4, // Rough estimate
				S3StorageCost:    cost.EstimatedCostUSD * 0.2, // Rough estimate
				LambdaCost:       cost.EstimatedCostUSD * 0.3, // Rough estimate
				DataTransferCost: cost.EstimatedCostUSD * 0.1, // Rough estimate
				Breakdown:        []*model.CostItem{},
			},
		})
	}

	return result
}

// Helper methods

func (s *Service) convertHealthStatus(health string) model.InstanceHealthStatus {
	switch health {
	case "healthy":
		return model.InstanceHealthStatusHealthy
	case "warning":
		return model.InstanceHealthStatusWarning
	case "critical":
		return model.InstanceHealthStatusCritical
	case "offline":
		return model.InstanceHealthStatusOffline
	default:
		return model.InstanceHealthStatusUnknown
	}
}

func (s *Service) convertMetadata(node *storage.FederationNode) *model.InstanceMetadata {
	primaryLanguage := "en"
	registrationsOpen := true
	approvalRequired := false
	monthlyActiveUsers := int(node.ActiveUsers)

	return &model.InstanceMetadata{
		FirstSeen:          model.Time(node.FirstSeen),
		LastActivity:       model.Time(node.LastSeen),
		MonthlyActiveUsers: monthlyActiveUsers,
		RegistrationsOpen:  registrationsOpen,
		ApprovalRequired:   approvalRequired,
		PrimaryLanguage:    primaryLanguage,
		Description:        &node.Description,
	}
}

func (s *Service) convertConnectionType(connType string) model.ConnectionType {
	switch connType {
	case "follows":
		return model.ConnectionTypeFollows
	case "mentions":
		return model.ConnectionTypeMentions
	case "replies":
		return model.ConnectionTypeReplies
	case "boosts":
		return model.ConnectionTypeBoosts
	case "quotes":
		return model.ConnectionTypeQuotes
	default:
		return model.ConnectionTypeMixed
	}
}

func (s *Service) calculateOverallHealthScore(nodes []*model.InstanceNode) float64 {
	if len(nodes) == 0 {
		return 1.0
	}

	totalScore := 0.0
	for _, node := range nodes {
		switch node.HealthStatus {
		case model.InstanceHealthStatusHealthy:
			totalScore += 1.0
		case model.InstanceHealthStatusWarning:
			totalScore += 0.7
		case model.InstanceHealthStatusCritical:
			totalScore += 0.3
		case model.InstanceHealthStatusOffline:
			totalScore += 0.0
		default:
			totalScore += 0.5
		}
	}

	return totalScore / float64(len(nodes))
}

func (s *Service) calculateFederationScore(connections []*storage.InstanceConnection, edges []*storage.FederationEdge) float64 {
	if len(connections) == 0 && len(edges) == 0 {
		return 0.0
	}

	// Calculate based on connection count and success rates
	score := 0.0

	// Connection count component (normalized to 0-50)
	connectionScore := math.Min(float64(len(connections))/10.0, 50.0)
	score += connectionScore

	// Success rate component (0-50)
	if len(edges) > 0 {
		totalSuccessRate := 0.0
		for _, edge := range edges {
			totalSuccessRate += edge.SuccessRate
		}
		avgSuccessRate := totalSuccessRate / float64(len(edges))
		score += avgSuccessRate * 50.0
	} else {
		score += 25.0 // Default middle score if no edges
	}

	// Normalize to 0.0-1.0
	return score / 100.0
}

func (s *Service) generateRecommendations(_ string, connections []*storage.InstanceConnection, edges []*storage.FederationEdge) []*model.FederationRecommendation {
	recommendations := make([]*model.FederationRecommendation, 0)

	// Check connection health
	lowSuccessRates := 0
	for _, edge := range edges {
		if edge.SuccessRate < 0.8 {
			lowSuccessRates++
		}
	}

	if lowSuccessRates > len(edges)/3 {
		recommendations = append(recommendations, &model.FederationRecommendation{
			Type:            model.RecommendationTypePerformance,
			Priority:        model.PriorityHigh,
			Reason:          "Multiple instances showing low success rates",
			PotentialImpact: "Federation reliability and user experience degraded",
			Action:          "Investigate network connectivity and instance health",
		})
	}

	// Check for low connectivity
	if len(connections) < 5 {
		recommendations = append(recommendations, &model.FederationRecommendation{
			Type:            model.RecommendationTypeConnectivity,
			Priority:        model.PriorityMedium,
			Reason:          "Low number of federated connections",
			PotentialImpact: "Limited content discovery and user reach",
			Action:          "Consider joining relays or promoting instance visibility",
		})
	}

	// Cost optimization suggestion
	if len(edges) > 50 {
		recommendations = append(recommendations, &model.FederationRecommendation{
			Type:            model.RecommendationTypeCost,
			Priority:        model.PriorityLow,
			Reason:          "High number of active connections",
			PotentialImpact: "Increased federation costs",
			Action:          "Review connection patterns and consider rate limiting",
		})
	}

	return recommendations
}
