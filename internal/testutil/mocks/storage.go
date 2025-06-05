package mocks

import (
	"context"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/stretchr/testify/mock"
)

// TimelineMethods provides mock implementations of timeline-related storage methods
// This can be embedded in test mock structs to satisfy the Storage interface
type TimelineMethods struct{}

// WriteToTimeline is a mock implementation
func (m *TimelineMethods) WriteToTimeline(ctx context.Context, timeline *storage.TimelineEntry) error {
	return nil
}

// WriteToTimelines is a mock implementation
func (m *TimelineMethods) WriteToTimelines(ctx context.Context, entries []*storage.TimelineEntry) error {
	return nil
}

// GetHomeTimeline is a mock implementation
func (m *TimelineMethods) GetHomeTimeline(ctx context.Context, username string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	return []*storage.TimelineEntry{}, "", nil
}

// GetPublicTimeline is a mock implementation
func (m *TimelineMethods) GetPublicTimeline(ctx context.Context, local bool, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	return []*storage.TimelineEntry{}, "", nil
}

// GetListTimeline is a mock implementation
func (m *TimelineMethods) GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	return []*storage.TimelineEntry{}, "", nil
}

// DeleteFromTimeline is a mock implementation
func (m *TimelineMethods) DeleteFromTimeline(ctx context.Context, timelineType, timelineID, entryID string) error {
	return nil
}

// DeleteExpiredTimelineEntries is a mock implementation
func (m *TimelineMethods) DeleteExpiredTimelineEntries(ctx context.Context, before time.Time) error {
	return nil
}

// BaseMockStorage provides no-op implementations of all Storage interface methods
// This can be embedded in test mocks to automatically satisfy the interface
// Test mocks can then override only the methods they need
type BaseMockStorage struct {
	TimelineMethods
}

// Actor operations
func (m *BaseMockStorage) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	return nil
}
func (m *BaseMockStorage) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	return nil, nil
}
func (m *BaseMockStorage) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	return "", nil
}
func (m *BaseMockStorage) UpdateActor(ctx context.Context, actor *activitypub.Actor) error {
	return nil
}
func (m *BaseMockStorage) DeleteActor(ctx context.Context, username string) error {
	return nil
}

// Activity operations
func (m *BaseMockStorage) CreateActivity(ctx context.Context, activity *activitypub.Activity) error {
	return nil
}
func (m *BaseMockStorage) GetActivity(ctx context.Context, id string) (*activitypub.Activity, error) {
	return nil, nil
}
func (m *BaseMockStorage) GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	return nil, "", nil
}

// Object operations
func (m *BaseMockStorage) CreateObject(ctx context.Context, object interface{}) error {
	return nil
}
func (m *BaseMockStorage) GetObject(ctx context.Context, id string) (interface{}, error) {
	return nil, nil
}
func (m *BaseMockStorage) UpdateObject(ctx context.Context, object interface{}) error {
	return nil
}
func (m *BaseMockStorage) DeleteObject(ctx context.Context, id string) error {
	return nil
}
func (m *BaseMockStorage) GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]interface{}, string, error) {
	return nil, "", nil
}

// Relationship operations
func (m *BaseMockStorage) CreateFollow(ctx context.Context, followerUsername, followedUsername, followActivityID string) error {
	return nil
}
func (m *BaseMockStorage) AcceptFollow(ctx context.Context, followerUsername, followedUsername string) error {
	return nil
}
func (m *BaseMockStorage) RejectFollow(ctx context.Context, followerUsername, followedUsername string) error {
	return nil
}
func (m *BaseMockStorage) RemoveFollow(ctx context.Context, followerUsername, followedUsername string) error {
	return nil
}
func (m *BaseMockStorage) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) IsFollowing(ctx context.Context, followerUsername, followedUsername string) (bool, error) {
	return false, nil
}

// Collection operations
func (m *BaseMockStorage) GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error) {
	return nil, nil
}

// OAuth operations
func (m *BaseMockStorage) CreateAuthorizationCode(ctx context.Context, code *storage.AuthorizationCode) error {
	return nil
}
func (m *BaseMockStorage) GetAuthorizationCode(ctx context.Context, code string) (*storage.AuthorizationCode, error) {
	return nil, nil
}
func (m *BaseMockStorage) DeleteAuthorizationCode(ctx context.Context, code string) error {
	return nil
}
func (m *BaseMockStorage) CreateRefreshToken(ctx context.Context, token *storage.RefreshToken) error {
	return nil
}
func (m *BaseMockStorage) GetRefreshToken(ctx context.Context, token string) (*storage.RefreshToken, error) {
	return nil, nil
}
func (m *BaseMockStorage) DeleteRefreshToken(ctx context.Context, token string) error {
	return nil
}

// User operations
func (m *BaseMockStorage) CreateUser(ctx context.Context, user *storage.User) error {
	return nil
}
func (m *BaseMockStorage) GetUser(ctx context.Context, username string) (*storage.User, error) {
	return nil, nil
}
func (m *BaseMockStorage) GetUserByEmail(ctx context.Context, email string) (*storage.User, error) {
	return nil, nil
}
func (m *BaseMockStorage) UpdateUser(ctx context.Context, username string, updates map[string]interface{}) error {
	return nil
}
func (m *BaseMockStorage) DeleteUser(ctx context.Context, username string) error {
	return nil
}
func (m *BaseMockStorage) ListUsers(ctx context.Context, limit int32, cursor string) ([]*storage.User, string, error) {
	return nil, "", nil
}

// OAuth Client operations
func (m *BaseMockStorage) CreateOAuthClient(ctx context.Context, client *storage.OAuthClient) error {
	return nil
}
func (m *BaseMockStorage) GetOAuthClient(ctx context.Context, clientID string) (*storage.OAuthClient, error) {
	return nil, nil
}
func (m *BaseMockStorage) UpdateOAuthClient(ctx context.Context, clientID string, updates map[string]interface{}) error {
	return nil
}
func (m *BaseMockStorage) DeleteOAuthClient(ctx context.Context, clientID string) error {
	return nil
}
func (m *BaseMockStorage) ListOAuthClients(ctx context.Context, limit int32, cursor string) ([]*storage.OAuthClient, string, error) {
	return nil, "", nil
}

// Like operations
func (m *BaseMockStorage) CreateLike(ctx context.Context, like *storage.Like) error {
	return nil
}
func (m *BaseMockStorage) GetLike(ctx context.Context, actor, object string) (*storage.Like, error) {
	return nil, nil
}
func (m *BaseMockStorage) DeleteLike(ctx context.Context, actor, object string) error {
	return nil
}
func (m *BaseMockStorage) GetObjectLikes(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Like, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) GetActorLikes(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Like, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) CountObjectLikes(ctx context.Context, objectID string) (int, error) {
	return 0, nil
}

// Announce operations
func (m *BaseMockStorage) CreateAnnounce(ctx context.Context, announce *storage.Announce) error {
	return nil
}
func (m *BaseMockStorage) GetAnnounce(ctx context.Context, actor, object string) (*storage.Announce, error) {
	return nil, nil
}
func (m *BaseMockStorage) DeleteAnnounce(ctx context.Context, actor, object string) error {
	return nil
}
func (m *BaseMockStorage) GetObjectAnnounces(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) GetActorAnnounces(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) CountObjectAnnounces(ctx context.Context, objectID string) (int, error) {
	return 0, nil
}

// Delete/Tombstone operations
func (m *BaseMockStorage) TombstoneObject(ctx context.Context, objectID string, deletedBy string) error {
	return nil
}
func (m *BaseMockStorage) GetTombstone(ctx context.Context, objectID string) (*storage.Tombstone, error) {
	return nil, nil
}
func (m *BaseMockStorage) CascadeDeleteLikes(ctx context.Context, objectID string) error {
	return nil
}
func (m *BaseMockStorage) CascadeDeleteAnnounces(ctx context.Context, objectID string) error {
	return nil
}

// Update history operations
func (m *BaseMockStorage) CreateUpdateHistory(ctx context.Context, history *storage.UpdateHistory) error {
	return nil
}
func (m *BaseMockStorage) GetUpdateHistory(ctx context.Context, objectID string, limit int) ([]*storage.UpdateHistory, error) {
	return nil, nil
}

// Block operations
func (m *BaseMockStorage) CreateBlock(ctx context.Context, block *storage.Block) error {
	return nil
}
func (m *BaseMockStorage) GetBlock(ctx context.Context, actor, blockedActor string) (*storage.Block, error) {
	return nil, nil
}
func (m *BaseMockStorage) DeleteBlock(ctx context.Context, actor, blockedActor string) error {
	return nil
}
func (m *BaseMockStorage) GetBlockedActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) GetBlockedByActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) IsBlocked(ctx context.Context, actor, targetActor string) (bool, error) {
	return false, nil
}
func (m *BaseMockStorage) IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error) {
	return false, nil
}

// Flag operations (content moderation)
func (m *BaseMockStorage) CreateFlag(ctx context.Context, flag *storage.Flag) error {
	return nil
}
func (m *BaseMockStorage) GetFlag(ctx context.Context, id string) (*storage.Flag, error) {
	return nil, nil
}
func (m *BaseMockStorage) GetFlagsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) GetFlagsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) GetPendingFlags(ctx context.Context, limit int, cursor string) ([]*storage.Flag, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) UpdateFlagStatus(ctx context.Context, id string, status storage.FlagStatus, reviewedBy string, reviewNote string) error {
	return nil
}
func (m *BaseMockStorage) CountPendingFlags(ctx context.Context) (int, error) {
	return 0, nil
}

// Move operations (account migration)
func (m *BaseMockStorage) CreateMove(ctx context.Context, move *storage.Move) error {
	return nil
}
func (m *BaseMockStorage) GetMove(ctx context.Context, actor string) (*storage.Move, error) {
	return nil, nil
}
func (m *BaseMockStorage) GetMoveByTarget(ctx context.Context, target string) ([]*storage.Move, error) {
	return nil, nil
}
func (m *BaseMockStorage) HasMovedFrom(ctx context.Context, oldActor, newActor string) (bool, error) {
	return false, nil
}

// Collection operations (Add/Remove activities)
func (m *BaseMockStorage) AddToCollection(ctx context.Context, collection string, item *storage.CollectionItem) error {
	return nil
}
func (m *BaseMockStorage) RemoveFromCollection(ctx context.Context, collection string, itemID string) error {
	return nil
}
func (m *BaseMockStorage) GetCollectionItems(ctx context.Context, collection string, limit int, cursor string) ([]*storage.CollectionItem, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) IsInCollection(ctx context.Context, collection string, itemID string) (bool, error) {
	return false, nil
}
func (m *BaseMockStorage) CountCollectionItems(ctx context.Context, collection string) (int, error) {
	return 0, nil
}

// Instance configuration operations
func (m *BaseMockStorage) GetInstanceRules(ctx context.Context) ([]storage.InstanceRule, error) {
	return []storage.InstanceRule{}, nil
}
func (m *BaseMockStorage) SetInstanceRules(ctx context.Context, rules []storage.InstanceRule) error {
	return nil
}
func (m *BaseMockStorage) GetExtendedDescription(ctx context.Context) (string, time.Time, error) {
	return "", time.Now(), nil
}
func (m *BaseMockStorage) SetExtendedDescription(ctx context.Context, description string) error {
	return nil
}

// Bookmark operations
func (m *BaseMockStorage) CreateBookmark(ctx context.Context, username, objectID string) error {
	return nil
}
func (m *BaseMockStorage) RemoveBookmark(ctx context.Context, username, objectID string) error {
	return nil
}
func (m *BaseMockStorage) GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return []string{}, "", nil
}
func (m *BaseMockStorage) IsBookmarked(ctx context.Context, username, objectID string) (bool, error) {
	return false, nil
}

// MockStorage is a testify mock implementation that embeds BaseMockStorage
// This provides default no-op implementations for all methods while allowing
// tests to set expectations on specific methods using testify's mock framework
type MockStorage struct {
	mock.Mock
	BaseMockStorage
}

// Override methods to use mock.Called() for testify expectations
// Only override the methods you need to set expectations on in your tests
// All other methods will use the BaseMockStorage no-op implementations

func (m *BaseMockStorage) DeleteExpiredTimelineEntries(ctx context.Context, before time.Time) error {
	return nil
}
