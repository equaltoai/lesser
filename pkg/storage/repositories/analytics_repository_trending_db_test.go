package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// ============================================================================
// RecordHashtagUsage Tests
// ============================================================================

func TestRecordHashtagUsage_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	hashtag := "golang"
	statusID := "status-123"
	authorID := "author-456"

	// First Create call for HashtagUsage
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagUsage")).Return(mockQuery)
	mockQuery.On("Create").Return(nil).Once()

	// Then updateHashtagTrendScore is called
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagUsage")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.HashtagUsage")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]models.HashtagUsage)
		*records = []models.HashtagUsage{
			{AuthorID: "author-1", UsedAt: time.Now()},
			{AuthorID: "author-2", UsedAt: time.Now()},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	// Create trend item
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagTrend")).Return(mockQuery)
	mockQuery.On("CreateOrUpdate").Return(nil).Once()

	err := repo.RecordHashtagUsage(ctx, hashtag, statusID, authorID)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
}

func TestRecordHashtagUsage_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagUsage")).Return(mockQuery)
	mockQuery.On("Create").Return(ErrTestMockError)

	err := repo.RecordHashtagUsage(ctx, "test", "status-1", "author-1")
	require.Error(t, err)

	mockDB.AssertExpectations(t)
}

// ============================================================================
// RecordStatusEngagement Tests
// ============================================================================

func TestRecordStatusEngagement_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	statusID := "status-123"
	engagementType := "like"
	userID := "user-456"

	// Create engagement
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.StatusEngagement")).Return(mockQuery)
	mockQuery.On("Create").Return(nil).Once()

	// updateStatusTrendScore
	mockDB.On("Model", mock.AnythingOfType("*models.StatusEngagement")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.StatusEngagement")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]models.StatusEngagement)
		*records = []models.StatusEngagement{
			{UserID: "user-1", EngagementType: "like"},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	// Create trend item
	mockDB.On("Model", mock.AnythingOfType("*models.StatusTrend")).Return(mockQuery)
	mockQuery.On("CreateOrUpdate").Return(nil).Once()

	err := repo.RecordStatusEngagement(ctx, statusID, engagementType, userID)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
}

func TestRecordStatusEngagement_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.StatusEngagement")).Return(mockQuery)
	mockQuery.On("Create").Return(ErrTestMockError)

	err := repo.RecordStatusEngagement(ctx, "status-1", "like", "user-1")
	require.Error(t, err)

	mockDB.AssertExpectations(t)
}

// ============================================================================
// RecordLinkShare Tests
// ============================================================================

func TestRecordLinkShare_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	linkURL := "https://example.com/article"
	statusID := "status-123"
	authorID := "author-456"

	// Create link share
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.LinkShare")).Return(mockQuery)
	mockQuery.On("Create").Return(nil).Once()

	// updateLinkTrendScore
	mockDB.On("Model", mock.AnythingOfType("*models.LinkShare")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.LinkShare")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]models.LinkShare)
		*records = []models.LinkShare{
			{AuthorID: "author-1", SharedAt: time.Now()},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	// Create trend item
	mockDB.On("Model", mock.AnythingOfType("*models.LinkTrend")).Return(mockQuery)
	mockQuery.On("CreateOrUpdate").Return(nil).Once()

	err := repo.RecordLinkShare(ctx, linkURL, statusID, authorID)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
}

func TestRecordLinkShare_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.LinkShare")).Return(mockQuery)
	mockQuery.On("Create").Return(ErrTestMockError)

	err := repo.RecordLinkShare(ctx, "https://example.com", "status-1", "author-1")
	require.Error(t, err)

	mockDB.AssertExpectations(t)
}

// ============================================================================
// storeTrendInternal Tests
// ============================================================================

func TestStoreHashtagTrendInternal_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	trend := &models.HashtagTrend{
		Name:       "golang",
		UsageCount: 100,
	}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagTrend")).Return(mockQuery)
	mockQuery.On("CreateOrUpdate").Return(nil)

	err := repo.storeHashtagTrendInternal(ctx, trend)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
}

func TestStoreHashtagTrendInternal_FromStorageType(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	// Use storage.TrendingHashtag instead of models.HashtagTrend
	trend := &storage.TrendingHashtag{
		Name:       "python",
		UsageCount: 50,
	}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagTrend")).Return(mockQuery)
	mockQuery.On("CreateOrUpdate").Return(nil)

	err := repo.storeHashtagTrendInternal(ctx, trend)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
}

func TestStoreHashtagTrendInternal_InvalidType(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	// Pass an invalid type
	err := repo.storeHashtagTrendInternal(ctx, "not a trend")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidHashtagTrendType)
}

func TestStoreStatusTrendInternal_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	trend := &models.StatusTrend{
		ID:          "status-123",
		Engagements: 50,
	}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.StatusTrend")).Return(mockQuery)
	mockQuery.On("CreateOrUpdate").Return(nil)

	err := repo.storeStatusTrendInternal(ctx, trend)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
}

func TestStoreStatusTrendInternal_FromStorageType(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	trend := &storage.TrendingStatus{
		ID:          "status-456",
		Engagements: 100,
	}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.StatusTrend")).Return(mockQuery)
	mockQuery.On("CreateOrUpdate").Return(nil)

	err := repo.storeStatusTrendInternal(ctx, trend)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
}

func TestStoreStatusTrendInternal_InvalidType(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	err := repo.storeStatusTrendInternal(ctx, 12345)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidStatusTrendType)
}

func TestStoreLinkTrendInternal_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	trend := &models.LinkTrend{
		URL:        "https://example.com",
		ShareCount: 25,
	}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.LinkTrend")).Return(mockQuery)
	mockQuery.On("CreateOrUpdate").Return(nil)

	err := repo.storeLinkTrendInternal(ctx, trend)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
}

func TestStoreLinkTrendInternal_FromStorageType(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	trend := &storage.TrendingLink{
		URL:        "https://news.example.com",
		ShareCount: 75,
	}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.LinkTrend")).Return(mockQuery)
	mockQuery.On("CreateOrUpdate").Return(nil)

	err := repo.storeLinkTrendInternal(ctx, trend)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
}

func TestStoreLinkTrendInternal_InvalidType(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	err := repo.storeLinkTrendInternal(ctx, struct{}{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidLinkTrendType)
}

// ============================================================================
// getTrendingLinksInternal Tests
// ============================================================================

func TestGetTrendingLinksInternal_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.LinkTrend")).Return(mockQuery)
	mockQuery.On("Where", "gsi8PK", "=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi8SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 10).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.LinkTrend")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]models.LinkTrend)
		*records = []models.LinkTrend{
			{URL: "https://example.com", Title: "Example", ShareCount: 100},
			{URL: "https://news.com", Title: "News", ShareCount: 50},
		}
	}).Return(nil)

	result, err := repo.getTrendingLinksInternal(ctx, "LINK", 10)
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "https://example.com", result[0].URL)
	assert.Equal(t, int64(100), result[0].ShareCount)

	mockDB.AssertExpectations(t)
}

func TestGetTrendingLinksInternal_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.LinkTrend")).Return(mockQuery)
	mockQuery.On("Where", "gsi8PK", "=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi8SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 5).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.LinkTrend")).Return(errors.ErrItemNotFound)

	result, err := repo.getTrendingLinksInternal(ctx, "LINK", 5)
	require.NoError(t, err)
	assert.Empty(t, result)

	mockDB.AssertExpectations(t)
}

func TestGetTrendingLinksInternal_Error(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.LinkTrend")).Return(mockQuery)
	mockQuery.On("Where", "gsi8PK", "=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi8SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 10).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.LinkTrend")).Return(ErrTestMockError)

	result, err := repo.getTrendingLinksInternal(ctx, "LINK", 10)
	require.Error(t, err)
	assert.Nil(t, result)

	mockDB.AssertExpectations(t)
}

// ============================================================================
// GetTrendingHashtags/Statuses/Links wrapper tests
// ============================================================================

func TestGetTrendingHashtags_DelegatesToInternal(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagTrend")).Return(mockQuery)
	mockQuery.On("Where", "gsi8PK", "=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi8SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 10).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		// Return empty result
	}).Return(errors.ErrItemNotFound)

	result, err := repo.GetTrendingHashtags(ctx, time.Now(), 10)
	require.NoError(t, err)
	assert.Empty(t, result)

	mockDB.AssertExpectations(t)
}

func TestGetTrendingStatuses_DelegatesToInternal(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.StatusTrend")).Return(mockQuery)
	mockQuery.On("Where", "gsi8PK", "=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi8SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 5).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(errors.ErrItemNotFound)

	result, err := repo.GetTrendingStatuses(ctx, time.Now(), 5)
	require.NoError(t, err)
	assert.Empty(t, result)

	mockDB.AssertExpectations(t)
}

func TestGetTrendingLinks_DelegatesToInternal(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.LinkTrend")).Return(mockQuery)
	mockQuery.On("Where", "gsi8PK", "=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi8SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 20).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.LinkTrend")).Return(errors.ErrItemNotFound)

	result, err := repo.GetTrendingLinks(ctx, time.Now(), 20)
	require.NoError(t, err)
	assert.Empty(t, result)

	mockDB.AssertExpectations(t)
}

// ============================================================================
// StoreHashtagTrend/StatusTrend/LinkTrend public wrapper tests
// ============================================================================

func TestStoreHashtagTrend_CallsInternal(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	trend := &models.HashtagTrend{Name: "test"}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagTrend")).Return(mockQuery)
	mockQuery.On("CreateOrUpdate").Return(nil)

	err := repo.StoreHashtagTrend(ctx, trend)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
}

func TestStoreStatusTrend_CallsInternal(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	trend := &models.StatusTrend{ID: "test-123"}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.StatusTrend")).Return(mockQuery)
	mockQuery.On("CreateOrUpdate").Return(nil)

	err := repo.StoreStatusTrend(ctx, trend)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
}

func TestStoreLinkTrend_CallsInternal(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	trend := &models.LinkTrend{URL: "https://example.com"}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.LinkTrend")).Return(mockQuery)
	mockQuery.On("CreateOrUpdate").Return(nil)

	err := repo.StoreLinkTrend(ctx, trend)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
}

// ============================================================================
// GetRecentHashtags Tests
// ============================================================================

func TestGetRecentHashtags_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	since := time.Now().Add(-24 * time.Hour)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Hashtag")).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("Where", "LastUsed", ">=", since.Format(time.RFC3339)).Return(mockQuery)
	mockQuery.On("OrderBy", "LastUsed", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 20).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.Hashtag")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]*models.Hashtag)
		*records = []*models.Hashtag{
			{Name: "golang", UsageCount: 100},
			{Name: "python", UsageCount: 50},
		}
	}).Return(nil)

	result, err := repo.GetRecentHashtags(ctx, since, 20)
	require.NoError(t, err)
	assert.Len(t, result, 2)

	mockDB.AssertExpectations(t)
}

func TestGetRecentHashtags_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	since := time.Now().Add(-1 * time.Hour)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Hashtag")).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("Where", "LastUsed", ">=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("OrderBy", "LastUsed", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 10).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.Hashtag")).Return(errors.ErrItemNotFound)

	result, err := repo.GetRecentHashtags(ctx, since, 10)
	require.NoError(t, err)
	assert.Empty(t, result)

	mockDB.AssertExpectations(t)
}

func TestGetRecentHashtags_InvalidLimit(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	since := time.Now().Add(-24 * time.Hour)

	// When limit > 100, it should be clamped to default (20)
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Hashtag")).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("Where", "LastUsed", ">=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("OrderBy", "LastUsed", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 20).Return(mockQuery) // Clamped to default
	mockQuery.On("All", mock.AnythingOfType("*[]*models.Hashtag")).Return(nil)

	_, err := repo.GetRecentHashtags(ctx, since, 200)
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
}

// ============================================================================
// GetRecentStatusesWithEngagement Tests
// ============================================================================

func TestGetRecentStatusesWithEngagement_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	since := time.Now().Add(-24 * time.Hour)
	limit := 3 // Use a smaller limit that matches our test data

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.StatusEngagement")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "ENGAGEMENTS#ALL").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", ">=", since.Format(time.RFC3339)).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 30).Return(mockQuery) // 3 * 10 = 30
	mockQuery.On("All", mock.AnythingOfType("*[]models.StatusEngagement")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]models.StatusEngagement)
		*records = []models.StatusEngagement{
			{StatusID: "status-1", EngagementType: "like", UserID: "user-1"},
			{StatusID: "status-1", EngagementType: "boost", UserID: "user-2"},
			{StatusID: "status-2", EngagementType: "reply", UserID: "user-3"},
			{StatusID: "status-3", EngagementType: "like", UserID: "user-4"},
		}
	}).Return(nil)

	result, err := repo.GetRecentStatusesWithEngagement(ctx, since, limit)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(result), limit)

	mockDB.AssertExpectations(t)
}

func TestGetRecentStatusesWithEngagement_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	since := time.Now().Add(-1 * time.Hour)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.StatusEngagement")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "ENGAGEMENTS#ALL").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", ">=", since.Format(time.RFC3339)).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", mock.AnythingOfType("int")).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.StatusEngagement")).Return(errors.ErrItemNotFound)

	result, err := repo.GetRecentStatusesWithEngagement(ctx, since, 10)
	require.NoError(t, err)
	assert.Empty(t, result)

	mockDB.AssertExpectations(t)
}

// ============================================================================
// GetRecentLinks Tests
// ============================================================================

func TestGetRecentLinks_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	since := time.Now().Add(-24 * time.Hour)
	limit := 2 // Use small limit that matches our test data

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.LinkShare")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "LINK_SHARES#ALL").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", ">=", since.Format(time.RFC3339)).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 10).Return(mockQuery) // 2 * 5 = 10
	mockQuery.On("All", mock.AnythingOfType("*[]models.LinkShare")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]models.LinkShare)
		*records = []models.LinkShare{
			{URL: "https://example.com/a", AuthorID: "author-1", SharedAt: time.Now()},
			{URL: "https://example.com/a", AuthorID: "author-2", SharedAt: time.Now()},
			{URL: "https://example.com/b", AuthorID: "author-3", SharedAt: time.Now()},
		}
	}).Return(nil)

	result, err := repo.GetRecentLinks(ctx, since, limit)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(result), limit)

	mockDB.AssertExpectations(t)
}

func TestGetRecentLinks_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)
	ctx := context.Background()

	since := time.Now().Add(-1 * time.Hour)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.LinkShare")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "LINK_SHARES#ALL").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", ">=", since.Format(time.RFC3339)).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", mock.AnythingOfType("int")).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.LinkShare")).Return(errors.ErrItemNotFound)

	result, err := repo.GetRecentLinks(ctx, since, 5)
	require.NoError(t, err)
	assert.Empty(t, result)

	mockDB.AssertExpectations(t)
}

// ============================================================================
// SetStatusRepository Test
// ============================================================================

func TestSetStatusRepository(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewTrendingRepository(mockDB, zap.NewNop(), nil)

	// Should not panic
	repo.SetStatusRepository(nil)
	repo.SetStatusRepository("some interface")
	repo.SetStatusRepository(struct{}{})
}

// ============================================================================
// NewTrendingRepository Tests
// ============================================================================

func TestNewTrendingRepository(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()

	repo := NewTrendingRepository(mockDB, logger, nil)

	require.NotNil(t, repo)
	assert.NotNil(t, repo.db)
	assert.NotNil(t, repo.logger)
	// Domain should be set (either from config or default)
	assert.NotEmpty(t, repo.domain)
}
