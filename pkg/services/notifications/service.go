// Package notifications provides the Notifications Service for the Lesser project's API alignment.
// This service handles all notification operations including creation, reading, clearing,
// and notification preferences. It emits appropriate events for real-time streaming.
package notifications

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// Service provides notification operations
type Service struct {
	notificationRepo interfaces.NotificationRepository
	accountRepo      interfaces.AccountRepository
	publisher        streaming.Publisher
	logger           *zap.Logger
	domainName       string
}

// NewService creates a new Notifications Service with the required dependencies
func NewService(
	notificationRepo interfaces.NotificationRepository,
	accountRepo interfaces.AccountRepository,
	publisher streaming.Publisher,
	logger *zap.Logger,
	domainName string,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{
		notificationRepo: notificationRepo,
		accountRepo:      accountRepo,
		publisher:        publisher,
		logger:           logger,
		domainName:       domainName,
	}
}

// Command structs for operations

// CreateNotificationCommand contains all data needed to create a new notification
type CreateNotificationCommand struct {
	UserID     string                 `json:"user_id" validate:"required"`  // Recipient user ID
	Type       string                 `json:"type" validate:"required"`     // Notification type
	ActorID    string                 `json:"actor_id" validate:"required"` // Who triggered the notification
	ActorType  string                 `json:"actor_type"`                   // Type of actor (user, remote_actor)
	TargetID   string                 `json:"target_id"`                    // What the notification is about
	TargetType string                 `json:"target_type"`                  // Type of target (status, user, account)
	Title      string                 `json:"title"`                        // Notification title
	Body       string                 `json:"body"`                         // Notification body
	Data       map[string]interface{} `json:"data"`                         // Additional data
	GroupKey   string                 `json:"group_key"`                    // Custom group key for consolidation
}

// MarkAsReadCommand contains data needed to mark a notification as read
type MarkAsReadCommand struct {
	NotificationID string `json:"notification_id" validate:"required"`
	UserID         string `json:"user_id" validate:"required"`
}

// ClearCommand contains data needed to clear notifications
type ClearCommand struct {
	UserID           string   `json:"user_id" validate:"required"`
	NotificationIDs  []string `json:"notification_ids"`   // Clear specific notifications
	Types            []string `json:"types"`              // Clear notifications of specific types
	ClearAll         bool     `json:"clear_all"`          // Clear all notifications
	OlderThanSeconds int64    `json:"older_than_seconds"` // Clear notifications older than N seconds
}

// GetNotificationQuery contains parameters for retrieving a single notification
type GetNotificationQuery struct {
	NotificationID string `json:"notification_id" validate:"required"`
	UserID         string `json:"user_id" validate:"required"` // For privacy checks
}

// ListNotificationsQuery contains parameters for listing notifications with filters
type ListNotificationsQuery struct {
	UserID       string                       `json:"user_id" validate:"required"`
	Types        []string                     `json:"types"`         // Filter by notification types
	ExcludeTypes []string                     `json:"exclude_types"` // Exclude specific types
	OnlyUnread   bool                         `json:"only_unread"`   // Only unread notifications
	IncludeRead  bool                         `json:"include_read"`  // Include read notifications (default: true)
	GroupedOnly  bool                         `json:"grouped_only"`  // Only grouped notifications
	ActorID      string                       `json:"actor_id"`      // Filter by specific actor
	TargetType   string                       `json:"target_type"`   // Filter by target type
	Pagination   interfaces.PaginationOptions `json:"pagination"`
}

// Result structs for operations

// NotificationResult contains a notification and associated events that were emitted
type NotificationResult struct {
	Notification *models.Notification `json:"notification"`
	Events       []*streaming.Event   `json:"events"`
}

// NotificationListResult contains multiple notifications and pagination information
type NotificationListResult struct {
	Notifications []*models.Notification                            `json:"notifications"`
	Pagination    *interfaces.PaginatedResult[*models.Notification] `json:"pagination"`
	Summary       *NotificationSummary                              `json:"summary"`
	Events        []*streaming.Event                                `json:"events"`
}

// NotificationSummary provides summary statistics
type NotificationSummary struct {
	TotalCount         int64            `json:"total_count"`
	UnreadCount        int64            `json:"unread_count"`
	CountsByType       map[string]int64 `json:"counts_by_type"`
	LastNotificationAt *time.Time       `json:"last_notification_at,omitempty"`
}

// ClearResult contains the results of a clear operation
type ClearResult struct {
	ClearedCount int64              `json:"cleared_count"`
	Events       []*streaming.Event `json:"events"`
}

// CreateNotification creates a new notification, stores it, and emits events
func (s *Service) CreateNotification(ctx context.Context, cmd *CreateNotificationCommand) (*NotificationResult, error) {
	s.logger.Info("creating notification",
		zap.String("user_id", cmd.UserID),
		zap.String("type", cmd.Type),
		zap.String("actor_id", cmd.ActorID))

	// Validate the command
	if err := s.validateCreateCommand(ctx, cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check if recipient user exists
	_, err := s.accountRepo.GetAccount(ctx, cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("recipient user not found: %w", err)
	}

	// Check if actor exists (optional validation)
	if cmd.ActorID != "" && cmd.ActorID != "system" {
		_, err := s.accountRepo.GetAccount(ctx, cmd.ActorID)
		if err != nil {
			s.logger.Warn("actor not found for notification",
				zap.String("actor_id", cmd.ActorID),
				zap.Error(err))
		}
	}

	// Build notification using the builder pattern
	builder := models.NewNotificationBuilder().
		ForUser(cmd.UserID).
		OfType(cmd.Type).
		FromActor(cmd.ActorID, cmd.ActorType)

	if cmd.TargetID != "" && cmd.TargetType != "" {
		builder = builder.AboutTarget(cmd.TargetID, cmd.TargetType)
	}

	if cmd.Title != "" || cmd.Body != "" {
		builder = builder.WithContent(cmd.Title, cmd.Body)
	}

	if cmd.GroupKey != "" {
		builder = builder.WithGroupKey(cmd.GroupKey)
	}

	// Add data fields
	for key, value := range cmd.Data {
		builder = builder.WithData(key, value)
	}

	notification := builder.Build()

	// Check for notification preferences and consolidation
	if err := s.handleNotificationConsolidation(ctx, notification); err != nil {
		s.logger.Warn("failed to handle notification consolidation",
			zap.String("notification_id", notification.ID),
			zap.Error(err))
	}

	// Store the notification
	if err := s.notificationRepo.CreateNotification(ctx, notification); err != nil {
		return nil, fmt.Errorf("failed to create notification: %w", err)
	}

	s.logger.Info("created notification successfully",
		zap.String("notification_id", notification.ID),
		zap.String("type", notification.Type),
		zap.String("group_key", notification.GroupKey))

	// Emit events
	events := s.emitNotificationCreatedEvents(ctx, notification)

	return &NotificationResult{
		Notification: notification,
		Events:       events,
	}, nil
}

// MarkAsRead marks a notification as read and emits events
func (s *Service) MarkAsRead(ctx context.Context, cmd *MarkAsReadCommand) (*NotificationResult, error) {
	s.logger.Info("marking notification as read",
		zap.String("notification_id", cmd.NotificationID),
		zap.String("user_id", cmd.UserID))

	// Get the notification
	notification, err := s.notificationRepo.GetNotification(ctx, cmd.NotificationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get notification: %w", err)
	}

	// Verify ownership
	if notification.UserID != cmd.UserID {
		return nil, fmt.Errorf("unauthorized: notification belongs to different user")
	}

	// Check if already read
	if notification.IsRead {
		// Return current state without error (idempotent)
		return &NotificationResult{
			Notification: notification,
			Events:       []*streaming.Event{},
		}, nil
	}

	// Mark as read
	notification.MarkRead()

	// Update the notification
	if err := s.notificationRepo.UpdateNotification(ctx, notification); err != nil {
		return nil, fmt.Errorf("failed to update notification: %w", err)
	}

	s.logger.Info("marked notification as read successfully",
		zap.String("notification_id", cmd.NotificationID))

	// Emit events
	events := s.emitNotificationReadEvents(ctx, notification)

	return &NotificationResult{
		Notification: notification,
		Events:       events,
	}, nil
}

// ClearNotifications clears notifications based on the specified criteria and emits events
func (s *Service) ClearNotifications(ctx context.Context, cmd *ClearCommand) (*ClearResult, error) {
	s.logger.Info("clearing notifications",
		zap.String("user_id", cmd.UserID),
		zap.Bool("clear_all", cmd.ClearAll),
		zap.Strings("types", cmd.Types),
		zap.Int("notification_ids_count", len(cmd.NotificationIDs)))

	var clearedCount int64
	var err error

	// Validate the command
	if err := s.validateClearCommand(cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	switch {
	case cmd.ClearAll:
		// Clear all notifications for user
		clearedCount, err = s.clearAllNotifications(ctx, cmd.UserID, cmd.OlderThanSeconds)

	case len(cmd.NotificationIDs) > 0:
		// Clear specific notifications
		clearedCount, err = s.clearSpecificNotifications(ctx, cmd.UserID, cmd.NotificationIDs)

	case len(cmd.Types) > 0:
		// Clear notifications by type
		clearedCount, err = s.clearNotificationsByType(ctx, cmd.UserID, cmd.Types, cmd.OlderThanSeconds)

	default:
		return nil, fmt.Errorf("no clear criteria specified")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to clear notifications: %w", err)
	}

	s.logger.Info("cleared notifications successfully",
		zap.String("user_id", cmd.UserID),
		zap.Int64("cleared_count", clearedCount))

	// Emit events
	events := s.emitNotificationsClearedEvents(ctx, cmd.UserID, clearedCount, cmd)

	return &ClearResult{
		ClearedCount: clearedCount,
		Events:       events,
	}, nil
}

// GetNotification retrieves a single notification with privacy checks
func (s *Service) GetNotification(ctx context.Context, query *GetNotificationQuery) (*models.Notification, error) {
	s.logger.Debug("getting notification",
		zap.String("notification_id", query.NotificationID),
		zap.String("user_id", query.UserID))

	// Get the notification
	notification, err := s.notificationRepo.GetNotification(ctx, query.NotificationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get notification: %w", err)
	}

	// Verify ownership
	if notification.UserID != query.UserID {
		return nil, fmt.Errorf("notification not found") // Don't reveal it exists for other users
	}

	return notification, nil
}

// ListNotifications retrieves notifications based on filters and pagination
func (s *Service) ListNotifications(ctx context.Context, query *ListNotificationsQuery) (*NotificationListResult, error) {
	s.logger.Debug("listing notifications",
		zap.String("user_id", query.UserID),
		zap.Strings("types", query.Types),
		zap.Bool("only_unread", query.OnlyUnread))

	var result *interfaces.PaginatedResult[*models.Notification]
	var err error

	// Get notifications based on filters
	if query.OnlyUnread {
		result, err = s.notificationRepo.GetUnreadNotifications(ctx, query.UserID, query.Pagination)
	} else if len(query.Types) > 0 {
		// Get notifications for each type and merge
		result, err = s.getNotificationsByTypes(ctx, query)
	} else {
		result, err = s.notificationRepo.GetUserNotifications(ctx, query.UserID, query.Pagination)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get notifications: %w", err)
	}

	// Apply additional filters
	filteredNotifications := s.applyNotificationFilters(result.Items, query)

	// Get summary statistics
	summary, err := s.getNotificationSummary(ctx, query.UserID)
	if err != nil {
		s.logger.Warn("failed to get notification summary", zap.Error(err))
		// Continue without summary
	}

	// Update result with filtered items
	filteredResult := &interfaces.PaginatedResult[*models.Notification]{
		Items:      filteredNotifications,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
		Total:      int64(len(filteredNotifications)),
	}

	return &NotificationListResult{
		Notifications: filteredNotifications,
		Pagination:    filteredResult,
		Summary:       summary,
		Events:        []*streaming.Event{}, // No events for read operations
	}, nil
}

// Private helper methods

func (s *Service) validateCreateCommand(_ context.Context, cmd *CreateNotificationCommand) error {
	if strings.TrimSpace(cmd.UserID) == "" {
		return fmt.Errorf("user_id is required")
	}

	if strings.TrimSpace(cmd.Type) == "" {
		return fmt.Errorf("type is required")
	}

	if strings.TrimSpace(cmd.ActorID) == "" {
		return fmt.Errorf("actor_id is required")
	}

	// Validate notification type
	validTypes := map[string]bool{
		"mention":        true,
		"reblog":         true,
		"favourite":      true,
		"follow":         true,
		"follow_request": true,
		"poll":           true,
		"status":         true,
		"update":         true,
		"admin.sign_up":  true,
		"admin.report":   true,
	}

	if !validTypes[strings.ToLower(cmd.Type)] {
		return fmt.Errorf("invalid notification type: %s", cmd.Type)
	}

	return nil
}

func (s *Service) validateClearCommand(cmd *ClearCommand) error {
	if strings.TrimSpace(cmd.UserID) == "" {
		return fmt.Errorf("user_id is required")
	}

	// At least one clear criteria must be specified
	if !cmd.ClearAll && len(cmd.NotificationIDs) == 0 && len(cmd.Types) == 0 {
		return fmt.Errorf("at least one clear criteria must be specified")
	}

	return nil
}

func (s *Service) handleNotificationConsolidation(_ context.Context, notification *models.Notification) error {
	// Try to find existing notification in the same group for consolidation
	if notification.GroupKey == "" {
		return nil // No grouping requested
	}

	// Check for existing notifications in the same group within recent time window
	// This is a simplified consolidation - in production, you might want more sophisticated logic
	return nil
}

func (s *Service) clearAllNotifications(ctx context.Context, userID string, olderThanSeconds int64) (int64, error) {
	if olderThanSeconds > 0 {
		expiredBefore := time.Now().Add(-time.Duration(olderThanSeconds) * time.Second)
		return s.notificationRepo.DeleteExpiredNotifications(ctx, expiredBefore)
	}

	// Mark all as read instead of deleting (better UX)
	return 0, s.notificationRepo.MarkAllNotificationsRead(ctx, userID)
}

func (s *Service) clearSpecificNotifications(ctx context.Context, userID string, notificationIDs []string) (int64, error) {
	var count int64
	for _, id := range notificationIDs {
		// Verify ownership before deletion
		notification, err := s.notificationRepo.GetNotification(ctx, id)
		if err != nil {
			continue // Skip notifications that don't exist
		}

		if notification.UserID != userID {
			continue // Skip notifications that don't belong to the user
		}

		if err := s.notificationRepo.DeleteNotification(ctx, id); err != nil {
			s.logger.Warn("failed to delete notification",
				zap.String("notification_id", id),
				zap.Error(err))
			continue
		}

		count++
	}

	return count, nil
}

func (s *Service) clearNotificationsByType(ctx context.Context, userID string, types []string, olderThanSeconds int64) (int64, error) {
	var totalCount int64

	for _, notifType := range types {
		if olderThanSeconds > 0 {
			// Delete old notifications of this type
			// This would require a custom repository method to delete by type and age
			if err := s.notificationRepo.MarkNotificationsReadByType(ctx, userID, notifType); err != nil {
				s.logger.Warn("failed to mark notifications read by type",
					zap.String("type", notifType),
					zap.Error(err))
				continue
			}
		} else {
			// Mark all notifications of this type as read
			if err := s.notificationRepo.MarkNotificationsReadByType(ctx, userID, notifType); err != nil {
				s.logger.Warn("failed to mark notifications read by type",
					zap.String("type", notifType),
					zap.Error(err))
				continue
			}
		}

		totalCount++
	}

	return totalCount, nil
}

func (s *Service) getNotificationsByTypes(ctx context.Context, query *ListNotificationsQuery) (*interfaces.PaginatedResult[*models.Notification], error) {
	// For simplicity, get notifications by first type
	// In production, you might want to merge results from multiple types
	if len(query.Types) > 0 {
		return s.notificationRepo.GetNotificationsByType(ctx, query.UserID, query.Types[0], query.Pagination)
	}

	return s.notificationRepo.GetUserNotifications(ctx, query.UserID, query.Pagination)
}

func (s *Service) applyNotificationFilters(notifications []*models.Notification, query *ListNotificationsQuery) []*models.Notification {
	filtered := make([]*models.Notification, 0, len(notifications))

	for _, notification := range notifications {
		// Filter by read status
		if query.OnlyUnread && notification.IsRead {
			continue
		}

		if !query.IncludeRead && notification.IsRead && !query.OnlyUnread {
			continue
		}

		// Filter by exclude types
		if len(query.ExcludeTypes) > 0 {
			excluded := false
			for _, excludeType := range query.ExcludeTypes {
				if strings.EqualFold(notification.Type, excludeType) {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
		}

		// Filter by actor ID
		if query.ActorID != "" && notification.ActorID != query.ActorID {
			continue
		}

		// Filter by target type
		if query.TargetType != "" && notification.TargetType != query.TargetType {
			continue
		}

		// Filter by grouped only
		if query.GroupedOnly && notification.GroupCount <= 1 {
			continue
		}

		filtered = append(filtered, notification)
	}

	return filtered
}

func (s *Service) getNotificationSummary(ctx context.Context, userID string) (*NotificationSummary, error) {
	// Get unread count
	unreadCount, err := s.notificationRepo.GetUnreadNotificationCount(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get unread count: %w", err)
	}

	// Get counts by type
	countsByType, err := s.notificationRepo.GetNotificationCountsByType(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get counts by type: %w", err)
	}

	// Calculate total count
	var totalCount int64
	for _, count := range countsByType {
		totalCount += count
	}

	// Get last notification time by fetching the most recent notification
	var lastNotificationAt *time.Time
	recentNotifications, err := s.notificationRepo.GetUserNotifications(ctx, userID, interfaces.PaginationOptions{Limit: 1})
	if err == nil && len(recentNotifications.Items) > 0 {
		lastNotificationAt = &recentNotifications.Items[0].CreatedAt
	}

	return &NotificationSummary{
		TotalCount:         totalCount,
		UnreadCount:        unreadCount,
		CountsByType:       countsByType,
		LastNotificationAt: lastNotificationAt,
	}, nil
}

func (s *Service) emitNotificationCreatedEvents(ctx context.Context, notification *models.Notification) []*streaming.Event {
	var events []*streaming.Event

	// Create base event
	event := &streaming.Event{
		Type:      "notification.created",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"notification": notification,
		},
	}

	// Emit to recipient's notification stream
	notificationEvent := *event
	notificationEvent.Stream = fmt.Sprintf("notifications:%s", notification.UserID)
	if err := s.publisher.PublishToUser(ctx, notification.UserID, &notificationEvent); err != nil {
		s.logger.Error("failed to publish notification created event",
			zap.String("user_id", notification.UserID),
			zap.Error(err))
	} else {
		events = append(events, &notificationEvent)
	}

	return events
}

func (s *Service) emitNotificationReadEvents(ctx context.Context, notification *models.Notification) []*streaming.Event {
	var events []*streaming.Event

	// Create base event
	event := &streaming.Event{
		Type:      "notification.read",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"notification_id": notification.ID,
			"read_at":         notification.ReadAt,
		},
	}

	// Emit to recipient's notification stream only
	notificationEvent := *event
	notificationEvent.Stream = fmt.Sprintf("notifications:%s", notification.UserID)
	if err := s.publisher.PublishToUser(ctx, notification.UserID, &notificationEvent); err != nil {
		s.logger.Error("failed to publish notification read event",
			zap.String("user_id", notification.UserID),
			zap.Error(err))
	} else {
		events = append(events, &notificationEvent)
	}

	return events
}

func (s *Service) emitNotificationsClearedEvents(ctx context.Context, userID string, clearedCount int64, cmd *ClearCommand) []*streaming.Event {
	var events []*streaming.Event

	// Create base event
	event := &streaming.Event{
		Type:      "notification.cleared",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"user_id":       userID,
			"cleared_count": clearedCount,
			"clear_all":     cmd.ClearAll,
			"types":         cmd.Types,
		},
	}

	// Emit to recipient's notification stream only
	notificationEvent := *event
	notificationEvent.Stream = fmt.Sprintf("notifications:%s", userID)
	if err := s.publisher.PublishToUser(ctx, userID, &notificationEvent); err != nil {
		s.logger.Error("failed to publish notifications cleared event",
			zap.String("user_id", userID),
			zap.Error(err))
	} else {
		events = append(events, &notificationEvent)
	}

	return events
}
