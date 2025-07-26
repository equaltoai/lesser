package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// FederationActivityRepository handles federation activity persistence
type FederationActivityRepository struct {
	dynamorm.BaseRepository
	logger *zap.Logger
}

// NewFederationActivityRepository creates a new federation activity repository
func NewFederationActivityRepository(db core.DB, tableName string, logger *zap.Logger) *FederationActivityRepository {
	return &FederationActivityRepository{
		BaseRepository: *dynamorm.NewBaseRepository(db, tableName),
		logger:         logger,
	}
}

// Create creates a new federation activity record
func (r *FederationActivityRepository) Create(ctx context.Context, activity *models.FederationActivity) error {
	// Call BeforeCreate to set up the model
	if err := activity.BeforeCreate(); err != nil {
		return fmt.Errorf("before create validation failed: %w", err)
	}

	// Create the activity
	err := r.GetDB().Model(activity).Create()
	if err != nil {
		return dynamorm.MapErrorWithContext(err, "failed to create federation activity")
	}

	r.logger.Debug("created federation activity",
		zap.String("id", activity.ID),
		zap.String("domain", activity.Domain),
		zap.String("type", activity.ActivityType))

	return nil
}

// Get retrieves a federation activity by ID and domain
func (r *FederationActivityRepository) Get(ctx context.Context, domain, id string) (*models.FederationActivity, error) {
	activity := &models.FederationActivity{}

	// We need to know the timestamp to construct the SK, so we'll query by GSI
	err := r.GetDB().Model(activity).
		Index("actor-index"). // Use actor index as a workaround
		Where("Domain", "=", domain).
		Where("ID", "=", id).
		First(activity)

	if err != nil {
		return nil, dynamorm.MapErrorWithContext(err, "failed to get federation activity")
	}

	return activity, nil
}

// ListByDomain lists federation activities for a specific domain
func (r *FederationActivityRepository) ListByDomain(ctx context.Context, domain string, startTime, endTime time.Time, limit int) ([]*models.FederationActivity, error) {
	var activities []*models.FederationActivity

	// Construct SK range for time-based query
	startSK := fmt.Sprintf("activity#%s", startTime.Format("20060102150405"))
	endSK := fmt.Sprintf("activity#%s", endTime.Format("20060102150405"))

	query := r.GetDB().Model(&models.FederationActivity{}).
		Where("PK", "=", fmt.Sprintf("fed_activity#%s", domain)).
		Where("SK", ">=", startSK).
		Where("SK", "<=", endSK).
		OrderBy("SK", "DESC").
		Limit(limit)
	
	err := query.All(&activities)

	if err != nil {
		return nil, dynamorm.MapErrorWithContext(err, "failed to list federation activities by domain")
	}

	return activities, nil
}

// ListByType lists federation activities by type
func (r *FederationActivityRepository) ListByType(ctx context.Context, activityType string, startTime, endTime time.Time, limit int) ([]*models.FederationActivity, error) {
	var activities []*models.FederationActivity

	// Use GSI1 for type-based queries
	startSK := fmt.Sprintf("%s", startTime.Format(time.RFC3339))
	endSK := fmt.Sprintf("%s", endTime.Format(time.RFC3339))

	query := r.GetDB().Model(&models.FederationActivity{}).
		Index("type-index").
		Where("GSI1PK", "=", fmt.Sprintf("FED_TYPE#%s", activityType)).
		Where("GSI1SK", ">=", startSK).
		Where("GSI1SK", "<=", endSK).
		OrderBy("GSI1SK", "DESC").
		Limit(limit)
	
	err := query.All(&activities)

	if err != nil {
		return nil, dynamorm.MapErrorWithContext(err, "failed to list federation activities by type")
	}

	return activities, nil
}

// ListByActor lists federation activities by actor
func (r *FederationActivityRepository) ListByActor(ctx context.Context, actorID string, startTime, endTime time.Time, limit int) ([]*models.FederationActivity, error) {
	var activities []*models.FederationActivity

	// Use GSI2 for actor-based queries
	startSK := fmt.Sprintf("%s", startTime.Format(time.RFC3339))
	endSK := fmt.Sprintf("%s", endTime.Format(time.RFC3339))

	query := r.GetDB().Model(&models.FederationActivity{}).
		Index("actor-index").
		Where("GSI2PK", "=", fmt.Sprintf("FED_ACTOR#%s", actorID)).
		Where("GSI2SK", ">=", startSK).
		Where("GSI2SK", "<=", endSK).
		OrderBy("GSI2SK", "DESC").
		Limit(limit)
	
	err := query.All(&activities)

	if err != nil {
		return nil, dynamorm.MapErrorWithContext(err, "failed to list federation activities by actor")
	}

	return activities, nil
}

// GetRecentActivities gets recent activities across all domains
func (r *FederationActivityRepository) GetRecentActivities(ctx context.Context, since time.Time, limit int) ([]*models.FederationActivity, error) {
	var activities []*models.FederationActivity

	// Use type index to get recent activities
	startSK := since.Format(time.RFC3339)

	err := r.GetDB().Model(&models.FederationActivity{}).
		Index("type-index").
		Where("GSI1SK", ">=", startSK).
		OrderBy("GSI1SK", "DESC").
		Limit(limit).
		All(&activities)

	if err != nil {
		return nil, dynamorm.MapErrorWithContext(err, "failed to get recent federation activities")
	}

	return activities, nil
}

// GetDomainStats gets aggregated statistics for a domain
func (r *FederationActivityRepository) GetDomainStats(ctx context.Context, domain string, startTime, endTime time.Time) (*DomainStats, error) {
	activities, err := r.ListByDomain(ctx, domain, startTime, endTime, 10000) // Get all activities in range
	if err != nil {
		return nil, err
	}

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

	return stats, nil
}

// UpdateInstanceInfo updates or creates instance information
func (r *FederationActivityRepository) UpdateInstanceInfo(ctx context.Context, info *models.InstanceInfo) error {
	// Store as a separate item with instance information
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

	err := r.GetDB().Model(item).Update()
	if err != nil {
		// If update fails, try create
		item.CreatedAt = time.Now()
		if info.FirstSeen.IsZero() {
			item.FirstSeen = time.Now()
		}
		err = r.GetDB().Model(item).Create()
		if err != nil {
			return dynamorm.MapErrorWithContext(err, "failed to update instance info")
		}
	}

	return nil
}

// GetInstanceInfo retrieves instance information
func (r *FederationActivityRepository) GetInstanceInfo(ctx context.Context, domain string) (*models.InstanceInfo, error) {
	item := &InstanceInfoItem{}

	err := r.GetDB().Model(item).
		Where("PK", "=", fmt.Sprintf("instance#%s", domain)).
		Where("SK", "=", "info").
		First(item)

	if err != nil {
		return nil, dynamorm.MapErrorWithContext(err, "failed to get instance info")
	}

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
	return "lesser-main"
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