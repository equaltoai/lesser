package main

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamock "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestActivityHandler_ExtractNoteFromActivity_Branches(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	h := &ActivityHandler{Logger: zap.NewNop()}

	note := &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "https://example.com/objects/1", Type: "Note"}, Content: "hello"}
	out, err := h.extractNoteFromActivity(&activitypub.Activity{Object: note})
	require.NoError(t, err)
	require.Same(t, note, out)

	out, err = h.extractNoteFromActivity(&activitypub.Activity{Object: map[string]any{
		"id":           "https://example.com/objects/2",
		"type":         "Note",
		"content":      "mapped",
		"attributedTo": "https://example.com/users/alice",
	}})
	require.NoError(t, err)
	require.Equal(t, "mapped", out.Content)

	_, err = h.extractNoteFromActivity(&activitypub.Activity{Object: "https://example.com/objects/3"})
	require.Error(t, err)
}

func TestActivityHandler_ProcessDeleteActivity_MoreBranches(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	t.Run("invalid object type", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "delete-1", Type: ActivityTypeDelete},
			Actor:      "https://example.com/users/alice",
			Object:     123,
		}
		require.ErrorIs(t, h.processDeleteActivity(ctx, del, "alice"), services.ErrDeleteInvalidObjectType)
	})

	t.Run("map missing id", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "delete-2", Type: ActivityTypeDelete},
			Actor:      "https://example.com/users/alice",
			Object:     map[string]any{},
		}
		require.ErrorIs(t, h.processDeleteActivity(ctx, del, "alice"), services.ErrDeleteMissingObjectID)
	})

	t.Run("empty string id", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "delete-3", Type: ActivityTypeDelete},
			Actor:      "https://example.com/users/alice",
			Object:     "",
		}
		require.ErrorIs(t, h.processDeleteActivity(ctx, del, "alice"), services.ErrDeleteMissingObjectID2)
	})

	t.Run("object not found is idempotent", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("GetObject", mock.Anything, "https://example.com/objects/1").Return(nil, errors.New("not found")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}
		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "delete-4", Type: ActivityTypeDelete},
			Actor:      "https://example.com/users/alice",
			Object:     "https://example.com/objects/1",
		}
		require.NoError(t, h.processDeleteActivity(ctx, del, "alice"))
		objectRepo.AssertExpectations(t)
	})

	t.Run("unauthorized actor", func(t *testing.T) {
		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("GetObject", mock.Anything, "https://example.com/objects/1").Return(&models.Object{
			ID:           "https://example.com/objects/1",
			Type:         ObjectTypeNote,
			AttributedTo: "https://example.com/users/bob",
		}, nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ObjectRepo: objectRepo}
		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "delete-5", Type: ActivityTypeDelete},
			Actor:      "https://example.com/users/alice",
			Object:     "https://example.com/objects/1",
		}
		require.ErrorIs(t, h.processDeleteActivity(ctx, del, "alice"), services.ErrActorNotAuthorizedDelete)
		objectRepo.AssertExpectations(t)
	})

	t.Run("object type map default and cascade removal error swallowed", func(t *testing.T) {
		mockDB := new(dynamock.MockDB)
		mockQuery := new(dynamock.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(nil)

		timelineRepo := testmocks.NewMockTimelineRepositoryInterface()
		timelineRepo.On("RemoveFromTimelines", mock.Anything, "https://example.com/objects/1").Return(errors.New("boom")).Once()

		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("GetObject", mock.Anything, "https://example.com/objects/1").Return(map[string]any{"actor": ""}, nil).Once()

		h := &ActivityHandler{
			DB:           mockDB,
			Logger:       zap.NewNop(),
			ObjectRepo:   objectRepo,
			TimelineRepo: timelineRepo,
		}

		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "delete-6", Type: ActivityTypeDelete},
			Actor:      "",
			Object:     "https://example.com/objects/1",
		}
		require.NoError(t, h.processDeleteActivity(ctx, del, "ignored"))

		mockQuery.AssertExpectations(t)
		mockDB.AssertExpectations(t)
		objectRepo.AssertExpectations(t)
		timelineRepo.AssertExpectations(t)
	})

	t.Run("object type status and cascade removal success", func(t *testing.T) {
		mockDB := new(dynamock.MockDB)
		mockQuery := new(dynamock.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(nil)

		timelineRepo := testmocks.NewMockTimelineRepositoryInterface()
		timelineRepo.On("RemoveFromTimelines", mock.Anything, "https://example.com/objects/2").Return(nil).Once()

		objectRepo := testmocks.NewMockObjectRepository()
		objectRepo.On("GetObject", mock.Anything, "https://example.com/objects/2").Return(&models.Status{StatusID: "s1"}, nil).Once()

		h := &ActivityHandler{
			DB:           mockDB,
			Logger:       zap.NewNop(),
			ObjectRepo:   objectRepo,
			TimelineRepo: timelineRepo,
		}

		del := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "delete-7", Type: ActivityTypeDelete},
			Actor:      "",
			Object:     "https://example.com/objects/2",
		}
		require.NoError(t, h.processDeleteActivity(ctx, del, "ignored"))

		mockQuery.AssertExpectations(t)
		mockDB.AssertExpectations(t)
		objectRepo.AssertExpectations(t)
		timelineRepo.AssertExpectations(t)
	})
}

func TestActivityHandler_ProcessBlockActivity_MoreBranches(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	t.Run("map missing id", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		block := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "block-1", Type: ActivityTypeBlock},
			Actor:      "https://example.com/users/alice",
			Object:     map[string]any{},
		}
		require.ErrorIs(t, h.processBlockActivity(ctx, block, "alice"), services.ErrBlockMissingObjectID)
	})

	t.Run("invalid object type", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		block := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "block-2", Type: ActivityTypeBlock},
			Actor:      "https://example.com/users/alice",
			Object:     1,
		}
		require.ErrorIs(t, h.processBlockActivity(ctx, block, "alice"), services.ErrBlockInvalidObjectType)
	})

	t.Run("missing blocked actor", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		block := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "block-3", Type: ActivityTypeBlock},
			Actor:      "https://example.com/users/alice",
			Object:     "",
		}
		require.ErrorIs(t, h.processBlockActivity(ctx, block, "alice"), services.ErrBlockMissingBlockedActor)
	})

	t.Run("missing blocker actor", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		block := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "block-4", Type: ActivityTypeBlock},
			Actor:      "",
			Object:     "https://remote.example/users/bob",
		}
		require.ErrorIs(t, h.processBlockActivity(ctx, block, "alice"), services.ErrBlockMissingBlockerActor)
	})

	t.Run("create block fails", func(t *testing.T) {
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("CreateBlock", mock.Anything, "https://example.com/users/alice", "https://remote.example/users/bob", "block-5").Return(errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), RelationshipRepo: relationshipRepo}
		block := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "block-5", Type: ActivityTypeBlock},
			Actor:      "https://example.com/users/alice",
			Object:     "https://remote.example/users/bob",
		}
		require.Error(t, h.processBlockActivity(ctx, block, "alice"))
		relationshipRepo.AssertExpectations(t)
	})

	t.Run("delete relationship errors are ignored", func(t *testing.T) {
		relationshipRepo := testmocks.NewMockRelationshipRepository()
		relationshipRepo.On("CreateBlock", mock.Anything, "https://example.com/users/alice", "https://remote.example/users/bob", "block-6").Return(nil).Once()
		relationshipRepo.On("DeleteRelationship", mock.Anything, "alice", "bob").Return(errors.New("boom")).Once()
		relationshipRepo.On("DeleteRelationship", mock.Anything, "bob", "alice").Return(errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), RelationshipRepo: relationshipRepo}
		block := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "block-6", Type: ActivityTypeBlock},
			Actor:      "https://example.com/users/alice",
			Object:     "https://remote.example/users/bob",
		}
		require.NoError(t, h.processBlockActivity(ctx, block, "alice"))
		relationshipRepo.AssertExpectations(t)
	})
}

func TestActivityHandler_ProcessActivityByType_UnsupportedTypes(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()
	h := &ActivityHandler{Logger: zap.NewNop()}

	unsupported := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "unsupported", Type: "NotARealType"},
		Actor:      "https://example.com/users/alice",
	}
	require.NoError(t, h.processActivityByType(ctx, unsupported, "alice", false))
	require.NoError(t, h.processActivityByType(ctx, unsupported, "alice", true))
}
