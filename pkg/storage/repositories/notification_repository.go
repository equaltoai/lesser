package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/batch"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
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
func (r *NotificationRepository) CreateNotification(ctx context.Context, notification *models.Notification) error {
	if err := notification.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare notification for creation: %w", err)
	}

	err := r.db.WithContext(ctx).Model(notification).Create()
	if err != nil {
		return fmt.Errorf("failed to create notification: %w", err)
	}

	return nil
}

// GetNotification retrieves a notification by ID
func (r *NotificationRepository) GetNotification(ctx context.Context, notificationID string) (*models.Notification, error) {
	var notification models.Notification
	err := r.db.WithContext(ctx).Model(&models.Notification{}).
		Filter("ID", "=", notificationID).
		First(&notification)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get notification: %w", err)
	}

	return &notification, nil
}

// UpdateNotification updates an existing notification
func (r *NotificationRepository) UpdateNotification(ctx context.Context, notification *models.Notification) error {
	if err := notification.BeforeUpdate(); err != nil {
		return fmt.Errorf("failed to prepare notification for update: %w", err)
	}

	err := r.db.WithContext(ctx).Model(notification).Update()
	if err != nil {
		return fmt.Errorf("failed to update notification: %w", err)
	}

	return nil
}

// DeleteNotification deletes a notification
func (r *NotificationRepository) DeleteNotification(ctx context.Context, notificationID string) error {
	notification, err := r.GetNotification(ctx, notificationID)
	if err != nil {
		return err
	}
	if notification == nil {
		return fmt.Errorf("notification not found: %s", notificationID)
	}

	err = r.db.WithContext(ctx).Model(notification).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete notification: %w", err)
	}

	return nil
}

// GetUserNotifications retrieves notifications for a user with pagination
func (r *NotificationRepository) GetUserNotifications(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}
	pk := "USER#" + userID
	query := r.db.WithContext(ctx).Model(&models.Notification{}).
		Where("PK", "=", pk).
		OrderBy("SK", "DESC") // Most recent first

	// Handle cursor-based pagination
	if opts.Cursor != "" {
		query = query.Where("SK", "<", opts.Cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(opts.Limit + 1)

	var notifications []models.Notification
	err := query.All(&notifications)
	if err != nil {
		return nil, fmt.Errorf("failed to get notifications for user: %w", err)
	}

	// Build result
	result := &interfaces.PaginatedResult[*models.Notification]{
		Items: make([]*models.Notification, 0, len(notifications)),
		Total: -1, // Not calculated
	}

	// Generate next cursor
	if len(notifications) > opts.Limit {
		// We got more results than requested, so there are more pages
		result.NextCursor = notifications[opts.Limit-1].SK
		result.HasMore = true
		notifications = notifications[:opts.Limit] // Trim to requested limit
	}

	// Convert to pointers
	for i := range notifications {
		result.Items = append(result.Items, &notifications[i])
	}

	return result, nil
}

// GetUnreadNotifications retrieves unread notifications for a user with pagination
func (r *NotificationRepository) GetUnreadNotifications(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}
	pk := "USER#" + userID

	query := r.db.WithContext(ctx).Model(&models.Notification{}).
		Where("PK", "=", pk).
		Filter("IsRead", "=", false).
		OrderBy("SK", "DESC")

	// Handle cursor-based pagination
	if opts.Cursor != "" {
		query = query.Where("SK", "<", opts.Cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(opts.Limit + 1)

	var notifications []models.Notification
	err := query.All(&notifications)
	if err != nil {
		return nil, fmt.Errorf("failed to get unread notifications for user: %w", err)
	}

	// Build result
	result := &interfaces.PaginatedResult[*models.Notification]{
		Items: make([]*models.Notification, 0, len(notifications)),
		Total: -1, // Not calculated
	}

	// Generate next cursor
	if len(notifications) > opts.Limit {
		// We got more results than requested, so there are more pages
		result.NextCursor = notifications[opts.Limit-1].SK
		result.HasMore = true
		notifications = notifications[:opts.Limit] // Trim to requested limit
	}

	// Convert to pointers
	for i := range notifications {
		result.Items = append(result.Items, &notifications[i])
	}

	return result, nil
}

// GetNotificationsByType retrieves notifications by type with pagination
func (r *NotificationRepository) GetNotificationsByType(ctx context.Context, userID, notificationType string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}

	// Filter notifications by type for specific user
	pk := "USER#" + userID
	query := r.db.WithContext(ctx).Model(&models.Notification{}).
		Where("PK", "=", pk).
		Filter("Type", "=", notificationType).
		OrderBy("SK", "DESC")

	if opts.Cursor != "" {
		query = query.Where("SK", "<", opts.Cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(opts.Limit + 1)

	var notifications []models.Notification
	err := query.All(&notifications)
	if err != nil {
		return nil, fmt.Errorf("failed to get notifications by type: %w", err)
	}

	// Build result
	result := &interfaces.PaginatedResult[*models.Notification]{
		Items: make([]*models.Notification, 0, len(notifications)),
		Total: -1, // Not calculated
	}

	// Generate next cursor
	if len(notifications) > opts.Limit {
		// We got more results than requested, so there are more pages
		result.NextCursor = notifications[opts.Limit-1].SK
		result.HasMore = true
		notifications = notifications[:opts.Limit] // Trim to requested limit
	}

	// Convert to pointers
	for i := range notifications {
		result.Items = append(result.Items, &notifications[i])
	}

	return result, nil
}

// MarkNotificationRead marks a notification as read
func (r *NotificationRepository) MarkNotificationRead(ctx context.Context, notificationID string) error {
	notification, err := r.GetNotification(ctx, notificationID)
	if err != nil {
		return err
	}
	if notification == nil {
		return fmt.Errorf("notification not found: %s", notificationID)
	}

	notification.MarkRead()
	return r.UpdateNotification(ctx, notification)
}

// MarkNotificationUnread marks a notification as unread
func (r *NotificationRepository) MarkNotificationUnread(ctx context.Context, notificationID string) error {
	notification, err := r.GetNotification(ctx, notificationID)
	if err != nil {
		return err
	}
	if notification == nil {
		return fmt.Errorf("notification not found: %s", notificationID)
	}

	notification.MarkUnread()
	return r.UpdateNotification(ctx, notification)
}

// MarkAllNotificationsRead marks all notifications as read for a user
func (r *NotificationRepository) MarkAllNotificationsRead(ctx context.Context, userID string) error {
	// Query all unread notifications
	pk := "USER#" + userID

	var notifications []models.Notification
	err := r.db.WithContext(ctx).Model(&models.Notification{}).
		Where("PK", "=", pk).
		Filter("IsRead", "=", false).
		Limit(1000). // Process in chunks
		All(&notifications)
	if err != nil {
		return fmt.Errorf("failed to query unread notifications: %w", err)
	}

	// Update each notification individually
	for i := range notifications {
		notifications[i].MarkRead()
		err := r.db.WithContext(ctx).Model(&notifications[i]).Update()
		if err != nil {
			r.logger.Warn("failed to mark notification as read",
				zap.String("notification_id", notifications[i].ID),
				zap.Error(err))
		}
	}

	return nil
}

// MarkNotificationsReadByType marks notifications as read by type for a user
func (r *NotificationRepository) MarkNotificationsReadByType(ctx context.Context, userID, notificationType string) error {
	// Query all unread notifications of the specified type
	pk := "USER#" + userID

	var notifications []models.Notification
	err := r.db.WithContext(ctx).Model(&models.Notification{}).
		Where("PK", "=", pk).
		Filter("IsRead", "=", false).
		Filter("Type", "=", notificationType).
		Limit(1000). // Process in chunks
		All(&notifications)
	if err != nil {
		return fmt.Errorf("failed to query unread notifications by type: %w", err)
	}

	// Update each notification individually
	for i := range notifications {
		notifications[i].MarkRead()
		err := r.db.WithContext(ctx).Model(&notifications[i]).Update()
		if err != nil {
			r.logger.Warn("failed to mark notification as read by type",
				zap.String("notification_id", notifications[i].ID),
				zap.String("type", notificationType),
				zap.Error(err))
		}
	}

	return nil
}

// MarkNotificationPushSent marks a notification's push as sent
func (r *NotificationRepository) MarkNotificationPushSent(ctx context.Context, notificationID string) error {
	notification, err := r.GetNotification(ctx, notificationID)
	if err != nil {
		return err
	}
	if notification == nil {
		return fmt.Errorf("notification not found: %s", notificationID)
	}

	notification.MarkPushSent()
	return r.UpdateNotification(ctx, notification)
}

// MarkNotificationPushFailed marks a notification's push as failed
func (r *NotificationRepository) MarkNotificationPushFailed(ctx context.Context, notificationID, errorMsg string) error {
	notification, err := r.GetNotification(ctx, notificationID)
	if err != nil {
		return err
	}
	if notification == nil {
		return fmt.Errorf("notification not found: %s", notificationID)
	}

	notification.MarkPushFailed(errorMsg)
	return r.UpdateNotification(ctx, notification)
}

// GetPendingPushNotifications retrieves notifications that need push delivery
func (r *NotificationRepository) GetPendingPushNotifications(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	if opts.Limit > 1000 {
		opts.Limit = 1000
	}

	var notifications []models.Notification
	query := r.db.WithContext(ctx).Model(&models.Notification{}).
		Filter("PushSent", "=", false).
		Filter("PushError", "=", "").
		OrderBy("CreatedAt", "ASC")

	if opts.Cursor != "" {
		query = query.Where("CreatedAt", ">", opts.Cursor)
	}

	query = query.Limit(opts.Limit + 1)

	err := query.All(&notifications)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending push notifications: %w", err)
	}

	// Build result
	result := &interfaces.PaginatedResult[*models.Notification]{
		Items: make([]*models.Notification, 0, len(notifications)),
		Total: -1, // Not calculated
	}

	// Generate next cursor
	if len(notifications) > opts.Limit {
		// We got more results than requested, so there are more pages
		result.NextCursor = fmt.Sprintf("%d", notifications[opts.Limit-1].CreatedAt.Unix())
		result.HasMore = true
		notifications = notifications[:opts.Limit] // Trim to requested limit
	}

	// Convert to pointers
	for i := range notifications {
		result.Items = append(result.Items, &notifications[i])
	}

	return result, nil
}

// GetNotificationGroups retrieves notification groups for a user with pagination
func (r *NotificationRepository) GetNotificationGroups(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}

	// Use GSI3 to efficiently query grouped notifications
	// We'll get all groups and then filter for the user
	query := r.db.WithContext(ctx).Model(&models.Notification{}).
		Index("group-index")

	// Handle cursor-based pagination on GSI3SK
	if opts.Cursor != "" {
		query = query.Where("GSI3SK", "<", opts.Cursor)
	}

	// Order by GSI3SK (timestamp#id) to get chronological groups
	query = query.OrderBy("GSI3SK", "DESC").
		Limit(opts.Limit + 1) // Get one extra to check for more results

	var allNotifications []models.Notification
	err := query.All(&allNotifications)
	if err != nil {
		return nil, fmt.Errorf("failed to get notification groups: %w", err)
	}

	// Filter notifications for the specific user and group them
	userGroups := make(map[string]*models.Notification)
	groupKeys := make([]string, 0)
	
	for i := range allNotifications {
		notif := &allNotifications[i]
		if notif.UserID != userID {
			continue // Skip notifications for other users
		}
		
		// Keep only the most recent notification per group
		if existing, exists := userGroups[notif.GroupKey]; !exists || notif.CreatedAt.After(existing.CreatedAt) {
			if !exists {
				groupKeys = append(groupKeys, notif.GroupKey)
			}
			userGroups[notif.GroupKey] = notif
		}
	}

	// Build result
	result := &interfaces.PaginatedResult[*models.Notification]{
		Items: make([]*models.Notification, 0, len(userGroups)),
		Total: -1, // Not calculated for grouped results
	}

	// Add grouped notifications in order
	actualCount := 0
	for _, groupKey := range groupKeys {
		if actualCount >= opts.Limit {
			// We have more results
			result.HasMore = true
			break
		}
		if notif, exists := userGroups[groupKey]; exists {
			result.Items = append(result.Items, notif)
			actualCount++
		}
	}

	// Set next cursor if we have more results
	if result.HasMore && len(result.Items) > 0 {
		lastNotif := result.Items[len(result.Items)-1]
		result.NextCursor = lastNotif.GSI3SK
	}

	r.logger.Debug("retrieved notification groups using GSI3",
		zap.String("user_id", userID),
		zap.Int("total_groups", len(userGroups)),
		zap.Int("returned_count", len(result.Items)),
		zap.Bool("has_more", result.HasMore),
	)

	return result, nil
}

// ConsolidateNotifications consolidates notifications by group key
func (r *NotificationRepository) ConsolidateNotifications(ctx context.Context, groupKey string) error {
	// Query notifications with the same group key using GSI3
	var notifications []models.Notification
	err := r.db.WithContext(ctx).Model(&models.Notification{}).
		Index("group-index").
		Where("GSI3PK", "=", "NOTIF_GROUP#"+groupKey).
		All(&notifications)
	if err != nil {
		return fmt.Errorf("failed to get notifications for consolidation: %w", err)
	}

	if len(notifications) <= 1 {
		return nil // Nothing to consolidate
	}

	// Keep the most recent notification and update its group count
	// Delete the older notifications in the group
	// This is a simple consolidation strategy
	mostRecentIndex := 0
	for i := 1; i < len(notifications); i++ {
		if notifications[i].CreatedAt.After(notifications[mostRecentIndex].CreatedAt) {
			mostRecentIndex = i
		}
	}

	// Update the group count on the most recent notification
	notifications[mostRecentIndex].GroupCount = len(notifications)
	err = r.db.WithContext(ctx).Model(&notifications[mostRecentIndex]).Update()
	if err != nil {
		r.logger.Warn("failed to update group count on consolidated notification",
			zap.String("notification_id", notifications[mostRecentIndex].ID),
			zap.Error(err))
	}

	// Delete other notifications in the group
	for i := 0; i < len(notifications); i++ {
		if i != mostRecentIndex {
			err := r.db.WithContext(ctx).Model(&notifications[i]).Delete()
			if err != nil {
				r.logger.Warn("failed to delete consolidated notification",
					zap.String("notification_id", notifications[i].ID),
					zap.Error(err))
			}
		}
	}

	return nil
}

// GetUnreadNotificationCount returns the count of unread notifications
func (r *NotificationRepository) GetUnreadNotificationCount(ctx context.Context, userID string) (int64, error) {
	pk := "USER#" + userID

	count, err := r.db.WithContext(ctx).Model(&models.Notification{}).
		Where("PK", "=", pk).
		Filter("IsRead", "=", false).
		Count()
	if err != nil {
		return 0, fmt.Errorf("failed to count unread notifications: %w", err)
	}

	return count, nil
}

// GetNotificationCountsByType returns notification counts by type
func (r *NotificationRepository) GetNotificationCountsByType(ctx context.Context, userID string) (map[string]int64, error) {
	pk := "USER#" + userID

	// Get all notifications for the user and count by type
	var notifications []models.Notification
	err := r.db.WithContext(ctx).Model(&models.Notification{}).
		Where("PK", "=", pk).
		All(&notifications)
	if err != nil {
		return nil, fmt.Errorf("failed to get notifications for counting: %w", err)
	}

	// Count by type
	counts := make(map[string]int64)
	for _, notif := range notifications {
		counts[notif.Type]++
	}

	return counts, nil
}

// CreateNotifications creates multiple notifications efficiently
func (r *NotificationRepository) CreateNotifications(ctx context.Context, notifications []*models.Notification) error {
	if err := common.ValidateSliceNotEmpty("notifications", notifications); err != nil {
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

// DeleteNotificationsByType deletes notifications by type for a user
func (r *NotificationRepository) DeleteNotificationsByType(ctx context.Context, userID, notificationType string) error {
	pk := "USER#" + userID

	// Get notifications of the specified type
	var notifications []models.Notification
	err := r.db.WithContext(ctx).Model(&models.Notification{}).
		Where("PK", "=", pk).
		Filter("Type", "=", notificationType).
		Limit(1000). // Process in chunks
		All(&notifications)
	if err != nil {
		return fmt.Errorf("failed to query notifications by type: %w", err)
	}

	if err := common.ValidateSliceNotEmpty("notifications", notifications); err != nil {
		return nil // Nothing to delete
	}

	// Use batch delete for efficient bulk deletion
	keys := make([]any, len(notifications))
	for i := range notifications {
		// Create key structs with PK and SK for deletion
		keys[i] = &models.Notification{
			PK: notifications[i].PK,
			SK: notifications[i].SK,
		}
	}

	// Use DynamORM's batch delete functionality
	err = r.db.WithContext(ctx).Model(&models.Notification{}).BatchDelete(keys)
	if err != nil {
		return fmt.Errorf("failed to batch delete notifications by type: %w", err)
	}

	r.logger.Info("batch deleted notifications by type",
		zap.String("user_id", userID),
		zap.String("type", notificationType),
		zap.Int("deleted_count", len(notifications)),
	)

	return nil
}

// DeleteNotificationsByObject deletes all notifications related to a specific object
func (r *NotificationRepository) DeleteNotificationsByObject(ctx context.Context, objectID string) error {
	// Query all notifications for this object
	var notifications []models.Notification
	err := r.db.WithContext(ctx).Model(&models.Notification{}).
		Where("ObjectID", "=", objectID).
		All(&notifications)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil // No notifications to delete
		}
		return fmt.Errorf("failed to query notifications for object: %w", err)
	}

	// Delete each notification
	for _, notification := range notifications {
		if err := r.db.WithContext(ctx).Model(&notification).Delete(); err != nil {
			r.logger.Warn("failed to delete notification",
				zap.String("notification_id", notification.ID),
				zap.Error(err))
		}
	}

	return nil
}

// DeleteExpiredNotifications deletes notifications that have expired
func (r *NotificationRepository) DeleteExpiredNotifications(ctx context.Context, expiredBefore time.Time) (int64, error) {
	// Use DynamoDB TTL for automatic expiration
	// This method is for manual cleanup if needed

	var expiredNotifications []models.Notification
	err := r.db.WithContext(ctx).Model(&models.Notification{}).
		Filter("ExpiresAt", "<", expiredBefore.Unix()).
		Limit(1000).
		All(&expiredNotifications)
	if err != nil {
		return 0, fmt.Errorf("failed to scan for expired notifications: %w", err)
	}

	if err := common.ValidateSliceNotEmpty("expiredNotifications", expiredNotifications); err != nil {
		return 0, nil // Nothing to delete
	}

	// Use batch delete for efficient bulk deletion
	keys := make([]any, len(expiredNotifications))
	for i := range expiredNotifications {
		// Create key structs with PK and SK for deletion
		keys[i] = &models.Notification{
			PK: expiredNotifications[i].PK,
			SK: expiredNotifications[i].SK,
		}
	}

	// Use DynamORM's batch delete functionality
	err = r.db.WithContext(ctx).Model(&models.Notification{}).BatchDelete(keys)
	if err != nil {
		return 0, fmt.Errorf("failed to batch delete expired notifications: %w", err)
	}

	r.logger.Info("batch deleted expired notifications",
		zap.Time("expired_before", expiredBefore),
		zap.Int("deleted_count", len(expiredNotifications)),
	)

	return int64(len(expiredNotifications)), nil
}

// GetNotificationsFiltered gets notifications with a filter
func (r *NotificationRepository) GetNotificationsFiltered(ctx context.Context, username string, filter map[string]interface{}) ([]*models.Notification, string, error) {
	// Parse filter parameters
	limit := 20
	if l, ok := filter["limit"].(int); ok && l > 0 && l <= 100 {
		limit = l
	}

	cursor := ""
	if c, ok := filter["cursor"].(string); ok {
		cursor = c
	}

	notificationType := ""
	if t, ok := filter["types"].([]string); ok && len(t) > 0 {
		notificationType = t[0] // Use first type for simplicity
	}

	includeRead := true
	if ir, ok := filter["include_read"].(bool); ok {
		includeRead = ir
	}

	// Build pagination options
	opts := interfaces.PaginationOptions{
		Limit:  limit,
		Cursor: cursor,
	}

	// Get notifications
	result, err := r.GetUserNotifications(ctx, username, opts)
	if err != nil {
		return nil, "", err
	}

	// Filter results based on criteria
	filtered := make([]*models.Notification, 0, len(result.Items))
	for _, notification := range result.Items {
		// Filter by read status
		if !includeRead && notification.IsRead {
			continue
		}

		// Filter by type
		if notificationType != "" && notification.Type != notificationType {
			continue
		}

		filtered = append(filtered, notification)
	}

	return filtered, result.NextCursor, nil
}

// ClearOldNotifications clears old notifications for a user
func (r *NotificationRepository) ClearOldNotifications(ctx context.Context, username string, olderThan time.Time) (int, error) {
	// Get all notifications older than the specified time
	opts := interfaces.PaginationOptions{Limit: 1000}
	result, err := r.GetUserNotifications(ctx, username, opts)
	if err != nil {
		return 0, fmt.Errorf("failed to get notifications: %w", err)
	}

	deleted := 0
	for _, notification := range result.Items {
		if notification.CreatedAt.Before(olderThan) {
			if err := r.DeleteNotification(ctx, notification.ID); err != nil {
				r.logger.Warn("failed to delete old notification",
					zap.String("notification_id", notification.ID),
					zap.Error(err))
				continue
			}
			deleted++
		}
	}

	return deleted, nil
}

// GetNotificationsAdvanced retrieves notifications with advanced filtering options
func (r *NotificationRepository) GetNotificationsAdvanced(ctx context.Context, userID string, filters map[string]interface{}, pagination interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	query := r.db.WithContext(ctx).Model(&models.Notification{}).
		Index("user-notifications-index").
		Filter("UserID", "=", userID)

	// Apply additional filters
	for key, value := range filters {
		query = query.Filter(key, "=", value)
	}

	// Apply pagination with limit
	if pagination.Limit > 0 {
		query = query.Limit(pagination.Limit)
	} else {
		query = query.Limit(20) // Default limit
	}

	result := &interfaces.PaginatedResult[*models.Notification]{
		Items: []*models.Notification{},
	}

	var notifications []*models.Notification
	err := query.Scan(&notifications)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to query notifications: %w", err)
	}

	result.Items = notifications
	// For now, simple pagination - could enhance with cursor later
	result.HasMore = len(notifications) == pagination.Limit
	result.Total = int64(len(notifications))

	return result, nil
}

// GetNotificationPreferences gets notification preferences for a user
func (r *NotificationRepository) GetNotificationPreferences(ctx context.Context, userID string) (*models.NotificationPreferences, error) {
	var prefs models.NotificationPreferences
	err := r.db.WithContext(ctx).Model(&models.NotificationPreferences{}).
		Filter("Username", "=", userID).
		First(&prefs)

	if errors.IsNotFound(err) {
		// Return default preferences if not found
		return &models.NotificationPreferences{
			Username:              userID,
			EmailEnabled:          true,
			PushEnabled:           true,
			FollowEnabled:         true,
			MentionEnabled:        true,
			ReblogEnabled:         true,
			FavoriteEnabled:       true,
			FollowNotifications:   true,
			MentionNotifications:  true,
			ReblogNotifications:   true,
			FavoriteNotifications: true,
		}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get notification preferences: %w", err)
	}

	return &prefs, nil
}

// UpdateNotificationPreferences updates notification preferences for a user
func (r *NotificationRepository) UpdateNotificationPreferences(ctx context.Context, prefs *models.NotificationPreferences) error {
	prefs.UpdateKeys()

	err := r.db.WithContext(ctx).Model(prefs).Update()
	if err != nil {
		return fmt.Errorf("failed to update notification preferences: %w", err)
	}

	return nil
}

// SetNotificationPreference sets a specific notification preference
func (r *NotificationRepository) SetNotificationPreference(ctx context.Context, userID string, preferenceType string, enabled bool) error {
	prefs, err := r.GetNotificationPreferences(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get notification preferences: %w", err)
	}

	// Update the specific preference
	switch preferenceType {
	case "email_follow":
		prefs.FollowEnabled = enabled
		prefs.EmailEnabled = enabled
	case "email_reblog":
		prefs.ReblogEnabled = enabled
		prefs.EmailEnabled = enabled
	case "email_mention":
		prefs.MentionEnabled = enabled
		prefs.EmailEnabled = enabled
	case "push_follow":
		prefs.FollowNotifications = enabled
		prefs.PushEnabled = enabled
	case "push_reblog":
		prefs.ReblogNotifications = enabled
		prefs.PushEnabled = enabled
	case "push_mention":
		prefs.MentionNotifications = enabled
		prefs.PushEnabled = enabled
	default:
		return fmt.Errorf("unknown preference type: %s", preferenceType)
	}

	return r.UpdateNotificationPreferences(ctx, prefs)
}
