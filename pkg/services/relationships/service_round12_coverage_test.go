package relationships

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

type statefulRelationshipRepo struct {
	mu sync.Mutex

	followStatus       map[string]map[string]string
	followRelationship map[string]map[string]*models.RelationshipRecord
	blocks             map[string]map[string]bool
	mutes              map[string]map[string]bool

	createFollowRequestCalls int
	acceptFollowRequestCalls int
	unfollowCalls            int
	blockCalls               int
	unblockCalls             int
	unmuteCalls              int
}

func newStatefulRelationshipRepo() *statefulRelationshipRepo {
	return &statefulRelationshipRepo{
		followStatus:       map[string]map[string]string{},
		followRelationship: map[string]map[string]*models.RelationshipRecord{},
		blocks:             map[string]map[string]bool{},
		mutes:              map[string]map[string]bool{},
	}
}

func (r *statefulRelationshipRepo) setFollowStatus(followerID, followingID, status string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.followStatus[followerID]; !ok {
		r.followStatus[followerID] = map[string]string{}
	}
	r.followStatus[followerID][followingID] = status
}

func (r *statefulRelationshipRepo) getFollowStatus(followerID, followingID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if targets, ok := r.followStatus[followerID]; ok {
		if status, ok := targets[followingID]; ok && status != "" {
			return status
		}
	}
	return "none"
}

func (r *statefulRelationshipRepo) setFollowRelationship(followerID, followingID string, rel *models.RelationshipRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.followRelationship[followerID]; !ok {
		r.followRelationship[followerID] = map[string]*models.RelationshipRecord{}
	}
	r.followRelationship[followerID][followingID] = rel
}

func (r *statefulRelationshipRepo) getFollowRelationship(followerID, followingID string) *models.RelationshipRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	if targets, ok := r.followRelationship[followerID]; ok {
		return targets[followingID]
	}
	return nil
}

func (r *statefulRelationshipRepo) setBlocked(blockerID, blockedID string, blocked bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.blocks[blockerID]; !ok {
		r.blocks[blockerID] = map[string]bool{}
	}
	r.blocks[blockerID][blockedID] = blocked
}

func (r *statefulRelationshipRepo) isBlocked(blockerID, blockedID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if targets, ok := r.blocks[blockerID]; ok {
		return targets[blockedID]
	}
	return false
}

func (r *statefulRelationshipRepo) setMuted(muterID, mutedID string, muted bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.mutes[muterID]; !ok {
		r.mutes[muterID] = map[string]bool{}
	}
	r.mutes[muterID][mutedID] = muted
}

func (r *statefulRelationshipRepo) isMuted(muterID, mutedID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if targets, ok := r.mutes[muterID]; ok {
		return targets[mutedID]
	}
	return false
}

func (r *statefulRelationshipRepo) CreateFollowRequest(ctx context.Context, followerID, followingID string) error {
	_ = ctx
	r.createFollowRequestCalls++
	r.setFollowStatus(followerID, followingID, models.RelationshipPending)
	return nil
}

func (r *statefulRelationshipRepo) AcceptFollowRequest(ctx context.Context, followerID, followingID string) error {
	_ = ctx
	r.acceptFollowRequestCalls++
	now := time.Now()
	r.setFollowStatus(followerID, followingID, models.RelationshipAccepted)
	r.setFollowRelationship(followerID, followingID, &models.RelationshipRecord{
		PK:             fmt.Sprintf("FOLLOW#%s", followerID),
		SK:             fmt.Sprintf("FOLLOWING#%s", followingID),
		State:          models.RelationshipAccepted,
		CreatedAt:      now,
		UpdatedAt:      now,
		Notifying:      true,
		ShowingReblogs: false,
		Languages:      []string{"en"},
		Note:           "hello",
	})
	return nil
}

func (r *statefulRelationshipRepo) RejectFollowRequest(ctx context.Context, followerID, followingID string) error {
	_ = ctx
	r.setFollowStatus(followerID, followingID, models.RelationshipRejected)
	return nil
}

func (r *statefulRelationshipRepo) Unfollow(ctx context.Context, followerID, followingID string) error {
	_ = ctx
	r.unfollowCalls++
	r.setFollowStatus(followerID, followingID, "none")
	return nil
}

func (r *statefulRelationshipRepo) IsFollowing(ctx context.Context, followerID, followingID string) (bool, error) {
	_ = ctx
	return r.getFollowStatus(followerID, followingID) == models.RelationshipAccepted, nil
}

func (r *statefulRelationshipRepo) GetFollowStatus(ctx context.Context, followerID, followingID string) (string, error) {
	_ = ctx
	return r.getFollowStatus(followerID, followingID), nil
}

func (r *statefulRelationshipRepo) GetFollowRelationship(ctx context.Context, followerID, followingID string) (*models.RelationshipRecord, error) {
	_ = ctx
	return r.getFollowRelationship(followerID, followingID), nil
}

func (r *statefulRelationshipRepo) GetFollowers(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{Items: []*storage.Account{}}, nil
}

func (r *statefulRelationshipRepo) GetFollowing(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{Items: []*storage.Account{}}, nil
}

func (r *statefulRelationshipRepo) GetFollowRequests(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{Items: []*storage.Account{}}, nil
}

func (r *statefulRelationshipRepo) GetPendingFollowRequests(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{Items: []*storage.Account{}}, nil
}

func (r *statefulRelationshipRepo) GetMutualFollows(context.Context, string, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{Items: []*storage.Account{}}, nil
}

func (r *statefulRelationshipRepo) BlockUser(ctx context.Context, blockerID, blockedID string) error {
	_ = ctx
	r.blockCalls++
	r.setBlocked(blockerID, blockedID, true)
	return nil
}

func (r *statefulRelationshipRepo) UnblockUser(ctx context.Context, blockerID, blockedID string) error {
	_ = ctx
	r.unblockCalls++
	r.setBlocked(blockerID, blockedID, false)
	return nil
}

func (r *statefulRelationshipRepo) IsBlocked(ctx context.Context, blockerID, blockedID string) (bool, error) {
	_ = ctx
	return r.isBlocked(blockerID, blockedID), nil
}

func (r *statefulRelationshipRepo) GetBlockedUsers(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{Items: []*storage.Account{}}, nil
}

func (r *statefulRelationshipRepo) MuteUser(ctx context.Context, muterID, mutedID string) error {
	_ = ctx
	r.setMuted(muterID, mutedID, true)
	return nil
}

func (r *statefulRelationshipRepo) UnmuteUser(ctx context.Context, muterID, mutedID string) error {
	_ = ctx
	r.unmuteCalls++
	r.setMuted(muterID, mutedID, false)
	return nil
}

func (r *statefulRelationshipRepo) IsMuted(ctx context.Context, muterID, mutedID string) (bool, error) {
	_ = ctx
	return r.isMuted(muterID, mutedID), nil
}

func (r *statefulRelationshipRepo) GetMutedUsers(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{Items: []*storage.Account{}}, nil
}

func (r *statefulRelationshipRepo) GetFollowerCount(context.Context, string) (int64, error)  { return 0, nil }
func (r *statefulRelationshipRepo) GetFollowingCount(context.Context, string) (int64, error) { return 0, nil }
func (r *statefulRelationshipRepo) GetMutualFollowCount(context.Context, string, string) (int64, error) {
	return 0, nil
}

func (r *statefulRelationshipRepo) GetRelationships(context.Context, string, []string) (map[string]*models.RelationshipRecord, error) {
	return map[string]*models.RelationshipRecord{}, nil
}

func TestService_normalizeActorIdentifier(t *testing.T) {
	service := NewService(nil, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")

	assert.Equal(t, "alice", service.normalizeActorIdentifier(" alice "))
	assert.Equal(t, "alice", service.normalizeActorIdentifier("https://example.com/users/alice"))
	assert.Equal(t, "bob@remote.social", service.normalizeActorIdentifier("https://remote.social/users/bob"))
	assert.Equal(t, "bob@remote.social", service.normalizeActorIdentifier("https://remote.social/users/bob.json"))
	assert.Equal(t, "https://%", service.normalizeActorIdentifier("https://%"))
}

func TestService_GetRelationship_LegacyRepoPopulatesFields(t *testing.T) {
	ctx := context.Background()
	repo := newStatefulRelationshipRepo()

	now := time.Now().Add(-time.Hour)
	repo.setFollowStatus("alice", "bob", models.RelationshipAccepted)
	repo.setFollowRelationship("alice", "bob", &models.RelationshipRecord{
		State:          models.RelationshipAccepted,
		CreatedAt:      now,
		UpdatedAt:      now.Add(5 * time.Minute),
		Notifying:      true,
		ShowingReblogs: true,
		Languages:      []string{"en", "es"},
		Note:           "note",
	})
	repo.setFollowStatus("bob", "alice", models.RelationshipPending)
	repo.setBlocked("alice", "bob", true)
	repo.setMuted("alice", "bob", true)

	service := NewService(repo, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")
	relationship, err := service.GetRelationship(ctx, "alice", "bob")
	assert.NoError(t, err)
	assert.True(t, relationship.Following)
	assert.False(t, relationship.FollowedBy)
	assert.True(t, relationship.Blocking)
	assert.True(t, relationship.Muting)
	assert.False(t, relationship.Requested)
	assert.True(t, relationship.RequestedBy)
	assert.True(t, relationship.Notifying)
	assert.True(t, relationship.ShowingReblogs)
	assert.Equal(t, []string{"en", "es"}, relationship.Languages)
	assert.Equal(t, "note", relationship.Note)
}

func TestService_Follow_PublicAccount_AcceptsAndEmitsEvents(t *testing.T) {
	ctx := context.Background()
	repo := newStatefulRelationshipRepo()
	accountRepo := &MockAccountRepository{}
	publisher := streaming.NewMockPublisher()
	federation := &MockFederationService{}

	follower := createTestAccount("alice", false)
	following := createTestAccount("bob", false)

	accountRepo.On("GetAccount", ctx, "alice").Return(follower, nil)
	accountRepo.On("GetAccount", ctx, "bob").Return(following, nil)

	federation.On("QueueActivity", ctx, mock.Anything).Return(nil).Maybe()

	service := NewService(repo, accountRepo, publisher, federation, zap.NewNop(), "example.com")

	result, err := service.Follow(ctx, &FollowCommand{FollowerID: "alice", FollowingID: "bob"})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsFollowing)
	assert.Empty(t, result.RequestID)
	assert.NotNil(t, result.Relationship)
	assert.True(t, result.Relationship.Following)
	assert.NotNil(t, result.Activity)
	assert.Len(t, result.Events, 2)
	assert.Equal(t, models.RelationshipAccepted, repo.getFollowStatus("alice", "bob"))
	assert.Equal(t, 1, repo.createFollowRequestCalls)
	assert.Equal(t, 1, repo.acceptFollowRequestCalls)
}

func TestService_Follow_LockedAccount_CreatesRequest(t *testing.T) {
	ctx := context.Background()
	repo := newStatefulRelationshipRepo()
	accountRepo := &MockAccountRepository{}
	publisher := streaming.NewMockPublisher()
	federation := &MockFederationService{}

	follower := createTestAccount("alice", false)
	following := createTestAccount("bob", true)

	accountRepo.On("GetAccount", ctx, "alice").Return(follower, nil)
	accountRepo.On("GetAccount", ctx, "bob").Return(following, nil)

	federation.On("QueueActivity", ctx, mock.Anything).Return(nil).Maybe()

	service := NewService(repo, accountRepo, publisher, federation, zap.NewNop(), "example.com")

	result, err := service.Follow(ctx, &FollowCommand{FollowerID: "alice", FollowingID: "bob"})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsFollowing)
	assert.NotEmpty(t, result.RequestID)
	assert.NotNil(t, result.Relationship)
	assert.False(t, result.Relationship.Following)
	assert.True(t, result.Relationship.Requested)
	assert.Len(t, result.Events, 2)
	assert.Equal(t, models.RelationshipPending, repo.getFollowStatus("alice", "bob"))
	assert.Equal(t, 1, repo.createFollowRequestCalls)
	assert.Equal(t, 0, repo.acceptFollowRequestCalls)
}

func TestService_Follow_AlreadyFollowing_ShortCircuits(t *testing.T) {
	ctx := context.Background()
	repo := newStatefulRelationshipRepo()
	repo.setFollowStatus("alice", "bob", models.RelationshipAccepted)

	accountRepo := &MockAccountRepository{}
	accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", false), nil)
	accountRepo.On("GetAccount", ctx, "bob").Return(createTestAccount("bob", false), nil)

	service := NewService(repo, accountRepo, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")

	result, err := service.Follow(ctx, &FollowCommand{FollowerID: "alice", FollowingID: "bob"})
	assert.NoError(t, err)
	assert.True(t, result.IsFollowing)
	assert.Equal(t, 0, repo.createFollowRequestCalls)
	assert.Equal(t, 0, repo.acceptFollowRequestCalls)
}

func TestService_Block_UnfollowsAndSetsBlock(t *testing.T) {
	ctx := context.Background()
	repo := newStatefulRelationshipRepo()
	repo.setFollowStatus("alice", "bob", models.RelationshipAccepted)
	repo.setFollowStatus("bob", "alice", models.RelationshipAccepted)

	accountRepo := &MockAccountRepository{}
	accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", false), nil)
	accountRepo.On("GetAccount", ctx, "bob").Return(createTestAccount("bob", false), nil)

	service := NewService(repo, accountRepo, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")

	result, err := service.Block(ctx, &BlockCommand{BlockerID: "alice", BlockedID: "bob"})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Relationship.Blocking)
	assert.False(t, result.Relationship.Following)
	assert.Equal(t, 2, repo.unfollowCalls)
	assert.Equal(t, 1, repo.blockCalls)
}

func TestService_Unfollow_IdempotentWhenNotFollowing(t *testing.T) {
	ctx := context.Background()
	repo := newStatefulRelationshipRepo()
	service := NewService(repo, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")

	result, err := service.Unfollow(ctx, &UnfollowCommand{FollowerID: "alice", FollowingID: "bob"})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Relationship.Following)
	assert.Equal(t, 0, repo.unfollowCalls)
}

func TestService_Unblock_NoAccountRepoOrStorage_UsesMinimalAccounts(t *testing.T) {
	ctx := context.Background()
	repo := newStatefulRelationshipRepo()
	repo.setBlocked("alice", "bob", true)

	service := NewService(repo, nil, streaming.NewMockPublisher(), nil, zap.NewNop(), "example.com")

	result, err := service.Unblock(ctx, &UnblockCommand{BlockerID: "alice", BlockedID: "bob"})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Relationship.Blocking)
	assert.Equal(t, 1, repo.unblockCalls)
}
