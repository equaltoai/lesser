package lift

import (
	"errors"
	"net/http"
	"reflect"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestFiltersHandlers_MoreCoverage_Round12(t *testing.T) {
	cfg := round11TestConfig()

	readFiltersToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read:filters"})
	writeFiltersToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:filters"})
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

	readHeaders := map[string]string{"Authorization": "Bearer " + readFiltersToken}
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeFiltersToken}
	writeHeadersJSON := map[string]string{"Authorization": "Bearer " + writeFiltersToken, "Content-Type": "application/json"}
	insufficientWriteHeaders := map[string]string{"Authorization": "Bearer " + readToken}
	readHeadersJSON := map[string]string{"Authorization": "Bearer " + readFiltersToken, "Content-Type": "application/json"}

	t.Run("parseFilterUpdateParams fallback uses request.Request.Body", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v2/filters/filter-1", writeHeadersJSON, nil, []byte(`{"title":"new title"}`))
		ctx.Request.Body = nil

		params, err := handler.parseFilterUpdateParams(ctx)
		require.NoError(t, err)
		require.Equal(t, "new title", params["title"])
	})

	t.Run("parseFilterUpdateParams invalid JSON returns 400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v2/filters/filter-1", writeHeadersJSON, nil, []byte("{"))
		ctx.Request.Body = nil

		_, err := handler.parseFilterUpdateParams(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("get filters continues on keywords/statuses errors", func(t *testing.T) {
		filterKeywordType := reflect.TypeOf(&[]storagemodels.FilterKeyword{}).String()
		filterStatusType := reflect.TypeOf(&[]storagemodels.FilterStatus{}).String()

		stateKWErr := &round10QueryState{
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
				"filter-2": {ID: "filter-2", Username: "alice", Title: "y", Context: []string{"home"}, FilterAction: "warn"},
			},
			allErrorByType: map[string]error{filterKeywordType: errors.New("boom")},
		}
		handlerKWErr, _, _ := round11NewHandler(t, cfg, stateKWErr)
		ctxKWErr, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters", readHeaders, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handlerKWErr.HandleGetFiltersLift(ctxKWErr))
		require.Equal(t, http.StatusOK, ctxKWErr.Response.StatusCode)

		stateStatusErr := &round10QueryState{
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
				"filter-2": {ID: "filter-2", Username: "alice", Title: "y", Context: []string{"home"}, FilterAction: "warn"},
			},
			allErrorByType: map[string]error{filterStatusType: errors.New("boom")},
		}
		handlerStatusErr, _, _ := round11NewHandler(t, cfg, stateStatusErr)
		ctxStatusErr, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters", readHeaders, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handlerStatusErr.HandleGetFiltersLift(ctxStatusErr))
		require.Equal(t, http.StatusOK, ctxStatusErr.Response.StatusCode)
	})

	t.Run("get filter auth and repository errors", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctxUnauthed, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1", nil, nil, nil)
		require.NoError(t, err)
		ctxUnauthed.SetParam("id", "filter-1")
		require.NoError(t, handler.HandleGetFilterLift(ctxUnauthed))
		require.Equal(t, http.StatusUnauthorized, ctxUnauthed.Response.StatusCode)

		ctxInsufficient, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1", insufficientWriteHeaders, nil, nil)
		require.NoError(t, err)
		ctxInsufficient.SetParam("id", "filter-1")
		require.NoError(t, handler.HandleGetFilterLift(ctxInsufficient))
		require.Equal(t, http.StatusForbidden, ctxInsufficient.Response.StatusCode)

		handlerGetErr, _, _ := round11NewHandler(t, cfg, &round10QueryState{allErrorOnce: errors.New("boom")})
		ctxGetErr, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxGetErr.SetParam("id", "filter-1")
		require.NoError(t, handlerGetErr.HandleGetFilterLift(ctxGetErr))
		require.Equal(t, http.StatusInternalServerError, ctxGetErr.Response.StatusCode)
	})

	t.Run("createFilterKeyword ignores invalid keyword", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v2/filters/filter-1", nil, nil, nil)
		require.NoError(t, err)

		handler.createFilterKeyword(ctx, "filter-1", map[string]any{"keyword": ""})
		handler.createFilterKeyword(ctx, "filter-1", map[string]any{"keyword": 123})
	})

	t.Run("add filter status internal error and delete keyword/status auth errors", func(t *testing.T) {
		state := &round10QueryState{
			createErrorOnce: errors.New("boom"),
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctxAddStatus, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters/filter-1/statuses", writeHeaders, nil, apimodels.AddFilterStatusRequest{StatusID: "status-1"})
		require.NoError(t, err)
		ctxAddStatus.SetParam("filter_id", "filter-1")
		_ = handler.HandleAddFilterStatusLift(ctxAddStatus)
		require.Equal(t, http.StatusInternalServerError, ctxAddStatus.Response.StatusCode)

		ctxDeleteKWUnauthed, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/keywords/kw-1", nil, nil, nil)
		require.NoError(t, err)
		ctxDeleteKWUnauthed.SetParam("filter_id", "filter-1")
		ctxDeleteKWUnauthed.SetParam("keyword_id", "kw-1")
		require.NoError(t, handler.HandleDeleteFilterKeywordLift(ctxDeleteKWUnauthed))
		require.Equal(t, http.StatusUnauthorized, ctxDeleteKWUnauthed.Response.StatusCode)

		ctxDeleteStatusInsufficient, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/statuses/fs-1", insufficientWriteHeaders, nil, nil)
		require.NoError(t, err)
		ctxDeleteStatusInsufficient.SetParam("filter_id", "filter-1")
		ctxDeleteStatusInsufficient.SetParam("status_id", "fs-1")
		require.NoError(t, handler.HandleDeleteFilterStatusLift(ctxDeleteStatusInsufficient))
		require.Equal(t, http.StatusForbidden, ctxDeleteStatusInsufficient.Response.StatusCode)
	})

	t.Run("test filter parse error returns 400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v2/filters/test", readHeadersJSON, nil, []byte("{"))
		_ = handler.HandleTestFilterLift(ctx)
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("add keyword/status missing filter_id returns 400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctxKW, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters//keywords", writeHeaders, nil, apimodels.AddFilterKeywordRequest{Keyword: "spam"})
		require.NoError(t, err)
		require.NoError(t, handler.HandleAddFilterKeywordLift(ctxKW))
		require.Equal(t, http.StatusBadRequest, ctxKW.Response.StatusCode)

		ctxStatus, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters//statuses", writeHeaders, nil, apimodels.AddFilterStatusRequest{StatusID: "status-1"})
		require.NoError(t, err)
		require.NoError(t, handler.HandleAddFilterStatusLift(ctxStatus))
		require.Equal(t, http.StatusBadRequest, ctxStatus.Response.StatusCode)
	})
}
