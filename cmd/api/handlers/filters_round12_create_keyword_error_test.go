package handlers

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilters_createFilterKeyword_CreateError_Round12(t *testing.T) {
	cfg := round11TestConfig()
	state := &round10QueryState{createErrorOnce: errors.New("boom")}
	handler, _, _ := round11NewHandler(t, cfg, state)

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v2/filters/filter-1", nil, nil, nil)
	require.NoError(t, err)

	handler.createFilterKeyword(ctx, "filter-1", map[string]any{
		"keyword":    "cats",
		"whole_word": true,
	})
}

func TestFilters_handleKeywordUpdates_NonMapEntries_Round12(t *testing.T) {
	handler, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{})

	ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v2/filters/filter-1", nil, nil, nil)
	require.NoError(t, err)

	handler.handleKeywordUpdates(ctx, "filter-1", map[string]any{
		"keywords_attributes": []any{"not-a-map"},
	})
}

func TestFilters_handleKeywordUpdates_MissingKey_Round12(t *testing.T) {
	handler, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{})

	ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v2/filters/filter-1", nil, nil, nil)
	require.NoError(t, err)

	handler.handleKeywordUpdates(ctx, "filter-1", map[string]any{})
}
