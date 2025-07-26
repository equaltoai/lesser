package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ActivityRepository implements activity operations using DynamORM
type ActivityRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewActivityRepository creates a new activity repository
func NewActivityRepository(db core.DB, tableName string, logger *zap.Logger) *ActivityRepository {
	return &ActivityRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreateActivity stores an activity in the database
func (r *ActivityRepository) CreateActivity(ctx context.Context, activity *activitypub.Activity) error {
	// Determine direction based on the activity
	direction := "inbox"
	if strings.Contains(activity.ID, "/activities/") {
		direction = "outbox"
	}

	// Extract username from the activity's target or actor
	username := r.extractUsername(activity)
	if username == "" {
		return fmt.Errorf("unable to determine username for activity")
	}

	// Serialize the activity to JSON
	activityJSON, err := json.Marshal(activity)
	if err != nil {
		return fmt.Errorf("failed to marshal activity: %w", err)
	}

	// Create the model
	now := time.Now()
	activityModel := &models.Activity{
		PK:         fmt.Sprintf("activity#%s", activity.ID),
		SK:         fmt.Sprintf("%s#%s#%s", direction, username, now.Format(time.RFC3339Nano)),
		Username:   username,
		Timestamp:  now.Format(time.RFC3339),
		ActivityID: activity.ID,
		Activity:   string(activityJSON),
		Direction:  direction,
		CreatedAt:  now,
	}

	// Store in DynamoDB
	if err := r.db.WithContext(ctx).Model(activityModel).Create(); err != nil {
		r.logger.Error("failed to create activity",
			zap.String("activity_id", activity.ID),
			zap.Error(err))
		return fmt.Errorf("failed to create activity: %w", err)
	}

	r.logger.Info("stored activity",
		zap.String("activity_id", activity.ID),
		zap.String("type", activity.Type),
		zap.String("direction", direction),
		zap.String("username", username))

	return nil
}

// GetActivity retrieves an activity by ID
func (r *ActivityRepository) GetActivity(ctx context.Context, activityID string) (*activitypub.Activity, error) {
	var activityModel models.Activity
	
	query := r.db.WithContext(ctx).Model(&activityModel).
		Where("PK", "=", fmt.Sprintf("activity#%s", activityID))

	if err := query.First(&activityModel); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("activity not found: %s", activityID)
		}
		return nil, fmt.Errorf("failed to get activity: %w", err)
	}

	// Unmarshal the activity
	var activity activitypub.Activity
	if err := json.Unmarshal([]byte(activityModel.Activity), &activity); err != nil {
		return nil, fmt.Errorf("failed to unmarshal activity: %w", err)
	}

	return &activity, nil
}

// GetInboxActivities retrieves inbox activities for a user
func (r *ActivityRepository) GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	query := r.db.WithContext(ctx).Model(&models.Activity{}).
		Index("username-index").
		Where("Username", "=", username).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	var activities []models.Activity
	err := query.All(&activities)
	nextCursor := "" // TODO: implement pagination
	if err != nil {
		return nil, "", fmt.Errorf("failed to scan activities: %w", err)
	}

	// Convert to ActivityPub activities
	result := make([]*activitypub.Activity, 0, len(activities))
	for _, activityModel := range activities {
		// Only include inbox activities
		if activityModel.Direction != "inbox" {
			continue
		}

		var activity activitypub.Activity
		if err := json.Unmarshal([]byte(activityModel.Activity), &activity); err != nil {
			r.logger.Warn("failed to unmarshal activity",
				zap.String("activity_id", activityModel.ActivityID),
				zap.Error(err))
			continue
		}
		result = append(result, &activity)
	}

	return result, nextCursor, nil
}

// extractUsername extracts the username from an activity
func (r *ActivityRepository) extractUsername(activity *activitypub.Activity) string {
	// For inbox activities, extract from To/CC fields
	for _, to := range activity.To {
		if username := r.extractUsernameFromURL(to); username != "" {
			return username
		}
	}
	for _, cc := range activity.CC {
		if username := r.extractUsernameFromURL(cc); username != "" {
			return username
		}
	}

	// For outbox activities, extract from Actor
	if username := r.extractUsernameFromURL(activity.Actor); username != "" {
		return username
	}

	return ""
}

// extractUsernameFromURL extracts username from an actor URL
func (r *ActivityRepository) extractUsernameFromURL(url string) string {
	// Handle local actor URLs like https://domain.com/users/username
	parts := strings.Split(url, "/")
	if len(parts) >= 2 && parts[len(parts)-2] == "users" {
		return parts[len(parts)-1]
	}
	return ""
}