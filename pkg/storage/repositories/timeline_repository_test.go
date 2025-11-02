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
	repo := NewTimelineRepository(nil, "test-table", logger, nil)

	assert.NotNil(t, repo)
	assert.Nil(t, repo.db)
	assert.Equal(t, "test-table", repo.tableName)
	assert.NotNil(t, repo.logger)
}

func expectWithContext(db *mocks.MockDB) {
	db.On("WithContext", mock.Anything).Return(db).Maybe()
}

func allowModel(db *mocks.MockDB, query *mocks.MockQuery) {
	db.On("Model", mock.Anything).Return(query).Maybe()
}

func TestGetConversations(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	username := "testuser"
	limit := 10
	cursor := ""

	// Set up test conversations
	testConv1 := &models.Conversation{
		ID:           "conv1",
		Participants: []string{username, "user2"},
		UpdatedAt:    time.Now(),
	}
	testConv2 := &models.Conversation{
		ID:           "conv2",
		Participants: []string{username, "user3"},
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}

	// Create participant records with conversations
	testParticipantRecords := []*models.ConversationParticipantRecord{
		{
			PK:           fmt.Sprintf("USER_CONVERSATIONS#%s", username),
			SK:           fmt.Sprintf("%d#conv1", time.Now().Unix()),
			Conversation: testConv1,
		},
		{
			PK:           fmt.Sprintf("USER_CONVERSATIONS#%s", username),
			SK:           fmt.Sprintf("%d#conv2", time.Now().Add(-1*time.Hour).Unix()),
			Conversation: testConv2,
		},
	}

	// Set up expectations
	expectWithContext(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ConversationParticipantRecord")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", fmt.Sprintf("USER_CONVERSATIONS#%s", username)).Return(mockQuery)
	mockQuery.On("OrderBy", "SK", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", limit+1).Return(mockQuery) // limit + 1 for pagination check
	mockQuery.On("All", mock.AnythingOfType("*[]*models.ConversationParticipantRecord")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]*models.ConversationParticipantRecord)
		*records = testParticipantRecords
	}).Return(nil)

	// Execute
	conversations, nextCursor, err := repo.GetConversations(ctx, username, limit, cursor)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, conversations, 2)
	assert.Equal(t, "conv1", conversations[0].ID)
	assert.Equal(t, "conv2", conversations[1].ID)
	assert.Empty(t, nextCursor) // No next cursor since we have fewer than limit+1 results

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestCreateTimelineEntry_ValidEntry(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	timeline := &models.Timeline{
		TimelineType: "HOME",
		TimelineID:   "testuser",
		PostID:       "post123",
		ActorID:      "actor456",
		Visibility:   "public",
	}

	// Set up expectations
	expectWithContext(mockDB)
	mockDB.On("Model", timeline).Return(mockQuery)
	mockQuery.On("Create").Return(nil).Once()

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
	expectWithContext(mockDB)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	err := repo.CreateTimelineEntries(context.Background(), []*models.Timeline{})

	assert.NoError(t, err)
}

func TestCreateTimelineEntries_ValidEntries(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

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
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
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

//nolint:dupl // test setup patterns are similar for different timeline types
func TestGetHomeTimeline_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	allowModel(mockDB, mockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	// Set up expectations
	mockQuery.On("Where", "PK", "=", "TIMELINE#HOME#testuser").Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery)
	mockQuery.On("Where", "SK", ">", "cursor123").Return(mockQuery)
	mockQuery.On("Limit", 21).Return(mockQuery) // limit + 1 for pagination
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError)

	// Execute
	_, _, err := repo.GetHomeTimeline(context.Background(), "testuser", 20, "cursor123")

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to query timeline entry (timeline entries)")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetPublicTimeline_LocalFlag(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	allowModel(mockDB, mockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	// Set up expectations for local timeline
	mockQuery.On("Index", "post-timeline-index").Return(mockQuery).Once()
	mockQuery.On("Where", "GSI1PK", "=", "TIMELINE#PUBLIC#LOCAL").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "GSI1SK", "ASC").Return(mockQuery).Once()
	mockQuery.On("Limit", 21).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError).Once()

	// Test local timeline
	_, _, err := repo.GetPublicTimeline(context.Background(), true, 20, "")
	assert.Error(t, err)

	// Set up expectations for federated timeline
	mockQuery.On("Index", "post-timeline-index").Return(mockQuery).Once()
	mockQuery.On("Where", "GSI1PK", "=", "TIMELINE#PUBLIC#FEDERATED").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "GSI1SK", "ASC").Return(mockQuery).Once()
	mockQuery.On("Limit", 21).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError).Once()

	// Test federated timeline
	_, _, err = repo.GetPublicTimeline(context.Background(), false, 20, "")
	assert.Error(t, err)

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

//nolint:dupl // test setup patterns are similar for different timeline types
func TestGetListTimeline_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	allowModel(mockDB, mockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	// Set up expectations
	mockQuery.On("Where", "PK", "=", "TIMELINE#LIST#list123").Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery)
	mockQuery.On("Where", "SK", ">", "cursor456").Return(mockQuery)
	mockQuery.On("Limit", 11).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError)

	_, _, err := repo.GetListTimeline(context.Background(), "list123", 10, "cursor456")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to query timeline entry (timeline entries)")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetDirectTimeline_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	allowModel(mockDB, mockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	// Set up expectations
	mockQuery.On("Where", "PK", "=", "TIMELINE#DIRECT#alice").Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery)
	mockQuery.On("Limit", 16).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError)

	_, _, err := repo.GetDirectTimeline(context.Background(), "alice", 15, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to query timeline entry (timeline entries)")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetHashtagTimeline_LocalFlag(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	allowModel(mockDB, mockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	// Set up expectations for local hashtag timeline
	mockQuery.On("Where", "PK", "=", "TIMELINE#HASHTAG#photography#LOCAL").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery).Once()
	mockQuery.On("Limit", 21).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError).Once()

	// Test local hashtag timeline
	_, _, err := repo.GetHashtagTimeline(context.Background(), "photography", true, 20, "")
	assert.Error(t, err)

	// Set up expectations for federated hashtag timeline
	mockQuery.On("Where", "PK", "=", "TIMELINE#HASHTAG#photography").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery).Once()
	mockQuery.On("Limit", 21).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError).Once()

	// Test federated hashtag timeline
	_, _, err = repo.GetHashtagTimeline(context.Background(), "photography", false, 20, "")
	assert.Error(t, err)

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

//nolint:dupl // test setup patterns are similar for different timeline query types
func TestGetTimelineEntriesByPost_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	// Set up expectations
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Index", "post-timeline-index").Return(mockQuery)
	mockQuery.On("Where", "GSI1PK", "=", "POST#post123").Return(mockQuery)
	mockQuery.On("OrderBy", "GSI1SK", "ASC").Return(mockQuery)
	mockQuery.On("Where", "GSI1SK", ">", "cursor789").Return(mockQuery)
	mockQuery.On("Limit", 51).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError)

	_, _, err := repo.GetTimelineEntriesByPost(context.Background(), "post123", 50, "cursor789")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to query timeline entry (post)")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetTimelineEntriesByActor_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	// Set up expectations
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Index", "actor-timeline-index").Return(mockQuery)
	mockQuery.On("Where", "GSI2PK", "=", "ACTOR#actor456").Return(mockQuery)
	mockQuery.On("OrderBy", "GSI2SK", "ASC").Return(mockQuery)
	mockQuery.On("Limit", 26).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError)

	_, _, err := repo.GetTimelineEntriesByActor(context.Background(), "actor456", 25, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to query timeline entry (actor)")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

//nolint:dupl // test setup patterns are similar for different timeline query types
func TestGetTimelineEntriesByVisibility_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	// Set up expectations - assuming it uses GSI3 for visibility
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Index", "visibility-timeline-index").Return(mockQuery)
	mockQuery.On("Where", "GSI3PK", "=", "VISIBILITY#public").Return(mockQuery)
	mockQuery.On("OrderBy", "GSI3SK", "ASC").Return(mockQuery)
	mockQuery.On("Where", "GSI3SK", ">", "cursor999").Return(mockQuery)
	mockQuery.On("Limit", 31).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError)

	_, _, err := repo.GetTimelineEntriesByVisibility(context.Background(), "public", 30, "cursor999")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to query timeline entry (visibility)")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetTimelineEntriesByLanguage_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	// Set up expectations - assuming it uses GSI4 for language
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Index", "language-timeline-index").Return(mockQuery)
	mockQuery.On("Where", "GSI4PK", "=", "LANGUAGE#en").Return(mockQuery)
	mockQuery.On("OrderBy", "GSI4SK", "ASC").Return(mockQuery)
	mockQuery.On("Limit", 41).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError)

	_, _, err := repo.GetTimelineEntriesByLanguage(context.Background(), "en", 40, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to query timeline entry (language)")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetTimelineEntry_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	allowModel(mockDB, mockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	now := time.Now()

	// Set up expectations
	mockQuery.On("Where", "PK", "=", "TIMELINE#HOME#alice").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", mock.Anything).Return(mockQuery) // SK contains timestamp
	mockQuery.On("First", mock.Anything).Return(ErrTestMockError)

	_, err := repo.GetTimelineEntry(context.Background(), "HOME", "alice", "entry123", now)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to retrieve timeline entry")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestUpdateTimelineEntry_ValidEntry(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	timeline := &models.Timeline{
		TimelineType: "HOME",
		TimelineID:   "testuser",
		PostID:       "post123",
		ActorID:      "actor456",
		Visibility:   "unlisted", // Changed visibility
		PK:           "TIMELINE#HOME#testuser",
		SK:           "2024-01-01T00:00:00Z#entry123",
	}

	// Set up expectations
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
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
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	now := time.Now()
	pk := "TIMELINE#HOME#alice"
	reverseTimestamp := 9999999999 - now.Unix()
	sk := fmt.Sprintf("%010d#entry123", reverseTimestamp)

	// Set up expectations
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", pk).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", sk).Return(mockQuery)
	mockQuery.On("Delete").Return(nil)

	err := repo.DeleteTimelineEntry(context.Background(), "HOME", "alice", "entry123", now)

	assert.NoError(t, err)

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestDeleteTimelineEntriesByPost_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	// Set up expectations for query (matches GetTimelineEntriesByPost)
	mockDB.On("Model", &models.Timeline{}).Return(mockQuery)
	mockQuery.On("Index", "post-timeline-index").Return(mockQuery)
	mockQuery.On("Where", "GSI1PK", "=", "POST#post123").Return(mockQuery)
	mockQuery.On("OrderBy", "GSI1SK", "ASC").Return(mockQuery)
	mockQuery.On("Limit", 1001).Return(mockQuery) // 1000 + 1
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError)

	err := repo.DeleteTimelineEntriesByPost(context.Background(), "post123")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to query timeline entry (deletion query)")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestDeleteExpiredTimelineEntries_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	before := time.Now().Add(-24 * time.Hour)

	err := repo.DeleteExpiredTimelineEntries(context.Background(), before)

	assert.NoError(t, err)
	mockDB.AssertNotCalled(t, "Model", mock.Anything)
}

func TestCountTimelineEntries_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	allowModel(mockDB, mockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	// Set up expectations
	mockQuery.On("Where", "PK", "=", "TIMELINE#HOME#alice").Return(mockQuery)
	mockQuery.On("Count").Return(int64(0), ErrTestMockError)

	_, err := repo.CountTimelineEntries(context.Background(), "HOME", "alice")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to query base entity (count query)")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetTimelineEntriesInRange_Parameters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	allowModel(mockDB, mockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	start := time.Now().Add(-2 * time.Hour)
	end := time.Now().Add(-1 * time.Hour)

	// Set up expectations
	mockQuery.On("Where", "PK", "=", "TIMELINE#PUBLIC#FEDERATED").Return(mockQuery)
	mockQuery.On("Where", "SK", ">=", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "SK", "<=", mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery)
	mockQuery.On("Limit", 21).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError)

	_, err := repo.GetTimelineEntriesInRange(context.Background(), "PUBLIC", "FEDERATED", start, end, 20)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to query timeline entry (range query)")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetTimelineEntriesWithFilters_AllFilters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	allowModel(mockDB, mockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	filters := TimelineFilters{
		OnlyMedia:      true,
		ExcludeReplies: true,
		ExcludeBoosts:  true,
		Language:       "en",
		MinID:          "1234567890",
		MaxID:          "9876543210",
	}

	// Set up expectations
	mockQuery.On("Where", "PK", "=", "TIMELINE#HOME#alice").Return(mockQuery)
	mockQuery.On("Filter", "HasMedia", "=", true).Return(mockQuery).Maybe()
	mockQuery.On("Filter", "IsReply", "=", false).Return(mockQuery).Maybe()
	mockQuery.On("Filter", "IsBoost", "=", false).Return(mockQuery).Maybe()
	mockQuery.On("Filter", "Language", "=", "en").Return(mockQuery)
	mockQuery.On("Filter", "TimelineAt", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", 21).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError)

	_, _, err := repo.GetTimelineEntriesWithFilters(context.Background(), "HOME", "alice", filters, 20, "cursor123")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to query timeline entry (filtered)")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetTimelineEntriesWithFilters_NoFilters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	expectWithContext(mockDB)
	mockQuery := new(mocks.MockQuery)
	allowModel(mockDB, mockQuery)
	logger := zap.NewNop()
	repo := NewTimelineRepository(mockDB, "test-table", logger, nil)

	filters := TimelineFilters{} // Empty filters

	// Set up expectations - no filter calls when filters are empty
	mockQuery.On("Where", "PK", "=", "TIMELINE#HOME#bob").Return(mockQuery)
	mockQuery.On("Limit", 11).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError)

	_, _, err := repo.GetTimelineEntriesWithFilters(context.Background(), "HOME", "bob", filters, 10, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to query timeline entry (filtered)")

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
	repo := NewTimelineRepository(nil, "test-table", logger, nil)

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
	repo := NewTimelineRepository(nil, "test-table", logger, nil)

	// Verify repository was created
	assert.NotNil(t, repo)
	assert.Equal(t, "test-table", repo.tableName)
	assert.Nil(t, repo.db)
}
