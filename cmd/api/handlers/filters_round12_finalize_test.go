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
		require.NoError(t, handler.HandleCreateFilterLift(ctxUnauthed))
		require.Equal(t, http.StatusUnauthorized, ctxUnauthed.Response.StatusCode)

		ctxInsufficient, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters", insufficientHeaders, nil, apimodels.CreateFilterRequest{Title: "x", Context: []string{"home"}})
		require.NoError(t, err)
		require.NoError(t, handler.HandleCreateFilterLift(ctxInsufficient))
		require.Equal(t, http.StatusForbidden, ctxInsufficient.Response.StatusCode)

		ctxInvalidAction, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters", writeHeaders, nil, apimodels.CreateFilterRequest{Title: "x", Context: []string{"home"}, FilterAction: "nope"})
		require.NoError(t, err)
		require.NoError(t, handler.HandleCreateFilterLift(ctxInvalidAction))
		require.Equal(t, http.StatusBadRequest, ctxInvalidAction.Response.StatusCode)
	})

	t.Run("get filter keywords/statuses auth branches", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctxKeywordsUnauthed, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1/keywords", nil, nil, nil)
		require.NoError(t, err)
		ctxKeywordsUnauthed.SetParam("filter_id", "filter-1")
		require.NoError(t, handler.HandleGetFilterKeywordsLift(ctxKeywordsUnauthed))
		require.Equal(t, http.StatusUnauthorized, ctxKeywordsUnauthed.Response.StatusCode)

		ctxStatusesInsufficient, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1/statuses", insufficientHeaders, nil, nil)
		require.NoError(t, err)
		ctxStatusesInsufficient.SetParam("filter_id", "filter-1")
		require.NoError(t, handler.HandleGetFilterStatusesLift(ctxStatusesInsufficient))
		require.Equal(t, http.StatusForbidden, ctxStatusesInsufficient.Response.StatusCode)
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
		ctxKWErr.SetParam("id", "filter-1")
		require.NoError(t, handlerKWErr.HandleGetFilterLift(ctxKWErr))
		require.Equal(t, http.StatusInternalServerError, ctxKWErr.Response.StatusCode)

		stateStatusErr := &round10QueryState{
			filtersByID:    map[string]storagemodels.Filter{"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"}},
			allErrorByType: map[string]error{statusType: errors.New("boom")},
		}
		handlerStatusErr, _, _ := round11NewHandler(t, cfg, stateStatusErr)
		ctxStatusErr, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxStatusErr.SetParam("id", "filter-1")
		require.NoError(t, handlerStatusErr.HandleGetFilterLift(ctxStatusErr))
		require.Equal(t, http.StatusInternalServerError, ctxStatusErr.Response.StatusCode)
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
		ctxAddStatusOwner.SetParam("filter_id", "filter-1")
		require.NoError(t, handler.HandleAddFilterStatusLift(ctxAddStatusOwner))
		require.Equal(t, http.StatusNotFound, ctxAddStatusOwner.Response.StatusCode)

		ctxDeleteStatusOwner, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/statuses/fs-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxDeleteStatusOwner.SetParam("filter_id", "filter-1")
		ctxDeleteStatusOwner.SetParam("status_id", "fs-1")
		require.NoError(t, handler.HandleDeleteFilterStatusLift(ctxDeleteStatusOwner))
		require.Equal(t, http.StatusNotFound, ctxDeleteStatusOwner.Response.StatusCode)

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
		ctxDeleteStatus.SetParam("filter_id", "filter-1")
		ctxDeleteStatus.SetParam("status_id", "fs-1")
		require.NoError(t, handlerOwned.HandleDeleteFilterStatusLift(ctxDeleteStatus))
		require.Equal(t, http.StatusInternalServerError, ctxDeleteStatus.Response.StatusCode)
	})

	t.Run("delete filter auth and get filter error", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctxUnauthed, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1", nil, nil, nil)
		require.NoError(t, err)
		ctxUnauthed.SetParam("id", "filter-1")
		require.NoError(t, handler.HandleDeleteFilterLift(ctxUnauthed))
		require.Equal(t, http.StatusUnauthorized, ctxUnauthed.Response.StatusCode)

		ctxInsufficient, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxInsufficient.SetParam("id", "filter-1")
		require.NoError(t, handler.HandleDeleteFilterLift(ctxInsufficient))
		require.Equal(t, http.StatusForbidden, ctxInsufficient.Response.StatusCode)

		handlerGetErr, _, _ := round11NewHandler(t, cfg, &round10QueryState{allErrorOnce: errors.New("boom")})
		ctxGetErr, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxGetErr.SetParam("id", "filter-1")
		require.NoError(t, handlerGetErr.HandleDeleteFilterLift(ctxGetErr))
		require.Equal(t, http.StatusInternalServerError, ctxGetErr.Response.StatusCode)
	})
}
