package relationships

import (
	"context"
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

// Simple mocks that don't enforce strict order
type SimpleRelationshipRepo struct {
	mock.Mock
}

func (m *SimpleRelationshipRepo) IsFollowing(ctx context.Context, followerID, followingID string) (bool, error) {
	args := m.Called(ctx, followerID, followingID)
	return args.Bool(0), args.Error(1)
}

func (m *SimpleRelationshipRepo) IsBlocked(ctx context.Context, blockerID, blockedID string) (bool, error) {
	args := m.Called(ctx, blockerID, blockedID)
	return args.Bool(0), args.Error(1)
}

func (m *SimpleRelationshipRepo) IsMuted(ctx context.Context, muterID, mutedID string) (bool, error) {
	args := m.Called(ctx, muterID, mutedID)
	return args.Bool(0), args.Error(1)
}

func (m *SimpleRelationshipRepo) GetFollowStatus(ctx context.Context, followerID, followingID string) (string, error) {
	args := m.Called(ctx, followerID, followingID)
	return args.String(0), args.Error(1)
}

func (m *SimpleRelationshipRepo) CreateFollowRequest(ctx context.Context, followerID, followingID string) error {
	args := m.Called(ctx, followerID, followingID)
	return args.Error(0)
}

func (m *SimpleRelationshipRepo) AcceptFollowRequest(ctx context.Context, followerID, followingID string) error {
	args := m.Called(ctx, followerID, followingID)
	return args.Error(0)
}

// Stub out other methods we don't use in simple tests
func (m *SimpleRelationshipRepo) RejectFollowRequest(ctx context.Context, followerID, followingID string) error {
	return nil
}
func (m *SimpleRelationshipRepo) Unfollow(ctx context.Context, followerID, followingID string) error {
	return nil
}
func (m *SimpleRelationshipRepo) GetFollowRelationship(ctx context.Context, followerID, followingID string) (*models.RelationshipRecord, error) {
	return nil, nil
}
func (m *SimpleRelationshipRepo) GetFollowers(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return nil, nil
}
func (m *SimpleRelationshipRepo) GetFollowing(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return nil, nil
}
func (m *SimpleRelationshipRepo) GetFollowRequests(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return nil, nil
}
func (m *SimpleRelationshipRepo) GetPendingFollowRequests(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return nil, nil
}
func (m *SimpleRelationshipRepo) GetMutualFollows(ctx context.Context, userID, otherUserID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return nil, nil
}
func (m *SimpleRelationshipRepo) BlockUser(ctx context.Context, blockerID, blockedID string) error {
	return nil
}
func (m *SimpleRelationshipRepo) UnblockUser(ctx context.Context, blockerID, blockedID string) error {
	return nil
}
func (m *SimpleRelationshipRepo) GetBlockedUsers(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return nil, nil
}
func (m *SimpleRelationshipRepo) MuteUser(ctx context.Context, muterID, mutedID string) error {
	return nil
}
func (m *SimpleRelationshipRepo) UnmuteUser(ctx context.Context, muterID, mutedID string) error {
	return nil
}
func (m *SimpleRelationshipRepo) GetMutedUsers(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return nil, nil
}
func (m *SimpleRelationshipRepo) GetFollowerCount(ctx context.Context, userID string) (int64, error) {
	return 0, nil
}
func (m *SimpleRelationshipRepo) GetFollowingCount(ctx context.Context, userID string) (int64, error) {
	return 0, nil
}
func (m *SimpleRelationshipRepo) GetMutualFollowCount(ctx context.Context, userID, otherUserID string) (int64, error) {
	return 0, nil
}
func (m *SimpleRelationshipRepo) GetRelationships(ctx context.Context, requestingUserID string, targetUserIDs []string) (map[string]*models.RelationshipRecord, error) {
	return nil, nil
}

type SimpleAccountRepo struct {
	mock.Mock
}

func (m *SimpleAccountRepo) GetAccount(ctx context.Context, username string) (*storage.Account, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Account), args.Error(1)
}

// Stub out other methods
func (m *SimpleAccountRepo) CreateAccount(ctx context.Context, account *storage.Account) error {
	return nil
}
func (m *SimpleAccountRepo) GetAccountByURL(ctx context.Context, actorURL string) (*storage.Account, error) {
	return nil, nil
}
func (m *SimpleAccountRepo) GetAccountByEmail(ctx context.Context, email string) (*storage.Account, error) {
	return nil, nil
}
func (m *SimpleAccountRepo) UpdateAccount(ctx context.Context, account *storage.Account) error {
	return nil
}
func (m *SimpleAccountRepo) DeleteAccount(ctx context.Context, username string) error { return nil }
func (m *SimpleAccountRepo) SearchAccounts(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return nil, nil
}
func (m *SimpleAccountRepo) GetSuggestedAccounts(ctx context.Context, forUserID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.AccountSuggestion], error) {
	return nil, nil
}
func (m *SimpleAccountRepo) GetFeaturedAccounts(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return nil, nil
}
func (m *SimpleAccountRepo) ApproveAccount(ctx context.Context, username string) error { return nil }
func (m *SimpleAccountRepo) SuspendAccount(ctx context.Context, username string, reason string) error {
	return nil
}
func (m *SimpleAccountRepo) UnsuspendAccount(ctx context.Context, username string) error { return nil }
func (m *SimpleAccountRepo) SilenceAccount(ctx context.Context, username string, reason string) error {
	return nil
}
func (m *SimpleAccountRepo) UnsilenceAccount(ctx context.Context, username string) error { return nil }
func (m *SimpleAccountRepo) UpdateAccountPreferences(ctx context.Context, username string, preferences map[string]interface{}) error {
	return nil
}
func (m *SimpleAccountRepo) GetAccountPreferences(ctx context.Context, username string) (map[string]interface{}, error) {
	return nil, nil
}
func (m *SimpleAccountRepo) UpdateAccountFeatures(ctx context.Context, username string, features map[string]bool) error {
	return nil
}
func (m *SimpleAccountRepo) GetAccountFeatures(ctx context.Context, username string) (map[string]bool, error) {
	return nil, nil
}
func (m *SimpleAccountRepo) ValidateCredentials(ctx context.Context, username, password string) (*storage.Account, error) {
	return nil, nil
}
func (m *SimpleAccountRepo) UpdatePassword(ctx context.Context, username, newPasswordHash string) error {
	return nil
}
func (m *SimpleAccountRepo) CreatePasswordReset(ctx context.Context, reset *storage.PasswordReset) error {
	return nil
}
func (m *SimpleAccountRepo) GetPasswordReset(ctx context.Context, token string) (*storage.PasswordReset, error) {
	return nil, nil
}
func (m *SimpleAccountRepo) UsePasswordReset(ctx context.Context, token string) error { return nil }
func (m *SimpleAccountRepo) RecordLogin(ctx context.Context, attempt *storage.LoginAttempt) error {
	return nil
}
func (m *SimpleAccountRepo) GetLoginHistory(ctx context.Context, username string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.LoginAttempt], error) {
	return nil, nil
}
func (m *SimpleAccountRepo) UpdateLastActivity(ctx context.Context, username string, activity time.Time) error {
	return nil
}
func (m *SimpleAccountRepo) GetAccountsByUsernames(ctx context.Context, usernames []string) ([]*storage.Account, error) {
	return nil, nil
}
func (m *SimpleAccountRepo) GetAccountsCount(ctx context.Context) (int64, error) { return 0, nil }

func (r *SimpleAccountRepo) AddBookmark(ctx context.Context, username, objectID string) error {
	return nil
}

func (r *SimpleAccountRepo) RemoveBookmark(ctx context.Context, username, objectID string) error {
	return nil
}

func (r *SimpleAccountRepo) GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]*storage.Bookmark, string, error) {
	return []*storage.Bookmark{}, "", nil
}

func (r *SimpleAccountRepo) GetBookmarkedStatuses(ctx context.Context, username string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	return &interfaces.PaginatedResult[*models.Status]{
		Items: []*models.Status{},
	}, nil
}

func TestSimpleFollow(t *testing.T) {
	// Create repos and service
	relationshipRepo := &SimpleRelationshipRepo{}
	accountRepo := &SimpleAccountRepo{}
	publisher := streaming.NewMockPublisher()
	logger := zap.NewNop()

	service := NewService(relationshipRepo, accountRepo, publisher, nil, logger, "example.com")

	ctx := context.Background()

	// Create test accounts
	follower := &storage.Account{
		User: &storage.User{Username: "alice"},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/alice",
				Type: "Person",
			},
		},
	}

	following := &storage.Account{
		User: &storage.User{Username: "bob"},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/bob",
				Type: "Person",
			},
			ManuallyApprovesFollowers: false, // Public account
		},
	}

	// Set up mocks with flexible matching
	accountRepo.On("GetAccount", ctx, "alice").Return(follower, nil)
	accountRepo.On("GetAccount", ctx, "bob").Return(following, nil)
	relationshipRepo.On("IsFollowing", ctx, "alice", "bob").Return(false, nil).Once()
	relationshipRepo.On("IsBlocked", ctx, "bob", "alice").Return(false, nil)
	relationshipRepo.On("CreateFollowRequest", ctx, "alice", "bob").Return(nil)
	relationshipRepo.On("AcceptFollowRequest", ctx, "alice", "bob").Return(nil)

	// For the relationship building (allowing flexible order)
	relationshipRepo.On("IsFollowing", ctx, "alice", "bob").Return(true, nil).Maybe()
	relationshipRepo.On("IsFollowing", ctx, "bob", "alice").Return(false, nil).Maybe()
	relationshipRepo.On("IsBlocked", ctx, "alice", "bob").Return(false, nil).Maybe()
	relationshipRepo.On("IsBlocked", ctx, "bob", "alice").Return(false, nil).Maybe()
	relationshipRepo.On("IsMuted", ctx, "alice", "bob").Return(false, nil).Maybe()
	relationshipRepo.On("GetFollowStatus", ctx, "alice", "bob").Return(models.RelationshipAccepted, nil).Maybe()
	relationshipRepo.On("GetFollowStatus", ctx, "bob", "alice").Return("none", nil).Maybe()

	// Execute follow command
	cmd := &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob",
	}

	result, err := service.Follow(ctx, cmd)

	// Verify results
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsFollowing)
	assert.Len(t, result.Events, 2)
}

func TestSimpleValidation(t *testing.T) {
	service := NewService(nil, nil, nil, nil, zap.NewNop(), "example.com")
	ctx := context.Background()

	// Test empty follower ID
	cmd := &FollowCommand{
		FollowerID:  "",
		FollowingID: "bob",
	}

	result, err := service.Follow(ctx, cmd)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "validation failed for follower_id")

	// Test self-follow
	cmd = &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "alice",
	}

	result, err = service.Follow(ctx, cmd)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Cannot perform operation on self")
}
