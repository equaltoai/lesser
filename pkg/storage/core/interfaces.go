// Package core provides core storage interfaces and repository access patterns for the DynamORM migration.
package core

import (
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// RepositoryStorage is the new minimal storage interface that exposes only repository access
// This replaces the massive Storage interface with a clean, repository-focused approach
type RepositoryStorage interface {
	// Repository access methods - only expose the core repositories that are actually used
	Account() *repositories.AccountRepository
	Actor() *repositories.ActorRepository
	Object() *repositories.ObjectRepository
	Activity() *repositories.ActivityRepository
	Timeline() *repositories.TimelineRepository
	Notification() *repositories.NotificationRepository
	Like() *repositories.LikeRepository
	Moderation() *repositories.ModerationRepository
	List() *repositories.ListRepository
	Media() *repositories.MediaRepository
	MediaMetadata() *repositories.MediaMetadataRepository
	Poll() *repositories.PollRepository
	PushSubscription() *repositories.PushSubscriptionRepository
	Hashtag() *repositories.HashtagRepository
	ScheduledStatus() *repositories.ScheduledStatusRepository
	Announcement() *repositories.AnnouncementRepository
	DomainBlock() *repositories.DomainBlockRepository
	Relationship() *repositories.RelationshipRepository
	Instance() *repositories.InstanceRepository
	Federation() *repositories.FederationRepository
	Recovery() *repositories.RecoveryRepository
	Analytics() *repositories.TrendingRepository // Analytics/Trending repository
	Social() *repositories.SocialRepository
	User() *repositories.UserRepository
	Status() *repositories.StatusRepository
	Cost() *repositories.TrackingRepository
	WebSocketCost() *repositories.WebSocketCostRepository
	Trust() *repositories.TrustRepository
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
	DNSCache() *repositories.DNSCacheRepository
	Filter() *repositories.FilterRepository
	Thread() *repositories.ThreadRepository
	Severance() *repositories.SeveranceRepository
	ModerationML() *repositories.ModerationMLRepository

	// Utility methods
	GetDB() dynamormCore.DB
	GetTableName() string
	GetLogger() *zap.Logger
}
