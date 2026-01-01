// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockRelationshipRepository is a mock implementation of interfaces.RelationshipRepository
// using testify/mock for expectation-based testing.
type MockRelationshipRepository struct {
	mock.Mock
}

// NewMockRelationshipRepository creates a new mock relationship repository
func NewMockRelationshipRepository() *MockRelationshipRepository {
	return &MockRelationshipRepository{}
}

// ===== Core Follow Relationship Operations =====

// CreateRelationship mocks the CreateRelationship method
func (m *MockRelationshipRepository) CreateRelationship(ctx context.Context, followerUsername, followingUsername, activityID string) error {
	args := m.Called(ctx, followerUsername, followingUsername, activityID)
	return args.Error(0)
}

// DeleteRelationship mocks the DeleteRelationship method
func (m *MockRelationshipRepository) DeleteRelationship(ctx context.Context, followerUsername, followingUsername string) error {
	args := m.Called(ctx, followerUsername, followingUsername)
	return args.Error(0)
}

// GetRelationship mocks the GetRelationship method
func (m *MockRelationshipRepository) GetRelationship(ctx context.Context, followerUsername, followingUsername string) (*models.RelationshipRecord, error) {
	args := m.Called(ctx, followerUsername, followingUsername)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.RelationshipRecord), args.Error(1)
}

// UpdateRelationship mocks the UpdateRelationship method
func (m *MockRelationshipRepository) UpdateRelationship(ctx context.Context, followerUsername, followingUsername string, updates map[string]interface{}) error {
	args := m.Called(ctx, followerUsername, followingUsername, updates)
	return args.Error(0)
}

// IsFollowing mocks the IsFollowing method
func (m *MockRelationshipRepository) IsFollowing(ctx context.Context, followerUsername, targetActorID string) (bool, error) {
	args := m.Called(ctx, followerUsername, targetActorID)
	return args.Bool(0), args.Error(1)
}


// ===== Follow Request Operations =====

// GetFollowRequest mocks the GetFollowRequest method
func (m *MockRelationshipRepository) GetFollowRequest(ctx context.Context, followerID, targetID string) (*storage.RelationshipRecord, error) {
	args := m.Called(ctx, followerID, targetID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.RelationshipRecord), args.Error(1)
}

// HasFollowRequest mocks the HasFollowRequest method
func (m *MockRelationshipRepository) HasFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error) {
	args := m.Called(ctx, requesterID, targetID)
	return args.Bool(0), args.Error(1)
}

// HasPendingFollowRequest mocks the HasPendingFollowRequest method
func (m *MockRelationshipRepository) HasPendingFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error) {
	args := m.Called(ctx, requesterID, targetID)
	return args.Bool(0), args.Error(1)
}

// GetPendingFollowRequests mocks the GetPendingFollowRequests method
func (m *MockRelationshipRepository) GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

// AcceptFollowRequest mocks the AcceptFollowRequest method
func (m *MockRelationshipRepository) AcceptFollowRequest(ctx context.Context, followerUsername, followingUsername string) error {
	args := m.Called(ctx, followerUsername, followingUsername)
	return args.Error(0)
}

// RejectFollowRequest mocks the RejectFollowRequest method
func (m *MockRelationshipRepository) RejectFollowRequest(ctx context.Context, followerUsername, followingUsername string) error {
	args := m.Called(ctx, followerUsername, followingUsername)
	return args.Error(0)
}

// ===== Follower/Following List Operations =====

// GetFollowers mocks the GetFollowers method
func (m *MockRelationshipRepository) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

// GetFollowing mocks the GetFollowing method
func (m *MockRelationshipRepository) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

// CountFollowers mocks the CountFollowers method
func (m *MockRelationshipRepository) CountFollowers(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

// CountFollowing mocks the CountFollowing method
func (m *MockRelationshipRepository) CountFollowing(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

// GetFollowerCount mocks the GetFollowerCount method
func (m *MockRelationshipRepository) GetFollowerCount(ctx context.Context, userID string) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

// GetFollowingCount mocks the GetFollowingCount method
func (m *MockRelationshipRepository) GetFollowingCount(ctx context.Context, userID string) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

// CountRelationshipsByDomain mocks the CountRelationshipsByDomain method
func (m *MockRelationshipRepository) CountRelationshipsByDomain(ctx context.Context, domain string) (followers, following int, err error) {
	args := m.Called(ctx, domain)
	return args.Int(0), args.Int(1), args.Error(2)
}

// Unfollow mocks the Unfollow method
func (m *MockRelationshipRepository) Unfollow(ctx context.Context, followerID, followingID string) error {
	args := m.Called(ctx, followerID, followingID)
	return args.Error(0)
}


// ===== Block Operations =====

// CreateBlock mocks the CreateBlock method
func (m *MockRelationshipRepository) CreateBlock(ctx context.Context, blockerActor, blockedActor, activityID string) error {
	args := m.Called(ctx, blockerActor, blockedActor, activityID)
	return args.Error(0)
}

// DeleteBlock mocks the DeleteBlock method
func (m *MockRelationshipRepository) DeleteBlock(ctx context.Context, blockerActor, blockedActor string) error {
	args := m.Called(ctx, blockerActor, blockedActor)
	return args.Error(0)
}

// BlockUser mocks the BlockUser method
func (m *MockRelationshipRepository) BlockUser(ctx context.Context, blockerID, blockedID string) error {
	args := m.Called(ctx, blockerID, blockedID)
	return args.Error(0)
}

// UnblockUser mocks the UnblockUser method
func (m *MockRelationshipRepository) UnblockUser(ctx context.Context, blockerID, blockedID string) error {
	args := m.Called(ctx, blockerID, blockedID)
	return args.Error(0)
}

// IsBlocked mocks the IsBlocked method
func (m *MockRelationshipRepository) IsBlocked(ctx context.Context, blockerActor, blockedActor string) (bool, error) {
	args := m.Called(ctx, blockerActor, blockedActor)
	return args.Bool(0), args.Error(1)
}

// IsBlockedBidirectional mocks the IsBlockedBidirectional method
func (m *MockRelationshipRepository) IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error) {
	args := m.Called(ctx, actor1, actor2)
	return args.Bool(0), args.Error(1)
}

// GetBlockedUsers mocks the GetBlockedUsers method
func (m *MockRelationshipRepository) GetBlockedUsers(ctx context.Context, blockerActor string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, blockerActor, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

// GetUsersWhoBlocked mocks the GetUsersWhoBlocked method
func (m *MockRelationshipRepository) GetUsersWhoBlocked(ctx context.Context, blockedActor string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, blockedActor, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

// GetBlock mocks the GetBlock method
func (m *MockRelationshipRepository) GetBlock(ctx context.Context, blockerActor, blockedActor string) (*storage.Block, error) {
	args := m.Called(ctx, blockerActor, blockedActor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Block), args.Error(1)
}

// CountBlockedUsers mocks the CountBlockedUsers method
func (m *MockRelationshipRepository) CountBlockedUsers(ctx context.Context, blockerActor string) (int, error) {
	args := m.Called(ctx, blockerActor)
	return args.Int(0), args.Error(1)
}

// CountUsersWhoBlocked mocks the CountUsersWhoBlocked method
func (m *MockRelationshipRepository) CountUsersWhoBlocked(ctx context.Context, blockedActor string) (int, error) {
	args := m.Called(ctx, blockedActor)
	return args.Int(0), args.Error(1)
}


// ===== Mute Operations =====

// CreateMute mocks the CreateMute method
func (m *MockRelationshipRepository) CreateMute(ctx context.Context, muterActor, mutedActor, activityID string, hideNotifications bool, duration *time.Duration) error {
	args := m.Called(ctx, muterActor, mutedActor, activityID, hideNotifications, duration)
	return args.Error(0)
}

// DeleteMute mocks the DeleteMute method
func (m *MockRelationshipRepository) DeleteMute(ctx context.Context, muterActor, mutedActor string) error {
	args := m.Called(ctx, muterActor, mutedActor)
	return args.Error(0)
}

// UnmuteUser mocks the UnmuteUser method
func (m *MockRelationshipRepository) UnmuteUser(ctx context.Context, muterID, mutedID string) error {
	args := m.Called(ctx, muterID, mutedID)
	return args.Error(0)
}

// IsMuted mocks the IsMuted method
func (m *MockRelationshipRepository) IsMuted(ctx context.Context, muterActor, mutedActor string) (bool, error) {
	args := m.Called(ctx, muterActor, mutedActor)
	return args.Bool(0), args.Error(1)
}

// GetMutedUsers mocks the GetMutedUsers method
func (m *MockRelationshipRepository) GetMutedUsers(ctx context.Context, muterActor string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, muterActor, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

// GetUsersWhoMuted mocks the GetUsersWhoMuted method
func (m *MockRelationshipRepository) GetUsersWhoMuted(ctx context.Context, mutedActor string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, mutedActor, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

// GetMute mocks the GetMute method
func (m *MockRelationshipRepository) GetMute(ctx context.Context, muterActor, mutedActor string) (*storage.Mute, error) {
	args := m.Called(ctx, muterActor, mutedActor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Mute), args.Error(1)
}

// CountMutedUsers mocks the CountMutedUsers method
func (m *MockRelationshipRepository) CountMutedUsers(ctx context.Context, muterActor string) (int, error) {
	args := m.Called(ctx, muterActor)
	return args.Int(0), args.Error(1)
}

// CountUsersWhoMuted mocks the CountUsersWhoMuted method
func (m *MockRelationshipRepository) CountUsersWhoMuted(ctx context.Context, mutedActor string) (int, error) {
	args := m.Called(ctx, mutedActor)
	return args.Int(0), args.Error(1)
}


// ===== Endorsement Operations =====

// IsEndorsed mocks the IsEndorsed method
func (m *MockRelationshipRepository) IsEndorsed(ctx context.Context, userID, targetID string) (bool, error) {
	args := m.Called(ctx, userID, targetID)
	return args.Bool(0), args.Error(1)
}

// CreateEndorsement mocks the CreateEndorsement method
func (m *MockRelationshipRepository) CreateEndorsement(ctx context.Context, endorsement *storage.AccountPin) error {
	args := m.Called(ctx, endorsement)
	return args.Error(0)
}

// DeleteEndorsement mocks the DeleteEndorsement method
func (m *MockRelationshipRepository) DeleteEndorsement(ctx context.Context, endorserID, endorsedID string) error {
	args := m.Called(ctx, endorserID, endorsedID)
	return args.Error(0)
}

// GetEndorsements mocks the GetEndorsements method
func (m *MockRelationshipRepository) GetEndorsements(ctx context.Context, userID string, limit int, cursor string) ([]*storage.AccountPin, string, error) {
	args := m.Called(ctx, userID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.AccountPin), args.String(1), args.Error(2)
}

// ===== Relationship Note Operations =====

// GetRelationshipNote mocks the GetRelationshipNote method
func (m *MockRelationshipRepository) GetRelationshipNote(ctx context.Context, userID, targetID string) (*storage.AccountNote, error) {
	args := m.Called(ctx, userID, targetID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.AccountNote), args.Error(1)
}


// ===== Move Operations =====

// CreateMove mocks the CreateMove method
func (m *MockRelationshipRepository) CreateMove(ctx context.Context, move *storage.Move) error {
	args := m.Called(ctx, move)
	return args.Error(0)
}

// GetMove mocks the GetMove method
func (m *MockRelationshipRepository) GetMove(ctx context.Context, actor string) (*storage.Move, error) {
	args := m.Called(ctx, actor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Move), args.Error(1)
}

// GetAccountMoves mocks the GetAccountMoves method
func (m *MockRelationshipRepository) GetAccountMoves(ctx context.Context, actor string) ([]*storage.Move, error) {
	args := m.Called(ctx, actor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Move), args.Error(1)
}

// UpdateMoveProgress mocks the UpdateMoveProgress method
func (m *MockRelationshipRepository) UpdateMoveProgress(ctx context.Context, actor, target string, progress map[string]interface{}) error {
	args := m.Called(ctx, actor, target, progress)
	return args.Error(0)
}

// VerifyMove mocks the VerifyMove method
func (m *MockRelationshipRepository) VerifyMove(ctx context.Context, actor, target string) error {
	args := m.Called(ctx, actor, target)
	return args.Error(0)
}

// GetPendingMoves mocks the GetPendingMoves method
func (m *MockRelationshipRepository) GetPendingMoves(ctx context.Context, limit int) ([]*storage.Move, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Move), args.Error(1)
}

// GetMoveByTarget mocks the GetMoveByTarget method
func (m *MockRelationshipRepository) GetMoveByTarget(ctx context.Context, target string) ([]*storage.Move, error) {
	args := m.Called(ctx, target)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Move), args.Error(1)
}

// HasMovedFrom mocks the HasMovedFrom method
func (m *MockRelationshipRepository) HasMovedFrom(ctx context.Context, oldActor, newActor string) (bool, error) {
	args := m.Called(ctx, oldActor, newActor)
	return args.Bool(0), args.Error(1)
}


// ===== Collection Operations =====

// AddToCollection mocks the AddToCollection method
func (m *MockRelationshipRepository) AddToCollection(ctx context.Context, collection string, item *storage.CollectionItem) error {
	args := m.Called(ctx, collection, item)
	return args.Error(0)
}

// RemoveFromCollection mocks the RemoveFromCollection method
func (m *MockRelationshipRepository) RemoveFromCollection(ctx context.Context, collection, itemID string) error {
	args := m.Called(ctx, collection, itemID)
	return args.Error(0)
}

// GetCollectionItems mocks the GetCollectionItems method
func (m *MockRelationshipRepository) GetCollectionItems(ctx context.Context, collection string, limit int, cursor string) ([]*storage.CollectionItem, string, error) {
	args := m.Called(ctx, collection, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.CollectionItem), args.String(1), args.Error(2)
}

// IsInCollection mocks the IsInCollection method
func (m *MockRelationshipRepository) IsInCollection(ctx context.Context, collection, itemID string) (bool, error) {
	args := m.Called(ctx, collection, itemID)
	return args.Bool(0), args.Error(1)
}

// CountCollectionItems mocks the CountCollectionItems method
func (m *MockRelationshipRepository) CountCollectionItems(ctx context.Context, collection string) (int, error) {
	args := m.Called(ctx, collection)
	return args.Int(0), args.Error(1)
}

// ClearCollection mocks the ClearCollection method
func (m *MockRelationshipRepository) ClearCollection(ctx context.Context, collection string) error {
	args := m.Called(ctx, collection)
	return args.Error(0)
}

// Ensure MockRelationshipRepository implements interfaces.ConcreteRelationshipRepository
var _ interfaces.ConcreteRelationshipRepository = (*MockRelationshipRepository)(nil)
