package notifications

import (
	"context"
	"testing"
	"time"

	notifpush "github.com/equaltoai/lesser/pkg/notifications"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockPublisher interface to access mock-specific methods
type MockPublisher interface {
	streaming.Publisher
	GetPublishedEvents() []streaming.MockPublishedEvent
	GetPublishedEventCount() int
	GetPublishedEventsForUser(userID string) []streaming.MockPublishedEvent
	Reset()
}

// MockNotificationRepository implements interfaces.NotificationRepository for testing
type MockNotificationRepository struct {
	mock.Mock
}

func (m *MockNotificationRepository) CreateNotification(ctx context.Context, notification *models.Notification) error {
	args := m.Called(ctx, notification)
	return args.Error(0)
}

func (m *MockNotificationRepository) GetNotification(ctx context.Context, notificationID string) (*models.Notification, error) {
	args := m.Called(ctx, notificationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Notification), args.Error(1)
}

func (m *MockNotificationRepository) GetUserNotification(ctx context.Context, userID, notificationID string) (*models.Notification, error) {
	args := m.Called(ctx, userID, notificationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Notification), args.Error(1)
}

func (m *MockNotificationRepository) UpdateNotification(ctx context.Context, notification *models.Notification) error {
	args := m.Called(ctx, notification)
	return args.Error(0)
}

func (m *MockNotificationRepository) DeleteNotification(ctx context.Context, notificationID string) error {
	args := m.Called(ctx, notificationID)
	return args.Error(0)
}

func (m *MockNotificationRepository) DeleteUserNotification(ctx context.Context, userID, notificationID string) error {
	args := m.Called(ctx, userID, notificationID)
	return args.Error(0)
}

func (m *MockNotificationRepository) GetUserNotifications(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Notification]), args.Error(1)
}

func (m *MockNotificationRepository) GetUnreadNotifications(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Notification]), args.Error(1)
}

func (m *MockNotificationRepository) GetNotificationsByType(ctx context.Context, userID, notificationType string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	args := m.Called(ctx, userID, notificationType, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Notification]), args.Error(1)
}

func (m *MockNotificationRepository) MarkNotificationRead(ctx context.Context, notificationID string) error {
	args := m.Called(ctx, notificationID)
	return args.Error(0)
}

func (m *MockNotificationRepository) MarkNotificationUnread(ctx context.Context, notificationID string) error {
	args := m.Called(ctx, notificationID)
	return args.Error(0)
}

func (m *MockNotificationRepository) MarkAllNotificationsRead(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockNotificationRepository) MarkNotificationsReadByType(ctx context.Context, userID, notificationType string) error {
	args := m.Called(ctx, userID, notificationType)
	return args.Error(0)
}

func (m *MockNotificationRepository) MarkNotificationPushSent(ctx context.Context, notificationID string) error {
	args := m.Called(ctx, notificationID)
	return args.Error(0)
}

func (m *MockNotificationRepository) MarkNotificationPushFailed(ctx context.Context, notificationID, errorMsg string) error {
	args := m.Called(ctx, notificationID, errorMsg)
	return args.Error(0)
}

func (m *MockNotificationRepository) GetPendingPushNotifications(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Notification]), args.Error(1)
}

func (m *MockNotificationRepository) GetNotificationGroups(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Notification]), args.Error(1)
}

func (m *MockNotificationRepository) ConsolidateNotifications(ctx context.Context, groupKey string) error {
	args := m.Called(ctx, groupKey)
	return args.Error(0)
}

func (m *MockNotificationRepository) GetUnreadNotificationCount(ctx context.Context, userID string) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockNotificationRepository) GetNotificationCountsByType(ctx context.Context, userID string) (map[string]int64, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int64), args.Error(1)
}

func (m *MockNotificationRepository) CreateNotifications(ctx context.Context, notifications []*models.Notification) error {
	args := m.Called(ctx, notifications)
	return args.Error(0)
}

func (m *MockNotificationRepository) DeleteNotificationsByType(ctx context.Context, userID, notificationType string) error {
	args := m.Called(ctx, userID, notificationType)
	return args.Error(0)
}

func (m *MockNotificationRepository) DeleteExpiredNotifications(ctx context.Context, expiredBefore time.Time) (int64, error) {
	args := m.Called(ctx, expiredBefore)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockNotificationRepository) SetDispatcher(dispatcher interfaces.NotificationDispatcher) {
	m.Called(dispatcher)
}

func (m *MockNotificationRepository) DeleteNotificationsByObject(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

func (m *MockNotificationRepository) GetNotificationsFiltered(ctx context.Context, username string, filter map[string]interface{}) ([]*models.Notification, string, error) {
	args := m.Called(ctx, username, filter)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Notification), args.String(1), args.Error(2)
}

func (m *MockNotificationRepository) ClearOldNotifications(ctx context.Context, username string, olderThan time.Time) (int, error) {
	args := m.Called(ctx, username, olderThan)
	return args.Int(0), args.Error(1)
}

func (m *MockNotificationRepository) GetNotificationsAdvanced(ctx context.Context, userID string, filters map[string]interface{}, pagination interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	args := m.Called(ctx, userID, filters, pagination)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Notification]), args.Error(1)
}

func (m *MockNotificationRepository) GetNotificationPreferences(ctx context.Context, userID string) (*models.NotificationPreferences, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.NotificationPreferences), args.Error(1)
}

func (m *MockNotificationRepository) UpdateNotificationPreferences(ctx context.Context, prefs *models.NotificationPreferences) error {
	args := m.Called(ctx, prefs)
	return args.Error(0)
}

func (m *MockNotificationRepository) SetNotificationPreference(ctx context.Context, userID string, preferenceType string, enabled bool) error {
	args := m.Called(ctx, userID, preferenceType, enabled)
	return args.Error(0)
}

// MockAccountRepository implements interfaces.AccountRepository for testing
type MockAccountRepository struct {
	mock.Mock
}

func (m *MockAccountRepository) GetAccount(ctx context.Context, username string) (*storage.Account, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Account), args.Error(1)
}

// Implement other required methods with basic mock behavior
func (m *MockAccountRepository) CreateAccount(ctx context.Context, account *storage.Account) error {
	args := m.Called(ctx, account)
	return args.Error(0)
}

func (m *MockAccountRepository) GetAccountByURL(ctx context.Context, actorURL string) (*storage.Account, error) {
	args := m.Called(ctx, actorURL)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Account), args.Error(1)
}

func (m *MockAccountRepository) GetAccountByEmail(ctx context.Context, email string) (*storage.Account, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Account), args.Error(1)
}

func (m *MockAccountRepository) UpdateAccount(ctx context.Context, account *storage.Account) error {
	args := m.Called(ctx, account)
	return args.Error(0)
}

func (m *MockAccountRepository) DeleteAccount(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockAccountRepository) SearchAccounts(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, query, opts)
	return nil, args.Error(1)
}

func (m *MockAccountRepository) GetSuggestedAccounts(ctx context.Context, forUserID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.AccountSuggestion], error) {
	args := m.Called(ctx, forUserID, opts)
	return nil, args.Error(1)
}

func (m *MockAccountRepository) GetFeaturedAccounts(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, opts)
	return nil, args.Error(1)
}

func (m *MockAccountRepository) ApproveAccount(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockAccountRepository) SuspendAccount(ctx context.Context, username string, reason string) error {
	args := m.Called(ctx, username, reason)
	return args.Error(0)
}

func (m *MockAccountRepository) UnsuspendAccount(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockAccountRepository) SilenceAccount(ctx context.Context, username string, reason string) error {
	args := m.Called(ctx, username, reason)
	return args.Error(0)
}

func (m *MockAccountRepository) UnsilenceAccount(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockAccountRepository) UpdateAccountPreferences(ctx context.Context, username string, preferences map[string]interface{}) error {
	args := m.Called(ctx, username, preferences)
	return args.Error(0)
}

func (m *MockAccountRepository) GetAccountPreferences(ctx context.Context, username string) (map[string]interface{}, error) {
	args := m.Called(ctx, username)
	return nil, args.Error(1)
}

func (m *MockAccountRepository) UpdateAccountFeatures(ctx context.Context, username string, features map[string]bool) error {
	args := m.Called(ctx, username, features)
	return args.Error(0)
}

func (m *MockAccountRepository) GetAccountFeatures(ctx context.Context, username string) (map[string]bool, error) {
	args := m.Called(ctx, username)
	return nil, args.Error(1)
}

func (m *MockAccountRepository) ValidateCredentials(ctx context.Context, username, password string) (*storage.Account, error) {
	args := m.Called(ctx, username, password)
	return nil, args.Error(1)
}

func (m *MockAccountRepository) UpdatePassword(ctx context.Context, username, newPasswordHash string) error {
	args := m.Called(ctx, username, newPasswordHash)
	return args.Error(0)
}

func (m *MockAccountRepository) CreatePasswordReset(ctx context.Context, reset *storage.PasswordReset) error {
	args := m.Called(ctx, reset)
	return args.Error(0)
}

func (m *MockAccountRepository) GetPasswordReset(ctx context.Context, token string) (*storage.PasswordReset, error) {
	args := m.Called(ctx, token)
	return nil, args.Error(1)
}

func (m *MockAccountRepository) UsePasswordReset(ctx context.Context, token string) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockAccountRepository) RecordLogin(ctx context.Context, attempt *storage.LoginAttempt) error {
	args := m.Called(ctx, attempt)
	return args.Error(0)
}

func (m *MockAccountRepository) GetLoginHistory(ctx context.Context, username string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.LoginAttempt], error) {
	args := m.Called(ctx, username, opts)
	return nil, args.Error(1)
}

func (m *MockAccountRepository) UpdateLastActivity(ctx context.Context, username string, activity time.Time) error {
	args := m.Called(ctx, username, activity)
	return args.Error(0)
}

func (m *MockAccountRepository) GetAccountsByUsernames(ctx context.Context, usernames []string) ([]*storage.Account, error) {
	args := m.Called(ctx, usernames)
	return nil, args.Error(1)
}

func (m *MockAccountRepository) GetAccountsCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockAccountRepository) AddBookmark(ctx context.Context, username, objectID string) error {
	args := m.Called(ctx, username, objectID)
	return args.Error(0)
}

func (m *MockAccountRepository) RemoveBookmark(ctx context.Context, username, objectID string) error {
	args := m.Called(ctx, username, objectID)
	return args.Error(0)
}

func (m *MockAccountRepository) GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]*storage.Bookmark, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Bookmark), args.String(1), args.Error(2)
}

func (m *MockAccountRepository) GetBookmarkedStatuses(ctx context.Context, username string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, username, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

// Helper functions for tests

type fakePushService struct {
	messages []*notifpush.PushMessage
	err      error
}

func (f *fakePushService) QueueNotification(_ context.Context, msg *notifpush.PushMessage) error {
	f.messages = append(f.messages, msg)
	return f.err
}

func (f *fakePushService) Messages() []*notifpush.PushMessage {
	return f.messages
}

func setupTestService() (*Service, *MockNotificationRepository, *MockAccountRepository, MockPublisher, *fakePushService) {
	mockNotificationRepo := new(MockNotificationRepository)
	mockAccountRepo := new(MockAccountRepository)
	mockPublisher := streaming.NewMockPublisher().(MockPublisher)
	pushService := &fakePushService{}

	service := NewService(
		mockNotificationRepo,
		mockAccountRepo,
		mockPublisher,
		zap.NewNop(),
		"example.com",
		pushService,
	)

	return service, mockNotificationRepo, mockAccountRepo, mockPublisher, pushService
}

func createTestUser(username string) *storage.Account {
	return &storage.Account{
		User: &storage.User{
			Username:    username,
			Email:       username + "@example.com",
			DisplayName: "Test User " + username,
			CreatedAt:   time.Now(),
			Approved:    true,
		},
	}
}

func createTestNotification(userID, notifType, actorID string) *models.Notification {
	return models.NewNotificationBuilder().
		ForUser(userID).
		OfType(notifType).
		FromActor(actorID, "user").
		WithContent("Test Notification", "This is a test notification").
		Build()
}

// Test CreateNotification

func TestService_CreateNotification_Success(t *testing.T) {
	service, mockNotificationRepo, mockAccountRepo, mockPublisher, pushService := setupTestService()
	ctx := context.Background()

	// Setup mocks
	user := createTestUser("testuser")
	actor := createTestUser("actor")

	mockAccountRepo.On("GetAccount", ctx, "testuser").Return(user, nil)
	mockAccountRepo.On("GetAccount", ctx, "actor").Return(actor, nil)
	mockNotificationRepo.On("CreateNotification", ctx, mock.AnythingOfType("*models.Notification")).Return(nil)

	cmd := &CreateNotificationCommand{
		UserID:     "testuser",
		Type:       "mention",
		ActorID:    "actor",
		ActorType:  "user",
		TargetID:   "status123",
		TargetType: "status",
		Title:      "You were mentioned",
		Body:       "Someone mentioned you in a post",
	}

	result, err := service.CreateNotification(ctx, cmd)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Notification)
	assert.Equal(t, "testuser", result.Notification.UserID)
	assert.Equal(t, "mention", result.Notification.Type)
	assert.Equal(t, "actor", result.Notification.ActorID)
	assert.Len(t, result.Events, 1)

	// Verify publisher was called
	publishedEvents := mockPublisher.GetPublishedEvents()
	assert.Len(t, publishedEvents, 1)
	assert.Equal(t, "notification.created", publishedEvents[0].Event.Type)
	assert.Equal(t, "testuser", publishedEvents[0].TargetID)

	assert.Len(t, pushService.Messages(), 1)
	pushMsg := pushService.Messages()[0]
	assert.Equal(t, "testuser", pushMsg.Username)
	assert.Equal(t, "mention", pushMsg.NotificationType)
	assert.Equal(t, "You were mentioned", pushMsg.Title)
	assert.Equal(t, "Someone mentioned you in a post", pushMsg.Body)
	assert.Equal(t, result.Notification.ID, pushMsg.NotificationID)

	mockNotificationRepo.AssertExpectations(t)
	mockAccountRepo.AssertExpectations(t)
}

func TestService_CreateNotification_ReplyType_Success(t *testing.T) {
	service, mockNotificationRepo, mockAccountRepo, _, pushService := setupTestService()
	ctx := context.Background()

	user := createTestUser("testuser")
	actor := createTestUser("actor")

	mockAccountRepo.On("GetAccount", ctx, "testuser").Return(user, nil)
	mockAccountRepo.On("GetAccount", ctx, "actor").Return(actor, nil)
	mockNotificationRepo.On("CreateNotification", ctx, mock.AnythingOfType("*models.Notification")).Return(nil)

	result, err := service.CreateNotification(ctx, &CreateNotificationCommand{
		UserID:     "testuser",
		Type:       "reply",
		ActorID:    "actor",
		ActorType:  "user",
		TargetID:   "status123",
		TargetType: "status",
		Title:      "New reply",
		Body:       "Someone replied to your post",
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "reply", result.Notification.Type)
	assert.Len(t, pushService.Messages(), 1)
	assert.Equal(t, "reply", pushService.Messages()[0].NotificationType)

	mockNotificationRepo.AssertExpectations(t)
	mockAccountRepo.AssertExpectations(t)
}

func TestService_CreateNotification_ValidationError(t *testing.T) {
	service, _, _, _, pushService := setupTestService()
	ctx := context.Background()

	// Test missing user_id
	cmd := &CreateNotificationCommand{
		Type:    "mention",
		ActorID: "actor",
	}

	result, err := service.CreateNotification(ctx, cmd)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Validation failed")
	assert.Len(t, pushService.Messages(), 0)
}

func TestService_CreateNotification_InvalidType(t *testing.T) {
	service, _, _, _, pushService := setupTestService()
	ctx := context.Background()

	cmd := &CreateNotificationCommand{
		UserID:  "testuser",
		Type:    "invalid_type",
		ActorID: "actor",
	}

	result, err := service.CreateNotification(ctx, cmd)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Validation failed")
	assert.Len(t, pushService.Messages(), 0)
}

func TestService_CreateNotification_UserNotFound(t *testing.T) {
	service, _, mockAccountRepo, _, pushService := setupTestService()
	ctx := context.Background()

	mockAccountRepo.On("GetAccount", ctx, "nonexistent").Return(nil, assert.AnError)

	cmd := &CreateNotificationCommand{
		UserID:  "nonexistent",
		Type:    "mention",
		ActorID: "actor",
	}

	result, err := service.CreateNotification(ctx, cmd)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Resource not found")

	mockAccountRepo.AssertExpectations(t)
	assert.Len(t, pushService.Messages(), 0)
}

// Test MarkAsRead

func TestService_MarkAsRead_Success(t *testing.T) {
	service, mockNotificationRepo, _, mockPublisher, _ := setupTestService()
	ctx := context.Background()

	notification := createTestNotification("testuser", "mention", "actor")
	notification.ID = "notif123"
	notification.IsRead = false

	mockNotificationRepo.On("GetUserNotification", ctx, "testuser", "notif123").Return(notification, nil)
	mockNotificationRepo.On("UpdateNotification", ctx, notification).Return(nil)

	cmd := &MarkAsReadCommand{
		NotificationID: "notif123",
		UserID:         "testuser",
	}

	result, err := service.MarkAsRead(ctx, cmd)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Notification.IsRead)
	assert.NotNil(t, result.Notification.ReadAt)
	assert.Len(t, result.Events, 1)

	// Verify publisher was called
	publishedEvents := mockPublisher.GetPublishedEvents()
	assert.Len(t, publishedEvents, 1)
	assert.Equal(t, "notification.read", publishedEvents[0].Event.Type)

	mockNotificationRepo.AssertExpectations(t)
}

func TestService_MarkAsRead_AlreadyRead(t *testing.T) {
	service, mockNotificationRepo, _, mockPublisher, _ := setupTestService()
	ctx := context.Background()

	notification := createTestNotification("testuser", "mention", "actor")
	notification.ID = "notif123"
	notification.IsRead = true
	readTime := time.Now()
	notification.ReadAt = &readTime

	mockNotificationRepo.On("GetUserNotification", ctx, "testuser", "notif123").Return(notification, nil)

	cmd := &MarkAsReadCommand{
		NotificationID: "notif123",
		UserID:         "testuser",
	}

	result, err := service.MarkAsRead(ctx, cmd)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Notification.IsRead)
	assert.Len(t, result.Events, 0) // No events for already read notification

	// Verify publisher was not called
	publishedEvents := mockPublisher.GetPublishedEvents()
	assert.Len(t, publishedEvents, 0)

	mockNotificationRepo.AssertExpectations(t)
}

func TestService_MarkAsRead_Unauthorized(t *testing.T) {
	service, mockNotificationRepo, _, _, _ := setupTestService()
	ctx := context.Background()

	mockNotificationRepo.On("GetUserNotification", ctx, "testuser", "notif123").Return(nil, storage.ErrNotFound)

	cmd := &MarkAsReadCommand{
		NotificationID: "notif123",
		UserID:         "testuser", // Different user
	}

	result, err := service.MarkAsRead(ctx, cmd)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "notification not found")

	mockNotificationRepo.AssertExpectations(t)
}

// Test ClearNotifications

func TestService_ClearNotifications_ClearAll(t *testing.T) {
	service, mockNotificationRepo, _, mockPublisher, _ := setupTestService()
	ctx := context.Background()

	mockNotificationRepo.On("MarkAllNotificationsRead", ctx, "testuser").Return(nil)

	cmd := &ClearCommand{
		UserID:   "testuser",
		ClearAll: true,
	}

	result, err := service.ClearNotifications(ctx, cmd)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Events, 1)

	// Verify publisher was called
	publishedEvents := mockPublisher.GetPublishedEvents()
	assert.Len(t, publishedEvents, 1)
	assert.Equal(t, "notification.cleared", publishedEvents[0].Event.Type)

	mockNotificationRepo.AssertExpectations(t)
}

func TestService_ClearNotifications_ByType(t *testing.T) {
	service, mockNotificationRepo, _, _, _ := setupTestService()
	ctx := context.Background()

	mockNotificationRepo.On("MarkNotificationsReadByType", ctx, "testuser", "mention").Return(nil)
	mockNotificationRepo.On("MarkNotificationsReadByType", ctx, "testuser", "follow").Return(nil)

	cmd := &ClearCommand{
		UserID: "testuser",
		Types:  []string{"mention", "follow"},
	}

	result, err := service.ClearNotifications(ctx, cmd)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(2), result.ClearedCount)
	assert.Len(t, result.Events, 1)

	mockNotificationRepo.AssertExpectations(t)
}

func TestService_ClearNotifications_SpecificIDs(t *testing.T) {
	service, mockNotificationRepo, _, _, _ := setupTestService()
	ctx := context.Background()

	notification1 := createTestNotification("testuser", "mention", "actor")
	notification1.ID = "notif1"
	notification2 := createTestNotification("testuser", "follow", "actor2")
	notification2.ID = "notif2"

	mockNotificationRepo.On("DeleteUserNotification", ctx, "testuser", "notif1").Return(nil)
	mockNotificationRepo.On("DeleteUserNotification", ctx, "testuser", "notif2").Return(nil)

	cmd := &ClearCommand{
		UserID:          "testuser",
		NotificationIDs: []string{"notif1", "notif2"},
	}

	result, err := service.ClearNotifications(ctx, cmd)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(2), result.ClearedCount)

	mockNotificationRepo.AssertExpectations(t)
}

func TestService_ClearNotifications_ValidationError(t *testing.T) {
	service, _, _, _, _ := setupTestService()
	ctx := context.Background()

	cmd := &ClearCommand{
		UserID: "testuser",
		// No clear criteria specified
	}

	result, err := service.ClearNotifications(ctx, cmd)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Validation failed")
}

// Test GetNotification

func TestService_GetNotification_Success(t *testing.T) {
	service, mockNotificationRepo, _, _, _ := setupTestService()
	ctx := context.Background()

	notification := createTestNotification("testuser", "mention", "actor")
	notification.ID = "notif123"

	mockNotificationRepo.On("GetUserNotification", ctx, "testuser", "notif123").Return(notification, nil)

	query := &GetNotificationQuery{
		NotificationID: "notif123",
		UserID:         "testuser",
	}

	result, err := service.GetNotification(ctx, query)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "notif123", result.ID)
	assert.Equal(t, "testuser", result.UserID)

	mockNotificationRepo.AssertExpectations(t)
}

func TestService_GetNotification_Unauthorized(t *testing.T) {
	service, mockNotificationRepo, _, _, _ := setupTestService()
	ctx := context.Background()

	mockNotificationRepo.On("GetUserNotification", ctx, "testuser", "notif123").Return(nil, storage.ErrNotFound)

	query := &GetNotificationQuery{
		NotificationID: "notif123",
		UserID:         "testuser", // Different user
	}

	result, err := service.GetNotification(ctx, query)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "notification not found")

	mockNotificationRepo.AssertExpectations(t)
}

// Test ListNotifications

func TestService_ListNotifications_AllNotifications(t *testing.T) {
	service, mockNotificationRepo, _, _, _ := setupTestService()
	ctx := context.Background()

	notifications := []*models.Notification{
		createTestNotification("testuser", "mention", "actor1"),
		createTestNotification("testuser", "follow", "actor2"),
	}

	paginatedResult := &interfaces.PaginatedResult[*models.Notification]{
		Items:      notifications,
		NextCursor: "",
		HasMore:    false,
		Total:      2,
	}

	mockNotificationRepo.On("GetUserNotifications", ctx, "testuser", mock.AnythingOfType("interfaces.PaginationOptions")).Return(paginatedResult, nil)
	mockNotificationRepo.On("GetUnreadNotificationCount", ctx, "testuser").Return(int64(1), nil)
	mockNotificationRepo.On("GetNotificationCountsByType", ctx, "testuser").Return(map[string]int64{
		"mention": 1,
		"follow":  1,
	}, nil)

	query := &ListNotificationsQuery{
		UserID:      "testuser",
		IncludeRead: true,
		Pagination: interfaces.PaginationOptions{
			Limit: 20,
		},
	}

	result, err := service.ListNotifications(ctx, query)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Notifications, 2)
	assert.NotNil(t, result.Summary)
	assert.Equal(t, int64(2), result.Summary.TotalCount)
	assert.Equal(t, int64(1), result.Summary.UnreadCount)

	mockNotificationRepo.AssertExpectations(t)
}

func TestService_ListNotifications_OnlyUnread(t *testing.T) {
	service, mockNotificationRepo, _, _, _ := setupTestService()
	ctx := context.Background()

	unreadNotification := createTestNotification("testuser", "mention", "actor")
	unreadNotification.IsRead = false

	notifications := []*models.Notification{unreadNotification}

	paginatedResult := &interfaces.PaginatedResult[*models.Notification]{
		Items:      notifications,
		NextCursor: "",
		HasMore:    false,
		Total:      1,
	}

	// Mock for the main query
	mockNotificationRepo.On("GetUnreadNotifications", ctx, "testuser", mock.AnythingOfType("interfaces.PaginationOptions")).Return(paginatedResult, nil)
	// Mock for summary statistics
	mockNotificationRepo.On("GetUnreadNotificationCount", ctx, "testuser").Return(int64(1), nil)
	mockNotificationRepo.On("GetNotificationCountsByType", ctx, "testuser").Return(map[string]int64{
		"mention": 1,
	}, nil)
	// Mock for getting last notification time in summary
	mockNotificationRepo.On("GetUserNotifications", ctx, "testuser", interfaces.PaginationOptions{Limit: 1}).Return(paginatedResult, nil)

	query := &ListNotificationsQuery{
		UserID:     "testuser",
		OnlyUnread: true,
		Pagination: interfaces.PaginationOptions{
			Limit: 20,
		},
	}

	result, err := service.ListNotifications(ctx, query)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Notifications, 1)
	assert.False(t, result.Notifications[0].IsRead)

	mockNotificationRepo.AssertExpectations(t)
}

func TestService_ListNotifications_FilterByType(t *testing.T) {
	service, mockNotificationRepo, _, _, _ := setupTestService()
	ctx := context.Background()

	mentionNotification := createTestNotification("testuser", "mention", "actor")

	notifications := []*models.Notification{mentionNotification}

	paginatedResult := &interfaces.PaginatedResult[*models.Notification]{
		Items:      notifications,
		NextCursor: "",
		HasMore:    false,
		Total:      1,
	}

	mockNotificationRepo.On("GetNotificationsByType", ctx, "testuser", "mention", mock.AnythingOfType("interfaces.PaginationOptions")).Return(paginatedResult, nil)
	mockNotificationRepo.On("GetUnreadNotificationCount", ctx, "testuser").Return(int64(1), nil)
	mockNotificationRepo.On("GetNotificationCountsByType", ctx, "testuser").Return(map[string]int64{
		"mention": 1,
	}, nil)
	// Mock for getting last notification time in summary
	mockNotificationRepo.On("GetUserNotifications", ctx, "testuser", interfaces.PaginationOptions{Limit: 1}).Return(paginatedResult, nil)

	query := &ListNotificationsQuery{
		UserID:      "testuser",
		Types:       []string{"mention"},
		IncludeRead: true,
		Pagination: interfaces.PaginationOptions{
			Limit: 20,
		},
	}

	result, err := service.ListNotifications(ctx, query)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Notifications, 1)
	assert.Equal(t, "mention", result.Notifications[0].Type)

	mockNotificationRepo.AssertExpectations(t)
}

// Test convenience functions

func TestService_CreateMentionNotification(t *testing.T) {
	service, mockNotificationRepo, mockAccountRepo, _, pushService := setupTestService()
	ctx := context.Background()

	user := createTestUser("testuser")
	actor := createTestUser("actor")

	mockAccountRepo.On("GetAccount", ctx, "testuser").Return(user, nil)
	mockAccountRepo.On("GetAccount", ctx, "actor").Return(actor, nil)
	mockNotificationRepo.On("CreateNotification", ctx, mock.AnythingOfType("*models.Notification")).Return(nil)

	cmd := &CreateNotificationCommand{
		UserID:     "testuser",
		Type:       "mention",
		ActorID:    "actor",
		ActorType:  "user",
		TargetID:   "status123",
		TargetType: "status",
		Title:      "New mention",
		Body:       "You were mentioned in a post",
	}

	result, err := service.CreateNotification(ctx, cmd)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "mention", result.Notification.Type)
	assert.Equal(t, "New mention", result.Notification.Title)

	assert.Len(t, pushService.Messages(), 1)

	mockNotificationRepo.AssertExpectations(t)
	mockAccountRepo.AssertExpectations(t)
}

// Test error cases

func TestService_CreateNotification_RepositoryError(t *testing.T) {
	service, mockNotificationRepo, mockAccountRepo, _, pushService := setupTestService()
	ctx := context.Background()

	user := createTestUser("testuser")
	actor := createTestUser("actor")

	mockAccountRepo.On("GetAccount", ctx, "testuser").Return(user, nil)
	mockAccountRepo.On("GetAccount", ctx, "actor").Return(actor, nil)
	mockNotificationRepo.On("CreateNotification", ctx, mock.AnythingOfType("*models.Notification")).Return(assert.AnError)

	cmd := &CreateNotificationCommand{
		UserID:  "testuser",
		Type:    "mention",
		ActorID: "actor",
	}

	result, err := service.CreateNotification(ctx, cmd)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Failed to create notification")

	mockNotificationRepo.AssertExpectations(t)
	mockAccountRepo.AssertExpectations(t)
	assert.Len(t, pushService.Messages(), 0)
}

func TestService_MarkAsRead_NotificationNotFound(t *testing.T) {
	service, mockNotificationRepo, _, _, _ := setupTestService()
	ctx := context.Background()

	mockNotificationRepo.On("GetUserNotification", ctx, "testuser", "nonexistent").Return(nil, assert.AnError)

	cmd := &MarkAsReadCommand{
		NotificationID: "nonexistent",
		UserID:         "testuser",
	}

	result, err := service.MarkAsRead(ctx, cmd)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "notification not found")

	mockNotificationRepo.AssertExpectations(t)
}
