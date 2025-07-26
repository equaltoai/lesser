package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/trust"
)

// Storage defines the interface for data storage operations
type Storage interface {
	// Actor operations
	CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error
	GetActor(ctx context.Context, username string) (*activitypub.Actor, error)
	GetActorByNumericID(ctx context.Context, numericID string) (*activitypub.Actor, error)
	GetActorWithMetadata(ctx context.Context, username string) (*activitypub.Actor, *ActorMetadata, error)
	GetActorPrivateKey(ctx context.Context, username string) (string, error)
	UpdateActor(ctx context.Context, actor *activitypub.Actor) error
	UpdateActorLastStatusTime(ctx context.Context, username string) error
	SetActorFields(ctx context.Context, username string, fields []ActorField) error
	DeleteActor(ctx context.Context, username string) error
	SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error)
	GetSearchSuggestions(ctx context.Context, prefix string) ([]SearchSuggestion, error)

	// Status search operations
	SearchStatuses(ctx context.Context, query string, limit int) ([]*StatusSearchResult, error)
	SearchStatusesWithOptions(ctx context.Context, query string, options StatusSearchOptions) ([]*StatusSearchResult, error)
	SearchStatusesByURL(ctx context.Context, url string) (*StatusSearchResult, error)

	// Enhanced search operations for GraphQL
	SearchAll(ctx context.Context, query string, limit int, accountID string) (*SearchResults, error)
	SearchAccountsAdvanced(ctx context.Context, query string, resolve bool, limit int, offset int, following bool, accountID string) ([]*activitypub.Actor, error)
	SearchStatusesAdvanced(ctx context.Context, query string, limit int, maxID, minID *string, accountID string) ([]*StatusSearchResult, error)
	SearchHashtagsAdvanced(ctx context.Context, query string, limit int, accountID string) ([]*HashtagSearchResult, error)

	// Activity operations
	CreateActivity(ctx context.Context, activity *activitypub.Activity) error
	GetActivity(ctx context.Context, id string) (*activitypub.Activity, error)
	GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error)
	GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error)

	// Object operations
	CreateObject(ctx context.Context, object any) error
	GetObject(ctx context.Context, id string) (any, error)
	UpdateObject(ctx context.Context, object any) error
	DeleteObject(ctx context.Context, id string) error
	GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]any, string, error)
	CountObjectReplies(ctx context.Context, objectID string) (int, error)

	// Update history operations
	CreateUpdateHistory(ctx context.Context, history *UpdateHistory) error
	GetUpdateHistory(ctx context.Context, objectID string, limit int) ([]*UpdateHistory, error)

	// Relationship operations
	CreateFollow(ctx context.Context, followerUsername, followedUsername, followActivityID string) error
	AcceptFollow(ctx context.Context, followerUsername, followedUsername string) error
	RejectFollow(ctx context.Context, followerUsername, followedUsername string) error
	RemoveFollow(ctx context.Context, followerUsername, followedUsername string) error
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	IsFollowing(ctx context.Context, followerUsername, followedUsername string) (bool, error)
	GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	GetFollowRequestState(ctx context.Context, followerUsername, followedUsername string) (string, error)

	// Collection operations
	GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error)

	// OAuth operations
	CreateAuthorizationCode(ctx context.Context, code *AuthorizationCode) error
	GetAuthorizationCode(ctx context.Context, code string) (*AuthorizationCode, error)
	DeleteAuthorizationCode(ctx context.Context, code string) error
	CreateRefreshToken(ctx context.Context, token *RefreshToken) error
	GetRefreshToken(ctx context.Context, token string) (*RefreshToken, error)
	DeleteRefreshToken(ctx context.Context, token string) error

	// User operations
	CreateUser(ctx context.Context, user *User) error
	GetUser(ctx context.Context, username string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUser(ctx context.Context, username string, updates map[string]any) error
	DeleteUser(ctx context.Context, username string) error
	ListUsers(ctx context.Context, limit int32, cursor string) ([]*User, string, error)
	GetActiveUserCount(ctx context.Context, days int) (int64, error)

	// Instance metrics operations
	GetTotalUserCount(ctx context.Context) (int64, error)
	GetTotalStatusCount(ctx context.Context) (int64, error)
	GetTotalDomainCount(ctx context.Context) (int64, error)
	GetWeeklyActivity(ctx context.Context, weekTimestamp int64) (*WeeklyActivity, error)
	RecordActivity(ctx context.Context, activityType string, actorID string, timestamp time.Time) error
	GetContactAccount(ctx context.Context) (*ActorRecord, error)

	// OAuth provider operations
	GetUserByProviderID(ctx context.Context, provider, providerID string) (*User, error)
	LinkProviderAccount(ctx context.Context, username, provider, providerID string) error
	UnlinkProviderAccount(ctx context.Context, username, provider string) error
	GetLinkedProviders(ctx context.Context, username string) ([]string, error)

	// Recovery operations
	StoreRecoveryToken(ctx context.Context, key string, data map[string]any) error
	GetRecoveryToken(ctx context.Context, key string) (map[string]any, error)
	DeleteRecoveryToken(ctx context.Context, key string) error

	// OAuth state operations
	StoreOAuthState(ctx context.Context, state string, data *OAuthState) error
	GetOAuthState(ctx context.Context, state string) (*OAuthState, error)
	DeleteOAuthState(ctx context.Context, state string) error

	// OAuth Client operations
	CreateOAuthClient(ctx context.Context, client *OAuthClient) error
	GetOAuthClient(ctx context.Context, clientID string) (*OAuthClient, error)
	UpdateOAuthClient(ctx context.Context, clientID string, updates map[string]any) error
	DeleteOAuthClient(ctx context.Context, clientID string) error
	ListOAuthClients(ctx context.Context, limit int32, cursor string) ([]*OAuthClient, string, error)

	// Like operations
	CreateLike(ctx context.Context, like *Like) error
	GetLike(ctx context.Context, actor, object string) (*Like, error)
	DeleteLike(ctx context.Context, actor, object string) error
	GetObjectLikes(ctx context.Context, objectID string, limit int, cursor string) ([]*Like, string, error)
	GetActorLikes(ctx context.Context, actorID string, limit int, cursor string) ([]*Like, string, error)
	CountObjectLikes(ctx context.Context, objectID string) (int, error)

	// Announce operations
	CreateAnnounce(ctx context.Context, announce *Announce) error
	GetAnnounce(ctx context.Context, actor, object string) (*Announce, error)
	DeleteAnnounce(ctx context.Context, actor, object string) error
	GetObjectAnnounces(ctx context.Context, objectID string, limit int, cursor string) ([]*Announce, string, error)
	GetActorAnnounces(ctx context.Context, actorID string, limit int, cursor string) ([]*Announce, string, error)
	CountObjectAnnounces(ctx context.Context, objectID string) (int, error)

	// Delete/Tombstone operations
	TombstoneObject(ctx context.Context, objectID string, deletedBy string) error
	GetTombstone(ctx context.Context, objectID string) (*Tombstone, error)
	CascadeDeleteLikes(ctx context.Context, objectID string) error
	CascadeDeleteAnnounces(ctx context.Context, objectID string) error

	// Block operations
	CreateBlock(ctx context.Context, block *Block) error
	GetBlock(ctx context.Context, actor, blockedActor string) (*Block, error)
	DeleteBlock(ctx context.Context, actor, blockedActor string) error
	GetBlockedActors(ctx context.Context, actor string, limit int, cursor string) ([]*Block, string, error)
	GetBlockedByActors(ctx context.Context, actor string, limit int, cursor string) ([]*Block, string, error)
	IsBlocked(ctx context.Context, actor, targetActor string) (bool, error)
	IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error)

	// Flag operations (content moderation)
	CreateFlag(ctx context.Context, flag *Flag) error
	GetFlag(ctx context.Context, id string) (*Flag, error)
	GetFlagsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*Flag, string, error)
	GetFlagsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*Flag, string, error)
	GetPendingFlags(ctx context.Context, limit int, cursor string) ([]*Flag, string, error)
	UpdateFlagStatus(ctx context.Context, id string, status FlagStatus, reviewedBy string, reviewNote string) error
	CountPendingFlags(ctx context.Context) (int, error)

	// Move operations (account migration)
	CreateMove(ctx context.Context, move *Move) error
	GetMove(ctx context.Context, actor string) (*Move, error)
	GetMoveByTarget(ctx context.Context, target string) ([]*Move, error)
	HasMovedFrom(ctx context.Context, oldActor, newActor string) (bool, error)

	// Collection operations (Add/Remove activities)
	AddToCollection(ctx context.Context, collection string, item *CollectionItem) error
	RemoveFromCollection(ctx context.Context, collection string, itemID string) error
	GetCollectionItems(ctx context.Context, collection string, limit int, cursor string) ([]*CollectionItem, string, error)
	IsInCollection(ctx context.Context, collection string, itemID string) (bool, error)
	CountCollectionItems(ctx context.Context, collection string) (int, error)

	// Timeline operations
	WriteToTimeline(ctx context.Context, timeline *TimelineEntry) error
	WriteToTimelines(ctx context.Context, entries []*TimelineEntry) error
	GetHomeTimeline(ctx context.Context, username string, limit int, cursor string) ([]*TimelineEntry, string, error)
	GetPublicTimeline(ctx context.Context, local bool, limit int, cursor string) ([]*TimelineEntry, string, error)
	GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*TimelineEntry, string, error)
	GetDirectTimeline(ctx context.Context, username string, limit int, cursor string) ([]*TimelineEntry, string, error)
	GetHashtagTimeline(ctx context.Context, hashtag string, local bool, limit int, cursor string) ([]*TimelineEntry, string, error)
	DeleteFromTimeline(ctx context.Context, timelineType, timelineID, entryID string) error
	DeleteExpiredTimelineEntries(ctx context.Context, before time.Time) error
	FanOutPost(ctx context.Context, activity *activitypub.Activity) error // Fan out posts to timelines

	// Instance configuration operations
	GetInstanceRules(ctx context.Context) ([]InstanceRule, error)
	SetInstanceRules(ctx context.Context, rules []InstanceRule) error
	GetExtendedDescription(ctx context.Context) (string, time.Time, error)
	SetExtendedDescription(ctx context.Context, description string) error

	// Bookmark operations
	CreateBookmark(ctx context.Context, username, objectID string) error
	RemoveBookmark(ctx context.Context, username, objectID string) error
	GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	IsBookmarked(ctx context.Context, username, objectID string) (bool, error)

	// Conversation operations
	CreateConversation(ctx context.Context, conversation *Conversation) error
	GetConversation(ctx context.Context, id string) (*Conversation, error)
	GetConversationByParticipants(ctx context.Context, participants []string) (*Conversation, error)
	UpdateConversationLastStatus(ctx context.Context, id, lastStatusID string) error
	MarkConversationRead(ctx context.Context, id, username string) error
	DeleteConversation(ctx context.Context, id string) error
	GetUserConversations(ctx context.Context, username string, limit int, cursor string) ([]*Conversation, string, error)
	AddParticipantToConversation(ctx context.Context, conversationID, participantID string) error

	// List operations
	CreateList(ctx context.Context, username, title, repliesPolicy string) (*List, error)
	GetList(ctx context.Context, listID string) (*List, error)
	GetListsForUser(ctx context.Context, username string) ([]*List, error)
	UpdateList(ctx context.Context, listID string, updates map[string]any) error
	DeleteList(ctx context.Context, listID string) error
	AddAccountsToList(ctx context.Context, listID string, accountIDs []string) error
	RemoveAccountsFromList(ctx context.Context, listID string, accountIDs []string) error
	GetListAccounts(ctx context.Context, listID string) ([]string, error)
	IsAccountInList(ctx context.Context, listID, accountID string) (bool, error)
	GetListsContainingAccount(ctx context.Context, accountID, username string) ([]*List, error)

	// Notification operations
	CreateNotification(ctx context.Context, notification *Notification) error
	GetNotification(ctx context.Context, id string) (*Notification, error)
	GetNotifications(ctx context.Context, username string, limit int, cursor string) ([]*Notification, string, error)
	GetNotificationsFiltered(ctx context.Context, username string, filter *NotificationFilter) ([]*Notification, string, error)
	MarkNotificationAsRead(ctx context.Context, id string) error
	MarkAllNotificationsAsRead(ctx context.Context, username string) error
	DeleteNotification(ctx context.Context, id string) error
	ClearNotifications(ctx context.Context, username string) error
	CountUnreadNotifications(ctx context.Context, username string) (int, error)

	// Enhanced notification operations for GraphQL
	GetNotificationsAdvanced(ctx context.Context, userID string, excludeTypes []string, maxID, sinceID, minID *string, limit int, includeFiltered bool) ([]*Notification, error)
	GetNotificationsByAccount(ctx context.Context, userID, accountID string, limit int) ([]*Notification, error)
	GetUnreadNotificationCount(ctx context.Context, userID string) (int64, error)

	// Remote actor caching operations
	CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error
	GetCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error)

	// Push notification operations
	CreatePushSubscription(ctx context.Context, username string, subscription *PushSubscription) error
	GetPushSubscription(ctx context.Context, username, subscriptionID string) (*PushSubscription, error)
	GetUserPushSubscriptions(ctx context.Context, username string) ([]*PushSubscription, error)
	UpdatePushSubscription(ctx context.Context, username, subscriptionID string, alerts PushSubscriptionAlerts) error
	DeletePushSubscription(ctx context.Context, username, subscriptionID string) error
	DeleteAllPushSubscriptions(ctx context.Context, username string) error

	// VAPID key operations
	GetVAPIDKeys(ctx context.Context) (*VAPIDKeys, error)
	SetVAPIDKeys(ctx context.Context, keys *VAPIDKeys) error

	// Poll operations
	CreatePoll(ctx context.Context, poll *Poll) error
	GetPoll(ctx context.Context, pollID string) (*Poll, error)
	GetPollByStatusID(ctx context.Context, statusID string) (*Poll, error)
	VoteOnPoll(ctx context.Context, pollID string, voterID string, choices []int) error
	GetPollVotes(ctx context.Context, pollID string) (map[string][]int, error)
	HasUserVoted(ctx context.Context, pollID string, userID string) (bool, []int, error)

	// Mute operations
	CreateMute(ctx context.Context, mute *Mute) error
	GetMute(ctx context.Context, actor, mutedActor string) (*Mute, error)
	DeleteMute(ctx context.Context, actor, mutedActor string) error
	GetMutedActors(ctx context.Context, actor string, limit int, cursor string) ([]*Mute, string, error)
	IsMuted(ctx context.Context, actor, targetActor string) (bool, error)

	// Filter operations (v2)
	CreateFilter(ctx context.Context, filter *Filter) error
	GetFilter(ctx context.Context, filterID string) (*Filter, error)
	GetFiltersForUser(ctx context.Context, username string) ([]*Filter, error)
	UpdateFilter(ctx context.Context, filterID string, updates map[string]any) error
	DeleteFilter(ctx context.Context, filterID string) error

	// Filter keyword operations
	AddFilterKeyword(ctx context.Context, filterID string, keyword *FilterKeyword) error
	GetFilterKeywords(ctx context.Context, filterID string) ([]*FilterKeyword, error)
	UpdateFilterKeyword(ctx context.Context, keywordID string, updates map[string]any) error
	DeleteFilterKeyword(ctx context.Context, keywordID string) error

	// Filter status operations
	AddFilterStatus(ctx context.Context, filterID string, status *FilterStatus) error
	GetFilterStatuses(ctx context.Context, filterID string) ([]*FilterStatus, error)
	DeleteFilterStatus(ctx context.Context, statusID string) error

	// Moderation operations
	CreateModerationEvent(ctx context.Context, event *ModerationEvent) error
	GetModerationEvent(ctx context.Context, eventID string) (*ModerationEvent, error)
	GetModerationQueuePaginated(ctx context.Context, limit int, cursor string) ([]*ModerationQueueItem, string, error)
	GetModerationEventsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*ModerationEvent, string, error)
	GetModerationEventsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*ModerationEvent, string, error)
	AddModerationReview(ctx context.Context, review *ModerationReview) error
	GetModerationReviews(ctx context.Context, eventID string) ([]*ModerationReview, error)
	CreateModerationDecision(ctx context.Context, decision *ModerationDecision) error
	GetModerationDecision(ctx context.Context, objectID string) (*ModerationDecision, error)
	GetModerationHistory(ctx context.Context, objectID string) (*ModerationHistory, error)
	// New admin-specific moderation methods
	GetModerationEvents(ctx context.Context, filter *ModerationEventFilter, limit int, cursor string) ([]*ModerationEvent, string, error)
	CreateAdminReview(ctx context.Context, eventID string, adminID string, action ActionType, reason string) error
	GetReviewerStats(ctx context.Context, reviewerID string) (*ReviewerStats, error)

	// Trust operations
	CreateTrustRelationship(ctx context.Context, relationship *TrustRelationship) error
	GetTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) (*TrustRelationship, error)
	UpdateTrustRelationship(ctx context.Context, relationship *TrustRelationship) error
	DeleteTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) error
	GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*TrustRelationship, string, error)
	GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*TrustRelationship, string, error)
	GetTrustScore(ctx context.Context, actorID, category string) (*TrustScore, error)
	UpdateTrustScore(ctx context.Context, score *TrustScore) error
	RecordTrustUpdate(ctx context.Context, update *TrustUpdate) error
	// New admin trust method
	GetAllTrustRelationships(ctx context.Context, limit int) ([]*TrustRelationship, error)

	// Account pin operations (endorsed accounts)
	CreateAccountPin(ctx context.Context, pin *AccountPin) error
	DeleteAccountPin(ctx context.Context, username, pinnedActorID string) error
	GetAccountPins(ctx context.Context, username string) ([]*AccountPin, error)
	IsAccountPinned(ctx context.Context, username, actorID string) (bool, error)
	CreateAccountNote(ctx context.Context, note *AccountNote) error
	GetAccountNote(ctx context.Context, username, targetActorID string) (*AccountNote, error)
	UpdateAccountNote(ctx context.Context, note *AccountNote) error
	DeleteAccountNote(ctx context.Context, username, targetActorID string) error
	RemoveFromFollowers(ctx context.Context, username, followerUsername string) error

	// Status pinning operations
	CreateStatusPin(ctx context.Context, pin *StatusPin) error
	DeleteStatusPin(ctx context.Context, username, statusID string) error
	GetStatusPins(ctx context.Context, username string) ([]*StatusPin, error)
	IsStatusPinned(ctx context.Context, username, statusID string) (bool, error)
	CountUserPinnedStatuses(ctx context.Context, username string) (int, error)

	// Conversation muting operations
	CreateConversationMute(ctx context.Context, mute *ConversationMute) error
	DeleteConversationMute(ctx context.Context, username, conversationID string) error
	IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error)
	GetMutedConversations(ctx context.Context, username string) ([]string, error)

	// Scheduled status operations
	CreateScheduledStatus(ctx context.Context, scheduled *ScheduledStatus) error
	GetScheduledStatus(ctx context.Context, id string) (*ScheduledStatus, error)
	GetScheduledStatuses(ctx context.Context, username string, limit int, cursor string) ([]*ScheduledStatus, string, error)
	UpdateScheduledStatus(ctx context.Context, scheduled *ScheduledStatus) error
	DeleteScheduledStatus(ctx context.Context, id string) error
	GetDueScheduledStatuses(ctx context.Context, before time.Time, limit int) ([]*ScheduledStatus, error)
	MarkScheduledStatusPublished(ctx context.Context, id string) error

	// Hashtag following
	FollowHashtag(ctx context.Context, userID string, hashtag string) error
	UnfollowHashtag(ctx context.Context, userID string, hashtag string) error
	IsFollowingHashtag(ctx context.Context, userID string, hashtag string) (bool, error)
	GetFollowedHashtags(ctx context.Context, userID string, limit int, cursor string) ([]string, string, error)

	// Enhanced hashtag operations for GraphQL
	UpdateHashtagNotificationSettings(ctx context.Context, userID, hashtag string, notify bool) error
	MuteHashtag(ctx context.Context, userID, hashtag string) error
	IsHashtagMuted(ctx context.Context, userID, hashtag string) (bool, error)
	GetHashtagTimelineAdvanced(ctx context.Context, hashtag string, maxID *string, limit int, userID string) ([]*StatusSearchResult, error)
	GetMultiHashtagTimeline(ctx context.Context, hashtags []string, maxID *string, limit int, userID string) ([]*StatusSearchResult, error)
	GetSuggestedHashtags(ctx context.Context, userID string, limit int) ([]*HashtagSearchResult, error)
	GetHashtagActivity(ctx context.Context, hashtag string, since time.Time) ([]*Activity, error)

	// Featured tags
	CreateFeaturedTag(ctx context.Context, userID string, tagName string) (*FeaturedTag, error)
	DeleteFeaturedTag(ctx context.Context, userID string, featuredTagID string) error
	GetFeaturedTags(ctx context.Context, userID string) ([]*FeaturedTag, error)
	GetTagSuggestions(ctx context.Context, userID string, limit int) ([]string, error)

	// Hashtag operations
	IndexHashtag(ctx context.Context, hashtag string, statusID string, authorID string, visibility string) error
	SearchHashtags(ctx context.Context, query string, limit int) ([]*Hashtag, error)
	GetHashtagInfo(ctx context.Context, hashtag string) (*Hashtag, error)
	GetHashtagUsageHistory(ctx context.Context, hashtag string, days int) ([]int64, error)

	// Language detection and user preferences
	GetUserLanguagePreference(ctx context.Context, username string) (string, error)
	SetUserLanguagePreference(ctx context.Context, username string, language string) error
	GetUserPreferences(ctx context.Context, username string) (*UserPreferences, error)
	UpdateUserPreferences(ctx context.Context, username string, preferences *UserPreferences) error

	// Search suggestion tracking and analytics
	TrackSearchQuery(ctx context.Context, userID, query string, resultCount int) error
	GetPopularSearchQueries(ctx context.Context, limit int, timeWindow time.Duration) ([]SearchQueryStats, error)
	GetUserSearchHistory(ctx context.Context, userID string, limit int) ([]SearchHistoryEntry, error)
	GenerateSearchSuggestions(ctx context.Context, userID, partialQuery string, limit int) ([]string, error)

	// Trending operations
	RecordHashtagUsage(ctx context.Context, hashtag string, statusID string, authorID string) error
	RecordStatusEngagement(ctx context.Context, statusID string, engagementType string, userID string) error
	RecordLinkShare(ctx context.Context, url string, statusID string, authorID string) error
	GetTrendingHashtags(ctx context.Context, since time.Time, limit int) ([]*TrendingHashtag, error)
	GetTrendingStatuses(ctx context.Context, since time.Time, limit int) ([]*TrendingStatus, error)
	GetTrendingLinks(ctx context.Context, since time.Time, limit int) ([]*TrendingLink, error)

	// Engagement metrics operations
	StoreEngagementMetrics(ctx context.Context, metrics *EngagementMetrics) error
	IndexByEngagement(ctx context.Context, statusID string, bucket string) error
	GetEngagementMetrics(ctx context.Context, statusID string) (*EngagementMetrics, error)

	// Announcement operations
	CreateAnnouncement(ctx context.Context, announcement *Announcement) error
	GetAnnouncement(ctx context.Context, id string) (*Announcement, error)
	GetAnnouncements(ctx context.Context, active bool) ([]*Announcement, error)
	UpdateAnnouncement(ctx context.Context, announcement *Announcement) error
	DeleteAnnouncement(ctx context.Context, id string) error
	DismissAnnouncement(ctx context.Context, username, announcementID string) error
	IsDismissed(ctx context.Context, username, announcementID string) (bool, error)
	GetDismissedAnnouncements(ctx context.Context, username string) ([]string, error)
	AddAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error
	RemoveAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error
	GetAnnouncementReactions(ctx context.Context, announcementID string) (map[string][]string, error)

	// Custom emoji operations
	CreateCustomEmoji(ctx context.Context, emoji *CustomEmoji) error
	GetCustomEmoji(ctx context.Context, shortcode string) (*CustomEmoji, error)
	GetCustomEmojis(ctx context.Context) ([]*CustomEmoji, error)
	UpdateCustomEmoji(ctx context.Context, emoji *CustomEmoji) error
	DeleteCustomEmoji(ctx context.Context, shortcode string) error
	GetCustomEmojisByCategory(ctx context.Context, category string) ([]*CustomEmoji, error)

	// Report operations
	CreateReport(ctx context.Context, report *Report) error
	GetReport(ctx context.Context, id string) (*Report, error)
	GetUserReports(ctx context.Context, username string, limit int, cursor string) ([]*Report, string, error)
	GetReportsByTarget(ctx context.Context, targetAccountID string, limit int, cursor string) ([]*Report, string, error)
	GetReportsByStatus(ctx context.Context, status ReportStatus, limit int, cursor string) ([]*Report, string, error)
	UpdateReportStatus(ctx context.Context, id string, status ReportStatus, actionTaken string, moderatorID string) error
	GetReportStats(ctx context.Context, username string) (*ReportStats, error)
	IncrementFalseReports(ctx context.Context, username string) error
	// New report assignment methods
	AssignReport(ctx context.Context, reportID string, assignedTo string) error
	UnassignReport(ctx context.Context, reportID string) error

	// Reputation-related operations
	GetStatusCount(ctx context.Context, actorID string) (int, error)
	GetFollowersCount(ctx context.Context, actorID string) (int, error)
	GetFollowingCount(ctx context.Context, actorID string) (int, error)
	GetLatestStatus(ctx context.Context, actorID string) (*StatusSearchResult, error)
	GetCommunityNotesByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*CommunityNote, string, error)
	GetCommunityNoteVotes(ctx context.Context, noteID string) ([]*CommunityNoteVote, error)

	// Community note operations
	CreateCommunityNote(ctx context.Context, note *CommunityNote) error
	GetCommunityNote(ctx context.Context, noteID string) (*CommunityNote, error)
	GetVisibleCommunityNotes(ctx context.Context, objectID string) ([]*CommunityNote, error)
	UpdateCommunityNoteScore(ctx context.Context, noteID string, score float64, status string) error
	CreateCommunityNoteVote(ctx context.Context, vote *CommunityNoteVote) error
	GetUserCommunityNoteVotes(ctx context.Context, userID string, noteIDs []string) (map[string]*CommunityNoteVote, error)
	CheckCommunityNoteRateLimit(ctx context.Context, userID string, limit int) (bool, int, error)

	// Domain block operations (user-level)
	AddDomainBlock(ctx context.Context, username, domain string) error
	RemoveDomainBlock(ctx context.Context, username, domain string) error
	GetUserDomainBlocks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	IsBlockedDomain(ctx context.Context, username, domain string) (bool, error)

	// Instance domain block operations (admin-level)
	CreateInstanceDomainBlock(ctx context.Context, block *InstanceDomainBlock) error
	GetInstanceDomainBlock(ctx context.Context, domain string) (*InstanceDomainBlock, error)
	GetInstanceDomainBlockByID(ctx context.Context, id string) (*InstanceDomainBlock, error)
	ListInstanceDomainBlocks(ctx context.Context, limit int, cursor string) ([]*InstanceDomainBlock, string, error)
	UpdateInstanceDomainBlock(ctx context.Context, domain string, updates map[string]any) error
	DeleteInstanceDomainBlock(ctx context.Context, domain string) error
	IsInstanceDomainBlocked(ctx context.Context, domain string) (bool, *InstanceDomainBlock, error)

	// Federation domain management operations (admin-level)
	GetDomainBlocks(ctx context.Context, limit int, cursor string) ([]*InstanceDomainBlock, string, error)
	GetDomainBlock(ctx context.Context, id string) (*InstanceDomainBlock, error)
	CreateDomainBlock(ctx context.Context, block *InstanceDomainBlock) error
	UpdateDomainBlock(ctx context.Context, id string, updates map[string]any) error
	DeleteDomainBlock(ctx context.Context, id string) error
	IsDomainBlocked(ctx context.Context, domain string) (bool, *InstanceDomainBlock, error)

	// Domain allow operations (for allowlist mode)
	GetDomainAllows(ctx context.Context, limit int, cursor string) ([]*DomainAllow, string, error)
	CreateDomainAllow(ctx context.Context, allow *DomainAllow) error
	DeleteDomainAllow(ctx context.Context, id string) error

	// Federation instance tracking
	GetInstanceInfo(ctx context.Context, domain string) (*InstanceInfo, error)
	UpsertInstanceInfo(ctx context.Context, info *InstanceInfo) error
	GetKnownInstances(ctx context.Context, limit int, cursor string) ([]*InstanceInfo, string, error)
	GetFederationStatistics(ctx context.Context, startTime, endTime time.Time) (*FederationStats, error)

	// Federation cost tracking
	RecordFederationActivity(ctx context.Context, activity *FederationActivity) error
	GetFederationCosts(ctx context.Context, startTime, endTime time.Time, limit int, cursor string) ([]*FederationCost, string, error)
	GetInstanceHealthReport(ctx context.Context, domain string, period time.Duration) (*InstanceHealthReport, error)
	GetCostProjections(ctx context.Context, period string) (*CostProjection, error)

	// Federation graph methods
	GetFederationNodes(ctx context.Context, depth int) ([]*FederationNode, error)
	GetFederationEdges(ctx context.Context, domains []string) ([]*FederationEdge, error)
	GetInstanceMetadata(ctx context.Context, domain string) (*InstanceMetadata, error)
	CalculateFederationClusters(ctx context.Context) ([]*InstanceCluster, error)
	GetInstanceConnections(ctx context.Context, domain string, connectionType string) ([]*InstanceConnection, error)
	GetRecentInstanceConnections(ctx context.Context, domain string, since time.Duration) ([]*InstanceConnection, error)
	UpdateFederationNode(ctx context.Context, node *FederationNode) error
	UpdateFederationEdge(ctx context.Context, edge *FederationEdge) error
	UpdateInstanceMetadata(ctx context.Context, metadata *InstanceMetadata) error
	GetStrongestConnectionsByType(ctx context.Context, connectionType string, limit int) ([]*FederationEdge, error)
	StoreFederationTimeSeries(ctx context.Context, data *FederationTimeSeries) error
	StoreInstanceCluster(ctx context.Context, cluster *InstanceCluster) error

	// Moderation pattern management
	CreateModerationPattern(ctx context.Context, pattern *ModerationPattern) error
	GetModerationPattern(ctx context.Context, patternID string) (*ModerationPattern, error)
	GetModerationPatterns(ctx context.Context, active bool, severity string, limit int) ([]*ModerationPattern, error)
	UpdateModerationPattern(ctx context.Context, pattern *ModerationPattern) error
	DeleteModerationPattern(ctx context.Context, patternID string) error
	RecordPatternMatch(ctx context.Context, patternID string, matched bool, timestamp time.Time) error
	StoreModerationDecision(ctx context.Context, decision *ModerationDecision) error
	UpdateModerationDecision(ctx context.Context, contentID string, review *ModerationReview) error
	GetModerationQueue(ctx context.Context, filter *ModerationFilter) ([]*ModerationQueueItem, error)

	// Email domain blocks
	CreateEmailDomainBlock(ctx context.Context, block *EmailDomainBlock) error
	GetEmailDomainBlocks(ctx context.Context, limit int, cursor string) ([]*EmailDomainBlock, string, error)
	DeleteEmailDomainBlock(ctx context.Context, id string) error

	// Marker operations
	SaveMarker(ctx context.Context, username, timeline string, lastReadID string, version int) error
	GetMarkers(ctx context.Context, username string, timelines []string) (map[string]*Marker, error)

	// Extended preferences operations
	SetPreference(ctx context.Context, username string, key string, value any) error
	GetPreference(ctx context.Context, username string, key string) (any, error)
	GetAllPreferences(ctx context.Context, username string) (map[string]any, error)
	UpdatePreferences(ctx context.Context, username string, prefs map[string]any) error

	// Streaming preferences operations
	GetStreamingPreferences(ctx context.Context, username string) (*StreamingPreferences, error)
	UpdateStreamingPreferences(ctx context.Context, prefs *StreamingPreferences) error
	GetStreamingPreferencesByDevice(ctx context.Context, username, deviceID string) (*StreamingPreferences, error)
	UpdateDeviceStreamingPreferences(ctx context.Context, prefs *StreamingPreferences, deviceID string) error
	GetStreamingPreferenceHistory(ctx context.Context, username string, limit int) ([]*StreamingPreferences, error)
	SyncStreamingPreferences(ctx context.Context, username string, sourceDeviceID string) error
	ResolvePreferenceConflict(ctx context.Context, username string, strategy ConflictResolutionStrategy) (*StreamingPreferences, error)

	// Session management operations
	CreateSession(ctx context.Context, session *Session) error
	GetSession(ctx context.Context, sessionID string) (*Session, error)
	GetSessionByRefreshToken(ctx context.Context, refreshToken string) (*Session, error)
	UpdateSession(ctx context.Context, session *Session) error
	DeleteSession(ctx context.Context, sessionID string) error
	GetUserSessions(ctx context.Context, username string) ([]*Session, error)

	// Device management operations
	CreateDevice(ctx context.Context, device *Device) error
	GetDevice(ctx context.Context, deviceID string) (*Device, error)
	UpdateDevice(ctx context.Context, device *Device) error
	GetUserDevices(ctx context.Context, username string) ([]*Device, error)

	// Rate limiting operations
	RecordLoginAttempt(ctx context.Context, identifier string, success bool) error
	GetLoginAttemptCount(ctx context.Context, identifier string, since time.Time) (int, error)
	IsRateLimited(ctx context.Context, identifier string) (bool, time.Time, error)
	ClearLoginAttempts(ctx context.Context, identifier string) error

	// WebAuthn operations
	StoreWebAuthnCredential(ctx context.Context, credential *WebAuthnCredential) error
	GetWebAuthnCredential(ctx context.Context, credentialID string) (*WebAuthnCredential, error)
	GetUserWebAuthnCredentials(ctx context.Context, username string) ([]*WebAuthnCredential, error)
	UpdateWebAuthnCredential(ctx context.Context, credential *WebAuthnCredential) error
	DeleteWebAuthnCredential(ctx context.Context, credentialID string) error

	// WebAuthn challenge operations
	StoreWebAuthnChallenge(ctx context.Context, challenge *WebAuthnChallenge) error
	GetWebAuthnChallenge(ctx context.Context, challengeID string) (*WebAuthnChallenge, error)
	DeleteWebAuthnChallenge(ctx context.Context, challengeID string) error

	// Wallet authentication operations
	StoreWalletChallenge(ctx context.Context, challenge *WalletChallenge) error
	GetWalletChallenge(ctx context.Context, challengeID string) (*WalletChallenge, error)
	DeleteWalletChallenge(ctx context.Context, challengeID string) error

	// Wallet credential operations
	StoreWalletCredential(ctx context.Context, credential *WalletCredential) error
	GetWalletCredential(ctx context.Context, walletType, address string) (*WalletCredential, error)
	GetUserWalletCredentials(ctx context.Context, username string) ([]*WalletCredential, error)
	DeleteWalletCredential(ctx context.Context, username, address string) error
	UpdateWalletLastUsed(ctx context.Context, username, address string) error

	// Social recovery operations
	StoreTrustee(ctx context.Context, username string, trustee *TrusteeConfig) error
	GetTrustees(ctx context.Context, username string) ([]*TrusteeConfig, error)
	DeleteTrustee(ctx context.Context, username, trusteeActorID string) error
	UpdateTrusteeConfirmed(ctx context.Context, username, trusteeActorID string, confirmed bool) error

	// Recovery request operations
	StoreRecoveryRequest(ctx context.Context, request *SocialRecoveryRequest) error
	GetRecoveryRequest(ctx context.Context, requestID string) (*SocialRecoveryRequest, error)
	UpdateRecoveryRequest(ctx context.Context, request *SocialRecoveryRequest) error
	DeleteRecoveryRequest(ctx context.Context, requestID string) error
	GetActiveRecoveryRequests(ctx context.Context, username string) ([]*SocialRecoveryRequest, error)

	// Recovery code operations
	StoreRecoveryCode(ctx context.Context, username string, code *RecoveryCodeItem) error
	GetRecoveryCodes(ctx context.Context, username string) ([]*RecoveryCodeItem, error)
	MarkRecoveryCodeUsed(ctx context.Context, username, codeHash string) error
	DeleteAllRecoveryCodes(ctx context.Context, username string) error
	CountUnusedRecoveryCodes(ctx context.Context, username string) (int, error)

	// Reputation storage operations
	StoreReputation(ctx context.Context, actorID string, reputation *Reputation) error
	GetReputation(ctx context.Context, actorID string) (*Reputation, error)
	GetReputationHistory(ctx context.Context, actorID string, limit int) ([]*Reputation, error)

	// Vouch operations
	CreateVouch(ctx context.Context, vouch *Vouch) error
	GetVouch(ctx context.Context, vouchID string) (*Vouch, error)
	GetVouchesByActor(ctx context.Context, actorID string, activeOnly bool) ([]*Vouch, error)
	GetVouchesForActor(ctx context.Context, actorID string, activeOnly bool) ([]*Vouch, error)
	UpdateVouchStatus(ctx context.Context, vouchID string, active bool, revokedAt *time.Time) error
	GetMonthlyVouchCount(ctx context.Context, actorID string, year int, month time.Month) (int, error)

	// DNS cache operations
	GetDNSCache(ctx context.Context, hostname string) (*DNSCacheEntry, error)
	SetDNSCache(ctx context.Context, entry *DNSCacheEntry) error

	// Reply operations
	GetReplies(ctx context.Context, objectID string, limit int, cursor string) ([]any, string, error)
	CountReplies(ctx context.Context, objectID string) (int, error)
	IncrementReplyCount(ctx context.Context, objectID string) error

	// Thread synchronization operations for GraphQL
	SyncThreadFromRemote(ctx context.Context, statusID string) (*StatusSearchResult, error)
	SyncMissingRepliesFromRemote(ctx context.Context, statusID string) ([]*StatusSearchResult, error)
	GetThreadContext(ctx context.Context, statusID string) (*ThreadContext, error)
	MarkThreadAsSynced(ctx context.Context, statusID string) error
	GetMissingReplies(ctx context.Context, statusID string) ([]*StatusSearchResult, error)

	// Reblog operations
	IncrementReblogCount(ctx context.Context, objectID string) error

	// Quote operations
	CreateQuoteRelationship(ctx context.Context, quote *QuoteRelationship) error
	GetQuotesForNote(ctx context.Context, noteID string, limit int, cursor string) ([]*QuoteRelationship, string, error)
	IsQuoted(ctx context.Context, actorID, noteID string) (bool, error)
	WithdrawQuote(ctx context.Context, quoteNoteID string) error
	CountQuotes(ctx context.Context, noteID string) (int, error)

	// Enhanced quote operations for GraphQL
	WithdrawStatusFromQuotes(ctx context.Context, statusID string) error
	UpdateQuotePermissions(ctx context.Context, statusID string, permissions *QuotePermissions) error
	IsQuoteAllowed(ctx context.Context, statusID, quoterID string) (bool, error)
	GetQuoteType(ctx context.Context, statusID string) (string, error)
	IsWithdrawnFromQuotes(ctx context.Context, statusID string) (bool, error)
	GetQuotesOfStatus(ctx context.Context, statusID string, limit int) ([]*StatusSearchResult, error)

	// Additional missing methods for resolving compilation errors
	IsNotificationEnabled(ctx context.Context, userID, targetID string) (bool, error)
	IsNotificationMuted(ctx context.Context, userID, targetID string) (bool, error)
	HasPendingFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error)
	GetFieldVerification(ctx context.Context, username, fieldName string) (*ActorField, error)
	GetDomainStats(ctx context.Context, domain string) (any, error)
	UnmarkAllMediaAsSensitive(ctx context.Context, username string) error
	GetUserMedia(ctx context.Context, username string) ([]any, error)
	UpdateMediaAttachment(ctx context.Context, mediaID string, updates map[string]any) error
	GetLocalPostCount(ctx context.Context) (int64, error)
	SaveOAuthState(ctx context.Context, state *OAuthState) error
	GetOAuthApp(ctx context.Context, clientID string) (*OAuthApp, error)
	SaveUserAppConsent(ctx context.Context, consent *UserAppConsent) error
	GetLikeCount(ctx context.Context, statusID string) (int64, error)
	GetBoostCount(ctx context.Context, statusID string) (int64, error)
	GetReplyCount(ctx context.Context, statusID string) (int64, error)
	GetModerationQueueCount(ctx context.Context) (int, error)
	GetOpenReportsCount(ctx context.Context) (int, error)
	ListUsersByRole(ctx context.Context, role string) ([]*User, error)
	GetAccountSuggestions(ctx context.Context, userID string, limit int) ([]*activitypub.Actor, error)
	RemoveAccountSuggestion(ctx context.Context, userID, targetID string) error
	GetFollowRequest(ctx context.Context, followerID, targetID string) (*RelationshipRecord, error)
	AcceptFollowRequest(ctx context.Context, followerID, targetID string) error
	RejectFollowRequest(ctx context.Context, followerID, targetID string) error
	HasFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error)
	IsEndorsed(ctx context.Context, userID, targetID string) (bool, error)
	GetRelationshipNote(ctx context.Context, userID, targetID string) (*AccountNote, error)
	GetStatusReplyCount(ctx context.Context, statusID string) (int, error)
	GetHashtagStats(ctx context.Context, hashtag string) (any, error)
	GetUserStatusCount(ctx context.Context, userID string) (int, error)
	GetStorageUsage(ctx context.Context) (any, error)
	GetStorageHistory(ctx context.Context, days int) ([]any, error)
	GetUserGrowthHistory(ctx context.Context, days int) ([]any, error)
	GetDailyActiveUserCount(ctx context.Context) (int64, error)
	GetStatus(ctx context.Context, statusID string) (any, error)
	GetReportedStatuses(ctx context.Context, reportID string) ([]any, error)
	GetRulesByCategory(ctx context.Context, category string) ([]InstanceRule, error)
	GetUserAppConsent(ctx context.Context, userID, appID string) (*UserAppConsent, error)
	GetScheduledStatusMedia(ctx context.Context, statusID string) ([]any, error)
	GetRecentHashtags(ctx context.Context, since time.Time, limit int) ([]*TrendingHashtag, error)
	StoreHashtagTrend(ctx context.Context, trend any) error
	GetRecentStatusesWithEngagement(ctx context.Context, since time.Time, limit int) ([]*TrendingStatus, error)
	GetUserTrustScore(ctx context.Context, userID string) (float64, error)
	StoreStatusTrend(ctx context.Context, trend any) error
	GetRecentLinks(ctx context.Context, since time.Time, limit int) ([]*TrendingLink, error)
	StoreLinkTrend(ctx context.Context, trend any) error
	DeleteOldHashtagTrends(ctx context.Context, before time.Time) error
	DeleteOldStatusTrends(ctx context.Context, before time.Time) error
	DeleteOldLinkTrends(ctx context.Context, before time.Time) error
	GetStatusesByLink(ctx context.Context, linkURL string, limit int) ([]any, error)

	// Relay operations
	StoreRelayInfo(ctx context.Context, relay *RelayInfo) error
	GetRelayInfo(ctx context.Context, relayURL string) (*RelayInfo, error)
	RemoveRelayInfo(ctx context.Context, relayURL string) error
	GetActiveRelays(ctx context.Context) ([]*RelayInfo, error)
	GetAllRelays(ctx context.Context, limit int, cursor string) ([]*RelayInfo, string, error)
	UpdateRelayStatus(ctx context.Context, relayURL string, active bool) error

	// Federation severance operations for GraphQL
	AcknowledgeSeverance(ctx context.Context, userID, domain string) error
	AttemptReconnection(ctx context.Context, userID, domain string) error
	GetUserSeveredRelationships(ctx context.Context, userID string) ([]*SeveredRelationship, error)
	GetAffectedRelationships(ctx context.Context, userID, domain string) ([]*RelationshipRecord, error)
	TrackFederationIssue(ctx context.Context, domain, issueType string) error
}

// User represents a user account in the system
type User struct {
	Username     string    `dynamodbav:"username"`
	Email        string    `dynamodbav:"email,omitempty"`         // Optional - not required for email-free auth
	PasswordHash string    `dynamodbav:"password_hash,omitempty"` // Optional - not required for passkey/wallet auth
	DisplayName  string    `dynamodbav:"display_name,omitempty"`  // Display name for the user
	CreatedAt    time.Time `dynamodbav:"created_at"`
	UpdatedAt    time.Time `dynamodbav:"updated_at"`
	Approved     bool      `dynamodbav:"approved"`
	Suspended    bool      `dynamodbav:"suspended"`
	Silenced     bool      `dynamodbav:"silenced"`
	Role         string    `dynamodbav:"role"` // user, moderator, admin
	Locale       string    `dynamodbav:"locale,omitempty"`

	// Recovery options (email-free)
	RecoveryMethods []string `dynamodbav:"recovery_methods,omitempty"` // ["passkey", "wallet", "social", "recovery_code"]
}

// OAuthClient represents an OAuth client application
type OAuthClient struct {
	ClientID     string    `dynamodbav:"client_id"`
	ClientSecret string    `dynamodbav:"client_secret"`
	Name         string    `dynamodbav:"name"`
	Website      string    `dynamodbav:"website,omitempty"`
	RedirectURIs []string  `dynamodbav:"redirect_uris"`
	Scopes       []string  `dynamodbav:"scopes,omitempty"`
	CreatedAt    time.Time `dynamodbav:"created_at"`
	UpdatedAt    time.Time `dynamodbav:"updated_at,omitempty"`
}

// Like represents a Like activity
type Like struct {
	Actor     string    `dynamodbav:"actor"`      // Who liked
	Object    string    `dynamodbav:"object"`     // What was liked
	ID        string    `dynamodbav:"id"`         // Like activity ID
	Published time.Time `dynamodbav:"published"`  // When it was liked
	CreatedAt time.Time `dynamodbav:"created_at"` // When stored in DB
}

// Announce represents an Announce activity (boost/repost)
type Announce struct {
	Actor     string    `dynamodbav:"actor"`        // Who announced
	Object    string    `dynamodbav:"object"`       // What was announced
	ID        string    `dynamodbav:"id"`           // Announce activity ID
	Published time.Time `dynamodbav:"published"`    // When it was announced
	CreatedAt time.Time `dynamodbav:"created_at"`   // When stored in DB
	To        []string  `dynamodbav:"to,omitempty"` // Audience
	CC        []string  `dynamodbav:"cc,omitempty"` // CC audience
}

// Tombstone represents a deleted object
type Tombstone struct {
	ID         string    `dynamodbav:"id"`                // Original object ID
	Type       string    `dynamodbav:"type"`              // Always "Tombstone"
	FormerType string    `dynamodbav:"formerType"`        // Original object type
	Deleted    time.Time `dynamodbav:"deleted"`           // When it was deleted
	DeletedBy  string    `dynamodbav:"deletedBy"`         // Actor who deleted it
	Summary    string    `dynamodbav:"summary,omitempty"` // Optional deletion reason
}

// Block represents a block relationship
type Block struct {
	Actor     string    // The actor doing the blocking
	Object    string    // The actor being blocked
	ID        string    // The block activity ID
	Published time.Time // When the block was created
	CreatedAt time.Time // Database timestamp
}

// UpdateHistory represents the edit history of an object
type UpdateHistory struct {
	ObjectID      string    `dynamodbav:"objectId"`          // The object that was updated
	Version       int       `dynamodbav:"version"`           // Version number (1 is original)
	UpdatedAt     time.Time `dynamodbav:"updatedAt"`         // When the update occurred
	UpdatedBy     string    `dynamodbav:"updatedBy"`         // Actor who made the update
	PreviousState string    `dynamodbav:"previousState"`     // JSON of previous state
	Summary       string    `dynamodbav:"summary,omitempty"` // Edit summary
}

// ActorRecord represents an actor stored in DynamoDB
type ActorRecord struct {
	PK           string             `dynamodbav:"PK"`
	SK           string             `dynamodbav:"SK"`
	Actor        *activitypub.Actor `dynamodbav:"Actor"`
	Username     string             `dynamodbav:"Username,omitempty"` // Local username for convenience
	PrivateKey   string             `dynamodbav:"PrivateKey,omitempty"`
	CreatedAt    time.Time          `dynamodbav:"CreatedAt"`
	UpdatedAt    time.Time          `dynamodbav:"UpdatedAt"`
	LastStatusAt *time.Time         `dynamodbav:"LastStatusAt,omitempty"`
	Fields       []ActorField       `dynamodbav:"Fields,omitempty"`
}

// ActorMetadata contains additional metadata about an actor
type ActorMetadata struct {
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	LastStatusAt *time.Time   `json:"last_status_at,omitempty"`
	Fields       []ActorField `json:"fields,omitempty"`
}

// ActorField represents a profile field (like bio fields in Mastodon)
type ActorField struct {
	Name       string     `json:"name" dynamodbav:"name"`
	Value      string     `json:"value" dynamodbav:"value"`
	VerifiedAt *time.Time `json:"verified_at,omitempty" dynamodbav:"verified_at,omitempty"`
}

// ActivityRecord represents an activity stored in DynamoDB
type ActivityRecord struct {
	PK        string `dynamodbav:"PK"`
	SK        string `dynamodbav:"SK"`
	GSI1PK    string `dynamodbav:"GSI1PK,omitempty"`
	GSI1SK    string `dynamodbav:"GSI1SK,omitempty"`
	Activity  *activitypub.Activity
	CreatedAt time.Time `dynamodbav:"CreatedAt"`
}

// RelationshipRecord represents a follow relationship in DynamoDB
type RelationshipRecord struct {
	PK         string    `dynamodbav:"PK"`
	SK         string    `dynamodbav:"SK"`
	GSI1PK     string    `dynamodbav:"GSI1PK"`
	GSI1SK     string    `dynamodbav:"GSI1SK"`
	ActivityID string    `dynamodbav:"ActivityID"`
	State      string    `dynamodbav:"State"` // pending, accepted, rejected
	CreatedAt  time.Time `dynamodbav:"CreatedAt"`
	UpdatedAt  time.Time `dynamodbav:"UpdatedAt"`
}

// ObjectRecord represents an object stored in DynamoDB
type ObjectRecord struct {
	PK        string    `dynamodbav:"PK"`
	SK        string    `dynamodbav:"SK"`
	Type      string    `dynamodbav:"Type"`
	Object    any       `dynamodbav:"Object"`
	CreatedAt time.Time `dynamodbav:"CreatedAt"`
	UpdatedAt time.Time `dynamodbav:"UpdatedAt"`
}

// AuthorizationCode represents an OAuth authorization code
type AuthorizationCode struct {
	Code          string    `dynamodbav:"Code"`
	ClientID      string    `dynamodbav:"ClientID"`
	Username      string    `dynamodbav:"Username"`
	CodeChallenge string    `dynamodbav:"CodeChallenge"`
	ExpiresAt     time.Time `dynamodbav:"ExpiresAt"`
	Scopes        []string  `dynamodbav:"Scopes"`
}

// RefreshToken represents an OAuth refresh token
type RefreshToken struct {
	Token     string    `dynamodbav:"Token"`
	ClientID  string    `dynamodbav:"ClientID"`
	Username  string    `dynamodbav:"Username"`
	ExpiresAt time.Time `dynamodbav:"ExpiresAt"`
	Scopes    []string  `dynamodbav:"Scopes"`
}

// AuthorizationCodeRecord represents an authorization code stored in DynamoDB
type AuthorizationCodeRecord struct {
	PK        string             `dynamodbav:"PK"`
	SK        string             `dynamodbav:"SK"`
	Code      *AuthorizationCode `dynamodbav:"Code"`
	CreatedAt time.Time          `dynamodbav:"CreatedAt"`
}

// RefreshTokenRecord represents a refresh token stored in DynamoDB
type RefreshTokenRecord struct {
	PK        string        `dynamodbav:"PK"`
	SK        string        `dynamodbav:"SK"`
	Token     *RefreshToken `dynamodbav:"Token"`
	CreatedAt time.Time     `dynamodbav:"CreatedAt"`
}

// OAuthState represents OAuth state stored for CSRF protection
type OAuthState struct {
	State               string    `dynamodbav:"State"`
	Provider            string    `dynamodbav:"Provider"`
	RedirectURI         string    `dynamodbav:"RedirectURI"`
	Username            string    `dynamodbav:"Username,omitempty"` // For account linking
	ClientID            string    `dynamodbav:"ClientID,omitempty"` // For standard OAuth
	Scopes              []string  `dynamodbav:"Scopes,omitempty"`
	CodeChallenge       string    `dynamodbav:"CodeChallenge,omitempty"`       // For PKCE
	CodeChallengeMethod string    `dynamodbav:"CodeChallengeMethod,omitempty"` // For PKCE
	CreatedAt           time.Time `dynamodbav:"CreatedAt"`
	ExpiresAt           time.Time `dynamodbav:"ExpiresAt"` // For TTL
}

// UserAppConsent represents user consent for an OAuth app
type UserAppConsent struct {
	UserID    string    `dynamodbav:"UserID"`
	AppID     string    `dynamodbav:"AppID"`
	Scopes    []string  `dynamodbav:"Scopes"`
	CreatedAt time.Time `dynamodbav:"CreatedAt"`
}

// OAuthApp represents an OAuth application
type OAuthApp struct {
	ClientID     string    `dynamodbav:"ClientID"`
	ClientSecret string    `dynamodbav:"ClientSecret"`
	Name         string    `dynamodbav:"Name"`
	RedirectURIs []string  `dynamodbav:"RedirectURIs"`
	Scopes       []string  `dynamodbav:"Scopes"`
	CreatedAt    time.Time `dynamodbav:"CreatedAt"`
}

// EngagementMetrics represents engagement statistics for a status
type EngagementMetrics struct {
	StatusID         string  `dynamodbav:"StatusID"`
	LikeCount        int64   `dynamodbav:"LikeCount"`
	BoostCount       int64   `dynamodbav:"BoostCount"`
	ReplyCount       int64   `dynamodbav:"ReplyCount"`
	Score            float64 `dynamodbav:"Score"`
	EngagementBucket string  `dynamodbav:"EngagementBucket"`
}

// InstanceRule represents a server rule
type InstanceRule struct {
	ID   string `json:"id" dynamodbav:"ID"`
	Text string `json:"text" dynamodbav:"Text"`
}

// Constants for DynamoDB key patterns
const (
	// Actor keys
	ActorPKPrefix = "ACTOR#"
	ActorSK       = "PROFILE"

	// Activity keys
	ActivitySKPrefix  = "ACTIVITY#"
	InboxGSI1PKPrefix = "INBOX#"

	// Relationship keys
	FollowPKPrefix    = "FOLLOW#"
	FollowingSKPrefix = "FOLLOWING#"
	FollowerSKPrefix  = "FOLLOWER#"

	// Object keys
	ObjectPKPrefix  = "OBJECT#"
	VersionSKPrefix = "VERSION#"

	// OAuth keys
	AuthCodePKPrefix     = "AUTHCODE#"
	AuthCodeSK           = "CODE"
	RefreshTokenPKPrefix = "REFRESHTOKEN#"
	RefreshTokenSK       = "TOKEN"

	// Relationship states
	RelationshipPending  = "pending"
	RelationshipAccepted = "accepted"
	RelationshipRejected = "rejected"
)

// FlagStatus represents the status of a flag
type FlagStatus string

const (
	FlagStatusPending   FlagStatus = "pending"
	FlagStatusReviewed  FlagStatus = "reviewed"
	FlagStatusResolved  FlagStatus = "resolved"
	FlagStatusDismissed FlagStatus = "dismissed"
)

// Flag represents a Flag activity for content moderation
type Flag struct {
	ID         string     // The flag activity ID
	Actor      string     // Who flagged
	Object     []string   // What was flagged (can be multiple objects)
	Content    string     // Reason/description for the flag
	Published  time.Time  // When it was flagged
	Status     FlagStatus // Current status of the flag
	ReviewedBy string     // Moderator who reviewed (if reviewed)
	ReviewedAt *time.Time // When it was reviewed
	ReviewNote string     // Note from reviewer
	CreatedAt  time.Time  // Database timestamp
}

// Move represents a Move activity for account migration
type Move struct {
	ID        string    // The move activity ID
	Actor     string    // The old account moving
	Target    string    // The new account location
	Published time.Time // When the move was announced
	CreatedAt time.Time // Database timestamp
}

// CollectionItem represents an item in a collection (for Add/Remove activities)
type CollectionItem struct {
	Collection string    // The collection ID (e.g., featured, likes, etc.)
	ItemID     string    // The item being added/removed
	ItemType   string    // Type of the item (Note, Article, etc.)
	AddedBy    string    // Who added the item
	AddedAt    time.Time // When it was added
	Position   int       // Optional position in ordered collections
}

// TimelineEntry represents an entry in a user's timeline
type TimelineEntry struct {
	TimelineType string    // HOME, PUBLIC, LIST
	TimelineID   string    // Username for HOME, LOCAL/FEDERATED for PUBLIC, list ID for LIST
	EntryID      string    // Unique ID for this entry (usually timestamp + post ID)
	PostID       string    // The actual post/object ID
	ActorID      string    // Who created the post
	ActorHandle  string    // Actor's handle for quick display
	Content      string    // First 500 chars for preview
	ContentType  string    // Note, Article, etc.
	HasMedia     bool      // Quick flag for media
	IsReply      bool      // Is this a reply?
	InReplyTo    string    // ID of post being replied to
	IsBoost      bool      // Is this a boost/announce?
	BoostedBy    string    // Who boosted it (if applicable)
	Visibility   string    // public, unlisted, private, direct
	Language     string    // Language code
	Sensitive    bool      // Content warning flag
	SpoilerText  string    // Content warning text
	CreatedAt    time.Time // When the post was created
	TimelineAt   time.Time // When it was added to timeline (for sorting)
	ExpiresAt    time.Time // TTL for auto-deletion
}

// Conversation represents a direct message conversation
type Conversation struct {
	ID           string    `dynamodbav:"id"`
	Participants []string  `dynamodbav:"participants"` // Actor IDs
	LastStatusID string    `dynamodbav:"last_status_id"`
	CreatedAt    time.Time `dynamodbav:"created_at"`
	UpdatedAt    time.Time `dynamodbav:"updated_at"`
}

// ConversationStatus tracks read status per user
type ConversationStatus struct {
	ConversationID string    `dynamodbav:"conversation_id"`
	UserID         string    `dynamodbav:"user_id"`
	Unread         bool      `dynamodbav:"unread"`
	LastReadAt     time.Time `dynamodbav:"last_read_at"`
}

// List represents a user-created list for organizing followed accounts
type List struct {
	ID            string    `dynamodbav:"id"`
	Username      string    `dynamodbav:"username"` // Owner of the list
	Title         string    `dynamodbav:"title"`
	RepliesPolicy string    `dynamodbav:"replies_policy"` // "followed", "list", or "none"
	CreatedAt     time.Time `dynamodbav:"created_at"`
	UpdatedAt     time.Time `dynamodbav:"updated_at"`
}

// ListMember represents membership of an account in a list
type ListMember struct {
	ListID    string    `dynamodbav:"list_id"`
	AccountID string    `dynamodbav:"account_id"`
	AddedAt   time.Time `dynamodbav:"added_at"`
}

// Notification represents a notification to a user
type Notification struct {
	ID        string    `dynamodbav:"id"`
	Type      string    `dynamodbav:"type"`                // follow, mention, favourite, reblog
	Username  string    `dynamodbav:"username"`            // Recipient of the notification
	AccountID string    `dynamodbav:"account_id"`          // Who triggered the notification
	StatusID  string    `dynamodbav:"status_id,omitempty"` // Related status (if any)
	Read      bool      `dynamodbav:"read"`
	CreatedAt time.Time `dynamodbav:"created_at"`
}

// NotificationFilter represents parameters for filtering notifications
type NotificationFilter struct {
	Types        []string // Filter by notification types
	ExcludeTypes []string // Exclude specific notification types
	AccountID    string   // Filter by account
	Limit        int      // Maximum number to return
	MinID        string   // Return results newer than this ID
	MaxID        string   // Return results older than this ID
	SinceID      string   // Return results newer than this ID (for polling)
}

// SearchSuggestion represents a search suggestion
type SearchSuggestion struct {
	Type  string `json:"type"`  // account, hashtag, etc.
	Value string `json:"value"` // The suggested value
	Score int    `json:"score"` // Relevance score
}

// StatusSearchResult represents a status search result
type StatusSearchResult struct {
	StatusID       string
	Content        string
	URL            string
	AuthorID       string
	AuthorUsername string
	Published      time.Time
	Score          float64
	Highlights     map[string]string
}

// PushSubscription represents a web push subscription
type PushSubscription struct {
	ID        string                 `dynamodbav:"id"`
	Username  string                 `dynamodbav:"username"`
	Endpoint  string                 `dynamodbav:"endpoint"`
	P256dh    string                 `dynamodbav:"p256dh"`
	Auth      string                 `dynamodbav:"auth"`
	Alerts    PushSubscriptionAlerts `dynamodbav:"alerts"`
	Policy    string                 `dynamodbav:"policy,omitempty"`
	CreatedAt time.Time              `dynamodbav:"created_at"`
	UpdatedAt time.Time              `dynamodbav:"updated_at"`
}

// PushSubscriptionAlerts represents which events trigger push notifications
type PushSubscriptionAlerts struct {
	Follow        bool `dynamodbav:"follow"`
	Favourite     bool `dynamodbav:"favourite"`
	Reblog        bool `dynamodbav:"reblog"`
	Mention       bool `dynamodbav:"mention"`
	Poll          bool `dynamodbav:"poll"`
	FollowRequest bool `dynamodbav:"follow_request"`
	Status        bool `dynamodbav:"status"`
	Update        bool `dynamodbav:"update"`
	AdminSignUp   bool `dynamodbav:"admin_sign_up"`
	AdminReport   bool `dynamodbav:"admin_report"`
}

// VAPIDKeys represents the VAPID keys for web push
type VAPIDKeys struct {
	PublicKey  string    `dynamodbav:"public_key"`
	PrivateKey string    `dynamodbav:"private_key"`
	Subject    string    `dynamodbav:"subject"`
	CreatedAt  time.Time `dynamodbav:"created_at"`
}

// Poll represents a poll in the storage layer
type Poll struct {
	ID          string           `dynamodbav:"id"`
	StatusID    string           `dynamodbav:"statusId"`   // The status this poll belongs to
	CreatedBy   string           `dynamodbav:"createdBy"`  // Actor ID who created the poll
	Options     []string         `dynamodbav:"options"`    // Poll options
	Multiple    bool             `dynamodbav:"multiple"`   // Allow multiple choices
	HideTotals  bool             `dynamodbav:"hideTotals"` // Hide vote counts until poll ends
	ExpiresAt   time.Time        `dynamodbav:"expiresAt"`  // When the poll expires
	CreatedAt   time.Time        `dynamodbav:"createdAt"`
	UpdatedAt   time.Time        `dynamodbav:"updatedAt"`
	VotesCount  int              `dynamodbav:"votesCount"`  // Total number of votes
	VotersCount int              `dynamodbav:"votersCount"` // Total number of voters
	Votes       map[string][]int `dynamodbav:"votes"`       // Map of voter ID to option indices
}

// Mute represents a mute relationship
type Mute struct {
	Actor             string    `dynamodbav:"actor"`              // The actor doing the muting
	Object            string    `dynamodbav:"object"`             // The actor being muted
	ID                string    `dynamodbav:"id"`                 // The mute activity ID
	HideNotifications bool      `dynamodbav:"hide_notifications"` // Whether to hide notifications from this user
	Published         time.Time `dynamodbav:"published"`          // When the mute was created
	CreatedAt         time.Time `dynamodbav:"created_at"`         // Database timestamp
}

// Filter represents a filter in the storage layer
type Filter struct {
	ID           string     `dynamodbav:"id"`
	Username     string     `dynamodbav:"username"`
	Title        string     `dynamodbav:"title"`
	Context      []string   `dynamodbav:"context"`       // home, notifications, public, thread, account
	FilterAction string     `dynamodbav:"filter_action"` // warn, hide, blur
	ExpiresAt    *time.Time `dynamodbav:"expires_at,omitempty"`
	CreatedAt    time.Time  `dynamodbav:"created_at"`
	UpdatedAt    time.Time  `dynamodbav:"updated_at"`
}

// FilterKeyword represents a keyword in a filter
type FilterKeyword struct {
	ID        string    `dynamodbav:"id"`
	FilterID  string    `dynamodbav:"filter_id"`
	Keyword   string    `dynamodbav:"keyword"`
	WholeWord bool      `dynamodbav:"whole_word"`
	CreatedAt time.Time `dynamodbav:"created_at"`
}

// FilterStatus represents a status in a filter
type FilterStatus struct {
	ID        string    `dynamodbav:"id"`
	FilterID  string    `dynamodbav:"filter_id"`
	StatusID  string    `dynamodbav:"status_id"`
	CreatedAt time.Time `dynamodbav:"created_at"`
}

// Type aliases for moderation and trust types to avoid repetition
type (
	ModerationEvent     = moderation.ModerationEvent
	ModerationReview    = moderation.Review
	ModerationDecision  = moderation.ModerationDecision
	ModerationHistory   = moderation.ModerationHistory
	ModerationQueueItem = moderation.QueueItem
	EventType           = moderation.EventType
	Category            = moderation.Category
	Severity            = moderation.Severity
	ActionType          = moderation.ActionType
)

type (
	TrustRelationship = trust.TrustRelationship
	TrustScore        = trust.TrustScore
	TrustUpdate       = trust.TrustUpdate
	TrustCategory     = trust.TrustCategory
)

// Constant aliases for moderation types
const (
	// Severity levels
	SeverityLow      = moderation.SeverityLow
	SeverityMedium   = moderation.SeverityMedium
	SeverityHigh     = moderation.SeverityHigh
	SeverityCritical = moderation.SeverityCritical

	// Action types
	ActionTypeNone    = moderation.ActionTypeNone
	ActionTypeWarning = moderation.ActionTypeWarning
	ActionTypeSilence = moderation.ActionTypeSilence
	ActionTypeSuspend = moderation.ActionTypeSuspend
	ActionTypeRemove  = moderation.ActionTypeRemove
)

// AccountPin represents a pinned/endorsed account
type AccountPin struct {
	Username       string    `dynamodbav:"username"`        // Who pinned the account
	PinnedActorID  string    `dynamodbav:"pinned_actor_id"` // The actor ID that was pinned
	PinnedUsername string    `dynamodbav:"pinned_username"` // The username that was pinned
	CreatedAt      time.Time `dynamodbav:"created_at"`
}

// AccountNote represents a private note on an account
type AccountNote struct {
	Username      string    `dynamodbav:"username"`        // Who wrote the note
	TargetActorID string    `dynamodbav:"target_actor_id"` // The actor the note is about
	Note          string    `dynamodbav:"note"`            // The note content
	CreatedAt     time.Time `dynamodbav:"created_at"`
	UpdatedAt     time.Time `dynamodbav:"updated_at"`
}

// StatusPin represents a pinned status on a user's profile
type StatusPin struct {
	Username  string    `dynamodbav:"username"`  // Who pinned the status
	StatusID  string    `dynamodbav:"status_id"` // The status that was pinned
	CreatedAt time.Time `dynamodbav:"created_at"`
}

// ConversationMute represents a muted conversation thread
type ConversationMute struct {
	Username       string    `dynamodbav:"username"`        // Who muted the conversation
	ConversationID string    `dynamodbav:"conversation_id"` // The conversation/thread ID (usually the root status ID)
	CreatedAt      time.Time `dynamodbav:"created_at"`
	ExpiresAt      time.Time `dynamodbav:"expires_at,omitempty"` // Optional expiration
}

// ScheduledStatus represents a status scheduled for future publication
type ScheduledStatus struct {
	ID            string         `dynamodbav:"id"`
	Username      string         `dynamodbav:"username"` // Who scheduled the status
	Status        string         `dynamodbav:"status"`   // The status content
	MediaIDs      []string       `dynamodbav:"media_ids,omitempty"`
	Sensitive     bool           `dynamodbav:"sensitive"`
	SpoilerText   string         `dynamodbav:"spoiler_text,omitempty"`
	Visibility    string         `dynamodbav:"visibility"` // public, unlisted, private, direct
	Language      string         `dynamodbav:"language,omitempty"`
	InReplyToID   string         `dynamodbav:"in_reply_to_id,omitempty"`
	Poll          map[string]any `dynamodbav:"poll,omitempty"` // Poll data if any
	ScheduledAt   time.Time      `dynamodbav:"scheduled_at"`   // When to publish
	CreatedAt     time.Time      `dynamodbav:"created_at"`
	UpdatedAt     time.Time      `dynamodbav:"updated_at"`
	Published     bool           `dynamodbav:"published"` // Whether it has been published
	PublishedAt   *time.Time     `dynamodbav:"published_at,omitempty"`
	ApplicationID string         `dynamodbav:"application_id,omitempty"` // OAuth app that created it
}

// FeaturedTag represents a hashtag featured on a user's profile
type FeaturedTag struct {
	ID            string    `dynamodbav:"id"`
	Username      string    `dynamodbav:"username"`       // Who featured the tag
	Name          string    `dynamodbav:"name"`           // The tag name (without #)
	URL           string    `dynamodbav:"url"`            // URL to the tag
	StatusesCount int       `dynamodbav:"statuses_count"` // Number of statuses with this tag
	LastStatusAt  string    `dynamodbav:"last_status_at"` // Last time the user posted with this tag
	CreatedAt     time.Time `dynamodbav:"created_at"`
}

// Hashtag represents a hashtag with usage statistics
type Hashtag struct {
	Name       string    `dynamodbav:"Name" json:"name"`
	URL        string    `dynamodbav:"URL" json:"url"`
	UsageCount int64     `dynamodbav:"UsageCount" json:"usage_count"`
	FirstSeen  time.Time `dynamodbav:"FirstSeen" json:"first_seen"`
	LastUsed   time.Time `dynamodbav:"LastUsed" json:"last_used"`
}

// StatusSearchOptions configures status search behavior
type StatusSearchOptions struct {
	Limit         int       // Maximum results to return
	Offset        int       // For pagination
	AccountID     string    // Filter by specific account
	FollowingOnly bool      // Only from accounts user follows
	LocalOnly     bool      // Only local statuses
	MediaOnly     bool      // Only statuses with media
	Language      string    // Filter by language
	MinEngagement int       // Minimum likes/boosts
	TimeRange     TimeRange // Time-based filtering
}

// TimeRange represents a time-based filter
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// UserPreferences represents user-specific preferences
type UserPreferences struct {
	Language                  string          `dynamodbav:"language"`
	DefaultPostingVisibility  string          `dynamodbav:"default_posting_visibility"`
	DefaultMediaSensitive     bool            `dynamodbav:"default_media_sensitive"`
	ExpandSpoilers            bool            `dynamodbav:"expand_spoilers"`
	ExpandMedia               string          `dynamodbav:"expand_media"`  // "default", "show_all", or "hide_all"
	AutoplayGifs              bool            `dynamodbav:"autoplay_gifs"` // Whether to autoplay GIFs
	ShowFollowCounts          bool            `dynamodbav:"show_follow_counts"`
	PreferredTimelineOrder    string          `dynamodbav:"preferred_timeline_order"` // newest, oldest, engagement
	SearchSuggestionsEnabled  bool            `dynamodbav:"search_suggestions_enabled"`
	PersonalizedSearchEnabled bool            `dynamodbav:"personalized_search_enabled"`
	ReblogFilters             map[string]bool `dynamodbav:"reblog_filters,omitempty"` // actor_id -> show_reblogs
}

// SearchQueryStats represents statistics about a search query
type SearchQueryStats struct {
	Query      string    `dynamodbav:"query"`
	Count      int       `dynamodbav:"count"`
	UserCount  int       `dynamodbav:"user_count"`  // Unique users who searched
	AvgResults float64   `dynamodbav:"avg_results"` // Average number of results
	LastUsed   time.Time `dynamodbav:"last_used"`
}

// SearchHistoryEntry represents a user's search history entry
type SearchHistoryEntry struct {
	UserID      string    `dynamodbav:"user_id"`
	Query       string    `dynamodbav:"query"`
	ResultCount int       `dynamodbav:"result_count"`
	ClickedIDs  []string  `dynamodbav:"clicked_ids"` // IDs of results user clicked
	SearchedAt  time.Time `dynamodbav:"searched_at"`
}

// TrendingHashtag represents a trending hashtag
type TrendingHashtag struct {
	Name        string    `dynamodbav:"name"`
	URL         string    `dynamodbav:"url"`
	UsageCount  int64     `dynamodbav:"usage_count"`
	UniqueUsers int64     `dynamodbav:"unique_users"`
	LastUsed    time.Time `dynamodbav:"last_used"`
	FirstSeen   time.Time `dynamodbav:"first_seen"`
	UserID      string    `dynamodbav:"user_id"`    // User who used the hashtag
	CreatedAt   time.Time `dynamodbav:"created_at"` // When this usage was recorded
}

// TrendingStatus represents a trending status
type TrendingStatus struct {
	ID          string    `dynamodbav:"id"`
	URL         string    `dynamodbav:"url"`
	AuthorID    string    `dynamodbav:"author_id"`
	Content     string    `dynamodbav:"content"`
	Engagements int64     `dynamodbav:"engagements"`
	PublishedAt time.Time `dynamodbav:"published_at"`
	CreatedAt   time.Time `dynamodbav:"created_at"` // When this trend record was created
	Likes       int       `dynamodbav:"likes"`      // Number of likes
	Boosts      int       `dynamodbav:"boosts"`     // Number of boosts
	Replies     int       `dynamodbav:"replies"`    // Number of replies
}

// TrendingLink represents a trending link
type TrendingLink struct {
	URL         string    `dynamodbav:"url"`
	Title       string    `dynamodbav:"title"`
	Description string    `dynamodbav:"description"`
	Type        string    `dynamodbav:"type"` // link, photo, video
	AuthorName  string    `dynamodbav:"author_name"`
	Image       string    `dynamodbav:"image"`
	ImageURL    string    `dynamodbav:"image_url"` // Additional field for image URL
	ShareCount  int64     `dynamodbav:"share_count"`
	UserID      string    `dynamodbav:"user_id"`    // User who shared the link
	CreatedAt   time.Time `dynamodbav:"created_at"` // When this share was recorded
}

// HashtagStats represents statistics for a hashtag
type HashtagStats struct {
	Name          string                `dynamodbav:"name"`
	UsageCount    int64                 `dynamodbav:"usage_count"`
	UniqueUsers   int64                 `dynamodbav:"unique_users"`
	FirstSeen     time.Time             `dynamodbav:"first_seen"`
	LastUsed      time.Time             `dynamodbav:"last_used"`
	TrendingScore float64               `dynamodbav:"trending_score"`
	TotalUses     int64                 `dynamodbav:"total_uses"`     // Total usage count
	TotalAccounts int64                 `dynamodbav:"total_accounts"` // Total unique accounts
	History       []HashtagHistoryEntry `dynamodbav:"history"`        // Historical data
}

// HashtagHistoryEntry represents a historical data point for hashtag usage
type HashtagHistoryEntry struct {
	Date       time.Time `dynamodbav:"date"`
	UsageCount int64     `dynamodbav:"usage_count"`
	UserCount  int64     `dynamodbav:"user_count"`
}

// HashtagTrend represents a trending hashtag with additional metadata
type HashtagTrend struct {
	*TrendingHashtag
	TrendingScore float64 `dynamodbav:"trending_score"`
	Velocity      float64 `dynamodbav:"velocity"`
}

// StatusTrend represents a trending status
type StatusTrend struct {
	*TrendingStatus
	TrendingScore float64 `dynamodbav:"trending_score"`
	Velocity      float64 `dynamodbav:"velocity"`
}

// LinkTrend represents a trending link
type LinkTrend struct {
	*TrendingLink
	TrendingScore float64 `dynamodbav:"trending_score"`
	Velocity      float64 `dynamodbav:"velocity"`
}

// LinkMetadata represents metadata about a link
type LinkMetadata struct {
	URL         string `dynamodbav:"url"`
	Title       string `dynamodbav:"title"`
	Description string `dynamodbav:"description"`
	Image       string `dynamodbav:"image"`
	Domain      string `dynamodbav:"domain"`
}

// Announcement represents an announcement activity
type Announcement struct {
	ID          string        `dynamodbav:"id"`
	Content     string        `dynamodbav:"content"`             // HTML content
	Text        string        `dynamodbav:"text"`                // Plain text version
	PublishedAt time.Time     `dynamodbav:"published_at"`        // When it was published
	UpdatedAt   time.Time     `dynamodbav:"updated_at"`          // When it was last updated
	AllDay      bool          `dynamodbav:"all_day"`             // Whether it's an all-day announcement
	StartsAt    *time.Time    `dynamodbav:"starts_at,omitempty"` // When the announcement starts (optional)
	EndsAt      *time.Time    `dynamodbav:"ends_at,omitempty"`   // When the announcement ends (optional)
	Reactions   []Reaction    `dynamodbav:"reactions,omitempty"` // Available reactions
	Tags        []string      `dynamodbav:"tags,omitempty"`      // Hashtags in the announcement
	Emojis      []CustomEmoji `dynamodbav:"emojis,omitempty"`    // Custom emojis used
	Mentions    []Mention     `dynamodbav:"mentions,omitempty"`  // Mentioned accounts
	CreatedBy   string        `dynamodbav:"created_by"`          // Admin who created it
}

// AnnouncementDismissal represents a user dismissing an announcement
type AnnouncementDismissal struct {
	Username       string    `dynamodbav:"username"`
	AnnouncementID string    `dynamodbav:"announcement_id"`
	DismissedAt    time.Time `dynamodbav:"dismissed_at"`
}

// AnnouncementReaction represents a user's reaction to an announcement
type AnnouncementReaction struct {
	Username       string    `dynamodbav:"username"`
	AnnouncementID string    `dynamodbav:"announcement_id"`
	EmojiName      string    `dynamodbav:"emoji_name"`
	ReactedAt      time.Time `dynamodbav:"reacted_at"`
}

// Reaction represents an available reaction for announcements
type Reaction struct {
	Name      string `dynamodbav:"name"`                 // Emoji name or custom emoji shortcode
	Count     int    `dynamodbav:"count"`                // Number of users who reacted
	Me        bool   `dynamodbav:"me"`                   // Whether the current user reacted
	URL       string `dynamodbav:"url,omitempty"`        // URL for custom emoji
	StaticURL string `dynamodbav:"static_url,omitempty"` // Static URL for custom emoji
}

// CustomEmoji represents a custom emoji (placeholder for now)
type CustomEmoji struct {
	Shortcode           string    `dynamodbav:"shortcode"`
	URL                 string    `dynamodbav:"url"`
	StaticURL           string    `dynamodbav:"static_url"`
	VisibleInPicker     bool      `dynamodbav:"visible_in_picker"`
	Category            string    `dynamodbav:"category,omitempty"`
	CreatedAt           time.Time `dynamodbav:"created_at"`
	UpdatedAt           time.Time `dynamodbav:"updated_at"`
	Disabled            bool      `dynamodbav:"disabled"`
	Domain              string    `dynamodbav:"domain,omitempty"` // Empty for local emojis
	ImageRemoteURL      string    `dynamodbav:"image_remote_url,omitempty"`
	ImageStorageVersion int       `dynamodbav:"image_storage_version"`
	ImageFileSize       int64     `dynamodbav:"image_file_size"`
	ImageContentType    string    `dynamodbav:"image_content_type"`
	ImageWidth          int       `dynamodbav:"image_width"`
	ImageHeight         int       `dynamodbav:"image_height"`
	ImageUpdatedAt      time.Time `dynamodbav:"image_updated_at"`
}

// Mention represents a mention in an announcement (placeholder)
type Mention struct {
	ID       string `dynamodbav:"id"`
	Username string `dynamodbav:"username"`
	URL      string `dynamodbav:"url"`
	Acct     string `dynamodbav:"acct"`
}

// ReportStatus represents the status of a report
type ReportStatus string

const (
	ReportStatusOpen     ReportStatus = "open"
	ReportStatusResolved ReportStatus = "resolved"
	ReportStatusRejected ReportStatus = "rejected"
)

// Report represents a user report
type Report struct {
	ID                string       `dynamodbav:"id"`
	ReporterID        string       `dynamodbav:"reporter_id"`            // Username of reporter
	TargetAccountID   string       `dynamodbav:"target_account_id"`      // Account being reported
	StatusIDs         []string     `dynamodbav:"status_ids,omitempty"`   // Specific statuses reported
	Comment           string       `dynamodbav:"comment"`                // Reporter's comment
	Category          string       `dynamodbav:"category"`               // spam, violation, other
	RuleIDs           []int        `dynamodbav:"rule_ids,omitempty"`     // Rule violations
	Forwarded         bool         `dynamodbav:"forwarded"`              // Forwarded to remote instance
	Status            ReportStatus `dynamodbav:"status"`                 // Current status
	ActionTaken       string       `dynamodbav:"action_taken,omitempty"` // What action was taken
	ActionTakenAt     *time.Time   `dynamodbav:"action_taken_at,omitempty"`
	ModeratorID       string       `dynamodbav:"moderator_id,omitempty"`        // Who handled the report
	ModerationEventID string       `dynamodbav:"moderation_event_id,omitempty"` // Link to moderation system
	CreatedAt         time.Time    `dynamodbav:"created_at"`
	UpdatedAt         time.Time    `dynamodbav:"updated_at"`
	AssignedTo        string       `dynamodbav:"assigned_to,omitempty"` // Admin/moderator assigned to handle this
}

// ReportStats represents reporting statistics for a user
type ReportStats struct {
	TotalReports    int       `dynamodbav:"total_reports"`
	ResolvedReports int       `dynamodbav:"resolved_reports"`
	FalseReports    int       `dynamodbav:"false_reports"`
	LastReportAt    time.Time `dynamodbav:"last_report_at"`
}

// Marker represents a timeline position marker
type Marker struct {
	LastReadID string    `dynamodbav:"last_read_id"`
	UpdatedAt  time.Time `dynamodbav:"updated_at"`
	Version    int       `dynamodbav:"version"` // For optimistic locking
}

// DomainBlock represents a user-level domain block
type DomainBlock struct {
	Username  string    `dynamodbav:"username"`
	Domain    string    `dynamodbav:"domain"`
	CreatedAt time.Time `dynamodbav:"created_at"`
}

// InstanceDomainBlock represents an instance-level domain block
type InstanceDomainBlock struct {
	ID             string    `dynamodbav:"ID"`
	Domain         string    `dynamodbav:"Domain"`
	Severity       string    `dynamodbav:"Severity"` // "silence" or "suspend"
	RejectMedia    bool      `dynamodbav:"RejectMedia"`
	RejectReports  bool      `dynamodbav:"RejectReports"`
	PrivateComment string    `dynamodbav:"PrivateComment"` // Admin-only notes
	PublicComment  string    `dynamodbav:"PublicComment"`  // Public reason
	Obfuscate      bool      `dynamodbav:"Obfuscate"`      // Whether to obfuscate in public lists
	CreatedBy      string    `dynamodbav:"CreatedBy"`      // Admin username who created
	CreatedByID    string    `dynamodbav:"CreatedByID"`    // Admin actor ID
	CreatedAt      time.Time `dynamodbav:"CreatedAt"`
	UpdatedAt      time.Time `dynamodbav:"UpdatedAt"`
}

// DomainAllow represents a domain in the allowlist
type DomainAllow struct {
	ID        string    `dynamodbav:"ID"`
	Domain    string    `dynamodbav:"Domain"`
	CreatedBy string    `dynamodbav:"CreatedBy"`
	CreatedAt time.Time `dynamodbav:"CreatedAt"`
}

// InstanceInfo represents tracked information about a federated instance
type InstanceInfo struct {
	Domain        string    `dynamodbav:"Domain"`
	Software      string    `dynamodbav:"Software"`      // mastodon, pleroma, etc.
	Version       string    `dynamodbav:"Version"`       // Software version
	FirstSeen     time.Time `dynamodbav:"FirstSeen"`     // When we first saw this instance
	LastSeen      time.Time `dynamodbav:"LastSeen"`      // Last activity from this instance
	PublicKey     string    `dynamodbav:"PublicKey"`     // Instance actor public key
	SharedInbox   string    `dynamodbav:"SharedInbox"`   // Shared inbox endpoint
	TrustScore    float64   `dynamodbav:"TrustScore"`    // Calculated trust score
	ActiveUsers   int       `dynamodbav:"ActiveUsers"`   // Number of active users
	TotalMessages int64     `dynamodbav:"TotalMessages"` // Total messages received
}

// FederationStats represents aggregated federation statistics
type FederationStats struct {
	ActiveInstances int   `dynamodbav:"ActiveInstances"`
	TotalMessages   int64 `dynamodbav:"TotalMessages"`
	TotalUsers      int   `dynamodbav:"TotalUsers"`
}

// EmailDomainBlock represents a blocked email domain for registration
type EmailDomainBlock struct {
	ID        string    `dynamodbav:"ID"`
	Domain    string    `dynamodbav:"Domain"`
	CreatedBy string    `dynamodbav:"CreatedBy"`
	CreatedAt time.Time `dynamodbav:"CreatedAt"`
}

// ModerationEventFilter represents filters for querying moderation events
type ModerationEventFilter struct {
	EventType   *EventType `json:"event_type,omitempty"`
	Category    *Category  `json:"category,omitempty"`
	MinSeverity *Severity  `json:"min_severity,omitempty"`
	ActorID     string     `json:"actor_id,omitempty"`
	ObjectID    string     `json:"object_id,omitempty"`
	StartTime   *time.Time `json:"start_time,omitempty"`
	EndTime     *time.Time `json:"end_time,omitempty"`
}

// ReviewerStats represents statistics about a moderation reviewer
type ReviewerStats struct {
	ReviewerID        string         `dynamodbav:"reviewer_id"`
	TotalReviews      int            `dynamodbav:"total_reviews"`
	AccurateReviews   int            `dynamodbav:"accurate_reviews"`
	AccuracyRate      float64        `dynamodbav:"accuracy_rate"`
	LastReviewAt      time.Time      `dynamodbav:"last_review_at"`
	TrustScore        float64        `dynamodbav:"trust_score"`
	JoinedAt          time.Time      `dynamodbav:"joined_at"`
	ReviewsByCategory map[string]int `dynamodbav:"reviews_by_category"`
}

// CommunityNote represents a fact-checking note on any ActivityPub object
type CommunityNote struct {
	ID               string    `dynamodbav:"id"`
	ObjectID         string    `dynamodbav:"object_id"`
	ObjectType       string    `dynamodbav:"object_type"`
	AuthorID         string    `dynamodbav:"author_id"`
	Content          string    `dynamodbav:"content"`
	Language         string    `dynamodbav:"language"`
	Sources          []string  `dynamodbav:"sources"`
	HelpfulVotes     int       `dynamodbav:"helpful_votes"`
	NotHelpfulVotes  int       `dynamodbav:"not_helpful_votes"`
	Score            float64   `dynamodbav:"score"`
	VisibilityStatus string    `dynamodbav:"visibility_status"`
	CreatedAt        time.Time `dynamodbav:"created_at"`
	UpdatedAt        time.Time `dynamodbav:"updated_at"`
}

// CommunityNoteVote represents a vote on a community note
type CommunityNoteVote struct {
	NoteID    string    `dynamodbav:"note_id"`
	VoterID   string    `dynamodbav:"voter_id"`
	VoteType  string    `dynamodbav:"vote_type"` // helpful, not_helpful, neutral
	Helpful   bool      `dynamodbav:"helpful"`   // For simplified access
	Weight    float64   `dynamodbav:"weight"`
	CreatedAt time.Time `dynamodbav:"created_at"`
}

// Session represents a user session
type Session struct {
	SessionID    string    `dynamodbav:"session_id"`
	Username     string    `dynamodbav:"username"`
	RefreshToken string    `dynamodbav:"refresh_token"`
	DeviceID     string    `dynamodbav:"device_id"`
	DeviceName   string    `dynamodbav:"device_name"`
	UserAgent    string    `dynamodbav:"user_agent"`
	IPAddress    string    `dynamodbav:"ip_address"`
	AuthMethod   string    `dynamodbav:"auth_method"` // password, passkey, wallet, oauth
	CreatedAt    time.Time `dynamodbav:"created_at"`
	LastActivity time.Time `dynamodbav:"last_activity"`
	ExpiresAt    time.Time `dynamodbav:"expires_at"`

	// Token rotation tracking
	PreviousRefreshToken string    `dynamodbav:"previous_refresh_token,omitempty"`
	TokenRotatedAt       time.Time `dynamodbav:"token_rotated_at,omitempty"`
}

// Device represents a user's device/session
type Device struct {
	DeviceID      string    `dynamodbav:"device_id"`
	Username      string    `dynamodbav:"username"`
	DeviceName    string    `dynamodbav:"device_name"`
	DeviceType    string    `dynamodbav:"device_type"` // web, mobile, desktop
	LastIPAddress string    `dynamodbav:"last_ip_address"`
	LastUserAgent string    `dynamodbav:"last_user_agent"`
	CreatedAt     time.Time `dynamodbav:"created_at"`
	LastSeenAt    time.Time `dynamodbav:"last_seen_at"`
	TrustLevel    string    `dynamodbav:"trust_level"` // trusted, untrusted, suspicious
}

// WebAuthnCredential represents a stored WebAuthn credential
type WebAuthnCredential struct {
	ID              string    `dynamodbav:"id"`
	UserID          string    `dynamodbav:"user_id"`
	PublicKey       []byte    `dynamodbav:"public_key"`
	AttestationType string    `dynamodbav:"attestation_type"`
	AAGUID          []byte    `dynamodbav:"aaguid"`
	SignCount       uint32    `dynamodbav:"sign_count"`
	CloneWarning    bool      `dynamodbav:"clone_warning"`
	BackupEligible  bool      `dynamodbav:"backup_eligible"`
	BackupState     bool      `dynamodbav:"backup_state"`
	CreatedAt       time.Time `dynamodbav:"created_at"`
	LastUsedAt      time.Time `dynamodbav:"last_used_at"`
	Name            string    `dynamodbav:"name"` // User-friendly name
}

// WebAuthnChallenge represents a temporary challenge for registration/login
type WebAuthnChallenge struct {
	Challenge   string    `dynamodbav:"challenge"`
	UserID      string    `dynamodbav:"user_id"`
	SessionData []byte    `dynamodbav:"session_data"` // Serialized session data
	ExpiresAt   time.Time `dynamodbav:"expires_at"`
	Type        string    `dynamodbav:"type"` // "registration" or "authentication"
}

// WalletChallenge represents a challenge for wallet authentication
type WalletChallenge struct {
	ID        string    `dynamodbav:"id"`
	Username  string    `dynamodbav:"username,omitempty"`
	Address   string    `dynamodbav:"address"`
	ChainID   int       `dynamodbav:"chain_id"`
	Nonce     string    `dynamodbav:"nonce"`
	Message   string    `dynamodbav:"message"`
	IssuedAt  time.Time `dynamodbav:"issued_at"`
	ExpiresAt time.Time `dynamodbav:"expires_at"`
}

// WalletCredential represents a linked wallet
type WalletCredential struct {
	Username string    `dynamodbav:"username"`
	Address  string    `dynamodbav:"address"`
	ChainID  int       `dynamodbav:"chain_id"`
	Type     string    `dynamodbav:"type"` // ethereum, solana, etc.
	ENS      string    `dynamodbav:"ens,omitempty"`
	LinkedAt time.Time `dynamodbav:"linked_at"`
	LastUsed time.Time `dynamodbav:"last_used"`
}

// TrusteeConfig represents a trusted contact for social recovery
type TrusteeConfig struct {
	Username  string    `dynamodbav:"username"` // Who owns this trustee relationship
	ActorID   string    `dynamodbav:"actor_id"` // @friend@mastodon.social
	AddedAt   time.Time `dynamodbav:"added_at"`
	Confirmed bool      `dynamodbav:"confirmed"`
}

// SocialRecoveryRequest represents an active recovery request
type SocialRecoveryRequest struct {
	ID            string          `dynamodbav:"id"`
	Username      string          `dynamodbav:"username"`
	InitiatedAt   time.Time       `dynamodbav:"initiated_at"`
	ExpiresAt     time.Time       `dynamodbav:"expires_at"`
	RequiredVotes int             `dynamodbav:"required_votes"`
	ReceivedVotes map[string]bool `dynamodbav:"received_votes"` // trustee_id -> voted
	RecoveryToken string          `dynamodbav:"recovery_token"`
	Status        string          `dynamodbav:"status"` // pending, approved, expired, cancelled
}

// RecoveryCodeItem represents a single recovery code
type RecoveryCodeItem struct {
	Username  string     `dynamodbav:"username"`
	CodeHash  string     `dynamodbav:"code_hash"` // bcrypt hash of the code
	CreatedAt time.Time  `dynamodbav:"created_at"`
	UsedAt    *time.Time `dynamodbav:"used_at,omitempty"`
	Position  int        `dynamodbav:"position"` // Position in the list (0-7 typically)
}

// WeeklyActivity represents activity metrics for a specific week
type WeeklyActivity struct {
	Week          int64 `dynamodbav:"week"`          // Unix timestamp of week start
	Statuses      int64 `dynamodbav:"statuses"`      // Number of statuses created
	Logins        int64 `dynamodbav:"logins"`        // Number of unique logins
	Registrations int64 `dynamodbav:"registrations"` // Number of new registrations
}

// Reputation represents a user's reputation score and evidence
type Reputation struct {
	// Identity
	ActorID     string `json:"@id" dynamodbav:"ActorID"`
	InstanceURL string `json:"instance" dynamodbav:"InstanceURL"`

	// Scores (0-1000 scale)
	TrustScore      int `json:"trustScore" dynamodbav:"TrustScore"`
	ActivityScore   int `json:"activityScore" dynamodbav:"ActivityScore"`
	ModerationScore int `json:"moderationScore" dynamodbav:"ModerationScore"`
	CommunityScore  int `json:"communityScore" dynamodbav:"CommunityScore"`
	TotalScore      int `json:"totalScore" dynamodbav:"TotalScore"`

	// Metadata
	CalculatedAt time.Time `json:"calculatedAt" dynamodbav:"CalculatedAt"`
	Version      string    `json:"version" dynamodbav:"Version"`

	// Evidence
	TotalPosts     int `json:"totalPosts" dynamodbav:"TotalPosts"`
	TotalFollowers int `json:"totalFollowers" dynamodbav:"TotalFollowers"`
	AccountAge     int `json:"accountAgeDays" dynamodbav:"AccountAge"`
	VouchCount     int `json:"vouchCount" dynamodbav:"VouchCount"`

	// Trust graph metrics
	TrustingActors    int     `json:"trustingActors" dynamodbav:"TrustingActors"`
	AverageTrustScore float64 `json:"averageTrustScore" dynamodbav:"AverageTrustScore"`

	// Moderation metrics
	ReportsReceived int `json:"reportsReceived" dynamodbav:"ReportsReceived"`
	ReportsUpheld   int `json:"reportsUpheld" dynamodbav:"ReportsUpheld"`
	FalseReports    int `json:"falseReports" dynamodbav:"FalseReports"`

	// Cryptographic proof
	Signature string `json:"signature,omitempty" dynamodbav:"Signature,omitempty"`
	PublicKey string `json:"publicKey,omitempty" dynamodbav:"PublicKey,omitempty"`
}

// Vouch represents one user vouching for another
type Vouch struct {
	ID          string    `json:"@id" dynamodbav:"ID"`
	From        string    `json:"from" dynamodbav:"From"` // Actor who vouched
	To          string    `json:"to" dynamodbav:"To"`     // Actor being vouched for
	InstanceURL string    `json:"instance" dynamodbav:"InstanceURL"`
	CreatedAt   time.Time `json:"createdAt" dynamodbav:"CreatedAt"`
	ExpiresAt   time.Time `json:"expiresAt" dynamodbav:"ExpiresAt"`
	Confidence  float64   `json:"confidence" dynamodbav:"Confidence"` // 0.0-1.0
	Context     string    `json:"context" dynamodbav:"Context"`       // Why vouching

	// Voucher reputation at time of vouch
	VoucherReputation int `json:"voucherReputation" dynamodbav:"VoucherReputation"`

	// Status
	Active    bool       `json:"active" dynamodbav:"Active"`
	Revoked   bool       `json:"revoked" dynamodbav:"Revoked"`
	RevokedAt *time.Time `json:"revokedAt,omitempty" dynamodbav:"RevokedAt,omitempty"`

	// Cryptographic proof
	Signature string `json:"signature" dynamodbav:"Signature"`
}

// DNSCacheEntry represents a cached DNS lookup result
type DNSCacheEntry struct {
	Hostname   string    `json:"hostname" dynamodbav:"hostname"`
	IPs        []string  `json:"ips" dynamodbav:"ips"`
	ResolvedAt time.Time `json:"resolved_at" dynamodbav:"resolved_at"`
	TTL        int       `json:"ttl" dynamodbav:"ttl"` // seconds
}

// QuoteRelationship represents a quote relationship between notes
type QuoteRelationship struct {
	ID             string     `json:"id" dynamodbav:"id"`
	QuoterNoteID   string     `json:"quoter_note_id" dynamodbav:"quoter_note_id"`
	TargetNoteID   string     `json:"target_note_id" dynamodbav:"target_note_id"`
	QuoterID       string     `json:"quoter_id" dynamodbav:"quoter_id"`
	TargetAuthorID string     `json:"target_author_id" dynamodbav:"target_author_id,omitempty"`
	Timestamp      time.Time  `json:"timestamp" dynamodbav:"timestamp"`
	Withdrawn      bool       `json:"withdrawn" dynamodbav:"withdrawn"`
	WithdrawnAt    *time.Time `json:"withdrawn_at,omitempty" dynamodbav:"withdrawn_at,omitempty"`
}

// FederationActivity represents a single federation activity for cost tracking
type FederationActivity struct {
	ID           string    `json:"id" dynamodbav:"id"`
	Domain       string    `json:"domain" dynamodbav:"domain"`
	Type         string    `json:"type" dynamodbav:"type"`                   // ingress/egress
	ActivityType string    `json:"activity_type" dynamodbav:"activity_type"` // Create/Update/Delete/Follow/etc
	ByteSize     int64     `json:"byte_size" dynamodbav:"byte_size"`
	Success      bool      `json:"success" dynamodbav:"success"`
	ResponseTime int64     `json:"response_time" dynamodbav:"response_time"` // milliseconds
	ErrorMessage string    `json:"error_message,omitempty" dynamodbav:"error_message,omitempty"`
	Timestamp    time.Time `json:"timestamp" dynamodbav:"timestamp"`
}

// FederationCost represents aggregated cost data for a domain
type FederationCost struct {
	Domain           string    `json:"domain" dynamodbav:"domain"`
	Period           string    `json:"period" dynamodbav:"period"` // daily/monthly
	IngressBytes     int64     `json:"ingress_bytes" dynamodbav:"ingress_bytes"`
	EgressBytes      int64     `json:"egress_bytes" dynamodbav:"egress_bytes"`
	RequestCount     int64     `json:"request_count" dynamodbav:"request_count"`
	ErrorCount       int64     `json:"error_count" dynamodbav:"error_count"`
	ErrorRate        float64   `json:"error_rate" dynamodbav:"error_rate"`
	AvgResponseTime  float64   `json:"avg_response_time" dynamodbav:"avg_response_time"`
	EstimatedCostUSD float64   `json:"estimated_cost_usd" dynamodbav:"estimated_cost_usd"`
	LastUpdated      time.Time `json:"last_updated" dynamodbav:"last_updated"`
}

// InstanceHealthReport represents health metrics for a federated instance
type InstanceHealthReport struct {
	Domain          string    `json:"domain" dynamodbav:"domain"`
	Status          string    `json:"status" dynamodbav:"status"` // healthy/warning/critical
	ResponseTime    float64   `json:"response_time" dynamodbav:"response_time"`
	ErrorRate       float64   `json:"error_rate" dynamodbav:"error_rate"`
	FederationDelay float64   `json:"federation_delay" dynamodbav:"federation_delay"`
	QueueDepth      int       `json:"queue_depth" dynamodbav:"queue_depth"`
	Issues          []string  `json:"issues" dynamodbav:"issues"`
	Recommendations []string  `json:"recommendations" dynamodbav:"recommendations"`
	LastChecked     time.Time `json:"last_checked" dynamodbav:"last_checked"`
}

// CostProjection represents projected costs for federation
type CostProjection struct {
	Period          string       `json:"period" dynamodbav:"period"`
	CurrentCost     float64      `json:"current_cost" dynamodbav:"current_cost"`
	ProjectedCost   float64      `json:"projected_cost" dynamodbav:"projected_cost"`
	Variance        float64      `json:"variance" dynamodbav:"variance"`
	TopDrivers      []CostDriver `json:"top_drivers" dynamodbav:"top_drivers"`
	Recommendations []string     `json:"recommendations" dynamodbav:"recommendations"`
}

// CostDriver represents a major cost contributor
type CostDriver struct {
	Type           string  `json:"type" dynamodbav:"type"`
	Domain         string  `json:"domain,omitempty" dynamodbav:"domain,omitempty"`
	Cost           float64 `json:"cost" dynamodbav:"cost"`
	PercentOfTotal float64 `json:"percent_of_total" dynamodbav:"percent_of_total"`
	Trend          string  `json:"trend" dynamodbav:"trend"` // increasing/stable/decreasing
}

// FederationNode represents an instance in the federation graph
type FederationNode struct {
	Domain            string         `json:"domain" dynamodbav:"domain"`
	DisplayName       string         `json:"display_name" dynamodbav:"display_name"`
	Description       string         `json:"description,omitempty" dynamodbav:"description,omitempty"`
	Software          string         `json:"software" dynamodbav:"software"`
	Version           string         `json:"version" dynamodbav:"version"`
	UserCount         int64          `json:"user_count" dynamodbav:"user_count"`
	StatusCount       int64          `json:"status_count" dynamodbav:"status_count"`
	ActiveUsers       int64          `json:"active_users" dynamodbav:"active_users"`
	FirstSeen         time.Time      `json:"first_seen" dynamodbav:"first_seen"`
	LastSeen          time.Time      `json:"last_seen" dynamodbav:"last_seen"`
	Health            string         `json:"health" dynamodbav:"health"` // healthy/warning/critical/unknown
	ErrorRate         float64        `json:"error_rate" dynamodbav:"error_rate"`
	ResponseTime      float64        `json:"response_time" dynamodbav:"response_time"`
	ConnectionType    string         `json:"connection_type" dynamodbav:"connection_type"` // direct/relay/blocked
	TotalConnections  int64          `json:"total_connections,omitempty" dynamodbav:"total_connections,omitempty"`
	ActiveConnections int64          `json:"active_connections,omitempty" dynamodbav:"active_connections,omitempty"`
	ActivityVolume    int64          `json:"activity_volume,omitempty" dynamodbav:"activity_volume,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty" dynamodbav:"metadata,omitempty"`
}

// FederationEdge represents a connection between two instances
type FederationEdge struct {
	SourceDomain   string    `json:"source_domain" dynamodbav:"source_domain"`
	TargetDomain   string    `json:"target_domain" dynamodbav:"target_domain"`
	ConnectionType string    `json:"connection_type" dynamodbav:"connection_type"` // follows/mentions/boosts/replies
	VolumeIn       int64     `json:"volume_in" dynamodbav:"volume_in"`
	VolumeOut      int64     `json:"volume_out" dynamodbav:"volume_out"`
	Strength       float64   `json:"strength" dynamodbav:"strength"` // 0.0-1.0 based on activity volume
	LastActivity   time.Time `json:"last_activity" dynamodbav:"last_activity"`
	SharedUsers    int64     `json:"shared_users" dynamodbav:"shared_users"`
	ErrorCount     int64     `json:"error_count" dynamodbav:"error_count"`
	SuccessRate    float64   `json:"success_rate" dynamodbav:"success_rate"`
}

// InstanceMetadata contains detailed metadata about a federated instance
type InstanceMetadata struct {
	Domain          string    `json:"domain" dynamodbav:"domain"`
	DisplayName     string    `json:"display_name,omitempty" dynamodbav:"display_name,omitempty"`
	Description     string    `json:"description,omitempty" dynamodbav:"description,omitempty"`
	Software        string    `json:"software,omitempty" dynamodbav:"software,omitempty"`
	Version         string    `json:"version,omitempty" dynamodbav:"version,omitempty"`
	UserCount       int64     `json:"user_count,omitempty" dynamodbav:"user_count,omitempty"`
	StatusCount     int64     `json:"status_count,omitempty" dynamodbav:"status_count,omitempty"`
	NodeInfo        string    `json:"nodeinfo" dynamodbav:"nodeinfo"`           // JSON string of nodeinfo response
	InstanceInfo    string    `json:"instance_info" dynamodbav:"instance_info"` // JSON string of instance API response
	AdminContact    string    `json:"admin_contact,omitempty" dynamodbav:"admin_contact,omitempty"`
	Rules           []string  `json:"rules,omitempty" dynamodbav:"rules,omitempty"`
	Languages       []string  `json:"languages,omitempty" dynamodbav:"languages,omitempty"`
	Categories      []string  `json:"categories,omitempty" dynamodbav:"categories,omitempty"`
	FederationNotes string    `json:"federation_notes,omitempty" dynamodbav:"federation_notes,omitempty"`
	LastUpdated     time.Time `json:"last_updated" dynamodbav:"last_updated"`
}

// InstanceCluster represents a group of closely connected instances
type InstanceCluster struct {
	ClusterID   string    `json:"cluster_id" dynamodbav:"cluster_id"`
	Name        string    `json:"name" dynamodbav:"name"`
	Instances   []string  `json:"instances" dynamodbav:"instances"`
	CenterNode  string    `json:"center_node" dynamodbav:"center_node"` // Most connected instance
	Cohesion    float64   `json:"cohesion" dynamodbav:"cohesion"`       // How tightly connected (0.0-1.0)
	Size        int       `json:"size" dynamodbav:"size"`
	Description string    `json:"description,omitempty" dynamodbav:"description,omitempty"`
	UpdatedAt   time.Time `json:"updated_at" dynamodbav:"updated_at"`
}

// InstanceConnection represents a specific connection type between instances
type InstanceConnection struct {
	Domain         string    `json:"domain" dynamodbav:"domain"`
	TargetDomain   string    `json:"target_domain" dynamodbav:"target_domain"`
	Direction      string    `json:"direction" dynamodbav:"direction"` // inbound/outbound
	ConnectionType string    `json:"connection_type" dynamodbav:"connection_type"`
	VolumeIn       int64     `json:"volume_in" dynamodbav:"volume_in"`
	VolumeOut      int64     `json:"volume_out" dynamodbav:"volume_out"`
	LastActivity   time.Time `json:"last_activity" dynamodbav:"last_activity"`
	Success        bool      `json:"success" dynamodbav:"success"`
	ResponseTimeMs float64   `json:"response_time_ms" dynamodbav:"response_time_ms"`
	Health         string    `json:"health" dynamodbav:"health"`
}

// FederationTimeSeries represents time-series federation metrics
type FederationTimeSeries struct {
	Domain         string    `json:"domain" dynamodbav:"domain"`
	Timestamp      time.Time `json:"timestamp" dynamodbav:"timestamp"`
	Period         string    `json:"period" dynamodbav:"period"` // hourly/daily/weekly
	InboundVolume  int64     `json:"inbound_volume" dynamodbav:"inbound_volume"`
	OutboundVolume int64     `json:"outbound_volume" dynamodbav:"outbound_volume"`
	ErrorRate      float64   `json:"error_rate" dynamodbav:"error_rate"`
	ResponseTime   float64   `json:"response_time" dynamodbav:"response_time"`
	ActivePeers    int       `json:"active_peers" dynamodbav:"active_peers"`
	TTL            int64     `json:"ttl,omitempty" dynamodbav:"ttl,omitempty"` // For automatic cleanup
}

// StreamingPreferences represents user preferences for media streaming
type StreamingPreferences struct {
	Username                string    `json:"username" dynamodbav:"username"`
	DefaultQuality          string    `json:"default_quality" dynamodbav:"default_quality"` // auto/4k/1080p/720p/480p
	AutoQuality             bool      `json:"auto_quality" dynamodbav:"auto_quality"`
	PreloadNext             bool      `json:"preload_next" dynamodbav:"preload_next"`
	DataSaverMode           bool      `json:"data_saver_mode" dynamodbav:"data_saver_mode"`
	PreferredCodec          string    `json:"preferred_codec" dynamodbav:"preferred_codec"` // h264/h265/vp9/av1
	MaxBandwidthMbps        int64     `json:"max_bandwidth_mbps" dynamodbav:"max_bandwidth_mbps"`
	BufferSizeSeconds       int       `json:"buffer_size_seconds" dynamodbav:"buffer_size_seconds"`
	HDREnabled              *bool     `json:"hdr_enabled,omitempty" dynamodbav:"hdr_enabled,omitempty"`
	ColorSpace              string    `json:"color_space,omitempty" dynamodbav:"color_space,omitempty"` // bt709/bt2020/p3
	SubtitleLanguage        string    `json:"subtitle_language,omitempty" dynamodbav:"subtitle_language,omitempty"`
	SubtitleEnabled         *bool     `json:"subtitle_enabled,omitempty" dynamodbav:"subtitle_enabled,omitempty"`
	AudioDescriptionEnabled *bool     `json:"audio_description_enabled,omitempty" dynamodbav:"audio_description_enabled,omitempty"`
	ClosedCaptionsEnabled   *bool     `json:"closed_captions_enabled,omitempty" dynamodbav:"closed_captions_enabled,omitempty"`
	SchemaVersion           int       `json:"schema_version,omitempty" dynamodbav:"schema_version,omitempty"`
	DeviceID                string    `json:"device_id,omitempty" dynamodbav:"device_id,omitempty"`
	Version                 int       `json:"version" dynamodbav:"version"`
	UpdatedAt               time.Time `json:"updated_at" dynamodbav:"updated_at"`
}

// ConflictResolutionStrategy defines how to resolve preference conflicts
type ConflictResolutionStrategy string

const (
	ConflictResolutionLatest          ConflictResolutionStrategy = "latest"
	ConflictResolutionHighestQuality  ConflictResolutionStrategy = "highest_quality"
	ConflictResolutionLowestBandwidth ConflictResolutionStrategy = "lowest_bandwidth"
)

// ErrVersionConflict indicates a version conflict during update
var ErrVersionConflict = fmt.Errorf("version conflict")

// ModerationPattern represents a content moderation pattern
type ModerationPattern struct {
	ID                 string    `json:"id" dynamodbav:"id"`
	Name               string    `json:"name" dynamodbav:"name"`
	Description        string    `json:"description" dynamodbav:"description"`
	Type               string    `json:"type" dynamodbav:"type"` // keyword/regex/phrase/domain/ip/hash
	Content            string    `json:"content" dynamodbav:"content"`
	Severity           string    `json:"severity" dynamodbav:"severity"` // low/medium/high/critical
	Action             string    `json:"action" dynamodbav:"action"`     // flag/hide/block/escalate
	Active             bool      `json:"active" dynamodbav:"active"`
	MatchCount         int64     `json:"match_count" dynamodbav:"match_count"`
	FalsePositiveCount int64     `json:"false_positive_count" dynamodbav:"false_positive_count"`
	Effectiveness      float64   `json:"effectiveness" dynamodbav:"effectiveness"`
	LastMatch          time.Time `json:"last_match" dynamodbav:"last_match"`
	CreatedAt          time.Time `json:"created_at" dynamodbav:"created_at"`
	CreatedBy          string    `json:"created_by" dynamodbav:"created_by"`
	UpdatedAt          time.Time `json:"updated_at" dynamodbav:"updated_at"`
	Tags               []string  `json:"tags,omitempty" dynamodbav:"tags,omitempty"`
}

// ModerationFilter represents filters for querying moderation decisions
type ModerationFilter struct {
	Action      string    `json:"action,omitempty"`
	MinScore    float64   `json:"min_score,omitempty"`
	MaxScore    float64   `json:"max_score,omitempty"`
	StartTime   time.Time `json:"start_time,omitempty"`
	EndTime     time.Time `json:"end_time,omitempty"`
	ContentType string    `json:"content_type,omitempty"`
	Limit       int       `json:"limit,omitempty"`
}

// PatternFeedback represents feedback on pattern effectiveness
type PatternFeedback struct {
	WasMatch         bool `json:"was_match" dynamodbav:"was_match"`
	WasFalsePositive bool `json:"was_false_positive" dynamodbav:"was_false_positive"`
}

// RelayInfo represents information about a federation relay
type RelayInfo struct {
	URL        string    `json:"url" dynamodbav:"url"`
	InboxURL   string    `json:"inbox_url" dynamodbav:"inbox_url"`
	Active     bool      `json:"active" dynamodbav:"active"`
	CreatedAt  time.Time `json:"created_at" dynamodbav:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at" dynamodbav:"last_seen_at"`
	Domain     string    `json:"domain,omitempty" dynamodbav:"domain,omitempty"`
	Status     string    `json:"status,omitempty" dynamodbav:"status,omitempty"` // pending/active/rejected/error
	ErrorCount int       `json:"error_count,omitempty" dynamodbav:"error_count,omitempty"`
	TTL        int64     `json:"ttl,omitempty" dynamodbav:"ttl,omitempty"` // For automatic cleanup
}

// Enhanced types for GraphQL operations

// SearchResults combines all search result types
type SearchResults struct {
	Accounts []*activitypub.Actor   `json:"accounts"`
	Statuses []*StatusSearchResult  `json:"statuses"`
	Hashtags []*HashtagSearchResult `json:"hashtags"`
}

// HashtagSearchResult represents a hashtag search result
type HashtagSearchResult struct {
	Name      string             `json:"name"`
	URL       string             `json:"url"`
	History   []*TrendingHashtag `json:"history"`
	Following *bool              `json:"following,omitempty"`
}

// Activity represents an activity for hashtag tracking
type Activity struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Actor     string    `json:"actor"`
	Object    string    `json:"object"`
	Published time.Time `json:"published"`
	Content   string    `json:"content,omitempty"`
}

// QuotePermissions represents quote permissions for a status
type QuotePermissions struct {
	AllowPublic    bool     `json:"allow_public"`
	AllowFollowers bool     `json:"allow_followers"`
	AllowMentioned bool     `json:"allow_mentioned"`
	BlockList      []string `json:"block_list"`
}

// ThreadContext represents the context of a status thread
type ThreadContext struct {
	Ancestors   []*StatusSearchResult `json:"ancestors"`
	Descendants []*StatusSearchResult `json:"descendants"`
}

// SeveredRelationship represents a severed federation relationship
type SeveredRelationship struct {
	Domain       string    `json:"domain"`
	SeveredAt    time.Time `json:"severed_at"`
	Acknowledged bool      `json:"acknowledged"`
	Reason       string    `json:"reason"`
	Type         string    `json:"type"` // domain_block, suspension, etc.
}
