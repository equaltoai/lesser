package graph

import (
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/stretchr/testify/require"
)

func TestArticleRenderedHTMLComplexityPricesConnectionCardinality(t *testing.T) {
	t.Parallel()

	cfg := NewConfig(&Resolver{})
	require.NotNil(t, cfg.Complexity.Article.RenderedHTML)
	require.NotNil(t, cfg.Complexity.Query.Articles)

	markedChild := cfg.Complexity.Article.RenderedHTML(5)
	first := maxCMSPageSize
	cost := cfg.Complexity.Query.Articles(markedChild, nil, nil, nil, &first, nil)
	require.Equal(t, 5+maxCMSPageSize*articleRenderedHTMLComplexityCost, cost)

	defaultCost := cfg.Complexity.Query.Articles(markedChild, nil, nil, nil, nil, nil)
	require.Equal(t, 5+defaultCMSPageSize*articleRenderedHTMLComplexityCost, defaultCost)
}

func TestArticleRenderedHTMLComplexityPreservesCheapQueriesAndPricesAllArticleRoots(t *testing.T) {
	t.Parallel()

	cfg := NewConfig(&Resolver{})
	const base = 7
	markedChild := cfg.Complexity.Article.RenderedHTML(base)
	directCost := base + articleRenderedHTMLComplexityCost

	require.Equal(t, base, cfg.Complexity.Query.Article(base, "article-id"))
	require.Equal(t, directCost, cfg.Complexity.Query.Article(markedChild, "article-id"))
	require.Equal(t, directCost, cfg.Complexity.Query.ArticleBySlug(markedChild, "article-slug"))

	require.Equal(t, directCost, cfg.Complexity.Mutation.PublishDraft(markedChild, "draft-id"))
	require.Equal(t, directCost, cfg.Complexity.Mutation.CreateArticle(markedChild, model.CreateArticleInput{}))
	require.Equal(t, directCost, cfg.Complexity.Mutation.UpdateArticle(markedChild, "article-id", model.UpdateArticleInput{}))
	require.Equal(t, directCost, cfg.Complexity.Mutation.RestoreRevision(markedChild, "article-id", 1))
	require.Equal(t, directCost, cfg.Complexity.Mutation.AddArticleToCategory(markedChild, "category-id", "article-id"))
	require.Equal(t, directCost, cfg.Complexity.Mutation.RemoveArticleFromCategory(markedChild, "category-id", "article-id"))
}

func TestArticleRenderedHTMLComplexityClampsConnectionPageSize(t *testing.T) {
	t.Parallel()

	cfg := NewConfig(&Resolver{})
	markedChild := cfg.Complexity.Article.RenderedHTML(0)

	zero := 0
	require.Equal(t, defaultCMSPageSize*articleRenderedHTMLComplexityCost,
		cfg.Complexity.Query.Articles(markedChild, nil, nil, nil, &zero, nil))

	tooLarge := maxCMSPageSize + 500
	require.Equal(t, maxCMSPageSize*articleRenderedHTMLComplexityCost,
		cfg.Complexity.Query.Articles(markedChild, nil, nil, nil, &tooLarge, nil))
}
