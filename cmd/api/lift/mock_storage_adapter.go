package lift

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/mock"
)

// MockStorageAdapter implements the storage.Storage interface for testing
type MockStorageAdapter struct {
	mock.Mock
}

// Actor operations
func (m *MockStorageAdapter) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	args := m.Called(ctx, actor, privateKey)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

func (m *MockStorageAdapter) GetActorByNumericID(ctx context.Context, numericID string) (*activitypub.Actor, error) {
	args := m.Called(ctx, numericID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

func (m *MockStorageAdapter) GetActorWithMetadata(ctx context.Context, username string) (*activitypub.Actor, *storage.ActorMetadata, error) {
	args := m.Called(ctx, username)
	var actor *activitypub.Actor
	var metadata *storage.ActorMetadata
	if args.Get(0) != nil {
		actor = args.Get(0).(*activitypub.Actor)
	}
	if args.Get(1) != nil {
		metadata = args.Get(1).(*storage.ActorMetadata)
	}
	return actor, metadata, args.Error(2)
}

func (m *MockStorageAdapter) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	args := m.Called(ctx, username)
	return args.String(0), args.Error(1)
}

func (m *MockStorageAdapter) UpdateActor(ctx context.Context, actor *activitypub.Actor) error {
	args := m.Called(ctx, actor)
	return args.Error(0)
}

func (m *MockStorageAdapter) UpdateActorLastStatusTime(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorageAdapter) SetActorFields(ctx context.Context, username string, fields []storage.ActorField) error {
	args := m.Called(ctx, username, fields)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteActor(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorageAdapter) SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error) {
	args := m.Called(ctx, query, limit, followingOnly, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*activitypub.Actor), args.Error(1)
}

func (m *MockStorageAdapter) GetSearchSuggestions(ctx context.Context, prefix string) ([]storage.SearchSuggestion, error) {
	args := m.Called(ctx, prefix)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.SearchSuggestion), args.Error(1)
}

func (m *MockStorageAdapter) SearchStatuses(ctx context.Context, query string, limit int) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, query, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorageAdapter) SearchStatusesWithOptions(ctx context.Context, query string, options storage.StatusSearchOptions) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, query, options)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorageAdapter) SearchStatusesByURL(ctx context.Context, url string) (*storage.StatusSearchResult, error) {
	args := m.Called(ctx, url)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorageAdapter) SearchAll(ctx context.Context, query string, limit int, accountID string) (*storage.SearchResults, error) {
	args := m.Called(ctx, query, limit, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SearchResults), args.Error(1)
}

func (m *MockStorageAdapter) SearchAccountsAdvanced(ctx context.Context, query string, resolve bool, limit int, offset int, following bool, accountID string) ([]*activitypub.Actor, error) {
	args := m.Called(ctx, query, resolve, limit, offset, following, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*activitypub.Actor), args.Error(1)
}

func (m *MockStorageAdapter) SearchStatusesAdvanced(ctx context.Context, query string, limit int, maxID, minID *string, accountID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, query, limit, maxID, minID, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorageAdapter) SearchHashtagsAdvanced(ctx context.Context, query string, limit int, accountID string) ([]*storage.HashtagSearchResult, error) {
	args := m.Called(ctx, query, limit, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.HashtagSearchResult), args.Error(1)
}

// Activity operations
func (m *MockStorageAdapter) CreateActivity(ctx context.Context, activity *activitypub.Activity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetActivity(ctx context.Context, id string) (*activitypub.Activity, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Activity), args.Error(1)
}

func (m *MockStorageAdapter) GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	var activities []*activitypub.Activity
	if args.Get(0) != nil {
		activities = args.Get(0).([]*activitypub.Activity)
	}
	return activities, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	var activities []*activitypub.Activity
	if args.Get(0) != nil {
		activities = args.Get(0).([]*activitypub.Activity)
	}
	return activities, args.String(1), args.Error(2)
}

// Object operations
func (m *MockStorageAdapter) CreateObject(ctx context.Context, object any) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetObject(ctx context.Context, id string) (any, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0), args.Error(1)
}

func (m *MockStorageAdapter) UpdateObject(ctx context.Context, object any) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteObject(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]any, string, error) {
	args := m.Called(ctx, actorID, cursor, limit)
	var objects []any
	if args.Get(0) != nil {
		objects = args.Get(0).([]any)
	}
	return objects, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) CountObjectReplies(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// Update history
func (m *MockStorageAdapter) CreateUpdateHistory(ctx context.Context, history *storage.UpdateHistory) error {
	args := m.Called(ctx, history)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetUpdateHistory(ctx context.Context, objectID string, limit int) ([]*storage.UpdateHistory, error) {
	args := m.Called(ctx, objectID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.UpdateHistory), args.Error(1)
}

// Follow operations
func (m *MockStorageAdapter) CreateFollow(ctx context.Context, followerUsername, followedUsername, followActivityID string) error {
	args := m.Called(ctx, followerUsername, followedUsername, followActivityID)
	return args.Error(0)
}

func (m *MockStorageAdapter) AcceptFollow(ctx context.Context, followerUsername, followedUsername string) error {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Error(0)
}

func (m *MockStorageAdapter) RejectFollow(ctx context.Context, followerUsername, followedUsername string) error {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Error(0)
}

func (m *MockStorageAdapter) RemoveFollow(ctx context.Context, followerUsername, followedUsername string) error {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	var followers []string
	if args.Get(0) != nil {
		followers = args.Get(0).([]string)
	}
	return followers, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	var following []string
	if args.Get(0) != nil {
		following = args.Get(0).([]string)
	}
	return following, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) IsFollowing(ctx context.Context, followerUsername, followedUsername string) (bool, error) {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageAdapter) GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	var requests []string
	if args.Get(0) != nil {
		requests = args.Get(0).([]string)
	}
	return requests, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetFollowRequestState(ctx context.Context, followerUsername, followedUsername string) (string, error) {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.String(0), args.Error(1)
}

// Collection operations
func (m *MockStorageAdapter) GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error) {
	args := m.Called(ctx, username, collectionType, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.OrderedCollectionPage), args.Error(1)
}

// OAuth operations
func (m *MockStorageAdapter) CreateAuthorizationCode(ctx context.Context, code *storage.AuthorizationCode) error {
	args := m.Called(ctx, code)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetAuthorizationCode(ctx context.Context, code string) (*storage.AuthorizationCode, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.AuthorizationCode), args.Error(1)
}

func (m *MockStorageAdapter) DeleteAuthorizationCode(ctx context.Context, code string) error {
	args := m.Called(ctx, code)
	return args.Error(0)
}

func (m *MockStorageAdapter) CreateRefreshToken(ctx context.Context, token *storage.RefreshToken) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetRefreshToken(ctx context.Context, token string) (*storage.RefreshToken, error) {
	args := m.Called(ctx, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.RefreshToken), args.Error(1)
}

func (m *MockStorageAdapter) DeleteRefreshToken(ctx context.Context, token string) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

// User operations
func (m *MockStorageAdapter) CreateUser(ctx context.Context, user *storage.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetUser(ctx context.Context, username string) (*storage.User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

func (m *MockStorageAdapter) GetUserByEmail(ctx context.Context, email string) (*storage.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

func (m *MockStorageAdapter) UpdateUser(ctx context.Context, username string, updates map[string]any) error {
	args := m.Called(ctx, username, updates)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteUser(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorageAdapter) ListUsers(ctx context.Context, limit int32, cursor string) ([]*storage.User, string, error) {
	args := m.Called(ctx, limit, cursor)
	var users []*storage.User
	if args.Get(0) != nil {
		users = args.Get(0).([]*storage.User)
	}
	return users, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetActiveUserCount(ctx context.Context, days int) (int64, error) {
	args := m.Called(ctx, days)
	return args.Get(0).(int64), args.Error(1)
}

// Statistics operations
func (m *MockStorageAdapter) GetTotalUserCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockStorageAdapter) GetTotalStatusCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockStorageAdapter) GetTotalDomainCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockStorageAdapter) GetWeeklyActivity(ctx context.Context, weekTimestamp int64) (*storage.WeeklyActivity, error) {
	args := m.Called(ctx, weekTimestamp)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.WeeklyActivity), args.Error(1)
}

func (m *MockStorageAdapter) RecordActivity(ctx context.Context, activityType string, actorID string, timestamp time.Time) error {
	args := m.Called(ctx, activityType, actorID, timestamp)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetContactAccount(ctx context.Context) (*storage.ActorRecord, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ActorRecord), args.Error(1)
}

// Provider operations
func (m *MockStorageAdapter) GetUserByProviderID(ctx context.Context, provider, providerID string) (*storage.User, error) {
	args := m.Called(ctx, provider, providerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

func (m *MockStorageAdapter) LinkProviderAccount(ctx context.Context, username, provider, providerID string) error {
	args := m.Called(ctx, username, provider, providerID)
	return args.Error(0)
}

func (m *MockStorageAdapter) UnlinkProviderAccount(ctx context.Context, username, provider string) error {
	args := m.Called(ctx, username, provider)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetLinkedProviders(ctx context.Context, username string) ([]string, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// Recovery token operations
func (m *MockStorageAdapter) StoreRecoveryToken(ctx context.Context, key string, data map[string]any) error {
	args := m.Called(ctx, key, data)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetRecoveryToken(ctx context.Context, key string) (map[string]any, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]any), args.Error(1)
}

func (m *MockStorageAdapter) DeleteRecoveryToken(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

// OAuth state operations
func (m *MockStorageAdapter) StoreOAuthState(ctx context.Context, state string, data *storage.OAuthState) error {
	args := m.Called(ctx, state, data)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetOAuthState(ctx context.Context, state string) (*storage.OAuthState, error) {
	args := m.Called(ctx, state)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.OAuthState), args.Error(1)
}

func (m *MockStorageAdapter) DeleteOAuthState(ctx context.Context, state string) error {
	args := m.Called(ctx, state)
	return args.Error(0)
}

// OAuth client operations
func (m *MockStorageAdapter) CreateOAuthClient(ctx context.Context, client *storage.OAuthClient) error {
	args := m.Called(ctx, client)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetOAuthClient(ctx context.Context, clientID string) (*storage.OAuthClient, error) {
	args := m.Called(ctx, clientID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.OAuthClient), args.Error(1)
}

func (m *MockStorageAdapter) UpdateOAuthClient(ctx context.Context, clientID string, updates map[string]any) error {
	args := m.Called(ctx, clientID, updates)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteOAuthClient(ctx context.Context, clientID string) error {
	args := m.Called(ctx, clientID)
	return args.Error(0)
}

func (m *MockStorageAdapter) ListOAuthClients(ctx context.Context, limit int32, cursor string) ([]*storage.OAuthClient, string, error) {
	args := m.Called(ctx, limit, cursor)
	var clients []*storage.OAuthClient
	if args.Get(0) != nil {
		clients = args.Get(0).([]*storage.OAuthClient)
	}
	return clients, args.String(1), args.Error(2)
}

// Like operations
func (m *MockStorageAdapter) CreateLike(ctx context.Context, like *storage.Like) error {
	args := m.Called(ctx, like)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetLike(ctx context.Context, actor, object string) (*storage.Like, error) {
	args := m.Called(ctx, actor, object)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Like), args.Error(1)
}

func (m *MockStorageAdapter) DeleteLike(ctx context.Context, actor, object string) error {
	args := m.Called(ctx, actor, object)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetObjectLikes(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Like, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	var likes []*storage.Like
	if args.Get(0) != nil {
		likes = args.Get(0).([]*storage.Like)
	}
	return likes, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetActorLikes(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Like, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	var likes []*storage.Like
	if args.Get(0) != nil {
		likes = args.Get(0).([]*storage.Like)
	}
	return likes, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) CountObjectLikes(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// Announce operations
func (m *MockStorageAdapter) CreateAnnounce(ctx context.Context, announce *storage.Announce) error {
	args := m.Called(ctx, announce)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetAnnounce(ctx context.Context, actor, object string) (*storage.Announce, error) {
	args := m.Called(ctx, actor, object)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Announce), args.Error(1)
}

func (m *MockStorageAdapter) DeleteAnnounce(ctx context.Context, actor, object string) error {
	args := m.Called(ctx, actor, object)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetObjectAnnounces(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	var announces []*storage.Announce
	if args.Get(0) != nil {
		announces = args.Get(0).([]*storage.Announce)
	}
	return announces, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetActorAnnounces(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	var announces []*storage.Announce
	if args.Get(0) != nil {
		announces = args.Get(0).([]*storage.Announce)
	}
	return announces, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) CountObjectAnnounces(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// Tombstone operations
func (m *MockStorageAdapter) TombstoneObject(ctx context.Context, objectID string, deletedBy string) error {
	args := m.Called(ctx, objectID, deletedBy)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetTombstone(ctx context.Context, objectID string) (*storage.Tombstone, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Tombstone), args.Error(1)
}

func (m *MockStorageAdapter) CascadeDeleteLikes(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

func (m *MockStorageAdapter) CascadeDeleteAnnounces(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// Block operations
func (m *MockStorageAdapter) CreateBlock(ctx context.Context, block *storage.Block) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetBlock(ctx context.Context, actor, blockedActor string) (*storage.Block, error) {
	args := m.Called(ctx, actor, blockedActor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Block), args.Error(1)
}

func (m *MockStorageAdapter) DeleteBlock(ctx context.Context, actor, blockedActor string) error {
	args := m.Called(ctx, actor, blockedActor)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetBlockedActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	args := m.Called(ctx, actor, limit, cursor)
	var blocks []*storage.Block
	if args.Get(0) != nil {
		blocks = args.Get(0).([]*storage.Block)
	}
	return blocks, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetBlockedByActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	args := m.Called(ctx, actor, limit, cursor)
	var blocks []*storage.Block
	if args.Get(0) != nil {
		blocks = args.Get(0).([]*storage.Block)
	}
	return blocks, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) IsBlocked(ctx context.Context, actor, targetActor string) (bool, error) {
	args := m.Called(ctx, actor, targetActor)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageAdapter) IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error) {
	args := m.Called(ctx, actor1, actor2)
	return args.Bool(0), args.Error(1)
}

// Flag operations
func (m *MockStorageAdapter) CreateFlag(ctx context.Context, flag *storage.Flag) error {
	args := m.Called(ctx, flag)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetFlag(ctx context.Context, id string) (*storage.Flag, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Flag), args.Error(1)
}

func (m *MockStorageAdapter) GetFlagsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	var flags []*storage.Flag
	if args.Get(0) != nil {
		flags = args.Get(0).([]*storage.Flag)
	}
	return flags, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetFlagsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	var flags []*storage.Flag
	if args.Get(0) != nil {
		flags = args.Get(0).([]*storage.Flag)
	}
	return flags, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetPendingFlags(ctx context.Context, limit int, cursor string) ([]*storage.Flag, string, error) {
	args := m.Called(ctx, limit, cursor)
	var flags []*storage.Flag
	if args.Get(0) != nil {
		flags = args.Get(0).([]*storage.Flag)
	}
	return flags, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) UpdateFlagStatus(ctx context.Context, id string, status storage.FlagStatus, reviewedBy string, reviewNote string) error {
	args := m.Called(ctx, id, status, reviewedBy, reviewNote)
	return args.Error(0)
}

func (m *MockStorageAdapter) CountPendingFlags(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

// Move operations
func (m *MockStorageAdapter) CreateMove(ctx context.Context, move *storage.Move) error {
	args := m.Called(ctx, move)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetMove(ctx context.Context, actor string) (*storage.Move, error) {
	args := m.Called(ctx, actor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Move), args.Error(1)
}

func (m *MockStorageAdapter) GetMoveByTarget(ctx context.Context, target string) ([]*storage.Move, error) {
	args := m.Called(ctx, target)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Move), args.Error(1)
}

func (m *MockStorageAdapter) HasMovedFrom(ctx context.Context, oldActor, newActor string) (bool, error) {
	args := m.Called(ctx, oldActor, newActor)
	return args.Bool(0), args.Error(1)
}

// Collection operations
func (m *MockStorageAdapter) AddToCollection(ctx context.Context, collection string, item *storage.CollectionItem) error {
	args := m.Called(ctx, collection, item)
	return args.Error(0)
}

func (m *MockStorageAdapter) RemoveFromCollection(ctx context.Context, collection string, itemID string) error {
	args := m.Called(ctx, collection, itemID)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetCollectionItems(ctx context.Context, collection string, limit int, cursor string) ([]*storage.CollectionItem, string, error) {
	args := m.Called(ctx, collection, limit, cursor)
	var items []*storage.CollectionItem
	if args.Get(0) != nil {
		items = args.Get(0).([]*storage.CollectionItem)
	}
	return items, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) IsInCollection(ctx context.Context, collection string, itemID string) (bool, error) {
	args := m.Called(ctx, collection, itemID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageAdapter) CountCollectionItems(ctx context.Context, collection string) (int, error) {
	args := m.Called(ctx, collection)
	return args.Int(0), args.Error(1)
}

// Timeline operations
func (m *MockStorageAdapter) WriteToTimeline(ctx context.Context, timeline *storage.TimelineEntry) error {
	args := m.Called(ctx, timeline)
	return args.Error(0)
}

func (m *MockStorageAdapter) WriteToTimelines(ctx context.Context, entries []*storage.TimelineEntry) error {
	args := m.Called(ctx, entries)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetHomeTimeline(ctx context.Context, username string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	var entries []*storage.TimelineEntry
	if args.Get(0) != nil {
		entries = args.Get(0).([]*storage.TimelineEntry)
	}
	return entries, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetPublicTimeline(ctx context.Context, local bool, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, local, limit, cursor)
	var entries []*storage.TimelineEntry
	if args.Get(0) != nil {
		entries = args.Get(0).([]*storage.TimelineEntry)
	}
	return entries, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, listID, limit, cursor)
	var entries []*storage.TimelineEntry
	if args.Get(0) != nil {
		entries = args.Get(0).([]*storage.TimelineEntry)
	}
	return entries, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetDirectTimeline(ctx context.Context, username string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	var entries []*storage.TimelineEntry
	if args.Get(0) != nil {
		entries = args.Get(0).([]*storage.TimelineEntry)
	}
	return entries, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetHashtagTimeline(ctx context.Context, hashtag string, local bool, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, hashtag, local, limit, cursor)
	var entries []*storage.TimelineEntry
	if args.Get(0) != nil {
		entries = args.Get(0).([]*storage.TimelineEntry)
	}
	return entries, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) DeleteFromTimeline(ctx context.Context, timelineType, timelineID, entryID string) error {
	args := m.Called(ctx, timelineType, timelineID, entryID)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteExpiredTimelineEntries(ctx context.Context, before time.Time) error {
	args := m.Called(ctx, before)
	return args.Error(0)
}

func (m *MockStorageAdapter) FanOutPost(ctx context.Context, activity *activitypub.Activity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

// Instance operations
func (m *MockStorageAdapter) GetInstanceRules(ctx context.Context) ([]storage.InstanceRule, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.InstanceRule), args.Error(1)
}

func (m *MockStorageAdapter) SetInstanceRules(ctx context.Context, rules []storage.InstanceRule) error {
	args := m.Called(ctx, rules)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetExtendedDescription(ctx context.Context) (string, time.Time, error) {
	args := m.Called(ctx)
	return args.String(0), args.Get(1).(time.Time), args.Error(2)
}

func (m *MockStorageAdapter) SetExtendedDescription(ctx context.Context, description string) error {
	args := m.Called(ctx, description)
	return args.Error(0)
}

// Bookmark operations
func (m *MockStorageAdapter) CreateBookmark(ctx context.Context, username, objectID string) error {
	args := m.Called(ctx, username, objectID)
	return args.Error(0)
}

func (m *MockStorageAdapter) RemoveBookmark(ctx context.Context, username, objectID string) error {
	args := m.Called(ctx, username, objectID)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	var bookmarks []string
	if args.Get(0) != nil {
		bookmarks = args.Get(0).([]string)
	}
	return bookmarks, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) IsBookmarked(ctx context.Context, username, objectID string) (bool, error) {
	args := m.Called(ctx, username, objectID)
	return args.Bool(0), args.Error(1)
}

// Conversation operations
func (m *MockStorageAdapter) CreateConversation(ctx context.Context, conversation *storage.Conversation) error {
	args := m.Called(ctx, conversation)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetConversation(ctx context.Context, id string) (*storage.Conversation, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Conversation), args.Error(1)
}

func (m *MockStorageAdapter) GetConversationByParticipants(ctx context.Context, participants []string) (*storage.Conversation, error) {
	args := m.Called(ctx, participants)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Conversation), args.Error(1)
}

func (m *MockStorageAdapter) UpdateConversationLastStatus(ctx context.Context, id, lastStatusID string) error {
	args := m.Called(ctx, id, lastStatusID)
	return args.Error(0)
}

func (m *MockStorageAdapter) MarkConversationRead(ctx context.Context, id, username string) error {
	args := m.Called(ctx, id, username)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteConversation(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetUserConversations(ctx context.Context, username string, limit int, cursor string) ([]*storage.Conversation, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	var conversations []*storage.Conversation
	if args.Get(0) != nil {
		conversations = args.Get(0).([]*storage.Conversation)
	}
	return conversations, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) AddParticipantToConversation(ctx context.Context, conversationID, participantID string) error {
	args := m.Called(ctx, conversationID, participantID)
	return args.Error(0)
}

// List operations
func (m *MockStorageAdapter) CreateList(ctx context.Context, username, title, repliesPolicy string) (*storage.List, error) {
	args := m.Called(ctx, username, title, repliesPolicy)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.List), args.Error(1)
}

func (m *MockStorageAdapter) GetList(ctx context.Context, listID string) (*storage.List, error) {
	args := m.Called(ctx, listID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.List), args.Error(1)
}

func (m *MockStorageAdapter) GetListsForUser(ctx context.Context, username string) ([]*storage.List, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.List), args.Error(1)
}

func (m *MockStorageAdapter) UpdateList(ctx context.Context, listID string, updates map[string]any) error {
	args := m.Called(ctx, listID, updates)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteList(ctx context.Context, listID string) error {
	args := m.Called(ctx, listID)
	return args.Error(0)
}

func (m *MockStorageAdapter) AddAccountsToList(ctx context.Context, listID string, accountIDs []string) error {
	args := m.Called(ctx, listID, accountIDs)
	return args.Error(0)
}

func (m *MockStorageAdapter) RemoveAccountsFromList(ctx context.Context, listID string, accountIDs []string) error {
	args := m.Called(ctx, listID, accountIDs)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetListAccounts(ctx context.Context, listID string) ([]string, error) {
	args := m.Called(ctx, listID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockStorageAdapter) IsAccountInList(ctx context.Context, listID, accountID string) (bool, error) {
	args := m.Called(ctx, listID, accountID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageAdapter) GetListsContainingAccount(ctx context.Context, accountID, username string) ([]*storage.List, error) {
	args := m.Called(ctx, accountID, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.List), args.Error(1)
}

// Notification operations
func (m *MockStorageAdapter) CreateNotification(ctx context.Context, notification *storage.Notification) error {
	args := m.Called(ctx, notification)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetNotification(ctx context.Context, id string) (*storage.Notification, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Notification), args.Error(1)
}

func (m *MockStorageAdapter) GetNotifications(ctx context.Context, username string, limit int, cursor string) ([]*storage.Notification, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	var notifications []*storage.Notification
	if args.Get(0) != nil {
		notifications = args.Get(0).([]*storage.Notification)
	}
	return notifications, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetNotificationsFiltered(ctx context.Context, username string, filter *storage.NotificationFilter) ([]*storage.Notification, string, error) {
	args := m.Called(ctx, username, filter)
	var notifications []*storage.Notification
	if args.Get(0) != nil {
		notifications = args.Get(0).([]*storage.Notification)
	}
	return notifications, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) MarkNotificationAsRead(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorageAdapter) MarkAllNotificationsAsRead(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteNotification(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorageAdapter) ClearNotifications(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorageAdapter) CountUnreadNotifications(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

func (m *MockStorageAdapter) GetNotificationsAdvanced(ctx context.Context, userID string, excludeTypes []string, maxID, sinceID, minID *string, limit int, includeFiltered bool) ([]*storage.Notification, error) {
	args := m.Called(ctx, userID, excludeTypes, maxID, sinceID, minID, limit, includeFiltered)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Notification), args.Error(1)
}

func (m *MockStorageAdapter) GetNotificationsByAccount(ctx context.Context, userID, accountID string, limit int) ([]*storage.Notification, error) {
	args := m.Called(ctx, userID, accountID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Notification), args.Error(1)
}

func (m *MockStorageAdapter) GetUnreadNotificationCount(ctx context.Context, userID string) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockStorageAdapter) GetNotificationPreferences(ctx context.Context, username string) (*storage.NotificationPreferences, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.NotificationPreferences), args.Error(1)
}

func (m *MockStorageAdapter) UpdateNotificationPreferences(ctx context.Context, username string, prefs *storage.NotificationPreferences) error {
	args := m.Called(ctx, username, prefs)
	return args.Error(0)
}

func (m *MockStorageAdapter) BatchMarkNotificationsAsRead(ctx context.Context, username string, notificationIDs []string) error {
	args := m.Called(ctx, username, notificationIDs)
	return args.Error(0)
}

// Cache operations
func (m *MockStorageAdapter) CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	args := m.Called(ctx, handle, actor, ttl)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error) {
	args := m.Called(ctx, handle)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

// Push subscription operations
func (m *MockStorageAdapter) CreatePushSubscription(ctx context.Context, username string, subscription *storage.PushSubscription) error {
	args := m.Called(ctx, username, subscription)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetPushSubscription(ctx context.Context, username, subscriptionID string) (*storage.PushSubscription, error) {
	args := m.Called(ctx, username, subscriptionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.PushSubscription), args.Error(1)
}

func (m *MockStorageAdapter) GetUserPushSubscriptions(ctx context.Context, username string) ([]*storage.PushSubscription, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.PushSubscription), args.Error(1)
}

func (m *MockStorageAdapter) UpdatePushSubscription(ctx context.Context, username, subscriptionID string, alerts storage.PushSubscriptionAlerts) error {
	args := m.Called(ctx, username, subscriptionID, alerts)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeletePushSubscription(ctx context.Context, username, subscriptionID string) error {
	args := m.Called(ctx, username, subscriptionID)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteAllPushSubscriptions(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// VAPID keys
func (m *MockStorageAdapter) GetVAPIDKeys(ctx context.Context) (*storage.VAPIDKeys, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.VAPIDKeys), args.Error(1)
}

func (m *MockStorageAdapter) SetVAPIDKeys(ctx context.Context, keys *storage.VAPIDKeys) error {
	args := m.Called(ctx, keys)
	return args.Error(0)
}

// Poll operations
func (m *MockStorageAdapter) CreatePoll(ctx context.Context, poll *storage.Poll) error {
	args := m.Called(ctx, poll)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetPoll(ctx context.Context, pollID string) (*storage.Poll, error) {
	args := m.Called(ctx, pollID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Poll), args.Error(1)
}

func (m *MockStorageAdapter) GetPollByStatusID(ctx context.Context, statusID string) (*storage.Poll, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Poll), args.Error(1)
}

func (m *MockStorageAdapter) VoteOnPoll(ctx context.Context, pollID string, voterID string, choices []int) error {
	args := m.Called(ctx, pollID, voterID, choices)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetPollVotes(ctx context.Context, pollID string) (map[string][]int, error) {
	args := m.Called(ctx, pollID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string][]int), args.Error(1)
}

func (m *MockStorageAdapter) HasUserVoted(ctx context.Context, pollID string, userID string) (bool, []int, error) {
	args := m.Called(ctx, pollID, userID)
	var choices []int
	if args.Get(1) != nil {
		choices = args.Get(1).([]int)
	}
	return args.Bool(0), choices, args.Error(2)
}

// Mute operations
func (m *MockStorageAdapter) CreateMute(ctx context.Context, mute *storage.Mute) error {
	args := m.Called(ctx, mute)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetMute(ctx context.Context, actor, mutedActor string) (*storage.Mute, error) {
	args := m.Called(ctx, actor, mutedActor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Mute), args.Error(1)
}

func (m *MockStorageAdapter) DeleteMute(ctx context.Context, actor, mutedActor string) error {
	args := m.Called(ctx, actor, mutedActor)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetMutedActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Mute, string, error) {
	args := m.Called(ctx, actor, limit, cursor)
	var mutes []*storage.Mute
	if args.Get(0) != nil {
		mutes = args.Get(0).([]*storage.Mute)
	}
	return mutes, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) IsMuted(ctx context.Context, actor, targetActor string) (bool, error) {
	args := m.Called(ctx, actor, targetActor)
	return args.Bool(0), args.Error(1)
}

// Filter operations
func (m *MockStorageAdapter) CreateFilter(ctx context.Context, filter *storage.Filter) error {
	args := m.Called(ctx, filter)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetFilter(ctx context.Context, filterID string) (*storage.Filter, error) {
	args := m.Called(ctx, filterID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Filter), args.Error(1)
}

func (m *MockStorageAdapter) GetFiltersForUser(ctx context.Context, username string) ([]*storage.Filter, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Filter), args.Error(1)
}

func (m *MockStorageAdapter) UpdateFilter(ctx context.Context, filterID string, updates map[string]any) error {
	args := m.Called(ctx, filterID, updates)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteFilter(ctx context.Context, filterID string) error {
	args := m.Called(ctx, filterID)
	return args.Error(0)
}

// Filter keyword operations
func (m *MockStorageAdapter) AddFilterKeyword(ctx context.Context, filterID string, keyword *storage.FilterKeyword) error {
	args := m.Called(ctx, filterID, keyword)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetFilterKeywords(ctx context.Context, filterID string) ([]*storage.FilterKeyword, error) {
	args := m.Called(ctx, filterID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FilterKeyword), args.Error(1)
}

func (m *MockStorageAdapter) UpdateFilterKeyword(ctx context.Context, keywordID string, updates map[string]any) error {
	args := m.Called(ctx, keywordID, updates)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteFilterKeyword(ctx context.Context, keywordID string) error {
	args := m.Called(ctx, keywordID)
	return args.Error(0)
}

// Filter status operations
func (m *MockStorageAdapter) AddFilterStatus(ctx context.Context, filterID string, status *storage.FilterStatus) error {
	args := m.Called(ctx, filterID, status)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetFilterStatuses(ctx context.Context, filterID string) ([]*storage.FilterStatus, error) {
	args := m.Called(ctx, filterID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FilterStatus), args.Error(1)
}

func (m *MockStorageAdapter) DeleteFilterStatus(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

// Moderation operations
func (m *MockStorageAdapter) CreateModerationEvent(ctx context.Context, event *storage.ModerationEvent) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetModerationEvent(ctx context.Context, eventID string) (*storage.ModerationEvent, error) {
	args := m.Called(ctx, eventID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ModerationEvent), args.Error(1)
}

func (m *MockStorageAdapter) GetModerationQueuePaginated(ctx context.Context, limit int, cursor string) ([]*storage.ModerationQueueItem, string, error) {
	args := m.Called(ctx, limit, cursor)
	var items []*storage.ModerationQueueItem
	if args.Get(0) != nil {
		items = args.Get(0).([]*storage.ModerationQueueItem)
	}
	return items, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetModerationEventsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	var events []*storage.ModerationEvent
	if args.Get(0) != nil {
		events = args.Get(0).([]*storage.ModerationEvent)
	}
	return events, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetModerationEventsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	var events []*storage.ModerationEvent
	if args.Get(0) != nil {
		events = args.Get(0).([]*storage.ModerationEvent)
	}
	return events, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) AddModerationReview(ctx context.Context, review *storage.ModerationReview) error {
	args := m.Called(ctx, review)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetModerationReviews(ctx context.Context, eventID string) ([]*storage.ModerationReview, error) {
	args := m.Called(ctx, eventID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.ModerationReview), args.Error(1)
}

func (m *MockStorageAdapter) CreateModerationDecision(ctx context.Context, decision *storage.ModerationDecision) error {
	args := m.Called(ctx, decision)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetModerationDecision(ctx context.Context, objectID string) (*storage.ModerationDecision, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ModerationDecision), args.Error(1)
}

func (m *MockStorageAdapter) GetModerationHistory(ctx context.Context, objectID string) (*storage.ModerationHistory, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ModerationHistory), args.Error(1)
}

func (m *MockStorageAdapter) GetModerationEvents(ctx context.Context, filter *storage.ModerationEventFilter, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	args := m.Called(ctx, filter, limit, cursor)
	var events []*storage.ModerationEvent
	if args.Get(0) != nil {
		events = args.Get(0).([]*storage.ModerationEvent)
	}
	return events, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) CreateAdminReview(ctx context.Context, eventID string, adminID string, action storage.ActionType, reason string) error {
	args := m.Called(ctx, eventID, adminID, action, reason)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetReviewerStats(ctx context.Context, reviewerID string) (*storage.ReviewerStats, error) {
	args := m.Called(ctx, reviewerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ReviewerStats), args.Error(1)
}

// Trust operations
func (m *MockStorageAdapter) CreateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	args := m.Called(ctx, relationship)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error) {
	args := m.Called(ctx, trusterID, trusteeID, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.TrustRelationship), args.Error(1)
}

func (m *MockStorageAdapter) UpdateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	args := m.Called(ctx, relationship)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) error {
	args := m.Called(ctx, trusterID, trusteeID, category)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	args := m.Called(ctx, trusterID, limit, cursor)
	var relationships []*storage.TrustRelationship
	if args.Get(0) != nil {
		relationships = args.Get(0).([]*storage.TrustRelationship)
	}
	return relationships, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	args := m.Called(ctx, trusteeID, limit, cursor)
	var relationships []*storage.TrustRelationship
	if args.Get(0) != nil {
		relationships = args.Get(0).([]*storage.TrustRelationship)
	}
	return relationships, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error) {
	args := m.Called(ctx, actorID, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.TrustScore), args.Error(1)
}

func (m *MockStorageAdapter) UpdateTrustScore(ctx context.Context, score *storage.TrustScore) error {
	args := m.Called(ctx, score)
	return args.Error(0)
}

func (m *MockStorageAdapter) RecordTrustUpdate(ctx context.Context, update *storage.TrustUpdate) error {
	args := m.Called(ctx, update)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetAllTrustRelationships(ctx context.Context, limit int) ([]*storage.TrustRelationship, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrustRelationship), args.Error(1)
}

// Account operations
func (m *MockStorageAdapter) CreateAccountPin(ctx context.Context, pin *storage.AccountPin) error {
	args := m.Called(ctx, pin)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteAccountPin(ctx context.Context, username, pinnedActorID string) error {
	args := m.Called(ctx, username, pinnedActorID)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetAccountPins(ctx context.Context, username string) ([]*storage.AccountPin, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.AccountPin), args.Error(1)
}

func (m *MockStorageAdapter) IsAccountPinned(ctx context.Context, username, actorID string) (bool, error) {
	args := m.Called(ctx, username, actorID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageAdapter) CreateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	args := m.Called(ctx, note)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetAccountNote(ctx context.Context, username, targetActorID string) (*storage.AccountNote, error) {
	args := m.Called(ctx, username, targetActorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.AccountNote), args.Error(1)
}

func (m *MockStorageAdapter) UpdateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	args := m.Called(ctx, note)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteAccountNote(ctx context.Context, username, targetActorID string) error {
	args := m.Called(ctx, username, targetActorID)
	return args.Error(0)
}

func (m *MockStorageAdapter) RemoveFromFollowers(ctx context.Context, username, followerUsername string) error {
	args := m.Called(ctx, username, followerUsername)
	return args.Error(0)
}

// Status pin operations
func (m *MockStorageAdapter) CreateStatusPin(ctx context.Context, pin *storage.StatusPin) error {
	args := m.Called(ctx, pin)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteStatusPin(ctx context.Context, username, statusID string) error {
	args := m.Called(ctx, username, statusID)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetStatusPins(ctx context.Context, username string) ([]*storage.StatusPin, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusPin), args.Error(1)
}

func (m *MockStorageAdapter) IsStatusPinned(ctx context.Context, username, statusID string) (bool, error) {
	args := m.Called(ctx, username, statusID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageAdapter) CountUserPinnedStatuses(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

// Conversation muting operations
func (m *MockStorageAdapter) CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error {
	args := m.Called(ctx, mute)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteConversationMute(ctx context.Context, username, conversationID string) error {
	args := m.Called(ctx, username, conversationID)
	return args.Error(0)
}

func (m *MockStorageAdapter) IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error) {
	args := m.Called(ctx, username, conversationID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageAdapter) GetMutedConversations(ctx context.Context, username string) ([]string, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// Scheduled status operations
func (m *MockStorageAdapter) CreateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	args := m.Called(ctx, scheduled)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetScheduledStatus(ctx context.Context, id string) (*storage.ScheduledStatus, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ScheduledStatus), args.Error(1)
}

func (m *MockStorageAdapter) GetScheduledStatuses(ctx context.Context, username string, limit int, cursor string) ([]*storage.ScheduledStatus, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	var statuses []*storage.ScheduledStatus
	if args.Get(0) != nil {
		statuses = args.Get(0).([]*storage.ScheduledStatus)
	}
	return statuses, args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) UpdateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	args := m.Called(ctx, scheduled)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteScheduledStatus(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetDueScheduledStatuses(ctx context.Context, before time.Time, limit int) ([]*storage.ScheduledStatus, error) {
	args := m.Called(ctx, before, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.ScheduledStatus), args.Error(1)
}

func (m *MockStorageAdapter) MarkScheduledStatusPublished(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// Hashtag following
func (m *MockStorageAdapter) FollowHashtag(ctx context.Context, userID string, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

func (m *MockStorageAdapter) UnfollowHashtag(ctx context.Context, userID string, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

func (m *MockStorageAdapter) IsFollowingHashtag(ctx context.Context, userID string, hashtag string) (bool, error) {
	args := m.Called(ctx, userID, hashtag)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageAdapter) GetFollowedHashtags(ctx context.Context, userID string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, userID, limit, cursor)
	var hashtags []string
	if args.Get(0) != nil {
		hashtags = args.Get(0).([]string)
	}
	return hashtags, args.String(1), args.Error(2)
}

// Enhanced hashtag operations for GraphQL
func (m *MockStorageAdapter) UpdateHashtagNotificationSettings(ctx context.Context, userID, hashtag string, notify bool) error {
	args := m.Called(ctx, userID, hashtag, notify)
	return args.Error(0)
}

func (m *MockStorageAdapter) MuteHashtag(ctx context.Context, userID, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

func (m *MockStorageAdapter) UnmuteHashtag(ctx context.Context, userID, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

func (m *MockStorageAdapter) IsHashtagMuted(ctx context.Context, userID, hashtag string) (bool, error) {
	args := m.Called(ctx, userID, hashtag)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageAdapter) GetHashtagTimelineAdvanced(ctx context.Context, hashtag string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, hashtag, maxID, limit, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorageAdapter) GetMultiHashtagTimeline(ctx context.Context, hashtags []string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, hashtags, maxID, limit, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorageAdapter) GetSuggestedHashtags(ctx context.Context, userID string, limit int) ([]*storage.HashtagSearchResult, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.HashtagSearchResult), args.Error(1)
}

func (m *MockStorageAdapter) GetHashtagActivity(ctx context.Context, hashtag string, since time.Time) ([]*storage.Activity, error) {
	args := m.Called(ctx, hashtag, since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Activity), args.Error(1)
}

// Featured tags
func (m *MockStorageAdapter) CreateFeaturedTag(ctx context.Context, userID string, tagName string) (*storage.FeaturedTag, error) {
	args := m.Called(ctx, userID, tagName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.FeaturedTag), args.Error(1)
}

func (m *MockStorageAdapter) DeleteFeaturedTag(ctx context.Context, userID string, featuredTagID string) error {
	args := m.Called(ctx, userID, featuredTagID)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetFeaturedTags(ctx context.Context, userID string) ([]*storage.FeaturedTag, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FeaturedTag), args.Error(1)
}

func (m *MockStorageAdapter) GetTagSuggestions(ctx context.Context, userID string, limit int) ([]string, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// Hashtag operations
func (m *MockStorageAdapter) IndexHashtag(ctx context.Context, hashtag string, statusID string, authorID string, visibility string) error {
	args := m.Called(ctx, hashtag, statusID, authorID, visibility)
	return args.Error(0)
}

func (m *MockStorageAdapter) SearchHashtags(ctx context.Context, query string, limit int) ([]*storage.Hashtag, error) {
	args := m.Called(ctx, query, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Hashtag), args.Error(1)
}

func (m *MockStorageAdapter) GetHashtagInfo(ctx context.Context, hashtag string) (*storage.Hashtag, error) {
	args := m.Called(ctx, hashtag)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Hashtag), args.Error(1)
}

func (m *MockStorageAdapter) GetHashtagUsageHistory(ctx context.Context, hashtag string, days int) ([]int64, error) {
	args := m.Called(ctx, hashtag, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]int64), args.Error(1)
}

// Language detection and user preferences
func (m *MockStorageAdapter) GetUserLanguagePreference(ctx context.Context, username string) (string, error) {
	args := m.Called(ctx, username)
	return args.String(0), args.Error(1)
}

func (m *MockStorageAdapter) SetUserLanguagePreference(ctx context.Context, username string, language string) error {
	args := m.Called(ctx, username, language)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetUserPreferences(ctx context.Context, username string) (*storage.UserPreferences, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.UserPreferences), args.Error(1)
}

func (m *MockStorageAdapter) UpdateUserPreferences(ctx context.Context, username string, preferences *storage.UserPreferences) error {
	args := m.Called(ctx, username, preferences)
	return args.Error(0)
}

// Search suggestion tracking and analytics
func (m *MockStorageAdapter) TrackSearchQuery(ctx context.Context, userID, query string, resultCount int) error {
	args := m.Called(ctx, userID, query, resultCount)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetPopularSearchQueries(ctx context.Context, limit int, timeWindow time.Duration) ([]storage.SearchQueryStats, error) {
	args := m.Called(ctx, limit, timeWindow)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.SearchQueryStats), args.Error(1)
}

func (m *MockStorageAdapter) GetUserSearchHistory(ctx context.Context, userID string, limit int) ([]storage.SearchHistoryEntry, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.SearchHistoryEntry), args.Error(1)
}

func (m *MockStorageAdapter) GenerateSearchSuggestions(ctx context.Context, userID, partialQuery string, limit int) ([]string, error) {
	args := m.Called(ctx, userID, partialQuery, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// Trending operations
func (m *MockStorageAdapter) RecordHashtagUsage(ctx context.Context, hashtag string, statusID string, authorID string) error {
	args := m.Called(ctx, hashtag, statusID, authorID)
	return args.Error(0)
}

func (m *MockStorageAdapter) RecordStatusEngagement(ctx context.Context, statusID string, engagementType string, userID string) error {
	args := m.Called(ctx, statusID, engagementType, userID)
	return args.Error(0)
}

func (m *MockStorageAdapter) RecordLinkShare(ctx context.Context, url string, statusID string, authorID string) error {
	args := m.Called(ctx, url, statusID, authorID)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetTrendingHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingHashtag), args.Error(1)
}

func (m *MockStorageAdapter) GetTrendingStatuses(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingStatus), args.Error(1)
}

func (m *MockStorageAdapter) GetTrendingLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingLink), args.Error(1)
}

// Engagement metrics operations
func (m *MockStorageAdapter) StoreEngagementMetrics(ctx context.Context, metrics *storage.EngagementMetrics) error {
	args := m.Called(ctx, metrics)
	return args.Error(0)
}

func (m *MockStorageAdapter) IndexByEngagement(ctx context.Context, statusID string, bucket string) error {
	args := m.Called(ctx, statusID, bucket)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetEngagementMetrics(ctx context.Context, statusID string) (*storage.EngagementMetrics, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.EngagementMetrics), args.Error(1)
}

// Announcement operations
func (m *MockStorageAdapter) CreateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	args := m.Called(ctx, announcement)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetAnnouncement(ctx context.Context, id string) (*storage.Announcement, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Announcement), args.Error(1)
}

func (m *MockStorageAdapter) GetAnnouncements(ctx context.Context, active bool) ([]*storage.Announcement, error) {
	args := m.Called(ctx, active)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Announcement), args.Error(1)
}

func (m *MockStorageAdapter) UpdateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	args := m.Called(ctx, announcement)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteAnnouncement(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorageAdapter) DismissAnnouncement(ctx context.Context, username, announcementID string) error {
	args := m.Called(ctx, username, announcementID)
	return args.Error(0)
}

func (m *MockStorageAdapter) IsDismissed(ctx context.Context, username, announcementID string) (bool, error) {
	args := m.Called(ctx, username, announcementID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageAdapter) GetDismissedAnnouncements(ctx context.Context, username string) ([]string, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockStorageAdapter) AddAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error {
	args := m.Called(ctx, username, announcementID, emojiName)
	return args.Error(0)
}

func (m *MockStorageAdapter) RemoveAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error {
	args := m.Called(ctx, username, announcementID, emojiName)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetAnnouncementReactions(ctx context.Context, announcementID string) (map[string][]string, error) {
	args := m.Called(ctx, announcementID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string][]string), args.Error(1)
}

// Custom emoji operations
func (m *MockStorageAdapter) CreateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error {
	args := m.Called(ctx, emoji)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetCustomEmoji(ctx context.Context, shortcode string) (*storage.CustomEmoji, error) {
	args := m.Called(ctx, shortcode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.CustomEmoji), args.Error(1)
}

func (m *MockStorageAdapter) GetCustomEmojis(ctx context.Context) ([]*storage.CustomEmoji, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CustomEmoji), args.Error(1)
}

func (m *MockStorageAdapter) UpdateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error {
	args := m.Called(ctx, emoji)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteCustomEmoji(ctx context.Context, shortcode string) error {
	args := m.Called(ctx, shortcode)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetCustomEmojisByCategory(ctx context.Context, category string) ([]*storage.CustomEmoji, error) {
	args := m.Called(ctx, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CustomEmoji), args.Error(1)
}

// Report operations
func (m *MockStorageAdapter) CreateReport(ctx context.Context, report *storage.Report) error {
	args := m.Called(ctx, report)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetReport(ctx context.Context, id string) (*storage.Report, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Report), args.Error(1)
}

func (m *MockStorageAdapter) GetUserReports(ctx context.Context, username string, limit int, cursor string) ([]*storage.Report, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Report), args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetReportsByTarget(ctx context.Context, targetAccountID string, limit int, cursor string) ([]*storage.Report, string, error) {
	args := m.Called(ctx, targetAccountID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Report), args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetReportsByStatus(ctx context.Context, status storage.ReportStatus, limit int, cursor string) ([]*storage.Report, string, error) {
	args := m.Called(ctx, status, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Report), args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) UpdateReportStatus(ctx context.Context, id string, status storage.ReportStatus, actionTaken string, moderatorID string) error {
	args := m.Called(ctx, id, status, actionTaken, moderatorID)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetReportStats(ctx context.Context, username string) (*storage.ReportStats, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ReportStats), args.Error(1)
}

func (m *MockStorageAdapter) IncrementFalseReports(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorageAdapter) AssignReport(ctx context.Context, reportID string, assignedTo string) error {
	args := m.Called(ctx, reportID, assignedTo)
	return args.Error(0)
}

func (m *MockStorageAdapter) UnassignReport(ctx context.Context, reportID string) error {
	args := m.Called(ctx, reportID)
	return args.Error(0)
}

// Reputation-related operations
func (m *MockStorageAdapter) GetStatusCount(ctx context.Context, actorID string) (int, error) {
	args := m.Called(ctx, actorID)
	return args.Int(0), args.Error(1)
}

func (m *MockStorageAdapter) GetFollowersCount(ctx context.Context, actorID string) (int, error) {
	args := m.Called(ctx, actorID)
	return args.Int(0), args.Error(1)
}

func (m *MockStorageAdapter) GetFollowingCount(ctx context.Context, actorID string) (int, error) {
	args := m.Called(ctx, actorID)
	return args.Int(0), args.Error(1)
}

func (m *MockStorageAdapter) GetLatestStatus(ctx context.Context, actorID string) (*storage.StatusSearchResult, error) {
	args := m.Called(ctx, actorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorageAdapter) GetCommunityNotesByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*storage.CommunityNote, string, error) {
	args := m.Called(ctx, authorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.CommunityNote), args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetCommunityNoteVotes(ctx context.Context, noteID string) ([]*storage.CommunityNoteVote, error) {
	args := m.Called(ctx, noteID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CommunityNoteVote), args.Error(1)
}

// Community note operations
func (m *MockStorageAdapter) CreateCommunityNote(ctx context.Context, note *storage.CommunityNote) error {
	args := m.Called(ctx, note)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetCommunityNote(ctx context.Context, noteID string) (*storage.CommunityNote, error) {
	args := m.Called(ctx, noteID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.CommunityNote), args.Error(1)
}

func (m *MockStorageAdapter) GetVisibleCommunityNotes(ctx context.Context, objectID string) ([]*storage.CommunityNote, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CommunityNote), args.Error(1)
}

func (m *MockStorageAdapter) UpdateCommunityNoteScore(ctx context.Context, noteID string, score float64, status string) error {
	args := m.Called(ctx, noteID, score, status)
	return args.Error(0)
}

func (m *MockStorageAdapter) CreateCommunityNoteVote(ctx context.Context, vote *storage.CommunityNoteVote) error {
	args := m.Called(ctx, vote)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetUserCommunityNoteVotes(ctx context.Context, userID string, noteIDs []string) (map[string]*storage.CommunityNoteVote, error) {
	args := m.Called(ctx, userID, noteIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]*storage.CommunityNoteVote), args.Error(1)
}

func (m *MockStorageAdapter) CheckCommunityNoteRateLimit(ctx context.Context, userID string, limit int) (bool, int, error) {
	args := m.Called(ctx, userID, limit)
	return args.Bool(0), args.Int(1), args.Error(2)
}

// Domain block operations (user-level)
func (m *MockStorageAdapter) AddDomainBlock(ctx context.Context, username, domain string) error {
	args := m.Called(ctx, username, domain)
	return args.Error(0)
}

func (m *MockStorageAdapter) RemoveDomainBlock(ctx context.Context, username, domain string) error {
	args := m.Called(ctx, username, domain)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetUserDomainBlocks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) IsBlockedDomain(ctx context.Context, username, domain string) (bool, error) {
	args := m.Called(ctx, username, domain)
	return args.Bool(0), args.Error(1)
}

// Instance domain block operations (admin-level)
func (m *MockStorageAdapter) CreateInstanceDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetInstanceDomainBlock(ctx context.Context, domain string) (*storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceDomainBlock), args.Error(1)
}

func (m *MockStorageAdapter) GetInstanceDomainBlockByID(ctx context.Context, id string) (*storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceDomainBlock), args.Error(1)
}

func (m *MockStorageAdapter) ListInstanceDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.InstanceDomainBlock), args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) UpdateInstanceDomainBlock(ctx context.Context, domain string, updates map[string]any) error {
	args := m.Called(ctx, domain, updates)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteInstanceDomainBlock(ctx context.Context, domain string) error {
	args := m.Called(ctx, domain)
	return args.Error(0)
}

func (m *MockStorageAdapter) IsInstanceDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, domain)
	if args.Get(1) == nil {
		return args.Bool(0), nil, args.Error(2)
	}
	return args.Bool(0), args.Get(1).(*storage.InstanceDomainBlock), args.Error(2)
}

// Federation domain management operations (admin-level)
func (m *MockStorageAdapter) GetDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.InstanceDomainBlock), args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetDomainBlock(ctx context.Context, id string) (*storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceDomainBlock), args.Error(1)
}

func (m *MockStorageAdapter) CreateDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

func (m *MockStorageAdapter) UpdateDomainBlock(ctx context.Context, id string, updates map[string]any) error {
	args := m.Called(ctx, id, updates)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteDomainBlock(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorageAdapter) IsDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, domain)
	if args.Get(1) == nil {
		return args.Bool(0), nil, args.Error(2)
	}
	return args.Bool(0), args.Get(1).(*storage.InstanceDomainBlock), args.Error(2)
}

// Domain allow operations (for allowlist mode)
func (m *MockStorageAdapter) GetDomainAllows(ctx context.Context, limit int, cursor string) ([]*storage.DomainAllow, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.DomainAllow), args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) CreateDomainAllow(ctx context.Context, allow *storage.DomainAllow) error {
	args := m.Called(ctx, allow)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteDomainAllow(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// Federation instance tracking
func (m *MockStorageAdapter) GetInstanceInfo(ctx context.Context, domain string) (*storage.InstanceInfo, error) {
	args := m.Called(ctx, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceInfo), args.Error(1)
}

func (m *MockStorageAdapter) UpsertInstanceInfo(ctx context.Context, info *storage.InstanceInfo) error {
	args := m.Called(ctx, info)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetKnownInstances(ctx context.Context, limit int, cursor string) ([]*storage.InstanceInfo, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.InstanceInfo), args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetFederationStatistics(ctx context.Context, startTime, endTime time.Time) (*storage.FederationStats, error) {
	args := m.Called(ctx, startTime, endTime)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.FederationStats), args.Error(1)
}

// Federation cost tracking
func (m *MockStorageAdapter) RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetFederationCosts(ctx context.Context, startTime, endTime time.Time, limit int, cursor string) ([]*storage.FederationCost, string, error) {
	args := m.Called(ctx, startTime, endTime, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.FederationCost), args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) GetInstanceHealthReport(ctx context.Context, domain string, period time.Duration) (*storage.InstanceHealthReport, error) {
	args := m.Called(ctx, domain, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceHealthReport), args.Error(1)
}

func (m *MockStorageAdapter) GetCostProjections(ctx context.Context, period string) (*storage.CostProjection, error) {
	args := m.Called(ctx, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.CostProjection), args.Error(1)
}

// Federation graph methods
func (m *MockStorageAdapter) GetFederationNodes(ctx context.Context, depth int) ([]*storage.FederationNode, error) {
	args := m.Called(ctx, depth)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FederationNode), args.Error(1)
}

func (m *MockStorageAdapter) GetFederationEdges(ctx context.Context, domains []string) ([]*storage.FederationEdge, error) {
	args := m.Called(ctx, domains)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FederationEdge), args.Error(1)
}

func (m *MockStorageAdapter) GetInstanceMetadata(ctx context.Context, domain string) (*storage.InstanceMetadata, error) {
	args := m.Called(ctx, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceMetadata), args.Error(1)
}

func (m *MockStorageAdapter) CalculateFederationClusters(ctx context.Context) ([]*storage.InstanceCluster, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.InstanceCluster), args.Error(1)
}

func (m *MockStorageAdapter) GetInstanceConnections(ctx context.Context, domain string, connectionType string) ([]*storage.InstanceConnection, error) {
	args := m.Called(ctx, domain, connectionType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.InstanceConnection), args.Error(1)
}

func (m *MockStorageAdapter) GetRecentInstanceConnections(ctx context.Context, domain string, since time.Duration) ([]*storage.InstanceConnection, error) {
	args := m.Called(ctx, domain, since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.InstanceConnection), args.Error(1)
}

func (m *MockStorageAdapter) UpdateFederationNode(ctx context.Context, node *storage.FederationNode) error {
	args := m.Called(ctx, node)
	return args.Error(0)
}

func (m *MockStorageAdapter) UpdateFederationEdge(ctx context.Context, edge *storage.FederationEdge) error {
	args := m.Called(ctx, edge)
	return args.Error(0)
}

func (m *MockStorageAdapter) UpdateInstanceMetadata(ctx context.Context, metadata *storage.InstanceMetadata) error {
	args := m.Called(ctx, metadata)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetStrongestConnectionsByType(ctx context.Context, connectionType string, limit int) ([]*storage.FederationEdge, error) {
	args := m.Called(ctx, connectionType, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FederationEdge), args.Error(1)
}

func (m *MockStorageAdapter) StoreFederationTimeSeries(ctx context.Context, data *storage.FederationTimeSeries) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

func (m *MockStorageAdapter) StoreInstanceCluster(ctx context.Context, cluster *storage.InstanceCluster) error {
	args := m.Called(ctx, cluster)
	return args.Error(0)
}

// Moderation pattern management
func (m *MockStorageAdapter) CreateModerationPattern(ctx context.Context, pattern *storage.ModerationPattern) error {
	args := m.Called(ctx, pattern)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetModerationPattern(ctx context.Context, patternID string) (*storage.ModerationPattern, error) {
	args := m.Called(ctx, patternID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ModerationPattern), args.Error(1)
}

func (m *MockStorageAdapter) GetModerationPatterns(ctx context.Context, active bool, severity string, limit int) ([]*storage.ModerationPattern, error) {
	args := m.Called(ctx, active, severity, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.ModerationPattern), args.Error(1)
}

func (m *MockStorageAdapter) UpdateModerationPattern(ctx context.Context, pattern *storage.ModerationPattern) error {
	args := m.Called(ctx, pattern)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteModerationPattern(ctx context.Context, patternID string) error {
	args := m.Called(ctx, patternID)
	return args.Error(0)
}

func (m *MockStorageAdapter) RecordPatternMatch(ctx context.Context, patternID string, matched bool, timestamp time.Time) error {
	args := m.Called(ctx, patternID, matched, timestamp)
	return args.Error(0)
}

func (m *MockStorageAdapter) StoreModerationDecision(ctx context.Context, decision *storage.ModerationDecision) error {
	args := m.Called(ctx, decision)
	return args.Error(0)
}

func (m *MockStorageAdapter) UpdateModerationDecision(ctx context.Context, contentID string, review *storage.ModerationReview) error {
	args := m.Called(ctx, contentID, review)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetModerationQueue(ctx context.Context, filter *storage.ModerationFilter) ([]*storage.ModerationQueueItem, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.ModerationQueueItem), args.Error(1)
}

// Email domain blocks
func (m *MockStorageAdapter) CreateEmailDomainBlock(ctx context.Context, block *storage.EmailDomainBlock) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetEmailDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.EmailDomainBlock, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.EmailDomainBlock), args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) DeleteEmailDomainBlock(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// Marker operations
func (m *MockStorageAdapter) SaveMarker(ctx context.Context, username, timeline string, lastReadID string, version int) error {
	args := m.Called(ctx, username, timeline, lastReadID, version)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetMarkers(ctx context.Context, username string, timelines []string) (map[string]*storage.Marker, error) {
	args := m.Called(ctx, username, timelines)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]*storage.Marker), args.Error(1)
}

// Extended preferences operations
func (m *MockStorageAdapter) SetPreference(ctx context.Context, username string, key string, value any) error {
	args := m.Called(ctx, username, key, value)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetPreference(ctx context.Context, username string, key string) (any, error) {
	args := m.Called(ctx, username, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0), args.Error(1)
}

func (m *MockStorageAdapter) GetAllPreferences(ctx context.Context, username string) (map[string]any, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]any), args.Error(1)
}

func (m *MockStorageAdapter) UpdatePreferences(ctx context.Context, username string, prefs map[string]any) error {
	args := m.Called(ctx, username, prefs)
	return args.Error(0)
}

// Streaming preferences operations
func (m *MockStorageAdapter) GetStreamingPreferences(ctx context.Context, username string) (*storage.StreamingPreferences, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StreamingPreferences), args.Error(1)
}

func (m *MockStorageAdapter) UpdateStreamingPreferences(ctx context.Context, prefs *storage.StreamingPreferences) error {
	args := m.Called(ctx, prefs)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetStreamingPreferencesByDevice(ctx context.Context, username, deviceID string) (*storage.StreamingPreferences, error) {
	args := m.Called(ctx, username, deviceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StreamingPreferences), args.Error(1)
}

func (m *MockStorageAdapter) UpdateDeviceStreamingPreferences(ctx context.Context, prefs *storage.StreamingPreferences, deviceID string) error {
	args := m.Called(ctx, prefs, deviceID)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetStreamingPreferenceHistory(ctx context.Context, username string, limit int) ([]*storage.StreamingPreferences, error) {
	args := m.Called(ctx, username, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StreamingPreferences), args.Error(1)
}

func (m *MockStorageAdapter) SyncStreamingPreferences(ctx context.Context, username string, sourceDeviceID string) error {
	args := m.Called(ctx, username, sourceDeviceID)
	return args.Error(0)
}

func (m *MockStorageAdapter) ResolvePreferenceConflict(ctx context.Context, username string, strategy storage.ConflictResolutionStrategy) (*storage.StreamingPreferences, error) {
	args := m.Called(ctx, username, strategy)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StreamingPreferences), args.Error(1)
}

// Session management operations
func (m *MockStorageAdapter) CreateSession(ctx context.Context, session *storage.Session) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetSession(ctx context.Context, sessionID string) (*storage.Session, error) {
	args := m.Called(ctx, sessionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Session), args.Error(1)
}

func (m *MockStorageAdapter) GetSessionByRefreshToken(ctx context.Context, refreshToken string) (*storage.Session, error) {
	args := m.Called(ctx, refreshToken)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Session), args.Error(1)
}

func (m *MockStorageAdapter) UpdateSession(ctx context.Context, session *storage.Session) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteSession(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetUserSessions(ctx context.Context, username string) ([]*storage.Session, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Session), args.Error(1)
}

// Device management operations
func (m *MockStorageAdapter) CreateDevice(ctx context.Context, device *storage.Device) error {
	args := m.Called(ctx, device)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetDevice(ctx context.Context, deviceID string) (*storage.Device, error) {
	args := m.Called(ctx, deviceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Device), args.Error(1)
}

func (m *MockStorageAdapter) UpdateDevice(ctx context.Context, device *storage.Device) error {
	args := m.Called(ctx, device)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetUserDevices(ctx context.Context, username string) ([]*storage.Device, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Device), args.Error(1)
}

// Rate limiting operations
func (m *MockStorageAdapter) RecordLoginAttempt(ctx context.Context, identifier string, success bool) error {
	args := m.Called(ctx, identifier, success)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetLoginAttemptCount(ctx context.Context, identifier string, since time.Time) (int, error) {
	args := m.Called(ctx, identifier, since)
	return args.Int(0), args.Error(1)
}

func (m *MockStorageAdapter) IsRateLimited(ctx context.Context, identifier string) (bool, time.Time, error) {
	args := m.Called(ctx, identifier)
	return args.Bool(0), args.Get(1).(time.Time), args.Error(2)
}

func (m *MockStorageAdapter) ClearLoginAttempts(ctx context.Context, identifier string) error {
	args := m.Called(ctx, identifier)
	return args.Error(0)
}

func (m *MockStorageAdapter) StoreWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error {
	args := m.Called(ctx, credential)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetWebAuthnCredential(ctx context.Context, credentialID string) (*storage.WebAuthnCredential, error) {
	args := m.Called(ctx, credentialID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.WebAuthnCredential), args.Error(1)
}

func (m *MockStorageAdapter) GetUserWebAuthnCredentials(ctx context.Context, username string) ([]*storage.WebAuthnCredential, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.WebAuthnCredential), args.Error(1)
}

func (m *MockStorageAdapter) UpdateWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error {
	args := m.Called(ctx, credential)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteWebAuthnCredential(ctx context.Context, credentialID string) error {
	args := m.Called(ctx, credentialID)
	return args.Error(0)
}

func (m *MockStorageAdapter) StoreWebAuthnChallenge(ctx context.Context, challenge *storage.WebAuthnChallenge) error {
	args := m.Called(ctx, challenge)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetWebAuthnChallenge(ctx context.Context, challengeID string) (*storage.WebAuthnChallenge, error) {
	args := m.Called(ctx, challengeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.WebAuthnChallenge), args.Error(1)
}

func (m *MockStorageAdapter) DeleteWebAuthnChallenge(ctx context.Context, challengeID string) error {
	args := m.Called(ctx, challengeID)
	return args.Error(0)
}

func (m *MockStorageAdapter) StoreWalletChallenge(ctx context.Context, challenge *storage.WalletChallenge) error {
	args := m.Called(ctx, challenge)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetWalletChallenge(ctx context.Context, challengeID string) (*storage.WalletChallenge, error) {
	args := m.Called(ctx, challengeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.WalletChallenge), args.Error(1)
}

func (m *MockStorageAdapter) DeleteWalletChallenge(ctx context.Context, challengeID string) error {
	args := m.Called(ctx, challengeID)
	return args.Error(0)
}

func (m *MockStorageAdapter) StoreWalletCredential(ctx context.Context, credential *storage.WalletCredential) error {
	args := m.Called(ctx, credential)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetWalletCredential(ctx context.Context, walletType, address string) (*storage.WalletCredential, error) {
	args := m.Called(ctx, walletType, address)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.WalletCredential), args.Error(1)
}

func (m *MockStorageAdapter) GetUserWalletCredentials(ctx context.Context, username string) ([]*storage.WalletCredential, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.WalletCredential), args.Error(1)
}

func (m *MockStorageAdapter) DeleteWalletCredential(ctx context.Context, username, address string) error {
	args := m.Called(ctx, username, address)
	return args.Error(0)
}

func (m *MockStorageAdapter) UpdateWalletLastUsed(ctx context.Context, username, address string) error {
	args := m.Called(ctx, username, address)
	return args.Error(0)
}

func (m *MockStorageAdapter) StoreTrustee(ctx context.Context, username string, trustee *storage.TrusteeConfig) error {
	args := m.Called(ctx, username, trustee)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetTrustees(ctx context.Context, username string) ([]*storage.TrusteeConfig, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrusteeConfig), args.Error(1)
}

func (m *MockStorageAdapter) DeleteTrustee(ctx context.Context, username, trusteeActorID string) error {
	args := m.Called(ctx, username, trusteeActorID)
	return args.Error(0)
}

func (m *MockStorageAdapter) UpdateTrusteeConfirmed(ctx context.Context, username, trusteeActorID string, confirmed bool) error {
	args := m.Called(ctx, username, trusteeActorID, confirmed)
	return args.Error(0)
}

func (m *MockStorageAdapter) StoreRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error {
	args := m.Called(ctx, request)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetRecoveryRequest(ctx context.Context, requestID string) (*storage.SocialRecoveryRequest, error) {
	args := m.Called(ctx, requestID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SocialRecoveryRequest), args.Error(1)
}

func (m *MockStorageAdapter) UpdateRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error {
	args := m.Called(ctx, request)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteRecoveryRequest(ctx context.Context, requestID string) error {
	args := m.Called(ctx, requestID)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetActiveRecoveryRequests(ctx context.Context, username string) ([]*storage.SocialRecoveryRequest, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.SocialRecoveryRequest), args.Error(1)
}

func (m *MockStorageAdapter) StoreRecoveryCode(ctx context.Context, username string, code *storage.RecoveryCodeItem) error {
	args := m.Called(ctx, username, code)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetRecoveryCodes(ctx context.Context, username string) ([]*storage.RecoveryCodeItem, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.RecoveryCodeItem), args.Error(1)
}

func (m *MockStorageAdapter) MarkRecoveryCodeUsed(ctx context.Context, username, codeHash string) error {
	args := m.Called(ctx, username, codeHash)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteAllRecoveryCodes(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorageAdapter) CountUnusedRecoveryCodes(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

// StoreReputation stores reputation information for an actor
func (m *MockStorageAdapter) StoreReputation(ctx context.Context, actorID string, reputation *storage.Reputation) error {
	args := m.Called(ctx, actorID, reputation)
	return args.Error(0)
}

// GetReputation retrieves the current reputation for an actor
func (m *MockStorageAdapter) GetReputation(ctx context.Context, actorID string) (*storage.Reputation, error) {
	args := m.Called(ctx, actorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Reputation), args.Error(1)
}

// GetReputationHistory retrieves reputation history for an actor
func (m *MockStorageAdapter) GetReputationHistory(ctx context.Context, actorID string, limit int) ([]*storage.Reputation, error) {
	args := m.Called(ctx, actorID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Reputation), args.Error(1)
}

// CreateVouch creates a new vouch
func (m *MockStorageAdapter) CreateVouch(ctx context.Context, vouch *storage.Vouch) error {
	args := m.Called(ctx, vouch)
	return args.Error(0)
}

// GetVouch retrieves a vouch by ID
func (m *MockStorageAdapter) GetVouch(ctx context.Context, vouchID string) (*storage.Vouch, error) {
	args := m.Called(ctx, vouchID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Vouch), args.Error(1)
}

// GetVouchesByActor retrieves vouches made by an actor
func (m *MockStorageAdapter) GetVouchesByActor(ctx context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	args := m.Called(ctx, actorID, activeOnly)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Vouch), args.Error(1)
}

// GetVouchesForActor retrieves vouches received by an actor
func (m *MockStorageAdapter) GetVouchesForActor(ctx context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	args := m.Called(ctx, actorID, activeOnly)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Vouch), args.Error(1)
}

// UpdateVouchStatus updates the status of a vouch
func (m *MockStorageAdapter) UpdateVouchStatus(ctx context.Context, vouchID string, active bool, revokedAt *time.Time) error {
	args := m.Called(ctx, vouchID, active, revokedAt)
	return args.Error(0)
}

// GetMonthlyVouchCount retrieves the number of vouches made by an actor in a given month
func (m *MockStorageAdapter) GetMonthlyVouchCount(ctx context.Context, actorID string, year int, month time.Month) (int, error) {
	args := m.Called(ctx, actorID, year, month)
	return args.Int(0), args.Error(1)
}

// GetDNSCache retrieves a cached DNS entry
func (m *MockStorageAdapter) GetDNSCache(ctx context.Context, hostname string) (*storage.DNSCacheEntry, error) {
	args := m.Called(ctx, hostname)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.DNSCacheEntry), args.Error(1)
}

// SetDNSCache stores a DNS cache entry
func (m *MockStorageAdapter) SetDNSCache(ctx context.Context, entry *storage.DNSCacheEntry) error {
	args := m.Called(ctx, entry)
	return args.Error(0)
}

// GetReplies retrieves replies to an object
func (m *MockStorageAdapter) GetReplies(ctx context.Context, objectID string, limit int, cursor string) ([]any, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]any), args.String(1), args.Error(2)
}

// CountReplies counts the number of replies to an object
func (m *MockStorageAdapter) CountReplies(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// IncrementReplyCount increments the reply count for an object
func (m *MockStorageAdapter) IncrementReplyCount(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// SyncThreadFromRemote synchronizes a thread from a remote instance
func (m *MockStorageAdapter) SyncThreadFromRemote(ctx context.Context, statusID string) (*storage.StatusSearchResult, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StatusSearchResult), args.Error(1)
}

// SyncMissingRepliesFromRemote synchronizes missing replies from a remote instance
func (m *MockStorageAdapter) SyncMissingRepliesFromRemote(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// GetThreadContext retrieves the thread context for a status
func (m *MockStorageAdapter) GetThreadContext(ctx context.Context, statusID string) (*storage.ThreadContext, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ThreadContext), args.Error(1)
}

// MarkThreadAsSynced marks a thread as synchronized
func (m *MockStorageAdapter) MarkThreadAsSynced(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

// GetMissingReplies retrieves missing replies for a status
func (m *MockStorageAdapter) GetMissingReplies(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// IncrementReblogCount increments the reblog count for an object
func (m *MockStorageAdapter) IncrementReblogCount(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// Final batch of quote-related methods
func (m *MockStorageAdapter) CreateQuoteRelationship(ctx context.Context, quote *storage.QuoteRelationship) error {
	args := m.Called(ctx, quote)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetQuotesForNote(ctx context.Context, noteID string, limit int, cursor string) ([]*storage.QuoteRelationship, string, error) {
	args := m.Called(ctx, noteID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.QuoteRelationship), args.String(1), args.Error(2)
}

func (m *MockStorageAdapter) IsQuoted(ctx context.Context, actorID, noteID string) (bool, error) {
	args := m.Called(ctx, actorID, noteID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageAdapter) WithdrawQuote(ctx context.Context, quoteNoteID string) error {
	args := m.Called(ctx, quoteNoteID)
	return args.Error(0)
}

func (m *MockStorageAdapter) CountQuotes(ctx context.Context, noteID string) (int, error) {
	args := m.Called(ctx, noteID)
	return args.Int(0), args.Error(1)
}

func (m *MockStorageAdapter) WithdrawStatusFromQuotes(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

func (m *MockStorageAdapter) UpdateQuotePermissions(ctx context.Context, statusID string, permissions *storage.QuotePermissions) error {
	args := m.Called(ctx, statusID, permissions)
	return args.Error(0)
}

func (m *MockStorageAdapter) IsQuoteAllowed(ctx context.Context, statusID, quoterID string) (bool, error) {
	args := m.Called(ctx, statusID, quoterID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageAdapter) GetQuoteType(ctx context.Context, statusID string) (string, error) {
	args := m.Called(ctx, statusID)
	return args.String(0), args.Error(1)
}

func (m *MockStorageAdapter) IsWithdrawnFromQuotes(ctx context.Context, statusID string) (bool, error) {
	args := m.Called(ctx, statusID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageAdapter) GetQuotesOfStatus(ctx context.Context, statusID string, limit int) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, statusID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorageAdapter) IsNotificationEnabled(ctx context.Context, userID, targetID string) (bool, error) {
	args := m.Called(ctx, userID, targetID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageAdapter) IsNotificationMuted(ctx context.Context, userID, targetID string) (bool, error) {
	args := m.Called(ctx, userID, targetID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageAdapter) GetOpenReportsCount(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

func (m *MockStorageAdapter) ListUsersByRole(ctx context.Context, role string) ([]*storage.User, error) {
	args := m.Called(ctx, role)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.User), args.Error(1)
}

func (m *MockStorageAdapter) GetAccountSuggestions(ctx context.Context, userID string, limit int) ([]*activitypub.Actor, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*activitypub.Actor), args.Error(1)
}

func (m *MockStorageAdapter) RemoveAccountSuggestion(ctx context.Context, userID, targetID string) error {
	args := m.Called(ctx, userID, targetID)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetFollowRequest(ctx context.Context, followerID, targetID string) (*storage.RelationshipRecord, error) {
	args := m.Called(ctx, followerID, targetID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.RelationshipRecord), args.Error(1)
}

func (m *MockStorageAdapter) AcceptFollowRequest(ctx context.Context, followerID, targetID string) error {
	args := m.Called(ctx, followerID, targetID)
	return args.Error(0)
}

func (m *MockStorageAdapter) RejectFollowRequest(ctx context.Context, followerID, targetID string) error {
	args := m.Called(ctx, followerID, targetID)
	return args.Error(0)
}

func (m *MockStorageAdapter) HasFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error) {
	args := m.Called(ctx, requesterID, targetID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageAdapter) IsEndorsed(ctx context.Context, userID, targetID string) (bool, error) {
	args := m.Called(ctx, userID, targetID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorageAdapter) GetRelationshipNote(ctx context.Context, userID, targetID string) (*storage.AccountNote, error) {
	args := m.Called(ctx, userID, targetID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.AccountNote), args.Error(1)
}

func (m *MockStorageAdapter) GetStatusReplyCount(ctx context.Context, statusID string) (int, error) {
	args := m.Called(ctx, statusID)
	return args.Int(0), args.Error(1)
}

func (m *MockStorageAdapter) GetHashtagStats(ctx context.Context, hashtag string) (any, error) {
	args := m.Called(ctx, hashtag)
	return args.Get(0), args.Error(1)
}

func (m *MockStorageAdapter) GetUserStatusCount(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

func (m *MockStorageAdapter) GetStorageUsage(ctx context.Context) (any, error) {
	args := m.Called(ctx)
	return args.Get(0), args.Error(1)
}

func (m *MockStorageAdapter) GetStorageHistory(ctx context.Context, days int) ([]any, error) {
	args := m.Called(ctx, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

func (m *MockStorageAdapter) GetUserGrowthHistory(ctx context.Context, days int) ([]any, error) {
	args := m.Called(ctx, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

func (m *MockStorageAdapter) GetDailyActiveUserCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockStorageAdapter) GetStatus(ctx context.Context, statusID string) (any, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0), args.Error(1)
}

func (m *MockStorageAdapter) GetReportedStatuses(ctx context.Context, reportID string) ([]any, error) {
	args := m.Called(ctx, reportID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

func (m *MockStorageAdapter) GetRulesByCategory(ctx context.Context, category string) ([]storage.InstanceRule, error) {
	args := m.Called(ctx, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.InstanceRule), args.Error(1)
}

func (m *MockStorageAdapter) GetScheduledStatusMedia(ctx context.Context, statusID string) ([]any, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

func (m *MockStorageAdapter) GetRecentHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingHashtag), args.Error(1)
}

func (m *MockStorageAdapter) StoreHashtagTrend(ctx context.Context, trend any) error {
	args := m.Called(ctx, trend)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetRecentStatusesWithEngagement(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingStatus), args.Error(1)
}

func (m *MockStorageAdapter) GetUserTrustScore(ctx context.Context, userID string) (float64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(float64), args.Error(1)
}

func (m *MockStorageAdapter) StoreStatusTrend(ctx context.Context, trend any) error {
	args := m.Called(ctx, trend)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetRecentLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingLink), args.Error(1)
}

func (m *MockStorageAdapter) StoreLinkTrend(ctx context.Context, trend any) error {
	args := m.Called(ctx, trend)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteOldHashtagTrends(ctx context.Context, before time.Time) error {
	args := m.Called(ctx, before)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteOldStatusTrends(ctx context.Context, before time.Time) error {
	args := m.Called(ctx, before)
	return args.Error(0)
}

func (m *MockStorageAdapter) DeleteOldLinkTrends(ctx context.Context, before time.Time) error {
	args := m.Called(ctx, before)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetStatusesByLink(ctx context.Context, linkURL string, limit int) ([]any, error) {
	args := m.Called(ctx, linkURL, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

func (m *MockStorageAdapter) StoreRelayInfo(ctx context.Context, relay *storage.RelayInfo) error {
	args := m.Called(ctx, relay)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetRelayInfo(ctx context.Context, relayURL string) (*storage.RelayInfo, error) {
	args := m.Called(ctx, relayURL)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.RelayInfo), args.Error(1)
}

func (m *MockStorageAdapter) RemoveRelayInfo(ctx context.Context, relayURL string) error {
	args := m.Called(ctx, relayURL)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetActiveRelays(ctx context.Context) ([]*storage.RelayInfo, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.RelayInfo), args.Error(1)
}

func (m *MockStorageAdapter) GetAllRelays(ctx context.Context, limit int, cursor string) ([]*storage.RelayInfo, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.Get(1).(string), args.Error(2)
	}
	return args.Get(0).([]*storage.RelayInfo), args.Get(1).(string), args.Error(2)
}

func (m *MockStorageAdapter) UpdateRelayStatus(ctx context.Context, relayURL string, active bool) error {
	args := m.Called(ctx, relayURL, active)
	return args.Error(0)
}

func (m *MockStorageAdapter) AcknowledgeSeverance(ctx context.Context, userID, domain string) error {
	args := m.Called(ctx, userID, domain)
	return args.Error(0)
}

func (m *MockStorageAdapter) AttemptReconnection(ctx context.Context, userID, domain string) error {
	args := m.Called(ctx, userID, domain)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetUserSeveredRelationships(ctx context.Context, userID string) ([]*storage.SeveredRelationship, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.SeveredRelationship), args.Error(1)
}

func (m *MockStorageAdapter) GetAffectedRelationships(ctx context.Context, userID, domain string) ([]*storage.RelationshipRecord, error) {
	args := m.Called(ctx, userID, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.RelationshipRecord), args.Error(1)
}

func (m *MockStorageAdapter) TrackFederationIssue(ctx context.Context, domain, issueType string) error {
	args := m.Called(ctx, domain, issueType)
	return args.Error(0)
}

func (m *MockStorageAdapter) CreateSeveredRelationship(ctx context.Context, rel *storage.SeveredRelationship) error {
	args := m.Called(ctx, rel)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetSeveredRelationships(ctx context.Context, localInstance string, limit int, cursor string) ([]*storage.SeveredRelationship, string, error) {
	args := m.Called(ctx, localInstance, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.Get(1).(string), args.Error(2)
	}
	return args.Get(0).([]*storage.SeveredRelationship), args.Get(1).(string), args.Error(2)
}

func (m *MockStorageAdapter) GetSeveredRelationship(ctx context.Context, localInstance, remoteInstance string) (*storage.SeveredRelationship, error) {
	args := m.Called(ctx, localInstance, remoteInstance)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SeveredRelationship), args.Error(1)
}

func (m *MockStorageAdapter) UpdateSeveredRelationship(ctx context.Context, rel *storage.SeveredRelationship) error {
	args := m.Called(ctx, rel)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetAffectedFollows(ctx context.Context, localInstance, remoteInstance string) ([]storage.AffectedFollow, error) {
	args := m.Called(ctx, localInstance, remoteInstance)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.AffectedFollow), args.Error(1)
}

func (m *MockStorageAdapter) RecordAffectedFollow(ctx context.Context, localInstance, remoteInstance string, follow storage.AffectedFollow) error {
	args := m.Called(ctx, localInstance, remoteInstance, follow)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetUserMedia(ctx context.Context, username string) ([]any, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

func (m *MockStorageAdapter) UpdateMediaAttachment(ctx context.Context, mediaID string, updates map[string]any) error {
	args := m.Called(ctx, mediaID, updates)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetLocalPostCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockStorageAdapter) SaveOAuthState(ctx context.Context, state *storage.OAuthState) error {
	args := m.Called(ctx, state)
	return args.Error(0)
}

func (m *MockStorageAdapter) SaveUserAppConsent(ctx context.Context, consent *storage.UserAppConsent) error {
	args := m.Called(ctx, consent)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetOAuthApp(ctx context.Context, clientID string) (*storage.OAuthApp, error) {
	args := m.Called(ctx, clientID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.OAuthApp), args.Error(1)
}

func (m *MockStorageAdapter) GetUserAppConsent(ctx context.Context, userID, appID string) (*storage.UserAppConsent, error) {
	args := m.Called(ctx, userID, appID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.UserAppConsent), args.Error(1)
}

func (m *MockStorageAdapter) GetLikeCount(ctx context.Context, statusID string) (int64, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockStorageAdapter) GetBoostCount(ctx context.Context, statusID string) (int64, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockStorageAdapter) GetReplyCount(ctx context.Context, statusID string) (int64, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0).(int64), args.Error(1)
}

// Media Analytics operations
func (m *MockStorageAdapter) RecordManifestGeneration(ctx context.Context, mediaID, format string, duration float64) error {
	args := m.Called(ctx, mediaID, format, duration)
	return args.Error(0)
}

func (m *MockStorageAdapter) RecordQualityChange(ctx context.Context, mediaID, userID, oldQuality, newQuality string) error {
	args := m.Called(ctx, mediaID, userID, oldQuality, newQuality)
	return args.Error(0)
}

func (m *MockStorageAdapter) RecordMediaEvent(ctx context.Context, eventType, mediaID, userID string) error {
	args := m.Called(ctx, eventType, mediaID, userID)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetManifestGenerationStats(ctx context.Context, format, startDate, endDate string) (map[string]int64, error) {
	args := m.Called(ctx, format, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int64), args.Error(1)
}

func (m *MockStorageAdapter) GetMediaEventStats(ctx context.Context, eventType, startDate, endDate string) (map[string]int64, error) {
	args := m.Called(ctx, eventType, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int64), args.Error(1)
}

func (m *MockStorageAdapter) GetModerationQueueCount(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Get(0).(int), args.Error(1)
}

func (m *MockStorageAdapter) HasPendingFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error) {
	args := m.Called(ctx, requesterID, targetID)
	return args.Get(0).(bool), args.Error(1)
}

func (m *MockStorageAdapter) GetFieldVerification(ctx context.Context, username, fieldName string) (*storage.ActorField, error) {
	args := m.Called(ctx, username, fieldName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ActorField), args.Error(1)
}

func (m *MockStorageAdapter) GetDomainStats(ctx context.Context, domain string) (any, error) {
	args := m.Called(ctx, domain)
	return args.Get(0), args.Error(1)
}

func (m *MockStorageAdapter) UnmarkAllMediaAsSensitive(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorageAdapter) ReverseSeverance(ctx context.Context, localInstance, remoteInstance string) error {
	args := m.Called(ctx, localInstance, remoteInstance)
	return args.Error(0)
}

func (m *MockStorageAdapter) GetSeveranceHistory(ctx context.Context, localInstance, remoteInstance string, limit int) ([]*storage.SeveredRelationship, error) {
	args := m.Called(ctx, localInstance, remoteInstance, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.SeveredRelationship), args.Error(1)
}


// UpdateCommunityNoteAnalysis updates AI analysis results for a community note
func (m *MockStorageAdapter) UpdateCommunityNoteAnalysis(ctx context.Context, noteID string, sentiment, objectivity, sourceQuality float64) error {
	args := m.Called(ctx, noteID, sentiment, objectivity, sourceQuality)
	return args.Error(0)
}

// CheckAPIRateLimit checks if API rate limit is exceeded
func (m *MockStorageAdapter) CheckAPIRateLimit(ctx context.Context, userID, endpoint string, limit int, window time.Duration) error {
	args := m.Called(ctx, userID, endpoint, limit, window)
	return args.Error(0)
}

// GetAPIRateLimitInfo returns current rate limit info
func (m *MockStorageAdapter) GetAPIRateLimitInfo(ctx context.Context, userID, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error) {
	args := m.Called(ctx, userID, endpoint, limit, window)
	return args.Int(0), args.Get(1).(time.Time), args.Error(2)
}

// NewMockStorageAdapter creates a new mock storage adapter
func NewMockStorageAdapter() *MockStorageAdapter {
	return &MockStorageAdapter{}
}