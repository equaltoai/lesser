package lift

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

	t.Run("getStatusInteractionConfig default", func(t *testing.T) {
		cfg := getStatusInteractionConfig(statusInteractionType(99))
		require.Equal(t, statusInteractionConfig{}, cfg)
	})

	t.Run("invalid status id returns validation error", func(t *testing.T) {
		handler.registry = &RegistryStub{NotesSvc: &NotesServiceStub{}}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/%/favourited_by", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "%")
		require.NoError(t, handler.HandleGetStatusFavouritedByLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
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

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/favourited_by", nil, map[string]string{"limit": "nope"}, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "1")
		require.NoError(t, handler.HandleGetStatusFavouritedByLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		require.Contains(t, ctx.Response.Headers["Link"], "limit=20")
	})

	t.Run("not found from service returns 404", func(t *testing.T) {
		handler.registry = &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetRebloggersFunc: func(context.Context, *notes.GetRebloggersQuery) (*notes.UsersResult, error) {
					return nil, errors.New("not found")
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/reblogged_by", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "1")
		require.NoError(t, handler.HandleGetStatusRebloggedByLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("generic service error returns 500", func(t *testing.T) {
		handler.registry = &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetLikersFunc: func(context.Context, *notes.GetLikersQuery) (*notes.UsersResult, error) {
					return nil, errors.New("boom")
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/1/favourited_by", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "1")
		require.NoError(t, handler.HandleGetStatusFavouritedByLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})
}
