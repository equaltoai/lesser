// Package core provides core storage interfaces and repository access patterns for the DynamORM migration.
package core

import (
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

// RepositoryStorage is the new minimal storage interface that exposes only repository access
// This replaces the massive Storage interface with a clean, repository-focused approach
type RepositoryStorage interface {
	// Repository access methods - only expose the core repositories that are actually used
	Account() *repositories.AccountRepository
	Bookmark() *repositories.BookmarkRepository
	Actor() interfaces.ActorRepository               // Returns interface type for mockability
	Object() interfaces.ObjectRepository             // Returns interface type for mockability
	Activity() interfaces.ActivityRepository         // Returns interface type for mockability
	Timeline() interfaces.TimelineRepository         // Returns interface type for mockability
	Notification() interfaces.NotificationRepository // Returns interface type for mockability
	Like() *repositories.LikeRepository
	Moderation() interfaces.ModerationRepository // Returns interface type for mockability
	List() *repositories.ListRepository
	Media() *repositories.MediaRepository
	MediaMetadata() *repositories.MediaMetadataRepository
	Poll() *repositories.PollRepository
	PushSubscription() *repositories.PushSubscriptionRepository
	Hashtag() *repositories.HashtagRepository
	ScheduledStatus() *repositories.ScheduledStatusRepository
	Announcement() *repositories.AnnouncementRepository
	DomainBlock() *repositories.DomainBlockRepository
	Relationship() interfaces.ConcreteRelationshipRepository // Returns interface type for mockability
	Instance() *repositories.InstanceRepository
	Federation() *repositories.FederationRepository
	Recovery() *repositories.RecoveryRepository
	Analytics() *repositories.TrendingRepository // Analytics/Trending repository
	Social() *repositories.SocialRepository
	User() interfaces.UserRepository     // Returns interface type for mockability
	Status() interfaces.StatusRepository // Returns interface type for mockability
	Cost() *repositories.TrackingRepository
	WebSocketCost() *repositories.WebSocketCostRepository
	Trust() interfaces.TrustRepository
	Search() *repositories.SearchRepository
	Relay() *repositories.RelayRepository
	CommunityNote() *repositories.CommunityNoteRepository
	Emoji() *repositories.EmojiRepository
	RateLimit() *repositories.RateLimitRepository
	Conversation() *repositories.ConversationRepository
	Marker() *repositories.MarkerRepository
	FeaturedTag() *repositories.FeaturedTagRepository
	AI() *repositories.AIRepository
	Export() *repositories.ExportRepository
	Import() *repositories.ImportRepository
	DLQ() *repositories.DLQRepository
	MetricRecord() *repositories.MetricRecordRepository
	CloudWatchMetrics() *repositories.CloudWatchMetricsRepository
	StreamingCloudWatch() *repositories.StreamingCloudWatchRepository
	Audit() *repositories.AuditRepository
	OAuth() *repositories.OAuthRepository
	Skill() interfaces.SkillRepository
	DNSCache() *repositories.DNSCacheRepository
	Filter() *repositories.FilterRepository
	Thread() *repositories.ThreadRepository
	Severance() *repositories.SeveranceRepository
	ModerationML() *repositories.ModerationMLRepository
	Quote() *repositories.QuoteRepository
	MediaAnalytics() interfaces.MediaAnalyticsRepository
	MediaPopularity() interfaces.MediaPopularityRepository
	MediaSession() interfaces.MediaSessionRepository
	StreamingConnection() interfaces.StreamingConnectionRepository

	// CMS Repositories (interface types for mockability)
	Article() interfaces.ArticleRepository
	Draft() interfaces.DraftRepository
	Revision() interfaces.RevisionRepository
	Series() interfaces.SeriesRepository
	Category() interfaces.CategoryRepository
	Publication() interfaces.PublicationRepository
	PublicationMember() interfaces.PublicationMemberRepository

	// Utility methods
	GetDB() dynamormCore.DB
	GetTableName() string
	GetLogger() *zap.Logger
}
