// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/mock"
)

// MockUserRepositoryInterface is a mock implementation of interfaces.UserRepository
// using testify/mock for expectation-based testing.
type MockUserRepositoryInterface struct {
	mock.Mock
}

// NewMockUserRepositoryInterface creates a new mock user repository
func NewMockUserRepositoryInterface() *MockUserRepositoryInterface {
	return &MockUserRepositoryInterface{}
}

// Core CRUD operations

// CreateUser mocks the CreateUser method
func (m *MockUserRepositoryInterface) CreateUser(ctx context.Context, user *storage.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

// GetUser mocks the GetUser method
func (m *MockUserRepositoryInterface) GetUser(ctx context.Context, username string) (*storage.User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

// GetUserByEmail mocks the GetUserByEmail method
func (m *MockUserRepositoryInterface) GetUserByEmail(ctx context.Context, email string) (*storage.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

// UpdateUser mocks the UpdateUser method
func (m *MockUserRepositoryInterface) UpdateUser(ctx context.Context, username string, updates map[string]any) error {
	args := m.Called(ctx, username, updates)
	return args.Error(0)
}

// DeleteUser mocks the DeleteUser method
func (m *MockUserRepositoryInterface) DeleteUser(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// ListUsers mocks the ListUsers method
func (m *MockUserRepositoryInterface) ListUsers(ctx context.Context, limit int32, cursor string) ([]*storage.User, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.User), args.String(1), args.Error(2)
}

// ListAgents mocks the ListAgents method
func (m *MockUserRepositoryInterface) ListAgents(ctx context.Context, limit int32, cursor string) ([]*storage.User, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.User), args.String(1), args.Error(2)
}

// ListUsersByRole mocks the ListUsersByRole method
func (m *MockUserRepositoryInterface) ListUsersByRole(ctx context.Context, role string) ([]*storage.User, error) {
	args := m.Called(ctx, role)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.User), args.Error(1)
}

// Count operations

// GetActiveUserCount mocks the GetActiveUserCount method
func (m *MockUserRepositoryInterface) GetActiveUserCount(ctx context.Context, days int) (int64, error) {
	args := m.Called(ctx, days)
	return args.Get(0).(int64), args.Error(1)
}

// GetTotalUserCount mocks the GetTotalUserCount method
func (m *MockUserRepositoryInterface) GetTotalUserCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

// OAuth provider operations

// GetUserByProviderID mocks the GetUserByProviderID method
func (m *MockUserRepositoryInterface) GetUserByProviderID(ctx context.Context, provider, providerID string) (*storage.User, error) {
	args := m.Called(ctx, provider, providerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

// LinkProviderAccount mocks the LinkProviderAccount method
func (m *MockUserRepositoryInterface) LinkProviderAccount(ctx context.Context, username, provider, providerID string) error {
	args := m.Called(ctx, username, provider, providerID)
	return args.Error(0)
}

// UnlinkProviderAccount mocks the UnlinkProviderAccount method
func (m *MockUserRepositoryInterface) UnlinkProviderAccount(ctx context.Context, username, provider string) error {
	args := m.Called(ctx, username, provider)
	return args.Error(0)
}

// GetLinkedProviders mocks the GetLinkedProviders method
func (m *MockUserRepositoryInterface) GetLinkedProviders(ctx context.Context, username string) ([]string, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// Account pins (endorsed accounts)

// CreateAccountPin mocks the CreateAccountPin method
func (m *MockUserRepositoryInterface) CreateAccountPin(ctx context.Context, pin *storage.AccountPin) error {
	args := m.Called(ctx, pin)
	return args.Error(0)
}

// DeleteAccountPin mocks the DeleteAccountPin method
func (m *MockUserRepositoryInterface) DeleteAccountPin(ctx context.Context, username, pinnedActorID string) error {
	args := m.Called(ctx, username, pinnedActorID)
	return args.Error(0)
}

// GetAccountPins mocks the GetAccountPins method
func (m *MockUserRepositoryInterface) GetAccountPins(ctx context.Context, username string) ([]*storage.AccountPin, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.AccountPin), args.Error(1)
}

// IsAccountPinned mocks the IsAccountPinned method
func (m *MockUserRepositoryInterface) IsAccountPinned(ctx context.Context, username, actorID string) (bool, error) {
	args := m.Called(ctx, username, actorID)
	return args.Bool(0), args.Error(1)
}

// Account notes

// CreateAccountNote mocks the CreateAccountNote method
func (m *MockUserRepositoryInterface) CreateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	args := m.Called(ctx, note)
	return args.Error(0)
}

// GetAccountNote mocks the GetAccountNote method
func (m *MockUserRepositoryInterface) GetAccountNote(ctx context.Context, username, targetActorID string) (*storage.AccountNote, error) {
	args := m.Called(ctx, username, targetActorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.AccountNote), args.Error(1)
}

// UpdateAccountNote mocks the UpdateAccountNote method
func (m *MockUserRepositoryInterface) UpdateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	args := m.Called(ctx, note)
	return args.Error(0)
}

// DeleteAccountNote mocks the DeleteAccountNote method
func (m *MockUserRepositoryInterface) DeleteAccountNote(ctx context.Context, username, targetActorID string) error {
	args := m.Called(ctx, username, targetActorID)
	return args.Error(0)
}

// Reputation operations

// StoreReputation mocks the StoreReputation method
func (m *MockUserRepositoryInterface) StoreReputation(ctx context.Context, actorID string, reputation *storage.Reputation) error {
	args := m.Called(ctx, actorID, reputation)
	return args.Error(0)
}

// GetReputation mocks the GetReputation method
func (m *MockUserRepositoryInterface) GetReputation(ctx context.Context, actorID string) (*storage.Reputation, error) {
	args := m.Called(ctx, actorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Reputation), args.Error(1)
}

// GetReputationHistory mocks the GetReputationHistory method
func (m *MockUserRepositoryInterface) GetReputationHistory(ctx context.Context, actorID string, limit int) ([]*storage.Reputation, error) {
	args := m.Called(ctx, actorID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Reputation), args.Error(1)
}

// GetUserTrustScore mocks the GetUserTrustScore method
func (m *MockUserRepositoryInterface) GetUserTrustScore(ctx context.Context, userID string) (float64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(float64), args.Error(1)
}

// Vouch operations

// CreateVouch mocks the CreateVouch method
func (m *MockUserRepositoryInterface) CreateVouch(ctx context.Context, vouch *storage.Vouch) error {
	args := m.Called(ctx, vouch)
	return args.Error(0)
}

// GetVouch mocks the GetVouch method
func (m *MockUserRepositoryInterface) GetVouch(ctx context.Context, vouchID string) (*storage.Vouch, error) {
	args := m.Called(ctx, vouchID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Vouch), args.Error(1)
}

// GetVouchesByActor mocks the GetVouchesByActor method
func (m *MockUserRepositoryInterface) GetVouchesByActor(ctx context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	args := m.Called(ctx, actorID, activeOnly)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Vouch), args.Error(1)
}

// GetVouchesForActor mocks the GetVouchesForActor method
func (m *MockUserRepositoryInterface) GetVouchesForActor(ctx context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	args := m.Called(ctx, actorID, activeOnly)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Vouch), args.Error(1)
}

// UpdateVouchStatus mocks the UpdateVouchStatus method
func (m *MockUserRepositoryInterface) UpdateVouchStatus(ctx context.Context, vouchID string, active bool, revokedAt *time.Time) error {
	args := m.Called(ctx, vouchID, active, revokedAt)
	return args.Error(0)
}

// GetMonthlyVouchCount mocks the GetMonthlyVouchCount method
func (m *MockUserRepositoryInterface) GetMonthlyVouchCount(ctx context.Context, actorID string, year int, month time.Month) (int, error) {
	args := m.Called(ctx, actorID, year, month)
	return args.Int(0), args.Error(1)
}

// Trust relationship operations

// CreateTrustRelationship mocks the CreateTrustRelationship method
func (m *MockUserRepositoryInterface) CreateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	args := m.Called(ctx, relationship)
	return args.Error(0)
}

// GetTrustRelationship mocks the GetTrustRelationship method
func (m *MockUserRepositoryInterface) GetTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error) {
	args := m.Called(ctx, trusterID, trusteeID, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.TrustRelationship), args.Error(1)
}

// UpdateTrustRelationship mocks the UpdateTrustRelationship method
func (m *MockUserRepositoryInterface) UpdateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	args := m.Called(ctx, relationship)
	return args.Error(0)
}

// DeleteTrustRelationship mocks the DeleteTrustRelationship method
func (m *MockUserRepositoryInterface) DeleteTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) error {
	args := m.Called(ctx, trusterID, trusteeID, category)
	return args.Error(0)
}

// GetTrustRelationships mocks the GetTrustRelationships method
func (m *MockUserRepositoryInterface) GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	args := m.Called(ctx, trusterID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TrustRelationship), args.String(1), args.Error(2)
}

// GetTrustedByRelationships mocks the GetTrustedByRelationships method
func (m *MockUserRepositoryInterface) GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	args := m.Called(ctx, trusteeID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TrustRelationship), args.String(1), args.Error(2)
}

// GetAllTrustRelationships mocks the GetAllTrustRelationships method
func (m *MockUserRepositoryInterface) GetAllTrustRelationships(ctx context.Context, limit int) ([]*storage.TrustRelationship, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrustRelationship), args.Error(1)
}

// Trust score operations

// GetTrustScore mocks the GetTrustScore method
func (m *MockUserRepositoryInterface) GetTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error) {
	args := m.Called(ctx, actorID, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.TrustScore), args.Error(1)
}

// UpdateTrustScore mocks the UpdateTrustScore method
func (m *MockUserRepositoryInterface) UpdateTrustScore(ctx context.Context, score *storage.TrustScore) error {
	args := m.Called(ctx, score)
	return args.Error(0)
}

// RecordTrustUpdate mocks the RecordTrustUpdate method
func (m *MockUserRepositoryInterface) RecordTrustUpdate(ctx context.Context, update *storage.TrustUpdate) error {
	args := m.Called(ctx, update)
	return args.Error(0)
}

// User preferences operations

// GetUserLanguagePreference mocks the GetUserLanguagePreference method
func (m *MockUserRepositoryInterface) GetUserLanguagePreference(ctx context.Context, username string) (string, error) {
	args := m.Called(ctx, username)
	return args.String(0), args.Error(1)
}

// SetUserLanguagePreference mocks the SetUserLanguagePreference method
func (m *MockUserRepositoryInterface) SetUserLanguagePreference(ctx context.Context, username string, language string) error {
	args := m.Called(ctx, username, language)
	return args.Error(0)
}

// GetUserPreferences mocks the GetUserPreferences method
func (m *MockUserRepositoryInterface) GetUserPreferences(ctx context.Context, username string) (*storage.UserPreferences, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.UserPreferences), args.Error(1)
}

// UpdateUserPreferences mocks the UpdateUserPreferences method
func (m *MockUserRepositoryInterface) UpdateUserPreferences(ctx context.Context, username string, preferences *storage.UserPreferences) error {
	args := m.Called(ctx, username, preferences)
	return args.Error(0)
}

// SetPreference mocks the SetPreference method
func (m *MockUserRepositoryInterface) SetPreference(ctx context.Context, username, key string, value any) error {
	args := m.Called(ctx, username, key, value)
	return args.Error(0)
}

// GetPreference mocks the GetPreference method
func (m *MockUserRepositoryInterface) GetPreference(ctx context.Context, username, key string) (any, error) {
	args := m.Called(ctx, username, key)
	return args.Get(0), args.Error(1)
}

// GetAllPreferences mocks the GetAllPreferences method
func (m *MockUserRepositoryInterface) GetAllPreferences(ctx context.Context, username string) (map[string]any, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]any), args.Error(1)
}

// UpdatePreferences mocks the UpdatePreferences method
func (m *MockUserRepositoryInterface) UpdatePreferences(ctx context.Context, username string, preferences map[string]any) error {
	args := m.Called(ctx, username, preferences)
	return args.Error(0)
}

// Follow operations

// AcceptFollow mocks the AcceptFollow method
func (m *MockUserRepositoryInterface) AcceptFollow(ctx context.Context, followerUsername, followedUsername string) error {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Error(0)
}

// RejectFollow mocks the RejectFollow method
func (m *MockUserRepositoryInterface) RejectFollow(ctx context.Context, followerUsername, followedUsername string) error {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Error(0)
}

// GetFollowRequestState mocks the GetFollowRequestState method
func (m *MockUserRepositoryInterface) GetFollowRequestState(ctx context.Context, followerID, targetID string) (string, error) {
	args := m.Called(ctx, followerID, targetID)
	return args.String(0), args.Error(1)
}

// GetPendingFollowRequests mocks the GetPendingFollowRequests method
func (m *MockUserRepositoryInterface) GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

// RemoveFromFollowers mocks the RemoveFromFollowers method
func (m *MockUserRepositoryInterface) RemoveFromFollowers(ctx context.Context, username, followerUsername string) error {
	args := m.Called(ctx, username, followerUsername)
	return args.Error(0)
}

// Conversation mute operations

// CreateConversationMute mocks the CreateConversationMute method
func (m *MockUserRepositoryInterface) CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error {
	args := m.Called(ctx, mute)
	return args.Error(0)
}

// DeleteConversationMute mocks the DeleteConversationMute method
func (m *MockUserRepositoryInterface) DeleteConversationMute(ctx context.Context, username, conversationID string) error {
	args := m.Called(ctx, username, conversationID)
	return args.Error(0)
}

// IsConversationMuted mocks the IsConversationMuted method
func (m *MockUserRepositoryInterface) IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error) {
	args := m.Called(ctx, username, conversationID)
	return args.Bool(0), args.Error(1)
}

// GetMutedConversations mocks the GetMutedConversations method
func (m *MockUserRepositoryInterface) GetMutedConversations(ctx context.Context, username string) ([]string, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// Notification operations

// IsNotificationMuted mocks the IsNotificationMuted method
func (m *MockUserRepositoryInterface) IsNotificationMuted(ctx context.Context, userID, targetID string) (bool, error) {
	args := m.Called(ctx, userID, targetID)
	return args.Bool(0), args.Error(1)
}

// Remote actor caching

// CacheRemoteActor mocks the CacheRemoteActor method
func (m *MockUserRepositoryInterface) CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	args := m.Called(ctx, handle, actor, ttl)
	return args.Error(0)
}

// Bookmark operations

// CreateBookmark mocks the CreateBookmark method
func (m *MockUserRepositoryInterface) CreateBookmark(ctx context.Context, username, objectID string) error {
	args := m.Called(ctx, username, objectID)
	return args.Error(0)
}

// RemoveBookmark mocks the RemoveBookmark method
func (m *MockUserRepositoryInterface) RemoveBookmark(ctx context.Context, username, objectID string) error {
	args := m.Called(ctx, username, objectID)
	return args.Error(0)
}

// GetBookmarks mocks the GetBookmarks method
func (m *MockUserRepositoryInterface) GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

// IsBookmarked mocks the IsBookmarked method
func (m *MockUserRepositoryInterface) IsBookmarked(ctx context.Context, username, objectID string) (bool, error) {
	args := m.Called(ctx, username, objectID)
	return args.Bool(0), args.Error(1)
}

// Timeline operations

// DeleteFromTimeline mocks the DeleteFromTimeline method
func (m *MockUserRepositoryInterface) DeleteFromTimeline(ctx context.Context, timelineType, timelineID, entryID string) error {
	args := m.Called(ctx, timelineType, timelineID, entryID)
	return args.Error(0)
}

// DeleteExpiredTimelineEntries mocks the DeleteExpiredTimelineEntries method
func (m *MockUserRepositoryInterface) DeleteExpiredTimelineEntries(ctx context.Context, before time.Time) error {
	args := m.Called(ctx, before)
	return args.Error(0)
}

// GetDirectTimeline mocks the GetDirectTimeline method
func (m *MockUserRepositoryInterface) GetDirectTimeline(ctx context.Context, username string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TimelineEntry), args.String(1), args.Error(2)
}

// GetHashtagTimeline mocks the GetHashtagTimeline method
func (m *MockUserRepositoryInterface) GetHashtagTimeline(ctx context.Context, hashtag string, local bool, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, hashtag, local, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TimelineEntry), args.String(1), args.Error(2)
}

// GetListTimeline mocks the GetListTimeline method
func (m *MockUserRepositoryInterface) GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	args := m.Called(ctx, listID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.TimelineEntry), args.String(1), args.Error(2)
}

// Fan-out operations

// FanOutPost mocks the FanOutPost method
func (m *MockUserRepositoryInterface) FanOutPost(ctx context.Context, activity *activitypub.Activity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

// Ensure MockUserRepositoryInterface implements interfaces.UserRepository
var _ interface {
	CreateUser(ctx context.Context, user *storage.User) error
	GetUser(ctx context.Context, username string) (*storage.User, error)
} = (*MockUserRepositoryInterface)(nil)
