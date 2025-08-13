// Package interfaces defines repository interfaces for the Lesser project's API alignment.
// These interfaces provide technology-agnostic contracts between services and storage,
// supporting clean architecture patterns and testability.
package interfaces

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// PaginationOptions represents pagination parameters for repository queries
type PaginationOptions struct {
	Limit  int    // Maximum number of items to return
	Cursor string // Pagination cursor/token for the next page
	Since  *time.Time
	Until  *time.Time
}

// PaginatedResult represents a paginated result set
type PaginatedResult[T any] struct {
	Items      []T    // The actual data items
	NextCursor string // Cursor for the next page (empty if no more pages)
	HasMore    bool   // Whether there are more pages available
	Total      int64  // Total count (if available, -1 if not calculated)
}

// StatusRepository defines the interface for status/note operations
// This handles both local status creation and federated ActivityPub Note objects
type StatusRepository interface {
	// Core CRUD operations
	CreateStatus(ctx context.Context, status *models.Status) error
	GetStatus(ctx context.Context, statusID string) (*models.Status, error)
	GetStatusByURL(ctx context.Context, url string) (*models.Status, error)
	UpdateStatus(ctx context.Context, status *models.Status) error
	DeleteStatus(ctx context.Context, statusID string) error

	// Timeline operations
	GetPublicTimeline(ctx context.Context, opts PaginationOptions) (*PaginatedResult[*models.Status], error)
	GetHomeTimeline(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.Status], error)
	GetUserTimeline(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.Status], error)
	GetConversationThread(ctx context.Context, conversationID string, opts PaginationOptions) (*PaginatedResult[*models.Status], error)
	GetReplies(ctx context.Context, parentStatusID string, opts PaginationOptions) (*PaginatedResult[*models.Status], error)

	// Search and discovery
	SearchStatuses(ctx context.Context, query string, opts PaginationOptions) (*PaginatedResult[*models.Status], error)
	GetStatusesByHashtag(ctx context.Context, hashtag string, opts PaginationOptions) (*PaginatedResult[*models.Status], error)
	GetTrendingStatuses(ctx context.Context, opts PaginationOptions) (*PaginatedResult[*models.Status], error)

	// Engagement operations
	LikeStatus(ctx context.Context, userID, statusID string) error
	UnlikeStatus(ctx context.Context, userID, statusID string) error
	ReblogStatus(ctx context.Context, userID, statusID string, reblogStatusID string) error
	UnreblogStatus(ctx context.Context, userID, statusID string) error
	BookmarkStatus(ctx context.Context, userID, statusID string) error
	UnbookmarkStatus(ctx context.Context, userID, statusID string) error

	// Moderation operations
	FlagStatus(ctx context.Context, statusID, reason string, reportedBy string) error
	UnflagStatus(ctx context.Context, statusID string) error
	GetFlaggedStatuses(ctx context.Context, opts PaginationOptions) (*PaginatedResult[*models.Status], error)

	// Batch operations for performance
	GetStatusesByIDs(ctx context.Context, statusIDs []string) ([]*models.Status, error)
	GetStatusCounts(ctx context.Context, statusID string) (likes, reblogs, replies int, err error)

	// Context and metadata
	GetStatusContext(ctx context.Context, statusID string) (ancestors, descendants []*models.Status, err error)
	GetStatusEngagement(ctx context.Context, statusID, userID string) (liked, reblogged, bookmarked bool, err error)
}

// AccountRepository defines the interface for user/actor operations
// This handles both local users and federated remote actors
type AccountRepository interface {
	// Core account operations
	CreateAccount(ctx context.Context, account *storage.Account) error
	GetAccount(ctx context.Context, username string) (*storage.Account, error)
	GetAccountByURL(ctx context.Context, actorURL string) (*storage.Account, error)
	GetAccountByEmail(ctx context.Context, email string) (*storage.Account, error)
	UpdateAccount(ctx context.Context, account *storage.Account) error
	DeleteAccount(ctx context.Context, username string) error

	// Account discovery and search
	SearchAccounts(ctx context.Context, query string, opts PaginationOptions) (*PaginatedResult[*storage.Account], error)
	GetSuggestedAccounts(ctx context.Context, forUserID string, opts PaginationOptions) (*PaginatedResult[*storage.AccountSuggestion], error)
	GetFeaturedAccounts(ctx context.Context, opts PaginationOptions) (*PaginatedResult[*storage.Account], error)

	// Account verification and moderation
	ApproveAccount(ctx context.Context, username string) error
	SuspendAccount(ctx context.Context, username string, reason string) error
	UnsuspendAccount(ctx context.Context, username string) error
	SilenceAccount(ctx context.Context, username string, reason string) error
	UnsilenceAccount(ctx context.Context, username string) error

	// Account metadata and preferences
	UpdateAccountPreferences(ctx context.Context, username string, preferences map[string]interface{}) error
	GetAccountPreferences(ctx context.Context, username string) (map[string]interface{}, error)
	UpdateAccountFeatures(ctx context.Context, username string, features map[string]bool) error
	GetAccountFeatures(ctx context.Context, username string) (map[string]bool, error)

	// Authentication and session management
	ValidateCredentials(ctx context.Context, username, password string) (*storage.Account, error)
	UpdatePassword(ctx context.Context, username, newPasswordHash string) error
	CreatePasswordReset(ctx context.Context, reset *storage.PasswordReset) error
	GetPasswordReset(ctx context.Context, token string) (*storage.PasswordReset, error)
	UsePasswordReset(ctx context.Context, token string) error

	// Activity and usage tracking
	RecordLogin(ctx context.Context, attempt *storage.LoginAttempt) error
	GetLoginHistory(ctx context.Context, username string, opts PaginationOptions) (*PaginatedResult[*storage.LoginAttempt], error)
	UpdateLastActivity(ctx context.Context, username string, activity time.Time) error

	// Bookmark operations
	AddBookmark(ctx context.Context, username, objectID string) error
	RemoveBookmark(ctx context.Context, username, objectID string) error
	GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]*storage.Bookmark, string, error)
	GetBookmarkedStatuses(ctx context.Context, username string, opts PaginationOptions) (*PaginatedResult[*models.Status], error)

	// Batch operations
	GetAccountsByUsernames(ctx context.Context, usernames []string) ([]*storage.Account, error)
	GetAccountsCount(ctx context.Context) (int64, error)
}

// RelationshipRepository defines the interface for follow/relationship operations
// This handles both local and federated follow relationships
type RelationshipRepository interface {
	// Core relationship operations
	CreateFollowRequest(ctx context.Context, followerID, followingID string) error
	AcceptFollowRequest(ctx context.Context, followerID, followingID string) error
	RejectFollowRequest(ctx context.Context, followerID, followingID string) error
	Unfollow(ctx context.Context, followerID, followingID string) error

	// Relationship queries
	IsFollowing(ctx context.Context, followerID, followingID string) (bool, error)
	GetFollowStatus(ctx context.Context, followerID, followingID string) (string, error) // pending, accepted, rejected, none
	GetFollowRelationship(ctx context.Context, followerID, followingID string) (*models.RelationshipRecord, error)

	// Follower/Following lists
	GetFollowers(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*storage.Account], error)
	GetFollowing(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*storage.Account], error)
	GetFollowRequests(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*storage.Account], error)
	GetPendingFollowRequests(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*storage.Account], error)

	// Mutual relationships
	GetMutualFollows(ctx context.Context, userID, otherUserID string, opts PaginationOptions) (*PaginatedResult[*storage.Account], error)

	// Blocking operations
	BlockUser(ctx context.Context, blockerID, blockedID string) error
	UnblockUser(ctx context.Context, blockerID, blockedID string) error
	IsBlocked(ctx context.Context, blockerID, blockedID string) (bool, error)
	GetBlockedUsers(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*storage.Account], error)

	// Muting operations
	MuteUser(ctx context.Context, muterID, mutedID string) error
	UnmuteUser(ctx context.Context, muterID, mutedID string) error
	IsMuted(ctx context.Context, muterID, mutedID string) (bool, error)
	GetMutedUsers(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*storage.Account], error)

	// Relationship counts
	GetFollowerCount(ctx context.Context, userID string) (int64, error)
	GetFollowingCount(ctx context.Context, userID string) (int64, error)
	GetMutualFollowCount(ctx context.Context, userID, otherUserID string) (int64, error)

	// Batch operations
	GetRelationships(ctx context.Context, requestingUserID string, targetUserIDs []string) (map[string]*models.RelationshipRecord, error)
}

// MediaRepository defines the interface for media/attachment operations
// This handles file uploads, processing, and CDN management
type MediaRepository interface {
	// Core media operations
	CreateMedia(ctx context.Context, media *models.Media) error
	GetMedia(ctx context.Context, mediaID string) (*models.Media, error)
	UpdateMedia(ctx context.Context, media *models.Media) error
	DeleteMedia(ctx context.Context, mediaID string) error

	// Media processing
	MarkMediaProcessing(ctx context.Context, mediaID string) error
	MarkMediaReady(ctx context.Context, mediaID string) error
	MarkMediaFailed(ctx context.Context, mediaID, errorMsg string) error
	GetPendingMedia(ctx context.Context, opts PaginationOptions) (*PaginatedResult[*models.Media], error)
	GetProcessingMedia(ctx context.Context, opts PaginationOptions) (*PaginatedResult[*models.Media], error)

	// Media variants and thumbnails
	AddMediaVariant(ctx context.Context, mediaID, variantName string, variant models.MediaVariant) error
	GetMediaVariant(ctx context.Context, mediaID, variantName string) (*models.MediaVariant, error)
	DeleteMediaVariant(ctx context.Context, mediaID, variantName string) error

	// User media queries
	GetUserMedia(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.Media], error)
	GetUserMediaByType(ctx context.Context, userID, contentType string, opts PaginationOptions) (*PaginatedResult[*models.Media], error)
	GetUnusedMedia(ctx context.Context, olderThan time.Time, opts PaginationOptions) (*PaginatedResult[*models.Media], error)

	// Media usage tracking
	MarkMediaUsed(ctx context.Context, mediaID string) error
	GetMediaUsageStats(ctx context.Context, mediaID string) (usageCount int, lastUsed *time.Time, err error)

	// Content moderation
	SetMediaModeration(ctx context.Context, mediaID string, isNSFW bool, score float64, labels []string) error
	GetModerationPendingMedia(ctx context.Context, opts PaginationOptions) (*PaginatedResult[*models.Media], error)

	// Batch operations
	GetMediaByIDs(ctx context.Context, mediaIDs []string) ([]*models.Media, error)
	DeleteExpiredMedia(ctx context.Context, expiredBefore time.Time) (int64, error)

	// Storage and CDN operations
	GetMediaStorageUsage(ctx context.Context, userID string) (int64, error)
	GetTotalStorageUsage(ctx context.Context) (int64, error)
}

// ConversationRepository defines the interface for direct message conversation operations
type ConversationRepository interface {
	// Core conversation operations
	CreateConversation(ctx context.Context, conversation *models.Conversation, participants []string) error
	GetConversation(ctx context.Context, conversationID string) (*models.Conversation, error)
	UpdateConversation(ctx context.Context, conversation *models.Conversation) error
	DeleteConversation(ctx context.Context, conversationID string) error

	// Conversation discovery
	GetUserConversations(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.Conversation], error)
	GetConversationByParticipants(ctx context.Context, participants []string) (*models.Conversation, error)

	// Conversation management
	AddParticipant(ctx context.Context, conversationID, participantID string) error
	RemoveParticipant(ctx context.Context, conversationID, participantID string) error
	GetConversationParticipants(ctx context.Context, conversationID string) ([]string, error)

	// Conversation status tracking
	MarkConversationRead(ctx context.Context, conversationID, userID string) error
	MarkConversationUnread(ctx context.Context, conversationID, userID string) error
	GetUnreadConversations(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.Conversation], error)

	// Conversation muting
	CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error
	DeleteConversationMute(ctx context.Context, username, conversationID string) error

	// Conversation search
	SearchConversations(ctx context.Context, userID, query string, opts PaginationOptions) (*PaginatedResult[*models.Conversation], error)
}

// ListRepository defines the interface for user list operations
// This handles Mastodon-style user-created lists for timeline organization
type ListRepository interface {
	// Core list operations
	CreateList(ctx context.Context, list *models.List) error
	GetList(ctx context.Context, listID string) (*models.List, error)
	UpdateList(ctx context.Context, list *models.List) error
	DeleteList(ctx context.Context, listID string) error

	// User list management
	GetUserLists(ctx context.Context, username string, opts PaginationOptions) (*PaginatedResult[*models.List], error)
	GetListsByMember(ctx context.Context, memberUsername string, opts PaginationOptions) (*PaginatedResult[*models.List], error)

	// List membership operations
	AddListMember(ctx context.Context, listID, memberUsername string) error
	RemoveListMember(ctx context.Context, listID, memberUsername string) error
	GetListMembers(ctx context.Context, listID string, opts PaginationOptions) (*PaginatedResult[*storage.Account], error)
	IsListMember(ctx context.Context, listID, memberUsername string) (bool, error)

	// List timeline operations
	GetListTimeline(ctx context.Context, listID string, opts PaginationOptions) (*PaginatedResult[*models.Status], error)
	GetListStatuses(ctx context.Context, listID string, opts PaginationOptions) (*PaginatedResult[*models.Status], error)
}

// FilterRepository defines the interface for content filter operations
// This handles user-defined content filtering rules
type FilterRepository interface {
	// Core filter operations
	CreateFilter(ctx context.Context, filter *models.Filter) error
	GetFilter(ctx context.Context, filterID string) (*models.Filter, error)
	UpdateFilter(ctx context.Context, filter *models.Filter) error
	DeleteFilter(ctx context.Context, filterID string) error

	// User filter management
	GetUserFilters(ctx context.Context, username string) ([]*models.Filter, error)
	GetActiveFilters(ctx context.Context, username string, context []string) ([]*models.Filter, error)

	// Filter keyword operations
	AddFilterKeyword(ctx context.Context, keyword *models.FilterKeyword) error
	RemoveFilterKeyword(ctx context.Context, keywordID string) error
	GetFilterKeywords(ctx context.Context, filterID string) ([]*models.FilterKeyword, error)

	// Filter status operations
	AddFilterStatus(ctx context.Context, filterStatus *models.FilterStatus) error
	RemoveFilterStatus(ctx context.Context, filterStatusID string) error
	GetFilterStatuses(ctx context.Context, filterID string) ([]*models.FilterStatus, error)

	// Content filtering evaluation
	EvaluateFilters(ctx context.Context, username string, content string, context []string) ([]*models.Filter, error)
	CheckContentFiltered(ctx context.Context, username, statusID string, context []string) (bool, []*models.Filter, error)
}

// NotificationRepository defines the interface for notification operations
// This handles user notifications for various ActivityPub and app events
type NotificationRepository interface {
	// Core notification operations
	CreateNotification(ctx context.Context, notification *models.Notification) error
	GetNotification(ctx context.Context, notificationID string) (*models.Notification, error)
	UpdateNotification(ctx context.Context, notification *models.Notification) error
	DeleteNotification(ctx context.Context, notificationID string) error

	// User notification queries
	GetUserNotifications(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.Notification], error)
	GetUnreadNotifications(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.Notification], error)
	GetNotificationsByType(ctx context.Context, userID, notificationType string, opts PaginationOptions) (*PaginatedResult[*models.Notification], error)

	// Notification status management
	MarkNotificationRead(ctx context.Context, notificationID string) error
	MarkNotificationUnread(ctx context.Context, notificationID string) error
	MarkAllNotificationsRead(ctx context.Context, userID string) error
	MarkNotificationsReadByType(ctx context.Context, userID, notificationType string) error

	// Push notification tracking
	MarkNotificationPushSent(ctx context.Context, notificationID string) error
	MarkNotificationPushFailed(ctx context.Context, notificationID, errorMsg string) error
	GetPendingPushNotifications(ctx context.Context, opts PaginationOptions) (*PaginatedResult[*models.Notification], error)

	// Notification grouping and consolidation
	GetNotificationGroups(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.Notification], error)
	ConsolidateNotifications(ctx context.Context, groupKey string) error

	// Notification counts and summaries
	GetUnreadNotificationCount(ctx context.Context, userID string) (int64, error)
	GetNotificationCountsByType(ctx context.Context, userID string) (map[string]int64, error)

	// Batch operations
	CreateNotifications(ctx context.Context, notifications []*models.Notification) error
	DeleteNotificationsByType(ctx context.Context, userID, notificationType string) error
	DeleteExpiredNotifications(ctx context.Context, expiredBefore time.Time) (int64, error)
}

// LikeRepository defines the interface for like operations
type LikeRepository interface {
	CreateLike(ctx context.Context, actor, object string) (*models.Like, error)
	DeleteLike(ctx context.Context, actor, object string) error
	GetObjectLikes(ctx context.Context, objectID string, limit int, cursor string) ([]*models.Like, string, error)
	GetActorLikes(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Like, string, error)
}

// SocialRepository defines the interface for social interaction operations
type SocialRepository interface {
	CreateAnnounce(ctx context.Context, announce *storage.Announce) error
	DeleteAnnounce(ctx context.Context, actor, object string) error
	GetStatusAnnounces(ctx context.Context, statusID string, limit int, cursor string) ([]*storage.Announce, string, error)
	CreateStatusPin(ctx context.Context, pin *storage.StatusPin) error
	DeleteStatusPin(ctx context.Context, userID, statusID string) error
}
