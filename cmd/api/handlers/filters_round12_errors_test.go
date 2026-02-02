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

func TestFiltersHandlers_Errors_Round12(t *testing.T) {
	cfg := round11TestConfig()

	readFiltersToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read:filters"})
	writeFiltersToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:filters"})
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

	readHeaders := map[string]string{"Authorization": "Bearer " + readFiltersToken}
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeFiltersToken}
	writeHeadersJSON := map[string]string{"Authorization": "Bearer " + writeFiltersToken, "Content-Type": "application/json"}
	insufficientHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	t.Run("validateSeverity and validateMatchMode accept known values", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		require.Equal(t, "high", handler.validateSeverity("high"))
		require.Equal(t, "regex", handler.validateMatchMode("regex"))
	})

	t.Run("update filter auth and ownership errors", func(t *testing.T) {
		state := &round10QueryState{
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "bob", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctxUnauthorized, err := round10NewLiftContext(http.MethodPut, "/api/v2/filters/filter-1", nil, nil, map[string]any{"title": "x"})
		require.NoError(t, err)
		ctxUnauthorized.Params["id"] = "filter-1"
		requireStatus(t, http.StatusUnauthorized)(handler.HandleUpdateFilterLift(ctxUnauthorized))

		ctxInsufficient, err := round10NewLiftContext(http.MethodPut, "/api/v2/filters/filter-1", insufficientHeaders, nil, map[string]any{"title": "x"})
		require.NoError(t, err)
		ctxInsufficient.Params["id"] = "filter-1"
		requireStatus(t, http.StatusForbidden)(handler.HandleUpdateFilterLift(ctxInsufficient))

		ctxNotFound, err := round10NewLiftContext(http.MethodPut, "/api/v2/filters/filter-1", writeHeaders, nil, map[string]any{"title": "x"})
		require.NoError(t, err)
		ctxNotFound.Params["id"] = "filter-1"
		requireStatus(t, http.StatusNotFound)(handler.HandleUpdateFilterLift(ctxNotFound))
	})

	t.Run("update filter repository errors return 500", func(t *testing.T) {
		stateGetErr := &round10QueryState{allErrorOnce: errors.New("boom")}
		handlerGetErr, _, _ := round11NewHandler(t, cfg, stateGetErr)
		ctxGetErr, err := round10NewLiftContext(http.MethodPut, "/api/v2/filters/filter-1", writeHeaders, nil, map[string]any{"title": "x"})
		require.NoError(t, err)
		ctxGetErr.Params["id"] = "filter-1"
		requireStatus(t, http.StatusInternalServerError)(handlerGetErr.HandleUpdateFilterLift(ctxGetErr))

		stateUpdateErr := &round10QueryState{
			updateErrorOnce: errors.New("boom"),
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
		}
		handlerUpdateErr, _, _ := round11NewHandler(t, cfg, stateUpdateErr)
		ctxUpdateErr, err := round10NewLiftContext(http.MethodPut, "/api/v2/filters/filter-1", writeHeaders, nil, map[string]any{"title": "x"})
		require.NoError(t, err)
		ctxUpdateErr.Params["id"] = "filter-1"
		requireStatus(t, http.StatusInternalServerError)(handlerUpdateErr.HandleUpdateFilterLift(ctxUpdateErr))
	})

	t.Run("delete filter validation and delete error", func(t *testing.T) {
		state := &round10QueryState{
			deleteErrorOnce: errors.New("boom"),
			filterKeywords:  map[string][]storagemodels.FilterKeyword{"filter-1": {}},
			filterStatuses:  map[string][]storagemodels.FilterStatus{"filter-1": {}},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctxBad, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/", writeHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleDeleteFilterLift(ctxBad))

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "filter-1"
		requireStatus(t, http.StatusInternalServerError)(handler.HandleDeleteFilterLift(ctx))
	})

	t.Run("get filter keywords/statuses handle invalid id, ownership, and repo error", func(t *testing.T) {
		state := &round10QueryState{
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "bob", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctxBad, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters//keywords", readHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleGetFilterKeywordsLift(ctxBad))

		ctxNotFound, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1/keywords", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxNotFound.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusNotFound)(handler.HandleGetFilterKeywordsLift(ctxNotFound))

		// Force keyword list query to error.
		typeName := reflect.TypeOf(&[]storagemodels.FilterKeyword{}).String()
		stateKWErr := &round10QueryState{
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
			allErrorByType: map[string]error{typeName: errors.New("boom")},
		}
		handlerKWErr, _, _ := round11NewHandler(t, cfg, stateKWErr)
		ctxKWErr, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1/keywords", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxKWErr.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusInternalServerError)(handlerKWErr.HandleGetFilterKeywordsLift(ctxKWErr))

		// Force status list query to error.
		typeStatusName := reflect.TypeOf(&[]storagemodels.FilterStatus{}).String()
		stateStatusErr := &round10QueryState{
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
			allErrorByType: map[string]error{typeStatusName: errors.New("boom")},
		}
		handlerStatusErr, _, _ := round11NewHandler(t, cfg, stateStatusErr)
		ctxStatusErr, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1/statuses", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxStatusErr.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusInternalServerError)(handlerStatusErr.HandleGetFilterStatusesLift(ctxStatusErr))
	})

	t.Run("add keyword/status parse and repo errors", func(t *testing.T) {
		stateParse := &round10QueryState{
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
		}
		handlerParse, _, _ := round11NewHandler(t, cfg, stateParse)

		ctxKWBad := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v2/filters/filter-1/keywords", writeHeadersJSON, nil, []byte("{"))
		ctxKWBad.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusBadRequest)(handlerParse.HandleAddFilterKeywordLift(ctxKWBad))

		ctxStatusBad := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v2/filters/filter-1/statuses", writeHeadersJSON, nil, []byte("{"))
		ctxStatusBad.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusBadRequest)(handlerParse.HandleAddFilterStatusLift(ctxStatusBad))

		stateCreateErr := &round10QueryState{
			createErrorOnce: errors.New("boom"),
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
		}
		handlerCreateErr, _, _ := round11NewHandler(t, cfg, stateCreateErr)
		ctxKW, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters/filter-1/keywords", writeHeaders, nil, apimodels.AddFilterKeywordRequest{Keyword: "spam"})
		require.NoError(t, err)
		ctxKW.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusInternalServerError)(handlerCreateErr.HandleAddFilterKeywordLift(ctxKW))
	})

	t.Run("delete keyword/status validation and delete error", func(t *testing.T) {
		state := &round10QueryState{
			deleteErrorOnce: errors.New("boom"),
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctxBad, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/keywords", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxBad.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusBadRequest)(handler.HandleDeleteFilterKeywordLift(ctxBad))

		ctxDel, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/keywords/kw-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxDel.Params["filter_id"] = "filter-1"
		ctxDel.Params["keyword_id"] = "kw-1"
		requireStatus(t, http.StatusInternalServerError)(handler.HandleDeleteFilterKeywordLift(ctxDel))

		ctxStatusBad, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters//statuses/fs-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxStatusBad.Params["status_id"] = "fs-1"
		requireStatus(t, http.StatusBadRequest)(handler.HandleDeleteFilterStatusLift(ctxStatusBad))
	})

	t.Run("test filter auth and repository error", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{allErrorOnce: errors.New("boom")})

		ctxUnauthed, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters/test", nil, nil, apimodels.TestFilterRequest{Content: "hi"})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(handler.HandleTestFilterLift(ctxUnauthed))

		ctxInsufficient, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters/test", insufficientHeaders, nil, apimodels.TestFilterRequest{Content: "hi"})
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(handler.HandleTestFilterLift(ctxInsufficient))

		ctxErr, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters/test", readHeaders, nil, apimodels.TestFilterRequest{Content: "hi"})
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(handler.HandleTestFilterLift(ctxErr))
	})
}
