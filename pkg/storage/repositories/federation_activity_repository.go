package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// FederationActivityRepository handles federation activity persistence
type FederationActivityRepository struct {
	*BaseRepository[*models.FederationActivity]
}

// NewFederationActivityRepository creates a new federation activity repository
func NewFederationActivityRepository(db core.DB, tableName string, logger *zap.Logger) *FederationActivityRepository {
	return &FederationActivityRepository{
		BaseRepository: NewBaseRepository[*models.FederationActivity](db, tableName, logger),
	}
}

// NewFederationActivityRepositoryWithCostTracking creates a new federation activity repository with cost tracking
func NewFederationActivityRepositoryWithCostTracking(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *FederationActivityRepository {
	return &FederationActivityRepository{
		BaseRepository: NewBaseRepositoryWithCostTracking[*models.FederationActivity](db, tableName, logger, costService, "federation_activity"),
	}
}

// RecordFederationActivity creates a new federation activity with ActivityPub protocol logging
func (r *FederationActivityRepository) RecordFederationActivity(ctx context.Context, activity *models.FederationActivity) error {
	// Call BeforeCreate to set up the model with federation activity logic
	if err := activity.BeforeCreate(); err != nil {
		return fmt.Errorf("%w: %w", ErrFederationActivityValidationFailed, err)
	}

	// Log federation activity tracking for audit trail
	r.logger.Debug("recording federation activity for audit trail",
		zap.String("id", activity.ID),
		zap.String("domain", activity.Domain),
		zap.String("activity_type", activity.ActivityType),
		zap.String("actor_id", activity.ActorID))

	// Use BaseRepository.Create for CRUD operation
	err := r.Create(ctx, activity)
	if err != nil {
		return MapErrorWithContext(err, "failed to record federation activity")
	}

	return nil
}

// GetFederationActivity retrieves a federation activity by ID and domain using GSI
func (r *FederationActivityRepository) GetFederationActivity(ctx context.Context, domain, id string) (*models.FederationActivity, error) {
	var activities []*models.FederationActivity

	// Query by actor-index GSI to find activity by domain and ID
	err := r.db.WithContext(ctx).Model(&models.FederationActivity{}).
		Index("actor-index").
		Where("Domain", "=", domain).
		Where("ID", "=", id).
		All(&activities)

	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get federation activity")
	}

	if len(activities) == 0 {
		return nil, fmt.Errorf("%w: domain=%s, id=%s", ErrFederationActivityNotFound, domain, id)
	}

	return activities[0], nil
}

// ListByDomain lists federation activities for a specific domain - federation analytics logic preserved
func (r *FederationActivityRepository) ListByDomain(ctx context.Context, domain string, startTime, endTime time.Time, limit int) ([]*models.FederationActivity, error) {
	var activities []*models.FederationActivity

	// Construct SK range for time-based query (preserve exact ActivityPub protocol key patterns)
	startSK := fmt.Sprintf("activity#%s", startTime.Format("20060102150405"))
	endSK := fmt.Sprintf("activity#%s", endTime.Format("20060102150405"))

	// Use BaseRepository's underlying db but preserve federation-specific query logic
	query := r.db.WithContext(ctx).Model(&models.FederationActivity{}).
		Where("PK", "=", fmt.Sprintf("fed_activity#%s", domain)).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", "DESC").
		Limit(limit)

	err := query.All(&activities)

	// Track cost for federation activity query
	if r.costService != nil {
		if err := r.TrackRead(ctx, "ListFederationByDomain", int64(len(activities))); err != nil {
			r.logger.Warn("failed to track read operation", zap.Error(err))
		}
	}

	if err != nil {
		r.logger.Error("failed to list federation activities by domain",
			zap.Error(err),
			zap.String("domain", domain),
			zap.Time("startTime", startTime),
			zap.Time("endTime", endTime))
		return nil, MapErrorWithContext(err, "failed to list federation activities by domain")
	}

	return activities, nil
}

// ListByType lists federation activities by type - ActivityPub protocol compliance queries preserved
func (r *FederationActivityRepository) ListByType(ctx context.Context, activityType string, startTime, endTime time.Time, limit int) ([]*models.FederationActivity, error) {
	// Use shared GSI query helper with federation-specific parameters
	return r.QueryGSIWithTimeRangeHelper(ctx,
		"type-index", "GSI1PK", "GSI1SK",
		fmt.Sprintf("FED_TYPE#%s", activityType),
		startTime, endTime, limit,
		"list federation activities by type")
}

// ListByActor lists federation activities by actor - ActivityPub actor tracking preserved
func (r *FederationActivityRepository) ListByActor(ctx context.Context, actorID string, startTime, endTime time.Time, limit int) ([]*models.FederationActivity, error) {
	// Use shared GSI query helper with federation-specific parameters
	return r.QueryGSIWithTimeRangeHelper(ctx,
		"actor-index", "GSI2PK", "GSI2SK",
		fmt.Sprintf("FED_ACTOR#%s", actorID),
		startTime, endTime, limit,
		"list federation activities by actor")
}

// GetRecentActivities gets recent activities across all domains - federation monitoring preserved
func (r *FederationActivityRepository) GetRecentActivities(ctx context.Context, since time.Time, limit int) ([]*models.FederationActivity, error) {
	var activities []*models.FederationActivity

	// Use type index to get recent activities (preserve federation monitoring logic)
	startSK := since.Format(time.RFC3339)

	// Use BaseRepository's underlying db but preserve federation-specific recent query logic
	err := r.db.WithContext(ctx).Model(&models.FederationActivity{}).
		Index("type-index").
		Where("GSI1SK", ">=", startSK).
		OrderBy("GSI1SK", "DESC").
		Limit(limit).
		All(&activities)

	// Track cost for recent federation activities query
	if r.costService != nil {
		if err := r.TrackRead(ctx, "GetRecentFederationActivities", int64(len(activities))); err != nil {
			r.logger.Warn("failed to track read operation", zap.Error(err))
		}
	}

	if err != nil {
		r.logger.Error("failed to get recent federation activities",
			zap.Error(err),
			zap.Time("since", since),
			zap.Int("limit", limit))
		return nil, MapErrorWithContext(err, "failed to get recent federation activities")
	}

	return activities, nil
}

// GetDomainStats gets aggregated statistics for a domain - federation analytics and metrics preserved
func (r *FederationActivityRepository) GetDomainStats(ctx context.Context, domain string, startTime, endTime time.Time) (*DomainStats, error) {
	// Get all activities in range using our domain query method (preserves federation logic)
	activities, err := r.ListByDomain(ctx, domain, startTime, endTime, 10000)
	if err != nil {
		return nil, err
	}

	// Federation analytics aggregation logic preserved
	stats := &DomainStats{
		Domain:         domain,
		StartTime:      startTime,
		EndTime:        endTime,
		TotalCount:     len(activities),
		InboundVolume:  0,
		OutboundVolume: 0,
		SuccessCount:   0,
		ErrorCount:     0,
		ActivityTypes:  make(map[string]int),
		UniqueActors:   make(map[string]bool),
	}

	var totalResponseTime float64

	// Aggregate federation metrics - critical for ActivityPub protocol monitoring
	for _, activity := range activities {
		stats.InboundVolume += activity.InboundSize
		stats.OutboundVolume += activity.OutboundSize

		if activity.Success {
			stats.SuccessCount++
			totalResponseTime += activity.ResponseTime
		} else {
			stats.ErrorCount++
		}

		stats.ActivityTypes[activity.ActivityType]++
		stats.UniqueActors[activity.ActorID] = true
	}

	if stats.SuccessCount > 0 {
		stats.AvgResponseTime = totalResponseTime / float64(stats.SuccessCount)
	}

	stats.UniqueActorCount = len(stats.UniqueActors)

	// Log federation analytics for monitoring
	r.logger.Debug("computed domain statistics for federation monitoring",
		zap.String("domain", domain),
		zap.Int("total_activities", stats.TotalCount),
		zap.Int("success_count", stats.SuccessCount),
		zap.Int("error_count", stats.ErrorCount),
		zap.Float64("avg_response_time", stats.AvgResponseTime))

	return stats, nil
}

// UpdateInstanceInfo updates or creates instance information - federation instance tracking preserved
func (r *FederationActivityRepository) UpdateInstanceInfo(ctx context.Context, info *models.InstanceInfo) error {
	// Store as a separate item with instance information (preserve federation instance tracking)
	item := &InstanceInfoItem{
		PK:          fmt.Sprintf("instance#%s", info.Domain),
		SK:          "info",
		Domain:      info.Domain,
		Software:    info.Software,
		Version:     info.Version,
		PublicKey:   info.PublicKey,
		SharedInbox: info.SharedInbox,
		LastSeen:    info.LastSeen,
		FirstSeen:   info.FirstSeen,
		UpdatedAt:   time.Now(),
	}

	// Try to update first using BaseRepository's underlying db
	err := r.db.WithContext(ctx).Model(item).Update()
	if err != nil {
		// If update fails, try create (federation instance discovery logic)
		item.CreatedAt = time.Now()
		if info.FirstSeen.IsZero() {
			item.FirstSeen = time.Now()
		}
		err = r.db.WithContext(ctx).Model(item).Create()
		if err != nil {
			r.logger.Error("failed to update federation instance info",
				zap.Error(err),
				zap.String("domain", info.Domain))
			return MapErrorWithContext(err, "failed to update instance info")
		}
	}

	// Track cost for federation instance update
	if r.costService != nil {
		if err := r.TrackWrite(ctx, "UpdateFederationInstanceInfo", 1); err != nil {
			r.logger.Warn("failed to track write operation", zap.Error(err))
		}
	}

	r.logger.Debug("updated federation instance info",
		zap.String("domain", info.Domain),
		zap.String("software", info.Software),
		zap.String("version", info.Version))

	return nil
}

// GetInstanceInfo retrieves instance information - federation instance discovery preserved
func (r *FederationActivityRepository) GetInstanceInfo(ctx context.Context, domain string) (*models.InstanceInfo, error) {
	item := &InstanceInfoItem{}

	// Use BaseRepository's underlying db but preserve federation instance query logic
	err := r.db.WithContext(ctx).Model(item).
		Where("PK", "=", fmt.Sprintf("instance#%s", domain)).
		Where("SK", "=", "info").
		First(item)

	// Track cost for federation instance get
	if r.costService != nil {
		if err := r.TrackRead(ctx, "GetFederationInstanceInfo", 1); err != nil {
			r.logger.Warn("failed to track read operation", zap.Error(err))
		}
	}

	if err != nil {
		r.logger.Debug("federation instance info not found",
			zap.String("domain", domain),
			zap.Error(err))
		return nil, MapErrorWithContext(err, "failed to get instance info")
	}

	// Convert InstanceInfoItem to models.InstanceInfo (preserve federation data conversion)
	return &models.InstanceInfo{
		Domain:      item.Domain,
		Software:    item.Software,
		Version:     item.Version,
		PublicKey:   item.PublicKey,
		SharedInbox: item.SharedInbox,
		LastSeen:    item.LastSeen,
		FirstSeen:   item.FirstSeen,
	}, nil
}

// DomainStats represents aggregated statistics for a domain
type DomainStats struct {
	Domain           string
	StartTime        time.Time
	EndTime          time.Time
	TotalCount       int
	SuccessCount     int
	ErrorCount       int
	InboundVolume    int64
	OutboundVolume   int64
	AvgResponseTime  float64
	ActivityTypes    map[string]int
	UniqueActors     map[string]bool
	UniqueActorCount int
}

// InstanceInfoItem represents instance information in DynamoDB
type InstanceInfoItem struct {
	PK string `dynamorm:"pk"`
	SK string `dynamorm:"sk"`

	Domain      string    `json:"domain"`
	Software    string    `json:"software,omitempty"`
	Version     string    `json:"version,omitempty"`
	PublicKey   string    `json:"public_key,omitempty"`
	SharedInbox string    `json:"shared_inbox,omitempty"`
	LastSeen    time.Time `json:"last_seen"`
	FirstSeen   time.Time `json:"first_seen"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName returns the DynamoDB table name
func (InstanceInfoItem) TableName() string {
	return models.MainTableName
}

// BeforeCreate for InstanceInfoItem
func (i *InstanceInfoItem) BeforeCreate() error {
	if i.CreatedAt.IsZero() {
		i.CreatedAt = time.Now()
	}
	if i.UpdatedAt.IsZero() {
		i.UpdatedAt = time.Now()
	}
	return nil
}

// BeforeUpdate for InstanceInfoItem
func (i *InstanceInfoItem) BeforeUpdate() error {
	i.UpdatedAt = time.Now()
	return nil
}
