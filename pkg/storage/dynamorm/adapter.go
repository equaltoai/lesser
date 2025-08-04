package dynamorm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/media/streaming"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// ActorRepositoryDeps interface defines dependencies that an actor repository might need
type ActorRepositoryDeps interface {
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	GetPreference(ctx context.Context, username, key string) (any, error)
	SetPreference(ctx context.Context, username, key string, value any) error
}

// SearchRepositoryDeps interface defines dependencies that a search repository might need
type SearchRepositoryDeps interface {
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
}

// ActorRepository interface defines the methods we need from the actor repository
type ActorRepository interface {
	GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error)
	CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error
	UpdateActor(ctx context.Context, actor *activitypub.Actor) error
	DeleteActor(ctx context.Context, username string) error
	GetActorPrivateKey(ctx context.Context, username string) (string, error)
	GetActorByNumericID(ctx context.Context, numericID string) (*activitypub.Actor, error)
	GetActorWithMetadata(ctx context.Context, username string) (*activitypub.Actor, *storage.ActorMetadata, error)
	UpdateActorLastStatusTime(ctx context.Context, username string) error
	SetActorFields(ctx context.Context, username string, fields []storage.ActorField) error
	GetSearchSuggestions(ctx context.Context, prefix string) ([]storage.SearchSuggestion, error)

	// Search operations
	SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error)

	// Remote actor cache operations
	GetCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error)

	// Account suggestions
	GetAccountSuggestions(ctx context.Context, userID string, limit int) ([]*activitypub.Actor, error)
	RemoveAccountSuggestion(ctx context.Context, userID, targetID string) error
}

// ObjectRepository interface defines the methods we need from the object repository
type ObjectRepository interface {
	GetObject(ctx context.Context, id string) (any, error)
	CreateObject(ctx context.Context, object any) error
	UpdateObject(ctx context.Context, object any) error
	DeleteObject(ctx context.Context, objectID string) error
	GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]any, string, error)
	CountObjectReplies(ctx context.Context, objectID string) (int, error)
	TombstoneObject(ctx context.Context, objectID string, deletedBy string) error


	// Additional status methods
	GetStatus(ctx context.Context, statusID string) (any, error)
	GetUserStatusCount(ctx context.Context, userID string) (int, error)
	GetStatusReplyCount(ctx context.Context, statusID string) (int, error)

	// Collection methods
	AddToCollection(ctx context.Context, collection string, item *storage.CollectionItem) error
	RemoveFromCollection(ctx context.Context, collection string, itemID string) error
	GetCollectionItems(ctx context.Context, collection string, limit int, cursor string) ([]*storage.CollectionItem, string, error)
	IsInCollection(ctx context.Context, collection string, itemID string) (bool, error)
	// Update history methods
	CreateUpdateHistory(ctx context.Context, history *storage.UpdateHistory) error
	GetUpdateHistory(ctx context.Context, objectID string, limit int) ([]*storage.UpdateHistory, error)
	CountCollectionItems(ctx context.Context, collection string) (int, error)

	// Reply operations
	GetReplies(ctx context.Context, objectID string, limit int, cursor string) ([]any, string, error)
	CountReplies(ctx context.Context, objectID string) (int, error)
	IncrementReplyCount(ctx context.Context, objectID string) error
	GetReplyCount(ctx context.Context, statusID string) (int64, error)

	// Thread synchronization operations for GraphQL
	SyncThreadFromRemote(ctx context.Context, statusID string) (*storage.StatusSearchResult, error)
	SyncMissingRepliesFromRemote(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error)
	GetThreadContext(ctx context.Context, statusID string) (*storage.ThreadContext, error)
	MarkThreadAsSynced(ctx context.Context, statusID string) error
	GetMissingReplies(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error)

	// Quote operations
	CreateQuoteRelationship(ctx context.Context, quote *storage.QuoteRelationship) error
	GetQuotesForNote(ctx context.Context, noteID string, limit int, cursor string) ([]*storage.QuoteRelationship, string, error)
	IsQuoted(ctx context.Context, actorID, noteID string) (bool, error)
	WithdrawQuote(ctx context.Context, quoteNoteID string) error
	CountQuotes(ctx context.Context, noteID string) (int, error)

	// Enhanced quote operations for GraphQL
	WithdrawStatusFromQuotes(ctx context.Context, statusID string) error
	UpdateQuotePermissions(ctx context.Context, statusID string, permissions *storage.QuotePermissions) error
	IsQuoteAllowed(ctx context.Context, statusID, quoterID string) (bool, error)
	GetQuoteType(ctx context.Context, statusID string) (string, error)
	IsWithdrawnFromQuotes(ctx context.Context, statusID string) (bool, error)
	GetQuotesOfStatus(ctx context.Context, statusID string, limit int) ([]*storage.StatusSearchResult, error)
}

// ActivityRepository interface defines the methods we need from the activity repository
type ActivityRepository interface {
	CreateActivity(ctx context.Context, activity *activitypub.Activity) error
	GetActivity(ctx context.Context, id string) (*activitypub.Activity, error)
	GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error)
	GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error)
	GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error)
}

// TrustRepository interface defines the methods we need from the trust repository
type TrustRepository interface {
	CreateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error
	GetTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error)
	UpdateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error
	DeleteTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) error
	GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error)
	GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error)
	GetTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error)
	UpdateTrustScore(ctx context.Context, score *storage.TrustScore) error
	RecordTrustUpdate(ctx context.Context, update *storage.TrustUpdate) error
	GetAllTrustRelationships(ctx context.Context, limit int) ([]*storage.TrustRelationship, error)
	GetUserTrustScore(ctx context.Context, userID string) (float64, error)
}

// UserRepository interface defines the methods we need from the user repository
type UserRepository interface {
	CreateUser(ctx context.Context, user *storage.User) error
	GetUser(ctx context.Context, username string) (*storage.User, error)
	GetUserByEmail(ctx context.Context, email string) (*storage.User, error)
	UpdateUser(ctx context.Context, username string, updates map[string]any) error
	DeleteUser(ctx context.Context, username string) error

	// Bookmark methods
	CreateBookmark(ctx context.Context, username, objectID string) error
	RemoveBookmark(ctx context.Context, username, objectID string) error
	GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	IsBookmarked(ctx context.Context, username, objectID string) (bool, error)

	// Account management methods
	RemoveFromFollowers(ctx context.Context, username, followerUsername string) error

	// Reputation storage operations
	StoreReputation(ctx context.Context, actorID string, reputation *storage.Reputation) error
	GetReputation(ctx context.Context, actorID string) (*storage.Reputation, error)
	GetReputationHistory(ctx context.Context, actorID string, limit int) ([]*storage.Reputation, error)

	// Vouch operations
	CreateVouch(ctx context.Context, vouch *storage.Vouch) error
	GetVouch(ctx context.Context, vouchID string) (*storage.Vouch, error)
	GetVouchesByActor(ctx context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error)
	GetVouchesForActor(ctx context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error)
	UpdateVouchStatus(ctx context.Context, vouchID string, active bool, revokedAt *time.Time) error
	GetMonthlyVouchCount(ctx context.Context, actorID string, year int, month time.Month) (int, error)

	// DNS cache operations
	GetDNSCache(ctx context.Context, hostname string) (*storage.DNSCacheEntry, error)
	SetDNSCache(ctx context.Context, entry *storage.DNSCacheEntry) error

	// Cache operations for remote actors
	CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error

	// Advanced Timeline operations
	DeleteFromTimeline(ctx context.Context, timelineType, timelineID, entryID string) error
	DeleteExpiredTimelineEntries(ctx context.Context, before time.Time) error
	FanOutPost(ctx context.Context, activity *activitypub.Activity) error
	GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*storage.TimelineEntry, string, error)
	GetDirectTimeline(ctx context.Context, username string, limit int, cursor string) ([]*storage.TimelineEntry, string, error)
	GetHashtagTimeline(ctx context.Context, hashtag string, local bool, limit int, cursor string) ([]*storage.TimelineEntry, string, error)

	// User management operations
	ListUsers(ctx context.Context, limit int32, cursor string) ([]*storage.User, string, error)
	ListUsersByRole(ctx context.Context, role string) ([]*storage.User, error)

	// Follow request operations
	GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	GetFollowRequestState(ctx context.Context, followerUsername, followedUsername string) (string, error)
	AcceptFollow(ctx context.Context, followerUsername, followedUsername string) error
	RejectFollow(ctx context.Context, followerUsername, followedUsername string) error
	// User preferences methods
	GetUserLanguagePreference(ctx context.Context, username string) (string, error)
	SetUserLanguagePreference(ctx context.Context, username, language string) error
	GetUserPreferences(ctx context.Context, username string) (*storage.UserPreferences, error)
	UpdateUserPreferences(ctx context.Context, username string, preferences *storage.UserPreferences) error
	SetPreference(ctx context.Context, username, key string, value any) error
	GetPreference(ctx context.Context, username, key string) (any, error)
	GetAllPreferences(ctx context.Context, username string) (map[string]any, error)
	UpdatePreferences(ctx context.Context, username string, preferences map[string]any) error
	
	// Notification methods
	IsNotificationMuted(ctx context.Context, userID, targetID string) (bool, error)
	
	// Provider account methods
	GetUserByProviderID(ctx context.Context, provider, providerID string) (*storage.User, error)
	LinkProviderAccount(ctx context.Context, username, provider, providerID string) error
	UnlinkProviderAccount(ctx context.Context, username, provider string) error
	GetLinkedProviders(ctx context.Context, username string) ([]string, error)

	// Conversation mute methods
	CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error
	DeleteConversationMute(ctx context.Context, username, conversationID string) error
	IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error)
	GetMutedConversations(ctx context.Context, username string) ([]string, error)

	// Account Pin methods
	CreateAccountPin(ctx context.Context, pin *storage.AccountPin) error
	DeleteAccountPin(ctx context.Context, username, pinnedActorID string) error
	GetAccountPins(ctx context.Context, username string) ([]*storage.AccountPin, error)
	IsAccountPinned(ctx context.Context, username, pinnedActorID string) (bool, error)

	// Account Note methods
	CreateAccountNote(ctx context.Context, note *storage.AccountNote) error
	UpdateAccountNote(ctx context.Context, note *storage.AccountNote) error
	DeleteAccountNote(ctx context.Context, username, targetActorID string) error
	GetAccountNote(ctx context.Context, username, targetActorID string) (*storage.AccountNote, error)
}

// ConversationRepository interface defines the methods we need from the conversation repository
type ConversationRepository interface {
	// Conversation methods
	CreateConversation(ctx context.Context, conversation *storage.Conversation) error
	GetConversation(ctx context.Context, id string) (*storage.Conversation, error)
	UpdateConversation(ctx context.Context, conversation *storage.Conversation) error
	DeleteConversation(ctx context.Context, id string) error
	GetUserConversations(ctx context.Context, username string, limit int, cursor string) ([]*storage.Conversation, string, error)
	GetConversationByParticipants(ctx context.Context, participants []string) (*storage.Conversation, error)
	MarkConversationRead(ctx context.Context, conversationID, username string) error
	GetUnreadConversationCount(ctx context.Context, username string) (int, error)

	// ConversationStatus methods
	AddStatusToConversation(ctx context.Context, conversationID, statusID, senderUsername string) error
	GetConversationStatuses(ctx context.Context, conversationID string, limit int, cursor string) ([]*storage.ConversationStatus, string, error)
	RemoveStatusFromConversation(ctx context.Context, conversationID, statusID string) error
	MarkStatusRead(ctx context.Context, conversationID, statusID, username string) error
	GetUnreadStatusCount(ctx context.Context, conversationID, username string) (int, error)

	// Additional methods
	LeaveConversation(ctx context.Context, conversationID, username string) error
	AddParticipantToConversation(ctx context.Context, conversationID, username string) error
	GetConversationParticipants(ctx context.Context, conversationID string) ([]string, error)
	UpdateConversationLastStatus(ctx context.Context, id, lastStatusID string) error

	// Conversation mute methods
	CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error
	DeleteConversationMute(ctx context.Context, username, conversationID string) error
	IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error)
	GetMutedConversations(ctx context.Context, username string) ([]string, error)
}

// FollowRepository interface defines the methods we need from the follow repository
type FollowRepository interface {
	CreateFollow(ctx context.Context, followerUsername, followedUsername, followActivityID string) error
	RemoveFollow(ctx context.Context, followerUsername, followedUsername string) error
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]*models.Follow, string, error)
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]*models.Follow, string, error)
	IsFollowing(ctx context.Context, followerUsername, followedUsername string) (bool, error)
}

// TimelineRepository interface defines the methods we need from the timeline repository
type TimelineRepository interface {
	CreateTimelineEntry(ctx context.Context, entry *models.Timeline) error
	CreateTimelineEntries(ctx context.Context, entries []*models.Timeline) error
	GetHomeTimeline(ctx context.Context, username string, limit int, cursor string) ([]*models.Timeline, string, error)
	GetPublicTimeline(ctx context.Context, local bool, limit int, cursor string) ([]*models.Timeline, string, error)
	GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*models.Timeline, string, error)
	GetDirectTimeline(ctx context.Context, username string, limit int, cursor string) ([]*models.Timeline, string, error)
	GetHashtagTimeline(ctx context.Context, hashtag string, local bool, limit int, cursor string) ([]*models.Timeline, string, error)
	DeleteTimelineEntry(ctx context.Context, timelineType, timelineID, entryID string, timelineAt time.Time) error
	DeleteExpiredTimelineEntries(ctx context.Context, before time.Time) error
}

// NotificationRepository interface defines the methods we need from the notification repository
type NotificationRepository interface {
	CreateNotification(ctx context.Context, notification *models.Notification) error
	GetNotification(ctx context.Context, notificationID string) (*models.Notification, error)
	GetNotificationsByUser(ctx context.Context, username string, limit int, cursor string) ([]*models.Notification, string, error)
	MarkNotificationAsRead(ctx context.Context, notificationID string) error
	DeleteNotification(ctx context.Context, notificationID string) error
	
	// Enhanced methods
	GetNotificationsFiltered(ctx context.Context, username string, filter *storage.NotificationFilter) ([]*storage.Notification, string, error)
	MarkAllNotificationsAsRead(ctx context.Context, username string) error
	GetNotificationsAdvanced(ctx context.Context, userID string, excludeTypes []string, maxID, sinceID, minID *string, limit int, includeFiltered bool) ([]*storage.Notification, error)
	GetUnreadNotificationCount(ctx context.Context, userID string) (int64, error)
	CountUnreadNotifications(ctx context.Context, username string) (int, error)
	
	// Notification preferences
	GetNotificationPreferences(ctx context.Context, username string) (*storage.NotificationPreferences, error)
	UpdateNotificationPreferences(ctx context.Context, username string, prefs *storage.NotificationPreferences) error
	BatchMarkNotificationsAsRead(ctx context.Context, username string, notificationIDs []string) error
	SetNotificationPreference(ctx context.Context, username string, preference string, enabled bool) error
	
	// Delivery tracking
	RecordDeliveryAttempt(ctx context.Context, notificationID, method string, success bool, errorMsg string) error
	GetDeliveryStatus(ctx context.Context, notificationID, method string) (*models.NotificationDelivery, error)
	MarkDeliveryComplete(ctx context.Context, notificationID, method string) error
	GetFailedDeliveries(ctx context.Context, since time.Time, limit int) ([]*models.NotificationDelivery, error)
	RetryFailedDeliveries(ctx context.Context, before time.Time) error
	
	// Push subscriptions
	CreatePushSubscription(ctx context.Context, username string, subscription *storage.PushSubscription) error
	GetPushSubscriptions(ctx context.Context, username string) ([]*storage.PushSubscription, error)
	UpdatePushSubscription(ctx context.Context, username, subscriptionID string, alerts storage.PushSubscriptionAlerts) error
	DeletePushSubscription(ctx context.Context, username, subscriptionID string) error
	DeleteExpiredSubscriptions(ctx context.Context, before time.Time) error
	UpdateLastUsed(ctx context.Context, username, subscriptionID string) error
	
	// Stats and maintenance
	GetNotificationStats(ctx context.Context, username string) (map[string]int64, error)
	ClearOldNotifications(ctx context.Context, username string, olderThan time.Duration) error
}

// LikeRepository interface defines the methods we need from the like repository
type LikeRepository interface {
	CreateLike(ctx context.Context, actor, object string) (*models.Like, error)
	GetLike(ctx context.Context, actor, object string) (*models.Like, error)
	DeleteLike(ctx context.Context, actor, object string) error
	GetObjectLikes(ctx context.Context, objectID string, limit int, cursor string) ([]*models.Like, string, error)
	GetActorLikes(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Like, string, error)
	HasLiked(ctx context.Context, actor, object string) (bool, error)

	// Tombstone methods
	TombstoneObject(ctx context.Context, objectID string, deletedBy string) error
	GetTombstone(ctx context.Context, objectID string) (*storage.Tombstone, error)

	// Cascade delete methods
	CascadeDeleteLikes(ctx context.Context, objectID string) error

	// Boost/Like count methods
	GetLikeCount(ctx context.Context, statusID string) (int64, error)
	GetBoostCount(ctx context.Context, statusID string) (int64, error)
	IncrementReblogCount(ctx context.Context, objectID string) error
}

// OAuthRepository interface defines the methods we need from the OAuth repository
type OAuthRepository interface {
	CreateAuthorizationCode(ctx context.Context, code *storage.AuthorizationCode) error
	GetAuthorizationCode(ctx context.Context, code string) (*storage.AuthorizationCode, error)
	DeleteAuthorizationCode(ctx context.Context, code string) error
	CreateRefreshToken(ctx context.Context, token *storage.RefreshToken) error
	GetRefreshToken(ctx context.Context, token string) (*storage.RefreshToken, error)
	DeleteRefreshToken(ctx context.Context, token string) error
	CreateOAuthClient(ctx context.Context, client *storage.OAuthClient) error
	GetOAuthClient(ctx context.Context, clientID string) (*storage.OAuthClient, error)
	UpdateOAuthClient(ctx context.Context, clientID string, updates map[string]any) error
	DeleteOAuthClient(ctx context.Context, clientID string) error
	ListOAuthClients(ctx context.Context, limit int32, cursor string) ([]*storage.OAuthClient, string, error)
	StoreOAuthState(ctx context.Context, state string, data *storage.OAuthState) error
	GetOAuthState(ctx context.Context, state string) (*storage.OAuthState, error)
	DeleteOAuthState(ctx context.Context, state string) error
	GetOAuthApp(ctx context.Context, clientID string) (*storage.OAuthApp, error)
	SaveUserAppConsent(ctx context.Context, consent *storage.UserAppConsent) error
	GetUserAppConsent(ctx context.Context, userID, appID string) (*storage.UserAppConsent, error)
}

// SearchRepository interface defines the methods we need from the search repository
type SearchRepository interface {
	SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error)
	SearchStatuses(ctx context.Context, query string, limit int) ([]*storage.StatusSearchResult, error)
	SearchStatusesWithOptions(ctx context.Context, query string, options storage.StatusSearchOptions) ([]*storage.StatusSearchResult, error)
	SearchAll(ctx context.Context, query string, limit int, accountID string) (*storage.SearchResults, error)
	SearchAccountsAdvanced(ctx context.Context, query string, resolve bool, limit int, offset int, following bool, accountID string) ([]*activitypub.Actor, error)
	SearchStatusesAdvanced(ctx context.Context, query string, limit int, maxID, minID *string, accountID string) ([]*storage.StatusSearchResult, error)
	SearchHashtagsAdvanced(ctx context.Context, query string, limit int, accountID string) ([]*storage.HashtagSearchResult, error)
	SearchHashtags(ctx context.Context, query string, limit int) ([]*storage.Hashtag, error)
}

// SessionRepository interface defines the methods we need from the session repository
type SessionRepository interface {
	CreateSession(ctx context.Context, session *storage.Session) error
	GetSession(ctx context.Context, sessionID string) (*storage.Session, error)
	GetSessionByRefreshToken(ctx context.Context, refreshToken string) (*storage.Session, error)
	UpdateSession(ctx context.Context, session *storage.Session) error
	DeleteSession(ctx context.Context, sessionID string) error
	GetUserSessions(ctx context.Context, username string) ([]*storage.Session, error)
	CreateDevice(ctx context.Context, device *storage.Device) error
	GetDevice(ctx context.Context, deviceID string) (*storage.Device, error)
	UpdateDevice(ctx context.Context, device *storage.Device) error
	GetUserDevices(ctx context.Context, username string) ([]*storage.Device, error)
	StoreWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error
	GetWebAuthnCredential(ctx context.Context, credentialID string) (*storage.WebAuthnCredential, error)
	GetUserWebAuthnCredentials(ctx context.Context, username string) ([]*storage.WebAuthnCredential, error)
	UpdateWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error
	DeleteWebAuthnCredential(ctx context.Context, credentialID string) error
	StoreWebAuthnChallenge(ctx context.Context, challenge *storage.WebAuthnChallenge) error
	GetWebAuthnChallenge(ctx context.Context, challengeID string) (*storage.WebAuthnChallenge, error)
	DeleteWebAuthnChallenge(ctx context.Context, challengeID string) error
}

// ModerationRepository interface defines the methods we need from the moderation repository
type ModerationRepository interface {
	// Flag methods
	CreateFlag(ctx context.Context, flag *storage.Flag) error
	GetFlag(ctx context.Context, id string) (*storage.Flag, error)
	GetFlagsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Flag, string, error)
	GetFlagsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Flag, string, error)
	GetPendingFlags(ctx context.Context, limit int, cursor string) ([]*storage.Flag, string, error)
	UpdateFlagStatus(ctx context.Context, id string, status storage.FlagStatus, reviewedBy string, reviewNote string) error
	CountPendingFlags(ctx context.Context) (int, error)


	// Report methods
	CreateReport(ctx context.Context, report *storage.Report) error
	GetReport(ctx context.Context, id string) (*storage.Report, error)
	GetUserReports(ctx context.Context, username string, limit int, cursor string) ([]*storage.Report, string, error)
	UpdateReportStatus(ctx context.Context, id string, status storage.ReportStatus, actionTaken string, moderatorID string) error
	GetReportsByTarget(ctx context.Context, targetAccountID string, limit int, cursor string) ([]*storage.Report, string, error)
	GetReportsByStatus(ctx context.Context, status storage.ReportStatus, limit int, cursor string) ([]*storage.Report, string, error)
	GetReportStats(ctx context.Context, username string) (*storage.ReportStats, error)
	IncrementFalseReports(ctx context.Context, username string) error
	AssignReport(ctx context.Context, reportID string, assignedTo string) error
	UnassignReport(ctx context.Context, reportID string) error
	GetOpenReportsCount(ctx context.Context) (int, error)
	GetReportedStatuses(ctx context.Context, reportID string) ([]any, error)

	// Core event methods
	CreateModerationEvent(ctx context.Context, event *storage.ModerationEvent) error
	GetModerationEvent(ctx context.Context, eventID string) (*storage.ModerationEvent, error)
	GetModerationQueue(ctx context.Context, filter *storage.ModerationFilter) ([]*storage.ModerationQueueItem, error)
	GetModerationQueuePaginated(ctx context.Context, limit int, cursor string) ([]*storage.ModerationQueueItem, string, error)
	GetModerationEventsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error)
	GetModerationEventsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error)

	// Review methods
	AddModerationReview(ctx context.Context, review *storage.ModerationReview) error
	GetModerationReviews(ctx context.Context, eventID string) ([]*storage.ModerationReview, error)

	// Decision methods
	CreateModerationDecision(ctx context.Context, decision *storage.ModerationDecision) error
	GetModerationDecision(ctx context.Context, objectID string) (*storage.ModerationDecision, error)
	StoreModerationDecision(ctx context.Context, decision *storage.ModerationDecision) error
	UpdateModerationDecision(ctx context.Context, contentID string, review *storage.ModerationReview) error

	// Pattern methods
	CreateModerationPattern(ctx context.Context, pattern *storage.ModerationPattern) error
	GetModerationPattern(ctx context.Context, patternID string) (*storage.ModerationPattern, error)
	RecordPatternMatch(ctx context.Context, patternID string, matched bool, timestamp time.Time) error
	GetModerationPatterns(ctx context.Context, active bool, severity string, limit int) ([]*storage.ModerationPattern, error)
	UpdateModerationPattern(ctx context.Context, pattern *storage.ModerationPattern) error
	DeleteModerationPattern(ctx context.Context, patternID string) error

	// Additional moderation methods
	GetModerationHistory(ctx context.Context, objectID string) (*storage.ModerationHistory, error)
	GetModerationEvents(ctx context.Context, filter *storage.ModerationEventFilter, limit int, cursor string) ([]*storage.ModerationEvent, string, error)
	CreateAdminReview(ctx context.Context, eventID string, adminID string, action storage.ActionType, reason string) error
	GetReviewerStats(ctx context.Context, reviewerID string) (*storage.ReviewerStats, error)
	GetModerationQueueCount(ctx context.Context) (int, error)

	// Filter methods
	CreateFilter(ctx context.Context, filter *storage.Filter) error
	GetFilter(ctx context.Context, filterID string) (*storage.Filter, error)
	GetFiltersForUser(ctx context.Context, username string) ([]*storage.Filter, error)
	UpdateFilter(ctx context.Context, filterID string, updates map[string]any) error
	DeleteFilter(ctx context.Context, filterID string) error
	AddFilterKeyword(ctx context.Context, filterID string, keyword *storage.FilterKeyword) error
	GetFilterKeywords(ctx context.Context, filterID string) ([]*storage.FilterKeyword, error)
	UpdateFilterKeyword(ctx context.Context, keywordID string, updates map[string]any) error
	DeleteFilterKeyword(ctx context.Context, keywordID string) error
	AddFilterStatus(ctx context.Context, filterID string, status *storage.FilterStatus) error
	GetFilterStatuses(ctx context.Context, filterID string) ([]*storage.FilterStatus, error)
	DeleteFilterStatus(ctx context.Context, statusID string) error
}

// ModerationMetricsRepository interface defines the methods we need from the moderation metrics repository
type ModerationMetricsRepository interface {
	// Metrics recording
	RecordMetricsEntry(ctx context.Context, entry *models.ModerationMetricsEntry) error
	RecordMetricsEntries(ctx context.Context, entries []*models.ModerationMetricsEntry) error
	
	// False positive tracking
	RecordFalsePositive(ctx context.Context, fp *models.ModerationFalsePositive) error
	GetFalsePositives(ctx context.Context, timeRange models.ModerationMetricsTimeRange) ([]*models.ModerationFalsePositive, error)
	
	// Decision sampling
	RecordDecisionSample(ctx context.Context, sample *models.ModerationDecisionSample) error
	GetDecisionSamples(ctx context.Context, timeRange models.ModerationMetricsTimeRange, decision string) ([]*models.ModerationDecisionSample, error)
	
	// Pattern statistics
	UpdatePatternStats(ctx context.Context, stats *models.ModerationPatternStats) error
	GetTopPatterns(ctx context.Context, limit int) ([]*models.ModerationPatternStats, error)
	IncrementPatternHit(ctx context.Context, patternID, patternName string) error
	
	// Statistics retrieval
	GetMetricsEntries(ctx context.Context, timeRange models.ModerationMetricsTimeRange, metricTypes []string) ([]*models.ModerationMetricsEntry, error)
	GetAggregatedStats(ctx context.Context, timeRange models.ModerationMetricsTimeRange) (*models.ModerationMetricsStats, error)
}

// SocialRepository interface defines the methods we need from the social repository
type SocialRepository interface {
	// Block methods
	CreateBlock(ctx context.Context, block *storage.Block) error
	GetBlock(ctx context.Context, actor, blockedActor string) (*storage.Block, error)
	DeleteBlock(ctx context.Context, actor, blockedActor string) error
	IsBlocked(ctx context.Context, actor, targetActor string) (bool, error)
	GetBlockedUsers(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error)
	GetBlockedByUsers(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error)

	// Mute methods
	CreateMute(ctx context.Context, mute *storage.Mute) error
	GetMute(ctx context.Context, actor, mutedActor string) (*storage.Mute, error)
	DeleteMute(ctx context.Context, actor, mutedActor string) error
	IsMuted(ctx context.Context, actor, targetActor string) (bool, error)
	GetMutedUsers(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Mute, string, error)

	// Announce methods
	CreateAnnounce(ctx context.Context, announce *storage.Announce) error
	DeleteAnnounce(ctx context.Context, actor, object string) error
	GetAnnounce(ctx context.Context, actor, object string) (*storage.Announce, error)
	GetStatusAnnounces(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Announce, string, error)
	HasUserAnnounced(ctx context.Context, actor, object string) (bool, error)
	GetActorAnnounces(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Announce, string, error)
	CountObjectAnnounces(ctx context.Context, objectID string) (int, error)
	CascadeDeleteAnnounces(ctx context.Context, objectID string) error

	// Account Pin methods
	CreateAccountPin(ctx context.Context, pin *storage.AccountPin) error
	DeleteAccountPin(ctx context.Context, username, pinnedActorID string) error
	GetAccountPins(ctx context.Context, username string) ([]*storage.AccountPin, error)
	IsAccountPinned(ctx context.Context, username, pinnedActorID string) (bool, error)

	// Account Note methods
	CreateAccountNote(ctx context.Context, note *storage.AccountNote) error
	UpdateAccountNote(ctx context.Context, note *storage.AccountNote) error
	DeleteAccountNote(ctx context.Context, username, targetActorID string) error
	GetAccountNote(ctx context.Context, username, targetActorID string) (*storage.AccountNote, error)

	// Status Pin methods
	CreateStatusPin(ctx context.Context, pin *storage.StatusPin) error
	DeleteStatusPin(ctx context.Context, username, statusID string) error
	GetStatusPins(ctx context.Context, username string) ([]*storage.StatusPin, error)
	IsStatusPinned(ctx context.Context, username, statusID string) (bool, error)
	CountUserPinnedStatuses(ctx context.Context, username string) (int, error)
	ReorderStatusPins(ctx context.Context, username string, statusIDs []string) error
}

// RelationshipRepository interface defines the methods we need from the relationship repository
type RelationshipRepository interface {
	// Follow request methods
	GetFollowRequest(ctx context.Context, followerID, targetID string) (*storage.RelationshipRecord, error)
	AcceptFollowRequest(ctx context.Context, followerID, targetID string) error
	RejectFollowRequest(ctx context.Context, followerID, targetID string) error
	HasFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error)
	HasPendingFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error)
	
	// Endorsement methods
	IsEndorsed(ctx context.Context, userID, targetID string) (bool, error)
	
	// Relationship note methods
	GetRelationshipNote(ctx context.Context, userID, targetID string) (*storage.AccountNote, error)
	
	// Move methods
	CreateMove(ctx context.Context, move *storage.Move) error
	GetMove(ctx context.Context, actor string) (*storage.Move, error)
	GetMoveByTarget(ctx context.Context, target string) ([]*storage.Move, error)
	HasMovedFrom(ctx context.Context, oldActor, newActor string) (bool, error)
}

// ListRepository interface defines the methods we need from the list repository
type ListRepository interface {
	// List methods
	CreateList(ctx context.Context, username, title, repliesPolicy string) (*storage.List, error)
	GetList(ctx context.Context, listID string) (*storage.List, error)
	UpdateList(ctx context.Context, listID string, updates map[string]any) error
	DeleteList(ctx context.Context, listID string) error
	GetListsForUser(ctx context.Context, username string) ([]*storage.List, error)
	GetUserLists(ctx context.Context, username string) ([]*storage.List, error) // Alias for GetListsForUser
	CountUserLists(ctx context.Context, username string) (int, error)
	
	// ListMember methods
	AddAccountToList(ctx context.Context, listID, accountID string) error
	RemoveAccountFromList(ctx context.Context, listID, accountID string) error
	GetListMembers(ctx context.Context, listID string, limit int, cursor string) ([]*storage.ListMember, string, error)
	IsAccountInList(ctx context.Context, listID, accountID string) (bool, error)
	GetAccountLists(ctx context.Context, accountID string) ([]*storage.List, error)
	CountListMembers(ctx context.Context, listID string) (int, error)
	RemoveAccountFromAllLists(ctx context.Context, accountID string) error
	
	// Batch operations
	AddAccountsToList(ctx context.Context, listID string, accountIDs []string) error
	RemoveAccountsFromList(ctx context.Context, listID string, accountIDs []string) error
	GetListAccounts(ctx context.Context, listID string) ([]string, error)
	GetListsContainingAccount(ctx context.Context, accountID, username string) ([]*storage.List, error)
	
	// Timeline and other methods
	GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*storage.TimelineEntry, string, error)
	GetExclusiveLists(ctx context.Context, username string) ([]*storage.List, error)
}

// MediaRepository interface defines the methods we need from the media repository
type MediaRepository interface {
	GetUserMedia(ctx context.Context, username string) ([]any, error)
	UpdateMediaAttachment(ctx context.Context, mediaID string, updates map[string]any) error
	UnmarkAllMediaAsSensitive(ctx context.Context, username string) error
	
	// Media job operations
	CreateMediaJob(ctx context.Context, job *models.MediaJob) error
	GetMediaJob(ctx context.Context, jobID string) (*models.MediaJob, error)
	UpdateMediaJob(ctx context.Context, job *models.MediaJob) error
	
	// Media operations  
	CreateMedia(ctx context.Context, media *models.Media) error
	GetMedia(ctx context.Context, mediaID string) (*models.Media, error)
	UpdateMedia(ctx context.Context, media *models.Media) error
}

// PollRepository interface defines the methods we need from the poll repository
type PollRepository interface {
	CreatePoll(ctx context.Context, poll *storage.Poll) error
	GetPoll(ctx context.Context, pollID string) (*storage.Poll, error)
	GetPollByStatusID(ctx context.Context, statusID string) (*storage.Poll, error)
	GetPollVotes(ctx context.Context, pollID string) (map[string][]int, error)
	VoteOnPoll(ctx context.Context, pollID, userID string, choices []int) error
	HasUserVoted(ctx context.Context, pollID string, userID string) (bool, []int, error)
}

// PushSubscriptionRepository interface defines the methods we need from the push subscription repository
type PushSubscriptionRepository interface {
	CreatePushSubscription(ctx context.Context, username string, subscription *storage.PushSubscription) error
	GetPushSubscription(ctx context.Context, username, subscriptionID string) (*storage.PushSubscription, error)
	GetUserPushSubscriptions(ctx context.Context, username string) ([]*storage.PushSubscription, error)
	UpdatePushSubscription(ctx context.Context, username, subscriptionID string, alerts storage.PushSubscriptionAlerts) error
	DeletePushSubscription(ctx context.Context, username, subscriptionID string) error
	DeleteAllPushSubscriptions(ctx context.Context, username string) error
	
	// VAPID key operations
	GetVAPIDKeys(ctx context.Context) (*storage.VAPIDKeys, error)
	SetVAPIDKeys(ctx context.Context, keys *storage.VAPIDKeys) error
}

// InstanceRepository interface defines the methods we need from the instance repository
type InstanceRepository interface {
	// Instance rules methods
	GetInstanceRules(ctx context.Context) ([]storage.InstanceRule, error)
	SetInstanceRules(ctx context.Context, rules []storage.InstanceRule) error
	GetExtendedDescription(ctx context.Context) (string, time.Time, error)
	SetExtendedDescription(ctx context.Context, description string) error
	GetRulesByCategory(ctx context.Context, category string) ([]storage.InstanceRule, error)

	// Instance metrics methods
	GetTotalUserCount(ctx context.Context) (int64, error)
	GetTotalStatusCount(ctx context.Context) (int64, error)
	GetTotalDomainCount(ctx context.Context) (int64, error)
	GetActiveUserCount(ctx context.Context, days int) (int64, error)
	GetDailyActiveUserCount(ctx context.Context) (int64, error)
	GetLocalPostCount(ctx context.Context) (int64, error)
	GetWeeklyActivity(ctx context.Context, weekTimestamp int64) (*storage.WeeklyActivity, error)
	RecordActivity(ctx context.Context, activityType string, actorID string, timestamp time.Time) error
	GetContactAccount(ctx context.Context) (*storage.ActorRecord, error)

	// Storage and analytics
	GetStorageUsage(ctx context.Context) (any, error)
	GetStorageHistory(ctx context.Context, days int) ([]any, error)
	GetUserGrowthHistory(ctx context.Context, days int) ([]any, error)
	GetDomainStats(ctx context.Context, domain string) (any, error)
}

// InstanceHealthRepository interface defines methods for health checking operations
type InstanceHealthRepository interface {
	// Health check data operations
	SaveHealthCheck(ctx context.Context, health *models.InstanceHealth) error
	SaveHealthChecks(ctx context.Context, healthChecks []*models.InstanceHealth) error
	GetLatestHealthCheck(ctx context.Context, domain string) (*models.InstanceHealth, error)
	GetHealthHistory(ctx context.Context, domain string, since time.Time, limit int) ([]*models.InstanceHealth, error)
	
	// Domain management
	GetDomainsForHealthCheck(ctx context.Context, limit int) ([]string, error)
	GetUnhealthyInstances(ctx context.Context, threshold float64) ([]string, error)
	
	// Health summary operations
	SaveHealthSummary(ctx context.Context, summary *models.InstanceHealthSummary) error
	GetHealthSummary(ctx context.Context, domain string, window time.Duration) (*models.InstanceHealthSummary, error)
	CalculateHealthSummary(ctx context.Context, domain string, window time.Duration) (*models.InstanceHealthSummary, error)
	
	// Cleanup operations
	CleanupOldHealthData(ctx context.Context, olderThan time.Duration) (int, error)
}

// HashtagRepository interface defines the methods we need from the hashtag repository
type HashtagRepository interface {
	// Hashtag operations
	IndexHashtag(ctx context.Context, hashtag string, statusID string, authorID string, visibility string) error
	GetHashtagInfo(ctx context.Context, hashtag string) (*storage.Hashtag, error)
	GetHashtagUsageHistory(ctx context.Context, hashtag string, days int) ([]int64, error)
	GetHashtagActivity(ctx context.Context, hashtag string, since time.Time) ([]*storage.Activity, error)
	GetHashtagStats(ctx context.Context, hashtag string) (any, error)
	
	// Hashtag timeline operations
	GetHashtagTimelineAdvanced(ctx context.Context, hashtag string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error)
	GetMultiHashtagTimeline(ctx context.Context, hashtags []string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error)
	GetSuggestedHashtags(ctx context.Context, userID string, limit int) ([]*storage.HashtagSearchResult, error)
	
	// Trending hashtag operations
	GetTrendingHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error)
	GetRecentHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error)
	StoreHashtagTrend(ctx context.Context, trend any) error
	DeleteOldHashtagTrends(ctx context.Context, before time.Time) error
	
	// Hashtag follow operations
	FollowHashtag(ctx context.Context, userID string, hashtag string) error
	UnfollowHashtag(ctx context.Context, userID string, hashtag string) error
	IsFollowingHashtag(ctx context.Context, userID string, hashtag string) (bool, error)
	GetFollowedHashtags(ctx context.Context, userID string, limit int, cursor string) ([]string, string, error)
	UpdateHashtagNotificationSettings(ctx context.Context, userID, hashtag string, notify bool) error
	MuteHashtag(ctx context.Context, userID, hashtag string) error
	UnmuteHashtag(ctx context.Context, userID, hashtag string) error
	IsHashtagMuted(ctx context.Context, userID, hashtag string) (bool, error)
}

// TrendingRepository interface defines the methods for trending/analytics operations
type TrendingRepository interface {
	RecordHashtagUsage(ctx context.Context, hashtag string, statusID string, authorID string) error
	RecordStatusEngagement(ctx context.Context, statusID string, engagementType string, userID string) error
	RecordLinkShare(ctx context.Context, url string, statusID string, authorID string) error
	GetTrendingHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error)
	GetTrendingStatuses(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error)
	GetTrendingLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error)
	GetRecentHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error)
	GetRecentStatusesWithEngagement(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error)
	GetRecentLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error)
	StoreEngagementMetrics(ctx context.Context, metrics *storage.EngagementMetrics) error
	GetEngagementMetrics(ctx context.Context, statusID string) (*storage.EngagementMetrics, error)
	// Trending storage methods
	StoreHashtagTrend(ctx context.Context, trend any) error
	StoreStatusTrend(ctx context.Context, trend any) error
	StoreLinkTrend(ctx context.Context, trend any) error
	DeleteOldHashtagTrends(ctx context.Context, before time.Time) error
	DeleteOldStatusTrends(ctx context.Context, before time.Time) error
	DeleteOldLinkTrends(ctx context.Context, before time.Time) error
	GetStatusesByLink(ctx context.Context, linkURL string, limit int) ([]any, error)
	// Search analytics methods
	TrackSearchQuery(ctx context.Context, userID, query string, resultCount int) error
	GetPopularSearchQueries(ctx context.Context, limit int, timeWindow time.Duration) ([]storage.SearchQueryStats, error)
	GetUserSearchHistory(ctx context.Context, userID string, limit int) ([]storage.SearchHistoryEntry, error)
	// Engagement indexing
	IndexByEngagement(ctx context.Context, statusID string, bucket string) error
	// Search suggestions
	GenerateSearchSuggestions(ctx context.Context, userID, partialQuery string, limit int) ([]string, error)
	// Media analytics methods
	RecordManifestGeneration(ctx context.Context, mediaID, format string, duration float64) error
	RecordQualityChange(ctx context.Context, mediaID, userID, oldQuality, newQuality string) error
	RecordMediaEvent(ctx context.Context, eventType, mediaID, userID string) error
	GetManifestGenerationStats(ctx context.Context, format, startDate, endDate string) (map[string]int64, error)
	GetMediaEventStats(ctx context.Context, eventType, startDate, endDate string) (map[string]int64, error)
}

// FeaturedTagRepository interface defines the methods we need from the featured tag repository
type FeaturedTagRepository interface {
	CreateFeaturedTag(ctx context.Context, tag *storage.FeaturedTag) error
	DeleteFeaturedTag(ctx context.Context, username, name string) error
	GetFeaturedTags(ctx context.Context, username string) ([]*storage.FeaturedTag, error)
	GetTagSuggestions(ctx context.Context, username string, limit int) ([]string, error)
}

type ScheduledStatusRepository interface {
	CreateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error
	GetScheduledStatus(ctx context.Context, id string) (*storage.ScheduledStatus, error)
	GetScheduledStatuses(ctx context.Context, username string, limit int, cursor string) ([]*storage.ScheduledStatus, string, error)
	UpdateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error
	DeleteScheduledStatus(ctx context.Context, id string) error
	GetDueScheduledStatuses(ctx context.Context, before time.Time, limit int) ([]*storage.ScheduledStatus, error)
	MarkScheduledStatusPublished(ctx context.Context, id string) error
	GetScheduledStatusMedia(ctx context.Context, id string) ([]any, error)
}

// MarkerRepository interface defines the methods we need from the marker repository
type MarkerRepository interface {
	SaveMarker(ctx context.Context, username, timeline string, lastReadID string, version int) error
	GetMarkers(ctx context.Context, username string, timelines []string) (map[string]*storage.Marker, error)
}

// AnnouncementRepository interface defines the methods we need from the announcement repository
type AnnouncementRepository interface {
	CreateAnnouncement(ctx context.Context, announcement *storage.Announcement) error
	GetAnnouncement(ctx context.Context, id string) (*storage.Announcement, error)
	GetAnnouncements(ctx context.Context, active bool) ([]*storage.Announcement, error)
	UpdateAnnouncement(ctx context.Context, announcement *storage.Announcement) error
	DeleteAnnouncement(ctx context.Context, id string) error
	DismissAnnouncement(ctx context.Context, username, announcementID string) error
	IsDismissed(ctx context.Context, username, announcementID string) (bool, error)
	GetDismissedAnnouncements(ctx context.Context, username string) ([]string, error)
	AddAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error
	RemoveAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error
	GetAnnouncementReactions(ctx context.Context, announcementID string) (map[string][]string, error)
}

// EmojiRepository interface defines the methods we need from the emoji repository
type EmojiRepository interface {
	CreateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error
	GetCustomEmoji(ctx context.Context, shortcode string) (*storage.CustomEmoji, error)
	GetCustomEmojis(ctx context.Context) ([]*storage.CustomEmoji, error)
	UpdateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error
	DeleteCustomEmoji(ctx context.Context, shortcode string) error
	GetCustomEmojisByCategory(ctx context.Context, category string) ([]*storage.CustomEmoji, error)
}

// DomainBlockRepository interface defines the methods we need from the domain block repository
type DomainBlockRepository interface {
	// User-level domain blocks
	AddDomainBlock(ctx context.Context, username, domain string) error
	RemoveDomainBlock(ctx context.Context, username, domain string) error
	GetUserDomainBlocks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	IsBlockedDomain(ctx context.Context, username, domain string) (bool, error)

	// Instance domain blocks
	CreateInstanceDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error
	GetInstanceDomainBlock(ctx context.Context, domain string) (*storage.InstanceDomainBlock, error)
	GetInstanceDomainBlockByID(ctx context.Context, id string) (*storage.InstanceDomainBlock, error)
	ListInstanceDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error)
	UpdateInstanceDomainBlock(ctx context.Context, domain string, updates map[string]any) error
	DeleteInstanceDomainBlock(ctx context.Context, domain string) error
	IsInstanceDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error)
	GetDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error)
	GetDomainBlock(ctx context.Context, id string) (*storage.InstanceDomainBlock, error)
	CreateDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error
	UpdateDomainBlock(ctx context.Context, id string, updates map[string]any) error
	DeleteDomainBlock(ctx context.Context, id string) error
	IsDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error)

	// Email domain blocks
	CreateEmailDomainBlock(ctx context.Context, block *storage.EmailDomainBlock) error
	GetEmailDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.EmailDomainBlock, string, error)
	DeleteEmailDomainBlock(ctx context.Context, id string) error

	// Domain allow operations (for allowlist mode)
	GetDomainAllows(ctx context.Context, limit int, cursor string) ([]*storage.DomainAllow, string, error)
	CreateDomainAllow(ctx context.Context, allow *storage.DomainAllow) error
	DeleteDomainAllow(ctx context.Context, id string) error
}

// RelayRepository interface defines the methods we need from the relay repository
type RelayRepository interface {
	StoreRelayInfo(ctx context.Context, relay *storage.RelayInfo) error
	GetRelayInfo(ctx context.Context, relayURL string) (*storage.RelayInfo, error)
	RemoveRelayInfo(ctx context.Context, relayURL string) error
	GetActiveRelays(ctx context.Context) ([]*storage.RelayInfo, error)
	GetAllRelays(ctx context.Context, limit int, cursor string) ([]*storage.RelayInfo, string, error)
	UpdateRelayStatus(ctx context.Context, relayURL string, active bool) error
}

// FederationRepository interface defines the methods we need from the federation repository
type FederationRepository interface {
	// Federation instance tracking
	GetInstanceInfo(ctx context.Context, domain string) (*storage.InstanceInfo, error)
	UpsertInstanceInfo(ctx context.Context, info *storage.InstanceInfo) error
	GetKnownInstances(ctx context.Context, limit int, cursor string) ([]*storage.InstanceInfo, string, error)
	GetFederationStatistics(ctx context.Context, startTime, endTime time.Time) (*storage.FederationStats, error)
	
	// Federation cost tracking
	RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error
	GetFederationCosts(ctx context.Context, startTime, endTime time.Time, limit int, cursor string) ([]*storage.FederationCost, string, error)
	GetInstanceHealthReport(ctx context.Context, domain string, period time.Duration) (*storage.InstanceHealthReport, error)
	GetCostProjections(ctx context.Context, period string) (*storage.CostProjection, error)
	
	// Federation graph methods
	GetFederationNodes(ctx context.Context, depth int) ([]*storage.FederationNode, error)
	GetFederationEdges(ctx context.Context, domains []string) ([]*storage.FederationEdge, error)
	GetInstanceMetadata(ctx context.Context, domain string) (*storage.InstanceMetadata, error)
	CalculateFederationClusters(ctx context.Context) ([]*storage.InstanceCluster, error)
	GetInstanceConnections(ctx context.Context, domain string, connectionType string) ([]*storage.InstanceConnection, error)
	
	// Federation severance methods
	AcknowledgeSeverance(ctx context.Context, userID, domain string) error
	AttemptReconnection(ctx context.Context, userID, domain string) error
	GetUserSeveredRelationships(ctx context.Context, userID string) ([]*storage.SeveredRelationship, error)
	GetAffectedRelationships(ctx context.Context, userID, domain string) ([]*storage.RelationshipRecord, error)
	TrackFederationIssue(ctx context.Context, domain, issueType string) error
	
	// Additional severed relationship methods from legacy implementation
	CreateSeveredRelationship(ctx context.Context, rel *models.SeveredRelationship) error
	GetSeveredRelationships(ctx context.Context, localInstance string, limit int, cursor string) ([]*models.SeveredRelationship, string, error)
	GetSeveredRelationship(ctx context.Context, localInstance, remoteInstance string) (*models.SeveredRelationship, error)
	UpdateSeveredRelationship(ctx context.Context, rel *models.SeveredRelationship) error
	GetAffectedFollows(ctx context.Context, localInstance, remoteInstance string) ([]models.AffectedFollow, error)
	RecordAffectedFollow(ctx context.Context, localInstance, remoteInstance string, follow models.AffectedFollow) error
	ReverseSeverance(ctx context.Context, localInstance, remoteInstance string) error
	GetSeveranceHistory(ctx context.Context, localInstance, remoteInstance string, limit int) ([]*models.SeveredRelationship, error)
	
	// Additional federation methods
	GetRecentInstanceConnections(ctx context.Context, domain string, since time.Duration) ([]*storage.InstanceConnection, error)
	UpdateFederationNode(ctx context.Context, node *storage.FederationNode) error
	UpdateFederationEdge(ctx context.Context, edge *storage.FederationEdge) error
	UpdateInstanceMetadata(ctx context.Context, metadata *storage.InstanceMetadata) error
	StoreFederationTimeSeries(ctx context.Context, data *storage.FederationTimeSeries) error
	StoreInstanceCluster(ctx context.Context, cluster *storage.InstanceCluster) error
	GetStrongestConnectionsByType(ctx context.Context, connectionType string, limit int) ([]*storage.FederationEdge, error)
}

// FederationInstanceRepository interface defines the methods for federation instance registry operations
type FederationInstanceRepository interface {
	// Instance CRUD operations
	CreateInstance(ctx context.Context, instance *types.Instance) error
	GetInstance(ctx context.Context, instanceID string) (*types.Instance, error)
	GetInstanceByDomain(ctx context.Context, domain string) (*types.Instance, error)
	UpdateInstance(ctx context.Context, instance *types.Instance) error
	DeleteInstance(ctx context.Context, instanceID string) error

	// Instance queries
	ListInstancesByStatus(ctx context.Context, status types.InstanceStatus, limit int) ([]*types.Instance, error)
	ListHealthyInstances(ctx context.Context) ([]*types.Instance, error)
	GetInstancesByTier(ctx context.Context, tier types.TierLevel, limit int) ([]*types.Instance, error)
	BatchGetInstances(ctx context.Context, instanceIDs []string) ([]*types.Instance, error)
	SearchInstances(ctx context.Context, domainPattern string, limit int) ([]*types.Instance, error)
	ListAllInstances(ctx context.Context, limit int, startKey map[string]interface{}) ([]*types.Instance, map[string]interface{}, error)

	// Instance health and metrics
	UpdateInstanceHealth(ctx context.Context, instanceID string, health *types.HealthStatus) error
	UpdateInstanceUsage(ctx context.Context, instanceID string, bytesUsed int64) error
	GetHealthHistory(ctx context.Context, instanceID string, duration time.Duration) ([]*types.HealthStatus, error)
}

// AuthRepository interface defines methods for WebAuthn and Wallet authentication
type AuthRepository interface {
	// WebAuthn credential operations
	CreateWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error
	GetWebAuthnCredential(ctx context.Context, credentialID string) (*storage.WebAuthnCredential, error)
	GetUserWebAuthnCredentials(ctx context.Context, userID string) ([]*storage.WebAuthnCredential, error)
	DeleteWebAuthnCredential(ctx context.Context, credentialID string) error
	UpdateWebAuthnLastUsed(ctx context.Context, credentialID string, signCount uint32) error
	
	// WebAuthn challenge operations
	CreateWebAuthnChallenge(ctx context.Context, challenge *storage.WebAuthnChallenge) error
	GetWebAuthnChallenge(ctx context.Context, challengeID string) (*storage.WebAuthnChallenge, error)
	DeleteWebAuthnChallenge(ctx context.Context, challengeID string) error
	
	// Wallet challenge operations
	StoreWalletChallenge(ctx context.Context, challenge *storage.WalletChallenge) error
	GetWalletChallenge(ctx context.Context, challengeID string) (*storage.WalletChallenge, error)
	DeleteWalletChallenge(ctx context.Context, challengeID string) error
	
	// Wallet credential operations
	StoreWalletCredential(ctx context.Context, credential *storage.WalletCredential) error
	GetWalletByAddress(ctx context.Context, walletType, address string) (*storage.WalletCredential, error)
	GetUserWallets(ctx context.Context, username string) ([]*storage.WalletCredential, error)
	DeleteWalletCredential(ctx context.Context, username, address string) error
}

// WalletRepository interface defines the methods for wallet authentication
type WalletRepository interface {
	// Wallet challenge operations
	StoreWalletChallenge(ctx context.Context, challenge *storage.WalletChallenge) error
	GetWalletChallenge(ctx context.Context, challengeID string) (*storage.WalletChallenge, error)
	DeleteWalletChallenge(ctx context.Context, challengeID string) error
	
	// Wallet credential operations
	StoreWalletCredential(ctx context.Context, credential *storage.WalletCredential) error
	GetWalletCredential(ctx context.Context, walletType, address string) (*storage.WalletCredential, error)
	GetUserWalletCredentials(ctx context.Context, username string) ([]*storage.WalletCredential, error)
	DeleteWalletCredential(ctx context.Context, username, address string) error
	UpdateWalletLastUsed(ctx context.Context, username, address string) error
}

// RecoveryRepository interface defines the methods for social recovery operations
type RecoveryRepository interface {
	// Trustee operations
	StoreTrustee(ctx context.Context, username string, trustee *storage.TrusteeConfig) error
	GetTrustees(ctx context.Context, username string) ([]*storage.TrusteeConfig, error)
	DeleteTrustee(ctx context.Context, username, trusteeActorID string) error
	UpdateTrusteeConfirmed(ctx context.Context, username, trusteeActorID string, confirmed bool) error
	
	// Recovery request operations
	StoreRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error
	GetRecoveryRequest(ctx context.Context, requestID string) (*storage.SocialRecoveryRequest, error)
	UpdateRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error
	DeleteRecoveryRequest(ctx context.Context, requestID string) error
	GetActiveRecoveryRequests(ctx context.Context, username string) ([]*storage.SocialRecoveryRequest, error)
	
	// Recovery code operations
	StoreRecoveryCode(ctx context.Context, username string, code *storage.RecoveryCodeItem) error
	GetRecoveryCodes(ctx context.Context, username string) ([]*storage.RecoveryCodeItem, error)
	MarkRecoveryCodeUsed(ctx context.Context, username, codeHash string) error
	DeleteAllRecoveryCodes(ctx context.Context, username string) error
	CountUnusedRecoveryCodes(ctx context.Context, username string) (int, error)
	
	// Recovery token operations
	StoreRecoveryToken(ctx context.Context, key string, data map[string]any) error
	GetRecoveryToken(ctx context.Context, key string) (map[string]any, error)
	DeleteRecoveryToken(ctx context.Context, key string) error
}

// RateLimitRepository interface defines the methods we need from the rate limit repository
type RateLimitRepository interface {
	// Rate limiting operations
	RecordLoginAttempt(ctx context.Context, identifier string, success bool) error
	GetLoginAttemptCount(ctx context.Context, identifier string, since time.Time) (int, error)
	IsRateLimited(ctx context.Context, identifier string) (bool, time.Time, error)
	ClearLoginAttempts(ctx context.Context, identifier string) error
	CheckCommunityNoteRateLimit(ctx context.Context, userID string, limit int) (bool, int, error)
	
	// API rate limiting operations
	CheckAPIRateLimit(ctx context.Context, userID, endpoint string, limit int, window time.Duration) error
	GetAPIRateLimitInfo(ctx context.Context, userID, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error)
}

// StreamingRepository interface defines the methods we need from the streaming repository
type StreamingRepository interface {
	// Streaming preferences operations
	GetStreamingPreferences(ctx context.Context, username string) (*storage.StreamingPreferences, error)
	UpdateStreamingPreferences(ctx context.Context, prefs *storage.StreamingPreferences) error
	GetStreamingPreferencesByDevice(ctx context.Context, username, deviceID string) (*storage.StreamingPreferences, error)
	UpdateDeviceStreamingPreferences(ctx context.Context, prefs *storage.StreamingPreferences, deviceID string) error
	GetStreamingPreferenceHistory(ctx context.Context, username string, limit int) ([]*storage.StreamingPreferences, error)
	SyncStreamingPreferences(ctx context.Context, username string, sourceDeviceID string) error
	ResolvePreferenceConflict(ctx context.Context, username string, strategy storage.ConflictResolutionStrategy) (*storage.StreamingPreferences, error)
}

// CommunityNoteRepository interface defines the methods we need from the community note repository
type CommunityNoteRepository interface {
	CreateCommunityNote(ctx context.Context, note *storage.CommunityNote) error
	GetCommunityNote(ctx context.Context, noteID string) (*storage.CommunityNote, error)
	GetVisibleCommunityNotes(ctx context.Context, objectID string) ([]*storage.CommunityNote, error)
	UpdateCommunityNoteScore(ctx context.Context, noteID string, score float64, status string) error
	UpdateCommunityNoteAnalysis(ctx context.Context, noteID string, sentiment, objectivity, sourceQuality float64) error
	CreateCommunityNoteVote(ctx context.Context, vote *storage.CommunityNoteVote) error
	GetUserCommunityNoteVotes(ctx context.Context, userID string, noteIDs []string) (map[string]*storage.CommunityNoteVote, error)
	GetCommunityNotesByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*storage.CommunityNote, string, error)
	GetCommunityNoteVotes(ctx context.Context, noteID string) ([]*storage.CommunityNoteVote, error)
}

// CSRFRepository interface defines the methods we need from the CSRF repository
type CSRFRepository interface {
	Store(ctx context.Context, token string, userID string, expiresAt time.Time) error
	Get(ctx context.Context, token string) (string, string, time.Time, bool, error)
	Delete(ctx context.Context, token string) error
	ValidateAndConsume(ctx context.Context, token string, userID string) error
	GetUserActiveTokenCount(ctx context.Context, userID string) (int, error)
	CleanupUserTokens(ctx context.Context, userID string) error
	CleanExpired(ctx context.Context) error
}

// CircuitBreakerRepository interface defines the methods we need from the circuit breaker repository
type CircuitBreakerRepository interface {
	GetCircuitState(ctx context.Context, instanceID string) (*models.CircuitBreakerState, error)
	SaveCircuitState(ctx context.Context, state *models.CircuitBreakerState) error
	UpdateCircuitState(ctx context.Context, instanceID string, updateFn func(*models.CircuitBreakerState) error) (*models.CircuitBreakerState, error)
	RecordEvent(ctx context.Context, event *models.CircuitBreakerEvent) error
	RecordStateChange(ctx context.Context, instanceID, oldStatus, newStatus, reason string) error
	RecordMetric(ctx context.Context, instanceID string, success bool, err error, errorType string) error
	GetRecentEvents(ctx context.Context, instanceID string, limit int) ([]*models.CircuitBreakerEvent, error)
	DeleteCircuitState(ctx context.Context, instanceID string) error
	GetAllCircuitStates(ctx context.Context) ([]*models.CircuitBreakerState, error)
}

// RouteOptimizationRepository interface defines the methods for route performance tracking and optimization  
type RouteOptimizationRepository interface {
	// Core operations used by SmartRouteOptimizer
	RecordDeliveryResult(ctx context.Context, result *types.DeliveryResult) error
	GetRouteMetrics(ctx context.Context, routeID string) (*types.RouteMetrics, error)
	GetRoutePerformance(ctx context.Context, routeID string) (interface{}, error) // Returns internal perf data
	StoreOptimizationDecision(ctx context.Context, routes []*types.Route, messageSize int64) error
}

// RoutingMetricsRepository interface defines the methods for routing metrics aggregation
type RoutingMetricsRepository interface {
	// Core operations used by RoutingMetrics component
	RecordRouteSelection(ctx context.Context, routeID, destination, messageType string) error
	RecordDelivery(ctx context.Context, result *types.DeliveryResult) error
	GetMetrics(ctx context.Context, timeWindow time.Duration) (interface{}, error) // Returns aggregated metrics
}

// MediaSessionRepository interface defines the methods for media session management
type MediaSessionRepository interface {
	CreateSession(ctx context.Context, session *streaming.StreamingSession) error
	GetSession(ctx context.Context, sessionID string) (*streaming.StreamingSession, error)
	UpdateSession(ctx context.Context, session *streaming.StreamingSession) error
	EndSession(ctx context.Context, sessionID string) error
	GetUserSessions(ctx context.Context, userID string) ([]*streaming.StreamingSession, error)
	GetMediaSessions(ctx context.Context, mediaID string, limit int32) ([]*streaming.StreamingSession, error)
	CleanupExpiredSessions(ctx context.Context, maxAge time.Duration) error
	SetSessionTTL(ttl time.Duration)
}

// StorageAdapter adapts the DynamORM repositories to the storage.Storage interface
// This allows for incremental migration to DynamORM while maintaining backward compatibility
type StorageAdapter struct {
	// Repository fields
	accountRepo          AccountRepository
	actorRepo            ActorRepository
	objectRepo           ObjectRepository
	activityRepo         ActivityRepository
	userRepo             UserRepository
	trustRepo            TrustRepository
	conversationRepo     ConversationRepository
	followRepo           FollowRepository
	timelineRepo         TimelineRepository
	notificationRepo     NotificationRepository
	likeRepo             LikeRepository
	searchRepo           SearchRepository
	sessionRepo          SessionRepository
	moderationRepo       ModerationRepository
	socialRepo           SocialRepository
	relationshipRepo     RelationshipRepository
	listRepo             ListRepository
	mediaRepo            MediaRepository
	pollRepo             PollRepository
	pushSubscriptionRepo PushSubscriptionRepository
	instanceRepo         InstanceRepository
	instanceHealthRepo   InstanceHealthRepository
	hashtagRepo          HashtagRepository
	featuredTagRepo      FeaturedTagRepository
	trendingRepo         TrendingRepository
	scheduledStatusRepo  ScheduledStatusRepository
	markerRepo           MarkerRepository
	announcementRepo     AnnouncementRepository
	emojiRepo            EmojiRepository
	domainBlockRepo      DomainBlockRepository
	relayRepo            RelayRepository
	federationRepo       FederationRepository
	federationInstanceRepo FederationInstanceRepository
	walletRepo           WalletRepository
	recoveryRepo         RecoveryRepository
	rateLimitRepo        RateLimitRepository
	streamingRepo        StreamingRepository
	communityNoteRepo    CommunityNoteRepository
	csrfRepo             CSRFRepository
	circuitBreakerRepo   CircuitBreakerRepository
	routeOptimizationRepo RouteOptimizationRepository
	routingMetricsRepo   RoutingMetricsRepository
	mediaSessionRepo     MediaSessionRepository

	// Common fields
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewStorageAdapter creates a new StorageAdapter
func NewStorageAdapter(db core.DB, tableName string, logger *zap.Logger) *StorageAdapter {
	return &StorageAdapter{
		// Repositories will be set via SetActorRepository method
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// GetDB returns the underlying DynamORM database connection
func (a *StorageAdapter) GetDB() core.DB {
	return a.db
}

// SetActorRepository sets the actor repository
func (a *StorageAdapter) SetActorRepository(repo ActorRepository) {
	a.actorRepo = repo
	
	// Set dependencies if the repository supports it
	if repoWithDeps, ok := repo.(interface{ SetDependencies(ActorRepositoryDeps) }); ok {
		repoWithDeps.SetDependencies(a)
	}
}

// SetObjectRepository sets the object repository
func (a *StorageAdapter) SetObjectRepository(repo ObjectRepository) {
	a.objectRepo = repo
}

// SetActivityRepository sets the activity repository
func (a *StorageAdapter) SetActivityRepository(repo ActivityRepository) {
	a.activityRepo = repo
}

// SetUserRepository sets the user repository
func (a *StorageAdapter) SetUserRepository(repo UserRepository) {
	a.userRepo = repo
}

// SetTrustRepository sets the trust repository
func (a *StorageAdapter) SetTrustRepository(repo TrustRepository) {
	a.trustRepo = repo
}

// SetConversationRepository sets the conversation repository
func (a *StorageAdapter) SetConversationRepository(repo ConversationRepository) {
	a.conversationRepo = repo
}

// SetFollowRepository sets the follow repository
func (a *StorageAdapter) SetFollowRepository(repo FollowRepository) {
	a.followRepo = repo
}

// SetTimelineRepository sets the timeline repository
func (a *StorageAdapter) SetTimelineRepository(repo TimelineRepository) {
	a.timelineRepo = repo
}

// SetNotificationRepository sets the notification repository
func (a *StorageAdapter) SetNotificationRepository(repo NotificationRepository) {
	a.notificationRepo = repo
}

// SetLikeRepository sets the like repository
func (a *StorageAdapter) SetLikeRepository(repo LikeRepository) {
	a.likeRepo = repo
}


// SetSearchRepository sets the search repository
func (a *StorageAdapter) SetSearchRepository(repo SearchRepository) {
	a.searchRepo = repo
	
	// Set dependencies if the repository supports it
	if repoWithDeps, ok := repo.(interface{ SetDependencies(SearchRepositoryDeps) }); ok {
		repoWithDeps.SetDependencies(a)
	}
}

// SetSessionRepository sets the session repository
func (a *StorageAdapter) SetSessionRepository(repo SessionRepository) {
	a.sessionRepo = repo
}

// SetModerationRepository sets the moderation repository
func (a *StorageAdapter) SetModerationRepository(repo ModerationRepository) {
	a.moderationRepo = repo
}

// SetSocialRepository sets the social repository
func (a *StorageAdapter) SetSocialRepository(repo SocialRepository) {
	a.socialRepo = repo
}

// SetRelationshipRepository sets the relationship repository
func (a *StorageAdapter) SetRelationshipRepository(repo RelationshipRepository) {
	a.relationshipRepo = repo
}

// SetListRepository sets the list repository
func (a *StorageAdapter) SetListRepository(repo ListRepository) {
	a.listRepo = repo
}

// SetMediaRepository sets the media repository
func (a *StorageAdapter) SetMediaRepository(repo MediaRepository) {
	a.mediaRepo = repo
}

// SetPollRepository sets the poll repository
func (a *StorageAdapter) SetPollRepository(repo PollRepository) {
	a.pollRepo = repo
}

// SetPushSubscriptionRepository sets the push subscription repository
func (a *StorageAdapter) SetPushSubscriptionRepository(repo PushSubscriptionRepository) {
	a.pushSubscriptionRepo = repo
}

// SetInstanceRepository sets the instance repository
func (a *StorageAdapter) SetInstanceRepository(repo InstanceRepository) {
	a.instanceRepo = repo
}

// SetInstanceHealthRepository sets the instance health repository
func (a *StorageAdapter) SetInstanceHealthRepository(repo InstanceHealthRepository) {
	a.instanceHealthRepo = repo
}

// SetHashtagRepository sets the hashtag repository
func (a *StorageAdapter) SetHashtagRepository(repo HashtagRepository) {
	a.hashtagRepo = repo
}

// SetFeaturedTagRepository sets the featured tag repository
func (a *StorageAdapter) SetFeaturedTagRepository(repo FeaturedTagRepository) {
	a.featuredTagRepo = repo
}

// SetTrendingRepository sets the trending repository
func (a *StorageAdapter) SetTrendingRepository(repo TrendingRepository) {
	a.trendingRepo = repo
}

// SetScheduledStatusRepository sets the scheduled status repository
func (a *StorageAdapter) SetScheduledStatusRepository(repo ScheduledStatusRepository) {
	a.scheduledStatusRepo = repo
}

// SetMarkerRepository sets the marker repository
func (a *StorageAdapter) SetMarkerRepository(repo MarkerRepository) {
	a.markerRepo = repo
}

// SetAnnouncementRepository sets the announcement repository
func (a *StorageAdapter) SetAnnouncementRepository(repo AnnouncementRepository) {
	a.announcementRepo = repo
}

// SetEmojiRepository sets the emoji repository
func (a *StorageAdapter) SetEmojiRepository(repo EmojiRepository) {
	a.emojiRepo = repo
}

// SetDomainBlockRepository sets the domain block repository
func (a *StorageAdapter) SetDomainBlockRepository(repo DomainBlockRepository) {
	a.domainBlockRepo = repo
}

// SetRelayRepository sets the relay repository
func (a *StorageAdapter) SetRelayRepository(repo RelayRepository) {
	a.relayRepo = repo
}

// SetFederationRepository sets the federation repository
func (a *StorageAdapter) SetFederationRepository(repo FederationRepository) {
	a.federationRepo = repo
}

// SetFederationInstanceRepository sets the federation instance repository
func (a *StorageAdapter) SetFederationInstanceRepository(repo FederationInstanceRepository) {
	a.federationInstanceRepo = repo
}

// SetAccountRepository sets the account repository
func (a *StorageAdapter) SetAccountRepository(repo AccountRepository) {
	a.accountRepo = repo
}

// SetWalletRepository sets the wallet repository
func (a *StorageAdapter) SetWalletRepository(repo WalletRepository) {
	a.walletRepo = repo
}

// SetRecoveryRepository sets the recovery repository
func (a *StorageAdapter) SetRecoveryRepository(repo RecoveryRepository) {
	a.recoveryRepo = repo
}

// SetRateLimitRepository sets the rate limit repository
func (a *StorageAdapter) SetRateLimitRepository(repo RateLimitRepository) {
	a.rateLimitRepo = repo
}

// SetStreamingRepository sets the streaming repository
func (a *StorageAdapter) SetStreamingRepository(repo StreamingRepository) {
	a.streamingRepo = repo
}

// SetMediaSessionRepository sets the media session repository
func (a *StorageAdapter) SetMediaSessionRepository(repo MediaSessionRepository) {
	a.mediaSessionRepo = repo
}

// SetCommunityNoteRepository sets the community note repository
func (a *StorageAdapter) SetCommunityNoteRepository(repo CommunityNoteRepository) {
	a.communityNoteRepo = repo
}

// SetCSRFRepository sets the CSRF repository
func (a *StorageAdapter) SetCSRFRepository(repo CSRFRepository) {
	a.csrfRepo = repo
}

func (a *StorageAdapter) SetCircuitBreakerRepository(repo CircuitBreakerRepository) {
	a.circuitBreakerRepo = repo
}

// SetRouteOptimizationRepository sets the route optimization repository
func (a *StorageAdapter) SetRouteOptimizationRepository(repo RouteOptimizationRepository) {
	a.routeOptimizationRepo = repo
}

// SetRoutingMetricsRepository sets the routing metrics repository
func (a *StorageAdapter) SetRoutingMetricsRepository(repo RoutingMetricsRepository) {
	a.routingMetricsRepo = repo
}

// RepositoryAdapter is a generic adapter for repository interfaces
// It allows for adapting DynamORM repositories to existing interfaces
type RepositoryAdapter struct {
	DB        core.DB
	TableName string
}

// NewRepositoryAdapter creates a new RepositoryAdapter
func NewRepositoryAdapter(db core.DB, tableName string) *RepositoryAdapter {
	return &RepositoryAdapter{
		DB:        db,
		TableName: tableName,
	}
}

// AdapterError wraps errors from DynamORM and provides context
type AdapterError struct {
	OriginalError error
	Operation     string
	Entity        string
	ID            string
}

// Error implements the error interface
func (e *AdapterError) Error() string {
	return fmt.Sprintf("%s %s (ID: %s): %v", e.Operation, e.Entity, e.ID, e.OriginalError)
}

// Unwrap returns the original error
func (e *AdapterError) Unwrap() error {
	return e.OriginalError
}

// NewAdapterError creates a new AdapterError
func NewAdapterError(err error, operation, entity, id string) error {
	if err == nil {
		return nil
	}
	return &AdapterError{
		OriginalError: err,
		Operation:     operation,
		Entity:        entity,
		ID:            id,
	}
}

// MapRepositoryError maps DynamORM errors to storage errors with context
func MapRepositoryError(err error, operation, entity, id string) error {
	if err == nil {
		return nil
	}

	// Map common error types
	mappedErr := MapError(err)

	// Add context to the error
	return NewAdapterError(mappedErr, operation, entity, id)
}

// IsNotFoundError checks if an error is a not found error
func IsNotFoundError(err error) bool {
	return errors.Is(err, storage.ErrNotFound)
}

// IsConditionalCheckFailedError checks if an error is a conditional check failed error
func IsConditionalCheckFailedError(err error) bool {
	return errors.Is(err, ErrConditionalCheckFailed)
}

// IsThrottlingError checks if an error is a throttling error
func IsThrottlingError(err error) bool {
	return errors.Is(err, ErrThrottling)
}

// IsNotificationEnabled checks if notifications are enabled for a user and type
func (a *StorageAdapter) IsNotificationEnabled(ctx context.Context, userID, notificationType string) (bool, error) {
	// Default to enabled if preferences haven't been set
	// This matches the legacy behavior
	return true, nil
}

// IsNotificationMuted checks if notifications are muted for a specific target
func (a *StorageAdapter) IsNotificationMuted(ctx context.Context, userID, targetID string) (bool, error) {
	// Check if the target is muted by this user
	if a.socialRepo != nil {
		return a.socialRepo.IsMuted(ctx, userID, targetID)
	}
	return false, nil
}

// HasPendingFollowRequest checks if there's a pending follow request
func (a *StorageAdapter) HasPendingFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error) {
	if a.relationshipRepo != nil {
		request, err := a.relationshipRepo.GetFollowRequest(ctx, requesterID, targetID)
		if err != nil {
			return false, nil // No pending request if error
		}
		return request != nil && request.State == "pending", nil
	}
	return false, fmt.Errorf("relationship repository not initialized")
}

// Implement storage.Storage interface methods as they are migrated to DynamORM



// GetActor retrieves an actor by username
func (a *StorageAdapter) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	// Check if actor repository is set
	if a.actorRepo == nil {
		return nil, fmt.Errorf("actor repository not initialized")
	}

	// Call the DynamORM repository
	actorObject, err := a.actorRepo.GetActorByUsername(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetActor", "Actor", username)
	}

	// The repository already returns *activitypub.Actor, so no conversion needed
	return actorObject, nil
}

// CreateActor creates a new actor in the database
func (a *StorageAdapter) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	// Check if actor repository is set
	if a.actorRepo == nil {
		return fmt.Errorf("actor repository not initialized")
	}

	// Call the DynamORM repository
	err := a.actorRepo.CreateActor(ctx, actor, privateKey)
	if err != nil {
		return MapRepositoryError(err, "CreateActor", "Actor", actor.PreferredUsername)
	}

	return nil
}

// UpdateActor updates an existing actor
func (a *StorageAdapter) UpdateActor(ctx context.Context, actor *activitypub.Actor) error {
	// Check if actor repository is set
	if a.actorRepo == nil {
		return fmt.Errorf("actor repository not initialized")
	}

	// Call the DynamORM repository
	err := a.actorRepo.UpdateActor(ctx, actor)
	if err != nil {
		return MapRepositoryError(err, "UpdateActor", "Actor", actor.PreferredUsername)
	}

	return nil
}

// DeleteActor deletes an actor by username
func (a *StorageAdapter) DeleteActor(ctx context.Context, username string) error {
	// Check if actor repository is set
	if a.actorRepo == nil {
		return fmt.Errorf("actor repository not initialized")
	}

	// Call the DynamORM repository
	err := a.actorRepo.DeleteActor(ctx, username)
	if err != nil {
		return MapRepositoryError(err, "DeleteActor", "Actor", username)
	}

	return nil
}

// GetActorPrivateKey retrieves an actor's private key
func (a *StorageAdapter) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	// Check if actor repository is set
	if a.actorRepo == nil {
		return "", fmt.Errorf("actor repository not initialized")
	}

	// Call the DynamORM repository
	privateKey, err := a.actorRepo.GetActorPrivateKey(ctx, username)
	if err != nil {
		return "", MapRepositoryError(err, "GetActorPrivateKey", "Actor", username)
	}

	return privateKey, nil
}

// GetActorByNumericID retrieves an actor by its numeric ID (for Mastodon API compatibility)
func (a *StorageAdapter) GetActorByNumericID(ctx context.Context, numericID string) (*activitypub.Actor, error) {
	// Check if actor repository is set
	if a.actorRepo == nil {
		return nil, fmt.Errorf("actor repository not initialized")
	}

	// Call the DynamORM repository
	actor, err := a.actorRepo.GetActorByNumericID(ctx, numericID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetActorByNumericID", "Actor", numericID)
	}

	return actor, nil
}

// GetActorWithMetadata retrieves an actor by username from DynamoDB along with metadata
func (a *StorageAdapter) GetActorWithMetadata(ctx context.Context, username string) (*activitypub.Actor, *storage.ActorMetadata, error) {
	// Check if actor repository is set
	if a.actorRepo == nil {
		return nil, nil, fmt.Errorf("actor repository not initialized")
	}

	// Call the DynamORM repository
	actor, metadata, err := a.actorRepo.GetActorWithMetadata(ctx, username)
	if err != nil {
		return nil, nil, MapRepositoryError(err, "GetActorWithMetadata", "Actor", username)
	}

	return actor, metadata, nil
}

// UpdateActorLastStatusTime updates the last status timestamp for an actor
func (a *StorageAdapter) UpdateActorLastStatusTime(ctx context.Context, username string) error {
	// Check if actor repository is set
	if a.actorRepo == nil {
		return fmt.Errorf("actor repository not initialized")
	}

	// Call the DynamORM repository
	err := a.actorRepo.UpdateActorLastStatusTime(ctx, username)
	if err != nil {
		return MapRepositoryError(err, "UpdateActorLastStatusTime", "Actor", username)
	}

	return nil
}

// SetActorFields updates the profile fields for an actor
func (a *StorageAdapter) SetActorFields(ctx context.Context, username string, fields []storage.ActorField) error {
	// Check if actor repository is set
	if a.actorRepo == nil {
		return fmt.Errorf("actor repository not initialized")
	}

	// Call the DynamORM repository
	err := a.actorRepo.SetActorFields(ctx, username, fields)
	if err != nil {
		return MapRepositoryError(err, "SetActorFields", "Actor", username)
	}

	return nil
}

// GetSearchSuggestions returns search suggestions for autocomplete
func (a *StorageAdapter) GetSearchSuggestions(ctx context.Context, prefix string) ([]storage.SearchSuggestion, error) {
	// Check if actor repository is set
	if a.actorRepo == nil {
		return nil, fmt.Errorf("actor repository not initialized")
	}

	// Call the DynamORM repository
	suggestions, err := a.actorRepo.GetSearchSuggestions(ctx, prefix)
	if err != nil {
		return nil, MapRepositoryError(err, "GetSearchSuggestions", "SearchSuggestion", prefix)
	}

	return suggestions, nil
}

// GetObject retrieves an object by ID
func (a *StorageAdapter) GetObject(ctx context.Context, id string) (any, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return nil, fmt.Errorf("object repository not initialized")
	}

	// Call the DynamORM repository
	object, err := a.objectRepo.GetObject(ctx, id)
	if err != nil {
		return nil, MapRepositoryError(err, "GetObject", "Object", id)
	}

	return object, nil
}

// CreateObject creates a new object in the database
func (a *StorageAdapter) CreateObject(ctx context.Context, object any) error {
	// Check if object repository is set
	if a.objectRepo == nil {
		return fmt.Errorf("object repository not initialized")
	}

	// Extract ID from object for error context
	objectID := "unknown"
	if objMap, ok := object.(map[string]any); ok {
		if id, ok := objMap["id"].(string); ok {
			objectID = id
		}
	}

	// Call the DynamORM repository
	err := a.objectRepo.CreateObject(ctx, object)
	if err != nil {
		return MapRepositoryError(err, "CreateObject", "Object", objectID)
	}

	return nil
}

// DeleteObject deletes an object by ID
func (a *StorageAdapter) DeleteObject(ctx context.Context, id string) error {
	// Check if object repository is set
	if a.objectRepo == nil {
		return fmt.Errorf("object repository not initialized")
	}

	// Call the DynamORM repository
	err := a.objectRepo.DeleteObject(ctx, id)
	if err != nil {
		return MapRepositoryError(err, "DeleteObject", "Object", id)
	}

	return nil
}

// UpdateObject updates an existing object
func (a *StorageAdapter) UpdateObject(ctx context.Context, object any) error {
	// Check if object repository is set
	if a.objectRepo == nil {
		return fmt.Errorf("object repository not initialized")
	}

	// Extract ID from object for error context
	objectID := "unknown"
	if objMap, ok := object.(map[string]any); ok {
		if id, ok := objMap["id"].(string); ok {
			objectID = id
		}
	}

	// Call the DynamORM repository
	err := a.objectRepo.UpdateObject(ctx, object)
	if err != nil {
		return MapRepositoryError(err, "UpdateObject", "Object", objectID)
	}

	return nil
}

// GetObjectsByActor retrieves objects created by a specific actor
func (a *StorageAdapter) GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]any, string, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return nil, "", fmt.Errorf("object repository not initialized")
	}

	// Call the DynamORM repository
	objects, nextCursor, err := a.objectRepo.GetObjectsByActor(ctx, actorID, cursor, limit)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetObjectsByActor", "Object", actorID)
	}

	return objects, nextCursor, nil
}

// CountObjectReplies counts the number of replies to an object
func (a *StorageAdapter) CountObjectReplies(ctx context.Context, objectID string) (int, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return 0, fmt.Errorf("object repository not initialized")
	}

	// Call the DynamORM repository
	count, err := a.objectRepo.CountObjectReplies(ctx, objectID)
	if err != nil {
		return 0, MapRepositoryError(err, "CountObjectReplies", "Object", objectID)
	}

	return count, nil
}

// CreateUpdateHistory creates a new update history entry for an object
func (a *StorageAdapter) CreateUpdateHistory(ctx context.Context, history *storage.UpdateHistory) error {
	// Check if object repository is set
	if a.objectRepo == nil {
		return fmt.Errorf("object repository not initialized")
	}
	// Call the DynamORM repository
	err := a.objectRepo.CreateUpdateHistory(ctx, history)
	if err != nil {
		return MapRepositoryError(err, "CreateUpdateHistory", "UpdateHistory", history.ObjectID)
	}
	return nil
}

// GetUpdateHistory retrieves update history for an object
func (a *StorageAdapter) GetUpdateHistory(ctx context.Context, objectID string, limit int) ([]*storage.UpdateHistory, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return nil, fmt.Errorf("object repository not initialized")
	}
	// Call the DynamORM repository
	histories, err := a.objectRepo.GetUpdateHistory(ctx, objectID, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetUpdateHistory", "UpdateHistory", objectID)
	}
	return histories, nil
}

// TombstoneObject marks an object as deleted by creating a tombstone
func (a *StorageAdapter) TombstoneObject(ctx context.Context, objectID string, deletedBy string) error {
	// Check if object repository is set
	if a.objectRepo == nil {
		return fmt.Errorf("object repository not initialized")
	}

	// Call the DynamORM repository
	err := a.objectRepo.TombstoneObject(ctx, objectID, deletedBy)
	if err != nil {
		return MapRepositoryError(err, "TombstoneObject", "Object", objectID)
	}

	return nil
}

// CreateActivity creates a new activity in the database
func (a *StorageAdapter) CreateActivity(ctx context.Context, activity *activitypub.Activity) error {
	// Check if activity repository is set
	if a.activityRepo == nil {
		return fmt.Errorf("activity repository not initialized")
	}

	// Call the DynamORM repository
	err := a.activityRepo.CreateActivity(ctx, activity)
	if err != nil {
		return MapRepositoryError(err, "CreateActivity", "Activity", activity.ID)
	}

	return nil
}

// GetActivity retrieves an activity by ID
func (a *StorageAdapter) GetActivity(ctx context.Context, id string) (*activitypub.Activity, error) {
	// Check if activity repository is set
	if a.activityRepo == nil {
		return nil, fmt.Errorf("activity repository not initialized")
	}

	// Call the DynamORM repository
	activity, err := a.activityRepo.GetActivity(ctx, id)
	if err != nil {
		return nil, MapRepositoryError(err, "GetActivity", "Activity", id)
	}

	return activity, nil
}

// GetOutboxActivities retrieves outbox activities for a user
func (a *StorageAdapter) GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	// Check if activity repository is set
	if a.activityRepo == nil {
		return nil, "", fmt.Errorf("activity repository not initialized")
	}

	// Call the DynamORM repository
	activities, nextCursor, err := a.activityRepo.GetOutboxActivities(ctx, username, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetOutboxActivities", "Activity", username)
	}

	return activities, nextCursor, nil
}

// GetInboxActivities retrieves activities delivered to a user (inbox)
func (a *StorageAdapter) GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	// Check if activity repository is set
	if a.activityRepo == nil {
		return nil, "", fmt.Errorf("activity repository not initialized")
	}

	// Call the DynamORM repository
	activities, nextCursor, err := a.activityRepo.GetInboxActivities(ctx, username, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetInboxActivities", "Activity", username)
	}

	return activities, nextCursor, nil
}

// GetCollection retrieves a collection for an actor (followers, following, etc.)
func (a *StorageAdapter) GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error) {
	// Handle different collection types by delegating to appropriate repositories
	switch collectionType {
	case activitypub.FollowersCollection:
		followers, nextCursor, err := a.GetFollowers(ctx, username, limit, cursor)
		if err != nil {
			return nil, fmt.Errorf("failed to get followers: %w", err)
		}

		// Convert to collection page
		baseURL := config.Get().BaseURL()
		actorID := fmt.Sprintf("%s/users/%s", baseURL, username)
		collectionID := fmt.Sprintf("%s/%s", actorID, collectionType)

		page := &activitypub.OrderedCollectionPage{
			CollectionPage: activitypub.CollectionPage{
				Collection: activitypub.Collection{
					BaseObject: activitypub.BaseObject{
						Context: activitypub.Context,
						ID:      collectionID,
						Type:    activitypub.OrderedCollectionType,
					},
				},
				PartOf: collectionID,
			},
		}

		// Convert usernames to actor IDs
		items := make([]any, len(followers))
		for i, follower := range followers {
			items[i] = fmt.Sprintf("%s/users/%s", baseURL, follower)
		}
		page.OrderedItems = items

		// Set pagination info
		if nextCursor != "" {
			page.Next = fmt.Sprintf("%s?cursor=%s&limit=%d", collectionID, nextCursor, limit)
		}

		page.TotalItems = len(items)
		return page, nil

	case activitypub.FollowingCollection:
		following, nextCursor, err := a.GetFollowing(ctx, username, limit, cursor)
		if err != nil {
			return nil, fmt.Errorf("failed to get following: %w", err)
		}

		// Convert to collection page
		baseURL := config.Get().BaseURL()
		actorID := fmt.Sprintf("%s/users/%s", baseURL, username)
		collectionID := fmt.Sprintf("%s/%s", actorID, collectionType)

		page := &activitypub.OrderedCollectionPage{
			CollectionPage: activitypub.CollectionPage{
				Collection: activitypub.Collection{
					BaseObject: activitypub.BaseObject{
						Context: activitypub.Context,
						ID:      collectionID,
						Type:    activitypub.OrderedCollectionType,
					},
				},
				PartOf: collectionID,
			},
		}

		// Convert usernames to actor IDs
		items := make([]any, len(following))
		for i, followed := range following {
			items[i] = fmt.Sprintf("%s/users/%s", baseURL, followed)
		}
		page.OrderedItems = items

		// Set pagination info
		if nextCursor != "" {
			page.Next = fmt.Sprintf("%s?cursor=%s&limit=%d", collectionID, nextCursor, limit)
		}

		page.TotalItems = len(items)
		return page, nil

	case activitypub.LikedCollection:
		// Get the liked posts for this user
		likes, nextCursor, err := a.GetActorLikes(ctx, fmt.Sprintf("%s/users/%s", config.Get().BaseURL(), username), limit, cursor)
		if err != nil {
			return nil, fmt.Errorf("failed to get liked posts: %w", err)
		}

		// Convert to collection page
		baseURL := config.Get().BaseURL()
		actorID := fmt.Sprintf("%s/users/%s", baseURL, username)
		collectionID := fmt.Sprintf("%s/%s", actorID, collectionType)

		page := &activitypub.OrderedCollectionPage{
			CollectionPage: activitypub.CollectionPage{
				Collection: activitypub.Collection{
					BaseObject: activitypub.BaseObject{
						Context: activitypub.Context,
						ID:      collectionID,
						Type:    activitypub.OrderedCollectionType,
					},
				},
				PartOf: collectionID,
			},
		}

		// Convert likes to object IDs
		items := make([]any, len(likes))
		for i, like := range likes {
			items[i] = like.Object
		}
		page.OrderedItems = items

		// Set pagination info
		if nextCursor != "" {
			page.Next = fmt.Sprintf("%s?cursor=%s&limit=%d", collectionID, nextCursor, limit)
		}

		page.TotalItems = len(items)
		return page, nil

	case activitypub.InboxCollection:
		activities, nextCursor, err := a.GetInboxActivities(ctx, username, limit, cursor)
		if err != nil {
			return nil, fmt.Errorf("failed to get inbox activities: %w", err)
		}

		// Convert to collection page
		baseURL := config.Get().BaseURL()
		actorID := fmt.Sprintf("%s/users/%s", baseURL, username)
		collectionID := fmt.Sprintf("%s/%s", actorID, collectionType)

		page := &activitypub.OrderedCollectionPage{
			CollectionPage: activitypub.CollectionPage{
				Collection: activitypub.Collection{
					BaseObject: activitypub.BaseObject{
						Context: activitypub.Context,
						ID:      collectionID,
						Type:    activitypub.OrderedCollectionType,
					},
				},
				PartOf: collectionID,
			},
		}

		// Convert activities to interfaces
		items := make([]any, len(activities))
		for i, activity := range activities {
			items[i] = activity
		}
		page.OrderedItems = items

		// Set pagination info
		if nextCursor != "" {
			page.Next = fmt.Sprintf("%s?cursor=%s&limit=%d", collectionID, nextCursor, limit)
		}

		page.TotalItems = len(items)
		return page, nil

	case activitypub.OutboxCollection:
		activities, nextCursor, err := a.GetOutboxActivities(ctx, username, limit, cursor)
		if err != nil {
			return nil, fmt.Errorf("failed to get outbox activities: %w", err)
		}

		// Convert to collection page
		baseURL := config.Get().BaseURL()
		actorID := fmt.Sprintf("%s/users/%s", baseURL, username)
		collectionID := fmt.Sprintf("%s/%s", actorID, collectionType)

		page := &activitypub.OrderedCollectionPage{
			CollectionPage: activitypub.CollectionPage{
				Collection: activitypub.Collection{
					BaseObject: activitypub.BaseObject{
						Context: activitypub.Context,
						ID:      collectionID,
						Type:    activitypub.OrderedCollectionType,
					},
				},
				PartOf: collectionID,
			},
		}

		// Convert activities to interfaces
		items := make([]any, len(activities))
		for i, activity := range activities {
			items[i] = activity
		}
		page.OrderedItems = items

		// Set pagination info
		if nextCursor != "" {
			page.Next = fmt.Sprintf("%s?cursor=%s&limit=%d", collectionID, nextCursor, limit)
		}

		page.TotalItems = len(items)
		return page, nil

	default:
		// For unknown collection types, return empty
		baseURL := config.Get().BaseURL()
		actorID := fmt.Sprintf("%s/users/%s", baseURL, username)
		collectionID := fmt.Sprintf("%s/%s", actorID, collectionType)

		page := &activitypub.OrderedCollectionPage{
			CollectionPage: activitypub.CollectionPage{
				Collection: activitypub.Collection{
					BaseObject: activitypub.BaseObject{
						Context: activitypub.Context,
						ID:      collectionID,
						Type:    activitypub.OrderedCollectionType,
					},
				},
				PartOf: collectionID,
			},
		}

		page.OrderedItems = []any{}
		page.TotalItems = 0
		return page, nil
	}
}

// CreateUser creates a new user in the database
func (a *StorageAdapter) CreateUser(ctx context.Context, user *storage.User) error {
	// Check if user repository is set
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}

	// Call the DynamORM repository
	err := a.userRepo.CreateUser(ctx, user)
	if err != nil {
		return MapRepositoryError(err, "CreateUser", "User", user.Username)
	}

	return nil
}

// GetUser retrieves a user by username
func (a *StorageAdapter) GetUser(ctx context.Context, username string) (*storage.User, error) {
	// Check if user repository is set
	if a.userRepo == nil {
		return nil, fmt.Errorf("user repository not initialized")
	}

	// Call the DynamORM repository
	user, err := a.userRepo.GetUser(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetUser", "User", username)
	}

	return user, nil
}

// GetUserByEmail retrieves a user by email address
func (a *StorageAdapter) GetUserByEmail(ctx context.Context, email string) (*storage.User, error) {
	// Check if user repository is set
	if a.userRepo == nil {
		return nil, fmt.Errorf("user repository not initialized")
	}

	// Call the DynamORM repository
	user, err := a.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, MapRepositoryError(err, "GetUserByEmail", "User", email)
	}

	return user, nil
}

// UpdateUser updates an existing user
func (a *StorageAdapter) UpdateUser(ctx context.Context, username string, updates map[string]any) error {
	// Check if user repository is set
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}

	// Call the DynamORM repository
	err := a.userRepo.UpdateUser(ctx, username, updates)
	if err != nil {
		return MapRepositoryError(err, "UpdateUser", "User", username)
	}

	return nil
}

// DeleteUser deletes a user by username
func (a *StorageAdapter) DeleteUser(ctx context.Context, username string) error {
	// Check if user repository is set
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}

	// Call the DynamORM repository
	err := a.userRepo.DeleteUser(ctx, username)
	if err != nil {
		return MapRepositoryError(err, "DeleteUser", "User", username)
	}

	return nil
}

// OAuth Provider User Methods

// GetUserByProviderID gets a user by their OAuth provider ID
func (a *StorageAdapter) GetUserByProviderID(ctx context.Context, provider, providerID string) (*storage.User, error) {
	// Check if user repository is set
	if a.userRepo == nil {
		return nil, fmt.Errorf("user repository not initialized")
	}

	// Call the DynamORM repository
	user, err := a.userRepo.GetUserByProviderID(ctx, provider, providerID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetUserByProviderID", "User", fmt.Sprintf("%s:%s", provider, providerID))
	}

	return user, nil
}

// LinkProviderAccount links an OAuth provider account to a user
func (a *StorageAdapter) LinkProviderAccount(ctx context.Context, username, provider, providerID string) error {
	// Check if user repository is set
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}

	// Call the DynamORM repository
	err := a.userRepo.LinkProviderAccount(ctx, username, provider, providerID)
	if err != nil {
		return MapRepositoryError(err, "LinkProviderAccount", "ProviderAccount", fmt.Sprintf("%s:%s->%s", provider, providerID, username))
	}

	return nil
}

// UnlinkProviderAccount unlinks an OAuth provider account from a user
func (a *StorageAdapter) UnlinkProviderAccount(ctx context.Context, username, provider string) error {
	// Check if user repository is set
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}

	// Call the DynamORM repository
	err := a.userRepo.UnlinkProviderAccount(ctx, username, provider)
	if err != nil {
		return MapRepositoryError(err, "UnlinkProviderAccount", "ProviderAccount", fmt.Sprintf("%s->%s", provider, username))
	}

	return nil
}

// CreateFollow creates a new follow relationship
func (a *StorageAdapter) CreateFollow(ctx context.Context, followerUsername, followedUsername, followActivityID string) error {
	// Check if follow repository is set
	if a.followRepo == nil {
		return fmt.Errorf("follow repository not initialized")
	}

	// Call the DynamORM repository
	err := a.followRepo.CreateFollow(ctx, followerUsername, followedUsername, followActivityID)
	if err != nil {
		return MapRepositoryError(err, "CreateFollow", "Follow", fmt.Sprintf("%s->%s", followerUsername, followedUsername))
	}

	return nil
}

// RemoveFollow deletes a follow relationship
func (a *StorageAdapter) RemoveFollow(ctx context.Context, followerUsername, followedUsername string) error {
	// Check if follow repository is set
	if a.followRepo == nil {
		return fmt.Errorf("follow repository not initialized")
	}

	// Call the DynamORM repository
	err := a.followRepo.RemoveFollow(ctx, followerUsername, followedUsername)
	if err != nil {
		return MapRepositoryError(err, "RemoveFollow", "Follow", fmt.Sprintf("%s->%s", followerUsername, followedUsername))
	}

	return nil
}

// GetFollowers retrieves all followers for a user
func (a *StorageAdapter) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	// Check if follow repository is set
	if a.followRepo == nil {
		return nil, "", fmt.Errorf("follow repository not initialized")
	}

	// Call the DynamORM repository
	follows, nextCursor, err := a.followRepo.GetFollowers(ctx, username, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetFollowers", "Follow", username)
	}

	// Extract usernames from follow objects
	usernames := make([]string, len(follows))
	for i, follow := range follows {
		usernames[i] = follow.FollowerUsername
	}

	return usernames, nextCursor, nil
}

// GetFollowing retrieves all users that a user is following
func (a *StorageAdapter) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	// Check if follow repository is set
	if a.followRepo == nil {
		return nil, "", fmt.Errorf("follow repository not initialized")
	}

	// Call the DynamORM repository
	follows, nextCursor, err := a.followRepo.GetFollowing(ctx, username, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetFollowing", "Follow", username)
	}

	// Extract usernames from follow objects
	usernames := make([]string, len(follows))
	for i, follow := range follows {
		usernames[i] = follow.FollowedUsername
	}

	return usernames, nextCursor, nil
}

// IsFollowing checks if one user follows another
func (a *StorageAdapter) IsFollowing(ctx context.Context, followerUsername, followedUsername string) (bool, error) {
	// Check if follow repository is set
	if a.followRepo == nil {
		return false, fmt.Errorf("follow repository not initialized")
	}

	// Call the DynamORM repository
	following, err := a.followRepo.IsFollowing(ctx, followerUsername, followedUsername)
	if err != nil {
		return false, MapRepositoryError(err, "IsFollowing", "Follow", fmt.Sprintf("%s->%s", followerUsername, followedUsername))
	}

	return following, nil
}

// WriteToTimeline writes a single timeline entry
func (a *StorageAdapter) WriteToTimeline(ctx context.Context, timeline *storage.TimelineEntry) error {
	// Check if timeline repository is set
	if a.timelineRepo == nil {
		return fmt.Errorf("timeline repository not initialized")
	}

	// Convert storage.TimelineEntry to models.Timeline
	entry := &models.Timeline{
		TimelineType: timeline.TimelineType,
		TimelineID:   timeline.TimelineID,
		EntryID:      timeline.EntryID,
		PostID:       timeline.PostID,
		ActorID:      timeline.ActorID,
		ActorHandle:  timeline.ActorHandle,
		Content:      timeline.Content,
		ContentType:  timeline.ContentType,
		HasMedia:     timeline.HasMedia,
		IsReply:      timeline.IsReply,
		InReplyTo:    timeline.InReplyTo,
		IsBoost:      timeline.IsBoost,
		BoostedBy:    timeline.BoostedBy,
		Visibility:   timeline.Visibility,
		Language:     timeline.Language,
		Sensitive:    timeline.Sensitive,
		SpoilerText:  timeline.SpoilerText,
		CreatedAt:    timeline.CreatedAt,
		TimelineAt:   timeline.TimelineAt,
		ExpiresAt:    timeline.ExpiresAt,
	}

	// Call the DynamORM repository
	err := a.timelineRepo.CreateTimelineEntry(ctx, entry)
	if err != nil {
		return MapRepositoryError(err, "WriteToTimeline", "Timeline", timeline.EntryID)
	}

	return nil
}

// WriteToTimelines writes multiple timeline entries in batch
func (a *StorageAdapter) WriteToTimelines(ctx context.Context, entries []*storage.TimelineEntry) error {
	// Check if timeline repository is set
	if a.timelineRepo == nil {
		return fmt.Errorf("timeline repository not initialized")
	}

	// Convert storage.TimelineEntry slice to models.Timeline slice
	modelEntries := make([]*models.Timeline, len(entries))
	for i, timeline := range entries {
		modelEntries[i] = &models.Timeline{
			TimelineType: timeline.TimelineType,
			TimelineID:   timeline.TimelineID,
			EntryID:      timeline.EntryID,
			PostID:       timeline.PostID,
			ActorID:      timeline.ActorID,
			ActorHandle:  timeline.ActorHandle,
			Content:      timeline.Content,
			ContentType:  timeline.ContentType,
			HasMedia:     timeline.HasMedia,
			IsReply:      timeline.IsReply,
			InReplyTo:    timeline.InReplyTo,
			IsBoost:      timeline.IsBoost,
			BoostedBy:    timeline.BoostedBy,
			Visibility:   timeline.Visibility,
			Language:     timeline.Language,
			Sensitive:    timeline.Sensitive,
			SpoilerText:  timeline.SpoilerText,
			CreatedAt:    timeline.CreatedAt,
			TimelineAt:   timeline.TimelineAt,
			ExpiresAt:    timeline.ExpiresAt,
		}
	}

	// Call the DynamORM repository
	err := a.timelineRepo.CreateTimelineEntries(ctx, modelEntries)
	if err != nil {
		return MapRepositoryError(err, "WriteToTimelines", "Timeline", fmt.Sprintf("%d entries", len(entries)))
	}

	return nil
}

// GetHomeTimeline retrieves home timeline entries for a user
func (a *StorageAdapter) GetHomeTimeline(ctx context.Context, username string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	// Check if timeline repository is set
	if a.timelineRepo == nil {
		return nil, "", fmt.Errorf("timeline repository not initialized")
	}

	// Call the DynamORM repository
	modelEntries, nextCursor, err := a.timelineRepo.GetHomeTimeline(ctx, username, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetHomeTimeline", "Timeline", username)
	}

	// Convert models.Timeline slice to storage.TimelineEntry slice
	entries := make([]*storage.TimelineEntry, len(modelEntries))
	for i, model := range modelEntries {
		entries[i] = &storage.TimelineEntry{
			TimelineType: model.TimelineType,
			TimelineID:   model.TimelineID,
			EntryID:      model.EntryID,
			PostID:       model.PostID,
			ActorID:      model.ActorID,
			ActorHandle:  model.ActorHandle,
			Content:      model.Content,
			ContentType:  model.ContentType,
			HasMedia:     model.HasMedia,
			IsReply:      model.IsReply,
			InReplyTo:    model.InReplyTo,
			IsBoost:      model.IsBoost,
			BoostedBy:    model.BoostedBy,
			Visibility:   model.Visibility,
			Language:     model.Language,
			Sensitive:    model.Sensitive,
			SpoilerText:  model.SpoilerText,
			CreatedAt:    model.CreatedAt,
			TimelineAt:   model.TimelineAt,
			ExpiresAt:    model.ExpiresAt,
		}
	}

	return entries, nextCursor, nil
}

// GetPublicTimeline retrieves public timeline entries
func (a *StorageAdapter) GetPublicTimeline(ctx context.Context, local bool, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	// Check if timeline repository is set
	if a.timelineRepo == nil {
		return nil, "", fmt.Errorf("timeline repository not initialized")
	}

	// Call the DynamORM repository
	modelEntries, nextCursor, err := a.timelineRepo.GetPublicTimeline(ctx, local, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetPublicTimeline", "Timeline", fmt.Sprintf("local=%v", local))
	}

	// Convert models.Timeline slice to storage.TimelineEntry slice
	entries := make([]*storage.TimelineEntry, len(modelEntries))
	for i, model := range modelEntries {
		entries[i] = &storage.TimelineEntry{
			TimelineType: model.TimelineType,
			TimelineID:   model.TimelineID,
			EntryID:      model.EntryID,
			PostID:       model.PostID,
			ActorID:      model.ActorID,
			ActorHandle:  model.ActorHandle,
			Content:      model.Content,
			ContentType:  model.ContentType,
			HasMedia:     model.HasMedia,
			IsReply:      model.IsReply,
			InReplyTo:    model.InReplyTo,
			IsBoost:      model.IsBoost,
			BoostedBy:    model.BoostedBy,
			Visibility:   model.Visibility,
			Language:     model.Language,
			Sensitive:    model.Sensitive,
			SpoilerText:  model.SpoilerText,
			CreatedAt:    model.CreatedAt,
			TimelineAt:   model.TimelineAt,
			ExpiresAt:    model.ExpiresAt,
		}
	}

	return entries, nextCursor, nil
}

// DeleteFromTimeline deletes a timeline entry
func (a *StorageAdapter) DeleteFromTimeline(ctx context.Context, timelineType, timelineID, entryID string) error {
	// Check if timeline repository is set
	if a.timelineRepo == nil {
		return fmt.Errorf("timeline repository not initialized")
	}

	// For deletion, we need to extract the timestamp from the entryID
	// The entryID format is typically "{timestamp}_{postID}"
	// Since we don't have the exact timestamp, we'll need to parse it from the entryID
	// However, the current repository requires a time.Time parameter
	// This is a limitation we need to handle

	// For now, we'll use a zero time and let the repository handle it
	// In a real implementation, you might want to store the timestamp in the entryID
	// or query for the entry first to get its timestamp
	zeroTime := time.Time{}

	// Call the DynamORM repository
	err := a.timelineRepo.DeleteTimelineEntry(ctx, timelineType, timelineID, entryID, zeroTime)
	if err != nil {
		return MapRepositoryError(err, "DeleteFromTimeline", "Timeline", entryID)
	}

	return nil
}

// Notification-related methods

// CreateNotification creates a new notification in the database
func (a *StorageAdapter) CreateNotification(ctx context.Context, notification *storage.Notification) error {
	// Check if notification repository is set
	if a.notificationRepo == nil {
		return fmt.Errorf("notification repository not initialized")
	}

	// Convert storage.Notification to models.Notification
	modelNotification := &models.Notification{
		ID:        notification.ID,
		UserID:    notification.Username,
		Type:      notification.Type,
		ActorID:   notification.AccountID,
		TargetID:  notification.StatusID,
		IsRead:    notification.Read,
		CreatedAt: notification.CreatedAt,
	}

	// If StatusID is not empty, set TargetType to "status"
	if notification.StatusID != "" {
		modelNotification.TargetType = "status"
	}

	// Call the DynamORM repository
	err := a.notificationRepo.CreateNotification(ctx, modelNotification)
	if err != nil {
		return MapRepositoryError(err, "CreateNotification", "Notification", notification.ID)
	}

	return nil
}

// GetNotification retrieves a notification by ID
func (a *StorageAdapter) GetNotification(ctx context.Context, id string) (*storage.Notification, error) {
	// Check if notification repository is set
	if a.notificationRepo == nil {
		return nil, fmt.Errorf("notification repository not initialized")
	}

	// Call the DynamORM repository
	modelNotification, err := a.notificationRepo.GetNotification(ctx, id)
	if err != nil {
		return nil, MapRepositoryError(err, "GetNotification", "Notification", id)
	}

	// Convert models.Notification to storage.Notification
	storageNotification := &storage.Notification{
		ID:        modelNotification.ID,
		Type:      modelNotification.Type,
		Username:  modelNotification.UserID,
		AccountID: modelNotification.ActorID,
		StatusID:  modelNotification.TargetID,
		Read:      modelNotification.IsRead,
		CreatedAt: modelNotification.CreatedAt,
	}

	return storageNotification, nil
}

// GetNotifications retrieves notifications for a user with pagination
func (a *StorageAdapter) GetNotifications(ctx context.Context, username string, limit int, cursor string) ([]*storage.Notification, string, error) {
	// Check if notification repository is set
	if a.notificationRepo == nil {
		return nil, "", fmt.Errorf("notification repository not initialized")
	}

	// Call the DynamORM repository
	modelNotifications, nextCursor, err := a.notificationRepo.GetNotificationsByUser(ctx, username, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetNotifications", "Notification", username)
	}

	// Convert models.Notification slice to storage.Notification slice
	storageNotifications := make([]*storage.Notification, len(modelNotifications))
	for i, model := range modelNotifications {
		storageNotifications[i] = &storage.Notification{
			ID:        model.ID,
			Type:      model.Type,
			Username:  model.UserID,
			AccountID: model.ActorID,
			StatusID:  model.TargetID,
			Read:      model.IsRead,
			CreatedAt: model.CreatedAt,
		}
	}

	return storageNotifications, nextCursor, nil
}

// MarkNotificationAsRead marks a notification as read
func (a *StorageAdapter) MarkNotificationAsRead(ctx context.Context, id string) error {
	// Check if notification repository is set
	if a.notificationRepo == nil {
		return fmt.Errorf("notification repository not initialized")
	}

	// Call the DynamORM repository
	err := a.notificationRepo.MarkNotificationAsRead(ctx, id)
	if err != nil {
		return MapRepositoryError(err, "MarkNotificationAsRead", "Notification", id)
	}

	return nil
}

// DeleteNotification deletes a notification by ID
func (a *StorageAdapter) DeleteNotification(ctx context.Context, id string) error {
	// Check if notification repository is set
	if a.notificationRepo == nil {
		return fmt.Errorf("notification repository not initialized")
	}

	// Call the DynamORM repository
	err := a.notificationRepo.DeleteNotification(ctx, id)
	if err != nil {
		return MapRepositoryError(err, "DeleteNotification", "Notification", id)
	}

	return nil
}

// GetNotificationsFiltered retrieves notifications with filtering options
func (a *StorageAdapter) GetNotificationsFiltered(ctx context.Context, username string, filter *storage.NotificationFilter) ([]*storage.Notification, string, error) {
	// Check if notification repository is set
	if a.notificationRepo == nil {
		return nil, "", fmt.Errorf("notification repository not initialized")
	}

	// Call the DynamORM repository
	notifications, cursor, err := a.notificationRepo.GetNotificationsFiltered(ctx, username, filter)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetNotificationsFiltered", "Notification", username)
	}

	return notifications, cursor, nil
}

// MarkAllNotificationsAsRead marks all notifications as read for a user
func (a *StorageAdapter) MarkAllNotificationsAsRead(ctx context.Context, username string) error {
	// Check if notification repository is set
	if a.notificationRepo == nil {
		return fmt.Errorf("notification repository not initialized")
	}

	// Call the DynamORM repository
	err := a.notificationRepo.MarkAllNotificationsAsRead(ctx, username)
	if err != nil {
		return MapRepositoryError(err, "MarkAllNotificationsAsRead", "Notification", username)
	}

	return nil
}

// GetNotificationsAdvanced retrieves notifications with advanced filtering
func (a *StorageAdapter) GetNotificationsAdvanced(ctx context.Context, userID string, excludeTypes []string, maxID, sinceID, minID *string, limit int, includeFiltered bool) ([]*storage.Notification, error) {
	// Check if notification repository is set
	if a.notificationRepo == nil {
		return nil, fmt.Errorf("notification repository not initialized")
	}

	// Call the DynamORM repository
	notifications, err := a.notificationRepo.GetNotificationsAdvanced(ctx, userID, excludeTypes, maxID, sinceID, minID, limit, includeFiltered)
	if err != nil {
		return nil, MapRepositoryError(err, "GetNotificationsAdvanced", "Notification", userID)
	}

	return notifications, nil
}

// GetUnreadNotificationCount returns the count of unread notifications
func (a *StorageAdapter) GetUnreadNotificationCount(ctx context.Context, userID string) (int64, error) {
	// Check if notification repository is set
	if a.notificationRepo == nil {
		return 0, fmt.Errorf("notification repository not initialized")
	}

	// Call the DynamORM repository
	count, err := a.notificationRepo.GetUnreadNotificationCount(ctx, userID)
	if err != nil {
		return 0, MapRepositoryError(err, "GetUnreadNotificationCount", "Notification", userID)
	}

	return count, nil
}

// Like-related methods

// CreateLike creates a new like in the database
func (a *StorageAdapter) CreateLike(ctx context.Context, like *storage.Like) error {
	// Check if like repository is set
	if a.likeRepo == nil {
		return fmt.Errorf("like repository not initialized")
	}

	// Call the DynamORM repository which creates the like with proper keys
	modelLike, err := a.likeRepo.CreateLike(ctx, like.Actor, like.Object)
	if err != nil {
		return MapRepositoryError(err, "CreateLike", "Like", fmt.Sprintf("%s->%s", like.Actor, like.Object))
	}

	// Update the storage like with the generated ID and timestamps
	like.ID = modelLike.ID
	like.Published = modelLike.Published
	like.CreatedAt = modelLike.CreatedAt

	return nil
}

// GetLike retrieves a like by actor and object
func (a *StorageAdapter) GetLike(ctx context.Context, actor, object string) (*storage.Like, error) {
	// Check if like repository is set
	if a.likeRepo == nil {
		return nil, fmt.Errorf("like repository not initialized")
	}

	// Call the DynamORM repository
	modelLike, err := a.likeRepo.GetLike(ctx, actor, object)
	if err != nil {
		return nil, MapRepositoryError(err, "GetLike", "Like", fmt.Sprintf("%s->%s", actor, object))
	}

	// Convert models.Like to storage.Like
	storageLike := &storage.Like{
		Actor:     modelLike.Actor,
		Object:    modelLike.Object,
		ID:        modelLike.ID,
		Published: modelLike.Published,
		CreatedAt: modelLike.CreatedAt,
	}

	return storageLike, nil
}

// DeleteLike deletes a like by actor and object
func (a *StorageAdapter) DeleteLike(ctx context.Context, actor, object string) error {
	// Check if like repository is set
	if a.likeRepo == nil {
		return fmt.Errorf("like repository not initialized")
	}

	// Call the DynamORM repository
	err := a.likeRepo.DeleteLike(ctx, actor, object)
	if err != nil {
		return MapRepositoryError(err, "DeleteLike", "Like", fmt.Sprintf("%s->%s", actor, object))
	}

	return nil
}

// GetObjectLikes retrieves all likes for an object with pagination
func (a *StorageAdapter) GetObjectLikes(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Like, string, error) {
	// Check if like repository is set
	if a.likeRepo == nil {
		return nil, "", fmt.Errorf("like repository not initialized")
	}

	// Call the DynamORM repository
	modelLikes, nextCursor, err := a.likeRepo.GetObjectLikes(ctx, objectID, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetObjectLikes", "Like", objectID)
	}

	// Convert models.Like slice to storage.Like slice
	storageLikes := make([]*storage.Like, len(modelLikes))
	for i, model := range modelLikes {
		storageLikes[i] = &storage.Like{
			Actor:     model.Actor,
			Object:    model.Object,
			ID:        model.ID,
			Published: model.Published,
			CreatedAt: model.CreatedAt,
		}
	}

	return storageLikes, nextCursor, nil
}

// CountObjectLikes counts the number of likes for an object
func (a *StorageAdapter) CountObjectLikes(ctx context.Context, objectID string) (int, error) {
	// Check if like repository is set
	if a.likeRepo == nil {
		return 0, fmt.Errorf("like repository not initialized")
	}

	// For now, we'll get all likes and count them
	// This could be optimized with a dedicated count method in the repository
	likes, _, err := a.likeRepo.GetObjectLikes(ctx, objectID, 1000, "")
	if err != nil {
		return 0, MapRepositoryError(err, "CountObjectLikes", "Like", objectID)
	}

	return len(likes), nil
}

// GetActorLikes retrieves all likes by an actor with pagination
func (a *StorageAdapter) GetActorLikes(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Like, string, error) {
	// Check if like repository is set
	if a.likeRepo == nil {
		return nil, "", fmt.Errorf("like repository not initialized")
	}

	// Call the DynamORM repository
	modelLikes, nextCursor, err := a.likeRepo.GetActorLikes(ctx, actorID, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetActorLikes", "Like", actorID)
	}

	// Convert models.Like slice to storage.Like slice
	storageLikes := make([]*storage.Like, len(modelLikes))
	for i, model := range modelLikes {
		storageLikes[i] = &storage.Like{
			Actor:     model.Actor,
			Object:    model.Object,
			ID:        model.ID,
			Published: model.Published,
			CreatedAt: model.CreatedAt,
		}
	}

	return storageLikes, nextCursor, nil
}

// OAuth-related methods

// CreateAuthorizationCode creates a new OAuth authorization code
func (a *StorageAdapter) CreateAuthorizationCode(ctx context.Context, code *storage.AuthorizationCode) error {
	// Check if OAuth repository is set
	if a.accountRepo == nil {
		return fmt.Errorf("Account repository not initialized")
	}

	// Call the DynamORM repository
	err := a.accountRepo.CreateAuthorizationCode(ctx, code)
	if err != nil {
		return MapRepositoryError(err, "CreateAuthorizationCode", "AuthorizationCode", code.Code)
	}

	return nil
}

// GetAuthorizationCode retrieves an OAuth authorization code
func (a *StorageAdapter) GetAuthorizationCode(ctx context.Context, code string) (*storage.AuthorizationCode, error) {
	// Check if OAuth repository is set
	if a.accountRepo == nil {
		return nil, fmt.Errorf("Account repository not initialized")
	}

	// Call the DynamORM repository
	authCode, err := a.accountRepo.GetAuthorizationCode(ctx, code)
	if err != nil {
		return nil, MapRepositoryError(err, "GetAuthorizationCode", "AuthorizationCode", code)
	}

	return authCode, nil
}

// DeleteAuthorizationCode deletes an OAuth authorization code
func (a *StorageAdapter) DeleteAuthorizationCode(ctx context.Context, code string) error {
	// Check if OAuth repository is set
	if a.accountRepo == nil {
		return fmt.Errorf("Account repository not initialized")
	}

	// Call the DynamORM repository
	err := a.accountRepo.DeleteAuthorizationCode(ctx, code)
	if err != nil {
		return MapRepositoryError(err, "DeleteAuthorizationCode", "AuthorizationCode", code)
	}

	return nil
}

// CreateRefreshToken creates a new OAuth refresh token
func (a *StorageAdapter) CreateRefreshToken(ctx context.Context, token *storage.RefreshToken) error {
	// Check if OAuth repository is set
	if a.accountRepo == nil {
		return fmt.Errorf("Account repository not initialized")
	}

	// Call the DynamORM repository
	err := a.accountRepo.CreateRefreshToken(ctx, token)
	if err != nil {
		return MapRepositoryError(err, "CreateRefreshToken", "RefreshToken", token.Token)
	}

	return nil
}

// GetRefreshToken retrieves an OAuth refresh token
func (a *StorageAdapter) GetRefreshToken(ctx context.Context, token string) (*storage.RefreshToken, error) {
	// Check if OAuth repository is set
	if a.accountRepo == nil {
		return nil, fmt.Errorf("Account repository not initialized")
	}

	// Call the DynamORM repository
	refreshToken, err := a.accountRepo.GetRefreshToken(ctx, token)
	if err != nil {
		return nil, MapRepositoryError(err, "GetRefreshToken", "RefreshToken", token)
	}

	return refreshToken, nil
}

// DeleteRefreshToken deletes an OAuth refresh token
func (a *StorageAdapter) DeleteRefreshToken(ctx context.Context, token string) error {
	// Check if OAuth repository is set
	if a.accountRepo == nil {
		return fmt.Errorf("Account repository not initialized")
	}

	// Call the DynamORM repository
	err := a.accountRepo.DeleteRefreshToken(ctx, token)
	if err != nil {
		return MapRepositoryError(err, "DeleteRefreshToken", "RefreshToken", token)
	}

	return nil
}

// CreateOAuthClient creates a new OAuth client
func (a *StorageAdapter) CreateOAuthClient(ctx context.Context, client *storage.OAuthClient) error {
	// Check if OAuth repository is set
	if a.accountRepo == nil {
		return fmt.Errorf("Account repository not initialized")
	}

	// Call the DynamORM repository
	err := a.accountRepo.CreateOAuthClient(ctx, client)
	if err != nil {
		return MapRepositoryError(err, "CreateOAuthClient", "OAuthClient", client.ClientID)
	}

	return nil
}

// GetOAuthClient retrieves an OAuth client by client ID
func (a *StorageAdapter) GetOAuthClient(ctx context.Context, clientID string) (*storage.OAuthClient, error) {
	// Check if OAuth repository is set
	if a.accountRepo == nil {
		return nil, fmt.Errorf("Account repository not initialized")
	}

	// Call the DynamORM repository
	client, err := a.accountRepo.GetOAuthClient(ctx, clientID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetOAuthClient", "OAuthClient", clientID)
	}

	return client, nil
}

// DeleteOAuthClient deletes an OAuth client
func (a *StorageAdapter) DeleteOAuthClient(ctx context.Context, clientID string) error {
	// Check if OAuth repository is set
	if a.accountRepo == nil {
		return fmt.Errorf("Account repository not initialized")
	}

	// Call the DynamORM repository
	err := a.accountRepo.DeleteOAuthClient(ctx, clientID)
	if err != nil {
		return MapRepositoryError(err, "DeleteOAuthClient", "OAuthClient", clientID)
	}

	return nil
}

// StoreOAuthState stores OAuth state for CSRF protection
func (a *StorageAdapter) StoreOAuthState(ctx context.Context, state string, data *storage.OAuthState) error {
	// Check if OAuth repository is set
	if a.accountRepo == nil {
		return fmt.Errorf("Account repository not initialized")
	}

	// Call the DynamORM repository
	err := a.accountRepo.StoreOAuthState(ctx, state, data)
	if err != nil {
		return MapRepositoryError(err, "StoreOAuthState", "OAuthState", state)
	}

	return nil
}

// GetOAuthState retrieves OAuth state
func (a *StorageAdapter) GetOAuthState(ctx context.Context, state string) (*storage.OAuthState, error) {
	// Check if OAuth repository is set
	if a.accountRepo == nil {
		return nil, fmt.Errorf("Account repository not initialized")
	}

	// Call the DynamORM repository
	result, err := a.accountRepo.GetOAuthState(ctx, state)
	if err != nil {
		return nil, MapRepositoryError(err, "GetOAuthState", "OAuthState", state)
	}

	return result, nil
}

// SaveOAuthState saves OAuth state (alternate method with different signature)
func (a *StorageAdapter) SaveOAuthState(ctx context.Context, state *storage.OAuthState) error {
	if a.accountRepo == nil {
		return fmt.Errorf("Account repository not initialized")
	}
	
	// Call StoreOAuthState with the state key from the object
	err := a.accountRepo.StoreOAuthState(ctx, state.State, state)
	if err != nil {
		return MapRepositoryError(err, "SaveOAuthState", "OAuthState", state.State)
	}
	
	return nil
}

// DeleteOAuthState deletes OAuth state
func (a *StorageAdapter) DeleteOAuthState(ctx context.Context, state string) error {
	// Check if OAuth repository is set
	if a.accountRepo == nil {
		return fmt.Errorf("Account repository not initialized")
	}

	// Call the DynamORM repository
	err := a.accountRepo.DeleteOAuthState(ctx, state)
	if err != nil {
		return MapRepositoryError(err, "DeleteOAuthState", "OAuthState", state)
	}

	return nil
}

// UpdateOAuthClient updates an existing OAuth client
func (a *StorageAdapter) UpdateOAuthClient(ctx context.Context, clientID string, updates map[string]any) error {
	// Check if OAuth repository is set
	if a.accountRepo == nil {
		return fmt.Errorf("Account repository not initialized")
	}

	// Call the DynamORM repository
	err := a.accountRepo.UpdateOAuthClient(ctx, clientID, updates)
	if err != nil {
		return MapRepositoryError(err, "UpdateOAuthClient", "OAuthClient", clientID)
	}

	return nil
}

// ListOAuthClients lists OAuth clients with pagination
func (a *StorageAdapter) ListOAuthClients(ctx context.Context, limit int32, cursor string) ([]*storage.OAuthClient, string, error) {
	// Check if OAuth repository is set
	if a.accountRepo == nil {
		return nil, "", fmt.Errorf("Account repository not initialized")
	}

	// Call the DynamORM repository
	clients, nextCursor, err := a.accountRepo.ListOAuthClients(ctx, int(limit), cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "ListOAuthClients", "OAuthClient", "")
	}

	return clients, nextCursor, nil
}

// GetOAuthApp retrieves an OAuth app by client ID
func (a *StorageAdapter) GetOAuthApp(ctx context.Context, clientID string) (*storage.OAuthApp, error) {
	// Check if OAuth repository is set
	if a.accountRepo == nil {
		return nil, fmt.Errorf("Account repository not initialized")
	}

	// Call the DynamORM repository
	app, err := a.accountRepo.GetOAuthApp(ctx, clientID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetOAuthApp", "OAuthApp", clientID)
	}

	return app, nil
}

// SaveUserAppConsent saves user consent for an OAuth app
func (a *StorageAdapter) SaveUserAppConsent(ctx context.Context, consent *storage.UserAppConsent) error {
	// Check if OAuth repository is set
	if a.accountRepo == nil {
		return fmt.Errorf("Account repository not initialized")
	}

	// Call the DynamORM repository
	err := a.accountRepo.SaveUserAppConsent(ctx, consent)
	if err != nil {
		return MapRepositoryError(err, "SaveUserAppConsent", "UserAppConsent", consent.UserID+":"+consent.AppID)
	}

	return nil
}

// GetUserAppConsent retrieves user consent for an OAuth app
func (a *StorageAdapter) GetUserAppConsent(ctx context.Context, userID, appID string) (*storage.UserAppConsent, error) {
	// Check if OAuth repository is set
	if a.accountRepo == nil {
		return nil, fmt.Errorf("Account repository not initialized")
	}

	// Call the DynamORM repository
	consent, err := a.accountRepo.GetUserAppConsent(ctx, userID, appID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetUserAppConsent", "UserAppConsent", userID+":"+appID)
	}

	return consent, nil
}

// Search-related methods

// SearchAccounts searches for accounts matching the given query
func (a *StorageAdapter) SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error) {
	// Check if search repository is set
	if a.searchRepo == nil {
		return nil, fmt.Errorf("search repository not initialized")
	}

	// Call the DynamORM repository
	actors, err := a.searchRepo.SearchAccounts(ctx, query, limit, followingOnly, offset)
	if err != nil {
		return nil, MapRepositoryError(err, "SearchAccounts", "Actor", query)
	}

	return actors, nil
}

// SearchStatuses searches for statuses matching the given query
func (a *StorageAdapter) SearchStatuses(ctx context.Context, query string, limit int) ([]*storage.StatusSearchResult, error) {
	// Check if search repository is set
	if a.searchRepo == nil {
		return nil, fmt.Errorf("search repository not initialized")
	}

	// Call the DynamORM repository
	results, err := a.searchRepo.SearchStatuses(ctx, query, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "SearchStatuses", "Status", query)
	}

	return results, nil
}

// SearchStatusesWithOptions searches for statuses with advanced options
func (a *StorageAdapter) SearchStatusesWithOptions(ctx context.Context, query string, options storage.StatusSearchOptions) ([]*storage.StatusSearchResult, error) {
	// Check if search repository is set
	if a.searchRepo == nil {
		return nil, fmt.Errorf("search repository not initialized")
	}

	// Call the DynamORM repository
	results, err := a.searchRepo.SearchStatusesWithOptions(ctx, query, options)
	if err != nil {
		return nil, MapRepositoryError(err, "SearchStatusesWithOptions", "Status", query)
	}

	return results, nil
}

// SearchAll performs a comprehensive search across accounts, statuses, and hashtags
func (a *StorageAdapter) SearchAll(ctx context.Context, query string, limit int, accountID string) (*storage.SearchResults, error) {
	// Check if search repository is set
	if a.searchRepo == nil {
		return nil, fmt.Errorf("search repository not initialized")
	}

	// Call the DynamORM repository
	results, err := a.searchRepo.SearchAll(ctx, query, limit, accountID)
	if err != nil {
		return nil, MapRepositoryError(err, "SearchAll", "All", query)
	}

	return results, nil
}

// SearchAccountsAdvanced searches for accounts with advanced filtering
func (a *StorageAdapter) SearchAccountsAdvanced(ctx context.Context, query string, resolve bool, limit int, offset int, following bool, accountID string) ([]*activitypub.Actor, error) {
	// Check if search repository is set
	if a.searchRepo == nil {
		return nil, fmt.Errorf("search repository not initialized")
	}

	// Call the DynamORM repository
	actors, err := a.searchRepo.SearchAccountsAdvanced(ctx, query, resolve, limit, offset, following, accountID)
	if err != nil {
		return nil, MapRepositoryError(err, "SearchAccountsAdvanced", "Actor", query)
	}

	return actors, nil
}

// SearchStatusesAdvanced searches for statuses with advanced filtering
func (a *StorageAdapter) SearchStatusesAdvanced(ctx context.Context, query string, limit int, maxID, minID *string, accountID string) ([]*storage.StatusSearchResult, error) {
	// Check if search repository is set
	if a.searchRepo == nil {
		return nil, fmt.Errorf("search repository not initialized")
	}

	// Call the DynamORM repository
	results, err := a.searchRepo.SearchStatusesAdvanced(ctx, query, limit, maxID, minID, accountID)
	if err != nil {
		return nil, MapRepositoryError(err, "SearchStatusesAdvanced", "Status", query)
	}

	return results, nil
}

// SearchHashtagsAdvanced searches for hashtags with advanced filtering
func (a *StorageAdapter) SearchHashtagsAdvanced(ctx context.Context, query string, limit int, accountID string) ([]*storage.HashtagSearchResult, error) {
	// Check if search repository is set
	if a.searchRepo == nil {
		return nil, fmt.Errorf("search repository not initialized")
	}

	// Call the DynamORM repository
	results, err := a.searchRepo.SearchHashtagsAdvanced(ctx, query, limit, accountID)
	if err != nil {
		return nil, MapRepositoryError(err, "SearchHashtagsAdvanced", "Hashtag", query)
	}

	return results, nil
}

// SearchHashtags searches for hashtags matching the given query
func (a *StorageAdapter) SearchHashtags(ctx context.Context, query string, limit int) ([]*storage.Hashtag, error) {
	// Check if search repository is set
	if a.searchRepo == nil {
		return nil, fmt.Errorf("search repository not initialized")
	}

	// Call the DynamORM repository
	hashtags, err := a.searchRepo.SearchHashtags(ctx, query, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "SearchHashtags", "Hashtag", query)
	}

	return hashtags, nil
}

// IndexHashtag indexes a hashtag when used in a status
func (a *StorageAdapter) IndexHashtag(ctx context.Context, hashtag string, statusID string, authorID string, visibility string) error {
	// Check if hashtag repository is set
	if a.hashtagRepo == nil {
		return fmt.Errorf("hashtag repository not initialized")
	}

	// Call the DynamORM repository
	err := a.hashtagRepo.IndexHashtag(ctx, hashtag, statusID, authorID, visibility)
	if err != nil {
		return MapRepositoryError(err, "IndexHashtag", "Hashtag", hashtag)
	}

	return nil
}

// GetHashtagInfo retrieves information about a specific hashtag
func (a *StorageAdapter) GetHashtagInfo(ctx context.Context, hashtag string) (*storage.Hashtag, error) {
	// Check if hashtag repository is set
	if a.hashtagRepo == nil {
		return nil, fmt.Errorf("hashtag repository not initialized")
	}

	// Call the DynamORM repository
	hashtagInfo, err := a.hashtagRepo.GetHashtagInfo(ctx, hashtag)
	if err != nil {
		return nil, MapRepositoryError(err, "GetHashtagInfo", "Hashtag", hashtag)
	}

	return hashtagInfo, nil
}

// GetHashtagUsageHistory retrieves recent usage history for a hashtag
func (a *StorageAdapter) GetHashtagUsageHistory(ctx context.Context, hashtag string, days int) ([]int64, error) {
	// Check if hashtag repository is set
	if a.hashtagRepo == nil {
		return nil, fmt.Errorf("hashtag repository not initialized")
	}

	// Call the DynamORM repository
	history, err := a.hashtagRepo.GetHashtagUsageHistory(ctx, hashtag, days)
	if err != nil {
		return nil, MapRepositoryError(err, "GetHashtagUsageHistory", "Hashtag", hashtag)
	}

	return history, nil
}

// GetHashtagActivity retrieves activities for a hashtag since a specific time
func (a *StorageAdapter) GetHashtagActivity(ctx context.Context, hashtag string, since time.Time) ([]*storage.Activity, error) {
	// Check if hashtag repository is set
	if a.hashtagRepo == nil {
		return nil, fmt.Errorf("hashtag repository not initialized")
	}

	// Call the DynamORM repository
	activities, err := a.hashtagRepo.GetHashtagActivity(ctx, hashtag, since)
	if err != nil {
		return nil, MapRepositoryError(err, "GetHashtagActivity", "Hashtag", hashtag)
	}

	return activities, nil
}

// GetHashtagStats retrieves hashtag statistics
func (a *StorageAdapter) GetHashtagStats(ctx context.Context, hashtag string) (any, error) {
	// Check if hashtag repository is set
	if a.hashtagRepo == nil {
		return nil, fmt.Errorf("hashtag repository not initialized")
	}

	// Call the DynamORM repository
	stats, err := a.hashtagRepo.GetHashtagStats(ctx, hashtag)
	if err != nil {
		return nil, MapRepositoryError(err, "GetHashtagStats", "Hashtag", hashtag)
	}

	return stats, nil
}

// GetHashtagTimelineAdvanced retrieves hashtag timeline with advanced filtering
func (a *StorageAdapter) GetHashtagTimelineAdvanced(ctx context.Context, hashtag string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error) {
	// Check if hashtag repository is set
	if a.hashtagRepo == nil {
		return nil, fmt.Errorf("hashtag repository not initialized")
	}

	// Call the DynamORM repository
	results, err := a.hashtagRepo.GetHashtagTimelineAdvanced(ctx, hashtag, maxID, limit, userID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetHashtagTimelineAdvanced", "Hashtag", hashtag)
	}

	return results, nil
}

// GetMultiHashtagTimeline retrieves timeline for multiple hashtags
func (a *StorageAdapter) GetMultiHashtagTimeline(ctx context.Context, hashtags []string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error) {
	// Check if hashtag repository is set
	if a.hashtagRepo == nil {
		return nil, fmt.Errorf("hashtag repository not initialized")
	}

	// Call the DynamORM repository
	results, err := a.hashtagRepo.GetMultiHashtagTimeline(ctx, hashtags, maxID, limit, userID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetMultiHashtagTimeline", "Hashtag", fmt.Sprintf("%v", hashtags))
	}

	return results, nil
}

// GetSuggestedHashtags gets suggested hashtags for a user
func (a *StorageAdapter) GetSuggestedHashtags(ctx context.Context, userID string, limit int) ([]*storage.HashtagSearchResult, error) {
	// Check if hashtag repository is set
	if a.hashtagRepo == nil {
		return nil, fmt.Errorf("hashtag repository not initialized")
	}

	// Call the DynamORM repository
	results, err := a.hashtagRepo.GetSuggestedHashtags(ctx, userID, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetSuggestedHashtags", "Hashtag", userID)
	}

	return results, nil
}

// Session-related methods

// CreateSession creates a new session in the database
func (a *StorageAdapter) CreateSession(ctx context.Context, session *storage.Session) error {
	// Check if session repository is set
	if a.sessionRepo == nil {
		return fmt.Errorf("session repository not initialized")
	}

	// Call the DynamORM repository
	err := a.sessionRepo.CreateSession(ctx, session)
	if err != nil {
		return MapRepositoryError(err, "CreateSession", "Session", session.SessionID)
	}

	return nil
}

// GetSession retrieves a session by ID
func (a *StorageAdapter) GetSession(ctx context.Context, sessionID string) (*storage.Session, error) {
	// Check if session repository is set
	if a.sessionRepo == nil {
		return nil, fmt.Errorf("session repository not initialized")
	}

	// Call the DynamORM repository
	session, err := a.sessionRepo.GetSession(ctx, sessionID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetSession", "Session", sessionID)
	}

	return session, nil
}

// GetSessionByRefreshToken retrieves a session by refresh token
func (a *StorageAdapter) GetSessionByRefreshToken(ctx context.Context, refreshToken string) (*storage.Session, error) {
	// Check if session repository is set
	if a.sessionRepo == nil {
		return nil, fmt.Errorf("session repository not initialized")
	}

	// Call the DynamORM repository
	session, err := a.sessionRepo.GetSessionByRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, MapRepositoryError(err, "GetSessionByRefreshToken", "Session", refreshToken)
	}

	return session, nil
}

// UpdateSession updates an existing session
func (a *StorageAdapter) UpdateSession(ctx context.Context, session *storage.Session) error {
	// Check if session repository is set
	if a.sessionRepo == nil {
		return fmt.Errorf("session repository not initialized")
	}

	// Call the DynamORM repository
	err := a.sessionRepo.UpdateSession(ctx, session)
	if err != nil {
		return MapRepositoryError(err, "UpdateSession", "Session", session.SessionID)
	}

	return nil
}

// DeleteSession deletes a session by ID
func (a *StorageAdapter) DeleteSession(ctx context.Context, sessionID string) error {
	// Check if session repository is set
	if a.sessionRepo == nil {
		return fmt.Errorf("session repository not initialized")
	}

	// Call the DynamORM repository
	err := a.sessionRepo.DeleteSession(ctx, sessionID)
	if err != nil {
		return MapRepositoryError(err, "DeleteSession", "Session", sessionID)
	}

	return nil
}

// GetUserSessions retrieves all sessions for a user
func (a *StorageAdapter) GetUserSessions(ctx context.Context, username string) ([]*storage.Session, error) {
	// Check if session repository is set
	if a.sessionRepo == nil {
		return nil, fmt.Errorf("session repository not initialized")
	}

	// Call the DynamORM repository
	sessions, err := a.sessionRepo.GetUserSessions(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetUserSessions", "Session", username)
	}

	return sessions, nil
}

// Device-related methods

// CreateDevice creates a new device in the database
func (a *StorageAdapter) CreateDevice(ctx context.Context, device *storage.Device) error {
	// Check if session repository is set
	if a.sessionRepo == nil {
		return fmt.Errorf("session repository not initialized")
	}

	// Call the DynamORM repository
	err := a.sessionRepo.CreateDevice(ctx, device)
	if err != nil {
		return MapRepositoryError(err, "CreateDevice", "Device", device.DeviceID)
	}

	return nil
}

// GetDevice retrieves a device by ID
func (a *StorageAdapter) GetDevice(ctx context.Context, deviceID string) (*storage.Device, error) {
	// Check if session repository is set
	if a.sessionRepo == nil {
		return nil, fmt.Errorf("session repository not initialized")
	}

	// Call the DynamORM repository
	device, err := a.sessionRepo.GetDevice(ctx, deviceID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetDevice", "Device", deviceID)
	}

	return device, nil
}

// UpdateDevice updates an existing device
func (a *StorageAdapter) UpdateDevice(ctx context.Context, device *storage.Device) error {
	// Check if session repository is set
	if a.sessionRepo == nil {
		return fmt.Errorf("session repository not initialized")
	}

	// Call the DynamORM repository
	err := a.sessionRepo.UpdateDevice(ctx, device)
	if err != nil {
		return MapRepositoryError(err, "UpdateDevice", "Device", device.DeviceID)
	}

	return nil
}

// GetUserDevices retrieves all devices for a user
func (a *StorageAdapter) GetUserDevices(ctx context.Context, username string) ([]*storage.Device, error) {
	// Check if session repository is set
	if a.sessionRepo == nil {
		return nil, fmt.Errorf("session repository not initialized")
	}

	// Call the DynamORM repository
	devices, err := a.sessionRepo.GetUserDevices(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetUserDevices", "Device", username)
	}

	return devices, nil
}

// WebAuthn-related methods

// StoreWebAuthnCredential stores a new WebAuthn credential
func (a *StorageAdapter) StoreWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error {
	// Check if auth repository is set
	if a.accountRepo == nil {
		return fmt.Errorf("auth repository not initialized")
	}

	// Call the DynamORM repository
	err := a.accountRepo.CreateWebAuthnCredential(ctx, credential)
	if err != nil {
		return MapRepositoryError(err, "StoreWebAuthnCredential", "WebAuthnCredential", credential.ID)
	}

	return nil
}

// GetWebAuthnCredential retrieves a WebAuthn credential by ID
func (a *StorageAdapter) GetWebAuthnCredential(ctx context.Context, credentialID string) (*storage.WebAuthnCredential, error) {
	// Check if auth repository is set
	if a.accountRepo == nil {
		return nil, fmt.Errorf("auth repository not initialized")
	}

	// Call the DynamORM repository
	credential, err := a.accountRepo.GetWebAuthnCredential(ctx, credentialID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetWebAuthnCredential", "WebAuthnCredential", credentialID)
	}

	return credential, nil
}

// GetUserWebAuthnCredentials retrieves all WebAuthn credentials for a user
func (a *StorageAdapter) GetUserWebAuthnCredentials(ctx context.Context, username string) ([]*storage.WebAuthnCredential, error) {
	// Check if auth repository is set
	if a.accountRepo == nil {
		return nil, fmt.Errorf("auth repository not initialized")
	}

	// Call the DynamORM repository
	credentials, err := a.accountRepo.GetUserWebAuthnCredentials(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetUserWebAuthnCredentials", "WebAuthnCredential", username)
	}

	return credentials, nil
}

// UpdateWebAuthnCredential updates an existing WebAuthn credential
func (a *StorageAdapter) UpdateWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error {
	// Check if auth repository is set
	if a.accountRepo == nil {
		return fmt.Errorf("auth repository not initialized")
	}

	// Call the DynamORM repository
	err := a.accountRepo.UpdateWebAuthnLastUsed(ctx, credential.ID, credential.SignCount)
	if err != nil {
		return MapRepositoryError(err, "UpdateWebAuthnCredential", "WebAuthnCredential", credential.ID)
	}

	return nil
}

// DeleteWebAuthnCredential deletes a WebAuthn credential by ID
func (a *StorageAdapter) DeleteWebAuthnCredential(ctx context.Context, credentialID string) error {
	// Check if auth repository is set
	if a.accountRepo == nil {
		return fmt.Errorf("auth repository not initialized")
	}

	// Call the DynamORM repository
	err := a.accountRepo.DeleteWebAuthnCredential(ctx, credentialID)
	if err != nil {
		return MapRepositoryError(err, "DeleteWebAuthnCredential", "WebAuthnCredential", credentialID)
	}

	return nil
}

// StoreWebAuthnChallenge stores a new WebAuthn challenge
func (a *StorageAdapter) StoreWebAuthnChallenge(ctx context.Context, challenge *storage.WebAuthnChallenge) error {
	// Check if auth repository is set
	if a.accountRepo == nil {
		return fmt.Errorf("auth repository not initialized")
	}

	// Call the DynamORM repository
	err := a.accountRepo.CreateWebAuthnChallenge(ctx, challenge)
	if err != nil {
		return MapRepositoryError(err, "StoreWebAuthnChallenge", "WebAuthnChallenge", challenge.Challenge)
	}

	return nil
}

// GetWebAuthnChallenge retrieves a WebAuthn challenge by ID
func (a *StorageAdapter) GetWebAuthnChallenge(ctx context.Context, challengeID string) (*storage.WebAuthnChallenge, error) {
	// Check if auth repository is set
	if a.accountRepo == nil {
		return nil, fmt.Errorf("auth repository not initialized")
	}

	// Call the DynamORM repository
	challenge, err := a.accountRepo.GetWebAuthnChallenge(ctx, challengeID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetWebAuthnChallenge", "WebAuthnChallenge", challengeID)
	}

	return challenge, nil
}

// DeleteWebAuthnChallenge deletes a WebAuthn challenge by ID
func (a *StorageAdapter) DeleteWebAuthnChallenge(ctx context.Context, challengeID string) error {
	// Check if auth repository is set
	if a.accountRepo == nil {
		return fmt.Errorf("auth repository not initialized")
	}

	// Call the DynamORM repository
	err := a.accountRepo.DeleteWebAuthnChallenge(ctx, challengeID)
	if err != nil {
		return MapRepositoryError(err, "DeleteWebAuthnChallenge", "WebAuthnChallenge", challengeID)
	}

	return nil
}

// Flag-related methods

// CreateFlag creates a new flag in the database
func (a *StorageAdapter) CreateFlag(ctx context.Context, flag *storage.Flag) error {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	err := a.moderationRepo.CreateFlag(ctx, flag)
	if err != nil {
		return MapRepositoryError(err, "CreateFlag", "Flag", flag.ID)
	}

	return nil
}

// GetFlag retrieves a flag by ID
func (a *StorageAdapter) GetFlag(ctx context.Context, id string) (*storage.Flag, error) {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return nil, fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	flag, err := a.moderationRepo.GetFlag(ctx, id)
	if err != nil {
		return nil, MapRepositoryError(err, "GetFlag", "Flag", id)
	}

	return flag, nil
}

// GetFlagsByObject retrieves all flags for a specific object with pagination
func (a *StorageAdapter) GetFlagsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return nil, "", fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	flags, nextCursor, err := a.moderationRepo.GetFlagsByObject(ctx, objectID, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetFlagsByObject", "Flag", objectID)
	}

	return flags, nextCursor, nil
}

// UpdateFlagStatus updates the status of a flag
func (a *StorageAdapter) UpdateFlagStatus(ctx context.Context, id string, status storage.FlagStatus, reviewedBy string, reviewNote string) error {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	err := a.moderationRepo.UpdateFlagStatus(ctx, id, status, reviewedBy, reviewNote)
	if err != nil {
		return MapRepositoryError(err, "UpdateFlagStatus", "Flag", id)
	}

	return nil
}

// Report-related methods

// CreateReport creates a new report in the database
func (a *StorageAdapter) CreateReport(ctx context.Context, report *storage.Report) error {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	err := a.moderationRepo.CreateReport(ctx, report)
	if err != nil {
		return MapRepositoryError(err, "CreateReport", "Report", report.ID)
	}

	return nil
}

// GetReport retrieves a report by ID
func (a *StorageAdapter) GetReport(ctx context.Context, id string) (*storage.Report, error) {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return nil, fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	report, err := a.moderationRepo.GetReport(ctx, id)
	if err != nil {
		return nil, MapRepositoryError(err, "GetReport", "Report", id)
	}

	return report, nil
}

// GetUserReports retrieves all reports created by a user with pagination
func (a *StorageAdapter) GetUserReports(ctx context.Context, username string, limit int, cursor string) ([]*storage.Report, string, error) {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return nil, "", fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	reports, nextCursor, err := a.moderationRepo.GetUserReports(ctx, username, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetUserReports", "Report", username)
	}

	return reports, nextCursor, nil
}

// UpdateReportStatus updates the status of a report
func (a *StorageAdapter) UpdateReportStatus(ctx context.Context, id string, status storage.ReportStatus, actionTaken string, moderatorID string) error {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	err := a.moderationRepo.UpdateReportStatus(ctx, id, status, actionTaken, moderatorID)
	if err != nil {
		return MapRepositoryError(err, "UpdateReportStatus", "Report", id)
	}

	return nil
}

// GetReportsByTarget retrieves reports targeting a specific account
func (a *StorageAdapter) GetReportsByTarget(ctx context.Context, targetAccountID string, limit int, cursor string) ([]*storage.Report, string, error) {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return nil, "", fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	reports, nextCursor, err := a.moderationRepo.GetReportsByTarget(ctx, targetAccountID, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetReportsByTarget", "Report", targetAccountID)
	}

	return reports, nextCursor, nil
}

// GetReportsByStatus retrieves reports with a specific status
func (a *StorageAdapter) GetReportsByStatus(ctx context.Context, status storage.ReportStatus, limit int, cursor string) ([]*storage.Report, string, error) {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return nil, "", fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	reports, nextCursor, err := a.moderationRepo.GetReportsByStatus(ctx, status, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetReportsByStatus", "Report", string(status))
	}

	return reports, nextCursor, nil
}

// GetReportStats retrieves reporting statistics for a user
func (a *StorageAdapter) GetReportStats(ctx context.Context, username string) (*storage.ReportStats, error) {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return nil, fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	stats, err := a.moderationRepo.GetReportStats(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetReportStats", "ReportStats", username)
	}

	return stats, nil
}

// IncrementFalseReports increments the false report count for a user
func (a *StorageAdapter) IncrementFalseReports(ctx context.Context, username string) error {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	err := a.moderationRepo.IncrementFalseReports(ctx, username)
	if err != nil {
		return MapRepositoryError(err, "IncrementFalseReports", "ReportStats", username)
	}

	return nil
}

// AssignReport assigns a report to a moderator
func (a *StorageAdapter) AssignReport(ctx context.Context, reportID string, assignedTo string) error {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	err := a.moderationRepo.AssignReport(ctx, reportID, assignedTo)
	if err != nil {
		return MapRepositoryError(err, "AssignReport", "Report", reportID)
	}

	return nil
}

// UnassignReport removes assignment from a report
func (a *StorageAdapter) UnassignReport(ctx context.Context, reportID string) error {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	err := a.moderationRepo.UnassignReport(ctx, reportID)
	if err != nil {
		return MapRepositoryError(err, "UnassignReport", "Report", reportID)
	}

	return nil
}

// GetOpenReportsCount returns the count of open reports
func (a *StorageAdapter) GetOpenReportsCount(ctx context.Context) (int, error) {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return 0, fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	count, err := a.moderationRepo.GetOpenReportsCount(ctx)
	if err != nil {
		return 0, MapRepositoryError(err, "GetOpenReportsCount", "Report", "open")
	}

	return count, nil
}

// GetReportedStatuses gets reported statuses for a specific report
func (a *StorageAdapter) GetReportedStatuses(ctx context.Context, reportID string) ([]any, error) {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return nil, fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	statuses, err := a.moderationRepo.GetReportedStatuses(ctx, reportID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetReportedStatuses", "Report", reportID)
	}

	return statuses, nil
}

// ModerationEvent-related methods

// CreateModerationEvent creates a new moderation event in the database
func (a *StorageAdapter) CreateModerationEvent(ctx context.Context, event *storage.ModerationEvent) error {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	err := a.moderationRepo.CreateModerationEvent(ctx, event)
	if err != nil {
		return MapRepositoryError(err, "CreateModerationEvent", "ModerationEvent", event.ID)
	}

	return nil
}

// GetModerationEvent retrieves a moderation event by ID
func (a *StorageAdapter) GetModerationEvent(ctx context.Context, eventID string) (*storage.ModerationEvent, error) {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return nil, fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	event, err := a.moderationRepo.GetModerationEvent(ctx, eventID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetModerationEvent", "ModerationEvent", eventID)
	}

	return event, nil
}

// ModerationPattern-related methods

// CreateModerationPattern creates a new moderation pattern in the database
func (a *StorageAdapter) CreateModerationPattern(ctx context.Context, pattern *storage.ModerationPattern) error {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	err := a.moderationRepo.CreateModerationPattern(ctx, pattern)
	if err != nil {
		return MapRepositoryError(err, "CreateModerationPattern", "ModerationPattern", pattern.ID)
	}

	return nil
}

// GetModerationPattern retrieves a moderation pattern by ID
func (a *StorageAdapter) GetModerationPattern(ctx context.Context, patternID string) (*storage.ModerationPattern, error) {
	// Check if moderation repository is set
	if a.moderationRepo == nil {
		return nil, fmt.Errorf("moderation repository not initialized")
	}

	// Call the DynamORM repository
	pattern, err := a.moderationRepo.GetModerationPattern(ctx, patternID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetModerationPattern", "ModerationPattern", patternID)
	}

	return pattern, nil
}

// List-related methods

// CreateList creates a new list
func (a *StorageAdapter) CreateList(ctx context.Context, username, title, repliesPolicy string) (*storage.List, error) {
	// Check if list repository is set
	if a.listRepo == nil {
		return nil, fmt.Errorf("list repository not initialized")
	}

	// Call the DynamORM repository
	list, err := a.listRepo.CreateList(ctx, username, title, repliesPolicy)
	if err != nil {
		return nil, MapRepositoryError(err, "CreateList", "List", title)
	}

	return list, nil
}

// GetList retrieves a list by ID
func (a *StorageAdapter) GetList(ctx context.Context, listID string) (*storage.List, error) {
	// Check if list repository is set
	if a.listRepo == nil {
		return nil, fmt.Errorf("list repository not initialized")
	}

	// Call the DynamORM repository
	list, err := a.listRepo.GetList(ctx, listID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetList", "List", listID)
	}

	return list, nil
}

// GetListsForUser retrieves all lists owned by a user
func (a *StorageAdapter) GetListsForUser(ctx context.Context, username string) ([]*storage.List, error) {
	// Check if list repository is set
	if a.listRepo == nil {
		return nil, fmt.Errorf("list repository not initialized")
	}

	// Call the DynamORM repository
	lists, err := a.listRepo.GetListsForUser(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetListsForUser", "List", username)
	}

	return lists, nil
}

// UpdateList updates an existing list
func (a *StorageAdapter) UpdateList(ctx context.Context, listID string, updates map[string]any) error {
	// Check if list repository is set
	if a.listRepo == nil {
		return fmt.Errorf("list repository not initialized")
	}

	// Call the DynamORM repository
	err := a.listRepo.UpdateList(ctx, listID, updates)
	if err != nil {
		return MapRepositoryError(err, "UpdateList", "List", listID)
	}

	return nil
}

// DeleteList deletes a list
func (a *StorageAdapter) DeleteList(ctx context.Context, listID string) error {
	// Check if list repository is set
	if a.listRepo == nil {
		return fmt.Errorf("list repository not initialized")
	}

	// Call the DynamORM repository
	err := a.listRepo.DeleteList(ctx, listID)
	if err != nil {
		return MapRepositoryError(err, "DeleteList", "List", listID)
	}

	return nil
}

// GetListAccounts retrieves all accounts in a list
func (a *StorageAdapter) GetListAccounts(ctx context.Context, listID string) ([]string, error) {
	// Check if list repository is set
	if a.listRepo == nil {
		return nil, fmt.Errorf("list repository not initialized")
	}

	// Call the DynamORM repository
	accounts, err := a.listRepo.GetListAccounts(ctx, listID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetListAccounts", "List", listID)
	}

	return accounts, nil
}

// GetListsContainingAccount retrieves all lists (for a specific user) that contain an account
func (a *StorageAdapter) GetListsContainingAccount(ctx context.Context, accountID, username string) ([]*storage.List, error) {
	// Check if list repository is set
	if a.listRepo == nil {
		return nil, fmt.Errorf("list repository not initialized")
	}

	// Call the DynamORM repository
	lists, err := a.listRepo.GetListsContainingAccount(ctx, accountID, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetListsContainingAccount", "List", accountID)
	}

	return lists, nil
}

// AddAccountsToList adds multiple accounts to a list
func (a *StorageAdapter) AddAccountsToList(ctx context.Context, listID string, accountIDs []string) error {
	// Check if list repository is set
	if a.listRepo == nil {
		return fmt.Errorf("list repository not initialized")
	}

	// Call the DynamORM repository
	err := a.listRepo.AddAccountsToList(ctx, listID, accountIDs)
	if err != nil {
		return MapRepositoryError(err, "AddAccountsToList", "List", listID)
	}

	return nil
}

// RemoveAccountsFromList removes multiple accounts from a list
func (a *StorageAdapter) RemoveAccountsFromList(ctx context.Context, listID string, accountIDs []string) error {
	// Check if list repository is set
	if a.listRepo == nil {
		return fmt.Errorf("list repository not initialized")
	}

	// Call the DynamORM repository
	err := a.listRepo.RemoveAccountsFromList(ctx, listID, accountIDs)
	if err != nil {
		return MapRepositoryError(err, "RemoveAccountsFromList", "List", listID)
	}

	return nil
}

// IsAccountInList checks if an account is in a list
func (a *StorageAdapter) IsAccountInList(ctx context.Context, listID, accountID string) (bool, error) {
	// Check if list repository is set
	if a.listRepo == nil {
		return false, fmt.Errorf("list repository not initialized")
	}

	// Call the DynamORM repository
	inList, err := a.listRepo.IsAccountInList(ctx, listID, accountID)
	if err != nil {
		return false, MapRepositoryError(err, "IsAccountInList", "List", listID)
	}

	return inList, nil
}

// Poll-related methods

// CreatePoll creates a new poll in the database
func (a *StorageAdapter) CreatePoll(ctx context.Context, poll *storage.Poll) error {
	// Check if poll repository is set
	if a.pollRepo == nil {
		return fmt.Errorf("poll repository not initialized")
	}

	// Call the DynamORM repository
	err := a.pollRepo.CreatePoll(ctx, poll)
	if err != nil {
		return MapRepositoryError(err, "CreatePoll", "Poll", poll.ID)
	}

	return nil
}

// GetPoll retrieves a poll by ID
func (a *StorageAdapter) GetPoll(ctx context.Context, pollID string) (*storage.Poll, error) {
	// Check if poll repository is set
	if a.pollRepo == nil {
		return nil, fmt.Errorf("poll repository not initialized")
	}

	// Call the DynamORM repository
	poll, err := a.pollRepo.GetPoll(ctx, pollID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetPoll", "Poll", pollID)
	}

	return poll, nil
}

// GetPollByStatusID retrieves a poll by its associated status ID
func (a *StorageAdapter) GetPollByStatusID(ctx context.Context, statusID string) (*storage.Poll, error) {
	// Check if poll repository is set
	if a.pollRepo == nil {
		return nil, fmt.Errorf("poll repository not initialized")
	}

	// Call the DynamORM repository
	poll, err := a.pollRepo.GetPollByStatusID(ctx, statusID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetPollByStatusID", "Poll", statusID)
	}

	return poll, nil
}

// GetPollVotes retrieves all votes for a poll
func (a *StorageAdapter) GetPollVotes(ctx context.Context, pollID string) (map[string][]int, error) {
	// Check if poll repository is set
	if a.pollRepo == nil {
		return nil, fmt.Errorf("poll repository not initialized")
	}

	// Call the DynamORM repository
	votes, err := a.pollRepo.GetPollVotes(ctx, pollID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetPollVotes", "Poll", pollID)
	}

	return votes, nil
}

// VoteOnPoll records a vote on a poll
func (a *StorageAdapter) VoteOnPoll(ctx context.Context, pollID, userID string, choices []int) error {
	// Check if poll repository is set
	if a.pollRepo == nil {
		return fmt.Errorf("poll repository not initialized")
	}

	// Call the DynamORM repository
	err := a.pollRepo.VoteOnPoll(ctx, pollID, userID, choices)
	if err != nil {
		return MapRepositoryError(err, "VoteOnPoll", "Poll", pollID)
	}

	return nil
}

// HasUserVoted checks if a user has voted on a poll and returns their choices
func (a *StorageAdapter) HasUserVoted(ctx context.Context, pollID string, userID string) (bool, []int, error) {
	// Check if poll repository is set
	if a.pollRepo == nil {
		return false, nil, fmt.Errorf("poll repository not initialized")
	}

	// Call the DynamORM repository
	hasVoted, choices, err := a.pollRepo.HasUserVoted(ctx, pollID, userID)
	if err != nil {
		return false, nil, MapRepositoryError(err, "HasUserVoted", "Poll", pollID)
	}

	return hasVoted, choices, nil
}

// Media-related methods

// GetUserMedia retrieves all media for a user
func (a *StorageAdapter) GetUserMedia(ctx context.Context, username string) ([]any, error) {
	// Check if media repository is set
	if a.mediaRepo == nil {
		return nil, fmt.Errorf("media repository not initialized")
	}

	// Call the DynamORM repository
	media, err := a.mediaRepo.GetUserMedia(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetUserMedia", "Media", username)
	}

	return media, nil
}

// UpdateMediaAttachment updates a media attachment with the provided updates
func (a *StorageAdapter) UpdateMediaAttachment(ctx context.Context, mediaID string, updates map[string]any) error {
	// Check if media repository is set
	if a.mediaRepo == nil {
		return fmt.Errorf("media repository not initialized")
	}

	// Call the DynamORM repository
	err := a.mediaRepo.UpdateMediaAttachment(ctx, mediaID, updates)
	if err != nil {
		return MapRepositoryError(err, "UpdateMediaAttachment", "Media", mediaID)
	}

	return nil
}

// UnmarkAllMediaAsSensitive unmarks all media for a user as non-sensitive
func (a *StorageAdapter) UnmarkAllMediaAsSensitive(ctx context.Context, username string) error {
	// Check if media repository is set
	if a.mediaRepo == nil {
		return fmt.Errorf("media repository not initialized")
	}

	// Call the DynamORM repository
	err := a.mediaRepo.UnmarkAllMediaAsSensitive(ctx, username)
	if err != nil {
		return MapRepositoryError(err, "UnmarkAllMediaAsSensitive", "Media", username)
	}

	return nil
}

// Media Job-related methods

// CreateMediaJob creates a new media processing job
func (a *StorageAdapter) CreateMediaJob(ctx context.Context, job *models.MediaJob) error {
	// Check if media repository is set
	if a.mediaRepo == nil {
		return fmt.Errorf("media repository not initialized")
	}

	// Call the DynamORM repository
	err := a.mediaRepo.CreateMediaJob(ctx, job)
	if err != nil {
		return MapRepositoryError(err, "CreateMediaJob", "MediaJob", job.JobID)
	}

	return nil
}

// GetMediaJob retrieves a media job by ID
func (a *StorageAdapter) GetMediaJob(ctx context.Context, jobID string) (*models.MediaJob, error) {
	// Check if media repository is set
	if a.mediaRepo == nil {
		return nil, fmt.Errorf("media repository not initialized")
	}

	// Call the DynamORM repository
	job, err := a.mediaRepo.GetMediaJob(ctx, jobID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetMediaJob", "MediaJob", jobID)
	}

	return job, nil
}

// UpdateMediaJob updates an existing media job
func (a *StorageAdapter) UpdateMediaJob(ctx context.Context, job *models.MediaJob) error {
	// Check if media repository is set
	if a.mediaRepo == nil {
		return fmt.Errorf("media repository not initialized")
	}

	// Call the DynamORM repository
	err := a.mediaRepo.UpdateMediaJob(ctx, job)
	if err != nil {
		return MapRepositoryError(err, "UpdateMediaJob", "MediaJob", job.JobID)
	}

	return nil
}

// Media-related methods

// CreateMedia creates a new media record
func (a *StorageAdapter) CreateMedia(ctx context.Context, media *models.Media) error {
	// Check if media repository is set
	if a.mediaRepo == nil {
		return fmt.Errorf("media repository not initialized")
	}

	// Call the DynamORM repository
	err := a.mediaRepo.CreateMedia(ctx, media)
	if err != nil {
		return MapRepositoryError(err, "CreateMedia", "Media", media.MediaID)
	}

	return nil
}

// GetMedia retrieves a media record by ID
func (a *StorageAdapter) GetMedia(ctx context.Context, mediaID string) (*models.Media, error) {
	// Check if media repository is set
	if a.mediaRepo == nil {
		return nil, fmt.Errorf("media repository not initialized")
	}

	// Call the DynamORM repository
	media, err := a.mediaRepo.GetMedia(ctx, mediaID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetMedia", "Media", mediaID)
	}

	return media, nil
}

// UpdateMedia updates an existing media record
func (a *StorageAdapter) UpdateMedia(ctx context.Context, media *models.Media) error {
	// Check if media repository is set
	if a.mediaRepo == nil {
		return fmt.Errorf("media repository not initialized")
	}

	// Call the DynamORM repository
	err := a.mediaRepo.UpdateMedia(ctx, media)
	if err != nil {
		return MapRepositoryError(err, "UpdateMedia", "Media", media.MediaID)
	}

	return nil
}

// Push Subscription-related methods

// CreatePushSubscription creates a new push subscription
func (a *StorageAdapter) CreatePushSubscription(ctx context.Context, username string, subscription *storage.PushSubscription) error {
	// Check if push subscription repository is set
	if a.pushSubscriptionRepo == nil {
		return fmt.Errorf("push subscription repository not initialized")
	}

	// Call the DynamORM repository
	err := a.pushSubscriptionRepo.CreatePushSubscription(ctx, username, subscription)
	if err != nil {
		return MapRepositoryError(err, "CreatePushSubscription", "PushSubscription", subscription.ID)
	}

	return nil
}

// GetPushSubscription retrieves a push subscription by ID
func (a *StorageAdapter) GetPushSubscription(ctx context.Context, username, subscriptionID string) (*storage.PushSubscription, error) {
	// Check if push subscription repository is set
	if a.pushSubscriptionRepo == nil {
		return nil, fmt.Errorf("push subscription repository not initialized")
	}

	// Call the DynamORM repository
	subscription, err := a.pushSubscriptionRepo.GetPushSubscription(ctx, username, subscriptionID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetPushSubscription", "PushSubscription", subscriptionID)
	}

	return subscription, nil
}

// GetUserPushSubscriptions retrieves all push subscriptions for a user
func (a *StorageAdapter) GetUserPushSubscriptions(ctx context.Context, username string) ([]*storage.PushSubscription, error) {
	// Check if push subscription repository is set
	if a.pushSubscriptionRepo == nil {
		return nil, fmt.Errorf("push subscription repository not initialized")
	}

	// Call the DynamORM repository
	subscriptions, err := a.pushSubscriptionRepo.GetUserPushSubscriptions(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetUserPushSubscriptions", "PushSubscription", username)
	}

	return subscriptions, nil
}

// UpdatePushSubscription updates the alerts for a push subscription
func (a *StorageAdapter) UpdatePushSubscription(ctx context.Context, username, subscriptionID string, alerts storage.PushSubscriptionAlerts) error {
	// Check if push subscription repository is set
	if a.pushSubscriptionRepo == nil {
		return fmt.Errorf("push subscription repository not initialized")
	}

	// Call the DynamORM repository
	err := a.pushSubscriptionRepo.UpdatePushSubscription(ctx, username, subscriptionID, alerts)
	if err != nil {
		return MapRepositoryError(err, "UpdatePushSubscription", "PushSubscription", subscriptionID)
	}

	return nil
}

// DeletePushSubscription deletes a push subscription
func (a *StorageAdapter) DeletePushSubscription(ctx context.Context, username, subscriptionID string) error {
	// Check if push subscription repository is set
	if a.pushSubscriptionRepo == nil {
		return fmt.Errorf("push subscription repository not initialized")
	}

	// Call the DynamORM repository
	err := a.pushSubscriptionRepo.DeletePushSubscription(ctx, username, subscriptionID)
	if err != nil {
		return MapRepositoryError(err, "DeletePushSubscription", "PushSubscription", subscriptionID)
	}

	return nil
}

// DeleteAllPushSubscriptions deletes all push subscriptions for a user
func (a *StorageAdapter) DeleteAllPushSubscriptions(ctx context.Context, username string) error {
	// Check if push subscription repository is set
	if a.pushSubscriptionRepo == nil {
		return fmt.Errorf("push subscription repository not initialized")
	}

	// Call the DynamORM repository
	err := a.pushSubscriptionRepo.DeleteAllPushSubscriptions(ctx, username)
	if err != nil {
		return MapRepositoryError(err, "DeleteAllPushSubscriptions", "PushSubscription", username)
	}

	return nil
}

// GetVAPIDKeys retrieves the VAPID keys for the instance
func (a *StorageAdapter) GetVAPIDKeys(ctx context.Context) (*storage.VAPIDKeys, error) {
	// Check if push subscription repository is set
	if a.pushSubscriptionRepo == nil {
		return nil, fmt.Errorf("push subscription repository not initialized")
	}

	// Call the DynamORM repository
	keys, err := a.pushSubscriptionRepo.GetVAPIDKeys(ctx)
	if err != nil {
		return nil, MapRepositoryError(err, "GetVAPIDKeys", "VAPIDKeys", "instance")
	}

	return keys, nil
}

// SetVAPIDKeys stores the VAPID keys for the instance
func (a *StorageAdapter) SetVAPIDKeys(ctx context.Context, keys *storage.VAPIDKeys) error {
	// Check if push subscription repository is set
	if a.pushSubscriptionRepo == nil {
		return fmt.Errorf("push subscription repository not initialized")
	}

	// Call the DynamORM repository
	err := a.pushSubscriptionRepo.SetVAPIDKeys(ctx, keys)
	if err != nil {
		return MapRepositoryError(err, "SetVAPIDKeys", "VAPIDKeys", "instance")
	}

	return nil
}

// Block methods
func (a *StorageAdapter) CreateBlock(ctx context.Context, block *storage.Block) error {
	if a.socialRepo == nil {
		return fmt.Errorf("social repository not initialized")
	}
	err := a.socialRepo.CreateBlock(ctx, block)
	if err != nil {
		return MapRepositoryError(err, "CreateBlock", "Block", fmt.Sprintf("%s->%s", block.Actor, block.Object))
	}
	return nil
}

func (a *StorageAdapter) GetBlock(ctx context.Context, actor, blockedActor string) (*storage.Block, error) {
	if a.socialRepo == nil {
		return nil, fmt.Errorf("social repository not initialized")
	}
	block, err := a.socialRepo.GetBlock(ctx, actor, blockedActor)
	if err != nil {
		return nil, MapRepositoryError(err, "GetBlock", "Block", fmt.Sprintf("%s->%s", actor, blockedActor))
	}
	return block, nil
}

func (a *StorageAdapter) DeleteBlock(ctx context.Context, actor, blockedActor string) error {
	if a.socialRepo == nil {
		return fmt.Errorf("social repository not initialized")
	}
	err := a.socialRepo.DeleteBlock(ctx, actor, blockedActor)
	if err != nil {
		return MapRepositoryError(err, "DeleteBlock", "Block", fmt.Sprintf("%s->%s", actor, blockedActor))
	}
	return nil
}

func (a *StorageAdapter) GetBlockedActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	if a.socialRepo == nil {
		return nil, "", fmt.Errorf("social repository not initialized")
	}
	blocks, nextCursor, err := a.socialRepo.GetBlockedUsers(ctx, actor, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetBlockedActors", "Block", actor)
	}
	return blocks, nextCursor, nil
}

func (a *StorageAdapter) GetBlockedByActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	if a.socialRepo == nil {
		return nil, "", fmt.Errorf("social repository not initialized")
	}
	blocks, nextCursor, err := a.socialRepo.GetBlockedByUsers(ctx, actor, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetBlockedByActors", "Block", actor)
	}
	return blocks, nextCursor, nil
}

func (a *StorageAdapter) IsBlocked(ctx context.Context, actor, targetActor string) (bool, error) {
	if a.socialRepo == nil {
		return false, fmt.Errorf("social repository not initialized")
	}
	blocked, err := a.socialRepo.IsBlocked(ctx, actor, targetActor)
	if err != nil {
		return false, MapRepositoryError(err, "IsBlocked", "Block", fmt.Sprintf("%s->%s", actor, targetActor))
	}
	return blocked, nil
}

func (a *StorageAdapter) IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error) {
	if a.socialRepo == nil {
		return false, fmt.Errorf("social repository not initialized")
	}
	// Check if actor1 blocked actor2
	blocked1to2, err := a.socialRepo.IsBlocked(ctx, actor1, actor2)
	if err != nil {
		return false, MapRepositoryError(err, "IsBlockedBidirectional", "Block", fmt.Sprintf("%s<->%s", actor1, actor2))
	}
	if blocked1to2 {
		return true, nil
	}
	
	// Check if actor2 blocked actor1
	blocked2to1, err := a.socialRepo.IsBlocked(ctx, actor2, actor1)
	if err != nil {
		return false, MapRepositoryError(err, "IsBlockedBidirectional", "Block", fmt.Sprintf("%s<->%s", actor1, actor2))
	}
	return blocked2to1, nil
}

// Flag/Report methods - additional methods not already implemented
func (a *StorageAdapter) GetFlagsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	if a.moderationRepo == nil {
		return nil, "", fmt.Errorf("moderation repository not initialized")
	}
	flags, nextCursor, err := a.moderationRepo.GetFlagsByActor(ctx, actorID, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetFlagsByActor", "Flag", actorID)
	}
	return flags, nextCursor, nil
}

func (a *StorageAdapter) GetPendingFlags(ctx context.Context, limit int, cursor string) ([]*storage.Flag, string, error) {
	if a.moderationRepo == nil {
		return nil, "", fmt.Errorf("moderation repository not initialized")
	}
	flags, nextCursor, err := a.moderationRepo.GetPendingFlags(ctx, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetPendingFlags", "Flag", "pending")
	}
	return flags, nextCursor, nil
}

func (a *StorageAdapter) CountPendingFlags(ctx context.Context) (int, error) {
	if a.moderationRepo == nil {
		return 0, fmt.Errorf("moderation repository not initialized")
	}
	count, err := a.moderationRepo.CountPendingFlags(ctx)
	if err != nil {
		return 0, MapRepositoryError(err, "CountPendingFlags", "Flag", "pending")
	}
	return count, nil
}

// Mute methods
func (a *StorageAdapter) CreateMute(ctx context.Context, mute *storage.Mute) error {
	if a.socialRepo == nil {
		return fmt.Errorf("social repository not initialized")
	}
	err := a.socialRepo.CreateMute(ctx, mute)
	if err != nil {
		return MapRepositoryError(err, "CreateMute", "Mute", fmt.Sprintf("%s->%s", mute.Actor, mute.Object))
	}
	return nil
}

func (a *StorageAdapter) GetMute(ctx context.Context, actor, mutedActor string) (*storage.Mute, error) {
	if a.socialRepo == nil {
		return nil, fmt.Errorf("social repository not initialized")
	}
	mute, err := a.socialRepo.GetMute(ctx, actor, mutedActor)
	if err != nil {
		return nil, MapRepositoryError(err, "GetMute", "Mute", fmt.Sprintf("%s->%s", actor, mutedActor))
	}
	return mute, nil
}

func (a *StorageAdapter) DeleteMute(ctx context.Context, actor, mutedActor string) error {
	if a.socialRepo == nil {
		return fmt.Errorf("social repository not initialized")
	}
	err := a.socialRepo.DeleteMute(ctx, actor, mutedActor)
	if err != nil {
		return MapRepositoryError(err, "DeleteMute", "Mute", fmt.Sprintf("%s->%s", actor, mutedActor))
	}
	return nil
}

func (a *StorageAdapter) GetMutedActors(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Mute, string, error) {
	if a.socialRepo == nil {
		return nil, "", fmt.Errorf("social repository not initialized")
	}
	mutes, nextCursor, err := a.socialRepo.GetMutedUsers(ctx, actor, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetMutedActors", "Mute", actor)
	}
	return mutes, nextCursor, nil
}

func (a *StorageAdapter) IsMuted(ctx context.Context, actor, targetActor string) (bool, error) {
	if a.socialRepo == nil {
		return false, fmt.Errorf("social repository not initialized")
	}
	muted, err := a.socialRepo.IsMuted(ctx, actor, targetActor)
	if err != nil {
		return false, MapRepositoryError(err, "IsMuted", "Mute", fmt.Sprintf("%s->%s", actor, targetActor))
	}
	return muted, nil
}

// Move methods
func (a *StorageAdapter) CreateMove(ctx context.Context, move *storage.Move) error {
	if a.relationshipRepo == nil {
		return fmt.Errorf("relationship repository not initialized")
	}
	err := a.relationshipRepo.CreateMove(ctx, move)
	if err != nil {
		return MapRepositoryError(err, "CreateMove", "Move", move.Actor)
	}
	return nil
}

func (a *StorageAdapter) GetMove(ctx context.Context, actor string) (*storage.Move, error) {
	if a.relationshipRepo == nil {
		return nil, fmt.Errorf("relationship repository not initialized")
	}
	move, err := a.relationshipRepo.GetMove(ctx, actor)
	if err != nil {
		return nil, MapRepositoryError(err, "GetMove", "Move", actor)
	}
	return move, nil
}

func (a *StorageAdapter) GetMoveByTarget(ctx context.Context, target string) ([]*storage.Move, error) {
	if a.relationshipRepo == nil {
		return nil, fmt.Errorf("relationship repository not initialized")
	}
	moves, err := a.relationshipRepo.GetMoveByTarget(ctx, target)
	if err != nil {
		return nil, MapRepositoryError(err, "GetMoveByTarget", "Move", target)
	}
	return moves, nil
}

func (a *StorageAdapter) HasMovedFrom(ctx context.Context, oldActor, newActor string) (bool, error) {
	if a.relationshipRepo == nil {
		return false, fmt.Errorf("relationship repository not initialized")
	}
	moved, err := a.relationshipRepo.HasMovedFrom(ctx, oldActor, newActor)
	if err != nil {
		return false, MapRepositoryError(err, "HasMovedFrom", "Move", fmt.Sprintf("%s->%s", oldActor, newActor))
	}
	return moved, nil
}

// Collection methods
func (a *StorageAdapter) AddToCollection(ctx context.Context, collection string, item *storage.CollectionItem) error {
	if a.objectRepo == nil {
		return fmt.Errorf("object repository not initialized")
	}
	err := a.objectRepo.AddToCollection(ctx, collection, item)
	if err != nil {
		return MapRepositoryError(err, "AddToCollection", "CollectionItem", item.ItemID)
	}
	return nil
}

func (a *StorageAdapter) RemoveFromCollection(ctx context.Context, collection string, itemID string) error {
	if a.objectRepo == nil {
		return fmt.Errorf("object repository not initialized")
	}
	err := a.objectRepo.RemoveFromCollection(ctx, collection, itemID)
	if err != nil {
		return MapRepositoryError(err, "RemoveFromCollection", "CollectionItem", itemID)
	}
	return nil
}

func (a *StorageAdapter) GetCollectionItems(ctx context.Context, collection string, limit int, cursor string) ([]*storage.CollectionItem, string, error) {
	if a.objectRepo == nil {
		return nil, "", fmt.Errorf("object repository not initialized")
	}
	items, nextCursor, err := a.objectRepo.GetCollectionItems(ctx, collection, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetCollectionItems", "Collection", collection)
	}
	return items, nextCursor, nil
}

func (a *StorageAdapter) IsInCollection(ctx context.Context, collection string, itemID string) (bool, error) {
	if a.objectRepo == nil {
		return false, fmt.Errorf("object repository not initialized")
	}
	inCollection, err := a.objectRepo.IsInCollection(ctx, collection, itemID)
	if err != nil {
		return false, MapRepositoryError(err, "IsInCollection", "CollectionItem", itemID)
	}
	return inCollection, nil
}

func (a *StorageAdapter) CountCollectionItems(ctx context.Context, collection string) (int, error) {
	if a.objectRepo == nil {
		return 0, fmt.Errorf("object repository not initialized")
	}
	count, err := a.objectRepo.CountCollectionItems(ctx, collection)
	if err != nil {
		return 0, MapRepositoryError(err, "CountCollectionItems", "Collection", collection)
	}
	return count, nil
}

// Filter methods
func (a *StorageAdapter) CreateFilter(ctx context.Context, filter *storage.Filter) error {
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}
	err := a.moderationRepo.CreateFilter(ctx, filter)
	if err != nil {
		return MapRepositoryError(err, "CreateFilter", "Filter", filter.ID)
	}
	return nil
}

func (a *StorageAdapter) GetFilter(ctx context.Context, filterID string) (*storage.Filter, error) {
	if a.moderationRepo == nil {
		return nil, fmt.Errorf("moderation repository not initialized")
	}
	filter, err := a.moderationRepo.GetFilter(ctx, filterID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetFilter", "Filter", filterID)
	}
	return filter, nil
}

func (a *StorageAdapter) GetFiltersForUser(ctx context.Context, username string) ([]*storage.Filter, error) {
	if a.moderationRepo == nil {
		return nil, fmt.Errorf("moderation repository not initialized")
	}
	filters, err := a.moderationRepo.GetFiltersForUser(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetFiltersForUser", "Filter", username)
	}
	return filters, nil
}

func (a *StorageAdapter) UpdateFilter(ctx context.Context, filterID string, updates map[string]any) error {
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}
	err := a.moderationRepo.UpdateFilter(ctx, filterID, updates)
	if err != nil {
		return MapRepositoryError(err, "UpdateFilter", "Filter", filterID)
	}
	return nil
}

func (a *StorageAdapter) DeleteFilter(ctx context.Context, filterID string) error {
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}
	err := a.moderationRepo.DeleteFilter(ctx, filterID)
	if err != nil {
		return MapRepositoryError(err, "DeleteFilter", "Filter", filterID)
	}
	return nil
}

func (a *StorageAdapter) AddFilterKeyword(ctx context.Context, filterID string, keyword *storage.FilterKeyword) error {
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}
	err := a.moderationRepo.AddFilterKeyword(ctx, filterID, keyword)
	if err != nil {
		return MapRepositoryError(err, "AddFilterKeyword", "FilterKeyword", keyword.ID)
	}
	return nil
}

func (a *StorageAdapter) GetFilterKeywords(ctx context.Context, filterID string) ([]*storage.FilterKeyword, error) {
	if a.moderationRepo == nil {
		return nil, fmt.Errorf("moderation repository not initialized")
	}
	keywords, err := a.moderationRepo.GetFilterKeywords(ctx, filterID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetFilterKeywords", "FilterKeyword", filterID)
	}
	return keywords, nil
}

func (a *StorageAdapter) UpdateFilterKeyword(ctx context.Context, keywordID string, updates map[string]any) error {
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}
	err := a.moderationRepo.UpdateFilterKeyword(ctx, keywordID, updates)
	if err != nil {
		return MapRepositoryError(err, "UpdateFilterKeyword", "FilterKeyword", keywordID)
	}
	return nil
}

func (a *StorageAdapter) DeleteFilterKeyword(ctx context.Context, keywordID string) error {
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}
	err := a.moderationRepo.DeleteFilterKeyword(ctx, keywordID)
	if err != nil {
		return MapRepositoryError(err, "DeleteFilterKeyword", "FilterKeyword", keywordID)
	}
	return nil
}

func (a *StorageAdapter) AddFilterStatus(ctx context.Context, filterID string, status *storage.FilterStatus) error {
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}
	err := a.moderationRepo.AddFilterStatus(ctx, filterID, status)
	if err != nil {
		return MapRepositoryError(err, "AddFilterStatus", "FilterStatus", status.ID)
	}
	return nil
}

func (a *StorageAdapter) GetFilterStatuses(ctx context.Context, filterID string) ([]*storage.FilterStatus, error) {
	if a.moderationRepo == nil {
		return nil, fmt.Errorf("moderation repository not initialized")
	}
	statuses, err := a.moderationRepo.GetFilterStatuses(ctx, filterID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetFilterStatuses", "FilterStatus", filterID)
	}
	return statuses, nil
}

func (a *StorageAdapter) DeleteFilterStatus(ctx context.Context, statusID string) error {
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}
	err := a.moderationRepo.DeleteFilterStatus(ctx, statusID)
	if err != nil {
		return MapRepositoryError(err, "DeleteFilterStatus", "FilterStatus", statusID)
	}
	return nil
}

// Conversation methods

// CreateConversation creates a new conversation in the database
func (a *StorageAdapter) CreateConversation(ctx context.Context, conversation *storage.Conversation) error {
	// Check if conversation repository is set
	if a.conversationRepo == nil {
		return fmt.Errorf("conversation repository not initialized")
	}

	// Call the DynamORM repository
	err := a.conversationRepo.CreateConversation(ctx, conversation)
	if err != nil {
		return MapRepositoryError(err, "CreateConversation", "Conversation", conversation.ID)
	}

	return nil
}

// GetConversation retrieves a conversation by ID
func (a *StorageAdapter) GetConversation(ctx context.Context, id string) (*storage.Conversation, error) {
	// Check if conversation repository is set
	if a.conversationRepo == nil {
		return nil, fmt.Errorf("conversation repository not initialized")
	}

	// Call the DynamORM repository
	conversation, err := a.conversationRepo.GetConversation(ctx, id)
	if err != nil {
		return nil, MapRepositoryError(err, "GetConversation", "Conversation", id)
	}

	return conversation, nil
}

// GetConversationByParticipants retrieves a conversation by its participants
func (a *StorageAdapter) GetConversationByParticipants(ctx context.Context, participants []string) (*storage.Conversation, error) {
	// Check if conversation repository is set
	if a.conversationRepo == nil {
		return nil, fmt.Errorf("conversation repository not initialized")
	}

	// Call the DynamORM repository
	conversation, err := a.conversationRepo.GetConversationByParticipants(ctx, participants)
	if err != nil {
		return nil, MapRepositoryError(err, "GetConversationByParticipants", "Conversation", fmt.Sprintf("participants: %v", participants))
	}

	return conversation, nil
}

// UpdateConversationLastStatus updates the last status of a conversation
func (a *StorageAdapter) UpdateConversationLastStatus(ctx context.Context, id, lastStatusID string) error {
	// Check if conversation repository is set
	if a.conversationRepo == nil {
		return fmt.Errorf("conversation repository not initialized")
	}

	// Call the DynamORM repository
	err := a.conversationRepo.UpdateConversationLastStatus(ctx, id, lastStatusID)
	if err != nil {
		return MapRepositoryError(err, "UpdateConversationLastStatus", "Conversation", id)
	}

	return nil
}

// MarkConversationRead marks a conversation as read by a user
func (a *StorageAdapter) MarkConversationRead(ctx context.Context, id, username string) error {
	// Check if conversation repository is set
	if a.conversationRepo == nil {
		return fmt.Errorf("conversation repository not initialized")
	}

	// Call the DynamORM repository
	err := a.conversationRepo.MarkConversationRead(ctx, id, username)
	if err != nil {
		return MapRepositoryError(err, "MarkConversationRead", "Conversation", fmt.Sprintf("%s:%s", id, username))
	}

	return nil
}

// DeleteConversation deletes a conversation by ID
func (a *StorageAdapter) DeleteConversation(ctx context.Context, id string) error {
	// Check if conversation repository is set
	if a.conversationRepo == nil {
		return fmt.Errorf("conversation repository not initialized")
	}

	// Call the DynamORM repository
	err := a.conversationRepo.DeleteConversation(ctx, id)
	if err != nil {
		return MapRepositoryError(err, "DeleteConversation", "Conversation", id)
	}

	return nil
}

// GetUserConversations retrieves conversations for a user with pagination
func (a *StorageAdapter) GetUserConversations(ctx context.Context, username string, limit int, cursor string) ([]*storage.Conversation, string, error) {
	// Check if conversation repository is set
	if a.conversationRepo == nil {
		return nil, "", fmt.Errorf("conversation repository not initialized")
	}

	// Call the DynamORM repository
	conversations, nextCursor, err := a.conversationRepo.GetUserConversations(ctx, username, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetUserConversations", "Conversation", username)
	}

	return conversations, nextCursor, nil
}

// AddParticipantToConversation adds a participant to a conversation
func (a *StorageAdapter) AddParticipantToConversation(ctx context.Context, conversationID, participantID string) error {
	// Check if conversation repository is set
	if a.conversationRepo == nil {
		return fmt.Errorf("conversation repository not initialized")
	}

	// Call the DynamORM repository
	err := a.conversationRepo.AddParticipantToConversation(ctx, conversationID, participantID)
	if err != nil {
		return MapRepositoryError(err, "AddParticipantToConversation", "Conversation", fmt.Sprintf("%s:%s", conversationID, participantID))
	}

	return nil
}

// Bookmark methods

// CreateBookmark creates a new bookmark for a user
func (a *StorageAdapter) CreateBookmark(ctx context.Context, username, objectID string) error {
	// Check if user repository is set
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}

	// Call the DynamORM repository
	err := a.userRepo.CreateBookmark(ctx, username, objectID)
	if err != nil {
		return MapRepositoryError(err, "CreateBookmark", "Bookmark", fmt.Sprintf("%s:%s", username, objectID))
	}

	return nil
}

// RemoveBookmark removes a bookmark for a user
func (a *StorageAdapter) RemoveBookmark(ctx context.Context, username, objectID string) error {
	// Check if user repository is set
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}

	// Call the DynamORM repository
	err := a.userRepo.RemoveBookmark(ctx, username, objectID)
	if err != nil {
		return MapRepositoryError(err, "RemoveBookmark", "Bookmark", fmt.Sprintf("%s:%s", username, objectID))
	}

	return nil
}

// GetBookmarks retrieves bookmarks for a user with pagination
func (a *StorageAdapter) GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	// Check if user repository is set
	if a.userRepo == nil {
		return nil, "", fmt.Errorf("user repository not initialized")
	}

	// Call the DynamORM repository
	bookmarks, nextCursor, err := a.userRepo.GetBookmarks(ctx, username, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetBookmarks", "Bookmark", username)
	}

	return bookmarks, nextCursor, nil
}

// IsBookmarked checks if an object is bookmarked by a user
func (a *StorageAdapter) IsBookmarked(ctx context.Context, username, objectID string) (bool, error) {
	// Check if user repository is set
	if a.userRepo == nil {
		return false, fmt.Errorf("user repository not initialized")
	}

	// Call the DynamORM repository
	bookmarked, err := a.userRepo.IsBookmarked(ctx, username, objectID)
	if err != nil {
		return false, MapRepositoryError(err, "IsBookmarked", "Bookmark", fmt.Sprintf("%s:%s", username, objectID))
	}

	return bookmarked, nil
}

// Account management methods

// CreateAccountPin creates a new account pin (endorsed account)
func (a *StorageAdapter) CreateAccountPin(ctx context.Context, pin *storage.AccountPin) error {
	// Check if user repository is set
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}

	err := a.userRepo.CreateAccountPin(ctx, pin)
	if err != nil {
		return MapRepositoryError(err, "CreateAccountPin", "AccountPin", pin.Username)
	}
	
	return nil
}

// DeleteAccountPin deletes an account pin
func (a *StorageAdapter) DeleteAccountPin(ctx context.Context, username, pinnedActorID string) error {
	// Check if user repository is set
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}

	err := a.userRepo.DeleteAccountPin(ctx, username, pinnedActorID)
	if err != nil {
		return MapRepositoryError(err, "DeleteAccountPin", "AccountPin", username)
	}
	
	return nil
}

// GetAccountPins retrieves all account pins for a user
func (a *StorageAdapter) GetAccountPins(ctx context.Context, username string) ([]*storage.AccountPin, error) {
	// Check if user repository is set
	if a.userRepo == nil {
		return nil, fmt.Errorf("user repository not initialized")
	}

	pins, err := a.userRepo.GetAccountPins(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetAccountPins", "AccountPin", username)
	}
	
	return pins, nil
}

// IsAccountPinned checks if an account is pinned
func (a *StorageAdapter) IsAccountPinned(ctx context.Context, username, actorID string) (bool, error) {
	// Check if user repository is set
	if a.userRepo == nil {
		return false, fmt.Errorf("user repository not initialized")
	}

	pinned, err := a.userRepo.IsAccountPinned(ctx, username, actorID)
	if err != nil {
		return false, MapRepositoryError(err, "IsAccountPinned", "AccountPin", username)
	}
	
	return pinned, nil
}

// CreateAccountNote creates a new account note
func (a *StorageAdapter) CreateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	// Check if user repository is set
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}

	err := a.userRepo.CreateAccountNote(ctx, note)
	if err != nil {
		return MapRepositoryError(err, "CreateAccountNote", "AccountNote", note.Username)
	}
	
	return nil
}

// GetAccountNote retrieves an account note
func (a *StorageAdapter) GetAccountNote(ctx context.Context, username, targetActorID string) (*storage.AccountNote, error) {
	// Check if user repository is set
	if a.userRepo == nil {
		return nil, fmt.Errorf("user repository not initialized")
	}

	note, err := a.userRepo.GetAccountNote(ctx, username, targetActorID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetAccountNote", "AccountNote", username)
	}
	
	return note, nil
}

// UpdateAccountNote updates an existing account note
func (a *StorageAdapter) UpdateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	// Check if user repository is set
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}

	err := a.userRepo.UpdateAccountNote(ctx, note)
	if err != nil {
		return MapRepositoryError(err, "UpdateAccountNote", "AccountNote", note.Username)
	}
	
	return nil
}

// DeleteAccountNote deletes an account note
func (a *StorageAdapter) DeleteAccountNote(ctx context.Context, username, targetActorID string) error {
	// Check if user repository is set
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}

	err := a.userRepo.DeleteAccountNote(ctx, username, targetActorID)
	if err != nil {
		return MapRepositoryError(err, "DeleteAccountNote", "AccountNote", username)
	}
	
	return nil
}

// RemoveFromFollowers removes a follower from a user's followers list
func (a *StorageAdapter) RemoveFromFollowers(ctx context.Context, username, followerUsername string) error {
	// Check if follow repository is set
	if a.followRepo == nil {
		return fmt.Errorf("follow repository not initialized")
	}

	// Call the DynamORM repository - this should use RemoveFollow as it's the same operation
	err := a.followRepo.RemoveFollow(ctx, followerUsername, username)
	if err != nil {
		return MapRepositoryError(err, "RemoveFromFollowers", "Follow", fmt.Sprintf("%s->%s", followerUsername, username))
	}

	return nil
}

// Trust relationship methods

// CreateTrustRelationship creates a new trust relationship
func (a *StorageAdapter) CreateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	// Check if trust repository is set
	if a.trustRepo == nil {
		return fmt.Errorf("trust repository not initialized")
	}

	err := a.trustRepo.CreateTrustRelationship(ctx, relationship)
	if err != nil {
		return MapRepositoryError(err, "CreateTrustRelationship", "TrustRelationship", relationship.TrusterID)
	}
	
	return nil
}

// GetTrustRelationship retrieves a trust relationship
func (a *StorageAdapter) GetTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error) {
	// Check if trust repository is set
	if a.trustRepo == nil {
		return nil, fmt.Errorf("trust repository not initialized")
	}

	relationship, err := a.trustRepo.GetTrustRelationship(ctx, trusterID, trusteeID, category)
	if err != nil {
		return nil, MapRepositoryError(err, "GetTrustRelationship", "TrustRelationship", trusterID)
	}
	
	return relationship, nil
}

// UpdateTrustRelationship updates an existing trust relationship
func (a *StorageAdapter) UpdateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error {
	// Check if trust repository is set
	if a.trustRepo == nil {
		return fmt.Errorf("trust repository not initialized")
	}

	err := a.trustRepo.UpdateTrustRelationship(ctx, relationship)
	if err != nil {
		return MapRepositoryError(err, "UpdateTrustRelationship", "TrustRelationship", relationship.TrusterID)
	}
	
	return nil
}

// DeleteTrustRelationship deletes a trust relationship
func (a *StorageAdapter) DeleteTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) error {
	// Check if trust repository is set
	if a.trustRepo == nil {
		return fmt.Errorf("trust repository not initialized")
	}

	err := a.trustRepo.DeleteTrustRelationship(ctx, trusterID, trusteeID, category)
	if err != nil {
		return MapRepositoryError(err, "DeleteTrustRelationship", "TrustRelationship", trusterID)
	}
	
	return nil
}

// GetTrustRelationships retrieves trust relationships for a truster
func (a *StorageAdapter) GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	// Check if trust repository is set
	if a.trustRepo == nil {
		return nil, "", fmt.Errorf("trust repository not initialized")
	}

	relationships, nextCursor, err := a.trustRepo.GetTrustRelationships(ctx, trusterID, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetTrustRelationships", "TrustRelationship", trusterID)
	}
	
	return relationships, nextCursor, nil
}

// GetTrustedByRelationships retrieves trust relationships where the actor is trusted
func (a *StorageAdapter) GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error) {
	// Check if trust repository is set
	if a.trustRepo == nil {
		return nil, "", fmt.Errorf("trust repository not initialized")
	}

	relationships, nextCursor, err := a.trustRepo.GetTrustedByRelationships(ctx, trusteeID, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetTrustedByRelationships", "TrustRelationship", trusteeID)
	}
	
	return relationships, nextCursor, nil
}

// GetTrustScore retrieves a trust score for an actor in a category
func (a *StorageAdapter) GetTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error) {
	// Check if trust repository is set
	if a.trustRepo == nil {
		return nil, fmt.Errorf("trust repository not initialized")
	}

	score, err := a.trustRepo.GetTrustScore(ctx, actorID, category)
	if err != nil {
		return nil, MapRepositoryError(err, "GetTrustScore", "TrustScore", actorID)
	}
	
	return score, nil
}

// UpdateTrustScore updates a trust score
func (a *StorageAdapter) UpdateTrustScore(ctx context.Context, score *storage.TrustScore) error {
	// Check if trust repository is set
	if a.trustRepo == nil {
		return fmt.Errorf("trust repository not initialized")
	}

	err := a.trustRepo.UpdateTrustScore(ctx, score)
	if err != nil {
		return MapRepositoryError(err, "UpdateTrustScore", "TrustScore", score.ActorID)
	}
	
	return nil
}

// RecordTrustUpdate records a trust update event
func (a *StorageAdapter) RecordTrustUpdate(ctx context.Context, update *storage.TrustUpdate) error {
	// Check if trust repository is set
	if a.trustRepo == nil {
		return fmt.Errorf("trust repository not initialized")
	}

	err := a.trustRepo.RecordTrustUpdate(ctx, update)
	if err != nil {
		return MapRepositoryError(err, "RecordTrustUpdate", "TrustUpdate", update.ActorID)
	}
	
	return nil
}

// GetAllTrustRelationships retrieves all trust relationships (admin function)
func (a *StorageAdapter) GetAllTrustRelationships(ctx context.Context, limit int) ([]*storage.TrustRelationship, error) {
	// Check if trust repository is set
	if a.trustRepo == nil {
		return nil, fmt.Errorf("trust repository not initialized")
	}

	relationships, err := a.trustRepo.GetAllTrustRelationships(ctx, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetAllTrustRelationships", "TrustRelationship", "all")
	}
	
	return relationships, nil
}

// Status pin methods
func (a *StorageAdapter) CreateStatusPin(ctx context.Context, pin *storage.StatusPin) error {
	// Check if social repository is set
	if a.socialRepo == nil {
		return fmt.Errorf("social repository not initialized")
	}

	// Call the repository method
	err := a.socialRepo.CreateStatusPin(ctx, pin)
	if err != nil {
		return MapRepositoryError(err, "CreateStatusPin", "StatusPin", fmt.Sprintf("%s:%s", pin.Username, pin.StatusID))
	}

	return nil
}

func (a *StorageAdapter) DeleteStatusPin(ctx context.Context, username, statusID string) error {
	// Check if social repository is set
	if a.socialRepo == nil {
		return fmt.Errorf("social repository not initialized")
	}

	// Call the repository method
	err := a.socialRepo.DeleteStatusPin(ctx, username, statusID)
	if err != nil {
		return MapRepositoryError(err, "DeleteStatusPin", "StatusPin", fmt.Sprintf("%s:%s", username, statusID))
	}

	return nil
}

func (a *StorageAdapter) GetStatusPins(ctx context.Context, username string) ([]*storage.StatusPin, error) {
	// Check if social repository is set
	if a.socialRepo == nil {
		return nil, fmt.Errorf("social repository not initialized")
	}

	// Call the repository method
	pins, err := a.socialRepo.GetStatusPins(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetStatusPins", "StatusPin", username)
	}

	return pins, nil
}

func (a *StorageAdapter) IsStatusPinned(ctx context.Context, username, statusID string) (bool, error) {
	// Check if social repository is set
	if a.socialRepo == nil {
		return false, fmt.Errorf("social repository not initialized")
	}

	// Call the repository method
	pinned, err := a.socialRepo.IsStatusPinned(ctx, username, statusID)
	if err != nil {
		return false, MapRepositoryError(err, "IsStatusPinned", "StatusPin", fmt.Sprintf("%s:%s", username, statusID))
	}

	return pinned, nil
}

func (a *StorageAdapter) CountUserPinnedStatuses(ctx context.Context, username string) (int, error) {
	// Check if social repository is set
	if a.socialRepo == nil {
		return 0, fmt.Errorf("social repository not initialized")
	}

	// Call the repository method
	count, err := a.socialRepo.CountUserPinnedStatuses(ctx, username)
	if err != nil {
		return 0, MapRepositoryError(err, "CountUserPinnedStatuses", "StatusPin", username)
	}

	return count, nil
}

// Additional status methods
func (a *StorageAdapter) GetStatus(ctx context.Context, statusID string) (any, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return nil, fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	status, err := a.objectRepo.GetStatus(ctx, statusID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetStatus", "Status", statusID)
	}

	return status, nil
}

func (a *StorageAdapter) GetUserStatusCount(ctx context.Context, userID string) (int, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return 0, fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	count, err := a.objectRepo.GetUserStatusCount(ctx, userID)
	if err != nil {
		return 0, MapRepositoryError(err, "GetUserStatusCount", "Status", userID)
	}

	return count, nil
}

func (a *StorageAdapter) GetStatusReplyCount(ctx context.Context, statusID string) (int, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return 0, fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	count, err := a.objectRepo.GetStatusReplyCount(ctx, statusID)
	if err != nil {
		return 0, MapRepositoryError(err, "GetStatusReplyCount", "Status", statusID)
	}

	return count, nil
}

// Instance rules methods

func (a *StorageAdapter) GetInstanceRules(ctx context.Context) ([]storage.InstanceRule, error) {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return nil, fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	rules, err := a.instanceRepo.GetInstanceRules(ctx)
	if err != nil {
		return nil, MapRepositoryError(err, "GetInstanceRules", "InstanceRule", "")
	}

	return rules, nil
}

func (a *StorageAdapter) SetInstanceRules(ctx context.Context, rules []storage.InstanceRule) error {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	err := a.instanceRepo.SetInstanceRules(ctx, rules)
	if err != nil {
		return MapRepositoryError(err, "SetInstanceRules", "InstanceRule", "")
	}

	return nil
}

func (a *StorageAdapter) GetExtendedDescription(ctx context.Context) (string, time.Time, error) {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return "", time.Time{}, fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	description, timestamp, err := a.instanceRepo.GetExtendedDescription(ctx)
	if err != nil {
		return "", time.Time{}, MapRepositoryError(err, "GetExtendedDescription", "ExtendedDescription", "")
	}

	return description, timestamp, nil
}

func (a *StorageAdapter) SetExtendedDescription(ctx context.Context, description string) error {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	err := a.instanceRepo.SetExtendedDescription(ctx, description)
	if err != nil {
		return MapRepositoryError(err, "SetExtendedDescription", "ExtendedDescription", "")
	}

	return nil
}

func (a *StorageAdapter) GetRulesByCategory(ctx context.Context, category string) ([]storage.InstanceRule, error) {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return nil, fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	rules, err := a.instanceRepo.GetRulesByCategory(ctx, category)
	if err != nil {
		return nil, MapRepositoryError(err, "GetRulesByCategory", "InstanceRule", category)
	}

	return rules, nil
}

// Instance metrics methods

func (a *StorageAdapter) GetTotalUserCount(ctx context.Context) (int64, error) {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return 0, fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	count, err := a.instanceRepo.GetTotalUserCount(ctx)
	if err != nil {
		return 0, MapRepositoryError(err, "GetTotalUserCount", "UserCount", "")
	}

	return count, nil
}

func (a *StorageAdapter) GetTotalStatusCount(ctx context.Context) (int64, error) {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return 0, fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	count, err := a.instanceRepo.GetTotalStatusCount(ctx)
	if err != nil {
		return 0, MapRepositoryError(err, "GetTotalStatusCount", "StatusCount", "")
	}

	return count, nil
}

func (a *StorageAdapter) GetTotalDomainCount(ctx context.Context) (int64, error) {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return 0, fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	count, err := a.instanceRepo.GetTotalDomainCount(ctx)
	if err != nil {
		return 0, MapRepositoryError(err, "GetTotalDomainCount", "DomainCount", "")
	}

	return count, nil
}

func (a *StorageAdapter) GetActiveUserCount(ctx context.Context, days int) (int64, error) {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return 0, fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	count, err := a.instanceRepo.GetActiveUserCount(ctx, days)
	if err != nil {
		return 0, MapRepositoryError(err, "GetActiveUserCount", "ActiveUserCount", fmt.Sprintf("days:%d", days))
	}

	return count, nil
}

func (a *StorageAdapter) GetDailyActiveUserCount(ctx context.Context) (int64, error) {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return 0, fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	count, err := a.instanceRepo.GetDailyActiveUserCount(ctx)
	if err != nil {
		return 0, MapRepositoryError(err, "GetDailyActiveUserCount", "DailyActiveUserCount", "")
	}

	return count, nil
}

func (a *StorageAdapter) GetLocalPostCount(ctx context.Context) (int64, error) {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return 0, fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	count, err := a.instanceRepo.GetLocalPostCount(ctx)
	if err != nil {
		return 0, MapRepositoryError(err, "GetLocalPostCount", "LocalPostCount", "")
	}

	return count, nil
}

func (a *StorageAdapter) GetWeeklyActivity(ctx context.Context, weekTimestamp int64) (*storage.WeeklyActivity, error) {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return nil, fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	activity, err := a.instanceRepo.GetWeeklyActivity(ctx, weekTimestamp)
	if err != nil {
		return nil, MapRepositoryError(err, "GetWeeklyActivity", "WeeklyActivity", fmt.Sprintf("week:%d", weekTimestamp))
	}

	return activity, nil
}

func (a *StorageAdapter) RecordActivity(ctx context.Context, activityType string, actorID string, timestamp time.Time) error {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	err := a.instanceRepo.RecordActivity(ctx, activityType, actorID, timestamp)
	if err != nil {
		return MapRepositoryError(err, "RecordActivity", "Activity", fmt.Sprintf("type:%s,actor:%s", activityType, actorID))
	}

	return nil
}

func (a *StorageAdapter) GetContactAccount(ctx context.Context) (*storage.ActorRecord, error) {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return nil, fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	actor, err := a.instanceRepo.GetContactAccount(ctx)
	if err != nil {
		return nil, MapRepositoryError(err, "GetContactAccount", "ContactAccount", "")
	}

	return actor, nil
}

// Storage and analytics methods

func (a *StorageAdapter) GetStorageUsage(ctx context.Context) (any, error) {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return nil, fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	usage, err := a.instanceRepo.GetStorageUsage(ctx)
	if err != nil {
		return nil, MapRepositoryError(err, "GetStorageUsage", "StorageUsage", "")
	}

	return usage, nil
}

func (a *StorageAdapter) GetStorageHistory(ctx context.Context, days int) ([]any, error) {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return nil, fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	history, err := a.instanceRepo.GetStorageHistory(ctx, days)
	if err != nil {
		return nil, MapRepositoryError(err, "GetStorageHistory", "StorageHistory", fmt.Sprintf("days:%d", days))
	}

	return history, nil
}

func (a *StorageAdapter) GetUserGrowthHistory(ctx context.Context, days int) ([]any, error) {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return nil, fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	history, err := a.instanceRepo.GetUserGrowthHistory(ctx, days)
	if err != nil {
		return nil, MapRepositoryError(err, "GetUserGrowthHistory", "UserGrowthHistory", fmt.Sprintf("days:%d", days))
	}

	return history, nil
}

func (a *StorageAdapter) GetDomainStats(ctx context.Context, domain string) (any, error) {
	// Check if instance repository is set
	if a.instanceRepo == nil {
		return nil, fmt.Errorf("instance repository not initialized")
	}

	// Call the repository method
	stats, err := a.instanceRepo.GetDomainStats(ctx, domain)
	if err != nil {
		return nil, MapRepositoryError(err, "GetDomainStats", "DomainStats", domain)
	}

	return stats, nil
}

// Announce methods
func (a *StorageAdapter) CreateAnnounce(ctx context.Context, announce *storage.Announce) error {
	// Check if social repository is set
	if a.socialRepo == nil {
		return fmt.Errorf("social repository not initialized")
	}

	// Call the repository method
	err := a.socialRepo.CreateAnnounce(ctx, announce)
	if err != nil {
		return MapRepositoryError(err, "CreateAnnounce", "Announce", fmt.Sprintf("%s->%s", announce.Actor, announce.Object))
	}

	return nil
}

func (a *StorageAdapter) GetAnnounce(ctx context.Context, actor, object string) (*storage.Announce, error) {
	// Check if social repository is set
	if a.socialRepo == nil {
		return nil, fmt.Errorf("social repository not initialized")
	}

	// Call the repository method
	announce, err := a.socialRepo.GetAnnounce(ctx, actor, object)
	if err != nil {
		return nil, MapRepositoryError(err, "GetAnnounce", "Announce", fmt.Sprintf("%s->%s", actor, object))
	}

	return announce, nil
}

func (a *StorageAdapter) DeleteAnnounce(ctx context.Context, actor, object string) error {
	// Check if social repository is set
	if a.socialRepo == nil {
		return fmt.Errorf("social repository not initialized")
	}

	// Call the repository method
	err := a.socialRepo.DeleteAnnounce(ctx, actor, object)
	if err != nil {
		return MapRepositoryError(err, "DeleteAnnounce", "Announce", fmt.Sprintf("%s->%s", actor, object))
	}

	return nil
}

func (a *StorageAdapter) GetObjectAnnounces(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	// Check if social repository is set
	if a.socialRepo == nil {
		return nil, "", fmt.Errorf("social repository not initialized")
	}

	// Call the repository method
	announces, nextCursor, err := a.socialRepo.GetStatusAnnounces(ctx, objectID, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetObjectAnnounces", "Announce", objectID)
	}

	return announces, nextCursor, nil
}

func (a *StorageAdapter) GetActorAnnounces(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	// Check if social repository is set
	if a.socialRepo == nil {
		return nil, "", fmt.Errorf("social repository not initialized")
	}

	// Call the repository method
	announces, nextCursor, err := a.socialRepo.GetActorAnnounces(ctx, actorID, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetActorAnnounces", "Announce", actorID)
	}

	return announces, nextCursor, nil
}

func (a *StorageAdapter) CountObjectAnnounces(ctx context.Context, objectID string) (int, error) {
	// Check if social repository is set
	if a.socialRepo == nil {
		return 0, fmt.Errorf("social repository not initialized")
	}

	// Call the repository method
	count, err := a.socialRepo.CountObjectAnnounces(ctx, objectID)
	if err != nil {
		return 0, MapRepositoryError(err, "CountObjectAnnounces", "Announce", objectID)
	}

	return count, nil
}

// Tombstone methods
func (a *StorageAdapter) GetTombstone(ctx context.Context, objectID string) (*storage.Tombstone, error) {
	// Check if like repository is set
	if a.likeRepo == nil {
		return nil, fmt.Errorf("like repository not initialized")
	}

	// Call the repository method
	tombstone, err := a.likeRepo.GetTombstone(ctx, objectID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetTombstone", "Tombstone", objectID)
	}

	return tombstone, nil
}

// Cascade delete methods
func (a *StorageAdapter) CascadeDeleteLikes(ctx context.Context, objectID string) error {
	// Check if like repository is set
	if a.likeRepo == nil {
		return fmt.Errorf("like repository not initialized")
	}

	// Call the repository method
	err := a.likeRepo.CascadeDeleteLikes(ctx, objectID)
	if err != nil {
		return MapRepositoryError(err, "CascadeDeleteLikes", "Like", objectID)
	}

	return nil
}

func (a *StorageAdapter) CascadeDeleteAnnounces(ctx context.Context, objectID string) error {
	// Check if social repository is set
	if a.socialRepo == nil {
		return fmt.Errorf("social repository not initialized")
	}

	// Call the repository method
	err := a.socialRepo.CascadeDeleteAnnounces(ctx, objectID)
	if err != nil {
		return MapRepositoryError(err, "CascadeDeleteAnnounces", "Announce", objectID)
	}

	return nil
}

// Boost/Like count methods
func (a *StorageAdapter) GetLikeCount(ctx context.Context, statusID string) (int64, error) {
	// Check if like repository is set
	if a.likeRepo == nil {
		return 0, fmt.Errorf("like repository not initialized")
	}

	// Call the repository method
	count, err := a.likeRepo.GetLikeCount(ctx, statusID)
	if err != nil {
		return 0, MapRepositoryError(err, "GetLikeCount", "Like", statusID)
	}

	return count, nil
}

func (a *StorageAdapter) GetBoostCount(ctx context.Context, statusID string) (int64, error) {
	// Check if like repository is set
	if a.likeRepo == nil {
		return 0, fmt.Errorf("like repository not initialized")
	}

	// Call the repository method
	count, err := a.likeRepo.GetBoostCount(ctx, statusID)
	if err != nil {
		return 0, MapRepositoryError(err, "GetBoostCount", "Boost", statusID)
	}

	return count, nil
}

func (a *StorageAdapter) IncrementReblogCount(ctx context.Context, objectID string) error {
	// Check if like repository is set
	if a.likeRepo == nil {
		return fmt.Errorf("like repository not initialized")
	}

	// Call the repository method
	err := a.likeRepo.IncrementReblogCount(ctx, objectID)
	if err != nil {
		return MapRepositoryError(err, "IncrementReblogCount", "Reblog", objectID)
	}

	return nil
}

// Reply operations
func (a *StorageAdapter) GetReplies(ctx context.Context, objectID string, limit int, cursor string) ([]any, string, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return nil, "", fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	replies, nextCursor, err := a.objectRepo.GetReplies(ctx, objectID, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetReplies", "Reply", objectID)
	}

	return replies, nextCursor, nil
}

func (a *StorageAdapter) CountReplies(ctx context.Context, objectID string) (int, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return 0, fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	count, err := a.objectRepo.CountReplies(ctx, objectID)
	if err != nil {
		return 0, MapRepositoryError(err, "CountReplies", "Reply", objectID)
	}

	return count, nil
}

func (a *StorageAdapter) IncrementReplyCount(ctx context.Context, objectID string) error {
	// Check if object repository is set
	if a.objectRepo == nil {
		return fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	err := a.objectRepo.IncrementReplyCount(ctx, objectID)
	if err != nil {
		return MapRepositoryError(err, "IncrementReplyCount", "Reply", objectID)
	}

	return nil
}

func (a *StorageAdapter) GetReplyCount(ctx context.Context, statusID string) (int64, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return 0, fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	count, err := a.objectRepo.GetReplyCount(ctx, statusID)
	if err != nil {
		return 0, MapRepositoryError(err, "GetReplyCount", "Reply", statusID)
	}

	return count, nil
}

// Thread synchronization operations for GraphQL
func (a *StorageAdapter) SyncThreadFromRemote(ctx context.Context, statusID string) (*storage.StatusSearchResult, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return nil, fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	result, err := a.objectRepo.SyncThreadFromRemote(ctx, statusID)
	if err != nil {
		return nil, MapRepositoryError(err, "SyncThreadFromRemote", "Thread", statusID)
	}

	return result, nil
}

func (a *StorageAdapter) SyncMissingRepliesFromRemote(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return nil, fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	results, err := a.objectRepo.SyncMissingRepliesFromRemote(ctx, statusID)
	if err != nil {
		return nil, MapRepositoryError(err, "SyncMissingRepliesFromRemote", "Thread", statusID)
	}

	return results, nil
}

func (a *StorageAdapter) GetThreadContext(ctx context.Context, statusID string) (*storage.ThreadContext, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return nil, fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	context, err := a.objectRepo.GetThreadContext(ctx, statusID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetThreadContext", "Thread", statusID)
	}

	return context, nil
}

func (a *StorageAdapter) MarkThreadAsSynced(ctx context.Context, statusID string) error {
	// Check if object repository is set
	if a.objectRepo == nil {
		return fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	err := a.objectRepo.MarkThreadAsSynced(ctx, statusID)
	if err != nil {
		return MapRepositoryError(err, "MarkThreadAsSynced", "Thread", statusID)
	}

	return nil
}

func (a *StorageAdapter) GetMissingReplies(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return nil, fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	results, err := a.objectRepo.GetMissingReplies(ctx, statusID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetMissingReplies", "Thread", statusID)
	}

	return results, nil
}

// Quote operations
func (a *StorageAdapter) CreateQuoteRelationship(ctx context.Context, quote *storage.QuoteRelationship) error {
	// Check if object repository is set
	if a.objectRepo == nil {
		return fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	err := a.objectRepo.CreateQuoteRelationship(ctx, quote)
	if err != nil {
		return MapRepositoryError(err, "CreateQuoteRelationship", "Quote", quote.QuoterNoteID)
	}

	return nil
}

func (a *StorageAdapter) GetQuotesForNote(ctx context.Context, noteID string, limit int, cursor string) ([]*storage.QuoteRelationship, string, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return nil, "", fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	quotes, nextCursor, err := a.objectRepo.GetQuotesForNote(ctx, noteID, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetQuotesForNote", "Quote", noteID)
	}

	return quotes, nextCursor, nil
}

func (a *StorageAdapter) IsQuoted(ctx context.Context, actorID, noteID string) (bool, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return false, fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	isQuoted, err := a.objectRepo.IsQuoted(ctx, actorID, noteID)
	if err != nil {
		return false, MapRepositoryError(err, "IsQuoted", "Quote", noteID)
	}

	return isQuoted, nil
}

func (a *StorageAdapter) WithdrawQuote(ctx context.Context, quoteNoteID string) error {
	// Check if object repository is set
	if a.objectRepo == nil {
		return fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	err := a.objectRepo.WithdrawQuote(ctx, quoteNoteID)
	if err != nil {
		return MapRepositoryError(err, "WithdrawQuote", "Quote", quoteNoteID)
	}

	return nil
}

func (a *StorageAdapter) CountQuotes(ctx context.Context, noteID string) (int, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return 0, fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	count, err := a.objectRepo.CountQuotes(ctx, noteID)
	if err != nil {
		return 0, MapRepositoryError(err, "CountQuotes", "Quote", noteID)
	}

	return count, nil
}

// Enhanced quote operations for GraphQL
func (a *StorageAdapter) WithdrawStatusFromQuotes(ctx context.Context, statusID string) error {
	// Check if object repository is set
	if a.objectRepo == nil {
		return fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	err := a.objectRepo.WithdrawStatusFromQuotes(ctx, statusID)
	if err != nil {
		return MapRepositoryError(err, "WithdrawStatusFromQuotes", "Quote", statusID)
	}

	return nil
}

func (a *StorageAdapter) UpdateQuotePermissions(ctx context.Context, statusID string, permissions *storage.QuotePermissions) error {
	// Check if object repository is set
	if a.objectRepo == nil {
		return fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	err := a.objectRepo.UpdateQuotePermissions(ctx, statusID, permissions)
	if err != nil {
		return MapRepositoryError(err, "UpdateQuotePermissions", "Quote", statusID)
	}

	return nil
}

func (a *StorageAdapter) IsQuoteAllowed(ctx context.Context, statusID, quoterID string) (bool, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return false, fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	allowed, err := a.objectRepo.IsQuoteAllowed(ctx, statusID, quoterID)
	if err != nil {
		return false, MapRepositoryError(err, "IsQuoteAllowed", "Quote", statusID)
	}

	return allowed, nil
}

func (a *StorageAdapter) GetQuoteType(ctx context.Context, statusID string) (string, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return "", fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	quoteType, err := a.objectRepo.GetQuoteType(ctx, statusID)
	if err != nil {
		return "", MapRepositoryError(err, "GetQuoteType", "Quote", statusID)
	}

	return quoteType, nil
}

func (a *StorageAdapter) IsWithdrawnFromQuotes(ctx context.Context, statusID string) (bool, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return false, fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	withdrawn, err := a.objectRepo.IsWithdrawnFromQuotes(ctx, statusID)
	if err != nil {
		return false, MapRepositoryError(err, "IsWithdrawnFromQuotes", "Quote", statusID)
	}

	return withdrawn, nil
}

func (a *StorageAdapter) GetQuotesOfStatus(ctx context.Context, statusID string, limit int) ([]*storage.StatusSearchResult, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return nil, fmt.Errorf("object repository not initialized")
	}

	// Call the repository method
	quotes, err := a.objectRepo.GetQuotesOfStatus(ctx, statusID, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetQuotesOfStatus", "Quote", statusID)
	}

	return quotes, nil
}

// Reputation storage operations
func (a *StorageAdapter) StoreReputation(ctx context.Context, actorID string, reputation *storage.Reputation) error {
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}
	
	err := a.userRepo.StoreReputation(ctx, actorID, reputation)
	if err != nil {
		return MapRepositoryError(err, "StoreReputation", "Reputation", actorID)
	}
	
	return nil
}

func (a *StorageAdapter) GetReputation(ctx context.Context, actorID string) (*storage.Reputation, error) {
	if a.userRepo == nil {
		return nil, fmt.Errorf("user repository not initialized")
	}
	
	reputation, err := a.userRepo.GetReputation(ctx, actorID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetReputation", "Reputation", actorID)
	}
	
	return reputation, nil
}

func (a *StorageAdapter) GetReputationHistory(ctx context.Context, actorID string, limit int) ([]*storage.Reputation, error) {
	if a.userRepo == nil {
		return nil, fmt.Errorf("user repository not initialized")
	}
	
	history, err := a.userRepo.GetReputationHistory(ctx, actorID, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetReputationHistory", "Reputation", actorID)
	}
	
	return history, nil
}

func (a *StorageAdapter) GetUserTrustScore(ctx context.Context, userID string) (float64, error) {
	if a.trustRepo == nil {
		return 0.0, fmt.Errorf("trust repository not initialized")
	}
	
	score, err := a.trustRepo.GetUserTrustScore(ctx, userID)
	if err != nil {
		return 0.0, MapRepositoryError(err, "GetUserTrustScore", "TrustScore", userID)
	}
	
	return score, nil
}

// Vouch operations
func (a *StorageAdapter) CreateVouch(ctx context.Context, vouch *storage.Vouch) error {
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}
	
	err := a.userRepo.CreateVouch(ctx, vouch)
	if err != nil {
		return MapRepositoryError(err, "CreateVouch", "Vouch", vouch.ID)
	}
	
	return nil
}

func (a *StorageAdapter) GetVouch(ctx context.Context, vouchID string) (*storage.Vouch, error) {
	if a.userRepo == nil {
		return nil, fmt.Errorf("user repository not initialized")
	}
	
	vouch, err := a.userRepo.GetVouch(ctx, vouchID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetVouch", "Vouch", vouchID)
	}
	
	return vouch, nil
}

func (a *StorageAdapter) GetVouchesByActor(ctx context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	if a.userRepo == nil {
		return nil, fmt.Errorf("user repository not initialized")
	}
	
	vouches, err := a.userRepo.GetVouchesByActor(ctx, actorID, activeOnly)
	if err != nil {
		return nil, MapRepositoryError(err, "GetVouchesByActor", "Vouch", actorID)
	}
	
	return vouches, nil
}

func (a *StorageAdapter) GetVouchesForActor(ctx context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error) {
	if a.userRepo == nil {
		return nil, fmt.Errorf("user repository not initialized")
	}
	
	vouches, err := a.userRepo.GetVouchesForActor(ctx, actorID, activeOnly)
	if err != nil {
		return nil, MapRepositoryError(err, "GetVouchesForActor", "Vouch", actorID)
	}
	
	return vouches, nil
}

func (a *StorageAdapter) UpdateVouchStatus(ctx context.Context, vouchID string, active bool, revokedAt *time.Time) error {
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}
	
	err := a.userRepo.UpdateVouchStatus(ctx, vouchID, active, revokedAt)
	if err != nil {
		return MapRepositoryError(err, "UpdateVouchStatus", "Vouch", vouchID)
	}
	
	return nil
}

func (a *StorageAdapter) GetMonthlyVouchCount(ctx context.Context, actorID string, year int, month time.Month) (int, error) {
	if a.userRepo == nil {
		return 0, fmt.Errorf("user repository not initialized")
	}
	
	count, err := a.userRepo.GetMonthlyVouchCount(ctx, actorID, year, month)
	if err != nil {
		return 0, MapRepositoryError(err, "GetMonthlyVouchCount", "Vouch", actorID)
	}
	
	return count, nil
}

// DNS cache operations
func (a *StorageAdapter) GetDNSCache(ctx context.Context, hostname string) (*storage.DNSCacheEntry, error) {
	if a.userRepo == nil {
		return nil, fmt.Errorf("user repository not initialized")
	}
	
	entry, err := a.userRepo.GetDNSCache(ctx, hostname)
	if err != nil {
		return nil, MapRepositoryError(err, "GetDNSCache", "DNSCache", hostname)
	}
	
	return entry, nil
}

func (a *StorageAdapter) SetDNSCache(ctx context.Context, entry *storage.DNSCacheEntry) error {
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}
	
	err := a.userRepo.SetDNSCache(ctx, entry)
	if err != nil {
		return MapRepositoryError(err, "SetDNSCache", "DNSCache", entry.Hostname)
	}
	
	return nil
}

// Cache operations for remote actors
func (a *StorageAdapter) CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}

	err := a.userRepo.CacheRemoteActor(ctx, handle, actor, ttl)
	if err != nil {
		return MapRepositoryError(err, "CacheRemoteActor", "Actor", handle)
	}

	return nil
}

// Advanced Timeline operations
func (a *StorageAdapter) DeleteExpiredTimelineEntries(ctx context.Context, before time.Time) error {
	if a.timelineRepo == nil {
		return fmt.Errorf("timeline repository not initialized")
	}

	err := a.timelineRepo.DeleteExpiredTimelineEntries(ctx, before)
	if err != nil {
		return MapRepositoryError(err, "DeleteExpiredTimelineEntries", "TimelineEntry", before.String())
	}

	return nil
}

func (a *StorageAdapter) FanOutPost(ctx context.Context, activity *activitypub.Activity) error {
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}

	err := a.userRepo.FanOutPost(ctx, activity)
	if err != nil {
		return MapRepositoryError(err, "FanOutPost", "Activity", activity.ID)
	}

	return nil
}

func (a *StorageAdapter) GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	if a.timelineRepo == nil {
		return nil, "", fmt.Errorf("timeline repository not initialized")
	}

	modelEntries, nextCursor, err := a.timelineRepo.GetListTimeline(ctx, listID, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetListTimeline", "Timeline", listID)
	}

	// Convert models.Timeline slice to storage.TimelineEntry slice
	entries := make([]*storage.TimelineEntry, len(modelEntries))
	for i, model := range modelEntries {
		entries[i] = &storage.TimelineEntry{
			TimelineType: model.TimelineType,
			TimelineID:   model.TimelineID,
			EntryID:      model.EntryID,
			PostID:       model.PostID,
			ActorID:      model.ActorID,
			ActorHandle:  model.ActorHandle,
			Content:      model.Content,
			ContentType:  model.ContentType,
			HasMedia:     model.HasMedia,
			IsReply:      model.IsReply,
			InReplyTo:    model.InReplyTo,
			IsBoost:      model.IsBoost,
			BoostedBy:    model.BoostedBy,
			Visibility:   model.Visibility,
			Language:     model.Language,
			Sensitive:    model.Sensitive,
			SpoilerText:  model.SpoilerText,
			CreatedAt:    model.CreatedAt,
			TimelineAt:   model.TimelineAt,
			ExpiresAt:    model.ExpiresAt,
		}
	}

	return entries, nextCursor, nil
}

func (a *StorageAdapter) GetDirectTimeline(ctx context.Context, username string, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	if a.timelineRepo == nil {
		return nil, "", fmt.Errorf("timeline repository not initialized")
	}

	modelEntries, nextCursor, err := a.timelineRepo.GetDirectTimeline(ctx, username, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetDirectTimeline", "Timeline", username)
	}

	// Convert models.Timeline slice to storage.TimelineEntry slice
	entries := make([]*storage.TimelineEntry, len(modelEntries))
	for i, model := range modelEntries {
		entries[i] = &storage.TimelineEntry{
			TimelineType: model.TimelineType,
			TimelineID:   model.TimelineID,
			EntryID:      model.EntryID,
			PostID:       model.PostID,
			ActorID:      model.ActorID,
			ActorHandle:  model.ActorHandle,
			Content:      model.Content,
			ContentType:  model.ContentType,
			HasMedia:     model.HasMedia,
			IsReply:      model.IsReply,
			InReplyTo:    model.InReplyTo,
			IsBoost:      model.IsBoost,
			BoostedBy:    model.BoostedBy,
			Visibility:   model.Visibility,
			Language:     model.Language,
			Sensitive:    model.Sensitive,
			SpoilerText:  model.SpoilerText,
			CreatedAt:    model.CreatedAt,
			TimelineAt:   model.TimelineAt,
			ExpiresAt:    model.ExpiresAt,
		}
	}

	return entries, nextCursor, nil
}

func (a *StorageAdapter) GetHashtagTimeline(ctx context.Context, hashtag string, local bool, limit int, cursor string) ([]*storage.TimelineEntry, string, error) {
	if a.timelineRepo == nil {
		return nil, "", fmt.Errorf("timeline repository not initialized")
	}

	modelEntries, nextCursor, err := a.timelineRepo.GetHashtagTimeline(ctx, hashtag, local, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetHashtagTimeline", "Timeline", hashtag)
	}

	// Convert models.Timeline slice to storage.TimelineEntry slice
	entries := make([]*storage.TimelineEntry, len(modelEntries))
	for i, model := range modelEntries {
		entries[i] = &storage.TimelineEntry{
			TimelineType: model.TimelineType,
			TimelineID:   model.TimelineID,
			EntryID:      model.EntryID,
			PostID:       model.PostID,
			ActorID:      model.ActorID,
			ActorHandle:  model.ActorHandle,
			Content:      model.Content,
			ContentType:  model.ContentType,
			HasMedia:     model.HasMedia,
			IsReply:      model.IsReply,
			InReplyTo:    model.InReplyTo,
			IsBoost:      model.IsBoost,
			BoostedBy:    model.BoostedBy,
			Visibility:   model.Visibility,
			Language:     model.Language,
			Sensitive:    model.Sensitive,
			SpoilerText:  model.SpoilerText,
			CreatedAt:    model.CreatedAt,
			TimelineAt:   model.TimelineAt,
			ExpiresAt:    model.ExpiresAt,
		}
	}

	return entries, nextCursor, nil
}

// User management operations
func (a *StorageAdapter) ListUsers(ctx context.Context, limit int32, cursor string) ([]*storage.User, string, error) {
	if a.userRepo == nil {
		return nil, "", fmt.Errorf("user repository not initialized")
	}

	users, nextCursor, err := a.userRepo.ListUsers(ctx, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "ListUsers", "User", "")
	}

	return users, nextCursor, nil
}

func (a *StorageAdapter) ListUsersByRole(ctx context.Context, role string) ([]*storage.User, error) {
	if a.userRepo == nil {
		return nil, fmt.Errorf("user repository not initialized")
	}

	users, err := a.userRepo.ListUsersByRole(ctx, role)
	if err != nil {
		return nil, MapRepositoryError(err, "ListUsersByRole", "User", role)
	}

	return users, nil
}

// Follow request operations
func (a *StorageAdapter) GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	if a.userRepo == nil {
		return nil, "", fmt.Errorf("user repository not initialized")
	}

	requests, nextCursor, err := a.userRepo.GetPendingFollowRequests(ctx, username, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetPendingFollowRequests", "FollowRequest", username)
	}

	return requests, nextCursor, nil
}

func (a *StorageAdapter) GetFollowRequestState(ctx context.Context, followerUsername, followedUsername string) (string, error) {
	if a.userRepo == nil {
		return "", fmt.Errorf("user repository not initialized")
	}

	state, err := a.userRepo.GetFollowRequestState(ctx, followerUsername, followedUsername)
	if err != nil {
		return "", MapRepositoryError(err, "GetFollowRequestState", "FollowRequest", followerUsername+"->"+followedUsername)
	}

	return state, nil
}

func (a *StorageAdapter) AcceptFollow(ctx context.Context, followerUsername, followedUsername string) error {
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}

	err := a.userRepo.AcceptFollow(ctx, followerUsername, followedUsername)
	if err != nil {
		return MapRepositoryError(err, "AcceptFollow", "FollowRequest", followerUsername+"->"+followedUsername)
	}

	return nil
}

func (a *StorageAdapter) RejectFollow(ctx context.Context, followerUsername, followedUsername string) error {
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}

	err := a.userRepo.RejectFollow(ctx, followerUsername, followedUsername)
	if err != nil {
		return MapRepositoryError(err, "RejectFollow", "FollowRequest", followerUsername+"->"+followedUsername)
	}

	return nil
}

// Remote actor cache operations
func (a *StorageAdapter) GetCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error) {
	if a.actorRepo == nil {
		return nil, fmt.Errorf("actor repository not initialized")
	}

	actor, err := a.actorRepo.GetCachedRemoteActor(ctx, handle)
	if err != nil {
		return nil, MapRepositoryError(err, "GetCachedRemoteActor", "Actor", handle)
	}

	return actor, nil
}

// Account suggestions
func (a *StorageAdapter) GetAccountSuggestions(ctx context.Context, userID string, limit int) ([]*activitypub.Actor, error) {
	if a.actorRepo == nil {
		return nil, fmt.Errorf("actor repository not initialized")
	}

	actors, err := a.actorRepo.GetAccountSuggestions(ctx, userID, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetAccountSuggestions", "Actor", userID)
	}

	return actors, nil
}

func (a *StorageAdapter) RemoveAccountSuggestion(ctx context.Context, userID, targetID string) error {
	if a.actorRepo == nil {
		return fmt.Errorf("actor repository not initialized")
	}

	err := a.actorRepo.RemoveAccountSuggestion(ctx, userID, targetID)
	if err != nil {
		return MapRepositoryError(err, "RemoveAccountSuggestion", "Actor", userID+"->"+targetID)
	}

	return nil
}

// Relationship operations
func (a *StorageAdapter) GetFollowRequest(ctx context.Context, followerID, targetID string) (*storage.RelationshipRecord, error) {
	if a.relationshipRepo == nil {
		return nil, fmt.Errorf("relationship repository not initialized")
	}

	relationship, err := a.relationshipRepo.GetFollowRequest(ctx, followerID, targetID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetFollowRequest", "Relationship", followerID+"->"+targetID)
	}

	return relationship, nil
}

func (a *StorageAdapter) AcceptFollowRequest(ctx context.Context, followerID, targetID string) error {
	if a.relationshipRepo == nil {
		return fmt.Errorf("relationship repository not initialized")
	}

	err := a.relationshipRepo.AcceptFollowRequest(ctx, followerID, targetID)
	if err != nil {
		return MapRepositoryError(err, "AcceptFollowRequest", "Relationship", followerID+"->"+targetID)
	}

	return nil
}

func (a *StorageAdapter) RejectFollowRequest(ctx context.Context, followerID, targetID string) error {
	if a.relationshipRepo == nil {
		return fmt.Errorf("relationship repository not initialized")
	}

	err := a.relationshipRepo.RejectFollowRequest(ctx, followerID, targetID)
	if err != nil {
		return MapRepositoryError(err, "RejectFollowRequest", "Relationship", followerID+"->"+targetID)
	}

	return nil
}

func (a *StorageAdapter) HasFollowRequest(ctx context.Context, requesterID, targetID string) (bool, error) {
	if a.relationshipRepo == nil {
		return false, fmt.Errorf("relationship repository not initialized")
	}

	hasRequest, err := a.relationshipRepo.HasFollowRequest(ctx, requesterID, targetID)
	if err != nil {
		return false, MapRepositoryError(err, "HasFollowRequest", "Relationship", requesterID+"->"+targetID)
	}

	return hasRequest, nil
}

func (a *StorageAdapter) IsEndorsed(ctx context.Context, userID, targetID string) (bool, error) {
	if a.relationshipRepo == nil {
		return false, fmt.Errorf("relationship repository not initialized")
	}

	isEndorsed, err := a.relationshipRepo.IsEndorsed(ctx, userID, targetID)
	if err != nil {
		return false, MapRepositoryError(err, "IsEndorsed", "Relationship", userID+"->"+targetID)
	}

	return isEndorsed, nil
}

func (a *StorageAdapter) GetRelationshipNote(ctx context.Context, userID, targetID string) (*storage.AccountNote, error) {
	if a.relationshipRepo == nil {
		return nil, fmt.Errorf("relationship repository not initialized")
	}

	note, err := a.relationshipRepo.GetRelationshipNote(ctx, userID, targetID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetRelationshipNote", "Relationship", userID+"->"+targetID)
	}

	return note, nil
}

// Featured Tag adapter methods

func (a *StorageAdapter) CreateFeaturedTag(ctx context.Context, userID string, tagName string) (*storage.FeaturedTag, error) {
	if a.featuredTagRepo == nil {
		return nil, fmt.Errorf("featured tag repository not initialized")
	}

	// Create the featured tag object
	tag := &storage.FeaturedTag{
		ID:           fmt.Sprintf("%s-%s", userID, tagName),
		Username:     userID,
		Name:         tagName,
		LastStatusAt: time.Now().Format(time.RFC3339),
		CreatedAt:    time.Now(),
	}

	err := a.featuredTagRepo.CreateFeaturedTag(ctx, tag)
	if err != nil {
		return nil, MapRepositoryError(err, "CreateFeaturedTag", "FeaturedTag", tag.ID)
	}

	return tag, nil
}

func (a *StorageAdapter) DeleteFeaturedTag(ctx context.Context, username, name string) error {
	if a.featuredTagRepo == nil {
		return fmt.Errorf("featured tag repository not initialized")
	}

	err := a.featuredTagRepo.DeleteFeaturedTag(ctx, username, name)
	if err != nil {
		return MapRepositoryError(err, "DeleteFeaturedTag", "FeaturedTag", fmt.Sprintf("%s:%s", username, name))
	}

	return nil
}

func (a *StorageAdapter) GetFeaturedTags(ctx context.Context, username string) ([]*storage.FeaturedTag, error) {
	if a.featuredTagRepo == nil {
		return nil, fmt.Errorf("featured tag repository not initialized")
	}

	tags, err := a.featuredTagRepo.GetFeaturedTags(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetFeaturedTags", "FeaturedTag", username)
	}

	return tags, nil
}

func (a *StorageAdapter) GetTagSuggestions(ctx context.Context, username string, limit int) ([]string, error) {
	if a.featuredTagRepo == nil {
		return nil, fmt.Errorf("featured tag repository not initialized")
	}

	suggestions, err := a.featuredTagRepo.GetTagSuggestions(ctx, username, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetTagSuggestions", "FeaturedTag", username)
	}

	return suggestions, nil
}

// ==================== Scheduled Status Methods ====================

// CreateScheduledStatus creates a new scheduled status
func (a *StorageAdapter) CreateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	if a.scheduledStatusRepo == nil {
		return fmt.Errorf("scheduled status repository not initialized")
	}

	err := a.scheduledStatusRepo.CreateScheduledStatus(ctx, scheduled)
	if err != nil {
		return MapRepositoryError(err, "CreateScheduledStatus", "ScheduledStatus", scheduled.ID)
	}

	return nil
}

// GetScheduledStatus retrieves a scheduled status by ID
func (a *StorageAdapter) GetScheduledStatus(ctx context.Context, id string) (*storage.ScheduledStatus, error) {
	if a.scheduledStatusRepo == nil {
		return nil, fmt.Errorf("scheduled status repository not initialized")
	}

	scheduled, err := a.scheduledStatusRepo.GetScheduledStatus(ctx, id)
	if err != nil {
		return nil, MapRepositoryError(err, "GetScheduledStatus", "ScheduledStatus", id)
	}

	return scheduled, nil
}

// GetScheduledStatuses retrieves scheduled statuses for a user
func (a *StorageAdapter) GetScheduledStatuses(ctx context.Context, username string, limit int, cursor string) ([]*storage.ScheduledStatus, string, error) {
	if a.scheduledStatusRepo == nil {
		return nil, "", fmt.Errorf("scheduled status repository not initialized")
	}

	statuses, nextCursor, err := a.scheduledStatusRepo.GetScheduledStatuses(ctx, username, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetScheduledStatuses", "ScheduledStatus", username)
	}

	return statuses, nextCursor, nil
}

// UpdateScheduledStatus updates a scheduled status
func (a *StorageAdapter) UpdateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	if a.scheduledStatusRepo == nil {
		return fmt.Errorf("scheduled status repository not initialized")
	}

	err := a.scheduledStatusRepo.UpdateScheduledStatus(ctx, scheduled)
	if err != nil {
		return MapRepositoryError(err, "UpdateScheduledStatus", "ScheduledStatus", scheduled.ID)
	}

	return nil
}

// DeleteScheduledStatus deletes a scheduled status
func (a *StorageAdapter) DeleteScheduledStatus(ctx context.Context, id string) error {
	if a.scheduledStatusRepo == nil {
		return fmt.Errorf("scheduled status repository not initialized")
	}

	err := a.scheduledStatusRepo.DeleteScheduledStatus(ctx, id)
	if err != nil {
		return MapRepositoryError(err, "DeleteScheduledStatus", "ScheduledStatus", id)
	}

	return nil
}

// GetDueScheduledStatuses retrieves scheduled statuses that are due to be published
func (a *StorageAdapter) GetDueScheduledStatuses(ctx context.Context, before time.Time, limit int) ([]*storage.ScheduledStatus, error) {
	if a.scheduledStatusRepo == nil {
		return nil, fmt.Errorf("scheduled status repository not initialized")
	}

	statuses, err := a.scheduledStatusRepo.GetDueScheduledStatuses(ctx, before, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetDueScheduledStatuses", "ScheduledStatus", before.String())
	}

	return statuses, nil
}

// MarkScheduledStatusPublished marks a scheduled status as published
func (a *StorageAdapter) MarkScheduledStatusPublished(ctx context.Context, id string) error {
	if a.scheduledStatusRepo == nil {
		return fmt.Errorf("scheduled status repository not initialized")
	}

	err := a.scheduledStatusRepo.MarkScheduledStatusPublished(ctx, id)
	if err != nil {
		return MapRepositoryError(err, "MarkScheduledStatusPublished", "ScheduledStatus", id)
	}

	return nil
}

// GetScheduledStatusMedia gets media for scheduled status
func (a *StorageAdapter) GetScheduledStatusMedia(ctx context.Context, statusID string) ([]any, error) {
	if a.scheduledStatusRepo == nil {
		return nil, fmt.Errorf("scheduled status repository not initialized")
	}

	media, err := a.scheduledStatusRepo.GetScheduledStatusMedia(ctx, statusID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetScheduledStatusMedia", "ScheduledStatus", statusID)
	}

	return media, nil
}

// User Preferences Methods

// GetUserLanguagePreference retrieves a user's preferred language
func (a *StorageAdapter) GetUserLanguagePreference(ctx context.Context, username string) (string, error) {
	if a.userRepo == nil {
		return "", fmt.Errorf("user repository not initialized")
	}
	
	language, err := a.userRepo.GetUserLanguagePreference(ctx, username)
	if err != nil {
		return "", MapRepositoryError(err, "GetUserLanguagePreference", "UserPreferences", username)
	}
	
	return language, nil
}

// SetUserLanguagePreference updates a user's preferred language
func (a *StorageAdapter) SetUserLanguagePreference(ctx context.Context, username, language string) error {
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}
	
	err := a.userRepo.SetUserLanguagePreference(ctx, username, language)
	if err != nil {
		return MapRepositoryError(err, "SetUserLanguagePreference", "UserPreferences", username)
	}
	
	return nil
}

// GetUserPreferences retrieves all user preferences
func (a *StorageAdapter) GetUserPreferences(ctx context.Context, username string) (*storage.UserPreferences, error) {
	if a.userRepo == nil {
		return nil, fmt.Errorf("user repository not initialized")
	}
	
	preferences, err := a.userRepo.GetUserPreferences(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetUserPreferences", "UserPreferences", username)
	}
	
	return preferences, nil
}

// UpdateUserPreferences updates user preferences
func (a *StorageAdapter) UpdateUserPreferences(ctx context.Context, username string, preferences *storage.UserPreferences) error {
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}
	
	err := a.userRepo.UpdateUserPreferences(ctx, username, preferences)
	if err != nil {
		return MapRepositoryError(err, "UpdateUserPreferences", "UserPreferences", username)
	}
	
	return nil
}

// SetPreference sets a specific preference key-value pair
func (a *StorageAdapter) SetPreference(ctx context.Context, username, key string, value any) error {
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}
	
	err := a.userRepo.SetPreference(ctx, username, key, value)
	if err != nil {
		return MapRepositoryError(err, "SetPreference", "UserPreferences", fmt.Sprintf("%s:%s", username, key))
	}
	
	return nil
}

// GetPreference gets a specific preference value
func (a *StorageAdapter) GetPreference(ctx context.Context, username, key string) (any, error) {
	if a.userRepo == nil {
		return nil, fmt.Errorf("user repository not initialized")
	}
	
	value, err := a.userRepo.GetPreference(ctx, username, key)
	if err != nil {
		return nil, MapRepositoryError(err, "GetPreference", "UserPreferences", fmt.Sprintf("%s:%s", username, key))
	}
	
	return value, nil
}

// GetAllPreferences gets all preferences as a map
func (a *StorageAdapter) GetAllPreferences(ctx context.Context, username string) (map[string]any, error) {
	if a.userRepo == nil {
		return nil, fmt.Errorf("user repository not initialized")
	}
	
	preferences, err := a.userRepo.GetAllPreferences(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetAllPreferences", "UserPreferences", username)
	}
	
	return preferences, nil
}

// UpdatePreferences updates multiple preferences at once
func (a *StorageAdapter) UpdatePreferences(ctx context.Context, username string, preferences map[string]any) error {
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}
	
	err := a.userRepo.UpdatePreferences(ctx, username, preferences)
	if err != nil {
		return MapRepositoryError(err, "UpdatePreferences", "UserPreferences", username)
	}
	
	return nil
}

// SaveMarker saves or updates a timeline position marker
func (a *StorageAdapter) SaveMarker(ctx context.Context, username, timeline string, lastReadID string, version int) error {
	if a.markerRepo == nil {
		return fmt.Errorf("marker repository not initialized")
	}

	// Call the repository directly with the legacy signature
	err := a.markerRepo.SaveMarker(ctx, username, timeline, lastReadID, version)
	if err != nil {
		return MapRepositoryError(err, "SaveMarker", "Marker", fmt.Sprintf("%s/%s", username, timeline))
	}

	return nil
}

// GetMarkers retrieves timeline position markers for specified timelines
func (a *StorageAdapter) GetMarkers(ctx context.Context, username string, timelines []string) (map[string]*storage.Marker, error) {
	if a.markerRepo == nil {
		return nil, fmt.Errorf("marker repository not initialized")
	}

	// Call the repository directly with the legacy signature
	markers, err := a.markerRepo.GetMarkers(ctx, username, timelines)
	if err != nil {
		return nil, MapRepositoryError(err, "GetMarkers", "Marker", username)
	}

	return markers, nil
}

// Trending and Analytics Operations

// RecordHashtagUsage records hashtag usage in a status
func (a *StorageAdapter) RecordHashtagUsage(ctx context.Context, hashtag string, statusID string, authorID string) error {
	if a.trendingRepo == nil {
		return fmt.Errorf("trending repository not initialized")
	}

	err := a.trendingRepo.RecordHashtagUsage(ctx, hashtag, statusID, authorID)
	if err != nil {
		return MapRepositoryError(err, "RecordHashtagUsage", "HashtagUsage", hashtag)
	}

	return nil
}

// RecordStatusEngagement records engagement on a status
func (a *StorageAdapter) RecordStatusEngagement(ctx context.Context, statusID string, engagementType string, userID string) error {
	if a.trendingRepo == nil {
		return fmt.Errorf("trending repository not initialized")
	}

	err := a.trendingRepo.RecordStatusEngagement(ctx, statusID, engagementType, userID)
	if err != nil {
		return MapRepositoryError(err, "RecordStatusEngagement", "StatusEngagement", statusID)
	}

	return nil
}

// RecordLinkShare records link sharing in a status
func (a *StorageAdapter) RecordLinkShare(ctx context.Context, url string, statusID string, authorID string) error {
	if a.trendingRepo == nil {
		return fmt.Errorf("trending repository not initialized")
	}

	err := a.trendingRepo.RecordLinkShare(ctx, url, statusID, authorID)
	if err != nil {
		return MapRepositoryError(err, "RecordLinkShare", "LinkShare", url)
	}

	return nil
}

// GetTrendingHashtags returns trending hashtags since a specific time
func (a *StorageAdapter) GetTrendingHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	if a.trendingRepo == nil {
		return nil, fmt.Errorf("trending repository not initialized")
	}

	hashtags, err := a.trendingRepo.GetTrendingHashtags(ctx, since, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetTrendingHashtags", "TrendingHashtag", "")
	}

	return hashtags, nil
}

// GetTrendingStatuses returns trending statuses since a specific time
func (a *StorageAdapter) GetTrendingStatuses(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error) {
	if a.trendingRepo == nil {
		return nil, fmt.Errorf("trending repository not initialized")
	}

	statuses, err := a.trendingRepo.GetTrendingStatuses(ctx, since, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetTrendingStatuses", "TrendingStatus", "")
	}

	return statuses, nil
}

// GetTrendingLinks returns trending links since a specific time
func (a *StorageAdapter) GetTrendingLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	if a.trendingRepo == nil {
		return nil, fmt.Errorf("trending repository not initialized")
	}

	links, err := a.trendingRepo.GetTrendingLinks(ctx, since, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetTrendingLinks", "TrendingLink", "")
	}

	return links, nil
}

// GetRecentHashtags returns recent hashtags since a specific time
func (a *StorageAdapter) GetRecentHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	if a.trendingRepo == nil {
		return nil, fmt.Errorf("trending repository not initialized")
	}

	hashtags, err := a.trendingRepo.GetRecentHashtags(ctx, since, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetRecentHashtags", "TrendingHashtag", "")
	}

	return hashtags, nil
}

// GetRecentStatusesWithEngagement returns recent statuses with engagement since a specific time
func (a *StorageAdapter) GetRecentStatusesWithEngagement(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error) {
	if a.trendingRepo == nil {
		return nil, fmt.Errorf("trending repository not initialized")
	}

	statuses, err := a.trendingRepo.GetRecentStatusesWithEngagement(ctx, since, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetRecentStatusesWithEngagement", "TrendingStatus", "")
	}

	return statuses, nil
}

// GetRecentLinks returns recent links since a specific time
func (a *StorageAdapter) GetRecentLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	if a.trendingRepo == nil {
		return nil, fmt.Errorf("trending repository not initialized")
	}

	links, err := a.trendingRepo.GetRecentLinks(ctx, since, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetRecentLinks", "TrendingLink", "")
	}

	return links, nil
}

// StoreEngagementMetrics stores engagement metrics for a status
func (a *StorageAdapter) StoreEngagementMetrics(ctx context.Context, metrics *storage.EngagementMetrics) error {
	if a.trendingRepo == nil {
		return fmt.Errorf("trending repository not initialized")
	}

	err := a.trendingRepo.StoreEngagementMetrics(ctx, metrics)
	if err != nil {
		return MapRepositoryError(err, "StoreEngagementMetrics", "EngagementMetrics", metrics.StatusID)
	}

	return nil
}

// GetEngagementMetrics retrieves stored engagement metrics for a status
func (a *StorageAdapter) GetEngagementMetrics(ctx context.Context, statusID string) (*storage.EngagementMetrics, error) {
	if a.trendingRepo == nil {
		return nil, fmt.Errorf("trending repository not initialized")
	}

	metrics, err := a.trendingRepo.GetEngagementMetrics(ctx, statusID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetEngagementMetrics", "EngagementMetrics", statusID)
	}

	return metrics, nil
}

// Announcement operations

// CreateAnnouncement creates a new announcement
func (a *StorageAdapter) CreateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	if a.announcementRepo == nil {
		return fmt.Errorf("announcement repository not initialized")
	}

	err := a.announcementRepo.CreateAnnouncement(ctx, announcement)
	if err != nil {
		return MapRepositoryError(err, "CreateAnnouncement", "Announcement", announcement.ID)
	}

	return nil
}

// GetAnnouncement retrieves a single announcement by ID
func (a *StorageAdapter) GetAnnouncement(ctx context.Context, id string) (*storage.Announcement, error) {
	if a.announcementRepo == nil {
		return nil, fmt.Errorf("announcement repository not initialized")
	}

	announcement, err := a.announcementRepo.GetAnnouncement(ctx, id)
	if err != nil {
		return nil, MapRepositoryError(err, "GetAnnouncement", "Announcement", id)
	}

	return announcement, nil
}

// GetAnnouncements retrieves all announcements (active or all)
func (a *StorageAdapter) GetAnnouncements(ctx context.Context, active bool) ([]*storage.Announcement, error) {
	if a.announcementRepo == nil {
		return nil, fmt.Errorf("announcement repository not initialized")
	}

	announcements, err := a.announcementRepo.GetAnnouncements(ctx, active)
	if err != nil {
		return nil, MapRepositoryError(err, "GetAnnouncements", "Announcement", fmt.Sprintf("active=%v", active))
	}

	return announcements, nil
}

// UpdateAnnouncement updates an existing announcement
func (a *StorageAdapter) UpdateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	if a.announcementRepo == nil {
		return fmt.Errorf("announcement repository not initialized")
	}

	err := a.announcementRepo.UpdateAnnouncement(ctx, announcement)
	if err != nil {
		return MapRepositoryError(err, "UpdateAnnouncement", "Announcement", announcement.ID)
	}

	return nil
}

// DeleteAnnouncement deletes an announcement
func (a *StorageAdapter) DeleteAnnouncement(ctx context.Context, id string) error {
	if a.announcementRepo == nil {
		return fmt.Errorf("announcement repository not initialized")
	}

	err := a.announcementRepo.DeleteAnnouncement(ctx, id)
	if err != nil {
		return MapRepositoryError(err, "DeleteAnnouncement", "Announcement", id)
	}

	return nil
}

// DismissAnnouncement marks an announcement as dismissed by a user
func (a *StorageAdapter) DismissAnnouncement(ctx context.Context, username, announcementID string) error {
	if a.announcementRepo == nil {
		return fmt.Errorf("announcement repository not initialized")
	}

	err := a.announcementRepo.DismissAnnouncement(ctx, username, announcementID)
	if err != nil {
		return MapRepositoryError(err, "DismissAnnouncement", "Announcement", fmt.Sprintf("%s:%s", username, announcementID))
	}

	return nil
}

// IsDismissed checks if a user has dismissed an announcement
func (a *StorageAdapter) IsDismissed(ctx context.Context, username, announcementID string) (bool, error) {
	if a.announcementRepo == nil {
		return false, fmt.Errorf("announcement repository not initialized")
	}

	dismissed, err := a.announcementRepo.IsDismissed(ctx, username, announcementID)
	if err != nil {
		return false, MapRepositoryError(err, "IsDismissed", "Announcement", fmt.Sprintf("%s:%s", username, announcementID))
	}

	return dismissed, nil
}

// GetDismissedAnnouncements gets all announcement IDs dismissed by a user
func (a *StorageAdapter) GetDismissedAnnouncements(ctx context.Context, username string) ([]string, error) {
	if a.announcementRepo == nil {
		return nil, fmt.Errorf("announcement repository not initialized")
	}

	announcementIDs, err := a.announcementRepo.GetDismissedAnnouncements(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetDismissedAnnouncements", "Announcement", username)
	}

	return announcementIDs, nil
}

// AddAnnouncementReaction adds a user's reaction to an announcement
func (a *StorageAdapter) AddAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error {
	if a.announcementRepo == nil {
		return fmt.Errorf("announcement repository not initialized")
	}

	err := a.announcementRepo.AddAnnouncementReaction(ctx, username, announcementID, emojiName)
	if err != nil {
		return MapRepositoryError(err, "AddAnnouncementReaction", "Announcement", fmt.Sprintf("%s:%s:%s", username, announcementID, emojiName))
	}

	return nil
}

// RemoveAnnouncementReaction removes a user's reaction from an announcement
func (a *StorageAdapter) RemoveAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error {
	if a.announcementRepo == nil {
		return fmt.Errorf("announcement repository not initialized")
	}

	err := a.announcementRepo.RemoveAnnouncementReaction(ctx, username, announcementID, emojiName)
	if err != nil {
		return MapRepositoryError(err, "RemoveAnnouncementReaction", "Announcement", fmt.Sprintf("%s:%s:%s", username, announcementID, emojiName))
	}

	return nil
}

// GetAnnouncementReactions gets all reactions for an announcement
func (a *StorageAdapter) GetAnnouncementReactions(ctx context.Context, announcementID string) (map[string][]string, error) {
	if a.announcementRepo == nil {
		return nil, fmt.Errorf("announcement repository not initialized")
	}

	reactions, err := a.announcementRepo.GetAnnouncementReactions(ctx, announcementID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetAnnouncementReactions", "Announcement", announcementID)
	}

	return reactions, nil
}

// CreateCustomEmoji creates a new custom emoji
func (a *StorageAdapter) CreateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error {
	if a.emojiRepo == nil {
		return fmt.Errorf("emoji repository not initialized")
	}

	err := a.emojiRepo.CreateCustomEmoji(ctx, emoji)
	if err != nil {
		return MapRepositoryError(err, "CreateCustomEmoji", "CustomEmoji", emoji.Shortcode)
	}

	return nil
}

// GetCustomEmoji retrieves a custom emoji by shortcode
func (a *StorageAdapter) GetCustomEmoji(ctx context.Context, shortcode string) (*storage.CustomEmoji, error) {
	if a.emojiRepo == nil {
		return nil, fmt.Errorf("emoji repository not initialized")
	}

	emoji, err := a.emojiRepo.GetCustomEmoji(ctx, shortcode)
	if err != nil {
		return nil, MapRepositoryError(err, "GetCustomEmoji", "CustomEmoji", shortcode)
	}

	return emoji, nil
}

// GetCustomEmojis retrieves all custom emojis
func (a *StorageAdapter) GetCustomEmojis(ctx context.Context) ([]*storage.CustomEmoji, error) {
	if a.emojiRepo == nil {
		return nil, fmt.Errorf("emoji repository not initialized")
	}

	emojis, err := a.emojiRepo.GetCustomEmojis(ctx)
	if err != nil {
		return nil, MapRepositoryError(err, "GetCustomEmojis", "CustomEmoji", "all")
	}

	return emojis, nil
}

// UpdateCustomEmoji updates an existing custom emoji
func (a *StorageAdapter) UpdateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error {
	if a.emojiRepo == nil {
		return fmt.Errorf("emoji repository not initialized")
	}

	err := a.emojiRepo.UpdateCustomEmoji(ctx, emoji)
	if err != nil {
		return MapRepositoryError(err, "UpdateCustomEmoji", "CustomEmoji", emoji.Shortcode)
	}

	return nil
}

// DeleteCustomEmoji deletes a custom emoji
func (a *StorageAdapter) DeleteCustomEmoji(ctx context.Context, shortcode string) error {
	if a.emojiRepo == nil {
		return fmt.Errorf("emoji repository not initialized")
	}

	err := a.emojiRepo.DeleteCustomEmoji(ctx, shortcode)
	if err != nil {
		return MapRepositoryError(err, "DeleteCustomEmoji", "CustomEmoji", shortcode)
	}

	return nil
}

// GetCustomEmojisByCategory retrieves custom emojis by category
func (a *StorageAdapter) GetCustomEmojisByCategory(ctx context.Context, category string) ([]*storage.CustomEmoji, error) {
	if a.emojiRepo == nil {
		return nil, fmt.Errorf("emoji repository not initialized")
	}

	emojis, err := a.emojiRepo.GetCustomEmojisByCategory(ctx, category)
	if err != nil {
		return nil, MapRepositoryError(err, "GetCustomEmojisByCategory", "CustomEmoji", category)
	}

	return emojis, nil
}

// Domain Block Operations

// AddDomainBlock adds a domain to the user's block list
func (a *StorageAdapter) AddDomainBlock(ctx context.Context, username, domain string) error {
	if a.domainBlockRepo == nil {
		return fmt.Errorf("domain block repository not initialized")
	}

	err := a.domainBlockRepo.AddDomainBlock(ctx, username, domain)
	if err != nil {
		return MapRepositoryError(err, "AddDomainBlock", "DomainBlock", fmt.Sprintf("%s#%s", username, domain))
	}

	return nil
}

// RemoveDomainBlock removes a domain from the user's block list
func (a *StorageAdapter) RemoveDomainBlock(ctx context.Context, username, domain string) error {
	if a.domainBlockRepo == nil {
		return fmt.Errorf("domain block repository not initialized")
	}

	err := a.domainBlockRepo.RemoveDomainBlock(ctx, username, domain)
	if err != nil {
		return MapRepositoryError(err, "RemoveDomainBlock", "DomainBlock", fmt.Sprintf("%s#%s", username, domain))
	}

	return nil
}

// GetUserDomainBlocks retrieves all domains blocked by a user
func (a *StorageAdapter) GetUserDomainBlocks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	if a.domainBlockRepo == nil {
		return nil, "", fmt.Errorf("domain block repository not initialized")
	}

	domains, nextCursor, err := a.domainBlockRepo.GetUserDomainBlocks(ctx, username, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetUserDomainBlocks", "DomainBlock", username)
	}

	return domains, nextCursor, nil
}

// IsBlockedDomain checks if a domain is blocked by a user
func (a *StorageAdapter) IsBlockedDomain(ctx context.Context, username, domain string) (bool, error) {
	if a.domainBlockRepo == nil {
		return false, fmt.Errorf("domain block repository not initialized")
	}

	isBlocked, err := a.domainBlockRepo.IsBlockedDomain(ctx, username, domain)
	if err != nil {
		return false, MapRepositoryError(err, "IsBlockedDomain", "DomainBlock", fmt.Sprintf("%s#%s", username, domain))
	}

	return isBlocked, nil
}

// CreateInstanceDomainBlock creates an instance-level domain block
func (a *StorageAdapter) CreateInstanceDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error {
	if a.domainBlockRepo == nil {
		return fmt.Errorf("domain block repository not initialized")
	}

	err := a.domainBlockRepo.CreateInstanceDomainBlock(ctx, block)
	if err != nil {
		return MapRepositoryError(err, "CreateInstanceDomainBlock", "InstanceDomainBlock", block.Domain)
	}

	return nil
}

// GetInstanceDomainBlock retrieves a domain block by domain
func (a *StorageAdapter) GetInstanceDomainBlock(ctx context.Context, domain string) (*storage.InstanceDomainBlock, error) {
	if a.domainBlockRepo == nil {
		return nil, fmt.Errorf("domain block repository not initialized")
	}

	block, err := a.domainBlockRepo.GetInstanceDomainBlock(ctx, domain)
	if err != nil {
		return nil, MapRepositoryError(err, "GetInstanceDomainBlock", "InstanceDomainBlock", domain)
	}

	return block, nil
}

// GetInstanceDomainBlockByID retrieves a domain block by ID
func (a *StorageAdapter) GetInstanceDomainBlockByID(ctx context.Context, id string) (*storage.InstanceDomainBlock, error) {
	if a.domainBlockRepo == nil {
		return nil, fmt.Errorf("domain block repository not initialized")
	}

	block, err := a.domainBlockRepo.GetInstanceDomainBlockByID(ctx, id)
	if err != nil {
		return nil, MapRepositoryError(err, "GetInstanceDomainBlockByID", "InstanceDomainBlock", id)
	}

	return block, nil
}

// ListInstanceDomainBlocks lists all instance domain blocks with pagination
func (a *StorageAdapter) ListInstanceDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	if a.domainBlockRepo == nil {
		return nil, "", fmt.Errorf("domain block repository not initialized")
	}

	blocks, nextCursor, err := a.domainBlockRepo.ListInstanceDomainBlocks(ctx, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "ListInstanceDomainBlocks", "InstanceDomainBlock", "all")
	}

	return blocks, nextCursor, nil
}

// UpdateInstanceDomainBlock updates an existing domain block
func (a *StorageAdapter) UpdateInstanceDomainBlock(ctx context.Context, domain string, updates map[string]any) error {
	if a.domainBlockRepo == nil {
		return fmt.Errorf("domain block repository not initialized")
	}

	err := a.domainBlockRepo.UpdateInstanceDomainBlock(ctx, domain, updates)
	if err != nil {
		return MapRepositoryError(err, "UpdateInstanceDomainBlock", "InstanceDomainBlock", domain)
	}

	return nil
}

// DeleteInstanceDomainBlock deletes a domain block
func (a *StorageAdapter) DeleteInstanceDomainBlock(ctx context.Context, domain string) error {
	if a.domainBlockRepo == nil {
		return fmt.Errorf("domain block repository not initialized")
	}

	err := a.domainBlockRepo.DeleteInstanceDomainBlock(ctx, domain)
	if err != nil {
		return MapRepositoryError(err, "DeleteInstanceDomainBlock", "InstanceDomainBlock", domain)
	}

	return nil
}

// IsInstanceDomainBlocked checks if a domain is blocked at the instance level
func (a *StorageAdapter) IsInstanceDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	if a.domainBlockRepo == nil {
		return false, nil, fmt.Errorf("domain block repository not initialized")
	}

	blocked, block, err := a.domainBlockRepo.IsInstanceDomainBlocked(ctx, domain)
	if err != nil {
		return false, nil, MapRepositoryError(err, "IsInstanceDomainBlocked", "InstanceDomainBlock", domain)
	}

	return blocked, block, nil
}

// GetDomainBlocks retrieves instance-level domain blocks with pagination
func (a *StorageAdapter) GetDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	if a.domainBlockRepo == nil {
		return nil, "", fmt.Errorf("domain block repository not initialized")
	}

	blocks, nextCursor, err := a.domainBlockRepo.GetDomainBlocks(ctx, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetDomainBlocks", "InstanceDomainBlock", "all")
	}

	return blocks, nextCursor, nil
}

// GetDomainBlock retrieves a specific domain block by ID
func (a *StorageAdapter) GetDomainBlock(ctx context.Context, id string) (*storage.InstanceDomainBlock, error) {
	if a.domainBlockRepo == nil {
		return nil, fmt.Errorf("domain block repository not initialized")
	}

	block, err := a.domainBlockRepo.GetDomainBlock(ctx, id)
	if err != nil {
		return nil, MapRepositoryError(err, "GetDomainBlock", "InstanceDomainBlock", id)
	}

	return block, nil
}

// CreateDomainBlock creates a new instance-level domain block
func (a *StorageAdapter) CreateDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error {
	if a.domainBlockRepo == nil {
		return fmt.Errorf("domain block repository not initialized")
	}

	err := a.domainBlockRepo.CreateDomainBlock(ctx, block)
	if err != nil {
		return MapRepositoryError(err, "CreateDomainBlock", "InstanceDomainBlock", block.Domain)
	}

	return nil
}

// UpdateDomainBlock updates an existing domain block
func (a *StorageAdapter) UpdateDomainBlock(ctx context.Context, id string, updates map[string]any) error {
	if a.domainBlockRepo == nil {
		return fmt.Errorf("domain block repository not initialized")
	}

	err := a.domainBlockRepo.UpdateDomainBlock(ctx, id, updates)
	if err != nil {
		return MapRepositoryError(err, "UpdateDomainBlock", "InstanceDomainBlock", id)
	}

	return nil
}

// DeleteDomainBlock removes a domain block
func (a *StorageAdapter) DeleteDomainBlock(ctx context.Context, id string) error {
	if a.domainBlockRepo == nil {
		return fmt.Errorf("domain block repository not initialized")
	}

	err := a.domainBlockRepo.DeleteDomainBlock(ctx, id)
	if err != nil {
		return MapRepositoryError(err, "DeleteDomainBlock", "InstanceDomainBlock", id)
	}

	return nil
}

// IsDomainBlocked checks if a domain is blocked at the instance level
func (a *StorageAdapter) IsDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	if a.domainBlockRepo == nil {
		return false, nil, fmt.Errorf("domain block repository not initialized")
	}

	blocked, block, err := a.domainBlockRepo.IsDomainBlocked(ctx, domain)
	if err != nil {
		return false, nil, MapRepositoryError(err, "IsDomainBlocked", "InstanceDomainBlock", domain)
	}

	return blocked, block, nil
}

// CreateEmailDomainBlock creates an email domain block
func (a *StorageAdapter) CreateEmailDomainBlock(ctx context.Context, block *storage.EmailDomainBlock) error {
	if a.domainBlockRepo == nil {
		return fmt.Errorf("domain block repository not initialized")
	}

	err := a.domainBlockRepo.CreateEmailDomainBlock(ctx, block)
	if err != nil {
		return MapRepositoryError(err, "CreateEmailDomainBlock", "EmailDomainBlock", block.Domain)
	}

	return nil
}

// GetEmailDomainBlocks retrieves email domain blocks with pagination
func (a *StorageAdapter) GetEmailDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.EmailDomainBlock, string, error) {
	if a.domainBlockRepo == nil {
		return nil, "", fmt.Errorf("domain block repository not initialized")
	}

	blocks, nextCursor, err := a.domainBlockRepo.GetEmailDomainBlocks(ctx, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetEmailDomainBlocks", "EmailDomainBlock", "all")
	}

	return blocks, nextCursor, nil
}

// DeleteEmailDomainBlock deletes an email domain block
func (a *StorageAdapter) DeleteEmailDomainBlock(ctx context.Context, id string) error {
	if a.domainBlockRepo == nil {
		return fmt.Errorf("domain block repository not initialized")
	}

	err := a.domainBlockRepo.DeleteEmailDomainBlock(ctx, id)
	if err != nil {
		return MapRepositoryError(err, "DeleteEmailDomainBlock", "EmailDomainBlock", id)
	}

	return nil
}

// Domain allow operations

// GetDomainAllows retrieves domain allows (for allowlist mode)
func (a *StorageAdapter) GetDomainAllows(ctx context.Context, limit int, cursor string) ([]*storage.DomainAllow, string, error) {
	if a.domainBlockRepo == nil {
		return nil, "", fmt.Errorf("domain block repository not initialized")
	}

	allows, nextCursor, err := a.domainBlockRepo.GetDomainAllows(ctx, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetDomainAllows", "DomainAllow", "all")
	}

	return allows, nextCursor, nil
}

// CreateDomainAllow adds a domain to the allowlist
func (a *StorageAdapter) CreateDomainAllow(ctx context.Context, allow *storage.DomainAllow) error {
	if a.domainBlockRepo == nil {
		return fmt.Errorf("domain block repository not initialized")
	}

	err := a.domainBlockRepo.CreateDomainAllow(ctx, allow)
	if err != nil {
		return MapRepositoryError(err, "CreateDomainAllow", "DomainAllow", allow.Domain)
	}

	return nil
}

// DeleteDomainAllow removes a domain from the allowlist
func (a *StorageAdapter) DeleteDomainAllow(ctx context.Context, id string) error {
	if a.domainBlockRepo == nil {
		return fmt.Errorf("domain block repository not initialized")
	}

	err := a.domainBlockRepo.DeleteDomainAllow(ctx, id)
	if err != nil {
		return MapRepositoryError(err, "DeleteDomainAllow", "DomainAllow", id)
	}

	return nil
}

// Relay operations

// StoreRelayInfo stores relay information
func (a *StorageAdapter) StoreRelayInfo(ctx context.Context, relay *storage.RelayInfo) error {
	if a.relayRepo == nil {
		return fmt.Errorf("relay repository not initialized")
	}

	err := a.relayRepo.StoreRelayInfo(ctx, relay)
	if err != nil {
		return MapRepositoryError(err, "StoreRelayInfo", "RelayInfo", relay.URL)
	}

	return nil
}

// GetRelayInfo retrieves relay information
func (a *StorageAdapter) GetRelayInfo(ctx context.Context, relayURL string) (*storage.RelayInfo, error) {
	if a.relayRepo == nil {
		return nil, fmt.Errorf("relay repository not initialized")
	}

	relay, err := a.relayRepo.GetRelayInfo(ctx, relayURL)
	if err != nil {
		return nil, MapRepositoryError(err, "GetRelayInfo", "RelayInfo", relayURL)
	}

	return relay, nil
}

// RemoveRelayInfo removes relay information
func (a *StorageAdapter) RemoveRelayInfo(ctx context.Context, relayURL string) error {
	if a.relayRepo == nil {
		return fmt.Errorf("relay repository not initialized")
	}

	err := a.relayRepo.RemoveRelayInfo(ctx, relayURL)
	if err != nil {
		return MapRepositoryError(err, "RemoveRelayInfo", "RelayInfo", relayURL)
	}

	return nil
}

// GetActiveRelays retrieves all active relays
func (a *StorageAdapter) GetActiveRelays(ctx context.Context) ([]*storage.RelayInfo, error) {
	if a.relayRepo == nil {
		return nil, fmt.Errorf("relay repository not initialized")
	}

	relays, err := a.relayRepo.GetActiveRelays(ctx)
	if err != nil {
		return nil, MapRepositoryError(err, "GetActiveRelays", "RelayInfo", "active")
	}

	return relays, nil
}

// GetAllRelays retrieves all relays with pagination
func (a *StorageAdapter) GetAllRelays(ctx context.Context, limit int, cursor string) ([]*storage.RelayInfo, string, error) {
	if a.relayRepo == nil {
		return nil, "", fmt.Errorf("relay repository not initialized")
	}

	relays, nextCursor, err := a.relayRepo.GetAllRelays(ctx, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetAllRelays", "RelayInfo", "all")
	}

	return relays, nextCursor, nil
}

// UpdateRelayStatus updates the active status of a relay
func (a *StorageAdapter) UpdateRelayStatus(ctx context.Context, relayURL string, active bool) error {
	if a.relayRepo == nil {
		return fmt.Errorf("relay repository not initialized")
	}

	err := a.relayRepo.UpdateRelayStatus(ctx, relayURL, active)
	if err != nil {
		return MapRepositoryError(err, "UpdateRelayStatus", "RelayInfo", relayURL)
	}

	return nil
}

// Federation instance tracking operations

// GetInstanceInfo retrieves information about a federated instance
func (a *StorageAdapter) GetInstanceInfo(ctx context.Context, domain string) (*storage.InstanceInfo, error) {
	if a.federationRepo == nil {
		return nil, fmt.Errorf("federation repository not initialized")
	}

	info, err := a.federationRepo.GetInstanceInfo(ctx, domain)
	if err != nil {
		return nil, MapRepositoryError(err, "GetInstanceInfo", "InstanceInfo", domain)
	}

	return info, nil
}

// UpsertInstanceInfo creates or updates instance information
func (a *StorageAdapter) UpsertInstanceInfo(ctx context.Context, info *storage.InstanceInfo) error {
	if a.federationRepo == nil {
		return fmt.Errorf("federation repository not initialized")
	}

	err := a.federationRepo.UpsertInstanceInfo(ctx, info)
	if err != nil {
		return MapRepositoryError(err, "UpsertInstanceInfo", "InstanceInfo", info.Domain)
	}

	return nil
}

// GetKnownInstances retrieves a list of known federated instances
func (a *StorageAdapter) GetKnownInstances(ctx context.Context, limit int, cursor string) ([]*storage.InstanceInfo, string, error) {
	if a.federationRepo == nil {
		return nil, "", fmt.Errorf("federation repository not initialized")
	}

	instances, nextCursor, err := a.federationRepo.GetKnownInstances(ctx, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetKnownInstances", "InstanceInfo", "known")
	}

	return instances, nextCursor, nil
}

// GetFederationStatistics retrieves federation statistics for a time range
func (a *StorageAdapter) GetFederationStatistics(ctx context.Context, startTime, endTime time.Time) (*storage.FederationStats, error) {
	if a.federationRepo == nil {
		return nil, fmt.Errorf("federation repository not initialized")
	}

	stats, err := a.federationRepo.GetFederationStatistics(ctx, startTime, endTime)
	if err != nil {
		return nil, MapRepositoryError(err, "GetFederationStatistics", "FederationStats", "statistics")
	}

	return stats, nil
}

// Federation cost tracking operations

// RecordFederationActivity records a single federation activity for cost tracking
func (a *StorageAdapter) RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error {
	if a.federationRepo == nil {
		return fmt.Errorf("federation repository not initialized")
	}

	err := a.federationRepo.RecordFederationActivity(ctx, activity)
	if err != nil {
		return MapRepositoryError(err, "RecordFederationActivity", "FederationActivity", activity.ID)
	}

	return nil
}

// GetFederationCosts retrieves aggregated federation costs
func (a *StorageAdapter) GetFederationCosts(ctx context.Context, startTime, endTime time.Time, limit int, cursor string) ([]*storage.FederationCost, string, error) {
	if a.federationRepo == nil {
		return nil, "", fmt.Errorf("federation repository not initialized")
	}

	costs, nextCursor, err := a.federationRepo.GetFederationCosts(ctx, startTime, endTime, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetFederationCosts", "FederationCost", "costs")
	}

	return costs, nextCursor, nil
}

// GetInstanceHealthReport generates a health report for a specific instance
func (a *StorageAdapter) GetInstanceHealthReport(ctx context.Context, domain string, period time.Duration) (*storage.InstanceHealthReport, error) {
	if a.federationRepo == nil {
		return nil, fmt.Errorf("federation repository not initialized")
	}

	report, err := a.federationRepo.GetInstanceHealthReport(ctx, domain, period)
	if err != nil {
		return nil, MapRepositoryError(err, "GetInstanceHealthReport", "InstanceHealthReport", domain)
	}

	return report, nil
}

// GetCostProjections generates cost projections based on historical data
func (a *StorageAdapter) GetCostProjections(ctx context.Context, period string) (*storage.CostProjection, error) {
	if a.federationRepo == nil {
		return nil, fmt.Errorf("federation repository not initialized")
	}

	projection, err := a.federationRepo.GetCostProjections(ctx, period)
	if err != nil {
		return nil, MapRepositoryError(err, "GetCostProjections", "CostProjection", period)
	}

	return projection, nil
}

// Federation graph operations

// GetFederationNodes retrieves federation nodes up to a certain depth
func (a *StorageAdapter) GetFederationNodes(ctx context.Context, depth int) ([]*storage.FederationNode, error) {
	if a.federationRepo == nil {
		return nil, fmt.Errorf("federation repository not initialized")
	}

	nodes, err := a.federationRepo.GetFederationNodes(ctx, depth)
	if err != nil {
		return nil, MapRepositoryError(err, "GetFederationNodes", "FederationNode", fmt.Sprintf("depth:%d", depth))
	}

	return nodes, nil
}

// GetFederationEdges retrieves edges between specified domains
func (a *StorageAdapter) GetFederationEdges(ctx context.Context, domains []string) ([]*storage.FederationEdge, error) {
	if a.federationRepo == nil {
		return nil, fmt.Errorf("federation repository not initialized")
	}

	edges, err := a.federationRepo.GetFederationEdges(ctx, domains)
	if err != nil {
		return nil, MapRepositoryError(err, "GetFederationEdges", "FederationEdge", fmt.Sprintf("domains:%d", len(domains)))
	}

	return edges, nil
}

// GetInstanceMetadata retrieves metadata for a specific instance
func (a *StorageAdapter) GetInstanceMetadata(ctx context.Context, domain string) (*storage.InstanceMetadata, error) {
	if a.federationRepo == nil {
		return nil, fmt.Errorf("federation repository not initialized")
	}

	metadata, err := a.federationRepo.GetInstanceMetadata(ctx, domain)
	if err != nil {
		return nil, MapRepositoryError(err, "GetInstanceMetadata", "InstanceMetadata", domain)
	}

	return metadata, nil
}

// CalculateFederationClusters calculates instance clusters based on connections
func (a *StorageAdapter) CalculateFederationClusters(ctx context.Context) ([]*storage.InstanceCluster, error) {
	if a.federationRepo == nil {
		return nil, fmt.Errorf("federation repository not initialized")
	}

	clusters, err := a.federationRepo.CalculateFederationClusters(ctx)
	if err != nil {
		return nil, MapRepositoryError(err, "CalculateFederationClusters", "InstanceCluster", "clusters")
	}

	return clusters, nil
}

// GetInstanceConnections retrieves connections for a specific instance
func (a *StorageAdapter) GetInstanceConnections(ctx context.Context, domain string, connectionType string) ([]*storage.InstanceConnection, error) {
	if a.federationRepo == nil {
		return nil, fmt.Errorf("federation repository not initialized")
	}

	connections, err := a.federationRepo.GetInstanceConnections(ctx, domain, connectionType)
	if err != nil {
		return nil, MapRepositoryError(err, "GetInstanceConnections", "InstanceConnection", domain)
	}

	return connections, nil
}

// AcknowledgeSeverance marks a severance as acknowledged by the user
func (a *StorageAdapter) AcknowledgeSeverance(ctx context.Context, userID, domain string) error {
	if a.federationRepo == nil {
		return fmt.Errorf("federation repository not initialized")
	}

	err := a.federationRepo.AcknowledgeSeverance(ctx, userID, domain)
	if err != nil {
		return MapRepositoryError(err, "AcknowledgeSeverance", "FederationSeverance", fmt.Sprintf("%s:%s", userID, domain))
	}

	return nil
}

// AttemptReconnection records an attempt to reconnect to a severed domain
func (a *StorageAdapter) AttemptReconnection(ctx context.Context, userID, domain string) error {
	if a.federationRepo == nil {
		return fmt.Errorf("federation repository not initialized")
	}

	err := a.federationRepo.AttemptReconnection(ctx, userID, domain)
	if err != nil {
		return MapRepositoryError(err, "AttemptReconnection", "ReconnectionAttempt", fmt.Sprintf("%s:%s", userID, domain))
	}

	return nil
}

// GetUserSeveredRelationships returns all severed relationships for a user
func (a *StorageAdapter) GetUserSeveredRelationships(ctx context.Context, userID string) ([]*storage.SeveredRelationship, error) {
	if a.federationRepo == nil {
		return nil, fmt.Errorf("federation repository not initialized")
	}

	relationships, err := a.federationRepo.GetUserSeveredRelationships(ctx, userID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetUserSeveredRelationships", "SeveredRelationship", userID)
	}

	return relationships, nil
}

// GetAffectedRelationships returns relationships affected by domain severance
func (a *StorageAdapter) GetAffectedRelationships(ctx context.Context, userID, domain string) ([]*storage.RelationshipRecord, error) {
	if a.federationRepo == nil {
		return nil, fmt.Errorf("federation repository not initialized")
	}

	relationships, err := a.federationRepo.GetAffectedRelationships(ctx, userID, domain)
	if err != nil {
		return nil, MapRepositoryError(err, "GetAffectedRelationships", "RelationshipRecord", fmt.Sprintf("%s:%s", userID, domain))
	}

	return relationships, nil
}

// TrackFederationIssue records a federation issue for monitoring
func (a *StorageAdapter) TrackFederationIssue(ctx context.Context, domain, issueType string) error {
	if a.federationRepo == nil {
		return fmt.Errorf("federation repository not initialized")
	}

	err := a.federationRepo.TrackFederationIssue(ctx, domain, issueType)
	if err != nil {
		return MapRepositoryError(err, "TrackFederationIssue", "FederationIssue", domain)
	}

	return nil
}

// GetRecentInstanceConnections retrieves connections for an instance within a time window
func (a *StorageAdapter) GetRecentInstanceConnections(ctx context.Context, domain string, since time.Duration) ([]*storage.InstanceConnection, error) {
	if a.federationRepo == nil {
		return nil, fmt.Errorf("federation repository not initialized")
	}

	connections, err := a.federationRepo.GetRecentInstanceConnections(ctx, domain, since)
	if err != nil {
		return nil, MapRepositoryError(err, "GetRecentInstanceConnections", "InstanceConnection", domain)
	}

	return connections, nil
}

// UpdateFederationNode updates or creates a federation node
func (a *StorageAdapter) UpdateFederationNode(ctx context.Context, node *storage.FederationNode) error {
	if a.federationRepo == nil {
		return fmt.Errorf("federation repository not initialized")
	}

	err := a.federationRepo.UpdateFederationNode(ctx, node)
	if err != nil {
		return MapRepositoryError(err, "UpdateFederationNode", "FederationNode", node.Domain)
	}

	return nil
}

// UpdateFederationEdge updates or creates a federation edge
func (a *StorageAdapter) UpdateFederationEdge(ctx context.Context, edge *storage.FederationEdge) error {
	if a.federationRepo == nil {
		return fmt.Errorf("federation repository not initialized")
	}

	err := a.federationRepo.UpdateFederationEdge(ctx, edge)
	if err != nil {
		return MapRepositoryError(err, "UpdateFederationEdge", "FederationEdge", fmt.Sprintf("%s->%s", edge.SourceDomain, edge.TargetDomain))
	}

	return nil
}

// UpdateInstanceMetadata updates instance metadata
func (a *StorageAdapter) UpdateInstanceMetadata(ctx context.Context, metadata *storage.InstanceMetadata) error {
	if a.federationRepo == nil {
		return fmt.Errorf("federation repository not initialized")
	}

	err := a.federationRepo.UpdateInstanceMetadata(ctx, metadata)
	if err != nil {
		return MapRepositoryError(err, "UpdateInstanceMetadata", "InstanceMetadata", metadata.Domain)
	}

	return nil
}

// StoreFederationTimeSeries stores time-series federation metrics
func (a *StorageAdapter) StoreFederationTimeSeries(ctx context.Context, data *storage.FederationTimeSeries) error {
	if a.federationRepo == nil {
		return fmt.Errorf("federation repository not initialized")
	}

	err := a.federationRepo.StoreFederationTimeSeries(ctx, data)
	if err != nil {
		return MapRepositoryError(err, "StoreFederationTimeSeries", "FederationTimeSeries", data.Domain)
	}

	return nil
}

// StoreInstanceCluster stores a calculated federation cluster
func (a *StorageAdapter) StoreInstanceCluster(ctx context.Context, cluster *storage.InstanceCluster) error {
	if a.federationRepo == nil {
		return fmt.Errorf("federation repository not initialized")
	}

	err := a.federationRepo.StoreInstanceCluster(ctx, cluster)
	if err != nil {
		return MapRepositoryError(err, "StoreInstanceCluster", "InstanceCluster", cluster.ClusterID)
	}

	return nil
}

// Wallet authentication operations

// StoreWalletChallenge stores a temporary wallet authentication challenge
func (a *StorageAdapter) StoreWalletChallenge(ctx context.Context, challenge *storage.WalletChallenge) error {
	if a.accountRepo == nil {
		return fmt.Errorf("auth repository not initialized")
	}

	err := a.accountRepo.StoreWalletChallenge(ctx, challenge)
	if err != nil {
		return MapRepositoryError(err, "StoreWalletChallenge", "WalletChallenge", challenge.ID)
	}

	return nil
}

// GetWalletChallenge retrieves a wallet challenge by ID
func (a *StorageAdapter) GetWalletChallenge(ctx context.Context, challengeID string) (*storage.WalletChallenge, error) {
	if a.accountRepo == nil {
		return nil, fmt.Errorf("auth repository not initialized")
	}

	challenge, err := a.accountRepo.GetWalletChallenge(ctx, challengeID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetWalletChallenge", "WalletChallenge", challengeID)
	}

	return challenge, nil
}

// StoreWalletCredential stores a wallet credential linked to a user
func (a *StorageAdapter) StoreWalletCredential(ctx context.Context, credential *storage.WalletCredential) error {
	if a.accountRepo == nil {
		return fmt.Errorf("auth repository not initialized")
	}

	err := a.accountRepo.StoreWalletCredential(ctx, credential)
	if err != nil {
		return MapRepositoryError(err, "StoreWalletCredential", "WalletCredential", credential.Address)
	}

	return nil
}

// GetWalletCredential retrieves a wallet credential by wallet type and address
func (a *StorageAdapter) GetWalletCredential(ctx context.Context, walletType, address string) (*storage.WalletCredential, error) {
	if a.accountRepo == nil {
		return nil, fmt.Errorf("auth repository not initialized")
	}

	credential, err := a.accountRepo.GetWalletByAddress(ctx, walletType, address)
	if err != nil {
		return nil, MapRepositoryError(err, "GetWalletCredential", "WalletCredential", address)
	}

	return credential, nil
}

// DeleteWalletChallenge deletes a wallet challenge
func (a *StorageAdapter) DeleteWalletChallenge(ctx context.Context, challengeID string) error {
	if a.accountRepo == nil {
		return fmt.Errorf("auth repository not initialized")
	}

	err := a.accountRepo.DeleteWalletChallenge(ctx, challengeID)
	if err != nil {
		return MapRepositoryError(err, "DeleteWalletChallenge", "WalletChallenge", challengeID)
	}

	return nil
}

// GetUserWalletCredentials retrieves all wallet credentials for a user
func (a *StorageAdapter) GetUserWalletCredentials(ctx context.Context, username string) ([]*storage.WalletCredential, error) {
	if a.accountRepo == nil {
		return nil, fmt.Errorf("auth repository not initialized")
	}

	credentials, err := a.accountRepo.GetUserWallets(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetUserWalletCredentials", "WalletCredential", username)
	}

	return credentials, nil
}

// DeleteWalletCredential deletes a wallet credential
func (a *StorageAdapter) DeleteWalletCredential(ctx context.Context, username, address string) error {
	if a.accountRepo == nil {
		return fmt.Errorf("auth repository not initialized")
	}

	err := a.accountRepo.DeleteWalletCredential(ctx, username, address)
	if err != nil {
		return MapRepositoryError(err, "DeleteWalletCredential", "WalletCredential", fmt.Sprintf("%s/%s", username, address))
	}

	return nil
}

// UpdateWalletLastUsed updates the last used timestamp for a wallet
func (a *StorageAdapter) UpdateWalletLastUsed(ctx context.Context, username, address string) error {
	// Note: Since authRepo doesn't have UpdateWalletLastUsed, we need to delegate to walletRepo if available
	if a.walletRepo != nil {
		err := a.walletRepo.UpdateWalletLastUsed(ctx, username, address)
		if err != nil {
			return MapRepositoryError(err, "UpdateWalletLastUsed", "WalletCredential", fmt.Sprintf("%s/%s", username, address))
		}
		return nil
	}

	// If no walletRepo, return error
	return fmt.Errorf("wallet repository not initialized for UpdateWalletLastUsed")
}

// Social recovery operations

// StoreTrustee stores a trustee configuration for social recovery
func (a *StorageAdapter) StoreTrustee(ctx context.Context, username string, trustee *storage.TrusteeConfig) error {
	if a.recoveryRepo == nil {
		return fmt.Errorf("recovery repository not initialized")
	}

	err := a.recoveryRepo.StoreTrustee(ctx, username, trustee)
	if err != nil {
		return MapRepositoryError(err, "StoreTrustee", "Trustee", username)
	}

	return nil
}

// GetTrustees retrieves all trustees for a user
func (a *StorageAdapter) GetTrustees(ctx context.Context, username string) ([]*storage.TrusteeConfig, error) {
	if a.recoveryRepo == nil {
		return nil, fmt.Errorf("recovery repository not initialized")
	}

	trustees, err := a.recoveryRepo.GetTrustees(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetTrustees", "Trustee", username)
	}

	return trustees, nil
}

// DeleteTrustee removes a trustee
func (a *StorageAdapter) DeleteTrustee(ctx context.Context, username, trusteeActorID string) error {
	if a.recoveryRepo == nil {
		return fmt.Errorf("recovery repository not initialized")
	}

	err := a.recoveryRepo.DeleteTrustee(ctx, username, trusteeActorID)
	if err != nil {
		return MapRepositoryError(err, "DeleteTrustee", "Trustee", username)
	}

	return nil
}

// UpdateTrusteeConfirmed updates the confirmed status of a trustee
func (a *StorageAdapter) UpdateTrusteeConfirmed(ctx context.Context, username, trusteeActorID string, confirmed bool) error {
	if a.recoveryRepo == nil {
		return fmt.Errorf("recovery repository not initialized")
	}

	err := a.recoveryRepo.UpdateTrusteeConfirmed(ctx, username, trusteeActorID, confirmed)
	if err != nil {
		return MapRepositoryError(err, "UpdateTrusteeConfirmed", "Trustee", username)
	}

	return nil
}

// Recovery request operations

// StoreRecoveryRequest stores a social recovery request
func (a *StorageAdapter) StoreRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error {
	if a.recoveryRepo == nil {
		return fmt.Errorf("recovery repository not initialized")
	}

	err := a.recoveryRepo.StoreRecoveryRequest(ctx, request)
	if err != nil {
		return MapRepositoryError(err, "StoreRecoveryRequest", "RecoveryRequest", request.ID)
	}

	return nil
}

// GetRecoveryRequest retrieves a recovery request by ID
func (a *StorageAdapter) GetRecoveryRequest(ctx context.Context, requestID string) (*storage.SocialRecoveryRequest, error) {
	if a.recoveryRepo == nil {
		return nil, fmt.Errorf("recovery repository not initialized")
	}

	request, err := a.recoveryRepo.GetRecoveryRequest(ctx, requestID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetRecoveryRequest", "RecoveryRequest", requestID)
	}

	return request, nil
}

// UpdateRecoveryRequest updates a recovery request
func (a *StorageAdapter) UpdateRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error {
	if a.recoveryRepo == nil {
		return fmt.Errorf("recovery repository not initialized")
	}

	err := a.recoveryRepo.UpdateRecoveryRequest(ctx, request)
	if err != nil {
		return MapRepositoryError(err, "UpdateRecoveryRequest", "RecoveryRequest", request.ID)
	}

	return nil
}

// DeleteRecoveryRequest deletes a recovery request
func (a *StorageAdapter) DeleteRecoveryRequest(ctx context.Context, requestID string) error {
	if a.recoveryRepo == nil {
		return fmt.Errorf("recovery repository not initialized")
	}

	err := a.recoveryRepo.DeleteRecoveryRequest(ctx, requestID)
	if err != nil {
		return MapRepositoryError(err, "DeleteRecoveryRequest", "RecoveryRequest", requestID)
	}

	return nil
}

// GetActiveRecoveryRequests gets all active recovery requests for a user
func (a *StorageAdapter) GetActiveRecoveryRequests(ctx context.Context, username string) ([]*storage.SocialRecoveryRequest, error) {
	if a.recoveryRepo == nil {
		return nil, fmt.Errorf("recovery repository not initialized")
	}

	requests, err := a.recoveryRepo.GetActiveRecoveryRequests(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetActiveRecoveryRequests", "RecoveryRequest", username)
	}

	return requests, nil
}

// Recovery code operations

// StoreRecoveryCode stores a recovery code
func (a *StorageAdapter) StoreRecoveryCode(ctx context.Context, username string, code *storage.RecoveryCodeItem) error {
	if a.recoveryRepo == nil {
		return fmt.Errorf("recovery repository not initialized")
	}

	err := a.recoveryRepo.StoreRecoveryCode(ctx, username, code)
	if err != nil {
		return MapRepositoryError(err, "StoreRecoveryCode", "RecoveryCode", username)
	}

	return nil
}

// GetRecoveryCodes retrieves all recovery codes for a user
func (a *StorageAdapter) GetRecoveryCodes(ctx context.Context, username string) ([]*storage.RecoveryCodeItem, error) {
	if a.recoveryRepo == nil {
		return nil, fmt.Errorf("recovery repository not initialized")
	}

	codes, err := a.recoveryRepo.GetRecoveryCodes(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetRecoveryCodes", "RecoveryCode", username)
	}

	return codes, nil
}

// MarkRecoveryCodeUsed marks a recovery code as used
func (a *StorageAdapter) MarkRecoveryCodeUsed(ctx context.Context, username, codeHash string) error {
	if a.recoveryRepo == nil {
		return fmt.Errorf("recovery repository not initialized")
	}

	err := a.recoveryRepo.MarkRecoveryCodeUsed(ctx, username, codeHash)
	if err != nil {
		return MapRepositoryError(err, "MarkRecoveryCodeUsed", "RecoveryCode", username)
	}

	return nil
}

// DeleteAllRecoveryCodes deletes all recovery codes for a user
func (a *StorageAdapter) DeleteAllRecoveryCodes(ctx context.Context, username string) error {
	if a.recoveryRepo == nil {
		return fmt.Errorf("recovery repository not initialized")
	}

	err := a.recoveryRepo.DeleteAllRecoveryCodes(ctx, username)
	if err != nil {
		return MapRepositoryError(err, "DeleteAllRecoveryCodes", "RecoveryCode", username)
	}

	return nil
}

// CountUnusedRecoveryCodes counts how many unused recovery codes the user has
func (a *StorageAdapter) CountUnusedRecoveryCodes(ctx context.Context, username string) (int, error) {
	if a.recoveryRepo == nil {
		return 0, fmt.Errorf("recovery repository not initialized")
	}

	count, err := a.recoveryRepo.CountUnusedRecoveryCodes(ctx, username)
	if err != nil {
		return 0, MapRepositoryError(err, "CountUnusedRecoveryCodes", "RecoveryCode", username)
	}

	return count, nil
}

// Rate limiting operations

// RecordLoginAttempt records a login attempt for rate limiting
func (a *StorageAdapter) RecordLoginAttempt(ctx context.Context, identifier string, success bool) error {
	if a.rateLimitRepo == nil {
		return fmt.Errorf("rate limit repository not initialized")
	}

	err := a.rateLimitRepo.RecordLoginAttempt(ctx, identifier, success)
	if err != nil {
		return MapRepositoryError(err, "RecordLoginAttempt", "LoginAttempt", identifier)
	}

	return nil
}

// GetLoginAttemptCount returns the number of login attempts since the given time
func (a *StorageAdapter) GetLoginAttemptCount(ctx context.Context, identifier string, since time.Time) (int, error) {
	if a.rateLimitRepo == nil {
		return 0, fmt.Errorf("rate limit repository not initialized")
	}

	count, err := a.rateLimitRepo.GetLoginAttemptCount(ctx, identifier, since)
	if err != nil {
		return 0, MapRepositoryError(err, "GetLoginAttemptCount", "LoginAttempt", identifier)
	}

	return count, nil
}

// IsRateLimited checks if an identifier is currently rate limited
func (a *StorageAdapter) IsRateLimited(ctx context.Context, identifier string) (bool, time.Time, error) {
	if a.rateLimitRepo == nil {
		return false, time.Time{}, fmt.Errorf("rate limit repository not initialized")
	}

	isLimited, unlockTime, err := a.rateLimitRepo.IsRateLimited(ctx, identifier)
	if err != nil {
		return false, time.Time{}, MapRepositoryError(err, "IsRateLimited", "RateLimit", identifier)
	}

	return isLimited, unlockTime, nil
}

// ClearLoginAttempts clears all login attempts for an identifier
func (a *StorageAdapter) ClearLoginAttempts(ctx context.Context, identifier string) error {
	if a.rateLimitRepo == nil {
		return fmt.Errorf("rate limit repository not initialized")
	}

	err := a.rateLimitRepo.ClearLoginAttempts(ctx, identifier)
	if err != nil {
		return MapRepositoryError(err, "ClearLoginAttempts", "LoginAttempt", identifier)
	}

	return nil
}

// API rate limiting operations

// CheckAPIRateLimit checks and updates API rate limiting for a user/endpoint combination
func (a *StorageAdapter) CheckAPIRateLimit(ctx context.Context, userID, endpoint string, limit int, window time.Duration) error {
	if a.rateLimitRepo == nil {
		return fmt.Errorf("rate limit repository not initialized")
	}

	err := a.rateLimitRepo.CheckAPIRateLimit(ctx, userID, endpoint, limit, window)
	if err != nil {
		return MapRepositoryError(err, "CheckAPIRateLimit", "APIRateLimit", fmt.Sprintf("%s:%s", userID, endpoint))
	}

	return nil
}

// GetAPIRateLimitInfo returns current rate limit info for response headers
func (a *StorageAdapter) GetAPIRateLimitInfo(ctx context.Context, userID, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error) {
	if a.rateLimitRepo == nil {
		return 0, time.Time{}, fmt.Errorf("rate limit repository not initialized")
	}

	remaining, resetTime, err = a.rateLimitRepo.GetAPIRateLimitInfo(ctx, userID, endpoint, limit, window)
	if err != nil {
		return 0, time.Time{}, MapRepositoryError(err, "GetAPIRateLimitInfo", "APIRateLimit", fmt.Sprintf("%s:%s", userID, endpoint))
	}

	return remaining, resetTime, nil
}

// CheckCommunityNoteRateLimit checks if a user can create more community notes today
func (a *StorageAdapter) CheckCommunityNoteRateLimit(ctx context.Context, userID string, limit int) (bool, int, error) {
	if a.rateLimitRepo == nil {
		return false, 0, fmt.Errorf("rate limit repository not initialized")
	}

	canCreate, remaining, err := a.rateLimitRepo.CheckCommunityNoteRateLimit(ctx, userID, limit)
	if err != nil {
		return false, 0, MapRepositoryError(err, "CheckCommunityNoteRateLimit", "CommunityNoteRateLimit", userID)
	}

	return canCreate, remaining, nil
}

// Streaming preferences operations

// GetStreamingPreferences retrieves streaming preferences for a user
func (a *StorageAdapter) GetStreamingPreferences(ctx context.Context, username string) (*storage.StreamingPreferences, error) {
	if a.streamingRepo == nil {
		return nil, fmt.Errorf("streaming repository not initialized")
	}

	prefs, err := a.streamingRepo.GetStreamingPreferences(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetStreamingPreferences", "StreamingPreferences", username)
	}

	return prefs, nil
}

// UpdateStreamingPreferences updates streaming preferences for a user
func (a *StorageAdapter) UpdateStreamingPreferences(ctx context.Context, prefs *storage.StreamingPreferences) error {
	if a.streamingRepo == nil {
		return fmt.Errorf("streaming repository not initialized")
	}

	err := a.streamingRepo.UpdateStreamingPreferences(ctx, prefs)
	if err != nil {
		return MapRepositoryError(err, "UpdateStreamingPreferences", "StreamingPreferences", prefs.Username)
	}

	return nil
}

// GetStreamingPreferencesByDevice retrieves device-specific streaming preferences
func (a *StorageAdapter) GetStreamingPreferencesByDevice(ctx context.Context, username, deviceID string) (*storage.StreamingPreferences, error) {
	if a.streamingRepo == nil {
		return nil, fmt.Errorf("streaming repository not initialized")
	}

	prefs, err := a.streamingRepo.GetStreamingPreferencesByDevice(ctx, username, deviceID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetStreamingPreferencesByDevice", "StreamingPreferences", username+"/"+deviceID)
	}

	return prefs, nil
}

// UpdateDeviceStreamingPreferences updates device-specific streaming preferences
func (a *StorageAdapter) UpdateDeviceStreamingPreferences(ctx context.Context, prefs *storage.StreamingPreferences, deviceID string) error {
	if a.streamingRepo == nil {
		return fmt.Errorf("streaming repository not initialized")
	}

	err := a.streamingRepo.UpdateDeviceStreamingPreferences(ctx, prefs, deviceID)
	if err != nil {
		return MapRepositoryError(err, "UpdateDeviceStreamingPreferences", "StreamingPreferences", prefs.Username+"/"+deviceID)
	}

	return nil
}

// GetStreamingPreferenceHistory retrieves the version history of streaming preferences
func (a *StorageAdapter) GetStreamingPreferenceHistory(ctx context.Context, username string, limit int) ([]*storage.StreamingPreferences, error) {
	if a.streamingRepo == nil {
		return nil, fmt.Errorf("streaming repository not initialized")
	}

	history, err := a.streamingRepo.GetStreamingPreferenceHistory(ctx, username, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetStreamingPreferenceHistory", "StreamingPreferences", username)
	}

	return history, nil
}

// SyncStreamingPreferences syncs preferences across devices
func (a *StorageAdapter) SyncStreamingPreferences(ctx context.Context, username string, sourceDeviceID string) error {
	if a.streamingRepo == nil {
		return fmt.Errorf("streaming repository not initialized")
	}

	err := a.streamingRepo.SyncStreamingPreferences(ctx, username, sourceDeviceID)
	if err != nil {
		return MapRepositoryError(err, "SyncStreamingPreferences", "StreamingPreferences", username+"/"+sourceDeviceID)
	}

	return nil
}

// ResolvePreferenceConflict resolves conflicts between different preference versions
func (a *StorageAdapter) ResolvePreferenceConflict(ctx context.Context, username string, strategy storage.ConflictResolutionStrategy) (*storage.StreamingPreferences, error) {
	if a.streamingRepo == nil {
		return nil, fmt.Errorf("streaming repository not initialized")
	}

	prefs, err := a.streamingRepo.ResolvePreferenceConflict(ctx, username, strategy)
	if err != nil {
		return nil, MapRepositoryError(err, "ResolvePreferenceConflict", "StreamingPreferences", username)
	}

	return prefs, nil
}

// Community Note operations

// CreateCommunityNote creates a new community note
func (a *StorageAdapter) CreateCommunityNote(ctx context.Context, note *storage.CommunityNote) error {
	if a.communityNoteRepo == nil {
		return fmt.Errorf("community note repository not initialized")
	}

	err := a.communityNoteRepo.CreateCommunityNote(ctx, note)
	if err != nil {
		return MapRepositoryError(err, "CreateCommunityNote", "CommunityNote", note.ID)
	}

	return nil
}

// GetCommunityNote retrieves a note by ID
func (a *StorageAdapter) GetCommunityNote(ctx context.Context, noteID string) (*storage.CommunityNote, error) {
	if a.communityNoteRepo == nil {
		return nil, fmt.Errorf("community note repository not initialized")
	}

	note, err := a.communityNoteRepo.GetCommunityNote(ctx, noteID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetCommunityNote", "CommunityNote", noteID)
	}

	return note, nil
}

// GetVisibleCommunityNotes retrieves visible notes for an object
func (a *StorageAdapter) GetVisibleCommunityNotes(ctx context.Context, objectID string) ([]*storage.CommunityNote, error) {
	if a.communityNoteRepo == nil {
		return nil, fmt.Errorf("community note repository not initialized")
	}

	notes, err := a.communityNoteRepo.GetVisibleCommunityNotes(ctx, objectID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetVisibleCommunityNotes", "CommunityNote", objectID)
	}

	return notes, nil
}

// UpdateCommunityNoteScore updates a note's score and visibility
func (a *StorageAdapter) UpdateCommunityNoteScore(ctx context.Context, noteID string, score float64, status string) error {
	if a.communityNoteRepo == nil {
		return fmt.Errorf("community note repository not initialized")
	}

	err := a.communityNoteRepo.UpdateCommunityNoteScore(ctx, noteID, score, status)
	if err != nil {
		return MapRepositoryError(err, "UpdateCommunityNoteScore", "CommunityNote", noteID)
	}

	return nil
}

// UpdateCommunityNoteAnalysis updates AI analysis results for a note
func (a *StorageAdapter) UpdateCommunityNoteAnalysis(ctx context.Context, noteID string, sentiment, objectivity, sourceQuality float64) error {
	if a.communityNoteRepo == nil {
		return fmt.Errorf("community note repository not initialized")
	}

	err := a.communityNoteRepo.UpdateCommunityNoteAnalysis(ctx, noteID, sentiment, objectivity, sourceQuality)
	if err != nil {
		return MapRepositoryError(err, "UpdateCommunityNoteAnalysis", "CommunityNote", noteID)
	}

	return nil
}

// CreateCommunityNoteVote creates a vote on a note
func (a *StorageAdapter) CreateCommunityNoteVote(ctx context.Context, vote *storage.CommunityNoteVote) error {
	if a.communityNoteRepo == nil {
		return fmt.Errorf("community note repository not initialized")
	}

	err := a.communityNoteRepo.CreateCommunityNoteVote(ctx, vote)
	if err != nil {
		return MapRepositoryError(err, "CreateCommunityNoteVote", "CommunityNoteVote", vote.NoteID)
	}

	return nil
}

// GetUserCommunityNoteVotes retrieves a user's votes on specific notes
func (a *StorageAdapter) GetUserCommunityNoteVotes(ctx context.Context, userID string, noteIDs []string) (map[string]*storage.CommunityNoteVote, error) {
	if a.communityNoteRepo == nil {
		return nil, fmt.Errorf("community note repository not initialized")
	}

	votes, err := a.communityNoteRepo.GetUserCommunityNoteVotes(ctx, userID, noteIDs)
	if err != nil {
		return nil, MapRepositoryError(err, "GetUserCommunityNoteVotes", "CommunityNoteVote", userID)
	}

	return votes, nil
}

// GetCommunityNotesByAuthor retrieves community notes authored by a specific actor
func (a *StorageAdapter) GetCommunityNotesByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*storage.CommunityNote, string, error) {
	if a.communityNoteRepo == nil {
		return nil, "", fmt.Errorf("community note repository not initialized")
	}

	notes, nextCursor, err := a.communityNoteRepo.GetCommunityNotesByAuthor(ctx, authorID, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetCommunityNotesByAuthor", "CommunityNote", authorID)
	}

	return notes, nextCursor, nil
}

// GetCommunityNoteVotes retrieves votes on a specific community note
func (a *StorageAdapter) GetCommunityNoteVotes(ctx context.Context, noteID string) ([]*storage.CommunityNoteVote, error) {
	if a.communityNoteRepo == nil {
		return nil, fmt.Errorf("community note repository not initialized")
	}

	votes, err := a.communityNoteRepo.GetCommunityNoteVotes(ctx, noteID)
	if err != nil {
		return nil, MapRepositoryError(err, "GetCommunityNoteVotes", "CommunityNoteVote", noteID)
	}

	return votes, nil
}

// SearchStatusesByURL searches for a status by exact URL
func (a *StorageAdapter) SearchStatusesByURL(ctx context.Context, url string) (*storage.StatusSearchResult, error) {
	// Check if object repository is set
	if a.objectRepo == nil {
		return nil, fmt.Errorf("object repository not initialized")
	}

	// Try to get the object directly by ID
	obj, err := a.objectRepo.GetObject(ctx, url)
	if err != nil {
		return nil, MapRepositoryError(err, "GetObject", "Object", url)
	}

	// Check if it's a Note type
	if note, ok := obj.(*activitypub.Note); ok {
		published := time.Now()
		if note.Published != nil {
			published = *note.Published
		}

		// Extract username from author ID
		authorUsername := ""
		if note.AttributedTo != "" {
			parts := strings.Split(note.AttributedTo, "/")
			if len(parts) > 0 {
				authorUsername = parts[len(parts)-1]
			}
		}

		return &storage.StatusSearchResult{
			StatusID:       note.ID,
			Content:        note.Content,
			URL:            note.ID, // URL is typically the ID for notes
			AuthorID:       note.AttributedTo,
			AuthorUsername: authorUsername,
			Published:      published,
			Score:          1.0, // Perfect match for URL search
		}, nil
	}

	// Try to handle generic object types
	if objMap, ok := obj.(map[string]any); ok {
		result := &storage.StatusSearchResult{
			Score:      1.0,
			Highlights: make(map[string]string),
			Published:  time.Now(), // Default
		}

		if id, ok := objMap["id"].(string); ok {
			result.StatusID = id
			result.URL = id // Default URL to ID
		}
		if content, ok := objMap["content"].(string); ok {
			result.Content = content
		}
		if urlStr, ok := objMap["url"].(string); ok {
			result.URL = urlStr
		}
		if attributedTo, ok := objMap["attributedTo"].(string); ok {
			result.AuthorID = attributedTo
			// Extract username from author ID
			parts := strings.Split(attributedTo, "/")
			if len(parts) > 0 {
				result.AuthorUsername = parts[len(parts)-1]
			}
		}
		if published, ok := objMap["published"].(string); ok {
			if t, err := time.Parse(time.RFC3339, published); err == nil {
				result.Published = t
			}
		}

		return result, nil
	}

	return nil, fmt.Errorf("object is not a status")
}

// GetNotificationPreferences retrieves notification preferences for a user
func (s *StorageAdapter) GetNotificationPreferences(ctx context.Context, username string) (*storage.NotificationPreferences, error) {
	if s.notificationRepo == nil {
		return nil, fmt.Errorf("notification repository not initialized")
	}
	return s.notificationRepo.GetNotificationPreferences(ctx, username)
}

// UpdateNotificationPreferences creates or updates notification preferences for a user
func (s *StorageAdapter) UpdateNotificationPreferences(ctx context.Context, username string, prefs *storage.NotificationPreferences) error {
	if s.notificationRepo == nil {
		return fmt.Errorf("notification repository not initialized")
	}
	return s.notificationRepo.UpdateNotificationPreferences(ctx, username, prefs)
}

// BatchMarkNotificationsAsRead marks multiple notifications as read
func (s *StorageAdapter) BatchMarkNotificationsAsRead(ctx context.Context, username string, notificationIDs []string) error {
	if s.notificationRepo == nil {
		return fmt.Errorf("notification repository not initialized")
	}
	return s.notificationRepo.BatchMarkNotificationsAsRead(ctx, username, notificationIDs)
}

// StoreRecoveryToken stores a generic recovery token with data
func (a *StorageAdapter) StoreRecoveryToken(ctx context.Context, key string, data map[string]any) error {
	if a.recoveryRepo == nil {
		return fmt.Errorf("recovery repository not initialized")
	}
	
	err := a.recoveryRepo.StoreRecoveryToken(ctx, key, data)
	if err != nil {
		return MapRepositoryError(err, "StoreRecoveryToken", "RecoveryToken", key)
	}
	
	return nil
}

// GetRecoveryToken retrieves a recovery token by key
func (a *StorageAdapter) GetRecoveryToken(ctx context.Context, key string) (map[string]any, error) {
	if a.recoveryRepo == nil {
		return nil, fmt.Errorf("recovery repository not initialized")
	}
	
	data, err := a.recoveryRepo.GetRecoveryToken(ctx, key)
	if err != nil {
		return nil, MapRepositoryError(err, "GetRecoveryToken", "RecoveryToken", key)
	}
	
	return data, nil
}

// DeleteRecoveryToken deletes a recovery token
func (a *StorageAdapter) DeleteRecoveryToken(ctx context.Context, key string) error {
	if a.recoveryRepo == nil {
		return fmt.Errorf("recovery repository not initialized")
	}
	
	err := a.recoveryRepo.DeleteRecoveryToken(ctx, key)
	if err != nil {
		return MapRepositoryError(err, "DeleteRecoveryToken", "RecoveryToken", key)
	}
	
	return nil
}

// GetModerationQueue gets the moderation queue
func (s *StorageAdapter) GetModerationQueue(ctx context.Context, filter *storage.ModerationFilter) ([]*storage.ModerationQueueItem, error) {
	if s.moderationRepo == nil {
		return nil, fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.GetModerationQueue(ctx, filter)
}

// GetModerationQueuePaginated gets paginated moderation queue
func (s *StorageAdapter) GetModerationQueuePaginated(ctx context.Context, limit int, cursor string) ([]*storage.ModerationQueueItem, string, error) {
	if s.moderationRepo == nil {
		return nil, "", fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.GetModerationQueuePaginated(ctx, limit, cursor)
}

// GetModerationEventsByObject gets moderation events by object
func (s *StorageAdapter) GetModerationEventsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	if s.moderationRepo == nil {
		return nil, "", fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.GetModerationEventsByObject(ctx, objectID, limit, cursor)
}

// GetModerationEventsByActor gets moderation events by actor
func (s *StorageAdapter) GetModerationEventsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	if s.moderationRepo == nil {
		return nil, "", fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.GetModerationEventsByActor(ctx, actorID, limit, cursor)
}

// AddModerationReview adds a review to a moderation event
func (s *StorageAdapter) AddModerationReview(ctx context.Context, review *storage.ModerationReview) error {
	if s.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.AddModerationReview(ctx, review)
}

// GetModerationReviews gets reviews for a moderation event
func (s *StorageAdapter) GetModerationReviews(ctx context.Context, eventID string) ([]*storage.ModerationReview, error) {
	if s.moderationRepo == nil {
		return nil, fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.GetModerationReviews(ctx, eventID)
}

// CreateModerationDecision creates a moderation decision
func (s *StorageAdapter) CreateModerationDecision(ctx context.Context, decision *storage.ModerationDecision) error {
	if s.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.CreateModerationDecision(ctx, decision)
}

// GetModerationDecision gets a moderation decision
func (s *StorageAdapter) GetModerationDecision(ctx context.Context, objectID string) (*storage.ModerationDecision, error) {
	if s.moderationRepo == nil {
		return nil, fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.GetModerationDecision(ctx, objectID)
}

// StoreModerationDecision stores a moderation decision
func (s *StorageAdapter) StoreModerationDecision(ctx context.Context, decision *storage.ModerationDecision) error {
	if s.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.StoreModerationDecision(ctx, decision)
}

// UpdateModerationDecision updates a moderation decision
func (s *StorageAdapter) UpdateModerationDecision(ctx context.Context, contentID string, review *storage.ModerationReview) error {
	if s.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.UpdateModerationDecision(ctx, contentID, review)
}

// GetModerationPatterns gets moderation patterns
func (s *StorageAdapter) GetModerationPatterns(ctx context.Context, active bool, severity string, limit int) ([]*storage.ModerationPattern, error) {
	if s.moderationRepo == nil {
		return nil, fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.GetModerationPatterns(ctx, active, severity, limit)
}

// UpdateModerationPattern updates a moderation pattern
func (s *StorageAdapter) UpdateModerationPattern(ctx context.Context, pattern *storage.ModerationPattern) error {
	if s.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.UpdateModerationPattern(ctx, pattern)
}

// DeleteModerationPattern deletes a moderation pattern
func (s *StorageAdapter) DeleteModerationPattern(ctx context.Context, patternID string) error {
	if s.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.DeleteModerationPattern(ctx, patternID)
}

// GetModerationHistory retrieves the complete moderation history for an object
func (s *StorageAdapter) GetModerationHistory(ctx context.Context, objectID string) (*storage.ModerationHistory, error) {
	if s.moderationRepo == nil {
		return nil, fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.GetModerationHistory(ctx, objectID)
}

// GetModerationEvents retrieves all moderation events with optional filters
func (s *StorageAdapter) GetModerationEvents(ctx context.Context, filter *storage.ModerationEventFilter, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	if s.moderationRepo == nil {
		return nil, "", fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.GetModerationEvents(ctx, filter, limit, cursor)
}

// CreateAdminReview creates an admin review that overrides consensus
func (s *StorageAdapter) CreateAdminReview(ctx context.Context, eventID string, adminID string, action storage.ActionType, reason string) error {
	if s.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.CreateAdminReview(ctx, eventID, adminID, action, reason)
}

// GetReviewerStats retrieves statistics for a reviewer
func (s *StorageAdapter) GetReviewerStats(ctx context.Context, reviewerID string) (*storage.ReviewerStats, error) {
	if s.moderationRepo == nil {
		return nil, fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.GetReviewerStats(ctx, reviewerID)
}

// GetModerationQueueCount returns the count of items in the moderation queue
func (s *StorageAdapter) GetModerationQueueCount(ctx context.Context) (int, error) {
	if s.moderationRepo == nil {
		return 0, fmt.Errorf("moderation repository not initialized")
	}
	return s.moderationRepo.GetModerationQueueCount(ctx)
}

// Trending and Analytics Methods

// DeleteOldHashtagTrends deletes hashtag trend records older than the specified time
func (a *StorageAdapter) DeleteOldHashtagTrends(ctx context.Context, before time.Time) error {
	if a.trendingRepo == nil {
		return fmt.Errorf("trending repository not initialized")
	}
	err := a.trendingRepo.DeleteOldHashtagTrends(ctx, before)
	if err != nil {
		return MapRepositoryError(err, "DeleteOldHashtagTrends", "HashtagTrend", before.String())
	}
	return nil
}

// DeleteOldLinkTrends deletes link trend records older than the specified time
func (a *StorageAdapter) DeleteOldLinkTrends(ctx context.Context, before time.Time) error {
	if a.trendingRepo == nil {
		return fmt.Errorf("trending repository not initialized")
	}
	err := a.trendingRepo.DeleteOldLinkTrends(ctx, before)
	if err != nil {
		return MapRepositoryError(err, "DeleteOldLinkTrends", "LinkTrend", before.String())
	}
	return nil
}

// DeleteOldStatusTrends deletes status trend records older than the specified time
func (a *StorageAdapter) DeleteOldStatusTrends(ctx context.Context, before time.Time) error {
	if a.trendingRepo == nil {
		return fmt.Errorf("trending repository not initialized")
	}
	err := a.trendingRepo.DeleteOldStatusTrends(ctx, before)
	if err != nil {
		return MapRepositoryError(err, "DeleteOldStatusTrends", "StatusTrend", before.String())
	}
	return nil
}

// GetPopularSearchQueries retrieves the most popular search queries
func (a *StorageAdapter) GetPopularSearchQueries(ctx context.Context, limit int, timeWindow time.Duration) ([]storage.SearchQueryStats, error) {
	if a.trendingRepo == nil {
		return nil, fmt.Errorf("trending repository not initialized")
	}
	queries, err := a.trendingRepo.GetPopularSearchQueries(ctx, limit, timeWindow)
	if err != nil {
		return nil, MapRepositoryError(err, "GetPopularSearchQueries", "SearchQuery", fmt.Sprintf("limit=%d,window=%v", limit, timeWindow))
	}
	return queries, nil
}

// GetUserSearchHistory retrieves a user's search history
func (a *StorageAdapter) GetUserSearchHistory(ctx context.Context, userID string, limit int) ([]storage.SearchHistoryEntry, error) {
	if a.trendingRepo == nil {
		return nil, fmt.Errorf("trending repository not initialized")
	}
	history, err := a.trendingRepo.GetUserSearchHistory(ctx, userID, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetUserSearchHistory", "SearchHistory", userID)
	}
	return history, nil
}

// StoreHashtagTrend stores a hashtag trend record
func (a *StorageAdapter) StoreHashtagTrend(ctx context.Context, trend any) error {
	if a.trendingRepo == nil {
		return fmt.Errorf("trending repository not initialized")
	}
	err := a.trendingRepo.StoreHashtagTrend(ctx, trend)
	if err != nil {
		return MapRepositoryError(err, "StoreHashtagTrend", "HashtagTrend", "trend")
	}
	return nil
}

// StoreLinkTrend stores a link trend record
func (a *StorageAdapter) StoreLinkTrend(ctx context.Context, trend any) error {
	if a.trendingRepo == nil {
		return fmt.Errorf("trending repository not initialized")
	}
	err := a.trendingRepo.StoreLinkTrend(ctx, trend)
	if err != nil {
		return MapRepositoryError(err, "StoreLinkTrend", "LinkTrend", "trend")
	}
	return nil
}

// StoreStatusTrend stores a status trend record
func (a *StorageAdapter) StoreStatusTrend(ctx context.Context, trend any) error {
	if a.trendingRepo == nil {
		return fmt.Errorf("trending repository not initialized")
	}
	err := a.trendingRepo.StoreStatusTrend(ctx, trend)
	if err != nil {
		return MapRepositoryError(err, "StoreStatusTrend", "StatusTrend", "trend")
	}
	return nil
}

// TrackSearchQuery records a search query for analytics and suggestions
func (a *StorageAdapter) TrackSearchQuery(ctx context.Context, userID, query string, resultCount int) error {
	if a.trendingRepo == nil {
		return fmt.Errorf("trending repository not initialized")
	}
	err := a.trendingRepo.TrackSearchQuery(ctx, userID, query, resultCount)
	if err != nil {
		return MapRepositoryError(err, "TrackSearchQuery", "SearchQuery", query)
	}
	return nil
}

// FollowHashtag creates a hashtag follow relationship
func (a *StorageAdapter) FollowHashtag(ctx context.Context, userID string, hashtag string) error {
	if a.hashtagRepo == nil {
		return fmt.Errorf("hashtag repository not initialized")
	}
	err := a.hashtagRepo.FollowHashtag(ctx, userID, hashtag)
	if err != nil {
		return MapRepositoryError(err, "FollowHashtag", "HashtagFollow", hashtag)
	}
	return nil
}

// UnfollowHashtag removes a hashtag follow relationship
func (a *StorageAdapter) UnfollowHashtag(ctx context.Context, userID string, hashtag string) error {
	if a.hashtagRepo == nil {
		return fmt.Errorf("hashtag repository not initialized")
	}
	err := a.hashtagRepo.UnfollowHashtag(ctx, userID, hashtag)
	if err != nil {
		return MapRepositoryError(err, "UnfollowHashtag", "HashtagFollow", hashtag)
	}
	return nil
}

// IsFollowingHashtag checks if a user is following a hashtag
func (a *StorageAdapter) IsFollowingHashtag(ctx context.Context, userID string, hashtag string) (bool, error) {
	if a.hashtagRepo == nil {
		return false, fmt.Errorf("hashtag repository not initialized")
	}
	isFollowing, err := a.hashtagRepo.IsFollowingHashtag(ctx, userID, hashtag)
	if err != nil {
		return false, MapRepositoryError(err, "IsFollowingHashtag", "HashtagFollow", hashtag)
	}
	return isFollowing, nil
}

// GetFollowedHashtags retrieves hashtags followed by a user
func (a *StorageAdapter) GetFollowedHashtags(ctx context.Context, userID string, limit int, cursor string) ([]string, string, error) {
	if a.hashtagRepo == nil {
		return nil, "", fmt.Errorf("hashtag repository not initialized")
	}
	hashtags, nextCursor, err := a.hashtagRepo.GetFollowedHashtags(ctx, userID, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetFollowedHashtags", "HashtagFollow", userID)
	}
	return hashtags, nextCursor, nil
}

// UpdateHashtagNotificationSettings updates notification settings for a followed hashtag
func (a *StorageAdapter) UpdateHashtagNotificationSettings(ctx context.Context, userID, hashtag string, notify bool) error {
	if a.hashtagRepo == nil {
		return fmt.Errorf("hashtag repository not initialized")
	}
	err := a.hashtagRepo.UpdateHashtagNotificationSettings(ctx, userID, hashtag, notify)
	if err != nil {
		return MapRepositoryError(err, "UpdateHashtagNotificationSettings", "HashtagFollow", hashtag)
	}
	return nil
}

// MuteHashtag mutes a hashtag for a user
func (a *StorageAdapter) MuteHashtag(ctx context.Context, userID, hashtag string) error {
	if a.hashtagRepo == nil {
		return fmt.Errorf("hashtag repository not initialized")
	}
	err := a.hashtagRepo.MuteHashtag(ctx, userID, hashtag)
	if err != nil {
		return MapRepositoryError(err, "MuteHashtag", "HashtagFollow", hashtag)
	}
	return nil
}

// UnmuteHashtag unmutes a hashtag for a user
func (a *StorageAdapter) UnmuteHashtag(ctx context.Context, userID, hashtag string) error {
	if a.hashtagRepo == nil {
		return fmt.Errorf("hashtag repository not initialized")
	}
	err := a.hashtagRepo.UnmuteHashtag(ctx, userID, hashtag)
	if err != nil {
		return MapRepositoryError(err, "UnmuteHashtag", "HashtagFollow", hashtag)
	}
	return nil
}

// IsHashtagMuted checks if a hashtag is muted for a user
func (a *StorageAdapter) IsHashtagMuted(ctx context.Context, userID, hashtag string) (bool, error) {
	if a.hashtagRepo == nil {
		return false, fmt.Errorf("hashtag repository not initialized")
	}
	isMuted, err := a.hashtagRepo.IsHashtagMuted(ctx, userID, hashtag)
	if err != nil {
		return false, MapRepositoryError(err, "IsHashtagMuted", "HashtagFollow", hashtag)
	}
	return isMuted, nil
}

// RecordPatternMatch records a moderation pattern match for analytics
func (a *StorageAdapter) RecordPatternMatch(ctx context.Context, patternID string, matched bool, timestamp time.Time) error {
	if a.moderationRepo == nil {
		return fmt.Errorf("moderation repository not initialized")
	}
	err := a.moderationRepo.RecordPatternMatch(ctx, patternID, matched, timestamp)
	if err != nil {
		return MapRepositoryError(err, "RecordPatternMatch", "ModerationPattern", patternID)
	}
	return nil
}

// GetStrongestConnectionsByType retrieves the strongest federation connections by type
func (a *StorageAdapter) GetStrongestConnectionsByType(ctx context.Context, connectionType string, limit int) ([]*storage.FederationEdge, error) {
	if a.federationRepo == nil {
		return nil, fmt.Errorf("federation repository not initialized")
	}
	edges, err := a.federationRepo.GetStrongestConnectionsByType(ctx, connectionType, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetStrongestConnectionsByType", "FederationEdge", connectionType)
	}
	return edges, nil
}

// GetStatusesByLink retrieves statuses that contain a specific link
func (a *StorageAdapter) GetStatusesByLink(ctx context.Context, linkURL string, limit int) ([]any, error) {
	if a.trendingRepo == nil {
		return nil, fmt.Errorf("trending repository not initialized")
	}
	statuses, err := a.trendingRepo.GetStatusesByLink(ctx, linkURL, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetStatusesByLink", "Status", linkURL)
	}
	return statuses, nil
}



// IndexByEngagement creates an index entry for engagement-based discovery
func (a *StorageAdapter) IndexByEngagement(ctx context.Context, statusID string, bucket string) error {
	if a.trendingRepo == nil {
		return fmt.Errorf("trending repository not initialized")
	}
	err := a.trendingRepo.IndexByEngagement(ctx, statusID, bucket)
	if err != nil {
		return MapRepositoryError(err, "IndexByEngagement", "EngagementIndex", statusID)
	}
	return nil
}

// GenerateSearchSuggestions generates search suggestions based on user history and popular queries
func (a *StorageAdapter) GenerateSearchSuggestions(ctx context.Context, userID, partialQuery string, limit int) ([]string, error) {
	if a.trendingRepo == nil {
		return nil, fmt.Errorf("trending repository not initialized")
	}
	suggestions, err := a.trendingRepo.GenerateSearchSuggestions(ctx, userID, partialQuery, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GenerateSearchSuggestions", "SearchSuggestion", partialQuery)
	}
	return suggestions, nil
}

// ========== Media Analytics Methods ==========

// RecordManifestGeneration records when a media manifest is generated
func (a *StorageAdapter) RecordManifestGeneration(ctx context.Context, mediaID, format string, duration float64) error {
	if a.trendingRepo == nil {
		return fmt.Errorf("trending repository not initialized")
	}
	err := a.trendingRepo.RecordManifestGeneration(ctx, mediaID, format, duration)
	if err != nil {
		return MapRepositoryError(err, "RecordManifestGeneration", "MediaAnalytics", mediaID)
	}
	return nil
}

// RecordQualityChange records when a user changes video quality
func (a *StorageAdapter) RecordQualityChange(ctx context.Context, mediaID, userID, oldQuality, newQuality string) error {
	if a.trendingRepo == nil {
		return fmt.Errorf("trending repository not initialized")
	}
	err := a.trendingRepo.RecordQualityChange(ctx, mediaID, userID, oldQuality, newQuality)
	if err != nil {
		return MapRepositoryError(err, "RecordQualityChange", "MediaAnalytics", mediaID)
	}
	return nil
}

// RecordMediaEvent records general media streaming events
func (a *StorageAdapter) RecordMediaEvent(ctx context.Context, eventType, mediaID, userID string) error {
	if a.trendingRepo == nil {
		return fmt.Errorf("trending repository not initialized")
	}
	err := a.trendingRepo.RecordMediaEvent(ctx, eventType, mediaID, userID)
	if err != nil {
		return MapRepositoryError(err, "RecordMediaEvent", "MediaAnalytics", mediaID)
	}
	return nil
}

// GetManifestGenerationStats retrieves manifest generation statistics for a date range
func (a *StorageAdapter) GetManifestGenerationStats(ctx context.Context, format, startDate, endDate string) (map[string]int64, error) {
	if a.trendingRepo == nil {
		return nil, fmt.Errorf("trending repository not initialized")
	}
	stats, err := a.trendingRepo.GetManifestGenerationStats(ctx, format, startDate, endDate)
	if err != nil {
		return nil, MapRepositoryError(err, "GetManifestGenerationStats", "MediaAnalytics", format)
	}
	return stats, nil
}

// GetMediaEventStats retrieves general media event statistics
func (a *StorageAdapter) GetMediaEventStats(ctx context.Context, eventType, startDate, endDate string) (map[string]int64, error) {
	if a.trendingRepo == nil {
		return nil, fmt.Errorf("trending repository not initialized")
	}
	stats, err := a.trendingRepo.GetMediaEventStats(ctx, eventType, startDate, endDate)
	if err != nil {
		return nil, MapRepositoryError(err, "GetMediaEventStats", "MediaAnalytics", eventType)
	}
	return stats, nil
}

// CreateConversationMute creates a new conversation mute
func (a *StorageAdapter) CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error {
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}
	err := a.userRepo.CreateConversationMute(ctx, mute)
	if err != nil {
		return MapRepositoryError(err, "CreateConversationMute", "ConversationMute", mute.ConversationID)
	}
	return nil
}

// DeleteConversationMute removes a conversation mute
func (a *StorageAdapter) DeleteConversationMute(ctx context.Context, username, conversationID string) error {
	if a.userRepo == nil {
		return fmt.Errorf("user repository not initialized")
	}
	err := a.userRepo.DeleteConversationMute(ctx, username, conversationID)
	if err != nil {
		return MapRepositoryError(err, "DeleteConversationMute", "ConversationMute", conversationID)
	}
	return nil
}

// IsConversationMuted checks if a conversation is muted by a user
func (a *StorageAdapter) IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error) {
	if a.userRepo == nil {
		return false, fmt.Errorf("user repository not initialized")
	}
	isMuted, err := a.userRepo.IsConversationMuted(ctx, username, conversationID)
	if err != nil {
		return false, MapRepositoryError(err, "IsConversationMuted", "ConversationMute", conversationID)
	}
	return isMuted, nil
}

// GetMutedConversations retrieves all muted conversations for a user
func (a *StorageAdapter) GetMutedConversations(ctx context.Context, username string) ([]string, error) {
	if a.userRepo == nil {
		return nil, fmt.Errorf("user repository not initialized")
	}
	conversationIDs, err := a.userRepo.GetMutedConversations(ctx, username)
	if err != nil {
		return nil, MapRepositoryError(err, "GetMutedConversations", "ConversationMute", username)
	}
	return conversationIDs, nil
}

// CreateSeveredRelationship records a new severed federation relationship
func (a *StorageAdapter) CreateSeveredRelationship(ctx context.Context, rel *storage.SeveredRelationship) error {
	if a.federationRepo == nil {
		return fmt.Errorf("federation repository not initialized")
	}
	
	// Convert storage.SeveredRelationship to models.SeveredRelationship
	modelRel := &models.SeveredRelationship{
		ID:              rel.ID,
		LocalInstance:   rel.LocalInstance,
		RemoteInstance:  rel.RemoteInstance,
		Reason:          models.SeveranceReason(rel.Reason),
		Timestamp:       rel.Timestamp,
		Reversible:      rel.Reversible,
		Details:         rel.Details,
		EstimatedImpact: rel.EstimatedImpact,
		AffectedFollows: convertToModelAffectedFollows(rel.AffectedFollows),
	}
	
	err := a.federationRepo.CreateSeveredRelationship(ctx, modelRel)
	if err != nil {
		return MapRepositoryError(err, "CreateSeveredRelationship", "SeveredRelationship", rel.ID)
	}
	return nil
}

// GetSeveredRelationships retrieves severed relationships for a local instance
func (a *StorageAdapter) GetSeveredRelationships(ctx context.Context, localInstance string, limit int, cursor string) ([]*storage.SeveredRelationship, string, error) {
	if a.federationRepo == nil {
		return nil, "", fmt.Errorf("federation repository not initialized")
	}
	
	modelRels, nextCursor, err := a.federationRepo.GetSeveredRelationships(ctx, localInstance, limit, cursor)
	if err != nil {
		return nil, "", MapRepositoryError(err, "GetSeveredRelationships", "SeveredRelationship", localInstance)
	}
	
	// Convert models.SeveredRelationship to storage.SeveredRelationship
	result := make([]*storage.SeveredRelationship, len(modelRels))
	for i, rel := range modelRels {
		result[i] = &storage.SeveredRelationship{
			ID:              rel.ID,
			LocalInstance:   rel.LocalInstance,
			RemoteInstance:  rel.RemoteInstance,
			Reason:          storage.SeveranceReason(rel.Reason),
			Timestamp:       rel.Timestamp,
			Reversible:      rel.Reversible,
			Details:         rel.Details,
			EstimatedImpact: rel.EstimatedImpact,
			AffectedFollows: convertToStorageAffectedFollows(rel.AffectedFollows),
		}
	}
	
	return result, nextCursor, nil
}

// GetSeveredRelationship retrieves a specific severed relationship
func (a *StorageAdapter) GetSeveredRelationship(ctx context.Context, localInstance, remoteInstance string) (*storage.SeveredRelationship, error) {
	if a.federationRepo == nil {
		return nil, fmt.Errorf("federation repository not initialized")
	}
	
	modelRel, err := a.federationRepo.GetSeveredRelationship(ctx, localInstance, remoteInstance)
	if err != nil {
		return nil, MapRepositoryError(err, "GetSeveredRelationship", "SeveredRelationship", fmt.Sprintf("%s-%s", localInstance, remoteInstance))
	}
	
	// Convert to storage type
	result := &storage.SeveredRelationship{
		ID:              modelRel.ID,
		LocalInstance:   modelRel.LocalInstance,
		RemoteInstance:  modelRel.RemoteInstance,
		Reason:          storage.SeveranceReason(modelRel.Reason),
		Timestamp:       modelRel.Timestamp,
		Reversible:      modelRel.Reversible,
		Details:         modelRel.Details,
		EstimatedImpact: modelRel.EstimatedImpact,
		AffectedFollows: convertToStorageAffectedFollows(modelRel.AffectedFollows),
	}
	
	return result, nil
}

// UpdateSeveredRelationship updates an existing severed relationship
func (a *StorageAdapter) UpdateSeveredRelationship(ctx context.Context, rel *storage.SeveredRelationship) error {
	if a.federationRepo == nil {
		return fmt.Errorf("federation repository not initialized")
	}
	
	// Convert to model type
	modelRel := &models.SeveredRelationship{
		ID:              rel.ID,
		LocalInstance:   rel.LocalInstance,
		RemoteInstance:  rel.RemoteInstance,
		Reason:          models.SeveranceReason(rel.Reason),
		Timestamp:       rel.Timestamp,
		Reversible:      rel.Reversible,
		Details:         rel.Details,
		EstimatedImpact: rel.EstimatedImpact,
		AffectedFollows: convertToModelAffectedFollows(rel.AffectedFollows),
	}
	
	err := a.federationRepo.UpdateSeveredRelationship(ctx, modelRel)
	if err != nil {
		return MapRepositoryError(err, "UpdateSeveredRelationship", "SeveredRelationship", rel.ID)
	}
	return nil
}

// GetAffectedFollows retrieves follow relationships affected by a severance
func (a *StorageAdapter) GetAffectedFollows(ctx context.Context, localInstance, remoteInstance string) ([]storage.AffectedFollow, error) {
	if a.federationRepo == nil {
		return nil, fmt.Errorf("federation repository not initialized")
	}
	
	modelFollows, err := a.federationRepo.GetAffectedFollows(ctx, localInstance, remoteInstance)
	if err != nil {
		return nil, MapRepositoryError(err, "GetAffectedFollows", "SeveredRelationship", fmt.Sprintf("%s-%s", localInstance, remoteInstance))
	}
	
	// Convert to storage type
	return convertToStorageAffectedFollows(modelFollows), nil
}

// RecordAffectedFollow adds an affected follow to a severed relationship
func (a *StorageAdapter) RecordAffectedFollow(ctx context.Context, localInstance, remoteInstance string, follow storage.AffectedFollow) error {
	if a.federationRepo == nil {
		return fmt.Errorf("federation repository not initialized")
	}
	
	// Convert to model type
	modelFollow := models.AffectedFollow{
		LocalUser:    follow.LocalUser,
		RemoteUser:   follow.RemoteUser,
		Direction:    follow.Direction,
		LastActivity: follow.LastActivity,
	}
	
	err := a.federationRepo.RecordAffectedFollow(ctx, localInstance, remoteInstance, modelFollow)
	if err != nil {
		return MapRepositoryError(err, "RecordAffectedFollow", "SeveredRelationship", fmt.Sprintf("%s-%s", localInstance, remoteInstance))
	}
	return nil
}

// ReverseSeverance marks a severed relationship as restored
func (a *StorageAdapter) ReverseSeverance(ctx context.Context, localInstance, remoteInstance string) error {
	if a.federationRepo == nil {
		return fmt.Errorf("federation repository not initialized")
	}
	
	err := a.federationRepo.ReverseSeverance(ctx, localInstance, remoteInstance)
	if err != nil {
		return MapRepositoryError(err, "ReverseSeverance", "SeveredRelationship", fmt.Sprintf("%s-%s", localInstance, remoteInstance))
	}
	return nil
}

// GetSeveranceHistory retrieves the history of severances between two instances
func (a *StorageAdapter) GetSeveranceHistory(ctx context.Context, localInstance, remoteInstance string, limit int) ([]*storage.SeveredRelationship, error) {
	if a.federationRepo == nil {
		return nil, fmt.Errorf("federation repository not initialized")
	}
	
	modelHistory, err := a.federationRepo.GetSeveranceHistory(ctx, localInstance, remoteInstance, limit)
	if err != nil {
		return nil, MapRepositoryError(err, "GetSeveranceHistory", "SeveredRelationship", fmt.Sprintf("%s-%s", localInstance, remoteInstance))
	}
	
	// Convert to storage types
	result := make([]*storage.SeveredRelationship, len(modelHistory))
	for i, rel := range modelHistory {
		result[i] = &storage.SeveredRelationship{
			ID:              rel.ID,
			LocalInstance:   rel.LocalInstance,
			RemoteInstance:  rel.RemoteInstance,
			Reason:          storage.SeveranceReason(rel.Reason),
			Timestamp:       rel.Timestamp,
			Reversible:      rel.Reversible,
			Details:         rel.Details,
			EstimatedImpact: rel.EstimatedImpact,
			AffectedFollows: convertToStorageAffectedFollows(rel.AffectedFollows),
		}
	}
	
	return result, nil
}

// Helper functions to convert between storage and model types
func convertToModelAffectedFollows(follows []storage.AffectedFollow) []models.AffectedFollow {
	result := make([]models.AffectedFollow, len(follows))
	for i, f := range follows {
		result[i] = models.AffectedFollow{
			LocalUser:    f.LocalUser,
			RemoteUser:   f.RemoteUser,
			Direction:    f.Direction,
			LastActivity: f.LastActivity,
		}
	}
	return result
}

func convertToStorageAffectedFollows(follows []models.AffectedFollow) []storage.AffectedFollow {
	result := make([]storage.AffectedFollow, len(follows))
	for i, f := range follows {
		result[i] = storage.AffectedFollow{
			LocalUser:    f.LocalUser,
			RemoteUser:   f.RemoteUser,
			Direction:    f.Direction,
			LastActivity: f.LastActivity,
		}
	}
	return result
}

// SetNotificationPreference sets a specific notification preference
func (a *StorageAdapter) SetNotificationPreference(ctx context.Context, username string, preference string, enabled bool) error {
	if a.notificationRepo == nil {
		return fmt.Errorf("notification repository not initialized")
	}
	return a.notificationRepo.SetNotificationPreference(ctx, username, preference, enabled)
}


// RecordDeliveryAttempt records a notification delivery attempt
func (a *StorageAdapter) RecordDeliveryAttempt(ctx context.Context, notificationID, method string, success bool, errorMsg string) error {
	if a.notificationRepo == nil {
		return fmt.Errorf("notification repository not initialized")
	}
	return a.notificationRepo.RecordDeliveryAttempt(ctx, notificationID, method, success, errorMsg)
}

// GetDeliveryStatus gets the delivery status for a notification
func (a *StorageAdapter) GetDeliveryStatus(ctx context.Context, notificationID, method string) (*models.NotificationDelivery, error) {
	if a.notificationRepo == nil {
		return nil, fmt.Errorf("notification repository not initialized")
	}
	return a.notificationRepo.GetDeliveryStatus(ctx, notificationID, method)
}

// MarkDeliveryComplete marks a delivery as complete
func (a *StorageAdapter) MarkDeliveryComplete(ctx context.Context, notificationID, method string) error {
	if a.notificationRepo == nil {
		return fmt.Errorf("notification repository not initialized")
	}
	return a.notificationRepo.MarkDeliveryComplete(ctx, notificationID, method)
}

// GetFailedDeliveries gets failed delivery attempts
func (a *StorageAdapter) GetFailedDeliveries(ctx context.Context, since time.Time, limit int) ([]*models.NotificationDelivery, error) {
	if a.notificationRepo == nil {
		return nil, fmt.Errorf("notification repository not initialized")
	}
	return a.notificationRepo.GetFailedDeliveries(ctx, since, limit)
}

// RetryFailedDeliveries retries failed delivery attempts
func (a *StorageAdapter) RetryFailedDeliveries(ctx context.Context, before time.Time) error {
	if a.notificationRepo == nil {
		return fmt.Errorf("notification repository not initialized")
	}
	return a.notificationRepo.RetryFailedDeliveries(ctx, before)
}

// DeleteExpiredSubscriptions deletes expired push subscriptions
func (a *StorageAdapter) DeleteExpiredSubscriptions(ctx context.Context, before time.Time) error {
	if a.notificationRepo == nil {
		return fmt.Errorf("notification repository not initialized")
	}
	return a.notificationRepo.DeleteExpiredSubscriptions(ctx, before)
}

// UpdateLastUsed updates the last used timestamp for a push subscription
func (a *StorageAdapter) UpdateLastUsed(ctx context.Context, username, subscriptionID string) error {
	if a.notificationRepo == nil {
		return fmt.Errorf("notification repository not initialized")
	}
	return a.notificationRepo.UpdateLastUsed(ctx, username, subscriptionID)
}

// GetNotificationStats gets notification statistics for a user
func (a *StorageAdapter) GetNotificationStats(ctx context.Context, username string) (map[string]int64, error) {
	if a.notificationRepo == nil {
		return nil, fmt.Errorf("notification repository not initialized")
	}
	return a.notificationRepo.GetNotificationStats(ctx, username)
}

// ClearOldNotifications deletes notifications older than the specified duration
func (a *StorageAdapter) ClearOldNotifications(ctx context.Context, username string, olderThan time.Duration) error {
	if a.notificationRepo == nil {
		return fmt.Errorf("notification repository not initialized")
	}
	return a.notificationRepo.ClearOldNotifications(ctx, username, olderThan)
}

// ClearNotifications clears all notifications for a user
func (a *StorageAdapter) ClearNotifications(ctx context.Context, username string) error {
	if a.notificationRepo == nil {
		return fmt.Errorf("notification repository not initialized")
	}
	// Clear all notifications by marking them as read
	return a.notificationRepo.MarkAllNotificationsAsRead(ctx, username)
}

// CountUnreadNotifications counts unread notifications for a user
func (a *StorageAdapter) CountUnreadNotifications(ctx context.Context, username string) (int, error) {
	if a.notificationRepo == nil {
		return 0, fmt.Errorf("notification repository not initialized")
	}
	return a.notificationRepo.CountUnreadNotifications(ctx, username)
}

// GetFieldVerification gets field verification status
func (a *StorageAdapter) GetFieldVerification(ctx context.Context, username, fieldName string) (*storage.ActorField, error) {
	// TODO: Implement field verification when repository is ready
	return nil, fmt.Errorf("field verification not implemented")
}

// GetStatusCount gets the total number of statuses for an actor
func (a *StorageAdapter) GetStatusCount(ctx context.Context, actorID string) (int, error) {
	if a.objectRepo == nil {
		return 0, fmt.Errorf("object repository not initialized")
	}
	// TODO: Implement status count
	return 0, nil
}

// GetFollowersCount gets the total number of followers for an actor
func (a *StorageAdapter) GetFollowersCount(ctx context.Context, actorID string) (int, error) {
	if a.actorRepo == nil {
		return 0, fmt.Errorf("actor repository not initialized")
	}
	// TODO: Implement followers count
	return 0, nil
}

// GetFollowingCount gets the total number of accounts an actor is following
func (a *StorageAdapter) GetFollowingCount(ctx context.Context, actorID string) (int, error) {
	if a.actorRepo == nil {
		return 0, fmt.Errorf("actor repository not initialized")
	}
	// TODO: Implement following count
	return 0, nil
}

// GetLatestStatus gets the latest status for an actor
func (a *StorageAdapter) GetLatestStatus(ctx context.Context, actorID string) (*storage.StatusSearchResult, error) {
	if a.objectRepo == nil {
		return nil, fmt.Errorf("object repository not initialized")
	}
	// TODO: Implement latest status
	return nil, nil
}

// GetLinkedProviders gets all linked OAuth providers for a user
func (a *StorageAdapter) GetLinkedProviders(ctx context.Context, username string) ([]string, error) {
	if a.userRepo == nil {
		return nil, fmt.Errorf("user repository not initialized")
	}
	return a.userRepo.GetLinkedProviders(ctx, username)
}

// GetNotificationsByAccount gets notifications for a user filtered by a specific account
func (a *StorageAdapter) GetNotificationsByAccount(ctx context.Context, userID, accountID string, limit int) ([]*storage.Notification, error) {
	if a.notificationRepo == nil {
		return nil, fmt.Errorf("notification repository not initialized")
	}
	
	// For now, we'll get all notifications and filter by account
	// TODO: Implement a more efficient method in the repository
	notifications, _, err := a.notificationRepo.GetNotificationsByUser(ctx, userID, limit, "")
	if err != nil {
		return nil, MapRepositoryError(err, "GetNotificationsByAccount", "Notification", userID)
	}
	
	// Convert models.Notification to storage.Notification and filter by account
	var filtered []*storage.Notification
	for _, n := range notifications {
		// Filter by actor ID (who triggered the notification)
		if n.ActorID == accountID {
			notif := &storage.Notification{
				ID:        n.ID,
				Type:      n.Type,
				Username:  n.UserID, // Map UserID to Username
				AccountID: n.ActorID, // Who triggered the notification
				StatusID:  n.TargetID, // Assuming target is the status when TargetType is "status"
				Read:      n.IsRead,
				CreatedAt: n.CreatedAt,
			}
			filtered = append(filtered, notif)
		}
	}
	
	return filtered, nil
}

// Circuit Breaker methods - provide access to circuit breaker functionality

// GetCircuitBreakerRepository returns the circuit breaker repository for direct access
func (a *StorageAdapter) GetCircuitBreakerRepository() CircuitBreakerRepository {
	return a.circuitBreakerRepo
}

// GetCircuitState retrieves the current state of a circuit breaker for an instance
func (a *StorageAdapter) GetCircuitState(ctx context.Context, instanceID string) (*models.CircuitBreakerState, error) {
	if a.circuitBreakerRepo == nil {
		return nil, fmt.Errorf("circuit breaker repository not initialized")
	}
	return a.circuitBreakerRepo.GetCircuitState(ctx, instanceID)
}

// SaveCircuitState saves the circuit breaker state
func (a *StorageAdapter) SaveCircuitState(ctx context.Context, state *models.CircuitBreakerState) error {
	if a.circuitBreakerRepo == nil {
		return fmt.Errorf("circuit breaker repository not initialized")
	}
	return a.circuitBreakerRepo.SaveCircuitState(ctx, state)
}

// UpdateCircuitState updates an existing circuit state atomically
func (a *StorageAdapter) UpdateCircuitState(ctx context.Context, instanceID string, updateFn func(*models.CircuitBreakerState) error) (*models.CircuitBreakerState, error) {
	if a.circuitBreakerRepo == nil {
		return nil, fmt.Errorf("circuit breaker repository not initialized")
	}
	return a.circuitBreakerRepo.UpdateCircuitState(ctx, instanceID, updateFn)
}
