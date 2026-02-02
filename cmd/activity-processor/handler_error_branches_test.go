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
	dynamock "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestActivityHandler_ErrorBranches(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	t.Run("follow relationship create fails", func(t *testing.T) {
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("CreateRelationship", mock.Anything, "bob", "alice", "follow-err").Return(errors.New("boom"))

		handler := &ActivityHandler{Logger: zap.NewNop(), RelationshipRepo: relationshipRepo}

		follow := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "follow-err", Type: ActivityTypeFollow},
			Actor:      "https://remote.example/users/bob",
			Object:     map[string]any{"id": "https://example.com/users/alice"},
		}
		require.Error(t, handler.processFollowActivity(ctx, follow, "alice"))
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("accept relationship update fails", func(t *testing.T) {
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("UpdateRelationship", mock.Anything, "bob", "alice", mock.Anything).Return(errors.New("boom"))

		handler := &ActivityHandler{Logger: zap.NewNop(), RelationshipRepo: relationshipRepo}

		accept := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "accept-err", Type: ActivityTypeAccept},
			Actor:      "https://example.com/users/alice",
			Object:     map[string]any{"actor": "https://remote.example/users/bob"},
		}
		require.Error(t, handler.processAcceptActivity(ctx, accept, "ignored"))
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("reject relationship delete fails", func(t *testing.T) {
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("DeleteRelationship", mock.Anything, "bob", "alice").Return(errors.New("boom"))

		handler := &ActivityHandler{Logger: zap.NewNop(), RelationshipRepo: relationshipRepo}

		reject := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "reject-err", Type: ActivityTypeReject},
			Actor:      "https://example.com/users/alice",
			Object:     map[string]any{"actor": "https://remote.example/users/bob"},
		}
		require.Error(t, handler.processRejectActivity(ctx, reject, "ignored"))
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("create object storage fails", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("CreateObject", mock.Anything, mock.Anything).Return(errors.New("boom"))

		handler := &ActivityHandler{
			Logger:       zap.NewNop(),
			ObjectRepo:   objectRepo,
			TimelineRepo: testmocks.NewMockTimelineRepositoryInterface(),
		}

		create := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "create-err", Type: ActivityTypeCreate, To: []string{"https://www.w3.org/ns/activitystreams#Public"}},
			Actor:      "https://example.com/users/alice",
			Object: map[string]any{
				"id":           "https://example.com/objects/1",
				"type":         ObjectTypeNote,
				"content":      "hello",
				"attributedTo": "https://example.com/users/alice",
			},
		}
		require.Error(t, handler.processCreateActivity(ctx, create, "alice"))
		objectRepo.AssertExpectations(t)
	})

	t.Run("status timeline entry creation fails", func(t *testing.T) {
		timelineRepo := testmocks.NewMockTimelineRepositoryInterface()
		timelineRepo.On("CreateTimelineEntries", mock.Anything, mock.Anything).Return(errors.New("boom"))

		handler := &ActivityHandler{
			Logger:       zap.NewNop(),
			TimelineRepo: timelineRepo,
		}

		status := &models.Status{
			StatusID:    "s1",
			AuthorID:    "https://example.com/users/alice",
			PublishedAt: time.Now(),
			Note:        &activitypub.Note{Content: "hi"},
		}
		require.Error(t, handler.processStatusForTimelines(ctx, status, VisibilityPublic, "alice"))
		timelineRepo.AssertExpectations(t)
	})

	t.Run("delete tombstone create fails", func(t *testing.T) {
		mockDB := new(dynamock.MockDB)
		mockQuery := new(dynamock.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(errors.New("boom"))

		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("GetObject", mock.Anything, "https://example.com/objects/1").Return(&models.Object{
			ID:           "https://example.com/objects/1",
			Type:         ObjectTypeNote,
			AttributedTo: "https://example.com/users/alice",
		}, nil)

		handler := &ActivityHandler{
			DB:         mockDB,
			Logger:     zap.NewNop(),
			ObjectRepo: objectRepo,
		}

		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "delete-err", Type: ActivityTypeDelete},
			Actor:      "https://example.com/users/alice",
			Object:     "https://example.com/objects/1",
		}
		require.Error(t, handler.processDeleteActivity(ctx, del, "alice"))

		mockQuery.AssertExpectations(t)
		mockDB.AssertExpectations(t)
		objectRepo.AssertExpectations(t)
	})

	t.Run("like create fails", func(t *testing.T) {
		likeRepo := testmocks.NewMockLikeRepository()
		likeRepo.On("CreateLike", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return((*models.Like)(nil), errors.New("boom"))

		handler := &ActivityHandler{Logger: zap.NewNop(), LikeRepo: likeRepo}

		like := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "like-err", Type: ActivityTypeLike},
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/objects/1",
		}
		require.Error(t, handler.processLikeActivity(ctx, like, "alice"))
		likeRepo.AssertExpectations(t)
	})

	t.Run("announce create fails", func(t *testing.T) {
		socialRepo := testmocks.NewMockSocialRepository()
		socialRepo.On("CreateAnnounce", mock.Anything, mock.Anything).Return(errors.New("boom"))

		handler := &ActivityHandler{Logger: zap.NewNop(), SocialRepo: socialRepo}

		announce := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "announce-err", Type: ActivityTypeAnnounce},
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/objects/1",
		}
		require.Error(t, handler.processAnnounceActivity(ctx, announce, "alice"))
		socialRepo.AssertExpectations(t)
	})

	t.Run("getActivityByID without repo", func(t *testing.T) {
		handler := &ActivityHandler{Logger: zap.NewNop()}
		_, err := handler.getActivityByID(ctx, "activity-id")
		require.Error(t, err)
	})

	t.Run("undo with string object fetch fails", func(t *testing.T) {
		handler := &ActivityHandler{Logger: zap.NewNop()}
		undo := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "undo-err", Type: ActivityTypeUndo},
			Actor:      "https://example.com/users/alice",
			Object:     "some-activity-id",
		}
		require.Error(t, handler.processUndoActivity(ctx, undo, "alice"))
	})

	t.Run("getObjectAuthor map path", func(t *testing.T) {
		handler := &ActivityHandler{Logger: zap.NewNop()}
		require.Equal(t, "https://example.com/users/alice", handler.getObjectAuthor(map[string]any{"attributedTo": "https://example.com/users/alice"}))
		require.Equal(t, "https://example.com/users/bob", handler.getObjectAuthor(map[string]any{"actor": "https://example.com/users/bob"}))
	})
}
