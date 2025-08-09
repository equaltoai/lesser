// Package lift provides the Lift framework handlers for the Lesser API.
// This file contains test mocks for the repository storage interface.
package lift

import (
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockRepositoryStorage implements core.RepositoryStorage for testing purposes.
// It provides a mock implementation of all repository methods using testify/mock.
type MockRepositoryStorage struct {
	mock.Mock
}

// Ensure MockRepositoryStorage implements core.RepositoryStorage
var _ core.RepositoryStorage = (*MockRepositoryStorage)(nil)

// Account returns a mock account repository for testing
func (m *MockRepositoryStorage) Account() *repositories.AccountRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.AccountRepository)
}

// Actor returns a mock actor repository for testing
func (m *MockRepositoryStorage) Actor() *repositories.ActorRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ActorRepository)
}

// Object returns a mock object repository for testing
func (m *MockRepositoryStorage) Object() *repositories.ObjectRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ObjectRepository)
}

// Activity returns a mock activity repository for testing
func (m *MockRepositoryStorage) Activity() *repositories.ActivityRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ActivityRepository)
}

// Timeline returns a mock timeline repository for testing
func (m *MockRepositoryStorage) Timeline() *repositories.TimelineRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.TimelineRepository)
}

// Notification returns a mock notification repository for testing
func (m *MockRepositoryStorage) Notification() *repositories.NotificationRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.NotificationRepository)
}

// Like returns a mock like repository for testing
func (m *MockRepositoryStorage) Like() *repositories.LikeRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.LikeRepository)
}

// Moderation returns a mock moderation repository for testing
func (m *MockRepositoryStorage) Moderation() *repositories.ModerationRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ModerationRepository)
}

// List returns a mock list repository for testing
func (m *MockRepositoryStorage) List() *repositories.ListRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ListRepository)
}

// Media returns a mock media repository for testing
func (m *MockRepositoryStorage) Media() *repositories.MediaRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.MediaRepository)
}

// Poll returns a mock poll repository for testing
func (m *MockRepositoryStorage) Poll() *repositories.PollRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.PollRepository)
}

// PushSubscription returns a mock pushsubscription repository for testing
func (m *MockRepositoryStorage) PushSubscription() *repositories.PushSubscriptionRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.PushSubscriptionRepository)
}

// Hashtag returns a mock hashtag repository for testing
func (m *MockRepositoryStorage) Hashtag() *repositories.HashtagRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.HashtagRepository)
}

// ScheduledStatus returns a mock scheduledstatus repository for testing
func (m *MockRepositoryStorage) ScheduledStatus() *repositories.ScheduledStatusRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ScheduledStatusRepository)
}

// Announcement returns a mock announcement repository for testing
func (m *MockRepositoryStorage) Announcement() *repositories.AnnouncementRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.AnnouncementRepository)
}

// DomainBlock returns a mock domainblock repository for testing
func (m *MockRepositoryStorage) DomainBlock() *repositories.DomainBlockRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.DomainBlockRepository)
}

// Relationship returns a mock relationship repository for testing
func (m *MockRepositoryStorage) Relationship() *repositories.RelationshipRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.RelationshipRepository)
}

// Instance returns a mock instance repository for testing
func (m *MockRepositoryStorage) Instance() *repositories.InstanceRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.InstanceRepository)
}

// Federation returns a mock federation repository for testing
func (m *MockRepositoryStorage) Federation() *repositories.FederationRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.FederationRepository)
}

// Recovery returns a mock recovery repository for testing
func (m *MockRepositoryStorage) Recovery() *repositories.RecoveryRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.RecoveryRepository)
}

// Analytics returns a mock analytics repository for testing
func (m *MockRepositoryStorage) Analytics() *repositories.TrendingRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.TrendingRepository)
}

// Social returns a mock social repository for testing
func (m *MockRepositoryStorage) Social() *repositories.SocialRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.SocialRepository)
}

// User returns a mock user repository for testing
func (m *MockRepositoryStorage) User() *repositories.UserRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.UserRepository)
}

// Status returns a mock status repository for testing
func (m *MockRepositoryStorage) Status() *repositories.StatusRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.StatusRepository)
}

// Cost returns a mock cost repository for testing
func (m *MockRepositoryStorage) Cost() *repositories.CostTrackingRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.CostTrackingRepository)
}

// Trust returns a mock trust repository for testing
func (m *MockRepositoryStorage) Trust() *repositories.TrustRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.TrustRepository)
}

// Search returns a mock search repository for testing
func (m *MockRepositoryStorage) Search() *repositories.SearchRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.SearchRepository)
}

// Relay returns a mock relay repository for testing
func (m *MockRepositoryStorage) Relay() *repositories.RelayRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.RelayRepository)
}

// CommunityNote returns a mock communitynote repository for testing
func (m *MockRepositoryStorage) CommunityNote() *repositories.CommunityNoteRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.CommunityNoteRepository)
}

// Emoji returns a mock emoji repository for testing
func (m *MockRepositoryStorage) Emoji() *repositories.EmojiRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.EmojiRepository)
}

// RateLimit returns a mock ratelimit repository for testing
func (m *MockRepositoryStorage) RateLimit() *repositories.RateLimitRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.RateLimitRepository)
}

// Conversation returns a mock conversation repository for testing
func (m *MockRepositoryStorage) Conversation() *repositories.ConversationRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ConversationRepository)
}

// Marker returns a mock marker repository for testing
func (m *MockRepositoryStorage) Marker() *repositories.MarkerRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.MarkerRepository)
}

// FeaturedTag returns a mock featuredtag repository for testing
func (m *MockRepositoryStorage) FeaturedTag() *repositories.FeaturedTagRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.FeaturedTagRepository)
}

// AI returns a mock ai repository for testing
func (m *MockRepositoryStorage) AI() *repositories.AIRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.AIRepository)
}

// Export returns a mock export repository for testing
func (m *MockRepositoryStorage) Export() *repositories.ExportRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ExportRepository)
}

// Import returns a mock import repository for testing
func (m *MockRepositoryStorage) Import() *repositories.ImportRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ImportRepository)
}

// DLQ returns a mock dlq repository for testing
func (m *MockRepositoryStorage) DLQ() *repositories.DLQRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.DLQRepository)
}

// GetDB returns a mock getdb repository for testing
func (m *MockRepositoryStorage) GetDB() dynamormCore.DB {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(dynamormCore.DB)
}

// GetTableName returns a mock gettablename repository for testing
func (m *MockRepositoryStorage) GetTableName() string {
	args := m.Called()
	return args.String(0)
}

// GetLogger returns a mock getlogger repository for testing
func (m *MockRepositoryStorage) GetLogger() *zap.Logger {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*zap.Logger)
}

// MetricRecord returns a mock metricrecord repository for testing
func (m *MockRepositoryStorage) MetricRecord() *repositories.MetricRecordRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.MetricRecordRepository)
}

// CloudWatchMetrics returns a mock cloudwatchmetrics repository for testing
func (m *MockRepositoryStorage) CloudWatchMetrics() *repositories.CloudWatchMetricsRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.CloudWatchMetricsRepository)
}
