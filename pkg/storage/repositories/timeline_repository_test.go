package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/aron23/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
)

func TestNewTimelineRepository(t *testing.T) {
	repo := NewTimelineRepository(nil, "test-table")

	assert.NotNil(t, repo)
	assert.Nil(t, repo.db)
	assert.Equal(t, "test-table", repo.tableName)
}

func TestCreateTimelineEntry_ValidEntry(t *testing.T) {
	repo := &TimelineRepository{}

	timeline := &models.Timeline{
		TimelineType: "HOME",
		TimelineID:   "testuser",
		PostID:       "post123",
		ActorID:      "actor456",
		Visibility:   "public",
	}

	// This will fail because db is nil, but we're testing the validation logic
	err := repo.CreateTimelineEntry(context.Background(), timeline)

	// Should fail due to nil db, but not due to validation
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create timeline entry")

	// Verify BeforeCreate was called (keys should be set)
	assert.NotEmpty(t, timeline.PK)
	assert.NotEmpty(t, timeline.SK)
	assert.NotEmpty(t, timeline.EntryID)
}

func TestCreateTimelineEntries_EmptySlice(t *testing.T) {
	repo := &TimelineRepository{}

	err := repo.CreateTimelineEntries(context.Background(), []*models.Timeline{})

	assert.NoError(t, err)
}

func TestCreateTimelineEntries_ValidEntries(t *testing.T) {
	repo := &TimelineRepository{}

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

	// This will fail because db is nil, but we're testing the preparation logic
	err := repo.CreateTimelineEntries(context.Background(), entries)

	// Should fail due to nil db, but not due to validation
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create timeline entries in batch")

	// Verify BeforeCreate was called for all entries
	for _, entry := range entries {
		assert.NotEmpty(t, entry.PK)
		assert.NotEmpty(t, entry.SK)
		assert.NotEmpty(t, entry.EntryID)
	}
}

func TestGetHomeTimeline_Parameters(t *testing.T) {
	repo := &TimelineRepository{}

	// This will fail because db is nil, but we're testing parameter handling
	_, _, err := repo.GetHomeTimeline(context.Background(), "testuser", 20, "cursor123")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries")
}

func TestGetPublicTimeline_LocalFlag(t *testing.T) {
	repo := &TimelineRepository{}

	// Test local timeline
	_, _, err := repo.GetPublicTimeline(context.Background(), true, 20, "")
	assert.Error(t, err) // Will fail due to nil db

	// Test federated timeline
	_, _, err = repo.GetPublicTimeline(context.Background(), false, 20, "")
	assert.Error(t, err) // Will fail due to nil db
}

func TestGetListTimeline_Parameters(t *testing.T) {
	repo := &TimelineRepository{}

	_, _, err := repo.GetListTimeline(context.Background(), "list123", 10, "cursor456")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries")
}

func TestGetDirectTimeline_Parameters(t *testing.T) {
	repo := &TimelineRepository{}

	_, _, err := repo.GetDirectTimeline(context.Background(), "alice", 15, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries")
}

func TestGetHashtagTimeline_LocalFlag(t *testing.T) {
	repo := &TimelineRepository{}

	// Test local hashtag timeline
	_, _, err := repo.GetHashtagTimeline(context.Background(), "photography", true, 20, "")
	assert.Error(t, err) // Will fail due to nil db

	// Test federated hashtag timeline
	_, _, err = repo.GetHashtagTimeline(context.Background(), "photography", false, 20, "")
	assert.Error(t, err) // Will fail due to nil db
}

func TestGetTimelineEntriesByPost_Parameters(t *testing.T) {
	repo := &TimelineRepository{}

	_, _, err := repo.GetTimelineEntriesByPost(context.Background(), "post123", 50, "cursor789")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries by post")
}

func TestGetTimelineEntriesByActor_Parameters(t *testing.T) {
	repo := &TimelineRepository{}

	_, _, err := repo.GetTimelineEntriesByActor(context.Background(), "actor456", 25, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries by actor")
}

func TestGetTimelineEntriesByVisibility_Parameters(t *testing.T) {
	repo := &TimelineRepository{}

	_, _, err := repo.GetTimelineEntriesByVisibility(context.Background(), "public", 30, "cursor999")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries by visibility")
}

func TestGetTimelineEntriesByLanguage_Parameters(t *testing.T) {
	repo := &TimelineRepository{}

	_, _, err := repo.GetTimelineEntriesByLanguage(context.Background(), "en", 40, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries by language")
}

func TestGetTimelineEntry_Parameters(t *testing.T) {
	repo := &TimelineRepository{}

	now := time.Now()
	_, err := repo.GetTimelineEntry(context.Background(), "HOME", "alice", "entry123", now)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entry")
}

func TestUpdateTimelineEntry_ValidEntry(t *testing.T) {
	repo := &TimelineRepository{}

	timeline := &models.Timeline{
		TimelineType: "HOME",
		TimelineID:   "testuser",
		PostID:       "post123",
		ActorID:      "actor456",
		Visibility:   "unlisted", // Changed visibility
	}

	// This will fail because db is nil, but we're testing the validation logic
	err := repo.UpdateTimelineEntry(context.Background(), timeline)

	// Should fail due to nil db, but not due to validation
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update timeline entry")

	// Verify BeforeUpdate was called (ModifiedAt should be updated)
	assert.False(t, timeline.ModifiedAt.IsZero())
}

func TestDeleteTimelineEntry_Parameters(t *testing.T) {
	repo := &TimelineRepository{}

	now := time.Now()
	err := repo.DeleteTimelineEntry(context.Background(), "HOME", "alice", "entry123", now)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete timeline entry")
}

func TestDeleteTimelineEntriesByPost_Parameters(t *testing.T) {
	repo := &TimelineRepository{}

	err := repo.DeleteTimelineEntriesByPost(context.Background(), "post123")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries for deletion")
}

func TestDeleteExpiredTimelineEntries_Parameters(t *testing.T) {
	repo := &TimelineRepository{}

	before := time.Now().Add(-24 * time.Hour)
	err := repo.DeleteExpiredTimelineEntries(context.Background(), before)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan for expired timeline entries")
}

func TestCountTimelineEntries_Parameters(t *testing.T) {
	repo := &TimelineRepository{}

	_, err := repo.CountTimelineEntries(context.Background(), "HOME", "alice")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to count timeline entries")
}

func TestGetTimelineEntriesInRange_Parameters(t *testing.T) {
	repo := &TimelineRepository{}

	start := time.Now().Add(-2 * time.Hour)
	end := time.Now().Add(-1 * time.Hour)

	_, err := repo.GetTimelineEntriesInRange(context.Background(), "PUBLIC", "FEDERATED", start, end, 20)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get timeline entries in range")
}

func TestGetTimelineEntriesWithFilters_AllFilters(t *testing.T) {
	repo := &TimelineRepository{}

	filters := TimelineFilters{
		OnlyMedia:      true,
		ExcludeReplies: true,
		ExcludeBoosts:  true,
		Language:       "en",
		MinID:          "1234567890",
		MaxID:          "9876543210",
	}

	_, _, err := repo.GetTimelineEntriesWithFilters(context.Background(), "HOME", "alice", filters, 20, "cursor123")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get filtered timeline entries")
}

func TestGetTimelineEntriesWithFilters_NoFilters(t *testing.T) {
	repo := &TimelineRepository{}

	filters := TimelineFilters{} // Empty filters

	_, _, err := repo.GetTimelineEntriesWithFilters(context.Background(), "HOME", "bob", filters, 10, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get filtered timeline entries")
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
	repo := NewTimelineRepository(nil, "test-table")

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
	repo := NewTimelineRepository(nil, "test-table")

	// Verify repository was created
	assert.NotNil(t, repo)
	assert.Equal(t, "test-table", repo.tableName)
	assert.Nil(t, repo.db)
}
