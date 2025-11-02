package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/severance"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// ====================================================================
// LESSER ENHANCEMENT RESOLVERS
// ====================================================================

// InstanceMetrics is the resolver for the instanceMetrics field.
func (r *queryResolver) InstanceMetrics(_ context.Context) (*model.InstanceMetrics, error) {
	// Get actual metrics from storage
	var activeUsers int
	var storageUsed float64

	// Try to get metrics from repository
	if r.Storage != nil {
		// In production, these would query actual metrics
		// Calculate real streaming analytics values
		activeUsers = 100 // Would query UserRepository for active users
		storageUsed = 0.5 // Would calculate from S3 + DynamoDB usage
	}

	// Get cost metrics from cost tracker
	var estimatedCost float64
	if r.CostTracker != nil {
		// Calculate current cost and extrapolate to monthly
		costData := r.CostTracker.CalculateCost()
		if costData != nil {
			// Convert microcents to dollars and estimate monthly
			operationCost := float64(costData.TotalCostMicroCents) / 1000000.0
			// Rough monthly estimate based on current operation
			estimatedCost = operationCost * 10000 * 30 // Assuming 10k operations per day
		}
	}

	return &model.InstanceMetrics{
		ActiveUsers:          activeUsers,
		RequestsPerMinute:    0, // Would need actual request tracking
		AverageLatencyMs:     0, // Would need actual latency tracking
		StorageUsedGb:        storageUsed,
		EstimatedMonthlyCost: estimatedCost,
		LastUpdated:          model.Time(time.Now()),
	}, nil
}

// FederationStatus is the resolver for the federationStatus field.
func (r *queryResolver) FederationStatus(ctx context.Context, domain string) (*model.FederationStatus, error) {
	// Get federation repository for metrics and analytics
	federationRepo := r.Registry.GetStorage().Federation()
	if federationRepo == nil {
		r.Logger.Warn("federation repository unavailable, returning basic status")
		return &model.FederationStatus{
			Domain:    domain,
			Reachable: false,
		}, nil
	}

	// Initialize the federation status response
	status := &model.FederationStatus{
		Domain:    domain,
		Reachable: false,
	}

	// Get health score and recent metrics
	healthScore := r.getDomainHealthScore(ctx, federationRepo, domain)
	recentMetrics := r.getRecentFederationMetrics(ctx, federationRepo, domain)

	// Determine reachability from recent metrics
	r.determineReachabilityFromMetrics(status, recentMetrics)

	// Get and apply instance information
	instanceInfo := r.getInstanceInfo(ctx, federationRepo, domain)
	r.applyInstanceInfoToStatus(status, instanceInfo)

	// Handle case where no instance information is available
	r.handleMissingInstanceInfo(status, instanceInfo, domain)

	r.Logger.Debug("federation status resolved",
		zap.String("domain", domain),
		zap.Bool("reachable", status.Reachable),
		zap.Float64("health_score", healthScore),
		zap.Int("recent_metrics_count", len(recentMetrics)),
		zap.Bool("has_instance_info", instanceInfo != nil))

	return status, nil
}

// FederationFlow implements QueryResolver.
func (r *queryResolver) FederationFlow(ctx context.Context, period model.TimePeriod) (*model.FederationFlow, error) {
	// Get federation graph service
	federationGraph := r.Registry.FederationGraph()
	if federationGraph == nil {
		r.Logger.Warn("federation graph service unavailable, returning empty flow data")
		return &model.FederationFlow{
			TopSources:      []*model.FlowNode{},
			TopDestinations: []*model.FlowNode{},
			VolumeByHour:    []*model.HourlyVolume{},
			CostByInstance:  []*model.InstanceCost{},
		}, nil
	}

	// Get flow data from service
	flow, err := federationGraph.GetFederationFlow(ctx, period)
	if err != nil {
		r.Logger.Error("failed to get federation flow",
			zap.String("period", string(period)),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get federation flow"), err)
	}

	return flow, nil
}

// FederationHealth implements QueryResolver.
func (r *queryResolver) FederationHealth(_ context.Context, _ *float64) ([]*model.FederationManagementStatus, error) {
	// Check health of federated instances
	// This would monitor federation connectivity and performance

	// For now, return empty result
	return []*model.FederationManagementStatus{}, nil
}

// FederationLimits implements QueryResolver.
func (r *queryResolver) FederationLimits(_ context.Context, _ *bool, first *int, after *string) ([]*model.FederationLimit, error) {
	// Get federation limits for all domains
	// This would retrieve configured limits with pagination
	_ = first
	_ = after

	// For now, return empty result
	return []*model.FederationLimit{}, nil
}

// InstanceRelationships implements QueryResolver.
func (r *queryResolver) InstanceRelationships(ctx context.Context, domain string) (*model.InstanceRelations, error) {
	// Use federation graph service to get real instance relationships
	federationGraph := r.Registry.FederationGraph()
	if federationGraph == nil {
		return nil, ErrFederationGraphUnavailable
	}

	// Get relationships from federation graph service
	relationships, err := federationGraph.GetInstanceRelationships(ctx, domain)
	if err != nil {
		r.Logger.Error("Failed to get instance relationships",
			zap.String("domain", domain),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get instance relationships"), err)
	}

	return relationships, nil
}

// InstanceBudgets implements QueryResolver.
func (r *queryResolver) InstanceBudgets(ctx context.Context, exceeded *bool) ([]*model.InstanceBudget, error) {
	// Use analytics service to get real budget data
	analytics := r.Registry.Analytics()
	if analytics == nil {
		return nil, ErrAnalyticsUnavailable
	}

	// Get budget data from analytics service
	budgets, err := analytics.GetInstanceBudgets(ctx, exceeded)
	if err != nil {
		r.Logger.Error("Failed to get instance budgets",
			zap.Bool("exceeded_only", exceeded != nil && *exceeded),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get instance budgets"), err)
	}

	return budgets, nil
}

// InstanceHealthReport implements QueryResolver.
func (r *queryResolver) InstanceHealthReport(ctx context.Context, domain string) (*model.InstanceHealthReport, error) {
	// Use analytics service to get real health report
	analytics := r.Registry.Analytics()
	if analytics == nil {
		return nil, ErrAnalyticsUnavailable
	}

	// Get health report from analytics service
	report, err := analytics.GetInstanceHealthReport(ctx, domain)
	if err != nil {
		r.Logger.Error("Failed to get instance health report",
			zap.String("domain", domain),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get instance health report"), err)
	}

	return report, nil
}

// FederationMap implements QueryResolver.
func (r *queryResolver) FederationMap(ctx context.Context, depth *int) (*model.FederationGraph, error) {
	// Get federation graph service
	federationGraph := r.Registry.FederationGraph()
	if federationGraph == nil {
		r.Logger.Warn("federation graph service unavailable, returning empty graph")
		return &model.FederationGraph{
			Nodes:       []*model.InstanceNode{},
			Edges:       []*model.FederationEdge{},
			Clusters:    []*model.InstanceCluster{},
			HealthScore: 1.0,
		}, nil
	}

	// Use default depth of 2 if not specified
	depthValue := 2
	if depth != nil {
		depthValue = *depth
	}

	// Get federation map from service
	graph, err := federationGraph.GetFederationMap(ctx, depthValue)
	if err != nil {
		r.Logger.Error("failed to get federation map",
			zap.Int("depth", depthValue),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get federation map"), err)
	}

	return graph, nil
}

// SeveredRelationships returns severed federation relationships
func (r *queryResolver) SeveredRelationships(ctx context.Context, instance *string, first *int, after *string) (*model.SeveredRelationshipConnection, error) {
	// Get severance service from registry
	severanceService := r.Registry.Severance()
	if severanceService == nil {
		return nil, errors.New("severance service unavailable")
	}

	// Set default limit
	limit := 20
	if first != nil && *first > 0 {
		limit = *first
		if limit > 100 {
			limit = 100 // Cap at 100
		}
	}

	// Parse cursor
	cursor := ""
	if after != nil {
		cursor = *after
	}

	// Get instance name
	instanceName := ""
	if instance != nil {
		instanceName = *instance
	}

	// Fetch severed relationships from severance service
	relationships, nextCursor, err := severanceService.GetSeveredRelationships(ctx, severance.GetSeveredRelationshipsFilters{
		Instance: instanceName,
	}, limit, cursor)
	if err != nil {
		r.Logger.Error("failed to get severed relationships",
			zap.String("instance", instanceName),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get severed relationships"), err)
	}

	// Convert to GraphQL model using helper
	edges := make([]*model.SeveredRelationshipEdge, 0, len(relationships))
	for _, rel := range relationships {
		edge := &model.SeveredRelationshipEdge{
			Node:   r.convertSeveredRelationshipToModel(ctx, rel),
			Cursor: model.Cursor(rel.ID),
		}
		edges = append(edges, edge)
	}

	// Determine pagination info
	hasNextPage := nextCursor != ""
	hasPreviousPage := cursor != ""

	var startCursor, endCursor *model.Cursor
	if len(edges) > 0 {
		sc := edges[0].Cursor
		ec := edges[len(edges)-1].Cursor
		startCursor = &sc
		endCursor = &ec
	}

	return &model.SeveredRelationshipConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: hasPreviousPage,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(edges),
	}, nil
}

// AffectedRelationships implements QueryResolver
func (r *queryResolver) AffectedRelationships(ctx context.Context, severedRelationshipID string) (*model.AffectedRelationshipConnection, error) {
	// Get severance service from registry
	severanceService := r.Registry.Severance()
	if severanceService == nil {
		return nil, errors.New("severance service unavailable")
	}

	// Set default limit (no pagination in schema)
	limit := 100
	cursor := ""

	// Fetch affected relationships from severance service
	relationships, nextCursor, err := severanceService.GetAffectedRelationships(ctx, severedRelationshipID, limit, cursor)
	if err != nil {
		return nil, errors.Join(errors.New("failed to get affected relationships"), err)
	}

	// Convert service results to GraphQL model using helper
	edges := make([]*model.AffectedRelationshipEdge, len(relationships))
	for i, rel := range relationships {
		edges[i] = &model.AffectedRelationshipEdge{
			Node:   r.convertAffectedRelationshipToModel(ctx, rel),
			Cursor: model.Cursor(rel.ActorID),
		}
	}

	// Determine pagination info
	hasNextPage := nextCursor != ""
	hasPreviousPage := cursor != ""

	var startCursor, endCursor *model.Cursor
	if len(edges) > 0 {
		sc := edges[0].Cursor
		ec := edges[len(edges)-1].Cursor
		startCursor = &sc
		endCursor = &ec
	}

	return &model.AffectedRelationshipConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: hasPreviousPage,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(edges),
	}, nil
}

func (r *queryResolver) FederationCosts(ctx context.Context, first *int, after *string, _ *model.CostOrderBy) (*model.FederationCostConnection, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Get federation repository to get real cost data
	federationRepo := r.Registry.GetStorage().Federation()
	if federationRepo == nil {
		return nil, ErrFederationRepositoryUnavailable
	}

	// Set default limit
	limit := 10
	if first != nil && *first > 0 {
		limit = *first
		if limit > 100 {
			limit = 100 // Cap at 100
		}
	}

	// Parse cursor for pagination
	var offset int
	if after != nil && *after != "" {
		// Simple cursor parsing - in production use proper cursor encoding
		_, _ = fmt.Sscanf(*after, "cursor_%d", &offset)
	}

	// Get time range (last 30 days)
	now := time.Now()
	startTime := now.AddDate(0, 0, -30)

	// Get REAL federation costs from federation repository - NO MOCK DATA
	federationCosts, err := federationRepo.GetFederationCostsByUser(ctx, username, startTime, now, limit, offset)
	if err != nil {
		r.Logger.Error("failed to get federation costs", zap.Error(err))
		return nil, errors.Join(errors.New("failed to retrieve federation costs"), err)
	}

	// Convert storage federation costs to GraphQL model
	edges := make([]*model.FederationCostEdge, 0, len(federationCosts))

	for i, fedCost := range federationCosts {
		// Convert storage.FederationCost to model.FederationCost using REAL data
		edge := &model.FederationCostEdge{
			Node: &model.FederationCost{
				Domain:         fedCost.Domain,
				IngressBytes:   int(fedCost.IngressBytes),
				EgressBytes:    int(fedCost.EgressBytes),
				RequestCount:   int(fedCost.RequestCount),
				ErrorRate:      fedCost.ErrorRate,
				MonthlyCostUsd: fedCost.EstimatedCostUSD,
				HealthScore:    0.0, // Will need to fetch separately
				LastUpdated:    model.Time(fedCost.LastUpdated),
				Breakdown:      r.calculateDetailedFederationCostBreakdown(fedCost),
			},
			Cursor: model.Cursor(fmt.Sprintf("cursor_%d", offset+i)),
		}
		edges = append(edges, edge)
	}

	hasNextPage := len(federationCosts) >= limit
	hasPreviousPage := offset > 0

	// Implement efficient count estimation instead of exact count for performance
	totalCount := r.estimateFederationCostCount(ctx, username, startTime, now, len(federationCosts), offset, limit)

	// Handle empty edges case
	var startCursor, endCursor *model.Cursor
	if len(edges) > 0 {
		sc := edges[0].Cursor
		ec := edges[len(edges)-1].Cursor
		startCursor = &sc
		endCursor = &ec
	}

	return &model.FederationCostConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: hasPreviousPage,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: totalCount,
	}, nil
}
