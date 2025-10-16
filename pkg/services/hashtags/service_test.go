package hashtags

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockHashtagRepository is a mock implementation of HashtagRepository
type MockHashtagRepository struct {
	mock.Mock
}

func (m *MockHashtagRepository) FollowHashtag(ctx context.Context, userID string, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

func (m *MockHashtagRepository) UnfollowHashtag(ctx context.Context, userID string, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

func (m *MockHashtagRepository) IsFollowingHashtag(ctx context.Context, userID string, hashtag string) (bool, error) {
	args := m.Called(ctx, userID, hashtag)
	return args.Bool(0), args.Error(1)
}

func (m *MockHashtagRepository) GetFollowedHashtags(ctx context.Context, userID string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, userID, limit, cursor)
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

func (m *MockHashtagRepository) GetHashtagInfo(ctx context.Context, hashtag string) (*storage.Hashtag, error) {
	args := m.Called(ctx, hashtag)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Hashtag), args.Error(1)
}

func (m *MockHashtagRepository) GetHashtagStats(ctx context.Context, hashtag string) (interface{}, error) {
	args := m.Called(ctx, hashtag)
	return args.Get(0), args.Error(1)
}

func (m *MockHashtagRepository) GetHashtagTimelineAdvanced(ctx context.Context, hashtag string, maxID *string, limit int, visibility string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, hashtag, maxID, limit, visibility)
	if args.Get(0) == nil {
		return []*storage.StatusSearchResult{}, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockHashtagRepository) GetMultiHashtagTimeline(ctx context.Context, hashtags []string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, hashtags, maxID, limit, userID)
	if args.Get(0) == nil {
		return []*storage.StatusSearchResult{}, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockHashtagRepository) GetSuggestedHashtags(ctx context.Context, userID string, limit int) ([]*storage.HashtagSearchResult, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return []*storage.HashtagSearchResult{}, args.Error(1)
	}
	return args.Get(0).([]*storage.HashtagSearchResult), args.Error(1)
}

func (m *MockHashtagRepository) GetTrendingHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return []*storage.TrendingHashtag{}, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingHashtag), args.Error(1)
}

func (m *MockHashtagRepository) GetRecentHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	args := m.Called(ctx, since, limit)
	if args.Get(0) == nil {
		return []*storage.TrendingHashtag{}, args.Error(1)
	}
	return args.Get(0).([]*storage.TrendingHashtag), args.Error(1)
}

func (m *MockHashtagRepository) UpdateHashtagNotificationSettings(ctx context.Context, userID, hashtag string, notify bool) error {
	args := m.Called(ctx, userID, hashtag, notify)
	return args.Error(0)
}

func (m *MockHashtagRepository) MuteHashtag(ctx context.Context, userID, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

func (m *MockHashtagRepository) UnmuteHashtag(ctx context.Context, userID, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

// Test helper to create a test service
func createTestService(hashtagRepo *MockHashtagRepository) *Service {
	logger := zap.NewNop()
	return NewService(
		&repositories.HashtagRepository{}, // Use actual type but won't be called in mocked tests
		nil,                               // statusRepo
		nil,                               // relationshipRepo
		nil,                               // publisher
		logger,
		"example.com",
	)
}

// TestFollowHashtag tests the FollowHashtag method
func TestFollowHashtag(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockHashtagRepository)

	mockRepo.On("FollowHashtag", ctx, "user1", "golang").Return(nil)

	service := createTestService(mockRepo)
	service.hashtagRepo = (*repositories.HashtagRepository)(nil) // Will use mock in actual implementation

	cmd := &FollowHashtagCommand{
		UserID:               "user1",
		Hashtag:              "golang",
		NotificationsEnabled: true,
	}

	result, err := service.FollowHashtag(ctx, cmd)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "golang", result.Hashtag)
}

// TestUnfollowHashtag tests the UnfollowHashtag method
func TestUnfollowHashtag(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockHashtagRepository)

	mockRepo.On("UnfollowHashtag", ctx, "user1", "golang").Return(nil)

	service := createTestService(mockRepo)

	cmd := &UnfollowHashtagCommand{
		UserID:  "user1",
		Hashtag: "golang",
	}

	result, err := service.UnfollowHashtag(ctx, cmd)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "golang", result.Hashtag)
}

// TestGetHashtag tests the GetHashtag method
func TestGetHashtag(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockHashtagRepository)

	expectedHashtag := &storage.Hashtag{
		Name:       "golang",
		URL:        "https://example.com/tags/golang",
		UsageCount: 100,
		FirstSeen:  time.Now().AddDate(0, 0, -30),
		LastUsed:   time.Now(),
	}

	mockRepo.On("GetHashtagInfo", ctx, "golang").Return(expectedHashtag, nil)
	mockRepo.On("IsFollowingHashtag", ctx, "user1", "golang").Return(true, nil)
	mockRepo.On("GetHashtagStats", ctx, "golang").Return(nil, nil)

	service := createTestService(mockRepo)

	query := &GetHashtagQuery{
		Name:     "golang",
		ViewerID: "user1",
	}

	result, err := service.GetHashtag(ctx, query)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "golang", result.Name)
	assert.True(t, result.Following)
}

// TestGetHashtagTimeline tests the GetHashtagTimeline method
func TestGetHashtagTimeline(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockHashtagRepository)

	expectedPosts := []*storage.StatusSearchResult{
		{
			StatusID:  "post1",
			Content:   "Test post #golang",
			URL:       "https://example.com/posts/post1",
			Published: time.Now(),
		},
	}

	mockRepo.On("GetHashtagTimelineAdvanced", ctx, "golang", (*string)(nil), 20, "").Return(expectedPosts, nil)

	service := createTestService(mockRepo)

	query := &GetHashtagTimelineQuery{
		Hashtag:  "golang",
		First:    20,
		ViewerID: "user1",
	}

	result, err := service.GetHashtagTimeline(ctx, query)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Posts, 1)
	assert.Equal(t, "post1", result.Posts[0].StatusID)
}

// TestGetFollowedHashtags tests the GetFollowedHashtags method
func TestGetFollowedHashtags(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockHashtagRepository)

	expectedHashtags := []string{"golang", "typescript", "rust"}

	mockRepo.On("GetFollowedHashtags", ctx, "user1", 20, "").Return(expectedHashtags, "", nil)
	mockRepo.On("GetHashtagInfo", ctx, "golang").Return(&storage.Hashtag{
		Name:       "golang",
		URL:        "https://example.com/tags/golang",
		UsageCount: 100,
	}, nil)
	mockRepo.On("GetHashtagInfo", ctx, "typescript").Return(&storage.Hashtag{
		Name:       "typescript",
		URL:        "https://example.com/tags/typescript",
		UsageCount: 50,
	}, nil)
	mockRepo.On("GetHashtagInfo", ctx, "rust").Return(&storage.Hashtag{
		Name:       "rust",
		URL:        "https://example.com/tags/rust",
		UsageCount: 75,
	}, nil)

	service := createTestService(mockRepo)

	query := &GetFollowedHashtagsQuery{
		UserID: "user1",
		First:  20,
	}

	result, err := service.GetFollowedHashtags(ctx, query)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Hashtags, 3)
}

// TestValidateHashtagName tests hashtag name validation
func TestValidateHashtagName(t *testing.T) {
	service := createTestService(nil)

	tests := []struct {
		name      string
		hashtag   string
		expectErr bool
	}{
		{"Valid hashtag", "golang", false},
		{"Valid with hash", "#golang", false},
		{"Empty hashtag", "", true},
		{"Too long hashtag", string(make([]byte, 101)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.validateHashtagName(tt.hashtag)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExtractHashtagsFromContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "Single hashtag",
			content:  "This is a post with #golang",
			expected: []string{"golang"},
		},
		{
			name:     "Multiple hashtags",
			content:  "I love #golang and #programming!",
			expected: []string{"golang", "programming"},
		},
		{
			name:     "Hashtag with trailing punctuation",
			content:  "Check out #coding, #dev.",
			expected: []string{"coding", "dev"},
		},
		{
			name:     "No hashtags",
			content:  "This is a post without hashtags",
			expected: []string{},
		},
		{
			name:     "Mixed case",
			content:  "#GoLang is #Awesome",
			expected: []string{"GoLang", "Awesome"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractHashtagsFromContent(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}
