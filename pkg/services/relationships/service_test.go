package relationships

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// Mock implementations for testing

type MockRelationshipRepository struct {
	mock.Mock
}

func (m *MockRelationshipRepository) CreateFollowRequest(ctx context.Context, followerID, followingID string) error {
	args := m.Called(ctx, followerID, followingID)
	return args.Error(0)
}

func (m *MockRelationshipRepository) AcceptFollowRequest(ctx context.Context, followerID, followingID string) error {
	args := m.Called(ctx, followerID, followingID)
	return args.Error(0)
}

func (m *MockRelationshipRepository) RejectFollowRequest(ctx context.Context, followerID, followingID string) error {
	args := m.Called(ctx, followerID, followingID)
	return args.Error(0)
}

func (m *MockRelationshipRepository) Unfollow(ctx context.Context, followerID, followingID string) error {
	args := m.Called(ctx, followerID, followingID)
	return args.Error(0)
}

func (m *MockRelationshipRepository) IsFollowing(ctx context.Context, followerID, followingID string) (bool, error) {
	args := m.Called(ctx, followerID, followingID)
	return args.Bool(0), args.Error(1)
}

func (m *MockRelationshipRepository) GetFollowStatus(ctx context.Context, followerID, followingID string) (string, error) {
	args := m.Called(ctx, followerID, followingID)
	return args.String(0), args.Error(1)
}

func (m *MockRelationshipRepository) GetFollowRelationship(ctx context.Context, followerID, followingID string) (*models.RelationshipRecord, error) {
	args := m.Called(ctx, followerID, followingID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.RelationshipRecord), args.Error(1)
}

func (m *MockRelationshipRepository) BlockUser(ctx context.Context, blockerID, blockedID string) error {
	args := m.Called(ctx, blockerID, blockedID)
	return args.Error(0)
}

func (m *MockRelationshipRepository) UnblockUser(ctx context.Context, blockerID, blockedID string) error {
	args := m.Called(ctx, blockerID, blockedID)
	return args.Error(0)
}

func (m *MockRelationshipRepository) IsBlocked(ctx context.Context, blockerID, blockedID string) (bool, error) {
	args := m.Called(ctx, blockerID, blockedID)
	return args.Bool(0), args.Error(1)
}

func (m *MockRelationshipRepository) MuteUser(ctx context.Context, muterID, mutedID string) error {
	args := m.Called(ctx, muterID, mutedID)
	return args.Error(0)
}

func (m *MockRelationshipRepository) UnmuteUser(ctx context.Context, muterID, mutedID string) error {
	args := m.Called(ctx, muterID, mutedID)
	return args.Error(0)
}

func (m *MockRelationshipRepository) IsMuted(ctx context.Context, muterID, mutedID string) (bool, error) {
	args := m.Called(ctx, muterID, mutedID)
	return args.Bool(0), args.Error(1)
}

// Implement remaining interface methods with minimal mocks
func (m *MockRelationshipRepository) GetFollowers(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, userID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*storage.Account]), args.Error(1)
}

func (m *MockRelationshipRepository) GetFollowing(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, userID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*storage.Account]), args.Error(1)
}

func (m *MockRelationshipRepository) GetFollowRequests(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, userID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*storage.Account]), args.Error(1)
}

func (m *MockRelationshipRepository) GetPendingFollowRequests(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, userID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*storage.Account]), args.Error(1)
}

func (m *MockRelationshipRepository) GetMutualFollows(ctx context.Context, userID, otherUserID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, userID, otherUserID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*storage.Account]), args.Error(1)
}

func (m *MockRelationshipRepository) GetBlockedUsers(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, userID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*storage.Account]), args.Error(1)
}

func (m *MockRelationshipRepository) GetMutedUsers(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, userID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*storage.Account]), args.Error(1)
}

func (m *MockRelationshipRepository) GetFollowerCount(ctx context.Context, userID string) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockRelationshipRepository) GetFollowingCount(ctx context.Context, userID string) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockRelationshipRepository) GetMutualFollowCount(ctx context.Context, userID, otherUserID string) (int64, error) {
	args := m.Called(ctx, userID, otherUserID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockRelationshipRepository) GetRelationships(ctx context.Context, requestingUserID string, targetUserIDs []string) (map[string]*models.RelationshipRecord, error) {
	args := m.Called(ctx, requestingUserID, targetUserIDs)
	return args.Get(0).(map[string]*models.RelationshipRecord), args.Error(1)
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

// Implement remaining interface methods with minimal mocks
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

// Helper functions for creating test data

func createTestAccount(username string, locked bool) *storage.Account {
	return &storage.Account{
		User: &storage.User{
			Username: username,
		},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/" + username,
				Type: "Person",
			},
			ManuallyApprovesFollowers: locked,
		},
	}
}

func createRemoteTestAccount(username, domain string, locked bool) *storage.Account {
	return &storage.Account{
		User: &storage.User{
			Username: username,
		},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://" + domain + "/users/" + username,
				Type: "Person",
			},
			ManuallyApprovesFollowers: locked,
		},
	}
}

func createTestService() (*Service, *MockRelationshipRepository, *MockAccountRepository, streaming.Publisher, *MockFederationService) {
	relationshipRepo := &MockRelationshipRepository{}
	accountRepo := &MockAccountRepository{}
	publisher := streaming.NewMockPublisher()
	federation := &MockFederationService{}
	logger := zap.NewNop()

	service := NewService(relationshipRepo, accountRepo, publisher, federation, logger, "example.com")

	return service, relationshipRepo, accountRepo, publisher, federation
}

// Test cases

func TestService_Follow_Success_LocalAccount(t *testing.T) {
	service, relationshipRepo, accountRepo, _, _ := createTestService()
	ctx := context.Background()

	follower := createTestAccount("alice", false)
	following := createTestAccount("bob", false) // Local account - no federation

	cmd := &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob",
		ShowReblogs: true,
		Notify:      false,
	}

	// Mock expectations for follow workflow
	accountRepo.On("GetAccount", ctx, "alice").Return(follower, nil)
	accountRepo.On("GetAccount", ctx, "bob").Return(following, nil)
	relationshipRepo.On("IsFollowing", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("IsBlocked", ctx, "bob", "alice").Return(false, nil)
	relationshipRepo.On("CreateFollowRequest", ctx, "alice", "bob").Return(nil)
	relationshipRepo.On("AcceptFollowRequest", ctx, "alice", "bob").Return(nil)

	// Mock relationship queries for building result (both directions)
	relationshipRepo.On("IsFollowing", ctx, "alice", "bob").Return(true, nil) // After follow
	relationshipRepo.On("IsFollowing", ctx, "bob", "alice").Return(false, nil)
	relationshipRepo.On("IsBlocked", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("IsBlocked", ctx, "bob", "alice").Return(false, nil)
	relationshipRepo.On("IsMuted", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("GetFollowStatus", ctx, "alice", "bob").Return(models.RelationshipAccepted, nil)
	relationshipRepo.On("GetFollowStatus", ctx, "bob", "alice").Return("none", nil)

	// Execute
	result, err := service.Follow(ctx, cmd)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsFollowing)
	assert.Empty(t, result.RequestID)
	assert.True(t, result.Relationship.Following)
	assert.False(t, result.Relationship.Requested)
	assert.Len(t, result.Events, 2) // Events to both users' streams

	// Verify mock calls
	relationshipRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)

	// Publisher events are validated via the Events field in the result
}

func TestService_Follow_Success_LockedAccount(t *testing.T) {
	service, relationshipRepo, accountRepo, _, federation := createTestService()
	ctx := context.Background()

	follower := createTestAccount("alice", false)
	following := createTestAccount("bob", true) // Locked account

	cmd := &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob",
		ShowReblogs: true,
		Notify:      false,
	}

	// Mock expectations
	accountRepo.On("GetAccount", ctx, "alice").Return(follower, nil)
	accountRepo.On("GetAccount", ctx, "bob").Return(following, nil)
	relationshipRepo.On("IsFollowing", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("IsBlocked", ctx, "bob", "alice").Return(false, nil)
	relationshipRepo.On("CreateFollowRequest", ctx, "alice", "bob").Return(nil)

	// Mock relationship queries for building result
	relationshipRepo.On("IsFollowing", ctx, "bob", "alice").Return(false, nil)
	relationshipRepo.On("IsBlocked", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("IsMuted", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("GetFollowStatus", ctx, "alice", "bob").Return(models.RelationshipPending, nil)
	relationshipRepo.On("GetFollowStatus", ctx, "bob", "alice").Return("none", nil)

	federation.On("QueueActivity", ctx, mock.AnythingOfType("*activitypub.Activity")).Return(nil)

	// Execute
	result, err := service.Follow(ctx, cmd)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsFollowing) // Not following yet, pending approval
	assert.NotEmpty(t, result.RequestID)
	assert.False(t, result.Relationship.Following)
	assert.True(t, result.Relationship.Requested)
	assert.Len(t, result.Events, 2) // Follow request events

	// Verify mock calls
	relationshipRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
	federation.AssertExpectations(t)
}

func TestService_Follow_AlreadyFollowing(t *testing.T) {
	service, relationshipRepo, accountRepo, _, _ := createTestService()
	ctx := context.Background()

	follower := createTestAccount("alice", false)
	following := createTestAccount("bob", false)

	cmd := &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob",
	}

	// Mock expectations
	accountRepo.On("GetAccount", ctx, "alice").Return(follower, nil)
	accountRepo.On("GetAccount", ctx, "bob").Return(following, nil)
	relationshipRepo.On("IsFollowing", ctx, "alice", "bob").Return(true, nil)

	// Mock relationship queries for building result
	relationshipRepo.On("IsFollowing", ctx, "bob", "alice").Return(false, nil)
	relationshipRepo.On("IsBlocked", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("IsMuted", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("GetFollowStatus", ctx, "alice", "bob").Return(models.RelationshipAccepted, nil)
	relationshipRepo.On("GetFollowStatus", ctx, "bob", "alice").Return("none", nil)

	// Execute
	result, err := service.Follow(ctx, cmd)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsFollowing)
	assert.True(t, result.Relationship.Following)
	assert.Empty(t, result.Events) // No new events

	// Verify mock calls
	relationshipRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
}

func TestService_Follow_Blocked(t *testing.T) {
	service, relationshipRepo, accountRepo, _, _ := createTestService()
	ctx := context.Background()

	follower := createTestAccount("alice", false)
	following := createTestAccount("bob", false)

	cmd := &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob",
	}

	// Mock expectations
	accountRepo.On("GetAccount", ctx, "alice").Return(follower, nil)
	accountRepo.On("GetAccount", ctx, "bob").Return(following, nil)
	relationshipRepo.On("IsFollowing", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("IsBlocked", ctx, "bob", "alice").Return(true, nil) // Alice is blocked by Bob

	// Execute
	result, err := service.Follow(ctx, cmd)

	// Assertions
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "you are blocked")
	assert.Nil(t, result)

	// Verify mock calls
	relationshipRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
}

func TestService_Follow_SelfFollow(t *testing.T) {
	service, _, _, _, _ := createTestService()
	ctx := context.Background()

	cmd := &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "alice", // Self follow
	}

	// Execute
	result, err := service.Follow(ctx, cmd)

	// Assertions
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot follow themselves")
	assert.Nil(t, result)
}

func TestService_Unfollow_Success(t *testing.T) {
	service, relationshipRepo, accountRepo, _, federation := createTestService()
	ctx := context.Background()

	follower := createTestAccount("alice", false)
	following := createTestAccount("bob", false)

	cmd := &UnfollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob",
	}

	// Mock expectations
	relationshipRepo.On("IsFollowing", ctx, "alice", "bob").Return(true, nil)
	accountRepo.On("GetAccount", ctx, "alice").Return(follower, nil)
	accountRepo.On("GetAccount", ctx, "bob").Return(following, nil)
	relationshipRepo.On("Unfollow", ctx, "alice", "bob").Return(nil)

	// Mock relationship queries for building result
	relationshipRepo.On("IsFollowing", ctx, "bob", "alice").Return(false, nil)
	relationshipRepo.On("IsBlocked", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("IsMuted", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("GetFollowStatus", ctx, "alice", "bob").Return("none", nil)
	relationshipRepo.On("GetFollowStatus", ctx, "bob", "alice").Return("none", nil)

	federation.On("QueueActivity", ctx, mock.AnythingOfType("*activitypub.Activity")).Return(nil)

	// Execute
	result, err := service.Unfollow(ctx, cmd)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Relationship.Following)
	assert.Len(t, result.Events, 2) // Events to both users' streams

	// Verify mock calls
	relationshipRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
	federation.AssertExpectations(t)

	// Publisher events are validated via the Events field in the result
}

func TestService_Block_Success(t *testing.T) {
	service, relationshipRepo, accountRepo, _, federation := createTestService()
	ctx := context.Background()

	blocker := createTestAccount("alice", false)
	blocked := createTestAccount("bob", false)

	cmd := &BlockCommand{
		BlockerID: "alice",
		BlockedID: "bob",
		Reason:    "spam",
	}

	// Mock expectations
	accountRepo.On("GetAccount", ctx, "alice").Return(blocker, nil)
	accountRepo.On("GetAccount", ctx, "bob").Return(blocked, nil)
	relationshipRepo.On("IsBlocked", ctx, "alice", "bob").Return(false, nil)

	// Mock unfollow checks during block
	relationshipRepo.On("IsFollowing", ctx, "alice", "bob").Return(true, nil)
	relationshipRepo.On("Unfollow", ctx, "alice", "bob").Return(nil)
	relationshipRepo.On("IsFollowing", ctx, "bob", "alice").Return(false, nil)

	relationshipRepo.On("BlockUser", ctx, "alice", "bob").Return(nil)

	// Mock relationship queries for building result
	relationshipRepo.On("IsFollowing", ctx, "bob", "alice").Return(false, nil)
	relationshipRepo.On("IsMuted", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("GetFollowStatus", ctx, "alice", "bob").Return("none", nil)
	relationshipRepo.On("GetFollowStatus", ctx, "bob", "alice").Return("none", nil)

	federation.On("QueueActivity", ctx, mock.AnythingOfType("*activitypub.Activity")).Return(nil)

	// Execute
	result, err := service.Block(ctx, cmd)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Relationship.Blocking)
	assert.False(t, result.Relationship.Following) // Should automatically unfollow
	assert.Len(t, result.Events, 1)                // Event only to blocker's stream

	// Verify mock calls
	relationshipRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
	federation.AssertExpectations(t)

	// Publisher events are validated via the Events field in the result
}

func TestService_Mute_Success(t *testing.T) {
	service, relationshipRepo, accountRepo, _, _ := createTestService()
	ctx := context.Background()

	muter := createTestAccount("alice", false)
	muted := createTestAccount("bob", false)

	duration := 24 * time.Hour
	cmd := &MuteCommand{
		MuterID:           "alice",
		MutedID:           "bob",
		MuteNotifications: true,
		Duration:          &duration,
		Reason:            "too noisy",
	}

	// Mock expectations
	accountRepo.On("GetAccount", ctx, "alice").Return(muter, nil)
	accountRepo.On("GetAccount", ctx, "bob").Return(muted, nil)
	relationshipRepo.On("IsMuted", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("MuteUser", ctx, "alice", "bob").Return(nil)

	// Mock relationship queries for building result
	relationshipRepo.On("IsFollowing", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("IsFollowing", ctx, "bob", "alice").Return(false, nil)
	relationshipRepo.On("IsBlocked", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("IsBlocked", ctx, "bob", "alice").Return(false, nil)
	relationshipRepo.On("GetFollowStatus", ctx, "alice", "bob").Return("none", nil)
	relationshipRepo.On("GetFollowStatus", ctx, "bob", "alice").Return("none", nil)

	// Execute
	result, err := service.Mute(ctx, cmd)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Relationship.Muting)
	assert.Len(t, result.Events, 1) // Event only to muter's stream

	// Verify mock calls
	relationshipRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)

	// Publisher events are validated via the Events field in the result
}

func TestService_GetRelationship_Success(t *testing.T) {
	service, relationshipRepo, _, _, _ := createTestService()
	ctx := context.Background()

	query := &GetRelationshipQuery{
		RequesterID: "alice",
		TargetID:    "bob",
	}

	// Mock expectations for building relationship data
	relationshipRepo.On("IsFollowing", ctx, "alice", "bob").Return(true, nil)
	relationshipRepo.On("IsFollowing", ctx, "bob", "alice").Return(false, nil)
	relationshipRepo.On("IsBlocked", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("IsBlocked", ctx, "bob", "alice").Return(false, nil)
	relationshipRepo.On("IsMuted", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("GetFollowStatus", ctx, "alice", "bob").Return(models.RelationshipAccepted, nil)
	relationshipRepo.On("GetFollowStatus", ctx, "bob", "alice").Return("none", nil)

	// Execute
	result, err := service.GetRelationship(ctx, query.RequesterID, query.TargetID)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "bob", result.ID)
	assert.True(t, result.Following)
	assert.False(t, result.FollowedBy)
	assert.False(t, result.Blocking)
	assert.False(t, result.BlockedBy)
	assert.False(t, result.Muting)
	assert.False(t, result.Requested)
	assert.False(t, result.RequestedBy)

	// Verify mock calls
	relationshipRepo.AssertExpectations(t)
}

func TestService_GetRelationships_Success(t *testing.T) {
	service, relationshipRepo, _, _, _ := createTestService()
	ctx := context.Background()

	query := &GetRelationshipsQuery{
		RequesterID: "alice",
		TargetIDs:   []string{"bob", "charlie"},
	}

	// Mock expectations for first target (bob)
	relationshipRepo.On("IsFollowing", ctx, "alice", "bob").Return(true, nil)
	relationshipRepo.On("IsFollowing", ctx, "bob", "alice").Return(false, nil)
	relationshipRepo.On("IsBlocked", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("IsBlocked", ctx, "bob", "alice").Return(false, nil)
	relationshipRepo.On("IsMuted", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("GetFollowStatus", ctx, "alice", "bob").Return(models.RelationshipAccepted, nil)
	relationshipRepo.On("GetFollowStatus", ctx, "bob", "alice").Return("none", nil)

	// Mock expectations for second target (charlie)
	relationshipRepo.On("IsFollowing", ctx, "alice", "charlie").Return(false, nil)
	relationshipRepo.On("IsFollowing", ctx, "charlie", "alice").Return(true, nil)
	relationshipRepo.On("IsBlocked", ctx, "alice", "charlie").Return(false, nil)
	relationshipRepo.On("IsBlocked", ctx, "charlie", "alice").Return(false, nil)
	relationshipRepo.On("IsMuted", ctx, "alice", "charlie").Return(true, nil)
	relationshipRepo.On("GetFollowStatus", ctx, "alice", "charlie").Return("none", nil)
	relationshipRepo.On("GetFollowStatus", ctx, "charlie", "alice").Return(models.RelationshipAccepted, nil)

	// Execute
	result, err := service.GetRelationships(ctx, query)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Relationships, 2)
	assert.Empty(t, result.Events)

	// Check first relationship (alice -> bob)
	bobRel := result.Relationships[0]
	assert.Equal(t, "bob", bobRel.ID)
	assert.True(t, bobRel.Following)
	assert.False(t, bobRel.FollowedBy)

	// Check second relationship (alice -> charlie)
	charlieRel := result.Relationships[1]
	assert.Equal(t, "charlie", charlieRel.ID)
	assert.False(t, charlieRel.Following)
	assert.True(t, charlieRel.FollowedBy)
	assert.True(t, charlieRel.Muting)

	// Verify mock calls
	relationshipRepo.AssertExpectations(t)
}

func TestService_Follow_ValidationErrors(t *testing.T) {
	service, _, _, _, _ := createTestService()
	ctx := context.Background()

	tests := []struct {
		name string
		cmd  *FollowCommand
		want string
	}{
		{
			name: "empty follower ID",
			cmd:  &FollowCommand{FollowerID: "", FollowingID: "bob"},
			want: "follower_id is required",
		},
		{
			name: "empty following ID",
			cmd:  &FollowCommand{FollowerID: "alice", FollowingID: ""},
			want: "following_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.Follow(ctx, tt.cmd)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Nil(t, result)
		})
	}
}

func TestService_GetRelationships_ValidationErrors(t *testing.T) {
	service, _, _, _, _ := createTestService()
	ctx := context.Background()

	tests := []struct {
		name  string
		query *GetRelationshipsQuery
		want  string
	}{
		{
			name:  "empty requester ID",
			query: &GetRelationshipsQuery{RequesterID: "", TargetIDs: []string{"bob"}},
			want:  "requester_id is required",
		},
		{
			name:  "empty target IDs",
			query: &GetRelationshipsQuery{RequesterID: "alice", TargetIDs: []string{}},
			want:  "target_ids cannot be empty",
		},
		{
			name: "too many target IDs",
			query: &GetRelationshipsQuery{
				RequesterID: "alice",
				TargetIDs:   make([]string, 41), // Too many
			},
			want: "too many target_ids",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.GetRelationships(ctx, tt.query)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Nil(t, result)
		})
	}
}

func TestService_Follow_RepositoryError(t *testing.T) {
	service, relationshipRepo, accountRepo, _, _ := createTestService()
	ctx := context.Background()

	follower := createTestAccount("alice", false)
	following := createTestAccount("bob", false)

	cmd := &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob",
	}

	// Mock expectations with repository error
	accountRepo.On("GetAccount", ctx, "alice").Return(follower, nil)
	accountRepo.On("GetAccount", ctx, "bob").Return(following, nil)
	relationshipRepo.On("IsFollowing", ctx, "alice", "bob").Return(false, nil)
	relationshipRepo.On("IsBlocked", ctx, "bob", "alice").Return(false, nil)
	relationshipRepo.On("CreateFollowRequest", ctx, "alice", "bob").Return(errors.New("database error"))

	// Execute
	result, err := service.Follow(ctx, cmd)

	// Assertions
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create follow request")
	assert.Nil(t, result)

	// Verify mock calls
	relationshipRepo.AssertExpectations(t)
	accountRepo.AssertExpectations(t)
}
