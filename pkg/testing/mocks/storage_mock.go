package mocks

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockStorage is a mock implementation of the Storage interface
type MockStorage struct {
	mock.Mock

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

// CreateActor mocks the CreateActor method
func (m *MockStorage) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	args := m.Called(ctx, actor, privateKey)
	return args.Error(0)
}

// GetActor mocks the GetActor method
func (m *MockStorage) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

// GetActorByNumericID mocks the GetActorByNumericID method
func (m *MockStorage) GetActorByNumericID(ctx context.Context, numericID string) (*activitypub.Actor, error) {
	args := m.Called(ctx, numericID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

// GetActorWithMetadata mocks the GetActorWithMetadata method
func (m *MockStorage) GetActorWithMetadata(ctx context.Context, username string) (*activitypub.Actor, *storage.ActorMetadata, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).(*activitypub.Actor), args.Get(1).(*storage.ActorMetadata), args.Error(2)
}

// GetActorPrivateKey mocks the GetActorPrivateKey method
func (m *MockStorage) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	args := m.Called(ctx, username)
	return args.String(0), args.Error(1)
}

// UpdateActor mocks the UpdateActor method
func (m *MockStorage) UpdateActor(ctx context.Context, actor *activitypub.Actor) error {
	args := m.Called(ctx, actor)
	return args.Error(0)
}

// UpdateActorLastStatusTime mocks the UpdateActorLastStatusTime method
func (m *MockStorage) UpdateActorLastStatusTime(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// SetActorFields mocks the SetActorFields method
func (m *MockStorage) SetActorFields(ctx context.Context, username string, fields []storage.ActorField) error {
	args := m.Called(ctx, username, fields)
	return args.Error(0)
}

// DeleteActor mocks the DeleteActor method
func (m *MockStorage) DeleteActor(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// SearchAccounts mocks the SearchAccounts method
func (m *MockStorage) SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error) {
	args := m.Called(ctx, query, limit, followingOnly, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*activitypub.Actor), args.Error(1)
}

// GetSearchSuggestions mocks the GetSearchSuggestions method
func (m *MockStorage) GetSearchSuggestions(ctx context.Context, prefix string) ([]storage.SearchSuggestion, error) {
	args := m.Called(ctx, prefix)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.SearchSuggestion), args.Error(1)
}

// SearchStatuses mocks the SearchStatuses method
func (m *MockStorage) SearchStatuses(ctx context.Context, query string, limit int) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, query, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// SearchStatusesWithOptions mocks the SearchStatusesWithOptions method
func (m *MockStorage) SearchStatusesWithOptions(ctx context.Context, query string, options storage.StatusSearchOptions) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, query, options)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// SearchStatusesByURL mocks the SearchStatusesByURL method
func (m *MockStorage) SearchStatusesByURL(ctx context.Context, url string) (*storage.StatusSearchResult, error) {
	args := m.Called(ctx, url)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StatusSearchResult), args.Error(1)
}

// SearchAll mocks the SearchAll method
func (m *MockStorage) SearchAll(ctx context.Context, query string, limit int, accountID string) (*storage.SearchResults, error) {
	args := m.Called(ctx, query, limit, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SearchResults), args.Error(1)
}

// SearchAccountsAdvanced mocks the SearchAccountsAdvanced method
func (m *MockStorage) SearchAccountsAdvanced(ctx context.Context, query string, resolve bool, limit int, offset int, following bool, accountID string) ([]*activitypub.Actor, error) {
	args := m.Called(ctx, query, resolve, limit, offset, following, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*activitypub.Actor), args.Error(1)
}

// SearchStatusesAdvanced mocks the SearchStatusesAdvanced method
func (m *MockStorage) SearchStatusesAdvanced(ctx context.Context, query string, limit int, maxID, minID *string, accountID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, query, limit, maxID, minID, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// SearchHashtags mocks the SearchHashtags method
func (m *MockStorage) SearchHashtags(ctx context.Context, query string, limit int) ([]*storage.Hashtag, error) {
	args := m.Called(ctx, query, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Hashtag), args.Error(1)
}

// SearchHashtagsAdvanced mocks the SearchHashtagsAdvanced method
func (m *MockStorage) SearchHashtagsAdvanced(ctx context.Context, query string, limit int, accountID string) ([]*storage.HashtagSearchResult, error) {
	args := m.Called(ctx, query, limit, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.HashtagSearchResult), args.Error(1)
}

// CreateActivity mocks the CreateActivity method
func (m *MockStorage) CreateActivity(ctx context.Context, activity *activitypub.Activity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

// GetActivity mocks the GetActivity method
func (m *MockStorage) GetActivity(ctx context.Context, id string) (*activitypub.Activity, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Activity), args.Error(1)
}

// GetOutboxActivities mocks the GetOutboxActivities method
func (m *MockStorage) GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*activitypub.Activity), args.String(1), args.Error(2)
}

// GetInboxActivities mocks the GetInboxActivities method
func (m *MockStorage) GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*activitypub.Activity), args.String(1), args.Error(2)
}

// CreateObject mocks the CreateObject method
func (m *MockStorage) CreateObject(ctx context.Context, object any) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

// GetObject mocks the GetObject method
func (m *MockStorage) GetObject(ctx context.Context, id string) (any, error) {
	args := m.Called(ctx, id)
	return args.Get(0), args.Error(1)
}

// UpdateObject mocks the UpdateObject method
func (m *MockStorage) UpdateObject(ctx context.Context, object any) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

// DeleteObject mocks the DeleteObject method
func (m *MockStorage) DeleteObject(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// GetObjectsByActor mocks the GetObjectsByActor method
func (m *MockStorage) GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]any, string, error) {
	args := m.Called(ctx, actorID, cursor, limit)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]any), args.String(1), args.Error(2)
}

// MockStoragePreset represents a preset configuration for mock storage
type MockStoragePreset string

const (
	// PresetEmpty represents an empty preset with no data
	PresetEmpty MockStoragePreset = "empty"
	// PresetWithTestUser represents a preset with test user data
	PresetWithTestUser MockStoragePreset = "test_user"
	// PresetWithMultiUsers represents a preset with multiple test users
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

// CreateFollow mocks the CreateFollow method
func (m *MockStorage) CreateFollow(ctx context.Context, followerUsername, followedUsername, followActivityID string) error {
	args := m.Called(ctx, followerUsername, followedUsername, followActivityID)
	return args.Error(0)
}

// AcceptFollow mocks the AcceptFollow method
func (m *MockStorage) AcceptFollow(ctx context.Context, followerUsername, followedUsername string) error {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Error(0)
}

// RejectFollow mocks the RejectFollow method
func (m *MockStorage) RejectFollow(ctx context.Context, followerUsername, followedUsername string) error {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Error(0)
}

// RemoveFollow mocks the RemoveFollow method
func (m *MockStorage) RemoveFollow(ctx context.Context, followerUsername, followedUsername string) error {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Error(0)
}

// RemoveFromFollowers mocks the RemoveFromFollowers method
func (m *MockStorage) RemoveFromFollowers(ctx context.Context, username, followerUsername string) error {
	args := m.Called(ctx, username, followerUsername)
	return args.Error(0)
}

// GetFollowRequest mocks the GetFollowRequest method
func (m *MockStorage) GetFollowRequest(ctx context.Context, followerID, targetID string) (*storage.RelationshipRecord, error) {
	args := m.Called(ctx, followerID, targetID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.RelationshipRecord), args.Error(1)
}

// AcceptFollowRequest mocks the AcceptFollowRequest method
func (m *MockStorage) AcceptFollowRequest(ctx context.Context, followerID, targetID string) error {
	args := m.Called(ctx, followerID, targetID)
	return args.Error(0)
}

// RejectFollowRequest mocks the RejectFollowRequest method
func (m *MockStorage) RejectFollowRequest(ctx context.Context, followerID, targetID string) error {
	args := m.Called(ctx, followerID, targetID)
	return args.Error(0)
}

// HasFollowRequest mocks the HasFollowRequest method
func (m *MockStorage) HasFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error) {
	args := m.Called(ctx, requesterID, targetID)
	return args.Bool(0), args.Error(1)
}

// GetOAuthClient mocks the GetOAuthClient method
func (m *MockStorage) GetOAuthClient(ctx context.Context, clientID string) (*storage.OAuthClient, error) {
	args := m.Called(ctx, clientID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.OAuthClient), args.Error(1)
}

// CreateOAuthClient mocks the CreateOAuthClient method
func (m *MockStorage) CreateOAuthClient(ctx context.Context, client *storage.OAuthClient) error {
	args := m.Called(ctx, client)
	return args.Error(0)
}

// UpdateOAuthClient mocks the UpdateOAuthClient method
func (m *MockStorage) UpdateOAuthClient(ctx context.Context, clientID string, updates map[string]any) error {
	args := m.Called(ctx, clientID, updates)
	return args.Error(0)
}

// DeleteOAuthClient mocks the DeleteOAuthClient method
func (m *MockStorage) DeleteOAuthClient(ctx context.Context, clientID string) error {
	args := m.Called(ctx, clientID)
	return args.Error(0)
}

// ListOAuthClients mocks the ListOAuthClients method
func (m *MockStorage) ListOAuthClients(ctx context.Context, limit int32, cursor string) ([]*storage.OAuthClient, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.OAuthClient), args.String(1), args.Error(2)
}

// StoreOAuthState mocks the StoreOAuthState method
func (m *MockStorage) StoreOAuthState(ctx context.Context, state string, data *storage.OAuthState) error {
	args := m.Called(ctx, state, data)
	return args.Error(0)
}

// SaveOAuthState mocks the SaveOAuthState method
func (m *MockStorage) SaveOAuthState(ctx context.Context, state *storage.OAuthState) error {
	args := m.Called(ctx, state)
	return args.Error(0)
}

// GetOAuthApp mocks the GetOAuthApp method
func (m *MockStorage) GetOAuthApp(ctx context.Context, clientID string) (*storage.OAuthApp, error) {
	args := m.Called(ctx, clientID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.OAuthApp), args.Error(1)
}

// SaveUserAppConsent mocks the SaveUserAppConsent method
func (m *MockStorage) SaveUserAppConsent(ctx context.Context, consent *storage.UserAppConsent) error {
	args := m.Called(ctx, consent)
	return args.Error(0)
}

// GetOAuthState mocks the GetOAuthState method
func (m *MockStorage) GetOAuthState(ctx context.Context, state string) (*storage.OAuthState, error) {
	args := m.Called(ctx, state)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.OAuthState), args.Error(1)
}

// DeleteOAuthState mocks the DeleteOAuthState method
func (m *MockStorage) DeleteOAuthState(ctx context.Context, state string) error {
	args := m.Called(ctx, state)
	return args.Error(0)
}

// CountObjectReplies mocks the CountObjectReplies method
func (m *MockStorage) CountObjectReplies(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// CreateUpdateHistory mocks the CreateUpdateHistory method
func (m *MockStorage) CreateUpdateHistory(ctx context.Context, history *storage.UpdateHistory) error {
	args := m.Called(ctx, history)
	return args.Error(0)
}

// GetUpdateHistory mocks the GetUpdateHistory method
func (m *MockStorage) GetUpdateHistory(ctx context.Context, objectID string, limit int) ([]*storage.UpdateHistory, error) {
	args := m.Called(ctx, objectID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.UpdateHistory), args.Error(1)
}

// GetFollowers mocks the GetFollowers method
func (m *MockStorage) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

// GetFollowing mocks the GetFollowing method
func (m *MockStorage) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

// IsFollowing mocks the IsFollowing method
func (m *MockStorage) IsFollowing(ctx context.Context, followerUsername, followedUsername string) (bool, error) {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Bool(0), args.Error(1)
}

// GetPendingFollowRequests mocks the GetPendingFollowRequests method
func (m *MockStorage) GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

// GetFollowRequestState mocks the GetFollowRequestState method
func (m *MockStorage) GetFollowRequestState(ctx context.Context, followerUsername, followedUsername string) (string, error) {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.String(0), args.Error(1)
}

// GetCollection mocks the GetCollection method
func (m *MockStorage) GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error) {
	args := m.Called(ctx, username, collectionType, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.OrderedCollectionPage), args.Error(1)
}

// CreateAuthorizationCode mocks the CreateAuthorizationCode method
func (m *MockStorage) CreateAuthorizationCode(ctx context.Context, code *storage.AuthorizationCode) error {
	args := m.Called(ctx, code)
	return args.Error(0)
}

// GetAuthorizationCode mocks the GetAuthorizationCode method
func (m *MockStorage) GetAuthorizationCode(ctx context.Context, code string) (*storage.AuthorizationCode, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.AuthorizationCode), args.Error(1)
}

// DeleteAuthorizationCode mocks the DeleteAuthorizationCode method
func (m *MockStorage) DeleteAuthorizationCode(ctx context.Context, code string) error {
	args := m.Called(ctx, code)
	return args.Error(0)
}

// CreateRefreshToken mocks the CreateRefreshToken method
func (m *MockStorage) CreateRefreshToken(ctx context.Context, token *storage.RefreshToken) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

// GetRefreshToken mocks the GetRefreshToken method
func (m *MockStorage) GetRefreshToken(ctx context.Context, token string) (*storage.RefreshToken, error) {
	args := m.Called(ctx, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.RefreshToken), args.Error(1)
}

// DeleteRefreshToken mocks the DeleteRefreshToken method
func (m *MockStorage) DeleteRefreshToken(ctx context.Context, token string) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

// CreateUser mocks the CreateUser method
func (m *MockStorage) CreateUser(ctx context.Context, user *storage.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

// GetUser mocks the GetUser method
func (m *MockStorage) GetUser(ctx context.Context, username string) (*storage.User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

// GetUserByEmail mocks the GetUserByEmail method
func (m *MockStorage) GetUserByEmail(ctx context.Context, email string) (*storage.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

// UpdateUser mocks the UpdateUser method
func (m *MockStorage) UpdateUser(ctx context.Context, username string, updates map[string]any) error {
	args := m.Called(ctx, username, updates)
	return args.Error(0)
}

// DeleteUser mocks the DeleteUser method
func (m *MockStorage) DeleteUser(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// ListUsers mocks the ListUsers method
func (m *MockStorage) ListUsers(ctx context.Context, limit int32, cursor string) ([]*storage.User, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.User), args.String(1), args.Error(2)
}

// GetActiveUserCount mocks the GetActiveUserCount method
func (m *MockStorage) GetActiveUserCount(ctx context.Context, days int) (int64, error) {
	args := m.Called(ctx, days)
	return args.Get(0).(int64), args.Error(1)
}

// GetTotalUserCount mocks the GetTotalUserCount method
func (m *MockStorage) GetTotalUserCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

// GetTotalStatusCount mocks the GetTotalStatusCount method
func (m *MockStorage) GetTotalStatusCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

// GetTotalDomainCount mocks the GetTotalDomainCount method
func (m *MockStorage) GetTotalDomainCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

// GetWeeklyActivity mocks the GetWeeklyActivity method
func (m *MockStorage) GetWeeklyActivity(ctx context.Context, weekTimestamp int64) (*storage.WeeklyActivity, error) {
	args := m.Called(ctx, weekTimestamp)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.WeeklyActivity), args.Error(1)
}

// RecordActivity mocks the RecordActivity method
func (m *MockStorage) RecordActivity(ctx context.Context, activityType string, actorID string, timestamp time.Time) error {
	args := m.Called(ctx, activityType, actorID, timestamp)
	return args.Error(0)
}

// GetContactAccount mocks the GetContactAccount method
func (m *MockStorage) GetContactAccount(ctx context.Context) (*storage.ActorRecord, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ActorRecord), args.Error(1)
}

// GetUserByProviderID mocks the GetUserByProviderID method
func (m *MockStorage) GetUserByProviderID(ctx context.Context, provider, providerID string) (*storage.User, error) {
	args := m.Called(ctx, provider, providerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

// LinkProviderAccount mocks the LinkProviderAccount method
func (m *MockStorage) LinkProviderAccount(ctx context.Context, username, provider, providerID string) error {
	args := m.Called(ctx, username, provider, providerID)
	return args.Error(0)
}

// UnlinkProviderAccount mocks the UnlinkProviderAccount method
func (m *MockStorage) UnlinkProviderAccount(ctx context.Context, username, provider string) error {
	args := m.Called(ctx, username, provider)
	return args.Error(0)
}

// GetLinkedProviders mocks the GetLinkedProviders method
func (m *MockStorage) GetLinkedProviders(ctx context.Context, username string) ([]string, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// StoreRecoveryToken mocks the StoreRecoveryToken method
func (m *MockStorage) StoreRecoveryToken(ctx context.Context, key string, data map[string]any) error {
	args := m.Called(ctx, key, data)
	return args.Error(0)
}

// GetRecoveryToken mocks the GetRecoveryToken method
func (m *MockStorage) GetRecoveryToken(ctx context.Context, key string) (map[string]any, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]any), args.Error(1)
}

// DeleteRecoveryToken mocks the DeleteRecoveryToken method
func (m *MockStorage) DeleteRecoveryToken(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

// CreateLike mocks the CreateLike method
func (m *MockStorage) CreateLike(ctx context.Context, like *storage.Like) error {
	args := m.Called(ctx, like)
	return args.Error(0)
}

// GetLike mocks the GetLike method
func (m *MockStorage) GetLike(ctx context.Context, actor, object string) (*storage.Like, error) {
	args := m.Called(ctx, actor, object)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Like), args.Error(1)
}

// DeleteLike mocks the DeleteLike method
func (m *MockStorage) DeleteLike(ctx context.Context, actor, object string) error {
	args := m.Called(ctx, actor, object)
	return args.Error(0)
}

// GetLikeCount mocks the GetLikeCount method
func (m *MockStorage) GetLikeCount(ctx context.Context, statusID string) (int64, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0).(int64), args.Error(1)
}

// GetLocalPostCount mocks the GetLocalPostCount method
func (m *MockStorage) GetLocalPostCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

// GetManifestGenerationStats mocks the GetManifestGenerationStats method
func (m *MockStorage) GetManifestGenerationStats(ctx context.Context, format, startDate, endDate string) (map[string]int64, error) {
	args := m.Called(ctx, format, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int64), args.Error(1)
}

// SaveMarker mocks the SaveMarker method
func (m *MockStorage) SaveMarker(ctx context.Context, username, timeline string, lastReadID string, version int) error {
	args := m.Called(ctx, username, timeline, lastReadID, version)
	return args.Error(0)
}

// GetMarkers mocks the GetMarkers method
func (m *MockStorage) GetMarkers(ctx context.Context, username string, timelines []string) (map[string]*storage.Marker, error) {
	args := m.Called(ctx, username, timelines)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]*storage.Marker), args.Error(1)
}

// GetMediaEventStats mocks the GetMediaEventStats method
func (m *MockStorage) GetMediaEventStats(ctx context.Context, eventType, startDate, endDate string) (map[string]int64, error) {
	args := m.Called(ctx, eventType, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int64), args.Error(1)
}

// GetMultiHashtagTimeline mocks the GetMultiHashtagTimeline method
func (m *MockStorage) GetMultiHashtagTimeline(ctx context.Context, hashtags []string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, hashtags, maxID, limit, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// TrackSearchQuery mocks the TrackSearchQuery method
func (m *MockStorage) TrackSearchQuery(ctx context.Context, userID, query string, resultCount int) error {
	args := m.Called(ctx, userID, query, resultCount)
	return args.Error(0)
}

// GetPopularSearchQueries mocks the GetPopularSearchQueries method
func (m *MockStorage) GetPopularSearchQueries(ctx context.Context, limit int, timeWindow time.Duration) ([]storage.SearchQueryStats, error) {
	args := m.Called(ctx, limit, timeWindow)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.SearchQueryStats), args.Error(1)
}

// GetPublicTimeline mocks the GetPublicTimeline method
func (m *MockStorage) GetPublicTimeline(ctx context.Context, local bool, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, local, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TimelineEntry), args.String(1), args.Error(2)
}

// GetRecentHashtags mocks the GetRecentHashtags method
func (m *MockStorage) GetRecentHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingHashtag), args.Error(1)
}

// GetRecentLinks mocks the GetRecentLinks method
func (m *MockStorage) GetRecentLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingLink), args.Error(1)
}

// GetRelationshipNote mocks the GetRelationshipNote method
func (m *MockStorage) GetRelationshipNote(ctx context.Context, userID, targetID string) (*storage.AccountNote, error) {
	args := m.Called(ctx, userID, targetID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.AccountNote), args.Error(1)
}

// StoreReputation mocks the StoreReputation method
func (m *MockStorage) StoreReputation(ctx context.Context, actorID string, reputation *storage.Reputation) error {
	args := m.Called(ctx, actorID, reputation)
	return args.Error(0)
}

// GetReputation mocks the GetReputation method
func (m *MockStorage) GetReputation(ctx context.Context, actorID string) (*storage.Reputation, error) {
	args := m.Called(ctx, actorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Reputation), args.Error(1)
}

// GetReputationHistory mocks the GetReputationHistory method
func (m *MockStorage) GetReputationHistory(ctx context.Context, actorID string, limit int) ([]*storage.Reputation, error) {
	args := m.Called(ctx, actorID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Reputation), args.Error(1)
}

// GetRulesByCategory mocks the GetRulesByCategory method
func (m *MockStorage) GetRulesByCategory(ctx context.Context, category string) ([]storage.InstanceRule, error) {
	args := m.Called(ctx, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.InstanceRule), args.Error(1)
}

// GetSeveranceHistory mocks the GetSeveranceHistory method
func (m *MockStorage) GetSeveranceHistory(ctx context.Context, localInstance, remoteInstance string, limit int) ([]*storage.SeveredRelationship, error) {
	args := m.Called(ctx, localInstance, remoteInstance, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.SeveredRelationship), args.Error(1)
}

// GetStatus mocks the GetStatus method
func (m *MockStorage) GetStatus(ctx context.Context, statusID string) (any, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0), args.Error(1)
}

// GetStatusCount mocks the GetStatusCount method
func (m *MockStorage) GetStatusCount(ctx context.Context, actorID string) (int, error) {
	args := m.Called(ctx, actorID)
	return args.Int(0), args.Error(1)
}

// GetStatusesByLink mocks the GetStatusesByLink method
func (m *MockStorage) GetStatusesByLink(ctx context.Context, linkURL string, limit int) ([]any, error) {
	args := m.Called(ctx, linkURL, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

// GetStorageHistory mocks the GetStorageHistory method
func (m *MockStorage) GetStorageHistory(ctx context.Context, days int) ([]any, error) {
	args := m.Called(ctx, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

// GetStorageUsage mocks the GetStorageUsage method
func (m *MockStorage) GetStorageUsage(ctx context.Context) (any, error) {
	args := m.Called(ctx)
	return args.Get(0), args.Error(1)
}

// GetStrongestConnectionsByType mocks the GetStrongestConnectionsByType method
func (m *MockStorage) GetStrongestConnectionsByType(ctx context.Context, connectionType string, limit int) ([]*storage.FederationEdge, error) {
	args := m.Called(ctx, connectionType, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FederationEdge), args.Error(1)
}

// GetSuggestedHashtags mocks the GetSuggestedHashtags method
func (m *MockStorage) GetSuggestedHashtags(ctx context.Context, userID string, limit int) ([]*storage.HashtagSearchResult, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.HashtagSearchResult), args.Error(1)
}

// GetTagSuggestions mocks the GetTagSuggestions method
func (m *MockStorage) GetTagSuggestions(ctx context.Context, userID string, limit int) ([]string, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// GetThreadContext mocks the GetThreadContext method
func (m *MockStorage) GetThreadContext(ctx context.Context, statusID string) (*storage.ThreadContext, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ThreadContext), args.Error(1)
}

// GetTrendingHashtags mocks the GetTrendingHashtags method
func (m *MockStorage) GetTrendingHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingHashtag), args.Error(1)
}

// GetTrendingLinks mocks the GetTrendingLinks method
func (m *MockStorage) GetTrendingLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingLink), args.Error(1)
}

// GetTrendingStatuses mocks the GetTrendingStatuses method
func (m *MockStorage) GetTrendingStatuses(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingStatus), args.Error(1)
}

// GetUserAppConsent mocks the GetUserAppConsent method
func (m *MockStorage) GetUserAppConsent(ctx context.Context, userID, appID string) (*storage.UserAppConsent, error) {
	args := m.Called(ctx, userID, appID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.UserAppConsent), args.Error(1)
}

// GetUserGrowthHistory mocks the GetUserGrowthHistory method
func (m *MockStorage) GetUserGrowthHistory(ctx context.Context, days int) ([]any, error) {
	args := m.Called(ctx, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

// UnmarkAllMediaAsSensitive mocks the UnmarkAllMediaAsSensitive method
func (m *MockStorage) UnmarkAllMediaAsSensitive(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// GetUserMedia mocks the GetUserMedia method
func (m *MockStorage) GetUserMedia(ctx context.Context, username string) ([]any, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

// UpdateMediaAttachment mocks the UpdateMediaAttachment method
func (m *MockStorage) UpdateMediaAttachment(ctx context.Context, mediaID string, updates map[string]any) error {
	args := m.Called(ctx, mediaID, updates)
	return args.Error(0)
}

// GetUserSearchHistory mocks the GetUserSearchHistory method
func (m *MockStorage) GetUserSearchHistory(ctx context.Context, userID string, limit int) ([]storage.SearchHistoryEntry, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.SearchHistoryEntry), args.Error(1)
}

// GetUserStatusCount mocks the GetUserStatusCount method
func (m *MockStorage) GetUserStatusCount(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

// GetVAPIDKeys mocks the GetVAPIDKeys method
func (m *MockStorage) GetVAPIDKeys(ctx context.Context) (*storage.VAPIDKeys, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.VAPIDKeys), args.Error(1)
}

// SetVAPIDKeys mocks the SetVAPIDKeys method
func (m *MockStorage) SetVAPIDKeys(ctx context.Context, keys *storage.VAPIDKeys) error {
	args := m.Called(ctx, keys)
	return args.Error(0)
}

// HasPendingFollowRequest mocks the HasPendingFollowRequest method
func (m *MockStorage) HasPendingFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error) {
	args := m.Called(ctx, requesterID, targetID)
	return args.Bool(0), args.Error(1)
}

// HasUserVoted mocks the HasUserVoted method
func (m *MockStorage) HasUserVoted(ctx context.Context, pollID string, userID string) (bool, []int, error) {
	args := m.Called(ctx, pollID, userID)
	if args.Get(1) == nil {
		return args.Bool(0), nil, args.Error(2)
	}
	return args.Bool(0), args.Get(1).([]int), args.Error(2)
}

// IncrementReblogCount mocks the IncrementReblogCount method
func (m *MockStorage) IncrementReblogCount(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// IndexHashtag mocks the IndexHashtag method
func (m *MockStorage) IndexHashtag(ctx context.Context, hashtag string, statusID string, authorID string, visibility string) error {
	args := m.Called(ctx, hashtag, statusID, authorID, visibility)
	return args.Error(0)
}

// IsBlockedDomain mocks the IsBlockedDomain method
func (m *MockStorage) IsBlockedDomain(ctx context.Context, username, domain string) (bool, error) {
	args := m.Called(ctx, username, domain)
	return args.Bool(0), args.Error(1)
}

// IsDismissed mocks the IsDismissed method
func (m *MockStorage) IsDismissed(ctx context.Context, username, announcementID string) (bool, error) {
	args := m.Called(ctx, username, announcementID)
	return args.Bool(0), args.Error(1)
}

// IsEndorsed mocks the IsEndorsed method
func (m *MockStorage) IsEndorsed(ctx context.Context, userID, targetID string) (bool, error) {
	args := m.Called(ctx, userID, targetID)
	return args.Bool(0), args.Error(1)
}

// IsNotificationEnabled mocks the IsNotificationEnabled method
func (m *MockStorage) IsNotificationEnabled(ctx context.Context, userID, targetID string) (bool, error) {
	args := m.Called(ctx, userID, targetID)
	return args.Bool(0), args.Error(1)
}

// MarkThreadAsSynced mocks the MarkThreadAsSynced method
func (m *MockStorage) MarkThreadAsSynced(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

// RecordHashtagUsage mocks the RecordHashtagUsage method
func (m *MockStorage) RecordHashtagUsage(ctx context.Context, hashtag string, statusID string, authorID string) error {
	args := m.Called(ctx, hashtag, statusID, authorID)
	return args.Error(0)
}

// RecordLinkShare mocks the RecordLinkShare method
func (m *MockStorage) RecordLinkShare(ctx context.Context, url string, statusID string, authorID string) error {
	args := m.Called(ctx, url, statusID, authorID)
	return args.Error(0)
}

// RecordManifestGeneration mocks the RecordManifestGeneration method
func (m *MockStorage) RecordManifestGeneration(ctx context.Context, mediaID, format string, duration float64) error {
	args := m.Called(ctx, mediaID, format, duration)
	return args.Error(0)
}

// RecordQualityChange mocks the RecordQualityChange method
func (m *MockStorage) RecordQualityChange(ctx context.Context, mediaID, userID, oldQuality, newQuality string) error {
	args := m.Called(ctx, mediaID, userID, oldQuality, newQuality)
	return args.Error(0)
}

// RecordMediaEvent mocks the RecordMediaEvent method
func (m *MockStorage) RecordMediaEvent(ctx context.Context, eventType, mediaID, userID string) error {
	args := m.Called(ctx, eventType, mediaID, userID)
	return args.Error(0)
}

// RecordPatternMatch mocks the RecordPatternMatch method
func (m *MockStorage) RecordPatternMatch(ctx context.Context, patternID string, matched bool, timestamp time.Time) error {
	args := m.Called(ctx, patternID, matched, timestamp)
	return args.Error(0)
}

// GetObjectLikes mocks the GetObjectLikes method
func (m *MockStorage) GetObjectLikes(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Like, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Like), args.String(1), args.Error(2)
}

// GetActorLikes mocks the GetActorLikes method
func (m *MockStorage) GetActorLikes(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Like, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Like), args.String(1), args.Error(2)
}

// CountObjectLikes mocks the CountObjectLikes method
func (m *MockStorage) CountObjectLikes(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// CreateAnnounce mocks the CreateAnnounce method
func (m *MockStorage) CreateAnnounce(ctx context.Context, announce *storage.Announce) error {
	args := m.Called(ctx, announce)
	return args.Error(0)
}

// GetAnnounce mocks the GetAnnounce method
func (m *MockStorage) GetAnnounce(ctx context.Context, actor, object string) (*storage.Announce, error) {
	args := m.Called(ctx, actor, object)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Announce), args.Error(1)
}

// DeleteAnnounce mocks the DeleteAnnounce method
func (m *MockStorage) DeleteAnnounce(ctx context.Context, actor, object string) error {
	args := m.Called(ctx, actor, object)
	return args.Error(0)
}

// GetObjectAnnounces mocks the GetObjectAnnounces method
func (m *MockStorage) GetObjectAnnounces(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Announce), args.String(1), args.Error(2)
}

// GetActorAnnounces mocks the GetActorAnnounces method
func (m *MockStorage) GetActorAnnounces(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Announce), args.String(1), args.Error(2)
}

// CountObjectAnnounces mocks the CountObjectAnnounces method
func (m *MockStorage) CountObjectAnnounces(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// TombstoneObject mocks the TombstoneObject method
func (m *MockStorage) TombstoneObject(ctx context.Context, objectID string, deletedBy string) error {
	args := m.Called(ctx, objectID, deletedBy)
	return args.Error(0)
}

// GetTombstone mocks the GetTombstone method
func (m *MockStorage) GetTombstone(ctx context.Context, objectID string) (*storage.Tombstone, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Tombstone), args.Error(1)
}

// CascadeDeleteLikes mocks the CascadeDeleteLikes method
func (m *MockStorage) CascadeDeleteLikes(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// CascadeDeleteAnnounces mocks the CascadeDeleteAnnounces method
func (m *MockStorage) CascadeDeleteAnnounces(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// CreateBlock mocks the CreateBlock method
func (m *MockStorage) CreateBlock(ctx context.Context, block *storage.Block) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

// GetBlock mocks the GetBlock method
func (m *MockStorage) GetBlock(ctx context.Context, actor, blockedActor string) (*storage.Block, error) {
	args := m.Called(ctx, actor, blockedActor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Block), args.Error(1)
}

// DeleteBlock mocks the DeleteBlock method
func (m *MockStorage) DeleteBlock(ctx context.Context, actor, blockedActor string) error {
	args := m.Called(ctx, actor, blockedActor)
	return args.Error(0)
}

// GetBlockedActors mocks the GetBlockedActors method
func (m *MockStorage) GetBlockedActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	args := m.Called(ctx, actor, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Block), args.String(1), args.Error(2)
}

// GetBlockedByActors mocks the GetBlockedByActors method
func (m *MockStorage) GetBlockedByActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	args := m.Called(ctx, actor, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Block), args.String(1), args.Error(2)
}

// IsBlocked mocks the IsBlocked method
func (m *MockStorage) IsBlocked(ctx context.Context, actor, targetActor string) (bool, error) {
	args := m.Called(ctx, actor, targetActor)
	return args.Bool(0), args.Error(1)
}

// IsBlockedBidirectional mocks the IsBlockedBidirectional method
func (m *MockStorage) IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error) {
	args := m.Called(ctx, actor1, actor2)
	return args.Bool(0), args.Error(1)
}

// CreateFlag creates a new flag for content moderation
func (m *MockStorage) CreateFlag(ctx context.Context, flag *storage.Flag) error {
	args := m.Called(ctx, flag)
	return args.Error(0)
}

// GetFlag mocks the GetFlag method
func (m *MockStorage) GetFlag(ctx context.Context, id string) (*storage.Flag, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Flag), args.Error(1)
}

// GetFlagsByObject mocks the GetFlagsByObject method
func (m *MockStorage) GetFlagsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Flag), args.String(1), args.Error(2)
}

// GetFlagsByActor mocks the GetFlagsByActor method
func (m *MockStorage) GetFlagsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Flag), args.String(1), args.Error(2)
}

// GetPendingFlags mocks the GetPendingFlags method
func (m *MockStorage) GetPendingFlags(ctx context.Context, limit int, cursor string) ([]*storage.Flag, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Flag), args.String(1), args.Error(2)
}

// UpdateFlagStatus mocks the UpdateFlagStatus method
func (m *MockStorage) UpdateFlagStatus(ctx context.Context, id string, status storage.FlagStatus, reviewedBy string, reviewNote string) error {
	args := m.Called(ctx, id, status, reviewedBy, reviewNote)
	return args.Error(0)
}

// CountPendingFlags mocks the CountPendingFlags method
func (m *MockStorage) CountPendingFlags(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

// CreateMove creates a new account move record
func (m *MockStorage) CreateMove(ctx context.Context, move *storage.Move) error {
	args := m.Called(ctx, move)
	return args.Error(0)
}

// GetMove mocks the GetMove method
func (m *MockStorage) GetMove(ctx context.Context, actor string) (*storage.Move, error) {
	args := m.Called(ctx, actor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Move), args.Error(1)
}

// GetMoveByTarget mocks the GetMoveByTarget method
func (m *MockStorage) GetMoveByTarget(ctx context.Context, target string) ([]*storage.Move, error) {
	args := m.Called(ctx, target)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Move), args.Error(1)
}

// HasMovedFrom mocks the HasMovedFrom method
func (m *MockStorage) HasMovedFrom(ctx context.Context, oldActor, newActor string) (bool, error) {
	args := m.Called(ctx, oldActor, newActor)
	return args.Bool(0), args.Error(1)
}

// AddToCollection adds an item to a collection
func (m *MockStorage) AddToCollection(ctx context.Context, collection string, item *storage.CollectionItem) error {
	args := m.Called(ctx, collection, item)
	return args.Error(0)
}

// RemoveFromCollection mocks the RemoveFromCollection method
func (m *MockStorage) RemoveFromCollection(ctx context.Context, collection string, itemID string) error {
	args := m.Called(ctx, collection, itemID)
	return args.Error(0)
}

// GetCollectionItems mocks the GetCollectionItems method
func (m *MockStorage) GetCollectionItems(ctx context.Context, collection string, limit int, cursor string) ([]*storage.CollectionItem, string, error) {
	args := m.Called(ctx, collection, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.CollectionItem), args.String(1), args.Error(2)
}

// IsInCollection mocks the IsInCollection method
func (m *MockStorage) IsInCollection(ctx context.Context, collection string, itemID string) (bool, error) {
	args := m.Called(ctx, collection, itemID)
	return args.Bool(0), args.Error(1)
}

// CountCollectionItems mocks the CountCollectionItems method
func (m *MockStorage) CountCollectionItems(ctx context.Context, collection string) (int, error) {
	args := m.Called(ctx, collection)
	return args.Int(0), args.Error(1)
}

// AcknowledgeSeverance mocks the AcknowledgeSeverance method
func (m *MockStorage) AcknowledgeSeverance(ctx context.Context, userID, domain string) error {
	args := m.Called(ctx, userID, domain)
	return args.Error(0)
}

// AttemptReconnection mocks the AttemptReconnection method
func (m *MockStorage) AttemptReconnection(ctx context.Context, userID, domain string) error {
	args := m.Called(ctx, userID, domain)
	return args.Error(0)
}

// GetUserSeveredRelationships mocks the GetUserSeveredRelationships method
func (m *MockStorage) GetUserSeveredRelationships(ctx context.Context, userID string) ([]*storage.SeveredRelationship, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.SeveredRelationship), args.Error(1)
}

// CreateList mocks the CreateList method
func (m *MockStorage) CreateList(ctx context.Context, username, title, repliesPolicy string) (*storage.List, error) {
	args := m.Called(ctx, username, title, repliesPolicy)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.List), args.Error(1)
}

// GetList mocks the GetList method
func (m *MockStorage) GetList(ctx context.Context, listID string) (*storage.List, error) {
	args := m.Called(ctx, listID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.List), args.Error(1)
}

// GetListsForUser mocks the GetListsForUser method
func (m *MockStorage) GetListsForUser(ctx context.Context, username string) ([]*storage.List, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.List), args.Error(1)
}

// UpdateList mocks the UpdateList method
func (m *MockStorage) UpdateList(ctx context.Context, listID string, updates map[string]any) error {
	args := m.Called(ctx, listID, updates)
	return args.Error(0)
}

// DeleteList mocks the DeleteList method
func (m *MockStorage) DeleteList(ctx context.Context, listID string) error {
	args := m.Called(ctx, listID)
	return args.Error(0)
}

// AddAccountsToList mocks the AddAccountsToList method
func (m *MockStorage) AddAccountsToList(ctx context.Context, listID string, accountIDs []string) error {
	args := m.Called(ctx, listID, accountIDs)
	return args.Error(0)
}

// RemoveAccountsFromList mocks the RemoveAccountsFromList method
func (m *MockStorage) RemoveAccountsFromList(ctx context.Context, listID string, accountIDs []string) error {
	args := m.Called(ctx, listID, accountIDs)
	return args.Error(0)
}

// GetListAccounts mocks the GetListAccounts method
func (m *MockStorage) GetListAccounts(ctx context.Context, listID string) ([]string, error) {
	args := m.Called(ctx, listID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// IsAccountInList mocks the IsAccountInList method
func (m *MockStorage) IsAccountInList(ctx context.Context, listID, accountID string) (bool, error) {
	args := m.Called(ctx, listID, accountID)
	return args.Bool(0), args.Error(1)
}

// GetListsContainingAccount mocks the GetListsContainingAccount method
func (m *MockStorage) GetListsContainingAccount(ctx context.Context, accountID, username string) ([]*storage.List, error) {
	args := m.Called(ctx, accountID, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.List), args.Error(1)
}

// GetListTimeline mocks the GetListTimeline method
func (m *MockStorage) GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, listID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TimelineEntry), args.String(1), args.Error(2)
}

// ListInstanceDomainBlocks mocks the ListInstanceDomainBlocks method
func (m *MockStorage) ListInstanceDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.InstanceDomainBlock), args.String(1), args.Error(2)
}

// ListUsersByRole mocks the ListUsersByRole method
func (m *MockStorage) ListUsersByRole(ctx context.Context, role string) ([]*storage.User, error) {
	args := m.Called(ctx, role)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.User), args.Error(1)
}

// CreateAnnouncement mocks the CreateAnnouncement method
func (m *MockStorage) CreateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	args := m.Called(ctx, announcement)
	return args.Error(0)
}

// GetAnnouncement mocks the GetAnnouncement method
func (m *MockStorage) GetAnnouncement(ctx context.Context, id string) (*storage.Announcement, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Announcement), args.Error(1)
}

// GetAnnouncements mocks the GetAnnouncements method
func (m *MockStorage) GetAnnouncements(ctx context.Context, active bool) ([]*storage.Announcement, error) {
	args := m.Called(ctx, active)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Announcement), args.Error(1)
}

// UpdateAnnouncement mocks the UpdateAnnouncement method
func (m *MockStorage) UpdateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	args := m.Called(ctx, announcement)
	return args.Error(0)
}

// DeleteAnnouncement mocks the DeleteAnnouncement method
func (m *MockStorage) DeleteAnnouncement(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// DismissAnnouncement mocks the DismissAnnouncement method
func (m *MockStorage) DismissAnnouncement(ctx context.Context, username, announcementID string) error {
	args := m.Called(ctx, username, announcementID)
	return args.Error(0)
}

// GetDismissedAnnouncements mocks the GetDismissedAnnouncements method
func (m *MockStorage) GetDismissedAnnouncements(ctx context.Context, username string) ([]string, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// AddAnnouncementReaction mocks the AddAnnouncementReaction method
func (m *MockStorage) AddAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error {
	args := m.Called(ctx, username, announcementID, emojiName)
	return args.Error(0)
}

// RemoveAnnouncementReaction mocks the RemoveAnnouncementReaction method
func (m *MockStorage) RemoveAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error {
	args := m.Called(ctx, username, announcementID, emojiName)
	return args.Error(0)
}

// GetAnnouncementReactions mocks the GetAnnouncementReactions method
func (m *MockStorage) GetAnnouncementReactions(ctx context.Context, announcementID string) (map[string][]string, error) {
	args := m.Called(ctx, announcementID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string][]string), args.Error(1)
}

// AddDomainBlock mocks the AddDomainBlock method
func (m *MockStorage) AddDomainBlock(ctx context.Context, username, domain string) error {
	args := m.Called(ctx, username, domain)
	return args.Error(0)
}

// RemoveDomainBlock mocks the RemoveDomainBlock method
func (m *MockStorage) RemoveDomainBlock(ctx context.Context, username, domain string) error {
	args := m.Called(ctx, username, domain)
	return args.Error(0)
}

// GetUserDomainBlocks mocks the GetUserDomainBlocks method
func (m *MockStorage) GetUserDomainBlocks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

// CreateInstanceDomainBlock mocks the CreateInstanceDomainBlock method
func (m *MockStorage) CreateInstanceDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

// GetInstanceDomainBlock mocks the GetInstanceDomainBlock method
func (m *MockStorage) GetInstanceDomainBlock(ctx context.Context, domain string) (*storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceDomainBlock), args.Error(1)
}

// GetInstanceDomainBlockByID mocks the GetInstanceDomainBlockByID method
func (m *MockStorage) GetInstanceDomainBlockByID(ctx context.Context, id string) (*storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceDomainBlock), args.Error(1)
}

// UpdateInstanceDomainBlock mocks the UpdateInstanceDomainBlock method
func (m *MockStorage) UpdateInstanceDomainBlock(ctx context.Context, domain string, updates map[string]any) error {
	args := m.Called(ctx, domain, updates)
	return args.Error(0)
}

// DeleteInstanceDomainBlock mocks the DeleteInstanceDomainBlock method
func (m *MockStorage) DeleteInstanceDomainBlock(ctx context.Context, domain string) error {
	args := m.Called(ctx, domain)
	return args.Error(0)
}

// IsInstanceDomainBlocked mocks the IsInstanceDomainBlocked method
func (m *MockStorage) IsInstanceDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, domain)
	return args.Bool(0), args.Get(1).(*storage.InstanceDomainBlock), args.Error(2)
}

// GetDomainBlocks mocks the GetDomainBlocks method
func (m *MockStorage) GetDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.InstanceDomainBlock), args.String(1), args.Error(2)
}

// GetDomainBlock mocks the GetDomainBlock method
func (m *MockStorage) GetDomainBlock(ctx context.Context, id string) (*storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceDomainBlock), args.Error(1)
}

// CreateDomainBlock mocks the CreateDomainBlock method
func (m *MockStorage) CreateDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

// UpdateDomainBlock mocks the UpdateDomainBlock method
func (m *MockStorage) UpdateDomainBlock(ctx context.Context, id string, updates map[string]any) error {
	args := m.Called(ctx, id, updates)
	return args.Error(0)
}

// DeleteDomainBlock mocks the DeleteDomainBlock method
func (m *MockStorage) DeleteDomainBlock(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// IsDomainBlocked mocks the IsDomainBlocked method
func (m *MockStorage) IsDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, domain)
	return args.Bool(0), args.Get(1).(*storage.InstanceDomainBlock), args.Error(2)
}

// CreateEmailDomainBlock mocks the CreateEmailDomainBlock method
func (m *MockStorage) CreateEmailDomainBlock(ctx context.Context, block *storage.EmailDomainBlock) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

// GetEmailDomainBlocks mocks the GetEmailDomainBlocks method
func (m *MockStorage) GetEmailDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.EmailDomainBlock, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.EmailDomainBlock), args.String(1), args.Error(2)
}

// DeleteEmailDomainBlock mocks the DeleteEmailDomainBlock method
func (m *MockStorage) DeleteEmailDomainBlock(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// CreateFilter mocks the CreateFilter method
func (m *MockStorage) CreateFilter(ctx context.Context, filter *storage.Filter) error {
	args := m.Called(ctx, filter)
	return args.Error(0)
}

// GetFilter mocks the GetFilter method
func (m *MockStorage) GetFilter(ctx context.Context, filterID string) (*storage.Filter, error) {
	args := m.Called(ctx, filterID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Filter), args.Error(1)
}

// GetFiltersForUser mocks the GetFiltersForUser method
func (m *MockStorage) GetFiltersForUser(ctx context.Context, username string) ([]*storage.Filter, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Filter), args.Error(1)
}

// UpdateFilter mocks the UpdateFilter method
func (m *MockStorage) UpdateFilter(ctx context.Context, filterID string, updates map[string]any) error {
	args := m.Called(ctx, filterID, updates)
	return args.Error(0)
}

// DeleteFilter mocks the DeleteFilter method
func (m *MockStorage) DeleteFilter(ctx context.Context, filterID string) error {
	args := m.Called(ctx, filterID)
	return args.Error(0)
}

// AddFilterKeyword mocks the AddFilterKeyword method
func (m *MockStorage) AddFilterKeyword(ctx context.Context, filterID string, keyword *storage.FilterKeyword) error {
	args := m.Called(ctx, filterID, keyword)
	return args.Error(0)
}

// GetFilterKeywords mocks the GetFilterKeywords method
func (m *MockStorage) GetFilterKeywords(ctx context.Context, filterID string) ([]*storage.FilterKeyword, error) {
	args := m.Called(ctx, filterID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FilterKeyword), args.Error(1)
}

// UpdateFilterKeyword mocks the UpdateFilterKeyword method
func (m *MockStorage) UpdateFilterKeyword(ctx context.Context, keywordID string, updates map[string]any) error {
	args := m.Called(ctx, keywordID, updates)
	return args.Error(0)
}

// DeleteFilterKeyword mocks the DeleteFilterKeyword method
func (m *MockStorage) DeleteFilterKeyword(ctx context.Context, keywordID string) error {
	args := m.Called(ctx, keywordID)
	return args.Error(0)
}

// AddFilterStatus mocks the AddFilterStatus method
func (m *MockStorage) AddFilterStatus(ctx context.Context, filterID string, status *storage.FilterStatus) error {
	args := m.Called(ctx, filterID, status)
	return args.Error(0)
}

// GetFilterStatuses mocks the GetFilterStatuses method
func (m *MockStorage) GetFilterStatuses(ctx context.Context, filterID string) ([]*storage.FilterStatus, error) {
	args := m.Called(ctx, filterID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FilterStatus), args.Error(1)
}

// DeleteFilterStatus mocks the DeleteFilterStatus method
func (m *MockStorage) DeleteFilterStatus(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

// CreateModerationEvent mocks the CreateModerationEvent method
func (m *MockStorage) CreateModerationEvent(ctx context.Context, event *storage.ModerationEvent) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

// GetModerationEvent mocks the GetModerationEvent method
func (m *MockStorage) GetModerationEvent(ctx context.Context, eventID string) (*storage.ModerationEvent, error) {
	args := m.Called(ctx, eventID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ModerationEvent), args.Error(1)
}

// GetModerationQueue mocks the GetModerationQueue method
func (m *MockStorage) GetModerationQueue(ctx context.Context, filter *storage.ModerationFilter) ([]*storage.ModerationQueueItem, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.ModerationQueueItem), args.Error(1)
}

// GetModerationQueuePaginated mocks the GetModerationQueuePaginated method
func (m *MockStorage) GetModerationQueuePaginated(ctx context.Context, limit int, cursor string) ([]*storage.ModerationQueueItem, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.ModerationQueueItem), args.String(1), args.Error(2)
}

// GetModerationEventsByObject mocks the GetModerationEventsByObject method
func (m *MockStorage) GetModerationEventsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.ModerationEvent), args.String(1), args.Error(2)
}

// GetModerationEventsByActor mocks the GetModerationEventsByActor method
func (m *MockStorage) GetModerationEventsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.ModerationEvent), args.String(1), args.Error(2)
}

// AddModerationReview mocks the AddModerationReview method
func (m *MockStorage) AddModerationReview(ctx context.Context, review *storage.ModerationReview) error {
	args := m.Called(ctx, review)
	return args.Error(0)
}

// GetModerationReviews mocks the GetModerationReviews method
func (m *MockStorage) GetModerationReviews(ctx context.Context, eventID string) ([]*storage.ModerationReview, error) {
	args := m.Called(ctx, eventID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.ModerationReview), args.Error(1)
}

// CreateModerationDecision mocks the CreateModerationDecision method
func (m *MockStorage) CreateModerationDecision(ctx context.Context, decision *storage.ModerationDecision) error {
	args := m.Called(ctx, decision)
	return args.Error(0)
}

// GetModerationDecision mocks the GetModerationDecision method
func (m *MockStorage) GetModerationDecision(ctx context.Context, objectID string) (*storage.ModerationDecision, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ModerationDecision), args.Error(1)
}

// StoreModerationDecision mocks the StoreModerationDecision method
func (m *MockStorage) StoreModerationDecision(ctx context.Context, decision *storage.ModerationDecision) error {
	args := m.Called(ctx, decision)
	return args.Error(0)
}

// UpdateModerationDecision mocks the UpdateModerationDecision method
func (m *MockStorage) UpdateModerationDecision(ctx context.Context, contentID string, review *storage.ModerationReview) error {
	args := m.Called(ctx, contentID, review)
	return args.Error(0)
}

// GetModerationHistory mocks the GetModerationHistory method
func (m *MockStorage) GetModerationHistory(ctx context.Context, objectID string) (*storage.ModerationHistory, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ModerationHistory), args.Error(1)
}

// GetModerationEvents mocks the GetModerationEvents method
func (m *MockStorage) GetModerationEvents(ctx context.Context, filter *storage.ModerationEventFilter, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	args := m.Called(ctx, filter, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.ModerationEvent), args.String(1), args.Error(2)
}

// CreateModerationPattern mocks the CreateModerationPattern method
func (m *MockStorage) CreateModerationPattern(ctx context.Context, pattern *storage.ModerationPattern) error {
	args := m.Called(ctx, pattern)
	return args.Error(0)
}

// GetModerationPattern mocks the GetModerationPattern method
func (m *MockStorage) GetModerationPattern(ctx context.Context, patternID string) (*storage.ModerationPattern, error) {
	args := m.Called(ctx, patternID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ModerationPattern), args.Error(1)
}

// GetModerationPatterns mocks the GetModerationPatterns method
func (m *MockStorage) GetModerationPatterns(ctx context.Context, active bool, severity string, limit int) ([]*storage.ModerationPattern, error) {
	args := m.Called(ctx, active, severity, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.ModerationPattern), args.Error(1)
}

// UpdateModerationPattern mocks the UpdateModerationPattern method
func (m *MockStorage) UpdateModerationPattern(ctx context.Context, pattern *storage.ModerationPattern) error {
	args := m.Called(ctx, pattern)
	return args.Error(0)
}

// DeleteModerationPattern mocks the DeleteModerationPattern method
func (m *MockStorage) DeleteModerationPattern(ctx context.Context, patternID string) error {
	args := m.Called(ctx, patternID)
	return args.Error(0)
}

// GetModerationQueueCount mocks the GetModerationQueueCount method
func (m *MockStorage) GetModerationQueueCount(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

// CreateConversation mocks the CreateConversation method
func (m *MockStorage) CreateConversation(ctx context.Context, conversation *storage.Conversation) error {
	args := m.Called(ctx, conversation)
	return args.Error(0)
}

// GetConversation mocks the GetConversation method
func (m *MockStorage) GetConversation(ctx context.Context, id string) (*storage.Conversation, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Conversation), args.Error(1)
}

// GetConversationByParticipants mocks the GetConversationByParticipants method
func (m *MockStorage) GetConversationByParticipants(ctx context.Context, participants []string) (*storage.Conversation, error) {
	args := m.Called(ctx, participants)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Conversation), args.Error(1)
}

// UpdateConversationLastStatus mocks the UpdateConversationLastStatus method
func (m *MockStorage) UpdateConversationLastStatus(ctx context.Context, id, lastStatusID string) error {
	args := m.Called(ctx, id, lastStatusID)
	return args.Error(0)
}

// MarkConversationRead mocks the MarkConversationRead method
func (m *MockStorage) MarkConversationRead(ctx context.Context, id, username string) error {
	args := m.Called(ctx, id, username)
	return args.Error(0)
}

// DeleteConversation mocks the DeleteConversation method
func (m *MockStorage) DeleteConversation(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// GetUserConversations mocks the GetUserConversations method
func (m *MockStorage) GetUserConversations(ctx context.Context, username string, limit int, cursor string) ([]*storage.Conversation, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Conversation), args.String(1), args.Error(2)
}

// AddParticipantToConversation mocks the AddParticipantToConversation method
func (m *MockStorage) AddParticipantToConversation(ctx context.Context, conversationID, participantID string) error {
	args := m.Called(ctx, conversationID, participantID)
	return args.Error(0)
}

// CreateConversationMute mocks the CreateConversationMute method
func (m *MockStorage) CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error {
	args := m.Called(ctx, mute)
	return args.Error(0)
}

// DeleteConversationMute mocks the DeleteConversationMute method
func (m *MockStorage) DeleteConversationMute(ctx context.Context, username, conversationID string) error {
	args := m.Called(ctx, username, conversationID)
	return args.Error(0)
}

// IsConversationMuted mocks the IsConversationMuted method
func (m *MockStorage) IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error) {
	args := m.Called(ctx, username, conversationID)
	return args.Bool(0), args.Error(1)
}

// GetMutedConversations mocks the GetMutedConversations method
func (m *MockStorage) GetMutedConversations(ctx context.Context, username string) ([]string, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// CreateReport mocks the CreateReport method
func (m *MockStorage) CreateReport(ctx context.Context, report *storage.Report) error {
	args := m.Called(ctx, report)
	return args.Error(0)
}

// GetReport mocks the GetReport method
func (m *MockStorage) GetReport(ctx context.Context, id string) (*storage.Report, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Report), args.Error(1)
}

// GetUserReports mocks the GetUserReports method
func (m *MockStorage) GetUserReports(ctx context.Context, username string, limit int, cursor string) ([]*storage.Report, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Report), args.String(1), args.Error(2)
}

// GetReportsByTarget mocks the GetReportsByTarget method
func (m *MockStorage) GetReportsByTarget(ctx context.Context, targetAccountID string, limit int, cursor string) ([]*storage.Report, string, error) {
	args := m.Called(ctx, targetAccountID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Report), args.String(1), args.Error(2)
}

// GetReportsByStatus mocks the GetReportsByStatus method
func (m *MockStorage) GetReportsByStatus(ctx context.Context, status storage.ReportStatus, limit int, cursor string) ([]*storage.Report, string, error) {
	args := m.Called(ctx, status, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Report), args.String(1), args.Error(2)
}

// UpdateReportStatus mocks the UpdateReportStatus method
func (m *MockStorage) UpdateReportStatus(ctx context.Context, id string, status storage.ReportStatus, actionTaken string, moderatorID string) error {
	args := m.Called(ctx, id, status, actionTaken, moderatorID)
	return args.Error(0)
}

// GetReportStats mocks the GetReportStats method
func (m *MockStorage) GetReportStats(ctx context.Context, username string) (*storage.ReportStats, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ReportStats), args.Error(1)
}

// IncrementFalseReports mocks the IncrementFalseReports method
func (m *MockStorage) IncrementFalseReports(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// AssignReport mocks the AssignReport method
func (m *MockStorage) AssignReport(ctx context.Context, reportID string, assignedTo string) error {
	args := m.Called(ctx, reportID, assignedTo)
	return args.Error(0)
}

// UnassignReport mocks the UnassignReport method
func (m *MockStorage) UnassignReport(ctx context.Context, reportID string) error {
	args := m.Called(ctx, reportID)
	return args.Error(0)
}

// GetInstanceHealthReport mocks the GetInstanceHealthReport method
func (m *MockStorage) GetInstanceHealthReport(ctx context.Context, domain string, period time.Duration) (*storage.InstanceHealthReport, error) {
	args := m.Called(ctx, domain, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceHealthReport), args.Error(1)
}

// GetOpenReportsCount mocks the GetOpenReportsCount method
func (m *MockStorage) GetOpenReportsCount(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

// GetReportedStatuses mocks the GetReportedStatuses method
func (m *MockStorage) GetReportedStatuses(ctx context.Context, reportID string) ([]any, error) {
	args := m.Called(ctx, reportID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

// CreateNotification mocks the CreateNotification method
func (m *MockStorage) CreateNotification(ctx context.Context, notification *storage.Notification) error {
	args := m.Called(ctx, notification)
	return args.Error(0)
}

// GetNotification mocks the GetNotification method
func (m *MockStorage) GetNotification(ctx context.Context, id string) (*storage.Notification, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Notification), args.Error(1)
}

// GetNotifications mocks the GetNotifications method
func (m *MockStorage) GetNotifications(ctx context.Context, username string, limit int, cursor string) ([]*storage.Notification, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Notification), args.String(1), args.Error(2)
}

// GetNotificationsFiltered mocks the GetNotificationsFiltered method
func (m *MockStorage) GetNotificationsFiltered(ctx context.Context, username string, filter *storage.NotificationFilter) ([]*storage.Notification, string, error) {
	args := m.Called(ctx, username, filter)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Notification), args.String(1), args.Error(2)
}

// MarkNotificationAsRead mocks the MarkNotificationAsRead method
func (m *MockStorage) MarkNotificationAsRead(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// MarkAllNotificationsAsRead mocks the MarkAllNotificationsAsRead method
func (m *MockStorage) MarkAllNotificationsAsRead(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// DeleteNotification mocks the DeleteNotification method
func (m *MockStorage) DeleteNotification(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// ClearNotifications mocks the ClearNotifications method
func (m *MockStorage) ClearNotifications(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// CountUnreadNotifications mocks the CountUnreadNotifications method
func (m *MockStorage) CountUnreadNotifications(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

// GetNotificationsAdvanced mocks the GetNotificationsAdvanced method
func (m *MockStorage) GetNotificationsAdvanced(ctx context.Context, userID string, excludeTypes []string, maxID, sinceID, minID *string, limit int, includeFiltered bool) ([]*storage.Notification, error) {
	args := m.Called(ctx, userID, excludeTypes, maxID, sinceID, minID, limit, includeFiltered)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Notification), args.Error(1)
}

// GetNotificationsByAccount mocks the GetNotificationsByAccount method
func (m *MockStorage) GetNotificationsByAccount(ctx context.Context, userID, accountID string, limit int) ([]*storage.Notification, error) {
	args := m.Called(ctx, userID, accountID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Notification), args.Error(1)
}

// GetUnreadNotificationCount mocks the GetUnreadNotificationCount method
func (m *MockStorage) GetUnreadNotificationCount(ctx context.Context, userID string) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

// GetNotificationPreferences mocks the GetNotificationPreferences method
func (m *MockStorage) GetNotificationPreferences(ctx context.Context, username string) (*storage.NotificationPreferences, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.NotificationPreferences), args.Error(1)
}

// UpdateNotificationPreferences mocks the UpdateNotificationPreferences method
func (m *MockStorage) UpdateNotificationPreferences(ctx context.Context, username string, prefs *storage.NotificationPreferences) error {
	args := m.Called(ctx, username, prefs)
	return args.Error(0)
}

// BatchMarkNotificationsAsRead mocks the BatchMarkNotificationsAsRead method
func (m *MockStorage) BatchMarkNotificationsAsRead(ctx context.Context, username string, notificationIDs []string) error {
	args := m.Called(ctx, username, notificationIDs)
	return args.Error(0)
}

// CacheRemoteActor mocks the CacheRemoteActor method
func (m *MockStorage) CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	args := m.Called(ctx, handle, actor, ttl)
	return args.Error(0)
}

// GetCachedRemoteActor mocks the GetCachedRemoteActor method
func (m *MockStorage) GetCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error) {
	args := m.Called(ctx, handle)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

// GetDNSCache mocks the GetDNSCache method
func (m *MockStorage) GetDNSCache(ctx context.Context, hostname string) (*storage.DNSCacheEntry, error) {
	args := m.Called(ctx, hostname)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.DNSCacheEntry), args.Error(1)
}

// SetDNSCache mocks the SetDNSCache method
func (m *MockStorage) SetDNSCache(ctx context.Context, entry *storage.DNSCacheEntry) error {
	args := m.Called(ctx, entry)
	return args.Error(0)
}

// GetFederationStatistics mocks the GetFederationStatistics method
func (m *MockStorage) GetFederationStatistics(ctx context.Context, startTime, endTime time.Time) (*storage.FederationStats, error) {
	args := m.Called(ctx, startTime, endTime)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.FederationStats), args.Error(1)
}

// RecordFederationActivity mocks the RecordFederationActivity method
func (m *MockStorage) RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

// GetFederationCosts mocks the GetFederationCosts method
func (m *MockStorage) GetFederationCosts(ctx context.Context, startTime, endTime time.Time, limit int, cursor string) ([]*storage.FederationCost, string, error) {
	args := m.Called(ctx, startTime, endTime, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.FederationCost), args.String(1), args.Error(2)
}

// GetFederationNodes mocks the GetFederationNodes method
func (m *MockStorage) GetFederationNodes(ctx context.Context, depth int) ([]*storage.FederationNode, error) {
	args := m.Called(ctx, depth)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FederationNode), args.Error(1)
}

// GetFederationEdges mocks the GetFederationEdges method
func (m *MockStorage) GetFederationEdges(ctx context.Context, domains []string) ([]*storage.FederationEdge, error) {
	args := m.Called(ctx, domains)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FederationEdge), args.Error(1)
}

// CalculateFederationClusters mocks the CalculateFederationClusters method
func (m *MockStorage) CalculateFederationClusters(ctx context.Context) ([]*storage.InstanceCluster, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.InstanceCluster), args.Error(1)
}

// UpdateFederationNode mocks the UpdateFederationNode method
func (m *MockStorage) UpdateFederationNode(ctx context.Context, node *storage.FederationNode) error {
	args := m.Called(ctx, node)
	return args.Error(0)
}

// UpdateFederationEdge mocks the UpdateFederationEdge method
func (m *MockStorage) UpdateFederationEdge(ctx context.Context, edge *storage.FederationEdge) error {
	args := m.Called(ctx, edge)
	return args.Error(0)
}

// StoreFederationTimeSeries mocks the StoreFederationTimeSeries method
func (m *MockStorage) StoreFederationTimeSeries(ctx context.Context, data *storage.FederationTimeSeries) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

// StoreInstanceCluster mocks the StoreInstanceCluster method
func (m *MockStorage) StoreInstanceCluster(ctx context.Context, cluster *storage.InstanceCluster) error {
	args := m.Called(ctx, cluster)
	return args.Error(0)
}

// TrackFederationIssue mocks the TrackFederationIssue method
func (m *MockStorage) TrackFederationIssue(ctx context.Context, domain, issueType string) error {
	args := m.Called(ctx, domain, issueType)
	return args.Error(0)
}

// CheckCommunityNoteRateLimit mocks the CheckCommunityNoteRateLimit method
func (m *MockStorage) CheckCommunityNoteRateLimit(ctx context.Context, userID string, limit int) (bool, int, error) {
	args := m.Called(ctx, userID, limit)
	return args.Bool(0), args.Int(1), args.Error(2)
}

// IsRateLimited mocks the IsRateLimited method
func (m *MockStorage) IsRateLimited(ctx context.Context, identifier string) (bool, time.Time, error) {
	args := m.Called(ctx, identifier)
	return args.Bool(0), args.Get(1).(time.Time), args.Error(2)
}

// CheckAPIRateLimit mocks the CheckAPIRateLimit method
func (m *MockStorage) CheckAPIRateLimit(ctx context.Context, userID, endpoint string, limit int, window time.Duration) error {
	args := m.Called(ctx, userID, endpoint, limit, window)
	return args.Error(0)
}

// GetAPIRateLimitInfo mocks the GetAPIRateLimitInfo method
func (m *MockStorage) GetAPIRateLimitInfo(ctx context.Context, userID, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error) {
	args := m.Called(ctx, userID, endpoint, limit, window)
	return args.Int(0), args.Get(1).(time.Time), args.Error(2)
}

// RecordLoginAttempt mocks the RecordLoginAttempt method
func (m *MockStorage) RecordLoginAttempt(ctx context.Context, identifier string, success bool) error {
	args := m.Called(ctx, identifier, success)
	return args.Error(0)
}

// GetLoginAttemptCount mocks the GetLoginAttemptCount method
func (m *MockStorage) GetLoginAttemptCount(ctx context.Context, identifier string, since time.Time) (int, error) {
	args := m.Called(ctx, identifier, since)
	return args.Int(0), args.Error(1)
}

// ClearLoginAttempts mocks the ClearLoginAttempts method
func (m *MockStorage) ClearLoginAttempts(ctx context.Context, identifier string) error {
	args := m.Called(ctx, identifier)
	return args.Error(0)
}

// CreateQuoteRelationship mocks the CreateQuoteRelationship method
func (m *MockStorage) CreateQuoteRelationship(ctx context.Context, quote *storage.QuoteRelationship) error {
	args := m.Called(ctx, quote)
	return args.Error(0)
}

// GetQuotesForNote mocks the GetQuotesForNote method
func (m *MockStorage) GetQuotesForNote(ctx context.Context, noteID string, limit int, cursor string) ([]*storage.QuoteRelationship, string, error) {
	args := m.Called(ctx, noteID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.QuoteRelationship), args.String(1), args.Error(2)
}

// IsQuoted mocks the IsQuoted method
func (m *MockStorage) IsQuoted(ctx context.Context, actorID, noteID string) (bool, error) {
	args := m.Called(ctx, actorID, noteID)
	return args.Bool(0), args.Error(1)
}

// WithdrawQuote mocks the WithdrawQuote method
func (m *MockStorage) WithdrawQuote(ctx context.Context, quoteNoteID string) error {
	args := m.Called(ctx, quoteNoteID)
	return args.Error(0)
}

// CountQuotes mocks the CountQuotes method
func (m *MockStorage) CountQuotes(ctx context.Context, noteID string) (int, error) {
	args := m.Called(ctx, noteID)
	return args.Int(0), args.Error(1)
}

// WithdrawStatusFromQuotes mocks the WithdrawStatusFromQuotes method
func (m *MockStorage) WithdrawStatusFromQuotes(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

// UpdateQuotePermissions mocks the UpdateQuotePermissions method
func (m *MockStorage) UpdateQuotePermissions(ctx context.Context, statusID string, permissions *storage.QuotePermissions) error {
	args := m.Called(ctx, statusID, permissions)
	return args.Error(0)
}

// IsQuoteAllowed mocks the IsQuoteAllowed method
func (m *MockStorage) IsQuoteAllowed(ctx context.Context, statusID, quoterID string) (bool, error) {
	args := m.Called(ctx, statusID, quoterID)
	return args.Bool(0), args.Error(1)
}

// GetQuoteType mocks the GetQuoteType method
func (m *MockStorage) GetQuoteType(ctx context.Context, statusID string) (string, error) {
	args := m.Called(ctx, statusID)
	return args.String(0), args.Error(1)
}

// IsWithdrawnFromQuotes mocks the IsWithdrawnFromQuotes method
func (m *MockStorage) IsWithdrawnFromQuotes(ctx context.Context, statusID string) (bool, error) {
	args := m.Called(ctx, statusID)
	return args.Bool(0), args.Error(1)
}

// GetQuotesOfStatus mocks the GetQuotesOfStatus method
func (m *MockStorage) GetQuotesOfStatus(ctx context.Context, statusID string, limit int) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, statusID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// GetReplies mocks the GetReplies method
func (m *MockStorage) GetReplies(ctx context.Context, objectID string, limit int, cursor string) ([]any, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]any), args.String(1), args.Error(2)
}

// CountReplies mocks the CountReplies method
func (m *MockStorage) CountReplies(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// IncrementReplyCount mocks the IncrementReplyCount method
func (m *MockStorage) IncrementReplyCount(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// SyncThreadFromRemote mocks the SyncThreadFromRemote method
func (m *MockStorage) SyncThreadFromRemote(ctx context.Context, statusID string) (*storage.StatusSearchResult, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StatusSearchResult), args.Error(1)
}

// SyncMissingRepliesFromRemote mocks the SyncMissingRepliesFromRemote method
func (m *MockStorage) SyncMissingRepliesFromRemote(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// GetMissingReplies mocks the GetMissingReplies method
func (m *MockStorage) GetMissingReplies(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// GetReplyCount mocks the GetReplyCount method
func (m *MockStorage) GetReplyCount(ctx context.Context, statusID string) (int64, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0).(int64), args.Error(1)
}

// GetStatusReplyCount mocks the GetStatusReplyCount method
func (m *MockStorage) GetStatusReplyCount(ctx context.Context, statusID string) (int, error) {
	args := m.Called(ctx, statusID)
	return args.Int(0), args.Error(1)
}

// Recovery Code operations (excluding duplicates)

// StoreRecoveryRequest mocks the StoreRecoveryRequest method
func (m *MockStorage) StoreRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error {
	args := m.Called(ctx, request)
	return args.Error(0)
}

// GetRecoveryRequest mocks the GetRecoveryRequest method
func (m *MockStorage) GetRecoveryRequest(ctx context.Context, requestID string) (*storage.SocialRecoveryRequest, error) {
	args := m.Called(ctx, requestID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SocialRecoveryRequest), args.Error(1)
}

// UpdateRecoveryRequest mocks the UpdateRecoveryRequest method
func (m *MockStorage) UpdateRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error {
	args := m.Called(ctx, request)
	return args.Error(0)
}

// DeleteRecoveryRequest mocks the DeleteRecoveryRequest method
func (m *MockStorage) DeleteRecoveryRequest(ctx context.Context, requestID string) error {
	args := m.Called(ctx, requestID)
	return args.Error(0)
}

// GetActiveRecoveryRequests mocks the GetActiveRecoveryRequests method
func (m *MockStorage) GetActiveRecoveryRequests(ctx context.Context, username string) ([]*storage.SocialRecoveryRequest, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.SocialRecoveryRequest), args.Error(1)
}

// StoreRecoveryCode mocks the StoreRecoveryCode method
func (m *MockStorage) StoreRecoveryCode(ctx context.Context, username string, code *storage.RecoveryCodeItem) error {
	args := m.Called(ctx, username, code)
	return args.Error(0)
}

// GetRecoveryCodes mocks the GetRecoveryCodes method
func (m *MockStorage) GetRecoveryCodes(ctx context.Context, username string) ([]*storage.RecoveryCodeItem, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.RecoveryCodeItem), args.Error(1)
}

// MarkRecoveryCodeUsed mocks the MarkRecoveryCodeUsed method
func (m *MockStorage) MarkRecoveryCodeUsed(ctx context.Context, username, codeHash string) error {
	args := m.Called(ctx, username, codeHash)
	return args.Error(0)
}

// DeleteAllRecoveryCodes mocks the DeleteAllRecoveryCodes method
func (m *MockStorage) DeleteAllRecoveryCodes(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// CountUnusedRecoveryCodes mocks the CountUnusedRecoveryCodes method
func (m *MockStorage) CountUnusedRecoveryCodes(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

// CreateAccountPin mocks the CreateAccountPin method
func (m *MockStorage) CreateAccountPin(ctx context.Context, pin *storage.AccountPin) error {
	args := m.Called(ctx, pin)
	return args.Error(0)
}

// DeleteAccountPin mocks the DeleteAccountPin method
func (m *MockStorage) DeleteAccountPin(ctx context.Context, username, pinnedActorID string) error {
	args := m.Called(ctx, username, pinnedActorID)
	return args.Error(0)
}

// GetAccountPins mocks the GetAccountPins method
func (m *MockStorage) GetAccountPins(ctx context.Context, username string) ([]*storage.AccountPin, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.AccountPin), args.Error(1)
}

// IsAccountPinned mocks the IsAccountPinned method
func (m *MockStorage) IsAccountPinned(ctx context.Context, username, actorID string) (bool, error) {
	args := m.Called(ctx, username, actorID)
	return args.Bool(0), args.Error(1)
}

// CreateStatusPin mocks the CreateStatusPin method
func (m *MockStorage) CreateStatusPin(ctx context.Context, pin *storage.StatusPin) error {
	args := m.Called(ctx, pin)
	return args.Error(0)
}

// DeleteStatusPin mocks the DeleteStatusPin method
func (m *MockStorage) DeleteStatusPin(ctx context.Context, username, statusID string) error {
	args := m.Called(ctx, username, statusID)
	return args.Error(0)
}

// GetStatusPins mocks the GetStatusPins method
func (m *MockStorage) GetStatusPins(ctx context.Context, username string) ([]*storage.StatusPin, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusPin), args.Error(1)
}

// IsStatusPinned mocks the IsStatusPinned method
func (m *MockStorage) IsStatusPinned(ctx context.Context, username, statusID string) (bool, error) {
	args := m.Called(ctx, username, statusID)
	return args.Bool(0), args.Error(1)
}

// CountUserPinnedStatuses mocks the CountUserPinnedStatuses method
func (m *MockStorage) CountUserPinnedStatuses(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

// CreateAccountNote mocks the CreateAccountNote method
func (m *MockStorage) CreateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	args := m.Called(ctx, note)
	return args.Error(0)
}

// GetAccountNote mocks the GetAccountNote method
func (m *MockStorage) GetAccountNote(ctx context.Context, username, targetActorID string) (*storage.AccountNote, error) {
	args := m.Called(ctx, username, targetActorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.AccountNote), args.Error(1)
}

// UpdateAccountNote mocks the UpdateAccountNote method
func (m *MockStorage) UpdateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	args := m.Called(ctx, note)
	return args.Error(0)
}

// DeleteAccountNote mocks the DeleteAccountNote method
func (m *MockStorage) DeleteAccountNote(ctx context.Context, username, targetActorID string) error {
	args := m.Called(ctx, username, targetActorID)
	return args.Error(0)
}

// CreateAdminReview mocks the CreateAdminReview method
func (m *MockStorage) CreateAdminReview(ctx context.Context, eventID string, adminID string, action storage.ActionType, reason string) error {
	args := m.Called(ctx, eventID, adminID, action, reason)
	return args.Error(0)
}

// GetReviewerStats mocks the GetReviewerStats method
func (m *MockStorage) GetReviewerStats(ctx context.Context, reviewerID string) (*storage.ReviewerStats, error) {
	args := m.Called(ctx, reviewerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ReviewerStats), args.Error(1)
}

// CreateBookmark mocks the CreateBookmark method
func (m *MockStorage) CreateBookmark(ctx context.Context, username, objectID string) error {
	args := m.Called(ctx, username, objectID)
	return args.Error(0)
}

// RemoveBookmark mocks the RemoveBookmark method
func (m *MockStorage) RemoveBookmark(ctx context.Context, username, objectID string) error {
	args := m.Called(ctx, username, objectID)
	return args.Error(0)
}

// GetBookmarks mocks the GetBookmarks method
func (m *MockStorage) GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

// IsBookmarked mocks the IsBookmarked method
func (m *MockStorage) IsBookmarked(ctx context.Context, username, objectID string) (bool, error) {
	args := m.Called(ctx, username, objectID)
	return args.Bool(0), args.Error(1)
}

// CreateCommunityNote mocks the CreateCommunityNote method
func (m *MockStorage) CreateCommunityNote(ctx context.Context, note *storage.CommunityNote) error {
	args := m.Called(ctx, note)
	return args.Error(0)
}

// GetCommunityNote mocks the GetCommunityNote method
func (m *MockStorage) GetCommunityNote(ctx context.Context, noteID string) (*storage.CommunityNote, error) {
	args := m.Called(ctx, noteID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.CommunityNote), args.Error(1)
}

// GetCommunityNotesByAuthor mocks the GetCommunityNotesByAuthor method
func (m *MockStorage) GetCommunityNotesByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*storage.CommunityNote, string, error) {
	args := m.Called(ctx, authorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.CommunityNote), args.String(1), args.Error(2)
}

// GetVisibleCommunityNotes mocks the GetVisibleCommunityNotes method
func (m *MockStorage) GetVisibleCommunityNotes(ctx context.Context, objectID string) ([]*storage.CommunityNote, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CommunityNote), args.Error(1)
}

// UpdateCommunityNoteScore mocks the UpdateCommunityNoteScore method
func (m *MockStorage) UpdateCommunityNoteScore(ctx context.Context, noteID string, score float64, status string) error {
	args := m.Called(ctx, noteID, score, status)
	return args.Error(0)
}

// UpdateCommunityNoteAnalysis mocks the UpdateCommunityNoteAnalysis method
func (m *MockStorage) UpdateCommunityNoteAnalysis(ctx context.Context, noteID string, sentiment, objectivity, sourceQuality float64) error {
	args := m.Called(ctx, noteID, sentiment, objectivity, sourceQuality)
	return args.Error(0)
}

// CreateCommunityNoteVote mocks the CreateCommunityNoteVote method
func (m *MockStorage) CreateCommunityNoteVote(ctx context.Context, vote *storage.CommunityNoteVote) error {
	args := m.Called(ctx, vote)
	return args.Error(0)
}

// GetCommunityNoteVotes mocks the GetCommunityNoteVotes method
func (m *MockStorage) GetCommunityNoteVotes(ctx context.Context, noteID string) ([]*storage.CommunityNoteVote, error) {
	args := m.Called(ctx, noteID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CommunityNoteVote), args.Error(1)
}

// GetUserCommunityNoteVotes mocks the GetUserCommunityNoteVotes method
func (m *MockStorage) GetUserCommunityNoteVotes(ctx context.Context, userID string, noteIDs []string) (map[string]*storage.CommunityNoteVote, error) {
	args := m.Called(ctx, userID, noteIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]*storage.CommunityNoteVote), args.Error(1)
}

// CreateCustomEmoji mocks the CreateCustomEmoji method
func (m *MockStorage) CreateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error {
	args := m.Called(ctx, emoji)
	return args.Error(0)
}

// GetCustomEmoji mocks the GetCustomEmoji method
func (m *MockStorage) GetCustomEmoji(ctx context.Context, shortcode string) (*storage.CustomEmoji, error) {
	args := m.Called(ctx, shortcode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.CustomEmoji), args.Error(1)
}

// GetCustomEmojis mocks the GetCustomEmojis method
func (m *MockStorage) GetCustomEmojis(ctx context.Context) ([]*storage.CustomEmoji, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CustomEmoji), args.Error(1)
}

// UpdateCustomEmoji mocks the UpdateCustomEmoji method
func (m *MockStorage) UpdateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error {
	args := m.Called(ctx, emoji)
	return args.Error(0)
}

// DeleteCustomEmoji mocks the DeleteCustomEmoji method
func (m *MockStorage) DeleteCustomEmoji(ctx context.Context, shortcode string) error {
	args := m.Called(ctx, shortcode)
	return args.Error(0)
}

// GetCustomEmojisByCategory mocks the GetCustomEmojisByCategory method
func (m *MockStorage) GetCustomEmojisByCategory(ctx context.Context, category string) ([]*storage.CustomEmoji, error) {
	args := m.Called(ctx, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CustomEmoji), args.Error(1)
}

// CreateDevice mocks the CreateDevice method
func (m *MockStorage) CreateDevice(ctx context.Context, device *storage.Device) error {
	args := m.Called(ctx, device)
	return args.Error(0)
}

// GetDevice mocks the GetDevice method
func (m *MockStorage) GetDevice(ctx context.Context, deviceID string) (*storage.Device, error) {
	args := m.Called(ctx, deviceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Device), args.Error(1)
}

// UpdateDevice mocks the UpdateDevice method
func (m *MockStorage) UpdateDevice(ctx context.Context, device *storage.Device) error {
	args := m.Called(ctx, device)
	return args.Error(0)
}

// GetUserDevices mocks the GetUserDevices method
func (m *MockStorage) GetUserDevices(ctx context.Context, username string) ([]*storage.Device, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Device), args.Error(1)
}

// GetStreamingPreferencesByDevice mocks the GetStreamingPreferencesByDevice method
func (m *MockStorage) GetStreamingPreferencesByDevice(ctx context.Context, username, deviceID string) (*storage.StreamingPreferences, error) {
	args := m.Called(ctx, username, deviceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StreamingPreferences), args.Error(1)
}

// UpdateDeviceStreamingPreferences mocks the UpdateDeviceStreamingPreferences method
func (m *MockStorage) UpdateDeviceStreamingPreferences(ctx context.Context, prefs *storage.StreamingPreferences, deviceID string) error {
	args := m.Called(ctx, prefs, deviceID)
	return args.Error(0)
}

// CreateDomainAllow mocks the CreateDomainAllow method
func (m *MockStorage) CreateDomainAllow(ctx context.Context, allow *storage.DomainAllow) error {
	args := m.Called(ctx, allow)
	return args.Error(0)
}

// GetDomainAllows mocks the GetDomainAllows method
func (m *MockStorage) GetDomainAllows(ctx context.Context, limit int, cursor string) ([]*storage.DomainAllow, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.DomainAllow), args.String(1), args.Error(2)
}

// DeleteDomainAllow mocks the DeleteDomainAllow method
func (m *MockStorage) DeleteDomainAllow(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// CreateFeaturedTag mocks the CreateFeaturedTag method
func (m *MockStorage) CreateFeaturedTag(ctx context.Context, userID string, tagName string) (*storage.FeaturedTag, error) {
	args := m.Called(ctx, userID, tagName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.FeaturedTag), args.Error(1)
}

// DeleteFeaturedTag mocks the DeleteFeaturedTag method
func (m *MockStorage) DeleteFeaturedTag(ctx context.Context, userID string, featuredTagID string) error {
	args := m.Called(ctx, userID, featuredTagID)
	return args.Error(0)
}

// GetFeaturedTags mocks the GetFeaturedTags method
func (m *MockStorage) GetFeaturedTags(ctx context.Context, userID string) ([]*storage.FeaturedTag, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FeaturedTag), args.Error(1)
}

// CreateMute mocks the CreateMute method
func (m *MockStorage) CreateMute(ctx context.Context, mute *storage.Mute) error {
	args := m.Called(ctx, mute)
	return args.Error(0)
}

// GetMute mocks the GetMute method
func (m *MockStorage) GetMute(ctx context.Context, actor, mutedActor string) (*storage.Mute, error) {
	args := m.Called(ctx, actor, mutedActor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Mute), args.Error(1)
}

// DeleteMute mocks the DeleteMute method
func (m *MockStorage) DeleteMute(ctx context.Context, actor, mutedActor string) error {
	args := m.Called(ctx, actor, mutedActor)
	return args.Error(0)
}

// GetMutedActors mocks the GetMutedActors method
func (m *MockStorage) GetMutedActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Mute, string, error) {
	args := m.Called(ctx, actor, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Mute), args.String(1), args.Error(2)
}

// IsMuted mocks the IsMuted method
func (m *MockStorage) IsMuted(ctx context.Context, actor, targetActor string) (bool, error) {
	args := m.Called(ctx, actor, targetActor)
	return args.Bool(0), args.Error(1)
}

// UpdateHashtagNotificationSettings mocks the UpdateHashtagNotificationSettings method
func (m *MockStorage) UpdateHashtagNotificationSettings(ctx context.Context, userID, hashtag string, notify bool) error {
	args := m.Called(ctx, userID, hashtag, notify)
	return args.Error(0)
}

// MuteHashtag mocks the MuteHashtag method
func (m *MockStorage) MuteHashtag(ctx context.Context, userID, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

// UnmuteHashtag mocks the UnmuteHashtag method
func (m *MockStorage) UnmuteHashtag(ctx context.Context, userID, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

// IsHashtagMuted mocks the IsHashtagMuted method
func (m *MockStorage) IsHashtagMuted(ctx context.Context, userID, hashtag string) (bool, error) {
	args := m.Called(ctx, userID, hashtag)
	return args.Bool(0), args.Error(1)
}

// IsNotificationMuted mocks the IsNotificationMuted method
func (m *MockStorage) IsNotificationMuted(ctx context.Context, userID, targetID string) (bool, error) {
	args := m.Called(ctx, userID, targetID)
	return args.Bool(0), args.Error(1)
}

// ==================== Poll Operations ====================

// CreatePoll mocks the CreatePoll method
func (m *MockStorage) CreatePoll(ctx context.Context, poll *storage.Poll) error {
	args := m.Called(ctx, poll)
	return args.Error(0)
}

// GetPoll mocks the GetPoll method
func (m *MockStorage) GetPoll(ctx context.Context, pollID string) (*storage.Poll, error) {
	args := m.Called(ctx, pollID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Poll), args.Error(1)
}

// GetPollByStatusID mocks the GetPollByStatusID method
func (m *MockStorage) GetPollByStatusID(ctx context.Context, statusID string) (*storage.Poll, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Poll), args.Error(1)
}

// VoteOnPoll mocks the VoteOnPoll method
func (m *MockStorage) VoteOnPoll(ctx context.Context, pollID string, voterID string, choices []int) error {
	args := m.Called(ctx, pollID, voterID, choices)
	return args.Error(0)
}

// GetPollVotes mocks the GetPollVotes method
func (m *MockStorage) GetPollVotes(ctx context.Context, pollID string) (map[string][]int, error) {
	args := m.Called(ctx, pollID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string][]int), args.Error(1)
}

// ==================== Push Subscription Operations ====================

// CreatePushSubscription mocks the CreatePushSubscription method
func (m *MockStorage) CreatePushSubscription(ctx context.Context, username string, subscription *storage.PushSubscription) error {
	args := m.Called(ctx, username, subscription)
	return args.Error(0)
}

// GetPushSubscription mocks the GetPushSubscription method
func (m *MockStorage) GetPushSubscription(ctx context.Context, username, subscriptionID string) (*storage.PushSubscription, error) {
	args := m.Called(ctx, username, subscriptionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.PushSubscription), args.Error(1)
}

// GetUserPushSubscriptions mocks the GetUserPushSubscriptions method
func (m *MockStorage) GetUserPushSubscriptions(ctx context.Context, username string) ([]*storage.PushSubscription, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.PushSubscription), args.Error(1)
}

// UpdatePushSubscription mocks the UpdatePushSubscription method
func (m *MockStorage) UpdatePushSubscription(ctx context.Context, username, subscriptionID string, alerts storage.PushSubscriptionAlerts) error {
	args := m.Called(ctx, username, subscriptionID, alerts)
	return args.Error(0)
}

// DeletePushSubscription mocks the DeletePushSubscription method
func (m *MockStorage) DeletePushSubscription(ctx context.Context, username, subscriptionID string) error {
	args := m.Called(ctx, username, subscriptionID)
	return args.Error(0)
}

// DeleteAllPushSubscriptions mocks the DeleteAllPushSubscriptions method
func (m *MockStorage) DeleteAllPushSubscriptions(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// ==================== Scheduled Status Operations ====================

// CreateScheduledStatus mocks the CreateScheduledStatus method
func (m *MockStorage) CreateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	args := m.Called(ctx, scheduled)
	return args.Error(0)
}

// GetScheduledStatus mocks the GetScheduledStatus method
func (m *MockStorage) GetScheduledStatus(ctx context.Context, id string) (*storage.ScheduledStatus, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ScheduledStatus), args.Error(1)
}

// GetScheduledStatuses mocks the GetScheduledStatuses method
func (m *MockStorage) GetScheduledStatuses(ctx context.Context, username string, limit int, cursor string) ([]*storage.ScheduledStatus, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.ScheduledStatus), args.String(1), args.Error(2)
}

// UpdateScheduledStatus mocks the UpdateScheduledStatus method
func (m *MockStorage) UpdateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	args := m.Called(ctx, scheduled)
	return args.Error(0)
}

// DeleteScheduledStatus mocks the DeleteScheduledStatus method
func (m *MockStorage) DeleteScheduledStatus(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// GetDueScheduledStatuses mocks the GetDueScheduledStatuses method
func (m *MockStorage) GetDueScheduledStatuses(ctx context.Context, before time.Time, limit int) ([]*storage.ScheduledStatus, error) {
	args := m.Called(ctx, before, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.ScheduledStatus), args.Error(1)
}

// MarkScheduledStatusPublished mocks the MarkScheduledStatusPublished method
func (m *MockStorage) MarkScheduledStatusPublished(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// GetScheduledStatusMedia mocks the GetScheduledStatusMedia method
func (m *MockStorage) GetScheduledStatusMedia(ctx context.Context, statusID string) ([]any, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

// ==================== Session Operations ====================

// CreateSession mocks the CreateSession method
func (m *MockStorage) CreateSession(ctx context.Context, session *storage.Session) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

// GetSession mocks the GetSession method
func (m *MockStorage) GetSession(ctx context.Context, sessionID string) (*storage.Session, error) {
	args := m.Called(ctx, sessionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Session), args.Error(1)
}

// GetSessionByRefreshToken mocks the GetSessionByRefreshToken method
func (m *MockStorage) GetSessionByRefreshToken(ctx context.Context, refreshToken string) (*storage.Session, error) {
	args := m.Called(ctx, refreshToken)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Session), args.Error(1)
}

// UpdateSession mocks the UpdateSession method
func (m *MockStorage) UpdateSession(ctx context.Context, session *storage.Session) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

// DeleteSession mocks the DeleteSession method
func (m *MockStorage) DeleteSession(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

// GetUserSessions mocks the GetUserSessions method
func (m *MockStorage) GetUserSessions(ctx context.Context, username string) ([]*storage.Session, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Session), args.Error(1)
}

// ==================== Severed Relationship Operations ====================

// CreateSeveredRelationship mocks the CreateSeveredRelationship method
func (m *MockStorage) CreateSeveredRelationship(ctx context.Context, rel *storage.SeveredRelationship) error {
	args := m.Called(ctx, rel)
	return args.Error(0)
}

// GetSeveredRelationships mocks the GetSeveredRelationships method
func (m *MockStorage) GetSeveredRelationships(ctx context.Context, localInstance string, limit int, cursor string) ([]*storage.SeveredRelationship, string, error) {
	args := m.Called(ctx, localInstance, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.SeveredRelationship), args.String(1), args.Error(2)
}

// GetSeveredRelationship mocks the GetSeveredRelationship method
func (m *MockStorage) GetSeveredRelationship(ctx context.Context, localInstance, remoteInstance string) (*storage.SeveredRelationship, error) {
	args := m.Called(ctx, localInstance, remoteInstance)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SeveredRelationship), args.Error(1)
}

// UpdateSeveredRelationship mocks the UpdateSeveredRelationship method
func (m *MockStorage) UpdateSeveredRelationship(ctx context.Context, rel *storage.SeveredRelationship) error {
	args := m.Called(ctx, rel)
	return args.Error(0)
}

// ==================== Trust Relationship Operations ====================

// CreateTrustRelationship mocks the CreateTrustRelationship method
func (m *MockStorage) CreateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	args := m.Called(ctx, relationship)
	return args.Error(0)
}

// GetTrustRelationship mocks the GetTrustRelationship method
func (m *MockStorage) GetTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error) {
	args := m.Called(ctx, trusterID, trusteeID, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.TrustRelationship), args.Error(1)
}

// UpdateTrustRelationship mocks the UpdateTrustRelationship method
func (m *MockStorage) UpdateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	args := m.Called(ctx, relationship)
	return args.Error(0)
}

// DeleteTrustRelationship mocks the DeleteTrustRelationship method
func (m *MockStorage) DeleteTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) error {
	args := m.Called(ctx, trusterID, trusteeID, category)
	return args.Error(0)
}

// GetTrustRelationships mocks the GetTrustRelationships method
func (m *MockStorage) GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	args := m.Called(ctx, trusterID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TrustRelationship), args.String(1), args.Error(2)
}

// GetTrustedByRelationships mocks the GetTrustedByRelationships method
func (m *MockStorage) GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	args := m.Called(ctx, trusteeID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TrustRelationship), args.String(1), args.Error(2)
}

// GetTrustScore mocks the GetTrustScore method
func (m *MockStorage) GetTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error) {
	args := m.Called(ctx, actorID, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.TrustScore), args.Error(1)
}

// UpdateTrustScore mocks the UpdateTrustScore method
func (m *MockStorage) UpdateTrustScore(ctx context.Context, score *storage.TrustScore) error {
	args := m.Called(ctx, score)
	return args.Error(0)
}

// RecordTrustUpdate mocks the RecordTrustUpdate method
func (m *MockStorage) RecordTrustUpdate(ctx context.Context, update *storage.TrustUpdate) error {
	args := m.Called(ctx, update)
	return args.Error(0)
}

// GetAllTrustRelationships mocks the GetAllTrustRelationships method
func (m *MockStorage) GetAllTrustRelationships(ctx context.Context, limit int) ([]*storage.TrustRelationship, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrustRelationship), args.Error(1)
}

// StoreTrustee mocks the StoreTrustee method
func (m *MockStorage) StoreTrustee(ctx context.Context, username string, trustee *storage.TrusteeConfig) error {
	args := m.Called(ctx, username, trustee)
	return args.Error(0)
}

// GetTrustees mocks the GetTrustees method
func (m *MockStorage) GetTrustees(ctx context.Context, username string) ([]*storage.TrusteeConfig, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrusteeConfig), args.Error(1)
}

// DeleteTrustee mocks the DeleteTrustee method
func (m *MockStorage) DeleteTrustee(ctx context.Context, username, trusteeActorID string) error {
	args := m.Called(ctx, username, trusteeActorID)
	return args.Error(0)
}

// UpdateTrusteeConfirmed mocks the UpdateTrusteeConfirmed method
func (m *MockStorage) UpdateTrusteeConfirmed(ctx context.Context, username, trusteeActorID string, confirmed bool) error {
	args := m.Called(ctx, username, trusteeActorID, confirmed)
	return args.Error(0)
}

// GetUserTrustScore mocks the GetUserTrustScore method
func (m *MockStorage) GetUserTrustScore(ctx context.Context, userID string) (float64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(float64), args.Error(1)
}

// ==================== Vouch Operations ====================

// CreateVouch mocks the CreateVouch method
func (m *MockStorage) CreateVouch(ctx context.Context, vouch *storage.Vouch) error {
	args := m.Called(ctx, vouch)
	return args.Error(0)
}

// GetVouch mocks the GetVouch method
func (m *MockStorage) GetVouch(ctx context.Context, vouchID string) (*storage.Vouch, error) {
	args := m.Called(ctx, vouchID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Vouch), args.Error(1)
}

// GetVouchesByActor mocks the GetVouchesByActor method
func (m *MockStorage) GetVouchesByActor(ctx context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	args := m.Called(ctx, actorID, activeOnly)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Vouch), args.Error(1)
}

// GetVouchesForActor mocks the GetVouchesForActor method
func (m *MockStorage) GetVouchesForActor(ctx context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	args := m.Called(ctx, actorID, activeOnly)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Vouch), args.Error(1)
}

// UpdateVouchStatus mocks the UpdateVouchStatus method
func (m *MockStorage) UpdateVouchStatus(ctx context.Context, vouchID string, active bool, revokedAt *time.Time) error {
	args := m.Called(ctx, vouchID, active, revokedAt)
	return args.Error(0)
}

// GetMonthlyVouchCount mocks the GetMonthlyVouchCount method
func (m *MockStorage) GetMonthlyVouchCount(ctx context.Context, actorID string, year int, month time.Month) (int, error) {
	args := m.Called(ctx, actorID, year, month)
	return args.Int(0), args.Error(1)
}

// ==================== Timeline Cleanup Operations ====================

// DeleteExpiredTimelineEntries mocks the DeleteExpiredTimelineEntries method
func (m *MockStorage) DeleteExpiredTimelineEntries(ctx context.Context, before time.Time) error {
	args := m.Called(ctx, before)
	return args.Error(0)
}

// DeleteFromTimeline mocks the DeleteFromTimeline method
func (m *MockStorage) DeleteFromTimeline(ctx context.Context, timelineType, timelineID, entryID string) error {
	args := m.Called(ctx, timelineType, timelineID, entryID)
	return args.Error(0)
}

// DeleteOldHashtagTrends mocks the DeleteOldHashtagTrends method
func (m *MockStorage) DeleteOldHashtagTrends(ctx context.Context, before time.Time) error {
	args := m.Called(ctx, before)
	return args.Error(0)
}

// StoreHashtagTrend mocks the StoreHashtagTrend method
func (m *MockStorage) StoreHashtagTrend(ctx context.Context, trend any) error {
	args := m.Called(ctx, trend)
	return args.Error(0)
}

// DeleteOldLinkTrends mocks the DeleteOldLinkTrends method
func (m *MockStorage) DeleteOldLinkTrends(ctx context.Context, before time.Time) error {
	args := m.Called(ctx, before)
	return args.Error(0)
}

// StoreLinkTrend mocks the StoreLinkTrend method
func (m *MockStorage) StoreLinkTrend(ctx context.Context, trend any) error {
	args := m.Called(ctx, trend)
	return args.Error(0)
}

// DeleteOldStatusTrends mocks the DeleteOldStatusTrends method
func (m *MockStorage) DeleteOldStatusTrends(ctx context.Context, before time.Time) error {
	args := m.Called(ctx, before)
	return args.Error(0)
}

// StoreStatusTrend mocks the StoreStatusTrend method
func (m *MockStorage) StoreStatusTrend(ctx context.Context, trend any) error {
	args := m.Called(ctx, trend)
	return args.Error(0)
}

// ==================== Wallet Operations ====================

// StoreWalletChallenge mocks the StoreWalletChallenge method
func (m *MockStorage) StoreWalletChallenge(ctx context.Context, challenge *storage.WalletChallenge) error {
	args := m.Called(ctx, challenge)
	return args.Error(0)
}

// GetWalletChallenge mocks the GetWalletChallenge method
func (m *MockStorage) GetWalletChallenge(ctx context.Context, challengeID string) (*storage.WalletChallenge, error) {
	args := m.Called(ctx, challengeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.WalletChallenge), args.Error(1)
}

// DeleteWalletChallenge mocks the DeleteWalletChallenge method
func (m *MockStorage) DeleteWalletChallenge(ctx context.Context, challengeID string) error {
	args := m.Called(ctx, challengeID)
	return args.Error(0)
}

// StoreWalletCredential mocks the StoreWalletCredential method
func (m *MockStorage) StoreWalletCredential(ctx context.Context, credential *storage.WalletCredential) error {
	args := m.Called(ctx, credential)
	return args.Error(0)
}

// GetWalletCredential mocks the GetWalletCredential method
func (m *MockStorage) GetWalletCredential(ctx context.Context, walletType, address string) (*storage.WalletCredential, error) {
	args := m.Called(ctx, walletType, address)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.WalletCredential), args.Error(1)
}

// GetUserWalletCredentials mocks the GetUserWalletCredentials method
func (m *MockStorage) GetUserWalletCredentials(ctx context.Context, username string) ([]*storage.WalletCredential, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.WalletCredential), args.Error(1)
}

// DeleteWalletCredential mocks the DeleteWalletCredential method
func (m *MockStorage) DeleteWalletCredential(ctx context.Context, username, address string) error {
	args := m.Called(ctx, username, address)
	return args.Error(0)
}

// UpdateWalletLastUsed mocks the UpdateWalletLastUsed method
func (m *MockStorage) UpdateWalletLastUsed(ctx context.Context, username, address string) error {
	args := m.Called(ctx, username, address)
	return args.Error(0)
}

// ==================== WebAuthn Operations ====================

// StoreWebAuthnCredential mocks the StoreWebAuthnCredential method
func (m *MockStorage) StoreWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error {
	args := m.Called(ctx, credential)
	return args.Error(0)
}

// GetWebAuthnCredential mocks the GetWebAuthnCredential method
func (m *MockStorage) GetWebAuthnCredential(ctx context.Context, credentialID string) (*storage.WebAuthnCredential, error) {
	args := m.Called(ctx, credentialID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.WebAuthnCredential), args.Error(1)
}

// GetUserWebAuthnCredentials mocks the GetUserWebAuthnCredentials method
func (m *MockStorage) GetUserWebAuthnCredentials(ctx context.Context, username string) ([]*storage.WebAuthnCredential, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.WebAuthnCredential), args.Error(1)
}

// UpdateWebAuthnCredential mocks the UpdateWebAuthnCredential method
func (m *MockStorage) UpdateWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error {
	args := m.Called(ctx, credential)
	return args.Error(0)
}

// DeleteWebAuthnCredential mocks the DeleteWebAuthnCredential method
func (m *MockStorage) DeleteWebAuthnCredential(ctx context.Context, credentialID string) error {
	args := m.Called(ctx, credentialID)
	return args.Error(0)
}

// StoreWebAuthnChallenge mocks the StoreWebAuthnChallenge method
func (m *MockStorage) StoreWebAuthnChallenge(ctx context.Context, challenge *storage.WebAuthnChallenge) error {
	args := m.Called(ctx, challenge)
	return args.Error(0)
}

// GetWebAuthnChallenge mocks the GetWebAuthnChallenge method
func (m *MockStorage) GetWebAuthnChallenge(ctx context.Context, challengeID string) (*storage.WebAuthnChallenge, error) {
	args := m.Called(ctx, challengeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.WebAuthnChallenge), args.Error(1)
}

// DeleteWebAuthnChallenge mocks the DeleteWebAuthnChallenge method
func (m *MockStorage) DeleteWebAuthnChallenge(ctx context.Context, challengeID string) error {
	args := m.Called(ctx, challengeID)
	return args.Error(0)
}

// ==================== Fan Out Operations ====================

// FanOutPost mocks the FanOutPost method
func (m *MockStorage) FanOutPost(ctx context.Context, activity *activitypub.Activity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

// ==================== Hashtag Follow Operations ====================

// FollowHashtag mocks the FollowHashtag method
func (m *MockStorage) FollowHashtag(ctx context.Context, userID string, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

// UnfollowHashtag mocks the UnfollowHashtag method
func (m *MockStorage) UnfollowHashtag(ctx context.Context, userID string, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

// IsFollowingHashtag mocks the IsFollowingHashtag method
func (m *MockStorage) IsFollowingHashtag(ctx context.Context, userID string, hashtag string) (bool, error) {
	args := m.Called(ctx, userID, hashtag)
	return args.Bool(0), args.Error(1)
}

// GetFollowedHashtags mocks the GetFollowedHashtags method
func (m *MockStorage) GetFollowedHashtags(ctx context.Context, userID string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, userID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

// GenerateSearchSuggestions mocks the GenerateSearchSuggestions method
func (m *MockStorage) GenerateSearchSuggestions(ctx context.Context, userID, partialQuery string, limit int) ([]string, error) {
	args := m.Called(ctx, userID, partialQuery, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// GetAccountSuggestions mocks the GetAccountSuggestions method
func (m *MockStorage) GetAccountSuggestions(ctx context.Context, userID string, limit int) ([]*activitypub.Actor, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*activitypub.Actor), args.Error(1)
}

// RemoveAccountSuggestion mocks the RemoveAccountSuggestion method
func (m *MockStorage) RemoveAccountSuggestion(ctx context.Context, userID, targetID string) error {
	args := m.Called(ctx, userID, targetID)
	return args.Error(0)
}

// ==================== Relay Operations ====================

// StoreRelayInfo mocks the StoreRelayInfo method
func (m *MockStorage) StoreRelayInfo(ctx context.Context, relay *storage.RelayInfo) error {
	args := m.Called(ctx, relay)
	return args.Error(0)
}

// GetRelayInfo mocks the GetRelayInfo method
func (m *MockStorage) GetRelayInfo(ctx context.Context, relayURL string) (*storage.RelayInfo, error) {
	args := m.Called(ctx, relayURL)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.RelayInfo), args.Error(1)
}

// RemoveRelayInfo mocks the RemoveRelayInfo method
func (m *MockStorage) RemoveRelayInfo(ctx context.Context, relayURL string) error {
	args := m.Called(ctx, relayURL)
	return args.Error(0)
}

// GetActiveRelays mocks the GetActiveRelays method
func (m *MockStorage) GetActiveRelays(ctx context.Context) ([]*storage.RelayInfo, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.RelayInfo), args.Error(1)
}

// GetAllRelays mocks the GetAllRelays method
func (m *MockStorage) GetAllRelays(ctx context.Context, limit int, cursor string) ([]*storage.RelayInfo, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.RelayInfo), args.String(1), args.Error(2)
}

// UpdateRelayStatus mocks the UpdateRelayStatus method
func (m *MockStorage) UpdateRelayStatus(ctx context.Context, relayURL string, active bool) error {
	args := m.Called(ctx, relayURL, active)
	return args.Error(0)
}

// ==================== Affected Relationship Operations ====================

// GetAffectedRelationships mocks the GetAffectedRelationships method
func (m *MockStorage) GetAffectedRelationships(ctx context.Context, userID, domain string) ([]*storage.RelationshipRecord, error) {
	args := m.Called(ctx, userID, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.RelationshipRecord), args.Error(1)
}

// GetAffectedFollows mocks the GetAffectedFollows method
func (m *MockStorage) GetAffectedFollows(ctx context.Context, localInstance, remoteInstance string) ([]storage.AffectedFollow, error) {
	args := m.Called(ctx, localInstance, remoteInstance)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.AffectedFollow), args.Error(1)
}

// RecordAffectedFollow mocks the RecordAffectedFollow method
func (m *MockStorage) RecordAffectedFollow(ctx context.Context, localInstance, remoteInstance string, follow storage.AffectedFollow) error {
	args := m.Called(ctx, localInstance, remoteInstance, follow)
	return args.Error(0)
}

// ReverseSeverance mocks the ReverseSeverance method
func (m *MockStorage) ReverseSeverance(ctx context.Context, localInstance, remoteInstance string) error {
	args := m.Called(ctx, localInstance, remoteInstance)
	return args.Error(0)
}

// ==================== Preference Operations ====================

// GetUserLanguagePreference mocks the GetUserLanguagePreference method
func (m *MockStorage) GetUserLanguagePreference(ctx context.Context, username string) (string, error) {
	args := m.Called(ctx, username)
	return args.String(0), args.Error(1)
}

// SetUserLanguagePreference mocks the SetUserLanguagePreference method
func (m *MockStorage) SetUserLanguagePreference(ctx context.Context, username string, language string) error {
	args := m.Called(ctx, username, language)
	return args.Error(0)
}

// GetUserPreferences mocks the GetUserPreferences method
func (m *MockStorage) GetUserPreferences(ctx context.Context, username string) (*storage.UserPreferences, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.UserPreferences), args.Error(1)
}

// UpdateUserPreferences mocks the UpdateUserPreferences method
func (m *MockStorage) UpdateUserPreferences(ctx context.Context, username string, preferences *storage.UserPreferences) error {
	args := m.Called(ctx, username, preferences)
	return args.Error(0)
}

// SetPreference mocks the SetPreference method
func (m *MockStorage) SetPreference(ctx context.Context, username string, key string, value any) error {
	args := m.Called(ctx, username, key, value)
	return args.Error(0)
}

// GetPreference mocks the GetPreference method
func (m *MockStorage) GetPreference(ctx context.Context, username string, key string) (any, error) {
	args := m.Called(ctx, username, key)
	return args.Get(0), args.Error(1)
}

// GetAllPreferences mocks the GetAllPreferences method
func (m *MockStorage) GetAllPreferences(ctx context.Context, username string) (map[string]any, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]any), args.Error(1)
}

// UpdatePreferences mocks the UpdatePreferences method
func (m *MockStorage) UpdatePreferences(ctx context.Context, username string, prefs map[string]any) error {
	args := m.Called(ctx, username, prefs)
	return args.Error(0)
}

// GetStreamingPreferences mocks the GetStreamingPreferences method
func (m *MockStorage) GetStreamingPreferences(ctx context.Context, username string) (*storage.StreamingPreferences, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StreamingPreferences), args.Error(1)
}

// UpdateStreamingPreferences mocks the UpdateStreamingPreferences method
func (m *MockStorage) UpdateStreamingPreferences(ctx context.Context, prefs *storage.StreamingPreferences) error {
	args := m.Called(ctx, prefs)
	return args.Error(0)
}

// GetStreamingPreferenceHistory mocks the GetStreamingPreferenceHistory method
func (m *MockStorage) GetStreamingPreferenceHistory(ctx context.Context, username string, limit int) ([]*storage.StreamingPreferences, error) {
	args := m.Called(ctx, username, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StreamingPreferences), args.Error(1)
}

// SyncStreamingPreferences mocks the SyncStreamingPreferences method
func (m *MockStorage) SyncStreamingPreferences(ctx context.Context, username string, sourceDeviceID string) error {
	args := m.Called(ctx, username, sourceDeviceID)
	return args.Error(0)
}

// ResolvePreferenceConflict mocks the ResolvePreferenceConflict method
func (m *MockStorage) ResolvePreferenceConflict(ctx context.Context, username string, strategy storage.ConflictResolutionStrategy) (*storage.StreamingPreferences, error) {
	args := m.Called(ctx, username, strategy)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StreamingPreferences), args.Error(1)
}

// ==================== Boost Operations ====================

// GetBoostCount mocks the GetBoostCount method
func (m *MockStorage) GetBoostCount(ctx context.Context, statusID string) (int64, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0).(int64), args.Error(1)
}

// ==================== Cost Operations ====================

// GetCostProjections mocks the GetCostProjections method
func (m *MockStorage) GetCostProjections(ctx context.Context, period string) (*storage.CostProjection, error) {
	args := m.Called(ctx, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.CostProjection), args.Error(1)
}

// ==================== User Count Operations ====================

// GetDailyActiveUserCount mocks the GetDailyActiveUserCount method
func (m *MockStorage) GetDailyActiveUserCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

// GetDirectTimeline mocks the GetDirectTimeline method
func (m *MockStorage) GetDirectTimeline(ctx context.Context, username string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TimelineEntry), args.String(1), args.Error(2)
}

// GetDomainStats mocks the GetDomainStats method
func (m *MockStorage) GetDomainStats(ctx context.Context, domain string) (any, error) {
	args := m.Called(ctx, domain)
	return args.Get(0), args.Error(1)
}

// ==================== Engagement Operations ====================

// RecordStatusEngagement mocks the RecordStatusEngagement method
func (m *MockStorage) RecordStatusEngagement(ctx context.Context, statusID string, engagementType string, userID string) error {
	args := m.Called(ctx, statusID, engagementType, userID)
	return args.Error(0)
}

// StoreEngagementMetrics mocks the StoreEngagementMetrics method
func (m *MockStorage) StoreEngagementMetrics(ctx context.Context, metrics *storage.EngagementMetrics) error {
	args := m.Called(ctx, metrics)
	return args.Error(0)
}

// IndexByEngagement mocks the IndexByEngagement method
func (m *MockStorage) IndexByEngagement(ctx context.Context, statusID string, bucket string) error {
	args := m.Called(ctx, statusID, bucket)
	return args.Error(0)
}

// GetEngagementMetrics mocks the GetEngagementMetrics method
func (m *MockStorage) GetEngagementMetrics(ctx context.Context, statusID string) (*storage.EngagementMetrics, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.EngagementMetrics), args.Error(1)
}

// GetRecentStatusesWithEngagement mocks the GetRecentStatusesWithEngagement method
func (m *MockStorage) GetRecentStatusesWithEngagement(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingStatus), args.Error(1)
}

// GetFieldVerification mocks the GetFieldVerification method
func (m *MockStorage) GetFieldVerification(ctx context.Context, username, fieldName string) (*storage.ActorField, error) {
	args := m.Called(ctx, username, fieldName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ActorField), args.Error(1)
}

// GetFollowersCount mocks the GetFollowersCount method
func (m *MockStorage) GetFollowersCount(ctx context.Context, actorID string) (int, error) {
	args := m.Called(ctx, actorID)
	return args.Int(0), args.Error(1)
}

// GetFollowingCount mocks the GetFollowingCount method
func (m *MockStorage) GetFollowingCount(ctx context.Context, actorID string) (int, error) {
	args := m.Called(ctx, actorID)
	return args.Int(0), args.Error(1)
}

// GetHashtagActivity mocks the GetHashtagActivity method
func (m *MockStorage) GetHashtagActivity(ctx context.Context, hashtag string, since time.Time) ([]*storage.Activity, error) {
	args := m.Called(ctx, hashtag, since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Activity), args.Error(1)
}

// GetHashtagInfo mocks the GetHashtagInfo method
func (m *MockStorage) GetHashtagInfo(ctx context.Context, hashtag string) (*storage.Hashtag, error) {
	args := m.Called(ctx, hashtag)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Hashtag), args.Error(1)
}

// GetHashtagStats mocks the GetHashtagStats method
func (m *MockStorage) GetHashtagStats(ctx context.Context, hashtag string) (any, error) {
	args := m.Called(ctx, hashtag)
	return args.Get(0), args.Error(1)
}

// GetHashtagTimeline mocks the GetHashtagTimeline method
func (m *MockStorage) GetHashtagTimeline(ctx context.Context, hashtag string, local bool, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, hashtag, local, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TimelineEntry), args.String(1), args.Error(2)
}

// GetHashtagTimelineAdvanced mocks the GetHashtagTimelineAdvanced method
func (m *MockStorage) GetHashtagTimelineAdvanced(ctx context.Context, hashtag string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, hashtag, maxID, limit, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// GetHashtagUsageHistory mocks the GetHashtagUsageHistory method
func (m *MockStorage) GetHashtagUsageHistory(ctx context.Context, hashtag string, days int) ([]int64, error) {
	args := m.Called(ctx, hashtag, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]int64), args.Error(1)
}

// GetHomeTimeline mocks the GetHomeTimeline method
func (m *MockStorage) GetHomeTimeline(ctx context.Context, username string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TimelineEntry), args.String(1), args.Error(2)
}

// ==================== Instance Connection Operations ====================

// GetInstanceConnections mocks the GetInstanceConnections method
func (m *MockStorage) GetInstanceConnections(ctx context.Context, domain string, connectionType string) ([]*storage.InstanceConnection, error) {
	args := m.Called(ctx, domain, connectionType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.InstanceConnection), args.Error(1)
}

// GetRecentInstanceConnections mocks the GetRecentInstanceConnections method
func (m *MockStorage) GetRecentInstanceConnections(ctx context.Context, domain string, since time.Duration) ([]*storage.InstanceConnection, error) {
	args := m.Called(ctx, domain, since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.InstanceConnection), args.Error(1)
}

// GetInstanceInfo mocks the GetInstanceInfo method
func (m *MockStorage) GetInstanceInfo(ctx context.Context, domain string) (*storage.InstanceInfo, error) {
	args := m.Called(ctx, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceInfo), args.Error(1)
}

// UpsertInstanceInfo mocks the UpsertInstanceInfo method
func (m *MockStorage) UpsertInstanceInfo(ctx context.Context, info *storage.InstanceInfo) error {
	args := m.Called(ctx, info)
	return args.Error(0)
}

// GetInstanceMetadata mocks the GetInstanceMetadata method
func (m *MockStorage) GetInstanceMetadata(ctx context.Context, domain string) (*storage.InstanceMetadata, error) {
	args := m.Called(ctx, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceMetadata), args.Error(1)
}

// UpdateInstanceMetadata mocks the UpdateInstanceMetadata method
func (m *MockStorage) UpdateInstanceMetadata(ctx context.Context, metadata *storage.InstanceMetadata) error {
	args := m.Called(ctx, metadata)
	return args.Error(0)
}

// GetInstanceRules mocks the GetInstanceRules method
func (m *MockStorage) GetInstanceRules(ctx context.Context) ([]storage.InstanceRule, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.InstanceRule), args.Error(1)
}

// SetInstanceRules mocks the SetInstanceRules method
func (m *MockStorage) SetInstanceRules(ctx context.Context, rules []storage.InstanceRule) error {
	args := m.Called(ctx, rules)
	return args.Error(0)
}

// GetKnownInstances mocks the GetKnownInstances method
func (m *MockStorage) GetKnownInstances(ctx context.Context, limit int, cursor string) ([]*storage.InstanceInfo, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.InstanceInfo), args.String(1), args.Error(2)
}

// GetLatestStatus mocks the GetLatestStatus method
func (m *MockStorage) GetLatestStatus(ctx context.Context, actorID string) (*storage.StatusSearchResult, error) {
	args := m.Called(ctx, actorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StatusSearchResult), args.Error(1)
}

// ==================== Extended Description Operations ====================

// GetExtendedDescription mocks the GetExtendedDescription method
func (m *MockStorage) GetExtendedDescription(ctx context.Context) (string, time.Time, error) {
	args := m.Called(ctx)
	return args.String(0), args.Get(1).(time.Time), args.Error(2)
}

// SetExtendedDescription mocks the SetExtendedDescription method
func (m *MockStorage) SetExtendedDescription(ctx context.Context, description string) error {
	args := m.Called(ctx, description)
	return args.Error(0)
}

// ==================== Timeline Write Operations ====================

// WriteToTimeline mocks the WriteToTimeline method
func (m *MockStorage) WriteToTimeline(ctx context.Context, timeline *storage.TimelineEntry) error {
	args := m.Called(ctx, timeline)
	return args.Error(0)
}

// WriteToTimelines mocks the WriteToTimelines method
func (m *MockStorage) WriteToTimelines(ctx context.Context, entries []*storage.TimelineEntry) error {
	args := m.Called(ctx, entries)
	return args.Error(0)
}

// GetConversations mocks the GetConversations method
func (m *MockStorage) GetConversations(ctx context.Context, username string, limit int, cursor string) ([]*models.Conversation, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Conversation), args.String(1), args.Error(2)
}

// RemoveFromTimelines mocks the RemoveFromTimelines method
func (m *MockStorage) RemoveFromTimelines(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// ListStatusesForAdmin mocks the ListStatusesForAdmin method
func (m *MockStorage) ListStatusesForAdmin(ctx context.Context, filter *repositories.StatusFilter, limit int, cursor string) ([]*models.Status, string, error) {
	args := m.Called(ctx, filter, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Status), args.String(1), args.Error(2)
}

// CountStatusesForAdmin mocks the CountStatusesForAdmin method
func (m *MockStorage) CountStatusesForAdmin(ctx context.Context, filter *repositories.StatusFilter) (int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).(int64), args.Error(1)
}

// RecordSearchAnalytics mocks the RecordSearchAnalytics method
func (m *MockStorage) RecordSearchAnalytics(ctx context.Context, analytics interface{}) error {
	args := m.Called(ctx, analytics)
	return args.Error(0)
}

// CheckRateLimit mocks the CheckRateLimit method
func (m *MockStorage) CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	args := m.Called(ctx, key, limit, window)
	return args.Get(0).(bool), args.Error(1)
}

// MockRepositoryStorage implements the RepositoryStorage interface for testing
type MockRepositoryStorage struct {
	mock.Mock
	accountRepo          *repositories.AccountRepository
	actorRepo            *repositories.ActorRepository
	objectRepo           *repositories.ObjectRepository
	activityRepo         *repositories.ActivityRepository
	timelineRepo         *repositories.TimelineRepository
	notificationRepo     *repositories.NotificationRepository
	likeRepo             *repositories.LikeRepository
	moderationRepo       *repositories.ModerationRepository
	listRepo             *repositories.ListRepository
	mediaRepo            *repositories.MediaRepository
	mediaMetadataRepo    *repositories.MediaMetadataRepository
	pollRepo             *repositories.PollRepository
	hashtagRepo          *repositories.HashtagRepository
	scheduledStatusRepo  *repositories.ScheduledStatusRepository
	announcementRepo     *repositories.AnnouncementRepository
	domainBlockRepo      *repositories.DomainBlockRepository
	relationshipRepo     *repositories.RelationshipRepository
	instanceRepo         *repositories.InstanceRepository
	federationRepo       *repositories.FederationRepository
	recoveryRepo         *repositories.RecoveryRepository
	conversationRepo     *repositories.ConversationRepository
	pushSubscriptionRepo *repositories.PushSubscriptionRepository
	analyticsRepo        *repositories.TrendingRepository
	socialRepo           *repositories.SocialRepository
	userRepo             *repositories.UserRepository
	statusRepo           *repositories.StatusRepository
	costRepo             *repositories.TrackingRepository
	trustRepo            *repositories.TrustRepository
	searchRepo           *repositories.SearchRepository
	relayRepo            *repositories.RelayRepository
	communityNoteRepo    *repositories.CommunityNoteRepository
	emojiRepo            *repositories.EmojiRepository
	rateLimitRepo        *repositories.RateLimitRepository
	markerRepo           *repositories.MarkerRepository
	featuredTagRepo      *repositories.FeaturedTagRepository
	aiRepo               *repositories.AIRepository
	exportRepo           *repositories.ExportRepository
	importRepo           *repositories.ImportRepository
	dlqRepo              *repositories.DLQRepository
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
	mediaMetadataRepo := repositories.NewMediaMetadataRepository(nil, "test-table", logger, nil)
	pollRepo := repositories.NewPollRepository(nil, "test-table", logger)
	hashtagRepo := repositories.NewHashtagRepository(nil, "test-table", logger, "test.example.com")
	scheduledStatusRepo := repositories.NewScheduledStatusRepository(nil, "test-table", logger)
	announcementRepo := repositories.NewAnnouncementRepository(nil, "test-table", logger)
	domainBlockRepo := repositories.NewDomainBlockRepository(nil, "test-table", logger)
	relationshipRepo := repositories.NewRelationshipRepository(nil, "test-table", logger)
	instanceRepo := repositories.NewInstanceRepository(nil, "test-table", logger)
	federationRepo := repositories.NewFederationRepository(nil, logger, config.Get())
	recoveryRepo := repositories.NewRecoveryRepository(nil, "test-table", logger)
	conversationRepo := repositories.NewConversationRepository(nil, logger)
	pushSubscriptionRepo := repositories.NewPushSubscriptionRepository(nil, "test-table", logger)
	analyticsRepo := repositories.NewTrendingRepository(nil, logger, nil)
	socialRepo := repositories.NewSocialRepository(nil, logger)
	userRepo := repositories.NewUserRepository(nil, "test-table", logger)
	statusRepo := repositories.NewStatusRepository(nil, "test-table", logger)
	costRepo := repositories.NewTrackingRepository(nil, "test-table", logger)
	trustRepo := repositories.NewTrustRepository(nil, "test-table", logger)
	searchRepo := repositories.NewSearchRepository(&dynamorm.DB{}, logger)
	relayRepo := repositories.NewRelayRepository(nil, "test-table", logger)
	markerRepo := repositories.NewMarkerRepository(nil, "test-table", logger)
	featuredTagRepo := repositories.NewFeaturedTagRepository(nil, "test-table", logger)
	aiRepo := repositories.NewAIRepository(nil, "test-table", logger)
	exportRepo := repositories.NewExportRepository(nil, "test-table", logger)
	importRepo := repositories.NewImportRepository(nil, "test-table", logger)
	dlqRepo := repositories.NewDLQRepositorySimple(nil, "test-table", logger)
	emojiRepo := repositories.NewEmojiRepository(nil, logger)
	rateLimitRepo := repositories.NewRateLimitRepository(nil, "test-table", logger)

	return &MockRepositoryStorage{
		accountRepo:          accountRepo,
		actorRepo:            actorRepo,
		objectRepo:           objectRepo,
		activityRepo:         activityRepo,
		timelineRepo:         timelineRepo,
		notificationRepo:     notificationRepo,
		likeRepo:             likeRepo,
		moderationRepo:       moderationRepo,
		listRepo:             listRepo,
		mediaRepo:            mediaRepo,
		mediaMetadataRepo:    mediaMetadataRepo,
		pollRepo:             pollRepo,
		hashtagRepo:          hashtagRepo,
		scheduledStatusRepo:  scheduledStatusRepo,
		announcementRepo:     announcementRepo,
		domainBlockRepo:      domainBlockRepo,
		relationshipRepo:     relationshipRepo,
		instanceRepo:         instanceRepo,
		federationRepo:       federationRepo,
		recoveryRepo:         recoveryRepo,
		conversationRepo:     conversationRepo,
		pushSubscriptionRepo: pushSubscriptionRepo,
		analyticsRepo:        analyticsRepo,
		socialRepo:           socialRepo,
		userRepo:             userRepo,
		statusRepo:           statusRepo,
		costRepo:             costRepo,
		trustRepo:            trustRepo,
		searchRepo:           searchRepo,
		relayRepo:            relayRepo,
		communityNoteRepo:    repositories.NewCommunityNoteRepository(&dynamorm.DB{}, "test-table", logger),
		emojiRepo:            emojiRepo,
		rateLimitRepo:        rateLimitRepo,
		markerRepo:           markerRepo,
		featuredTagRepo:      featuredTagRepo,
		aiRepo:               aiRepo,
		exportRepo:           exportRepo,
		importRepo:           importRepo,
		dlqRepo:              dlqRepo,
	}
}

// Account returns the mock account repository
func (m *MockRepositoryStorage) Account() *repositories.AccountRepository {
	return m.accountRepo
}

// Actor returns the mock actor repository
func (m *MockRepositoryStorage) Actor() *repositories.ActorRepository {
	return m.actorRepo
}

// Object returns the mock object repository
func (m *MockRepositoryStorage) Object() *repositories.ObjectRepository {
	return m.objectRepo
}

// Activity returns the mock activity repository
func (m *MockRepositoryStorage) Activity() *repositories.ActivityRepository {
	return m.activityRepo
}

// Timeline returns the mock timeline repository
func (m *MockRepositoryStorage) Timeline() *repositories.TimelineRepository {
	return m.timelineRepo
}

// Notification returns the mock notification repository
func (m *MockRepositoryStorage) Notification() *repositories.NotificationRepository {
	return m.notificationRepo
}

// Like returns the mock like repository
func (m *MockRepositoryStorage) Like() *repositories.LikeRepository {
	return m.likeRepo
}

// Moderation returns the mock moderation repository
func (m *MockRepositoryStorage) Moderation() *repositories.ModerationRepository {
	return m.moderationRepo
}

// List returns the mock list repository
func (m *MockRepositoryStorage) List() *repositories.ListRepository {
	return m.listRepo
}

// Media returns the mock media repository
func (m *MockRepositoryStorage) Media() *repositories.MediaRepository {
	return m.mediaRepo
}

// MediaMetadata returns the mock media metadata repository
func (m *MockRepositoryStorage) MediaMetadata() *repositories.MediaMetadataRepository {
	return m.mediaMetadataRepo
}

// Poll returns the mock poll repository
func (m *MockRepositoryStorage) Poll() *repositories.PollRepository {
	return m.pollRepo
}

// Hashtag returns the mock hashtag repository
func (m *MockRepositoryStorage) Hashtag() *repositories.HashtagRepository {
	return m.hashtagRepo
}

// ScheduledStatus returns the mock scheduled status repository
func (m *MockRepositoryStorage) ScheduledStatus() *repositories.ScheduledStatusRepository {
	return m.scheduledStatusRepo
}

// Announcement returns the mock announcement repository
func (m *MockRepositoryStorage) Announcement() *repositories.AnnouncementRepository {
	return m.announcementRepo
}

// DomainBlock returns the mock domain block repository
func (m *MockRepositoryStorage) DomainBlock() *repositories.DomainBlockRepository {
	return m.domainBlockRepo
}

// Relationship returns the mock relationship repository
func (m *MockRepositoryStorage) Relationship() *repositories.RelationshipRepository {
	return m.relationshipRepo
}

// Instance returns the mock instance repository
func (m *MockRepositoryStorage) Instance() *repositories.InstanceRepository {
	return m.instanceRepo
}

// Federation returns the mock federation repository
func (m *MockRepositoryStorage) Federation() *repositories.FederationRepository {
	return m.federationRepo
}

// Recovery returns the mock recovery repository
func (m *MockRepositoryStorage) Recovery() *repositories.RecoveryRepository {
	return m.recoveryRepo
}

// Conversation returns the mock conversation repository
func (m *MockRepositoryStorage) Conversation() *repositories.ConversationRepository {
	return m.conversationRepo
}

// PushSubscription returns the mock push subscription repository
func (m *MockRepositoryStorage) PushSubscription() *repositories.PushSubscriptionRepository {
	return m.pushSubscriptionRepo
}

// Analytics returns the mock analytics repository
func (m *MockRepositoryStorage) Analytics() *repositories.TrendingRepository {
	return m.analyticsRepo
}

// Social returns the mock social repository
func (m *MockRepositoryStorage) Social() *repositories.SocialRepository {
	return m.socialRepo
}

// User returns the mock user repository
func (m *MockRepositoryStorage) User() *repositories.UserRepository {
	return m.userRepo
}

// Status returns the mock status repository
func (m *MockRepositoryStorage) Status() *repositories.StatusRepository {
	return m.statusRepo
}

// Cost returns the mock cost tracking repository
func (m *MockRepositoryStorage) Cost() *repositories.TrackingRepository {
	return m.costRepo
}

// WebSocketCost returns a mock WebSocket cost repository
func (m *MockRepositoryStorage) WebSocketCost() *repositories.WebSocketCostRepository {
	return nil // Mock implementation
}

// Trust returns the mock trust repository
func (m *MockRepositoryStorage) Trust() *repositories.TrustRepository {
	return m.trustRepo
}

// Search returns the mock search repository
func (m *MockRepositoryStorage) Search() *repositories.SearchRepository {
	return m.searchRepo
}

// Relay returns the mock relay repository
func (m *MockRepositoryStorage) Relay() *repositories.RelayRepository {
	return m.relayRepo
}

// CommunityNote returns the mock community note repository
func (m *MockRepositoryStorage) CommunityNote() *repositories.CommunityNoteRepository {
	return m.communityNoteRepo
}

// Emoji returns the mock emoji repository
func (m *MockRepositoryStorage) Emoji() *repositories.EmojiRepository {
	return m.emojiRepo
}

// RateLimit returns the mock rate limit repository
func (m *MockRepositoryStorage) RateLimit() *repositories.RateLimitRepository {
	return m.rateLimitRepo
}

// Marker returns the mock marker repository
func (m *MockRepositoryStorage) Marker() *repositories.MarkerRepository {
	return m.markerRepo
}

// FeaturedTag returns the mock featured tag repository
func (m *MockRepositoryStorage) FeaturedTag() *repositories.FeaturedTagRepository {
	return m.featuredTagRepo
}

// AI returns the mock AI repository
func (m *MockRepositoryStorage) AI() *repositories.AIRepository {
	return m.aiRepo
}

// Export returns the mock export repository
func (m *MockRepositoryStorage) Export() *repositories.ExportRepository {
	return m.exportRepo
}

// Import returns the mock import repository
func (m *MockRepositoryStorage) Import() *repositories.ImportRepository {
	return m.importRepo
}

// DLQ returns the mock DLQ repository
func (m *MockRepositoryStorage) DLQ() *repositories.DLQRepository {
	return m.dlqRepo
}

// GetDB returns the mock database connection
func (m *MockRepositoryStorage) GetDB() dynamormCore.DB {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(dynamormCore.DB)
}

// GetTableName returns the mock table name
func (m *MockRepositoryStorage) GetTableName() string {
	args := m.Called()
	return args.String(0)
}

// GetLogger returns the mock logger
func (m *MockRepositoryStorage) GetLogger() *zap.Logger {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*zap.Logger)
}

// MetricRecord returns the mock metric record repository
func (m *MockRepositoryStorage) MetricRecord() *repositories.MetricRecordRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.MetricRecordRepository)
}

// CloudWatchMetrics returns the mock CloudWatch metrics repository
func (m *MockRepositoryStorage) CloudWatchMetrics() *repositories.CloudWatchMetricsRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.CloudWatchMetricsRepository)
}

// Audit returns the mock audit repository
func (m *MockRepositoryStorage) Audit() *repositories.AuditRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.AuditRepository)
}

// OAuth returns the OAuth repository mock
func (m *MockRepositoryStorage) OAuth() *repositories.OAuthRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.OAuthRepository)
}

// StreamingCloudWatch returns the StreamingCloudWatch repository mock
func (m *MockRepositoryStorage) StreamingCloudWatch() *repositories.StreamingCloudWatchRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.StreamingCloudWatchRepository)
}

// DNSCache returns the DNSCacheRepository instance for testing.
func (m *MockRepositoryStorage) DNSCache() *repositories.DNSCacheRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.DNSCacheRepository)
}

// Filter returns the FilterRepository instance for testing.
func (m *MockRepositoryStorage) Filter() *repositories.FilterRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.FilterRepository)
}

// Ensure MockRepositoryStorage implements RepositoryStorage interface
var _ core.RepositoryStorage = (*MockRepositoryStorage)(nil)
