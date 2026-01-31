package lift

import (
	"context"
	stdErrors "errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/scheduled"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

type round12AlwaysInvalidValidator struct{}

func (round12AlwaysInvalidValidator) Validate(any) error {
	return stdErrors.New("invalid")
}

func TestScheduledStatuses_Round12_ExtractQueryParamAndPagination(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("extract_nil_request", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/scheduled_statuses", nil, nil, nil)
		require.NoError(t, err)
		ctx.Request = nil

		require.Equal(t, "", handler.extractScheduledQueryParam(ctx, "limit"))
	})

	t.Run("extract_from_path_query_string", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/scheduled_statuses?limit=5&max_id=mx&min_id=mn", nil, nil, nil)
		require.NoError(t, err)

		require.Equal(t, "5", handler.extractScheduledQueryParam(ctx, "limit"))
		require.Equal(t, "mx", handler.extractScheduledQueryParam(ctx, "max_id"))
		require.Equal(t, "mn", handler.extractScheduledQueryParam(ctx, "min_id"))

		require.Equal(t, 5, handler.parseScheduledStatusLimit(ctx))
		require.Equal(t, "mx", handler.parseScheduledStatusCursor(ctx))
	})

	t.Run("extract_from_query_params_map", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/scheduled_statuses", nil, map[string]string{"limit": "7", "min_id": "mn"}, nil)
		require.NoError(t, err)

		require.Equal(t, "7", handler.extractScheduledQueryParam(ctx, "limit"))
		require.Equal(t, "mn", handler.parseScheduledStatusCursor(ctx))
	})

	t.Run("limit_parse_error_defaults", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/scheduled_statuses", nil, map[string]string{"limit": "bad"}, nil)
		require.NoError(t, err)

		require.Equal(t, 20, handler.parseScheduledStatusLimit(ctx))
	})

	t.Run("pagination_header_next_cursor_empty", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/scheduled_statuses", nil, nil, nil)
		require.NoError(t, err)

		handler.setScheduledStatusPaginationHeader(ctx, "", 10)
		require.Nil(t, ctx.Get("Link"))
	})

	t.Run("pagination_header_sets_link_value", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/scheduled_statuses", nil, nil, nil)
		require.NoError(t, err)

		handler.setScheduledStatusPaginationHeader(ctx, "next", 10)
		require.Equal(t, `</api/v1/scheduled_statuses?max_id=next&limit=10>; rel="next"`, ctx.Get("Link"))
	})
}

func TestScheduledStatuses_Round12_ConversionHelpers(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("convert_storage_poll_nil", func(t *testing.T) {
		require.Nil(t, handler.convertStoragePollToAPI(nil))
	})

	t.Run("convert_storage_poll_fields", func(t *testing.T) {
		poll := map[string]any{
			"options":     []string{"a", "b"},
			"expires_in":  300,
			"multiple":    true,
			"hide_totals": false,
		}

		apiPoll := handler.convertStoragePollToAPI(poll)
		require.NotNil(t, apiPoll)
		require.Equal(t, []string{"a", "b"}, apiPoll.Options)
		require.Equal(t, 300, apiPoll.ExpiresIn)
		require.True(t, apiPoll.Multiple)
		require.False(t, apiPoll.HideTotals)
	})

	t.Run("convert_scheduled_status_poll_and_media", func(t *testing.T) {
		now := time.Now().Add(2 * time.Hour).UTC()
		status := &storage.ScheduledStatus{
			ID:          "sched-1",
			Status:      "scheduled post",
			ScheduledAt: now,
			Poll: map[string]interface{}{
				"options":     []string{"a", "b"},
				"expires_in":  300,
				"multiple":    false,
				"hide_totals": true,
			},
		}

		zeroHeight := &storagemodels.Media{MediaID: "m0", ContentType: "image/png", CDNUrl: "https://example.com/0.png", Width: 10, Height: 0}
		audio := &storagemodels.Media{MediaID: "m1", ContentType: "audio/mpeg", CDNUrl: "https://example.com/1.mp3", Width: 1, Height: 1, Duration: 3}
		gif := &storagemodels.Media{MediaID: "m2", ContentType: "image/gif", CDNUrl: "https://example.com/2.gif", Width: 2, Height: 1}

		api := handler.convertScheduledStatusToAPIWithMedia(nil, status, []*storagemodels.Media{zeroHeight, audio, gif})
		require.NotNil(t, api.Params.Poll)
		require.Equal(t, "sched-1", api.ID)
		require.Len(t, api.MediaAttachments, 3)
	})
}

func TestScheduledStatuses_Round12_ParseScheduledStatusRequest(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	now := time.Now().Add(2 * time.Hour).UTC()
	body := apimodels.ScheduledStatusUpdateRequest{ScheduledAt: now.Format(time.RFC3339)}

	t.Run("parse_request_success", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/scheduled_statuses/sched-1", nil, nil, body)
		require.NoError(t, err)

		var req apimodels.ScheduledStatusUpdateRequest
		require.NoError(t, handler.parseScheduledStatusRequest(ctx, &req))
		require.Equal(t, body.ScheduledAt, req.ScheduledAt)
	})

	t.Run("parse_request_fallback_on_validation_error", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/scheduled_statuses/sched-1", nil, nil, body)
		require.NoError(t, err)

		ctx.SetValidator(round12AlwaysInvalidValidator{})

		var req apimodels.ScheduledStatusUpdateRequest
		require.NoError(t, handler.parseScheduledStatusRequest(ctx, &req))
		require.Equal(t, body.ScheduledAt, req.ScheduledAt)
	})

	t.Run("parse_request_fallback_json_error", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/scheduled_statuses/sched-1", nil, nil, []byte(`{invalid}`))
		var req apimodels.ScheduledStatusUpdateRequest
		require.Error(t, handler.parseScheduledStatusRequest(ctx, &req))
	})

	t.Run("parse_request_no_body_returns_error", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/scheduled_statuses/sched-1", nil, nil, nil)
		require.NoError(t, err)

		var req apimodels.ScheduledStatusUpdateRequest
		require.Error(t, handler.parseScheduledStatusRequest(ctx, &req))
	})
}

func TestScheduledStatuses_Round12_HandlerBranches(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now().Add(2 * time.Hour).UTC()

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeToken}

	t.Run("list_service_unavailable", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/scheduled_statuses", readHeaders, nil, nil)
		require.NoError(t, err)

		require.NoError(t, handler.HandleGetScheduledStatusesLift(ctx))
		require.Equal(t, http.StatusServiceUnavailable, ctx.Response.StatusCode)
	})

	t.Run("list_service_error", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			ScheduledSvc: &ScheduledServiceStub{
				ListScheduledStatusesFunc: func(ctx context.Context, query *scheduled.ListScheduledStatusesQuery) (*scheduled.StatusListResult, error) {
					return nil, stdErrors.New("boom")
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/scheduled_statuses", readHeaders, nil, nil)
		require.NoError(t, err)

		require.NoError(t, handler.HandleGetScheduledStatusesLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("list_no_pagination_cursor", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			ScheduledSvc: &ScheduledServiceStub{
				ListScheduledStatusesFunc: func(ctx context.Context, query *scheduled.ListScheduledStatusesQuery) (*scheduled.StatusListResult, error) {
					return &scheduled.StatusListResult{
						ScheduledStatuses: []*storage.ScheduledStatus{
							{ID: "sched-1", Status: "ok", ScheduledAt: now},
							{ID: "sched-2", Status: "ok", ScheduledAt: now, MediaIDs: []string{"m"}},
						},
						Pagination: nil,
					}, nil
				},
				GetScheduledMediaAttachmentsFunc: func(ctx context.Context, scheduledStatusID string) ([]*storagemodels.Media, error) {
					return []*storagemodels.Media{{MediaID: "m", ContentType: "video/mp4", CDNUrl: "https://example.com/v.mp4", Width: 1, Height: 1, Duration: 1}}, nil
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/scheduled_statuses", readHeaders, nil, nil)
		require.NoError(t, err)

		require.NoError(t, handler.HandleGetScheduledStatusesLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		require.Nil(t, ctx.Get("Link"))
	})

	t.Run("get_missing_id", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{ScheduledSvc: &ScheduledServiceStub{}}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/scheduled_statuses/", readHeaders, nil, nil)
		require.NoError(t, err)

		require.NoError(t, handler.HandleGetScheduledStatusLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("get_service_unavailable", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/scheduled_statuses/sched-1", readHeaders, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "sched-1")

		require.NoError(t, handler.HandleGetScheduledStatusLift(ctx))
		require.Equal(t, http.StatusServiceUnavailable, ctx.Response.StatusCode)
	})

	t.Run("get_not_found", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			ScheduledSvc: &ScheduledServiceStub{
				GetScheduledStatusFunc: func(ctx context.Context, query *scheduled.GetScheduledStatusQuery) (*scheduled.StatusResult, error) {
					return nil, stdErrors.New("not found")
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/scheduled_statuses/sched-1", readHeaders, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "sched-1")

		require.NoError(t, handler.HandleGetScheduledStatusLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("update_missing_id", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{ScheduledSvc: &ScheduledServiceStub{}}

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/scheduled_statuses/", writeHeaders, nil, apimodels.ScheduledStatusUpdateRequest{})
		require.NoError(t, err)

		require.NoError(t, handler.HandleUpdateScheduledStatusLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("update_invalid_body", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{ScheduledSvc: &ScheduledServiceStub{}}

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/scheduled_statuses/sched-1", writeHeaders, nil, []byte(`{invalid}`))
		ctx.SetParam("id", "sched-1")

		require.NoError(t, handler.HandleUpdateScheduledStatusLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("update_scheduled_at_invalid", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{ScheduledSvc: &ScheduledServiceStub{}}

		past := time.Now().Add(-1 * time.Hour).UTC()
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/scheduled_statuses/sched-1", writeHeaders, nil, apimodels.ScheduledStatusUpdateRequest{ScheduledAt: past.Format(time.RFC3339)})
		require.NoError(t, err)
		ctx.SetParam("id", "sched-1")

		require.NoError(t, handler.HandleUpdateScheduledStatusLift(ctx))
		require.Equal(t, http.StatusUnprocessableEntity, ctx.Response.StatusCode)
	})

	t.Run("update_service_unavailable", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{}

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/scheduled_statuses/sched-1", writeHeaders, nil, apimodels.ScheduledStatusUpdateRequest{})
		require.NoError(t, err)
		ctx.SetParam("id", "sched-1")

		require.NoError(t, handler.HandleUpdateScheduledStatusLift(ctx))
		require.Equal(t, http.StatusServiceUnavailable, ctx.Response.StatusCode)
	})

	for name, updateErr := range map[string]struct {
		err    error
		status int
	}{
		"update_error_not_found":               {err: stdErrors.New("not found"), status: http.StatusNotFound},
		"update_error_cannot_update_published": {err: stdErrors.New("cannot update published"), status: http.StatusUnprocessableEntity},
		"update_error_must_be_at_least":        {err: stdErrors.New("must be at least"), status: http.StatusUnprocessableEntity},
		"update_error_internal":                {err: stdErrors.New("boom"), status: http.StatusInternalServerError},
	} {
		name := name
		updateErr := updateErr
		t.Run(name, func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			handler.registry = &RegistryStub{
				ScheduledSvc: &ScheduledServiceStub{
					UpdateScheduledStatusFunc: func(ctx context.Context, cmd *scheduled.UpdateScheduledStatusCommand) (*scheduled.StatusResult, error) {
						return nil, updateErr.err
					},
				},
			}

			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/scheduled_statuses/sched-1", writeHeaders, nil, apimodels.ScheduledStatusUpdateRequest{})
			require.NoError(t, err)
			ctx.SetParam("id", "sched-1")

			require.NoError(t, handler.HandleUpdateScheduledStatusLift(ctx))
			require.Equal(t, updateErr.status, ctx.Response.StatusCode)
		})
	}

	t.Run("delete_missing_id", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{ScheduledSvc: &ScheduledServiceStub{}}

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/scheduled_statuses/", writeHeaders, nil, nil)
		require.NoError(t, err)

		require.NoError(t, handler.HandleDeleteScheduledStatusLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("delete_service_unavailable", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{}

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/scheduled_statuses/sched-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "sched-1")

		require.NoError(t, handler.HandleDeleteScheduledStatusLift(ctx))
		require.Equal(t, http.StatusServiceUnavailable, ctx.Response.StatusCode)
	})

	for name, deleteErr := range map[string]struct {
		err    error
		status int
	}{
		"delete_error_not_found":               {err: stdErrors.New("not found"), status: http.StatusNotFound},
		"delete_error_cannot_delete_published": {err: stdErrors.New("cannot delete published"), status: http.StatusUnprocessableEntity},
		"delete_error_internal":                {err: stdErrors.New("boom"), status: http.StatusInternalServerError},
	} {
		name := name
		deleteErr := deleteErr
		t.Run(name, func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
			handler.registry = &RegistryStub{
				ScheduledSvc: &ScheduledServiceStub{
					DeleteScheduledStatusFunc: func(ctx context.Context, cmd *scheduled.DeleteScheduledStatusCommand) error {
						return deleteErr.err
					},
				},
			}

			ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/scheduled_statuses/sched-1", writeHeaders, nil, nil)
			require.NoError(t, err)
			ctx.SetParam("id", "sched-1")

			require.NoError(t, handler.HandleDeleteScheduledStatusLift(ctx))
			require.Equal(t, deleteErr.status, ctx.Response.StatusCode)
		})
	}

	t.Run("delete_success", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			ScheduledSvc: &ScheduledServiceStub{
				DeleteScheduledStatusFunc: func(ctx context.Context, cmd *scheduled.DeleteScheduledStatusCommand) error {
					return nil
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/scheduled_statuses/sched-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "sched-1")

		require.NoError(t, handler.HandleDeleteScheduledStatusLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("create_scheduled_status_missing_scheduled_at", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{ScheduledSvc: &ScheduledServiceStub{}}

		statusReq := apimodels.CreateStatusRequest{Status: "future post"}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", writeHeaders, nil, statusReq)
		require.NoError(t, err)

		created, err := handler.HandleCreateScheduledStatusLift(ctx, &statusReq)
		require.NoError(t, err)
		require.Nil(t, created)
		require.Equal(t, http.StatusUnprocessableEntity, ctx.Response.StatusCode)
	})

	t.Run("create_scheduled_status_invalid_scheduled_at", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{ScheduledSvc: &ScheduledServiceStub{}}

		past := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
		statusReq := apimodels.CreateStatusRequest{Status: "future post", ScheduledAt: &past}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", writeHeaders, nil, statusReq)
		require.NoError(t, err)

		created, err := handler.HandleCreateScheduledStatusLift(ctx, &statusReq)
		require.NoError(t, err)
		require.Nil(t, created)
		require.Equal(t, http.StatusUnprocessableEntity, ctx.Response.StatusCode)
	})

	t.Run("create_scheduled_status_service_unavailable", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{}

		future := now.Format(time.RFC3339)
		statusReq := apimodels.CreateStatusRequest{Status: "future post", ScheduledAt: &future}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", writeHeaders, nil, statusReq)
		require.NoError(t, err)

		created, err := handler.HandleCreateScheduledStatusLift(ctx, &statusReq)
		require.NoError(t, err)
		require.Nil(t, created)
		require.Equal(t, http.StatusServiceUnavailable, ctx.Response.StatusCode)
	})

	t.Run("create_scheduled_status_service_error_must_be_at_least", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			ScheduledSvc: &ScheduledServiceStub{
				CreateScheduledStatusFunc: func(ctx context.Context, cmd *scheduled.CreateScheduledStatusCommand) (*scheduled.StatusResult, error) {
					return nil, stdErrors.New("must be at least")
				},
			},
		}

		future := now.Format(time.RFC3339)
		statusReq := apimodels.CreateStatusRequest{Status: "future post", ScheduledAt: &future}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", writeHeaders, nil, statusReq)
		require.NoError(t, err)

		created, err := handler.HandleCreateScheduledStatusLift(ctx, &statusReq)
		require.NoError(t, err)
		require.Nil(t, created)
		require.Equal(t, http.StatusUnprocessableEntity, ctx.Response.StatusCode)
	})

	t.Run("create_scheduled_status_service_error_internal", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			ScheduledSvc: &ScheduledServiceStub{
				CreateScheduledStatusFunc: func(ctx context.Context, cmd *scheduled.CreateScheduledStatusCommand) (*scheduled.StatusResult, error) {
					return nil, stdErrors.New("boom")
				},
			},
		}

		future := now.Format(time.RFC3339)
		statusReq := apimodels.CreateStatusRequest{Status: "future post", ScheduledAt: &future}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", writeHeaders, nil, statusReq)
		require.NoError(t, err)

		created, err := handler.HandleCreateScheduledStatusLift(ctx, &statusReq)
		require.NoError(t, err)
		require.Nil(t, created)
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("create_scheduled_status_success", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			ScheduledSvc: &ScheduledServiceStub{
				CreateScheduledStatusFunc: func(ctx context.Context, cmd *scheduled.CreateScheduledStatusCommand) (*scheduled.StatusResult, error) {
					return &scheduled.StatusResult{
						ScheduledStatus: &storage.ScheduledStatus{
							ID:          "sched-new",
							Status:      cmd.Status,
							ScheduledAt: cmd.ScheduledAt,
						},
						MediaAttachments: []*storagemodels.Media{
							{MediaID: "m1", ContentType: "audio/mpeg", CDNUrl: "https://example.com/a.mp3", Width: 1, Height: 1, Duration: 1},
						},
					}, nil
				},
			},
		}

		future := now.Format(time.RFC3339)
		statusReq := apimodels.CreateStatusRequest{Status: "future post", ScheduledAt: &future}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", writeHeaders, nil, statusReq)
		require.NoError(t, err)

		created, err := handler.HandleCreateScheduledStatusLift(ctx, &statusReq)
		require.NoError(t, err)
		require.NotNil(t, created)
		require.Equal(t, "sched-new", created.ID)
	})
}
