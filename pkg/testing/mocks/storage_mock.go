package mocks

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockStorage is a mock implementation of the Storage interface
type MockStorage struct {
	mock.Mock
	mu sync.RWMutex

	// In-memory storage for testing
	actors     map[string]*activitypub.Actor
	activities map[string]*activitypub.Activity
	objects    map[string]any
}

// NewMockStorage creates a new mock storage instance
func NewMockStorage() *MockStorage {
	return &MockStorage{
		actors:     make(map[string]*activitypub.Actor),
		activities: make(map[string]*activitypub.Activity),
		objects:    make(map[string]any),
	}
}

// Actor operations
func (m *MockStorage) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	args := m.Called(ctx, actor, privateKey)
	return args.Error(0)
}

func (m *MockStorage) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

func (m *MockStorage) GetActorByNumericID(ctx context.Context, numericID string) (*activitypub.Actor, error) {
	args := m.Called(ctx, numericID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

func (m *MockStorage) GetActorWithMetadata(ctx context.Context, username string) (*activitypub.Actor, *storage.ActorMetadata, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).(*activitypub.Actor), args.Get(1).(*storage.ActorMetadata), args.Error(2)
}

func (m *MockStorage) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	args := m.Called(ctx, username)
	return args.String(0), args.Error(1)
}

func (m *MockStorage) UpdateActor(ctx context.Context, actor *activitypub.Actor) error {
	args := m.Called(ctx, actor)
	return args.Error(0)
}

func (m *MockStorage) UpdateActorLastStatusTime(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorage) SetActorFields(ctx context.Context, username string, fields []storage.ActorField) error {
	args := m.Called(ctx, username, fields)
	return args.Error(0)
}

func (m *MockStorage) DeleteActor(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorage) SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error) {
	args := m.Called(ctx, query, limit, followingOnly, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*activitypub.Actor), args.Error(1)
}

func (m *MockStorage) GetSearchSuggestions(ctx context.Context, prefix string) ([]storage.SearchSuggestion, error) {
	args := m.Called(ctx, prefix)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.SearchSuggestion), args.Error(1)
}

// Status search operations
func (m *MockStorage) SearchStatuses(ctx context.Context, query string, limit int) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, query, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorage) SearchStatusesWithOptions(ctx context.Context, query string, options storage.StatusSearchOptions) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, query, options)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorage) SearchStatusesByURL(ctx context.Context, url string) (*storage.StatusSearchResult, error) {
	args := m.Called(ctx, url)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StatusSearchResult), args.Error(1)
}

// Enhanced search operations
func (m *MockStorage) SearchAll(ctx context.Context, query string, limit int, accountID string) (*storage.SearchResults, error) {
	args := m.Called(ctx, query, limit, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SearchResults), args.Error(1)
}

func (m *MockStorage) SearchAccountsAdvanced(ctx context.Context, query string, resolve bool, limit int, offset int, following bool, accountID string) ([]*activitypub.Actor, error) {
	args := m.Called(ctx, query, resolve, limit, offset, following, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*activitypub.Actor), args.Error(1)
}

func (m *MockStorage) SearchStatusesAdvanced(ctx context.Context, query string, limit int, maxID, minID *string, accountID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, query, limit, maxID, minID, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorage) SearchHashtags(ctx context.Context, query string, limit int) ([]*storage.Hashtag, error) {
	args := m.Called(ctx, query, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Hashtag), args.Error(1)
}

func (m *MockStorage) SearchHashtagsAdvanced(ctx context.Context, query string, limit int, accountID string) ([]*storage.HashtagSearchResult, error) {
	args := m.Called(ctx, query, limit, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.HashtagSearchResult), args.Error(1)
}

// Activity operations
func (m *MockStorage) CreateActivity(ctx context.Context, activity *activitypub.Activity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

func (m *MockStorage) GetActivity(ctx context.Context, id string) (*activitypub.Activity, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Activity), args.Error(1)
}

func (m *MockStorage) GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*activitypub.Activity), args.String(1), args.Error(2)
}

func (m *MockStorage) GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*activitypub.Activity), args.String(1), args.Error(2)
}

// Object operations
func (m *MockStorage) CreateObject(ctx context.Context, object any) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

func (m *MockStorage) GetObject(ctx context.Context, id string) (any, error) {
	args := m.Called(ctx, id)
	return args.Get(0), args.Error(1)
}

func (m *MockStorage) UpdateObject(ctx context.Context, object any) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

func (m *MockStorage) DeleteObject(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorage) GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]any, string, error) {
	args := m.Called(ctx, actorID, cursor, limit)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]any), args.String(1), args.Error(2)
}

// MockStorageWithPresets creates a mock storage with common test presets
type MockStoragePreset string

const (
	PresetEmpty          MockStoragePreset = "empty"
	PresetWithTestUser   MockStoragePreset = "test_user"
	PresetWithMultiUsers MockStoragePreset = "multi_users"
)

// NewMockStorageWithPreset creates a mock storage with preset data
func NewMockStorageWithPreset(preset MockStoragePreset) *MockStorage {
	m := NewMockStorage()

	switch preset {
	case PresetWithTestUser:
		actor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/testuser",
				Type: "Person",
			},
			PreferredUsername: "testuser",
			Inbox:             "https://example.com/users/testuser/inbox",
			Outbox:            "https://example.com/users/testuser/outbox",
		}
		m.Mock.On("GetActor", mock.Anything, "testuser").Return(actor, nil)
		m.Mock.On("GetActorPrivateKey", mock.Anything, "testuser").Return("test-private-key", nil)

	case PresetWithMultiUsers:
		for i := 1; i <= 5; i++ {
			username := fmt.Sprintf("user%d", i)
			actor := &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID:   fmt.Sprintf("https://example.com/users/%s", username),
					Type: "Person",
				},
				PreferredUsername: username,
				Inbox:             fmt.Sprintf("https://example.com/users/%s/inbox", username),
				Outbox:            fmt.Sprintf("https://example.com/users/%s/outbox", username),
			}
			m.Mock.On("GetActor", mock.Anything, username).Return(actor, nil)
		}
	}

	return m
}

// Follow operations  
func (m *MockStorage) CreateFollow(ctx context.Context, followerUsername, followedUsername, followActivityID string) error {
	args := m.Called(ctx, followerUsername, followedUsername, followActivityID)
	return args.Error(0)
}

func (m *MockStorage) AcceptFollow(ctx context.Context, followerUsername, followedUsername string) error {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Error(0)
}

func (m *MockStorage) RejectFollow(ctx context.Context, followerUsername, followedUsername string) error {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Error(0)
}

func (m *MockStorage) RemoveFollow(ctx context.Context, followerUsername, followedUsername string) error {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Error(0)
}

func (m *MockStorage) RemoveFromFollowers(ctx context.Context, username, followerUsername string) error {
	args := m.Called(ctx, username, followerUsername)
	return args.Error(0)
}

func (m *MockStorage) GetFollowRequest(ctx context.Context, followerID, targetID string) (*storage.RelationshipRecord, error) {
	args := m.Called(ctx, followerID, targetID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.RelationshipRecord), args.Error(1)
}

func (m *MockStorage) AcceptFollowRequest(ctx context.Context, followerID, targetID string) error {
	args := m.Called(ctx, followerID, targetID)
	return args.Error(0)
}

func (m *MockStorage) RejectFollowRequest(ctx context.Context, followerID, targetID string) error {
	args := m.Called(ctx, followerID, targetID)
	return args.Error(0)
}

func (m *MockStorage) HasFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error) {
	args := m.Called(ctx, requesterID, targetID)
	return args.Bool(0), args.Error(1)
}

// OAuth operations  
func (m *MockStorage) GetOAuthClient(ctx context.Context, clientID string) (*storage.OAuthClient, error) {
	args := m.Called(ctx, clientID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.OAuthClient), args.Error(1)
}

func (m *MockStorage) CreateOAuthClient(ctx context.Context, client *storage.OAuthClient) error {
	args := m.Called(ctx, client)
	return args.Error(0)
}

func (m *MockStorage) UpdateOAuthClient(ctx context.Context, clientID string, updates map[string]any) error {
	args := m.Called(ctx, clientID, updates)
	return args.Error(0)
}

func (m *MockStorage) DeleteOAuthClient(ctx context.Context, clientID string) error {
	args := m.Called(ctx, clientID)
	return args.Error(0)
}

func (m *MockStorage) ListOAuthClients(ctx context.Context, limit int32, cursor string) ([]*storage.OAuthClient, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.OAuthClient), args.String(1), args.Error(2)
}

func (m *MockStorage) StoreOAuthState(ctx context.Context, state string, data *storage.OAuthState) error {
	args := m.Called(ctx, state, data)
	return args.Error(0)
}

func (m *MockStorage) SaveOAuthState(ctx context.Context, state *storage.OAuthState) error {
	args := m.Called(ctx, state)
	return args.Error(0)
}

func (m *MockStorage) GetOAuthApp(ctx context.Context, clientID string) (*storage.OAuthApp, error) {
	args := m.Called(ctx, clientID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.OAuthApp), args.Error(1)
}

func (m *MockStorage) SaveUserAppConsent(ctx context.Context, consent *storage.UserAppConsent) error {
	args := m.Called(ctx, consent)
	return args.Error(0)
}

func (m *MockStorage) GetOAuthState(ctx context.Context, state string) (*storage.OAuthState, error) {
	args := m.Called(ctx, state)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.OAuthState), args.Error(1)
}

func (m *MockStorage) DeleteOAuthState(ctx context.Context, state string) error {
	args := m.Called(ctx, state)
	return args.Error(0)
}

// Object operations
func (m *MockStorage) CountObjectReplies(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// Update history operations
func (m *MockStorage) CreateUpdateHistory(ctx context.Context, history *storage.UpdateHistory) error {
	args := m.Called(ctx, history)
	return args.Error(0)
}

func (m *MockStorage) GetUpdateHistory(ctx context.Context, objectID string, limit int) ([]*storage.UpdateHistory, error) {
	args := m.Called(ctx, objectID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.UpdateHistory), args.Error(1)
}

// Follow operations
func (m *MockStorage) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

func (m *MockStorage) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

func (m *MockStorage) IsFollowing(ctx context.Context, followerUsername, followedUsername string) (bool, error) {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

func (m *MockStorage) GetFollowRequestState(ctx context.Context, followerUsername, followedUsername string) (string, error) {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.String(0), args.Error(1)
}

// Collection operations
func (m *MockStorage) GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error) {
	args := m.Called(ctx, username, collectionType, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.OrderedCollectionPage), args.Error(1)
}

// OAuth operations
func (m *MockStorage) CreateAuthorizationCode(ctx context.Context, code *storage.AuthorizationCode) error {
	args := m.Called(ctx, code)
	return args.Error(0)
}

func (m *MockStorage) GetAuthorizationCode(ctx context.Context, code string) (*storage.AuthorizationCode, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.AuthorizationCode), args.Error(1)
}

func (m *MockStorage) DeleteAuthorizationCode(ctx context.Context, code string) error {
	args := m.Called(ctx, code)
	return args.Error(0)
}

func (m *MockStorage) CreateRefreshToken(ctx context.Context, token *storage.RefreshToken) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockStorage) GetRefreshToken(ctx context.Context, token string) (*storage.RefreshToken, error) {
	args := m.Called(ctx, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.RefreshToken), args.Error(1)
}

func (m *MockStorage) DeleteRefreshToken(ctx context.Context, token string) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

// User operations
func (m *MockStorage) CreateUser(ctx context.Context, user *storage.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockStorage) GetUser(ctx context.Context, username string) (*storage.User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

func (m *MockStorage) GetUserByEmail(ctx context.Context, email string) (*storage.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

func (m *MockStorage) UpdateUser(ctx context.Context, username string, updates map[string]any) error {
	args := m.Called(ctx, username, updates)
	return args.Error(0)
}

func (m *MockStorage) DeleteUser(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorage) ListUsers(ctx context.Context, limit int32, cursor string) ([]*storage.User, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.User), args.String(1), args.Error(2)
}

func (m *MockStorage) GetActiveUserCount(ctx context.Context, days int) (int64, error) {
	args := m.Called(ctx, days)
	return args.Get(0).(int64), args.Error(1)
}

// Instance metrics operations
func (m *MockStorage) GetTotalUserCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockStorage) GetTotalStatusCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockStorage) GetTotalDomainCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockStorage) GetWeeklyActivity(ctx context.Context, weekTimestamp int64) (*storage.WeeklyActivity, error) {
	args := m.Called(ctx, weekTimestamp)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.WeeklyActivity), args.Error(1)
}

func (m *MockStorage) RecordActivity(ctx context.Context, activityType string, actorID string, timestamp time.Time) error {
	args := m.Called(ctx, activityType, actorID, timestamp)
	return args.Error(0)
}

func (m *MockStorage) GetContactAccount(ctx context.Context) (*storage.ActorRecord, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ActorRecord), args.Error(1)
}

// OAuth provider operations
func (m *MockStorage) GetUserByProviderID(ctx context.Context, provider, providerID string) (*storage.User, error) {
	args := m.Called(ctx, provider, providerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

func (m *MockStorage) LinkProviderAccount(ctx context.Context, username, provider, providerID string) error {
	args := m.Called(ctx, username, provider, providerID)
	return args.Error(0)
}

func (m *MockStorage) UnlinkProviderAccount(ctx context.Context, username, provider string) error {
	args := m.Called(ctx, username, provider)
	return args.Error(0)
}

func (m *MockStorage) GetLinkedProviders(ctx context.Context, username string) ([]string, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// Recovery operations
func (m *MockStorage) StoreRecoveryToken(ctx context.Context, key string, data map[string]any) error {
	args := m.Called(ctx, key, data)
	return args.Error(0)
}

func (m *MockStorage) GetRecoveryToken(ctx context.Context, key string) (map[string]any, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]any), args.Error(1)
}

func (m *MockStorage) DeleteRecoveryToken(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}


// Like operations
func (m *MockStorage) CreateLike(ctx context.Context, like *storage.Like) error {
	args := m.Called(ctx, like)
	return args.Error(0)
}

func (m *MockStorage) GetLike(ctx context.Context, actor, object string) (*storage.Like, error) {
	args := m.Called(ctx, actor, object)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Like), args.Error(1)
}

func (m *MockStorage) DeleteLike(ctx context.Context, actor, object string) error {
	args := m.Called(ctx, actor, object)
	return args.Error(0)
}

func (m *MockStorage) GetLikeCount(ctx context.Context, statusID string) (int64, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockStorage) GetLocalPostCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockStorage) GetManifestGenerationStats(ctx context.Context, format, startDate, endDate string) (map[string]int64, error) {
	args := m.Called(ctx, format, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int64), args.Error(1)
}

func (m *MockStorage) SaveMarker(ctx context.Context, username, timeline string, lastReadID string, version int) error {
	args := m.Called(ctx, username, timeline, lastReadID, version)
	return args.Error(0)
}

func (m *MockStorage) GetMarkers(ctx context.Context, username string, timelines []string) (map[string]*storage.Marker, error) {
	args := m.Called(ctx, username, timelines)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]*storage.Marker), args.Error(1)
}

func (m *MockStorage) GetMediaEventStats(ctx context.Context, eventType, startDate, endDate string) (map[string]int64, error) {
	args := m.Called(ctx, eventType, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int64), args.Error(1)
}

func (m *MockStorage) GetMultiHashtagTimeline(ctx context.Context, hashtags []string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, hashtags, maxID, limit, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorage) TrackSearchQuery(ctx context.Context, userID, query string, resultCount int) error {
	args := m.Called(ctx, userID, query, resultCount)
	return args.Error(0)
}

func (m *MockStorage) GetPopularSearchQueries(ctx context.Context, limit int, timeWindow time.Duration) ([]storage.SearchQueryStats, error) {
	args := m.Called(ctx, limit, timeWindow)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.SearchQueryStats), args.Error(1)
}

func (m *MockStorage) GetPublicTimeline(ctx context.Context, local bool, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, local, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TimelineEntry), args.String(1), args.Error(2)
}

func (m *MockStorage) GetRecentHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingHashtag), args.Error(1)
}

func (m *MockStorage) GetRecentLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingLink), args.Error(1)
}

func (m *MockStorage) GetRelationshipNote(ctx context.Context, userID, targetID string) (*storage.AccountNote, error) {
	args := m.Called(ctx, userID, targetID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.AccountNote), args.Error(1)
}

func (m *MockStorage) StoreReputation(ctx context.Context, actorID string, reputation *storage.Reputation) error {
	args := m.Called(ctx, actorID, reputation)
	return args.Error(0)
}

func (m *MockStorage) GetReputation(ctx context.Context, actorID string) (*storage.Reputation, error) {
	args := m.Called(ctx, actorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Reputation), args.Error(1)
}

func (m *MockStorage) GetReputationHistory(ctx context.Context, actorID string, limit int) ([]*storage.Reputation, error) {
	args := m.Called(ctx, actorID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Reputation), args.Error(1)
}

func (m *MockStorage) GetRulesByCategory(ctx context.Context, category string) ([]storage.InstanceRule, error) {
	args := m.Called(ctx, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.InstanceRule), args.Error(1)
}

func (m *MockStorage) GetSeveranceHistory(ctx context.Context, localInstance, remoteInstance string, limit int) ([]*storage.SeveredRelationship, error) {
	args := m.Called(ctx, localInstance, remoteInstance, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.SeveredRelationship), args.Error(1)
}

func (m *MockStorage) GetStatus(ctx context.Context, statusID string) (any, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0), args.Error(1)
}

func (m *MockStorage) GetStatusCount(ctx context.Context, actorID string) (int, error) {
	args := m.Called(ctx, actorID)
	return args.Int(0), args.Error(1)
}

func (m *MockStorage) GetStatusesByLink(ctx context.Context, linkURL string, limit int) ([]any, error) {
	args := m.Called(ctx, linkURL, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

func (m *MockStorage) GetStorageHistory(ctx context.Context, days int) ([]any, error) {
	args := m.Called(ctx, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

func (m *MockStorage) GetStorageUsage(ctx context.Context) (any, error) {
	args := m.Called(ctx)
	return args.Get(0), args.Error(1)
}

func (m *MockStorage) GetStrongestConnectionsByType(ctx context.Context, connectionType string, limit int) ([]*storage.FederationEdge, error) {
	args := m.Called(ctx, connectionType, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FederationEdge), args.Error(1)
}

func (m *MockStorage) GetSuggestedHashtags(ctx context.Context, userID string, limit int) ([]*storage.HashtagSearchResult, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.HashtagSearchResult), args.Error(1)
}

func (m *MockStorage) GetTagSuggestions(ctx context.Context, userID string, limit int) ([]string, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockStorage) GetThreadContext(ctx context.Context, statusID string) (*storage.ThreadContext, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ThreadContext), args.Error(1)
}

func (m *MockStorage) GetTrendingHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingHashtag), args.Error(1)
}

func (m *MockStorage) GetTrendingLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingLink), args.Error(1)
}

func (m *MockStorage) GetTrendingStatuses(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingStatus), args.Error(1)
}

func (m *MockStorage) GetUserAppConsent(ctx context.Context, userID, appID string) (*storage.UserAppConsent, error) {
	args := m.Called(ctx, userID, appID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.UserAppConsent), args.Error(1)
}

func (m *MockStorage) GetUserGrowthHistory(ctx context.Context, days int) ([]any, error) {
	args := m.Called(ctx, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

func (m *MockStorage) UnmarkAllMediaAsSensitive(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorage) GetUserMedia(ctx context.Context, username string) ([]any, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

func (m *MockStorage) UpdateMediaAttachment(ctx context.Context, mediaID string, updates map[string]any) error {
	args := m.Called(ctx, mediaID, updates)
	return args.Error(0)
}

func (m *MockStorage) GetUserSearchHistory(ctx context.Context, userID string, limit int) ([]storage.SearchHistoryEntry, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.SearchHistoryEntry), args.Error(1)
}

func (m *MockStorage) GetUserStatusCount(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

func (m *MockStorage) GetVAPIDKeys(ctx context.Context) (*storage.VAPIDKeys, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.VAPIDKeys), args.Error(1)
}

func (m *MockStorage) SetVAPIDKeys(ctx context.Context, keys *storage.VAPIDKeys) error {
	args := m.Called(ctx, keys)
	return args.Error(0)
}

func (m *MockStorage) HasPendingFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error) {
	args := m.Called(ctx, requesterID, targetID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) HasUserVoted(ctx context.Context, pollID string, userID string) (bool, []int, error) {
	args := m.Called(ctx, pollID, userID)
	if args.Get(1) == nil {
		return args.Bool(0), nil, args.Error(2)
	}
	return args.Bool(0), args.Get(1).([]int), args.Error(2)
}

func (m *MockStorage) IncrementReblogCount(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

func (m *MockStorage) IndexHashtag(ctx context.Context, hashtag string, statusID string, authorID string, visibility string) error {
	args := m.Called(ctx, hashtag, statusID, authorID, visibility)
	return args.Error(0)
}

func (m *MockStorage) IsBlockedDomain(ctx context.Context, username, domain string) (bool, error) {
	args := m.Called(ctx, username, domain)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) IsDismissed(ctx context.Context, username, announcementID string) (bool, error) {
	args := m.Called(ctx, username, announcementID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) IsEndorsed(ctx context.Context, userID, targetID string) (bool, error) {
	args := m.Called(ctx, userID, targetID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) IsNotificationEnabled(ctx context.Context, userID, targetID string) (bool, error) {
	args := m.Called(ctx, userID, targetID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) MarkThreadAsSynced(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

func (m *MockStorage) RecordHashtagUsage(ctx context.Context, hashtag string, statusID string, authorID string) error {
	args := m.Called(ctx, hashtag, statusID, authorID)
	return args.Error(0)
}

func (m *MockStorage) RecordLinkShare(ctx context.Context, url string, statusID string, authorID string) error {
	args := m.Called(ctx, url, statusID, authorID)
	return args.Error(0)
}

func (m *MockStorage) RecordManifestGeneration(ctx context.Context, mediaID, format string, duration float64) error {
	args := m.Called(ctx, mediaID, format, duration)
	return args.Error(0)
}

func (m *MockStorage) RecordQualityChange(ctx context.Context, mediaID, userID, oldQuality, newQuality string) error {
	args := m.Called(ctx, mediaID, userID, oldQuality, newQuality)
	return args.Error(0)
}

func (m *MockStorage) RecordMediaEvent(ctx context.Context, eventType, mediaID, userID string) error {
	args := m.Called(ctx, eventType, mediaID, userID)
	return args.Error(0)
}

func (m *MockStorage) RecordPatternMatch(ctx context.Context, patternID string, matched bool, timestamp time.Time) error {
	args := m.Called(ctx, patternID, matched, timestamp)
	return args.Error(0)
}

func (m *MockStorage) GetObjectLikes(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Like, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Like), args.String(1), args.Error(2)
}

func (m *MockStorage) GetActorLikes(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Like, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Like), args.String(1), args.Error(2)
}

func (m *MockStorage) CountObjectLikes(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// Announce operations
func (m *MockStorage) CreateAnnounce(ctx context.Context, announce *storage.Announce) error {
	args := m.Called(ctx, announce)
	return args.Error(0)
}

func (m *MockStorage) GetAnnounce(ctx context.Context, actor, object string) (*storage.Announce, error) {
	args := m.Called(ctx, actor, object)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Announce), args.Error(1)
}

func (m *MockStorage) DeleteAnnounce(ctx context.Context, actor, object string) error {
	args := m.Called(ctx, actor, object)
	return args.Error(0)
}

func (m *MockStorage) GetObjectAnnounces(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Announce), args.String(1), args.Error(2)
}

func (m *MockStorage) GetActorAnnounces(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Announce), args.String(1), args.Error(2)
}

func (m *MockStorage) CountObjectAnnounces(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// Delete/Tombstone operations
func (m *MockStorage) TombstoneObject(ctx context.Context, objectID string, deletedBy string) error {
	args := m.Called(ctx, objectID, deletedBy)
	return args.Error(0)
}

func (m *MockStorage) GetTombstone(ctx context.Context, objectID string) (*storage.Tombstone, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Tombstone), args.Error(1)
}

func (m *MockStorage) CascadeDeleteLikes(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

func (m *MockStorage) CascadeDeleteAnnounces(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// Block operations
func (m *MockStorage) CreateBlock(ctx context.Context, block *storage.Block) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

func (m *MockStorage) GetBlock(ctx context.Context, actor, blockedActor string) (*storage.Block, error) {
	args := m.Called(ctx, actor, blockedActor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Block), args.Error(1)
}

func (m *MockStorage) DeleteBlock(ctx context.Context, actor, blockedActor string) error {
	args := m.Called(ctx, actor, blockedActor)
	return args.Error(0)
}

func (m *MockStorage) GetBlockedActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	args := m.Called(ctx, actor, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Block), args.String(1), args.Error(2)
}

func (m *MockStorage) GetBlockedByActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	args := m.Called(ctx, actor, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Block), args.String(1), args.Error(2)
}

func (m *MockStorage) IsBlocked(ctx context.Context, actor, targetActor string) (bool, error) {
	args := m.Called(ctx, actor, targetActor)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error) {
	args := m.Called(ctx, actor1, actor2)
	return args.Bool(0), args.Error(1)
}

// Flag operations (content moderation)
func (m *MockStorage) CreateFlag(ctx context.Context, flag *storage.Flag) error {
	args := m.Called(ctx, flag)
	return args.Error(0)
}

func (m *MockStorage) GetFlag(ctx context.Context, id string) (*storage.Flag, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Flag), args.Error(1)
}

func (m *MockStorage) GetFlagsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Flag), args.String(1), args.Error(2)
}

func (m *MockStorage) GetFlagsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Flag), args.String(1), args.Error(2)
}

func (m *MockStorage) GetPendingFlags(ctx context.Context, limit int, cursor string) ([]*storage.Flag, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Flag), args.String(1), args.Error(2)
}

func (m *MockStorage) UpdateFlagStatus(ctx context.Context, id string, status storage.FlagStatus, reviewedBy string, reviewNote string) error {
	args := m.Called(ctx, id, status, reviewedBy, reviewNote)
	return args.Error(0)
}

func (m *MockStorage) CountPendingFlags(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

// Move operations (account migration)
func (m *MockStorage) CreateMove(ctx context.Context, move *storage.Move) error {
	args := m.Called(ctx, move)
	return args.Error(0)
}

func (m *MockStorage) GetMove(ctx context.Context, actor string) (*storage.Move, error) {
	args := m.Called(ctx, actor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Move), args.Error(1)
}

func (m *MockStorage) GetMoveByTarget(ctx context.Context, target string) ([]*storage.Move, error) {
	args := m.Called(ctx, target)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Move), args.Error(1)
}

func (m *MockStorage) HasMovedFrom(ctx context.Context, oldActor, newActor string) (bool, error) {
	args := m.Called(ctx, oldActor, newActor)
	return args.Bool(0), args.Error(1)
}

// Collection operations (Add/Remove activities)
func (m *MockStorage) AddToCollection(ctx context.Context, collection string, item *storage.CollectionItem) error {
	args := m.Called(ctx, collection, item)
	return args.Error(0)
}

func (m *MockStorage) RemoveFromCollection(ctx context.Context, collection string, itemID string) error {
	args := m.Called(ctx, collection, itemID)
	return args.Error(0)
}

func (m *MockStorage) GetCollectionItems(ctx context.Context, collection string, limit int, cursor string) ([]*storage.CollectionItem, string, error) {
	args := m.Called(ctx, collection, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.CollectionItem), args.String(1), args.Error(2)
}

func (m *MockStorage) IsInCollection(ctx context.Context, collection string, itemID string) (bool, error) {
	args := m.Called(ctx, collection, itemID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) CountCollectionItems(ctx context.Context, collection string) (int, error) {
	args := m.Called(ctx, collection)
	return args.Int(0), args.Error(1)
}

// Federation severance operations
func (m *MockStorage) AcknowledgeSeverance(ctx context.Context, userID, domain string) error {
	args := m.Called(ctx, userID, domain)
	return args.Error(0)
}

func (m *MockStorage) AttemptReconnection(ctx context.Context, userID, domain string) error {
	args := m.Called(ctx, userID, domain)
	return args.Error(0)
}

func (m *MockStorage) GetUserSeveredRelationships(ctx context.Context, userID string) ([]*storage.SeveredRelationship, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.SeveredRelationship), args.Error(1)
}

// List management operations
func (m *MockStorage) CreateList(ctx context.Context, username, title, repliesPolicy string) (*storage.List, error) {
	args := m.Called(ctx, username, title, repliesPolicy)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.List), args.Error(1)
}

func (m *MockStorage) GetList(ctx context.Context, listID string) (*storage.List, error) {
	args := m.Called(ctx, listID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.List), args.Error(1)
}

func (m *MockStorage) GetListsForUser(ctx context.Context, username string) ([]*storage.List, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.List), args.Error(1)
}

func (m *MockStorage) UpdateList(ctx context.Context, listID string, updates map[string]any) error {
	args := m.Called(ctx, listID, updates)
	return args.Error(0)
}

func (m *MockStorage) DeleteList(ctx context.Context, listID string) error {
	args := m.Called(ctx, listID)
	return args.Error(0)
}

func (m *MockStorage) AddAccountsToList(ctx context.Context, listID string, accountIDs []string) error {
	args := m.Called(ctx, listID, accountIDs)
	return args.Error(0)
}

func (m *MockStorage) RemoveAccountsFromList(ctx context.Context, listID string, accountIDs []string) error {
	args := m.Called(ctx, listID, accountIDs)
	return args.Error(0)
}

func (m *MockStorage) GetListAccounts(ctx context.Context, listID string) ([]string, error) {
	args := m.Called(ctx, listID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockStorage) IsAccountInList(ctx context.Context, listID, accountID string) (bool, error) {
	args := m.Called(ctx, listID, accountID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) GetListsContainingAccount(ctx context.Context, accountID, username string) ([]*storage.List, error) {
	args := m.Called(ctx, accountID, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.List), args.Error(1)
}

func (m *MockStorage) GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, listID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TimelineEntry), args.String(1), args.Error(2)
}

func (m *MockStorage) ListInstanceDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.InstanceDomainBlock), args.String(1), args.Error(2)
}

func (m *MockStorage) ListUsersByRole(ctx context.Context, role string) ([]*storage.User, error) {
	args := m.Called(ctx, role)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.User), args.Error(1)
}

// Announcement operations
func (m *MockStorage) CreateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	args := m.Called(ctx, announcement)
	return args.Error(0)
}

func (m *MockStorage) GetAnnouncement(ctx context.Context, id string) (*storage.Announcement, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Announcement), args.Error(1)
}

func (m *MockStorage) GetAnnouncements(ctx context.Context, active bool) ([]*storage.Announcement, error) {
	args := m.Called(ctx, active)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Announcement), args.Error(1)
}

func (m *MockStorage) UpdateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	args := m.Called(ctx, announcement)
	return args.Error(0)
}

func (m *MockStorage) DeleteAnnouncement(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorage) DismissAnnouncement(ctx context.Context, username, announcementID string) error {
	args := m.Called(ctx, username, announcementID)
	return args.Error(0)
}

func (m *MockStorage) GetDismissedAnnouncements(ctx context.Context, username string) ([]string, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockStorage) AddAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error {
	args := m.Called(ctx, username, announcementID, emojiName)
	return args.Error(0)
}

func (m *MockStorage) RemoveAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error {
	args := m.Called(ctx, username, announcementID, emojiName)
	return args.Error(0)
}

func (m *MockStorage) GetAnnouncementReactions(ctx context.Context, announcementID string) (map[string][]string, error) {
	args := m.Called(ctx, announcementID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string][]string), args.Error(1)
}

// Domain Block operations
func (m *MockStorage) AddDomainBlock(ctx context.Context, username, domain string) error {
	args := m.Called(ctx, username, domain)
	return args.Error(0)
}

func (m *MockStorage) RemoveDomainBlock(ctx context.Context, username, domain string) error {
	args := m.Called(ctx, username, domain)
	return args.Error(0)
}

func (m *MockStorage) GetUserDomainBlocks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

func (m *MockStorage) CreateInstanceDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

func (m *MockStorage) GetInstanceDomainBlock(ctx context.Context, domain string) (*storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceDomainBlock), args.Error(1)
}

func (m *MockStorage) GetInstanceDomainBlockByID(ctx context.Context, id string) (*storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceDomainBlock), args.Error(1)
}

func (m *MockStorage) UpdateInstanceDomainBlock(ctx context.Context, domain string, updates map[string]any) error {
	args := m.Called(ctx, domain, updates)
	return args.Error(0)
}

func (m *MockStorage) DeleteInstanceDomainBlock(ctx context.Context, domain string) error {
	args := m.Called(ctx, domain)
	return args.Error(0)
}

func (m *MockStorage) IsInstanceDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, domain)
	return args.Bool(0), args.Get(1).(*storage.InstanceDomainBlock), args.Error(2)
}

func (m *MockStorage) GetDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.InstanceDomainBlock), args.String(1), args.Error(2)
}

func (m *MockStorage) GetDomainBlock(ctx context.Context, id string) (*storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceDomainBlock), args.Error(1)
}

func (m *MockStorage) CreateDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

func (m *MockStorage) UpdateDomainBlock(ctx context.Context, id string, updates map[string]any) error {
	args := m.Called(ctx, id, updates)
	return args.Error(0)
}

func (m *MockStorage) DeleteDomainBlock(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorage) IsDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, domain)
	return args.Bool(0), args.Get(1).(*storage.InstanceDomainBlock), args.Error(2)
}

func (m *MockStorage) CreateEmailDomainBlock(ctx context.Context, block *storage.EmailDomainBlock) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

func (m *MockStorage) GetEmailDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.EmailDomainBlock, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.EmailDomainBlock), args.String(1), args.Error(2)
}

func (m *MockStorage) DeleteEmailDomainBlock(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// Filter operations
func (m *MockStorage) CreateFilter(ctx context.Context, filter *storage.Filter) error {
	args := m.Called(ctx, filter)
	return args.Error(0)
}

func (m *MockStorage) GetFilter(ctx context.Context, filterID string) (*storage.Filter, error) {
	args := m.Called(ctx, filterID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Filter), args.Error(1)
}

func (m *MockStorage) GetFiltersForUser(ctx context.Context, username string) ([]*storage.Filter, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Filter), args.Error(1)
}

func (m *MockStorage) UpdateFilter(ctx context.Context, filterID string, updates map[string]any) error {
	args := m.Called(ctx, filterID, updates)
	return args.Error(0)
}

func (m *MockStorage) DeleteFilter(ctx context.Context, filterID string) error {
	args := m.Called(ctx, filterID)
	return args.Error(0)
}

func (m *MockStorage) AddFilterKeyword(ctx context.Context, filterID string, keyword *storage.FilterKeyword) error {
	args := m.Called(ctx, filterID, keyword)
	return args.Error(0)
}

func (m *MockStorage) GetFilterKeywords(ctx context.Context, filterID string) ([]*storage.FilterKeyword, error) {
	args := m.Called(ctx, filterID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FilterKeyword), args.Error(1)
}

func (m *MockStorage) UpdateFilterKeyword(ctx context.Context, keywordID string, updates map[string]any) error {
	args := m.Called(ctx, keywordID, updates)
	return args.Error(0)
}

func (m *MockStorage) DeleteFilterKeyword(ctx context.Context, keywordID string) error {
	args := m.Called(ctx, keywordID)
	return args.Error(0)
}

func (m *MockStorage) AddFilterStatus(ctx context.Context, filterID string, status *storage.FilterStatus) error {
	args := m.Called(ctx, filterID, status)
	return args.Error(0)
}

func (m *MockStorage) GetFilterStatuses(ctx context.Context, filterID string) ([]*storage.FilterStatus, error) {
	args := m.Called(ctx, filterID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FilterStatus), args.Error(1)
}

func (m *MockStorage) DeleteFilterStatus(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

// Moderation operations
func (m *MockStorage) CreateModerationEvent(ctx context.Context, event *storage.ModerationEvent) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *MockStorage) GetModerationEvent(ctx context.Context, eventID string) (*storage.ModerationEvent, error) {
	args := m.Called(ctx, eventID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ModerationEvent), args.Error(1)
}

func (m *MockStorage) GetModerationQueue(ctx context.Context, filter *storage.ModerationFilter) ([]*storage.ModerationQueueItem, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.ModerationQueueItem), args.Error(1)
}

func (m *MockStorage) GetModerationQueuePaginated(ctx context.Context, limit int, cursor string) ([]*storage.ModerationQueueItem, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.ModerationQueueItem), args.String(1), args.Error(2)
}

func (m *MockStorage) GetModerationEventsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.ModerationEvent), args.String(1), args.Error(2)
}

func (m *MockStorage) GetModerationEventsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.ModerationEvent), args.String(1), args.Error(2)
}

func (m *MockStorage) AddModerationReview(ctx context.Context, review *storage.ModerationReview) error {
	args := m.Called(ctx, review)
	return args.Error(0)
}

func (m *MockStorage) GetModerationReviews(ctx context.Context, eventID string) ([]*storage.ModerationReview, error) {
	args := m.Called(ctx, eventID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.ModerationReview), args.Error(1)
}

func (m *MockStorage) CreateModerationDecision(ctx context.Context, decision *storage.ModerationDecision) error {
	args := m.Called(ctx, decision)
	return args.Error(0)
}

func (m *MockStorage) GetModerationDecision(ctx context.Context, objectID string) (*storage.ModerationDecision, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ModerationDecision), args.Error(1)
}

func (m *MockStorage) StoreModerationDecision(ctx context.Context, decision *storage.ModerationDecision) error {
	args := m.Called(ctx, decision)
	return args.Error(0)
}

func (m *MockStorage) UpdateModerationDecision(ctx context.Context, contentID string, review *storage.ModerationReview) error {
	args := m.Called(ctx, contentID, review)
	return args.Error(0)
}

func (m *MockStorage) GetModerationHistory(ctx context.Context, objectID string) (*storage.ModerationHistory, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ModerationHistory), args.Error(1)
}

func (m *MockStorage) GetModerationEvents(ctx context.Context, filter *storage.ModerationEventFilter, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	args := m.Called(ctx, filter, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.ModerationEvent), args.String(1), args.Error(2)
}

func (m *MockStorage) CreateModerationPattern(ctx context.Context, pattern *storage.ModerationPattern) error {
	args := m.Called(ctx, pattern)
	return args.Error(0)
}

func (m *MockStorage) GetModerationPattern(ctx context.Context, patternID string) (*storage.ModerationPattern, error) {
	args := m.Called(ctx, patternID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ModerationPattern), args.Error(1)
}

func (m *MockStorage) GetModerationPatterns(ctx context.Context, active bool, severity string, limit int) ([]*storage.ModerationPattern, error) {
	args := m.Called(ctx, active, severity, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.ModerationPattern), args.Error(1)
}

func (m *MockStorage) UpdateModerationPattern(ctx context.Context, pattern *storage.ModerationPattern) error {
	args := m.Called(ctx, pattern)
	return args.Error(0)
}

func (m *MockStorage) DeleteModerationPattern(ctx context.Context, patternID string) error {
	args := m.Called(ctx, patternID)
	return args.Error(0)
}

func (m *MockStorage) GetModerationQueueCount(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

// Conversation operations
func (m *MockStorage) CreateConversation(ctx context.Context, conversation *storage.Conversation) error {
	args := m.Called(ctx, conversation)
	return args.Error(0)
}

func (m *MockStorage) GetConversation(ctx context.Context, id string) (*storage.Conversation, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Conversation), args.Error(1)
}

func (m *MockStorage) GetConversationByParticipants(ctx context.Context, participants []string) (*storage.Conversation, error) {
	args := m.Called(ctx, participants)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Conversation), args.Error(1)
}

func (m *MockStorage) UpdateConversationLastStatus(ctx context.Context, id, lastStatusID string) error {
	args := m.Called(ctx, id, lastStatusID)
	return args.Error(0)
}

func (m *MockStorage) MarkConversationRead(ctx context.Context, id, username string) error {
	args := m.Called(ctx, id, username)
	return args.Error(0)
}

func (m *MockStorage) DeleteConversation(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorage) GetUserConversations(ctx context.Context, username string, limit int, cursor string) ([]*storage.Conversation, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Conversation), args.String(1), args.Error(2)
}

func (m *MockStorage) AddParticipantToConversation(ctx context.Context, conversationID, participantID string) error {
	args := m.Called(ctx, conversationID, participantID)
	return args.Error(0)
}

func (m *MockStorage) CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error {
	args := m.Called(ctx, mute)
	return args.Error(0)
}

func (m *MockStorage) DeleteConversationMute(ctx context.Context, username, conversationID string) error {
	args := m.Called(ctx, username, conversationID)
	return args.Error(0)
}

func (m *MockStorage) IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error) {
	args := m.Called(ctx, username, conversationID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) GetMutedConversations(ctx context.Context, username string) ([]string, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// Report operations
func (m *MockStorage) CreateReport(ctx context.Context, report *storage.Report) error {
	args := m.Called(ctx, report)
	return args.Error(0)
}

func (m *MockStorage) GetReport(ctx context.Context, id string) (*storage.Report, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Report), args.Error(1)
}

func (m *MockStorage) GetUserReports(ctx context.Context, username string, limit int, cursor string) ([]*storage.Report, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Report), args.String(1), args.Error(2)
}

func (m *MockStorage) GetReportsByTarget(ctx context.Context, targetAccountID string, limit int, cursor string) ([]*storage.Report, string, error) {
	args := m.Called(ctx, targetAccountID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Report), args.String(1), args.Error(2)
}

func (m *MockStorage) GetReportsByStatus(ctx context.Context, status storage.ReportStatus, limit int, cursor string) ([]*storage.Report, string, error) {
	args := m.Called(ctx, status, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Report), args.String(1), args.Error(2)
}

func (m *MockStorage) UpdateReportStatus(ctx context.Context, id string, status storage.ReportStatus, actionTaken string, moderatorID string) error {
	args := m.Called(ctx, id, status, actionTaken, moderatorID)
	return args.Error(0)
}

func (m *MockStorage) GetReportStats(ctx context.Context, username string) (*storage.ReportStats, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ReportStats), args.Error(1)
}

func (m *MockStorage) IncrementFalseReports(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorage) AssignReport(ctx context.Context, reportID string, assignedTo string) error {
	args := m.Called(ctx, reportID, assignedTo)
	return args.Error(0)
}

func (m *MockStorage) UnassignReport(ctx context.Context, reportID string) error {
	args := m.Called(ctx, reportID)
	return args.Error(0)
}

func (m *MockStorage) GetInstanceHealthReport(ctx context.Context, domain string, period time.Duration) (*storage.InstanceHealthReport, error) {
	args := m.Called(ctx, domain, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceHealthReport), args.Error(1)
}

func (m *MockStorage) GetOpenReportsCount(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

func (m *MockStorage) GetReportedStatuses(ctx context.Context, reportID string) ([]any, error) {
	args := m.Called(ctx, reportID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

// Notification operations
func (m *MockStorage) CreateNotification(ctx context.Context, notification *storage.Notification) error {
	args := m.Called(ctx, notification)
	return args.Error(0)
}

func (m *MockStorage) GetNotification(ctx context.Context, id string) (*storage.Notification, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Notification), args.Error(1)
}

func (m *MockStorage) GetNotifications(ctx context.Context, username string, limit int, cursor string) ([]*storage.Notification, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Notification), args.String(1), args.Error(2)
}

func (m *MockStorage) GetNotificationsFiltered(ctx context.Context, username string, filter *storage.NotificationFilter) ([]*storage.Notification, string, error) {
	args := m.Called(ctx, username, filter)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Notification), args.String(1), args.Error(2)
}

func (m *MockStorage) MarkNotificationAsRead(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorage) MarkAllNotificationsAsRead(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorage) DeleteNotification(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorage) ClearNotifications(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorage) CountUnreadNotifications(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

func (m *MockStorage) GetNotificationsAdvanced(ctx context.Context, userID string, excludeTypes []string, maxID, sinceID, minID *string, limit int, includeFiltered bool) ([]*storage.Notification, error) {
	args := m.Called(ctx, userID, excludeTypes, maxID, sinceID, minID, limit, includeFiltered)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Notification), args.Error(1)
}

func (m *MockStorage) GetNotificationsByAccount(ctx context.Context, userID, accountID string, limit int) ([]*storage.Notification, error) {
	args := m.Called(ctx, userID, accountID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Notification), args.Error(1)
}

func (m *MockStorage) GetUnreadNotificationCount(ctx context.Context, userID string) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockStorage) GetNotificationPreferences(ctx context.Context, username string) (*storage.NotificationPreferences, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.NotificationPreferences), args.Error(1)
}

func (m *MockStorage) UpdateNotificationPreferences(ctx context.Context, username string, prefs *storage.NotificationPreferences) error {
	args := m.Called(ctx, username, prefs)
	return args.Error(0)
}

func (m *MockStorage) BatchMarkNotificationsAsRead(ctx context.Context, username string, notificationIDs []string) error {
	args := m.Called(ctx, username, notificationIDs)
	return args.Error(0)
}

// Cache operations
func (m *MockStorage) CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	args := m.Called(ctx, handle, actor, ttl)
	return args.Error(0)
}

func (m *MockStorage) GetCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error) {
	args := m.Called(ctx, handle)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

func (m *MockStorage) GetDNSCache(ctx context.Context, hostname string) (*storage.DNSCacheEntry, error) {
	args := m.Called(ctx, hostname)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.DNSCacheEntry), args.Error(1)
}

func (m *MockStorage) SetDNSCache(ctx context.Context, entry *storage.DNSCacheEntry) error {
	args := m.Called(ctx, entry)
	return args.Error(0)
}

// Federation operations
func (m *MockStorage) GetFederationStatistics(ctx context.Context, startTime, endTime time.Time) (*storage.FederationStats, error) {
	args := m.Called(ctx, startTime, endTime)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.FederationStats), args.Error(1)
}

func (m *MockStorage) RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

func (m *MockStorage) GetFederationCosts(ctx context.Context, startTime, endTime time.Time, limit int, cursor string) ([]*storage.FederationCost, string, error) {
	args := m.Called(ctx, startTime, endTime, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.FederationCost), args.String(1), args.Error(2)
}

func (m *MockStorage) GetFederationNodes(ctx context.Context, depth int) ([]*storage.FederationNode, error) {
	args := m.Called(ctx, depth)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FederationNode), args.Error(1)
}

func (m *MockStorage) GetFederationEdges(ctx context.Context, domains []string) ([]*storage.FederationEdge, error) {
	args := m.Called(ctx, domains)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FederationEdge), args.Error(1)
}

func (m *MockStorage) CalculateFederationClusters(ctx context.Context) ([]*storage.InstanceCluster, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.InstanceCluster), args.Error(1)
}

func (m *MockStorage) UpdateFederationNode(ctx context.Context, node *storage.FederationNode) error {
	args := m.Called(ctx, node)
	return args.Error(0)
}

func (m *MockStorage) UpdateFederationEdge(ctx context.Context, edge *storage.FederationEdge) error {
	args := m.Called(ctx, edge)
	return args.Error(0)
}

func (m *MockStorage) StoreFederationTimeSeries(ctx context.Context, data *storage.FederationTimeSeries) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

func (m *MockStorage) StoreInstanceCluster(ctx context.Context, cluster *storage.InstanceCluster) error {
	args := m.Called(ctx, cluster)
	return args.Error(0)
}

func (m *MockStorage) TrackFederationIssue(ctx context.Context, domain, issueType string) error {
	args := m.Called(ctx, domain, issueType)
	return args.Error(0)
}

// Rate Limit operations
func (m *MockStorage) CheckCommunityNoteRateLimit(ctx context.Context, userID string, limit int) (bool, int, error) {
	args := m.Called(ctx, userID, limit)
	return args.Bool(0), args.Int(1), args.Error(2)
}

func (m *MockStorage) IsRateLimited(ctx context.Context, identifier string) (bool, time.Time, error) {
	args := m.Called(ctx, identifier)
	return args.Bool(0), args.Get(1).(time.Time), args.Error(2)
}

func (m *MockStorage) CheckAPIRateLimit(ctx context.Context, userID, endpoint string, limit int, window time.Duration) error {
	args := m.Called(ctx, userID, endpoint, limit, window)
	return args.Error(0)
}

func (m *MockStorage) GetAPIRateLimitInfo(ctx context.Context, userID, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error) {
	args := m.Called(ctx, userID, endpoint, limit, window)
	return args.Int(0), args.Get(1).(time.Time), args.Error(2)
}

// Login/Auth operations
func (m *MockStorage) RecordLoginAttempt(ctx context.Context, identifier string, success bool) error {
	args := m.Called(ctx, identifier, success)
	return args.Error(0)
}

func (m *MockStorage) GetLoginAttemptCount(ctx context.Context, identifier string, since time.Time) (int, error) {
	args := m.Called(ctx, identifier, since)
	return args.Int(0), args.Error(1)
}

func (m *MockStorage) ClearLoginAttempts(ctx context.Context, identifier string) error {
	args := m.Called(ctx, identifier)
	return args.Error(0)
}

// Quote operations
func (m *MockStorage) CreateQuoteRelationship(ctx context.Context, quote *storage.QuoteRelationship) error {
	args := m.Called(ctx, quote)
	return args.Error(0)
}

func (m *MockStorage) GetQuotesForNote(ctx context.Context, noteID string, limit int, cursor string) ([]*storage.QuoteRelationship, string, error) {
	args := m.Called(ctx, noteID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.QuoteRelationship), args.String(1), args.Error(2)
}

func (m *MockStorage) IsQuoted(ctx context.Context, actorID, noteID string) (bool, error) {
	args := m.Called(ctx, actorID, noteID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) WithdrawQuote(ctx context.Context, quoteNoteID string) error {
	args := m.Called(ctx, quoteNoteID)
	return args.Error(0)
}

func (m *MockStorage) CountQuotes(ctx context.Context, noteID string) (int, error) {
	args := m.Called(ctx, noteID)
	return args.Int(0), args.Error(1)
}

func (m *MockStorage) WithdrawStatusFromQuotes(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

func (m *MockStorage) UpdateQuotePermissions(ctx context.Context, statusID string, permissions *storage.QuotePermissions) error {
	args := m.Called(ctx, statusID, permissions)
	return args.Error(0)
}

func (m *MockStorage) IsQuoteAllowed(ctx context.Context, statusID, quoterID string) (bool, error) {
	args := m.Called(ctx, statusID, quoterID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) GetQuoteType(ctx context.Context, statusID string) (string, error) {
	args := m.Called(ctx, statusID)
	return args.String(0), args.Error(1)
}

func (m *MockStorage) IsWithdrawnFromQuotes(ctx context.Context, statusID string) (bool, error) {
	args := m.Called(ctx, statusID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) GetQuotesOfStatus(ctx context.Context, statusID string, limit int) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, statusID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// Reply operations
func (m *MockStorage) GetReplies(ctx context.Context, objectID string, limit int, cursor string) ([]any, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]any), args.String(1), args.Error(2)
}

func (m *MockStorage) CountReplies(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

func (m *MockStorage) IncrementReplyCount(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

func (m *MockStorage) SyncThreadFromRemote(ctx context.Context, statusID string) (*storage.StatusSearchResult, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorage) SyncMissingRepliesFromRemote(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorage) GetMissingReplies(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorage) GetReplyCount(ctx context.Context, statusID string) (int64, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockStorage) GetStatusReplyCount(ctx context.Context, statusID string) (int, error) {
	args := m.Called(ctx, statusID)
	return args.Int(0), args.Error(1)
}

// Recovery Code operations (excluding duplicates)

func (m *MockStorage) StoreRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error {
	args := m.Called(ctx, request)
	return args.Error(0)
}

func (m *MockStorage) GetRecoveryRequest(ctx context.Context, requestID string) (*storage.SocialRecoveryRequest, error) {
	args := m.Called(ctx, requestID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SocialRecoveryRequest), args.Error(1)
}

func (m *MockStorage) UpdateRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error {
	args := m.Called(ctx, request)
	return args.Error(0)
}

func (m *MockStorage) DeleteRecoveryRequest(ctx context.Context, requestID string) error {
	args := m.Called(ctx, requestID)
	return args.Error(0)
}

func (m *MockStorage) GetActiveRecoveryRequests(ctx context.Context, username string) ([]*storage.SocialRecoveryRequest, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.SocialRecoveryRequest), args.Error(1)
}

func (m *MockStorage) StoreRecoveryCode(ctx context.Context, username string, code *storage.RecoveryCodeItem) error {
	args := m.Called(ctx, username, code)
	return args.Error(0)
}

func (m *MockStorage) GetRecoveryCodes(ctx context.Context, username string) ([]*storage.RecoveryCodeItem, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.RecoveryCodeItem), args.Error(1)
}

func (m *MockStorage) MarkRecoveryCodeUsed(ctx context.Context, username, codeHash string) error {
	args := m.Called(ctx, username, codeHash)
	return args.Error(0)
}

func (m *MockStorage) DeleteAllRecoveryCodes(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorage) CountUnusedRecoveryCodes(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

// Pin operations
func (m *MockStorage) CreateAccountPin(ctx context.Context, pin *storage.AccountPin) error {
	args := m.Called(ctx, pin)
	return args.Error(0)
}

func (m *MockStorage) DeleteAccountPin(ctx context.Context, username, pinnedActorID string) error {
	args := m.Called(ctx, username, pinnedActorID)
	return args.Error(0)
}

func (m *MockStorage) GetAccountPins(ctx context.Context, username string) ([]*storage.AccountPin, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.AccountPin), args.Error(1)
}

func (m *MockStorage) IsAccountPinned(ctx context.Context, username, actorID string) (bool, error) {
	args := m.Called(ctx, username, actorID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) CreateStatusPin(ctx context.Context, pin *storage.StatusPin) error {
	args := m.Called(ctx, pin)
	return args.Error(0)
}

func (m *MockStorage) DeleteStatusPin(ctx context.Context, username, statusID string) error {
	args := m.Called(ctx, username, statusID)
	return args.Error(0)
}

func (m *MockStorage) GetStatusPins(ctx context.Context, username string) ([]*storage.StatusPin, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusPin), args.Error(1)
}

func (m *MockStorage) IsStatusPinned(ctx context.Context, username, statusID string) (bool, error) {
	args := m.Called(ctx, username, statusID)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) CountUserPinnedStatuses(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

// Account Note operations
func (m *MockStorage) CreateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	args := m.Called(ctx, note)
	return args.Error(0)
}

func (m *MockStorage) GetAccountNote(ctx context.Context, username, targetActorID string) (*storage.AccountNote, error) {
	args := m.Called(ctx, username, targetActorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.AccountNote), args.Error(1)
}

func (m *MockStorage) UpdateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	args := m.Called(ctx, note)
	return args.Error(0)
}

func (m *MockStorage) DeleteAccountNote(ctx context.Context, username, targetActorID string) error {
	args := m.Called(ctx, username, targetActorID)
	return args.Error(0)
}

// Admin operations
func (m *MockStorage) CreateAdminReview(ctx context.Context, eventID string, adminID string, action storage.ActionType, reason string) error {
	args := m.Called(ctx, eventID, adminID, action, reason)
	return args.Error(0)
}

func (m *MockStorage) GetReviewerStats(ctx context.Context, reviewerID string) (*storage.ReviewerStats, error) {
	args := m.Called(ctx, reviewerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ReviewerStats), args.Error(1)
}

// Bookmark operations
func (m *MockStorage) CreateBookmark(ctx context.Context, username, objectID string) error {
	args := m.Called(ctx, username, objectID)
	return args.Error(0)
}

func (m *MockStorage) RemoveBookmark(ctx context.Context, username, objectID string) error {
	args := m.Called(ctx, username, objectID)
	return args.Error(0)
}

func (m *MockStorage) GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

func (m *MockStorage) IsBookmarked(ctx context.Context, username, objectID string) (bool, error) {
	args := m.Called(ctx, username, objectID)
	return args.Bool(0), args.Error(1)
}

// Community Note operations
func (m *MockStorage) CreateCommunityNote(ctx context.Context, note *storage.CommunityNote) error {
	args := m.Called(ctx, note)
	return args.Error(0)
}

func (m *MockStorage) GetCommunityNote(ctx context.Context, noteID string) (*storage.CommunityNote, error) {
	args := m.Called(ctx, noteID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.CommunityNote), args.Error(1)
}

func (m *MockStorage) GetCommunityNotesByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*storage.CommunityNote, string, error) {
	args := m.Called(ctx, authorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.CommunityNote), args.String(1), args.Error(2)
}

func (m *MockStorage) GetVisibleCommunityNotes(ctx context.Context, objectID string) ([]*storage.CommunityNote, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CommunityNote), args.Error(1)
}

func (m *MockStorage) UpdateCommunityNoteScore(ctx context.Context, noteID string, score float64, status string) error {
	args := m.Called(ctx, noteID, score, status)
	return args.Error(0)
}

func (m *MockStorage) UpdateCommunityNoteAnalysis(ctx context.Context, noteID string, sentiment, objectivity, sourceQuality float64) error {
	args := m.Called(ctx, noteID, sentiment, objectivity, sourceQuality)
	return args.Error(0)
}

func (m *MockStorage) CreateCommunityNoteVote(ctx context.Context, vote *storage.CommunityNoteVote) error {
	args := m.Called(ctx, vote)
	return args.Error(0)
}

func (m *MockStorage) GetCommunityNoteVotes(ctx context.Context, noteID string) ([]*storage.CommunityNoteVote, error) {
	args := m.Called(ctx, noteID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CommunityNoteVote), args.Error(1)
}

func (m *MockStorage) GetUserCommunityNoteVotes(ctx context.Context, userID string, noteIDs []string) (map[string]*storage.CommunityNoteVote, error) {
	args := m.Called(ctx, userID, noteIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]*storage.CommunityNoteVote), args.Error(1)
}

// Custom Emoji operations
func (m *MockStorage) CreateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error {
	args := m.Called(ctx, emoji)
	return args.Error(0)
}

func (m *MockStorage) GetCustomEmoji(ctx context.Context, shortcode string) (*storage.CustomEmoji, error) {
	args := m.Called(ctx, shortcode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.CustomEmoji), args.Error(1)
}

func (m *MockStorage) GetCustomEmojis(ctx context.Context) ([]*storage.CustomEmoji, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CustomEmoji), args.Error(1)
}

func (m *MockStorage) UpdateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error {
	args := m.Called(ctx, emoji)
	return args.Error(0)
}

func (m *MockStorage) DeleteCustomEmoji(ctx context.Context, shortcode string) error {
	args := m.Called(ctx, shortcode)
	return args.Error(0)
}

func (m *MockStorage) GetCustomEmojisByCategory(ctx context.Context, category string) ([]*storage.CustomEmoji, error) {
	args := m.Called(ctx, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CustomEmoji), args.Error(1)
}

// Device operations
func (m *MockStorage) CreateDevice(ctx context.Context, device *storage.Device) error {
	args := m.Called(ctx, device)
	return args.Error(0)
}

func (m *MockStorage) GetDevice(ctx context.Context, deviceID string) (*storage.Device, error) {
	args := m.Called(ctx, deviceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Device), args.Error(1)
}

func (m *MockStorage) UpdateDevice(ctx context.Context, device *storage.Device) error {
	args := m.Called(ctx, device)
	return args.Error(0)
}

func (m *MockStorage) GetUserDevices(ctx context.Context, username string) ([]*storage.Device, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Device), args.Error(1)
}

func (m *MockStorage) GetStreamingPreferencesByDevice(ctx context.Context, username, deviceID string) (*storage.StreamingPreferences, error) {
	args := m.Called(ctx, username, deviceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StreamingPreferences), args.Error(1)
}

func (m *MockStorage) UpdateDeviceStreamingPreferences(ctx context.Context, prefs *storage.StreamingPreferences, deviceID string) error {
	args := m.Called(ctx, prefs, deviceID)
	return args.Error(0)
}

// Domain Allow operations
func (m *MockStorage) CreateDomainAllow(ctx context.Context, allow *storage.DomainAllow) error {
	args := m.Called(ctx, allow)
	return args.Error(0)
}

func (m *MockStorage) GetDomainAllows(ctx context.Context, limit int, cursor string) ([]*storage.DomainAllow, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.DomainAllow), args.String(1), args.Error(2)
}

func (m *MockStorage) DeleteDomainAllow(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// Featured Tag operations
func (m *MockStorage) CreateFeaturedTag(ctx context.Context, userID string, tagName string) (*storage.FeaturedTag, error) {
	args := m.Called(ctx, userID, tagName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.FeaturedTag), args.Error(1)
}

func (m *MockStorage) DeleteFeaturedTag(ctx context.Context, userID string, featuredTagID string) error {
	args := m.Called(ctx, userID, featuredTagID)
	return args.Error(0)
}

func (m *MockStorage) GetFeaturedTags(ctx context.Context, userID string) ([]*storage.FeaturedTag, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FeaturedTag), args.Error(1)
}

// Mute operations
func (m *MockStorage) CreateMute(ctx context.Context, mute *storage.Mute) error {
	args := m.Called(ctx, mute)
	return args.Error(0)
}

func (m *MockStorage) GetMute(ctx context.Context, actor, mutedActor string) (*storage.Mute, error) {
	args := m.Called(ctx, actor, mutedActor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Mute), args.Error(1)
}

func (m *MockStorage) DeleteMute(ctx context.Context, actor, mutedActor string) error {
	args := m.Called(ctx, actor, mutedActor)
	return args.Error(0)
}

func (m *MockStorage) GetMutedActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Mute, string, error) {
	args := m.Called(ctx, actor, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Mute), args.String(1), args.Error(2)
}

func (m *MockStorage) IsMuted(ctx context.Context, actor, targetActor string) (bool, error) {
	args := m.Called(ctx, actor, targetActor)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) UpdateHashtagNotificationSettings(ctx context.Context, userID, hashtag string, notify bool) error {
	args := m.Called(ctx, userID, hashtag, notify)
	return args.Error(0)
}

func (m *MockStorage) MuteHashtag(ctx context.Context, userID, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

func (m *MockStorage) UnmuteHashtag(ctx context.Context, userID, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

func (m *MockStorage) IsHashtagMuted(ctx context.Context, userID, hashtag string) (bool, error) {
	args := m.Called(ctx, userID, hashtag)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) IsNotificationMuted(ctx context.Context, userID, targetID string) (bool, error) {
	args := m.Called(ctx, userID, targetID)
	return args.Bool(0), args.Error(1)
}

// ==================== Poll Operations ====================

func (m *MockStorage) CreatePoll(ctx context.Context, poll *storage.Poll) error {
	args := m.Called(ctx, poll)
	return args.Error(0)
}

func (m *MockStorage) GetPoll(ctx context.Context, pollID string) (*storage.Poll, error) {
	args := m.Called(ctx, pollID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Poll), args.Error(1)
}

func (m *MockStorage) GetPollByStatusID(ctx context.Context, statusID string) (*storage.Poll, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Poll), args.Error(1)
}

func (m *MockStorage) VoteOnPoll(ctx context.Context, pollID string, voterID string, choices []int) error {
	args := m.Called(ctx, pollID, voterID, choices)
	return args.Error(0)
}

func (m *MockStorage) GetPollVotes(ctx context.Context, pollID string) (map[string][]int, error) {
	args := m.Called(ctx, pollID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string][]int), args.Error(1)
}

// ==================== Push Subscription Operations ====================

func (m *MockStorage) CreatePushSubscription(ctx context.Context, username string, subscription *storage.PushSubscription) error {
	args := m.Called(ctx, username, subscription)
	return args.Error(0)
}

func (m *MockStorage) GetPushSubscription(ctx context.Context, username, subscriptionID string) (*storage.PushSubscription, error) {
	args := m.Called(ctx, username, subscriptionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.PushSubscription), args.Error(1)
}

func (m *MockStorage) GetUserPushSubscriptions(ctx context.Context, username string) ([]*storage.PushSubscription, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.PushSubscription), args.Error(1)
}

func (m *MockStorage) UpdatePushSubscription(ctx context.Context, username, subscriptionID string, alerts storage.PushSubscriptionAlerts) error {
	args := m.Called(ctx, username, subscriptionID, alerts)
	return args.Error(0)
}

func (m *MockStorage) DeletePushSubscription(ctx context.Context, username, subscriptionID string) error {
	args := m.Called(ctx, username, subscriptionID)
	return args.Error(0)
}

func (m *MockStorage) DeleteAllPushSubscriptions(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// ==================== Scheduled Status Operations ====================

func (m *MockStorage) CreateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	args := m.Called(ctx, scheduled)
	return args.Error(0)
}

func (m *MockStorage) GetScheduledStatus(ctx context.Context, id string) (*storage.ScheduledStatus, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ScheduledStatus), args.Error(1)
}

func (m *MockStorage) GetScheduledStatuses(ctx context.Context, username string, limit int, cursor string) ([]*storage.ScheduledStatus, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.ScheduledStatus), args.String(1), args.Error(2)
}

func (m *MockStorage) UpdateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	args := m.Called(ctx, scheduled)
	return args.Error(0)
}

func (m *MockStorage) DeleteScheduledStatus(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorage) GetDueScheduledStatuses(ctx context.Context, before time.Time, limit int) ([]*storage.ScheduledStatus, error) {
	args := m.Called(ctx, before, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.ScheduledStatus), args.Error(1)
}

func (m *MockStorage) MarkScheduledStatusPublished(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorage) GetScheduledStatusMedia(ctx context.Context, statusID string) ([]any, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

// ==================== Session Operations ====================

func (m *MockStorage) CreateSession(ctx context.Context, session *storage.Session) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

func (m *MockStorage) GetSession(ctx context.Context, sessionID string) (*storage.Session, error) {
	args := m.Called(ctx, sessionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Session), args.Error(1)
}

func (m *MockStorage) GetSessionByRefreshToken(ctx context.Context, refreshToken string) (*storage.Session, error) {
	args := m.Called(ctx, refreshToken)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Session), args.Error(1)
}

func (m *MockStorage) UpdateSession(ctx context.Context, session *storage.Session) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

func (m *MockStorage) DeleteSession(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

func (m *MockStorage) GetUserSessions(ctx context.Context, username string) ([]*storage.Session, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Session), args.Error(1)
}

// ==================== Severed Relationship Operations ====================

func (m *MockStorage) CreateSeveredRelationship(ctx context.Context, rel *storage.SeveredRelationship) error {
	args := m.Called(ctx, rel)
	return args.Error(0)
}

func (m *MockStorage) GetSeveredRelationships(ctx context.Context, localInstance string, limit int, cursor string) ([]*storage.SeveredRelationship, string, error) {
	args := m.Called(ctx, localInstance, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.SeveredRelationship), args.String(1), args.Error(2)
}

func (m *MockStorage) GetSeveredRelationship(ctx context.Context, localInstance, remoteInstance string) (*storage.SeveredRelationship, error) {
	args := m.Called(ctx, localInstance, remoteInstance)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SeveredRelationship), args.Error(1)
}

func (m *MockStorage) UpdateSeveredRelationship(ctx context.Context, rel *storage.SeveredRelationship) error {
	args := m.Called(ctx, rel)
	return args.Error(0)
}

// ==================== Trust Relationship Operations ====================

func (m *MockStorage) CreateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	args := m.Called(ctx, relationship)
	return args.Error(0)
}

func (m *MockStorage) GetTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error) {
	args := m.Called(ctx, trusterID, trusteeID, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.TrustRelationship), args.Error(1)
}

func (m *MockStorage) UpdateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	args := m.Called(ctx, relationship)
	return args.Error(0)
}

func (m *MockStorage) DeleteTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) error {
	args := m.Called(ctx, trusterID, trusteeID, category)
	return args.Error(0)
}

func (m *MockStorage) GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	args := m.Called(ctx, trusterID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TrustRelationship), args.String(1), args.Error(2)
}

func (m *MockStorage) GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	args := m.Called(ctx, trusteeID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TrustRelationship), args.String(1), args.Error(2)
}

func (m *MockStorage) GetTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error) {
	args := m.Called(ctx, actorID, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.TrustScore), args.Error(1)
}

func (m *MockStorage) UpdateTrustScore(ctx context.Context, score *storage.TrustScore) error {
	args := m.Called(ctx, score)
	return args.Error(0)
}

func (m *MockStorage) RecordTrustUpdate(ctx context.Context, update *storage.TrustUpdate) error {
	args := m.Called(ctx, update)
	return args.Error(0)
}

func (m *MockStorage) GetAllTrustRelationships(ctx context.Context, limit int) ([]*storage.TrustRelationship, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrustRelationship), args.Error(1)
}

func (m *MockStorage) StoreTrustee(ctx context.Context, username string, trustee *storage.TrusteeConfig) error {
	args := m.Called(ctx, username, trustee)
	return args.Error(0)
}

func (m *MockStorage) GetTrustees(ctx context.Context, username string) ([]*storage.TrusteeConfig, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrusteeConfig), args.Error(1)
}

func (m *MockStorage) DeleteTrustee(ctx context.Context, username, trusteeActorID string) error {
	args := m.Called(ctx, username, trusteeActorID)
	return args.Error(0)
}

func (m *MockStorage) UpdateTrusteeConfirmed(ctx context.Context, username, trusteeActorID string, confirmed bool) error {
	args := m.Called(ctx, username, trusteeActorID, confirmed)
	return args.Error(0)
}

func (m *MockStorage) GetUserTrustScore(ctx context.Context, userID string) (float64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(float64), args.Error(1)
}

// ==================== Vouch Operations ====================

func (m *MockStorage) CreateVouch(ctx context.Context, vouch *storage.Vouch) error {
	args := m.Called(ctx, vouch)
	return args.Error(0)
}

func (m *MockStorage) GetVouch(ctx context.Context, vouchID string) (*storage.Vouch, error) {
	args := m.Called(ctx, vouchID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Vouch), args.Error(1)
}

func (m *MockStorage) GetVouchesByActor(ctx context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	args := m.Called(ctx, actorID, activeOnly)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Vouch), args.Error(1)
}

func (m *MockStorage) GetVouchesForActor(ctx context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	args := m.Called(ctx, actorID, activeOnly)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Vouch), args.Error(1)
}

func (m *MockStorage) UpdateVouchStatus(ctx context.Context, vouchID string, active bool, revokedAt *time.Time) error {
	args := m.Called(ctx, vouchID, active, revokedAt)
	return args.Error(0)
}

func (m *MockStorage) GetMonthlyVouchCount(ctx context.Context, actorID string, year int, month time.Month) (int, error) {
	args := m.Called(ctx, actorID, year, month)
	return args.Int(0), args.Error(1)
}

// ==================== Timeline Cleanup Operations ====================

func (m *MockStorage) DeleteExpiredTimelineEntries(ctx context.Context, before time.Time) error {
	args := m.Called(ctx, before)
	return args.Error(0)
}

func (m *MockStorage) DeleteFromTimeline(ctx context.Context, timelineType, timelineID, entryID string) error {
	args := m.Called(ctx, timelineType, timelineID, entryID)
	return args.Error(0)
}

func (m *MockStorage) DeleteOldHashtagTrends(ctx context.Context, before time.Time) error {
	args := m.Called(ctx, before)
	return args.Error(0)
}

func (m *MockStorage) StoreHashtagTrend(ctx context.Context, trend any) error {
	args := m.Called(ctx, trend)
	return args.Error(0)
}

func (m *MockStorage) DeleteOldLinkTrends(ctx context.Context, before time.Time) error {
	args := m.Called(ctx, before)
	return args.Error(0)
}

func (m *MockStorage) StoreLinkTrend(ctx context.Context, trend any) error {
	args := m.Called(ctx, trend)
	return args.Error(0)
}

func (m *MockStorage) DeleteOldStatusTrends(ctx context.Context, before time.Time) error {
	args := m.Called(ctx, before)
	return args.Error(0)
}

func (m *MockStorage) StoreStatusTrend(ctx context.Context, trend any) error {
	args := m.Called(ctx, trend)
	return args.Error(0)
}

// ==================== Wallet Operations ====================

func (m *MockStorage) StoreWalletChallenge(ctx context.Context, challenge *storage.WalletChallenge) error {
	args := m.Called(ctx, challenge)
	return args.Error(0)
}

func (m *MockStorage) GetWalletChallenge(ctx context.Context, challengeID string) (*storage.WalletChallenge, error) {
	args := m.Called(ctx, challengeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.WalletChallenge), args.Error(1)
}

func (m *MockStorage) DeleteWalletChallenge(ctx context.Context, challengeID string) error {
	args := m.Called(ctx, challengeID)
	return args.Error(0)
}

func (m *MockStorage) StoreWalletCredential(ctx context.Context, credential *storage.WalletCredential) error {
	args := m.Called(ctx, credential)
	return args.Error(0)
}

func (m *MockStorage) GetWalletCredential(ctx context.Context, walletType, address string) (*storage.WalletCredential, error) {
	args := m.Called(ctx, walletType, address)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.WalletCredential), args.Error(1)
}

func (m *MockStorage) GetUserWalletCredentials(ctx context.Context, username string) ([]*storage.WalletCredential, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.WalletCredential), args.Error(1)
}

func (m *MockStorage) DeleteWalletCredential(ctx context.Context, username, address string) error {
	args := m.Called(ctx, username, address)
	return args.Error(0)
}

func (m *MockStorage) UpdateWalletLastUsed(ctx context.Context, username, address string) error {
	args := m.Called(ctx, username, address)
	return args.Error(0)
}

// ==================== WebAuthn Operations ====================

func (m *MockStorage) StoreWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error {
	args := m.Called(ctx, credential)
	return args.Error(0)
}

func (m *MockStorage) GetWebAuthnCredential(ctx context.Context, credentialID string) (*storage.WebAuthnCredential, error) {
	args := m.Called(ctx, credentialID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.WebAuthnCredential), args.Error(1)
}

func (m *MockStorage) GetUserWebAuthnCredentials(ctx context.Context, username string) ([]*storage.WebAuthnCredential, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.WebAuthnCredential), args.Error(1)
}

func (m *MockStorage) UpdateWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error {
	args := m.Called(ctx, credential)
	return args.Error(0)
}

func (m *MockStorage) DeleteWebAuthnCredential(ctx context.Context, credentialID string) error {
	args := m.Called(ctx, credentialID)
	return args.Error(0)
}

func (m *MockStorage) StoreWebAuthnChallenge(ctx context.Context, challenge *storage.WebAuthnChallenge) error {
	args := m.Called(ctx, challenge)
	return args.Error(0)
}

func (m *MockStorage) GetWebAuthnChallenge(ctx context.Context, challengeID string) (*storage.WebAuthnChallenge, error) {
	args := m.Called(ctx, challengeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.WebAuthnChallenge), args.Error(1)
}

func (m *MockStorage) DeleteWebAuthnChallenge(ctx context.Context, challengeID string) error {
	args := m.Called(ctx, challengeID)
	return args.Error(0)
}

// ==================== Fan Out Operations ====================

func (m *MockStorage) FanOutPost(ctx context.Context, activity *activitypub.Activity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

// ==================== Hashtag Follow Operations ====================

func (m *MockStorage) FollowHashtag(ctx context.Context, userID string, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

func (m *MockStorage) UnfollowHashtag(ctx context.Context, userID string, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

func (m *MockStorage) IsFollowingHashtag(ctx context.Context, userID string, hashtag string) (bool, error) {
	args := m.Called(ctx, userID, hashtag)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) GetFollowedHashtags(ctx context.Context, userID string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, userID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

func (m *MockStorage) GenerateSearchSuggestions(ctx context.Context, userID, partialQuery string, limit int) ([]string, error) {
	args := m.Called(ctx, userID, partialQuery, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockStorage) GetAccountSuggestions(ctx context.Context, userID string, limit int) ([]*activitypub.Actor, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*activitypub.Actor), args.Error(1)
}

func (m *MockStorage) RemoveAccountSuggestion(ctx context.Context, userID, targetID string) error {
	args := m.Called(ctx, userID, targetID)
	return args.Error(0)
}

// ==================== Relay Operations ====================

func (m *MockStorage) StoreRelayInfo(ctx context.Context, relay *storage.RelayInfo) error {
	args := m.Called(ctx, relay)
	return args.Error(0)
}

func (m *MockStorage) GetRelayInfo(ctx context.Context, relayURL string) (*storage.RelayInfo, error) {
	args := m.Called(ctx, relayURL)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.RelayInfo), args.Error(1)
}

func (m *MockStorage) RemoveRelayInfo(ctx context.Context, relayURL string) error {
	args := m.Called(ctx, relayURL)
	return args.Error(0)
}

func (m *MockStorage) GetActiveRelays(ctx context.Context) ([]*storage.RelayInfo, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.RelayInfo), args.Error(1)
}

func (m *MockStorage) GetAllRelays(ctx context.Context, limit int, cursor string) ([]*storage.RelayInfo, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.RelayInfo), args.String(1), args.Error(2)
}

func (m *MockStorage) UpdateRelayStatus(ctx context.Context, relayURL string, active bool) error {
	args := m.Called(ctx, relayURL, active)
	return args.Error(0)
}

// ==================== Affected Relationship Operations ====================

func (m *MockStorage) GetAffectedRelationships(ctx context.Context, userID, domain string) ([]*storage.RelationshipRecord, error) {
	args := m.Called(ctx, userID, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.RelationshipRecord), args.Error(1)
}

func (m *MockStorage) GetAffectedFollows(ctx context.Context, localInstance, remoteInstance string) ([]storage.AffectedFollow, error) {
	args := m.Called(ctx, localInstance, remoteInstance)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.AffectedFollow), args.Error(1)
}

func (m *MockStorage) RecordAffectedFollow(ctx context.Context, localInstance, remoteInstance string, follow storage.AffectedFollow) error {
	args := m.Called(ctx, localInstance, remoteInstance, follow)
	return args.Error(0)
}

func (m *MockStorage) ReverseSeverance(ctx context.Context, localInstance, remoteInstance string) error {
	args := m.Called(ctx, localInstance, remoteInstance)
	return args.Error(0)
}

// ==================== Preference Operations ====================

func (m *MockStorage) GetUserLanguagePreference(ctx context.Context, username string) (string, error) {
	args := m.Called(ctx, username)
	return args.String(0), args.Error(1)
}

func (m *MockStorage) SetUserLanguagePreference(ctx context.Context, username string, language string) error {
	args := m.Called(ctx, username, language)
	return args.Error(0)
}

func (m *MockStorage) GetUserPreferences(ctx context.Context, username string) (*storage.UserPreferences, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.UserPreferences), args.Error(1)
}

func (m *MockStorage) UpdateUserPreferences(ctx context.Context, username string, preferences *storage.UserPreferences) error {
	args := m.Called(ctx, username, preferences)
	return args.Error(0)
}

func (m *MockStorage) SetPreference(ctx context.Context, username string, key string, value any) error {
	args := m.Called(ctx, username, key, value)
	return args.Error(0)
}

func (m *MockStorage) GetPreference(ctx context.Context, username string, key string) (any, error) {
	args := m.Called(ctx, username, key)
	return args.Get(0), args.Error(1)
}

func (m *MockStorage) GetAllPreferences(ctx context.Context, username string) (map[string]any, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]any), args.Error(1)
}

func (m *MockStorage) UpdatePreferences(ctx context.Context, username string, prefs map[string]any) error {
	args := m.Called(ctx, username, prefs)
	return args.Error(0)
}

func (m *MockStorage) GetStreamingPreferences(ctx context.Context, username string) (*storage.StreamingPreferences, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StreamingPreferences), args.Error(1)
}

func (m *MockStorage) UpdateStreamingPreferences(ctx context.Context, prefs *storage.StreamingPreferences) error {
	args := m.Called(ctx, prefs)
	return args.Error(0)
}

func (m *MockStorage) GetStreamingPreferenceHistory(ctx context.Context, username string, limit int) ([]*storage.StreamingPreferences, error) {
	args := m.Called(ctx, username, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StreamingPreferences), args.Error(1)
}

func (m *MockStorage) SyncStreamingPreferences(ctx context.Context, username string, sourceDeviceID string) error {
	args := m.Called(ctx, username, sourceDeviceID)
	return args.Error(0)
}

func (m *MockStorage) ResolvePreferenceConflict(ctx context.Context, username string, strategy storage.ConflictResolutionStrategy) (*storage.StreamingPreferences, error) {
	args := m.Called(ctx, username, strategy)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StreamingPreferences), args.Error(1)
}

// ==================== Boost Operations ====================

func (m *MockStorage) GetBoostCount(ctx context.Context, statusID string) (int64, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0).(int64), args.Error(1)
}

// ==================== Cost Operations ====================

func (m *MockStorage) GetCostProjections(ctx context.Context, period string) (*storage.CostProjection, error) {
	args := m.Called(ctx, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.CostProjection), args.Error(1)
}

// ==================== User Count Operations ====================

func (m *MockStorage) GetDailyActiveUserCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockStorage) GetDirectTimeline(ctx context.Context, username string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TimelineEntry), args.String(1), args.Error(2)
}

func (m *MockStorage) GetDomainStats(ctx context.Context, domain string) (any, error) {
	args := m.Called(ctx, domain)
	return args.Get(0), args.Error(1)
}

// ==================== Engagement Operations ====================

func (m *MockStorage) RecordStatusEngagement(ctx context.Context, statusID string, engagementType string, userID string) error {
	args := m.Called(ctx, statusID, engagementType, userID)
	return args.Error(0)
}

func (m *MockStorage) StoreEngagementMetrics(ctx context.Context, metrics *storage.EngagementMetrics) error {
	args := m.Called(ctx, metrics)
	return args.Error(0)
}

func (m *MockStorage) IndexByEngagement(ctx context.Context, statusID string, bucket string) error {
	args := m.Called(ctx, statusID, bucket)
	return args.Error(0)
}

func (m *MockStorage) GetEngagementMetrics(ctx context.Context, statusID string) (*storage.EngagementMetrics, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.EngagementMetrics), args.Error(1)
}

func (m *MockStorage) GetRecentStatusesWithEngagement(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingStatus), args.Error(1)
}

func (m *MockStorage) GetFieldVerification(ctx context.Context, username, fieldName string) (*storage.ActorField, error) {
	args := m.Called(ctx, username, fieldName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ActorField), args.Error(1)
}

func (m *MockStorage) GetFollowersCount(ctx context.Context, actorID string) (int, error) {
	args := m.Called(ctx, actorID)
	return args.Int(0), args.Error(1)
}

func (m *MockStorage) GetFollowingCount(ctx context.Context, actorID string) (int, error) {
	args := m.Called(ctx, actorID)
	return args.Int(0), args.Error(1)
}

func (m *MockStorage) GetHashtagActivity(ctx context.Context, hashtag string, since time.Time) ([]*storage.Activity, error) {
	args := m.Called(ctx, hashtag, since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Activity), args.Error(1)
}

func (m *MockStorage) GetHashtagInfo(ctx context.Context, hashtag string) (*storage.Hashtag, error) {
	args := m.Called(ctx, hashtag)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Hashtag), args.Error(1)
}

func (m *MockStorage) GetHashtagStats(ctx context.Context, hashtag string) (any, error) {
	args := m.Called(ctx, hashtag)
	return args.Get(0), args.Error(1)
}

func (m *MockStorage) GetHashtagTimeline(ctx context.Context, hashtag string, local bool, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, hashtag, local, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TimelineEntry), args.String(1), args.Error(2)
}

func (m *MockStorage) GetHashtagTimelineAdvanced(ctx context.Context, hashtag string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, hashtag, maxID, limit, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorage) GetHashtagUsageHistory(ctx context.Context, hashtag string, days int) ([]int64, error) {
	args := m.Called(ctx, hashtag, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]int64), args.Error(1)
}

func (m *MockStorage) GetHomeTimeline(ctx context.Context, username string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TimelineEntry), args.String(1), args.Error(2)
}

// ==================== Instance Connection Operations ====================

func (m *MockStorage) GetInstanceConnections(ctx context.Context, domain string, connectionType string) ([]*storage.InstanceConnection, error) {
	args := m.Called(ctx, domain, connectionType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.InstanceConnection), args.Error(1)
}

func (m *MockStorage) GetRecentInstanceConnections(ctx context.Context, domain string, since time.Duration) ([]*storage.InstanceConnection, error) {
	args := m.Called(ctx, domain, since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.InstanceConnection), args.Error(1)
}

func (m *MockStorage) GetInstanceInfo(ctx context.Context, domain string) (*storage.InstanceInfo, error) {
	args := m.Called(ctx, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceInfo), args.Error(1)
}

func (m *MockStorage) UpsertInstanceInfo(ctx context.Context, info *storage.InstanceInfo) error {
	args := m.Called(ctx, info)
	return args.Error(0)
}

func (m *MockStorage) GetInstanceMetadata(ctx context.Context, domain string) (*storage.InstanceMetadata, error) {
	args := m.Called(ctx, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceMetadata), args.Error(1)
}

func (m *MockStorage) UpdateInstanceMetadata(ctx context.Context, metadata *storage.InstanceMetadata) error {
	args := m.Called(ctx, metadata)
	return args.Error(0)
}

func (m *MockStorage) GetInstanceRules(ctx context.Context) ([]storage.InstanceRule, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.InstanceRule), args.Error(1)
}

func (m *MockStorage) SetInstanceRules(ctx context.Context, rules []storage.InstanceRule) error {
	args := m.Called(ctx, rules)
	return args.Error(0)
}

func (m *MockStorage) GetKnownInstances(ctx context.Context, limit int, cursor string) ([]*storage.InstanceInfo, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.InstanceInfo), args.String(1), args.Error(2)
}

func (m *MockStorage) GetLatestStatus(ctx context.Context, actorID string) (*storage.StatusSearchResult, error) {
	args := m.Called(ctx, actorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StatusSearchResult), args.Error(1)
}

// ==================== Extended Description Operations ====================

func (m *MockStorage) GetExtendedDescription(ctx context.Context) (string, time.Time, error) {
	args := m.Called(ctx)
	return args.String(0), args.Get(1).(time.Time), args.Error(2)
}

func (m *MockStorage) SetExtendedDescription(ctx context.Context, description string) error {
	args := m.Called(ctx, description)
	return args.Error(0)
}

// ==================== Timeline Write Operations ====================

func (m *MockStorage) WriteToTimeline(ctx context.Context, timeline *storage.TimelineEntry) error {
	args := m.Called(ctx, timeline)
	return args.Error(0)
}

func (m *MockStorage) WriteToTimelines(ctx context.Context, entries []*storage.TimelineEntry) error {
	args := m.Called(ctx, entries)
	return args.Error(0)
}

// MockRepositoryStorage implements the RepositoryStorage interface for testing
type MockRepositoryStorage struct {
	mock.Mock
	accountRepo         *repositories.AccountRepository
	actorRepo           *repositories.ActorRepository
	objectRepo          *repositories.ObjectRepository
	activityRepo        *repositories.ActivityRepository
	timelineRepo        *repositories.TimelineRepository
	notificationRepo    *repositories.NotificationRepository
	likeRepo            *repositories.LikeRepository
	moderationRepo      *repositories.ModerationRepository
	listRepo            *repositories.ListRepository
	mediaRepo           *repositories.MediaRepository
	pollRepo            *repositories.PollRepository
	hashtagRepo         *repositories.HashtagRepository
	scheduledStatusRepo *repositories.ScheduledStatusRepository
	announcementRepo    *repositories.AnnouncementRepository
	domainBlockRepo     *repositories.DomainBlockRepository
	relationshipRepo    *repositories.RelationshipRepository
	instanceRepo        *repositories.InstanceRepository
	federationRepo      *repositories.FederationRepository
	recoveryRepo        *repositories.RecoveryRepository
	conversationRepo    *repositories.ConversationRepository
	pushSubscriptionRepo *repositories.PushSubscriptionRepository
	analyticsRepo       *repositories.TrendingRepository
	socialRepo          *repositories.SocialRepository
	userRepo            *repositories.UserRepository
	statusRepo          *repositories.StatusRepository
	costRepo            *repositories.CostTrackingRepository
	trustRepo           *repositories.TrustRepository
	searchRepo          *repositories.SearchRepository
	relayRepo           *repositories.RelayRepository
	communityNoteRepo   *repositories.CommunityNoteRepository
	emojiRepo           *repositories.EmojiRepository
	rateLimitRepo       *repositories.RateLimitRepository
	markerRepo          *repositories.MarkerRepository
	featuredTagRepo     *repositories.FeaturedTagRepository
	aiRepo              *repositories.AIRepository
	exportRepo          *repositories.ExportRepository
	importRepo          *repositories.ImportRepository
}

// NewMockRepositoryStorage creates a new mock repository storage with mock repositories
func NewMockRepositoryStorage() *MockRepositoryStorage {
	// Create mock repositories that can handle method calls during testing
	// We'll use a mock logger and nil DB for testing since we're not actually using DynamoDB
	logger := zap.NewNop()
	
	// Create repository instances with nil DB - they'll return "not implemented" errors
	// which is what we expect during testing phase
	accountRepo := repositories.NewAccountRepository(nil, "test-table", "test.example.com", logger)
	actorRepo := repositories.NewActorRepository(nil, "test-table", logger)
	objectRepo := repositories.NewObjectRepository(nil, "test-table", "test.example.com", logger)
	activityRepo := repositories.NewActivityRepository(nil, "test-table", logger)
	timelineRepo := repositories.NewTimelineRepository(nil, "test-table", logger)
	notificationRepo := repositories.NewNotificationRepository(nil, "test-table", logger)
	likeRepo := repositories.NewLikeRepository(nil, "test-table", logger)
	moderationRepo := repositories.NewModerationRepository(nil, "test-table", logger)
	listRepo := repositories.NewListRepository(nil, "test-table", logger)
	mediaRepo := repositories.NewMediaRepository(nil, "test-table", logger)
	pollRepo := repositories.NewPollRepository(nil, "test-table", logger)
	hashtagRepo := repositories.NewHashtagRepository(nil, "test-table", logger, "test.example.com")
	scheduledStatusRepo := repositories.NewScheduledStatusRepository(nil, "test-table", logger)
	announcementRepo := repositories.NewAnnouncementRepository(nil, "test-table", logger)
	domainBlockRepo := repositories.NewDomainBlockRepository(nil, "test-table", logger)
	relationshipRepo := repositories.NewRelationshipRepository(nil, "test-table", logger)
	instanceRepo := repositories.NewInstanceRepository(nil, "test-table", logger)
	federationRepo := repositories.NewFederationRepository(nil, logger)
	recoveryRepo := repositories.NewRecoveryRepository(nil, "test-table", logger)
	conversationRepo := repositories.NewConversationRepository(nil, logger)
	pushSubscriptionRepo := repositories.NewPushSubscriptionRepository(nil, "test-table", logger)
	analyticsRepo := repositories.NewTrendingRepository(nil, logger)
	socialRepo := repositories.NewSocialRepository(nil, logger)
	userRepo := repositories.NewUserRepository(nil, "test-table", logger)
	statusRepo := repositories.NewStatusRepository(nil, "test-table", logger)
	costRepo := repositories.NewCostTrackingRepository(nil, "test-table", logger)
	trustRepo := repositories.NewTrustRepository(nil, logger)
	searchRepo := repositories.NewSearchRepository(&dynamorm.DB{}, logger)
	relayRepo := repositories.NewRelayRepository(nil, "test-table", logger)
	markerRepo := repositories.NewMarkerRepository(nil, "test-table", logger)
	featuredTagRepo := repositories.NewFeaturedTagRepository(nil, "test-table", logger)
	aiRepo := repositories.NewAIRepository(nil, "test-table", logger)
	exportRepo := repositories.NewExportRepository(nil, "test-table", logger)
	importRepo := repositories.NewImportRepository(nil, "test-table", logger)
	
	return &MockRepositoryStorage{
		accountRepo:         accountRepo,
		actorRepo:           actorRepo,
		objectRepo:          objectRepo,
		activityRepo:        activityRepo,
		timelineRepo:        timelineRepo,
		notificationRepo:    notificationRepo,
		likeRepo:            likeRepo,
		moderationRepo:      moderationRepo,
		listRepo:            listRepo,
		mediaRepo:           mediaRepo,
		pollRepo:            pollRepo,
		hashtagRepo:         hashtagRepo,
		scheduledStatusRepo: scheduledStatusRepo,
		announcementRepo:    announcementRepo,
		domainBlockRepo:     domainBlockRepo,
		relationshipRepo:    relationshipRepo,
		instanceRepo:        instanceRepo,
		federationRepo:      federationRepo,
		recoveryRepo:        recoveryRepo,
		conversationRepo:    conversationRepo,
		pushSubscriptionRepo: pushSubscriptionRepo,
		analyticsRepo:       analyticsRepo,
		socialRepo:          socialRepo,
		userRepo:            userRepo,
		statusRepo:          statusRepo,
		costRepo:            costRepo,
		trustRepo:           trustRepo,
		searchRepo:          searchRepo,
		relayRepo:           relayRepo,
		communityNoteRepo:   repositories.NewCommunityNoteRepository(&dynamorm.DB{}, "test-table", logger),
		markerRepo:          markerRepo,
		featuredTagRepo:     featuredTagRepo,
		aiRepo:              aiRepo,
		exportRepo:          exportRepo,
		importRepo:          importRepo,
	}
}

// Repository access methods for MockRepositoryStorage
func (m *MockRepositoryStorage) Account() *repositories.AccountRepository {
	return m.accountRepo
}

func (m *MockRepositoryStorage) Actor() *repositories.ActorRepository {
	return m.actorRepo
}

func (m *MockRepositoryStorage) Object() *repositories.ObjectRepository {
	return m.objectRepo
}

func (m *MockRepositoryStorage) Activity() *repositories.ActivityRepository {
	return m.activityRepo
}

func (m *MockRepositoryStorage) Timeline() *repositories.TimelineRepository {
	return m.timelineRepo
}

func (m *MockRepositoryStorage) Notification() *repositories.NotificationRepository {
	return m.notificationRepo
}

func (m *MockRepositoryStorage) Like() *repositories.LikeRepository {
	return m.likeRepo
}

func (m *MockRepositoryStorage) Moderation() *repositories.ModerationRepository {
	return m.moderationRepo
}

func (m *MockRepositoryStorage) List() *repositories.ListRepository {
	return m.listRepo
}

func (m *MockRepositoryStorage) Media() *repositories.MediaRepository {
	return m.mediaRepo
}

func (m *MockRepositoryStorage) Poll() *repositories.PollRepository {
	return m.pollRepo
}

func (m *MockRepositoryStorage) Hashtag() *repositories.HashtagRepository {
	return m.hashtagRepo
}

func (m *MockRepositoryStorage) ScheduledStatus() *repositories.ScheduledStatusRepository {
	return m.scheduledStatusRepo
}

func (m *MockRepositoryStorage) Announcement() *repositories.AnnouncementRepository {
	return m.announcementRepo
}

func (m *MockRepositoryStorage) DomainBlock() *repositories.DomainBlockRepository {
	return m.domainBlockRepo
}

func (m *MockRepositoryStorage) Relationship() *repositories.RelationshipRepository {
	return m.relationshipRepo
}

func (m *MockRepositoryStorage) Instance() *repositories.InstanceRepository {
	return m.instanceRepo
}

func (m *MockRepositoryStorage) Federation() *repositories.FederationRepository {
	return m.federationRepo
}

func (m *MockRepositoryStorage) Recovery() *repositories.RecoveryRepository {
	return m.recoveryRepo
}

func (m *MockRepositoryStorage) Conversation() *repositories.ConversationRepository {
	return m.conversationRepo
}

func (m *MockRepositoryStorage) PushSubscription() *repositories.PushSubscriptionRepository {
	return m.pushSubscriptionRepo
}

func (m *MockRepositoryStorage) Analytics() *repositories.TrendingRepository {
	return m.analyticsRepo
}

func (m *MockRepositoryStorage) Social() *repositories.SocialRepository {
	return m.socialRepo
}

func (m *MockRepositoryStorage) User() *repositories.UserRepository {
	return m.userRepo
}

func (m *MockRepositoryStorage) Status() *repositories.StatusRepository {
	return m.statusRepo
}

func (m *MockRepositoryStorage) Cost() *repositories.CostTrackingRepository {
	return m.costRepo
}

func (m *MockRepositoryStorage) Trust() *repositories.TrustRepository {
	return m.trustRepo
}

func (m *MockRepositoryStorage) Search() *repositories.SearchRepository {
	return m.searchRepo
}

func (m *MockRepositoryStorage) Relay() *repositories.RelayRepository {
	return m.relayRepo
}

func (m *MockRepositoryStorage) CommunityNote() *repositories.CommunityNoteRepository {
	return m.communityNoteRepo
}

func (m *MockRepositoryStorage) Emoji() *repositories.EmojiRepository {
	return m.emojiRepo
}

func (m *MockRepositoryStorage) RateLimit() *repositories.RateLimitRepository {
	return m.rateLimitRepo
}

func (m *MockRepositoryStorage) Marker() *repositories.MarkerRepository {
	return m.markerRepo
}

func (m *MockRepositoryStorage) FeaturedTag() *repositories.FeaturedTagRepository {
	return m.featuredTagRepo
}

func (m *MockRepositoryStorage) AI() *repositories.AIRepository {
	return m.aiRepo
}

func (m *MockRepositoryStorage) Export() *repositories.ExportRepository {
	return m.exportRepo
}

func (m *MockRepositoryStorage) Import() *repositories.ImportRepository {
	return m.importRepo
}

// Utility methods
func (m *MockRepositoryStorage) GetDB() dynamormCore.DB {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(dynamormCore.DB)
}

func (m *MockRepositoryStorage) GetTableName() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockRepositoryStorage) GetLogger() *zap.Logger {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*zap.Logger)
}

// Ensure MockRepositoryStorage implements RepositoryStorage interface
var _ core.RepositoryStorage = (*MockRepositoryStorage)(nil)

