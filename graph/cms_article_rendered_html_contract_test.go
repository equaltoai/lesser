package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestCMSArticleRenderedHTMLUsesCanonicalRenderer(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{}
	source := "# Canonical\n\n<script>alert('x')</script>\n\n[ok](https://example.com) [bad](javascript:alert(1))"
	article := resolver.convertCMSArticle(context.Background(), &models.Article{
		Object: models.Object{
			ID:        "https://example.com/articles/canonical",
			Type:      activitypub.ArticleType,
			Name:      "Canonical",
			Content:   source,
			Published: time.Now().UTC(),
			CreatedAt: time.Now().UTC(),
		},
		Slug:          "canonical",
		ContentFormat: "markdown",
		UpdatedAt:     time.Now().UTC(),
	}, false)

	require.NotNil(t, article)
	require.Equal(t, source, article.Content, "content remains the stored authoring source")
	renderedHTML, err := resolver.Article().RenderedHTML(context.Background(), article)
	require.NoError(t, err)
	require.NotNil(t, renderedHTML)
	require.Contains(t, *renderedHTML, `<h1 id="canonical">Canonical</h1>`)
	require.Contains(t, *renderedHTML, `href="https://example.com"`)
	require.NotContains(t, *renderedHTML, "<script")
	require.NotContains(t, *renderedHTML, "javascript:")
}

func TestCMSArticleRenderedHTMLFailsClosedOnRendererError(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{}
	article := resolver.convertCMSArticle(context.Background(), &models.Article{
		Object: models.Object{
			ID:        "https://example.com/articles/unsupported",
			Type:      activitypub.ArticleType,
			Name:      "Unsupported",
			Content:   "source that must not be presented as html",
			Published: time.Now().UTC(),
			CreatedAt: time.Now().UTC(),
		},
		Slug:          "unsupported",
		ContentFormat: "asciidoc",
		UpdatedAt:     time.Now().UTC(),
	}, false)

	require.NotNil(t, article)
	renderedHTML, err := resolver.Article().RenderedHTML(context.Background(), article)
	require.Error(t, err)
	require.Nil(t, renderedHTML)
}

func TestCMSArticleRenderedHTMLHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	renderedHTML, err := (&Resolver{}).Article().RenderedHTML(ctx, &model.Article{
		ID:               "https://example.com/articles/canceled",
		Content:          "# must not render",
		ContentFormat:    model.ContentFormatMarkdown,
		RawContentFormat: "markdown",
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, renderedHTML)
}
