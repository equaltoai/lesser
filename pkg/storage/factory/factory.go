package factory

import (
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

	// Repository instances (initialize once)
	accountRepo               *repositories.AccountRepository
	actorRepo                 *repositories.ActorRepository
	objectRepo                *repositories.ObjectRepository
	activityRepo              *repositories.ActivityRepository
	userRepo                  *repositories.UserRepository
	trustRepo                 *repositories.TrustRepository
	conversationRepo          *repositories.ConversationRepository
	timelineRepo              *repositories.TimelineRepository
	notificationRepo          *repositories.NotificationRepository
	likeRepo                  *repositories.LikeRepository
	moderationRepo            *repositories.ModerationRepository
	relationshipRepo          *repositories.RelationshipRepository
	listRepo                  *repositories.ListRepository
	mediaRepo                 *repositories.MediaRepository
	pollRepo                  *repositories.PollRepository
	pushSubscriptionRepo      *repositories.PushSubscriptionRepository
	instanceRepo              *repositories.InstanceRepository
	hashtagRepo               *repositories.HashtagRepository
	scheduledStatusRepo       *repositories.ScheduledStatusRepository
	announcementRepo          *repositories.AnnouncementRepository
	domainBlockRepo           *repositories.DomainBlockRepository
	federationRepo            *repositories.FederationRepository
	recoveryRepo              *repositories.RecoveryRepository
	analyticsRepo             *repositories.TrendingRepository
	socialRepo                *repositories.SocialRepository
	statusRepo                *repositories.StatusRepository
	costRepo                  *repositories.CostTrackingRepository
	searchRepo                *repositories.SearchRepository
	relayRepo                 *repositories.RelayRepository
	communityNoteRepo         *repositories.CommunityNoteRepository
	emojiRepo                 *repositories.EmojiRepository
	rateLimitRepo             *repositories.RateLimitRepository
	markerRepo                *repositories.MarkerRepository
	featuredTagRepo           *repositories.FeaturedTagRepository
	aiRepo                    *repositories.AIRepository
}

// NewRepositoryFactory creates a new repository factory with all repositories initialized
func NewRepositoryFactory(db dynamormCore.DB, tableName string, logger *zap.Logger) (*RepositoryFactory, error) {
	cfg := config.Get()
	
	factory := &RepositoryFactory{
		db:        db,
		tableName: tableName,
		logger:    logger,
		cfg:       cfg,
	}

	// Initialize all repositories
	if err := factory.initializeRepositories(); err != nil {
		return nil, err
	}

	// Set up dependencies after all repositories are created
	factory.setupDependencies()

	return factory, nil
}

// initializeRepositories creates repository instances for core functionality
// This mirrors exactly what's initialized in cmd/api/main.go
func (f *RepositoryFactory) initializeRepositories() error {
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

	// All other repositories are nil until needed/implemented
	// This allows the factory to be created without breaking the application

	return nil
}

// setupDependencies configures repository dependencies after all repositories are created
func (f *RepositoryFactory) setupDependencies() {
	// TODO: Set up repository dependencies when all repositories are properly implemented
	// For now, skip dependency setup to allow the factory to function without all repositories
}

// Getter methods for each repository type
func (f *RepositoryFactory) Account() *repositories.AccountRepository {
	return f.accountRepo
}

func (f *RepositoryFactory) Actor() *repositories.ActorRepository {
	return f.actorRepo
}

func (f *RepositoryFactory) Object() *repositories.ObjectRepository {
	return f.objectRepo
}

func (f *RepositoryFactory) Activity() *repositories.ActivityRepository {
	return f.activityRepo
}

func (f *RepositoryFactory) User() *repositories.UserRepository {
	return f.userRepo
}

func (f *RepositoryFactory) Trust() *repositories.TrustRepository {
	return f.trustRepo
}

func (f *RepositoryFactory) Conversation() *repositories.ConversationRepository {
	return f.conversationRepo
}

func (f *RepositoryFactory) Timeline() *repositories.TimelineRepository {
	return f.timelineRepo
}

func (f *RepositoryFactory) Notification() *repositories.NotificationRepository {
	return f.notificationRepo
}

func (f *RepositoryFactory) Like() *repositories.LikeRepository {
	return f.likeRepo
}


func (f *RepositoryFactory) Moderation() *repositories.ModerationRepository {
	return f.moderationRepo
}

func (f *RepositoryFactory) Relationship() *repositories.RelationshipRepository {
	return f.relationshipRepo
}

func (f *RepositoryFactory) List() *repositories.ListRepository {
	return f.listRepo
}

func (f *RepositoryFactory) Media() *repositories.MediaRepository {
	return f.mediaRepo
}

func (f *RepositoryFactory) Poll() *repositories.PollRepository {
	return f.pollRepo
}

func (f *RepositoryFactory) PushSubscription() *repositories.PushSubscriptionRepository {
	return f.pushSubscriptionRepo
}

func (f *RepositoryFactory) Instance() *repositories.InstanceRepository {
	return f.instanceRepo
}

func (f *RepositoryFactory) Hashtag() *repositories.HashtagRepository {
	return f.hashtagRepo
}

func (f *RepositoryFactory) ScheduledStatus() *repositories.ScheduledStatusRepository {
	return f.scheduledStatusRepo
}

func (f *RepositoryFactory) Announcement() *repositories.AnnouncementRepository {
	return f.announcementRepo
}

func (f *RepositoryFactory) DomainBlock() *repositories.DomainBlockRepository {
	return f.domainBlockRepo
}

func (f *RepositoryFactory) Federation() *repositories.FederationRepository {
	return f.federationRepo
}

func (f *RepositoryFactory) Recovery() *repositories.RecoveryRepository {
	return f.recoveryRepo
}

func (f *RepositoryFactory) Analytics() *repositories.TrendingRepository {
	return f.analyticsRepo
}

func (f *RepositoryFactory) Social() *repositories.SocialRepository {
	return f.socialRepo
}

func (f *RepositoryFactory) Status() *repositories.StatusRepository {
	return f.statusRepo
}

func (f *RepositoryFactory) Cost() *repositories.CostTrackingRepository {
	return f.costRepo
}

func (f *RepositoryFactory) Search() *repositories.SearchRepository {
	return f.searchRepo
}

func (f *RepositoryFactory) Relay() *repositories.RelayRepository {
	return f.relayRepo
}

func (f *RepositoryFactory) CommunityNote() *repositories.CommunityNoteRepository {
	return f.communityNoteRepo
}

func (f *RepositoryFactory) Emoji() *repositories.EmojiRepository {
	return f.emojiRepo
}

func (f *RepositoryFactory) RateLimit() *repositories.RateLimitRepository {
	return f.rateLimitRepo
}
func (f *RepositoryFactory) Marker() *repositories.MarkerRepository {
	return f.markerRepo
}
func (f *RepositoryFactory) FeaturedTag() *repositories.FeaturedTagRepository {
	return f.featuredTagRepo
}

func (f *RepositoryFactory) AI() *repositories.AIRepository {
	return f.aiRepo
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