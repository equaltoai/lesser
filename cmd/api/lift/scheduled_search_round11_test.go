package lift

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/scheduled"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

type round11RemoteSearchStub struct {
	results []*federation.SearchResult
}

func (s *round11RemoteSearchStub) SearchRemoteActors(ctx context.Context, query string, limit int) ([]*federation.SearchResult, error) {
	return s.results, nil
}

func TestScheduledStatusHandlers_Round11(t *testing.T) {
	cfg := round10TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	now := time.Now().Add(2 * time.Hour).UTC()

	handler.registry = &RegistryStub{
		ScheduledSvc: &ScheduledServiceStub{
			ListScheduledStatusesFunc: func(ctx context.Context, query *scheduled.ListScheduledStatusesQuery) (*scheduled.StatusListResult, error) {
				return &scheduled.StatusListResult{
					ScheduledStatuses: []*storage.ScheduledStatus{
						{
							ID:          "sched-1",
							Status:      "scheduled post",
							ScheduledAt: now,
							MediaIDs:    []string{"media-1"},
						},
					},
					Pagination: &interfaces.PaginatedResult[string]{NextCursor: "next"},
				}, nil
			},
			GetScheduledStatusFunc: func(ctx context.Context, query *scheduled.GetScheduledStatusQuery) (*scheduled.StatusResult, error) {
				return &scheduled.StatusResult{
					ScheduledStatus: &storage.ScheduledStatus{
						ID:          query.ID,
						Status:      "scheduled status",
						ScheduledAt: now,
						MediaIDs:    []string{"media-1"},
					},
					MediaAttachments: []*storagemodels.Media{
						{MediaID: "media-1", ContentType: "image/png", CDNUrl: "https://example.com/media.png", Width: 10, Height: 5},
					},
				}, nil
			},
			UpdateScheduledStatusFunc: func(ctx context.Context, cmd *scheduled.UpdateScheduledStatusCommand) (*scheduled.StatusResult, error) {
				return &scheduled.StatusResult{
					ScheduledStatus: &storage.ScheduledStatus{
						ID:          cmd.ID,
						Status:      "updated status",
						ScheduledAt: now,
					},
				}, nil
			},
			DeleteScheduledStatusFunc: func(ctx context.Context, cmd *scheduled.DeleteScheduledStatusCommand) error {
				return nil
			},
			CreateScheduledStatusFunc: func(ctx context.Context, cmd *scheduled.CreateScheduledStatusCommand) (*scheduled.StatusResult, error) {
				return &scheduled.StatusResult{
					ScheduledStatus: &storage.ScheduledStatus{
						ID:          "sched-new",
						Status:      cmd.Status,
						ScheduledAt: cmd.ScheduledAt,
					},
				}, nil
			},
			GetScheduledMediaAttachmentsFunc: func(ctx context.Context, scheduledStatusID string) ([]*storagemodels.Media, error) {
				return []*storagemodels.Media{{MediaID: "media-1", ContentType: "video/mp4", CDNUrl: "https://example.com/video.mp4", Width: 1920, Height: 1080, Duration: 3}}, nil
			},
		},
	}

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	ctxList, err := round10NewLiftContext(http.MethodGet, "/api/v1/scheduled_statuses", readHeaders, map[string]string{"limit": "5"}, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetScheduledStatusesLift(ctxList))
	require.Equal(t, http.StatusOK, ctxList.Response.StatusCode)

	ctxGet, err := round10NewLiftContext(http.MethodGet, "/api/v1/scheduled_statuses/sched-1", readHeaders, nil, nil)
	require.NoError(t, err)
	ctxGet.SetParam("id", "sched-1")
	require.NoError(t, handler.HandleGetScheduledStatusLift(ctxGet))
	require.Equal(t, http.StatusOK, ctxGet.Response.StatusCode)

	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write"})
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeToken}

	updateReq := models.ScheduledStatusUpdateRequest{ScheduledAt: now.Format(time.RFC3339)}
	ctxUpdate, err := round10NewLiftContext(http.MethodPut, "/api/v1/scheduled_statuses/sched-1", writeHeaders, nil, updateReq)
	require.NoError(t, err)
	ctxUpdate.SetParam("id", "sched-1")
	require.NoError(t, handler.HandleUpdateScheduledStatusLift(ctxUpdate))
	require.Equal(t, http.StatusOK, ctxUpdate.Response.StatusCode)

	ctxDelete, err := round10NewLiftContext(http.MethodDelete, "/api/v1/scheduled_statuses/sched-1", writeHeaders, nil, nil)
	require.NoError(t, err)
	ctxDelete.SetParam("id", "sched-1")
	require.NoError(t, handler.HandleDeleteScheduledStatusLift(ctxDelete))
	require.Equal(t, http.StatusOK, ctxDelete.Response.StatusCode)

	statusReq := models.CreateStatusRequest{
		Status:      "future post",
		ScheduledAt: func() *string { v := now.Format(time.RFC3339); return &v }(),
		Poll:        &models.Poll{Options: []string{"a", "b"}, ExpiresIn: 300, Multiple: false, HideTotals: true},
	}
	ctxCreate, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", writeHeaders, nil, statusReq)
	require.NoError(t, err)
	created, err := handler.HandleCreateScheduledStatusLift(ctxCreate, &statusReq)
	require.NoError(t, err)
	require.Nil(t, created)
	require.Equal(t, http.StatusUnprocessableEntity, ctxCreate.Response.StatusCode)
}

func TestSearchHandlers_Round11(t *testing.T) {
	cfg := round10TestConfig()
	state := &round10QueryState{
		actorsByUser: map[string]storagemodels.Actor{
			"alice": {
				Username: "alice",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Person"},
					PreferredUsername: "alice",
					Name:              "Alice",
				},
			},
		},
		objectList: []storagemodels.Object{
			{ID: "obj-1", Type: activitypub.NoteType, Content: "hello search", Published: time.Now(), AttributedTo: "https://example.com/users/alice"},
		},
	}
	handler, _, _ := round11NewHandler(t, cfg, state)

	handler.registry = &RegistryStub{
		NotesSvc: &NotesServiceStub{
			GetSearchSuggestionsFunc: func(ctx context.Context, query *notes.GetSearchSuggestionsQuery) (*notes.GetSearchSuggestionsResult, error) {
				return &notes.GetSearchSuggestionsResult{
					Suggestions: []*storage.SearchSuggestion{{Type: "hashtag", Value: "#hello", Score: 0.5}},
				}, nil
			},
		},
	}

	handler.remoteSearch = func(store core.RepositoryStorage) remoteSearchService {
		return &round11RemoteSearchStub{
			results: []*federation.SearchResult{
				{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://remote.example/users/remote"}, PreferredUsername: "remote"}},
			},
		}
	}

	ctxAcct, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/search", nil, map[string]string{"q": "alice", "resolve": "true"}, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleAccountSearchLift(ctxAcct))
	require.Equal(t, http.StatusOK, ctxAcct.Response.StatusCode)

	suggestCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/search/suggestions", nil, map[string]string{"q": "he"}, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetSearchSuggestionsLift(suggestCtx))
	require.Equal(t, http.StatusOK, suggestCtx.Response.StatusCode)

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken}
	statusCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/search/statuses", readHeaders, map[string]string{"q": "hello"}, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleStatusSearchLift(statusCtx))
	require.Equal(t, http.StatusOK, statusCtx.Response.StatusCode)

	require.True(t, isValidHandle("@user@example.com"))
	require.False(t, isValidHandle("plain"))
}
