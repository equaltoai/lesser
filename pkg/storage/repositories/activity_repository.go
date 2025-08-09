package repositories

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	// Removed unused import
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

// CreateActivity stores an activity in the database - matches legacy implementation
func (r *ActivityRepository) CreateActivity(ctx context.Context, activity *activitypub.Activity) error {
	if activity.ID == "" {
		return fmt.Errorf("activity ID is required")
	}

	// Extract username from actor ID (e.g., "https://example.com/users/alice" -> "alice")
	username := activityExtractUsernameFromActorID(activity.Actor)
	if username == "" {
		return fmt.Errorf("invalid actor ID format")
	}

	// Build the activity record
	now := time.Now()
	timestamp := now.Format(time.RFC3339Nano)

	record := &models.Activity{
		PK:        "ACTOR#" + username,
		SK:        "ACTIVITY#" + timestamp + "#" + activity.ID,
		Activity:  activity,
		CreatedAt: now,
	}

	// If this is an inbox activity (someone else's activity delivered to us), set GSI keys
	if isInboxActivity(activity, username) {
		record.GSI1PK = "INBOX#" + username
		record.GSI1SK = timestamp
	}

	// Store in DynamoDB
	if err := r.db.WithContext(ctx).Model(record).Create(); err != nil {
		r.logger.Error("failed to create activity",
			zap.String("activity_id", activity.ID),
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to create activity: %w", err)
	}

	r.logger.Info("activity created successfully",
		zap.String("activity_id", activity.ID),
		zap.String("username", username),
		zap.String("type", activity.Type))

	return nil
}

// GetActivity retrieves an activity by ID - matches legacy implementation
func (r *ActivityRepository) GetActivity(ctx context.Context, id string) (*activitypub.Activity, error) {
	// We need to scan for the activity since we don't know the username
	// In a production system, you might want to extract username from the ID
	// or maintain a separate GSI for activity lookups

	// Use DynamORM to scan the table for the activity by ID
	// This is a simplified approach - in production you might want a GSI for activity ID lookups
	var activities []models.Activity
	err := r.db.WithContext(ctx).Model(&models.Activity{}).
		Where("SK", "CONTAINS", id).
		Limit(50). // Limit results to avoid scanning too much
		All(&activities)

	if err != nil {
		r.logger.Error("failed to search for activity",
			zap.String("activity_id", id),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get activity: %w", err)
	}

	// Filter activities to find exact match
	for _, activity := range activities {
		if activity.Activity != nil && activity.Activity.ID == id {
			return activity.Activity, nil
		}
	}

	// Activity not found
	return nil, fmt.Errorf("activity not found: %s", id)
}

// GetInboxActivities retrieves inbox activities for a user - matches legacy implementation
func (r *ActivityRepository) GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	// Set default limit if not specified
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Query using GSI1 for inbox activities
	query := r.db.WithContext(ctx).Model(&models.Activity{}).
		Index("GSI1").
		Where("GSI1PK", "=", "INBOX#"+username).
		Limit(limit).
		OrderBy("GSI1SK", "DESC") // Newest first

	// If cursor provided, decode and set it
	if cursor != "" {
		decodedCursor, err := activityDecodeCursor(cursor)
		if err != nil {
			r.logger.Warn("invalid cursor provided",
				zap.String("cursor", cursor),
				zap.Error(err))
			// Continue without cursor
		} else {
			query = query.Cursor(decodedCursor)
		}
	}

	// Execute the query
	var activities []models.Activity
	err := query.All(&activities)
	if err != nil {
		r.logger.Error("failed to query inbox activities",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to query inbox activities: %w", err)
	}

	// Convert to ActivityPub activities
	result := make([]*activitypub.Activity, 0, len(activities))
	for _, record := range activities {
		result = append(result, record.Activity)
	}

	// Encode next cursor if there are more results
	var nextCursor string
	if len(activities) == limit {
		// There might be more results
		lastItem := activities[len(activities)-1]
		cursorData := map[string]string{
			"GSI1PK": lastItem.GSI1PK,
			"GSI1SK": lastItem.GSI1SK,
			"PK":     lastItem.PK,
			"SK":     lastItem.SK,
		}
		nextCursor = activityEncodeCursor(cursorData)
	}

	r.logger.Debug("retrieved inbox activities",
		zap.String("username", username),
		zap.Int("count", len(result)),
		zap.Bool("has_more", nextCursor != ""))

	return result, nextCursor, nil
}

// GetOutboxActivities retrieves activities created by a user - matches legacy implementation
func (r *ActivityRepository) GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	// Set default limit if not specified
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Query activities for the user
	query := r.db.WithContext(ctx).Model(&models.Activity{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "BEGINS_WITH", "ACTIVITY#").
		Limit(limit).
		OrderBy("SK", "DESC") // Newest first

	// If cursor provided, decode and set it
	if cursor != "" {
		decodedCursor, err := activityDecodeCursor(cursor)
		if err != nil {
			r.logger.Warn("invalid cursor provided",
				zap.String("cursor", cursor),
				zap.Error(err))
			// Continue without cursor
		} else {
			query = query.Cursor(decodedCursor)
		}
	}

	// Execute the query
	var activities []models.Activity
	err := query.All(&activities)
	if err != nil {
		r.logger.Error("failed to query outbox activities",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to query outbox activities: %w", err)
	}

	// Convert to ActivityPub activities
	result := make([]*activitypub.Activity, 0, len(activities))
	for _, record := range activities {
		result = append(result, record.Activity)
	}

	// Encode next cursor if there are more results
	var nextCursor string
	if len(activities) == limit {
		// There might be more results
		lastItem := activities[len(activities)-1]
		cursorData := map[string]string{
			"PK": lastItem.PK,
			"SK": lastItem.SK,
		}
		nextCursor = activityEncodeCursor(cursorData)
	}

	r.logger.Debug("retrieved outbox activities",
		zap.String("username", username),
		zap.Int("count", len(result)),
		zap.Bool("has_more", nextCursor != ""))

	return result, nextCursor, nil
}

// GetCollection retrieves a collection for an actor - matches legacy implementation
func (r *ActivityRepository) GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error) {
	r.logger.Debug("GetCollection called",
		zap.String("username", username),
		zap.String("collection_type", collectionType),
		zap.Int("limit", limit))

	// Handle activity-related collections directly
	switch collectionType {
	case activitypub.InboxCollection:
		// Get inbox activities and convert to collection page
		activities, nextCursor, err := r.GetInboxActivities(ctx, username, limit, cursor)
		if err != nil {
			return nil, fmt.Errorf("failed to get inbox activities: %w", err)
		}
		return r.createActivityCollectionPage(username, collectionType, activities, nextCursor, limit), nil

	case activitypub.OutboxCollection:
		// Get outbox activities and convert to collection page
		activities, nextCursor, err := r.GetOutboxActivities(ctx, username, limit, cursor)
		if err != nil {
			return nil, fmt.Errorf("failed to get outbox activities: %w", err)
		}
		return r.createActivityCollectionPage(username, collectionType, activities, nextCursor, limit), nil

	case activitypub.FollowersCollection, activitypub.FollowingCollection, activitypub.LikedCollection:
		// These collections are handled by other repositories
		// The adapter should route these to the appropriate repository
		return nil, fmt.Errorf("collection type %s should be handled by adapter routing to appropriate repository", collectionType)

	default:
		// Unknown collection type - return empty collection
		return r.createEmptyCollectionPage(username, collectionType), nil
	}
}

// createActivityCollectionPage creates an OrderedCollectionPage from activities
func (r *ActivityRepository) createActivityCollectionPage(username, collectionType string, activities []*activitypub.Activity, nextCursor string, limit int) *activitypub.OrderedCollectionPage {
	// Note: config import would be needed for baseURL, but this is a simplified version
	baseURL := "https://your-domain.com" // This should be injected or retrieved from config
	actorID := fmt.Sprintf("%s/users/%s", baseURL, username)
	collectionID := fmt.Sprintf("%s/%s", actorID, collectionType)

	page := &activitypub.OrderedCollectionPage{
		CollectionPage: activitypub.CollectionPage{
			Collection: activitypub.Collection{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					ID:      collectionID,
					Type:    activitypub.OrderedCollectionType,
				},
			},
			PartOf: collectionID,
		},
	}

	// Convert activities to interfaces
	items := make([]any, len(activities))
	for i, activity := range activities {
		items[i] = activity
	}
	page.OrderedItems = items

	// Set pagination info
	if nextCursor != "" {
		page.Next = fmt.Sprintf("%s?cursor=%s&limit=%d", collectionID, nextCursor, limit)
	}

	page.TotalItems = len(items)
	return page
}

// createEmptyCollectionPage creates an empty OrderedCollectionPage
func (r *ActivityRepository) createEmptyCollectionPage(username, collectionType string) *activitypub.OrderedCollectionPage {
	baseURL := "https://your-domain.com" // This should be injected or retrieved from config
	actorID := fmt.Sprintf("%s/users/%s", baseURL, username)
	collectionID := fmt.Sprintf("%s/%s", actorID, collectionType)

	page := &activitypub.OrderedCollectionPage{
		CollectionPage: activitypub.CollectionPage{
			Collection: activitypub.Collection{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					ID:      collectionID,
					Type:    activitypub.OrderedCollectionType,
				},
				OrderedItems: []any{},
				TotalItems:   0,
			},
			PartOf: collectionID,
		},
	}

	return page
}

// GetWeeklyActivity retrieves weekly activity statistics
func (r *ActivityRepository) GetWeeklyActivity(ctx context.Context, weekTimestamp int64) (*storage.WeeklyActivity, error) {
	// For now, we'll create a simple implementation that counts activities in that week
	// In a production system, this would likely use a separate analytics table

	// Calculate week start and end
	weekStart := time.Unix(weekTimestamp, 0)
	weekEnd := weekStart.Add(7 * 24 * time.Hour)

	// Query activities for the week
	// This is a simplified implementation - in production you'd likely use a separate analytics table
	var activities []models.Activity
	err := r.db.WithContext(ctx).Model(&models.Activity{}).
		Where("CreatedAt", ">=", weekStart).
		Where("CreatedAt", "<", weekEnd).
		All(&activities)

	if err != nil {
		return nil, fmt.Errorf("failed to query weekly activities: %w", err)
	}

	// Count different types of activities
	statuses := int64(0)
	logins := int64(0)        // Would need separate tracking
	registrations := int64(0) // Would need separate tracking

	for _, activity := range activities {
		if activity.Activity != nil {
			switch activity.Activity.Type {
			case "Create":
				statuses++
			}
		}
	}

	return &storage.WeeklyActivity{
		Week:          fmt.Sprintf("%d", weekTimestamp),
		Statuses:      int(statuses),
		Logins:        int(logins),
		Registrations: int(registrations),
	}, nil
}

// RecordActivity records general activity metrics
func (r *ActivityRepository) RecordActivity(ctx context.Context, activityType string, actorID string, timestamp time.Time) error {
	// Create a simple activity record
	// In production, this might aggregate into time buckets for efficient querying

	pk := fmt.Sprintf("activity_metric#%s", actorID)
	sk := fmt.Sprintf("%s#%s", activityType, timestamp.Format(time.RFC3339Nano))

	activityRecord := map[string]interface{}{
		"PK":           pk,
		"SK":           sk,
		"ActivityType": activityType,
		"ActorID":      actorID,
		"Timestamp":    timestamp.Format(time.RFC3339),
		"CreatedAt":    timestamp,
		"Type":         "activity_metric",
	}

	// Use the generic model interface to store this
	if err := r.db.WithContext(ctx).Model(activityRecord).Create(); err != nil {
		r.logger.Error("failed to record activity metric",
			zap.String("activity_type", activityType),
			zap.String("actor_id", actorID),
			zap.Error(err))
		return fmt.Errorf("failed to record activity: %w", err)
	}

	return nil
}

// GetHashtagActivity retrieves activities related to a hashtag since a given time
func (r *ActivityRepository) GetHashtagActivity(ctx context.Context, hashtag string, since time.Time) ([]*storage.Activity, error) {
	// Query activities that contain the hashtag
	// This is a simplified implementation - production would likely use a hashtag index

	var activities []models.Activity
	err := r.db.WithContext(ctx).Model(&models.Activity{}).
		Where("CreatedAt", ">=", since).
		All(&activities)

	if err != nil {
		return nil, fmt.Errorf("failed to query hashtag activities: %w", err)
	}

	// Filter activities that contain the hashtag
	var result []*storage.Activity
	for _, activityModel := range activities {
		if activityModel.Activity == nil {
			continue
		}

		// Check if the activity contains the hashtag
		activityJSON, _ := json.Marshal(activityModel.Activity)
		if strings.Contains(strings.ToLower(string(activityJSON)), strings.ToLower("#"+hashtag)) {
			result = append(result, &storage.Activity{
				ID:        activityModel.Activity.ID,
				Type:      activityModel.Activity.Type,
				Actor:     activityModel.Activity.Actor,
				Object:    fmt.Sprintf("%v", activityModel.Activity.Object), // Convert interface{} to string
				Published: activityModel.CreatedAt,
				Content:   extractContent(*activityModel.Activity),
			})
		}
	}

	return result, nil
}

// RecordFederationActivity records federation activity metrics
func (r *ActivityRepository) RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error {
	pk := fmt.Sprintf("federation#%s", activity.Domain)
	sk := fmt.Sprintf("%s#%s", activity.Type, activity.Timestamp.Format(time.RFC3339Nano))

	federationRecord := map[string]interface{}{
		"PK":           pk,
		"SK":           sk,
		"ID":           activity.ID,
		"Domain":       activity.Domain,
		"Type":         activity.Type,
		"ActivityType": activity.ActivityType,
		"ByteSize":     activity.ByteSize,
		"Success":      activity.Success,
		"ResponseTime": activity.ResponseTime,
		"ErrorMessage": activity.ErrorMessage,
		"Timestamp":    activity.Timestamp.Format(time.RFC3339),
		"CreatedAt":    activity.Timestamp,
		"RecordType":   "federation_activity",
	}

	if err := r.db.WithContext(ctx).Model(federationRecord).Create(); err != nil {
		r.logger.Error("failed to record federation activity",
			zap.String("domain", activity.Domain),
			zap.String("type", activity.Type),
			zap.Error(err))
		return fmt.Errorf("failed to record federation activity: %w", err)
	}

	r.logger.Debug("recorded federation activity",
		zap.String("domain", activity.Domain),
		zap.String("type", activity.Type),
		zap.String("activity_type", activity.ActivityType),
		zap.Bool("success", activity.Success))

	return nil
}

// extractContent extracts content from an ActivityPub activity
func extractContent(activity activitypub.Activity) string {
	if activity.Object != nil {
		// Try to extract content from object
		if objMap, ok := activity.Object.(map[string]interface{}); ok {
			if content, exists := objMap["content"]; exists {
				if contentStr, ok := content.(string); ok {
					return contentStr
				}
			}
		}
	}
	return ""
}

// Helper functions to match legacy implementation

// activityExtractUsernameFromActorID extracts username from an actor ID
// e.g., "https://example.com/users/alice" -> "alice"
func activityExtractUsernameFromActorID(actorID string) string {
	parts := strings.Split(actorID, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-1]
}

// isInboxActivity determines if an activity should be stored in the inbox
// An activity is an inbox activity if the actor is different from the local user
func isInboxActivity(activity *activitypub.Activity, localUsername string) bool {
	actorUsername := activityExtractUsernameFromActorID(activity.Actor)
	return actorUsername != localUsername
}

// activityEncodeCursor encodes a map to a string cursor
func activityEncodeCursor(data map[string]string) string {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(jsonData)
}

// activityDecodeCursor decodes a string cursor to a map
func activityDecodeCursor(cursor string) (string, error) {
	data, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return "", fmt.Errorf("invalid cursor format: %w", err)
	}

	var cursorMap map[string]string
	if err := json.Unmarshal(data, &cursorMap); err != nil {
		return "", fmt.Errorf("failed to unmarshal cursor: %w", err)
	}

	// DynamORM expects a string cursor, so we'll re-encode it
	// This is a simplified approach - in production you might handle this differently
	return string(data), nil
}
