package graph

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/moderation"
	dbmodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound12FiltersV2_CRUDAndTest(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	ctx := round12AuthContext("alice")

	mut := resolver.Mutation()

	whole := true
	expires := 60
	filter, err := mut.CreateFilter(ctx, model.CreateFilterInput{
		Title:            "My Filter",
		Context:          []string{"home", "public"},
		ExpiresInSeconds: &expires,
		Keywords: []*model.CreateFilterKeywordInput{
			{Keyword: "spam", WholeWord: &whole},
			nil,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, filter)
	require.NotEmpty(t, filter.ID)

	kw, err := mut.AddFilterKeyword(ctx, filter.ID, model.AddFilterKeywordInput{Keyword: "bot"})
	require.NoError(t, err)
	require.NotNil(t, kw)
	require.NotEmpty(t, kw.ID)

	fs, err := mut.AddFilterStatus(ctx, filter.ID, "status-1")
	require.NoError(t, err)
	require.NotNil(t, fs)
	require.Equal(t, "status-1", fs.StatusID)

	deleted, err := mut.DeleteFilterStatus(ctx, filter.ID, fs.ID)
	require.NoError(t, err)
	require.True(t, deleted)

	deleted, err = mut.DeleteFilterStatus(ctx, filter.ID, "missing")
	require.Error(t, err)
	require.False(t, deleted)

	title := "Updated Filter"
	action := model.FilterActionHide
	noExpiry := 0
	updated, err := mut.UpdateFilter(ctx, filter.ID, model.UpdateFilterInput{
		Title:            &title,
		Context:          []string{"home"},
		FilterAction:     &action,
		ExpiresInSeconds: &noExpiry,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)

	deleted, err = mut.DeleteFilterKeyword(ctx, filter.ID, kw.ID)
	require.NoError(t, err)
	require.True(t, deleted)

	// Exercise the filter testing path using the moderation repository fallback
	// (the filter repository path does not provide keywords from account preferences).
	storageRepo.filterRepo = nil
	payload, err := mut.TestFilters(ctx, model.FilterTestInput{
		Content: "this is spam",
		Context: []string{"home", "", "public"},
	})
	require.NoError(t, err)
	require.NotNil(t, payload)
	require.Equal(t, "this is spam", payload.Content)

	ok, err := mut.DeleteFilter(ctx, filter.ID)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestRound12FiltersV2_ErrorCasesAndHelpers(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	mut := resolver.Mutation()

	invalidAction := model.FilterAction("INVALID")
	_, err := mut.CreateFilter(round12AuthContext("alice"), model.CreateFilterInput{
		Title:        "My Filter",
		Context:      []string{"home"},
		FilterAction: &invalidAction,
	})
	require.Error(t, err)

	filter, err := mut.CreateFilter(round12AuthContext("alice"), model.CreateFilterInput{
		Title:   "Owner Filter",
		Context: []string{"home"},
	})
	require.NoError(t, err)

	_, err = mut.UpdateFilter(round12AuthContext("bob"), filter.ID, model.UpdateFilterInput{})
	require.Error(t, err)

	_, err = evaluateFilterTest(round12AuthContext("alice"), nil, "content", nil, []string{"home"})
	require.Error(t, err)

	now := time.Now()
	baseFilter := &dbmodels.Filter{ID: "f1", Title: "t1"}
	upsertMergedFilterResult(nil, &moderation.FilterResult{Filter: baseFilter, MatchedRules: []string{"a"}, MatchScore: 0.1})
	upsertMergedFilterResult(map[string]*moderation.FilterResult{}, nil)
	upsertMergedFilterResult(map[string]*moderation.FilterResult{}, &moderation.FilterResult{})

	merged := map[string]*moderation.FilterResult{
		"f1": {Filter: baseFilter, MatchedRules: []string{"a"}, MatchScore: 0.1},
	}
	upsertMergedFilterResult(merged, &moderation.FilterResult{
		Filter:       baseFilter,
		MatchedRules: []string{"a", "b"},
		MatchScore:   0.9,
	})

	results := buildFilterTestResults(map[string]*moderation.FilterResult{
		"f1": merged["f1"],
		"f2": {
			Filter:       &dbmodels.Filter{ID: "f2", Title: "t2"},
			Action:       "warn",
			Severity:     "",
			MatchScore:   0.2,
			MatchedRules: nil,
		},
		"f3": nil,
	})
	require.Len(t, results, 2)
	require.Equal(t, "f1", results[0].FilterID)
	require.Equal(t, defaultFilterSeverity, results[1].Severity)
	require.NotNil(t, results[1].MatchedRules)
	require.False(t, now.IsZero())
}

