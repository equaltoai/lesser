// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockNotificationRepository is a mock implementation of interfaces.NotificationRepository
// using testify/mock for expectation-based testing.
type MockNotificationRepository struct {
	mock.Mock
}

// NewMockNotificationRepository creates a new mock notification repository
func NewMockNotificationRepository() *MockNotificationRepository {
	return &MockNotificationRepository{}
}

// SetDispatcher mocks the SetDispatcher method
func (m *MockNotificationRepository) SetDispatcher(dispatcher interfaces.NotificationDispatcher) {
	m.Called(dispatcher)
}

// Core notification operations

// CreateNotification mocks the CreateNotification method
func (m *MockNotificationRepository) CreateNotification(ctx context.Context, notification *models.Notification) error {
	args := m.Called(ctx, notification)
	return args.Error(0)
}

// GetNotification mocks the GetNotification method
func (m *MockNotificationRepository) GetNotification(ctx context.Context, notificationID string) (*models.Notification, error) {
	args := m.Called(ctx, notificationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Notification), args.Error(1)
}

// UpdateNotification mocks the UpdateNotification method
func (m *MockNotificationRepository) UpdateNotification(ctx context.Context, notification *models.Notification) error {
	args := m.Called(ctx, notification)
	return args.Error(0)
}

// DeleteNotification mocks the DeleteNotification method
func (m *MockNotificationRepository) DeleteNotification(ctx context.Context, notificationID string) error {
	args := m.Called(ctx, notificationID)
	return args.Error(0)
}


// User notification queries

// GetUserNotifications mocks the GetUserNotifications method
func (m *MockNotificationRepository) GetUserNotifications(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Notification]), args.Error(1)
}

// GetUnreadNotifications mocks the GetUnreadNotifications method
func (m *MockNotificationRepository) GetUnreadNotifications(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Notification]), args.Error(1)
}

// GetNotificationsByType mocks the GetNotificationsByType method
func (m *MockNotificationRepository) GetNotificationsByType(ctx context.Context, userID, notificationType string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	args := m.Called(ctx, userID, notificationType, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Notification]), args.Error(1)
}

// Notification status management

// MarkNotificationRead mocks the MarkNotificationRead method
func (m *MockNotificationRepository) MarkNotificationRead(ctx context.Context, notificationID string) error {
	args := m.Called(ctx, notificationID)
	return args.Error(0)
}

// MarkNotificationUnread mocks the MarkNotificationUnread method
func (m *MockNotificationRepository) MarkNotificationUnread(ctx context.Context, notificationID string) error {
	args := m.Called(ctx, notificationID)
	return args.Error(0)
}

// MarkAllNotificationsRead mocks the MarkAllNotificationsRead method
func (m *MockNotificationRepository) MarkAllNotificationsRead(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

// MarkNotificationsReadByType mocks the MarkNotificationsReadByType method
func (m *MockNotificationRepository) MarkNotificationsReadByType(ctx context.Context, userID, notificationType string) error {
	args := m.Called(ctx, userID, notificationType)
	return args.Error(0)
}

// Push notification tracking

// MarkNotificationPushSent mocks the MarkNotificationPushSent method
func (m *MockNotificationRepository) MarkNotificationPushSent(ctx context.Context, notificationID string) error {
	args := m.Called(ctx, notificationID)
	return args.Error(0)
}

// MarkNotificationPushFailed mocks the MarkNotificationPushFailed method
func (m *MockNotificationRepository) MarkNotificationPushFailed(ctx context.Context, notificationID, errorMsg string) error {
	args := m.Called(ctx, notificationID, errorMsg)
	return args.Error(0)
}

// GetPendingPushNotifications mocks the GetPendingPushNotifications method
func (m *MockNotificationRepository) GetPendingPushNotifications(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Notification]), args.Error(1)
}


// Notification grouping and consolidation

// GetNotificationGroups mocks the GetNotificationGroups method
func (m *MockNotificationRepository) GetNotificationGroups(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Notification]), args.Error(1)
}

// ConsolidateNotifications mocks the ConsolidateNotifications method
func (m *MockNotificationRepository) ConsolidateNotifications(ctx context.Context, groupKey string) error {
	args := m.Called(ctx, groupKey)
	return args.Error(0)
}

// Notification counts and summaries

// GetUnreadNotificationCount mocks the GetUnreadNotificationCount method
func (m *MockNotificationRepository) GetUnreadNotificationCount(ctx context.Context, userID string) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

// GetNotificationCountsByType mocks the GetNotificationCountsByType method
func (m *MockNotificationRepository) GetNotificationCountsByType(ctx context.Context, userID string) (map[string]int64, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int64), args.Error(1)
}

// Batch operations

// CreateNotifications mocks the CreateNotifications method
func (m *MockNotificationRepository) CreateNotifications(ctx context.Context, notifications []*models.Notification) error {
	args := m.Called(ctx, notifications)
	return args.Error(0)
}

// DeleteNotificationsByType mocks the DeleteNotificationsByType method
func (m *MockNotificationRepository) DeleteNotificationsByType(ctx context.Context, userID, notificationType string) error {
	args := m.Called(ctx, userID, notificationType)
	return args.Error(0)
}

// DeleteNotificationsByObject mocks the DeleteNotificationsByObject method
func (m *MockNotificationRepository) DeleteNotificationsByObject(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// DeleteExpiredNotifications mocks the DeleteExpiredNotifications method
func (m *MockNotificationRepository) DeleteExpiredNotifications(ctx context.Context, expiredBefore time.Time) (int64, error) {
	args := m.Called(ctx, expiredBefore)
	return args.Get(0).(int64), args.Error(1)
}


// Filtered and advanced queries

// GetNotificationsFiltered mocks the GetNotificationsFiltered method
func (m *MockNotificationRepository) GetNotificationsFiltered(ctx context.Context, username string, filter map[string]interface{}) ([]*models.Notification, string, error) {
	args := m.Called(ctx, username, filter)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Notification), args.String(1), args.Error(2)
}

// ClearOldNotifications mocks the ClearOldNotifications method
func (m *MockNotificationRepository) ClearOldNotifications(ctx context.Context, username string, olderThan time.Time) (int, error) {
	args := m.Called(ctx, username, olderThan)
	return args.Int(0), args.Error(1)
}

// GetNotificationsAdvanced mocks the GetNotificationsAdvanced method
func (m *MockNotificationRepository) GetNotificationsAdvanced(ctx context.Context, userID string, filters map[string]interface{}, pagination interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	args := m.Called(ctx, userID, filters, pagination)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Notification]), args.Error(1)
}

// Notification preferences

// GetNotificationPreferences mocks the GetNotificationPreferences method
func (m *MockNotificationRepository) GetNotificationPreferences(ctx context.Context, userID string) (*models.NotificationPreferences, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.NotificationPreferences), args.Error(1)
}

// UpdateNotificationPreferences mocks the UpdateNotificationPreferences method
func (m *MockNotificationRepository) UpdateNotificationPreferences(ctx context.Context, prefs *models.NotificationPreferences) error {
	args := m.Called(ctx, prefs)
	return args.Error(0)
}

// SetNotificationPreference mocks the SetNotificationPreference method
func (m *MockNotificationRepository) SetNotificationPreference(ctx context.Context, userID string, preferenceType string, enabled bool) error {
	args := m.Called(ctx, userID, preferenceType, enabled)
	return args.Error(0)
}

// Ensure MockNotificationRepository implements interfaces.NotificationRepository
var _ interfaces.NotificationRepository = (*MockNotificationRepository)(nil)
