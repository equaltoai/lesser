package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
)

// NotificationRepository handles notification operations using DynamORM
type NotificationRepository struct {
	db        core.DB
	tableName string
}

// NewNotificationRepository creates a new notification repository
func NewNotificationRepository(db core.DB, tableName string) *NotificationRepository {
	return &NotificationRepository{
		db:        db,
		tableName: tableName,
	}
}

// CreateNotification creates a new notification
func (r *NotificationRepository) CreateNotification(ctx context.Context, notification *models.Notification) error {
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
func (r *NotificationRepository) GetNotification(ctx context.Context, id string) (*models.Notification, error) {
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
func (r *NotificationRepository) GetNotificationsByUser(ctx context.Context, userID string, limit int, cursor string) ([]*models.Notification, string, error) {
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
func (r *NotificationRepository) GetNotificationsByType(ctx context.Context, notificationType string, limit int, cursor string) ([]*models.Notification, string, error) {
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
func (r *NotificationRepository) GetNotificationsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Notification, string, error) {
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
func (r *NotificationRepository) GetNotificationsByGroup(ctx context.Context, groupKey string, limit int, cursor string) ([]*models.Notification, string, error) {
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
func (r *NotificationRepository) UpdateNotification(ctx context.Context, notification *models.Notification) error {
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
func (r *NotificationRepository) CountUnreadNotifications(ctx context.Context, userID string) (int, error) {
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
func (r *NotificationRepository) GetUnreadNotifications(ctx context.Context, userID string, limit int) ([]*models.Notification, error) {
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
func (r *NotificationRepository) GetPendingPushNotifications(ctx context.Context, limit int) ([]*models.Notification, error) {
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
func (r *NotificationRepository) GetNotificationsWithFilters(ctx context.Context, userID string, filters NotificationFilters, limit int, cursor string) ([]*models.Notification, string, error) {
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
func (r *NotificationRepository) GetDeliveryStatusStats(ctx context.Context, userID string, since time.Time) (*DeliveryStats, error) {
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

	// Create notifications one by one (in a real implementation, you'd use batch operations)
	for _, notification := range notifications {
		err := r.db.Model(notification).Create()
		if err != nil {
			return fmt.Errorf("failed to create notification in batch: %w", err)
		}
	}

	return nil
}

// DeleteExpiredNotifications deletes notifications that have expired
func (r *NotificationRepository) DeleteExpiredNotifications(ctx context.Context, before time.Time) error {
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

	// Delete notifications one by one (in a real implementation, you'd use batch operations)
	for _, notification := range expiredNotifications {
		err := r.db.Model(notification).Delete()
		if err != nil {
			return fmt.Errorf("failed to delete expired notification: %w", err)
		}
	}

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