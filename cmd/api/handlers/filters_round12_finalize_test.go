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

func TestFiltersHandlers_Finalize_Round12(t *testing.T) {
	cfg := round11TestConfig()

	readFiltersToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read:filters"})
	writeFiltersToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:filters"})
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

	readHeaders := map[string]string{"Authorization": "Bearer " + readFiltersToken}
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeFiltersToken}
	insufficientHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	t.Run("create filter auth and invalid action", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctxUnauthed, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters", nil, nil, apimodels.CreateFilterRequest{Title: "x", Context: []string{"home"}})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(handler.HandleCreateFilterLift(ctxUnauthed))

		ctxInsufficient, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters", insufficientHeaders, nil, apimodels.CreateFilterRequest{Title: "x", Context: []string{"home"}})
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(handler.HandleCreateFilterLift(ctxInsufficient))

		ctxInvalidAction, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters", writeHeaders, nil, apimodels.CreateFilterRequest{Title: "x", Context: []string{"home"}, FilterAction: "nope"})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleCreateFilterLift(ctxInvalidAction))
	})

	t.Run("get filter keywords/statuses auth branches", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctxKeywordsUnauthed, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1/keywords", nil, nil, nil)
		require.NoError(t, err)
		ctxKeywordsUnauthed.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusUnauthorized)(handler.HandleGetFilterKeywordsLift(ctxKeywordsUnauthed))

		ctxStatusesInsufficient, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1/statuses", insufficientHeaders, nil, nil)
		require.NoError(t, err)
		ctxStatusesInsufficient.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusForbidden)(handler.HandleGetFilterStatusesLift(ctxStatusesInsufficient))
	})

	t.Run("get filter keyword/status repository errors", func(t *testing.T) {
		kwType := reflect.TypeOf(&[]storagemodels.FilterKeyword{}).String()
		statusType := reflect.TypeOf(&[]storagemodels.FilterStatus{}).String()

		stateKWErr := &round10QueryState{
			filtersByID:    map[string]storagemodels.Filter{"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"}},
			allErrorByType: map[string]error{kwType: errors.New("boom")},
		}
		handlerKWErr, _, _ := round11NewHandler(t, cfg, stateKWErr)
		ctxKWErr, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxKWErr.Params["id"] = "filter-1"
		requireStatus(t, http.StatusInternalServerError)(handlerKWErr.HandleGetFilterLift(ctxKWErr))

		stateStatusErr := &round10QueryState{
			filtersByID:    map[string]storagemodels.Filter{"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"}},
			allErrorByType: map[string]error{statusType: errors.New("boom")},
		}
		handlerStatusErr, _, _ := round11NewHandler(t, cfg, stateStatusErr)
		ctxStatusErr, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxStatusErr.Params["id"] = "filter-1"
		requireStatus(t, http.StatusInternalServerError)(handlerStatusErr.HandleGetFilterLift(ctxStatusErr))
	})

	t.Run("add/delete filter status ownership and delete error", func(t *testing.T) {
		state := &round10QueryState{
			deleteErrorOnce: errors.New("boom"),
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "bob", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctxAddStatusOwner, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters/filter-1/statuses", writeHeaders, nil, apimodels.AddFilterStatusRequest{StatusID: "status-1"})
		require.NoError(t, err)
		ctxAddStatusOwner.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusNotFound)(handler.HandleAddFilterStatusLift(ctxAddStatusOwner))

		ctxDeleteStatusOwner, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/statuses/fs-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxDeleteStatusOwner.Params["filter_id"] = "filter-1"
		ctxDeleteStatusOwner.Params["status_id"] = "fs-1"
		requireStatus(t, http.StatusNotFound)(handler.HandleDeleteFilterStatusLift(ctxDeleteStatusOwner))

		// Delete error when filter belongs to user.
		stateOwned := &round10QueryState{
			deleteErrorOnce: errors.New("boom"),
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
		}
		handlerOwned, _, _ := round11NewHandler(t, cfg, stateOwned)
		ctxDeleteStatus, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/statuses/fs-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxDeleteStatus.Params["filter_id"] = "filter-1"
		ctxDeleteStatus.Params["status_id"] = "fs-1"
		requireStatus(t, http.StatusInternalServerError)(handlerOwned.HandleDeleteFilterStatusLift(ctxDeleteStatus))
	})

	t.Run("delete filter auth and get filter error", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctxUnauthed, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1", nil, nil, nil)
		require.NoError(t, err)
		ctxUnauthed.Params["id"] = "filter-1"
		requireStatus(t, http.StatusUnauthorized)(handler.HandleDeleteFilterLift(ctxUnauthed))

		ctxInsufficient, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxInsufficient.Params["id"] = "filter-1"
		requireStatus(t, http.StatusForbidden)(handler.HandleDeleteFilterLift(ctxInsufficient))

		handlerGetErr, _, _ := round11NewHandler(t, cfg, &round10QueryState{allErrorOnce: errors.New("boom")})
		ctxGetErr, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxGetErr.Params["id"] = "filter-1"
		requireStatus(t, http.StatusInternalServerError)(handlerGetErr.HandleDeleteFilterLift(ctxGetErr))
	})
}
