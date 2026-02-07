package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/stretchr/testify/require"
)

func TestTimelinesRound19_TagAndListErrorBranches(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("tag timeline requires hashtag", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/tag/", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(h.HandleGetTagTimelineLift(ctx))
	})

	t.Run("tag timeline notes service error returns 500", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			NotesSvc: &NotesServiceStub{
				ListNotesFunc: func(context.Context, *notes.ListNotesQuery) (*notes.Result, error) {
					return nil, errors.New("boom")
				},
			},
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/tag/go", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["hashtag"] = "go"

		requireStatus(t, http.StatusInternalServerError)(h.HandleGetTagTimelineLift(ctx))
	})

	t.Run("tag timeline optional auth sets ViewerID", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			NotesSvc: &NotesServiceStub{
				ListNotesFunc: func(_ context.Context, query *notes.ListNotesQuery) (*notes.Result, error) {
					require.Equal(t, "alice", query.ViewerID)
					return &notes.Result{}, nil
				},
			},
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/tag/go", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["hashtag"] = "go"

		requireStatus(t, http.StatusOK)(h.HandleGetTagTimelineLift(ctx))
	})

	t.Run("list timeline missing lists service returns 500", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/list/1", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["list_id"] = "1"

		requireStatus(t, http.StatusInternalServerError)(h.HandleGetListTimelineLift(ctx))
	})

	t.Run("list timeline unauthorized error returns 403", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		listStub := &ListsServiceStub{
			GetListTimelineFunc: func(context.Context, *lists.GetListTimelineQuery) (*lists.TimelineResult, error) {
				return nil, errors.New("unauthorized")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: listStub})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/list/1", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["list_id"] = "1"

		requireStatus(t, http.StatusForbidden)(h.HandleGetListTimelineLift(ctx))
	})

	t.Run("list timeline other errors return 500", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		listStub := &ListsServiceStub{
			GetListTimelineFunc: func(context.Context, *lists.GetListTimelineQuery) (*lists.TimelineResult, error) {
				return nil, errors.New("boom")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: listStub})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/list/1", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["list_id"] = "1"

		requireStatus(t, http.StatusInternalServerError)(h.HandleGetListTimelineLift(ctx))
	})
}
