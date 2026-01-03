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
	"go.uber.org/zap"
)

func TestActivityHandler_ProcessUndoListActivity_Branches(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	t.Run("missing object id", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		undo := &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "undo-1"}, Actor: "https://example.com/users/alice"}
		orig := map[string]any{"object": map[string]any{}, "target": "https://example.com/lists/list-123/accounts"}
		require.Error(t, h.processUndoListActivity(ctx, undo, orig, "add"))
	})

	t.Run("missing actor", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		undo := &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "undo-2"}, Actor: ""}
		orig := map[string]any{"object": "https://example.com/users/bob", "target": "https://example.com/lists/list-123/accounts"}
		require.Error(t, h.processUndoListActivity(ctx, undo, orig, "add"))
	})

	t.Run("list ID extraction fails", func(t *testing.T) {
		listRepo := testmocks.NewMockListRepository()
		h := &ActivityHandler{Logger: zap.NewNop(), ListRepo: listRepo}

		undo := &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "undo-3"}, Actor: "https://example.com/users/alice"}
		orig := map[string]any{"object": "https://example.com/users/bob", "target": "///"}
		require.ErrorIs(t, h.processUndoListActivity(ctx, undo, orig, "add"), services.ErrExtractListIDFromTargetCollection)
	})

	t.Run("list retrieval fails", func(t *testing.T) {
		listRepo := testmocks.NewMockListRepository()
		listRepo.On("GetList", mock.Anything, "list-123").Return((*models.List)(nil), errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ListRepo: listRepo}

		undo := &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "undo-4"}, Actor: "https://example.com/users/alice"}
		orig := map[string]any{"object": "https://example.com/users/bob", "target": "https://example.com/lists/list-123/accounts"}
		require.Error(t, h.processUndoListActivity(ctx, undo, orig, "add"))
		listRepo.AssertExpectations(t)
	})

	t.Run("permission denied", func(t *testing.T) {
		listRepo := testmocks.NewMockListRepository()
		listRepo.On("GetList", mock.Anything, "list-123").Return(&models.List{ID: "list-123", Username: "someone-else"}, nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ListRepo: listRepo}

		undo := &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "undo-5"}, Actor: "https://example.com/users/alice"}
		orig := map[string]any{"object": "https://example.com/users/bob", "target": "https://example.com/lists/list-123/accounts"}
		require.ErrorIs(t, h.processUndoListActivity(ctx, undo, orig, "add"), services.ErrActorNoPermissionModifyList)
		listRepo.AssertExpectations(t)
	})

	t.Run("member username extraction fails", func(t *testing.T) {
		listRepo := testmocks.NewMockListRepository()
		listRepo.On("GetList", mock.Anything, "list-123").Return(&models.List{ID: "list-123", Username: "alice"}, nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ListRepo: listRepo}

		undo := &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "undo-6"}, Actor: "https://example.com/users/alice"}
		orig := map[string]any{"object": "https://example.com/users/", "target": "https://example.com/lists/list-123/accounts"}
		require.ErrorIs(t, h.processUndoListActivity(ctx, undo, orig, "add"), services.ErrExtractUsernameFromObjectID)
		listRepo.AssertExpectations(t)
	})

	t.Run("operation failure returns error", func(t *testing.T) {
		listRepo := testmocks.NewMockListRepository()
		listRepo.On("GetList", mock.Anything, "list-123").Return(&models.List{ID: "list-123", Username: "alice"}, nil).Once()
		listRepo.On("RemoveListMember", mock.Anything, "list-123", "bob").Return(errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ListRepo: listRepo}

		undo := &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "undo-7"}, Actor: "https://example.com/users/alice"}
		orig := map[string]any{"object": "https://example.com/users/bob", "target": "https://example.com/lists/list-123/accounts"}
		require.Error(t, h.processUndoListActivity(ctx, undo, orig, "add"))
		listRepo.AssertExpectations(t)
	})

	t.Run("success for undo remove (add member)", func(t *testing.T) {
		listRepo := testmocks.NewMockListRepository()
		listRepo.On("GetList", mock.Anything, "list-123").Return(&models.List{ID: "list-123", Username: "alice"}, nil).Once()
		listRepo.On("AddListMember", mock.Anything, "list-123", "bob").Return(nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ListRepo: listRepo}

		undo := &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "undo-8"}, Actor: "https://example.com/users/alice"}
		orig := map[string]any{"object": map[string]any{"id": "https://example.com/users/bob"}, "target": "https://example.com/lists/list-123/accounts"}
		require.NoError(t, h.processUndoListActivity(ctx, undo, orig, "remove"))
		listRepo.AssertExpectations(t)
	})
}
