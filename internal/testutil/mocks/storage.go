package mocks

import (
	"context"
	"fmt"
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

// Actor operations
func (m *BaseMockStorage) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	return nil
}
func (m *BaseMockStorage) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	return nil, nil
}
func (m *BaseMockStorage) GetActorWithMetadata(ctx context.Context, username string) (*activitypub.Actor, *storage.ActorMetadata, error) {
	return nil, nil, nil
}
func (m *BaseMockStorage) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	return "", nil
}
func (m *BaseMockStorage) UpdateActor(ctx context.Context, actor *activitypub.Actor) error {
	return nil
}
func (m *BaseMockStorage) UpdateActorLastStatusTime(ctx context.Context, username string) error {
	return nil
}
func (m *BaseMockStorage) SetActorFields(ctx context.Context, username string, fields []storage.ActorField) error {
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
	return nil, fmt.Errorf("not found")
}
func (m *BaseMockStorage) UpdateObject(ctx context.Context, object interface{}) error {
	return nil
}
func (m *BaseMockStorage) DeleteObject(ctx context.Context, id string) error {
	return nil
}
func (m *BaseMockStorage) GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]interface{}, string, error) {
	return []interface{}{}, "", nil
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
func (m *BaseMockStorage) GetActiveUserCount(ctx context.Context, days int) (int64, error) {
	return 10, nil
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
	return []*storage.UpdateHistory{}, nil
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

// Conversation operations
func (m *BaseMockStorage) CreateConversation(ctx context.Context, conversation *storage.Conversation) error {
	return nil
}
func (m *BaseMockStorage) GetConversation(ctx context.Context, id string) (*storage.Conversation, error) {
	return nil, fmt.Errorf("not found")
}
func (m *BaseMockStorage) GetConversationByParticipants(ctx context.Context, participants []string) (*storage.Conversation, error) {
	return nil, fmt.Errorf("not found")
}
func (m *BaseMockStorage) UpdateConversationLastStatus(ctx context.Context, id, lastStatusID string) error {
	return nil
}
func (m *BaseMockStorage) MarkConversationRead(ctx context.Context, id, username string) error {
	return nil
}
func (m *BaseMockStorage) DeleteConversation(ctx context.Context, id string) error {
	return nil
}
func (m *BaseMockStorage) GetUserConversations(ctx context.Context, userID string, limit int, cursor string) ([]*storage.Conversation, string, error) {
	return []*storage.Conversation{}, "", nil
}
func (m *BaseMockStorage) AddParticipantToConversation(ctx context.Context, conversationID, participantID string) error {
	return nil
}

// List operations
func (m *BaseMockStorage) CreateList(ctx context.Context, username, title, repliesPolicy string) (*storage.List, error) {
	return &storage.List{
		ID:            "list-123",
		Username:      username,
		Title:         title,
		RepliesPolicy: repliesPolicy,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}, nil
}
func (m *BaseMockStorage) GetList(ctx context.Context, listID string) (*storage.List, error) {
	return nil, fmt.Errorf("not found")
}
func (m *BaseMockStorage) GetListsForUser(ctx context.Context, username string) ([]*storage.List, error) {
	return []*storage.List{}, nil
}
func (m *BaseMockStorage) UpdateList(ctx context.Context, listID string, updates map[string]interface{}) error {
	return nil
}
func (m *BaseMockStorage) DeleteList(ctx context.Context, listID string) error {
	return nil
}
func (m *BaseMockStorage) AddAccountsToList(ctx context.Context, listID string, accountIDs []string) error {
	return nil
}
func (m *BaseMockStorage) RemoveAccountsFromList(ctx context.Context, listID string, accountIDs []string) error {
	return nil
}
func (m *BaseMockStorage) GetListAccounts(ctx context.Context, listID string) ([]string, error) {
	return []string{}, nil
}
func (m *BaseMockStorage) IsAccountInList(ctx context.Context, listID, accountID string) (bool, error) {
	return false, nil
}
func (m *BaseMockStorage) GetListsContainingAccount(ctx context.Context, accountID, username string) ([]*storage.List, error) {
	return []*storage.List{}, nil
}

// Notification operations
func (m *BaseMockStorage) CreateNotification(ctx context.Context, notification *storage.Notification) error {
	return nil
}
func (m *BaseMockStorage) GetNotification(ctx context.Context, id string) (*storage.Notification, error) {
	return nil, fmt.Errorf("not found")
}
func (m *BaseMockStorage) GetNotifications(ctx context.Context, username string, limit int, cursor string) ([]*storage.Notification, string, error) {
	return []*storage.Notification{}, "", nil
}
func (m *BaseMockStorage) GetNotificationsFiltered(ctx context.Context, username string, filter *storage.NotificationFilter) ([]*storage.Notification, string, error) {
	return []*storage.Notification{}, "", nil
}
func (m *BaseMockStorage) MarkNotificationAsRead(ctx context.Context, id string) error {
	return nil
}
func (m *BaseMockStorage) MarkAllNotificationsAsRead(ctx context.Context, username string) error {
	return nil
}
func (m *BaseMockStorage) DeleteNotification(ctx context.Context, id string) error {
	return nil
}
func (m *BaseMockStorage) ClearNotifications(ctx context.Context, username string) error {
	return nil
}
func (m *BaseMockStorage) CountUnreadNotifications(ctx context.Context, username string) (int, error) {
	return 0, nil
}

// Search methods
func (m *BaseMockStorage) SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error) {
	return []*activitypub.Actor{}, nil
}
func (m *BaseMockStorage) GetSearchSuggestions(ctx context.Context, prefix string) ([]storage.SearchSuggestion, error) {
	return []storage.SearchSuggestion{}, nil
}

// Status search operations
func (m *BaseMockStorage) SearchStatuses(ctx context.Context, query string, limit int) ([]*storage.StatusSearchResult, error) {
	return []*storage.StatusSearchResult{}, nil
}
func (m *BaseMockStorage) SearchStatusesWithOptions(ctx context.Context, query string, options storage.StatusSearchOptions) ([]*storage.StatusSearchResult, error) {
	return []*storage.StatusSearchResult{}, nil
}
func (m *BaseMockStorage) SearchStatusesByURL(ctx context.Context, url string) (*storage.StatusSearchResult, error) {
	return nil, nil
}

// Remote actor cache operations
func (m *BaseMockStorage) CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	return nil
}
func (m *BaseMockStorage) GetCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error) {
	return nil, fmt.Errorf("not found")
}

// Push notification operations
func (m *BaseMockStorage) CreatePushSubscription(ctx context.Context, username string, subscription *storage.PushSubscription) error {
	return nil
}
func (m *BaseMockStorage) GetPushSubscription(ctx context.Context, username, subscriptionID string) (*storage.PushSubscription, error) {
	return nil, fmt.Errorf("not found")
}
func (m *BaseMockStorage) GetUserPushSubscriptions(ctx context.Context, username string) ([]*storage.PushSubscription, error) {
	return []*storage.PushSubscription{}, nil
}
func (m *BaseMockStorage) UpdatePushSubscription(ctx context.Context, username, subscriptionID string, alerts storage.PushSubscriptionAlerts) error {
	return nil
}
func (m *BaseMockStorage) DeletePushSubscription(ctx context.Context, username, subscriptionID string) error {
	return nil
}
func (m *BaseMockStorage) DeleteAllPushSubscriptions(ctx context.Context, username string) error {
	return nil
}

// Filter operations
func (m *BaseMockStorage) CreateFilter(ctx context.Context, filter *storage.Filter) error {
	return nil
}
func (m *BaseMockStorage) GetFilter(ctx context.Context, filterID string) (*storage.Filter, error) {
	return nil, nil
}
func (m *BaseMockStorage) GetFiltersForUser(ctx context.Context, username string) ([]*storage.Filter, error) {
	return []*storage.Filter{}, nil
}
func (m *BaseMockStorage) UpdateFilter(ctx context.Context, filterID string, updates map[string]interface{}) error {
	return nil
}
func (m *BaseMockStorage) DeleteFilter(ctx context.Context, filterID string) error {
	return nil
}
func (m *BaseMockStorage) AddFilterKeyword(ctx context.Context, filterID string, keyword *storage.FilterKeyword) error {
	return nil
}
func (m *BaseMockStorage) GetFilterKeywords(ctx context.Context, filterID string) ([]*storage.FilterKeyword, error) {
	return []*storage.FilterKeyword{}, nil
}
func (m *BaseMockStorage) UpdateFilterKeyword(ctx context.Context, keywordID string, updates map[string]interface{}) error {
	return nil
}
func (m *BaseMockStorage) DeleteFilterKeyword(ctx context.Context, keywordID string) error {
	return nil
}
func (m *BaseMockStorage) AddFilterStatus(ctx context.Context, filterID string, status *storage.FilterStatus) error {
	return nil
}
func (m *BaseMockStorage) GetFilterStatuses(ctx context.Context, filterID string) ([]*storage.FilterStatus, error) {
	return []*storage.FilterStatus{}, nil
}
func (m *BaseMockStorage) DeleteFilterStatus(ctx context.Context, statusID string) error {
	return nil
}

// Mute operations
func (m *BaseMockStorage) CreateMute(ctx context.Context, mute *storage.Mute) error {
	return nil
}
func (m *BaseMockStorage) GetMute(ctx context.Context, actor, mutedActor string) (*storage.Mute, error) {
	return nil, nil
}
func (m *BaseMockStorage) DeleteMute(ctx context.Context, actor, mutedActor string) error {
	return nil
}
func (m *BaseMockStorage) GetMutedActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Mute, string, error) {
	return []*storage.Mute{}, "", nil
}
func (m *BaseMockStorage) IsMuted(ctx context.Context, actor, targetActor string) (bool, error) {
	return false, nil
}

// Poll operations
func (m *BaseMockStorage) CreatePoll(ctx context.Context, poll *storage.Poll) error {
	return nil
}
func (m *BaseMockStorage) GetPoll(ctx context.Context, pollID string) (*storage.Poll, error) {
	return nil, nil
}
func (m *BaseMockStorage) GetPollByStatusID(ctx context.Context, statusID string) (*storage.Poll, error) {
	return nil, nil
}
func (m *BaseMockStorage) VoteOnPoll(ctx context.Context, pollID, voterID string, choices []int) error {
	return nil
}
func (m *BaseMockStorage) GetPollVotes(ctx context.Context, pollID string) (map[string][]int, error) {
	return map[string][]int{}, nil
}
func (m *BaseMockStorage) HasUserVoted(ctx context.Context, pollID string, userID string) (bool, []int, error) {
	return false, nil, nil
}

// VAPID key operations
func (m *BaseMockStorage) GetVAPIDKeys(ctx context.Context) (*storage.VAPIDKeys, error) {
	return nil, fmt.Errorf("not found")
}
func (m *BaseMockStorage) SetVAPIDKeys(ctx context.Context, keys *storage.VAPIDKeys) error {
	return nil
}

// Status pinning operations
func (m *BaseMockStorage) CreateStatusPin(ctx context.Context, pin *storage.StatusPin) error {
	return nil
}
func (m *BaseMockStorage) DeleteStatusPin(ctx context.Context, username, statusID string) error {
	return nil
}
func (m *BaseMockStorage) GetStatusPins(ctx context.Context, username string) ([]*storage.StatusPin, error) {
	return []*storage.StatusPin{}, nil
}
func (m *BaseMockStorage) IsStatusPinned(ctx context.Context, username, statusID string) (bool, error) {
	return false, nil
}
func (m *BaseMockStorage) CountUserPinnedStatuses(ctx context.Context, username string) (int, error) {
	return 0, nil
}

// Conversation muting operations
func (m *BaseMockStorage) CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error {
	return nil
}
func (m *BaseMockStorage) DeleteConversationMute(ctx context.Context, username, conversationID string) error {
	return nil
}
func (m *BaseMockStorage) IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error) {
	return false, nil
}
func (m *BaseMockStorage) GetMutedConversations(ctx context.Context, username string) ([]string, error) {
	return []string{}, nil
}

// Scheduled status operations
func (m *BaseMockStorage) CreateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	return nil
}
func (m *BaseMockStorage) GetScheduledStatus(ctx context.Context, id string) (*storage.ScheduledStatus, error) {
	return nil, fmt.Errorf("not found")
}
func (m *BaseMockStorage) GetScheduledStatuses(ctx context.Context, username string, limit int, cursor string) ([]*storage.ScheduledStatus, string, error) {
	return []*storage.ScheduledStatus{}, "", nil
}
func (m *BaseMockStorage) UpdateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	return nil
}
func (m *BaseMockStorage) DeleteScheduledStatus(ctx context.Context, id string) error {
	return nil
}
func (m *BaseMockStorage) GetDueScheduledStatuses(ctx context.Context, before time.Time, limit int) ([]*storage.ScheduledStatus, error) {
	return []*storage.ScheduledStatus{}, nil
}
func (m *BaseMockStorage) MarkScheduledStatusPublished(ctx context.Context, id string) error {
	return nil
}

// Hashtag following operations
func (m *BaseMockStorage) FollowHashtag(ctx context.Context, userID string, hashtag string) error {
	return nil
}
func (m *BaseMockStorage) UnfollowHashtag(ctx context.Context, userID string, hashtag string) error {
	return nil
}
func (m *BaseMockStorage) IsFollowingHashtag(ctx context.Context, userID string, hashtag string) (bool, error) {
	return false, nil
}
func (m *BaseMockStorage) GetFollowedHashtags(ctx context.Context, userID string, limit int, cursor string) ([]string, string, error) {
	return []string{}, "", nil
}

// Featured tags operations
func (m *BaseMockStorage) CreateFeaturedTag(ctx context.Context, userID string, tagName string) (*storage.FeaturedTag, error) {
	return nil, nil
}
func (m *BaseMockStorage) DeleteFeaturedTag(ctx context.Context, userID string, featuredTagID string) error {
	return nil
}
func (m *BaseMockStorage) GetFeaturedTags(ctx context.Context, userID string) ([]*storage.FeaturedTag, error) {
	return []*storage.FeaturedTag{}, nil
}
func (m *BaseMockStorage) GetTagSuggestions(ctx context.Context, userID string, limit int) ([]string, error) {
	return []string{}, nil
}

// Hashtag operations
func (m *BaseMockStorage) IndexHashtag(ctx context.Context, hashtag string, statusID string, authorID string, visibility string) error {
	return nil
}
func (m *BaseMockStorage) SearchHashtags(ctx context.Context, query string, limit int) ([]*storage.Hashtag, error) {
	return []*storage.Hashtag{}, nil
}
func (m *BaseMockStorage) GetHashtagInfo(ctx context.Context, hashtag string) (*storage.Hashtag, error) {
	return nil, nil
}
func (m *BaseMockStorage) GetHashtagUsageHistory(ctx context.Context, hashtag string, days int) ([]int64, error) {
	return []int64{}, nil
}

// Moderation operations
func (m *BaseMockStorage) CreateModerationEvent(ctx context.Context, event *storage.ModerationEvent) error {
	return nil
}
func (m *BaseMockStorage) GetModerationEvent(ctx context.Context, eventID string) (*storage.ModerationEvent, error) {
	return nil, nil
}
func (m *BaseMockStorage) GetModerationQueue(ctx context.Context, limit int, cursor string) ([]*storage.ModerationQueueItem, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) GetModerationEventsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) GetModerationEventsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) AddModerationReview(ctx context.Context, review *storage.ModerationReview) error {
	return nil
}
func (m *BaseMockStorage) GetModerationReviews(ctx context.Context, eventID string) ([]*storage.ModerationReview, error) {
	return nil, nil
}
func (m *BaseMockStorage) CreateModerationDecision(ctx context.Context, decision *storage.ModerationDecision) error {
	return nil
}
func (m *BaseMockStorage) GetModerationDecision(ctx context.Context, objectID string) (*storage.ModerationDecision, error) {
	return nil, nil
}
func (m *BaseMockStorage) GetModerationHistory(ctx context.Context, objectID string) (*storage.ModerationHistory, error) {
	return nil, nil
}
func (m *BaseMockStorage) GetModerationEvents(ctx context.Context, filter *storage.ModerationEventFilter, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	return []*storage.ModerationEvent{}, "", nil
}
func (m *BaseMockStorage) CreateAdminReview(ctx context.Context, eventID string, adminID string, action storage.ActionType, reason string) error {
	return nil
}
func (m *BaseMockStorage) GetReviewerStats(ctx context.Context, reviewerID string) (*storage.ReviewerStats, error) {
	return &storage.ReviewerStats{}, nil
}

// Trust operations
func (m *BaseMockStorage) CreateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	return nil
}
func (m *BaseMockStorage) GetTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error) {
	return nil, nil
}
func (m *BaseMockStorage) UpdateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	return nil
}
func (m *BaseMockStorage) DeleteTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) error {
	return nil
}
func (m *BaseMockStorage) GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) GetTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error) {
	return nil, nil
}
func (m *BaseMockStorage) UpdateTrustScore(ctx context.Context, score *storage.TrustScore) error {
	return nil
}
func (m *BaseMockStorage) RecordTrustUpdate(ctx context.Context, update *storage.TrustUpdate) error {
	return nil
}
func (m *BaseMockStorage) GetAllTrustRelationships(ctx context.Context, limit int) ([]*storage.TrustRelationship, error) {
	return []*storage.TrustRelationship{}, nil
}

// Account pin operations
func (m *BaseMockStorage) CreateAccountPin(ctx context.Context, pin *storage.AccountPin) error {
	return nil
}
func (m *BaseMockStorage) DeleteAccountPin(ctx context.Context, username, pinnedActorID string) error {
	return nil
}
func (m *BaseMockStorage) GetAccountPins(ctx context.Context, username string) ([]*storage.AccountPin, error) {
	return []*storage.AccountPin{}, nil
}
func (m *BaseMockStorage) IsAccountPinned(ctx context.Context, username, pinnedActorID string) (bool, error) {
	return false, nil
}

// Account note operations
func (m *BaseMockStorage) CreateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	return nil
}
func (m *BaseMockStorage) GetAccountNote(ctx context.Context, username, targetActorID string) (*storage.AccountNote, error) {
	return nil, nil
}
func (m *BaseMockStorage) UpdateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	return nil
}
func (m *BaseMockStorage) DeleteAccountNote(ctx context.Context, username, targetActorID string) error {
	return nil
}
func (m *BaseMockStorage) RemoveFromFollowers(ctx context.Context, username, followerUsername string) error {
	return nil
}

// Language detection and user preferences mocks
func (m *BaseMockStorage) GetUserLanguagePreference(ctx context.Context, username string) (string, error) {
	return "en", nil
}
func (m *BaseMockStorage) SetUserLanguagePreference(ctx context.Context, username string, language string) error {
	return nil
}
func (m *BaseMockStorage) GetUserPreferences(ctx context.Context, username string) (*storage.UserPreferences, error) {
	return &storage.UserPreferences{
		Language:                  "en",
		DefaultPostingVisibility:  "public",
		DefaultMediaSensitive:     false,
		ExpandSpoilers:            false,
		ShowFollowCounts:          true,
		PreferredTimelineOrder:    "newest",
		SearchSuggestionsEnabled:  true,
		PersonalizedSearchEnabled: true,
	}, nil
}
func (m *BaseMockStorage) UpdateUserPreferences(ctx context.Context, username string, preferences *storage.UserPreferences) error {
	return nil
}

// Marker operations
func (m *BaseMockStorage) SaveMarker(ctx context.Context, username, timeline string, lastReadID string, version int) error {
	return nil
}
func (m *BaseMockStorage) GetMarkers(ctx context.Context, username string, timelines []string) (map[string]*storage.Marker, error) {
	return make(map[string]*storage.Marker), nil
}

// Domain block operations
func (m *BaseMockStorage) AddDomainBlock(ctx context.Context, username, domain string) error {
	return nil
}
func (m *BaseMockStorage) RemoveDomainBlock(ctx context.Context, username, domain string) error {
	return nil
}
func (m *BaseMockStorage) GetUserDomainBlocks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return []string{}, "", nil
}
func (m *BaseMockStorage) IsBlockedDomain(ctx context.Context, username, domain string) (bool, error) {
	return false, nil
}

// Extended preferences operations
func (m *BaseMockStorage) SetPreference(ctx context.Context, username string, key string, value interface{}) error {
	return nil
}
func (m *BaseMockStorage) GetPreference(ctx context.Context, username string, key string) (interface{}, error) {
	return nil, nil
}
func (m *BaseMockStorage) GetAllPreferences(ctx context.Context, username string) (map[string]interface{}, error) {
	return make(map[string]interface{}), nil
}
func (m *BaseMockStorage) UpdatePreferences(ctx context.Context, username string, prefs map[string]interface{}) error {
	return nil
}

// Search suggestion tracking and analytics mocks
func (m *BaseMockStorage) TrackSearchQuery(ctx context.Context, userID, query string, resultCount int) error {
	return nil
}
func (m *BaseMockStorage) GetPopularSearchQueries(ctx context.Context, limit int, timeWindow time.Duration) ([]storage.SearchQueryStats, error) {
	return []storage.SearchQueryStats{}, nil
}
func (m *BaseMockStorage) GetUserSearchHistory(ctx context.Context, userID string, limit int) ([]storage.SearchHistoryEntry, error) {
	return []storage.SearchHistoryEntry{}, nil
}
func (m *BaseMockStorage) GenerateSearchSuggestions(ctx context.Context, userID, partialQuery string, limit int) ([]string, error) {
	return []string{}, nil
}

// Trending operations
func (m *BaseMockStorage) RecordHashtagUsage(ctx context.Context, hashtag string, statusID string, authorID string) error {
	return nil
}
func (m *BaseMockStorage) RecordStatusEngagement(ctx context.Context, statusID string, engagementType string, userID string) error {
	return nil
}
func (m *BaseMockStorage) RecordLinkShare(ctx context.Context, url string, statusID string, authorID string) error {
	return nil
}
func (m *BaseMockStorage) GetTrendingHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	return []*storage.TrendingHashtag{}, nil
}
func (m *BaseMockStorage) GetTrendingStatuses(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error) {
	return []*storage.TrendingStatus{}, nil
}
func (m *BaseMockStorage) GetTrendingLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	return []*storage.TrendingLink{}, nil
}

// Announcement operations
func (m *BaseMockStorage) CreateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	return nil
}
func (m *BaseMockStorage) GetAnnouncement(ctx context.Context, id string) (*storage.Announcement, error) {
	return nil, fmt.Errorf("not found")
}
func (m *BaseMockStorage) GetAnnouncements(ctx context.Context, active bool) ([]*storage.Announcement, error) {
	return []*storage.Announcement{}, nil
}
func (m *BaseMockStorage) UpdateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	return nil
}
func (m *BaseMockStorage) DeleteAnnouncement(ctx context.Context, id string) error {
	return nil
}
func (m *BaseMockStorage) DismissAnnouncement(ctx context.Context, username, announcementID string) error {
	return nil
}
func (m *BaseMockStorage) IsDismissed(ctx context.Context, username, announcementID string) (bool, error) {
	return false, nil
}
func (m *BaseMockStorage) GetDismissedAnnouncements(ctx context.Context, username string) ([]string, error) {
	return []string{}, nil
}
func (m *BaseMockStorage) AddAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error {
	return nil
}
func (m *BaseMockStorage) RemoveAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error {
	return nil
}
func (m *BaseMockStorage) GetAnnouncementReactions(ctx context.Context, announcementID string) (map[string][]string, error) {
	return map[string][]string{}, nil
}

// Custom emoji operations
func (m *BaseMockStorage) CreateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error {
	return nil
}
func (m *BaseMockStorage) GetCustomEmoji(ctx context.Context, shortcode string) (*storage.CustomEmoji, error) {
	return nil, fmt.Errorf("not found")
}
func (m *BaseMockStorage) GetCustomEmojis(ctx context.Context) ([]*storage.CustomEmoji, error) {
	return []*storage.CustomEmoji{}, nil
}
func (m *BaseMockStorage) UpdateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error {
	return nil
}
func (m *BaseMockStorage) DeleteCustomEmoji(ctx context.Context, shortcode string) error {
	return nil
}
func (m *BaseMockStorage) GetCustomEmojisByCategory(ctx context.Context, category string) ([]*storage.CustomEmoji, error) {
	return []*storage.CustomEmoji{}, nil
}

// Report operations
func (m *BaseMockStorage) CreateReport(ctx context.Context, report *storage.Report) error {
	return nil
}
func (m *BaseMockStorage) GetReport(ctx context.Context, id string) (*storage.Report, error) {
	return nil, fmt.Errorf("not found")
}
func (m *BaseMockStorage) GetUserReports(ctx context.Context, username string, limit int, cursor string) ([]*storage.Report, string, error) {
	return []*storage.Report{}, "", nil
}
func (m *BaseMockStorage) GetReportsByTarget(ctx context.Context, targetAccountID string, limit int, cursor string) ([]*storage.Report, string, error) {
	return []*storage.Report{}, "", nil
}
func (m *BaseMockStorage) GetReportsByStatus(ctx context.Context, status storage.ReportStatus, limit int, cursor string) ([]*storage.Report, string, error) {
	return []*storage.Report{}, "", nil
}
func (m *BaseMockStorage) UpdateReportStatus(ctx context.Context, id string, status storage.ReportStatus, actionTaken string, moderatorID string) error {
	return nil
}
func (m *BaseMockStorage) GetReportStats(ctx context.Context, username string) (*storage.ReportStats, error) {
	return &storage.ReportStats{}, nil
}
func (m *BaseMockStorage) IncrementFalseReports(ctx context.Context, username string) error {
	return nil
}
func (m *BaseMockStorage) AssignReport(ctx context.Context, reportID string, assignedTo string) error {
	return nil
}
func (m *BaseMockStorage) UnassignReport(ctx context.Context, reportID string) error {
	return nil
}

// Instance domain block operations
func (m *BaseMockStorage) CreateInstanceDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error {
	return nil
}
func (m *BaseMockStorage) GetInstanceDomainBlock(ctx context.Context, domain string) (*storage.InstanceDomainBlock, error) {
	return nil, fmt.Errorf("not found")
}
func (m *BaseMockStorage) GetInstanceDomainBlockByID(ctx context.Context, id string) (*storage.InstanceDomainBlock, error) {
	return nil, fmt.Errorf("not found")
}
func (m *BaseMockStorage) ListInstanceDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	return []*storage.InstanceDomainBlock{}, "", nil
}
func (m *BaseMockStorage) UpdateInstanceDomainBlock(ctx context.Context, domain string, updates map[string]interface{}) error {
	return nil
}
func (m *BaseMockStorage) DeleteInstanceDomainBlock(ctx context.Context, domain string) error {
	return nil
}
func (m *BaseMockStorage) IsInstanceDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	return false, nil, nil
}

// Federation domain management operations (admin-level)
func (m *BaseMockStorage) GetDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	return nil, "", nil
}
func (m *BaseMockStorage) GetDomainBlock(ctx context.Context, id string) (*storage.InstanceDomainBlock, error) {
	return nil, nil
}
func (m *BaseMockStorage) CreateDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error {
	return nil
}
func (m *BaseMockStorage) UpdateDomainBlock(ctx context.Context, id string, updates map[string]interface{}) error {
	return nil
}
func (m *BaseMockStorage) DeleteDomainBlock(ctx context.Context, id string) error {
	return nil
}
func (m *BaseMockStorage) IsDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	return false, nil, nil
}

// Domain allow operations
func (m *BaseMockStorage) GetDomainAllows(ctx context.Context, limit int, cursor string) ([]*storage.DomainAllow, string, error) {
	return []*storage.DomainAllow{}, "", nil
}
func (m *BaseMockStorage) CreateDomainAllow(ctx context.Context, allow *storage.DomainAllow) error {
	return nil
}
func (m *BaseMockStorage) DeleteDomainAllow(ctx context.Context, id string) error {
	return nil
}

// Federation instance tracking
func (m *BaseMockStorage) GetInstanceInfo(ctx context.Context, domain string) (*storage.InstanceInfo, error) {
	return nil, fmt.Errorf("not found")
}
func (m *BaseMockStorage) UpsertInstanceInfo(ctx context.Context, info *storage.InstanceInfo) error {
	return nil
}
func (m *BaseMockStorage) GetKnownInstances(ctx context.Context, limit int, cursor string) ([]*storage.InstanceInfo, string, error) {
	return []*storage.InstanceInfo{}, "", nil
}
func (m *BaseMockStorage) GetFederationStatistics(ctx context.Context, startTime, endTime time.Time) (*storage.FederationStats, error) {
	return &storage.FederationStats{}, nil
}

// Email domain blocks
func (m *BaseMockStorage) CreateEmailDomainBlock(ctx context.Context, block *storage.EmailDomainBlock) error {
	return nil
}
func (m *BaseMockStorage) GetEmailDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.EmailDomainBlock, string, error) {
	return []*storage.EmailDomainBlock{}, "", nil
}
func (m *BaseMockStorage) DeleteEmailDomainBlock(ctx context.Context, id string) error {
	return nil
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

// CreateFollow with mock support
func (m *MockStorage) CreateFollow(ctx context.Context, followerUsername, followedUsername, followActivityID string) error {
	args := m.Called(ctx, followerUsername, followedUsername, followActivityID)
	return args.Error(0)
}

// AcceptFollow with mock support
func (m *MockStorage) AcceptFollow(ctx context.Context, followerUsername, followedUsername string) error {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Error(0)
}

// CreateObject with mock support
func (m *MockStorage) CreateObject(ctx context.Context, object interface{}) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

// GetActorPrivateKey with mock support
func (m *MockStorage) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	args := m.Called(ctx, username)
	return args.String(0), args.Error(1)
}

// CreateFlag with mock support
func (m *MockStorage) CreateFlag(ctx context.Context, flag *storage.Flag) error {
	args := m.Called(ctx, flag)
	return args.Error(0)
}

// CreateMove with mock support
func (m *MockStorage) CreateMove(ctx context.Context, move *storage.Move) error {
	args := m.Called(ctx, move)
	return args.Error(0)
}

// AddToCollection with mock support
func (m *MockStorage) AddToCollection(ctx context.Context, collection string, item *storage.CollectionItem) error {
	args := m.Called(ctx, collection, item)
	return args.Error(0)
}

// RemoveFromCollection with mock support
func (m *MockStorage) RemoveFromCollection(ctx context.Context, collection string, itemID string) error {
	args := m.Called(ctx, collection, itemID)
	return args.Error(0)
}

// CreateLike with mock support
func (m *MockStorage) CreateLike(ctx context.Context, like *storage.Like) error {
	args := m.Called(ctx, like)
	return args.Error(0)
}

// GetActor with mock support
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

// CreateActivity with mock support
func (m *MockStorage) CreateActivity(ctx context.Context, activity *activitypub.Activity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

// GetUser with mock support
func (m *MockStorage) GetUser(ctx context.Context, username string) (*storage.User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

// GetAuthorizationCode with mock support
func (m *MockStorage) GetAuthorizationCode(ctx context.Context, code string) (*storage.AuthorizationCode, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.AuthorizationCode), args.Error(1)
}

// DeleteAuthorizationCode with mock support
func (m *MockStorage) DeleteAuthorizationCode(ctx context.Context, code string) error {
	args := m.Called(ctx, code)
	return args.Error(0)
}

// CreateRefreshToken with mock support
func (m *MockStorage) CreateRefreshToken(ctx context.Context, token *storage.RefreshToken) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

// GetRefreshToken with mock support
func (m *MockStorage) GetRefreshToken(ctx context.Context, token string) (*storage.RefreshToken, error) {
	args := m.Called(ctx, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.RefreshToken), args.Error(1)
}

// DeleteRefreshToken with mock support
func (m *MockStorage) DeleteRefreshToken(ctx context.Context, token string) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

// GetObjectsByActor with mock support
func (m *MockStorage) GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]interface{}, string, error) {
	args := m.Called(ctx, actorID, cursor, limit)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]interface{}), args.String(1), args.Error(2)
}

// IsFollowing with mock support
func (m *MockStorage) IsFollowing(ctx context.Context, followerUsername, followedUsername string) (bool, error) {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Bool(0), args.Error(1)
}

// GetLike with mock support
func (m *MockStorage) GetLike(ctx context.Context, actor, object string) (*storage.Like, error) {
	args := m.Called(ctx, actor, object)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Like), args.Error(1)
}

// DeleteLike with mock support
func (m *MockStorage) DeleteLike(ctx context.Context, actor, object string) error {
	args := m.Called(ctx, actor, object)
	return args.Error(0)
}

// GetCollection with mock support
func (m *MockStorage) GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error) {
	args := m.Called(ctx, username, collectionType, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.OrderedCollectionPage), args.Error(1)
}

// RemoveFollow with mock support
func (m *MockStorage) RemoveFollow(ctx context.Context, followerUsername, followedUsername string) error {
	args := m.Called(ctx, followerUsername, followedUsername)
	return args.Error(0)
}

// GetObject with mock support
func (m *MockStorage) GetObject(ctx context.Context, id string) (interface{}, error) {
	args := m.Called(ctx, id)
	return args.Get(0), args.Error(1)
}

// TombstoneObject with mock support
func (m *MockStorage) TombstoneObject(ctx context.Context, objectID string, deletedBy string) error {
	args := m.Called(ctx, objectID, deletedBy)
	return args.Error(0)
}

// UpdateObject with mock support
func (m *MockStorage) UpdateObject(ctx context.Context, object interface{}) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

// CreateAnnounce with mock support
func (m *MockStorage) CreateAnnounce(ctx context.Context, announce *storage.Announce) error {
	args := m.Called(ctx, announce)
	return args.Error(0)
}

// GetAnnounce with mock support
func (m *MockStorage) GetAnnounce(ctx context.Context, actor, object string) (*storage.Announce, error) {
	args := m.Called(ctx, actor, object)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Announce), args.Error(1)
}

// DeleteAnnounce with mock support
func (m *MockStorage) DeleteAnnounce(ctx context.Context, actor, object string) error {
	args := m.Called(ctx, actor, object)
	return args.Error(0)
}

// CreateBlock with mock support
func (m *MockStorage) CreateBlock(ctx context.Context, block *storage.Block) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

// GetActiveUserCount with mock support
func (m *MockStorage) GetActiveUserCount(ctx context.Context, days int) (int64, error) {
	args := m.Called(ctx, days)
	return args.Get(0).(int64), args.Error(1)
}

// GetOAuthClient with mock support
func (m *MockStorage) GetOAuthClient(ctx context.Context, clientID string) (*storage.OAuthClient, error) {
	args := m.Called(ctx, clientID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.OAuthClient), args.Error(1)
}

// GetStatusCount retrieves the number of statuses posted by an actor
func (m *BaseMockStorage) GetStatusCount(ctx context.Context, actorID string) (int, error) {
	return 0, nil
}

// GetFollowerCount retrieves the exact follower count for an actor
func (m *BaseMockStorage) GetFollowerCount(ctx context.Context, actorID string) (int, error) {
	return 0, nil
}

// GetLatestStatus retrieves the most recent status by an actor
func (m *BaseMockStorage) GetLatestStatus(ctx context.Context, actorID string) (*storage.StatusSearchResult, error) {
	return nil, nil
}

// GetCommunityNotesByAuthor retrieves community notes authored by a specific actor
func (m *BaseMockStorage) GetCommunityNotesByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*storage.CommunityNote, string, error) {
	return nil, "", nil
}

// GetCommunityNoteVotes retrieves votes on a specific community note
func (m *BaseMockStorage) GetCommunityNoteVotes(ctx context.Context, noteID string) ([]*storage.CommunityNoteVote, error) {
	return nil, nil
}
