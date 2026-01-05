package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestActivityHandler_CreateObjectInteractionNotification_Branches(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	t.Run("object lookup fails", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("GetObject", mock.Anything, "https://example.com/objects/1").Return(nil, errors.New("not found")).Once()

		h := &ActivityHandler{
			Logger:     zap.NewNop(),
			ObjectRepo: objectRepo,
		}

		h.createObjectInteractionNotification(ctx, "https://example.com/objects/1", "https://remote.example/users/bob", "favourite", "liked", "liker")
		objectRepo.AssertExpectations(t)
	})

	t.Run("status author path and notification error swallowed", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("GetObject", mock.Anything, "https://example.com/objects/1").Return(&models.Status{
			AuthorID: "https://example.com/users/alice",
		}, nil).Once()

		notificationRepo := testmocks.NewMockNotificationRepository()
		notificationRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(errors.New("boom")).Once()

		h := &ActivityHandler{
			Logger:           zap.NewNop(),
			ObjectRepo:       objectRepo,
			NotificationRepo: notificationRepo,
		}

		h.createObjectInteractionNotification(ctx, "https://example.com/objects/1", "https://remote.example/users/bob", "favourite", "liked", "liker")
		objectRepo.AssertExpectations(t)
		notificationRepo.AssertExpectations(t)
	})

	t.Run("object attributedTo map path", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("GetObject", mock.Anything, "https://example.com/objects/2").Return(map[string]any{
			"attributedTo": "https://example.com/users/alice",
		}, nil).Once()

		notificationRepo := testmocks.NewMockNotificationRepository()
		notificationRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil).Once()

		h := &ActivityHandler{
			Logger:           zap.NewNop(),
			ObjectRepo:       objectRepo,
			NotificationRepo: notificationRepo,
		}

		h.createObjectInteractionNotification(ctx, "https://example.com/objects/2", "https://remote.example/users/bob", "reblog", "boosted", "announcer")
		objectRepo.AssertExpectations(t)
		notificationRepo.AssertExpectations(t)
	})
}

func TestActivityHandler_DistributeToFollowersTimeline_Branches(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	t.Run("missing actor username", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		err := h.distributeToFollowersTimeline(ctx, &models.Status{}, "", "post-1", "ignored", false, "", time.Now())
		require.Error(t, err)
	})

	t.Run("followers retrieval fails", func(t *testing.T) {
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("GetFollowers", mock.Anything, "alice", 1000, "").Return([]string(nil), "", errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), RelationshipRepo: relationshipRepo}
		err := h.distributeToFollowersTimeline(ctx, &models.Status{Visibility: VisibilityPublic}, "https://example.com/users/alice", "post-1", "ignored", false, "", time.Now())
		require.Error(t, err)
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("direct messages are skipped", func(t *testing.T) {
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("GetFollowers", mock.Anything, "alice", 1000, "").Return([]string{"bob"}, "", nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), RelationshipRepo: relationshipRepo, TimelineRepo: testmocks.NewMockTimelineRepositoryInterface()}
		err := h.distributeToFollowersTimeline(ctx, &models.Status{Visibility: VisibilityDirect}, "https://example.com/users/alice", "post-1", "ignored", false, "", time.Now())
		require.NoError(t, err)
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("update keys failure is ignored", func(t *testing.T) {
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("GetFollowers", mock.Anything, "alice", 1000, "").Return([]string{"bob"}, "", nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), RelationshipRepo: relationshipRepo, TimelineRepo: testmocks.NewMockTimelineRepositoryInterface()}
		err := h.distributeToFollowersTimeline(ctx, &models.Status{Visibility: VisibilityPublic}, "https://example.com/users/alice", "", "ignored", false, "", time.Now())
		require.NoError(t, err)
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("timeline write fails", func(t *testing.T) {
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("GetFollowers", mock.Anything, "alice", 1000, "").Return([]string{"bob"}, "", nil).Once()

		timelineRepo := testmocks.NewMockTimelineRepositoryInterface()
		timelineRepo.On("CreateTimelineEntries", mock.Anything, mock.Anything).Return(errors.New("boom")).Once()

		h := &ActivityHandler{
			Logger:           zap.NewNop(),
			RelationshipRepo: relationshipRepo,
			TimelineRepo:     timelineRepo,
		}
		err := h.distributeToFollowersTimeline(ctx, &models.Status{Visibility: VisibilityPublic}, "https://example.com/users/alice", "post-1", "ignored", false, "", time.Now())
		require.Error(t, err)

		relationshipRepo.AssertExpectations(t)
		timelineRepo.AssertExpectations(t)
	})

	t.Run("success", func(t *testing.T) {
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("GetFollowers", mock.Anything, "alice", 1000, "").Return([]string{"bob"}, "", nil).Once()

		timelineRepo := testmocks.NewMockTimelineRepositoryInterface()
		timelineRepo.On("CreateTimelineEntries", mock.Anything, mock.Anything).Return(nil).Once()

		h := &ActivityHandler{
			Logger:           zap.NewNop(),
			RelationshipRepo: relationshipRepo,
			TimelineRepo:     timelineRepo,
		}
		err := h.distributeToFollowersTimeline(ctx, &models.Status{Visibility: VisibilityPublic}, "https://example.com/users/alice", "post-1", "ignored", false, "", time.Now())
		require.NoError(t, err)

		relationshipRepo.AssertExpectations(t)
		timelineRepo.AssertExpectations(t)
	})
}
