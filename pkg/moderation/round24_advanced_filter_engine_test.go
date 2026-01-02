package moderation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeFilterRepository struct {
	keywordsByFilterID map[string][]*models.FilterKeyword
	errByFilterID      map[string]error
}

func (f *fakeFilterRepository) GetFilterKeywords(_ context.Context, filterID string) ([]*models.FilterKeyword, error) {
	if err, ok := f.errByFilterID[filterID]; ok {
		return nil, err
	}
	return f.keywordsByFilterID[filterID], nil
}

func TestAdvancedFilterEngine_EvaluateContent_KeywordRepository(t *testing.T) {
	afe := NewAdvancedFilterEngine(zap.NewNop())
	afe.SetFilterRepository(&fakeFilterRepository{
		keywordsByFilterID: map[string][]*models.FilterKeyword{
			"f1": {
				{ID: "k1", FilterID: "f1", Keyword: "spam", WholeWord: true},
				{ID: "k2", FilterID: "f1", Keyword: "buy now", WholeWord: false},
				{ID: "k3", FilterID: "f1", Keyword: `\\bscam\\b`, IsRegex: true},
			},
		},
	})

	filters := []*models.Filter{
		{
			ID:           "f1",
			FilterAction: "hide",
			Severity:     "high",
			MatchMode:    MatchModeKeyword,
			Context:      []string{"home"},
		},
	}

	contentCtx := &ContentContext{Type: "home", Timestamp: time.Now()}
	results, err := afe.EvaluateContent(context.Background(), "this is spam buy now", filters, contentCtx)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.True(t, results[0].Matched)
	assert.Equal(t, "hide", results[0].Action)
	assert.Equal(t, "high", results[0].Severity)
	assert.Greater(t, results[0].MatchScore, 0.0)
	assert.Contains(t, results[0].MatchedRules, "keyword:spam")
	assert.Contains(t, results[0].MatchedRules, "keyword:buy now")
}

func TestAdvancedFilterEngine_EvaluateContent_KeywordFallbackAndWholeWord(t *testing.T) {
	afe := NewAdvancedFilterEngine(zap.NewNop())

	filter := &models.Filter{
		ID:           "f1",
		FilterAction: "warn",
		Severity:     "low",
		MatchMode:    MatchModeKeyword,
	}

	results, err := afe.EvaluateContent(context.Background(), "this is a SCAM message", []*models.Filter{filter}, &ContentContext{Type: "public"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Contains(t, results[0].MatchedRules, "offensive_word:scam")

	// Whole-word matches do not match substrings.
	afe.SetFilterRepository(&fakeFilterRepository{
		keywordsByFilterID: map[string][]*models.FilterKeyword{
			"f2": {
				{ID: "k1", FilterID: "f2", Keyword: "spam", WholeWord: true},
			},
		},
	})
	filter2 := &models.Filter{ID: "f2", FilterAction: "hide", Severity: "medium", MatchMode: MatchModeKeyword}
	results, err = afe.EvaluateContent(context.Background(), "spammer", []*models.Filter{filter2}, &ContentContext{Type: "public"})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestAdvancedFilterEngine_EvaluateContent_RegexRepositoryAndFallback(t *testing.T) {
	afe := NewAdvancedFilterEngine(zap.NewNop())

	filter := &models.Filter{ID: "f1", FilterAction: "hide", Severity: "high", MatchMode: string(URLPatternRegex)}
	afe.SetFilterRepository(&fakeFilterRepository{
		keywordsByFilterID: map[string][]*models.FilterKeyword{
			"f1": {
				{ID: "rx1", FilterID: "f1", Keyword: `\b(spam|scam)\b`, IsRegex: true},
			},
		},
	})

	results, err := afe.EvaluateContent(context.Background(), "this is spam", []*models.Filter{filter}, &ContentContext{Type: "public"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Contains(t, results[0].MatchedRules, "regex:"+`\b(spam|scam)\b`)

	// Fallback patterns when repository is nil.
	afe.SetFilterRepository(nil)
	results, err = afe.EvaluateContent(context.Background(), "card 4111 1111 1111 1111", []*models.Filter{filter}, &ContentContext{Type: "public"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.NotEmpty(t, results[0].MatchedRules)
}

func TestAdvancedFilterEngine_EvaluateContent_SemanticAndExact(t *testing.T) {
	afe := NewAdvancedFilterEngine(zap.NewNop())

	semantic := &models.Filter{ID: "s1", FilterAction: "warn", Severity: "high", MatchMode: "semantic"}
	results, err := afe.EvaluateContent(context.Background(), "i hate you", []*models.Filter{semantic}, &ContentContext{Type: "public"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Contains(t, results[0].MatchedRules[0], "semantic:")

	// Exact matching via repository keywords.
	exact := &models.Filter{ID: "e1", FilterAction: "hide", Severity: "medium", MatchMode: "exact"}
	afe.SetFilterRepository(&fakeFilterRepository{
		keywordsByFilterID: map[string][]*models.FilterKeyword{
			"e1": {{ID: "k1", FilterID: "e1", Keyword: "click here now"}},
		},
	})
	results, err = afe.EvaluateContent(context.Background(), "Click here now", []*models.Filter{exact}, &ContentContext{Type: "public"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Contains(t, results[0].MatchedRules, "exact:click here now")

	// Exact fallback phrases when repository is not available.
	afe.SetFilterRepository(nil)
	results, err = afe.EvaluateContent(context.Background(), "act now", []*models.Filter{exact}, &ContentContext{Type: "public"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Contains(t, results[0].MatchedRules, "exact:act now")
}

func TestAdvancedFilterEngine_FilterApplicabilityAndExpiry(t *testing.T) {
	afe := NewAdvancedFilterEngine(zap.NewNop())
	afe.SetFilterRepository(&fakeFilterRepository{errByFilterID: map[string]error{"f1": errors.New("boom")}})

	expired := time.Now().Add(-time.Hour)
	filters := []*models.Filter{
		{ID: "f1", FilterAction: "hide", Severity: "high", MatchMode: MatchModeKeyword, Context: []string{"home"}},
		{ID: "f2", FilterAction: "hide", Severity: "high", MatchMode: MatchModeKeyword, ExpiresAt: &expired},
	}

	// Not applicable: wrong context.
	results, err := afe.EvaluateContent(context.Background(), "spam", filters, &ContentContext{Type: "public"})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSemanticMatcher_AnalyzeContent_Branches(t *testing.T) {
	sm := NewSemanticMatcher()

	score, cats := sm.AnalyzeContent("HATE and violence")
	assert.InDelta(t, 0.9, score, 0.0001)
	assert.Contains(t, cats, "hate_speech")

	score, cats = sm.AnalyzeContent("buy now, limited offer")
	assert.InDelta(t, 0.8, score, 0.0001)
	assert.Contains(t, cats, "spam")
	assert.Contains(t, cats, "commercial")

	score, cats = sm.AnalyzeContent("click here for free money")
	assert.InDelta(t, 0.85, score, 0.0001)
	assert.Contains(t, cats, "spam")
	assert.Contains(t, cats, "suspicious")

	score, cats = sm.AnalyzeContent("just normal text")
	assert.InDelta(t, 0.1, score, 0.0001)
	assert.Contains(t, cats, "normal")
}
