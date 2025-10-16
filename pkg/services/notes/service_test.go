package notes

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap/zaptest"
)

// Mock implementations for testing

// MockPublisher interface for testing includes extra methods
type MockPublisher interface {
	streaming.Publisher
	GetPublishedEvents() []streaming.MockPublishedEvent
	Reset()
}

type MockNoteRepository struct {
	mock.Mock
}

func (m *MockNoteRepository) CreateStatus(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

func (m *MockNoteRepository) GetStatus(ctx context.Context, statusID string) (*models.Status, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0).(*models.Status), args.Error(1)
}

func (m *MockNoteRepository) GetStatusByURL(ctx context.Context, url string) (*models.Status, error) {
	args := m.Called(ctx, url)
	return args.Get(0).(*models.Status), args.Error(1)
}

func (m *MockNoteRepository) UpdateStatus(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

func (m *MockNoteRepository) DeleteStatus(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

func (m *MockNoteRepository) GetPublicTimeline(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) GetHomeTimeline(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, userID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) GetUserTimeline(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, userID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) GetConversationThread(ctx context.Context, conversationID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, conversationID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) GetReplies(ctx context.Context, parentStatusID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, parentStatusID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) SearchStatuses(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, query, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) GetStatusesByHashtag(ctx context.Context, hashtag string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, hashtag, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) GetTrendingStatuses(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) LikeStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

func (m *MockNoteRepository) UnlikeStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

func (m *MockNoteRepository) ReblogStatus(ctx context.Context, userID, statusID string, reblogStatusID string) error {
	args := m.Called(ctx, userID, statusID, reblogStatusID)
	return args.Error(0)
}

func (m *MockNoteRepository) UnreblogStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

func (m *MockNoteRepository) BookmarkStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

func (m *MockNoteRepository) UnbookmarkStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

func (m *MockNoteRepository) FlagStatus(ctx context.Context, statusID, reason string, reportedBy string) error {
	args := m.Called(ctx, statusID, reason, reportedBy)
	return args.Error(0)
}

func (m *MockNoteRepository) UnflagStatus(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

func (m *MockNoteRepository) GetFlaggedStatuses(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) GetStatusesByIDs(ctx context.Context, statusIDs []string) ([]*models.Status, error) {
	args := m.Called(ctx, statusIDs)
	return args.Get(0).([]*models.Status), args.Error(1)
}

func (m *MockNoteRepository) GetStatusCounts(ctx context.Context, statusID string) (likes, reblogs, replies int, err error) {
	args := m.Called(ctx, statusID)
	return args.Int(0), args.Int(1), args.Int(2), args.Error(3)
}

func (m *MockNoteRepository) GetStatusContext(ctx context.Context, statusID string) (ancestors, descendants []*models.Status, err error) {
	args := m.Called(ctx, statusID)
	return args.Get(0).([]*models.Status), args.Get(1).([]*models.Status), args.Error(2)
}

func (m *MockNoteRepository) GetStatusEngagement(ctx context.Context, statusID, userID string) (liked, reblogged, bookmarked bool, err error) {
	args := m.Called(ctx, statusID, userID)
	return args.Bool(0), args.Bool(1), args.Bool(2), args.Error(3)
}

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

// Add other required methods with basic implementations
func (m *MockAccountRepository) CreateAccount(ctx context.Context, account *storage.Account) error {
	args := m.Called(ctx, account)
	return args.Error(0)
}

func (m *MockAccountRepository) GetAccountByURL(ctx context.Context, actorURL string) (*storage.Account, error) {
	args := m.Called(ctx, actorURL)
	return args.Get(0).(*storage.Account), args.Error(1)
}

func (m *MockAccountRepository) GetAccountByEmail(ctx context.Context, email string) (*storage.Account, error) {
	args := m.Called(ctx, email)
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
	return args.Get(0).(*interfaces.PaginatedResult[*storage.Account]), args.Error(1)
}

func (m *MockAccountRepository) GetSuggestedAccounts(ctx context.Context, forUserID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.AccountSuggestion], error) {
	args := m.Called(ctx, forUserID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*storage.AccountSuggestion]), args.Error(1)
}

func (m *MockAccountRepository) GetFeaturedAccounts(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*storage.Account]), args.Error(1)
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
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

func (m *MockAccountRepository) UpdateAccountFeatures(ctx context.Context, username string, features map[string]bool) error {
	args := m.Called(ctx, username, features)
	return args.Error(0)
}

func (m *MockAccountRepository) GetAccountFeatures(ctx context.Context, username string) (map[string]bool, error) {
	args := m.Called(ctx, username)
	return args.Get(0).(map[string]bool), args.Error(1)
}

func (m *MockAccountRepository) ValidateCredentials(ctx context.Context, username, password string) (*storage.Account, error) {
	args := m.Called(ctx, username, password)
	return args.Get(0).(*storage.Account), args.Error(1)
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
	return args.Get(0).(*storage.PasswordReset), args.Error(1)
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
	return args.Get(0).(*interfaces.PaginatedResult[*storage.LoginAttempt]), args.Error(1)
}

func (m *MockAccountRepository) UpdateLastActivity(ctx context.Context, username string, activity time.Time) error {
	args := m.Called(ctx, username, activity)
	return args.Error(0)
}

func (m *MockAccountRepository) GetAccountsByUsernames(ctx context.Context, usernames []string) ([]*storage.Account, error) {
	args := m.Called(ctx, usernames)
	return args.Get(0).([]*storage.Account), args.Error(1)
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

type MockFederationService struct {
	mock.Mock
}

func (m *MockFederationService) QueueActivity(ctx context.Context, activity *activitypub.Activity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

// MockAnalyticsService implements AnalyticsService for testing
type MockAnalyticsService struct {
	mock.Mock
}

func (m *MockAnalyticsService) RecordStatusCreation(ctx context.Context, actorID string, timestamp time.Time) error {
	args := m.Called(ctx, actorID, timestamp)
	return args.Error(0)
}

func (m *MockAnalyticsService) RecordHashtagUsage(ctx context.Context, hashtags []string, objectID, actorID string) error {
	args := m.Called(ctx, hashtags, objectID, actorID)
	return args.Error(0)
}

func (m *MockAnalyticsService) RecordLinkShare(ctx context.Context, links []string, objectID, actorID string) error {
	args := m.Called(ctx, links, objectID, actorID)
	return args.Error(0)
}

func (m *MockAnalyticsService) RecordEngagement(ctx context.Context, objectID, engagementType, actorID string) error {
	args := m.Called(ctx, objectID, engagementType, actorID)
	return args.Error(0)
}

func (m *MockAnalyticsService) RecordInstanceActivity(ctx context.Context, activityType string, timestamp time.Time) error {
	args := m.Called(ctx, activityType, timestamp)
	return args.Error(0)
}

// MockLikeRepository implements interfaces.LikeRepository
type MockLikeRepository struct {
	mock.Mock
}

func (m *MockLikeRepository) CreateLike(ctx context.Context, actor, object, statusAuthorID string) (*models.Like, error) {
	args := m.Called(ctx, actor, object, statusAuthorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Like), args.Error(1)
}

func (m *MockLikeRepository) DeleteLike(ctx context.Context, actor, object string) error {
	args := m.Called(ctx, actor, object)
	return args.Error(0)
}

func (m *MockLikeRepository) GetObjectLikes(ctx context.Context, objectID string, limit int, cursor string) ([]*models.Like, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Like), args.String(1), args.Error(2)
}

func (m *MockLikeRepository) GetActorLikes(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Like, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Like), args.String(1), args.Error(2)
}

// MockSocialRepository implements interfaces.SocialRepository
type MockSocialRepository struct {
	mock.Mock
}

func (m *MockSocialRepository) CreateAnnounce(ctx context.Context, announce *storage.Announce) error {
	args := m.Called(ctx, announce)
	return args.Error(0)
}

func (m *MockSocialRepository) DeleteAnnounce(ctx context.Context, actor, object string) error {
	args := m.Called(ctx, actor, object)
	return args.Error(0)
}

func (m *MockSocialRepository) GetStatusAnnounces(ctx context.Context, statusID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	args := m.Called(ctx, statusID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Announce), args.String(1), args.Error(2)
}

func (m *MockSocialRepository) CreateStatusPin(ctx context.Context, pin *storage.StatusPin) error {
	args := m.Called(ctx, pin)
	return args.Error(0)
}

func (m *MockSocialRepository) DeleteStatusPin(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

// MockConversationRepository implements interfaces.ConversationRepository
type MockConversationRepository struct {
	mock.Mock
}

func (m *MockConversationRepository) CreateConversation(ctx context.Context, conversation *models.Conversation, participants []string) error {
	args := m.Called(ctx, conversation, participants)
	return args.Error(0)
}

func (m *MockConversationRepository) GetConversation(ctx context.Context, conversationID string) (*models.Conversation, error) {
	args := m.Called(ctx, conversationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Conversation), args.Error(1)
}

func (m *MockConversationRepository) UpdateConversation(ctx context.Context, conversation *models.Conversation) error {
	args := m.Called(ctx, conversation)
	return args.Error(0)
}

func (m *MockConversationRepository) DeleteConversation(ctx context.Context, conversationID string) error {
	args := m.Called(ctx, conversationID)
	return args.Error(0)
}

func (m *MockConversationRepository) GetUserConversations(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Conversation]), args.Error(1)
}

func (m *MockConversationRepository) GetConversationByParticipants(ctx context.Context, participants []string) (*models.Conversation, error) {
	args := m.Called(ctx, participants)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Conversation), args.Error(1)
}

func (m *MockConversationRepository) AddParticipant(ctx context.Context, conversationID, participantID string) error {
	args := m.Called(ctx, conversationID, participantID)
	return args.Error(0)
}

func (m *MockConversationRepository) RemoveParticipant(ctx context.Context, conversationID, participantID string) error {
	args := m.Called(ctx, conversationID, participantID)
	return args.Error(0)
}

func (m *MockConversationRepository) GetConversationParticipants(ctx context.Context, conversationID string) ([]string, error) {
	args := m.Called(ctx, conversationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockConversationRepository) MarkConversationRead(ctx context.Context, conversationID, userID string) error {
	args := m.Called(ctx, conversationID, userID)
	return args.Error(0)
}

func (m *MockConversationRepository) MarkConversationUnread(ctx context.Context, conversationID, userID string) error {
	args := m.Called(ctx, conversationID, userID)
	return args.Error(0)
}

func (m *MockConversationRepository) GetUnreadConversations(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Conversation]), args.Error(1)
}

func (m *MockConversationRepository) SearchConversations(ctx context.Context, userID, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	args := m.Called(ctx, userID, query, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Conversation]), args.Error(1)
}

func (m *MockConversationRepository) CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error {
	args := m.Called(ctx, mute)
	return args.Error(0)
}

func (m *MockConversationRepository) DeleteConversationMute(ctx context.Context, username, conversationID string) error {
	args := m.Called(ctx, username, conversationID)
	return args.Error(0)
}

// Additional mock repositories needed for Notes service
type MockObjectRepository struct {
	mock.Mock
}

type MockSearchRepository struct {
	mock.Mock
}

type MockCommunityNoteRepository struct {
	mock.Mock
}

type MockUserRepository struct {
	mock.Mock
}

// Test helper functions

func createTestService() (*Service, *MockNoteRepository, *MockAccountRepository, streaming.Publisher, *MockFederationService) {
	// Create a version that uses interfaces - we'll need to modify the service constructor
	// For now, let's create repository-like mocks that satisfy the constructor

	// Create mocks that will work for testing
	noteRepo := &MockNoteRepository{}
	accountRepo := &MockAccountRepository{}
	publisher := streaming.NewMockPublisher()
	analytics := &MockAnalyticsService{}
	federation := &MockFederationService{}
	logger := zaptest.NewLogger(&testing.T{})

	// Create minimal repository instances - these might not be used in the actual tests
	likeRepo := &repositories.LikeRepository{}
	objectRepo := &repositories.ObjectRepository{}
	searchRepo := &repositories.SearchRepository{}
	communityNoteRepo := &repositories.CommunityNoteRepository{}
	userRepo := &repositories.UserRepository{}

	// Create concrete relationship repository for testing
	relationshipRepo := &repositories.RelationshipRepository{}

	// Use mock interfaces for interface types
	socialRepo := &MockSocialRepository{}
	conversationRepo := &MockConversationRepository{}

	// For testing, we'll use nil repositories since most tests focus on business logic
	// The individual repository methods will need separate testing
	service := NewService(
		nil, // StatusRepository - tests focus on business logic, not repository implementation
		accountRepo,
		relationshipRepo, // Add the concrete relationship repository
		likeRepo,
		socialRepo,
		conversationRepo,
		objectRepo,
		searchRepo,
		communityNoteRepo,
		userRepo,
		nil, // pollRepo - Add missing PollRepository parameter
		publisher,
		analytics, // Analytics service
		federation,
		logger,
		"example.com",
	)

	return service, noteRepo, accountRepo, publisher, federation
}

func createTestAccount() *storage.Account {
	return &storage.Account{
		User: &storage.User{
			Username:    "testuser",
			DisplayName: "Test User",
			Email:       "test@example.com",
			CreatedAt:   time.Now(),
		},
	}
}

func createTestStatus() *models.Status {
	now := time.Now()
	return &models.Status{
		StatusID:       "test123",
		AuthorID:       "testuser",
		AuthorUsername: "testuser",
		Content:        "Test content",
		Visibility:     models.VisibilityPublic,
		PublishedAt:    now,
		CreatedAt:      now,
		ModifiedAt:     now,
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				Type:      "Note",
				ID:        "https://example.com/users/testuser/statuses/test123",
				Published: &now,
				To:        []string{"https://www.w3.org/ns/activitystreams#Public"},
			},
			AttributedTo: "https://example.com/users/testuser",
			Content:      "Test content",
		},
	}
}

// Tests

func TestCreateNote_Success(t *testing.T) {
	service, noteRepo, accountRepo, _, federation := createTestService()
	ctx := context.Background()

	// Setup mocks
	account := createTestAccount()
	accountRepo.On("GetAccount", ctx, "testuser").Return(account, nil)
	noteRepo.On("CreateStatus", ctx, mock.AnythingOfType("*models.Status")).Return(nil)
	federation.On("QueueActivity", ctx, mock.AnythingOfType("*activitypub.Activity")).Return(nil)

	// Create command
	cmd := &CreateNoteCommand{
		AuthorID:   "testuser",
		Content:    "Hello, world!",
		Visibility: models.VisibilityPublic,
	}

	// Execute
	result, err := service.CreateNote(ctx, cmd)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Note)
	assert.Equal(t, "testuser", result.Note.AuthorID)
	assert.Equal(t, "Hello, world!", result.Note.Content)
	assert.Equal(t, models.VisibilityPublic, result.Note.Visibility)
	assert.NotEmpty(t, result.Note.StatusID)
	assert.True(t, len(result.Events) > 0)

	// Verify mocks
	noteRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
	federation.AssertExpectations(t)

	// Just verify no errors occurred during publishing
	// The streaming package will log any issues
	// Note: In a production test, we would use a mock with exported methods
}

func TestCreateNote_ValidationError(t *testing.T) {
	service, _, _, _, _ := createTestService()
	ctx := context.Background()

	tests := []struct {
		name    string
		cmd     *CreateNoteCommand
		wantErr string
	}{
		{
			name: "missing author",
			cmd: &CreateNoteCommand{
				Content:    "Hello, world!",
				Visibility: models.VisibilityPublic,
			},
			wantErr: "author_id is required",
		},
		{
			name: "empty content",
			cmd: &CreateNoteCommand{
				AuthorID:   "testuser",
				Content:    "   ",
				Visibility: models.VisibilityPublic,
			},
			wantErr: "is required",
		},
		{
			name: "content too long",
			cmd: &CreateNoteCommand{
				AuthorID:   "testuser",
				Content:    string(make([]byte, 5001)),
				Visibility: models.VisibilityPublic,
			},
			wantErr: "content too long",
		},
		{
			name: "invalid visibility",
			cmd: &CreateNoteCommand{
				AuthorID:   "testuser",
				Content:    "Hello, world!",
				Visibility: "invalid",
			},
			wantErr: "invalid visibility",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.CreateNote(ctx, tt.cmd)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestUpdateNote_Success(t *testing.T) {
	service, noteRepo, _, _, federation := createTestService()
	ctx := context.Background()

	// Setup existing status
	status := createTestStatus()
	status.AuthorID = "testuser"

	noteRepo.On("GetStatus", ctx, "test123").Return(status, nil)
	noteRepo.On("UpdateStatus", ctx, mock.AnythingOfType("*models.Status")).Return(nil)
	federation.On("QueueActivity", ctx, mock.AnythingOfType("*activitypub.Activity")).Return(nil)

	// Create command
	cmd := &UpdateNoteCommand{
		StatusID:  "test123",
		UpdaterID: "testuser",
		Content:   "Updated content",
	}

	// Execute
	result, err := service.UpdateNote(ctx, cmd)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "Updated content", result.Note.Content)
	assert.True(t, len(result.Events) > 0)

	// Verify mocks
	noteRepo.AssertExpectations(t)
	federation.AssertExpectations(t)
}

func TestUpdateNote_UnauthorizedUser(t *testing.T) {
	service, noteRepo, _, _, _ := createTestService()
	ctx := context.Background()

	// Setup existing status
	status := createTestStatus()
	status.AuthorID = "testuser"

	noteRepo.On("GetStatus", ctx, "test123").Return(status, nil)

	// Create command with different user
	cmd := &UpdateNoteCommand{
		StatusID:  "test123",
		UpdaterID: "otheruser",
		Content:   "Updated content",
	}

	// Execute
	_, err := service.UpdateNote(ctx, cmd)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Access denied")

	// Verify mocks
	noteRepo.AssertExpectations(t)
}

func TestDeleteNote_Success(t *testing.T) {
	service, noteRepo, _, _, federation := createTestService()
	ctx := context.Background()

	// Setup existing status
	status := createTestStatus()
	status.AuthorID = "testuser"

	noteRepo.On("GetStatus", ctx, "test123").Return(status, nil)
	noteRepo.On("UpdateStatus", ctx, mock.AnythingOfType("*models.Status")).Return(nil)
	federation.On("QueueActivity", ctx, mock.AnythingOfType("*activitypub.Activity")).Return(nil)

	// Create command
	cmd := &DeleteNoteCommand{
		StatusID:  "test123",
		DeleterID: "testuser",
	}

	// Execute
	err := service.DeleteNote(ctx, cmd)

	// Assert
	assert.NoError(t, err)

	// Verify the status was marked as deleted
	updateCall := noteRepo.Calls[1]
	updatedStatus := updateCall.Arguments[1].(*models.Status)
	assert.True(t, updatedStatus.Deleted)
	assert.NotNil(t, updatedStatus.DeletedAt)

	// Verify mocks
	noteRepo.AssertExpectations(t)
	federation.AssertExpectations(t)
}

func TestGetNote_Success(t *testing.T) {
	service, noteRepo, _, _, _ := createTestService()
	ctx := context.Background()

	// Setup status
	status := createTestStatus()
	noteRepo.On("GetStatus", ctx, "test123").Return(status, nil)

	// Create query
	query := &GetNoteQuery{
		StatusID: "test123",
		ViewerID: "testuser",
	}

	// Execute
	result, err := service.GetNote(ctx, query.StatusID)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test123", result.StatusID)

	// Verify mocks
	noteRepo.AssertExpectations(t)
}

func TestGetNote_Deleted(t *testing.T) {
	service, noteRepo, _, _, _ := createTestService()
	ctx := context.Background()

	// Setup deleted status
	status := createTestStatus()
	status.Deleted = true
	noteRepo.On("GetStatus", ctx, "test123").Return(status, nil)

	// Create query
	query := &GetNoteQuery{
		StatusID: "test123",
		ViewerID: "testuser",
	}

	// Execute
	_, err := service.GetNote(ctx, query.StatusID)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status not found")

	// Verify mocks
	noteRepo.AssertExpectations(t)
}

func TestListNotes_PublicTimeline(t *testing.T) {
	service, noteRepo, _, _, _ := createTestService()
	ctx := context.Background()

	// Setup response
	status := createTestStatus()
	result := &interfaces.PaginatedResult[*models.Status]{
		Items:      []*models.Status{status},
		NextCursor: "next123",
		HasMore:    true,
		Total:      1,
	}

	noteRepo.On("GetPublicTimeline", ctx, mock.AnythingOfType("interfaces.PaginationOptions")).Return(result, nil)

	// Create query
	query := &ListNotesQuery{
		TimelineType: "public",
		ViewerID:     "testuser",
		Pagination:   interfaces.PaginationOptions{Limit: 20},
	}

	// Execute
	notesResult, err := service.ListNotes(ctx, query)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, notesResult)
	assert.Len(t, notesResult.Notes, 1)
	assert.Equal(t, "test123", notesResult.Notes[0].StatusID)
	assert.True(t, notesResult.Pagination.HasMore)

	// Verify mocks
	noteRepo.AssertExpectations(t)
}

func TestListNotes_HomeTimelineRequiresViewerID(t *testing.T) {
	service, _, _, _, _ := createTestService()
	ctx := context.Background()

	// Create query without viewer ID
	query := &ListNotesQuery{
		TimelineType: "home",
		Pagination:   interfaces.PaginationOptions{Limit: 20},
	}

	// Execute
	_, err := service.ListNotes(ctx, query)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "home timeline requires viewer_id")
}

func TestListNotes_UnsupportedTimelineType(t *testing.T) {
	service, _, _, _, _ := createTestService()
	ctx := context.Background()

	// Create query with unsupported timeline type
	query := &ListNotesQuery{
		TimelineType: "unsupported",
		ViewerID:     "testuser",
		Pagination:   interfaces.PaginationOptions{Limit: 20},
	}

	// Execute
	_, err := service.ListNotes(ctx, query)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported timeline type")
}

func TestCreateNote_WithHashtags(t *testing.T) {
	service, noteRepo, accountRepo, _, federation := createTestService()
	ctx := context.Background()

	// Setup mocks
	account := createTestAccount()
	accountRepo.On("GetAccount", ctx, "testuser").Return(account, nil)
	noteRepo.On("CreateStatus", ctx, mock.AnythingOfType("*models.Status")).Return(nil)
	federation.On("QueueActivity", ctx, mock.AnythingOfType("*activitypub.Activity")).Return(nil)

	// Create command with hashtags
	cmd := &CreateNoteCommand{
		AuthorID:   "testuser",
		Content:    "Hello #world #test",
		Visibility: models.VisibilityPublic,
	}

	// Execute
	result, err := service.CreateNote(ctx, cmd)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Verify events were emitted (hashtag processing)
	// Note: In production tests, we would verify hashtag stream publishing
}

func TestCreateNote_DirectMessage(t *testing.T) {
	service, noteRepo, accountRepo, _, federation := createTestService()
	ctx := context.Background()

	// Setup mocks
	account := createTestAccount()
	accountRepo.On("GetAccount", ctx, "testuser").Return(account, nil)
	noteRepo.On("CreateStatus", ctx, mock.AnythingOfType("*models.Status")).Return(nil)
	federation.On("QueueActivity", ctx, mock.AnythingOfType("*activitypub.Activity")).Return(nil)

	// Create direct message command
	cmd := &CreateNoteCommand{
		AuthorID:       "testuser",
		Content:        "Direct message",
		Visibility:     models.VisibilityDirect,
		ConversationID: "conv123",
	}

	// Execute
	result, err := service.CreateNote(ctx, cmd)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, models.VisibilityDirect, result.Note.Visibility)

	// Verify direct message handling
	// Note: In production tests, we would verify conversation stream publishing
	// and confirm public stream is NOT used for direct messages
}

// Benchmark tests

func BenchmarkCreateNote(b *testing.B) {
	service, noteRepo, accountRepo, _, federation := createTestService()
	ctx := context.Background()

	// Setup mocks
	account := createTestAccount()
	accountRepo.On("GetAccount", ctx, "testuser").Return(account, nil)
	noteRepo.On("CreateStatus", ctx, mock.AnythingOfType("*models.Status")).Return(nil)
	federation.On("QueueActivity", ctx, mock.AnythingOfType("*activitypub.Activity")).Return(nil)

	cmd := &CreateNoteCommand{
		AuthorID:   "testuser",
		Content:    "Benchmark test content",
		Visibility: models.VisibilityPublic,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.CreateNote(ctx, cmd)
	}
}

func BenchmarkGetNote(b *testing.B) {
	service, noteRepo, _, _, _ := createTestService()
	ctx := context.Background()

	status := createTestStatus()
	noteRepo.On("GetStatus", ctx, "test123").Return(status, nil)

	query := &GetNoteQuery{
		StatusID: "test123",
		ViewerID: "testuser",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.GetNote(ctx, query.StatusID)
	}
}
