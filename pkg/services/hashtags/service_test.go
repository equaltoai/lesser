package hashtags

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockHashtagRepository struct {
	mock.Mock
}

// Compile-time check to ensure mockHashtagRepository implements HashtagRepository
var _ HashtagRepository = (*mockHashtagRepository)(nil)

func (m *mockHashtagRepository) FollowHashtag(ctx context.Context, userID, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

func (m *mockHashtagRepository) UnfollowHashtag(ctx context.Context, userID, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

func (m *mockHashtagRepository) IsFollowingHashtag(ctx context.Context, userID, hashtag string) (bool, error) {
	args := m.Called(ctx, userID, hashtag)
	return args.Bool(0), args.Error(1)
}

func (m *mockHashtagRepository) GetFollowedHashtags(ctx context.Context, userID string, limit int, cursor string) ([]*storage.HashtagFollow, string, error) {
	args := m.Called(ctx, userID, limit, cursor)
	return args.Get(0).([]*storage.HashtagFollow), args.String(1), args.Error(2)
}

func (m *mockHashtagRepository) GetHashtagInfo(ctx context.Context, hashtag string) (*storage.Hashtag, error) {
	args := m.Called(ctx, hashtag)
	if v := args.Get(0); v != nil {
		return v.(*storage.Hashtag), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockHashtagRepository) GetHashtagStats(ctx context.Context, hashtag string) (any, error) {
	args := m.Called(ctx, hashtag)
	return args.Get(0), args.Error(1)
}

func (m *mockHashtagRepository) GetHashtagTimelineAdvanced(ctx context.Context, hashtag string, maxID *string, limit int, visibility string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, hashtag, maxID, limit, visibility)
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *mockHashtagRepository) GetTrendingHashtags(context.Context, time.Time, int) ([]*storage.TrendingHashtag, error) {
	return nil, nil
}

func (m *mockHashtagRepository) UpdateHashtagNotificationSettings(ctx context.Context, userID, hashtag string, settings *storage.HashtagNotificationSettings) error {
	args := m.Called(ctx, userID, hashtag, settings)
	return args.Error(0)
}

func (m *mockHashtagRepository) MuteHashtag(ctx context.Context, userID, hashtag string, until *time.Time) error {
	args := m.Called(ctx, userID, hashtag, until)
	return args.Error(0)
}

func (m *mockHashtagRepository) UnmuteHashtag(context.Context, string, string) error {
	return nil
}

func (m *mockHashtagRepository) IsHashtagMuted(ctx context.Context, userID, hashtag string) (bool, error) {
	args := m.Called(ctx, userID, hashtag)
	return args.Bool(0), args.Error(1)
}

func (m *mockHashtagRepository) GetHashtagNotificationSettings(ctx context.Context, userID, hashtag string) (*storage.HashtagNotificationSettings, error) {
	args := m.Called(ctx, userID, hashtag)
	if v := args.Get(0); v != nil {
		return v.(*storage.HashtagNotificationSettings), args.Error(1)
	}
	return nil, args.Error(1)
}

type mockPublisher struct {
	mock.Mock
}

func (m *mockPublisher) PublishToUser(ctx context.Context, userID string, event *streaming.Event) error {
	args := m.Called(ctx, userID, event)
	return args.Error(0)
}

func (m *mockPublisher) PublishToStream(ctx context.Context, streamName string, event *streaming.Event) error {
	args := m.Called(ctx, streamName, event)
	return args.Error(0)
}

func (m *mockPublisher) PublishToConversation(ctx context.Context, conversationID string, event *streaming.Event) error {
	args := m.Called(ctx, conversationID, event)
	return args.Error(0)
}

func (m *mockPublisher) Close() error {
	args := m.Called()
	return args.Error(0)
}

func newTestService(repo *mockHashtagRepository, publisher streaming.Publisher) *Service {
	return NewService(repo, nil, nil, publisher, zap.NewNop())
}

func TestNewService(t *testing.T) {
	repo := new(mockHashtagRepository)
	publisher := new(mockPublisher)
	service := NewService(repo, nil, nil, publisher, zap.NewNop())

	require.NotNil(t, service)
	// Can't check private fields, but service should not be nil
}

func TestGetHashtag(t *testing.T) {
	ctx := context.Background()
	repo := new(mockHashtagRepository)

	repo.On("GetHashtagInfo", mock.Anything, "golang").Return(&storage.Hashtag{
		Name:       "golang",
		URL:        "https://example.com/tags/golang",
		UsageCount: 21,
		Accounts:   5,
		CreatedAt:  time.Now().Add(-24 * time.Hour),
		UpdatedAt:  time.Now(),
	}, nil)
	repo.On("GetHashtagStats", mock.Anything, "golang").Return(&storage.HashtagStats{
		Name:          "golang",
		UsageCount:    30,
		TotalAccounts: 7,
		TrendingScore: 1.5,
	}, nil)
	repo.On("IsFollowingHashtag", mock.Anything, "alice", "golang").Return(true, nil)
	repo.On("GetHashtagNotificationSettings", mock.Anything, "alice", "golang").Return(&storage.HashtagNotificationSettings{
		UserID:  "alice",
		Hashtag: "golang",
		Level:   "all",
	}, nil)
	repo.On("IsHashtagMuted", mock.Anything, "alice", "golang").Return(false, nil)
	repo.On("GetHashtagTimelineAdvanced", mock.Anything, "golang", (*string)(nil), defaultRelatedHashtagSampleSize, "").Return([]*storage.StatusSearchResult{
		{Content: "Learning #golang with #testing"},
		{Content: "Advanced #golang for #devs"},
	}, nil)

	service := newTestService(repo, nil)
	result, err := service.GetHashtag(ctx, "golang", "alice")
	require.NoError(t, err)

	assert.Equal(t, "golang", result.Name)
	assert.Equal(t, "https://example.com/tags/golang", result.URL)
	assert.True(t, result.IsFollowing)
	assert.False(t, result.IsMuted)
	assert.NotNil(t, result.NotificationSettings)
	assert.Equal(t, "all", result.NotificationSettings.Level)
	assert.Equal(t, 30, result.PostCount)
	assert.Equal(t, 7, result.FollowerCount)
	assert.Greater(t, result.TrendingScore, 0.0)
	assert.NotEmpty(t, result.Related)

	repo.AssertExpectations(t)
}

func TestFollowHashtag(t *testing.T) {
	ctx := context.Background()
	repo := new(mockHashtagRepository)
	publisher := new(mockPublisher)

	repo.On("FollowHashtag", mock.Anything, "alice", "golang").Return(nil).Once()
	repo.On("UpdateHashtagNotificationSettings", mock.Anything, "alice", "golang", mock.AnythingOfType("*storage.HashtagNotificationSettings")).Return(nil).Once()
	repo.On("GetHashtagInfo", mock.Anything, "golang").Return(&storage.Hashtag{
		Name:       "golang",
		URL:        "https://example.com/tags/golang",
		UsageCount: 10,
		Accounts:   2,
		CreatedAt:  time.Now().Add(-time.Hour),
		UpdatedAt:  time.Now(),
	}, nil)
	repo.On("GetHashtagStats", mock.Anything, "golang").Return(&storage.HashtagStats{
		Name:          "golang",
		UsageCount:    12,
		TotalAccounts: 3,
		TrendingScore: 2.3,
	}, nil)
	repo.On("IsFollowingHashtag", mock.Anything, "alice", "golang").Return(true, nil)
	repo.On("GetHashtagNotificationSettings", mock.Anything, "alice", "golang").Return(&storage.HashtagNotificationSettings{
		UserID:  "alice",
		Hashtag: "golang",
		Level:   "mentions",
	}, nil)
	repo.On("IsHashtagMuted", mock.Anything, "alice", "golang").Return(false, nil)
	repo.On("GetHashtagTimelineAdvanced", mock.Anything, "golang", (*string)(nil), defaultRelatedHashtagSampleSize, "").Return([]*storage.StatusSearchResult{}, nil)

	publisher.On("PublishToUser", mock.Anything, "alice", mock.MatchedBy(func(event *streaming.Event) bool {
		return event != nil && event.Type == streaming.HashtagFollowed
	})).Return(nil).Once()
	publisher.On("PublishToStream", mock.Anything, streaming.HashtagStreamName("golang"), mock.MatchedBy(func(event *streaming.Event) bool {
		return event != nil && event.Type == string(streaming.EventTypeHashtagUpdate)
	})).Return(nil).Once()

	service := newTestService(repo, publisher)

	settings := &storage.HashtagNotificationSettings{
		Level: "mentions",
	}

	result, err := service.FollowHashtag(ctx, "alice", "golang", settings)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.IsFollowing)
	assert.Equal(t, "golang", result.Name)

	repo.AssertExpectations(t)
	publisher.AssertExpectations(t)
	publisher.AssertNotCalled(t, "PublishToStream", mock.Anything, streaming.HashtagStreamName("golang"), mock.MatchedBy(func(event *streaming.Event) bool {
		return event != nil && event.Type == streaming.HashtagFollowed
	}))
}

func TestUnfollowHashtag(t *testing.T) {
	ctx := context.Background()
	repo := new(mockHashtagRepository)
	publisher := new(mockPublisher)

	repo.On("UnfollowHashtag", mock.Anything, "alice", "golang").Return(nil).Once()
	repo.On("GetHashtagInfo", mock.Anything, "golang").Return(&storage.Hashtag{
		Name:       "golang",
		URL:        "https://example.com/tags/golang",
		UsageCount: 8,
		Accounts:   1,
		CreatedAt:  time.Now().Add(-2 * time.Hour),
		UpdatedAt:  time.Now(),
	}, nil)
	repo.On("GetHashtagStats", mock.Anything, "golang").Return(&storage.HashtagStats{
		Name:          "golang",
		UsageCount:    8,
		TotalAccounts: 1,
	}, nil)
	repo.On("IsFollowingHashtag", mock.Anything, "alice", "golang").Return(false, nil)
	repo.On("GetHashtagNotificationSettings", mock.Anything, "alice", "golang").Return((*storage.HashtagNotificationSettings)(nil), nil)
	repo.On("IsHashtagMuted", mock.Anything, "alice", "golang").Return(false, nil)
	repo.On("GetHashtagTimelineAdvanced", mock.Anything, "golang", (*string)(nil), defaultRelatedHashtagSampleSize, "").Return([]*storage.StatusSearchResult{}, nil)

	publisher.On("PublishToUser", mock.Anything, "alice", mock.MatchedBy(func(event *streaming.Event) bool {
		return event != nil && event.Type == "hashtag.unfollowed"
	})).Return(nil).Once()
	publisher.On("PublishToStream", mock.Anything, streaming.HashtagStreamName("golang"), mock.MatchedBy(func(event *streaming.Event) bool {
		return event != nil && event.Type == string(streaming.EventTypeHashtagUpdate)
	})).Return(nil).Once()

	service := newTestService(repo, publisher)

	result, err := service.UnfollowHashtag(ctx, "alice", "golang")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.False(t, result.IsFollowing)

	repo.AssertExpectations(t)
	publisher.AssertExpectations(t)
	publisher.AssertNotCalled(t, "PublishToStream", mock.Anything, streaming.HashtagStreamName("golang"), mock.MatchedBy(func(event *streaming.Event) bool {
		return event != nil && event.Type == "hashtag.unfollowed"
	}))
}

func TestGetFollowedHashtags(t *testing.T) {
	ctx := context.Background()
	repo := new(mockHashtagRepository)

	now := time.Now()
	repo.On("GetFollowedHashtags", mock.Anything, "alice", defaultFollowedHashtagLimit, "").Return([]*storage.HashtagFollow{
		{
			UserID:  "alice",
			Hashtag: "golang",
			CreatedAt: func() time.Time {
				return now.Add(-time.Minute)
			}(),
		},
		{
			UserID:  "alice",
			Hashtag: "testing",
			CreatedAt: func() time.Time {
				return now.Add(-2 * time.Minute)
			}(),
		},
	}, "cursor", nil).Once()

	repo.On("GetHashtagInfo", mock.Anything, "golang").Return(&storage.Hashtag{
		Name:       "golang",
		URL:        "https://example.com/tags/golang",
		UsageCount: 10,
		Accounts:   3,
		CreatedAt:  now.Add(-time.Hour),
		UpdatedAt:  now,
	}, nil)
	repo.On("GetHashtagInfo", mock.Anything, "testing").Return(&storage.Hashtag{
		Name:       "testing",
		URL:        "https://example.com/tags/testing",
		UsageCount: 7,
		Accounts:   2,
		CreatedAt:  now.Add(-2 * time.Hour),
		UpdatedAt:  now,
	}, nil)

	repo.On("GetHashtagStats", mock.Anything, "golang").Return(&storage.HashtagStats{Name: "golang", UsageCount: 11}, nil)
	repo.On("GetHashtagStats", mock.Anything, "testing").Return(&storage.HashtagStats{Name: "testing", UsageCount: 7}, nil)

	repo.On("IsFollowingHashtag", mock.Anything, "alice", mock.AnythingOfType("string")).Return(true, nil)
	repo.On("GetHashtagNotificationSettings", mock.Anything, "alice", mock.AnythingOfType("string")).Return((*storage.HashtagNotificationSettings)(nil), nil)
	repo.On("IsHashtagMuted", mock.Anything, "alice", mock.AnythingOfType("string")).Return(false, nil)
	repo.On("GetHashtagTimelineAdvanced", mock.Anything, mock.AnythingOfType("string"), (*string)(nil), defaultRelatedHashtagSampleSize, "").Return([]*storage.StatusSearchResult{}, nil)

	service := newTestService(repo, nil)
	results, cursor, err := service.GetFollowedHashtags(ctx, "alice", nil)
	require.NoError(t, err)

	assert.Len(t, results, 2)
	assert.Equal(t, "cursor", cursor)
	assert.Equal(t, "golang", results[0].Name)
	assert.NotNil(t, results[0].FollowedAt)
	assert.Equal(t, "testing", results[1].Name)

	repo.AssertExpectations(t)
}

func TestMuteHashtag(t *testing.T) {
	ctx := context.Background()
	repo := new(mockHashtagRepository)
	publisher := new(mockPublisher)

	repo.On("MuteHashtag", mock.Anything, "alice", "golang", (*time.Time)(nil)).Return(nil).Once()
	repo.On("GetHashtagInfo", mock.Anything, "golang").Return(&storage.Hashtag{
		Name:       "golang",
		URL:        "https://example.com/tags/golang",
		UsageCount: 5,
		Accounts:   1,
		CreatedAt:  time.Now().Add(-3 * time.Hour),
		UpdatedAt:  time.Now(),
	}, nil)
	repo.On("GetHashtagStats", mock.Anything, "golang").Return(&storage.HashtagStats{Name: "golang", UsageCount: 5}, nil)
	repo.On("IsFollowingHashtag", mock.Anything, "alice", "golang").Return(true, nil)
	repo.On("GetHashtagNotificationSettings", mock.Anything, "alice", "golang").Return((*storage.HashtagNotificationSettings)(nil), nil)
	repo.On("IsHashtagMuted", mock.Anything, "alice", "golang").Return(true, nil)
	repo.On("GetHashtagTimelineAdvanced", mock.Anything, "golang", (*string)(nil), defaultRelatedHashtagSampleSize, "").Return([]*storage.StatusSearchResult{}, nil)

	publisher.On("PublishToUser", mock.Anything, "alice", mock.MatchedBy(func(event *streaming.Event) bool {
		return event != nil && event.Type == "hashtag.muted"
	})).Return(nil).Once()
	publisher.On("PublishToStream", mock.Anything, streaming.HashtagStreamName("golang"), mock.MatchedBy(func(event *streaming.Event) bool {
		return event != nil && event.Type == string(streaming.EventTypeHashtagUpdate)
	})).Return(nil).Once()

	service := newTestService(repo, publisher)
	result, err := service.MuteHashtag(ctx, "alice", "golang", nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsMuted)

	repo.AssertExpectations(t)
	publisher.AssertExpectations(t)
	publisher.AssertNotCalled(t, "PublishToStream", mock.Anything, streaming.HashtagStreamName("golang"), mock.MatchedBy(func(event *streaming.Event) bool {
		return event != nil && event.Type == "hashtag.muted"
	}))
}

// TestGetHashtagActivity verifies the deprecated GetHashtagActivity method returns empty channel
// DEPRECATED: This test is kept for backward compatibility validation
func TestGetHashtagActivity(t *testing.T) {
	logger := zap.NewNop()
	service := NewService(nil, nil, nil, nil, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	activityCh, err := service.GetHashtagActivity(ctx, []string{"golang"})
	require.NoError(t, err)
	require.NotNil(t, activityCh)

	// Channel should be closed immediately (deprecated behavior)
	select {
	case _, ok := <-activityCh:
		assert.False(t, ok, "channel should be closed for deprecated method")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("channel should be closed immediately")
	}
}
