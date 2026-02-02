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
	"github.com/theory-cloud/tabletheory/pkg/mocks"
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
		ptr, ok := args.Get(0).(*[]*models.ConversationParticipantRecord)
		if !ok {
			return
		}
		*ptr = append(*ptr,
			&models.ConversationParticipantRecord{
				PK: "USER_CONVERSATIONS#user-1",
				SK: baseTime.Format(time.RFC3339) + "#c1",
				Conversation: &models.Conversation{
					ID:        "c1",
					UpdatedAt: baseTime,
				},
			},
			&models.ConversationParticipantRecord{PK: "USER_CONVERSATIONS#user-1", SK: baseTime.Format(time.RFC3339) + "#c2"},
		)
	}).Return(nil).Once()

	conversations, next, err := repo.GetConversations(ctx, "user-1", 1, "")
	require.NoError(t, err)
	require.Len(t, conversations, 1)
	require.NotEmpty(t, next)
}
