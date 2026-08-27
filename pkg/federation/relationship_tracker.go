package federation

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/theorydb/batch"
	"github.com/google/uuid"
	dynamormcore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

// RelationshipTracker tracks and analyzes federation relationships with persistence and lifecycle management
type RelationshipTracker struct {
	storage core.RepositoryStorage
	db      dynamormcore.DB
	logger  *zap.Logger

	// S3 client for archival operations
	s3Client      s3API
	archiveBucket string

	// In-memory cache for active relationships (performance optimization)
	relationshipCache map[string]*models.FederationRelationship
	cacheMutex        sync.RWMutex

	// Configuration
	warmupDuration  time.Duration
	archiveAfter    time.Duration
	cleanupInterval time.Duration
}

type s3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// NewRelationshipTracker creates a new relationship tracker with DynamORM persistence
func NewRelationshipTracker(store core.RepositoryStorage, db dynamormcore.DB, logger *zap.Logger) *RelationshipTracker {
	rt := &RelationshipTracker{
		storage:           store,
		db:                db,
		logger:            logger,
		relationshipCache: make(map[string]*models.FederationRelationship),
		warmupDuration:    1 * time.Hour,
		archiveAfter:      90 * 24 * time.Hour,
		cleanupInterval:   1 * time.Hour,
	}

	// Start background cleanup process
	go rt.backgroundCleanup()

	return rt
}

// UpdateInstanceMetadata updates stored metadata for a federated instance.
func (rt *RelationshipTracker) UpdateInstanceMetadata(ctx context.Context, metadata *storage.InstanceMetadata) error {
	return rt.storage.Federation().UpdateInstanceMetadata(ctx, metadata)
}

// NewRelationshipTrackerWithS3 creates a new relationship tracker with S3 archival support
func NewRelationshipTrackerWithS3(store core.RepositoryStorage, db dynamormcore.DB, logger *zap.Logger, s3Client *s3.Client, archiveBucket string) *RelationshipTracker {
	rt := NewRelationshipTracker(store, db, logger)
	if s3Client != nil {
		rt.s3Client = s3Client
	}
	rt.archiveBucket = archiveBucket
	return rt
}

// TrackDeliveryAttempt records a federation delivery attempt with comprehensive relationship tracking
func (rt *RelationshipTracker) TrackDeliveryAttempt(ctx context.Context, attempt *DeliveryAttempt) error {
	// Track both user-level and instance-level relationships
	if err := rt.trackUserRelationship(ctx, attempt); err != nil {
		rt.logger.Error("Failed to track user relationship",
			zap.String("source", attempt.SourceDomain),
			zap.String("target", attempt.TargetDomain),
			zap.Error(err))
	}

	if err := rt.trackInstanceRelationship(ctx, attempt); err != nil {
		rt.logger.Error("Failed to track instance relationship",
			zap.String("source", attempt.SourceDomain),
			zap.String("target", attempt.TargetDomain),
			zap.Error(err))
	}

	// Update legacy federation edge for backward compatibility
	edge := &storage.FederationEdge{
		SourceDomain:   attempt.SourceDomain,
		TargetDomain:   attempt.TargetDomain,
		ConnectionType: attempt.ActivityType,
		LastActivity:   time.Now(),
	}

	if attempt.Success {
		edge.VolumeOut++
	}

	if err := rt.storage.Federation().UpdateFederationEdge(ctx, edge); err != nil {
		rt.logger.Warn("Failed to update legacy federation edge", zap.Error(err))
	}

	return nil
}

// TrackInboundActivity records inbound federation activity with comprehensive relationship tracking
func (rt *RelationshipTracker) TrackInboundActivity(ctx context.Context, activity *InboundActivity) error {
	// Convert to delivery attempt format for consistent tracking
	attempt := &DeliveryAttempt{
		SourceDomain:   activity.SourceDomain,
		TargetDomain:   activity.TargetDomain,
		ActivityType:   activity.ActivityType,
		Success:        true, // Inbound activities are successful by definition
		ResponseTimeMs: 0,    // Not applicable for inbound
		Timestamp:      activity.Timestamp,
		UserID:         activity.UserID, // Add this field to InboundActivity if not present
	}

	// Track relationships
	if err := rt.trackUserRelationship(ctx, attempt); err != nil {
		rt.logger.Error("Failed to track inbound user relationship",
			zap.String("source", activity.SourceDomain),
			zap.String("target", activity.TargetDomain),
			zap.Error(err))
	}

	if err := rt.trackInstanceRelationship(ctx, attempt); err != nil {
		rt.logger.Error("Failed to track inbound instance relationship",
			zap.String("source", activity.SourceDomain),
			zap.String("target", activity.TargetDomain),
			zap.Error(err))
	}

	// Update legacy federation edge for backward compatibility
	edge := &storage.FederationEdge{
		SourceDomain:   activity.SourceDomain,
		TargetDomain:   activity.TargetDomain,
		ConnectionType: activity.ActivityType,
		VolumeIn:       1,
		LastActivity:   time.Now(),
	}

	if err := rt.storage.Federation().UpdateFederationEdge(ctx, edge); err != nil {
		rt.logger.Warn("Failed to update legacy federation edge", zap.Error(err))
	}

	return nil
}

// AnalyzeRelationshipStrength calculates the strength of relationships between instances
func (rt *RelationshipTracker) AnalyzeRelationshipStrength(ctx context.Context, sourceDomain, targetDomain string) (*RelationshipAnalysis, error) {
	// Get edge data
	edges, err := rt.storage.Federation().GetFederationEdges(ctx, []string{sourceDomain, targetDomain})
	if err != nil {
		rt.logger.Error("Failed to get federation edges",
			zap.String("source_domain", sourceDomain),
			zap.String("target_domain", targetDomain),
			zap.Error(err))
		return nil, errors.Join(ErrGetFederationEdgesFailed, err)
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
	connections, err := rt.storage.Federation().GetInstanceConnections(ctx, domain, "")
	if err != nil {
		rt.logger.Error("Failed to get connections",
			zap.String("domain", domain),
			zap.Error(err))
		return nil, errors.Join(ErrGetConnectionsFailed, err)
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
					"success_rate":     rt.calculateSuccessRateForConnection(conn),
				},
			}
			recommendations = append(recommendations, rec)
		}
	}

	// Find underutilized connections
	strongEdges, err := rt.storage.Federation().GetStrongestConnectionsByType(ctx, "all", 50)
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

// Helper methods for relationship tracking

// trackUserRelationship tracks a user-level federation relationship
func (rt *RelationshipTracker) trackUserRelationship(ctx context.Context, attempt *DeliveryAttempt) error {
	if err := common.ValidateRequiredParam("user_id", attempt.UserID); err != nil {
		// Can't track user-level without user ID
		return nil
	}

	// Generate relationship ID
	relID := rt.generateRelationshipID(attempt.UserID, attempt.TargetDomain, attempt.ActivityType)

	// Get or create relationship
	rel, err := rt.getOrCreateRelationship(ctx, attempt.UserID, attempt.TargetDomain, attempt.ActivityType, relID)
	if err != nil {
		rt.logger.Error("Failed to get/create relationship",
			zap.String("user_id", attempt.UserID),
			zap.String("target_domain", attempt.TargetDomain),
			zap.String("activity_type", attempt.ActivityType),
			zap.Error(err))
		return errors.Join(ErrGetCreateRelationshipFailed, err)
	}

	// Update success rate and metrics
	rel.UpdateSuccessRate(attempt.Success, attempt.ResponseTimeMs)

	// Check for state transitions
	if newState, shouldTransition := rel.ShouldTransitionState(); shouldTransition {
		rt.logger.Info("Relationship state transition",
			zap.String("user_id", attempt.UserID),
			zap.String("target", attempt.TargetDomain),
			zap.String("old_state", string(rel.State)),
			zap.String("new_state", string(newState)))

		rel.TransitionToState(newState)
	}

	// Save the relationship
	return rt.saveRelationship(ctx, rel)
}

// trackInstanceRelationship tracks an instance-level aggregate relationship
func (rt *RelationshipTracker) trackInstanceRelationship(ctx context.Context, attempt *DeliveryAttempt) error {
	// Update 15-minute aggregate
	agg, err := rt.getOrCreateAggregate(ctx, attempt.TargetDomain, "15min")
	if err != nil {
		rt.logger.Error("Failed to get/create aggregate",
			zap.String("target_domain", attempt.TargetDomain),
			zap.Error(err))
		return errors.Join(ErrGetCreateAggregateFailed, err)
	}

	// Update aggregate metrics
	if attempt.Success {
		agg.TotalSuccesses15m++
	} else {
		agg.TotalFailures15m++
	}

	// Recalculate success rate
	total := agg.TotalSuccesses15m + agg.TotalFailures15m
	if total > 0 {
		agg.OverallSuccessRate = float64(agg.TotalSuccesses15m) / float64(total)
	}

	// Update response time (weighted average)
	if attempt.ResponseTimeMs > 0 {
		if agg.AvgResponseTime == 0 {
			agg.AvgResponseTime = attempt.ResponseTimeMs
		} else {
			agg.AvgResponseTime = agg.AvgResponseTime*0.9 + attempt.ResponseTimeMs*0.1
		}
	}

	// Save the aggregate
	return rt.saveAggregate(ctx, agg)
}

// getOrCreateRelationship retrieves or creates a federation relationship
func (rt *RelationshipTracker) getOrCreateRelationship(ctx context.Context, userID, targetInstance, relType, relID string) (*models.FederationRelationship, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("%s:%s:%s", userID, targetInstance, relType)
	rt.cacheMutex.RLock()
	if cached, exists := rt.relationshipCache[cacheKey]; exists {
		rt.cacheMutex.RUnlock()
		return cached, nil
	}
	rt.cacheMutex.RUnlock()

	// Try to get from database
	var rel models.FederationRelationship
	pk := fmt.Sprintf("USER#%s#FEDERATION", userID)
	sk := fmt.Sprintf("REL#%s#%s#%s", targetInstance, relType, relID)

	err := rt.db.WithContext(ctx).Model(&models.FederationRelationship{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&rel)

	if err != nil && !dynamormerrors.IsNotFound(err) {
		rt.logger.Error("Failed to query relationship",
			zap.String("user_id", userID),
			zap.String("target_instance", targetInstance),
			zap.String("rel_type", relType),
			zap.Error(err))
		return nil, errors.Join(ErrQueryRelationshipFailed, err)
	}

	if dynamormerrors.IsNotFound(err) {
		// Create new relationship
		now := time.Now()
		rel = models.FederationRelationship{
			ID:               relID,
			UserID:           userID,
			TargetInstance:   targetInstance,
			RelationshipType: relType,
			State:            models.StateActive,
			FirstSeen:        now,
			LastActivity:     now,
			StateChangedAt:   now,
			WindowStart15m:   now.Truncate(15 * time.Minute),
			SuccessRate:      1.0, // Optimistic start
			CurrentRate:      1.0,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		rel.UpdateKeys()
	}

	// Cache the relationship
	rt.cacheMutex.Lock()
	rt.relationshipCache[cacheKey] = &rel
	rt.cacheMutex.Unlock()

	return &rel, nil
}

// getOrCreateAggregate retrieves or creates a relationship aggregate
func (rt *RelationshipTracker) getOrCreateAggregate(ctx context.Context, instanceDomain, period string) (*models.FederationRelationshipAggregate, error) {
	now := time.Now()
	timestamp := now.Truncate(15 * time.Minute) // 15-minute buckets

	var agg models.FederationRelationshipAggregate
	pk := fmt.Sprintf("INSTANCE#%s#FEDERATION_AGG", instanceDomain)
	sk := fmt.Sprintf("PERIOD#%s#%s", period, timestamp.Format("20060102150405"))

	err := rt.db.WithContext(ctx).Model(&models.FederationRelationshipAggregate{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&agg)

	if err != nil && !dynamormerrors.IsNotFound(err) {
		rt.logger.Error("Failed to query aggregate",
			zap.String("instance_domain", instanceDomain),
			zap.String("period", period),
			zap.Error(err))
		return nil, errors.Join(ErrQueryAggregateFailed, err)
	}

	if dynamormerrors.IsNotFound(err) {
		// Create new aggregate
		agg = models.FederationRelationshipAggregate{
			InstanceDomain:   instanceDomain,
			Period:           period,
			Timestamp:        timestamp,
			StateTransitions: make(map[string]int64),
			CreatedAt:        now,
		}
		agg.UpdateKeys()
	}

	return &agg, nil
}

// saveRelationship persists a federation relationship
func (rt *RelationshipTracker) saveRelationship(ctx context.Context, rel *models.FederationRelationship) error {
	rel.UpdateKeys()

	err := rt.db.WithContext(ctx).Model(rel).CreateOrUpdate()
	if err != nil {
		rt.logger.Error("Failed to save relationship",
			zap.String("rel_id", rel.ID),
			zap.String("user_id", rel.UserID),
			zap.String("target_instance", rel.TargetInstance),
			zap.Error(err))
		return errors.Join(ErrSaveRelationshipFailed, err)
	}

	// Update cache
	cacheKey := fmt.Sprintf("%s:%s:%s", rel.UserID, rel.TargetInstance, rel.RelationshipType)
	rt.cacheMutex.Lock()
	rt.relationshipCache[cacheKey] = rel
	rt.cacheMutex.Unlock()

	return nil
}

// saveAggregate persists a federation relationship aggregate
func (rt *RelationshipTracker) saveAggregate(ctx context.Context, agg *models.FederationRelationshipAggregate) error {
	agg.UpdateKeys()

	err := rt.db.WithContext(ctx).Model(agg).CreateOrUpdate()
	if err != nil {
		rt.logger.Error("Failed to save aggregate",
			zap.String("instance_domain", agg.InstanceDomain),
			zap.String("period", agg.Period),
			zap.Error(err))
		return errors.Join(ErrSaveAggregateFailed, err)
	}

	return nil
}

// generateRelationshipID generates a unique relationship ID
func (rt *RelationshipTracker) generateRelationshipID(userID, targetInstance, relType string) string {
	// Use deterministic ID based on relationship components
	base := fmt.Sprintf("%s-%s-%s", userID, targetInstance, relType)
	id := uuid.NewSHA1(uuid.NameSpaceURL, []byte(base))
	return id.String()[:8] // Short ID for efficiency
}

// backgroundCleanup runs background tasks for relationship lifecycle management
func (rt *RelationshipTracker) backgroundCleanup() {
	ticker := time.NewTicker(rt.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

		// Process state transitions
		if err := rt.processStateTransitions(ctx); err != nil {
			rt.logger.Error("Failed to process state transitions", zap.Error(err))
		}

		// Archive dormant relationships
		if err := rt.archiveDormantRelationships(ctx); err != nil {
			rt.logger.Error("Failed to archive dormant relationships", zap.Error(err))
		}

		// Clean cache of old entries
		rt.cleanCache()

		cancel()
	}
}

// processStateTransitions finds and processes relationships that need state transitions
func (rt *RelationshipTracker) processStateTransitions(ctx context.Context) error {
	// Query active and idle relationships that might need transitions
	states := []models.RelationshipState{models.StateActive, models.StateIdle, models.StateDormant}

	for _, state := range states {
		var relationships []models.FederationRelationship

		err := rt.db.WithContext(ctx).Model(&models.FederationRelationship{}).
			Index(models.IndexGSI1).
			Where("gsi1PK", "=", fmt.Sprintf("FEDERATION_STATE#%s", state)).
			Limit(100). // Process in batches
			All(&relationships)

		if err != nil {
			rt.logger.Error("Failed to query relationships for state transitions",
				zap.String("state", string(state)),
				zap.Error(err))
			continue
		}

		for _, rel := range relationships {
			if newState, shouldTransition := rel.ShouldTransitionState(); shouldTransition {
				rt.logger.Info("Processing state transition",
					zap.String("user_id", rel.UserID),
					zap.String("target", rel.TargetInstance),
					zap.String("old_state", string(rel.State)),
					zap.String("new_state", string(newState)))

				rel.TransitionToState(newState)

				if err := rt.saveRelationship(ctx, &rel); err != nil {
					rt.logger.Error("Failed to save state transition",
						zap.String("rel_id", rel.ID),
						zap.Error(err))
				}
			}
		}
	}

	return nil
}

// archiveDormantRelationships moves dormant relationships to S3 for cost optimization
func (rt *RelationshipTracker) archiveDormantRelationships(ctx context.Context) error {
	// Query relationships that should be archived
	var relationships []models.FederationRelationship

	err := rt.db.WithContext(ctx).Model(&models.FederationRelationship{}).
		Index(models.IndexGSI1).
		Where("gsi1PK", "=", fmt.Sprintf("FEDERATION_STATE#%s", models.StateDormant)).
		Where("gsi1SK", "<", fmt.Sprintf("%d", time.Now().Add(-rt.archiveAfter).Unix())).
		Limit(100).
		All(&relationships)

	if err != nil {
		rt.logger.Error("Failed to query dormant relationships",
			zap.Error(err))
		return errors.Join(ErrQueryDormantRelationshipsFailed, err)
	}

	for _, rel := range relationships {
		// Archive to S3 if client is configured
		if err := rt.archiveToS3(ctx, &rel); err != nil {
			rt.logger.Error("Failed to archive relationship to S3",
				zap.String("rel_id", rel.ID),
				zap.String("user_id", rel.UserID),
				zap.String("target", rel.TargetInstance),
				zap.Error(err))
			continue
		}

		// Transition to archived state after successful archival
		rel.TransitionToState(models.StateArchived)

		// Create index entry and remove full record
		index := &models.FederationRelationshipIndex{
			RelationshipID:  rel.ID,
			UserID:          rel.UserID,
			TargetInstance:  rel.TargetInstance,
			State:           rel.State,
			LastActivity:    rel.LastActivity,
			ArchiveLocation: fmt.Sprintf("s3://federation-archive/%s/%s.gz", time.Now().Format("2006/01/02"), rel.ID),
			CreatedAt:       time.Now(),
		}
		index.UpdateKeys()

		// Save index and delete full record
		if err := rt.db.WithContext(ctx).Model(index).CreateOrUpdate(); err != nil {
			rt.logger.Error("Failed to create relationship index",
				zap.String("rel_id", rel.ID),
				zap.Error(err))
			continue
		}

		// Delete full relationship record
		if err := rt.db.WithContext(ctx).Model(&rel).Delete(); err != nil {
			rt.logger.Error("Failed to delete archived relationship",
				zap.String("rel_id", rel.ID),
				zap.Error(err))
		}

		rt.logger.Info("Archived relationship",
			zap.String("rel_id", rel.ID),
			zap.String("user_id", rel.UserID),
			zap.String("target", rel.TargetInstance))
	}

	return nil
}

// cleanCache removes old entries from the relationship cache
func (rt *RelationshipTracker) cleanCache() {
	rt.cacheMutex.Lock()
	defer rt.cacheMutex.Unlock()

	// Remove entries older than 1 hour
	cutoff := time.Now().Add(-1 * time.Hour)
	for key, rel := range rt.relationshipCache {
		if rel.UpdatedAt.Before(cutoff) {
			delete(rt.relationshipCache, key)
		}
	}
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
	smaller := float64(mathMin(inbound, outbound))
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

// calculateSuccessRate calculates the 15-minute rolling window success rate for an instance
func (rt *RelationshipTracker) calculateSuccessRate(ctx context.Context, targetDomain string) float64 {
	// Get the current 15-minute aggregate
	agg, err := rt.getOrCreateAggregate(ctx, targetDomain, "15min")
	if err != nil {
		rt.logger.Error("Failed to get aggregate for success rate calculation",
			zap.String("domain", targetDomain),
			zap.Error(err))
		return 0.5 // Default neutral rate
	}

	return agg.OverallSuccessRate
}

// calculateSuccessRateForConnection calculates success rate for a specific connection (legacy compatibility)
func (rt *RelationshipTracker) calculateSuccessRateForConnection(conn *storage.InstanceConnection) float64 {
	// For legacy compatibility, use a simple success/failure approach
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

func mathMin(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// Public API methods for relationship tracking

// GetSuccessRate returns the current 15-minute success rate for a target domain
func (rt *RelationshipTracker) GetSuccessRate(ctx context.Context, targetDomain string) (float64, error) {
	return rt.calculateSuccessRate(ctx, targetDomain), nil
}

// GetRelationshipByID retrieves a specific federation relationship
func (rt *RelationshipTracker) GetRelationshipByID(ctx context.Context, userID, targetInstance, relType string) (*models.FederationRelationship, error) {
	relID := rt.generateRelationshipID(userID, targetInstance, relType)
	return rt.getOrCreateRelationship(ctx, userID, targetInstance, relType, relID)
}

// GetUserRelationships retrieves all federation relationships for a user
func (rt *RelationshipTracker) GetUserRelationships(ctx context.Context, userID string, limit int) ([]*models.FederationRelationship, error) {
	var relationships []models.FederationRelationship

	query := rt.db.WithContext(ctx).Model(&models.FederationRelationship{}).
		Where("PK", "=", fmt.Sprintf("USER#%s#FEDERATION", userID)).
		Limit(limit)

	err := query.All(&relationships)
	if err != nil {
		rt.logger.Error("Failed to query user relationships",
			zap.String("user_id", userID),
			zap.Error(err))
		return nil, errors.Join(ErrQueryUserRelationshipsFailed, err)
	}

	// Convert to pointer slice
	result := make([]*models.FederationRelationship, len(relationships))
	for i := range relationships {
		result[i] = &relationships[i]
	}

	return result, nil
}

// GetInstanceAggregate retrieves the current aggregate metrics for an instance
func (rt *RelationshipTracker) GetInstanceAggregate(ctx context.Context, instanceDomain string, period string) (*models.FederationRelationshipAggregate, error) {
	if err := common.ValidateRequiredParam("period", period); err != nil {
		period = "15min"
	}

	return rt.getOrCreateAggregate(ctx, instanceDomain, period)
}

// GetRelationshipsByState retrieves relationships in a specific state
func (rt *RelationshipTracker) GetRelationshipsByState(ctx context.Context, state models.RelationshipState, limit int) ([]*models.FederationRelationship, error) {
	var relationships []models.FederationRelationship

	query := rt.db.WithContext(ctx).Model(&models.FederationRelationship{}).
		Index(models.IndexGSI1).
		Where("gsi1PK", "=", fmt.Sprintf("FEDERATION_STATE#%s", state)).
		Limit(limit)

	err := query.All(&relationships)
	if err != nil {
		rt.logger.Error("Failed to query relationships by state",
			zap.String("state", string(state)),
			zap.Error(err))
		return nil, errors.Join(ErrQueryRelationshipsByStateFailed, err)
	}

	// Convert to pointer slice
	result := make([]*models.FederationRelationship, len(relationships))
	for i := range relationships {
		result[i] = &relationships[i]
	}

	return result, nil
}

// ForceStateTransition manually transitions a relationship to a new state
func (rt *RelationshipTracker) ForceStateTransition(ctx context.Context, userID, targetInstance, relType string, newState models.RelationshipState) error {
	relID := rt.generateRelationshipID(userID, targetInstance, relType)
	rel, err := rt.getOrCreateRelationship(ctx, userID, targetInstance, relType, relID)
	if err != nil {
		rt.logger.Error("Failed to get relationship",
			zap.String("user_id", userID),
			zap.String("target_instance", targetInstance),
			zap.String("rel_type", relType),
			zap.Error(err))
		return errors.Join(ErrGetRelationshipFailed, err)
	}

	oldState := rel.State
	rel.TransitionToState(newState)

	if err := rt.saveRelationship(ctx, rel); err != nil {
		rt.logger.Error("Failed to save state transition",
			zap.String("user_id", userID),
			zap.String("target_instance", targetInstance),
			zap.String("rel_type", relType),
			zap.Error(err))
		return errors.Join(ErrSaveStateTransitionFailed, err)
	}

	rt.logger.Info("Forced state transition",
		zap.String("user_id", userID),
		zap.String("target", targetInstance),
		zap.String("old_state", string(oldState)),
		zap.String("new_state", string(newState)))

	return nil
}

// GetHealthScore calculates a health score for relationships with a target instance
func (rt *RelationshipTracker) GetHealthScore(ctx context.Context, targetInstance string) (float64, error) {
	// Get recent 15-minute aggregate
	agg, err := rt.getOrCreateAggregate(ctx, targetInstance, "15min")
	if err != nil {
		rt.logger.Error("Failed to get aggregate",
			zap.String("target_instance", targetInstance),
			zap.Error(err))
		return 0.0, errors.Join(ErrGetAggregateFailed, err)
	}

	// Calculate health score based on success rate and response time
	successScore := agg.OverallSuccessRate * 100 // 0-100

	// Response time penalty: penalize high response times
	responseTimePenalty := 0.0
	if agg.AvgResponseTime > 1000 { // > 1 second
		responseTimePenalty = math.Min((agg.AvgResponseTime-1000)/10000*50, 50) // Up to 50 point penalty
	}

	healthScore := successScore - responseTimePenalty
	if healthScore < 0 {
		healthScore = 0
	}
	if healthScore > 100 {
		healthScore = 100
	}

	return healthScore, nil
}

// GetUnhealthyRelationships returns relationships that are performing poorly
func (rt *RelationshipTracker) GetUnhealthyRelationships(ctx context.Context, threshold float64, limit int) ([]*models.FederationRelationship, error) {
	// Get all active relationships
	activeRelationships, err := rt.GetRelationshipsByState(ctx, models.StateActive, limit*2) // Get more to filter
	if err != nil {
		rt.logger.Error("Failed to get active relationships",
			zap.Float64("threshold", threshold),
			zap.Int("limit", limit),
			zap.Error(err))
		return nil, errors.Join(ErrGetActiveRelationshipsFailed, err)
	}

	// Filter for unhealthy relationships
	var unhealthy []*models.FederationRelationship
	for _, rel := range activeRelationships {
		if rel.SuccessRate < threshold {
			unhealthy = append(unhealthy, rel)
			if len(unhealthy) >= limit {
				break
			}
		}
	}

	return unhealthy, nil
}

// ReactivateRelationship manually reactivates an archived relationship
func (rt *RelationshipTracker) ReactivateRelationship(ctx context.Context, userID, targetInstance, relType string) error {
	// Check if there's an archived index entry
	var index models.FederationRelationshipIndex
	indexPK := fmt.Sprintf("FEDERATION_REL_INDEX#%s", rt.generateRelationshipID(userID, targetInstance, relType))

	err := rt.db.WithContext(ctx).Model(&models.FederationRelationshipIndex{}).
		Where("PK", "=", indexPK).
		Where("SK", "=", "INDEX").
		First(&index)

	if dynamormerrors.IsNotFound(err) {
		// No archived relationship, create a new active one
		return rt.ForceStateTransition(ctx, userID, targetInstance, relType, models.StateActive)
	} else if err != nil {
		rt.logger.Error("Failed to check for archived relationship",
			zap.String("user_id", userID),
			zap.String("target_instance", targetInstance),
			zap.String("rel_type", relType),
			zap.Error(err))
		return errors.Join(ErrCheckArchivedRelationshipFailed, err)
	}

	// Restore from S3 archive if ArchiveLocation is set
	if index.ArchiveLocation != "" {
		if restoredRel, err := rt.restoreFromS3(ctx, index.ArchiveLocation); err != nil {
			rt.logger.Warn("Failed to restore from S3 archive, creating new relationship",
				zap.String("archive_location", index.ArchiveLocation),
				zap.Error(err))
		} else {
			// Use restored relationship data
			now := time.Now()
			restoredRel.State = models.StateActive
			restoredRel.StateChangedAt = now
			restoredRel.UpdatedAt = now

			// Set warmup period for reactivated relationship
			warmupEnd := now.Add(rt.warmupDuration)
			restoredRel.WarmupUntil = &warmupEnd
			restoredRel.CurrentRate = 0.1

			// Save the restored relationship
			if err := rt.saveRelationship(ctx, restoredRel); err != nil {
				rt.logger.Error("Failed to save restored relationship",
					zap.String("user_id", userID),
					zap.String("target_instance", targetInstance),
					zap.String("rel_type", relType),
					zap.Error(err))
				return errors.Join(ErrSaveRestoredRelationshipFailed, err)
			}

			// Delete the index entry (relationship is now active)
			if err := rt.db.WithContext(ctx).Model(&index).Delete(); err != nil {
				rt.logger.Warn("Failed to delete index after restoration",
					zap.String("rel_id", restoredRel.ID),
					zap.Error(err))
			}

			rt.logger.Info("Restored relationship from S3 archive",
				zap.String("user_id", userID),
				zap.String("target", targetInstance),
				zap.String("rel_type", relType),
				zap.String("archive_location", index.ArchiveLocation))

			return nil
		}
	}

	// For archived relationships without S3 location or failed restore, create a new active relationship with historical baseline

	now := time.Now()
	rel := &models.FederationRelationship{
		ID:               rt.generateRelationshipID(userID, targetInstance, relType),
		UserID:           userID,
		TargetInstance:   targetInstance,
		RelationshipType: relType,
		State:            models.StateActive,
		FirstSeen:        index.CreatedAt, // Keep original creation time
		LastActivity:     now,
		StateChangedAt:   now,
		WindowStart15m:   now.Truncate(15 * time.Minute),
		SuccessRate:      0.5, // Neutral starting point
		CurrentRate:      0.1, // Start with warmup
		CreatedAt:        index.CreatedAt,
		UpdatedAt:        now,
	}

	// Set warmup period
	warmupEnd := now.Add(rt.warmupDuration)
	rel.WarmupUntil = &warmupEnd

	// Save the reactivated relationship
	if err := rt.saveRelationship(ctx, rel); err != nil {
		rt.logger.Error("Failed to save reactivated relationship",
			zap.String("user_id", userID),
			zap.String("target_instance", targetInstance),
			zap.String("rel_type", relType),
			zap.Error(err))
		return errors.Join(ErrSaveReactivatedRelationshipFailed, err)
	}

	// Delete the index entry (relationship is now active)
	if err := rt.db.WithContext(ctx).Model(&index).Delete(); err != nil {
		rt.logger.Warn("Failed to delete index after reactivation",
			zap.String("rel_id", rel.ID),
			zap.Error(err))
	}

	rt.logger.Info("Reactivated relationship",
		zap.String("user_id", userID),
		zap.String("target", targetInstance),
		zap.String("rel_type", relType))

	return nil
}

// Types for relationship tracking

// DeliveryAttempt represents a federation delivery attempt
type DeliveryAttempt struct {
	SourceDomain   string
	TargetDomain   string
	ActivityType   string
	Success        bool
	ResponseTimeMs float64
	Timestamp      time.Time
	UserID         string // Local user ID for user-level tracking
}

// InboundActivity represents an inbound federation activity
type InboundActivity struct {
	SourceDomain string
	TargetDomain string
	ActivityType string
	Timestamp    time.Time
	UserID       string // Local user ID for user-level tracking
}

// RelationshipAnalysis represents analysis of federation relationship strength
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

// FederationRecommendation represents a recommendation for improving federation
//
//nolint:revive // Federation prefix clarifies this is federation-specific recommendation
type FederationRecommendation struct {
	Type         string         `json:"type"`     // performance/opportunity/cost/security
	Priority     string         `json:"priority"` // high/medium/low
	TargetDomain string         `json:"target_domain,omitempty"`
	Description  string         `json:"description"`
	Action       string         `json:"action"`
	Metrics      map[string]any `json:"metrics,omitempty"`
}

// S3 archival and restore operations

// ArchiveData represents the data structure for S3 archived relationships
type ArchiveData struct {
	Relationships []models.FederationRelationship `json:"relationships"`
	Metadata      ArchiveMetadata                 `json:"metadata"`
}

// ArchiveMetadata contains metadata about the archived data
type ArchiveMetadata struct {
	ArchivedAt      time.Time `json:"archived_at"`
	Reason          string    `json:"reason"`
	LastActivity    time.Time `json:"last_activity"`
	TotalItems      int       `json:"total_items"`
	CompressionType string    `json:"compression_type"`
	Version         string    `json:"version"`
}

// archiveToS3 archives a relationship to S3 in compressed JSON format
func (rt *RelationshipTracker) archiveToS3(ctx context.Context, rel *models.FederationRelationship) error {
	if rt.s3Client == nil {
		rt.logger.Debug("S3 archival not configured, skipping archival",
			zap.String("rel_id", rel.ID))
		return nil
	}
	if err := common.ValidateRequiredParam("archive_bucket", rt.archiveBucket); err != nil {
		rt.logger.Debug("S3 archival not configured, skipping archival",
			zap.String("rel_id", rel.ID))
		return nil
	}

	// Create archive data structure
	archiveData := ArchiveData{
		Relationships: []models.FederationRelationship{*rel},
		Metadata: ArchiveMetadata{
			ArchivedAt:      time.Now(),
			Reason:          "dormant_lifecycle",
			LastActivity:    rel.LastActivity,
			TotalItems:      1,
			CompressionType: "gzip",
			Version:         "1.0",
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(archiveData)
	if err != nil {
		rt.logger.Error("Failed to marshal archive data",
			zap.String("rel_id", rel.ID),
			zap.Error(err))
		return errors.Join(ErrMarshalArchiveDataFailed, err)
	}

	// Compress with gzip for cost optimization
	var compressedData bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressedData)
	if _, err := gzipWriter.Write(jsonData); err != nil {
		if closeErr := gzipWriter.Close(); closeErr != nil {
			rt.logger.Warn("Failed to close gzip writer after error", zap.Error(closeErr))
		}
		rt.logger.Error("Failed to compress data",
			zap.String("rel_id", rel.ID),
			zap.Error(err))
		return errors.Join(ErrCompressDataFailed, err)
	}
	if err := gzipWriter.Close(); err != nil {
		rt.logger.Error("Failed to close gzip writer",
			zap.String("rel_id", rel.ID),
			zap.Error(err))
		return errors.Join(ErrCloseGzipWriterFailed, err)
	}

	// Generate S3 key with organized structure: s3://bucket/federation-archives/YYYY/MM/DD/instanceID-timestamp.json.gz
	now := time.Now()
	s3Key := fmt.Sprintf("federation-archives/%04d/%02d/%02d/%s-%d.json.gz",
		now.Year(), now.Month(), now.Day(),
		rel.TargetInstance, now.Unix())

	// Upload to S3 with retry logic
	maxRetries := 3
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			backoffDuration := time.Duration(1<<(attempt-1)) * time.Second
			rt.logger.Info("Retrying S3 upload after backoff",
				zap.String("rel_id", rel.ID),
				zap.Int("attempt", attempt+1),
				zap.Duration("backoff", backoffDuration))

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoffDuration):
			}
		}

		putInput := &s3.PutObjectInput{
			Bucket:          aws.String(rt.archiveBucket),
			Key:             aws.String(s3Key),
			Body:            bytes.NewReader(compressedData.Bytes()),
			ContentType:     aws.String("application/json"),
			ContentEncoding: aws.String("gzip"),
			Metadata: map[string]string{
				"relationship-id": rel.ID,
				"user-id":         rel.UserID,
				"target-instance": rel.TargetInstance,
				"archived-at":     now.Format(time.RFC3339),
				"last-activity":   rel.LastActivity.Format(time.RFC3339),
				"original-state":  string(rel.State),
				"total-attempts":  fmt.Sprintf("%d", rel.TotalAttempts),
				"success-rate":    fmt.Sprintf("%.2f", rel.SuccessRate),
			},
		}

		_, err = rt.s3Client.PutObject(ctx, putInput)
		if err == nil {
			// Success - update relationship with archive location
			rel.ArchiveLocation = s3Key

			// Emit metrics for monitoring
			rt.logger.Info("Successfully archived relationship to S3",
				zap.String("rel_id", rel.ID),
				zap.String("user_id", rel.UserID),
				zap.String("target", rel.TargetInstance),
				zap.String("s3_key", s3Key),
				zap.Int("original_size", len(jsonData)),
				zap.Int("compressed_size", compressedData.Len()),
				zap.Float64("compression_ratio", float64(compressedData.Len())/float64(len(jsonData))))

			return nil
		}

		lastErr = err
		rt.logger.Warn("S3 upload attempt failed",
			zap.String("rel_id", rel.ID),
			zap.Int("attempt", attempt+1),
			zap.Error(err))
	}

	rt.logger.Error("Failed to archive to S3 after retries",
		zap.String("rel_id", rel.ID),
		zap.Int("max_retries", maxRetries),
		zap.Error(lastErr))
	return errors.Join(ErrArchiveToS3Failed, lastErr)
}

// BatchArchiveToS3 archives multiple relationships to S3 in a single compressed file for efficiency
func (rt *RelationshipTracker) BatchArchiveToS3(ctx context.Context, relationships []models.FederationRelationship) error {
	if rt.s3Client == nil || rt.archiveBucket == "" || len(relationships) == 0 {
		return nil
	}

	// Group relationships by target instance for better organization
	instanceGroups := make(map[string][]models.FederationRelationship)
	for _, rel := range relationships {
		instanceGroups[rel.TargetInstance] = append(instanceGroups[rel.TargetInstance], rel)
	}

	// Process each instance group
	for targetInstance, relGroup := range instanceGroups {
		if err := rt.archiveInstanceGroup(ctx, targetInstance, relGroup); err != nil {
			rt.logger.Error("Failed to archive instance group",
				zap.String("target_instance", targetInstance),
				zap.Int("count", len(relGroup)),
				zap.Error(err))
			// Continue with other groups even if one fails
		}
	}

	return nil
}

// archiveInstanceGroup archives a group of relationships for a single instance
func (rt *RelationshipTracker) archiveInstanceGroup(ctx context.Context, targetInstance string, relationships []models.FederationRelationship) error {
	// Find the latest activity time for metadata
	var latestActivity time.Time
	for _, rel := range relationships {
		if rel.LastActivity.After(latestActivity) {
			latestActivity = rel.LastActivity
		}
	}

	// Create archive data structure
	archiveData := ArchiveData{
		Relationships: relationships,
		Metadata: ArchiveMetadata{
			ArchivedAt:      time.Now(),
			Reason:          "batch_dormant_lifecycle",
			LastActivity:    latestActivity,
			TotalItems:      len(relationships),
			CompressionType: "gzip",
			Version:         "1.0",
		},
	}

	// Marshal and compress
	jsonData, err := json.Marshal(archiveData)
	if err != nil {
		rt.logger.Error("Failed to marshal batch archive data",
			zap.String("target_instance", targetInstance),
			zap.Int("count", len(relationships)),
			zap.Error(err))
		return errors.Join(ErrMarshalBatchArchiveDataFailed, err)
	}

	var compressedData bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressedData)
	if _, err := gzipWriter.Write(jsonData); err != nil {
		if closeErr := gzipWriter.Close(); closeErr != nil {
			rt.logger.Warn("Failed to close gzip writer after error", zap.Error(closeErr))
		}
		rt.logger.Error("Failed to compress batch data",
			zap.String("target_instance", targetInstance),
			zap.Int("count", len(relationships)),
			zap.Error(err))
		return errors.Join(ErrCompressBatchDataFailed, err)
	}
	if err := gzipWriter.Close(); err != nil {
		rt.logger.Error("Failed to close gzip writer",
			zap.String("target_instance", targetInstance),
			zap.Int("count", len(relationships)),
			zap.Error(err))
		return errors.Join(ErrCloseGzipWriterFailed, err)
	}

	// Generate batch S3 key
	now := time.Now()
	s3Key := fmt.Sprintf("federation-archives/%04d/%02d/%02d/batch-%s-%d.json.gz",
		now.Year(), now.Month(), now.Day(),
		targetInstance, now.Unix())

	// Upload to S3
	putInput := &s3.PutObjectInput{
		Bucket:          aws.String(rt.archiveBucket),
		Key:             aws.String(s3Key),
		Body:            bytes.NewReader(compressedData.Bytes()),
		ContentType:     aws.String("application/json"),
		ContentEncoding: aws.String("gzip"),
		Metadata: map[string]string{
			"batch-size":        fmt.Sprintf("%d", len(relationships)),
			"target-instance":   targetInstance,
			"archived-at":       now.Format(time.RFC3339),
			"last-activity":     latestActivity.Format(time.RFC3339),
			"compression-ratio": fmt.Sprintf("%.2f", float64(compressedData.Len())/float64(len(jsonData))),
		},
	}

	if _, err := rt.s3Client.PutObject(ctx, putInput); err != nil {
		rt.logger.Error("Failed to upload batch archive to S3",
			zap.String("target_instance", targetInstance),
			zap.Int("count", len(relationships)),
			zap.String("s3_key", s3Key),
			zap.Error(err))
		return errors.Join(ErrUploadBatchArchiveToS3Failed, err)
	}

	// Update all relationships with archive location
	for i := range relationships {
		relationships[i].ArchiveLocation = s3Key
	}

	rt.logger.Info("Successfully archived batch relationships to S3",
		zap.String("target_instance", targetInstance),
		zap.Int("count", len(relationships)),
		zap.String("s3_key", s3Key),
		zap.Int("original_size", len(jsonData)),
		zap.Int("compressed_size", compressedData.Len()),
		zap.Float64("compression_ratio", float64(compressedData.Len())/float64(len(jsonData))))

	return nil
}

// restoreFromS3 restores a relationship from S3 archive
func (rt *RelationshipTracker) restoreFromS3(ctx context.Context, archiveLocation string) (*models.FederationRelationship, error) {
	if rt.s3Client == nil {
		return nil, ErrS3ClientNotConfigured
	}
	if err := common.ValidateRequiredParam("archive_bucket", rt.archiveBucket); err != nil {
		return nil, ErrS3ClientNotConfigured
	}

	// Download from S3 with retry logic
	maxRetries := 3
	var archiveData ArchiveData
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoffDuration := time.Duration(1<<(attempt-1)) * time.Second
			rt.logger.Info("Retrying S3 download after backoff",
				zap.String("archive_location", archiveLocation),
				zap.Int("attempt", attempt+1),
				zap.Duration("backoff", backoffDuration))

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoffDuration):
			}
		}

		getInput := &s3.GetObjectInput{
			Bucket: aws.String(rt.archiveBucket),
			Key:    aws.String(archiveLocation),
		}

		result, err := rt.s3Client.GetObject(ctx, getInput)
		if err != nil {
			lastErr = err
			rt.logger.Warn("S3 download attempt failed",
				zap.String("archive_location", archiveLocation),
				zap.Int("attempt", attempt+1),
				zap.Error(err))
			continue
		}

		defer func() {
			if closeErr := result.Body.Close(); closeErr != nil {
				rt.logger.Warn("Failed to close S3 object body",
					zap.String("archive_location", archiveLocation),
					zap.Error(closeErr))
			}
		}()

		// Decompress the data
		gzipReader, err := gzip.NewReader(result.Body)
		if err != nil {
			lastErr = errors.Join(ErrCreateGzipReaderFailed, err)
			rt.logger.Warn("Gzip decompression failed",
				zap.String("archive_location", archiveLocation),
				zap.Error(lastErr))
			continue
		}

		compressedData, err := io.ReadAll(gzipReader)
		if err != nil {
			if closeErr := gzipReader.Close(); closeErr != nil {
				rt.logger.Warn("Failed to close gzip reader after error", zap.Error(closeErr))
			}
			lastErr = errors.Join(ErrReadCompressedDataFailed, err)
			rt.logger.Warn("Failed to read compressed data",
				zap.String("archive_location", archiveLocation),
				zap.Error(lastErr))
			continue
		}
		if closeErr := gzipReader.Close(); closeErr != nil {
			rt.logger.Warn("Failed to close gzip reader", zap.Error(closeErr))
		}

		// Parse JSON data
		if err := json.Unmarshal(compressedData, &archiveData); err != nil {
			lastErr = errors.Join(ErrUnmarshalArchiveDataFailed, err)
			rt.logger.Warn("Failed to unmarshal archive data",
				zap.String("archive_location", archiveLocation),
				zap.Error(lastErr))
			continue
		}

		// Success - validate data integrity
		if err := common.ValidateSliceNotEmpty("archive_relationships", archiveData.Relationships); err != nil {
			return nil, ErrArchiveContainsNoRelationships
		}

		// For single relationship archives, return the first relationship
		// For batch archives, this method should be extended to handle selection
		restoredRel := archiveData.Relationships[0]

		rt.logger.Info("Successfully restored relationship from S3",
			zap.String("archive_location", archiveLocation),
			zap.String("rel_id", restoredRel.ID),
			zap.String("user_id", restoredRel.UserID),
			zap.String("target", restoredRel.TargetInstance),
			zap.Time("archived_at", archiveData.Metadata.ArchivedAt),
			zap.Int("archive_size", len(compressedData)))

		return &restoredRel, nil
	}

	rt.logger.Error("Failed to restore from S3 after retries",
		zap.String("archive_location", archiveLocation),
		zap.Int("max_retries", maxRetries),
		zap.Error(lastErr))
	return nil, errors.Join(ErrRestoreFromS3Failed, lastErr)
}

// restoreMultipleFromS3 restores multiple relationships from a batch S3 archive
func (rt *RelationshipTracker) restoreMultipleFromS3(ctx context.Context, archiveLocation string) ([]models.FederationRelationship, error) {
	if rt.s3Client == nil {
		return nil, ErrS3ClientNotConfigured
	}
	if err := common.ValidateRequiredParam("archive_bucket", rt.archiveBucket); err != nil {
		return nil, ErrS3ClientNotConfigured
	}

	getInput := &s3.GetObjectInput{
		Bucket: aws.String(rt.archiveBucket),
		Key:    aws.String(archiveLocation),
	}

	result, err := rt.s3Client.GetObject(ctx, getInput)
	if err != nil {
		rt.logger.Error("Failed to download archive from S3",
			zap.String("archive_location", archiveLocation),
			zap.Error(err))
		return nil, errors.Join(ErrDownloadArchiveFromS3Failed, err)
	}
	defer func() {
		if closeErr := result.Body.Close(); closeErr != nil {
			rt.logger.Warn("Failed to close S3 object body", zap.Error(closeErr))
		}
	}()

	// Decompress the data
	gzipReader, err := gzip.NewReader(result.Body)
	if err != nil {
		rt.logger.Error("Failed to create gzip reader",
			zap.String("archive_location", archiveLocation),
			zap.Error(err))
		return nil, errors.Join(ErrCreateGzipReaderFailed, err)
	}
	defer func() {
		if closeErr := gzipReader.Close(); closeErr != nil {
			rt.logger.Warn("Failed to close gzip reader", zap.Error(closeErr))
		}
	}()

	compressedData, err := io.ReadAll(gzipReader)
	if err != nil {
		rt.logger.Error("Failed to read compressed data",
			zap.String("archive_location", archiveLocation),
			zap.Error(err))
		return nil, errors.Join(ErrReadCompressedDataFailed, err)
	}

	// Parse JSON data
	var archiveData ArchiveData
	if err := json.Unmarshal(compressedData, &archiveData); err != nil {
		rt.logger.Error("Failed to unmarshal archive data",
			zap.String("archive_location", archiveLocation),
			zap.Error(err))
		return nil, errors.Join(ErrUnmarshalArchiveDataFailed, err)
	}

	// Validate data integrity
	if err := common.ValidateSliceNotEmpty("archive_relationships", archiveData.Relationships); err != nil {
		return nil, ErrArchiveContainsNoRelationships
	}

	rt.logger.Info("Successfully restored multiple relationships from S3",
		zap.String("archive_location", archiveLocation),
		zap.Int("count", len(archiveData.Relationships)),
		zap.Time("archived_at", archiveData.Metadata.ArchivedAt))

	return archiveData.Relationships, nil
}

// cleanupS3Archive removes the S3 archive after successful restore
func (rt *RelationshipTracker) cleanupS3Archive(ctx context.Context, archiveLocation string) error {
	if rt.s3Client == nil {
		return nil
	}
	if err := common.ValidateRequiredParam("archive_bucket", rt.archiveBucket); err != nil {
		return nil
	}

	deleteInput := &s3.DeleteObjectInput{
		Bucket: aws.String(rt.archiveBucket),
		Key:    aws.String(archiveLocation),
	}

	if _, err := rt.s3Client.DeleteObject(ctx, deleteInput); err != nil {
		rt.logger.Error("Failed to delete S3 archive",
			zap.String("archive_location", archiveLocation),
			zap.Error(err))
		return errors.Join(ErrDeleteS3ArchiveFailed, err)
	}

	rt.logger.Info("Successfully deleted S3 archive after restore",
		zap.String("archive_location", archiveLocation))

	return nil
}

// BatchRestoreRelationships restores multiple relationships from archives using batch operations
func (rt *RelationshipTracker) BatchRestoreRelationships(ctx context.Context, archiveLocations []string) error {
	if err := common.ValidateSliceNotEmpty("archive_locations", archiveLocations); err != nil {
		return nil
	}

	// Create batch writer for efficient DynamoDB operations
	batchWriter := batch.NewBatchWriter(rt.db, batch.BatchWriterConfig{
		BatchSize: 25, // DynamoDB batch write limit
		Logger:    rt.logger,
	})

	// Track progress and collect restored relationships
	var allRelationships []any
	var archivesToCleanup []string

	// Process each archive location
	for _, archiveLocation := range archiveLocations {
		relationships, err := rt.restoreMultipleFromS3(ctx, archiveLocation)
		if err != nil {
			rt.logger.Error("Failed to restore from archive",
				zap.String("archive_location", archiveLocation),
				zap.Error(err))
			continue
		}

		// Update relationship status to active and prepare for batch insert
		now := time.Now()
		for i := range relationships {
			rel := &relationships[i]
			rel.State = models.StateActive
			rel.StateChangedAt = now
			rel.UpdatedAt = now
			rel.ArchiveLocation = "" // Clear archive location

			// Set warmup period for reactivated relationships
			warmupEnd := now.Add(rt.warmupDuration)
			rel.WarmupUntil = &warmupEnd
			rel.CurrentRate = 0.1

			rel.UpdateKeys()
			allRelationships = append(allRelationships, *rel)
		}

		archivesToCleanup = append(archivesToCleanup, archiveLocation)
	}

	// Batch write all restored relationships to DynamoDB
	if err := common.ValidateSliceNotEmpty("restored_relationships", allRelationships); err == nil {
		result, err := batchWriter.WriteItems(ctx, allRelationships)
		if err != nil {
			rt.logger.Error("Failed to batch write restored relationships",
				zap.Int("total_relationships", len(allRelationships)),
				zap.Error(err))
			return errors.Join(ErrBatchWriteRestoredRelationshipsFailed, err)
		}

		rt.logger.Info("Batch restore completed",
			zap.Int("total_items", result.TotalItems),
			zap.Int("processed_items", result.ProcessedItems),
			zap.Int("failed_items", result.FailedItems),
			zap.Duration("duration", result.Duration))

		// Clean up S3 archives after successful restore
		for _, archiveLocation := range archivesToCleanup {
			if err := rt.cleanupS3Archive(ctx, archiveLocation); err != nil {
				rt.logger.Warn("Failed to cleanup S3 archive",
					zap.String("archive_location", archiveLocation),
					zap.Error(err))
			}
		}
	}

	return nil
}

// ArchiveInstanceGroup archives a group of relationships for a single instance (public interface)
func (rt *RelationshipTracker) ArchiveInstanceGroup(ctx context.Context, targetInstance string, relationships []models.FederationRelationship) error {
	return rt.archiveInstanceGroup(ctx, targetInstance, relationships)
}
