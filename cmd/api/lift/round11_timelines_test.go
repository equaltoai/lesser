package lift

import (
	"context"
	"errors"
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestTimelines_TagTimeline(t *testing.T) {
	cfg := round11TestConfig()

	noteLocal := &storagemodels.Status{StatusID: "s1", AuthorUsername: "alice", Content: "local"}
	noteRemote := &storagemodels.Status{StatusID: "s2", AuthorUsername: "bob@example.com", Content: "remote"}

	notesStub := &NotesServiceStub{
		ListNotesFunc: func(_ context.Context, query *notes.ListNotesQuery) (*notes.Result, error) {
			require.Equal(t, "hashtag", query.TimelineType)
			return &notes.Result{
				Notes: []*storagemodels.Status{noteLocal, noteRemote},
				Pagination: &interfaces.PaginatedResult[*storagemodels.Status]{
					NextCursor: "next",
				},
			}, nil
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesStub})

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/tag/go", nil, map[string]string{"local": "true", "limit": "2"}, nil)
	require.NoError(t, err)
	ctx.SetParam("hashtag", "go")

	require.NoError(t, handler.HandleGetTagTimelineLift(ctx))
	resp := ctx.Response.Body.([]*apimodels.Status)
	require.Len(t, resp, 1)
	require.Contains(t, ctx.Response.Headers["Link"], "max_id=")

	params, err := handler.parseTagTimelineParams(ctx, "go")
	require.NoError(t, err)
	require.Equal(t, 2, params.Limit)
}

func TestTimelines_ListTimeline(t *testing.T) {
	cfg := round11TestConfig()
	status := &storagemodels.Status{StatusID: "s1", AuthorUsername: "alice", Content: "list"}

	listStub := &ListsServiceStub{
		GetListTimelineFunc: func(_ context.Context, query *lists.GetListTimelineQuery) (*lists.TimelineResult, error) {
			return &lists.TimelineResult{
				Statuses: []*storagemodels.Status{status},
				Pagination: &interfaces.PaginatedResult[*storagemodels.Status]{
					NextCursor: "next",
				},
			}, nil
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: listStub})
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/list/1", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
	require.NoError(t, err)
	ctx.SetParam("list_id", "1")
	require.NoError(t, handler.HandleGetListTimelineLift(ctx))
	require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

	listStub.GetListTimelineFunc = func(_ context.Context, _ *lists.GetListTimelineQuery) (*lists.TimelineResult, error) {
		return nil, errors.New("not found")
	}
	ctxNotFound, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/list/2", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
	require.NoError(t, err)
	ctxNotFound.SetParam("list_id", "2")
	require.NoError(t, handler.HandleGetListTimelineLift(ctxNotFound))
	require.Equal(t, http.StatusNotFound, ctxNotFound.Response.StatusCode)
}

func TestTimelines_DirectTimeline(t *testing.T) {
	cfg := round11TestConfig()
	status := &storagemodels.Status{StatusID: "s1", AuthorUsername: "alice", Content: "direct"}

	notesStub := &NotesServiceStub{
		ListNotesFunc: func(_ context.Context, _ *notes.ListNotesQuery) (*notes.Result, error) {
			return &notes.Result{
				Notes: []*storagemodels.Status{status},
				Pagination: &interfaces.PaginatedResult[*storagemodels.Status]{
					NextCursor: "next",
				},
			}, nil
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{NotesSvc: notesStub})
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/direct", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetDirectTimelineLift(ctx))
	require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
}
