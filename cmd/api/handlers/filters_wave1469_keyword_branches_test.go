package handlers

import (
	"errors"
	"net/http"
	"testing"
	"time"

	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

// TestFiltersHandlers_Wave1469_KeywordErrorBranches covers the filter-keyword
// handler branches that were not exercised before the scan elimination made the
// keyword/status CRUD filterID-scoped point operations (umbrella #1469): the
// validation, ownership, and repository-error branches of the v2 delete-keyword
// and delete-status lift handlers plus the v1 keyword helper error paths.
func TestFiltersHandlers_Wave1469_KeywordErrorBranches(t *testing.T) {
	cfg := round11TestConfig()
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:filters"})
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read:filters"})
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeToken}

	now := time.Now()
	aliceState := &round10QueryState{
		filtersByID: map[string]storagemodels.Filter{
			"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			"filter-2": {ID: "filter-2", Username: "bob", Title: "y", Context: []string{"home"}, FilterAction: "warn"},
		},
		filterKeywords: map[string][]storagemodels.FilterKeyword{
			"filter-1": {
				{ID: "kw-1", FilterID: "filter-1", Keyword: "spam", WholeWord: false, CreatedAt: now},
			},
		},
	}

	t.Run("delete keyword lift validation and ownership error branches", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, aliceState)

		// Empty filter_id -> 400 (validation branch).
		ctxBad, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters//keywords/kw-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxBad.Params["filter_id"] = ""
		ctxBad.Params["keyword_id"] = "kw-1"
		requireStatus(t, http.StatusBadRequest)(handler.HandleDeleteFilterKeywordLift(ctxBad))

		// Repo error fetching the filter -> 500.
		stateGetErr := &round10QueryState{firstErrorOnce: errors.New("boom")}
		handlerGetErr, _, _ := round11NewHandler(t, cfg, stateGetErr)
		ctxGetErr, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/keywords/kw-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxGetErr.Params["filter_id"] = "filter-1"
		ctxGetErr.Params["keyword_id"] = "kw-1"
		requireStatus(t, http.StatusInternalServerError)(handlerGetErr.HandleDeleteFilterKeywordLift(ctxGetErr))

		// Filter owned by someone else -> 404 (not found, do not reveal).
		ctxOwner, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-2/keywords/kw-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxOwner.Params["filter_id"] = "filter-2"
		ctxOwner.Params["keyword_id"] = "kw-1"
		requireStatus(t, http.StatusNotFound)(handler.HandleDeleteFilterKeywordLift(ctxOwner))

		// Keyword ownership check repo error (not not-found) -> 500.
		typeName := "*[]models.FilterKeyword"
		stateKWErr := &round10QueryState{
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
			allErrorByType: map[string]error{typeName: errors.New("boom")},
		}
		handlerKWErr, _, _ := round11NewHandler(t, cfg, stateKWErr)
		ctxKWErr, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/keywords/kw-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxKWErr.Params["filter_id"] = "filter-1"
		ctxKWErr.Params["keyword_id"] = "kw-1"
		requireStatus(t, http.StatusInternalServerError)(handlerKWErr.HandleDeleteFilterKeywordLift(ctxKWErr))
	})

	t.Run("delete status lift validation and ownership error branches", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, aliceState)

		// Empty status_id -> 400.
		ctxBad, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/statuses/", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxBad.Params["filter_id"] = "filter-1"
		ctxBad.Params["status_id"] = ""
		requireStatus(t, http.StatusBadRequest)(handler.HandleDeleteFilterStatusLift(ctxBad))

		// Repo error fetching the filter -> 500.
		stateGetErr := &round10QueryState{allErrorOnce: errors.New("boom")}
		handlerGetErr, _, _ := round11NewHandler(t, cfg, stateGetErr)
		ctxGetErr, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/statuses/fs-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxGetErr.Params["filter_id"] = "filter-1"
		ctxGetErr.Params["status_id"] = "fs-1"
		requireStatus(t, http.StatusInternalServerError)(handlerGetErr.HandleDeleteFilterStatusLift(ctxGetErr))

		// Filter owned by someone else -> 404.
		ctxOwner, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-2/statuses/fs-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxOwner.Params["filter_id"] = "filter-2"
		ctxOwner.Params["status_id"] = "fs-1"
		requireStatus(t, http.StatusNotFound)(handler.HandleDeleteFilterStatusLift(ctxOwner))

		// Insufficient scope -> 403.
		ctxInsufficient, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/statuses/fs-1", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
		require.NoError(t, err)
		ctxInsufficient.Params["filter_id"] = "filter-1"
		ctxInsufficient.Params["status_id"] = "fs-1"
		requireStatus(t, http.StatusForbidden)(handler.HandleDeleteFilterStatusLift(ctxInsufficient))
	})

	t.Run("get keywords/statuses and add keyword error branches", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, aliceState)
		readHeaders := map[string]string{"Authorization": "Bearer " + readToken}
		insufficientHeaders := map[string]string{"Authorization": "Bearer " + writeToken}

		// GetFilterKeywordsLift: insufficient scope -> 403, unauthorized -> 401,
		// repo error -> 500, wrong owner -> 404.
		ctxInsufficient, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1/keywords", insufficientHeaders, nil, nil)
		require.NoError(t, err)
		ctxInsufficient.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusForbidden)(handler.HandleGetFilterKeywordsLift(ctxInsufficient))

		ctxUnauthorized, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1/keywords", nil, nil, nil)
		require.NoError(t, err)
		ctxUnauthorized.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusUnauthorized)(handler.HandleGetFilterKeywordsLift(ctxUnauthorized))

		stateGetErr := &round10QueryState{allErrorOnce: errors.New("boom")}
		handlerGetErr, _, _ := round11NewHandler(t, cfg, stateGetErr)
		ctxGetErr, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1/keywords", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxGetErr.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusInternalServerError)(handlerGetErr.HandleGetFilterKeywordsLift(ctxGetErr))

		ctxOwner, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-2/keywords", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxOwner.Params["filter_id"] = "filter-2"
		requireStatus(t, http.StatusNotFound)(handler.HandleGetFilterKeywordsLift(ctxOwner))

		// GetFilterStatusesLift: bad id -> 400, unauthorized -> 401, repo error -> 500.
		ctxBad, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters//statuses", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxBad.Params["filter_id"] = ""
		requireStatus(t, http.StatusBadRequest)(handler.HandleGetFilterStatusesLift(ctxBad))

		ctxUnauthStatus, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1/statuses", nil, nil, nil)
		require.NoError(t, err)
		ctxUnauthStatus.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusUnauthorized)(handler.HandleGetFilterStatusesLift(ctxUnauthStatus))

		ctxStatusErr, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1/statuses", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxStatusErr.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusInternalServerError)(handlerGetErr.HandleGetFilterStatusesLift(ctxStatusErr))

		// AddFilterKeywordLift: unauthorized -> 401, repo error -> 500, wrong owner -> 404.
		ctxAddUnauth, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters/filter-1/keywords", nil, nil, nil)
		require.NoError(t, err)
		ctxAddUnauth.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusUnauthorized)(handler.HandleAddFilterKeywordLift(ctxAddUnauth))

		ctxAddErr, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters/filter-1/keywords", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxAddErr.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusInternalServerError)(handlerGetErr.HandleAddFilterKeywordLift(ctxAddErr))

		ctxAddOwner, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters/filter-2/keywords", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxAddOwner.Params["filter_id"] = "filter-2"
		requireStatus(t, http.StatusNotFound)(handler.HandleAddFilterKeywordLift(ctxAddOwner))
	})

	t.Run("v1 keyword helpers surface repo errors", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, aliceState)

		mkCtx := func() *apptheory.Context {
			ctx, err := round10NewLiftContext(http.MethodPut, "/api/v2/filters/filter-1", writeHeaders, nil, nil)
			require.NoError(t, err)
			return ctx
		}

		// deleteFilterKeyword: keyword not in the filter -> ownership error.
		require.Error(t, handler.deleteFilterKeyword(mkCtx(), "filter-1", "kw-missing"))

		// deleteFilterKeyword: repo delete error surfaces.
		stateDel := &round10QueryState{
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
			filterKeywords: map[string][]storagemodels.FilterKeyword{
				"filter-1": {{ID: "kw-1", FilterID: "filter-1", Keyword: "spam", WholeWord: false, CreatedAt: now}},
			},
			deleteErrorOnce: errors.New("boom"),
		}
		handlerDel, _, _ := round11NewHandler(t, cfg, stateDel)
		require.Error(t, handlerDel.deleteFilterKeyword(mkCtx(), "filter-1", "kw-1"))

		// updateFilterKeyword: keyword not in the filter -> ownership error.
		require.Error(t, handler.updateFilterKeyword(mkCtx(), "filter-1", "kw-missing", map[string]any{"keyword": "x"}))

		// createFilterKeyword: repo create error is logged and swallowed.
		stateCreate := &round10QueryState{createErrorOnce: errors.New("boom")}
		handlerCreate, _, _ := round11NewHandler(t, cfg, stateCreate)
		handlerCreate.createFilterKeyword(mkCtx(), "filter-1", map[string]any{"keyword": "spam", "whole_word": true})
		handlerCreate.createFilterKeyword(mkCtx(), "filter-1", map[string]any{"keyword": ""})

		// ensureFilterKeywordBelongsToFilter: nil entry in the keyword list is
		// skipped (defensive branch) and the missing keyword reports not-found.
		err := handler.ensureFilterKeywordBelongsToFilter(mkCtx(), "filter-1", "kw-missing")
		require.ErrorIs(t, err, errFilterKeywordNotFound)
	})
}
