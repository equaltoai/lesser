// Package factory provides repository factory implementation for centralized storage dependency management.
package factory

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/converters"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

var loadDefaultAWSConfig = awsconfig.LoadDefaultConfig

// RepositoryFactory manages all repository instances and their dependencies
// and implements the RepositoryStorage interface
type RepositoryFactory struct {
	db        dynamormCore.DB
	tableName string
	logger    *zap.Logger
	cfg       *config.Config

	// Storage adapter for unified interface access
	storageAdapter *dynamorm.StorageAdapter

	// Repository instances (initialize once)
	accountRepo             *repositories.AccountRepository
	actorRepo               *repositories.ActorRepository
	objectRepo              *repositories.ObjectRepository
	activityRepo            *repositories.ActivityRepository
	userRepo                *repositories.UserRepository
	trustRepo               *repositories.TrustRepository
	conversationRepo        *repositories.ConversationRepository
	timelineRepo            *repositories.TimelineRepository
	notificationRepo        *repositories.NotificationRepository
	likeRepo                *repositories.LikeRepository
	moderationRepo          *repositories.ModerationRepository
	relationshipRepo        *repositories.RelationshipRepository
	listRepo                *repositories.ListRepository
	mediaRepo               *repositories.MediaRepository
	mediaMetadataRepo       *repositories.MediaMetadataRepository
	pollRepo                *repositories.PollRepository
	pushSubscriptionRepo    *repositories.PushSubscriptionRepository
	instanceRepo            *repositories.InstanceRepository
	hashtagRepo             *repositories.HashtagRepository
	scheduledStatusRepo     *repositories.ScheduledStatusRepository
	announcementRepo        *repositories.AnnouncementRepository
	domainBlockRepo         *repositories.DomainBlockRepository
	federationRepo          *repositories.FederationRepository
	recoveryRepo            *repositories.RecoveryRepository
	analyticsRepo           *repositories.TrendingRepository
	socialRepo              *repositories.SocialRepository
	statusRepo              *repositories.StatusRepository
	costRepo                *repositories.TrackingRepository
	webSocketCostRepo       *repositories.WebSocketCostRepository
	searchRepo              *repositories.SearchRepository
	relayRepo               *repositories.RelayRepository
	communityNoteRepo       *repositories.CommunityNoteRepository
	emojiRepo               *repositories.EmojiRepository
	rateLimitRepo           *repositories.RateLimitRepository
	markerRepo              *repositories.MarkerRepository
	featuredTagRepo         *repositories.FeaturedTagRepository
	aiRepo                  *repositories.AIRepository
	exportRepo              *repositories.ExportRepository
	importRepo              *repositories.ImportRepository
	dlqRepo                 *repositories.DLQRepository
	metricRecordRepo        *repositories.MetricRecordRepository
	cloudWatchMetricsRepo   *repositories.CloudWatchMetricsRepository
	streamingCloudWatchRepo *repositories.StreamingCloudWatchRepository
	bookmarkRepo            *repositories.BookmarkRepository
	publicKeyCacheRepo      *repositories.PublicKeyCacheRepository
	auditRepo               *repositories.AuditRepository
	oauthRepo               *repositories.OAuthRepository
	dnsCacheRepo            *repositories.DNSCacheRepository
	filterRepo              *repositories.FilterRepository
	threadRepo              *repositories.ThreadRepository
	severanceRepo           *repositories.SeveranceRepository
	moderationMLRepo        *repositories.ModerationMLRepository
	quoteRepo               *repositories.QuoteRepository
	mediaAnalyticsRepo      *repositories.MediaAnalyticsRepository
	mediaPopularityRepo     *repositories.MediaPopularityRepository
	mediaSessionRepo        *repositories.MediaSessionRepository
	streamingConnectionRepo *repositories.StreamingConnectionRepository

	// CMS Repositories
	articleRepo           *repositories.ArticleRepository
	draftRepo             *repositories.DraftRepository
	revisionRepo          *repositories.RevisionRepository
	seriesRepo            *repositories.SeriesRepository
	categoryRepo          *repositories.CategoryRepository
	publicationRepo       *repositories.PublicationRepository
	publicationMemberRepo *repositories.PublicationMemberRepository
}

// NewRepositoryFactory creates a new repository factory with all repositories initialized
func NewRepositoryFactory(db dynamormCore.DB, tableName string, logger *zap.Logger) (*RepositoryFactory, error) {
	cfg := config.Get()

	if err := registerStorageConverters(db); err != nil {
		return nil, err
	}

	factory := &RepositoryFactory{
		db:        db,
		tableName: tableName,
		logger:    logger,
		cfg:       cfg,
	}

	// Initialize all repositories
	factory.initializeRepositories()

	// Set up dependencies after all repositories are created
	factory.setupDependencies()

	// Initialize storage adapter with factory as RepositoryStorage
	factory.storageAdapter = dynamorm.NewStorageAdapter(factory)

	return factory, nil
}

// initializeRepositories creates repository instances for core functionality
// This mirrors exactly what's initialized in cmd/api/main.go
func (f *RepositoryFactory) initializeRepositories() {
	// Core repositories from main.go (only these are actually used)
	f.accountRepo = repositories.NewAccountRepository(f.db, f.tableName, f.cfg.Domain, f.logger)
	f.bookmarkRepo = repositories.NewBookmarkRepository(f.db, f.tableName, f.logger)
	f.actorRepo = repositories.NewActorRepository(f.db, f.tableName, f.logger)
	f.objectRepo = repositories.NewObjectRepository(f.db, f.tableName, f.cfg.Domain, f.logger)
	f.activityRepo = repositories.NewActivityRepository(f.db, f.tableName, f.logger, nil)
	f.userRepo = repositories.NewUserRepository(f.db, f.tableName, f.logger)
	f.timelineRepo = repositories.NewTimelineRepository(f.db, f.tableName, f.logger, nil)
	f.notificationRepo = repositories.NewNotificationRepository(f.db, f.tableName, f.logger, nil)
	f.likeRepo = repositories.NewLikeRepository(f.db, f.tableName, f.logger)
	f.moderationRepo = repositories.NewModerationRepository(f.db, f.tableName, f.logger)
	f.filterRepo = repositories.NewFilterRepository(f.db, f.tableName, f.logger, nil)
	f.threadRepo = repositories.NewThreadRepository(f.db, f.logger)
	f.severanceRepo = repositories.NewSeveranceRepository(f.db, f.tableName, f.logger)
	f.quoteRepo = repositories.NewQuoteRepository(f.db, f.tableName, f.logger, nil)
	f.listRepo = repositories.NewListRepository(f.db, f.tableName, f.logger, nil)
	f.conversationRepo = repositories.NewConversationRepository(f.db, f.tableName, f.logger, nil)
	f.mediaRepo = repositories.NewMediaRepository(f.db, f.tableName, f.logger, nil)
	f.pollRepo = repositories.NewPollRepository(f.db, f.tableName, f.logger, nil)
	var (
		vapidSecretsClient  repositories.SecretsManagerClient
		vapidSecretARN      string
		defaultVAPIDSubject string
	)

	if f.cfg != nil {
		vapidSecretARN = f.cfg.VAPIDSecretARN
		defaultVAPIDSubject = f.cfg.VAPIDSubject
		if defaultVAPIDSubject == "" && f.cfg.Domain != "" {
			defaultVAPIDSubject = fmt.Sprintf("mailto:push@%s", f.cfg.Domain)
		}
	}

	if vapidSecretARN != "" {
		if awsCfg, err := loadDefaultAWSConfig(context.Background()); err != nil {
			if f.logger != nil {
				f.logger.Warn("repository factory: failed to initialize secrets manager client for VAPID keys", zap.Error(err))
			}
		} else {
			vapidSecretsClient = secretsmanager.NewFromConfig(awsCfg)
		}
	}

	f.pushSubscriptionRepo = repositories.NewPushSubscriptionRepository(
		f.db,
		f.tableName,
		f.logger,
		nil,
		vapidSecretsClient,
		vapidSecretARN,
		defaultVAPIDSubject,
	)
	f.hashtagRepo = repositories.NewHashtagRepository(f.db, f.tableName, f.logger, f.cfg.Domain)
	f.scheduledStatusRepo = repositories.NewScheduledStatusRepository(f.db, f.tableName, f.logger, nil)
	f.announcementRepo = repositories.NewAnnouncementRepository(f.db, f.tableName, f.logger)
	f.domainBlockRepo = repositories.NewDomainBlockRepository(f.db, f.tableName, f.logger)
	f.relationshipRepo = repositories.NewRelationshipRepository(f.db, f.tableName, f.logger)
	f.instanceRepo = repositories.NewInstanceRepository(f.db, f.tableName, f.logger)
	f.federationRepo = repositories.NewFederationRepository(f.db, f.tableName, f.logger, nil, f.cfg)
	f.recoveryRepo = repositories.NewRecoveryRepository(f.db, f.tableName, f.logger, nil)
	f.analyticsRepo = repositories.NewTrendingRepository(f.db, f.logger, nil)
	f.socialRepo = repositories.NewSocialRepository(f.db, f.tableName, f.logger, nil)
	f.statusRepo = repositories.NewStatusRepository(f.db, f.tableName, f.logger, nil)
	f.costRepo = repositories.NewTrackingRepository(f.db, f.tableName, f.logger, nil)
	f.webSocketCostRepo = repositories.NewWebSocketCostRepository(f.db, f.tableName, f.logger, nil)
	f.trustRepo = repositories.NewTrustRepository(f.db, f.tableName, f.logger, nil)
	f.searchRepo = repositories.NewSearchRepository(f.db, f.tableName, f.logger, nil)
	f.relayRepo = repositories.NewRelayRepository(f.db, f.tableName, f.logger, nil)
	f.communityNoteRepo = repositories.NewCommunityNoteRepository(f.db, f.tableName, f.logger, nil)
	f.emojiRepo = repositories.NewEmojiRepository(f.db, f.tableName, f.logger, nil)
	f.rateLimitRepo = repositories.NewRateLimitRepository(f.db, f.tableName, f.logger, nil)
	f.markerRepo = repositories.NewMarkerRepository(f.db, f.tableName, f.logger, nil)
	f.featuredTagRepo = repositories.NewFeaturedTagRepository(f.db, f.tableName, f.logger, nil)
	f.aiRepo = repositories.NewAIRepository(f.db, f.tableName, f.logger, nil)
	f.exportRepo = repositories.NewExportRepository(f.db, f.tableName, f.logger)
	f.importRepo = repositories.NewImportRepository(f.db, f.tableName, f.logger)
	f.auditRepo = repositories.NewAuditRepository(f.db, f.tableName, f.logger, nil)
	f.dlqRepo = repositories.NewDLQRepositorySimple(f.db, f.tableName, f.logger)
	f.metricRecordRepo = repositories.NewMetricRecordRepository(f.db, f.tableName, f.logger, nil)
	f.cloudWatchMetricsRepo = repositories.NewCloudWatchMetricsRepository("Lesser/Production", "prod", f.logger)
	f.streamingCloudWatchRepo = repositories.NewStreamingCloudWatchRepository(f.db, f.tableName, f.logger, nil)
	f.publicKeyCacheRepo = repositories.NewPublicKeyCacheRepository(f.db, f.tableName, f.logger, nil)
	f.mediaMetadataRepo = repositories.NewMediaMetadataRepository(f.db, f.tableName, f.logger, nil)
	f.oauthRepo = repositories.NewOAuthRepository(f.db, f.logger)
	f.dnsCacheRepo = repositories.NewDNSCacheRepository(f.db, f.tableName, f.logger, nil)
	f.moderationMLRepo = repositories.NewModerationMLRepository(f.db, f.tableName, f.logger)
	f.mediaAnalyticsRepo = repositories.NewMediaAnalyticsRepository(f.db, f.tableName, f.logger, nil)
	f.mediaPopularityRepo = repositories.NewMediaPopularityRepository(f.db, f.tableName, f.logger, nil)
	f.mediaSessionRepo = repositories.NewMediaSessionRepository(f.db, f.logger, nil)
	f.streamingConnectionRepo = repositories.NewStreamingConnectionRepository(f.db, f.tableName, f.db, f.tableName, f.logger, nil)

	// Initialize CMS repositories
	f.articleRepo = repositories.NewArticleRepository(f.db, f.tableName, f.logger, nil)
	f.draftRepo = repositories.NewDraftRepository(f.db, f.tableName, f.logger, nil)
	f.revisionRepo = repositories.NewRevisionRepository(f.db, f.tableName, f.logger, nil)
	f.seriesRepo = repositories.NewSeriesRepository(f.db, f.tableName, f.logger, nil)
	f.categoryRepo = repositories.NewCategoryRepository(f.db, f.tableName, f.logger, nil)
	f.publicationRepo = repositories.NewPublicationRepository(f.db, f.tableName, f.logger, nil)
	f.publicationMemberRepo = repositories.NewPublicationMemberRepository(f.db, f.tableName, f.logger, nil)

	// All other repositories are nil until needed/implemented
	// This allows the factory to be created without breaking the application
}

// setupDependencies configures repository dependencies after all repositories are created
func (f *RepositoryFactory) setupDependencies() {
	// Set up scheduled status repository dependency on media repository
	if f.scheduledStatusRepo != nil && f.mediaRepo != nil {
		f.scheduledStatusRepo.SetMediaRepository(f.mediaRepo)
	}

	// Wire bookmark repository dependencies
	if f.bookmarkRepo != nil {
		if f.accountRepo != nil {
			f.accountRepo.SetBookmarkRepository(f.bookmarkRepo)
		}
		if f.userRepo != nil {
			f.userRepo.SetBookmarkRepository(f.bookmarkRepo)
		}
		if f.statusRepo != nil {
			f.statusRepo.SetBookmarkRepository(f.bookmarkRepo)
		}
	}

	// Set up search repository dependencies for privacy enforcement
	if f.searchRepo != nil && f.relationshipRepo != nil {
		// Create a deps adapter that implements SearchRepositoryDeps
		deps := &searchRepositoryDeps{
			relationshipRepo: f.relationshipRepo,
		}
		f.searchRepo.SetDependencies(deps)
	}

	// Set up account repository dependency on status repository for GetBookmarkedStatuses
	if f.accountRepo != nil && f.statusRepo != nil {
		f.accountRepo.SetStatusRepository(f.statusRepo)
	}

	// Set up status repository dependency on relationship repository for home timeline queries
	if f.statusRepo != nil && f.relationshipRepo != nil {
		f.statusRepo.SetRelationshipRepository(f.relationshipRepo)
	}

	// Additional repository dependencies can be configured here as needed.
}

func registerStorageConverters(db dynamormCore.DB) error {
	if db == nil {
		return fmt.Errorf("dynamorm DB is nil")
	}

	if err := converters.RegisterContextConverters(db); err != nil {
		return fmt.Errorf("register context converter: %w", err)
	}

	return nil
}

// searchRepositoryDeps implements SearchRepositoryDeps interface
type searchRepositoryDeps struct {
	relationshipRepo *repositories.RelationshipRepository
}

func (d *searchRepositoryDeps) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return d.relationshipRepo.GetFollowing(ctx, username, limit, cursor)
}

func (d *searchRepositoryDeps) IsBlocked(ctx context.Context, blockerActor, blockedActor string) (bool, error) {
	return d.relationshipRepo.IsBlocked(ctx, blockerActor, blockedActor)
}

func (d *searchRepositoryDeps) IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error) {
	return d.relationshipRepo.IsBlockedBidirectional(ctx, actor1, actor2)
}

func (d *searchRepositoryDeps) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return d.relationshipRepo.GetFollowers(ctx, username, limit, cursor)
}

// Getter methods for each repository type

// Account returns the Account repository instance
func (f *RepositoryFactory) Account() *repositories.AccountRepository {
	return f.accountRepo
}

// Bookmark returns the Bookmark repository instance
func (f *RepositoryFactory) Bookmark() *repositories.BookmarkRepository {
	return f.bookmarkRepo
}

// Actor returns the Actor repository instance
func (f *RepositoryFactory) Actor() interfaces.ActorRepository {
	return f.actorRepo
}

// Object returns the Object repository instance
func (f *RepositoryFactory) Object() interfaces.ObjectRepository {
	return f.objectRepo
}

// Activity returns the Activity repository instance (interface type for mockability).
func (f *RepositoryFactory) Activity() interfaces.ActivityRepository {
	return f.activityRepo
}

// User returns the User repository instance
func (f *RepositoryFactory) User() interfaces.UserRepository {
	return f.userRepo
}

// Trust returns the Trust repository instance (interface type for mockability).
func (f *RepositoryFactory) Trust() interfaces.TrustRepository {
	return f.trustRepo
}

// Conversation returns the Conversation repository instance
func (f *RepositoryFactory) Conversation() *repositories.ConversationRepository {
	return f.conversationRepo
}

// Timeline returns the Timeline repository instance (interface type for mockability).
func (f *RepositoryFactory) Timeline() interfaces.TimelineRepository {
	return f.timelineRepo
}

// Notification returns the Notification repository instance (interface type for mockability).
func (f *RepositoryFactory) Notification() interfaces.NotificationRepository {
	return f.notificationRepo
}

// Like returns the Like repository instance
func (f *RepositoryFactory) Like() *repositories.LikeRepository {
	return f.likeRepo
}

// Moderation returns the Moderation repository instance
func (f *RepositoryFactory) Moderation() interfaces.ModerationRepository {
	return f.moderationRepo
}

// Relationship returns the Relationship repository instance
func (f *RepositoryFactory) Relationship() interfaces.ConcreteRelationshipRepository {
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

// MediaMetadata returns the MediaMetadata repository instance
func (f *RepositoryFactory) MediaMetadata() *repositories.MediaMetadataRepository {
	return f.mediaMetadataRepo
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

// Audit returns the Audit repository instance
func (f *RepositoryFactory) Audit() *repositories.AuditRepository {
	return f.auditRepo
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
func (f *RepositoryFactory) Status() interfaces.StatusRepository {
	return f.statusRepo
}

// Cost returns the Cost repository instance
func (f *RepositoryFactory) Cost() *repositories.TrackingRepository {
	return f.costRepo
}

// WebSocketCost returns the WebSocketCost repository instance
func (f *RepositoryFactory) WebSocketCost() *repositories.WebSocketCostRepository {
	return f.webSocketCostRepo
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

// StreamingCloudWatch returns the StreamingCloudWatch repository instance
func (f *RepositoryFactory) StreamingCloudWatch() *repositories.StreamingCloudWatchRepository {
	return f.streamingCloudWatchRepo
}

// PublicKeyCache returns the PublicKeyCache repository instance
func (f *RepositoryFactory) PublicKeyCache() *repositories.PublicKeyCacheRepository {
	return f.publicKeyCacheRepo
}

// OAuth returns the OAuth repository instance
func (f *RepositoryFactory) OAuth() *repositories.OAuthRepository {
	return f.oauthRepo
}

// DNSCache returns the DNSCache repository instance
func (f *RepositoryFactory) DNSCache() *repositories.DNSCacheRepository {
	return f.dnsCacheRepo
}

// Filter returns the Filter repository instance
func (f *RepositoryFactory) Filter() *repositories.FilterRepository {
	return f.filterRepo
}

// Thread returns the Thread repository instance
func (f *RepositoryFactory) Thread() *repositories.ThreadRepository {
	return f.threadRepo
}

// Severance returns the Severance repository instance
func (f *RepositoryFactory) Severance() *repositories.SeveranceRepository {
	return f.severanceRepo
}

// ModerationML returns the ModerationML repository instance
func (f *RepositoryFactory) ModerationML() *repositories.ModerationMLRepository {
	return f.moderationMLRepo
}

// Quote returns the Quote repository instance
func (f *RepositoryFactory) Quote() *repositories.QuoteRepository {
	return f.quoteRepo
}

// MediaAnalytics returns the MediaAnalytics repository instance
func (f *RepositoryFactory) MediaAnalytics() interfaces.MediaAnalyticsRepository {
	return f.mediaAnalyticsRepo
}

// MediaPopularity returns the MediaPopularity repository instance
func (f *RepositoryFactory) MediaPopularity() interfaces.MediaPopularityRepository {
	return f.mediaPopularityRepo
}

// MediaSession returns the MediaSession repository instance
func (f *RepositoryFactory) MediaSession() interfaces.MediaSessionRepository {
	return f.mediaSessionRepo
}

// StreamingConnection returns the StreamingConnection repository instance
func (f *RepositoryFactory) StreamingConnection() interfaces.StreamingConnectionRepository {
	return f.streamingConnectionRepo
}

// CMS Repository Getters (interface types for mockability)

// Article returns the Article repository instance
func (f *RepositoryFactory) Article() interfaces.ArticleRepository {
	return f.articleRepo
}

// Draft returns the Draft repository instance
func (f *RepositoryFactory) Draft() interfaces.DraftRepository {
	return f.draftRepo
}

// Revision returns the Revision repository instance
func (f *RepositoryFactory) Revision() interfaces.RevisionRepository {
	return f.revisionRepo
}

// Series returns the Series repository instance
func (f *RepositoryFactory) Series() interfaces.SeriesRepository {
	return f.seriesRepo
}

// Category returns the Category repository instance
func (f *RepositoryFactory) Category() interfaces.CategoryRepository {
	return f.categoryRepo
}

// Publication returns the Publication repository instance
func (f *RepositoryFactory) Publication() interfaces.PublicationRepository {
	return f.publicationRepo
}

// PublicationMember returns the PublicationMember repository instance
func (f *RepositoryFactory) PublicationMember() interfaces.PublicationMemberRepository {
	return f.publicationMemberRepo
}

// Additional repositories can be added here as needed
// For now, only the core repositories that are actually used are exposed

// GetStorageAdapter returns the unified storage interface
// This provides access to both repository methods AND legacy storage operations
func (f *RepositoryFactory) GetStorageAdapter() interfaces.Storage {
	if f.storageAdapter == nil {
		f.storageAdapter = dynamorm.NewStorageAdapter(f)
	}
	return f.storageAdapter
}

// AsStorage returns this factory as a Storage interface
// This enables the factory to be used anywhere Storage is expected
func (f *RepositoryFactory) AsStorage() interfaces.Storage {
	return f.GetStorageAdapter()
}

// ResetStorageAdapter recreates the storage adapter
// Useful for testing and when repository dependencies change
func (f *RepositoryFactory) ResetStorageAdapter() {
	f.storageAdapter = dynamorm.NewStorageAdapter(f)
}

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
