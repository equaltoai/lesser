package repositories

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

// ActivityRepository implements activity operations using enhanced repository patterns
type ActivityRepository struct {
	*EnhancedBaseRepository[*models.Activity]
}

// NewActivityRepository creates a new activity repository with enhanced functionality
func NewActivityRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *ActivityRepository {
	// Create enhanced repository optimized for ActivityPub operations
	enhancedRepo := NewEnhancedBaseRepository[*models.Activity](db, tableName, logger, costService, "ActivityRepository", "activity")

	// Set up enhanced services for ActivityPub operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService()) // ActivityPub protocol permissions
	enhancedRepo.SetCachingService(NewInMemoryCachingService())      // Cache recent activities
	enhancedRepo.SetEventService(NewDefaultEventService())           // Critical for ActivityPub federation

	return &ActivityRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreateActivity stores an activity in the database - matches legacy implementation
func (r *ActivityRepository) CreateActivity(ctx context.Context, activity *activitypub.Activity) error {
	if err := common.ValidateRequiredParam("activity.ID", activity.ID); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityActivity, "create")
	}

	// Extract username from actor ID (e.g., "https://example.com/users/alice" -> "alice")
	username := activityExtractUsernameFromActorID(activity.Actor)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityActivity, "create")
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

	// Direct lookup by activity ID (no scans).
	record.GSI2PK = "ACTIVITYID#" + activity.ID
	record.GSI2SK = record.SK

	// If this is an inbox activity (someone else's activity delivered to us), set GSI keys
	if isInboxActivity(activity, username) {
		record.GSI1PK = "INBOX#" + username
		record.GSI1SK = timestamp
	}

	// Store using enhanced validation and creation
	if err := r.ValidateAndCreate(ctx, record); err != nil {
		return ErrorHandler.HandleCreateError(err, "activity", activity.ID)
	}

	// Maintain the O(1) active-month rollup: count the actor once for its UTC
	// day (best-effort, never fails the create — see instance_counts.go).
	day := models.DayFormat(time.Now())
	if activity.Published != nil {
		day = models.DayFormat(*activity.Published)
	}
	recordActivityActorDay(ctx, r.db, r.logger, activity.Actor, day)

	r.logger.Info("activity created successfully",
		zap.String("activity_id", activity.ID),
		zap.String("username", username),
		zap.String("type", activity.Type))

	return nil
}

// GetActivity retrieves an activity by ID - matches legacy implementation
func (r *ActivityRepository) GetActivity(ctx context.Context, id string) (*activitypub.Activity, error) {
	var activities []*models.Activity

	err := r.db.WithContext(ctx).Model(&models.Activity{}).
		Index("gsi2").
		Where("gsi2PK", "=", "ACTIVITYID#"+id).
		Limit(1).
		All(&activities)

	if err != nil {
		r.logger.Error("failed to get activity",
			zap.String("activity_id", id),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "activity", id)
	}

	if err := common.ValidateSliceNotEmpty("activities", activities); err != nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityActivity, id)
	}

	if activities[0].Activity == nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityActivity, id)
	}

	return activities[0].Activity, nil
}

// DeleteActivity removes an activity by ID.
func (r *ActivityRepository) DeleteActivity(ctx context.Context, id string) error {
	var activities []*models.Activity

	err := r.db.WithContext(ctx).Model(&models.Activity{}).
		Index("gsi2").
		Where("gsi2PK", "=", "ACTIVITYID#"+id).
		Limit(1).
		All(&activities)
	if err != nil {
		r.logger.Error("failed to find activity for delete",
			zap.String("activity_id", id),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityActivity, id)
	}

	if len(activities) == 0 || activities[0] == nil {
		return nil
	}

	if err := r.Delete(ctx, activities[0].PK, activities[0].SK); err != nil {
		r.logger.Error("failed to delete activity",
			zap.String("activity_id", id),
			zap.String("pk", activities[0].PK),
			zap.String("sk", activities[0].SK),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityActivity, id)
	}

	r.logger.Info("activity deleted successfully", zap.String("activity_id", id))
	return nil
}

// GetInboxActivities retrieves inbox activities for a user - matches legacy implementation
const (
	activityDefaultLimit      = 20
	activityMaxLimit          = 100
	maxOutboxPublicQueryPages = 4
)

func clampActivityLimit(limit int) int {
	if limit <= 0 {
		return activityDefaultLimit
	}
	if limit > activityMaxLimit {
		return activityMaxLimit
	}
	return limit
}

// GetInboxActivities returns inbox activities ordered newest-first with opaque pagination cursors.
func (r *ActivityRepository) GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	safeLimit := clampActivityLimit(limit)

	// Query using GSI1 for inbox activities
	// Note: Using direct DynamORM query since BaseRepository doesn't have GSI query with custom cursor handling
	query := r.db.WithContext(ctx).Model(&models.Activity{}).
		Index("gsi1").
		Where("gsi1PK", "=", "INBOX#"+username).
		Limit(safeLimit+1).
		OrderBy("gsi1SK", "DESC") // Newest first

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
	var activities []*models.Activity
	err := query.All(&activities)
	if err != nil {
		r.logger.Error("failed to query inbox activities",
			zap.String("username", username),
			zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, "activity", "inbox")
	}

	// Track cost using BaseRepository method
	if r.costService != nil {
		itemCount := int64(len(activities))
		estimatedRU := itemCount
		if estimatedRU == 0 {
			estimatedRU = 1
		}
		if err := r.TrackRead(ctx, "GSI1_Query", estimatedRU); err != nil {
			r.logger.Warn("failed to track read operation", zap.Error(err))
		}
	}

	// Convert to ActivityPub activities
	hasMore := len(activities) > safeLimit
	if hasMore {
		activities = activities[:safeLimit]
	}

	result := make([]*activitypub.Activity, 0, len(activities))
	for _, record := range activities {
		result = append(result, record.Activity)
	}

	// Encode next cursor if there are more results
	var nextCursor string
	if hasMore && len(result) > 0 {
		// There might be more results
		lastItem := activities[len(activities)-1]
		cursorData := map[string]string{
			"gsi1PK": lastItem.GSI1PK,
			"gsi1SK": lastItem.GSI1SK,
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

// GetOutboxActivities retrieves public-addressed activities created by a user.
//
// The ActivityPub outbox endpoint is unauthenticated and externally visible, so
// followers-only and direct activities must not be returned from this read path
// even though they share the same actor partition as public activities.
func (r *ActivityRepository) GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	safeLimit := clampActivityLimit(limit)
	cursorData := ""
	if cursor != "" {
		var cursorErr error
		cursorData, cursorErr = activityDecodeCursor(cursor)
		if cursorErr != nil {
			r.logger.Warn("invalid cursor provided",
				zap.String("cursor", cursor),
				zap.Error(cursorErr))
		}
	}

	result := make([]*activitypub.Activity, 0, safeLimit)
	var nextCursor string
	for pagesQueried := 0; pagesQueried < maxOutboxPublicQueryPages; pagesQueried++ {
		activities, hasMore, batchCursor, err := r.queryOutboxActivityRecords(ctx, username, safeLimit, cursorData)
		if err != nil {
			r.logger.Error("failed to query outbox activities",
				zap.String("username", username),
				zap.Error(err))
			return nil, "", ErrorHandler.HandleQueryError(err, "activity", "outbox")
		}
		if len(activities) == 0 {
			break
		}

		for i, record := range activities {
			if record == nil || !activitypub.IsPublicAddressedActivity(record.Activity) {
				continue
			}
			result = append(result, record.Activity)
			if len(result) == safeLimit {
				if hasMore || i < len(activities)-1 {
					nextCursor = activityEncodeCursor(map[string]string{
						"PK": record.PK,
						"SK": record.SK,
					})
				}
				r.logger.Debug("retrieved outbox activities",
					zap.String("username", username),
					zap.Int("count", len(result)),
					zap.Bool("has_more", nextCursor != ""))
				return result, nextCursor, nil
			}
		}

		if !hasMore {
			break
		}
		if batchCursor == "" || batchCursor == cursorData {
			r.logger.Warn("stopping public outbox pagination because cursor did not advance",
				zap.String("username", username))
			break
		}

		if pagesQueried == maxOutboxPublicQueryPages-1 {
			r.logger.Warn("stopping public outbox pagination after bounded query budget",
				zap.String("username", username),
				zap.Int("pages_queried", pagesQueried+1),
				zap.Int("returned_public_activities", len(result)),
				zap.Bool("has_more_private_or_filtered_activities", true))
			break
		}

		cursorData = batchCursor
	}

	r.logger.Debug("retrieved outbox activities",
		zap.String("username", username),
		zap.Int("count", len(result)),
		zap.Bool("has_more", nextCursor != ""))

	return result, nextCursor, nil
}

func (r *ActivityRepository) queryOutboxActivityRecords(ctx context.Context, username string, safeLimit int, cursorData string) ([]*models.Activity, bool, string, error) {
	pk := "ACTOR#" + username
	const skPrefix = "ACTIVITY#"

	var activities []*models.Activity
	query := r.db.WithContext(ctx).Model(&models.Activity{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", skPrefix).
		Limit(safeLimit+1).
		OrderBy("SK", "DESC")

	if cursorData != "" {
		query = query.Cursor(cursorData)
	}

	if err := query.All(&activities); err != nil {
		return nil, false, "", err
	}

	if r.costService != nil {
		itemCount := int64(len(activities))
		estimatedRU := itemCount
		if estimatedRU == 0 {
			estimatedRU = 1
		}
		if err := r.TrackRead(ctx, "Query", estimatedRU); err != nil {
			r.logger.Warn("failed to track read operation", zap.Error(err))
		}
	}

	hasMore := len(activities) > safeLimit
	if hasMore {
		activities = activities[:safeLimit]
	}

	nextCursor := ""
	if hasMore && len(activities) > 0 {
		lastItem := activities[len(activities)-1]
		nextCursor = activityQueryCursor(map[string]string{
			"PK": lastItem.PK,
			"SK": lastItem.SK,
		})
	}

	return activities, hasMore, nextCursor, nil
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
			return nil, ErrorHandler.HandleGetError(err, "activity", "inbox activities")
		}
		return r.createActivityCollectionPage(username, collectionType, activities, nextCursor, limit), nil

	case activitypub.OutboxCollection:
		// Get outbox activities and convert to collection page
		activities, nextCursor, err := r.GetOutboxActivities(ctx, username, limit, cursor)
		if err != nil {
			return nil, ErrorHandler.HandleGetError(err, "activity", "outbox activities")
		}
		return r.createActivityCollectionPage(username, collectionType, activities, nextCursor, limit), nil

	case activitypub.FollowersCollection, activitypub.FollowingCollection, activitypub.LikedCollection:
		// These collections are handled by other repositories
		// The adapter should route these to the appropriate repository
		return nil, ErrorHandler.HandleGetError(storage.ErrInvalidInput, "activity collection", collectionType)

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

// RecordActivity records general activity metrics
func (r *ActivityRepository) RecordActivity(ctx context.Context, activityType string, actorID string, timestamp time.Time) error {
	// Create a simple activity record
	// In production, this might aggregate into time buckets for efficient querying
	// Note: This uses direct DynamORM since it's not an Activity model

	activityRecord := models.NewActivityMetric(activityType, actorID, timestamp)

	// Use direct DynamORM since this is not an Activity model
	// BaseRepository is typed for Activity models only
	if err := r.db.WithContext(ctx).Model(activityRecord).Create(); err != nil {
		r.logger.Error("failed to record activity metric",
			zap.String("activity_type", activityType),
			zap.String("actor_id", actorID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "activity metric", actorID)
	}

	// Track cost manually since we're not using BaseRepository
	if r.costService != nil {
		if err := r.TrackWrite(ctx, "PutItem", 1); err != nil {
			r.logger.Warn("failed to track write operation", zap.Error(err))
		}
	}

	return nil
}

// GetHashtagActivity retrieves activities related to a hashtag since a given time
func (r *ActivityRepository) GetHashtagActivity(ctx context.Context, hashtag string, since time.Time) ([]*storage.Activity, error) {
	// Query activities that contain the hashtag. The read is a page-bounded
	// iteration with an explicit page cap (wave #1469): each page carries a
	// clamped Limit and the loop stops after maxHashtagPages pages instead of
	// one unbounded scan.
	const hashtagPageSize = 500
	const maxHashtagPages = 100

	baseQuery := r.db.WithContext(ctx).Model(&models.Activity{}).
		Where("CreatedAt", ">=", since).
		Limit(hashtagPageSize)

	var activities []*models.Activity
	cursor := ""
	for page := 0; page < maxHashtagPages; page++ {
		var pageActivities []*models.Activity
		query := baseQuery
		if cursor != "" {
			query = query.Cursor(cursor)
		}
		res, err := query.AllPaginated(&pageActivities)
		if err != nil {
			return nil, ErrorHandler.HandleQueryError(err, "activity", "hashtag activities")
		}
		activities = append(activities, pageActivities...)
		if res == nil || !res.HasMore || res.NextCursor == "" || len(pageActivities) < hashtagPageSize {
			break
		}
		cursor = res.NextCursor
	}

	// Track cost manually since we're using a page-bounded scan
	if r.costService != nil {
		itemCount := int64(len(activities))
		estimatedRU := itemCount
		if estimatedRU == 0 {
			estimatedRU = 1
		}
		if err := r.TrackRead(ctx, "Scan", estimatedRU); err != nil {
			r.logger.Warn("failed to track read operation", zap.Error(err))
		}
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

	// Use direct DynamORM since this is not an Activity model
	if err := r.db.WithContext(ctx).Model(federationRecord).Create(); err != nil {
		r.logger.Error("failed to record federation activity",
			zap.String("domain", activity.Domain),
			zap.String("type", activity.Type),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "federation activity", activity.Domain)
	}

	// Track cost manually since we're not using BaseRepository
	if r.costService != nil {
		if err := r.TrackWrite(ctx, "PutItem", 1); err != nil {
			r.logger.Warn("failed to track write operation", zap.Error(err))
		}
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
	return strings.ToLower(strings.TrimSpace(parts[len(parts)-1]))
}

// isInboxActivity determines if an activity should be stored in the inbox
// An activity is an inbox activity if the actor is different from the local user
func isInboxActivity(activity *activitypub.Activity, localUsername string) bool {
	actorUsername := activityExtractUsernameFromActorID(activity.Actor)
	return actorUsername != strings.ToLower(strings.TrimSpace(localUsername))
}

// activityEncodeCursor encodes a map to a string cursor
func activityEncodeCursor(data map[string]string) string {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(jsonData)
}

func activityQueryCursor(data map[string]string) string {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return string(jsonData)
}

// activityDecodeCursor decodes a string cursor to a map
func activityDecodeCursor(cursor string) (string, error) {
	// Validate cursor format first
	if err := common.ValidateRepositoryCursor(cursor); err != nil {
		return "", ErrorHandler.HandleGetError(err, "activity", "cursor")
	}

	data, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return "", ErrorHandler.HandleGetError(err, "activity", "cursor format")
	}

	var cursorMap map[string]string
	if err := json.Unmarshal(data, &cursorMap); err != nil {
		return "", ErrorHandler.HandleGetError(err, "activity", "unmarshal cursor")
	}

	// DynamORM expects a string cursor, so we'll re-encode it
	// This is a simplified approach - in production you might handle this differently
	return string(data), nil
}
