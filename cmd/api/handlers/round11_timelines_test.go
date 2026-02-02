package handlers

import (
	"context"
	"encoding/json"
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
	ctx.Params["hashtag"] = "go"

	resp := requireStatus(t, http.StatusOK)(handler.HandleGetTagTimelineLift(ctx))
	var statuses []*apimodels.Status
	require.NoError(t, json.Unmarshal(resp.Body, &statuses))
	require.Len(t, statuses, 1)
	require.Contains(t, firstStringValue(resp.Headers, "link"), "max_id=")

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
	ctx.Params["list_id"] = "1"
	requireStatus(t, http.StatusOK)(handler.HandleGetListTimelineLift(ctx))

	listStub.GetListTimelineFunc = func(_ context.Context, _ *lists.GetListTimelineQuery) (*lists.TimelineResult, error) {
		return nil, errors.New("not found")
	}
	ctxNotFound, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/list/2", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
	require.NoError(t, err)
	ctxNotFound.Params["list_id"] = "2"
	requireStatus(t, http.StatusNotFound)(handler.HandleGetListTimelineLift(ctxNotFound))
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
	requireStatus(t, http.StatusOK)(handler.HandleGetDirectTimelineLift(ctx))
}
