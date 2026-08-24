package theorydb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

// TestSimpleAdapter tests basic adapter functionality without complex mocking
func TestSimpleAdapter(t *testing.T) {
	t.Run("adapter creation without database connection", func(t *testing.T) {
		// Create a minimal repository storage mock using nil DB
		// This tests adapter creation and interface compliance

		logger := zaptest.NewLogger(t)

		// Create simple repository storage that satisfies the interface
		repoStorage := &SimpleRepositoryStorage{
			db:        nil, // No real DB needed for interface test
			tableName: "test-table",
			logger:    logger,
		}

		// Create adapter
		adapter := NewStorageAdapter(repoStorage)
		require.NotNil(t, adapter)

		// Verify it implements the storage interface
		var _ interfaces.Storage = adapter

		// Test basic accessor methods
		assert.Equal(t, "test-table", adapter.GetTableName())
		assert.Equal(t, logger, adapter.GetLogger())
	})
}

// SimpleRepositoryStorage provides a minimal implementation for testing
type SimpleRepositoryStorage struct {
	db        dynamormCore.DB
	tableName string
	logger    *zap.Logger
}

func (s *SimpleRepositoryStorage) GetDB() dynamormCore.DB { return s.db }
func (s *SimpleRepositoryStorage) GetTableName() string   { return s.tableName }
func (s *SimpleRepositoryStorage) GetLogger() *zap.Logger { return s.logger }

// Repository access methods - return nil repositories for testing
// This is sufficient to test interface compliance and basic adapter functionality
func (s *SimpleRepositoryStorage) Account() *repositories.AccountRepository             { return nil }
func (s *SimpleRepositoryStorage) Bookmark() *repositories.BookmarkRepository           { return nil }
func (s *SimpleRepositoryStorage) Actor() interfaces.ActorRepository                    { return nil }
func (s *SimpleRepositoryStorage) Object() interfaces.ObjectRepository                  { return nil }
func (s *SimpleRepositoryStorage) Activity() interfaces.ActivityRepository              { return nil }
func (s *SimpleRepositoryStorage) Timeline() interfaces.TimelineRepository              { return nil }
func (s *SimpleRepositoryStorage) Notification() interfaces.NotificationRepository      { return nil }
func (s *SimpleRepositoryStorage) Like() *repositories.LikeRepository                   { return nil }
func (s *SimpleRepositoryStorage) Moderation() interfaces.ModerationRepository          { return nil }
func (s *SimpleRepositoryStorage) List() *repositories.ListRepository                   { return nil }
func (s *SimpleRepositoryStorage) Media() *repositories.MediaRepository                 { return nil }
func (s *SimpleRepositoryStorage) MediaMetadata() *repositories.MediaMetadataRepository { return nil }
func (s *SimpleRepositoryStorage) Poll() *repositories.PollRepository                   { return nil }
func (s *SimpleRepositoryStorage) PushSubscription() *repositories.PushSubscriptionRepository {
	return nil
}
func (s *SimpleRepositoryStorage) Hashtag() *repositories.HashtagRepository { return nil }
func (s *SimpleRepositoryStorage) ScheduledStatus() *repositories.ScheduledStatusRepository {
	return nil
}
func (s *SimpleRepositoryStorage) Announcement() *repositories.AnnouncementRepository { return nil }
func (s *SimpleRepositoryStorage) DomainBlock() *repositories.DomainBlockRepository   { return nil }
func (s *SimpleRepositoryStorage) Relationship() interfaces.ConcreteRelationshipRepository {
	return nil
}
func (s *SimpleRepositoryStorage) Instance() *repositories.InstanceRepository           { return nil }
func (s *SimpleRepositoryStorage) Federation() *repositories.FederationRepository       { return nil }
func (s *SimpleRepositoryStorage) Recovery() *repositories.RecoveryRepository           { return nil }
func (s *SimpleRepositoryStorage) Analytics() *repositories.TrendingRepository          { return nil }
func (s *SimpleRepositoryStorage) Social() *repositories.SocialRepository               { return nil }
func (s *SimpleRepositoryStorage) User() interfaces.UserRepository                      { return nil }
func (s *SimpleRepositoryStorage) Status() interfaces.StatusRepository                  { return nil }
func (s *SimpleRepositoryStorage) Cost() *repositories.TrackingRepository               { return nil }
func (s *SimpleRepositoryStorage) WebSocketCost() *repositories.WebSocketCostRepository { return nil }
func (s *SimpleRepositoryStorage) Trust() interfaces.TrustRepository                    { return nil }
func (s *SimpleRepositoryStorage) Search() *repositories.SearchRepository               { return nil }
func (s *SimpleRepositoryStorage) Relay() *repositories.RelayRepository                 { return nil }
func (s *SimpleRepositoryStorage) CommunityNote() *repositories.CommunityNoteRepository { return nil }
func (s *SimpleRepositoryStorage) Emoji() *repositories.EmojiRepository                 { return nil }
func (s *SimpleRepositoryStorage) RateLimit() *repositories.RateLimitRepository         { return nil }
func (s *SimpleRepositoryStorage) Conversation() *repositories.ConversationRepository   { return nil }
func (s *SimpleRepositoryStorage) Marker() *repositories.MarkerRepository               { return nil }
func (s *SimpleRepositoryStorage) FeaturedTag() *repositories.FeaturedTagRepository     { return nil }
func (s *SimpleRepositoryStorage) AI() *repositories.AIRepository                       { return nil }
func (s *SimpleRepositoryStorage) Export() *repositories.ExportRepository               { return nil }
func (s *SimpleRepositoryStorage) Import() *repositories.ImportRepository               { return nil }
func (s *SimpleRepositoryStorage) DLQ() *repositories.DLQRepository                     { return nil }
func (s *SimpleRepositoryStorage) MetricRecord() *repositories.MetricRecordRepository   { return nil }
func (s *SimpleRepositoryStorage) CloudWatchMetrics() *repositories.CloudWatchMetricsRepository {
	return nil
}
func (s *SimpleRepositoryStorage) StreamingCloudWatch() *repositories.StreamingCloudWatchRepository {
	return nil
}
func (s *SimpleRepositoryStorage) Audit() *repositories.AuditRepository               { return nil }
func (s *SimpleRepositoryStorage) OAuth() *repositories.OAuthRepository               { return nil }
func (s *SimpleRepositoryStorage) Skill() interfaces.SkillRepository                  { return nil }
func (s *SimpleRepositoryStorage) DNSCache() *repositories.DNSCacheRepository         { return nil }
func (s *SimpleRepositoryStorage) Filter() *repositories.FilterRepository             { return nil }
func (s *SimpleRepositoryStorage) Thread() *repositories.ThreadRepository             { return nil }
func (s *SimpleRepositoryStorage) Severance() *repositories.SeveranceRepository       { return nil }
func (s *SimpleRepositoryStorage) ModerationML() *repositories.ModerationMLRepository { return nil }
func (s *SimpleRepositoryStorage) Quote() *repositories.QuoteRepository               { return nil }
func (s *SimpleRepositoryStorage) MediaAnalytics() interfaces.MediaAnalyticsRepository {
	return nil
}
func (s *SimpleRepositoryStorage) MediaPopularity() interfaces.MediaPopularityRepository {
	return nil
}
func (s *SimpleRepositoryStorage) MediaSession() interfaces.MediaSessionRepository { return nil }
func (s *SimpleRepositoryStorage) StreamingConnection() interfaces.StreamingConnectionRepository {
	return nil
}

func (s *SimpleRepositoryStorage) Article() interfaces.ArticleRepository           { return nil }
func (s *SimpleRepositoryStorage) Draft() interfaces.DraftRepository               { return nil }
func (s *SimpleRepositoryStorage) UploadGrant() interfaces.UploadGrantRepository   { return nil }
func (s *SimpleRepositoryStorage) PromoPackage() interfaces.PromoPackageRepository { return nil }
func (s *SimpleRepositoryStorage) Revision() interfaces.RevisionRepository         { return nil }
func (s *SimpleRepositoryStorage) Series() interfaces.SeriesRepository             { return nil }
func (s *SimpleRepositoryStorage) Category() interfaces.CategoryRepository         { return nil }
func (s *SimpleRepositoryStorage) Publication() interfaces.PublicationRepository   { return nil }
func (s *SimpleRepositoryStorage) PublicationMember() interfaces.PublicationMemberRepository {
	return nil
}
