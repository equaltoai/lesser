package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestActivityHandler_ProcessStatusForTimelines_CoversVisibilityBranches(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	t.Run("public local actor writes federated and local entries", func(t *testing.T) {
		timelineRepo := testmocks.NewMockTimelineRepositoryInterface()
		timelineRepo.On("CreateTimelineEntries", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			require.Len(t, args.Get(1).([]*models.Timeline), 2)
		}).Return(nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), TimelineRepo: timelineRepo}
		status := &models.Status{
			StatusID:    "s1",
			AuthorID:    "https://example.com/users/alice",
			PublishedAt: time.Now(),
			Note:        &models.NoteField{Note: &activitypub.Note{Content: "hi"}},
		}

		require.NoError(t, h.processStatusForTimelines(ctx, status, VisibilityPublic, "alice"))
		timelineRepo.AssertExpectations(t)
	})

	t.Run("public remote actor writes only federated entry", func(t *testing.T) {
		timelineRepo := testmocks.NewMockTimelineRepositoryInterface()
		timelineRepo.On("CreateTimelineEntries", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			require.Len(t, args.Get(1).([]*models.Timeline), 1)
		}).Return(nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), TimelineRepo: timelineRepo}
		status := &models.Status{
			StatusID:    "s2",
			AuthorID:    "https://remote.example/users/bob",
			PublishedAt: time.Now(),
			Note:        &models.NoteField{Note: &activitypub.Note{Content: "hi"}},
		}

		require.NoError(t, h.processStatusForTimelines(ctx, status, VisibilityPublic, "bob"))
		timelineRepo.AssertExpectations(t)
	})

	t.Run("unlisted distribution failure is swallowed", func(t *testing.T) {
		timelineRepo := testmocks.NewMockTimelineRepositoryInterface()
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("GetFollowers", mock.Anything, "alice", 1000, "").Return([]string(nil), "", errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), TimelineRepo: timelineRepo, RelationshipRepo: relationshipRepo}
		status := &models.Status{
			StatusID:    "s3",
			AuthorID:    "https://example.com/users/alice",
			PublishedAt: time.Now(),
			Note:        &models.NoteField{Note: &activitypub.Note{Content: "hi"}},
		}

		require.NoError(t, h.processStatusForTimelines(ctx, status, "unlisted", "alice"))
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("private distribution success", func(t *testing.T) {
		timelineRepo := testmocks.NewMockTimelineRepositoryInterface()
		timelineRepo.On("CreateTimelineEntries", mock.Anything, mock.Anything).Return(nil).Once()

		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("GetFollowers", mock.Anything, "alice", 1000, "").Return([]string{"bob"}, "", nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), TimelineRepo: timelineRepo, RelationshipRepo: relationshipRepo}
		status := &models.Status{
			StatusID:    "s4",
			AuthorID:    "https://example.com/users/alice",
			PublishedAt: time.Now(),
			Note:        &models.NoteField{Note: &activitypub.Note{Content: "hi"}},
		}

		require.NoError(t, h.processStatusForTimelines(ctx, status, "private", "alice"))
		relationshipRepo.AssertExpectations(t)
		timelineRepo.AssertExpectations(t)
	})

	t.Run("unknown visibility defaults to direct without writes", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop(), TimelineRepo: testmocks.NewMockTimelineRepositoryInterface()}
		status := &models.Status{
			StatusID:    "s5",
			AuthorID:    "https://example.com/users/alice",
			PublishedAt: time.Now(),
			Note:        &models.NoteField{Note: &activitypub.Note{Content: "hi"}},
		}

		require.NoError(t, h.processStatusForTimelines(ctx, status, "something-else", "alice"))
	})
}
