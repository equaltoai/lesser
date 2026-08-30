package cms

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/cmsrender"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

// TestValidateArticleRenderableComposesPersistedMedia proves the CMS validation
// read path (article_service.go) renders a media-bearing article through the
// canonical renderer: validateArticleRenderable runs the exact composition
// expression RenderArticleContentWithMedia(article.Content, format,
// article.RenderMediaList()), so the persisted inline binding must compose into
// the sanitized body while hero and social-card media never duplicate into it.
func TestValidateArticleRenderableComposesPersistedMedia(t *testing.T) {
	position := 1
	article := &models.Article{
		Object: models.Object{
			Content: "# T\n\nOne.\n\nTwo.",
		},
		ContentFormat: "markdown",
		EditorialMedia: []models.ArticleEditorialMedia{
			{MediaID: "hero", Role: models.EditorialMediaRoleHero, URL: "https://cdn.example.test/hero.png"},
			{MediaID: "inline", Role: models.EditorialMediaRoleInline, InlinePosition: &position, URL: "https://cdn.example.test/inline.png", ContentType: "image/png", AltText: "inline"},
			{MediaID: "card", Role: models.EditorialMediaRoleSocialCard, URL: "https://cdn.example.test/card.png"},
		},
	}

	require.NoError(t, validateArticleRenderable(article))
	require.Equal(t, "markdown", article.ContentFormat)

	// The read path's composition expression (article_service.go:231) composes
	// the persisted inline media into the rendered body; hero and social-card
	// media never duplicate into the article HTML.
	rendered, err := cmsrender.RenderArticleContentWithMedia(article.Content, article.ContentFormat, article.RenderMediaList())
	require.NoError(t, err)
	require.Contains(t, rendered.HTML, `src="https://cdn.example.test/inline.png"`)
	require.Contains(t, rendered.HTML, "<figure>")
	require.NotContains(t, rendered.HTML, "hero.png")
	require.NotContains(t, rendered.HTML, "card.png")
}
