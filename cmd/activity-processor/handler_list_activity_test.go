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

func TestActivityHandler_ProcessListActivity_Branches(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	t.Run("missing target collection", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		act := &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "list-1"}, Actor: "https://example.com/users/alice"}
		require.Error(t, h.processListActivity(ctx, act, "alice", "add"))
	})

	t.Run("invalid object extraction", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		act := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "list-2"},
			Actor:      "https://example.com/users/alice",
			Target:     "https://example.com/lists/list-123/accounts",
			Object:     123,
		}
		require.Error(t, h.processListActivity(ctx, act, "alice", "add"))
	})

	t.Run("list ID extraction fails", func(t *testing.T) {
		h := &ActivityHandler{Logger: zap.NewNop()}
		act := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "list-3"},
			Actor:      "https://example.com/users/alice",
			Target:     "///",
			Object:     "https://example.com/users/bob",
		}
		require.ErrorIs(t, h.processListActivity(ctx, act, "alice", "add"), services.ErrExtractListIDFromTargetCollection)
	})

	t.Run("list retrieval fails", func(t *testing.T) {
		listRepo := testmocks.NewMockListRepository()
		listRepo.On("GetList", mock.Anything, "list-123").Return((*models.List)(nil), errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ListRepo: listRepo}
		act := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "list-4"},
			Actor:      "https://example.com/users/alice",
			Target:     "https://example.com/lists/list-123/accounts",
			Object:     "https://example.com/users/bob",
		}
		require.Error(t, h.processListActivity(ctx, act, "alice", "add"))
		listRepo.AssertExpectations(t)
	})

	t.Run("permission denied", func(t *testing.T) {
		listRepo := testmocks.NewMockListRepository()
		listRepo.On("GetList", mock.Anything, "list-123").Return(&models.List{ID: "list-123", Username: "someone-else"}, nil).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ListRepo: listRepo}
		act := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "list-5"},
			Actor:      "https://example.com/users/alice",
			Target:     "https://example.com/lists/list-123/accounts",
			Object:     "https://example.com/users/bob",
		}
		require.ErrorIs(t, h.processListActivity(ctx, act, "alice", "add"), services.ErrActorNoPermissionModifyList)
		listRepo.AssertExpectations(t)
	})

	t.Run("add and remove with mixed object list and partial failures", func(t *testing.T) {
		listRepo := testmocks.NewMockListRepository()
		listRepo.On("GetList", mock.Anything, "list-123").Return(&models.List{ID: "list-123", Username: "alice"}, nil).Once()

		listRepo.On("AddListMember", mock.Anything, "list-123", "bob").Return(nil).Once()
		listRepo.On("AddListMember", mock.Anything, "list-123", "carol").Return(errors.New("boom")).Once()

		h := &ActivityHandler{Logger: zap.NewNop(), ListRepo: listRepo}
		act := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "list-6"},
			Actor:      "https://example.com/users/alice",
			Target:     "https://example.com/lists/list-123/accounts",
			Object: []any{
				"https://example.com/users/bob",
				map[string]any{"id": "https://example.com/users/carol"},
				"",
				123,
			},
		}
		require.NoError(t, h.processListActivity(ctx, act, "alice", "add"))

		// Same input list, but remove mode.
		listRepo2 := testmocks.NewMockListRepository()
		listRepo2.On("GetList", mock.Anything, "list-123").Return(&models.List{ID: "list-123", Username: "alice"}, nil).Once()
		listRepo2.On("RemoveListMember", mock.Anything, "list-123", "bob").Return(nil).Once()
		listRepo2.On("RemoveListMember", mock.Anything, "list-123", "carol").Return(nil).Once()

		h2 := &ActivityHandler{Logger: zap.NewNop(), ListRepo: listRepo2}
		require.NoError(t, h2.processListActivity(ctx, act, "alice", "remove"))

		listRepo.AssertExpectations(t)
		listRepo2.AssertExpectations(t)
	})
}
