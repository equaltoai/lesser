package graph

import "github.com/equaltoai/lesser/graph/model"

const (
	// articleRenderedHTMLComplexityMarker lets the root field price an Article
	// rendering once it knows the requested connection cardinality. The marker
	// is far above any query admitted by the parser and ordinary complexity gate.
	articleRenderedHTMLComplexityMarker = 1 << 40
	// One Article render costs enough to reject oversized rendered connections
	// while leaving the ordinary 25-Article page usable under the default limit.
	articleRenderedHTMLComplexityCost = 16
)

// NewConfig returns the executable-schema configuration with Lesser's custom
// resource-sensitive complexity pricing installed.
func NewConfig(resolvers ResolverRoot) Config {
	cfg := Config{Resolvers: resolvers}
	configureArticleRenderingComplexity(&cfg.Complexity)
	return cfg
}

func configureArticleRenderingComplexity(complexity *ComplexityRoot) {
	if complexity == nil {
		return
	}

	complexity.Article.RenderedHTML = func(childComplexity int) int {
		return childComplexity + articleRenderedHTMLComplexityMarker
	}

	complexity.Query.Article = func(childComplexity int, _ string) int {
		return priceArticleRenderingComplexity(childComplexity, 1)
	}
	complexity.Query.ArticleBySlug = func(childComplexity int, _ string) int {
		return priceArticleRenderingComplexity(childComplexity, 1)
	}
	complexity.Query.Articles = func(
		childComplexity int,
		_ *string,
		_ *string,
		_ *string,
		_ *string,
		first *int,
		_ *model.Cursor,
	) int {
		return priceArticleRenderingComplexity(childComplexity, articleConnectionComplexitySize(first))
	}

	complexity.Mutation.PublishDraft = func(childComplexity int, _ string) int {
		return priceArticleRenderingComplexity(childComplexity, 1)
	}
	complexity.Mutation.CreateArticle = func(childComplexity int, _ model.CreateArticleInput) int {
		return priceArticleRenderingComplexity(childComplexity, 1)
	}
	complexity.Mutation.UpdateArticle = func(childComplexity int, _ string, _ model.UpdateArticleInput) int {
		return priceArticleRenderingComplexity(childComplexity, 1)
	}
	complexity.Mutation.RestoreRevision = func(childComplexity int, _ string, _ int) int {
		return priceArticleRenderingComplexity(childComplexity, 1)
	}
	complexity.Mutation.AddArticleToCategory = func(childComplexity int, _ string, _ string) int {
		return priceArticleRenderingComplexity(childComplexity, 1)
	}
	complexity.Mutation.RemoveArticleFromCategory = func(childComplexity int, _ string, _ string) int {
		return priceArticleRenderingComplexity(childComplexity, 1)
	}
}

func priceArticleRenderingComplexity(childComplexity int, articleCount int) int {
	renderedFields := childComplexity / articleRenderedHTMLComplexityMarker
	if renderedFields == 0 {
		return childComplexity
	}
	baseComplexity := childComplexity % articleRenderedHTMLComplexityMarker
	return baseComplexity + renderedFields*articleRenderedHTMLComplexityCost*articleCount
}

func articleConnectionComplexitySize(first *int) int {
	if first == nil || *first <= 0 {
		return defaultCMSPageSize
	}
	if *first > maxCMSPageSize {
		return maxCMSPageSize
	}
	return *first
}
