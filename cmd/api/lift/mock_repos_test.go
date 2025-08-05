package lift

import (
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/mock"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// MockRepositoryStorage is a test mock that allows setting up expectations on repository methods
type MockRepoStorage struct {
	mock.Mock
}

// Repository access methods
func (m *MockRepoStorage) Account() *repositories.AccountRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.AccountRepository)
}

func (m *MockRepoStorage) Actor() *repositories.ActorRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ActorRepository)
}

func (m *MockRepoStorage) Object() *repositories.ObjectRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ObjectRepository)
}

func (m *MockRepoStorage) Activity() *repositories.ActivityRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ActivityRepository)
}

func (m *MockRepoStorage) Timeline() *repositories.TimelineRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.TimelineRepository)
}

func (m *MockRepoStorage) Notification() *repositories.NotificationRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.NotificationRepository)
}

func (m *MockRepoStorage) Like() *repositories.LikeRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.LikeRepository)
}

func (m *MockRepoStorage) Moderation() *repositories.ModerationRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ModerationRepository)
}

func (m *MockRepoStorage) List() *repositories.ListRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ListRepository)
}

func (m *MockRepoStorage) Media() *repositories.MediaRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.MediaRepository)
}

func (m *MockRepoStorage) Poll() *repositories.PollRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.PollRepository)
}

func (m *MockRepoStorage) PushSubscription() *repositories.PushSubscriptionRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.PushSubscriptionRepository)
}

func (m *MockRepoStorage) Hashtag() *repositories.HashtagRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.HashtagRepository)
}

func (m *MockRepoStorage) ScheduledStatus() *repositories.ScheduledStatusRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ScheduledStatusRepository)
}

func (m *MockRepoStorage) Announcement() *repositories.AnnouncementRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.AnnouncementRepository)
}

func (m *MockRepoStorage) DomainBlock() *repositories.DomainBlockRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.DomainBlockRepository)
}

func (m *MockRepoStorage) Relationship() *repositories.RelationshipRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.RelationshipRepository)
}

func (m *MockRepoStorage) Instance() *repositories.InstanceRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.InstanceRepository)
}

func (m *MockRepoStorage) Federation() *repositories.FederationRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.FederationRepository)
}

func (m *MockRepoStorage) Recovery() *repositories.RecoveryRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.RecoveryRepository)
}

func (m *MockRepoStorage) Analytics() *repositories.TrendingRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.TrendingRepository)
}

func (m *MockRepoStorage) Social() *repositories.SocialRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.SocialRepository)
}

func (m *MockRepoStorage) User() *repositories.UserRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.UserRepository)
}

func (m *MockRepoStorage) Status() *repositories.StatusRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.StatusRepository)
}

func (m *MockRepoStorage) Cost() *repositories.CostTrackingRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.CostTrackingRepository)
}

func (m *MockRepoStorage) Trust() *repositories.TrustRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.TrustRepository)
}

func (m *MockRepoStorage) Search() *repositories.SearchRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.SearchRepository)
}

func (m *MockRepoStorage) Relay() *repositories.RelayRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.RelayRepository)
}

func (m *MockRepoStorage) CommunityNote() *repositories.CommunityNoteRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.CommunityNoteRepository)
}

func (m *MockRepoStorage) Emoji() *repositories.EmojiRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.EmojiRepository)
}

func (m *MockRepoStorage) RateLimit() *repositories.RateLimitRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.RateLimitRepository)
}

func (m *MockRepoStorage) Conversation() *repositories.ConversationRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ConversationRepository)
}

func (m *MockRepoStorage) Marker() *repositories.MarkerRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.MarkerRepository)
}

func (m *MockRepoStorage) FeaturedTag() *repositories.FeaturedTagRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.FeaturedTagRepository)
}

func (m *MockRepoStorage) AI() *repositories.AIRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.AIRepository)
}

func (m *MockRepoStorage) Export() *repositories.ExportRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ExportRepository)
}

func (m *MockRepoStorage) Import() *repositories.ImportRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ImportRepository)
}

func (m *MockRepoStorage) DLQ() *repositories.DLQRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.DLQRepository)
}

// Utility methods
func (m *MockRepoStorage) GetDB() dynamormCore.DB {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(dynamormCore.DB)
}

func (m *MockRepoStorage) GetTableName() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockRepoStorage) GetLogger() *zap.Logger {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*zap.Logger)
}

// Ensure MockRepoStorage implements RepositoryStorage interface
var _ core.RepositoryStorage = (*MockRepoStorage)(nil)
