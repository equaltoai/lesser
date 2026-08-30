package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

// TestArticleRenderedHTMLComposesPersistedMedia proves the GraphQL article
// read path composes the persisted inline bindings into renderedHtml.
func TestArticleRenderedHTMLComposesPersistedMedia(t *testing.T) {
	position := 1
	obj := &model.Article{
		Content:          "# T\n\nOne.\n\nTwo.",
		ContentFormat:    model.ContentFormatMarkdown,
		RawContentFormat: "markdown",
		RawEditorialMedia: []models.ArticleEditorialMedia{
			{MediaID: "inline", Role: models.EditorialMediaRoleInline, InlinePosition: &position, URL: "https://cdn.example.test/inline.png", ContentType: "image/png", AltText: "inline"},
		},
	}
	resolver := &articleResolver{&Resolver{}}
	html, err := resolver.RenderedHTML(context.Background(), obj)
	require.NoError(t, err)
	require.NotNil(t, html)
	require.Contains(t, *html, `src="https://cdn.example.test/inline.png"`)
	require.Contains(t, *html, "<figure>")

	// The hero never duplicates into published article HTML; it lives on
	// Article.featuredImage and composes only in draft previews.
	withHero := &model.Article{
		Content:          "# T\n\nOne.",
		ContentFormat:    model.ContentFormatMarkdown,
		RawContentFormat: "markdown",
		RawEditorialMedia: []models.ArticleEditorialMedia{
			{MediaID: "hero", Role: models.EditorialMediaRoleHero, URL: "https://cdn.example.test/hero.png"},
		},
	}
	heroHTML, err := resolver.RenderedHTML(context.Background(), withHero)
	require.NoError(t, err)
	require.NotContains(t, *heroHTML, "hero.png")

	// Without media the render is unchanged.
	plain := &model.Article{Content: "# T\n\nOne.", ContentFormat: model.ContentFormatMarkdown, RawContentFormat: "markdown"}
	plainHTML, err := resolver.RenderedHTML(context.Background(), plain)
	require.NoError(t, err)
	require.NotNil(t, plainHTML)
	require.Contains(t, *plainHTML, "<h1")
	require.NotContains(t, *plainHTML, "cdn.example.test")

	// Nil articles render nil.
	nilHTML, err := resolver.RenderedHTML(context.Background(), nil)
	require.NoError(t, err)
	require.Nil(t, nilHTML)
}

// TestArticleRenderedHTMLSkipsUnmintedBindings proves the GraphQL article read
// path never composes a persisted binding without a minted serving (fail-closed
// skip): the resolver maps media through the shared RenderArticleMedia helper,
// which skips empty-URL bindings.
func TestArticleRenderedHTMLSkipsUnmintedBindings(t *testing.T) {
	position := 0
	obj := &model.Article{
		Content:          "# T\n\nOne.",
		ContentFormat:    model.ContentFormatMarkdown,
		RawContentFormat: "markdown",
		RawEditorialMedia: []models.ArticleEditorialMedia{
			{MediaID: "inline", Role: models.EditorialMediaRoleInline, InlinePosition: &position, URL: "https://cdn.example.test/good.png"},
			{MediaID: "unminted", Role: models.EditorialMediaRoleInline, InlinePosition: &position, URL: " "},
		},
	}
	resolver := &articleResolver{&Resolver{}}
	html, err := resolver.RenderedHTML(context.Background(), obj)
	require.NoError(t, err)
	require.NotNil(t, html)
	require.Contains(t, *html, `src="https://cdn.example.test/good.png"`)
	require.NotContains(t, *html, "unminted")
}
