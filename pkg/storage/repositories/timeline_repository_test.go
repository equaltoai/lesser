package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestNewTimelineRepository(t *testing.T) {
	logger := zap.NewNop()
	repo := NewTimelineRepository(nil, "test-table", logger)

	assert.NotNil(t, repo)
	assert.Nil(t, repo.db)
	assert.Equal(t, "test-table", repo.tableName)
	assert.NotNil(t, repo.logger)
}

func TestCreateTimelineEntry_ValidEntry(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	timeline := &models.Timeline{
		TimelineType: "HOME",
		TimelineID:   "testuser",
		PostID:       "post123",
		ActorID:      "actor456",
		Visibility:   "public",
	}

	// Set up expectations
	mockDB.On("Model", timeline).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	// Execute
	err := repo.CreateTimelineEntry(context.Background(), timeline)

	// Assert
	assert.NoError(t, err)

	// Verify BeforeCreate was called (keys should be set)
	assert.NotEmpty(t, timeline.PK)
	assert.NotEmpty(t, timeline.SK)
	assert.NotEmpty(t, timeline.EntryID)

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestCreateTimelineEntries_EmptySlice(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	err := repo.CreateTimelineEntries(context.Background(), []*models.Timeline{})

	assert.NoError(t, err)
}

func TestCreateTimelineEntries_ValidEntries(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	entries := []*models.Timeline{
		{
			TimelineType: "HOME",
			TimelineID:   "alice",
			PostID:       "post1",
			ActorID:      "actor1",
			Visibility:   "public",
		},
		{
			TimelineType: "HOME",
			TimelineID:   "bob",
			PostID:       "post2",
			ActorID:      "actor2",
			Visibility:   "unlisted",
		},
	}

	// Set up expectations for each entry creation
	for _, entry := range entries {
		mockDB.On("Model", entry).Return(mockQuery).Once()
		mockQuery.On("Create").Return(nil).Once()
	}

	// Execute
	err := repo.CreateTimelineEntries(context.Background(), entries)

	// Assert
	assert.NoError(t, err)

	// Verify BeforeCreate was called for all entries
	for _, entry := range entries {
		assert.NotEmpty(t, entry.PK)
		assert.NotEmpty(t, entry.SK)
		assert.NotEmpty(t, entry.EntryID)
	}

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetHomeTimeline_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	// Set up expectations
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "timeline#HOME#testuser").Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Where", "SK", "<", "cursor123").Return(mockQuery)
	mockQuery.On("Limit", 21).Return(mockQuery) // limit + 1 for pagination
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("mock error"))

	// Execute
	_, _, err := repo.GetHomeTimeline(context.Background(), "testuser", 20, "cursor123")

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetPublicTimeline_LocalFlag(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	// Set up expectations for local timeline
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "timeline#PUBLIC#LOCAL").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 21).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("mock error")).Once()

	// Test local timeline
	_, _, err := repo.GetPublicTimeline(context.Background(), true, 20, "")
	assert.Error(t, err)

	// Set up expectations for federated timeline
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "timeline#PUBLIC#FEDERATED").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 21).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("mock error")).Once()

	// Test federated timeline
	_, _, err = repo.GetPublicTimeline(context.Background(), false, 20, "")
	assert.Error(t, err)

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetListTimeline_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	// Set up expectations
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "timeline#LIST#list123").Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Where", "SK", "<", "cursor456").Return(mockQuery)
	mockQuery.On("Limit", 11).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("mock error"))

	_, _, err := repo.GetListTimeline(context.Background(), "list123", 10, "cursor456")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetDirectTimeline_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	// Set up expectations
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "timeline#DIRECT#alice").Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 16).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("mock error"))

	_, _, err := repo.GetDirectTimeline(context.Background(), "alice", 15, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetHashtagTimeline_LocalFlag(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	// Set up expectations for local hashtag timeline
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "timeline#HASHTAG#photography#LOCAL").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 21).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("mock error")).Once()

	// Test local hashtag timeline
	_, _, err := repo.GetHashtagTimeline(context.Background(), "photography", true, 20, "")
	assert.Error(t, err)

	// Set up expectations for federated hashtag timeline
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "timeline#HASHTAG#photography").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 21).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("mock error")).Once()

	// Test federated hashtag timeline
	_, _, err = repo.GetHashtagTimeline(context.Background(), "photography", false, 20, "")
	assert.Error(t, err)

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetTimelineEntriesByPost_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	// Set up expectations
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Index", "post-timeline-index").Return(mockQuery)
	mockQuery.On("Where", "GSI1PK", "=", "POST#post123").Return(mockQuery)
	mockQuery.On("OrderBy", "GSI1SK", "DESC").Return(mockQuery)
	mockQuery.On("Where", "GSI1SK", "<", "cursor789").Return(mockQuery)
	mockQuery.On("Limit", 51).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("mock error"))

	_, _, err := repo.GetTimelineEntriesByPost(context.Background(), "post123", 50, "cursor789")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries by post")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetTimelineEntriesByActor_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	// Set up expectations
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Index", "actor-timeline-index").Return(mockQuery)
	mockQuery.On("Where", "GSI2PK", "=", "ACTOR#actor456").Return(mockQuery)
	mockQuery.On("OrderBy", "GSI2SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 26).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("mock error"))

	_, _, err := repo.GetTimelineEntriesByActor(context.Background(), "actor456", 25, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries by actor")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetTimelineEntriesByVisibility_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	// Set up expectations - assuming it uses GSI3 for visibility
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Index", "visibility-timeline-index").Return(mockQuery)
	mockQuery.On("Where", "GSI3PK", "=", "VISIBILITY#public").Return(mockQuery)
	mockQuery.On("OrderBy", "GSI3SK", "DESC").Return(mockQuery)
	mockQuery.On("Where", "GSI3SK", "<", "cursor999").Return(mockQuery)
	mockQuery.On("Limit", 31).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("mock error"))

	_, _, err := repo.GetTimelineEntriesByVisibility(context.Background(), "public", 30, "cursor999")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries by visibility")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetTimelineEntriesByLanguage_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	// Set up expectations - assuming it uses GSI4 for language
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Index", "language-timeline-index").Return(mockQuery)
	mockQuery.On("Where", "GSI4PK", "=", "LANGUAGE#en").Return(mockQuery)
	mockQuery.On("OrderBy", "GSI4SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 41).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("mock error"))

	_, _, err := repo.GetTimelineEntriesByLanguage(context.Background(), "en", 40, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries by language")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetTimelineEntry_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	now := time.Now()

	// Set up expectations
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "timeline#HOME#alice").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", mock.Anything).Return(mockQuery) // SK contains timestamp
	mockQuery.On("First", mock.Anything).Return(fmt.Errorf("mock error"))

	_, err := repo.GetTimelineEntry(context.Background(), "HOME", "alice", "entry123", now)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entry")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestUpdateTimelineEntry_ValidEntry(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	timeline := &models.Timeline{
		TimelineType: "HOME",
		TimelineID:   "testuser",
		PostID:       "post123",
		ActorID:      "actor456",
		Visibility:   "unlisted", // Changed visibility
		PK:           "timeline#HOME#testuser",
		SK:           "2024-01-01T00:00:00Z#entry123",
	}

	// Set up expectations
	mockDB.On("Model", timeline).Return(mockQuery)
	mockQuery.On("Update", mock.Anything).Return(nil) // Update can take optional fields

	// Execute
	err := repo.UpdateTimelineEntry(context.Background(), timeline)

	// Assert
	assert.NoError(t, err)

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)

	// Verify BeforeUpdate was called (ModifiedAt should be updated)
	assert.False(t, timeline.ModifiedAt.IsZero())
}

func TestDeleteTimelineEntry_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	now := time.Now()
	pk := "timeline#HOME#alice"
	sk := fmt.Sprintf("%d#entry123", now.Unix())

	// Set up expectations
	mockDB.On("Model", &models.Timeline{PK: pk, SK: sk}).Return(mockQuery)
	mockQuery.On("Delete").Return(nil)

	err := repo.DeleteTimelineEntry(context.Background(), "HOME", "alice", "entry123", now)

	assert.NoError(t, err)

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestDeleteTimelineEntriesByPost_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	// Set up expectations for query (matches GetTimelineEntriesByPost)
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Index", "post-timeline-index").Return(mockQuery)
	mockQuery.On("Where", "GSI1PK", "=", "POST#post123").Return(mockQuery)
	mockQuery.On("OrderBy", "GSI1SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 1001).Return(mockQuery) // 1000 + 1
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("mock error"))

	err := repo.DeleteTimelineEntriesByPost(context.Background(), "post123")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries for deletion")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestDeleteExpiredTimelineEntries_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	before := time.Now().Add(-24 * time.Hour)

	// Set up expectations
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Filter", "ExpiresAt", "<", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("mock error"))

	err := repo.DeleteExpiredTimelineEntries(context.Background(), before)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan for expired timeline entries")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestCountTimelineEntries_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	// Set up expectations
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "timeline#HOME#alice").Return(mockQuery)
	mockQuery.On("Count").Return(int64(0), fmt.Errorf("mock error"))

	_, err := repo.CountTimelineEntries(context.Background(), "HOME", "alice")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to count timeline entries")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetTimelineEntriesInRange_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	start := time.Now().Add(-2 * time.Hour)
	end := time.Now().Add(-1 * time.Hour)

	// Set up expectations
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "timeline#PUBLIC#FEDERATED").Return(mockQuery)
	mockQuery.On("Where", "SK", ">=", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "SK", "<=", mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 20).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("mock error"))

	_, err := repo.GetTimelineEntriesInRange(context.Background(), "PUBLIC", "FEDERATED", start, end, 20)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries in range")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetTimelineEntriesWithFilters_AllFilters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	filters := TimelineFilters{
		OnlyMedia:      true,
		ExcludeReplies: true,
		ExcludeBoosts:  true,
		Language:       "en",
		MinID:          "1234567890",
		MaxID:          "9876543210",
	}

	// Set up expectations
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "timeline#HOME#alice").Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Filter", "HasMedia", "=", true).Return(mockQuery)
	mockQuery.On("Filter", "IsReply", "=", false).Return(mockQuery)
	mockQuery.On("Filter", "IsBoost", "=", false).Return(mockQuery)
	mockQuery.On("Filter", "Language", "=", "en").Return(mockQuery)
	mockQuery.On("Filter", "TimelineAt", ">=", mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", "TimelineAt", "<=", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "SK", "<", "cursor123").Return(mockQuery)
	mockQuery.On("Limit", 21).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("mock error"))

	_, _, err := repo.GetTimelineEntriesWithFilters(context.Background(), "HOME", "alice", filters, 20, "cursor123")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get filtered timeline entries")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetTimelineEntriesWithFilters_NoFilters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger)

	filters := TimelineFilters{} // Empty filters

	// Set up expectations - no filter calls when filters are empty
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "timeline#HOME#bob").Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 11).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("mock error"))

	_, _, err := repo.GetTimelineEntriesWithFilters(context.Background(), "HOME", "bob", filters, 10, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get filtered timeline entries")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestTimelineFilters_Struct(t *testing.T) {
	filters := TimelineFilters{
		OnlyMedia:      true,
		ExcludeReplies: false,
		ExcludeBoosts:  true,
		Language:       "fr",
		MinID:          "1000",
		MaxID:          "2000",
	}

	assert.True(t, filters.OnlyMedia)
	assert.False(t, filters.ExcludeReplies)
	assert.True(t, filters.ExcludeBoosts)
	assert.Equal(t, "fr", filters.Language)
	assert.Equal(t, "1000", filters.MinID)
	assert.Equal(t, "2000", filters.MaxID)
}

func TestTimelineRepository_MethodSignatures(t *testing.T) {
	// Test that all methods have the expected signatures
	logger := zap.NewNop()
	repo := NewTimelineRepository(nil, "test-table", logger)

	// Verify repository was created
	assert.NotNil(t, repo)
	assert.Equal(t, "test-table", repo.tableName)
	assert.Nil(t, repo.db)

	// Since db is nil, we can't actually call the methods
	// This test just verifies the repository was created correctly
}

func TestTimelineRepository_EdgeCases(t *testing.T) {
	// This test verifies the repository's behavior with edge cases
	// Since we can't call methods on a nil DB, we'll just verify the repository structure
	logger := zap.NewNop()
	repo := NewTimelineRepository(nil, "test-table", logger)

	// Verify repository was created
	assert.NotNil(t, repo)
	assert.Equal(t, "test-table", repo.tableName)
	assert.Nil(t, repo.db)
}
