// Package testing provides test utilities and mock implementations for the Lesser application.
package testing

import (
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// MockRepositoryStorage provides configurable repository implementations for testing.
// It implements the core.RepositoryStorage interface and defaults to in-memory
// implementations for all repositories, allowing custom mock injection via
// functional options.
//
// Usage:
//
//	// Default in-memory storage
//	storage := testing.NewMockRepositoryStorage()
//
//	// With custom mock for specific repository
//	mockUserRepo := mocks.NewMockUserRepositoryInterface()
//	mockUserRepo.On("GetUser", mock.Anything, "testuser").Return(user, nil)
//	storage := testing.NewMockRepositoryStorage(
//	    testing.WithUserRepository(mockUserRepo),
//	)
type MockRepositoryStorage struct {
	// Repository implementations (interface types for mockability)
	userRepo     interfaces.UserRepository
	actorRepo    interfaces.ActorRepository
	statusRepo   interfaces.StatusRepository
	timelineRepo interfaces.TimelineRepository

	// Concrete repository implementations (for repositories not yet converted to interfaces)
	accountRepo  *repositories.AccountRepository
	bookmarkRepo *repositories.BookmarkRepository
	objectRepo           *repositories.ObjectRepository
	activityRepo         *repositories.ActivityRepository
	notificationRepo     interfaces.NotificationRepository
	likeRepo             *repositories.LikeRepository
	moderationRepo       *repositories.ModerationRepository
	listRepo             *repositories.ListRepository
	mediaRepo            *repositories.MediaRepository
	mediaMetadataRepo    *repositories.MediaMetadataRepository
	pollRepo             *repositories.PollRepository
	pushSubscriptionRepo *repositories.PushSubscriptionRepository
	hashtagRepo          *repositories.HashtagRepository
	scheduledStatusRepo  *repositories.ScheduledStatusRepository
	announcementRepo     *repositories.AnnouncementRepository
	domainBlockRepo      *repositories.DomainBlockRepository
	relationshipRepo     *repositories.RelationshipRepository
	instanceRepo         *repositories.InstanceRepository
	federationRepo       *repositories.FederationRepository
	recoveryRepo         *repositories.RecoveryRepository
	analyticsRepo        *repositories.TrendingRepository
	socialRepo           *repositories.SocialRepository
	costRepo             *repositories.TrackingRepository
	webSocketCostRepo    *repositories.WebSocketCostRepository
	trustRepo            *repositories.TrustRepository
	searchRepo           *repositories.SearchRepository
	relayRepo            *repositories.RelayRepository
	communityNoteRepo    *repositories.CommunityNoteRepository
	emojiRepo            *repositories.EmojiRepository
	rateLimitRepo        *repositories.RateLimitRepository
	conversationRepo     *repositories.ConversationRepository
	markerRepo           *repositories.MarkerRepository
	featuredTagRepo      *repositories.FeaturedTagRepository
	aiRepo               *repositories.AIRepository
	exportRepo           *repositories.ExportRepository
	importRepo           *repositories.ImportRepository
	dlqRepo              *repositories.DLQRepository
	metricRecordRepo     *repositories.MetricRecordRepository
	cloudWatchRepo       *repositories.CloudWatchMetricsRepository
	streamingCWRepo      *repositories.StreamingCloudWatchRepository
	auditRepo            *repositories.AuditRepository
	oauthRepo            *repositories.OAuthRepository
	dnsCacheRepo         *repositories.DNSCacheRepository
	filterRepo           *repositories.FilterRepository
	threadRepo           *repositories.ThreadRepository
	severanceRepo        *repositories.SeveranceRepository
	moderationMLRepo     *repositories.ModerationMLRepository
	quoteRepo            *repositories.QuoteRepository
	mediaAnalyticsRepo   *repositories.MediaAnalyticsRepository
	mediaPopularityRepo  *repositories.MediaPopularityRepository
	mediaSessionRepo     *repositories.MediaSessionRepository
	streamingConnRepo    *repositories.StreamingConnectionRepository

	// CMS repositories
	articleRepo           *repositories.ArticleRepository
	draftRepo             *repositories.DraftRepository
	revisionRepo          *repositories.RevisionRepository
	seriesRepo            *repositories.SeriesRepository
	categoryRepo          *repositories.CategoryRepository
	publicationRepo       *repositories.PublicationRepository
	publicationMemberRepo *repositories.PublicationMemberRepository

	// Utility fields
	logger    *zap.Logger
	tableName string
}

// Option configures MockRepositoryStorage
type Option func(*MockRepositoryStorage)

// WithUserRepository sets a custom user repository implementation.
// Use this to inject a mock for testing specific user repository behavior.
func WithUserRepository(repo interfaces.UserRepository) Option {
	return func(s *MockRepositoryStorage) {
		s.userRepo = repo
	}
}

// WithActorRepository sets a custom actor repository implementation.
// Use this to inject a mock for testing specific actor repository behavior.
func WithActorRepository(repo interfaces.ActorRepository) Option {
	return func(s *MockRepositoryStorage) {
		s.actorRepo = repo
	}
}

// WithStatusRepository sets a custom status repository implementation.
// Use this to inject a mock for testing specific status repository behavior.
func WithStatusRepository(repo interfaces.StatusRepository) Option {
	return func(s *MockRepositoryStorage) {
		s.statusRepo = repo
	}
}

// WithTimelineRepository sets a custom timeline repository implementation.
// Use this to inject a mock for testing specific timeline repository behavior.
func WithTimelineRepository(repo interfaces.TimelineRepository) Option {
	return func(s *MockRepositoryStorage) {
		s.timelineRepo = repo
	}
}

// WithLogger sets a custom logger for the mock storage.
func WithLogger(logger *zap.Logger) Option {
	return func(s *MockRepositoryStorage) {
		s.logger = logger
	}
}

// WithTableName sets a custom table name for the mock storage.
func WithTableName(tableName string) Option {
	return func(s *MockRepositoryStorage) {
		s.tableName = tableName
	}
}

// NewMockRepositoryStorage creates a new MockRepositoryStorage with in-memory defaults.
// All repositories default to in-memory implementations that can be overridden
// using functional options.
//
// Example:
//
//	// Create with defaults
//	storage := NewMockRepositoryStorage()
//
//	// Create with custom user repository
//	storage := NewMockRepositoryStorage(
//	    WithUserRepository(customMock),
//	)
func NewMockRepositoryStorage(opts ...Option) *MockRepositoryStorage {
	s := &MockRepositoryStorage{
		// Default to in-memory repositories
		userRepo:     inmemory.NewUserRepository(),
		actorRepo:    inmemory.NewActorRepository(),
		statusRepo:   inmemory.NewStatusRepository(),
		timelineRepo: inmemory.NewTimelineRepository(),
		logger:       zap.NewNop(),
		tableName:    "test-table",
	}

	// Apply all options
	for _, opt := range opts {
		opt(s)
	}

	return s
}

// User returns the user repository (interface type for mockability).
func (s *MockRepositoryStorage) User() interfaces.UserRepository {
	return s.userRepo
}

// Account returns the account repository.
func (s *MockRepositoryStorage) Account() *repositories.AccountRepository {
	return s.accountRepo
}

// Bookmark returns the bookmark repository.
func (s *MockRepositoryStorage) Bookmark() *repositories.BookmarkRepository {
	return s.bookmarkRepo
}

// Actor returns the actor repository (interface type for mockability).
func (s *MockRepositoryStorage) Actor() interfaces.ActorRepository {
	return s.actorRepo
}

// Object returns the object repository.
func (s *MockRepositoryStorage) Object() *repositories.ObjectRepository {
	return s.objectRepo
}

// Activity returns the activity repository.
func (s *MockRepositoryStorage) Activity() *repositories.ActivityRepository {
	return s.activityRepo
}

// Timeline returns the timeline repository (interface type for mockability).
func (s *MockRepositoryStorage) Timeline() interfaces.TimelineRepository {
	return s.timelineRepo
}

// Notification returns the notification repository (interface type for mockability).
func (s *MockRepositoryStorage) Notification() interfaces.NotificationRepository {
	return s.notificationRepo
}

// Like returns the like repository.
func (s *MockRepositoryStorage) Like() *repositories.LikeRepository {
	return s.likeRepo
}

// Moderation returns the moderation repository.
func (s *MockRepositoryStorage) Moderation() *repositories.ModerationRepository {
	return s.moderationRepo
}

// List returns the list repository.
func (s *MockRepositoryStorage) List() *repositories.ListRepository {
	return s.listRepo
}

// Media returns the media repository.
func (s *MockRepositoryStorage) Media() *repositories.MediaRepository {
	return s.mediaRepo
}

// MediaMetadata returns the media metadata repository.
func (s *MockRepositoryStorage) MediaMetadata() *repositories.MediaMetadataRepository {
	return s.mediaMetadataRepo
}

// Poll returns the poll repository.
func (s *MockRepositoryStorage) Poll() *repositories.PollRepository {
	return s.pollRepo
}

// PushSubscription returns the push subscription repository.
func (s *MockRepositoryStorage) PushSubscription() *repositories.PushSubscriptionRepository {
	return s.pushSubscriptionRepo
}

// Hashtag returns the hashtag repository.
func (s *MockRepositoryStorage) Hashtag() *repositories.HashtagRepository {
	return s.hashtagRepo
}

// ScheduledStatus returns the scheduled status repository.
func (s *MockRepositoryStorage) ScheduledStatus() *repositories.ScheduledStatusRepository {
	return s.scheduledStatusRepo
}

// Announcement returns the announcement repository.
func (s *MockRepositoryStorage) Announcement() *repositories.AnnouncementRepository {
	return s.announcementRepo
}

// DomainBlock returns the domain block repository.
func (s *MockRepositoryStorage) DomainBlock() *repositories.DomainBlockRepository {
	return s.domainBlockRepo
}

// Relationship returns the relationship repository.
func (s *MockRepositoryStorage) Relationship() *repositories.RelationshipRepository {
	return s.relationshipRepo
}

// Instance returns the instance repository.
func (s *MockRepositoryStorage) Instance() *repositories.InstanceRepository {
	return s.instanceRepo
}

// Federation returns the federation repository.
func (s *MockRepositoryStorage) Federation() *repositories.FederationRepository {
	return s.federationRepo
}

// Recovery returns the recovery repository.
func (s *MockRepositoryStorage) Recovery() *repositories.RecoveryRepository {
	return s.recoveryRepo
}

// Analytics returns the analytics/trending repository.
func (s *MockRepositoryStorage) Analytics() *repositories.TrendingRepository {
	return s.analyticsRepo
}

// Social returns the social repository.
func (s *MockRepositoryStorage) Social() *repositories.SocialRepository {
	return s.socialRepo
}

// Status returns the status repository (interface type for mockability).
func (s *MockRepositoryStorage) Status() interfaces.StatusRepository {
	return s.statusRepo
}

// Cost returns the cost tracking repository.
func (s *MockRepositoryStorage) Cost() *repositories.TrackingRepository {
	return s.costRepo
}

// WebSocketCost returns the WebSocket cost repository.
func (s *MockRepositoryStorage) WebSocketCost() *repositories.WebSocketCostRepository {
	return s.webSocketCostRepo
}

// Trust returns the trust repository.
func (s *MockRepositoryStorage) Trust() *repositories.TrustRepository {
	return s.trustRepo
}

// Search returns the search repository.
func (s *MockRepositoryStorage) Search() *repositories.SearchRepository {
	return s.searchRepo
}

// Relay returns the relay repository.
func (s *MockRepositoryStorage) Relay() *repositories.RelayRepository {
	return s.relayRepo
}

// CommunityNote returns the community note repository.
func (s *MockRepositoryStorage) CommunityNote() *repositories.CommunityNoteRepository {
	return s.communityNoteRepo
}

// Emoji returns the emoji repository.
func (s *MockRepositoryStorage) Emoji() *repositories.EmojiRepository {
	return s.emojiRepo
}

// RateLimit returns the rate limit repository.
func (s *MockRepositoryStorage) RateLimit() *repositories.RateLimitRepository {
	return s.rateLimitRepo
}

// Conversation returns the conversation repository.
func (s *MockRepositoryStorage) Conversation() *repositories.ConversationRepository {
	return s.conversationRepo
}

// Marker returns the marker repository.
func (s *MockRepositoryStorage) Marker() *repositories.MarkerRepository {
	return s.markerRepo
}

// FeaturedTag returns the featured tag repository.
func (s *MockRepositoryStorage) FeaturedTag() *repositories.FeaturedTagRepository {
	return s.featuredTagRepo
}

// AI returns the AI repository.
func (s *MockRepositoryStorage) AI() *repositories.AIRepository {
	return s.aiRepo
}

// Export returns the export repository.
func (s *MockRepositoryStorage) Export() *repositories.ExportRepository {
	return s.exportRepo
}

// Import returns the import repository.
func (s *MockRepositoryStorage) Import() *repositories.ImportRepository {
	return s.importRepo
}

// DLQ returns the DLQ repository.
func (s *MockRepositoryStorage) DLQ() *repositories.DLQRepository {
	return s.dlqRepo
}

// MetricRecord returns the metric record repository.
func (s *MockRepositoryStorage) MetricRecord() *repositories.MetricRecordRepository {
	return s.metricRecordRepo
}

// CloudWatchMetrics returns the CloudWatch metrics repository.
func (s *MockRepositoryStorage) CloudWatchMetrics() *repositories.CloudWatchMetricsRepository {
	return s.cloudWatchRepo
}

// StreamingCloudWatch returns the streaming CloudWatch repository.
func (s *MockRepositoryStorage) StreamingCloudWatch() *repositories.StreamingCloudWatchRepository {
	return s.streamingCWRepo
}

// Audit returns the audit repository.
func (s *MockRepositoryStorage) Audit() *repositories.AuditRepository {
	return s.auditRepo
}

// OAuth returns the OAuth repository.
func (s *MockRepositoryStorage) OAuth() *repositories.OAuthRepository {
	return s.oauthRepo
}

// DNSCache returns the DNS cache repository.
func (s *MockRepositoryStorage) DNSCache() *repositories.DNSCacheRepository {
	return s.dnsCacheRepo
}

// Filter returns the filter repository.
func (s *MockRepositoryStorage) Filter() *repositories.FilterRepository {
	return s.filterRepo
}

// Thread returns the thread repository.
func (s *MockRepositoryStorage) Thread() *repositories.ThreadRepository {
	return s.threadRepo
}

// Severance returns the severance repository.
func (s *MockRepositoryStorage) Severance() *repositories.SeveranceRepository {
	return s.severanceRepo
}

// ModerationML returns the moderation ML repository.
func (s *MockRepositoryStorage) ModerationML() *repositories.ModerationMLRepository {
	return s.moderationMLRepo
}

// Quote returns the quote repository.
func (s *MockRepositoryStorage) Quote() *repositories.QuoteRepository {
	return s.quoteRepo
}

// MediaAnalytics returns the media analytics repository.
func (s *MockRepositoryStorage) MediaAnalytics() *repositories.MediaAnalyticsRepository {
	return s.mediaAnalyticsRepo
}

// MediaPopularity returns the media popularity repository.
func (s *MockRepositoryStorage) MediaPopularity() *repositories.MediaPopularityRepository {
	return s.mediaPopularityRepo
}

// MediaSession returns the media session repository.
func (s *MockRepositoryStorage) MediaSession() *repositories.MediaSessionRepository {
	return s.mediaSessionRepo
}

// StreamingConnection returns the streaming connection repository.
func (s *MockRepositoryStorage) StreamingConnection() *repositories.StreamingConnectionRepository {
	return s.streamingConnRepo
}

// CMS Repository accessors

// Article returns the article repository.
func (s *MockRepositoryStorage) Article() *repositories.ArticleRepository {
	return s.articleRepo
}

// Draft returns the draft repository.
func (s *MockRepositoryStorage) Draft() *repositories.DraftRepository {
	return s.draftRepo
}

// Revision returns the revision repository.
func (s *MockRepositoryStorage) Revision() *repositories.RevisionRepository {
	return s.revisionRepo
}

// Series returns the series repository.
func (s *MockRepositoryStorage) Series() *repositories.SeriesRepository {
	return s.seriesRepo
}

// Category returns the category repository.
func (s *MockRepositoryStorage) Category() *repositories.CategoryRepository {
	return s.categoryRepo
}

// Publication returns the publication repository.
func (s *MockRepositoryStorage) Publication() *repositories.PublicationRepository {
	return s.publicationRepo
}

// PublicationMember returns the publication member repository.
func (s *MockRepositoryStorage) PublicationMember() *repositories.PublicationMemberRepository {
	return s.publicationMemberRepo
}

// Utility methods

// GetDB returns nil for mock storage (no real database connection).
func (s *MockRepositoryStorage) GetDB() dynamormCore.DB {
	return nil
}

// GetTableName returns the configured table name.
func (s *MockRepositoryStorage) GetTableName() string {
	return s.tableName
}

// GetLogger returns the configured logger.
func (s *MockRepositoryStorage) GetLogger() *zap.Logger {
	return s.logger
}

// Ensure MockRepositoryStorage implements core.RepositoryStorage interface
var _ core.RepositoryStorage = (*MockRepositoryStorage)(nil)
