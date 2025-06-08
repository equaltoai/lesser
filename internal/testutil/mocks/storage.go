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

// GetHashtagTimeline is a mock implementation
func (m *TimelineMethods) GetHashtagTimeline(ctx context.Context, hashtag string, local bool, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
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

// FanOutPost is a mock implementation
func (m *TimelineMethods) FanOutPost(ctx context.Context, activity *activitypub.Activity) error {
	return nil
}

// BaseMockStorage provides no-op implementations of all Storage interface methods
// This can be embedded in test mocks to automatically satisfy the interface
// Test mocks can then override only the methods they need
type BaseMockStorage struct {
	TimelineMethods
}

// MockStorage is a mock implementation of the storage.Storage interface
// that uses testify's mock framework for testing
type MockStorage struct {
	mock.Mock
	BaseMockStorage
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
func (m *MockStorage) CreateObject(ctx context.Context, object interface{}) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

// GetObject mocks the GetObject method
func (m *MockStorage) GetObject(ctx context.Context, id string) (interface{}, error) {
	args := m.Called(ctx, id)
	return args.Get(0), args.Error(1)
}

// UpdateObject mocks the UpdateObject method
func (m *MockStorage) UpdateObject(ctx context.Context, object interface{}) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

// DeleteObject mocks the DeleteObject method
func (m *MockStorage) DeleteObject(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// GetObjectsByActor mocks the GetObjectsByActor method
func (m *MockStorage) GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]interface{}, string, error) {
	args := m.Called(ctx, actorID, cursor, limit)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]interface{}), args.String(1), args.Error(2)
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

// GetCollection mocks the GetCollection method
func (m *MockStorage) GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error) {
	args := m.Called(ctx, username, collectionType, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.OrderedCollectionPage), args.Error(1)
}

// CreateFlag mocks the CreateFlag method
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

// CreateMove mocks the CreateMove method
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

// CommunityNote operations
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

func (m *MockStorage) CreateCommunityNoteVote(ctx context.Context, vote *storage.CommunityNoteVote) error {
	args := m.Called(ctx, vote)
	return args.Error(0)
}

func (m *MockStorage) GetUserCommunityNoteVotes(ctx context.Context, userID string, noteIDs []string) (map[string]*storage.CommunityNoteVote, error) {
	args := m.Called(ctx, userID, noteIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]*storage.CommunityNoteVote), args.Error(1)
}

func (m *MockStorage) CheckCommunityNoteRateLimit(ctx context.Context, userID string, limit int) (bool, int, error) {
	args := m.Called(ctx, userID, limit)
	return args.Bool(0), args.Int(1), args.Error(2)
}

func (m *MockStorage) GetCommunityNotesByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*storage.CommunityNote, string, error) {
	args := m.Called(ctx, authorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.CommunityNote), args.String(1), args.Error(2)
}

func (m *MockStorage) GetCommunityNoteVotes(ctx context.Context, noteID string) ([]*storage.CommunityNoteVote, error) {
	args := m.Called(ctx, noteID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CommunityNoteVote), args.Error(1)
}

// Embed the BaseMockStorage for all other methods not explicitly overridden
// This allows tests to use MockStorage without implementing every single method
type embeddedMockStorage struct {
	BaseMockStorage
}

// Ensure MockStorage can access methods from BaseMockStorage when not explicitly mocked
func (m *MockStorage) embedDefaults() *embeddedMockStorage {
	return &embeddedMockStorage{}
}

// ClearLoginAttempts mocks the ClearLoginAttempts method
func (m *MockStorage) ClearLoginAttempts(ctx context.Context, identifier string) error {
	args := m.Called(ctx, identifier)
	return args.Error(0)
}

// CountUnusedRecoveryCodes mocks the CountUnusedRecoveryCodes method
func (m *MockStorage) CountUnusedRecoveryCodes(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

// AddAccountsToList mocks the AddAccountsToList method
func (m *MockStorage) AddAccountsToList(ctx context.Context, listID string, accountIDs []string) error {
	args := m.Called(ctx, listID, accountIDs)
	return args.Error(0)
}

// AddAnnouncementReaction mocks the AddAnnouncementReaction method
func (m *MockStorage) AddAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error {
	args := m.Called(ctx, username, announcementID, emojiName)
	return args.Error(0)
}

// AddDomainBlock mocks the AddDomainBlock method
func (m *MockStorage) AddDomainBlock(ctx context.Context, username, domain string) error {
	args := m.Called(ctx, username, domain)
	return args.Error(0)
}

// AddFilterKeyword mocks the AddFilterKeyword method
func (m *MockStorage) AddFilterKeyword(ctx context.Context, filterID string, keyword *storage.FilterKeyword) error {
	args := m.Called(ctx, filterID, keyword)
	return args.Error(0)
}

// AddFilterStatus mocks the AddFilterStatus method
func (m *MockStorage) AddFilterStatus(ctx context.Context, filterID string, status *storage.FilterStatus) error {
	args := m.Called(ctx, filterID, status)
	return args.Error(0)
}

// AddModerationReview mocks the AddModerationReview method
func (m *MockStorage) AddModerationReview(ctx context.Context, review *storage.ModerationReview) error {
	args := m.Called(ctx, review)
	return args.Error(0)
}

// AddParticipantToConversation mocks the AddParticipantToConversation method
func (m *MockStorage) AddParticipantToConversation(ctx context.Context, conversationID, participantID string) error {
	args := m.Called(ctx, conversationID, participantID)
	return args.Error(0)
}

// AddToCollection mocks the AddToCollection method
func (m *MockStorage) AddToCollection(ctx context.Context, collection string, item *storage.CollectionItem) error {
	args := m.Called(ctx, collection, item)
	return args.Error(0)
}

// AssignReport mocks the AssignReport method
func (m *MockStorage) AssignReport(ctx context.Context, reportID string, assignedTo string) error {
	args := m.Called(ctx, reportID, assignedTo)
	return args.Error(0)
}

// CacheRemoteActor mocks the CacheRemoteActor method
func (m *MockStorage) CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	args := m.Called(ctx, handle, actor, ttl)
	return args.Error(0)
}

// CascadeDeleteAnnounces mocks the CascadeDeleteAnnounces method
func (m *MockStorage) CascadeDeleteAnnounces(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// CascadeDeleteLikes mocks the CascadeDeleteLikes method
func (m *MockStorage) CascadeDeleteLikes(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// ClearNotifications mocks the ClearNotifications method
func (m *MockStorage) ClearNotifications(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// CountCollectionItems mocks the CountCollectionItems method
func (m *MockStorage) CountCollectionItems(ctx context.Context, collection string) (int, error) {
	args := m.Called(ctx, collection)
	return args.Int(0), args.Error(1)
}

// CountObjectAnnounces mocks the CountObjectAnnounces method
func (m *MockStorage) CountObjectAnnounces(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// CountObjectLikes mocks the CountObjectLikes method
func (m *MockStorage) CountObjectLikes(ctx context.Context, objectID string) (int, error) {
	args := m.Called(ctx, objectID)
	return args.Int(0), args.Error(1)
}

// Device management operations
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

// MockStorage is the full mock implementation for tests

// Now implementing BaseMockStorage methods that were missing

// Device management operations for BaseMockStorage
func (m *BaseMockStorage) CreateDevice(ctx context.Context, device *storage.Device) error {
	return nil
}

func (m *BaseMockStorage) GetDevice(ctx context.Context, deviceID string) (*storage.Device, error) {
	return nil, nil
}

func (m *BaseMockStorage) UpdateDevice(ctx context.Context, device *storage.Device) error {
	return nil
}

func (m *BaseMockStorage) GetUserDevices(ctx context.Context, username string) ([]*storage.Device, error) {
	return []*storage.Device{}, nil
}
