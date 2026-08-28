// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

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

// NotificationDispatcher receives callbacks after notifications are persisted.
type NotificationDispatcher interface {
	DispatchPushForNotification(ctx context.Context, notification *models.Notification)
}

// StatusFilter represents filtering criteria for admin status listing
type StatusFilter struct {
	Local      *bool      // Filter by local vs remote statuses
	Remote     *bool      // Filter by remote statuses only
	ByDomain   string     // Filter by specific domain
	Visibility string     // Filter by visibility (public, unlisted, private, direct)
	Flagged    *bool      // Filter by flagged status
	Reported   *bool      // Filter by reported status
	WithMedia  *bool      // Filter by presence of media attachments
	Sensitive  *bool      // Filter by sensitive flag
	MinDate    *time.Time // Filter by minimum creation date
	MaxDate    *time.Time // Filter by maximum creation date
}

// StatusRepository defines the interface for status/note operations
// This handles both local status creation and federated ActivityPub Note objects
type StatusRepository interface {
	// Core CRUD operations
	CreateStatus(ctx context.Context, status *models.Status) error
	CreateBoostStatus(ctx context.Context, status *models.Status) error
	GetStatus(ctx context.Context, statusID string) (*models.Status, error)
	GetStatusByURL(ctx context.Context, url string) (*models.Status, error)
	UpdateStatus(ctx context.Context, status *models.Status) error
	DeleteStatus(ctx context.Context, statusID string) error
	DeleteBoostStatus(ctx context.Context, boosterID, targetStatusID string) (*models.Status, error)

	// Timeline operations
	GetPublicTimeline(ctx context.Context, opts PaginationOptions) (*PaginatedResult[*models.Status], error)
	GetHomeTimeline(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.Status], error)
	GetUserTimeline(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.Status], error)
	GetConversationThread(ctx context.Context, conversationID string, opts PaginationOptions) (*PaginatedResult[*models.Status], error)
	GetConversationThreadReverse(ctx context.Context, conversationID string, opts PaginationOptions) (*PaginatedResult[*models.Status], error)
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

	// Count operations
	CountStatusesByAuthor(ctx context.Context, authorID string) (int, error)
	CountReplies(ctx context.Context, statusID string) (int, error)

	// Admin operations
	ListStatusesForAdmin(ctx context.Context, filter *StatusFilter, limit int, cursor string) ([]*models.Status, string, error)
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

// NOTE: MediaRepository is now defined in media.go with full method signatures

// NOTE: ConversationRepository is now defined in conversation.go with full method signatures

// NOTE: ListRepository is now defined in list.go with full method signatures

// FilterRepository defines the interface for content filter operations
// This handles user-defined content filtering rules
type FilterRepository interface {
	// Core filter operations
	CreateFilter(ctx context.Context, filter *models.Filter) error
	GetFilter(ctx context.Context, filterID string) (*models.Filter, error)
	UpdateFilter(ctx context.Context, filter *models.Filter) error

	// User filter management
	GetUserFilters(ctx context.Context, username string) ([]*models.Filter, error)
	GetActiveFilters(ctx context.Context, username string, context []string) ([]*models.Filter, error)

	// Filter keyword operations
	AddFilterKeyword(ctx context.Context, keyword *models.FilterKeyword) error
	GetFilterKeywords(ctx context.Context, filterID string) ([]*models.FilterKeyword, error)

	// Filter status operations
	AddFilterStatus(ctx context.Context, filterStatus *models.FilterStatus) error
	GetFilterStatuses(ctx context.Context, filterID string) ([]*models.FilterStatus, error)

	// Content filtering evaluation
	EvaluateFilters(ctx context.Context, username string, content string, context []string) ([]*models.Filter, error)
	CheckContentFiltered(ctx context.Context, username, statusID string, context []string) (bool, []*models.Filter, error)
}

// NotificationRepository defines the interface for notification operations
// This handles user notifications for various ActivityPub and app events
type NotificationRepository interface {
	// Dispatcher configuration
	SetDispatcher(dispatcher NotificationDispatcher)

	// Core notification operations
	CreateNotification(ctx context.Context, notification *models.Notification) error
	GetUserNotification(ctx context.Context, userID, notificationID string) (*models.Notification, error)
	UpdateNotification(ctx context.Context, notification *models.Notification) error
	DeleteUserNotification(ctx context.Context, userID, notificationID string) error

	// User notification queries
	GetUserNotifications(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.Notification], error)
	GetUnreadNotifications(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.Notification], error)
	GetNotificationsByType(ctx context.Context, userID, notificationType string, opts PaginationOptions) (*PaginatedResult[*models.Notification], error)

	// Notification status management
	MarkAllNotificationsRead(ctx context.Context, userID string) error
	MarkNotificationsReadByType(ctx context.Context, userID, notificationType string) error

	// Push notification tracking
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
	DeleteNotificationsByObject(ctx context.Context, objectID string) error
	DeleteExpiredNotifications(ctx context.Context, expiredBefore time.Time) (int64, error)

	// Filtered and advanced queries
	GetNotificationsFiltered(ctx context.Context, username string, filter map[string]interface{}) ([]*models.Notification, string, error)
	ClearOldNotifications(ctx context.Context, username string, olderThan time.Time) (int, error)
	GetNotificationsAdvanced(ctx context.Context, userID string, filters map[string]interface{}, pagination PaginationOptions) (*PaginatedResult[*models.Notification], error)

	// Notification preferences
	GetNotificationPreferences(ctx context.Context, userID string) (*models.NotificationPreferences, error)
	UpdateNotificationPreferences(ctx context.Context, prefs *models.NotificationPreferences) error
	SetNotificationPreference(ctx context.Context, userID string, preferenceType string, enabled bool) error
}

// NOTE: LikeRepository is now defined in like.go with full method signatures

// NOTE: SocialRepository is now defined in social.go with full method signatures

// QuoteRepository defines the interface for quote post operations
type QuoteRepository interface {
	// Core quote operations
	CreateQuoteRelationship(ctx context.Context, relationship *models.QuoteRelationship) error
	GetQuoteRelationship(ctx context.Context, quoteStatusID, targetStatusID string) (*models.QuoteRelationship, error)
	UpdateQuoteRelationship(ctx context.Context, relationship *models.QuoteRelationship) error
	DeleteQuoteRelationship(ctx context.Context, quoteStatusID, targetStatusID string) error

	// Quote discovery
	GetQuotesForStatus(ctx context.Context, statusID string, opts PaginationOptions) (*PaginatedResult[*models.QuoteRelationship], error)
	GetQuotesByUser(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.QuoteRelationship], error)

	// Quote permissions
	CreateQuotePermissions(ctx context.Context, permissions *models.QuotePermissions) error
	GetQuotePermissions(ctx context.Context, username string) (*models.QuotePermissions, error)
	UpdateQuotePermissions(ctx context.Context, permissions *models.QuotePermissions) error
	DeleteQuotePermissions(ctx context.Context, username string) error

	// Quote counts and statistics
	GetQuoteCount(ctx context.Context, statusID string) (int64, error)
	IncrementQuoteCount(ctx context.Context, statusID string) error
	DecrementQuoteCount(ctx context.Context, statusID string) error

	// Quote withdrawal operations
	WithdrawQuotes(ctx context.Context, noteID, userID string) (int, error)
}

// RepositoryRegistry provides access to all repository interfaces
// This allows services to access storage operations through a single interface
type RepositoryRegistry interface {
	Status() StatusRepository
	Account() AccountRepository
	Relationship() RelationshipRepository
	Media() MediaRepository
	Conversation() ConversationRepository
	List() ListRepository
	Filter() FilterRepository
	Notification() NotificationRepository
	Like() LikeRepository
	Social() SocialRepository
	Quote() QuoteRepository
}
