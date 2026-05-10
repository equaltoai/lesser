// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// NotificationRepository is a thread-safe in-memory implementation of interfaces.NotificationRepository.
// It stores data in memory for integration-style testing without requiring DynamoDB.
type NotificationRepository struct {
	mu sync.RWMutex

	// Core notification data - keyed by notification ID
	notifications map[string]*notificationEntry

	// Indexes for efficient querying
	byUser     map[string][]string // userID -> notification IDs
	byType     map[string][]string // type -> notification IDs
	byGroupKey map[string][]string // groupKey -> notification IDs
	byObject   map[string][]string // objectID -> notification IDs

	// Notification preferences - keyed by userID
	preferences map[string]*models.NotificationPreferences

	// Dispatcher for push notifications
	dispatcher interfaces.NotificationDispatcher
}

// notificationEntry stores a notification with metadata
type notificationEntry struct {
	notification *models.Notification
	createdAt    time.Time
	updatedAt    time.Time
}

// NewNotificationRepository creates a new in-memory notification repository
func NewNotificationRepository() *NotificationRepository {
	return &NotificationRepository{
		notifications: make(map[string]*notificationEntry),
		byUser:        make(map[string][]string),
		byType:        make(map[string][]string),
		byGroupKey:    make(map[string][]string),
		byObject:      make(map[string][]string),
		preferences:   make(map[string]*models.NotificationPreferences),
	}
}

// copyNotification creates a deep copy of a notification
func copyNotification(n *models.Notification) *models.Notification {
	if n == nil {
		return nil
	}
	notificationCopy := *n
	if n.Data != nil {
		notificationCopy.Data = make(map[string]interface{})
		for k, v := range n.Data {
			notificationCopy.Data[k] = v
		}
	}
	if n.ReadAt != nil {
		readAt := *n.ReadAt
		notificationCopy.ReadAt = &readAt
	}
	if n.PushSentAt != nil {
		pushSentAt := *n.PushSentAt
		notificationCopy.PushSentAt = &pushSentAt
	}
	return &notificationCopy
}

// SetDispatcher sets the notification dispatcher
func (r *NotificationRepository) SetDispatcher(dispatcher interfaces.NotificationDispatcher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dispatcher = dispatcher
}

// Core notification operations

// CreateNotification creates a new notification
func (r *NotificationRepository) CreateNotification(_ context.Context, notification *models.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if notification == nil {
		return fmt.Errorf("notification is required")
	}

	// Call BeforeCreate to set up keys and defaults
	if err := notification.BeforeCreate(); err != nil {
		return err
	}

	if _, exists := r.notifications[notification.ID]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	r.notifications[notification.ID] = &notificationEntry{
		notification: copyNotification(notification),
		createdAt:    now,
		updatedAt:    now,
	}

	// Update indexes
	r.addToIndexes(notification)

	// Dispatch push notification if dispatcher is set
	if r.dispatcher != nil {
		r.dispatcher.DispatchPushForNotification(context.Background(), notification)
	}

	return nil
}

// GetNotification retrieves a notification by ID
func (r *NotificationRepository) GetNotification(_ context.Context, notificationID string) (*models.Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.notifications[notificationID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return copyNotification(entry.notification), nil
}

// GetUserNotification retrieves a recipient-owned notification by
// (userID, notificationID).
func (r *NotificationRepository) GetUserNotification(_ context.Context, userID, notificationID string) (*models.Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.notifications[notificationID]
	if !exists || entry.notification.UserID != userID {
		return nil, storage.ErrNotFound
	}

	return copyNotification(entry.notification), nil
}

// UpdateNotification updates an existing notification
func (r *NotificationRepository) UpdateNotification(_ context.Context, notification *models.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if notification == nil {
		return fmt.Errorf("notification is required")
	}

	entry, exists := r.notifications[notification.ID]
	if !exists {
		return storage.ErrNotFound
	}

	// Call BeforeUpdate
	if err := notification.BeforeUpdate(); err != nil {
		return err
	}

	entry.notification = copyNotification(notification)
	entry.updatedAt = time.Now()

	return nil
}

// DeleteNotification deletes a notification
func (r *NotificationRepository) DeleteNotification(_ context.Context, notificationID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.notifications[notificationID]
	if !exists {
		return storage.ErrNotFound
	}

	// Remove from indexes
	r.removeFromIndexes(entry.notification)

	delete(r.notifications, notificationID)
	return nil
}

// DeleteUserNotification deletes a recipient-owned notification by
// (userID, notificationID). Wrong-user IDs resolve as not found.
func (r *NotificationRepository) DeleteUserNotification(_ context.Context, userID, notificationID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.notifications[notificationID]
	if !exists || entry.notification.UserID != userID {
		return storage.ErrNotFound
	}

	r.removeFromIndexes(entry.notification)
	delete(r.notifications, notificationID)
	return nil
}

// User notification queries

// GetUserNotifications retrieves notifications for a user with pagination
func (r *NotificationRepository) GetUserNotifications(_ context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids, exists := r.byUser[userID]
	if !exists {
		return &interfaces.PaginatedResult[*models.Notification]{
			Items: []*models.Notification{},
			Total: 0,
		}, nil
	}

	return r.paginateNotifications(ids, opts)
}

// GetUnreadNotifications retrieves unread notifications for a user with pagination
func (r *NotificationRepository) GetUnreadNotifications(_ context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids, exists := r.byUser[userID]
	if !exists {
		return &interfaces.PaginatedResult[*models.Notification]{
			Items: []*models.Notification{},
			Total: 0,
		}, nil
	}

	// Filter to unread only
	var unreadIDs []string
	for _, id := range ids {
		if entry, ok := r.notifications[id]; ok && !entry.notification.IsRead {
			unreadIDs = append(unreadIDs, id)
		}
	}

	return r.paginateNotifications(unreadIDs, opts)
}

// GetNotificationsByType retrieves notifications by type with pagination
func (r *NotificationRepository) GetNotificationsByType(_ context.Context, userID, notificationType string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	userIDs, exists := r.byUser[userID]
	if !exists {
		return &interfaces.PaginatedResult[*models.Notification]{
			Items: []*models.Notification{},
			Total: 0,
		}, nil
	}

	// Filter by type
	var filteredIDs []string
	for _, id := range userIDs {
		if entry, ok := r.notifications[id]; ok && strings.EqualFold(entry.notification.Type, notificationType) {
			filteredIDs = append(filteredIDs, id)
		}
	}

	return r.paginateNotifications(filteredIDs, opts)
}

// Notification status management

// MarkNotificationRead marks a notification as read
func (r *NotificationRepository) MarkNotificationRead(_ context.Context, notificationID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.notifications[notificationID]
	if !exists {
		return storage.ErrNotFound
	}

	entry.notification.MarkRead()
	entry.updatedAt = time.Now()
	return nil
}

// MarkNotificationUnread marks a notification as unread
func (r *NotificationRepository) MarkNotificationUnread(_ context.Context, notificationID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.notifications[notificationID]
	if !exists {
		return storage.ErrNotFound
	}

	entry.notification.MarkUnread()
	entry.updatedAt = time.Now()
	return nil
}

// MarkAllNotificationsRead marks all notifications as read for a user
func (r *NotificationRepository) MarkAllNotificationsRead(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids, exists := r.byUser[userID]
	if !exists {
		return nil
	}

	now := time.Now()
	for _, id := range ids {
		if entry, ok := r.notifications[id]; ok {
			entry.notification.MarkRead()
			entry.updatedAt = now
		}
	}

	return nil
}

// MarkNotificationsReadByType marks notifications as read by type for a user
func (r *NotificationRepository) MarkNotificationsReadByType(_ context.Context, userID, notificationType string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids, exists := r.byUser[userID]
	if !exists {
		return nil
	}

	now := time.Now()
	for _, id := range ids {
		if entry, ok := r.notifications[id]; ok && strings.EqualFold(entry.notification.Type, notificationType) {
			entry.notification.MarkRead()
			entry.updatedAt = now
		}
	}

	return nil
}

// Push notification tracking

// MarkNotificationPushSent marks a notification's push as sent
func (r *NotificationRepository) MarkNotificationPushSent(_ context.Context, notificationID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.notifications[notificationID]
	if !exists {
		return storage.ErrNotFound
	}

	entry.notification.MarkPushSent()
	entry.updatedAt = time.Now()
	return nil
}

// MarkNotificationPushFailed marks a notification's push as failed
func (r *NotificationRepository) MarkNotificationPushFailed(_ context.Context, notificationID, errorMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.notifications[notificationID]
	if !exists {
		return storage.ErrNotFound
	}

	entry.notification.MarkPushFailed(errorMsg)
	entry.updatedAt = time.Now()
	return nil
}

// GetPendingPushNotifications retrieves notifications that need push delivery
func (r *NotificationRepository) GetPendingPushNotifications(_ context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var pendingIDs []string
	for id, entry := range r.notifications {
		if !entry.notification.PushSent && entry.notification.PushError == "" {
			pendingIDs = append(pendingIDs, id)
		}
	}

	// Sort by creation time (oldest first for push delivery)
	sort.Slice(pendingIDs, func(i, j int) bool {
		ei := r.notifications[pendingIDs[i]]
		ej := r.notifications[pendingIDs[j]]
		return ei.notification.CreatedAt.Before(ej.notification.CreatedAt)
	})

	return r.paginateNotificationsAsc(pendingIDs, opts)
}

// Notification grouping and consolidation

// GetNotificationGroups retrieves notification groups for a user with pagination
func (r *NotificationRepository) GetNotificationGroups(_ context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids, exists := r.byUser[userID]
	if !exists {
		return &interfaces.PaginatedResult[*models.Notification]{
			Items: []*models.Notification{},
			Total: 0,
		}, nil
	}

	// Group by group key, keeping only the most recent notification per group
	groups := make(map[string]*models.Notification)
	groupOrder := make([]string, 0)

	for _, id := range ids {
		entry, ok := r.notifications[id]
		if !ok {
			continue
		}
		notif := entry.notification
		if existing, exists := groups[notif.GroupKey]; !exists || notif.CreatedAt.After(existing.CreatedAt) {
			if !exists {
				groupOrder = append(groupOrder, notif.GroupKey)
			}
			groups[notif.GroupKey] = notif
		}
	}

	// Sort groups by most recent notification
	sort.Slice(groupOrder, func(i, j int) bool {
		return groups[groupOrder[i]].CreatedAt.After(groups[groupOrder[j]].CreatedAt)
	})

	// Apply pagination
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	result := &interfaces.PaginatedResult[*models.Notification]{
		Items: make([]*models.Notification, 0),
		Total: int64(len(groups)),
	}

	startIdx := 0
	if opts.Cursor != "" {
		for i, key := range groupOrder {
			if groups[key].SK == opts.Cursor {
				startIdx = i + 1
				break
			}
		}
	}

	for i := startIdx; i < len(groupOrder) && len(result.Items) < limit+1; i++ {
		result.Items = append(result.Items, copyNotification(groups[groupOrder[i]]))
	}

	if len(result.Items) > limit {
		result.HasMore = true
		result.NextCursor = result.Items[limit-1].SK
		result.Items = result.Items[:limit]
	}

	return result, nil
}

// ConsolidateNotifications consolidates notifications by group key
func (r *NotificationRepository) ConsolidateNotifications(_ context.Context, groupKey string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids, exists := r.byGroupKey[groupKey]
	if !exists || len(ids) <= 1 {
		return nil // Nothing to consolidate
	}

	// Find the most recent notification
	var mostRecentID string
	var mostRecentTime time.Time
	for _, id := range ids {
		entry, ok := r.notifications[id]
		if !ok {
			continue
		}
		if mostRecentID == "" || entry.notification.CreatedAt.After(mostRecentTime) {
			mostRecentID = id
			mostRecentTime = entry.notification.CreatedAt
		}
	}

	if mostRecentID == "" {
		return nil
	}

	// Update group count on most recent and delete others
	mostRecent := r.notifications[mostRecentID]
	mostRecent.notification.GroupCount = len(ids)
	mostRecent.updatedAt = time.Now()

	for _, id := range ids {
		if id != mostRecentID {
			if entry, ok := r.notifications[id]; ok {
				r.removeFromIndexes(entry.notification)
				delete(r.notifications, id)
			}
		}
	}

	return nil
}

// Notification counts and summaries

// GetUnreadNotificationCount returns the count of unread notifications
func (r *NotificationRepository) GetUnreadNotificationCount(_ context.Context, userID string) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids, exists := r.byUser[userID]
	if !exists {
		return 0, nil
	}

	var count int64
	for _, id := range ids {
		if entry, ok := r.notifications[id]; ok && !entry.notification.IsRead {
			count++
		}
	}

	return count, nil
}

// GetNotificationCountsByType returns notification counts by type
func (r *NotificationRepository) GetNotificationCountsByType(_ context.Context, userID string) (map[string]int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	counts := make(map[string]int64)

	ids, exists := r.byUser[userID]
	if !exists {
		return counts, nil
	}

	for _, id := range ids {
		if entry, ok := r.notifications[id]; ok {
			counts[entry.notification.Type]++
		}
	}

	return counts, nil
}

// Batch operations

// CreateNotifications creates multiple notifications efficiently
func (r *NotificationRepository) CreateNotifications(ctx context.Context, notifications []*models.Notification) error {
	for _, notification := range notifications {
		if err := r.CreateNotification(ctx, notification); err != nil {
			return err
		}
	}
	return nil
}

// DeleteNotificationsByType deletes notifications by type for a user
func (r *NotificationRepository) DeleteNotificationsByType(_ context.Context, userID, notificationType string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids, exists := r.byUser[userID]
	if !exists {
		return nil
	}

	// Collect IDs to delete
	var toDelete []string
	for _, id := range ids {
		if entry, ok := r.notifications[id]; ok && strings.EqualFold(entry.notification.Type, notificationType) {
			toDelete = append(toDelete, id)
		}
	}

	// Delete notifications
	for _, id := range toDelete {
		if entry, ok := r.notifications[id]; ok {
			r.removeFromIndexes(entry.notification)
			delete(r.notifications, id)
		}
	}

	return nil
}

// DeleteNotificationsByObject deletes all notifications related to a specific object
func (r *NotificationRepository) DeleteNotificationsByObject(_ context.Context, objectID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids, exists := r.byObject[objectID]
	if !exists {
		return nil
	}

	// Make a copy since we'll be modifying the slice
	toDelete := make([]string, len(ids))
	copy(toDelete, ids)

	for _, id := range toDelete {
		if entry, ok := r.notifications[id]; ok {
			r.removeFromIndexes(entry.notification)
			delete(r.notifications, id)
		}
	}

	return nil
}

// DeleteExpiredNotifications deletes notifications that have expired
func (r *NotificationRepository) DeleteExpiredNotifications(_ context.Context, expiredBefore time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var deleted int64
	var toDelete []string

	for id, entry := range r.notifications {
		if entry.notification.ExpiresAt > 0 && time.Unix(entry.notification.ExpiresAt, 0).Before(expiredBefore) {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		if entry, ok := r.notifications[id]; ok {
			r.removeFromIndexes(entry.notification)
			delete(r.notifications, id)
			deleted++
		}
	}

	return deleted, nil
}

// Filtered and advanced queries

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
		notificationType = t[0]
	}

	includeRead := true
	if ir, ok := filter["include_read"].(bool); ok {
		includeRead = ir
	}

	opts := interfaces.PaginationOptions{
		Limit:  limit,
		Cursor: cursor,
	}

	result, err := r.GetUserNotifications(ctx, username, opts)
	if err != nil {
		return nil, "", err
	}

	// Filter results
	filtered := make([]*models.Notification, 0, len(result.Items))
	for _, notification := range result.Items {
		if !includeRead && notification.IsRead {
			continue
		}
		if notificationType != "" && notification.Type != notificationType {
			continue
		}
		filtered = append(filtered, notification)
	}

	return filtered, result.NextCursor, nil
}

// ClearOldNotifications clears old notifications for a user
func (r *NotificationRepository) ClearOldNotifications(_ context.Context, username string, olderThan time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids, exists := r.byUser[username]
	if !exists {
		return 0, nil
	}

	var deleted int
	var toDelete []string

	for _, id := range ids {
		if entry, ok := r.notifications[id]; ok && entry.notification.CreatedAt.Before(olderThan) {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		if entry, ok := r.notifications[id]; ok {
			r.removeFromIndexes(entry.notification)
			delete(r.notifications, id)
			deleted++
		}
	}

	return deleted, nil
}

// GetNotificationsAdvanced retrieves notifications with advanced filtering options
func (r *NotificationRepository) GetNotificationsAdvanced(_ context.Context, userID string, filters map[string]interface{}, pagination interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids, exists := r.byUser[userID]
	if !exists {
		return &interfaces.PaginatedResult[*models.Notification]{
			Items: []*models.Notification{},
			Total: 0,
		}, nil
	}

	// Filter notifications
	var filteredIDs []string
	for _, id := range ids {
		entry, ok := r.notifications[id]
		if !ok {
			continue
		}

		// Apply filters
		match := true
		for key, value := range filters {
			switch key {
			case "IsRead":
				if v, ok := value.(bool); ok && entry.notification.IsRead != v {
					match = false
				}
			case "Type":
				if v, ok := value.(string); ok && !strings.EqualFold(entry.notification.Type, v) {
					match = false
				}
			}
		}

		if match {
			filteredIDs = append(filteredIDs, id)
		}
	}

	return r.paginateNotifications(filteredIDs, pagination)
}

// Notification preferences

// GetNotificationPreferences gets notification preferences for a user
func (r *NotificationRepository) GetNotificationPreferences(_ context.Context, userID string) (*models.NotificationPreferences, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefs, exists := r.preferences[userID]
	if !exists {
		// Return default preferences
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

	// Return a copy
	prefsCopy := *prefs
	return &prefsCopy, nil
}

// UpdateNotificationPreferences updates notification preferences for a user
func (r *NotificationRepository) UpdateNotificationPreferences(_ context.Context, prefs *models.NotificationPreferences) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if prefs == nil {
		return fmt.Errorf("preferences is required")
	}

	prefs.UpdateKeys()
	prefsCopy := *prefs
	r.preferences[prefs.Username] = &prefsCopy

	return nil
}

// SetNotificationPreference sets a specific notification preference
func (r *NotificationRepository) SetNotificationPreference(ctx context.Context, userID string, preferenceType string, enabled bool) error {
	prefs, err := r.GetNotificationPreferences(ctx, userID)
	if err != nil {
		return err
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

// Helper methods

// addToIndexes adds a notification to all relevant indexes
func (r *NotificationRepository) addToIndexes(notification *models.Notification) {
	// User index
	r.byUser[notification.UserID] = append(r.byUser[notification.UserID], notification.ID)

	// Type index
	r.byType[notification.Type] = append(r.byType[notification.Type], notification.ID)

	// Group key index
	if notification.GroupKey != "" {
		r.byGroupKey[notification.GroupKey] = append(r.byGroupKey[notification.GroupKey], notification.ID)
	}

	// Object index (using TargetID as object reference)
	if notification.TargetID != "" {
		r.byObject[notification.TargetID] = append(r.byObject[notification.TargetID], notification.ID)
	}
}

// removeFromIndexes removes a notification from all indexes
func (r *NotificationRepository) removeFromIndexes(notification *models.Notification) {
	// User index
	r.byUser[notification.UserID] = removeFromSlice(r.byUser[notification.UserID], notification.ID)

	// Type index
	r.byType[notification.Type] = removeFromSlice(r.byType[notification.Type], notification.ID)

	// Group key index
	if notification.GroupKey != "" {
		r.byGroupKey[notification.GroupKey] = removeFromSlice(r.byGroupKey[notification.GroupKey], notification.ID)
	}

	// Object index
	if notification.TargetID != "" {
		r.byObject[notification.TargetID] = removeFromSlice(r.byObject[notification.TargetID], notification.ID)
	}
}

// paginateNotifications paginates a list of notification IDs (newest first)
func (r *NotificationRepository) paginateNotifications(ids []string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	// Sort by creation time (newest first)
	sortedIDs := make([]string, len(ids))
	copy(sortedIDs, ids)
	sort.Slice(sortedIDs, func(i, j int) bool {
		ei := r.notifications[sortedIDs[i]]
		ej := r.notifications[sortedIDs[j]]
		if ei == nil || ej == nil {
			return false
		}
		return ei.notification.CreatedAt.After(ej.notification.CreatedAt)
	})

	// Normalize limit
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Find start index based on cursor
	startIdx := 0
	if opts.Cursor != "" {
		for i, id := range sortedIDs {
			entry := r.notifications[id]
			if entry != nil && entry.notification.SK == opts.Cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Collect results
	result := &interfaces.PaginatedResult[*models.Notification]{
		Items: make([]*models.Notification, 0),
		Total: int64(len(sortedIDs)),
	}

	for i := startIdx; i < len(sortedIDs) && len(result.Items) < limit+1; i++ {
		entry := r.notifications[sortedIDs[i]]
		if entry != nil {
			result.Items = append(result.Items, copyNotification(entry.notification))
		}
	}

	// Determine next cursor
	if len(result.Items) > limit {
		result.HasMore = true
		result.NextCursor = result.Items[limit-1].SK
		result.Items = result.Items[:limit]
	}

	return result, nil
}

// paginateNotificationsAsc paginates a list of notification IDs (oldest first)
func (r *NotificationRepository) paginateNotificationsAsc(ids []string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	// Normalize limit
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	// Find start index based on cursor
	startIdx := 0
	if opts.Cursor != "" {
		for i, id := range ids {
			entry := r.notifications[id]
			if entry != nil && fmt.Sprintf("%d", entry.notification.CreatedAt.Unix()) == opts.Cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Collect results
	result := &interfaces.PaginatedResult[*models.Notification]{
		Items: make([]*models.Notification, 0),
		Total: int64(len(ids)),
	}

	for i := startIdx; i < len(ids) && len(result.Items) < limit+1; i++ {
		entry := r.notifications[ids[i]]
		if entry != nil {
			result.Items = append(result.Items, copyNotification(entry.notification))
		}
	}

	// Determine next cursor
	if len(result.Items) > limit {
		result.HasMore = true
		result.NextCursor = fmt.Sprintf("%d", result.Items[limit-1].CreatedAt.Unix())
		result.Items = result.Items[:limit]
	}

	return result, nil
}

// Ensure NotificationRepository implements interfaces.NotificationRepository
var _ interfaces.NotificationRepository = (*NotificationRepository)(nil)
