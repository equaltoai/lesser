package lift

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/mock"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
	
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
)

// MockRepositoryStorage implements core.RepositoryStorage for testing
type MockRepositoryStorage struct {
	mock.Mock
}

// Ensure MockRepositoryStorage implements core.RepositoryStorage
var _ core.RepositoryStorage = (*MockRepositoryStorage)(nil)

func (m *MockRepositoryStorage) Account() *repositories.AccountRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.AccountRepository)
}

func (m *MockRepositoryStorage) Actor() *repositories.ActorRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ActorRepository)
}

func (m *MockRepositoryStorage) Object() *repositories.ObjectRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ObjectRepository)
}

func (m *MockRepositoryStorage) Activity() *repositories.ActivityRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ActivityRepository)
}

func (m *MockRepositoryStorage) Timeline() *repositories.TimelineRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.TimelineRepository)
}

func (m *MockRepositoryStorage) Notification() *repositories.NotificationRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.NotificationRepository)
}

func (m *MockRepositoryStorage) Like() *repositories.LikeRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.LikeRepository)
}

func (m *MockRepositoryStorage) Moderation() *repositories.ModerationRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ModerationRepository)
}

func (m *MockRepositoryStorage) List() *repositories.ListRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ListRepository)
}

func (m *MockRepositoryStorage) Media() *repositories.MediaRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.MediaRepository)
}

func (m *MockRepositoryStorage) Poll() *repositories.PollRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.PollRepository)
}

func (m *MockRepositoryStorage) PushSubscription() *repositories.PushSubscriptionRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.PushSubscriptionRepository)
}

func (m *MockRepositoryStorage) Hashtag() *repositories.HashtagRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.HashtagRepository)
}

func (m *MockRepositoryStorage) ScheduledStatus() *repositories.ScheduledStatusRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ScheduledStatusRepository)
}

func (m *MockRepositoryStorage) Announcement() *repositories.AnnouncementRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.AnnouncementRepository)
}

func (m *MockRepositoryStorage) DomainBlock() *repositories.DomainBlockRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.DomainBlockRepository)
}

func (m *MockRepositoryStorage) Relationship() *repositories.RelationshipRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.RelationshipRepository)
}

func (m *MockRepositoryStorage) Instance() *repositories.InstanceRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.InstanceRepository)
}

func (m *MockRepositoryStorage) Federation() *repositories.FederationRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.FederationRepository)
}

func (m *MockRepositoryStorage) Recovery() *repositories.RecoveryRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.RecoveryRepository)
}

func (m *MockRepositoryStorage) Analytics() *repositories.TrendingRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.TrendingRepository)
}

func (m *MockRepositoryStorage) Social() *repositories.SocialRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.SocialRepository)
}

func (m *MockRepositoryStorage) User() *repositories.UserRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.UserRepository)
}

func (m *MockRepositoryStorage) Status() *repositories.StatusRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.StatusRepository)
}

func (m *MockRepositoryStorage) Cost() *repositories.CostTrackingRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.CostTrackingRepository)
}

func (m *MockRepositoryStorage) Trust() *repositories.TrustRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.TrustRepository)
}

func (m *MockRepositoryStorage) Search() *repositories.SearchRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.SearchRepository)
}

func (m *MockRepositoryStorage) Relay() *repositories.RelayRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.RelayRepository)
}

func (m *MockRepositoryStorage) CommunityNote() *repositories.CommunityNoteRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.CommunityNoteRepository)
}

func (m *MockRepositoryStorage) Emoji() *repositories.EmojiRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.EmojiRepository)
}

func (m *MockRepositoryStorage) RateLimit() *repositories.RateLimitRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.RateLimitRepository)
}

func (m *MockRepositoryStorage) Conversation() *repositories.ConversationRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ConversationRepository)
}

func (m *MockRepositoryStorage) Marker() *repositories.MarkerRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.MarkerRepository)
}

func (m *MockRepositoryStorage) FeaturedTag() *repositories.FeaturedTagRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.FeaturedTagRepository)
}

func (m *MockRepositoryStorage) AI() *repositories.AIRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.AIRepository)
}

func (m *MockRepositoryStorage) Export() *repositories.ExportRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ExportRepository)
}

func (m *MockRepositoryStorage) Import() *repositories.ImportRepository {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*repositories.ImportRepository)
}

func (m *MockRepositoryStorage) GetDB() dynamormCore.DB {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(dynamormCore.DB)
}

func (m *MockRepositoryStorage) GetTableName() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockRepositoryStorage) GetLogger() *zap.Logger {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*zap.Logger)
}


// Helper function to create test context - extracted from statuses_unified_boost_test.go
func createTestContext(method, path, body string, headers map[string]string) *lift.Context {
	req := &lift.Request{
		Request: &adapters.Request{
			Method:  method,
			Path:    path,
			Body:    []byte(body),
			Headers: headers,
		},
	}
	
	ctx := lift.NewContext(context.Background(), req)
	return ctx
}