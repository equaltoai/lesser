package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound09_TimelineRepository_CRUDAndQueries(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2025, 1, 17, 0, 0, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewTimelineRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	ctx := context.Background()

	entry := &models.Timeline{
		TimelineType: "HOME",
		TimelineID:   "user-1",
		EntryID:      "entry-1",
		TimelineAt:   baseTime,
		PostID:       "post-1",
		ActorID:      "https://example.com/users/user-1",
		Visibility:   "public",
		Language:     "en",
	}
	require.NoError(t, repo.CreateTimelineEntry(ctx, entry))

	require.NoError(t, repo.CreateTimelineEntries(ctx, []*models.Timeline{entry}))

	entries, cursor, err := repo.GetPublicTimeline(ctx, false, 1, "")
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	_ = cursor

	byPost, next, err := repo.GetTimelineEntriesByPost(ctx, "post-1", 1, "")
	require.NoError(t, err)
	require.NotEmpty(t, byPost)
	require.NotEmpty(t, next)

	byActor, _, err := repo.GetTimelineEntriesByActor(ctx, "actor-1", 1, "")
	require.NoError(t, err)
	require.NotEmpty(t, byActor)

	byVis, _, err := repo.GetTimelineEntriesByVisibility(ctx, "public", 1, "")
	require.NoError(t, err)
	require.NotEmpty(t, byVis)

	byLang, _, err := repo.GetTimelineEntriesByLanguage(ctx, "en", 1, "")
	require.NoError(t, err)
	require.NotEmpty(t, byLang)

	got, err := repo.GetTimelineEntry(ctx, "HOME", "user-1", "entry-1", baseTime)
	require.NoError(t, err)
	require.NotNil(t, got)

	require.NoError(t, repo.UpdateTimelineEntry(ctx, entry))
	require.NoError(t, repo.DeleteTimelineEntry(ctx, "HOME", "user-1", "entry-1", baseTime))

	require.NoError(t, repo.DeleteExpiredTimelineEntries(ctx, baseTime))

	count, err := repo.CountTimelineEntries(ctx, "HOME", "user-1")
	require.NoError(t, err)
	require.GreaterOrEqual(t, count, 0)

	mockDBErr := new(mocks.MockDB)
	mockQueryErr := new(mocks.MockQuery)
	mockQueryErr.On("Create").Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDBErr, mockQueryErr, nil, baseTime)

	repoErr := NewTimelineRepository(mockDBErr, "test-table", zap.NewNop(), nil)
	repoErr.SetValidationService(nil)
	repoErr.SetPermissionService(nil)
	repoErr.SetEventService(nil)
	repoErr.SetCachingService(nil)

	require.Error(t, repoErr.CreateTimelineEntry(ctx, entry))
}

func TestRound09_TimelineRepository_DeletionFiltersConversations(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2025, 1, 18, 0, 0, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewTimelineRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	ctx := context.Background()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr, ok := args.Get(0).(*[]*models.Timeline)
		if ok {
			a := &models.Timeline{TimelineType: "HOME", TimelineID: "user-1", EntryID: "e1", TimelineAt: baseTime, PostID: "post-1"}
			_ = a.BeforeCreate()
			b := &models.Timeline{TimelineType: "HOME", TimelineID: "user-1", EntryID: "e2", TimelineAt: baseTime.Add(1 * time.Minute), PostID: "post-1"}
			_ = b.BeforeCreate()
			*ptr = append(*ptr, a, b)
		}
	}).Return(nil).Once()

	require.NoError(t, repo.DeleteTimelineEntriesByPost(ctx, "post-1"))
	require.NoError(t, repo.RemoveFromTimelines(ctx, "post-1"))

	rangeEntries, err := repo.GetTimelineEntriesInRange(ctx, "HOME", "user-1", baseTime.Add(-1*time.Hour), baseTime.Add(1*time.Hour), 10)
	require.NoError(t, err)
	require.NotEmpty(t, rangeEntries)

	filters := interfaces.TimelineFilters{OnlyMedia: true, ExcludeReplies: true, ExcludeBoosts: true, Language: "en", MinID: "0", MaxID: "9999999999"}
	filtered, _, err := repo.GetTimelineEntriesWithFilters(ctx, "HOME", "user-1", filters, 10, "")
	require.NoError(t, err)
	require.NotNil(t, filtered)

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr, ok := args.Get(0).(*[]*models.UserConversationState)
		if !ok {
			return
		}
		*ptr = append(*ptr,
			&models.UserConversationState{
				ViewerID:       "user-1",
				ConversationID: "c1",
				CounterpartID:  "user-2",
				Folder:         models.UserConversationFolderInbox,
				SortAt:         baseTime,
				Unread:         true,
			},
			&models.UserConversationState{
				ViewerID:       "user-1",
				ConversationID: "c2",
				CounterpartID:  "user-3",
				Folder:         models.UserConversationFolderInbox,
				SortAt:         baseTime.Add(-time.Minute),
			},
		)
	}).Return(nil).Once()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		conversation, ok := args.Get(0).(*models.Conversation)
		if !ok {
			return
		}
		conversation.ID = "c1"
		conversation.Participants = []string{"user-1", "user-2"}
		conversation.UpdatedAt = baseTime
	}).Return(nil).Once()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		conversation, ok := args.Get(0).(*models.Conversation)
		if !ok {
			return
		}
		conversation.ID = "c2"
		conversation.Participants = []string{"user-1", "user-3"}
		conversation.UpdatedAt = baseTime.Add(-time.Minute)
	}).Return(nil).Once()

	conversations, next, err := repo.GetConversations(ctx, "user-1", 1, "")
	require.NoError(t, err)
	require.Len(t, conversations, 1)
	require.NotEmpty(t, next)
}
