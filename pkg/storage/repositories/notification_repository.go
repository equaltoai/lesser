package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/batch"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// NotificationRepository handles notification operations using DynamORM
type NotificationRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewNotificationRepository creates a new notification repository
func NewNotificationRepository(db core.DB, tableName string, logger *zap.Logger) *NotificationRepository {
	return &NotificationRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreateNotification creates a new notification
func (r *NotificationRepository) CreateNotification(_ context.Context, notification *models.Notification) error {
	if err := notification.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare notification for creation: %w", err)
	}

	err := r.db.Model(notification).Create()
	if err != nil {
		return fmt.Errorf("failed to create notification: %w", err)
	}

	return nil
}

// GetNotification retrieves a notification by ID
func (r *NotificationRepository) GetNotification(_ context.Context, id string) (*models.Notification, error) {
	var notification models.Notification
	err := r.db.Model(&models.Notification{}).
		Where("ID", "=", id).
		First(&notification)
	if err != nil {
		return nil, fmt.Errorf("failed to get notification: %w", err)
	}

	return &notification, nil
}

// GetNotificationsByUser retrieves notifications for a user with pagination
func (r *NotificationRepository) GetNotificationsByUser(_ context.Context, userID string, limit int, cursor string) ([]*models.Notification, string, error) {
	pk := "user#" + userID
	query := r.db.Model(&models.Notification{}).
		Where("PK", "=", pk).
		OrderBy("SK", "DESC") // Most recent first

	// Handle cursor-based pagination
	if cursor != "" {
		query = query.Where("SK", "<", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var notifications []*models.Notification
	err := query.All(&notifications)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get notifications for user: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(notifications) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = notifications[limit-1].SK
		notifications = notifications[:limit] // Trim to requested limit
	}

	return notifications, nextCursor, nil
}

// GetNotificationsByType retrieves notifications by type with pagination
func (r *NotificationRepository) GetNotificationsByType(_ context.Context, notificationType string, limit int, cursor string) ([]*models.Notification, string, error) {
	query := r.db.Model(&models.Notification{}).
		Index("type-index").
		Where("GSI1PK", "=", "NOTIF_TYPE#"+notificationType).
		OrderBy("GSI1SK", "DESC")

	if cursor != "" {
		query = query.Where("GSI1SK", "<", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var notifications []*models.Notification
	err := query.All(&notifications)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get notifications by type: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(notifications) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = notifications[limit-1].GSI1SK
		notifications = notifications[:limit] // Trim to requested limit
	}

	return notifications, nextCursor, nil
}

// GetNotificationsByActor retrieves notifications by actor with pagination
func (r *NotificationRepository) GetNotificationsByActor(_ context.Context, actorID string, limit int, cursor string) ([]*models.Notification, string, error) {
	query := r.db.Model(&models.Notification{}).
		Index("actor-index").
		Where("GSI2PK", "=", "NOTIF_ACTOR#"+actorID).
		OrderBy("GSI2SK", "DESC")

	if cursor != "" {
		query = query.Where("GSI2SK", "<", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var notifications []*models.Notification
	err := query.All(&notifications)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get notifications by actor: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(notifications) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = notifications[limit-1].GSI2SK
		notifications = notifications[:limit] // Trim to requested limit
	}

	return notifications, nextCursor, nil
}

// GetNotificationsByGroup retrieves notifications by group key with pagination
func (r *NotificationRepository) GetNotificationsByGroup(_ context.Context, groupKey string, limit int, cursor string) ([]*models.Notification, string, error) {
	query := r.db.Model(&models.Notification{}).
		Index("group-index").
		Where("GSI3PK", "=", "NOTIF_GROUP#"+groupKey).
		OrderBy("GSI3SK", "DESC")

	if cursor != "" {
		query = query.Where("GSI3SK", "<", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var notifications []*models.Notification
	err := query.All(&notifications)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get notifications by group: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(notifications) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = notifications[limit-1].GSI3SK
		notifications = notifications[:limit] // Trim to requested limit
	}

	return notifications, nextCursor, nil
}

// UpdateNotification updates an existing notification
func (r *NotificationRepository) UpdateNotification(_ context.Context, notification *models.Notification) error {
	if err := notification.BeforeUpdate(); err != nil {
		return fmt.Errorf("failed to prepare notification for update: %w", err)
	}

	err := r.db.Model(notification).Update()
	if err != nil {
		return fmt.Errorf("failed to update notification: %w", err)
	}

	return nil
}

// MarkNotificationAsRead marks a notification as read
func (r *NotificationRepository) MarkNotificationAsRead(ctx context.Context, id string) error {
	notification, err := r.GetNotification(ctx, id)
	if err != nil {
		return err
	}

	notification.MarkRead()
	return r.UpdateNotification(ctx, notification)
}

// MarkNotificationAsUnread marks a notification as unread
func (r *NotificationRepository) MarkNotificationAsUnread(ctx context.Context, id string) error {
	notification, err := r.GetNotification(ctx, id)
	if err != nil {
		return err
	}

	notification.MarkUnread()
	return r.UpdateNotification(ctx, notification)
}

// DeleteNotification deletes a notification
func (r *NotificationRepository) DeleteNotification(ctx context.Context, id string) error {
	notification, err := r.GetNotification(ctx, id)
	if err != nil {
		return err
	}

	err = r.db.Model(notification).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete notification: %w", err)
	}

	return nil
}

// CountUnreadNotifications counts unread notifications for a user
func (r *NotificationRepository) CountUnreadNotifications(_ context.Context, userID string) (int, error) {
	pk := "user#" + userID

	count, err := r.db.Model(&models.Notification{}).
		Where("PK", "=", pk).
		Filter("IsRead", "=", false).
		Count()
	if err != nil {
		return 0, fmt.Errorf("failed to count unread notifications: %w", err)
	}

	return int(count), nil
}

// GetUnreadNotifications retrieves unread notifications for a user
func (r *NotificationRepository) GetUnreadNotifications(_ context.Context, userID string, limit int) ([]*models.Notification, error) {
	pk := "user#" + userID

	var notifications []*models.Notification
	err := r.db.Model(&models.Notification{}).
		Where("PK", "=", pk).
		Filter("IsRead", "=", false).
		OrderBy("SK", "DESC").
		Limit(limit).
		All(&notifications)
	if err != nil {
		return nil, fmt.Errorf("failed to get unread notifications: %w", err)
	}

	return notifications, nil
}

// GetPendingPushNotifications retrieves notifications that need push delivery
func (r *NotificationRepository) GetPendingPushNotifications(_ context.Context, limit int) ([]*models.Notification, error) {
	var notifications []*models.Notification
	err := r.db.Model(&models.Notification{}).
		Filter("PushSent", "=", false).
		Filter("PushError", "=", "").
		OrderBy("CreatedAt", "ASC").
		Limit(limit).
		All(&notifications)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending push notifications: %w", err)
	}

	return notifications, nil
}

// MarkPushNotificationSent marks a notification's push as sent
func (r *NotificationRepository) MarkPushNotificationSent(ctx context.Context, id string) error {
	notification, err := r.GetNotification(ctx, id)
	if err != nil {
		return err
	}

	notification.MarkPushSent()
	return r.UpdateNotification(ctx, notification)
}

// MarkPushNotificationFailed marks a notification's push as failed
func (r *NotificationRepository) MarkPushNotificationFailed(ctx context.Context, id string, errorMsg string) error {
	notification, err := r.GetNotification(ctx, id)
	if err != nil {
		return err
	}

	notification.MarkPushFailed(errorMsg)
	return r.UpdateNotification(ctx, notification)
}

// GetNotificationsWithFilters retrieves notifications with various filters
func (r *NotificationRepository) GetNotificationsWithFilters(_ context.Context, userID string, filters NotificationFilters, limit int, cursor string) ([]*models.Notification, string, error) {
	pk := "user#" + userID
	query := r.db.Model(&models.Notification{}).
		Where("PK", "=", pk).
		OrderBy("SK", "DESC")

	// Apply filters
	if len(filters.Types) > 0 {
		// For multiple types, we'd need to use multiple queries or filter in memory
		// For now, use the first type as primary filter
		if len(filters.Types) == 1 {
			query = query.Filter("Type", "=", filters.Types[0])
		}
	}

	if len(filters.ExcludeTypes) > 0 {
		for _, excludeType := range filters.ExcludeTypes {
			query = query.Filter("Type", "!=", excludeType)
		}
	}

	if filters.ActorID != "" {
		query = query.Filter("ActorID", "=", filters.ActorID)
	}

	if filters.OnlyUnread {
		query = query.Filter("IsRead", "=", false)
	}

	if filters.MinID != "" {
		query = query.Filter("ID", ">=", filters.MinID)
	}

	if filters.MaxID != "" {
		query = query.Filter("ID", "<=", filters.MaxID)
	}

	if filters.SinceID != "" {
		query = query.Filter("ID", ">", filters.SinceID)
	}

	// Handle cursor-based pagination
	if cursor != "" {
		query = query.Where("SK", "<", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var notifications []*models.Notification
	err := query.All(&notifications)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get filtered notifications: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(notifications) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = notifications[limit-1].SK
		notifications = notifications[:limit] // Trim to requested limit
	}

	return notifications, nextCursor, nil
}

// GetDeliveryStatusStats gets statistics about notification delivery status
func (r *NotificationRepository) GetDeliveryStatusStats(_ context.Context, userID string, since time.Time) (*DeliveryStats, error) {
	pk := "user#" + userID
	sinceStr := since.Format("20060102150405")

	var notifications []*models.Notification
	err := r.db.Model(&models.Notification{}).
		Where("PK", "=", pk).
		Filter("SK", ">=", "notif#"+sinceStr).
		All(&notifications)
	if err != nil {
		return nil, fmt.Errorf("failed to get notifications for stats: %w", err)
	}

	stats := &DeliveryStats{}
	for _, notif := range notifications {
		stats.Total++
		if notif.IsRead {
			stats.Read++
		} else {
			stats.Unread++
		}
		if notif.PushSent {
			stats.PushSent++
		}
		if notif.PushError != "" {
			stats.PushFailed++
		}
	}

	return stats, nil
}

// BatchCreateNotifications creates multiple notifications efficiently
func (r *NotificationRepository) BatchCreateNotifications(ctx context.Context, notifications []*models.Notification) error {
	if len(notifications) == 0 {
		return nil
	}

	// Prepare all notifications
	for _, notification := range notifications {
		if err := notification.BeforeCreate(); err != nil {
			return fmt.Errorf("failed to prepare notification for creation: %w", err)
		}
	}

	// Convert to []any for batch operations
	items := make([]any, len(notifications))
	for i, notification := range notifications {
		items[i] = notification
	}

	// Use batch writer for efficient bulk creation
	batchWriter := batch.NewBatchWriter(r.db, batch.BatchWriterConfig{
		BatchSize: batch.DefaultBatchSize,
		Logger:    r.logger,
	})

	result, err := batchWriter.WriteItems(ctx, items)
	if err != nil {
		return fmt.Errorf("failed to batch create notifications: %w", err)
	}

	// Check if any items failed
	if result.FailedItems > 0 {
		r.logger.Warn("some notifications failed to create",
			zap.Int("failed_items", result.FailedItems),
			zap.Int("total_items", result.TotalItems),
		)
		// For notifications, we'll continue even with some failures
		// since they're not critical for app functionality
	}

	return nil
}

// DeleteExpiredNotifications deletes notifications that have expired
func (r *NotificationRepository) DeleteExpiredNotifications(_ context.Context, before time.Time) error {
	// Use DynamoDB TTL for automatic expiration
	// This method is for manual cleanup if needed

	var expiredNotifications []*models.Notification
	err := r.db.Model(&models.Notification{}).
		Filter("ExpiresAt", "<", before.Unix()).
		All(&expiredNotifications)
	if err != nil {
		return fmt.Errorf("failed to scan for expired notifications: %w", err)
	}

	if len(expiredNotifications) == 0 {
		return nil // Nothing to delete
	}

	// Use batch delete for efficient bulk deletion
	keys := make([]any, len(expiredNotifications))
	for i, notification := range expiredNotifications {
		// Create key structs with PK and SK for deletion
		keys[i] = &models.Notification{
			PK: notification.PK,
			SK: notification.SK,
		}
	}

	// Use DynamORM's batch delete functionality
	err = r.db.Model(&models.Notification{}).BatchDelete(keys)
	if err != nil {
		return fmt.Errorf("failed to batch delete expired notifications: %w", err)
	}

	r.logger.Info("batch deleted expired notifications",
		zap.Time("before", before),
		zap.Int("deleted_count", len(expiredNotifications)),
	)

	return nil
}

// NotificationFilters represents filters for notification queries
type NotificationFilters struct {
	Types        []string // Filter by notification types
	ExcludeTypes []string // Exclude specific notification types
	ActorID      string   // Filter by actor ID
	OnlyUnread   bool     // Only show unread notifications
	MinID        string   // Minimum notification ID (for pagination)
	MaxID        string   // Maximum notification ID (for pagination)
	SinceID      string   // Since notification ID (for pagination)
}

// DeliveryStats represents notification delivery statistics
type DeliveryStats struct {
	Total      int `json:"total"`
	Read       int `json:"read"`
	Unread     int `json:"unread"`
	PushSent   int `json:"push_sent"`
	PushFailed int `json:"push_failed"`
}

// GetNotificationsFiltered retrieves notifications with filtering options using legacy key patterns
func (r *NotificationRepository) GetNotificationsFiltered(ctx context.Context, username string, filter *storage.NotificationFilter) ([]*storage.Notification, string, error) {
	// Query all notifications for the user
	legacyNotifications, err := r.queryNotifications(ctx, username, r.normalizeLimit(filter.Limit))
	if err != nil {
		return nil, "", err
	}

	// Convert and filter notifications
	filtered := r.filterAndConvertNotifications(legacyNotifications, filter)

	// Apply final limit
	return r.applyLimit(filtered, filter.Limit), "", nil
}

// normalizeLimit ensures a reasonable limit value
func (r *NotificationRepository) normalizeLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	return limit
}

// queryNotifications queries legacy notifications from the database
func (r *NotificationRepository) queryNotifications(ctx context.Context, username string, limit int) ([]*models.NotificationLegacy, error) {
	pk := fmt.Sprintf("NOTIFICATIONS#%s", username)
	
	var notifications []*models.NotificationLegacy
	err := r.db.WithContext(ctx).Model(&models.NotificationLegacy{}).
		Where("PK", "=", pk).
		OrderBy("SK", "DESC"). // Newest first
		Limit(limit).
		All(&notifications)
	if err != nil {
		return nil, fmt.Errorf("failed to query notifications: %w", err)
	}
	
	return notifications, nil
}

// filterAndConvertNotifications applies filters and converts to storage format
func (r *NotificationRepository) filterAndConvertNotifications(legacyNotifications []*models.NotificationLegacy, filter *storage.NotificationFilter) []*storage.Notification {
	filtered := make([]*storage.Notification, 0)

	for _, record := range legacyNotifications {
		if !r.passesFilters(record, filter) {
			continue
		}

		notification := r.convertToStorageNotification(record)
		filtered = append(filtered, notification)
	}

	return filtered
}

// passesFilters checks if a notification passes all filters
func (r *NotificationRepository) passesFilters(record *models.NotificationLegacy, filter *storage.NotificationFilter) bool {
	// Check type filters
	if !r.passesTypeFilter(record.Type, filter.Types) {
		return false
	}

	// Check exclude type filters
	if r.isExcludedType(record.Type, filter.ExcludeTypes) {
		return false
	}

	// Check account filter
	if !r.passesAccountFilter(record.AccountID, filter.AccountID) {
		return false
	}

	// Check ID filters
	if !r.passesIDFilters(record.ID, filter) {
		return false
	}

	return true
}

// passesTypeFilter checks if notification type passes inclusion filter
func (r *NotificationRepository) passesTypeFilter(notificationType string, allowedTypes []string) bool {
	if len(allowedTypes) == 0 {
		return true
	}
	
	for _, t := range allowedTypes {
		if notificationType == t {
			return true
		}
	}
	return false
}

// isExcludedType checks if notification type is in exclusion list
func (r *NotificationRepository) isExcludedType(notificationType string, excludeTypes []string) bool {
	for _, t := range excludeTypes {
		if notificationType == t {
			return true
		}
	}
	return false
}

// passesAccountFilter checks if notification passes account filter
func (r *NotificationRepository) passesAccountFilter(notificationAccountID, filterAccountID string) bool {
	return filterAccountID == "" || notificationAccountID == filterAccountID
}

// passesIDFilters checks if notification passes ID-based filters
func (r *NotificationRepository) passesIDFilters(notificationID string, filter *storage.NotificationFilter) bool {
	if filter.MinID != "" && notificationID <= filter.MinID {
		return false
	}
	if filter.MaxID != "" && notificationID >= filter.MaxID {
		return false
	}
	if filter.SinceID != "" && notificationID <= filter.SinceID {
		return false
	}
	return true
}

// convertToStorageNotification converts legacy notification to storage format
func (r *NotificationRepository) convertToStorageNotification(record *models.NotificationLegacy) *storage.Notification {
	return &storage.Notification{
		ID:        record.ID,
		Type:      record.Type,
		Username:  record.Username,
		AccountID: record.AccountID,
		StatusID:  record.StatusID,
		Read:      record.Read,
		CreatedAt: time.Unix(record.CreatedAt, 0),
	}
}

// applyLimit applies the final limit to filtered results
func (r *NotificationRepository) applyLimit(notifications []*storage.Notification, limit int) []*storage.Notification {
	normalizedLimit := r.normalizeLimit(limit)
	if len(notifications) > normalizedLimit {
		return notifications[:normalizedLimit]
	}
	return notifications
}

// MarkAllNotificationsAsRead marks all notifications as read for a user using legacy key patterns
func (r *NotificationRepository) MarkAllNotificationsAsRead(ctx context.Context, username string) error {
	// Query all unread notifications
	pk := fmt.Sprintf("NOTIFICATIONS#%s", username)

	var notifications []*models.NotificationLegacy
	err := r.db.WithContext(ctx).Model(&models.NotificationLegacy{}).
		Where("PK", "=", pk).
		Filter("Read", "=", false).
		Limit(100). // Process in chunks
		All(&notifications)
	if err != nil {
		return fmt.Errorf("failed to query unread notifications: %w", err)
	}

	// Update each notification individually (matching legacy behavior)
	for _, notif := range notifications {
		notif.Read = true
		err := r.db.WithContext(ctx).Model(notif).Update()
		if err != nil {
			r.logger.Warn("failed to mark notification as read",
				zap.String("notification_id", notif.ID),
				zap.Error(err))
		}
	}

	return nil
}

// GetNotificationsAdvanced retrieves notifications with advanced filtering using legacy key patterns
func (r *NotificationRepository) GetNotificationsAdvanced(ctx context.Context, userID string, excludeTypes []string, maxID, sinceID, minID *string, limit int, _ bool) ([]*storage.Notification, error) {
	pk := fmt.Sprintf("NOTIFICATIONS#%s", userID)

	// Build query
	query := r.db.WithContext(ctx).Model(&models.NotificationLegacy{}).
		Where("PK", "=", pk).
		OrderBy("SK", "DESC") // Recent first

	// Get more items for filtering
	if limit <= 0 {
		limit = 20
	}
	query = query.Limit(limit * 2)

	var notifications []*models.NotificationLegacy
	err := query.All(&notifications)
	if err != nil {
		return nil, fmt.Errorf("failed to query notifications: %w", err)
	}

	// Convert and filter
	result := make([]*storage.Notification, 0)

	for _, record := range notifications {
		// Apply exclude types filter
		if len(excludeTypes) > 0 {
			excluded := false
			for _, excludeType := range excludeTypes {
				if record.Type == excludeType {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
		}

		// Apply ID filters
		if maxID != nil && record.SK >= *maxID {
			continue
		}
		if minID != nil && record.SK <= *minID {
			continue
		}
		if sinceID != nil && record.SK <= *sinceID {
			continue
		}

		// Convert to storage.Notification
		notification := &storage.Notification{
			ID:        record.ID,
			Type:      record.Type,
			Username:  record.Username,
			AccountID: record.AccountID,
			StatusID:  record.StatusID,
			Read:      record.Read,
			CreatedAt: time.Unix(record.CreatedAt, 0),
		}

		result = append(result, notification)

		if len(result) >= limit {
			break
		}
	}

	return result, nil
}

// GetUnreadNotificationCount returns the count of unread notifications using legacy key patterns
func (r *NotificationRepository) GetUnreadNotificationCount(ctx context.Context, userID string) (int64, error) {
	pk := fmt.Sprintf("NOTIFICATIONS#%s", userID)

	count, err := r.db.WithContext(ctx).Model(&models.NotificationLegacy{}).
		Where("PK", "=", pk).
		Filter("Read", "=", false).
		Count()
	if err != nil {
		return 0, fmt.Errorf("failed to count unread notifications: %w", err)
	}

	return count, nil
}

// GetNotificationPreferences retrieves notification preferences for a user
func (r *NotificationRepository) GetNotificationPreferences(ctx context.Context, username string) (*storage.NotificationPreferences, error) {
	var prefs models.NotificationPreferences
	prefs.Username = username
	prefs.UpdateKeys()

	err := r.db.WithContext(ctx).Model(&models.NotificationPreferences{}).
		Where("PK", "=", prefs.PK).
		Where("SK", "=", prefs.SK).
		First(&prefs)

	if err != nil {
		if errors.IsNotFound(err) {
			// Return nil if not found (not an error)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get notification preferences: %w", err)
	}

	// Convert to storage.NotificationPreferences
	result := &storage.NotificationPreferences{
		Username:        prefs.Username,
		EmailEnabled:    prefs.EmailEnabled,
		PushEnabled:     prefs.PushEnabled,
		FollowEnabled:   prefs.FollowEnabled,
		MentionEnabled:  prefs.MentionEnabled,
		ReblogEnabled:   prefs.ReblogEnabled,
		FavoriteEnabled: prefs.FavoriteEnabled,
		PollEnabled:     prefs.PollEnabled,
		UpdatedAt:       prefs.UpdatedAt,
	}

	return result, nil
}

// UpdateNotificationPreferences creates or updates notification preferences for a user
func (r *NotificationRepository) UpdateNotificationPreferences(ctx context.Context, username string, prefs *storage.NotificationPreferences) error {
	// Create model
	modelPrefs := &models.NotificationPreferences{
		Username:        username,
		EmailEnabled:    prefs.EmailEnabled,
		PushEnabled:     prefs.PushEnabled,
		FollowEnabled:   prefs.FollowEnabled,
		MentionEnabled:  prefs.MentionEnabled,
		ReblogEnabled:   prefs.ReblogEnabled,
		FavoriteEnabled: prefs.FavoriteEnabled,
		PollEnabled:     prefs.PollEnabled,
	}

	// Set keys and timestamp
	if err := modelPrefs.BeforeUpdate(); err != nil {
		return fmt.Errorf("failed to prepare preferences for update: %w", err)
	}

	// Try to update existing record first
	err := r.db.WithContext(ctx).Model(modelPrefs).Update()

	if err != nil {
		// If not found, create new record
		if errors.IsNotFound(err) {
			err = r.db.WithContext(ctx).Model(modelPrefs).Create()
			if err != nil {
				return fmt.Errorf("failed to create notification preferences: %w", err)
			}
		} else {
			return fmt.Errorf("failed to update notification preferences: %w", err)
		}
	}

	// Update the passed in preferences with the new timestamp
	prefs.UpdatedAt = modelPrefs.UpdatedAt

	return nil
}

// BatchMarkNotificationsAsRead marks multiple notifications as read
func (r *NotificationRepository) BatchMarkNotificationsAsRead(ctx context.Context, username string, notificationIDs []string) error {
	if len(notificationIDs) == 0 {
		return nil
	}

	// For each notification ID, we need to find the full key (with timestamp)
	pk := fmt.Sprintf("NOTIFICATIONS#%s", username)

	// First, query all user's notifications to find the ones we need to update
	var allNotifications []*models.NotificationLegacy
	err := r.db.WithContext(ctx).Model(&models.NotificationLegacy{}).
		Where("PK", "=", pk).
		All(&allNotifications)
	if err != nil {
		return fmt.Errorf("failed to query notifications: %w", err)
	}

	// Create a map for quick lookup
	idMap := make(map[string]bool)
	for _, id := range notificationIDs {
		idMap[id] = true
	}

	// Update each matching notification
	updatedCount := 0
	for _, notif := range allNotifications {
		if idMap[notif.ID] && !notif.Read {
			notif.Read = true
			err := r.db.WithContext(ctx).Model(notif).Update()
			if err != nil {
				r.logger.Warn("failed to mark notification as read in batch",
					zap.String("notification_id", notif.ID),
					zap.Error(err))
			} else {
				updatedCount++
			}
		}
	}

	r.logger.Info("batch marked notifications as read",
		zap.String("username", username),
		zap.Int("requested", len(notificationIDs)),
		zap.Int("updated", updatedCount))

	return nil
}

// SetNotificationPreference sets a specific notification preference
func (r *NotificationRepository) SetNotificationPreference(ctx context.Context, username string, preference string, enabled bool) error {
	// Get existing preferences or create new ones
	prefs, err := r.GetNotificationPreferences(ctx, username)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if prefs == nil {
		prefs = &storage.NotificationPreferences{
			Username: username,
		}
	}

	// Update specific preference
	switch preference {
	case "email":
		prefs.EmailEnabled = enabled
	case "push":
		prefs.PushEnabled = enabled
	case "follow":
		prefs.FollowEnabled = enabled
	case "mention":
		prefs.MentionEnabled = enabled
	case "reblog":
		prefs.ReblogEnabled = enabled
	case "favorite":
		prefs.FavoriteEnabled = enabled
	case "poll":
		prefs.PollEnabled = enabled
	default:
		return fmt.Errorf("unknown preference: %s", preference)
	}

	return r.UpdateNotificationPreferences(ctx, username, prefs)
}

// RecordDeliveryAttempt records a notification delivery attempt
func (r *NotificationRepository) RecordDeliveryAttempt(ctx context.Context, notificationID, method string, success bool, errorMsg string) error {
	delivery := models.NewNotificationDelivery(notificationID, method)

	if success {
		delivery.MarkSent()
	} else {
		delivery.MarkFailed(errorMsg)
	}

	if err := delivery.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare delivery record: %w", err)
	}

	err := r.db.WithContext(ctx).Model(delivery).Create()
	if err != nil {
		return fmt.Errorf("failed to record delivery attempt: %w", err)
	}

	return nil
}

// GetDeliveryStatus gets the delivery status for a notification
func (r *NotificationRepository) GetDeliveryStatus(ctx context.Context, notificationID, method string) (*models.NotificationDelivery, error) {
	var delivery models.NotificationDelivery
	delivery.NotificationID = notificationID
	delivery.DeliveryMethod = method
	delivery.UpdateKeys()

	err := r.db.WithContext(ctx).Model(&models.NotificationDelivery{}).
		Where("PK", "=", delivery.PK).
		Where("SK", "=", delivery.SK).
		First(&delivery)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get delivery status: %w", err)
	}

	return &delivery, nil
}

// MarkDeliveryComplete marks a delivery as complete
func (r *NotificationRepository) MarkDeliveryComplete(ctx context.Context, notificationID, method string) error {
	delivery, err := r.GetDeliveryStatus(ctx, notificationID, method)
	if err != nil {
		return err
	}

	if delivery == nil {
		// Create new delivery record
		delivery = models.NewNotificationDelivery(notificationID, method)
	}

	delivery.MarkSent()

	if err := delivery.BeforeUpdate(); err != nil {
		return fmt.Errorf("failed to prepare delivery update: %w", err)
	}

	err = r.db.WithContext(ctx).Model(delivery).Update()
	if err != nil {
		// If not found, create it
		if errors.IsNotFound(err) {
			if err := delivery.BeforeCreate(); err != nil {
				return fmt.Errorf("failed to prepare delivery record: %w", err)
			}
			err = r.db.WithContext(ctx).Model(delivery).Create()
		}
		if err != nil {
			return fmt.Errorf("failed to mark delivery complete: %w", err)
		}
	}

	return nil
}

// GetFailedDeliveries gets failed delivery attempts
func (r *NotificationRepository) GetFailedDeliveries(ctx context.Context, since time.Time, limit int) ([]*models.NotificationDelivery, error) {
	// This would benefit from a GSI on status in production
	var deliveries []*models.NotificationDelivery

	// For now, we'll scan with filters (inefficient for large datasets)
	err := r.db.WithContext(ctx).Model(&models.NotificationDelivery{}).
		Filter("Status", "=", "failed").
		Filter("LastAttempt", ">=", since.Unix()).
		Limit(limit).
		All(&deliveries)

	if err != nil {
		return nil, fmt.Errorf("failed to get failed deliveries: %w", err)
	}

	return deliveries, nil
}

// RetryFailedDeliveries retries failed delivery attempts
func (r *NotificationRepository) RetryFailedDeliveries(ctx context.Context, before time.Time) error {
	deliveries, err := r.GetFailedDeliveries(ctx, before, 100)
	if err != nil {
		return err
	}

	for _, delivery := range deliveries {
		if delivery.CanRetry() {
			delivery.MarkPending()
			delivery.IncrementAttempt()

			if err := delivery.BeforeUpdate(); err != nil {
				r.logger.Warn("failed to prepare delivery retry",
					zap.String("notification_id", delivery.NotificationID),
					zap.String("method", delivery.DeliveryMethod),
					zap.Error(err))
				continue
			}

			err := r.db.WithContext(ctx).Model(delivery).Update()
			if err != nil {
				r.logger.Warn("failed to retry delivery",
					zap.String("notification_id", delivery.NotificationID),
					zap.String("method", delivery.DeliveryMethod),
					zap.Error(err))
			}
		}
	}

	return nil
}

// CreatePushSubscription creates a new push subscription
func (r *NotificationRepository) CreatePushSubscription(ctx context.Context, username string, subscription *storage.PushSubscription) error {
	model := &models.PushSubscription{
		ID:        subscription.ID,
		Username:  username,
		Endpoint:  subscription.Endpoint,
		P256dh:    subscription.P256dh,
		Auth:      subscription.Auth,
		Alerts:    convertStorageToPushAlerts(subscription.Alerts),
		Policy:    subscription.Policy,
		CreatedAt: subscription.CreatedAt,
		UpdatedAt: subscription.UpdatedAt,
	}

	if err := model.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare push subscription: %w", err)
	}

	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		return fmt.Errorf("failed to create push subscription: %w", err)
	}

	// Update the storage object with generated ID
	subscription.ID = model.ID

	return nil
}

// GetPushSubscriptions retrieves all push subscriptions for a user
func (r *NotificationRepository) GetPushSubscriptions(ctx context.Context, username string) ([]*storage.PushSubscription, error) {
	pk := fmt.Sprintf("PUSH#%s", username)

	var subscriptions []*models.PushSubscription
	err := r.db.WithContext(ctx).Model(&models.PushSubscription{}).
		Where("PK", "=", pk).
		All(&subscriptions)

	if err != nil {
		return nil, fmt.Errorf("failed to get push subscriptions: %w", err)
	}

	// Convert to storage types
	result := make([]*storage.PushSubscription, len(subscriptions))
	for i, sub := range subscriptions {
		result[i] = &storage.PushSubscription{
			ID:        sub.ID,
			Username:  sub.Username,
			Endpoint:  sub.Endpoint,
			P256dh:    sub.P256dh,
			Auth:      sub.Auth,
			Alerts:    convertPushToStorageAlerts(sub.Alerts),
			Policy:    sub.Policy,
			CreatedAt: sub.CreatedAt,
			UpdatedAt: sub.UpdatedAt,
		}
	}

	return result, nil
}

// UpdatePushSubscription updates a push subscription's alerts
func (r *NotificationRepository) UpdatePushSubscription(ctx context.Context, username, subscriptionID string, alerts storage.PushSubscriptionAlerts) error {
	// Get existing subscription
	var sub models.PushSubscription
	sub.Username = username
	sub.ID = subscriptionID
	sub.UpdateKeys()

	err := r.db.WithContext(ctx).Model(&models.PushSubscription{}).
		Where("PK", "=", sub.PK).
		Where("SK", "=", sub.SK).
		First(&sub)

	if err != nil {
		return fmt.Errorf("failed to get push subscription: %w", err)
	}

	// Update alerts
	sub.Alerts = convertStorageToPushAlerts(alerts)

	if err := sub.BeforeUpdate(); err != nil {
		return fmt.Errorf("failed to prepare push subscription update: %w", err)
	}

	err = r.db.WithContext(ctx).Model(&sub).Update()
	if err != nil {
		return fmt.Errorf("failed to update push subscription: %w", err)
	}

	return nil
}

// DeletePushSubscription deletes a push subscription
func (r *NotificationRepository) DeletePushSubscription(ctx context.Context, username, subscriptionID string) error {
	var sub models.PushSubscription
	sub.Username = username
	sub.ID = subscriptionID
	sub.UpdateKeys()

	err := r.db.WithContext(ctx).Model(&sub).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete push subscription: %w", err)
	}

	return nil
}

// DeleteExpiredSubscriptions deletes expired push subscriptions
func (r *NotificationRepository) DeleteExpiredSubscriptions(ctx context.Context, before time.Time) error {
	// This would benefit from a GSI on LastUsed in production
	// For now, we'll scan and delete old subscriptions

	// Query subscriptions not used in 90 days
	cutoff := before.Add(-90 * 24 * time.Hour)

	var oldSubscriptions []*models.PushSubscription
	err := r.db.WithContext(ctx).Model(&models.PushSubscription{}).
		Filter("LastUsed", "<", cutoff.Unix()).
		Limit(100).
		All(&oldSubscriptions)

	if err != nil {
		return fmt.Errorf("failed to find expired subscriptions: %w", err)
	}

	if len(oldSubscriptions) == 0 {
		return nil // Nothing to delete
	}

	// Use batch delete for efficient bulk deletion
	keys := make([]any, len(oldSubscriptions))
	for i, sub := range oldSubscriptions {
		// Create key structs with PK and SK for deletion
		keys[i] = &models.PushSubscription{
			PK: sub.PK,
			SK: sub.SK,
		}
	}

	// Use DynamORM's batch delete functionality
	err = r.db.WithContext(ctx).Model(&models.PushSubscription{}).BatchDelete(keys)
	if err != nil {
		return fmt.Errorf("failed to batch delete expired subscriptions: %w", err)
	}

	r.logger.Info("batch deleted expired push subscriptions",
		zap.Time("cutoff", cutoff),
		zap.Int("deleted_count", len(oldSubscriptions)),
	)

	return nil
}

// UpdateLastUsed updates the last used timestamp for a push subscription
func (r *NotificationRepository) UpdateLastUsed(ctx context.Context, username, subscriptionID string) error {
	var sub models.PushSubscription
	sub.Username = username
	sub.ID = subscriptionID
	sub.UpdateKeys()

	err := r.db.WithContext(ctx).Model(&models.PushSubscription{}).
		Where("PK", "=", sub.PK).
		Where("SK", "=", sub.SK).
		First(&sub)

	if err != nil {
		return fmt.Errorf("failed to get push subscription: %w", err)
	}

	sub.UpdateLastUsed()

	if err := sub.BeforeUpdate(); err != nil {
		return fmt.Errorf("failed to prepare last used update: %w", err)
	}

	err = r.db.WithContext(ctx).Model(&sub).Update()
	if err != nil {
		return fmt.Errorf("failed to update last used: %w", err)
	}

	return nil
}

// GetNotificationStats gets notification statistics for a user
func (r *NotificationRepository) GetNotificationStats(ctx context.Context, username string) (map[string]int64, error) {
	stats := make(map[string]int64)

	// Get total count
	pk := fmt.Sprintf("NOTIFICATIONS#%s", username)
	total, err := r.db.WithContext(ctx).Model(&models.NotificationLegacy{}).
		Where("PK", "=", pk).
		Count()
	if err != nil {
		return nil, fmt.Errorf("failed to count total notifications: %w", err)
	}
	stats["total"] = total

	// Get unread count
	unread, err := r.db.WithContext(ctx).Model(&models.NotificationLegacy{}).
		Where("PK", "=", pk).
		Filter("Read", "=", false).
		Count()
	if err != nil {
		return nil, fmt.Errorf("failed to count unread notifications: %w", err)
	}
	stats["unread"] = unread
	stats["read"] = total - unread

	return stats, nil
}

// ClearOldNotifications deletes notifications older than the specified duration
func (r *NotificationRepository) ClearOldNotifications(ctx context.Context, username string, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan).Unix()
	pk := fmt.Sprintf("NOTIFICATIONS#%s", username)

	// Get old notifications
	var oldNotifications []*models.NotificationLegacy
	err := r.db.WithContext(ctx).Model(&models.NotificationLegacy{}).
		Where("PK", "=", pk).
		Filter("CreatedAt", "<", cutoff).
		Limit(100).
		All(&oldNotifications)

	if err != nil {
		return fmt.Errorf("failed to find old notifications: %w", err)
	}

	if len(oldNotifications) == 0 {
		return nil // Nothing to delete
	}

	// Use batch delete for efficient bulk deletion
	keys := make([]any, len(oldNotifications))
	for i, notif := range oldNotifications {
		// Create key structs with PK and SK for deletion
		keys[i] = &models.NotificationLegacy{
			PK: notif.PK,
			SK: notif.SK,
		}
	}

	// Use DynamORM's batch delete functionality
	err = r.db.WithContext(ctx).Model(&models.NotificationLegacy{}).BatchDelete(keys)
	if err != nil {
		return fmt.Errorf("failed to batch delete old notifications: %w", err)
	}

	r.logger.Info("batch deleted old notifications",
		zap.String("username", username),
		zap.Duration("older_than", olderThan),
		zap.Int("deleted_count", len(oldNotifications)),
	)

	return nil
}

// convertStorageToPushAlerts converts storage.PushSubscriptionAlerts to models.PushSubscriptionAlerts
func convertStorageToPushAlerts(alerts storage.PushSubscriptionAlerts) models.PushSubscriptionAlerts {
	return models.PushSubscriptionAlerts{
		Follow:        alerts.Follow,
		Favourite:     alerts.Favourite,
		Reblog:        alerts.Reblog,
		Mention:       alerts.Mention,
		Poll:          alerts.Poll,
		FollowRequest: alerts.FollowRequest,
		Status:        alerts.Status,
		Update:        alerts.Update,
		AdminSignUp:   alerts.AdminSignUp,
		AdminReport:   alerts.AdminReport,
	}
}

// convertPushToStorageAlerts converts models.PushSubscriptionAlerts to storage.PushSubscriptionAlerts
func convertPushToStorageAlerts(alerts models.PushSubscriptionAlerts) storage.PushSubscriptionAlerts {
	return storage.PushSubscriptionAlerts{
		Follow:        alerts.Follow,
		Favourite:     alerts.Favourite,
		Reblog:        alerts.Reblog,
		Mention:       alerts.Mention,
		Poll:          alerts.Poll,
		FollowRequest: alerts.FollowRequest,
		Status:        alerts.Status,
		Update:        alerts.Update,
		AdminSignUp:   alerts.AdminSignUp,
		AdminReport:   alerts.AdminReport,
	}
}
