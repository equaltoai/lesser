package handlers

import (
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestFiltersHandlers_Round12(t *testing.T) {
	cfg := round11TestConfig()

	readFiltersToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read:filters"})
	writeFiltersToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:filters"})
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

	readHeaders := map[string]string{"Authorization": "Bearer " + readFiltersToken}
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeFiltersToken}
	writeHeadersJSON := map[string]string{"Authorization": "Bearer " + writeFiltersToken, "Content-Type": "application/json"}
	insufficientHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	t.Run("get filters unauthorized and insufficient scope", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctxUnauthed, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(handler.HandleGetFiltersLift(ctxUnauthed))

		ctxInsufficient, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters", insufficientHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(handler.HandleGetFiltersLift(ctxInsufficient))
	})

	t.Run("get filters success and repository error", func(t *testing.T) {
		state := &round10QueryState{}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters", readHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(handler.HandleGetFiltersLift(ctx))

		stateErr := &round10QueryState{allErrorOnce: errors.New("boom")}
		handlerErr, _, _ := round11NewHandler(t, cfg, stateErr)
		ctxErr, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters", readHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(handlerErr.HandleGetFiltersLift(ctxErr))
	})

	t.Run("get filter invalid id and not found", func(t *testing.T) {
		state := &round10QueryState{
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "bob", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctxBad, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/", readHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleGetFilterLift(ctxBad))

		ctxNotFound, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxNotFound.Params["id"] = "filter-1"
		requireStatus(t, http.StatusNotFound)(handler.HandleGetFilterLift(ctxNotFound))
	})

	t.Run("get filter success", func(t *testing.T) {
		state := &round10QueryState{
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
			filterKeywords: map[string][]storagemodels.FilterKeyword{
				"filter-1": {{ID: "kw-1", FilterID: "filter-1", Keyword: "spam", WholeWord: true, CreatedAt: time.Now()}},
			},
			filterStatuses: map[string][]storagemodels.FilterStatus{
				"filter-1": {{ID: "fs-1", FilterID: "filter-1", StatusID: "status-1", CreatedAt: time.Now()}},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1", readHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "filter-1"
		requireStatus(t, http.StatusOK)(handler.HandleGetFilterLift(ctx))
	})

	t.Run("create filter invalid request and invalid params", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctxBad := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v2/filters", writeHeaders, nil, []byte("{"))
		requireStatus(t, http.StatusBadRequest)(handler.HandleCreateFilterLift(ctxBad))

		ctxInvalid, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters", writeHeaders, nil, apimodels.CreateFilterRequest{Title: ""})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleCreateFilterLift(ctxInvalid))
	})

	t.Run("create filter success covers saveFilter and addFilterKeywords", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		expiresIn := 60
		matchWeight := 0.5
		isRegex := true
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters", writeHeaders, nil, apimodels.CreateFilterRequest{
			Title:        "My Filter",
			Context:      []string{"home"},
			FilterAction: "",
			Severity:     "unknown",
			MatchMode:    "invalid",
			ExpiresIn:    &expiresIn,
			KeywordsAttributes: []apimodels.FilterKeywordAttribute{
				{
					Keyword:      "spam",
					WholeWord:    true,
					IsRegex:      &isRegex,
					MatchWeight:  &matchWeight,
					ContextTypes: []string{"home"},
				},
				{Keyword: ""}, // invalid keyword, skipped
				{Keyword: "eggs", MatchWeight: ptrFloat64(2.0)}, // out of range weight
			},
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(handler.HandleCreateFilterLift(ctx))
	})

	t.Run("saveFilter returns 500 on storage error", func(t *testing.T) {
		state := &round10QueryState{createErrorOnce: errors.New("boom")}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters", nil, nil, nil)
		require.NoError(t, err)

		filter := &storage.Filter{
			Username:     "alice",
			Title:        "My Filter",
			Context:      []string{"home"},
			FilterAction: "warn",
		}

		require.Error(t, handler.saveFilter(ctx, filter))
	})

	t.Run("update filter parse error and success with keyword updates", func(t *testing.T) {
		state := &round10QueryState{
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v2/filters/filter-1", writeHeaders, nil, map[string]any{
			"title":         "new title",
			"context":       []any{"home", 123},
			"filter_action": "hide",
			"expires_in":    float64(10),
			"keywords_attributes": []any{
				map[string]any{"id": "kw-1", "_destroy": true},
				map[string]any{"id": "kw-2", "keyword": "spam", "whole_word": true},
				map[string]any{"keyword": "eggs", "whole_word": false},
				"skip",
			},
		})
		require.NoError(t, err)
		ctx.Params["id"] = "filter-1"
		requireStatus(t, http.StatusOK)(handler.HandleUpdateFilterLift(ctx))
	})

	t.Run("parseFilterUpdateParams supports JSON body bytes", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v2/filters/filter-1", writeHeadersJSON, nil, []byte(`{"title":"new title","context":["home"],"expires_in":10}`))
		params, err := handler.parseFilterUpdateParams(ctx)
		require.NoError(t, err)
		require.Equal(t, "new title", params["title"])
	})

	t.Run("delete filter and filter ownership checks", func(t *testing.T) {
		state := &round10QueryState{
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
				"filter-2": {ID: "filter-2", Username: "bob", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctxNotFound, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-2", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxNotFound.Params["id"] = "filter-2"
		requireStatus(t, http.StatusNotFound)(handler.HandleDeleteFilterLift(ctxNotFound))

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "filter-1"
		requireStatus(t, http.StatusOK)(handler.HandleDeleteFilterLift(ctx))
	})

	t.Run("keywords and statuses endpoints", func(t *testing.T) {
		state := &round10QueryState{
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
			filterKeywords: map[string][]storagemodels.FilterKeyword{
				"filter-1": {{ID: "kw-1", FilterID: "filter-1", Keyword: "spam", WholeWord: true, CreatedAt: time.Now()}},
			},
			filterStatuses: map[string][]storagemodels.FilterStatus{
				"filter-1": {{ID: "fs-1", FilterID: "filter-1", StatusID: "status-1", CreatedAt: time.Now()}},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctxKeywords, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1/keywords", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxKeywords.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusOK)(handler.HandleGetFilterKeywordsLift(ctxKeywords))

		ctxStatuses, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1/statuses", readHeaders, nil, nil)
		require.NoError(t, err)
		ctxStatuses.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusOK)(handler.HandleGetFilterStatusesLift(ctxStatuses))
	})

	t.Run("add and delete filter keyword", func(t *testing.T) {
		state := &round10QueryState{
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctxInvalid, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters/filter-1/keywords", writeHeaders, nil, apimodels.AddFilterKeywordRequest{Keyword: ""})
		require.NoError(t, err)
		ctxInvalid.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusUnprocessableEntity)(handler.HandleAddFilterKeywordLift(ctxInvalid))

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters/filter-1/keywords", writeHeaders, nil, apimodels.AddFilterKeywordRequest{Keyword: "spam", WholeWord: true})
		require.NoError(t, err)
		ctx.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusOK)(handler.HandleAddFilterKeywordLift(ctx))

		ctxDel, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/keywords/kw-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxDel.Params["filter_id"] = "filter-1"
		ctxDel.Params["keyword_id"] = "kw-1"
		requireStatus(t, http.StatusOK)(handler.HandleDeleteFilterKeywordLift(ctxDel))
	})

	t.Run("add and delete filter status", func(t *testing.T) {
		state := &round10QueryState{
			filtersByID: map[string]storagemodels.Filter{
				"filter-1": {ID: "filter-1", Username: "alice", Title: "x", Context: []string{"home"}, FilterAction: "warn"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctxMissing, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters/filter-1/statuses", writeHeaders, nil, apimodels.AddFilterStatusRequest{})
		require.NoError(t, err)
		ctxMissing.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusUnprocessableEntity)(handler.HandleAddFilterStatusLift(ctxMissing))

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters/filter-1/statuses", writeHeaders, nil, apimodels.AddFilterStatusRequest{StatusID: "status-1"})
		require.NoError(t, err)
		ctx.Params["filter_id"] = "filter-1"
		requireStatus(t, http.StatusOK)(handler.HandleAddFilterStatusLift(ctx))

		ctxDel, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/statuses/fs-1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxDel.Params["filter_id"] = "filter-1"
		ctxDel.Params["status_id"] = "fs-1"
		requireStatus(t, http.StatusOK)(handler.HandleDeleteFilterStatusLift(ctxDel))
	})

	t.Run("test filter handler validates content and returns results", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctxMissing, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters/test", readHeaders, nil, apimodels.TestFilterRequest{})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnprocessableEntity)(handler.HandleTestFilterLift(ctxMissing))

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters/test", readHeaders, nil, apimodels.TestFilterRequest{Content: "hello world"})
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(handler.HandleTestFilterLift(ctx))
	})

	t.Run("addFilterKeywords helper handles AddFilterKeyword error", func(t *testing.T) {
		state := &round10QueryState{createErrorOnce: errors.New("boom")}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters", nil, nil, nil)
		require.NoError(t, err)

		keywords := handler.addFilterKeywords(ctx, "filter-1", []apimodels.FilterKeywordAttribute{{Keyword: "spam"}})
		require.Empty(t, keywords)
	})
}

func ptrFloat64(v float64) *float64 { return &v }
