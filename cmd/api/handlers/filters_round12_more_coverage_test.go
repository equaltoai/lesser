package handlers

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

		params, err := handler.parseFilterUpdateParams(ctx)
		require.NoError(t, err)
		require.Equal(t, "new title", params["title"])
	})

	t.Run("parseFilterUpdateParams invalid JSON returns 400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v2/filters/filter-1", writeHeadersJSON, nil, []byte("{"))

		_, err := handler.parseFilterUpdateParams(ctx)
		require.Error(t, err)
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
		requireStatus(t, http.StatusOK)(handlerKWErr.HandleGetFiltersLift(ctxKWErr))

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
		requireStatus(t, http.StatusOK)(handlerStatusErr.HandleGetFiltersLift(ctxStatusErr))
	})

	t.Run("get filter auth and repository errors", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctxUnauthed, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1", nil, nil, nil)
		require.NoError(t, err)
		ctxUnauthed.Params["id"] = "filter-1"
		requireStatus(t, http.StatusUnauthorized)(handler.HandleGetFilterLift(ctxUnauthed))

		ctxInsufficient, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1", insufficientWriteHeaders, nil, nil)
		require.NoError(t, err)
		ctxInsufficient.Params["id"] = "filter-1"
		requireStatus(t, http.StatusForbidden)(handler.HandleGetFilterLift(ctxInsufficient))

		handlerGetErr, _, _ := round11NewHandler(t, cfg, &round10QueryState{allErrorOnce: errors.New("boom")})
		ctxGetErr, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxGetErr.Params["id"] = "filter-1"
		requireStatus(t, http.StatusInternalServerError)(handlerGetErr.HandleGetFilterLift(ctxGetErr))
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
		ctxAddStatus.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusInternalServerError)(handler.HandleAddFilterStatusLift(ctxAddStatus))

		ctxDeleteKWUnauthed, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/keywords/kw-1", nil, nil, nil)
		require.NoError(t, err)
		ctxDeleteKWUnauthed.Params["filter_id"] = "filter-1"
		ctxDeleteKWUnauthed.Params["keyword_id"] = "kw-1"
		requireStatus(t, http.StatusUnauthorized)(handler.HandleDeleteFilterKeywordLift(ctxDeleteKWUnauthed))

		ctxDeleteStatusInsufficient, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/statuses/fs-1", insufficientWriteHeaders, nil, nil)
		require.NoError(t, err)
		ctxDeleteStatusInsufficient.Params["filter_id"] = "filter-1"
		ctxDeleteStatusInsufficient.Params["status_id"] = "fs-1"
		requireStatus(t, http.StatusForbidden)(handler.HandleDeleteFilterStatusLift(ctxDeleteStatusInsufficient))
	})

	t.Run("test filter parse error returns 400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v2/filters/test", readHeadersJSON, nil, []byte("{"))
		requireStatus(t, http.StatusBadRequest)(handler.HandleTestFilterLift(ctx))
	})

	t.Run("add keyword/status missing filter_id returns 400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctxKW, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters//keywords", writeHeaders, nil, apimodels.AddFilterKeywordRequest{Keyword: "spam"})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleAddFilterKeywordLift(ctxKW))

		ctxStatus, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters//statuses", writeHeaders, nil, apimodels.AddFilterStatusRequest{StatusID: "status-1"})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleAddFilterStatusLift(ctxStatus))
	})
}
