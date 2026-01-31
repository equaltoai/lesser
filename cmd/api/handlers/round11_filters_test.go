package lift

import (
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestFiltersHandlers(t *testing.T) {
	state := &round10QueryState{
		filtersByID: map[string]storagemodels.Filter{
			"filter-1": {ID: "filter-1", Username: "alice", Title: "Test", Context: []string{"home"}, FilterAction: "warn", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
		filterKeywords: map[string][]storagemodels.FilterKeyword{
			"filter-1": {{ID: "kw-1", FilterID: "filter-1", Keyword: "spam", WholeWord: true, CreatedAt: time.Now()}},
		},
		filterStatuses: map[string][]storagemodels.FilterStatus{
			"filter-1": {{ID: "fs-1", FilterID: "filter-1", StatusID: "status-1", CreatedAt: time.Now()}},
		},
	}

	h, _, _ := round11NewHandlerSliceC(t, state)

	readToken := round11SignToken(t, h.cfg.JWTSecret, "alice", []string{"read:filters"}, "sess-1")
	writeToken := round11SignToken(t, h.cfg.JWTSecret, "alice", []string{"write:filters"}, "sess-1")

	ctxGet, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
	require.NoError(t, err)
	require.NoError(t, h.HandleGetFiltersLift(ctxGet))

	ctxGetOne, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
	require.NoError(t, err)
	ctxGetOne.SetParam("id", "filter-1")
	require.NoError(t, h.HandleGetFilterLift(ctxGetOne))

	createBody := models.CreateFilterRequest{
		Title:              "My Filter",
		Context:            []string{"home"},
		FilterAction:       "warn",
		KeywordsAttributes: []models.FilterKeywordAttribute{{Keyword: "spam", WholeWord: true}},
	}
	ctxCreate, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters", map[string]string{"Authorization": "Bearer " + writeToken}, nil, createBody)
	require.NoError(t, err)
	require.NoError(t, h.HandleCreateFilterLift(ctxCreate))

	updateBody := map[string]any{
		"title":         "Updated",
		"context":       []string{"notifications"},
		"filter_action": "hide",
		"keywords_attributes": []any{
			map[string]any{"id": "kw-1", "keyword": "spam2"},
			map[string]any{"id": "kw-2", "_destroy": true},
			map[string]any{"keyword": "new", "whole_word": true},
		},
	}
	ctxUpdate := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v2/filters/filter-1", map[string]string{"Authorization": "Bearer " + writeToken}, nil, round11JSONBody(t, updateBody))
	ctxUpdate.SetParam("id", "filter-1")
	require.NoError(t, h.HandleUpdateFilterLift(ctxUpdate))

	ctxDelete, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1", map[string]string{"Authorization": "Bearer " + writeToken}, nil, nil)
	require.NoError(t, err)
	ctxDelete.SetParam("id", "filter-1")
	require.NoError(t, h.HandleDeleteFilterLift(ctxDelete))

	ctxKeywords, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1/keywords", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
	require.NoError(t, err)
	ctxKeywords.SetParam("filter_id", "filter-1")
	require.NoError(t, h.HandleGetFilterKeywordsLift(ctxKeywords))

	ctxStatuses, err := round10NewLiftContext(http.MethodGet, "/api/v2/filters/filter-1/statuses", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
	require.NoError(t, err)
	ctxStatuses.SetParam("filter_id", "filter-1")
	require.NoError(t, h.HandleGetFilterStatusesLift(ctxStatuses))

	ctxAddKeyword := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v2/filters/filter-1/keywords", map[string]string{"Authorization": "Bearer " + writeToken}, nil, round11JSONBody(t, models.AddFilterKeywordRequest{Keyword: "spam", WholeWord: true}))
	ctxAddKeyword.SetParam("filter_id", "filter-1")
	require.NoError(t, h.HandleAddFilterKeywordLift(ctxAddKeyword))

	ctxDelKeyword, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/keywords/kw-1", map[string]string{"Authorization": "Bearer " + writeToken}, nil, nil)
	require.NoError(t, err)
	ctxDelKeyword.SetParam("filter_id", "filter-1")
	ctxDelKeyword.SetParam("keyword_id", "kw-1")
	require.NoError(t, h.HandleDeleteFilterKeywordLift(ctxDelKeyword))

	ctxAddStatus := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v2/filters/filter-1/statuses", map[string]string{"Authorization": "Bearer " + writeToken}, nil, round11JSONBody(t, models.AddFilterStatusRequest{StatusID: "status-1"}))
	ctxAddStatus.SetParam("filter_id", "filter-1")
	require.NoError(t, h.HandleAddFilterStatusLift(ctxAddStatus))

	ctxDelStatus, err := round10NewLiftContext(http.MethodDelete, "/api/v2/filters/filter-1/statuses/status-1", map[string]string{"Authorization": "Bearer " + writeToken}, nil, nil)
	require.NoError(t, err)
	ctxDelStatus.SetParam("filter_id", "filter-1")
	ctxDelStatus.SetParam("status_id", "status-1")
	require.NoError(t, h.HandleDeleteFilterStatusLift(ctxDelStatus))

	ctxTest := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v2/filters/test", map[string]string{"Authorization": "Bearer " + readToken}, nil, round11JSONBody(t, models.TestFilterRequest{Content: "hello"}))
	require.NoError(t, h.HandleTestFilterLift(ctxTest))
}

func TestFilterHelpers(t *testing.T) {
	h, _, _ := round11NewHandlerSliceC(t, nil)

	filter := h.buildFilterFromParams("alice", &models.CreateFilterRequest{Title: "t", Context: []string{"home"}})
	require.Equal(t, "alice", filter.Username)
	require.Equal(t, "medium", h.validateSeverity("unknown"))
	require.Equal(t, "keyword", h.validateMatchMode("unknown"))

	kw := h.extractFilterKeyword(models.FilterKeywordAttribute{Keyword: "hello", WholeWord: true})
	require.NotNil(t, kw)
	updates := h.buildFilterUpdates(map[string]any{"title": "new"})
	require.Equal(t, "new", updates["title"])
}
