// Package factory provides repository factory implementation for centralized storage dependency management.
package factory

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// RepositoryFactory manages all repository instances and their dependencies
// and implements the RepositoryStorage interface
type RepositoryFactory struct {
	db        dynamormCore.DB
	tableName string
	logger    *zap.Logger
	cfg       *config.Config
	awsConfig aws.Config

	// Repository instances (initialize once)
	accountRepo          *repositories.AccountRepository
	actorRepo            *repositories.ActorRepository
	objectRepo           *repositories.ObjectRepository
	activityRepo         *repositories.ActivityRepository
	userRepo             *repositories.UserRepository
	trustRepo            *repositories.TrustRepository
	conversationRepo     *repositories.ConversationRepository
	timelineRepo         *repositories.TimelineRepository
	notificationRepo     *repositories.NotificationRepository
	likeRepo             *repositories.LikeRepository
	moderationRepo       *repositories.ModerationRepository
	relationshipRepo     *repositories.RelationshipRepository
	listRepo             *repositories.ListRepository
	mediaRepo            *repositories.MediaRepository
	pollRepo             *repositories.PollRepository
	pushSubscriptionRepo *repositories.PushSubscriptionRepository
	instanceRepo         *repositories.InstanceRepository
	hashtagRepo          *repositories.HashtagRepository
	scheduledStatusRepo  *repositories.ScheduledStatusRepository
	announcementRepo     *repositories.AnnouncementRepository
	domainBlockRepo      *repositories.DomainBlockRepository
	federationRepo       *repositories.FederationRepository
	recoveryRepo         *repositories.RecoveryRepository
	analyticsRepo        *repositories.TrendingRepository
	socialRepo           *repositories.SocialRepository
	statusRepo           *repositories.StatusRepository
	costRepo             *repositories.CostTrackingRepository
	searchRepo           *repositories.SearchRepository
	relayRepo            *repositories.RelayRepository
	communityNoteRepo    *repositories.CommunityNoteRepository
	emojiRepo            *repositories.EmojiRepository
	rateLimitRepo        *repositories.RateLimitRepository
	markerRepo           *repositories.MarkerRepository
	featuredTagRepo      *repositories.FeaturedTagRepository
	aiRepo               *repositories.AIRepository
	exportRepo           *repositories.ExportRepository
	importRepo           *repositories.ImportRepository
	dlqRepo              *repositories.DLQRepository
	metricRecordRepo     *repositories.MetricRecordRepository
	cloudWatchMetricsRepo *repositories.CloudWatchMetricsRepository
}

// NewRepositoryFactory creates a new repository factory with all repositories initialized
func NewRepositoryFactory(db dynamormCore.DB, tableName string, awsConfig aws.Config, logger *zap.Logger) (*RepositoryFactory, error) {
	cfg := config.Get()

	factory := &RepositoryFactory{
		db:        db,
		tableName: tableName,
		logger:    logger,
		cfg:       cfg,
		awsConfig: awsConfig,
	}

	// Initialize all repositories
	factory.initializeRepositories()

	// Set up dependencies after all repositories are created
	factory.setupDependencies()

	return factory, nil
}

// initializeRepositories creates repository instances for core functionality
// This mirrors exactly what's initialized in cmd/api/main.go
func (f *RepositoryFactory) initializeRepositories() {
	// Core repositories from main.go (only these are actually used)
	f.accountRepo = repositories.NewAccountRepository(f.db, f.tableName, f.cfg.Domain, f.logger)
	f.actorRepo = repositories.NewActorRepository(f.db, f.tableName, f.logger)
	f.objectRepo = repositories.NewObjectRepository(f.db, f.tableName, f.cfg.Domain, f.logger)
	f.activityRepo = repositories.NewActivityRepository(f.db, f.tableName, f.logger)
	f.userRepo = repositories.NewUserRepository(f.db, f.tableName, f.logger)
	f.timelineRepo = repositories.NewTimelineRepository(f.db, f.tableName, f.logger)
	f.notificationRepo = repositories.NewNotificationRepository(f.db, f.tableName, f.logger)
	f.likeRepo = repositories.NewLikeRepository(f.db, f.tableName, f.logger)
	f.moderationRepo = repositories.NewModerationRepository(f.db, f.tableName, f.logger)
	f.listRepo = repositories.NewListRepository(f.db, f.tableName, f.logger)
	f.mediaRepo = repositories.NewMediaRepository(f.db, f.tableName, f.logger)
	f.pollRepo = repositories.NewPollRepository(f.db, f.tableName, f.logger)
	f.pushSubscriptionRepo = repositories.NewPushSubscriptionRepository(f.db, f.tableName, f.logger)
	f.hashtagRepo = repositories.NewHashtagRepository(f.db, f.tableName, f.logger, f.cfg.Domain)
	f.scheduledStatusRepo = repositories.NewScheduledStatusRepository(f.db, f.tableName, f.logger)
	f.announcementRepo = repositories.NewAnnouncementRepository(f.db, f.tableName, f.logger)
	f.domainBlockRepo = repositories.NewDomainBlockRepository(f.db, f.tableName, f.logger)
	f.relationshipRepo = repositories.NewRelationshipRepository(f.db, f.tableName, f.logger)
	f.instanceRepo = repositories.NewInstanceRepository(f.db, f.tableName, f.logger)
	f.federationRepo = repositories.NewFederationRepository(f.db, f.logger)
	f.recoveryRepo = repositories.NewRecoveryRepository(f.db, f.tableName, f.logger)
	f.analyticsRepo = repositories.NewTrendingRepository(f.db, f.logger)
	f.socialRepo = repositories.NewSocialRepository(f.db, f.logger)
	f.statusRepo = repositories.NewStatusRepository(f.db, f.tableName, f.logger)
	f.costRepo = repositories.NewCostTrackingRepository(f.db, f.tableName, f.logger)
	f.trustRepo = repositories.NewTrustRepository(f.db, f.logger)
	f.searchRepo = repositories.NewSearchRepository(f.db, f.logger)
	f.relayRepo = repositories.NewRelayRepository(f.db, f.tableName, f.logger)
	f.communityNoteRepo = repositories.NewCommunityNoteRepository(f.db, f.tableName, f.logger)
	f.emojiRepo = repositories.NewEmojiRepository(f.db, f.logger)
	f.rateLimitRepo = repositories.NewRateLimitRepository(f.db, f.tableName, f.logger)
	f.markerRepo = repositories.NewMarkerRepository(f.db, f.tableName, f.logger)
	f.featuredTagRepo = repositories.NewFeaturedTagRepository(f.db, f.tableName, f.logger)
	f.aiRepo = repositories.NewAIRepository(f.db, f.tableName, f.logger)
	f.exportRepo = repositories.NewExportRepository(f.db, f.tableName, f.logger)
	f.importRepo = repositories.NewImportRepository(f.db, f.tableName, f.logger)
	f.dlqRepo = repositories.NewDLQRepository(f.db, f.tableName, f.logger)
	f.metricRecordRepo = repositories.NewMetricRecordRepository(f.db, f.tableName, f.logger)
	f.cloudWatchMetricsRepo = repositories.NewCloudWatchMetricsRepository(f.awsConfig, "Lesser/Production", "prod", f.logger)

	// All other repositories are nil until needed/implemented
	// This allows the factory to be created without breaking the application
}

// setupDependencies configures repository dependencies after all repositories are created
func (f *RepositoryFactory) setupDependencies() {
	// Set up scheduled status repository dependency on media repository
	if f.scheduledStatusRepo != nil && f.mediaRepo != nil {
		f.scheduledStatusRepo.SetMediaRepository(f.mediaRepo)
	}
	
	// Additional repository dependencies can be configured here as needed.
	// Currently, only the ScheduledStatusRepository requires the MediaRepository dependency.
	// All other repositories are self-contained and don't require cross-repository dependencies.
}

// Getter methods for each repository type

// Account returns the Account repository instance
func (f *RepositoryFactory) Account() *repositories.AccountRepository {
	return f.accountRepo
}

// Actor returns the Actor repository instance
func (f *RepositoryFactory) Actor() *repositories.ActorRepository {
	return f.actorRepo
}

// Object returns the Object repository instance
func (f *RepositoryFactory) Object() *repositories.ObjectRepository {
	return f.objectRepo
}

// Activity returns the Activity repository instance
func (f *RepositoryFactory) Activity() *repositories.ActivityRepository {
	return f.activityRepo
}

// User returns the User repository instance
func (f *RepositoryFactory) User() *repositories.UserRepository {
	return f.userRepo
}

// Trust returns the Trust repository instance
func (f *RepositoryFactory) Trust() *repositories.TrustRepository {
	return f.trustRepo
}

// Conversation returns the Conversation repository instance
func (f *RepositoryFactory) Conversation() *repositories.ConversationRepository {
	return f.conversationRepo
}

// Timeline returns the Timeline repository instance
func (f *RepositoryFactory) Timeline() *repositories.TimelineRepository {
	return f.timelineRepo
}

// Notification returns the Notification repository instance
func (f *RepositoryFactory) Notification() *repositories.NotificationRepository {
	return f.notificationRepo
}

// Like returns the Like repository instance
func (f *RepositoryFactory) Like() *repositories.LikeRepository {
	return f.likeRepo
}

// Moderation returns the Moderation repository instance
func (f *RepositoryFactory) Moderation() *repositories.ModerationRepository {
	return f.moderationRepo
}

// Relationship returns the Relationship repository instance
func (f *RepositoryFactory) Relationship() *repositories.RelationshipRepository {
	return f.relationshipRepo
}

// List returns the List repository instance
func (f *RepositoryFactory) List() *repositories.ListRepository {
	return f.listRepo
}

// Media returns the Media repository instance
func (f *RepositoryFactory) Media() *repositories.MediaRepository {
	return f.mediaRepo
}

// Poll returns the Poll repository instance
func (f *RepositoryFactory) Poll() *repositories.PollRepository {
	return f.pollRepo
}

// PushSubscription returns the PushSubscription repository instance
func (f *RepositoryFactory) PushSubscription() *repositories.PushSubscriptionRepository {
	return f.pushSubscriptionRepo
}

// Instance returns the Instance repository instance
func (f *RepositoryFactory) Instance() *repositories.InstanceRepository {
	return f.instanceRepo
}

// Hashtag returns the Hashtag repository instance
func (f *RepositoryFactory) Hashtag() *repositories.HashtagRepository {
	return f.hashtagRepo
}

// ScheduledStatus returns the ScheduledStatus repository instance
func (f *RepositoryFactory) ScheduledStatus() *repositories.ScheduledStatusRepository {
	return f.scheduledStatusRepo
}

// Announcement returns the Announcement repository instance
func (f *RepositoryFactory) Announcement() *repositories.AnnouncementRepository {
	return f.announcementRepo
}

// DomainBlock returns the DomainBlock repository instance
func (f *RepositoryFactory) DomainBlock() *repositories.DomainBlockRepository {
	return f.domainBlockRepo
}

// Federation returns the Federation repository instance
func (f *RepositoryFactory) Federation() *repositories.FederationRepository {
	return f.federationRepo
}

// Recovery returns the Recovery repository instance
func (f *RepositoryFactory) Recovery() *repositories.RecoveryRepository {
	return f.recoveryRepo
}

// Analytics returns the Analytics repository instance
func (f *RepositoryFactory) Analytics() *repositories.TrendingRepository {
	return f.analyticsRepo
}

// Social returns the Social repository instance
func (f *RepositoryFactory) Social() *repositories.SocialRepository {
	return f.socialRepo
}

// Status returns the Status repository instance
func (f *RepositoryFactory) Status() *repositories.StatusRepository {
	return f.statusRepo
}

// Cost returns the Cost repository instance
func (f *RepositoryFactory) Cost() *repositories.CostTrackingRepository {
	return f.costRepo
}

// Search returns the Search repository instance
func (f *RepositoryFactory) Search() *repositories.SearchRepository {
	return f.searchRepo
}

// Relay returns the Relay repository instance
func (f *RepositoryFactory) Relay() *repositories.RelayRepository {
	return f.relayRepo
}

// CommunityNote returns the CommunityNote repository instance
func (f *RepositoryFactory) CommunityNote() *repositories.CommunityNoteRepository {
	return f.communityNoteRepo
}

// Emoji returns the Emoji repository instance
func (f *RepositoryFactory) Emoji() *repositories.EmojiRepository {
	return f.emojiRepo
}

// RateLimit returns the RateLimit repository instance
func (f *RepositoryFactory) RateLimit() *repositories.RateLimitRepository {
	return f.rateLimitRepo
}
// Marker returns the Marker repository instance
func (f *RepositoryFactory) Marker() *repositories.MarkerRepository {
	return f.markerRepo
}
// FeaturedTag returns the FeaturedTag repository instance
func (f *RepositoryFactory) FeaturedTag() *repositories.FeaturedTagRepository {
	return f.featuredTagRepo
}

// AI returns the AI repository instance
func (f *RepositoryFactory) AI() *repositories.AIRepository {
	return f.aiRepo
}

// Export returns the Export repository instance
func (f *RepositoryFactory) Export() *repositories.ExportRepository {
	return f.exportRepo
}

// Import returns the Import repository instance
func (f *RepositoryFactory) Import() *repositories.ImportRepository {
	return f.importRepo
}

// DLQ returns the DLQ repository instance
func (f *RepositoryFactory) DLQ() *repositories.DLQRepository {
	return f.dlqRepo
}

// MetricRecord returns the MetricRecord repository instance
func (f *RepositoryFactory) MetricRecord() *repositories.MetricRecordRepository {
	return f.metricRecordRepo
}

// CloudWatchMetrics returns the CloudWatchMetrics repository instance
func (f *RepositoryFactory) CloudWatchMetrics() *repositories.CloudWatchMetricsRepository {
	return f.cloudWatchMetricsRepo
}

// Additional repositories can be added here as needed
// For now, only the core repositories that are actually used are exposed

// Ensure RepositoryFactory implements RepositoryStorage interface
var _ core.RepositoryStorage = (*RepositoryFactory)(nil)

// GetDB returns the underlying DynamORM database connection
func (f *RepositoryFactory) GetDB() dynamormCore.DB {
	return f.db
}

// GetTableName returns the DynamoDB table name
func (f *RepositoryFactory) GetTableName() string {
	return f.tableName
}

// GetLogger returns the logger instance
func (f *RepositoryFactory) GetLogger() *zap.Logger {
	return f.logger
}
