package repositories

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// FederationRepository implements federation tracking operations using DynamORM
type FederationRepository struct {
	db     core.DB
	logger *zap.Logger
}

// NewFederationRepository creates a new federation repository
func NewFederationRepository(db core.DB, logger *zap.Logger) *FederationRepository {
	return &FederationRepository{
		db:     db,
		logger: logger,
	}
}

// GetInstanceInfo retrieves information about a federated instance
func (r *FederationRepository) GetInstanceInfo(ctx context.Context, domain string) (*storage.InstanceInfo, error) {
	var instance models.FederationInstance
	
	err := r.db.WithContext(ctx).Model(&models.FederationInstance{}).
		Where("PK", "=", fmt.Sprintf("INSTANCE#%s", domain)).
		Where("SK", "=", fmt.Sprintf("INSTANCE#%s", domain)).
		First(&instance)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		r.logger.Error("Failed to get instance info",
			zap.String("domain", domain),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get instance info: %w", err)
	}
	
	// Convert to storage type
	return &storage.InstanceInfo{
		Domain:        instance.Domain,
		Software:      instance.Software,
		Version:       instance.Version,
		FirstSeen:     instance.FirstSeen,
		LastSeen:      instance.LastSeen,
		PublicKey:     instance.PublicKey,
		SharedInbox:   instance.SharedInbox,
		TrustScore:    instance.TrustScore,
		ActiveUsers:   int64(instance.ActiveUsers),
		TotalMessages: instance.TotalMessages,
	}, nil
}

// UpsertInstanceInfo creates or updates instance information
func (r *FederationRepository) UpsertInstanceInfo(ctx context.Context, info *storage.InstanceInfo) error {
	// Convert to model
	instance := &models.FederationInstance{
		Domain:        info.Domain,
		Software:      info.Software,
		Version:       info.Version,
		FirstSeen:     info.FirstSeen,
		LastSeen:      info.LastSeen,
		PublicKey:     info.PublicKey,
		SharedInbox:   info.SharedInbox,
		TrustScore:    info.TrustScore,
		ActiveUsers:   int(info.ActiveUsers),
		TotalMessages: info.TotalMessages,
	}
	
	// Update keys
	instance.UpdateKeys()
	
	// Use Create to upsert (it will overwrite existing)
	err := r.db.WithContext(ctx).Model(instance).Create()
	if err != nil {
		r.logger.Error("Failed to upsert instance info",
			zap.String("domain", info.Domain),
			zap.Error(err))
		return fmt.Errorf("failed to upsert instance info: %w", err)
	}
	
	return nil
}

// GetKnownInstances retrieves a list of known federated instances
func (r *FederationRepository) GetKnownInstances(ctx context.Context, limit int, cursor string) ([]*storage.InstanceInfo, string, error) {
	query := r.db.WithContext(ctx).Model(&models.FederationInstance{}).
		Index("gsi1").
		Where("GSI1PK", "=", "FEDERATION_ACTIVE").
		Limit(limit)
	
	var instances []models.FederationInstance
	err := query.Scan(&instances)
	if err != nil {
		r.logger.Error("Failed to query known instances",
			zap.Int("limit", limit),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to query known instances: %w", err)
	}
	
	// Convert to storage types
	result := make([]*storage.InstanceInfo, len(instances))
	for i, instance := range instances {
		result[i] = &storage.InstanceInfo{
			Domain:        instance.Domain,
			Software:      instance.Software,
			Version:       instance.Version,
			FirstSeen:     instance.FirstSeen,
			LastSeen:      instance.LastSeen,
			PublicKey:     instance.PublicKey,
			SharedInbox:   instance.SharedInbox,
			TrustScore:    instance.TrustScore,
			ActiveUsers:   int64(instance.ActiveUsers),
			TotalMessages: instance.TotalMessages,
		}
	}
	
	// For now, return empty cursor as DynamORM doesn't expose last evaluated key directly
	nextCursor := ""
	
	return result, nextCursor, nil
}

// GetFederationStatistics retrieves federation statistics for a time range
func (r *FederationRepository) GetFederationStatistics(ctx context.Context, startTime, endTime time.Time) (*storage.FederationStats, error) {
	// Query instances active within the time range
	var instances []models.FederationInstance
	
	err := r.db.WithContext(ctx).Model(&models.FederationInstance{}).
		Index("gsi1").
		Where("GSI1PK", "=", "FEDERATION_ACTIVE").
		Where("GSI1SK", ">=", startTime.Format(time.RFC3339)).
		Where("GSI1SK", "<=", endTime.Format(time.RFC3339)).
		Scan(&instances)
	
	if err != nil {
		r.logger.Error("Failed to query federation statistics",
			zap.Time("start", startTime),
			zap.Time("end", endTime),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query federation statistics: %w", err)
	}
	
	// Aggregate statistics
	stats := &storage.FederationStats{
		ActiveInstances: int64(len(instances)),
		TotalMessages:   0,
		TotalUsers:      0,
	}
	
	for _, instance := range instances {
		stats.TotalMessages += instance.TotalMessages
		stats.TotalUsers += int64(instance.ActiveUsers)
	}
	
	return stats, nil
}

// GetInstanceStats retrieves comprehensive statistics for a specific instance
func (r *FederationRepository) GetInstanceStats(ctx context.Context, domain string) (*storage.InstanceStats, error) {
	// Get instance info
	instanceInfo, err := r.GetInstanceInfo(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance info: %w", err)
	}
	
	// Get recent activities for error rate calculation
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour)
	
	pk := fmt.Sprintf("FEDERATION#%s#%s", domain, startTime.Format("2006-01"))
	
	var activities []models.FederationCostActivity
	err = r.db.WithContext(ctx).Model(&models.FederationCostActivity{}).
		Where("PK", "=", pk).
		Where("SK", ">=", fmt.Sprintf("ACTIVITY#%s", startTime.Format("20060102150405"))).
		Limit(1000).
		Scan(&activities)
	
	if err != nil {
		r.logger.Debug("Failed to query recent activities",
			zap.String("domain", domain),
			zap.Error(err))
		// Don't fail, just continue with partial stats
	}
	
	// Calculate metrics
	var totalResponseTime int64
	var errorCount int64
	var successCount int64
	
	for _, activity := range activities {
		totalResponseTime += activity.ResponseTime
		if activity.Success {
			successCount++
		} else {
			errorCount++
		}
	}
	
	totalRequests := successCount + errorCount
	avgResponseTime := float64(0)
	errorRate := float64(0)
	
	if totalRequests > 0 {
		avgResponseTime = float64(totalResponseTime) / float64(totalRequests)
		errorRate = float64(errorCount) / float64(totalRequests)
	}
	
	// Build stats
	stats := &storage.InstanceStats{
		Domain:          domain,
		Software:        instanceInfo.Software,
		Version:         instanceInfo.Version,
		ActiveUsers:     instanceInfo.ActiveUsers,
		TotalMessages:   instanceInfo.TotalMessages,
		FirstSeen:       instanceInfo.FirstSeen,
		LastSeen:        instanceInfo.LastSeen,
		TrustScore:      instanceInfo.TrustScore,
		ErrorRate:       errorRate,
		AvgResponseTime: avgResponseTime,
		TotalRequests:   totalRequests,
		LastDayStats: &storage.DayStats{
			Messages:     successCount,
			Errors:       errorCount,
			ResponseTime: avgResponseTime,
		},
	}
	
	return stats, nil
}

// RecordFederationActivity records a single federation activity for cost tracking
func (r *FederationRepository) RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error {
	if activity.ID == "" {
		activity.ID = fmt.Sprintf("fed_activity_%s", generateFederationRandomString(12))
	}
	
	// Convert to model
	fedActivity := &models.FederationCostActivity{
		ID:           activity.ID,
		Domain:       activity.Domain,
		Type:         activity.Type,
		ActivityType: activity.ActivityType,
		ByteSize:     activity.ByteSize,
		Success:      activity.Success,
		ResponseTime: activity.ResponseTime,
		ErrorMessage: activity.ErrorMessage,
		Timestamp:    activity.Timestamp,
	}
	
	// Update keys
	fedActivity.UpdateKeys()
	
	// Store the activity
	err := r.db.WithContext(ctx).Model(fedActivity).Create()
	if err != nil {
		r.logger.Error("Failed to record federation activity",
			zap.String("domain", activity.Domain),
			zap.String("type", activity.Type),
			zap.Error(err))
		return fmt.Errorf("failed to record federation activity: %w", err)
	}
	
	// Update aggregated costs asynchronously
	go r.updateAggregatedCosts(context.Background(), activity)
	
	return nil
}

// GetFederationCosts retrieves aggregated federation costs
func (r *FederationRepository) GetFederationCosts(ctx context.Context, startTime, endTime time.Time, limit int, cursor string) ([]*storage.FederationCost, string, error) {
	// Query aggregated monthly costs
	pk := fmt.Sprintf("FEDERATION_COSTS#%s", startTime.Format("2006-01"))
	
	query := r.db.WithContext(ctx).Model(&models.FederationCost{}).
		Where("PK", "=", pk).
		Limit(limit)
	
	var costRecords []models.FederationCost
	err := query.Scan(&costRecords)
	if err != nil {
		r.logger.Error("Failed to query federation costs",
			zap.Time("start", startTime),
			zap.Time("end", endTime),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to query federation costs: %w", err)
	}
	
	// Convert to storage types
	costs := make([]*storage.FederationCost, len(costRecords))
	for i, record := range costRecords {
		costs[i] = &storage.FederationCost{
			Domain:           record.Domain,
			Period:           record.Period,
			IngressBytes:     record.IngressBytes,
			EgressBytes:      record.EgressBytes,
			RequestCount:     record.RequestCount,
			ErrorCount:       record.ErrorCount,
			ErrorRate:        record.ErrorRate,
			AvgResponseTime:  record.AvgResponseTime,
			EstimatedCostUSD: record.EstimatedCostUSD,
			LastUpdated:      record.LastUpdated,
		}
	}
	
	// For now, return empty cursor as DynamORM doesn't expose last evaluated key directly
	nextCursor := ""
	
	return costs, nextCursor, nil
}

// GetInstanceHealthReport generates a health report for a specific instance
func (r *FederationRepository) GetInstanceHealthReport(ctx context.Context, domain string, period time.Duration) (*storage.InstanceHealthReport, error) {
	// Calculate time range
	endTime := time.Now()
	startTime := endTime.Add(-period)
	
	// Query recent activities for this domain
	pk := fmt.Sprintf("FEDERATION#%s#%s", domain, startTime.Format("2006-01"))
	
	var activities []models.FederationCostActivity
	err := r.db.WithContext(ctx).Model(&models.FederationCostActivity{}).
		Where("PK", "=", pk).
		Where("SK", ">=", fmt.Sprintf("ACTIVITY#%s", startTime.Format("20060102150405"))).
		Limit(1000). // Sample up to 1000 recent activities
		Scan(&activities)
	
	if err != nil {
		r.logger.Error("Failed to query instance activities",
			zap.String("domain", domain),
			zap.Duration("period", period),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query instance activities: %w", err)
	}
	
	// Calculate metrics
	var totalResponseTime int64
	var errorCount int64
	var successCount int64
	issues := []string{}
	recommendations := []string{}
	
	for _, activity := range activities {
		totalResponseTime += activity.ResponseTime
		if activity.Success {
			successCount++
		} else {
			errorCount++
		}
	}
	
	totalRequests := successCount + errorCount
	avgResponseTime := float64(0)
	errorRate := float64(0)
	
	if totalRequests > 0 {
		avgResponseTime = float64(totalResponseTime) / float64(totalRequests)
		errorRate = float64(errorCount) / float64(totalRequests)
	}
	
	// Determine status and generate recommendations
	status := "healthy"
	if errorRate > 0.1 {
		status = "critical"
		issues = append(issues, fmt.Sprintf("High error rate: %.2f%%", errorRate*100))
		recommendations = append(recommendations, "Consider temporarily blocking or rate limiting this instance")
	} else if errorRate > 0.05 {
		status = "warning"
		issues = append(issues, fmt.Sprintf("Elevated error rate: %.2f%%", errorRate*100))
		recommendations = append(recommendations, "Monitor this instance closely")
	}
	
	if avgResponseTime > 5000 { // 5 seconds
		if status == "healthy" {
			status = "warning"
		}
		issues = append(issues, fmt.Sprintf("Slow response time: %.2fs", avgResponseTime/1000))
		recommendations = append(recommendations, "Enable request caching for this instance")
	}
	
	// Estimate queue depth based on recent activity patterns
	queueDepth := int(math.Min(float64(errorCount)*2, 1000))
	
	report := &storage.InstanceHealthReport{
		Domain:          domain,
		Status:          status,
		ResponseTime:    avgResponseTime,
		ErrorRate:       errorRate,
		FederationDelay: avgResponseTime / 1000, // Convert to seconds
		QueueDepth:      int64(queueDepth),
		Issues:          issues,
		Recommendations: recommendations,
		LastChecked:     time.Now(),
	}
	
	return report, nil
}

// GetCostProjections generates cost projections based on historical data
func (r *FederationRepository) GetCostProjections(ctx context.Context, period string) (*storage.CostProjection, error) {
	// Get current month's costs
	currentMonth := time.Now().Format("2006-01")
	pk := fmt.Sprintf("FEDERATION_COSTS#%s", currentMonth)
	
	var costRecords []models.FederationCost
	err := r.db.WithContext(ctx).Model(&models.FederationCost{}).
		Where("PK", "=", pk).
		Scan(&costRecords)
	
	if err != nil {
		r.logger.Error("Failed to query current costs",
			zap.String("period", period),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query current costs: %w", err)
	}
	
	// Calculate current total cost
	currentCost := float64(0)
	domainCosts := make(map[string]float64)
	
	for _, record := range costRecords {
		currentCost += record.EstimatedCostUSD
		domainCosts[record.Domain] += record.EstimatedCostUSD
	}
	
	// Simple projection: assume 15% growth rate
	growthRate := 0.15
	projectedCost := currentCost * (1 + growthRate)
	
	// Identify top cost drivers
	topDrivers := []storage.CostDriver{}
	
	for domain, cost := range domainCosts {
		driver := storage.CostDriver{
			Type:           "Federation Traffic",
			Domain:         domain,
			Cost:           cost,
			PercentOfTotal: (cost / currentCost) * 100,
			Trend:          "stable", // Would need historical data to determine trend
		}
		topDrivers = append(topDrivers, driver)
	}
	
	// Sort and limit to top 3 drivers
	// In production, would use a proper sorting algorithm
	if len(topDrivers) > 3 {
		topDrivers = topDrivers[:3]
	}
	
	recommendations := []string{
		"Enable progressive media loading to reduce bandwidth costs",
		"Implement federation rate limiting for high-traffic instances",
		"Consider archiving old media to cheaper storage tiers",
	}
	
	projection := &storage.CostProjection{
		Period:          period,
		CurrentCost:     currentCost,
		ProjectedCost:   projectedCost,
		Variance:        growthRate,
		TopDrivers:      topDrivers,
		Recommendations: recommendations,
	}
	
	return projection, nil
}

// GetFederationNodes retrieves federation nodes up to a certain depth
func (r *FederationRepository) GetFederationNodes(ctx context.Context, depth int) ([]*storage.FederationNode, error) {
	// Query active federation instances
	var nodes []models.FederationNode
	
	err := r.db.WithContext(ctx).Model(&models.FederationNode{}).
		Index("gsi1").
		Where("GSI1PK", "=", "FEDERATION_ACTIVE").
		Limit(100). // Limit to 100 nodes initially
		Scan(&nodes)
	
	if err != nil {
		r.logger.Error("Failed to query federation nodes",
			zap.Int("depth", depth),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query federation nodes: %w", err)
	}
	
	// Convert to storage types
	result := make([]*storage.FederationNode, len(nodes))
	for i, node := range nodes {
		result[i] = &storage.FederationNode{
			Domain:            node.Domain,
			DisplayName:       node.DisplayName,
			Description:       node.Description,
			Software:          node.Software,
			Version:           node.Version,
			UserCount:         node.UserCount,
			StatusCount:       node.StatusCount,
			ActiveUsers:       node.ActiveUsers,
			FirstSeen:         node.FirstSeen,
			LastSeen:          node.LastSeen,
			Health:            node.Health,
			ErrorRate:         node.ErrorRate,
			ResponseTime:      node.ResponseTime,
			ConnectionType:    node.ConnectionType,
			TotalConnections:  node.TotalConnections,
			ActiveConnections: node.ActiveConnections,
			ActivityVolume:    node.ActivityVolume,
			Metadata:          node.Metadata,
		}
	}
	
	return result, nil
}

// GetFederationEdges retrieves edges between specified domains
func (r *FederationRepository) GetFederationEdges(ctx context.Context, domains []string) ([]*storage.FederationEdge, error) {
	if len(domains) == 0 {
		return []*storage.FederationEdge{}, nil
	}
	
	// Build batch get items for all possible domain pairs
	edges := make([]*storage.FederationEdge, 0)
	
	// Query edges for each domain pair
	for i, source := range domains {
		for j, target := range domains {
			if i != j {
				var edge models.FederationEdge
				err := r.db.WithContext(ctx).Model(&models.FederationEdge{}).
					Where("PK", "=", fmt.Sprintf("FEDERATION_EDGE#%s", source)).
					Where("SK", "=", target).
					First(&edge)
				
				if err != nil {
					if !errors.IsNotFound(err) {
						r.logger.Debug("Failed to get edge",
							zap.String("source", source),
							zap.String("target", target),
							zap.Error(err))
					}
					continue
				}
				
				edges = append(edges, &storage.FederationEdge{
					SourceDomain:   edge.SourceDomain,
					TargetDomain:   edge.TargetDomain,
					ConnectionType: edge.ConnectionType,
					VolumeIn:       edge.VolumeIn,
					VolumeOut:      edge.VolumeOut,
					Strength:       edge.Strength,
					LastActivity:   edge.LastActivity,
					SharedUsers:    edge.SharedUsers,
					ErrorCount:     edge.ErrorCount,
					SuccessRate:    edge.SuccessRate,
				})
			}
		}
	}
	
	return edges, nil
}

// GetInstanceMetadata retrieves metadata for a specific instance
func (r *FederationRepository) GetInstanceMetadata(ctx context.Context, domain string) (*storage.InstanceMetadata, error) {
	var metadata models.InstanceMetadata
	
	err := r.db.WithContext(ctx).Model(&models.InstanceMetadata{}).
		Where("PK", "=", fmt.Sprintf("INSTANCE_META#%s", domain)).
		Where("SK", "=", "METADATA").
		First(&metadata)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		r.logger.Error("Failed to get instance metadata",
			zap.String("domain", domain),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get instance metadata: %w", err)
	}
	
	// Convert to storage type
	return &storage.InstanceMetadata{
		Domain:          metadata.Domain,
		DisplayName:     metadata.DisplayName,
		Description:     metadata.Description,
		Software:        metadata.Software,
		Version:         metadata.Version,
		UserCount:       metadata.UserCount,
		StatusCount:     metadata.StatusCount,
		NodeInfo:        metadata.NodeInfo,
		InstanceInfo:    metadata.InstanceInfo,
		AdminContact:    metadata.AdminContact,
		Rules:           metadata.Rules,
		Languages:       metadata.Languages,
		Categories:      metadata.Categories,
		FederationNotes: metadata.FederationNotes,
		LastUpdated:     metadata.LastUpdated,
	}, nil
}

// CalculateFederationClusters calculates instance clusters based on connections
func (r *FederationRepository) CalculateFederationClusters(ctx context.Context) ([]*storage.InstanceCluster, error) {
	// This is a complex operation that would typically be done in a batch job
	// For now, return pre-calculated clusters stored in DynamoDB
	
	var clusters []models.InstanceCluster
	
	err := r.db.WithContext(ctx).Model(&models.InstanceCluster{}).
		Where("PK", "=", "FEDERATION_CLUSTER#CLUSTERS").
		Limit(50). // Limit to 50 clusters
		Scan(&clusters)
	
	if err != nil {
		r.logger.Error("Failed to query federation clusters", zap.Error(err))
		return nil, fmt.Errorf("failed to query federation clusters: %w", err)
	}
	
	// Convert to storage types
	result := make([]*storage.InstanceCluster, len(clusters))
	for i, cluster := range clusters {
		result[i] = &storage.InstanceCluster{
			ClusterID:   cluster.ClusterID,
			Name:        cluster.Name,
			Instances:   cluster.Instances,
			CenterNode:  cluster.CenterNode,
			Cohesion:    cluster.Cohesion,
			Size:        cluster.Size,
			Description: cluster.Description,
			UpdatedAt:   cluster.UpdatedAt,
		}
	}
	
	return result, nil
}

// GetInstanceConnections retrieves connections for a specific instance
func (r *FederationRepository) GetInstanceConnections(ctx context.Context, domain string, connectionType string) ([]*storage.InstanceConnection, error) {
	// Query using GSI2 for instance connections
	pkValue := fmt.Sprintf("INSTANCE#%s#CONNECTIONS", domain)
	if connectionType != "" {
		pkValue = fmt.Sprintf("INSTANCE#%s#CONNECTIONS#%s", domain, connectionType)
	}
	
	var connections []models.InstanceConnection
	
	err := r.db.WithContext(ctx).Model(&models.InstanceConnection{}).
		Index("gsi2").
		Where("GSI2PK", "=", pkValue).
		Limit(100).
		Scan(&connections)
	
	if err != nil {
		r.logger.Error("Failed to query instance connections",
			zap.String("domain", domain),
			zap.String("connectionType", connectionType),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query instance connections: %w", err)
	}
	
	// Convert to storage types
	result := make([]*storage.InstanceConnection, len(connections))
	for i, conn := range connections {
		result[i] = &storage.InstanceConnection{
			Domain:         conn.Domain,
			TargetDomain:   conn.TargetDomain,
			Direction:      conn.Direction,
			ConnectionType: conn.ConnectionType,
			VolumeIn:       conn.VolumeIn,
			VolumeOut:      conn.VolumeOut,
			LastActivity:   conn.LastActivity,
			Success:        conn.Success,
		}
	}
	
	return result, nil
}

// updateAggregatedCosts updates the aggregated cost data asynchronously
func (r *FederationRepository) updateAggregatedCosts(ctx context.Context, activity *storage.FederationActivity) {
	// Get current aggregated cost record
	pk := fmt.Sprintf("FEDERATION_COSTS#%s", time.Now().Format("2006-01"))
	sk := fmt.Sprintf("DOMAIN#%s", activity.Domain)
	
	var cost models.FederationCost
	err := r.db.WithContext(ctx).Model(&models.FederationCost{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&cost)
	
	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("Failed to get existing cost record",
			zap.String("domain", activity.Domain),
			zap.Error(err))
		return
	}
	
	// Initialize if not found
	if errors.IsNotFound(err) {
		cost = models.FederationCost{
			Domain:      activity.Domain,
			Period:      "monthly",
			LastUpdated: time.Now(),
		}
	}
	
	// Update metrics
	if activity.Type == "ingress" {
		cost.IngressBytes += activity.ByteSize
	} else {
		cost.EgressBytes += activity.ByteSize
	}
	
	cost.RequestCount++
	if !activity.Success {
		cost.ErrorCount++
	}
	
	// Update error rate
	if cost.RequestCount > 0 {
		cost.ErrorRate = float64(cost.ErrorCount) / float64(cost.RequestCount)
	}
	
	// Update average response time (simple moving average)
	cost.AvgResponseTime = (cost.AvgResponseTime*float64(cost.RequestCount-1) + float64(activity.ResponseTime)) / float64(cost.RequestCount)
	
	// Estimate cost (simplified calculation)
	// $0.09 per GB data transfer + $0.20 per million requests
	dataTransferGB := float64(cost.IngressBytes+cost.EgressBytes) / (1024 * 1024 * 1024)
	requestMillions := float64(cost.RequestCount) / 1000000
	cost.EstimatedCostUSD = (dataTransferGB * 0.09) + (requestMillions * 0.20)
	
	// Update keys
	cost.UpdateKeys()
	
	// Save updated cost record
	err = r.db.WithContext(ctx).Model(&cost).Create()
	if err != nil {
		r.logger.Error("Failed to update aggregated costs",
			zap.String("domain", activity.Domain),
			zap.Error(err))
	}
}

// AcknowledgeSeverance marks a severance as acknowledged by the user
func (r *FederationRepository) AcknowledgeSeverance(ctx context.Context, userID, domain string) error {
	// Get the existing severance record
	var severance models.FederationSeverance
	err := r.db.WithContext(ctx).Model(&models.FederationSeverance{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", userID)).
		Where("SK", "=", fmt.Sprintf("SEVERANCE#%s", domain)).
		First(&severance)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return storage.ErrNotFound
		}
		r.logger.Error("Failed to get severance record",
			zap.String("user_id", userID),
			zap.String("domain", domain),
			zap.Error(err))
		return fmt.Errorf("failed to get severance record: %w", err)
	}
	
	// Update acknowledged status
	severance.Acknowledged = true
	severance.UpdateKeys()
	
	// Save the updated record
	err = r.db.WithContext(ctx).Model(&severance).Create()
	if err != nil {
		r.logger.Error("Failed to acknowledge severance",
			zap.String("user_id", userID),
			zap.String("domain", domain),
			zap.Error(err))
		return fmt.Errorf("failed to acknowledge severance: %w", err)
	}
	
	r.logger.Info("Severance acknowledged",
		zap.String("user_id", userID),
		zap.String("domain", domain))
	
	return nil
}

// AttemptReconnection records an attempt to reconnect to a severed domain
func (r *FederationRepository) AttemptReconnection(ctx context.Context, userID, domain string) error {
	now := time.Now()
	
	attempt := &models.ReconnectionAttempt{
		UserID:      userID,
		Domain:      domain,
		AttemptedAt: now,
		Success:     false, // Will be updated if successful
		Method:      "manual",
	}
	
	// Update keys
	attempt.UpdateKeys()
	
	// Save the attempt
	err := r.db.WithContext(ctx).Model(attempt).Create()
	if err != nil {
		r.logger.Error("Failed to record reconnection attempt",
			zap.String("user_id", userID),
			zap.String("domain", domain),
			zap.Error(err))
		return fmt.Errorf("failed to record reconnection attempt: %w", err)
	}
	
	// TODO: Implement actual reconnection logic here
	// This would involve:
	// 1. Testing connectivity to the domain
	// 2. Attempting to re-establish federation
	// 3. Updating the attempt record with results
	
	r.logger.Info("Reconnection attempt recorded",
		zap.String("user_id", userID),
		zap.String("domain", domain))
	
	return nil
}

// GetUserSeveredRelationships returns all severed relationships for a user
func (r *FederationRepository) GetUserSeveredRelationships(ctx context.Context, userID string) ([]*storage.SeveredRelationship, error) {
	var severances []models.FederationSeverance
	
	err := r.db.WithContext(ctx).Model(&models.FederationSeverance{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", userID)).
		Filter("SK", "BEGINS_WITH", "SEVERANCE#").
		Scan(&severances)
	
	if err != nil {
		r.logger.Error("Failed to query severed relationships",
			zap.String("user_id", userID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query severed relationships: %w", err)
	}
	
	// Convert to storage types
	relationships := make([]*storage.SeveredRelationship, len(severances))
	for i, sev := range severances {
		relationships[i] = &storage.SeveredRelationship{
			Domain:       sev.Domain,
			SeveredAt:    sev.SeveredAt,
			Acknowledged: sev.Acknowledged,
			Reason:       sev.Reason,
			Type:         sev.Type,
		}
	}
	
	return relationships, nil
}

// GetAffectedRelationships returns relationships affected by domain severance
func (r *FederationRepository) GetAffectedRelationships(ctx context.Context, userID, domain string) ([]*storage.RelationshipRecord, error) {
	// This is a complex query that would ideally use a GSI
	// For now, we'll implement a basic scan with filters
	// In production, you'd want to ensure proper indexing exists
	
	var relationships []models.Follow
	
	// Query follows where the user follows someone from the severed domain
	err := r.db.WithContext(ctx).Model(&models.Follow{}).
		Where("PK", "=", fmt.Sprintf("follow#%s", userID)).
		Filter("SK", "BEGINS_WITH", "following#").
		Scan(&relationships)
	
	if err != nil {
		r.logger.Error("Failed to query affected relationships",
			zap.String("user_id", userID),
			zap.String("domain", domain),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query affected relationships: %w", err)
	}
	
	// Filter for the specific domain and convert to storage type
	affected := make([]*storage.RelationshipRecord, 0)
	domainSuffix := "@" + domain
	
	for _, rel := range relationships {
		if strings.HasSuffix(rel.FollowedUsername, domainSuffix) {
			affected = append(affected, &storage.RelationshipRecord{
				PK:         rel.PK,
				SK:         rel.SK,
				GSI1PK:     rel.GSI1PK,
				GSI1SK:     rel.GSI1SK,
				ActivityID: rel.ActivityID,
				State:      rel.State,
				CreatedAt:  rel.CreatedAt,
				UpdatedAt:  rel.UpdatedAt,
			})
		}
	}
	
	return affected, nil
}

// TrackFederationIssue records a federation issue for monitoring
func (r *FederationRepository) TrackFederationIssue(ctx context.Context, domain, issueType string) error {
	now := time.Now()
	
	issue := &models.FederationIssue{
		Domain:    domain,
		IssueType: issueType,
		Timestamp: now,
		Severity:  r.determineSeverity(issueType),
		Resolved:  false,
	}
	
	// Update keys
	issue.UpdateKeys()
	
	// Save the issue
	err := r.db.WithContext(ctx).Model(issue).Create()
	if err != nil {
		r.logger.Error("Failed to track federation issue",
			zap.String("domain", domain),
			zap.String("issue_type", issueType),
			zap.Error(err))
		return fmt.Errorf("failed to track federation issue: %w", err)
	}
	
	r.logger.Warn("Federation issue tracked",
		zap.String("domain", domain),
		zap.String("issue_type", issueType),
		zap.String("severity", issue.Severity))
	
	return nil
}

// GetRecentInstanceConnections retrieves connections for an instance within a time window
func (r *FederationRepository) GetRecentInstanceConnections(ctx context.Context, domain string, since time.Duration) ([]*storage.InstanceConnection, error) {
	cutoffTime := time.Now().Add(-since)
	
	var connections []models.InstanceConnection
	
	err := r.db.WithContext(ctx).Model(&models.InstanceConnection{}).
		Index("gsi2").
		Where("GSI2PK", "=", fmt.Sprintf("INSTANCE#%s#CONNECTIONS", domain)).
		Where("GSI2SK", ">", fmt.Sprintf("%d", cutoffTime.Unix())).
		Limit(1000).
		Scan(&connections)
	
	if err != nil {
		r.logger.Error("Failed to query recent connections",
			zap.String("domain", domain),
			zap.Duration("since", since),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query recent connections: %w", err)
	}
	
	// Filter by cutoff time and convert to storage types
	result := make([]*storage.InstanceConnection, 0, len(connections))
	for _, conn := range connections {
		if conn.LastActivity.After(cutoffTime) {
			result = append(result, &storage.InstanceConnection{
				Domain:         conn.Domain,
				TargetDomain:   conn.TargetDomain,
				Direction:      conn.Direction,
				ConnectionType: conn.ConnectionType,
				VolumeIn:       conn.VolumeIn,
				VolumeOut:      conn.VolumeOut,
				LastActivity:   conn.LastActivity,
				Success:        conn.Success,
			})
		}
	}
	
	return result, nil
}

// UpdateFederationNode updates or creates a federation node
func (r *FederationRepository) UpdateFederationNode(ctx context.Context, node *storage.FederationNode) error {
	if node.Domain == "" {
		return fmt.Errorf("domain is required")
	}
	
	// Convert to model
	modelNode := &models.FederationNode{
		Domain:            node.Domain,
		DisplayName:       node.DisplayName,
		Description:       node.Description,
		Software:          node.Software,
		Version:           node.Version,
		UserCount:         node.UserCount,
		StatusCount:       node.StatusCount,
		ActiveUsers:       node.ActiveUsers,
		FirstSeen:         node.FirstSeen,
		LastSeen:          node.LastSeen,
		Health:            node.Health,
		ErrorRate:         node.ErrorRate,
		ResponseTime:      node.ResponseTime,
		ConnectionType:    node.ConnectionType,
		TotalConnections:  node.TotalConnections,
		ActiveConnections: node.ActiveConnections,
		ActivityVolume:    node.ActivityVolume,
		Metadata:          node.Metadata,
	}
	
	// Update keys
	modelNode.UpdateKeys()
	
	// Save the node
	err := r.db.WithContext(ctx).Model(modelNode).Create()
	if err != nil {
		r.logger.Error("Failed to update federation node",
			zap.String("domain", node.Domain),
			zap.Error(err))
		return fmt.Errorf("failed to update federation node: %w", err)
	}
	
	// Also create a health index item for efficient health-based queries
	// Note: This would need a generic create method or separate model
	// For now, we'll skip the health index item
	
	return nil
}

// UpdateFederationEdge updates or creates a federation edge
func (r *FederationRepository) UpdateFederationEdge(ctx context.Context, edge *storage.FederationEdge) error {
	if edge.SourceDomain == "" || edge.TargetDomain == "" {
		return fmt.Errorf("source and target domains are required")
	}
	
	// Convert to model
	modelEdge := &models.FederationEdge{
		SourceDomain:   edge.SourceDomain,
		TargetDomain:   edge.TargetDomain,
		ConnectionType: edge.ConnectionType,
		VolumeIn:       edge.VolumeIn,
		VolumeOut:      edge.VolumeOut,
		Strength:       edge.Strength,
		LastActivity:   edge.LastActivity,
		SharedUsers:    edge.SharedUsers,
		ErrorCount:     edge.ErrorCount,
		SuccessRate:    edge.SuccessRate,
	}
	
	// Update keys
	modelEdge.UpdateKeys()
	
	// Save the edge
	err := r.db.WithContext(ctx).Model(modelEdge).Create()
	if err != nil {
		r.logger.Error("Failed to update federation edge",
			zap.String("source", edge.SourceDomain),
			zap.String("target", edge.TargetDomain),
			zap.Error(err))
		return fmt.Errorf("failed to update federation edge: %w", err)
	}
	
	// Also create a volume index item for efficient volume-based queries
	// For now, we'll skip this additional index
	
	return nil
}

// UpdateInstanceMetadata updates instance metadata
func (r *FederationRepository) UpdateInstanceMetadata(ctx context.Context, metadata *storage.InstanceMetadata) error {
	if metadata.Domain == "" {
		return fmt.Errorf("domain is required")
	}
	
	// Set last updated time
	metadata.LastUpdated = time.Now()
	
	// Convert to model
	modelMetadata := &models.InstanceMetadata{
		Domain:          metadata.Domain,
		DisplayName:     metadata.DisplayName,
		Description:     metadata.Description,
		Software:        metadata.Software,
		Version:         metadata.Version,
		UserCount:       metadata.UserCount,
		StatusCount:     metadata.StatusCount,
		NodeInfo:        metadata.NodeInfo,
		InstanceInfo:    metadata.InstanceInfo,
		AdminContact:    metadata.AdminContact,
		Rules:           metadata.Rules,
		Languages:       metadata.Languages,
		Categories:      metadata.Categories,
		FederationNotes: metadata.FederationNotes,
		LastUpdated:     metadata.LastUpdated,
	}
	
	// Update keys
	modelMetadata.UpdateKeys()
	
	// Save the metadata
	err := r.db.WithContext(ctx).Model(modelMetadata).Create()
	if err != nil {
		r.logger.Error("Failed to update instance metadata",
			zap.String("domain", metadata.Domain),
			zap.Error(err))
		return fmt.Errorf("failed to update instance metadata: %w", err)
	}
	
	return nil
}

// StoreFederationTimeSeries stores time-series federation metrics
func (r *FederationRepository) StoreFederationTimeSeries(ctx context.Context, data *storage.FederationTimeSeries) error {
	if data.Domain == "" || data.Period == "" {
		return fmt.Errorf("domain and period are required")
	}
	
	// Convert to model
	modelData := &models.FederationTimeSeries{
		Domain:    data.Domain,
		Period:    data.Period,
		Timestamp: data.Timestamp,
		// Map storage fields to model fields
		Metrics: map[string]interface{}{
			"inbound_volume":  data.InboundVolume,
			"outbound_volume": data.OutboundVolume,
			"error_rate":      data.ErrorRate,
			"response_time":   data.ResponseTime,
			"active_peers":    data.ActivePeers,
		},
		ActivityVolume: data.InboundVolume + data.OutboundVolume,
		ErrorCount:     int64(data.ErrorRate * 1000), // Convert rate to count (approximation)
		SuccessCount:   int64((1.0 - data.ErrorRate) * 1000), // Convert rate to count (approximation)
	}
	
	// Update keys (sets TTL based on period)
	modelData.UpdateKeys()
	
	// Save the time series data
	err := r.db.WithContext(ctx).Model(modelData).Create()
	if err != nil {
		r.logger.Error("Failed to store time series data",
			zap.String("domain", data.Domain),
			zap.String("period", data.Period),
			zap.Error(err))
		return fmt.Errorf("failed to store time series data: %w", err)
	}
	
	return nil
}

// StoreInstanceCluster stores a calculated federation cluster
func (r *FederationRepository) StoreInstanceCluster(ctx context.Context, cluster *storage.InstanceCluster) error {
	if cluster.ClusterID == "" {
		return fmt.Errorf("cluster ID is required")
	}
	
	cluster.Size = len(cluster.Instances)
	cluster.UpdatedAt = time.Now()
	
	// Convert to model
	modelCluster := &models.InstanceCluster{
		ClusterID:   cluster.ClusterID,
		Name:        cluster.Name,
		Instances:   cluster.Instances,
		CenterNode:  cluster.CenterNode,
		Cohesion:    cluster.Cohesion,
		Size:        cluster.Size,
		Description: cluster.Description,
		UpdatedAt:   cluster.UpdatedAt,
	}
	
	// Update keys
	modelCluster.UpdateKeys()
	
	// Save the cluster
	err := r.db.WithContext(ctx).Model(modelCluster).Create()
	if err != nil {
		r.logger.Error("Failed to store cluster",
			zap.String("cluster_id", cluster.ClusterID),
			zap.Error(err))
		return fmt.Errorf("failed to store cluster: %w", err)
	}
	
	return nil
}

// CreateSeveredRelationship records a new severed federation relationship
func (r *FederationRepository) CreateSeveredRelationship(ctx context.Context, rel *models.SeveredRelationship) error {
	// Generate ID if not provided
	if rel.ID == "" {
		rel.ID = fmt.Sprintf("%s-%s-%d", rel.LocalInstance, rel.RemoteInstance, time.Now().Unix())
	}

	// Set timestamp if not provided
	if rel.Timestamp.IsZero() {
		rel.Timestamp = time.Now()
	}

	// Update keys
	rel.UpdateKeys()

	// Store the relationship
	err := r.db.WithContext(ctx).Model(rel).Create()
	if err != nil {
		r.logger.Error("Failed to create severed relationship",
			zap.String("id", rel.ID),
			zap.String("local", rel.LocalInstance),
			zap.String("remote", rel.RemoteInstance),
			zap.String("reason", string(rel.Reason)),
			zap.Error(err))
		return fmt.Errorf("failed to create severed relationship: %w", err)
	}

	// Log the severance
	r.logger.Info("Severed relationship created",
		zap.String("id", rel.ID),
		zap.String("local", rel.LocalInstance),
		zap.String("remote", rel.RemoteInstance),
		zap.String("reason", string(rel.Reason)),
		zap.Int("impact", rel.EstimatedImpact))

	return nil
}

// GetSeveredRelationships retrieves severed relationships for a local instance
func (r *FederationRepository) GetSeveredRelationships(ctx context.Context, localInstance string, limit int, cursor string) ([]*models.SeveredRelationship, string, error) {
	query := r.db.WithContext(ctx).Model(&models.SeveredRelationship{}).
		Filter("PK", "BEGINS_WITH", fmt.Sprintf("SEVERED#%s#", localInstance)).
		Limit(limit)

	// If cursor provided, set the starting point
	if cursor != "" {
		query = query.Where("SK", ">", cursor)
	}

	var relationships []models.SeveredRelationship
	err := query.Scan(&relationships)
	if err != nil {
		r.logger.Error("Failed to query severed relationships",
			zap.String("local_instance", localInstance),
			zap.Int("limit", limit),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to query severed relationships: %w", err)
	}

	// Convert to pointer slice
	result := make([]*models.SeveredRelationship, len(relationships))
	for i := range relationships {
		result[i] = &relationships[i]
	}

	// Prepare next cursor
	nextCursor := ""
	if len(relationships) > 0 && len(relationships) == limit {
		nextCursor = relationships[len(relationships)-1].SK
	}

	return result, nextCursor, nil
}

// GetSeveredRelationship retrieves a specific severed relationship
func (r *FederationRepository) GetSeveredRelationship(ctx context.Context, localInstance, remoteInstance string) (*models.SeveredRelationship, error) {
	// Query for the most recent severance between these instances
	var relationships []models.SeveredRelationship
	
	err := r.db.WithContext(ctx).Model(&models.SeveredRelationship{}).
		Where("PK", "=", fmt.Sprintf("SEVERED#%s#%s", localInstance, remoteInstance)).
		Limit(1).
		All(&relationships) // Most recent first

	if err != nil {
		r.logger.Error("Failed to query severed relationship",
			zap.String("local", localInstance),
			zap.String("remote", remoteInstance),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query severed relationship: %w", err)
	}

	if len(relationships) == 0 {
		return nil, fmt.Errorf("no severed relationship found between %s and %s", localInstance, remoteInstance)
	}

	return &relationships[0], nil
}

// UpdateSeveredRelationship updates an existing severed relationship
func (r *FederationRepository) UpdateSeveredRelationship(ctx context.Context, rel *models.SeveredRelationship) error {
	// Ensure keys are set
	if rel.PK == "" || rel.SK == "" {
		rel.UpdateKeys()
	}

	// Fetch, update, save pattern for DynamORM
	var existing models.SeveredRelationship
	err := r.db.WithContext(ctx).Model(&models.SeveredRelationship{}).
		Where("PK", "=", rel.PK).
		Where("SK", "=", rel.SK).
		First(&existing)
	
	if err != nil {
		return fmt.Errorf("failed to find relationship to update: %w", err)
	}
	
	// Update fields
	existing.Reason = rel.Reason
	existing.Reversible = rel.Reversible
	existing.Details = rel.Details
	existing.EstimatedImpact = rel.EstimatedImpact
	
	// Save updated record
	err = r.db.WithContext(ctx).Model(&existing).Update()
	if err != nil {
		r.logger.Error("Failed to update severed relationship",
			zap.String("pk", rel.PK),
			zap.String("sk", rel.SK),
			zap.Error(err))
		return fmt.Errorf("failed to update severed relationship: %w", err)
	}

	return nil
}

// GetAffectedFollows retrieves follow relationships affected by a severance
func (r *FederationRepository) GetAffectedFollows(ctx context.Context, localInstance, remoteInstance string) ([]models.AffectedFollow, error) {
	// Get the severed relationship
	rel, err := r.GetSeveredRelationship(ctx, localInstance, remoteInstance)
	if err != nil {
		return nil, err
	}

	return rel.AffectedFollows, nil
}

// RecordAffectedFollow adds an affected follow to a severed relationship
func (r *FederationRepository) RecordAffectedFollow(ctx context.Context, localInstance, remoteInstance string, follow models.AffectedFollow) error {
	// Get the current relationship
	rel, err := r.GetSeveredRelationship(ctx, localInstance, remoteInstance)
	if err != nil {
		return fmt.Errorf("failed to get severed relationship: %w", err)
	}

	// Add the affected follow
	rel.AffectedFollows = append(rel.AffectedFollows, follow)
	rel.EstimatedImpact = len(rel.AffectedFollows)

	// Update the relationship by creating a new version (DynamoDB pattern)
	err = r.db.WithContext(ctx).Model(rel).Create()
	if err != nil {
		r.logger.Error("Failed to update severed relationship with affected follow",
			zap.String("local", localInstance),
			zap.String("remote", remoteInstance),
			zap.Error(err))
		return fmt.Errorf("failed to update severed relationship: %w", err)
	}

	return nil
}

// ReverseSeverance marks a severed relationship as restored
func (r *FederationRepository) ReverseSeverance(ctx context.Context, localInstance, remoteInstance string) error {
	// Get the current relationship
	rel, err := r.GetSeveredRelationship(ctx, localInstance, remoteInstance)
	if err != nil {
		return fmt.Errorf("failed to get severed relationship: %w", err)
	}

	if !rel.Reversible {
		return fmt.Errorf("severed relationship is not reversible")
	}

	// Create a new "restored" entry
	restored := &models.SeveredRelationship{
		ID:              fmt.Sprintf("%s-restored-%d", rel.ID, time.Now().Unix()),
		LocalInstance:   localInstance,
		RemoteInstance:  remoteInstance,
		Reason:          models.SeveranceReasonRestored,
		Timestamp:       time.Now(),
		Reversible:      false,
		Details:         fmt.Sprintf("Relationship restored after previous severance: %s", rel.Reason),
		EstimatedImpact: 0,
		AffectedFollows: []models.AffectedFollow{}, // Empty slice
	}

	// Update keys
	restored.UpdateKeys()

	// Store the restored entry
	err = r.db.WithContext(ctx).Model(restored).Create()
	if err != nil {
		r.logger.Error("Failed to create restored relationship entry",
			zap.String("local", localInstance),
			zap.String("remote", remoteInstance),
			zap.Error(err))
		return fmt.Errorf("failed to create restored relationship entry: %w", err)
	}

	r.logger.Info("Severed relationship restored",
		zap.String("local", localInstance),
		zap.String("remote", remoteInstance))

	return nil
}

// GetSeveranceHistory retrieves the history of severances between two instances
func (r *FederationRepository) GetSeveranceHistory(ctx context.Context, localInstance, remoteInstance string, limit int) ([]*models.SeveredRelationship, error) {
	var history []models.SeveredRelationship
	
	err := r.db.WithContext(ctx).Model(&models.SeveredRelationship{}).
		Where("PK", "=", fmt.Sprintf("SEVERED#%s#%s", localInstance, remoteInstance)).
		Limit(limit).
		All(&history) // Most recent first

	if err != nil {
		r.logger.Error("Failed to query severance history",
			zap.String("local", localInstance),
			zap.String("remote", remoteInstance),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query severance history: %w", err)
	}

	// Convert to pointer slice
	result := make([]*models.SeveredRelationship, len(history))
	for i := range history {
		result[i] = &history[i]
	}

	return result, nil
}

// determineSeverity determines issue severity based on type
func (r *FederationRepository) determineSeverity(issueType string) string {
	switch issueType {
	case "blocked", "defederation":
		return "critical"
	case "unreachable", "timeout":
		return "high"
	case "error":
		return "medium"
	default:
		return "low"
	}
}

// generateFederationRandomString generates a random string of specified length
func generateFederationRandomString(length int) string {
	id := uuid.New().String()
	// Remove hyphens and return the requested length
	cleaned := strings.ReplaceAll(id, "-", "")
	if len(cleaned) > length {
		return cleaned[:length]
	}
	return cleaned
}

// RecordDeliveryAttempt records a delivery attempt for an activity
func (r *FederationRepository) RecordDeliveryAttempt(ctx context.Context, activityID, targetDomain string, success bool, errorMsg string) error {
	now := time.Now()
	
	// Get or create delivery status
	delivery := &models.DeliveryStatus{
		ActivityID:   activityID,
		TargetDomain: targetDomain,
		CreatedAt:    now,
		LastAttempt:  now,
	}
	
	// Try to get existing record
	var existing models.DeliveryStatus
	err := r.db.WithContext(ctx).Model(&models.DeliveryStatus{}).
		Where("PK", "=", fmt.Sprintf("DELIVERY#%s", activityID)).
		Where("SK", "=", fmt.Sprintf("TARGET#%s", targetDomain)).
		First(&existing)
	
	if err == nil {
		// Update existing record
		delivery = &existing
		delivery.Attempts++
		delivery.LastAttempt = now
	} else if !errors.IsNotFound(err) {
		r.logger.Error("Failed to check existing delivery status",
			zap.String("activity_id", activityID),
			zap.String("target_domain", targetDomain),
			zap.Error(err))
		return fmt.Errorf("failed to check existing delivery status: %w", err)
	} else {
		// New delivery record
		delivery.Attempts = 1
		delivery.Status = "pending"
	}
	
	// Update status based on result
	if success {
		delivery.Status = "delivered"
		delivery.DeliveredAt = now
		delivery.Error = ""
	} else {
		delivery.Status = "failed"
		delivery.Error = errorMsg
		// Calculate exponential backoff for next retry
		// 1st retry: 1 min, 2nd: 5 min, 3rd: 15 min, 4th: 1 hour, then give up
		retryDelays := []time.Duration{
			1 * time.Minute,
			5 * time.Minute,
			15 * time.Minute,
			1 * time.Hour,
		}
		if delivery.Attempts < len(retryDelays) {
			delivery.NextRetry = now.Add(retryDelays[delivery.Attempts-1])
		}
	}
	
	// Update keys
	delivery.UpdateKeys()
	
	// Save the delivery status
	err = r.db.WithContext(ctx).Model(delivery).Create()
	if err != nil {
		r.logger.Error("Failed to record delivery attempt",
			zap.String("activity_id", activityID),
			zap.String("target_domain", targetDomain),
			zap.Bool("success", success),
			zap.Error(err))
		return fmt.Errorf("failed to record delivery attempt: %w", err)
	}
	
	r.logger.Info("Recorded delivery attempt",
		zap.String("activity_id", activityID),
		zap.String("target_domain", targetDomain),
		zap.Bool("success", success),
		zap.Int("attempts", delivery.Attempts))
	
	return nil
}

// GetDeliveryStatus retrieves the delivery status for an activity to a domain
func (r *FederationRepository) GetDeliveryStatus(ctx context.Context, activityID, targetDomain string) (*storage.DeliveryStatus, error) {
	var delivery models.DeliveryStatus
	
	err := r.db.WithContext(ctx).Model(&models.DeliveryStatus{}).
		Where("PK", "=", fmt.Sprintf("DELIVERY#%s", activityID)).
		Where("SK", "=", fmt.Sprintf("TARGET#%s", targetDomain)).
		First(&delivery)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		r.logger.Error("Failed to get delivery status",
			zap.String("activity_id", activityID),
			zap.String("target_domain", targetDomain),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get delivery status: %w", err)
	}
	
	// Convert to storage type
	return &storage.DeliveryStatus{
		ActivityID:   delivery.ActivityID,
		TargetDomain: delivery.TargetDomain,
		Status:       delivery.Status,
		Attempts:     delivery.Attempts,
		LastAttempt:  delivery.LastAttempt,
		Error:        delivery.Error,
		CreatedAt:    delivery.CreatedAt,
		DeliveredAt:  &delivery.DeliveredAt,
		NextRetry:    &delivery.NextRetry,
	}, nil
}

// ListFailedDeliveries retrieves deliveries that need retry
func (r *FederationRepository) ListFailedDeliveries(ctx context.Context, limit int) ([]*storage.DeliveryStatus, error) {
	now := time.Now()
	
	var deliveries []models.DeliveryStatus
	err := r.db.WithContext(ctx).Model(&models.DeliveryStatus{}).
		Index("gsi1").
		Where("GSI1PK", "=", "FAILED_DELIVERIES").
		Where("GSI1SK", "<=", fmt.Sprintf("%d", now.Unix())).
		Limit(limit).
		Scan(&deliveries)
	
	if err != nil {
		r.logger.Error("Failed to list failed deliveries",
			zap.Int("limit", limit),
			zap.Error(err))
		return nil, fmt.Errorf("failed to list failed deliveries: %w", err)
	}
	
	// Convert to storage types
	result := make([]*storage.DeliveryStatus, len(deliveries))
	for i, delivery := range deliveries {
		result[i] = &storage.DeliveryStatus{
			ActivityID:   delivery.ActivityID,
			TargetDomain: delivery.TargetDomain,
			Status:       delivery.Status,
			Attempts:     delivery.Attempts,
			LastAttempt:  delivery.LastAttempt,
			Error:        delivery.Error,
			CreatedAt:    delivery.CreatedAt,
			DeliveredAt:  &delivery.DeliveredAt,
			NextRetry:    &delivery.NextRetry,
		}
	}
	
	return result, nil
}

// RetryDelivery marks a delivery for immediate retry
func (r *FederationRepository) RetryDelivery(ctx context.Context, activityID, targetDomain string) error {
	// Get existing delivery status
	var delivery models.DeliveryStatus
	err := r.db.WithContext(ctx).Model(&models.DeliveryStatus{}).
		Where("PK", "=", fmt.Sprintf("DELIVERY#%s", activityID)).
		Where("SK", "=", fmt.Sprintf("TARGET#%s", targetDomain)).
		First(&delivery)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("delivery not found for activity %s to %s", activityID, targetDomain)
		}
		r.logger.Error("Failed to get delivery for retry",
			zap.String("activity_id", activityID),
			zap.String("target_domain", targetDomain),
			zap.Error(err))
		return fmt.Errorf("failed to get delivery for retry: %w", err)
	}
	
	// Update for immediate retry
	delivery.NextRetry = time.Now()
	delivery.Status = "pending"
	
	// Update keys
	delivery.UpdateKeys()
	
	// Save the updated status
	err = r.db.WithContext(ctx).Model(&delivery).Create()
	if err != nil {
		r.logger.Error("Failed to update delivery for retry",
			zap.String("activity_id", activityID),
			zap.String("target_domain", targetDomain),
			zap.Error(err))
		return fmt.Errorf("failed to update delivery for retry: %w", err)
	}
	
	r.logger.Info("Marked delivery for retry",
		zap.String("activity_id", activityID),
		zap.String("target_domain", targetDomain))
	
	return nil
}

// CleanupOldDeliveries removes old delivery records (called by scheduled job)
func (r *FederationRepository) CleanupOldDeliveries(ctx context.Context, olderThan time.Duration) (int, error) {
	// TTL should handle this automatically, but this provides manual cleanup if needed
	cutoff := time.Now().Add(-olderThan)
	
	// Since DynamORM doesn't support batch deletes with conditions easily,
	// we'll rely on TTL for automatic cleanup
	// This method is here for API completeness
	
	r.logger.Info("Cleanup old deliveries called - relying on TTL",
		zap.Time("cutoff", cutoff))
	
	return 0, nil
}

// AddToInbox adds an activity to an actor's inbox
func (r *FederationRepository) AddToInbox(ctx context.Context, actorID string, activity *activitypub.Activity) error {
	now := time.Now()
	
	inbox := &models.InboxItem{
		ActorID:    actorID,
		ActivityID: activity.ID,
		Activity:   activity,
		Timestamp:  now,
		CreatedAt:  now,
	}
	
	// Update keys
	inbox.UpdateKeys()
	
	// Save to inbox
	err := r.db.WithContext(ctx).Model(inbox).Create()
	if err != nil {
		r.logger.Error("Failed to add activity to inbox",
			zap.String("actor_id", actorID),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return fmt.Errorf("failed to add activity to inbox: %w", err)
	}
	
	r.logger.Info("Added activity to inbox",
		zap.String("actor_id", actorID),
		zap.String("activity_id", activity.ID),
		zap.String("activity_type", activity.Type))
	
	return nil
}

// GetInboxItems retrieves activities from an actor's inbox
func (r *FederationRepository) GetInboxItems(ctx context.Context, actorID string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	query := r.db.WithContext(ctx).Model(&models.InboxItem{}).
		Index("gsi1").
		Where("GSI1PK", "=", fmt.Sprintf("INBOX#%s", actorID)).
		Limit(limit)
	
	// Add cursor if provided
	if cursor != "" {
		query = query.Where("GSI1SK", "<", cursor)
	}
	
	var items []models.InboxItem
	err := query.Scan(&items)
	if err != nil {
		r.logger.Error("Failed to get inbox items",
			zap.String("actor_id", actorID),
			zap.Int("limit", limit),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to get inbox items: %w", err)
	}
	
	// Extract activities
	activities := make([]*activitypub.Activity, len(items))
	for i, item := range items {
		activities[i] = item.Activity
	}
	
	// Prepare next cursor
	nextCursor := ""
	if len(items) == limit && len(items) > 0 {
		nextCursor = items[len(items)-1].GSI1SK
	}
	
	return activities, nextCursor, nil
}

// AddToOutbox adds an activity to an actor's outbox
func (r *FederationRepository) AddToOutbox(ctx context.Context, actorID string, activity *activitypub.Activity, public bool) error {
	now := time.Now()
	
	outbox := &models.OutboxItem{
		ActorID:    actorID,
		ActivityID: activity.ID,
		Activity:   activity,
		Timestamp:  now,
		CreatedAt:  now,
		Public:     public,
	}
	
	// Update keys
	outbox.UpdateKeys()
	
	// Save to outbox
	err := r.db.WithContext(ctx).Model(outbox).Create()
	if err != nil {
		r.logger.Error("Failed to add activity to outbox",
			zap.String("actor_id", actorID),
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return fmt.Errorf("failed to add activity to outbox: %w", err)
	}
	
	r.logger.Info("Added activity to outbox",
		zap.String("actor_id", actorID),
		zap.String("activity_id", activity.ID),
		zap.String("activity_type", activity.Type),
		zap.Bool("public", public))
	
	return nil
}

// GetOutboxItems retrieves activities from an actor's outbox
func (r *FederationRepository) GetOutboxItems(ctx context.Context, actorID string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	// Query by primary key pattern since outbox uses same key structure as inbox
	query := r.db.WithContext(ctx).Model(&models.OutboxItem{}).
		Where("PK", "=", fmt.Sprintf("ACTOR#%s", actorID)).
		Filter("SK", "BEGINS_WITH", "ACTIVITY#").
		Limit(limit)
	
	var items []models.OutboxItem
	err := query.Scan(&items)
	if err != nil {
		r.logger.Error("Failed to get outbox items",
			zap.String("actor_id", actorID),
			zap.Int("limit", limit),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to get outbox items: %w", err)
	}
	
	// Extract activities
	activities := make([]*activitypub.Activity, len(items))
	for i, item := range items {
		activities[i] = item.Activity
	}
	
	// Prepare next cursor
	nextCursor := ""
	if len(items) == limit && len(items) > 0 {
		nextCursor = items[len(items)-1].SK
	}
	
	return activities, nextCursor, nil
}

// GetPublicOutbox retrieves only public activities from an actor's outbox
func (r *FederationRepository) GetPublicOutbox(ctx context.Context, actorID string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	query := r.db.WithContext(ctx).Model(&models.OutboxItem{}).
		Index("gsi1").
		Where("GSI1PK", "=", fmt.Sprintf("PUBLIC_OUTBOX#%s", actorID)).
		Limit(limit)
	
	// Add cursor if provided
	if cursor != "" {
		query = query.Where("GSI1SK", "<", cursor)
	}
	
	var items []models.OutboxItem
	err := query.Scan(&items)
	if err != nil {
		r.logger.Error("Failed to get public outbox items",
			zap.String("actor_id", actorID),
			zap.Int("limit", limit),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to get public outbox items: %w", err)
	}
	
	// Extract activities
	activities := make([]*activitypub.Activity, len(items))
	for i, item := range items {
		activities[i] = item.Activity
	}
	
	// Prepare next cursor
	nextCursor := ""
	if len(items) == limit && len(items) > 0 {
		nextCursor = items[len(items)-1].GSI1SK
	}
	
	return activities, nextCursor, nil
}

// GetStrongestConnectionsByType retrieves the strongest federation connections by type
func (r *FederationRepository) GetStrongestConnectionsByType(ctx context.Context, connectionType string, limit int) ([]*storage.FederationEdge, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	
	var edges []*models.FederationEdge
	err := r.db.Model(&models.FederationEdge{}).
		Where("ConnectionType", "=", connectionType).
		OrderBy("Weight", "DESC").
		Limit(limit).
		All(&edges)
		
	if err != nil {
		r.logger.Error("failed to get strongest connections by type",
			zap.String("connection_type", connectionType),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get strongest connections by type: %w", err)
	}
	
	// Convert to storage.FederationEdge
	result := make([]*storage.FederationEdge, len(edges))
	for i, edge := range edges {
		result[i] = &storage.FederationEdge{
			SourceDomain:   edge.SourceDomain,
			TargetDomain:   edge.TargetDomain,
			ConnectionType: edge.ConnectionType,
			VolumeIn:       edge.VolumeIn,
			VolumeOut:      edge.VolumeOut,
			Strength:       edge.Strength,
			LastActivity:   edge.LastActivity,
			SharedUsers:    edge.SharedUsers,
			ErrorCount:     edge.ErrorCount,
			SuccessRate:    edge.SuccessRate,
		}
	}
	
	return result, nil
}

