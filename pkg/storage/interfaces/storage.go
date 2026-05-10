// Package interfaces provides comprehensive storage interfaces that bridge legacy storage patterns
// with the new repository-based DynamORM approach for the Lesser ActivityPub server.
//
// This is the foundation interface for Phase 1 completion of the DynamORM migration initiative.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
)

// RepositoryAccess provides access to all DynamORM repositories
// This replaces the core.RepositoryStorage embedding to avoid circular imports
type RepositoryAccess interface {
	// Repository access methods - return interface{} to allow flexibility with actual repository types
	Account() interface{}
	Actor() interface{}
	Object() interface{}
	Activity() interface{}
	Timeline() interface{}
	Notification() interface{}
	Like() interface{}
	Moderation() interface{}
	List() interface{}
	Media() interface{}
	MediaMetadata() interface{}
	Poll() interface{}
	PushSubscription() interface{}
	Hashtag() interface{}
	ScheduledStatus() interface{}
	Announcement() interface{}
	DomainBlock() interface{}
	Relationship() interface{}
	Instance() interface{}
	Federation() interface{}
	Recovery() interface{}
	Analytics() interface{} // Analytics/Trending repository
	Social() interface{}
	User() interface{}
	Status() interface{}
	Cost() interface{}
	WebSocketCost() interface{}
	Trust() interface{}
	Search() interface{}
	Relay() interface{}
	CommunityNote() interface{}
	Emoji() interface{}
	RateLimit() interface{}
	Conversation() interface{}
	Marker() interface{}
	FeaturedTag() interface{}
	AI() interface{}
	Export() interface{}
	Import() interface{}
	DLQ() interface{}
	MetricRecord() interface{}
	CloudWatchMetrics() interface{}
	StreamingCloudWatch() interface{}
	Audit() interface{}
	OAuth() interface{}
	DNSCache() interface{}
	Filter() interface{}

	// Utility methods
	GetDB() interface{}
	GetTableName() string
	GetLogger() interface{}
}

// Storage represents the comprehensive storage interface that combines repository access
// with legacy storage operations still used throughout the Lesser application.
//
// This interface bridges the gap between the legacy storage patterns and the new
// repository-based DynamORM approach, ensuring all existing functionality continues
// to work during the migration process.
//
// CRITICAL PATTERNS PRESERVED:
// - Users: PK=`USER#username`, SK=`PROFILE`
// - Actors: PK=`ACTOR#username`, SK=`PROFILE`
// - Objects: PK=`object#id`, SK=`object#id`
// - DNS Cache: PK=`DNSCACHE#hostname`, SK=`ENTRY`
// - All existing GSI patterns and TTL logic
type Storage interface {
	// Repository Access - provides access to all DynamORM repositories
	RepositoryAccess

	// =======================================
	// ACTOR MANAGEMENT OPERATIONS
	// =======================================

	// Actor lifecycle operations
	CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error
	GetActor(ctx context.Context, username string) (*activitypub.Actor, error)
	UpdateActor(ctx context.Context, actor *activitypub.Actor) error
	DeleteActor(ctx context.Context, username string) error
	GetActorByID(ctx context.Context, actorID string) (*activitypub.Actor, error)

	// Actor key management
	GetActorPrivateKey(ctx context.Context, username string) (string, error)
	UpdateActorKeys(ctx context.Context, username, publicKey, privateKey string) error

	// =======================================
	// USER MANAGEMENT OPERATIONS
	// =======================================

	// User lifecycle operations
	CreateUser(ctx context.Context, user interface{}) error
	GetUser(ctx context.Context, username string) (interface{}, error)
	UpdateUser(ctx context.Context, user interface{}) error
	DeleteUser(ctx context.Context, username string) error
	GetUserByID(ctx context.Context, userID string) (interface{}, error)
	GetUserByEmail(ctx context.Context, email string) (interface{}, error)

	// User preferences and settings
	GetUserPreferences(ctx context.Context, username string) (interface{}, error)
	UpdateUserPreferences(ctx context.Context, username string, preferences interface{}) error

	// User validation and authentication
	ValidateCredentials(ctx context.Context, username, password string) (interface{}, error)
	UpdatePassword(ctx context.Context, username, hashedPassword string) error

	// =======================================
	// OBJECT MANAGEMENT OPERATIONS
	// =======================================

	// Object lifecycle operations
	CreateObject(ctx context.Context, object interface{}) error
	GetObject(ctx context.Context, objectID string) (interface{}, error)
	UpdateObject(ctx context.Context, objectID string, object interface{}) error
	DeleteObject(ctx context.Context, objectID string) error
	TombstoneObject(ctx context.Context, objectID, actorID string) error

	// Object metadata and relationships
	IncrementReplyCount(ctx context.Context, objectID string) error
	DecrementReplyCount(ctx context.Context, objectID string) error
	GetObjectMetadata(ctx context.Context, objectID string) (interface{}, error)

	// Object history and versioning
	GetObjectHistory(ctx context.Context, objectID string) ([]interface{}, error)
	RestoreObjectVersion(ctx context.Context, objectID string, version int) error

	// =======================================
	// ACTIVITY MANAGEMENT OPERATIONS
	// =======================================

	// Activity lifecycle operations
	StoreActivity(ctx context.Context, activity *activitypub.Activity) error
	CreateActivity(ctx context.Context, activity *activitypub.Activity) error
	GetActivity(ctx context.Context, activityID string) (*activitypub.Activity, error)
	UpdateActivity(ctx context.Context, activity *activitypub.Activity) error
	DeleteActivity(ctx context.Context, activityID string) error

	// Activity processing and routing
	ProcessInboundActivity(ctx context.Context, activity *activitypub.Activity, fromDomain string) error
	ProcessOutboundActivity(ctx context.Context, activity *activitypub.Activity) error
	GetActivitiesByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*activitypub.Activity, string, error)

	// =======================================
	// RELATIONSHIP MANAGEMENT OPERATIONS
	// =======================================

	// Follow relationships
	CreateRelationship(ctx context.Context, followerUsername, followingID, activityID string) error
	RemoveRelationship(ctx context.Context, followerUsername, followingID string) error
	IsFollowing(ctx context.Context, followerUsername, followingID string) (bool, error)
	GetRelationship(ctx context.Context, followerUsername, followingID string) (interface{}, error)
	UpdateRelationshipStatus(ctx context.Context, followerUsername, followingID string, status string) error

	// Follow requests
	CreateFollowRequest(ctx context.Context, followerUsername, followingID, activityID string) error
	AcceptFollowRequest(ctx context.Context, followerUsername, followingID, activityID string) error
	RejectFollowRequest(ctx context.Context, followerUsername, followingID string) error
	GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]interface{}, string, error)

	// Follower/following operations
	FollowActor(ctx context.Context, followerUsername, targetUsername string) error
	UnfollowActor(ctx context.Context, followerUsername, targetUsername string) error
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	GetFollowersCount(ctx context.Context, username string) (int64, error)
	GetFollowingCount(ctx context.Context, username string) (int64, error)

	// Blocking and muting
	BlockActor(ctx context.Context, blockerUsername, blockedUsername string) error
	UnblockActor(ctx context.Context, blockerUsername, blockedUsername string) error
	IsBlocked(ctx context.Context, blockerUsername, blockedUsername string) (bool, error)
	GetBlockedUsers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)

	MuteActor(ctx context.Context, muterUsername, mutedUsername string) error
	UnmuteActor(ctx context.Context, muterUsername, mutedUsername string) error
	IsMuted(ctx context.Context, muterUsername, mutedUsername string) (bool, error)
	GetMutedUsers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)

	// =======================================
	// LIKE AND FAVORITE OPERATIONS
	// =======================================

	// Like operations
	CreateLike(ctx context.Context, actorID, objectID, activityID string) error
	RemoveLike(ctx context.Context, actorID, objectID string) error
	HasLiked(ctx context.Context, actorID, objectID string) (bool, error)
	GetLikeCount(ctx context.Context, objectID string) (int64, error)
	GetLikedObjects(ctx context.Context, actorID string, limit int, cursor string) ([]string, string, error)
	GetObjectLikes(ctx context.Context, objectID string, limit int, cursor string) ([]interface{}, string, error)

	// =======================================
	// TIMELINE OPERATIONS
	// =======================================

	// Timeline management
	GetTimeline(ctx context.Context, username string, limit int, cursor string) ([]interface{}, string, error)
	GetHomeTimeline(ctx context.Context, username string, limit int, cursor string) ([]interface{}, string, error)
	GetPublicTimeline(ctx context.Context, limit int, cursor string) ([]interface{}, string, error)
	GetHashtagTimeline(ctx context.Context, hashtag string, limit int, cursor string) ([]interface{}, string, error)
	GetUserTimeline(ctx context.Context, username string, limit int, cursor string) ([]interface{}, string, error)

	// Timeline entry management
	AddToTimeline(ctx context.Context, username string, objectID string, activityType string) error
	RemoveFromTimeline(ctx context.Context, username, objectID string) error
	RemoveFromTimelines(ctx context.Context, objectID string) error
	FanOutPost(ctx context.Context, activity *activitypub.Activity) error

	// =======================================
	// NOTIFICATION OPERATIONS
	// =======================================

	// Notification management
	CreateNotification(ctx context.Context, notification interface{}) error
	GetNotifications(ctx context.Context, username string, limit int, cursor string) ([]interface{}, string, error)
	GetUnreadNotificationCount(ctx context.Context, username string) (int64, error)
	MarkAllNotificationsAsRead(ctx context.Context, username string) error
	DeleteNotificationsByObject(ctx context.Context, objectID string) error

	// =======================================
	// SEARCH OPERATIONS
	// =======================================

	// Search functionality
	SearchUsers(ctx context.Context, query string, limit int, cursor string) ([]interface{}, string, error)
	SearchStatuses(ctx context.Context, query string, limit int, cursor string) ([]interface{}, string, error)
	SearchHashtags(ctx context.Context, query string, limit int, cursor string) ([]interface{}, string, error)
	SearchAll(ctx context.Context, query string, limit int, cursor string) (interface{}, string, error)

	// Search suggestions and trending
	GetSearchSuggestions(ctx context.Context, query string, limit int) ([]interface{}, error)
	GetTrendingHashtags(ctx context.Context, limit int) ([]interface{}, error)
	GetTrendingStatuses(ctx context.Context, limit int, cursor string) ([]interface{}, string, error)

	// =======================================
	// SESSION AND AUTHENTICATION OPERATIONS
	// =======================================

	// Session management
	CreateSession(ctx context.Context, session interface{}) error
	GetSession(ctx context.Context, sessionID string) (interface{}, error)
	UpdateSession(ctx context.Context, session interface{}) error
	DeleteSession(ctx context.Context, sessionID string) error
	CleanupExpiredSessions(ctx context.Context) error
	GetUserSessions(ctx context.Context, username string) ([]interface{}, error)

	// Token management
	ValidateToken(ctx context.Context, token string) (interface{}, error)
	CreateAccessToken(ctx context.Context, token interface{}) error
	GetAccessToken(ctx context.Context, tokenID string) (interface{}, error)
	RevokeAccessToken(ctx context.Context, tokenID string) error
	CleanupExpiredTokens(ctx context.Context) error

	// OAuth operations
	CreateOAuthState(ctx context.Context, state interface{}) error
	GetOAuthState(ctx context.Context, state string) (interface{}, error)
	DeleteOAuthState(ctx context.Context, state string) error
	CreateOAuthClient(ctx context.Context, client interface{}) error
	GetOAuthClient(ctx context.Context, clientID string) (interface{}, error)

	// =======================================
	// MEDIA OPERATIONS
	// =======================================

	// Media attachment management
	CreateMediaAttachment(ctx context.Context, media interface{}) error
	GetMediaAttachment(ctx context.Context, mediaID string) (interface{}, error)
	UpdateMediaAttachment(ctx context.Context, media interface{}) error
	DeleteMediaAttachment(ctx context.Context, mediaID string) error
	GetMediaAttachmentsByUser(ctx context.Context, username string, limit int, cursor string) ([]interface{}, string, error)

	// Media processing and metadata
	QueueMediaProcessing(ctx context.Context, mediaID string, processingType interface{}) error
	UpdateMediaProcessingStatus(ctx context.Context, mediaID string, status interface{}, metadata map[string]interface{}) error
	GetMediaMetadata(ctx context.Context, mediaID string) (interface{}, error)

	// =======================================
	// FEDERATION OPERATIONS
	// =======================================

	// Federation instance management
	CreateFederationInstance(ctx context.Context, instance interface{}) error
	GetFederationInstance(ctx context.Context, domain string) (interface{}, error)
	UpdateFederationInstance(ctx context.Context, instance interface{}) error
	GetAllFederationInstances(ctx context.Context, limit int, cursor string) ([]interface{}, string, error)

	// Federation activity tracking
	RecordFederationActivity(ctx context.Context, activity interface{}) error
	GetFederationActivities(ctx context.Context, domain string, limit int, cursor string) ([]interface{}, string, error)
	GetFederationStatistics(ctx context.Context, domain string, since time.Time) (interface{}, error)

	// Federation health and monitoring
	UpdateFederationHealth(ctx context.Context, domain string, isHealthy bool, responseTime time.Duration) error
	GetFederationHealth(ctx context.Context, domain string) (interface{}, error)
	GetUnhealthyFederationInstances(ctx context.Context, limit int) ([]interface{}, error)

	// =======================================
	// MODERATION OPERATIONS
	// =======================================

	// Report management
	CreateReport(ctx context.Context, report interface{}) error
	GetReport(ctx context.Context, reportID string) (interface{}, error)
	UpdateReport(ctx context.Context, report interface{}) error
	GetReportsByStatus(ctx context.Context, status interface{}, limit int, cursor string) ([]interface{}, string, error)
	GetReportsByUser(ctx context.Context, username string, limit int, cursor string) ([]interface{}, string, error)

	// Moderation queue operations
	GetModerationQueue(ctx context.Context, filter interface{}) ([]interface{}, error)
	CreateModerationDecision(ctx context.Context, decision interface{}) error
	GetModerationDecision(ctx context.Context, contentID string) (interface{}, error)
	UpdateModerationDecision(ctx context.Context, contentID string, decision interface{}) error

	// Content flagging
	CreateFlag(ctx context.Context, flag interface{}) error
	GetFlags(ctx context.Context, objectID string) ([]interface{}, error)
	GetFlagsByUser(ctx context.Context, username string, limit int, cursor string) ([]interface{}, string, error)

	// Domain blocking
	CreateDomainBlock(ctx context.Context, domain string, reason string) error
	RemoveDomainBlock(ctx context.Context, domain string) error
	IsDomainBlocked(ctx context.Context, domain string) (bool, error)
	GetDomainBlocks(ctx context.Context, limit int, cursor string) ([]interface{}, string, error)

	// =======================================
	// DNS AND CACHING OPERATIONS
	// =======================================

	// DNS cache management (PK=`DNSCACHE#hostname`, SK=`ENTRY`)
	SetDNSCache(ctx context.Context, hostname string, record interface{}) error
	GetDNSCache(ctx context.Context, hostname string) (interface{}, error)
	DeleteDNSCache(ctx context.Context, hostname string) error
	CleanupExpiredDNSCache(ctx context.Context) error

	// General cache operations
	SetCache(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	GetCache(ctx context.Context, key string, value interface{}) error
	DeleteCache(ctx context.Context, key string) error
	ClearCache(ctx context.Context, pattern string) error

	// =======================================
	// COST TRACKING OPERATIONS
	// =======================================

	// DynamoDB cost tracking
	TrackDynamoRead(ctx context.Context, tableName string, units int64) error
	TrackDynamoWrite(ctx context.Context, tableName string, units int64) error
	TrackDynamoOperation(ctx context.Context, operation interface{}) error
	GetCostSummary(ctx context.Context, since time.Time) (interface{}, error)
	GetDailyCostSummary(ctx context.Context, date time.Time) (interface{}, error)

	// AWS service cost tracking
	TrackAWSCost(ctx context.Context, service string, operation string, cost float64) error
	TrackLambdaInvocation(ctx context.Context, functionName string, duration time.Duration) error
	TrackS3Operation(ctx context.Context, bucket string, operation string, bytes int64) error

	// Cost alerting and monitoring
	GetCostAlerts(ctx context.Context) ([]interface{}, error)
	CreateCostAlert(ctx context.Context, alert interface{}) error
	UpdateCostAlert(ctx context.Context, alertID string, alert interface{}) error

	// =======================================
	// ANALYTICS AND METRICS OPERATIONS
	// =======================================

	// Activity analytics
	RecordActivity(ctx context.Context, activityType, actorID string, timestamp time.Time) error
	RecordInstanceActivity(ctx context.Context, activityType string, timestamp time.Time) error
	RecordHashtagUsage(ctx context.Context, hashtag, objectID, actorID string) error
	RecordLinkShare(ctx context.Context, link, objectID, actorID string) error
	RecordStatusEngagement(ctx context.Context, objectID, engagementType, actorID string) error

	// Instance metrics
	GetInstanceMetrics(ctx context.Context, since time.Time) (interface{}, error)
	GetUserActivityMetrics(ctx context.Context, username string, since time.Time) (interface{}, error)
	GetContentMetrics(ctx context.Context, since time.Time) (interface{}, error)

	// =======================================
	// SCHEDULED OPERATIONS
	// =======================================

	// Scheduled status operations
	CreateScheduledStatus(ctx context.Context, scheduled interface{}) error
	GetScheduledStatus(ctx context.Context, id string) (interface{}, error)
	GetScheduledStatuses(ctx context.Context, username string, limit int, cursor string) ([]interface{}, string, error)
	UpdateScheduledStatus(ctx context.Context, scheduled interface{}) error
	DeleteScheduledStatus(ctx context.Context, id string) error
	GetDueScheduledStatuses(ctx context.Context, before time.Time, limit int) ([]interface{}, error)
	MarkScheduledStatusPublished(ctx context.Context, id string) error

	// Background job management
	CreateBackgroundJob(ctx context.Context, job interface{}) error
	GetBackgroundJob(ctx context.Context, jobID string) (interface{}, error)
	UpdateBackgroundJobStatus(ctx context.Context, jobID string, status interface{}, result interface{}) error
	GetPendingBackgroundJobs(ctx context.Context, jobType string, limit int) ([]interface{}, error)

	// =======================================
	// LIST OPERATIONS
	// =======================================

	// List management
	CreateList(ctx context.Context, list interface{}) error
	GetList(ctx context.Context, listID string) (interface{}, error)
	UpdateList(ctx context.Context, list interface{}) error
	DeleteList(ctx context.Context, listID string) error
	GetListsByUser(ctx context.Context, username string, limit int, cursor string) ([]interface{}, string, error)

	// List membership
	AddListMember(ctx context.Context, listID, username string) error
	RemoveListMember(ctx context.Context, listID, username string) error
	GetListMembers(ctx context.Context, listID string, limit int, cursor string) ([]interface{}, string, error)
	IsListMember(ctx context.Context, listID, username string) (bool, error)
	GetUserLists(ctx context.Context, username string) ([]interface{}, error)

	// =======================================
	// POLL OPERATIONS
	// =======================================

	// Poll management
	CreatePoll(ctx context.Context, poll interface{}) error
	GetPoll(ctx context.Context, pollID string) (interface{}, error)
	UpdatePoll(ctx context.Context, poll interface{}) error
	GetPollsByUser(ctx context.Context, username string, limit int, cursor string) ([]interface{}, string, error)

	// Poll voting
	CastPollVote(ctx context.Context, vote interface{}) error
	GetPollVote(ctx context.Context, pollID, voterID string) (interface{}, error)
	GetPollResults(ctx context.Context, pollID string) (interface{}, error)

	// =======================================
	// HASHTAG OPERATIONS
	// =======================================

	// Hashtag management
	CreateHashtag(ctx context.Context, hashtag interface{}) error
	GetHashtag(ctx context.Context, name string) (interface{}, error)
	UpdateHashtag(ctx context.Context, hashtag interface{}) error
	IncrementHashtagUsage(ctx context.Context, name string) error
	GetHashtagsByPopularity(ctx context.Context, limit int, since time.Time) ([]interface{}, error)

	// Hashtag following
	FollowHashtag(ctx context.Context, username, hashtagName string) error
	UnfollowHashtag(ctx context.Context, username, hashtagName string) error
	IsFollowingHashtag(ctx context.Context, username, hashtagName string) (bool, error)
	GetFollowedHashtags(ctx context.Context, username string, limit int, cursor string) ([]*storage.HashtagFollow, string, error)
	MuteHashtag(ctx context.Context, username, hashtagName string, until *time.Time) error
	UnmuteHashtag(ctx context.Context, username, hashtagName string) error
	IsHashtagMuted(ctx context.Context, username, hashtagName string) (bool, error)
	GetHashtagNotificationSettings(ctx context.Context, username, hashtagName string) (*storage.HashtagNotificationSettings, error)
	UpdateHashtagNotificationSettings(ctx context.Context, username, hashtagName string, settings *storage.HashtagNotificationSettings) error

	// =======================================
	// ANNOUNCEMENT OPERATIONS
	// =======================================

	// Instance announcement management
	CreateAnnouncement(ctx context.Context, announcement interface{}) error
	GetAnnouncement(ctx context.Context, announcementID string) (interface{}, error)
	UpdateAnnouncement(ctx context.Context, announcement interface{}) error
	DeleteAnnouncement(ctx context.Context, announcementID string) error
	GetActiveAnnouncements(ctx context.Context) ([]interface{}, error)
	GetAllAnnouncements(ctx context.Context, limit int, cursor string) ([]interface{}, string, error)

	// User announcement interactions
	DismissAnnouncement(ctx context.Context, username, announcementID string) error
	AddAnnouncementReaction(ctx context.Context, username, announcementID, emoji string) error
	RemoveAnnouncementReaction(ctx context.Context, username, announcementID, emoji string) error

	// =======================================
	// INFRASTRUCTURE MONITORING OPERATIONS
	// =======================================

	// Health checks and monitoring
	GetInfrastructureHealth(ctx context.Context) (*model.InfrastructureStatus, error)
	GetInstanceBudgets(ctx context.Context, exceeded *bool) ([]*model.InstanceBudget, error)
	GetInstanceHealthReport(ctx context.Context, domain string) (*model.InstanceHealthReport, error)
	GetInstanceRelationships(ctx context.Context, domain string) (*model.InstanceRelations, error)

	// Database health monitoring
	GetDatabaseStatus(ctx context.Context) (interface{}, error)
	GetDatabaseMetrics(ctx context.Context, since time.Time) (interface{}, error)
	RecordDatabaseError(ctx context.Context, operation string, err error) error

	// Service health monitoring
	RecordServiceHealth(ctx context.Context, service string, status interface{}, metrics map[string]interface{}) error
	GetServiceHealth(ctx context.Context, service string) (interface{}, error)
	GetAllServiceHealth(ctx context.Context) ([]interface{}, error)

	// =======================================
	// TRANSACTION SUPPORT
	// =======================================

	// Transaction management for atomic operations
	BeginTransaction(ctx context.Context) (Transaction, error)
	ExecuteInTransaction(ctx context.Context, fn func(tx Transaction) error) error

	// =======================================
	// UTILITY AND METADATA OPERATIONS
	// =======================================

	// Storage metadata and configuration
	GetStorageInfo(ctx context.Context) (interface{}, error)
	GetStorageStatistics(ctx context.Context) (interface{}, error)

	// Maintenance operations
	PerformMaintenance(ctx context.Context, operation interface{}) error
	GetMaintenanceStatus(ctx context.Context) (interface{}, error)

	// Migration support
	GetMigrationStatus(ctx context.Context) (interface{}, error)
	UpdateMigrationProgress(ctx context.Context, step string, progress int) error
}

// Transaction interface for atomic operations across multiple storage operations
// This provides ACID transaction support for complex multi-step operations
type Transaction interface {
	// Commit applies all transaction operations atomically
	Commit() error

	// Rollback discards all transaction operations
	Rollback() error

	// GetContext returns the transaction context for operation scoping
	GetContext() context.Context

	// IsActive returns true if the transaction is still active (not committed or rolled back)
	IsActive() bool

	// Transaction-aware storage operations
	// These operations are queued and executed atomically on Commit()

	// Actor operations within transaction
	TxCreateActor(actor *activitypub.Actor, privateKey string) error
	TxUpdateActor(actor *activitypub.Actor) error
	TxDeleteActor(username string) error

	// User operations within transaction
	TxCreateUser(user interface{}) error
	TxUpdateUser(user interface{}) error
	TxDeleteUser(username string) error

	// Object operations within transaction
	TxCreateObject(object interface{}) error
	TxUpdateObject(objectID string, object interface{}) error
	TxDeleteObject(objectID string) error

	// Activity operations within transaction
	TxCreateActivity(activity *activitypub.Activity) error
	TxUpdateActivity(activity *activitypub.Activity) error
	TxDeleteActivity(activityID string) error

	// Relationship operations within transaction
	TxCreateRelationship(followerUsername, followingID, activityID string) error
	TxRemoveRelationship(followerUsername, followingID string) error
	TxUpdateRelationshipStatus(followerUsername, followingID string, status string) error
}

// StorageOptions provides configuration options for storage implementations
type StorageOptions struct {
	// TableName is the DynamoDB table name for single-table design
	TableName string

	// Region is the AWS region for DynamoDB operations
	Region string

	// EnableCostTracking enables DynamoDB cost tracking and monitoring
	EnableCostTracking bool

	// EnableTransactions enables transaction support for atomic operations
	EnableTransactions bool

	// CacheConfig provides caching configuration for performance optimization
	CacheConfig *CacheConfig

	// MetricsConfig provides metrics and monitoring configuration
	MetricsConfig *MetricsConfig
}

// CacheConfig provides caching configuration options
type CacheConfig struct {
	// EnableCache enables in-memory caching for frequently accessed data
	EnableCache bool

	// CacheTTL is the default time-to-live for cached items
	CacheTTL time.Duration

	// MaxCacheSize is the maximum number of items to store in cache
	MaxCacheSize int

	// CacheKeyPrefix is the prefix for all cache keys
	CacheKeyPrefix string
}

// MetricsConfig provides metrics and monitoring configuration
type MetricsConfig struct {
	// EnableMetrics enables metrics collection and reporting
	EnableMetrics bool

	// MetricsPrefix is the prefix for all metric names
	MetricsPrefix string

	// EnableDetailedMetrics enables detailed operation-level metrics
	EnableDetailedMetrics bool

	// MetricsInterval is the interval for metrics aggregation and reporting
	MetricsInterval time.Duration
}

// StorageFactory provides methods for creating storage implementations
type StorageFactory interface {
	// CreateStorage creates a new storage implementation with the given options
	CreateStorage(options *StorageOptions) (Storage, error)

	// CreateTransactionStorage creates a storage implementation with transaction support
	CreateTransactionStorage(options *StorageOptions) (Storage, error)

	// CreateRepositoryStorage creates a repository-only storage implementation
	CreateRepositoryStorage(options *StorageOptions) (RepositoryAccess, error)
}

// Additional repository interfaces not defined in repositories.go
// These are the repositories that are referenced in the Storage interface
// but don't have definitions in the existing repositories.go file

// NOTE: ObjectRepository is now defined in object.go with full method signatures

// NOTE: ActivityRepository is now defined in activity.go with full method signatures

// NOTE: ModerationRepository is now defined in moderation.go with full method signatures

// NOTE: MediaMetadataRepository is now defined in media_metadata.go with full method signatures

// NOTE: PollRepository is now defined in poll.go with full method signatures

// NOTE: PushSubscriptionRepository is now defined in push_subscription.go with full method signatures

// NOTE: HashtagRepository is now defined in hashtag.go with full method signatures

// NOTE: ScheduledStatusRepository is now defined in scheduled_status.go with full method signatures

// NOTE: AnnouncementRepository is now defined in announcement.go with full method signatures

// NOTE: DomainBlockRepository is now defined in domain_block.go with full method signatures

// NOTE: InstanceRepository is now defined in instance.go with full method signatures

// NOTE: FederationRepository is now defined in federation.go with full method signatures

// NOTE: RecoveryRepository is now defined in recovery.go with full method signatures

// NOTE: TrendingRepository is now defined in trending.go with full method signatures

// UserRepository provides methods for user data management.
// This interface mirrors the public methods of repositories.UserRepository,
// enabling mock implementations for unit testing.
type UserRepository interface {
	// Core CRUD operations
	CreateUser(ctx context.Context, user *storage.User) error
	GetUser(ctx context.Context, username string) (*storage.User, error)
	GetUserByEmail(ctx context.Context, email string) (*storage.User, error)
	UpdateUser(ctx context.Context, username string, updates map[string]any) error
	DeleteUser(ctx context.Context, username string) error

	ListUsers(ctx context.Context, limit int32, cursor string) ([]*storage.User, string, error)
	ListAgents(ctx context.Context, limit int32, cursor string) ([]*storage.User, string, error)
	ListUsersByRole(ctx context.Context, role string) ([]*storage.User, error)

	// Count operations
	GetActiveUserCount(ctx context.Context, days int) (int64, error)
	GetTotalUserCount(ctx context.Context) (int64, error)

	// OAuth provider operations
	GetUserByProviderID(ctx context.Context, provider, providerID string) (*storage.User, error)
	LinkProviderAccount(ctx context.Context, username, provider, providerID string) error
	UnlinkProviderAccount(ctx context.Context, username, provider string) error
	GetLinkedProviders(ctx context.Context, username string) ([]string, error)

	// Account pins (endorsed accounts)
	CreateAccountPin(ctx context.Context, pin *storage.AccountPin) error
	DeleteAccountPin(ctx context.Context, username, pinnedActorID string) error
	GetAccountPins(ctx context.Context, username string) ([]*storage.AccountPin, error)
	IsAccountPinned(ctx context.Context, username, actorID string) (bool, error)

	// Account notes
	CreateAccountNote(ctx context.Context, note *storage.AccountNote) error
	GetAccountNote(ctx context.Context, username, targetActorID string) (*storage.AccountNote, error)
	UpdateAccountNote(ctx context.Context, note *storage.AccountNote) error
	DeleteAccountNote(ctx context.Context, username, targetActorID string) error

	// Reputation operations
	StoreReputation(ctx context.Context, actorID string, reputation *storage.Reputation) error
	GetReputation(ctx context.Context, actorID string) (*storage.Reputation, error)
	GetReputationHistory(ctx context.Context, actorID string, limit int) ([]*storage.Reputation, error)
	GetUserTrustScore(ctx context.Context, userID string) (float64, error)

	// Vouch operations
	CreateVouch(ctx context.Context, vouch *storage.Vouch) error
	GetVouch(ctx context.Context, vouchID string) (*storage.Vouch, error)
	GetVouchesByActor(ctx context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error)
	GetVouchesForActor(ctx context.Context, actorID string, activeOnly bool) ([]*storage.Vouch, error)
	UpdateVouchStatus(ctx context.Context, vouchID string, active bool, revokedAt *time.Time) error
	GetMonthlyVouchCount(ctx context.Context, actorID string, year int, month time.Month) (int, error)

	// Trust relationship operations
	CreateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error
	GetTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error)
	UpdateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error
	DeleteTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) error
	GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error)
	GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error)
	GetAllTrustRelationships(ctx context.Context, limit int) ([]*storage.TrustRelationship, error)

	// Trust score operations
	GetTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error)
	UpdateTrustScore(ctx context.Context, score *storage.TrustScore) error
	RecordTrustUpdate(ctx context.Context, update *storage.TrustUpdate) error

	// User preferences operations
	GetUserLanguagePreference(ctx context.Context, username string) (string, error)
	SetUserLanguagePreference(ctx context.Context, username string, language string) error
	GetUserPreferences(ctx context.Context, username string) (*storage.UserPreferences, error)
	UpdateUserPreferences(ctx context.Context, username string, preferences *storage.UserPreferences) error
	SetPreference(ctx context.Context, username, key string, value any) error
	GetPreference(ctx context.Context, username, key string) (any, error)
	GetAllPreferences(ctx context.Context, username string) (map[string]any, error)
	UpdatePreferences(ctx context.Context, username string, preferences map[string]any) error

	// Follow operations
	AcceptFollow(ctx context.Context, followerUsername, followedUsername string) error
	RejectFollow(ctx context.Context, followerUsername, followedUsername string) error
	GetFollowRequestState(ctx context.Context, followerID, targetID string) (string, error)
	GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	RemoveFromFollowers(ctx context.Context, username, followerUsername string) error

	// Conversation mute operations
	CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error
	DeleteConversationMute(ctx context.Context, username, conversationID string) error
	IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error)
	GetMutedConversations(ctx context.Context, username string) ([]string, error)

	// Notification operations
	IsNotificationMuted(ctx context.Context, userID, targetID string) (bool, error)

	// Remote actor caching
	CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error

	// Bookmark operations
	CreateBookmark(ctx context.Context, username, objectID string) error
	RemoveBookmark(ctx context.Context, username, objectID string) error
	GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	IsBookmarked(ctx context.Context, username, objectID string) (bool, error)

	// Timeline operations
	DeleteFromTimeline(ctx context.Context, timelineType, timelineID, entryID string) error
	DeleteExpiredTimelineEntries(ctx context.Context, before time.Time) error
	GetDirectTimeline(ctx context.Context, username string, limit int, cursor string) ([]*storage.TimelineEntry, string, error)
	GetHashtagTimeline(ctx context.Context, hashtag string, local bool, limit int, cursor string) ([]*storage.TimelineEntry, string, error)
	GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*storage.TimelineEntry, string, error)

	// Fan-out operations
	FanOutPost(ctx context.Context, activity *activitypub.Activity) error
}

// NOTE: TrackingRepository is now defined in tracking.go with full method signatures

// NOTE: WebSocketCostRepository is now defined in websocket_cost.go with full method signatures

// TrustRepository provides methods for trust relationship management.
// This handles trust relationships between actors, trust scores, and trust updates.
type TrustRepository interface {
	// ===== Trust Relationship Operations =====

	// CreateTrustRelationship creates or updates a trust relationship between two actors
	CreateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error

	// GetTrustRelationship retrieves a specific trust relationship
	GetTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) (*storage.TrustRelationship, error)

	// UpdateTrustRelationship updates an existing trust relationship
	UpdateTrustRelationship(ctx context.Context, relationship *storage.TrustRelationship) error

	// DeleteTrustRelationship removes a trust relationship
	DeleteTrustRelationship(ctx context.Context, trusterID, trusteeID, category string) error

	// GetTrustRelationships retrieves all trust relationships for a truster with pagination
	GetTrustRelationships(ctx context.Context, trusterID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error)

	// GetTrustedByRelationships retrieves all relationships where the actor is trusted with pagination
	GetTrustedByRelationships(ctx context.Context, trusteeID string, limit int, cursor string) ([]*storage.TrustRelationship, string, error)

	// GetAllTrustRelationships retrieves all trust relationships for admin visualization
	GetAllTrustRelationships(ctx context.Context, limit int) ([]*storage.TrustRelationship, error)

	// ===== Trust Score Operations =====

	// GetTrustScore retrieves a cached trust score or calculates it
	GetTrustScore(ctx context.Context, actorID, category string) (*storage.TrustScore, error)

	// UpdateTrustScore updates a cached trust score
	UpdateTrustScore(ctx context.Context, score *storage.TrustScore) error

	// GetUserTrustScore retrieves the trust score for a user
	GetUserTrustScore(ctx context.Context, userID string) (float64, error)

	// ===== Trust Update Operations =====

	// RecordTrustUpdate records a trust score update event
	RecordTrustUpdate(ctx context.Context, update *storage.TrustUpdate) error
}

// NOTE: The following repository interfaces are defined in their own files:
// - FeaturedTagRepository (featured_tag.go)
// - AIRepository (ai.go)
// - ExportRepository (export.go)
// - ImportRepository (import.go)
// - DLQRepository (dlq.go)
// - MetricRecordRepository (metric_record.go)
// - CloudWatchMetricsRepository (cloudwatch_metrics.go)
// - StreamingCloudWatchRepository (streaming_cloudwatch.go)
// - AuditRepository (audit.go)
// - OAuthRepository (oauth.go)
// - DNSCacheRepository (dns_cache.go)
// - ThreadRepository (thread.go)
// - SeveranceRepository (severance.go)
// - ModerationMLRepository (moderation_ml.go)
