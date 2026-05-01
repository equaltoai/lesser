package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/require"
)

func TestStatusInteractions_Round12(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	readHeaders := map[string]string{"Authorization": "Bearer " + round10SignAccessToken(t, cfg.JWTSecret, "alice")}

	t.Run("getStatusInteractionConfig default", func(t *testing.T) {
		cfg := getStatusInteractionConfig(statusInteractionType(99))
		require.Equal(t, statusInteractionConfig{}, cfg)
	})

	t.Run("invalid status id returns validation error", func(t *testing.T) {
		handler.registry = &RegistryStub{NotesSvc: &NotesServiceStub{}}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/%/favourited_by", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "%"
		requireStatus(t, http.StatusBadRequest)(handler.HandleGetStatusFavouritedByLift(ctx))
	})

	t.Run("invalid limit defaults and sets pagination header", func(t *testing.T) {
		handler.registry = &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetLikersFunc: func(context.Context, *notes.GetLikersQuery) (*notes.UsersResult, error) {
					return &notes.UsersResult{
						Users: []*storage.Account{
							{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("bob"), Type: "Person"}, PreferredUsername: "bob"}},
						},
						Pagination: &interfaces.PaginatedResult[*storage.Account]{NextCursor: "next"},
					}, nil
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/favourited_by", readHeaders, map[string]string{"limit": "nope"}, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "1"
		resp := requireStatus(t, http.StatusOK)(handler.HandleGetStatusFavouritedByLift(ctx))
		require.Contains(t, firstStringValue(resp.Headers, "link"), "limit=20")
	})

	t.Run("not found from service returns 404", func(t *testing.T) {
		handler.registry = &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetRebloggersFunc: func(context.Context, *notes.GetRebloggersQuery) (*notes.UsersResult, error) {
					return nil, errors.New("not found")
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/reblogged_by", readHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "1"
		requireStatus(t, http.StatusNotFound)(handler.HandleGetStatusRebloggedByLift(ctx))
	})

	t.Run("generic service error returns 500", func(t *testing.T) {
		handler.registry = &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetLikersFunc: func(context.Context, *notes.GetLikersQuery) (*notes.UsersResult, error) {
					return nil, errors.New("boom")
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/favourited_by", readHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "1"
		requireStatus(t, http.StatusInternalServerError)(handler.HandleGetStatusFavouritedByLift(ctx))
	})

	t.Run("missing auth is rejected", func(t *testing.T) {
		handler.registry = &RegistryStub{NotesSvc: &NotesServiceStub{}}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/favourited_by", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "1"
		requireStatus(t, http.StatusUnauthorized)(handler.HandleGetStatusFavouritedByLift(ctx))
	})

	t.Run("passes authenticated viewer to service", func(t *testing.T) {
		handler.registry = &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetRebloggersFunc: func(_ context.Context, query *notes.GetRebloggersQuery) (*notes.UsersResult, error) {
					require.Equal(t, "alice", query.ViewerID)
					return &notes.UsersResult{
						Users:      []*storage.Account{},
						Pagination: &interfaces.PaginatedResult[*storage.Account]{},
					}, nil
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/reblogged_by", readHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "1"
		requireStatus(t, http.StatusOK)(handler.HandleGetStatusRebloggedByLift(ctx))
	})
}
